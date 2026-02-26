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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
)

// ============================================================================
// BUSINESS OUTCOME TESTS: Phase-Based Deduplication (DD-GATEWAY-011)
// ============================================================================
//
// BR-GATEWAY-181: Deduplication prevents wasteful duplicate remediations
//
// BUSINESS VALUE:
// - Prevents wasted compute on duplicate alerts
// - Single remediation per issue occurrence
// - Duplicate signals update status (occurrence tracking)
// ============================================================================

var _ = Describe("BR-GATEWAY-181: Terminal Phase Classification determines deduplication behavior", func() {
	Context("Terminal phases allow new RemediationRequest creation", func() {
		It("classifies Completed as terminal - allows retry for recurring issues", func() {
			// BUSINESS OUTCOME: After successful remediation, new alert = new issue
			isTerminal := processing.IsTerminalPhase(remediationv1alpha1.PhaseCompleted)

			Expect(isTerminal).To(BeTrue(),
				"Completed = terminal → recurring issues get new remediation")
		})

		It("classifies Failed as terminal - allows retry attempts", func() {
			// BUSINESS OUTCOME: Failed remediation can be retried with new approach
			isTerminal := processing.IsTerminalPhase(remediationv1alpha1.PhaseFailed)

			Expect(isTerminal).To(BeTrue(),
				"Failed = terminal → allows new remediation attempt")
		})

		It("classifies TimedOut as terminal - allows retry", func() {
			// BUSINESS OUTCOME: Timed out remediation can be retried
			isTerminal := processing.IsTerminalPhase(remediationv1alpha1.PhaseTimedOut)

			Expect(isTerminal).To(BeTrue(),
				"TimedOut = terminal → allows retry with fresh remediation")
		})
	})

	Context("Non-terminal phases trigger deduplication", func() {
		It("classifies Pending as non-terminal - duplicates update status", func() {
			// BUSINESS OUTCOME: Alert arrives while RR pending → update occurrence count
			isTerminal := processing.IsTerminalPhase(remediationv1alpha1.PhasePending)

			Expect(isTerminal).To(BeFalse(),
				"Pending = non-terminal → duplicate updates occurrence count")
		})

		It("classifies Processing as non-terminal - remediation in progress", func() {
			// BUSINESS OUTCOME: Alert during active remediation → skip duplicate
			isTerminal := processing.IsTerminalPhase(remediationv1alpha1.PhaseProcessing)

			Expect(isTerminal).To(BeFalse(),
				"Processing = non-terminal → no duplicate remediation started")
		})

		It("classifies Blocked as non-terminal - RO manages cooldown", func() {
			// BUSINESS OUTCOME: Blocked RR = RO holding for cooldown
			isTerminal := processing.IsTerminalPhase(remediationv1alpha1.PhaseBlocked)

			Expect(isTerminal).To(BeFalse(),
				"Blocked = non-terminal → RO owns cooldown logic")
		})
	})
})

var _ = Describe("BR-GATEWAY-181: Phase Checker initialization for Gateway", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		scheme    *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		_ = ctx // Used in actual tests
		scheme = runtime.NewScheme()
		Expect(remediationv1alpha1.AddToScheme(scheme)).To(Succeed())

		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&remediationv1alpha1.RemediationRequest{}).
			Build()
	})

	It("creates phase checker for Gateway startup", func() {
		// BUSINESS OUTCOME: Gateway can instantiate deduplication checker
		checker := processing.NewPhaseBasedDeduplicationChecker(k8sClient, 5*time.Minute)

		Expect(checker).NotTo(BeNil(),
			"Phase checker created for Gateway deduplication decisions")
	})
})

// ============================================================================
// ISSUE #195: Gateway deduplication must search controller namespace (ADR-057)
// ============================================================================
//
// BUG: After ADR-057, all RRs live in the controller namespace (kubernaut-system),
// but ShouldDeduplicate was called with signal.Namespace (workload namespace).
// This caused deduplication to always miss existing RRs.
//
// BUSINESS VALUE:
// - Deduplication prevents duplicate remediations for the same signal
// - After ADR-057 namespace consolidation, dedup must search the correct namespace
// ============================================================================

