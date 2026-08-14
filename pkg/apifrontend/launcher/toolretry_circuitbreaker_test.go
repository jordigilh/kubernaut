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

package launcher

import (
	"testing"

	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"
)

// presentDecisionToolName is the tool name used across most circuit breaker
// scenarios below, since present-decision schema-validation retries are the
// confirmed real-world trigger for #2078.
const presentDecisionToolName = "kubernaut_present_decision"

func functionResponseEvent(toolName string, errMsg string) *adksession.Event {
	resp := map[string]any{}
	if errMsg != "" {
		resp["error"] = errMsg
	} else {
		resp["output"] = "ok"
	}
	event := adksession.NewEvent("inv-test")
	event.Content = &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{FunctionResponse: &genai.FunctionResponse{Name: toolName, Response: resp}},
		},
	}
	return event
}

// UT-AF-2078-001: N consecutive failures for the same tool trips the breaker.
func TestToolRetryCircuitBreaker_TripsAfterThresholdConsecutiveFailures(t *testing.T) {
	b := newToolRetryCircuitBreaker(3)
	for i := 0; i < 2; i++ {
		if _, tripped := b.observe(functionResponseEvent(presentDecisionToolName, "schema validation failed")); tripped {
			t.Fatalf("must not trip before reaching the threshold (failure %d)", i+1)
		}
	}
	toolName, tripped := b.observe(functionResponseEvent(presentDecisionToolName, "schema validation failed"))
	if !tripped {
		t.Fatal("expected breaker to trip on the 3rd consecutive failure")
	}
	if toolName != presentDecisionToolName {
		t.Errorf("expected tripped tool name %q, got %q", presentDecisionToolName, toolName)
	}
}

// UT-AF-2078-002: a success resets that tool's consecutive-failure count.
func TestToolRetryCircuitBreaker_SuccessResetsCount(t *testing.T) {
	b := newToolRetryCircuitBreaker(3)
	b.observe(functionResponseEvent(presentDecisionToolName, "schema validation failed"))
	b.observe(functionResponseEvent(presentDecisionToolName, "schema validation failed"))
	b.observe(functionResponseEvent(presentDecisionToolName, "")) // success resets

	for i := 0; i < 2; i++ {
		if _, tripped := b.observe(functionResponseEvent(presentDecisionToolName, "schema validation failed")); tripped {
			t.Fatalf("must not trip -- count should have been reset by the intervening success (failure %d after reset)", i+1)
		}
	}
}

// UT-AF-2078-003: a different tool's failures are tracked independently and
// do not contribute to another tool's count.
func TestToolRetryCircuitBreaker_PerToolIsolation(t *testing.T) {
	b := newToolRetryCircuitBreaker(3)
	b.observe(functionResponseEvent(presentDecisionToolName, "schema validation failed"))
	b.observe(functionResponseEvent(presentDecisionToolName, "schema validation failed"))

	// A different tool failing must not push kubernaut_present_decision over
	// the threshold, and must not itself trip after just one failure.
	if _, tripped := b.observe(functionResponseEvent("kubernaut_list_workflows", "unrelated error")); tripped {
		t.Fatal("a different tool's first failure must not trip the breaker")
	}

	toolName, tripped := b.observe(functionResponseEvent(presentDecisionToolName, "schema validation failed"))
	if !tripped {
		t.Fatal("expected kubernaut_present_decision to trip on its own 3rd consecutive failure, undisturbed by the interleaved different-tool failure")
	}
	if toolName != presentDecisionToolName {
		t.Errorf("expected tripped tool name %q, got %q", presentDecisionToolName, toolName)
	}
}

// UT-AF-2078-004: events without a function response (e.g. plain model text)
// are a no-op and never trip the breaker.
func TestToolRetryCircuitBreaker_NonToolEventIsNoop(t *testing.T) {
	b := newToolRetryCircuitBreaker(1)
	event := adksession.NewEvent("inv-test")
	event.Content = genai.NewContentFromText("just some text", genai.RoleModel)

	if toolName, tripped := b.observe(event); tripped || toolName != "" {
		t.Fatalf("expected a non-function-response event to be a no-op, got tripped=%v toolName=%q", tripped, toolName)
	}

	if _, tripped := b.observe(nil); tripped {
		t.Fatal("expected a nil event to be a no-op")
	}
}
