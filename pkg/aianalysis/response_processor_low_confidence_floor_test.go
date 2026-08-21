/*
Copyright 2026 Jordi Gil.

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

// Package aianalysis_test: Issue #1828 / BR-AI-088.4 — the Investigating-phase
// low-confidence floor (whether a KA-selected workflow's confidence is high
// enough to proceed to Analyzing, vs. requiring human review) was hardcoded
// to 0.7 in two places (response_processor.go's ProcessIncidentResponse and
// handleLowConfidenceFailure). This is a distinct, earlier gate from Rego's
// operator-configurable auto-approval ConfidenceThreshold (#225) — see
// RegoConfig.LowConfidenceFloor's doc comment for how the two differ.
package aianalysis_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// buildLowConfidenceFloorTestAnalysis returns a minimal, valid AIAnalysis for
// exercising ResponseProcessor/InvestigatingHandler directly, mirroring
// createTestAnalysis in investigating_handler_test.go (file-scoped there).
func buildLowConfidenceFloorTestAnalysis() *aianalysisv1.AIAnalysis {
	return &aianalysisv1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-1828-analysis",
			Namespace: "default",
		},
		Spec: aianalysisv1.AIAnalysisSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Kind:      "RemediationRequest",
				Name:      "test-rr-1828",
				Namespace: "default",
			},
			RemediationID: "test-remediation-1828",
			AnalysisRequest: aianalysisv1.AnalysisRequest{
				SignalContext: aianalysisv1.SignalContextInput{
					Fingerprint:      "test-fingerprint-1828",
					Severity:         "warning",
					SignalName:       "OOMKilled",
					Environment:      "production",
					BusinessPriority: "P0",
					TargetResource: aianalysisv1.TargetResource{
						Kind:      "Pod",
						Name:      "test-pod-1828",
						Namespace: "default",
					},
				},
				AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
			},
		},
	}
}

// buildIncidentResponseWithWorkflowConfidence returns an AgentSessionResult
// with a SelectedWorkflow at the given confidence and NeedsHumanReview unset
// (false), so ProcessAgentSessionResult falls through to the
// low-confidence-floor check (BR-HAPI-197 AC-4).
func buildIncidentResponseWithWorkflowConfidence(confidence float64) *agentsessionv1.AgentSessionResult {
	return &agentsessionv1.AgentSessionResult{
		IncidentID:       "test-incident-1828",
		Analysis:         "Investigated OOMKilled signal",
		Confidence:       confidence,
		Timestamp:        "2026-07-29T00:00:00Z",
		SelectedWorkflow: mocks.BuildMockSelectedWorkflow("restart-pod-v1", "ghcr.io/kubernaut/restart-pod:v1.0", confidence, ""),
	}
}

var _ = Describe("ResponseProcessor.WithLowConfidenceFloor (BR-AI-088.4, Issue #1828)", func() {
	var (
		m   *metrics.Metrics
		ctx context.Context
	)

	BeforeEach(func() {
		m = metrics.NewMetrics()
		ctx = context.Background()
	})

	It("UT-AA-1828-001: a nil LowConfidenceFloor preserves the V1.0 70% default", func() {
		processor := handlers.NewResponseProcessor(logr.Discard(), m, &noopAuditClient{})
		analysis := buildLowConfidenceFloorTestAnalysis()
		resp := buildIncidentResponseWithWorkflowConfidence(0.65) // below the 0.7 default

		_, err := processor.ProcessAgentSessionResult(ctx, analysis, resp)

		Expect(err).ToNot(HaveOccurred())
		Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
		Expect(analysis.Status.GetReview().HumanReviewReason).To(Equal(aianalysisv1.HumanReviewReasonLowConfidence))
		Expect(analysis.Status.Message).To(ContainSubstring("below threshold 0.70"))
	})

	It("UT-AA-1828-002: an explicit LowConfidenceFloor overrides the 70% default", func() {
		floor := 0.5
		processor := handlers.NewResponseProcessor(logr.Discard(), m, &noopAuditClient{}).WithLowConfidenceFloor(&floor)
		analysis := buildLowConfidenceFloorTestAnalysis()
		resp := buildIncidentResponseWithWorkflowConfidence(0.65) // below 0.7 default, above the 0.5 custom floor

		_, err := processor.ProcessAgentSessionResult(ctx, analysis, resp)

		Expect(err).ToNot(HaveOccurred())
		Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseAnalyzing),
			"0.65 confidence clears the operator-configured 0.5 floor even though it's below the 0.7 default")
		Expect(analysis.Status.GetReview().NeedsHumanReview).To(BeFalse())
	})

	It("UT-AA-1828-003: the low-confidence failure message reports the configured floor, not the hardcoded 0.70 default", func() {
		floor := 0.5
		processor := handlers.NewResponseProcessor(logr.Discard(), m, &noopAuditClient{}).WithLowConfidenceFloor(&floor)
		analysis := buildLowConfidenceFloorTestAnalysis()
		resp := buildIncidentResponseWithWorkflowConfidence(0.3) // below both the 0.5 custom floor and the 0.7 default

		_, err := processor.ProcessAgentSessionResult(ctx, analysis, resp)

		Expect(err).ToNot(HaveOccurred())
		Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
		Expect(analysis.Status.Message).To(ContainSubstring("below threshold 0.50"))
		Expect(analysis.Status.Message).ToNot(ContainSubstring("0.70"))
	})

	It("UT-AA-1828-004: the audit RecordAnalysisFailed error reports the configured floor (BR-AUDIT-005 content)", func() {
		floor := 0.5
		spy := &auditClientSpy{}
		processor := handlers.NewResponseProcessor(logr.Discard(), m, spy).WithLowConfidenceFloor(&floor)
		analysis := buildLowConfidenceFloorTestAnalysis()
		resp := buildIncidentResponseWithWorkflowConfidence(0.3)

		_, err := processor.ProcessAgentSessionResult(ctx, analysis, resp)

		Expect(err).ToNot(HaveOccurred())
		events := spy.getFailedEvents()
		Expect(events).To(HaveLen(1))
		Expect(events[0].err).To(MatchError(ContainSubstring("below threshold 0.50")))
	})

	// IT-AA-1828-001: proves the option is wired end-to-end through the production
	// entry point (NewInvestigatingHandler + Handle), not just reachable on a
	// directly-constructed ResponseProcessor.
	It("IT-AA-1828-001: handlers.WithLowConfidenceFloor wires the floor through NewInvestigatingHandler into the ResponseProcessor", func() {
		floor := 0.5
		mockClient := mocks.NewMockAgentClient()
		mockClient.WithResult(buildIncidentResponseWithWorkflowConfidence(0.6)) // below 0.7 default, above 0.5 custom floor
		handler := handlers.NewInvestigatingHandler(mockClient, logr.Discard(), m, &noopAuditClient{},
			handlers.WithLowConfidenceFloor(&floor))
		analysis := buildLowConfidenceFloorTestAnalysis()

		_, err := handler.Handle(ctx, analysis)

		Expect(err).ToNot(HaveOccurred())
		Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseAnalyzing),
			"0.6 confidence clears the operator-configured 0.5 floor wired via handlers.WithLowConfidenceFloor")
		Expect(analysis.Status.GetReview().NeedsHumanReview).To(BeFalse())
	})
})
