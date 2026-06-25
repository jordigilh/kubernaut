/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	pkgconfig "github.com/jordigilh/kubernaut/pkg/kubernautagent/config"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"gopkg.in/yaml.v3"
)

// Config holds all Kubernaut Agent configuration organized into 3 concern domains.
type Config struct {
	Runtime      RuntimeConfig      `yaml:"runtime"`
	AI           AIConfig           `yaml:"ai"`
	Integrations IntegrationsConfig `yaml:"integrations"`
	Interactive  InteractiveConfig  `yaml:"interactive"`
}

// RuntimeConfig holds operational infrastructure settings.
type RuntimeConfig struct {
	Logging  internalconfig.LoggingConfig `yaml:"logging"`
	Server   ServerConfig                 `yaml:"server"`
	Session  SessionConfig                `yaml:"session"`
	Audit    AuditConfig                  `yaml:"audit"`
	Shutdown ShutdownConfig               `yaml:"shutdown"`
}

// ShutdownConfig holds graceful shutdown parameters.
// The operator renders runtime.shutdown.drainSeconds from the CRD's
// spec.kubernautAgent.shutdown.drainSeconds field.
type ShutdownConfig struct {
	DrainSeconds int `yaml:"drainSeconds"`
}

// AIConfig holds LLM, investigation behavior, and safety guardrails.
type AIConfig struct {
	LLM            types.LLMConfig      `yaml:"llm"`
	Investigation  InvestigationConfig  `yaml:"investigation"`
	Summarizer     SummarizerConfig     `yaml:"summarizer"`
	Enrichment     EnrichmentConfig     `yaml:"enrichment"`
	AlignmentCheck AlignmentCheckConfig `yaml:"alignmentCheck"`
	Safety         SafetyConfig         `yaml:"safety"`
}

// SafetyConfig holds guardrails that constrain AI behavior.
type SafetyConfig struct {
	Sanitization SanitizationConfig `yaml:"sanitization"`
	Anomaly      AnomalyConfig      `yaml:"anomaly"`
}

// IntegrationsConfig holds external service connection settings.
type IntegrationsConfig struct {
	DataStorage DataStorageConfig `yaml:"dataStorage"`
	Tools       ToolsConfig       `yaml:"tools"`
	MCP         MCPConfig         `yaml:"mcp"`
	Fleet       FleetConfig       `yaml:"fleet"`
}

// FleetConfig configures multi-cluster fleet access via MCP Gateway.
// When Endpoint is non-empty, KA discovers remote cluster tools via
// tools/list and registers them as BridgeTools (additive to local tools).
type FleetConfig struct {
	Endpoint string       `yaml:"endpoint"`
	OAuth2   FleetOAuth2  `yaml:"oauth2"`
}

// FleetOAuth2 holds OAuth2 client credentials for MCP Gateway authentication.
type FleetOAuth2 struct {
	Enabled              bool     `yaml:"enabled"`
	TokenURL             string   `yaml:"tokenURL"`
	CredentialsSecretRef string   `yaml:"credentialsSecretRef"`
	Scopes               []string `yaml:"scopes,omitempty"`
}


// LLMRuntimeConfig holds hot-reloadable LLM settings that can change without restart.
// This struct maps to a separate ConfigMap (kubernaut-agent-llm-runtime) watched by
// the FileWatcher.
type LLMRuntimeConfig struct {
	Model          string                       `yaml:"model"`
	Endpoint       string                       `yaml:"endpoint"`
	APIKeyFile     string                       `yaml:"apiKeyFile,omitempty"`
	Temperature    float64                      `yaml:"temperature"`
	MaxRetries     int                          `yaml:"maxRetries"`
	TimeoutSeconds int                          `yaml:"timeoutSeconds"`
	CustomHeaders  []types.LLMHeaderDef `yaml:"customHeaders,omitempty"`
	PhaseModels    map[string]*LLMOverrideConfig `yaml:"phaseModels,omitempty"`
	// Resolved at runtime from APIKeyFile. Not serialized.
	APIKey string `yaml:"-"`
}

// ValidPhaseNames enumerates the phase keys accepted in PhaseModels.
var ValidPhaseNames = map[string]bool{
	"rca":                true,
	"workflow_discovery": true,
	"validation":         true,
}

