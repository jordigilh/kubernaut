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
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// ════════════════════════════════════════════════════════════════════════════
// Consecutive Failure Blocking - Unit Tests (Implementation Correctness)
// ════════════════════════════════════════════════════════════════════════════
//
// Business Requirement: BR-ORCH-042
// Purpose: Validates implementation mechanics for consecutive failure blocking
//
// Components Under Test:
// - ConsecutiveFailureBlocker: Detects and blocks consecutive failures
// - Reconciler.HandleBlockedPhase: Manages cooldown expiry
// - IsTerminalPhase: Phase classification logic
//
// Test Strategy: Table-driven tests for threshold scenarios
// ════════════════════════════════════════════════════════════════════════════

var _ = Describe("ConsecutiveFailureBlocker", func() {
	var (
		ctx              context.Context
		namespace        string
		fingerprint      string
		reconciler       *controller.Reconciler
		consecutiveBlock *controller.ConsecutiveFailureBlocker
		fakeClient       client.Client
		scheme           *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "test-consecutive-failure-" + generateRandomString(5)
		fingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" // 64 chars

		// Build scheme with all required types
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = notificationv1.AddToScheme(scheme)

		// Create fake client with field index and status subresource
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&remediationv1.RemediationRequest{}, &notificationv1.NotificationRequest{}).
			WithIndex(
				&remediationv1.RemediationRequest{},
				"spec.signalFingerprint",
				func(obj client.Object) []string {
					rr := obj.(*remediationv1.RemediationRequest)
					if rr.Spec.SignalFingerprint == "" {
						return nil
					}
					return []string{rr.Spec.SignalFingerprint}
				},
			).
			Build()

		// Create ConsecutiveFailureBlocker
		consecutiveBlock = controller.NewConsecutiveFailureBlocker(
			fakeClient,
			3,           // threshold: block after 3 consecutive failures
			1*time.Hour, // cooldown: 1 hour
			true,        // notifyOnBlock: create notification
		)

		// Create reconciler with consecutive failure blocking enabled
		// DD-METRICS-001: Metrics are required (use fresh registry per test to avoid conflicts)
		metrics := rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		reconciler = controller.NewReconciler(
			fakeClient,
			fakeClient, // apiReader (same as client for tests)
			scheme,
			nil,     // audit store
			nil,     // recorder
			metrics, // metrics (DD-METRICS-001: required)
			controller.TimeoutConfig{},
			nil, // routing engine
		)
		reconciler.SetConsecutiveFailureBlocker(consecutiveBlock)
	})

	// ════════════════════════════════════════════════════════════════════════
	// CountConsecutiveFailures Method Tests
	// ════════════════════════════════════════════════════════════════════════

	Describe("CountConsecutiveFailures", func() {
		DescribeTable("consecutive failure counting",
			func(setupFunc func(), expectedCount int, description string) {
				// Given: Setup test scenario
				setupFunc()

				// When: Count consecutive failures
				count, err := consecutiveBlock.CountConsecutiveFailures(ctx, fingerprint)

				// Then: Verify count
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(expectedCount), description)
			},
			Entry("no failures", func() {
				// No setup - empty list
			}, 0, "Should return 0 for no failures"),

			Entry("1 failure", func() {
				createFailedRR(ctx, fakeClient, namespace, fingerprint, 1)
			}, 1, "Should count single failure"),

			Entry("3 consecutive failures", func() {
				for i := 0; i < 3; i++ {
					createFailedRR(ctx, fakeClient, namespace, fingerprint, i+1)
				}
			}, 3, "Should count all consecutive failures"),

			Entry("5 consecutive failures", func() {
				for i := 0; i < 5; i++ {
					createFailedRR(ctx, fakeClient, namespace, fingerprint, i+1)
				}
			}, 5, "Should handle counts above threshold"),

			Entry("failures interrupted by Completed", func() {
				// 2 old failures, then success, then 2 recent failures
				createFailedRR(ctx, fakeClient, namespace, fingerprint, 5) // Oldest
				createFailedRR(ctx, fakeClient, namespace, fingerprint, 4)
				createCompletedRR(ctx, fakeClient, namespace, fingerprint, 3) // Reset point
				createFailedRR(ctx, fakeClient, namespace, fingerprint, 2)
				createFailedRR(ctx, fakeClient, namespace, fingerprint, 1) // Most recent
			}, 2, "Count should reset at Completed RR, only count 2 recent failures"),
		)

		Context("field selector usage", func() {
			It("should use spec.signalFingerprint not labels", func() {
				// Given: RR with fingerprint in spec but wrong label
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rr-field-selector-" + generateRandomString(5),
						Namespace: namespace,
						Labels: map[string]string{
							"kubernaut.ai/signal-fingerprint": "wrong-fingerprint-in-label",
						},
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: fingerprint, // Correct value in spec
						SignalName:        "HighCPUUsage",
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: remediationv1.PhaseFailed,
					},
				}
				Expect(fakeClient.Create(ctx, rr)).To(Succeed())

				// When: Count using field selector
				count, err := consecutiveBlock.CountConsecutiveFailures(ctx, fingerprint)

				// Then: Should find RR by spec field (ignore label mismatch)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(1), "Should use immutable spec.signalFingerprint, not mutable labels")
			})
		})

		Context("chronological ordering", func() {
			It("should count from most recent backwards", func() {
				// Given: 5 Failed RRs created at different times
				for i := 5; i >= 1; i-- {
					createFailedRR(ctx, fakeClient, namespace, fingerprint, i)
				}

				// When: Count consecutive failures
				count, err := consecutiveBlock.CountConsecutiveFailures(ctx, fingerprint)

				// Then: Should count all 5 (no Completed interruption)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(5), "Should count from most recent backwards")
			})
		})

		Context("fingerprint isolation", func() {
			It("should track different fingerprints independently", func() {
				// Given: Different fingerprints with different failure counts
				fingerprintA := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
				fingerprintB := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

				// Create 3 failures for fingerprint A
				for i := 0; i < 3; i++ {
					createFailedRR(ctx, fakeClient, namespace, fingerprintA, i+1)
				}

				// Create 1 failure for fingerprint B
				createFailedRR(ctx, fakeClient, namespace, fingerprintB, 1)

				// When: Check both fingerprints
				countA, errA := consecutiveBlock.CountConsecutiveFailures(ctx, fingerprintA)
				countB, errB := consecutiveBlock.CountConsecutiveFailures(ctx, fingerprintB)

				// Then: Each fingerprint tracked independently
				Expect(errA).ToNot(HaveOccurred())
				Expect(errB).ToNot(HaveOccurred())
				Expect(countA).To(Equal(3), "Fingerprint A should have 3 failures")
				Expect(countB).To(Equal(1), "Fingerprint B should have 1 failure")
			})
		})
	})

	// ════════════════════════════════════════════════════════════════════════
	// BlockIfNeeded Method Tests
	// ════════════════════════════════════════════════════════════════════════

	Describe("BlockIfNeeded", func() {
		DescribeTable("threshold-based blocking decisions",
			func(priorFailures int, shouldBlock bool) {
				// Given: Prior failures for this fingerprint
				for i := 0; i < priorFailures; i++ {
					createFailedRR(ctx, fakeClient, namespace, fingerprint, i+1)
				}

				// When: Check if new RR should be blocked
				newRR := createPendingRR(ctx, fakeClient, namespace, fingerprint)
				err := consecutiveBlock.BlockIfNeeded(ctx, newRR)

				// Then: Verify blocking decision
				Expect(err).ToNot(HaveOccurred())

				if shouldBlock {
					Expect(newRR.Status.OverallPhase).To(Equal(remediationv1.PhaseBlocked),
						"Should block when threshold met")
					Expect(newRR.Status.BlockedUntil).ToNot(BeNil(),
						"Should set BlockedUntil when blocking")
					Expect(newRR.Status.BlockReason).To(Equal(string(remediationv1.BlockReasonConsecutiveFailures)),
						"Should set BlockReason")

					// Verify cooldown timing (1 hour ±10 seconds)
					expectedExpiry := time.Now().Add(1 * time.Hour)
					Expect(newRR.Status.BlockedUntil.Time).To(BeTemporally("~", expectedExpiry, 10*time.Second))
				} else {
					Expect(newRR.Status.OverallPhase).ToNot(Equal(remediationv1.PhaseBlocked),
						"Should not block when below threshold")
					Expect(newRR.Status.BlockedUntil).To(BeNil(),
						"Should not set BlockedUntil when not blocking")
				}
			},
			Entry("0 prior failures - no block", 0, false),
			Entry("1 prior failure - no block", 1, false),
			Entry("2 prior failures - no block", 2, false),
			Entry("3 prior failures - BLOCK", 3, true),
			Entry("4 prior failures - BLOCK", 4, true),
			Entry("10 prior failures - BLOCK", 10, true),
		)

		Context("notification creation when blocking", func() {
			It("should create NotificationRequest with consecutive_failures_blocked type", func() {
				// Given: 3 consecutive failures (above threshold)
				for i := 0; i < 3; i++ {
					createFailedRR(ctx, fakeClient, namespace, fingerprint, i+1)
				}

				// When: Block new RR
				newRR := createPendingRR(ctx, fakeClient, namespace, fingerprint)
				err := consecutiveBlock.BlockIfNeeded(ctx, newRR)
				Expect(err).ToNot(HaveOccurred())

				// Then: Should create notification
				Expect(newRR.Status.NotificationRequestRefs).ToNot(BeEmpty())
				Expect(len(newRR.Status.NotificationRequestRefs)).To(Equal(1))

				// Verify notification exists and has correct type
				notifName := newRR.Status.NotificationRequestRefs[0].Name
				notif := &notificationv1.NotificationRequest{}
				notifKey := types.NamespacedName{Name: notifName, Namespace: namespace}
				Expect(fakeClient.Get(ctx, notifKey, notif)).To(Succeed())

				Expect(notif.Spec.Type).To(Equal(notificationv1.NotificationType("consecutive_failures_blocked")))
			})

			It("should populate notification with complete context", func() {
				// Given: 4 consecutive failures
				for i := 0; i < 4; i++ {
					createFailedRR(ctx, fakeClient, namespace, fingerprint, i+1)
				}

				// When: Block new RR
				newRR := createPendingRR(ctx, fakeClient, namespace, fingerprint)
				newRR.Spec.SignalName = "HighCPUUsage" // For notification body
				err := consecutiveBlock.BlockIfNeeded(ctx, newRR)
				Expect(err).ToNot(HaveOccurred())

				// Then: Notification should include full context
				notifName := newRR.Status.NotificationRequestRefs[0].Name
				notif := &notificationv1.NotificationRequest{}
				notifKey := types.NamespacedName{Name: notifName, Namespace: namespace}
				Expect(fakeClient.Get(ctx, notifKey, notif)).To(Succeed())

				// Verify notification context
				Expect(notif.Spec.Subject).To(ContainSubstring("HighCPUUsage"))
				Expect(notif.Spec.Body).To(ContainSubstring(fingerprint))
				Expect(notif.Spec.Body).To(ContainSubstring("4 consecutive"))
				Expect(notif.Spec.Body).To(ContainSubstring("1h0m0s")) // Go duration format
			})
		})
	})

	// ════════════════════════════════════════════════════════════════════════
	// HandleBlockedPhase Method Tests
	// ════════════════════════════════════════════════════════════════════════

	Describe("Reconciler.HandleBlockedPhase", func() {
		DescribeTable("cooldown expiry behavior",
			func(blockedUntil *metav1.Time, shouldTransition bool, expectedPhase remediationv1.RemediationPhase) {
				// Given: Blocked RR with specific expiry time
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rr-blocked-" + generateRandomString(5),
						Namespace: namespace,
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: fingerprint,
						SignalName:        "HighCPUUsage",
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: remediationv1.PhaseBlocked,
						BlockedUntil: blockedUntil,
						BlockReason:  string(remediationv1.BlockReasonConsecutiveFailures),
					},
				}
				Expect(fakeClient.Create(ctx, rr)).To(Succeed())

				// When: Handle Blocked phase
				result, err := reconciler.HandleBlockedPhase(ctx, rr)

				// Then: Verify outcome
				Expect(err).ToNot(HaveOccurred())

				if shouldTransition {
					Expect(result.RequeueAfter).To(BeZero(), "Should not requeue when transitioning to terminal phase")

					// Verify phase transitioned to Failed
					updatedRR := &remediationv1.RemediationRequest{}
					key := types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}
					Expect(fakeClient.Get(ctx, key, updatedRR)).To(Succeed())
					Expect(updatedRR.Status.OverallPhase).To(Equal(expectedPhase))
				} else {
					// Should requeue at expiry time
					Expect(result.RequeueAfter).To(BeNumerically(">", 0))

					// Phase should remain Blocked
					updatedRR := &remediationv1.RemediationRequest{}
					key := types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}
					Expect(fakeClient.Get(ctx, key, updatedRR)).To(Succeed())
					Expect(updatedRR.Status.OverallPhase).To(Equal(remediationv1.PhaseBlocked))
				}
			},
			Entry("expired cooldown (-5 min) - transition to Failed",
				&metav1.Time{Time: time.Now().Add(-5 * time.Minute)}, true, remediationv1.PhaseFailed),
			Entry("active cooldown (+30 min) - stay Blocked",
				&metav1.Time{Time: time.Now().Add(30 * time.Minute)}, false, remediationv1.PhaseBlocked),
			Entry("active cooldown (+1 hour) - stay Blocked",
				&metav1.Time{Time: time.Now().Add(1 * time.Hour)}, false, remediationv1.PhaseBlocked),
		)

		Context("requeue timing precision", func() {
			It("should calculate exact requeue duration for future expiry", func() {
				// Given: Blocked RR expiring in exactly 15 minutes
				expiryTime := metav1.NewTime(time.Now().Add(15 * time.Minute))
				rr := createBlockedRR(ctx, fakeClient, namespace, fingerprint, &expiryTime)

				// When: Handle Blocked phase
				result, err := reconciler.HandleBlockedPhase(ctx, rr)

				// Then: Should requeue at expiry time (±10 seconds tolerance)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically("~", 15*time.Minute, 10*time.Second))
			})
		})

		Context("manual block handling", func() {
			It("should not auto-expire when BlockedUntil is nil", func() {
				// Given: Manually blocked RR (no BlockedUntil set)
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rr-manual-block-" + generateRandomString(5),
						Namespace: namespace,
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: fingerprint,
						SignalName:        "HighCPUUsage",
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: remediationv1.PhaseBlocked,
						BlockReason:  "manual_block", // Manual block reason (not one of the standard constants)
						// No BlockedUntil - manual intervention required
					},
				}
				Expect(fakeClient.Create(ctx, rr)).To(Succeed())

				// When: Handle manually blocked RR
				result, err := reconciler.HandleBlockedPhase(ctx, rr)

				// Then: Should stay in Blocked without requeue
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero(), "Manual block should not auto-requeue")

				// Phase should remain Blocked
				updatedRR := &remediationv1.RemediationRequest{}
				key := types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}
				Expect(fakeClient.Get(ctx, key, updatedRR)).To(Succeed())
				Expect(updatedRR.Status.OverallPhase).To(Equal(remediationv1.PhaseBlocked))
			})
		})
	})

	// ════════════════════════════════════════════════════════════════════════
	// IsTerminalPhase Function Tests
	// ════════════════════════════════════════════════════════════════════════

	Describe("IsTerminalPhase", func() {
		DescribeTable("phase classification",
			func(phase remediationv1.RemediationPhase, isTerminal bool, reasoning string) {
				// When: Check if phase is terminal
				result := controller.IsTerminalPhase(phase)

				// Then: Should match expected classification
				Expect(result).To(Equal(isTerminal), reasoning)
			},
			Entry("Blocked is non-terminal", remediationv1.PhaseBlocked, false,
				"Blocked should be non-terminal to prevent Gateway from creating new RRs"),
			Entry("Failed is terminal", remediationv1.PhaseFailed, true,
				"Failed allows Gateway to create new RRs after cooldown"),
			Entry("Completed is terminal", remediationv1.PhaseCompleted, true,
				"Completed allows Gateway to create new RRs"),
			Entry("TimedOut is terminal", remediationv1.PhaseTimedOut, true,
				"TimedOut allows Gateway to create new RRs"),
			Entry("Pending is non-terminal", remediationv1.PhasePending, false,
				"Pending is active state"),
			Entry("Processing is non-terminal", remediationv1.PhaseProcessing, false,
				"Processing is active state"),
			Entry("Analyzing is non-terminal", remediationv1.PhaseAnalyzing, false,
				"Analyzing is active state"),
		)
	})
})

