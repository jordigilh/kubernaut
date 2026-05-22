package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/semaphore"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ds"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ratelimit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

const (
	defaultToolTimeout       = 30 * time.Second
	defaultMaxConcurrentTool = 10
)

// MCPBridgeConfig holds the configuration for the real MCP tool bridge.
type MCPBridgeConfig struct {
	DynFactory         auth.DynamicClientFactory
	KAClient           *ka.Client
	KAMCPClient        ka.MCPClient
	DSClient           ds.Client
	Triager            *severity.Triager
	Authorizer         auth.ToolAuthorizer
	Auditor            audit.Emitter
	Logger             logr.Logger
	Metrics            *MCPBridgeMetrics
	ToolTimeout        time.Duration
	MaxConcurrentTools int64
	UserLimiter        *ratelimit.UserLimiter
}

// MCPBridgeMetrics holds Prometheus collectors specific to MCP bridge operations.
type MCPBridgeMetrics struct {
	ToolCallsTotal   *prometheus.CounterVec
	ToolCallDuration *prometheus.HistogramVec
}

// GetToolTimeout returns the configured tool timeout or the default.
func (c *MCPBridgeConfig) GetToolTimeout() time.Duration {
	if c.ToolTimeout > 0 {
		return c.ToolTimeout
	}
	return defaultToolTimeout
}

// GetMaxConcurrentTools returns the configured max concurrency or the default.
func (c *MCPBridgeConfig) GetMaxConcurrentTools() int64 {
	if c.MaxConcurrentTools > 0 {
		return c.MaxConcurrentTools
	}
	return defaultMaxConcurrentTool
}

// RegisterTools registers all 14 MCP domain tools on the server with the real dispatch handlers.
func RegisterTools(srv *mcp.Server, cfg *MCPBridgeConfig) {
	if cfg == nil {
		panic("RegisterTools: cfg must not be nil")
	}
	if cfg.Authorizer == nil {
		panic("RegisterTools: Authorizer must not be nil")
	}
	if cfg.Logger.GetSink() == nil {
		cfg.Logger = logr.Discard()
	}
	sem := semaphore.NewWeighted(cfg.GetMaxConcurrentTools())

	// K8s CRD tools (use DynFactory for impersonated clients)
	registerTool(srv, cfg, sem, "kubernaut_list_remediations", "List active and recent remediations",
		func(ctx context.Context, args tools.ListRemediationsArgs) (any, error) {
			client, err := cfg.DynFactory(ctx)
			if err != nil {
				return nil, err
			}
			return tools.HandleListRemediations(ctx, client, args)
		})

	registerTool(srv, cfg, sem, "kubernaut_get_remediation", "Get details of a specific remediation",
		func(ctx context.Context, args tools.GetRemediationArgs) (any, error) {
			client, err := cfg.DynFactory(ctx)
			if err != nil {
				return nil, err
			}
			return tools.HandleGetRemediation(ctx, client, args)
		})

	registerTool(srv, cfg, sem, "kubernaut_approve", "Approve a remediation action",
		func(ctx context.Context, args tools.ApproveArgs) (any, error) {
			client, err := cfg.DynFactory(ctx)
			if err != nil {
				return nil, err
			}
			username := usernameFromCtx(ctx)
			return tools.HandleApprove(ctx, client, args, username)
		})

	registerTool(srv, cfg, sem, "kubernaut_cancel_remediation", "Cancel an active remediation",
		func(ctx context.Context, args tools.CancelRemediationArgs) (any, error) {
			client, err := cfg.DynFactory(ctx)
			if err != nil {
				return nil, err
			}
			return tools.HandleCancelRemediation(ctx, client, args)
		})

	registerTool(srv, cfg, sem, "kubernaut_watch", "Watch for remediation state changes",
		func(ctx context.Context, args tools.WatchArgs) (any, error) {
			client, err := cfg.DynFactory(ctx)
			if err != nil {
				return nil, err
			}
			return tools.HandleWatch(ctx, client, args)
		})

	// KA REST tools
	registerTool(srv, cfg, sem, "kubernaut_start_investigation", "Start a new investigation session",
		func(ctx context.Context, args tools.StartInvestigationArgs) (any, error) {
			return tools.HandleStartInvestigation(ctx, cfg.KAClient, args, cfg.Auditor)
		})

	registerTool(srv, cfg, sem, "kubernaut_poll_investigation", "Poll an investigation session for updates",
		func(ctx context.Context, args tools.PollInvestigationArgs) (any, error) {
			return tools.HandlePollInvestigation(ctx, cfg.KAClient, args, 5, 3*time.Second, cfg.Auditor)
		})

	// KA MCP tools
	registerTool(srv, cfg, sem, "kubernaut_select_workflow", "Select a workflow for an investigation",
		func(ctx context.Context, args tools.SelectWorkflowArgs) (any, error) {
			return tools.HandleSelectWorkflow(ctx, cfg.KAMCPClient, args, cfg.Auditor)
		})

	registerTool(srv, cfg, sem, "kubernaut_discover_workflows", "Discover available workflows with parameter schemas",
		func(ctx context.Context, args tools.DiscoverWorkflowsArgs) (any, error) {
			result, err := tools.HandleDiscoverWorkflows(ctx, cfg.KAMCPClient, args)
			if err != nil {
				return result, err
			}
			emitAudit(ctx, cfg, "kubernaut_discover_workflows", audit.EventWorkflowDiscovery,
				map[string]string{"workflow_count": strconv.Itoa(result.Count)})
			return result, nil
		})

	// Presentation tool (no backend dependency)
	registerTool(srv, cfg, sem, "kubernaut_present_decision", "Present a decision point requiring user input",
		func(_ context.Context, args tools.PresentDecisionArgs) (any, error) {
			return tools.HandlePresentDecision(args), nil
		})

	// DS tools
	registerTool(srv, cfg, sem, "kubernaut_list_workflows", "List available workflows",
		func(ctx context.Context, args tools.ListWorkflowsArgs) (any, error) {
			if cfg.DSClient == nil {
				return nil, fmt.Errorf("datastorage service unavailable")
			}
			return tools.HandleListWorkflows(ctx, cfg.DSClient, args)
		})

	registerTool(srv, cfg, sem, "kubernaut_get_remediation_history", "Get remediation execution history",
		func(ctx context.Context, args tools.GetRemediationHistoryArgs) (any, error) {
			if cfg.DSClient == nil {
				return nil, fmt.Errorf("datastorage service unavailable")
			}
			return tools.HandleGetRemediationHistory(ctx, cfg.DSClient, args)
		})

	registerTool(srv, cfg, sem, "kubernaut_get_effectiveness", "Get remediation effectiveness metrics",
		func(ctx context.Context, args tools.GetEffectivenessArgs) (any, error) {
			if cfg.DSClient == nil {
				return nil, fmt.Errorf("datastorage service unavailable")
			}
			return tools.HandleGetEffectiveness(ctx, cfg.DSClient, args)
		})

	registerTool(srv, cfg, sem, "kubernaut_get_audit_trail", "Get audit trail for remediations",
		func(ctx context.Context, args tools.GetAuditTrailArgs) (any, error) {
			if cfg.DSClient == nil {
				return nil, fmt.Errorf("datastorage service unavailable")
			}
			return tools.HandleGetAuditTrail(ctx, cfg.DSClient, args)
		})

	// Internal triage tools (kubectl_get, kubectl_list, kubectl_list_events,
	// af_check_existing_rr, af_create_rr) are available only to AF's LLM
	// agent (ADK path) and are not exposed via MCP.
}