// EffectivePhaseConfig returns a merged set of static + runtime fields for the
// given phase. If PhaseModels contains an override for the phase, non-empty
// fields win over the base values. If no override exists, the base values are
// returned unchanged. Follows the same merge pattern as
// AlignmentCheckConfig.EffectiveLLM().
func (r *LLMRuntimeConfig) EffectivePhaseConfig(phase string, baseLLM types.LLMConfig, baseRuntime LLMRuntimeConfig) (types.LLMConfig, LLMRuntimeConfig) {
	if len(r.PhaseModels) == 0 {
		return baseLLM, baseRuntime
	}
	override, ok := r.PhaseModels[phase]
	if !ok || override == nil {
		return baseLLM, baseRuntime
	}
	staticOut := baseLLM
	runtimeOut := baseRuntime
	if override.Provider != "" {
		staticOut.Provider = override.Provider
	}
	if override.AzureAPIVersion != "" {
		staticOut.AzureAPIVersion = override.AzureAPIVersion
	}
	if override.VertexProject != "" {
		staticOut.VertexProject = override.VertexProject
	}
	if override.VertexLocation != "" {
		staticOut.VertexLocation = override.VertexLocation
	}
	if override.BedrockRegion != "" {
		staticOut.BedrockRegion = override.BedrockRegion
	}
	if override.Model != "" {
		runtimeOut.Model = override.Model
	}
	if override.Endpoint != "" {
		runtimeOut.Endpoint = override.Endpoint
	}
	if override.APIKeyFile != "" {
		runtimeOut.APIKeyFile = override.APIKeyFile
	}
	return staticOut, runtimeOut
}


type DataStorageConfig struct {
	URL            string              `yaml:"url"`
	SATokenPath    string              `yaml:"saTokenPath"`
	TLS            sharedtls.TLSConfig `yaml:"tls,omitempty"`
	CircuitBreaker types.LLMCircuitBreaker `yaml:"circuitBreaker,omitempty"`
}

type ServerConfig struct {
	Address               string              `yaml:"address"`
	Port                  int                 `yaml:"port"`
	HealthAddr            string              `yaml:"healthAddr"`
	MetricsAddr           string              `yaml:"metricsAddr"`
	DisableProfiling      bool                `yaml:"disableProfiling"`
	DisableAdminEndpoints bool                `yaml:"disableAdminEndpoints"`
	MaxConcurrentRequests int                 `yaml:"maxConcurrentRequests"`
	TLS                   sharedtls.TLSConfig `yaml:"tls,omitempty"`
	TLSProfile            string              `yaml:"tlsProfile,omitempty"`
	RateLimit             RateLimitConfig     `yaml:"rateLimit"`
}

// RateLimitConfig configures per-IP HTTP rate limiting for the agent API.
type RateLimitConfig struct {
	RequestsPerSecond float64       `yaml:"requestsPerSecond"`
	Burst             int           `yaml:"burst"`
	CleanupInterval   time.Duration `yaml:"cleanupInterval"`
	MaxAge            time.Duration `yaml:"maxAge"`
	TrustedProxyCIDRs []string      `yaml:"trustedProxyCIDRs"`
}

type SessionConfig struct {
	TTL                        time.Duration `yaml:"ttl"`
	MaxConcurrentInvestigations int          `yaml:"maxConcurrentInvestigations"`
}

type AuditConfig struct {
	Enabled              bool    `yaml:"enabled"`
	Endpoint             string  `yaml:"endpoint"`
	FlushIntervalSeconds float64 `yaml:"flushIntervalSeconds"`
	BufferSize           int     `yaml:"bufferSize"`
	BatchSize            int     `yaml:"batchSize"`
	Verbosity            string  `yaml:"verbosity"`
}

// Deprecated: MCPConfig configures outbound MCP client connections.
// For interactive MCP server configuration, use InteractiveConfig instead (#703).
type MCPConfig struct {
	Servers []MCPServerEntry `yaml:"servers"`
}

