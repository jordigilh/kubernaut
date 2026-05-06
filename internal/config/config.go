// Package config provides file-based configuration loading for the
// kubernaut API Frontend. Configuration is read from a YAML file
// mounted via Kubernetes ConfigMap (no environment variables).
package config

import (
	"fmt"
	"net/url"

	"gopkg.in/yaml.v3"
)

// Config holds all operational configuration for the API Frontend.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Agent     AgentConfig     `yaml:"agent"`
	MCP       MCPConfig       `yaml:"mcp"`
	AgentCard AgentCardConfig `yaml:"agentCard"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port int `yaml:"port"`
}

// AgentConfig holds ADK agent and backend connectivity settings.
type AgentConfig struct {
	GCPProject    string `yaml:"gcpProject"`
	GCPRegion     string `yaml:"gcpRegion"`
	KABaseURL     string `yaml:"kaBaseURL"`
	KAMCPEndpoint string `yaml:"kaMCPEndpoint"`
	DSBaseURL     string `yaml:"dsBaseURL"`
}

// MCPConfig holds Model Context Protocol feature flags.
type MCPConfig struct {
	Enabled bool `yaml:"enabled"`
}

// AgentCardConfig holds the agent card endpoint configuration.
type AgentCardConfig struct {
	URL string `yaml:"url"`
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
	return nil
}

// ResolveDefaults fills in derived fields that depend on other config values.
// For example, AgentCard.URL is derived from Server.Port if left empty.
func (c *Config) ResolveDefaults() {
	if c.AgentCard.URL == "" {
		c.AgentCard.URL = fmt.Sprintf("https://localhost:%d", c.Server.Port)
	}
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
