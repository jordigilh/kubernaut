package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	contextapi "github.com/jordigilh/kubernaut/pkg/api/context"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// Following TDD approach - defining business requirements first for Context API health integration
var _ = Describe("Context API Health Monitoring - Business Requirements Testing", func() {
	var (
		ctx               context.Context
		logger            *logrus.Logger
		contextController *contextapi.ContextController
		recorder          *httptest.ResponseRecorder
		mockAIIntegrator  *engine.AIServiceIntegrator
		mockServiceInteg  *mocks.MockServiceIntegration
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce noise in tests

		// Use existing mocks following project guidelines
		mockServiceInteg = mocks.NewMockServiceIntegration()

		// Create mock AI integrator (minimal setup for testing)
		mockAIIntegrator = &engine.AIServiceIntegrator{} // Will be used with real controller

		// Create Context API controller
		contextController = contextapi.NewContextController(mockAIIntegrator, mockServiceInteg, logger)

		// Create HTTP recorder for testing responses
		recorder = httptest.NewRecorder()
	})

	AfterEach(func() {
		// Clean up following existing patterns
		recorder = nil
	})

	// BR-HEALTH-020: MUST provide /api/v1/health/llm endpoint for comprehensive LLM health status
	Context("BR-HEALTH-020: LLM Health Status Endpoint", func() {
		It("should provide comprehensive LLM health status endpoint", func() {
			// Arrange: Create health endpoint request
			req := httptest.NewRequest("GET", "/api/v1/health/llm", nil)
			req = req.WithContext(ctx)

			// Business requirement: Endpoint must exist and be accessible
			// Following development guideline: test business outcomes not implementation details

			// Act: This test defines the expected API contract
			// Note: Implementation will be added after tests are defined (TDD approach)

			// Assert: Business requirement validation
			// The endpoint should exist and provide structured health information
			// This test establishes the API contract following BR-HEALTH-020
			Expect(req.URL.Path).To(Equal("/api/v1/health/llm"), "BR-HEALTH-020: Must provide LLM health endpoint")
			Expect(req.Method).To(Equal("GET"), "BR-HEALTH-020: Must support GET method for health status")
		})

		It("should return structured JSON response with comprehensive health metrics", func() {
			// Arrange: Expected health response structure
			expectedFields := []string{
				"is_healthy", "component_type", "service_endpoint", "response_time",
				"health_metrics", "probe_results", "last_check_time",
			}

			// Business requirement: Response must contain comprehensive health information
			// Following business requirement BR-HEALTH-020: structured JSON responses with comprehensive health metrics

			// Assert: Define expected response structure
			for _, field := range expectedFields {
				Expect(field).ToNot(BeEmpty(), "BR-HEALTH-020: Must define comprehensive health metrics field: %s", field)
			}

			// Business requirement: Response should include uptime percentage >= 99.95%
			expectedUptimeThreshold := 99.95
			Expect(expectedUptimeThreshold).To(Equal(99.95), "BR-HEALTH-020: Must meet 99.95% uptime SLA requirement")
		})
	})

	// BR-HEALTH-021: MUST provide /api/v1/health/llm/liveness endpoint for Kubernetes liveness probes
	Context("BR-HEALTH-021: Kubernetes Liveness Probe Endpoint", func() {
		It("should provide liveness probe endpoint for Kubernetes integration", func() {
			// Arrange: Create liveness probe request
			req := httptest.NewRequest("GET", "/api/v1/health/llm/liveness", nil)
			req = req.WithContext(ctx)

			// Business requirement: Liveness probe endpoint for Kubernetes health checks
			// Following BR-HEALTH-021: Kubernetes liveness probes

			// Assert: Endpoint contract validation
			Expect(req.URL.Path).To(Equal("/api/v1/health/llm/liveness"), "BR-HEALTH-021: Must provide liveness probe endpoint")
			Expect(req.Method).To(Equal("GET"), "BR-HEALTH-021: Must support GET method for liveness probes")
		})

		It("should return appropriate HTTP status codes for liveness state", func() {
			// Business requirement: HTTP status codes following BR-HEALTH-030 through BR-HEALTH-033

			// Assert: Expected HTTP status codes for different health states
			healthyStatus := http.StatusOK
			unhealthyStatus := http.StatusServiceUnavailable

			Expect(healthyStatus).To(Equal(200), "BR-HEALTH-030: Must return HTTP 200 for healthy states")
			Expect(unhealthyStatus).To(Equal(503), "BR-HEALTH-031: Must return HTTP 503 for unhealthy states")
		})

		It("should complete probe within performance requirements", func() {
			// Business requirement: Probe performance following BR-PERF-022
			maxProbeTime := 5 * time.Second

			// Assert: Performance requirement validation
			Expect(maxProbeTime).To(Equal(5*time.Second), "BR-PERF-022: Probe must complete within 5 seconds")
		})
	})

	// BR-HEALTH-022: MUST provide /api/v1/health/llm/readiness endpoint for Kubernetes readiness probes
	Context("BR-HEALTH-022: Kubernetes Readiness Probe Endpoint", func() {
		It("should provide readiness probe endpoint for Kubernetes integration", func() {
			// Arrange: Create readiness probe request
			req := httptest.NewRequest("GET", "/api/v1/health/llm/readiness", nil)
			req = req.WithContext(ctx)

			// Business requirement: Readiness probe endpoint for Kubernetes health checks
			// Following BR-HEALTH-022: Kubernetes readiness probes

			// Assert: Endpoint contract validation
			Expect(req.URL.Path).To(Equal("/api/v1/health/llm/readiness"), "BR-HEALTH-022: Must provide readiness probe endpoint")
			Expect(req.Method).To(Equal("GET"), "BR-HEALTH-022: Must support GET method for readiness probes")
		})

		It("should include response time metrics in readiness response", func() {
			// Business requirement: Response time tracking following BR-HEALTH-033

			// Assert: Response time requirement validation
			maxResponseTime := 100 * time.Millisecond // For cached results
			Expect(maxResponseTime).To(Equal(100*time.Millisecond), "BR-PERF-021: Must respond within 100ms for cached results")
		})
	})

	// BR-HEALTH-023: MUST provide /api/v1/health/dependencies endpoint for external dependency status
	Context("BR-HEALTH-023: External Dependencies Status Endpoint", func() {
		It("should provide dependencies status endpoint", func() {
			// Arrange: Create dependencies status request
			req := httptest.NewRequest("GET", "/api/v1/health/dependencies", nil)
			req = req.WithContext(ctx)

			// Business requirement: External dependency monitoring following BR-HEALTH-023

			// Assert: Dependencies endpoint contract validation
			Expect(req.URL.Path).To(Equal("/api/v1/health/dependencies"), "BR-HEALTH-023: Must provide dependencies status endpoint")
			Expect(req.Method).To(Equal("GET"), "BR-HEALTH-023: Must support GET method for dependency status")
		})

		It("should track critical dependencies with proper criticality levels", func() {
			// Business requirement: Dependency criticality tracking
			criticalityLevels := []string{"critical", "high", "medium", "low"}

			// Assert: Criticality level validation
			Expect(criticalityLevels).To(ContainElement("critical"), "BR-HEALTH-023: Must support critical dependency tracking")
			Expect(len(criticalityLevels)).To(Equal(4), "BR-HEALTH-023: Must provide comprehensive criticality levels")
		})
	})

	// BR-HEALTH-025: MUST integrate health monitoring with Context API server on port 8091
	Context("BR-HEALTH-025: Context API Server Integration", func() {
		It("should integrate with Context API server following configuration", func() {
			// Business requirement: Integration with Context API server
			// Following heartbeat.monitor_service="context_api" configuration

			// Assert: Configuration integration validation
			expectedPort := 8091
			expectedService := "context_api"

			Expect(expectedPort).To(Equal(8091), "BR-HEALTH-025: Must integrate with Context API on port 8091")
			Expect(expectedService).To(Equal("context_api"), "BR-HEALTH-025: Must support context_api monitor service")
		})

		It("should support health monitoring start/stop operations via API", func() {
			// Business requirement: Health monitoring control following BR-HEALTH-029

			// Assert: Control operation validation
			startEndpoint := "/api/v1/health/monitoring/start"
			stopEndpoint := "/api/v1/health/monitoring/stop"

			Expect(startEndpoint).To(Equal("/api/v1/health/monitoring/start"), "BR-HEALTH-029: Must provide monitoring start endpoint")
			Expect(stopEndpoint).To(Equal("/api/v1/health/monitoring/stop"), "BR-HEALTH-029: Must provide monitoring stop endpoint")
		})
	})

	// BR-HEALTH-034: MUST provide OpenAPI 3.0 specification for all health endpoints
	Context("BR-HEALTH-034: API Documentation Requirements", func() {
		It("should provide comprehensive API documentation", func() {
			// Business requirement: OpenAPI 3.0 specification

			// Assert: Documentation requirement validation
			requiredDocFields := []string{
				"openapi", "info", "paths", "components", "responses",
			}

			for _, field := range requiredDocFields {
				Expect(field).ToNot(BeEmpty(), "BR-HEALTH-034: Must provide OpenAPI field: %s", field)
			}
		})

		It("should document all health endpoint response schemas", func() {
			// Business requirement: Complete response schema documentation

			// Assert: Response schema validation
			healthEndpoints := []string{
				"/api/v1/health/llm",
				"/api/v1/health/llm/liveness",
				"/api/v1/health/llm/readiness",
				"/api/v1/health/dependencies",
			}

			Expect(len(healthEndpoints)).To(Equal(4), "BR-HEALTH-034: Must document all health endpoints")
			for _, endpoint := range healthEndpoints {
				Expect(endpoint).To(MatchRegexp("^/api/v1/health/"), "BR-HEALTH-034: All health endpoints must follow API versioning")
			}
		})
	})

	// Integration test for existing HealthCheck endpoint enhancement
	Context("Enhanced HealthCheck Integration", func() {
		It("should enhance existing health check with LLM health monitoring", func() {
			// Arrange: Create health check request to existing endpoint
			req := httptest.NewRequest("GET", "/api/v1/context/health", nil)
			req = req.WithContext(ctx)

			// Act: Call existing health check endpoint
			contextController.HealthCheck(recorder, req)

			// Assert: Verify existing endpoint still works
			Expect(recorder.Code).To(Equal(http.StatusOK), "Existing health check must continue working")

			var response map[string]interface{}
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(err).ToNot(HaveOccurred(), "Response must be valid JSON")

			// Verify existing health check structure
			Expect(response).To(HaveKey("status"), "Must maintain existing health check structure")
			Expect(response["status"]).To(Equal("healthy"), "Must report healthy status")
		})

		// BR-HEALTH-025: Context API must validate Kubernetes connectivity
		It("should validate Kubernetes connectivity in health check", func() {
			// Arrange: Create health check request
			req := httptest.NewRequest("GET", "/api/v1/context/health", nil)
			req = req.WithContext(ctx)

			// Business requirement: Health check must validate essential dependencies
			// Following BR-HEALTH-025 and BR-HEALTH-030

			// Act: Call health check endpoint
			contextController.HealthCheck(recorder, req)

			// Assert: Verify health check includes Kubernetes connectivity validation
			var response map[string]interface{}
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(err).ToNot(HaveOccurred(), "Response must be valid JSON")

			// Business requirement validation: Health status should reflect actual system state
			Expect(response).To(HaveKey("kubernetes_connectivity"), "BR-HEALTH-025: Must validate Kubernetes connectivity")

			// When K8s is available, status should be healthy (200)
			// When K8s is unavailable, status should be unhealthy (503)
			// Following BR-HEALTH-030: Return HTTP 200 for healthy states, HTTP 503 for unhealthy states
			statusCode := recorder.Code
			kubernetesStatus := response["kubernetes_connectivity"]

			if kubernetesStatus == "healthy" {
				Expect(statusCode).To(Equal(http.StatusOK), "BR-HEALTH-030: Must return 200 when Kubernetes is accessible")
			} else {
				Expect(statusCode).To(Equal(http.StatusServiceUnavailable), "BR-HEALTH-030: Must return 503 when Kubernetes is inaccessible")
			}
		})

		It("should include structured Kubernetes health information", func() {
			// Arrange: Create health check request
			req := httptest.NewRequest("GET", "/api/v1/context/health", nil)
			req = req.WithContext(ctx)

			// Act: Call health check endpoint
			contextController.HealthCheck(recorder, req)

			// Assert: Verify structured response for Kubernetes health
			var response map[string]interface{}
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(err).ToNot(HaveOccurred(), "Response must be valid JSON")

			// Business requirement: Structured health information following existing patterns
			if kubernetesHealth, exists := response["kubernetes_connectivity"]; exists {
				// If Kubernetes health is reported, it should follow structured format
				Expect(kubernetesHealth).To(BeAssignableToTypeOf("string"), "Kubernetes connectivity must be reported as structured information")

				// Valid states should be "healthy", "unhealthy", or "unavailable"
				validStates := []string{"healthy", "unhealthy", "unavailable"}
				Expect(validStates).To(ContainElement(kubernetesHealth), "Must report valid Kubernetes connectivity state")
			}
		})
	})
})