// InteractiveConfig controls the MCP interactive session server (#703).
// When Enabled=true, KA exposes an SSE-based MCP endpoint for user-driven
// investigations. Feature is gated off by default.
type InteractiveConfig struct {
	Enabled                bool                `yaml:"enabled"`
	SessionTTL             time.Duration       `yaml:"sessionTTL"`
	InactivityTimeout      time.Duration       `yaml:"inactivityTimeout"`
	MaxConcurrentSessions  int                 `yaml:"maxConcurrentSessions"`
	RateLimitPerUser       int                 `yaml:"rateLimitPerUser"`
	MaxAnalyzingTimeout    time.Duration       `yaml:"maxAnalyzingTimeout"`
	DisconnectGracePeriod  time.Duration       `yaml:"disconnectGracePeriod"`
	JWTProviders []JWTProviderConfig `yaml:"jwtProviders,omitempty"`

	// MCPKeepAlive is the server-side ping interval for MCP sessions (#1387).
	// Keeps SSE streams alive through OCP router idle timeouts.
	// Zero means no keepalive pings. Default: 15s.
	MCPKeepAlive time.Duration `yaml:"mcpKeepAlive"`
	// MCPSessionTimeout auto-closes idle MCP sessions that AF abandoned
	// without a proper disconnect (#1387). Zero means never. Default: 10m.
	MCPSessionTimeout time.Duration `yaml:"mcpSessionTimeout"`
}

const (
	DefaultMCPKeepAlive      = 15 * time.Second
	DefaultMCPSessionTimeout = 10 * time.Minute
)

// JWTProviderConfig defines a trusted JWT issuer for Pattern B authentication.
// DD-AUTH-MCP-001 v2.0: KA validates JWT signatures via JWKS and extracts
// identity from verified claims. Multiple providers support KEP-3331
// multi-issuer architecture (v1.5: Keycloak, v1.6: + SPIRE).
type JWTProviderConfig struct {
	Name          string        `yaml:"name"`
	Issuer        string        `yaml:"issuer"`
	JWKSURL       string        `yaml:"jwksURL"`
	Audience      string        `yaml:"audience"`
	ClaimMappings ClaimMappings `yaml:"claimMappings,omitempty"`
}

// ClaimMappings configures which JWT claims map to user identity fields.
// Supports dot-notation for nested Keycloak claims (e.g., "realm_access.roles").
type ClaimMappings struct {
	Username string `yaml:"username"`
	Groups   string `yaml:"groups"`
}

const maxURLLength = 2048

// maxDrainSeconds mirrors the operator CRD's max(300) for shutdown.drainSeconds.
const maxDrainSeconds = 300

// validateJWTProviders checks all configured JWT providers for required fields,
// validates URL format and length, applies claim mapping defaults, and rejects
// duplicate issuer URLs.
//
// HTTPS enforcement for JWKS URLs is the operator's responsibility
// (kubernaut-operator#46). KA accepts http:// for dev/test flexibility.
func (ic *InteractiveConfig) validateJWTProviders() error {
	if len(ic.JWTProviders) == 0 {
		return nil
	}

	seenIssuers := make(map[string]string, len(ic.JWTProviders))
	for i := range ic.JWTProviders {
		p := &ic.JWTProviders[i]
		name := p.Name
		if name == "" {
			name = fmt.Sprintf("jwtProviders[%d]", i)
		}

		if p.Issuer == "" {
			return fmt.Errorf("interactive.%s: issuer is required", name)
		}
		if len(p.Issuer) > maxURLLength {
			return fmt.Errorf("interactive.%s: issuer exceeds maximum length of %d characters", name, maxURLLength)
		}
		if p.JWKSURL == "" {
			return fmt.Errorf("interactive.%s: jwksURL is required", name)
		}
		if len(p.JWKSURL) > maxURLLength {
			return fmt.Errorf("interactive.%s: jwksURL exceeds maximum length of %d characters", name, maxURLLength)
		}
		if err := validateJWKSURL(p.JWKSURL); err != nil {
			return fmt.Errorf("interactive.%s: %w", name, err)
		}
		if p.Audience == "" {
			return fmt.Errorf("interactive.%s: audience is required", name)
		}

		if prev, dup := seenIssuers[p.Issuer]; dup {
			return fmt.Errorf("interactive.jwtProviders: duplicate issuer %q in providers %q and %q", p.Issuer, prev, name)
		}
		seenIssuers[p.Issuer] = name

		if p.ClaimMappings.Username == "" {
			p.ClaimMappings.Username = "preferred_username"
		}
		if p.ClaimMappings.Groups == "" {
			p.ClaimMappings.Groups = "groups"
		}
	}
	return nil
}

