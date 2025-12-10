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

package workflowexecution

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// WorkflowExecution Lifecycle E2E Tests
//
// These tests validate complete end-to-end workflow execution with real:
// - Kubernetes API
// - Tekton Pipelines
// - WorkflowExecution Controller
//
// Per TESTING_GUIDELINES.md: E2E tests validate business value delivery

var _ = Describe("WorkflowExecution Lifecycle E2E", func() {
	Context("BR-WE-001: Remediation Completes Within SLA", func() {
		It("should execute workflow to completion", func() {
			// Unique target for this test
			testName := fmt.Sprintf("e2e-lifecycle-%d", time.Now().UnixNano())
			targetResource := "default/deployment/test-app"
			wfe := createTestWFE(testName, targetResource)

			// Cleanup
			defer func() {
				_ = deleteWFE(wfe)
			}()

			// Create WorkflowExecution
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Should transition to Running
			Eventually(func() string {
				updated, _ := getWFE(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 30*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

			GinkgoWriter.Println("✅ WFE transitioned to Running")

			// Should complete (success or failure - depends on pipeline)
			Eventually(func() bool {
				updated, _ := getWFE(wfe.Name, wfe.Namespace)
				if updated != nil {
					phase := updated.Status.Phase
					return phase == workflowexecutionv1alpha1.PhaseCompleted ||
						phase == workflowexecutionv1alpha1.PhaseFailed
				}
				return false
			}, 120*time.Second).Should(BeTrue(), "WFE should complete within SLA")

			// Verify completion
			completed, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(completed.Status.CompletionTime).ToNot(BeNil())

			GinkgoWriter.Printf("✅ WFE completed with phase: %s\n", completed.Status.Phase)
		})
	})

	Context("BR-WE-009: Parallel Execution Prevention", func() {
		It("should skip second WFE when first is Running", func() {
			// Unique target for this test
			targetResource := fmt.Sprintf("default/deployment/parallel-test-%d", time.Now().UnixNano())

			// Create first WFE
			wfe1Name := fmt.Sprintf("e2e-parallel1-%d", time.Now().UnixNano())
			wfe1 := createTestWFE(wfe1Name, targetResource)

			defer func() {
				_ = deleteWFE(wfe1)
			}()

			Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())

			// Wait for first WFE to be Running
			Eventually(func() string {
				updated, _ := getWFE(wfe1.Name, wfe1.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 30*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

			GinkgoWriter.Println("✅ First WFE is Running")

			// Create second WFE targeting same resource
			wfe2Name := fmt.Sprintf("e2e-parallel2-%d", time.Now().UnixNano())
			wfe2 := createTestWFE(wfe2Name, targetResource)

			defer func() {
				_ = deleteWFE(wfe2)
			}()

			Expect(k8sClient.Create(ctx, wfe2)).To(Succeed())

			// Second WFE should be Skipped with ResourceBusy
			Eventually(func() string {
				updated, _ := getWFE(wfe2.Name, wfe2.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 10*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseSkipped))

			// Verify skip reason
			wfe2Updated, err := getWFE(wfe2.Name, wfe2.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(wfe2Updated.Status.SkipDetails).ToNot(BeNil())
			Expect(wfe2Updated.Status.SkipDetails.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonResourceBusy))

			GinkgoWriter.Println("✅ Second WFE skipped with ResourceBusy (BR-WE-009)")
		})
	})

	Context("BR-WE-010: Cooldown Enforcement", func() {
		It("should skip WFE within cooldown period", func() {
			// This test depends on cooldown being set to 1 minute in E2E config
			// Create first WFE and let it complete
			targetResource := fmt.Sprintf("default/deployment/cooldown-test-%d", time.Now().UnixNano())

			wfe1Name := fmt.Sprintf("e2e-cooldown1-%d", time.Now().UnixNano())
			wfe1 := createTestWFE(wfe1Name, targetResource)

			defer func() {
				_ = deleteWFE(wfe1)
			}()

			Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())

			// Wait for completion
			Eventually(func() bool {
				updated, _ := getWFE(wfe1.Name, wfe1.Namespace)
				if updated != nil {
					phase := updated.Status.Phase
					return phase == workflowexecutionv1alpha1.PhaseCompleted ||
						phase == workflowexecutionv1alpha1.PhaseFailed
				}
				return false
			}, 120*time.Second).Should(BeTrue())

			GinkgoWriter.Println("✅ First WFE completed")

			// Create second WFE immediately (within cooldown)
			wfe2Name := fmt.Sprintf("e2e-cooldown2-%d", time.Now().UnixNano())
			wfe2 := createTestWFE(wfe2Name, targetResource)

			defer func() {
				_ = deleteWFE(wfe2)
			}()

			Expect(k8sClient.Create(ctx, wfe2)).To(Succeed())

			// Second WFE should be Skipped with RecentlyRemediated
			Eventually(func() string {
				updated, _ := getWFE(wfe2.Name, wfe2.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 10*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseSkipped))

			// Verify skip reason
			wfe2Updated, err := getWFE(wfe2.Name, wfe2.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(wfe2Updated.Status.SkipDetails).ToNot(BeNil())
			Expect(wfe2Updated.Status.SkipDetails.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonRecentlyRemediated))

			GinkgoWriter.Println("✅ Second WFE skipped with RecentlyRemediated (BR-WE-010)")
		})
	})

	Context("BR-WE-012: PreviousExecutionFailed Safety Blocking", func() {
		// Business Outcome: Operators are protected from cascading failures when
		// a workflow execution fails during execution (wasExecutionFailure: true).
		// The system must block subsequent retries until manual intervention.
		//
		// Per TESTING_GUIDELINES.md: Tests must validate business outcomes

		It("should block retry with PreviousExecutionFailed after execution failure", func() {
			// Business Value: Prevents non-idempotent operations from causing cascading damage
			// Example: "increase-replicas" fails after step 1 (replicas +1). Retry would add +1 again.

			// Create first WFE that will fail during execution
			targetResource := fmt.Sprintf("default/deployment/backoff-test-%d", time.Now().UnixNano())
			wfe1Name := fmt.Sprintf("e2e-backoff1-%d", time.Now().UnixNano())

			wfe1 := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      wfe1Name,
					Namespace: "default",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-" + wfe1Name,
						Namespace:  "default",
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "test-intentional-failure",
						Version:        "v1.0.0",
						ContainerImage: "quay.io/kubernaut/workflows/test-intentional-failure:v1.0.0",
					},
					TargetResource: targetResource,
					Parameters: map[string]string{
						"FAILURE_REASON": "E2E backoff test - simulated execution failure",
					},
				},
			}

			defer func() {
				_ = deleteWFE(wfe1)
			}()

			Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())

			// Wait for first WFE to fail (simulates wasExecutionFailure: true)
			Eventually(func() string {
				updated, _ := getWFE(wfe1.Name, wfe1.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 120*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseFailed))

			GinkgoWriter.Println("✅ First WFE failed (execution failure simulated)")

			// Verify first WFE has wasExecutionFailure: true
			failed, _ := getWFE(wfe1.Name, wfe1.Namespace)
			if failed.Status.FailureDetails != nil {
				GinkgoWriter.Printf("   FailureDetails.WasExecutionFailure: %v\n",
					failed.Status.FailureDetails.WasExecutionFailure)
			}

			// Create second WFE targeting same resource + workflow
			// Business Expectation: Should be blocked with PreviousExecutionFailed
			wfe2Name := fmt.Sprintf("e2e-backoff2-%d", time.Now().UnixNano())
			wfe2 := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      wfe2Name,
					Namespace: "default",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-" + wfe2Name,
						Namespace:  "default",
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "test-intentional-failure",
						Version:        "v1.0.0",
						ContainerImage: "quay.io/kubernaut/workflows/test-intentional-failure:v1.0.0",
					},
					TargetResource: targetResource, // Same target as first WFE
					Parameters: map[string]string{
						"FAILURE_REASON": "Should be blocked by PreviousExecutionFailed",
					},
				},
			}

			defer func() {
				_ = deleteWFE(wfe2)
			}()

			Expect(k8sClient.Create(ctx, wfe2)).To(Succeed())

			// Business Outcome: Second WFE should be Skipped with PreviousExecutionFailed
			// This protects operators from cascading non-idempotent failures
			Eventually(func() string {
				updated, _ := getWFE(wfe2.Name, wfe2.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 30*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseSkipped),
				"Business Outcome: Retry MUST be blocked when previous execution failed")

			// Verify skip reason
			wfe2Updated, err := getWFE(wfe2.Name, wfe2.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(wfe2Updated.Status.SkipDetails).ToNot(BeNil(),
				"Business Outcome: SkipDetails must explain why retry was blocked")
			Expect(wfe2Updated.Status.SkipDetails.Reason).To(Equal(workflowexecutionv1alpha1.SkipReasonPreviousExecutionFailed),
				"Business Outcome: Reason must be PreviousExecutionFailed to indicate manual review needed")

			GinkgoWriter.Println("✅ BR-WE-012: Second WFE correctly blocked with PreviousExecutionFailed")
			GinkgoWriter.Printf("   SkipReason: %s\n", wfe2Updated.Status.SkipDetails.Reason)
			GinkgoWriter.Printf("   Message: %s\n", wfe2Updated.Status.SkipDetails.Message)
		})
	})

	Context("BR-WE-004: Failure Details Actionable", func() {
		It("should populate failure details when workflow fails", func() {
			// Create a WFE that uses the intentionally failing pipeline
			testName := fmt.Sprintf("e2e-failure-%d", time.Now().UnixNano())
			targetResource := fmt.Sprintf("default/deployment/fail-test-%d", time.Now().UnixNano())

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: "default",
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-" + testName,
						Namespace:  "default",
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "test-intentional-failure",
						Version:        "v1.0.0",
						ContainerImage: "quay.io/kubernaut/workflows/test-intentional-failure:v1.0.0",
					},
					TargetResource: targetResource,
					Parameters: map[string]string{
						"FAILURE_REASON": "E2E test simulated failure",
					},
				},
			}

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Should transition to Failed
			Eventually(func() string {
				updated, _ := getWFE(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 120*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseFailed))

			// Verify failure details are populated
			failed, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(failed.Status.FailureDetails).ToNot(BeNil(), "FailureDetails should be populated")
			Expect(failed.Status.FailureDetails.Message).ToNot(BeEmpty(), "Failure message should be set")

			GinkgoWriter.Printf("✅ Failure details populated: %s\n", failed.Status.FailureDetails.Message)
		})
	})
})
