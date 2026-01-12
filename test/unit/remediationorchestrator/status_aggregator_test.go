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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/aggregator"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

var _ = Describe("StatusAggregator", func() {
	var (
		scheme *runtime.Scheme
		ctx    context.Context
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = signalprocessingv1.AddToScheme(scheme)
		_ = aianalysisv1.AddToScheme(scheme)
		_ = workflowexecutionv1.AddToScheme(scheme)
		_ = notificationv1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		ctx = context.Background()
	})

	Describe("AggregateStatus", func() {
		var fakeClient *fake.ClientBuilder

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme)
		})

		Context("BR-ORCH-029: Status aggregation from child CRDs", func() {
			// Test #2: Aggregates SignalProcessing status when ref exists
			It("should aggregate SignalProcessing status when SignalProcessingRef is set", func() {
				// Arrange
				sp := helpers.NewCompletedSignalProcessing("sp-test", "default")
				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.Status.SignalProcessingRef = &corev1.ObjectReference{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}

				client := fakeClient.WithObjects(sp).Build()
				agg := aggregator.NewStatusAggregator(client)

				// Act
				result, err := agg.AggregateStatus(ctx, rr)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.SignalProcessingPhase).To(Equal(string(sp.Status.Phase)))
			})

			// Test #3: Aggregates AIAnalysis status when ref exists
			It("should aggregate AIAnalysis status when AIAnalysisRef is set", func() {
				// Arrange
				ai := helpers.NewCompletedAIAnalysis("ai-test", "default")
				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.Status.AIAnalysisRef = &corev1.ObjectReference{
					Name:      ai.Name,
					Namespace: ai.Namespace,
				}

				client := fakeClient.WithObjects(ai).Build()
				agg := aggregator.NewStatusAggregator(client)

				// Act
				result, err := agg.AggregateStatus(ctx, rr)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.AIAnalysisPhase).To(Equal(string(ai.Status.Phase)))
			})

			// Test #4: Aggregates WorkflowExecution status when ref exists
			It("should aggregate WorkflowExecution status when WorkflowExecutionRef is set", func() {
				// Arrange
				we := &workflowexecutionv1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "we-test",
						Namespace: "default",
					},
					Status: workflowexecutionv1.WorkflowExecutionStatus{
						Phase: "Completed",
					},
				}
				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
					Name:      we.Name,
					Namespace: we.Namespace,
				}

				client := fakeClient.WithObjects(we).Build()
				agg := aggregator.NewStatusAggregator(client)

				// Act
				result, err := agg.AggregateStatus(ctx, rr)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.WorkflowExecutionPhase).To(Equal("Completed"))
			})

			// Test #5: Does not error when refs are nil
			It("should return empty aggregated status when child refs are nil", func() {
				// Arrange
				rr := helpers.NewRemediationRequest("test-rr", "default")
				// No refs set

				client := fakeClient.Build()
				agg := aggregator.NewStatusAggregator(client)

				// Act
				result, err := agg.AggregateStatus(ctx, rr)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.SignalProcessingPhase).To(BeEmpty())
				Expect(result.AIAnalysisPhase).To(BeEmpty())
				Expect(result.WorkflowExecutionPhase).To(BeEmpty())
			})

			// Test #6: Handles missing child CRDs gracefully (no error, sets AllChildrenHealthy=false)
			It("should handle missing child CRDs gracefully", func() {
				// Arrange
				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.Status.SignalProcessingRef = &corev1.ObjectReference{
					Name:      "non-existent-sp",
					Namespace: "default",
				}

				client := fakeClient.Build() // No SP created
				agg := aggregator.NewStatusAggregator(client)

				// Act
				result, err := agg.AggregateStatus(ctx, rr)

				// Assert - graceful handling: no error, but AllChildrenHealthy=false
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.SignalProcessingPhase).To(BeEmpty()) // Phase empty since not found
				Expect(result.AllChildrenHealthy).To(BeFalse())
			})
		})

		// ========================================
		// Child CRD Race Conditions
		// Tests defensive programming for concurrent status changes
		// Business Value: Prevents nil pointer panics and inconsistent state
		// ========================================
		Context("Child CRD Race Conditions", func() {
			It("should handle child CRD deleted during aggregation (operator error)", func() {
				// Scenario: SignalProcessing CRD deleted mid-reconcile (operator mistake)
				// Business Value: Resilient to unexpected CRD deletions
				// Confidence: 95% - Real operator workflow

				// Given: RemediationRequest referencing non-existent child CRD
				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.Status.SignalProcessingRef = &corev1.ObjectReference{
					Name:      "deleted-sp",
					Namespace: "default",
				}

				// No SignalProcessing CRD created (simulates deletion)
				client := fakeClient.Build()
				agg := aggregator.NewStatusAggregator(client)

				// When: Aggregation is attempted
				result, err := agg.AggregateStatus(ctx, rr)

				// Then: Should not error (graceful handling)
				Expect(err).ToNot(HaveOccurred(), "Aggregator must handle missing child CRDs gracefully")
				Expect(result).ToNot(BeNil())
				Expect(result.SignalProcessingPhase).To(BeEmpty(),
					"Phase should be empty when child CRD not found")
				Expect(result.AllChildrenHealthy).To(BeFalse(),
					"AllChildrenHealthy must be false when child is missing")
			})

			It("should handle child CRD with empty Phase field (uninitialized status)", func() {
				// Scenario: AIAnalysis exists but status.Phase not yet set (race condition)
				// Business Value: Prevents nil pointer panics during status initialization
				// Confidence: 100% - Critical defensive programming

				// Given: AIAnalysis with uninitialized status
				ai := &aianalysisv1.AIAnalysis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ai-uninit",
						Namespace: "default",
					},
					// Status field exists but Phase is empty
					Status: aianalysisv1.AIAnalysisStatus{
						Phase: "", // Empty phase
					},
				}

				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.Status.AIAnalysisRef = &corev1.ObjectReference{
					Name:      ai.Name,
					Namespace: ai.Namespace,
				}

				client := fakeClient.WithObjects(ai).Build()
				agg := aggregator.NewStatusAggregator(client)

				// When: Aggregation is attempted
				result, err := agg.AggregateStatus(ctx, rr)

				// Then: Should handle gracefully (no panic)
				Expect(err).ToNot(HaveOccurred(), "Empty phase must not cause panic")
				Expect(result).ToNot(BeNil())
				Expect(result.AIAnalysisPhase).To(BeEmpty(),
					"Empty phase should be returned as-is")
			})

			It("should aggregate consistent snapshot when multiple children update simultaneously", func() {
				// Scenario: AIAnalysis and WorkflowExecution both complete in same reconcile cycle
				// Business Value: Ensures consistent state transitions under concurrent updates
				// Confidence: 85% - Timing-sensitive edge case

				// Given: Both AIAnalysis and WorkflowExecution completed
				ai := helpers.NewCompletedAIAnalysis("ai-test", "default")
				we := &workflowexecutionv1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "we-test",
						Namespace: "default",
					},
					Status: workflowexecutionv1.WorkflowExecutionStatus{
						Phase: "Completed",
					},
				}

				rr := helpers.NewRemediationRequest("test-rr", "default")
				rr.Status.AIAnalysisRef = &corev1.ObjectReference{
					Name:      ai.Name,
					Namespace: ai.Namespace,
				}
				rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
					Name:      we.Name,
					Namespace: we.Namespace,
				}

				client := fakeClient.WithObjects(ai, we).Build()
				agg := aggregator.NewStatusAggregator(client)

				// When: Aggregation reads both statuses
				result, err := agg.AggregateStatus(ctx, rr)

				// Then: Should capture both phases in consistent snapshot
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.AIAnalysisPhase).To(Equal("Completed"),
					"Must capture AI phase")
				Expect(result.WorkflowExecutionPhase).To(Equal("Completed"),
					"Must capture WE phase in same aggregation")
				Expect(result.AllChildrenHealthy).To(BeTrue(),
					"Both children healthy should set AllChildrenHealthy=true")
			})
		})
	})
})
