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

	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
)

var _ = Describe("Kubernaut Agent Result Parser — #433", func() {

	Describe("UT-KA-433-021: Parser extracts InvestigationResult from valid JSON", func() {
		It("should parse a valid LLM JSON response into InvestigationResult", func() {
			validJSON := `{
				"rca_summary": "Container OOMKilled due to memory limit of 256Mi being exceeded under load",
				"workflow_id": "oom-increase-memory",
				"remediation_target": {
					"kind": "Deployment",
					"name": "api-server",
					"namespace": "production"
				},
				"parameters": {
					"memory_increase_pct": 50
				},
				"confidence": 0.92
			}`
			p := parser.NewResultParser()
			result, err := p.Parse(validJSON)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil(), "Parse should return a result for valid JSON")
			Expect(result.RCASummary).To(ContainSubstring("OOMKilled"))
			Expect(result.WorkflowID).To(Equal("oom-increase-memory"))
			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"))
			Expect(result.RemediationTarget.Name).To(Equal("api-server"))
			Expect(result.Confidence).To(BeNumerically("~", 0.92, 0.01))
		})
	})

	Describe("UT-KA-433-022: Parser returns structured error for malformed JSON", func() {
		It("should return an error for invalid JSON", func() {
			p := parser.NewResultParser()
			result, err := p.Parse(`{invalid json`)
			Expect(err).To(HaveOccurred(), "Parse should error on malformed JSON")
			Expect(result).To(BeNil())
		})

		It("should return an error for empty string", func() {
			p := parser.NewResultParser()
			result, err := p.Parse("")
			Expect(err).To(HaveOccurred(), "Parse should error on empty input")
			Expect(result).To(BeNil())
		})
	})

	Describe("UT-KA-433-023: Validator accepts workflow_id in allowlist", func() {
		It("should accept a workflow_id that is in the session allowlist", func() {
			v := parser.NewValidator([]string{"oom-increase-memory", "restart-pod", "rollback-deployment"})
			result := &katypes.InvestigationResult{
				WorkflowID: "oom-increase-memory",
				Confidence: 0.85,
			}
			err := v.Validate(result)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("UT-KA-433-024: Validator rejects workflow_id absent from allowlist", func() {
		It("should reject a workflow_id not in the session allowlist", func() {
			v := parser.NewValidator([]string{"oom-increase-memory", "restart-pod"})
			result := &katypes.InvestigationResult{
				WorkflowID: "delete-everything",
				Confidence: 0.9,
			}
			err := v.Validate(result)
			Expect(err).To(HaveOccurred(), "Validator should reject unlisted workflow_id")
			Expect(err.Error()).To(ContainSubstring("workflow"))
		})
	})

	Describe("UT-KA-433-025: Validator enforces parameter bounds", func() {
		It("should reject confidence outside [0,1] range", func() {
			v := parser.NewValidator([]string{"oom-increase-memory"})
			result := &katypes.InvestigationResult{
				WorkflowID: "oom-increase-memory",
				Confidence: 1.5,
			}
			err := v.Validate(result)
			Expect(err).To(HaveOccurred(), "Validator should reject confidence > 1.0")
		})

		It("should reject negative confidence", func() {
			v := parser.NewValidator([]string{"oom-increase-memory"})
			result := &katypes.InvestigationResult{
				WorkflowID: "oom-increase-memory",
				Confidence: -0.5,
			}
			err := v.Validate(result)
			Expect(err).To(HaveOccurred(), "Validator should reject negative confidence")
		})
	})

	Describe("UT-KA-433-026: Self-correction loop retries up to 3 times", func() {
		It("should retry correction and return corrected result on second attempt", func() {
			v := parser.NewValidator([]string{"oom-increase-memory"})
			attempt := 0
			badResult := &katypes.InvestigationResult{
				WorkflowID: "invalid-workflow",
				Confidence: 0.9,
			}
			correctedResult, err := v.SelfCorrect(badResult, 3, func(r *katypes.InvestigationResult, validationErr error) (*katypes.InvestigationResult, error) {
				attempt++
				if attempt >= 2 {
					return &katypes.InvestigationResult{
						WorkflowID: "oom-increase-memory",
						Confidence: 0.85,
					}, nil
				}
				return &katypes.InvestigationResult{
					WorkflowID: "still-invalid",
					Confidence: 0.9,
				}, nil
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(correctedResult).NotTo(BeNil(), "SelfCorrect should return a result")
			Expect(correctedResult.WorkflowID).To(Equal("oom-increase-memory"))
			Expect(attempt).To(BeNumerically(">=", 2))
		})
	})

	Describe("UT-KA-433-027: Self-correction exhaustion produces human-review flag", func() {
		It("should set HumanReviewNeeded when all attempts fail", func() {
			v := parser.NewValidator([]string{"oom-increase-memory"})
			badResult := &katypes.InvestigationResult{
				WorkflowID: "invalid-workflow",
				Confidence: 0.9,
			}
			exhaustedResult, err := v.SelfCorrect(badResult, 3, func(r *katypes.InvestigationResult, validationErr error) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{
					WorkflowID: "still-invalid",
					Confidence: 0.9,
				}, nil
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(exhaustedResult).NotTo(BeNil(), "SelfCorrect should return a result even on exhaustion")
			Expect(exhaustedResult.HumanReviewNeeded).To(BeTrue(),
				"HumanReviewNeeded should be true after all correction attempts fail")
		})
	})

	// ========================================
	// ISSUE #607: ACTIONABLE=FALSE CONFIDENCE FLOOR + SIGNAL SYNTHESIS
	// Go Kubernaut Agent must parse `actionable: false` from LLM JSON,
	// synthesize the warning string, set IsActionable, and apply
	// confidence floor of 0.8 for defense-in-depth.
	// ========================================
	Describe("KA Parser — Not-Actionable Signal Synthesis (#607)", func() {

		Describe("UT-KA-607-001: Parser applies confidence floor when actionable=false without confidence", func() {
			It("should set confidence to 0.8 when LLM omits confidence for not-actionable", func() {
				p := parser.NewResultParser()
				result, err := p.Parse(`{
					"rca_summary": "Orphaned PVCs from completed batch jobs",
					"actionable": false
				}`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Confidence).To(BeNumerically(">=", 0.8),
					"#607: Confidence floor of 0.8 must apply when actionable=false and confidence omitted")
			})
		})

		Describe("UT-KA-607-002: Parser applies confidence floor when actionable=false with low confidence", func() {
			It("should override low confidence to 0.8", func() {
				p := parser.NewResultParser()
				result, err := p.Parse(`{
					"rca_summary": "Old config artifacts in namespace",
					"actionable": false,
					"confidence": 0.3
				}`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Confidence).To(BeNumerically(">=", 0.8),
					"#607: Confidence floor overrides low LLM confidence for actionable=false")
			})
		})

		Describe("UT-KA-607-003: Parser synthesizes warning and sets IsActionable=false", func() {
			It("should produce the standard warning and set IsActionable pointer to false", func() {
				p := parser.NewResultParser()
				result, err := p.Parse(`{
					"rca_summary": "Orphaned PVCs not impacting workloads",
					"actionable": false,
					"confidence": 0.9
				}`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())

				Expect(result.IsActionable).NotTo(BeNil(),
					"#607: IsActionable must be set when LLM provides actionable field")
				Expect(*result.IsActionable).To(BeFalse(),
					"#607: IsActionable must be false")
				Expect(result.Warnings).To(ContainElement(ContainSubstring("Alert not actionable")),
					"#607: Standard warning string must be synthesized")
			})
		})

		Describe("UT-KA-607-004: Parser does NOT apply floor for actionable=true or absent", func() {
			It("should preserve original confidence when actionable is true", func() {
				p := parser.NewResultParser()
				result, err := p.Parse(`{
					"rca_summary": "OOMKilled due to memory pressure",
					"workflow_id": "oom-increase-memory",
					"actionable": true,
					"confidence": 0.3
				}`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Confidence).To(BeNumerically("~", 0.3, 0.01),
					"#607: Floor must NOT apply when actionable=true")
			})

			It("should preserve original confidence when actionable is absent", func() {
				p := parser.NewResultParser()
				result, err := p.Parse(`{
					"rca_summary": "Network partition detected",
					"confidence": 0.4
				}`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Confidence).To(BeNumerically("~", 0.4, 0.01),
					"#607: Floor must NOT apply when actionable is absent")
			})
		})

		Describe("UT-KA-607-005: InvestigationResult carries IsActionable and Warnings for response mapping", func() {
			It("should populate IsActionable=false and Warnings when actionable=false", func() {
				p := parser.NewResultParser()
				result, err := p.Parse(`{
					"rca_summary": "Stale config objects from previous deployment",
					"actionable": false,
					"confidence": 0.85
				}`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())

				Expect(result.IsActionable).NotTo(BeNil())
				Expect(*result.IsActionable).To(BeFalse())
				Expect(result.Warnings).NotTo(BeEmpty(),
					"#607: Warnings must be populated for response mapping to set IncidentResponse.Warnings")
			})

			It("should NOT populate IsActionable or Warnings when actionable is absent", func() {
				p := parser.NewResultParser()
				result, err := p.Parse(`{
					"rca_summary": "Normal investigation",
					"workflow_id": "restart-pod",
					"confidence": 0.9
				}`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.IsActionable).To(BeNil(),
					"IsActionable should be nil when LLM doesn't provide actionable field")
				Expect(result.Warnings).To(BeEmpty())
			})
		})
	})
})
