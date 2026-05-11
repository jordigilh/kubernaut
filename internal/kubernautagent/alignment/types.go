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

import (
	"errors"
	"time"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// ErrCircuitBreaker is the sentinel error passed to context.WithCancelCause
// when the shadow agent detects suspicious content in enforce mode.
// context.Cause returns this exact value, so == comparison is correct.
var ErrCircuitBreaker = errors.New("shadow agent circuit breaker: suspicious content detected")

// StepKind distinguishes the source of content being evaluated.
type StepKind string

const (
	StepKindLLMReasoning StepKind = "llm_reasoning"
	StepKindToolResult   StepKind = "tool_result"
	StepKindSignalInput  StepKind = "signal_input"
)

// Step represents a single piece of content to be evaluated by the shadow agent.
type Step struct {
	Index         int      `json:"index"`
	Kind          StepKind `json:"kind"`
	Tool          string   `json:"tool,omitempty"`
	Content       string   `json:"content"`
	CorrelationID string   `json:"correlation_id,omitempty"`
}

// Observation is the evaluator's assessment of a single step.
type Observation struct {
	Step        Step           `json:"step"`
	Suspicious  bool           `json:"suspicious"`
	Explanation string         `json:"explanation"`
	Usage       llm.TokenUsage `json:"usage"`
}

// VerdictResult summarizes the shadow agent's overall assessment.
type VerdictResult string

const (
	// VerdictClean indicates all evaluated steps passed alignment checks.
	// Value "aligned" matches OpenAPI AlignmentVerdictPayload.result enum.
	VerdictClean VerdictResult = "aligned"
	// VerdictSuspicious indicates at least one step was flagged or pending.
	// Value "suspicious" matches OpenAPI AlignmentVerdictPayload.result enum.
	VerdictSuspicious VerdictResult = "suspicious"
)

// Verdict is the final output of the shadow agent for an investigation.
type Verdict struct {
	Result         VerdictResult `json:"result"`
	Summary        string        `json:"summary"`
	Observations   []Observation `json:"observations"`
	Flagged        int           `json:"flagged"`
	Total          int           `json:"total"`
	Pending        int           `json:"pending,omitempty"`
	TimedOut       bool          `json:"timed_out,omitempty"`
	CircuitBreaker bool          `json:"circuit_breaker,omitempty"`
}

// GroundingObservation is the evaluator's assessment of an entire RCA conversation.
// It answers: "given the tool evidence, are the RCA conclusions well-grounded?"
type GroundingObservation struct {
	Grounded    bool           `json:"grounded"`
	Explanation string         `json:"explanation"`
	Usage       llm.TokenUsage `json:"usage"`
	Duration    time.Duration  `json:"duration"`
}

// WaitResult captures the observer state at the moment WaitForCompletion returns.
// It provides a single snapshot for verdict rendering — no re-reading internal state.
type WaitResult struct {
	Complete             bool
	Submitted            int
	Observations         []Observation
	Pending              int
	GroundingObservation *GroundingObservation
}
