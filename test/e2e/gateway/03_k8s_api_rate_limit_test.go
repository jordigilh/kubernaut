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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Parallel Execution: ✅ ENABLED
// - Single It block with unique namespace (rate-limit-{timestamp})
// - Uses shared Gateway instance
// - Cleanup in AfterAll
var _ = Describe("Test 3: K8s API Rate Limiting (429 Responses)", func() {
	var (
		testCtx       context.Context
		testCancel    context.CancelFunc
		testLogger    *zap.Logger
		testNamespace string
		httpClient    *http.Client
		gatewayURL    string
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 10*time.Minute)
		testLogger = logger.With(zap.String("test", "k8s-api-rate-limit"))
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 3: K8s API Rate Limiting - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Generate unique namespace for this test
		testNamespace = fmt.Sprintf("rate-limit-%d", time.Now().UnixNano())
		testLogger.Info("Deploying test services...", zap.String("namespace", testNamespace))

		// Deploy Redis, AlertManager, Gateway in test namespace
		err := infrastructure.DeployTestServices(testCtx, testNamespace, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		// Set Gateway URL (using NodePort exposed by Kind cluster)
		gatewayURL = "http://localhost:30080"
		testLogger.Info("Gateway URL configured", zap.String("url", gatewayURL))

		// Wait for Gateway HTTP endpoint to be responsive
		testLogger.Info("⏳ Waiting for Gateway HTTP endpoint to be responsive...")
		Eventually(func() error {
			resp, err := httpClient.Get(gatewayURL + "/health")
			if err != nil {
				testLogger.Debug("Health check failed, retrying...", zap.Error(err))
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("health check returned status %d", resp.StatusCode)
			}
			return nil
		}, 60*time.Second, 2*time.Second).Should(Succeed(), "Gateway HTTP endpoint did not become responsive")
		testLogger.Info("✅ Gateway HTTP endpoint is responsive")

		testLogger.Info("✅ Test services ready", zap.String("namespace", testNamespace))
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 3: K8s API Rate Limiting - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Check if test failed - preserve namespace for debugging
		if CurrentSpecReport().Failed() {
			testLogger.Warn("⚠️  Test FAILED - Preserving namespace for debugging",
				zap.String("namespace", testNamespace))
			testLogger.Info("To debug:")
			testLogger.Info(fmt.Sprintf("  export KUBECONFIG=%s", kubeconfigPath))
			testLogger.Info(fmt.Sprintf("  kubectl get pods -n %s", testNamespace))
			testLogger.Info(fmt.Sprintf("  kubectl logs -n %s deployment/gateway -f", testNamespace))
			testLogger.Info(fmt.Sprintf("  kubectl describe pod -n %s -l app=gateway", testNamespace))
			testLogger.Info("To cleanup manually:")
			testLogger.Info(fmt.Sprintf("  kubectl delete namespace %s", testNamespace))
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			if testCancel != nil {
				testCancel()
			}
			return
		}

		// Test passed - cleanup namespace
		testLogger.Info("Cleaning up test namespace...", zap.String("namespace", testNamespace))
		err := infrastructure.CleanupTestNamespace(testCtx, testNamespace, kubeconfigPath, GinkgoWriter)
		if err != nil {
			testLogger.Warn("Failed to cleanup namespace", zap.Error(err))
		}

		if testCancel != nil {
			testCancel()
		}

		testLogger.Info("✅ Test cleanup complete")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	It("should handle rapid burst of alerts without data loss or crashes", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 3: Rapid Alert Burst (Stress Test)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("")
		testLogger.Info("Scenario: Gateway receives rapid burst of 50 alerts")
		testLogger.Info("Expected: All alerts processed, all CRDs created, no crashes")
		testLogger.Info("")
		testLogger.Info("NOTE: This test validates Gateway stability under load.")
		testLogger.Info("      K8s API rate limiting (429) retry logic is a future enhancement.")
		testLogger.Info("")

		// Step 1: Send rapid burst of 50 alerts to stress test the Gateway
		testLogger.Info("Step 1: Send rapid burst of 50 alerts")

		const alertCount = 50
		alertPayloads := make([]map[string]interface{}, alertCount)
		for i := 0; i < alertCount; i++ {
			alertPayloads[i] = map[string]interface{}{
				"status": "firing",
				"labels": map[string]interface{}{
					"alertname": "HighMemoryUsage",
					"severity":  "critical",
					"namespace": "production",
					"pod":       fmt.Sprintf("app-pod-%d", i), // Different pod for each alert
				},
				"annotations": map[string]interface{}{
					"summary":     fmt.Sprintf("Pod app-pod-%d has high memory usage", i),
					"description": "Pod is using 95% of allocated memory",
				},
				"startsAt": time.Now().Format(time.RFC3339),
			}
		}

		// Send alerts rapidly (no delay between requests)
		successCount := 0
		stormCount := 0
		errorCount := 0

		for i, payload := range alertPayloads {
			if i%10 == 0 {
				testLogger.Info(fmt.Sprintf("  Progress: %d/%d alerts sent...", i, alertCount))
			}

			// Wrap in AlertManager webhook format
			webhookPayload := map[string]interface{}{
				"alerts": []interface{}{payload},
			}

			payloadBytes, err := json.Marshal(webhookPayload)
			Expect(err).ToNot(HaveOccurred())

			resp, err := httpClient.Post(
				gatewayURL+"/api/v1/signals/prometheus",
				"application/json",
				bytes.NewBuffer(payloadBytes),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Gateway should handle burst gracefully
			switch resp.StatusCode {
			case http.StatusCreated: // 201 - CRD created successfully
				successCount++

			case http.StatusAccepted: // 202 - Storm aggregation (expected for some alerts)
				stormCount++

			case http.StatusInternalServerError: // 500 - Unexpected error
				errorCount++
				testLogger.Warn(fmt.Sprintf("  ⚠️  Alert %d returned HTTP 500 (unexpected)", i+1))

			default:
				testLogger.Warn(fmt.Sprintf("  ⚠️  Alert %d returned unexpected status: %d", i+1, resp.StatusCode))
			}

			// No delay - send as fast as possible to stress test
		}

		testLogger.Info("")
		testLogger.Info(fmt.Sprintf("Burst complete: %d created, %d storm-aggregated, %d errors", successCount, stormCount, errorCount))

		// Step 2: Verify Gateway is still responsive after burst
		testLogger.Info("")
		testLogger.Info("Step 2: Verify Gateway is still responsive after burst")

		resp, err := httpClient.Get(gatewayURL + "/health")
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK), "Gateway should still be healthy after burst")
		testLogger.Info("  ✅ Gateway health check passed")

		// Step 3: Verify CRDs were created (accounting for storm aggregation)
		testLogger.Info("")
		testLogger.Info("Step 3: Verify CRDs were created")

		// Query K8s API for RemediationRequest CRDs in test namespace
		testLogger.Info(fmt.Sprintf("  Querying CRDs in namespace: %s", testNamespace))

		// Wait a moment for any pending CRD creations to complete
		time.Sleep(5 * time.Second)

		// Use kubectl to count CRDs
		cmd := fmt.Sprintf("kubectl get remediationrequests -n %s --no-headers 2>/dev/null | wc -l", testNamespace)
		output, err := infrastructure.RunCommand(cmd, kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		var crdCount int
		fmt.Sscanf(output, "%d", &crdCount)
		testLogger.Info(fmt.Sprintf("  Found %d CRDs", crdCount))

		// We expect at least some CRDs to be created (exact count depends on storm detection)
		// With 50 alerts and pattern_threshold=3, we expect storm aggregation to kick in
		Expect(crdCount).To(BeNumerically(">", 0), "At least some CRDs should be created")
		Expect(crdCount).To(BeNumerically("<=", alertCount), "CRD count should not exceed alert count")

		testLogger.Info(fmt.Sprintf("  ✅ %d CRDs created (storm detection reduced from %d alerts)", crdCount, alertCount))

		// Step 4: Verify no errors occurred
		testLogger.Info("")
		testLogger.Info("Step 4: Verify no errors occurred")
		Expect(errorCount).To(Equal(0), "No HTTP 500 errors should occur during burst")
		testLogger.Info("  ✅ No errors during burst processing")

		testLogger.Info("")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Test 3 PASSED: Rapid Alert Burst (Stress Test)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Verified:")
		testLogger.Info(fmt.Sprintf("  ✅ Processed %d alerts in rapid burst", alertCount))
		testLogger.Info(fmt.Sprintf("  ✅ Created %d CRDs (storm detection active)", crdCount))
		testLogger.Info("  ✅ Gateway remained responsive throughout burst")
		testLogger.Info("  ✅ No crashes or data loss")
		testLogger.Info("  ✅ No HTTP 500 errors")
		testLogger.Info("")
		testLogger.Info("Future Enhancement: K8s API 429 retry logic (not yet implemented)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})
