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
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// E2E-FLEET-009: Full fleet journey -- alert -> signal -> enrichment -> RR -> workflow
// Authority: Issue #54, ADR-068
// FedRAMP: AC-3, AC-4, SI-4 (end-to-end fleet remediation)
//
// This is the integration test that validates the complete multi-cluster
// remediation lifecycle against the genuinely remote cluster (AllRegistrationsRemote,
// DD-TEST-013). It exercises:
//  1. Gateway receives alert with cluster_id
//  2. RR is created with spec.clusterID
//  3. SP enriches the signal via MCP gateway (remote)
//  4. RO routes the RR through the workflow pipeline
var _ = Describe("E2E-FLEET-009 [AC-3, AC-4, SI-4]: Full fleet journey from alert to enrichment (BR-INTEGRATION-054)", Label("fleet"), func() {
	It("should complete the full fleet remediation pipeline: alert -> RR -> SP enrichment", func() {
		By("Step 1: Sending alert with cluster_id=remote-cluster to Gateway (AC-4)")
		// Issue #54 flakiness fix: distinct resource name from E2E-FLEET-004
		// (03_ro_clusterid_routing_test.go), which also used "memory-eater" +
		// "remote-cluster" and therefore produced an identical dedup fingerprint
		// (see pkg/gateway/types/fingerprint.go). Running in parallel Ginkgo
		// processes, the two specs raced for the same dedup slot.
		//
		// The renamed target must exist as a real K8s object: Gateway's owner
		// resolution does a live lookup of the target resource and drops the
		// signal with a 400/500 when it is not found (see the equivalent note
		// in 01_signal_ingestion_test.go for the full explanation).
		const targetName = "memory-eater-journey"
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetName,
				Namespace: namespace,
				// BR-SCOPE-001/ADR-053: label the resource directly (see the detailed
				// note in 01_signal_ingestion_test.go for why the namespace-level
				// fallback alone was not sufficient).
				Labels: map[string]string{"kubernaut.ai/managed": "true"},
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
		// Created on the REMOTE cluster (DD-TEST-013): see the equivalent note
		// in 01_signal_ingestion_test.go.
		if createErr := remoteK8sClient.Create(ctx, dep); createErr != nil && !apierrors.IsAlreadyExists(createErr) {
			Expect(createErr).NotTo(HaveOccurred(), "Failed to create %s fixture", targetName)
		}
		DeferCleanup(func() { _ = remoteK8sClient.Delete(context.Background(), dep) })

		payload := buildPrometheusAlertWithCluster("FleetJourney", namespace, "critical",
			"Deployment", targetName, "remote-cluster")

		gatewayURL := urlLocalhost30080
		_, body := postFleetAlertUntilAccepted(gatewayURL, payload)

		var response map[string]interface{}
		Expect(json.Unmarshal(body, &response)).To(Succeed())
		Expect(response["status"]).To(Equal("created"))

		rrName := response["remediationRequestName"].(string)

		By("Step 2: Verifying RR created with spec.clusterID=remote-cluster (AC-3)")
		Eventually(func(g Gomega) {
			var rr remediationv1.RemediationRequest
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name: rrName, Namespace: namespace,
			}, &rr)).To(Succeed())
			g.Expect(rr.Spec.ClusterID).To(Equal("remote-cluster"),
				"AC-3: RR must carry remote-cluster identity")
		}, timeout, interval).Should(Succeed())

		By("Step 3: Verifying SP is created for signal enrichment")
		Eventually(func(g Gomega) {
			spList := &signalprocessingv1.SignalProcessingList{}
			g.Expect(k8sClient.List(ctx, spList, client.InNamespace(namespace))).To(Succeed())

			var foundSP *signalprocessingv1.SignalProcessing
			for i := range spList.Items {
				sp := &spList.Items[i]
				if sp.Spec.Signal.ClusterID == "remote-cluster" &&
					sp.Spec.RemediationRequestRef.Name == rrName {
					foundSP = sp
					break
				}
			}
			g.Expect(foundSP).ToNot(BeNil(),
				"SP should be created for the fleet signal with clusterID=remote-cluster")
		}, timeout, interval).Should(Succeed())

		By("Step 4: Waiting for SP enrichment to complete via MCP gateway (SI-4)")
		Eventually(func(g Gomega) {
			spList := &signalprocessingv1.SignalProcessingList{}
			g.Expect(k8sClient.List(ctx, spList, client.InNamespace(namespace))).To(Succeed())

			for i := range spList.Items {
				sp := &spList.Items[i]
				if sp.Spec.Signal.ClusterID == "remote-cluster" &&
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
