package audit

import (
	"fmt"
	"time"
)

// Config contains configuration for the buffered audit store.
//
// Authority: DD-AUDIT-002 (Audit Shared Library Design)
//
// This configuration controls the behavior of the asynchronous buffered audit ingestion,
// including buffer sizes, batch sizes, flush intervals, and retry policies.
//
// All services MUST use the same configuration defaults to ensure consistent behavior.
type Config struct {
	// BufferSize is the maximum number of events to buffer in memory before blocking or dropping.
	//
	// Default: 10000
	// Recommendation: 10 seconds of peak traffic
	//
	// When the buffer is full, new events will be dropped (graceful degradation).
	// Monitor audit_events_dropped_total metric to detect buffer overruns.
	BufferSize int

	// BatchSize is the number of events to batch before writing to the Data Storage Service.
	//
	// Default: 1000
	// Recommendation: Optimal for PostgreSQL INSERT performance
	//
	// Larger batches improve throughput but increase latency for individual events.
	// Smaller batches reduce latency but decrease throughput.
	BatchSize int

	// FlushInterval is the maximum time to wait before flushing a partial batch.
	//
	// Default: 1 second
	// Recommendation: Balance between latency and efficiency
	//
	// If a batch doesn't reach BatchSize within FlushInterval, it will be flushed anyway
	// to prevent events from sitting in the buffer indefinitely.
	FlushInterval time.Duration

	// MaxRetries is the number of retry attempts for failed writes.
	//
	// Default: 3
	// Recommendation: Handles transient failures (network blips, DB restarts)
	//
	// Retries use exponential backoff: 1s, 4s, 9s (attempt^2 seconds).
	// After MaxRetries, the batch is dropped and logged.
	MaxRetries int
}

// DefaultConfig returns the recommended default configuration for all services.
//
// These defaults are based on:
// - Expected peak traffic: 1000 events/second
// - PostgreSQL batch insert performance: ~1000 rows optimal
// - Acceptable latency: 1 second for partial batches
// - Transient failure handling: 3 retries with exponential backoff
//
// Services with higher traffic volumes (e.g., Gateway) may want to increase BufferSize.
func DefaultConfig() Config {
	return Config{
		BufferSize:    10000,
		BatchSize:     1000,
		FlushInterval: 1 * time.Second,
		MaxRetries:    3,
	}
}

// RecommendedConfig returns service-specific recommended configurations.
//
// Some services have different traffic patterns and may benefit from tuned configurations:
// - Gateway: High volume (2x buffer size)
// - AI Analysis: LLM retries (1.5x buffer size)
// - Default: Standard configuration for most services
func RecommendedConfig(serviceName string) Config {
	switch serviceName {
	case "gateway":
		// Gateway receives all external signals, high volume expected
		return Config{
			BufferSize:    20000, // 2x default
			BatchSize:     1000,
			FlushInterval: 1 * time.Second,
			MaxRetries:    3,
		}
	case "ai-analysis":
		// AI Analysis may generate multiple events per analysis (LLM retries, fallbacks)
		return Config{
			BufferSize:    15000, // 1.5x default
			BatchSize:     1000,
			FlushInterval: 1 * time.Second,
			MaxRetries:    3,
		}
	default:
		return DefaultConfig()
	}
}

// Validate validates the configuration for correctness.
//
// Returns an error if any configuration value is invalid.
func (c Config) Validate() error {
	if c.BufferSize <= 0 {
		return fmt.Errorf("buffer_size must be positive, got %d", c.BufferSize)
	}
	if c.BatchSize <= 0 {
		return fmt.Errorf("batch_size must be positive, got %d", c.BatchSize)
	}
	if c.BatchSize > c.BufferSize {
		return fmt.Errorf("batch_size (%d) must be <= buffer_size (%d)", c.BatchSize, c.BufferSize)
	}
	if c.FlushInterval <= 0 {
		return fmt.Errorf("flush_interval must be positive, got %v", c.FlushInterval)
	}
	if c.MaxRetries < 0 {
		return fmt.Errorf("max_retries must be non-negative, got %d", c.MaxRetries)
	}
	return nil
}
