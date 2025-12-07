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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

var _ = Describe("Gateway Metrics", func() {
	var (
		m        *metrics.Metrics
		registry *prometheus.Registry
	)

	BeforeEach(func() {
		// Create fresh registry for each test
		registry = prometheus.NewRegistry()
		m = metrics.NewMetricsWithRegistry(registry)
	})

	Describe("Metrics Registration", func() {
		It("should register all metrics successfully", func() {
			// BR-GATEWAY-016: Prometheus metrics registration
			// Verify metrics can be gathered without errors
			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())
			Expect(metricFamilies).ToNot(BeEmpty())
		})

		It("should have correct metric namespace", func() {
			// BR-GATEWAY-024: Metrics endpoint with proper naming
			// All metrics should start with "gateway_"
			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			for _, mf := range metricFamilies {
				Expect(mf.GetName()).To(HavePrefix("gateway_"))
			}
		})
	})

	Describe("Counter Metrics", func() {
		It("should increment HTTPRequestsTotal correctly", func() {
			// BR-GATEWAY-017: HTTP request metrics
			// Increment counter and verify value
			m.HTTPRequestsTotal.WithLabelValues("POST", "/signal", "200").Inc()
			m.HTTPRequestsTotal.WithLabelValues("POST", "/signal", "200").Inc()

			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var found bool
			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_http_requests_total" {
					found = true
					Expect(mf.GetType()).To(Equal(dto.MetricType_COUNTER))
					metrics := mf.GetMetric()
					Expect(metrics).ToNot(BeEmpty())
					Expect(metrics[0].GetCounter().GetValue()).To(Equal(float64(2)))
				}
			}
			Expect(found).To(BeTrue(), "gateway_http_requests_total metric should exist")
		})

		It("should increment AlertsReceivedTotal with correct labels", func() {
			// BR-GATEWAY-018: Signal processing metrics (renamed from alerts to signals)
			// Note: environment label removed (2025-12-06) - SP owns classification
			// Labels: source_type, severity (2 labels)
			m.AlertsReceivedTotal.WithLabelValues("prometheus", "critical").Inc()
			m.AlertsReceivedTotal.WithLabelValues("prometheus", "warning").Inc()

			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var found bool
			for _, mf := range metricFamilies {
				// Metric name is gateway_signals_received_total (not alerts)
				if mf.GetName() == "gateway_signals_received_total" {
					found = true
					metrics := mf.GetMetric()
					Expect(metrics).To(HaveLen(2))
				}
			}
			Expect(found).To(BeTrue(), "gateway_signals_received_total metric should exist")
		})
	})

	Describe("Histogram Metrics", func() {
		It("should observe HTTPRequestDuration correctly", func() {
			// BR-GATEWAY-017: HTTP request duration tracking
			// Observe duration and verify histogram
			m.HTTPRequestDuration.WithLabelValues("/signal", "POST", "200").Observe(0.123)
			m.HTTPRequestDuration.WithLabelValues("/signal", "POST", "200").Observe(0.456)

			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var found bool
			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_http_request_duration_seconds" {
					found = true
					Expect(mf.GetType()).To(Equal(dto.MetricType_HISTOGRAM))
					metrics := mf.GetMetric()
					Expect(metrics).ToNot(BeEmpty())
					histogram := metrics[0].GetHistogram()
					Expect(histogram.GetSampleCount()).To(Equal(uint64(2)))
					Expect(histogram.GetSampleSum()).To(BeNumerically("~", 0.579, 0.001))
				}
			}
			Expect(found).To(BeTrue())
		})

		It("should have correct histogram buckets", func() {
			// BR-GATEWAY-017: HTTP request duration buckets
			// Verify exponential buckets are configured
			m.HTTPRequestDuration.WithLabelValues("/signal", "POST", "200").Observe(0.001)

			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_http_request_duration_seconds" {
					metrics := mf.GetMetric()
					Expect(metrics).ToNot(BeEmpty())
					histogram := metrics[0].GetHistogram()
					buckets := histogram.GetBucket()
					Expect(buckets).ToNot(BeEmpty())
					Expect(len(buckets)).To(BeNumerically(">", 5))
				}
			}
		})
	})

	Describe("Gauge Metrics", func() {
		It("should set HTTPRequestsInFlight correctly", func() {
			// BR-GATEWAY-072: In-flight request tracking
			// Test gauge increment and decrement
			m.HTTPRequestsInFlight.Inc()
			m.HTTPRequestsInFlight.Inc()

			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var found bool
			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_http_requests_in_flight" {
					found = true
					Expect(mf.GetType()).To(Equal(dto.MetricType_GAUGE))
					metrics := mf.GetMetric()
					Expect(metrics).ToNot(BeEmpty())
					Expect(metrics[0].GetGauge().GetValue()).To(Equal(float64(2)))
				}
			}
			Expect(found).To(BeTrue())

			// Decrement and verify
			m.HTTPRequestsInFlight.Dec()
			metricFamilies, err = registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_http_requests_in_flight" {
					metrics := mf.GetMetric()
					Expect(metrics[0].GetGauge().GetValue()).To(Equal(float64(1)))
				}
			}
		})

		It("should set RedisPoolTotal correctly", func() {
			// BR-GATEWAY-025: Observability metrics
			// Test gauge set operation
			m.RedisPoolTotal.Set(10)

			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			var found bool
			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_redis_pool_total_connections" {
					found = true
					metrics := mf.GetMetric()
					Expect(metrics).ToNot(BeEmpty())
					Expect(metrics[0].GetGauge().GetValue()).To(Equal(float64(10)))
				}
			}
			Expect(found).To(BeTrue())
		})
	})

	Describe("Metrics Export", func() {
		It("should export metrics in Prometheus format", func() {
			// BR-GATEWAY-024: Metrics endpoint export
			// Verify metrics can be exported
			m.HTTPRequestsTotal.WithLabelValues("GET", "/health", "200").Inc()
			m.HTTPRequestDuration.WithLabelValues("/health", "GET", "200").Observe(0.001)
			m.HTTPRequestsInFlight.Set(5)

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
			// BR-GATEWAY-024: Metrics with labels
			// Note: environment label removed (2025-12-06) - SP owns classification
			// Labels: source_type, severity (2 labels)
			m.AlertsReceivedTotal.WithLabelValues("prometheus", "critical").Inc()

			metricFamilies, err := registry.Gather()
			Expect(err).ToNot(HaveOccurred())

			for _, mf := range metricFamilies {
				if mf.GetName() == "gateway_signals_received_total" {
					metrics := mf.GetMetric()
					Expect(metrics).ToNot(BeEmpty())
					labels := metrics[0].GetLabel()
					Expect(labels).To(HaveLen(2))

					labelMap := make(map[string]string)
					for _, label := range labels {
						labelMap[label.GetName()] = label.GetValue()
					}

					// Labels: source_type, severity (environment removed 2025-12-06)
					Expect(labelMap["source_type"]).To(Equal("prometheus"))
					Expect(labelMap["severity"]).To(Equal("critical"))
				}
			}
		})
	})
})
