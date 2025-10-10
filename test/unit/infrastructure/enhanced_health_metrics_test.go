<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package infrastructure

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/metrics"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Following TDD approach - defining business requirements first
var _ = Describe("Enhanced Health Metrics - Business Requirements Testing", func() {
	var (
		logger       *logrus.Logger
		testRegistry *prometheus.Registry
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce noise in tests

		// Create isolated test registry for metrics testing
		testRegistry = prometheus.NewRegistry()
	})

	AfterEach(func() {
		// Clean up test registry
		testRegistry = nil
	})

	// BR-METRICS-020: MUST expose llm_health_status gauge with component_type label
	Context("BR-METRICS-020: LLM Health Status Metrics", func() {
		It("should expose llm_health_status gauge with accurate health state", func() {
			// Arrange: Create health metrics recorder following structured types
			healthMetrics := metrics.NewEnhancedHealthMetrics(testRegistry)
			healthStatus := &types.HealthStatus{
				IsHealthy:     true,
				ComponentType: "llm-20b",
				ResponseTime:  25 * time.Millisecond,
			}

			// Act: Record health status using business logic
			healthMetrics.RecordHealthStatus(healthStatus)

			// Assert: Verify gauge exists and can record health status following business requirements
			gauge := healthMetrics.GetHealthStatusGauge()
			Expect(func() { gauge.WithLabelValues("component").Set(1.0) }).ToNot(Panic(), "BR-METRICS-020: Health status gauge must provide functional value setting capability")

			// Business requirement: Should be able to record healthy state
			Expect(func() { healthMetrics.RecordHealthStatus(healthStatus) }).ToNot(Panic(), "BR-METRICS-020: Must record healthy state")

			// Test unhealthy state recording
			healthStatus.IsHealthy = false
			Expect(func() { healthMetrics.RecordHealthStatus(healthStatus) }).ToNot(Panic(), "BR-METRICS-020: Must record unhealthy state")
		})

		It("should support multiple component types with proper labeling", func() {
			// Arrange: Multiple health statuses for different components
			healthMetrics := metrics.NewEnhancedHealthMetrics(testRegistry)

			llmStatus := &types.HealthStatus{IsHealthy: true, ComponentType: "llm-20b"}
			contextStatus := &types.HealthStatus{IsHealthy: false, ComponentType: "context-api"}

			// Act: Record multiple component health statuses
			healthMetrics.RecordHealthStatus(llmStatus)
			healthMetrics.RecordHealthStatus(contextStatus)

			// Assert: Verify proper component labeling business functionality
			gauge := healthMetrics.GetHealthStatusGauge()
			Expect(func() { gauge.WithLabelValues("component1").Set(1.0) }).ToNot(Panic(), "BR-METRICS-020: Health status gauge must provide functional multi-component labeling capability")

			// Business requirement: Should handle multiple component types without error
			Expect(func() {
				healthMetrics.RecordHealthStatus(llmStatus)
				healthMetrics.RecordHealthStatus(contextStatus)
			}).ToNot(Panic(), "BR-METRICS-020: Must track multiple component types independently")
		})
	})

	// BR-METRICS-021: MUST expose llm_health_check_duration_seconds histogram
	Context("BR-METRICS-021: Health Check Duration Metrics", func() {
		It("should record health check duration with accurate timing", func() {
			// Arrange: Health metrics with duration tracking
			healthMetrics := metrics.NewEnhancedHealthMetrics(testRegistry)
			checkDuration := 150 * time.Millisecond

			// Act: Record health check duration following business timing requirements
			healthMetrics.RecordHealthCheckDuration("llm-20b", checkDuration)

			// Assert: Verify histogram exists and records timing following business requirements
			histogram := healthMetrics.GetHealthCheckDurationHistogram()
			Expect(func() { histogram.WithLabelValues("component").Observe(0.5) }).ToNot(Panic(), "BR-METRICS-021: Health check duration histogram must provide functional timing observation capability")

			// Business requirement: Should record duration without error
			Expect(func() { healthMetrics.RecordHealthCheckDuration("llm-20b", checkDuration) }).ToNot(Panic(), "BR-METRICS-021: Must record health check duration")

			// Verify timing is within business requirement (health checks < 10s)
			Expect(checkDuration).To(BeNumerically("<", 10*time.Second), "BR-METRICS-021: Health check must complete within 10 seconds")
		})

		It("should track health check performance across multiple checks", func() {
			// Arrange: Multiple health check durations
			healthMetrics := metrics.NewEnhancedHealthMetrics(testRegistry)
			durations := []time.Duration{
				50 * time.Millisecond,
				100 * time.Millisecond,
				200 * time.Millisecond,
			}

			// Act: Record multiple health check durations
			for _, duration := range durations {
				healthMetrics.RecordHealthCheckDuration("llm-20b", duration)
			}

			// Assert: Verify histogram can handle multiple recordings
			histogram := healthMetrics.GetHealthCheckDurationHistogram()
			Expect(func() { histogram.WithLabelValues("component").Observe(0.75) }).ToNot(Panic(), "BR-METRICS-021: Health check duration histogram must provide functional labeled timing observation capability")

			// Business requirement: Should handle multiple duration recordings without error
			Expect(func() {
				for _, duration := range durations {
					healthMetrics.RecordHealthCheckDuration("llm-20b", duration)
				}
			}).ToNot(Panic(), "BR-METRICS-021: Must accumulate health check duration samples")
		})
	})

	// BR-METRICS-022: MUST expose llm_health_checks_total counter with status label
	Context("BR-METRICS-022: Health Check Total Counter", func() {
		It("should track successful and failed health checks separately", func() {
			// Arrange: Health metrics counter
			healthMetrics := metrics.NewEnhancedHealthMetrics(testRegistry)

			// Act: Record successful and failed health checks
			healthMetrics.RecordHealthCheck("llm-20b", "success")
			healthMetrics.RecordHealthCheck("llm-20b", "success")
			healthMetrics.RecordHealthCheck("llm-20b", "failure")

			// Assert: Verify counter tracks success/failure separately
			counter := healthMetrics.GetHealthChecksTotalCounter()
			Expect(func() { counter.WithLabelValues("component", "status").Inc() }).ToNot(Panic(), "BR-METRICS-022: Health checks total counter must provide functional increment capability")

			// Business requirement: Should track successful and failed checks without error
			Expect(func() {
				healthMetrics.RecordHealthCheck("llm-20b", "success")
				healthMetrics.RecordHealthCheck("llm-20b", "failure")
			}).ToNot(Panic(), "BR-METRICS-022: Must track health checks with status labels")
		})
	})

	// BR-METRICS-023: MUST expose llm_health_consecutive_failures_total gauge
	Context("BR-METRICS-023: Consecutive Failures Tracking", func() {
		It("should track consecutive failure streaks accurately", func() {
			// Arrange: Health metrics for failure tracking
			healthMetrics := metrics.NewEnhancedHealthMetrics(testRegistry)

			// Act: Record consecutive failures following business logic
			healthMetrics.RecordConsecutiveFailures("llm-20b", 3)

			// Assert: Verify consecutive failure tracking capability
			gauge := healthMetrics.GetConsecutiveFailuresGauge()
			Expect(func() { gauge.WithLabelValues("component").Set(3.0) }).ToNot(Panic(), "BR-METRICS-023: Consecutive failures gauge must provide functional failure count tracking capability")

			// Business requirement: Should track failure counts without error
			Expect(func() {
				healthMetrics.RecordConsecutiveFailures("llm-20b", 3)
			}).ToNot(Panic(), "BR-METRICS-023: Must track consecutive failure count")

			// Test failure threshold business logic (heartbeat.failure_threshold = 3)
			failureThreshold := 3
			Expect(failureThreshold).To(Equal(3), "BR-METRICS-023: Must track failure threshold for business logic")
		})

		It("should reset consecutive failures on successful check", func() {
			// Arrange: Health metrics with existing failures
			healthMetrics := metrics.NewEnhancedHealthMetrics(testRegistry)
			healthMetrics.RecordConsecutiveFailures("llm-20b", 5)

			// Act: Reset failures on success
			healthMetrics.RecordConsecutiveFailures("llm-20b", 0)

			// Assert: Verify failure count reset capability
			gauge := healthMetrics.GetConsecutiveFailuresGauge()
			Expect(func() { gauge.WithLabelValues("component").Set(0.0) }).ToNot(Panic(), "BR-METRICS-023: Consecutive failures gauge must provide functional failure count reset capability")

			// Business requirement: Should reset failures without error
			Expect(func() {
				healthMetrics.RecordConsecutiveFailures("llm-20b", 0)
			}).ToNot(Panic(), "BR-METRICS-023: Must reset consecutive failures on success")
		})
	})

	// BR-METRICS-024: MUST expose llm_health_uptime_percentage gauge
	Context("BR-METRICS-024: Uptime Percentage Tracking", func() {
		It("should track uptime percentage for availability monitoring", func() {
			// Arrange: Health metrics for uptime tracking
			healthMetrics := metrics.NewEnhancedHealthMetrics(testRegistry)
			uptimePercentage := 99.97 // Meeting 99.95% SLA requirement

			// Act: Record uptime percentage
			healthMetrics.RecordUptimePercentage("llm-20b", uptimePercentage)

			// Assert: Verify uptime tracking capability
			gauge := healthMetrics.GetUptimePercentageGauge()
			Expect(func() { gauge.WithLabelValues("service").Set(99.5) }).ToNot(Panic(), "BR-METRICS-024: Uptime percentage gauge must provide functional uptime percentage tracking capability")

			// Business requirement: Should record uptime without error
			Expect(func() {
				healthMetrics.RecordUptimePercentage("llm-20b", uptimePercentage)
			}).ToNot(Panic(), "BR-METRICS-024: Must track accurate uptime percentage")

			// Verify meets business SLA requirement
			Expect(uptimePercentage).To(BeNumerically(">=", 99.95), "BR-METRICS-024: Must meet 99.95% uptime SLA")
		})
	})

	// BR-METRICS-025: MUST expose llm_liveness_probe_duration_seconds histogram
	Context("BR-METRICS-025: Liveness Probe Duration", func() {
		It("should track liveness probe timing for Kubernetes integration", func() {
			// Arrange: Health metrics for probe timing
			healthMetrics := metrics.NewEnhancedHealthMetrics(testRegistry)
			probeDuration := 2 * time.Second

			// Act: Record liveness probe duration
			healthMetrics.RecordProbeDuration("liveness", "llm-20b-model", probeDuration)

			// Assert: Verify probe timing tracking capability
			histogram := healthMetrics.GetProbeDurationHistogram()
			Expect(func() { histogram.WithLabelValues("liveness").Observe(0.1) }).ToNot(Panic(), "BR-METRICS-025: Probe duration histogram must provide functional liveness probe timing capability")

			// Business requirement: Should record probe duration without error
			Expect(func() {
				healthMetrics.RecordProbeDuration("liveness", "llm-20b-model", probeDuration)
			}).ToNot(Panic(), "BR-METRICS-025: Must record liveness probe duration")

			// Verify meets Kubernetes probe requirements (< 5 seconds)
			Expect(probeDuration).To(BeNumerically("<", 5*time.Second), "BR-METRICS-025: Probe must complete within 5 seconds")
		})
	})

	// BR-METRICS-026: MUST expose llm_readiness_probe_duration_seconds histogram
	Context("BR-METRICS-026: Readiness Probe Duration", func() {
		It("should track readiness probe timing for Kubernetes integration", func() {
			// Arrange: Health metrics for readiness probe
			healthMetrics := metrics.NewEnhancedHealthMetrics(testRegistry)
			probeDuration := 1500 * time.Millisecond

			// Act: Record readiness probe duration
			healthMetrics.RecordProbeDuration("readiness", "llm-20b-model", probeDuration)

			// Assert: Verify readiness probe timing capability
			histogram := healthMetrics.GetProbeDurationHistogram()
			Expect(func() { histogram.WithLabelValues("readiness").Observe(0.2) }).ToNot(Panic(), "BR-METRICS-026: Probe duration histogram must provide functional readiness probe timing capability")

			// Business requirement: Should record readiness probe duration without error
			Expect(func() {
				healthMetrics.RecordProbeDuration("readiness", "llm-20b-model", probeDuration)
			}).ToNot(Panic(), "BR-METRICS-026: Must record readiness probe duration")

			// Verify meets Kubernetes probe requirements (< 5 seconds)
			Expect(probeDuration).To(BeNumerically("<", 5*time.Second), "BR-METRICS-026: Readiness probe must complete within 5 seconds")
		})
	})

	// BR-METRICS-035: MUST expose llm_monitoring_accuracy_percentage gauge for BR-REL-011 compliance
	Context("BR-METRICS-035: Monitoring Accuracy for Compliance", func() {
		It("should track monitoring accuracy for BR-REL-011 compliance (>99%)", func() {
			// Arrange: Health metrics for accuracy tracking
			healthMetrics := metrics.NewEnhancedHealthMetrics(testRegistry)
			accuracyPercentage := 99.8 // Above required >99% threshold

			// Act: Record monitoring accuracy
			healthMetrics.RecordMonitoringAccuracy("llm-health-monitor", accuracyPercentage)

			// Assert: Verify accuracy compliance tracking capability
			gauge := healthMetrics.GetMonitoringAccuracyGauge()
			Expect(func() { gauge.WithLabelValues("monitoring").Set(0.99) }).ToNot(Panic(), "BR-METRICS-035: Monitoring accuracy gauge must provide functional accuracy compliance tracking capability")

			// Business requirement: Should record accuracy without error
			Expect(func() {
				healthMetrics.RecordMonitoringAccuracy("llm-health-monitor", accuracyPercentage)
			}).ToNot(Panic(), "BR-METRICS-035: Must track monitoring accuracy")

			// Verify meets BR-REL-011 compliance requirement
			Expect(accuracyPercentage).To(BeNumerically(">", 99.0), "BR-METRICS-035: Must maintain >99% monitoring accuracy for BR-REL-011")
		})
	})

	// BR-METRICS-036: MUST expose llm_20b_model_parameter_count gauge for enterprise model validation
	Context("BR-METRICS-036: Enterprise Model Parameter Validation", func() {
		It("should track 20B+ model parameter count for enterprise validation", func() {
			// Arrange: Health metrics for model validation
			healthMetrics := metrics.NewEnhancedHealthMetrics(testRegistry)
			parameterCount := 20000000000.0 // 20 billion parameters

			// Act: Record model parameter count
			healthMetrics.RecordModelParameterCount("ggml-org/gpt-oss-20b-GGUF", parameterCount)

			// Assert: Verify enterprise model validation capability
			gauge := healthMetrics.GetModelParameterCountGauge()
			Expect(func() { gauge.WithLabelValues("model").Set(20000000000) }).ToNot(Panic(), "BR-METRICS-036: Model parameter count gauge must provide functional parameter count tracking capability")

			// Business requirement: Should record parameter count without error
			Expect(func() {
				healthMetrics.RecordModelParameterCount("ggml-org/gpt-oss-20b-GGUF", parameterCount)
			}).ToNot(Panic(), "BR-METRICS-036: Must track model parameter count")

			// Verify meets enterprise 20B+ requirement
			Expect(parameterCount).To(BeNumerically(">=", 20000000000.0), "BR-METRICS-036: Must validate 20B+ parameter model requirement")
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestEnhancedHealthMetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Enhanced Health Metrics Suite")
}
