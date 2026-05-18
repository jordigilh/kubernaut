package ka

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jordigilh/kubernaut-apifrontend/internal/security"
)

// SDKMCPClient implements MCPClient using the MCP Go SDK's StreamableClientTransport.
// Each call creates a session-per-call to forward per-request JWT credentials
// via the HTTPClient (Pattern B JWT delegation, DD-AUTH-MCP-001 v2.0).
//
// Session-per-call overhead is acceptable for v1.5 volume (P2 Architect finding).
type SDKMCPClient struct {
	endpoint   string
	client     *mcp.Client
	httpClient *http.Client
	logger     logr.Logger
}

// NewSDKMCPClient creates a new MCP client for KA communication.
// The httpClient should include auth transport (e.g., ContextJWTDelegationTransport).
func NewSDKMCPClient(endpoint string, httpClient *http.Client, logger logr.Logger) *SDKMCPClient {
	mcpClient := mcp.NewClient(&mcp.Implementation{
		Name:    "kubernaut-apifrontend",
		Version: "0.1.0",
	}, nil)

	return &SDKMCPClient{
		endpoint:   endpoint,
		client:     mcpClient,
		httpClient: httpClient,
		logger:     logger.WithName("ka-mcp"),
	}
}

// SelectWorkflow calls kubernaut_select_workflow on KA's MCP server.
//
//nolint:gocritic // hugeParam: matches MCPClient interface contract
func (c *SDKMCPClient) SelectWorkflow(ctx context.Context, args SelectWorkflowArgs) (*SelectWorkflowResult, error) {
	argsMap := map[string]any{
		"rr_id":       args.RRID,
		"workflow_id": args.WorkflowID,
	}

	result, err := c.callTool(ctx, "kubernaut_select_workflow", argsMap)
	if err != nil {
		return nil, err
	}

	var swResult SelectWorkflowResult
	if err := json.Unmarshal(result, &swResult); err != nil {
		return nil, fmt.Errorf("parse select_workflow response: %w", err)
	}
	return &swResult, nil
}

// Investigate calls kubernaut_investigate on KA's MCP server.
//
//nolint:gocritic // hugeParam: matches MCPClient interface contract
func (c *SDKMCPClient) Investigate(ctx context.Context, args InvestigateArgs) (*InvestigateResult, error) {
	argsMap := map[string]any{
		"namespace": args.Namespace,
		"kind":      args.Kind,
		"name":      args.Name,
	}

	result, err := c.callTool(ctx, "kubernaut_investigate", argsMap)
	if err != nil {
		return nil, err
	}

	var invResult InvestigateResult
	if err := json.Unmarshal(result, &invResult); err != nil {
		return nil, fmt.Errorf("parse investigate response: %w", err)
	}
	return &invResult, nil
}

func (c *SDKMCPClient) callTool(ctx context.Context, name string, args map[string]any) (json.RawMessage, error) {
	transport := &mcp.StreamableClientTransport{
		Endpoint:   c.endpoint,
		HTTPClient: c.httpClient,
	}

	session, err := c.client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, kaToUserFriendlyError(fmt.Errorf("MCP connect: %w", err))
	}
	defer func() { _ = session.Close() }()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return nil, kaToUserFriendlyError(fmt.Errorf("MCP call %s: %w", name, err))
	}

	if result.IsError {
		msg := "tool call returned error"
		if len(result.Content) > 0 {
			if textContent, ok := result.Content[0].(*mcp.TextContent); ok {
				msg = security.RedactError(fmt.Errorf("%s", textContent.Text))
			}
		}
		return nil, fmt.Errorf("kubernaut agent: %s", msg)
	}

	if len(result.Content) == 0 {
		return json.RawMessage("{}"), nil
	}

	if textContent, ok := result.Content[0].(*mcp.TextContent); ok {
		return json.RawMessage(textContent.Text), nil
	}

	return json.RawMessage("{}"), nil
}

// Compile-time interface check.
var _ MCPClient = (*SDKMCPClient)(nil)
