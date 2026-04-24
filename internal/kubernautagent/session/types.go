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

package session

// InvestigationEvent represents a discrete event emitted during an
// investigation session. Event types are runtime-agnostic to provide a
// stable SSE contract across runtime migrations (LangChainGo -> Goose ACP).
//
// Goose ACP mapping (future):
//   - EventTypeReasoningDelta -> acp.StreamEvent with kind="reasoning"
//   - EventTypeToolCall       -> acp.StreamEvent with kind="tool_use"
//   - EventTypeToolResult     -> acp.StreamEvent with kind="tool_result"
//   - EventTypeError          -> acp.StreamEvent with kind="error"
//   - EventTypeComplete       -> acp.StreamEvent with kind="end"
type InvestigationEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
}

// Event type constants for investigation lifecycle events.
// These are wire-format values sent over SSE to observers.
const (
	EventTypeReasoningDelta = "reasoning_delta"
	EventTypeToolCall       = "tool_call"
	EventTypeToolResult     = "tool_result"
	EventTypeError          = "error"
	EventTypeComplete       = "complete"
)
