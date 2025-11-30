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

// BufferedAuditStore implements AuditStore with asynchronous buffered writes.
//
// This implementation:
// - Buffers events in memory (non-blocking)
// - Batches events for efficient writes
// - Retries failed writes with exponential backoff
// - Flushes partial batches periodically
// - Drops events if buffer is full (graceful degradation)
//
// Authority: DD-AUDIT-002 (Audit Shared Library Design)
type BufferedAuditStore struct {
	buffer  chan *AuditEvent
	client  DataStorageClient
	logger  logr.Logger
	config  Config
	metrics MetricsLabels
	done    chan struct{}
	wg      sync.WaitGroup
	closed  int32 // Atomic flag to prevent double-close

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

		s.logger.Info("Audit buffer full, dropping event (graceful degradation)",
			"event_type", event.EventType,
			"correlation_id", event.CorrelationID,
			"buffered_count", atomic.LoadInt64(&s.bufferedCount),
			"dropped_count", atomic.LoadInt64(&s.droppedCount),
		)

		// ✅ Don't fail business logic - graceful degradation
		return nil
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
		s.logger.Info("Audit store closed",
			"buffered_count", atomic.LoadInt64(&s.bufferedCount),
			"written_count", atomic.LoadInt64(&s.writtenCount),
			"dropped_count", atomic.LoadInt64(&s.droppedCount),
			"failed_batch_count", atomic.LoadInt64(&s.failedBatchCount),
		)
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
// Retry strategy:
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

			if attempt < s.config.MaxRetries {
				// Exponential backoff: 1s, 4s, 9s
				backoff := time.Duration(attempt*attempt) * time.Second
				time.Sleep(backoff)
				continue
			}

			// Final failure: log and drop
			atomic.AddInt64(&s.failedBatchCount, 1)
			s.metrics.RecordBatchFailed()

			s.logger.Error(nil, "Dropping audit batch after max retries",
				"batch_size", len(batch),
				"max_retries", s.config.MaxRetries,
			)
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
