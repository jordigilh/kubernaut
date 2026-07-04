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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// E2E-FLEET-004: RO creates RR with clusterID and routes to fleet-aware workflow
// Authority: Issue #54, ADR-068
// FedRAMP: AC-6 (least privilege -- cluster-scoped workflow routing)
var _ = Describe("E2E-FLEET-004 [AC-6]: RO creates RR with clusterID and routes to fleet-aware workflow (BR-INTEGRATION-054)", Label("fleet"), func() {
	It("should route alert with cluster_id to a workflow that respects cluster scope", func() {
		payload := buildPrometheusAlertWithCluster("FleetRouting", namespace, "critical",
			"Deployment", "memory-eater", "remote-cluster")

		gatewayURL := "http://localhost:30080"
		_, body := postFleetAlertUntilAccepted(gatewayURL, payload)

		var response map[string]interface{}
		Expect(json.Unmarshal(body, &response)).To(Succeed())
		Expect(response["status"]).To(Equal("created"))

		rrName := response["remediationRequestName"].(string)

		By("Verifying RR has clusterID=remote-cluster and enters workflow processing (AC-6)")
		Eventually(func(g Gomega) {
			var rr remediationv1.RemediationRequest
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name: rrName, Namespace: namespace,
			}, &rr)).To(Succeed())
			g.Expect(rr.Spec.ClusterID).To(Equal("remote-cluster"),
				"AC-6: RR must carry cluster identity for scoped workflow routing")
			g.Expect(rr.Status.OverallPhase).ToNot(BeEmpty(),
				"RR should enter workflow processing via RO")
		}, timeout, interval).Should(Succeed())

		By("Verifying WFE carries clusterID=remote-cluster (AC-3)")
		Eventually(func(g Gomega) {
			var rr remediationv1.RemediationRequest
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name: rrName, Namespace: namespace,
			}, &rr)).To(Succeed())

			var wfeList workflowexecutionv1.WorkflowExecutionList
			g.Expect(k8sClient.List(ctx, &wfeList, client.InNamespace(namespace))).To(Succeed())

			var owned *workflowexecutionv1.WorkflowExecution
			for i := range wfeList.Items {
				for _, ref := range wfeList.Items[i].OwnerReferences {
					if ref.Kind == "RemediationRequest" && ref.UID == rr.UID {
						owned = &wfeList.Items[i]
						break
					}
				}
			}
			g.Expect(owned).ToNot(BeNil(),
				"RO should have created a WFE owned by this RR")
			g.Expect(owned.Spec.ClusterID).To(Equal("remote-cluster"),
				"AC-3: WFE must carry ClusterID for remote cluster execution routing")
		}, timeout, interval).Should(Succeed())
	})
})
