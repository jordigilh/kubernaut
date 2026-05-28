package ka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
)

// terminalActions are MCP actions that end an interactive session.
// After a successful terminal action, the pooled session is released.
var terminalActions = map[string]bool{
	"complete": true,
	"cancel":   true,
}

// PooledMCPClient implements MCPClient using KASessionPool for persistent
// MCP sessions. Sessions are keyed by (rr_id, username) and persist across
// multiple tool calls within the same interactive investigation (#1306).
//
// Terminal actions (complete, cancel) automatically release the pooled session.
// Non-terminal actions (takeover, message, status, discover_workflows) reuse
// the existing session or create a new one via the pool factory.
type PooledMCPClient struct {
	pool   *KASessionPool
	logger logr.Logger
}

// NewPooledMCPClient creates a PooledMCPClient backed by the given session pool.
func NewPooledMCPClient(pool *KASessionPool, logger logr.Logger) *PooledMCPClient {
	return &PooledMCPClient{
		pool:   pool,
		logger: logger.WithName("pooled-mcp"),
	}
}

// Investigate is not supported by pooled sessions — investigation uses KA REST,
// not MCP interactive protocol. Callers should use SDKMCPClient.Investigate.
func (c *PooledMCPClient) Investigate(_ context.Context, _ InvestigateArgs) (*InvestigateResult, error) {
	return nil, fmt.Errorf("Investigate is a REST operation; use SDKMCPClient for non-interactive calls")
}

// InvokeAction calls kubernaut_investigate with the given action via a pooled
// MCP session. Terminal actions (complete, cancel) release the session after
// a successful call.
func (c *PooledMCPClient) InvokeAction(ctx context.Context, args InvokeActionArgs) (*InvokeActionResult, error) {
	identity := auth.UserIdentityFromContext(ctx)
	if identity == nil {
		return nil, fmt.Errorf("user identity required: no identity in context")
	}

	session, err := c.pool.Acquire(ctx, args.RRID, identity.Username)
	if err != nil {
		return nil, fmt.Errorf("acquire MCP session: %w", err)
	}

	argsMap := map[string]any{
		"rr_id":              args.RRID,
		"action":             args.Action,
		"acting_user":        identity.Username,
		"acting_user_groups": identity.Groups,
	}
	if args.Message != "" {
		argsMap["message"] = args.Message
	}

	raw, err := c.callPooledTool(ctx, session, "kubernaut_investigate", argsMap)
	if err != nil {
		return nil, err
	}

	var result InvokeActionResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse invoke_action response: %w", err)
	}

	if terminalActions[args.Action] {
		c.pool.Release(args.RRID, identity.Username)
		c.logger.Info("pooled session released (terminal action)",
			"rr_id", args.RRID, "action", args.Action)
	}

	return &result, nil
}

// DiscoverWorkflows calls kubernaut_investigate with action "discover_workflows"
// via a pooled MCP session.
//
//nolint:gocritic // hugeParam: matches MCPClient interface contract
func (c *PooledMCPClient) DiscoverWorkflows(ctx context.Context, args DiscoverWorkflowsArgs) (*DiscoverWorkflowsResult, error) {
	identity := auth.UserIdentityFromContext(ctx)
	if identity == nil {
		return nil, fmt.Errorf("user identity required: no identity in context")
	}

	session, err := c.pool.Acquire(ctx, args.RRID, identity.Username)
	if err != nil {
		return nil, fmt.Errorf("acquire MCP session: %w", err)
	}

	argsMap := map[string]any{
		"rr_id":              args.RRID,
		"action":             "discover_workflows",
		"acting_user":        identity.Username,
		"acting_user_groups": identity.Groups,
	}

	raw, err := c.callPooledTool(ctx, session, "kubernaut_investigate", argsMap)
	if err != nil {
		return nil, err
	}

	var result DiscoverWorkflowsResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse discover_workflows response: %w", err)
	}
	return &result, nil
}

// SelectWorkflow calls kubernaut_select_workflow via a pooled MCP session.
//
//nolint:gocritic // hugeParam: matches MCPClient interface contract
func (c *PooledMCPClient) SelectWorkflow(ctx context.Context, args SelectWorkflowArgs) (*SelectWorkflowResult, error) {
	identity := auth.UserIdentityFromContext(ctx)
	if identity == nil {
		return nil, fmt.Errorf("user identity required: no identity in context")
	}

	session, err := c.pool.Acquire(ctx, args.RRID, identity.Username)
	if err != nil {
		return nil, fmt.Errorf("acquire MCP session: %w", err)
	}

	argsMap := map[string]any{
		"rr_id":              args.RRID,
		"workflow_id":        args.WorkflowID,
		"acting_user":        identity.Username,
		"acting_user_groups": identity.Groups,
	}
	if args.Kind != "" {
		argsMap["kind"] = args.Kind
	}
	if args.Name != "" {
		argsMap["name"] = args.Name
	}
	if args.Namespace != "" {
		argsMap["namespace"] = args.Namespace
	}
	if args.Parameters != nil {
		argsMap["parameters"] = args.Parameters
	}

	raw, err := c.callPooledTool(ctx, session, "kubernaut_select_workflow", argsMap)
	if err != nil {
		return nil, err
	}

	var result SelectWorkflowResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse select_workflow response: %w", err)
	}
	return &result, nil
}

// StartInvestigation is not supported by pooled sessions — dedicated investigations
// require a long-lived MCP session with LoggingMessageHandler for event streaming.
// Callers should use SDKMCPClient.StartInvestigation.
func (c *PooledMCPClient) StartInvestigation(_ context.Context, _ StartInvestigationArgs) (*StartInvestigationResult, error) {
	return nil, fmt.Errorf("StartInvestigation requires a dedicated MCP session; use SDKMCPClient")
}

// callPooledTool dispatches a tool call to the given pooled session, handling
// error response parsing and security redaction consistently with SDKMCPClient.
func (c *PooledMCPClient) callPooledTool(ctx context.Context, session PoolSession, name string, args map[string]any) (json.RawMessage, error) {
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return nil, fmt.Errorf("MCP call %s: %w", name, err)
	}

	if result.IsError {
		msg := "tool call returned error"
		if len(result.Content) > 0 {
			if tc, ok := result.Content[0].(*mcp.TextContent); ok {
				msg = security.RedactError(fmt.Errorf("%s", tc.Text))
			}
		}
		return nil, fmt.Errorf("kubernaut agent: %s", msg)
	}

	if len(result.Content) == 0 {
		return json.RawMessage("{}"), nil
	}

	if tc, ok := result.Content[0].(*mcp.TextContent); ok {
		return json.RawMessage(tc.Text), nil
	}

	return json.RawMessage("{}"), nil
}

// Compile-time interface check.
var _ MCPClient = (*PooledMCPClient)(nil)
