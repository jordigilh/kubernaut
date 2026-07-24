// Package ds provides the Data Store client interface and mock implementation.
package ds

import (
	"context"
)

// Client defines the interface for Data Store operations.
//
// Issue #1677 Phase 2g (DD-WORKFLOW-019): ListWorkflows (backed by DS's now-
// retired GET /api/v1/workflows) was removed from this interface. The
// kubernaut_list_workflows tool is KA-backed now -- see HandleListWorkflowsKA
// (ka_tools.go) and ka.MCPClient.ListWorkflows.
type Client interface {
	GetRemediationHistory(ctx context.Context, opts HistoryOpts) ([]HistoricalRemediation, error)
	GetEffectiveness(ctx context.Context, opts EffectivenessOpts) (*EffectivenessReport, error)
	GetAuditTrail(ctx context.Context, opts AuditTrailOpts) ([]AuditEvent, error)
}

// HistoryOpts are the query options for remediation history.
type HistoryOpts struct {
	Namespace string
	Kind      string
	Name      string
	Since     string
	SpecHash  string
}

// HistoricalRemediation is a past remediation record from the Data Store.
type HistoricalRemediation struct {
	ID        string `json:"id"`
	Namespace string `json:"namespace"`
	Phase     string `json:"phase"`
	CreatedAt string `json:"created_at"`
	Workflow  string `json:"workflow,omitempty"`
}

// EffectivenessOpts are the query options for effectiveness scores.
type EffectivenessOpts struct {
	WorkflowID string
	Namespace  string
}

// EffectivenessReport contains workflow effectiveness metrics.
type EffectivenessReport struct {
	WorkflowID  string  `json:"workflow_id,omitempty"`
	SuccessRate float64 `json:"success_rate"`
	AvgDuration string  `json:"avg_duration,omitempty"`
	SampleSize  int     `json:"sample_size"`
}

// AuditTrailOpts are the query options for audit trail.
type AuditTrailOpts struct {
	RRID      string
	EventType string
}

// AuditEvent represents a single audit trail entry.
type AuditEvent struct {
	Timestamp string `json:"timestamp"`
	EventType string `json:"event_type"`
	Actor     string `json:"actor"`
	Detail    string `json:"detail,omitempty"`
}

// MockClient is a test double for the DS Client interface.
type MockClient struct {
	GetRemediationHistoryFn func(ctx context.Context, opts HistoryOpts) ([]HistoricalRemediation, error)
	GetEffectivenessFn      func(ctx context.Context, opts EffectivenessOpts) (*EffectivenessReport, error)
	GetAuditTrailFn         func(ctx context.Context, opts AuditTrailOpts) ([]AuditEvent, error)
}

// GetRemediationHistory delegates to the mock function.
func (m *MockClient) GetRemediationHistory(ctx context.Context, opts HistoryOpts) ([]HistoricalRemediation, error) {
	return m.GetRemediationHistoryFn(ctx, opts)
}

// GetEffectiveness delegates to the mock function.
func (m *MockClient) GetEffectiveness(ctx context.Context, opts EffectivenessOpts) (*EffectivenessReport, error) {
	return m.GetEffectivenessFn(ctx, opts)
}

// GetAuditTrail delegates to the mock function.
func (m *MockClient) GetAuditTrail(ctx context.Context, opts AuditTrailOpts) ([]AuditEvent, error) {
	return m.GetAuditTrailFn(ctx, opts)
}

// Compile-time interface check.
var _ Client = (*MockClient)(nil)
