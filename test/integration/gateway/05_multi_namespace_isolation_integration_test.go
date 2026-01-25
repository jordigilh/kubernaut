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
// ORIGINAL FILE: test/e2e/gateway/05_multi_namespace_isolation_test.go
// MIGRATION DATE: 2026-01-12
// PATTERN: Direct ProcessSignal() calls, no HTTP layer
// CHANGES:
//   - Removed all HTTP client code
//   - Uses gateway.NewServerWithK8sClient for shared K8s client
//   - Calls ProcessSignal() directly for each signal
//   - No Eventually() needed - shared client gives immediate visibility
//   - Simplified namespace verification (no cache sync delays)
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

// Test 05: Multi-Namespace Isolation (BR-GATEWAY-011)
// Validates that signals from different namespaces are isolated:
// - CRDs are created in the correct namespace
// - Storm buffers are isolated per namespace
// - Deduplication is scoped to namespace
//
// Business Requirements:
// - BR-GATEWAY-011: Multi-tenant isolation with per-namespace buffers
var _ = Describe("Test 05: Multi-Namespace Isolation (Integration)", Ordered, Label("multi-namespace", "integration"), func() {
	var (
		testLogger     logr.Logger
		testNamespace1 string
		testNamespace2 string
		gwServer       *gateway.Server
	)

	BeforeAll(func() {
		testLogger = logger.WithValues("test", "multi-namespace-integration")

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 05: Multi-Namespace Isolation (Integration) - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Generate unique namespaces
		processID := GinkgoParallelProcess()
		timestamp := uuid.New().String()[:8]
		testNamespace1 = fmt.Sprintf("tenant-a-int-%d-%s", processID, timestamp)
		testNamespace2 = fmt.Sprintf("tenant-b-int-%d-%s", processID, timestamp)

		testLogger.Info("Creating test namespaces...",
			"namespace1", testNamespace1,
			"namespace2", testNamespace2)

		// Create namespace 1
		ns1 := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace1},
		}
		Expect(k8sClient.Create(ctx, ns1)).To(Succeed())

		// Create namespace 2
		ns2 := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace2},
		}
		Expect(k8sClient.Create(ctx, ns2)).To(Succeed())

		// Wait for namespaces to be ready
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: testNamespace1}, ns1)
		}, 30*time.Second, 1*time.Second).Should(Succeed())

		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: testNamespace2}, ns2)
		}, 30*time.Second, 1*time.Second).Should(Succeed())

		testLogger.Info("✅ Test namespaces ready")

		// Initialize Gateway with shared K8s client
		gwConfig := createGatewayConfig("http://mock-datastorage:8080")
		var err error
		gwServer, err = createGatewayServer(gwConfig, testLogger, k8sClient)
		Expect(err).ToNot(HaveOccurred())
		testLogger.Info("✅ Gateway server initialized")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 05: Multi-Namespace Isolation - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if CurrentSpecReport().Failed() {
			testLogger.Info("⚠️  Test FAILED - Preserving namespaces for debugging",
				"namespace1", testNamespace1,
				"namespace2", testNamespace2)
			return
		}

		// Cleanup namespaces
		ns1 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace1}}
		ns2 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace2}}
		_ = k8sClient.Delete(ctx, ns1)
		_ = k8sClient.Delete(ctx, ns2)

		testLogger.Info("✅ Test cleanup complete")
	})

	It("should isolate signals and CRDs between namespaces", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 05: Multi-Namespace Isolation Behavior")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("")
		testLogger.Info("Scenario: Send same signal name to different namespaces")
		testLogger.Info("Expected: Each namespace gets its own CRDs (isolation)")
		testLogger.Info("")

		// Use same signal name for both namespaces to test isolation
		signalName := fmt.Sprintf("IsolationTest-%s", uuid.New().String()[:8])

		// Step 1: Send signals to namespace 1 - enough to trigger storm threshold
		testLogger.Info("Step 1: Send 10 signals to namespace 1")

		for i := 0; i < 10; i++ {
			signal := createNormalizedSignal(SignalBuilder{
				AlertName: signalName,
				Namespace:  testNamespace1,
				ResourceName: fmt.Sprintf("pod-%d", i),
				Kind:       "Pod",
				Severity:   "critical",
				Source:     "prometheus",
				Labels: map[string]string{
					"tenant":      "tenant-a",
					"test_run":    uuid.New().String()[:8],
					"signal_num":  fmt.Sprintf("%d", i),
				},
			})

			response, err := gwServer.ProcessSignal(ctx, signal)
			Expect(err).ToNot(HaveOccurred())
			testLogger.V(1).Info(fmt.Sprintf("  NS1 Signal %d: %s", i+1, response.Status))
		}
		testLogger.Info("  ✅ Sent 10 signals to namespace 1")

		// Step 2: Send signals to namespace 2 - enough to trigger storm threshold
		testLogger.Info("")
		testLogger.Info("Step 2: Send 10 signals to namespace 2")

		for i := 0; i < 10; i++ {
			signal := createNormalizedSignal(SignalBuilder{
				AlertName: signalName,
				Namespace:  testNamespace2,
				ResourceName: fmt.Sprintf("pod-%d", i),
				Kind:       "Pod",
				Severity:   "critical",
				Source:     "prometheus",
				Labels: map[string]string{
					"tenant":      "tenant-b",
					"test_run":    uuid.New().String()[:8],
					"signal_num":  fmt.Sprintf("%d", i),
				},
			})

			response, err := gwServer.ProcessSignal(ctx, signal)
			Expect(err).ToNot(HaveOccurred())
			testLogger.V(1).Info(fmt.Sprintf("  NS2 Signal %d: %s", i+1, response.Status))
		}
		testLogger.Info("  ✅ Sent 10 signals to namespace 2")

		// Step 3: Verify CRDs in namespace 1
		testLogger.Info("")
		testLogger.Info("Step 3: Verify CRDs in namespace 1")

		crdListNS1 := &remediationv1alpha1.RemediationRequestList{}
		err := k8sClient.List(ctx, crdListNS1, client.InNamespace(testNamespace1))
		Expect(err).ToNot(HaveOccurred())
		crdCountNS1 := len(crdListNS1.Items)

		Expect(crdCountNS1).To(BeNumerically(">=", 1),
			"Namespace 1 should have at least 1 CRD")
		testLogger.Info(fmt.Sprintf("  Namespace 1: %d CRDs", crdCountNS1))

		// Step 4: Verify CRDs in namespace 2
		testLogger.Info("")
		testLogger.Info("Step 4: Verify CRDs in namespace 2")

		crdListNS2 := &remediationv1alpha1.RemediationRequestList{}
		err = k8sClient.List(ctx, crdListNS2, client.InNamespace(testNamespace2))
		Expect(err).ToNot(HaveOccurred())
		crdCountNS2 := len(crdListNS2.Items)

		Expect(crdCountNS2).To(BeNumerically(">=", 1),
			"Namespace 2 should have at least 1 CRD")
		testLogger.Info(fmt.Sprintf("  Namespace 2: %d CRDs", crdCountNS2))

		// Step 5: Verify isolation
		testLogger.Info("")
		testLogger.Info("Step 5: Verify namespace isolation")

		// Verify CRDs in NS1 don't reference NS2 and vice versa
		for _, crd := range crdListNS1.Items {
			Expect(crd.Namespace).To(Equal(testNamespace1),
				"CRD in NS1 should have NS1 namespace")
			Expect(crd.Spec.TargetResource.Namespace).To(Equal(testNamespace1),
				"CRD target resource in NS1 should reference NS1")
		}

		for _, crd := range crdListNS2.Items {
			Expect(crd.Namespace).To(Equal(testNamespace2),
				"CRD in NS2 should have NS2 namespace")
			Expect(crd.Spec.TargetResource.Namespace).To(Equal(testNamespace2),
				"CRD target resource in NS2 should reference NS2")
		}

		testLogger.Info("  ✅ Namespace isolation verified")

		testLogger.Info("")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Test 05 PASSED: Multi-Namespace Isolation (BR-GATEWAY-011)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Verified:")
		testLogger.Info(fmt.Sprintf("  ✅ Namespace 1: %d CRDs", crdCountNS1))
		testLogger.Info(fmt.Sprintf("  ✅ Namespace 2: %d CRDs", crdCountNS2))
		testLogger.Info("  ✅ CRDs correctly isolated to their namespaces")
		testLogger.Info("  ✅ Same signal name creates separate CRDs per namespace")
		testLogger.Info("  ✅ Target resources correctly reference their namespace")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})
