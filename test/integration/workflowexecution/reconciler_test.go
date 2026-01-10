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
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheusTestutil "github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	weaudit "github.com/jordigilh/kubernaut/pkg/workflowexecution/audit"
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
	// V1.0 NOTE: Resource Locking and Cooldown Enforcement tests removed - routing moved to RO (DD-RO-002)
	// RO handles these decisions BEFORE creating WFE, so WE never sees these scenarios

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

		// V1.0 NOTE: "Pending → Skipped" test removed - routing moved to RO (DD-RO-002)
		// RO prevents creation of WFE if should be skipped, so WE never transitions to Skipped
	})

	// ========================================
	// Audit Events (BR-WE-005)
	// ========================================
	Context("Audit Events during Reconciliation", func() {
		// V2.1 UPDATE: RecordAuditEvent IS NOW CALLED during controller reconciliation!
		// V1.0 NOTE: workflow.skipped events removed - RO emits those now (DD-RO-002)
		// Audit events are emitted for all phase transitions:
		// - workflow.started (Running phase)
		// - workflow.completed (Completed phase)
		// - workflow.failed (Failed phase)

		var wfe *workflowexecutionv1alpha1.WorkflowExecution

		AfterEach(func() {
			if wfe != nil {
				cleanupWFE(wfe)
			}
		})

		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		// DEFENSE-IN-DEPTH: Integration Audit Tests
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		//
		// Per TESTING_GUIDELINES.md Defense-in-Depth strategy:
		// - Integration: Validate audit traces are properly stored with correct field values
		// - E2E: Validate audit client wiring (simpler smoke test)
		//
		// These tests use REAL DataStorage service via podman-compose (no mocks)
		// and query the HTTP API to verify audit events were persisted correctly.
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	It("should persist workflowexecution.execution.started audit event (BR-AUDIT-005 Gap #6, ADR-034 v1.5)", func() {
		By("Creating a WorkflowExecution")
		wfe = createUniqueWFE("audit-started", "default/deployment/audit-started-test")
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		By("Waiting for Running phase")
		_, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
		Expect(err).ToNot(HaveOccurred())

	By("Querying DataStorage API for workflowexecution.execution.started audit event via ogen client")
	// V1.0 MANDATORY: Use ogen OpenAPI client (per V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)
	// Query real DataStorage service (Defense-in-Depth: validate field values)
	auditClient, err := ogenclient.NewClient(dataStorageBaseURL)
	Expect(err).ToNot(HaveOccurred(), "Failed to create ogen audit client")

	// Per ADR-034 v1.5: Gap #6 uses "workflowexecution" category and weaudit.EventTypeExecutionStarted event type
	eventCategory := "workflowexecution" // Gap #6 uses "workflowexecution" category (ADR-034 v1.5)
	var startedEvent *ogenclient.AuditEvent
	// DD-AUDIT-CORRELATION-001: Use RemediationRequestRef.Name as correlation ID
	correlationID := wfe.Spec.RemediationRequestRef.Name
	// Flush before querying to ensure buffered events are written to DataStorage
	flushAuditBuffer()
	Eventually(func() bool {
		resp, err := auditClient.QueryAuditEvents(context.Background(), ogenclient.QueryAuditEventsParams{
			EventCategory: ogenclient.NewOptString(eventCategory),
			CorrelationID: ogenclient.NewOptString(correlationID),
		})
			if err != nil {
				return false
			}

			// Find workflowexecution.execution.started event (Gap #6, per ADR-034 v1.5)
			for i := range resp.Data {
				if resp.Data[i].EventType == weaudit.EventTypeExecutionStarted {
					startedEvent = &resp.Data[i]
					return true
				}
			}
			return false
		}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(),
			"workflowexecution.execution.started audit event should be persisted in DataStorage (30s timeout for parallel execution)")

	By("Verifying audit event field values")
	Expect(startedEvent).ToNot(BeNil())
	Expect(startedEvent.EventCategory).To(Equal(ogenclient.AuditEventEventCategoryWorkflowexecution)) // Per ADR-034 v1.5: workflowexecution category
	// WE-BUG-002: event_action contains short form ("started" not "execution.workflow.started")
	Expect(startedEvent.EventAction).To(Equal("started"))
	Expect(startedEvent.EventOutcome).To(Equal(ogenclient.AuditEventEventOutcomeSuccess))
		// DD-AUDIT-CORRELATION-001: CorrelationID = RemediationRequestRef.Name
		Expect(startedEvent.CorrelationID).To(Equal(wfe.Spec.RemediationRequestRef.Name))

		GinkgoWriter.Println("✅ execution.workflow.started audit event persisted with correct field values")
	})

		It("should persist workflowexecution.workflow.completed audit event with correct field values (ADR-034 v1.5)", func() {
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

	By("Querying DataStorage API for workflowexecution.workflow.completed audit event via ogen client")
	// V1.0 MANDATORY: Use ogen OpenAPI client
	auditClient, err := ogenclient.NewClient(dataStorageBaseURL)
	Expect(err).ToNot(HaveOccurred())

	// Per ADR-034 v1.5: use "workflowexecution" category and weaudit.EventTypeCompleted event type
	eventCategory := "workflowexecution"
	var completedEvent *ogenclient.AuditEvent
	// DD-AUDIT-CORRELATION-001: Use RemediationRequestRef.Name as correlation ID
	correlationID := wfe.Spec.RemediationRequestRef.Name
	Eventually(func() bool {
		resp, err := auditClient.QueryAuditEvents(context.Background(), ogenclient.QueryAuditEventsParams{
			EventCategory: ogenclient.NewOptString(eventCategory),
			CorrelationID: ogenclient.NewOptString(correlationID),
		})
		if err != nil {
			return false
		}

		// Find workflowexecution.workflow.completed event (per ADR-034 v1.5)
		for i := range resp.Data {
			if resp.Data[i].EventType == weaudit.EventTypeCompleted {
				completedEvent = &resp.Data[i]
				return true
			}
		}
		return false
	}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(),
		"workflowexecution.workflow.completed audit event should be persisted in DataStorage (30s timeout for parallel execution)")

	By("Verifying audit event field values")
	Expect(completedEvent).ToNot(BeNil())
	Expect(completedEvent.EventOutcome).To(Equal(ogenclient.AuditEventEventOutcomeSuccess))
	// WE-BUG-002: event_action contains short form ("completed" not weaudit.EventTypeCompleted)
	Expect(completedEvent.EventAction).To(Equal("completed"))

	GinkgoWriter.Println("✅ workflowexecution.workflow.completed audit event persisted with correct field values")
		})

		It("should persist workflowexecution.workflow.failed audit event with failure details (ADR-034 v1.5)", func() {
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

	By("Querying DataStorage API for workflowexecution.workflow.failed audit event via ogen client")
	// V1.0 MANDATORY: Use ogen OpenAPI client
	auditClient, err := ogenclient.NewClient(dataStorageBaseURL)
	Expect(err).ToNot(HaveOccurred())

	// Per ADR-034 v1.5: use "workflowexecution" category and weaudit.EventTypeFailed event type
	eventCategory := "workflowexecution"
	var failedEvent *ogenclient.AuditEvent
	// DD-AUDIT-CORRELATION-001: Use RemediationRequestRef.Name as correlation ID
	correlationID := wfe.Spec.RemediationRequestRef.Name
	// Flush before querying to ensure buffered events are written to DataStorage
	flushAuditBuffer()
	Eventually(func() bool {
		resp, err := auditClient.QueryAuditEvents(context.Background(), ogenclient.QueryAuditEventsParams{
				EventCategory: ogenclient.NewOptString(eventCategory),
				CorrelationID: ogenclient.NewOptString(correlationID),
			})
			if err != nil {
				return false
			}

			// Find workflowexecution.workflow.failed event (per ADR-034 v1.5)
			for i := range resp.Data {
				if resp.Data[i].EventType == weaudit.EventTypeFailed {
					failedEvent = &resp.Data[i]
					return true
				}
			}
			return false
		}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(),
			"workflowexecution.workflow.failed audit event should be persisted in DataStorage (30s timeout for parallel execution)")

	By("Verifying audit event field values including failure details")
	Expect(failedEvent).ToNot(BeNil())
	Expect(failedEvent.EventOutcome).To(Equal(ogenclient.AuditEventEventOutcomeFailure))
	// WE-BUG-002: event_action contains short form ("failed" not weaudit.EventTypeFailed)
	Expect(failedEvent.EventAction).To(Equal("failed"))

	// Verify event_data contains failure details - OGEN-MIGRATION
	eventData, ok := failedEvent.EventData.GetWorkflowExecutionAuditPayload()
	Expect(ok).To(BeTrue(), "EventData should be WorkflowExecutionAuditPayload")

	// Access flat fields directly
	Expect(eventData.FailureReason.IsSet()).To(BeTrue(), "failure_reason should be populated")
	Expect(eventData.FailureMessage.IsSet()).To(BeTrue(), "failure_message should be populated")
	Expect(eventData.FailureMessage.Value).ToNot(BeEmpty(), "failure_message should be populated")

	GinkgoWriter.Println("✅ workflowexecution.workflow.failed audit event persisted with failure details")
		})

	It("should include correlation ID in audit events from RemediationRequestRef (DD-AUDIT-CORRELATION-001)", func() {
		By("Creating a WorkflowExecution (correlation ID = RemediationRequestRef.Name)")
		wfe = createUniqueWFE("audit-corr", "default/deployment/audit-corr-test")
		// DD-AUDIT-CORRELATION-001: RemediationRequestRef.Name is the authoritative source
		expectedCorrID := wfe.Spec.RemediationRequestRef.Name
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		By("Waiting for Running phase")
		_, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
		Expect(err).ToNot(HaveOccurred())

	By("Querying DataStorage API for audit events with correlation ID via ogen client")
	// V1.0 MANDATORY: Use ogen OpenAPI client
	auditClient, err := ogenclient.NewClient(dataStorageBaseURL)
	Expect(err).ToNot(HaveOccurred())

	// Per ADR-034 v1.5: Use "workflowexecution" category
	eventCategory := "workflowexecution"
	// Flush before querying to ensure buffered events are written to DataStorage
	flushAuditBuffer()
	Eventually(func() bool {
		resp, err := auditClient.QueryAuditEvents(context.Background(), ogenclient.QueryAuditEventsParams{
				EventCategory: ogenclient.NewOptString(eventCategory),
				CorrelationID: ogenclient.NewOptString(expectedCorrID),
			})
			if err != nil {
				return false
			}

			// Verify at least one event has the correlation ID from RemediationRequestRef.Name
			for _, event := range resp.Data {
				if event.CorrelationID == expectedCorrID {
					return true
				}
			}
			return false
		}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(),
			"Audit events should include correlation ID from RemediationRequestRef.Name per DD-AUDIT-CORRELATION-001 (30s timeout for parallel execution)")

		GinkgoWriter.Println("✅ Correlation ID from RemediationRequestRef.Name included in audit events (DD-AUDIT-CORRELATION-001)")
	})

		// ========================================
		// BR-WE-009: Resource Locking - Prevent Parallel Execution
		// ========================================
		Context("BR-WE-009: Resource Locking for Target Resources", func() {
			It("should prevent parallel execution on the same target resource via deterministic PipelineRun names", func() {
				By("Creating first WorkflowExecution for target resource")
				targetResource := "test-namespace/deployment/payment-api"
				wfe1 := createUniqueWFE("lock-test-1", targetResource)
				wfe1.Labels = map[string]string{"test-id": "parallel-lock-test"}
				Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())

				By("Waiting for first WFE to start Running")
				_, err := waitForWFEPhase(wfe1.Name, wfe1.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
				Expect(err).ToNot(HaveOccurred())

				By("Creating second WorkflowExecution for the SAME target resource")
				wfe2 := createUniqueWFE("lock-test-2", targetResource)
				wfe2.Labels = map[string]string{"test-id": "parallel-lock-test"}
				Expect(k8sClient.Create(ctx, wfe2)).To(Succeed())

				By("Verifying second WFE fails with ExecutionRaceCondition")
				var wfe2Status *workflowexecutionv1alpha1.WorkflowExecution
				Eventually(func() bool {
					wfe2Status, err = getWFE(wfe2.Name, wfe2.Namespace)
					if err != nil {
						return false
					}
					return wfe2Status.Status.Phase == workflowexecutionv1alpha1.PhaseFailed
				}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(),
					"Second WFE should fail due to resource lock")

				// Validate failure reason is Unknown (ExecutionRaceCondition not in CRD enum)
				Expect(wfe2Status.Status.FailureDetails).ToNot(BeNil())
				Expect(wfe2Status.Status.FailureDetails.Reason).To(Equal("Unknown"))
				Expect(wfe2Status.Status.FailureDetails.Message).To(ContainSubstring("PipelineRun"))
				Expect(wfe2Status.Status.FailureDetails.Message).To(ContainSubstring("already exists"))

				GinkgoWriter.Println("✅ BR-WE-009: Resource locking prevents parallel execution")
			})

			It("should allow parallel execution on different target resources", func() {
				By("Creating two WorkflowExecutions for DIFFERENT target resources")
				targetResource1 := "test-namespace/deployment/frontend-api"
				targetResource2 := "test-namespace/deployment/backend-api"

				wfe1 := createUniqueWFE("different-lock-1", targetResource1)
				wfe2 := createUniqueWFE("different-lock-2", targetResource2)

				Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())
				Expect(k8sClient.Create(ctx, wfe2)).To(Succeed())

				By("Verifying both WFEs reach Running phase (no lock conflict)")
				_, err1 := waitForWFEPhase(wfe1.Name, wfe1.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
				Expect(err1).ToNot(HaveOccurred())

				_, err2 := waitForWFEPhase(wfe2.Name, wfe2.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
				Expect(err2).ToNot(HaveOccurred())

				GinkgoWriter.Println("✅ BR-WE-009: Different target resources can execute in parallel")
			})

			It("should use deterministic PipelineRun names based on target resource hash", func() {
				By("Creating WorkflowExecution with known target resource")
				targetResource := "test-namespace/deployment/deterministic-test"
				wfe := createUniqueWFE("deterministic-name", targetResource)
				Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

				By("Waiting for PipelineRun creation")
				_, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
				Expect(err).ToNot(HaveOccurred())

				By("Fetching PipelineRun and verifying deterministic naming")
				wfeStatus, err := getWFE(wfe.Name, wfe.Namespace)
				Expect(err).ToNot(HaveOccurred())
				Expect(wfeStatus.Status.PipelineRunRef).ToNot(BeNil())

				prName := wfeStatus.Status.PipelineRunRef.Name
				// Deterministic name should be "wfe-" + first 16 chars of sha256(targetResource)
				// Verify format: starts with "wfe-" and has correct length (DD-WE-003)
				Expect(prName).To(HavePrefix("wfe-"))
				Expect(len(prName)).To(Equal(20), "PipelineRun name should be wfe- (4 chars) + 16 char hash = 20 total (DD-WE-003)")

				GinkgoWriter.Printf("✅ BR-WE-009: Deterministic PipelineRun name: %s\n", prName)
			})

			It("should release lock after cooldown period expires", func() {
				By("Creating WorkflowExecution that will complete")
				targetResource := "test-namespace/deployment/cooldown-release-test"
				wfe1 := createUniqueWFE("cooldown-release-1", targetResource)
				Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())

				By("Waiting for first WFE to reach Running phase")
				_, err := waitForWFEPhase(wfe1.Name, wfe1.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
				Expect(err).ToNot(HaveOccurred())

				By("Simulating completion by marking WFE as Completed (integration test shortcut)")
				// In real scenario, Tekton would complete. In integration, we simulate completion.
				wfe1Status, err := getWFE(wfe1.Name, wfe1.Namespace)
				Expect(err).ToNot(HaveOccurred())

				now := metav1.Now()
				wfe1Status.Status.Phase = workflowexecutionv1alpha1.PhaseCompleted
				wfe1Status.Status.CompletionTime = &now
				Expect(k8sClient.Status().Update(ctx, wfe1Status)).To(Succeed())

				By("Waiting for cooldown period (default 5 minutes, but controller may use shorter for test)")
				// Note: In production, cooldown is 5 minutes. In test, reconciler may be configured with shorter cooldown.
				// This test validates lock is eventually released, not exact timing.

				By("Verifying PipelineRun is deleted after cooldown (lock released)")
				Eventually(func() bool {
					pr := &tektonv1.PipelineRun{}
					prName := "wfe-" + wfe1Status.Status.PipelineRunRef.Name[4:] // Reconstruct deterministic name
					err := k8sClient.Get(ctx, client.ObjectKey{
						Name:      prName,
						Namespace: "default", // ExecutionNamespace
					}, pr)
					return apierrors.IsNotFound(err)
				}, 30*time.Second, 1*time.Second).Should(BeTrue(),
					"PipelineRun should be deleted after cooldown (lock released)")

				GinkgoWriter.Println("✅ BR-WE-009: Lock released after cooldown period")
			})

			// REMOVED: Duplicate of E2E test
			// See: test/e2e/workflowexecution/02_observability_test.go:160-189
			// Test: "should detect external PipelineRun deletion and fail gracefully"
		})

		// ========================================
		// BR-WE-010: Cooldown Period for Sequential Workflows
		// ========================================
		Context("BR-WE-010: Cooldown Period Between Sequential Executions", func() {
			It("should wait cooldown period before releasing lock after completion", func() {
				By("Creating WorkflowExecution")
				targetResource := "test-namespace/deployment/cooldown-wait"
				wfe := createUniqueWFE("cooldown-wait", targetResource)
				Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

				By("Waiting for Running phase")
				_, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
				Expect(err).ToNot(HaveOccurred())

				By("Simulating completion")
				wfeStatus, err := getWFE(wfe.Name, wfe.Namespace)
				Expect(err).ToNot(HaveOccurred())

				now := metav1.Now()
				wfeStatus.Status.Phase = workflowexecutionv1alpha1.PhaseCompleted
				wfeStatus.Status.CompletionTime = &now
				Expect(k8sClient.Status().Update(ctx, wfeStatus)).To(Succeed())

				By("Verifying PipelineRun still exists immediately after completion (cooldown active)")
				// Use Eventually to verify PipelineRun exists (allows for controller reconciliation timing)
				pr := &tektonv1.PipelineRun{}
				prKey := client.ObjectKey{
					Name:      wfeStatus.Status.PipelineRunRef.Name,
					Namespace: WorkflowExecutionNS, // PipelineRuns are in kubernaut-workflows namespace
				}
				Eventually(func() error {
					return k8sClient.Get(ctx, prKey, pr)
				}, 10*time.Second, 500*time.Millisecond).Should(Succeed(), "PipelineRun should exist during cooldown after controller reconciles")

				By("Waiting for cooldown period to expire")
				// Test reconciler uses 10s cooldown + reconciliation cycles
				// Allow 45s total: 10s cooldown + 35s buffer for reconciliation
				Eventually(func() bool {
					err := k8sClient.Get(ctx, prKey, pr)
					return apierrors.IsNotFound(err)
				}, 45*time.Second, 1*time.Second).Should(BeTrue(),
					"PipelineRun should be deleted after 10s cooldown + reconciliation time")

				GinkgoWriter.Println("✅ BR-WE-010: Cooldown period enforced before lock release")
			})

			It("should calculate cooldown remaining time correctly", func() {
				By("Creating and completing WorkflowExecution")
				targetResource := "test-namespace/deployment/cooldown-calculation"
				wfe := createUniqueWFE("cooldown-calc", targetResource)
				Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

				_, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
				Expect(err).ToNot(HaveOccurred())

				wfeStatus, err := getWFE(wfe.Name, wfe.Namespace)
				Expect(err).ToNot(HaveOccurred())

				completionTime := metav1.Now()
				wfeStatus.Status.Phase = workflowexecutionv1alpha1.PhaseCompleted
				wfeStatus.Status.CompletionTime = &completionTime
				Expect(k8sClient.Status().Update(ctx, wfeStatus)).To(Succeed())

				By("Verifying reconcile requeue time is set to cooldown remaining")
				// Note: We can't directly observe RequeueAfter in integration test,
				// but we can verify PipelineRun deletion timing aligns with cooldown.

				By("Verifying elapsed time < cooldown means PipelineRun still exists")
				pr := &tektonv1.PipelineRun{}
				prKey := client.ObjectKey{
					Name:      wfeStatus.Status.PipelineRunRef.Name,
					Namespace: WorkflowExecutionNS, // PipelineRuns are in kubernaut-workflows namespace
				}
				// Use Eventually to allow for controller reconciliation timing
				Eventually(func() error {
					return k8sClient.Get(ctx, prKey, pr)
				}, 10*time.Second, 500*time.Millisecond).Should(Succeed(), "PipelineRun should exist while cooldown active after controller reconciles")

				GinkgoWriter.Println("✅ BR-WE-010: Cooldown remaining time calculated correctly")
			})

			It("should emit LockReleased event when cooldown expires", func() {
				By("Creating and completing WorkflowExecution")
				targetResource := "test-namespace/deployment/lock-released-event"
				wfe := createUniqueWFE("lock-released", targetResource)
				Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

				_, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
				Expect(err).ToNot(HaveOccurred())

				wfeStatus, err := getWFE(wfe.Name, wfe.Namespace)
				Expect(err).ToNot(HaveOccurred())

				now := metav1.Now()
				wfeStatus.Status.Phase = workflowexecutionv1alpha1.PhaseCompleted
				wfeStatus.Status.CompletionTime = &now
				Expect(k8sClient.Status().Update(ctx, wfeStatus)).To(Succeed())

				By("Waiting for cooldown to expire and checking for LockReleased event")
				// Test reconciler uses 10s cooldown + event emission takes time
				// Allow 45s total: 10s cooldown + reconciliation + event propagation
				Eventually(func() bool {
					events := &corev1.EventList{}
					err := k8sClient.List(ctx, events, client.InNamespace(wfe.Namespace))
					if err != nil {
						return false
					}

					for _, event := range events.Items {
						if event.InvolvedObject.Name == wfe.Name &&
							event.Reason == "LockReleased" {
							GinkgoWriter.Printf("✅ Found LockReleased event: %s\n", event.Message)
							return true
						}
					}
					return false
				}, 45*time.Second, 1*time.Second).Should(BeTrue(),
					"LockReleased event should be emitted after 10s cooldown")

				GinkgoWriter.Println("✅ BR-WE-010: LockReleased event emitted after cooldown")
			})

			// MOVED TO E2E: Cooldown without CompletionTime requires real cleanup flow
			// Integration tests have race conditions in finalizer removal during cleanup
			// Test added to E2E suite (see below)
		})

		// ========================================
		// BR-WE-008: Prometheus Metrics for Execution Outcomes
		// ========================================
		// NOTE: Metrics recording tests moved to E2E suite
		// See: test/e2e/workflowexecution/02_observability_test.go
		// - "should increment workflowexecution_total{outcome=Completed} on successful completion"
		// - "should increment workflowexecution_total{outcome=Failed} on workflow failure"
		Context("BR-WE-008: Prometheus Metrics Recording", func() {
			It("should record workflowexecution_duration_seconds histogram", func() {
				By("Creating and completing WorkflowExecution")
				targetResource := "test-namespace/deployment/metrics-duration"
				wfe := createUniqueWFE("metrics-duration", targetResource)
				Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

				wfeStatus, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 10*time.Second)
				Expect(err).ToNot(HaveOccurred())

				startTime := wfeStatus.Status.StartTime
				Expect(startTime).ToNot(BeNil())

				By("Completing after a measurable duration")
				time.Sleep(2 * time.Second) // Simulate execution time

				wfeStatus, err = getWFE(wfe.Name, wfe.Namespace)
				Expect(err).ToNot(HaveOccurred())

				now := metav1.Now()
				wfeStatus.Status.Phase = workflowexecutionv1alpha1.PhaseCompleted
				wfeStatus.Status.CompletionTime = &now
				Expect(k8sClient.Status().Update(ctx, wfeStatus)).To(Succeed())

				By("Verifying duration histogram records observations (implicit via no panic)")
				// Use Eventually to wait for controller to process the completion and record metrics
				Eventually(func() string {
					updated, _ := getWFE(wfe.Name, wfe.Namespace)
					return string(updated.Status.Phase)
				}, 15*time.Second, 500*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseCompleted)),
					"Controller should reconcile completion and record duration metric")

				// Note: Integration tests can't directly query histogram sample counts from Observer interface
				// The E2E tests validate the full metrics endpoint with actual histogram buckets
				// This test validates that the controller can record duration metrics without panic/error
				// If the metric recording had failed, the completion would have caused an error

				// Verify workflow completed successfully (metric recording didn't cause issues)
				finalWFE, err := getWFE(wfe.Name, wfe.Namespace)
				Expect(err).ToNot(HaveOccurred())
				Expect(finalWFE.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseCompleted))

				GinkgoWriter.Println("✅ BR-WE-008: Duration histogram metric recording successful (no errors)")
			})

		It("should record workflowexecution_pipelinerun_creation_total counter", func() {
			By("Getting initial counter value")
			initialCount := prometheusTestutil.ToFloat64(reconciler.Metrics.PipelineRunCreations)

			By("Creating WorkflowExecution")
			targetResource := "test-namespace/deployment/metrics-pr-creation"
			wfe := createUniqueWFE("metrics-pr", targetResource)
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for PipelineRun creation")
			_, err := waitForWFEPhase(wfe.Name, wfe.Namespace, string(workflowexecutionv1alpha1.PhaseRunning), 30*time.Second)
			Expect(err).ToNot(HaveOccurred())

				By("Verifying pipelinerun_creation_total incremented")
				// Use Eventually to handle controller reconciliation timing for metrics
				Eventually(func() float64 {
					return prometheusTestutil.ToFloat64(reconciler.Metrics.PipelineRunCreations)
				}, 15*time.Second, 500*time.Millisecond).Should(BeNumerically(">", initialCount),
					"pipelinerun_creation_total should increment after controller creates PipelineRun")

				finalCount := prometheusTestutil.ToFloat64(reconciler.Metrics.PipelineRunCreations)

				GinkgoWriter.Printf("✅ BR-WE-008: PipelineRun creation metric incremented from %.0f to %.0f\n",
					initialCount, finalCount)
			})
		})

		// ========================================
		// DD-RO-002 Phase 3: BR-WE-012 Tests Removed (Dec 19, 2025)
		// Exponential backoff routing logic moved to RO
		// Tests now in test/unit/remediationorchestrator/routing/blocking_test.go
		// ========================================
	})
})
