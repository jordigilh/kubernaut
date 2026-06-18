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

// briefInvestigationConfig returns a scenario that keeps a KA session alive
// long enough for IT tests to create an IS and observe the terminal phase
// transition, without the 30s delay of slowInvestigationConfig.
//
// Turn 1: ToolCallName → kubectl_get (immediate)
// Turn 2: NextToolCall → kubectl_get (extends session via Evaluate, 2s delay)
// Turn 3: DAG final_analysis → text response (session completes, 2s delay)
//
// The 6s SecondTurnDelay provides a reliable window for the test's Eventually
// loop (200ms interval) to detect KASession.ID and create the IS before the
// investigation completes. Total investigation time: ~12s (2 delayed turns).
//
// Used by IT tests (IT-AA-1376-001) that need the session to remain active
// briefly for IS creation and upgrade detection, then complete naturally.
//
// Matches the keyword "brief-investigation-test" with priority 1.0.
func briefInvestigationConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName:    "brief_investigation",
		ToolCallName:    "kubectl_get",
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
		ForceText:       BoolPtr(false),
		SecondTurnDelay: 6 * time.Second,
	}
}
