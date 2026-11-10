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

const interactiveRCARetrySignal = "InteractiveRCAExtractionRetry2586"

type interactiveRCARetry2586Scenario struct {
	*configScenario
}

func interactiveRCARetry2586ScenarioFactory() Scenario {
	cfg := MockScenarioConfig{
		ScenarioName:         "interactive_rca_retry_2586",
		SignalName:           interactiveRCARetrySignal,
		Severity:             "high",
		Confidence:           0.65,
		RootCause:            "Corrected interactive RCA after bounded parse retry",
		ResourceKind:         "Deployment",
		APIVersion:           "apps/v1",
		InvestigationOutcome: "actionable",
		IsActionable:         BoolPtr(true),
		Contributing:         []string{"correction_validated"},
	}

	base := newSelectorScenario("interactive_rca_retry_2586", ScenarioSelector{
		Scope:          ScenarioScope{Caller: CallerKA, Phase: PhaseRCA},
		SignalPatterns: []string{interactiveRCARetrySignal},
		Confidence:     1.1,
		CustomMatch: func(ctx *DetectionContext) (bool, float64) {
			if ctx == nil || len(ctx.AvailableTools) != 1 ||
				!strings.EqualFold(ctx.AvailableTools[0], "submit_result") {
				return false, 0
			}
			if !strings.Contains(extractSignal(ctx), strings.ToLower(interactiveRCARetrySignal)) {
				return false, 0
			}
			return true, 1.1
		},
	}, cfg)
	return &interactiveRCARetry2586Scenario{configScenario: base}
}

func (s *interactiveRCARetry2586Scenario) ConfigForContext(ctx *DetectionContext) MockScenarioConfig {
	cfg := s.config
	cfg.ToolCallArgs = correctedInteractiveRCAArguments()
	cfg.ReasoningText = "accepted correction evidence"
	if ctx == nil || !strings.Contains(strings.ToLower(ctx.LastUserContent), "your response could not be parsed") {
		cfg.ToolCallArgs = malformedInteractiveRCAArguments()
		cfg.ReasoningText = "rejected RCA turn"
	}
	return cfg
}

func malformedInteractiveRCAArguments() map[string]interface{} {
	return map[string]interface{}{
		"root_cause_analysis": "malformed RCA object",
		"confidence":          0.65,
	}
}

func correctedInteractiveRCAArguments() map[string]interface{} {
	return map[string]interface{}{
		"root_cause_analysis": map[string]interface{}{
			"summary":              "Corrected interactive RCA after bounded parse retry",
			"severity":             "high",
			"signal_name":          interactiveRCARetrySignal,
			"contributing_factors": []string{"correction_validated"},
		},
		"severity":              "high",
		"confidence":            0.65,
		"investigation_outcome": "actionable",
	}
}
