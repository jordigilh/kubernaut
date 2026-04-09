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

package investigator

import "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"

// TokenAccumulator tracks cumulative token usage across multiple LLM turns.
// Tokens are audit-only (not surfaced in the API response per #435).
type TokenAccumulator struct {
	promptTokens     int
	completionTokens int
	totalTokens      int
}

// Add records token usage from a single LLM response.
func (ta *TokenAccumulator) Add(usage llm.TokenUsage) {
	ta.promptTokens += usage.PromptTokens
	ta.completionTokens += usage.CompletionTokens
	ta.totalTokens += usage.TotalTokens
}

// PromptTokens returns cumulative prompt tokens.
func (ta *TokenAccumulator) PromptTokens() int { return ta.promptTokens }

// CompletionTokens returns cumulative completion tokens.
func (ta *TokenAccumulator) CompletionTokens() int { return ta.completionTokens }

// TotalTokens returns cumulative total tokens.
func (ta *TokenAccumulator) TotalTokens() int { return ta.totalTokens }

// AuditData returns a map suitable for embedding in audit event Data.
func (ta *TokenAccumulator) AuditData() map[string]interface{} {
	return map[string]interface{}{
		"total_prompt_tokens":     ta.promptTokens,
		"total_completion_tokens": ta.completionTokens,
		"total_tokens":            ta.totalTokens,
	}
}
