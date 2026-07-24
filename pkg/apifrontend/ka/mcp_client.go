package ka

import "context"

// MCPClient is the interface for KA MCP operations.
type MCPClient interface {
	SelectWorkflow(ctx context.Context, args SelectWorkflowArgs) (*SelectWorkflowResult, error)
	Investigate(ctx context.Context, args InvestigateArgs) (*InvestigateResult, error)
	DiscoverWorkflows(ctx context.Context, args DiscoverWorkflowsArgs) (*DiscoverWorkflowsResult, error)
	InvokeAction(ctx context.Context, args InvokeActionArgs) (*InvokeActionResult, error)
	StartInvestigation(ctx context.Context, args StartInvestigationArgs) (*StartInvestigationResult, error)
	CompleteNoAction(ctx context.Context, args CompleteNoActionArgs) (*CompleteNoActionResult, error)
	// ListWorkflows lists the workflow catalog, optionally filtered by
	// resource kind. Stateless (no rr_id/session) -- #1677 Phase 2f
	// (DD-WORKFLOW-019).
	ListWorkflows(ctx context.Context, args ListWorkflowsArgs) (*ListWorkflowsResult, error)
}

// ListWorkflowsArgs is the input for MCPClient.ListWorkflows.
type ListWorkflowsArgs struct {
	Kind string
}

// WorkflowSummary is a compact view of a cataloged workflow.
type WorkflowSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind,omitempty"`
}

// ListWorkflowsResult is the output of MCPClient.ListWorkflows.
type ListWorkflowsResult struct {
	Workflows []WorkflowSummary `json:"workflows"`
	Count     int               `json:"count"`
}

// MockMCPClient is a test double for MCPClient.
type MockMCPClient struct {
	SelectWorkflowFn     func(ctx context.Context, args SelectWorkflowArgs) (*SelectWorkflowResult, error)
	InvestigateFn        func(ctx context.Context, args InvestigateArgs) (*InvestigateResult, error)
	DiscoverWorkflowsFn  func(ctx context.Context, args DiscoverWorkflowsArgs) (*DiscoverWorkflowsResult, error)
	InvokeActionFn       func(ctx context.Context, args InvokeActionArgs) (*InvokeActionResult, error)
	StartInvestigationFn func(ctx context.Context, args StartInvestigationArgs) (*StartInvestigationResult, error)
	CompleteNoActionFn   func(ctx context.Context, args CompleteNoActionArgs) (*CompleteNoActionResult, error)
	ListWorkflowsFn      func(ctx context.Context, args ListWorkflowsArgs) (*ListWorkflowsResult, error)
	Token                string
}

// SelectWorkflow calls the mock function.
//
//nolint:gocritic // hugeParam: matches MCPClient interface contract
func (m *MockMCPClient) SelectWorkflow(ctx context.Context, args SelectWorkflowArgs) (*SelectWorkflowResult, error) {
	return m.SelectWorkflowFn(ctx, args)
}

// Investigate calls the mock function.
//
//nolint:gocritic // hugeParam: matches MCPClient interface contract
func (m *MockMCPClient) Investigate(ctx context.Context, args InvestigateArgs) (*InvestigateResult, error) {
	if m.InvestigateFn != nil {
		return m.InvestigateFn(ctx, args)
	}
	return nil, ErrMCPUnavailable
}

// DiscoverWorkflows calls the mock function.
//
//nolint:gocritic // hugeParam: matches MCPClient interface contract
func (m *MockMCPClient) DiscoverWorkflows(ctx context.Context, args DiscoverWorkflowsArgs) (*DiscoverWorkflowsResult, error) {
	if m.DiscoverWorkflowsFn != nil {
		return m.DiscoverWorkflowsFn(ctx, args)
	}
	return nil, ErrMCPUnavailable
}

// InvokeAction calls the mock function.
func (m *MockMCPClient) InvokeAction(ctx context.Context, args InvokeActionArgs) (*InvokeActionResult, error) {
	if m.InvokeActionFn != nil {
		return m.InvokeActionFn(ctx, args)
	}
	return nil, ErrMCPUnavailable
}

// StartInvestigation calls the mock function.
func (m *MockMCPClient) StartInvestigation(ctx context.Context, args StartInvestigationArgs) (*StartInvestigationResult, error) {
	if m.StartInvestigationFn != nil {
		return m.StartInvestigationFn(ctx, args)
	}
	return nil, ErrMCPUnavailable
}

// CompleteNoAction calls the mock function.
func (m *MockMCPClient) CompleteNoAction(ctx context.Context, args CompleteNoActionArgs) (*CompleteNoActionResult, error) {
	if m.CompleteNoActionFn != nil {
		return m.CompleteNoActionFn(ctx, args)
	}
	return nil, ErrMCPUnavailable
}

// ListWorkflows calls the mock function.
func (m *MockMCPClient) ListWorkflows(ctx context.Context, args ListWorkflowsArgs) (*ListWorkflowsResult, error) {
	if m.ListWorkflowsFn != nil {
		return m.ListWorkflowsFn(ctx, args)
	}
	return nil, ErrMCPUnavailable
}
