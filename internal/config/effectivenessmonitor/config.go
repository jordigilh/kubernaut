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

package effectivenessmonitor

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	sharedconfig "github.com/jordigilh/kubernaut/internal/config"
)

// DefaultConfigPath is the standard Kubernetes ConfigMap mount path for this service.
// ADR-030: All services MUST use /etc/{service}/config.yaml as the default.
const DefaultConfigPath = "/etc/effectivenessmonitor/config.yaml"

// Config represents the complete Effectiveness Monitor configuration.
// ADR-030: Service Configuration Management
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase
type Config struct {
	// Assessment configuration (BR-EM-006, BR-EM-007, BR-EM-008)
	Assessment AssessmentConfig `yaml:"assessment"`

	// Controller runtime configuration (DD-005)
	Controller sharedconfig.ControllerConfig `yaml:"controller"`

	// DataStorage connectivity (ADR-030: audit trail + workflow catalog)
	DataStorage sharedconfig.DataStorageConfig `yaml:"datastorage"`

	// External service configuration
	External ExternalConfig `yaml:"external"`
}

// AssessmentConfig defines assessment behavior.
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase
type AssessmentConfig struct {
	// StabilizationWindow is the duration to wait after remediation before assessment.
	// Default: 5m. Range: [30s, 1h].
	StabilizationWindow time.Duration `yaml:"stabilizationWindow"`

	// ValidityWindow is the maximum duration for assessment completion.
	// Default: 30m. Range: [30s, 24h].
	ValidityWindow time.Duration `yaml:"validityWindow"`
}

// ExternalConfig defines external service connection parameters.
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase
type ExternalConfig struct {
	// PrometheusURL is the Prometheus API base URL.
	PrometheusURL string `yaml:"prometheusUrl"`

	// PrometheusEnabled indicates whether Prometheus metric comparison is active.
	PrometheusEnabled bool `yaml:"prometheusEnabled"`

	// AlertManagerURL is the AlertManager API base URL.
	AlertManagerURL string `yaml:"alertManagerUrl"`

	// AlertManagerEnabled indicates whether AlertManager alert checking is active.
	AlertManagerEnabled bool `yaml:"alertManagerEnabled"`

	// ConnectionTimeout for external service HTTP clients.
	ConnectionTimeout time.Duration `yaml:"connectionTimeout"`
}

// DefaultConfig returns safe defaults for the Effectiveness Monitor.
func DefaultConfig() *Config {
	return &Config{
		Assessment: AssessmentConfig{
			StabilizationWindow: 5 * time.Minute,
			ValidityWindow:      30 * time.Minute,
		},
		DataStorage: sharedconfig.DefaultDataStorageConfig(),
		Controller: sharedconfig.ControllerConfig{
			MetricsAddr:      ":9090",
			HealthProbeAddr:  ":8081",
			LeaderElection:   false,
			LeaderElectionID: "effectivenessmonitor.kubernaut.ai",
		},
		External: ExternalConfig{
			PrometheusURL:       "http://prometheus:9090",
			PrometheusEnabled:   true,
			AlertManagerURL:     "http://alertmanager:9093",
			AlertManagerEnabled: true,
			ConnectionTimeout:   10 * time.Second,
		},
	}
}

// LoadFromFile loads EM configuration from YAML file with defaults.
// ADR-030: Service Configuration Management pattern.
// Graceful degradation: Falls back to defaults if file not found or invalid.
func LoadFromFile(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate checks EM configuration for common issues.
func (c *Config) Validate() error {
	// Validate assessment config
	if c.Assessment.StabilizationWindow < 30*time.Second {
		return fmt.Errorf("assessment.stabilizationWindow must be at least 30s, got %v", c.Assessment.StabilizationWindow)
	}
	if c.Assessment.StabilizationWindow > 1*time.Hour {
		return fmt.Errorf("assessment.stabilizationWindow must not exceed 1h, got %v", c.Assessment.StabilizationWindow)
	}
	if c.Assessment.ValidityWindow < 30*time.Second {
		return fmt.Errorf("assessment.validityWindow must be at least 30s, got %v", c.Assessment.ValidityWindow)
	}
	if c.Assessment.ValidityWindow > 24*time.Hour {
		return fmt.Errorf("assessment.validityWindow must not exceed 24h, got %v", c.Assessment.ValidityWindow)
	}
	if c.Assessment.StabilizationWindow >= c.Assessment.ValidityWindow {
		return fmt.Errorf("assessment.stabilizationWindow (%v) must be shorter than validityWindow (%v)",
			c.Assessment.StabilizationWindow, c.Assessment.ValidityWindow)
	}

	// Validate DataStorage config (ADR-030)
	if err := sharedconfig.ValidateDataStorageConfig(&c.DataStorage); err != nil {
		return err
	}

	// Validate controller config
	if c.Controller.MetricsAddr == "" {
		return fmt.Errorf("controller.metricsAddr is required")
	}
	if c.Controller.HealthProbeAddr == "" {
		return fmt.Errorf("controller.healthProbeAddr is required")
	}

	// Validate external service config
	if c.External.PrometheusEnabled && c.External.PrometheusURL == "" {
		return fmt.Errorf("external.prometheusUrl is required when Prometheus is enabled")
	}
	if c.External.AlertManagerEnabled && c.External.AlertManagerURL == "" {
		return fmt.Errorf("external.alertManagerUrl is required when AlertManager is enabled")
	}
	if c.External.ConnectionTimeout <= 0 {
		return fmt.Errorf("external.connectionTimeout must be positive")
	}

	return nil
}
