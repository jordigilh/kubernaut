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

package processing

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// ============================================================================
// DD-TEST-009: Field Index Smoke Test
// ============================================================================
//
// PURPOSE: DIRECTLY validate that field index setup is correct in envtest.
//
// AUTHORITATIVE REFERENCE:
//   DD-TEST-009 (docs/architecture/decisions/DD-TEST-009-FIELD-INDEX-ENVTEST-SETUP.md)
//   Section: "Smoke Test Pattern" (lines 226-268)
//
// This test DIRECTLY queries the field index to verify envtest setup.
// The existing deduplication_integration_test.go tests business logic
// (indirect field index usage through phaseChecker.ShouldDeduplicate).
//
// This smoke test provides fail-fast validation that:
// 1. Field index was registered correctly in suite_test.go
// 2. Client was retrieved AFTER SetupWithManager() (DD-TEST-009 §3)
// 3. Manager's cached client is being used (not direct client)
// 4. Field selector queries work (not silently degrading to O(n))
//
// BUSINESS VALUE:
// - DD-TEST-009 §1: Fail-fast validation at startup
// - BR-GATEWAY-185 v1.1: Field selector infrastructure validation
// - Production safety: Detect setup problems immediately
//
// FAILURE MODES DETECTED:
// - Client retrieved before SetupWithManager() called
// - Using direct client instead of manager's client
// - Manager not started before tests run
// - Field index not registered in SetupWithManager()
//
// ============================================================================

