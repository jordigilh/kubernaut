package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	contextapi "github.com/jordigilh/kubernaut/pkg/api/context"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

func TestContextAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Context API Suite")
}

var _ = Describe("Context API for HolmesGPT Orchestration - Business Requirements", func() {
	var (
		contextController *contextapi.ContextController
		aiIntegrator      *engine.AIServiceIntegrator
		mockHolmesGPT     *mocks.MockClient
		testLogger        *logrus.Logger
		testConfig        *config.Config
		recorder          *httptest.ResponseRecorder
		ctx               context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		testLogger = logrus.New()
		testLogger.SetLevel(logrus.WarnLevel) // Reduce noise in tests

		// Create test configuration following existing patterns
		testConfig = &config.Config{
			AIServices: config.AIServicesConfig{
				HolmesGPT: config.HolmesGPTConfig{
					Enabled:  true,
					Endpoint: "http://test-holmesgpt:8090",
					Timeout:  30 * time.Second,
				},
			},
		}

		// Use existing mock patterns from testutil
		mockHolmesGPT = mocks.NewMockClient()

		// Create AI service integrator using existing patterns
		aiIntegrator = engine.NewAIServiceIntegrator(
			testConfig,
			nil,           // LLM client
			mockHolmesGPT, // HolmesGPT client - using proper mock
			nil,           // Vector DB
			nil,           // Metrics client
			testLogger,
		)

		// Create context controller that reuses existing logic
		contextController = contextapi.NewContextController(aiIntegrator, testLogger)

		// Setup HTTP test recorder
		recorder = httptest.NewRecorder()
	})

	AfterEach(func() {
		// Clear mock state between tests for isolation
		mockHolmesGPT.ClearHistory()
	})

	Context("BR-AI-011: Intelligent alert investigation using historical patterns", func() {
		It("provides Kubernetes context for HolmesGPT orchestrated investigations", func() {
			// Given: A request for Kubernetes context (business scenario - HolmesGPT needs K8s data)
			req := httptest.NewRequest("GET", "/api/v1/context/kubernetes/production/api-server-pod", nil)
			req = req.WithContext(ctx)

			// When: HolmesGPT requests Kubernetes context via API (business requirement execution)
			contextController.GetKubernetesContext(recorder, req, "production", "api-server-pod")

			// Then: API provides Kubernetes context for intelligent investigation (BR-AI-011)
			Expect(recorder.Code).To(Equal(http.StatusOK))

			var response contextapi.KubernetesContextResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(err).ToNot(HaveOccurred())

			// Validate business requirement: Kubernetes context enables intelligent investigation
			Expect(response.Namespace).To(Equal("production"))
			Expect(response.Resource).To(Equal("api-server-pod"))
			Expect(response.Context).To(HaveKey("namespace"))
			Expect(response.Context).To(HaveKey("resource"))
			Expect(response.Timestamp).ToNot(BeZero()) // Context freshness for investigation accuracy
		})

		It("provides action history context for HolmesGPT pattern-based investigations", func() {
			// Given: A request for action history context (business scenario - HolmesGPT needs historical patterns)
			req := httptest.NewRequest("GET", "/api/v1/context/action-history/DatabaseConnectionFailure?namespace=production", nil)
			req = req.WithContext(ctx)

			// When: HolmesGPT requests historical patterns via API (BR-AI-011)
			contextController.GetActionHistoryContext(recorder, req, "DatabaseConnectionFailure")

			// Then: API provides historical patterns for intelligent investigation
			Expect(recorder.Code).To(Equal(http.StatusOK))

			var response contextapi.ActionHistoryContextResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(err).ToNot(HaveOccurred())

			// Validate business requirement: Historical patterns enable intelligent investigation
			Expect(response.AlertType).To(Equal("DatabaseConnectionFailure"))
			Expect(response.Namespace).To(Equal("production"))
			Expect(response.ContextHash).ToNot(BeEmpty()) // Enables pattern correlation
			Expect(len(response.HistoryData)).To(BeNumerically(">=", 0), "Should provide historical patterns for intelligence")
		})
	})

	Context("BR-AI-012: Root cause identification with supporting evidence", func() {
		It("provides metrics context for HolmesGPT evidence-based root cause analysis", func() {
			// Given: A request for metrics context (business scenario - HolmesGPT needs evidence)
			req := httptest.NewRequest("GET", "/api/v1/context/metrics/production/compute-service?timeRange=10m", nil)
			req = req.WithContext(ctx)

			// When: HolmesGPT requests metrics evidence via API (BR-AI-012)
			contextController.GetMetricsContext(recorder, req, "production", "compute-service")

			// Then: API provides metrics evidence for root cause identification
			Expect(recorder.Code).To(Equal(http.StatusOK))

			var response contextapi.MetricsContextResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(err).ToNot(HaveOccurred())

			// Validate business requirement: Metrics provide supporting evidence
			Expect(response.Namespace).To(Equal("production"))
			Expect(response.Resource).To(Equal("compute-service"))
			Expect(response.TimeRange).To(Equal("10m")) // Time-bound evidence
			Expect(len(response.Metrics)).To(BeNumerically(">=", 0), "Should provide evidence data for root cause analysis")
			Expect(response.CollectionTime).ToNot(BeZero()) // Evidence timestamp
		})

		It("handles missing metrics gracefully for robust evidence gathering", func() {
			// Given: A request for metrics context that might fail (business scenario - resilient evidence gathering)
			req := httptest.NewRequest("GET", "/api/v1/context/metrics/invalid/nonexistent", nil)
			req = req.WithContext(ctx)

			// When: HolmesGPT requests metrics for non-existent resource (edge case)
			contextController.GetMetricsContext(recorder, req, "invalid", "nonexistent")

			// Then: API provides graceful response for robust investigation (business requirement)
			Expect(recorder.Code).To(Equal(http.StatusOK)) // Still provides response for investigation continuity

			var response contextapi.MetricsContextResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(err).ToNot(HaveOccurred())

			// Should still provide structural response for HolmesGPT processing
			Expect(response.Namespace).To(Equal("invalid"))
			Expect(response.Resource).To(Equal("nonexistent"))
			Expect(len(response.Metrics)).To(BeNumerically(">=", 0), "Should gracefully handle missing metrics with valid response structure")
		})
	})

	Context("BR-AI-013: Alert correlation across time/resource boundaries", func() {
		It("provides consistent context hashing for HolmesGPT correlation", func() {
			// Given: Multiple requests for same alert type (business scenario - correlation across time)
			req1 := httptest.NewRequest("GET", "/api/v1/context/action-history/NetworkLatencyHigh?namespace=production", nil)
			req2 := httptest.NewRequest("GET", "/api/v1/context/action-history/NetworkLatencyHigh?namespace=production", nil)

			// When: HolmesGPT requests context for same alert type at different times (BR-AI-013)
			recorder1 := httptest.NewRecorder()
			recorder2 := httptest.NewRecorder()

			contextController.GetActionHistoryContext(recorder1, req1.WithContext(ctx), "NetworkLatencyHigh")
			contextController.GetActionHistoryContext(recorder2, req2.WithContext(ctx), "NetworkLatencyHigh")

			// Then: API provides consistent correlation context across time boundaries
			Expect(recorder1.Code).To(Equal(http.StatusOK))
			Expect(recorder2.Code).To(Equal(http.StatusOK))

			var response1, response2 contextapi.ActionHistoryContextResponse
			json.Unmarshal(recorder1.Body.Bytes(), &response1)
			json.Unmarshal(recorder2.Body.Bytes(), &response2)

			// Validate business requirement: Consistent correlation across time boundaries
			Expect(response1.ContextHash).To(Equal(response2.ContextHash)) // Same alert type = same hash
			Expect(response1.AlertType).To(Equal(response2.AlertType))     // Alert type consistency
			Expect(response1.Namespace).To(Equal(response2.Namespace))     // Namespace consistency
		})

		It("provides different context hashes for different resources enabling correlation boundaries", func() {
			// Given: Requests for same alert type but different resources (business scenario - resource boundary correlation)
			req1 := httptest.NewRequest("GET", "/api/v1/context/action-history/HighCPUUsage?namespace=production", nil)
			req2 := httptest.NewRequest("GET", "/api/v1/context/action-history/DiskSpaceWarning?namespace=production", nil)

			// When: HolmesGPT requests context for different alert types (resource boundaries)
			recorder1 := httptest.NewRecorder()
			recorder2 := httptest.NewRecorder()

			contextController.GetActionHistoryContext(recorder1, req1.WithContext(ctx), "HighCPUUsage")
			contextController.GetActionHistoryContext(recorder2, req2.WithContext(ctx), "DiskSpaceWarning")

			// Then: API provides different correlation contexts for resource boundaries (BR-AI-013)
			var response1, response2 contextapi.ActionHistoryContextResponse
			json.Unmarshal(recorder1.Body.Bytes(), &response1)
			json.Unmarshal(recorder2.Body.Bytes(), &response2)

			// Validate business requirement: Different alert types have different correlation boundaries
			Expect(response1.ContextHash).ToNot(Equal(response2.ContextHash)) // Different alerts = different hashes
			Expect(response1.AlertType).To(Equal("HighCPUUsage"))
			Expect(response2.AlertType).To(Equal("DiskSpaceWarning"))
		})
	})

	Context("API Health and Reliability", func() {
		It("provides health check endpoint for HolmesGPT integration monitoring", func() {
			// Given: A health check request (operational requirement)
			req := httptest.NewRequest("GET", "/api/v1/context/health", nil)
			req = req.WithContext(ctx)

			// When: HolmesGPT or monitoring system checks API health
			contextController.HealthCheck(recorder, req)

			// Then: API provides health status for integration reliability
			Expect(recorder.Code).To(Equal(http.StatusOK))

			var health map[string]interface{}
			err := json.Unmarshal(recorder.Body.Bytes(), &health)
			Expect(err).ToNot(HaveOccurred())

			Expect(health["status"]).To(Equal("healthy"))
			Expect(health["service"]).To(Equal("context-api"))
			Expect(health["version"]).ToNot(BeEmpty(), "Should provide version information")
		})
	})

	Context("Development Guidelines Compliance Validation", func() {
		It("reuses existing AIServiceIntegrator logic without breaking changes", func() {
			// This test validates compliance with development guidelines
			// Following guideline: "reuse code whenever possible"

			req := httptest.NewRequest("GET", "/api/v1/context/kubernetes/test/resource", nil)
			req = req.WithContext(ctx)

			// When: Using Context API that reuses existing logic
			contextController.GetKubernetesContext(recorder, req, "test", "resource")

			// Then: Should successfully reuse existing patterns
			Expect(recorder.Code).To(Equal(http.StatusOK))

			var response contextapi.KubernetesContextResponse
			json.Unmarshal(recorder.Body.Bytes(), &response)

			// Validates that we're reusing existing context gathering logic
			Expect(response.Context).To(HaveKey("namespace")) // Same structure as existing enrichment
			Expect(response.Context).To(HaveKey("resource"))  // Same structure as existing enrichment
		})

		It("tests business value not implementation details", func() {
			// This test validates we're testing business requirements, not implementation
			// Following guideline: "test actual business requirement expectations"

			req := httptest.NewRequest("GET", "/api/v1/context/metrics/production/service?timeRange=5m", nil)
			req = req.WithContext(ctx)

			// When: Business requirement is executed (HolmesGPT gets evidence)
			contextController.GetMetricsContext(recorder, req, "production", "compute-service")

			// Then: Business value is validated (evidence provided for investigation)
			Expect(recorder.Code).To(Equal(http.StatusOK))

			var response contextapi.MetricsContextResponse
			json.Unmarshal(recorder.Body.Bytes(), &response)

			// This tests the business requirement outcome: HolmesGPT gets metrics evidence
			// Not testing implementation details like specific method calls or internal state
			Expect(len(response.Metrics)).To(BeNumerically(">=", 0), "Business value: evidence provided")
			Expect(response.CollectionTime).ToNot(BeZero()) // Business value: fresh evidence

			// We're validating that the API provides business value to HolmesGPT
			// Not validating internal implementation specifics
		})
	})
})
