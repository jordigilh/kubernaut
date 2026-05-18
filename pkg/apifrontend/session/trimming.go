package session

import (
	"encoding/json"

	adksession "google.golang.org/adk/session"
)

const (
	// MaxToolResultBytes is the maximum serialized size of a FunctionResponse
	// before it is truncated. Prevents etcd bloat from large tool outputs
	// like pod lists or metric dumps.
	MaxToolResultBytes = 4096

	// TrimSummaryPrefix is the number of bytes of the original JSON to include
	// in the truncation summary for debugging context. Kept small (128B) to
	// reduce the risk of leaking secrets embedded in tool responses.
	TrimSummaryPrefix = 128
)

// trimEventFunctionResponses walks the event's Content.Parts and truncates any
// FunctionResponse whose serialized Response exceeds MaxToolResultBytes.
// User messages, model text, and FunctionCall parts are never modified.
func trimEventFunctionResponses(event *adksession.Event) {
	if event == nil || event.Content == nil {
		return
	}
	for _, part := range event.Content.Parts {
		if part == nil || part.FunctionResponse == nil {
			continue
		}
		fr := part.FunctionResponse
		if fr.Response == nil {
			continue
		}

		raw, err := json.Marshal(fr.Response)
		if err != nil || len(raw) <= MaxToolResultBytes {
			continue
		}

		summary := string(raw)
		if len(summary) > TrimSummaryPrefix {
			summary = summary[:TrimSummaryPrefix]
		}

		fr.Response = map[string]any{
			"truncated":     true,
			"original_size": len(raw),
			"summary":       summary,
		}
	}
}
