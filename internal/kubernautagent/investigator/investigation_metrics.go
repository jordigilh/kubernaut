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

// InvestigationMetrics accumulates LLM interaction counts across all
// investigation phases (RCA + workflow selection). Unlike TokenAccumulator
// which tracks token usage for audit, this tracks call-level metrics
// for surfacing in the structured decision payload.
type InvestigationMetrics struct {
	llmTurns  int
	toolCalls int
}

func NewInvestigationMetrics() *InvestigationMetrics {
	return &InvestigationMetrics{}
}

func (m *InvestigationMetrics) IncLLMTurns() {
	m.llmTurns++
}

func (m *InvestigationMetrics) IncToolCalls() {
	m.toolCalls++
}

func (m *InvestigationMetrics) LLMTurns() int {
	return m.llmTurns
}

func (m *InvestigationMetrics) ToolCalls() int {
	return m.toolCalls
}
