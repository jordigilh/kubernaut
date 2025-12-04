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
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

// ========================================
// MOCK REGO EVALUATOR
// ========================================

// MockRegoEvaluator is a mock implementation of rego.EvaluatorInterface.
type MockRegoEvaluator struct {
	result *rego.PolicyResult
	err    error
}

// NewMockRegoEvaluator creates a new MockRegoEvaluator.
func NewMockRegoEvaluator() *MockRegoEvaluator {
	return &MockRegoEvaluator{
		result: &rego.PolicyResult{
			ApprovalRequired: false,
			Reason:           "Auto-approved by default",
			Degraded:         false,
		},
	}
}

// SetResult sets the result to return from Evaluate.
func (m *MockRegoEvaluator) SetResult(result *rego.PolicyResult) {
	m.result = result
	m.err = nil
}

// SetError sets the error to return from Evaluate.
func (m *MockRegoEvaluator) SetError(err error) {
	m.err = err
}

// Evaluate implements rego.EvaluatorInterface.
func (m *MockRegoEvaluator) Evaluate(_ context.Context, _ *rego.PolicyInput) (*rego.PolicyResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

var _ = Describe("AnalyzingHandler", func() {
	var (
		ctx           context.Context
		handler       *handlers.AnalyzingHandler
		mockEvaluator *MockRegoEvaluator
		logger        logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logr.Discard()
		mockEvaluator = NewMockRegoEvaluator()
		handler = handlers.NewAnalyzingHandler(
			handlers.WithRegoEvaluator(mockEvaluator),
			handlers.WithAnalyzingLogger(logger),
		)
	})

	// Helper to create an AIAnalysis in Analyzing phase
	createAnalysisInAnalyzingPhase := func() *aianalysisv1.AIAnalysis {
		targetInOwnerChain := true
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-analysis",
				Namespace: "default",
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationID: "test-remediation",
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Environment:      "production",
						BusinessPriority: "P1",
						EnrichmentResults: aianalysisv1.EnrichmentResults{
							DetectedLabels: &aianalysisv1.DetectedLabels{
								GitOpsManaged:   true,
								PDBProtected:    true,
								HPAEnabled:      false,
								Stateful:        false,
								HelmManaged:     false,
								NetworkIsolated: false,
							},
							CustomLabels: map[string][]string{
								"team": {"platform"},
							},
						},
					},
				},
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:              string(aianalysis.PhaseAnalyzing),
				TargetInOwnerChain: &targetInOwnerChain,
				Warnings:           []string{},
			},
		}
	}

	// BR-AI-012: Analyzing phase handling
	Describe("Handle", func() {
		Context("when Rego evaluation succeeds with approval required", func() {
			BeforeEach(func() {
				mockEvaluator.SetResult(&rego.PolicyResult{
					ApprovalRequired: true,
					Reason:           "Production environment requires approval",
					Degraded:         false,
				})
			})

			It("should transition to Recommending phase with approval required - BR-AI-012", func() {
				analysis := createAnalysisInAnalyzingPhase()

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())
				Expect(analysis.Status.Phase).To(Equal(string(aianalysis.PhaseRecommending)))
				Expect(analysis.Status.ApprovalRequired).To(BeTrue())
				Expect(analysis.Status.ApprovalReason).To(Equal("Production environment requires approval"))
			})
		})

		Context("when Rego evaluation succeeds with auto-approve", func() {
			BeforeEach(func() {
				mockEvaluator.SetResult(&rego.PolicyResult{
					ApprovalRequired: false,
					Reason:           "Non-production environment auto-approved",
					Degraded:         false,
				})
			})

			It("should transition to Recommending phase without approval - BR-AI-012", func() {
				analysis := createAnalysisInAnalyzingPhase()

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())
				Expect(analysis.Status.Phase).To(Equal(string(aianalysis.PhaseRecommending)))
				Expect(analysis.Status.ApprovalRequired).To(BeFalse())
			})
		})

		// BR-AI-013: Approval scenarios
		DescribeTable("determines approval based on Rego result",
			func(approvalRequired bool, degraded bool, expectedApproval bool, expectedDegraded bool) {
				mockEvaluator.SetResult(&rego.PolicyResult{
					ApprovalRequired: approvalRequired,
					Reason:           "Test reason",
					Degraded:         degraded,
				})

				analysis := createAnalysisInAnalyzingPhase()

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())
				Expect(analysis.Status.ApprovalRequired).To(Equal(expectedApproval))
				Expect(analysis.Status.DegradedMode).To(Equal(expectedDegraded))
			},
			Entry("approval required, not degraded", true, false, true, false),
			Entry("no approval, not degraded", false, false, false, false),
			Entry("approval required, degraded", true, true, true, true),
			Entry("no approval, degraded", false, true, false, true),
		)

		// BR-AI-014: Degraded mode
		Context("when Rego evaluation fails gracefully", func() {
			BeforeEach(func() {
				mockEvaluator.SetResult(&rego.PolicyResult{
					ApprovalRequired: true, // Safe default
					Reason:           "Policy evaluation failed - defaulting to manual approval",
					Degraded:         true,
				})
			})

			It("should continue with safe default and set degraded mode - BR-AI-014", func() {
				analysis := createAnalysisInAnalyzingPhase()

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())
				Expect(analysis.Status.ApprovalRequired).To(BeTrue())
				Expect(analysis.Status.DegradedMode).To(BeTrue())
			})
		})
	})

	Describe("Phase", func() {
		It("should return PhaseAnalyzing", func() {
			Expect(handler.Phase()).To(Equal(aianalysis.PhaseAnalyzing))
		})
	})
})

