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

// Package aianalysis contains unit tests for ResponseProcessor terminal handler
// status completeness — TotalAnalysisTime, conditions, and approval metadata.
//
// Issue #610: All 5 terminal handlers in response_processor.go must set the
// same status fields that the main analyzing.go success path sets.
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
	client "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

var _ = Describe("ResponseProcessor Terminal Handler Status Completeness (#610)", func() {
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

	createAnalysisWithStartedAt := func() *aianalysisv1.AIAnalysis {
		startedAt := metav1.NewTime(time.Now().Add(-5 * time.Second))
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-610",
				Namespace:  "default",
				UID:        types.UID("test-uid-610"),
				Generation: 1,
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationID: "test-rr-610",
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:     aianalysis.PhaseAnalyzing,
				StartedAt: &startedAt,
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
	// UT-AA-610-001: handleWorkflowResolutionFailureFromIncident
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-610-001: handleWorkflowResolutionFailureFromIncident sets TotalAnalysisTime and all conditions", func() {
		analysis := createAnalysisWithStartedAt()

		resp := &client.IncidentResponse{
			IncidentID:       "test-wrf-001",
			Analysis:         "Workflow resolution failed",
			NeedsHumanReview: client.NewOptBool(true),
			HumanReviewReason: client.OptNilHumanReviewReason{
				Value: client.HumanReviewReasonParameterValidationFailed,
				Set:   true,
			},
			Confidence: 0.8,
			Timestamp:  "2026-04-03T00:00:00Z",
			Warnings:   []string{"parameter validation failed"},
		}

		_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
		Expect(err).ToNot(HaveOccurred())

		Expect(analysis.Status.TotalAnalysisTime).To(BeNumerically(">", 0),
			"#610: TotalAnalysisTime must be calculated from StartedAt")

		assertCondition(analysis.Status.Conditions, aianalysis.ConditionInvestigationComplete,
			metav1.ConditionFalse, aianalysis.ReasonInvestigationFailed)
		assertCondition(analysis.Status.Conditions, aianalysis.ConditionAnalysisComplete,
			metav1.ConditionFalse, aianalysis.ReasonAnalysisFailed)
		assertCondition(analysis.Status.Conditions, aianalysis.ConditionWorkflowResolved,
			metav1.ConditionFalse, aianalysis.ReasonWorkflowResolutionFailed)
		assertCondition(analysis.Status.Conditions, aianalysis.ConditionApprovalRequired,
			metav1.ConditionFalse, "NotApplicable")
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-610-002: handleProblemResolvedFromIncident
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-610-002: handleProblemResolvedFromIncident sets TotalAnalysisTime and all conditions", func() {
		analysis := createAnalysisWithStartedAt()

		resp := &client.IncidentResponse{
			IncidentID:       "test-pr-001",
			Analysis:         "Problem self-resolved",
			NeedsHumanReview: client.NewOptBool(false),
			Confidence:       0.9,
			Timestamp:        "2026-04-03T00:00:00Z",
			Warnings:         []string{"Problem self-resolved: alert condition no longer active"},
		}

		_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
		Expect(err).ToNot(HaveOccurred())

		Expect(analysis.Status.TotalAnalysisTime).To(BeNumerically(">", 0),
			"#610: TotalAnalysisTime must be calculated from StartedAt")

		assertCondition(analysis.Status.Conditions, aianalysis.ConditionInvestigationComplete,
			metav1.ConditionTrue, aianalysis.ReasonInvestigationSucceeded)
		assertCondition(analysis.Status.Conditions, aianalysis.ConditionAnalysisComplete,
			metav1.ConditionTrue, aianalysis.ReasonAnalysisSucceeded)
		assertCondition(analysis.Status.Conditions, aianalysis.ConditionWorkflowResolved,
			metav1.ConditionFalse, aianalysis.ReasonNoWorkflowNeeded)
		assertCondition(analysis.Status.Conditions, aianalysis.ConditionApprovalRequired,
			metav1.ConditionFalse, "NotApplicable")
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-610-003: handleNotActionableFromIncident
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-610-003: handleNotActionableFromIncident sets TotalAnalysisTime and all conditions", func() {
		analysis := createAnalysisWithStartedAt()

		resp := &client.IncidentResponse{
			IncidentID:       "test-na-001",
			Analysis:         "Alert not actionable",
			NeedsHumanReview: client.NewOptBool(false),
			Confidence:       0.85,
			Timestamp:        "2026-04-03T00:00:00Z",
			Warnings:         []string{"Alert not actionable: condition is benign"},
			IsActionable:     client.NewOptNilBool(false),
		}

		_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
		Expect(err).ToNot(HaveOccurred())

		Expect(analysis.Status.TotalAnalysisTime).To(BeNumerically(">", 0),
			"#610: TotalAnalysisTime must be calculated from StartedAt")

		assertCondition(analysis.Status.Conditions, aianalysis.ConditionInvestigationComplete,
			metav1.ConditionTrue, aianalysis.ReasonInvestigationSucceeded)
		assertCondition(analysis.Status.Conditions, aianalysis.ConditionAnalysisComplete,
			metav1.ConditionTrue, aianalysis.ReasonAnalysisSucceeded)
		assertCondition(analysis.Status.Conditions, aianalysis.ConditionWorkflowResolved,
			metav1.ConditionFalse, aianalysis.ReasonNoWorkflowNeeded)
		assertCondition(analysis.Status.Conditions, aianalysis.ConditionApprovalRequired,
			metav1.ConditionFalse, "NotApplicable")
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-610-004: handleNoWorkflowTerminalFailure
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-610-004: handleNoWorkflowTerminalFailure sets TotalAnalysisTime and all conditions", func() {
		analysis := createAnalysisWithStartedAt()

		resp := &client.IncidentResponse{
			IncidentID:       "test-nw-001",
			Analysis:         "No workflow found",
			NeedsHumanReview: client.NewOptBool(false),
			Confidence:       0.3,
			Timestamp:        "2026-04-03T00:00:00Z",
			Warnings:         []string{"No workflows matched the alert criteria"},
		}

		_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
		Expect(err).ToNot(HaveOccurred())

		Expect(analysis.Status.TotalAnalysisTime).To(BeNumerically(">", 0),
			"#610: TotalAnalysisTime must be calculated from StartedAt")

		assertCondition(analysis.Status.Conditions, aianalysis.ConditionInvestigationComplete,
			metav1.ConditionFalse, aianalysis.ReasonInvestigationFailed)
		assertCondition(analysis.Status.Conditions, aianalysis.ConditionAnalysisComplete,
			metav1.ConditionFalse, aianalysis.ReasonAnalysisFailed)
		assertCondition(analysis.Status.Conditions, aianalysis.ConditionWorkflowResolved,
			metav1.ConditionFalse, aianalysis.ReasonWorkflowResolutionFailed)
		assertCondition(analysis.Status.Conditions, aianalysis.ConditionApprovalRequired,
			metav1.ConditionFalse, "NotApplicable")
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-610-005: handleLowConfidenceFailure
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-610-005: handleLowConfidenceFailure sets TotalAnalysisTime and all conditions", func() {
		analysis := createAnalysisWithStartedAt()

		resp := &client.IncidentResponse{
			IncidentID:       "test-lc-001",
			Analysis:         "Low confidence workflow",
			NeedsHumanReview: client.NewOptBool(false),
			Confidence:       0.3,
			Timestamp:        "2026-04-03T00:00:00Z",
			SelectedWorkflow: client.OptNilIncidentResponseSelectedWorkflow{
				Value: client.IncidentResponseSelectedWorkflow{
					"workflow_id":      jx.Raw(`"restart-pod-v1"`),
					"execution_bundle": jx.Raw(`"ghcr.io/kubernaut/restart-pod:v1.0"`),
					"confidence":       jx.Raw(`0.30`),
				},
				Set: true,
			},
		}

		_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
		Expect(err).ToNot(HaveOccurred())

		Expect(analysis.Status.TotalAnalysisTime).To(BeNumerically(">", 0),
			"#610: TotalAnalysisTime must be calculated from StartedAt")

		assertCondition(analysis.Status.Conditions, aianalysis.ConditionInvestigationComplete,
			metav1.ConditionFalse, aianalysis.ReasonInvestigationFailed)
		assertCondition(analysis.Status.Conditions, aianalysis.ConditionAnalysisComplete,
			metav1.ConditionFalse, aianalysis.ReasonAnalysisFailed)
		assertCondition(analysis.Status.Conditions, aianalysis.ConditionWorkflowResolved,
			metav1.ConditionFalse, aianalysis.ReasonWorkflowResolutionFailed)
		assertCondition(analysis.Status.Conditions, aianalysis.ConditionApprovalRequired,
			metav1.ConditionFalse, "NotApplicable")
	})

	// ═══════════════════════════════════════════════════════════════════════
	// UT-AA-610-006: TotalAnalysisTime when StartedAt is nil
	// ═══════════════════════════════════════════════════════════════════════

	It("UT-AA-610-006: TotalAnalysisTime remains 0 when StartedAt is nil", func() {
		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-610-nil-start",
				Namespace:  "default",
				UID:        types.UID("test-uid-610-nil"),
				Generation: 1,
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationID: "test-rr-610-nil",
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: aianalysis.PhaseAnalyzing,
			},
		}

		resp := &client.IncidentResponse{
			IncidentID:       "test-nil-start-001",
			Analysis:         "Test with nil StartedAt",
			NeedsHumanReview: client.NewOptBool(true),
			Confidence:       0.5,
			Timestamp:        "2026-04-03T00:00:00Z",
		}

		_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
		Expect(err).ToNot(HaveOccurred())

		Expect(analysis.Status.TotalAnalysisTime).To(BeZero(),
			"#610: TotalAnalysisTime must remain 0 when StartedAt is nil")
	})
})
