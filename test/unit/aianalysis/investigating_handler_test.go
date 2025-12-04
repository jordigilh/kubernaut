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
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	aiclient "github.com/jordigilh/kubernaut/pkg/aianalysis/client"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ========================================
// MOCK CLIENT FOR TESTING
// ========================================

// MockHolmesGPTClient is a mock implementation of HolmesGPTClient for testing.
type MockHolmesGPTClient struct {
	response *aiclient.IncidentResponse
	err      error
	calls    int
}

func NewMockHolmesGPTClient() *MockHolmesGPTClient {
	return &MockHolmesGPTClient{}
}

func (m *MockHolmesGPTClient) SetResponse(resp *aiclient.IncidentResponse, err error) {
	m.response = resp
	m.err = err
}

func (m *MockHolmesGPTClient) Investigate(ctx context.Context, req *aiclient.IncidentRequest) (*aiclient.IncidentResponse, error) {
	m.calls++
	return m.response, m.err
}

func (m *MockHolmesGPTClient) CallCount() int {
	return m.calls
}

// ========================================
// INVESTIGATING HANDLER TESTS (BR-AI-007, BR-AI-008, BR-AI-009)
// ========================================

var _ = Describe("InvestigatingHandler", func() {
	var (
		ctx        context.Context
		scheme     *runtime.Scheme
		fakeClient client.Client
		handler    *handlers.InvestigatingHandler
		mockClient *MockHolmesGPTClient
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Setup scheme
		scheme = runtime.NewScheme()
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		// Create fake client
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
			Build()

		// Create mock HolmesGPT client
		mockClient = NewMockHolmesGPTClient()

		// Create handler
		handler = handlers.NewInvestigatingHandler(
			handlers.WithInvestigatingLogger(logf.Log.WithName("test")),
			handlers.WithInvestigatingClient(fakeClient),
			handlers.WithHolmesGPTClient(mockClient),
		)
	})

	// BR-AI-007: Process HolmesGPT response
	Describe("Handle", func() {
		Context("with successful API response", func() {
			BeforeEach(func() {
				mockClient.SetResponse(&aiclient.IncidentResponse{
					IncidentID: "test-incident",
					Analysis:   "Root cause: Memory leak in application",
					RootCauseAnalysis: &aiclient.RootCauseAnalysis{
						Summary:             "Memory leak identified",
						Severity:            "high",
						SignalType:          "OOMKilled",
						ContributingFactors: []string{"Resource limits too low"},
					},
					SelectedWorkflow: &aiclient.SelectedWorkflow{
						WorkflowID:     "restart-pod",
						Version:        "v1.0.0",
						ContainerImage: "kubernaut/workflows:restart-pod-v1.0.0",
						Confidence:     0.9,
						Parameters:     map[string]string{"GRACE_PERIOD": "30"},
						Rationale:      "Pod restart is the safest approach",
					},
					Confidence:         0.9,
					Timestamp:          "2025-12-04T10:00:00Z",
					TargetInOwnerChain: true,
					Warnings:           []string{},
				}, nil)
			})

			It("should transition to Analyzing phase - BR-AI-007", func() {
				analysis := createTestAnalysisForInvestigating("test-success")
				Expect(fakeClient.Create(ctx, analysis)).To(Succeed())

				// Initialize status
				now := metav1.Now()
				analysis.Status.Phase = aianalysis.PhaseInvestigating
				analysis.Status.StartedAt = &now
				Expect(fakeClient.Status().Update(ctx, analysis)).To(Succeed())

				// Handle
				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				// Verify status was updated
				var updated aianalysisv1.AIAnalysis
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)).To(Succeed())
				Expect(updated.Status.Phase).To(Equal(aianalysis.PhaseAnalyzing))
				Expect(updated.Status.RootCause).To(ContainSubstring("Memory leak"))
				Expect(updated.Status.SelectedWorkflow).NotTo(BeNil())
				Expect(updated.Status.SelectedWorkflow.WorkflowID).To(Equal("restart-pod"))
			})

			It("should capture targetInOwnerChain - BR-AI-008", func() {
				analysis := createTestAnalysisForInvestigating("test-owner-chain")
				Expect(fakeClient.Create(ctx, analysis)).To(Succeed())

				analysis.Status.Phase = aianalysis.PhaseInvestigating
				Expect(fakeClient.Status().Update(ctx, analysis)).To(Succeed())

				_, err := handler.Handle(ctx, analysis)
				Expect(err).NotTo(HaveOccurred())

				var updated aianalysisv1.AIAnalysis
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)).To(Succeed())
				Expect(updated.Status.TargetInOwnerChain).NotTo(BeNil())
				Expect(*updated.Status.TargetInOwnerChain).To(BeTrue())
			})
		})

		// BR-AI-008: Handle warnings
		Context("with warnings in response", func() {
			BeforeEach(func() {
				mockClient.SetResponse(&aiclient.IncidentResponse{
					IncidentID:         "test-incident",
					Analysis:           "Analysis with warnings",
					Confidence:         0.65,
					TargetInOwnerChain: false,
					Warnings:           []string{"Low confidence due to limited data", "OwnerChain validation failed"},
				}, nil)
			})

			It("should capture warnings in status - BR-AI-008", func() {
				analysis := createTestAnalysisForInvestigating("test-warnings")
				Expect(fakeClient.Create(ctx, analysis)).To(Succeed())

				analysis.Status.Phase = aianalysis.PhaseInvestigating
				Expect(fakeClient.Status().Update(ctx, analysis)).To(Succeed())

				_, err := handler.Handle(ctx, analysis)
				Expect(err).NotTo(HaveOccurred())

				var updated aianalysisv1.AIAnalysis
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)).To(Succeed())
				Expect(updated.Status.Warnings).To(HaveLen(2))
				Expect(updated.Status.Warnings[0]).To(ContainSubstring("Low confidence"))
				Expect(*updated.Status.TargetInOwnerChain).To(BeFalse())
			})
		})

		// BR-AI-009: Retry on transient errors
		DescribeTable("handles transient errors with retry",
			func(statusCode int, shouldRetry bool) {
				mockClient.SetResponse(nil, aiclient.NewAPIError(statusCode, "error"))

				analysis := createTestAnalysisForInvestigating("test-transient")
				Expect(fakeClient.Create(ctx, analysis)).To(Succeed())

				analysis.Status.Phase = aianalysis.PhaseInvestigating
				Expect(fakeClient.Status().Update(ctx, analysis)).To(Succeed())

				result, _ := handler.Handle(ctx, analysis)

				var updated aianalysisv1.AIAnalysis
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)).To(Succeed())

				if shouldRetry {
					Expect(result.RequeueAfter).To(BeNumerically(">", 0))
					Expect(updated.Status.Phase).To(Equal(aianalysis.PhaseInvestigating))
				} else {
					Expect(updated.Status.Phase).To(Equal(aianalysis.PhaseFailed))
				}
			},
			Entry("503 Service Unavailable - retry", http.StatusServiceUnavailable, true),
			Entry("429 Too Many Requests - retry", http.StatusTooManyRequests, true),
			Entry("502 Bad Gateway - retry", http.StatusBadGateway, true),
			Entry("401 Unauthorized - no retry", http.StatusUnauthorized, false),
			Entry("400 Bad Request - no retry", http.StatusBadRequest, false),
		)

		// BR-AI-009: Max retries exceeded
		Context("when max retries exceeded", func() {
			BeforeEach(func() {
				mockClient.SetResponse(nil, aiclient.NewAPIError(http.StatusServiceUnavailable, "service unavailable"))
			})

			It("should mark as Failed after max retries - BR-AI-009", func() {
				analysis := createTestAnalysisForInvestigating("test-max-retries")
				// Set annotations to simulate max retries reached
				analysis.Annotations = map[string]string{
					handlers.RetryCountAnnotation: "5",
				}
				Expect(fakeClient.Create(ctx, analysis)).To(Succeed())

				analysis.Status.Phase = aianalysis.PhaseInvestigating
				Expect(fakeClient.Status().Update(ctx, analysis)).To(Succeed())

				_, _ = handler.Handle(ctx, analysis)

				var updated aianalysisv1.AIAnalysis
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)).To(Succeed())
				Expect(updated.Status.Phase).To(Equal(aianalysis.PhaseFailed))
				Expect(updated.Status.Message).To(ContainSubstring("max retries"))
			})
		})
	})
})

