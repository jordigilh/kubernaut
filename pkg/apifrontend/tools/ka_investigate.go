package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
)

// InvestigateArgs defines the input for the merged kubernaut_investigate tool.
// Two modes:
//   - New investigation: provide namespace + name (+ optional kind). session_id must be empty.
//   - Resume/poll existing: provide session_id. namespace/name/kind are ignored.
type InvestigateArgs struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
	Kind      string `json:"kind,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// InvestigateResult is the output of kubernaut_investigate.
type InvestigateResult struct {
	SessionID  string              `json:"session_id"`
	Status     string              `json:"status"`
	Summary    string              `json:"summary,omitempty"`
	Events     []StreamEventDigest `json:"events,omitempty"`
	EventLog   string              `json:"event_log,omitempty"`
	ToolErrors []string            `json:"tool_errors,omitempty"`
}

// HandleInvestigation implements the merged kubernaut_investigate tool.
// When session_id is empty, it starts a new investigation (Analyze + StreamEvents).
// When session_id is present, it resumes or polls an existing investigation.
func HandleInvestigation(ctx context.Context, kaClient *ka.Client, args InvestigateArgs, auditor audit.Emitter) (InvestigateResult, error) {
	if args.SessionID != "" {
		return handleExistingSession(ctx, kaClient, args.SessionID, auditor)
	}
	return handleNewInvestigation(ctx, kaClient, args, auditor)
}

func handleNewInvestigation(ctx context.Context, kaClient *ka.Client, args InvestigateArgs, auditor audit.Emitter) (InvestigateResult, error) {
	if args.Namespace == "" || args.Name == "" {
		return InvestigateResult{}, fmt.Errorf("namespace and name are required when starting a new investigation (no session_id provided)")
	}

	sessionID, err := kaClient.Analyze(ctx, ka.AnalyzeRequest{
		Namespace: args.Namespace,
		Kind:      args.Kind,
		Name:      args.Name,
	})
	if err != nil {
		return InvestigateResult{}, fmt.Errorf("starting investigation: %w", err)
	}

	if auditor != nil {
		auditor.Emit(ctx, &audit.Event{
			Type: audit.EventKADelegated,
			Detail: map[string]string{
				"namespace":         args.Namespace,
				"rr_name":           args.Name,
				"session_id":        sessionID,
				"ka_correlation_id": sessionID,
				"delegation_type":   "autonomous",
			},
		})
	}

	result, err := streamInvestigation(ctx, kaClient, sessionID)
	if err != nil {
		return InvestigateResult{}, err
	}
	result.SessionID = sessionID
	emitResultReceivedIfTerminal(ctx, auditor, sessionID, result.Status)
	return result, nil
}

func handleExistingSession(ctx context.Context, kaClient *ka.Client, sessionID string, auditor audit.Emitter) (InvestigateResult, error) {
	status, err := kaClient.Status(ctx, sessionID)
	if err != nil {
		return InvestigateResult{}, fmt.Errorf(
			"session %q does not exist on the investigation service — "+
				"provide namespace and name to start a new investigation", sessionID)
	}

	switch status.Status {
	case "completed":
		return fetchCompletedResult(ctx, kaClient, sessionID, auditor)
	case "failed":
		if auditor != nil {
			auditor.Emit(ctx, &audit.Event{
				Type: audit.EventKAResultReceived,
				Detail: map[string]string{
					"session_id":        sessionID,
					"ka_correlation_id": sessionID,
					"result_type":       "rca_failed",
				},
			})
		}
		return InvestigateResult{
			SessionID: sessionID,
			Status:    "failed",
		}, nil
	case "cancelled":
		return InvestigateResult{
			SessionID: sessionID,
			Status:    "cancelled",
		}, nil
	default:
		result, err := streamInvestigation(ctx, kaClient, sessionID)
		if err != nil {
			return InvestigateResult{}, err
		}
		result.SessionID = sessionID
		emitResultReceivedIfTerminal(ctx, auditor, sessionID, result.Status)
		return result, nil
	}
}

func fetchCompletedResult(ctx context.Context, kaClient *ka.Client, sessionID string, auditor audit.Emitter) (InvestigateResult, error) {
	result, err := kaClient.Result(ctx, sessionID)
	if err != nil {
		return InvestigateResult{}, fmt.Errorf("fetching investigation result: %w", err)
	}
	if auditor != nil {
		auditor.Emit(ctx, &audit.Event{
			Type: audit.EventKAResultReceived,
			Detail: map[string]string{
				"session_id":        sessionID,
				"ka_correlation_id": sessionID,
				"result_type":       "rca_complete",
			},
		})
	}
	return InvestigateResult{
		SessionID: sessionID,
		Status:    "completed",
		Summary:   result.Summary,
	}, nil
}

// emitResultReceivedIfTerminal emits a ka.result_received audit event when
// streaming ends in a terminal state (completed or failed). This closes the
// AU-12 gap: every delegation (ka.delegated) must have a matching result event.
func emitResultReceivedIfTerminal(ctx context.Context, auditor audit.Emitter, sessionID, status string) {
	if auditor == nil {
		return
	}
	var resultType string
	switch status {
	case "completed":
		resultType = "rca_complete"
	case "failed":
		resultType = "rca_failed"
	default:
		return
	}
	auditor.Emit(ctx, &audit.Event{
		Type: audit.EventKAResultReceived,
		Detail: map[string]string{
			"session_id":        sessionID,
			"ka_correlation_id": sessionID,
			"result_type":       resultType,
		},
	})
}

func streamInvestigation(ctx context.Context, kaClient *ka.Client, sessionID string) (InvestigateResult, error) {
	logger := logr.FromContextOrDiscard(ctx)

	ch, err := kaClient.StreamEvents(ctx, sessionID)
	if err != nil {
		return InvestigateResult{}, fmt.Errorf("connecting to investigation stream: %w", err)
	}

	var events []StreamEventDigest
	var narrative strings.Builder
	var finalSummary string
	var toolErrors []string
	var bridgedCount int

	logger.Info("investigation stream opened", "session_id", sessionID)

	for event := range ch {
		select {
		case <-ctx.Done():
			logger.Info("investigation stream cancelled",
				"session_id", sessionID, "events_bridged", bridgedCount, "tool_errors", len(toolErrors))
			return InvestigateResult{
				Status:     "cancelled",
				Events:     events,
				EventLog:   narrative.String(),
				ToolErrors: toolErrors,
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
			emitViaBridgeSafe(ctx, text)
			bridgedCount++
		case ka.EventTypeToolCallStart, ka.EventTypeToolCall:
			text := extractTextFromData(event.Data)
			digest.Text = text
			if text != "" {
				narrative.WriteString(fmt.Sprintf("\n[Tool: %s]\n", text))
				emitViaBridgeSafe(ctx, fmt.Sprintf("[Tool: %s]", text))
				bridgedCount++
			}
		case ka.EventTypeToolResult:
			toolName, preview, isErr := ExtractToolResult(event.Data)
			digest.Text = truncateForLLM(preview, 500)
			if isErr {
				errMsg := FormatToolError(toolName, preview)
				toolErrors = append(toolErrors, errMsg)
				emitViaBridgeSafe(ctx, fmt.Sprintf("[Error: %s]", errMsg))
				bridgedCount++
				logger.Info("tool error detected", "session_id", sessionID, "tool", toolName)
			}
		case ka.EventTypeComplete:
			finalSummary = extractSummaryFromComplete(event.Data)
			events = append(events, digest)
			emitViaBridgeSafe(ctx, finalSummary)
			bridgedCount++
			logger.Info("investigation stream completed",
				"session_id", sessionID, "events_bridged", bridgedCount, "tool_errors", len(toolErrors))
			return InvestigateResult{
				Status:     "completed",
				Summary:    finalSummary,
				Events:     events,
				EventLog:   narrative.String(),
				ToolErrors: toolErrors,
			}, nil
		case ka.EventTypeCancelled:
			events = append(events, digest)
			logger.Info("investigation stream cancelled by KA",
				"session_id", sessionID, "events_bridged", bridgedCount, "tool_errors", len(toolErrors))
			return InvestigateResult{
				Status:     "cancelled",
				Events:     events,
				EventLog:   narrative.String(),
				ToolErrors: toolErrors,
			}, nil
		case ka.EventTypeError:
			text := extractTextFromData(event.Data)
			digest.Text = text
			events = append(events, digest)
			emitViaBridgeSafe(ctx, fmt.Sprintf("[Investigation error: %s]", truncateForLLM(text, 200)))
			bridgedCount++
			logger.Info("investigation stream error",
				"session_id", sessionID, "error", text, "events_bridged", bridgedCount, "tool_errors", len(toolErrors))
			return InvestigateResult{
				Status:     "failed",
				Events:     events,
				EventLog:   narrative.String(),
				ToolErrors: toolErrors,
			}, nil
		}

		events = append(events, digest)
	}

	status := "completed"
	if finalSummary == "" {
		status = "disconnected"
	}
	logger.Info("investigation stream closed",
		"session_id", sessionID, "status", status, "events_bridged", bridgedCount, "tool_errors", len(toolErrors))
	return InvestigateResult{
		Status:     status,
		Summary:    finalSummary,
		Events:     events,
		EventLog:   narrative.String(),
		ToolErrors: toolErrors,
	}, nil
}

// emitViaBridgeSafe sends text to the A2A event bridge if present in context.
// Bridge write errors are logged but do not interrupt the tool execution.
func emitViaBridgeSafe(ctx context.Context, text string) {
	if text == "" {
		return
	}
	if err := launcher.EmitReasoningSafe(ctx, text); err != nil {
		logr.FromContextOrDiscard(ctx).Info("WARNING: bridge emit failed", "error", security.RedactError(err))
	}
}

// NewInvestigateTool creates the kubernaut_investigate tool that merges
// start, stream, and poll into a single LLM-callable operation.
func NewInvestigateTool(kaClient *ka.Client, auditor audit.Emitter) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: "kubernaut_investigate",
		Description: "Investigate an infrastructure incident. " +
			"To start a new investigation, provide namespace and name (and optional kind). " +
			"To resume or check an existing investigation, provide session_id. " +
			"Returns the investigation summary when complete.",
	}, func(ctx tool.Context, args InvestigateArgs) (InvestigateResult, error) {
		return HandleInvestigation(ctx, kaClient, args, auditor)
	})
}

