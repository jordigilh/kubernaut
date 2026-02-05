package audit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// AuditStore provides non-blocking audit event storage with asynchronous buffered writes.
//
// Authority: DD-AUDIT-002 (Audit Shared Library Design)
// Related: ADR-038 (Asynchronous Buffered Audit Ingestion)
//
// All services MUST use this interface for audit event storage to ensure:
// - Consistent behavior across all services
// - Non-blocking business logic (audit failures don't block operations)
// - Efficient batching and retry logic
// - Graceful degradation (drop events if buffer full, don't crash)
//
// Implementation: BufferedAuditStore (see NewBufferedStore)
type AuditStore interface {
	// StoreAudit adds an event to the buffer (non-blocking).
	//
	// This method returns immediately and does NOT wait for the write to complete.
	// The event is added to an in-memory buffer and written asynchronously by a background worker.
	//
	// Returns an error only if:
	// - The event fails validation (missing required fields)
	// - The buffer is full (graceful degradation - event is dropped)
	//
	// IMPORTANT: This method NEVER blocks business logic. If the buffer is full,
	// the event is dropped and an error is returned, but the service continues operating.
	//
	// DD-AUDIT-002 V2.0: Updated to use OpenAPI types directly
	StoreAudit(ctx context.Context, event *ogenclient.AuditEventRequest) error

	// Flush forces an immediate write of all buffered events to DataStorage.
	//
	// This method blocks until:
	// - All currently buffered events are written to DataStorage, OR
	// - The provided context is cancelled
	//
	// Use cases:
	// - Integration tests: Ensure events are persisted before querying
	// - Graceful shutdown: Flush before Close() (though Close() also flushes)
	// - Critical events: Force immediate persistence for compliance
	//
	// NOTE: This method is primarily for testing. Production code should rely
	// on automatic flushing (FlushInterval) for better performance.
	//
	// Example (integration test):
	//   auditStore.Flush(ctx)  // Force immediate write
	//   events := queryDataStorage(correlationID)  // Now reliable
	Flush(ctx context.Context) error

	// Close flushes remaining events and stops the background worker.
	//
	// This method blocks until all buffered events are written or the max timeout is reached.
	// Call this during graceful shutdown to ensure no events are lost.
	//
	// Returns an error if the flush fails or times out.
	Close() error
}

// DataStorageClient is the interface for writing audit events to the Data Storage Service.
//
// This interface abstracts the HTTP client for the Data Storage Service,
// allowing for easy mocking in tests.
//
// DD-AUDIT-002 V2.0: Updated to use OpenAPI types directly
//
// Implementation: pkg/datastorage/client.DataStorageClient
type DataStorageClient interface {
	// StoreBatch writes a batch of audit events to the Data Storage Service.
	//
	// Returns an error if the write fails. The caller is responsible for retry logic.
	//
	// DD-AUDIT-002 V2.0: Uses OpenAPI-generated types
	StoreBatch(ctx context.Context, events []*ogenclient.AuditEventRequest) error
}

// BufferedAuditStore implements AuditStore with asynchronous buffered writes.
//
// This implementation:
// - Buffers events in memory (non-blocking)
// - Batches events for efficient writes
// - Retries failed writes with exponential backoff
// - Flushes partial batches periodically
// - Drops events if buffer is full (graceful degradation)
// - Drops events if DataStorage write fails after retries (infrastructure problem)
//
// Authority: DD-AUDIT-002 (Audit Shared Library Design)
// Related: ADR-032 ("No Audit Loss" - server-side DLQ in DataStorage handles persistence failures)
//
// DD-AUDIT-002 V2.0: Updated to use OpenAPI types directly
// DD-AUDIT-002 V3.0: Removed client-side DLQ (over-engineered, server-side DLQ sufficient)
type BufferedAuditStore struct {
	buffer     chan *ogenclient.AuditEventRequest
	flushChan  chan chan error // Channel to signal flush request and receive completion
	client     DataStorageClient
	logger     logr.Logger
	config     Config
	metrics    MetricsLabels
	done       chan struct{}
	wg         sync.WaitGroup
	closed     int32 // Atomic flag to prevent double-close

	// Metrics (atomic counters for thread-safe access)
	bufferedCount    int64
	droppedCount     int64
	writtenCount     int64
	failedBatchCount int64
}