// validateJWKSURL checks that the JWKS URL is syntactically valid and uses
// an HTTP or HTTPS scheme. HTTPS enforcement in production is delegated to
// the kubernaut-operator admission webhook (kubernaut-operator#46).
func validateJWKSURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid jwksURL %q: %w", rawURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid jwksURL %q: scheme must be http or https, got %q", rawURL, u.Scheme)
	}
	return nil
}

type MCPServerEntry struct {
	Name      string `yaml:"name"`
	URL       string `yaml:"url"`
	Transport string `yaml:"transport"`
}

type InvestigationConfig struct {
	MaxTurns int `yaml:"maxTurns"`
}

type ToolsConfig struct {
	Prometheus   PrometheusToolConfig   `yaml:"prometheus"`
	Alertmanager AlertmanagerToolConfig `yaml:"alertmanager"`
}

type AlertmanagerToolConfig struct {
	URL       string        `yaml:"url"`
	Timeout   time.Duration `yaml:"timeout"`
	SizeLimit int           `yaml:"sizeLimit"`
	TLSCaFile string        `yaml:"tlsCaFile"`
}

type PrometheusToolConfig struct {
	URL       string        `yaml:"url"`
	Timeout   time.Duration `yaml:"timeout"`
	SizeLimit int           `yaml:"sizeLimit"`
	TLSCaFile string        `yaml:"tlsCaFile"`
}

type SanitizationConfig struct {
	InjectionPatternsEnabled bool `yaml:"injectionPatternsEnabled"`
	CredentialScrubEnabled   bool `yaml:"credentialScrubEnabled"`
	SecretRedactionEnabled   bool `yaml:"secretRedactionEnabled"`
}

// DefaultMaxToolOutputSize is the default hard character limit for tool output
// before it enters the LLM context window. ~25K tokens for most models.
const DefaultMaxToolOutputSize = 100000

type SummarizerConfig struct {
	Threshold         int `yaml:"threshold"`
	MaxToolOutputSize int `yaml:"maxToolOutputSize"`
}

// EnrichmentConfig controls retry behavior for K8s owner chain resolution
// during enrichment. HAPI-aligned defaults (MaxRetries=3, BaseBackoff=1s)
// ensure rca_incomplete is triggered on definitive enrichment failure.
type EnrichmentConfig struct {
	MaxRetries  int           `yaml:"maxRetries"`
	BaseBackoff time.Duration `yaml:"baseBackoff"`
}

type AnomalyConfig struct {
	MaxToolCallsPerTool int      `yaml:"maxToolCallsPerTool"`
	MaxTotalToolCalls   int      `yaml:"maxTotalToolCalls"`
	MaxRepeatedFailures int      `yaml:"maxRepeatedFailures"`
	ExemptPrefixes      []string `yaml:"exemptPrefixes"`
}

// AlignmentMode determines how the alignment checker acts on its verdict.
type AlignmentMode string

const (
	AlignmentModeEnforce AlignmentMode = "enforce"
	AlignmentModeMonitor AlignmentMode = "monitor"
)

// AlignmentCheckConfig holds settings for the shadow agent alignment checker (#601).
type AlignmentCheckConfig struct {
	Enabled        bool                 `yaml:"enabled"`
	Mode           AlignmentMode        `yaml:"mode"`
	LLM            *LLMOverrideConfig   `yaml:"llm"`
	Timeout        time.Duration        `yaml:"timeout"`
	MaxStepTokens  int                  `yaml:"maxStepTokens"`
	MaxRetries     int                  `yaml:"maxRetries"`
	VerdictTimeout time.Duration        `yaml:"verdictTimeout"`
	Canary         CanaryConfig         `yaml:"canary"`
	GroundingReview GroundingReviewConfig `yaml:"groundingReview"`
}

