// Package config provides file-based configuration loading for the
// kubernaut API Frontend. Configuration is read from a YAML file
// mounted via Kubernetes ConfigMap (no environment variables).
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"gopkg.in/yaml.v3"
)

// Config holds all operational configuration for the API Frontend.
type Config struct {
	Server         ServerConfig         `yaml:"server"`
	Agent          AgentConfig          `yaml:"agent"`
	MCP            MCPConfig            `yaml:"mcp"`
	AgentCard      AgentCardConfig      `yaml:"agentCard"`
	Auth           AuthConfig           `yaml:"auth"`
	Logging        LoggingConfig        `yaml:"logging"`
	RateLimit      RateLimitConfig      `yaml:"rateLimit"`
	Shutdown       ShutdownConfig       `yaml:"shutdown"`
	Resilience     ResilienceConfig     `yaml:"resilience"`
	RBAC           RBACConfig           `yaml:"rbac"`
	SeverityTriage SeverityTriageConfig `yaml:"severityTriage"`
	Session        SessionConfig        `yaml:"session"`
	Interactive    InteractiveConfig    `yaml:"interactive"`
	// Fleet holds multi-cluster federation settings (BR-FLEET-054). When
	// Enabled and MCPGatewayEndpoint is set, kubectl_get/kubectl_list/
	// list_clusters tools route to remote managed clusters via the MCP
	// Gateway. Mirrors GW/RO/SP/WE/KA's fleet.FleetConfig embedding.
	Fleet fleet.FleetConfig `yaml:"fleet"`
}

// InteractiveConfig controls whether session-dependent MCP tools are registered.
// When Enabled=false, tools that require a KA interactive session (e.g.
// kubernaut_investigate, kubernaut_message) are hidden from MCP enumeration
// and the A2A agent tool list (#1366).
type InteractiveConfig struct {
	Enabled                 bool          `yaml:"enabled"`
	AwaitSessionTimeout     time.Duration `yaml:"awaitSessionTimeout,omitempty"`
	BridgeInactivityTimeout time.Duration `yaml:"bridgeInactivityTimeout,omitempty"`
}

// SessionConfig holds InvestigationSession CRD controller settings.
type SessionConfig struct {
	Namespace     string        `yaml:"namespace"`
	DisconnectTTL time.Duration `yaml:"disconnectTTL"`
	RetentionTTL  time.Duration `yaml:"retentionTTL"`
}

// SeverityTriageConfig holds settings for the Prometheus-based severity triage pipeline.
type SeverityTriageConfig struct {
	Enabled                   bool             `yaml:"enabled"`
	LLM                       *types.LLMConfig `yaml:"llm,omitempty"`
	PrometheusURL             string           `yaml:"prometheusURL,omitempty"`
	PrometheusTLSCaFile       string           `yaml:"prometheusTlsCaFile,omitempty"`
	PrometheusBearerTokenFile string           `yaml:"prometheusBearerTokenFile,omitempty"`
	CacheTTLSeconds           int              `yaml:"cacheTTLSeconds,omitempty"`
	MaxQueriesPerCall         int              `yaml:"maxQueriesPerCall,omitempty"`
	MaxRulesEvaluated         int              `yaml:"maxRulesEvaluated,omitempty"`
	LLMConfidence             float64          `yaml:"llmConfidence,omitempty"`
}

// ResilienceConfig holds per-dependency circuit breaker and retry settings.
type ResilienceConfig struct {
	KA         DependencyConfig `yaml:"ka"`
	DS         DependencyConfig `yaml:"ds"`
	K8s        DependencyConfig `yaml:"k8s"`
	Prometheus DependencyConfig `yaml:"prometheus"`
}

// DependencyConfig holds resilience parameters for a single downstream dependency.
type DependencyConfig struct {
	ConnectTimeout     time.Duration `yaml:"connectTimeout"`
	RequestTimeout     time.Duration `yaml:"requestTimeout"`
	CBMaxRequests      uint32        `yaml:"cbMaxRequests"`
	CBInterval         time.Duration `yaml:"cbInterval"`
	CBTimeout          time.Duration `yaml:"cbTimeout"`
	CBFailureThreshold uint32        `yaml:"cbFailureThreshold"`
	RetryMax           int           `yaml:"retryMax"`
	RetryInitBackoff   time.Duration `yaml:"retryInitBackoff"`
	RetryMaxBackoff    time.Duration `yaml:"retryMaxBackoff"`
	RetryableStatuses  []int         `yaml:"retryableStatuses"`
}

