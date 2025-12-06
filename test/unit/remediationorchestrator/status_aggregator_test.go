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

package remediationorchestrator_test

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
	"github.com/jordigilh/kubernaut/pkg/testutil"
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

	Describe("Constructor", func() {
		// Test #1: Constructor returns non-nil
		It("should return non-nil StatusAggregator", func() {
			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			agg := aggregator.NewStatusAggregator(client)
			Expect(agg).ToNot(BeNil())
		})
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
				sp := testutil.NewCompletedSignalProcessing("sp-test", "default")
				rr := testutil.NewRemediationRequest("test-rr", "default")
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
				ai := testutil.NewCompletedAIAnalysis("ai-test", "default")
				rr := testutil.NewRemediationRequest("test-rr", "default")
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
				rr := testutil.NewRemediationRequest("test-rr", "default")
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
				rr := testutil.NewRemediationRequest("test-rr", "default")
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

			// Test #6: Returns error when child CRD not found
			It("should return error when referenced child CRD not found", func() {
				// Arrange
				rr := testutil.NewRemediationRequest("test-rr", "default")
				rr.Status.SignalProcessingRef = &corev1.ObjectReference{
					Name:      "non-existent-sp",
					Namespace: "default",
				}

				client := fakeClient.Build() // No SP created
				agg := aggregator.NewStatusAggregator(client)

				// Act
				result, err := agg.AggregateStatus(ctx, rr)

				// Assert
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("not found"))
			})
		})
	})
})

