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
	"time"

	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
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
// GatewayType selects the discovery strategy: "eaigw" or "kuadrant".
// When GatewayType is empty, fleet is disabled regardless of Endpoint.
type FleetConfig struct {
	Endpoint       string                `yaml:"endpoint"`
	GatewayType    string                `yaml:"gatewayType"`
	OAuth2         FleetOAuth2           `yaml:"oauth2"`
	AlignmentCheck *AlignmentCheckConfig `yaml:"alignmentCheck,omitempty"`
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
	Model          string                        `yaml:"model"`
	Endpoint       string                        `yaml:"endpoint"`
	APIKeyFile     string                        `yaml:"apiKeyFile,omitempty"`
	Temperature    float64                       `yaml:"temperature"`
	MaxRetries     int                           `yaml:"maxRetries"`
	TimeoutSeconds int                           `yaml:"timeoutSeconds"`
	CustomHeaders  []types.LLMHeaderDef          `yaml:"customHeaders,omitempty"`
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
//
// Reasoning is a tuning field, not identity (#1616, BR-AI-086, DD-LLM-008):
// unlike Provider/Model, changing only a phase's Reasoning across a hot
// reload is not subject to the #1599 restart-required identity lock — see
// validatePhaseIdentity (cmd/kubernautagent/llm_builder.go), which compares
// only Provider/Model between boot and reload snapshots.
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
	if override.Reasoning != nil {
		staticOut.Reasoning = override.Reasoning
	}
	return staticOut, runtimeOut
}

type DataStorageConfig struct {
	URL            string                  `yaml:"url"`
	SATokenPath    string                  `yaml:"saTokenPath"`
	TLS            sharedtls.TLSConfig     `yaml:"tls,omitempty"`
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
	TTL                         time.Duration `yaml:"ttl"`
	MaxConcurrentInvestigations int           `yaml:"maxConcurrentInvestigations"`
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
	Enabled               bool                `yaml:"enabled"`
	SessionTTL            time.Duration       `yaml:"sessionTTL"`
	InactivityTimeout     time.Duration       `yaml:"inactivityTimeout"`
	MaxConcurrentSessions int                 `yaml:"maxConcurrentSessions"`
	RateLimitPerUser      int                 `yaml:"rateLimitPerUser"`
	MaxAnalyzingTimeout   time.Duration       `yaml:"maxAnalyzingTimeout"`
	DisconnectGracePeriod time.Duration       `yaml:"disconnectGracePeriod"`
	JWTProviders          []JWTProviderConfig `yaml:"jwtProviders,omitempty"`

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
	Name     string `yaml:"name"`
	Issuer   string `yaml:"issuer"`
	JWKSURL  string `yaml:"jwksURL"`
	Audience string `yaml:"audience"`
	// TLSCaFile is an optional path to a PEM-encoded CA bundle used to verify
	// the JWKSURL's TLS certificate when signed by a private CA (e.g. an
	// in-cluster inter-service CA). Empty uses the system trust store.
	TLSCaFile     string        `yaml:"tlsCaFile,omitempty"`
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
		if err := validateJWTProviderFields(p, name, seenIssuers); err != nil {
			return err
		}
		applyJWTClaimMappingDefaults(p)
	}
	return nil
}

// validateJWTProviderFields validates a single JWT provider entry's
// required fields, length limits, JWKS URL well-formedness, and issuer
// uniqueness against seenIssuers (which it updates on success).
func validateJWTProviderFields(p *JWTProviderConfig, name string, seenIssuers map[string]string) error {
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
	return nil
}

// applyJWTClaimMappingDefaults fills in the standard OIDC claim names for
// any claim mapping the provider config left unset.
func applyJWTClaimMappingDefaults(p *JWTProviderConfig) {
	if p.ClaimMappings.Username == "" {
		p.ClaimMappings.Username = "preferred_username"
	}
	if p.ClaimMappings.Groups == "" {
		p.ClaimMappings.Groups = "groups"
	}
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
	Enabled         bool                  `yaml:"enabled"`
	Mode            AlignmentMode         `yaml:"mode"`
	LLM             *LLMOverrideConfig    `yaml:"llm"`
	Timeout         time.Duration         `yaml:"timeout"`
	MaxStepTokens   int                   `yaml:"maxStepTokens"`
	MaxRetries      int                   `yaml:"maxRetries"`
	VerdictTimeout  time.Duration         `yaml:"verdictTimeout"`
	Canary          CanaryConfig          `yaml:"canary"`
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

// LLMOverrideConfig allows a phase (via LLMRuntimeConfig.PhaseModels) or the
// alignment checker (via AlignmentCheckConfig.LLM) to use a different LLM
// than the primary investigator. All fields are optional; non-zero fields
// override the corresponding base values.
type LLMOverrideConfig struct {
	Provider        string `yaml:"provider"`
	Endpoint        string `yaml:"endpoint"`
	Model           string `yaml:"model"`
	APIKeyFile      string `yaml:"apiKeyFile,omitempty"`
	AzureAPIVersion string `yaml:"azureApiVersion"`
	VertexProject   string `yaml:"vertexProject"`
	VertexLocation  string `yaml:"vertexLocation"`
	BedrockRegion   string `yaml:"bedrockRegion"`
	// Reasoning tunes reasoning/thinking-token behavior independently of the
	// base ai.llm.reasoning config (#1616, BR-AI-086). Nil means "inherit
	// base reasoning unchanged". Not part of LLM identity (DD-LLM-008): a
	// hot-reload changing only this field is not subject to the
	// restart-required identity lock.
	Reasoning *types.LLMReasoningConfig `yaml:"reasoning,omitempty"`
}

// EffectiveLLM returns a merged set of static + runtime fields for the
// alignment checker client builder. If override fields are set, they win.
//
// Reasoning is a tuning field, not identity (#1616, BR-AI-086, DD-LLM-008):
// the shadow/alignment-checker LLM is never subject to the #1599
// restart-required identity lock in the first place (only the primary
// investigator's per-phase identity is), so a Reasoning-only override here
// always applies immediately.
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
	if c.LLM.Reasoning != nil {
		staticOut.Reasoning = c.LLM.Reasoning
	}
	return staticOut, runtimeOut
}
