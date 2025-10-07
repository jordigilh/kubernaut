package gateway_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

var _ = Describe("BR-WH-004: Authentication and Authorization", func() {
	var (
		gatewayService  gateway.Service
		mockAlertClient *mocks.MockAlertProcessorClient
		testServer      *httptest.Server
		logger          *logrus.Logger
		validPayload    map[string]interface{}
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)
		mockAlertClient = mocks.NewMockAlertProcessorClient()

		validPayload = map[string]interface{}{
			"alerts": []map[string]interface{}{
				{
					"status": "firing",
					"labels": map[string]interface{}{
						"alertname": "TestAlert",
					},
				},
			},
		}
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
	})

	Context("Bearer Token Authentication", func() {
		It("should accept requests with valid bearer token", func() {
			// TDD RED: This will fail because authentication isn't implemented yet
			config := &gateway.Config{
				Port: 8080,
				Authentication: gateway.AuthConfig{
					Type:  "bearer",
					Token: "valid-secret-token",
				},
				AlertProcessor: gateway.AlertProcessorConfig{
					Endpoint: "http://alert-processor:8081",
					Timeout:  5 * time.Second,
				},
			}

			gatewayService = gateway.NewService(config, mockAlertClient, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			mockAlertClient.ExpectForwardAlert().Return(nil)

			payloadBytes, err := json.Marshal(validPayload)
			Expect(err).ToNot(HaveOccurred())

			req, err := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(payloadBytes))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer valid-secret-token")

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("should reject requests with invalid bearer token", func() {
			// TDD RED: This will fail because authentication isn't implemented yet
			config := &gateway.Config{
				Port: 8080,
				Authentication: gateway.AuthConfig{
					Type:  "bearer",
					Token: "valid-secret-token",
				},
				AlertProcessor: gateway.AlertProcessorConfig{
					Endpoint: "http://alert-processor:8081",
					Timeout:  5 * time.Second,
				},
			}

			gatewayService = gateway.NewService(config, mockAlertClient, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			payloadBytes, err := json.Marshal(validPayload)
			Expect(err).ToNot(HaveOccurred())

			req, err := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(payloadBytes))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer invalid-token")

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("should reject requests without authorization header", func() {
			// TDD RED: This will fail because authentication isn't implemented yet
			config := &gateway.Config{
				Port: 8080,
				Authentication: gateway.AuthConfig{
					Type:  "bearer",
					Token: "valid-secret-token",
				},
				AlertProcessor: gateway.AlertProcessorConfig{
					Endpoint: "http://alert-processor:8081",
					Timeout:  5 * time.Second,
				},
			}

			gatewayService = gateway.NewService(config, mockAlertClient, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			payloadBytes, err := json.Marshal(validPayload)
			Expect(err).ToNot(HaveOccurred())

			req, err := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(payloadBytes))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			// No Authorization header

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})
	})
})

var _ = Describe("BR-WH-023: Rate Limiting", func() {
	var (
		gatewayService  gateway.Service
		mockAlertClient *mocks.MockAlertProcessorClient
		testServer      *httptest.Server
		logger          *logrus.Logger
		validPayload    map[string]interface{}
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)
		mockAlertClient = mocks.NewMockAlertProcessorClient()

		validPayload = map[string]interface{}{
			"alerts": []map[string]interface{}{
				{
					"status": "firing",
					"labels": map[string]interface{}{
						"alertname": "TestAlert",
					},
				},
			},
		}
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
	})

	Context("Request Rate Limiting", func() {
		It("should allow requests within rate limit", func() {
			// TDD RED: This will fail because rate limiting isn't implemented yet
			config := &gateway.Config{
				Port: 8080,
				RateLimit: gateway.RateLimitConfig{
					RequestsPerMinute: 10,
					BurstSize:         5,
				},
				AlertProcessor: gateway.AlertProcessorConfig{
					Endpoint: "http://alert-processor:8081",
					Timeout:  5 * time.Second,
				},
			}

			gatewayService = gateway.NewService(config, mockAlertClient, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			mockAlertClient.ExpectForwardAlert().Return(nil)

			payloadBytes, err := json.Marshal(validPayload)
			Expect(err).ToNot(HaveOccurred())

			// Make 3 requests (within limit)
			for i := 0; i < 3; i++ {
				req, err := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(payloadBytes))
				Expect(err).ToNot(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")

				client := &http.Client{Timeout: 5 * time.Second}
				resp, err := client.Do(req)
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			}
		})

		It("should reject requests exceeding rate limit", func() {
			// TDD RED: This will fail because rate limiting isn't implemented yet
			config := &gateway.Config{
				Port: 8080,
				RateLimit: gateway.RateLimitConfig{
					RequestsPerMinute: 2, // Very low limit for testing
					BurstSize:         1,
				},
				AlertProcessor: gateway.AlertProcessorConfig{
					Endpoint: "http://alert-processor:8081",
					Timeout:  5 * time.Second,
				},
			}

			gatewayService = gateway.NewService(config, mockAlertClient, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			mockAlertClient.ExpectForwardAlert().Return(nil)

			payloadBytes, err := json.Marshal(validPayload)
			Expect(err).ToNot(HaveOccurred())

			var wg sync.WaitGroup
			var responses []int

			// Make multiple concurrent requests to exceed rate limit
			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()

					req, err := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(payloadBytes))
					Expect(err).ToNot(HaveOccurred())
					req.Header.Set("Content-Type", "application/json")

					client := &http.Client{Timeout: 5 * time.Second}
					resp, err := client.Do(req)
					Expect(err).ToNot(HaveOccurred())
					defer resp.Body.Close()

					responses = append(responses, resp.StatusCode)
				}()
			}

			wg.Wait()

			// At least one request should be rate limited
			rateLimitedCount := 0
			for _, statusCode := range responses {
				if statusCode == http.StatusTooManyRequests {
					rateLimitedCount++
				}
			}

			Expect(rateLimitedCount).To(BeNumerically(">", 0))
		})
	})
})

