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

// Package aianalysis contains unit tests for the no_matching_workflows handler
// in ResponseProcessor.
//
// Issue #768: Phase should be Completed (not Failed) when humanReviewReason=no_matching_workflows
// Issue #769: rootCauseAnalysis must be preserved (rootCause must be populated)
package aianalysis

import (
	"context"
	"time"

	"github.com/go-faster/jx"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	client "github.com/jordigilh/kubernaut/pkg/agentclient"
)

var _ = Describe("ResponseProcessor no_matching_workflows (#768, #769)", func() {
	var (
		processor *handlers.ResponseProcessor
		ctx       context.Context
		m         *metrics.Metrics
	)

	BeforeEach(func() {
		m = metrics.NewMetrics()
		processor = handlers.NewResponseProcessor(logr.Discard(), m, &noopAuditClient{})
		ctx = context.Background()
	})

	createAnalysis := func() *aianalysisv1.AIAnalysis {
		startedAt := metav1.NewTime(time.Now().Add(-5 * time.Second))
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-768",
				Namespace:  "default",
				UID:        types.UID("test-uid-768"),
				Generation: 1,
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationID: "test-rr-768",
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:     aianalysis.PhaseInvestigating,
				StartedAt: &startedAt,
			},
		}
	}

	buildNoMatchingWorkflowsResp := func() *client.IncidentResponse {
		return &client.IncidentResponse{
			IncidentID:       "inc-768-001",
			Analysis:         "The namespace-quota ResourceQuota in demo-quota is 100% exhausted on memory.",
			NeedsHumanReview: client.NewOptBool(true),
			HumanReviewReason: client.OptNilHumanReviewReason{
				Value: client.HumanReviewReasonNoMatchingWorkflows,
				Set:   true,
			},
			Confidence: 0.98,
			Timestamp:  "2026-04-21T00:00:00Z",
			Warnings:   []string{"No workflows matched the alert criteria"},
			RootCauseAnalysis: client.IncidentResponseRootCauseAnalysis{
				"summary":             jx.Raw(`"The namespace-quota ResourceQuota is exhausted"`),
				"severity":            jx.Raw(`"medium"`),
				"contributing_factors": jx.Raw(`["ResourceQuota caps memory at 512Mi","Each pod requests 256Mi"]`),
				"remediationTarget": jx.Raw(`{"kind":"Deployment","name":"api-server","namespace":"demo-quota"}`),
			},
		}
	}

	assertCondition := func(conditions []metav1.Condition, condType string, expectedStatus metav1.ConditionStatus, expectedReason string) {
		cond := meta.FindStatusCondition(conditions, condType)
		ExpectWithOffset(1, cond).ToNot(BeNil(), "Condition %s must be set", condType)
		ExpectWithOffset(1, cond.Status).To(Equal(expectedStatus),
			"Condition %s status: expected %s, got %s", condType, expectedStatus, cond.Status)
		ExpectWithOffset(1, cond.Reason).To(Equal(expectedReason),
			"Condition %s reason: expected %s, got %s", condType, expectedReason, cond.Reason)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// Issue #768: Phase should be Completed for no_matching_workflows
	// ═══════════════════════════════════════════════════════════════════════

	Context("#768: Phase correctness for no_matching_workflows", func() {
		It("UT-AA-768-001: sets Phase=Completed when KA returns needsHumanReview=true + humanReviewReason=no_matching_workflows", func() {
			analysis := createAnalysis()
			resp := buildNoMatchingWorkflowsResp()

			_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
			Expect(err).ToNot(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseCompleted),
				"#768: Phase must be Completed — investigation succeeded, only workflow selection was empty")
			Expect(string(analysis.Status.Reason)).To(Equal("AnalysisCompleted"),
				"#768: Reason should be AnalysisCompleted, not WorkflowResolutionFailed")
			Expect(analysis.Status.SubReason).To(Equal("NoMatchingWorkflows"))
			Expect(analysis.Status.NeedsHumanReview).To(BeTrue(),
				"#768: NeedsHumanReview must remain true")
			Expect(analysis.Status.HumanReviewReason).To(Equal("no_matching_workflows"))
		})

		It("UT-AA-768-002: sets InvestigationComplete=True and AnalysisComplete=True for no_matching_workflows", func() {
			analysis := createAnalysis()
			resp := buildNoMatchingWorkflowsResp()

			_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
			Expect(err).ToNot(HaveOccurred())

			assertCondition(analysis.Status.Conditions, aianalysis.ConditionInvestigationComplete,
				metav1.ConditionTrue, aianalysis.ReasonInvestigationSucceeded)
			assertCondition(analysis.Status.Conditions, aianalysis.ConditionAnalysisComplete,
				metav1.ConditionTrue, aianalysis.ReasonAnalysisSucceeded)
		})

		It("UT-AA-768-003: sets WorkflowResolved=False with reason NoMatchingWorkflows", func() {
			analysis := createAnalysis()
			resp := buildNoMatchingWorkflowsResp()

			_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
			Expect(err).ToNot(HaveOccurred())

			assertCondition(analysis.Status.Conditions, aianalysis.ConditionWorkflowResolved,
				metav1.ConditionFalse, aianalysis.ReasonNoMatchingWorkflows)
			assertCondition(analysis.Status.Conditions, aianalysis.ConditionApprovalRequired,
				metav1.ConditionFalse, "NotApplicable")
		})

		It("UT-AA-768-004: calls RecordAnalysisComplete (not RecordAnalysisFailed)", func() {
			auditSpy := &spyAuditClient{}
			proc := handlers.NewResponseProcessor(logr.Discard(), m, auditSpy)
			analysis := createAnalysis()
			resp := buildNoMatchingWorkflowsResp()

			_, err := proc.ProcessIncidentResponse(ctx, analysis, resp)
			Expect(err).ToNot(HaveOccurred())

			Expect(auditSpy.completeCount).To(Equal(1),
				"#768: RecordAnalysisComplete must be called exactly once")
			Expect(auditSpy.failedCount).To(Equal(0),
				"#768: RecordAnalysisFailed must NOT be called for no_matching_workflows")
		})

		It("UT-AA-768-005: preserves Phase=Failed for other humanReviewReasons", func() {
			otherReasons := []client.HumanReviewReason{
				client.HumanReviewReasonParameterValidationFailed,
				client.HumanReviewReasonImageMismatch,
				client.HumanReviewReasonLowConfidence,
				client.HumanReviewReasonLlmParsingError,
				client.HumanReviewReasonInvestigationInconclusive,
				client.HumanReviewReasonRcaIncomplete,
			}

			for _, reason := range otherReasons {
				analysis := createAnalysis()
				resp := &client.IncidentResponse{
					IncidentID:       "inc-768-other",
					Analysis:         "Test other reason",
					NeedsHumanReview: client.NewOptBool(true),
					HumanReviewReason: client.OptNilHumanReviewReason{
						Value: reason,
						Set:   true,
					},
					Confidence: 0.5,
					Timestamp:  "2026-04-21T00:00:00Z",
				}

				_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
				Expect(err).ToNot(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
					"Phase must remain Failed for humanReviewReason=%s", reason)
			}
		})
	})

	// ═══════════════════════════════════════════════════════════════════════
	// Issue #769: rootCauseAnalysis preservation
	// ═══════════════════════════════════════════════════════════════════════

	Context("#769: RCA preservation for no_matching_workflows", func() {
		It("UT-AA-769-001: sets rootCause to RCA summary string", func() {
			analysis := createAnalysis()
			resp := buildNoMatchingWorkflowsResp()

			_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
			Expect(err).ToNot(HaveOccurred())

			Expect(analysis.Status.RootCause).To(Equal("The namespace-quota ResourceQuota is exhausted"),
				"#769: rootCause must contain the RCA summary, not 'N/A'")
		})

		It("UT-AA-769-002: preserves full rootCauseAnalysis struct with remediationTarget", func() {
			analysis := createAnalysis()
			resp := buildNoMatchingWorkflowsResp()

			_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
			Expect(err).ToNot(HaveOccurred())

			Expect(analysis.Status.RootCauseAnalysis).ToNot(BeNil(),
				"#769: rootCauseAnalysis must be set")
			Expect(analysis.Status.RootCauseAnalysis.Summary).To(
				Equal("The namespace-quota ResourceQuota is exhausted"))
			Expect(analysis.Status.RootCauseAnalysis.Severity).To(Equal("medium"))
			Expect(analysis.Status.RootCauseAnalysis.ContributingFactors).To(HaveLen(2))
			Expect(analysis.Status.RootCauseAnalysis.RemediationTarget.Kind).To(Equal("Deployment"))
			Expect(analysis.Status.RootCauseAnalysis.RemediationTarget.Name).To(Equal("api-server"))
			Expect(analysis.Status.RootCauseAnalysis.RemediationTarget.Namespace).To(Equal("demo-quota"))
		})

		It("UT-AA-769-003: handles nil RCA gracefully — no panic, fields remain empty", func() {
			analysis := createAnalysis()
			resp := buildNoMatchingWorkflowsResp()
			resp.RootCauseAnalysis = nil

			_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
			Expect(err).ToNot(HaveOccurred())

			Expect(analysis.Status.RootCause).To(BeEmpty())
			Expect(analysis.Status.RootCauseAnalysis).To(BeNil())
			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseCompleted),
				"Phase must still be Completed even without RCA")
		})
	})
})

// spyAuditClient tracks which audit methods are called for assertion.
type spyAuditClient struct {
	noopAuditClient
	completeCount int
	failedCount   int
}

func (s *spyAuditClient) RecordAnalysisComplete(_ context.Context, _ *aianalysisv1.AIAnalysis) {
	s.completeCount++
}

func (s *spyAuditClient) RecordAnalysisFailed(_ context.Context, _ *aianalysisv1.AIAnalysis, _ error) error {
	s.failedCount++
	return nil
}
