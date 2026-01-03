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
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// AUDIT EVENTS WRITE API INTEGRATION TESTS (TDD RED Phase)
// 📋 Tests Define Contract: OpenAPI spec audit-write-api.openapi.yaml
// Authority: DAY21_PHASE1_IMPLEMENTATION_PLAN.md Phase 3
// ========================================
//
// This file defines the integration test contract for the generic audit write API.
//
// TDD DRIVEN DESIGN:
// - Tests written FIRST (this file)
// - Implementation code written SECOND (audit_events_handler.go, audit_events_repository.go)
// - Contract: POST /api/v1/audit/events with headers + JSONB body
//
// Business Requirements:
// - BR-STORAGE-033: Generic audit write API
// - BR-STORAGE-032: Unified audit trail
//
// OpenAPI Compliance:
// - Endpoint: POST /api/v1/audit/events
// - Headers: X-Event-Type, X-Service, X-Correlation-ID, X-Resource-Type, X-Resource-ID, X-Outcome, X-Severity
// - Request Body: JSONB event_data (pre-validated by Phase 2 builders)
// - Response: 201 Created with event_id (UUID) and created_at
// - Errors: 400 Bad Request, 429 Rate Limit, 500 Internal Server Error (RFC 7807)
//
// ========================================

var _ = Describe("Audit Events Write API Integration Tests",  func() {
	var testCorrelationID string

	BeforeEach(func() {
		// CRITICAL: API tests MUST use public schema
		// Rationale: The in-process HTTP API server (testServer) uses public schema,
		// not parallel process schemas. If tests insert/query data in test_process_X
		// schemas, the API won't find the data and tests will fail.
		usePublicSchema()

		// Ensure service is ready before each test
		Eventually(func() int {
			resp, err := http.Get(datastorageURL + "/health")
			if err != nil || resp == nil {
				return 0
			}
			defer resp.Body.Close()
			return resp.StatusCode
		}, "10s", "500ms").Should(Equal(200), "Data Storage Service should be ready")

		// Generate unique correlation ID for test isolation
		testCorrelationID = generateTestID()

		// Clean up test data before each test
		// Use DELETE instead of TRUNCATE to avoid table-level locks in parallel execution
		_, err := db.Exec("DELETE FROM audit_events WHERE correlation_id = $1", testCorrelationID)
		if err != nil {
			// Table might not exist yet (migration 013 not applied) - this is expected for TDD RED
			GinkgoWriter.Printf("Note: audit_events table doesn't exist yet (expected for TDD RED phase): %v\n", err)
		}
	})

	Context("BR-STORAGE-033: Generic Audit Write API", func() {
		When("Gateway service writes a signal received event", func() {
			It("should create audit event and return 201 Created", func() {
				// TDD GREEN: Handler now uses JSON body instead of headers

				By("Building Gateway event data using structured builder")
				eventData, err := audit.NewGatewayEvent("gateway.signal.received").
					WithSignalType("prometheus").
					WithAlertName("PodOOMKilled").
					WithFingerprint("sha256:abc123").
					WithNamespace("production").
					WithResource("pod", "api-server-xyz-123").
					WithSeverity("critical").
					WithPriority("P0").
					WithEnvironment("production").
					WithDeduplicationStatus("new").
					Build()
				Expect(err).ToNot(HaveOccurred())

				By("Sending POST request with JSON body")
				eventPayload := map[string]interface{}{
					"version":            "1.0",
					"event_category":     "gateway",
					"event_type":         "gateway.signal.received",
					"event_timestamp":    time.Now().UTC().Format(time.RFC3339Nano),
					"correlation_id":     testCorrelationID,
					"resource_type":      "pod",
					"resource_id":        "api-server-xyz-123",
					"resource_namespace": "production",
					"event_outcome":      "success",
					"event_action":       "signal_received",
					"severity":           "critical",
					"event_data":         eventData,
				}
				body, _ := json.Marshal(eventPayload)
				req, _ := http.NewRequest("POST", datastorageURL+"/api/v1/audit/events", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")

				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				By("Verifying 201 Created response")
				if resp.StatusCode != http.StatusCreated {
					bodyBytes, _ := io.ReadAll(resp.Body)
					GinkgoWriter.Printf("ERROR: Got status %d, body: %s\n", resp.StatusCode, string(bodyBytes))
				}
				Expect(resp.StatusCode).To(Equal(http.StatusCreated))

				By("Verifying response body contains event_id and event_timestamp (ADR-034)")
				var response map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred())
				Expect(response).To(HaveKey("event_id"))
				Expect(response).To(HaveKey("event_timestamp")) // ADR-034: event_timestamp replaces created_at
				Expect(response).To(HaveKey("message"))
				Expect(response["message"]).To(Equal("Audit event created successfully"))

				By("Verifying event_id is a valid UUID")
				eventID, ok := response["event_id"].(string)
				Expect(ok).To(BeTrue())
				Expect(eventID).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`))

				By("Verifying audit event was inserted into database")
				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE event_id = $1", eventID).Scan(&count)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(1))

				// ✅ CORRECTNESS TEST: Data in database matches input exactly (BR-STORAGE-033)
				// Schema per ADR-034: event_type, event_category, event_action, event_outcome, actor_id, actor_type
				By("Verifying audit event content matches sent payload (CORRECTNESS)")
				var dbEventType, dbEventCategory, dbEventAction, dbEventOutcome string
				var dbActorID, dbActorType, dbCorrelationID, dbResourceType, dbResourceID string
				var dbEventData []byte

				row := db.QueryRow(`
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

				By("Building AI Analysis event data using structured builder")
				eventData, err := audit.NewAIAnalysisEvent("analysis.completed").
					WithAnalysisID("analysis-2025-001").
					WithLLM("anthropic", "claude-haiku-4-5-20251001").
					WithTokenUsage(2500, 750).
					WithDuration(4200).
					WithRCA("OOMKilled", "critical", 0.95).
					WithWorkflow("workflow-increase-memory").
					WithToolsInvoked([]string{"kubernetes/describe_pod", "workflow/search_catalog"}).
					Build()
				Expect(err).ToNot(HaveOccurred())

				By("Sending POST request with JSON body")
				eventPayload := map[string]interface{}{
					"version":         "1.0",
					"event_category":  "analysis",
					"event_type":      "analysis.analysis.completed",
					"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
					"correlation_id":  testCorrelationID,
					"event_outcome":   "success",
					"event_action":    "analysis",
					"event_data":      eventData,
				}
				body, _ := json.Marshal(eventPayload)
				req, _ := http.NewRequest("POST", datastorageURL+"/api/v1/audit/events", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")

				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusCreated))

				// Extract event_id from response for CORRECTNESS validation
				var response map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred())
				eventID, ok := response["event_id"].(string)
				Expect(ok).To(BeTrue())

				// ✅ CORRECTNESS TEST: Verify AI Analysis event stored correctly (BR-STORAGE-033)
				// Schema per ADR-034: event_type, event_category, event_action, event_outcome, actor_id
				By("Verifying AI Analysis audit event content matches sent payload (CORRECTNESS)")
				var dbEventType, dbEventCategory, dbEventAction, dbEventOutcome string
				var dbActorID, dbCorrelationID string
				var dbEventData []byte

				row := db.QueryRow(`
					SELECT event_type, event_category, event_action, event_outcome,
					       actor_id, correlation_id, event_data
					FROM audit_events
					WHERE event_id = $1
				`, eventID)

				err = row.Scan(&dbEventType, &dbEventCategory, &dbEventAction, &dbEventOutcome,
					&dbActorID, &dbCorrelationID, &dbEventData)
				Expect(err).ToNot(HaveOccurred(), "Should retrieve AI Analysis audit event from database")

				Expect(dbEventType).To(Equal("analysis.analysis.completed"), "event_type should match")
				Expect(dbEventCategory).To(Equal("analysis"), "event_category should match service prefix")
				Expect(dbEventAction).To(Equal("analysis"), "event_action should match operation")
				Expect(dbEventOutcome).To(Equal("success"), "event_outcome should match")
				Expect(dbActorID).To(Equal("analysis-service"), "actor_id defaults to event_category + '-service'")
				Expect(dbCorrelationID).To(Equal(testCorrelationID), "correlation_id should match")

				// Verify AI-specific event_data content is non-empty and valid JSON
				var storedEventData map[string]interface{}
				err = json.Unmarshal(dbEventData, &storedEventData)
				Expect(err).ToNot(HaveOccurred(), "event_data should be valid JSON")
				Expect(storedEventData).ToNot(BeEmpty(), "event_data should not be empty")

				// Verify AI-specific fields are present somewhere in the event_data
				jsonBytes, _ := json.Marshal(storedEventData)
				Expect(string(jsonBytes)).To(ContainSubstring("analysis-2025-001"), "event_data should contain analysis_id")
				Expect(string(jsonBytes)).To(ContainSubstring("anthropic"), "event_data should contain llm_provider")
			})
		})

		When("Workflow service writes a workflow completed event", func() {
			It("should create audit event with workflow-specific data", func() {
				// TDD GREEN: Handler now uses JSON body

				By("Building Workflow event data using structured builder")
				eventData, err := audit.NewWorkflowEvent("workflow.completed").
					WithWorkflowID("workflow-increase-memory").
					WithExecutionID("exec-2025-001").
					WithPhase("completed").
					WithOutcome("success").
					WithDuration(45000).
					WithCurrentStep(5, 5).
					WithApprovalDecision("approved", "sre-team@example.com").
					Build()
				Expect(err).ToNot(HaveOccurred())

				By("Sending POST request with JSON body")
				eventPayload := map[string]interface{}{
					"version":         "1.0",
					"event_category":  "workflow",
					"event_type":      "workflow.workflow.completed",
					"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
					"correlation_id":  testCorrelationID,
					"event_outcome":   "success",
					"event_action":    "workflow_execution",
					"event_data":      eventData,
				}
				body, _ := json.Marshal(eventPayload)
				req, _ := http.NewRequest("POST", datastorageURL+"/api/v1/audit/events", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")

				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusCreated))

				// Extract event_id from response for CORRECTNESS validation
				var response map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred())
				eventID, ok := response["event_id"].(string)
				Expect(ok).To(BeTrue())

				// ✅ CORRECTNESS TEST: Verify Workflow event stored correctly (BR-STORAGE-033)
				// Schema per ADR-034: event_type, event_category, event_action, event_outcome, actor_id
				By("Verifying Workflow audit event content matches sent payload (CORRECTNESS)")
				var dbEventType, dbEventCategory, dbEventAction, dbEventOutcome string
				var dbActorID, dbCorrelationID string
				var dbEventData []byte

				row := db.QueryRow(`
					SELECT event_type, event_category, event_action, event_outcome,
					       actor_id, correlation_id, event_data
					FROM audit_events
					WHERE event_id = $1
				`, eventID)

				err = row.Scan(&dbEventType, &dbEventCategory, &dbEventAction, &dbEventOutcome,
					&dbActorID, &dbCorrelationID, &dbEventData)
				Expect(err).ToNot(HaveOccurred(), "Should retrieve Workflow audit event from database")

				Expect(dbEventType).To(Equal("workflow.workflow.completed"), "event_type should match")
				Expect(dbEventCategory).To(Equal("workflow"), "event_category should match service prefix")
				Expect(dbEventAction).To(Equal("workflow_execution"), "event_action should match operation")
				Expect(dbEventOutcome).To(Equal("success"), "event_outcome should match")
				Expect(dbActorID).To(Equal("workflow-service"), "actor_id defaults to event_category + '-service'")
				Expect(dbCorrelationID).To(Equal(testCorrelationID), "correlation_id should match")

				// Verify Workflow-specific event_data content is non-empty and valid JSON
				var storedEventData map[string]interface{}
				err = json.Unmarshal(dbEventData, &storedEventData)
				Expect(err).ToNot(HaveOccurred(), "event_data should be valid JSON")
				Expect(storedEventData).ToNot(BeEmpty(), "event_data should not be empty")

				// Verify Workflow-specific fields are present somewhere in the event_data
				jsonBytes, _ := json.Marshal(storedEventData)
				Expect(string(jsonBytes)).To(ContainSubstring("workflow-increase-memory"), "event_data should contain workflow_id")
				Expect(string(jsonBytes)).To(ContainSubstring("exec-2025-001"), "event_data should contain execution_id")
			})
		})

		When("request is missing required field event_type", func() {
			It("should return 400 Bad Request with RFC 7807 error", func() {
				// TDD GREEN: Validation now checks JSON body fields

				eventPayload := map[string]interface{}{
					"version":        "1.0",
					"event_category": "gateway",
					// Missing "event_type" field
					"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
					"correlation_id":  testCorrelationID,
					"event_outcome":   "success",
					"event_action":    "test",
					"event_data":      map[string]interface{}{},
				}

				body, _ := json.Marshal(eventPayload)
				req, _ := http.NewRequest("POST", datastorageURL+"/api/v1/audit/events", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")

				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				By("Verifying 400 Bad Request response")
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

				By("Verifying RFC 7807 problem details")
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

				var problem map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&problem)
				Expect(err).ToNot(HaveOccurred())
				Expect(problem).To(HaveKey("type"))
				Expect(problem).To(HaveKey("title"))
				Expect(problem).To(HaveKey("status"))
				Expect(problem).To(HaveKey("detail"))
				Expect(problem["status"]).To(BeNumerically("==", 400))
				Expect(problem["detail"]).To(ContainSubstring("event_type"))
			})
		})

		When("request body has invalid JSON", func() {
			It("should return 400 Bad Request with RFC 7807 error", func() {
				// TDD GREEN: Validation checks JSON parsing

				req, _ := http.NewRequest("POST", datastorageURL+"/api/v1/audit/events", bytes.NewBufferString("{invalid json"))
				req.Header.Set("Content-Type", "application/json")

				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
				Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))
			})
		})

		When("request body is missing required 'version' field", func() {
			It("should return 400 Bad Request with RFC 7807 error", func() {
				// TDD GREEN: Validation checks required fields

				eventPayload := map[string]interface{}{
					// Missing "version" field
					"event_category":  "gateway",
					"event_type":      "gateway.signal.received",
					"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
					"correlation_id":  testCorrelationID,
					"event_outcome":   "success",
					"event_action":    "test",
					"event_data":      map[string]interface{}{},
				}

				body, _ := json.Marshal(eventPayload)
				req, _ := http.NewRequest("POST", datastorageURL+"/api/v1/audit/events", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")

				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

				var problem map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&problem)
				Expect(err).ToNot(HaveOccurred())
				Expect(problem["detail"]).To(ContainSubstring("version"))
			})
		})

		// NOTE: Database failure scenarios moved to unit tests
		// See: test/unit/datastorage/audit_events_handler_test.go
		// Reason: Database failure simulation requires mock infrastructure,
		// which is more appropriate for unit tests than integration tests.

		When("multiple events are written with same correlation_id", func() {
			It("should create all events and link them via correlation_id", func() {
				// TDD GREEN: Handler creates events with correlation linking

				correlationID := generateTestID() // Unique per test for isolation

				By("Writing Gateway signal received event")
				gatewayEventData, err := audit.NewGatewayEvent("signal.received").
					WithSignalType("prometheus").
					Build()
				Expect(err).ToNot(HaveOccurred())

				gatewayPayload := map[string]interface{}{
					"version":         "1.0",
					"event_category":  "gateway",
					"event_type":      "gateway.signal.received",
					"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
					"correlation_id":  correlationID,
					"event_outcome":   "success",
					"event_action":    "signal_received",
					"event_data":      gatewayEventData,
				}
				body1, _ := json.Marshal(gatewayPayload)
				req1, _ := http.NewRequest("POST", datastorageURL+"/api/v1/audit/events", bytes.NewBuffer(body1))
				req1.Header.Set("Content-Type", "application/json")

				resp1, err := http.DefaultClient.Do(req1)
				Expect(err).ToNot(HaveOccurred())
				defer resp1.Body.Close()
				Expect(resp1.StatusCode).To(Equal(http.StatusCreated))

				By("Writing AI Analysis completed event with same correlation_id")
				aiEventData, err := audit.NewAIAnalysisEvent("analysis.completed").
					WithAnalysisID("analysis-001").
					Build()
				Expect(err).ToNot(HaveOccurred())

				aiPayload := map[string]interface{}{
					"version":         "1.0",
					"event_category":  "analysis",
					"event_type":      "analysis.analysis.completed",
					"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
					"correlation_id":  correlationID,
					"event_outcome":   "success",
					"event_action":    "analysis",
					"event_data":      aiEventData,
				}
				body2, _ := json.Marshal(aiPayload)
				req2, _ := http.NewRequest("POST", datastorageURL+"/api/v1/audit/events", bytes.NewBuffer(body2))
				req2.Header.Set("Content-Type", "application/json")

				resp2, err := http.DefaultClient.Do(req2)
				Expect(err).ToNot(HaveOccurred())
				defer resp2.Body.Close()
				Expect(resp2.StatusCode).To(Equal(http.StatusCreated))

			By("Verifying both events exist with same correlation_id")
			// Use Eventually to handle async HTTP API processing and parallel execution timing
			Eventually(func() int {
				var count int
				err := db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1", correlationID).Scan(&count)
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
				// BEHAVIOR: PostgreSQL FK constraint prevents orphaned parent references
				// CORRECTNESS: RFC 7807 error response with clear FK violation message

				By("Generating non-existent parent_event_id")
				nonExistentParentID := "00000000-0000-0000-0000-000000000001" // UUID that doesn't exist in database
				parentEventDate := time.Now().UTC().Format("2006-01-02")      // Today's date for partition

				By("Attempting to create event with invalid parent_event_id")
				eventData, err := audit.NewGatewayEvent("gateway.signal.received").
					WithSignalType("prometheus").
					WithAlertName("ChildEvent").
					Build()
				Expect(err).ToNot(HaveOccurred())

				eventPayload := map[string]interface{}{
					"version":            "1.0",
					"event_category":     "gateway",
					"event_type":         "gateway.signal.received",
					"event_timestamp":    time.Now().UTC().Format(time.RFC3339Nano),
					"correlation_id":     testCorrelationID,
					"parent_event_id":    nonExistentParentID,
					"parent_event_date":  parentEventDate,
					"resource_type":      "pod",
					"resource_id":        "child-pod-123",
					"resource_namespace": "production",
					"event_outcome":      "success",
					"event_action":       "signal_received",
					"severity":           "info",
					"event_data":         eventData,
				}
				body, _ := json.Marshal(eventPayload)
				req, _ := http.NewRequest("POST", datastorageURL+"/api/v1/audit/events", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")

				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				By("Verifying 400 Bad Request response")
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Should reject FK constraint violation")

				By("Verifying RFC 7807 Problem Details error format")
				bodyBytes, err := io.ReadAll(resp.Body)
				Expect(err).ToNot(HaveOccurred())

				var errorResponse map[string]interface{}
				err = json.Unmarshal(bodyBytes, &errorResponse)
				Expect(err).ToNot(HaveOccurred())

				// RFC 7807 requires these fields
				Expect(errorResponse).To(HaveKey("type"))
				Expect(errorResponse).To(HaveKey("title"))
				Expect(errorResponse).To(HaveKey("status"))
				Expect(errorResponse).To(HaveKey("detail"))

				// Verify error indicates FK constraint violation
				detail, ok := errorResponse["detail"].(string)
				Expect(ok).To(BeTrue(), "detail should be a string")
				Expect(detail).To(ContainSubstring("parent"), "Error should mention parent event")

				By("Verifying no event was created in database")
				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1", testCorrelationID).Scan(&count)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(0), "No event should be created when FK constraint fails")
			})
		})
	})
})
