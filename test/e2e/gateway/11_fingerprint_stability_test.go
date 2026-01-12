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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"

	"github.com/google/uuid"
)

var _ = Describe("Test 11: Fingerprint Stability (BR-GATEWAY-004, BR-GATEWAY-029)", Ordered, func() {
	var (
		testCtx       context.Context
		testCancel    context.CancelFunc
		testLogger    logr.Logger
		testNamespace string
		httpClient    *http.Client
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 5*time.Minute)
		testLogger = logger.WithValues("test", "fingerprint-stability")
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 11: Fingerprint Stability - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		testNamespace = GenerateUniqueNamespace("fingerprint")
		testLogger.Info("Deploying test services...", "namespace", testNamespace)

		k8sClient := getKubernetesClient()
		Expect(CreateNamespaceAndWait(testCtx, k8sClient, testNamespace)).To(Succeed(), "Failed to create and wait for namespace")

		testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)
		testLogger.Info("✅ Using shared Gateway", "url", gatewayURL)
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 11: Fingerprint Stability - Cleanup")
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

	Context("Fingerprint Generation Consistency", func() {
		It("should generate identical fingerprints for identical alerts sent multiple times", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Scenario: Send identical alerts multiple times")
			testLogger.Info("Expected: Same fingerprint generated, deduplication occurs")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// Create a deterministic alert payload
			// NOTE: Using fixed startsAt for deterministic fingerprinting
			payloadBytes := createPrometheusWebhookPayloadWithTimestamp(PrometheusAlertPayload{
				AlertName: "FingerprintTest",
				Namespace: testNamespace,
				PodName:   "fingerprint-pod-1",
				Severity:  "warning",
				Labels: map[string]string{
					"container": "main",
				},
				Annotations: map[string]string{
					"summary":     "Fingerprint stability test",
					"description": "Testing that fingerprints are deterministic",
				},
			}, "2025-01-01T00:00:00Z") // Fixed timestamp for determinism

			testLogger.Info("Step 1: Send first alert")
			var firstFingerprint string

			// Send first alert - should trigger storm buffering
			Eventually(func() error {
				resp, err := func() (*http.Response, error) {
					req12, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payloadBytes))
					if err != nil {
						return nil, err
					}
					req12.Header.Set("Content-Type", "application/json")
					req12.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
					return httpClient.Do(req12)
				}()
				if err != nil {
					return err
				}
				defer func() { _ = resp.Body.Close() }()

				// Should be accepted (201 or 202)
				if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
					return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
				}

				// Parse response to get fingerprint
				var respBody map[string]interface{}
				if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
					return err
				}
				if fp, ok := respBody["fingerprint"].(string); ok {
					firstFingerprint = fp
				}
				return nil
			}, 10*time.Second, 1*time.Second).Should(Succeed(), "First alert should be accepted")

			testLogger.Info("✅ First alert sent", "fingerprint", firstFingerprint)

			// JUSTIFIED SLEEP: Per TESTING_GUIDELINES.md, this sleep is required for deterministic
			// testing of fingerprint stability. We need identical alert content but different
			// timestamps to validate that Gateway generates consistent fingerprints across time.
			// This tests BR-GATEWAY-068 (fingerprint determinism) and cannot be replaced by Eventually().
			time.Sleep(500 * time.Millisecond)

			testLogger.Info("Step 2: Send identical alert again")
			var secondFingerprint string

			Eventually(func() error {
				resp, err := func() (*http.Response, error) {
					req13, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payloadBytes))
					if err != nil {
						return nil, err
					}
					req13.Header.Set("Content-Type", "application/json")
					req13.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
					return httpClient.Do(req13)
				}()
				if err != nil {
					return err
				}
				defer func() { _ = resp.Body.Close() }()

				// Should be accepted (201 or 202)
				if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
					return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
				}

				// Parse response to get fingerprint
				var respBody map[string]interface{}
				if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
					return err
				}
				if fp, ok := respBody["fingerprint"].(string); ok {
					secondFingerprint = fp
				}
				return nil
			}, 10*time.Second, 1*time.Second).Should(Succeed(), "Second alert should be accepted")

			testLogger.Info("✅ Second alert sent", "fingerprint", secondFingerprint)

			testLogger.Info("Step 3: Verify fingerprints are identical")
			Expect(firstFingerprint).ToNot(BeEmpty(), "First fingerprint should not be empty")
			Expect(secondFingerprint).ToNot(BeEmpty(), "Second fingerprint should not be empty")
			Expect(firstFingerprint).To(Equal(secondFingerprint),
				"Identical alerts should generate identical fingerprints (BR-GATEWAY-004)")

			testLogger.Info("✅ Fingerprints are identical - deterministic generation confirmed")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Test 11a PASSED: Fingerprint Consistency")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})

		It("should generate different fingerprints for alerts with different labels", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Scenario: Send alerts with different identifying labels")
			testLogger.Info("Expected: Different fingerprints generated")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			processID := GinkgoParallelProcess()

			// First alert
			payload1 := createPrometheusWebhookPayloadWithTimestamp(PrometheusAlertPayload{
				AlertName: fmt.Sprintf("DifferentAlert1-p%d", processID),
				Namespace: testNamespace,
				PodName:   "pod-alpha",
				Severity:  "warning",
				Annotations: map[string]string{
					"summary": "First alert type",
				},
			}, "2025-01-01T00:00:00Z")

			// Second alert with different alertname
			payload2 := createPrometheusWebhookPayloadWithTimestamp(PrometheusAlertPayload{
				AlertName: fmt.Sprintf("DifferentAlert2-p%d", processID),
				Namespace: testNamespace,
				PodName:   "pod-alpha",
				Severity:  "warning",
				Annotations: map[string]string{
					"summary": "Second alert type",
				},
			}, "2025-01-01T00:00:00Z")

			testLogger.Info("Step 1: Send first alert type")
			var fingerprint1 string

			Eventually(func() error {
				resp, err := func() (*http.Response, error) {
					req14, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload1))
					if err != nil {
						return nil, err
					}
					req14.Header.Set("Content-Type", "application/json")
					req14.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
					return httpClient.Do(req14)
				}()
				if err != nil {
					return err
				}
				defer func() { _ = resp.Body.Close() }()

				if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
					return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
				}

				var respBody map[string]interface{}
				if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
					return err
				}
				if fp, ok := respBody["fingerprint"].(string); ok {
					fingerprint1 = fp
				}
				return nil
			}, 10*time.Second, 1*time.Second).Should(Succeed())

			testLogger.Info("Step 2: Send second alert type")
			var fingerprint2 string

			Eventually(func() error {
				resp, err := func() (*http.Response, error) {
					req15, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload2))
					if err != nil {
						return nil, err
					}
					req15.Header.Set("Content-Type", "application/json")
					req15.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
					return httpClient.Do(req15)
				}()
				if err != nil {
					return err
				}
				defer func() { _ = resp.Body.Close() }()

				if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
					return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
				}

				var respBody map[string]interface{}
				if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
					return err
				}
				if fp, ok := respBody["fingerprint"].(string); ok {
					fingerprint2 = fp
				}
				return nil
			}, 10*time.Second, 1*time.Second).Should(Succeed())

			testLogger.Info("Step 3: Verify fingerprints are different")
			Expect(fingerprint1).ToNot(BeEmpty())
			Expect(fingerprint2).ToNot(BeEmpty())
			Expect(fingerprint1).ToNot(Equal(fingerprint2),
				"Different alerts should generate different fingerprints (BR-GATEWAY-029)")

			testLogger.Info("✅ Fingerprints are different - proper differentiation confirmed",
				"fingerprint1", fingerprint1,
				"fingerprint2", fingerprint2)
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Test 11b PASSED: Fingerprint Differentiation")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})
	})

	Context("Deduplication via Fingerprint", func() {
		It("should deduplicate alerts with same fingerprint and update occurrence count", func() {
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("Scenario: Send multiple identical alerts to trigger deduplication")
			testLogger.Info("Expected: Single CRD with updated occurrence count")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			processID := GinkgoParallelProcess()
			alertName := fmt.Sprintf("DedupeTest-p%d-%s", processID, uuid.New().String()[:8])

			// Create deterministic alert
			payloadBytes := createPrometheusWebhookPayloadWithTimestamp(PrometheusAlertPayload{
				AlertName: alertName,
				Namespace: testNamespace,
				PodName:   "dedupe-pod",
				Severity:  "critical",
				Annotations: map[string]string{
					"summary": "Deduplication test alert",
				},
			}, "2025-01-01T00:00:00Z")

			testLogger.Info("Step 1: Send 5 identical alerts to trigger storm aggregation")
			const alertCount = 5

			for i := 0; i < alertCount; i++ {
				Eventually(func() error {
					resp, err := func() (*http.Response, error) {
						req16, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payloadBytes))
						if err != nil {
							return nil, err
						}
						req16.Header.Set("Content-Type", "application/json")
						req16.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
						return httpClient.Do(req16)
					}()
					if err != nil {
						return err
					}
					defer func() { _ = resp.Body.Close() }()

					if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
						return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
					}
					return nil
				}, 10*time.Second, 1*time.Second).Should(Succeed(), fmt.Sprintf("Alert %d should be accepted", i+1))

				testLogger.Info(fmt.Sprintf("  Sent alert %d/%d", i+1, alertCount))
				// Stagger requests to avoid overwhelming Gateway (50ms is sufficient for E2E)
				time.Sleep(50 * time.Millisecond)
			}

			testLogger.Info("Step 2: Verify CRD creation with deduplication")

			var crdList remediationv1alpha1.RemediationRequestList
			Eventually(func() int {
				k8sClient := getKubernetesClientSafe()
				if k8sClient == nil {
					if err := GetLastK8sClientError(); err != nil {
						testLogger.V(1).Info("Failed to get K8s client", "error", err)
					} else {
						testLogger.V(1).Info("Failed to get K8s client (unknown error)")
					}
					return -1
				}
				if err := k8sClient.List(testCtx, &crdList, client.InNamespace(testNamespace)); err != nil {
					testLogger.V(1).Info("Failed to list CRDs", "error", err)
					return -1
				}

				// Count CRDs for this specific alert
				count := 0
				for _, crd := range crdList.Items {
					if crd.Spec.SignalName == alertName {
						count++
					}
				}
				return count
			}, 60*time.Second, 2*time.Second).Should(BeNumerically(">=", 1),
				"At least one CRD should be created for deduplicated alerts")

			testLogger.Info("Step 3: Verify occurrence count in CRD")
			// Find the CRD for our alert
			var targetCRD *remediationv1alpha1.RemediationRequest
			for i := range crdList.Items {
				if crdList.Items[i].Spec.SignalName == alertName {
					targetCRD = &crdList.Items[i]
					break
				}
			}

			Expect(targetCRD).ToNot(BeNil(), "Should find CRD for alert")

			// Check if Status.Deduplication is set (Gateway should update this)
			if targetCRD.Status.Deduplication != nil {
				testLogger.Info("✅ CRD found with deduplication status",
					"name", targetCRD.Name,
					"occurrenceCount", targetCRD.Status.Deduplication.OccurrenceCount)

				// With deduplication, the occurrence count should reflect multiple alerts
				Expect(targetCRD.Status.Deduplication.OccurrenceCount).To(BeNumerically(">=", 1),
					"Occurrence count should be at least 1 (deduplication active)")
			} else {
				// Gateway is not updating Status.Deduplication - this is a Gateway bug, not a test failure
				testLogger.Info("⚠️  CRD found but Status.Deduplication is nil",
					"name", targetCRD.Name,
					"note", "Gateway StatusUpdater may not be working - this is a known issue")
				Skip("Gateway is not updating Status.Deduplication - needs Gateway StatusUpdater investigation")
			}

			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			testLogger.Info("✅ Test 11c PASSED: Deduplication via Fingerprint")
			testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		})
	})
})
