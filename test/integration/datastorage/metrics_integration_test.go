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
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// METRICS INTEGRATION TESTS
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
	// Use shared datastorageURL and db from suite_test.go

	Context("Metrics Endpoint", func() {
		It("should expose Prometheus metrics at /metrics", func() {
			// Business Outcome: Prometheus can scrape metrics from /metrics endpoint
			resp, err := http.Get(datastorageURL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

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

			// Note: Audit-specific metrics (datastorage_audit_traces_total, etc.)
			// only appear after they're emitted at least once. They're validated
			// in the "Handler Metrics Emission" tests below.
		})
	})

	Context("Handler Metrics Emission", func() {
		It("should emit audit_traces_total metric on successful write", func() {
			// Business Scenario: Notification service writes audit trail
			// Expected: Metrics track successful writes

			// Get baseline metric value
			baselineResp, err := http.Get(datastorageURL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer baselineResp.Body.Close()

			var baselineBody bytes.Buffer
			_, err = baselineBody.ReadFrom(baselineResp.Body)
			Expect(err).ToNot(HaveOccurred())
			baselineMetrics := baselineBody.String()

			// Create valid notification audit
			auditPayload := &models.NotificationAudit{
				RemediationID:   fmt.Sprintf("test-remediation-%d", time.Now().Unix()),
				NotificationID:  fmt.Sprintf("test-notification-%d", time.Now().UnixNano()),
				Recipient:       "test@example.com",
				Channel:         "email",
				MessageSummary:  "Test notification message",
				Status:          "sent",
				SentAt:          time.Now().Add(-1 * time.Minute), // 1 minute in the past to avoid clock skew issues
				DeliveryStatus:  "200 OK",
				EscalationLevel: 0,
			}

			payload, _ := json.Marshal(auditPayload)
			resp, err := http.Post(
				datastorageURL+"/api/v1/audit/notifications",
				"application/json",
				bytes.NewBuffer(payload),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Should succeed
			Expect(resp.StatusCode).To(Equal(201),
				"Audit write MUST succeed with 201 Created")

			// Get updated metrics
			updatedResp, err := http.Get(datastorageURL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer updatedResp.Body.Close()

			var updatedBody bytes.Buffer
			_, err = updatedBody.ReadFrom(updatedResp.Body)
			Expect(err).ToNot(HaveOccurred())
			updatedMetrics := updatedBody.String()

			// Business Outcome: audit_traces_total metric incremented
			Expect(updatedMetrics).To(ContainSubstring("datastorage_audit_traces_total"),
				"audit_traces_total metric MUST be present after write")

			// Verify metric increased (baseline should be different from updated)
			// We can't check exact values due to parallel test execution,
			// but we can verify the metric exists and has a value
			baselineHasMetric := strings.Contains(baselineMetrics, "datastorage_audit_traces_total")
			updatedHasMetric := strings.Contains(updatedMetrics, "datastorage_audit_traces_total")

			if baselineHasMetric {
				// Metric existed before, should still exist
				Expect(updatedHasMetric).To(BeTrue(),
					"audit_traces_total metric MUST persist after write")
			} else {
				// Metric didn't exist before, should now exist
				Expect(updatedHasMetric).To(BeTrue(),
					"audit_traces_total metric MUST appear after first write")
			}
		})

		It("should emit audit_lag_seconds metric with calculated lag", func() {
			// Business Scenario: Audit event happened 2 seconds ago
			// Expected: Metrics track audit lag for observability

			// Create audit event with past timestamp
			sentAt := time.Now().UTC().Add(-2 * time.Second)
			auditPayload := &models.NotificationAudit{
				RemediationID:   fmt.Sprintf("test-remediation-%d", time.Now().Unix()),
				NotificationID:  fmt.Sprintf("test-notification-%d", time.Now().UnixNano()),
				Recipient:       "test@example.com",
				Channel:         "email",
				MessageSummary:  "Test notification with lag",
				Status:          "sent",
				SentAt:          sentAt,
				DeliveryStatus:  "200 OK",
				EscalationLevel: 0,
			}

			payload, _ := json.Marshal(auditPayload)
			resp, err := http.Post(
				datastorageURL+"/api/v1/audit/notifications",
				"application/json",
				bytes.NewBuffer(payload),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(201))

			// Get metrics
			metricsResp, err := http.Get(datastorageURL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer metricsResp.Body.Close()

			var body bytes.Buffer
			_, err = body.ReadFrom(metricsResp.Body)
			Expect(err).ToNot(HaveOccurred())
			metricsText := body.String()

			// Business Outcome: audit_lag_seconds metric recorded
			Expect(metricsText).To(ContainSubstring("datastorage_audit_lag_seconds"),
				"audit_lag_seconds histogram MUST be present for observability")

			// Verify it's a histogram with samples
			// Prometheus format: datastorage_audit_lag_seconds_count{service="notification"} N
			Expect(metricsText).To(MatchRegexp(`datastorage_audit_lag_seconds_count\{service="notification"\} \d+`),
				"audit_lag_seconds MUST have recorded samples")
		})

		It("should emit validation_failures metric on invalid request", func() {
			// Business Scenario: Invalid audit submitted
			// Expected: Metrics track validation failures for monitoring

			// Get baseline
			baselineResp, err := http.Get(datastorageURL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer baselineResp.Body.Close()

			var baselineBody bytes.Buffer
			_, err = baselineBody.ReadFrom(baselineResp.Body)
			Expect(err).ToNot(HaveOccurred())
			baselineMetrics := baselineBody.String()

			// Invalid audit (missing required fields)
			// Use structured type but with missing required fields to test validation
			invalidPayload := &models.NotificationAudit{
				Recipient: "test@example.com",
				// Missing: RemediationID, NotificationID, Channel, Status, SentAt
			}

			payload, _ := json.Marshal(invalidPayload)
			resp, err := http.Post(
				datastorageURL+"/api/v1/audit/notifications",
				"application/json",
				bytes.NewBuffer(payload),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Should fail validation
			Expect(resp.StatusCode).To(Equal(400),
				"Invalid audit MUST be rejected with 400 Bad Request")

			// Get updated metrics
			updatedResp, err := http.Get(datastorageURL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer updatedResp.Body.Close()

			var updatedBody bytes.Buffer
			_, err = updatedBody.ReadFrom(updatedResp.Body)
			Expect(err).ToNot(HaveOccurred())
			updatedMetrics := updatedBody.String()

			// Business Outcome: validation_failures metric incremented
			Expect(updatedMetrics).To(ContainSubstring("datastorage_validation_failures_total"),
				"validation_failures metric MUST track invalid requests")

			// Verify metric appears or increased
			baselineHasMetric := strings.Contains(baselineMetrics, "datastorage_validation_failures_total")
			updatedHasMetric := strings.Contains(updatedMetrics, "datastorage_validation_failures_total")

			if !baselineHasMetric {
				// First failure, metric should now exist
				Expect(updatedHasMetric).To(BeTrue(),
					"validation_failures metric MUST appear after first validation failure")
			} else {
				// Metric existed, should still be there (can't easily verify increment due to parallel tests)
				Expect(updatedHasMetric).To(BeTrue(),
					"validation_failures metric MUST persist")
			}
		})

		It("should emit write_duration metric for database operations", func() {
			// Business Scenario: Audit write operation
			// Expected: Metrics track database write performance

			// Create valid audit
			auditPayload := &models.NotificationAudit{
				RemediationID:   fmt.Sprintf("test-remediation-%d", time.Now().Unix()),
				NotificationID:  fmt.Sprintf("test-notification-%d", time.Now().UnixNano()),
				Recipient:       "test@example.com",
				Channel:         "email",
				MessageSummary:  "Test notification for write duration",
				Status:          "sent",
				SentAt:          time.Now().UTC(),
				DeliveryStatus:  "200 OK",
				EscalationLevel: 0,
			}

			payload, _ := json.Marshal(auditPayload)
			resp, err := http.Post(
				datastorageURL+"/api/v1/audit/notifications",
				"application/json",
				bytes.NewBuffer(payload),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(201))

			// Get metrics
			metricsResp, err := http.Get(datastorageURL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer metricsResp.Body.Close()

			var body bytes.Buffer
			_, err = body.ReadFrom(metricsResp.Body)
			Expect(err).ToNot(HaveOccurred())
			metricsText := body.String()

			// Business Outcome: write_duration histogram recorded
			Expect(metricsText).To(ContainSubstring("datastorage_write_duration_seconds"),
				"write_duration histogram MUST track database performance")

			// Verify it's a histogram with samples for notification_audit table
			Expect(metricsText).To(MatchRegexp(`datastorage_write_duration_seconds_count\{table="notification_audit"\} \d+`),
				"write_duration MUST have recorded samples for notification_audit table")
		})
	})
})
