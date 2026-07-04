/*
Copyright 2025 Jordi Gil.

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

// Package config provides configuration types for the SignalProcessing controller.
//
// Configuration Structure:
//   - EnrichmentConfig: K8s context enrichment settings (cache TTL, timeout)
//   - ClassifierConfig: Rego-based classification settings (ConfigMap, hot-reload)
//   - DataStorageConfig: Data Storage connectivity (ADR-030)
//   - ControllerConfig: Controller runtime settings (metrics, health probes, leader election)
//
// All configuration values are validated via Config.Validate() before use.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	sharedconfig "github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// DefaultConfigPath is the standard Kubernetes ConfigMap mount path for this service.
// ADR-030: All services MUST use /etc/{service}/config.yaml as the default.
const DefaultConfigPath = "/etc/signalprocessing/config.yaml"

// Config holds the complete configuration for the SignalProcessing controller.
// ADR-030: YAML-based configuration with camelCase field names.
type Config struct {
	Enrichment  EnrichmentConfig               `yaml:"enrichment"`
	Classifier  ClassifierConfig               `yaml:"classifier"`
	DataStorage sharedconfig.DataStorageConfig `yaml:"datastorage"`
	Controller  ControllerConfig               `yaml:"controller"`

	// Fleet configuration for multi-cluster enrichment via MCP Gateway.
	// BR-INTEGRATION-054: When Endpoint is set, SP connects to MCP Gateway
	// for remote cluster resource reads.
	Fleet FleetConfig `yaml:"fleet"`

	// Logging configuration (Issue #875: config-file-only log level with hot-reload)
	Logging sharedconfig.LoggingConfig `yaml:"logging"`

	// TLSProfile selects the TLS security profile (Old/Intermediate/Modern).
	// Issue #748: OCP-only — set by kubernaut-operator from the cluster APIServer CR.
	TLSProfile string `yaml:"tlsProfile,omitempty"`
}

// FleetConfig holds MCP Gateway connectivity settings for multi-cluster support.
// BR-INTEGRATION-054: Optional -- when Endpoint is empty, SP operates in local-only mode.
type FleetConfig struct {
	Endpoint string      `yaml:"endpoint"`
	OAuth2   FleetOAuth2 `yaml:"oauth2"`

	// MCPGatewayType selects the MCP Gateway CRD implementation ("eaigw" or
	// "kuadrant") used to construct a ClusterRegistry for cluster
	// classification label lookups (BR-FLEET-003, #1511). Optional -- when
	// empty, SP does not construct a ClusterRegistry and the `cluster`
	// classification dimension is never evaluated (non-fleet deployments).
	MCPGatewayType fleet.MCPGatewayType `yaml:"mcpGatewayType,omitempty"`

	// Namespace restricts the ClusterRegistry's CRD watch to a specific
	// namespace. Empty means watch all namespaces (BR-FLEET-003, #1511).
	Namespace string `yaml:"namespace,omitempty"`
}

// FleetOAuth2 holds OAuth2 credentials for MCP Gateway authentication.
type FleetOAuth2 struct {
	Enabled              bool     `yaml:"enabled"`
	TokenURL             string   `yaml:"tokenURL"`
	CredentialsSecretRef string   `yaml:"credentialsSecretRef"`
	Scopes               []string `yaml:"scopes,omitempty"`

	// TLSCAFile is the path to a CA bundle for verifying TokenURL's TLS
	// certificate when the OAuth2 provider presents one not signed by a
	// public/system CA (e.g. a cluster-local Keycloak/Dex issuer). Mirrors
	// pkg/fleet.FleetOAuth2Config.TLSCAFile; see that field's doc comment
	// for the root cause this addresses.
	TLSCAFile string `yaml:"tlsCAFile,omitempty"`
}

// EnrichmentConfig holds settings for K8s context enrichment.
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase.
type EnrichmentConfig struct {
	CacheTTL time.Duration `yaml:"cacheTtl"`
	Timeout  time.Duration `yaml:"timeout"`
}

// ClassifierConfig holds settings for Rego-based classification.
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase.
type ClassifierConfig struct {
	RegoConfigMapName string        `yaml:"regoConfigMapName"`
	RegoConfigMapKey  string        `yaml:"regoConfigMapKey"`
	HotReloadInterval time.Duration `yaml:"hotReloadInterval"`
}

// ControllerConfig holds controller runtime settings.
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase.
type ControllerConfig struct {
	MetricsAddr      string `yaml:"metricsAddr"`
	HealthProbeAddr  string `yaml:"healthProbeAddr"`
	LeaderElection   bool   `yaml:"leaderElection"`
	LeaderElectionID string `yaml:"leaderElectionId"`
}

// DefaultControllerConfig returns the default controller configuration.
// Per STATELESS_SERVICES_PORT_STANDARD.md: :9090 metrics, :8081 health.
func DefaultControllerConfig() *ControllerConfig {
	return &ControllerConfig{
		MetricsAddr:      ":9090",
		HealthProbeAddr:  ":8081",
		LeaderElection:   false,
		LeaderElectionID: "signalprocessing.kubernaut.ai",
	}
}

// DefaultConfig returns the default SignalProcessing controller configuration.
func DefaultConfig() *Config {
	return &Config{
		Enrichment: EnrichmentConfig{
			CacheTTL: 5 * time.Minute,
			Timeout:  10 * time.Second,
		},
		Classifier: ClassifierConfig{
			RegoConfigMapName: "signalprocessing-policy",
			RegoConfigMapKey:  "policy.rego",
			HotReloadInterval: 30 * time.Second,
		},
		DataStorage: sharedconfig.DefaultDataStorageConfig(),
		Controller:  *DefaultControllerConfig(),
		Logging:     sharedconfig.DefaultLoggingConfig(),
	}
}

// LoadFromFile loads configuration from a YAML file.
// ADR-030: Unmarshal into DefaultConfig() so missing YAML fields retain defaults.
func LoadFromFile(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.Enrichment.CacheTTL <= 0 {
		return fmt.Errorf("enrichment cacheTtl must be positive, got %v", c.Enrichment.CacheTTL)
	}
	if c.Enrichment.Timeout <= 0 {
		return fmt.Errorf("enrichment timeout must be positive, got %v", c.Enrichment.Timeout)
	}
	if c.Classifier.RegoConfigMapName == "" {
		return fmt.Errorf("rego ConfigMap name is required")
	}
	if c.Classifier.HotReloadInterval <= 0 {
		return fmt.Errorf("hot-reload interval must be positive, got %v", c.Classifier.HotReloadInterval)
	}
	// ADR-030: Validate DataStorage section
	if err := sharedconfig.ValidateDataStorageConfig(&c.DataStorage); err != nil {
		return err
	}

	if c.Fleet.OAuth2.Enabled {
		if c.Fleet.OAuth2.TokenURL == "" {
			return fmt.Errorf("fleet.oauth2.tokenURL is required when oauth2.enabled=true")
		}
		if c.Fleet.OAuth2.CredentialsSecretRef == "" {
			return fmt.Errorf("fleet.oauth2.credentialsSecretRef is required when oauth2.enabled=true")
		}
	}

	// BR-FLEET-003 (#1511): mcpGatewayType, when set, must be a supported gateway.
	// Optional overall -- an empty value simply means SP does not construct a
	// ClusterRegistry (no cluster classification dimension).
	if c.Fleet.MCPGatewayType != "" && !registry.SupportedGateways[c.Fleet.MCPGatewayType] {
		return fmt.Errorf("fleet.mcpGatewayType %q is unsupported; must be one of: eaigw, kuadrant", c.Fleet.MCPGatewayType)
	}

	// Issue #875: Logging validation
	if err := c.Logging.Validate(); err != nil {
		return err
	}

	return nil
}
