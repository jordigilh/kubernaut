package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/tool"

	"github.com/prometheus/client_golang/prometheus"

	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ratelimit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// NewRootAgent creates the ADK root agent with all registered tools.
// Returns the agent, the full tool list (for RBAC filtering), and any error.
//
//nolint:gocritic // hugeParam: value receiver intentional for immutable copy semantics
func NewRootAgent(cfg AgentConfig, opts ...Option) (agent.Agent, []tool.Tool, error) {
	cfg = cfg.Apply(opts...)

	if cfg.Instruction == "" {
		return nil, nil, fmt.Errorf("agent instruction must not be empty")
	}

	allTools, err := buildToolList(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("building tool list: %w", err)
	}

	if len(allTools) == 0 {
		return nil, nil, fmt.Errorf("tool list must not be empty: at least one tool is required")
	}

	beforeMetrics, afterMetrics := newMetricsToolCallbacks(cfg.ToolCallsTotal, cfg.ToolCallDuration)
	afterAudit := newAuditToolCallback(cfg.Auditor, cfg.SessionService, cfg.Namespace)

	var beforeCallbacks []llmagent.BeforeToolCallback
	if cfg.Authorizer != nil {
		beforeCallbacks = append(beforeCallbacks, newRBACGuard(cfg.Authorizer, cfg.Auditor))
	}
	if cfg.UserLimiter != nil {
		beforeCallbacks = append(beforeCallbacks, newRateLimitGuard(cfg.UserLimiter, cfg.Auditor))
	}
	beforeCallbacks = append(beforeCallbacks, beforeMetrics)

	beforePhase, afterPhase := newPhaseGuard(cfg.ActiveContextRegistry)
	beforeCallbacks = append(beforeCallbacks, beforePhase)

	beforeLog, afterLog := newToolLoggingCallbacks()
	beforeCallbacks = append(beforeCallbacks, beforeLog)

	a, err := llmagent.New(llmagent.Config{
		Name:                 "kubernaut-apifrontend",
		Description:          "Kubernaut API Frontend agent for incident triage and remediation",
		Model:                cfg.LLMModel,
		Tools:                allTools,
		Instruction:          cfg.Instruction,
		InstructionProvider:  cfg.InstructionProvider,
		BeforeModelCallbacks: []llmagent.BeforeModelCallback{historySanitizer},
		BeforeToolCallbacks:  beforeCallbacks,
		AfterToolCallbacks:   []llmagent.AfterToolCallback{afterMetrics, afterAudit, afterPhase, afterLog},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("creating agent: %w", err)
	}

	return a, allTools, nil
}

// toolConstructor pairs a diagnostic name with its constructor function.
type toolConstructor struct {
	name string
	fn   func() (tool.Tool, error)
}

//nolint:gocritic // hugeParam: value copy intentional; function is internal
func buildToolList(cfg AgentConfig) ([]tool.Tool, error) {
	if cfg.SkipTools {
		return nil, nil
	}

	k8s := cfg.K8sClient
	dsC := cfg.DSClient
	mcpC := cfg.MCPClient
	dedicatedC := cfg.DedicatedClient
	if dedicatedC == nil {
		dedicatedC = mcpC
	}

	// All internal tools use AF ServiceAccount. Access control is enforced
	// at the MCP tool level (RBAC guard): if the user has permission to invoke
	// kubernaut_investigate, AF investigates on their behalf using its
	// own SA. Users do not need direct K8s permissions for triage.
	saFactory := auth.StaticDynamicFactory(k8s)

	constructors := []toolConstructor{
		{"list_remediations", func() (tool.Tool, error) { return tools.NewListRemediationsTool(k8s, cfg.Namespace) }},
		{"get_remediation", func() (tool.Tool, error) { return tools.NewGetRemediationTool(k8s, cfg.Namespace) }},
		{"list_approval_requests", func() (tool.Tool, error) { return tools.NewListApprovalRequestsTool(k8s, cfg.Namespace) }},
		{"get_approval_request", func() (tool.Tool, error) { return tools.NewGetApprovalRequestTool(k8s, cfg.Namespace) }},
		{"approve", func() (tool.Tool, error) { return tools.NewApproveTool(k8s, cfg.Namespace) }},
		{"cancel_remediation", func() (tool.Tool, error) { return tools.NewCancelRemediationTool(k8s, cfg.Namespace) }},
		{"watch", func() (tool.Tool, error) { return tools.NewWatchTool(k8s, cfg.Namespace) }},
		{"investigate", func() (tool.Tool, error) {
			return tools.NewInvestigateMCPTool(dedicatedC, k8s, cfg.Namespace, cfg.Auditor, cfg.InvestigationRegistry, nil, cfg.Pool, buildAgentISSignaler(cfg), cfg.Triager)
		}},
		{"discover_workflows", func() (tool.Tool, error) { return tools.NewDiscoverWorkflowsTool(mcpC) }},
		{"select_workflow", func() (tool.Tool, error) { return tools.NewSelectWorkflowTool(mcpC, cfg.Auditor) }},
		{"present_decision", func() (tool.Tool, error) { return tools.NewPresentDecisionTool() }},
		{"get_remediation_history", func() (tool.Tool, error) { return tools.NewGetRemediationHistoryTool(dsC) }},
		{"get_effectiveness", func() (tool.Tool, error) { return tools.NewGetEffectivenessTool(dsC) }},
		{"get_audit_trail", func() (tool.Tool, error) { return tools.NewGetAuditTrailTool(dsC) }},
		// Generic K8s triage tools (#1230) — AF SA reads; access gated by MCP RBAC
		{"kubectl_get", func() (tool.Tool, error) { return tools.NewKubectlGetTool(saFactory, cfg.RESTMapper) }},
		{"kubectl_list", func() (tool.Tool, error) { return tools.NewKubectlListTool(saFactory, cfg.RESTMapper) }},
		{"kubectl_list_events", func() (tool.Tool, error) { return tools.NewKubectlListEventsTool(saFactory) }},
		// Interactive investigation tools — KA MCP backed
		{"message", func() (tool.Tool, error) { return tools.NewMessageTool(mcpC, cfg.Auditor) }},
		{"complete", func() (tool.Tool, error) { return tools.NewCompleteTool(mcpC, cfg.Auditor) }},
		{"cancel", func() (tool.Tool, error) { return tools.NewCancelInvestigationTool(mcpC, cfg.Auditor) }},
		{"status", func() (tool.Tool, error) { return tools.NewStatusTool(mcpC, cfg.Auditor) }},
		{"reconnect", func() (tool.Tool, error) { return tools.NewReconnectTool(mcpC, cfg.Auditor) }},
		// RR tools — AF SA writes AF-owned CRDs
		{"check_existing_remediation", func() (tool.Tool, error) {
			return tools.NewCheckExistingRemediationTool(k8s, cfg.Namespace)
		}},
		{"remediate", func() (tool.Tool, error) {
			return tools.NewRemediateTool(k8s, cfg.Namespace, cfg.Triager, cfg.Auditor)
		}},
	}

	result := make([]tool.Tool, 0, len(constructors))
	for _, c := range constructors {
		t, err := c.fn()
		if err != nil {
			return nil, fmt.Errorf("creating tool %q: %w", c.name, err)
		}
		if !cfg.InteractiveEnabled && tools.SessionDependentTools[t.Name()] {
			continue
		}
		result = append(result, t)
	}

	return result, nil
}

// NewRBACGuardForTest is an exported alias of newRBACGuard for integration
// testing via runner.Run. Production code should use the unexported constructor.
func NewRBACGuardForTest(authorizer auth.ToolAuthorizer, auditor audit.Emitter) llmagent.BeforeToolCallback {
	return newRBACGuard(authorizer, auditor)
}

// newRBACGuard returns a BeforeToolCallback that enforces RBAC via SAR.
// Fail-closed: if no identity, authorizer error, or denial, the tool call is rejected.
// Denied attempts are emitted as audit events for FedRAMP SI-4 compliance.
func newRBACGuard(authorizer auth.ToolAuthorizer, auditor audit.Emitter) llmagent.BeforeToolCallback {
	return func(ctx tool.Context, t tool.Tool, _ map[string]any) (map[string]any, error) {
		identity := auth.UserIdentityFromContext(ctx)
		if identity == nil {
			logr.FromContextOrDiscard(ctx).Info("rbac-guard denied tool", "tool", t.Name(), "reason", "no_identity_in_context")
			if auditor != nil {
				auditor.Emit(ctx, &audit.Event{
					Type: audit.EventAuthAccessDenied,
					Detail: map[string]string{
						"tool_name": t.Name(),
						"endpoint":  "a2a",
						"reason":    "no_identity_in_context",
					},
				})
			}
			return map[string]any{"error": "unauthorized: no identity in context"}, nil
		}

		toolName := t.Name()
		allowed, err := authorizer.Check(ctx, identity.Username, identity.Groups, toolName)
		if err != nil {
			logr.FromContextOrDiscard(ctx).Error(err, "rbac-guard denied tool", "tool", toolName, "user", identity.Username, "reason", "authorizer_error")
			if auditor != nil {
				auditor.Emit(ctx, &audit.Event{
					Type:   audit.EventAuthAccessDenied,
					UserID: identity.Username,
					Detail: map[string]string{
						"tool_name": toolName,
						"endpoint":  "a2a",
						"reason":    "authorizer_error",
						"groups":    strings.Join(identity.Groups, ","),
					},
				})
			}
			return map[string]any{"error": "authorization check failed"}, nil
		}
		if allowed {
			return nil, nil
		}

		if auditor != nil {
			auditor.Emit(ctx, &audit.Event{
				Type:   audit.EventAuthAccessDenied,
				UserID: identity.Username,
				Detail: map[string]string{
					"tool_name": toolName,
					"endpoint":  "a2a",
					"groups":    strings.Join(identity.Groups, ","),
				},
			})
		}

		return map[string]any{"error": fmt.Sprintf("forbidden: role does not grant access to tool %q", toolName)}, nil
	}
}

// newRateLimitGuard returns a BeforeToolCallback that enforces per-user
// tool-call rate limits in the A2A path (SEC-05). MCP bridge has its own
// rate limiter in wrapTool; this mirrors it for the A2A entry point.
func newRateLimitGuard(limiter *ratelimit.UserLimiter, auditor audit.Emitter) llmagent.BeforeToolCallback {
	return func(ctx tool.Context, t tool.Tool, _ map[string]any) (map[string]any, error) {
		identity := auth.UserIdentityFromContext(ctx)
		if identity == nil {
			return nil, nil
		}
		if limiter.AllowToolCall(identity.Username) {
			return nil, nil
		}
		logr.FromContextOrDiscard(ctx).Info("rate-limit-guard denied tool", "tool", t.Name(), "user", identity.Username, "reason", "rate_limited")
		if auditor != nil {
			auditor.Emit(ctx, &audit.Event{
				Type:   audit.EventRateLimitDenied,
				UserID: identity.Username,
				Detail: map[string]string{
					"tool_name": t.Name(),
					"endpoint":  "a2a",
					"tier":      "a2a_tool",
				},
			})
		}
		return map[string]any{"error": "rate limit exceeded — too many tool calls per minute, please retry later"}, nil
	}
}

// newMetricsToolCallbacks returns Before/After callbacks that track tool call
// metrics: af_tool_calls_total (counter) and af_tool_call_duration_seconds (histogram).
// Safe for concurrent use via sync.Map keyed by FunctionCallID.
//
// Leak analysis (SRE-1): entries are removed in `after` via LoadAndDelete. If `after`
// never runs (panic/cancel), leaked entries are bounded by LLM call rate (typically
// <100/min). Each entry is 24 bytes (time.Time). Worst case at 100 RPM with 100% loss
// is ~140KB/day — negligible for a long-running service. A periodic sweep is added as
// defense-in-depth.
func newMetricsToolCallbacks(toolCalls *prometheus.CounterVec, toolDuration *prometheus.HistogramVec) (llmagent.BeforeToolCallback, llmagent.AfterToolCallback) {
	var starts sync.Map

	// Periodic sweep: evict entries older than 5 minutes (abandoned tool calls).
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cutoff := time.Now().Add(-5 * time.Minute)
			starts.Range(func(key, value any) bool {
				if t, ok := value.(time.Time); ok && t.Before(cutoff) {
					starts.Delete(key)
				}
				return true
			})
		}
	}()

	before := func(ctx tool.Context, _ tool.Tool, _ map[string]any) (map[string]any, error) {
		if toolCalls == nil && toolDuration == nil {
			return nil, nil
		}
		starts.Store(ctx.FunctionCallID(), time.Now())
		return nil, nil
	}

	after := func(ctx tool.Context, t tool.Tool, _, _ map[string]any, toolErr error) (map[string]any, error) {
		resultLabel := "success"
		if toolErr != nil {
			resultLabel = "error"
		}
		if toolCalls != nil {
			toolCalls.WithLabelValues(t.Name(), resultLabel).Inc()
		}
		if toolDuration != nil {
			if raw, ok := starts.LoadAndDelete(ctx.FunctionCallID()); ok {
				if start, ok := raw.(time.Time); ok {
					elapsed := time.Since(start).Seconds()
					toolDuration.WithLabelValues(t.Name(), "function").Observe(elapsed)
				}
			}
		}
		return nil, nil
	}

	return before, after
}

