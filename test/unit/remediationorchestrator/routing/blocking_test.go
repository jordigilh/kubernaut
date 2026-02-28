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
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

var _ = Describe("Routing Engine - Blocking Logic", func() {
	var (
		ctx        context.Context
		fakeClient client.Client
		engine     *routing.RoutingEngine
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create scheme with both CRD types
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())

		// Create fake client with field indexes
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

		// Create routing engine with test config
		config := routing.Config{
			ConsecutiveFailureThreshold: 3,
			ConsecutiveFailureCooldown:  3600, // 1 hour in seconds
			RecentlyRemediatedCooldown:  300,  // 5 minutes in seconds
			// Exponential backoff config (DD-WE-004, V1.0)
			ExponentialBackoffBase:        60,  // 1 minute
			ExponentialBackoffMax:         600, // 10 minutes
			ExponentialBackoffMaxExponent: 4,   // 2^4 = 16x
		}
		// DD-STATUS-001: Pass fakeClient as both client and apiReader
		// In unit tests, fake client implements both interfaces
		// BR-SCOPE-010: AlwaysManagedScopeChecker so existing tests pass through scope check
		engine = routing.NewRoutingEngine(fakeClient, fakeClient, "default", config, &mocks.AlwaysManagedScopeChecker{})
	})

	// ========================================
	// Test Group 1: CheckConsecutiveFailures (3 tests)
	// Reference: BR-ORCH-042
	// ========================================
	Context("CheckConsecutiveFailures", func() {
		It("should block when consecutive failures >= threshold", func() {
			// Create 3 previous Failed RRs with same fingerprint (threshold = 3)
			// Set explicit CreationTimestamp so routing engine can sort them properly
			// Set explicit UID because fake client doesn't auto-generate them
			baseTime := time.Now().Add(-10 * time.Minute)
			for i := 0; i < 3; i++ {
				failedRR := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:              fmt.Sprintf("failed-rr-%d", i),
						Namespace:         "default",
						UID:               types.UID(fmt.Sprintf("failed-uid-%d", i)), // Explicit UID for fake client
						CreationTimestamp: metav1.Time{Time: baseTime.Add(time.Duration(i) * time.Minute)},
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: "abc123",
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: remediationv1.PhaseFailed,
					},
				}
				Expect(fakeClient.Create(ctx, failedRR)).To(Succeed())
			}

			// Create incoming RR (NOT created in client - routing engine will query for EXISTING RRs only)
			// The routing engine uses item.UID == rr.UID to skip the incoming RR during List() iteration
			// Set explicit UID different from the Failed RRs so it won't be skipped
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rr-consecutive-fail",
					Namespace:         "default",
					UID:               types.UID("incoming-rr-uid"), // Different UID so it won't be skipped
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "abc123",
				},
			}

			blocked := engine.CheckConsecutiveFailures(ctx, rr)

			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonConsecutiveFailures)))
			Expect(blocked.Blocked).To(BeTrue())
			Expect(blocked.RequeueAfter).To(Equal(time.Duration(3600) * time.Second))
		})

		It("should not block when consecutive failures < threshold", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-below-threshold",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "xyz789",
				},
				Status: remediationv1.RemediationRequestStatus{
					ConsecutiveFailureCount: 2, // Below threshold
				},
			}

			blocked := engine.CheckConsecutiveFailures(ctx, rr)

			Expect(blocked).To(BeNil())
		})

		It("should isolate consecutive failure counting by TargetResource.Namespace (#222)", func() {
			// BUG REPRODUCTION: ADR-057 consolidated all CRDs into ROControllerNamespace.
			// CheckConsecutiveFailures uses client.InNamespace(rr.Namespace) which is always
			// ROControllerNamespace, so failures from tenant-A's workloads incorrectly
			// block tenant-B's workloads if they share a fingerprint.
			//
			// Reference: https://github.com/jordigilh/kubernaut/issues/222

			sharedFingerprint := "shared-fp-multitenant"

			// Create 3 Failed RRs targeting namespace "tenant-a"
			baseTime := time.Now().Add(-10 * time.Minute)
			for i := 0; i < 3; i++ {
				failedRR := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:              fmt.Sprintf("tenant-a-failed-%d", i),
						Namespace:         "default",
						UID:               types.UID(fmt.Sprintf("tenant-a-uid-%d", i)),
						CreationTimestamp: metav1.Time{Time: baseTime.Add(time.Duration(i) * time.Minute)},
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: sharedFingerprint,
						TargetResource: remediationv1.ResourceIdentifier{
							Kind:      "Deployment",
							Name:      "app",
							Namespace: "tenant-a",
						},
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: remediationv1.PhaseFailed,
					},
				}
				Expect(fakeClient.Create(ctx, failedRR)).To(Succeed())
			}

			// Incoming RR targets namespace "tenant-b" with the SAME fingerprint
			incomingRR := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "tenant-b-new",
					Namespace:         "default",
					UID:               types.UID("tenant-b-uid"),
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: sharedFingerprint,
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "app",
						Namespace: "tenant-b",
					},
				},
			}

			blocked := engine.CheckConsecutiveFailures(ctx, incomingRR)

			Expect(blocked).To(BeNil(),
				"Tenant-B should NOT be blocked by tenant-A's 3 failures (namespace isolation)")
		})

		It("should set cooldown message with expiry time", func() {
			// Create 5 previous Failed RRs with same fingerprint
			// Set explicit UID because fake client doesn't auto-generate them
			baseTime := time.Now().Add(-15 * time.Minute)
			for i := 0; i < 5; i++ {
				failedRR := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:              fmt.Sprintf("failed-msg-rr-%d", i),
						Namespace:         "default",
						UID:               types.UID(fmt.Sprintf("failed-msg-uid-%d", i)),
						CreationTimestamp: metav1.Time{Time: baseTime.Add(time.Duration(i) * time.Minute)},
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: "msg123",
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: remediationv1.PhaseFailed,
					},
				}
				Expect(fakeClient.Create(ctx, failedRR)).To(Succeed())
			}

			// Create incoming RR with different UID so it won't be skipped
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rr-message-check",
					Namespace:         "default",
					UID:               types.UID("incoming-msg-uid"),
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "msg123",
				},
			}

			blocked := engine.CheckConsecutiveFailures(ctx, rr)

			Expect(blocked.Message).To(ContainSubstring("5 consecutive failures"))
			Expect(blocked.Message).To(ContainSubstring("Cooldown expires:"))
		})
	})

	// ========================================
	// Test Group 2: CheckDuplicateInProgress (8 tests)
	// Reference: DD-RO-002-ADDENDUM
	// CRITICAL: Prevents Gateway RR flood
	// ========================================
	Context("CheckDuplicateInProgress", func() {
		It("should isolate duplicate detection by TargetResource.Namespace (#222)", func() {
			// BUG REPRODUCTION: Same ADR-057 regression as CheckConsecutiveFailures.
			// FindActiveRRForFingerprint uses client.InNamespace(rr.Namespace) which is
			// always ROControllerNamespace, so an active/blocked RR targeting tenant-A
			// incorrectly blocks a new RR targeting tenant-B with the same fingerprint.
			//
			// Reference: https://github.com/jordigilh/kubernaut/issues/222

			sharedFingerprint := "dup-multitenant-fp"

			// Create an active RR targeting namespace "tenant-a"
			activeRR := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tenant-a-active",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: sharedFingerprint,
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "app",
						Namespace: "tenant-a",
					},
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseBlocked,
				},
			}
			Expect(fakeClient.Create(ctx, activeRR)).To(Succeed())

			// Incoming RR targets "tenant-b" — should NOT be a duplicate
			incomingRR := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tenant-b-new",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: sharedFingerprint,
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "app",
						Namespace: "tenant-b",
					},
				},
			}

			blocked, err := engine.CheckDuplicateInProgress(ctx, incomingRR)

			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeNil(),
				"Tenant-B should NOT be blocked as duplicate of tenant-A's active RR (namespace isolation)")
		})

		It("should not block the older RR when a newer duplicate exists (#209)", func() {
			// BUG REPRODUCTION: Circular duplicate deadlock.
			// Two RRs with the same fingerprint arrive close together.
			// RR-A (older) progresses to Processing. RR-B (newer) is blocked as duplicate.
			// On re-reconcile, RR-A's CheckDuplicateInProgress finds RR-B (Blocked,
			// non-terminal) and blocks RR-A → circular deadlock.
			//
			// Fix: Deterministic tiebreaker — the oldest RR is always the original.
			// Reference: https://github.com/jordigilh/kubernaut/issues/209

			sharedFP := "circular-deadlock-fp"

			// RR-A: older, already progressed to Processing
			rrA := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rr-a-older",
					Namespace:         "default",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-2 * time.Minute)},
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: sharedFP,
					TargetResource: remediationv1.ResourceIdentifier{
						Kind: "Deployment", Name: "app", Namespace: "ns-a",
					},
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseProcessing,
				},
			}
			Expect(fakeClient.Create(ctx, rrA)).To(Succeed())

			// RR-B: newer, blocked as duplicate of A (non-terminal)
			rrB := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rr-b-newer",
					Namespace:         "default",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: sharedFP,
					TargetResource: remediationv1.ResourceIdentifier{
						Kind: "Deployment", Name: "app", Namespace: "ns-a",
					},
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseBlocked,
					BlockReason:  string(remediationv1.BlockReasonDuplicateInProgress),
					DuplicateOf:  "rr-a-older",
				},
			}
			Expect(fakeClient.Create(ctx, rrB)).To(Succeed())

			// When RR-A is re-reconciled, it should NOT be blocked by RR-B
			blocked, err := engine.CheckDuplicateInProgress(ctx, rrA)

			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeNil(),
				"Older RR-A should NOT be blocked by newer RR-B — prevents circular deadlock (#209)")
		})

		It("should block the newer RR when an older active RR exists (#209)", func() {
			// Counterpart to the circular deadlock test: the NEWER RR should still be blocked.
			sharedFP := "tiebreaker-fp"

			// RR-A: older, actively processing
			rrA := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rr-older-active",
					Namespace:         "default",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: sharedFP,
					TargetResource: remediationv1.ResourceIdentifier{
						Kind: "Deployment", Name: "app", Namespace: "ns-a",
					},
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseProcessing,
				},
			}
			Expect(fakeClient.Create(ctx, rrA)).To(Succeed())

			// RR-B: newer, in Pending phase
			rrB := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rr-newer-pending",
					Namespace:         "default",
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: sharedFP,
					TargetResource: remediationv1.ResourceIdentifier{
						Kind: "Deployment", Name: "app", Namespace: "ns-a",
					},
				},
			}

			blocked, err := engine.CheckDuplicateInProgress(ctx, rrB)

			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).ToNot(BeNil(), "Newer RR-B should be blocked by older RR-A")
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonDuplicateInProgress)))
			Expect(blocked.DuplicateOf).To(Equal("rr-older-active"))
		})

		It("should block when active RR with same fingerprint exists", func() {
			// Create original RR (active - non-terminal phase)
			originalRR := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-original",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "duplicate-fp-123",
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseExecuting, // Non-terminal
				},
			}
			Expect(fakeClient.Create(ctx, originalRR)).To(Succeed())

			// Create duplicate RR
			duplicateRR := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-duplicate",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "duplicate-fp-123", // Same fingerprint
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhasePending,
				},
			}

			blocked, err := engine.CheckDuplicateInProgress(ctx, duplicateRR)

			// GREEN: Test should pass
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonDuplicateInProgress)))
			Expect(blocked.Blocked).To(BeTrue())
			Expect(blocked.DuplicateOf).To(Equal("rr-original"))
			Expect(blocked.RequeueAfter).To(Equal(30 * time.Second))
		})

		It("should not block when original RR is terminal", func() {
			// Create original RR (terminal phase)
			originalRR := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-terminal",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "terminal-fp-456",
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseCompleted, // Terminal
				},
			}
			Expect(fakeClient.Create(ctx, originalRR)).To(Succeed())

			// Create new RR with same fingerprint
			newRR := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-new",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "terminal-fp-456",
				},
			}

			blocked, err := engine.CheckDuplicateInProgress(ctx, newRR)

			// GREEN: Test should pass
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeNil()) // Not blocked (original is terminal)
		})

		It("should not block when no duplicate exists", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-unique",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "unique-fp-789",
				},
			}

			blocked, err := engine.CheckDuplicateInProgress(ctx, rr)

			// GREEN: Test should pass
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeNil())
		})

		It("should not block on self (same name)", func() {
			// Create RR
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-self",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "self-fp-999",
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseExecuting,
				},
			}
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			// Check if it blocks itself (should not)
			blocked, err := engine.CheckDuplicateInProgress(ctx, rr)

			// GREEN: Test should pass
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeNil()) // Should not block on self
		})

		It("should block even when multiple active duplicates exist", func() {
			// Create first active RR
			rr1 := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-first",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "multi-fp-111",
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseExecuting,
				},
			}
			Expect(fakeClient.Create(ctx, rr1)).To(Succeed())

			// Create second active RR (also active)
			rr2 := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-second",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "multi-fp-111",
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseAnalyzing,
				},
			}
			Expect(fakeClient.Create(ctx, rr2)).To(Succeed())

			// Create third RR (checking duplicate)
			rr3 := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-third",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "multi-fp-111",
				},
			}

			blocked, err := engine.CheckDuplicateInProgress(ctx, rr3)

			// GREEN: Test should pass
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonDuplicateInProgress)))
			// Should reference one of the active RRs (either rr-first or rr-second)
			Expect(blocked.DuplicateOf).To(Or(Equal("rr-first"), Equal("rr-second")))
		})
	})

	// ========================================
	// Test Group 3: CheckResourceBusy (3 tests)
	// Reference: DD-RO-002, DD-WE-001
	// ========================================
	Context("CheckResourceBusy", func() {
		It("should block when Running WFE on same target exists", func() {
			// Create running WFE on target resource
			wfe := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-running",
					Namespace: "default",
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource: "default/pod/nginx-12345",
				},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1.PhaseRunning, // Active
				},
			}
			Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

			// Create RR targeting same resource
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-busy-target",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "nginx-12345",
						Namespace: "default",
					},
				},
			}

			blocked, err := engine.CheckResourceBusy(ctx, rr, "default/pod/nginx-12345")

			// GREEN: Test should pass
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonResourceBusy)))
			Expect(blocked.Blocked).To(BeTrue())
			Expect(blocked.BlockingWorkflowExecution).To(Equal("wfe-running"))
			Expect(blocked.RequeueAfter).To(Equal(30 * time.Second))
		})

		It("should not block when WFE is terminal", func() {
			// Create completed WFE
			wfe := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-completed",
					Namespace: "default",
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource: "default/pod/nginx-67890",
				},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1.PhaseCompleted, // Terminal
				},
			}
			Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

			// Create RR targeting same resource
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-available-target",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "nginx-67890",
						Namespace: "default",
					},
				},
			}

			blocked, err := engine.CheckResourceBusy(ctx, rr, "default/pod/nginx-67890")

			// GREEN: Test should pass
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeNil()) // Not blocked (WFE is terminal)
		})

		It("should not block when no WFE on target", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-free-target",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "nginx-99999",
						Namespace: "default",
					},
				},
			}

			blocked, err := engine.CheckResourceBusy(ctx, rr, "default/pod/nginx-99999")

			// GREEN: Test should pass
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeNil())
		})
	})

	// ========================================
	// Test Group 4: CheckRecentlyRemediated (4 tests)
	// Reference: DD-WE-001
	// Note: Tests simplified to check target resource cooldown
	// WorkflowID matching done via WorkflowRef in actual implementation
	// ========================================
	Context("CheckRecentlyRemediated", func() {
		It("should block when recent WFE within 5min cooldown", func() {
			// Create recently completed WFE (within cooldown)
			completionTime := metav1.Now()
			wfe := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-recent",
					Namespace: "default",
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource: "default/pod/nginx-recent",
					WorkflowRef: workflowexecutionv1.WorkflowRef{
						WorkflowID: "restart-workflow",
						Version:    "v1",
					},
				},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase:          workflowexecutionv1.PhaseCompleted,
					CompletionTime: &completionTime,
				},
			}
			Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

			// Create RR for same target
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-cooldown",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "nginx-recent",
						Namespace: "default",
					},
				},
			}

			// Pass same workflow ID as the WFE to trigger cooldown
			blocked, err := engine.CheckRecentlyRemediated(ctx, rr, "restart-workflow", "default/pod/nginx-recent")

			// GREEN: Test should pass
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonRecentlyRemediated)))
			Expect(blocked.Blocked).To(BeTrue())
			Expect(blocked.RequeueAfter).To(BeNumerically(">", 0))
		})

		It("should not block when WFE outside cooldown", func() {
			// Create old completed WFE (outside cooldown)
			oldTime := metav1.NewTime(time.Now().Add(-10 * time.Minute)) // 10 min ago
			wfe := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-old",
					Namespace: "default",
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource: "default/pod/nginx-old",
					WorkflowRef: workflowexecutionv1.WorkflowRef{
						WorkflowID: "restart-workflow",
						Version:    "v1",
					},
				},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase:          workflowexecutionv1.PhaseCompleted,
					CompletionTime: &oldTime,
				},
			}
			Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

			// Create RR for same target
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-expired-cooldown",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "nginx-old",
						Namespace: "default",
					},
				},
			}

			// Same workflow ID but outside cooldown window
			blocked, err := engine.CheckRecentlyRemediated(ctx, rr, "restart-workflow", "default/pod/nginx-old")

			// GREEN: Test should pass
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeNil()) // Not blocked (cooldown expired)
		})

		It("should set BlockedUntil to cooldown expiry", func() {
			// Create recently completed WFE
			completionTime := metav1.Now()
			wfe := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-check-expiry",
					Namespace: "default",
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource: "default/pod/nginx-expiry",
					WorkflowRef: workflowexecutionv1.WorkflowRef{
						WorkflowID: "restart-workflow",
						Version:    "v1",
					},
				},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase:          workflowexecutionv1.PhaseCompleted,
					CompletionTime: &completionTime,
				},
			}
			Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-expiry-check",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "nginx-expiry",
						Namespace: "default",
					},
				},
			}

			// Same workflow ID to trigger cooldown
			blocked, err := engine.CheckRecentlyRemediated(ctx, rr, "restart-workflow", "default/pod/nginx-expiry")

			// GREEN: Test should pass
			Expect(err).ToNot(HaveOccurred())
			// BlockedUntil should be approximately completion time + 5 minutes
			expectedExpiry := completionTime.Add(5 * time.Minute)
			Expect(blocked.BlockedUntil.Sub(expectedExpiry)).To(BeNumerically("<", 1*time.Second))
		})

		It("should not block for different workflow on same target", func() {
			// TDD RED: This test validates DD-RO-002 Check 4 - workflow-specific cooldown
			//
			// ARCHITECTURE NOTE (December 16, 2025):
			// The workflow ID comes from AIAnalysis.Status.SelectedWorkflow.WorkflowID,
			// NOT from RR.Spec (which is immutable and doesn't contain workflow info).
			// The reconciler passes this ID to CheckRecentlyRemediated when checking
			// routing conditions in handleAnalyzingPhase, after AIAnalysis completes.
			//
			// Scenario:
			// - WFE for "restart-workflow" on pod X completed recently
			// - New RR targeting pod X wants to run "scale-workflow" (different workflow)
			// - Should NOT be blocked (different remediation approach)

			// Create completed WFE with workflow A (restart-workflow)
			completionTime := metav1.Now()
			wfe := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-workflow-a",
					Namespace: "default",
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource: "default/pod/nginx-multi-workflow",
					WorkflowRef: workflowexecutionv1.WorkflowRef{
						WorkflowID: "restart-workflow", // Workflow A
						Version:    "v1",
					},
				},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase:          workflowexecutionv1.PhaseCompleted,
					CompletionTime: &completionTime,
				},
			}
			Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

			// Create RR for same target
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-workflow-b",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "nginx-multi-workflow",
						Namespace: "default",
					},
				},
			}

			// Call with DIFFERENT workflow ID (scale-workflow vs restart-workflow)
			// This simulates: AIAnalysis selected "scale-workflow" for this RR
			blocked, err := engine.CheckRecentlyRemediated(ctx, rr, "scale-workflow", "default/pod/nginx-multi-workflow")

			// Expect NOT blocked - different workflow on same target should be allowed
			// Per DD-RO-002 Check 4: "If same workflow executed recently for same target"
			// Since workflow is DIFFERENT, cooldown should NOT apply
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeNil()) // Not blocked (different workflow)
		})
	})

	// ========================================
	// Test Group 4b: Target resource casing (Issue #203)
	// Test Plan: docs/testing/COOLDOWN_GW_RO/TEST_PLAN.md
	// Reference: DD-WE-001 (target resource format namespace/Kind/name)
	//
	// BUSINESS VALUE:
	// - Correct casing ensures CheckRecentlyRemediated and CheckResourceBusy
	//   find previously completed WFEs, enabling defense-in-depth cooldown
	// - Case mismatch breaks field selectors, silently disabling cooldown
	// ========================================
	Context("UT-RO-WE001: Target resource casing must match WFE storage format (#203)", func() {
		It("UT-RO-WE001-001: should find recently completed WFE when target casing matches", func() {
			// BUSINESS OUTCOME: RO correctly detects that the same workflow+target
			// was recently executed, preventing redundant remediation that would waste
			// LLM calls and potentially conflict with the previous remediation.
			completionTime := metav1.Now()
			wfe := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-casing-match",
					Namespace: "default",
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource: "demo-hpa/Deployment/api-frontend",
					WorkflowRef: workflowexecutionv1.WorkflowRef{
						WorkflowID: "patch-hpa-v1",
						Version:    "v1",
					},
				},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase:          workflowexecutionv1.PhaseCompleted,
					CompletionTime: &completionTime,
				},
			}
			Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-casing-match",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "api-frontend",
						Namespace: "demo-hpa",
					},
				},
			}

			blocked, err := engine.CheckRecentlyRemediated(ctx, rr, "patch-hpa-v1", "demo-hpa/Deployment/api-frontend")

			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).ToNot(BeNil(),
				"Same casing (Deployment) must match the WFE and trigger cooldown")
			Expect(blocked.Blocked).To(BeTrue())
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonRecentlyRemediated)))
		})

		It("UT-RO-WE001-002: should NOT find WFE when target uses lowercase Kind (bug reproduction)", func() {
			// BUSINESS OUTCOME: This test proves the case mismatch bug exists.
			// The WFE stores "Deployment" (original casing from AIAnalysis) but the
			// RO reconciler lowercases it to "deployment" before passing to routing.
			// Field selectors are case-sensitive, so the query returns no results,
			// and cooldown is silently bypassed -- leading to redundant remediations.
			completionTime := metav1.Now()
			wfe := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-casing-mismatch",
					Namespace: "default",
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource: "demo-hpa/Deployment/api-frontend",
					WorkflowRef: workflowexecutionv1.WorkflowRef{
						WorkflowID: "patch-hpa-v1",
						Version:    "v1",
					},
				},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase:          workflowexecutionv1.PhaseCompleted,
					CompletionTime: &completionTime,
				},
			}
			Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-casing-mismatch",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "api-frontend",
						Namespace: "demo-hpa",
					},
				},
			}

			// Pass lowercase "deployment" (as the buggy reconciler does on line 955)
			blocked, err := engine.CheckRecentlyRemediated(ctx, rr, "patch-hpa-v1", "demo-hpa/deployment/api-frontend")

			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeNil(),
				"Bug #203: Lowercase 'deployment' does not match WFE's 'Deployment' -- cooldown bypassed")
		})
	})

	// ========================================
	// Test Group 5: CheckExponentialBackoff (3 tests)
	// Reference: DD-WE-004 (V1.0 Implementation)
	// TDD RED PHASE: Tests will FAIL until CheckExponentialBackoff is implemented
	// ========================================
	Context("CheckExponentialBackoff", func() {
		It("should block when exponential backoff active", func() {
			// Set NextAllowedExecution to future time (5 minutes from now)
			futureTime := metav1.NewTime(time.Now().Add(5 * time.Minute))
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-backoff-active",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "nginx-backoff",
						Namespace: "default",
					},
				},
				Status: remediationv1.RemediationRequestStatus{
					ConsecutiveFailureCount: 3,
					NextAllowedExecution:    &futureTime,
				},
			}

			// RED: Will panic("not implemented") until GREEN phase
			blocked := engine.CheckExponentialBackoff(ctx, rr)

			// Assertions (will FAIL until CheckExponentialBackoff is implemented)
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonExponentialBackoff)))
			Expect(blocked.Blocked).To(BeTrue())
			Expect(blocked.RequeueAfter).To(BeNumerically(">", 0))
			Expect(blocked.RequeueAfter).To(BeNumerically("<=", 5*time.Minute))
			Expect(blocked.Message).To(ContainSubstring("Exponential backoff active"))
			Expect(blocked.Message).To(ContainSubstring(futureTime.Format(time.RFC3339)))
		})

		It("should not block when no backoff configured", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-no-backoff",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "nginx-no-backoff",
						Namespace: "default",
					},
				},
				Status: remediationv1.RemediationRequestStatus{
					ConsecutiveFailureCount: 2,
					NextAllowedExecution:    nil, // No backoff configured
				},
			}

			// RED: Will panic("not implemented") until GREEN phase
			blocked := engine.CheckExponentialBackoff(ctx, rr)

			// Assertions (will FAIL until CheckExponentialBackoff is implemented)
			Expect(blocked).To(BeNil()) // Not blocked (no backoff)
		})

		It("should not block when backoff expired", func() {
			// Set NextAllowedExecution to past time (10 minutes ago)
			pastTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-backoff-expired",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "nginx-expired",
						Namespace: "default",
					},
				},
				Status: remediationv1.RemediationRequestStatus{
					ConsecutiveFailureCount: 5,
					NextAllowedExecution:    &pastTime, // Backoff expired
				},
			}

			// RED: Will panic("not implemented") until GREEN phase
			blocked := engine.CheckExponentialBackoff(ctx, rr)

			// Assertions (will FAIL until CheckExponentialBackoff is implemented)
			Expect(blocked).To(BeNil()) // Not blocked (backoff expired)
		})
	})

	// ========================================
	// Test Group 6: Split Routing API (3 tests)
	// Reference: DD-RO-002-ADDENDUM, Issue #165
	// CheckPreAnalysisConditions: fingerprint-based checks (pre-AA)
	// CheckPostAnalysisConditions: all checks including resource-level (post-AA)
	// ========================================
	Context("Split routing API (CheckPreAnalysisConditions / CheckPostAnalysisConditions)", func() {
		It("should check all conditions in priority order via CheckPostAnalysisConditions", func() {
			// Create 3 previous Failed RRs with same fingerprint (threshold = 3)
			// Set explicit UID because fake client doesn't auto-generate them
			baseTime := time.Now().Add(-10 * time.Minute)
			for i := 0; i < 3; i++ {
				failedRR := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:              fmt.Sprintf("failed-priority-rr-%d", i),
						Namespace:         "default",
						UID:               types.UID(fmt.Sprintf("failed-priority-uid-%d", i)),
						CreationTimestamp: metav1.Time{Time: baseTime.Add(time.Duration(i) * time.Minute)},
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: "priority-test-fp",
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: remediationv1.PhaseFailed,
					},
				}
				Expect(fakeClient.Create(ctx, failedRR)).To(Succeed())
			}

			// Create scenario where ConsecutiveFailures blocks (highest priority)
			// Set explicit UID different from the Failed RRs so it won't be skipped
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rr-priority-test",
					Namespace:         "default",
					UID:               types.UID("incoming-priority-uid"),
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "priority-test-fp",
				},
			}

			blocked, err := engine.CheckPostAnalysisConditions(ctx, rr, "", "")

			Expect(err).ToNot(HaveOccurred())
			Expect(blocked.Reason).To(Equal("ConsecutiveFailures")) // First check should win
		})

		It("should return first blocking condition found via CheckPreAnalysisConditions", func() {
			// Create scenario where DuplicateInProgress blocks (second check)
			// No consecutive failures (first check passes)
			originalRR := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-second-check-original",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "short-circuit-fp",
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseExecuting,
				},
			}
			Expect(fakeClient.Create(ctx, originalRR)).To(Succeed())

			duplicateRR := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-second-check-duplicate",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "short-circuit-fp",
				},
				Status: remediationv1.RemediationRequestStatus{
					ConsecutiveFailureCount: 0, // First check passes
				},
			}

			blocked, err := engine.CheckPreAnalysisConditions(ctx, duplicateRR)

			Expect(err).ToNot(HaveOccurred())
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonDuplicateInProgress))) // Second check should block
		})

		It("should return nil when no blocking condition found", func() {
			// Create RR with no blocking conditions
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-no-blocks",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "unique-no-block-fp",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "unique-target",
						Namespace: "default",
					},
				},
				Status: remediationv1.RemediationRequestStatus{
					ConsecutiveFailureCount: 0, // No consecutive failures
					OverallPhase:            remediationv1.PhasePending,
				},
			}

			blocked, err := engine.CheckPreAnalysisConditions(ctx, rr)

			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeNil()) // Can proceed to execution
		})
	})

	// ========================================
	// Test Group 7: Helper Functions (3 tests)
	// ========================================
	Context("IsTerminalPhase", func() {
		It("should return true for terminal phases", func() {
			terminalPhases := []remediationv1.RemediationPhase{
				remediationv1.PhaseCompleted,
				remediationv1.PhaseFailed,
				remediationv1.PhaseTimedOut,
				remediationv1.PhaseSkipped,
				remediationv1.PhaseCancelled,
			}

			for _, phase := range terminalPhases {
				Expect(routing.IsTerminalPhase(phase)).To(BeTrue(),
					"Expected %s to be terminal", phase)
			}
		})

		It("should return false for non-terminal phases", func() {
			nonTerminalPhases := []remediationv1.RemediationPhase{
				remediationv1.PhasePending,
				remediationv1.PhaseProcessing,
				remediationv1.PhaseAnalyzing,
				remediationv1.PhaseAwaitingApproval,
				remediationv1.PhaseExecuting,
				remediationv1.PhaseBlocked,
			}

			for _, phase := range nonTerminalPhases {
				Expect(routing.IsTerminalPhase(phase)).To(BeFalse(),
					"Expected %s to be non-terminal", phase)
			}
		})

		It("should handle empty phase string", func() {
			Expect(routing.IsTerminalPhase("")).To(BeFalse())
		})
	})

	// ========================================
	// Test Group 8: Edge Cases (10 tests)
	// Reference: Day 4 REFACTOR - robustness
	// ========================================
	Context("Edge Cases", func() {
		It("should not block when RR has empty SignalFingerprint", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-empty-fp",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "", // Empty fingerprint
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: "default",
					},
				},
			}
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			blocked, err := engine.CheckPreAnalysisConditions(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeNil()) // Not blocked (empty fingerprint doesn't match anything)
		})

		It("should not block when RR has empty TargetResource name", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-empty-target",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "test-fp",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "", // Empty name
						Namespace: "default",
					},
				},
			}
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			blocked, err := engine.CheckPostAnalysisConditions(ctx, rr, "", "")
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeNil()) // Not blocked (empty target doesn't match)
		})

		It("should handle cluster-scoped resources (no namespace)", func() {
			// Create WFE for cluster-scoped resource (e.g., Node)
			wfe := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-cluster-scoped",
					Namespace: "default",
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource: "node/worker-1", // No namespace (cluster-scoped)
					WorkflowRef: workflowexecutionv1.WorkflowRef{
						WorkflowID: "reboot-node",
					},
				},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1.PhaseRunning,
				},
			}
			Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

			// Create RR for same node
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-node",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					TargetResource: remediationv1.ResourceIdentifier{
						Kind: "Node",
						Name: "worker-1",
						// No namespace (cluster-scoped)
					},
				},
			}
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			blocked, err := engine.CheckResourceBusy(ctx, rr, "node/worker-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonResourceBusy)))
		})

		It("should not block when WFE CompletionTime is missing", func() {
			// Create completed WFE without CompletionTime
			wfe := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-no-completion",
					Namespace: "default",
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource: "default/pod/test-pod",
					WorkflowRef: workflowexecutionv1.WorkflowRef{
						WorkflowID: "restart-pod",
					},
				},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase:          workflowexecutionv1.PhaseCompleted,
					CompletionTime: nil, // Missing CompletionTime
				},
			}
			Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-missing-time",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: "default",
					},
					// Note: WorkflowRef not in RR.Spec (selected by AI later)
				},
			}
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			// Workflow ID from AIAnalysis.Status.SelectedWorkflow.WorkflowID
			blocked, err := engine.CheckRecentlyRemediated(ctx, rr, "restart-pod", "default/pod/test-pod")
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeNil()) // Not blocked (no CompletionTime = skip)
		})

		It("should handle very old WFE (outside cooldown window)", func() {
			// Create old WFE (1 hour ago, well outside 5 min cooldown)
			oldTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
			wfe := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-very-old",
					Namespace: "default",
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource: "default/pod/test-pod-old",
					WorkflowRef: workflowexecutionv1.WorkflowRef{
						WorkflowID: "restart-pod",
					},
				},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase:          workflowexecutionv1.PhaseCompleted,
					CompletionTime: &oldTime,
				},
			}
			Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-old-cooldown",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod-old",
						Namespace: "default",
					},
					// Note: WorkflowRef not in RR.Spec (selected by AI later)
				},
			}
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			// Same workflow ID but outside cooldown window
			blocked, err := engine.CheckRecentlyRemediated(ctx, rr, "restart-pod", "default/pod/test-pod-old")
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeNil()) // Not blocked (cooldown expired)
		})

		It("should handle ConsecutiveFailureCount at exactly threshold boundary", func() {
			// Create 3 previous Failed RRs with same fingerprint (exactly at threshold)
			// Set explicit UID because fake client doesn't auto-generate them
			baseTime := time.Now().Add(-10 * time.Minute)
			for i := 0; i < 3; i++ {
				failedRR := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:              fmt.Sprintf("failed-threshold-rr-%d", i),
						Namespace:         "default",
						UID:               types.UID(fmt.Sprintf("failed-threshold-uid-%d", i)),
						CreationTimestamp: metav1.Time{Time: baseTime.Add(time.Duration(i) * time.Minute)},
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: "threshold-fp",
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: remediationv1.PhaseFailed,
					},
				}
				Expect(fakeClient.Create(ctx, failedRR)).To(Succeed())
			}

			// Create incoming RR with different UID so it won't be skipped
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rr-at-threshold",
					Namespace:         "default",
					UID:               types.UID("incoming-threshold-uid"),
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "threshold-fp",
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseAnalyzing,
				},
			}

			blocked := engine.CheckConsecutiveFailures(ctx, rr)
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonConsecutiveFailures)))
		})

		It("should handle ConsecutiveFailureCount just below threshold", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-below-threshold",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "below-threshold-fp",
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase:            remediationv1.PhaseAnalyzing,
					ConsecutiveFailureCount: 2, // Just below threshold
				},
			}
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			blocked := engine.CheckConsecutiveFailures(ctx, rr)
			Expect(blocked).To(BeNil()) // Should not block below threshold
		})

		It("should handle multiple WFEs on same target (return first Running)", func() {
			// Create multiple WFEs on same target
			wfe1 := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-multi-1",
					Namespace: "default",
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource: "default/pod/multi-target",
					WorkflowRef: workflowexecutionv1.WorkflowRef{
						WorkflowID: "workflow-a",
					},
				},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1.PhaseCompleted, // Terminal
				},
			}
			Expect(fakeClient.Create(ctx, wfe1)).To(Succeed())

			wfe2 := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-multi-2",
					Namespace: "default",
				},
				Spec: workflowexecutionv1.WorkflowExecutionSpec{
					TargetResource: "default/pod/multi-target",
					WorkflowRef: workflowexecutionv1.WorkflowRef{
						WorkflowID: "workflow-b",
					},
				},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1.PhaseRunning, // Active
				},
			}
			Expect(fakeClient.Create(ctx, wfe2)).To(Succeed())

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-multi-wfe",
					Namespace: "default",
				},
				Spec: remediationv1.RemediationRequestSpec{
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "multi-target",
						Namespace: "default",
					},
				},
			}
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			blocked, err := engine.CheckResourceBusy(ctx, rr, "default/pod/multi-target")
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked.BlockingWorkflowExecution).To(Equal("wfe-multi-2"))
		})

		It("should handle RR with ConsecutiveFailureCount > threshold", func() {
			// Create 10 previous Failed RRs with same fingerprint (way above threshold of 3)
			// Set explicit UID because fake client doesn't auto-generate them
			baseTime := time.Now().Add(-30 * time.Minute)
			for i := 0; i < 10; i++ {
				failedRR := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:              fmt.Sprintf("failed-above-rr-%d", i),
						Namespace:         "default",
						UID:               types.UID(fmt.Sprintf("failed-above-uid-%d", i)),
						CreationTimestamp: metav1.Time{Time: baseTime.Add(time.Duration(i) * time.Minute)},
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: "above-threshold-fp",
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: remediationv1.PhaseFailed,
					},
				}
				Expect(fakeClient.Create(ctx, failedRR)).To(Succeed())
			}

			// Create incoming RR with different UID so it won't be skipped
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rr-above-threshold",
					Namespace:         "default",
					UID:               types.UID("incoming-above-uid"),
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "above-threshold-fp",
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseAnalyzing,
				},
			}

			blocked := engine.CheckConsecutiveFailures(ctx, rr)
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonConsecutiveFailures)))
			Expect(blocked.Message).To(ContainSubstring("10 consecutive failures"))
		})

		It("should handle priority order: ConsecutiveFailures > DuplicateInProgress", func() {
			// Create 3 previous Failed RRs with same fingerprint (for ConsecutiveFailures check)
			// Set explicit UID because fake client doesn't auto-generate them
			baseTime := time.Now().Add(-15 * time.Minute)
			for i := 0; i < 3; i++ {
				failedRR := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:              fmt.Sprintf("failed-priority-combo-rr-%d", i),
						Namespace:         "default",
						UID:               types.UID(fmt.Sprintf("failed-combo-uid-%d", i)),
						CreationTimestamp: metav1.Time{Time: baseTime.Add(time.Duration(i) * time.Minute)},
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: "priority-test-fp",
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: remediationv1.PhaseFailed,
					},
				}
				Expect(fakeClient.Create(ctx, failedRR)).To(Succeed())
			}

			// Create original active RR (for DuplicateInProgress check)
			originalRR := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rr-original-priority",
					Namespace:         "default",
					UID:               types.UID("original-priority-uid"),
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "priority-test-fp",
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseExecuting,
				},
			}
			Expect(fakeClient.Create(ctx, originalRR)).To(Succeed())

			// Create incoming RR with BOTH ConsecutiveFailures and DuplicateInProgress conditions
			// Set explicit UID different from all other RRs so it won't be skipped
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rr-priority-test",
					Namespace:         "default",
					UID:               types.UID("incoming-combo-uid"),
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "priority-test-fp", // Same fingerprint (duplicate)
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseAnalyzing,
				},
			}

			// Check blocking conditions (post-AA includes all checks)
			blocked, err := engine.CheckPostAnalysisConditions(ctx, rr, "", "")
			Expect(err).ToNot(HaveOccurred())
			// Should return ConsecutiveFailures (higher priority than DuplicateInProgress)
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonConsecutiveFailures)))
		})
	})
})
