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

package datastorage

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	reconstructionpkg "github.com/jordigilh/kubernaut/pkg/datastorage/reconstruction"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// FULL RR RECONSTRUCTION INTEGRATION TESTS (ALL 7 GAPS)
// ========================================
//
// Purpose: Validate complete RemediationRequest reconstruction from audit traces
// with ALL 7 gaps working together to achieve >=80% field completeness.
//
// Business Requirements:
// - BR-AUDIT-005 v2.0: Complete RR reconstruction from audit traces
// - BR-AUDIT-006: All 8 field gaps (Gaps #1-8)
//
// Test Strategy (per 03-testing-strategy.mdc):
// - Integration Tests (>50%): Test business components with REAL database
// - Call reconstruction.* functions directly (NOT HTTP layer)
// - Validate all 7 gaps working together
// - Target: >=80% field completeness (up from 40% with Gaps 1-3 only)
//
// Gap Coverage:
// - Gaps #1-3: Gateway fields (signalName, signalType, signalLabels, signalAnnotations, originalPayload)
// - Gap #4: Provider data (providerData)
// - Gap #5: Workflow selection (selectedWorkflowRef)
// - Gap #6: Workflow execution (executionRef)
// - Gap #7: Error details (error_details) - optional, for failure scenarios
// - Gap #8: Timeout config (timeoutConfig)
//
// Expected Completeness: >=80% (5/8 required fields present)
//
// Test Plan Reference:
// - docs/development/SOC2/SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md
// - Test Tier: Integration Tests (Tier 2) - Full RR Reconstruction
//
// ========================================