// NewToolLoggingCallbacksForTest is an exported alias of newToolLoggingCallbacks
// for unit testing. Production code should use the unexported constructor.
func NewToolLoggingCallbacksForTest() (llmagent.BeforeToolCallback, llmagent.AfterToolCallback) {
	return newToolLoggingCallbacks()
}

// newToolLoggingCallbacks returns Before/After callbacks that log tool call
// start and completion at info level for operator observability (FedRAMP AU-12).
// Uses sync.Map keyed by FunctionCallID to correlate start times.
func newToolLoggingCallbacks() (llmagent.BeforeToolCallback, llmagent.AfterToolCallback) {
	var starts sync.Map

	before := func(ctx tool.Context, t tool.Tool, _ map[string]any) (map[string]any, error) {
		starts.Store(ctx.FunctionCallID(), time.Now())
		logr.FromContextOrDiscard(ctx).Info("tool call started", "tool", t.Name(), "callID", ctx.FunctionCallID())
		return nil, nil
	}

	after := func(ctx tool.Context, t tool.Tool, _, _ map[string]any, toolErr error) (map[string]any, error) {
		result := "success"
		if toolErr != nil {
			result = "error"
		}
		var durationMs int64
		if raw, ok := starts.LoadAndDelete(ctx.FunctionCallID()); ok {
			if start, ok := raw.(time.Time); ok {
				durationMs = time.Since(start).Milliseconds()
			}
		}
		logr.FromContextOrDiscard(ctx).Info("tool call completed", "tool", t.Name(), "callID", ctx.FunctionCallID(), "durationMs", durationMs, "result", result)
		return nil, nil
	}

	return before, after
}

