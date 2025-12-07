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

	Context("BR-WE-012: Exponential Backoff Skip Reasons", func() {
		It("should report ExhaustedRetries or PreviousExecutionFailed in skip details", func() {
			// This is a fast test that just verifies the skip reason codes
			// are correctly persisted when set by the controller.
			// Actual backoff timing tests are in integration tests.

			Skip("Exponential backoff E2E requires multiple failure cycles - covered by unit/integration tests")
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

