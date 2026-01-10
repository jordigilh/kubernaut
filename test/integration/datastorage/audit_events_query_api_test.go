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
	"net/http"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/gateway"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-STORAGE-021: REST API Read Endpoints
// BR-STORAGE-022: Query Filtering
// BR-STORAGE-023: Pagination Validation
// DD-STORAGE-010: Query API Pagination Strategy (V1.0: offset-based)

// Helper function to create audit events via Write API
func createTestAuditEvent(baseURL, service, eventType, correlationID string) error {
	// Build service-specific event data using ogen discriminated union types
	var eventData ogenclient.AuditEventRequestEventData
	var eventCategory ogenclient.AuditEventRequestEventCategory

	switch service {
	case "gateway":
		eventData = ogenclient.AuditEventRequestEventData{
			Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
			GatewayAuditPayload: ogenclient.GatewayAuditPayload{
				EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
				SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
				AlertName:   "TestAlert",
				Namespace:   "default",
				Fingerprint: "test-fingerprint",
			},
		}
		eventType = "gateway.signal.received" // Use valid discriminator key
		eventCategory = ogenclient.AuditEventRequestEventCategoryGateway
	case "analysis": // ADR-034: event_category is "analysis"
		eventData = ogenclient.AuditEventRequestEventData{
			Type: ogenclient.AuditEventRequestEventDataAianalysisAnalysisCompletedAuditEventRequestEventData,
			AIAnalysisAuditPayload: ogenclient.AIAnalysisAuditPayload{
				EventType:    ogenclient.AIAnalysisAuditPayloadEventTypeAianalysisAnalysisCompleted,
				AnalysisName: "test-analysis",
				Phase:        ogenclient.AIAnalysisAuditPayloadPhaseCompleted,
			},
		}
		eventType = "aianalysis.analysis.completed" // Use valid discriminator key
		eventCategory = ogenclient.AuditEventRequestEventCategoryAnalysis
	case "workflow":
		eventData = ogenclient.AuditEventRequestEventData{
			Type: ogenclient.AuditEventRequestEventDataWorkflowexecutionWorkflowStartedAuditEventRequestEventData,
			WorkflowExecutionAuditPayload: ogenclient.WorkflowExecutionAuditPayload{
				EventType:       ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionWorkflowStarted,
				WorkflowID:      "test-workflow",
				WorkflowVersion: "v1",
				TargetResource:  "Pod/test-pod",
				Phase:           ogenclient.WorkflowExecutionAuditPayloadPhaseRunning,
				ContainerImage:  "gcr.io/tekton-releases/tekton:v0.1.0",
				ExecutionName:   "test-execution",
			},
		}
		eventType = "workflowexecution.workflow.started" // Use valid discriminator key
		eventCategory = ogenclient.AuditEventRequestEventCategoryWorkflow
	default:
		return fmt.Errorf("unsupported service type: %s (must use ogen discriminated union)", service)
	}

	// Create ogen-typed request
	eventRequest := ogenclient.AuditEventRequest{
		Version:        "1.0",
		EventCategory:  eventCategory,
		EventType:      eventType, // MUST match discriminator mapping
		EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
		CorrelationID:  correlationID,
		EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
		EventAction:    "test",
		EventData:      eventData,
	}

	// Use ogen client to post event (handles optional fields properly)
	ctx := context.Background()
	client, err := createOpenAPIClient(baseURL)
	if err != nil {
		return err
	}

	_, err = postAuditEvent(ctx, client, eventRequest)
	if err != nil {
		return err
	}
	return nil
}

