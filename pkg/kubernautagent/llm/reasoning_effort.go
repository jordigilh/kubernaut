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

package llm

import "google.golang.org/genai"

// EffortToThinkingConfig resolves the effective genai.ThinkingConfig for a
// ReasoningRequest. An explicit BudgetTokens always wins with a manual
// token budget (an exact-value power-user override); otherwise Effort
// (#1604) selects a tier via EffortToThinkingLevel; with neither set, the
// High tier applies (the pre-#1604, zero-regression default).
//
// genai.ThinkingConfig is used here as a provider-agnostic intermediate
// representation (DD-LLM-005), not a dependency on the Gemini API client:
// it is Anthropic's *and* Gemini's actual native wire type for
// reasoning/thinking configuration, so both anthropicfamily (via
// adk-anthropic-go/converters.ThinkingConfigToAnthropic) and geminifamily
// (directly, as agenticgemini.Config.ThinkingConfig) can share this single
// mapping instead of each maintaining an independent effort-tier table
// (promoted here from what was previously an anthropicfamily-only
// function, #1778/DD-LLM-010).
func EffortToThinkingConfig(r *ReasoningRequest) *genai.ThinkingConfig {
	if r.BudgetTokens > 0 {
		budget := int32(r.BudgetTokens)
		return &genai.ThinkingConfig{ThinkingBudget: &budget}
	}
	if level, ok := EffortToThinkingLevel(r.Effort); ok {
		return &genai.ThinkingConfig{ThinkingLevel: level}
	}
	return &genai.ThinkingConfig{ThinkingLevel: genai.ThinkingLevelHigh}
}

// EffortToThinkingLevel maps the canonical, provider-agnostic Effort
// vocabulary (#1604) onto genai.ThinkingLevel. "none" maps to Minimal —
// genai's Minimal already means "no thinking" via the converter (a
// coherent, real off-state, not a contradiction at this layer); the
// enabled:true + effort:none contradiction is rejected earlier, at config
// validation (shared/types.LLMConfig.Validate), for operator-facing
// clarity — this mapping stays defensively correct even if that gate is
// ever bypassed. "xhigh" clamps to High: genai.ThinkingLevel has no tier
// above High. The empty string (unset) is intentionally not handled here;
// it is only reachable via EffortToThinkingConfig's default-High fallback.
func EffortToThinkingLevel(effort string) (genai.ThinkingLevel, bool) {
	switch effort {
	case "none":
		return genai.ThinkingLevelMinimal, true
	case "minimal":
		return genai.ThinkingLevelMinimal, true
	case "low":
		return genai.ThinkingLevelLow, true
	case "medium":
		return genai.ThinkingLevelMedium, true
	case "high", "xhigh":
		return genai.ThinkingLevelHigh, true
	default:
		return "", false
	}
}
