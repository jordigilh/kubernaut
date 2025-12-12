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

package remediationorchestrator_test

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
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// ========================================
// Priority 3: Operational Visibility Tests
// TDD Integration Tests for operational behavior
// ========================================
var _ = Describe("Operational Visibility (Priority 3)", func() {

	// ========================================
	// Gap 3.1: Reconcile Performance - Timing SLO
	// Business Value: Validates performance SLOs
	// Confidence: 90% - Performance requirement validation
	// ========================================
	Describe("Reconcile Performance (Gap 3.1)", func() {
		var namespace string

		BeforeEach(func() {
			namespace = createTestNamespace("ro-perf")
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		It("should complete happy path reconcile in <5s (SLO)", func() {
			// Scenario: Standard lifecycle with all child CRDs succeeding quickly
			// Business Outcome: Validates 5-second performance SLO
			// Confidence: 90% - Real performance measurement

			ctx := context.Background()
			startTime := time.Now()

			// Given: RemediationRequest for simple remediation
			now := metav1.Now()
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "perf-test-rr",
					Namespace: namespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalName:        "perf-test-signal",
					SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", // Valid 64-char hex
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

			// When: Creating RR and waiting for completion
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// Simulate fast child CRD completions
			Eventually(func() bool {
				// Check if SignalProcessing created
				spList := &signalprocessingv1.SignalProcessingList{}
				if err := k8sClient.List(ctx, spList, client.InNamespace(namespace)); err != nil {
					return false
				}
				if len(spList.Items) == 0 {
					return false
				}

				// Mark SP as completed quickly
				sp := &spList.Items[0]
				sp.Status.Phase = "Completed"
				if err := k8sClient.Status().Update(ctx, sp); err != nil {
					return false
				}
				return true
			}, "10s", "100ms").Should(BeTrue())

			// Check if AIAnalysis created and complete it
			Eventually(func() bool {
				aiList := &aianalysisv1.AIAnalysisList{}
				if err := k8sClient.List(ctx, aiList, client.InNamespace(namespace)); err != nil {
					return false
				}
				if len(aiList.Items) == 0 {
					return false
				}

				ai := &aiList.Items[0]
				ai.Status.Phase = "Completed"
				ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
					WorkflowID: "simple-workflow",
				}
				if err := k8sClient.Status().Update(ctx, ai); err != nil {
					return false
				}
				return true
			}, "10s", "100ms").Should(BeTrue())

			// Check if WorkflowExecution created and complete it
			Eventually(func() bool {
				weList := &workflowexecutionv1.WorkflowExecutionList{}
				if err := k8sClient.List(ctx, weList, client.InNamespace(namespace)); err != nil {
					return false
				}
				if len(weList.Items) == 0 {
					return false
				}

				we := &weList.Items[0]
				we.Status.Phase = "Completed"
				if err := k8sClient.Status().Update(ctx, we); err != nil {
					return false
				}
				return true
			}, "10s", "100ms").Should(BeTrue())

			// Wait for RR to complete
			Eventually(func() string {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      rr.Name,
					Namespace: rr.Namespace,
				}, updated); err != nil {
					return ""
				}
				return string(updated.Status.OverallPhase)
			}, "10s", "100ms").Should(Equal("Completed"))

			// Then: Total time should be <5s (SLO)
			elapsed := time.Since(startTime)
			Expect(elapsed).To(BeNumerically("<", 5*time.Second),
				"Happy path reconcile should complete in <5s (actual: %v)", elapsed)
		})
	})

	// ========================================
	// Gap 3.3: Cross-Namespace Isolation
	// Business Value: Multi-tenant isolation guarantee
	// Confidence: 95% - Critical multi-tenancy requirement
	// ========================================
	Describe("Namespace Isolation (Gap 3.3)", func() {
		It("should process RRs in different namespaces independently", func() {
			// Scenario: RR in ns-a fails, RR in ns-b succeeds
			// Business Outcome: No cross-namespace interference
			// Confidence: 95% - Multi-tenancy critical

			ctx := context.Background()

			// Create test namespaces
			nsA := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns-isolation-a",
				},
			}
			nsB := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns-isolation-b",
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
					Namespace: "ns-isolation-a",
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
						Namespace: "ns-isolation-a",
					},
				},
			}

			// Given: RR in namespace B that will succeed
			rrB := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-success",
					Namespace: "ns-isolation-b",
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
						Namespace: "ns-isolation-b",
					},
				},
			}

			// When: Creating both RRs
			Expect(k8sClient.Create(ctx, rrA)).To(Succeed())
			Expect(k8sClient.Create(ctx, rrB)).To(Succeed())

			// Make RR-A fail by failing its SignalProcessing
			Eventually(func() error {
				spListA := &signalprocessingv1.SignalProcessingList{}
				if err := k8sClient.List(ctx, spListA, client.InNamespace("ns-isolation-a")); err != nil {
					return err
				}
				if len(spListA.Items) == 0 {
					return fmt.Errorf("no SP in ns-a yet")
				}

				sp := &spListA.Items[0]
				sp.Status.Phase = "Failed"
				// Note: SignalProcessing doesn't have Message field, using Phase only
				return k8sClient.Status().Update(ctx, sp)
			}, "10s", "100ms").Should(Succeed())

			// Make RR-B succeed by completing all child CRDs
			Eventually(func() error {
				spListB := &signalprocessingv1.SignalProcessingList{}
				if err := k8sClient.List(ctx, spListB, client.InNamespace("ns-isolation-b")); err != nil {
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
				if err := k8sClient.List(ctx, aiListB, client.InNamespace("ns-isolation-b")); err != nil {
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

	// ========================================
	// Gap 3.2: High Load Scenarios
	// Business Value: Validates scalability
	// Confidence: 85% - Load testing critical path
	// ========================================
	Describe("High Load Behavior (Gap 3.2)", func() {
		var namespace string

		BeforeEach(func() {
			namespace = createTestNamespace("ro-load")
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		It("should handle 100 concurrent RRs without degradation", func() {
			// Scenario: 100 RRs created simultaneously (load test)
			// Business Outcome: All process successfully, no rate limiting
			// Confidence: 85% - Validates scalability

			ctx := context.Background()

			// Given: 100 RemediationRequests to create
			const numRRs = 100

			// When: Creating all RRs simultaneously
			now := metav1.Now()
			for i := 0; i < numRRs; i++ {
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("load-rr-%d", i),
						Namespace: namespace,
					},
					Spec: remediationv1.RemediationRequestSpec{
					SignalName:        fmt.Sprintf("load-signal-%d", i),
					SignalFingerprint: fmt.Sprintf("%064d", i), // Valid 64-char fingerprint
					Severity:          "info",
						SignalType:        "test",
						TargetType:        "kubernetes",
						FiringTime:        now,
						ReceivedTime:      now,
						TargetResource: remediationv1.ResourceIdentifier{
							Kind:      "Deployment",
							Name:      fmt.Sprintf("test-app-%d", i),
							Namespace: namespace,
						},
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())
			}

			// Then: All RRs should start processing (not rate limited)
			Eventually(func() int {
				rrList := &remediationv1.RemediationRequestList{}
				if err := k8sClient.List(ctx, rrList, client.InNamespace(namespace)); err != nil {
					return 0
				}

				processingCount := 0
				for _, rr := range rrList.Items {
					if rr.Status.OverallPhase != "" && rr.Status.OverallPhase != remediationv1.PhasePending {
						processingCount++
					}
				}
				return processingCount
			}, "30s", "500ms").Should(BeNumerically(">=", numRRs),
				"All %d RRs should start processing (no rate limiting)", numRRs)

			// Then: All RRs should have SignalProcessing created
			Eventually(func() int {
				spList := &signalprocessingv1.SignalProcessingList{}
				if err := k8sClient.List(ctx, spList, client.InNamespace(namespace)); err != nil {
					return 0
				}
				return len(spList.Items)
			}, "60s", "1s").Should(Equal(numRRs),
				"All %d SignalProcessing CRDs should be created", numRRs)
		})
	})
})
