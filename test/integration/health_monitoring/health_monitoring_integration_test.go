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
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	contextapi "github.com/jordigilh/kubernaut/pkg/api/context"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/metrics"
	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	testshared "github.com/jordigilh/kubernaut/test/integration/shared"
)

func TestHealthMonitoringIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Health Monitoring Integration Test Suite")
}

var _ = Describe("Health Monitoring Integration Tests", Ordered, func() {
	var (
		ctx               context.Context
		logger            *logrus.Logger
		mockLLMClient     *mocks.MockLLMClient
		realLLMClient     llm.Client
		healthMonitor     monitoring.HealthMonitor
		contextController *contextapi.ContextController
		testServer        *httptest.Server
		metricsRegistry   *prometheus.Registry
		testConfig        testshared.IntegrationConfig
		useRealLLM        bool
	)

	BeforeAll(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
		ctx = context.Background()

		GinkgoWriter.Printf("🔍 Starting Health Monitoring Integration Test Suite\n")

		// Load test configuration
		testConfig = testshared.LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests skipped via SKIP_INTEGRATION")
		}

		// Determine whether to use real LLM (preferred) or mock
		useRealLLM = os.Getenv("USE_REAL_LLM") != "false" && testConfig.LLMEndpoint != ""

		if useRealLLM {
			GinkgoWriter.Printf("🎯 Using REAL LLM endpoint: %s (preferred for e2e validation)\n", testConfig.LLMEndpoint)

			// Configure real LLM client for integration testing
			llmConfig := config.LLMConfig{
				Provider:    testConfig.LLMProvider,
				Model:       testConfig.LLMModel,
				Endpoint:    testConfig.LLMEndpoint,
				Temperature: 0.3,  // Lower temperature for consistent health checks
				MaxTokens:   2000, // Sufficient for health check responses
				Timeout:     30 * time.Second,
			}

			var err error
			realLLMClient, err = llm.NewClient(llmConfig, logger)
			Expect(err).ToNot(HaveOccurred(), "Failed to create real LLM client for health monitoring integration")

			// Verify real LLM is accessible before running tests
			err = realLLMClient.LivenessCheck(ctx)
			if err != nil {
				GinkgoWriter.Printf("⚠️  Real LLM not accessible, falling back to mock: %v\n", err)
				useRealLLM = false
			} else {
				GinkgoWriter.Printf("✅ Real LLM connectivity verified for health monitoring integration\n")
			}
		}

		if !useRealLLM {
			GinkgoWriter.Printf("🎭 Using Mock LLM client for consistent integration testing\n")
			mockLLMClient = mocks.NewMockLLMClient()
			mockLLMClient.SetHealthy(true)
			mockLLMClient.SetResponseTime(25 * time.Millisecond)
		}

		GinkgoWriter.Printf("📊 Health monitoring integration test setup completed\n")
	})

	BeforeEach(func() {
		// Reset context and create fresh components for each test
		ctx = context.Background()

		// Create isolated Prometheus registry for each test (hybrid approach)
		metricsRegistry = prometheus.NewRegistry()

		// Choose LLM client based on configuration
		var llmClient llm.Client
		if useRealLLM {
			llmClient = realLLMClient
		} else {
			// Reset mock state for each test
			mockLLMClient.ClearState()
			mockLLMClient.SetHealthy(true)
			mockLLMClient.SetResponseTime(25 * time.Millisecond)
			llmClient = mockLLMClient
		}

		// Create health monitor with the configured LLM client and isolated metrics
		enhancedMetrics := metrics.NewEnhancedHealthMetrics(metricsRegistry)
		healthMonitor = monitoring.NewLLMHealthMonitorWithMetrics(llmClient, logger, enhancedMetrics)

		// Create Context API controller and integrate health monitoring
		aiIntegrator := &engine.AIServiceIntegrator{} // Minimal setup for testing
		contextController = contextapi.NewContextController(aiIntegrator, nil, logger)
		contextController.SetHealthMonitor(healthMonitor)

		// Create test HTTP server for Context API endpoints
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v1/health/llm/liveness", contextController.LLMLivenessProbe)
		mux.HandleFunc("/api/v1/health/llm/readiness", contextController.LLMReadinessProbe)
		mux.HandleFunc("/api/v1/health/dependencies", contextController.DependenciesHealthCheck)
		mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
			// Serve Prometheus metrics using our isolated registry
			promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{}).ServeHTTP(w, r)
		})

		testServer = httptest.NewServer(mux)
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}

		// Stop health monitoring if started
		if healthMonitor != nil {
			_ = healthMonitor.StopHealthMonitoring(ctx)
		}
	})

	// BR-HEALTH-001: MUST implement comprehensive health checks for all components
	Context("BR-HEALTH-001: Comprehensive Health Checks", func() {
		It("should provide accurate LLM health status through Context API", func() {
			By("Performing health status check")
			startTime := time.Now()

			healthStatus, err := healthMonitor.GetHealthStatus(ctx)
			Expect(err).ToNot(HaveOccurred(), "Health status check should not fail")
			Expect(healthStatus).ToNot(BeNil(), "Health status should be returned")

			duration := time.Since(startTime)

			By("Validating business requirements")
			Expect(duration).To(BeNumerically("<", 30*time.Second), "BR-HEALTH-001: Health check must complete within 30 seconds")
			Expect(healthStatus.ComponentType).To(Equal("llm-20b"), "BR-HEALTH-001: Must identify component type")
			Expect(healthStatus.BaseEntity.UpdatedAt).To(BeTemporally("~", time.Now(), 5*time.Second), "BR-HEALTH-001: Must track check timestamps")

			if useRealLLM {
				GinkgoWriter.Printf("✅ Real LLM health check completed in %v\n", duration)
			} else {
				GinkgoWriter.Printf("🎭 Mock LLM health check completed in %v\n", duration)
			}
		})

		It("should detect and report LLM unavailability", func() {
			if useRealLLM {
				Skip("Skipping unavailability test with real LLM endpoint")
			}

			By("Configuring mock LLM for failure scenario")
			mockLLMClient.SetError("connection refused: LLM service unavailable")

			By("Testing health status detection")
			healthStatus, err := healthMonitor.GetHealthStatus(ctx)

			// Health status should still be returned, but indicate unhealthy state
			Expect(err).ToNot(HaveOccurred(), "GetHealthStatus should not fail, but report unhealthy state")
			Expect(healthStatus.IsHealthy).To(BeFalse(), "BR-HEALTH-001: Must accurately detect LLM unavailability")
			Expect(healthStatus.BaseTimestampedResult.Error).To(ContainSubstring("LLM service unavailable"), "BR-HEALTH-001: Must provide descriptive error information")
		})
	})

	// BR-HEALTH-002: MUST provide liveness and readiness probes for Kubernetes
	Context("BR-HEALTH-002: Kubernetes Probes Support", func() {
		It("should support liveness probe endpoint", func() {
			By("Testing /api/v1/health/llm/liveness endpoint")
			resp, err := http.Get(testServer.URL + "/api/v1/health/llm/liveness")
			Expect(err).ToNot(HaveOccurred(), "Liveness endpoint should be accessible")
			defer resp.Body.Close()

			By("Validating liveness probe response")
			if useRealLLM {
				// Real LLM should return healthy status
				Expect(resp.StatusCode).To(Equal(http.StatusOK), "BR-HEALTH-002: Liveness probe must return 200 for healthy real LLM")
			} else {
				// Mock LLM configured as healthy
				Expect(resp.StatusCode).To(Equal(http.StatusOK), "BR-HEALTH-002: Liveness probe must return 200 for healthy mock LLM")
			}

			GinkgoWriter.Printf("✅ Liveness probe endpoint validated with status %d\n", resp.StatusCode)
		})

		It("should support readiness probe endpoint", func() {
			By("Testing /api/v1/health/llm/readiness endpoint")
			resp, err := http.Get(testServer.URL + "/api/v1/health/llm/readiness")
			Expect(err).ToNot(HaveOccurred(), "Readiness endpoint should be accessible")
			defer resp.Body.Close()

			By("Validating readiness probe response")
			if useRealLLM {
				// Real LLM readiness depends on actual model availability
				if resp.StatusCode == http.StatusOK {
					GinkgoWriter.Printf("✅ Real LLM is ready for traffic (status 200)\n")
				} else {
					GinkgoWriter.Printf("⚠️  Real LLM not ready, status: %d\n", resp.StatusCode)
				}
				// Accept either 200 (ready) or 503 (not ready) as valid responses
				Expect(resp.StatusCode).To(SatisfyAny(Equal(http.StatusOK), Equal(http.StatusServiceUnavailable)),
					"BR-HEALTH-002: Readiness probe must return valid status")
			} else {
				// Mock LLM configured as ready
				Expect(resp.StatusCode).To(Equal(http.StatusOK), "BR-HEALTH-002: Readiness probe must return 200 for ready mock LLM")
			}
		})

		It("should handle probe failures gracefully", func() {
			if useRealLLM {
				Skip("Skipping failure handling test with real LLM endpoint")
			}

			By("Configuring mock LLM for probe failure")
			mockLLMClient.SetError("probe connection timeout")

			By("Testing liveness probe failure handling")
			resp, err := http.Get(testServer.URL + "/api/v1/health/llm/liveness")
			Expect(err).ToNot(HaveOccurred(), "Liveness endpoint should be accessible even during failures")
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable), "BR-HEALTH-002: Failed liveness probe must return 503")
			GinkgoWriter.Printf("✅ Liveness probe failure handling validated (status 503)\n")
		})
	})

	// BR-HEALTH-003: MUST monitor external dependency health and availability
	Context("BR-HEALTH-003: External Dependency Monitoring", func() {
		It("should monitor LLM as critical external dependency", func() {
			By("Testing /api/v1/health/dependencies endpoint")
			resp, err := http.Get(testServer.URL + "/api/v1/health/dependencies")
			Expect(err).ToNot(HaveOccurred(), "Dependencies endpoint should be accessible")
			defer resp.Body.Close()

			By("Validating dependency monitoring response")
			if useRealLLM {
				// Real LLM dependency status depends on actual connectivity
				if resp.StatusCode == http.StatusOK {
					GinkgoWriter.Printf("✅ Real LLM dependency is available (status 200)\n")
				} else {
					GinkgoWriter.Printf("⚠️  Real LLM dependency issues detected, status: %d\n", resp.StatusCode)
				}
			} else {
				// Mock LLM configured as available
				Expect(resp.StatusCode).To(Equal(http.StatusOK), "BR-HEALTH-003: Dependencies endpoint must return 200 for available dependencies")
			}

			GinkgoWriter.Printf("✅ External dependency monitoring validated\n")
		})
	})
})