// GroundingReviewConfig holds settings for the full-context grounding review (#1096).
// When enabled, the shadow agent evaluates the entire RCA conversation at the
// RCA-to-workflow-discovery boundary to detect distributed prompt injection.
type GroundingReviewConfig struct {
	Enabled               bool          `yaml:"enabled"`
	Timeout               time.Duration `yaml:"timeout"`
	MaxConversationTokens int           `yaml:"maxConversationTokens"`
}

// CanaryConfig controls per-investigation canary integrity checks.
type CanaryConfig struct {
	ForceEscalation bool `yaml:"forceEscalation"`
}

// LLMOverrideConfig allows the alignment checker to use a different LLM than
// the primary investigator. All fields are optional; non-zero fields override
// the corresponding base values.
type LLMOverrideConfig struct {
	Provider        string `yaml:"provider"`
	Endpoint        string `yaml:"endpoint"`
	Model           string `yaml:"model"`
	APIKeyFile      string `yaml:"apiKeyFile,omitempty"`
	AzureAPIVersion string `yaml:"azureApiVersion"`
	VertexProject   string `yaml:"vertexProject"`
	VertexLocation  string `yaml:"vertexLocation"`
	BedrockRegion   string `yaml:"bedrockRegion"`
}

// EffectiveLLM returns a merged set of static + runtime fields for the
// alignment checker client builder. If override fields are set, they win.
func (c *AlignmentCheckConfig) EffectiveLLM(base types.LLMConfig, runtime LLMRuntimeConfig) (types.LLMConfig, LLMRuntimeConfig) {
	if c.LLM == nil {
		return base, runtime
	}
	staticOut := base
	runtimeOut := runtime
	if c.LLM.Provider != "" {
		staticOut.Provider = c.LLM.Provider
	}
	if c.LLM.AzureAPIVersion != "" {
		staticOut.AzureAPIVersion = c.LLM.AzureAPIVersion
	}
	if c.LLM.VertexProject != "" {
		staticOut.VertexProject = c.LLM.VertexProject
	}
	if c.LLM.VertexLocation != "" {
		staticOut.VertexLocation = c.LLM.VertexLocation
	}
	if c.LLM.BedrockRegion != "" {
		staticOut.BedrockRegion = c.LLM.BedrockRegion
	}
	if c.LLM.Model != "" {
		runtimeOut.Model = c.LLM.Model
	}
	if c.LLM.Endpoint != "" {
		runtimeOut.Endpoint = c.LLM.Endpoint
	}
	if c.LLM.APIKeyFile != "" {
		runtimeOut.APIKeyFile = c.LLM.APIKeyFile
	}
	return staticOut, runtimeOut
}

