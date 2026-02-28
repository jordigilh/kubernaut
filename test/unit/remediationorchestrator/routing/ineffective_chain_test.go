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

package routing

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// mockHistoryQuerier implements routing.RemediationHistoryQuerier for unit tests.
type mockHistoryQuerier struct {
	entries []ogenclient.RemediationHistoryEntry
	err     error
}

func (m *mockHistoryQuerier) GetRemediationHistory(
	ctx context.Context,
	target routing.TargetResource,
	currentSpecHash string,
	window time.Duration,
) ([]ogenclient.RemediationHistoryEntry, error) {
	return m.entries, m.err
}

// helper to create a DS entry with hash chain data
func newDSEntry(preHash, postHash string, hashMatch ogenclient.RemediationHistoryEntryHashMatch, outcome string, completedAt time.Time) ogenclient.RemediationHistoryEntry {
	entry := ogenclient.RemediationHistoryEntry{
		RemediationUID:          fmt.Sprintf("rr-uid-%d", completedAt.UnixNano()),
		PreRemediationSpecHash:  ogenclient.NewOptString(preHash),
		PostRemediationSpecHash: ogenclient.NewOptString(postHash),
		HashMatch:               ogenclient.NewOptRemediationHistoryEntryHashMatch(hashMatch),
		Outcome:                 ogenclient.NewOptString(outcome),
		CompletedAt:             completedAt,
	}
	return entry
}

// helper to create a DS entry with no hash data (for safety net tests)
func newDSEntryNoHash(outcome string, completedAt time.Time) ogenclient.RemediationHistoryEntry {
	return ogenclient.RemediationHistoryEntry{
		RemediationUID: fmt.Sprintf("rr-uid-%d", completedAt.UnixNano()),
		Outcome:        ogenclient.NewOptString(outcome),
		CompletedAt:    completedAt,
	}
}

