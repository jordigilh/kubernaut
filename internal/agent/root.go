package agent

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/tool"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

//go:embed rbac_roles.yaml
var embeddedRBACConfig []byte

// rbacConfig holds the parsed RBAC role-to-tools mapping.
type rbacConfig struct {
	Roles map[string][]string `yaml:"roles"`
}

var (
	defaultRBAC     rbacConfig
	defaultRBACOnce sync.Once
	defaultRBACErr  error
)

func loadDefaultRBAC() (rbacConfig, error) {
	defaultRBACOnce.Do(func() {
		defaultRBACErr = yaml.Unmarshal(embeddedRBACConfig, &defaultRBAC)
	})
	return defaultRBAC, defaultRBACErr
}

// LoadRBACRoles returns the embedded RBAC role-to-tools mapping for use by
// components that need to filter capabilities per persona (e.g., Agent Card).
func LoadRBACRoles() (map[string][]string, error) {
	cfg, err := loadDefaultRBAC()
	if err != nil {
		return nil, fmt.Errorf("load rbac_roles.yaml: %w", err)
	}
	return cfg.Roles, nil
}

// NewRootAgent creates the ADK root agent with all registered tools.
// Returns the agent, the full tool list (for RBAC filtering), and any error.
// The agent is configured without a model (model wiring is deferred to PR5 launcher).
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
	afterAudit := newAuditToolCallback(cfg.Auditor)

	a, err := llmagent.New(llmagent.Config{
		Name:                "kubernaut-apifrontend",
		Description:         "Kubernaut API Frontend agent for incident triage and remediation",
		Tools:               allTools,
		Instruction:         cfg.Instruction,
		BeforeToolCallbacks: []llmagent.BeforeToolCallback{newRBACGuard(cfg.Auditor), beforeMetrics},
		AfterToolCallbacks:  []llmagent.AfterToolCallback{afterMetrics, afterAudit},
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
	kaC := cfg.KAClient
	mcpC := cfg.MCPClient

	// Triage tools use impersonation; fall back to static SA client if no factory provided.
	triageFactory := cfg.ImpersonatingClientFactory
	if triageFactory == nil {
		triageFactory = auth.StaticDynamicFactory(k8s)
	}

	constructors := []toolConstructor{
		{"list_remediations", func() (tool.Tool, error) { return tools.NewListRemediationsTool(k8s) }},
		{"get_remediation", func() (tool.Tool, error) { return tools.NewGetRemediationTool(k8s) }},
		{"submit_signal", func() (tool.Tool, error) { return tools.NewSubmitSignalTool(k8s) }},
		{"approve", func() (tool.Tool, error) { return tools.NewApproveTool(k8s) }},
		{"cancel_remediation", func() (tool.Tool, error) { return tools.NewCancelRemediationTool(k8s) }},
		{"watch", func() (tool.Tool, error) { return tools.NewWatchTool(k8s) }},
		{"start_investigation", func() (tool.Tool, error) { return tools.NewStartInvestigationTool(kaC) }},
		{"poll_investigation", func() (tool.Tool, error) { return tools.NewPollInvestigationTool(kaC) }},
		{"select_workflow", func() (tool.Tool, error) { return tools.NewSelectWorkflowTool(mcpC) }},
		{"present_decision", func() (tool.Tool, error) { return tools.NewPresentDecisionTool() }},
		{"list_workflows", func() (tool.Tool, error) { return tools.NewListWorkflowsTool(dsC) }},
		{"get_remediation_history", func() (tool.Tool, error) { return tools.NewGetRemediationHistoryTool(dsC) }},
		{"get_effectiveness", func() (tool.Tool, error) { return tools.NewGetEffectivenessTool(dsC) }},
		{"get_audit_trail", func() (tool.Tool, error) { return tools.NewGetAuditTrailTool(dsC) }},
		// NL Signal Intake tools (#52) — read-only tools use impersonation (SEC-05)
		{"list_events", func() (tool.Tool, error) { return tools.NewListEventsTool(triageFactory) }},
		{"get_pods", func() (tool.Tool, error) { return tools.NewGetPodsTool(triageFactory) }},
		{"get_workloads", func() (tool.Tool, error) { return tools.NewGetWorkloadsTool(triageFactory) }},
		{"resolve_owner", func() (tool.Tool, error) { return tools.NewResolveOwnerTool(triageFactory) }},
		// RR tools use AF ServiceAccount (write AF-owned CRDs)
		{"check_existing_rr", func() (tool.Tool, error) { return tools.NewCheckExistingRRTool(k8s) }},
		{"create_rr", func() (tool.Tool, error) { return tools.NewCreateRRTool(k8s) }},
	}

	result := make([]tool.Tool, 0, len(constructors))
	for _, c := range constructors {
		t, err := c.fn()
		if err != nil {
			return nil, fmt.Errorf("creating tool %q: %w", c.name, err)
		}
		result = append(result, t)
	}

	return result, nil
}

// newRBACGuard returns a BeforeToolCallback that enforces RBAC by checking
// whether the authenticated user's groups grant access to the requested tool.
// Fail-closed: if no identity or no matching role, the tool call is rejected.
// Denied attempts are emitted as audit events for FedRAMP SI-4 compliance.
func newRBACGuard(auditor audit.Emitter) llmagent.BeforeToolCallback {
	return func(ctx tool.Context, t tool.Tool, _ map[string]any) (map[string]any, error) {
		identity := auth.UserIdentityFromContext(ctx)
		if identity == nil {
			return map[string]any{"error": "unauthorized: no identity in context"}, nil
		}

		rbac, err := loadDefaultRBAC()
		if err != nil {
			return map[string]any{"error": "rbac configuration error"}, nil
		}

		toolName := t.Name()
		for _, group := range identity.Groups {
			allowed, ok := rbac.Roles[group]
			if !ok {
				continue
			}
			for _, name := range allowed {
				if name == toolName {
					return nil, nil
				}
			}
		}

		if auditor != nil {
			auditor.Emit(ctx, &audit.Event{
				Type:   audit.EventRBACDenied,
				UserID: identity.Username,
				Detail: map[string]string{
					"tool":   toolName,
					"groups": strings.Join(identity.Groups, ","),
				},
			})
		}

		return map[string]any{"error": fmt.Sprintf("forbidden: role does not grant access to tool %q", toolName)}, nil
	}
}

// FilterToolsByRole returns only the tools accessible to the given role.
// Unknown roles get an empty list (fail-closed).
// The returned slice is a new allocation; the original is not mutated.
// Role mappings are loaded from the embedded rbac_roles.yaml; override at
// runtime via operator ConfigMap injection (PR7).
func FilterToolsByRole(role string, allTools []tool.Tool) []tool.Tool {
	return FilterToolsByRoles([]string{role}, allTools)
}

// FilterToolsByRoles returns the union of tools accessible to any of the given
// roles. This models multi-group membership where a user in [cicd, l3-audit]
// gets the combined tool set from both roles.
func FilterToolsByRoles(roles []string, allTools []tool.Tool) []tool.Tool {
	rbac, err := loadDefaultRBAC()
	if err != nil {
		return nil
	}

	allowSet := make(map[string]bool)
	for _, role := range roles {
		allowed, ok := rbac.Roles[role]
		if !ok {
			continue
		}
		for _, name := range allowed {
			allowSet[name] = true
		}
	}

	if len(allowSet) == 0 {
		return nil
	}

	result := make([]tool.Tool, 0, len(allowSet))
	for _, t := range allTools {
		if allowSet[t.Name()] {
			result = append(result, t)
		}
	}
	return result
}

// newMetricsToolCallbacks returns Before/After callbacks that track tool call
// metrics: af_tool_calls_total (counter) and af_tool_call_duration_seconds (histogram).
// Safe for concurrent use via sync.Map keyed by FunctionCallID.
func newMetricsToolCallbacks(toolCalls *prometheus.CounterVec, toolDuration *prometheus.HistogramVec) (llmagent.BeforeToolCallback, llmagent.AfterToolCallback) {
	var starts sync.Map

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

// newAuditToolCallback returns an AfterToolCallback that emits a structured
// audit event for every tool invocation (FedRAMP AU-12 compliance).
// The event includes tool name, result status, and user identity.
func newAuditToolCallback(auditor audit.Emitter) llmagent.AfterToolCallback {
	return func(ctx tool.Context, t tool.Tool, input, _ map[string]any, toolErr error) (map[string]any, error) {
		if auditor == nil {
			return nil, nil
		}

		result := "success"
		if toolErr != nil {
			result = "error"
		}

		detail := map[string]string{
			"tool":   t.Name(),
			"result": result,
		}
		if toolErr != nil {
			detail["error"] = toolErr.Error()
		}
		if ns, ok := input["namespace"].(string); ok && ns != "" {
			detail["namespace"] = ns
		}

		userID := ""
		if identity := auth.UserIdentityFromContext(ctx); identity != nil {
			userID = identity.Username
		}

		auditor.Emit(ctx, &audit.Event{
			Type:   audit.EventToolInvoked,
			UserID: userID,
			Detail: detail,
		})

		return nil, nil
	}
}
