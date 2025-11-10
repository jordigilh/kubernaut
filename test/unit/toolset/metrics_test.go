package toolset

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/jordigilh/kubernaut/pkg/toolset/metrics"
)

var _ = Describe("BR-TOOLSET-035: Prometheus Metrics (9 metrics)", func() {
	// DD-TOOLSET-001: REST API Deprecation - Removed 11 low-value metrics (55%)
	// See: docs/architecture/decisions/DD-TOOLSET-001-REST-API-Deprecation.md
	//
	// Removed Test Suites (11 metrics):
	// - API Request Metrics (3 tests) - REST API disabled
	// - Authentication Metrics (3 tests) - No authenticated endpoints
	// - Graceful Shutdown Metrics (2 tests) - Pod terminates before scrape
	// - Content-Type Validation (1 test) - No POST/PUT/PATCH endpoints
	// - RFC 7807 Errors (1 test) - No REST API errors
	//
	// Remaining Test Suites (9 metrics):
	// - Service Discovery Metrics (4 tests)
	// - ConfigMap Metrics (3 tests)
	// - Toolset Generation Metrics (2 tests)

	BeforeEach(func() {
		// Reset metrics before each test
		metrics.ResetMetrics()
	})

	Describe("Service Discovery Metrics", func() {
		It("should track services discovered count", func() {
			metrics.ServicesDiscovered.WithLabelValues("prometheus").Inc()
			metrics.ServicesDiscovered.WithLabelValues("grafana").Inc()
			metrics.ServicesDiscovered.WithLabelValues("prometheus").Inc()

			// Check prometheus service count
			count := testutil.ToFloat64(metrics.ServicesDiscovered.WithLabelValues("prometheus"))
			Expect(count).To(Equal(2.0))

			// Check grafana service count
			count = testutil.ToFloat64(metrics.ServicesDiscovered.WithLabelValues("grafana"))
			Expect(count).To(Equal(1.0))
		})

		It("should track discovery duration", func() {
			timer := prometheus.NewTimer(metrics.DiscoveryDuration)
			// Simulate discovery work
			timer.ObserveDuration()

			count := testutil.CollectAndCount(metrics.DiscoveryDuration)
			Expect(count).To(BeNumerically(">", 0))
		})

		It("should track discovery errors", func() {
			metrics.DiscoveryErrors.WithLabelValues("api_error").Inc()
			metrics.DiscoveryErrors.WithLabelValues("timeout").Inc()

			count := testutil.ToFloat64(metrics.DiscoveryErrors.WithLabelValues("api_error"))
			Expect(count).To(Equal(1.0))
		})

		It("should track health check failures", func() {
			metrics.HealthCheckFailures.WithLabelValues("prometheus", "timeout").Inc()
			metrics.HealthCheckFailures.WithLabelValues("grafana", "connection_refused").Inc()

			count := testutil.ToFloat64(metrics.HealthCheckFailures.WithLabelValues("prometheus", "timeout"))
			Expect(count).To(Equal(1.0))
		})
	})

	Describe("ConfigMap Metrics", func() {
		It("should track ConfigMap updates", func() {
			metrics.ConfigMapUpdates.WithLabelValues("success").Inc()
			metrics.ConfigMapUpdates.WithLabelValues("success").Inc()
			metrics.ConfigMapUpdates.WithLabelValues("failure").Inc()

			successCount := testutil.ToFloat64(metrics.ConfigMapUpdates.WithLabelValues("success"))
			Expect(successCount).To(Equal(2.0))

			failureCount := testutil.ToFloat64(metrics.ConfigMapUpdates.WithLabelValues("failure"))
			Expect(failureCount).To(Equal(1.0))
		})

		It("should track ConfigMap reconciliation duration", func() {
			timer := prometheus.NewTimer(metrics.ConfigMapReconcileDuration)
			timer.ObserveDuration()

			count := testutil.CollectAndCount(metrics.ConfigMapReconcileDuration)
			Expect(count).To(BeNumerically(">", 0))
		})

		It("should track ConfigMap drift detections", func() {
			metrics.ConfigMapDriftDetected.Inc()

			count := testutil.ToFloat64(metrics.ConfigMapDriftDetected)
			Expect(count).To(Equal(1.0))
		})
	})

	Describe("Toolset Generation Metrics", func() {
		It("should track toolset generations", func() {
			metrics.ToolsetGenerations.WithLabelValues("success").Inc()
			metrics.ToolsetGenerations.WithLabelValues("success").Inc()

			count := testutil.ToFloat64(metrics.ToolsetGenerations.WithLabelValues("success"))
			Expect(count).To(Equal(2.0))
		})

		It("should track tools in generated toolset", func() {
			metrics.ToolsInToolset.Set(5.0)

			count := testutil.ToFloat64(metrics.ToolsInToolset)
			Expect(count).To(Equal(5.0))
		})
	})

	Describe("Metrics Export", func() {
		It("should export metrics in Prometheus format", func() {
			// Set some metric values
			metrics.ServicesDiscovered.WithLabelValues("prometheus").Inc()
			metrics.ConfigMapUpdates.WithLabelValues("success").Inc()

			// Collect metrics
			metricFamilies, err := prometheus.DefaultGatherer.Gather()
			Expect(err).ToNot(HaveOccurred())
			Expect(metricFamilies).ToNot(BeEmpty())

			// Verify our metrics are present
			found := false
			for _, mf := range metricFamilies {
				if strings.HasPrefix(*mf.Name, "dynamic_toolset_") {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue())
		})

		It("should include metric help text", func() {
			metricFamilies, err := prometheus.DefaultGatherer.Gather()
			Expect(err).ToNot(HaveOccurred())

			for _, mf := range metricFamilies {
				if strings.HasPrefix(*mf.Name, "dynamic_toolset_") {
					Expect(mf.Help).ToNot(BeNil())
					Expect(*mf.Help).ToNot(BeEmpty())
				}
			}
		})
	})
})
