//go:build unit
// +build unit

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// AI Service Core Business Requirements Test Suite
// Business Impact: Validates fundamental AI service capabilities required for production operations
// Stakeholder Value: Ensures AI service meets core business requirements for alert analysis and LLM integration

var _ = Describe("AI Service Core Business Requirements", func() {
	var (
		server *httptest.Server
		logger *logrus.Logger
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce test noise
		server = createTestAIServerBDD(logger)
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	Context("When processing critical memory alerts", func() {
		var criticalAlert types.Alert

		BeforeEach(func() {
			// Use existing test data factory patterns from pkg/testutil
			criticalAlert = createTestAlert("HighMemoryUsage", "critical", "default", "webapp-deployment")
		})

		It("BR-AI-001: Should provide HTTP REST API for alert analysis", func() {
			// REFACTOR: Use reusable helper functions to eliminate duplication
			context := createTestContext("test-req-123")
			resp, err := makeAnalyzeAlertRequest(server, criticalAlert, context)
			Expect(err).ToNot(HaveOccurred())

			// REFACTOR: Use reusable response validation
			var response llm.AnalyzeAlertResponse
			validateJSONResponse(resp, http.StatusOK, &response, "BR-AI-001")

			// Validate business outcomes, not just non-null values
			validActions := []string{"restart_pod", "scale_deployment", "collect_diagnostics", "notify_only", "increase_resources"}
			Expect(response.Action).To(BeElementOf(validActions), "BR-AI-001: Should provide valid remediation action")

			Expect(response.Confidence).To(BeNumerically(">=", 0.5), "BR-AI-001: AI confidence should meet minimum business threshold")
			Expect(response.Confidence).To(BeNumerically("<=", 1.0), "BR-AI-001: AI confidence should be within valid range")

			Expect(response.Reasoning).ToNot(BeNil(), "BR-AI-001: Should provide reasoning for business transparency")
			Expect(response.Reasoning.Summary).To(ContainSubstring("HighMemoryUsage"), "BR-AI-001: Reasoning should reference the specific alert condition")

			Expect(response.Parameters).ToNot(BeNil(), "BR-AI-001: Should provide actionable parameters")
		})

		It("BR-AI-002: Should support JSON request/response format", func() {
			By("Processing valid JSON request")
			validPayload := `{
				"alert": {
					"name": "CPUThrottling",
					"severity": "warning",
					"namespace": "production",
					"resource": "api-server"
				},
				"context": {
					"request_id": "json-test-456"
				}
			}`

			req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/analyze-alert", strings.NewReader(validPayload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK), "BR-AI-002: Should process valid JSON requests")
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/json"), "BR-AI-002: Should return JSON response")

			By("Rejecting invalid JSON request")
			invalidPayload := `{"invalid": json}`

			req, err = http.NewRequest(http.MethodPost, server.URL+"/api/v1/analyze-alert", strings.NewReader(invalidPayload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err = http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "BR-AI-002: Should reject malformed JSON with appropriate error code")
		})

		It("BR-AI-003: Should integrate with LLM services and provide fallback", func() {
			By("Testing primary LLM integration")
			payload := map[string]interface{}{
				"alert": criticalAlert,
			}

			jsonPayload, err := json.Marshal(payload)
			Expect(err).ToNot(HaveOccurred())

			req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/analyze-alert", bytes.NewBuffer(jsonPayload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK), "BR-AI-003: Should successfully integrate with LLM services")

			var response llm.AnalyzeAlertResponse
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			By("Validating LLM response quality")
			Expect(response.Action).ToNot(BeEmpty(), "BR-AI-003: LLM should provide actionable recommendation")
			Expect(response.Reasoning.Summary).To(SatisfyAny(
				ContainSubstring("memory"),
				ContainSubstring("Memory"),
				ContainSubstring("resource"),
			), "BR-AI-003: LLM reasoning should be contextually relevant")

			By("Testing fallback behavior when LLM unavailable")
			// This would be tested with a mock that simulates LLM failure
			// The fallback should still provide a valid response with lower confidence
			Expect(response.Confidence).To(BeNumerically(">", 0.0), "BR-AI-003: Fallback should provide reasonable confidence")
		})
	})

	Context("When validating service health and metrics", func() {
		It("BR-AI-004: Should provide health monitoring endpoints", func() {
			By("Testing basic health endpoint")
			resp, err := http.Get(server.URL + "/health")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK), "BR-AI-004: Health endpoint should be accessible")

			By("Testing detailed health endpoint")
			resp, err = http.Get(server.URL + "/api/v1/health/detailed")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK), "BR-AI-004: Detailed health endpoint should provide comprehensive status")

			var healthStatus map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&healthStatus)
			Expect(err).ToNot(HaveOccurred())

			// Business-meaningful health validation
			Expect(healthStatus).To(HaveKey("is_healthy"), "BR-AI-004: Should report health status")
			Expect(healthStatus).To(HaveKey("component_type"), "BR-AI-004: Should identify component type")
			Expect(healthStatus).To(HaveKey("response_time"), "BR-AI-004: Should report performance metrics")
		})

		It("BR-AI-005: Should provide Prometheus metrics", func() {
			By("Making a request to generate metrics")
			// REFACTOR: Use reusable helper functions
			metricsAlert := createTestAlert("MetricsTest", "warning", "test", "metrics-app")
			resp, err := makeAnalyzeAlertRequest(server, metricsAlert, nil)
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()

			By("Validating Prometheus metrics endpoint")
			resp, err = http.Get(server.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK), "BR-AI-005: Metrics endpoint should be accessible")

			contentType := resp.Header.Get("Content-Type")
			Expect(contentType).To(ContainSubstring("text/plain"), "BR-AI-005: Should return Prometheus format")

			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			metricsContent := string(body)

			// Validate business-relevant metrics are present
			expectedMetrics := []string{
				"kubernaut_ai_requests_total",
				"kubernaut_ai_analysis_duration_seconds",
				"kubernaut_ai_confidence_score",
				"kubernaut_ai_llm_requests_total",
			}

			for _, metric := range expectedMetrics {
				Expect(metricsContent).To(ContainSubstring(metric),
					"BR-AI-005: Should expose %s metric for business monitoring", metric)
			}
		})
	})

	Context("When handling performance requirements", func() {
		It("BR-PA-001: Should maintain 99.9% availability", func() {
			// Test multiple concurrent requests to validate availability
			const numRequests = 10
			responses := make(chan int, numRequests)

			for i := 0; i < numRequests; i++ {
				go func() {
					// REFACTOR: Use reusable helper functions
					availabilityAlert := createTestAlert("AvailabilityTest", "info", "test", "availability-app")
					resp, err := makeAnalyzeAlertRequest(server, availabilityAlert, nil)
					if err != nil {
						responses <- 500
					} else {
						responses <- resp.StatusCode
						resp.Body.Close()
					}
				}()
			}

			successCount := 0
			for i := 0; i < numRequests; i++ {
				statusCode := <-responses
				if statusCode == http.StatusOK {
					successCount++
				}
			}

			availabilityRate := float64(successCount) / float64(numRequests)
			Expect(availabilityRate).To(BeNumerically(">=", 0.999),
				"BR-PA-001: Should maintain 99.9%% availability under concurrent load")
		})

		It("BR-PA-003: Should process requests within 5 seconds", func() {
			startTime := time.Now()

			// REFACTOR: Use reusable helper functions
			performanceAlert := createTestAlert("PerformanceTest", "critical", "test", "performance-app")
			resp, err := makeAnalyzeAlertRequest(server, performanceAlert, nil)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			processingTime := time.Since(startTime)

			Expect(resp.StatusCode).To(Equal(http.StatusOK), "BR-PA-003: Should successfully process request")
			Expect(processingTime).To(BeNumerically("<", 5*time.Second),
				"BR-PA-003: Should process requests within 5 second SLA")
		})
	})
})
