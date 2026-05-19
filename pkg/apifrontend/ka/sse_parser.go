package ka

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
)

// parseSSEStream reads an SSE-formatted stream from r and sends parsed
// InvestigationEvents to ch. The function returns when:
//   - the stream is exhausted (io.EOF)
//   - ctx is cancelled
//   - a terminal event (complete, cancelled) is received
//
// KA's SSE wire format:
//
//	id: <seq>
//	event: <type>
//	data: <json>
//	<blank line>
func parseSSEStream(ctx context.Context, r io.Reader, ch chan<- InvestigationEvent) {
	scanner := bufio.NewScanner(r)

	var eventType string
	var dataLine string

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()

		if line == "" {
			if eventType != "" && dataLine != "" {
				var event InvestigationEvent
				if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
					event = InvestigationEvent{
						Type: eventType,
						Data: json.RawMessage(dataLine),
					}
				}
				if event.Type == "" {
					event.Type = eventType
				}

				select {
				case ch <- event:
				case <-ctx.Done():
					return
				}

				if event.Type == EventTypeComplete || event.Type == EventTypeCancelled {
					return
				}
			}
			eventType = ""
			dataLine = ""
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
		}
		// "id: " lines are ignored — sequence numbers are not needed by AF.
	}
}
