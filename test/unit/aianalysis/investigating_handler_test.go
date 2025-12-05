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
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/client"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

// BR-AI-007: InvestigatingHandler tests
var _ = Describe("InvestigatingHandler", func() {
	var (
		handler    *handlers.InvestigatingHandler
		mockClient *testutil.MockHolmesGPTClient
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockClient = testutil.NewMockHolmesGPTClient()
		handler = handlers.NewInvestigatingHandler(mockClient, ctrl.Log.WithName("test"))
	})

	// Helper to create valid AIAnalysis
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
					AnalysisTypes: []string{"investigation"},
				},
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: aianalysis.PhaseInvestigating,
			},
		}
	}

	Describe("Handle", func() {
		// BR-AI-007: Process HolmesGPT response
		Context("with successful API response", func() {
			BeforeEach(func() {
				mockClient.WithSuccessResponse(
					"Root cause identified: OOM",
					0.9,
					true,
					[]string{},
				)
			})

			It("should transition to Analyzing phase", func() {
				analysis := createTestAnalysis()

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseAnalyzing))
			})

			It("should capture targetInOwnerChain in status", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.TargetInOwnerChain).NotTo(BeNil())
				Expect(*analysis.Status.TargetInOwnerChain).To(BeTrue())
			})
		})

		// BR-AI-008: Handle warnings
		Context("with warnings in response", func() {
			BeforeEach(func() {
				mockClient.WithSuccessResponse(
					"Analysis with warnings",
					0.7,
					false,
					[]string{"High memory pressure", "Node scheduling delayed"},
				)
			})

			It("should capture warnings in status", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Warnings).To(HaveLen(2))
				Expect(analysis.Status.Warnings).To(ContainElement("High memory pressure"))
				Expect(analysis.Status.Warnings).To(ContainElement("Node scheduling delayed"))
			})

			It("should set targetInOwnerChain to false", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.TargetInOwnerChain).NotTo(BeNil())
				Expect(*analysis.Status.TargetInOwnerChain).To(BeFalse())
			})
		})

		// BR-AI-008: v1.5 Response Fields - RootCauseAnalysis capture
		Context("with full v1.5 response (RCA, Workflow, Alternatives)", func() {
			BeforeEach(func() {
				mockClient.WithFullResponse(
					"Full analysis with all fields", // analysis
					0.92,                            // confidence
					true,                            // targetInChain
					[]string{"Memory pressure high"}, // warnings
					&client.RootCauseAnalysis{
						Summary:             "OOM caused by memory leak",
						Severity:            "high",
						ContributingFactors: []string{"Memory leak in container"},
					}, // rca
					&client.SelectedWorkflow{
						WorkflowID:     "wf-restart-pod",
						Version:        "1.0.0",
						ContainerImage: "kubernaut.io/workflows/restart:v1.0.0",
						Confidence:     0.92,
						Rationale:      "Selected for OOM recovery",
						Parameters:     map[string]string{"NAMESPACE": "default"},
					}, // selectedWorkflow
					[]client.AlternativeWorkflow{
						{
							WorkflowID:     "wf-scale-deployment",
							ContainerImage: "kubernaut.io/workflows/scale:v1.0.0",
							Confidence:     0.75,
							Rationale:      "Consider scaling if restart fails",
						},
					}, // alternatives
				)
			})

			// BR-AI-008: RootCauseAnalysis capture
			It("should capture RootCauseAnalysis in status", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.RootCauseAnalysis).NotTo(BeNil())
				Expect(analysis.Status.RootCauseAnalysis.Summary).To(Equal("OOM caused by memory leak"))
				Expect(analysis.Status.RootCauseAnalysis.Severity).To(Equal("high"))
				Expect(analysis.Status.RootCause).To(Equal("OOM caused by memory leak"))
			})

			// BR-AI-008: SelectedWorkflow capture (DD-CONTRACT-002)
			It("should capture SelectedWorkflow in status", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.SelectedWorkflow).NotTo(BeNil())
				Expect(analysis.Status.SelectedWorkflow.WorkflowID).To(Equal("wf-restart-pod"))
				Expect(analysis.Status.SelectedWorkflow.ContainerImage).To(Equal("kubernaut.io/workflows/restart:v1.0.0"))
				Expect(analysis.Status.SelectedWorkflow.Confidence).To(BeNumerically("~", 0.92, 0.01))
				Expect(analysis.Status.SelectedWorkflow.Rationale).To(Equal("Selected for OOM recovery"))
			})

			// BR-AI-008: AlternativeWorkflows capture (Q12 - for audit/context only)
			It("should capture AlternativeWorkflows in status for operator context", func() {
				analysis := createTestAnalysis()

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.AlternativeWorkflows).To(HaveLen(1))
				Expect(analysis.Status.AlternativeWorkflows[0].WorkflowID).To(Equal("wf-scale-deployment"))
				Expect(analysis.Status.AlternativeWorkflows[0].Confidence).To(BeNumerically("~", 0.75, 0.01))
				Expect(analysis.Status.AlternativeWorkflows[0].Rationale).To(ContainSubstring("scaling"))
			})
		})

		// BR-AI-009: Retry on transient errors
		// BR-AI-010: Permanent error handling
		// Using DescribeTable per 03-testing-strategy.mdc for reduced duplication
		DescribeTable("error handling based on HTTP status code",
			func(statusCode int, shouldRetry bool, expectedPhase string) {
				mockClient.WithAPIError(statusCode, "API Error")
				analysis := createTestAnalysis()

				result, err := handler.Handle(ctx, analysis)

				if shouldRetry {
					// BR-AI-009: Transient errors should retry
					Expect(err).To(HaveOccurred())
					Expect(result.RequeueAfter).To(BeNumerically(">", 0))
					Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating))
				} else {
					// BR-AI-010: Permanent errors should fail immediately
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Requeue).To(BeFalse())
					Expect(analysis.Status.Phase).To(Equal(expectedPhase))
				}
			},
			// BR-AI-009: Transient errors (retry)
			Entry("503 Service Unavailable - should retry", 503, true, aianalysis.PhaseInvestigating),
			Entry("429 Too Many Requests - should retry", 429, true, aianalysis.PhaseInvestigating),
			Entry("502 Bad Gateway - should retry", 502, true, aianalysis.PhaseInvestigating),
			Entry("504 Gateway Timeout - should retry", 504, true, aianalysis.PhaseInvestigating),

			// BR-AI-010: Permanent errors (fail immediately)
			Entry("401 Unauthorized - should fail", 401, false, aianalysis.PhaseFailed),
			Entry("400 Bad Request - should fail", 400, false, aianalysis.PhaseFailed),
			Entry("403 Forbidden - should fail", 403, false, aianalysis.PhaseFailed),
			Entry("404 Not Found - should fail", 404, false, aianalysis.PhaseFailed),
		)

		// BR-AI-009: Max retries exceeded
		Context("when max retries exceeded", func() {
			BeforeEach(func() {
				mockClient.WithAPIError(503, "Service Unavailable")
			})

			It("should mark as Failed after max retries", func() {
				analysis := createTestAnalysis()
				// Simulate max retries already reached
				analysis.Annotations = map[string]string{
					handlers.RetryCountAnnotation: "5",
				}

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
				Expect(analysis.Status.Message).To(ContainSubstring("max retries"))
			})
		})

		// Test mock call tracking
		Context("mock call verification", func() {
			It("should track API calls", func() {
				analysis := createTestAnalysis()

				Expect(mockClient.AssertNotCalled()).To(Succeed())

				_, _ = handler.Handle(ctx, analysis)

				Expect(mockClient.AssertCalled(1)).To(Succeed())
			})
		})
	})
})
