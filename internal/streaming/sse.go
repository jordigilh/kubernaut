// Package streaming provides SSE (Server-Sent Events) formatting utilities
// for streaming ADK session events to HTTP clients.
package streaming

import (
	"encoding/json"
	"fmt"
	"strings"

	adksession "google.golang.org/adk/session"
)

// StateKeyTerminal is set in an event's StateDelta to signal that the
// investigation has reached a terminal state and the SSE stream should close.
const StateKeyTerminal = "af:terminal"

// SSEPayload is the JSON structure sent in the data: field of SSE frames.
type SSEPayload struct {
	Seq       int    `json:"seq"`
	EventType string `json:"event_type"`
	Author    string `json:"author"`
	Text      string `json:"text,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	ToolArgs  any    `json:"tool_args,omitempty"`
}

// FormatSSEFrame converts an ADK session event into an SSE-compliant byte
// frame. Partial events are skipped (returns nil, nil). The seq parameter
// is a monotonically increasing sequence number for client ordering.
func FormatSSEFrame(event *adksession.Event, seq int) ([]byte, error) {
	if event.Partial {
		return nil, nil
	}

	eventType := EventTypeFromEvent(event)

	if isTerminalEvent(event) {
		eventType = "done"
	}

	payload := eventToPayload(event, seq, eventType)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal SSE payload: %w", err)
	}

	var buf strings.Builder
	fmt.Fprintf(&buf, "id: %d\n", seq)
	fmt.Fprintf(&buf, "event: %s\n", eventType)
	fmt.Fprintf(&buf, "data: %s\n", data)
	buf.WriteString("\n")

	return []byte(buf.String()), nil
}

// EventTypeFromEvent determines the SSE event type based on the ADK event
// content. FunctionCall parts produce "tool-call", everything else maps to
// the sanitized author name.
func EventTypeFromEvent(event *adksession.Event) string {
	if event.Content != nil {
		for _, part := range event.Content.Parts {
			if part != nil && part.FunctionCall != nil {
				return "tool-call"
			}
		}
	}

	author := strings.ReplaceAll(event.Author, "\n", "")
	author = strings.ReplaceAll(author, "\r", "")
	if author == "" {
		return "message"
	}
	return author
}

// HeartbeatFrame returns an SSE comment frame (prefixed with `:`) used as a
// keep-alive signal to prevent proxy/load-balancer timeouts.
func HeartbeatFrame() []byte {
	return []byte(": heartbeat\n\n")
}

func isTerminalEvent(event *adksession.Event) bool {
	if event.Actions.StateDelta == nil {
		return false
	}
	v, ok := event.Actions.StateDelta[StateKeyTerminal]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func eventToPayload(event *adksession.Event, seq int, eventType string) SSEPayload {
	p := SSEPayload{
		Seq:       seq,
		EventType: eventType,
		Author:    event.Author,
	}

	if event.Content == nil {
		return p
	}

	for _, part := range event.Content.Parts {
		if part == nil {
			continue
		}
		if part.Text != "" {
			p.Text += part.Text
		}
		if part.FunctionCall != nil {
			p.ToolName = part.FunctionCall.Name
			p.ToolArgs = part.FunctionCall.Args
		}
	}

	return p
}
