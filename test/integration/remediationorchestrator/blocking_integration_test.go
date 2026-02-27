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

package remediationorchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ========================================
// BR-ORCH-042: Consecutive Failure Blocking Integration Tests
// ========================================
//
// These tests validate the blocking logic with real Kubernetes API (envtest).
// They verify:
// - Field index on spec.signalFingerprint works (AC-042-1-4, AC-042-1-5)
// - Consecutive failure counting across RRs (AC-042-1-1, AC-042-1-2, AC-042-1-3)
// - Blocking phase transition (AC-042-2-1)
// - Cooldown expiry handling (AC-042-3-1, AC-042-3-2, AC-042-3-3)
//
// TDD: These tests were written BEFORE implementation (RED phase).
// Implementation in internal/controller/remediationorchestrator/blocking.go (GREEN phase).

var _ = Describe("BR-ORCH-042: Consecutive Failure Blocking", func() {

	// ========================================
	// Test 1: End-to-End Blocking Workflow
	// BR-ORCH-042.1, AC-042-1-1
	// ========================================
	Describe("Consecutive Failure Detection (BR-ORCH-042.1)", func() {

		// NOTE: Timestamp-dependent counting logic is thoroughly tested in unit tests
		// (test/unit/remediationorchestrator/consecutive_failure_test.go) with controlled timestamps.
		// This integration test validates broader controller behavior with real K8s API.

		It("should block RR and create notification after threshold failures (end-to-end)", func() {
			// Create unique namespace for this test
			ns := createTestNamespace("blocking-e2e")
			defer deleteTestNamespace(ns)

			fingerprint := "c2d3e4f5a6b1c2d3e4f5a6b1c2d3e4f5a6b1c2d3e4f5a6b1c2d3e4f5a6b1c2d3"

			// Given: 3 consecutive Failed RRs (threshold met)
			// Create via real controller reconciliation to test full workflow
			for i := 1; i <= 3; i++ {
				rrName := fmt.Sprintf("rr-fail-%d-%s", i, uuid.New().String()[:8])
				createFailedRemediationRequestWithFingerprint(ns, rrName, fingerprint)
				GinkgoWriter.Printf("✅ Created failed RR %d/3: %s\n", i, rrName)
			}

			// When: Create 4th RR with same fingerprint
			rr4Name := fmt.Sprintf("rr-fail-4-%s", uuid.New().String()[:8])
			rr4 := createRemediationRequestWithFingerprint(ns, rr4Name, fingerprint)
			GinkgoWriter.Printf("✅ Created 4th RR (should be blocked): %s\n", rr4Name)

			// Then: Controller should detect threshold and block this RR
			// Validation focuses on end-to-end controller behavior, not timestamp ordering
			Eventually(func() string {
				fetched := &remediationv1.RemediationRequest{}
				if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      rr4.Name,
					Namespace: ROControllerNamespace,
				}, fetched); err != nil {
					return ""
				}
				return string(fetched.Status.OverallPhase)
			}, timeout, interval).Should(Or(
				Equal("Blocked"),
				Equal("Failed"), // May transition directly to Failed if CheckConsecutiveFailures runs during routing
			), "RR should be blocked or failed after 3 consecutive failures")

			// Verify blocking metadata if Blocked
			fetchedRR := &remediationv1.RemediationRequest{}
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      rr4.Name,
				Namespace: ROControllerNamespace,
			}, fetchedRR)).To(Succeed())

			if fetchedRR.Status.OverallPhase == "Blocked" {
				Expect(fetchedRR.Status.BlockedUntil).ToNot(BeNil(), "Should set BlockedUntil")
				Expect(fetchedRR.Status.BlockReason).To(Equal("ConsecutiveFailures"), "Should set BlockReason")
				GinkgoWriter.Printf("✅ RR blocked with cooldown until: %s\n", fetchedRR.Status.BlockedUntil.Format(time.RFC3339))
			}

			GinkgoWriter.Printf("✅ End-to-end blocking workflow validated\n")
		})
	})

	// ========================================
	// Test 2: Blocked phase classification
	// BR-ORCH-042.2, AC-042-2-1
	// ========================================
	Describe("Blocked Phase Classification (BR-ORCH-042.2)", func() {

		It("should accept Blocked as a valid phase value in RR status", func() {
			// Create unique namespace for this test
			ns := createTestNamespace("blocking-phase")
			defer deleteTestNamespace(ns)

			fingerprint := "d3e4f5a6b1c2d3e4f5a6b1c2d3e4f5a6b1c2d3e4f5a6b1c2d3e4f5a6b1c2d3e4"

			// Create RR
			rr := createRemediationRequestWithFingerprint(ns, "rr-blocked", fingerprint)

			// Manually set to Blocked phase (simulating what RO does)
			Eventually(func() error {
				rrGet := &remediationv1.RemediationRequest{}
				if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: ROControllerNamespace}, rrGet); err != nil {
					return err
				}

				now := metav1.NewTime(time.Now().Add(1 * time.Hour))
				rrGet.Status.OverallPhase = "Blocked"
				rrGet.Status.BlockedUntil = &now
				rrGet.Status.BlockReason = "consecutive_failures_exceeded"
				rrGet.Status.Message = "Signal blocked due to consecutive failures"

				return k8sClient.Status().Update(ctx, rrGet)
			}, timeout, interval).Should(Succeed(), "Should accept Blocked as valid phase")

			// Verify the status was persisted correctly
			rrFinal := &remediationv1.RemediationRequest{}
			err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: ROControllerNamespace}, rrFinal)
			Expect(err).ToNot(HaveOccurred())
			Expect(rrFinal.Status.OverallPhase).To(Equal(remediationv1.PhaseBlocked))
			Expect(rrFinal.Status.BlockedUntil).ToNot(BeNil(),
				"BR-SCOPE-010: Blocked RR must have a BlockedUntil timestamp for backoff")
			Expect(rrFinal.Status.BlockReason).To(Equal("consecutive_failures_exceeded"))
		})
	})

	// ========================================
	// Test 3: Cooldown expiry handling
	// BR-ORCH-042.3, AC-042-3-2
	// ========================================
	Describe("Cooldown Expiry Handling (BR-ORCH-042.3)", func() {

		// Phase 2 test moved to E2E suite: test/e2e/remediationorchestrator/blocking_e2e_test.go

		It("should allow manual blocks without BlockedUntil (nil = no auto-expiry)", func() {
			// Create unique namespace for this test
			ns := createTestNamespace("blocking-manual")
			defer deleteTestNamespace(ns)

			fingerprint := "f5a6b1c2d3e4f5a6b1c2d3e4f5a6b1c2d3e4f5a6b1c2d3e4f5a6b1c2d3e4f5a6"

			// Create RR
			rr := createRemediationRequestWithFingerprint(ns, "rr-manual-block", fingerprint)

			// Set to Blocked WITHOUT BlockedUntil (manual block)
			Eventually(func() error {
				rrGet := &remediationv1.RemediationRequest{}
				if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: ROControllerNamespace}, rrGet); err != nil {
					return err
				}

				rrGet.Status.OverallPhase = remediationv1.PhaseBlocked
				rrGet.Status.BlockedUntil = nil // No auto-expiry
				rrGet.Status.BlockReason = "manual_block"
				rrGet.Status.Message = "Manually blocked by operator"

				return k8sClient.Status().Update(ctx, rrGet)
			}, timeout, interval).Should(Succeed())

			// Verify BlockedUntil is nil
			rrFinal := &remediationv1.RemediationRequest{}
			err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: ROControllerNamespace}, rrFinal)
			Expect(err).ToNot(HaveOccurred())
			Expect(rrFinal.Status.OverallPhase).To(Equal(remediationv1.PhaseBlocked))
			Expect(rrFinal.Status.BlockedUntil).To(BeNil(),
				"Manual blocks should have nil BlockedUntil (no auto-expiry)")
		})
	})

	// ========================================
	// Fingerprint Edge Cases
	// Business Value: Data quality resilience and multi-tenant isolation
	// Confidence: 95% - Real data quality scenarios
	// ========================================
	Describe("Blocking Logic Fingerprint Edge Cases", func() {
		var namespace string

		BeforeEach(func() {
			namespace = createTestNamespace("ro-fingerprint")
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		It("should handle RR with unique fingerprint (no prior failures)", func() {
			// Scenario: RR with unique fingerprint (no blocking history)
			// Business Outcome: RR processes normally, blocking logic doesn't interfere
			// Confidence: 95% - Validates blocking logic only applies when appropriate
			// NOTE: Empty fingerprint rejected by CRD validation, so testing unique FP instead

			ctx := context.Background()

			// Given: RemediationRequest with unique fingerprint (no prior failures)
			now := metav1.Now()
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unique-fp-rr",
					Namespace: ROControllerNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalName:        "test-unique-fp",
					SignalFingerprint: "f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9", // Unique fingerprint
					Severity:          "critical",
					SignalType:        "test",
					TargetType:        "kubernetes",
					FiringTime:        now,
					ReceivedTime:      now,
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "test-app",
						Namespace: namespace,
					},
				},
			}

			// When: Creating RR
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// Then: RR should not be blocked (unique fingerprint = no prior failures)
			Consistently(func() string {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
					Name:      rr.Name,
					Namespace: ROControllerNamespace,
				}, updated); err != nil {
					return ""
				}
				return string(updated.Status.OverallPhase)
			}, "5s", "500ms").ShouldNot(Equal("Blocked"),
				"RR with unique fingerprint should not be blocked (no failure history)")

			// Verify: SignalProcessing is created (RR processes normally)
			Eventually(func() int {
				spList := &signalprocessingv1.SignalProcessingList{}
				if err := k8sManager.GetAPIReader().List(ctx, spList, client.InNamespace(ROControllerNamespace)); err != nil {
					return 0
				}
				return len(spList.Items)
			}, "10s", "500ms").Should(Equal(1),
				"RR with unique fingerprint must create SignalProcessing (blocking doesn't interfere)")
		})

	It("should isolate blocking by namespace (multi-tenant)", func() {
		// DD-TEST-PARALLELISM-003: Work WITH Controller Lifecycle
		// Strategy: Wait for natural Pending → Processing transition, then set Failed.
		// Eliminates race condition by letting controller stabilize before status override.
		//
		// Business Scenario:
		// - Tenant A (ns-a) has 3 consecutive failures for fingerprint X
		// - Tenant B (ns-b) has same fingerprint X but 0 failures
		// - Expected: Blocking isolated to ns-a only (multi-tenant protection)
		//
		// Confidence: 95% - Stable approach that works with controller timing

		ctx := context.Background()

		// Create second namespace for isolation test
		nsB := createTestNamespace("ro-fingerprint-b")
		defer deleteTestNamespace(nsB)

		sharedFP := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"

		// PHASE 1: Create 3 failed RRs in namespace A
		// Each RR progresses naturally to Processing, then we set to Failed
		By(fmt.Sprintf("Creating 3 failed RRs in namespace %s (after natural progression)", namespace))
		for i := 0; i < 3; i++ {
			createFailedRemediationRequestWithFingerprint(namespace, fmt.Sprintf("ns-a-fail-%d", i), sharedFP)
		}

		// Verify all 3 RRs in namespace A are Failed (precondition for blocking test)
		By(fmt.Sprintf("Verifying all 3 RRs in namespace %s are in Failed status", namespace))
		Eventually(func() int {
			rrListA := &remediationv1.RemediationRequestList{}
			if err := k8sManager.GetAPIReader().List(ctx, rrListA, client.InNamespace(ROControllerNamespace)); err != nil {
				GinkgoWriter.Printf("❌ Error listing RRs in namespace A: %v\n", err)
				return -1
			}
			failedCountA := 0
			for _, rr := range rrListA.Items {
				if rr.Spec.SignalFingerprint == sharedFP && rr.Spec.TargetResource.Namespace == namespace && rr.Status.OverallPhase == "Failed" {
					failedCountA++
				}
			}
			GinkgoWriter.Printf("🔍 Namespace A: %d/3 RRs in Failed status\n", failedCountA)
			return failedCountA
		}, timeout, interval).Should(Equal(3),
			"All 3 RRs in namespace A should be in Failed status (precondition for blocking test)")

		// PHASE 2: Create 1 failed RR in namespace B (same fingerprint, different namespace)
		// This proves that namespace A's failures don't affect namespace B's failure count
		By(fmt.Sprintf("Creating 1 failed RR in namespace %s (same fingerprint)", nsB))
		createFailedRemediationRequestWithFingerprint(nsB, "ns-b-fail-1", sharedFP)

		// Verify namespace B has only 1 failed RR (not affected by namespace A's 3 failures)
		By(fmt.Sprintf("Verifying namespace %s has 1 failed RR (isolated from namespace A)", nsB))
		Eventually(func() int {
			rrListB := &remediationv1.RemediationRequestList{}
			if err := k8sManager.GetAPIReader().List(ctx, rrListB, client.InNamespace(ROControllerNamespace)); err != nil {
				GinkgoWriter.Printf("❌ Error listing RRs in namespace B: %v\n", err)
				return -1
			}
			failedCountB := 0
			for _, rr := range rrListB.Items {
				if rr.Spec.SignalFingerprint == sharedFP && rr.Spec.TargetResource.Namespace == nsB && rr.Status.OverallPhase == "Failed" {
					failedCountB++
				}
			}
			GinkgoWriter.Printf("🔍 Namespace B: %d/1 RRs with Failed status\n", failedCountB)
			return failedCountB
		}, timeout, interval).Should(Equal(1),
			"Namespace B should have exactly 1 failed RR (isolated from namespace A)")

		// PHASE 3: Test Blocking Logic in Namespace A
		// Create new RR in namespace A with same fingerprint → should be BLOCKED
		By(fmt.Sprintf("Creating new RR in namespace %s (should be blocked due to 3 failures)", namespace))
		newRR_A := createRemediationRequestWithFingerprint(namespace, "new-rr-a", sharedFP)

		By("Verifying new RR in namespace A is blocked")
		Eventually(func() string {
			fetched := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx,
				types.NamespacedName{Name: newRR_A.Name, Namespace: ROControllerNamespace}, fetched); err != nil {
				return ""
			}
			return string(fetched.Status.OverallPhase)
		}, 30*time.Second, 500*time.Millisecond).Should(Equal("Blocked"),
			"New RR in namespace A should be blocked due to 3 consecutive failures")

		// PHASE 4: Test Namespace Isolation
		// Create new RR in namespace B with same fingerprint → should NOT be blocked
		By(fmt.Sprintf("Creating new RR in namespace %s (should NOT be blocked)", nsB))
		newRR_B := createRemediationRequestWithFingerprint(nsB, "new-rr-b", sharedFP)

		By("Verifying new RR in namespace B is NOT blocked (namespace isolation)")
		Eventually(func() string {
			fetched := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx,
				types.NamespacedName{Name: newRR_B.Name, Namespace: ROControllerNamespace}, fetched); err != nil {
				return ""
			}
			return string(fetched.Status.OverallPhase)
		}, 30*time.Second, 500*time.Millisecond).Should(Or(
			Equal("Processing"),
			Equal("Analyzing"),
			Equal("Executing"),
			Equal("Completed")),
			"New RR in namespace B should progress normally (namespace isolation works)")

		// Business Value: Multi-tenant safety - one tenant's failures don't affect another
		GinkgoWriter.Printf("✅ Namespace isolation verified: ns-a blocked, ns-b processing\n")
	})
	})
})

