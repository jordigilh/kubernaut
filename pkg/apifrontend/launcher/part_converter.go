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
	"kubernaut_start_investigation":   "Starting investigation for %s...",
	"kubernaut_stream_investigation":  "Streaming live investigation events...",
	"kubernaut_poll_investigation":    "Polling investigation status...",
	"kubernaut_discover_workflows":    "Discovering available remediation workflows...",
	"kubernaut_select_workflow":       "Selecting remediation workflow %s...",
	"kubernaut_watch":                 "Watching remediation progress...",
	"af_create_rr":                    "Creating remediation request...",
	"af_check_existing_rr":            "Checking for existing remediation...",
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
	"kubernaut_stream_investigation": summarizeStreamInvestigation,
	"kubernaut_start_investigation":  summarizeStartInvestigation,
	"kubernaut_discover_workflows":   summarizeDiscoverWorkflows,
	"kubernaut_select_workflow":      summarizeSelectWorkflow,
	"kubernaut_watch":                summarizeWatch,
	"af_create_rr":                   summarizeCreateRR,
}

// buildPartConverter returns a GenAIPartConverter that transforms raw ADK
// FunctionCall/FunctionResponse parts into human-readable A2A TextParts.
// This provides progressive status updates to external agents during the
// 4-phase interactive remediation journey (AC 5, AC 10).
func buildPartConverter() adka2a.GenAIPartConverter {
	return func(_ context.Context, _ *session.Event, part *genai.Part) (a2a.Part, error) {
		if part == nil {
			return nil, nil
		}

		if part.FunctionCall != nil {
			return convertFunctionCall(part.FunctionCall), nil
		}
		if part.FunctionResponse != nil {
			return convertFunctionResponse(part.FunctionResponse), nil
		}
		if part.Thought {
			return &a2a.TextPart{Text: "Analyzing...\n\n"}, nil
		}
		return &a2a.TextPart{Text: ensureTrailingParagraphBreak(part.Text)}, nil
	}
}

func convertFunctionCall(fc *genai.FunctionCall) a2a.Part {
	template, ok := toolStatusMessages[fc.Name]
	if !ok {
		return &a2a.TextPart{Text: "Processing...\n\n"}
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
		case "kubernaut_start_investigation":
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

func summarizeStreamInvestigation(resp map[string]any) string {
	if summary, ok := resp["summary"].(string); ok && summary != "" {
		return summary
	}
	return "Investigation completed."
}

func summarizeStartInvestigation(resp map[string]any) string {
	if sid, ok := resp["session_id"].(string); ok && sid != "" {
		return fmt.Sprintf("Investigation started: %s", sid)
	}
	return "Investigation started."
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
	phase, _ := resp["phase"].(string)
	status, _ := resp["status"].(string)
	if phase != "" && status != "" {
		return fmt.Sprintf("Phase: %s — %s", phase, status)
	}
	if phase != "" {
		return fmt.Sprintf("Phase: %s", phase)
	}
	if status != "" {
		return status
	}
	return "Watching remediation..."
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

// buildStreamingPartConverter returns a GenAIPartConverter for streaming mode
// that suppresses kubernaut_stream_investigation FunctionResponse parts (since
// the EventBridge already delivered the reasoning progressively). All other
// behavior is identical to the standard converter.
func buildStreamingPartConverter() adka2a.GenAIPartConverter {
	return func(_ context.Context, _ *session.Event, part *genai.Part) (a2a.Part, error) {
		if part == nil {
			return nil, nil
		}

		if part.FunctionCall != nil {
			return convertFunctionCall(part.FunctionCall), nil
		}
		if part.FunctionResponse != nil {
			if part.FunctionResponse.Name == "kubernaut_stream_investigation" {
				return nil, nil
			}
			return convertFunctionResponse(part.FunctionResponse), nil
		}
		if part.Thought {
			return &a2a.TextPart{Text: "Analyzing...\n\n"}, nil
		}
		return &a2a.TextPart{Text: ensureTrailingParagraphBreak(part.Text)}, nil
	}
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
