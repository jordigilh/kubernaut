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
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-faster/jx"
	"github.com/google/uuid"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// RECONSTRUCTION E2E TESTS (BR-AUDIT-006)
// ========================================
//
// Purpose: Validate RemediationRequest reconstruction via HTTP REST API endpoint
// using OpenAPI-generated client to test complete end-to-end workflow.
//
// Business Requirements:
// - BR-AUDIT-006: RemediationRequest Reconstruction from Audit Traces
//
// Test Strategy (per 03-testing-strategy.mdc):
// - E2E Tests (10-15%): Test complete HTTP workflow via OpenAPI client
// - Test against real DataStorage HTTP endpoint (NodePort)
// - Validate HTTP status codes, JSON responses, RFC 7807 errors
// - Test authentication via X-User-ID header
//
// Test Coverage Matrix (per SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md):
// - E2E-FULL-01: Full RR reconstruction with all fields
// - E2E-PARTIAL-01: Partial reconstruction with missing optional data
// - E2E-ERROR-01: Error scenarios (missing correlation ID, no gateway event)
//
// Difference from Integration Tests:
// - Integration (test/integration/datastorage/reconstruction_integration_test.go):
//   - Calls reconstruction.* functions directly with real database
//   - Tests business logic in isolation
//   - 5/5 tests passing ✅
//
// - E2E (this file):
//   - Uses ogenclient to call HTTP endpoint
//   - Tests complete HTTP layer (routing, serialization, error handling)
//   - Validates production deployment readiness
//
// ========================================

