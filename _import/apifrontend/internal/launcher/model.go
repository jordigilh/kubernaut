package launcher

import "fmt"

// Supported model providers.
const (
	ProviderVertexAI  = "vertexai"
	ProviderAnthropic = "anthropic"
)

// ModelConfig holds configuration for the LLM model used by the agent.
type ModelConfig struct {
	Provider      string
	Model         string
	JWTDelegation bool
}

// DefaultModelConfig returns the default model configuration targeting
// Claude Sonnet 4.6 via Vertex AI with JWT delegation enabled.
// TODO(PR6+): wired when model selection is configurable via env/CRD.
func DefaultModelConfig() ModelConfig {
	return ModelConfig{
		Provider:      ProviderVertexAI,
		Model:         "claude-sonnet-4-20250514",
		JWTDelegation: true,
	}
}

// Validate checks that the model configuration is well-formed.
func (c ModelConfig) Validate() error {
	if c.Model == "" {
		return fmt.Errorf("model must not be empty")
	}
	switch c.Provider {
	case ProviderVertexAI, ProviderAnthropic:
		return nil
	default:
		return fmt.Errorf("unsupported provider %q; must be %q or %q", c.Provider, ProviderVertexAI, ProviderAnthropic)
	}
}