// ════════════════════════════════════════════════════════════════════════════
// Helper Functions - Test Data Creation
// ════════════════════════════════════════════════════════════════════════════

// createFailedRR creates a RemediationRequest in Failed phase.
// minutesAgo specifies how many minutes ago it was created (for chronological ordering).
func createFailedRR(ctx context.Context, c client.Client, namespace, fingerprint string, minutesAgo int) {
	rr := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "rr-failed-" + generateRandomString(5),
			Namespace:         namespace,
			CreationTimestamp: metav1.NewTime(time.Now().Add(time.Duration(-minutesAgo) * time.Minute)),
		},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: fingerprint,
			SignalName:        "TestSignal",
		},
		Status: remediationv1.RemediationRequestStatus{
			OverallPhase: remediationv1.PhaseFailed,
		},
	}
	Expect(c.Create(ctx, rr)).To(Succeed())
}

// createCompletedRR creates a RemediationRequest in Completed phase.
// minutesAgo specifies how many minutes ago it was created (for chronological ordering).
func createCompletedRR(ctx context.Context, c client.Client, namespace, fingerprint string, minutesAgo int) {
	rr := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "rr-completed-" + generateRandomString(5),
			Namespace:         namespace,
			CreationTimestamp: metav1.NewTime(time.Now().Add(time.Duration(-minutesAgo) * time.Minute)),
		},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: fingerprint,
			SignalName:        "TestSignal",
		},
		Status: remediationv1.RemediationRequestStatus{
			OverallPhase: remediationv1.PhaseCompleted,
			Outcome:      "Success",
		},
	}
	Expect(c.Create(ctx, rr)).To(Succeed())
}

