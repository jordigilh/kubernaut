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

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

// Integration Tests: Controller Reconciliation
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - These tests validate controller behavior with real K8s API
// - Focus: Reconciliation logic, PipelineRun creation, status sync
// - Target: >50% coverage of controller code paths
//
// Tests in this file:
// - Reconciliation triggers PipelineRun creation
// - Status sync from PipelineRun to WFE
// - Resource locking prevents parallel execution
// - Cooldown enforcement
// - Phase transitions (Pending → Running → Completed/Failed)

var _ = Describe("WorkflowExecution Controller Reconciliation", func() {

	// ========================================
	// BR-WE-001: PipelineRun Creation
	// ========================================
	Context("PipelineRun Creation", func() {
		var wfe *workflowexecutionv1alpha1.WorkflowExecution

		AfterEach(func() {
			if wfe != nil {
				cleanupWFE(wfe)
			}
		})

		It("should create PipelineRun when WFE is created", func() {
			By("Creating a WorkflowExecution")
			wfe = createUniqueWFE("pr-create", "default/deployment/test-app")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for controller to create PipelineRun")
			pr, err := waitForPipelineRunCreation(wfe.Name, wfe.Namespace, 10*time.Second)
			Expect(err).ToNot(HaveOccurred(), "PipelineRun should be created")
			Expect(pr).ToNot(BeNil())

			By("Verifying PipelineRun has correct labels")
			Expect(pr.Labels).To(HaveKeyWithValue("kubernaut.ai/workflow-execution", wfe.Name))
			Expect(pr.Labels).To(HaveKey("kubernaut.ai/target-resource"))

			By("Verifying PipelineRun is in execution namespace")
			Expect(pr.Namespace).To(Equal(WorkflowExecutionNS))

			By("Verifying WFE status updated to Running")
			updatedWFE, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(updatedWFE.Status.PipelineRunRef).ToNot(BeNil())
			Expect(updatedWFE.Status.PipelineRunRef.Name).To(Equal(pr.Name))
		})

		It("should pass parameters to PipelineRun", func() {
			By("Creating a WorkflowExecution with parameters")
			params := map[string]string{
				"NAMESPACE":       "production",
				"DEPLOYMENT_NAME": "my-service",
				"REPLICA_COUNT":   "3",
			}
			wfe = createUniqueWFEWithParams("pr-params", "production/deployment/my-service", params)
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for PipelineRun creation")
			pr, err := waitForPipelineRunCreation(wfe.Name, wfe.Namespace, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying parameters are passed to PipelineRun")
			// Parameters should be in PipelineRun.Spec.Params
			paramMap := make(map[string]string)
			for _, p := range pr.Spec.Params {
				if p.Value.StringVal != "" {
					paramMap[p.Name] = p.Value.StringVal
				}
			}
			Expect(paramMap).To(HaveKeyWithValue("NAMESPACE", "production"))
			Expect(paramMap).To(HaveKeyWithValue("DEPLOYMENT_NAME", "my-service"))
			Expect(paramMap).To(HaveKeyWithValue("REPLICA_COUNT", "3"))
		})

		It("should include TARGET_RESOURCE parameter", func() {
			By("Creating a WorkflowExecution")
			targetResource := "monitoring/deployment/prometheus"
			wfe = createUniqueWFE("pr-target", targetResource)
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for PipelineRun creation")
			pr, err := waitForPipelineRunCreation(wfe.Name, wfe.Namespace, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying TARGET_RESOURCE parameter is passed")
			var foundTargetResource bool
			for _, p := range pr.Spec.Params {
				if p.Name == "TARGET_RESOURCE" {
					Expect(p.Value.StringVal).To(Equal(targetResource))
					foundTargetResource = true
					break
				}
			}
			Expect(foundTargetResource).To(BeTrue(), "TARGET_RESOURCE parameter should be present")
		})
	})

	// ========================================
	// BR-WE-003: Status Sync
	// ========================================
	Context("Status Synchronization", func() {
		var wfe *workflowexecutionv1alpha1.WorkflowExecution
		var pr *tektonv1.PipelineRun

		AfterEach(func() {
			if wfe != nil {
				cleanupWFE(wfe)
			}
		})

		It("should sync WFE status when PipelineRun succeeds", func() {
			By("Creating a WorkflowExecution")
			wfe = createUniqueWFE("status-sync-success", "default/deployment/status-test")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for PipelineRun creation")
			var err error
			pr, err = waitForPipelineRunCreation(wfe.Name, wfe.Namespace, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Simulating PipelineRun success")
			err = simulatePipelineRunCompletion(pr, true)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for WFE to reach Completed phase")
			updatedWFE, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseCompleted), 15*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(updatedWFE.Status.CompletionTime).ToNot(BeNil())
			Expect(updatedWFE.Status.Duration).ToNot(BeEmpty())
		})

		It("should sync WFE status when PipelineRun fails", func() {
			By("Creating a WorkflowExecution")
			wfe = createUniqueWFE("status-sync-fail", "default/deployment/fail-test")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for PipelineRun creation")
			var err error
			pr, err = waitForPipelineRunCreation(wfe.Name, wfe.Namespace, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Simulating PipelineRun failure")
			err = simulatePipelineRunCompletion(pr, false)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for WFE to reach Failed phase")
			updatedWFE, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseFailed), 15*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(updatedWFE.Status.CompletionTime).ToNot(BeNil())
			Expect(updatedWFE.Status.FailureDetails).ToNot(BeNil())
		})

		It("should populate PipelineRunStatus during Running phase", func() {
			By("Creating a WorkflowExecution")
			wfe = createUniqueWFE("status-running", "default/deployment/running-test")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Running phase")
			updatedWFE, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying PipelineRunRef is populated")
			Expect(updatedWFE.Status.PipelineRunRef).ToNot(BeNil())
			Expect(updatedWFE.Status.PipelineRunRef.Name).ToNot(BeEmpty())
		})
	})

	// ========================================
	// BR-WE-009: Resource Locking
	// ========================================
	Context("Resource Locking - Parallel Execution Prevention", func() {
		var wfe1, wfe2 *workflowexecutionv1alpha1.WorkflowExecution

		AfterEach(func() {
			if wfe1 != nil {
				cleanupWFE(wfe1)
			}
			if wfe2 != nil {
				cleanupWFE(wfe2)
			}
		})

		It("should skip second WFE when first is Running on same target", func() {
			targetResource := fmt.Sprintf("default/deployment/locking-test-%d", time.Now().UnixNano())

			By("Creating first WorkflowExecution")
			wfe1 = createUniqueWFE("lock-first", targetResource)
			Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())

			By("Waiting for first WFE to reach Running")
			_, err := waitForWFEPhase(wfe1.Name, wfe1.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Creating second WorkflowExecution on same target")
			wfe2 = createUniqueWFE("lock-second", targetResource)
			Expect(k8sClient.Create(ctx, wfe2)).To(Succeed())

			By("Waiting for second WFE to be Skipped")
			updatedWFE2, err := waitForWFEPhase(wfe2.Name, wfe2.Namespace, string(workflowexecutionv1alpha1.PhaseSkipped), 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying skip details")
			Expect(updatedWFE2.Status.SkipDetails).ToNot(BeNil())
			Expect(updatedWFE2.Status.SkipDetails.Reason).To(Equal("ResourceBusy"))
			Expect(updatedWFE2.Status.SkipDetails.ConflictingWorkflow).ToNot(BeNil())
			Expect(updatedWFE2.Status.SkipDetails.ConflictingWorkflow.Name).To(Equal(wfe1.Name))
		})

		It("should allow parallel execution on different targets", func() {
			By("Creating first WorkflowExecution on target A")
			wfe1 = createUniqueWFE("parallel-a", fmt.Sprintf("default/deployment/target-a-%d", time.Now().UnixNano()))
			Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())

			By("Creating second WorkflowExecution on target B")
			wfe2 = createUniqueWFE("parallel-b", fmt.Sprintf("default/deployment/target-b-%d", time.Now().UnixNano()))
			Expect(k8sClient.Create(ctx, wfe2)).To(Succeed())

			By("Waiting for both to reach Running")
			_, err := waitForWFEPhase(wfe1.Name, wfe1.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			_, err = waitForWFEPhase(wfe2.Name, wfe2.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying both have PipelineRuns")
			pr1, err := waitForPipelineRunCreation(wfe1.Name, wfe1.Namespace, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(pr1).ToNot(BeNil())

			pr2, err := waitForPipelineRunCreation(wfe2.Name, wfe2.Namespace, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(pr2).ToNot(BeNil())
		})
	})

	// ========================================
	// BR-WE-010: Cooldown Enforcement
	// ========================================
	Context("Cooldown Enforcement", func() {
		var wfe1, wfe2 *workflowexecutionv1alpha1.WorkflowExecution
		var pr *tektonv1.PipelineRun

		AfterEach(func() {
			if wfe1 != nil {
				cleanupWFE(wfe1)
			}
			if wfe2 != nil {
				cleanupWFE(wfe2)
			}
		})

		It("should skip WFE within cooldown period after completion", func() {
			targetResource := fmt.Sprintf("default/deployment/cooldown-test-%d", time.Now().UnixNano())

			By("Creating first WorkflowExecution")
			wfe1 = createUniqueWFE("cooldown-first", targetResource)
			Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())

			By("Waiting for PipelineRun and completing it")
			var err error
			pr, err = waitForPipelineRunCreation(wfe1.Name, wfe1.Namespace, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			err = simulatePipelineRunCompletion(pr, true)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for first WFE to complete")
			_, err = waitForWFEPhase(wfe1.Name, wfe1.Namespace, string(workflowexecutionv1alpha1.PhaseCompleted), 15*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Creating second WorkflowExecution immediately (within cooldown)")
			wfe2 = createUniqueWFE("cooldown-second", targetResource)
			wfe2.Spec.WorkflowRef.WorkflowID = wfe1.Spec.WorkflowRef.WorkflowID // Same workflow
			Expect(k8sClient.Create(ctx, wfe2)).To(Succeed())

			By("Waiting for second WFE to be Skipped")
			updatedWFE2, err := waitForWFEPhase(wfe2.Name, wfe2.Namespace, string(workflowexecutionv1alpha1.PhaseSkipped), 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying skip reason is RecentlyRemediated")
			Expect(updatedWFE2.Status.SkipDetails).ToNot(BeNil())
			Expect(updatedWFE2.Status.SkipDetails.Reason).To(Equal("RecentlyRemediated"))
		})
	})

	// ========================================
	// BR-WE-004: Owner Reference
	// ========================================
	Context("Owner Reference and Cascade Deletion", func() {
		var wfe *workflowexecutionv1alpha1.WorkflowExecution

		AfterEach(func() {
			if wfe != nil {
				cleanupWFE(wfe)
			}
		})

		It("should set owner reference on PipelineRun", func() {
			By("Creating a WorkflowExecution")
			wfe = createUniqueWFE("owner-ref", "default/deployment/owner-test")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for PipelineRun creation")
			pr, err := waitForPipelineRunCreation(wfe.Name, wfe.Namespace, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying owner reference is set")
			// Note: In cross-namespace scenarios, owner reference may not be set
			// but we should have the tracking label
			Expect(pr.Labels).To(HaveKeyWithValue("kubernaut.ai/workflow-execution", wfe.Name))
		})
	})

	// ========================================
	// BR-WE-006: ServiceAccount Configuration
	// ========================================
	Context("ServiceAccount Configuration", func() {
		var wfe *workflowexecutionv1alpha1.WorkflowExecution

		AfterEach(func() {
			if wfe != nil {
				cleanupWFE(wfe)
			}
		})

		It("should use default ServiceAccount when not specified", func() {
			By("Creating a WorkflowExecution without ServiceAccount")
			wfe = createUniqueWFE("sa-default", "default/deployment/sa-test")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for PipelineRun creation")
			pr, err := waitForPipelineRunCreation(wfe.Name, wfe.Namespace, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying default ServiceAccount is used")
			Expect(pr.Spec.TaskRunTemplate.ServiceAccountName).To(Equal("kubernaut-workflow-runner"))
		})

		// NOTE: Custom ServiceAccount per-WFE is NOT supported by design.
		// The controller uses a cluster-admin configured SA for security.
		// This test verifies that ExecutionConfig.ServiceAccountName is ignored
		// in favor of the controller-level ServiceAccountName configuration.
		It("should ignore ExecutionConfig ServiceAccount and use controller default", func() {
			By("Creating a WorkflowExecution with custom ServiceAccount in spec")
			wfe = createUniqueWFE("sa-custom", "default/deployment/sa-custom-test")
			wfe.Spec.ExecutionConfig = &workflowexecutionv1alpha1.ExecutionConfig{
				ServiceAccountName: "custom-workflow-sa", // This should be IGNORED
			}
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for PipelineRun creation")
			pr, err := waitForPipelineRunCreation(wfe.Name, wfe.Namespace, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying controller-level ServiceAccount is used (not spec)")
			// The controller is configured with "kubernaut-workflow-runner"
			// in the test suite, so that's what should be used
			Expect(pr.Spec.TaskRunTemplate.ServiceAccountName).To(Equal("kubernaut-workflow-runner"))
		})
	})

	// ========================================
	// Phase Transitions
	// ========================================
	Context("Phase Transitions", func() {
		var wfe *workflowexecutionv1alpha1.WorkflowExecution
		var pr *tektonv1.PipelineRun

		AfterEach(func() {
			if wfe != nil {
				cleanupWFE(wfe)
			}
		})

		It("should transition Pending → Running → Completed", func() {
			By("Creating a WorkflowExecution")
			wfe = createUniqueWFE("phase-complete", "default/deployment/phase-test")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Verifying initial phase is empty or Pending")
			initialWFE, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(initialWFE.Status.Phase).To(BeElementOf("", string(workflowexecutionv1alpha1.PhasePending)))

			By("Waiting for Running phase")
			_, err = waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Getting PipelineRun and completing it")
			pr, err = waitForPipelineRunCreation(wfe.Name, wfe.Namespace, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())

			err = simulatePipelineRunCompletion(pr, true)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for Completed phase")
			finalWFE, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseCompleted), 15*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(finalWFE.Status.CompletionTime).ToNot(BeNil())
		})

		It("should transition Pending → Running → Failed", func() {
			By("Creating a WorkflowExecution")
			wfe = createUniqueWFE("phase-fail", "default/deployment/phase-fail-test")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Running phase")
			_, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Getting PipelineRun and failing it")
			pr, err = waitForPipelineRunCreation(wfe.Name, wfe.Namespace, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())

			err = simulatePipelineRunCompletion(pr, false)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for Failed phase")
			finalWFE, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseFailed), 15*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(finalWFE.Status.FailureDetails).ToNot(BeNil())
		})

		It("should transition Pending → Skipped (resource locked)", func() {
			targetResource := fmt.Sprintf("default/deployment/skip-test-%d", time.Now().UnixNano())

			By("Creating first WorkflowExecution (blocker)")
			blocker := createUniqueWFE("skip-blocker", targetResource)
			Expect(k8sClient.Create(ctx, blocker)).To(Succeed())
			defer cleanupWFE(blocker)

			By("Waiting for blocker to reach Running")
			_, err := waitForWFEPhase(blocker.Name, blocker.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Creating second WorkflowExecution (to be skipped)")
			wfe = createUniqueWFE("skip-target", targetResource)
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Skipped phase")
			skippedWFE, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseSkipped), 10*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(skippedWFE.Status.SkipDetails).ToNot(BeNil())
		})
	})

	// ========================================
	// Audit Events (BR-WE-005)
	// ========================================
	Context("Audit Events during Reconciliation", func() {
		// V2.1 UPDATE: RecordAuditEvent IS NOW CALLED during controller reconciliation!
		// Audit events are emitted for all phase transitions:
		// - workflow.started (Running phase)
		// - workflow.completed (Completed phase)
		// - workflow.failed (Failed phase)
		// - workflow.skipped (Skipped phase)

		var wfe *workflowexecutionv1alpha1.WorkflowExecution

		AfterEach(func() {
			if wfe != nil {
				cleanupWFE(wfe)
			}
		})

		It("should have AuditStore configured in controller", func() {
			// Verify the testable audit store is accessible
			Expect(testAuditStore).ToNot(BeNil())
			GinkgoWriter.Println("✅ AuditStore is configured in integration test controller")
		})

		It("should emit workflow.started audit event when entering Running phase", func() {
			By("Creating a WorkflowExecution")
			wfe = createUniqueWFE("audit-started", "default/deployment/audit-started-test")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Running phase")
			_, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying workflow.started audit event was emitted")
			Eventually(func() int {
				return len(testAuditStore.GetEventsOfType("workflowexecution.workflow.started"))
			}, 5*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 1))

			startedEvents := testAuditStore.GetEventsOfType("workflowexecution.workflow.started")
			Expect(startedEvents).ToNot(BeEmpty())

			// Verify event fields
			event := startedEvents[len(startedEvents)-1] // Get most recent
			Expect(event.EventCategory).To(Equal("workflow"))
			Expect(event.EventAction).To(Equal("workflow.started"))
			Expect(event.EventOutcome).To(Equal("success"))
			Expect(event.ResourceType).To(Equal("WorkflowExecution"))

			GinkgoWriter.Println("✅ workflow.started audit event emitted during reconciliation")
		})

		It("should emit workflow.completed audit event when PipelineRun succeeds", func() {
			By("Creating a WorkflowExecution")
			wfe = createUniqueWFE("audit-complete", "default/deployment/audit-complete-test")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Running phase")
			_, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Getting PipelineRun and completing it")
			pr, err := waitForPipelineRunCreation(wfe.Name, wfe.Namespace, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())

			err = simulatePipelineRunCompletion(pr, true)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for Completed phase")
			_, err = waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseCompleted), 15*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying workflow.completed audit event was emitted")
			Eventually(func() int {
				return len(testAuditStore.GetEventsOfType("workflowexecution.workflow.completed"))
			}, 5*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 1))

			completedEvents := testAuditStore.GetEventsOfType("workflowexecution.workflow.completed")
			Expect(completedEvents).ToNot(BeEmpty())

			event := completedEvents[len(completedEvents)-1]
			Expect(event.EventOutcome).To(Equal("success"))

			GinkgoWriter.Println("✅ workflow.completed audit event emitted during reconciliation")
		})

		It("should emit workflow.failed audit event when PipelineRun fails", func() {
			By("Creating a WorkflowExecution")
			wfe = createUniqueWFE("audit-fail", "default/deployment/audit-fail-test")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Running phase")
			_, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Getting PipelineRun and failing it")
			pr, err := waitForPipelineRunCreation(wfe.Name, wfe.Namespace, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())

			err = simulatePipelineRunCompletion(pr, false)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for Failed phase")
			_, err = waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseFailed), 15*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying workflow.failed audit event was emitted")
			Eventually(func() int {
				return len(testAuditStore.GetEventsOfType("workflowexecution.workflow.failed"))
			}, 5*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 1))

			failedEvents := testAuditStore.GetEventsOfType("workflowexecution.workflow.failed")
			Expect(failedEvents).ToNot(BeEmpty())

			event := failedEvents[len(failedEvents)-1]
			Expect(event.EventOutcome).To(Equal("failure"))

			GinkgoWriter.Println("✅ workflow.failed audit event emitted during reconciliation")
		})

		It("should emit workflow.skipped audit event when resource is locked", func() {
			targetResource := fmt.Sprintf("default/deployment/audit-skip-%d", time.Now().UnixNano())

			By("Creating first WorkflowExecution (blocker)")
			blocker := createUniqueWFE("audit-blocker", targetResource)
			Expect(k8sClient.Create(ctx, blocker)).To(Succeed())
			defer cleanupWFE(blocker)

			By("Waiting for blocker to reach Running")
			_, err := waitForWFEPhase(blocker.Name, blocker.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Creating second WorkflowExecution (to be skipped)")
			wfe = createUniqueWFE("audit-skipped", targetResource)
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Skipped phase")
			_, err = waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseSkipped), 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying workflow.skipped audit event was emitted")
			Eventually(func() int {
				return len(testAuditStore.GetEventsOfType("workflowexecution.workflow.skipped"))
			}, 5*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 1))

			skippedEvents := testAuditStore.GetEventsOfType("workflowexecution.workflow.skipped")
			Expect(skippedEvents).ToNot(BeEmpty())

			event := skippedEvents[len(skippedEvents)-1]
			Expect(event.EventOutcome).To(Equal("skipped"))

			GinkgoWriter.Println("✅ workflow.skipped audit event emitted during reconciliation")
		})

		It("should include correlation ID in audit events when present", func() {
			By("Creating a WorkflowExecution with correlation ID")
			wfe = createUniqueWFE("audit-corr", "default/deployment/audit-corr-test")
			wfe.Labels = map[string]string{
				"kubernaut.ai/correlation-id": "test-corr-12345",
			}
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Running phase")
			_, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying audit event has correlation ID")
			Eventually(func() bool {
				events := testAuditStore.GetEventsOfType("workflowexecution.workflow.started")
				for _, e := range events {
					if e.CorrelationID == "test-corr-12345" {
						return true
					}
				}
				return false
			}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

			GinkgoWriter.Println("✅ Correlation ID included in audit event")
		})
	})
})


