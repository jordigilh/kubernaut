package types

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Supported LLM provider values. Both AF and KA use these constants.
const (
	LLMProviderVertexAI         = "vertex_ai"
	LLMProviderGemini           = "gemini"
	LLMProviderAnthropic        = "anthropic"
	LLMProviderOpenAI           = "openai"
	LLMProviderOpenAICompatible = "openai_compatible"
)

// validPhaseNames enumerates the phase keys accepted in PhaseModels.
var validPhaseNames = map[string]bool{
	"rca":                true,
	"workflow_discovery": true,
	"validation":         true,
}

// IsValidPhaseName reports whether name is an accepted PhaseModels key.
func IsValidPhaseName(name string) bool {
	return validPhaseNames[name]
}

// LLMConfig is the shared LLM configuration type used by both the API Frontend
// and Kubernaut Agent. It covers the union of all fields from both services.
// KA internally decides which fields are hot-reloadable vs restart-only; the
// type itself is agnostic to that split.
type LLMConfig struct {
	Provider        string                    `yaml:"provider"`
	Model           string                    `yaml:"model"`
	Endpoint        string                    `yaml:"endpoint,omitempty"`
	APIKeyFile      string                    `yaml:"apiKeyFile,omitempty"`
	VertexProject   string                    `yaml:"vertexProject,omitempty"`
	VertexLocation  string                    `yaml:"vertexLocation,omitempty"`
	AzureAPIVersion string                    `yaml:"azureApiVersion,omitempty"`
	BedrockRegion   string                    `yaml:"bedrockRegion,omitempty"`
	Temperature     *float64                  `yaml:"temperature,omitempty"`
	MaxRetries      *int                      `yaml:"maxRetries,omitempty"`
	TimeoutSeconds  int                       `yaml:"timeoutSeconds,omitempty"`
	TLSCaFile       string                    `yaml:"tlsCaFile,omitempty"`
	TLSCertFile     string                    `yaml:"tlsCertFile,omitempty"`
	TLSKeyFile      string                    `yaml:"tlsKeyFile,omitempty"`
	OAuth2          LLMOAuth2Config           `yaml:"oauth2,omitempty"`
	CircuitBreaker  LLMCircuitBreaker         `yaml:"circuitBreaker,omitempty"`
	CustomHeaders   []LLMHeaderDef            `yaml:"customHeaders,omitempty"`
	PhaseModels     map[string]*LLMOverride   `yaml:"phaseModels,omitempty"`
	// Resolved at runtime from APIKeyFile. Not serialized.
	APIKey string `yaml:"-"`
}

// LLMOAuth2Config holds OAuth2 client credentials for auth-gated LLM gateways.
type LLMOAuth2Config struct {
	Enabled        bool     `yaml:"enabled"`
	TokenURL       string   `yaml:"tokenURL"`
	Scopes         []string `yaml:"scopes,omitempty"`
	CredentialsDir string   `yaml:"credentialsDir"`
	// Resolved at runtime. Not serialized.
	ClientID     string `yaml:"-"`
	ClientSecret string `yaml:"-"`
}

// LLMCircuitBreaker configures resilience for LLM HTTP calls.
type LLMCircuitBreaker struct {
	Enabled          bool          `yaml:"enabled"`
	MaxRequests      uint32        `yaml:"maxRequests"`
	Interval         time.Duration `yaml:"interval"`
	Timeout          time.Duration `yaml:"timeout"`
	FailureThreshold uint32        `yaml:"failureThreshold"`
	FailureRatio     float64       `yaml:"failureRatio,omitempty"`
}

// LLMHeaderDef describes a custom HTTP header injected into outbound LLM requests.
// Exactly one value source (Value, SecretKeyRef, or FilePath) must be set.
type LLMHeaderDef struct {
	Name         string `yaml:"name"`
	Value        string `yaml:"value,omitempty"`
	SecretKeyRef string `yaml:"secretKeyRef,omitempty"`
	FilePath     string `yaml:"filePath,omitempty"`
}

// LLMOverride allows per-phase or per-component LLM configuration overrides.
// Non-empty fields win over the base LLMConfig values.
type LLMOverride struct {
	Provider        string `yaml:"provider,omitempty"`
	Endpoint        string `yaml:"endpoint,omitempty"`
	Model           string `yaml:"model,omitempty"`
	APIKeyFile      string `yaml:"apiKeyFile,omitempty"`
	AzureAPIVersion string `yaml:"azureApiVersion,omitempty"`
	VertexProject   string `yaml:"vertexProject,omitempty"`
	VertexLocation  string `yaml:"vertexLocation,omitempty"`
	BedrockRegion   string `yaml:"bedrockRegion,omitempty"`
}

// IsEmpty reports whether all override fields are zero-valued.
func (o *LLMOverride) IsEmpty() bool {
	return o.Provider == "" && o.Endpoint == "" && o.Model == "" &&
		o.APIKeyFile == "" && o.AzureAPIVersion == "" &&
		o.VertexProject == "" && o.VertexLocation == "" && o.BedrockRegion == ""
}

