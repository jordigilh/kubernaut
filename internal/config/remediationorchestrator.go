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
}

// AuditConfig defines audit client behavior
// Per DS Team: Controls client-side buffering (not server-side)
type AuditConfig struct {
	// DataStorage service URL for audit events (REQUIRED)
	DataStorageURL string `yaml:"datastorage_url"`

	// Timeout for audit API calls
	Timeout time.Duration `yaml:"timeout"`

	// Buffer configuration controls client-side buffering
	// CRITICAL: FlushInterval directly affects integration test timing!
	Buffer BufferConfig `yaml:"buffer"`
}

// BufferConfig controls audit event buffering and batching
// Per DS Team: This is where the 60s delay issue originates
type BufferConfig struct {
	// Max events to buffer in memory before blocking
	BufferSize int `yaml:"buffer_size"`

	// Events per batch write to DataStorage
	BatchSize int `yaml:"batch_size"`

	// Max time before partial batch flush
	// CRITICAL for test timing: Lower = faster feedback, Higher = more efficient batching
	// Production: 1s (default), Integration Tests: 1s (fast feedback)
	FlushInterval time.Duration `yaml:"flush_interval"`

	// Retry attempts for failed writes (DLQ fallback after exhaustion)
	MaxRetries int `yaml:"max_retries"`
}

// ControllerConfig defines controller runtime settings
type ControllerConfig struct {
	MetricsAddr      string `yaml:"metrics_addr"`
	HealthProbeAddr  string `yaml:"health_probe_addr"`
	LeaderElection   bool   `yaml:"leader_election"`
	LeaderElectionID string `yaml:"leader_election_id"`
}

// DefaultConfig returns safe defaults matching pkg/audit defaults
// Per DS Team: Default FlushInterval is 1s (not 5s!)
func DefaultConfig() *Config {
	return &Config{
		Audit: AuditConfig{
			DataStorageURL: "http://datastorage:8080", // Correct service name (not datastorage-service)
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
		return fmt.Errorf("audit.datastorage_url is required")
	}
	if c.Audit.Timeout <= 0 {
		return fmt.Errorf("audit.timeout must be positive")
	}

	// Validate buffer config
	if c.Audit.Buffer.BufferSize <= 0 {
		return fmt.Errorf("audit.buffer.buffer_size must be positive")
	}
	if c.Audit.Buffer.BatchSize <= 0 {
		return fmt.Errorf("audit.buffer.batch_size must be positive")
	}
	if c.Audit.Buffer.FlushInterval <= 0 {
		return fmt.Errorf("audit.buffer.flush_interval must be positive")
	}
	if c.Audit.Buffer.MaxRetries < 0 {
		return fmt.Errorf("audit.buffer.max_retries must be non-negative")
	}

	// Validate controller config
	if c.Controller.MetricsAddr == "" {
		return fmt.Errorf("controller.metrics_addr is required")
	}
	if c.Controller.HealthProbeAddr == "" {
		return fmt.Errorf("controller.health_probe_addr is required")
	}

	return nil
}




