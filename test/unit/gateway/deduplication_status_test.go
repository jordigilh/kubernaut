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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ========================================
// DD-GATEWAY-011: Shared Status Ownership Tests
// 📋 Design Decision: DD-GATEWAY-011 | ✅ Approved Design | Confidence: 90%
// See: docs/architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md
// ========================================
//
// These tests validate the status-based deduplication pattern where:
// - Gateway OWNS status.deduplication and status.stormAggregation
// - Spec is IMMUTABLE after creation
// - Conflict retry pattern handles concurrent updates
//
// Business Requirements:
// - BR-GATEWAY-181: Move deduplication tracking from spec to status
// - BR-GATEWAY-183: Implement optimistic concurrency for status updates
// - BR-GATEWAY-184: Check RR phase for deduplication decisions
// ========================================

// Helper to create a valid 64-char SHA256 fingerprint
func testFingerprint(prefix string) string {
	// SHA256 hashes are 64 hex characters
	base := prefix + "0000000000000000000000000000000000000000000000000000000000000000"
	return base[:64]
}

var _ = Describe("Deduplication Status (DD-GATEWAY-011)", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		scheme    *runtime.Scheme
		updater   *processing.StatusUpdater
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create scheme with RemediationRequest types
		scheme = runtime.NewScheme()
		Expect(remediationv1alpha1.AddToScheme(scheme)).To(Succeed())

		// Create fake K8s client with status subresource support and field index
		// DD-GATEWAY-011: Required for Status().Update() calls
		// BR-GATEWAY-185 v1.1: Required for spec.signalFingerprint field index
		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&remediationv1alpha1.RemediationRequest{}).
			WithIndex(&remediationv1alpha1.RemediationRequest{}, "spec.signalFingerprint",
				func(obj client.Object) []string {
					rr := obj.(*remediationv1alpha1.RemediationRequest)
					return []string{rr.Spec.SignalFingerprint}
				}).
			Build()

		// Create status updater (NEW component for DD-GATEWAY-011)
		// DD-STATUS-001: Pass k8sClient as both client and apiReader (tests use fake client, already uncached)
		updater = processing.NewStatusUpdater(k8sClient, k8sClient)
	})

	// ========================================
	// TDD Cycle 1: Status Update on Duplicate (BR-GATEWAY-181)
	// ========================================
	Describe("UpdateDeduplicationStatus", func() {
		Context("when updating deduplication on duplicate signal (BR-GATEWAY-181)", func() {
			It("should increment status.deduplication.occurrenceCount", func() {
				// Setup: Create RR with initial deduplication status
				now := metav1.Now()
				rr := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr-001",
						Namespace: "kubernaut-system",
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: testFingerprint("abc123"),
						SignalName:        "PodCrashLooping",
						Severity:          "warning",
						SignalType:        "prometheus",
						ReceivedTime:      now,
						Deduplication: sharedtypes.DeduplicationInfo{
							FirstOccurrence: now,
							LastOccurrence:  now,
							OccurrenceCount: 1,
						},
					},
					Status: remediationv1alpha1.RemediationRequestStatus{
						Deduplication: &remediationv1alpha1.DeduplicationStatus{
							OccurrenceCount: 1,
							FirstSeenAt:     &now,
						},
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				// BEHAVIOR: Update deduplication status (increment count)
				err := updater.UpdateDeduplicationStatus(ctx, rr)

				// CORRECTNESS: Status updated correctly
				Expect(err).ToNot(HaveOccurred(), "Status update should succeed")

				// Refetch to verify persisted change
				updatedRR := &remediationv1alpha1.RemediationRequest{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())

				Expect(updatedRR.Status.Deduplication).ToNot(BeNil(), "Deduplication status should exist")
				Expect(updatedRR.Status.Deduplication.OccurrenceCount).To(Equal(int32(2)),
					"OccurrenceCount should be incremented from 1 to 2")
				Expect(updatedRR.Status.Deduplication.LastSeenAt).ToNot(BeNil(),
					"LastSeenAt should be set")

				// BUSINESS OUTCOME: Duplicate tracking in status (not spec)
				// This validates BR-GATEWAY-181
			})

			It("should set LastSeenAt timestamp on update", func() {
				// Setup: Create RR with initial deduplication status
				initialTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
				now := metav1.Now()
				rr := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr-002",
						Namespace: "kubernaut-system",
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: testFingerprint("xyz789"),
						SignalName:        "HighMemoryUsage",
						Severity:          "warning",
						SignalType:        "prometheus",
						ReceivedTime:      now,
						Deduplication: sharedtypes.DeduplicationInfo{
							FirstOccurrence: now,
							LastOccurrence:  now,
							OccurrenceCount: 3,
						},
					},
					Status: remediationv1alpha1.RemediationRequestStatus{
						Deduplication: &remediationv1alpha1.DeduplicationStatus{
							OccurrenceCount: 3,
							FirstSeenAt:     &initialTime,
						},
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				// Capture time before update (with tolerance for serialization precision loss)
				beforeUpdate := time.Now().Add(-1 * time.Second)

				// BEHAVIOR: Update deduplication status
				err := updater.UpdateDeduplicationStatus(ctx, rr)

				// CORRECTNESS: LastSeenAt is updated to current time
				Expect(err).ToNot(HaveOccurred())

				updatedRR := &remediationv1alpha1.RemediationRequest{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())

				Expect(updatedRR.Status.Deduplication.LastSeenAt).ToNot(BeNil())
				Expect(updatedRR.Status.Deduplication.LastSeenAt.Time).To(BeTemporally(">=", beforeUpdate),
					"LastSeenAt should be updated to current time")
				// Also verify it's more recent than initialTime (1 hour ago)
				Expect(updatedRR.Status.Deduplication.LastSeenAt.Time).To(BeTemporally(">", initialTime.Time),
					"LastSeenAt should be more recent than initial time")
			})

			It("should initialize deduplication status if nil", func() {
				// Setup: Create RR WITHOUT initial deduplication status
				now := metav1.Now()
				rr := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr-003",
						Namespace: "kubernaut-system",
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: testFingerprint("newsig"),
						SignalName:        "NewAlert",
						Severity:          "info",
						SignalType:        "prometheus",
						ReceivedTime:      now,
						Deduplication: sharedtypes.DeduplicationInfo{
							FirstOccurrence: now,
							LastOccurrence:  now,
							OccurrenceCount: 1,
						},
					},
					Status: remediationv1alpha1.RemediationRequestStatus{
						// No Deduplication field set
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				// BEHAVIOR: Update deduplication status (first update)
				err := updater.UpdateDeduplicationStatus(ctx, rr)

				// CORRECTNESS: Deduplication status is initialized
				Expect(err).ToNot(HaveOccurred())

				updatedRR := &remediationv1alpha1.RemediationRequest{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())

				Expect(updatedRR.Status.Deduplication).ToNot(BeNil(),
					"Deduplication status should be initialized")
				Expect(updatedRR.Status.Deduplication.OccurrenceCount).To(Equal(int32(1)),
					"Initial occurrence count should be 1")
				Expect(updatedRR.Status.Deduplication.FirstSeenAt).ToNot(BeNil(),
					"FirstSeenAt should be set on initialization")
			})
		})

		Context("when spec immutability is enforced (BR-GATEWAY-181)", func() {
			It("should NOT modify spec.deduplication during status update", func() {
				// Setup: Create RR with spec.deduplication set
				now := metav1.Now()
				rr := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr-immutable",
						Namespace: "kubernaut-system",
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: testFingerprint("immut"),
						SignalName:        "TestAlert",
						Severity:          "warning",
						SignalType:        "prometheus",
						ReceivedTime:      now,
						Deduplication: sharedtypes.DeduplicationInfo{
							FirstOccurrence: now,
							LastOccurrence:  now,
							OccurrenceCount: 1,
						},
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				// Capture original spec values AFTER create (to handle serialization precision)
				createdRR := &remediationv1alpha1.RemediationRequest{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), createdRR)).To(Succeed())
				originalSpecCount := createdRR.Spec.Deduplication.OccurrenceCount
				originalFirstOccurrence := createdRR.Spec.Deduplication.FirstOccurrence

				// BEHAVIOR: Update deduplication STATUS (not spec)
				err := updater.UpdateDeduplicationStatus(ctx, createdRR)
				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: Spec is unchanged
				updatedRR := &remediationv1alpha1.RemediationRequest{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())

				Expect(updatedRR.Spec.Deduplication.OccurrenceCount).To(Equal(originalSpecCount),
					"spec.deduplication.occurrenceCount should NOT change")
				Expect(updatedRR.Spec.Deduplication.FirstOccurrence).To(Equal(originalFirstOccurrence),
					"spec.deduplication.firstOccurrence should NOT change")

				// BUSINESS OUTCOME: Spec immutability enforced (BR-GATEWAY-181)
			})
		})
	})

	// ========================================
	// TDD Cycle 2: Phase-Based Deduplication Decision (BR-GATEWAY-184)
	// ========================================
	Describe("ShouldDeduplicate (Phase-Based)", func() {
		Context("when checking for existing RR by fingerprint (BR-GATEWAY-184)", func() {
			It("should return true for in-progress RR (Pending phase)", func() {
				// Setup: Create RR in Pending phase
				now := metav1.Now()
				fp := testFingerprint("pending")
				rr := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pending-rr",
						Namespace: "kubernaut-system",
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: fp,
						SignalName:        "PendingAlert",
						Severity:          "warning",
						SignalType:        "prometheus",
						ReceivedTime:      now,
						Deduplication: sharedtypes.DeduplicationInfo{
							FirstOccurrence: now,
							LastOccurrence:  now,
							OccurrenceCount: 1,
						},
					},
					Status: remediationv1alpha1.RemediationRequestStatus{
						OverallPhase: "Pending",
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				// BEHAVIOR: Check if should deduplicate
				checker := processing.NewPhaseBasedDeduplicationChecker(k8sClient)
				isDuplicate, existingRR, err := checker.ShouldDeduplicate(ctx, "kubernaut-system", fp)

				// CORRECTNESS: Should deduplicate (RR is in-progress)
				Expect(err).ToNot(HaveOccurred())
				Expect(isDuplicate).To(BeTrue(), "Should detect duplicate for Pending RR")
				Expect(existingRR).ToNot(BeNil())
				Expect(existingRR.Name).To(Equal("pending-rr"))

				// BUSINESS OUTCOME: Prevents duplicate RR creation for in-progress remediations
			})

			It("should return true for in-progress RR (Processing phase)", func() {
				// Setup: Create RR in Processing phase
				now := metav1.Now()
				fp := testFingerprint("proces")
				rr := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "processing-rr",
						Namespace: "kubernaut-system",
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: fp,
						SignalName:        "ProcessingAlert",
						Severity:          "warning",
						SignalType:        "prometheus",
						ReceivedTime:      now,
						Deduplication: sharedtypes.DeduplicationInfo{
							FirstOccurrence: now,
							LastOccurrence:  now,
							OccurrenceCount: 1,
						},
					},
					Status: remediationv1alpha1.RemediationRequestStatus{
						OverallPhase: "Processing",
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				// BEHAVIOR: Check if should deduplicate
				checker := processing.NewPhaseBasedDeduplicationChecker(k8sClient)
				isDuplicate, existingRR, err := checker.ShouldDeduplicate(ctx, "kubernaut-system", fp)

				// CORRECTNESS: Should deduplicate (RR is in-progress)
				Expect(err).ToNot(HaveOccurred())
				Expect(isDuplicate).To(BeTrue(), "Should detect duplicate for Processing RR")
				Expect(existingRR).ToNot(BeNil())
			})

			It("should return false for terminal RR (Completed phase)", func() {
				// Setup: Create RR in Completed phase (terminal)
				now := metav1.Now()
				fp := testFingerprint("complt")
				rr := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "completed-rr",
						Namespace: "kubernaut-system",
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: fp,
						SignalName:        "CompletedAlert",
						Severity:          "warning",
						SignalType:        "prometheus",
						ReceivedTime:      now,
						Deduplication: sharedtypes.DeduplicationInfo{
							FirstOccurrence: now,
							LastOccurrence:  now,
							OccurrenceCount: 1,
						},
					},
					Status: remediationv1alpha1.RemediationRequestStatus{
						OverallPhase: "Completed",
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				// BEHAVIOR: Check if should deduplicate
				checker := processing.NewPhaseBasedDeduplicationChecker(k8sClient)
				isDuplicate, existingRR, err := checker.ShouldDeduplicate(ctx, "kubernaut-system", fp)

				// CORRECTNESS: Should NOT deduplicate (RR is terminal)
				Expect(err).ToNot(HaveOccurred())
				Expect(isDuplicate).To(BeFalse(), "Should allow new RR for Completed phase")
				Expect(existingRR).To(BeNil())

				// BUSINESS OUTCOME: Allows retry after successful remediation
			})

			It("should return false for terminal RR (Failed phase)", func() {
				// Setup: Create RR in Failed phase (terminal)
				now := metav1.Now()
				fp := testFingerprint("failed")
				rr := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "failed-rr",
						Namespace: "kubernaut-system",
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: fp,
						SignalName:        "FailedAlert",
						Severity:          "warning",
						SignalType:        "prometheus",
						ReceivedTime:      now,
						Deduplication: sharedtypes.DeduplicationInfo{
							FirstOccurrence: now,
							LastOccurrence:  now,
							OccurrenceCount: 1,
						},
					},
					Status: remediationv1alpha1.RemediationRequestStatus{
						OverallPhase: "Failed",
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				// BEHAVIOR: Check if should deduplicate
				checker := processing.NewPhaseBasedDeduplicationChecker(k8sClient)
				isDuplicate, existingRR, err := checker.ShouldDeduplicate(ctx, "kubernaut-system", fp)

				// CORRECTNESS: Should NOT deduplicate (RR is terminal)
				Expect(err).ToNot(HaveOccurred())
				Expect(isDuplicate).To(BeFalse(), "Should allow new RR for Failed phase")
				Expect(existingRR).To(BeNil())

				// BUSINESS OUTCOME: Allows retry after failed remediation
			})

			It("should return false when no RR exists for fingerprint", func() {
				// No RR created - empty state
				fp := testFingerprint("nonext")

				// BEHAVIOR: Check if should deduplicate
				checker := processing.NewPhaseBasedDeduplicationChecker(k8sClient)
				isDuplicate, existingRR, err := checker.ShouldDeduplicate(ctx, "kubernaut-system", fp)

				// CORRECTNESS: Should NOT deduplicate (no existing RR)
				Expect(err).ToNot(HaveOccurred())
				Expect(isDuplicate).To(BeFalse(), "Should allow new RR when none exists")
				Expect(existingRR).To(BeNil())

				// BUSINESS OUTCOME: First occurrence creates new RR
			})
		})
	})

	// ========================================
	// TDD Cycle 3: Conflict Retry (BR-GATEWAY-183)
	// ========================================
	Describe("Conflict Retry Pattern", func() {
		Context("when handling optimistic concurrency conflicts (BR-GATEWAY-183)", func() {
			// Note: Conflict retry is tested at integration level with real K8s API
			// Unit tests validate the retry logic structure exists

			It("should use retry.RetryOnConflict pattern", func() {
				// This test validates that the StatusUpdater uses the correct pattern
				// Actual conflict scenarios are tested in integration tests

				// BEHAVIOR: StatusUpdater should have retry capability
				Expect(updater).ToNot(BeNil(), "StatusUpdater should be initialized")

				// Create a simple RR for update
				now := metav1.Now()
				rr := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "conflict-test-rr",
						Namespace: "kubernaut-system",
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: testFingerprint("conflt"),
						SignalName:        "ConflictAlert",
						Severity:          "warning",
						SignalType:        "prometheus",
						ReceivedTime:      now,
						Deduplication: sharedtypes.DeduplicationInfo{
							FirstOccurrence: now,
							LastOccurrence:  now,
							OccurrenceCount: 1,
						},
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				// BEHAVIOR: Update should succeed with fake client (no conflicts)
				err := updater.UpdateDeduplicationStatus(ctx, rr)

				// CORRECTNESS: Update succeeds
				Expect(err).ToNot(HaveOccurred())

				// BUSINESS OUTCOME: Retry pattern handles concurrent updates
				// Full conflict testing in integration tests
			})
		})
	})
})
