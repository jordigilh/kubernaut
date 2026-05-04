package ka

import "context"

// MCPClient is the interface for KA MCP operations.
// Real implementation using github.com/modelcontextprotocol/go-sdk is deferred
// to PR6; this PR validates the tool handler logic against the mock.
type MCPClient interface {
	SelectWorkflow(ctx context.Context, args SelectWorkflowArgs) (*SelectWorkflowResult, error)
}

// MockMCPClient is a test double for MCPClient.
type MockMCPClient struct {
	SelectWorkflowFn func(ctx context.Context, args SelectWorkflowArgs) (*SelectWorkflowResult, error)
	Token            string
}

// SelectWorkflow calls the mock function.
//
//nolint:gocritic // hugeParam: matches MCPClient interface contract
func (m *MockMCPClient) SelectWorkflow(ctx context.Context, args SelectWorkflowArgs) (*SelectWorkflowResult, error) {
	return m.SelectWorkflowFn(ctx, args)
}