// Load parses configuration from YAML bytes and applies defaults.
func Load(data []byte) (*Config, error) {
	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

// LoadLLMRuntime parses LLM runtime configuration from YAML bytes.
func LoadLLMRuntime(data []byte) (*LLMRuntimeConfig, error) {
	var rt LLMRuntimeConfig
	if err := yaml.Unmarshal(data, &rt); err != nil {
		return nil, fmt.Errorf("parsing llm runtime config: %w", err)
	}
	return &rt, nil
}

// Validate checks that the LLM runtime config has the minimum required fields.
func (r *LLMRuntimeConfig) Validate(provider string) error {
	if r.Model == "" {
		return fmt.Errorf("model is required")
	}
	switch provider {
	case "bedrock", "huggingface", "anthropic", "openai", "vertex", "vertex_ai":
	default:
		if r.Endpoint == "" {
			return fmt.Errorf("endpoint is required for provider %q", provider)
		}
	}
	if len(r.CustomHeaders) > 0 {
		if err := pkgconfig.ValidateHeaderSources(r.CustomHeaders); err != nil {
			return fmt.Errorf("customHeaders: %w", err)
		}
	}
	for phase, override := range r.PhaseModels {
		if !ValidPhaseNames[phase] {
			return fmt.Errorf("phaseModels: unknown phase %q", phase)
		}
		if override == nil || (override.Provider == "" && override.Endpoint == "" &&
			override.Model == "" && override.APIKeyFile == "" &&
			override.AzureAPIVersion == "" && override.VertexProject == "" &&
			override.VertexLocation == "" && override.BedrockRegion == "") {
			return fmt.Errorf("phaseModels[%q]: at least one override field must be set", phase)
		}
	}
	return nil
}

// Validate checks required fields and value constraints for the static config.
// Runtime LLM fields are validated separately via LLMRuntimeConfig.Validate().
func (c *Config) Validate() error {
	if err := c.Runtime.Logging.Validate(); err != nil {
		return err
	}

	if c.Runtime.Server.Port < 1 || c.Runtime.Server.Port > 65535 {
		return fmt.Errorf("runtime.server.port must be 1-65535, got %d", c.Runtime.Server.Port)
	}
	if c.Runtime.Server.MaxConcurrentRequests < 0 {
		return fmt.Errorf("runtime.server.maxConcurrentRequests must be non-negative, got %d", c.Runtime.Server.MaxConcurrentRequests)
	}
	if c.Runtime.Session.TTL <= 0 {
		return fmt.Errorf("runtime.session.ttl must be positive, got %v", c.Runtime.Session.TTL)
	}
	if c.Runtime.Session.MaxConcurrentInvestigations <= 0 {
		return fmt.Errorf("runtime.session.maxConcurrentInvestigations must be positive, got %d", c.Runtime.Session.MaxConcurrentInvestigations)
	}
	if c.Runtime.Audit.Enabled {
		if c.Runtime.Audit.BufferSize <= 0 {
			return fmt.Errorf("runtime.audit.bufferSize must be positive when audit enabled, got %d", c.Runtime.Audit.BufferSize)
		}
		if c.Runtime.Audit.BatchSize <= 0 {
			return fmt.Errorf("runtime.audit.batchSize must be positive when audit enabled, got %d", c.Runtime.Audit.BatchSize)
		}
	}
	switch c.Runtime.Audit.Verbosity {
	case "full", "standard", "minimal", "":
	default:
		return fmt.Errorf("runtime.audit.verbosity must be full, standard, or minimal, got %q", c.Runtime.Audit.Verbosity)
	}
	if c.Runtime.Shutdown.DrainSeconds <= 0 {
		return fmt.Errorf("runtime.shutdown.drainSeconds must be positive, got %d; "+
			"the operator default is 30s", c.Runtime.Shutdown.DrainSeconds)
	}
	if c.Runtime.Shutdown.DrainSeconds > maxDrainSeconds {
		return fmt.Errorf("runtime.shutdown.drainSeconds must not exceed %d, got %d; "+
			"the operator enforces this upper bound via CRD validation",
			maxDrainSeconds, c.Runtime.Shutdown.DrainSeconds)
	}

	if c.AI.Investigation.MaxTurns <= 0 {
		return fmt.Errorf("ai.investigation.maxTurns must be positive, got %d", c.AI.Investigation.MaxTurns)
	}
	if c.AI.Summarizer.MaxToolOutputSize <= 0 {
		return fmt.Errorf("ai.summarizer.maxToolOutputSize must be positive, got %d", c.AI.Summarizer.MaxToolOutputSize)
	}
	if c.AI.Enrichment.MaxRetries < 0 {
		return fmt.Errorf("ai.enrichment.maxRetries must be non-negative, got %d", c.AI.Enrichment.MaxRetries)
	}
	if c.AI.Safety.Anomaly.MaxToolCallsPerTool <= 0 {
		return fmt.Errorf("ai.safety.anomaly.maxToolCallsPerTool must be positive, got %d", c.AI.Safety.Anomaly.MaxToolCallsPerTool)
	}
	if c.AI.Safety.Anomaly.MaxTotalToolCalls <= 0 {
		return fmt.Errorf("ai.safety.anomaly.maxTotalToolCalls must be positive, got %d", c.AI.Safety.Anomaly.MaxTotalToolCalls)
	}
	if c.AI.Safety.Anomaly.MaxRepeatedFailures <= 0 {
		return fmt.Errorf("ai.safety.anomaly.maxRepeatedFailures must be positive, got %d", c.AI.Safety.Anomaly.MaxRepeatedFailures)
	}

	if c.AI.LLM.OAuth2.Enabled {
		if c.AI.LLM.OAuth2.TokenURL == "" {
			return fmt.Errorf("ai.llm.oauth2.tokenURL is required when oauth2.enabled=true")
		}
		if c.AI.LLM.OAuth2.CredentialsDir == "" {
			return fmt.Errorf("ai.llm.oauth2.credentialsDir is required when oauth2.enabled=true")
		}
	}
	if err := validateTLSCertPair("ai.llm", c.AI.LLM.TLSCertFile, c.AI.LLM.TLSKeyFile, c.AI.LLM.TLSCaFile); err != nil {
		return err
	}
	if c.Integrations.Fleet.OAuth2.Enabled {
		if c.Integrations.Fleet.OAuth2.TokenURL == "" {
			return fmt.Errorf("integrations.fleet.oauth2.tokenURL is required when oauth2.enabled=true")
		}
		if c.Integrations.Fleet.OAuth2.CredentialsSecretRef == "" {
			return fmt.Errorf("integrations.fleet.oauth2.credentialsSecretRef is required when oauth2.enabled=true")
		}
	}
	if c.Runtime.Server.RateLimit.RequestsPerSecond <= 0 {
		return fmt.Errorf("runtime.server.rateLimit.requestsPerSecond must be positive, got %v", c.Runtime.Server.RateLimit.RequestsPerSecond)
	}
	if c.Runtime.Server.RateLimit.Burst <= 0 {
		return fmt.Errorf("runtime.server.rateLimit.burst must be positive, got %d", c.Runtime.Server.RateLimit.Burst)
	}
	if c.AI.AlignmentCheck.Enabled {
		if c.AI.AlignmentCheck.Timeout <= 0 {
			return fmt.Errorf("ai.alignmentCheck.timeout must be positive when enabled, got %v", c.AI.AlignmentCheck.Timeout)
		}
		if c.AI.AlignmentCheck.MaxStepTokens <= 0 {
			return fmt.Errorf("ai.alignmentCheck.maxStepTokens must be positive when enabled, got %d", c.AI.AlignmentCheck.MaxStepTokens)
		}
		if c.AI.AlignmentCheck.VerdictTimeout <= 0 {
			return fmt.Errorf("ai.alignmentCheck.verdictTimeout must be positive when enabled, got %v", c.AI.AlignmentCheck.VerdictTimeout)
		}
		if c.AI.AlignmentCheck.MaxRetries < 0 {
			return fmt.Errorf("ai.alignmentCheck.maxRetries must be non-negative when enabled, got %d", c.AI.AlignmentCheck.MaxRetries)
		}
		switch c.AI.AlignmentCheck.Mode {
		case AlignmentModeEnforce, AlignmentModeMonitor:
		default:
			return fmt.Errorf("ai.alignmentCheck.mode must be 'enforce' or 'monitor', got %q", c.AI.AlignmentCheck.Mode)
		}
		if c.AI.AlignmentCheck.GroundingReview.Enabled {
			if c.AI.AlignmentCheck.GroundingReview.Timeout <= 0 {
				return fmt.Errorf("ai.alignmentCheck.groundingReview.timeout must be positive when enabled, got %v", c.AI.AlignmentCheck.GroundingReview.Timeout)
			}
			if c.AI.AlignmentCheck.GroundingReview.MaxConversationTokens <= 0 {
				return fmt.Errorf("ai.alignmentCheck.groundingReview.maxConversationTokens must be positive when enabled, got %d", c.AI.AlignmentCheck.GroundingReview.MaxConversationTokens)
			}
		}
	}
	if c.Interactive.Enabled {
		if c.Interactive.SessionTTL <= 0 {
			return fmt.Errorf("interactive.sessionTTL must be positive when enabled, got %v", c.Interactive.SessionTTL)
		}
		if c.Interactive.SessionTTL > time.Hour {
			return fmt.Errorf("interactive.sessionTTL must not exceed 1h, got %v", c.Interactive.SessionTTL)
		}
		if c.Interactive.InactivityTimeout <= 0 {
			return fmt.Errorf("interactive.inactivityTimeout must be positive when enabled, got %v", c.Interactive.InactivityTimeout)
		}
		if c.Interactive.InactivityTimeout > 30*time.Minute {
			return fmt.Errorf("interactive.inactivityTimeout must not exceed 30m, got %v", c.Interactive.InactivityTimeout)
		}
		if c.Interactive.MaxConcurrentSessions <= 0 {
			return fmt.Errorf("interactive.maxConcurrentSessions must be positive when enabled, got %d", c.Interactive.MaxConcurrentSessions)
		}
		if c.Interactive.MaxConcurrentSessions > 100 {
			return fmt.Errorf("interactive.maxConcurrentSessions must not exceed 100, got %d", c.Interactive.MaxConcurrentSessions)
		}
		if c.Interactive.RateLimitPerUser <= 0 {
			c.Interactive.RateLimitPerUser = 10
		}
		if c.Interactive.RateLimitPerUser > 100 {
			return fmt.Errorf("interactive.rateLimitPerUser must not exceed 100, got %d", c.Interactive.RateLimitPerUser)
		}
		if err := c.Interactive.validateJWTProviders(); err != nil {
			return err
		}
	}
	return nil
}

// DefaultConfig returns a Config with production defaults applied.
func DefaultConfig() *Config {
	return &Config{
		Runtime: RuntimeConfig{
			Logging: internalconfig.DefaultLoggingConfig(),
			Server: ServerConfig{
			Address: "0.0.0.0", Port: 8443, HealthAddr: ":8081", MetricsAddr: ":9090",
			DisableProfiling:      true,
			DisableAdminEndpoints: true,
			MaxConcurrentRequests: 100,
				RateLimit: RateLimitConfig{
					RequestsPerSecond: 5,
					Burst:             10,
					CleanupInterval:   5 * time.Minute,
					MaxAge:            10 * time.Minute,
				},
			},
			Session: SessionConfig{TTL: 30 * time.Minute, MaxConcurrentInvestigations: 10},
			Audit: AuditConfig{
				Enabled:    true,
				BufferSize: 100,
				BatchSize:  10,
				Verbosity:  "full",
			},
			Shutdown: ShutdownConfig{DrainSeconds: 30},
		},
		AI: AIConfig{
			LLM:           types.LLMConfig{Provider: "openai"},
			Investigation: InvestigationConfig{MaxTurns: 40},
			Summarizer: SummarizerConfig{
				Threshold:         8000,
				MaxToolOutputSize: DefaultMaxToolOutputSize,
			},
			Enrichment: EnrichmentConfig{
				MaxRetries:  3,
				BaseBackoff: 1 * time.Second,
			},
			AlignmentCheck: AlignmentCheckConfig{
				Enabled:        false,
				Mode:           AlignmentModeEnforce,
				Timeout:        10 * time.Second,
				MaxStepTokens:  500,
				MaxRetries:     1,
				VerdictTimeout: 30 * time.Second,
				Canary:         CanaryConfig{ForceEscalation: true},
				GroundingReview: GroundingReviewConfig{
					Enabled:               false,
					Timeout:               30 * time.Second,
					MaxConversationTokens: 32000,
				},
			},
			Safety: SafetyConfig{
			Sanitization: SanitizationConfig{
				InjectionPatternsEnabled: true,
				CredentialScrubEnabled:   true,
				SecretRedactionEnabled:   true,
			},
				Anomaly: AnomalyConfig{
					MaxToolCallsPerTool: 10,
					MaxTotalToolCalls:   30,
					MaxRepeatedFailures: 3,
					ExemptPrefixes:      []string{"todo_"},
				},
			},
		},
		Integrations: IntegrationsConfig{
			DataStorage: DataStorageConfig{SATokenPath: "/var/run/secrets/kubernetes.io/serviceaccount/token"},
		},
	}
}

// DefaultLLMRuntime returns an LLMRuntimeConfig with sensible production defaults.
func DefaultLLMRuntime() *LLMRuntimeConfig {
	return &LLMRuntimeConfig{
		Temperature:    0.7,
		MaxRetries:     3,
		TimeoutSeconds: 120,
	}
}

// validateTLSCertPair validates that tlsCertFile and tlsKeyFile are set
// together, use absolute paths, and require tlsCaFile (server verification
// remains mandatory per SC-8).
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
