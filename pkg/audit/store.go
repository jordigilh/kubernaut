package audit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
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
	StoreAudit(ctx context.Context, event *AuditEvent) error

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
// Implementation: pkg/datastorage/client.DataStorageClient
type DataStorageClient interface {
	// StoreBatch writes a batch of audit events to the Data Storage Service.
	//
	// Returns an error if the write fails. The caller is responsible for retry logic.
	StoreBatch(ctx context.Context, events []*AuditEvent) error
}

// DLQClient is the interface for writing audit events to the Dead Letter Queue.
//
// Authority: DD-009 (Audit Write Error Recovery - Dead Letter Queue Pattern)
// Related: ADR-032 ("No Audit Loss" mandate)
//
// This interface is used as a fallback when the primary Data Storage write fails
// after max retries. Events written to DLQ are later processed by the async retry worker.
//
// Implementation: pkg/datastorage/dlq.Client
type DLQClient interface {
	// EnqueueAuditEvent adds an audit event to the DLQ for async retry.
	//
	// Parameters:
	// - ctx: Context for cancellation
	// - event: The audit event that failed to write
	// - originalError: The error that caused the primary write to fail
	//
	// Returns an error if the DLQ write fails.
	EnqueueAuditEvent(ctx context.Context, event *AuditEvent, originalError error) error
}

// BufferedAuditStore implements AuditStore with asynchronous buffered writes.
//
// This implementation:
// - Buffers events in memory (non-blocking)
// - Batches events for efficient writes
// - Retries failed writes with exponential backoff
// - Flushes partial batches periodically
// - Drops events if buffer is full (graceful degradation)
// - Falls back to DLQ if primary write fails (GAP-10, DD-009)
//
// Authority: DD-AUDIT-002 (Audit Shared Library Design)
// Related: DD-009 (Dead Letter Queue Pattern), ADR-032 ("No Audit Loss")
type BufferedAuditStore struct {
	buffer    chan *AuditEvent
	client    DataStorageClient
	dlqClient DLQClient // GAP-10: Optional DLQ fallback for failed writes (DD-009)
	logger    logr.Logger
	config    Config
	metrics   MetricsLabels
	done      chan struct{}
	wg        sync.WaitGroup
	closed    int32 // Atomic flag to prevent double-close

	// Metrics (atomic counters for thread-safe access)
	bufferedCount    int64
	droppedCount     int64
	writtenCount     int64
	failedBatchCount int64
	dlqEnqueueCount  int64 // GAP-10: Track events sent to DLQ
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
		buffer:  make(chan *AuditEvent, config.BufferSize),
		client:  client,
		logger:  logger.WithName("audit-store"),
		config:  config,
		metrics: MetricsLabels{Service: serviceName},
		done:    make(chan struct{}),
	}

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

