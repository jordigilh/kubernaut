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
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ds"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ratelimit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
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

// ISSessionInitializer creates an IS CRD for interactive investigation flows.
// Implemented by session.CRDSessionService.
type ISSessionInitializer interface {
	InitializeSessionByRR(ctx context.Context, rrNamespace, rrName, kaSessionID, username string, groups []string) error
	CreateInvestigationSession(ctx context.Context, cfg session.CreateISConfig) (string, error)
	UpdateISCorrelation(ctx context.Context, crdName, kaSessionID string) error
}

// MCPBridgeConfig holds the configuration for the real MCP tool bridge.
type MCPBridgeConfig struct {
	// K8sClient is the dynamic K8s client. Not used by the bridge directly
	// (kubernaut CRDs use TypedClient); retained for test compatibility and
	// future kubectl/events tool registration if moved from the agent path.
	K8sClient             dynamic.Interface
	TypedClient           crclient.WithWatch
	Namespace             string
	KAMCPClient           ka.MCPClient
	KADedicatedClient     ka.MCPClient
	InvestigationRegistry *tools.MonitorRegistry
	DSClient              ds.Client
	PromClient            prom.Client
	Triager               *severity.Triager
	Authorizer            auth.ToolAuthorizer
	Auditor               audit.Emitter
	Logger                logr.Logger
	Metrics               *MCPBridgeMetrics
	ToolTimeout           time.Duration
	ToolTimeouts          map[string]time.Duration
	MaxConcurrentTools    int64
	UserLimiter           *ratelimit.UserLimiter
	SessionFinalizer      ISPhaseFinalizer
	SessionInitializer    ISSessionInitializer
	// ActiveContextRegistry enables clearing the per-user session mapping when
	// a terminal MCP tool (e.g. kubernaut_complete_no_action) succeeds (#1496).
	// When non-nil, the CNA handler calls Clear(username) on success.
	ActiveContextRegistry *launcher.ActiveContextRegistry
	// InteractiveEnabled controls whether session-dependent tools are registered.
	// When false, tools in tools.SessionDependentTools are skipped (#1366).
	InteractiveEnabled bool
	// RESTMapper resolves Kind strings to GVR for scope-aware namespace stripping.
	// If nil, falls back to the static clusterScopedKinds map.
	RESTMapper meta.RESTMapper
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

// toolGate reports whether a session-dependent tool should be registered,
// given cfg.InteractiveEnabled (#1366).
type toolGate func(name string) bool

// RegisterTools registers all MCP domain tools on the server with the real dispatch handlers.
// When cfg.InteractiveEnabled is false, session-dependent tools (those in
// tools.SessionDependentTools) are skipped (#1366).
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
	shouldRegister := newToolGate(cfg)

	registerCRDTools(srv, cfg, sem, shouldRegister)
	registerInvestigationTool(srv, cfg, sem, shouldRegister)
	registerKAMCPTools(srv, cfg, sem, shouldRegister)
	registerDSTools(srv, cfg, sem)
	registerInteractiveTools(srv, cfg, sem, shouldRegister)
	registerAlertTools(srv, cfg, sem)
}

// newToolGate builds the shouldRegister predicate used to skip session-dependent
// tools (those in tools.SessionDependentTools) when interactive mode is disabled.
func newToolGate(cfg *MCPBridgeConfig) toolGate {
	return func(name string) bool {
		if cfg.InteractiveEnabled {
			return true
		}
		if tools.SessionDependentTools[name] {
			cfg.Logger.Info("skipping session-dependent tool (interactive disabled)", "tool", name)
			return false
		}
		return true
	}
}

