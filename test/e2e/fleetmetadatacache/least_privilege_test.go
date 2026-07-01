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

package fleetmetadatacache

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// canI runs `kubectl auth can-i <verb> <resource>` impersonating the given
// ServiceAccount and returns true if allowed. Uses --as (SubjectAccessReview
// via impersonation) rather than fetching a real token, since the assertion
// is about RBAC policy, not authentication.
func canI(verb, resource, asServiceAccount string) bool {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"auth", "can-i", verb, resource,
		"--as", fmt.Sprintf("system:serviceaccount:%s:%s", namespace, asServiceAccount))
	out, _ := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)) == "yes"
}

// E2E-FMC-054-011: Proves FMC enforces least-privilege scope boundaries at
// two layers:
//  1. The scope-check API itself (default-deny for unlabeled resources and
//     unknown clusters -- SC-7 boundary protection).
//  2. FMC's own ServiceAccount RBAC surface (AC-6 least privilege) --
//     FMC can only get/list/watch MCPServerRegistrations and Gateway API
//     objects, and has zero access to core cluster resources (Pods, Secrets)
//     it doesn't need for its own operation. This is verified against the
//     ClusterRole "fleetmetadatacache" (test/infrastructure/fleet_e2e.go).
//
// Authority: Issue #54, ADR-068 (AC-6 least privilege, SC-7 boundary
// protection), BR-INTEGRATION-065.
var _ = Describe("E2E-FMC-054-011: FMC enforces least-privilege scope boundaries", Ordered, func() {
	var testNS *corev1.Namespace

	BeforeAll(func() {
		testNS = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("fmc-e2e-unmanaged-%d", time.Now().UnixNano()),
			},
		}
		Expect(k8sClient.Create(ctx, testNS)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, testNS)
		})
	})

	It("returns managed=false for a resource without the kubernaut.ai/managed label (default-deny)", func() {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fmc-e2e-unmanaged-svc",
				Namespace: testNS.Name,
				// Deliberately no kubernaut.ai/managed label.
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Port: 80}},
			},
		}
		Expect(k8sClient.Create(ctx, svc)).To(Succeed())

		Eventually(func(g Gomega) {
			managed := scopeCheck(g, "loopback-cluster", "", "v1", "Service", testNS.Name, "fmc-e2e-unmanaged-svc")
			g.Expect(managed).To(BeFalse(),
				"unlabeled resource must never be reported as managed")
		}, timeout, interval).Should(Succeed())
	})

	It("returns managed=false for a resource in an unregistered cluster (SC-7 boundary enforcement)", func() {
		Eventually(func(g Gomega) {
			managed := scopeCheck(g, "totally-unknown-cluster", "", "v1", "Service", testNS.Name, "anything")
			g.Expect(managed).To(BeFalse(),
				"scope check against an unregistered cluster must fail closed")
		}, timeout, interval).Should(Succeed())
	})

	// This proves a boundary transition no lower tier can prove: IT-FLEET-VALKEY-003
	// only proves Valkey's own TTL eviction mechanics against a manually-seeded key
	// (see valkey_scope_cache_test.go) -- it never proves that FMC's real sync
	// pipeline actually STOPS refreshing a key once the label disappears. Without
	// this, a stale "managed=true" cache entry (never refreshed, but also never
	// actively invalidated) would be a genuine SC-7 boundary leak: a resource an
	// operator explicitly un-scoped from Kubernaut would keep granting Gateway/RO
	// access until an operator noticed and manually flushed Valkey.
	It("stops reporting managed=true once the kubernaut.ai/managed label is removed (SC-7, real resync)", func() {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fmc-e2e-delabeled-svc",
				Namespace: testNS.Name,
				Labels:    map[string]string{"kubernaut.ai/managed": "true"},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Port: 80}},
			},
		}
		Expect(k8sClient.Create(ctx, svc)).To(Succeed())

		By("Confirming FMC's real sync pipeline first marks it managed")
		Eventually(func(g Gomega) {
			managed := scopeCheck(g, "loopback-cluster", "", "v1", "Service", testNS.Name, "fmc-e2e-delabeled-svc")
			g.Expect(managed).To(BeTrue(), "resource must be marked managed before the label is removed")
		}, fmcSyncTimeout, interval).Should(Succeed())

		By("Removing the kubernaut.ai/managed label")
		Eventually(func(g Gomega) error {
			latest := &corev1.Service{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), latest); err != nil {
				return err
			}
			delete(latest.Labels, "kubernaut.ai/managed")
			return k8sClient.Update(ctx, latest)
		}, timeout, interval).Should(Succeed())

		By("Confirming FMC's cache entry is not refreshed and eventually expires (SC-7 boundary re-closes)")
		Eventually(func(g Gomega) {
			managed := scopeCheck(g, "loopback-cluster", "", "v1", "Service", testNS.Name, "fmc-e2e-delabeled-svc")
			g.Expect(managed).To(BeFalse(),
				"a de-labeled resource must stop being reported as managed once its cache key's TTL lapses")
		}, fmcSyncTimeout, interval).Should(Succeed())
	})

	It("restricts FMC's ServiceAccount to read-only MCP Gateway resources (AC-6 least privilege)", func() {
		const fmcSA = "fleetmetadatacache"

		By("allowing read access to the resources FMC needs")
		Expect(canI("get", "mcpserverregistrations.mcp.kuadrant.io", fmcSA)).To(BeTrue(),
			"FMC must be able to get MCPServerRegistrations to discover fleet clusters")
		Expect(canI("list", "mcpserverregistrations.mcp.kuadrant.io", fmcSA)).To(BeTrue())
		Expect(canI("watch", "mcpserverregistrations.mcp.kuadrant.io", fmcSA)).To(BeTrue())
		Expect(canI("list", "gateways.gateway.networking.k8s.io", fmcSA)).To(BeTrue())
		Expect(canI("list", "httproutes.gateway.networking.k8s.io", fmcSA)).To(BeTrue())

		By("denying write access to the resources FMC only needs to read")
		Expect(canI("create", "mcpserverregistrations.mcp.kuadrant.io", fmcSA)).To(BeFalse(),
			"FMC has no business creating MCPServerRegistrations")
		Expect(canI("delete", "mcpserverregistrations.mcp.kuadrant.io", fmcSA)).To(BeFalse())

		By("denying access to unrelated core resources (FMC reads remote-cluster resources via kube-mcp-server, not local RBAC)")
		Expect(canI("list", "pods", fmcSA)).To(BeFalse(),
			"FMC must not have direct local RBAC access to Pods -- it reads remote clusters via kube-mcp-server")
		Expect(canI("list", "secrets", fmcSA)).To(BeFalse(),
			"FMC must not have direct local RBAC access to Secrets")
		Expect(canI("list", "deployments.apps", fmcSA)).To(BeFalse())
	})
})
