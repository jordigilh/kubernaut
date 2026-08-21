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
package scenarios

// structuredDecisionGrounding3Config is KA's own (mock-llm) side of
// structured_decision_e2e_test.go's E2E-AF-1396-001 groundSessionBeta call
// (deploy/apifrontend/overlays/e2e/mock-llm.yaml's af_structured_decision_ground_3
// AF-scenario), which dispatches a real kubernaut_investigate MCP call for
// the dedicated structured-decision-target-3 fixture.
//
// This apifrontend E2E suite runs no AIAnalysis controller (Gateway creates
// the RemediationRequest directly from the StructuredDecisionGrounding3
// synthetic Prometheus alert; there is no AA-driven autonomous dispatch), so
// no AgentSession CRD ever exists for this RR -- every kubernaut_investigate
// call for it is genuinely fresh and, since #1818's Gap 3 fix,
// createFreshInteractiveSession now runs a REAL synchronous Investigate()
// call instead of a dead placeholder (internal/kubernautagent/mcp/tools/
// investigate_start.go). Before that fix, this seed-only grounding call
// never produced authoritative RCA content, so present_decision's grounding
// guard (enforceGroundingGuard/substituteGroundedRCA, phase_guard.go)
// harmlessly left AF's own af_structured_decision scenario's scripted
// "critical" RCA (mock-llm.yaml) untouched, only zero-backfilling
// tool_calls_count/llm_turns.
//
// Without a dedicated scenario here, this call falls through to
// defaultFallbackScenario (scenario_default_fallback.go), which hardcodes
// Severity: "warning" -- surfacing as "severity must flow from mock-LLM
// through AF to SSE: expected warning to equal critical" (E2E-AF-1396-001).
//
// #1818 follow-up correction: an earlier version of this file tried to
// leave Severity == "" here so canonicalGroundedRCA (phase_guard.go) would
// treat the grounding call as "nothing authoritative to substitute" and
// preserve mock-llm.yaml's scripted "critical" RCA untouched. That does
// NOT work: internal/kubernautagent/investigator/investigator.go's
// backfillSeverity unconditionally guarantees InvestigationResult.Severity
// is never empty for any investigation that actually completes (falls back
// to the signal's own severity, then "unknown" -- required for CRD enum
// validation), so canonicalGroundedRCA's nil-on-empty-severity branch is
// unreachable for a real completing investigation. Confirmed empirically
// on helios08 (2026-08-19): leaving Severity unset still produced
// severity=="warning" (backfilled from the StructuredDecisionGrounding3
// alert's own severity label) at the SSE payload, not "".
//
// The actual fix: since full substitution of args["rca"] is unavoidable
// for any real completing grounding investigation, make the substituted
// content agree with mock-llm.yaml's own af_structured_decision
// present_decision script instead of describing this call's real (but
// irrelevant to the test) target.
//
// ToolCallArgs (not the Severity/Confidence/ResourceKind/... config fields,
// and not ExactAnalysisText) is required here: this scenario resolves in a
// single turn via a "submit_result" tool call (confirmed via mock-llm pod
// logs, hasSubmitOnly=true from the first turn), and
// response/openai.go's buildToolArguments routes ToolSubmitResult through
// rcaOnlyJSON(cfg) -- which builds its JSON purely from cfg's typed fields
// and, critically, has no field/key for the LLM's "causal_chain" JSON key
// (distinct from "contributing_factors" --
// internal/kubernautagent/parser/parser_llm_types.go's llmRCA struct).
// ExactAnalysisText is a dead end for this same reason: it only feeds
// buildAnalysisText, which is exclusively used for free-text (non-tool-call)
// responses -- ineffective once the scenario resolves via a tool call.
// buildToolArguments checks cfg.ToolCallArgs before the toolName switch, so
// setting it here fully overrides rcaOnlyJSON's output for whichever tool
// call this scenario ends up emitting (submit_result, confirmed above).
//
// The test only asserts causal_chain's length (3), not its content, so any
// 3-item list satisfies it. remediation_target uses "Deployment" (not the
// real signal's "Pod") specifically to steer clear of
// investigator_gates.go's sameKindValidationGate, which retries when the
// RCA's target kind matches the signal's own resource kind.
func structuredDecisionGrounding3Config() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "af_structured_decision_ground_3",
		SignalName:   "StructuredDecisionGrounding3",
		ToolCallArgs: map[string]interface{}{
			"root_cause_analysis": map[string]interface{}{
				"summary":     "Memory leak in the data-processor Deployment's worker goroutine caused sustained memory growth until the container exceeded its configured limit and was OOMKilled.",
				"severity":    "critical",
				"signal_name": "StructuredDecisionGrounding3",
				"contributing_factors": []string{
					"Memory leak in data-processor worker goroutine",
					"Container hit 512Mi memory limit",
					"Kernel sent OOMKill signal to container",
				},
				"causal_chain": []string{
					"Memory leak in data-processor worker goroutine",
					"Container hit 512Mi memory limit",
					"Kernel sent OOMKill signal to container",
				},
				"remediation_target": map[string]string{
					"kind":        "Deployment",
					"name":        "data-processor",
					"namespace":   "production",
					"api_version": "apps/v1",
				},
			},
			"severity":              "critical",
			"confidence":            0.92,
			"investigation_outcome": "inconclusive",
			"actionable":            false,
		},
	}
}
