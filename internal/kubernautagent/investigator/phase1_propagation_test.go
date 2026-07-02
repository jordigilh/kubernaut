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

package investigator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("Phase 1 → Phase 3 forensic field propagation — #847", func() {

	Describe("UT-KA-847-010: BuildPhase1Context captures CausalChain and DueDiligence", func() {
		It("should include CausalChain in Phase1Data when present in RCA result", func() {
			rca := &katypes.InvestigationResult{
				RCASummary: "disk pressure from emptyDir overuse",
				Confidence: 0.92,
				CausalChain: []string{
					"PredictedDiskPressure alert fired on node",
					"Four deployments writing to unbounded emptyDir volumes",
					"Combined emptyDir limits exceed node allocatable storage",
				},
			}

			p1 := investigator.BuildPhase1Context(rca)
			Expect(p1).NotTo(BeNil())
			Expect(p1.CausalChain).To(Equal(rca.CausalChain))
		})

		It("should include DueDiligence in Phase1Data when present in RCA result", func() {
			dd := &katypes.DueDiligenceReview{
				CausalCompleteness:    "Traced to emptyDir sizing issue",
				TargetAccuracy:        "log-collector is primary continuous writer",
				EvidenceSufficiency:   "Backed by describe, logs, and metrics",
				AlternativeHypotheses: "Considered image layer size; ruled out",
				ScopeCompleteness:     "All 4 deployments investigated",
				Proportionality:       "Targeting primary offender among 4",
				RegressionAwareness:   "N/A — first incident",
				ConfidenceCalibration: "0.92 — reduced from 1.0 due to multi-contributor",
			}
			rca := &katypes.InvestigationResult{
				RCASummary:   "disk pressure",
				Confidence:   0.92,
				DueDiligence: dd,
			}

			p1 := investigator.BuildPhase1Context(rca)
			Expect(p1).NotTo(BeNil())
			Expect(p1.DueDiligence).To(Equal(dd))
		})
	})

	Describe("UT-KA-847-011: MergePhase1Fallbacks propagates CausalChain and DueDiligence to Phase 3 result", func() {
		It("should fill CausalChain from Phase 1 when Phase 3 has none", func() {
			chain := []string{"signal fired", "root cause identified"}
			p1 := &prompt.Phase1Data{
				CausalChain: chain,
			}
			result := &katypes.InvestigationResult{
				RCASummary: "workflow selected",
			}

			investigator.MergePhase1Fallbacks(result, p1)
			Expect(result.CausalChain).To(Equal(chain))
		})

		It("should fill DueDiligence from Phase 1 when Phase 3 has none", func() {
			dd := &katypes.DueDiligenceReview{
				CausalCompleteness: "full trace",
				TargetAccuracy:     "correct target",
			}
			p1 := &prompt.Phase1Data{
				DueDiligence: dd,
			}
			result := &katypes.InvestigationResult{
				RCASummary: "workflow selected",
			}

			investigator.MergePhase1Fallbacks(result, p1)
			Expect(result.DueDiligence).To(Equal(dd))
		})

		It("should NOT overwrite Phase 3 CausalChain if already set", func() {
			phase3Chain := []string{"phase 3 chain"}
			p1 := &prompt.Phase1Data{
				CausalChain: []string{"phase 1 chain"},
			}
			result := &katypes.InvestigationResult{
				CausalChain: phase3Chain,
			}

			investigator.MergePhase1Fallbacks(result, p1)
			Expect(result.CausalChain).To(Equal(phase3Chain))
		})

		// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 4 §7l-1: MergePhase1Fallbacks
		// characterization tests for branches not exercised above, written
		// before decomposing the function (cyclomatic 17).
		It("should be a no-op when result is nil", func() {
			p1 := &prompt.Phase1Data{Severity: "critical"}
			Expect(func() { investigator.MergePhase1Fallbacks(nil, p1) }).NotTo(Panic())
		})

		It("should be a no-op when p1 is nil", func() {
			result := &katypes.InvestigationResult{Severity: "warning"}
			investigator.MergePhase1Fallbacks(result, nil)
			Expect(result.Severity).To(Equal("warning"))
		})

		It("should fill Severity from Phase 1 when Phase 3 has none", func() {
			p1 := &prompt.Phase1Data{Severity: "critical"}
			result := &katypes.InvestigationResult{}
			investigator.MergePhase1Fallbacks(result, p1)
			Expect(result.Severity).To(Equal("critical"))
		})

		It("should NOT overwrite Phase 3 Severity if already set", func() {
			p1 := &prompt.Phase1Data{Severity: "critical"}
			result := &katypes.InvestigationResult{Severity: "warning"}
			investigator.MergePhase1Fallbacks(result, p1)
			Expect(result.Severity).To(Equal("warning"))
		})

		It("should fill ContributingFactors from Phase 1 when Phase 3 has none", func() {
			factors := []string{"disk pressure", "memory leak"}
			p1 := &prompt.Phase1Data{ContributingFactors: factors}
			result := &katypes.InvestigationResult{}
			investigator.MergePhase1Fallbacks(result, p1)
			Expect(result.ContributingFactors).To(Equal(factors))
		})

		It("should fill Confidence from Phase 1 when Phase 3 has zero value", func() {
			p1 := &prompt.Phase1Data{Confidence: 0.75}
			result := &katypes.InvestigationResult{}
			investigator.MergePhase1Fallbacks(result, p1)
			Expect(result.Confidence).To(Equal(0.75))
		})

		It("should NOT overwrite a non-zero Phase 3 Confidence", func() {
			p1 := &prompt.Phase1Data{Confidence: 0.75}
			result := &katypes.InvestigationResult{Confidence: 0.9}
			investigator.MergePhase1Fallbacks(result, p1)
			Expect(result.Confidence).To(Equal(0.9))
		})

		It("should fill InvestigationOutcome from Phase 1 and apply its side effects", func() {
			p1 := &prompt.Phase1Data{InvestigationOutcome: "predictive_no_action"}
			result := &katypes.InvestigationResult{}
			investigator.MergePhase1Fallbacks(result, p1)
			Expect(result.InvestigationOutcome).To(Equal("predictive_no_action"))
			Expect(result.IsActionable).NotTo(BeNil(),
				"ApplyInvestigationOutcome must run as a side effect of filling InvestigationOutcome")
			Expect(*result.IsActionable).To(BeFalse())
		})

		It("should clear a contradictory HumanReviewNeeded when Phase 1 outcome is problem_resolved (#301)", func() {
			p1 := &prompt.Phase1Data{InvestigationOutcome: "problem_resolved"}
			result := &katypes.InvestigationResult{
				HumanReviewNeeded: true,
				HumanReviewReason: "no_matching_workflows",
			}
			investigator.MergePhase1Fallbacks(result, p1)
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"#301: problem_resolved outcome from Phase 1 must override a contradictory Phase 3 HumanReviewNeeded")
			Expect(result.HumanReviewReason).To(BeEmpty())
		})

		It("should NOT overwrite an existing Phase 3 InvestigationOutcome", func() {
			p1 := &prompt.Phase1Data{InvestigationOutcome: "problem_resolved"}
			result := &katypes.InvestigationResult{
				InvestigationOutcome: "actionable",
				HumanReviewNeeded:    true,
			}
			investigator.MergePhase1Fallbacks(result, p1)
			Expect(result.InvestigationOutcome).To(Equal("actionable"))
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"the problem_resolved override only applies when Phase 3's outcome is actually filled from Phase 1")
		})

		It("should NOT overwrite Phase 3 DueDiligence if already set", func() {
			phase3DD := &katypes.DueDiligenceReview{CausalCompleteness: "phase 3"}
			p1 := &prompt.Phase1Data{DueDiligence: &katypes.DueDiligenceReview{CausalCompleteness: "phase 1"}}
			result := &katypes.InvestigationResult{DueDiligence: phase3DD}
			investigator.MergePhase1Fallbacks(result, p1)
			Expect(result.DueDiligence).To(Equal(phase3DD))
		})
	})

	Describe("UT-KA-847-012: ResultToAuditJSON includes forensic fields in audit event", func() {
		It("should serialize CausalChain and DueDiligence into audit map", func() {
			dd := &katypes.DueDiligenceReview{
				CausalCompleteness:    "complete",
				TargetAccuracy:        "accurate",
				EvidenceSufficiency:   "sufficient",
				AlternativeHypotheses: "none",
				ScopeCompleteness:     "complete",
				Proportionality:       "proportional",
				RegressionAwareness:   "N/A",
				ConfidenceCalibration: "0.95",
			}
			result := &katypes.InvestigationResult{
				RCASummary: "test summary",
				Confidence: 0.95,
				CausalChain: []string{
					"symptom observed",
					"root cause found",
				},
				DueDiligence: dd,
			}

			m := investigator.ResultToAuditJSON(result)

			chain, ok := m["causal_chain"].([]string)
			Expect(ok).To(BeTrue(), "causal_chain should be []string")
			Expect(chain).To(HaveLen(2))
			Expect(chain[0]).To(Equal("symptom observed"))

			ddMap, ok := m["due_diligence"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "due_diligence should be a map")
			Expect(ddMap["causal_completeness"]).To(Equal("complete"))
			Expect(ddMap["target_accuracy"]).To(Equal("accurate"))
			Expect(ddMap["evidence_sufficiency"]).To(Equal("sufficient"))
		})
	})
})
