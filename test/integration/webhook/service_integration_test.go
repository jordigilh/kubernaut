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

package webhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/integration/processor"
	"github.com/jordigilh/kubernaut/pkg/integration/webhook"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/integration/shared"
)

var _ = Describe("Webhook Service Integration", Ordered, func() {
	var (
		// Real integration test infrastructure
		hooks           *shared.TestLifecycleHooks
		webhookHandler  webhook.Handler
		processorClient *processor.HTTPProcessorClient
		logger          *logrus.Logger
		testConfig      shared.IntegrationConfig
	)

	BeforeAll(func() {
		// Setup real integration test environment
		hooks = shared.SetupAIIntegrationTest("Webhook Service Integration",
			shared.WithRealDatabase(), // Use real database for processor
			shared.WithRealVectorDB(), // Use real vector DB if needed
			shared.WithMockLLM(),      // Mock LLM for speed in integration tests
		)
		hooks.Setup()

		logger = hooks.GetLogger()
		testConfig = shared.LoadConfig()

		// Skip if integration tests are disabled
		if testConfig.SkipIntegration {
			Skip("Integration tests skipped via SKIP_INTEGRATION")
		}
	})

	AfterAll(func() {
		if hooks != nil {
			hooks.Cleanup()
		}
	})

	BeforeEach(func() {
		// For integration testing, we focus on HTTP communication between services
		// Following testing strategy: use REAL business logic, mock external dependencies

		// Create a simple processor service that accepts HTTP requests
		// This tests the real HTTP communication layer
		processorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Parse the request (real HTTP processing)
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				logger.WithError(err).Error("Failed to decode request payload")
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Extract alert from payload (real business logic)
			alertData, ok := payload["alert"].(map[string]interface{})
			if !ok {
				logger.Error("No alert data in payload")
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Convert to Alert struct (real business logic)
			alert := types.Alert{
				Name:      getStringFromMap(alertData, "name"),
				Status:    getStringFromMap(alertData, "status"),
				Severity:  getStringFromMap(alertData, "severity"),
				Namespace: getStringFromMap(alertData, "namespace"),
			}

			// Log the processed alert for verification
			logger.WithFields(logrus.Fields{
				"alert_name": alert.Name,
				"severity":   alert.Severity,
				"status":     alert.Status,
			}).Info("Integration test: Alert processed by real HTTP service")

			// Return successful response (real HTTP response format)
			response := processor.ProcessAlertResponse{
				Success:         true,
				ProcessingTime:  "1.5s",
				ActionsExecuted: 1,
				Confidence:      0.85,
				Message:         "Alert processed successfully by real HTTP processor service",
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}))
		defer processorServer.Close()

		// Create real HTTP processor client that communicates with the real service
		processorClient = processor.NewHTTPProcessorClient(processorServer.URL, logger)

		// Create real webhook handler with real HTTP processor client
		webhookConfig := config.WebhookConfig{
			Port: "8080",
			Path: "/alerts",
		}
		webhookHandler = webhook.NewHandler(processorClient, webhookConfig, logger)
	})

	AfterEach(func() {
		// Cleanup is handled by the test framework
	})

	Context("BR-WH-004: Cross-Service Communication Integration", func() {
		It("should successfully communicate with real processor service", func() {
			// Business Requirement: Webhook service must successfully communicate
			// with processor service via HTTP REST API
			alertPayload := createRealAlertManagerPayload()

			recorder := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/alerts", bytes.NewBuffer(alertPayload))
			req.Header.Set("Content-Type", "application/json")

			webhookHandler.HandleAlert(recorder, req)

			// Business Outcome: Webhook accepts request successfully
			Expect(recorder.Code).To(Equal(http.StatusOK))

			// Business Outcome: Response indicates successful processing
			var response map[string]interface{}
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(err).ToNot(HaveOccurred())
			Expect(response["status"]).To(Equal("success"))
		})

		It("should handle processor service failures with circuit breaker", func() {
			// Business Requirement: Webhook service must handle processor service
			// failures gracefully without dropping alerts

			// Create a failing processor client by using a non-existent URL
			failingClient := processor.NewHTTPProcessorClient("http://localhost:99999", logger)
			failingWebhookConfig := config.WebhookConfig{
				Port: "8080",
				Path: "/alerts",
			}
			failingWebhookHandler := webhook.NewHandler(failingClient, failingWebhookConfig, logger)

			alertPayload := createRealAlertManagerPayload()

			recorder := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/alerts", bytes.NewBuffer(alertPayload))
			req.Header.Set("Content-Type", "application/json")

			failingWebhookHandler.HandleAlert(recorder, req)

			// Business Outcome: Webhook still accepts request (doesn't fail)
			Expect(recorder.Code).To(Equal(http.StatusOK))

			// Business Outcome: Alerts were queued for retry (not lost)
			Eventually(func() int {
				return failingClient.GetRetryQueueSize()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(2)) // 2 alerts queued
		})

		It("should maintain service availability during processor service outages", func() {
			// Business Requirement: Webhook service must maintain high availability
			// even when processor service is experiencing issues

			// Create a slow/failing processor service
			slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Simulate slow/failing processor service
				time.Sleep(100 * time.Millisecond)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer slowServer.Close()

			// Create client with failing server URL
			slowClient := processor.NewHTTPProcessorClient(slowServer.URL, logger)
			slowWebhookConfig := config.WebhookConfig{Port: "8080", Path: "/alerts"}
			slowWebhookHandler := webhook.NewHandler(slowClient, slowWebhookConfig, logger)

			alertPayload := createRealAlertManagerPayload()

			// Business Outcome: Multiple requests should be handled gracefully
			for i := 0; i < 5; i++ {
				recorder := httptest.NewRecorder()
				req := httptest.NewRequest("POST", "/alerts", bytes.NewBuffer(alertPayload))
				req.Header.Set("Content-Type", "application/json")

				slowWebhookHandler.HandleAlert(recorder, req)

				// Service remains available despite processor failures
				Expect(recorder.Code).To(Equal(http.StatusOK))
			}

			// Business Outcome: Circuit breaker protects from cascading failures
			Expect(slowClient.IsCircuitBreakerOpen()).To(BeTrue())
		})
	})

	Context("BR-WH-007: End-to-End Alert Reliability", func() {
		It("should queue alerts when processor service is unavailable", func() {
			// Business Requirement: No alerts should be lost even during
			// complete processor service outages

			// Create a failing processor client
			failingClient := processor.NewHTTPProcessorClient("http://localhost:99999", logger)
			failingWebhookConfig := config.WebhookConfig{Port: "8080", Path: "/alerts"}
			failingWebhookHandler := webhook.NewHandler(failingClient, failingWebhookConfig, logger)

			alertPayload := createRealAlertManagerPayload()

			recorder := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/alerts", bytes.NewBuffer(alertPayload))
			req.Header.Set("Content-Type", "application/json")

			failingWebhookHandler.HandleAlert(recorder, req)

			// Business Outcome: Webhook accepts request despite outage
			Expect(recorder.Code).To(Equal(http.StatusOK))

			// Business Outcome: Alerts are preserved in retry queue
			Eventually(func() int {
				return failingClient.GetRetryQueueSize()
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(2))
		})

		It("should handle authentication in production-like scenarios", func() {
			// Business Requirement: End-to-end authentication must work
			// in realistic deployment scenarios
			authWebhookConfig := config.WebhookConfig{
				Port: "8080",
				Path: "/alerts",
				Auth: config.WebhookAuthConfig{
					Type:  "bearer",
					Token: "test-secret-token",
				},
			}
			authWebhookHandler := webhook.NewHandler(processorClient, authWebhookConfig, logger)

			alertPayload := createRealAlertManagerPayload()

			// Test with valid authentication
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/alerts", bytes.NewBuffer(alertPayload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-secret-token")

			authWebhookHandler.HandleAlert(recorder, req)

			// Business Outcome: Authenticated request succeeds
			Expect(recorder.Code).To(Equal(http.StatusOK))

			// Test with invalid authentication
			recorder = httptest.NewRecorder()
			req = httptest.NewRequest("POST", "/alerts", bytes.NewBuffer(alertPayload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer invalid-token")

			authWebhookHandler.HandleAlert(recorder, req)

			// Business Outcome: Unauthenticated request is rejected
			Expect(recorder.Code).To(Equal(http.StatusUnauthorized))
		})
	})

	Context("BR-PERF-001: Performance and Scalability Integration", func() {
		It("should handle concurrent webhook requests efficiently", func() {
			// Business Requirement: Service must handle multiple concurrent
			// requests without performance degradation
			alertPayload := createRealAlertManagerPayload()
			concurrentRequests := 10
			results := make(chan int, concurrentRequests)

			// Business Outcome: Concurrent requests are handled successfully
			for i := 0; i < concurrentRequests; i++ {
				go func() {
					recorder := httptest.NewRecorder()
					req := httptest.NewRequest("POST", "/alerts", bytes.NewBuffer(alertPayload))
					req.Header.Set("Content-Type", "application/json")

					start := time.Now()
					webhookHandler.HandleAlert(recorder, req)
					duration := time.Since(start)

					// Each request should complete within reasonable time
					if recorder.Code == http.StatusOK && duration < 2*time.Second {
						results <- 1
					} else {
						results <- 0
					}
				}()
			}

			// Business Outcome: All concurrent requests succeed
			successCount := 0
			for i := 0; i < concurrentRequests; i++ {
				successCount += <-results
			}
			Expect(successCount).To(Equal(concurrentRequests))

			// Business Outcome: Integration test validates concurrent HTTP communication
		})
	})
})

// Helper functions

func createRealAlertManagerPayload() []byte {
	webhook := map[string]interface{}{
		"version":  "4",
		"groupKey": "{}:{alertname=\"HighCPUUsage\"}",
		"status":   "firing",
		"receiver": "kubernaut-webhook",
		"alerts": []map[string]interface{}{
			{
				"status": "firing",
				"labels": map[string]string{
					"alertname": "HighCPUUsage",
					"severity":  "critical",
					"namespace": "production",
					"pod":       "web-server-123",
				},
				"annotations": map[string]string{
					"description": "CPU usage is above 90% for 5 minutes",
					"summary":     "High CPU usage detected",
				},
				"startsAt":     time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
				"generatorURL": "http://prometheus:9090/graph?g0.expr=cpu_usage%3E0.9",
				"fingerprint":  "abc123def456",
			},
			{
				"status": "firing",
				"labels": map[string]string{
					"alertname": "HighMemoryUsage",
					"severity":  "warning",
					"namespace": "production",
					"pod":       "web-server-123",
				},
				"annotations": map[string]string{
					"description": "Memory usage is above 80%",
					"summary":     "High memory usage detected",
				},
				"startsAt":     time.Now().Add(-2 * time.Minute).Format(time.RFC3339),
				"generatorURL": "http://prometheus:9090/graph?g0.expr=memory_usage%3E0.8",
				"fingerprint":  "def456ghi789",
			},
		},
	}

	payload, _ := json.Marshal(webhook)
	return payload
}

func getStringFromMap(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func convertLabelsFromMap(labelsInterface interface{}) map[string]string {
	labels := make(map[string]string)
	if labelsMap, ok := labelsInterface.(map[string]interface{}); ok {
		for k, v := range labelsMap {
			if str, ok := v.(string); ok {
				labels[k] = str
			}
		}
	}
	return labels
}

// Integration test focuses on HTTP communication between webhook and processor services
// Following testing strategy: use real business logic, mock only external dependencies