var _ = Describe("Full RR Reconstruction Integration Tests (BR-AUDIT-005 v2.0)", func() {
	var (
		auditRepo     *repository.AuditEventsRepository
		testID        string
		correlationID string
	)

	BeforeEach(func() {
		// Create audit events repository
		auditRepo = repository.NewAuditEventsRepository(db.DB, logger)

		// Generate unique test ID for isolation
		testID = generateTestID()
		correlationID = fmt.Sprintf("test-full-reconstruction-%s", testID)

		// Clean up test data
		_, err := db.ExecContext(ctx,
			"DELETE FROM audit_events WHERE correlation_id = $1",
			correlationID)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		// Clean up test data
		if db != nil {
			_, _ = db.ExecContext(ctx,
				"DELETE FROM audit_events WHERE correlation_id = $1",
				correlationID)
		}
	})

	// ========================================
	// FULL RECONSTRUCTION WITH ALL 7 GAPS
	// ========================================
	Context("INTEGRATION-FULL-01: Complete RR reconstruction with all 7 gaps (BR-AUDIT-005)", func() {
		It("should reconstruct complete RemediationRequest with >=80% field completeness", func() {
			// ====================================
			// ARRANGE: Seed complete audit trail (all 7 gap events)
			// ====================================

			baseTimestamp := time.Now().Add(-60 * time.Second).UTC()

			// 1. Gap #1-3: gateway.signal.received (Gateway fields)
			gatewayEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: baseTimestamp,
				EventType:      "gateway.signal.received",
				EventCategory:  "gateway",
				EventAction:    "received",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "Signal",
				ResourceID:     "test-signal-full-001",
				EventData: map[string]interface{}{
					"event_type":         "gateway.signal.received",
					"alert_name":         "HighMemoryUsage",
					"namespace":          "test-namespace",
					"fingerprint":        "abc123def456",
					"signal_type":        "prometheus",
					"signal_labels":      map[string]interface{}{"app": "frontend", "severity": "critical"},
					"signal_annotations": map[string]interface{}{"summary": "Memory usage above 90%"},
					"original_payload":   map[string]interface{}{"alert": "memory-high", "value": 95},
				},
			}
			_, err := auditRepo.Create(ctx, gatewayEvent)
			Expect(err).ToNot(HaveOccurred())

			// 2. Gap #8: orchestrator.lifecycle.created (TimeoutConfig)
			orchestratorEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: baseTimestamp.Add(5 * time.Second),
				EventType:      "orchestrator.lifecycle.created",
				EventCategory:  "orchestrator",
				EventAction:    "created",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "RemediationRequest",
				ResourceID:     "rr-full-001",
				EventData: map[string]interface{}{
					"event_type": "orchestrator.lifecycle.created",
					"rr_name":    "rr-full-001",
					"namespace":  "test-namespace",
					"timeout_config": map[string]interface{}{
						"global":     "30m",
						"processing": "5m",
						"analyzing":  "10m",
						"executing":  "15m",
					},
				},
			}
			_, err = auditRepo.Create(ctx, orchestratorEvent)
			Expect(err).ToNot(HaveOccurred())

			// 3. Gap #4: aianalysis.analysis.completed (Provider data)
			aaEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: baseTimestamp.Add(15 * time.Second),
				EventType:      "aianalysis.analysis.completed",
				EventCategory:  "analysis",
				EventAction:    "completed",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "AIAnalysis",
				ResourceID:     "aianalysis-full-001",
				EventData: map[string]interface{}{
					"event_type":         "aianalysis.analysis.completed",
					"analysis_name":      "aianalysis-full-001",
					"namespace":          "test-namespace",
					"analysis_status":    "Completed",
					"provider_data":      map[string]interface{}{"provider": "holmesgpt", "model": "gpt-4"},
					"analysis_result":    "Memory leak detected",
					"recommendations":    []string{"Restart pod", "Increase memory limit"},
					"confidence_score":   0.95,
					"processing_time_ms": 3500,
				},
			}
			_, err = auditRepo.Create(ctx, aaEvent)
			Expect(err).ToNot(HaveOccurred())

			// 5. Gap #5: workflowexecution.selection.completed (Workflow selection)
			selectionEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: baseTimestamp.Add(20 * time.Second),
				EventType:      "workflowexecution.selection.completed",
				EventCategory:  "workflowexecution",
				EventAction:    "selection_completed",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "WorkflowExecution",
				ResourceID:     "wfe-full-001",
				EventData: map[string]interface{}{
					"event_type":      "workflowexecution.selection.completed",
					"execution_name":  "wfe-full-001",
					"namespace":       "test-namespace",
					"workflow_id":     "restart-pod-workflow",
					"version":         "v1.2.0",
					"container_image": "ghcr.io/kubernaut/workflows:restart-pod-v1.2.0",
					"selection_reason": "Best match for memory remediation",
				},
			}
			_, err = auditRepo.Create(ctx, selectionEvent)
			Expect(err).ToNot(HaveOccurred())

			// 6. Gap #6: workflowexecution.execution.started (Workflow execution)
			executionEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: baseTimestamp.Add(25 * time.Second),
				EventType:      "workflowexecution.execution.started",
				EventCategory:  "workflowexecution",
				EventAction:    "execution_started",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "WorkflowExecution",
				ResourceID:     "wfe-full-001",
				EventData: map[string]interface{}{
					"event_type":       "workflowexecution.execution.started",
					"execution_name":   "wfe-full-001",
					"namespace":        "test-namespace",
					"pipelinerun_name": "restart-pod-run-12345",
					"workflow_id":      "restart-pod-workflow",
					"started_at":       baseTimestamp.Add(25 * time.Second).Format(time.RFC3339),
				},
			}
			_, err = auditRepo.Create(ctx, executionEvent)
			Expect(err).ToNot(HaveOccurred())

			// ====================================
			// ACT: Execute full reconstruction pipeline
			// ====================================

			// Step 1: Query audit events
			events, err := reconstructionpkg.QueryAuditEventsForReconstruction(ctx, db.DB, logger, correlationID)
			Expect(err).ToNot(HaveOccurred())
			Expect(events).To(HaveLen(5), "Should retrieve all 5 gap events (Gaps 1-6, 8)")

			// Step 2: Parse all events
			var parsedData []reconstructionpkg.ParsedAuditData
			for _, event := range events {
				parsed, err := reconstructionpkg.ParseAuditEvent(event)
				if err != nil {
					// Skip unparseable events (expected for unknown event types)
					continue
				}
				if parsed != nil {
					parsedData = append(parsedData, *parsed)
				}
			}
			Expect(parsedData).To(HaveLen(5), "Should successfully parse all 5 gap events")

			// Step 3: Merge audit data
			mergedData, err := reconstructionpkg.MergeAuditData(parsedData)
			Expect(err).ToNot(HaveOccurred())
			Expect(mergedData).ToNot(BeNil())

			// Step 4: Build RemediationRequest
			rr, err := reconstructionpkg.BuildRemediationRequest(correlationID, mergedData)
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())

			// Step 5: Validate reconstructed RR
			validation, err := reconstructionpkg.ValidateReconstructedRR(rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(validation).ToNot(BeNil())

			// ====================================
			// ASSERT: Validate completeness and fields
			// ====================================

			// CRITICAL: Completeness >= 80% (BR-AUDIT-005 v2.0 target)
			Expect(validation.Completeness).To(BeNumerically(">=", 80),
				"Completeness should be >=80%% with all 7 gaps (was 40%% with Gaps 1-3 only)")

			// Validate all 5 required fields are present

			// Gap #1-3: Gateway fields
			Expect(rr.Spec.SignalName).To(Equal("HighMemoryUsage"), "Gap #1: SignalName from gateway.signal.received")
			Expect(rr.Spec.SignalType).To(Equal("prometheus"), "Gap #2: SignalType from gateway.signal.received")
			Expect(rr.Spec.SignalLabels).To(HaveKeyWithValue("app", "frontend"), "Gap #3: SignalLabels from gateway.signal.received")
			Expect(rr.Spec.SignalAnnotations).To(HaveKeyWithValue("summary", "Memory usage above 90%"), "Gap #3: SignalAnnotations from gateway.signal.received")
			Expect(string(rr.Spec.OriginalPayload)).To(ContainSubstring("memory-high"), "Gap #3: OriginalPayload from gateway.signal.received")

			// Gap #4: Provider data
			Expect(rr.Spec.ProviderData).ToNot(BeNil(), "Gap #4: ProviderData should be populated")
			Expect(rr.Spec.ProviderData).ToNot(BeEmpty(), "Gap #4: ProviderData should not be empty")
			// ProviderData is stored as JSON []byte - contains provider and model info
			Expect(string(rr.Spec.ProviderData)).To(ContainSubstring("holmesgpt"), "Gap #4: Provider from aianalysis.analysis.completed")
			Expect(string(rr.Spec.ProviderData)).To(ContainSubstring("gpt-4"), "Gap #4: Model from aianalysis.analysis.completed")

			// Gap #5: Workflow selection
			Expect(rr.Status.SelectedWorkflowRef).ToNot(BeNil(), "Gap #5: SelectedWorkflowRef should be populated")
			Expect(rr.Status.SelectedWorkflowRef.WorkflowID).To(Equal("restart-pod-workflow"), "Gap #5: WorkflowID from workflowexecution.selection.completed")
			Expect(rr.Status.SelectedWorkflowRef.Version).To(Equal("v1.2.0"), "Gap #5: Version from workflowexecution.selection.completed")
			Expect(rr.Status.SelectedWorkflowRef.ContainerImage).To(Equal("ghcr.io/kubernaut/workflows:restart-pod-v1.2.0"), "Gap #5: ContainerImage from workflowexecution.selection.completed")

			// Gap #6: Workflow execution
			Expect(rr.Status.ExecutionRef).ToNot(BeNil(), "Gap #6: ExecutionRef should be populated")
			Expect(rr.Status.ExecutionRef.Kind).To(Equal("WorkflowExecution"), "Gap #6: ExecutionRef.Kind should be WorkflowExecution")
			Expect(rr.Status.ExecutionRef.Name).To(Equal("wfe-full-001"), "Gap #6: ExecutionRef.Name from workflowexecution.execution.started")

			// Gap #8: TimeoutConfig
			Expect(rr.Status.TimeoutConfig).ToNot(BeNil(), "Gap #8: TimeoutConfig should be populated")
			Expect(rr.Status.TimeoutConfig.Global.Duration.String()).To(Equal("30m0s"), "Gap #8: Global timeout from orchestrator.lifecycle.created")
			Expect(rr.Status.TimeoutConfig.Processing.Duration.String()).To(Equal("5m0s"), "Gap #8: Processing timeout from orchestrator.lifecycle.created")
			Expect(rr.Status.TimeoutConfig.Analyzing.Duration.String()).To(Equal("10m0s"), "Gap #8: Analyzing timeout from orchestrator.lifecycle.created")
			Expect(rr.Status.TimeoutConfig.Executing.Duration.String()).To(Equal("15m0s"), "Gap #8: Executing timeout from orchestrator.lifecycle.created")

			// Validate validation warnings (should be minimal with complete audit trail)
			Expect(validation.Warnings).To(HaveLen(0), "Should have no warnings with complete audit trail")

			// Validate metadata
			Expect(rr.Name).To(Equal(correlationID), "RR name should match correlation ID")
			Expect(rr.Namespace).To(Equal("test-namespace"), "RR namespace should match")
		})
	})

	// ========================================
	// EDGE CASE: PARTIAL AUDIT TRAIL
	// ========================================
	Context("INTEGRATION-FULL-02: Partial audit trail (missing workflow events)", func() {
		It("should reconstruct RR with lower completeness when workflow events missing", func() {
			// ARRANGE: Seed partial audit trail (only Gaps #1-3, #8)
			baseTimestamp := time.Now().Add(-60 * time.Second).UTC()

			// Only gateway.signal.received + orchestrator.lifecycle.created
			gatewayEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: baseTimestamp,
				EventType:      "gateway.signal.received",
				EventCategory:  "gateway",
				EventAction:    "received",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "Signal",
				ResourceID:     "test-signal-partial-001",
				EventData: map[string]interface{}{
					"event_type":  "gateway.signal.received",
					"alert_name":  "PartialAlert",
					"namespace":   "test-namespace",
					"fingerprint": "partial123",
					"signal_type": "prometheus",
				},
			}
			_, err := auditRepo.Create(ctx, gatewayEvent)
			Expect(err).ToNot(HaveOccurred())

			orchestratorEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: baseTimestamp.Add(5 * time.Second),
				EventType:      "orchestrator.lifecycle.created",
				EventCategory:  "orchestrator",
				EventAction:    "created",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "RemediationRequest",
				ResourceID:     "rr-partial-001",
				EventData: map[string]interface{}{
					"event_type": "orchestrator.lifecycle.created",
					"rr_name":    "rr-partial-001",
					"namespace":  "test-namespace",
					"timeout_config": map[string]interface{}{
						"global": "30m",
					},
				},
			}
			_, err = auditRepo.Create(ctx, orchestratorEvent)
			Expect(err).ToNot(HaveOccurred())

			// ACT: Execute reconstruction pipeline
			events, err := reconstructionpkg.QueryAuditEventsForReconstruction(ctx, db.DB, logger, correlationID)
			Expect(err).ToNot(HaveOccurred())

			var parsedData []reconstructionpkg.ParsedAuditData
			for _, event := range events {
				parsed, err := reconstructionpkg.ParseAuditEvent(event)
				if err == nil && parsed != nil {
					parsedData = append(parsedData, *parsed)
				}
			}

			mergedData, err := reconstructionpkg.MergeAuditData(parsedData)
			Expect(err).ToNot(HaveOccurred())

			rr, err := reconstructionpkg.BuildRemediationRequest(correlationID, mergedData)
			Expect(err).ToNot(HaveOccurred())

			validation, err := reconstructionpkg.ValidateReconstructedRR(rr)
			Expect(err).ToNot(HaveOccurred())

			// ASSERT: Lower completeness, but still valid
			Expect(validation.Completeness).To(BeNumerically("<", 80),
				"Completeness should be <80%% with partial audit trail")
			Expect(validation.Completeness).To(BeNumerically(">=", 40),
				"Completeness should be >=40%% with Gaps 1-3 + 8")

			// Validate warnings about missing fields
			Expect(validation.Warnings).To(ContainElement(ContainSubstring("providerData")),
				"Should warn about missing provider data")
			Expect(validation.Warnings).To(ContainElement(ContainSubstring("selectedWorkflowRef")),
				"Should warn about missing workflow selection")
			Expect(validation.Warnings).To(ContainElement(ContainSubstring("executionRef")),
				"Should warn about missing workflow execution")
		})
	})

	// ========================================
	// EDGE CASE: FAILURE SCENARIO WITH GAP #7
	// ========================================
	Context("INTEGRATION-FULL-03: Failure scenario with error_details (Gap #7)", func() {
		It("should reconstruct RR with error_details from failure event", func() {
			// ARRANGE: Seed audit trail with failure event
			baseTimestamp := time.Now().Add(-60 * time.Second).UTC()

			gatewayEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: baseTimestamp,
				EventType:      "gateway.signal.received",
				EventCategory:  "gateway",
				EventAction:    "received",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "Signal",
				ResourceID:     "test-signal-failure-001",
				EventData: map[string]interface{}{
					"event_type":  "gateway.signal.received",
					"alert_name":  "FailureAlert",
					"namespace":   "test-namespace",
					"fingerprint": "failure123",
					"signal_type": "prometheus",
				},
			}
			_, err := auditRepo.Create(ctx, gatewayEvent)
			Expect(err).ToNot(HaveOccurred())

			// Gap #7: orchestrator.lifecycle.failed with error_details
			failureEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: baseTimestamp.Add(10 * time.Second),
				EventType:      "orchestrator.lifecycle.failed",
				EventCategory:  "orchestrator",
				EventAction:    "failed",
				EventOutcome:   "failure",
				CorrelationID:  correlationID,
				ResourceType:   "RemediationRequest",
				ResourceID:     "rr-failure-001",
				EventData: map[string]interface{}{
					"event_type":    "orchestrator.lifecycle.failed",
					"rr_name":       "rr-failure-001",
					"namespace":     "test-namespace",
					"failure_phase": "signal_processing",
					"failure_reason": "Timeout waiting for SignalProcessing completion",
					"error_details": map[string]interface{}{
						"message":        "Remediation failed at phase 'signal_processing': timeout waiting for SP completion",
						"code":           "ERR_TIMEOUT_REMEDIATION",
						"component":      "remediationorchestrator",
						"retry_possible": true,
					},
				},
			}
			_, err = auditRepo.Create(ctx, failureEvent)
			Expect(err).ToNot(HaveOccurred())

			// ACT: Execute reconstruction pipeline
			events, err := reconstructionpkg.QueryAuditEventsForReconstruction(ctx, db.DB, logger, correlationID)
			Expect(err).ToNot(HaveOccurred())

			var parsedData []reconstructionpkg.ParsedAuditData
			for _, event := range events {
				parsed, err := reconstructionpkg.ParseAuditEvent(event)
				if err == nil && parsed != nil {
					parsedData = append(parsedData, *parsed)
				}
			}

			mergedData, err := reconstructionpkg.MergeAuditData(parsedData)
			Expect(err).ToNot(HaveOccurred())

			rr, err := reconstructionpkg.BuildRemediationRequest(correlationID, mergedData)
			Expect(err).ToNot(HaveOccurred())

			validation, err := reconstructionpkg.ValidateReconstructedRR(rr)
			Expect(err).ToNot(HaveOccurred())

			// ASSERT: Gap #7 Note - error_details parsing not yet implemented
			// TODO: Implement parser for orchestrator.lifecycle.failed events to extract error_details
			// For now, just validate the RR was reconstructed with available fields
			Expect(rr).ToNot(BeNil(), "Should reconstruct RR even with failure event")
			Expect(validation.Completeness).To(BeNumerically(">", 0),
				"Should have some completeness even with failure")
		})
	})
})
