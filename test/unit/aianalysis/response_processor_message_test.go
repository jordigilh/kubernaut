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

// Issue #588: Validates that Status.Message and Status.Warnings are independent fields.
// Bug 3 fix: response_processor must not append warnings to Status.Message.
//
// Business Requirements:
//   - BR-HAPI-197: NeedsHumanReview handling — Status.Message for validation errors, Status.Warnings for warnings
//   - BR-ORCH-036: Manual review notification body — Details and Warnings sections must not duplicate content
package aianalysis

import (
	"context"

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

var _ = Describe("Issue #588: Status.Message and Status.Warnings Independence", func() {
	var (
		processor *handlers.ResponseProcessor
		analysis  *aianalysisv1.AIAnalysis
		ctx       context.Context
	)

	BeforeEach(func() {
		m := metrics.NewMetrics()
		processor = handlers.NewResponseProcessor(logr.Discard(), m, &noopAuditClient{})
		ctx = context.Background()
	})

	createAnalysis := func() *aianalysisv1.AIAnalysis {
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-msg-588",
				Namespace: "default",
				UID:       types.UID("test-uid-588"),
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationID: "test-rr-588",
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: aianalysis.PhaseInvestigating,
			},
		}
	}

	// UT-AA-588-001: Status.Message contains only validation attempt errors, not warnings.
	// Operator should see validation errors in the Details section and warnings separately
	// in the Warnings section — no duplication.
	It("UT-AA-588-001: Status.Message contains only validation attempt errors, not warnings", func() {
		analysis = createAnalysis()

		resp := &client.IncidentResponse{
			IncidentID:       "test-588-001",
			Analysis:         "Analysis text",
			NeedsHumanReview: client.NewOptBool(true),
			HumanReviewReason: client.OptNilHumanReviewReason{
				Value: client.HumanReviewReasonParameterValidationFailed,
				Set:   true,
			},
			Confidence: 0.3,
			Timestamp:  "2026-03-04T12:00:00Z",
			ValidationAttemptsHistory: []client.ValidationAttempt{
				{Attempt: 1, IsValid: false, Errors: []string{"missing workflow_id"}, Timestamp: "2026-03-04T12:00:01Z"},
				{Attempt: 2, IsValid: false, Errors: []string{"invalid parameters"}, Timestamp: "2026-03-04T12:00:02Z"},
			},
			Warnings: []string{"LLM output was not structured JSON", "Retry budget exhausted"},
		}

		_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
		Expect(err).ToNot(HaveOccurred())

		// Message must contain ONLY validation attempt errors
		Expect(analysis.Status.Message).To(ContainSubstring("Attempt 1: missing workflow_id"))
		Expect(analysis.Status.Message).To(ContainSubstring("Attempt 2: invalid parameters"))

		// Message must NOT contain any warning text
		Expect(analysis.Status.Message).ToNot(ContainSubstring("LLM output was not structured JSON"))
		Expect(analysis.Status.Message).ToNot(ContainSubstring("Retry budget exhausted"))

		// Warnings must be set independently
		Expect(analysis.Status.Warnings).To(ConsistOf("LLM output was not structured JSON", "Retry budget exhausted"))
	})

	// UT-AA-588-002: Status.Warnings is populated independently from Status.Message.
	// When there are no validation attempts but warnings exist, Warnings should be populated.
	// Note: no_matching_workflows routes to handleNoMatchingWorkflowsCompleted per #768,
	// which sets an informational Message. Use a different humanReviewReason for this test.
	It("UT-AA-588-002: Status.Warnings is populated independently when no validation attempts exist", func() {
		analysis = createAnalysis()

		resp := &client.IncidentResponse{
			IncidentID:       "test-588-002",
			Analysis:         "Analysis text",
			NeedsHumanReview: client.NewOptBool(true),
			HumanReviewReason: client.OptNilHumanReviewReason{
				Value: client.HumanReviewReasonWorkflowNotFound,
				Set:   true,
			},
			Confidence:                0.2,
			Timestamp:                 "2026-03-04T12:00:00Z",
			ValidationAttemptsHistory: nil,
			Warnings:                  []string{"Workflow not found in catalog", "Search scope was empty"},
		}

		_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
		Expect(err).ToNot(HaveOccurred())

		// Message must be empty — no validation attempts means no attempt errors
		Expect(analysis.Status.Message).To(BeEmpty(),
			"Status.Message must be empty when there are no validation attempts")

		// Warnings must still be populated
		Expect(analysis.Status.Warnings).To(ConsistOf("Workflow not found in catalog", "Search scope was empty"))
	})

	// UT-AA-588-003: Both fields empty when no validation attempts and no warnings.
	// No misleading content in either field.
	It("UT-AA-588-003: both fields empty when no validation attempts and no warnings", func() {
		analysis = createAnalysis()

		resp := &client.IncidentResponse{
			IncidentID:       "test-588-003",
			Analysis:         "Analysis text",
			NeedsHumanReview: client.NewOptBool(true),
			HumanReviewReason: client.OptNilHumanReviewReason{
				Value: client.HumanReviewReasonWorkflowNotFound,
				Set:   true,
			},
			Confidence:                0.1,
			Timestamp:                 "2026-03-04T12:00:00Z",
			ValidationAttemptsHistory: nil,
			Warnings:                  nil,
		}

		_, err := processor.ProcessIncidentResponse(ctx, analysis, resp)
		Expect(err).ToNot(HaveOccurred())

		Expect(analysis.Status.Message).To(BeEmpty(),
			"Status.Message must be empty when no validation attempts and no warnings")
		Expect(analysis.Status.Warnings).To(BeNil(),
			"Status.Warnings must be nil when resp.Warnings is nil")
	})
})
