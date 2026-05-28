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
package ka

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
)

// parseSSEStream reads a text/event-stream body and emits InvestigationEvents
// on the provided channel. It returns when the stream ends, ctx is cancelled,
// or a terminal event (complete/cancelled/error) is encountered.
// If the underlying reader returns an error, an EventTypeError event is emitted
// so callers can distinguish clean completion from I/O failure.
func parseSSEStream(ctx context.Context, body io.Reader, ch chan<- InvestigationEvent) {
	scanner := bufio.NewScanner(body)
	var eventType string
	var dataBuf strings.Builder

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()

		if line == "" {
			if dataBuf.Len() > 0 {
				evt := InvestigationEvent{Type: eventType}
				raw := dataBuf.String()
				evt.Data = json.RawMessage(raw)

				var parsed struct {
					Turn  int    `json:"turn"`
					Phase string `json:"phase"`
				}
				_ = json.Unmarshal([]byte(raw), &parsed)
				evt.Turn = parsed.Turn
				evt.Phase = parsed.Phase

				select {
				case ch <- evt:
				case <-ctx.Done():
					return
				}

				if eventType == EventTypeComplete || eventType == EventTypeCancelled || eventType == EventTypeError {
					return
				}
			}
			eventType = ""
			dataBuf.Reset()
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			if dataBuf.Len() > 0 {
				dataBuf.WriteString("\n")
			}
			dataBuf.WriteString(strings.TrimPrefix(line, "data: "))
		}
	}

	if err := scanner.Err(); err != nil {
		errMsg, marshalErr := json.Marshal(map[string]string{"error": err.Error()})
		if marshalErr != nil {
			errMsg = []byte(`{"error":"stream scanner error"}`)
		}
		select {
		case ch <- InvestigationEvent{Type: EventTypeError, Data: json.RawMessage(errMsg)}:
		case <-ctx.Done():
		}
	}
}