var _ = Describe("DD-TEST-009: Field Index Smoke Test (DIRECT validation)", func() {
	It("should DIRECTLY query by spec.signalFingerprint field selector", func() {
		ctx := context.Background()

		// Create temporary namespace for this test
		testNamespace := helpers.CreateTestNamespace(ctx, k8sClient, "field-index-smoke")
		defer func() {
			helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)
		}()

		// Generate valid 64-char hex fingerprint (DD-TEST-009 example uses repeated 'a')
		fingerprint := strings.Repeat("a", 64)

		// Create minimal RemediationRequest with required fields (ADR-057: RRs in controller namespace)
		now := metav1.Now()
		rr := &remediationv1alpha1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "smoke-test-",
				Namespace:    controllerNamespace,
				Generation:   1, // Required for ObservedGeneration pattern
			},
			Spec: remediationv1alpha1.RemediationRequestSpec{
				SignalFingerprint: fingerprint, // Field we're indexing
				SignalName:        "smoke-test-signal",
				Severity:          "info",
				SignalType:        "test",
				SignalSource:      "smoke-test",
				FiringTime:        now,
				ReceivedTime:      now,
				TargetType:        "kubernetes",
				TargetResource: remediationv1alpha1.ResourceIdentifier{
					Kind: "Pod",
					Name: "smoke-test-pod",
				},
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed(), "Should create test RemediationRequest")

		// ========================================
		// DD-TEST-009 SMOKE TEST: DIRECT Field Selector Query
		// ========================================
		// This is the CRITICAL test that validates field index setup.
		// Unlike deduplication_integration_test.go which calls phaseChecker.ShouldDeduplicate(),
		// this test DIRECTLY calls k8sClient.List() with MatchingFields.
		//
		// Per DD-TEST-009 §3: Register indexes FIRST, then get client
		// If this fails, it means:
		// 1. Client retrieved before SetupWithManager() called (suite_test.go order wrong)
		// 2. Using direct client instead of manager's client
		// 3. Manager not started before tests run
		// 4. Field index not registered in SetupWithManager()
		//
		// This validates the INFRASTRUCTURE, not just the business logic.
		//
		// Use Eventually to allow cache sync time (typical in envtest)
		var rrList *remediationv1alpha1.RemediationRequestList
		Eventually(func() int {
			rrList = &remediationv1alpha1.RemediationRequestList{}
			err := k8sClient.List(ctx, rrList,
				client.InNamespace(controllerNamespace),
				client.MatchingFields{"spec.signalFingerprint": fingerprint},
			)

			// ========================================
			// FAIL FAST - NO FALLBACKS (DD-TEST-009 §2)
			// ========================================
			// Per DD-TEST-009: If field selector doesn't work → Fail immediately
			// NO runtime fallbacks, NO graceful degradation
			// This ensures infrastructure problems are detected at test time, not production
			if err != nil {
				Fail("🚨 DD-TEST-009 VIOLATION: Field index setup is incorrect\n\n" +
					"Expected field selector query to work, but got error:\n" +
					"  " + err.Error() + "\n\n" +
					"Common causes (check test/integration/gateway/processing/suite_test.go):\n" +
					"  1. Client retrieved before SetupWithManager() called\n" +
					"     ❌ WRONG: k8sClient = k8sManager.GetClient(); reconciler.SetupWithManager()\n" +
					"     ✅ RIGHT: reconciler.SetupWithManager(); k8sClient = k8sManager.GetClient()\n\n" +
					"  2. Using direct client instead of manager's client\n" +
					"     ❌ WRONG: client.New(k8sConfig, ...)\n" +
					"     ✅ RIGHT: k8sManager.GetClient()\n\n" +
					"  3. Manager not started before tests run\n" +
					"     ✅ Check: go k8sManager.Start(ctx) in BeforeSuite\n\n" +
					"  4. Field index not registered in SetupWithManager()\n" +
					"     ✅ Check: mgr.GetFieldIndexer().IndexField(..., \"spec.signalFingerprint\", ...)\n\n" +
					"See: docs/architecture/decisions/DD-TEST-009-FIELD-INDEX-ENVTEST-SETUP.md")
			}

			// Return the number of items found (Eventually will retry until it equals 1)
			return len(rrList.Items)
		}, "5s", "100ms").Should(Equal(1), "Field selector query should eventually return 1 result")

		// Verify the returned RemediationRequest has correct fingerprint
		Expect(rrList.Items[0].Spec.SignalFingerprint).To(Equal(fingerprint),
			"Returned RemediationRequest should have matching fingerprint")

		suiteLogger.Info("✅ DD-TEST-009 Smoke Test PASSED: Field index infrastructure working correctly")
		suiteLogger.Info("   Field selector query successful (DIRECT validation)")
		suiteLogger.Info("   Setup order correct: Register indexes → Get client")
		suiteLogger.Info("   Using manager's cached client (not direct client)")
	})

	It("should validate field selector precision with multiple fingerprints", func() {
		ctx := context.Background()

		testNamespace := helpers.CreateTestNamespace(ctx, k8sClient, "field-precision")
		defer func() {
			helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)
		}()

		now := metav1.Now()

		// Create RRs with different fingerprints
		fingerprint1 := strings.Repeat("a", 64)
		fingerprint2 := strings.Repeat("b", 64)
		fingerprint3 := strings.Repeat("c", 64)

		for i, fp := range []string{fingerprint1, fingerprint2, fingerprint3} {
			rr := &remediationv1alpha1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "precision-test-",
				Namespace:    controllerNamespace,
			},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: fp,
					SignalName:        "test-signal",
					Severity:          "info",
					SignalType:        "test",
					SignalSource:      "smoke-test",
					FiringTime:        now,
					ReceivedTime:      now,
					TargetType:        "kubernetes",
					TargetResource: remediationv1alpha1.ResourceIdentifier{
						Kind: "Pod",
						Name: "test-pod",
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed(), "Should create RR %d", i+1)
		}

		// Wait for all to be created
		Eventually(func() int {
			list := &remediationv1alpha1.RemediationRequestList{}
			_ = k8sClient.List(ctx, list, client.InNamespace(controllerNamespace))
			return len(list.Items)
		}, "10s", "500ms").Should(Equal(3))

		// Query for fingerprint2 only - should return exactly 1 (not 3)
		// Use Eventually to allow for field index cache sync
		var rrList *remediationv1alpha1.RemediationRequestList
		Eventually(func() int {
			rrList = &remediationv1alpha1.RemediationRequestList{}
			err := k8sClient.List(ctx, rrList,
				client.InNamespace(controllerNamespace),
				client.MatchingFields{"spec.signalFingerprint": fingerprint2},
			)
			Expect(err).ToNot(HaveOccurred(),
				"Field selector query should succeed")
			return len(rrList.Items)
		}, "5s", "100ms").Should(Equal(1),
			"Field selector should return ONLY matching fingerprint (O(1) query, not O(n) filter)")

		Expect(rrList.Items[0].Spec.SignalFingerprint).To(Equal(fingerprint2),
			"Returned RR should have fingerprint2")

		suiteLogger.Info("✅ DD-TEST-009 Field Selector Precision: O(1) query verified (not O(n) in-memory)")
	})
})