var _ = Describe("BR-WH-025: Access Logging", func() {
	var (
		gatewayService  gateway.Service
		mockAlertClient *mocks.MockAlertProcessorClient
		testServer      *httptest.Server
		logger          *logrus.Logger
		validPayload    map[string]interface{}
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)
		mockAlertClient = mocks.NewMockAlertProcessorClient()

		validPayload = map[string]interface{}{
			"alerts": []map[string]interface{}{
				{
					"status": "firing",
					"labels": map[string]interface{}{
						"alertname": "TestAlert",
					},
				},
			},
		}
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
	})

	Context("Security Access Logging", func() {
		It("should log successful webhook requests", func() {
			// TDD RED: This will fail because enhanced logging isn't implemented yet
			config := &gateway.Config{
				Port: 8080,
				Logging: gateway.LoggingConfig{
					AccessLog: true,
					Level:     "info",
				},
				AlertProcessor: gateway.AlertProcessorConfig{
					Endpoint: "http://alert-processor:8081",
					Timeout:  5 * time.Second,
				},
			}

			gatewayService = gateway.NewService(config, mockAlertClient, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

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

			// Verify access logging is enabled (implementation will be in GREEN phase)
			accessLogs := gatewayService.GetAccessLogs()
			Expect(len(accessLogs)).To(BeNumerically(">", 0))
		})

		It("should log failed authentication attempts", func() {
			// TDD RED: This will fail because security logging isn't implemented yet
			config := &gateway.Config{
				Port: 8080,
				Authentication: gateway.AuthConfig{
					Type:  "bearer",
					Token: "valid-secret-token",
				},
				Logging: gateway.LoggingConfig{
					AccessLog:   true,
					SecurityLog: true,
					Level:       "info",
				},
				AlertProcessor: gateway.AlertProcessorConfig{
					Endpoint: "http://alert-processor:8081",
					Timeout:  5 * time.Second,
				},
			}

			gatewayService = gateway.NewService(config, mockAlertClient, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			payloadBytes, err := json.Marshal(validPayload)
			Expect(err).ToNot(HaveOccurred())

			req, err := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(payloadBytes))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer invalid-token")

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

			// Verify security logging captured the failed attempt
			securityLogs := gatewayService.GetSecurityLogs()
			Expect(len(securityLogs)).To(BeNumerically(">", 0))

			// Check that the log contains authentication failure
			lastLog := securityLogs[len(securityLogs)-1]
			Expect(lastLog).To(ContainSubstring("authentication_failed"))
		})
	})
})








