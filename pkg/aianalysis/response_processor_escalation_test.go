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

// Package aianalysis contains unit tests for the operator_escalation path
// in ResponseProcessor.
//
// Issue #1449: CRD enum missing operator_escalation causes infinite retry loop
// FedRAMP: IR-5 (Incident Monitoring), AU-12 (Audit Generation)
package aianalysis_test

import (
	"context"
	"time"

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

var _ = Describe("ResponseProcessor operator_escalation (#1449)", func() {
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
				Name:       "test-1449",
				Namespace:  "default",
				UID:        types.UID("test-uid-1449"),
				Generation: 1,
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationID: "test-rr-1449",
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:     aianalysis.PhaseInvestigating,
				StartedAt: &startedAt,
			},
		}
	}

	buildEscalationResp := func() *client.IncidentResponse {
		return &client.IncidentResponse{
			IncidentID:       "inc-1449-001",
			Analysis:         "Operator escalated: critical infrastructure alert requires human review.",
			NeedsHumanReview: client.NewOptBool(true),
			HumanReviewReason: client.OptNilHumanReviewReason{
				Value: client.HumanReviewReasonOperatorEscalation,
				Set:   true,
			},
			Confidence: 0.95,
			Timestamp:  "2026-06-17T17:06:10Z",
		}
	}

	// ═══════════════════════════════════════════════════════════════════════
	// Issue #1449 / FedRAMP IR-5: Escalation status must be persisted
	// ═══════════════════════════════════════════════════════════════════════

	Context("IR-5: Operator escalation via kubernaut_complete_no_action", func() {
		It("UT-AA-1449-001: sets Phase=Failed, NeedsHumanReview=true, HumanReviewReason=operator_escalation", func() {
			analysis := createAnalysis()
			resp := buildEscalationResp()

			_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
			Expect(err).ToNot(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"IR-5: Escalation must result in Failed phase to route to human review")
			Expect(analysis.Status.NeedsHumanReview).To(BeTrue(),
				"IR-5: NeedsHumanReview must be true for escalation routing")
			Expect(analysis.Status.HumanReviewReason).To(Equal("operator_escalation"),
				"IR-5/AU-12: operator_escalation must be persisted as the audit-trail reason for escalation")
			Expect(string(analysis.Status.Reason)).To(Equal("WorkflowResolutionFailed"),
				"IR-5: Reason must indicate workflow resolution failure requiring human intervention")
		})

		It("UT-AA-1449-002: maps operator_escalation to OperatorEscalation SubReason", func() {
			analysis := createAnalysis()
			resp := buildEscalationResp()

			_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
			Expect(err).ToNot(HaveOccurred())

			Expect(analysis.Status.SubReason).To(Equal("OperatorEscalation"),
				"AU-12: SubReason must map operator_escalation enum to structured OperatorEscalation value")
		})

		It("UT-AA-1449-003: sets CompletedAt timestamp for audit completeness", func() {
			analysis := createAnalysis()
			resp := buildEscalationResp()

			_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
			Expect(err).ToNot(HaveOccurred())

			Expect(analysis.Status.CompletedAt).ToNot(BeNil(),
				"AU-12: CompletedAt must be set for FedRAMP audit trail temporal completeness")
		})
	})
})
