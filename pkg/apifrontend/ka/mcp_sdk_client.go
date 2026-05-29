package ka

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
)

// SDKMCPClient implements MCPClient using the MCP Go SDK's StreamableClientTransport.
// AF authenticates to KA as itself using a SA bearer token injected by
// bearerTokenTransport (trusted intermediary model, DD-AUTH-MCP-001 v3.0).
// User identity is passed as acting_user/acting_user_groups in MCP args.
//
// Session-per-call overhead is acceptable for v1.5 volume (P2 Architect finding).
type SDKMCPClient struct {
	endpoint           string
	client             *mcp.Client
	httpClient         *http.Client
	logger             logr.Logger
	downstreamDuration *prometheus.HistogramVec
}

// NewSDKMCPClient creates a new MCP client for KA communication.
// The httpClient should include auth transport (e.g., bearerTokenTransport for SA token).
// WithDownstreamDuration injects the af_downstream_request_duration_seconds
// histogram for MCP call latency instrumentation (G18).
func (c *SDKMCPClient) WithDownstreamDuration(h *prometheus.HistogramVec) *SDKMCPClient {
	c.downstreamDuration = h
	return c
}

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
	identity := auth.UserIdentityFromContext(ctx)
	if identity == nil {
		return nil, fmt.Errorf("user identity required: no identity in context")
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
	start := time.Now()
	statusLabel := "2xx"

	defer func() {
		if c.downstreamDuration != nil {
			c.downstreamDuration.WithLabelValues("ka-mcp", statusLabel).Observe(time.Since(start).Seconds())
		}
	}()

	transport := &mcp.StreamableClientTransport{
		Endpoint:   c.endpoint,
		HTTPClient: c.httpClient,
	}

	session, err := c.client.Connect(ctx, transport, nil)
	if err != nil {
		statusLabel = "5xx"
		return nil, kaToUserFriendlyError(fmt.Errorf("MCP connect: %w", err))
	}
	defer func() { _ = session.Close() }()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		statusLabel = "5xx"
		return nil, kaToUserFriendlyError(fmt.Errorf("MCP call %s: %w", name, err))
	}

	if result.IsError {
		statusLabel = "4xx"
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

// DiscoverWorkflows calls kubernaut_investigate with action "discover_workflows"
// on KA's MCP server. KA exposes workflow discovery as an action within the
// kubernaut_investigate tool, not as a standalone tool.
//
//nolint:gocritic // hugeParam: matches MCPClient interface contract
func (c *SDKMCPClient) DiscoverWorkflows(ctx context.Context, args DiscoverWorkflowsArgs) (*DiscoverWorkflowsResult, error) {
	identity := auth.UserIdentityFromContext(ctx)
	if identity == nil {
		return nil, fmt.Errorf("user identity required: no identity in context")
	}

	argsMap := map[string]any{
		"rr_id":              args.RRID,
		"action":             "discover_workflows",
		"acting_user":        identity.Username,
		"acting_user_groups": identity.Groups,
	}

	result, err := c.callTool(ctx, "kubernaut_investigate", argsMap)
	if err != nil {
		return nil, err
	}

	return ParseDiscoverWorkflowsResponse(result)
}

// InvokeAction calls kubernaut_investigate with a specific action on KA's MCP server.
// The acting_user and acting_user_groups are extracted from ctx via
// auth.UserIdentityFromContext and injected into the MCP args map (ADR-022).
func (c *SDKMCPClient) InvokeAction(ctx context.Context, args InvokeActionArgs) (*InvokeActionResult, error) {
	identity := auth.UserIdentityFromContext(ctx)
	if identity == nil {
		return nil, fmt.Errorf("user identity required: no identity in context")
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

	result, err := c.callTool(ctx, "kubernaut_investigate", argsMap)
	if err != nil {
		return nil, err
	}

	var invResult InvokeActionResult
	if err := json.Unmarshal(result, &invResult); err != nil {
		return nil, fmt.Errorf("parse invoke_action response: %w", err)
	}
	return &invResult, nil
}

// StartInvestigation connects to KA MCP, sends action=start to launch the
// deferred investigation and acquire the interactive lease, and registers a
// LoggingMessageHandler to stream events back to the caller. The caller
// receives events on the returned channel and must call Closer when done.
func (c *SDKMCPClient) StartInvestigation(ctx context.Context, args StartInvestigationArgs) (*StartInvestigationResult, error) {
	identity := auth.UserIdentityFromContext(ctx)
	if identity == nil {
		return nil, fmt.Errorf("user identity required: no identity in context")
	}

	eventCh := make(chan InvestigationEvent, 64)
	doneCh := make(chan struct{})

	var eventsReceived int64
	streamClient := mcp.NewClient(&mcp.Implementation{
		Name:    "kubernaut-apifrontend-dedicated",
		Version: "0.1.0",
	}, &mcp.ClientOptions{
		LoggingMessageHandler: func(_ context.Context, req *mcp.LoggingMessageRequest) {
			eventsReceived++
			if eventsReceived == 1 {
				c.logger.Info("LoggingMessageHandler: first event received from KA",
					"rr_id", args.RRID, "level", string(req.Params.Level))
			}

			raw, err := json.Marshal(req.Params.Data)
			if err != nil {
				c.logger.Error(err, "failed to marshal logging message data")
				return
			}

			var evt InvestigationEvent
			if err := json.Unmarshal(raw, &evt); err != nil {
				c.logger.Error(err, "failed to parse logging message as InvestigationEvent")
				return
			}

			select {
			case <-doneCh:
				c.logger.Info("LoggingMessageHandler: doneCh closed, dropping event",
					"rr_id", args.RRID, "event_type", evt.Type, "total_received", eventsReceived)
				return
			default:
			}
			select {
			case eventCh <- evt:
			case <-doneCh:
			default:
				c.logger.Info("event channel full, dropping event", "event_type", evt.Type)
			}
		},
	})

	transport := &mcp.StreamableClientTransport{
		Endpoint:   c.endpoint,
		HTTPClient: c.httpClient,
	}

	// Use a detached context for the MCP session so its lifetime is not tied
	// to the request/tool timeout context. The investigation may run for
	// minutes; the caller controls cleanup via Closer(). We still use ctx
	// for the initial Connect and SetLoggingLevel calls (which must complete
	// within the request deadline), but the session itself lives beyond ctx.
	session, err := streamClient.Connect(context.Background(), transport, nil)
	if err != nil {
		close(doneCh)
		close(eventCh)
		return nil, fmt.Errorf("MCP connect for investigation: %w", err)
	}

	if err := session.SetLoggingLevel(context.Background(), &mcp.SetLoggingLevelParams{Level: "info"}); err != nil {
		_ = session.Close()
		close(doneCh)
		close(eventCh)
		return nil, fmt.Errorf("set logging level: %w", err)
	}

	argsMap := map[string]any{
		"rr_id":              args.RRID,
		"action":             "start",
		"acting_user":        identity.Username,
		"acting_user_groups": identity.Groups,
	}

	callCtx, callCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer callCancel()
	result, err := session.CallTool(callCtx, &mcp.CallToolParams{
		Name:      "kubernaut_investigate",
		Arguments: argsMap,
	})
	if err != nil {
		_ = session.Close()
		close(doneCh)
		close(eventCh)
		return nil, fmt.Errorf("start investigation MCP call: %w", err)
	}

	if result.IsError {
		msg := "KA tool error"
		if len(result.Content) > 0 {
			if tc, ok := result.Content[0].(*mcp.TextContent); ok {
				msg = tc.Text
			}
		}
		_ = session.Close()
		close(eventCh)
		return nil, fmt.Errorf("kubernaut_investigate start_autonomous: %s", msg)
	}

	var invResult struct {
		SessionID string `json:"session_id"`
		Status    string `json:"status"`
	}
	if len(result.Content) > 0 {
		if tc, ok := result.Content[0].(*mcp.TextContent); ok {
			_ = json.Unmarshal([]byte(tc.Text), &invResult)
		}
	}

	var once sync.Once
	closer := func() {
		once.Do(func() {
			close(doneCh)
			_ = session.Close()
			close(eventCh)
		})
	}

	return &StartInvestigationResult{
		SessionID: invResult.SessionID,
		Status:    invResult.Status,
		Events:    eventCh,
		Closer:    closer,
	}, nil
}

// ConnectSession establishes a new MCP session without auto-closing it.
// Used by the KASessionPool factory to create persistent sessions (#1306).
func (c *SDKMCPClient) ConnectSession(ctx context.Context, transport *mcp.StreamableClientTransport) (PoolSession, error) {
	session, err := c.client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("MCP connect: %w", err)
	}
	return session, nil
}

// Compile-time interface check.
var _ MCPClient = (*SDKMCPClient)(nil)
