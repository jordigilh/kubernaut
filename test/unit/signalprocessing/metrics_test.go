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
// Unit tests validate implementation correctness, not business value delivery.
//
// BR-SP-008: Prometheus Metrics - System MUST expose Prometheus-compatible metrics
// for processing counts, duration, and errors.
//
// Pattern: Follows Gateway/DataStorage authoritative pattern (per DD-TEST-005)
// Uses prometheus/client_model/go (dto) package for value verification.
package signalprocessing

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
)

var _ = Describe("BR-SP-008: SignalProcessing Metrics", func() {
	var (
		registry *prometheus.Registry
		m        *metrics.Metrics
	)

	BeforeEach(func() {
		// Create fresh registry for each test to avoid cross-test pollution
		// Per TESTING_GUIDELINES.md: Unit tests should use "Fresh Prometheus registry"
		registry = prometheus.NewRegistry()
		m = metrics.NewMetricsWithRegistry(registry)
	})

	// ========================================
	// Metrics Registration
	// ========================================

	Describe("Metrics Registration", func() {
		It("should register all metrics successfully", func() {
			// BR-SP-008: Verify all metrics are registered without errors
			// Note: Vec metrics only appear in registry after first observation
			m.IncrementProcessingTotal("enriching", "success")
			m.ObserveProcessingDuration("enriching", 0.1)
			m.EnrichmentTotal.WithLabelValues("success").Inc()
			m.EnrichmentDuration.WithLabelValues("k8s_context").Observe(0.1)
			m.RecordEnrichmentError("test_error")

			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())
			Expect(metricFamilies).ToNot(BeEmpty())
		})

		It("should have correct metric namespace", func() {
			// BR-SP-008: All metrics should start with "signalprocessing_"
			// Per DD-005: Metrics naming convention {service}_{component}_{metric}_{unit}
			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			for _, mf := range metricFamilies {
				Expect(mf.GetName()).To(HavePrefix("signalprocessing_"),
					"All metrics should use signalprocessing_ prefix per DD-005")
			}
		})

		It("should register all required metrics", func() {
			// BR-SP-008: Verify all expected metrics exist
			// Note: Vec metrics only appear in registry after first observation
			m.IncrementProcessingTotal("enriching", "success")
			m.ObserveProcessingDuration("enriching", 0.1)
			m.EnrichmentTotal.WithLabelValues("success").Inc()
			m.EnrichmentDuration.WithLabelValues("k8s_context").Observe(0.1)
			m.RecordEnrichmentError("test_error")

			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			// Build map of metric names
			metricNames := make(map[string]bool)
			for _, mf := range metricFamilies {
				metricNames[mf.GetName()] = true
			}

			// Core metrics must exist
			Expect(metricNames).To(HaveKey("signalprocessing_processing_total"))
			Expect(metricNames).To(HaveKey("signalprocessing_processing_duration_seconds"))
			Expect(metricNames).To(HaveKey("signalprocessing_enrichment_total"))
			Expect(metricNames).To(HaveKey("signalprocessing_enrichment_duration_seconds"))
			Expect(metricNames).To(HaveKey("signalprocessing_enrichment_errors_total"))
		})
	})

	// ========================================
	// Counter Metrics - Value Verification
	// ========================================

	Describe("Counter Metrics", func() {
		Context("ProcessingTotal counter", func() {
			It("should increment processing total counter correctly", func() {
				// BR-SP-008: Processing count metric
				// Increment counter twice and verify value is 2
				m.IncrementProcessingTotal("enriching", "success")
				m.IncrementProcessingTotal("enriching", "success")

				metricFamilies, err := registry.Gather()
				Expect(err).ToNot(HaveOccurred())

				var found bool
				for _, mf := range metricFamilies {
					if mf.GetName() == "signalprocessing_processing_total" {
						found = true
						Expect(mf.GetType()).To(Equal(dto.MetricType_COUNTER))
						metrics := mf.GetMetric()
						Expect(metrics).ToNot(BeEmpty())
						// Verify actual counter value is 2
						Expect(metrics[0].GetCounter().GetValue()).To(Equal(float64(2)))
					}
				}
				Expect(found).To(BeTrue(), "signalprocessing_processing_total metric should exist")
			})

			It("should track different phases separately", func() {
				// BR-SP-008: Processing phases should be tracked independently
				m.IncrementProcessingTotal("enriching", "success")
				m.IncrementProcessingTotal("classifying", "success")
				m.IncrementProcessingTotal("categorizing", "success")

				metricFamilies, err := registry.Gather()
				Expect(err).ToNot(HaveOccurred())

				for _, mf := range metricFamilies {
					if mf.GetName() == "signalprocessing_processing_total" {
						// Should have 3 separate label combinations
						metrics := mf.GetMetric()
						Expect(metrics).To(HaveLen(3))
					}
				}
			})

			It("should track different results separately", func() {
				// BR-SP-008: Success/failure should be tracked independently
				m.IncrementProcessingTotal("enriching", "success")
				m.IncrementProcessingTotal("enriching", "failure")

				metricFamilies, err := registry.Gather()
				Expect(err).ToNot(HaveOccurred())

				for _, mf := range metricFamilies {
					if mf.GetName() == "signalprocessing_processing_total" {
						metrics := mf.GetMetric()
						Expect(metrics).To(HaveLen(2))
						// Both should have value 1
						for _, metric := range metrics {
							Expect(metric.GetCounter().GetValue()).To(Equal(float64(1)))
						}
					}
				}
			})
		})

		Context("EnrichmentErrors counter", func() {
			It("should increment enrichment errors counter correctly", func() {
				// BR-SP-008: Error tracking metric
				m.RecordEnrichmentError("k8s_api_timeout")
				m.RecordEnrichmentError("k8s_api_timeout")

				metricFamilies, err := registry.Gather()
				Expect(err).ToNot(HaveOccurred())

				var found bool
				for _, mf := range metricFamilies {
					if mf.GetName() == "signalprocessing_enrichment_errors_total" {
						found = true
						Expect(mf.GetType()).To(Equal(dto.MetricType_COUNTER))
						metrics := mf.GetMetric()
						Expect(metrics).ToNot(BeEmpty())
						// Verify actual counter value is 2
						Expect(metrics[0].GetCounter().GetValue()).To(Equal(float64(2)))
					}
				}
				Expect(found).To(BeTrue(), "signalprocessing_enrichment_errors_total metric should exist")
			})

			It("should track different error types separately", func() {
				// BR-SP-008: Error types should be tracked with labels
				m.RecordEnrichmentError("k8s_api_timeout")
				m.RecordEnrichmentError("resource_not_found")
				m.RecordEnrichmentError("rbac_denied")

				metricFamilies, err := registry.Gather()
				Expect(err).ToNot(HaveOccurred())

				for _, mf := range metricFamilies {
					if mf.GetName() == "signalprocessing_enrichment_errors_total" {
						metrics := mf.GetMetric()
						Expect(metrics).To(HaveLen(3))
					}
				}
			})
		})

		Context("EnrichmentTotal counter", func() {
			It("should increment enrichment total counter correctly", func() {
				// BR-SP-008: Enrichment operation tracking
				m.EnrichmentTotal.WithLabelValues("success").Inc()
				m.EnrichmentTotal.WithLabelValues("success").Inc()

				metricFamilies, err := registry.Gather()
				Expect(err).ToNot(HaveOccurred())

				var found bool
				for _, mf := range metricFamilies {
					if mf.GetName() == "signalprocessing_enrichment_total" {
						found = true
						Expect(mf.GetType()).To(Equal(dto.MetricType_COUNTER))
						metrics := mf.GetMetric()
						Expect(metrics).ToNot(BeEmpty())
						Expect(metrics[0].GetCounter().GetValue()).To(Equal(float64(2)))
					}
				}
				Expect(found).To(BeTrue(), "signalprocessing_enrichment_total metric should exist")
			})
		})
	})

	// ========================================
	// Histogram Metrics - Value Verification
	// ========================================

	Describe("Histogram Metrics", func() {
		Context("ProcessingDuration histogram", func() {
			It("should observe processing duration correctly", func() {
				// BR-SP-008: Duration tracking metric
				m.ObserveProcessingDuration("enriching", 0.5)
				m.ObserveProcessingDuration("enriching", 1.5)

				metricFamilies, err := registry.Gather()
				Expect(err).ToNot(HaveOccurred())

				var found bool
				for _, mf := range metricFamilies {
					if mf.GetName() == "signalprocessing_processing_duration_seconds" {
						found = true
						Expect(mf.GetType()).To(Equal(dto.MetricType_HISTOGRAM))
						metrics := mf.GetMetric()
						Expect(metrics).ToNot(BeEmpty())
						histogram := metrics[0].GetHistogram()
						// Verify sample count is 2
						Expect(histogram.GetSampleCount()).To(Equal(uint64(2)))
						// Verify sample sum is 2.0 (0.5 + 1.5)
						Expect(histogram.GetSampleSum()).To(BeNumerically("~", 2.0, 0.001))
					}
				}
				Expect(found).To(BeTrue(), "signalprocessing_processing_duration_seconds metric should exist")
			})

			It("should have histogram buckets configured", func() {
				// BR-SP-008: Duration histogram should have buckets for SLO tracking
				m.ObserveProcessingDuration("enriching", 0.001)

				metricFamilies, err := registry.Gather()
				Expect(err).ToNot(HaveOccurred())

				for _, mf := range metricFamilies {
					if mf.GetName() == "signalprocessing_processing_duration_seconds" {
						metrics := mf.GetMetric()
						Expect(metrics).ToNot(BeEmpty())
						histogram := metrics[0].GetHistogram()
						buckets := histogram.GetBucket()
						Expect(buckets).ToNot(BeEmpty())
						// Default prometheus buckets have multiple entries
						Expect(len(buckets)).To(BeNumerically(">", 5))
					}
				}
			})

			It("should track different phases separately", func() {
				// BR-SP-008: Each phase should have its own histogram
				m.ObserveProcessingDuration("enriching", 0.5)
				m.ObserveProcessingDuration("classifying", 0.3)
				m.ObserveProcessingDuration("categorizing", 0.2)

				metricFamilies, err := registry.Gather()
				Expect(err).ToNot(HaveOccurred())

				for _, mf := range metricFamilies {
					if mf.GetName() == "signalprocessing_processing_duration_seconds" {
						metrics := mf.GetMetric()
						Expect(metrics).To(HaveLen(3))
					}
				}
			})
		})

		Context("EnrichmentDuration histogram", func() {
			It("should observe enrichment duration correctly", func() {
				// BR-SP-001: Enrichment latency SLO <2 seconds P95
				m.EnrichmentDuration.WithLabelValues("k8s_context").Observe(0.5)
				m.EnrichmentDuration.WithLabelValues("k8s_context").Observe(0.8)

				metricFamilies, err := registry.Gather()
				Expect(err).ToNot(HaveOccurred())

				var found bool
				for _, mf := range metricFamilies {
					if mf.GetName() == "signalprocessing_enrichment_duration_seconds" {
						found = true
						Expect(mf.GetType()).To(Equal(dto.MetricType_HISTOGRAM))
						metrics := mf.GetMetric()
						Expect(metrics).ToNot(BeEmpty())
						histogram := metrics[0].GetHistogram()
						Expect(histogram.GetSampleCount()).To(Equal(uint64(2)))
						Expect(histogram.GetSampleSum()).To(BeNumerically("~", 1.3, 0.001))
					}
				}
				Expect(found).To(BeTrue(), "signalprocessing_enrichment_duration_seconds metric should exist")
			})
		})
	})

	// ========================================
	// Metrics Export
	// ========================================

	Describe("Metrics Export", func() {
		It("should export metrics in Prometheus format", func() {
			// BR-SP-008: Metrics endpoint export verification
			m.IncrementProcessingTotal("enriching", "success")
			m.ObserveProcessingDuration("enriching", 0.5)
			m.RecordEnrichmentError("k8s_api_timeout")

			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())
			Expect(len(metricFamilies)).To(BeNumerically(">", 0))

			// Verify each metric family has proper structure
			for _, mf := range metricFamilies {
				Expect(mf.GetName()).ToNot(BeEmpty())
				Expect(mf.GetHelp()).ToNot(BeEmpty())
				Expect(mf.GetType()).ToNot(Equal(dto.MetricType_UNTYPED))
			}
		})

		It("should export metrics with correct labels", func() {
			// BR-SP-008: Verify label structure
			m.IncrementProcessingTotal("enriching", "success")

			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			for _, mf := range metricFamilies {
				if mf.GetName() == "signalprocessing_processing_total" {
					metrics := mf.GetMetric()
					Expect(metrics).ToNot(BeEmpty())
					labels := metrics[0].GetLabel()
					Expect(labels).To(HaveLen(2)) // phase, result

					labelMap := make(map[string]string)
					for _, label := range labels {
						labelMap[label.GetName()] = label.GetValue()
					}

					Expect(labelMap["phase"]).To(Equal("enriching"))
					Expect(labelMap["result"]).To(Equal("success"))
				}
			}
		})
	})
})

