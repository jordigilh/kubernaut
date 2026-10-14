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

package session_test

import (
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("Marshal RCA Subset — TP-1395-1396 (#1396)", func() {
	Describe("UT-KA-2496-001: safe current selection is distinct from RCA and execution state (BR-KA-016, SI-10)", func() {
		It("redacts credentials in new reasoning while retaining JSON-safe ordinary evidence", func() {
			result := &katypes.InvestigationResult{
				WorkflowID: "catalog-id-2496", WorkflowName: "increase-memory",
				RepeatJustification: &sharedtypes.RepeatJustification{EvidenceBasis: "Current limit; password=sentinel-credential-2496", PriorOutcomeRelevance: "Prior Full zero", ExpectedMechanism: "Increase \"memory\"\nwithout retrying the old limit"},
			}
			wire := session.MarshalRCASubset(result)
			var projected map[string]interface{}
			Expect(json.Unmarshal(wire, &projected)).To(Succeed())
			Expect(projected).To(HaveKey("selected_workflow"))
			Expect(string(wire)).NotTo(ContainSubstring("sentinel-credential-2496"))
			selected := projected["selected_workflow"].(map[string]interface{})
			Expect(selected["repeat_justification"]).To(HaveKeyWithValue("expected_mechanism", result.RepeatJustification.ExpectedMechanism))
			Expect(result.RepeatJustification.EvidenceBasis).To(ContainSubstring("sentinel-credential-2496"), "projection must not mutate stored reasoning")
		})

		It("omits oversized or invalid UTF-8 rejected parts instead of manufacturing apparently valid truncated reasoning", func() {
			result := &katypes.InvestigationResult{
				ProposedWorkflowID: "rejected-2496", WorkflowName: "increase-memory", RepeatValidationOutcome: "rejected",
				RepeatJustification: &sharedtypes.RepeatJustification{EvidenceBasis: strings.Repeat("界", 2001), PriorOutcomeRelevance: string([]byte{0xff}), ExpectedMechanism: "Bounded valid part"},
			}
			var projected map[string]interface{}
			Expect(json.Unmarshal(session.MarshalRCASubset(result), &projected)).To(Succeed())
			Expect(projected).To(HaveKey("selection_assessment"))
			assessment := projected["selection_assessment"].(map[string]interface{})
			Expect(assessment["repeat_justification"]).NotTo(HaveKey("evidence_basis"))
			Expect(assessment["repeat_justification"]).NotTo(HaveKey("prior_outcome_relevance"))
			Expect(assessment["repeat_justification"]).To(HaveKeyWithValue("expected_mechanism", "Bounded valid part"))
		})

		It("preserves explicit zero selection confidence and trusted repeat context without private execution fields", func() {
			rcaConfidence, selectionConfidence, required, matched := .93, 0., true, true
			result := &katypes.InvestigationResult{
				Severity: "critical", RCASummary: "Current memory pressure", Confidence: selectionConfidence,
				RCAConfidence: &rcaConfidence, WorkflowSelectionConfidence: &selectionConfidence,
				WorkflowID: "catalog-id-2496", WorkflowName: "increase-memory",
				RepeatJustification: &sharedtypes.RepeatJustification{
					EvidenceBasis: "Current memory limit observed", PriorOutcomeRelevance: "Prior Full assessment was ineffective", ExpectedMechanism: "Increase the limit"},
				RepeatJustificationRequired: &required, PriorWorkflowMatch: &matched, RepeatValidationOutcome: "accepted",
				HandoffDigest: "sha256:" + strings.Repeat("b", 64), PriorRemediationUIDs: []string{"private-prior-2496"},
				ExecutionBundle: "private-bundle-2496", Parameters: map[string]interface{}{"PASSWORD": "private-parameter-2496"},
			}
			var projected map[string]interface{}
			wire := session.MarshalRCASubset(result)
			Expect(json.Unmarshal(wire, &projected)).To(Succeed())
			Expect(projected).To(HaveKeyWithValue("confidence", .93), "RCA confidence must not be replaced by selection confidence")
			Expect(projected).To(HaveKey("selected_workflow"))
			selected := projected["selected_workflow"].(map[string]interface{})
			Expect(selected).To(HaveKeyWithValue("workflow_id", "catalog-id-2496"))
			Expect(selected).To(HaveKeyWithValue("workflow_name", "increase-memory"))
			Expect(selected).To(HaveKeyWithValue("workflow_selection_confidence", 0.))
			Expect(selected).To(HaveKeyWithValue("repeat_justification_required", true))
			Expect(selected).To(HaveKeyWithValue("prior_workflow_match", true))
			Expect(selected).To(HaveKeyWithValue("repeat_validation_outcome", "accepted"))
			Expect(selected).To(HaveKeyWithValue("handoff_digest", result.HandoffDigest))
			Expect(string(wire)).NotTo(ContainSubstring("private-"))
			Expect(projected).NotTo(HaveKey("selection_assessment"))
		})

		It("never promotes a rejected identity or incomplete reasoning into an executable selection", func() {
			confidence, required := .81, true
			result := &katypes.InvestigationResult{
				Severity: "critical", RCASummary: "No justified retry", ProposedWorkflowID: "rejected-2496", WorkflowName: "increase-memory",
				WorkflowSelectionConfidence: &confidence, HumanReviewNeeded: true,
				RepeatJustification:         &sharedtypes.RepeatJustification{EvidenceBasis: "Current observation", PriorOutcomeRelevance: "Unknown cause"},
				RepeatJustificationRequired: &required, RepeatValidationOutcome: "rejected",
				ExecutionBundle: "private-bundle-2496", PriorRemediationUIDs: []string{"private-prior-2496"},
			}
			var projected map[string]interface{}
			wire := session.MarshalRCASubset(result)
			Expect(json.Unmarshal(wire, &projected)).To(Succeed())
			Expect(projected).NotTo(HaveKey("selected_workflow"))
			Expect(projected).To(HaveKey("selection_assessment"))
			assessment := projected["selection_assessment"].(map[string]interface{})
			Expect(assessment).NotTo(HaveKey("workflow_id"))
			Expect(assessment).To(HaveKeyWithValue("proposed_workflow_id", "rejected-2496"))
			Expect(assessment).To(HaveKeyWithValue("repeat_validation_outcome", "rejected"))
			Expect(assessment["repeat_justification"]).To(HaveKeyWithValue("evidence_basis", "Current observation"))
			Expect(string(wire)).NotTo(ContainSubstring("private-"))
		})

		It("does not invent a selection for explicit no-workflow, or a score when confidence is unknown", func() {
			result := &katypes.InvestigationResult{Severity: "warning", RCASummary: "No suitable workflow", HumanReviewNeeded: true}
			var projected map[string]interface{}
			Expect(json.Unmarshal(session.MarshalRCASubset(result), &projected)).To(Succeed())
			Expect(projected).NotTo(HaveKey("selected_workflow"))
			Expect(projected).NotTo(HaveKey("selection_assessment"))
			result.WorkflowID, result.WorkflowName, result.Confidence = "catalog-id-2496", "increase-memory", .93
			Expect(json.Unmarshal(session.MarshalRCASubset(result), &projected)).To(Succeed())
			Expect(projected).To(HaveKey("selected_workflow"))
			Expect(projected["selected_workflow"]).NotTo(HaveKey("workflow_selection_confidence"))
		})
	})

	Describe("UT-KA-1396-004: marshalRCASubset produces valid JSON with bounded fields", func() {
		It("should marshal severity, confidence, causal_chain, target, and metrics", func() {
			result := &katypes.InvestigationResult{
				Severity:    "critical",
				Confidence:  0.92,
				CausalChain: []string{"Memory leak in data-processor", "Container hit 512Mi limit", "OOMKill signal"},
				RCASummary:  "OOMKill caused by memory leak in worker pod",
				RemediationTarget: katypes.RemediationTarget{
					Kind:      "Deployment",
					Name:      "data-processor",
					Namespace: "production",
				},
				TotalLLMTurns:  17,
				TotalToolCalls: 19,
			}

			data := session.MarshalRCASubset(result)
			Expect(data).NotTo(BeNil(), "should produce non-nil JSON")

			var parsed map[string]interface{}
			err := json.Unmarshal(data, &parsed)
			Expect(err).NotTo(HaveOccurred(), "should produce valid JSON")

			Expect(parsed).To(HaveKeyWithValue("severity", "critical"))
			Expect(parsed).To(HaveKeyWithValue("confidence", BeNumerically("~", 0.92, 0.001)))
			Expect(parsed).To(HaveKey("causal_chain"))
			chain, ok := parsed["causal_chain"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(chain).To(HaveLen(3))

			Expect(parsed).To(HaveKeyWithValue("target", "Deployment/data-processor in production"))
			Expect(parsed).To(HaveKeyWithValue("rca_summary", "OOMKill caused by memory leak in worker pod"))
			Expect(parsed).To(HaveKeyWithValue("total_llm_turns", BeNumerically("==", 17)))
			Expect(parsed).To(HaveKeyWithValue("total_tool_calls", BeNumerically("==", 19)))
		})

		It("should NOT leak internal workflow or validation state", func() {
			result := &katypes.InvestigationResult{
				Severity:    "high",
				Confidence:  0.85,
				CausalChain: []string{"cert expired"},
				RCASummary:  "Certificate expired",
				RemediationTarget: katypes.RemediationTarget{
					Kind:      "Secret",
					Name:      "tls-cert",
					Namespace: "istio-system",
				},
				TotalLLMTurns:  5,
				TotalToolCalls: 3,
				WorkflowID:     "wf-renew-cert",
				ValidationAttemptsHistory: []katypes.ValidationAttemptRecord{
					{Attempt: 1, IsValid: true},
				},
			}

			data := session.MarshalRCASubset(result)
			Expect(data).NotTo(BeNil())

			raw := string(data)
			Expect(raw).NotTo(ContainSubstring("workflow_id"))
			Expect(raw).NotTo(ContainSubstring("validation_attempts_history"))
			Expect(raw).NotTo(ContainSubstring("due_diligence"))
			Expect(raw).NotTo(ContainSubstring("execution_bundle"))
		})

		It("should handle nil result gracefully", func() {
			data := session.MarshalRCASubset(nil)
			Expect(data).To(BeNil())
		})

		It("should produce bounded output size for typical payloads", func() {
			result := &katypes.InvestigationResult{
				Severity:    "critical",
				Confidence:  0.95,
				CausalChain: []string{"A", "B", "C", "D", "E"},
				RCASummary:  "Complex multi-factor root cause requiring detailed analysis of several contributing systems",
				RemediationTarget: katypes.RemediationTarget{
					Kind:      "StatefulSet",
					Name:      "postgres-primary",
					Namespace: "database",
				},
				TotalLLMTurns:  25,
				TotalToolCalls: 30,
			}

			data := session.MarshalRCASubset(result)
			Expect(len(data)).To(BeNumerically("<", 2048), "RCA subset should be < 2KB for typical payloads")
		})
	})

	// #1918: harness-enforced actionability gate. AF's phase_guard.go needs a
	// structured, model-independent signal to decide whether
	// kubernaut_discover_workflows should stay reachable after an
	// investigate call, instead of trusting the LLM's own reading of the RCA
	// narrative. is_actionable mirrors InvestigationResult.IsActionable
	// (*bool, so "never computed" is distinguishable from "computed false"
	// via JSON key absence); has_workflow is derived from WorkflowID != "" --
	// the same two-signal condition KA's own investigator.go guard already
	// treats as authoritative internally (actionable=false && workflow_id=="").
	Describe("UT-KA-1918: is_actionable/has_workflow bounded fields for AF's harness gate", func() {
		It("UT-KA-1918-001: marshals is_actionable=false and has_workflow=false when RCA concluded no remediation is warranted", func() {
			notActionable := false
			result := &katypes.InvestigationResult{
				Severity:    "info",
				Confidence:  0.90,
				RCASummary:  "Problem self-resolved after pod restart",
				CausalChain: []string{"transient network partition"},
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Pod", Name: "api-server-abc", Namespace: "production",
				},
				IsActionable: &notActionable,
			}

			data := session.MarshalRCASubset(result)
			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())

			Expect(parsed).To(HaveKeyWithValue("is_actionable", false),
				"#1918: is_actionable must be surfaced so AF can gate Phase 2 independent of the model's own RCA reading")
			Expect(parsed).NotTo(HaveKey("has_workflow"),
				"#1918: has_workflow is omitempty and WorkflowID is empty here, so the key should be absent (false)")
		})

		It("UT-KA-1918-002: marshals is_actionable=true and has_workflow=true when RCA is actionable with a selected workflow", func() {
			actionable := true
			result := &katypes.InvestigationResult{
				Severity:    "critical",
				Confidence:  0.92,
				RCASummary:  "OOMKill caused by memory leak",
				CausalChain: []string{"memory leak"},
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Deployment", Name: "data-processor", Namespace: "production",
				},
				IsActionable: &actionable,
				WorkflowID:   "wf-restart-deployment",
			}

			data := session.MarshalRCASubset(result)
			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())

			Expect(parsed).To(HaveKeyWithValue("is_actionable", true))
			Expect(parsed).To(HaveKeyWithValue("has_workflow", true),
				"#1918: has_workflow must be derived from WorkflowID presence, not the raw ID itself (SI-10 -- no workflow_id leak)")

			raw := string(data)
			Expect(raw).NotTo(ContainSubstring("wf-restart-deployment"),
				"#1918: has_workflow is a boolean derivation -- the raw workflow_id value must never appear in the bounded subset")
		})

		It("UT-KA-1918-003: omits is_actionable from JSON when IsActionable was never computed (nil), so AF cannot mistake unknown for false", func() {
			result := &katypes.InvestigationResult{
				Severity:    "warning",
				Confidence:  0.80,
				RCASummary:  "Investigation in progress",
				CausalChain: []string{"pending"},
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Pod", Name: "worker-1", Namespace: "staging",
				},
				// IsActionable intentionally left nil.
			}

			data := session.MarshalRCASubset(result)
			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())

			Expect(parsed).NotTo(HaveKey("is_actionable"),
				"#1918: a nil (never-computed) IsActionable must be omitted, not coerced to false -- AF's gate must only override on a genuine false, never on absence")
		})
	})
})
