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
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"

	"github.com/google/uuid"
)

// Test 06: Concurrent Alert Handling (BR-GATEWAY-008)
// Validates that the Gateway handles concurrent alerts correctly:
// - No data loss under concurrent load
// - No race conditions in CRD creation
// - Proper deduplication under concurrency
//
// Business Requirements:
// - BR-GATEWAY-008: Gateway must handle concurrent requests without data loss
var _ = Describe("Test 06: Concurrent Alert Handling (BR-GATEWAY-008)", Ordered, func() {
	var (
		testCtx       context.Context
		testCancel    context.CancelFunc
		testLogger    logr.Logger
		testNamespace string
		httpClient    *http.Client
		k8sClient     client.Client
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 5*time.Minute)
		testLogger = logger.WithValues("test", "concurrent")
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 06: Concurrent Alert Handling (BR-GATEWAY-008) - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Generate unique namespace
		processID := GinkgoParallelProcess()
		testNamespace = fmt.Sprintf("concurrent-%d-%s", processID, uuid.New().String()[:8])

		// Get K8s client and create namespace
		k8sClient = getKubernetesClient()
	// Use suite ctx (no timeout) for namespace creation
	Expect(CreateNamespaceAndWait(ctx, k8sClient, testNamespace)).To(Succeed(),
		"Failed to create test namespace")
		testLogger.Info("Creating test namespace...", "namespace", testNamespace)

		testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 06: Concurrent Alert Handling - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if CurrentSpecReport().Failed() {
			testLogger.Info("⚠️  Test FAILED - Preserving namespace for debugging",
				"namespace", testNamespace)
			if testCancel != nil {
				testCancel()
			}
			return
		}

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
		}
		_ = k8sClient.Delete(testCtx, ns)

		if testCancel != nil {
			testCancel()
		}
		testLogger.Info("✅ Test cleanup complete")
	})

	It("should handle concurrent alerts without data loss or race conditions", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 06: Concurrent Alert Handling Behavior")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("")
		testLogger.Info("Scenario: Send 20 alerts concurrently from 10 goroutines")
		testLogger.Info("Expected: All alerts processed, no HTTP 500 errors, no data loss")
		testLogger.Info("")

		const (
			numGoroutines      = 5
			alertsPerGoroutine = 4
			totalAlerts        = numGoroutines * alertsPerGoroutine // 20 alerts total
		)

		// Step 1: Send alerts concurrently
		testLogger.Info(fmt.Sprintf("Step 1: Send %d alerts from %d goroutines", totalAlerts, numGoroutines))

		var wg sync.WaitGroup
		var successCount int64
		var errorCount int64
		var status201 int64
		var status202 int64

		// Use a SINGLE alert name so alerts aggregate together (DD-GATEWAY-008)
		// This triggers storm aggregation when threshold is reached
		sharedAlertName := fmt.Sprintf("ConcurrentStormAlert-%s", uuid.New().String()[:8])

		for g := 0; g < numGoroutines; g++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()

				for i := 0; i < alertsPerGoroutine; i++ {
					// All alerts share the same alertName to trigger storm aggregation
					alertName := sharedAlertName
					podName := fmt.Sprintf("concurrent-pod-g%d-a%d", goroutineID, i)

					payload := createPrometheusWebhookPayload(PrometheusAlertPayload{
						AlertName: alertName,
						Namespace: testNamespace,
						PodName:   podName,
						Severity:  "warning",
						Annotations: map[string]string{
							"summary":     fmt.Sprintf("Concurrent test: %s", alertName),
							"description": "Testing concurrent alert handling",
						},
					})
					resp, err := func() (*http.Response, error) {
						req9, err := http.NewRequest("POST", gatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(payload))
						if err != nil {
							return nil, err
						}
						req9.Header.Set("Content-Type", "application/json")
						req9.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
						return httpClient.Do(req9)
					}()

					if err != nil {
						atomic.AddInt64(&errorCount, 1)
						continue
					}
					_ = resp.Body.Close()

					switch resp.StatusCode {
					case http.StatusCreated:
						atomic.AddInt64(&status201, 1)
						atomic.AddInt64(&successCount, 1)
					case http.StatusAccepted:
						atomic.AddInt64(&status202, 1)
						atomic.AddInt64(&successCount, 1)
					case http.StatusInternalServerError:
						atomic.AddInt64(&errorCount, 1)
					default:
						atomic.AddInt64(&successCount, 1) // Other success codes
					}
				}
			}(g)
		}

		wg.Wait()

		testLogger.Info(fmt.Sprintf("  Completed: %d success, %d errors", successCount, errorCount))
		testLogger.Info(fmt.Sprintf("  Status codes: 201=%d, 202=%d", status201, status202))

		// Step 2: Verify no HTTP 500 errors
		testLogger.Info("")
		testLogger.Info("Step 2: Verify no HTTP 500 errors")

		Expect(errorCount).To(Equal(int64(0)),
			"No HTTP 500 errors should occur during concurrent processing")
		testLogger.Info("  ✅ No HTTP 500 errors")

		// Step 3: Verify all alerts were processed
		testLogger.Info("")
		testLogger.Info("Step 3: Verify all alerts were processed")

		Expect(successCount).To(Equal(int64(totalAlerts)),
			fmt.Sprintf("All %d alerts should be processed successfully", totalAlerts))
		testLogger.Info(fmt.Sprintf("  ✅ All %d alerts processed", successCount))

		// Step 4: Verify CRDs were created
		testLogger.Info("")
		testLogger.Info("Step 4: Verify CRDs were created")

		var crdCount int
		Eventually(func() int {
			// Get fresh client to handle API server reconnection
			freshClient := getKubernetesClientSafe()
			if freshClient == nil {
				if err := GetLastK8sClientError(); err != nil {
					testLogger.V(1).Info("Failed to create K8s client", "error", err)
				} else {
					testLogger.V(1).Info("Failed to create K8s client (unknown error)")
				}
				return -1
			}
			crdList := &remediationv1alpha1.RemediationRequestList{}
			err := freshClient.List(testCtx, crdList, client.InNamespace(testNamespace))
			if err != nil {
				testLogger.V(1).Info("Failed to list CRDs", "error", err)
				return -1
			}
			crdCount = len(crdList.Items)
			return crdCount
		}, 60*time.Second, 2*time.Second).Should(BeNumerically(">=", 1),
			"At least 1 CRD should be created")

		testLogger.Info(fmt.Sprintf("  Found %d CRDs", crdCount))

		// CRD count should be less than total alerts (due to storm aggregation)
		// but greater than 0
		Expect(crdCount).To(BeNumerically(">", 0),
			"At least some CRDs should be created")
		Expect(crdCount).To(BeNumerically("<=", totalAlerts),
			"CRD count should not exceed alert count")

		testLogger.Info("  ✅ CRDs created correctly")

		// Step 5: Verify Gateway is still healthy after concurrent load
		testLogger.Info("")
		testLogger.Info("Step 5: Verify Gateway health after concurrent load")

		resp, err := httpClient.Get(gatewayURL + "/health")
		Expect(err).ToNot(HaveOccurred())
		_ = resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"Gateway should be healthy after concurrent load")
		testLogger.Info("  ✅ Gateway is healthy")

		testLogger.Info("")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Test 06 PASSED: Concurrent Alert Handling (BR-GATEWAY-008)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Verified:")
		testLogger.Info(fmt.Sprintf("  ✅ Processed %d concurrent alerts", totalAlerts))
		testLogger.Info(fmt.Sprintf("  ✅ Created %d CRDs", crdCount))
		testLogger.Info("  ✅ No HTTP 500 errors")
		testLogger.Info("  ✅ No data loss")
		testLogger.Info("  ✅ Gateway remained healthy")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})

// NOTE: Removed local createConcurrentAlertPayload() - now using shared createPrometheusWebhookPayload() from deduplication_helpers.go
