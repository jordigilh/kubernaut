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
	"present_decision":                "Presenting decision to user...",
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
			return &a2a.TextPart{Text: "Analyzing..."}, nil
		}
		return &a2a.TextPart{Text: part.Text}, nil
	}
}

func convertFunctionCall(fc *genai.FunctionCall) a2a.Part {
	template, ok := toolStatusMessages[fc.Name]
	if !ok {
		return &a2a.TextPart{Text: "Processing..."}
	}

	text := formatStatusWithContext(template, fc.Name, fc.Args)
	return &a2a.TextPart{Text: truncate(text, maxStatusLen)}
}

func convertFunctionResponse(fr *genai.FunctionResponse) a2a.Part {
	summarizer, ok := keyToolSummarizers[fr.Name]
	if !ok {
		return nil
	}

	resp := fr.Response
	if resp == nil {
		resp = map[string]any{}
	}

	text := summarizer(resp)
	return &a2a.TextPart{Text: truncate(text, maxSummaryLen)}
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
	rrID, _ := resp["rr_id"].(string)
	if rrID != "" {
		return fmt.Sprintf("Remediation request created: %s", rrID)
	}
	return "Remediation request created."
}

func stringArg(args map[string]any, key string) string {
	v, ok := args[key].(string)
	if !ok {
		return ""
	}
	return v
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
