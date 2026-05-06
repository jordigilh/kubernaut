// Package agent provides the ADK root agent skeleton, configuration, and
// RBAC-based tool filtering for the kubernaut API Frontend.
package agent

import (
	"k8s.io/client-go/dynamic"

	"github.com/jordigilh/kubernaut-apifrontend/internal/ds"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ka"
)

// AgentConfig holds the configuration for creating the ADK root agent.
//
//nolint:revive // stutters with package name but preferred for clarity across the codebase
type AgentConfig struct {
	// GCPProject is the Vertex AI project for the Claude model.
	GCPProject string
	// GCPRegion is the Vertex AI region.
	GCPRegion string
	// Instruction is the system prompt guiding agent behavior.
	Instruction string
	// SkipTools disables tool registration (for testing error paths).
	SkipTools bool
	// KABaseURL is the base URL for the Kubernaut Agent REST API.
	KABaseURL string
	// KAMCPEndpoint is the MCP endpoint URL for KA.
	KAMCPEndpoint string
	// DSBaseURL is the base URL for the Data Store API.
	DSBaseURL string
	// K8sClient is the dynamic K8s client for CRD operations.
	K8sClient dynamic.Interface
	// DSClient is the Data Store client for workflow/history queries.
	DSClient ds.Client
	// KAClient is the Kubernaut Agent REST client for investigations.
	KAClient *ka.Client
	// MCPClient is the KA MCP client for workflow selection.
	MCPClient ka.MCPClient
}

// Option applies a configuration override to AgentConfig.
type Option func(*AgentConfig)

// WithGCPProject sets the Vertex AI project.
func WithGCPProject(project string) Option {
	return func(c *AgentConfig) { c.GCPProject = project }
}

// WithGCPRegion sets the Vertex AI region.
func WithGCPRegion(region string) Option {
	return func(c *AgentConfig) { c.GCPRegion = region }
}

// WithInstruction sets the system prompt.
func WithInstruction(instruction string) Option {
	return func(c *AgentConfig) { c.Instruction = instruction }
}

// WithKABaseURL sets the KA REST API base URL.
func WithKABaseURL(url string) Option {
	return func(c *AgentConfig) { c.KABaseURL = url }
}

// WithKAMCPEndpoint sets the KA MCP endpoint URL.
func WithKAMCPEndpoint(url string) Option {
	return func(c *AgentConfig) { c.KAMCPEndpoint = url }
}

// WithDSBaseURL sets the Data Store API base URL.
func WithDSBaseURL(url string) Option {
	return func(c *AgentConfig) { c.DSBaseURL = url }
}

// DefaultTestConfig returns a config suitable for unit tests with placeholder values.
func DefaultTestConfig() AgentConfig {
	return AgentConfig{
		GCPProject:    "test-project",
		GCPRegion:     "us-central1",
		Instruction:   defaultInstruction(),
		KABaseURL:     "http://localhost:8080",
		KAMCPEndpoint: "http://localhost:8080/api/v1/mcp/",
		DSBaseURL:     "http://localhost:9090",
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
