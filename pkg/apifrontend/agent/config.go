// Package agent provides the ADK root agent skeleton, configuration, and
// RBAC-based tool filtering for the kubernaut API Frontend.
package agent

import (
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"

	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ds"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ratelimit"
	sessionpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/session"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// AgentConfig holds the configuration for creating the ADK root agent.
//
//nolint:revive // stutters with package name but preferred for clarity across the codebase
type AgentConfig struct {
	// Instruction is the system prompt guiding agent behavior.
	// Ignored when InstructionProvider is set.
	Instruction string
	// InstructionProvider dynamically generates per-request instructions.
	// Takes priority over Instruction when set.
	InstructionProvider llmagent.InstructionProvider
	// SkipTools disables tool registration (for testing error paths).
	SkipTools bool
	// K8sClient is the dynamic K8s client for CRD operations.
	K8sClient dynamic.Interface
	// DSClient is the Data Store client for workflow/history queries.
	DSClient ds.Client
	// MCPClient is the KA MCP client for interactive operations (pooled sessions).
	MCPClient ka.MCPClient
	// DedicatedClient is the KA MCP client for dedicated investigation sessions.
	// It must support StartInvestigation (SDKMCPClient). Falls back to MCPClient
	// when nil.
	DedicatedClient ka.MCPClient
	// InvestigationRegistry tracks active investigation sessions for graceful
	// shutdown. If nil, sessions clean up via bridge goroutine defer only.
	InvestigationRegistry *tools.MonitorRegistry
	// Pool is the KA session pool. When set, the blocking investigate path
	// hands off its MCP session to the pool so that discover_workflows /
	// select_workflow reuse the same connection and driver lease.
	Pool *ka.KASessionPool
	// Authorizer checks tool-level authorization via SAR.
	Authorizer auth.ToolAuthorizer
	// Auditor emits audit events for RBAC denials (FedRAMP SI-4).
	Auditor audit.Emitter
	// ToolCallsTotal is the af_tool_calls_total counter for observability wiring.
	ToolCallsTotal *prometheus.CounterVec
	// ToolCallDuration is the af_tool_call_duration_seconds histogram.
	ToolCallDuration *prometheus.HistogramVec
	// UserLimiter enforces per-user tool call rate limits in the A2A path (SEC-05).
	UserLimiter *ratelimit.UserLimiter
	// RESTMapper resolves Kind strings to GVR for generic kubectl tools.
	// If nil, only statically-known kinds are supported.
	RESTMapper meta.RESTMapper
	// Namespace is the resolved operational namespace for CRD creation.
	Namespace string
	// Triager performs severity triage for kubernaut_remediate and
	// kubernaut_investigate (when auto-creating RRs from namespace/kind/name).
	// If nil, severity defaults to "medium" without source attribution.
	Triager *severity.Triager
	// SessionService is the CRD session service for deferred CRD materialization (G6).
	// When non-nil, used by kubernaut_investigate to create the InvestigationSession
	// CRD once a real RR reference is available.
	SessionService *sessionpkg.CRDSessionService
	// LLMModel is the model backend for the ADK agent. When non-nil, the agent
	// uses this model for generateContent calls. When nil, the agent is created
	// without a model (tools-only mode for MCP bridge).
	LLMModel model.LLM
	// ActiveContextRegistry enables multi-turn session continuity (BR-SESS-020).
	// When non-nil, the phase guard stores/clears the active context on
	// investigate success and complete/cancel respectively.
	ActiveContextRegistry *launcher.ActiveContextRegistry
	// InteractiveEnabled controls whether session-dependent tools are registered
	// in the A2A agent tool list. When false, tools in tools.SessionDependentTools
	// are excluded (#1366). DefaultTestConfig sets this to true.
	InteractiveEnabled bool
}

// Option applies a configuration override to AgentConfig.
type Option func(*AgentConfig)

// WithInstruction sets the system prompt.
func WithInstruction(instruction string) Option {
	return func(c *AgentConfig) { c.Instruction = instruction }
}

// DefaultTestConfig returns a config suitable for unit tests with placeholder values.
func DefaultTestConfig() AgentConfig {
	return AgentConfig{
		Instruction:        defaultInstruction(),
		InteractiveEnabled: true,
	}
}

// Apply returns a new AgentConfig with the given options applied.
//
//nolint:gocritic // hugeParam: value receiver intentional for immutable copy semantics
func (c AgentConfig) Apply(opts ...Option) AgentConfig {
	for _, opt := range opts {
		opt(&c)
	}
	return c
}
