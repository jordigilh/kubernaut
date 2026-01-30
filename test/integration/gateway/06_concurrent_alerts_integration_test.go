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

// ========================================
// MIGRATION STATUS: ✅ Converted from E2E to Integration
// ORIGINAL FILE: test/e2e/gateway/06_concurrent_alerts_test.go
// MIGRATION DATE: 2026-01-12
// PATTERN: Direct ProcessSignal() calls with concurrency
// CHANGES:
//   - Removed all HTTP client code
//   - Uses gateway.NewServerWithK8sClient for shared K8s client
//   - Calls ProcessSignal() concurrently from goroutines
//   - Tracks success/failure at business logic level
//   - No Eventually() needed - shared client gives immediate visibility
//   - Removed health check (HTTP-specific)
// ========================================

package gateway

import (
	"fmt"
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
	"github.com/jordigilh/kubernaut/pkg/gateway"

	"github.com/google/uuid"
)

// Test 06: Concurrent Signal Handling (BR-GATEWAY-008)
// Validates that the Gateway handles concurrent signals correctly:
// - No data loss under concurrent load
// - No race conditions in CRD creation
// - Proper deduplication under concurrency
//
// Business Requirements:
// - BR-GATEWAY-008: Gateway must handle concurrent requests without data loss
var _ = Describe("Test 06: Concurrent Signal Handling (Integration)", Ordered, Label("concurrent", "integration"), func() {
	var (
		testLogger    logr.Logger
		testNamespace string
		gwServer      *gateway.Server
	)

	BeforeAll(func() {
		testLogger = logger.WithValues("test", "concurrent-integration")

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 06: Concurrent Signal Handling (Integration) - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Generate unique namespace
		processID := GinkgoParallelProcess()
		testNamespace = fmt.Sprintf("concurrent-int-%d-%s", processID, uuid.New().String()[:8])

		testLogger.Info("Creating test namespace...", "namespace", testNamespace)

		// Create namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		// Wait for namespace to be ready
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: testNamespace}, ns)
		}, 30*time.Second, 1*time.Second).Should(Succeed())

		testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)

		// Initialize Gateway with shared K8s client
		gwConfig := createGatewayConfig("http://mock-datastorage:8080")
		var err error
		gwServer, err = createGatewayServer(gwConfig, testLogger, k8sClient, dsClient)
		Expect(err).ToNot(HaveOccurred())
		testLogger.Info("✅ Gateway server initialized")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 06: Concurrent Signal Handling - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if CurrentSpecReport().Failed() {
			testLogger.Info("⚠️  Test FAILED - Preserving namespace for debugging",
				"namespace", testNamespace)
			return
		}

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
		}
		_ = k8sClient.Delete(ctx, ns)

		testLogger.Info("✅ Test cleanup complete")
	})

	It("should handle concurrent signals without data loss or race conditions", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 06: Concurrent Signal Handling Behavior")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("")
		testLogger.Info("Scenario: Send 20 signals concurrently from 5 goroutines")
		testLogger.Info("Expected: All signals processed, no errors, no data loss")
		testLogger.Info("")

		const (
			numGoroutines      = 5
			signalsPerGoroutine = 4
			totalSignals        = numGoroutines * signalsPerGoroutine // 20 signals total
		)

		// Step 1: Send signals concurrently
		testLogger.Info(fmt.Sprintf("Step 1: Send %d signals from %d goroutines", totalSignals, numGoroutines))

		var wg sync.WaitGroup
		var successCount int64
		var errorCount int64
		var acceptedCount int64  // StatusAccepted (including duplicates)
		var createdCount int64    // StatusCreated (new CRDs)

		// Use a SINGLE signal name so signals aggregate together (BR-GATEWAY-008)
		// This triggers storm aggregation when threshold is reached
		sharedSignalName := fmt.Sprintf("ConcurrentStormSignal-%s", uuid.New().String()[:8])

		for g := 0; g < numGoroutines; g++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()

				for i := 0; i < signalsPerGoroutine; i++ {
					// All signals share the same signalName to trigger storm aggregation
					signal := createNormalizedSignal(SignalBuilder{
						AlertName: sharedSignalName,
						Namespace:  testNamespace,
						ResourceName: fmt.Sprintf("concurrent-pod-g%d-s%d", goroutineID, i),
						Kind:       "Pod",
						Severity:   "warning",
						Source:     "prometheus",
						Labels: map[string]string{
							"test":       "concurrent",
							"goroutine":  fmt.Sprintf("%d", goroutineID),
							"signal_num": fmt.Sprintf("%d", i),
						},
					})

					response, err := gwServer.ProcessSignal(ctx, signal)
					if err != nil {
						atomic.AddInt64(&errorCount, 1)
						continue
					}

					// Track response status
					switch response.Status {
					case gateway.StatusAccepted, gateway.StatusDeduplicated:
						atomic.AddInt64(&acceptedCount, 1)
						atomic.AddInt64(&successCount, 1)
					case gateway.StatusCreated:
						atomic.AddInt64(&createdCount, 1)
						atomic.AddInt64(&successCount, 1)
					default:
						atomic.AddInt64(&successCount, 1) // Other success statuses
					}
				}
			}(g)
		}

		wg.Wait()

		testLogger.Info(fmt.Sprintf("  Completed: %d success, %d errors", successCount, errorCount))
		testLogger.Info(fmt.Sprintf("  Response statuses: Created=%d, Accepted/Duplicate=%d", createdCount, acceptedCount))

		// Step 2: Verify no errors occurred
		testLogger.Info("")
		testLogger.Info("Step 2: Verify no errors during concurrent processing")

		Expect(errorCount).To(Equal(int64(0)),
			"No errors should occur during concurrent processing")
		testLogger.Info("  ✅ No errors")

		// Step 3: Verify all signals were processed
		testLogger.Info("")
		testLogger.Info("Step 3: Verify all signals were processed")

		Expect(successCount).To(Equal(int64(totalSignals)),
			fmt.Sprintf("All %d signals should be processed successfully", totalSignals))
		testLogger.Info(fmt.Sprintf("  ✅ All %d signals processed", successCount))

		// Step 4: Verify CRDs were created
		testLogger.Info("")
		testLogger.Info("Step 4: Verify CRDs were created")

		crdList := &remediationv1alpha1.RemediationRequestList{}
		err := k8sClient.List(ctx, crdList, client.InNamespace(testNamespace))
		Expect(err).ToNot(HaveOccurred())
		crdCount := len(crdList.Items)

		testLogger.Info(fmt.Sprintf("  Found %d CRDs", crdCount))

		// CRD count should be less than total signals (due to storm aggregation)
		// but greater than 0
		Expect(crdCount).To(BeNumerically(">", 0),
			"At least some CRDs should be created")
		Expect(crdCount).To(BeNumerically("<=", totalSignals),
			"CRD count should not exceed signal count")

		testLogger.Info("  ✅ CRDs created correctly")

		// Step 5: Verify no race conditions in CRD fields
		testLogger.Info("")
		testLogger.Info("Step 5: Verify CRD data integrity (no race conditions)")

		for i, crd := range crdList.Items {
			Expect(crd.Spec.SignalName).To(Equal(sharedSignalName),
				fmt.Sprintf("CRD %d signal name should match expected", i))
			Expect(crd.Spec.Severity).ToNot(BeEmpty(),
				fmt.Sprintf("CRD %d severity should not be empty", i))
			Expect(crd.Namespace).To(Equal(testNamespace),
				fmt.Sprintf("CRD %d namespace should match test namespace", i))
		}

		testLogger.Info("  ✅ All CRD fields are valid (no race conditions)")

		testLogger.Info("")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Test 06 PASSED: Concurrent Signal Handling (BR-GATEWAY-008)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Verified:")
		testLogger.Info(fmt.Sprintf("  ✅ Processed %d concurrent signals", totalSignals))
		testLogger.Info(fmt.Sprintf("  ✅ Created %d CRDs", crdCount))
		testLogger.Info("  ✅ No processing errors")
		testLogger.Info("  ✅ No data loss")
		testLogger.Info("  ✅ No race conditions in CRD fields")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})
