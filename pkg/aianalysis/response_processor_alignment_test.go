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

package aianalysis_test

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
)

var _ = Describe("Fix 7: AA response processor AlignmentVerdict mapping", func() {
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
				Name:       "test-schema-alignment",
				Namespace:  "default",
				UID:        types.UID("test-uid-schema"),
				Generation: 1,
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationID: "test-rr-schema",
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:     aianalysis.PhaseAnalyzing,
				StartedAt: &startedAt,
			},
		}
	}

	// UT-AA-SCHEMA-001: alignment_verdict present -> maps to AIAnalysisStatus.GetReview().AlignmentVerdict
	It("UT-AA-SCHEMA-001: maps AlignmentVerdict from AgentSessionResult to AIAnalysisStatus", func() {
		analysis := createAnalysis()

		res := &agentsessionv1.AgentSessionResult{
			IncidentID:        "aa-schema-001",
			Analysis:          "RCA summary",
			Confidence:        0.85,
			Timestamp:         time.Now().Format(time.RFC3339),
			NeedsHumanReview:  true,
			HumanReviewReason: "alignment_check_failed",
			Warnings:          []string{"Shadow agent flagged suspicious content"},
			AlignmentVerdict: &agentsessionv1.AgentSessionAlignmentVerdict{
				Result:                  "suspicious",
				CircuitBreakerActivated: true,
				Summary:                 "Suspicious tool call detected",
				Flagged:                 2,
				Total:                   5,
				Findings: []agentsessionv1.AgentSessionAlignmentFinding{
					{
						StepIndex:   1,
						StepKind:    "tool_result",
						Tool:        "kubectl_get",
						Explanation: "Data exfiltration pattern",
					},
				},
			},
		}

		_, err := processor.ProcessAgentSessionResult(ctx, analysis, res)
		Expect(err).NotTo(HaveOccurred())

		// Verify AlignmentVerdict mapped to CRD status
		Expect(analysis.Status.GetReview().AlignmentVerdict).NotTo(BeNil(),
			"AlignmentVerdict must be mapped to AIAnalysisStatus")
		Expect(analysis.Status.GetReview().AlignmentVerdict.Result).To(Equal("suspicious"))
		Expect(analysis.Status.GetReview().AlignmentVerdict.CircuitBreakerActivated).To(BeTrue())
		Expect(analysis.Status.GetReview().AlignmentVerdict.Summary).To(Equal("Suspicious tool call detected"))
		Expect(analysis.Status.GetReview().AlignmentVerdict.Flagged).To(Equal(2))
		Expect(analysis.Status.GetReview().AlignmentVerdict.Total).To(Equal(5))
		Expect(analysis.Status.GetReview().AlignmentVerdict.Findings).To(HaveLen(1))
		Expect(analysis.Status.GetReview().AlignmentVerdict.Findings[0].StepKind).To(Equal("tool_result"))
		Expect(analysis.Status.GetReview().AlignmentVerdict.Findings[0].Tool).To(Equal("kubectl_get"))
	})

	// UT-AA-SCHEMA-002: alignment_verdict absent -> AlignmentVerdict nil on CRD (no crash)
	It("UT-AA-SCHEMA-002: absent AlignmentVerdict does not crash and leaves CRD field nil", func() {
		analysis := createAnalysis()

		res := &agentsessionv1.AgentSessionResult{
			IncidentID:        "aa-schema-002",
			Analysis:          "Normal RCA",
			Confidence:        0.9,
			Timestamp:         time.Now().Format(time.RFC3339),
			NeedsHumanReview:  true,
			HumanReviewReason: "low_confidence",
		}

		// AlignmentVerdict is NOT set (nil)
		_, err := processor.ProcessAgentSessionResult(ctx, analysis, res)
		Expect(err).NotTo(HaveOccurred())

		Expect(analysis.Status.GetReview().AlignmentVerdict).To(BeNil(),
			"AlignmentVerdict must be nil when not present in the result")
	})
})
