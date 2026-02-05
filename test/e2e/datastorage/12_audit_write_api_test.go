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
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// AUDIT EVENTS WRITE API E2E TESTS
// 📋 Tests Define Contract: OpenAPI spec audit-write-api.openapi.yaml
// Authority: DAY21_PHASE1_IMPLEMENTATION_PLAN.md Phase 3
// ========================================
//
// This file tests the complete audit write API flow end-to-end.
//
// Business Requirements:
// - BR-STORAGE-033: Generic audit write API
// - BR-STORAGE-032: Unified audit trail
//
// OpenAPI Compliance:
// - Endpoint: POST /api/v1/audit/events
// - Request Body: Strongly-typed event_data using ogen discriminated unions
// - Response: 201 Created with event_id (UUID)
// - Errors: 400 Bad Request, 500 Internal Server Error (RFC 7807)
//
// ========================================

var _ = Describe("Audit Events Write API E2E Tests", Label("e2e", "audit-write-api"), func() {
	var (
		testCorrelationID string
		testDB            *sql.DB
		serviceURL        string
	)

	BeforeEach(func() {
		// Use shared DataStorage deployment
		serviceURL = dataStorageURL

		// Ensure service is ready before each test
		Eventually(func() bool {
			_, err := DSClient.HealthCheck(ctx)
			return err == nil
		}, "10s", "500ms").Should(BeTrue(), "Data Storage Service should be ready")

		// Connect to PostgreSQL for verification
		connStr := fmt.Sprintf("host=localhost port=25433 user=slm_user password=test_password dbname=action_history sslmode=disable")
		var err error
		testDB, err = sql.Open("pgx", connStr)
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() error {
			return testDB.Ping()
		}, "10s", "1s").Should(Succeed())

		// Generate unique correlation ID for test isolation
		testCorrelationID = generateTestID()

		// Clean up test data before each test
		_, err = testDB.Exec("DELETE FROM audit_events WHERE correlation_id = $1", testCorrelationID)
		if err != nil {
			GinkgoWriter.Printf("Note: cleanup skipped: %v\n", err)
		}
	})

	AfterEach(func() {
		if testDB != nil {
			_ = testDB.Close()
		}
	})

	Context("BR-STORAGE-033: Generic Audit Write API", func() {
		When("Gateway service writes a signal received event", func() {
			It("should create audit event and return 201 Created", func() {
				// TDD GREEN: Handler now uses JSON body instead of headers

				By("Building Gateway event data using ogen discriminated union types")
				ctx := context.Background()
				client, err := createOpenAPIClient(serviceURL)
				Expect(err).ToNot(HaveOccurred())

				eventData := ogenclient.AuditEventRequestEventData{
					Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
					GatewayAuditPayload: ogenclient.GatewayAuditPayload{
						EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
						SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
						AlertName:   "PodOOMKilled",
						Namespace:   "production",
						Fingerprint: "sha256:abc123",
					},
				}

				By("Sending POST request using ogen client")
				eventRequest := ogenclient.AuditEventRequest{
					Version:        "1.0",
					EventCategory:  ogenclient.AuditEventRequestEventCategoryGateway,
					EventType:      "gateway.signal.received",
					EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
					CorrelationID:  testCorrelationID,
					ResourceType:   ogenclient.NewOptString("pod"),
					ResourceID:     ogenclient.NewOptString("api-server-xyz-123"),
					Namespace:      ogenclient.NewOptNilString("production"),
					EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
					EventAction:    "signal_received",
					Severity:       ogenclient.NewOptNilString("critical"),
					EventData:      eventData,
				}

				By("Verifying 201 Created response")
				eventID, err := postAuditEvent(ctx, client, eventRequest)
				Expect(err).ToNot(HaveOccurred())
				Expect(eventID).ToNot(BeEmpty())

				By("Verifying event_id is a valid UUID")
				Expect(eventID).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`))

				By("Verifying audit event was inserted into database")
				// Handle async HTTP API processing
				Eventually(func() int {
					var count int
					err := testDB.QueryRow("SELECT COUNT(*) FROM audit_events WHERE event_id = $1", eventID).Scan(&count)
					if err != nil {
						return -1
					}
					return count
				}, 5*time.Second, 100*time.Millisecond).Should(Equal(1))

				// ✅ CORRECTNESS TEST: Data in database matches input exactly (BR-STORAGE-033)
				// Schema per ADR-034: event_type, event_category, event_action, event_outcome, actor_id, actor_type
				By("Verifying audit event content matches sent payload (CORRECTNESS)")
				var dbEventType, dbEventCategory, dbEventAction, dbEventOutcome string
				var dbActorID, dbActorType, dbCorrelationID, dbResourceType, dbResourceID string
				var dbEventData []byte

				row := testDB.QueryRow(`
					SELECT event_type, event_category, event_action, event_outcome,
					       actor_id, actor_type, correlation_id, resource_type, resource_id, event_data
					FROM audit_events
					WHERE event_id = $1
				`, eventID)

				err = row.Scan(&dbEventType, &dbEventCategory, &dbEventAction, &dbEventOutcome,
					&dbActorID, &dbActorType, &dbCorrelationID, &dbResourceType, &dbResourceID, &dbEventData)
				Expect(err).ToNot(HaveOccurred(), "Should retrieve audit event from database")

				// Verify each field matches what was sent (ADR-034 schema)
				Expect(dbEventType).To(Equal("gateway.signal.received"), "event_type should match")
				Expect(dbEventCategory).To(Equal("gateway"), "event_category should match service prefix")
				Expect(dbEventAction).To(Equal("signal_received"), "event_action should match operation")
				Expect(dbEventOutcome).To(Equal("success"), "event_outcome should match")
				Expect(dbActorID).To(Equal("gateway-service"), "actor_id defaults to event_category + '-service'")
				Expect(dbActorType).To(Equal("service"), "actor_type should be 'service'")
				Expect(dbCorrelationID).To(Equal(testCorrelationID), "correlation_id should match")
				Expect(dbResourceType).To(Equal("pod"), "resource_type should match")
				Expect(dbResourceID).To(Equal("api-server-xyz-123"), "resource_id should match")

				// Verify event_data JSONB content is non-empty and valid JSON
				var storedEventData map[string]interface{}
				err = json.Unmarshal(dbEventData, &storedEventData)
				Expect(err).ToNot(HaveOccurred(), "event_data should be valid JSON")
				Expect(storedEventData).ToNot(BeEmpty(), "event_data should not be empty")

				// Verify signal_type is present somewhere in the event_data (nested or flat)
				jsonBytes, _ := json.Marshal(storedEventData)
				Expect(string(jsonBytes)).To(ContainSubstring("prometheus"), "event_data should contain prometheus signal_type")
				Expect(string(jsonBytes)).To(ContainSubstring("PodOOMKilled"), "event_data should contain PodOOMKilled alert_name")
			})
		})

		When("AI Analysis service writes an analysis completed event", func() {
			It("should create audit event with AI-specific data", func() {
				// TDD GREEN: Handler now uses JSON body

				By("Building AI Analysis event data using ogen discriminated union types")
				eventData := ogenclient.AuditEventRequestEventData{
					Type: ogenclient.AuditEventRequestEventDataAianalysisAnalysisCompletedAuditEventRequestEventData,
					AIAnalysisAuditPayload: ogenclient.AIAnalysisAuditPayload{
						EventType:    ogenclient.AIAnalysisAuditPayloadEventTypeAianalysisAnalysisCompleted,
						AnalysisName: "analysis-2025-001",
						Phase:        ogenclient.AIAnalysisAuditPayloadPhaseCompleted,
					},
				}

				By("Sending POST request with ogen-typed request")
				eventRequest := ogenclient.AuditEventRequest{
					Version:        "1.0",
					EventCategory:  ogenclient.AuditEventRequestEventCategoryAnalysis,
					EventType:      "aianalysis.analysis.completed",
					EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
					CorrelationID:  testCorrelationID,
					EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
					EventAction:    "analysis",
					EventData:      eventData,
				}

				// Use ogen client to post event (handles optional fields properly)
				ctx := context.Background()
				client, err := createOpenAPIClient(serviceURL)
				Expect(err).ToNot(HaveOccurred())

				eventID, err := postAuditEvent(ctx, client, eventRequest)
				Expect(err).ToNot(HaveOccurred())
				Expect(eventID).ToNot(BeEmpty())

				// ✅ CORRECTNESS TEST: Verify AI Analysis event stored correctly (BR-STORAGE-033)
				// Schema per ADR-034: event_type, event_category, event_action, event_outcome, actor_id
				By("Verifying AI Analysis audit event content matches sent payload (CORRECTNESS)")
				var dbEventType, dbEventCategory, dbEventAction, dbEventOutcome string
				var dbActorID, dbCorrelationID string
				var dbEventData []byte

				// Handle async HTTP API processing
				Eventually(func() error {
					row := testDB.QueryRow(`
						SELECT event_type, event_category, event_action, event_outcome,
						       actor_id, correlation_id, event_data
						FROM audit_events
						WHERE event_id = $1
					`, eventID)

					return row.Scan(&dbEventType, &dbEventCategory, &dbEventAction, &dbEventOutcome,
						&dbActorID, &dbCorrelationID, &dbEventData)
				}, 5*time.Second, 100*time.Millisecond).Should(Succeed(), "Should retrieve AI Analysis audit event from database")

				Expect(dbEventType).To(Equal("aianalysis.analysis.completed"), "event_type should match")
				Expect(dbEventCategory).To(Equal("analysis"), "event_category should match service prefix")
				Expect(dbEventAction).To(Equal("analysis"), "event_action should match operation")
				Expect(dbEventOutcome).To(Equal("success"), "event_outcome should match")
				Expect(dbCorrelationID).To(Equal(testCorrelationID), "correlation_id should match")

				// Verify AI-specific event_data content is non-empty and valid JSON
				var storedEventData map[string]interface{}
				err = json.Unmarshal(dbEventData, &storedEventData)
				Expect(err).ToNot(HaveOccurred(), "event_data should be valid JSON")
				Expect(storedEventData).ToNot(BeEmpty(), "event_data should not be empty")

				// Verify AI-specific fields are present somewhere in the event_data
				jsonBytes, _ := json.Marshal(storedEventData)
				Expect(string(jsonBytes)).To(ContainSubstring("analysis-2025-001"), "event_data should contain analysis_name")
			})
		})

		When("Workflow service writes a workflow completed event", func() {
			It("should create audit event with workflow-specific data", func() {
				// TDD GREEN: Handler now uses JSON body

				By("Building Workflow event data using ogen discriminated union types")
				eventData := ogenclient.AuditEventRequestEventData{
					Type: ogenclient.AuditEventRequestEventDataWorkflowexecutionWorkflowCompletedAuditEventRequestEventData,
					WorkflowExecutionAuditPayload: ogenclient.WorkflowExecutionAuditPayload{
						EventType:       ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionWorkflowCompleted,
						WorkflowID:      "workflow-increase-memory",
						WorkflowVersion: "v1",
						TargetResource:  "Pod/test-pod",
						Phase:           ogenclient.WorkflowExecutionAuditPayloadPhaseCompleted,
						ContainerImage:  "gcr.io/tekton-releases/tekton:v0.1.0",
						ExecutionName:   "exec-increase-memory-001",
					},
				}

				By("Sending POST request with ogen-typed request")
				eventRequest := ogenclient.AuditEventRequest{
					Version:        "1.0",
					EventCategory:  ogenclient.AuditEventRequestEventCategoryWorkflow,
					EventType:      "workflowexecution.workflow.completed",
					EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
					CorrelationID:  testCorrelationID,
					EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
					EventAction:    "workflow_execution",
					EventData:      eventData,
				}

				// Use ogen client to post event (handles optional fields properly)
				ctx := context.Background()
				client, err := createOpenAPIClient(serviceURL)
				Expect(err).ToNot(HaveOccurred())

				eventID, err := postAuditEvent(ctx, client, eventRequest)
				Expect(err).ToNot(HaveOccurred())
				Expect(eventID).ToNot(BeEmpty())

				// ✅ CORRECTNESS TEST: Verify Workflow event stored correctly (BR-STORAGE-033)
				// Schema per ADR-034: event_type, event_category, event_action, event_outcome, actor_id
				By("Verifying Workflow audit event content matches sent payload (CORRECTNESS)")
				var dbEventType, dbEventCategory, dbEventAction, dbEventOutcome string
				var dbActorID, dbCorrelationID string
				var dbEventData []byte

				// Handle async HTTP API processing
				Eventually(func() error {
					row := testDB.QueryRow(`
						SELECT event_type, event_category, event_action, event_outcome,
						       actor_id, correlation_id, event_data
						FROM audit_events
						WHERE event_id = $1
					`, eventID)

					return row.Scan(&dbEventType, &dbEventCategory, &dbEventAction, &dbEventOutcome,
						&dbActorID, &dbCorrelationID, &dbEventData)
				}, 5*time.Second, 100*time.Millisecond).Should(Succeed(), "Should retrieve Workflow audit event from database")

				Expect(dbEventType).To(Equal("workflowexecution.workflow.completed"), "event_type should match")
				Expect(dbEventCategory).To(Equal("workflow"), "event_category should match service prefix")
				Expect(dbEventAction).To(Equal("workflow_execution"), "event_action should match operation")
				Expect(dbEventOutcome).To(Equal("success"), "event_outcome should match")
				Expect(dbCorrelationID).To(Equal(testCorrelationID), "correlation_id should match")

				// Verify Workflow-specific event_data content is non-empty and valid JSON
				var storedEventData map[string]interface{}
				err = json.Unmarshal(dbEventData, &storedEventData)
				Expect(err).ToNot(HaveOccurred(), "event_data should be valid JSON")
				Expect(storedEventData).ToNot(BeEmpty(), "event_data should not be empty")

				// Verify Workflow-specific fields are present somewhere in the event_data
				jsonBytes, _ := json.Marshal(storedEventData)
				Expect(string(jsonBytes)).To(ContainSubstring("workflow-increase-memory"), "event_data should contain workflow_id")
			})
		})

		// NOTE: Validation tests (missing fields, invalid JSON) are covered in unit tests
		// See: test/unit/datastorage/server/middleware/openapi_test.go (lines 152+, 267+)
		// Reason: Field-level validation belongs in unit test scope, not E2E

		// NOTE: Database failure scenarios moved to unit tests
		// See: test/unit/datastorage/audit_events_handler_test.go
		// Reason: Database failure simulation requires mock infrastructure,
		// which is more appropriate for unit tests than E2E tests.

		When("multiple events are written with same correlation_id", func() {
			It("should create all events and link them via correlation_id", func() {
				// TDD GREEN: Handler creates events with correlation linking

				ctx := context.Background()
				client, err := createOpenAPIClient(serviceURL)
				Expect(err).ToNot(HaveOccurred())

				correlationID := generateTestID() // Unique per test for isolation
				timestamp := time.Now().Add(-5 * time.Second).UTC()

				By("Writing Gateway signal received event")
				gatewayEventData := ogenclient.AuditEventRequestEventData{
					Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
					GatewayAuditPayload: ogenclient.GatewayAuditPayload{
						EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
						SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
						AlertName:   "TestAlert",
						Namespace:   "default",
						Fingerprint: "test-fingerprint",
					},
				}

				gatewayRequest := ogenclient.AuditEventRequest{
					Version:        "1.0",
					EventCategory:  ogenclient.AuditEventRequestEventCategoryGateway,
					EventType:      "gateway.signal.received",
					EventTimestamp: timestamp,
					CorrelationID:  correlationID,
					EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
					EventAction:    "signal_received",
					EventData:      gatewayEventData,
				}
				_, err = postAuditEvent(ctx, client, gatewayRequest)
				Expect(err).ToNot(HaveOccurred())

				By("Writing AI Analysis completed event with same correlation_id")
				aiEventData := ogenclient.AuditEventRequestEventData{
					Type: ogenclient.AuditEventRequestEventDataAianalysisAnalysisCompletedAuditEventRequestEventData,
					AIAnalysisAuditPayload: ogenclient.AIAnalysisAuditPayload{
						EventType:    ogenclient.AIAnalysisAuditPayloadEventTypeAianalysisAnalysisCompleted,
						AnalysisName: "analysis-001",
						Phase:        ogenclient.AIAnalysisAuditPayloadPhaseCompleted, // Required field!
					},
				}

				aiRequest := ogenclient.AuditEventRequest{
					Version:        "1.0",
					EventCategory:  ogenclient.AuditEventRequestEventCategoryAnalysis,
					EventType:      "aianalysis.analysis.completed",
					EventTimestamp: timestamp,
					CorrelationID:  correlationID,
					EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
					EventAction:    "analysis",
					EventData:      aiEventData,
				}
				_, err = postAuditEvent(ctx, client, aiRequest)
				Expect(err).ToNot(HaveOccurred())

				By("Verifying both events exist with same correlation_id")
				// Use Eventually to handle async HTTP API processing and parallel execution timing
				Eventually(func() int {
					var count int
					err := testDB.QueryRow("SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1", correlationID).Scan(&count)
					if err != nil {
						return -1
					}
					return count
				}, 5*time.Second, 100*time.Millisecond).Should(Equal(2),
					"Both events should be written with same correlation_id")
			})
		})

		When("event references non-existent parent_event_id", func() {
			It("should reject with 400 Bad Request (FK constraint violation)", func() {
				// BR-STORAGE-032: Audit events can reference parent events for causality chains
				// BEHAVIOR: DataStorage API validates parent_event_id exists before inserting
				// CORRECTNESS: RFC 7807 error response with clear FK violation message
				// MIGRATION NOTE: Test was updated to use ogen client (no manual json.Marshal)

				ctx := context.Background()
				client, err := createOpenAPIClient(serviceURL)
				Expect(err).ToNot(HaveOccurred())

				By("Generating non-existent parent_event_id")
				nonExistentParentID := "00000000-0000-0000-0000-000000000001" // UUID that doesn't exist in database

				By("Attempting to create event with invalid parent_event_id")
				eventData := ogenclient.AuditEventRequestEventData{
					Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
					GatewayAuditPayload: ogenclient.GatewayAuditPayload{
						EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
						SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
						AlertName:   "ChildEvent",
						Namespace:   "production",
						Fingerprint: "test-fingerprint",
					},
				}

				// Parse the non-existent UUID
				parentUUID, _ := uuid.Parse(nonExistentParentID)

				eventRequest := ogenclient.AuditEventRequest{
					Version:        "1.0",
					EventCategory:  ogenclient.AuditEventRequestEventCategoryGateway,
					EventType:      "gateway.signal.received",
					EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
					CorrelationID:  testCorrelationID,
					ParentEventID:  ogenclient.NewOptNilUUID(parentUUID),
					ResourceType:   ogenclient.NewOptString("pod"),
					ResourceID:     ogenclient.NewOptString("child-pod-123"),
					Namespace:      ogenclient.NewOptNilString("production"),
					EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
					EventAction:    "signal_received",
					Severity:       ogenclient.NewOptNilString("info"),
					EventData:      eventData,
				}

				// Use ogen client directly to properly serialize the request
				// OGEN NOTE: ogen returns err=nil for parsed error responses (4xx/5xx)
				// The error is in the response type itself (CreateAuditEventBadRequest)
				resp, err := client.CreateAuditEvent(ctx, &eventRequest)
				Expect(err).ToNot(HaveOccurred(), "ogen should parse response without error")

				By("Verifying 400 Bad Request response")
				// ogen returns typed error responses, not Go errors
				badReq, ok := resp.(*ogenclient.CreateAuditEventBadRequest)
				Expect(ok).To(BeTrue(), "Response should be BadRequest type")

				By("Verifying RFC 7807 Problem Details error format")
				// BadRequest response should have detail about the constraint violation
				Expect(badReq.Detail.Value).ToNot(BeEmpty(), "Error detail should not be empty")
				Expect(badReq.Detail.Value).To(ContainSubstring("parent"), "Error should mention parent event")

				By("Verifying no event was created in database")
				var count int
				err = testDB.QueryRow("SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1", testCorrelationID).Scan(&count)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(0), "No event should be created when FK constraint fails")
			})
		})
	})
})
