package ka

import "context"

// MCPClient is the interface for KA MCP operations.
type MCPClient interface {
	SelectWorkflow(ctx context.Context, args SelectWorkflowArgs) (*SelectWorkflowResult, error)
	Investigate(ctx context.Context, args InvestigateArgs) (*InvestigateResult, error)
}

// MockMCPClient is a test double for MCPClient.
type MockMCPClient struct {
	SelectWorkflowFn func(ctx context.Context, args SelectWorkflowArgs) (*SelectWorkflowResult, error)
	InvestigateFn    func(ctx context.Context, args InvestigateArgs) (*InvestigateResult, error)
	Token            string
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
