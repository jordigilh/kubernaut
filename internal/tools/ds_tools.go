package tools

import (
	"context"
	"fmt"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/jordigilh/kubernaut-apifrontend/internal/ds"
)

// ListWorkflowsArgs defines the input for kubernaut_list_workflows.
type ListWorkflowsArgs struct {
	Kind string `json:"kind,omitempty"`
}

// ListWorkflowsResult is the output of kubernaut_list_workflows.
type ListWorkflowsResult struct {
	Workflows []WorkflowSummary `json:"workflows"`
	Count     int               `json:"count"`
}

// WorkflowSummary is a compact view of a workflow definition.
type WorkflowSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind,omitempty"`
}

// HandleListWorkflows implements the kubernaut_list_workflows logic.
func HandleListWorkflows(ctx context.Context, client ds.Client, args ListWorkflowsArgs) (ListWorkflowsResult, error) {
	workflows, err := client.ListWorkflows(ctx, ds.ListWorkflowsOpts{Kind: args.Kind})
	if err != nil {
		return ListWorkflowsResult{}, fmt.Errorf("querying workflow catalog: %w", err)
	}

	summaries := make([]WorkflowSummary, 0, len(workflows))
	for _, w := range workflows {
		summaries = append(summaries, WorkflowSummary{
			ID: w.ID, Name: w.Name, Description: w.Description, Kind: w.Kind,
		})
	}

	return ListWorkflowsResult{Workflows: summaries, Count: len(summaries)}, nil
}

// NewListWorkflowsTool creates the kubernaut_list_workflows tool.
func NewListWorkflowsTool(client ds.Client) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_list_workflows",
		Description: "List available remediation workflows from the catalog, optionally filtered by resource kind",
	}, func(ctx tool.Context, args ListWorkflowsArgs) (ListWorkflowsResult, error) {
		return HandleListWorkflows(ctx, client, args)
	})
}

// GetRemediationHistoryArgs defines the input for kubernaut_get_remediation_history.
type GetRemediationHistoryArgs struct {
	Namespace string `json:"namespace,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Name      string `json:"name,omitempty"`
	Since     string `json:"since,omitempty"`
}

// HistoricalRemediation is a past remediation record from the Data Store.
type HistoricalRemediation struct {
	ID        string `json:"id"`
	Namespace string `json:"namespace"`
	Phase     string `json:"phase"`
	CreatedAt string `json:"created_at"`
	Workflow  string `json:"workflow,omitempty"`
}

// GetRemediationHistoryResult is the output of kubernaut_get_remediation_history.
type GetRemediationHistoryResult struct {
	Remediations []HistoricalRemediation `json:"remediations"`
	Count        int                     `json:"count"`
}

// HandleGetRemediationHistory implements the kubernaut_get_remediation_history logic.
func HandleGetRemediationHistory(ctx context.Context, client ds.Client, args GetRemediationHistoryArgs) (GetRemediationHistoryResult, error) {
	history, err := client.GetRemediationHistory(ctx, ds.HistoryOpts{
		Namespace: args.Namespace, Kind: args.Kind, Name: args.Name, Since: args.Since,
	})
	if err != nil {
		return GetRemediationHistoryResult{}, fmt.Errorf("querying remediation history: %w", err)
	}

	items := make([]HistoricalRemediation, 0, len(history))
	for _, h := range history {
		items = append(items, HistoricalRemediation{
			ID: h.ID, Namespace: h.Namespace, Phase: h.Phase, CreatedAt: h.CreatedAt, Workflow: h.Workflow,
		})
	}

	return GetRemediationHistoryResult{Remediations: items, Count: len(items)}, nil
}

// NewGetRemediationHistoryTool creates the kubernaut_get_remediation_history tool.
func NewGetRemediationHistoryTool(client ds.Client) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_get_remediation_history",
		Description: "Query historical remediations from the Data Store with optional filtering",
	}, func(ctx tool.Context, args GetRemediationHistoryArgs) (GetRemediationHistoryResult, error) {
		return HandleGetRemediationHistory(ctx, client, args)
	})
}

// GetEffectivenessArgs defines the input for kubernaut_get_effectiveness.
type GetEffectivenessArgs struct {
	WorkflowID string `json:"workflow_id,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
}

// GetEffectivenessResult is the output of kubernaut_get_effectiveness.
type GetEffectivenessResult struct {
	WorkflowID  string  `json:"workflow_id,omitempty"`
	SuccessRate float64 `json:"success_rate"`
	AvgDuration string  `json:"avg_duration,omitempty"`
	SampleSize  int     `json:"sample_size"`
}

// HandleGetEffectiveness implements the kubernaut_get_effectiveness logic.
func HandleGetEffectiveness(ctx context.Context, client ds.Client, args GetEffectivenessArgs) (GetEffectivenessResult, error) {
	report, err := client.GetEffectiveness(ctx, ds.EffectivenessOpts{
		WorkflowID: args.WorkflowID, Namespace: args.Namespace,
	})
	if err != nil {
		return GetEffectivenessResult{}, fmt.Errorf("querying effectiveness: %w", err)
	}

	return GetEffectivenessResult{
		WorkflowID: report.WorkflowID, SuccessRate: report.SuccessRate,
		AvgDuration: report.AvgDuration, SampleSize: report.SampleSize,
	}, nil
}

// NewGetEffectivenessTool creates the kubernaut_get_effectiveness tool.
func NewGetEffectivenessTool(client ds.Client) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_get_effectiveness",
		Description: "Get effectiveness scores and metrics for remediation workflows",
	}, func(ctx tool.Context, args GetEffectivenessArgs) (GetEffectivenessResult, error) {
		return HandleGetEffectiveness(ctx, client, args)
	})
}

// GetAuditTrailArgs defines the input for kubernaut_get_audit_trail.
type GetAuditTrailArgs struct {
	RRID      string `json:"rr_id"`
	EventType string `json:"event_type,omitempty"`
}

// AuditEvent represents a single audit trail entry.
type AuditEvent struct {
	Timestamp string `json:"timestamp"`
	EventType string `json:"event_type"`
	Actor     string `json:"actor"`
	Detail    string `json:"detail,omitempty"`
}

// GetAuditTrailResult is the output of kubernaut_get_audit_trail.
type GetAuditTrailResult struct {
	Events []AuditEvent `json:"events"`
	Count  int          `json:"count"`
}

// HandleGetAuditTrail implements the kubernaut_get_audit_trail logic.
func HandleGetAuditTrail(ctx context.Context, client ds.Client, args GetAuditTrailArgs) (GetAuditTrailResult, error) {
	events, err := client.GetAuditTrail(ctx, ds.AuditTrailOpts{
		RRID: args.RRID, EventType: args.EventType,
	})
	if err != nil {
		return GetAuditTrailResult{}, fmt.Errorf("querying audit trail: %w", err)
	}

	items := make([]AuditEvent, 0, len(events))
	for _, e := range events {
		items = append(items, AuditEvent{
			Timestamp: e.Timestamp, EventType: e.EventType, Actor: e.Actor, Detail: e.Detail,
		})
	}

	return GetAuditTrailResult{Events: items, Count: len(items)}, nil
}

// NewGetAuditTrailTool creates the kubernaut_get_audit_trail tool.
func NewGetAuditTrailTool(client ds.Client) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_get_audit_trail",
		Description: "Retrieve the audit trail for a remediation, showing all actions and decisions",
	}, func(ctx tool.Context, args GetAuditTrailArgs) (GetAuditTrailResult, error) {
		return HandleGetAuditTrail(ctx, client, args)
	})
}
