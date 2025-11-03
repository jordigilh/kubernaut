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

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMetricsStruct(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Storage Metrics Struct Suite")
}

// ========================================
// TDD RED PHASE: Metrics Struct Tests
// Business Requirement: BR-STORAGE-019 (Logging and metrics)
// GAP-10: Audit-specific metrics
// ========================================

var _ = Describe("Metrics Struct", func() {
	var (
		metrics  *Metrics
		registry *prometheus.Registry
	)

	BeforeEach(func() {
		// Create custom registry for testing (avoids duplicate registration panics)
		registry = prometheus.NewRegistry()
		metrics = NewMetricsWithRegistry("datastorage", "", registry)
	})

	Context("Metrics Creation", func() {
		It("should create metrics struct with all required metrics", func() {
			Expect(metrics).ToNot(BeNil())
			Expect(metrics.AuditTracesTotal).ToNot(BeNil(), "AuditTracesTotal should be initialized")
			Expect(metrics.AuditLagSeconds).ToNot(BeNil(), "AuditLagSeconds should be initialized")
			Expect(metrics.WriteDuration).ToNot(BeNil(), "WriteDuration should be initialized")
			Expect(metrics.ValidationFailures).ToNot(BeNil(), "ValidationFailures should be initialized")
		})

		It("should register metrics with custom registry", func() {
			// Record some values to ensure metrics appear in Gather()
			metrics.AuditTracesTotal.WithLabelValues(ServiceNotification, AuditStatusSuccess).Inc()
			metrics.AuditLagSeconds.WithLabelValues(ServiceNotification).Observe(0.5)
			metrics.WriteDuration.WithLabelValues("notification_audit").Observe(0.025)
			metrics.ValidationFailures.WithLabelValues("notification_id", ValidationReasonRequired).Inc()

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
			metrics.AuditTracesTotal.WithLabelValues(ServiceNotification, AuditStatusSuccess).Inc()

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
					Expect(labelMap["service"]).To(Equal(ServiceNotification))
					Expect(labelMap["status"]).To(Equal(AuditStatusSuccess))
					break
				}
			}
			Expect(found).To(BeTrue(), "audit_traces_total metric should exist in registry")
		})

		It("should support different audit statuses", func() {
			// Increment for different statuses
			metrics.AuditTracesTotal.WithLabelValues(ServiceNotification, AuditStatusSuccess).Inc()
			metrics.AuditTracesTotal.WithLabelValues(ServiceNotification, AuditStatusFailure).Inc()
			metrics.AuditTracesTotal.WithLabelValues(ServiceNotification, AuditStatusDLQFallback).Inc()

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
			metrics.AuditLagSeconds.WithLabelValues(ServiceNotification).Observe(0.5)
			metrics.AuditLagSeconds.WithLabelValues(ServiceNotification).Observe(1.2)
			metrics.AuditLagSeconds.WithLabelValues(ServiceNotification).Observe(0.8)

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
					Expect(labels[0].GetValue()).To(Equal(ServiceNotification))
					break
				}
			}
			Expect(found).To(BeTrue(), "audit_lag_seconds metric should exist in registry")
		})
	})

	Context("Write Duration Metric", func() {
		It("should record write duration observations", func() {
			// Record write durations
			metrics.WriteDuration.WithLabelValues("notification_audit").Observe(0.025) // 25ms
			metrics.WriteDuration.WithLabelValues("notification_audit").Observe(0.050) // 50ms

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
			metrics.ValidationFailures.WithLabelValues("notification_id", ValidationReasonRequired).Inc()

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
					Expect(labelMap["reason"]).To(Equal(ValidationReasonRequired))
					break
				}
			}
			Expect(found).To(BeTrue())
		})
	})
})