// AuthConfig holds OIDC authentication settings.
// Auth mode is auto-detected from provider presence (#1309):
//   - issuerURL or jwtProviders set → OIDC/JWKS validation only
//   - both empty → K8s TokenReview only
type AuthConfig struct {
	// Legacy single-provider fields (backward compat, dev environments)
	IssuerURL              string `yaml:"issuerURL"`
	JWKSURL                string `yaml:"jwksURL,omitempty"`
	Audience               string `yaml:"audience"`
	OIDCCaFile             string `yaml:"oidcCaFile,omitempty"`
	EnableReplayProtection bool   `yaml:"enableReplayProtection,omitempty"`
	AllowInsecureIssuers   bool   `yaml:"allowInsecureIssuers,omitempty"`
	// Multi-provider JWT configuration (#1436)
	JWTProviders []JWTProviderConfig `yaml:"jwtProviders,omitempty"`
	// ReplayCache selects the jti replay-detection backend (GAP-08, #1505).
	// When set with backend "redis"/"valkey", replay detection is shared across
	// all APIFrontend replicas via Valkey — closing the HA gap of the in-memory
	// cache (a token replayed against a different replica would otherwise go
	// undetected). When nil, EnableReplayProtection controls the legacy
	// single-process in-memory cache for backward compatibility.
	ReplayCache *ReplayCacheConfig `yaml:"replayCache,omitempty"`
}

// ReplayCacheConfig configures the jti replay-detection backend. Mirrors the
// afReplayCacheYAML contract emitted by kubernaut-operator's DataStorageConfigMap
// equivalent for APIFrontend, so the same config shape works whether the
// deployment is Helm-managed or operator-managed.
type ReplayCacheConfig struct {
	// Backend is "redis" (or "valkey", equivalent) for the distributed cache, or
	// "memory"/"" for the legacy single-process in-memory cache.
	Backend string `yaml:"backend"`
	// RedisAddr is the host:port of the shared Valkey/Redis instance. Required
	// when Backend is "redis"/"valkey".
	RedisAddr string `yaml:"redisAddr,omitempty"`
	// RedisDB selects the logical Redis database index (default 0).
	RedisDB int `yaml:"redisDB,omitempty"`
	// CredentialsPath points to a YAML file containing a "password" key,
	// mounted from a Kubernetes Secret (e.g. the shared valkey-secrets.yaml
	// projection at /etc/apifrontend/valkey/valkey-secrets.yaml). Optional —
	// leave empty for an unauthenticated Valkey instance.
	CredentialsPath string `yaml:"credentialsPath,omitempty"`
}

// IsDistributed reports whether the configured backend is the distributed
// Valkey/Redis replay cache (as opposed to the legacy in-memory cache).
func (r *ReplayCacheConfig) IsDistributed() bool {
	return r != nil && (r.Backend == "redis" || r.Backend == "valkey")
}

// JWTProviderConfig defines a single JWT provider within the multi-provider
// configuration. Each entry maps to one auth.ProviderConfig at runtime.
type JWTProviderConfig struct {
	Name          string              `yaml:"name"`
	IssuerURL     string              `yaml:"issuerURL"`
	JWKSURL       string              `yaml:"jwksURL,omitempty"`
	Audiences     []string            `yaml:"audiences"`
	ClaimMappings ConfigClaimMappings `yaml:"claimMappings,omitempty"`
}

// ConfigClaimMappings defines per-provider claim mapping expressions.
type ConfigClaimMappings struct {
	Username string `yaml:"username,omitempty"`
	Groups   string `yaml:"groups,omitempty"`
}

// LoggingConfig holds structured logging settings.
type LoggingConfig struct {
	Level string `yaml:"level"`
}

// RateLimitConfig holds rate limiting thresholds.
type RateLimitConfig struct {
	IPRequestsPerSec      int `yaml:"ipRequestsPerSec"`
	UserRequestsPerSec    int `yaml:"userRequestsPerSec"`
	MaxConcurrentSessions int `yaml:"maxConcurrentSessions"`
	ToolCallsPerMinute    int `yaml:"toolCallsPerMinute"`
}