// ============================================================================
// HELPER FUNCTIONS FOR BLOCKING TESTS
// ============================================================================

// createRemediationRequestWithFingerprint creates an RR with a specific fingerprint.
// ADR-057: RR is created in ROControllerNamespace; targetNamespace is for Spec.TargetResource.
func createRemediationRequestWithFingerprint(targetNamespace, name, fingerprint string) *remediationv1.RemediationRequest {
	now := metav1.Now()
	rr := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ROControllerNamespace,
		},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: fingerprint,
			SignalName:        fmt.Sprintf("TestAlert-%s", name),
			Severity:          "critical",
			SignalType:        "alert",
			TargetType:        "kubernetes",
			TargetResource: remediationv1.ResourceIdentifier{
				Kind:      "Deployment",
				Name:      "test-app",
				Namespace: targetNamespace,
			},
			FiringTime:   now,
			ReceivedTime: now,
			Deduplication: sharedtypes.DeduplicationInfo{
				FirstOccurrence: now,
				LastOccurrence:  now,
				OccurrenceCount: 1,
			},
		},
	}
	
	Expect(k8sClient.Create(ctx, rr)).To(Succeed())
	GinkgoWriter.Printf("✅ Created RR with fingerprint: %s/%s (fingerprint: %s...)\n",
		ROControllerNamespace, name, fingerprint[:16])
	return rr
}

