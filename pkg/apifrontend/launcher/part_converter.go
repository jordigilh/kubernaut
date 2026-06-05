package launcher

import (
	"context"
	"fmt"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
	"google.golang.org/adk/server/adka2a"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

const (
	maxStatusLen  = 512
	maxSummaryLen = 1024
)

var toolStatusMessages = map[string]string{
	"kubernaut_investigate":           "Investigating %s...",
	"kubernaut_discover_workflows":    "Discovering available remediation workflows...",
	"kubernaut_select_workflow":       "Selecting remediation workflow %s...",
	"kubernaut_watch":                 "Watching remediation progress...",
	"kubernaut_remediate":             "Creating remediation request...",
	"kubernaut_check_existing_remediation": "Checking for existing remediation...",
	"kubectl_list_events":             "Fetching cluster events...",
	"kubectl_get":                     "Fetching resource details...",
	"kubectl_list":                    "Listing cluster resources...",
	"kubernaut_list_remediations":     "Listing remediations...",
	"kubernaut_get_remediation":       "Getting remediation details...",
	"kubernaut_approve":               "Approving remediation...",
	"kubernaut_cancel_remediation":    "Cancelling remediation...",
	"kubernaut_present_decision":      "Presenting decision to user...",
	"kubernaut_list_workflows":        "Listing available workflows...",
	"kubernaut_get_remediation_history": "Getting remediation history...",
	"kubernaut_get_effectiveness":     "Getting effectiveness data...",
	"kubernaut_get_audit_trail":       "Getting audit trail...",
}

// keyToolSummarizers maps tool names to functions that extract a human-readable
// summary from FunctionResponse.Response. Tools not listed here have their
// responses dropped (the LLM's subsequent reasoning covers the content).
var keyToolSummarizers = map[string]func(map[string]any) string{
	"kubernaut_investigate":          summarizeInvestigation,
	"kubernaut_discover_workflows":   summarizeDiscoverWorkflows,
	"kubernaut_select_workflow":      summarizeSelectWorkflow,
	"kubernaut_watch":                summarizeWatch,
	"kubernaut_remediate":            summarizeCreateRR,
}

// buildPartConverter returns a GenAIPartConverter that uses a hybrid approach:
//   - FunctionCall/FunctionResponse/Thought → emitted via EventBridge as
//     TaskStatusUpdateEvent (ephemeral, shown in Kagenti's streaming bubble)
//   - Text (LLM output) → returned as TextPart artifact so the ADK executor
//     wraps it in TaskArtifactUpdateEvent with lastChunk lifecycle, which
//     Kagenti renders as the final persistent chat message
//
// If no EventBridge is in the context, falls back to returning TextParts for
// all part types.
func buildPartConverter() adka2a.GenAIPartConverter {
	return func(ctx context.Context, _ *session.Event, part *genai.Part) (a2a.Part, error) {
		if part == nil {
			return nil, nil
		}

		bridge := EventBridgeFromContext(ctx)
		if bridge == nil {
			return convertPartToText(part), nil
		}

		return emitPartViaBridge(ctx, bridge, part), nil
	}
}

func convertFunctionCall(fc *genai.FunctionCall) a2a.Part {
	template, ok := toolStatusMessages[fc.Name]
	if !ok {
		return &a2a.TextPart{Text: "...\n\n"}
	}

	text := formatStatusWithContext(template, fc.Name, fc.Args)
	return &a2a.TextPart{Text: truncate(text, maxStatusLen) + "\n\n"}
}

func convertFunctionResponse(fr *genai.FunctionResponse) a2a.Part {
	if errPart := toolErrorPart(fr); errPart != nil {
		return errPart
	}

	summarizer, ok := keyToolSummarizers[fr.Name]
	if !ok {
		return nil
	}

	resp := fr.Response
	if resp == nil {
		resp = map[string]any{}
	}

	text := summarizer(resp)
	return &a2a.TextPart{Text: truncate(text, maxSummaryLen) + "\n\n"}
}

// toolErrorPart returns an error text part when a tool response indicates
// failure. Matches both simple {"error":"..."} and the KA MCP pattern
// {"status":"error","error":"..."}. This ensures tool failures surface on the
// SSE stream instead of being silently dropped (#1302).
func toolErrorPart(fr *genai.FunctionResponse) a2a.Part {
	if fr.Response == nil {
		return nil
	}
	errMsg, _ := fr.Response["error"].(string)
	if errMsg == "" {
		return nil
	}
	status, _ := fr.Response["status"].(string)
	if status != "error" && status != "" {
		return nil
	}
	text := fmt.Sprintf("Error: %s\n\n", truncate(errMsg, maxSummaryLen))
	return &a2a.TextPart{Text: text}
}

func formatStatusWithContext(template, toolName string, args map[string]any) string {
	if !strings.Contains(template, "%s") {
		return template
	}

	if args != nil {
		switch toolName {
		case "kubernaut_investigate":
			ns := stringArg(args, "namespace")
			name := stringArg(args, "name")
			if ns != "" && name != "" {
				return fmt.Sprintf(template, truncate(ns, 64)+"/"+truncate(name, 64))
			}
		case "kubernaut_select_workflow":
			wfID := stringArg(args, "workflow_id")
			if wfID != "" {
				return fmt.Sprintf(template, truncate(wfID, 128))
			}
		}
	}

	return strings.ReplaceAll(template, " %s", "")
}

func summarizeInvestigation(resp map[string]any) string {
	if summary, ok := resp["summary"].(string); ok && summary != "" {
		return summary
	}
	if sid, ok := resp["session_id"].(string); ok && sid != "" {
		return fmt.Sprintf("Investigation started: %s", sid)
	}
	return "Investigation completed."
}

func summarizeDiscoverWorkflows(resp map[string]any) string {
	workflows, ok := resp["workflows"].([]any)
	if !ok || len(workflows) == 0 {
		return "No workflows discovered."
	}

	var sb strings.Builder
	sb.WriteString("Available workflows:\n")
	for _, w := range workflows {
		wm, ok := w.(map[string]any)
		if !ok {
			continue
		}
		name, _ := wm["name"].(string)
		confidence, _ := wm["confidence"].(float64)
		if name != "" {
			if confidence > 0 {
				fmt.Fprintf(&sb, "- %s (confidence: %.0f%%)\n", name, confidence*100)
			} else {
				fmt.Fprintf(&sb, "- %s\n", name)
			}
		}
	}
	return strings.TrimSpace(sb.String())
}

func summarizeSelectWorkflow(resp map[string]any) string {
	status, _ := resp["status"].(string)
	message, _ := resp["message"].(string)
	if message != "" {
		if status != "" {
			return fmt.Sprintf("%s: %s", status, message)
		}
		return message
	}
	if status != "" {
		return fmt.Sprintf("Workflow %s.", status)
	}
	return "Workflow selection completed."
}

func summarizeWatch(resp map[string]any) string {
	events, _ := resp["events"].([]any)
	status, _ := resp["status"].(string)
	outcome, _ := resp["outcome"].(string)
	message, _ := resp["message"].(string)

	var sb strings.Builder
	if len(events) > 0 {
		if last, ok := events[len(events)-1].(map[string]any); ok {
			phase, _ := last["phase"].(string)
			if phase != "" {
				fmt.Fprintf(&sb, "Remediation %s (final phase: %s)", status, phase)
			}
		}
	}
	if sb.Len() == 0 && status != "" {
		fmt.Fprintf(&sb, "Remediation %s.", status)
	}
	if sb.Len() == 0 {
		return "Watching remediation..."
	}
	if outcome != "" {
		fmt.Fprintf(&sb, "\nOutcome: %s", outcome)
	}
	if message != "" {
		fmt.Fprintf(&sb, "\nDetails: %s", message)
	}
	return sb.String()
}

func summarizeCreateRR(resp map[string]any) string {
	if exists, _ := resp["already_exists"].(bool); exists {
		if rrid, ok := resp["rr_id"].(string); ok && rrid != "" {
			return fmt.Sprintf("Remediation request already exists: %s", rrid)
		}
		return "Remediation request already exists."
	}
	msg, _ := resp["message"].(string)
	if msg != "" {
		return fmt.Sprintf("Remediation request created: %s", msg)
	}
	return "Remediation request created."
}

// buildStreamingPartConverter returns a GenAIPartConverter for streaming mode.
// Same hybrid approach as buildPartConverter: status-like parts go through the
// EventBridge as TaskStatusUpdateEvent, LLM text goes as artifact TextParts.
// kubernaut_investigate FunctionResponse parts are suppressed since the
// EventBridge already delivered the reasoning progressively.
func buildStreamingPartConverter() adka2a.GenAIPartConverter {
	return func(ctx context.Context, _ *session.Event, part *genai.Part) (a2a.Part, error) {
		if part == nil {
			return nil, nil
		}

		if part.FunctionResponse != nil && part.FunctionResponse.Name == "kubernaut_investigate" {
			return nil, nil
		}

		bridge := EventBridgeFromContext(ctx)
		if bridge == nil {
			return convertPartToText(part), nil
		}

		return emitPartViaBridge(ctx, bridge, part), nil
	}
}

// emitPartViaBridge routes status-like parts (FunctionCall, FunctionResponse,
// Thought) through the EventBridge as TaskStatusUpdateEvent and returns nil.
// LLM text output is returned as a TextPart so the ADK executor wraps it in
// a TaskArtifactUpdateEvent with the lastChunk lifecycle that Kagenti renders
// as the persistent chat response.
func emitPartViaBridge(ctx context.Context, bridge *EventBridge, part *genai.Part) a2a.Part {
	switch {
	case part.FunctionCall != nil:
		text := convertFunctionCall(part.FunctionCall)
		if text != nil {
			_ = bridge.EmitStatus(ctx, text.(*a2a.TextPart).Text)
		}
		return nil
	case part.FunctionResponse != nil:
		text := convertFunctionResponse(part.FunctionResponse)
		if text != nil {
			_ = bridge.EmitStatus(ctx, text.(*a2a.TextPart).Text)
		}
		return nil
	case part.Thought:
		_ = bridge.EmitStatus(ctx, "Analyzing...\n\n")
		return nil
	default:
		return &a2a.TextPart{Text: ensureTrailingParagraphBreak(part.Text)}
	}
}

// convertPartToText is the fallback path when no EventBridge is available.
// Returns the part as a TextPart artifact for backward compatibility.
func convertPartToText(part *genai.Part) a2a.Part {
	if part.FunctionCall != nil {
		return convertFunctionCall(part.FunctionCall)
	}
	if part.FunctionResponse != nil {
		return convertFunctionResponse(part.FunctionResponse)
	}
	if part.Thought {
		return &a2a.TextPart{Text: "Analyzing...\n\n"}
	}
	return &a2a.TextPart{Text: ensureTrailingParagraphBreak(part.Text)}
}

func stringArg(args map[string]any, key string) string {
	v, ok := args[key].(string)
	if !ok {
		return ""
	}
	return v
}

// ensureTrailingParagraphBreak appends a double-newline paragraph break to
// text that doesn't already end with one. A single \n is insufficient because
// A2A clients (e.g. kagenti) strip trailing whitespace before concatenating
// artifact text parts, collapsing single newlines. Double-newline survives
// stripping and renders as a visible paragraph break in markdown UIs.
func ensureTrailingParagraphBreak(s string) string {
	if s == "" || strings.HasSuffix(s, "\n\n") {
		return s
	}
	s = strings.TrimRight(s, "\n")
	return s + "\n\n"
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Truncate by rune count to avoid splitting multi-byte UTF-8 characters.
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}
