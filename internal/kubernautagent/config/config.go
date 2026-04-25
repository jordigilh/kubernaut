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
	"time"

	pkgconfig "github.com/jordigilh/kubernaut/pkg/kubernautagent/config"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	"gopkg.in/yaml.v3"
)

// Config holds all Kubernaut Agent configuration.
// Nested sub-configs support forward-compatibility with Phases 2-6
// without breaking Phase 1 tests.
type Config struct {
	LLM            LLMConfig            `yaml:"llm"`
	DataStorage    DataStorageConfig    `yaml:"data_storage"`
	Server         ServerConfig         `yaml:"server"`
	Session        SessionConfig        `yaml:"session"`
	Audit          AuditConfig          `yaml:"audit"`
	MCP            MCPConfig            `yaml:"mcp"`
	Investigator   InvestigatorConfig   `yaml:"investigator"`
	Tools          ToolsConfig          `yaml:"tools"`
	Sanitization   SanitizationConfig   `yaml:"sanitization"`
	Anomaly        AnomalyConfig        `yaml:"anomaly"`
	Summarizer     SummarizerConfig     `yaml:"summarizer"`
	Conversation   ConversationConfig   `yaml:"conversation"`
	AlignmentCheck AlignmentCheckConfig `yaml:"alignmentCheck"`
	Enrichment     EnrichmentConfig     `yaml:"enrichment"`

	// TLSProfile selects the TLS security profile (Old/Intermediate/Modern).
	// Issue #748: OCP-only — set by kubernaut-operator from the cluster APIServer CR.
	TLSProfile string `yaml:"tlsProfile,omitempty"`
}

type LLMConfig struct {
	Provider         string                       `yaml:"provider"`
	Endpoint         string                       `yaml:"endpoint"`
	Model            string                       `yaml:"model"`
	APIKey           string                       `yaml:"api_key"`
	AzureAPIVersion  string                       `yaml:"azure_api_version"`
	VertexProject    string                       `yaml:"vertex_project"`
	VertexLocation   string                       `yaml:"vertex_location"`
	BedrockRegion    string                       `yaml:"bedrock_region"`
	Temperature      float64                      `yaml:"temperature"`
	MaxRetries       int                          `yaml:"max_retries"`
	TimeoutSeconds   int                          `yaml:"timeout_seconds"`
	StructuredOutput bool                         `yaml:"-"`
	CustomHeaders    []pkgconfig.HeaderDefinition `yaml:"-"`
	OAuth2           OAuth2Config                 `yaml:"-"`
}

// OAuth2Config holds OAuth2 client credentials configuration for enterprise
// LLM gateway authentication. When enabled, KA acquires and refreshes JWTs
// automatically via the client credentials grant (RFC 6749 s4.4).
type OAuth2Config struct {
	Enabled      bool     `yaml:"enabled"`
	TokenURL     string   `yaml:"token_url"`
	ClientID     string   `yaml:"client_id"`
	ClientSecret string   `yaml:"client_secret"`
	Scopes       []string `yaml:"scopes,omitempty"`
}

type DataStorageConfig struct {
	URL         string              `yaml:"url"`
	SATokenPath string              `yaml:"sa_token_path"`
	TLS         sharedtls.TLSConfig `yaml:"tls,omitempty"`
}

type ServerConfig struct {
	Address     string              `yaml:"address"`
	Port        int                 `yaml:"port"`
	HealthAddr  string              `yaml:"health_addr"`  // Issue #753: Dedicated health probe port (default ":8081")
	MetricsAddr string              `yaml:"metrics_addr"` // Issue #753: Dedicated metrics port (default ":9090")
	TLS         sharedtls.TLSConfig `yaml:"tls,omitempty"`
}

type SessionConfig struct {
	TTL time.Duration `yaml:"ttl"`
}

type AuditConfig struct {
	Enabled              bool    `yaml:"enabled"`
	Endpoint             string  `yaml:"endpoint"`
	FlushIntervalSeconds float64 `yaml:"flush_interval_seconds"`
	BufferSize           int     `yaml:"buffer_size"`
	BatchSize            int     `yaml:"batch_size"`
}

type MCPConfig struct {
	Servers []MCPServerEntry `yaml:"servers"`
}

type MCPServerEntry struct {
	Name      string `yaml:"name"`
	URL       string `yaml:"url"`
	Transport string `yaml:"transport"`
}

