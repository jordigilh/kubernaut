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
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test 19: Replay Attack Prevention (BR-GATEWAY-074, BR-GATEWAY-075)", Ordered, func() {
	var (
		testLogger    logr.Logger
		testNamespace string
		httpClient    *http.Client
	)

	BeforeAll(func() {
		testLogger = logger.WithValues("test", "replay-attack-prevention")
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 19: Replay Attack Prevention - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Create unique test namespace (Pattern: RO E2E)
		// k8sClient available from suite (DD-E2E-K8S-CLIENT-001)
		testNamespace = createTestNamespace("replay")

		testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)
		testLogger.Info("✅ Using shared Gateway", "url", gatewayURL)
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 19: Replay Attack Prevention - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if CurrentSpecReport().Failed() {
			testLogger.Info("⚠️  Test FAILED - Preserving namespace for debugging",
				"namespace", testNamespace)
			testLogger.Info("To debug:")
			testLogger.Info(fmt.Sprintf("  export KUBECONFIG=%s", kubeconfigPath))
			testLogger.Info(fmt.Sprintf("  kubectl get pods -n %s", testNamespace))
		testLogger.Info(fmt.Sprintf("  kubectl logs -n kubernaut-system deployment/gateway -f"))
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		return
		}

	testLogger.Info("Cleaning up test namespace...", "namespace", testNamespace)
	// Namespace cleanup handled by suite-level AfterSuite (Kind cluster deletion)

	testLogger.Info("✅ Test cleanup complete")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	Context("Timestamp Validation (BR-GATEWAY-074, BR-GATEWAY-075)", func() {
		It("should reject alerts without timestamp header (mandatory validation)", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Scenario: Send alert without X-Timestamp header")
			testLogger.Info("Expected: HTTP 400 Bad Request (timestamp is mandatory)")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			testLogger.Info("Step 1: Create Prometheus webhook payload")
			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestAlertNoTimestamp",
				Namespace: testNamespace,
				Severity:  "warning",
				PodName:   "test-pod",
				Labels: map[string]string{
					"scenario": "no-timestamp",
				},
			})

			testLogger.Info("Step 2: Send request to Gateway WITHOUT X-Timestamp header")
			req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			// Deliberately NOT setting X-Timestamp header to test rejection
			resp, err := httpClient.Do(req)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should complete")
			defer func() { _ = resp.Body.Close() }()

			testLogger.Info("Step 3: Verify HTTP 400 Bad Request response")
			body, _ := io.ReadAll(resp.Body)
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"Request without timestamp must be rejected (BR-GATEWAY-074)",
				"response", string(body))
			Expect(string(body)).To(ContainSubstring("missing timestamp header"),
				"Error message should indicate missing timestamp")

			testLogger.Info("✅ Alert rejected without timestamp header")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Test 19a PASSED: Missing Timestamp Rejected")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})

		It("should accept alerts with valid timestamp within tolerance window", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Scenario: Send alert with valid current timestamp")
			testLogger.Info("Expected: HTTP 201 Created (timestamp within tolerance)")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			testLogger.Info("Step 1: Create Prometheus webhook payload with current timestamp")
			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestAlertValidTimestamp",
				Namespace: testNamespace,
				Severity:  "warning",
				PodName:   "test-pod",
				Labels: map[string]string{
					"scenario": "valid-timestamp",
				},
			})

			currentTimestamp := time.Now().Unix()
			testLogger.Info("Step 2: Send request with X-Timestamp header",
				"timestamp", currentTimestamp,
				"time", time.Unix(currentTimestamp, 0).Format(time.RFC3339))

			req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", strconv.FormatInt(currentTimestamp, 10))

			resp, err := httpClient.Do(req)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer func() { _ = resp.Body.Close() }()

			testLogger.Info("Step 3: Verify HTTP 201 Created response")
			body, _ := io.ReadAll(resp.Body)
			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"Request with valid timestamp should succeed (BR-GATEWAY-074)",
				"response", string(body))

			testLogger.Info("✅ Alert accepted with valid timestamp")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Test 19b PASSED: Valid Timestamp Accepted")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})

		It("should reject alerts with timestamp too old (replay attack)", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Scenario: Send alert with timestamp >5min old (replay attack)")
			testLogger.Info("Expected: HTTP 400 Bad Request with 'timestamp too old' message")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			testLogger.Info("Step 1: Create Prometheus webhook payload")
			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestAlertOldTimestamp",
				Namespace: testNamespace,
				Severity:  "critical",
				PodName:   "test-pod",
				Labels: map[string]string{
					"scenario": "replay-attack",
				},
			})

			// Create timestamp >5min old (tolerance is 5min, so use 10min to be safe)
			oldTimestamp := time.Now().Add(-10 * time.Minute).Unix()
			testLogger.Info("Step 2: Send request with OLD timestamp (10min ago)",
				"timestamp", oldTimestamp,
				"time", time.Unix(oldTimestamp, 0).Format(time.RFC3339),
				"age", "10 minutes old")

			req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", strconv.FormatInt(oldTimestamp, 10))

			resp, err := httpClient.Do(req)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer func() { _ = resp.Body.Close() }()

			testLogger.Info("Step 3: Verify HTTP 400 Bad Request response")
			body, _ := io.ReadAll(resp.Body)
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"Request with old timestamp should be rejected (BR-GATEWAY-075: Replay Attack Prevention)",
				"response", string(body))

			testLogger.Info("Step 4: Verify error message contains 'timestamp too old'")
			bodyStr := string(body)
			Expect(bodyStr).To(ContainSubstring("timestamp too old"),
				"Error message should indicate replay attack concern")

			testLogger.Info("✅ Replay attack prevented - old timestamp rejected")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Test 19c PASSED: Replay Attack Prevented (Old Timestamp)")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})

		It("should reject alerts with timestamp in future (clock skew attack)", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Scenario: Send alert with timestamp >2min in future (clock skew)")
			testLogger.Info("Expected: HTTP 400 Bad Request with 'timestamp in future' message")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			testLogger.Info("Step 1: Create Prometheus webhook payload")
			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestAlertFutureTimestamp",
				Namespace: testNamespace,
				Severity:  "warning",
				PodName:   "test-pod",
				Labels: map[string]string{
					"scenario": "clock-skew-attack",
				},
			})

			// Create timestamp >2min in future (tolerance is 2min, so use 5min to be safe)
			futureTimestamp := time.Now().Add(5 * time.Minute).Unix()
			testLogger.Info("Step 2: Send request with FUTURE timestamp (5min ahead)",
				"timestamp", futureTimestamp,
				"time", time.Unix(futureTimestamp, 0).Format(time.RFC3339),
				"skew", "5 minutes ahead")

			req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", strconv.FormatInt(futureTimestamp, 10))

			resp, err := httpClient.Do(req)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer func() { _ = resp.Body.Close() }()

			testLogger.Info("Step 3: Verify HTTP 400 Bad Request response")
			body, _ := io.ReadAll(resp.Body)
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"Request with future timestamp should be rejected (BR-GATEWAY-075: Clock Skew Attack Prevention)",
				"response", string(body))

			testLogger.Info("Step 4: Verify error message contains 'timestamp in future' or 'clock skew'")
			bodyStr := string(body)
			Expect(bodyStr).To(Or(
				ContainSubstring("timestamp in future"),
				ContainSubstring("clock skew"),
			), "Error message should indicate clock skew concern")

			testLogger.Info("✅ Clock skew attack prevented - future timestamp rejected")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Test 19d PASSED: Clock Skew Attack Prevented (Future Timestamp)")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})

		It("should reject alerts with invalid timestamp format", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Scenario: Send alert with invalid timestamp format")
			testLogger.Info("Expected: HTTP 400 Bad Request with 'invalid timestamp format' message")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			testLogger.Info("Step 1: Create Prometheus webhook payload")
			payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
				AlertName: "TestAlertInvalidTimestamp",
				Namespace: testNamespace,
				Severity:  "warning",
				PodName:   "test-pod",
				Labels: map[string]string{
					"scenario": "invalid-timestamp-format",
				},
			})

			testLogger.Info("Step 2: Send request with INVALID timestamp format")
			req, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", "not-a-valid-timestamp")

			resp, err := httpClient.Do(req)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer func() { _ = resp.Body.Close() }()

			testLogger.Info("Step 3: Verify HTTP 400 Bad Request response")
			body, _ := io.ReadAll(resp.Body)
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"Request with invalid timestamp should be rejected (BR-GATEWAY-074: Timestamp Validation)",
				"response", string(body))

			testLogger.Info("Step 4: Verify error message contains 'invalid' or 'format'")
			bodyStr := string(body)
			Expect(bodyStr).To(Or(
				ContainSubstring("invalid"),
				ContainSubstring("format"),
			), "Error message should indicate format problem")

			testLogger.Info("✅ Invalid timestamp format rejected")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Test 19e PASSED: Invalid Timestamp Format Rejected")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})
	})
})