// NewBufferedStore creates a new buffered audit store.
//
// Parameters:
// - client: Data Storage Service client for writing events
// - config: Configuration for buffer size, batch size, flush interval, etc.
// - serviceName: Name of the service using this store (for metrics labels)
// - logger: Structured logger for audit store operations (logr.Logger per DD-005 v2.0)
//
// The store starts a background worker goroutine that:
// - Batches events for efficient writes
// - Flushes partial batches periodically
// - Retries failed writes with exponential backoff
//
// Call Close() during graceful shutdown to flush remaining events.
//
// DD-005 v2.0: Accepts logr.Logger for unified logging interface across all services.
func NewBufferedStore(client DataStorageClient, config Config, serviceName string, logger logr.Logger) (AuditStore, error) {
	if client == nil {
		return nil, fmt.Errorf("client cannot be nil")
	}

	if err := config.Validate(); err != nil {
		logger.Error(err, "Invalid audit config, using defaults")
		config = DefaultConfig()
	}

	store := &BufferedAuditStore{
		buffer:    make(chan *ogenclient.AuditEventRequest, config.BufferSize),
		flushChan: make(chan chan error, 1), // Buffered to prevent deadlock
		client:    client,
		logger:    logger.WithName("audit-store"),
		config:    config,
		metrics:   MetricsLabels{Service: serviceName},
		done:      make(chan struct{}),
	}

	// DD-AUDIT-004: Initialize buffer capacity metric for saturation monitoring
	store.metrics.SetBufferCapacity(config.BufferSize)

	// Start background worker
	store.wg.Add(1)
	go store.backgroundWriter()

	logger.Info("Audit store initialized",
		"service", serviceName,
		"buffer_size", config.BufferSize,
		"batch_size", config.BatchSize,
		"flush_interval", config.FlushInterval,
		"max_retries", config.MaxRetries,
	)

	return store, nil
}

// StoreAudit adds an event to the buffer (non-blocking).
//
// This method validates the event and adds it to the buffer.
// If the buffer is full, the event is dropped (graceful degradation).
//
// Returns an error if:
// - The event fails validation (missing required fields)
// - The buffer is full (event is dropped, but service continues)
func (s *BufferedAuditStore) StoreAudit(ctx context.Context, event *ogenclient.AuditEventRequest) error {
	// DEBUG: Log entry to StoreAudit (E2E debugging - Dec 28, 2025)
	s.logger.Info("🔍 StoreAudit called",
		"event_type", event.EventType,
		"event_action", event.EventAction,
		"correlation_id", event.CorrelationID,
		"buffer_capacity", cap(s.buffer),
		"buffer_current_size", len(s.buffer))

	// Validate event using OpenAPI spec validation (DD-AUDIT-002 V2.0)
	if err := ValidateAuditEventRequest(event); err != nil {
		s.logger.Error(err, "Invalid audit event (OpenAPI validation)")
		return fmt.Errorf("invalid audit event: %w", err)
	}

	// Check if store is closed before sending (prevents panic during test cleanup)
	if atomic.LoadInt32(&s.closed) == 1 {
		s.logger.V(1).Info("⚠️ Audit store closed, dropping event",
			"event_type", event.EventType,
			"correlation_id", event.CorrelationID)
		atomic.AddInt64(&s.droppedCount, 1)
		s.metrics.RecordDropped()
		return nil // Silently drop event during graceful shutdown
	}

	// DEBUG: Validation passed
	s.logger.Info("✅ Validation passed, attempting to buffer event",
		"event_type", event.EventType,
		"buffer_size_before", len(s.buffer))

	select {
	case s.buffer <- event:
		// ✅ Event buffered successfully
		newCount := atomic.AddInt64(&s.bufferedCount, 1)
		s.metrics.RecordBuffered()

		// DEBUG: Event successfully added to buffer
		s.logger.Info("✅ Event buffered successfully",
			"event_type", event.EventType,
			"correlation_id", event.CorrelationID,
			"buffer_size_after", len(s.buffer),
			"total_buffered", newCount)
		return nil

	default:
		// ⚠️ Buffer full (rare, indicates system overload)
		atomic.AddInt64(&s.droppedCount, 1)
		s.metrics.RecordDropped()

		s.logger.Info("Audit buffer full, event dropped",
			"event_type", event.EventType,
			"correlation_id", event.CorrelationID,
			"buffered_count", atomic.LoadInt64(&s.bufferedCount),
			"dropped_count", atomic.LoadInt64(&s.droppedCount),
		)

		// DD-AUDIT-002 V3.0: Return error to inform caller of data loss
		// Buffer full indicates system overload - fix by increasing buffer size or reducing load
		return fmt.Errorf("audit buffer full: event dropped (correlation_id=%s)", event.CorrelationID)
	}
}

