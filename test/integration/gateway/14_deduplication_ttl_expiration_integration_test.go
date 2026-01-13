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

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 🔄 MIGRATED FROM E2E TO INTEGRATION TIER
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Migration Date: 2026-01-13
// Pattern: DD-INTEGRATION-001 v2.0 (envtest + direct business logic calls)
//
// Changes from E2E:
// ❌ REMOVED: HTTP client, gatewayURL, HTTP requests
// ✅ ADDED: Direct ProcessSignal() calls to Gateway business logic
// ✅ ADDED: Shared K8s client (suite-level) for immediate CRD visibility
// ✅ KEPT: time.Sleep(15s) for TTL expiration (time-dependent test)
//
// Test validates BR-GATEWAY-012: Deduplication TTL expiration behavior
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

package gateway

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway"

	"github.com/google/uuid"
)

// TODO: Gateway no longer uses TTL-based deduplication (as of DD-GATEWAY-011)
// Gateway switched to pure status-based deduplication using K8s CRDs
// pkg/gateway/server.go line 1497: "Redis is no longer used for deduplication state"
// This test validates a feature that no longer exists in the current architecture
// Recommendation: Keep this test in E2E tier only, or redesign for status-based deduplication
var _ = PDescribe("Test 14: Deduplication TTL Expiration (Integration)", Label("deduplication", "integration", "ttl", "pending-no-ttl-implementation"), Ordered, func() {
	var (
		testLogger    logr.Logger
		testNamespace string
		gwServer      *gateway.Server
	)

	BeforeAll(func() {
		testLogger = logger.WithValues("test", "dedup-ttl-integration")

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 14: Deduplication TTL Expiration (Integration) - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		processID := GinkgoParallelProcess()
		testNamespace = fmt.Sprintf("dedup-ttl-int-%d-%s", processID, uuid.New().String()[:8])
		testLogger.Info("Creating test namespace...", "namespace", testNamespace)

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed(), "Failed to create test namespace")

		testLogger.Info("✅ Test namespace ready", "namespace", testNamespace)

		// Create Gateway server with shared K8s client
		cfg := createGatewayConfig(getDataStorageURL())
		var err error
		gwServer, err = createGatewayServer(cfg, testLogger, k8sClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Gateway server")

		testLogger.Info("✅ Gateway server ready for direct business logic calls")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test 14: Deduplication TTL Expiration - Cleanup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if CurrentSpecReport().Failed() {
			testLogger.Info("⚠️  Test FAILED - Preserving namespace for debugging",
				"namespace", testNamespace)
		} else {
			testLogger.Info("Cleaning up test namespace...", "namespace", testNamespace)
			// Cleanup CRDs in namespace
			crdList := &remediationv1alpha1.RemediationRequestList{}
			_ = k8sClient.List(ctx, crdList, client.InNamespace(testNamespace))
			for i := range crdList.Items {
				_ = k8sClient.Delete(ctx, &crdList.Items[i])
			}
		}

		testLogger.Info("✅ Test cleanup complete")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	It("should allow new CRD creation after deduplication TTL expires (BR-GATEWAY-012)", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario: Send signal, wait for TTL, send same signal again")
		testLogger.Info("Expected: Second signal creates new CRD after TTL expiration")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		processID := GinkgoParallelProcess()
		alertName := fmt.Sprintf("TTLExpirationTest-p%d-%s", processID, uuid.New().String()[:8])

		testLogger.Info("Step 1: Send initial signal to create CRD")
		signal := createNormalizedSignal(SignalBuilder{
			AlertName:    alertName,
			Namespace:    testNamespace,
			ResourceName: "ttl-test-pod",
			Kind:         "Pod",
			Severity:     "warning",
			Source:       "prometheus",
			Labels: map[string]string{
				"test_type": "ttl-expiration",
			},
		})

		response, err := gwServer.ProcessSignal(ctx, signal)
		Expect(err).ToNot(HaveOccurred(), "Initial signal should be processed")
		Expect(response.Status).To(Equal(gateway.StatusCreated), "First signal should create new CRD")

		testLogger.Info("✅ Initial signal sent", "status", response.Status)

		testLogger.Info("Step 2: Verify initial CRD creation")
		crdList := &remediationv1alpha1.RemediationRequestList{}
		err = k8sClient.List(ctx, crdList, client.InNamespace(testNamespace))
		Expect(err).ToNot(HaveOccurred())

		initialCRDCount := 0
		for _, crd := range crdList.Items {
			if crd.Spec.SignalName == alertName {
				initialCRDCount++
			}
		}
		Expect(initialCRDCount).To(Equal(1), "Exactly 1 CRD should be created")

		testLogger.Info("✅ Initial CRD created")

		// Note: Integration environment uses default 5m TTL from Gateway config
		// For testing, we'll use a shorter wait time and rely on the fact that
		// Gateway's TTL should have expired. In production, TTL is 5 minutes.
		// This test validates TTL expiration behavior.
		testLogger.Info("Step 3: Wait for deduplication TTL to expire")
		testLogger.Info("  ⚠️  Waiting 15 seconds for TTL expiration...")
		testLogger.Info("  Note: Integration tests use Gateway's default TTL config")
		time.Sleep(15 * time.Second) // Wait for TTL expiration

		testLogger.Info("Step 4: Send SAME signal again after TTL expiration")
		// Send the exact same signal as before (same fingerprint)
		response2, err := gwServer.ProcessSignal(ctx, signal)
		Expect(err).ToNot(HaveOccurred(), "Post-TTL signal should be processed")
		// After TTL expires, Gateway should create a NEW CRD for the same fingerprint
		Expect(response2.Status).To(Equal(gateway.StatusCreated),
			"After TTL expiration, same signal should create NEW CRD (BR-GATEWAY-012)")

		testLogger.Info("✅ Post-TTL signal sent", "status", response2.Status)

		testLogger.Info("Step 5: Verify new CRD creation after TTL")
		crdList = &remediationv1alpha1.RemediationRequestList{}
		err = k8sClient.List(ctx, crdList, client.InNamespace(testNamespace))
		Expect(err).ToNot(HaveOccurred())

		totalCRDCount := 0
		for _, crd := range crdList.Items {
			if crd.Spec.SignalName == alertName {
				totalCRDCount++
			}
		}
		// After TTL expiration, we should have 2 CRDs with the same alertName
		// (one from before TTL, one created after TTL expiration)
		Expect(totalCRDCount).To(Equal(2),
			"2 CRDs should exist: original + new after TTL expiration (BR-GATEWAY-012)")

		testLogger.Info("✅ New CRD created after TTL expiration", "totalCRDs", totalCRDCount)
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Test 14 PASSED: Deduplication TTL Expiration")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})
