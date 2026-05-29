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
	"k8s.io/client-go/dynamic"

	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
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

// ISPhaseFinalizer updates the IS CRD phase after a terminal MCP action
// (complete/cancel). Implemented by session.CRDSessionService.
type ISPhaseFinalizer interface {
	FinalizeSessionByRR(ctx context.Context, rrNamespace, rrName string, phase isv1alpha1.SessionPhase) error
}

// ISSessionInitializer creates an IS CRD when a user explicitly takes over
// an investigation via kubernaut_takeover. Unlike MaterializeCRD (which uses
// deferred A2A state), this creates the CRD directly from MCP context.
// Implemented by session.CRDSessionService.
type ISSessionInitializer interface {
	InitializeSessionByRR(ctx context.Context, rrNamespace, rrName, kaSessionID, username string, groups []string) error
}

// MCPBridgeConfig holds the configuration for the real MCP tool bridge.
type MCPBridgeConfig struct {
	K8sClient             dynamic.Interface
	Namespace             string
	KAMCPClient           ka.MCPClient
	KADedicatedClient     ka.MCPClient
	InvestigationRegistry *tools.MonitorRegistry
	Pool               *ka.KASessionPool
	DSClient           ds.Client
	Triager            *severity.Triager
	Authorizer         auth.ToolAuthorizer
	Auditor            audit.Emitter
	Logger             logr.Logger
	Metrics            *MCPBridgeMetrics
	ToolTimeout        time.Duration
	ToolTimeouts       map[string]time.Duration
	MaxConcurrentTools int64
	UserLimiter        *ratelimit.UserLimiter
	SessionFinalizer    ISPhaseFinalizer
	SessionInitializer  ISSessionInitializer
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

// GetToolTimeoutFor returns the timeout for a specific tool, falling back
// to the global ToolTimeout if no per-tool override exists.
func (c *MCPBridgeConfig) GetToolTimeoutFor(toolName string) time.Duration {
	if c.ToolTimeouts != nil {
		if t, ok := c.ToolTimeouts[toolName]; ok && t > 0 {
			return t
		}
	}
	return c.GetToolTimeout()
}

// GetMaxConcurrentTools returns the configured max concurrency or the default.
func (c *MCPBridgeConfig) GetMaxConcurrentTools() int64 {
	if c.MaxConcurrentTools > 0 {
		return c.MaxConcurrentTools
	}
	return defaultMaxConcurrentTool
}

// RegisterTools registers all 22 MCP domain tools on the server with the real dispatch handlers.
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

	// K8s CRD tools (ADR-022: all use AF's ServiceAccount)
	registerTool(srv, cfg, sem, "kubernaut_list_remediations", "List active and recent remediations",
		func(ctx context.Context, args tools.ListRemediationsArgs) (any, error) {
			return tools.HandleListRemediations(ctx, cfg.K8sClient, args)
		})

	registerTool(srv, cfg, sem, "kubernaut_get_remediation", "Get details of a specific remediation",
		func(ctx context.Context, args tools.GetRemediationArgs) (any, error) {
			if args.Namespace == "" {
				args.Namespace = cfg.Namespace
			}
			return tools.HandleGetRemediation(ctx, cfg.K8sClient, args)
		})

	registerTool(srv, cfg, sem, "kubernaut_list_approval_requests", "List remediation approval requests with optional filtering by decision status",
		func(ctx context.Context, args tools.ListApprovalRequestsArgs) (any, error) {
			return tools.HandleListApprovalRequests(ctx, cfg.K8sClient, args)
		})

	registerTool(srv, cfg, sem, "kubernaut_get_approval_request", "Get full details of a specific remediation approval request",
		func(ctx context.Context, args tools.GetApprovalRequestArgs) (any, error) {
			return tools.HandleGetApprovalRequest(ctx, cfg.K8sClient, args)
		})

	registerTool(srv, cfg, sem, "kubernaut_approve", "Approve a remediation action",
		func(ctx context.Context, args tools.ApproveArgs) (any, error) {
			username := usernameFromCtx(ctx)
			return tools.HandleApprove(ctx, cfg.K8sClient, args, username)
		})

	registerTool(srv, cfg, sem, "kubernaut_cancel_remediation", "Cancel an active remediation",
		func(ctx context.Context, args tools.CancelRemediationArgs) (any, error) {
			if args.Namespace == "" {
				args.Namespace = cfg.Namespace
			}
			return tools.HandleCancelRemediation(ctx, cfg.K8sClient, args)
		})

	registerTool(srv, cfg, sem, "kubernaut_watch", "Watch for remediation state changes",
		func(ctx context.Context, args tools.WatchArgs) (any, error) {
			return tools.HandleWatch(ctx, cfg.K8sClient, args)
		})

	registerTool(srv, cfg, sem, "kubernaut_await_session", "Wait for KA investigation session to become ready",
		func(ctx context.Context, args tools.AwaitSessionArgs) (any, error) {
			return tools.HandleAwaitSession(ctx, cfg.K8sClient, args)
		})

	// KA investigation tool (MCP-only, non-blocking).
	// Uses KADedicatedClient (SDKMCPClient) which creates dedicated non-pooled
	// sessions for StartInvestigation. PooledMCPClient does not support StartInvestigation.
	dedicatedClient := cfg.KADedicatedClient
	if dedicatedClient == nil {
		dedicatedClient = cfg.KAMCPClient
	}
	onInvestigateStarted := buildSessionStartedHook(cfg)
	registerTool(srv, cfg, sem, "kubernaut_investigate", "Investigate an infrastructure incident",
		func(ctx context.Context, args tools.InvestigateMCPArgs) (any, error) {
			return tools.HandleInvestigationMCPWithRegistry(ctx, dedicatedClient, cfg.K8sClient, cfg.Namespace, args, cfg.Auditor, cfg.InvestigationRegistry, onInvestigateStarted, false)
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

	// Interactive investigation tools (G1: 4-phase journey)
	registerTool(srv, cfg, sem, "kubernaut_takeover", "Take over an existing investigation session",
		func(ctx context.Context, args tools.InteractiveActionArgs) (any, error) {
			result, err := tools.HandleTakeover(ctx, cfg.KAMCPClient, args, cfg.Auditor)
			if err != nil {
				return result, err
			}
			if cfg.SessionInitializer != nil {
				identity := auth.UserIdentityFromContext(ctx)
				if identity == nil {
					return result, nil
				}
				if iErr := cfg.SessionInitializer.InitializeSessionByRR(ctx, cfg.Namespace, args.RRID, result.SessionID, identity.Username, identity.Groups); iErr != nil {
					cfg.Logger.Error(iErr, "IS CRD initialization failed on takeover", "rr_id", args.RRID, "session_id", result.SessionID)
					return nil, fmt.Errorf("takeover succeeded but IS CRD creation failed: %w", iErr)
				}
			}
			return result, nil
		})

	registerTool(srv, cfg, sem, "kubernaut_message", "Send a message to an active investigation session",
		func(ctx context.Context, args tools.InteractiveActionArgs) (any, error) {
			return tools.HandleMessage(ctx, cfg.KAMCPClient, args, cfg.Auditor)
		})

	registerTool(srv, cfg, sem, "kubernaut_complete", "Complete an investigation session",
		func(ctx context.Context, args tools.InteractiveActionArgs) (any, error) {
			result, err := tools.HandleComplete(ctx, cfg.KAMCPClient, args, cfg.Auditor)
			if err == nil && cfg.SessionFinalizer != nil {
				if fErr := cfg.SessionFinalizer.FinalizeSessionByRR(ctx, cfg.Namespace, args.RRID, isv1alpha1.SessionPhaseCompleted); fErr != nil {
					cfg.Logger.Error(fErr, "IS CRD phase finalization failed", "rr_id", args.RRID, "phase", "Completed")
				}
			}
			return result, err
		})

	registerTool(srv, cfg, sem, "kubernaut_cancel", "Cancel an active investigation session",
		func(ctx context.Context, args tools.InteractiveActionArgs) (any, error) {
			result, err := tools.HandleCancel(ctx, cfg.KAMCPClient, args, cfg.Auditor)
			if err == nil && cfg.SessionFinalizer != nil {
				if fErr := cfg.SessionFinalizer.FinalizeSessionByRR(ctx, cfg.Namespace, args.RRID, isv1alpha1.SessionPhaseCancelled); fErr != nil {
					cfg.Logger.Error(fErr, "IS CRD phase finalization failed", "rr_id", args.RRID, "phase", "Cancelled")
				}
			}
			return result, err
		})

	registerTool(srv, cfg, sem, "kubernaut_status", "Get the current status of an investigation session",
		func(ctx context.Context, args tools.InteractiveActionArgs) (any, error) {
			return tools.HandleStatus(ctx, cfg.KAMCPClient, args, cfg.Auditor)
		})

	registerTool(srv, cfg, sem, "kubernaut_reconnect", "Reconnect to a disconnected investigation session",
		func(ctx context.Context, args tools.InteractiveActionArgs) (any, error) {
			return tools.HandleReconnect(ctx, cfg.KAMCPClient, args, cfg.Auditor)
		})

	// Stream investigation tool removed — merged into kubernaut_investigate above.

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
		toolCtx, cancel := context.WithTimeout(ctx, cfg.GetToolTimeoutFor(toolName))
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
			errDetail := map[string]string{"error": redacted}
			enrichAuditFromArgs(errDetail, input)
			emitAudit(ctx, cfg, toolName, audit.EventMCPToolFailed, errDetail)
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

		durationMs := fmt.Sprintf("%d", time.Since(start).Milliseconds())
		recordMetrics(cfg, toolName, resultLabel, start)
		auditDetail := map[string]string{
			"tool_outcome":          "success",
			"execution_duration_ms": durationMs,
		}
		enrichAuditFromArgs(auditDetail, input)
		emitAudit(ctx, cfg, toolName, audit.EventToolExecuted, auditDetail)
		cfg.Logger.Info("tool call succeeded",
			"tool", toolName,
			"user", usernameFromCtx(ctx),
			"duration_ms", durationMs,
		)

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

// enrichAuditFromArgs extracts known fields (session_id, rr_id, namespace) from tool
// args and adds them to the audit detail map. Uses JSON round-trip to inspect
// the generic In type without reflection, and also checks the AuditableInput interface.
func enrichAuditFromArgs[In any](detail map[string]string, input In) {
	if ai, ok := any(input).(tools.AuditableInput); ok {
		for k, v := range ai.AuditFields() {
			if v != "" {
				detail[k] = v
			}
		}
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return
	}
	for _, key := range []string{"session_id", "rr_id"} {
		if v, ok := fields[key]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil && s != "" {
				detail[key] = s
			}
		}
	}
}

func usernameFromCtx(ctx context.Context) string {
	if identity := auth.UserIdentityFromContext(ctx); identity != nil && identity.Username != "" {
		return identity.Username
	}
	return "system"
}

// buildSessionStartedHook returns a SessionStartedHook that creates an IS CRD
// after a successful StartInvestigation. Returns nil if no SessionInitializer
// is configured. Both the MCP bridge and A2A agent paths use this to consolidate
// IS CRD creation in a single place (HandleInvestigationMCPWithRegistry).
func buildSessionStartedHook(cfg *MCPBridgeConfig) tools.SessionStartedHook {
	if cfg.SessionInitializer == nil {
		return nil
	}
	return func(ctx context.Context, namespace, rrID, sessionID string) error {
		identity := auth.UserIdentityFromContext(ctx)
		if identity == nil {
			return nil
		}
		return cfg.SessionInitializer.InitializeSessionByRR(ctx, namespace, rrID, sessionID, identity.Username, identity.Groups)
	}
}
