package config

import (
	"fmt"
	"time"
)

// DataStorageConfig defines connectivity and buffering for the Data Storage service.
// ADR-030: All non-DS services MUST include a `datastorage` top-level section
// that maps to this struct. DataStorage serves audit trail and workflow catalog APIs.
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase.
type DataStorageConfig struct {
	// URL is the Data Storage service HTTP endpoint (REQUIRED).
	// Example: "http://data-storage-service.kubernaut-system.svc.cluster.local:8080"
	URL string `yaml:"url"`

	// Timeout for individual Data Storage API calls.
	Timeout time.Duration `yaml:"timeout"`

	// Buffer controls client-side event buffering and batching.
	// CRITICAL: FlushInterval directly affects integration test timing!
	Buffer BufferConfig `yaml:"buffer"`
}

// BufferConfig controls audit event buffering and batching.
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase.
type BufferConfig struct {
	// BufferSize is the max events to buffer in memory before blocking.
	BufferSize int `yaml:"bufferSize"`

	// BatchSize is the number of events per batch write to DataStorage.
	BatchSize int `yaml:"batchSize"`

	// FlushInterval is the max time before a partial batch is flushed.
	// CRITICAL for test timing: Lower = faster feedback, Higher = more efficient batching.
	// Production: 1s (default), Integration Tests: 1s (fast feedback).
	FlushInterval time.Duration `yaml:"flushInterval"`

	// MaxRetries is the number of retry attempts for failed writes (DLQ fallback after exhaustion).
	MaxRetries int `yaml:"maxRetries"`
}

// DefaultDataStorageConfig returns safe defaults for Data Storage connectivity.
func DefaultDataStorageConfig() DataStorageConfig {
	return DataStorageConfig{
		URL:     "http://data-storage-service:8080",
		Timeout: 10 * time.Second,
		Buffer: BufferConfig{
			BufferSize:    10000,
			BatchSize:     100,
			FlushInterval: 1 * time.Second,
			MaxRetries:    3,
		},
	}
}

// ValidateDataStorageConfig checks Data Storage configuration for common issues.
// Error messages use the YAML path prefix "datastorage." for user-facing diagnostics.
func ValidateDataStorageConfig(ds *DataStorageConfig) error {
	if ds.URL == "" {
		return fmt.Errorf("datastorage.url is required")
	}
	if ds.Timeout <= 0 {
		return fmt.Errorf("datastorage.timeout must be positive")
	}
	if ds.Buffer.BufferSize <= 0 {
		return fmt.Errorf("datastorage.buffer.bufferSize must be positive")
	}
	if ds.Buffer.BatchSize <= 0 {
		return fmt.Errorf("datastorage.buffer.batchSize must be positive")
	}
	if ds.Buffer.FlushInterval <= 0 {
		return fmt.Errorf("datastorage.buffer.flushInterval must be positive")
	}
	if ds.Buffer.MaxRetries < 0 {
		return fmt.Errorf("datastorage.buffer.maxRetries must be non-negative")
	}
	return nil
}
