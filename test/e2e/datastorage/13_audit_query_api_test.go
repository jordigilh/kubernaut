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
	"fmt"
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
func createTestAuditEvent(baseURL, service, _ /* eventType */, correlationID string) error {
	// Build service-specific event data using ogen discriminated union types
	var eventData ogenclient.AuditEventRequestEventData
	var eventCategory ogenclient.AuditEventRequestEventCategory
	var eventType string

	switch service {
	case "gateway":
		eventData = ogenclient.AuditEventRequestEventData{
			Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
			GatewayAuditPayload: ogenclient.GatewayAuditPayload{
				EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
				SignalType:  ogenclient.GatewayAuditPayloadSignalTypeAlert,
				SignalName:   "TestAlert",
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
	BeforeEach(func() {
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
							SignalType:  ogenclient.GatewayAuditPayloadSignalTypeAlert,
							SignalName:   "TestAlert0",
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
			client, err := createOpenAPIClient(dataStorageURL)
			Expect(err).ToNot(HaveOccurred())

			baseTime := time.Now().Add(-5 * time.Second).UTC()
			for i, evt := range testEvents {
				eventRequest := ogenclient.AuditEventRequest{
					Version:        "1.0",
					EventCategory:  evt.category,
					EventType:      evt.eventType,
					EventTimestamp: baseTime.Add(time.Duration(i) * time.Second),
					CorrelationID:  correlationID,
					EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
					EventAction:    "test",
					EventData:      evt.eventData,
				}

				_, err := postAuditEvent(ctx, client, eventRequest)
				Expect(err).ToNot(HaveOccurred(), "Failed to create audit event: %s", evt.eventType)
			}

			// ACT: Query by correlation_id using typed OpenAPI client
			// Use Eventually() to wait for events to be visible (async buffer may delay persistence)
			var queryResp *ogenclient.AuditEventsQueryResponse
			Eventually(func() int {
				resp, err := DSClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					CorrelationID: ogenclient.NewOptString(correlationID),
				})
				if err != nil {
					return 0
				}
				queryResp = resp
				return len(resp.Data)
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(4), "should return all 4 events")

			// ASSERT: Events are in chronological order (DESC)
			for i := 0; i < len(queryResp.Data)-1; i++ {
				timestamp1 := queryResp.Data[i].EventTimestamp
				timestamp2 := queryResp.Data[i+1].EventTimestamp
				Expect(timestamp1.After(timestamp2) || timestamp1.Equal(timestamp2)).To(BeTrue(),
					"events should be in chronological order (DESC)")
			}

			// ASSERT: Pagination metadata is present
			Expect(queryResp.Pagination.IsSet()).To(BeTrue(), "response should have pagination metadata")
			Expect(queryResp.Pagination.Value.Limit.Value).To(BeNumerically("==", 50)) // Default limit per OpenAPI spec
			Expect(queryResp.Pagination.Value.Offset.Value).To(BeNumerically("==", 0))
			Expect(queryResp.Pagination.Value.Total.Value).To(BeNumerically("==", 4))
			Expect(queryResp.Pagination.Value.HasMore.Value).To(BeFalse())
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
							SignalType:  ogenclient.GatewayAuditPayloadSignalTypeAlert,
							SignalName:   "TestAlert-Gateway",
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
			client, err := createOpenAPIClient(dataStorageURL)
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

			// ACT: Query by event_type using typed OpenAPI client
			targetEventType := gateway.EventTypeSignalReceived
			queryResp, err := DSClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				EventType:     ogenclient.NewOptString(targetEventType),
			})
			Expect(err).ToNot(HaveOccurred())

			// ASSERT: Only events with matching event_type are returned
			Expect(queryResp.Data).To(HaveLen(3), "should return only 3 gateway.signal.received events")

			for _, event := range queryResp.Data {
				Expect(event.EventType).To(Equal(targetEventType))
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
					err := createTestAuditEvent(dataStorageURL, service, "test.event", correlationID)
					Expect(err).ToNot(HaveOccurred())
				}
			}

			// ACT: Query by event_category using typed OpenAPI client (ADR-034)
			targetService := "analysis" // ADR-034: Use "analysis" not "aianalysis"
			queryResp, err := DSClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				EventCategory: ogenclient.NewOptString(targetService),
			})
			Expect(err).ToNot(HaveOccurred())

			// ASSERT: Only events from target service are returned
			Expect(queryResp.Data).To(HaveLen(3), "should return only 3 analysis events")

			for _, event := range queryResp.Data {
				// ADR-034: Response uses event_category, not service
				Expect(string(event.EventCategory)).To(Equal(targetService))
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
				err := createTestAuditEvent(dataStorageURL, "gateway", "signal.received", correlationID)
				Expect(err).ToNot(HaveOccurred())
			}

			// ACT: Query with since=24h using typed OpenAPI client
			queryResp, err := DSClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				Since:         ogenclient.NewOptString("24h"),
			})
			Expect(err).ToNot(HaveOccurred())

			// ASSERT: All recent events are returned
			Expect(queryResp.Data).To(HaveLen(5), "should return all 5 recent events")

			// ASSERT: All events are within last 24 hours
			now := time.Now()
			for _, event := range queryResp.Data {
				Expect(now.Sub(event.EventTimestamp)).To(BeNumerically("<", 24*time.Hour))
			}
		})

		It("should return events within absolute time range (since/until)", func() {
			// DD-STORAGE-010: Time parsing (absolute: RFC3339)

			// ARRANGE: Insert events
			correlationID := generateTestID() // Unique per test for isolation
			for i := 0; i < 3; i++ {
				err := createTestAuditEvent(dataStorageURL, "gateway", "signal.received", correlationID)
				Expect(err).ToNot(HaveOccurred())
			}

			// ACT: Query with absolute time range using typed OpenAPI client
			now := time.Now()
			since := now.Add(-1 * time.Hour).Format(time.RFC3339)
			until := now.Add(1 * time.Hour).Format(time.RFC3339)
			queryResp, err := DSClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				Since:         ogenclient.NewOptString(since),
				Until:         ogenclient.NewOptString(until),
			})
			Expect(err).ToNot(HaveOccurred())

			// ASSERT: Events within time range are returned
			Expect(queryResp.Data).To(HaveLen(3))
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
			client, err := createOpenAPIClient(dataStorageURL)
			Expect(err).ToNot(HaveOccurred())

			for _, outcome := range outcomes {
				eventData := ogenclient.AuditEventRequestEventData{
					Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
					GatewayAuditPayload: ogenclient.GatewayAuditPayload{
						EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
						SignalType:  ogenclient.GatewayAuditPayloadSignalTypeAlert,
						SignalName:   "TestAlert",
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

			// ACT: Query with multiple filters using typed OpenAPI client (ADR-034 field names)
			queryResp, err := DSClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				EventCategory: ogenclient.NewOptString("gateway"),
				EventOutcome:  ogenclient.NewOptQueryAuditEventsEventOutcome(ogenclient.QueryAuditEventsEventOutcomeFailure),
			})
			Expect(err).ToNot(HaveOccurred())

			// ASSERT: Only events matching ALL filters are returned
			Expect(queryResp.Data).To(HaveLen(2), "should return only 2 failure events")

			for _, event := range queryResp.Data {
				// ADR-034: Response uses event_category and event_outcome
				Expect(string(event.EventCategory)).To(Equal("gateway"))
				Expect(string(event.EventOutcome)).To(Equal("failure"))
			}
		})
	})

	Context("Pagination", func() {
		var testCorrelationID string

		AfterEach(func() {
			// Cleanup test data
			if testCorrelationID != "" {
				_, err := testDB.ExecContext(ctx, "DELETE FROM audit_events WHERE correlation_id = $1", testCorrelationID)
				Expect(err).ToNot(HaveOccurred())
			}
		})

		It("should return correct subset with limit and offset", func() {
			// BR-STORAGE-023: Pagination support
			// DD-STORAGE-010: Offset-based pagination

			// ARRANGE: Insert 75 events via batch endpoint (DD-AUDIT-002)
			// Uses CreateBatch which acquires the advisory lock once per correlation_id
			// instead of 75 sequential single-event POSTs that serialize on the hash chain lock.
			testCorrelationID = generateTestID()
			correlationID := testCorrelationID

			events := make([]ogenclient.AuditEventRequest, 75)
			for i := range events {
				events[i] = ogenclient.AuditEventRequest{
					Version:       "1.0",
					EventCategory: ogenclient.AuditEventRequestEventCategoryGateway,
					EventType:     "gateway.signal.received",
					EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
					CorrelationID: correlationID,
					EventOutcome:  ogenclient.AuditEventRequestEventOutcomeSuccess,
					EventAction:   "test",
					EventData: ogenclient.AuditEventRequestEventData{
						Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
						GatewayAuditPayload: ogenclient.GatewayAuditPayload{
							EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
							SignalType:  ogenclient.GatewayAuditPayloadSignalTypeAlert,
							SignalName:  "TestAlert",
							Namespace:   "default",
							Fingerprint: "test-fingerprint",
						},
					},
				}
			}

			resp, err := DSClient.CreateAuditEventsBatch(ctx, events)
			Expect(err).ToNot(HaveOccurred(), "batch insert should succeed")
			Expect(resp.EventIds).To(HaveLen(75), "batch should return 75 event IDs")

			// Query page 1 — batch insert is synchronous, no Eventually needed
			var queryResp *ogenclient.AuditEventsQueryResponse
			queryResp, err = DSClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				EventCategory: ogenclient.NewOptString("gateway"),
				EventType:     ogenclient.NewOptString("gateway.signal.received"),
				Limit:         ogenclient.NewOptInt(50),
				Offset:        ogenclient.NewOptInt(0),
			})
			Expect(err).ToNot(HaveOccurred())

			// ASSERT: Correct subset is returned
			Expect(queryResp.Data).To(HaveLen(50), "should return 50 events (page 1)")

			// ASSERT: Pagination metadata is correct
			Expect(queryResp.Pagination.Value.Limit.Value).To(BeNumerically("==", 50))
			Expect(queryResp.Pagination.Value.Offset.Value).To(BeNumerically("==", 0))
			Expect(queryResp.Pagination.Value.Total.Value).To(BeNumerically(">=", 75),
				"should have at least 75 events for this correlation_id")
			Expect(queryResp.Pagination.Value.HasMore.Value).To(BeTrue())

			// ACT: Query page 2 (limit=50, offset=50) - use typed OpenAPI client with 3 filters
			queryResp2, err := DSClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				EventCategory: ogenclient.NewOptString("gateway"),
				EventType:     ogenclient.NewOptString("gateway.signal.received"),
				Limit:         ogenclient.NewOptInt(50),
				Offset:        ogenclient.NewOptInt(50),
			})
			Expect(err).ToNot(HaveOccurred())

			// ASSERT: Page 2 returns remaining events (25 events = 75 total - 50 offset)
			Expect(queryResp2.Data).To(HaveLen(25), "should return 25 remaining events (page 2)")

			Expect(queryResp2.Pagination.Value.Offset.Value).To(BeNumerically("==", 50))
			Expect(queryResp2.Pagination.Value.Total.Value).To(BeNumerically(">=", 75))
			Expect(queryResp2.Pagination.Value.HasMore.Value).To(BeFalse(), "no more pages after 75 events (50 + 25 = 75)")

			// ASSERT: Total events retrieved across 2 pages
			totalRetrieved := len(queryResp.Data) + len(queryResp2.Data)
			Expect(totalRetrieved).To(Equal(75), "should retrieve exactly 75 events across 2 pages")
		})
	})

	// NOTE: Pagination validation tests (limit, offset) belong in unit tests
	// See: test/unit/datastorage/handlers_test.go for RFC 7807 validation

	// NOTE: Time parsing validation tests belong in unit tests
	// See: test/unit/datastorage/handlers_test.go for RFC 7807 validation

	Context("Empty result set", func() {
		It("should return empty data array with pagination metadata", func() {
			// BR-STORAGE-021: Empty result handling

			// ACT: Query for non-existent correlation_id using typed client
			queryResp, err := DSClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString("rr-9999-999"),
			})
			Expect(err).ToNot(HaveOccurred())

			// ASSERT: Empty data array (not error)
			Expect(queryResp.Data).To(BeEmpty())

			// ASSERT: Pagination metadata is present
			Expect(queryResp.Pagination.IsSet()).To(BeTrue())
			Expect(queryResp.Pagination.Value.Total.Value).To(BeNumerically("==", 0))
			Expect(queryResp.Pagination.Value.HasMore.Value).To(BeFalse())
		})
	})
})