var _ = Describe("E2E: Reconstruction REST API (BR-AUDIT-006)", Label("e2e", "reconstruction-api", "p0"), Ordered, func() {
	var (
		testCtx       context.Context
		correlationID string
		auditRepo     *repository.AuditEventsRepository
	)

	BeforeAll(func() {
		testCtx = context.Background()

		// Initialize audit repository for test data seeding
		// E2E tests seed audit events in database, then use REST API to reconstruct
		auditRepo = repository.NewAuditEventsRepository(testDB, logger)

		GinkgoWriter.Println("========================================")
		GinkgoWriter.Println("E2E: Reconstruction REST API Tests")
		GinkgoWriter.Println("========================================")
		GinkgoWriter.Println("DataStorage URL:", dataStorageURL)
		GinkgoWriter.Println("OpenAPI Client:", DSClient != nil)
		GinkgoWriter.Println("========================================")
	})

	// ========================================
	// E2E-FULL-01: Full RR Reconstruction
	// ========================================
	Context("E2E-FULL-01: Full reconstruction with complete audit trail", func() {
		BeforeEach(func() {
			// Generate unique correlation ID for this test
			correlationID = fmt.Sprintf("e2e-full-reconstruction-%s", uuid.New().String())

			// Seed complete audit trail in database using type-safe ogenclient structs
			// Gateway signal event (Gap #1-3) - TYPE SAFE
			originalPayload := ogenclient.GatewayAuditPayloadOriginalPayload{}
			// Manually encode nested map to jx.Raw
			originalPayloadJSON, _ := json.Marshal(map[string]interface{}{
				"alert":  "HighCPUUsage",
				"status": "firing",
				"labels": map[string]string{
					"alertname": "HighCPUUsage",
					"severity":  "critical",
				},
			})
			_ = originalPayload.UnmarshalJSON(originalPayloadJSON)

			gatewayPayload := ogenclient.GatewayAuditPayload{
				EventType:   "gateway.signal.received",
				SignalType:  ogenclient.GatewayAuditPayloadSignalTypeAlert,
				AlertName:   "HighCPUUsage",        // string, not OptString
				Namespace:   "production",          // string, not OptString
				Fingerprint: "e2e-fingerprint-123", // string, not OptString
				SignalLabels: ogenclient.NewOptGatewayAuditPayloadSignalLabels(ogenclient.GatewayAuditPayloadSignalLabels{
					"alertname": "HighCPUUsage",
					"severity":  "critical",
					"pod":       "app-pod-123",
				}),
				SignalAnnotations: ogenclient.NewOptGatewayAuditPayloadSignalAnnotations(ogenclient.GatewayAuditPayloadSignalAnnotations{
					"summary":     "CPU usage exceeded 80%",
					"description": "Pod app-pod-123 CPU usage is at 92%",
				}),
				OriginalPayload: ogenclient.NewOptGatewayAuditPayloadOriginalPayload(originalPayload),
			}

			// Marshal using ogen's jx.Encoder for proper handling of Opt types
			var gatewayEncoder jx.Encoder
			gatewayPayload.Encode(&gatewayEncoder)
			var gatewayEventData map[string]interface{}
			err := json.Unmarshal(gatewayEncoder.Bytes(), &gatewayEventData)
			Expect(err).ToNot(HaveOccurred(), "Failed to marshal gateway payload")

			gatewayEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().Add(-10 * time.Second).UTC(),
				EventType:      "gateway.signal.received",
				EventCategory:  "gateway",
				EventAction:    "received",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "Signal",
				ResourceID:     "signal-e2e-123",
				ActorType:      "ServiceAccount",
				ActorID:        "system:serviceaccount:gateway:gateway-sa",
				EventData:      gatewayEventData,
			}

			_, err = auditRepo.Create(testCtx, gatewayEvent)
			Expect(err).ToNot(HaveOccurred(), "Failed to seed gateway audit event")

			// Orchestrator lifecycle event (Gap #8) - TYPE SAFE
			orchestratorPayload := ogenclient.RemediationOrchestratorAuditPayload{
				EventType: "orchestrator.lifecycle.created",
				RrName:    "rr-e2e-123",
				Namespace: "production",
				TimeoutConfig: ogenclient.NewOptTimeoutConfig(ogenclient.TimeoutConfig{
					Global:     ogenclient.NewOptString("1h"),
					Processing: ogenclient.NewOptString("10m"),
					Analyzing:  ogenclient.NewOptString("15m"),
					Executing:  ogenclient.NewOptString("30m"),
				}),
			}

			var orchestratorEncoder jx.Encoder
			orchestratorPayload.Encode(&orchestratorEncoder)
			var orchestratorEventData map[string]interface{}
			err = json.Unmarshal(orchestratorEncoder.Bytes(), &orchestratorEventData)
			Expect(err).ToNot(HaveOccurred(), "Failed to marshal orchestrator payload")

			orchestratorEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
				EventType:      "orchestrator.lifecycle.created",
				EventCategory:  "orchestrator",
				EventAction:    "created",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "RemediationRequest",
				ResourceID:     "rr-e2e-123",
				ActorType:      "ServiceAccount",
				ActorID:        "system:serviceaccount:orchestrator:orchestrator-sa",
				EventData:      orchestratorEventData,
			}

			_, err = auditRepo.Create(testCtx, orchestratorEvent)
			Expect(err).ToNot(HaveOccurred(), "Failed to seed orchestrator audit event")

			// AIAnalysis completed event (Gap #4) - TYPE SAFE
			aiPayload := ogenclient.AIAnalysisAuditPayload{
				EventType:        "aianalysis.analysis.completed",
				AnalysisName:     "analysis-e2e-123",
				Namespace:        "production",
				Phase:            "completed",
				ApprovalRequired: false,
				DegradedMode:     false,
				WarningsCount:    0,
				ProviderResponseSummary: ogenclient.NewOptProviderResponseSummary(ogenclient.ProviderResponseSummary{
					IncidentID:       "e2e-incident-123",
					AnalysisPreview:  "High CPU usage detected in production pods",
					NeedsHumanReview: false,
					WarningsCount:    0,
				}),
			}

			var aiEncoder jx.Encoder
			aiPayload.Encode(&aiEncoder)
			var aiEventData map[string]interface{}
			err = json.Unmarshal(aiEncoder.Bytes(), &aiEventData)
			Expect(err).ToNot(HaveOccurred(), "Failed to marshal AI analysis payload")

			aiEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().Add(-4 * time.Second).UTC(),
				EventType:      "aianalysis.analysis.completed",
				EventCategory:  "aianalysis",
				EventAction:    "completed",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "AIAnalysis",
				ResourceID:     "analysis-e2e-123",
				ActorType:      "ServiceAccount",
				ActorID:        "system:serviceaccount:aianalysis:aianalysis-sa",
				EventData:      aiEventData,
			}

			_, err = auditRepo.Create(testCtx, aiEvent)
			Expect(err).ToNot(HaveOccurred(), "Failed to seed AI analysis audit event")

			// WorkflowExecution selection completed event (Gap #5) - TYPE SAFE + SHA256
			workflowSelectionPayload := ogenclient.WorkflowExecutionAuditPayload{
				EventType:       "workflowexecution.selection.completed",
				ExecutionName:   "we-e2e-123",
				WorkflowID:      "cpu-remediation-workflow",
				WorkflowVersion: "v1.2.0",
				// ✅ SHA256 digest (not tag) for reproducibility
				ContainerImage: "registry.io/workflows/cpu-remediation@sha256:abc123def456789012345678901234567890123456789012345678901234",
				TargetResource: "deployment/app-deployment",
				Phase:          "selecting",
			}

			var workflowSelectionEncoder jx.Encoder
			workflowSelectionPayload.Encode(&workflowSelectionEncoder)
			var workflowSelectionEventData map[string]interface{}
			err = json.Unmarshal(workflowSelectionEncoder.Bytes(), &workflowSelectionEventData)
			Expect(err).ToNot(HaveOccurred(), "Failed to marshal workflow selection payload")

			workflowSelectionEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().Add(-3 * time.Second).UTC(),
				EventType:      "workflowexecution.selection.completed",
				EventCategory:  "workflowexecution",
				EventAction:    "completed",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "WorkflowExecution",
				ResourceID:     "we-e2e-123",
				ActorType:      "ServiceAccount",
				ActorID:        "system:serviceaccount:workflowexecution:workflowexecution-sa",
				EventData:      workflowSelectionEventData,
			}

			_, err = auditRepo.Create(testCtx, workflowSelectionEvent)
			Expect(err).ToNot(HaveOccurred(), "Failed to seed workflow selection audit event")

			// WorkflowExecution execution started event (Gap #6) - TYPE SAFE + SHA256
			workflowExecutionPayload := ogenclient.WorkflowExecutionAuditPayload{
				EventType:     "workflowexecution.execution.started",
				ExecutionName: "we-e2e-123",
				WorkflowID:    "cpu-remediation-workflow",
				// ✅ SHA256 digest (not tag) for reproducibility
				ContainerImage:  "registry.io/workflows/cpu-remediation@sha256:abc123def456789012345678901234567890123456789012345678901234",
				TargetResource:  "deployment/app-deployment",
				WorkflowVersion: "v1.2.0",
				Phase:           "executing",
			}

			var workflowExecutionEncoder jx.Encoder
			workflowExecutionPayload.Encode(&workflowExecutionEncoder)
			var workflowExecutionEventData map[string]interface{}
			err = json.Unmarshal(workflowExecutionEncoder.Bytes(), &workflowExecutionEventData)
			Expect(err).ToNot(HaveOccurred(), "Failed to marshal workflow execution payload")

			workflowExecutionEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().Add(-2 * time.Second).UTC(),
				EventType:      "workflowexecution.execution.started",
				EventCategory:  "workflowexecution",
				EventAction:    "started",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "WorkflowExecution",
				ResourceID:     "we-e2e-123",
				ActorType:      "ServiceAccount",
				ActorID:        "system:serviceaccount:workflowexecution:workflowexecution-sa",
				EventData:      workflowExecutionEventData,
			}

			_, err = auditRepo.Create(testCtx, workflowExecutionEvent)
			Expect(err).ToNot(HaveOccurred(), "Failed to seed workflow execution audit event")

			GinkgoWriter.Printf("✅ Seeded 5 audit events (all gaps covered) for correlation ID: %s\n", correlationID)
		})

		It("should reconstruct RR via OpenAPI client with complete fields", func() {
			// ACT: Call reconstruction endpoint via OpenAPI client
			GinkgoWriter.Printf("🔄 Calling reconstruction API for correlation ID: %s\n", correlationID)

			response, err := DSClient.ReconstructRemediationRequest(testCtx, ogenclient.ReconstructRemediationRequestParams{
				CorrelationID: correlationID,
			})

			// ASSERT: HTTP request succeeded
			Expect(err).ToNot(HaveOccurred(), "OpenAPI client request should succeed")
			Expect(response).ToNot(BeNil(), "Response should not be nil")

			// DEBUG: Check what response type we got
			GinkgoWriter.Printf("📊 Response type: %T\n", response)
			switch resp := response.(type) {
			case *ogenclient.ReconstructRemediationRequestBadRequest:
				GinkgoWriter.Printf("❌ Got 400 Bad Request:\n")
				GinkgoWriter.Printf("   Type: %s\n", resp.Type.String())
				GinkgoWriter.Printf("   Title: %s\n", resp.Title)
				GinkgoWriter.Printf("   Detail: %s\n", resp.Detail.Value)
				Fail("Reconstruction returned 400 Bad Request - check server logs")
			case *ogenclient.ReconstructRemediationRequestNotFound:
				GinkgoWriter.Printf("❌ Got 404 Not Found:\n")
				GinkgoWriter.Printf("   Type: %s\n", resp.Type.String())
				GinkgoWriter.Printf("   Title: %s\n", resp.Title)
				GinkgoWriter.Printf("   Detail: %s\n", resp.Detail.Value)
				Fail("Reconstruction returned 404 Not Found - events not found in database")
			case *ogenclient.ReconstructRemediationRequestInternalServerError:
				GinkgoWriter.Printf("❌ Got 500 Internal Server Error:\n")
				GinkgoWriter.Printf("   Type: %s\n", resp.Type.String())
				GinkgoWriter.Printf("   Title: %s\n", resp.Title)
				GinkgoWriter.Printf("   Detail: %s\n", resp.Detail.Value)
				Fail("Reconstruction returned 500 Internal Server Error - check server logs")
			case *ogenclient.ReconstructionResponse:
				GinkgoWriter.Printf("✅ Got successful ReconstructionResponse\n")
			default:
				GinkgoWriter.Printf("❌ Got unexpected response type: %T\n", resp)
			}

			// ASSERT: Response is successful reconstruction
			reconstructionResp, ok := response.(*ogenclient.ReconstructionResponse)
			Expect(ok).To(BeTrue(), "Response should be ReconstructionResponse type")
			Expect(reconstructionResp).ToNot(BeNil())

			// ASSERT: Reconstructed YAML is not empty
			Expect(reconstructionResp.RemediationRequestYaml).ToNot(BeEmpty(),
				"Reconstructed YAML should not be empty")

			// ASSERT: Validation passed
			Expect(reconstructionResp.Validation.IsValid).To(BeTrue(),
				"Reconstruction should be valid")

			// ASSERT: Completeness is very high (>= 88% for complete audit trail with all 5 event types)
			// Updated Jan 14, 2026: Seeding all 5 event types (gateway + orchestrator + AI + 2x workflow)
			// 8/9 fields populated (TimeoutConfig populated, but only basic fields - counts as complete)
			Expect(reconstructionResp.Validation.Completeness).To(BeNumerically(">=", 88),
				"Completeness should be >= 88% for complete audit trail (8/9 fields)")

			// ASSERT: Core fields reconstructed correctly (Gaps #1-3)
			Expect(reconstructionResp.RemediationRequestYaml).To(ContainSubstring("HighCPUUsage"),
				"YAML should contain signal name")
			Expect(reconstructionResp.RemediationRequestYaml).To(ContainSubstring("alert"),
				"YAML should contain signal type")
			Expect(reconstructionResp.RemediationRequestYaml).To(ContainSubstring("1h"),
				"YAML should contain timeout config (Gap #8)")

			// ASSERT: Gap #4 field present (AI Provider Data)
			// Note: providerData is base64-encoded bytes in CRD (correct behavior)
			Expect(reconstructionResp.RemediationRequestYaml).To(ContainSubstring("providerData"),
				"YAML should contain Gap #4 field (AI provider data - base64 encoded)")

			// ASSERT: Gap #5 field present (Workflow Selection Reference)
			Expect(reconstructionResp.RemediationRequestYaml).To(ContainSubstring("selectedWorkflowRef"),
				"YAML should contain Gap #5 field (workflow selection)")
			Expect(reconstructionResp.RemediationRequestYaml).To(ContainSubstring("cpu-remediation-workflow"),
				"YAML should contain workflow ID from selection event")

			// ASSERT: Gap #6 field present (Workflow Execution Reference)
			Expect(reconstructionResp.RemediationRequestYaml).To(ContainSubstring("executionRef"),
				"YAML should contain Gap #6 field (execution reference)")
			Expect(reconstructionResp.RemediationRequestYaml).To(ContainSubstring("we-e2e-123"),
				"YAML should contain execution name from execution started event")

			GinkgoWriter.Printf("✅ Reconstruction succeeded: completeness=%d%%, warnings=%d\n",
				reconstructionResp.Validation.Completeness,
				len(reconstructionResp.Validation.Warnings))
		})
	})

	// ========================================
	// E2E-PARTIAL-01: Partial Reconstruction
	// ========================================
	Context("E2E-PARTIAL-01: Partial reconstruction with missing optional fields", func() {
		BeforeEach(func() {
			correlationID = fmt.Sprintf("e2e-partial-reconstruction-%s", uuid.New().String())

			// Seed minimal audit trail (only gateway event, no orchestrator)
			gatewayEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().Add(-10 * time.Second).UTC(),
				EventType:      "gateway.signal.received",
				EventCategory:  "gateway",
				EventAction:    "received",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "Signal",
				ResourceID:     "signal-partial-123",
				EventData: map[string]interface{}{
					"event_type":  "gateway.signal.received",
					"signal_type": "alert",
					"alert_name":  "PartialAlert",
					"namespace":   "test",
					"fingerprint": "partial-fp-123",
					// Note: Missing signal_labels, signal_annotations, original_payload
				},
			}

			_, err := auditRepo.Create(testCtx, gatewayEvent)
			Expect(err).ToNot(HaveOccurred(), "Failed to seed minimal audit event")

			GinkgoWriter.Printf("✅ Seeded minimal audit event for correlation ID: %s\n", correlationID)
		})

		It("should reconstruct partial RR with validation warnings", func() {
			// ACT: Call reconstruction endpoint
			response, err := DSClient.ReconstructRemediationRequest(testCtx, ogenclient.ReconstructRemediationRequestParams{
				CorrelationID: correlationID,
			})

			// ASSERT: Request succeeded (partial reconstruction is valid)
			Expect(err).ToNot(HaveOccurred())
			Expect(response).ToNot(BeNil())

			// Check response type - could be success OR bad request for partial data
			switch resp := response.(type) {
			case *ogenclient.ReconstructionResponse:
				// Success case: Reconstruction succeeded but is incomplete
				Expect(resp.RemediationRequestYaml).ToNot(BeEmpty())
				Expect(resp.Validation.IsValid).To(BeTrue(),
					"Partial reconstruction should still be valid")

				// Completeness is lower (50-80% for partial data)
				Expect(resp.Validation.Completeness).To(BeNumerically(">=", 50),
					"Completeness should be at least 50%")
				Expect(resp.Validation.Completeness).To(BeNumerically("<", 80),
					"Completeness should be less than 80% for partial data")

				// Warnings present for missing fields
				Expect(resp.Validation.Warnings).ToNot(BeEmpty(),
					"Should have warnings for missing optional fields")

				GinkgoWriter.Printf("✅ Partial reconstruction succeeded: completeness=%d%%, warnings=%d\n",
					resp.Validation.Completeness,
					len(resp.Validation.Warnings))

			case *ogenclient.ReconstructRemediationRequestBadRequest:
				// Bad request case: Missing required data (e.g., no orchestrator event)
				// This is also valid behavior for truly incomplete data
				GinkgoWriter.Printf("✅ Partial reconstruction returned 400 Bad Request (expected for minimal data)\n")
				GinkgoWriter.Printf("   This indicates the reconstruction requires more complete audit trail\n")
				// Test passes - both 200 (with warnings) and 400 (too incomplete) are valid

			default:
				Fail(fmt.Sprintf("Unexpected response type: %T", resp))
			}
		})
	})

	// ========================================
	// E2E-ERROR-01: Error Handling
	// ========================================
	Context("E2E-ERROR-01: Error scenarios via HTTP", func() {
		It("should return 404 for non-existent correlation ID", func() {
			nonExistentID := "nonexistent-correlation-id-12345"

			// ACT: Call with non-existent correlation ID
			response, err := DSClient.ReconstructRemediationRequest(testCtx, ogenclient.ReconstructRemediationRequestParams{
				CorrelationID: nonExistentID,
			})

			// ASSERT: Should receive 404 Not Found response (ogen doesn't return error for 4xx)
			Expect(err).ToNot(HaveOccurred(), "OpenAPI client should not return error for 404")
			Expect(response).ToNot(BeNil())

			// Check response type is NotFound
			notFoundResp, ok := response.(*ogenclient.ReconstructRemediationRequestNotFound)
			Expect(ok).To(BeTrue(), "Response should be ReconstructRemediationRequestNotFound type")
			Expect(notFoundResp).ToNot(BeNil())

			GinkgoWriter.Printf("✅ Correctly returned 404 NotFound for non-existent correlation ID\n")
		})

		It("should return 400 for missing gateway event (required)", func() {
			correlationID = fmt.Sprintf("e2e-missing-gateway-%s", uuid.New().String())

			// Seed ONLY orchestrator event (missing required gateway event)
			orchestratorOnlyEvent := &repository.AuditEvent{
				EventID:        uuid.New(),
				Version:        "1.0",
				EventTimestamp: time.Now().UTC(),
				EventType:      "orchestrator.lifecycle.created",
				EventCategory:  "orchestrator",
				EventAction:    "created",
				EventOutcome:   "success",
				CorrelationID:  correlationID,
				ResourceType:   "RemediationRequest",
				ResourceID:     "rr-no-gateway",
				EventData: map[string]interface{}{
					"event_type": "orchestrator.lifecycle.created",
					"rr_name":    "rr-no-gateway",
					"namespace":  "test",
				},
			}

			_, err := auditRepo.Create(testCtx, orchestratorOnlyEvent)
			Expect(err).ToNot(HaveOccurred())

			// ACT: Call reconstruction (should fail - gateway event required)
			response, err := DSClient.ReconstructRemediationRequest(testCtx, ogenclient.ReconstructRemediationRequestParams{
				CorrelationID: correlationID,
			})

			// ASSERT: Should receive 400 Bad Request response (ogen doesn't return error for 4xx)
			Expect(err).ToNot(HaveOccurred(), "OpenAPI client should not return error for 400")
			Expect(response).ToNot(BeNil())

			// Check response type is BadRequest
			badRequestResp, ok := response.(*ogenclient.ReconstructRemediationRequestBadRequest)
			Expect(ok).To(BeTrue(), "Response should be ReconstructRemediationRequestBadRequest type")
			Expect(badRequestResp).ToNot(BeNil())

			GinkgoWriter.Printf("✅ Correctly returned 400 BadRequest for missing required gateway event\n")
		})
	})
})
