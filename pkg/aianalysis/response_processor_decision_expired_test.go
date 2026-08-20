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

// Package aianalysis contains unit tests for the decision_expired path
// in ResponseProcessor.
//
// Issue #2019/#2020: KA finalizes a discovered-but-unanswered-decision session
// with has_workflow=false on inactivity timeout, discarding the real
// recommendation. This test proves that once KA reports
// human_review_reason=decision_expired AND a selected_workflow, AA's
// ResponseProcessor preserves both onto the CRD -- it must NOT be treated
// like no_matching_workflows (which has no workflow at all).
// FedRAMP: AU-2/AU-3 (accurate audit record), AC-6/CM-3 (no silent auto-approval).
package aianalysis_test

import (
	"context"
	"time"

	"github.com/go-faster/jx"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	client "github.com/jordigilh/kubernaut/pkg/agentclient"
)

var _ = Describe("ResponseProcessor decision_expired (#2019/#2020)", func() {
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
				Name:       "test-2020",
				Namespace:  "default",
				UID:        types.UID("test-uid-2020"),
				Generation: 1,
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationID: "test-rr-2020",
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:     aianalysis.PhaseInvestigating,
				StartedAt: &startedAt,
			},
		}
	}

	buildDecisionExpiredResp := func() *client.IncidentResponse {
		return &client.IncidentResponse{
			IncidentID:       "inc-2020-001",
			Analysis:         "Workflow discovered and presented, but no decision was received before the inactivity timeout.",
			NeedsHumanReview: client.NewOptBool(true),
			HumanReviewReason: client.OptNilHumanReviewReason{
				Value: client.HumanReviewReasonDecisionExpired,
				Set:   true,
			},
			Confidence: 0.9,
			Timestamp:  "2026-08-08T12:00:00Z",
			SelectedWorkflow: client.OptNilIncidentResponseSelectedWorkflow{
				Value: client.IncidentResponseSelectedWorkflow{
					"workflow_id": jx.Raw(`"wf-recommended-2020"`),
					"confidence":  jx.Raw(`0.9`),
					"rationale":   jx.Raw(`"restart the crashlooping pod"`),
				},
				Set: true,
			},
		}
	}

	// ═══════════════════════════════════════════════════════════════════════
	// Issue #2019/#2020 / FedRAMP AU-2, AU-3, AC-6, CM-3: A presented-but-
	// unanswered decision must be recorded accurately, and the discovered
	// workflow must survive for retroactive human action -- never silently
	// discarded.
	// ═══════════════════════════════════════════════════════════════════════

	Context("AU-2/AU-3: decision_expired via kubernaut_discover_workflows + inactivity timeout", func() {
		It("UT-AA-2020-006: sets Phase=Failed, NeedsHumanReview=true, HumanReviewReason=decision_expired", func() {
			analysis := createAnalysis()
			resp := buildDecisionExpiredResp()

			_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
			Expect(err).ToNot(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"AU-2/AU-3: decision_expired must result in Failed phase to route to human review")
			Expect(analysis.Status.GetReview().NeedsHumanReview).To(BeTrue(),
				"AC-6/CM-3: NeedsHumanReview must be true -- a presented decision with no answer is never auto-approved")
			Expect(analysis.Status.GetReview().HumanReviewReason).To(Equal("decision_expired"),
				"AU-2/AU-3: decision_expired must be persisted as the audit-trail reason")
		})

		It("UT-AA-2020-007: maps decision_expired to DecisionExpired SubReason", func() {
			analysis := createAnalysis()
			resp := buildDecisionExpiredResp()

			_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
			Expect(err).ToNot(HaveOccurred())

			Expect(analysis.Status.SubReason).To(Equal("DecisionExpired"),
				"AU-2/AU-3: SubReason must map decision_expired enum to structured DecisionExpired value")
		})

		It("UT-AA-2020-008: preserves the discovered SelectedWorkflow instead of discarding it (the actual #2019/#2020 bug)", func() {
			analysis := createAnalysis()
			resp := buildDecisionExpiredResp()

			_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
			Expect(err).ToNot(HaveOccurred())

			Expect(analysis.Status.GetRCAResult().SelectedWorkflow).NotTo(BeNil(),
				"AU-2/AU-3: the discovered workflow must survive a decision_expired outcome -- "+
					"this is the false negative #2019/#2020 fixes (previously has_workflow:false discarded it)")
			Expect(analysis.Status.GetRCAResult().SelectedWorkflow.WorkflowID).To(Equal("wf-recommended-2020"))
		})

		It("UT-AA-2020-009: never sets ApprovalRequired=true for an unanswered decision", func() {
			analysis := createAnalysis()
			resp := buildDecisionExpiredResp()

			_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
			Expect(err).ToNot(HaveOccurred())

			Expect(analysis.Status.GetApproval().ApprovalRequired).To(BeFalse(),
				"AC-6/CM-3: no human ever approved this workflow -- decision_expired must never imply consent")
		})
	})
})