// registerCRDTools registers the K8s CRD tools (ADR-022: all use AF's ServiceAccount).
// Namespace is always injected server-side — never exposed to LLM.
func registerCRDTools(srv *mcp.Server, cfg *MCPBridgeConfig, sem *semaphore.Weighted, shouldRegister toolGate) {
	registerTool(srv, cfg, sem, "kubernaut_list_remediations", "List active and recent remediations",
		func(ctx context.Context, args tools.ListRemediationsArgs) (any, error) {
			args.Namespace = cfg.Namespace
			return tools.HandleListRemediations(ctx, cfg.TypedClient, args)
		})

	registerTool(srv, cfg, sem, "kubernaut_get_remediation", "Get details of a specific remediation",
		func(ctx context.Context, args tools.GetRemediationArgs) (any, error) {
			args.Namespace = cfg.Namespace
			return tools.HandleGetRemediation(ctx, cfg.TypedClient, args)
		})

	registerTool(srv, cfg, sem, "kubernaut_list_approval_requests", "List remediation approval requests with optional filtering by decision status",
		func(ctx context.Context, args tools.ListApprovalRequestsArgs) (any, error) {
			args.Namespace = cfg.Namespace
			return tools.HandleListApprovalRequests(ctx, cfg.TypedClient, args)
		})

	registerTool(srv, cfg, sem, "kubernaut_get_approval_request", "Get full details of a specific remediation approval request",
		func(ctx context.Context, args tools.GetApprovalRequestArgs) (any, error) {
			args.Namespace = cfg.Namespace
			return tools.HandleGetApprovalRequest(ctx, cfg.TypedClient, args)
		})

	registerTool(srv, cfg, sem, "kubernaut_approve", "Approve a remediation action",
		func(ctx context.Context, args tools.ApproveArgs) (any, error) {
			args.Namespace = cfg.Namespace
			username := usernameFromCtx(ctx)
			return tools.HandleApprove(ctx, cfg.TypedClient, args, username)
		})

	registerTool(srv, cfg, sem, "kubernaut_cancel_remediation", "Cancel an active remediation",
		func(ctx context.Context, args tools.CancelRemediationArgs) (any, error) {
			args.Namespace = cfg.Namespace
			return tools.HandleCancelRemediation(ctx, cfg.TypedClient, args)
		})

	registerTool(srv, cfg, sem, "kubernaut_watch", "Watch for remediation state changes",
		func(ctx context.Context, args tools.WatchArgs) (any, error) {
			args.Namespace = cfg.Namespace
			return tools.HandleWatch(ctx, cfg.TypedClient, args)
		})

	if shouldRegister("kubernaut_await_session") {
		registerTool(srv, cfg, sem, "kubernaut_await_session", "Wait for KA investigation session to become ready",
			func(ctx context.Context, args tools.AwaitSessionArgs) (any, error) {
				args.Namespace = cfg.Namespace
				return tools.HandleAwaitSession(ctx, cfg.TypedClient, args)
			})
	}
}

// registerInvestigationTool registers the KA investigation tool (MCP-only, non-blocking).
// Uses KADedicatedClient (SDKMCPClient) which creates dedicated non-pooled
// sessions for StartInvestigation. PooledMCPClient does not support StartInvestigation.
func registerInvestigationTool(srv *mcp.Server, cfg *MCPBridgeConfig, sem *semaphore.Weighted, shouldRegister toolGate) {
	if !shouldRegister("kubernaut_investigate") {
		return
	}
	dedicatedClient := cfg.KADedicatedClient
	if dedicatedClient == nil {
		dedicatedClient = cfg.KAMCPClient
	}
	isSignaler := buildISSignaler(cfg)
	var onInvestigateStarted tools.SessionStartedHook
	if isSignaler == nil {
		onInvestigateStarted = buildSessionStartedHook(cfg)
	}
	registerTool(srv, cfg, sem, "kubernaut_investigate", "Investigate an infrastructure incident",
		func(ctx context.Context, args tools.InvestigateMCPArgs) (any, error) {
			ctx = tools.ContextWithRESTMapper(ctx, cfg.RESTMapper)
			return tools.HandleInvestigationMCPWithRegistry(ctx, &tools.InvestigateConfig{
				MCPClient: dedicatedClient,
				Client:    cfg.TypedClient,
				Namespace: cfg.Namespace,
				Auditor:   cfg.Auditor,
				Registry:  cfg.InvestigationRegistry,
				OnStarted: onInvestigateStarted,
				Signaler:  isSignaler,
				Triager:   cfg.Triager,
			}, args, false, "")
		})
}

// registerKAMCPTools registers the workflow-selection/discovery/decision KA MCP tools.
func registerKAMCPTools(srv *mcp.Server, cfg *MCPBridgeConfig, sem *semaphore.Weighted, shouldRegister toolGate) {
	if shouldRegister("kubernaut_select_workflow") {
		registerTool(srv, cfg, sem, "kubernaut_select_workflow", "Select a workflow for an investigation",
			func(ctx context.Context, args tools.SelectWorkflowArgs) (any, error) {
				return tools.HandleSelectWorkflow(ctx, cfg.KAMCPClient, args, cfg.Auditor)
			})
	}

	if shouldRegister("kubernaut_discover_workflows") {
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
	}

	if shouldRegister("kubernaut_present_decision") {
		registerTool(srv, cfg, sem, "kubernaut_present_decision", "Present a decision point requiring user input",
			func(_ context.Context, args tools.PresentDecisionArgs) (any, error) {
				return tools.HandlePresentDecision(args), nil
			})
	}
}

