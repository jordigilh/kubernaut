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
	"encoding/json"

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

			It("should derive IsActionable=true when workflow_id is present and actionable is absent (GAP-002)", func() {
				p := parser.NewResultParser()
				result, err := p.Parse(`{
					"rca_summary": "Normal investigation",
					"workflow_id": "restart-pod",
					"confidence": 0.9
				}`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.IsActionable).NotTo(BeNil(),
					"GAP-002: outcome routing derives is_actionable from workflow_id presence")
				Expect(*result.IsActionable).To(BeTrue())
				Expect(result.Warnings).To(BeEmpty())
			})
		})
	})

	Describe("UT-KA-433-RCA-001: Parser extracts remediation_target from nested root_cause_analysis", func() {
		It("should extract remediation_target kind/name/namespace from root_cause_analysis", func() {
			p := parser.NewResultParser()
			content := `{
				"root_cause_analysis": {
					"summary": "OOMKilled due to memory leak in web-deploy",
					"remediation_target": {
						"kind": "Deployment",
						"name": "web-deploy",
						"namespace": "production"
					}
				},
				"selected_workflow": {
					"workflow_id": "oom-recovery",
					"confidence": 0.92
				}
			}`
			result, err := p.Parse(content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"))
			Expect(result.RemediationTarget.Name).To(Equal("web-deploy"))
			Expect(result.RemediationTarget.Namespace).To(Equal("production"))
		})
	})

	Describe("UT-KA-433-RCA-003: Hybrid JSON — flat rca_summary + nested remediation_target", func() {
		It("should extract remediation_target from nested RCA when flat path wins", func() {
			p := parser.NewResultParser()
			content := `{
				"rca_summary": "OOMKilled due to memory leak",
				"workflow_id": "oom-recovery",
				"confidence": 0.92,
				"root_cause_analysis": {
					"summary": "OOMKilled due to memory leak in web-deploy",
					"remediation_target": {
						"kind": "Deployment",
						"name": "web-deploy",
						"namespace": "production"
					}
				}
			}`
			result, err := p.Parse(content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RCASummary).To(Equal("OOMKilled due to memory leak"))
			Expect(result.WorkflowID).To(Equal("oom-recovery"))
			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"))
			Expect(result.RemediationTarget.Name).To(Equal("web-deploy"))
			Expect(result.RemediationTarget.Namespace).To(Equal("production"))
		})
	})

	Describe("UT-KA-433-RCA-004: camelCase remediationTarget accepted", func() {
		It("should extract remediationTarget (camelCase) from nested RCA", func() {
			p := parser.NewResultParser()
			content := `{
				"root_cause_analysis": {
					"summary": "CrashLoopBackOff due to config error",
					"remediationTarget": {
						"kind": "Deployment",
						"name": "api-server",
						"namespace": "staging"
					}
				},
				"selected_workflow": {
					"workflow_id": "rollback-deployment",
					"confidence": 0.88
				}
			}`
			result, err := p.Parse(content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"))
			Expect(result.RemediationTarget.Name).To(Equal("api-server"))
			Expect(result.RemediationTarget.Namespace).To(Equal("staging"))
		})
	})

	Describe("UT-KA-433-RCA-002: Parser handles missing remediation_target gracefully", func() {
		It("should produce empty RemediationTarget when not present in LLM JSON", func() {
			p := parser.NewResultParser()
			content := `{
				"root_cause_analysis": {
					"summary": "Transient network issue"
				},
				"selected_workflow": {
					"workflow_id": "no-op",
					"confidence": 0.85
				}
			}`
			result, err := p.Parse(content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationTarget.Kind).To(BeEmpty())
			Expect(result.RemediationTarget.Name).To(BeEmpty())
		})
	})

	Describe("UT-KA-SCHEMA-001: InvestigationResultSchema returns valid JSON Schema", func() {
		It("should return parseable JSON with required top-level keys", func() {
			schema := parser.InvestigationResultSchema()
			Expect(schema).NotTo(BeEmpty())

			var parsed map[string]interface{}
			err := json.Unmarshal(schema, &parsed)
			Expect(err).NotTo(HaveOccurred(), "schema must be valid JSON")
			Expect(parsed).To(HaveKey("type"))
			Expect(parsed["type"]).To(Equal("object"))
			Expect(parsed).To(HaveKey("properties"))
			Expect(parsed).To(HaveKey("required"))

			props := parsed["properties"].(map[string]interface{})
			Expect(props).To(HaveKey("root_cause_analysis"))
			Expect(props).To(HaveKey("selected_workflow"))
			Expect(props).To(HaveKey("confidence"))
			Expect(props).To(HaveKey("severity"))
			Expect(props).To(HaveKey("actionable"))
			Expect(props).To(HaveKey("needs_human_review"))
			Expect(props).To(HaveKey("detected_labels"))
		})
	})

	Describe("UT-KA-STRUCTURED-002: Top-level confidence parsed from nested format without selected_workflow", func() {
		It("should extract top-level confidence when no selected_workflow is present", func() {
			p := parser.NewResultParser()
			content := `{
				"root_cause_analysis": {
					"summary": "Transient network issue resolved itself"
				},
				"confidence": 0.82,
				"investigation_outcome": "problem_resolved",
				"actionable": false
			}`
			result, err := p.Parse(content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Confidence).To(BeNumerically("~", 0.82, 0.01),
				"top-level confidence must be extracted even without selected_workflow")
		})
	})

	Describe("UT-KA-STRUCTURED-003: DetectedLabels parsed from nested format", func() {
		It("should extract detected_labels from nested LLM response", func() {
			p := parser.NewResultParser()
			content := `{
				"root_cause_analysis": {
					"summary": "OOMKilled due to memory spike"
				},
				"selected_workflow": {
					"workflow_id": "oom-increase-memory",
					"confidence": 0.9
				},
				"confidence": 0.9,
				"detected_labels": {
					"app": "web-server",
					"team": "platform"
				}
			}`
			result, err := p.Parse(content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.DetectedLabels).To(HaveKeyWithValue("app", "web-server"))
			Expect(result.DetectedLabels).To(HaveKeyWithValue("team", "platform"))
		})
	})

	Describe("UT-KA-STRUCTURED-001: Nested selected_workflow fields propagated to InvestigationResult", func() {
		It("should extract rationale, parameters, and execution_engine from nested selected_workflow", func() {
			p := parser.NewResultParser()
			content := `{
				"root_cause_analysis": {
					"summary": "OOMKilled due to memory limit exceeded",
					"severity": "high",
					"signal_name": "OOMKilled",
					"contributing_factors": ["memory_leak", "traffic_spike"],
					"remediationTarget": {
						"kind": "Deployment",
						"name": "web-app",
						"namespace": "production"
					}
				},
				"selected_workflow": {
					"workflow_id": "oom-increase-memory",
					"confidence": 0.95,
					"rationale": "Memory limit increase addresses the OOM condition directly",
					"parameters": {"MEMORY_LIMIT_NEW": "512Mi"},
					"execution_engine": "job"
				},
				"alternative_workflows": [
					{"workflow_id": "restart-pod", "confidence": 0.60, "rationale": "Temporary fix"}
				],
				"confidence": 0.95,
				"investigation_outcome": "actionable",
				"actionable": true
			}`
			result, err := p.Parse(content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.WorkflowID).To(Equal("oom-increase-memory"))
			Expect(result.Confidence).To(BeNumerically("~", 0.95, 0.01))
			Expect(result.Reason).To(Equal("Memory limit increase addresses the OOM condition directly"),
				"rationale from selected_workflow must map to InvestigationResult.Reason")
			Expect(result.Parameters).To(HaveKeyWithValue("MEMORY_LIMIT_NEW", "512Mi"),
				"parameters from selected_workflow must propagate")
			Expect(result.ExecutionEngine).To(Equal("job"),
				"execution_engine from selected_workflow must propagate")

			Expect(result.RCASummary).To(ContainSubstring("OOMKilled"))
			Expect(result.Severity).To(Equal("high"))
			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"))

			Expect(result.AlternativeWorkflows).To(HaveLen(1))
			Expect(result.AlternativeWorkflows[0].Rationale).To(Equal("Temporary fix"))
		})
	})

	Describe("UT-KA-433-AP-021: problem_resolved suppresses not-actionable warning", func() {
		It("should emit Problem self-resolved but NOT Alert not actionable", func() {
			p := parser.NewResultParser()
			result, err := p.Parse(`{
				"rca_summary": "Transient OOM cleared after restart",
				"actionable": false,
				"investigation_outcome": "problem_resolved",
				"confidence": 0.85
			}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Warnings).To(ContainElement(ContainSubstring("Problem self-resolved")))
			Expect(result.Warnings).NotTo(ContainElement(ContainSubstring("Alert not actionable")),
				"problem_resolved outcome must suppress the generic not-actionable warning")
		})
	})
})
