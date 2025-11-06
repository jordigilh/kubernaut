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
	"github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
)

// ========================================
// TDD RED PHASE: Metrics Struct Tests
// Business Requirement: BR-STORAGE-019 (Logging and metrics)
// GAP-10: Audit-specific metrics
// Test entry point is in helpers_test.go (TestMetrics)
// ========================================

var _ = Describe("Metrics Struct", func() {
	var (
		m        *metrics.Metrics
		registry *prometheus.Registry
	)

	BeforeEach(func() {
		// Create custom registry for testing (avoids duplicate registration panics)
		registry = prometheus.NewRegistry()
		m = metrics.NewMetricsWithRegistry("datastorage", "", registry)
	})

	// BEHAVIOR: Metrics constructor creates functional metrics with custom registry
	// CORRECTNESS: All metric fields are initialized and can record observations
	Context("Metrics Creation", func() {
		It("should create metrics struct with all functional metrics", func() {
			// CORRECTNESS: All metrics are functional (can record values without panicking)
			// If any metric were nil, these calls would panic
			m.AuditTracesTotal.WithLabelValues("test_table", "success").Inc()
			m.AuditLagSeconds.WithLabelValues("test_table").Observe(0.5)
			m.WriteDuration.WithLabelValues("test_table").Observe(0.1)
			m.ValidationFailures.WithLabelValues("test_field", "test_reason").Inc()
		})

		It("should register metrics with custom registry", func() {
			// Record some values to ensure metrics appear in Gather()
			m.AuditTracesTotal.WithLabelValues(metrics.ServiceNotification, metrics.AuditStatusSuccess).Inc()
			m.AuditLagSeconds.WithLabelValues(metrics.ServiceNotification).Observe(0.5)
			m.WriteDuration.WithLabelValues("notification_audit").Observe(0.025)
			m.ValidationFailures.WithLabelValues("notification_id", metrics.ValidationReasonRequired).Inc()

			// Gather metrics from registry
			families, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			// Should have metrics registered (4 metrics)
			Expect(families).To(HaveLen(4), "Registry should contain 4 metric families")

			// Check for key metrics
			metricNames := make(map[string]bool)
			for _, family := range families {
				metricNames[family.GetName()] = true
			}

			Expect(metricNames).To(HaveKey("datastorage_audit_traces_total"), "audit_traces_total metric should exist")
			Expect(metricNames).To(HaveKey("datastorage_audit_lag_seconds"), "audit_lag_seconds metric should exist")
			Expect(metricNames).To(HaveKey("datastorage_write_duration_seconds"), "write_duration metric should exist")
			Expect(metricNames).To(HaveKey("datastorage_validation_failures_total"), "validation_failures metric should exist")
		})
	})

	Context("GAP-10: Audit Traces Total Metric", func() {
		It("should increment audit traces total with service and status labels", func() {
			// Increment counter
			m.AuditTracesTotal.WithLabelValues(metrics.ServiceNotification, metrics.AuditStatusSuccess).Inc()

			// Verify metric was incremented
			families, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			// Find the audit_traces_total metric
			var found bool
			for _, family := range families {
				if family.GetName() == "datastorage_audit_traces_total" {
					found = true
					Expect(family.GetMetric()).To(HaveLen(1), "Should have 1 label combination")
					metric := family.GetMetric()[0]
					Expect(metric.GetCounter().GetValue()).To(BeNumerically("==", 1))

					// Check labels
					labels := metric.GetLabel()
					Expect(labels).To(HaveLen(2), "Should have 2 labels: service, status")

					// Verify label values
					labelMap := make(map[string]string)
					for _, label := range labels {
						labelMap[label.GetName()] = label.GetValue()
					}
					Expect(labelMap["service"]).To(Equal(metrics.ServiceNotification))
					Expect(labelMap["status"]).To(Equal(metrics.AuditStatusSuccess))
					break
				}
			}
			Expect(found).To(BeTrue(), "audit_traces_total metric should exist in registry")
		})

		It("should support different audit statuses", func() {
			// Increment for different statuses
			m.AuditTracesTotal.WithLabelValues(metrics.ServiceNotification, metrics.AuditStatusSuccess).Inc()
			m.AuditTracesTotal.WithLabelValues(metrics.ServiceNotification, metrics.AuditStatusFailure).Inc()
			m.AuditTracesTotal.WithLabelValues(metrics.ServiceNotification, metrics.AuditStatusDLQFallback).Inc()

			// Verify all statuses recorded
			families, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			for _, family := range families {
				if family.GetName() == "datastorage_audit_traces_total" {
					Expect(family.GetMetric()).To(HaveLen(3), "Should have 3 label combinations (3 statuses)")
				}
			}
		})
	})

	Context("GAP-10: Audit Lag Seconds Metric", func() {
		It("should record audit lag observations", func() {
			// Record lag observations
			m.AuditLagSeconds.WithLabelValues(metrics.ServiceNotification).Observe(0.5)
			m.AuditLagSeconds.WithLabelValues(metrics.ServiceNotification).Observe(1.2)
			m.AuditLagSeconds.WithLabelValues(metrics.ServiceNotification).Observe(0.8)

			// Verify histogram recorded observations
			families, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var found bool
			for _, family := range families {
				if family.GetName() == "datastorage_audit_lag_seconds" {
					found = true
					Expect(family.GetMetric()).To(HaveLen(1), "Should have 1 label combination")
					metric := family.GetMetric()[0]

					// Histogram should have count
					Expect(metric.GetHistogram().GetSampleCount()).To(BeNumerically("==", 3))

					// Check labels
					labels := metric.GetLabel()
					Expect(labels).To(HaveLen(1), "Should have 1 label: service")
					Expect(labels[0].GetName()).To(Equal("service"))
					Expect(labels[0].GetValue()).To(Equal(metrics.ServiceNotification))
					break
				}
			}
			Expect(found).To(BeTrue(), "audit_lag_seconds metric should exist in registry")
		})
	})

	Context("Write Duration Metric", func() {
		It("should record write duration observations", func() {
			// Record write durations
			m.WriteDuration.WithLabelValues("notification_audit").Observe(0.025) // 25ms
			m.WriteDuration.WithLabelValues("notification_audit").Observe(0.050) // 50ms

			// Verify histogram recorded observations
			families, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var found bool
			for _, family := range families {
				if family.GetName() == "datastorage_write_duration_seconds" {
					found = true
					metric := family.GetMetric()[0]
					Expect(metric.GetHistogram().GetSampleCount()).To(BeNumerically("==", 2))
					break
				}
			}
			Expect(found).To(BeTrue(), "write_duration metric should exist")
		})
	})

	Context("Validation Failures Metric", func() {
		It("should increment validation failures with field and reason labels", func() {
			// Increment validation failure
			m.ValidationFailures.WithLabelValues("notification_id", metrics.ValidationReasonRequired).Inc()

			// Verify metric was incremented
			families, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var found bool
			for _, family := range families {
				if family.GetName() == "datastorage_validation_failures_total" {
					found = true
					Expect(family.GetMetric()).To(HaveLen(1))
					metric := family.GetMetric()[0]
					Expect(metric.GetCounter().GetValue()).To(BeNumerically("==", 1))

					// Check labels
					labels := metric.GetLabel()
					labelMap := make(map[string]string)
					for _, label := range labels {
						labelMap[label.GetName()] = label.GetValue()
					}
					Expect(labelMap["field"]).To(Equal("notification_id"))
					Expect(labelMap["reason"]).To(Equal(metrics.ValidationReasonRequired))
					break
				}
			}
			Expect(found).To(BeTrue())
		})
	})
})
