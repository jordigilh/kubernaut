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

package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
)

var _ = Describe("TP-433-ADV P3: Parser + Outcome Routing — GAP-002/003", func() {

	var p *parser.ResultParser

	BeforeEach(func() {
		p = parser.NewResultParser()
	})

	// --- GAP-003: Multi-pattern parsing ---

	Describe("UT-KA-433-PRS-001: extractBalancedJSON extracts first complete JSON object", func() {
		It("should extract JSON embedded in prose text", func() {
			input := `Based on my analysis, here is the result:
{"rca_summary": "OOMKilled", "workflow_id": "oom-recovery", "confidence": 0.9}
That concludes my investigation.`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RCASummary).To(Equal("OOMKilled"))
			Expect(result.WorkflowID).To(Equal("oom-recovery"))
		})
	})

	Describe("UT-KA-433-PRS-002: extractBalancedJSON handles nested braces correctly", func() {
		It("should parse JSON with deeply nested objects", func() {
			input := `Here is my analysis:
{
  "rca_summary": "Config error in {\"key\": \"value\"} caused crash",
  "workflow_id": "crashloop-config-fix",
  "remediation_target": {"kind": "Deployment", "name": "api", "namespace": "prod"},
  "parameters": {"config": {"nested": "value"}},
  "confidence": 0.85
}
End of analysis.`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.WorkflowID).To(Equal("crashloop-config-fix"))
			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"))
		})
	})

	Describe("UT-KA-433-PRS-003: extractBalancedJSON returns empty on malformed input", func() {
		It("should return error when no JSON is found at all", func() {
			input := "This is just plain text without any JSON structure."
			result, err := p.Parse(input)
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("should return error for unclosed braces", func() {
			input := `{"rca_summary": "incomplete`
			result, err := p.Parse(input)
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})

	Describe("UT-KA-433-PRS-004: Pattern 2A — raw dict with root_cause_analysis key", func() {
		It("should parse nested LLM format with root_cause_analysis wrapper", func() {
			input := `{
				"root_cause_analysis": {
					"summary": "Memory pressure caused OOMKill"
				},
				"selected_workflow": {
					"workflow_id": "oom-recovery",
					"confidence": 0.88
				}
			}`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RCASummary).To(Equal("Memory pressure caused OOMKill"))
			Expect(result.WorkflowID).To(Equal("oom-recovery"))
			Expect(result.Confidence).To(BeNumerically("~", 0.88, 0.01))
		})
	})

	Describe("UT-KA-433-PRS-005: Pattern 2B — section headers parsed", func() {
		It("should handle markdown fenced JSON with leading text", func() {
			input := "## Analysis Result\n\n```json\n" +
				`{"rca_summary": "Disk full", "workflow_id": "disk-cleanup", "confidence": 0.7}` +
				"\n```\n\nPlease review."

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.WorkflowID).To(Equal("disk-cleanup"))
		})
	})

	Describe("UT-KA-433-PRS-006: Fallback chain — Pattern 1 → nested → balanced brace", func() {
		It("should prefer fenced JSON when available", func() {
			input := "Some text ```json\n{\"rca_summary\": \"fenced\", \"workflow_id\": \"w1\", \"confidence\": 0.9}\n```\n{\"rca_summary\": \"raw\", \"workflow_id\": \"w2\", \"confidence\": 0.5}"

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.WorkflowID).To(Equal("w1"), "fenced JSON should take priority")
		})
	})

	Describe("UT-KA-433-PRS-007: execution_bundle extracted from selected_workflow (GAP-009)", func() {
		It("should extract execution_bundle from nested selected_workflow", func() {
			input := `{
				"root_cause_analysis": {"summary": "OOMKill"},
				"selected_workflow": {
					"workflow_id": "oom-recovery",
					"execution_bundle": "ghcr.io/kubernaut/oom-recovery:v1.0@sha256:abc",
					"confidence": 0.92
				}
			}`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ExecutionBundle).To(Equal("ghcr.io/kubernaut/oom-recovery:v1.0@sha256:abc"))
		})

		It("should extract execution_bundle from flat format", func() {
			input := `{
				"rca_summary": "OOMKill",
				"workflow_id": "oom-recovery",
				"execution_bundle": "ghcr.io/kubernaut/oom:v1",
				"confidence": 0.9
			}`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ExecutionBundle).To(Equal("ghcr.io/kubernaut/oom:v1"))
		})
	})

	Describe("UT-KA-433-PRS-008: alternative_workflows extracted from LLM JSON (GAP-009)", func() {
		It("should extract alternative_workflows array from LLM response", func() {
			input := `{
				"rca_summary": "Memory leak in api-server",
				"workflow_id": "oom-recovery",
				"confidence": 0.85,
				"alternative_workflows": [
					{
						"workflow_id": "memory-optimize",
						"confidence": 0.65,
						"rationale": "Could optimize memory settings instead"
					},
					{
						"workflow_id": "horizontal-scale",
						"confidence": 0.5,
						"rationale": "Scaling out may distribute memory pressure"
					}
				]
			}`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.AlternativeWorkflows).To(HaveLen(2))
			Expect(result.AlternativeWorkflows[0].WorkflowID).To(Equal("memory-optimize"))
			Expect(result.AlternativeWorkflows[0].Rationale).To(ContainSubstring("optimize"))
			Expect(result.AlternativeWorkflows[1].WorkflowID).To(Equal("horizontal-scale"))
		})
	})

	Describe("UT-KA-433-PRS-009: Parser ignores LLM needs_human_review (BR-HAPI-200)", func() {
		It("should NOT propagate LLM-set needs_human_review; HR derived from investigation_outcome only", func() {
			input := `{
				"rca_summary": "Unclear root cause — multiple potential issues",
				"needs_human_review": true,
				"human_review_reason": "investigation_inconclusive",
				"confidence": 0.3
			}`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"LLM-set needs_human_review must be ignored — HR is parser-derived only")
			Expect(result.HumanReviewReason).To(BeEmpty(),
				"LLM-set human_review_reason must be ignored — HR reason is parser-derived only")
		})
	})

	// --- BR-HAPI-200: Parser-derived escalation ---

	Describe("UT-KA-700-PDE-001: inconclusive + RCA + no workflow → no_matching_workflows", func() {
		It("should derive no_matching_workflows from context signals", func() {
			input := `{
				"root_cause_analysis": {
					"summary": "Memory pressure detected but no remediation workflow available"
				},
				"investigation_outcome": "inconclusive",
				"confidence": 0.4
			}`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"inconclusive outcome must set HumanReviewNeeded")
			Expect(result.HumanReviewReason).To(Equal("no_matching_workflows"),
				"RCA present + no workflow + inconclusive = no_matching_workflows (BR-HAPI-197)")
		})
	})

	Describe("UT-KA-700-PDE-002: inconclusive + workflow present → investigation_inconclusive fallback", func() {
		It("should derive investigation_inconclusive when workflow is present despite inconclusive outcome", func() {
			input := `{
				"rca_summary": "",
				"workflow_id": "restart-pod",
				"investigation_outcome": "inconclusive",
				"confidence": 0.3
			}`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.HumanReviewNeeded).To(BeTrue(),
				"inconclusive outcome must set HumanReviewNeeded")
			Expect(result.HumanReviewReason).To(Equal("investigation_inconclusive"),
				"workflow present + inconclusive → investigation_inconclusive fallback")
		})
	})

	Describe("UT-KA-700-PDE-003: problem_resolved contradiction override clears HR (#301)", func() {
		It("should clear needs_human_review when problem_resolved contradicts LLM-set HR", func() {
			input := `{
				"rca_summary": "Problem self-resolved. Transient OOM cleared after pod restart",
				"investigation_outcome": "problem_resolved",
				"needs_human_review": true,
				"human_review_reason": "contradictory_signals",
				"actionable": false,
				"confidence": 0.85
			}`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"#301: problem_resolved must override needs_human_review=true")
			Expect(result.HumanReviewReason).To(BeEmpty(),
				"#301: problem_resolved must clear human_review_reason")
		})
	})

	// --- GAP-002: Outcome routing ---

	Describe("UT-KA-433-OUT-001: actionable=false → is_actionable=false (GAP-002)", func() {
		It("should set is_actionable=false when LLM signals not actionable", func() {
			input := `{
				"rca_summary": "Alert is informational, no remediation needed",
				"actionable": false,
				"confidence": 0.95
			}`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsActionable).NotTo(BeNil())
			Expect(*result.IsActionable).To(BeFalse())
		})
	})

	Describe("UT-KA-433-OUT-002: Self-resolved language → is_actionable=false", func() {
		It("should detect problem_resolved investigation outcome", func() {
			input := `{
				"rca_summary": "The issue has been resolved automatically by Kubernetes",
				"investigation_outcome": "problem_resolved",
				"confidence": 0.9
			}`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsActionable).NotTo(BeNil())
			Expect(*result.IsActionable).To(BeFalse())
		})
	})

	Describe("UT-KA-433-OUT-003: inconclusive + RCA + no workflow → no_matching_workflows (BR-HAPI-197)", func() {
		It("should derive no_matching_workflows when RCA present but no workflow selected", func() {
			input := `{
				"rca_summary": "Unable to determine root cause with available data",
				"investigation_outcome": "inconclusive",
				"confidence": 0.2
			}`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.HumanReviewNeeded).To(BeTrue())
			Expect(result.HumanReviewReason).To(Equal("no_matching_workflows"),
				"inconclusive + RCA present + no workflow = no_matching_workflows per BR-HAPI-197")
		})
	})

	Describe("UT-KA-433-OUT-004: LLM explicit needs_human_review must NOT be preserved (BR-HAPI-200)", func() {
		It("should ignore LLM-set needs_human_review when workflow is present", func() {
			input := `{
				"rca_summary": "Found issue but confidence too low",
				"workflow_id": "restart-pod",
				"confidence": 0.35,
				"needs_human_review": true,
				"human_review_reason": "low_confidence"
			}`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"LLM-set needs_human_review must NOT be preserved — HR is parser-derived only")
			Expect(result.HumanReviewReason).To(BeEmpty(),
				"LLM-set human_review_reason must NOT be preserved — HR reason is parser-derived only")
		})
	})

	Describe("UT-KA-433-OUT-005: Confidence floor for non-actionable scenarios", func() {
		It("should apply confidence floor when not actionable", func() {
			input := `{
				"rca_summary": "Informational alert",
				"actionable": false,
				"confidence": 0.1
			}`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Confidence).To(BeNumerically(">=", 0.8),
				"Non-actionable results should have confidence floor applied")
		})
	})

	Describe("UT-KA-433-OUT-006: Workflow present + valid → is_actionable=true", func() {
		It("should derive is_actionable=true when workflow is selected", func() {
			input := `{
				"rca_summary": "Memory limit exceeded",
				"workflow_id": "oom-recovery",
				"confidence": 0.9
			}`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsActionable).NotTo(BeNil())
			Expect(*result.IsActionable).To(BeTrue())
		})
	})

	// ===== Audit findings =====

	Describe("AUDIT-H5: actionable=false takes precedence over investigation_outcome=actionable", func() {
		It("should preserve actionable=false even when outcome says actionable", func() {
			input := `{
				"rca_summary": "Conflicting signals from LLM",
				"actionable": false,
				"investigation_outcome": "actionable",
				"confidence": 0.5
			}`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsActionable).NotTo(BeNil())
			Expect(*result.IsActionable).To(BeFalse(),
				"H5: explicit actionable=false must not be overridden by investigation_outcome")
			Expect(result.Warnings).To(ContainElement(ContainSubstring("not actionable")),
				"H5: not-actionable warning should be preserved")
		})
	})

	Describe("AUDIT-M3: extractBalancedJSON skips prose braces", func() {
		It("should extract JSON even when prose contains curly braces", func() {
			input := `Based on my analysis {of the system}, here is the result:
{"rca_summary": "OOMKilled due to memory leak", "confidence": 0.85, "workflow_id": "oom-recovery"}`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RCASummary).To(Equal("OOMKilled due to memory leak"),
				"M3: should skip prose brace and find actual JSON object")
			Expect(result.WorkflowID).To(Equal("oom-recovery"))
		})
	})

	Describe("AUDIT-H6: actionable=false + workflow_id logs warning", func() {
		It("should set is_actionable=false with workflow present and add warning", func() {
			input := `{
				"rca_summary": "Found issue but marked not actionable",
				"workflow_id": "restart-pod",
				"actionable": false,
				"confidence": 0.7
			}`

			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsActionable).NotTo(BeNil())
			Expect(*result.IsActionable).To(BeFalse(),
				"H6: actionable=false should be respected even with workflow_id")
			Expect(result.WorkflowID).To(Equal("restart-pod"),
				"H6: workflow_id should still be preserved")
		})
	})

	Describe("CI-1058-SEV: Severity extraction for AA CRD compliance", func() {
		It("should extract top-level severity from flat response", func() {
			input := `{"rca_summary": "OOM detected", "severity": "critical", "confidence": 0.9}`
			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("critical"))
		})

		It("should extract nested severity from root_cause_analysis", func() {
			input := `{"root_cause_analysis": {"summary": "OOM", "severity": "high"}, "confidence": 0.8}`
			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("high"))
		})

		It("should prefer top-level severity over nested when both present", func() {
			input := `{
				"root_cause_analysis": {"summary": "OOM", "severity": "low"},
				"severity": "critical",
				"confidence": 0.9
			}`
			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("critical"),
				"top-level severity should take precedence")
		})

		It("should handle Mock LLM response format with both paths", func() {
			input := `{
				"root_cause_analysis": {
					"summary": "Container exceeded memory limits",
					"severity": "critical",
					"contributing_factors": ["traffic_spike"]
				},
				"severity": "critical",
				"confidence": 0.95,
				"investigation_outcome": "actionable",
				"actionable": true,
				"selected_workflow": {
					"workflow_id": "oomkill-increase-memory-v1",
					"confidence": 0.95,
					"execution_engine": "job"
				}
			}`
			result, err := p.Parse(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("critical"))
			Expect(result.IsActionable).NotTo(BeNil())
			Expect(*result.IsActionable).To(BeTrue())
			Expect(result.WorkflowID).To(Equal("oomkill-increase-memory-v1"))
		})
	})
})