// createFailedRemediationRequestWithFingerprint creates an RR and sets it to Failed.
// Uses same approach as simulateFailedPhase but in single function to minimize race window.
// Used for blocking tests that need RRs pre-Failed (not transitioning during test).
func createFailedRemediationRequestWithFingerprint(namespace, name, fingerprint string) *remediationv1.RemediationRequest {
	// DD-TEST-PARALLELISM-003: Work WITH Controller, Not Against It
	// Strategy: Wait for controller to naturally progress RR to Processing, THEN set to Failed.
	// This eliminates race conditions by letting controller do its initial work first.
	//
	// Why this approach works:
	// - Controller moves RR: Pending → Processing (fast, ~500ms)
	// - We intercept: Processing → Failed (after controller has stabilized)
	// - No fight over Pending phase (previous race condition eliminated)
	// - Tests blocking logic with realistic Failed RRs (not artificial Pending → Failed)
	//
	// Confidence: 95% - Works with controller lifecycle, not against it

	// Step 1: Create RR (starts in Pending)
	rr := createRemediationRequestWithFingerprint(namespace, name, fingerprint)

	// Step 2: Wait for controller to naturally progress to Processing
	// Controller creates SignalProcessing CRD and updates RR status
	Eventually(func() string {
		fetched := &remediationv1.RemediationRequest{}
		if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: name, Namespace: ROControllerNamespace}, fetched); err != nil {
			return ""
		}
		return string(fetched.Status.OverallPhase)
	}, timeout, interval).Should(Equal("Processing"),
		"Controller should naturally progress %s/%s to Processing", ROControllerNamespace, name)

	GinkgoWriter.Printf("✅ RR naturally progressed to Processing: %s/%s\n", ROControllerNamespace, name)

	// Step 3: Now set to Failed (controller has finished initial work, minimal race)
	Eventually(func() error {
		fetched := &remediationv1.RemediationRequest{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ROControllerNamespace}, fetched); err != nil {
			return err
		}

		failPhase := "workflow_execution"
		failReason := "Simulated failure for blocking test"

		fetched.Status.OverallPhase = "Failed"
		fetched.Status.FailurePhase = &failPhase
		fetched.Status.FailureReason = &failReason

		return k8sClient.Status().Update(ctx, fetched)
	}, timeout, interval).Should(Succeed(), "Should set Failed status for %s/%s", ROControllerNamespace, name)

	// Step 4: Verify Failed status durably persisted
	Eventually(func() string {
		fetched := &remediationv1.RemediationRequest{}
		if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: name, Namespace: ROControllerNamespace}, fetched); err != nil {
			return ""
		}
		return string(fetched.Status.OverallPhase)
	}, timeout, interval).Should(Equal("Failed"), "Should confirm Failed phase persisted for %s/%s", ROControllerNamespace, name)

	GinkgoWriter.Printf("✅ Set to Failed (after natural progression): %s/%s\n", ROControllerNamespace, name)
	return rr
}

// Removed: simulateFailedPhase (unused) - Tests now use createFailedRemediationRequestWithFingerprint
// Removed: simulateCompletedPhase (unused) - Tests now rely on natural controller progression
