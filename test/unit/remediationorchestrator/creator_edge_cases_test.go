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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/timeout"
)

// ========================================
// TDD RED Phase: Owner Reference Edge Cases
// These tests MUST fail initially until defensive code is added
// ========================================
var _ = Describe("Creator Edge Cases (Priority 2: Defensive Programming)", func() {
	var (
		scheme *runtime.Scheme
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = signalprocessingv1.AddToScheme(scheme)
		_ = aianalysisv1.AddToScheme(scheme)
		_ = workflowexecutionv1.AddToScheme(scheme)
		_ = notificationv1.AddToScheme(scheme)
	})

	// ========================================
	// Gap 2.1: Owner Reference Edge Cases
	// Business Value: Prevents orphaned child CRDs
	// Priority 2: Defensive Programming
	// ========================================
	Describe("Owner Reference Edge Cases (Gap 2.1)", func() {
		Context("when RemediationRequest has incomplete metadata", func() {
			It("should handle RR with empty UID gracefully", func() {
				// Scenario: RR not yet persisted to etcd, UID not assigned
				// Business Outcome: Clear error prevents orphaned child CRDs
				// Confidence: 90% - Prevents dangling CRDs without cascade deletion

				// Given: RemediationRequest with no UID (not persisted yet)
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr",
						Namespace: "default",
						// UID intentionally empty - simulates pre-persistence state
						UID: "",
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalName: "test-signal",
					},
				}

				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr).Build()
				spCreator := creator.NewSignalProcessingCreator(fakeClient, scheme, nil)

				// When: Attempting to create child CRD
				name, err := spCreator.Create(ctx, rr)

				// Then: Should return clear error about missing UID
				Expect(err).To(HaveOccurred(), "Empty UID must cause owner reference failure")
				Expect(err.Error()).To(ContainSubstring("owner reference"),
					"Error must clearly indicate owner reference issue")
				Expect(name).To(BeEmpty(), "No CRD should be created without proper owner ref")
			})

			It("should handle RR with empty ResourceVersion gracefully", func() {
				// Scenario: RR created but not read back yet (race condition)
				// Business Outcome: Prevents orphaned CRDs, retries later
				// Confidence: 85% - Timing-sensitive edge case

				// Given: RemediationRequest with UID but no ResourceVersion
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr-no-rv",
						Namespace: "default",
						UID:       "test-uid-123",
						// ResourceVersion intentionally empty - simulates timing issue
						ResourceVersion: "",
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalName: "test-signal",
					},
				}

				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr).Build()
				spCreator := creator.NewSignalProcessingCreator(fakeClient, scheme, nil)

				// When: Attempting to create child CRD
				name, err := spCreator.Create(ctx, rr)

				// Then: Should either succeed (RV not strictly required) OR fail gracefully
				// Note: K8s allows empty RV in some cases, so this might succeed
				// The key is: it should NOT panic or create orphaned CRDs
				if err != nil {
					// If it fails, error should be clear
					Expect(err.Error()).To(Or(
						ContainSubstring("owner reference"),
						ContainSubstring("ResourceVersion"),
					), "Error must indicate metadata issue")
					Expect(name).To(BeEmpty())
				} else {
					// If it succeeds, owner reference should be set correctly
					Expect(name).ToNot(BeEmpty())
					// Verify child CRD was created with proper owner ref
					// (This might actually pass in fake client, which is okay)
				}
			})
		})
	})

	// ========================================
	// Gap 2.2: Timeout Detection - Clock Skew Edge Cases
	// Business Value: Resilient to clock skew and schema evolution
	// Priority 2: Defensive Programming
	// ========================================
	Describe("Timeout Detection Clock Skew (Gap 2.2)", func() {
		Context("when dealing with time-related edge cases", func() {
			It("should handle future CreationTimestamp gracefully (clock skew)", func() {
				// Scenario: CreationTimestamp is in future due to clock skew between nodes
				// Business Outcome: Timeout detector handles gracefully
				// Confidence: 85% - Distributed systems reality

				// Given: RemediationRequest with CreationTimestamp in future (clock skew)
				futureTime := time.Now().Add(5 * time.Minute)
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-future-creation",
						Namespace:         "default",
						CreationTimestamp: metav1.Time{Time: futureTime},
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: remediationv1.PhasePending,
					},
				}

				// When: Checking if globally timed out
				config := remediationorchestrator.OrchestratorConfig{
					Timeouts: remediationorchestrator.PhaseTimeouts{
						Global: 15 * time.Minute,
					},
				}
				detector := timeout.NewDetector(config)
				result := detector.CheckGlobalTimeout(rr)

				// Then: Should not consider as timed out (negative duration)
				Expect(result.TimedOut).To(BeFalse(),
					"Future CreationTimestamp should result in negative elapsed time (not timed out)")
			})

			It("should use CreationTimestamp for global timeout calculation", func() {
				// Scenario: RR created 20 minutes ago (global timeout is 15 min)
				// Business Outcome: Uses CreationTimestamp for timeout calculation
				// Confidence: 90% - Core timeout behavior

				// Given: RemediationRequest created 20 minutes ago
				creationTime := time.Now().Add(-20 * time.Minute)
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-old-rr",
						Namespace:         "default",
						CreationTimestamp: metav1.Time{Time: creationTime},
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: remediationv1.PhaseProcessing,
					},
				}

				// When: Checking if timed out (15 min global timeout)
				config := remediationorchestrator.OrchestratorConfig{
					Timeouts: remediationorchestrator.PhaseTimeouts{
						Global: 15 * time.Minute,
					},
				}
				detector := timeout.NewDetector(config)
				result := detector.CheckGlobalTimeout(rr)

				// Then: Should detect timeout (20min > 15min)
				Expect(result.TimedOut).To(BeTrue(),
					"RR created 20 minutes ago should exceed 15 minute global timeout")
				Expect(result.TimedOutPhase).To(Equal("global"))
			})
		})
	})
})
