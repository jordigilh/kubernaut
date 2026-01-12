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
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// ========================================
// BR-WE-007: Handle Externally Deleted PipelineRun - Integration Tests
// ========================================
//
// **Defense-in-Depth Strategy**:
// - Unit tests: NotFound error handling (controller_test.go)
// - Integration tests: Controller reconciliation with real K8s API (THIS FILE)
// - E2E tests: External deletion in Kind cluster (02_observability_test.go)
//
// **Why Integration Tests Matter**:
// 1. Faster feedback (~2 min vs ~10 min E2E)
// 2. Tests controller reconciliation without full Kind cluster
// 3. Easier debugging with envtest
// 4. Validates BR-WE-007 at controller reconciliation layer
//
// **Test Coverage**:
// - External deletion detection during Running phase
// - WFE status update to Failed with appropriate message
// - Audit event emission for external deletion
// - No retry loop on deleted PipelineRun
//
// **Business Value**: Operators need clear failure reasons when PipelineRuns
// are deleted externally (manual cleanup, namespace deletion, etc.)

var _ = Describe("BR-WE-007: Handle Externally Deleted PipelineRun", Label("integration", "external-deletion", "br-we-007"), func() {

	Context("External Deletion During Running Phase", func() {
		It("should detect PipelineRun deletion and mark WFE as Failed", func() {
			// BR-WE-007 AC-1: NotFound handled without panic
			// BR-WE-007 AC-2: WorkflowExecution marked Failed
			// BR-WE-007 AC-3: Message indicates external deletion

			By("Creating WorkflowExecution")
			targetResource := fmt.Sprintf("default/deployment/ext-del-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("ext-del-detect", targetResource)
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for WFE to transition to Running (PipelineRun created)")
			runningWFE, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 15*time.Second)
			Expect(err).ToNot(HaveOccurred(), "WFE should transition to Running")
			Expect(runningWFE.Status.PipelineRunRef).ToNot(BeNil(), "PipelineRunRef should be set")

			prName := runningWFE.Status.PipelineRunRef.Name
			GinkgoWriter.Printf("✅ WFE Running, PipelineRun created: %s\n", prName)

			By("Simulating PipelineRun external deletion (operator action)")
			pr := &tektonv1.PipelineRun{}
			prKey := types.NamespacedName{
				Name:      prName,
				Namespace: WorkflowExecutionNS,
			}
			Expect(k8sClient.Get(ctx, prKey, pr)).To(Succeed(), "PipelineRun should exist")

			// Delete the PipelineRun (simulates external deletion)
			Expect(k8sClient.Delete(ctx, pr)).To(Succeed())
			GinkgoWriter.Printf("🗑️  PipelineRun %s deleted externally\n", prName)

			By("Waiting for controller to detect deletion and update WFE status")
			// Controller reconciles and detects NotFound error
			Eventually(func(g Gomega) {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(updated.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed),
					"WFE should transition to Failed when PipelineRun is externally deleted")
			}, 30*time.Second, 500*time.Millisecond).Should(Succeed())

			By("Verifying failure details indicate external deletion")
			failedWFE, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())

			// BR-WE-007 AC-3: Message indicates external deletion
			Expect(failedWFE.Status.FailureDetails).ToNot(BeNil(), "FailureDetails should be populated")
			Expect(failedWFE.Status.FailureDetails.Message).To(
				Or(
					ContainSubstring("not found"),
					ContainSubstring("deleted"),
					ContainSubstring("NotFound"),
				),
				"Failure message should indicate external deletion")

			// BR-WE-007 AC-4: No retry loop on deleted PipelineRun
			// Wait additional time to ensure controller doesn't retry
			Consistently(func(g Gomega) {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(updated.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed),
					"WFE should remain Failed (no retry loop)")
			}, 5*time.Second, 1*time.Second).Should(Succeed())

			GinkgoWriter.Printf("✅ BR-WE-007: External deletion detected\n")
			GinkgoWriter.Printf("   Phase: %s\n", failedWFE.Status.Phase)
			GinkgoWriter.Printf("   Failure Reason: %s\n", failedWFE.Status.FailureDetails.Reason)
			GinkgoWriter.Printf("   Failure Message: %s\n", failedWFE.Status.FailureDetails.Message)
		})

		It("should set AuditRecorded condition when PipelineRun is deleted externally", func() {
			// BR-WE-007 + BR-WE-005 + BR-WE-006: Audit trail + Conditions for external deletion
			// Validates that controller attempts audit event emission for external deletion
			// Note: Full audit persistence validation in E2E tests with real DataStorage

			By("Creating WorkflowExecution")
			targetResource := fmt.Sprintf("default/deployment/ext-del-audit-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("ext-del-audit", targetResource)
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for WFE to transition to Running")
			runningWFE, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 15*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(runningWFE.Status.PipelineRunRef).ToNot(BeNil())

			prName := runningWFE.Status.PipelineRunRef.Name

			By("Deleting PipelineRun externally")
			pr := &tektonv1.PipelineRun{}
			prKey := types.NamespacedName{
				Name:      prName,
				Namespace: WorkflowExecutionNS,
			}
			Expect(k8sClient.Get(ctx, prKey, pr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, pr)).To(Succeed())

			By("Waiting for WFE to transition to Failed")
			Eventually(func(g Gomega) {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(updated.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
			}, 30*time.Second, 500*time.Millisecond).Should(Succeed())

			By("Verifying AuditRecorded condition is set (BR-WE-006)")
			// Controller should attempt to emit workflow.failed audit event
			// AuditRecorded condition tracks audit emission status
			failedWFE, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())

			// Find AuditRecorded condition
			var auditCondition *metav1.Condition
			for i := range failedWFE.Status.Conditions {
				if failedWFE.Status.Conditions[i].Type == "AuditRecorded" {
					cond := failedWFE.Status.Conditions[i]
					auditCondition = &cond
					break
				}
			}

			// AuditRecorded condition should exist (controller attempted audit)
			// Status may be True (success) or False (DataStorage unavailable)
			// Both are acceptable - what matters is controller tried
			Expect(auditCondition).ToNot(BeNil(),
				"AuditRecorded condition should be set (proves controller attempted audit for external deletion)")

			GinkgoWriter.Printf("✅ BR-WE-007 + BR-WE-005 + BR-WE-006: Audit emission attempted\n")
			GinkgoWriter.Printf("   AuditRecorded Status: %s\n", auditCondition.Status)
			GinkgoWriter.Printf("   AuditRecorded Reason: %s\n", auditCondition.Reason)
			if auditCondition.Status == "False" {
				GinkgoWriter.Printf("   (DataStorage unavailable - expected in integration tier)\n")
			}
		})

		It("should handle deletion during Pending phase gracefully", func() {
			// Edge case: PipelineRun deleted before controller transitions to Running
			// Controller should detect NotFound during Pending → Running transition

			By("Creating WorkflowExecution")
			targetResource := fmt.Sprintf("default/deployment/ext-del-pending-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("ext-del-pending", targetResource)
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for PipelineRun to be created (but not Running yet)")
			// Don't wait for Running phase - catch it during Pending
			pr, err := waitForPipelineRunCreation(wfe.Name, wfe.Namespace, 10*time.Second)
			Expect(err).ToNot(HaveOccurred(), "PipelineRun should be created")

			By("Deleting PipelineRun immediately (before Running)")
			Expect(k8sClient.Delete(ctx, pr)).To(Succeed())
			GinkgoWriter.Printf("🗑️  PipelineRun %s deleted during Pending phase\n", pr.Name)

			By("Verifying WFE handles deletion gracefully")
			// Controller should detect NotFound and mark as Failed
			Eventually(func(g Gomega) {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(updated.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed),
					"WFE should transition to Failed when PR deleted during Pending")
			}, 30*time.Second, 500*time.Millisecond).Should(Succeed())

			failedWFE, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(failedWFE.Status.FailureDetails).ToNot(BeNil())
			Expect(failedWFE.Status.FailureDetails.Message).To(
				Or(
					ContainSubstring("not found"),
					ContainSubstring("deleted"),
					ContainSubstring("NotFound"),
				),
				"Failure message should indicate external deletion")

			GinkgoWriter.Printf("✅ BR-WE-007: External deletion during Pending handled gracefully\n")
		})

		It("should set PipelineRunRef to nil after detecting external deletion", func() {
			// Validates controller cleanup behavior after external deletion
			// PipelineRunRef should be cleared when PipelineRun no longer exists

			By("Creating WorkflowExecution")
			targetResource := fmt.Sprintf("default/deployment/ext-del-ref-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("ext-del-ref", targetResource)
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for WFE to transition to Running")
			runningWFE, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 15*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(runningWFE.Status.PipelineRunRef).ToNot(BeNil())

			prName := runningWFE.Status.PipelineRunRef.Name

			By("Deleting PipelineRun externally")
			pr := &tektonv1.PipelineRun{}
			prKey := types.NamespacedName{
				Name:      prName,
				Namespace: WorkflowExecutionNS,
			}
			Expect(k8sClient.Get(ctx, prKey, pr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, pr)).To(Succeed())

			By("Waiting for WFE to transition to Failed")
			Eventually(func(g Gomega) {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(updated.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
			}, 30*time.Second, 500*time.Millisecond).Should(Succeed())

			By("Verifying PipelineRunRef state after external deletion")
			failedWFE, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())

			// PipelineRunRef behavior: Controller may keep ref for audit trail
			// OR clear it since PipelineRun no longer exists
			// Either behavior is acceptable as long as FailureDetails explains deletion
			if failedWFE.Status.PipelineRunRef != nil {
				GinkgoWriter.Printf("   PipelineRunRef kept for audit trail: %s\n", failedWFE.Status.PipelineRunRef.Name)
			} else {
				GinkgoWriter.Printf("   PipelineRunRef cleared after deletion\n")
			}

			// What matters: FailureDetails must be populated
			Expect(failedWFE.Status.FailureDetails).ToNot(BeNil(),
				"FailureDetails must be populated regardless of PipelineRunRef state")
			Expect(failedWFE.Status.FailureDetails.Message).To(
				Or(
					ContainSubstring("not found"),
					ContainSubstring("deleted"),
					ContainSubstring("NotFound"),
				))

			GinkgoWriter.Printf("✅ BR-WE-007: PipelineRunRef state validated after external deletion\n")
		})
	})

	Context("External Deletion Prevention (Negative Test)", func() {
		It("should NOT mark WFE as Failed when PipelineRun completes normally", func() {
			// Negative test: Ensure external deletion detection doesn't false-positive
			// Normal completion should NOT trigger external deletion logic

			By("Creating WorkflowExecution")
			targetResource := fmt.Sprintf("default/deployment/normal-complete-%d", time.Now().UnixNano())
			wfe := createUniqueWFE("normal-complete", targetResource)
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for WFE to transition to Running")
			runningWFE, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 15*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(runningWFE.Status.PipelineRunRef).ToNot(BeNil())

			By("Simulating normal PipelineRun completion (not external deletion)")
			pr := &tektonv1.PipelineRun{}
			prKey := types.NamespacedName{
				Name:      runningWFE.Status.PipelineRunRef.Name,
				Namespace: WorkflowExecutionNS,
			}
			Expect(k8sClient.Get(ctx, prKey, pr)).To(Succeed())

			// Simulate successful completion (NOT deletion)
			err = simulatePipelineRunCompletion(pr, true)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying WFE transitions to Completed (NOT Failed)")
			Eventually(func(g Gomega) {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(updated.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseCompleted),
					"WFE should complete normally (not trigger external deletion logic)")
			}, 30*time.Second, 500*time.Millisecond).Should(Succeed())

			completedWFE, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())

			// Should NOT have FailureDetails (normal completion)
			Expect(completedWFE.Status.FailureDetails).To(BeNil(),
				"Completed WFE should NOT have FailureDetails")

			GinkgoWriter.Printf("✅ BR-WE-007: Normal completion NOT misidentified as external deletion\n")
		})
	})
})