// registerTool is a generic helper that registers a single tool with all cross-cutting concerns:
// RBAC enforcement, timeout, semaphore concurrency limiting, metrics, audit, and error redaction.
// Uses the generic mcp.AddTool to auto-generate InputSchema from the In struct.
func registerTool[In any](srv *mcp.Server, cfg *MCPBridgeConfig, sem *semaphore.Weighted, name, description string, handler func(context.Context, In) (any, error)) {
	mcp.AddTool(srv, &mcp.Tool{Name: name, Description: description},
		wrapTool(cfg, sem, name, handler),
	)
}

// wrapTool applies cross-cutting middleware to a tool handler:
// 1. RBAC check
// 2. Semaphore acquisition
// 3. Timeout enforcement
// 4. Panic recovery
// 5. Metrics and audit emission
// 6. Error redaction
//
// Returns a mcp.ToolHandlerFor compatible with the generic mcp.AddTool.
func wrapTool[In any](cfg *MCPBridgeConfig, sem *semaphore.Weighted, toolName string, handler func(context.Context, In) (any, error)) mcp.ToolHandlerFor[In, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input In) (toolResult *mcp.CallToolResult, extra any, retErr error) {
		start := time.Now()
		resultLabel := "success"

		defer func() {
			if r := recover(); r != nil {
				resultLabel = "panic"
				recordMetrics(cfg, toolName, resultLabel, start)
				emitAudit(ctx, cfg, toolName, audit.EventMCPToolFailed, map[string]string{"error": "internal error"})
				cfg.Logger.Error(fmt.Errorf("panic: %v", r), "tool handler panicked",
					"tool", toolName, "user", usernameFromCtx(ctx))
				toolResult = &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "internal error"}},
					IsError: true,
				}
			}
		}()

		// RBAC enforcement at runtime
		if err := checkRBAC(ctx, cfg, toolName); err != nil {
			resultLabel = "denied"
			recordMetrics(cfg, toolName, resultLabel, start)
			emitAudit(ctx, cfg, toolName, audit.EventAuthAccessDenied, nil)
			cfg.Logger.Info("tool call denied by RBAC",
				"tool", toolName, "user", usernameFromCtx(ctx))
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil, nil
		}

		// Per-user tool call rate limiting
		if cfg.UserLimiter != nil {
			username := usernameFromCtx(ctx)
			if !cfg.UserLimiter.AllowToolCall(username) {
				resultLabel = "rate_limited"
				recordMetrics(cfg, toolName, resultLabel, start)
				emitAudit(ctx, cfg, toolName, audit.EventMCPToolFailed, map[string]string{"error": "rate_limited"})
				cfg.Logger.Info("tool call rate limited",
					"tool", toolName, "user", username)
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "rate limit exceeded — too many tool calls per minute, please retry later"}},
					IsError: true,
				}, nil, nil
			}
		}

		// Timeout enforcement — covers semaphore wait + tool execution
		toolCtx, cancel := context.WithTimeout(ctx, cfg.GetToolTimeout())
		defer cancel()

		// Semaphore for per-session concurrency limiting
		if err := sem.Acquire(toolCtx, 1); err != nil {
			resultLabel = "throttled"
			recordMetrics(cfg, toolName, resultLabel, start)
			emitAudit(ctx, cfg, toolName, audit.EventMCPToolFailed, map[string]string{"error": "throttled"})
			cfg.Logger.Info("tool call throttled",
				"tool", toolName, "user", usernameFromCtx(ctx))
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "server busy — too many concurrent tool calls, please retry"}},
				IsError: true,
			}, nil, nil
		}
		defer sem.Release(1)

		// Execute handler
		result, err := handler(toolCtx, input)
		if err != nil {
			if toolCtx.Err() != nil {
				resultLabel = "timeout"
			} else {
				resultLabel = "error"
			}
			recordMetrics(cfg, toolName, resultLabel, start)
			redacted := security.RedactError(err)
			emitAudit(ctx, cfg, toolName, audit.EventMCPToolFailed, map[string]string{"error": redacted})
			cfg.Logger.Error(err, "tool call failed",
				"tool", toolName, "result", resultLabel, "user", usernameFromCtx(ctx))
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: redacted}},
				IsError: true,
			}, nil, nil
		}

		// Marshal result to JSON text
		resultJSON, err := json.Marshal(result)
		if err != nil {
			resultLabel = "error"
			recordMetrics(cfg, toolName, resultLabel, start)
			emitAudit(ctx, cfg, toolName, audit.EventMCPToolFailed, map[string]string{"error": "marshal failure"})
			cfg.Logger.Error(err, "tool result marshal failed",
				"tool", toolName, "user", usernameFromCtx(ctx))
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "internal error: failed to marshal result"}},
				IsError: true,
			}, nil, nil
		}

		recordMetrics(cfg, toolName, resultLabel, start)
		emitAudit(ctx, cfg, toolName, audit.EventToolExecuted, map[string]string{"tool_outcome": "success"})

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(resultJSON)}},
		}, nil, nil
	}
}