// BuildAgentISSignalerForTest exposes buildAgentISSignaler for integration tests.
func BuildAgentISSignalerForTest(cfg AgentConfig) tools.ISSignaler {
	return buildAgentISSignaler(cfg)
}

// buildAgentISSignaler returns an ISSignaler wired to the CRDSessionService.
// Returns nil when no SessionService is configured (e.g., unit tests).
func buildAgentISSignaler(cfg AgentConfig) tools.ISSignaler {
	if cfg.SessionService == nil {
		return nil
	}
	return &agentISSignalerAdapter{svc: cfg.SessionService}
}

type agentISSignalerAdapter struct {
	svc *session.CRDSessionService
}

func (a *agentISSignalerAdapter) SignalInteractive(ctx context.Context, rrNamespace, rrName, taskID, username string, groups []string, joinMode string) (string, error) {
	jm := isv1alpha1.SessionJoinModeStart
	if joinMode == "takeover" {
		jm = isv1alpha1.SessionJoinModeTakeover
	}
	return a.svc.CreateInvestigationSession(ctx, session.CreateISConfig{
		RRNamespace: rrNamespace,
		RRName:      rrName,
		TaskID:      taskID,
		Username:    username,
		Groups:      groups,
		JoinMode:    jm,
	})
}

func (a *agentISSignalerAdapter) UpdateCorrelation(ctx context.Context, crdName, kaSessionID string) error {
	return a.svc.UpdateISCorrelation(ctx, crdName, kaSessionID)
}

