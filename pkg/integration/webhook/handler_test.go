package webhook_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	"github.com/jordigilh/kubernaut/pkg/integration/webhook"
)

// MockProcessor implements the processor.Processor interface for testing
type MockProcessor struct {
	ProcessAlertFunc   func(ctx context.Context, alert types.Alert) error
	ShouldProcessFunc  func(alert types.Alert) bool
	processedAlerts    []types.Alert
	shouldProcessCalls int
}

func (m *MockProcessor) ProcessAlert(ctx context.Context, alert types.Alert) error {
	m.processedAlerts = append(m.processedAlerts, alert)
	if m.ProcessAlertFunc != nil {
		return m.ProcessAlertFunc(ctx, alert)
	}
	return nil
}

func (m *MockProcessor) ShouldProcess(alert types.Alert) bool {
	m.shouldProcessCalls++
	if m.ShouldProcessFunc != nil {
		return m.ShouldProcessFunc(alert)
	}
	return true
}

func (m *MockProcessor) GetProcessedAlerts() []types.Alert {
	return m.processedAlerts
}

func (m *MockProcessor) GetShouldProcessCalls() int {
	return m.shouldProcessCalls
}

var _ = Describe("Webhook Handler", func() {
	var (
		handler       webhook.Handler
		mockProcessor *MockProcessor
		logger        *logrus.Logger
		webhookConfig config.WebhookConfig
		recorder      *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)
		logger.SetOutput(GinkgoWriter)

		mockProcessor = &MockProcessor{
			processedAlerts: []types.Alert{},
		}

		webhookConfig = config.WebhookConfig{
			Port: "8080",
			Path: "/alerts",
			Auth: config.WebhookAuthConfig{
				Type:  "",
				Token: "",
			},
		}

		handler = webhook.NewHandler(mockProcessor, webhookConfig, logger)
		recorder = httptest.NewRecorder()
	})

	Describe("HTTP Method Validation", func() {
		It("should reject GET requests", func() {
			req := httptest.NewRequest(http.MethodGet, "/alerts", nil)
			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusMethodNotAllowed))

			var response webhook.WebhookResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Status).To(Equal("error"))
			Expect(response.Error).To(ContainSubstring("Only POST method is allowed"))
		})

		It("should reject PUT requests", func() {
			req := httptest.NewRequest(http.MethodPut, "/alerts", nil)
			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusMethodNotAllowed))
		})

		It("should reject DELETE requests", func() {
			req := httptest.NewRequest(http.MethodDelete, "/alerts", nil)
			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusMethodNotAllowed))
		})

		It("should accept POST requests", func() {
			validPayload, _ := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "high-memory-alert.json"))
			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
			req.Header.Set("Content-Type", "application/json")

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("Content-Type Validation", func() {
		It("should reject requests without Content-Type", func() {
			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader([]byte("{}")))
			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusBadRequest))

			var response webhook.WebhookResponse
			_ = json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(response.Error).To(ContainSubstring("Content-Type must be application/json"))
		})

		It("should reject requests with wrong Content-Type", func() {
			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader([]byte("{}")))
			req.Header.Set("Content-Type", "text/plain")
			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusBadRequest))
		})

		It("should accept application/json Content-Type", func() {
			validPayload, _ := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "high-memory-alert.json"))
			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
			req.Header.Set("Content-Type", "application/json")

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))
		})

		It("should accept application/json with charset", func() {
			validPayload, _ := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "high-memory-alert.json"))
			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
			req.Header.Set("Content-Type", "application/json; charset=utf-8")

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("Authentication", func() {
		Context("when authentication is configured", func() {
			BeforeEach(func() {
				webhookConfig.Auth = config.WebhookAuthConfig{
					Type:  "bearer",
					Token: "test-secret-token",
				}
				handler = webhook.NewHandler(mockProcessor, webhookConfig, logger)
			})

			It("should reject requests without Authorization header", func() {
				validPayload, _ := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "high-memory-alert.json"))
				req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
				req.Header.Set("Content-Type", "application/json")

				handler.HandleAlert(recorder, req)

				Expect(recorder.Code).To(Equal(http.StatusUnauthorized))

				var response webhook.WebhookResponse
				_ = json.Unmarshal(recorder.Body.Bytes(), &response)
				Expect(response.Error).To(Equal("Authentication failed"))
			})

			It("should reject requests with invalid token", func() {
				validPayload, _ := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "high-memory-alert.json"))
				req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer wrong-token")

				handler.HandleAlert(recorder, req)

				Expect(recorder.Code).To(Equal(http.StatusUnauthorized))
			})

			It("should reject requests with malformed Authorization header", func() {
				validPayload, _ := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "high-memory-alert.json"))
				req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "InvalidFormat test-secret-token")

				handler.HandleAlert(recorder, req)

				Expect(recorder.Code).To(Equal(http.StatusUnauthorized))
			})

			It("should accept requests with valid bearer token", func() {
				validPayload, _ := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "high-memory-alert.json"))
				req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer test-secret-token")

				handler.HandleAlert(recorder, req)

				Expect(recorder.Code).To(Equal(http.StatusOK))
			})
		})

		Context("when authentication is not configured", func() {
			It("should accept requests without Authorization header", func() {
				validPayload, _ := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "high-memory-alert.json"))
				req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
				req.Header.Set("Content-Type", "application/json")

				handler.HandleAlert(recorder, req)

				Expect(recorder.Code).To(Equal(http.StatusOK))
			})
		})
	})

	Describe("Alert Parsing", func() {
		It("should parse a valid high memory alert", func() {
			validPayload, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "high-memory-alert.json"))
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
			req.Header.Set("Content-Type", "application/json")

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))
			Expect(mockProcessor.GetProcessedAlerts()).To(HaveLen(1))

			processedAlert := mockProcessor.GetProcessedAlerts()[0]
			Expect(processedAlert.Name).To(Equal("HighMemoryUsage"))
			Expect(processedAlert.Severity).To(Equal("warning"))
			Expect(processedAlert.Namespace).To(Equal("production"))
			Expect(processedAlert.Resource).To(Equal("webapp-deployment-5c7d8f9b4c-xyz123"))
			Expect(processedAlert.Status).To(Equal("firing"))
		})

		It("should parse a pod crash alert", func() {
			validPayload, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "pod-crash-alert.json"))
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
			req.Header.Set("Content-Type", "application/json")

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))
			Expect(mockProcessor.GetProcessedAlerts()).To(HaveLen(1))

			processedAlert := mockProcessor.GetProcessedAlerts()[0]
			Expect(processedAlert.Name).To(Equal("PodCrashLooping"))
			Expect(processedAlert.Severity).To(Equal("critical"))
			Expect(processedAlert.Annotations["last_termination_reason"]).To(Equal("OOMKilled"))
		})

		It("should handle multiple alerts in a single webhook", func() {
			validPayload, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "multiple-alerts.json"))
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
			req.Header.Set("Content-Type", "application/json")

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))
			Expect(mockProcessor.GetProcessedAlerts()).To(HaveLen(3))

			alertNames := []string{}
			for _, alert := range mockProcessor.GetProcessedAlerts() {
				alertNames = append(alertNames, alert.Name)
			}
			Expect(alertNames).To(ConsistOf("HighMemoryUsage", "CPUThrottlingHigh", "PodNotReady"))
		})

		It("should handle resolved alerts", func() {
			validPayload, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "resolved-alert.json"))
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
			req.Header.Set("Content-Type", "application/json")

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))
			Expect(mockProcessor.GetProcessedAlerts()).To(HaveLen(1))

			processedAlert := mockProcessor.GetProcessedAlerts()[0]
			Expect(processedAlert.Status).To(Equal("resolved"))
			Expect(processedAlert.EndsAt).NotTo(BeNil())
		})

		It("should reject invalid JSON payload", func() {
			invalidPayload := []byte("not valid json")
			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(invalidPayload))
			req.Header.Set("Content-Type", "application/json")

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusBadRequest))

			var response webhook.WebhookResponse
			_ = json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(response.Error).To(ContainSubstring("Invalid JSON payload"))
		})

		It("should handle empty alerts array", func() {
			emptyAlertsPayload := []byte(`{
				"version": "4",
				"groupKey": "{alertname=\"EmptyTest\"}",
				"status": "firing",
				"receiver": "slm-processor",
				"alerts": []
			}`)

			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(emptyAlertsPayload))
			req.Header.Set("Content-Type", "application/json")

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))
			Expect(mockProcessor.GetProcessedAlerts()).To(HaveLen(0))
		})
	})

	Describe("Alert Conversion", func() {
		It("should extract deployment name from labels", func() {
			validPayload, _ := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "high-memory-alert.json"))
			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
			req.Header.Set("Content-Type", "application/json")

			handler.HandleAlert(recorder, req)

			processedAlert := mockProcessor.GetProcessedAlerts()[0]
			Expect(processedAlert.Labels["deployment"]).To(Equal("webapp-deployment"))
		})

		It("should handle missing severity by defaulting to info", func() {
			payloadWithoutSeverity := []byte(`{
				"version": "4",
				"groupKey": "{alertname=\"TestAlert\"}",
				"status": "firing",
				"receiver": "slm-processor",
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "TestAlert",
						"namespace": "default"
					},
					"annotations": {},
					"startsAt": "2024-08-25T10:00:00Z"
				}]
			}`)

			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(payloadWithoutSeverity))
			req.Header.Set("Content-Type", "application/json")

			handler.HandleAlert(recorder, req)

			processedAlert := mockProcessor.GetProcessedAlerts()[0]
			Expect(processedAlert.Severity).To(Equal("info"))
		})

		It("should handle missing namespace by defaulting to default", func() {
			payloadWithoutNamespace := []byte(`{
				"version": "4",
				"groupKey": "{alertname=\"TestAlert\"}",
				"status": "firing",
				"receiver": "slm-processor",
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "TestAlert",
						"severity": "warning"
					},
					"annotations": {},
					"startsAt": "2024-08-25T10:00:00Z"
				}]
			}`)

			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(payloadWithoutNamespace))
			req.Header.Set("Content-Type", "application/json")

			handler.HandleAlert(recorder, req)

			processedAlert := mockProcessor.GetProcessedAlerts()[0]
			Expect(processedAlert.Namespace).To(Equal("default"))
		})

		It("should extract resource from pod label", func() {
			validPayload, _ := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "high-memory-alert.json"))
			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
			req.Header.Set("Content-Type", "application/json")

			handler.HandleAlert(recorder, req)

			processedAlert := mockProcessor.GetProcessedAlerts()[0]
			Expect(processedAlert.Resource).To(Equal("webapp-deployment-5c7d8f9b4c-xyz123"))
		})

		It("should preserve all labels and annotations", func() {
			validPayload, _ := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "cpu-throttling-alert.json"))
			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
			req.Header.Set("Content-Type", "application/json")

			handler.HandleAlert(recorder, req)

			processedAlert := mockProcessor.GetProcessedAlerts()[0]
			Expect(processedAlert.Labels).To(HaveKey("node"))
			Expect(processedAlert.Annotations).To(HaveKey("throttling_percentage"))
			Expect(processedAlert.Annotations["throttling_percentage"]).To(Equal("45"))
		})
	})

	Describe("Error Handling", func() {
		It("should continue processing other alerts if one fails", func() {
			callCount := 0
			mockProcessor.ProcessAlertFunc = func(ctx context.Context, alert types.Alert) error {
				callCount++
				if callCount == 2 {
					return io.ErrUnexpectedEOF // Simulate error on second alert
				}
				return nil
			}

			validPayload, _ := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "multiple-alerts.json"))
			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
			req.Header.Set("Content-Type", "application/json")

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))
			Expect(mockProcessor.GetProcessedAlerts()).To(HaveLen(3))
		})

		It("should handle processor panic gracefully", func() {
			mockProcessor.ProcessAlertFunc = func(ctx context.Context, alert types.Alert) error {
				panic("simulated panic")
			}

			validPayload, _ := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "high-memory-alert.json"))
			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
			req.Header.Set("Content-Type", "application/json")

			defer func() {
				if r := recover(); r == nil {
					Fail("Expected panic was not propagated")
				}
			}()

			handler.HandleAlert(recorder, req)
		})

		It("should handle context cancellation", func() {
			mockProcessor.ProcessAlertFunc = func(ctx context.Context, alert types.Alert) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(100 * time.Millisecond):
					return nil
				}
			}

			validPayload, _ := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "high-memory-alert.json"))
			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
			req.Header.Set("Content-Type", "application/json")

			ctx, cancel := context.WithCancel(req.Context())
			req = req.WithContext(ctx)

			go func() {
				time.Sleep(10 * time.Millisecond)
				cancel()
			}()

			handler.HandleAlert(recorder, req)

			// The response should still be successful as the handler doesn't propagate context errors
			Expect(recorder.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("Response Format", func() {
		It("should return proper success response", func() {
			validPayload, _ := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "high-memory-alert.json"))
			req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validPayload))
			req.Header.Set("Content-Type", "application/json")

			handler.HandleAlert(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))
			Expect(recorder.Header().Get("Content-Type")).To(Equal("application/json"))

			var response webhook.WebhookResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Status).To(Equal("success"))
			Expect(response.Message).To(ContainSubstring("Successfully processed 1 alerts"))
			Expect(response.Error).To(BeEmpty())
		})

		It("should return proper error response", func() {
			req := httptest.NewRequest(http.MethodGet, "/alerts", nil)
			handler.HandleAlert(recorder, req)

			Expect(recorder.Header().Get("Content-Type")).To(Equal("application/json"))

			var response webhook.WebhookResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Status).To(Equal("error"))
			Expect(response.Error).NotTo(BeEmpty())
			Expect(response.Message).To(BeEmpty())
		})
	})

})
