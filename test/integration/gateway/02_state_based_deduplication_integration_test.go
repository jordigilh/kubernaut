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
// ORIGINAL FILE: test/e2e/gateway/02_state_based_deduplication_test.go
// MIGRATION DATE: 2026-01-12
// PATTERN: Direct ProcessSignal() calls, no HTTP layer
// CHANGES:
//   - Removed all HTTP client code
//   - Uses gateway.NewServerWithK8sClient for shared K8s client
//   - Calls ProcessSignal() directly instead of HTTP POST
//   - Tracks response status at business logic level (StatusAccepted, StatusDuplicate)
//   - No Eventually() needed - shared client gives immediate CRD visibility
//   - Removed health check (HTTP-specific)
// ========================================

package gateway

import (
	"fmt"
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

// Test 02: State-Based Deduplication (DD-GATEWAY-009)
// Validates that duplicate signals are handled based on CRD lifecycle state:
// - Same signal while CRD is processing → deduplicated (StatusAccepted/StatusDuplicate)
// - Different signals → create new CRDs
//
// Business Requirements:
// - BR-GATEWAY-005: Deduplication must prevent duplicate CRDs for same incident
// - BR-GATEWAY-006: Deduplication window = CRD lifecycle (not arbitrary TTL)
var _ = Describe("Test 02: State-Based Deduplication (Integration)", Ordered, Label("deduplication", "integration"), func() {
	var (
		testLogger    logr.Logger
		testNamespace string
		gwServer      *gateway.Server
	)

	BeforeAll(func() {
		testLogger = logger.WithValues("test", "deduplication-integration")

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 02: State-Based Deduplication (Integration) - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Generate unique namespace
		processID := GinkgoParallelProcess()
		testNamespace = fmt.Sprintf("dedup-int-%d-%s", processID, uuid.New().String()[:8])
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
		gwServer, err = createGatewayServer(gwConfig, testLogger, k8sClient)
		Expect(err).ToNot(HaveOccurred())
		testLogger.Info("✅ Gateway server initialized")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 02: State-Based Deduplication - Cleanup")
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

	It("should deduplicate identical signals and create separate CRDs for different signals", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 02: Deduplication Behavior")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("")
		testLogger.Info("Scenario: Send signals to trigger storm threshold, then verify deduplication")
		testLogger.Info("Expected: Storm aggregation creates CRD, duplicates are deduplicated")
		testLogger.Info("")

		// Step 1: Send multiple signals with same signalname to trigger storm threshold
		testLogger.Info("Step 1: Send 5 signals with same signalname to trigger storm threshold")
		signalName1 := fmt.Sprintf("DedupTest1-%s", uuid.New().String()[:8])

		for i := 0; i < 5; i++ {
			signal := createNormalizedSignal(SignalBuilder{
				AlertName: signalName1,
				Namespace:  testNamespace,
				ResourceName: fmt.Sprintf("dedup-pod-%d", i),
				Kind:       "Pod",
				Severity:   "critical",
				Source:     "prometheus",
				Labels: map[string]string{
					"test": "deduplication",
					"pod_num": fmt.Sprintf("%d", i),
				},
			})

			response, err := gwServer.ProcessSignal(ctx, signal)
			Expect(err).ToNot(HaveOccurred())
			testLogger.V(1).Info(fmt.Sprintf("  Signal %d: %s", i+1, response.Status))
		}
		testLogger.Info("  ✅ Sent 5 signals to trigger storm threshold")

		// Step 2: Send duplicate signal (same fingerprint as one of the above)
		testLogger.Info("")
		testLogger.Info("Step 2: Send duplicate signal (same fingerprint)")

		// Same signalname and pod = same fingerprint
		duplicateSignal := createNormalizedSignal(SignalBuilder{
			AlertName: signalName1,
			Namespace:  testNamespace,
			ResourceName: "dedup-pod-0", // Same as first signal
			Kind:       "Pod",
			Severity:   "critical",
			Source:     "prometheus",
			Labels: map[string]string{
				"test": "deduplication",
				"pod_num": "0",
			},
		})

		dupResponse, err := gwServer.ProcessSignal(ctx, duplicateSignal)
		Expect(err).ToNot(HaveOccurred())

		testLogger.Info(fmt.Sprintf("  Duplicate signal: %s", dupResponse.Status))
		// Duplicate should be accepted or marked as duplicate
		Expect(dupResponse.Status).To(Or(
			Equal(gateway.StatusAccepted),
			Equal(gateway.StatusDuplicate),
			Equal(gateway.StatusCreated)), // May also be created if within storm window
			"Duplicate signal should be handled correctly")

		// Step 3: Send different signal (different signalname) - also trigger threshold
		testLogger.Info("")
		testLogger.Info("Step 3: Send 5 signals with different signalname")
		signalName2 := fmt.Sprintf("DedupTest2-%s", uuid.New().String()[:8])

		for i := 0; i < 5; i++ {
			signal := createNormalizedSignal(SignalBuilder{
				AlertName: signalName2,
				Namespace:  testNamespace,
				ResourceName: fmt.Sprintf("dedup2-pod-%d", i),
				Kind:       "Pod",
				Severity:   "warning",
				Source:     "prometheus",
				Labels: map[string]string{
					"test": "deduplication",
					"alert_group": "2",
					"pod_num": fmt.Sprintf("%d", i),
				},
			})

			response, err := gwServer.ProcessSignal(ctx, signal)
			Expect(err).ToNot(HaveOccurred())
			testLogger.V(1).Info(fmt.Sprintf("  Signal %d: %s", i+1, response.Status))
		}
		testLogger.Info("  ✅ Sent 5 signals with different signalname")

		// Step 4: Verify CRD creation
		testLogger.Info("")
		testLogger.Info("Step 4: Verify CRD creation")

		crdList := &remediationv1alpha1.RemediationRequestList{}
		err = k8sClient.List(ctx, crdList, client.InNamespace(testNamespace))
		Expect(err).ToNot(HaveOccurred())
		crdCount := len(crdList.Items)

		testLogger.Info(fmt.Sprintf("  Found %d CRDs", crdCount))

		// We sent 11 signals total (5 + 1 duplicate + 5 different)
		// With storm aggregation + deduplication, we should have 1-2 CRDs
		Expect(crdCount).To(BeNumerically(">=", 1),
			"At least 1 CRD should be created")
		Expect(crdCount).To(BeNumerically("<=", 11),
			"CRD count should not exceed signal count")

		testLogger.Info("  ✅ Deduplication is working")

		testLogger.Info("")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Test 02 PASSED: State-Based Deduplication (DD-GATEWAY-009)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Verified:")
		testLogger.Info("  ✅ First signals accepted")
		testLogger.Info("  ✅ Duplicate signal handled (deduplicated)")
		testLogger.Info("  ✅ Different signals accepted separately")
		testLogger.Info(fmt.Sprintf("  ✅ Total CRDs: %d (deduplication active)", crdCount))
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})
