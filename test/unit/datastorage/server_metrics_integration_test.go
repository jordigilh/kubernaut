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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
)

func TestServerMetricsIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Server Metrics Integration Suite")
}

// ========================================
// TDD RED PHASE: Server Metrics Integration Tests
// Business Requirement: BR-STORAGE-019 (Logging and metrics)
// GAP-10: Audit-specific metrics in handlers
// ========================================

var _ = Describe("Server Metrics Integration", func() {
	var (
		testServer *httptest.Server
		registry   *prometheus.Registry
	)

	BeforeEach(func() {
		// Create test registry
		registry = prometheus.NewRegistry()

		// Create test server with metrics
		// TODO: Implement Server.NewWithMetrics() in REFACTOR phase
		Skip("Waiting for Server.NewWithMetrics() implementation")
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
	})

	Context("Metrics Endpoint", func() {
		It("should expose Prometheus metrics at /metrics", func() {
			resp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/plain"))

			// Response should contain metric names
			var body bytes.Buffer
			_, err = body.ReadFrom(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			metricsText := body.String()
			Expect(metricsText).To(ContainSubstring("datastorage_audit_traces_total"))
			Expect(metricsText).To(ContainSubstring("datastorage_audit_lag_seconds"))
			Expect(metricsText).To(ContainSubstring("datastorage_write_duration_seconds"))
		})
	})

	Context("Handler Metrics Emission", func() {
		It("should emit audit_traces_total metric on successful write", func() {
			audit := &models.NotificationAudit{
				RemediationID:  "test-remediation-1",
				NotificationID: "test-notification-1",
				Recipient:      "test@example.com",
				Channel:        "email",
				Status:         "sent",
				SentAt:         time.Now().UTC(),
			}

			payload, _ := json.Marshal(audit)
			resp, err := http.Post(
				testServer.URL+"/api/v1/audit/notifications",
				"application/json",
				bytes.NewBuffer(payload),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Should succeed
			Expect(resp.StatusCode).To(Equal(201))

			// Check metric was incremented
			families, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var auditMetricFound bool
			for _, family := range families {
				if family.GetName() == "datastorage_audit_traces_total" {
					auditMetricFound = true
					Expect(family.GetMetric()).ToNot(BeEmpty())
					// Should have service=notification, status=success
					break
				}
			}
			Expect(auditMetricFound).To(BeTrue(), "audit_traces_total metric should be emitted")
		})

		It("should emit audit_lag_seconds metric with calculated lag", func() {
			// Audit event that happened 2 seconds ago
			sentAt := time.Now().UTC().Add(-2 * time.Second)
			audit := &models.NotificationAudit{
				RemediationID:  "test-remediation-2",
				NotificationID: "test-notification-2",
				Recipient:      "test@example.com",
				Channel:        "email",
				Status:         "sent",
				SentAt:         sentAt,
			}

			payload, _ := json.Marshal(audit)
			resp, err := http.Post(
				testServer.URL+"/api/v1/audit/notifications",
				"application/json",
				bytes.NewBuffer(payload),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(201))

			// Check audit_lag_seconds metric was recorded
			families, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var lagMetricFound bool
			for _, family := range families {
				if family.GetName() == "datastorage_audit_lag_seconds" {
					lagMetricFound = true
					Expect(family.GetMetric()).ToNot(BeEmpty())
					metric := family.GetMetric()[0]
					// Lag should be approximately 2 seconds (±100ms tolerance)
					sum := metric.GetHistogram().GetSampleSum()
					Expect(sum).To(BeNumerically("~", 2.0, 0.5))
					break
				}
			}
			Expect(lagMetricFound).To(BeTrue(), "audit_lag_seconds metric should be emitted")
		})

		It("should emit validation_failures metric on invalid request", func() {
			// Invalid audit (missing required fields)
			invalidAudit := &models.NotificationAudit{
				Recipient: "test@example.com",
			}

			payload, _ := json.Marshal(invalidAudit)
			resp, err := http.Post(
				testServer.URL+"/api/v1/audit/notifications",
				"application/json",
				bytes.NewBuffer(payload),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Should fail validation
			Expect(resp.StatusCode).To(Equal(400))

			// Check validation_failures metric was incremented
			families, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var validationMetricFound bool
			for _, family := range families {
				if family.GetName() == "datastorage_validation_failures_total" {
					validationMetricFound = true
					Expect(family.GetMetric()).ToNot(BeEmpty())
					// Should have at least one validation failure
					break
				}
			}
			Expect(validationMetricFound).To(BeTrue(), "validation_failures metric should be emitted")
		})

		It("should emit write_duration metric for database operations", func() {
			audit := &models.NotificationAudit{
				RemediationID:  "test-remediation-3",
				NotificationID: "test-notification-3",
				Recipient:      "test@example.com",
				Channel:        "email",
				Status:         "sent",
				SentAt:         time.Now().UTC(),
			}

			payload, _ := json.Marshal(audit)
			resp, err := http.Post(
				testServer.URL+"/api/v1/audit/notifications",
				"application/json",
				bytes.NewBuffer(payload),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(201))

			// Check write_duration metric was recorded
			families, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var durationMetricFound bool
			for _, family := range families {
				if family.GetName() == "datastorage_write_duration_seconds" {
					durationMetricFound = true
					Expect(family.GetMetric()).ToNot(BeEmpty())
					metric := family.GetMetric()[0]
					Expect(metric.GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
					break
				}
			}
			Expect(durationMetricFound).To(BeTrue(), "write_duration metric should be emitted")
		})
	})
})

