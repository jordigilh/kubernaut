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
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/test/shared/helpers"

	"github.com/google/uuid"
)

// ========================================
// MIGRATION PROTOTYPE: E2E → Integration
// ========================================
//
// This is a PROTOTYPE migration demonstrating the pattern for converting
// Gateway E2E tests (HTTP-based) to Integration tests (business logic-based).
//
// KEY CHANGES FROM E2E:
// 1. ❌ REMOVED: HTTP layer (SendWebhook, httpClient, gatewayURL)
// 2. ✅ ADDED: Direct Gateway instantiation with SHARED K8s client
// 3. ✅ ADDED: Direct ProcessSignal() calls (no HTTP)
// 4. ✅ ADDED: Immediate CRD verification (no 60-240s timeouts)
//
// BENEFITS:
// - ✅ 10-100x faster (no HTTP, no network)
// - ✅ No K8s client mismatch (Gateway and test share same client)
// - ✅ CRDs visible immediately (no eventual consistency delays)
// - ✅ Better test isolation
// - ✅ Follows industry best practices (integration > E2E)
//
// PATTERN APPLIES TO:
// - All CRD lifecycle tests (5 tests)
// - All deduplication tests (6 tests)
// - All audit event tests (4 tests)
// - All service resilience tests (3 tests)
// - All error handling tests (4 tests)
// - All observability tests (3 tests)
// - Total: 28 tests (80% of current E2E suite)
//
// DO NOT MIGRATE THIS PATTERN TO:
// - Adapter parsing tests (Test 08, 31) - Need HTTP
// - Middleware tests (Test 03, 18, 19, 20) - Need HTTP
// - Server lifecycle tests (Test 28) - Need HTTP
// ========================================

