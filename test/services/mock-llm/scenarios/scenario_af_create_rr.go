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

import "strings"

// afCreateRRConfig returns a config that instructs the mock LLM to call
// af_create_rr when the AF ADK agent sends a Gemini request whose user
// message contains "create a remediation request".
//
// Issue #1189: E2E-FP-1189-002 and E2E-FP-1189-003 need the mock LLM to
// tell the AF agent to call af_create_rr so that an RR is actually created
// and the full downstream pipeline (RO → SP → AA → KA → WE) triggers.
func afCreateRRConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "af_create_rr",
		ToolCallName: "af_create_rr",
		ResourceKind: "Deployment",
		ResourceNS:   "kubernaut-system",
		ResourceName: "memory-eater",
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
	}
}

func afCreateRRScenario() *configScenario {
	cfg := afCreateRRConfig()
	return &configScenario{
		config: cfg,
		matchFunc: func(ctx *DetectionContext) (bool, float64) {
			combined := strings.ToLower(ctx.Content + " " + ctx.AllText)
			if strings.Contains(combined, "create a remediation request") {
				return true, 0.9
			}
			return false, 0
		},
	}
}
