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

// Package config implements ADR-030 compliant YAML configuration for the FMC service.
// Field names use camelCase per CRD_FIELD_NAMING_CONVENTION.md V1.1.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultConfigPath is the standard Kubernetes ConfigMap mount path for FMC.
// ADR-030: All services MUST use /etc/{service}/config.yaml as the default.
const DefaultConfigPath = "/etc/fleetmetadatacache/config.yaml"

// ServiceConfig is the top-level configuration for the FMC service.
type ServiceConfig struct {
	Server     ServerConfig     `yaml:"server"`
	MCPGateway MCPGatewayConfig `yaml:"mcpGateway"`
	Valkey     ValkeyConfig     `yaml:"valkey"`
	Sync       SyncConfig       `yaml:"sync"`
	OAuth2     OAuth2Config     `yaml:"oauth2"`
}

// ServerConfig contains HTTP server settings.
type ServerConfig struct {
	APIAddr     string `yaml:"apiAddr"`
	MetricsAddr string `yaml:"metricsAddr"`
}

// MCPGatewayConfig contains MCP Gateway connectivity.
type MCPGatewayConfig struct {
	Endpoint    string `yaml:"endpoint"`
	GatewayType string `yaml:"gatewayType"`
	Namespace   string `yaml:"namespace"`
}

// ValkeyConfig contains Valkey cache connectivity.
type ValkeyConfig struct {
	Addr string `yaml:"addr"`
}

// SyncConfig contains syncer timing and resource settings.
type SyncConfig struct {
	Interval      time.Duration `yaml:"interval"`
	KeyTTL        time.Duration `yaml:"keyTtl"`
	ResourceKinds []string      `yaml:"resourceKinds"`
}

// OAuth2Config contains OAuth2 client_credentials settings.
// OAuth2 is mandatory for FMC — the MCP Gateway always requires authentication
// in production fleet deployments. There is no unauthenticated fallback.
type OAuth2Config struct {
	TokenURL       string        `yaml:"tokenUrl"`
	CredentialsDir string        `yaml:"credentialsDir"`
	Scopes         []string      `yaml:"scopes"`
	TokenTimeout   time.Duration `yaml:"tokenTimeout"`
	TlsCaFile      string        `yaml:"tlsCaFile"`
}

// DefaultServiceConfig returns production defaults.
func DefaultServiceConfig() *ServiceConfig {
	return &ServiceConfig{
		Server: ServerConfig{
			APIAddr:     ":8080",
			MetricsAddr: ":8081",
		},
		MCPGateway: MCPGatewayConfig{
			GatewayType: "eaigw",
			Namespace:   "kubernaut-system",
		},
		Valkey: ValkeyConfig{
			Addr: "valkey:6379",
		},
		Sync: SyncConfig{
			Interval:      30 * time.Second,
			KeyTTL:        45 * time.Second,
			ResourceKinds: []string{"Deployment", "StatefulSet", "DaemonSet", "Pod", "Service", "Node"},
		},
		OAuth2: OAuth2Config{
			CredentialsDir: "/etc/fleetmetadatacache/fleet-oauth2",
			Scopes:         []string{"openid", "groups"},
			TokenTimeout:   10 * time.Second,
		},
	}
}

// LoadFromFile reads a YAML config file and unmarshals it into ServiceConfig.
// ADR-030: All services MUST use this pattern for configuration loading.
func LoadFromFile(path string) (*ServiceConfig, error) {
	cfg := DefaultServiceConfig()

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	return cfg, nil
}

// Validate checks required fields and returns an error if any are missing.
func (c *ServiceConfig) Validate() error {
	if c.MCPGateway.Endpoint == "" {
		return fmt.Errorf("mcpGateway.endpoint is required")
	}
	if c.Valkey.Addr == "" {
		return fmt.Errorf("valkey.addr is required")
	}
	if c.OAuth2.TokenURL == "" {
		return fmt.Errorf("oauth2.tokenUrl is required — MCP Gateway requires authentication")
	}
	return nil
}