// Unit Test: NewMetricsWithRegistry for test isolation
var _ = Describe("NewMetricsWithRegistry", func() {
	It("METRICS-REG-01: should register metrics to custom registry", func() {
		// Per DD-METRICS-001: Use test-specific registry for isolation
		testRegistry := prometheus.NewRegistry()
		m := metrics.NewMetricsWithRegistry(testRegistry)

		Expect(m).ToNot(BeNil())

		// Record some metrics
		m.IncrementProcessingTotal("enriching", "success")
		m.ObserveProcessingDuration("enriching", 0.5)

		// Verify metrics are in the custom registry
		metricFamilies, err := testRegistry.Gather()
		Expect(err).ToNot(HaveOccurred())
		Expect(metricFamilies).ToNot(BeEmpty())

		// Verify the metrics have correct names
		metricNames := make(map[string]bool)
		for _, mf := range metricFamilies {
			metricNames[mf.GetName()] = true
		}

		Expect(metricNames).To(HaveKey("signalprocessing_processing_total"))
		Expect(metricNames).To(HaveKey("signalprocessing_processing_duration_seconds"))
	})

	It("METRICS-REG-02: should isolate metrics between registries", func() {
		// Per TESTING_GUIDELINES.md: Fresh Prometheus registry for each test
		registry1 := prometheus.NewRegistry()
		registry2 := prometheus.NewRegistry()

		m1 := metrics.NewMetricsWithRegistry(registry1)
		m2 := metrics.NewMetricsWithRegistry(registry2)

		// Increment only m1
		m1.IncrementProcessingTotal("enriching", "success")

		// Verify m1's registry has the metric
		families1, _ := registry1.Gather()
		var found1 bool
		for _, mf := range families1 {
			if mf.GetName() == "signalprocessing_processing_total" {
				found1 = true
				for _, metric := range mf.GetMetric() {
					Expect(metric.GetCounter().GetValue()).To(Equal(float64(1)))
				}
			}
		}
		Expect(found1).To(BeTrue())

		// Increment m2 twice
		m2.IncrementProcessingTotal("enriching", "success")
		m2.IncrementProcessingTotal("enriching", "success")

		// Verify m2's registry has different value
		families2, _ := registry2.Gather()
		for _, mf := range families2 {
			if mf.GetName() == "signalprocessing_processing_total" {
				for _, metric := range mf.GetMetric() {
					Expect(metric.GetCounter().GetValue()).To(Equal(float64(2)))
				}
			}
		}

		// Verify m1's registry still has 1 (not affected by m2)
		families1After, _ := registry1.Gather()
		for _, mf := range families1After {
			if mf.GetName() == "signalprocessing_processing_total" {
				for _, metric := range mf.GetMetric() {
					Expect(metric.GetCounter().GetValue()).To(Equal(float64(1)))
				}
			}
		}
	})
})
