package gateway_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

var _ = Describe("BR-WH-009: Timeout Handling", func() {
	var (
		gatewayService    gateway.Service
		mockAlertClient   *mocks.MockAlertProcessorClient
		mockAuthenticator *mocks.MockAuthenticator
		testServer        *httptest.Server
		logger            *logrus.Logger
		validPayload      []byte
		config            *gateway.Config
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)
		mockAlertClient = mocks.NewMockAlertProcessorClient()
		mockAuthenticator = mocks.NewMockAuthenticator()
		validPayload = []byte(`{"alerts":[{"status":"firing","labels":{"alertname":"TestAlert"}}]}`)

		// Configuration with timeout settings
		config = &gateway.Config{
			Port: 8080,
			AlertProcessor: gateway.AlertProcessorConfig{
				Endpoint: "http://alert-processor:8081",
				Timeout:  2 * time.Second, // Short timeout for testing
			},
			Authentication: auth.AuthConfig{
				Enabled: false, // Disable auth for timeout tests
			},
		}
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
	})

	Context("Alert Processor Timeout Scenarios", func() {
		It("should timeout when Alert Processor takes too long to respond", func() {
			// Configure mock to simulate slow Alert Processor
			mockAlertClient.ExpectForwardAlert().Return(nil).WithDelay(5 * time.Second) // Longer than 2s timeout

			// Configure successful authentication
			mockAuthenticator.SetShouldAuthenticate(true)

			gatewayService = gateway.NewService(config, mockAlertClient, mockAuthenticator, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			req, _ := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(validPayload))
			req.Header.Set("Content-Type", "application/json")

			start := time.Now()
			resp, err := http.DefaultClient.Do(req)
			duration := time.Since(start)

			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Should return Gateway Timeout status
			Expect(resp.StatusCode).To(Equal(http.StatusGatewayTimeout))

			// Should timeout around configured timeout (2s), not the mock delay (5s)
			Expect(duration).To(BeNumerically("~", 2*time.Second, 500*time.Millisecond))
		})

		It("should handle timeout with zero/negative timeout configuration", func() {
			// Test with zero timeout - should use default
			config.AlertProcessor.Timeout = 0

			// Configure mock with moderate delay
			mockAlertClient.ExpectForwardAlert().Return(nil).WithDelay(1 * time.Second)

			mockAuthenticator.SetShouldAuthenticate(true)
			gatewayService = gateway.NewService(config, mockAlertClient, mockAuthenticator, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			req, _ := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(validPayload))
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Should still work with default timeout (should be > 1s)
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("should succeed when Alert Processor responds within timeout", func() {
			// Configure mock to respond quickly
			mockAlertClient.ExpectForwardAlert().Return(nil).WithDelay(500 * time.Millisecond) // Well under 2s timeout

			mockAuthenticator.SetShouldAuthenticate(true)
			gatewayService = gateway.NewService(config, mockAlertClient, mockAuthenticator, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			req, _ := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(validPayload))
			req.Header.Set("Content-Type", "application/json")

			start := time.Now()
			resp, err := http.DefaultClient.Do(req)
			duration := time.Since(start)

			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Should succeed
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Should complete quickly
			Expect(duration).To(BeNumerically("<", 1*time.Second))
		})

		It("should handle multiple alerts with timeout on some", func() {
			// Use payload with multiple alerts
			multiAlertPayload := []byte(`{
				"alerts": [
					{"status":"firing","labels":{"alertname":"FastAlert"}},
					{"status":"firing","labels":{"alertname":"SlowAlert"}},
					{"status":"firing","labels":{"alertname":"FastAlert2"}}
				]
			}`)

			// Configure mock to timeout on any alert processing
			mockAlertClient.ExpectForwardAlert().Return(nil).WithDelay(3 * time.Second) // Over 2s timeout

			mockAuthenticator.SetShouldAuthenticate(true)
			gatewayService = gateway.NewService(config, mockAlertClient, mockAuthenticator, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			req, _ := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(multiAlertPayload))
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Should timeout for the entire request
			Expect(resp.StatusCode).To(Equal(http.StatusGatewayTimeout))
		})
	})

	Context("Timeout Error Messages", func() {
		It("should return descriptive error message for timeout", func() {
			// Configure mock to timeout
			mockAlertClient.ExpectForwardAlert().Return(nil).WithDelay(3 * time.Second)

			mockAuthenticator.SetShouldAuthenticate(true)
			gatewayService = gateway.NewService(config, mockAlertClient, mockAuthenticator, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			req, _ := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(validPayload))
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Check response body contains timeout information
			body := make([]byte, 1024)
			n, _ := resp.Body.Read(body)
			responseBody := string(body[:n])

			Expect(responseBody).To(ContainSubstring("timeout"))
			Expect(responseBody).To(ContainSubstring("Alert Processor"))
		})
	})

	Context("Timeout Configuration", func() {
		It("should respect configured timeout values", func() {
			// Test with different timeout configuration
			config.AlertProcessor.Timeout = 100 * time.Millisecond // Very short timeout

			// Configure mock with delay longer than timeout
			mockAlertClient.ExpectForwardAlert().Return(nil).WithDelay(200 * time.Millisecond)

			mockAuthenticator.SetShouldAuthenticate(true)
			gatewayService = gateway.NewService(config, mockAlertClient, mockAuthenticator, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			req, _ := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(validPayload))
			req.Header.Set("Content-Type", "application/json")

			start := time.Now()
			resp, err := http.DefaultClient.Do(req)
			duration := time.Since(start)

			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Should timeout quickly
			Expect(resp.StatusCode).To(Equal(http.StatusGatewayTimeout))
			Expect(duration).To(BeNumerically("~", 100*time.Millisecond, 50*time.Millisecond))
		})
	})

	Context("Context Cancellation", func() {
		It("should handle context cancellation gracefully", func() {
			// Configure mock with moderate delay
			mockAlertClient.ExpectForwardAlert().Return(context.Canceled)

			mockAuthenticator.SetShouldAuthenticate(true)
			gatewayService = gateway.NewService(config, mockAlertClient, mockAuthenticator, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			req, _ := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(validPayload))
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Should handle cancellation as internal server error
			Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
		})
	})
})