// DefaultLLMTimeoutSeconds is the fallback HTTP timeout for LLM requests.
const DefaultLLMTimeoutSeconds = 120

// ResolveAPIKey reads the API key from the file at APIKeyFile into the APIKey
// field. It is a no-op when APIKeyFile is empty (supports keyless providers).
func (c *LLMConfig) ResolveAPIKey() error {
	if c.APIKeyFile == "" {
		return nil
	}
	v, err := readSecretFile(c.APIKeyFile)
	if err != nil {
		return fmt.Errorf("reading API key from %q: %w", c.APIKeyFile, err)
	}
	c.APIKey = v
	return nil
}

// Validate checks that the LLMConfig has valid provider-specific fields.
// The prefix parameter is used in error messages (e.g. "agent.llm", "ai.llm").
// Returns nil when Provider is empty (LLM not configured).
func (c *LLMConfig) Validate(prefix string) error {
	if c.Provider == "" {
		return nil
	}
	if err := c.validateProviderAndModel(prefix); err != nil {
		return err
	}
	if err := c.validateProviderRequiredFields(prefix); err != nil {
		return err
	}
	if err := c.validatePaths(prefix); err != nil {
		return err
	}
	if err := c.validateEndpointURL(prefix); err != nil {
		return err
	}
	if err := c.validateOAuth2(prefix); err != nil {
		return err
	}
	if err := c.validateCircuitBreaker(prefix); err != nil {
		return err
	}
	return c.validatePhaseModels(prefix)
}

// validateProviderAndModel checks that Provider is one of the supported enum
// values and that Model is set.
func (c *LLMConfig) validateProviderAndModel(prefix string) error {
	switch c.Provider {
	case LLMProviderVertexAI, LLMProviderGemini, LLMProviderAnthropic,
		LLMProviderOpenAI, LLMProviderOpenAICompatible:
	default:
		return fmt.Errorf("%s.provider must be one of %q, %q, %q, %q, %q; got %q",
			prefix, LLMProviderVertexAI, LLMProviderGemini, LLMProviderAnthropic,
			LLMProviderOpenAI, LLMProviderOpenAICompatible, c.Provider)
	}
	if c.Model == "" {
		return fmt.Errorf("%s.model is required when provider is set", prefix)
	}
	return nil
}

// validateProviderRequiredFields checks provider-specific required fields:
// Vertex AI project/location, an API key (Gemini/Anthropic/OpenAI, unless
// OAuth2 is enabled), and an endpoint (OpenAI/OpenAI-compatible).
func (c *LLMConfig) validateProviderRequiredFields(prefix string) error {
	if c.Provider == LLMProviderVertexAI {
		if c.VertexProject == "" {
			return fmt.Errorf("%s.vertexProject is required for provider %q", prefix, c.Provider)
		}
		if c.VertexLocation == "" {
			return fmt.Errorf("%s.vertexLocation is required for provider %q", prefix, c.Provider)
		}
	}

	if (c.Provider == LLMProviderGemini || c.Provider == LLMProviderAnthropic) &&
		c.APIKeyFile == "" && !c.OAuth2.Enabled {
		return fmt.Errorf("%s.apiKeyFile (or oauth2) is required for provider %q", prefix, c.Provider)
	}

	if (c.Provider == LLMProviderOpenAI || c.Provider == LLMProviderOpenAICompatible) &&
		c.Endpoint == "" {
		return fmt.Errorf("%s.endpoint is required for provider %q", prefix, c.Provider)
	}

	if c.Provider == LLMProviderOpenAI && c.APIKeyFile == "" && !c.OAuth2.Enabled {
		return fmt.Errorf("%s.apiKeyFile (or oauth2) is required for provider %q", prefix, c.Provider)
	}
	return nil
}

// validatePaths checks that file-path fields are absolute and that any TLS
// cert/key pair is fully and consistently specified.
func (c *LLMConfig) validatePaths(prefix string) error {
	if c.APIKeyFile != "" && !filepath.IsAbs(c.APIKeyFile) {
		return fmt.Errorf("%s.apiKeyFile must be an absolute path, got %q", prefix, c.APIKeyFile)
	}
	if c.TLSCaFile != "" && !filepath.IsAbs(c.TLSCaFile) {
		return fmt.Errorf("%s.tlsCaFile must be an absolute path, got %q", prefix, c.TLSCaFile)
	}
	return validateTLSCertPair(prefix, c.TLSCertFile, c.TLSKeyFile, c.TLSCaFile)
}

// validateEndpointURL checks that Endpoint, if set, parses as a valid URL.
func (c *LLMConfig) validateEndpointURL(prefix string) error {
	if c.Endpoint == "" {
		return nil
	}
	if _, err := url.ParseRequestURI(c.Endpoint); err != nil {
		return fmt.Errorf("%s.endpoint is not a valid URL: %w", prefix, err)
	}
	return nil
}

