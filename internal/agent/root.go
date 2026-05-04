package agent

import (
	_ "embed"
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/tool"

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
		Name:        "kubernaut-apifrontend",
		Description: "Kubernaut API Frontend agent for incident triage and remediation",
		Tools:       allTools,
		Instruction: cfg.Instruction,
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

	constructors := []toolConstructor{
		{"list_remediations", tools.NewListRemediationsTool},
		{"get_remediation", tools.NewGetRemediationTool},
		{"submit_signal", tools.NewSubmitSignalTool},
		{"approve", tools.NewApproveTool},
		{"cancel_remediation", tools.NewCancelRemediationTool},
		{"watch", tools.NewWatchTool},
		{"start_investigation", tools.NewStartInvestigationTool},
		{"poll_investigation", tools.NewPollInvestigationTool},
		{"select_workflow", tools.NewSelectWorkflowTool},
		{"present_decision", tools.NewPresentDecisionTool},
		{"list_workflows", tools.NewListWorkflowsTool},
		{"get_remediation_history", tools.NewGetRemediationHistoryTool},
		{"get_effectiveness", tools.NewGetEffectivenessTool},
		{"get_audit_trail", tools.NewGetAuditTrailTool},
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

// FilterToolsByRole returns only the tools accessible to the given role.
// Unknown roles get an empty list (fail-closed).
// The returned slice is a new allocation; the original is not mutated.
// Role mappings are loaded from the embedded rbac_roles.yaml; override at
// runtime via operator ConfigMap injection (PR7).
func FilterToolsByRole(role string, allTools []tool.Tool) []tool.Tool {
	rbac, err := loadDefaultRBAC()
	if err != nil {
		return nil
	}

	allowed, ok := rbac.Roles[role]
	if !ok {
		return nil
	}

	allowSet := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		allowSet[name] = true
	}

	result := make([]tool.Tool, 0, len(allowed))
	for _, t := range allTools {
		if allowSet[t.Name()] {
			result = append(result, t)
		}
	}
	return result
}