// Close flushes remaining events and stops the background worker.
//
// This method blocks until all buffered events are written or the max timeout is reached.
// Call this during graceful shutdown to ensure no events are lost.
// Safe to call multiple times (idempotent).
// Flush forces an immediate write of all buffered events to DataStorage.
//
// This method is useful for:
// - Integration tests: Ensure events are persisted before querying
// - Graceful shutdown preparation: Flush before Close()
// - Critical events: Force immediate persistence
//
// Implementation:
// - Sends flush signal to background writer
// - Waits for flush completion or context cancellation
// - Does not stop the background writer (unlike Close())
//
// Note: Store continues accepting new events after Flush() completes.
func (s *BufferedAuditStore) Flush(ctx context.Context) error {
	// Check if store is closed
	if atomic.LoadInt32(&s.closed) == 1 {
		return fmt.Errorf("audit store is closed")
	}

	s.logger.V(1).Info("🔄 Explicit flush requested")

	// Create response channel
	done := make(chan error, 1)

	// Send flush request to background writer
	select {
	case s.flushChan <- done:
		// Flush request sent successfully
	case <-ctx.Done():
		return fmt.Errorf("flush cancelled: %w", ctx.Err())
	}

	// Wait for flush to complete
	select {
	case err := <-done:
		if err != nil {
			s.logger.Error(err, "❌ Flush failed")
			return fmt.Errorf("flush failed: %w", err)
		}
		s.logger.V(1).Info("✅ Explicit flush completed")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("flush timeout: %w", ctx.Err())
	}
}

func (s *BufferedAuditStore) Close() error {
	// Check if already closed (atomic operation)
	if !atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		s.logger.V(1).Info("Audit store already closed, skipping")
		return nil
	}

	s.logger.Info("Closing audit store, flushing remaining events")

	// Close buffer (signals background worker to stop)
	close(s.buffer)

	// Wait for background worker to finish (with timeout)
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Background worker finished
		dropped := atomic.LoadInt64(&s.droppedCount)
		failedBatches := atomic.LoadInt64(&s.failedBatchCount)

		s.logger.Info("Audit store closed",
			"buffered_count", atomic.LoadInt64(&s.bufferedCount),
			"written_count", atomic.LoadInt64(&s.writtenCount),
			"dropped_count", dropped,
			"failed_batch_count", failedBatches,
		)

		// GAP-12: ADR-032 requires callers to know if events were lost
		if dropped > 0 {
			return fmt.Errorf("audit store closed with %d dropped events", dropped)
		}
		if failedBatches > 0 {
			return fmt.Errorf("audit store closed with %d failed batches", failedBatches)
		}
		return nil

	case <-time.After(30 * time.Second):
		// Timeout waiting for background worker
		s.logger.Error(nil, "Timeout waiting for audit store to close")
		return fmt.Errorf("timeout waiting for audit store to close")
	}
}