// registerDSTools registers the DataStorage-backed read-only query tools.
// These are always registered regardless of InteractiveEnabled.
func registerDSTools(srv *mcp.Server, cfg *MCPBridgeConfig, sem *semaphore.Weighted) {
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
}

// registerInteractiveTools registers the interactive investigation tools (G1: 4-phase journey).
//
// Internal triage tools (kubectl_get, kubectl_list, kubectl_list_events,
// kubernaut_check_existing_remediation, kubernaut_remediate) are available
// only to AF's LLM agent (ADK path) and are not exposed via MCP.
// Stream investigation tool removed — merged into kubernaut_investigate.
func registerInteractiveTools(srv *mcp.Server, cfg *MCPBridgeConfig, sem *semaphore.Weighted, shouldRegister toolGate) {
	if shouldRegister("kubernaut_message") {
		registerTool(srv, cfg, sem, "kubernaut_message", "Send a message to an active investigation session",
			func(ctx context.Context, args tools.InteractiveActionArgs) (any, error) {
				return tools.HandleMessage(ctx, cfg.KAMCPClient, args, cfg.Auditor)
			})
	}

	if shouldRegister("kubernaut_complete") {
		registerTool(srv, cfg, sem, "kubernaut_complete", "Complete an investigation session",
			func(ctx context.Context, args tools.InteractiveActionArgs) (any, error) {
				result, err := tools.HandleComplete(ctx, cfg.KAMCPClient, args, cfg.Auditor)
				finalizeSessionPhase(ctx, cfg, args.RRID, isv1alpha1.SessionPhaseCompleted, err)
				return result, err
			})
	}

	if shouldRegister("kubernaut_cancel") {
		registerTool(srv, cfg, sem, "kubernaut_cancel", "Cancel an active investigation session",
			func(ctx context.Context, args tools.InteractiveActionArgs) (any, error) {
				result, err := tools.HandleCancel(ctx, cfg.KAMCPClient, args, cfg.Auditor)
				finalizeSessionPhase(ctx, cfg, args.RRID, isv1alpha1.SessionPhaseCancelled, err)
				return result, err
			})
	}

	if shouldRegister("kubernaut_complete_no_action") {
		registerTool(srv, cfg, sem, "kubernaut_complete_no_action", "Complete an investigation without selecting a workflow — dismiss or escalate to a human team",
			func(ctx context.Context, args tools.CompleteNoActionArgs) (any, error) {
				result, err := tools.HandleCompleteNoAction(ctx, cfg.KAMCPClient, args, cfg.Auditor)
				finalizeSessionPhase(ctx, cfg, args.RRID, isv1alpha1.SessionPhaseCompleted, err)
				if err == nil && cfg.ActiveContextRegistry != nil {
					if username := usernameFromCtx(ctx); username != "system" {
						cfg.ActiveContextRegistry.Clear(username)
					}
				}
				return result, err
			})
	}

	if shouldRegister("kubernaut_status") {
		registerTool(srv, cfg, sem, "kubernaut_status", "Get the current status of an investigation session",
			func(ctx context.Context, args tools.InteractiveActionArgs) (any, error) {
				return tools.HandleStatus(ctx, cfg.KAMCPClient, args, cfg.Auditor)
			})
	}

	if shouldRegister("kubernaut_reconnect") {
		registerTool(srv, cfg, sem, "kubernaut_reconnect", "Reconnect to a disconnected investigation session",
			func(ctx context.Context, args tools.InteractiveActionArgs) (any, error) {
				return tools.HandleReconnect(ctx, cfg.KAMCPClient, cfg.TypedClient, cfg.Namespace, args, cfg.Auditor)
			})
	}
}

// finalizeSessionPhase transitions the IS CRD to the given terminal phase after
// a successful interactive-action handler call, logging (not failing the tool
// call) if finalization itself errors.
func finalizeSessionPhase(ctx context.Context, cfg *MCPBridgeConfig, rrID string, phase isv1alpha1.SessionPhase, handlerErr error) {
	if handlerErr != nil || cfg.SessionFinalizer == nil {
		return
	}
	if fErr := cfg.SessionFinalizer.FinalizeSessionByRR(ctx, cfg.Namespace, rrID, phase); fErr != nil {
		cfg.Logger.Error(fErr, "IS CRD phase finalization failed", "rr_id", rrID, "phase", phase)
	}
}

