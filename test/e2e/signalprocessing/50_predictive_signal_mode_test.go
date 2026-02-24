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

// Package signalprocessing contains E2E tests for SignalProcessing controller.
//
// # Business Requirements
//
// BR-SP-106: Predictive Signal Mode Classification
//
// # Design Decisions
//
// ADR-054: Predictive Signal Mode Classification and Prompt Strategy
//
// # Test Infrastructure
//
// Uses KIND cluster with full kubernaut deployment.
// SignalProcessing controller is deployed with predictive-signal-mappings ConfigMap
// mounted at /etc/signalprocessing/predictive-signal-mappings.yaml.
package signalprocessing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

var _ = Describe("E2E-SP-106-001: Predictive Signal Mode Classification", Label("e2e", "signalmode", "signalprocessing"), func() {
	var (
		ctx       context.Context
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()

		// PARALLEL-SAFE: Unique namespace per test execution
		namespace = helpers.CreateTestNamespace(ctx, k8sClient, "sp-signalmode-e2e")

		// CLEANUP: Defer namespace deletion
		DeferCleanup(func() {
			helpers.DeleteTestNamespace(ctx, k8sClient, namespace)
		})
	})

	Context("Predictive Signal Classification (Real Controller)", func() {
		// E2E-SP-163-003: Predictive Signal Mode - SignalModeClassifier via predictive_signal_mappings
		It("E2E-SP-163-003: should classify PredictedOOMKill as predictive and normalize to OOMKilled", func() {
			// BUSINESS CONTEXT:
			// BR-SP-106: Real SP controller running in Kind cluster classifies a
			// PredictedOOMKill signal as predictive, normalizes it to OOMKilled,
			// and preserves the original type for SOC2 audit trail.
			//
			// This validates the full pipeline: ConfigMap → SignalModeClassifier → Status

			By("1. Create parent RemediationRequest")
			rr := createPredictiveTestRR(namespace, "rr-predictive-oomkill")
			rr.Spec.SignalType = "PredictedOOMKill"
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("2. Create SignalProcessing with PredictedOOMKill type")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sp-predictive-oomkill",
					Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{
						*metav1.NewControllerRef(rr, remediationv1alpha1.GroupVersion.WithKind("RemediationRequest")),
					},
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
						APIVersion: remediationv1alpha1.GroupVersion.String(),
						Kind:       "RemediationRequest",
						Name:       rr.Name,
						Namespace:  rr.Namespace,
						UID:        string(rr.UID),
					},
				Signal: signalprocessingv1alpha1.SignalData{
					Fingerprint:  rr.Spec.SignalFingerprint,
					Name:         "PredictedOOMKill",
					Severity:     "critical",
					Type:         "alert",
					Source:       "prometheus",
						TargetType:   "kubernetes",
						ReceivedTime: metav1.Now(),
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "api-server-e2e",
							Namespace: namespace,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			By("3. Wait for SP to reach Completed phase")
			Eventually(func(g Gomega) {
				var updated signalprocessingv1alpha1.SignalProcessing
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated)).To(Succeed())

				// Verify completion
				g.Expect(updated.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted),
					"SP should complete successfully with predictive signal")

				// BR-SP-106: Verify signal mode classification
				g.Expect(updated.Status.SignalMode).To(Equal("predictive"),
					"PredictedOOMKill should be classified as predictive")
				g.Expect(updated.Status.SignalName).To(Equal("OOMKilled"),
					"PredictedOOMKill should be normalized to OOMKilled for workflow catalog")
				g.Expect(updated.Status.SourceSignalName).To(Equal("PredictedOOMKill"),
					"Original signal type must be preserved for SOC2 CC7.4 audit trail")
			}, "60s", "2s").Should(Succeed())

			GinkgoWriter.Println("E2E-SP-106-001: Predictive signal mode classification validated in Kind cluster")
		})

		It("should classify reactive signals with default mode", func() {
			// BUSINESS CONTEXT:
			// BR-SP-106: Standard signals not in predictive mappings default to reactive.
			// This validates backwards compatibility in the real Kind cluster.

			By("1. Create parent RemediationRequest")
			rr := createPredictiveTestRR(namespace, "rr-reactive-oomkilled")
			rr.Spec.SignalType = "OOMKilled"
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("2. Create SignalProcessing with standard OOMKilled type")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sp-reactive-oomkilled",
					Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{
						*metav1.NewControllerRef(rr, remediationv1alpha1.GroupVersion.WithKind("RemediationRequest")),
					},
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
						APIVersion: remediationv1alpha1.GroupVersion.String(),
						Kind:       "RemediationRequest",
						Name:       rr.Name,
						Namespace:  rr.Namespace,
						UID:        string(rr.UID),
					},
				Signal: signalprocessingv1alpha1.SignalData{
					Fingerprint:  rr.Spec.SignalFingerprint,
					Name:         "OOMKilled",
					Severity:     "critical",
					Type:         "alert",
					Source:       "kubernetes",
						TargetType:   "kubernetes",
						ReceivedTime: metav1.Now(),
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "worker-e2e",
							Namespace: namespace,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			By("3. Wait for SP to reach Completed phase")
			Eventually(func(g Gomega) {
				var updated signalprocessingv1alpha1.SignalProcessing
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated)).To(Succeed())

				g.Expect(updated.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
				g.Expect(updated.Status.SignalMode).To(Equal("reactive"),
					"Standard OOMKilled should default to reactive")
				g.Expect(updated.Status.SignalName).To(Equal("OOMKilled"),
					"Reactive signal type should pass through unchanged")
				g.Expect(updated.Status.SourceSignalName).To(BeEmpty(),
					"No original type for reactive signals")
			}, "60s", "2s").Should(Succeed())

			GinkgoWriter.Println("E2E-SP-106-001: Reactive default signal mode validated in Kind cluster")
		})
	})
})

// createPredictiveTestRR creates a RemediationRequest for predictive signal mode E2E tests.
func createPredictiveTestRR(namespace, name string) *remediationv1alpha1.RemediationRequest {
	return &remediationv1alpha1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: remediationv1alpha1.RemediationRequestSpec{
			SignalFingerprint: func() string {
				h := sha256.Sum256([]byte("e2e-predictive-" + name))
				return hex.EncodeToString(h[:]) // Always exactly 64 hex chars
			}(),
			SignalName:        "E2EPredictiveAlert",
			Severity:          "critical",
			SignalType:        "alert",
			SignalSource:      "test-e2e-source",
			TargetType:        "kubernetes",
			FiringTime:        metav1.Now(),
			ReceivedTime:      metav1.Now(),
			TargetResource: remediationv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "test-e2e-pod",
				Namespace: namespace,
			},
		},
	}
}
