package agent

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/tool"

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

	a, err := llmagent.New(llmagent.Config{
		Name:                "kubernaut-apifrontend",
		Description:         "Kubernaut API Frontend agent for incident triage and remediation",
		Tools:               allTools,
		Instruction:         cfg.Instruction,
		BeforeToolCallbacks: []llmagent.BeforeToolCallback{newRBACGuard(cfg.Auditor)},
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