// backgroundWriter runs in a separate goroutine and handles batching and writing.
//
// This worker:
// - Batches events for efficient writes (up to BatchSize)
// - Flushes partial batches periodically (every FlushInterval)
// - Retries failed writes with exponential backoff (up to MaxRetries)
// - Drops batches after max retries (logs for manual investigation)
func (s *BufferedAuditStore) backgroundWriter() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.FlushInterval)
	defer ticker.Stop()

	startTime := time.Now()
	lastFlushTime := startTime
	tickCount := 0

	// DEBUG: Log when background writer starts (RO Team request - Dec 27, 2025)
	s.logger.Info("🚀 Audit background writer started",
		"flush_interval", s.config.FlushInterval,
		"batch_size", s.config.BatchSize,
		"buffer_size", s.config.BufferSize,
		"start_time", startTime.Format(time.RFC3339Nano))

	batch := make([]*ogenclient.AuditEventRequest, 0, s.config.BatchSize)

	for {
		select {
		case event, ok := <-s.buffer:
			if !ok {
				// Channel closed, flush remaining events
				if len(batch) > 0 {
					batchSizeBeforeFlush := len(batch)
					s.logger.V(1).Info("🛑 Flushing remaining events on shutdown",
						"batch_size", batchSizeBeforeFlush)
					s.writeBatchWithRetry(batch)
					s.logger.Info("✅ Shutdown flush completed",
						"flushed_count", batchSizeBeforeFlush)
				}
				s.logger.Info("🛑 Audit background writer stopped",
					"total_runtime", time.Since(startTime),
					"total_ticks", tickCount)
				return
			}

			// FIX: Moved inside case block - was incorrectly un-indented, causing events to be dropped
			batch = append(batch, event)
			bufferSize := len(s.buffer)
			s.metrics.SetBufferSize(bufferSize)
			s.metrics.SetBufferUtilization(bufferSize, s.config.BufferSize) // DD-AUDIT-004

			// Write when batch is full
			if len(batch) >= s.config.BatchSize {
				// Capture size BEFORE flushing (AA Team: prevent misleading logs)
				batchSizeBeforeFlush := len(batch)
				bufferUtilizationBeforeFlush := len(s.buffer)
				timeSinceLastFlush := time.Since(lastFlushTime)

				s.logger.V(1).Info("📦 Batch-full flush triggered",
					"batch_size", batchSizeBeforeFlush,
					"buffer_utilization", bufferUtilizationBeforeFlush,
					"time_since_last_flush", timeSinceLastFlush)
				s.writeBatchWithRetry(batch)
				lastFlushTime = time.Now()
				batch = batch[:0] // Reset batch
				s.logger.V(1).Info("✅ Batch-full flush completed",
					"flushed_count", batchSizeBeforeFlush,
					"batch_size_after_flush", len(batch),
					"buffer_utilization_after_flush", len(s.buffer))
			}

		case tickTime := <-ticker.C:
			tickCount++
			timeSinceLastFlush := time.Since(lastFlushTime)
			expectedInterval := s.config.FlushInterval
			drift := timeSinceLastFlush - expectedInterval

			// Capture batch size BEFORE any flushing (AA Team: prevent misleading logs)
			// This prevents the "batch_size: 0" confusion that occurred when logging AFTER flush
			batchSizeBeforeFlush := len(batch)
			bufferUtilizationBeforeFlush := len(s.buffer)

			// DEBUG: Log every ticker fire (RO Team issue - detecting 50-90s delays)
			// AA Team fix: Log batch_size BEFORE flush to avoid misleading "0" values
			s.logger.Info("⏰ Timer tick received",
				"tick_number", tickCount,
				"batch_size_before_flush", batchSizeBeforeFlush,
				"buffer_utilization", bufferUtilizationBeforeFlush,
				"expected_interval", expectedInterval,
				"actual_interval", timeSinceLastFlush,
				"drift", drift,
				"tick_time", tickTime.Format(time.RFC3339Nano))

			// Warn if tick drift exceeds 2x expected interval (potential bug)
			if timeSinceLastFlush > expectedInterval*2 {
				s.logger.Error(nil, "🚨 TIMER BUG DETECTED: Tick interval significantly exceeded expected",
					"expected_interval", expectedInterval,
					"actual_interval", timeSinceLastFlush,
					"drift", drift,
					"drift_multiplier", float64(timeSinceLastFlush)/float64(expectedInterval))
			}

			// Flush partial batch periodically
			if batchSizeBeforeFlush > 0 {
				s.logger.V(1).Info("⏱️  Timer-based flush triggered",
					"batch_size", batchSizeBeforeFlush,
					"buffer_utilization", bufferUtilizationBeforeFlush,
					"time_since_last_flush", timeSinceLastFlush)
				s.writeBatchWithRetry(batch)
				lastFlushTime = time.Now()
				batch = batch[:0]
				s.logger.V(1).Info("✅ Timer-based flush completed",
					"flushed_count", batchSizeBeforeFlush,
					"batch_size_after_flush", len(batch),
					"buffer_utilization_after_flush", len(s.buffer))
			} else {
				// Timer fired but no events to flush
				s.logger.V(2).Info("⏱️  Timer tick (no events to flush)",
					"buffer_utilization", bufferUtilizationBeforeFlush,
					"time_since_last_flush", timeSinceLastFlush)
				lastFlushTime = time.Now()
			}
			bufferSize := len(s.buffer)
			s.metrics.SetBufferSize(bufferSize)
			s.metrics.SetBufferUtilization(bufferSize, s.config.BufferSize) // DD-AUDIT-004

		case done := <-s.flushChan:
			// Explicit flush requested (typically from tests or graceful shutdown prep)
			initialBatchSize := len(batch)
			initialBufferSize := len(s.buffer)

			s.logger.V(1).Info("🔄 Processing explicit flush request",
				"batch_size_before_drain", initialBatchSize,
				"buffer_size_before_drain", initialBufferSize)

			// BUG FIX (SP-AUDIT-001): Drain s.buffer channel into batch BEFORE flushing
			// The explicit flush was only writing the batch array, missing events in s.buffer channel
			// This caused test failures where events were "buffered successfully" but never written
			drainedCount := 0
		drainLoop:
			for {
				select {
				case event := <-s.buffer:
					batch = append(batch, event)
					drainedCount++
				default:
					// Buffer drained (no more events available without blocking)
					break drainLoop
				}
			}

			if drainedCount > 0 {
				s.logger.V(1).Info("🔄 Drained buffer channel into batch",
					"drained_count", drainedCount,
					"batch_size_after_drain", len(batch),
					"buffer_size_after_drain", len(s.buffer))
			}

			if len(batch) > 0 {
				batchSizeBeforeFlush := len(batch)
				s.writeBatchWithRetry(batch)
				lastFlushTime = time.Now()
				batch = batch[:0] // Reset batch
				s.logger.V(1).Info("✅ Explicit flush completed",
					"flushed_count", batchSizeBeforeFlush,
					"drained_from_buffer", drainedCount,
					"buffer_size_after", len(s.buffer))
				done <- nil // Signal success
			} else {
				s.logger.V(1).Info("✅ Explicit flush completed (no events to flush)",
					"buffer_was_empty", initialBufferSize == 0)
				done <- nil // Signal success (nothing to flush)
			}
		}
	}
}

