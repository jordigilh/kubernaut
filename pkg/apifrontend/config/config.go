// Package config provides file-based configuration loading for the
// kubernaut API Frontend. Configuration is read from a YAML file
// mounted via Kubernetes ConfigMap (no environment variables).
package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
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
}

// SessionConfig holds InvestigationSession CRD controller settings.
type SessionConfig struct {
	Namespace     string        `yaml:"namespace"`
	DisconnectTTL time.Duration `yaml:"disconnectTTL"`
	RetentionTTL  time.Duration `yaml:"retentionTTL"`
}

// SeverityTriageConfig holds settings for the Prometheus-based severity triage pipeline.
type SeverityTriageConfig struct {
	Enabled                   bool    `yaml:"enabled"`
	PrometheusURL             string  `yaml:"prometheusURL,omitempty"`
	PrometheusTLSCaFile       string  `yaml:"prometheusTlsCaFile,omitempty"`
	PrometheusBearerTokenFile string  `yaml:"prometheusBearerTokenFile,omitempty"`
	CacheTTLSeconds           int     `yaml:"cacheTTLSeconds,omitempty"`
	MaxQueriesPerCall         int     `yaml:"maxQueriesPerCall,omitempty"`
	MaxRulesEvaluated         int     `yaml:"maxRulesEvaluated,omitempty"`
	LLMConfidence             float64 `yaml:"llmConfidence,omitempty"`
}

// ResilienceConfig holds per-dependency circuit breaker and retry settings.
type ResilienceConfig struct {
	KA  DependencyConfig `yaml:"ka"`
	DS  DependencyConfig `yaml:"ds"`
	K8s DependencyConfig `yaml:"k8s"`
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
type AuthConfig struct {
	IssuerURL              string `yaml:"issuerURL"`
	JWKSURL                string `yaml:"jwksURL,omitempty"`
	Audience               string `yaml:"audience"`
	EnableReplayProtection bool   `yaml:"enableReplayProtection,omitempty"`
	AllowInsecureIssuers   bool   `yaml:"allowInsecureIssuers,omitempty"`
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
	GCPProject        string `yaml:"gcpProject"`
	GCPRegion         string `yaml:"gcpRegion"`
	KABaseURL         string `yaml:"kaBaseURL"`
	KAMCPEndpoint     string `yaml:"kaMCPEndpoint"`
	DSBaseURL         string `yaml:"dsBaseURL"`
	DSBearerTokenFile string `yaml:"dsBearerTokenFile,omitempty"`
	KATLSCaFile       string `yaml:"kaTlsCaFile,omitempty"`
	DSTLSCaFile       string `yaml:"dsTlsCaFile,omitempty"`
	// LLMEndpoint is the base URL of a Gemini-compatible LLM endpoint.
	// When set, AF wires the A2A handler with a real ADK agent backed by this
	// endpoint. When empty, POST /a2a/invoke returns 501.
	LLMEndpoint string `yaml:"llmEndpoint,omitempty"`
	// LLMModel is the model name passed in generateContent requests.
	LLMModel string `yaml:"llmModel,omitempty"`
	// LLMAPIKey is the API key for the LLM endpoint, populated from the
	// LLM_API_KEY environment variable during ResolveDefaults. Never persisted
	// in config files.
	LLMAPIKey string `yaml:"-"`
}

// MCPConfig holds Model Context Protocol feature flags.
type MCPConfig struct {
	Enabled            bool          `yaml:"enabled"`
	SessionIdleTimeout time.Duration `yaml:"sessionIdleTimeout,omitempty"`
}

// AgentCardConfig holds the agent card endpoint configuration.
type AgentCardConfig struct {
	URL string `yaml:"url"`
}

// RBACConfig holds RBAC-related configuration for the Agent Card.
type RBACConfig struct {
	// GroupMapping maps OIDC group names to AF role keys (defined in rbac_roles.yaml).
	// When configured, the Agent Card handler uses this to filter skills per caller.
	GroupMapping map[string]string `yaml:"groupMapping"`
}

// DefaultConfig returns a Config populated with production defaults.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 8443,
		},
		Agent: AgentConfig{
			GCPRegion:     "us-central1",
			KABaseURL:     "http://localhost:8080",
			KAMCPEndpoint: "http://localhost:8080/api/v1/mcp/",
			DSBaseURL:     "http://localhost:9090",
		},
		MCP: MCPConfig{
			Enabled: false,
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
		Resilience: ResilienceConfig{
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
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be 1-65535, got %d", c.Server.Port)
	}
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
	if err := c.validateAuth(); err != nil {
		return err
	}
	if err := c.validateLogging(); err != nil {
		return err
	}
	if err := c.validateRateLimit(); err != nil {
		return err
	}
	if err := c.validateShutdown(); err != nil {
		return err
	}
	if err := c.validateResilience(); err != nil {
		return err
	}
	if err := c.validateTLSPaths(); err != nil {
		return err
	}
	if err := c.validateSeverityTriage(); err != nil {
		return err
	}
	if err := c.validateSession(); err != nil {
		return err
	}
	return nil
}

func (c *Config) validateSession() error {
	if c.Session.Namespace == "" && (c.Session.DisconnectTTL > 0 || c.Session.RetentionTTL > 0) {
		return fmt.Errorf("session.namespace must be set when session TTLs are configured")
	}
	return nil
}

func (c *Config) validateAuth() error {
	if c.Server.TLS.Required && c.Auth.IssuerURL == "" {
		return fmt.Errorf("auth.issuerURL is required when server.tls.required is true (production mode)")
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
	return nil
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
// LLMAPIKey is populated from the LLM_API_KEY environment variable (never
// persisted in config files — secrets stay out of YAML).
func (c *Config) ResolveDefaults() {
	if c.AgentCard.URL == "" {
		c.AgentCard.URL = fmt.Sprintf("https://localhost:%d", c.Server.Port)
	}
	if c.Agent.LLMAPIKey == "" {
		c.Agent.LLMAPIKey = os.Getenv("LLM_API_KEY")
	}
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
