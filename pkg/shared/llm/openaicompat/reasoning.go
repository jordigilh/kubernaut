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

package openaicompat

import "strings"

// ReasoningMode controls how a captured reasoning/thinking block is
// round-tripped back to the provider on a later turn. Different
// OpenAI-protocol-compatible providers have incompatible replay rules
// (BR-AI-086 req 3, citing #1578).
type ReasoningMode string

const (
	// ReasoningModeNone: the provider is not known to return reasoning
	// content, or reasoning support is explicitly disabled. Reasoning is
	// never read into replayed messages. This is the zero value and the
	// safe default for any unrecognized model (the compatibility floor).
	ReasoningModeNone ReasoningMode = "none"

	// ReasoningModePlain: the visible reasoning text is echoed back
	// verbatim in the message history on every subsequent turn. Used for
	// self-hosted reasoning-echo models (some vLLM/Ollama deployments) that
	// expect the full prior chain-of-thought as context.
	ReasoningModePlain ReasoningMode = "plain"

	// ReasoningModeDeepSeekConditional implements DeepSeek's asymmetric
	// replay rule: reasoning_content from a prior assistant turn must be
	// replayed ONLY when that turn also produced tool_calls; it must be
	// omitted for every other turn. DeepSeek's API returns HTTP 400 if
	// reasoning_content is present on a non-tool-call turn, and (per
	// DeepSeek V3.2+) rejects tool-call turns that omit it.
	// Reference: https://api-docs.deepseek.com/guides/thinking_mode
	ReasoningModeDeepSeekConditional ReasoningMode = "deepseek-conditional"
)

// DetectReasoningMode infers the round-trip mode for a model, implementing
// the compatibility-floor default: any model not explicitly recognized gets
// ReasoningModeNone (BR-AI-086 AC2/req 3) — never a speculative mode that
// could send an unsupported field to a bare-bones OpenAI-compatible server.
//
// override, when non-empty, short-circuits name-based detection entirely
// (shared/types.LLMReasoningConfig.CapabilityOverride) — the escape hatch
// for self-hosted/custom models that cannot be reliably identified by name
// pattern alone. "force_on" resolves to ReasoningModePlain: an operator
// enabling reasoning by hand for an unrecognized model is, by construction,
// asserting it echoes reasoning_content and expects it replayed verbatim —
// the same contract as every known "plain" model.
func DetectReasoningMode(model, override string) ReasoningMode {
	switch override {
	case "force_off":
		return ReasoningModeNone
	case "force_on":
		return ReasoningModePlain
	}

	lower := strings.ToLower(model)
	if strings.Contains(lower, "deepseek-reasoner") || strings.Contains(lower, "deepseek-r1") {
		return ReasoningModeDeepSeekConditional
	}
	return ReasoningModeNone
}

// shouldReplayReasoning decides, for one already-recorded message, whether
// its captured Reasoning should be sent back to the provider on this call.
func shouldReplayReasoning(mode ReasoningMode, hadToolCalls bool) bool {
	switch mode {
	case ReasoningModePlain:
		return true
	case ReasoningModeDeepSeekConditional:
		return hadToolCalls
	default:
		return false
	}
}
