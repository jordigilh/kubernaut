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
	"os"
	"path/filepath"
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

// LLMConfig holds static LLM provider settings that require a pod restart to change.
type LLMConfig struct {
	Provider        string       `yaml:"provider"`
	AzureAPIVersion string       `yaml:"azureApiVersion"`
	VertexProject   string       `yaml:"vertexProject"`
	VertexLocation  string       `yaml:"vertexLocation"`
	BedrockRegion   string       `yaml:"bedrockRegion"`
	OAuth2          OAuth2Config `yaml:"oauth2,omitempty"`
}

// LLMRuntimeConfig holds hot-reloadable LLM settings that can change without restart.
// This struct maps to a separate ConfigMap (kubernaut-agent-llm-runtime) watched by
// the FileWatcher.
type LLMRuntimeConfig struct {
	Model          string                       `yaml:"model"`
	Endpoint       string                       `yaml:"endpoint"`
	APIKey         string                       `yaml:"apiKey"`
	Temperature    float64                      `yaml:"temperature"`
	MaxRetries     int                          `yaml:"maxRetries"`
	TimeoutSeconds int                          `yaml:"timeoutSeconds"`
	CustomHeaders  []pkgconfig.HeaderDefinition `yaml:"customHeaders,omitempty"`
}

// OAuth2Config holds OAuth2 client credentials configuration for enterprise
// LLM gateway authentication. When enabled, KA acquires and refreshes JWTs
// automatically via the client credentials grant (RFC 6749 s4.4).
//
// Security: clientID and clientSecret are resolved from mounted Secret files
// at runtime (not stored in ConfigMap). Only tokenURL, scopes, and
// credentialsDir are configured via YAML.
type OAuth2Config struct {
	Enabled        bool     `yaml:"enabled"`
	TokenURL       string   `yaml:"tokenURL"`
	Scopes         []string `yaml:"scopes,omitempty"`
	CredentialsDir string   `yaml:"credentialsDir"`
	ClientID       string   `yaml:"-"`
	ClientSecret   string   `yaml:"-"`
}

// ResolveOAuth2Credentials reads clientID and clientSecret from mounted
// Secret files in the configured credentialsDir. Expected file layout:
//
//	<credentialsDir>/client-id
//	<credentialsDir>/client-secret
func (c *OAuth2Config) ResolveOAuth2Credentials() error {
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

func readSecretFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(data))
	if v == "" {
		return "", fmt.Errorf("file %s is empty", path)
	}
	return v, nil
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
	Enabled       bool               `yaml:"enabled"`
	LLM           *LLMOverrideConfig `yaml:"llm"`
	Timeout       time.Duration      `yaml:"timeout"`
	MaxStepTokens int                `yaml:"maxStepTokens"`
}

// LLMOverrideConfig allows the alignment checker to use a different LLM than
// the primary investigator. All fields are optional; non-zero fields override
// the corresponding base values.
type LLMOverrideConfig struct {
	Provider        string `yaml:"provider"`
	Endpoint        string `yaml:"endpoint"`
	Model           string `yaml:"model"`
	APIKey          string `yaml:"apiKey"`
	AzureAPIVersion string `yaml:"azureApiVersion"`
	VertexProject   string `yaml:"vertexProject"`
	VertexLocation  string `yaml:"vertexLocation"`
	BedrockRegion   string `yaml:"bedrockRegion"`
}

// EffectiveLLM returns a merged set of static + runtime fields for the
// alignment checker client builder. If override fields are set, they win.
func (c *AlignmentCheckConfig) EffectiveLLM(base LLMConfig, runtime LLMRuntimeConfig) (LLMConfig, LLMRuntimeConfig) {
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
	if c.LLM.APIKey != "" {
		runtimeOut.APIKey = c.LLM.APIKey
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
	return nil
}

// Validate checks required fields and value constraints for the static config.
// Runtime LLM fields are validated separately via LLMRuntimeConfig.Validate().
func (c *Config) Validate() error {
	if c.Runtime.Logging.Level != "" {
		validLevels := map[string]bool{"DEBUG": true, "INFO": true, "WARN": true, "ERROR": true}
		if !validLevels[strings.ToUpper(c.Runtime.Logging.Level)] {
			return fmt.Errorf("invalid log level: %s (must be DEBUG, INFO, WARN, or ERROR)", c.Runtime.Logging.Level)
		}
	}
	if c.AI.Investigation.MaxTurns <= 0 {
		return fmt.Errorf("ai.investigation.maxTurns must be positive, got %d", c.AI.Investigation.MaxTurns)
	}
	if c.AI.LLM.OAuth2.Enabled {
		if c.AI.LLM.OAuth2.TokenURL == "" {
			return fmt.Errorf("ai.llm.oauth2.tokenURL is required when oauth2.enabled=true")
		}
		if c.AI.LLM.OAuth2.CredentialsDir == "" {
			return fmt.Errorf("ai.llm.oauth2.credentialsDir is required when oauth2.enabled=true")
		}
	}
	if c.AI.AlignmentCheck.Enabled {
		if c.AI.AlignmentCheck.Timeout <= 0 {
			return fmt.Errorf("ai.alignmentCheck.timeout must be positive when enabled, got %v", c.AI.AlignmentCheck.Timeout)
		}
		if c.AI.AlignmentCheck.MaxStepTokens <= 0 {
			return fmt.Errorf("ai.alignmentCheck.maxStepTokens must be positive when enabled, got %d", c.AI.AlignmentCheck.MaxStepTokens)
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

// DefaultLLMRuntime returns an LLMRuntimeConfig with sensible production defaults.
func DefaultLLMRuntime() *LLMRuntimeConfig {
	return &LLMRuntimeConfig{
		Temperature:    0.7,
		MaxRetries:     3,
		TimeoutSeconds: 120,
	}
}
