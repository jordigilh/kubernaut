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

// briefInvestigationConfig returns a scenario that keeps a KA session alive for
// exactly 3 LLM turns (two tool calls + final response) without any time delay.
//
// Turn 1: ToolCallName → kubectl_get (initial investigation tool call)
// Turn 2: NextToolCall → kubectl_get (extends session by one turn via Evaluate)
// Turn 3: DAG final_analysis → text response (session completes)
//
// This is the condition-based (Evaluate) alternative to slowInvestigationConfig's
// 30s SecondTurnDelay. Session lifetime is measured in conversation turns, not
// wall-clock time, making tests deterministic and fast (~500ms total).
//
// Used by IT tests (IT-AA-1376-001) that need the session to remain active
// briefly for IS creation and upgrade detection, then complete naturally.
//
// Matches the keyword "brief-investigation-test" with priority 1.0.
func briefInvestigationConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "brief_investigation",
		ToolCallName: "kubectl_get",
		ToolCallArgs: map[string]interface{}{
			"resource_type": "pod",
			"namespace":     "default",
			"name":          "investigation-target",
		},
		NextToolCall: &MultiToolCallEntry{
			Name: "kubectl_get",
			Arguments: map[string]interface{}{
				"resource_type": "pod",
				"namespace":     "default",
				"name":          "investigation-target-2",
			},
		},
		ForceText: BoolPtr(false),
	}
}