// createPendingRR creates a RemediationRequest in Pending phase.
func createPendingRR(ctx context.Context, c client.Client, namespace, fingerprint string) *remediationv1.RemediationRequest {
	rr := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rr-pending-" + generateRandomString(5),
			Namespace: namespace,
		},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: fingerprint,
			SignalName:        "TestSignal",
		},
	}
	Expect(c.Create(ctx, rr)).To(Succeed())
	return rr
}

// createBlockedRR creates a RemediationRequest in Blocked phase with specified expiry.
func createBlockedRR(ctx context.Context, c client.Client, namespace, fingerprint string, blockedUntil *metav1.Time) *remediationv1.RemediationRequest {
	rr := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rr-blocked-" + generateRandomString(5),
			Namespace: namespace,
		},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: fingerprint,
			SignalName:        "TestSignal",
		},
		Status: remediationv1.RemediationRequestStatus{
			OverallPhase: remediationv1.PhaseBlocked,
			BlockedUntil: blockedUntil,
			BlockReason:  string(remediationv1.BlockReasonConsecutiveFailures),
		},
	}
	Expect(c.Create(ctx, rr)).To(Succeed())
	return rr
}

// stringPtr returns a pointer to a string.
func stringPtr(s string) *string { //nolint:unused
	return &s
}

// generateRandomString generates a random string of specified length.
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
