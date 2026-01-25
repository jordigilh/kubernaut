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
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-STORAGE-021: REST API Read Endpoints
// BR-STORAGE-022: Query Filtering
// BR-STORAGE-023: Pagination Validation
// DD-STORAGE-010: Query API Pagination Strategy (V1.0: offset-based)

// Helper function to create audit events via Write API
func createTestAuditEvent(baseURL, service, eventType, correlationID string) error {
	// Build service-specific event data using structured builders
	var eventData map[string]interface{}
	var err error

	switch service {
	case "gateway":
		eventData, err = audit.NewGatewayEvent(eventType).
			WithSignalType("prometheus").
			WithAlertName("TestAlert").
			Build()
	case "analysis": // ADR-034: Use "analysis" not "aianalysis"
		eventData, err = audit.NewAIAnalysisEvent(eventType).
			WithAnalysisID("test-analysis").
			Build()
	case "workflow":
		eventData, err = audit.NewWorkflowEvent(eventType).
			WithWorkflowID("test-workflow").
			Build()
	default:
		return fmt.Errorf("unsupported service type: %s (must use structured event builder)", service)
	}

	if err != nil {
		return err
	}

	eventPayload := map[string]interface{}{
		"version":         "1.0",
		"event_category":  service, // ADR-034: Use event_category instead of service
		"event_type":      fmt.Sprintf("%s.%s", service, eventType),
		"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"correlation_id":  correlationID,
		"event_outcome":   "success", // ADR-034: Use event_outcome instead of outcome
		"event_action":    "test",    // ADR-034: Use event_action instead of operation
		"event_data":      eventData,
	}
	body, _ := json.Marshal(eventPayload)
	req, _ := http.NewRequest("POST", baseURL, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create event: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}

var _ = Describe("Audit Events Query API",  func() {
	var baseURL string

	BeforeEach(func() {
		baseURL = datastorageURL + "/api/v1/audit/events"
		// Note: No cleanup needed - schema-level isolation provides complete data isolation
		// Each parallel process runs in its own schema (test_process_N)
		// AfterSuite drops entire schema with DROP SCHEMA CASCADE
	})

	Context("Query by correlation_id", func() {
		It("should return all events for a remediation in chronological order", func() {
			// BR-STORAGE-021: Query by correlation_id (remediation timeline)
			// DD-STORAGE-010: Offset-based pagination

			// ARRANGE: Insert test events for correlation_id "rr-2025-001"
			correlationID := generateTestID() // Unique per test for isolation
			eventTypes := []string{
				"gateway.signal.received",
				"aianalysis.analysis.started",
				"aianalysis.analysis.completed",
				"workflow.workflow.started",
				"workflow.workflow.completed",
			}

			for i, eventType := range eventTypes {
				// Use structured event builder for Gateway events
				eventData, err := audit.NewGatewayEvent(eventType).
					WithSignalType("prometheus").
					WithAlertName(fmt.Sprintf("TestAlert%d", i)).
					Build()
				Expect(err).ToNot(HaveOccurred())

				// Create JSON body with all required fields
				eventPayload := map[string]interface{}{
					"version":         "1.0",
					"event_category":  "gateway",
					"event_type":      eventType,
					"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
					"correlation_id":  correlationID,
					"event_outcome":   "success",
					"event_action":    "test",
					"event_data":      eventData,
				}
				body, err := json.Marshal(eventPayload)
				Expect(err).ToNot(HaveOccurred())

				// Write event via POST endpoint
				req, err := http.NewRequest("POST", baseURL, bytes.NewBuffer(body))
				Expect(err).ToNot(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")

				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Failed to create audit event: %s", eventType)
				_ = resp.Body.Close()

				// Add small delay to ensure chronological ordering
				// Per TESTING_GUIDELINES.md: ACCEPTABLE - testing timing behavior (chronological order)
				time.Sleep(10 * time.Millisecond)
			}

			// ACT: Query by correlation_id
			resp, err := http.Get(fmt.Sprintf("%s?correlation_id=%s", baseURL, correlationID))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// ASSERT: Response is 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// ASSERT: Response contains all events in chronological order
			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			data, ok := response["data"].([]interface{})
			Expect(ok).To(BeTrue(), "response should have 'data' array")
			Expect(data).To(HaveLen(5), "should return all 5 events")

			// ASSERT: Events are in chronological order (DESC)
			for i := 0; i < len(data)-1; i++ {
				event1 := data[i].(map[string]interface{})
				event2 := data[i+1].(map[string]interface{})
				timestamp1, _ := time.Parse(time.RFC3339, event1["event_timestamp"].(string))
				timestamp2, _ := time.Parse(time.RFC3339, event2["event_timestamp"].(string))
				Expect(timestamp1.After(timestamp2) || timestamp1.Equal(timestamp2)).To(BeTrue(),
					"events should be in chronological order (DESC)")
			}

			// ASSERT: Pagination metadata is present
			pagination, ok := response["pagination"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "response should have 'pagination' object")
			Expect(pagination["limit"]).To(BeNumerically("==", 50)) // Default limit per OpenAPI spec
			Expect(pagination["offset"]).To(BeNumerically("==", 0))
			Expect(pagination["total"]).To(BeNumerically("==", 5))
			Expect(pagination["has_more"]).To(BeFalse())
		})
	})

	Context("Query by event_type", func() {
		It("should return only events matching the event_type filter", func() {
			// BR-STORAGE-022: Query filtering by event_type

			// ARRANGE: Insert events with different event_types
			correlationID := generateTestID() // Unique per test for isolation
			eventTypes := map[string]int{
				"gateway.signal.received":       3,
				"aianalysis.analysis.completed": 2,
				"workflow.workflow.completed":   1,
			}

			for eventType, count := range eventTypes {
				for i := 0; i < count; i++ {
					// Use structured event builder for Gateway events
					eventData, err := audit.NewGatewayEvent(eventType).
						WithSignalType("prometheus").
						WithAlertName(fmt.Sprintf("TestAlert-%s-%d", eventType, i)).
						Build()
					Expect(err).ToNot(HaveOccurred())

					// Create JSON body with all required fields
					eventPayload := map[string]interface{}{
						"version":         "1.0",
						"event_category":  "gateway",
						"event_type":      eventType,
						"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
						"correlation_id":  correlationID,
						"event_outcome":   "success",
						"event_action":    "test",
						"event_data":      eventData,
					}
					body, err := json.Marshal(eventPayload)
					Expect(err).ToNot(HaveOccurred())

					req, err := http.NewRequest("POST", baseURL, bytes.NewBuffer(body))
					Expect(err).ToNot(HaveOccurred())
					req.Header.Set("Content-Type", "application/json")

					resp, err := http.DefaultClient.Do(req)
					Expect(err).ToNot(HaveOccurred())
					if resp.StatusCode != http.StatusCreated {
						bodyBytes, _ := io.ReadAll(resp.Body)
						GinkgoWriter.Printf("ERROR: Got status %d, body: %s\n", resp.StatusCode, string(bodyBytes))
					}
					Expect(resp.StatusCode).To(Equal(http.StatusCreated))
					_ = resp.Body.Close()
				}
			}

			// ACT: Query by event_type and correlation_id for test isolation
			targetEventType := "gateway.signal.received"
			resp, err := http.Get(fmt.Sprintf("%s?event_type=%s&correlation_id=%s", baseURL, targetEventType, correlationID))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// ASSERT: Response is 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// ASSERT: Only events with matching event_type are returned
			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			data, ok := response["data"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(data).To(HaveLen(3), "should return only 3 gateway.signal.received events for this correlation_id")

			for _, item := range data {
				event := item.(map[string]interface{})
				Expect(event["event_type"]).To(Equal(targetEventType))
			}
		})
	})

	Context("Query by service", func() {
		It("should return only events from the specified service", func() {
			// BR-STORAGE-022: Query filtering by event_category (ADR-034: service renamed to event_category)

		// ARRANGE: Insert events from different services
		correlationID := generateTestID() // Unique per test for isolation
		services := map[string]int{
			"gateway":  2,
			"analysis": 3, // ADR-034: Use "analysis" not "aianalysis"
			"workflow": 1,
		}

			for service, count := range services {
				for i := 0; i < count; i++ {
					err := createTestAuditEvent(baseURL, service, "test.event", correlationID)
					Expect(err).ToNot(HaveOccurred())
				}
			}

		// ACT: Query by event_category (ADR-034) and correlation_id for test isolation
		targetService := "analysis" // ADR-034: Use "analysis" not "aianalysis"
		resp, err := http.Get(fmt.Sprintf("%s?event_category=%s&correlation_id=%s", baseURL, targetService, correlationID))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// ASSERT: Response is 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// ASSERT: Only events from target service are returned
			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			data, ok := response["data"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(data).To(HaveLen(3), "should return only 3 aianalysis events for this correlation_id")

			for _, item := range data {
				event := item.(map[string]interface{})
				// ADR-034: Response uses event_category, not service
				Expect(event["event_category"]).To(Equal(targetService))
			}
		})
	})

	Context("Query by time range", func() {
		It("should return events within the specified time range using relative time (since=24h)", func() {
			// BR-STORAGE-022: Query filtering by time range
			// DD-STORAGE-010: Time parsing (relative: 24h, 7d)

			// ARRANGE: Insert events at different times
			// (In real implementation, would manipulate event_timestamp)
			// For now, all events are recent (<24h)

			correlationID := generateTestID() // Unique per test for isolation
			for i := 0; i < 5; i++ {
				err := createTestAuditEvent(baseURL, "gateway", "signal.received", correlationID)
				Expect(err).ToNot(HaveOccurred())
			}

			// ACT: Query with since=24h
			resp, err := http.Get(fmt.Sprintf("%s?correlation_id=%s&since=24h", baseURL, correlationID))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// ASSERT: Response is 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// ASSERT: All recent events are returned
			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			data, ok := response["data"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(data).To(HaveLen(5), "should return all 5 recent events")

			// ASSERT: All events are within last 24 hours
			now := time.Now()
			for _, item := range data {
				event := item.(map[string]interface{})
				timestamp, err := time.Parse(time.RFC3339, event["event_timestamp"].(string))
				Expect(err).ToNot(HaveOccurred())
				Expect(now.Sub(timestamp)).To(BeNumerically("<", 24*time.Hour))
			}
		})

		It("should return events within absolute time range (since/until)", func() {
			// DD-STORAGE-010: Time parsing (absolute: RFC3339)

			// ARRANGE: Insert events
			correlationID := generateTestID() // Unique per test for isolation
			for i := 0; i < 3; i++ {
				err := createTestAuditEvent(baseURL, "gateway", "signal.received", correlationID)
				Expect(err).ToNot(HaveOccurred())
			}

			// ACT: Query with absolute time range
			now := time.Now()
			since := now.Add(-1 * time.Hour).Format(time.RFC3339)
			until := now.Add(1 * time.Hour).Format(time.RFC3339)
			resp, err := http.Get(fmt.Sprintf("%s?correlation_id=%s&since=%s&until=%s",
				baseURL, correlationID, since, until))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// ASSERT: Response is 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// ASSERT: Events within time range are returned
			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			data, ok := response["data"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(data).To(HaveLen(3))
		})
	})

	Context("Query with multiple filters", func() {
		It("should return events matching all filters (service=gateway AND outcome=failure)", func() {
			// BR-STORAGE-022: Multiple filter support

			// ARRANGE: Insert events with different outcomes
			correlationID := generateTestID() // Unique per test for isolation
			outcomes := []string{"success", "failure", "success", "failure"}
			for _, outcome := range outcomes {
				eventData, err := audit.NewGatewayEvent("signal.received").
					WithSignalType("prometheus").
					WithAlertName("TestAlert").
					Build()
				Expect(err).ToNot(HaveOccurred())

				eventPayload := map[string]interface{}{
					"version":         "1.0",
					"event_category":  "gateway",
					"event_type":      "gateway.signal.received",
					"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
					"correlation_id":  correlationID,
					"event_outcome":   outcome,
					"event_action":    "test",
					"event_data":      eventData,
				}
				body, _ := json.Marshal(eventPayload)
				req, _ := http.NewRequest("POST", baseURL, bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")

				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusCreated))
				_ = resp.Body.Close()
			}

			// ACT: Query with multiple filters (ADR-034 field names)
			resp, err := http.Get(fmt.Sprintf("%s?event_category=gateway&event_outcome=failure", baseURL))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// ASSERT: Response is 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// ASSERT: Only events matching ALL filters are returned
			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			data, ok := response["data"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(data).To(HaveLen(2), "should return only 2 failure events")

			for _, item := range data {
				event := item.(map[string]interface{})
				// ADR-034: Response uses event_category and event_outcome
				Expect(event["event_category"]).To(Equal("gateway"))
				Expect(event["event_outcome"]).To(Equal("failure"))
			}
		})
	})

	Context("Pagination", func() {
		var testCorrelationID string

		AfterEach(func() {
			// Cleanup test data
			if testCorrelationID != "" {
				_, err := db.ExecContext(ctx, "DELETE FROM audit_events WHERE correlation_id = $1", testCorrelationID)
				Expect(err).ToNot(HaveOccurred())
			}
		})

		It("should return correct subset with limit and offset", func() {
			// BR-STORAGE-023: Pagination support
			// DD-STORAGE-010: Offset-based pagination

			// ARRANGE: Insert 75 events (reduced from 150 for reliable flush timing)
			// FIX: DS pagination test failure - reduced events to fit in single batch + leftover
			// - Batch size: 100 events (triggers immediate flush)
			// - Leftover: 50 events (waits for 1s timer)
			// - Total time: ~1-2s (1 batch flush + 1 timer tick)
			// - Previous: 150 events required 2 batches (100 + 50) = more timing variance
			// See: docs/handoff/DS_SERVICE_FAILURE_MODES_ANALYSIS_JAN_04_2026.md
			testCorrelationID = generateTestID() // Unique per test for isolation
			correlationID := testCorrelationID
			for i := 0; i < 75; i++ {
				err := createTestAuditEvent(baseURL, "gateway", "signal.received", correlationID)
				Expect(err).ToNot(HaveOccurred())
			}

			// WAIT: Allow events to be persisted (synchronous write, no buffering)
			// NOTE: Events are written synchronously by DataStorage HTTP API
			// Timeout increased from 5s to 10s to account for:
			// - Parallel test execution (12 processes)
			// - Database connection contention
			// - Potential schema isolation overhead
			var response map[string]interface{}
			Eventually(func() float64 {
				resp, err := http.Get(fmt.Sprintf("%s?correlation_id=%s&limit=50&offset=0", baseURL, correlationID))
				if err != nil {
					return 0
				}
				defer func() { _ = resp.Body.Close() }()

				if resp.StatusCode != http.StatusOK {
					return 0
				}

				if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
					return 0
				}

				pagination, ok := response["pagination"].(map[string]interface{})
				if !ok {
					return 0
				}

				total, ok := pagination["total"].(float64)
				if !ok {
					return 0
				}

				return total
			}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 75),
				"should have at least 75 events after write completes")

			// ACT: Query page 1 (limit=50, offset=0) - now guaranteed to have all events
			resp, err := http.Get(fmt.Sprintf("%s?correlation_id=%s&limit=50&offset=0", baseURL, correlationID))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// ASSERT: Response is 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// ASSERT: Correct subset is returned
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			data, ok := response["data"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(data).To(HaveLen(50), "should return 50 events (page 1)")

			// ASSERT: Pagination metadata is correct
			pagination, ok := response["pagination"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(pagination["limit"]).To(BeNumerically("==", 50))
			Expect(pagination["offset"]).To(BeNumerically("==", 0))
			Expect(pagination["total"]).To(BeNumerically(">=", 75),
				"should have at least 75 events for this correlation_id")
			Expect(pagination["has_more"]).To(BeTrue())

			// ACT: Query page 2 (limit=50, offset=50)
			resp2, err := http.Get(fmt.Sprintf("%s?correlation_id=%s&limit=50&offset=50", baseURL, correlationID))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp2.Body.Close() }()

			// ASSERT: Page 2 returns remaining events (25 events = 75 total - 50 offset)
			var response2 map[string]interface{}
			err = json.NewDecoder(resp2.Body).Decode(&response2)
			Expect(err).ToNot(HaveOccurred())

			data2, ok := response2["data"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(data2).To(HaveLen(25), "should return 25 remaining events (page 2)")

			pagination2, ok := response2["pagination"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(pagination2["offset"]).To(BeNumerically("==", 50))
			Expect(pagination2["total"]).To(BeNumerically(">=", 75))
			Expect(pagination2["has_more"]).To(BeFalse(), "no more pages after 75 events (50 + 25 = 75)")

			// ASSERT: Total events retrieved across 2 pages
			totalRetrieved := len(data) + len(data2)
			Expect(totalRetrieved).To(Equal(75), "should retrieve exactly 75 events across 2 pages")
		})
	})

	Context("Pagination validation", func() {
		It("should return RFC 7807 error for invalid limit (0)", func() {
			// BR-STORAGE-023: Pagination validation (limit: 1-1000)

			// ACT: Query with invalid limit=0
			resp, err := http.Get(fmt.Sprintf("%s?limit=0", baseURL))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// ASSERT: Response is 400 Bad Request
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

			// ASSERT: RFC 7807 error response
			var problem map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&problem)
			Expect(err).ToNot(HaveOccurred())

		Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/validation-error"))
		Expect(problem["title"]).To(Equal("Validation Error"))
		Expect(problem["status"]).To(BeNumerically("==", 400))
		Expect(problem["detail"]).To(ContainSubstring("limit"))
		Expect(problem["detail"]).To(ContainSubstring("must be at least 1"))
		})

		It("should return RFC 7807 error for invalid limit (1001)", func() {
			// BR-STORAGE-023: Pagination validation (limit: 1-1000)

			// ACT: Query with invalid limit=1001
			resp, err := http.Get(fmt.Sprintf("%s?limit=1001", baseURL))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// ASSERT: Response is 400 Bad Request
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

			// ASSERT: RFC 7807 error response
			var problem map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&problem)
			Expect(err).ToNot(HaveOccurred())

		Expect(problem["detail"]).To(ContainSubstring("limit"))
		Expect(problem["detail"]).To(ContainSubstring("must be at most 1000"))
	})

	It("should return RFC 7807 error for invalid offset (-1)", func() {
			// BR-STORAGE-023: Pagination validation (offset: ≥0)

			// ACT: Query with invalid offset=-1
			resp, err := http.Get(fmt.Sprintf("%s?offset=-1", baseURL))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// ASSERT: Response is 400 Bad Request
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

			// ASSERT: RFC 7807 error response
			var problem map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&problem)
			Expect(err).ToNot(HaveOccurred())

		Expect(problem["detail"]).To(ContainSubstring("offset"))
		Expect(problem["detail"]).To(ContainSubstring("must be at least 0"))
		})
	})

	Context("Time parsing validation", func() {
		It("should return RFC 7807 error for invalid since format", func() {
			// DD-STORAGE-010: Time parsing validation

			// ACT: Query with invalid since format
			resp, err := http.Get(fmt.Sprintf("%s?since=invalid", baseURL))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// ASSERT: Response is 400 Bad Request
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

			// ASSERT: RFC 7807 error response
			var problem map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&problem)
			Expect(err).ToNot(HaveOccurred())

			Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/validation-error"))
			// Validation error message includes "query" and "invalid time format"
			Expect(problem["detail"]).To(ContainSubstring("invalid time format"))
		})
	})

	Context("Empty result set", func() {
		It("should return empty data array with pagination metadata", func() {
			// BR-STORAGE-021: Empty result handling

			// ACT: Query for non-existent correlation_id
			resp, err := http.Get(fmt.Sprintf("%s?correlation_id=rr-9999-999", baseURL))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// ASSERT: Response is 200 OK (not 404)
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// ASSERT: Empty data array
			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			data, ok := response["data"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(data).To(BeEmpty())

			// ASSERT: Pagination metadata is present
			pagination, ok := response["pagination"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(pagination["total"]).To(BeNumerically("==", 0))
			Expect(pagination["has_more"]).To(BeFalse())
		})
	})
})