// registerAlertTools registers alert observation tools (#1412) — only when
// Prometheus/Thanos is configured.
func registerAlertTools(srv *mcp.Server, cfg *MCPBridgeConfig, sem *semaphore.Weighted) {
	if cfg.PromClient == nil {
		return
	}
	registerTool(srv, cfg, sem, "kubernaut_list_alerts", "List currently firing or pending Prometheus/Thanos alerts, optionally filtered by namespace, severity, or state",
		func(ctx context.Context, args tools.ListAlertsArgs) (any, error) {
			return tools.HandleListAlerts(ctx, cfg.PromClient, args)
		})
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

		defer func() {
			if r := recover(); r != nil {
				recordMetrics(cfg, toolName, "panic", start)
				emitAudit(ctx, cfg, toolName, audit.EventMCPToolFailed, map[string]string{"error": "internal error"})
				cfg.Logger.Error(fmt.Errorf("panic: %v", r), "tool handler panicked",
					"tool", toolName, "user", usernameFromCtx(ctx))
				toolResult = toolErrorResult("internal error")
			}
		}()

		// RBAC enforcement at runtime
		if err := checkRBAC(ctx, cfg, toolName); err != nil {
			return denyRBAC(ctx, cfg, toolName, start, err), nil, nil
		}

		// Per-user tool call rate limiting
		if cfg.UserLimiter != nil && !cfg.UserLimiter.AllowToolCall(usernameFromCtx(ctx)) {
			return denyRateLimited(ctx, cfg, toolName, start), nil, nil
		}

		// Timeout enforcement — covers semaphore wait + tool execution
		toolCtx, cancel := context.WithTimeout(ctx, cfg.GetToolTimeoutFor(toolName))
		defer cancel()

		// Semaphore for per-session concurrency limiting
		if err := sem.Acquire(toolCtx, 1); err != nil {
			return denyThrottled(ctx, cfg, toolName, start), nil, nil
		}
		defer sem.Release(1)

		// Execute handler
		result, err := handler(toolCtx, input)
		if err != nil {
			errResult, _ := handleToolExecError(ctx, toolCtx, cfg, toolName, start, input, err)
			return errResult, nil, nil
		}

		// Marshal result to JSON text
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return handleMarshalError(ctx, cfg, toolName, start, err), nil, nil
		}

		return handleToolSuccess(ctx, cfg, toolName, start, input, resultJSON), nil, nil
	}
}

// denyRBAC builds the response for an RBAC-denied tool call, recording
// metrics/audit/logging as a side effect.
func denyRBAC(ctx context.Context, cfg *MCPBridgeConfig, toolName string, start time.Time, err error) *mcp.CallToolResult {
	recordMetrics(cfg, toolName, "denied", start)
	emitAudit(ctx, cfg, toolName, audit.EventAuthAccessDenied, nil)
	cfg.Logger.Info("tool call denied by RBAC", "tool", toolName, "user", usernameFromCtx(ctx))
	return toolErrorResult(err.Error())
}

// denyRateLimited builds the response for a per-user rate-limited tool call.
func denyRateLimited(ctx context.Context, cfg *MCPBridgeConfig, toolName string, start time.Time) *mcp.CallToolResult {
	username := usernameFromCtx(ctx)
	recordMetrics(cfg, toolName, "rate_limited", start)
	emitAudit(ctx, cfg, toolName, audit.EventMCPToolFailed, map[string]string{"error": "rate_limited"})
	cfg.Logger.Info("tool call rate limited", "tool", toolName, "user", username)
	return toolErrorResult("rate limit exceeded — too many tool calls per minute, please retry later")
}

// denyThrottled builds the response for a concurrency-throttled tool call
// (per-session semaphore exhausted).
func denyThrottled(ctx context.Context, cfg *MCPBridgeConfig, toolName string, start time.Time) *mcp.CallToolResult {
	recordMetrics(cfg, toolName, "throttled", start)
	emitAudit(ctx, cfg, toolName, audit.EventMCPToolFailed, map[string]string{"error": "throttled"})
	cfg.Logger.Info("tool call throttled", "tool", toolName, "user", usernameFromCtx(ctx))
	return toolErrorResult("server busy — too many concurrent tool calls, please retry")
}

