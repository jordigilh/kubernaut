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

// slowInvestigationConfig returns a scenario config that keeps KA sessions in
// "investigating" state for an extended period. It uses a kubectl_get tool call
// on the first turn and a 30-second SecondTurnDelay to hold the session open.
//
// Used by AA integration tests (IT-AA-1293/1376/1449) that need a real KA
// session to remain active while testing the AA controller's behavior during
// investigation (poll, cancel, IS-triggered reconcile).
//
// Matches the keyword "slow-investigation-test" with priority 1.0.
func slowInvestigationConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "slow_investigation",
		ToolCallName: "kubectl_get",
		ToolCallArgs: map[string]interface{}{
			"resource_type": "pod",
			"namespace":     "default",
			"name":          "investigation-target",
		},
		ForceText:       BoolPtr(false),
		SecondTurnDelay: 30 * time.Second,
	}
}
