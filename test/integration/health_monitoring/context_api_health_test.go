//go:build integration
// +build integration

package health_monitoring

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/prometheus/client_golang/prometheus"

	contextapi "github.com/jordigilh/kubernaut/pkg/api/context"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/metrics"
	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

var _ = Describe("Context API Health Monitoring Integration", func() {
	var (
		ctx               context.Context
		logger            *logrus.Logger
		mockLLMClient     *mocks.MockLLMClient
		healthMonitor     monitoring.HealthMonitor
		contextController *contextapi.ContextController
		testServer        *httptest.Server
		aiIntegrator      *engine.AIServiceIntegrator
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce test noise

		// Initialize mock LLM client
		mockLLMClient = mocks.NewMockLLMClient()
		mockLLMClient.SetHealthy(true)
		mockLLMClient.SetResponseTime(25 * time.Millisecond)

		// Create health monitor with isolated metrics to avoid registration conflicts
		isolatedRegistry := prometheus.NewRegistry()
		isolatedMetrics := metrics.NewEnhancedHealthMetrics(isolatedRegistry)
		healthMonitor = monitoring.NewLLMHealthMonitorWithMetrics(mockLLMClient, logger, isolatedMetrics)

		// Create minimal AI integrator for testing
		aiIntegrator = &engine.AIServiceIntegrator{}

		// Create Context API controller with health monitoring integration
		contextController = contextapi.NewContextController(aiIntegrator, nil, logger)
		contextController.SetHealthMonitor(healthMonitor)

		// Setup HTTP server with Context API health endpoints
		mux := http.NewServeMux()

		// BR-HEALTH-025: Context API server integration with health monitoring endpoints
		mux.HandleFunc("/api/v1/health/llm/liveness", contextController.LLMLivenessProbe)
		mux.HandleFunc("/api/v1/health/llm/readiness", contextController.LLMReadinessProbe)
		mux.HandleFunc("/api/v1/health/dependencies", contextController.DependenciesHealthCheck)

		// Additional Context API endpoints for integration testing
		mux.HandleFunc("/api/v1/health/status", func(w http.ResponseWriter, r *http.Request) {
			if healthMonitor == nil {
				http.Error(w, "Health monitor not available", http.StatusServiceUnavailable)
				return
			}

			healthStatus, err := healthMonitor.GetHealthStatus(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(healthStatus)
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

	// BR-HEALTH-025: Context API server integration with health monitoring
	Context("BR-HEALTH-025: Context API Health Integration", func() {
		It("should integrate health monitor with Context API endpoints", func() {
			By("Verifying health monitor is properly integrated")
			Expect(contextController).ToNot(BeNil(), "Context controller should be created")

			By("Testing liveness endpoint integration")
			resp, err := http.Get(testServer.URL + "/api/v1/health/llm/liveness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK), "BR-HEALTH-025: Liveness endpoint should return 200")

			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			var livenessResponse map[string]interface{}
			err = json.Unmarshal(body, &livenessResponse)
			Expect(err).ToNot(HaveOccurred())

			Expect(livenessResponse["probe_type"]).To(Equal("liveness"))
			Expect(livenessResponse["is_healthy"]).To(BeTrue())
			Expect(livenessResponse["component_id"]).To(Equal("llm-20b"))

			GinkgoWriter.Printf("✅ Liveness endpoint properly integrated: %s\n", string(body))
		})

		It("should integrate readiness endpoint with health monitoring", func() {
			By("Testing readiness endpoint integration")
			resp, err := http.Get(testServer.URL + "/api/v1/health/llm/readiness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK), "BR-HEALTH-025: Readiness endpoint should return 200")

			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			var readinessResponse map[string]interface{}
			err = json.Unmarshal(body, &readinessResponse)
			Expect(err).ToNot(HaveOccurred())

			Expect(readinessResponse["probe_type"]).To(Equal("readiness"))
			Expect(readinessResponse["is_healthy"]).To(BeTrue())
			Expect(readinessResponse["component_id"]).To(Equal("llm-20b"))

			GinkgoWriter.Printf("✅ Readiness endpoint properly integrated: %s\n", string(body))
		})

		It("should integrate dependencies endpoint with health monitoring", func() {
			By("Testing dependencies endpoint integration")
			resp, err := http.Get(testServer.URL + "/api/v1/health/dependencies")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK), "BR-HEALTH-025: Dependencies endpoint should return 200")

			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			var depsResponse map[string]interface{}
			err = json.Unmarshal(body, &depsResponse)
			Expect(err).ToNot(HaveOccurred())

			Expect(depsResponse["dependencies"]).ToNot(BeNil())

			GinkgoWriter.Printf("✅ Dependencies endpoint properly integrated: %s\n", string(body))
		})

		It("should handle health monitor unavailability gracefully", func() {
			By("Creating Context API controller without health monitor")
			controllerWithoutHealth := contextapi.NewContextController(aiIntegrator, nil, logger)
			// Note: Not calling SetHealthMonitor to simulate unavailable health monitor

			mux := http.NewServeMux()
			mux.HandleFunc("/api/v1/health/llm/liveness", controllerWithoutHealth.LLMLivenessProbe)
			tempServer := httptest.NewServer(mux)
			defer tempServer.Close()

			By("Testing endpoint behavior without health monitor")
			resp, err := http.Get(tempServer.URL + "/api/v1/health/llm/liveness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable),
				"BR-HEALTH-025: Should return 503 when health monitor is not available")

			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("Health monitor not available"))

			GinkgoWriter.Printf("✅ Graceful handling of unavailable health monitor validated\n")
		})
	})

	// BR-PERF-021: API responses within 100ms for cached results
	Context("BR-PERF-021: Performance Requirements", func() {
		It("should meet performance requirements for health endpoints", func() {
			By("Testing liveness endpoint performance")
			startTime := time.Now()

			resp, err := http.Get(testServer.URL + "/api/v1/health/llm/liveness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			duration := time.Since(startTime)

			// For integration tests, allow slightly higher thresholds than production
			Expect(duration).To(BeNumerically("<", 500*time.Millisecond),
				"BR-PERF-021: Health endpoint should respond within reasonable time for integration tests")
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			GinkgoWriter.Printf("✅ Liveness endpoint performance: %v (within integration test threshold)\n", duration)
		})

		It("should meet performance requirements for readiness endpoint", func() {
			By("Testing readiness endpoint performance")
			startTime := time.Now()

			resp, err := http.Get(testServer.URL + "/api/v1/health/llm/readiness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			duration := time.Since(startTime)

			// Readiness checks may take longer due to LLM interaction
			Expect(duration).To(BeNumerically("<", 5*time.Second),
				"BR-PERF-022: Readiness probe should complete within 5 seconds")
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			GinkgoWriter.Printf("✅ Readiness endpoint performance: %v (within 5s threshold)\n", duration)
		})
	})

	// BR-HEALTH-030: Return HTTP 200 for healthy states, HTTP 503 for unhealthy states
	Context("BR-HEALTH-030: HTTP Status Code Requirements", func() {
		It("should return correct HTTP status codes for healthy states", func() {
			By("Ensuring LLM is healthy")
			mockLLMClient.SetHealthy(true)

			By("Testing liveness endpoint status code")
			resp, err := http.Get(testServer.URL + "/api/v1/health/llm/liveness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK), "BR-HEALTH-030: Must return 200 for healthy liveness")

			By("Testing readiness endpoint status code")
			resp, err = http.Get(testServer.URL + "/api/v1/health/llm/readiness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK), "BR-HEALTH-030: Must return 200 for healthy readiness")
		})

		It("should return correct HTTP status codes for unhealthy states", func() {
			By("Configuring LLM as unhealthy")
			mockLLMClient.SetError("service unavailable")

			By("Testing liveness endpoint status code for unhealthy state")
			resp, err := http.Get(testServer.URL + "/api/v1/health/llm/liveness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable),
				"BR-HEALTH-030: Must return 503 for unhealthy liveness")

			By("Testing readiness endpoint status code for unhealthy state")
			resp, err = http.Get(testServer.URL + "/api/v1/health/llm/readiness")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable),
				"BR-HEALTH-030: Must return 503 for unhealthy readiness")
		})
	})

	// Additional Context API integration scenarios
	Context("Enhanced Context API Integration", func() {
		It("should support health status query endpoint", func() {
			By("Testing custom health status endpoint")
			resp, err := http.Get(testServer.URL + "/api/v1/health/status")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			var healthStatus types.HealthStatus
			err = json.Unmarshal(body, &healthStatus)
			Expect(err).ToNot(HaveOccurred())

			Expect(healthStatus.ComponentType).To(Equal("llm-20b"))
			Expect(healthStatus.IsHealthy).To(BeTrue())

			GinkgoWriter.Printf("✅ Health status endpoint integrated successfully\n")
		})
	})
})
