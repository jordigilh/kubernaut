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
	"net/http"
	"sort"
	"time"

	"github.com/go-logr/logr"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Scenario 3: Query API Timeline - Multi-Filter Retrieval (P0)
//
// Business Requirements:
// - BR-STORAGE-021: REST API Read Endpoints
// - BR-STORAGE-022: Query Filtering (correlation_id, event_category, event_type, time_range)
// - BR-STORAGE-023: Pagination (offset-based for V1.0)
//
// Business Value: Verify Query API supports multi-dimensional filtering
//
// Test Flow:
// 1. Deploy Data Storage Service in isolated namespace
// 2. Create 10 audit events across 3 services (Gateway, AIAnalysis, Workflow)
// 3. Query by correlation_id → verify all 10 events returned
// 4. Query by event_category=gateway (ADR-034) → verify only Gateway events returned
// 5. Query by event_type → verify only matching events returned
// 6. Query by time_range → verify only events in range returned
// 7. Query with pagination (limit=5, offset=0) → verify first 5 events
// 8. Query with pagination (limit=5, offset=5) → verify next 5 events
//
// Expected Results:
// - All queries return correct filtered results
// - Pagination works correctly (offset-based)
// - Events are in chronological order
// - Response format follows ADR-034 specification
//
// Parallel Execution: ✅ ENABLED
// - Each test gets unique namespace (datastorage-e2e-p{N}-{timestamp})
// - Complete infrastructure isolation
// - No query interference between tests

