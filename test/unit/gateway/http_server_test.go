package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gateway "github.com/jordigilh/kubernaut/test/integration/gateway"
)

var _ = Describe("HTTP Server Unit Tests", func() {

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-041: RFC 7807 Error Responses
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("BR-041: RFC 7807 Error Responses", func() {
		It("should return RFC 7807 format for 400 Bad Request", func() {
			// BUSINESS OUTCOME: Clients can programmatically handle Gateway errors
			// BUSINESS SCENARIO: Prometheus sends malformed webhook, needs structured error response
			// BR-041: RFC 7807 error format

			// Setup test infrastructure
			ctx := context.Background()
			redisClient := gateway.SetupRedisTestClient(ctx)
			k8sClient := gateway.SetupK8sTestClient(ctx)
			defer redisClient.ResetRedisConfig(ctx)

			// Start Gateway server
			gatewayServer, err := gateway.StartTestGateway(ctx, redisClient, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			testServer := httptest.NewServer(gatewayServer.Handler())
			defer testServer.Close()

			// Send invalid JSON payload
			invalidPayload := []byte(`{"invalid": "json without required fields"}`)
			req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/v1/signals/prometheus", bytes.NewReader(invalidPayload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BUSINESS OUTCOME VERIFICATION: RFC 7807 format
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Invalid payload should return 400")

			// Verify Content-Type is application/problem+json
			contentType := resp.Header.Get("Content-Type")
			Expect(contentType).To(Equal("application/problem+json"), "RFC 7807 responses should use application/problem+json")

			// Parse RFC 7807 response
			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")

			// Verify RFC 7807 required fields
			Expect(errorResponse["type"]).To(ContainSubstring("kubernaut.io/errors"), "type field should be a URI")
			Expect(errorResponse["title"]).ToNot(BeEmpty(), "title field is required")
			Expect(errorResponse["detail"]).ToNot(BeEmpty(), "detail field is required")
			Expect(errorResponse["status"]).To(Equal(float64(400)), "status field should match HTTP status")
			Expect(errorResponse["instance"]).To(Equal("/api/v1/signals/prometheus"), "instance should be the request path")

			// Verify request_id extension member (BR-109)
			Expect(errorResponse["request_id"]).ToNot(BeEmpty(), "request_id extension member should be present")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Error responses follow RFC 7807 standard
			// ✅ Clients can parse structured error details
			// ✅ Error type URLs provide documentation links
		})

		It("should return RFC 7807 format for 415 Unsupported Media Type", func() {
			// BUSINESS OUTCOME: Clients receive standards-compliant error responses
			// BUSINESS SCENARIO: Client sends non-JSON Content-Type, receives RFC 7807 error
			// BR-041: RFC 7807 error format
			// BR-042: Content-Type validation

			// Setup test infrastructure
			ctx := context.Background()
			redisClient := gateway.SetupRedisTestClient(ctx)
			k8sClient := gateway.SetupK8sTestClient(ctx)
			defer redisClient.ResetRedisConfig(ctx)

			// Start Gateway server
			gatewayServer, err := gateway.StartTestGateway(ctx, redisClient, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			testServer := httptest.NewServer(gatewayServer.Handler())
			defer testServer.Close()

			// Send request with non-JSON Content-Type
			req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/v1/signals/prometheus", bytes.NewReader([]byte("<xml>test</xml>")))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "text/xml")

			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BUSINESS OUTCOME VERIFICATION: RFC 7807 format
			Expect(resp.StatusCode).To(Equal(http.StatusUnsupportedMediaType), "Non-JSON content types should return 415")

			// Verify Content-Type is application/problem+json
			contentType := resp.Header.Get("Content-Type")
			Expect(contentType).To(Equal("application/problem+json"), "RFC 7807 responses should use application/problem+json")

			// Parse RFC 7807 response
			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")

			// Verify RFC 7807 required fields
			Expect(errorResponse["type"]).To(ContainSubstring("kubernaut.io/errors"), "type field should be a URI")
			Expect(errorResponse["title"]).ToNot(BeEmpty(), "title field is required")
			Expect(errorResponse["detail"]).ToNot(BeEmpty(), "detail field is required")
			Expect(errorResponse["status"]).To(Equal(float64(415)), "status field should match HTTP status")
			Expect(errorResponse["instance"]).To(Equal("/api/v1/signals/prometheus"), "instance should be the request path")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Error responses follow RFC 7807 standard
			// ✅ Clients can parse structured error details
			// ✅ Error type URLs provide documentation links
		})

		It("should sanitize sensitive data in error responses", func() {
			// BUSINESS OUTCOME: Error responses don't leak sensitive information
			// BUSINESS SCENARIO: Error occurs with sensitive data, response is sanitized
			// BR-GATEWAY-078: Error message sanitization

			// Setup test infrastructure
			ctx := context.Background()
			redisClient := gateway.SetupRedisTestClient(ctx)
			k8sClient := gateway.SetupK8sTestClient(ctx)
			defer redisClient.ResetRedisConfig(ctx)

			// Start Gateway server
			gatewayServer, err := gateway.StartTestGateway(ctx, redisClient, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			testServer := httptest.NewServer(gatewayServer.Handler())
			defer testServer.Close()

			// Send malformed payload that contains sensitive data
			// This will trigger a validation error because required fields are missing
			// The error message might include the sensitive data from the payload
			sensitivePayload := []byte(`{
			"alerts": [{
				"status": "firing",
				"labels": {
					"api_key": "secret-api-key-12345",
					"password": "super-secret-password"
				}
			}]
		}`)

			resp, err := http.Post(testServer.URL+"/api/v1/signals/prometheus", "application/json", bytes.NewReader(sensitivePayload))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

		// BUSINESS OUTCOME VERIFICATION: Error response sanitizes sensitive data
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Invalid payload should return 400")

		var errorResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&errorResponse)
		Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")

		// Verify RFC 7807 detail field exists
		errorDetail, exists := errorResponse["detail"]
		Expect(exists).To(BeTrue(), "RFC 7807 error response should include detail field")

		errorDetailStr, ok := errorDetail.(string)
		Expect(ok).To(BeTrue(), "Error detail should be a string")

		// CRITICAL SECURITY VERIFICATION: Sensitive data MUST be redacted
		Expect(errorDetailStr).ToNot(ContainSubstring("secret-api-key-12345"), "API key MUST be redacted from error response")
		Expect(errorDetailStr).ToNot(ContainSubstring("super-secret-password"), "Password MUST be redacted from error response")

			// Verify redaction markers are present (if sensitive data was in error message)
			// Note: The error might not include the sensitive fields at all, which is also acceptable

			// BUSINESS CAPABILITY TO VERIFY:
			// ✅ Sensitive data not exposed in error responses
			// ✅ Security compliance maintained
			// ✅ API keys and passwords are redacted
		})

		It("should include request ID in error responses for tracing", func() {
			// BUSINESS OUTCOME: Operators can trace errors across Gateway components
			// BUSINESS SCENARIO: Error occurs, operator uses request ID to find logs
			// BR-109: Request ID propagation to error responses

			// Setup test infrastructure
			ctx := context.Background()
			redisClient := gateway.SetupRedisTestClient(ctx)
			k8sClient := gateway.SetupK8sTestClient(ctx)
			defer redisClient.ResetRedisConfig(ctx)

			// Start Gateway server
			gatewayServer, err := gateway.StartTestGateway(ctx, redisClient, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			testServer := httptest.NewServer(gatewayServer.Handler())
			defer testServer.Close()

			// Send invalid request to trigger error
			invalidPayload := []byte(`{"invalid": "json without required fields"}`)
			resp, err := http.Post(testServer.URL+"/api/v1/signals/prometheus", "application/json", bytes.NewReader(invalidPayload))
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BUSINESS OUTCOME VERIFICATION: Error response includes request_id
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Invalid payload should return 400")

			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")

			// Verify request_id field exists
			requestID, exists := errorResponse["request_id"]
			Expect(exists).To(BeTrue(), "Error response MUST include request_id field for tracing")
			Expect(requestID).ToNot(BeEmpty(), "request_id MUST not be empty")

			// Verify request_id format (should be UUID or similar)
			requestIDStr, ok := requestID.(string)
			Expect(ok).To(BeTrue(), "request_id should be a string")
			Expect(len(requestIDStr)).To(BeNumerically(">", 10), "request_id should be meaningful identifier")

			// BUSINESS CAPABILITY TO VERIFY:
			// ✅ Request ID enables distributed tracing
			// ✅ Operators can correlate HTTP errors with logs
			// ✅ Debugging is faster with request context
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-042: Content-Type Validation
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("BR-042: Content-Type Validation", func() {
		It("should reject non-JSON Content-Type with 415 Unsupported Media Type", func() {
			// BUSINESS OUTCOME: Gateway rejects invalid webhook payloads early
			// BUSINESS SCENARIO: Misconfigured client sends XML instead of JSON
			// BR-042: Content-Type validation

			// Setup test infrastructure
			ctx := context.Background()
			redisClient := gateway.SetupRedisTestClient(ctx)
			k8sClient := gateway.SetupK8sTestClient(ctx)
			defer redisClient.ResetRedisConfig(ctx)

			// Start Gateway server
			gatewayServer, err := gateway.StartTestGateway(ctx, redisClient, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			testServer := httptest.NewServer(gatewayServer.Handler())
			defer testServer.Close()

			// Send request with non-JSON Content-Type
			req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/v1/signals/prometheus", bytes.NewReader([]byte("<xml>test</xml>")))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "text/xml")

			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

		// BUSINESS OUTCOME VERIFICATION: Non-JSON content types rejected
		Expect(resp.StatusCode).To(Equal(http.StatusUnsupportedMediaType), "Non-JSON content types should be rejected with 415")

		// Verify Accept header guides clients
		acceptHeader := resp.Header.Get("Accept")
		Expect(acceptHeader).To(ContainSubstring("application/json"), "Accept header should guide clients to correct format")

		// Verify error response is RFC 7807 format
		var errorResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&errorResponse)
		Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")
		Expect(errorResponse["detail"]).To(ContainSubstring("application/json"), "RFC 7807 detail field should explain supported types")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Invalid payloads rejected before processing
			// ✅ Clear error message for misconfigured clients
			// ✅ Accept header guides clients to correct format
		})

		It("should accept application/json Content-Type", func() {
			// BUSINESS OUTCOME: Standard JSON webhooks are processed
			// BUSINESS SCENARIO: Prometheus sends webhook with application/json
			// BR-042: Content-Type validation

			// Setup test infrastructure
			ctx := context.Background()
			redisClient := gateway.SetupRedisTestClient(ctx)
			k8sClient := gateway.SetupK8sTestClient(ctx)
			defer redisClient.ResetRedisConfig(ctx)

			// Create test namespace
			gateway.EnsureTestNamespace(ctx, k8sClient, "production")

			// Start Gateway server
			gatewayServer, err := gateway.StartTestGateway(ctx, redisClient, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			testServer := httptest.NewServer(gatewayServer.Handler())
			defer testServer.Close()

			// Send valid Prometheus alert with application/json Content-Type
			validAlert := []byte(`{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "HighMemoryUsage",
						"severity": "critical",
						"namespace": "production",
						"pod": "payment-api-1"
					},
					"annotations": {
						"summary": "Pod memory usage at 95%"
					}
				}]
			}`)

			req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/v1/signals/prometheus", bytes.NewReader(validAlert))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BUSINESS OUTCOME VERIFICATION: JSON webhooks processed successfully
			Expect(resp.StatusCode).To(BeElementOf([]int{http.StatusCreated, http.StatusAccepted}), "Valid JSON webhooks should be processed")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Standard JSON webhooks work correctly
			// ✅ Prometheus AlertManager integration works
		})

		It("should accept application/json with charset parameter", func() {
			// BUSINESS OUTCOME: Webhooks with charset parameter are processed
			// BUSINESS SCENARIO: Client sends Content-Type: application/json; charset=utf-8
			// BR-042: Content-Type validation

			// Setup test infrastructure
			ctx := context.Background()
			redisClient := gateway.SetupRedisTestClient(ctx)
			k8sClient := gateway.SetupK8sTestClient(ctx)
			defer redisClient.ResetRedisConfig(ctx)

			// Create test namespace
			gateway.EnsureTestNamespace(ctx, k8sClient, "production")

			// Start Gateway server
			gatewayServer, err := gateway.StartTestGateway(ctx, redisClient, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			testServer := httptest.NewServer(gatewayServer.Handler())
			defer testServer.Close()

			// Send valid Prometheus alert with charset parameter
			validAlert := []byte(`{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "HighMemoryUsage",
						"severity": "critical",
						"namespace": "production",
						"pod": "payment-api-1"
					},
					"annotations": {
						"summary": "Pod memory usage at 95%"
					}
				}]
			}`)

			req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/v1/signals/prometheus", bytes.NewReader(validAlert))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json; charset=utf-8")

			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BUSINESS OUTCOME VERIFICATION: Charset parameter doesn't break processing
			Expect(resp.StatusCode).To(BeElementOf([]int{http.StatusCreated, http.StatusAccepted}), "Charset parameter should not break processing")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Charset parameter doesn't break processing
			// ✅ UTF-8 encoded webhooks work correctly
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-043: HTTP Method Validation
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("BR-043: HTTP Method Validation", func() {
		It("should reject GET requests to webhook endpoints with 405", func() {
			// BUSINESS OUTCOME: Gateway enforces correct webhook usage patterns
			// BUSINESS SCENARIO: Misconfigured client sends GET instead of POST
			// BR-043: HTTP Method Validation

			// Setup test infrastructure
			ctx := context.Background()
			redisClient := gateway.SetupRedisTestClient(ctx)
			k8sClient := gateway.SetupK8sTestClient(ctx)
			defer redisClient.ResetRedisConfig(ctx)

			// Start Gateway server
			gatewayServer, err := gateway.StartTestGateway(ctx, redisClient, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			testServer := httptest.NewServer(gatewayServer.Handler())
			defer testServer.Close()

			// Send GET request to webhook endpoint
			resp, err := http.Get(testServer.URL + "/api/v1/signals/prometheus")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

		// BUSINESS OUTCOME VERIFICATION: GET requests rejected with 405
		Expect(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed), "GET requests should be rejected with 405")

		// Verify error response is RFC 7807 format
		var errorResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&errorResponse)
		Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")

		// Verify RFC 7807 detail field
		errorDetail, exists := errorResponse["detail"]
		Expect(exists).To(BeTrue(), "RFC 7807 error response should include detail field")
		Expect(errorDetail).To(ContainSubstring("Method not allowed"), "RFC 7807 detail should indicate method not allowed")

			// BUSINESS CAPABILITY TO VERIFY:
			// ✅ Incorrect methods rejected early
			// ✅ Clear error message for misconfigured clients
			// ✅ Webhook endpoints are POST-only
		})

		It("should reject PUT requests to webhook endpoints with 405", func() {
			// BUSINESS OUTCOME: Gateway prevents accidental data modification attempts
			// BUSINESS SCENARIO: Client mistakenly uses PUT instead of POST
			// BR-043: HTTP Method Validation

			// Setup test infrastructure
			ctx := context.Background()
			redisClient := gateway.SetupRedisTestClient(ctx)
			k8sClient := gateway.SetupK8sTestClient(ctx)
			defer redisClient.ResetRedisConfig(ctx)

			// Start Gateway server
			gatewayServer, err := gateway.StartTestGateway(ctx, redisClient, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			testServer := httptest.NewServer(gatewayServer.Handler())
			defer testServer.Close()

			// Send PUT request to webhook endpoint
			req, err := http.NewRequest(http.MethodPut, testServer.URL+"/api/v1/signals/prometheus", nil)
			Expect(err).ToNot(HaveOccurred())

			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

		// BUSINESS OUTCOME VERIFICATION: PUT requests rejected with 405
		Expect(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed), "PUT requests should be rejected with 405")

		// Verify error response is RFC 7807 format
		var errorResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&errorResponse)
		Expect(err).ToNot(HaveOccurred(), "Error response should be valid JSON")

		// Verify RFC 7807 detail field
		errorDetail, exists := errorResponse["detail"]
		Expect(exists).To(BeTrue(), "RFC 7807 error response should include detail field")
		Expect(errorDetail).To(ContainSubstring("Method not allowed"), "RFC 7807 detail should indicate method not allowed")

			// BUSINESS CAPABILITY TO VERIFY:
		// ✅ PUT requests rejected
		// ✅ Clear error message for misconfigured clients
	})

	// Note: Health endpoint tests are in integration suite (test/integration/gateway/health_integration_test.go)
	// Health endpoints are inherently integration-level concerns that require real dependencies (Redis, HTTP server)
	// Integration tests provide superior coverage with 4 tests covering /health and /ready endpoints
	})
})
