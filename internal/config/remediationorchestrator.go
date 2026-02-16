package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete RemediationOrchestrator configuration
// ADR-030: Service Configuration Management
type Config struct {
	// Controller runtime configuration (DD-005)
	Controller ControllerConfig `yaml:"controller"`

	// DataStorage connectivity (ADR-030: audit trail + workflow catalog)
	DataStorage DataStorageConfig `yaml:"datastorage"`

	// EffectivenessAssessment configuration (ADR-EM-001)
	// Controls how the RO creates EffectivenessAssessment CRDs on remediation completion.
	// The RO only sets StabilizationWindow; all other assessment parameters
	// (PrometheusEnabled, AlertManagerEnabled, ValidityWindow) are EM-internal config.
	EA EACreationConfig `yaml:"effectivenessAssessment"`
}

// EACreationConfig controls EffectivenessAssessment CRD creation by the RO.
// ADR-EM-001: RO creates EA CRD when RR reaches terminal phase (Completed, Failed, TimedOut).
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase
type EACreationConfig struct {
	// StabilizationWindow is the duration the EM should wait after remediation
	// before starting assessment checks. Set in the EA spec by the RO.
	// Default: 5m (ADR-EM-001 Section 8). Range: [1s, 1h].
	StabilizationWindow time.Duration `yaml:"stabilizationWindow"`
}

// ControllerConfig defines controller runtime settings
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase
type ControllerConfig struct {
	MetricsAddr      string `yaml:"metricsAddr"`
	HealthProbeAddr  string `yaml:"healthProbeAddr"`
	LeaderElection   bool   `yaml:"leaderElection"`
	LeaderElectionID string `yaml:"leaderElectionId"`
}

// DefaultConfig returns safe defaults matching pkg/audit defaults.
func DefaultConfig() *Config {
	return &Config{
		DataStorage: DefaultDataStorageConfig(),
		Controller: ControllerConfig{
			MetricsAddr:      ":9090",
			HealthProbeAddr:  ":8081",
			LeaderElection:   false,
			LeaderElectionID: "remediationorchestrator.kubernaut.ai",
		},
		EA: EACreationConfig{
			StabilizationWindow: 5 * time.Minute, // ADR-EM-001: default 5m (Section 8)
		},
	}
}

// LoadFromFile loads configuration from YAML file with defaults
// ADR-030: Service Configuration Management pattern
// Graceful degradation: Falls back to defaults if file not found or invalid
func LoadFromFile(path string) (*Config, error) {
	// Start with defaults
	cfg := DefaultConfig()

	// If path is empty, return defaults
	if path == "" {
		return cfg, nil
	}

	// Read YAML file
	data, err := os.ReadFile(path)
	if err != nil {
		// Graceful degradation: Return defaults with error
		return cfg, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML into config struct
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate checks configuration for common issues.
func (c *Config) Validate() error {
	// Validate DataStorage config (ADR-030)
	if err := ValidateDataStorageConfig(&c.DataStorage); err != nil {
		return err
	}

	// Validate controller config
	if c.Controller.MetricsAddr == "" {
		return fmt.Errorf("controller.metricsAddr is required")
	}
	if c.Controller.HealthProbeAddr == "" {
		return fmt.Errorf("controller.healthProbeAddr is required")
	}

	// Validate EA creation config (ADR-EM-001)
	if c.EA.StabilizationWindow < 1*time.Second {
		return fmt.Errorf("effectivenessAssessment.stabilizationWindow must be at least 1s, got %v", c.EA.StabilizationWindow)
	}
	if c.EA.StabilizationWindow > 1*time.Hour {
		return fmt.Errorf("effectivenessAssessment.stabilizationWindow must not exceed 1h, got %v", c.EA.StabilizationWindow)
	}

	return nil
}




