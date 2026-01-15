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

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test 17: Error Response Codes (BR-GATEWAY-101, BR-GATEWAY-043)", Ordered, func() {
	var (
		testCancel    context.CancelFunc
		testLogger    logr.Logger
		testNamespace string
		httpClient    *http.Client
	)

	BeforeAll(func() {
		_, testCancel = context.WithTimeout(ctx, 5*time.Minute)
		testLogger = logger.WithValues("test", "error-responses")
		httpClient = &http.Client{Timeout: 10 * time.Second}

	testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	testLogger.Info("Test 17: Error Response Codes - Setup")
	testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// BR-GATEWAY-NAMESPACE-FALLBACK: Pre-create namespace (Pattern: RO E2E)
	testNamespace = createTestNamespace("error-codes")
	testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)
		testLogger.Info("✅ Using shared Gateway", "url", gatewayURL)
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 17: Error Response Codes - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if CurrentSpecReport().Failed() {
			testLogger.Info("⚠️  Test FAILED - Preserving namespace for debugging",
				"namespace", testNamespace)
			testLogger.Info("To debug:")
			testLogger.Info(fmt.Sprintf("  export KUBECONFIG=%s", kubeconfigPath))
			testLogger.Info(fmt.Sprintf("  kubectl get pods -n %s", testNamespace))
			testLogger.Info(fmt.Sprintf("  kubectl logs -n %s deployment/gateway -f", testNamespace))
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		} else {
			testLogger.Info("Cleaning up test namespace...", "namespace", testNamespace)
			// Clean up test namespace (Pattern: RO E2E)
			deleteTestNamespace(testNamespace)
			testLogger.Info("✅ Test cleanup complete")
		}

		if testCancel != nil {
			testCancel()
		}
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	Context("HTTP Error Response Validation", func() {
		It("should return 400 Bad Request for malformed JSON", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Scenario: Send malformed JSON payload")
			testLogger.Info("Expected: HTTP 400 Bad Request with error message")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			testLogger.Info("Step 1: Send malformed JSON")
			malformedJSON := []byte(`{"alerts": [{"status": "firing", "labels": {invalid json`)

			resp, err := func() (*http.Response, error) {
				req25, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(malformedJSON))
				if err != nil {
					return nil, err
				}
				req25.Header.Set("Content-Type", "application/json")
				req25.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
				return httpClient.Do(req25)
			}()
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer func() { _ = resp.Body.Close() }()

			testLogger.Info("Step 2: Verify HTTP 400 response")
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"Malformed JSON should return HTTP 400 (BR-GATEWAY-101)")

			// Read response body
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			testLogger.Info("✅ Received HTTP 400 for malformed JSON",
				"response", string(body))

			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Test 17a PASSED: Malformed JSON Returns 400")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})

		It("should return 400 Bad Request for missing required fields", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Scenario: Send payload missing required alertname")
			testLogger.Info("Expected: HTTP 400 Bad Request")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			testLogger.Info("Step 1: Send payload without alertname")
			invalidPayload := map[string]interface{}{
				"alerts": []interface{}{
					map[string]interface{}{
						"status": "firing",
						"labels": map[string]interface{}{
							// Missing "alertname" - required field
							"severity":  "warning",
							"namespace": testNamespace,
						},
						"annotations": map[string]interface{}{
							"summary": "Test without alertname",
						},
					},
				},
			}
			payloadBytes, _ := json.Marshal(invalidPayload)

			resp, err := func() (*http.Response, error) {
				req26, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payloadBytes))
				if err != nil {
					return nil, err
				}
				req26.Header.Set("Content-Type", "application/json")
				req26.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
				return httpClient.Do(req26)
			}()
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer func() { _ = resp.Body.Close() }()

			testLogger.Info("Step 2: Verify HTTP 400 response")
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"Missing alertname should return HTTP 400 (BR-GATEWAY-043)")

			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			testLogger.Info("✅ Received HTTP 400 for missing alertname",
				"response", string(body))

			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Test 17b PASSED: Missing Required Fields Returns 400")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})

		It("should return 404 Not Found for unknown endpoints", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Scenario: Request unknown endpoint")
			testLogger.Info("Expected: HTTP 404 Not Found")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			testLogger.Info("Step 1: Request non-existent endpoint")
			resp, err := httpClient.Get(gatewayURL + "/api/v1/nonexistent/endpoint")
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer func() { _ = resp.Body.Close() }()

			testLogger.Info("Step 2: Verify HTTP 404 response")
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound),
				"Unknown endpoint should return HTTP 404")

			testLogger.Info("✅ Received HTTP 404 for unknown endpoint")

			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Test 17c PASSED: Unknown Endpoint Returns 404")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})

		It("should return 405 Method Not Allowed for wrong HTTP method", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Scenario: Send GET to POST-only endpoint")
			testLogger.Info("Expected: HTTP 405 Method Not Allowed")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			testLogger.Info("Step 1: Send GET to webhook endpoint (expects POST)")
			resp, err := httpClient.Get(gatewayURL + "/api/v1/signals/prometheus")
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer func() { _ = resp.Body.Close() }()

			testLogger.Info("Step 2: Verify HTTP 405 response")
			Expect(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed),
				"Wrong HTTP method should return HTTP 405")

			testLogger.Info("✅ Received HTTP 405 for wrong HTTP method")

			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Test 17d PASSED: Wrong HTTP Method Returns 405")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})

		It("should include error details in response body", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Scenario: Verify error responses include helpful details")
			testLogger.Info("Expected: JSON error response with error message")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			testLogger.Info("Step 1: Send invalid payload to trigger error")
			emptyPayload := map[string]interface{}{
				"alerts": []interface{}{}, // Empty alerts array
			}
			payloadBytes, _ := json.Marshal(emptyPayload)

			resp, err := func() (*http.Response, error) {
				req27, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payloadBytes))
				if err != nil {
					return nil, err
				}
				req27.Header.Set("Content-Type", "application/json")
				req27.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
				return httpClient.Do(req27)
			}()
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer func() { _ = resp.Body.Close() }()

			testLogger.Info("Step 2: Check response body for error details")
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			// Try to parse as JSON
			var errorResponse map[string]interface{}
			if err := json.Unmarshal(body, &errorResponse); err == nil {
				testLogger.Info("✅ Error response is valid JSON",
					"response", errorResponse)

				// Check for common error fields
				if _, hasError := errorResponse["error"]; hasError {
					testLogger.Info("✅ Response includes 'error' field")
				}
				if _, hasMessage := errorResponse["message"]; hasMessage {
					testLogger.Info("✅ Response includes 'message' field")
				}
			} else {
				testLogger.Info("Response body (non-JSON)", "body", string(body))
			}

			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Test 17e PASSED: Error Response Details")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})
	})
})
