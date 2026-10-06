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

import "time"

// briefInvestigationScenario keeps the real RCA tool-call chain used by
// E2E-AA-065, but avoids making workflow discovery execute a three-step
// catalog lookup when the fixture intentionally has no workflow to select.
// ConfigForContext is request-scoped so concurrent investigations never share
// mutable response state.
type briefInvestigationScenario struct {
	*configScenario
}

func newBriefInvestigationScenario() *briefInvestigationScenario {
	return &briefInvestigationScenario{
		configScenario: newKeywordScenario(
			"brief_investigation",
			"brief-investigation-test",
			briefInvestigationConfig(),
		),
	}
}

// ConfigForContext returns real tool calls during RCA and a structured
// no-workflow response during workflow discovery. The phase distinction is
// necessary because AA-065 must prove KA's registered kubectl_get_by_name
// dispatch without also asking the mock catalog to resolve an empty workflow ID.
func (s *briefInvestigationScenario) ConfigForContext(ctx *DetectionContext) MockScenarioConfig {
	cfg := s.config
	cfg.ForceText = BoolPtr(false)
	if ctx != nil && (ctx.Phase == PhaseWorkflowDiscovery || ctx.Phase == PhaseWorkflowSelection) {
		cfg.ForceText = BoolPtr(true)
	}
	return cfg
}

// briefInvestigationConfig returns a scenario that keeps a KA session alive
// long enough for IT tests to create an IS and observe the terminal phase
// transition, without the 30s delay of slowInvestigationConfig.
//
// Turn 1: ToolCallName → kubectl_get_by_name (immediate)
// Turn 2: NextToolCall → kubectl_get_by_name (extends session via Evaluate, 2s delay)
// Turn 3: DAG final_analysis → text response (session completes, 2s delay)
//
// The 3s SecondTurnDelay provides a reliable window for the test's Eventually
// loop (200ms interval) to detect KASession.ID and create the IS before the
// investigation completes. The delay applies per-phase (RCA, workflow_discovery,
// etc.), so total investigation time is ~9-12s across all phases.
//
// Used by IT tests (IT-AA-1376-001) that need the session to remain active
// briefly for IS creation and upgrade detection, then complete naturally.
//
// Matches the keyword "brief-investigation-test" with priority 1.0.
func briefInvestigationConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "brief_investigation",
		SignalName:   "brief-investigation-test",
		Severity:     "warning",
		RootCause:    "Deterministic capacity-retry investigation fixture",
		ResourceKind: "Pod",
		ResourceNS:   "staging",
		ResourceName: "capacity-retry-target",
		APIVersion:   "v1",
		Contributing: []string{"capacity-retry-fixture", "controlled-investigation-duration"},
		// No workflow is intentional: AA-065 validates KA dispatch-capacity
		// retry/convergence, not workflow catalog selection.
		InvestigationOutcome: "inconclusive",
		ToolCallName:         "kubectl_get_by_name",
		ToolCallArgs: map[string]interface{}{
			"kind":      "Pod",
			"namespace": "staging",
			"name":      "capacity-retry-target",
		},
		NextToolCall: &MultiToolCallEntry{
			Name: "kubectl_get_by_name",
			Arguments: map[string]interface{}{
				"kind":      "Pod",
				"namespace": "staging",
				"name":      "capacity-retry-target",
			},
		},
		ForceText:       BoolPtr(false),
		SecondTurnDelay: 3 * time.Second,
	}
}
