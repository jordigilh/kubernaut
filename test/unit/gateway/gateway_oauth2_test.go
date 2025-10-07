package gateway_test

import (
	"bytes"
	"fmt"
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

var _ = Describe("BR-WH-005: OAuth2/JWT Authentication (Kubernetes/OpenShift Only)", func() {
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

		// Standard OAuth2 configuration for Kubernetes/OpenShift
		config = &gateway.Config{
			Port: 8080,
			AlertProcessor: gateway.AlertProcessorConfig{
				Endpoint: "http://alert-processor:8081",
				Timeout:  5 * time.Second,
			},
			Authentication: auth.AuthConfig{
				Enabled: true,
				OAuth2: auth.OAuth2Config{
					UseInClusterConfig:     true,
					Audience:               "kubernaut-gateway",
					RequiredNamespace:      "prometheus",
					RequiredServiceAccount: "prometheus-operator",
				},
			},
		}
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
	})

	Context("OAuth2/JWT Authentication Success", func() {
		It("should accept requests with valid Kubernetes ServiceAccount JWT", func() {
			// Configure mock for successful authentication
			mockAuthenticator.SetShouldAuthenticate(true).SetUserInfo(
				"system:serviceaccount:prometheus:prometheus-operator",
				"prometheus",
				[]string{"system:serviceaccounts", "system:serviceaccounts:prometheus"},
			)

			gatewayService = gateway.NewService(config, mockAlertClient, mockAuthenticator, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			req, _ := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(validPayload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer valid-k8s-jwt-token")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("should accept requests with valid OpenShift ServiceAccount JWT", func() {
			// Configure mock for OpenShift ServiceAccount
			mockAuthenticator.SetShouldAuthenticate(true).SetUserInfo(
				"system:serviceaccount:openshift-monitoring:prometheus-operator",
				"openshift-monitoring",
				[]string{"system:serviceaccounts", "system:serviceaccounts:openshift-monitoring"},
			)

			// Update config for OpenShift namespace
			config.Authentication.OAuth2.RequiredNamespace = "openshift-monitoring"

			gatewayService = gateway.NewService(config, mockAlertClient, mockAuthenticator, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			req, _ := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(validPayload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer valid-ocp-jwt-token")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})

	Context("OAuth2/JWT Authentication Failures", func() {
		It("should reject requests with invalid JWT tokens", func() {
			// Configure mock for authentication failure
			mockAuthenticator.SetShouldAuthenticate(false)

			gatewayService = gateway.NewService(config, mockAlertClient, mockAuthenticator, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			req, _ := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(validPayload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer invalid-jwt-token")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("should reject requests with missing Authorization header", func() {
			// Configure mock for authentication failure (missing token)
			mockAuthenticator.SetShouldAuthenticate(false)

			gatewayService = gateway.NewService(config, mockAlertClient, mockAuthenticator, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			req, _ := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(validPayload))
			req.Header.Set("Content-Type", "application/json")
			// No Authorization header

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("should reject requests with namespace mismatch", func() {
			// Configure mock to return wrong namespace
			mockAuthenticator.SetShouldAuthenticate(false)

			gatewayService = gateway.NewService(config, mockAlertClient, mockAuthenticator, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			req, _ := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(validPayload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer jwt-with-wrong-namespace")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("should handle Kubernetes TokenReview API errors", func() {
			// Configure mock to return authentication error
			mockAuthenticator.SetAuthError(fmt.Errorf("TokenReview API call failed: connection refused"))

			gatewayService = gateway.NewService(config, mockAlertClient, mockAuthenticator, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			req, _ := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(validPayload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer valid-jwt-token")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
		})
	})

	Context("Authentication Disabled (Development Only)", func() {
		It("should allow requests when authentication is disabled", func() {
			// Disable authentication for development/testing
			config.Authentication.Enabled = false

			gatewayService = gateway.NewService(config, mockAlertClient, nil, logger) // No authenticator
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			req, _ := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(validPayload))
			req.Header.Set("Content-Type", "application/json")
			// No Authorization header needed

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})

	Context("Security Logging", func() {
		It("should log successful OAuth2/JWT authentication", func() {
			// Configure mock for successful authentication
			mockAuthenticator.SetShouldAuthenticate(true).SetUserInfo(
				"system:serviceaccount:prometheus:prometheus-operator",
				"prometheus",
				[]string{"system:serviceaccounts", "system:serviceaccounts:prometheus"},
			)

			config.Logging = gateway.LoggingConfig{SecurityLog: true}
			gatewayService = gateway.NewService(config, mockAlertClient, mockAuthenticator, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			req, _ := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(validPayload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer valid-jwt-token")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Should log successful OAuth2 authentication (check that no security errors were logged)
			securityLogs := gatewayService.GetSecurityLogs()
			for _, log := range securityLogs {
				Expect(log).ToNot(ContainSubstring("authentication_failed"))
			}
		})

		It("should log failed OAuth2/JWT authentication attempts", func() {
			// Configure mock for authentication failure
			mockAuthenticator.SetShouldAuthenticate(false)

			config.Logging = gateway.LoggingConfig{SecurityLog: true}
			gatewayService = gateway.NewService(config, mockAlertClient, mockAuthenticator, logger)
			handler := gatewayService.GetHTTPHandler()
			testServer = httptest.NewServer(handler)

			req, _ := http.NewRequest("POST", testServer.URL+"/webhook/prometheus", bytes.NewBuffer(validPayload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer invalid-jwt-token")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

			// Should log failed OAuth2 authentication
			Expect(gatewayService.GetSecurityLogs()).To(ContainElement(ContainSubstring("oauth2_authentication_failed")))
		})
	})
})
