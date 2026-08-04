package session

import (
	"context"

	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"

	v1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
)

const (
	// MaxReinvocations is the maximum number of times the agent will be
	// re-invoked when a text-only turn end is detected during an active
	// investigation. This prevents infinite re-invocation loops.
	MaxReinvocations = 3

	// ReinvocationMessage is the synthetic user message injected to trigger
	// the agent to continue investigation when a premature text-only turn
	// end is detected.
	ReinvocationMessage = "Continue the investigation. If you need more information, use the available tools. If the investigation is complete, summarize your findings."
)

// NeedsReinvocation determines whether the agent should be re-invoked based
// on session phase, event history, state, and reinvocation count. Returns
// true when:
//  1. Phase is Active (not Disconnected, not terminal)
//  2. Events are non-empty
//  3. Last event has no FunctionCall parts (text-only turn end)
//  4. reinvokeCount < MaxReinvocations
//  5. An interactive driver session is active in state (DD-AF-011, #1899:
//     a text-only turn with no investigation ever started is the model
//     legitimately answering a question, not a stalled investigation)
//  6. No phase-2/phase-3 checkpoint is blocked in state (DD-AF-011, #1899:
//     reinvocation must never nudge the model past a consent gate the
//     harness deliberately put up)
//
// Wired into StreamingExecutor.Execute via WithReinvocation option.
func NeedsReinvocation(phase v1alpha1.SessionPhase, events adksession.Events, state adksession.State, reinvokeCount int) bool {
	return NeedsReinvocationCtx(context.Background(), phase, events, state, reinvokeCount)
}

// NeedsReinvocationCtx is the context-aware variant of NeedsReinvocation.
// Returns false when ctx is cancelled, preventing ghost re-invocation cascades
// that fail immediately with "context canceled" (#1435).
func NeedsReinvocationCtx(ctx context.Context, phase v1alpha1.SessionPhase, events adksession.Events, state adksession.State, reinvokeCount int) bool {
	if ctx.Err() != nil {
		return false
	}
	if phase != v1alpha1.SessionPhaseActive {
		return false
	}
	if events.Len() == 0 {
		return false
	}
	if reinvokeCount >= MaxReinvocations {
		return false
	}

	last := events.At(events.Len() - 1)
	if hasToolCall(last) {
		return false
	}

	if !driverActive(state) {
		return false
	}
	if checkpointBlocked(state) {
		return false
	}

	return true
}

// driverActive reports whether an interactive driver session (a successful
// kubernaut_investigate/kubernaut_reconnect) is recorded in state. A nil or
// unreadable state fails safe to "no driver" -- never nudge.
func driverActive(state adksession.State) bool {
	if state == nil {
		return false
	}
	v, err := state.Get(StateKeyDriverActive)
	if err != nil {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

// checkpointBlocked reports whether either DD-AF-011 (#1899) phase
// checkpoint is currently blocking, i.e. the harness is waiting for genuine
// user confirmation before the next phase transition.
func checkpointBlocked(state adksession.State) bool {
	if state == nil {
		return false
	}
	if v, err := state.Get(StateKeyPhase2Blocked); err == nil {
		if b, ok := v.(bool); ok && b {
			return true
		}
	}
	if v, err := state.Get(StateKeyPhase3Blocked); err == nil {
		if b, ok := v.(bool); ok && b {
			return true
		}
	}
	return false
}

// SyntheticMessage returns a user-role content message used to prompt the
// agent to continue its investigation after a premature text-only turn end.
func SyntheticMessage() *genai.Content {
	return genai.NewContentFromText(ReinvocationMessage, genai.RoleUser)
}

func hasToolCall(event *adksession.Event) bool {
	if event == nil || event.Content == nil {
		return false
	}
	for _, part := range event.Content.Parts {
		if part != nil && part.FunctionCall != nil {
			return true
		}
	}
	return false
}
