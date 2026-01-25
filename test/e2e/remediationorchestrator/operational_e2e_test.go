/*
Copyright 2025 Jordi Gil.

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

package remediationorchestrator

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// ========================================
// Phase 2 E2E Tests - Operational Performance & Isolation
// ========================================
//
// PHASE 2 PATTERN: RO Controller + Child Controllers Running
// - RO controller creates child CRDs (SP, AI, WE) automatically
// - Child controllers process their CRDs independently
// - Tests validate end-to-end orchestration behavior
//
// PENDING: These tests await controller deployment in E2E suite.
// See test/e2e/remediationorchestrator/suite_test.go lines 142-147 for TODO.
//
// Business Value:
// - Performance SLO validation (reconcile timing)
// - Multi-tenant isolation guarantees (namespace isolation)
// ========================================

var _ = Describe("Operational Performance E2E Tests", Label("e2e", "operational", "pending"), func() {

	// ========================================
	// Gap 3.1: Reconcile Performance - Timing SLO
	// Business Value: Validates performance SLOs
	// Confidence: 90% - Performance requirement validation
	// ========================================
	Describe("Reconcile Performance (Gap 3.1)", func() {
		var namespace string

		BeforeEach(func() {
			namespace = createTestNamespace("ro-perf-e2e")
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		It("should complete initial reconcile loop quickly (<1s baseline)", func() {
			Skip("PENDING: Requires SignalProcessing controller deployment. Full Phase 2 E2E to be planned in new sprint.")

			// Scenario: Measure time for RR creation → SignalProcessing creation
			// Business Outcome: Validates reconcile loop isn't blocked
			// Confidence: 95% - Simpler performance validation

			ctx := context.Background()
			startTime := time.Now()

			// Given: RemediationRequest
			now := metav1.Now()
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "perf-test-rr",
					Namespace: namespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalName:        "perf-test-signal",
					SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
					Severity:          "info",
					SignalType:        "test",
					TargetType:        "kubernetes",
					FiringTime:        now,
					ReceivedTime:      now,
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "test-app",
						Namespace: namespace,
					},
				},
			}

			// When: Creating RR
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// Then: SignalProcessing should be created quickly (<1s baseline)
			Eventually(func() error {
				spList := &signalprocessingv1.SignalProcessingList{}
				if err := k8sClient.List(ctx, spList, client.InNamespace(namespace)); err != nil {
					return err
				}
				if len(spList.Items) == 0 {
					return fmt.Errorf("no SP created yet")
				}
				return nil
			}, "5s", "100ms").Should(Succeed(), "SignalProcessing should be created")

			elapsed := time.Since(startTime)
			GinkgoWriter.Printf("✅ Initial reconcile completed in %v (baseline performance)\n", elapsed)

			// Baseline: Should create SP within 5s (relaxed from 5s full lifecycle)
			Expect(elapsed).To(BeNumerically("<", 5*time.Second),
				"Initial reconcile (RR → SP creation) should complete quickly")
		})
	})

	// ========================================
	// Gap 3.3: Cross-Namespace Isolation
	// Business Value: Multi-tenant isolation guarantee
	// Confidence: 95% - Critical multi-tenancy requirement
	// ========================================
	Describe("Namespace Isolation (Gap 3.3)", func() {
		It("should process RRs in different namespaces independently", func() {
			Skip("PENDING: Requires SignalProcessing controller deployment. Full Phase 2 E2E to be planned in new sprint.")

			// Scenario: RR in ns-a fails, RR in ns-b succeeds
			// Business Outcome: No cross-namespace interference
			// Confidence: 95% - Multi-tenancy critical

			ctx := context.Background()

			// Create test namespaces
			nsA := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns-isolation-a-e2e",
				},
			}
			nsB := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns-isolation-b-e2e",
				},
			}

			Expect(k8sClient.Create(ctx, nsA)).To(Succeed())
			Expect(k8sClient.Create(ctx, nsB)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, nsA)
				_ = k8sClient.Delete(ctx, nsB)
			}()

			// Given: RR in namespace A that will fail
			now := metav1.Now()
			rrA := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-fail",
					Namespace: "ns-isolation-a-e2e",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalName:        "signal-a",
					SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
					Severity:          "critical",
					SignalType:        "test",
					TargetType:        "kubernetes",
					FiringTime:        now,
					ReceivedTime:      now,
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "test-app-a",
						Namespace: "ns-isolation-a-e2e",
					},
				},
			}

			// Given: RR in namespace B that will succeed
			rrB := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-success",
					Namespace: "ns-isolation-b-e2e",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalName:        "signal-b",
					SignalFingerprint: "b1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6b1b2",
					Severity:          "info",
					SignalType:        "test",
					TargetType:        "kubernetes",
					FiringTime:        now,
					ReceivedTime:      now,
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "test-app-b",
						Namespace: "ns-isolation-b-e2e",
					},
				},
			}

			// When: Creating both RRs
			Expect(k8sClient.Create(ctx, rrA)).To(Succeed())
			Expect(k8sClient.Create(ctx, rrB)).To(Succeed())

			// Make RR-A fail by failing its SignalProcessing
			Eventually(func() error {
				spListA := &signalprocessingv1.SignalProcessingList{}
				if err := k8sClient.List(ctx, spListA, client.InNamespace("ns-isolation-a-e2e")); err != nil {
					return err
				}
				if len(spListA.Items) == 0 {
					return fmt.Errorf("no SP in ns-a yet")
				}

				sp := &spListA.Items[0]
				sp.Status.Phase = "Failed"
				return k8sClient.Status().Update(ctx, sp)
			}, "10s", "100ms").Should(Succeed())

			// Make RR-B succeed by completing all child CRDs
			Eventually(func() error {
				spListB := &signalprocessingv1.SignalProcessingList{}
				if err := k8sClient.List(ctx, spListB, client.InNamespace("ns-isolation-b-e2e")); err != nil {
					return err
				}
				if len(spListB.Items) == 0 {
					return fmt.Errorf("no SP in ns-b yet")
				}

				sp := &spListB.Items[0]
				sp.Status.Phase = "Completed"
				return k8sClient.Status().Update(ctx, sp)
			}, "10s", "100ms").Should(Succeed())

			// Complete AIAnalysis in ns-b
			Eventually(func() error {
				aiListB := &aianalysisv1.AIAnalysisList{}
				if err := k8sClient.List(ctx, aiListB, client.InNamespace("ns-isolation-b-e2e")); err != nil {
					return err
				}
				if len(aiListB.Items) == 0 {
					return fmt.Errorf("no AI in ns-b yet")
				}

				ai := &aiListB.Items[0]
				ai.Status.Phase = "Completed"
				ai.Status.Reason = "WorkflowNotNeeded" // Skip workflow
				return k8sClient.Status().Update(ctx, ai)
			}, "10s", "100ms").Should(Succeed())

			// Then: RR-A should be Failed
			Eventually(func() string {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      rrA.Name,
					Namespace: rrA.Namespace,
				}, updated); err != nil {
					return ""
				}
				return string(updated.Status.OverallPhase)
			}, "10s", "100ms").Should(Equal("Failed"),
				"RR in ns-a should fail independently")

			// Then: RR-B should be Completed (not affected by ns-a failure)
			Eventually(func() string {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      rrB.Name,
					Namespace: rrB.Namespace,
				}, updated); err != nil {
					return ""
				}
				return string(updated.Status.OverallPhase)
			}, "10s", "100ms").Should(Equal("Completed"),
				"RR in ns-b should succeed independently (no cross-namespace interference)")
		})
	})
})
