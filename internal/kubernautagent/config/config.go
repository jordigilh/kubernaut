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
	"log/slog"
	"strings"
	"time"

	pkgconfig "github.com/jordigilh/kubernaut/pkg/kubernautagent/config"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	"gopkg.in/yaml.v3"
)

// Config holds all Kubernaut Agent configuration organized into 3 concern domains.
type Config struct {
	Runtime      RuntimeConfig      `yaml:"runtime"`
	AI           AIConfig           `yaml:"ai"`
	Integrations IntegrationsConfig `yaml:"integrations"`
}

// RuntimeConfig holds operational infrastructure settings.
type RuntimeConfig struct {
	Logging LoggingConfig `yaml:"logging"`
	Server  ServerConfig  `yaml:"server"`
	Session SessionConfig `yaml:"session"`
	Audit   AuditConfig   `yaml:"audit"`
}

// AIConfig holds LLM, investigation behavior, and safety guardrails.
type AIConfig struct {
	LLM            LLMConfig            `yaml:"llm"`
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
}

// LoggingConfig holds log-level configuration (#875).
type LoggingConfig struct {
	Level string `yaml:"level"` // DEBUG, INFO, WARN, ERROR
}

// SlogLevel converts the configured level string to an slog.Level.
// Defaults to slog.LevelInfo for empty or unrecognised values.
func (l LoggingConfig) SlogLevel() slog.Level {
	switch strings.ToUpper(l.Level) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type LLMConfig struct {
	Provider         string                       `yaml:"provider"`
	Endpoint         string                       `yaml:"endpoint"`
	Model            string                       `yaml:"model"`
	APIKey           string                       `yaml:"apiKey"`
	AzureAPIVersion  string                       `yaml:"azureApiVersion"`
	VertexProject    string                       `yaml:"vertexProject"`
	VertexLocation   string                       `yaml:"vertexLocation"`
	BedrockRegion    string                       `yaml:"bedrockRegion"`
	Temperature      float64                      `yaml:"temperature"`
	MaxRetries       int                          `yaml:"maxRetries"`
	TimeoutSeconds   int                          `yaml:"timeoutSeconds"`
	StructuredOutput bool                         `yaml:"-"`
	CustomHeaders    []pkgconfig.HeaderDefinition `yaml:"-"`
	OAuth2           OAuth2Config                 `yaml:"-"`
}

// OAuth2Config holds OAuth2 client credentials configuration for enterprise
// LLM gateway authentication. When enabled, KA acquires and refreshes JWTs
// automatically via the client credentials grant (RFC 6749 s4.4).
type OAuth2Config struct {
	Enabled      bool     `yaml:"enabled"`
	TokenURL     string   `yaml:"tokenURL"`
	ClientID     string   `yaml:"clientID"`
	ClientSecret string   `yaml:"clientSecret"`
	Scopes       []string `yaml:"scopes,omitempty"`
}

type DataStorageConfig struct {
	URL         string              `yaml:"url"`
	SATokenPath string              `yaml:"saTokenPath"`
	TLS         sharedtls.TLSConfig `yaml:"tls,omitempty"`
}

type ServerConfig struct {
	Address    string              `yaml:"address"`
	Port       int                 `yaml:"port"`
	HealthAddr string              `yaml:"healthAddr"`
	MetricsAddr string             `yaml:"metricsAddr"`
	TLS        sharedtls.TLSConfig `yaml:"tls,omitempty"`
	TLSProfile string              `yaml:"tlsProfile,omitempty"`
}

type SessionConfig struct {
	TTL time.Duration `yaml:"ttl"`
}

type AuditConfig struct {
	Enabled              bool    `yaml:"enabled"`
	Endpoint             string  `yaml:"endpoint"`
	FlushIntervalSeconds float64 `yaml:"flushIntervalSeconds"`
	BufferSize           int     `yaml:"bufferSize"`
	BatchSize            int     `yaml:"batchSize"`
}

type MCPConfig struct {
	Servers []MCPServerEntry `yaml:"servers"`
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
	Prometheus PrometheusToolConfig `yaml:"prometheus"`
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
		APIKey           string                       `yaml:"apiKey"`
		AzureAPIVersion  string                       `yaml:"azureApiVersion"`
		VertexProject    string                       `yaml:"vertexProject"`
		VertexLocation   string                       `yaml:"vertexLocation"`
		BedrockRegion    string                       `yaml:"bedrockRegion"`
		MaxRetries       int                          `yaml:"maxRetries"`
		TimeoutSeconds   int                          `yaml:"timeoutSeconds"`
		Temperature      float64                      `yaml:"temperature"`
		StructuredOutput bool                         `yaml:"structuredOutput"`
		CustomHeaders    []pkgconfig.HeaderDefinition `yaml:"customHeaders,omitempty"`
		OAuth2           OAuth2Config                 `yaml:"oauth2,omitempty"`
	} `yaml:"llm"`
}

