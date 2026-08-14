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
	adksession "google.golang.org/adk/session"
)

// DefaultToolRetryCircuitBreakerThreshold is the number of consecutive
// same-tool failures that trips toolRetryCircuitBreaker (#2078, DD-AF-013).
// A plain Go constant, not a Helm/config value -- this is a safety-net
// default, not something an operator needs to tune per deployment
// (DD-PLATFORM-006's config-surface-reduction thesis). Slightly more
// lenient than KA's AnomalyDetector.MaxRepeatedFailures (3), since a
// schema-validation self-correction may reasonably take an extra attempt.
const DefaultToolRetryCircuitBreakerThreshold = 5

// toolRetryCircuitBreaker tracks, per tool name, a consecutive-failure count
// observed across the event stream of a single inner Runner.Run call. It
// exists because google.golang.org/adk's model-generation loop
// (internal/llminternal/base_flow.go's Flow.Run) has no retry cap of its own
// when a tool call repeatedly fails -- see DD-AF-013 for why this must be
// detected here (the wrapping event-stream consumer) rather than via an
// AfterToolCallback, which cannot stop that loop.
//
// Not safe for concurrent use -- each toolRetryCircuitBreaker is scoped to a
// single, sequential inner.Run() call within reinvokingRunner.Run.
type toolRetryCircuitBreaker struct {
	threshold int
	counts    map[string]int
}

// newToolRetryCircuitBreaker creates a breaker that trips a tool after
// threshold consecutive failures.
func newToolRetryCircuitBreaker(threshold int) *toolRetryCircuitBreaker {
	return &toolRetryCircuitBreaker{
		threshold: threshold,
		counts:    make(map[string]int),
	}
}

// observe inspects event for tool-call FunctionResponse parts and updates
// per-tool consecutive-failure counters: a response carrying a non-empty
// "error" key (ADK's own tool-failure convention, internal/llminternal/
// base_flow.go's callTool) increments that tool's count; any other response
// resets it to zero. Returns the tool name and true the moment any tool's
// count reaches the configured threshold. Events without a FunctionResponse
// part (model text, etc.) or a nil event are a no-op.
//
// Deliberately keyed by tool name only, not tool name + argument hash (unlike
// KA's AnomalyDetector.RecordFailure) -- a schema-validation retry is
// expected to vary its arguments as the LLM tries to self-correct, so an
// argument-hash key would never accumulate and the breaker would never trip.
func (b *toolRetryCircuitBreaker) observe(event *adksession.Event) (toolName string, tripped bool) {
	if event == nil || event.Content == nil {
		return "", false
	}
	for _, part := range event.Content.Parts {
		if part == nil || part.FunctionResponse == nil || part.FunctionResponse.Name == "" {
			continue
		}
		name := part.FunctionResponse.Name
		if responseHasError(part.FunctionResponse.Response) {
			b.counts[name]++
			if b.counts[name] >= b.threshold {
				return name, true
			}
		} else {
			b.counts[name] = 0
		}
	}
	return "", false
}

// responseHasError reports whether resp carries ADK's own tool-failure
// convention: a non-empty "error" key (genai.FunctionResponse.Response's
// documented "output"/"error" key convention).
func responseHasError(resp map[string]any) bool {
	if resp == nil {
		return false
	}
	v, ok := resp["error"]
	if !ok || v == nil {
		return false
	}
	if s, ok := v.(string); ok {
		return s != ""
	}
	return true
}