type InvestigatorConfig struct {
	MaxTurns int `yaml:"max_turns"`
}

type ToolsConfig struct {
	Prometheus PrometheusToolConfig `yaml:"prometheus"`
}

type PrometheusToolConfig struct {
	URL       string        `yaml:"url"`
	Timeout   time.Duration `yaml:"timeout"`
	SizeLimit int           `yaml:"size_limit"`
	TLSCaFile string        `yaml:"tls_ca_file"`
}

type SanitizationConfig struct {
	InjectionPatternsEnabled  bool `yaml:"injection_patterns_enabled"`
	CredentialScrubEnabled    bool `yaml:"credential_scrub_enabled"`
}

// DefaultMaxToolOutputSize is the default hard character limit for tool output
// before it enters the LLM context window. ~25K tokens for most models.
const DefaultMaxToolOutputSize = 100000

type SummarizerConfig struct {
	Threshold         int `yaml:"threshold"`
	MaxToolOutputSize int `yaml:"max_tool_output_size"`
}

// EnrichmentConfig controls retry behavior for K8s owner chain resolution
// during enrichment. HAPI-aligned defaults (MaxRetries=3, BaseBackoff=1s)
// ensure rca_incomplete is triggered on definitive enrichment failure.
type EnrichmentConfig struct {
	MaxRetries  int           `yaml:"max_retries"`
	BaseBackoff time.Duration `yaml:"base_backoff"`
}

type AnomalyConfig struct {
	MaxToolCallsPerTool int      `yaml:"max_tool_calls_per_tool"`
	MaxTotalToolCalls   int      `yaml:"max_total_tool_calls"`
	MaxRepeatedFailures int      `yaml:"max_repeated_failures"`
	ExemptPrefixes      []string `yaml:"exempt_prefixes"`
}

// ConversationConfig holds settings for the conversational RAR API (#592).
type ConversationConfig struct {
	Enabled      bool                      `yaml:"enabled"`
	LLM          *LLMConfig                `yaml:"llm"`
	Session      ConversationSessionConfig `yaml:"session"`
	RateLimit    RateLimitConfig           `yaml:"rate_limit"`
	MaxToolTurns int                       `yaml:"max_tool_turns"` // DD-CONV-001: bounded tool-call loop iterations (default 15)
}

// ConversationSessionConfig controls conversation session behavior.
type ConversationSessionConfig struct {
	TTL      time.Duration `yaml:"ttl"`
	MaxTurns int           `yaml:"max_turns"`
}

// RateLimitConfig controls per-user and per-session rate limits.
type RateLimitConfig struct {
	PerUserPerMinute int `yaml:"per_user_per_minute"`
	PerSession       int `yaml:"per_session"`
}

// AlignmentCheckConfig holds settings for the shadow agent alignment checker (#601).
type AlignmentCheckConfig struct {
	Enabled       bool          `yaml:"enabled"`
	LLM           *LLMConfig    `yaml:"llm"`
	Timeout       time.Duration `yaml:"timeout"`
	MaxStepTokens int           `yaml:"maxStepTokens"`
}

// mergeLLMConfig overlays non-zero fields from override onto base and returns the result.
func mergeLLMConfig(base LLMConfig, override *LLMConfig) LLMConfig {
	if override == nil {
		return base
	}
	merged := base
	if override.Provider != "" {
		merged.Provider = override.Provider
	}
	if override.Endpoint != "" {
		merged.Endpoint = override.Endpoint
	}
	if override.Model != "" {
		merged.Model = override.Model
	}
	if override.APIKey != "" {
		merged.APIKey = override.APIKey
	}
	if override.AzureAPIVersion != "" {
		merged.AzureAPIVersion = override.AzureAPIVersion
	}
	if override.VertexProject != "" {
		merged.VertexProject = override.VertexProject
	}
	if override.VertexLocation != "" {
		merged.VertexLocation = override.VertexLocation
	}
	if override.BedrockRegion != "" {
		merged.BedrockRegion = override.BedrockRegion
	}
	return merged
}

// EffectiveLLM returns a merged LLM config for the alignment checker.
func (c *AlignmentCheckConfig) EffectiveLLM(base LLMConfig) LLMConfig {
	return mergeLLMConfig(base, c.LLM)
}

