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
	weconditions "github.com/jordigilh/kubernaut/pkg/workflowexecution"

	"github.com/google/uuid"
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
			testName := fmt.Sprintf("e2e-lifecycle-%s", uuid.New().String()[:8])
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
			updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
			if updated != nil {
				return updated.Status.Phase
			}
			return ""
		}, 30*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

			GinkgoWriter.Println("✅ WFE transitioned to Running")

		// Should complete (success or failure - depends on pipeline)
		Eventually(func() bool {
			updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
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

			// BR-WE-006: Verify Kubernetes Conditions are set during lifecycle
			// Per WE_BR_WE_006_TESTING_TRIAGE.md: Validate conditions in existing E2E tests
			By("Verifying Kubernetes Conditions are set (BR-WE-006)")

		// ✅ REQUIRED: Use Eventually() pattern (NO time.Sleep())
		Eventually(func() bool {
			updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
			if updated == nil {
				GinkgoWriter.Printf("🔍 DEBUG: WFE not found yet\n")
				return false
			}

		// Verify final lifecycle conditions are present
		// We only care about final state (Complete), not transient states (Running)
		hasPipelineCreated := weconditions.IsConditionTrue(updated, weconditions.ConditionExecutionCreated)
		// TektonPipelineComplete can be True (success) or False (failure) - just verify it's set
		// Test accepts both success and failure (line 73-74), so only check existence
		hasPipelineComplete := weconditions.GetCondition(updated, weconditions.ConditionTektonPipelineComplete) != nil
		// AuditRecorded may be True or False depending on audit store availability
		hasAuditRecorded := weconditions.GetCondition(updated, weconditions.ConditionAuditRecorded) != nil

		// DEBUG: Show current status of final conditions
		GinkgoWriter.Printf("🔍 DEBUG: Condition status - PipelineCreated=%v, PipelineComplete=%v, AuditRecorded=%v\n",
			hasPipelineCreated, hasPipelineComplete, hasAuditRecorded)
		if !hasAuditRecorded || !hasPipelineComplete {
			// Show all current conditions to understand what's missing
			GinkgoWriter.Printf("🔍 DEBUG: Current conditions:\n")
			for _, cond := range updated.Status.Conditions {
				GinkgoWriter.Printf("  - %s: %s (reason: %s, message: %s)\n", cond.Type, cond.Status, cond.Reason, cond.Message)
			}
		}

		return hasPipelineCreated && hasPipelineComplete && hasAuditRecorded
	}, 30*time.Second, 5*time.Second).Should(BeTrue(),
		"All final lifecycle conditions (ExecutionCreated, TektonPipelineComplete, AuditRecorded) should be set")

			// Verify condition details
			final, _ := getWFE(wfe.Name, wfe.Namespace)
			GinkgoWriter.Println("✅ Kubernetes Conditions verified:")
			for _, cond := range final.Status.Conditions {
				GinkgoWriter.Printf("  - %s: %s (reason: %s)\n", cond.Type, cond.Status, cond.Reason)
			}
		})
	})

	// V1.0 NOTE: Routing tests removed (BR-WE-009, BR-WE-010, BR-WE-012) - routing moved to RO (DD-RO-002)
	// RO handles these decisions BEFORE creating WFE, so WE never sees these scenarios

	Context("BR-WE-004: Failure Details Actionable", func() {
		It("should populate failure details when workflow fails", func() {
			// Create a WFE that uses the intentionally failing pipeline
			testName := fmt.Sprintf("e2e-failure-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/fail-test-%s", uuid.New().String()[:8])

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: "default",
				},
			Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
				ExecutionEngine: "tekton", // BR-WE-014: Required field
				RemediationRequestRef: corev1.ObjectReference{
					APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
					Kind:       "RemediationRequest",
					Name:       "test-rr-" + testName,
					Namespace:  "default",
				},
			WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
				WorkflowID: "test-intentional-failure",
				Version:    "v1.0.0",
				// Use multi-arch bundle from quay.io/kubernaut-cicd (amd64 + arm64)
				ContainerImage: "quay.io/kubernaut-cicd/test-workflows/failing:v1.0.0",
			},
				TargetResource: targetResource,
				Parameters: map[string]string{
					// Per test/fixtures/tekton/failing-pipeline.yaml
					"FAILURE_MODE":    "exit",
					"FAILURE_MESSAGE": "E2E test simulated failure",
				},
			},
			}

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		// Should transition to Failed
		Eventually(func() string {
			updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
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

			// BR-WE-004 EXTENDED: Verify TektonPipelineComplete condition reflects failure
			// This validation moved from integration tests (EnvTest limitation) to E2E
			completeCond := weconditions.GetCondition(failed, weconditions.ConditionTektonPipelineComplete)
			Expect(completeCond).ToNot(BeNil(), "TektonPipelineComplete condition should exist")
			Expect(completeCond.Status).To(Equal(metav1.ConditionFalse), "TektonPipelineComplete should be False on failure")
			Expect(completeCond.Reason).To(Equal(weconditions.ReasonTaskFailed), "Condition should reflect task-level failure")

			GinkgoWriter.Printf("✅ Failure details populated: %s\n", failed.Status.FailureDetails.Message)
			GinkgoWriter.Printf("✅ TektonPipelineComplete condition: Status=%s, Reason=%s\n",
				completeCond.Status, completeCond.Reason)
		})

		// ========================================
		// BR-WE-010: Cooldown Period Edge Cases
		// ========================================
		Context("BR-WE-010: Cooldown Without CompletionTime", func() {
			It("should skip cooldown check when CompletionTime is not set", func() {
				// Business Outcome: Controller handles edge cases gracefully without crashing
				// This validates that missing CompletionTime doesn't cause panics or deadlocks

				testName := fmt.Sprintf("e2e-cooldown-no-completion-%s", uuid.New().String()[:8])
				targetResource := fmt.Sprintf("default/deployment/cooldown-test-%s", uuid.New().String()[:8])
				wfe := createTestWFE(testName, targetResource)

				defer func() {
					_ = deleteWFE(wfe)
				}()

				Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Wait for workflow to start
			Eventually(func() bool {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase == workflowexecutionv1alpha1.PhaseRunning
				}
				return false
			}, 120*time.Second, 2*time.Second).Should(BeTrue(), "Workflow should start")

				GinkgoWriter.Println("✅ Workflow started, simulating failure without CompletionTime")

				// Manually mark as Failed WITHOUT setting CompletionTime
				// This simulates an edge case where controller logic might miss setting CompletionTime
				// Use Eventually to handle concurrent controller updates (race condition fix)
				Eventually(func() error {
					wfeStatus, err := getWFE(wfe.Name, wfe.Namespace)
					if err != nil {
						return err
					}

					wfeStatus.Status.Phase = workflowexecutionv1alpha1.PhaseFailed
					wfeStatus.Status.FailureDetails = &workflowexecutionv1alpha1.FailureDetails{
						Reason:   workflowexecutionv1alpha1.FailureReasonUnknown, // Use constant instead of hardcoded string
						Message:  "Simulated failure without CompletionTime",
						FailedAt: metav1.Now(), // Required field per CRD schema
					}
					// Intentionally NOT setting CompletionTime (testing cooldown edge case)
					return k8sClient.Status().Update(ctx, wfeStatus)
				}, 30*time.Second, 1*time.Second).Should(Succeed(), "Should update WFE status despite concurrent controller updates")

				GinkgoWriter.Println("✅ Marked as Failed without CompletionTime")

			// Business Behavior: Controller should handle this gracefully
			// - No panic or crash
			// - Cooldown check is skipped (logged but not enforced)
			// - Workflow remains in Failed state
			Eventually(func() bool {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					// Verify phase is still Failed (controller didn't crash)
					return updated.Status.Phase == workflowexecutionv1alpha1.PhaseFailed
				}
				return false
			}, 60*time.Second, 2*time.Second).Should(BeTrue(),
				"Controller should handle missing CompletionTime without crashing")

				// Verify controller logs show cooldown was skipped (can't check logs in E2E, but verify no crash)
				finalWFE, err := getWFE(wfe.Name, wfe.Namespace)
				Expect(err).ToNot(HaveOccurred())
				Expect(finalWFE.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
				Expect(finalWFE.Status.FailureDetails).ToNot(BeNil())
				Expect(finalWFE.Status.FailureDetails.Reason).To(Equal(workflowexecutionv1alpha1.FailureReasonUnknown))

				GinkgoWriter.Println("✅ BR-WE-010: Controller gracefully handled missing CompletionTime")
			})
		})
	})
})
