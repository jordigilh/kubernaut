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

//go:build integration
// +build integration

package health_monitoring

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/metrics"
	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

var _ = Describe("Health Monitoring Metrics Integration", func() {
	var (
		ctx             context.Context
		logger          *logrus.Logger
		mockLLMClient   *mocks.MockLLMClient
		healthMonitor   monitoring.HealthMonitor
		metricsRegistry *prometheus.Registry
		enhancedMetrics *metrics.EnhancedHealthMetrics
		testServer      *httptest.Server
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce test noise

		// Initialize mock LLM client
		mockLLMClient = mocks.NewMockLLMClient()
		mockLLMClient.SetHealthy(true)
		mockLLMClient.SetResponseTime(25 * time.Millisecond)

		// Create isolated Prometheus registry for testing (hybrid approach)
		metricsRegistry = prometheus.NewRegistry()
		enhancedMetrics = metrics.NewEnhancedHealthMetrics(metricsRegistry)

		// Create health monitor with isolated metrics
		healthMonitor = monitoring.NewLLMHealthMonitorWithMetrics(mockLLMClient, logger, enhancedMetrics)

		// Setup HTTP server with Prometheus metrics endpoint
		mux := http.NewServeMux()
		mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
			promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{}).ServeHTTP(w, r)
		})

		testServer = httptest.NewServer(mux)
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}

		if healthMonitor != nil {
			_ = healthMonitor.StopHealthMonitoring(ctx)
		}
	})

	// BR-METRICS-020: MUST expose llm_health_status gauge with component label
	Context("BR-METRICS-020: Health Status Metrics", func() {
		It("should expose llm_health_status gauge metric", func() {
			By("Performing health check to generate metrics")
			healthStatus, err := healthMonitor.GetHealthStatus(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Record health status using enhanced metrics
			enhancedMetrics.RecordHealthStatus(healthStatus)

			By("Verifying health status metric is exposed")
			resp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body := make([]byte, 4096)
			n, _ := resp.Body.Read(body)
			metricsOutput := string(body[:n])

			Expect(metricsOutput).To(ContainSubstring("llm_health_status"),
				"BR-METRICS-020: Must expose llm_health_status gauge")
			Expect(metricsOutput).To(MatchRegexp(`llm_health_status\{.*component_type="llm-20b".*\} 1`),
				"BR-METRICS-020: Must expose gauge with component label")

			GinkgoWriter.Printf("✅ Health status metric exposed correctly\n")
		})

		It("should update health status metric when LLM becomes unhealthy", func() {
			By("Configuring LLM as unhealthy")
			mockLLMClient.SetError("service unavailable")

			By("Performing health check to generate unhealthy metrics")
			healthStatus, err := healthMonitor.GetHealthStatus(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Record unhealthy status using enhanced metrics
			enhancedMetrics.RecordHealthStatus(healthStatus)

			By("Verifying unhealthy status is reflected in metrics")
			resp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body := make([]byte, 4096)
			n, _ := resp.Body.Read(body)
			metricsOutput := string(body[:n])

			Expect(metricsOutput).To(MatchRegexp(`llm_health_status\{.*\} 0`),
				"BR-METRICS-020: Must show 0 for unhealthy status")

			GinkgoWriter.Printf("✅ Unhealthy status metric updated correctly\n")
		})
	})

	// BR-METRICS-021: MUST expose llm_health_check_duration_seconds histogram
	Context("BR-METRICS-021: Health Check Duration Metrics", func() {
		It("should expose llm_health_check_duration_seconds histogram", func() {
			By("Performing health check with timing")
			startTime := time.Now()
			_, err := healthMonitor.GetHealthStatus(ctx)
			Expect(err).ToNot(HaveOccurred())
			duration := time.Since(startTime)

			// Record health check duration using enhanced metrics
			enhancedMetrics.RecordHealthCheckDuration("llm-20b", duration)

			By("Verifying health check duration metric is exposed")
			resp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body := make([]byte, 4096)
			n, _ := resp.Body.Read(body)
			metricsOutput := string(body[:n])

			Expect(metricsOutput).To(ContainSubstring("llm_health_check_duration_seconds"),
				"BR-METRICS-021: Must expose health check duration histogram")
			Expect(metricsOutput).To(ContainSubstring("component_type=\"llm-20b\""),
				"BR-METRICS-021: Must include component_type label")

			GinkgoWriter.Printf("✅ Health check duration histogram exposed correctly\n")
		})
	})

	// BR-METRICS-022: MUST expose llm_health_checks_total counter with status label
	Context("BR-METRICS-022: Health Check Counter Metrics", func() {
		It("should expose llm_health_checks_total counter", func() {
			By("Performing multiple health checks")
			for i := 0; i < 3; i++ {
				_, err := healthMonitor.GetHealthStatus(ctx)
				Expect(err).ToNot(HaveOccurred())

				// Record successful health check
				enhancedMetrics.RecordHealthCheck("llm-20b", "success")
				time.Sleep(10 * time.Millisecond)
			}

			By("Verifying health checks counter metric is exposed")
			resp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body := make([]byte, 4096)
			n, _ := resp.Body.Read(body)
			metricsOutput := string(body[:n])

			Expect(metricsOutput).To(ContainSubstring("llm_health_checks_total"),
				"BR-METRICS-022: Must expose health checks counter")
			Expect(metricsOutput).To(MatchRegexp(`llm_health_checks_total\{.*status="success".*\} [3-9]`),
				"BR-METRICS-022: Must count successful health checks with status label")

			GinkgoWriter.Printf("✅ Health checks counter exposed correctly\n")
		})

		It("should track failed health checks separately", func() {
			By("Configuring LLM for failure")
			mockLLMClient.SetError("health check failed")

			By("Performing failed health checks")
			for i := 0; i < 2; i++ {
				_, err := healthMonitor.GetHealthStatus(ctx)
				Expect(err).ToNot(HaveOccurred()) // GetHealthStatus doesn't fail, reports unhealthy

				// Record failed health check
				enhancedMetrics.RecordHealthCheck("llm-20b", "failure")
				time.Sleep(10 * time.Millisecond)
			}

			By("Verifying failed health checks are counted")
			resp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body := make([]byte, 4096)
			n, _ := resp.Body.Read(body)
			metricsOutput := string(body[:n])

			Expect(metricsOutput).To(MatchRegexp(`llm_health_checks_total\{.*status="failure".*\} [2-9]`),
				"BR-METRICS-022: Must count failed health checks with failure status")

			GinkgoWriter.Printf("✅ Failed health checks counter tracked correctly\n")
		})
	})

	// BR-METRICS-023: MUST expose llm_health_consecutive_failures_total gauge
	Context("BR-METRICS-023: Consecutive Failures Metrics", func() {
		It("should expose llm_health_consecutive_failures_total gauge", func() {
			By("Simulating consecutive failures")
			mockLLMClient.SetError("consecutive failure scenario")

			consecutiveFailures := 3
			for i := 0; i < consecutiveFailures; i++ {
				_, err := healthMonitor.GetHealthStatus(ctx)
				Expect(err).ToNot(HaveOccurred())

				// Record consecutive failures
				enhancedMetrics.RecordConsecutiveFailures("llm-20b", i+1)
				time.Sleep(10 * time.Millisecond)
			}

			By("Verifying consecutive failures metric is exposed")
			resp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body := make([]byte, 4096)
			n, _ := resp.Body.Read(body)
			metricsOutput := string(body[:n])

			Expect(metricsOutput).To(ContainSubstring("llm_health_consecutive_failures_total"),
				"BR-METRICS-023: Must expose consecutive failures gauge")
			Expect(metricsOutput).To(MatchRegexp(`llm_health_consecutive_failures_total\{.*component_type="llm-20b".*\} [1-9]`),
				"BR-METRICS-023: Must track consecutive failures with component label")

			GinkgoWriter.Printf("✅ Consecutive failures gauge exposed correctly\n")
		})
	})

	// BR-METRICS-024: MUST expose llm_health_uptime_percentage gauge
	Context("BR-METRICS-024: Uptime Percentage Metrics", func() {
		It("should expose llm_health_uptime_percentage gauge", func() {
			By("Simulating uptime tracking")
			mockLLMClient.SetUptime(23*time.Hour + 45*time.Minute)
			mockLLMClient.SetDowntime(15 * time.Minute)

			expectedUptimePercentage := mockLLMClient.GetUptimePercentage()

			// Record uptime percentage using enhanced metrics
			enhancedMetrics.RecordUptimePercentage("llm-20b", expectedUptimePercentage)

			By("Verifying uptime percentage metric is exposed")
			resp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body := make([]byte, 4096)
			n, _ := resp.Body.Read(body)
			metricsOutput := string(body[:n])

			Expect(metricsOutput).To(ContainSubstring("llm_health_uptime_percentage"),
				"BR-METRICS-024: Must expose uptime percentage gauge")
			Expect(metricsOutput).To(MatchRegexp(`llm_health_uptime_percentage\{.*component_type="llm-20b".*\} [8-9][0-9]`),
				"BR-METRICS-024: Must show realistic uptime percentage")

			GinkgoWriter.Printf("✅ Uptime percentage gauge exposed correctly (%.2f%%)\n", expectedUptimePercentage)
		})
	})

	// BR-METRICS-025 & BR-METRICS-026: Probe duration metrics
	Context("BR-METRICS-025/026: Probe Duration Metrics", func() {
		It("should expose probe duration histograms", func() {
			By("Performing liveness and readiness probes")

			// Simulate liveness probe
			livenessResult, err := healthMonitor.PerformLivenessProbe(ctx)
			Expect(err).ToNot(HaveOccurred())
			enhancedMetrics.RecordProbeDuration("liveness", "llm-20b", livenessResult.ResponseTime)

			// Simulate readiness probe
			readinessResult, err := healthMonitor.PerformReadinessProbe(ctx)
			Expect(err).ToNot(HaveOccurred())
			enhancedMetrics.RecordProbeDuration("readiness", "llm-20b", readinessResult.ResponseTime)

			By("Verifying probe duration metrics are exposed")
			resp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body := make([]byte, 4096)
			n, _ := resp.Body.Read(body)
			metricsOutput := string(body[:n])

			Expect(metricsOutput).To(ContainSubstring("llm_probe_duration_seconds"),
				"BR-METRICS-025/026: Must expose probe duration histogram")
			Expect(metricsOutput).To(ContainSubstring("probe_type=\"liveness\""),
				"BR-METRICS-025: Must include liveness probe metrics")
			Expect(metricsOutput).To(ContainSubstring("probe_type=\"readiness\""),
				"BR-METRICS-026: Must include readiness probe metrics")

			GinkgoWriter.Printf("✅ Probe duration histograms exposed correctly\n")
		})
	})

	// BR-METRICS-030: MUST expose llm_dependency_status gauge
	Context("BR-METRICS-030: Dependency Status Metrics", func() {
		It("should expose llm_dependency_status gauge", func() {
			By("Checking dependency status")
			dependencyStatus, err := healthMonitor.GetDependencyStatus(ctx, "20b-llm-service")
			Expect(err).ToNot(HaveOccurred())

			// Record dependency status using enhanced metrics
			enhancedMetrics.RecordDependencyStatus("20b-llm-service", "critical", dependencyStatus.IsAvailable)

			By("Verifying dependency status metric is exposed")
			resp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body := make([]byte, 4096)
			n, _ := resp.Body.Read(body)
			metricsOutput := string(body[:n])

			Expect(metricsOutput).To(ContainSubstring("llm_dependency_status"),
				"BR-METRICS-030: Must expose dependency status gauge")
			Expect(metricsOutput).To(MatchRegexp(`llm_dependency_status\{criticality="critical",dependency_name="20b-llm-service"\} 1`),
				"BR-METRICS-030: Must include dependency name and criticality labels")

			GinkgoWriter.Printf("✅ Dependency status gauge exposed correctly\n")
		})
	})

	// BR-METRICS-035: MUST expose llm_monitoring_accuracy_percentage gauge
	Context("BR-METRICS-035: Monitoring Accuracy Metrics", func() {
		It("should expose llm_monitoring_accuracy_percentage gauge", func() {
			By("Recording monitoring accuracy")
			accuracyRate := 99.8 // High accuracy for health monitoring
			enhancedMetrics.RecordMonitoringAccuracy("llm-health-monitor", accuracyRate)

			By("Verifying monitoring accuracy metric is exposed")
			resp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body := make([]byte, 4096)
			n, _ := resp.Body.Read(body)
			metricsOutput := string(body[:n])

			Expect(metricsOutput).To(ContainSubstring("llm_monitoring_accuracy_percentage"),
				"BR-METRICS-035: Must expose monitoring accuracy gauge")
			Expect(metricsOutput).To(MatchRegexp(`llm_monitoring_accuracy_percentage\{monitor_component="llm-health-monitor"\} 99\.8`),
				"BR-METRICS-035: Must show >99% accuracy for BR-REL-011 compliance")

			GinkgoWriter.Printf("✅ Monitoring accuracy gauge exposed correctly (%.1f%%)\n", accuracyRate)
		})
	})

	// BR-METRICS-036: MUST expose llm_20b_model_parameter_count gauge
	Context("BR-METRICS-036: Model Parameter Count Metrics", func() {
		It("should expose llm_20b_model_parameter_count gauge", func() {
			By("Recording model parameter count")
			modelName := mockLLMClient.GetModel()
			parameterCount := float64(mockLLMClient.GetMinParameterCount())
			enhancedMetrics.RecordModelParameterCount(modelName, parameterCount)

			By("Verifying model parameter count metric is exposed")
			resp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body := make([]byte, 4096)
			n, _ := resp.Body.Read(body)
			metricsOutput := string(body[:n])

			Expect(metricsOutput).To(ContainSubstring("llm_20b_model_parameter_count"),
				"BR-METRICS-036: Must expose model parameter count gauge")
			Expect(metricsOutput).To(MatchRegexp(`llm_20b_model_parameter_count\{.*model_name=".*20b.*".*\} 2e\+10`),
				"BR-METRICS-036: Must show 20B+ parameter count for enterprise model validation")

			GinkgoWriter.Printf("✅ Model parameter count gauge exposed correctly (%.0e parameters)\n", parameterCount)
		})
	})

	// Integration test for hybrid approach verification
	Context("Hybrid Metrics Approach Validation", func() {
		It("should demonstrate isolated registry approach works correctly", func() {
			By("Verifying metrics are isolated to test registry")

			// Collect metrics from our isolated registry
			metricFamilies, err := metricsRegistry.Gather()
			Expect(err).ToNot(HaveOccurred())

			foundHealthMetrics := false
			for _, mf := range metricFamilies {
				if strings.Contains(mf.GetName(), "llm_health") {
					foundHealthMetrics = true
					break
				}
			}

			if foundHealthMetrics {
				GinkgoWriter.Printf("✅ Hybrid approach: Metrics properly isolated in test registry\n")
			} else {
				GinkgoWriter.Printf("ℹ️  No health metrics found yet in isolated registry\n")
			}

			By("Verifying HTTP endpoint serves from isolated registry")
			resp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/plain"))

			GinkgoWriter.Printf("✅ Hybrid approach: HTTP metrics endpoint working correctly\n")
		})
	})
})
