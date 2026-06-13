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
}

// MockMCPClient is a test double for MCPClient.
type MockMCPClient struct {
	SelectWorkflowFn      func(ctx context.Context, args SelectWorkflowArgs) (*SelectWorkflowResult, error)
	InvestigateFn         func(ctx context.Context, args InvestigateArgs) (*InvestigateResult, error)
	DiscoverWorkflowsFn   func(ctx context.Context, args DiscoverWorkflowsArgs) (*DiscoverWorkflowsResult, error)
	InvokeActionFn        func(ctx context.Context, args InvokeActionArgs) (*InvokeActionResult, error)
	StartInvestigationFn  func(ctx context.Context, args StartInvestigationArgs) (*StartInvestigationResult, error)
	CompleteNoActionFn    func(ctx context.Context, args CompleteNoActionArgs) (*CompleteNoActionResult, error)
	Token                 string
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
