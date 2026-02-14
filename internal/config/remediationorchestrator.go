package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete RemediationOrchestrator configuration
// ADR-030: Service Configuration Management
// Per DS Team Response (2025-12-27): YAML-based audit buffer configuration
type Config struct {
	// Audit configuration (ADR-032, DD-AUDIT-002)
	Audit AuditConfig `yaml:"audit"`

	// Controller runtime configuration (DD-005)
	Controller ControllerConfig `yaml:"controller"`

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

// AuditConfig defines audit client behavior
// Per DS Team: Controls client-side buffering (not server-side)
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase
type AuditConfig struct {
	// DataStorage service URL for audit events (REQUIRED)
	DataStorageURL string `yaml:"dataStorageUrl"`

	// Timeout for audit API calls
	Timeout time.Duration `yaml:"timeout"`

	// Buffer configuration controls client-side buffering
	// CRITICAL: FlushInterval directly affects integration test timing!
	Buffer BufferConfig `yaml:"buffer"`
}

// BufferConfig controls audit event buffering and batching
// Per DS Team: This is where the 60s delay issue originates
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase
type BufferConfig struct {
	// Max events to buffer in memory before blocking
	BufferSize int `yaml:"bufferSize"`

	// Events per batch write to DataStorage
	BatchSize int `yaml:"batchSize"`

	// Max time before partial batch flush
	// CRITICAL for test timing: Lower = faster feedback, Higher = more efficient batching
	// Production: 1s (default), Integration Tests: 1s (fast feedback)
	FlushInterval time.Duration `yaml:"flushInterval"`

	// Retry attempts for failed writes (DLQ fallback after exhaustion)
	MaxRetries int `yaml:"maxRetries"`
}

// ControllerConfig defines controller runtime settings
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase
type ControllerConfig struct {
	MetricsAddr      string `yaml:"metricsAddr"`
	HealthProbeAddr  string `yaml:"healthProbeAddr"`
	LeaderElection   bool   `yaml:"leaderElection"`
	LeaderElectionID string `yaml:"leaderElectionId"`
}

// DefaultConfig returns safe defaults matching pkg/audit defaults
// Per DS Team: Default FlushInterval is 1s (not 5s!)
func DefaultConfig() *Config {
	return &Config{
		Audit: AuditConfig{
			DataStorageURL: "http://data-storage-service:8080", // DD-AUTH-011: Match Service name
			Timeout:        10 * time.Second,
			Buffer: BufferConfig{
				BufferSize:    10000,
				BatchSize:     100,
				FlushInterval: 1 * time.Second, // Changed from 5s hardcoded to 1s default
				MaxRetries:    3,
			},
		},
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

// Validate checks configuration for common issues
func (c *Config) Validate() error {
	// Validate audit config
	if c.Audit.DataStorageURL == "" {
		return fmt.Errorf("audit.dataStorageUrl is required")
	}
	if c.Audit.Timeout <= 0 {
		return fmt.Errorf("audit.timeout must be positive")
	}

	// Validate buffer config
	if c.Audit.Buffer.BufferSize <= 0 {
		return fmt.Errorf("audit.buffer.bufferSize must be positive")
	}
	if c.Audit.Buffer.BatchSize <= 0 {
		return fmt.Errorf("audit.buffer.batchSize must be positive")
	}
	if c.Audit.Buffer.FlushInterval <= 0 {
		return fmt.Errorf("audit.buffer.flushInterval must be positive")
	}
	if c.Audit.Buffer.MaxRetries < 0 {
		return fmt.Errorf("audit.buffer.maxRetries must be non-negative")
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