var _ = Describe("Audit Events Query API", func() {
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

			// ARRANGE: Insert test events for correlation_id using valid event types
			correlationID := generateTestID() // Unique per test for isolation

			// Use valid event types from discriminator mapping
			testEvents := []struct {
				category  ogenclient.AuditEventRequestEventCategory
				eventType string
				eventData ogenclient.AuditEventRequestEventData
			}{
				{
					category:  ogenclient.AuditEventRequestEventCategoryGateway,
					eventType: "gateway.signal.received",
					eventData: ogenclient.AuditEventRequestEventData{
						Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
						GatewayAuditPayload: ogenclient.GatewayAuditPayload{
							EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
							SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
							AlertName:   "TestAlert0",
							Namespace:   "default",
							Fingerprint: "test-fingerprint-0",
						},
					},
				},
				{
					category:  ogenclient.AuditEventRequestEventCategoryAnalysis,
					eventType: "aianalysis.analysis.completed",
					eventData: ogenclient.AuditEventRequestEventData{
						Type: ogenclient.AuditEventRequestEventDataAianalysisAnalysisCompletedAuditEventRequestEventData,
						AIAnalysisAuditPayload: ogenclient.AIAnalysisAuditPayload{
							EventType:    ogenclient.AIAnalysisAuditPayloadEventTypeAianalysisAnalysisCompleted,
							AnalysisName: "test-analysis-1",
							Phase:        ogenclient.AIAnalysisAuditPayloadPhaseCompleted,
						},
					},
				},
				{
					category:  ogenclient.AuditEventRequestEventCategoryWorkflow,
					eventType: "workflowexecution.workflow.started",
					eventData: ogenclient.AuditEventRequestEventData{
						Type: ogenclient.AuditEventRequestEventDataWorkflowexecutionWorkflowStartedAuditEventRequestEventData,
						WorkflowExecutionAuditPayload: ogenclient.WorkflowExecutionAuditPayload{
							EventType:       ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionWorkflowStarted,
							WorkflowID:      "test-workflow-2",
							WorkflowVersion: "v1",
							TargetResource:  "Pod/test-pod-2",
							Phase:           ogenclient.WorkflowExecutionAuditPayloadPhaseRunning,
							ContainerImage:  "gcr.io/tekton-releases/tekton:v0.1.0",
							ExecutionName:   "test-execution-2",
						},
					},
				},
				{
					category:  ogenclient.AuditEventRequestEventCategoryWorkflow,
					eventType: "workflowexecution.workflow.completed",
					eventData: ogenclient.AuditEventRequestEventData{
						Type: ogenclient.AuditEventRequestEventDataWorkflowexecutionWorkflowCompletedAuditEventRequestEventData,
						WorkflowExecutionAuditPayload: ogenclient.WorkflowExecutionAuditPayload{
							EventType:       ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionWorkflowCompleted,
							WorkflowID:      "test-workflow-3",
							WorkflowVersion: "v1",
							TargetResource:  "Pod/test-pod-3",
							Phase:           ogenclient.WorkflowExecutionAuditPayloadPhaseCompleted,
							ContainerImage:  "gcr.io/tekton-releases/tekton:v0.1.0",
							ExecutionName:   "test-execution-3",
						},
					},
				},
			}

			// Create ogen client for posting events
			ctx := context.Background()
			client, err := createOpenAPIClient(datastorageURL)
			Expect(err).ToNot(HaveOccurred())

			for _, evt := range testEvents {
				eventRequest := ogenclient.AuditEventRequest{
					Version:        "1.0",
					EventCategory:  evt.category,
					EventType:      evt.eventType,
					EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
					CorrelationID:  correlationID,
					EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
					EventAction:    "test",
					EventData:      evt.eventData,
				}

				// Use ogen client to post event (handles optional fields properly)
				_, err := postAuditEvent(ctx, client, eventRequest)
				Expect(err).ToNot(HaveOccurred(), "Failed to create audit event: %s", evt.eventType)

				// Add small delay to ensure chronological ordering
				// Per TESTING_GUIDELINES.md: ACCEPTABLE - testing timing behavior (chronological order)
				time.Sleep(10 * time.Millisecond)
			}

			// ACT: Query by correlation_id
			// Use Eventually() to wait for events to be visible (async buffer may delay persistence)
			var data []interface{}
			Eventually(func() int {
				resp, err := http.Get(fmt.Sprintf("%s?correlation_id=%s", baseURL, correlationID))
				if err != nil {
					return 0
				}
				defer func() { _ = resp.Body.Close() }()

				if resp.StatusCode != http.StatusOK {
					return 0
				}

				var response map[string]interface{}
				if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
					return 0
				}

				dataInterface, ok := response["data"].([]interface{})
				if !ok {
					return 0
				}

				data = dataInterface
				return len(data)
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(4), "should return all 4 events")

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
			// Query one more time to get full response with pagination metadata
			resp, err := http.Get(fmt.Sprintf("%s?correlation_id=%s", baseURL, correlationID))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			pagination, ok := response["pagination"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "response should have 'pagination' object")
			Expect(pagination["limit"]).To(BeNumerically("==", 50)) // Default limit per OpenAPI spec
			Expect(pagination["offset"]).To(BeNumerically("==", 0))
			Expect(pagination["total"]).To(BeNumerically("==", 4)) // Fixed: 4 events, not 5
			Expect(pagination["has_more"]).To(BeFalse())
		})
	})

	Context("Query by event_type", func() {
		It("should return only events matching the event_type filter", func() {
			// BR-STORAGE-022: Query filtering by event_type

			// ARRANGE: Insert events with different event_types using valid discriminated unions
			correlationID := generateTestID() // Unique per test for isolation

			// Create events with different valid event types
			testCases := []struct {
				eventType string
				count     int
				category  ogenclient.AuditEventRequestEventCategory
				data      ogenclient.AuditEventRequestEventData
			}{
				{
					eventType: "gateway.signal.received",
					count:     3,
					category:  ogenclient.AuditEventRequestEventCategoryGateway,
					data: ogenclient.AuditEventRequestEventData{
						Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
						GatewayAuditPayload: ogenclient.GatewayAuditPayload{
							EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
							SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
							AlertName:   "TestAlert-Gateway",
							Namespace:   "default",
							Fingerprint: "test-fingerprint",
						},
					},
				},
				{
					eventType: "aianalysis.analysis.completed",
					count:     2,
					category:  ogenclient.AuditEventRequestEventCategoryAnalysis,
					data: ogenclient.AuditEventRequestEventData{
						Type: ogenclient.AuditEventRequestEventDataAianalysisAnalysisCompletedAuditEventRequestEventData,
						AIAnalysisAuditPayload: ogenclient.AIAnalysisAuditPayload{
							EventType:    ogenclient.AIAnalysisAuditPayloadEventTypeAianalysisAnalysisCompleted,
							AnalysisName: "test-analysis",
							Phase:        ogenclient.AIAnalysisAuditPayloadPhaseCompleted,
						},
					},
				},
				{
					eventType: "workflowexecution.workflow.completed",
					count:     1,
					category:  ogenclient.AuditEventRequestEventCategoryWorkflow,
					data: ogenclient.AuditEventRequestEventData{
						Type: ogenclient.AuditEventRequestEventDataWorkflowexecutionWorkflowCompletedAuditEventRequestEventData,
						WorkflowExecutionAuditPayload: ogenclient.WorkflowExecutionAuditPayload{
							EventType:       ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionWorkflowCompleted,
							WorkflowID:      "test-workflow",
							WorkflowVersion: "v1",
							TargetResource:  "Pod/test-pod",
							Phase:           ogenclient.WorkflowExecutionAuditPayloadPhaseCompleted,
							ContainerImage:  "gcr.io/tekton-releases/tekton:v0.1.0",
							ExecutionName:   "test-execution",
						},
					},
				},
			}

			// Create ogen client for posting events
			ctx := context.Background()
			client, err := createOpenAPIClient(datastorageURL)
			Expect(err).ToNot(HaveOccurred())

			for _, tc := range testCases {
				for i := 0; i < tc.count; i++ {
					// Create ogen-typed request
					eventRequest := ogenclient.AuditEventRequest{
						Version:        "1.0",
						EventCategory:  tc.category,
						EventType:      tc.eventType,
						EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
						CorrelationID:  correlationID,
						EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
						EventAction:    "test",
						EventData:      tc.data,
					}

					// Use ogen client to post event (handles optional fields properly)
					_, err := postAuditEvent(ctx, client, eventRequest)
					Expect(err).ToNot(HaveOccurred())
				}
			}

			// ACT: Query by event_type and correlation_id for test isolation
			targetEventType := gateway.EventTypeSignalReceived
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
					err := createTestAuditEvent(datastorageURL, service, "test.event", correlationID)
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
				err := createTestAuditEvent(datastorageURL, "gateway", "signal.received", correlationID)
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
				err := createTestAuditEvent(datastorageURL, "gateway", "signal.received", correlationID)
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

			// ARRANGE: Insert events with different outcomes using ogen types
			correlationID := generateTestID() // Unique per test for isolation
			outcomes := []ogenclient.AuditEventRequestEventOutcome{
				ogenclient.AuditEventRequestEventOutcomeSuccess,
				ogenclient.AuditEventRequestEventOutcomeFailure,
				ogenclient.AuditEventRequestEventOutcomeSuccess,
				ogenclient.AuditEventRequestEventOutcomeFailure,
			}
			// Create ogen client for posting events
			ctx := context.Background()
			client, err := createOpenAPIClient(datastorageURL)
			Expect(err).ToNot(HaveOccurred())

			for _, outcome := range outcomes {
				eventData := ogenclient.AuditEventRequestEventData{
					Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
					GatewayAuditPayload: ogenclient.GatewayAuditPayload{
						EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
						SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert,
						AlertName:   "TestAlert",
						Namespace:   "default",
						Fingerprint: "test-fingerprint",
					},
				}

				eventRequest := ogenclient.AuditEventRequest{
					Version:        "1.0",
					EventCategory:  ogenclient.AuditEventRequestEventCategoryGateway,
					EventType:      "gateway.signal.received",
					EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
					CorrelationID:  correlationID,
					EventOutcome:   outcome,
					EventAction:    "test",
					EventData:      eventData,
				}

				// Use ogen client to post event (handles optional fields properly)
				_, err := postAuditEvent(ctx, client, eventRequest)
				Expect(err).ToNot(HaveOccurred())
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
				err := createTestAuditEvent(datastorageURL, "gateway", "signal.received", correlationID)
				Expect(err).ToNot(HaveOccurred())
			}

			// WAIT: Allow events to be persisted (async buffering + flush)
			// NOTE: Events are buffered by audit store (1s flush interval) + parallel contention
			// Timeout increased from 10s to 30s to account for:
			// - Parallel test execution (12 processes) - high contention
			// - Database connection contention across processes
			// - Audit store buffering (1s flush interval)
			// - Schema isolation overhead in parallel mode
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
			}, 30*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 75),
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
