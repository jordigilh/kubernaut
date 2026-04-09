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

package alignment

// StepKind distinguishes the source of content being evaluated.
type StepKind string

const (
	StepKindLLMReasoning StepKind = "llm_reasoning"
	StepKindToolResult   StepKind = "tool_result"
)

// Step represents a single piece of content to be evaluated by the shadow agent.
type Step struct {
	Index   int      `json:"index"`
	Kind    StepKind `json:"kind"`
	Tool    string   `json:"tool,omitempty"`
	Content string   `json:"content"`
}

// Observation is the evaluator's assessment of a single step.
type Observation struct {
	Step        Step   `json:"step"`
	Suspicious  bool   `json:"suspicious"`
	Explanation string `json:"explanation"`
}

// VerdictResult summarizes the shadow agent's overall assessment.
type VerdictResult string

const (
	VerdictClean      VerdictResult = "clean"
	VerdictSuspicious VerdictResult = "suspicious"
)

// Verdict is the final output of the shadow agent for an investigation.
type Verdict struct {
	Result       VerdictResult `json:"result"`
	Summary      string        `json:"summary"`
	Observations []Observation `json:"observations"`
	Flagged      int           `json:"flagged"`
	Total        int           `json:"total"`
	Pending      int           `json:"pending,omitempty"`
	TimedOut     bool          `json:"timed_out,omitempty"`
}

// WaitResult captures the observer state at the moment WaitForCompletion returns.
// It provides a single snapshot for verdict rendering — no re-reading internal state.
type WaitResult struct {
	Complete     bool
	Submitted    int
	Observations []Observation
	Pending      int
}
