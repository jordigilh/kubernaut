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

	"gopkg.in/yaml.v3"
)

// Config holds all Kubernaut Agent configuration.
// Nested sub-configs support forward-compatibility with Phases 2-6
// without breaking Phase 1 tests.
type Config struct {
	LLM           LLMConfig           `yaml:"llm"`
	DataStorage   DataStorageConfig   `yaml:"data_storage"`
	Server        ServerConfig        `yaml:"server"`
	Session       SessionConfig       `yaml:"session"`
	Audit         AuditConfig         `yaml:"audit"`
	MCP           MCPConfig           `yaml:"mcp"`
	Investigator  InvestigatorConfig  `yaml:"investigator"`
	Tools         ToolsConfig         `yaml:"tools"`
	Sanitization  SanitizationConfig  `yaml:"sanitization"`
	Anomaly       AnomalyConfig       `yaml:"anomaly"`
	Summarizer    SummarizerConfig    `yaml:"summarizer"`
}

type LLMConfig struct {
	Provider        string `yaml:"provider"`
	Endpoint        string `yaml:"endpoint"`
	Model           string `yaml:"model"`
	APIKey          string `yaml:"api_key"`
	AzureAPIVersion string `yaml:"azure_api_version"`
	VertexProject   string `yaml:"vertex_project"`
	VertexLocation  string `yaml:"vertex_location"`
	BedrockRegion   string `yaml:"bedrock_region"`
}

type DataStorageConfig struct {
	URL         string `yaml:"url"`
	SATokenPath string `yaml:"sa_token_path"`
}

type ServerConfig struct {
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
}

type SessionConfig struct {
	TTL time.Duration `yaml:"ttl"`
}

type AuditConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Endpoint string `yaml:"endpoint"`
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
}

type SanitizationConfig struct {
	InjectionPatternsEnabled  bool `yaml:"injection_patterns_enabled"`
	CredentialScrubEnabled    bool `yaml:"credential_scrub_enabled"`
}

type SummarizerConfig struct {
	Threshold int `yaml:"threshold"`
}

type AnomalyConfig struct {
	MaxToolCallsPerTool int `yaml:"max_tool_calls_per_tool"`
	MaxTotalToolCalls   int `yaml:"max_total_tool_calls"`
	MaxRepeatedFailures int `yaml:"max_repeated_failures"`
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
// the Helm chart when llm.provider/model are set.
type SDKConfig struct {
	LLM struct {
		Provider       string  `yaml:"provider"`
		Model          string  `yaml:"model"`
		MaxRetries     int     `yaml:"max_retries"`
		TimeoutSeconds int     `yaml:"timeout_seconds"`
		Temperature    float64 `yaml:"temperature"`
	} `yaml:"llm"`
}

// MergeSDKConfig loads an SDK config file and merges LLM fields into
// the main config. SDK values are only applied when the main config
// doesn't already specify them (main config takes precedence).
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
	return nil
}

// Validate checks required fields and value constraints.
func (c *Config) Validate() error {
	switch c.LLM.Provider {
	case "bedrock", "huggingface", "anthropic", "openai":
		// endpoint is optional: LangChainGo uses default endpoints for these providers
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
	return nil
}

// DefaultConfig returns a Config with production defaults applied.
func DefaultConfig() *Config {
	return &Config{
		LLM:          LLMConfig{Provider: "openai"},
		DataStorage:  DataStorageConfig{SATokenPath: "/var/run/secrets/kubernetes.io/serviceaccount/token"},
		Server:       ServerConfig{Address: "0.0.0.0", Port: 8080},
		Session:      SessionConfig{TTL: 30 * time.Minute},
		Investigator: InvestigatorConfig{MaxTurns: 15},
		Audit:        AuditConfig{Enabled: true},
		Anomaly: AnomalyConfig{
			MaxToolCallsPerTool: 5,
			MaxTotalToolCalls:   30,
			MaxRepeatedFailures: 3,
		},
		Sanitization: SanitizationConfig{
			InjectionPatternsEnabled: true,
			CredentialScrubEnabled:   true,
		},
		Summarizer: SummarizerConfig{
			Threshold: 8000,
		},
	}
}
