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
// Authority: DD-AUDIT-004 (Buffer Sizing Strategy for Burst Traffic)
//
// Buffer sizes are based on 3-tier strategy for handling burst traffic (10x normal rate):
// - HIGH-VOLUME SERVICES (>2000 events/day): 50,000 buffer
// - MEDIUM-VOLUME SERVICES (1000-2000 events/day): 30,000 buffer
// - LOW-VOLUME SERVICES (<1000 events/day): 20,000 buffer
//
// Rationale: Stress testing showed 90% event loss with 20,000 buffer under 25,000 burst.
// New sizes provide 1.2x-2x headroom for burst scenarios while minimizing memory footprint.
//
// See: docs/architecture/decisions/DD-AUDIT-004-buffer-sizing-strategy.md
func RecommendedConfig(serviceName string) Config {
	switch serviceName {
	// ========================================
	// HIGH-VOLUME SERVICES (>2000 events/day)
	// DD-AUDIT-004: 50,000 buffer for burst handling
	// ========================================
	case "datastorage":
		// DataStorage: 5000 events/day (highest volume service)
		// Buffer sized for 10x burst + 1.5x safety margin
		return Config{
			BufferSize:    50000, // DD-AUDIT-004: HIGH tier
			BatchSize:     1000,
			FlushInterval: 1 * time.Second,
			MaxRetries:    3,
		}
	case "workflowexecution":
		// WorkflowExecution: 2000 events/day (high volume)
		// Buffer sized for 10x burst + 1.5x safety margin
		return Config{
			BufferSize:    50000, // DD-AUDIT-004: HIGH tier
			BatchSize:     1000,
			FlushInterval: 1 * time.Second,
			MaxRetries:    3,
		}

	// ========================================
	// MEDIUM-VOLUME SERVICES (1000-2000 events/day)
	// DD-AUDIT-004: 30,000 buffer for burst handling
	// ========================================
	case "gateway":
		// Gateway: 1000 events/day (external signal ingestion)
		// Buffer sized for 10x burst + 1.5x safety margin
		return Config{
			BufferSize:    30000, // DD-AUDIT-004: MEDIUM tier (was 20000)
			BatchSize:     1000,
			FlushInterval: 1 * time.Second,
			MaxRetries:    3,
		}
	case "signalprocessing":
		// SignalProcessing: 1000 events/day (enrichment operations)
		// Buffer sized for 10x burst + 1.5x safety margin
		return Config{
			BufferSize:    30000, // DD-AUDIT-004: MEDIUM tier
			BatchSize:     1000,
			FlushInterval: 1 * time.Second,
			MaxRetries:    3,
		}
	case "remediation-orchestrator":
		// RemediationOrchestrator: 1200 events/day (lifecycle coordination)
		// Buffer sized for 10x burst + 1.5x safety margin
		return Config{
			BufferSize:    30000, // DD-AUDIT-004: MEDIUM tier
			BatchSize:     1000,
			FlushInterval: 1 * time.Second,
			MaxRetries:    3,
		}

	// ========================================
	// LOW-VOLUME SERVICES (<1000 events/day)
	// DD-AUDIT-004: 20,000 buffer for burst handling
	// ========================================
	case "aianalysis", "ai-analysis":
		// AIAnalysis: 500 events/day (LLM analysis operations)
		// Buffer sized for 10x burst + 1.5x safety margin
		return Config{
			BufferSize:    20000, // DD-AUDIT-004: LOW tier (was 15000)
			BatchSize:     1000,
			FlushInterval: 1 * time.Second,
			MaxRetries:    3,
		}
	case "notification", "notification-controller":
		// Notification: 500 events/day (delivery operations)
		// Buffer sized for 10x burst + 1.5x safety margin
		return Config{
			BufferSize:    20000, // DD-AUDIT-004: LOW tier
			BatchSize:     1000,
			FlushInterval: 1 * time.Second,
			MaxRetries:    3,
		}
	case "effectivenessmonitor":
		// EffectivenessMonitor: 500 events/day (learning loop operations)
		// Buffer sized for 10x burst + 1.5x safety margin
		return Config{
			BufferSize:    20000, // DD-AUDIT-004: LOW tier
			BatchSize:     1000,
			FlushInterval: 1 * time.Second,
			MaxRetries:    3,
		}

	// ========================================
	// DEFAULT: MEDIUM tier for unknown services
	// DD-AUDIT-004: Conservative default
	// ========================================
	default:
		// Unknown services get medium-tier buffer (conservative)
		return Config{
			BufferSize:    30000, // DD-AUDIT-004: MEDIUM tier default
			BatchSize:     1000,
			FlushInterval: 1 * time.Second,
			MaxRetries:    3,
		}
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
