/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package shared

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL dot-import convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Ginkgo/Gomega DSL dot-import convention

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
)

// InventoryProjection proves the GitOps (MCP-sourced) lane of the FMC
// inventory projection end to end -- E2E-FMC-INV-2661-002 in
// docs/tests/2661/TEST_PLAN.md §8 Tier 3 -- with no ACM present anywhere in
// the lane. The same surface the ACM source adapter feeds (E2E-FMC-INV-2661-001,
// assessed separately on helios08) is here produced purely by the real
// Keycloak -> gateway -> kube-mcp-server -> fenced Valkey commit pipeline,
// proving the source abstraction (DD-FLEET-017 D4.1a rule 1: an absent
// `sources` block behaves exactly as today's MCP-only deployment) and that
// `empty` is only ever emitted from a proven partition.
//
// {ScenarioPrefix}-017, e.g. E2E-FMC-054-017 (Kuadrant) /
// E2E-FMC-EAIGW-054-017 (EAIGW).
//
// Authority: Issue #2661, DD-FLEET-017, BR-INTEGRATION-065, BR-API-001,
// BR-SCOPE-001, BR-SECURITY-017, BR-SAFETY-001.
func InventoryProjection(h *Harness, v Variant) bool {
	return Describe(fmt.Sprintf(
		"%s-017 / E2E-FMC-INV-2661-002: GitOps lane serves the FMC inventory projection from the real MCP pipeline with no ACM present",
		v.ScenarioPrefix(),
	), Ordered, Label("inventory"), func() {
		var (
			testNS  *corev1.Namespace
			emptyNS *corev1.Namespace
			svcName string
			// svcIdent carries the fixture Service's expected canonical wire
			// identity (group "" = core/v1), compared field-by-field against
			// the sanitized projection and used to derive its opaque ID.
			svcIdent inventory.Resource
		)

		BeforeAll(func() {
			testNS = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("%s-inv-%d", v.ResourcePrefix(), time.Now().UnixNano()),
					Labels: map[string]string{
						"kubernaut.ai/managed": "true",
					},
				},
			}
			Expect(h.K8sClient.Create(h.Ctx, testNS)).To(Succeed())
			DeferCleanup(func() {
				_ = h.K8sClient.Delete(h.Ctx, testNS)
			})

			// A managed namespace that deliberately contains NO resources of
			// any synced kind: its namespace-filtered, zero-item read must be
			// `empty` -- the state only a proven partition may emit.
			emptyNS = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("%s-inv-empty-%d", v.ResourcePrefix(), time.Now().UnixNano()),
					Labels: map[string]string{
						"kubernaut.ai/managed": "true",
					},
				},
			}
			Expect(h.K8sClient.Create(h.Ctx, emptyNS)).To(Succeed())
			DeferCleanup(func() {
				_ = h.K8sClient.Delete(h.Ctx, emptyNS)
			})

			svcName = v.ResourcePrefix() + "-inv-svc"
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: testNS.Name,
					// No kubernaut.ai/managed label on the resource itself:
					// the namespace carries it (ADR-053 inheritance, same
					// shape the sync journey's #2311 spec drives).
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 80}},
				},
			}
			Expect(h.K8sClient.Create(h.Ctx, svc)).To(Succeed())
			// A second Service in the same managed namespace: the paging It
			// reads the whole loopback-cluster partition with limit 1 and
			// must observe a cursor and a distinct second page -- under
			// GINKGO_FOCUS this suite's own fixtures are the only managed
			// items, so the partition needs at least two by construction.
			svcB := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      v.ResourcePrefix() + "-inv-svc-b",
					Namespace: testNS.Name,
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 80}},
				},
			}
			Expect(h.K8sClient.Create(h.Ctx, svcB)).To(Succeed())
			svcIdent = inventory.Resource{
				ClusterID: "loopback-cluster",
				Version:   "v1",
				Kind:      "Service",
				Namespace: testNS.Name,
				Name:      svcName,
			}
		})

		It("configures no inventory source -- the deployed FMC runs the MCP collector with no ACM present (DD-FLEET-017 D4.1a rule 1)", func() {
			var cm corev1.ConfigMap
			Expect(h.K8sClient.Get(h.Ctx, types.NamespacedName{
				Namespace: h.Namespace,
				Name:      "fleetmetadatacache-config",
			}, &cm)).To(Succeed(), "the lane's FMC ConfigMap must be readable")

			raw, ok := cm.Data["config.yaml"]
			Expect(ok).To(BeTrue(), "config.yaml key must exist in fleetmetadatacache-config")

			var parsed map[string]any
			Expect(json.Unmarshal([]byte(raw), &parsed)).To(Succeed())
			Expect(parsed).NotTo(HaveKey("sources"),
				"the GitOps lane's FMC config must not select a source -- an absent sources block is the "+
					"MCP-only shape whose inventory surface this spec proves (no ACM is deployed in this lane)")
		})

		It("lists the managed Service with freshness=fresh from the real MCP pipeline, sanitized to the canonical projection (BR-API-001, BR-SECURITY-017)", func() {
			client := fmc.NewHTTPClient(h.FMCAPIBaseURL, fmc.WithHTTPClient(h.FMCHTTPClient))

			var page inventory.Page
			Eventually(func(g Gomega) {
				var err error
				page, err = client.ListManagedResources(h.Ctx, inventory.Filters{
					ClusterID: "loopback-cluster",
					Namespace: testNS.Name,
					Kind:      "Service",
					Name:      svcName,
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(page.State).To(Equal(inventory.StateSuccess))
			}, SyncTimeout, Interval).Should(Succeed(),
				"the MCP-sourced partition must serve the same inventory surface the ACM lane serves")

			Expect(page.Completeness).To(Equal(inventory.CompletenessComplete))
			Expect(page.Freshness.State).To(Equal(inventory.FreshnessFresh),
				"freshness must derive from the proof state of a clean MCP-synced generation, not from query time")
			Expect(page.Freshness.AsOf).NotTo(BeZero())

			Expect(page.Items).To(HaveLen(1), "the namespace-filtered read must return exactly the fixture Service")
			item := page.Items[0]
			Expect(item.ClusterID).To(Equal("loopback-cluster"))
			Expect(item.Group).To(BeEmpty(), "core/v1 Service carries no group")
			Expect(item.Version).To(Equal("v1"))
			Expect(item.Kind).To(Equal("Service"))
			Expect(item.Namespace).To(Equal(testNS.Name))
			Expect(item.Name).To(Equal(svcName))
			Expect(item.Scope.Included).To(BeTrue(), "namespace-inherited managed state must project as included")
			Expect(item.Scope.Source).To(Equal(inventory.ScopeSourceNamespaceLabel))
			Expect(item.ID).To(Equal(inventory.CanonicalResourceID(item.Identity())),
				"the opaque ID must be the canonical encoding of the wire identity")

			// BR-SECURITY-017: the raw wire body is metadata-only -- no
			// labels, annotations, object data, spec, status or token material
			// beyond the typed sanitized schema.
			req, err := http.NewRequestWithContext(h.Ctx, http.MethodGet,
				h.FMCAPIBaseURL+fmc.InventoryPath+"?cluster_id=loopback-cluster&namespace="+testNS.Name+"&kind=Service",
				http.NoBody)
			Expect(err).NotTo(HaveOccurred())
			resp, err := h.FMCHTTPClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			for _, leaked := range []string{"labels", "annotations", "\"data\"", "spec", "status", "token", "secret"} {
				Expect(string(body)).NotTo(ContainSubstring(leaked),
					"the sanitized inventory projection must not carry %q on the wire", leaked)
			}
		})

		It("pages the projection with a cursor and reads one exact resource by its opaque ID (BR-API-001)", func() {
			client := fmc.NewHTTPClient(h.FMCAPIBaseURL, fmc.WithHTTPClient(h.FMCHTTPClient))

			// Page the whole loopback-cluster partition (unfiltered except by
			// cluster) with limit 1 so the cursor path is genuinely exercised:
			// the first page carries exactly one item and a next_cursor, and
			// the second page starts after it with no overlap.
			first, err := client.ListManagedResources(h.Ctx, inventory.Filters{
				ClusterID: "loopback-cluster",
				Limit:     1,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(first.State).To(Equal(inventory.StateSuccess))
			Expect(first.Items).To(HaveLen(1))
			Expect(first.NextCursor).NotTo(BeEmpty(), "a limited read with more items must carry a cursor")

			second, err := client.ListManagedResources(h.Ctx, inventory.Filters{
				ClusterID: "loopback-cluster",
				Limit:     1,
				Cursor:    first.NextCursor,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(second.State).To(Equal(inventory.StateSuccess))
			Expect(second.Items).To(HaveLen(1))
			Expect(second.Items[0].ID).NotTo(Equal(first.Items[0].ID),
				"the cursor must resume after the last served item, never repeat it")

			// Exact read by the canonical opaque ID: the same sanitized
			// resource comes back.
			exact, err := client.GetManagedResource(h.Ctx, "loopback-cluster", inventory.CanonicalResourceID(svcIdent.Identity()))
			Expect(err).NotTo(HaveOccurred(), "one exact resource read must succeed against the proven partition")
			Expect(exact.Identity()).To(Equal(svcIdent.Identity()))
			Expect(exact.Scope.Included).To(BeTrue())
		})

		It("emits empty only from a proven partition -- never for an unregistered cluster (BR-SAFETY-001, R2)", func() {
			client := fmc.NewHTTPClient(h.FMCAPIBaseURL, fmc.WithHTTPClient(h.FMCHTTPClient))

			By("zero-item read on a proven partition -> empty + fresh")
			Eventually(func(g Gomega) {
				page, err := client.ListManagedResources(h.Ctx, inventory.Filters{
					ClusterID: "loopback-cluster",
					Namespace: emptyNS.Name,
					Kind:      "Service",
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(page.State).To(Equal(inventory.StateEmpty),
					"a managed namespace with no synced resources reads empty from the proven partition")
				g.Expect(page.Completeness).To(Equal(inventory.CompletenessComplete))
				g.Expect(page.Freshness.State).To(Equal(inventory.FreshnessFresh))
				g.Expect(page.Items).To(BeEmpty())
				g.Expect(page.NextCursor).To(BeEmpty())
			}, SyncTimeout, Interval).Should(Succeed())

			By("unregistered cluster -> state-only unavailable, never empty")
			page, err := client.ListManagedResources(h.Ctx, inventory.Filters{
				ClusterID: v.ResourcePrefix() + "-unregistered-2661",
			})
			Expect(err).To(HaveOccurred(),
				"an unregistered cluster must be a read error, not a successful claim about inventory")
			Expect(page.State).To(Equal(inventory.StateUnavailable),
				"an unknown partition must fail closed -- \"nothing managed\" would be a false inventory claim (R2)")
			Expect(page.Items).To(BeEmpty())
			Expect(page.NextCursor).To(BeEmpty())
			Expect(string(page.Freshness.State)).To(BeEmpty(),
				"no freshness claim may be invented for an unproven partition")
		})
	})
}