// handleToolExecError builds the response for a failed tool handler
// invocation, classifying the failure as "timeout" (toolCtx deadline
// exceeded/cancelled) or "error" otherwise. Returns the response and the
// resultLabel the caller should record for its own bookkeeping.
func handleToolExecError[In any](ctx, toolCtx context.Context, cfg *MCPBridgeConfig, toolName string, start time.Time, input In, err error) (*mcp.CallToolResult, string) {
	resultLabel := "error"
	if toolCtx.Err() != nil {
		resultLabel = "timeout"
	}
	recordMetrics(cfg, toolName, resultLabel, start)
	redacted := security.RedactError(err)
	errDetail := map[string]string{"error": redacted}
	enrichAuditFromArgs(errDetail, input)
	if cfg.Namespace != "" {
		errDetail["namespace"] = cfg.Namespace
	}
	emitAudit(ctx, cfg, toolName, audit.EventMCPToolFailed, errDetail)
	cfg.Logger.Error(err, "tool call failed",
		"tool", toolName, "result", resultLabel, "user", usernameFromCtx(ctx))
	return toolErrorResult(redacted), resultLabel
}

// handleMarshalError builds the response for a tool result that failed to
// marshal to JSON (should not normally happen; indicates a bug in the tool).
func handleMarshalError(ctx context.Context, cfg *MCPBridgeConfig, toolName string, start time.Time, err error) *mcp.CallToolResult {
	recordMetrics(cfg, toolName, "error", start)
	emitAudit(ctx, cfg, toolName, audit.EventMCPToolFailed, map[string]string{"error": "marshal failure"})
	cfg.Logger.Error(err, "tool result marshal failed", "tool", toolName, "user", usernameFromCtx(ctx))
	return toolErrorResult("internal error: failed to marshal result")
}

// handleToolSuccess builds the response for a successful tool call.
func handleToolSuccess[In any](ctx context.Context, cfg *MCPBridgeConfig, toolName string, start time.Time, input In, resultJSON []byte) *mcp.CallToolResult {
	durationMs := fmt.Sprintf("%d", time.Since(start).Milliseconds())
	recordMetrics(cfg, toolName, "success", start)
	auditDetail := map[string]string{
		"tool_outcome":          "success",
		"execution_duration_ms": durationMs,
	}
	enrichAuditFromArgs(auditDetail, input)
	if cfg.Namespace != "" {
		auditDetail["namespace"] = cfg.Namespace
	}
	emitAudit(ctx, cfg, toolName, audit.EventToolExecuted, auditDetail)
	cfg.Logger.Info("tool call succeeded",
		"tool", toolName,
		"user", usernameFromCtx(ctx),
		"duration_ms", durationMs,
	)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(resultJSON)}},
	}
}

// toolErrorResult builds an error CallToolResult with the given user-facing
// text.
func toolErrorResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: true,
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
// is configured. Legacy path retained for the non-blocking MCP bridge flow.
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

// buildISSignaler returns an ISSignaler adapter that wires
// CreateInvestigationSession + UpdateISCorrelation to the SessionInitializer.
// Returns nil if no SessionInitializer is configured.
func buildISSignaler(cfg *MCPBridgeConfig) tools.ISSignaler {
	if cfg.SessionInitializer == nil {
		return nil
	}
	return &isSignalerAdapter{
		initializer: cfg.SessionInitializer,
		namespace:   cfg.Namespace,
	}
}

type isSignalerAdapter struct {
	initializer ISSessionInitializer
	namespace   string
}

func (a *isSignalerAdapter) SignalInteractive(ctx context.Context, rrNamespace, rrName, taskID, username string, groups []string, joinMode string) (string, error) {
	jm := isv1alpha1.SessionJoinModeStart
	if joinMode == "takeover" {
		jm = isv1alpha1.SessionJoinModeTakeover
	}
	return a.initializer.CreateInvestigationSession(ctx, session.CreateISConfig{
		RRNamespace: rrNamespace,
		RRName:      rrName,
		TaskID:      taskID,
		Username:    username,
		Groups:      groups,
		JoinMode:    jm,
	})
}

func (a *isSignalerAdapter) UpdateCorrelation(ctx context.Context, crdName, kaSessionID string) error {
	return a.initializer.UpdateISCorrelation(ctx, crdName, kaSessionID)
}
