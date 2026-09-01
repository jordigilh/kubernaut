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
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// E2E-FLEET-2326-001: workflow-declared execution cluster overrides the
// signal's origin cluster (DD-FLEET-008, BR-FLEET-004, Issue #2326).
//
// This proves the real, live chain UT/IT coverage cannot: AuthWebhook's
// content-hash admission of fleet-exec-cluster-override-v1 (declaring
// execution.clusterId: prod-east in its fixture) -> KA's catalog cache
// (cache_convert.go) -> KA's workflow selection (mock-llm scenario, matched
// by a dedicated, isolated signal name) -> InvestigationResult.ExecutionClusterID
// (investigator_gates.go, catalog-authoritative, never LLM-suppliable) ->
// AIAnalysis.Status.SelectedWorkflow (response_processor.go) ->
// RemediationOrchestrator's WorkflowExecutionCreator.resolveExecutionClusterID,
// which must prefer the workflow's declared cluster over
// RemediationRequest.Spec.ClusterID.
//
// The signal fires with cluster_id=prod-west (a different registered
// cluster than the workflow's declared prod-east, and different again from
// "remote-cluster" used by 03_ro_clusterid_routing_test.go) so a WFE that
// merely echoed the RR's ClusterID -- the pre-DD-FLEET-008 default --
// cannot accidentally pass this assertion.
//
// Authority: Issue #2326, DD-FLEET-008, BR-FLEET-004
// FedRAMP: AC-6 (least privilege -- cluster-scoped workflow execution routing)
var _ = Describe("E2E-FLEET-2326-001 [AC-6]: workflow-declared execution cluster overrides signal origin cluster (BR-FLEET-004)", Label("fleet"), func() {
	It("should route WorkflowExecution to the workflow's declared execution cluster, not the signal's origin cluster", func() {
		// Issue #54 dedup-fingerprint collision, recurring: the shared
		// "memory-eater" fixture + "prod-west" is already claimed by
		// E2E-FLEET-002 (01_signal_ingestion_test.go), which produces the
		// identical SHA256(clusterID:namespace:kind:name) fingerprint (see
		// pkg/gateway/types/fingerprint.go). Every prior fix for this class
		// (01_/08_/13_/15_) used a dedicated, uniquely-named Deployment
		// instead of the shared fixture; do the same here.
		//
		// The target must exist as a real K8s object on the REMOTE cluster
		// (DD-TEST-013): Gateway's owner resolution does a live lookup and
		// drops the signal with a 400/500 when it is not found (see
		// 01_signal_ingestion_test.go for the full explanation). Mock-LLM's
		// scenario match (scenario_fleet_exec_cluster_override.go) keys off
		// SignalName only, so renaming the Deployment does not affect it.
		const targetName = "memory-eater-exec-cluster-override"
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetName,
				Namespace: namespace,
				Labels:    map[string]string{"kubernaut.ai/managed": "true"},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](0),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": targetName}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": targetName}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "app", Image: "busybox:1.36"}},
					},
				},
			},
		}
		if createErr := remoteK8sClient.Create(ctx, dep); createErr != nil && !apierrors.IsAlreadyExists(createErr) {
			Expect(createErr).NotTo(HaveOccurred(), "Failed to create %s fixture", targetName)
		}
		DeferCleanup(func() { _ = remoteK8sClient.Delete(context.Background(), dep) })

		payload := buildPrometheusAlertWithCluster("FleetExecClusterOverride2326", "critical",
			targetName, "prod-west")

		gatewayURL := urlLocalhost30080
		body := postFleetAlertUntilAccepted(gatewayURL, payload)

		var response map[string]interface{}
		Expect(json.Unmarshal(body, &response)).To(Succeed())
		Expect(response["status"]).To(Equal("created"))

		rrName := response["remediationRequestName"].(string)

		By("Verifying RR carries the signal's origin cluster (prod-west), unchanged default behavior")
		Eventually(func(g Gomega) {
			var rr remediationv1.RemediationRequest
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name: rrName, Namespace: namespace,
			}, &rr)).To(Succeed())
			g.Expect(rr.Spec.ClusterID).To(Equal("prod-west"),
				"RR must carry the signal's origin cluster identity")
			g.Expect(rr.Status.OverallPhase).ToNot(BeEmpty(),
				"RR should enter workflow processing via RO")
		}, timeout, interval).Should(Succeed())

		By("Verifying WFE carries the workflow-declared execution cluster (prod-east), overriding the signal's origin cluster (DD-FLEET-008)")
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
					if ref.Kind == remediationRequestKind && ref.UID == rr.UID {
						owned = &wfeList.Items[i]
						break
					}
				}
			}
			g.Expect(owned).ToNot(BeNil(),
				"RO should have created a WFE owned by this RR")
			g.Expect(owned.Spec.ClusterID).To(Equal("prod-east"),
				"DD-FLEET-008: WFE.Spec.ClusterID must follow the selected workflow's "+
					"declared execution.clusterId (fleet-exec-cluster-override-v1 -> prod-east), "+
					"not RemediationRequest.Spec.ClusterID (prod-west)")
		}, timeout, interval).Should(Succeed())
	})
})
