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

// Package signalprocessing contains unit tests for Signal Processing controller.
// BR-SP-100: Observability - validates metrics follow DD-005 standards
package signalprocessing

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
)

var _ = Describe("BR-SP-100: Metrics (DD-005 Compliance)", func() {
	var (
		m   *metrics.Metrics
		reg *prometheus.Registry
	)

	BeforeEach(func() {
		// Use a fresh registry per test to avoid "already registered" errors
		reg = prometheus.NewRegistry()
		m = metrics.NewMetricsWithRegistry(reg)
	})

	// ==================================================
	// METRIC FUNCTIONALITY TESTS (NOT null-testing)
	// ==================================================
	Context("ReconciliationTotal Counter", func() {
		It("should increment reconciliation counter with phase and status labels", func() {
			// Act: Increment the counter with specific labels
			m.ReconciliationTotal.WithLabelValues("enriching", "success").Inc()
			m.ReconciliationTotal.WithLabelValues("enriching", "success").Inc()
			m.ReconciliationTotal.WithLabelValues("classifying", "failure").Inc()

			// Assert: Verify counter values
			enrichingSuccess := getCounterValue(m.ReconciliationTotal, "enriching", "success")
			classifyingFailure := getCounterValue(m.ReconciliationTotal, "classifying", "failure")

			Expect(enrichingSuccess).To(Equal(float64(2)))
			Expect(classifyingFailure).To(Equal(float64(1)))
		})
	})

	Context("ReconciliationDuration Histogram", func() {
		It("should observe reconciliation duration with phase label", func() {
			// Act: Observe some durations
			m.ReconciliationDuration.WithLabelValues("enriching").Observe(0.5)
			m.ReconciliationDuration.WithLabelValues("enriching").Observe(1.2)
			m.ReconciliationDuration.WithLabelValues("classifying").Observe(0.1)

			// Assert: Verify histogram sample counts
			enrichingCount := getHistogramSampleCount(m.ReconciliationDuration, "enriching")
			classifyingCount := getHistogramSampleCount(m.ReconciliationDuration, "classifying")

			Expect(enrichingCount).To(Equal(uint64(2)))
			Expect(classifyingCount).To(Equal(uint64(1)))
		})
	})

	Context("EnrichmentDuration Histogram", func() {
		It("should track K8s enrichment latency by resource kind (SLO: <2s P95)", func() {
			// Act: Observe enrichment times for different resource types
			m.EnrichmentDuration.WithLabelValues("Pod").Observe(0.5)
			m.EnrichmentDuration.WithLabelValues("Namespace").Observe(0.2)
			m.EnrichmentDuration.WithLabelValues("Node").Observe(0.8)

			// Assert: All observations recorded
			podCount := getHistogramSampleCount(m.EnrichmentDuration, "Pod")
			namespaceCount := getHistogramSampleCount(m.EnrichmentDuration, "Namespace")
			nodeCount := getHistogramSampleCount(m.EnrichmentDuration, "Node")

			Expect(podCount).To(Equal(uint64(1)))
			Expect(namespaceCount).To(Equal(uint64(1)))
			Expect(nodeCount).To(Equal(uint64(1)))
		})
	})

	Context("RegoEvaluationDuration Histogram", func() {
		It("should track Rego policy evaluation time by policy type (SLO: <100ms P95)", func() {
			// Act: Observe Rego evaluation times
			m.RegoEvaluationDuration.WithLabelValues("environment").Observe(0.05)   // 50ms
			m.RegoEvaluationDuration.WithLabelValues("priority").Observe(0.03)      // 30ms
			m.RegoEvaluationDuration.WithLabelValues("custom_labels").Observe(0.08) // 80ms

			// Assert: All observations recorded
			envCount := getHistogramSampleCount(m.RegoEvaluationDuration, "environment")
			priorityCount := getHistogramSampleCount(m.RegoEvaluationDuration, "priority")
			customCount := getHistogramSampleCount(m.RegoEvaluationDuration, "custom_labels")

			Expect(envCount).To(Equal(uint64(1)))
			Expect(priorityCount).To(Equal(uint64(1)))
			Expect(customCount).To(Equal(uint64(1)))
		})
	})

	Context("RegoHotReloadTotal Counter", func() {
		It("should track Rego policy hot-reload events by status", func() {
			// Act: Increment hot-reload counter
			m.RegoHotReloadTotal.WithLabelValues("success").Inc()
			m.RegoHotReloadTotal.WithLabelValues("success").Inc()
			m.RegoHotReloadTotal.WithLabelValues("failure").Inc()

			// Assert: Verify counter values
			successCount := getCounterValue(m.RegoHotReloadTotal, "success")
			failureCount := getCounterValue(m.RegoHotReloadTotal, "failure")

			Expect(successCount).To(Equal(float64(2)))
			Expect(failureCount).To(Equal(float64(1)))
		})
	})

	Context("CategorizationConfidence Histogram", func() {
		It("should track confidence scores for different classifiers", func() {
			// Act: Observe confidence scores
			m.CategorizationConfidence.WithLabelValues("environment").Observe(0.95)
			m.CategorizationConfidence.WithLabelValues("priority").Observe(0.85)
			m.CategorizationConfidence.WithLabelValues("business").Observe(0.70)

			// Assert: All observations recorded
			envCount := getHistogramSampleCount(m.CategorizationConfidence, "environment")
			priorityCount := getHistogramSampleCount(m.CategorizationConfidence, "priority")
			businessCount := getHistogramSampleCount(m.CategorizationConfidence, "business")

			Expect(envCount).To(Equal(uint64(1)))
			Expect(priorityCount).To(Equal(uint64(1)))
			Expect(businessCount).To(Equal(uint64(1)))
		})
	})

	// ==================================================
	// DD-005 NAMING CONVENTION COMPLIANCE
	// ==================================================
	Context("DD-005: Metric Naming Conventions", func() {
		It("should use kubernaut_signalprocessing namespace prefix", func() {
			// First, observe at least one metric so it appears in Gather()
			m.ReconciliationTotal.WithLabelValues("test", "test").Inc()

			// Collect all metrics from the registry
			metricFamilies, err := reg.Gather()
			Expect(err).NotTo(HaveOccurred())
			Expect(metricFamilies).NotTo(BeEmpty())

			// Verify all metrics have correct namespace prefix
			for _, mf := range metricFamilies {
				name := mf.GetName()
				Expect(name).To(HavePrefix("kubernaut_signalprocessing_"),
					"Metric %s should have kubernaut_signalprocessing_ prefix", name)
			}
		})
	})
})

// Helper function to get counter value
func getCounterValue(counter *prometheus.CounterVec, labelValues ...string) float64 {
	var metric dto.Metric
	err := counter.WithLabelValues(labelValues...).Write(&metric)
	if err != nil {
		return 0
	}
	return metric.GetCounter().GetValue()
}

// Helper function to get histogram sample count
func getHistogramSampleCount(histogram *prometheus.HistogramVec, labelValues ...string) uint64 {
	var metric dto.Metric
	observer := histogram.WithLabelValues(labelValues...)
	// Use type assertion to get the underlying Histogram interface
	if h, ok := observer.(prometheus.Histogram); ok {
		err := h.(prometheus.Metric).Write(&metric)
		if err != nil {
			return 0
		}
		return metric.GetHistogram().GetSampleCount()
	}
	return 0
}
