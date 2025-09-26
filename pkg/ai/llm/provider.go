package llm

import (
	"net/http"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/sirupsen/logrus"
)

// Provider defines the interface that all LLM providers must implement
// This enables true dependency injection and allows adding new providers
// without modifying existing code (Open/Closed Principle)
type Provider interface {
	// Initialize configures the provider with the given configuration
	Initialize(config config.LLMConfig, logger *logrus.Logger) error

	// GetEndpoint returns the provider's API endpoint
	GetEndpoint() string

	// GetAPIKey returns the provider's API key (empty if not needed)
	GetAPIKey() string

	// GetHeaders returns any additional headers needed for requests
	GetHeaders() map[string]string

	// ValidateConfig validates that the provider configuration is correct
	ValidateConfig() error

	// GetName returns the provider name for logging/identification
	GetName() string

	// CreateHTTPClient creates an HTTP client configured for this provider
	CreateHTTPClient() *http.Client

	// GetDefaultModel returns the default model for this provider
	GetDefaultModel() string

	// SupportsChatCompletion returns true if the provider supports chat completion API
	SupportsChatCompletion() bool

	// GetMaxTokens returns the maximum tokens supported by this provider
	GetMaxTokens() int
}

// ProviderRegistry manages the available LLM providers
type ProviderRegistry struct {
	providers map[string]func() Provider
}

// NewProviderRegistry creates a new provider registry with production providers
func NewProviderRegistry() *ProviderRegistry {
	registry := &ProviderRegistry{
		providers: make(map[string]func() Provider),
	}

	// Register production-ready providers only
	registry.RegisterProvider("openai", func() Provider { return &OpenAIProvider{} })
	registry.RegisterProvider("huggingface", func() Provider { return &HuggingFaceProvider{} })
	registry.RegisterProvider("ollama", func() Provider { return &OllamaProvider{} })
	registry.RegisterProvider("ramalama", func() Provider { return &RamallamaProvider{} })

	return registry
}

// NewDevelopmentProviderRegistry creates a provider registry suitable for development
// Note: For testing with mock providers, use pkg/ai/llm/testing.CreateTestRegistry()
func NewDevelopmentProviderRegistry() *ProviderRegistry {
	// In pure Option C, development registry is same as production
	// Testing concerns are completely separated
	return NewProviderRegistry()
}

// RegisterProvider registers a new provider factory function
func (r *ProviderRegistry) RegisterProvider(name string, factory func() Provider) {
	r.providers[name] = factory
}

// CreateProvider creates a new provider instance by name
func (r *ProviderRegistry) CreateProvider(name string) (Provider, error) {
	factory, exists := r.providers[name]
	if !exists {
		return nil, &UnsupportedProviderError{Provider: name}
	}

	return factory(), nil
}

// GetSupportedProviders returns a list of all supported provider names
func (r *ProviderRegistry) GetSupportedProviders() []string {
	var providers []string
	for name := range r.providers {
		providers = append(providers, name)
	}
	return providers
}

// UnsupportedProviderError represents an error when a provider is not supported
type UnsupportedProviderError struct {
	Provider string
}

func (e *UnsupportedProviderError) Error() string {
	return "unsupported LLM provider '" + e.Provider + "' for enterprise deployment. Use RegisterProvider() to add custom providers"
}

// Default provider registry instance - production-ready providers only
// For testing with mock providers, use pkg/ai/llm/testing.CreateTestRegistry()
var DefaultProviderRegistry = NewProviderRegistry()
