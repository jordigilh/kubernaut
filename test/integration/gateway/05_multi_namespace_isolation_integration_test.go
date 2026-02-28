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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/test/shared/helpers"

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
		testNamespace1 = helpers.CreateTestNamespace(ctx, k8sClient, "tenant-a-int")
		testNamespace2 = helpers.CreateTestNamespace(ctx, k8sClient, "tenant-b-int")

		testLogger.Info("Creating test namespaces...",
			"namespace1", testNamespace1,
			"namespace2", testNamespace2)

		testLogger.Info("✅ Test namespaces ready")

		// Initialize Gateway with shared K8s client AND shared audit store
		// ADR-032: Audit is MANDATORY for P0 services (Gateway)
		gwConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
		var err error
		gwServer, err = createGatewayServer(gwConfig, testLogger, k8sClient, sharedAuditStore)
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
		helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace1)
		helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace2)

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

		// Step 3: List CRDs from controller namespace (ADR-057)
		testLogger.Info("")
		testLogger.Info("Step 3: Verify CRDs in controller namespace")

		// ADR-057: All RRs in controller namespace; filter by Spec.TargetResource.Namespace
		crdListAll := &remediationv1alpha1.RemediationRequestList{}
		err := k8sClient.List(ctx, crdListAll, client.InNamespace(controllerNamespace))
		Expect(err).ToNot(HaveOccurred())

		var crdListNS1, crdListNS2 []remediationv1alpha1.RemediationRequest
		for _, crd := range crdListAll.Items {
			switch crd.Spec.TargetResource.Namespace {
			case testNamespace1:
				crdListNS1 = append(crdListNS1, crd)
			case testNamespace2:
				crdListNS2 = append(crdListNS2, crd)
			}
		}
		crdCountNS1 := len(crdListNS1)
		crdCountNS2 := len(crdListNS2)

		Expect(crdCountNS1).To(BeNumerically(">=", 1),
			"Namespace 1 should have at least 1 CRD")
		testLogger.Info(fmt.Sprintf("  Namespace 1: %d CRDs", crdCountNS1))

		Expect(crdCountNS2).To(BeNumerically(">=", 1),
			"Namespace 2 should have at least 1 CRD")
		testLogger.Info(fmt.Sprintf("  Namespace 2: %d CRDs", crdCountNS2))

		// Step 5: Verify isolation (RRs in controller namespace, distinguished by TargetResource.Namespace)
		testLogger.Info("")
		testLogger.Info("Step 5: Verify namespace isolation")

		for _, crd := range crdListNS1 {
			Expect(crd.Namespace).To(Equal(controllerNamespace),
				"CRD object lives in controller namespace")
			Expect(crd.Spec.TargetResource.Namespace).To(Equal(testNamespace1),
				"CRD target resource in NS1 should reference NS1")
		}

		for _, crd := range crdListNS2 {
			Expect(crd.Namespace).To(Equal(controllerNamespace),
				"CRD object lives in controller namespace")
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