// ShutdownConfig holds graceful shutdown parameters.
type ShutdownConfig struct {
	DrainSeconds int `yaml:"drainSeconds"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port              int             `yaml:"port"`
	MetricsPort       int             `yaml:"metricsPort"`
	HealthPort        int             `yaml:"healthPort"`
	TLS               ServerTLSConfig `yaml:"tls"`
	MaxSSEConnections int             `yaml:"maxSSEConnections,omitempty"`
}

// ServerTLSConfig extends the shared TLS config with a Required flag for FedRAMP compliance.
type ServerTLSConfig struct {
	sharedtls.TLSConfig `yaml:",inline"`
	Required            bool `yaml:"required,omitempty"`
}

// AgentConfig holds ADK agent and backend connectivity settings.
type AgentConfig struct {
	KABaseURL         string          `yaml:"kaBaseURL"`
	KAMCPEndpoint     string          `yaml:"kaMCPEndpoint"`
	DSBaseURL         string          `yaml:"dsBaseURL"`
	DSBearerTokenFile string          `yaml:"dsBearerTokenFile,omitempty"`
	KABearerTokenFile string          `yaml:"kaBearerTokenFile,omitempty"`
	KATLSCaFile       string          `yaml:"kaTlsCaFile,omitempty"`
	DSTLSCaFile       string          `yaml:"dsTlsCaFile,omitempty"`
	LLM               types.LLMConfig `yaml:"llm"`
}

// MCPConfig holds Model Context Protocol feature flags.
type MCPConfig struct {
	Enabled            bool                     `yaml:"enabled"`
	SessionIdleTimeout time.Duration            `yaml:"sessionIdleTimeout,omitempty"`
	ToolTimeout        time.Duration            `yaml:"toolTimeout,omitempty"`
	ToolTimeouts       map[string]time.Duration `yaml:"toolTimeouts,omitempty"`
}

// AgentCardConfig holds the agent card endpoint configuration.
type AgentCardConfig struct {
	URL string `yaml:"url"`
}

// RBACConfig holds SAR-based RBAC authorization configuration.
type RBACConfig struct {
	// SARCacheTTL controls how long SubjectAccessReview results are cached.
	// Zero disables caching (every call hits the API server).
	SARCacheTTL time.Duration `yaml:"sarCacheTTL"`
}

// DefaultConfig returns a Config populated with production defaults.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:        8443,
			MetricsPort: 9090,
			HealthPort:  8081,
		},
		Agent: AgentConfig{
			KABaseURL:     "http://localhost:8080",
			KAMCPEndpoint: "http://localhost:8080/api/v1/mcp/",
			DSBaseURL:     "http://localhost:9090",
			LLM: types.LLMConfig{
				VertexLocation: "us-central1",
			},
		},
		MCP: MCPConfig{
			Enabled:     false,
			ToolTimeout: 30 * time.Second,
			ToolTimeouts: map[string]time.Duration{
				"kubernaut_investigate":        15 * time.Minute,
				"kubernaut_await_session":      3 * time.Minute,
				"kubernaut_watch":              15 * time.Minute,
				"kubernaut_discover_workflows": 60 * time.Second,
			},
		},
		Logging: LoggingConfig{
			Level: "INFO",
		},
		RateLimit: RateLimitConfig{
			IPRequestsPerSec:   100,
			UserRequestsPerSec: 50,
		},
		Shutdown: ShutdownConfig{
			DrainSeconds: 15,
		},
		Resilience: defaultResilienceConfig(),
		RBAC: RBACConfig{
			SARCacheTTL: 30 * time.Second,
		},
		Interactive: InteractiveConfig{
			Enabled: true,
		},
	}
}

// defaultResilienceConfig returns the per-dependency circuit-breaker/retry
// defaults for KA, DataStorage, K8s, and Prometheus clients.
func defaultResilienceConfig() ResilienceConfig {
	return ResilienceConfig{
		KA: DependencyConfig{
			ConnectTimeout:     5 * time.Second,
			RequestTimeout:     30 * time.Second,
			CBMaxRequests:      3,
			CBInterval:         10 * time.Second,
			CBTimeout:          30 * time.Second,
			CBFailureThreshold: 5,
			RetryMax:           2,
			RetryInitBackoff:   500 * time.Millisecond,
			RetryMaxBackoff:    5 * time.Second,
			RetryableStatuses:  []int{502, 503, 504},
		},
		DS: DependencyConfig{
			ConnectTimeout:     3 * time.Second,
			RequestTimeout:     10 * time.Second,
			CBMaxRequests:      3,
			CBInterval:         10 * time.Second,
			CBTimeout:          15 * time.Second,
			CBFailureThreshold: 3,
			RetryMax:           3,
			RetryInitBackoff:   200 * time.Millisecond,
			RetryMaxBackoff:    3 * time.Second,
			RetryableStatuses:  []int{502, 503, 504},
		},
		K8s: DependencyConfig{
			ConnectTimeout:     5 * time.Second,
			RequestTimeout:     30 * time.Second,
			CBMaxRequests:      3,
			CBInterval:         10 * time.Second,
			CBTimeout:          30 * time.Second,
			CBFailureThreshold: 5,
			RetryMax:           0,
			RetryableStatuses:  []int{},
		},
		Prometheus: DependencyConfig{
			ConnectTimeout: 5 * time.Second,
			RequestTimeout: 10 * time.Second,
		},
	}
}

// Load parses YAML configuration bytes into a Config struct, applying
// defaults for any omitted fields.
func Load(data []byte) (*Config, error) {
	cfg := DefaultConfig()
	if len(data) == 0 {
		return cfg, nil
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

// Validate checks required fields and value constraints. Returns the first
// validation error encountered (fail-fast).
func (c *Config) Validate() error {
	if err := c.validateServerPorts(); err != nil {
		return err
	}
	if err := c.validateAgentBaseFields(); err != nil {
		return err
	}
	for _, validate := range []func() error{
		c.validateLLM,
		c.validateAuth,
		c.validateLogging,
		c.validateRateLimit,
		c.validateShutdown,
		c.validateResilience,
		c.validateTLSPaths,
		c.validateSeverityTriage,
		c.validateSession,
		c.Fleet.Validate,
	} {
		if err := validate(); err != nil {
			return err
		}
	}
	return nil
}

// validateServerPorts checks the three HTTP listener ports are in the
// non-privileged range and mutually distinct.
func (c *Config) validateServerPorts() error {
	if c.Server.Port < 1024 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be 1024-65535 (non-privileged), got %d", c.Server.Port)
	}
	if c.Server.MetricsPort < 1024 || c.Server.MetricsPort > 65535 {
		return fmt.Errorf("server.metricsPort must be 1024-65535 (non-privileged), got %d", c.Server.MetricsPort)
	}
	if c.Server.HealthPort < 1024 || c.Server.HealthPort > 65535 {
		return fmt.Errorf("server.healthPort must be 1024-65535 (non-privileged), got %d", c.Server.HealthPort)
	}
	return validatePortsDistinct(c.Server.Port, c.Server.MetricsPort, c.Server.HealthPort)
}

// validateAgentBaseFields checks the required KA/DataStorage base URLs are
// present and well-formed, and that any configured bearer-token files exist.
func (c *Config) validateAgentBaseFields() error {
	if c.Agent.KABaseURL == "" {
		return fmt.Errorf("agent.kaBaseURL is required")
	}
	if err := validateURL("agent.kaBaseURL", c.Agent.KABaseURL); err != nil {
		return err
	}
	if c.Agent.KAMCPEndpoint == "" {
		return fmt.Errorf("agent.kaMCPEndpoint is required")
	}
	if err := validateURL("agent.kaMCPEndpoint", c.Agent.KAMCPEndpoint); err != nil {
		return err
	}
	if c.Agent.DSBaseURL == "" {
		return fmt.Errorf("agent.dsBaseURL is required")
	}
	if err := validateURL("agent.dsBaseURL", c.Agent.DSBaseURL); err != nil {
		return err
	}
	if c.Agent.DSBearerTokenFile != "" {
		if _, err := os.Stat(c.Agent.DSBearerTokenFile); err != nil {
			return fmt.Errorf("agent.dsBearerTokenFile %q is not accessible: %w", c.Agent.DSBearerTokenFile, err)
		}
	}
	if c.Agent.KABearerTokenFile != "" {
		if _, err := os.Stat(c.Agent.KABearerTokenFile); err != nil {
			return fmt.Errorf("agent.kaBearerTokenFile %q is not accessible: %w", c.Agent.KABearerTokenFile, err)
		}
	}
	return nil
}

// MinRetentionTTL is the NIST AU-11 floor for audit record retention.
const MinRetentionTTL = 30 * 24 * time.Hour

func (c *Config) validateSession() error {
	if c.Session.Namespace == "" && (c.Session.DisconnectTTL > 0 || c.Session.RetentionTTL > 0) {
		return fmt.Errorf("session.namespace must be set when session TTLs are configured")
	}
	if c.Session.RetentionTTL > 0 && c.Session.RetentionTTL < MinRetentionTTL {
		return fmt.Errorf("session.retentionTTL must be >= %s (NIST AU-11), got %s",
			MinRetentionTTL, c.Session.RetentionTTL)
	}
	return nil
}

func (c *Config) validateLLM() error {
	return validateLLMConfig("agent.llm", &c.Agent.LLM)
}

// validateLLMConfig validates an LLMConfig under the given prefix.
// Shared by agent.llm and severityTriage.llm validation paths.
// Delegates to the shared types.LLMConfig.Validate().
func validateLLMConfig(prefix string, llm *types.LLMConfig) error {
	return llm.Validate(prefix)
}

func (c *Config) validateAuth() error {
	if c.Server.TLS.Required && c.Auth.IssuerURL == "" && len(c.Auth.JWTProviders) == 0 {
		return fmt.Errorf("auth.issuerURL or auth.jwtProviders is required when server.tls.required is true (production mode)")
	}
	if err := c.validateJWTProviders(); err != nil {
		return err
	}
	if err := c.validateReplayCache(); err != nil {
		return err
	}
	if c.Auth.IssuerURL == "" {
		return nil
	}
	if err := validateURL("auth.issuerURL", c.Auth.IssuerURL); err != nil {
		return err
	}
	if c.Auth.JWKSURL != "" {
		if err := validateURL("auth.jwksURL", c.Auth.JWKSURL); err != nil {
			return err
		}
	}
	if c.Auth.OIDCCaFile != "" && !filepath.IsAbs(c.Auth.OIDCCaFile) {
		return fmt.Errorf("auth.oidcCaFile must be an absolute path, got %q", c.Auth.OIDCCaFile)
	}
	return nil
}

func (c *Config) validateReplayCache() error {
	rc := c.Auth.ReplayCache
	if rc == nil {
		return nil
	}
	switch rc.Backend {
	case "redis", "valkey":
		if rc.RedisAddr == "" {
			return fmt.Errorf("auth.replayCache.redisAddr is required when backend is %q", rc.Backend)
		}
	case "memory", "":
		// No additional fields required for the in-memory backend.
	default:
		return fmt.Errorf("auth.replayCache.backend must be one of: redis, valkey, memory; got %q", rc.Backend)
	}
	if rc.CredentialsPath != "" && !filepath.IsAbs(rc.CredentialsPath) {
		return fmt.Errorf("auth.replayCache.credentialsPath must be an absolute path, got %q", rc.CredentialsPath)
	}
	return nil
}

func (c *Config) validateJWTProviders() error {
	if len(c.Auth.JWTProviders) == 0 {
		return nil
	}
	seenNames := make(map[string]struct{}, len(c.Auth.JWTProviders))
	for i, p := range c.Auth.JWTProviders {
		prefix := fmt.Sprintf("auth.jwtProviders[%d]", i)
		if p.Name != "" {
			if _, exists := seenNames[p.Name]; exists {
				return fmt.Errorf("%s: duplicate provider name %q", prefix, p.Name)
			}
			seenNames[p.Name] = struct{}{}
		}
		if p.IssuerURL == "" {
			return fmt.Errorf("%s: issuerURL must not be empty", prefix)
		}
		if err := validateJWTProviderURL(prefix+".issuerURL", p.IssuerURL, c.Auth.AllowInsecureIssuers); err != nil {
			return err
		}
		if len(p.Audiences) == 0 {
			return fmt.Errorf("%s: audiences must not be empty (would reject all tokens)", prefix)
		}
		if p.JWKSURL != "" {
			if err := validateJWTProviderURL(prefix+".jwksURL", p.JWKSURL, c.Auth.AllowInsecureIssuers); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateJWTProviderURL(field, rawURL string, allowInsecure bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%s is not a valid URL: %w", field, err)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if allowInsecure {
			return nil
		}
		return fmt.Errorf("%s: %q uses http; https required (set allowInsecureIssuers for dev/test)", field, rawURL)
	default:
		return fmt.Errorf("%s must include a scheme (http:// or https://), got %q", field, rawURL)
	}
}

var validLogLevels = map[string]bool{
	"DEBUG": true, "INFO": true, "WARN": true, "ERROR": true,
}

func (c *Config) validateLogging() error {
	if !validLogLevels[strings.ToUpper(c.Logging.Level)] {
		return fmt.Errorf("logging.level must be one of DEBUG, INFO, WARN, ERROR; got %q", c.Logging.Level)
	}
	return nil
}

func (c *Config) validateRateLimit() error {
	if c.RateLimit.IPRequestsPerSec <= 0 {
		return fmt.Errorf("rateLimit.ipRequestsPerSec must be positive, got %d", c.RateLimit.IPRequestsPerSec)
	}
	if c.RateLimit.UserRequestsPerSec <= 0 {
		return fmt.Errorf("rateLimit.userRequestsPerSec must be positive, got %d", c.RateLimit.UserRequestsPerSec)
	}
	return nil
}

func (c *Config) validateShutdown() error {
	if c.Shutdown.DrainSeconds <= 0 {
		return fmt.Errorf("shutdown.drainSeconds must be positive, got %d", c.Shutdown.DrainSeconds)
	}
	return nil
}

func (c *Config) validateResilience() error {
	deps := []struct {
		name string
		cfg  *DependencyConfig
	}{
		{"resilience.ka", &c.Resilience.KA},
		{"resilience.ds", &c.Resilience.DS},
		{"resilience.k8s", &c.Resilience.K8s},
	}
	for _, dep := range deps {
		if err := validateDependencyConfig(dep.name, dep.cfg); err != nil {
			return err
		}
	}
	return nil
}

func validateDependencyConfig(prefix string, cfg *DependencyConfig) error {
	if cfg.ConnectTimeout < 0 {
		return fmt.Errorf("%s.connectTimeout must be non-negative, got %v", prefix, cfg.ConnectTimeout)
	}
	if cfg.RequestTimeout < 0 {
		return fmt.Errorf("%s.requestTimeout must be non-negative, got %v", prefix, cfg.RequestTimeout)
	}
	if cfg.ConnectTimeout > 0 && cfg.RequestTimeout > 0 && cfg.RequestTimeout < cfg.ConnectTimeout {
		return fmt.Errorf("%s.requestTimeout (%v) must be >= connectTimeout (%v)",
			prefix, cfg.RequestTimeout, cfg.ConnectTimeout)
	}
	if cfg.CBFailureThreshold == 0 || cfg.CBFailureThreshold > 100 {
		return fmt.Errorf("%s.cbFailureThreshold must be 1-100, got %d", prefix, cfg.CBFailureThreshold)
	}
	if cfg.RetryMax > 10 {
		return fmt.Errorf("%s.retryMax must be 0-10, got %d", prefix, cfg.RetryMax)
	}
	for _, status := range cfg.RetryableStatuses {
		if status < 400 || status > 599 {
			return fmt.Errorf("%s.retryableStatuses values must be 400-599, got %d", prefix, status)
		}
	}
	return nil
}

// ResolveDefaults fills in derived fields that depend on other config values.
// For example, AgentCard.URL is derived from Server.Port if left empty.
// LLM API key is resolved from the mounted Secret file at APIKeyFile.
// Returns an error if a required credential file is configured but unreadable/empty.
func (c *Config) ResolveDefaults() error {
	if c.AgentCard.URL == "" {
		c.AgentCard.URL = fmt.Sprintf("https://localhost:%d", c.Server.Port)
	}
	if err := resolveLLMKey(&c.Agent.LLM); err != nil {
		return fmt.Errorf("agent.llm: %w", err)
	}
	if c.SeverityTriage.LLM != nil {
		if err := resolveLLMKey(c.SeverityTriage.LLM); err != nil {
			return fmt.Errorf("severityTriage.llm: %w", err)
		}
	}
	if c.Agent.LLM.OAuth2.Enabled && c.Agent.LLM.OAuth2.CredentialsDir != "" {
		dir := c.Agent.LLM.OAuth2.CredentialsDir
		for _, f := range []string{"client-id", "client-secret"} {
			p := filepath.Join(dir, f)
			data, err := os.ReadFile(p)
			if err != nil {
				return fmt.Errorf("agent.llm.oauth2.credentialsDir/%s: %w", f, err)
			}
			if strings.TrimSpace(string(data)) == "" {
				return fmt.Errorf("agent.llm.oauth2.credentialsDir/%s: file is empty", f)
			}
		}
	}
	return nil
}

// resolveLLMKey reads the API key from the mounted secret file if configured.
func resolveLLMKey(llm *types.LLMConfig) error {
	if llm.APIKey == "" && llm.APIKeyFile != "" {
		data, err := os.ReadFile(llm.APIKeyFile)
		if err != nil {
			return fmt.Errorf("apiKeyFile %q: %w", llm.APIKeyFile, err)
		}
		key := strings.TrimSpace(string(data))
		if key == "" {
			return fmt.Errorf("apiKeyFile %q: file is empty", llm.APIKeyFile)
		}
		llm.APIKey = key
	}
	return nil
}

func (c *Config) validateSeverityTriage() error {
	st := &c.SeverityTriage
	if !st.Enabled {
		return nil
	}
	if st.PrometheusURL == "" {
		return fmt.Errorf("severityTriage.prometheusURL is required when triage is enabled")
	}
	if err := validateURL("severityTriage.prometheusURL", st.PrometheusURL); err != nil {
		return err
	}
	if st.LLMConfidence < 0 || st.LLMConfidence > 1 {
		return fmt.Errorf("severityTriage.llmConfidence must be between 0.0 and 1.0, got %v", st.LLMConfidence)
	}
	if st.PrometheusBearerTokenFile != "" {
		if _, err := os.Stat(st.PrometheusBearerTokenFile); err != nil {
			return fmt.Errorf("severityTriage.prometheusBearerTokenFile %q is not accessible: %w", st.PrometheusBearerTokenFile, err)
		}
	}
	if st.LLM != nil {
		if err := validateLLMConfig("severityTriage.llm", st.LLM); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) validateTLSPaths() error {
	paths := []struct {
		field string
		path  string
	}{
		{"agent.kaTlsCaFile", c.Agent.KATLSCaFile},
		{"agent.dsTlsCaFile", c.Agent.DSTLSCaFile},
		{"severityTriage.prometheusTlsCaFile", c.SeverityTriage.PrometheusTLSCaFile},
	}
	for _, p := range paths {
		if p.path == "" {
			continue
		}
		if _, err := os.Stat(p.path); err != nil {
			return fmt.Errorf("%s: CA file %q is not accessible: %w", p.field, p.path, err)
		}
	}
	return nil
}

func validatePortsDistinct(apiPort, metricsPort, healthPort int) error {
	if apiPort == metricsPort {
		return fmt.Errorf("server.port and server.metricsPort must be distinct, both are %d", apiPort)
	}
	if apiPort == healthPort {
		return fmt.Errorf("server.port and server.healthPort must be distinct, both are %d", apiPort)
	}
	if metricsPort == healthPort {
		return fmt.Errorf("server.metricsPort and server.healthPort must be distinct, both are %d", metricsPort)
	}
	return nil
}

func validateURL(field, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is not a valid URL: %w", field, err)
	}
	if u.Scheme == "" {
		return fmt.Errorf("%s must include a scheme (http:// or https://), got %q", field, raw)
	}
	return nil
}

// ApplyPortEnvOverride overrides cfg.Server.Port from the PORT environment
// variable when set to a valid port number (1–65535). Invalid or out-of-range
// values are silently ignored, preserving the config-file default.
func ApplyPortEnvOverride(c *Config) {
	if p := os.Getenv("PORT"); p != "" {
		if port, err := strconv.Atoi(p); err == nil && port >= 1 && port <= 65535 {
			c.Server.Port = port
		}
	}
}