var _ = Describe("CheckIneffectiveRemediationChain (Issue #214)", func() {
	var (
		ctx        context.Context
		fakeClient client.Client
		engine     *routing.RoutingEngine
		scheme     *runtime.Scheme
	)

	target := routing.TargetResource{
		Kind:      "Deployment",
		Name:      "payment-api",
		Namespace: "prod",
	}
	preHash := "abc123hash"

	setupEngine := func(querier routing.RemediationHistoryQuerier) {
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithIndex(&remediationv1.RemediationRequest{}, "spec.signalFingerprint", func(obj client.Object) []string {
				rr := obj.(*remediationv1.RemediationRequest)
				if rr.Spec.SignalFingerprint == "" {
					return nil
				}
				return []string{rr.Spec.SignalFingerprint}
			}).
			WithIndex(&workflowexecutionv1.WorkflowExecution{}, "spec.targetResource", func(obj client.Object) []string {
				wfe := obj.(*workflowexecutionv1.WorkflowExecution)
				if wfe.Spec.TargetResource == "" {
					return nil
				}
				return []string{wfe.Spec.TargetResource}
			}).
			Build()

		config := routing.Config{
			ConsecutiveFailureThreshold: 3,
			ConsecutiveFailureCooldown:  3600,
			RecentlyRemediatedCooldown:  300,
			ExponentialBackoffBase:      60,
			ExponentialBackoffMax:       600,
			ExponentialBackoffMaxExponent: 4,
			IneffectiveChainThreshold:   3,
			RecurrenceCountThreshold:    5,
			IneffectiveTimeWindow:       4 * time.Hour,
		}

		engine = routing.NewRoutingEngine(fakeClient, fakeClient, "default", config, &mocks.AlwaysManagedScopeChecker{}, querier)
	}

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ========================================
	// Layer 1+2: Hash chain + spec_drift
	// ========================================

	Context("UT-RO-214-001: Hash chain match across consecutive entries", func() {
		It("should return BlockReasonIneffectiveChain when 3 consecutive entries match hash chain within window", func() {
			now := time.Now()
			entries := []ogenclient.RemediationHistoryEntry{
				newDSEntry(preHash, "post1", ogenclient.RemediationHistoryEntryHashMatchPostRemediation, "Completed", now.Add(-3*time.Hour)),
				newDSEntry(preHash, "post2", ogenclient.RemediationHistoryEntryHashMatchPostRemediation, "Completed", now.Add(-2*time.Hour)),
				newDSEntry(preHash, "post3", ogenclient.RemediationHistoryEntryHashMatchPostRemediation, "Completed", now.Add(-1*time.Hour)),
			}
			setupEngine(&mockHistoryQuerier{entries: entries})

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "incoming-rr",
					Namespace: "default",
					UID:       types.UID("incoming-uid"),
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "fp-214-001",
				},
			}

			blocked := engine.CheckIneffectiveRemediationChain(ctx, rr, target, preHash)
			Expect(blocked).ToNot(BeNil(), "Should block when hash chain matches across threshold entries")
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonIneffectiveChain)))
		})
	})

	Context("UT-RO-214-002: Regression/spec_drift (HashMatch == preRemediation)", func() {
		It("should return BlockReasonIneffectiveChain for consecutive regression entries", func() {
			now := time.Now()
			entries := []ogenclient.RemediationHistoryEntry{
				newDSEntry(preHash, "post1", ogenclient.RemediationHistoryEntryHashMatchPreRemediation, "Completed", now.Add(-3*time.Hour)),
				newDSEntry(preHash, "post2", ogenclient.RemediationHistoryEntryHashMatchPreRemediation, "Completed", now.Add(-2*time.Hour)),
				newDSEntry(preHash, "post3", ogenclient.RemediationHistoryEntryHashMatchPreRemediation, "Completed", now.Add(-1*time.Hour)),
			}
			setupEngine(&mockHistoryQuerier{entries: entries})

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "incoming-rr",
					Namespace: "default",
					UID:       types.UID("incoming-uid"),
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "fp-214-002",
				},
			}

			blocked := engine.CheckIneffectiveRemediationChain(ctx, rr, target, preHash)
			Expect(blocked).ToNot(BeNil(), "Should block when consecutive regression/spec_drift entries detected")
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonIneffectiveChain)))
		})
	})

	Context("UT-RO-214-003: Chain breaks when entry has no regression and hash differs", func() {
		It("should return nil when hash chain is broken by an effective entry", func() {
			now := time.Now()
			entries := []ogenclient.RemediationHistoryEntry{
				newDSEntry(preHash, "post1", ogenclient.RemediationHistoryEntryHashMatchPostRemediation, "Completed", now.Add(-3*time.Hour)),
				newDSEntry("different-hash", "post2", ogenclient.RemediationHistoryEntryHashMatchNone, "Completed", now.Add(-2*time.Hour)),
				newDSEntry(preHash, "post3", ogenclient.RemediationHistoryEntryHashMatchPostRemediation, "Completed", now.Add(-1*time.Hour)),
			}
			setupEngine(&mockHistoryQuerier{entries: entries})

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "incoming-rr",
					Namespace: "default",
					UID:       types.UID("incoming-uid"),
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "fp-214-003",
				},
			}

			blocked := engine.CheckIneffectiveRemediationChain(ctx, rr, target, preHash)
			Expect(blocked).To(BeNil(), "Should not block when chain is broken by an effective entry")
		})
	})

	Context("UT-RO-214-004: DS entry missing hash data", func() {
		It("should return nil when entries lack hash data to determine chain", func() {
			now := time.Now()
			entries := []ogenclient.RemediationHistoryEntry{
				newDSEntryNoHash("Completed", now.Add(-3*time.Hour)),
				newDSEntryNoHash("Completed", now.Add(-2*time.Hour)),
				newDSEntryNoHash("Completed", now.Add(-1*time.Hour)),
			}
			setupEngine(&mockHistoryQuerier{entries: entries})

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "incoming-rr",
					Namespace: "default",
					UID:       types.UID("incoming-uid"),
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "fp-214-004",
				},
			}

			blocked := engine.CheckIneffectiveRemediationChain(ctx, rr, target, preHash)
			Expect(blocked).To(BeNil(), "Should not block when hash data is missing (cannot determine chain)")
		})
	})

	Context("UT-RO-214-005: Below chain threshold", func() {
		It("should return nil when only 2 ineffective entries exist (threshold = 3)", func() {
			now := time.Now()
			entries := []ogenclient.RemediationHistoryEntry{
				newDSEntry(preHash, "post1", ogenclient.RemediationHistoryEntryHashMatchPostRemediation, "Completed", now.Add(-2*time.Hour)),
				newDSEntry(preHash, "post2", ogenclient.RemediationHistoryEntryHashMatchPostRemediation, "Completed", now.Add(-1*time.Hour)),
			}
			setupEngine(&mockHistoryQuerier{entries: entries})

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "incoming-rr",
					Namespace: "default",
					UID:       types.UID("incoming-uid"),
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "fp-214-005",
				},
			}

			blocked := engine.CheckIneffectiveRemediationChain(ctx, rr, target, preHash)
			Expect(blocked).To(BeNil(), "Should not block when below chain threshold")
		})
	})

	// ========================================
	// Layer 3: Safety net (count + time window)
	// ========================================

	Context("UT-RO-214-006: Safety net escalation with no hash data", func() {
		It("should return BlockReasonIneffectiveChain when entries >= recurrenceCountThreshold within window", func() {
			now := time.Now()
			entries := make([]ogenclient.RemediationHistoryEntry, 5)
			for i := 0; i < 5; i++ {
				entries[i] = newDSEntryNoHash("Completed", now.Add(-time.Duration(5-i)*30*time.Minute))
			}
			setupEngine(&mockHistoryQuerier{entries: entries})

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "incoming-rr",
					Namespace: "default",
					UID:       types.UID("incoming-uid"),
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "fp-214-006",
				},
			}

			blocked := engine.CheckIneffectiveRemediationChain(ctx, rr, target, preHash)
			Expect(blocked).ToNot(BeNil(), "Should block via safety net when total entries >= threshold")
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonIneffectiveChain)))
		})
	})

	Context("UT-RO-214-007: Stale entries outside time window", func() {
		It("should return nil when all entries are outside the time window", func() {
			now := time.Now()
			entries := make([]ogenclient.RemediationHistoryEntry, 5)
			for i := 0; i < 5; i++ {
				entries[i] = newDSEntryNoHash("Completed", now.Add(-5*time.Hour-time.Duration(i)*time.Hour))
			}
			setupEngine(&mockHistoryQuerier{entries: entries})

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "incoming-rr",
					Namespace: "default",
					UID:       types.UID("incoming-uid"),
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "fp-214-007",
				},
			}

			blocked := engine.CheckIneffectiveRemediationChain(ctx, rr, target, preHash)
			Expect(blocked).To(BeNil(), "Should not block when entries are outside time window")
		})
	})

	// ========================================
	// Cross-layer
	// ========================================

	Context("UT-RO-214-008: CheckConsecutiveFailures unchanged (regression guard)", func() {
		It("should preserve existing CheckConsecutiveFailures behavior", func() {
			setupEngine(&mockHistoryQuerier{entries: nil})

			baseTime := time.Now().Add(-10 * time.Minute)
			for i := 0; i < 3; i++ {
				failedRR := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:              fmt.Sprintf("failed-rr-214-008-%d", i),
						Namespace:         "default",
						UID:               types.UID(fmt.Sprintf("failed-uid-214-008-%d", i)),
						CreationTimestamp: metav1.Time{Time: baseTime.Add(time.Duration(i) * time.Minute)},
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: "fp-214-008-consecutive",
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: remediationv1.PhaseFailed,
					},
				}
				Expect(fakeClient.Create(ctx, failedRR)).To(Succeed())
			}

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "incoming-rr-214-008",
					Namespace:         "default",
					UID:               types.UID("incoming-uid-214-008"),
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "fp-214-008-consecutive",
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseAnalyzing,
				},
			}

			blocked := engine.CheckConsecutiveFailures(ctx, rr)
			Expect(blocked).ToNot(BeNil(), "CheckConsecutiveFailures should still work")
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonConsecutiveFailures)))
		})
	})

	Context("UT-RO-214-009: Mixed Failed + ineffective Completed in same fingerprint", func() {
		It("should allow both consecutive-failure and ineffective-chain checks to coexist", func() {
			now := time.Now()
			entries := []ogenclient.RemediationHistoryEntry{
				newDSEntry(preHash, "post1", ogenclient.RemediationHistoryEntryHashMatchPreRemediation, "Completed", now.Add(-3*time.Hour)),
				newDSEntry(preHash, "post2", ogenclient.RemediationHistoryEntryHashMatchPreRemediation, "Completed", now.Add(-2*time.Hour)),
				newDSEntry(preHash, "post3", ogenclient.RemediationHistoryEntryHashMatchPreRemediation, "Completed", now.Add(-1*time.Hour)),
			}
			setupEngine(&mockHistoryQuerier{entries: entries})

			baseTime := now.Add(-4 * time.Hour)
			for i := 0; i < 2; i++ {
				failedRR := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:              fmt.Sprintf("failed-rr-214-009-%d", i),
						Namespace:         "default",
						UID:               types.UID(fmt.Sprintf("failed-uid-214-009-%d", i)),
						CreationTimestamp: metav1.Time{Time: baseTime.Add(time.Duration(i) * time.Minute)},
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: "fp-214-009-mixed",
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: remediationv1.PhaseFailed,
					},
				}
				Expect(fakeClient.Create(ctx, failedRR)).To(Succeed())
			}

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "incoming-rr-214-009",
					Namespace:         "default",
					UID:               types.UID("incoming-uid-214-009"),
					CreationTimestamp: metav1.Time{Time: now},
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "fp-214-009-mixed",
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseAnalyzing,
				},
			}

			// ConsecutiveFailures should NOT block (only 2 failures, threshold = 3)
			failBlocked := engine.CheckConsecutiveFailures(ctx, rr)
			Expect(failBlocked).To(BeNil(), "2 failures < threshold 3, should not block")

			// IneffectiveChain SHOULD block (3 regression entries)
			chainBlocked := engine.CheckIneffectiveRemediationChain(ctx, rr, target, preHash)
			Expect(chainBlocked).ToNot(BeNil(), "3 ineffective entries should trigger ineffective chain block")
			Expect(chainBlocked.Reason).To(Equal(string(remediationv1.BlockReasonIneffectiveChain)))
		})
	})

	// ========================================
	// Fail-open on DS errors
	// ========================================

	Context("DS query failure (fail-open)", func() {
		It("should return nil when DataStorage query fails", func() {
			setupEngine(&mockHistoryQuerier{err: fmt.Errorf("DS connection timeout")})

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "incoming-rr-ds-err",
					Namespace: "default",
					UID:       types.UID("incoming-uid-ds-err"),
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "fp-ds-err",
				},
			}

			blocked := engine.CheckIneffectiveRemediationChain(ctx, rr, target, preHash)
			Expect(blocked).To(BeNil(), "DS failures must fail-open (log + return nil)")
		})
	})
})
