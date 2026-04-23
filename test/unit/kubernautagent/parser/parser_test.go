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
			Expect(props).NotTo(HaveKey("needs_human_review"),
				"needs_human_review is parser-derived, not exposed to LLM (BR-HAPI-200)")
			Expect(props).NotTo(HaveKey("human_review_reason"),
				"human_review_reason is parser-derived, not exposed to LLM (BR-HAPI-200)")
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

	Describe("UT-KA-686-001: section-header format from Vertex AI (no structured output)", func() {
		It("should parse # header format with workflow selection", func() {
			sectionContent := `I've completed the investigation. Here are my findings:

# root_cause_analysis
{"summary": "Bad Deployment rollout patched ConfigMap ref from worker-config to worker-config-bad", "severity": "critical", "contributing_factors": ["bad patch", "invalid directive", "no admission webhook"], "remediation_target": {"kind": "Deployment", "name": "worker", "namespace": "demo-crashloop"}}

# confidence
0.98

# selected_workflow
{"workflow_id": "f871d3c0-4c88-55aa-a412-7defebe000a3", "confidence": 0.98, "rationale": "crashloop-rollback-v1 is an exact match", "parameters": {"TARGET_RESOURCE_NAMESPACE": "demo-crashloop", "TARGET_RESOURCE_NAME": "worker", "TARGET_RESOURCE_KIND": "Deployment"}}

# alternative_workflows
[{"workflow_id": "crashloop-rollback-risk-v1", "confidence": 0.90, "rationale": "valid but over-engineered"}]
`
			p := parser.NewResultParser()
			result, err := p.Parse(sectionContent)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			By("RCA fields extracted")
			Expect(result.RCASummary).To(ContainSubstring("Bad Deployment rollout"))
			Expect(result.Severity).To(Equal("critical"))
			Expect(result.RemediationTarget.Kind).To(Equal("Deployment"))
			Expect(result.RemediationTarget.Name).To(Equal("worker"))
			Expect(result.RemediationTarget.Namespace).To(Equal("demo-crashloop"))

			By("Workflow selection extracted")
			Expect(result.WorkflowID).To(Equal("f871d3c0-4c88-55aa-a412-7defebe000a3"))
			Expect(result.Confidence).To(BeNumerically("~", 0.98, 0.01))
			Expect(result.Reason).To(ContainSubstring("exact match"))
			Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_NAMESPACE", "demo-crashloop"))
			Expect(result.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_NAME", "worker"))

			By("Alternatives extracted")
			Expect(result.AlternativeWorkflows).To(HaveLen(1))
			Expect(result.AlternativeWorkflows[0].WorkflowID).To(Equal("crashloop-rollback-risk-v1"))
		})

		It("should parse section headers with JSON in markdown code blocks", func() {
			fencedContent := `Here are my findings:

# root_cause_analysis
` + "```json" + `
{"summary": "OOMKilled pod", "severity": "high", "remediation_target": {"kind": "Deployment", "name": "api", "namespace": "prod"}}
` + "```" + `

# confidence
0.92

# selected_workflow
` + "```json" + `
{"workflow_id": "oom-fix-v1", "confidence": 0.92, "rationale": "increase memory", "parameters": {"MEMORY": "512Mi"}}
` + "```" + `
`
			p := parser.NewResultParser()
			result, err := p.Parse(fencedContent)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RCASummary).To(Equal("OOMKilled pod"))
			Expect(result.WorkflowID).To(Equal("oom-fix-v1"))
			Expect(result.Confidence).To(BeNumerically("~", 0.92, 0.01))
		})

		It("should parse section headers with RCA only (no workflow)", func() {
			rcaOnly := `# root_cause_analysis
{"summary": "Transient network issue", "severity": "low"}

# confidence
0.80

# actionable
false
`
			p := parser.NewResultParser()
			result, err := p.Parse(rcaOnly)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RCASummary).To(Equal("Transient network issue"))
			Expect(result.WorkflowID).To(BeEmpty())
		})
	})

	Describe("Phase 1-to-Phase 3 Propagation — #715", func() {

		Describe("UT-KA-715-004: Parser preserves raw investigation_outcome on result", func() {
			It("should store the raw investigation_outcome string on InvestigationResult", func() {
				p := parser.NewResultParser()
				result, err := p.Parse(`{
					"rca_summary": "Inconclusive investigation — multiple potential causes",
					"investigation_outcome": "inconclusive",
					"confidence": 0.4
				}`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.InvestigationOutcome).To(Equal("inconclusive"),
					"UT-KA-715-004: parser must preserve raw investigation_outcome string on result")
			})

			It("should store problem_resolved investigation_outcome", func() {
				p := parser.NewResultParser()
				result, err := p.Parse(`{
					"rca_summary": "Problem self-resolved after pod restart",
					"investigation_outcome": "problem_resolved",
					"confidence": 0.85
				}`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.InvestigationOutcome).To(Equal("problem_resolved"),
					"UT-KA-715-004: parser must preserve problem_resolved outcome")
			})

			It("should store actionable investigation_outcome", func() {
				p := parser.NewResultParser()
				result, err := p.Parse(`{
					"rca_summary": "OOMKilled due to memory limit",
					"investigation_outcome": "actionable",
					"confidence": 0.9
				}`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.InvestigationOutcome).To(Equal("actionable"),
					"UT-KA-715-004: parser must preserve actionable outcome")
			})

			It("should leave InvestigationOutcome empty when not provided by LLM", func() {
				p := parser.NewResultParser()
				result, err := p.Parse(`{
					"rca_summary": "OOMKilled due to memory limit",
					"confidence": 0.9
				}`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.InvestigationOutcome).To(BeEmpty(),
					"UT-KA-715-004: InvestigationOutcome must be empty when LLM doesn't provide it")
			})

			It("should preserve investigation_outcome from nested LLM format", func() {
				p := parser.NewResultParser()
				result, err := p.Parse(`{
					"root_cause_analysis": {
						"summary": "Memory pressure on api-server"
					},
					"investigation_outcome": "inconclusive",
					"confidence": 0.35
				}`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.InvestigationOutcome).To(Equal("inconclusive"),
					"UT-KA-715-004: parser must preserve investigation_outcome from nested format")
			})
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

	Describe("Investigation Analysis Field — #724", func() {

		Describe("UT-KA-724-001: Parser extracts investigation_analysis from nested LLM response", func() {
			It("should populate InvestigationAnalysis from root_cause_analysis.investigation_analysis", func() {
				p := parser.NewResultParser()
				content := `{
					"root_cause_analysis": {
						"summary": "OOMKilled due to memory leak in api-server",
						"severity": "high",
						"investigation_analysis": "The investigation revealed a steady memory growth pattern in the api-server container over the past 6 hours. Memory usage increased from 180Mi to 256Mi, hitting the container limit. The leak appears to correlate with increased gRPC streaming connections that are not being properly closed."
					},
					"confidence": 0.88,
					"investigation_outcome": "actionable"
				}`
				result, err := p.Parse(content)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.InvestigationAnalysis).To(Equal(
					"The investigation revealed a steady memory growth pattern in the api-server container over the past 6 hours. Memory usage increased from 180Mi to 256Mi, hitting the container limit. The leak appears to correlate with increased gRPC streaming connections that are not being properly closed."),
					"Parser must extract investigation_analysis from nested RCA into InvestigationResult.InvestigationAnalysis")
			})

			It("should leave InvestigationAnalysis empty when not present in LLM response", func() {
				p := parser.NewResultParser()
				content := `{
					"root_cause_analysis": {
						"summary": "OOMKilled due to memory limit exceeded"
					},
					"confidence": 0.9,
					"investigation_outcome": "actionable"
				}`
				result, err := p.Parse(content)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.InvestigationAnalysis).To(BeEmpty(),
					"InvestigationAnalysis must be empty when not provided by LLM (backward compat)")
			})

			It("should extract investigation_analysis from hybrid JSON (flat rca_summary + nested RCA)", func() {
				p := parser.NewResultParser()
				content := `{
					"rca_summary": "OOMKilled due to memory leak",
					"workflow_id": "oom-increase-memory",
					"confidence": 0.92,
					"root_cause_analysis": {
						"summary": "OOMKilled due to memory leak in api-server",
						"investigation_analysis": "Memory grew from 180Mi to 256Mi over 6h due to unclosed gRPC streams."
					}
				}`
				result, err := p.Parse(content)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.RCASummary).To(Equal("OOMKilled due to memory leak"),
					"flat rca_summary must take precedence")
				Expect(result.InvestigationAnalysis).To(Equal(
					"Memory grew from 180Mi to 256Mi over 6h due to unclosed gRPC streams."),
					"investigation_analysis must be merged from nested RCA in hybrid JSON (F2)")
			})
		})
	})

	// ========================================
	// ISSUE #746: NO-MATCHING-WORKFLOW MISCLASSIFICATION FIX
	// Parser must correctly classify LLM responses where no workflow matches
	// as no_matching_workflows instead of llm_parsing_error. Achieves behavioral
	// parity with HAPI v1.2.1 fallback chain (BR-HAPI-197.2).
	// ========================================
	Describe("KA Parser — No-Matching-Workflow Misclassification (#746)", func() {

		Describe("UT-KA-746-001: camelCase rootCauseAnalysis extracted by parseLLMFormat", func() {
			It("should extract RCA fields from camelCase rootCauseAnalysis", func() {
				p := parser.NewResultParser()
				content := `{
					"rootCauseAnalysis": {
						"summary": "ResourceQuota memory limit exceeded",
						"severity": "medium",
						"contributing_factors": ["quota ceiling", "pod request size"]
					},
					"selected_workflow": {
						"workflow_id": "patch-quota",
						"confidence": 0.85
					},
					"confidence": 0.85
				}`
				result, err := p.Parse(content)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.RCASummary).To(Equal("ResourceQuota memory limit exceeded"),
					"#746: camelCase rootCauseAnalysis.summary must be extracted")
				Expect(result.Severity).To(Equal("medium"),
					"#746: camelCase rootCauseAnalysis.severity must be extracted")
				Expect(result.ContributingFactors).To(ConsistOf("quota ceiling", "pod request size"),
					"#746: camelCase rootCauseAnalysis.contributing_factors must be extracted")
			})
		})

		Describe("UT-KA-746-002: camelCase RCA + snake_case workflow both extracted", func() {
			It("should extract both camelCase RCA and snake_case workflow", func() {
				p := parser.NewResultParser()
				content := `{
					"rootCauseAnalysis": {
						"summary": "CrashLoopBackOff due to missing ConfigMap",
						"severity": "high",
						"remediationTarget": {
							"kind": "Deployment",
							"name": "web-app",
							"namespace": "production"
						}
					},
					"selected_workflow": {
						"workflow_id": "rollback-deployment",
						"confidence": 0.90,
						"rationale": "Previous revision was stable"
					},
					"confidence": 0.90
				}`
				result, err := p.Parse(content)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.RCASummary).To(Equal("CrashLoopBackOff due to missing ConfigMap"),
					"#746: camelCase RCA summary extracted")
				Expect(result.WorkflowID).To(Equal("rollback-deployment"),
					"#746: snake_case selected_workflow.workflow_id extracted alongside camelCase RCA")
				Expect(result.RemediationTarget.Kind).To(Equal("Deployment"),
					"#746: camelCase remediationTarget extracted from camelCase RCA")
			})
		})

		Describe("UT-KA-746-003: No workflow + confidence > 0 derives no_matching_workflows", func() {
			It("should set HumanReviewNeeded and no_matching_workflows when no workflow selected", func() {
				p := parser.NewResultParser()
				content := `{
					"root_cause_analysis": {
						"summary": "ResourceQuota memory limit fully consumed"
					},
					"confidence": 0.95
				}`
				result, err := p.Parse(content)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.HumanReviewNeeded).To(BeTrue(),
					"#746: No workflow selected must trigger HumanReviewNeeded (BR-HAPI-197.2)")
				Expect(result.HumanReviewReason).To(Equal("no_matching_workflows"),
					"#746: Parser must derive no_matching_workflows when no workflow selected (HAPI parity)")
			})
		})

		Describe("UT-KA-746-004: Truly unrecognizable JSON still rejected", func() {
			It("should return error for JSON with no recognized fields", func() {
				p := parser.NewResultParser()
				content := `{"foo": "bar", "baz": 42}`
				result, err := p.Parse(content)
				Expect(err).To(HaveOccurred(),
					"#746: Unrecognizable JSON must still be rejected (defense-in-depth)")
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("no recognized fields"))
			})
		})

		Describe("UT-KA-746-005: investigation_outcome=inconclusive + RCA + no workflow -> no_matching_workflows", func() {
			It("should derive no_matching_workflows from inconclusive outcome with RCA and no workflow", func() {
				p := parser.NewResultParser()
				content := `{
					"root_cause_analysis": {
						"summary": "Disk pressure detected but no automated remediation available"
					},
					"confidence": 0.88,
					"investigation_outcome": "inconclusive"
				}`
				result, err := p.Parse(content)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.HumanReviewNeeded).To(BeTrue(),
					"#746: inconclusive + no workflow must trigger HumanReviewNeeded")
				Expect(result.HumanReviewReason).To(Equal("no_matching_workflows"),
					"#746: inconclusive + RCA summary + no workflow must derive no_matching_workflows")
			})
		})

		Describe("UT-KA-746-006: Workflow selected -> no false HR escalation", func() {
			It("should not set HumanReviewNeeded when a workflow is selected", func() {
				p := parser.NewResultParser()
				content := `{
					"root_cause_analysis": {
						"summary": "OOMKilled due to memory pressure"
					},
					"selected_workflow": {
						"workflow_id": "oom-increase-memory",
						"confidence": 0.92
					},
					"confidence": 0.92
				}`
				result, err := p.Parse(content)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.HumanReviewNeeded).To(BeFalse(),
					"#746: Workflow selected must NOT trigger HumanReviewNeeded (no regression)")
				Expect(result.WorkflowID).To(Equal("oom-increase-memory"))
			})
		})

		Describe("UT-KA-746-007: problem_resolved outcome -> no HR escalation", func() {
			It("should not set HumanReviewNeeded for problem_resolved outcome", func() {
				p := parser.NewResultParser()
				content := `{
					"root_cause_analysis": {
						"summary": "Network partition self-healed"
					},
					"confidence": 0.90,
					"investigation_outcome": "problem_resolved",
					"actionable": false
				}`
				result, err := p.Parse(content)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.HumanReviewNeeded).To(BeFalse(),
					"#746: problem_resolved must NOT trigger HumanReviewNeeded (no regression)")
				Expect(result.Warnings).To(ContainElement(ContainSubstring("Problem self-resolved")))
			})
		})

		Describe("UT-KA-746-008: Golden transcript — exact #746 audit JSON", func() {
			It("should classify exact #746 audit response as no_matching_workflows", func() {
				p := parser.NewResultParser()
				content := `{
					"needsHumanReview": true,
					"confidence": 0.98,
					"analysis": "The namespace-quota ResourceQuota memory limit (512Mi) is fully consumed by 2 running api-server pods (2x256Mi). The deployment requests 3 replicas but only 2 fit within the quota ceiling.",
					"rootCauseAnalysis": {
						"severity": "medium",
						"remediationTarget": {
							"kind": "Deployment",
							"name": "api-server",
							"namespace": "demo-quota"
						},
						"contributingFactors": [
							"ResourceQuota sets a hard limit of 512Mi",
							"Each pod requests/limits 256Mi — only 2 fit",
							"3x256Mi=768Mi exceeds 512Mi ceiling",
							"No remediation history — first-time mismatch",
							"Failure is purely quota enforcement"
						]
					}
				}`
				result, err := p.Parse(content)
				Expect(err).NotTo(HaveOccurred(),
					"#746: Golden transcript must parse successfully, not return llm_parsing_error")
				Expect(result).NotTo(BeNil())
				Expect(result.Confidence).To(BeNumerically("~", 0.98, 0.01),
					"#746: confidence must be preserved from LLM response")
				Expect(result.HumanReviewNeeded).To(BeTrue(),
					"#746: No workflow selected must trigger HumanReviewNeeded")
				Expect(result.HumanReviewReason).To(Equal("no_matching_workflows"),
					"#746: Must be classified as no_matching_workflows, not llm_parsing_error")
			})
		})
	})

	Describe("UT-KA-795: Parser handles double-serialized RCA JSON", func() {
		var p *parser.ResultParser
		BeforeEach(func() {
			p = parser.NewResultParser()
		})

		Describe("UT-KA-795-P01: root_cause_analysis as escaped JSON string with remediation_target", func() {
			It("should unwrap the double-serialized string and extract remediation_target", func() {
				content := `{"root_cause_analysis":"{\"summary\": \"CrashLoopBackOff caused by invalid directive\", \"severity\": \"critical\", \"signal_name\": \"KubePodCrashLooping\", \"contributing_factors\": [\"ConfigMap contains invalid_directive\"], \"remediation_target\": {\"kind\": \"Deployment\", \"name\": \"web-frontend\", \"namespace\": \"demo-gitops\"}}","confidence":0.98}`
				result, err := p.Parse(content)
				Expect(err).NotTo(HaveOccurred(),
					"UT-KA-795-P01: double-serialized root_cause_analysis must not cause parse failure")
				Expect(result).NotTo(BeNil())
				Expect(result.RCASummary).To(ContainSubstring("CrashLoopBackOff"),
					"UT-KA-795-P01: RCA summary must be extracted from unwrapped string")
				Expect(result.RemediationTarget).To(Equal(katypes.RemediationTarget{
					Kind: "Deployment", Name: "web-frontend", Namespace: "demo-gitops",
				}), "UT-KA-795-P01: remediation_target must be extracted from unwrapped string")
			})
		})

		Describe("UT-KA-795-P02: rootCauseAnalysis (camelCase) as escaped JSON string", func() {
			It("should unwrap the camelCase double-serialized string", func() {
				content := `{"rootCauseAnalysis":"{\"summary\": \"Node disk pressure\", \"remediation_target\": {\"kind\": \"Node\", \"name\": \"worker-1\", \"namespace\": \"\"}}","confidence":0.85}`
				result, err := p.Parse(content)
				Expect(err).NotTo(HaveOccurred(),
					"UT-KA-795-P02: camelCase double-serialized RCA must not cause parse failure")
				Expect(result).NotTo(BeNil())
				Expect(result.RCASummary).To(ContainSubstring("disk pressure"))
				Expect(result.RemediationTarget.Kind).To(Equal("Node"))
			})
		})

		Describe("UT-KA-795-P03: root_cause_analysis string that is NOT valid JSON", func() {
			It("should not false-positive unwrap a plain text string value", func() {
				content := `{"root_cause_analysis":"This is just a plain text summary, not JSON","confidence":0.7}`
				_, err := p.Parse(content)
				Expect(err).To(HaveOccurred(),
					"UT-KA-795-P03: plain text string in root_cause_analysis must still fail (no false-positive unwrap)")
			})
		})
	})
})
