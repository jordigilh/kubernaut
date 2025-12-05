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

package aianalysis

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

// BR-AI-012: AnalyzingHandler tests
var _ = Describe("AnalyzingHandler", func() {
	var (
		handler       *handlers.AnalyzingHandler
		mockEvaluator *testutil.MockRegoEvaluator
		ctx           context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockEvaluator = testutil.NewMockRegoEvaluator()
		handler = handlers.NewAnalyzingHandler(mockEvaluator, ctrl.Log.WithName("test"))
	})

	// Helper to create valid AIAnalysis in Analyzing phase
	createTestAnalysis := func() *aianalysisv1.AIAnalysis {
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-analysis",
				Namespace: "default",
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Kind:      "RemediationRequest",
					Name:      "test-rr",
					Namespace: "default",
				},
				RemediationID: "test-remediation-001",
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Fingerprint:      "test-fingerprint",
						Severity:         "warning",
						SignalType:       "OOMKilled",
						Environment:      "production",
						BusinessPriority: "P0",
						TargetResource: aianalysisv1.TargetResource{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
					AnalysisTypes: []string{"investigation", "analysis"},
				},
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: aianalysis.PhaseAnalyzing,
				// Simulating data from InvestigatingHandler
				RootCause: "OOM caused by memory leak",
				SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowID:     "wf-restart-pod",
					ContainerImage: "kubernaut.io/workflows/restart:v1.0.0",
					Confidence:     0.92,
					Rationale:      "Selected for OOM recovery",
				},
			},
		}
	}

	Describe("Handle", func() {
		// BR-AI-012: Successful Rego evaluation with approval required
		Context("when Rego evaluation requires approval", func() {
			BeforeEach(func() {
				mockEvaluator.WithApprovalRequired("Production environment requires approval")
			})

			It("should transition to Recommending phase", func() {
				analysis := createTestAnalysis()

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseRecommending))
			})

			It("should set ApprovalRequired to true", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.ApprovalRequired).To(BeTrue())
			})

			It("should set ApprovalReason", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.ApprovalReason).To(Equal("Production environment requires approval"))
			})
		})

		// BR-AI-012: Successful Rego evaluation with auto-approve
		Context("when Rego evaluation auto-approves", func() {
			BeforeEach(func() {
				mockEvaluator.WithAutoApprove("Non-production environment - auto-approved")
			})

			It("should transition to Recommending phase", func() {
				analysis := createTestAnalysis()
				analysis.Spec.AnalysisRequest.SignalContext.Environment = "development"

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseRecommending))
			})

			It("should set ApprovalRequired to false", func() {
				analysis := createTestAnalysis()
				analysis.Spec.AnalysisRequest.SignalContext.Environment = "development"

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.ApprovalRequired).To(BeFalse())
			})
		})

		// BR-AI-014: Degraded mode (policy failure fallback)
		Context("when Rego evaluation fails gracefully (degraded mode)", func() {
			BeforeEach(func() {
				mockEvaluator.WithDegradedMode("Policy evaluation failed - defaulting to manual approval")
			})

			It("should continue to Recommending phase with safe defaults", func() {
				analysis := createTestAnalysis()

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseRecommending))
			})

			It("should set ApprovalRequired to true (safe default)", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.ApprovalRequired).To(BeTrue())
			})

			It("should set DegradedMode to true", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.DegradedMode).To(BeTrue())
			})

			It("should include degraded reason in ApprovalReason", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.ApprovalReason).To(ContainSubstring("Policy evaluation failed"))
			})
		})

		// BR-AI-012: Policy input construction
		Context("policy input construction", func() {
			It("should pass correct environment to evaluator", func() {
				analysis := createTestAnalysis()
				analysis.Spec.AnalysisRequest.SignalContext.Environment = "staging"

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.LastInput).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.Environment).To(Equal("staging"))
			})

			It("should pass confidence from SelectedWorkflow", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.LastInput).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.Confidence).To(BeNumerically("~", 0.92, 0.01))
			})

			It("should pass TargetInOwnerChain from status", func() {
				analysis := createTestAnalysis()
				targetInChain := true
				analysis.Status.TargetInOwnerChain = &targetInChain

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.LastInput).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.TargetInOwnerChain).To(BeTrue())
			})

			It("should pass Warnings from status", func() {
				analysis := createTestAnalysis()
				analysis.Status.Warnings = []string{"High memory pressure", "Node scheduling delayed"}

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.LastInput).NotTo(BeNil())
				Expect(mockEvaluator.LastInput.Warnings).To(HaveLen(2))
				Expect(mockEvaluator.LastInput.Warnings).To(ContainElement("High memory pressure"))
			})
		})

		// BR-AI-012: Evaluator is called exactly once
		Context("evaluator invocation", func() {
			It("should call evaluator exactly once", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(mockEvaluator.AssertCalled(1)).To(Succeed())
			})
		})
	})

	// BR-AI-012: Name method
	Describe("Name", func() {
		It("should return 'analyzing'", func() {
			Expect(handler.Name()).To(Equal("analyzing"))
		})
	})
})