var _ = Describe("Issue #195: Deduplication must search controller namespace, not signal namespace", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		scheme    *runtime.Scheme
		checker   *processing.PhaseBasedDeduplicationChecker
	)

	const (
		controllerNS = "kubernaut-system"
		workloadNS   = "demo-taint"
		fingerprint  = "a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456"
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(remediationv1alpha1.AddToScheme(scheme)).To(Succeed())

		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&remediationv1alpha1.RemediationRequest{}).
			WithIndex(&remediationv1alpha1.RemediationRequest{}, "spec.signalFingerprint", func(o client.Object) []string {
				rr := o.(*remediationv1alpha1.RemediationRequest)
				return []string{rr.Spec.SignalFingerprint}
			}).
			Build()

		checker = processing.NewPhaseBasedDeduplicationChecker(k8sClient, 0)

		// Create an active (non-terminal) RR in the controller namespace — this is where
		// ADR-057 mandates all CRDs live after namespace consolidation
		rr := &remediationv1alpha1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-dedup-195",
				Namespace: controllerNS,
			},
			Spec: remediationv1alpha1.RemediationRequestSpec{
				SignalFingerprint: fingerprint,
			},
			Status: remediationv1alpha1.RemediationRequestStatus{
				OverallPhase: remediationv1alpha1.PhaseProcessing,
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())
	})

	// UT-GW-195-001: This test DEMONSTRATES the bug — querying workload NS misses the RR
	It("UT-GW-195-001: should NOT find active RR when searching workload namespace (bug reproduction)", func() {
		// BUG: Before fix, Gateway passed signal.Namespace ("demo-taint") to ShouldDeduplicate.
		// The RR lives in "kubernaut-system", so the query returns nothing — dedup is broken.
		shouldDedup, existingRR, err := checker.ShouldDeduplicate(ctx, workloadNS, fingerprint)
		Expect(err).NotTo(HaveOccurred())

		// This PASSES (proving the bug exists): searching the wrong namespace finds nothing
		Expect(shouldDedup).To(BeFalse(),
			"Bug #195: Searching workload namespace misses the RR in controller namespace")
		Expect(existingRR).To(BeNil())
	})

	// UT-GW-195-002: This test validates the CORRECT behavior — querying controller NS finds the RR
	It("UT-GW-195-002: should find active RR when searching controller namespace (correct behavior)", func() {
		// CORRECT: After fix, Gateway must pass controllerNamespace to ShouldDeduplicate
		shouldDedup, existingRR, err := checker.ShouldDeduplicate(ctx, controllerNS, fingerprint)
		Expect(err).NotTo(HaveOccurred())

		Expect(shouldDedup).To(BeTrue(),
			"Issue #195: Deduplication must find existing RR in controller namespace")
		Expect(existingRR).NotTo(BeNil())
		Expect(existingRR.Name).To(Equal("rr-dedup-195"))
		Expect(existingRR.Namespace).To(Equal(controllerNS),
			"Issue #195: Existing RR lives in controller namespace per ADR-057")
	})
})

// ============================================================================
// BUSINESS OUTCOME TESTS: Status Updater for Occurrence Tracking
// ============================================================================
//
// BR-GATEWAY-182: Track duplicate signal occurrences in CRD status
//
// BUSINESS VALUE:
// - Operators see how many duplicate signals arrived
// - Storm detection metrics based on occurrence count
// - Audit trail of signal frequency
// ============================================================================

var _ = Describe("BR-GATEWAY-182: Status Updater tracks duplicate signal occurrences", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		scheme    *runtime.Scheme
		updater   *processing.StatusUpdater
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(remediationv1alpha1.AddToScheme(scheme)).To(Succeed())

		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&remediationv1alpha1.RemediationRequest{}).
			Build()

		// DD-STATUS-001: Pass k8sClient as both client and apiReader (tests use fake client, already uncached)
		updater = processing.NewStatusUpdater(k8sClient, k8sClient)
	})

	Context("Deduplication status initialization", func() {
		It("initializes deduplication status on first duplicate", func() {
			// BUSINESS OUTCOME: First duplicate signal recorded in status
			rr := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-test",
					Namespace: "production",
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: "fingerprint-abc123",
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			err := updater.UpdateDeduplicationStatus(ctx, rr)

			Expect(err).NotTo(HaveOccurred())

			// Verify occurrence tracking initialized
			updatedRR := &remediationv1alpha1.RemediationRequest{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())
			Expect(updatedRR.Status.Deduplication).NotTo(BeNil(),
				"Deduplication status initialized on first duplicate")
			Expect(updatedRR.Status.Deduplication.OccurrenceCount).To(Equal(int32(1)),
				"First occurrence recorded")
		})

		It("increments occurrence count for subsequent duplicates", func() {
			// BUSINESS OUTCOME: Operators see total duplicate count
			rr := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-multi",
					Namespace: "production",
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: "fingerprint-multi",
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// Simulate 3 duplicate signals
			for i := 0; i < 3; i++ {
				err := updater.UpdateDeduplicationStatus(ctx, rr)
				Expect(err).NotTo(HaveOccurred())
			}

			// Verify count reflects all duplicates
			updatedRR := &remediationv1alpha1.RemediationRequest{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())
			Expect(updatedRR.Status.Deduplication.OccurrenceCount).To(Equal(int32(3)),
				"Occurrence count tracks duplicate frequency for operator visibility")
		})
	})
})