// ========================================
// TEST HELPERS
// ========================================

// createTestAnalysisForInvestigating creates a valid AIAnalysis for investigating tests.
func createTestAnalysisForInvestigating(name string) *aianalysisv1.AIAnalysis {
	return &aianalysisv1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: aianalysisv1.AIAnalysisSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Name:      "test-rr",
				Namespace: "default",
			},
			RemediationID: "rem-12345",
			AnalysisRequest: aianalysisv1.AnalysisRequest{
				SignalContext: aianalysisv1.SignalContextInput{
					Fingerprint:      "abc123def456",
					Severity:         "warning",
					SignalType:       "OOMKilled",
					Environment:      "production",
					BusinessPriority: "P1",
					TargetResource: aianalysisv1.TargetResource{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: "default",
					},
					EnrichmentResults: sharedtypes.EnrichmentResults{
						KubernetesContext: &sharedtypes.KubernetesContext{
							Namespace: "default",
						},
						DetectedLabels: &sharedtypes.DetectedLabels{
							GitOpsManaged: true,
							PDBProtected:  false,
						},
						OwnerChain: []sharedtypes.OwnerChainEntry{
							{Namespace: "default", Kind: "Deployment", Name: "test-deploy"},
						},
					},
				},
				AnalysisTypes: []string{"investigation", "root-cause", "workflow-selection"},
			},
		},
	}
}