// MergeSDKConfig loads an SDK config file and merges LLM fields into
// the main config. Gap-fill semantics: the main config value is kept
// when non-zero; the SDK value fills the gap only when the main value
// is the zero value.
func (c *Config) MergeSDKConfig(data []byte) error {
	var sdk SDKConfig
	if err := yaml.Unmarshal(data, &sdk); err != nil {
		return fmt.Errorf("parsing SDK config: %w", err)
	}
	if c.AI.LLM.Provider == "" || c.AI.LLM.Provider == DefaultConfig().AI.LLM.Provider {
		if sdk.LLM.Provider != "" {
			c.AI.LLM.Provider = sdk.LLM.Provider
		}
	}
	if c.AI.LLM.Model == "" && sdk.LLM.Model != "" {
		c.AI.LLM.Model = sdk.LLM.Model
	}
	if c.AI.LLM.Endpoint == "" && sdk.LLM.Endpoint != "" {
		c.AI.LLM.Endpoint = sdk.LLM.Endpoint
	}
	if c.AI.LLM.APIKey == "" && sdk.LLM.APIKey != "" {
		c.AI.LLM.APIKey = sdk.LLM.APIKey
	}
	if c.AI.LLM.VertexProject == "" && sdk.LLM.VertexProject != "" {
		c.AI.LLM.VertexProject = sdk.LLM.VertexProject
	}
	if c.AI.LLM.VertexLocation == "" && sdk.LLM.VertexLocation != "" {
		c.AI.LLM.VertexLocation = sdk.LLM.VertexLocation
	}
	if c.AI.LLM.BedrockRegion == "" && sdk.LLM.BedrockRegion != "" {
		c.AI.LLM.BedrockRegion = sdk.LLM.BedrockRegion
	}
	if c.AI.LLM.AzureAPIVersion == "" && sdk.LLM.AzureAPIVersion != "" {
		c.AI.LLM.AzureAPIVersion = sdk.LLM.AzureAPIVersion
	}
	if c.AI.LLM.Temperature == 0 && sdk.LLM.Temperature != 0 {
		c.AI.LLM.Temperature = sdk.LLM.Temperature
	}
	if c.AI.LLM.MaxRetries == 0 && sdk.LLM.MaxRetries != 0 {
		c.AI.LLM.MaxRetries = sdk.LLM.MaxRetries
	}
	if c.AI.LLM.TimeoutSeconds == 0 && sdk.LLM.TimeoutSeconds != 0 {
		c.AI.LLM.TimeoutSeconds = sdk.LLM.TimeoutSeconds
	}
	c.AI.LLM.StructuredOutput = sdk.LLM.StructuredOutput
	c.AI.LLM.CustomHeaders = sdk.LLM.CustomHeaders
	c.AI.LLM.OAuth2 = sdk.LLM.OAuth2
	return nil
}

// Validate checks required fields and value constraints.
func (c *Config) Validate() error {
	if c.Runtime.Logging.Level != "" {
		validLevels := map[string]bool{"DEBUG": true, "INFO": true, "WARN": true, "ERROR": true}
		if !validLevels[strings.ToUpper(c.Runtime.Logging.Level)] {
			return fmt.Errorf("invalid log level: %s (must be DEBUG, INFO, WARN, or ERROR)", c.Runtime.Logging.Level)
		}
	}
	switch c.AI.LLM.Provider {
	case "bedrock", "huggingface", "anthropic", "openai", "vertex", "vertex_ai":
	default:
		if c.AI.LLM.Endpoint == "" {
			return fmt.Errorf("ai.llm.endpoint is required for provider %q", c.AI.LLM.Provider)
		}
	}
	if c.AI.LLM.Model == "" {
		return fmt.Errorf("ai.llm.model is required")
	}
	if c.AI.Investigation.MaxTurns <= 0 {
		return fmt.Errorf("ai.investigation.maxTurns must be positive, got %d", c.AI.Investigation.MaxTurns)
	}
	if c.AI.LLM.OAuth2.Enabled {
		if c.AI.LLM.OAuth2.TokenURL == "" {
			return fmt.Errorf("ai.llm.oauth2.tokenURL is required when oauth2.enabled=true")
		}
		if c.AI.LLM.OAuth2.ClientID == "" {
			return fmt.Errorf("ai.llm.oauth2.clientID is required when oauth2.enabled=true")
		}
		if c.AI.LLM.OAuth2.ClientSecret == "" {
			return fmt.Errorf("ai.llm.oauth2.clientSecret is required when oauth2.enabled=true")
		}
	}
	if c.AI.AlignmentCheck.Enabled {
		if c.AI.AlignmentCheck.Timeout <= 0 {
			return fmt.Errorf("ai.alignmentCheck.timeout must be positive when enabled, got %v", c.AI.AlignmentCheck.Timeout)
		}
		if c.AI.AlignmentCheck.MaxStepTokens <= 0 {
			return fmt.Errorf("ai.alignmentCheck.maxStepTokens must be positive when enabled, got %d", c.AI.AlignmentCheck.MaxStepTokens)
		}
		if c.AI.AlignmentCheck.LLM != nil {
			merged := c.AI.AlignmentCheck.EffectiveLLM(c.AI.LLM)
			switch merged.Provider {
			case "bedrock", "huggingface", "anthropic", "openai":
			default:
				if merged.Endpoint == "" {
					return fmt.Errorf("ai.alignmentCheck.llm.endpoint is required for provider %q", merged.Provider)
				}
			}
			if merged.Model == "" {
				return fmt.Errorf("ai.alignmentCheck.llm.model is required when alignmentCheck.llm is set")
			}
		}
	}
	return nil
}

// DefaultConfig returns a Config with production defaults applied.
func DefaultConfig() *Config {
	return &Config{
		Runtime: RuntimeConfig{
			Logging: LoggingConfig{Level: "INFO"},
			Server:  ServerConfig{Address: "0.0.0.0", Port: 8080, HealthAddr: ":8081", MetricsAddr: ":9090"},
			Session: SessionConfig{TTL: 30 * time.Minute},
			Audit:   AuditConfig{Enabled: true},
		},
		AI: AIConfig{
			LLM:           LLMConfig{Provider: "openai"},
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
				Enabled:       false,
				Timeout:       10 * time.Second,
				MaxStepTokens: 500,
			},
			Safety: SafetyConfig{
				Sanitization: SanitizationConfig{
					InjectionPatternsEnabled: true,
					CredentialScrubEnabled:   true,
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
