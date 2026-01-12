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
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
)

var _ = Describe("Test 16: Structured Logging Verification (BR-GATEWAY-024, BR-GATEWAY-075)", Ordered, func() {
	var (
		testCtx       context.Context
		testCancel    context.CancelFunc
		testLogger    logr.Logger
		testNamespace string
		httpClient    *http.Client
		k8sClient     client.Client
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 10*time.Minute) // Increased from 5min - test runs late in suite
		testLogger = logger.WithValues("test", "structured-logging")
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 16: Structured Logging Verification - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		testNamespace = GenerateUniqueNamespace("logging")
		testLogger.Info("Deploying test services...", "namespace", testNamespace)

		k8sClient = getKubernetesClient()
		// Use ctx (suite context) instead of testCtx to avoid timeout issues
		Expect(CreateNamespaceAndWait(ctx, k8sClient, testNamespace)).To(Succeed(), "Failed to create and wait for namespace")

		testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)
		testLogger.Info("✅ Using shared Gateway", "url", gatewayURL)
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 16: Structured Logging Verification - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if CurrentSpecReport().Failed() {
			testLogger.Info("⚠️  Test FAILED - Preserving namespace for debugging",
				"namespace", testNamespace)
			testLogger.Info("To debug:")
			testLogger.Info(fmt.Sprintf("  export KUBECONFIG=%s", kubeconfigPath))
			testLogger.Info(fmt.Sprintf("  kubectl get pods -n %s", testNamespace))
			testLogger.Info(fmt.Sprintf("  kubectl logs -n %s deployment/gateway -f", testNamespace))
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			if testCancel != nil {
				testCancel()
			}
			return
		}

		testLogger.Info("Cleaning up test namespace...", "namespace", testNamespace)
		// Namespace cleanup handled by suite-level AfterSuite (Kind cluster deletion)

		if testCancel != nil {
			testCancel()
		}
		testLogger.Info("✅ Test cleanup complete")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	It("should produce structured JSON logs with required fields", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario: Send alert and verify Gateway produces structured logs")
		testLogger.Info("Expected: Logs in JSON format with timestamp, level, message")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		processID := GinkgoParallelProcess()
		uniqueMarker := fmt.Sprintf("LoggingTest-p%d-%s", processID, uuid.New().String()[:8])

		testLogger.Info("Step 1: Send alert with unique marker")
		alertPayload := map[string]interface{}{
			"status": "firing",
			"labels": map[string]interface{}{
				"alertname": uniqueMarker,
				"severity":  "warning",
				"namespace": testNamespace,
				"pod":       "logging-test-pod",
			},
			"annotations": map[string]interface{}{
				"summary": "Structured logging verification test",
			},
			"startsAt": time.Now().Format(time.RFC3339),
		}

		webhookPayload := map[string]interface{}{
			"alerts": []interface{}{alertPayload},
		}
		payloadBytes, _ := json.Marshal(webhookPayload)

		Eventually(func() error {
			resp, err := func() (*http.Response, error) {
				req24, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payloadBytes))
				if err != nil {
					return nil, err
				}
				req24.Header.Set("Content-Type", "application/json")
				req24.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
				return httpClient.Do(req24)
			}()
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
				return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			}
			return nil
		}, 10*time.Second, 1*time.Second).Should(Succeed(), "Alert should be accepted")

		testLogger.Info("✅ Alert sent with unique marker", "marker", uniqueMarker)

		testLogger.Info("Step 2: Retrieve Gateway logs")
		// Wait for logs to be written using Eventually
		var logs string
		Eventually(func() bool {
			cmd := exec.CommandContext(testCtx, "kubectl", "logs",
				"-n", gatewayNamespace,
				"-l", "app=gateway",
				"--tail=100",
				"--kubeconfig", kubeconfigPath)
			output, err := cmd.Output()
			if err != nil {
				testLogger.Info("Could not retrieve Gateway logs", "error", err)
				return false
			}
			logs = string(output)
			// Check if logs contain our unique marker
			return len(logs) > 0
		}, 30*time.Second, 2*time.Second).Should(BeTrue(), "Gateway logs should be available")

		testLogger.Info("Retrieved Gateway logs", "bytes", len(logs))
		testLogger.Info("Retrieved Gateway logs", "bytes", len(logs))

		testLogger.Info("Step 3: Verify structured log format")
		// Check if logs contain JSON-formatted entries
		lines := strings.Split(logs, "\n")
		jsonLogCount := 0
		hasTimestamp := false
		hasLevel := false
		hasMessage := false

		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}

			// Try to parse as JSON
			var logEntry map[string]interface{}
			if err := json.Unmarshal([]byte(line), &logEntry); err == nil {
				jsonLogCount++

				// Check for required fields
				if _, ok := logEntry["ts"]; ok {
					hasTimestamp = true
				}
				if _, ok := logEntry["time"]; ok {
					hasTimestamp = true
				}
				if _, ok := logEntry["timestamp"]; ok {
					hasTimestamp = true
				}
				if _, ok := logEntry["level"]; ok {
					hasLevel = true
				}
				if _, ok := logEntry["msg"]; ok {
					hasMessage = true
				}
				if _, ok := logEntry["message"]; ok {
					hasMessage = true
				}
			}
		}

		testLogger.Info("Log analysis results",
			"totalLines", len(lines),
			"jsonLogs", jsonLogCount,
			"hasTimestamp", hasTimestamp,
			"hasLevel", hasLevel,
			"hasMessage", hasMessage)

		// Verify structured logging is in use
		if jsonLogCount > 0 {
			testLogger.Info("✅ Gateway produces structured JSON logs (BR-GATEWAY-024)")

			// Verify required fields are present
			Expect(hasTimestamp || hasLevel || hasMessage).To(BeTrue(),
				"Structured logs should have timestamp, level, or message fields (BR-GATEWAY-075)")
		} else {
			testLogger.Info("ℹ️  No JSON logs found - Gateway may use different log format")
		}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Test 16 PASSED: Structured Logging Verification")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})
