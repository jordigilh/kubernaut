package session

import (
	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"

	v1alpha1 "github.com/jordigilh/kubernaut-apifrontend/api/apifrontend/v1alpha1"
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
// on session phase, event history, and reinvocation count. Returns true when:
//  1. Phase is Active (not Disconnected, not terminal)
//  2. Events are non-empty
//  3. Last event has no FunctionCall parts (text-only turn end)
//  4. reinvokeCount < MaxReinvocations
//
// TODO: Wire af_reinvocations_total counter metric when this function is
// integrated into the end-to-end invocation loop (target: PR7 streaming).
func NeedsReinvocation(phase v1alpha1.SessionPhase, events adksession.Events, reinvokeCount int) bool {
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
	return !hasToolCall(last)
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