// writeBatchWithRetry writes a batch with exponential backoff retry logic.
//
// Retry strategy (GAP-10/GAP-11: Error differentiation):
// - 4xx errors (client errors): Do NOT retry - indicates invalid data
// - 5xx errors (server errors): Retry with exponential backoff
// - Network errors: Retry with exponential backoff
//
// Retry timing:
// - Attempt 1: Immediate
// - Attempt 2: 1 second delay
// - Attempt 3: 4 seconds delay
// - Attempt 4: 9 seconds delay
// - After MaxRetries: Drop batch and log
func (s *BufferedAuditStore) writeBatchWithRetry(batch []*ogenclient.AuditEventRequest) {
	ctx := context.Background()

	start := time.Now()
	defer func() {
		duration := time.Since(start).Seconds()
		s.metrics.ObserveWriteDuration(duration)
	}()

	for attempt := 1; attempt <= s.config.MaxRetries; attempt++ {
		if err := s.client.StoreBatch(ctx, batch); err != nil {
			s.logger.Error(err, "Failed to write audit batch",
				"attempt", attempt,
				"batch_size", len(batch),
			)

			// GAP-10/GAP-11: Check if error is retryable
			// 4xx errors are NOT retryable (invalid data)
			// 5xx and network errors ARE retryable
			if !IsRetryable(err) {
				// Non-retryable error (4xx) - don't retry, log as invalid
				atomic.AddInt64(&s.failedBatchCount, 1)
				s.metrics.RecordBatchFailed()

				s.logger.Error(nil, "Dropping audit batch due to non-retryable error (invalid data)",
					"batch_size", len(batch),
					"is_4xx_error", Is4xxError(err),
				)
				return
			}

			if attempt < s.config.MaxRetries {
				// Exponential backoff: 1s, 4s, 9s
				backoff := time.Duration(attempt*attempt) * time.Second
				time.Sleep(backoff)
				continue
			}

		// Final failure after max retries
		atomic.AddInt64(&s.failedBatchCount, 1)
		s.metrics.RecordBatchFailed()

		// DD-AUDIT-002 V3.0: Transport failures indicate infrastructure problem
		// Server-side DLQ (in DataStorage) handles persistence failures
		// Client-side audit loss is acceptable for transport failures (fix infrastructure)
		s.logger.Error(err, "AUDIT DATA LOSS: Dropping batch after max retries (infrastructure unavailable)",
			"batch_size", len(batch),
			"max_retries", s.config.MaxRetries,
			"error_type", "transport_failure",
			"mitigation", "Fix DataStorage connectivity - ensure service is running and reachable",
		)
		return
		}

		// Success
		atomic.AddInt64(&s.writtenCount, int64(len(batch)))
		s.metrics.RecordWritten(len(batch))

		writeDuration := time.Since(start)
		s.logger.V(1).Info("✅ Wrote audit batch",
			"batch_size", len(batch),
			"attempt", attempt,
			"write_duration", writeDuration)

		// Warn if write took unusually long (>2s)
		if writeDuration > 2*time.Second {
			s.logger.Error(nil, "⚠️  Slow audit batch write detected",
				"batch_size", len(batch),
				"write_duration", writeDuration,
				"warning", "slow writes can delay timer-based flushes")
		}

		return
	}
}
