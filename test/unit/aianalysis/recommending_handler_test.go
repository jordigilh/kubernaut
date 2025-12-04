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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
)

var _ = Describe("RecommendingHandler", func() {
	var (
		ctx     context.Context
		handler *handlers.RecommendingHandler
		logger  logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logr.Discard()

		// Create fake client with scheme
		scheme := runtime.NewScheme()
		_ = aianalysisv1.AddToScheme(scheme)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
			Build()

		handler = handlers.NewRecommendingHandler(
			handlers.WithRecommendingLogger(logger),
			handlers.WithRecommendingClient(fakeClient),
		)
	})

	// Helper to create an AIAnalysis in Recommending phase with workflow selected
	createAnalysisWithWorkflow := func() *aianalysisv1.AIAnalysis {
		startedAt := metav1.NewTime(time.Now().Add(-30 * time.Second))
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-analysis-with-workflow",
				Namespace: "default",
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationID: "test-remediation",
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:     string(aianalysis.PhaseRecommending),
				StartedAt: &startedAt,
				SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowID:     "wf-restart-pod",
					Version:        "v1.0.0",
					ContainerImage: "ghcr.io/kubernaut/workflows/restart-pod:v1.0.0",
					Confidence:     0.92,
					Parameters: map[string]string{
						"NAMESPACE": "default",
						"POD_NAME":  "my-pod",
					},
					Rationale: "Pod stuck in CrashLoopBackOff",
				},
				ApprovalRequired: true,
				ApprovalReason:   "Production environment",
			},
		}
	}

	// Helper to create an AIAnalysis in Recommending phase without workflow
	createAnalysisWithoutWorkflow := func() *aianalysisv1.AIAnalysis {
		startedAt := metav1.NewTime(time.Now().Add(-30 * time.Second))
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-analysis-no-workflow",
				Namespace: "default",
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationID: "test-remediation",
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:            string(aianalysis.PhaseRecommending),
				StartedAt:        &startedAt,
				SelectedWorkflow: nil, // No workflow selected
				ApprovalRequired: false,
			},
		}
	}

	// Helper to create an AIAnalysis with low confidence workflow
	createAnalysisWithLowConfidenceWorkflow := func() *aianalysisv1.AIAnalysis {
		startedAt := metav1.NewTime(time.Now().Add(-30 * time.Second))
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-analysis-low-confidence",
				Namespace: "default",
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationID: "test-remediation",
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:     string(aianalysis.PhaseRecommending),
				StartedAt: &startedAt,
				SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowID:     "wf-scale-deployment",
					Version:        "v1.0.0",
					ContainerImage: "ghcr.io/kubernaut/workflows/scale:v1.0.0",
					Confidence:     0.35, // Low confidence
					Rationale:      "Uncertain workflow selection",
				},
				ApprovalRequired: true,
			},
		}
	}

	// BR-AI-016: Workflow recommendation finalization
	Describe("Handle", func() {
		Context("with selected workflow", func() {
			It("should complete successfully with workflow details - BR-AI-016", func() {
				analysis := createAnalysisWithWorkflow()

				// Create the analysis in the fake client
				scheme := runtime.NewScheme()
				_ = aianalysisv1.AddToScheme(scheme)
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(analysis).
					WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
					Build()

				handler = handlers.NewRecommendingHandler(
					handlers.WithRecommendingLogger(logger),
					handlers.WithRecommendingClient(fakeClient),
				)

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse()) // Final phase
				Expect(analysis.Status.Phase).To(Equal(string(aianalysis.PhaseCompleted)))
				Expect(analysis.Status.CompletedAt).NotTo(BeNil())
				Expect(analysis.Status.SelectedWorkflow).NotTo(BeNil())
				Expect(analysis.Status.SelectedWorkflow.WorkflowID).To(Equal("wf-restart-pod"))
			})
		})

		// BR-AI-018: No recommendations scenario
		Context("without selected workflow", func() {
			It("should complete with message about no workflow - BR-AI-018", func() {
				analysis := createAnalysisWithoutWorkflow()

				scheme := runtime.NewScheme()
				_ = aianalysisv1.AddToScheme(scheme)
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(analysis).
					WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
					Build()

				handler = handlers.NewRecommendingHandler(
					handlers.WithRecommendingLogger(logger),
					handlers.WithRecommendingClient(fakeClient),
				)

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
				Expect(analysis.Status.Phase).To(Equal(string(aianalysis.PhaseCompleted)))
				Expect(analysis.Status.Message).To(ContainSubstring("no suitable workflow"))
			})
		})

		Context("with low confidence workflow", func() {
			It("should complete with low confidence warning - BR-AI-017", func() {
				analysis := createAnalysisWithLowConfidenceWorkflow()

				scheme := runtime.NewScheme()
				_ = aianalysisv1.AddToScheme(scheme)
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(analysis).
					WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
					Build()

				handler = handlers.NewRecommendingHandler(
					handlers.WithRecommendingLogger(logger),
					handlers.WithRecommendingClient(fakeClient),
				)

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
				Expect(analysis.Status.Phase).To(Equal(string(aianalysis.PhaseCompleted)))
				Expect(analysis.Status.Message).To(ContainSubstring("Low confidence"))
			})
		})

		Context("timing calculations", func() {
			It("should calculate total analysis time", func() {
				analysis := createAnalysisWithWorkflow()

				scheme := runtime.NewScheme()
				_ = aianalysisv1.AddToScheme(scheme)
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(analysis).
					WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
					Build()

				handler = handlers.NewRecommendingHandler(
					handlers.WithRecommendingLogger(logger),
					handlers.WithRecommendingClient(fakeClient),
				)

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
				// Total analysis time should be >= 30 seconds (set in createAnalysisWithWorkflow)
				Expect(analysis.Status.TotalAnalysisTime).To(BeNumerically(">=", 30))
			})
		})
	})

	Describe("Name", func() {
		It("should return RecommendingHandler", func() {
			Expect(handler.Name()).To(Equal("RecommendingHandler"))
		})
	})
})