// ============================================================================
// BUSINESS OUTCOME TESTS: Post-Completion Cooldown (BR-GATEWAY-011 / BR-GATEWAY-012)
// ============================================================================
//
// Test Plan: docs/testing/COOLDOWN_GW_RO/TEST_PLAN.md
//
// BUSINESS VALUE:
// - Prevents wasted LLM calls on signals arriving shortly after successful remediation
// - Avoids stale RRs created from re-firing alerts during cooldown
// - Signals arriving after cooldown create fresh RRs with current data
// ============================================================================

var _ = Describe("BR-GATEWAY-011: Post-completion cooldown prevents stale remediation attempts", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		scheme    *runtime.Scheme
		checker   *processing.PhaseBasedDeduplicationChecker
	)

	const (
		namespace   = "kubernaut-system"
		fingerprint = "b2c3d4e5f6a789012345678901234567890abcdef1234567890abcdef1234567"
		cooldown    = 5 * time.Minute
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(remediationv1alpha1.AddToScheme(scheme)).To(Succeed())

		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&remediationv1alpha1.RemediationRequest{}).
			WithIndex(&remediationv1alpha1.RemediationRequest{}, "spec.signalFingerprint", func(o client.Object) []string {
				rr := o.(*remediationv1alpha1.RemediationRequest)
				return []string{rr.Spec.SignalFingerprint}
			}).
			Build()

		checker = processing.NewPhaseBasedDeduplicationChecker(k8sClient, cooldown)
	})

	Context("UT-GW-011-001: Signal during cooldown after successful remediation", func() {
		It("should deduplicate to prevent wasted LLM calls on stale signal data", func() {
			// BUSINESS OUTCOME: Alert re-fires 2 minutes after a successful fix.
			// Creating a new RR would trigger SP + AA, wasting an expensive LLM call
			// on signal data that describes the pre-remediation state. Gateway deduplicates
			// the signal, protecting LLM resources and preventing stale remediation attempts.
			completedAt := metav1.NewTime(time.Now().Add(-2 * time.Minute))
			rr := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-completed-recent",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
				},
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: remediationv1alpha1.PhaseCompleted,
					CompletedAt:  &completedAt,
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			shouldDedup, existingRR, err := checker.ShouldDeduplicate(ctx, namespace, fingerprint)
			Expect(err).NotTo(HaveOccurred())

			Expect(shouldDedup).To(BeTrue(),
				"Signal within 5-min cooldown after Completed RR must be deduplicated")
			Expect(existingRR).NotTo(BeNil())
			Expect(existingRR.Name).To(Equal("rr-completed-recent"))
		})
	})

	Context("UT-GW-011-002: Signal after cooldown expires", func() {
		It("should allow new RR creation with fresh signal data", func() {
			// BUSINESS OUTCOME: Alert re-fires 6 minutes after a successful fix.
			// The cooldown has expired, so this is likely a genuine recurrence.
			// Gateway allows a new RR so the pipeline processes fresh signal data.
			completedAt := metav1.NewTime(time.Now().Add(-6 * time.Minute))
			rr := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-completed-old",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
				},
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: remediationv1alpha1.PhaseCompleted,
					CompletedAt:  &completedAt,
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			shouldDedup, existingRR, err := checker.ShouldDeduplicate(ctx, namespace, fingerprint)
			Expect(err).NotTo(HaveOccurred())

			Expect(shouldDedup).To(BeFalse(),
				"Signal after cooldown expires must create a new RR with fresh data")
			Expect(existingRR).To(BeNil())
		})
	})

	Context("UT-GW-011-003: Only Completed RRs trigger cooldown", func() {
		It("should not apply cooldown for Failed or TimedOut terminal phases", func() {
			// BUSINESS OUTCOME: A failed or timed-out remediation should NOT suppress
			// new signals -- the issue was not resolved, so a fresh attempt is warranted.
			completedAt := metav1.NewTime(time.Now().Add(-2 * time.Minute))

			failedRR := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-failed-recent",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
				},
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: remediationv1alpha1.PhaseFailed,
					CompletedAt:  &completedAt,
				},
			}
			Expect(k8sClient.Create(ctx, failedRR)).To(Succeed())

			timedOutRR := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-timedout-recent",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
				},
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: remediationv1alpha1.PhaseTimedOut,
					CompletedAt:  &completedAt,
				},
			}
			Expect(k8sClient.Create(ctx, timedOutRR)).To(Succeed())

			shouldDedup, existingRR, err := checker.ShouldDeduplicate(ctx, namespace, fingerprint)
			Expect(err).NotTo(HaveOccurred())

			Expect(shouldDedup).To(BeFalse(),
				"Failed/TimedOut RRs do not trigger cooldown -- issue was not resolved")
			Expect(existingRR).To(BeNil())
		})
	})

	Context("UT-GW-011-004: Non-terminal RR takes priority over cooldown-eligible Completed RR", func() {
		It("should deduplicate against the active RR, not the completed one", func() {
			// BUSINESS OUTCOME: If an active RR is already in-progress AND a recently
			// completed RR exists, the active RR takes precedence for dedup. This preserves
			// existing dedup behavior -- a signal is already being remediated.
			completedAt := metav1.NewTime(time.Now().Add(-1 * time.Minute))
			completedRR := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-completed-with-active",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
				},
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: remediationv1alpha1.PhaseCompleted,
					CompletedAt:  &completedAt,
				},
			}
			Expect(k8sClient.Create(ctx, completedRR)).To(Succeed())

			activeRR := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-active-processing",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
				},
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: remediationv1alpha1.PhaseProcessing,
				},
			}
			Expect(k8sClient.Create(ctx, activeRR)).To(Succeed())

			shouldDedup, existingRR, err := checker.ShouldDeduplicate(ctx, namespace, fingerprint)
			Expect(err).NotTo(HaveOccurred())

			Expect(shouldDedup).To(BeTrue(),
				"Active RR must trigger dedup regardless of cooldown-eligible RRs")
			Expect(existingRR).NotTo(BeNil())
			Expect(existingRR.Status.OverallPhase).To(Equal(remediationv1alpha1.PhaseProcessing),
				"Dedup returns the active (non-terminal) RR, not the completed one")
		})
	})

	Context("UT-GW-011-005: Multiple Completed RRs use most recent CompletedAt", func() {
		It("should use the most recently completed RR for cooldown calculation", func() {
			// BUSINESS OUTCOME: If the same signal produced multiple completed RRs
			// (e.g., recurring issue remediated twice), the cooldown window is based
			// on the latest completion. An older completion that's outside cooldown
			// should not allow a new RR if a newer completion is still within cooldown.
			oldCompletedAt := metav1.NewTime(time.Now().Add(-10 * time.Minute))
			oldRR := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-completed-old-multi",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
				},
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: remediationv1alpha1.PhaseCompleted,
					CompletedAt:  &oldCompletedAt,
				},
			}
			Expect(k8sClient.Create(ctx, oldRR)).To(Succeed())

			recentCompletedAt := metav1.NewTime(time.Now().Add(-1 * time.Minute))
			recentRR := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-completed-recent-multi",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
				},
				Status: remediationv1alpha1.RemediationRequestStatus{
					OverallPhase: remediationv1alpha1.PhaseCompleted,
					CompletedAt:  &recentCompletedAt,
				},
			}
			Expect(k8sClient.Create(ctx, recentRR)).To(Succeed())

			shouldDedup, existingRR, err := checker.ShouldDeduplicate(ctx, namespace, fingerprint)
			Expect(err).NotTo(HaveOccurred())

			Expect(shouldDedup).To(BeTrue(),
				"Most recent Completed RR (1 min ago) is within cooldown -- must deduplicate")
			Expect(existingRR).NotTo(BeNil())
			Expect(existingRR.Name).To(Equal("rr-completed-recent-multi"),
				"Dedup returns the most recently completed RR")
		})
	})
})