// checkRBAC verifies the user has permission to invoke the named tool via SAR.
// Returns nil if allowed, or an error describing the denial.
func checkRBAC(ctx context.Context, cfg *MCPBridgeConfig, toolName string) error {
	user := auth.UserIdentityFromContext(ctx)
	if user == nil {
		return fmt.Errorf("permission denied: authentication required to invoke %s", toolName)
	}

	if cfg.Authorizer == nil {
		return fmt.Errorf("permission denied: no authorizer configured")
	}

	allowed, err := cfg.Authorizer.Check(ctx, user.Username, user.Groups, toolName)
	if err != nil {
		return fmt.Errorf("permission denied: authorization check failed for %s: %w", toolName, err)
	}
	if !allowed {
		return fmt.Errorf("permission denied: role lacks access to %s", toolName)
	}
	return nil
}

func recordMetrics(cfg *MCPBridgeConfig, toolName, result string, start time.Time) {
	if cfg.Metrics == nil {
		return
	}
	duration := time.Since(start).Seconds()
	if cfg.Metrics.ToolCallsTotal != nil {
		cfg.Metrics.ToolCallsTotal.With(prometheus.Labels{"tool": toolName, "result": result}).Inc()
	}
	if cfg.Metrics.ToolCallDuration != nil {
		cfg.Metrics.ToolCallDuration.With(prometheus.Labels{"tool": toolName, "type": "mcp"}).Observe(duration)
	}
}

func emitAudit(ctx context.Context, cfg *MCPBridgeConfig, toolName string, eventType audit.EventType, extra map[string]string) {
	if cfg.Auditor == nil {
		return
	}
	username := ""
	if user := auth.UserIdentityFromContext(ctx); user != nil {
		username = user.Username
	}
	detail := map[string]string{"tool_name": toolName}
	for k, v := range extra {
		detail[k] = v
	}
	cfg.Auditor.Emit(ctx, &audit.Event{
		Timestamp: time.Now(),
		Type:      eventType,
		UserID:    username,
		Detail:    detail,
	})
}

func usernameFromCtx(ctx context.Context) string {
	if identity := auth.UserIdentityFromContext(ctx); identity != nil && identity.Username != "" {
		return identity.Username
	}
	return "system"
}
