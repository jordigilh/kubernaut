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
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL dot-import convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Ginkgo/Gomega DSL dot-import convention

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

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
			DeferCleanup(func() error {
				if err := h.K8sClient.Delete(h.Ctx, testNS); err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				return nil
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
			DeferCleanup(func() error {
				if err := h.K8sClient.Delete(h.Ctx, emptyNS); err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				return nil
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
			// A group-ful kind (apps/v1) in the same managed namespace:
			// core-only kinds cannot discriminate a combined `apiversion`
			// value from separate group/version wire fields (AGENTS.md
			// Review Requirements), so the CI lane needs one group-ful
			// fixture of its own — replicas 0, never scheduled, inclusion
			// from the namespace label like the Services.
			zero := int32(0)
			deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
				Name:      v.ResourcePrefix() + "-inv-deploy",
				Namespace: testNS.Name,
			}, Spec: appsv1.DeploymentSpec{
				Replicas: &zero,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": v.ResourcePrefix() + "-inv"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": v.ResourcePrefix() + "-inv"}},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name:  "placeholder",
						Image: "registry.k8s.io/pause:3.10",
					}}},
				},
			}}
			Expect(h.K8sClient.Create(h.Ctx, deploy)).To(Succeed())
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
			Expect(yaml.Unmarshal([]byte(raw), &parsed)).To(Succeed())
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
				// A success page can still be stale (proof ageing between
				// syncs): keep polling until the page is fresh, exactly like
				// the empty-result poll below, instead of failing the outer
				// freshness assertion on the first success.
				g.Expect(page.Freshness.State).To(Equal(inventory.FreshnessFresh))
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

			By("reading the group-ful fixture with its separate group/version wire fields")
			var deployPage inventory.Page
			Eventually(func(g Gomega) {
				var err error
				deployPage, err = client.ListManagedResources(h.Ctx, inventory.Filters{
					ClusterID: "loopback-cluster",
					Namespace: testNS.Name,
					Kind:      "Deployment",
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(deployPage.State).To(Equal(inventory.StateSuccess))
				g.Expect(deployPage.Freshness.State).To(Equal(inventory.FreshnessFresh))
			}, SyncTimeout, Interval).Should(Succeed())
			Expect(deployPage.Items).To(HaveLen(1))
			deploy := deployPage.Items[0]
			Expect(deploy.Group).To(Equal("apps"),
				"the group-ful kind proves the separate group/version wire shape end to end (AGENTS.md fixture fidelity)")
			Expect(deploy.Version).To(Equal("v1"))
			Expect(deploy.Kind).To(Equal("Deployment"))
			Expect(deploy.Namespace).To(Equal(testNS.Name))
			Expect(deploy.Scope.Included).To(BeTrue())
			Expect(deploy.Scope.Source).To(Equal(inventory.ScopeSourceNamespaceLabel))

			// BR-SECURITY-017: the raw wire body is metadata-only -- no
			// labels, annotations, object data, spec, status or token material
			// beyond the typed sanitized schema. Polled for the same
			// proof-window reason as the reads above; the leak assertions run
			// on the final iteration's body.
			req, err := http.NewRequestWithContext(h.Ctx, http.MethodGet,
				h.FMCAPIBaseURL+fmc.InventoryPath+"?cluster_id=loopback-cluster&namespace="+testNS.Name+"&kind=Service",
				http.NoBody)
			Expect(err).NotTo(HaveOccurred())
			var body []byte
			Eventually(func(g Gomega) {
				resp, doErr := h.FMCHTTPClient.Do(req)
				g.Expect(doErr).NotTo(HaveOccurred())
				read, readErr := io.ReadAll(resp.Body)
				closeErr := resp.Body.Close()
				g.Expect(readErr).NotTo(HaveOccurred())
				g.Expect(closeErr).NotTo(HaveOccurred())
				g.Expect(resp.StatusCode).To(Equal(http.StatusOK))
				body = read
			}, SyncTimeout, Interval).Should(Succeed())
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
			//
			// The paging and exact reads are polled, not single-shot: in a
			// full parallel CI suite the proof window legitimately turns over
			// between reads (stale -> unavailable until the next 10 s sync
			// commits, the UT-FMC-INV-2661-003 model) -- the same churn the
			// list It above already tolerates. The assertions are unchanged
			// and all execute on the final iteration's pages.
			var first, second inventory.Page
			Eventually(func(g Gomega) {
				var err error
				first, err = client.ListManagedResources(h.Ctx, inventory.Filters{
					ClusterID: "loopback-cluster",
					Limit:     1,
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(first.State).To(Equal(inventory.StateSuccess))
				g.Expect(first.Items).To(HaveLen(1))
				g.Expect(first.NextCursor).NotTo(BeEmpty(), "a limited read with more items must carry a cursor")

				second, err = client.ListManagedResources(h.Ctx, inventory.Filters{
					ClusterID: "loopback-cluster",
					Limit:     1,
					Cursor:    first.NextCursor,
				})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(second.State).To(Equal(inventory.StateSuccess))
				g.Expect(second.Items).To(HaveLen(1))
				g.Expect(second.Items[0].ID).NotTo(Equal(first.Items[0].ID),
					"the cursor must resume after the last served item, never repeat it")
			}, SyncTimeout, Interval).Should(Succeed())

			// Exact read by the canonical opaque ID: the same sanitized
			// resource comes back.
			var exact inventory.Resource
			Eventually(func(g Gomega) {
				var err error
				exact, err = client.GetManagedResource(h.Ctx, "loopback-cluster", inventory.CanonicalResourceID(svcIdent.Identity()))
				g.Expect(err).NotTo(HaveOccurred(), "one exact resource read must succeed against the proven partition")
				g.Expect(exact.Identity()).To(Equal(svcIdent.Identity()))
				g.Expect(exact.Scope.Included).To(BeTrue())
			}, SyncTimeout, Interval).Should(Succeed())
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