// EffectiveLLM returns a merged LLM config for the conversation subsystem.
func (c *ConversationConfig) EffectiveLLM(base LLMConfig) LLMConfig {
	return mergeLLMConfig(base, c.LLM)
}

// Load parses configuration from YAML bytes and applies defaults.
func Load(data []byte) (*Config, error) {
	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

// SDKConfig represents the SDK configuration file structure.
// This maps to the kubernaut-agent-sdk-config ConfigMap rendered by
// the Helm chart when llm.provider/model are set. All LLM fields
// use gap-fill semantics (main config wins when non-zero) except
// transport fields (structured_output, custom_headers, oauth2) which
// are sourced exclusively from this config.
type SDKConfig struct {
	LLM struct {
		Provider         string                       `yaml:"provider"`
		Model            string                       `yaml:"model"`
		Endpoint         string                       `yaml:"endpoint"`
		APIKey           string                       `yaml:"api_key"`
		AzureAPIVersion  string                       `yaml:"azure_api_version"`
		VertexProject    string                       `yaml:"vertex_project"`
		VertexLocation   string                       `yaml:"vertex_location"`
		BedrockRegion    string                       `yaml:"bedrock_region"`
		MaxRetries       int                          `yaml:"max_retries"`
		TimeoutSeconds   int                          `yaml:"timeout_seconds"`
		Temperature      float64                      `yaml:"temperature"`
		StructuredOutput bool                         `yaml:"structured_output"`
		CustomHeaders    []pkgconfig.HeaderDefinition `yaml:"custom_headers,omitempty"`
		OAuth2           OAuth2Config                 `yaml:"oauth2,omitempty"`
	} `yaml:"llm"`
}

// MergeSDKConfig loads an SDK config file and merges LLM fields into
// the main config. Gap-fill semantics: the main config value is kept
// when non-zero; the SDK value fills the gap only when the main value
// is the zero value. Gap-fill fields: provider (special: also replaces
// the default "openai"), model, endpoint, api_key, vertex_project,
// vertex_location, bedrock_region, azure_api_version, temperature,
// max_retries, timeout_seconds. Override fields (SDK always wins):
// structured_output, custom_headers, oauth2.
func (c *Config) MergeSDKConfig(data []byte) error {
	var sdk SDKConfig
	if err := yaml.Unmarshal(data, &sdk); err != nil {
		return fmt.Errorf("parsing SDK config: %w", err)
	}
	if c.LLM.Provider == "" || c.LLM.Provider == DefaultConfig().LLM.Provider {
		if sdk.LLM.Provider != "" {
			c.LLM.Provider = sdk.LLM.Provider
		}
	}
	if c.LLM.Model == "" && sdk.LLM.Model != "" {
		c.LLM.Model = sdk.LLM.Model
	}
	if c.LLM.Endpoint == "" && sdk.LLM.Endpoint != "" {
		c.LLM.Endpoint = sdk.LLM.Endpoint
	}
	if c.LLM.APIKey == "" && sdk.LLM.APIKey != "" {
		c.LLM.APIKey = sdk.LLM.APIKey
	}
	if c.LLM.VertexProject == "" && sdk.LLM.VertexProject != "" {
		c.LLM.VertexProject = sdk.LLM.VertexProject
	}
	if c.LLM.VertexLocation == "" && sdk.LLM.VertexLocation != "" {
		c.LLM.VertexLocation = sdk.LLM.VertexLocation
	}
	if c.LLM.BedrockRegion == "" && sdk.LLM.BedrockRegion != "" {
		c.LLM.BedrockRegion = sdk.LLM.BedrockRegion
	}
	if c.LLM.AzureAPIVersion == "" && sdk.LLM.AzureAPIVersion != "" {
		c.LLM.AzureAPIVersion = sdk.LLM.AzureAPIVersion
	}
	if c.LLM.Temperature == 0 && sdk.LLM.Temperature != 0 {
		c.LLM.Temperature = sdk.LLM.Temperature
	}
	if c.LLM.MaxRetries == 0 && sdk.LLM.MaxRetries != 0 {
		c.LLM.MaxRetries = sdk.LLM.MaxRetries
	}
	if c.LLM.TimeoutSeconds == 0 && sdk.LLM.TimeoutSeconds != 0 {
		c.LLM.TimeoutSeconds = sdk.LLM.TimeoutSeconds
	}
	c.LLM.StructuredOutput = sdk.LLM.StructuredOutput
	c.LLM.CustomHeaders = sdk.LLM.CustomHeaders
	c.LLM.OAuth2 = sdk.LLM.OAuth2
	return nil
}

// Validate checks required fields and value constraints.
func (c *Config) Validate() error {
	switch c.LLM.Provider {
	case "bedrock", "huggingface", "anthropic", "openai", "vertex", "vertex_ai":
		// endpoint is optional: LangChainGo uses default endpoints or project/location for these providers
	default:
		if c.LLM.Endpoint == "" {
			return fmt.Errorf("llm.endpoint is required for provider %q", c.LLM.Provider)
		}
	}
	if c.LLM.Model == "" {
		return fmt.Errorf("llm.model is required")
	}
	if c.Investigator.MaxTurns <= 0 {
		return fmt.Errorf("investigator.max_turns must be positive, got %d", c.Investigator.MaxTurns)
	}
	if c.LLM.OAuth2.Enabled {
		if c.LLM.OAuth2.TokenURL == "" {
			return fmt.Errorf("llm.oauth2.token_url is required when oauth2.enabled=true")
		}
		if c.LLM.OAuth2.ClientID == "" {
			return fmt.Errorf("llm.oauth2.client_id is required when oauth2.enabled=true")
		}
		if c.LLM.OAuth2.ClientSecret == "" {
			return fmt.Errorf("llm.oauth2.client_secret is required when oauth2.enabled=true")
		}
	}
	if c.AlignmentCheck.Enabled {
		if c.AlignmentCheck.Timeout <= 0 {
			return fmt.Errorf("alignmentCheck.timeout must be positive when enabled, got %v", c.AlignmentCheck.Timeout)
		}
		if c.AlignmentCheck.MaxStepTokens <= 0 {
			return fmt.Errorf("alignmentCheck.maxStepTokens must be positive when enabled, got %d", c.AlignmentCheck.MaxStepTokens)
		}
		if c.AlignmentCheck.LLM != nil {
			merged := c.AlignmentCheck.EffectiveLLM(c.LLM)
			switch merged.Provider {
			case "bedrock", "huggingface", "anthropic", "openai":
			default:
				if merged.Endpoint == "" {
					return fmt.Errorf("alignmentCheck.llm.endpoint is required for provider %q", merged.Provider)
				}
			}
			if merged.Model == "" {
				return fmt.Errorf("alignmentCheck.llm.model is required when alignmentCheck.llm is set")
			}
		}
	}
	return nil
}

// DefaultConfig returns a Config with production defaults applied.
func DefaultConfig() *Config {
	return &Config{
		LLM:          LLMConfig{Provider: "openai"},
		DataStorage:  DataStorageConfig{SATokenPath: "/var/run/secrets/kubernetes.io/serviceaccount/token"},
		Server:       ServerConfig{Address: "0.0.0.0", Port: 8080, HealthAddr: ":8081", MetricsAddr: ":9090"},
		Session:      SessionConfig{TTL: 30 * time.Minute},
		Investigator: InvestigatorConfig{MaxTurns: 15},
		Audit:        AuditConfig{Enabled: true},
		Anomaly: AnomalyConfig{
			MaxToolCallsPerTool: 5,
			MaxTotalToolCalls:   30,
			MaxRepeatedFailures: 3,
			ExemptPrefixes:      []string{"todo_"},
		},
		Conversation: ConversationConfig{
			Session:      ConversationSessionConfig{TTL: 30 * time.Minute, MaxTurns: 30},
			RateLimit:    RateLimitConfig{PerUserPerMinute: 10, PerSession: 30},
			MaxToolTurns: 15,
		},
		Sanitization: SanitizationConfig{
			InjectionPatternsEnabled: true,
			CredentialScrubEnabled:   true,
		},
		Summarizer: SummarizerConfig{
			Threshold:         8000,
			MaxToolOutputSize: DefaultMaxToolOutputSize,
		},
		AlignmentCheck: AlignmentCheckConfig{
			Enabled:       false,
			Timeout:       10 * time.Second,
			MaxStepTokens: 500,
		},
		Enrichment: EnrichmentConfig{
			MaxRetries:  3,
			BaseBackoff: 1 * time.Second,
		},
	}
}
