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

var _ = Describe("Audit Events Write API Integration Tests", func() {
	var testCorrelationID string

	BeforeEach(func() {
		// Generate unique correlation ID for test isolation
		// Note: No TRUNCATE needed - tests are isolated via correlation_id
		testCorrelationID = generateTestID()
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
					"service":            "gateway",
					"event_type":         "gateway.signal.received",
					"event_timestamp":    time.Now().UTC().Format(time.RFC3339Nano),
					"correlation_id":     testCorrelationID,
					"resource_type":      "pod",
					"resource_id":        "api-server-xyz-123",
					"resource_namespace": "production",
					"outcome":            "success",
					"operation":          "signal_received",
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

				By("Verifying response body contains event_id and created_at")
				var response map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred())
				Expect(response).To(HaveKey("event_id"))
				Expect(response).To(HaveKey("created_at"))
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
					"version":          "1.0",
					"service":          "aianalysis",
					"event_type":       "aianalysis.analysis.completed",
					"event_timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
					"correlation_id":   testCorrelationID,
					"outcome":          "success",
					"operation":        "analysis",
					"event_data":       eventData,
				}
				body, _ := json.Marshal(eventPayload)
				req, _ := http.NewRequest("POST", datastorageURL+"/api/v1/audit/events", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")

				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusCreated))
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
					"version":          "1.0",
					"service":          "workflow",
					"event_type":       "workflow.workflow.completed",
					"event_timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
					"correlation_id":   testCorrelationID,
					"outcome":          "success",
					"operation":        "workflow_execution",
					"event_data":       eventData,
				}
				body, _ := json.Marshal(eventPayload)
				req, _ := http.NewRequest("POST", datastorageURL+"/api/v1/audit/events", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")

				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusCreated))
			})
		})

		When("request is missing required field event_type", func() {
			It("should return 400 Bad Request with RFC 7807 error", func() {
				// TDD GREEN: Validation now checks JSON body fields

				eventPayload := map[string]interface{}{
					"version":          "1.0",
					"service":          "gateway",
					// Missing "event_type" field
					"event_timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
					"correlation_id":   testCorrelationID,
					"outcome":          "success",
					"operation":        "test",
					"event_data":       map[string]interface{}{},
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
					"service":          "gateway",
					"event_type":       "gateway.signal.received",
					"event_timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
					"correlation_id":   testCorrelationID,
					"outcome":          "success",
					"operation":        "test",
					"event_data":       map[string]interface{}{},
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
					"version":          "1.0",
					"service":          "gateway",
					"event_type":       "gateway.signal.received",
					"event_timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
					"correlation_id":   correlationID,
					"outcome":          "success",
					"operation":        "signal_received",
					"event_data":       gatewayEventData,
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
					"version":          "1.0",
					"service":          "aianalysis",
					"event_type":       "aianalysis.analysis.completed",
					"event_timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
					"correlation_id":   correlationID,
					"outcome":          "success",
					"operation":        "analysis",
					"event_data":       aiEventData,
				}
				body2, _ := json.Marshal(aiPayload)
				req2, _ := http.NewRequest("POST", datastorageURL+"/api/v1/audit/events", bytes.NewBuffer(body2))
				req2.Header.Set("Content-Type", "application/json")

				resp2, err := http.DefaultClient.Do(req2)
				Expect(err).ToNot(HaveOccurred())
				defer resp2.Body.Close()
				Expect(resp2.StatusCode).To(Equal(http.StatusCreated))

				By("Verifying both events exist with same correlation_id")
				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1", correlationID).Scan(&count)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(2))
			})
		})
	})
})

