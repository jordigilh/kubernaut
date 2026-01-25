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

// Test 09: Signal Validation & Rejection (BR-GATEWAY-003, BR-GATEWAY-043)
// BEHAVIOR: Gateway validates incoming payloads and rejects malformed ones
// CORRECTNESS: Invalid payloads return HTTP 400, valid payloads return 201/202
// Parallel-safe: No namespace needed, tests HTTP responses only
var _ = Describe("Test 09: Signal Validation & Rejection (BR-GATEWAY-003)", Ordered, func() {
	var (
		testCancel context.CancelFunc
		testLogger logr.Logger
		httpClient *http.Client
	)

	BeforeAll(func() {
		_, testCancel = context.WithTimeout(ctx, 3*time.Minute)
		testLogger = logger.WithValues("test", "signal-validation")
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 09: Signal Validation & Rejection - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		if testCancel != nil {
			testCancel()
		}
		testLogger.Info("✅ Test cleanup complete")
	})

	It("should reject invalid payloads and accept valid ones with correct HTTP status codes", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 09: Signal Validation - Behavior & Correctness")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// BEHAVIOR TEST: Gateway rejects syntactically invalid JSON
		// CORRECTNESS: Returns HTTP 400 Bad Request
		testLogger.Info("Step 1: Verify Gateway rejects invalid JSON syntax")

		invalidJSON := []byte(`{"alerts": [{"status": "firing", INVALID}]}`)
		req1, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(invalidJSON))
		Expect(err).ToNot(HaveOccurred())
		req1.Header.Set("Content-Type", "application/json")
		req1.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
		resp1, err := httpClient.Do(req1)
		Expect(err).ToNot(HaveOccurred())
		body1, _ := io.ReadAll(resp1.Body)
		_ = resp1.Body.Close()

		// CORRECTNESS: Invalid JSON must return 400
		Expect(resp1.StatusCode).To(Equal(http.StatusBadRequest),
			"Invalid JSON syntax should return HTTP 400 (BR-GATEWAY-003)")
		testLogger.Info(fmt.Sprintf("  ✅ Invalid JSON rejected: HTTP %d", resp1.StatusCode))
		testLogger.V(1).Info(fmt.Sprintf("  Response body: %s", string(body1)))

		// BEHAVIOR TEST: Gateway rejects empty payload
		// CORRECTNESS: Returns HTTP 400 Bad Request
		testLogger.Info("")
		testLogger.Info("Step 2: Verify Gateway rejects empty payload")

		emptyPayload := []byte(`{}`)
		req2, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(emptyPayload))
		Expect(err).ToNot(HaveOccurred())
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
		resp2, err := httpClient.Do(req2)
		Expect(err).ToNot(HaveOccurred())
		body2, _ := io.ReadAll(resp2.Body)
		_ = resp2.Body.Close()

		// CORRECTNESS: Empty payload must return 400
		Expect(resp2.StatusCode).To(Equal(http.StatusBadRequest),
			"Empty payload should return HTTP 400 (BR-GATEWAY-003)")
		testLogger.Info(fmt.Sprintf("  ✅ Empty payload rejected: HTTP %d", resp2.StatusCode))
		testLogger.V(1).Info(fmt.Sprintf("  Response body: %s", string(body2)))

		// BEHAVIOR TEST: Gateway rejects payload with empty alerts array
		// CORRECTNESS: Returns HTTP 400 Bad Request
		testLogger.Info("")
		testLogger.Info("Step 3: Verify Gateway rejects empty alerts array")

		emptyAlerts := map[string]interface{}{
			"alerts": []map[string]interface{}{},
		}
		emptyAlertsBytes, _ := json.Marshal(emptyAlerts)
		req3, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(emptyAlertsBytes))
		Expect(err).ToNot(HaveOccurred())
		req3.Header.Set("Content-Type", "application/json")
		req3.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
		resp3, err := httpClient.Do(req3)
		Expect(err).ToNot(HaveOccurred())
		body3, _ := io.ReadAll(resp3.Body)
		_ = resp3.Body.Close()

		// CORRECTNESS: Empty alerts array must return 400
		Expect(resp3.StatusCode).To(Equal(http.StatusBadRequest),
			"Empty alerts array should return HTTP 400 (BR-GATEWAY-003)")
		testLogger.Info(fmt.Sprintf("  ✅ Empty alerts array rejected: HTTP %d", resp3.StatusCode))
		testLogger.V(1).Info(fmt.Sprintf("  Response body: %s", string(body3)))

		// BEHAVIOR TEST: Gateway accepts well-formed valid payload
		// CORRECTNESS: Returns HTTP 201 (Created) or 202 (Accepted for buffering)
		testLogger.Info("")
		testLogger.Info("Step 4: Verify Gateway accepts valid payload")

		validPayload := createPrometheusWebhookPayload(PrometheusAlertPayload{
			AlertName: fmt.Sprintf("ValidationTest-%d", time.Now().UnixNano()),
			Namespace: "default",
			PodName:   "validation-test-pod",
			Severity:  "warning",
			Annotations: map[string]string{
				"summary":     "Validation test alert",
				"description": "Testing signal validation",
			},
		})

		var resp4 *http.Response
		Eventually(func() error {
			var err error
			req4, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(validPayload))
			if err != nil {
				return err
			}
			req4.Header.Set("Content-Type", "application/json")
			req4.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
			resp4, err = httpClient.Do(req4)
			return err
		}, 10*time.Second, 1*time.Second).Should(Succeed())
		body4, _ := io.ReadAll(resp4.Body)
		_ = resp4.Body.Close()

		// CORRECTNESS: Valid payload must return 201 or 202
		Expect(resp4.StatusCode).To(Or(Equal(http.StatusCreated), Equal(http.StatusAccepted)),
			"Valid payload should return HTTP 201 or 202 (BR-GATEWAY-003)")
		testLogger.Info(fmt.Sprintf("  ✅ Valid payload accepted: HTTP %d", resp4.StatusCode))
		testLogger.V(1).Info(fmt.Sprintf("  Response body: %s", string(body4)))

		// BEHAVIOR TEST: Gateway differentiates between client and server errors
		// CORRECTNESS: Malformed request = 4xx, not 5xx
		testLogger.Info("")
		testLogger.Info("Step 5: Verify error classification (4xx vs 5xx)")

		// All our invalid requests should have returned 4xx client errors
		Expect(resp1.StatusCode).To(BeNumerically(">=", 400))
		Expect(resp1.StatusCode).To(BeNumerically("<", 500),
			"Invalid JSON should be client error (4xx), not server error (5xx)")

		Expect(resp2.StatusCode).To(BeNumerically(">=", 400))
		Expect(resp2.StatusCode).To(BeNumerically("<", 500),
			"Empty payload should be client error (4xx), not server error (5xx)")

		Expect(resp3.StatusCode).To(BeNumerically(">=", 400))
		Expect(resp3.StatusCode).To(BeNumerically("<", 500),
			"Empty alerts should be client error (4xx), not server error (5xx)")

		testLogger.Info("  ✅ All validation errors correctly classified as 4xx client errors")

		testLogger.Info("")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Test 09 PASSED: Signal Validation & Rejection")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Behavior Validated:")
		testLogger.Info("  ✅ Gateway rejects invalid JSON syntax")
		testLogger.Info("  ✅ Gateway rejects empty payloads")
		testLogger.Info("  ✅ Gateway rejects empty alerts arrays")
		testLogger.Info("  ✅ Gateway accepts valid payloads")
		testLogger.Info("Correctness Validated:")
		testLogger.Info("  ✅ Invalid requests return HTTP 400")
		testLogger.Info(fmt.Sprintf("  ✅ Valid requests return HTTP %d", resp4.StatusCode))
		testLogger.Info("  ✅ All validation errors are 4xx (client errors)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})
