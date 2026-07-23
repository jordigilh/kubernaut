package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ds"
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

// ErrDSUnavailable is returned when the Data Store client is nil.
var ErrDSUnavailable = fmt.Errorf("datastorage service unavailable")

// GetRemediationHistoryArgs defines the input for kubernaut_get_remediation_history.
type GetRemediationHistoryArgs struct {
	Namespace string `json:"namespace,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Name      string `json:"name,omitempty"`
	Since     string `json:"since,omitempty"`
	SpecHash  string `json:"spec_hash,omitempty"`
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
	if client == nil {
		return GetRemediationHistoryResult{}, ErrDSUnavailable
	}
	if args.Kind == "" {
		return GetRemediationHistoryResult{}, fmt.Errorf("kind is required for remediation history lookup")
	}
	if args.Name == "" {
		return GetRemediationHistoryResult{}, fmt.Errorf("name is required for remediation history lookup")
	}
	if args.SpecHash == "" {
		return GetRemediationHistoryResult{}, fmt.Errorf("spec_hash is required for remediation history lookup")
	}
	if !strings.HasPrefix(args.SpecHash, "sha256:") {
		return GetRemediationHistoryResult{}, fmt.Errorf("spec_hash must use sha256: prefix (got %q)", args.SpecHash)
	}
	history, err := client.GetRemediationHistory(ctx, ds.HistoryOpts{
		Namespace: args.Namespace, Kind: args.Kind, Name: args.Name, Since: args.Since, SpecHash: args.SpecHash,
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
		Description: "Query historical remediations from the Data Store. Required params: kind, name, spec_hash. Optional: namespace, since.",
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
	if client == nil {
		return GetEffectivenessResult{}, ErrDSUnavailable
	}
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

// PhaseGroup represents an aggregated group of audit events in one lifecycle phase.
type PhaseGroup struct {
	Phase      string `json:"phase"`
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
	EventCount int    `json:"event_count"`
	Outcome    string `json:"outcome,omitempty"`
	KeyActions string `json:"key_actions"`
	Actor      string `json:"actor,omitempty"`
}

// GetAuditTrailResult is the output of kubernaut_get_audit_trail.
type GetAuditTrailResult struct {
	Lifecycle   string       `json:"lifecycle"`
	Phases      []PhaseGroup `json:"phases"`
	TotalEvents int          `json:"total_events"`
}

// aggregateByPhase transforms raw DS audit events into a lifecycle-ordered
// phase summary. Each PhaseGroup aggregates events sharing the same lifecycle
// phase, tracking timestamps, actors, and key actions.
func aggregateByPhase(events []ds.AuditEvent) GetAuditTrailResult {
	if len(events) == 0 {
		return GetAuditTrailResult{}
	}

	type phaseAccum struct {
		startTime  string
		endTime    string
		eventCount int
		actors     map[string]struct{}
		actions    []string
	}

	accum := make(map[string]*phaseAccum)
	for _, e := range events {
		phase := resolvePhase(e.EventType)
		a, ok := accum[phase]
		if !ok {
			a = &phaseAccum{
				startTime: e.Timestamp,
				endTime:   e.Timestamp,
				actors:    make(map[string]struct{}),
			}
			accum[phase] = a
		}
		a.eventCount++
		if e.Timestamp < a.startTime {
			a.startTime = e.Timestamp
		}
		if e.Timestamp > a.endTime {
			a.endTime = e.Timestamp
		}
		if e.Actor != "" {
			a.actors[e.Actor] = struct{}{}
		}
		if e.Detail != "" {
			a.actions = append(a.actions, e.Detail)
		} else {
			a.actions = append(a.actions, e.EventType)
		}
	}

	var phases []PhaseGroup
	var phaseNames []string
	for _, name := range phaseOrder {
		a, ok := accum[name]
		if !ok {
			continue
		}
		actorList := make([]string, 0, len(a.actors))
		for actor := range a.actors {
			actorList = append(actorList, actor)
		}
		sort.Strings(actorList)

		phases = append(phases, PhaseGroup{
			Phase:      name,
			StartTime:  a.startTime,
			EndTime:    a.endTime,
			EventCount: a.eventCount,
			KeyActions: capKeyActions(a.actions),
			Actor:      strings.Join(actorList, ", "),
		})
		phaseNames = append(phaseNames, name)
	}

	return GetAuditTrailResult{
		Lifecycle:   strings.Join(phaseNames, " -> "),
		Phases:      phases,
		TotalEvents: len(events),
	}
}

// maxKeyActionsBytes caps the per-phase KeyActions string to keep the
// aggregated result well within MaxToolResultBytes. 1024 bytes per phase
// across 9 phases leaves ample headroom for the rest of the JSON envelope.
const maxKeyActionsBytes = 1024

// capKeyActions joins action strings with "; " and truncates to
// maxKeyActionsBytes, appending "... and N more actions" when capped.
func capKeyActions(actions []string) string {
	joined := strings.Join(actions, "; ")
	if len(joined) <= maxKeyActionsBytes {
		return joined
	}

	included := 0
	size := 0
	for i, a := range actions {
		need := len(a)
		if i > 0 {
			need += 2 // "; " separator
		}
		if size+need > maxKeyActionsBytes {
			break
		}
		size += need
		included++
	}
	if included == 0 {
		included = 1
	}
	remaining := len(actions) - included
	result := strings.Join(actions[:included], "; ")
	if remaining > 0 {
		result += fmt.Sprintf("... and %d more actions", remaining)
	}
	return result
}

// HandleGetAuditTrail implements the kubernaut_get_audit_trail logic.
func HandleGetAuditTrail(ctx context.Context, client ds.Client, args GetAuditTrailArgs) (GetAuditTrailResult, error) {
	if client == nil {
		return GetAuditTrailResult{}, ErrDSUnavailable
	}
	events, err := client.GetAuditTrail(ctx, ds.AuditTrailOpts{
		RRID: args.RRID, EventType: args.EventType,
	})
	if err != nil {
		return GetAuditTrailResult{}, fmt.Errorf("querying audit trail: %w", err)
	}

	return aggregateByPhase(events), nil
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