// validateOAuth2 checks OAuth2 fields when enabled: a valid tokenURL and a
// non-empty, absolute credentialsDir.
func (c *LLMConfig) validateOAuth2(prefix string) error {
	if !c.OAuth2.Enabled {
		return nil
	}
	if c.OAuth2.TokenURL == "" {
		return fmt.Errorf("%s.oauth2.tokenURL is required when oauth2 is enabled", prefix)
	}
	if _, err := url.ParseRequestURI(c.OAuth2.TokenURL); err != nil {
		return fmt.Errorf("%s.oauth2.tokenURL is not a valid URL: %w", prefix, err)
	}
	if c.OAuth2.CredentialsDir == "" {
		return fmt.Errorf("%s.oauth2.credentialsDir is required when oauth2 is enabled", prefix)
	}
	if !filepath.IsAbs(c.OAuth2.CredentialsDir) {
		return fmt.Errorf("%s.oauth2.credentialsDir must be an absolute path, got %q",
			prefix, c.OAuth2.CredentialsDir)
	}
	return nil
}

// validateCircuitBreaker checks CircuitBreaker fields when enabled: a failure
// threshold in [1,100] and a positive timeout.
func (c *LLMConfig) validateCircuitBreaker(prefix string) error {
	if !c.CircuitBreaker.Enabled {
		return nil
	}
	if c.CircuitBreaker.FailureThreshold == 0 || c.CircuitBreaker.FailureThreshold > 100 {
		return fmt.Errorf("%s.circuitBreaker.failureThreshold must be 1-100, got %d",
			prefix, c.CircuitBreaker.FailureThreshold)
	}
	if c.CircuitBreaker.Timeout <= 0 {
		return fmt.Errorf("%s.circuitBreaker.timeout must be positive", prefix)
	}
	return nil
}

// validatePhaseModels checks that every PhaseModels key is a known phase name
// and every override has at least one non-empty field.
func (c *LLMConfig) validatePhaseModels(prefix string) error {
	for phase, override := range c.PhaseModels {
		if !IsValidPhaseName(phase) {
			return fmt.Errorf("%s.phaseModels: unknown phase %q", prefix, phase)
		}
		if override == nil || override.IsEmpty() {
			return fmt.Errorf("%s.phaseModels[%q]: at least one override field must be set", prefix, phase)
		}
	}
	return nil
}

// ValidateSource checks that exactly one value source is set on a header definition.
func (h LLMHeaderDef) ValidateSource() error {
	count := 0
	if h.Value != "" {
		count++
	}
	if h.SecretKeyRef != "" {
		count++
	}
	if h.FilePath != "" {
		count++
	}
	if count != 1 {
		return fmt.Errorf("exactly one of value, secretKeyRef, or filePath must be set (got %d)", count)
	}
	return nil
}

// ResolveOAuth2Credentials reads clientID and clientSecret from mounted
// Secret files in the configured credentialsDir.
func (c *LLMOAuth2Config) ResolveOAuth2Credentials() error {
	if !c.Enabled {
		return nil
	}
	if c.CredentialsDir == "" {
		return fmt.Errorf("oauth2.credentialsDir is required when oauth2.enabled=true")
	}
	clientID, err := readSecretFile(filepath.Join(c.CredentialsDir, "client-id"))
	if err != nil {
		return fmt.Errorf("reading oauth2 client-id: %w", err)
	}
	clientSecret, err := readSecretFile(filepath.Join(c.CredentialsDir, "client-secret"))
	if err != nil {
		return fmt.Errorf("reading oauth2 client-secret: %w", err)
	}
	c.ClientID = clientID
	c.ClientSecret = clientSecret
	return nil
}

func validateTLSCertPair(prefix, certFile, keyFile, caFile string) error {
	hasCert := certFile != ""
	hasKey := keyFile != ""
	if hasCert != hasKey {
		return fmt.Errorf("%s.tlsCertFile and %s.tlsKeyFile must both be set or both be empty", prefix, prefix)
	}
	if !hasCert {
		return nil
	}
	if !filepath.IsAbs(certFile) {
		return fmt.Errorf("%s.tlsCertFile must be an absolute path, got %q", prefix, certFile)
	}
	if !filepath.IsAbs(keyFile) {
		return fmt.Errorf("%s.tlsKeyFile must be an absolute path, got %q", prefix, keyFile)
	}
	if caFile == "" {
		return fmt.Errorf("%s.tlsCaFile is required when client certificates are configured (server verification is mandatory)", prefix)
	}
	return nil
}

func readSecretFile(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path comes from validated config, not user input
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(data))
	if v == "" {
		return "", fmt.Errorf("file %s is empty", path)
	}
	return v, nil
}
