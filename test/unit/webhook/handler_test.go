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
package webhook_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/integration/webhook"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("BR-WH-001: Webhook Handler Enhancement", func() {
	var (
		handler       webhook.Handler
		mockProcessor *MockProcessor
		webhookConfig config.WebhookConfig
		logger        *logrus.Logger
		req           *http.Request
		recorder      *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		// Create mock processor - MUST use existing processor interface
		mockProcessor = &MockProcessor{}
		mockProcessor.SetShouldProcessResult(true) // Allow processing by default

		// Configure webhook
		webhookConfig = config.WebhookConfig{
			Port: "8080",
			Path: "/alerts",
			Auth: config.WebhookAuthConfig{
				Type:  "bearer",
				Token: "test-token",
			},
		}

		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

		// Create handler using existing interface - ENHANCE EXISTING
		handler = webhook.NewHandler(mockProcessor, webhookConfig, logger)
		recorder = httptest.NewRecorder()
	})

	Context("BR-WH-001: Webhook Endpoint Management", func() {
		It("should handle valid AlertManager webhooks", func() {
			// This test MUST fail initially - TDD RED phase
			req = createValidAlertManagerRequest()

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))
			Expect(mockProcessor.ProcessAlertCallCount()).To(Equal(2)) // 2 alerts in payload
		})

		It("should reject invalid payloads", func() {
			req = createMalformedJSONRequest() // Use truly malformed JSON

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusBadRequest))
			Expect(mockProcessor.ProcessAlertCallCount()).To(Equal(0))
		})

		It("should handle rate limiting", func() {
			// Test rate limiting functionality - MUST fail initially
			// The rate limiter allows burst of 10 requests, so we need to exhaust it
			for i := 0; i < 11; i++ { // Exceed rate limit (burst of 10)
				recorder = httptest.NewRecorder() // Reset recorder for each request
				req = createValidAlertManagerRequest()
				handler.HandleAlert(recorder, req)
			}

			Expect(recorder.Code).To(Equal(http.StatusTooManyRequests))
		})
	})

	Context("BR-WH-003: Authentication and Authorization", func() {
		It("should validate bearer tokens", func() {
			req = createRequestWithValidToken()

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))
		})

		It("should reject invalid tokens", func() {
			req = createRequestWithInvalidToken()

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusUnauthorized))
		})

		It("should reject missing authorization headers", func() {
			req = createRequestWithoutAuth()

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusUnauthorized))
		})
	})

	Context("BR-WH-002: Payload Validation", func() {
		It("should validate content type", func() {
			req = createRequestWithInvalidContentType()

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusBadRequest))
		})

		It("should validate HTTP method", func() {
			req = createGETRequest()

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusMethodNotAllowed))
		})

		It("should handle malformed JSON", func() {
			req = createMalformedJSONRequest()

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusBadRequest))
		})
	})

	Context("BR-WH-001: Webhook Endpoint Management - Health and Configuration", func() {
		It("BR-WH-001.4: should provide health check endpoint for service monitoring", func() {
			// Business Requirement: Webhook service must provide health check endpoint
			// for monitoring and operational visibility
			req := httptest.NewRequest("GET", "/health", nil)
			recorder := httptest.NewRecorder()

			handler.HealthCheck(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))

			var response webhook.WebhookResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(err).ToNot(HaveOccurred())
			Expect(response.Status).To(Equal("healthy"))
			Expect(response.Message).To(Equal("Webhook handler is running"))
		})

		It("BR-WH-001.5: should validate webhook configuration for proper service setup", func() {
			// Business Requirement: Webhook service must validate configuration
			// to ensure proper operational parameters
			validConfig := config.WebhookConfig{
				Port: "8080",
				Path: "/alerts",
				Auth: config.WebhookAuthConfig{
					Type:  "bearer",
					Token: "test-token",
				},
			}

			err := webhook.ValidateWebhookConfig(validConfig)
			Expect(err).ToNot(HaveOccurred())
		})

		It("BR-WH-001.6: should reject invalid webhook configuration to prevent service failures", func() {
			// Business Requirement: Webhook service must reject invalid configuration
			// to prevent runtime failures and ensure operational reliability
			invalidConfig := config.WebhookConfig{
				Port: "", // Missing port - violates operational requirements
				Path: "/alerts",
			}

			err := webhook.ValidateWebhookConfig(invalidConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("port is required"))
		})
	})
})

// MockProcessor implements the processor.Processor interface for testing
// Following AI assistant methodology: Use existing interfaces
type MockProcessor struct {
	processAlertCallCount int
	shouldProcessResult   bool
	processAlertError     error
}

func (m *MockProcessor) ProcessAlert(ctx context.Context, alert types.Alert) error {
	m.processAlertCallCount++
	return m.processAlertError
}

func (m *MockProcessor) ShouldProcess(alert types.Alert) bool {
	return m.shouldProcessResult
}

func (m *MockProcessor) ProcessAlertCallCount() int {
	return m.processAlertCallCount
}

func (m *MockProcessor) SetProcessAlertError(err error) {
	m.processAlertError = err
}

func (m *MockProcessor) SetShouldProcessResult(result bool) {
	m.shouldProcessResult = result
}

// Test helper functions - these will need to be implemented
func createValidAlertManagerRequest() *http.Request {
	payload := `{
		"version": "4",
		"groupKey": "test-group",
		"status": "firing",
		"receiver": "webhook",
		"alerts": [
			{
				"status": "firing",
				"labels": {
					"alertname": "HighCPU",
					"severity": "critical",
					"namespace": "default"
				},
				"annotations": {
					"description": "CPU usage is high"
				},
				"startsAt": "2023-01-01T00:00:00Z"
			},
			{
				"status": "firing",
				"labels": {
					"alertname": "HighMemory",
					"severity": "warning",
					"namespace": "kube-system"
				},
				"annotations": {
					"description": "Memory usage is high"
				},
				"startsAt": "2023-01-01T00:00:00Z"
			}
		]
	}`

	req := httptest.NewRequest("POST", "/alerts", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	return req
}

func createInvalidPayloadRequest() *http.Request {
	payload := `{"invalid": "payload"}` // Missing required AlertManager fields

	req := httptest.NewRequest("POST", "/alerts", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	return req
}

func createRequestWithValidToken() *http.Request {
	return createValidAlertManagerRequest()
}

func createRequestWithInvalidToken() *http.Request {
	req := createValidAlertManagerRequest()
	req.Header.Set("Authorization", "Bearer invalid-token")
	return req
}

func createRequestWithoutAuth() *http.Request {
	req := createValidAlertManagerRequest()
	req.Header.Del("Authorization")
	return req
}

func createRequestWithInvalidContentType() *http.Request {
	req := createValidAlertManagerRequest()
	req.Header.Set("Content-Type", "text/plain")
	return req
}

func createGETRequest() *http.Request {
	req := httptest.NewRequest("GET", "/alerts", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	return req
}

func createMalformedJSONRequest() *http.Request {
	payload := `{"invalid": json}` // Missing quotes around json - malformed JSON

	req := httptest.NewRequest("POST", "/alerts", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	return req
}
