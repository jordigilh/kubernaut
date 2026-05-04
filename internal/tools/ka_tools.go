package tools

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/jordigilh/kubernaut-apifrontend/internal/ka"
)

// StartInvestigationArgs defines the input for kubernaut_start_investigation.
type StartInvestigationArgs struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Kind      string `json:"kind,omitempty"`
}

// StartInvestigationResult is the output of kubernaut_start_investigation.
type StartInvestigationResult struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

// HandleStartInvestigation implements the kubernaut_start_investigation logic.
func HandleStartInvestigation(ctx context.Context, kaClient *ka.Client, args StartInvestigationArgs) (StartInvestigationResult, error) {
	sessionID, err := kaClient.Analyze(ctx, ka.AnalyzeRequest{
		Namespace: args.Namespace,
		Kind:      args.Kind,
		Name:      args.Name,
	})
	if err != nil {
		return StartInvestigationResult{}, fmt.Errorf("starting investigation: %w", err)
	}

	return StartInvestigationResult{
		SessionID: sessionID,
		Status:    "started",
		Message:   fmt.Sprintf("Investigation started for %s/%s (session: %s)", args.Namespace, args.Name, sessionID),
	}, nil
}

// NewStartInvestigationTool creates the kubernaut_start_investigation tool.
func NewStartInvestigationTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_start_investigation",
		Description: "Start an AI-powered investigation for an incident, returning a session ID for tracking",
	}, func(ctx tool.Context, args StartInvestigationArgs) (StartInvestigationResult, error) {
		return StartInvestigationResult{}, fmt.Errorf("not implemented: requires wiring in PR5")
	})
}

// PollInvestigationArgs defines the input for kubernaut_poll_investigation.
type PollInvestigationArgs struct {
	SessionID string `json:"session_id"`
}

// PollInvestigationResult is the output of kubernaut_poll_investigation.
type PollInvestigationResult struct {
	Status    string `json:"status"`
	Progress  string `json:"progress,omitempty"`
	Summary   string `json:"summary,omitempty"`
	PollCount int    `json:"poll_count"`
}

// HandlePollInvestigation implements kubernaut_poll_investigation with blocking poll.
// maxPolls controls how many times to poll; pollInterval is the wait between polls.
func HandlePollInvestigation(ctx context.Context, kaClient *ka.Client, args PollInvestigationArgs, maxPolls int, pollInterval time.Duration) (PollInvestigationResult, error) {
	for i := 1; i <= maxPolls; i++ {
		status, err := kaClient.Status(ctx, args.SessionID)
		if err != nil {
			return PollInvestigationResult{}, fmt.Errorf("polling investigation: %w", err)
		}

		switch status.Status {
		case "completed":
			result, err := kaClient.Result(ctx, args.SessionID)
			if err != nil {
				return PollInvestigationResult{
					Status:    "completed",
					PollCount: i,
				}, nil
			}
			return PollInvestigationResult{
				Status:    "completed",
				Summary:   result.Summary,
				PollCount: i,
			}, nil

		case "failed":
			return PollInvestigationResult{
				Status:    "failed",
				Progress:  status.Error,
				PollCount: i,
			}, nil

		case "cancelled":
			return PollInvestigationResult{
				Status:    "cancelled",
				PollCount: i,
			}, nil
		}

		if i < maxPolls {
			select {
			case <-ctx.Done():
				return PollInvestigationResult{}, ctx.Err()
			case <-time.After(pollInterval):
			}
		}
	}

	return PollInvestigationResult{
		Status:    "in_progress",
		Progress:  "Investigation is still running. Call kubernaut_poll_investigation again.",
		PollCount: maxPolls,
	}, nil
}

// NewPollInvestigationTool creates the kubernaut_poll_investigation tool.
func NewPollInvestigationTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_poll_investigation",
		Description: "Check investigation progress. Blocks for up to 15 seconds polling every 3 seconds. Re-call if status is in_progress.",
	}, func(ctx tool.Context, args PollInvestigationArgs) (PollInvestigationResult, error) {
		return PollInvestigationResult{}, fmt.Errorf("not implemented: requires wiring in PR5")
	})
}

// SelectWorkflowArgs defines the input for kubernaut_select_workflow.
type SelectWorkflowArgs struct {
	RRID       string `json:"rr_id"`
	WorkflowID string `json:"workflow_id"`
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
}

// SelectWorkflowResult is the output of kubernaut_select_workflow.
type SelectWorkflowResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// HandleSelectWorkflow implements kubernaut_select_workflow via KA MCP.
//
//nolint:gocritic // hugeParam: args passed by value for simplicity; not performance-critical
func HandleSelectWorkflow(ctx context.Context, mcpClient ka.MCPClient, args SelectWorkflowArgs) (SelectWorkflowResult, error) {
	result, err := mcpClient.SelectWorkflow(ctx, ka.SelectWorkflowArgs{
		RRID:       args.RRID,
		WorkflowID: args.WorkflowID,
		Kind:       args.Kind,
		Name:       args.Name,
		Namespace:  args.Namespace,
	})
	if err != nil {
		return SelectWorkflowResult{}, fmt.Errorf("selecting workflow: %w", err)
	}

	return SelectWorkflowResult{
		Status:  result.Status,
		Message: result.Message,
	}, nil
}

// NewSelectWorkflowTool creates the kubernaut_select_workflow tool.
func NewSelectWorkflowTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_select_workflow",
		Description: "Select a remediation workflow for execution. Triggers enrichment and workflow selection in the backend.",
	}, func(ctx tool.Context, args SelectWorkflowArgs) (SelectWorkflowResult, error) {
		return SelectWorkflowResult{}, fmt.Errorf("not implemented: requires wiring in PR5")
	})
}

// PresentDecisionArgs defines the input for present_decision.
type PresentDecisionArgs struct {
	SessionID string           `json:"session_id"`
	Summary   string           `json:"summary"`
	Options   []WorkflowOption `json:"options"`
}

// WorkflowOption represents a remediation workflow choice.
type WorkflowOption struct {
	WorkflowID  string `json:"workflow_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Risk        string `json:"risk,omitempty"`
}

// PresentDecisionResult is the output of present_decision.
type PresentDecisionResult struct {
	Presented bool   `json:"presented"`
	Message   string `json:"message"`
}

// HandlePresentDecision formats RCA and options for user presentation.
func HandlePresentDecision(args PresentDecisionArgs) PresentDecisionResult {
	msg := fmt.Sprintf("Investigation complete for session %s.\n\nSummary: %s\n\nAvailable actions:", args.SessionID, args.Summary)
	for i, opt := range args.Options {
		msg += fmt.Sprintf("\n  %d. %s", i+1, opt.Name)
		if opt.Description != "" {
			msg += fmt.Sprintf(" - %s", opt.Description)
		}
	}
	return PresentDecisionResult{
		Presented: true,
		Message:   msg,
	}
}

// NewPresentDecisionTool creates the present_decision tool (IsLongRunning).
func NewPresentDecisionTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:          "present_decision",
		Description:   "Present investigation results and remediation options to the user for a decision",
		IsLongRunning: true,
	}, func(ctx tool.Context, args PresentDecisionArgs) (PresentDecisionResult, error) {
		return HandlePresentDecision(args), nil
	})
}
