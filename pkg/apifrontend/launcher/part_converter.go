package launcher

import (
	"context"
	"encoding/json"
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
	"kubernaut_investigate":        summarizeInvestigation,
	"kubernaut_discover_workflows": summarizeDiscoverWorkflows,
	"kubernaut_select_workflow":    summarizeSelectWorkflow,
	"kubernaut_watch":              summarizeWatch,
	"kubernaut_remediate":          summarizeCreateRR,
	"kubernaut_present_decision":   summarizePresentDecision,
}

// decisionMetaTools lists tools whose FunctionResponse should be emitted
// with MetaTypeDecision metadata instead of the default MetaTypeStatus.
// This allows Console/Kagenti to render rich structured workflow cards.
var decisionMetaTools = map[string]bool{
	"kubernaut_present_decision": true,
}

// outputMetaTools lists tools whose FunctionResponse should be emitted
// with MetaTypeOutput metadata for structured rendering (e.g. execution
// progress steps in the Console).
var outputMetaTools = map[string]bool{
	"kubernaut_watch": true,
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
		return a2a.TextPart{Text: "...\n\n"}
	}

	text := formatStatusWithContext(template, fc.Name, fc.Args)
	return a2a.TextPart{Text: truncate(text, maxStatusLen) + "\n\n"}
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
	return a2a.TextPart{Text: truncate(text, maxSummaryLen) + "\n\n"}
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
	return a2a.TextPart{Text: text}
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

// summarizePresentDecision outputs a structured JSON payload containing the
// workflow options for rich card rendering in Console/Kagenti. The FunctionResponse
// contains {"presented":true,"message":"..."} but we reconstruct structured data
// from the original tool call args that are captured in the response context.
// Fallback: extract option names from the formatted message string.
func summarizePresentDecision(resp map[string]any) string {
	msg, _ := resp["message"].(string)
	if msg == "" {
		return `{"options":[]}`
	}
	return msg
}

// buildStreamingPartConverter returns a GenAIPartConverter for streaming mode.
// Same hybrid approach as buildPartConverter: status-like parts go through the
// EventBridge as TaskStatusUpdateEvent, LLM text goes as artifact TextParts.
// kubernaut_investigate FunctionResponse parts are suppressed since the
// EventBridge already delivered the reasoning progressively.
//
// Event-aware text routing (#1410): intermediate text (partial chunks or
// preamble before tool calls) is routed to reasoning (ThinkingPanel) instead
// of becoming a user-facing artifact. Only final definitive text without
// FunctionCalls becomes an artifact.
func buildStreamingPartConverter() adka2a.GenAIPartConverter {
	return func(ctx context.Context, event *session.Event, part *genai.Part) (a2a.Part, error) {
		if part == nil {
			return nil, nil
		}

		if part.FunctionResponse != nil && part.FunctionResponse.Name == "kubernaut_investigate" {
			return nil, nil
		}

		if part.FunctionResponse != nil && part.FunctionResponse.Name == "kubernaut_present_decision" {
			return nil, nil
		}

		bridge := EventBridgeFromContext(ctx)
		if bridge == nil {
			return convertPartToText(part), nil
		}

		if part.FunctionResponse != nil && outputMetaTools[part.FunctionResponse.Name] {
			emitStructuredOutput(ctx, bridge, part.FunctionResponse)
			return nil, nil
		}

		if isPlainText(part) && shouldRouteTextToReasoning(event) {
			_ = bridge.EmitReasoning(ctx, ensureTrailingParagraphBreak(part.Text))
			return nil, nil
		}

		return emitPartViaBridge(ctx, bridge, part), nil
	}
}

// isPlainText returns true when the part contains only text (not a function
// call, response, or thought).
func isPlainText(part *genai.Part) bool {
	return part.Text != "" && part.FunctionCall == nil && part.FunctionResponse == nil && !part.Thought
}

// shouldRouteTextToReasoning determines whether plain text should go to the
// reasoning channel (ThinkingPanel) instead of becoming a user-facing artifact.
// Text is reasoning when:
//   - The event is partial (streaming intermediate chunk), OR
//   - The event contains a FunctionCall alongside the text (preamble narration)
func shouldRouteTextToReasoning(event *session.Event) bool {
	if event == nil {
		return false
	}
	if event.Partial {
		return true
	}
	return eventHasFunctionCall(event)
}

// eventHasFunctionCall checks whether the event's content contains any
// FunctionCall part (indicating the text is preamble to a tool invocation).
func eventHasFunctionCall(event *session.Event) bool {
	if event.Content == nil {
		return false
	}
	for _, p := range event.Content.Parts {
		if p != nil && p.FunctionCall != nil {
			return true
		}
	}
	return false
}