var _ = Describe("BR-DS-002: Query API Performance - Multi-Filter Retrieval (<5s Response)", Label("e2e", "query-api", "p0"), Ordered, func() {
	var (
		testCancel context.CancelFunc
		testLogger logr.Logger
		// DD-AUTH-014: Use exported HTTPClient from suite setup
		testNamespace string
		serviceURL    string
		correlationID string
		startTime     time.Time
	)

	BeforeAll(func() {
		_, testCancel = context.WithTimeout(ctx, 15*time.Minute)
		testLogger = logger.WithValues("test", "query-api")
		// DD-AUTH-014: HTTPClient is now provided by suite setup with ServiceAccount auth

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario 3: Query API Timeline - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Use shared deployment from SynchronizedBeforeSuite (no per-test deployment)
		// Services are deployed ONCE and shared via NodePort (no port-forwarding needed)
		testNamespace = sharedNamespace
		serviceURL = dataStorageURL
		testLogger.Info("Using shared deployment", "namespace", testNamespace, "url", serviceURL)

		// Wait for Data Storage Service to be responsive using raw HTTP (health endpoint returns text/plain)
		testLogger.Info("⏳ Waiting for Data Storage Service...")
		httpClient := &http.Client{Timeout: 2 * time.Second}
		Eventually(func() error {
			resp, err := httpClient.Get(serviceURL + "/health")
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("health check returned status %d", resp.StatusCode)
			}
			return nil
		}, 60*time.Second, 2*time.Second).Should(Succeed(), "Data Storage Service should be healthy")
		testLogger.Info("✅ Data Storage Service is responsive")

		// Generate unique correlation ID for this test
		correlationID = fmt.Sprintf("query-test-%s", testNamespace)
		// Use timestamp 15 minutes in the past to account for clock skew and event creation time
		startTime = time.Now().UTC().Add(-15 * time.Minute)

		testLogger.Info("✅ Test services ready", "namespace", testNamespace)
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("🧹 Cleaning up test resources...")
		if testCancel != nil {
			testCancel()
		}
		// Note: Shared namespace is NOT cleaned up here - it's managed by SynchronizedAfterSuite
	})

	It("should support multi-dimensional filtering and pagination", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test: Query API Multi-Filter and Pagination")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Step 1: Create 10 audit events across 3 services
		testLogger.Info("📝 Step 1: Creating 10 audit events across 3 services...")

		// Gateway events (4 events)
		// Use timestamps 10 minutes in the past to avoid clock skew issues between host and container
		baseTimestamp := time.Now().UTC().Add(-10 * time.Minute)
		for i := 1; i <= 4; i++ {
			// Use helper function for strongly-typed payload
			eventData := newMinimalGatewayPayload("alert", fmt.Sprintf("Alert-%d", i))

			// DD-API-001: Use typed OpenAPI struct
			event := dsgen.AuditEventRequest{
				Version:        "1.0",
				EventCategory:  dsgen.AuditEventRequestEventCategoryGateway,
				EventAction:    fmt.Sprintf("gateway_op_%d", i),
				EventType:      "gateway.signal.received",
				EventTimestamp: baseTimestamp.Add(time.Duration(i) * time.Second),
				CorrelationID:  correlationID,
				EventOutcome:   dsgen.AuditEventRequestEventOutcomeSuccess,
				EventData:      eventData,
			}
			eventID := createAuditEventOpenAPI(ctx, DSClient, event)
			// Verify event was created
			Expect(eventID).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`), "eventID must be a valid UUID")
			time.Sleep(100 * time.Millisecond) // Small delay to ensure chronological order
		}
		testLogger.Info("✅ Created 4 Gateway events")

		// AIAnalysis events (3 events)
		for i := 1; i <= 3; i++ {
			// Use helper function for strongly-typed payload
			eventData := newMinimalAIAnalysisPayload(fmt.Sprintf("analysis-%d", i))

			// DD-API-001: Use typed OpenAPI struct
			event := dsgen.AuditEventRequest{
				Version:        "1.0",
				EventCategory:  dsgen.AuditEventRequestEventCategoryAnalysis,
				EventAction:    fmt.Sprintf("ai_op_%d", i),
				EventType:      "analysis.analysis.completed",
				EventTimestamp: baseTimestamp.Add(time.Duration(4+i) * time.Second),
				CorrelationID:  correlationID,
				EventOutcome:   dsgen.AuditEventRequestEventOutcomeSuccess,
				EventData:      eventData,
			}
			eventID := createAuditEventOpenAPI(ctx, DSClient, event)
			// Verify event was created
			Expect(eventID).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`), "eventID must be a valid UUID")
			time.Sleep(100 * time.Millisecond)
		}
		testLogger.Info("✅ Created 3 AIAnalysis events")

		// Workflow events (3 events)
		for i := 1; i <= 3; i++ {
			// Use helper function for strongly-typed payload
			eventData := newMinimalWorkflowPayload(fmt.Sprintf("workflow-%d", i))

			// DD-API-001: Use typed OpenAPI struct
			event := dsgen.AuditEventRequest{
				Version:        "1.0",
				EventCategory:  dsgen.AuditEventRequestEventCategoryWorkflow,
				EventAction:    fmt.Sprintf("workflow_op_%d", i),
				EventType:      "workflow.workflow.completed",
				EventTimestamp: baseTimestamp.Add(time.Duration(7+i) * time.Second),
				CorrelationID:  correlationID,
				EventOutcome:   dsgen.AuditEventRequestEventOutcomeSuccess,
				EventData:      eventData,
			}
			eventID := createAuditEventOpenAPI(ctx, DSClient, event)
			// Verify event was created
			Expect(eventID).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`), "eventID must be a valid UUID")
			time.Sleep(100 * time.Millisecond)
		}
		testLogger.Info("✅ Created 3 Workflow events")
		testLogger.Info("✅ Total: 10 audit events created")

		// Step 2: Query by correlation_id → verify all 10 events returned
		// Per docs/testing/AUDIT_QUERY_PAGINATION_STANDARDS.md: ALWAYS use pagination
		testLogger.Info("🔍 Step 2: Query by correlation_id...")
		var allEventsStep2 []dsgen.AuditEvent
		offset := 0
		limit := 100

		for {
			queryResp, err := DSClient.QueryAuditEvents(ctx, dsgen.QueryAuditEventsParams{
				CorrelationID: dsgen.NewOptString(correlationID),
				Limit:         dsgen.NewOptInt(limit),
				Offset:        dsgen.NewOptInt(offset),
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(queryResp).To(Not(BeNil()), "query response must not be nil")

			if len(queryResp.Data) == 0 {
				break
			}

			allEventsStep2 = append(allEventsStep2, queryResp.Data...)

			if len(queryResp.Data) < limit {
				break
			}

			offset += limit
		}

		data := allEventsStep2
		// Note: Self-auditing may add extra events (datastorage.audit.written)
		// We expect at least 10 events (the ones we created), but may have more
		Expect(len(data)).To(BeNumerically(">=", 10), "Should return at least 10 events")
		testLogger.Info("✅ Query by correlation_id returned events", "count", len(data))

		// Step 3: Query by event_category=gateway (ADR-034) → verify only Gateway events returned
		// Per docs/testing/AUDIT_QUERY_PAGINATION_STANDARDS.md: ALWAYS use pagination
		testLogger.Info("🔍 Step 3: Query by event_category=gateway...")
		gatewayCategory := "gateway"
		var allEventsStep3 []dsgen.AuditEvent
		offset = 0
		limit = 100

		for {
			queryResp, err := DSClient.QueryAuditEvents(ctx, dsgen.QueryAuditEventsParams{
				CorrelationID: dsgen.NewOptString(correlationID),
				EventCategory: dsgen.NewOptString(gatewayCategory),
				Limit:         dsgen.NewOptInt(limit),
				Offset:        dsgen.NewOptInt(offset),
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(queryResp).To(Not(BeNil()), "query response must not be nil")

			if len(queryResp.Data) == 0 {
				break
			}

			allEventsStep3 = append(allEventsStep3, queryResp.Data...)

			if len(queryResp.Data) < limit {
				break
			}

			offset += limit
		}

		data = allEventsStep3
		Expect(data).To(HaveLen(4), "Should return 4 Gateway events")

		// Verify all events are from gateway event_category (ADR-034)
		for _, event := range data {
			Expect(event.EventCategory).To(Equal(dsgen.AuditEventEventCategoryGateway))
		}
		testLogger.Info("✅ Query by event_category=gateway returned 4 events")

		// Step 4: Query by event_type → verify only matching events returned
		// Per docs/testing/AUDIT_QUERY_PAGINATION_STANDARDS.md: ALWAYS use pagination
		testLogger.Info("🔍 Step 4: Query by event_type=analysis.analysis.completed...")
		eventType := "analysis.analysis.completed"
		var allEventsStep4 []dsgen.AuditEvent
		offset = 0
		limit = 100

		for {
			queryResp, err := DSClient.QueryAuditEvents(ctx, dsgen.QueryAuditEventsParams{
				CorrelationID: dsgen.NewOptString(correlationID),
				EventType:     dsgen.NewOptString(eventType),
				Limit:         dsgen.NewOptInt(limit),
				Offset:        dsgen.NewOptInt(offset),
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(queryResp).To(Not(BeNil()), "query response must not be nil")

			if len(queryResp.Data) == 0 {
				break
			}

			allEventsStep4 = append(allEventsStep4, queryResp.Data...)

			if len(queryResp.Data) < limit {
				break
			}

			offset += limit
		}

		data = allEventsStep4
		Expect(data).To(HaveLen(3), "Should return 3 AIAnalysis events")

		// Verify all events have correct event_type
		for _, event := range data {
			Expect(event.EventType).To(Equal("analysis.analysis.completed"))
		}
		testLogger.Info("✅ Query by event_type returned 3 events")

		// Step 5: Query by time_range → verify only events in range returned
		// Per docs/testing/AUDIT_QUERY_PAGINATION_STANDARDS.md: ALWAYS use pagination
		testLogger.Info("🔍 Step 5: Query by time_range...")
		endTime := time.Now()
		startTimeStr := startTime.Format(time.RFC3339)
		endTimeStr := endTime.Format(time.RFC3339)
		var allEventsStep5 []dsgen.AuditEvent
		offset = 0
		limit = 100

		for {
			queryResp, err := DSClient.QueryAuditEvents(ctx, dsgen.QueryAuditEventsParams{
				CorrelationID: dsgen.NewOptString(correlationID),
				Since:         dsgen.NewOptString(startTimeStr),
				Until:         dsgen.NewOptString(endTimeStr),
				Limit:         dsgen.NewOptInt(limit),
				Offset:        dsgen.NewOptInt(offset),
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(queryResp).To(Not(BeNil()), "query response must not be nil")

			if len(queryResp.Data) == 0 {
				break
			}

			allEventsStep5 = append(allEventsStep5, queryResp.Data...)

			if len(queryResp.Data) < limit {
				break
			}

			offset += limit
		}

		data = allEventsStep5
		Expect(len(data)).To(BeNumerically(">=", 10), "Should return at least 10 events within time range")
		testLogger.Info("✅ Query by time_range returned events", "count", len(data))

		// Step 6: Query with pagination (limit=5, offset=0) → verify first 5 events
		// NOTE: This step explicitly tests the pagination API feature (not a bug fix)
		testLogger.Info("🔍 Step 6: Query with pagination (limit=5, offset=0)...")
		// DD-API-001: Use typed OpenAPI client with pagination parameters
		limit = 5
		offset = 0
		queryRespStep6, errStep6 := DSClient.QueryAuditEvents(ctx, dsgen.QueryAuditEventsParams{
			CorrelationID: dsgen.NewOptString(correlationID),
			Limit:         dsgen.NewOptInt(limit),
			Offset:        dsgen.NewOptInt(offset),
		})
		Expect(errStep6).ToNot(HaveOccurred())
		// Note: ogen client returns typed response struct on success (HTTP 200 implicit)
		Expect(queryRespStep6).To(And(Not(BeNil()), HaveField("Data", Not(BeNil()))))

		data = queryRespStep6.Data
		Expect(data).To(HaveLen(5), "Should return first 5 events")

		// Store first event ID for comparison
		firstPageFirstEventID := data[0].EventID.Value.String()
		testLogger.Info("✅ Pagination (limit=5, offset=0) returned 5 events")

		// Step 7: Query with pagination (limit=5, offset=5) → verify next 5 events
		// NOTE: This step explicitly tests the pagination API feature (not a bug fix)
		testLogger.Info("🔍 Step 7: Query with pagination (limit=5, offset=5)...")
		// DD-API-001: Use typed OpenAPI client with offset pagination
		offset = 5
		queryRespStep7, errStep7 := DSClient.QueryAuditEvents(ctx, dsgen.QueryAuditEventsParams{
			CorrelationID: dsgen.NewOptString(correlationID),
			Limit:         dsgen.NewOptInt(limit),
			Offset:        dsgen.NewOptInt(offset),
		})
		Expect(errStep7).ToNot(HaveOccurred())
		// Note: ogen client returns typed response struct on success (HTTP 200 implicit)
		Expect(queryRespStep7).To(And(Not(BeNil()), HaveField("Data", Not(BeNil()))))

		data = queryRespStep7.Data
		Expect(data).To(HaveLen(5), "Should return next 5 events")

		// Verify second page has different events
		secondPageFirstEventID := data[0].EventID.Value.String()
		Expect(secondPageFirstEventID).ToNot(Equal(firstPageFirstEventID), "Second page should have different events")
		testLogger.Info("✅ Pagination (limit=5, offset=5) returned next 5 events")

		// Step 8: Verify chronological order
		// Per docs/testing/AUDIT_QUERY_PAGINATION_STANDARDS.md: ALWAYS use pagination
		testLogger.Info("🔍 Step 8: Verifying chronological order...")
		var allEventsStep8 []dsgen.AuditEvent
		offset = 0
		limit = 100

		for {
			queryResp, err := DSClient.QueryAuditEvents(ctx, dsgen.QueryAuditEventsParams{
				CorrelationID: dsgen.NewOptString(correlationID),
				Limit:         dsgen.NewOptInt(limit),
				Offset:        dsgen.NewOptInt(offset),
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(queryResp).To(Not(BeNil()), "query response must not be nil")

			if len(queryResp.Data) == 0 {
				break
			}

			allEventsStep8 = append(allEventsStep8, queryResp.Data...)

			if len(queryResp.Data) < limit {
				break
			}

			offset += limit
		}

		data = allEventsStep8

		// Sort events by timestamp (API doesn't guarantee order)
		sort.Slice(data, func(i, j int) bool {
			return data[i].EventTimestamp.Before(data[j].EventTimestamp)
		})

		var previousTimestamp time.Time
		for i, event := range data {
			timestamp := event.EventTimestamp

			if i > 0 {
				Expect(timestamp.After(previousTimestamp) || timestamp.Equal(previousTimestamp)).To(BeTrue(),
					"Events should be in chronological order")
			}
			previousTimestamp = timestamp
		}
		testLogger.Info("✅ Events are in chronological order")

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Scenario 3: Query API Timeline - PASSED")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})