// newAuditToolCallback returns an AfterToolCallback that emits a structured
// audit event for every tool invocation (FedRAMP AU-12 compliance).
// The event includes tool name, result status, and user identity.
// Issue #1332: when kubernaut_remediate or kubernaut_investigate is called
// within an A2A task context, the audit event includes a2a_task_id for
// bidirectional task-to-RR correlation.
// G6 (revised #1332): IS CRD creation moved to kubernaut_investigate ISSignaler.
// The sessionSvc parameter is retained for future use but MaterializeCRD is no
// longer called from this callback.
func newAuditToolCallback(auditor audit.Emitter, sessionSvc *session.CRDSessionService, controllerNS string) llmagent.AfterToolCallback {
	return func(ctx tool.Context, t tool.Tool, input, output map[string]any, toolErr error) (map[string]any, error) {
		if auditor == nil {
			return nil, nil
		}

		result := "success"
		if toolErr != nil {
			result = "failure"
			logr.FromContextOrDiscard(ctx).Error(toolErr, "tool call failed", "tool", t.Name())
		}

		detail := map[string]string{
			"tool_name":    t.Name(),
			"tool_outcome": result,
		}
		if toolErr != nil {
			detail["error"] = security.RedactError(toolErr)
		}
		if ns, ok := input["namespace"].(string); ok && ns != "" {
			detail["namespace"] = ns
		}

		// Issue #1189: A2A task-to-RR correlation. When kubernaut_remediate or
		// kubernaut_investigate succeeds within an A2A task, include both
		// a2a_task_id and rr_id in the audit event so the Data Store can
		// correlate them bidirectionally.
		sc := session.CreateContextFromContext(ctx)
		if sc != nil && sc.TaskID != "" {
			detail["a2a_task_id"] = sc.TaskID
		}
		toolName := t.Name()
		isRRCreatingTool := (toolName == "kubernaut_remediate" || toolName == "kubernaut_investigate")
		if isRRCreatingTool && toolErr == nil && output != nil {
			if rrID, ok := output["rr_id"].(string); ok && rrID != "" {
				detail["rr_id"] = rrID
				if sc != nil {
					sc.RRName = rrID
					sc.RRNamespace = controllerNS
				}
			}
		}

		userID := ""
		if identity := auth.UserIdentityFromContext(ctx); identity != nil {
			userID = identity.Username
		}

		auditor.Emit(ctx, &audit.Event{
			Type:   audit.EventToolExecuted,
			UserID: userID,
			Detail: detail,
		})

		return nil, nil
	}
}

