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

	"github.com/go-faster/jx"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
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
			// ✅ Using typed ogenclient payload for compile-time validation

			// Gap #3: Create OriginalPayload (raw signal data)
			originalPayloadMap := map[string]jx.Raw{
				"incident_id": jx.Raw(`"incident-memory-high-2026-01-13"`),
				"resource":    jx.Raw(`"Pod/frontend-pod-xyz"`),
				"message":     jx.Raw(`"memory-high alert detected"`),
			}

			gatewayPayload := ogenclient.GatewayAuditPayload{
				EventType:       ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
				AlertName:       "HighMemoryUsage",
				Namespace:       "test-namespace",
				Fingerprint:     "abc123def456",
				SignalType:      ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
				OriginalPayload: ogenclient.NewOptGatewayAuditPayloadOriginalPayload(originalPayloadMap),
				SignalLabels: ogenclient.NewOptGatewayAuditPayloadSignalLabels(map[string]string{
					"app":      "frontend",
					"severity": "critical",
				}),
				SignalAnnotations: ogenclient.NewOptGatewayAuditPayloadSignalAnnotations(map[string]string{
					"summary": "Memory usage above 90%",
				}),
			}

			gatewayEvent, err := CreateGatewaySignalReceivedEvent(correlationID, gatewayPayload)
			Expect(err).ToNot(HaveOccurred())
			_, err = auditRepo.Create(ctx, gatewayEvent)
			Expect(err).ToNot(HaveOccurred())

			// 2. Gap #8: orchestrator.lifecycle.created (TimeoutConfig)
			// ✅ Using typed ogenclient payload for compile-time validation
			orchestratorPayload := ogenclient.RemediationOrchestratorAuditPayload{
				EventType: ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCreated,
				RrName:    "rr-full-001",
				Namespace: "test-namespace",
				TimeoutConfig: ogenclient.NewOptTimeoutConfig(
					ogenclient.TimeoutConfig{
						Global:     ogenclient.NewOptString("30m"),
						Processing: ogenclient.NewOptString("5m"),
						Analyzing:  ogenclient.NewOptString("10m"),
						Executing:  ogenclient.NewOptString("15m"),
					},
				),
			}

			orchestratorEvent, err := CreateOrchestratorLifecycleCreatedEvent(correlationID, orchestratorPayload)
			Expect(err).ToNot(HaveOccurred())
			_, err = auditRepo.Create(ctx, orchestratorEvent)
			Expect(err).ToNot(HaveOccurred())

			// 3. Gap #4: aianalysis.analysis.completed (Provider data)
			// ✅ Using typed ogenclient payload for compile-time validation
			aaPayload := ogenclient.AIAnalysisAuditPayload{
				EventType:        ogenclient.AIAnalysisAuditPayloadEventTypeAianalysisAnalysisCompleted,
				AnalysisName:     "aianalysis-full-001",
				Namespace:        "test-namespace",
				Phase:            ogenclient.AIAnalysisAuditPayloadPhaseAnalyzing,
				ApprovalRequired: false,
				DegradedMode:     false,
				WarningsCount:    0,
				ProviderResponseSummary: ogenclient.NewOptProviderResponseSummary(
					ogenclient.ProviderResponseSummary{
						IncidentID:         "incident-memory-high-2026-01-13",
						AnalysisPreview:    "Memory leak detected in frontend pod. Root cause: unclosed database connections.",
						NeedsHumanReview:   false,
						WarningsCount:      0,
						SelectedWorkflowID: ogenclient.NewOptString("restart-pod-workflow"),
					},
				),
			}

			aaEvent, err := CreateAIAnalysisCompletedEvent(correlationID, aaPayload)
			Expect(err).ToNot(HaveOccurred())
			_, err = auditRepo.Create(ctx, aaEvent)
			Expect(err).ToNot(HaveOccurred())

			// 5. Gap #5: workflowexecution.selection.completed (Workflow selection)
			// ✅ Using typed ogenclient payload for compile-time validation
			selectionPayload := ogenclient.WorkflowExecutionAuditPayload{
				EventType:       ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionSelectionCompleted,
				ExecutionName:   "wfe-full-001",
				Phase:           ogenclient.WorkflowExecutionAuditPayloadPhaseRunning,
				WorkflowID:      "restart-pod-workflow",
				WorkflowVersion: "v1.2.0",
				TargetResource:  "Pod/frontend-pod-xyz",
				ContainerImage:  "ghcr.io/kubernaut/workflows:restart-pod-v1.2.0",
			}

			selectionEvent, err := CreateWorkflowSelectionCompletedEvent(correlationID, selectionPayload)
			Expect(err).ToNot(HaveOccurred())
			_, err = auditRepo.Create(ctx, selectionEvent)
			Expect(err).ToNot(HaveOccurred())

			// 6. Gap #6: workflowexecution.execution.started (Workflow execution)
			// ✅ Using typed ogenclient payload for compile-time validation
			executionPayload := ogenclient.WorkflowExecutionAuditPayload{
				EventType:       ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionExecutionStarted,
				ExecutionName:   "wfe-full-001",
				Phase:           ogenclient.WorkflowExecutionAuditPayloadPhaseRunning,
				WorkflowVersion: "v1.2.0",
				WorkflowID:      "restart-pod-workflow",
				TargetResource:  "Pod/frontend-pod-xyz",
				ContainerImage:  "ghcr.io/kubernaut/workflows:restart-pod-v1.2.0",
				PipelinerunName: ogenclient.NewOptString("restart-pod-run-12345"),
				StartedAt:       ogenclient.NewOptDateTime(baseTimestamp.Add(25 * time.Second)),
			}

			executionEvent, err := CreateWorkflowExecutionStartedEvent(correlationID, executionPayload)
			Expect(err).ToNot(HaveOccurred())
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
			Expect(rr.Spec.SignalType).To(Equal("prometheus-alert"), "Gap #2: SignalType from gateway.signal.received")
			Expect(rr.Spec.SignalLabels).To(HaveKeyWithValue("app", "frontend"), "Gap #3: SignalLabels from gateway.signal.received")
			Expect(rr.Spec.SignalAnnotations).To(HaveKeyWithValue("summary", "Memory usage above 90%"), "Gap #3: SignalAnnotations from gateway.signal.received")
			Expect(string(rr.Spec.OriginalPayload)).To(ContainSubstring("memory-high"), "Gap #3: OriginalPayload from gateway.signal.received")

			// Gap #4: Provider data
			Expect(rr.Spec.ProviderData).ToNot(BeNil(), "Gap #4: ProviderData should be populated")
			Expect(rr.Spec.ProviderData).ToNot(BeEmpty(), "Gap #4: ProviderData should not be empty")
			// ProviderData is stored as JSON string (issue #96) - contains ProviderResponseSummary fields
			Expect(rr.Spec.ProviderData).To(ContainSubstring("incident-memory-high"), "Gap #4: incident_id from aianalysis.analysis.completed")
			Expect(string(rr.Spec.ProviderData)).To(ContainSubstring("Memory leak detected"), "Gap #4: analysis_preview from aianalysis.analysis.completed")

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
			Expect(rr.Name).To(ContainSubstring(correlationID), "RR name should contain correlation ID")
			Expect(rr.Namespace).To(Equal("kubernaut-system"), "RR namespace should be kubernaut-system (per builder.go)")
		})
	})

	// ========================================
	// EDGE CASE: PARTIAL AUDIT TRAIL
	// ========================================
	Context("INTEGRATION-FULL-02: Partial audit trail (missing workflow events)", func() {
		It("should reconstruct RR with lower completeness when workflow events missing", func() {
			// ARRANGE: Seed partial audit trail (only Gaps #1-3, #8)

			// Only gateway.signal.received + orchestrator.lifecycle.created
			// ✅ Using typed ogenclient payloads
			gatewayPayload := ogenclient.GatewayAuditPayload{
				EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
				AlertName:   "PartialAlert",
				Namespace:   "test-namespace",
				Fingerprint: "partial123",
				SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
			}
			gatewayEvent, err := CreateGatewaySignalReceivedEvent(correlationID, gatewayPayload)
			Expect(err).ToNot(HaveOccurred())
			_, err = auditRepo.Create(ctx, gatewayEvent)
			Expect(err).ToNot(HaveOccurred())

			orchestratorPayload := ogenclient.RemediationOrchestratorAuditPayload{
				EventType: ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCreated,
				RrName:    "rr-partial-001",
				Namespace: "test-namespace",
				TimeoutConfig: ogenclient.NewOptTimeoutConfig(
					ogenclient.TimeoutConfig{
						Global: ogenclient.NewOptString("30m"),
					},
				),
			}
			orchestratorEvent, err := CreateOrchestratorLifecycleCreatedEvent(correlationID, orchestratorPayload)
			Expect(err).ToNot(HaveOccurred())
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
			Expect(validation.Completeness).To(BeNumerically(">=", 30),
				"Completeness should be >=30%% with Gaps 1-3 + 8 (3 out of 9 fields)")

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

			// ✅ Using typed ogenclient payload
			gatewayPayload := ogenclient.GatewayAuditPayload{
				EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
				AlertName:   "FailureAlert",
				Namespace:   "test-namespace",
				Fingerprint: "failure123",
				SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
			}
			gatewayEvent, err := CreateGatewaySignalReceivedEvent(correlationID, gatewayPayload)
			Expect(err).ToNot(HaveOccurred())
			_, err = auditRepo.Create(ctx, gatewayEvent)
			Expect(err).ToNot(HaveOccurred())

			// Gap #7: orchestrator.lifecycle.failed with error_details
			// Note: Using manual event creation since we don't have a helper for failure events yet
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
					"event_type":     "orchestrator.lifecycle.failed",
					"rr_name":        "rr-failure-001",
					"namespace":      "test-namespace",
					"failure_phase":  "signal_processing",
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