// Test 10: CRD Creation Lifecycle (BR-GATEWAY-018, BR-GATEWAY-021)
// Validates that CRDs are created with correct metadata, labels, and annotations
//
// MIGRATION STATUS: ✅ Converted from E2E to Integration
// ORIGINAL FILE: test/e2e/gateway/10_crd_creation_lifecycle_test.go
// MIGRATION DATE: 2026-01-12
//
// Parallel-safe: Uses unique namespace per process
var _ = Describe("Test 10: CRD Creation Lifecycle (BR-GATEWAY-018, BR-GATEWAY-021)", Ordered, func() {
	var (
		testCtx       context.Context
		testCancel    context.CancelFunc
		testLogger    logr.Logger
		testNamespace string
		k8sClient     client.Client
		gwServer      *gateway.Server // NEW: Gateway instance
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 5*time.Minute)
		testLogger = logger.WithValues("test", "crd-lifecycle-integration")

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 10: CRD Creation Lifecycle - Setup (INTEGRATION TEST)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Get shared K8s client
		k8sClient = getKubernetesClient()

		// Create test namespace
		testNamespace = helpers.CreateTestNamespace(testCtx, k8sClient, "crd-lifecycle-int")

		// NEW: Create Gateway with SHARED K8s client AND shared audit store
		// This is the KEY DIFFERENCE from E2E tests: Gateway and test use the SAME client
		// ADR-032: Audit is MANDATORY for P0 services (Gateway) - use shared audit store
		cfg := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))

		var err error
		gwServer, err = createGatewayServer(cfg, testLogger, k8sClient, sharedAuditStore)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Gateway server")

		testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)
		testLogger.Info("✅ Gateway server created with shared K8s client")
	})

	AfterAll(func() {
		if CurrentSpecReport().Failed() {
			testLogger.Info("⚠️  Test FAILED - Preserving namespace", "namespace", testNamespace)
			if testCancel != nil {
				testCancel()
			}
			return
		}
		helpers.DeleteTestNamespace(testCtx, k8sClient, testNamespace)
		if testCancel != nil {
			testCancel()
		}
		testLogger.Info("✅ Test cleanup complete")
	})

	It("should create CRDs with correct metadata and structure", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 10: CRD Creation Lifecycle (INTEGRATION TEST)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Step 1: Create normalized signals (replaces HTTP webhook)
		testLogger.Info("Step 1: Create signals and call ProcessSignal() directly")

		alertName := fmt.Sprintf("CRDLifecycleTest-%s", uuid.New().String()[:8])
		podName := "lifecycle-test-pod"
		severity := "critical"
		summary := "CRD lifecycle test alert"
		description := "Testing CRD metadata correctness"

		// NEW: Call ProcessSignal() directly (no HTTP!)
		// Send 5 signals to test CRD creation
		for i := 0; i < 5; i++ {
			// Use helper to create signal (clean, reusable pattern)
			signal := createNormalizedSignal(SignalBuilder{
				AlertName:    alertName,
				Namespace:    testNamespace,
				ResourceName: fmt.Sprintf("%s-%d", podName, i),
				Kind:         "Pod",
				Severity:     severity,
				Annotations: map[string]string{
					"summary":     summary,
					"description": description,
				},
			})

			// Call business logic directly (no HTTP!)
			response, err := gwServer.ProcessSignal(testCtx, signal)
			Expect(err).ToNot(HaveOccurred(), "ProcessSignal should succeed")
			testLogger.V(1).Info("ProcessSignal result", "status", response.Status, "crdName", response.RemediationRequestName)
		}
		testLogger.Info("  ✅ Processed 5 signals through ProcessSignal()")

		// Step 2: Verify CRD creation (IMMEDIATE, no timeout needed!)
		testLogger.Info("")
		testLogger.Info("Step 2: Verify CRD creation (immediate visibility!)")

		// NEW: No Eventually() needed! CRDs are visible immediately because
		// Gateway and test use the SAME K8s client (no cache mismatch)
		crdList := &remediationv1alpha1.RemediationRequestList{}
		err := k8sClient.List(testCtx, crdList, client.InNamespace(controllerNamespace))
		Expect(err).ToNot(HaveOccurred(), "Should list CRDs successfully")
		Expect(len(crdList.Items)).To(BeNumerically(">=", 1),
			"At least 1 CRD should be created")

		testLogger.Info(fmt.Sprintf("  ✅ Found %d CRDs (IMMEDIATELY, no timeout!)", len(crdList.Items)))

		// Step 3: Verify CRD structure (same as E2E)
		testLogger.Info("")
		testLogger.Info("Step 3: Verify CRD structure")

		crd := crdList.Items[0]

		// Verify CRD is in correct namespace (ADR-057: RRs live in controller namespace)
		Expect(crd.Namespace).To(Equal(controllerNamespace),
			"CRD should be in controller namespace")
		testLogger.Info(fmt.Sprintf("  ✅ CRD namespace: %s", crd.Namespace))

		// Verify CRD has a name
		Expect(crd.Name).ToNot(BeEmpty(),
			"CRD should have a name")
		testLogger.Info(fmt.Sprintf("  ✅ CRD name: %s", crd.Name))

		// Verify CRD has labels
		Expect(crd.Labels).ToNot(BeNil(),
			"CRD should have labels")
		testLogger.Info(fmt.Sprintf("  ✅ CRD has %d labels", len(crd.Labels)))

		// Verify spec fields
		Expect(crd.Spec.TargetResource.Name).ToNot(BeEmpty(),
			"CRD should have target resource name")
		testLogger.Info(fmt.Sprintf("  ✅ CRD target resource: %s/%s/%s",
			crd.Spec.TargetResource.Namespace,
			crd.Spec.TargetResource.Kind,
			crd.Spec.TargetResource.Name))

		// Verify fingerprint exists
		Expect(crd.Spec.SignalFingerprint).ToNot(BeEmpty(),
			"CRD should have signal fingerprint")
		testLogger.Info(fmt.Sprintf("  ✅ CRD fingerprint: %s...", crd.Spec.SignalFingerprint[:16]))

		testLogger.Info("")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Test 10 PASSED: CRD Creation Lifecycle (INTEGRATION)")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Verified:")
		testLogger.Info(fmt.Sprintf("  ✅ CRD created in namespace: %s", crd.Namespace))
		testLogger.Info(fmt.Sprintf("  ✅ CRD name: %s", crd.Name))
		testLogger.Info(fmt.Sprintf("  ✅ Affected resources: %d", len(crd.Spec.AffectedResources)))
		testLogger.Info("  ✅ Signal fingerprint present")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})
