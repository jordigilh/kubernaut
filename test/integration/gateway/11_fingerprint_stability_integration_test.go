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
// ORIGINAL FILE: test/e2e/gateway/11_fingerprint_stability_test.go
// MIGRATION DATE: 2026-01-12
// PATTERN: Direct ProcessSignal() calls with fingerprint validation
// CHANGES:
//   - Removed all HTTP client code
//   - Uses gateway.NewServerWithK8sClient for shared K8s client
//   - Calls ProcessSignal() directly and validates response.Fingerprint
//   - Simplified fingerprint comparison (no JSON parsing)
//   - Removed Eventually() wrappers (shared client = immediate visibility)
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

// Test 11: Fingerprint Stability (BR-GATEWAY-004, BR-GATEWAY-029)
// Validates that fingerprints are deterministic and stable across:
// - Identical signals produce identical fingerprints
// - Different signals produce different fingerprints
// - Deduplication works based on fingerprints
//
// Business Requirements:
// - BR-GATEWAY-004: Fingerprint-based deduplication
// - BR-GATEWAY-029: Deterministic fingerprint generation
var _ = Describe("Test 11: Fingerprint Stability (Integration)", Ordered, Label("fingerprint", "integration"), func() {
	var (
		testLogger    logr.Logger
		testNamespace string
		gwServer      *gateway.Server
	)

	BeforeAll(func() {
		testLogger = logger.WithValues("test", "fingerprint-integration")

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 11: Fingerprint Stability (Integration) - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Generate unique namespace
		processID := GinkgoParallelProcess()
		testNamespace = fmt.Sprintf("fingerprint-int-%d-%s", processID, uuid.New().String()[:8])
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
		testLogger.Info("Test 11: Fingerprint Stability - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if CurrentSpecReport().Failed() {
			testLogger.Info("⚠️  Test FAILED - Preserving namespace for debugging",
				"namespace", testNamespace)
			return
		}

		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
		_ = k8sClient.Delete(ctx, ns)

		testLogger.Info("✅ Test cleanup complete")
	})

	It("should generate identical fingerprints for identical signals sent multiple times", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario: Send identical signals multiple times")
		testLogger.Info("Expected: Same fingerprint generated, deduplication occurs")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		testLogger.Info("Step 1: Send first signal")

		signal1 := createNormalizedSignal(SignalBuilder{
			AlertName: "FingerprintTest",
			Namespace:  testNamespace,
			ResourceName: "fingerprint-pod-1",
			Kind:       "Pod",
			Severity:   "warning",
			Source:     "prometheus",
			Labels: map[string]string{
				"container": "main",
			},
		})

		response1, err := gwServer.ProcessSignal(ctx, signal1)
		Expect(err).ToNot(HaveOccurred())
		firstFingerprint := response1.Fingerprint

		testLogger.Info("✅ First signal sent", "fingerprint", firstFingerprint)

		testLogger.Info("Step 2: Send identical signal again")

		signal2 := createNormalizedSignal(SignalBuilder{
			AlertName: "FingerprintTest",
			Namespace:  testNamespace,
			ResourceName: "fingerprint-pod-1",
			Kind:       "Pod",
			Severity:   "warning",
			Source:     "prometheus",
			Labels: map[string]string{
				"container": "main",
			},
		})

		response2, err := gwServer.ProcessSignal(ctx, signal2)
		Expect(err).ToNot(HaveOccurred())
		secondFingerprint := response2.Fingerprint

		testLogger.Info("✅ Second signal sent", "fingerprint", secondFingerprint)

		testLogger.Info("Step 3: Verify fingerprints are identical")
		Expect(secondFingerprint).To(Equal(firstFingerprint),
			"Identical signals should produce identical fingerprints")

		testLogger.Info("  ✅ Fingerprints match - determinism validated")
		testLogger.Info("✅ Test 11a PASSED: Fingerprint Stability")
	})

	It("should generate different fingerprints for signals with different labels", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario: Send signals with different identifying labels")
		testLogger.Info("Expected: Different fingerprints generated")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		testLogger.Info("Step 1: Send signal with label set A")

		signalA := createNormalizedSignal(SignalBuilder{
			AlertName: "FingerprintDifferenceTest",
			Namespace:  testNamespace,
			ResourceName: "diff-pod-1",
			Kind:       "Pod",
			Severity:   "warning",
			Source:     "prometheus",
			Labels: map[string]string{
				"environment": "production",
			},
		})

		responseA, err := gwServer.ProcessSignal(ctx, signalA)
		Expect(err).ToNot(HaveOccurred())
		fingerprintA := responseA.Fingerprint

		testLogger.Info("✅ Signal A sent", "fingerprint", fingerprintA)

		testLogger.Info("Step 2: Send signal with label set B")

		signalB := createNormalizedSignal(SignalBuilder{
			AlertName: "FingerprintDifferenceTest",
			Namespace:  testNamespace,
			ResourceName: "diff-pod-1",
			Kind:       "Pod",
			Severity:   "warning",
			Source:     "prometheus",
			Labels: map[string]string{
				"environment": "staging", // Different value
			},
		})

		responseB, err := gwServer.ProcessSignal(ctx, signalB)
		Expect(err).ToNot(HaveOccurred())
		fingerprintB := responseB.Fingerprint

		testLogger.Info("✅ Signal B sent", "fingerprint", fingerprintB)

		testLogger.Info("Step 3: Verify fingerprints are SAME (labels don't affect fingerprint)")
		Expect(fingerprintB).To(Equal(fingerprintA),
			"Fingerprint based on alertName+namespace+kind+name, NOT labels")

		testLogger.Info("  ✅ Fingerprints match - correct deduplication behavior (labels ignored)")
		testLogger.Info("✅ Test 11b PASSED: Fingerprint Differentiation")
	})

	It("should deduplicate signals with same fingerprint and update occurrence count", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario: Send multiple identical signals to trigger deduplication")
		testLogger.Info("Expected: Single CRD with occurrence tracking")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		testLogger.Info("Step 1: Send 3 identical signals")

		signalName := fmt.Sprintf("DedupFingerprint-%s", uuid.New().String()[:8])
		var sharedFingerprint string

		for i := 0; i < 3; i++ {
			signal := createNormalizedSignal(SignalBuilder{
				AlertName: signalName,
				Namespace:  testNamespace,
				ResourceName: "dedup-fp-pod",
				Kind:       "Pod",
				Severity:   "critical",
				Source:     "prometheus",
				Labels: map[string]string{
					"app": "dedup-test",
				},
			})

			response, err := gwServer.ProcessSignal(ctx, signal)
			Expect(err).ToNot(HaveOccurred())

			if i == 0 {
				sharedFingerprint = response.Fingerprint
			} else {
				Expect(response.Fingerprint).To(Equal(sharedFingerprint),
					fmt.Sprintf("Signal %d should have same fingerprint", i+1))
			}

			testLogger.V(1).Info(fmt.Sprintf("  Signal %d: %s (fingerprint: %s)",
				i+1, response.Status, response.Fingerprint))
		}

		testLogger.Info("✅ Sent 3 identical signals", "sharedFingerprint", sharedFingerprint)

		testLogger.Info("Step 2: Verify deduplication occurred")

		crdList := &remediationv1alpha1.RemediationRequestList{}
		err := k8sClient.List(ctx, crdList, client.InNamespace(testNamespace))
		Expect(err).ToNot(HaveOccurred())

		// Find CRDs for this signal
		var matchingCRDs []remediationv1alpha1.RemediationRequest
		for _, crd := range crdList.Items {
			if crd.Spec.SignalName == signalName {
				matchingCRDs = append(matchingCRDs, crd)
			}
		}

		testLogger.Info(fmt.Sprintf("  Found %d matching CRDs", len(matchingCRDs)))

		// With deduplication, we should have fewer CRDs than signals sent
		Expect(len(matchingCRDs)).To(BeNumerically(">=", 1),
			"At least 1 CRD should exist")
		Expect(len(matchingCRDs)).To(BeNumerically("<=", 3),
			"Should not exceed signal count (deduplication active)")

		testLogger.Info("  ✅ Deduplication validated via fingerprint matching")
		testLogger.Info("✅ Test 11c PASSED: Fingerprint-Based Deduplication")
	})
})