// emitPartViaBridge routes intermediate parts (FunctionCall, FunctionResponse,
// Thought) through the EventBridge as TaskStatusUpdateEvent and returns nil.
// Non-decision FunctionCall/FunctionResponse parts emit as reasoning (for the
// ThinkingPanel). Decision-related responses use their respective metadata types.
// LLM text output is returned as a TextPart (emoji-stripped) so the ADK executor
// wraps it in a TaskArtifactUpdateEvent with the lastChunk lifecycle.
func emitPartViaBridge(ctx context.Context, bridge *EventBridge, part *genai.Part) a2a.Part {
	switch {
	case part.FunctionCall != nil:
		if part.FunctionCall.Name == "kubernaut_present_decision" {
			emitDecisionEvent(ctx, bridge, part.FunctionCall)
			return nil
		}
		text := convertFunctionCall(part.FunctionCall)
		if text != nil {
			_ = bridge.EmitReasoning(ctx, text.(a2a.TextPart).Text)
		}
		return nil
	case part.FunctionResponse != nil:
		text := convertFunctionResponse(part.FunctionResponse)
		if text != nil {
			switch {
			case decisionMetaTools[part.FunctionResponse.Name]:
				_ = bridge.EmitStructuredMeta(ctx, text.(a2a.TextPart).Text, map[string]any{"type": MetaTypeDecision})
			case outputMetaTools[part.FunctionResponse.Name]:
				_ = bridge.EmitStatus(ctx, text.(a2a.TextPart).Text)
			default:
				_ = bridge.EmitReasoning(ctx, text.(a2a.TextPart).Text)
			}
		}
		return nil
	case part.Thought:
		_ = bridge.EmitReasoning(ctx, ensureTrailingParagraphBreak(part.Text))
		return nil
	default:
		return a2a.TextPart{Text: stripEmoji(ensureTrailingParagraphBreak(part.Text))}
	}
}

// emitDecisionEvent emits the kubernaut_present_decision FunctionCall args
// as a TaskArtifactUpdateEvent with DataPart (structured JSON) + TextPart
// (human-readable fallback). Compliant with A2A v1.0 artifact conventions.
// Falls back to EmitStructuredMeta on schema validation failure for graceful
// degradation (SI-17).
func emitDecisionEvent(ctx context.Context, bridge *EventBridge, fc *genai.FunctionCall) {
	data := fc.Args
	if data == nil {
		data = map[string]any{}
	}

	summary, _ := data["summary"].(string)
	if summary == "" {
		summary = "Remediation decision"
	}
	textFallback := fmt.Sprintf("Decision: %s\n\n", summary)

	meta := map[string]any{
		"type":           MetaTypeDecision,
		"schema":         "investigation_summary",
		"schema_version": "1.0",
	}

	_ = bridge.EmitArtifact(ctx, data, textFallback, meta)
}

// emitStructuredOutput emits the FunctionResponse from kubernaut_watch as a
// structured output event so the Console can render execution progress steps.
func emitStructuredOutput(ctx context.Context, bridge *EventBridge, fr *genai.FunctionResponse) {
	if fr.Response == nil {
		return
	}

	type outputStep struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		State string `json:"state"`
	}

	type outputPayload struct {
		Steps     []outputStep `json:"steps"`
		Completed bool         `json:"completed"`
	}

	raw, err := json.Marshal(fr.Response)
	if err != nil {
		return
	}

	var result struct {
		Events []struct {
			Resource string `json:"resource"`
			Phase    string `json:"phase"`
			Message  string `json:"message"`
		} `json:"events"`
		Status  string `json:"status"`
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return
	}

	steps := make([]outputStep, 0, len(result.Events))
	for i, evt := range result.Events {
		state := "done"
		if i == len(result.Events)-1 && result.Status != "completed" {
			state = "running"
		}
		label := evt.Phase
		if evt.Message != "" {
			label = evt.Message
		}
		steps = append(steps, outputStep{
			ID:    fmt.Sprintf("s%d", i+1),
			Label: label,
			State: state,
		})
	}

	completed := result.Status == "completed"
	payload := outputPayload{Steps: steps, Completed: completed}

	out, err := json.Marshal(payload)
	if err != nil {
		return
	}

	_ = bridge.EmitStatusWithMeta(ctx, string(out), map[string]any{"type": MetaTypeOutput})
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
		return a2a.TextPart{Text: "Analyzing...\n\n"}
	}
	return a2a.TextPart{Text: ensureTrailingParagraphBreak(part.Text)}
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