// NewBufferedStoreWithDLQ creates a buffered audit store with DLQ fallback.
//
// This constructor is used when you want to enable Dead Letter Queue fallback
// for audit events that fail to write to the Data Storage Service after max retries.
//
// Authority: DD-009 (Audit Write Error Recovery - Dead Letter Queue Pattern)
// Related: ADR-032 ("No Audit Loss" mandate)
//
// Parameters:
// - client: Data Storage Service client for writing events
// - dlqClient: DLQ client for fallback writes (nil to disable DLQ)
// - config: Configuration for buffer size, batch size, flush interval, etc.
// - serviceName: Name of the service using this store (for metrics labels)
// - logger: Structured logger for audit store operations
//
// When DLQ is enabled:
// - Events that fail after max retries are written to DLQ instead of being dropped
// - DLQ events are later processed by the async retry worker (cmd/audit-retry-worker)
// - ADR-032 "No Audit Loss" mandate is satisfied
func NewBufferedStoreWithDLQ(client DataStorageClient, dlqClient DLQClient, config Config, serviceName string, logger logr.Logger) (AuditStore, error) {
	store, err := NewBufferedStore(client, config, serviceName, logger)
	if err != nil {
		return nil, err
	}

	// Inject DLQ client
	bufferedStore := store.(*BufferedAuditStore)
	bufferedStore.dlqClient = dlqClient

	if dlqClient != nil {
		logger.Info("Audit store DLQ fallback enabled (DD-009)",
			"service", serviceName,
		)
	}

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
func (s *BufferedAuditStore) StoreAudit(ctx context.Context, event *AuditEvent) error {
	// Validate event
	if err := event.Validate(); err != nil {
		s.logger.Error(err, "Invalid audit event")
		return fmt.Errorf("invalid audit event: %w", err)
	}

	select {
	case s.buffer <- event:
		// ✅ Event buffered successfully
		atomic.AddInt64(&s.bufferedCount, 1)
		s.metrics.RecordBuffered()
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

		// GAP-9: ADR-032 requires callers to know about dropped events
		// so they can implement DLQ fallback
		return fmt.Errorf("audit buffer full: event dropped (correlation_id=%s)", event.CorrelationID)
	}
}

// Close flushes remaining events and stops the background worker.
//
// This method blocks until all buffered events are written or the max timeout is reached.
// Call this during graceful shutdown to ensure no events are lost.
// Safe to call multiple times (idempotent).
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

	batch := make([]*AuditEvent, 0, s.config.BatchSize)

	for {
		select {
		case event, ok := <-s.buffer:
			if !ok {
				// Channel closed, flush remaining events
				if len(batch) > 0 {
					s.writeBatchWithRetry(batch)
				}
				return
			}

			batch = append(batch, event)
			s.metrics.SetBufferSize(len(s.buffer))

			// Write when batch is full
			if len(batch) >= s.config.BatchSize {
				s.writeBatchWithRetry(batch)
				batch = batch[:0] // Reset batch
			}

		case <-ticker.C:
			// Flush partial batch periodically
			if len(batch) > 0 {
				s.writeBatchWithRetry(batch)
				batch = batch[:0]
			}
			s.metrics.SetBufferSize(len(s.buffer))
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
func (s *BufferedAuditStore) writeBatchWithRetry(batch []*AuditEvent) {
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

			// GAP-10: DLQ fallback (DD-009, ADR-032 "No Audit Loss")
			// Write failed events to DLQ for async retry
			if s.dlqClient != nil {
				s.enqueueBatchToDLQ(ctx, batch, err)
			} else {
				s.logger.Error(nil, "AUDIT DATA LOSS: Dropping batch, no DLQ configured (violates ADR-032)",
					"batch_size", len(batch),
					"max_retries", s.config.MaxRetries,
				)
			}
			return
		}

		// Success
		atomic.AddInt64(&s.writtenCount, int64(len(batch)))
		s.metrics.RecordWritten(len(batch))

		s.logger.V(1).Info("Wrote audit batch",
			"batch_size", len(batch),
			"attempt", attempt,
		)
		return
	}
}

// enqueueBatchToDLQ writes all events in the batch to the DLQ for async retry.
//
// GAP-10: DLQ Fallback (DD-009, ADR-032 "No Audit Loss")
//
// This method is called when the primary write to Data Storage fails after max retries.
// Events are written to DLQ individually to maximize recovery (partial success is better
// than total loss if some DLQ writes fail).
func (s *BufferedAuditStore) enqueueBatchToDLQ(ctx context.Context, batch []*AuditEvent, originalError error) {
	dlqSuccessCount := 0
	dlqFailCount := 0

	for _, event := range batch {
		if err := s.dlqClient.EnqueueAuditEvent(ctx, event, originalError); err != nil {
			s.logger.Error(err, "DLQ fallback failed for event",
				"event_type", event.EventType,
				"correlation_id", event.CorrelationID,
			)
			dlqFailCount++
		} else {
			atomic.AddInt64(&s.dlqEnqueueCount, 1)
			dlqSuccessCount++
		}
	}

	s.logger.Info("Batch enqueued to DLQ (DD-009 fallback)",
		"batch_size", len(batch),
		"dlq_success", dlqSuccessCount,
		"dlq_failed", dlqFailCount,
	)
}
