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
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-logr/logr"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
)

// StreamInvestigationArgs defines the input for kubernaut_stream_investigation.
type StreamInvestigationArgs struct {
	SessionID string `json:"session_id"`
}

// StreamInvestigationResult is the output of kubernaut_stream_investigation.
type StreamInvestigationResult struct {
	Status   string              `json:"status"`
	Summary  string              `json:"summary,omitempty"`
	Events   []StreamEventDigest `json:"events,omitempty"`
	EventLog string              `json:"event_log,omitempty"`
}

// StreamEventDigest is a compact representation of an SSE event for the LLM.
type StreamEventDigest struct {
	Type  string `json:"type"`
	Phase string `json:"phase,omitempty"`
	Text  string `json:"text,omitempty"`
}

// HandleStreamInvestigation connects to KA's SSE stream and collects events
// until the investigation reaches a terminal state. Returns a structured
// summary including the narrative of reasoning and tool execution events.
func HandleStreamInvestigation(ctx context.Context, kaClient *ka.Client, args StreamInvestigationArgs) (StreamInvestigationResult, error) {
	if args.SessionID == "" {
		return StreamInvestigationResult{}, fmt.Errorf("session_id is required")
	}

	ch, err := kaClient.StreamEvents(ctx, args.SessionID)
	if err != nil {
		return StreamInvestigationResult{}, fmt.Errorf("connecting to investigation stream: %w", err)
	}

	var events []StreamEventDigest
	var narrative strings.Builder
	var finalSummary string

	for event := range ch {
		select {
		case <-ctx.Done():
			return StreamInvestigationResult{
				Status:   "cancelled",
				Events:   events,
				EventLog: narrative.String(),
			}, nil
		default:
		}

		digest := StreamEventDigest{
			Type:  event.Type,
			Phase: event.Phase,
		}

		switch event.Type {
		case ka.EventTypeReasoningDelta, ka.EventTypeTokenDelta:
			text := extractTextFromData(event.Data)
			digest.Text = text
			narrative.WriteString(text)
			emitViaBridge(ctx, text)
		case ka.EventTypeToolCallStart, ka.EventTypeToolCall:
			text := extractTextFromData(event.Data)
			digest.Text = text
			if text != "" {
				narrative.WriteString(fmt.Sprintf("\n[Tool: %s]\n", text))
				emitViaBridge(ctx, fmt.Sprintf("[Tool: %s]", text))
			}
		case ka.EventTypeToolResult:
			text := extractTextFromData(event.Data)
			digest.Text = truncateForLLM(text, 500)
		case ka.EventTypeComplete:
			finalSummary = extractSummaryFromComplete(event.Data)
			events = append(events, digest)
			emitViaBridge(ctx, finalSummary)
			return StreamInvestigationResult{
				Status:   "completed",
				Summary:  finalSummary,
				Events:   events,
				EventLog: narrative.String(),
			}, nil
		case ka.EventTypeCancelled:
			events = append(events, digest)
			return StreamInvestigationResult{
				Status:   "cancelled",
				Events:   events,
				EventLog: narrative.String(),
			}, nil
		case ka.EventTypeError:
			text := extractTextFromData(event.Data)
			digest.Text = text
			events = append(events, digest)
			return StreamInvestigationResult{
				Status:   "failed",
				Events:   events,
				EventLog: narrative.String(),
			}, nil
		}

		events = append(events, digest)
	}

	status := "completed"
	if finalSummary == "" {
		status = "disconnected"
	}
	return StreamInvestigationResult{
		Status:   status,
		Summary:  finalSummary,
		Events:   events,
		EventLog: narrative.String(),
	}, nil
}

func extractTextFromData(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		return str
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err == nil {
		if text, ok := obj["text"].(string); ok {
			return text
		}
		if content, ok := obj["content"].(string); ok {
			return content
		}
		if preview, ok := obj["content_preview"].(string); ok {
			return preview
		}
		if delta, ok := obj["delta"].(string); ok {
			return delta
		}
		if summary, ok := obj["summary"].(string); ok {
			return summary
		}
	}
	return string(data)
}

func extractSummaryFromComplete(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err == nil {
		if summary, ok := obj["summary"].(string); ok {
			return summary
		}
		if rootCause, ok := obj["root_cause"].(string); ok {
			return rootCause
		}
	}
	text := extractTextFromData(data)
	return truncateForLLM(text, 2000)
}

func truncateForLLM(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "... (truncated)"
}

// NewStreamInvestigationTool creates the kubernaut_stream_investigation tool.
func NewStreamInvestigationTool(kaClient *ka.Client) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_stream_investigation",
		Description: "Stream live investigation events from the AI agent in real-time. Returns when investigation completes with a full summary. Use instead of kubernaut_poll_investigation for live narrative.",
	}, func(ctx tool.Context, args StreamInvestigationArgs) (StreamInvestigationResult, error) {
		return HandleStreamInvestigation(ctx, kaClient, args)
	})
}

// emitViaBridge sends text to the A2A event bridge if present in context.
// Bridge write errors are logged but do not interrupt the tool execution.
func emitViaBridge(ctx context.Context, text string) {
	if text == "" {
		return
	}
	if err := launcher.EmitReasoningSafe(ctx, text); err != nil {
		logr.FromContextOrDiscard(ctx).Info("WARNING: bridge emit failed", "error", security.RedactError(err))
	}
}
