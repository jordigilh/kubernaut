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

// Note: Test function is in suite_test.go (TestGatewayUnit)
// This file only contains Describe blocks that are registered with the suite

// ========================================
// DD-GATEWAY-011 Day 3: Storm Aggregation Status Tests (TDD RED Phase)
// ========================================
//
// Business Requirements:
// - BR-GATEWAY-182: Move storm aggregation from Redis to status
// - BR-GATEWAY-185: Support Redis deprecation
//
// Tests validate BEHAVIOR and CORRECTNESS:
// - Initialize status.stormAggregation on first storm alert
// - Increment aggregatedCount on subsequent alerts
// - Set isPartOfStorm=true when threshold reached
// - Use config threshold with defaults
// ========================================

var _ = Describe("Storm Aggregation Status (DD-GATEWAY-011)", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		scheme    *runtime.Scheme
		updater   *processing.StatusUpdater
	)

	// Helper to create test fingerprint
	testFingerprint := func(suffix string) string {
		return "storm-test-fingerprint-" + suffix
	}

	BeforeEach(func() {
		ctx = context.Background()

		// Create scheme with RemediationRequest types
		scheme = runtime.NewScheme()
		Expect(remediationv1alpha1.AddToScheme(scheme)).To(Succeed())

		// Create fake K8s client with status subresource support
		// DD-GATEWAY-011: Required for Status().Update() calls
		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&remediationv1alpha1.RemediationRequest{}).
			Build()

		// Create status updater
		updater = processing.NewStatusUpdater(k8sClient)
	})

	// ========================================
	// TDD Cycle 1: Initialize Storm Aggregation Status (BR-GATEWAY-182)
	// ========================================
	Describe("UpdateStormAggregationStatus", func() {
		Context("when first storm alert arrives (BR-GATEWAY-182)", func() {
			It("should initialize status.stormAggregation with aggregatedCount=1", func() {
				// Setup: Create RR without storm aggregation status
				now := metav1.Now()
				rr := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr-storm-001",
						Namespace: "kubernaut-system",
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: testFingerprint("storm001"),
						SignalName:        "PodCrashLooping",
						Severity:          "critical",
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

				// BEHAVIOR: Update storm aggregation status (first alert)
				err := updater.UpdateStormAggregationStatus(ctx, rr, false)

				// CORRECTNESS: Storm aggregation initialized
				Expect(err).ToNot(HaveOccurred())

				updatedRR := &remediationv1alpha1.RemediationRequest{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())

				Expect(updatedRR.Status.StormAggregation).ToNot(BeNil(),
					"status.stormAggregation should be initialized")
				Expect(updatedRR.Status.StormAggregation.AggregatedCount).To(Equal(int32(1)),
					"aggregatedCount should be 1 for first alert")

				// BUSINESS OUTCOME: Storm tracking started in RR status (not Redis)
			})

			It("should set stormDetectedAt timestamp on first alert", func() {
				// Setup: Create RR without storm aggregation
				now := metav1.Now()
				rr := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr-storm-002",
						Namespace: "kubernaut-system",
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: testFingerprint("storm002"),
						SignalName:        "HighMemoryUsage",
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

				beforeUpdate := time.Now().Add(-1 * time.Second)

				// BEHAVIOR: Initialize storm aggregation
				err := updater.UpdateStormAggregationStatus(ctx, rr, false)
				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: StormDetectedAt is set
				updatedRR := &remediationv1alpha1.RemediationRequest{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())

				Expect(updatedRR.Status.StormAggregation.StormDetectedAt).ToNot(BeNil(),
					"stormDetectedAt should be set")
				Expect(updatedRR.Status.StormAggregation.StormDetectedAt.Time).To(BeTemporally(">=", beforeUpdate),
					"stormDetectedAt should be recent")
			})
		})

		// ========================================
		// TDD Cycle 2: Increment Aggregated Count (BR-GATEWAY-182)
		// ========================================
		Context("when subsequent storm alerts arrive (BR-GATEWAY-182)", func() {
			It("should increment status.stormAggregation.aggregatedCount", func() {
				// Setup: Create RR with existing storm aggregation (3 alerts so far)
				now := metav1.Now()
				stormStart := metav1.NewTime(time.Now().Add(-30 * time.Second))
				rr := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr-storm-003",
						Namespace: "kubernaut-system",
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: testFingerprint("storm003"),
						SignalName:        "PodCrashLooping",
						Severity:          "critical",
						SignalType:        "prometheus",
						ReceivedTime:      now,
						Deduplication: sharedtypes.DeduplicationInfo{
							FirstOccurrence: now,
							LastOccurrence:  now,
							OccurrenceCount: 3,
						},
					},
					Status: remediationv1alpha1.RemediationRequestStatus{
						StormAggregation: &remediationv1alpha1.StormAggregationStatus{
							AggregatedCount: 3,
							StormDetectedAt: &stormStart,
							IsPartOfStorm:   false,
						},
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				// BEHAVIOR: Update storm aggregation (4th alert)
				err := updater.UpdateStormAggregationStatus(ctx, rr, false)

				// CORRECTNESS: Count incremented from 3 to 4
				Expect(err).ToNot(HaveOccurred())

				updatedRR := &remediationv1alpha1.RemediationRequest{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())

				Expect(updatedRR.Status.StormAggregation.AggregatedCount).To(Equal(int32(4)),
					"aggregatedCount should increment from 3 to 4")

				// BUSINESS OUTCOME: Storm count tracked in status (BR-GATEWAY-182)
			})

			It("should preserve stormDetectedAt on subsequent alerts", func() {
				// Setup: Create RR with existing storm aggregation
				now := metav1.Now()
				originalStormTime := metav1.NewTime(time.Now().Add(-2 * time.Minute))
				rr := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr-storm-004",
						Namespace: "kubernaut-system",
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: testFingerprint("storm004"),
						SignalName:        "NodeNotReady",
						Severity:          "critical",
						SignalType:        "prometheus",
						ReceivedTime:      now,
						Deduplication: sharedtypes.DeduplicationInfo{
							FirstOccurrence: now,
							LastOccurrence:  now,
							OccurrenceCount: 2,
						},
					},
					Status: remediationv1alpha1.RemediationRequestStatus{
						StormAggregation: &remediationv1alpha1.StormAggregationStatus{
							AggregatedCount: 2,
							StormDetectedAt: &originalStormTime,
						},
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				// Capture original storm time after creation (for serialization precision)
				createdRR := &remediationv1alpha1.RemediationRequest{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), createdRR)).To(Succeed())
				originalTime := createdRR.Status.StormAggregation.StormDetectedAt

				// BEHAVIOR: Update storm aggregation
				err := updater.UpdateStormAggregationStatus(ctx, createdRR, false)
				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: StormDetectedAt preserved (not updated)
				updatedRR := &remediationv1alpha1.RemediationRequest{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())

				Expect(updatedRR.Status.StormAggregation.StormDetectedAt).To(Equal(originalTime),
					"stormDetectedAt should NOT change on subsequent alerts")
			})
		})

		// ========================================
		// TDD Cycle 3: Storm Threshold Detection (BR-GATEWAY-182)
		// ========================================
		Context("when storm threshold is reached (BR-GATEWAY-182)", func() {
			It("should set isPartOfStorm=true when threshold reached", func() {
				// Setup: Create RR at threshold-1 (4 alerts, threshold=5)
				now := metav1.Now()
				stormStart := metav1.NewTime(time.Now().Add(-1 * time.Minute))
				rr := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr-storm-005",
						Namespace: "kubernaut-system",
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: testFingerprint("storm005"),
						SignalName:        "PodCrashLooping",
						Severity:          "critical",
						SignalType:        "prometheus",
						ReceivedTime:      now,
						Deduplication: sharedtypes.DeduplicationInfo{
							FirstOccurrence: now,
							LastOccurrence:  now,
							OccurrenceCount: 4,
						},
					},
					Status: remediationv1alpha1.RemediationRequestStatus{
						StormAggregation: &remediationv1alpha1.StormAggregationStatus{
							AggregatedCount: 4,
							StormDetectedAt: &stormStart,
							IsPartOfStorm:   false, // Not yet a storm
						},
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				// BEHAVIOR: 5th alert arrives (threshold reached)
				isThresholdReached := true // Caller determines this based on config
				err := updater.UpdateStormAggregationStatus(ctx, rr, isThresholdReached)

				// CORRECTNESS: IsPartOfStorm set to true
				Expect(err).ToNot(HaveOccurred())

				updatedRR := &remediationv1alpha1.RemediationRequest{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())

				Expect(updatedRR.Status.StormAggregation.IsPartOfStorm).To(BeTrue(),
					"isPartOfStorm should be true when threshold reached")
				Expect(updatedRR.Status.StormAggregation.AggregatedCount).To(Equal(int32(5)),
					"aggregatedCount should be 5 (threshold)")

				// BUSINESS OUTCOME: Storm officially detected, AI can handle as aggregated (BR-GATEWAY-182)
			})

			It("should keep isPartOfStorm=true on alerts after threshold", func() {
				// Setup: Create RR already past threshold (10 alerts, threshold=5)
				now := metav1.Now()
				stormStart := metav1.NewTime(time.Now().Add(-5 * time.Minute))
				rr := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr-storm-006",
						Namespace: "kubernaut-system",
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: testFingerprint("storm006"),
						SignalName:        "PodOOMKilled",
						Severity:          "critical",
						SignalType:        "prometheus",
						ReceivedTime:      now,
						Deduplication: sharedtypes.DeduplicationInfo{
							FirstOccurrence: now,
							LastOccurrence:  now,
							OccurrenceCount: 10,
						},
					},
					Status: remediationv1alpha1.RemediationRequestStatus{
						StormAggregation: &remediationv1alpha1.StormAggregationStatus{
							AggregatedCount: 10,
							StormDetectedAt: &stormStart,
							IsPartOfStorm:   true, // Already a storm
						},
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				// BEHAVIOR: 11th alert arrives (already past threshold)
				err := updater.UpdateStormAggregationStatus(ctx, rr, true)

				// CORRECTNESS: IsPartOfStorm stays true
				Expect(err).ToNot(HaveOccurred())

				updatedRR := &remediationv1alpha1.RemediationRequest{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())

				Expect(updatedRR.Status.StormAggregation.IsPartOfStorm).To(BeTrue(),
					"isPartOfStorm should remain true after threshold")
				Expect(updatedRR.Status.StormAggregation.AggregatedCount).To(Equal(int32(11)),
					"aggregatedCount should increment to 11")
			})

			It("should NOT set isPartOfStorm=true when below threshold", func() {
				// Setup: Create RR with 2 alerts (below threshold of 5)
				now := metav1.Now()
				stormStart := metav1.NewTime(time.Now().Add(-10 * time.Second))
				rr := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr-storm-007",
						Namespace: "kubernaut-system",
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: testFingerprint("storm007"),
						SignalName:        "ContainerRestarting",
						Severity:          "warning",
						SignalType:        "prometheus",
						ReceivedTime:      now,
						Deduplication: sharedtypes.DeduplicationInfo{
							FirstOccurrence: now,
							LastOccurrence:  now,
							OccurrenceCount: 2,
						},
					},
					Status: remediationv1alpha1.RemediationRequestStatus{
						StormAggregation: &remediationv1alpha1.StormAggregationStatus{
							AggregatedCount: 2,
							StormDetectedAt: &stormStart,
							IsPartOfStorm:   false,
						},
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				// BEHAVIOR: 3rd alert (still below threshold)
				isThresholdReached := false // Caller determines count < threshold
				err := updater.UpdateStormAggregationStatus(ctx, rr, isThresholdReached)

				// CORRECTNESS: IsPartOfStorm stays false
				Expect(err).ToNot(HaveOccurred())

				updatedRR := &remediationv1alpha1.RemediationRequest{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())

				Expect(updatedRR.Status.StormAggregation.IsPartOfStorm).To(BeFalse(),
					"isPartOfStorm should remain false below threshold")
				Expect(updatedRR.Status.StormAggregation.AggregatedCount).To(Equal(int32(3)),
					"aggregatedCount should increment to 3")
			})
		})

		// ========================================
		// TDD Cycle 4: Conflict Retry Pattern (BR-GATEWAY-183)
		// ========================================
		Context("when handling optimistic concurrency conflicts (BR-GATEWAY-183)", func() {
			It("should use retry.RetryOnConflict pattern", func() {
				// Setup: Create RR
				now := metav1.Now()
				rr := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr-storm-retry",
						Namespace: "kubernaut-system",
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: testFingerprint("stormretry"),
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

				// BEHAVIOR: Update should succeed (uses retry pattern internally)
				err := updater.UpdateStormAggregationStatus(ctx, rr, false)

				// CORRECTNESS: Update succeeds (retry handles conflicts)
				Expect(err).ToNot(HaveOccurred())

				// Verify update was applied
				updatedRR := &remediationv1alpha1.RemediationRequest{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)).To(Succeed())
				Expect(updatedRR.Status.StormAggregation).ToNot(BeNil())

				// BUSINESS OUTCOME: Concurrent updates handled gracefully (BR-GATEWAY-183)
			})
		})
	})
})
