package gateway_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

var _ = Describe("BR-WH-001: HTTP Webhook Reception from Prometheus", func() {
	var (
		gatewayService         gateway.Service
		mockAlertClient        *mocks.MockAlertProcessorClient
		testServer             *httptest.Server
		logger                 *logrus.Logger
		validPrometheusPayload map[string]interface{}
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)

		// Create mock Alert Processor client
		mockAlertClient = mocks.NewMockAlertProcessorClient()

		// Create valid Prometheus AlertManager payload
		validPrometheusPayload = map[string]interface{}{
			"version":  "4",
			"groupKey": "test-group",
			"status":   "firing",
			"receiver": "kubernaut-webhook",
			"alerts": []map[string]interface{}{
				{
					"status": "firing",
					"labels": map[string]interface{}{
						"alertname": "PodCrashLooping",
						"namespace": "production",
						"pod":       "app-deployment-xyz",
						"severity":  "critical",
					},
					"annotations": map[string]interface{}{
						"description": "Pod has been crash looping for 5 minutes",
						"summary":     "Pod crash loop detected",
					},
					"startsAt":     "2025-01-29T10:00:00Z",
					"generatorURL": "http://prometheus:9090/graph",
					"fingerprint":  "abc123def456",
				},
			},
		}

		// Gateway service will be created in tests
		gatewayService = nil
		testServer = nil
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
	})

	Context("Core Webhook Reception Business Logic", func() {
		It("should receive and validate Prometheus webhook requests", func() {
			// TDD RED: This test will fail because gateway.Service doesn't exist yet
			config := &gateway.Config{
				Port: 8080,
				AlertProcessor: gateway.AlertProcessorConfig{
					Endpoint: "http://alert-processor:8081",
					Timeout:  5 * time.Second,
				},
			}

			gatewayService = gateway.NewService(config, mockAlertClient, logger)
			Expect(gatewayService).ToNot(BeNil())

			// Start test server
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			// Create webhook request
			payloadBytes, err := json.Marshal(validPrometheusPayload)
			Expect(err).ToNot(HaveOccurred())

			req, err := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(payloadBytes))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			// Make request
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Validate response - BR-WH-011: Appropriate HTTP response codes
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("should forward alerts to Alert Processor within 50ms", func() {
			// TDD RED: Testing BR-WH-026 integration requirement
			config := &gateway.Config{
				Port: 8080,
				AlertProcessor: gateway.AlertProcessorConfig{
					Endpoint: "http://alert-processor:8081",
					Timeout:  5 * time.Second,
				},
			}

			gatewayService = gateway.NewService(config, mockAlertClient, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			// Setup mock expectation
			mockAlertClient.ExpectForwardAlert().Return(nil)

			// Create webhook request
			payloadBytes, err := json.Marshal(validPrometheusPayload)
			Expect(err).ToNot(HaveOccurred())

			req, err := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(payloadBytes))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			// Measure processing time
			start := time.Now()
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			processingTime := time.Since(start)

			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Validate BR-WH-026: Forward within 50ms
			Expect(processingTime).To(BeNumerically("<", 50*time.Millisecond))
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Verify Alert Processor was called
			Expect(mockAlertClient.ForwardAlertCallCount()).To(Equal(1))
		})
	})
})

var _ = Describe("BR-WH-003: Webhook Payload Validation", func() {
	var (
		gatewayService  gateway.Service
		mockAlertClient *mocks.MockAlertProcessorClient
		testServer      *httptest.Server
		logger          *logrus.Logger
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)

		mockAlertClient = mocks.NewMockAlertProcessorClient()

		config := &gateway.Config{
			Port: 8080,
			AlertProcessor: gateway.AlertProcessorConfig{
				Endpoint: "http://alert-processor:8081",
				Timeout:  5 * time.Second,
			},
		}

		gatewayService = gateway.NewService(config, mockAlertClient, logger)
		handler := gatewayService.GetHTTPHandler()
		testServer = httptest.NewServer(handler)
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
	})

	Context("Payload Validation Business Logic", func() {
		It("should reject malformed JSON payloads", func() {
			// TDD RED: Test payload validation
			malformedJSON := `{"invalid": json}`

			req, err := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBufferString(malformedJSON))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BR-WH-011: Return appropriate error code
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})

		It("should reject non-JSON content types", func() {
			// TDD RED: Test content-type validation
			req, err := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBufferString("plain text"))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "text/plain")

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BR-WH-011: Return appropriate error code
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})

		It("should reject non-POST methods", func() {
			// TDD RED: Test HTTP method validation
			req, err := http.NewRequest("GET", testServer.URL+"/webhook/prometheus", nil)
			Expect(err).ToNot(HaveOccurred())

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BR-WH-011: Return method not allowed
			Expect(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed))
		})
	})
})

var _ = Describe("BR-WH-011: HTTP Response Management", func() {
	var (
		gatewayService  gateway.Service
		mockAlertClient *mocks.MockAlertProcessorClient
		testServer      *httptest.Server
		logger          *logrus.Logger
	)

	BeforeEach(func() {
		logger = logrus.New()
		mockAlertClient = mocks.NewMockAlertProcessorClient()

		config := &gateway.Config{
			Port: 8080,
			AlertProcessor: gateway.AlertProcessorConfig{
				Endpoint: "http://alert-processor:8081",
				Timeout:  5 * time.Second,
			},
		}

		gatewayService = gateway.NewService(config, mockAlertClient, logger)
		handler := gatewayService.GetHTTPHandler()
		testServer = httptest.NewServer(handler)
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
	})

	Context("Response Management Business Logic", func() {
		It("should return structured success responses", func() {
			// TDD RED: Test structured response format
			validPayload := map[string]interface{}{
				"alerts": []map[string]interface{}{
					{
						"status": "firing",
						"labels": map[string]interface{}{
							"alertname": "TestAlert",
						},
					},
				},
			}

			mockAlertClient.ExpectForwardAlert().Return(nil)

			payloadBytes, err := json.Marshal(validPayload)
			Expect(err).ToNot(HaveOccurred())

			req, err := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(payloadBytes))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Validate response structure
			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			Expect(response).To(HaveKey("status"))
			Expect(response["status"]).To(Equal("success"))
			Expect(response).To(HaveKey("message"))
		})

		It("should provide health check endpoint", func() {
			// TDD RED: Test health check functionality
			req, err := http.NewRequest("GET", testServer.URL+"/health", nil)
			Expect(err).ToNot(HaveOccurred())

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			Expect(response).To(HaveKey("status"))
			Expect(response["status"]).To(Equal("healthy"))
		})
	})
})
