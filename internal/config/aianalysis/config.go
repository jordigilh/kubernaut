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

package aianalysis

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	sharedconfig "github.com/jordigilh/kubernaut/internal/config"
)

// DefaultConfigPath is the standard Kubernetes ConfigMap mount path for this service.
// ADR-030: All services MUST use /etc/{service}/config.yaml as the default.
const DefaultConfigPath = "/etc/aianalysis/config.yaml"

// Config represents the complete AIAnalysis controller configuration.
// ADR-030: Service Configuration Management
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase
type Config struct {
	// Controller runtime configuration (DD-005)
	Controller sharedconfig.ControllerConfig `yaml:"controller"`

	// HolmesGPT-API connectivity and session polling (BR-AI-007, BR-AA-HAPI-064)
	HolmesGPT HolmesGPTConfig `yaml:"holmesgpt"`

	// DataStorage connectivity (ADR-030: audit trail + workflow catalog)
	DataStorage sharedconfig.DataStorageConfig `yaml:"datastorage"`

	// Rego policy evaluation configuration (BR-AI-012)
	Rego RegoConfig `yaml:"rego"`
}

// HolmesGPTConfig defines HolmesGPT-API connectivity and session behavior.
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase
type HolmesGPTConfig struct {
	// URL is the HolmesGPT-API base URL (REQUIRED).
	URL string `yaml:"url"`

	// Timeout is the HTTP client timeout for HolmesGPT-API calls.
	Timeout time.Duration `yaml:"timeout"`

	// SessionPollInterval is the constant interval between session status polls.
	// BR-AA-HAPI-064.8: Polling is normal async behavior, not error recovery.
	// Default: 15s. Range: [1s, 5m].
	SessionPollInterval time.Duration `yaml:"sessionPollInterval"`
}

// RegoConfig defines Rego policy evaluation configuration.
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase
type RegoConfig struct {
	// PolicyPath is the file path to the Rego approval policy.
	PolicyPath string `yaml:"policyPath"`

	// ConfidenceThreshold is the operator-configurable auto-approval confidence threshold (#225).
	// When set, passed as input.confidence_threshold to the Rego policy, overriding the
	// policy's built-in default (0.8). Must be in range (0.0, 1.0].
	// nil means "use the Rego policy's built-in default".
	// Stepping stone toward BR-HAPI-198 (V1.1 rule-based thresholds).
	ConfidenceThreshold *float64 `yaml:"confidenceThreshold,omitempty"`
}

// DefaultConfig returns safe defaults for the AIAnalysis controller.
// DD-AUDIT-004: AA-specific buffer defaults (LOW tier: 20K buffer, 1K batch)
// override the shared DefaultDataStorageConfig() values.
func DefaultConfig() *Config {
	ds := sharedconfig.DefaultDataStorageConfig()
	ds.Buffer.BufferSize = 20000
	ds.Buffer.BatchSize = 1000

	return &Config{
		Controller: sharedconfig.ControllerConfig{
			MetricsAddr:      ":9090",
			HealthProbeAddr:  ":8081",
			LeaderElection:   false,
			LeaderElectionID: "aianalysis.kubernaut.ai",
		},
		HolmesGPT: HolmesGPTConfig{
			URL:                 "http://holmesgpt-api:8080",
			Timeout:             180 * time.Second,
			SessionPollInterval: 15 * time.Second,
		},
		DataStorage: ds,
		Rego: RegoConfig{
			PolicyPath: "/etc/aianalysis/policies/approval.rego",
		},
	}
}

// LoadFromFile loads AIAnalysis configuration from YAML file with defaults.
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

// Validate checks AIAnalysis configuration for common issues.
func (c *Config) Validate() error {
	// Validate controller config
	if c.Controller.MetricsAddr == "" {
		return fmt.Errorf("controller.metricsAddr is required")
	}
	if c.Controller.HealthProbeAddr == "" {
		return fmt.Errorf("controller.healthProbeAddr is required")
	}

	// Validate HolmesGPT config
	if c.HolmesGPT.URL == "" {
		return fmt.Errorf("holmesgpt.url is required")
	}
	if c.HolmesGPT.Timeout <= 0 {
		return fmt.Errorf("holmesgpt.timeout must be positive, got %v", c.HolmesGPT.Timeout)
	}
	if c.HolmesGPT.SessionPollInterval < 1*time.Second {
		return fmt.Errorf("holmesgpt.sessionPollInterval must be at least 1s, got %v", c.HolmesGPT.SessionPollInterval)
	}
	if c.HolmesGPT.SessionPollInterval > 5*time.Minute {
		return fmt.Errorf("holmesgpt.sessionPollInterval must not exceed 5m, got %v", c.HolmesGPT.SessionPollInterval)
	}

	// Validate DataStorage config (ADR-030)
	if err := sharedconfig.ValidateDataStorageConfig(&c.DataStorage); err != nil {
		return err
	}

	// Validate Rego config
	if c.Rego.PolicyPath == "" {
		return fmt.Errorf("rego.policyPath is required")
	}
	if c.Rego.ConfidenceThreshold != nil {
		t := *c.Rego.ConfidenceThreshold
		if t <= 0 || t > 1.0 {
			return fmt.Errorf("rego.confidenceThreshold must be in range (0.0, 1.0], got %v", t)
		}
	}

	return nil
}
