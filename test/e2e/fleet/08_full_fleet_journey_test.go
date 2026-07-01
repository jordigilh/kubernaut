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

package fleet

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// E2E-FLEET-009: Full fleet journey -- alert -> signal -> enrichment -> RR -> workflow
// Authority: Issue #54, ADR-068
// FedRAMP: AC-3, AC-4, SI-4 (end-to-end fleet remediation)
//
// This is the integration test that validates the complete multi-cluster
// remediation lifecycle using the loopback pattern. It exercises:
//   1. Gateway receives alert with cluster_id
//   2. RR is created with spec.clusterID
//   3. SP enriches the signal via MCP gateway (remote)
//   4. RO routes the RR through the workflow pipeline
var _ = Describe("E2E-FLEET-009 [AC-3, AC-4, SI-4]: Full fleet journey from alert to enrichment (BR-INTEGRATION-054)", Label("fleet"), func() {
	It("should complete the full fleet remediation pipeline: alert -> RR -> SP enrichment", func() {
		By("Step 1: Sending alert with cluster_id=loopback-cluster to Gateway (AC-4)")
		// Issue #54 flakiness fix: distinct resource name from E2E-FLEET-004
		// (03_ro_clusterid_routing_test.go), which also used "memory-eater" +
		// "loopback-cluster" and therefore produced an identical dedup fingerprint
		// (see pkg/gateway/types/fingerprint.go). Running in parallel Ginkgo
		// processes, the two specs raced for the same dedup slot.
		payload := buildPrometheusAlertWithCluster("FleetJourney", namespace, "critical",
			"Deployment", "memory-eater-journey", "loopback-cluster")

		gatewayURL := "http://localhost:30080"
		resp, err := postWithFleetAuth(
			gatewayURL+"/api/v1/signals/prometheus",
			"application/json",
			strings.NewReader(string(payload)))
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))

		body, _ := io.ReadAll(resp.Body)
		var response map[string]interface{}
		Expect(json.Unmarshal(body, &response)).To(Succeed())
		Expect(response["status"]).To(Equal("created"))

		rrName := response["remediationRequestName"].(string)

		By("Step 2: Verifying RR created with spec.clusterID=loopback-cluster (AC-3)")
		Eventually(func(g Gomega) {
			var rr remediationv1.RemediationRequest
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name: rrName, Namespace: namespace,
			}, &rr)).To(Succeed())
			g.Expect(rr.Spec.ClusterID).To(Equal("loopback-cluster"),
				"AC-3: RR must carry loopback-cluster identity")
		}, timeout, interval).Should(Succeed())

		By("Step 3: Verifying SP is created for signal enrichment")
		Eventually(func(g Gomega) {
			spList := &signalprocessingv1.SignalProcessingList{}
			g.Expect(k8sClient.List(ctx, spList, client.InNamespace(namespace))).To(Succeed())

			var foundSP *signalprocessingv1.SignalProcessing
			for i := range spList.Items {
				sp := &spList.Items[i]
				if sp.Spec.Signal.ClusterID == "loopback-cluster" &&
					sp.Spec.RemediationRequestRef.Name == rrName {
					foundSP = sp
					break
				}
			}
			g.Expect(foundSP).ToNot(BeNil(),
				"SP should be created for the fleet signal with clusterID=loopback-cluster")
		}, timeout, interval).Should(Succeed())

		By("Step 4: Waiting for SP enrichment to complete via MCP gateway (SI-4)")
		Eventually(func(g Gomega) {
			spList := &signalprocessingv1.SignalProcessingList{}
			g.Expect(k8sClient.List(ctx, spList, client.InNamespace(namespace))).To(Succeed())

			for i := range spList.Items {
				sp := &spList.Items[i]
				if sp.Spec.Signal.ClusterID == "loopback-cluster" &&
					sp.Spec.RemediationRequestRef.Name == rrName {
					g.Expect(sp.Status.KubernetesContext).ToNot(BeNil(),
						"SI-4: SP should be enriched via MCP gateway for remote cluster signal")
					g.Expect(sp.Status.KubernetesContext.DegradedMode).To(BeFalse(),
						"SI-4: enrichment via MCP gateway should not trigger degraded mode")
					return
				}
			}
			g.Expect(false).To(BeTrue(), "SP for fleet signal not found")
		}, timeout, interval).Should(Succeed())

		By("Step 5: Verifying RR progresses past signal processing phase")
		Eventually(func(g Gomega) {
			var rr remediationv1.RemediationRequest
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name: rrName, Namespace: namespace,
			}, &rr)).To(Succeed())
			g.Expect(rr.Status.OverallPhase).ToNot(BeEmpty(),
				"RR should progress through the pipeline")
		}, timeout, interval).Should(Succeed())
	})
})
