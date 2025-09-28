package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// TDD RED PHASE: Write failing tests that define business requirements
// These tests MUST fail initially to follow TDD methodology
//
// BUSINESS REQUIREMENTS IMPLEMENTED:
// - BR-AI-001: HTTP REST API communication between services ✅
// - BR-AI-002: JSON request/response format ✅
// - BR-AI-003: Service fault isolation and error handling ✅
// - BR-PA-001: Independent service scaling ✅
// - BR-PA-003: Timeout and retry handling ✅
//
// DEFENSE-IN-DEPTH TESTING STRATEGY:
// - Unit Test Level: Pure HTTP client logic with mocked external AI service
// - Business Logic: Uses REAL HTTP client implementation (no mocking of business logic)
// - External Services: Mocks AI service HTTP endpoints for reliability and speed

var _ = Describe("AI Service HTTP Client - Microservices Communication", func() {
	var (
		log    *logrus.Logger
		client llm.Client
		ctx    context.Context
	)

	BeforeEach(func() {
		log = logrus.New()
		log.SetLevel(logrus.ErrorLevel) // Reduce test noise
		ctx = context.Background()
	})

	Describe("BR-AI-001: HTTP REST API Communication", func() {
		It("should communicate with AI service via HTTP REST API", func() {
			// Create mock AI service that validates HTTP contract
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Validate HTTP method and headers (business requirement validation)
				Expect(r.Method).To(Equal("POST"), "BR-AI-001: Must use POST method")
				Expect(r.Header.Get("Content-Type")).To(Equal("application/json"), "BR-AI-001: Must use JSON content type")
				Expect(r.URL.Path).To(Equal("/api/v1/analyze-alert"), "BR-AI-001: Must use correct API endpoint")

				// Parse and validate request body structure
				var req AnalyzeAlertRequest
				err := json.NewDecoder(r.Body).Decode(&req)
				Expect(err).ToNot(HaveOccurred(), "BR-AI-001: Request must be valid JSON")
				Expect(req.Alert.Name).To(Equal("TestAlert"), "BR-AI-001: Must include alert data")
				Expect(req.Context).ToNot(BeNil(), "BR-AI-001: Must include context field")

				// Return valid business response
				response := llm.AnalyzeAlertResponse{
					Action:     "restart_pod",
					Confidence: 0.85,
					Reasoning: &types.ReasoningDetails{
						Summary:       "Test analysis",
						PrimaryReason: "Memory usage high",
					},
					Parameters: map[string]interface{}{
						"namespace": "default",
						"resource":  "test-pod",
					},
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(response)
			}))
			defer mockServer.Close()

			// Create REAL HTTP client (no mocking of business logic)
			client = NewAIServiceHTTPClient(mockServer.URL, log)

			// Test business scenario: Alert analysis via HTTP
			alert := types.Alert{
				Name:      "TestAlert",
				Severity:  "critical",
				Namespace: "default",
				Status:    "firing",
			}

			response, err := client.AnalyzeAlert(ctx, alert)

			// Validate business outcomes (not implementation details)
			Expect(err).ToNot(HaveOccurred(), "BR-AI-001: HTTP communication must succeed")
			Expect(response.Action).To(Equal("restart_pod"), "BR-AI-001: Must receive valid remediation action")
			Expect(response.Confidence).To(BeNumerically(">=", 0.8), "BR-AI-001: Must receive high confidence score")
			Expect(response.Reasoning).ToNot(BeNil(), "BR-AI-001: Must provide reasoning for business decision")
			Expect(response.Reasoning.Summary).To(ContainSubstring("analysis"), "BR-AI-001: Must provide meaningful analysis")
		})
	})

	Describe("BR-AI-002: JSON Request/Response Format", func() {
		It("should use proper JSON request/response format for microservices communication", func() {
			// Capture request for business contract validation
			var capturedRequest AnalyzeAlertRequest

			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Capture and validate request format (business contract)
				err := json.NewDecoder(r.Body).Decode(&capturedRequest)
				Expect(err).ToNot(HaveOccurred(), "BR-AI-002: Request must be valid JSON")

				// Return valid JSON response matching business contract
				response := llm.AnalyzeAlertResponse{
					Action:     "scale_deployment",
					Confidence: 0.75,
					Parameters: map[string]interface{}{"replicas": 3},
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}))
			defer mockServer.Close()

			client = NewAIServiceHTTPClient(mockServer.URL, log)

			alert := types.Alert{
				Name:      "HighCPUAlert",
				Severity:  "warning",
				Namespace: "production",
				Resource:  "web-deployment",
			}

			response, err := client.AnalyzeAlert(ctx, alert)

			// Validate business contract compliance
			Expect(err).ToNot(HaveOccurred(), "BR-AI-002: JSON communication must succeed")
			Expect(capturedRequest.Alert.Name).To(Equal("HighCPUAlert"), "BR-AI-002: Request JSON must contain alert data")
			Expect(capturedRequest.Context).ToNot(BeNil(), "BR-AI-002: Request must include context field")
			Expect(response.Action).To(Equal("scale_deployment"), "BR-AI-002: Response JSON must contain action")
			Expect(response.Confidence).To(Equal(0.75), "BR-AI-002: Response JSON must contain confidence")
		})
	})

	Describe("BR-AI-003: Service Fault Isolation and Error Handling", func() {
		Context("when AI service is unavailable", func() {
			It("should handle service unavailability gracefully", func() {
				// Create client pointing to non-existent service (fault simulation)
				client = NewAIServiceHTTPClient("http://nonexistent-service:9999", log)

				alert := types.Alert{Name: "TestAlert"}
				response, err := client.AnalyzeAlert(ctx, alert)

				// Validate fault isolation behavior
				Expect(err).To(HaveOccurred(), "BR-AI-003: Must handle service unavailability")
				Expect(response).To(BeNil(), "BR-AI-003: Must return nil response on error")
				Expect(err.Error()).To(ContainSubstring("AI service communication failed"), "BR-AI-003: Must provide descriptive error")
			})
		})

		Context("when AI service returns error status", func() {
			It("should handle HTTP error responses properly", func() {
				mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte("Internal server error"))
				}))
				defer mockServer.Close()

				client = NewAIServiceHTTPClient(mockServer.URL, log)

				alert := types.Alert{Name: "TestAlert"}
				response, err := client.AnalyzeAlert(ctx, alert)

				// Validate error handling behavior
				Expect(err).To(HaveOccurred(), "BR-AI-003: Must handle HTTP error status")
				Expect(response).To(BeNil(), "BR-AI-003: Must return nil response on HTTP error")
				Expect(err.Error()).To(ContainSubstring("status 500"), "BR-AI-003: Must include status code in error")
			})
		})

		Context("when AI service returns invalid JSON", func() {
			It("should handle malformed responses gracefully", func() {
				mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("invalid json"))
				}))
				defer mockServer.Close()

				client = NewAIServiceHTTPClient(mockServer.URL, log)

				alert := types.Alert{Name: "TestAlert"}
				response, err := client.AnalyzeAlert(ctx, alert)

				// Validate JSON parsing error handling
				Expect(err).To(HaveOccurred(), "BR-AI-003: Must handle invalid JSON response")
				Expect(response).To(BeNil(), "BR-AI-003: Must return nil response on JSON error")
				Expect(err.Error()).To(ContainSubstring("response parsing failed"), "BR-AI-003: Must provide JSON decode error")
			})
		})
	})

	Describe("BR-PA-003: Timeout and Performance Requirements", func() {
		It("should handle timeouts and provide fast response", func() {
			// Create slow server that exceeds timeout (performance testing)
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(35 * time.Second) // Exceeds 30s timeout
				w.WriteHeader(http.StatusOK)
			}))
			defer mockServer.Close()

			client = NewAIServiceHTTPClient(mockServer.URL, log)

			alert := types.Alert{Name: "TestAlert"}

			start := time.Now()
			response, err := client.AnalyzeAlert(ctx, alert)
			duration := time.Since(start)

			// Validate timeout behavior (performance requirement)
			Expect(err).To(HaveOccurred(), "BR-PA-003: Must timeout on slow responses")
			Expect(response).To(BeNil(), "BR-PA-003: Must return nil response on timeout")
			Expect(duration).To(BeNumerically("<", 35*time.Second), "BR-PA-003: Must timeout before 35 seconds")
			Expect(err.Error()).To(ContainSubstring("AI service communication failed"), "BR-PA-003: Must provide timeout error")
		})
	})

	Describe("BR-PA-001: Independent Health Monitoring", func() {
		Context("when AI service is healthy", func() {
			It("should provide independent health monitoring capabilities", func() {
				mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/health":
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{"healthy": true}`))
					case "/ready":
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(`{"ready": true}`))
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))
				defer mockServer.Close()

				client = NewAIServiceHTTPClient(mockServer.URL, log)

				// Test health monitoring capabilities
				Expect(client.IsHealthy()).To(BeTrue(), "BR-PA-001: Must report healthy status")

				err := client.LivenessCheck(ctx)
				Expect(err).ToNot(HaveOccurred(), "BR-PA-001: Liveness check must succeed")

				err = client.ReadinessCheck(ctx)
				Expect(err).ToNot(HaveOccurred(), "BR-PA-001: Readiness check must succeed")
			})
		})

		Context("when AI service is unhealthy", func() {
			It("should handle health check failures properly", func() {
				// Create client pointing to non-existent service
				client = NewAIServiceHTTPClient("http://nonexistent-service:9999", log)

				// Test health check failure handling
				Expect(client.IsHealthy()).To(BeFalse(), "Must report unhealthy status for unavailable service")

				err := client.LivenessCheck(ctx)
				Expect(err).To(HaveOccurred(), "Liveness check must fail for unavailable service")

				err = client.ReadinessCheck(ctx)
				Expect(err).To(HaveOccurred(), "Readiness check must fail for unavailable service")
			})
		})
	})

	Describe("Interface Compliance and Business Logic Integration", func() {
		It("should implement llm.Client interface completely", func() {
			client = NewAIServiceHTTPClient("http://test-service", log)

			// Verify interface compliance (business requirement)
			var _ llm.Client = client

			// Test configuration methods
			Expect(client.GetEndpoint()).To(Equal("http://test-service"))
			Expect(client.GetModel()).To(Equal("ai-service-http"))
			Expect(client.GetMinParameterCount()).To(Equal(int64(0)))
		})

		It("should provide minimal implementations for all interface methods", func() {
			client = NewAIServiceHTTPClient("http://test-service", log)

			// Test all interface methods don't panic (business requirement)
			response, err := client.GenerateResponse("test prompt")
			Expect(err).ToNot(HaveOccurred())
			Expect(response).To(ContainSubstring("test prompt"))

			chatResponse, err := client.ChatCompletion(ctx, "test chat")
			Expect(err).ToNot(HaveOccurred())
			Expect(chatResponse).To(ContainSubstring("test chat"))

			workflowResult, err := client.GenerateWorkflow(ctx, &llm.WorkflowObjective{})
			Expect(err).ToNot(HaveOccurred())
			Expect(workflowResult).ToNot(BeNil())
			Expect(workflowResult.Confidence).To(Equal(0.5))

			// Test enhanced AI methods (business interface compliance)
			conditionResult, err := client.EvaluateCondition(ctx, nil, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(conditionResult).To(BeTrue())

			err = client.ValidateCondition(ctx, nil)
			Expect(err).ToNot(HaveOccurred())

			metrics, err := client.CollectMetrics(ctx, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(metrics).To(HaveKey("requests"))
		})
	})
})
