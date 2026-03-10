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
	"fmt"
	"net/http"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// METRICS E2E TESTS
// Business Requirement: BR-STORAGE-019 (Logging and metrics)
// GAP-10: Audit-specific metrics in handlers
// ========================================
//
// These tests validate Prometheus metrics emission using the shared
// Podman PostgreSQL + Redis infrastructure from suite_test.go.
//
// Moved from test/unit/datastorage/server_metrics_integration_test.go
// because metrics testing requires real database operations.
// ========================================

var _ = Describe("BR-STORAGE-019: Prometheus Metrics Integration", Ordered, func() {
	// Local HTTP client for /metrics endpoint (Prometheus text format, not JSON/OpenAPI)
	var HTTPClient = &http.Client{Timeout: 10 * time.Second}
	// Use shared dataStorageURL and testDB from suite_test.go

	BeforeEach(func() {
		// Note: Metrics tests use unique timestamp-based correlation_ids
		// No cleanup needed - tests are isolated by correlation_id
	})

	Context("Metrics Endpoint", func() {
		It("should expose Prometheus metrics at /metrics", func() {
			// Business Outcome: Prometheus can scrape metrics from /metrics endpoint
			resp, err := HTTPClient.Get(dataStorageURL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(200),
				"Metrics endpoint MUST return 200 OK for Prometheus scraping")
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/plain"),
				"Metrics MUST be in Prometheus text format")

			// Business Outcome: Response contains standard Go metrics
			var body bytes.Buffer
			_, err = body.ReadFrom(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			metricsText := body.String()

			// Validate endpoint returns Prometheus format
			Expect(metricsText).To(ContainSubstring("go_goroutines"),
				"Metrics endpoint MUST expose standard Go runtime metrics")
			Expect(metricsText).To(ContainSubstring("# HELP"),
				"Metrics MUST be in Prometheus text format with help comments")
			Expect(metricsText).To(ContainSubstring("# TYPE"),
				"Metrics MUST be in Prometheus text format with type comments")

			// Note: External-facing metrics (datastorage_audit_lag_seconds, datastorage_write_duration_seconds)
			// only appear after they're emitted at least once. They're validated
			// in the "Handler Metrics Emission" tests below.
		})
	})

	Context("Handler Metrics Emission", func() {
		// BR-STORAGE-019: External-facing metrics (GitHub issue #294)
		// Implemented in: pkg/datastorage/server/audit_events_handler.go
		It("should emit audit_lag_seconds metric with calculated lag", func() {
			// Business Scenario: Audit event happened in the past
			// Expected: Metrics track audit lag for observability (time between event occurrence and write)

			// Create audit event with past timestamp (2 seconds ago)
			pastTimestamp := time.Now().UTC().Add(-2 * time.Second)

			eventData := ogenclient.AuditEventRequestEventData{
				Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
				GatewayAuditPayload: ogenclient.GatewayAuditPayload{
					EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
					SignalType:  ogenclient.GatewayAuditPayloadSignalTypeAlert,
					SignalName:   "LagTest",
					Namespace:   "default",
					Fingerprint: "test-fingerprint",
				},
			}

			eventRequest := ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventCategory:  ogenclient.AuditEventRequestEventCategoryGateway,
				EventType:      "gateway.signal.received",
				EventTimestamp: pastTimestamp,
				CorrelationID:  fmt.Sprintf("lag-test-%d", time.Now().UnixNano()),
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				EventAction:    "lag_test",
				EventData:      eventData,
			}

			// Use ogen client to post event (handles optional fields properly)
			ctx := context.Background()
			client, err := createOpenAPIClient(dataStorageURL)
			Expect(err).ToNot(HaveOccurred())

			_, err = postAuditEvent(ctx, client, eventRequest)
			Expect(err).ToNot(HaveOccurred())

			// Get metrics
			metricsResp, err := HTTPClient.Get(dataStorageURL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = metricsResp.Body.Close() }()

			var body bytes.Buffer
			_, err = body.ReadFrom(metricsResp.Body)
			Expect(err).ToNot(HaveOccurred())
			metricsText := body.String()

			// Business Outcome: audit_lag_seconds metric recorded
			Expect(metricsText).To(ContainSubstring("datastorage_audit_lag_seconds"),
				"audit_lag_seconds histogram MUST be present for observability")

			// Verify it's a histogram with samples
			// Prometheus format: datastorage_audit_lag_seconds_count{service="gateway"} N
			Expect(metricsText).To(MatchRegexp(`datastorage_audit_lag_seconds_count\{service="gateway"\} \d+`),
				"audit_lag_seconds MUST have recorded samples")
		})

		It("should emit write_duration metric for database operations", func() {
			// Business Scenario: Audit write operation
			// Expected: Metrics track database write performance
			// ADR-034: Use unified audit events endpoint

			// Create valid audit event
			eventData := ogenclient.AuditEventRequestEventData{
				Type: ogenclient.AuditEventRequestEventDataGatewaySignalReceivedAuditEventRequestEventData,
				GatewayAuditPayload: ogenclient.GatewayAuditPayload{
					EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
					SignalType:  ogenclient.GatewayAuditPayloadSignalTypeAlert,
					SignalName:   "DurationTest",
					Namespace:   "default",
					Fingerprint: "test-fingerprint",
				},
			}

			eventRequest := ogenclient.AuditEventRequest{
				Version:        "1.0",
				EventCategory:  ogenclient.AuditEventRequestEventCategoryGateway,
				EventType:      "gateway.signal.received",
				EventTimestamp: time.Now().Add(-5 * time.Second).UTC(),
				CorrelationID:  fmt.Sprintf("duration-test-%d", time.Now().UnixNano()),
				EventOutcome:   ogenclient.AuditEventRequestEventOutcomeSuccess,
				EventAction:    "duration_test",
				EventData:      eventData,
			}

			// Use ogen client to post event (handles optional fields properly)
			ctx := context.Background()
			client, err := createOpenAPIClient(dataStorageURL)
			Expect(err).ToNot(HaveOccurred())

			_, err = postAuditEvent(ctx, client, eventRequest)
			Expect(err).ToNot(HaveOccurred())

			// Get metrics
			metricsResp, err := HTTPClient.Get(dataStorageURL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = metricsResp.Body.Close() }()

			var body bytes.Buffer
			_, err = body.ReadFrom(metricsResp.Body)
			Expect(err).ToNot(HaveOccurred())
			metricsText := body.String()

			// Business Outcome: write_duration histogram recorded
			Expect(metricsText).To(ContainSubstring("datastorage_write_duration_seconds"),
				"write_duration histogram MUST track database performance")

			// Verify it's a histogram with samples for audit_events table (ADR-034 unified table)
			Expect(metricsText).To(MatchRegexp(`datastorage_write_duration_seconds_count\{table="audit_events"\} \d+`),
				"write_duration MUST have recorded samples for audit_events table (ADR-034)")
		})
	})
})
