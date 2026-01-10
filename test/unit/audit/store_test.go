package audit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	audit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

// MockDataStorageClient is a mock implementation of audit.DataStorageClient for testing
// DD-AUDIT-002 V2.0: Uses OpenAPI types (*ogenclient.AuditEventRequest)
type MockDataStorageClient struct {
	mu            sync.Mutex
	batches       [][]*ogenclient.AuditEventRequest
	failureCount  int32
	attemptCount  int32
	shouldFail    bool
	failUntilCall int32
	customError   error         // GAP-10: Support typed errors (HTTPError, NetworkError, etc.)
	writeDelay    time.Duration // GAP-9: Delay writes to test buffer full scenarios
}

func NewMockDataStorageClient() *MockDataStorageClient {
	return &MockDataStorageClient{
		batches: make([][]*ogenclient.AuditEventRequest, 0),
	}
}

func (m *MockDataStorageClient) StoreBatch(ctx context.Context, events []*ogenclient.AuditEventRequest) error {
	atomic.AddInt32(&m.attemptCount, 1)

	m.mu.Lock()
	delay := m.writeDelay
	m.mu.Unlock()

	// GAP-9: Apply delay to simulate slow writes (for buffer full testing)
	if delay > 0 {
		time.Sleep(delay)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// GAP-10: Check if custom error is set (for typed error testing)
	if m.customError != nil {
		atomic.AddInt32(&m.failureCount, 1)
		return m.customError
	}

	// Check if we should fail
	if m.shouldFail {
		atomic.AddInt32(&m.failureCount, 1)
		return fmt.Errorf("mock storage failure")
	}

	// Check if we should fail until a certain call
	if m.failUntilCall > 0 && atomic.LoadInt32(&m.attemptCount) <= m.failUntilCall {
		atomic.AddInt32(&m.failureCount, 1)
		return fmt.Errorf("mock transient failure")
	}

	// Success - store the batch
	batch := make([]*ogenclient.AuditEventRequest, len(events))
	copy(batch, events)
	m.batches = append(m.batches, batch)

	return nil
}

func (m *MockDataStorageClient) BatchCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.batches)
}

func (m *MockDataStorageClient) LastBatchSize() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.batches) == 0 {
		return 0
	}
	return len(m.batches[len(m.batches)-1])
}

func (m *MockDataStorageClient) TotalEventsWritten() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := 0
	for _, batch := range m.batches {
		total += len(batch)
	}
	return total
}

func (m *MockDataStorageClient) AttemptCount() int {
	return int(atomic.LoadInt32(&m.attemptCount))
}

func (m *MockDataStorageClient) FailureCount() int {
	return int(atomic.LoadInt32(&m.failureCount))
}

func (m *MockDataStorageClient) SetShouldFail(shouldFail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFail = shouldFail
}

// SetWriteDelay sets a delay for each StoreBatch call (for buffer full testing)
func (m *MockDataStorageClient) SetWriteDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeDelay = delay
}

func (m *MockDataStorageClient) SetFailUntilCall(failUntilCall int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failUntilCall = int32(failUntilCall)
}

// SetCustomError sets a custom error to return (GAP-10: typed error testing)
func (m *MockDataStorageClient) SetCustomError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.customError = err
}

func (m *MockDataStorageClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batches = make([][]*ogenclient.AuditEventRequest, 0)
	atomic.StoreInt32(&m.failureCount, 0)
	atomic.StoreInt32(&m.attemptCount, 0)
	m.shouldFail = false
	m.failUntilCall = 0
}

// MockDLQClient is a mock implementation of audit.DLQClient for testing (GAP-10)
// DD-AUDIT-002 V2.0: Uses OpenAPI types (*ogenclient.AuditEventRequest)
type MockDLQClient struct {
	mu           sync.Mutex
	events       []*ogenclient.AuditEventRequest
	errors       []error
	shouldFail   bool
	enqueueCount int32
	failCount    int32
}

func NewMockDLQClient() *MockDLQClient {
	return &MockDLQClient{
		events: make([]*ogenclient.AuditEventRequest, 0),
		errors: make([]error, 0),
	}
}

func (m *MockDLQClient) EnqueueAuditEvent(ctx context.Context, event *ogenclient.AuditEventRequest, originalError error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	atomic.AddInt32(&m.enqueueCount, 1)

	if m.shouldFail {
		atomic.AddInt32(&m.failCount, 1)
		return fmt.Errorf("mock DLQ failure")
	}

	m.events = append(m.events, event)
	m.errors = append(m.errors, originalError)
	return nil
}

func (m *MockDLQClient) EventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func (m *MockDLQClient) EnqueueCount() int {
	return int(atomic.LoadInt32(&m.enqueueCount))
}

func (m *MockDLQClient) FailCount() int {
	return int(atomic.LoadInt32(&m.failCount))
}

func (m *MockDLQClient) SetShouldFail(shouldFail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFail = shouldFail
}

func (m *MockDLQClient) GetEvents() []*ogenclient.AuditEventRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*ogenclient.AuditEventRequest, len(m.events))
	copy(result, m.events)
	return result
}

func (m *MockDLQClient) GetOriginalErrors() []error {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]error, len(m.errors))
	copy(result, m.errors)
	return result
}

// Helper function to create a test event
// DD-AUDIT-002 V2.0: Uses OpenAPI types and helper functions
func createTestEvent() *ogenclient.AuditEventRequest {
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, "test.event.created")
	audit.SetEventCategory(event, "gateway") // DD-TESTING-001: Use valid event_category from OpenAPI enum
	audit.SetEventAction(event, "created")
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "service", "test-service")
	audit.SetResource(event, "TestResource", "test-123")
	audit.SetCorrelationID(event, "corr-123")

	// Use GatewayAuditPayload for test event (ogen migration - discriminated union)
	payload := ogenclient.GatewayAuditPayload{
		EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewayCrdCreated,
		SignalType:  ogenclient.GatewayAuditPayloadSignalTypePrometheusAlert, // Updated enum
		AlertName:   "test-alert",
		Namespace:   "default",
		Fingerprint: "test-fingerprint",
	}
	audit.SetEventData(event, ogenclient.NewAuditEventRequestEventDataGatewayCrdCreatedAuditEventRequestEventData(payload))

	return event
}

var _ = Describe("BufferedAuditStore", func() {
	var (
		store      audit.AuditStore
		mockClient *MockDataStorageClient
		logger     logr.Logger
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockClient = NewMockDataStorageClient()
		// Use logr.Logger per DD-005 v2.0 (unified logging interface)
		logger = kubelog.NewLogger(kubelog.DevelopmentOptions())
	})

	AfterEach(func() {
		if store != nil {
			_ = store.Close()
		}
	})

	Describe("audit.NewBufferedStore", func() {
		It("should create store with valid config", func() {
			config := audit.Config{
				BufferSize:    100,
				BatchSize:     10,
				FlushInterval: 100 * time.Millisecond,
				MaxRetries:    3,
			}

			var err error
			store, err = audit.NewBufferedStore(mockClient, config, "test-service", logger)

			Expect(err).ToNot(HaveOccurred())
			Expect(store).ToNot(BeNil())
		})

		It("should return error if client is nil", func() {
			config := audit.DefaultConfig()

			_, err := audit.NewBufferedStore(nil, config, "test-service", logger)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("client cannot be nil"))
		})

		// Note: logr.Logger is a value type, not a pointer.
		// A zero-value logr.Logger is a valid no-op logger, so we don't test for nil.
		// This is different from *zap.Logger which could be nil.

		It("should use default config if validation fails", func() {
			config := audit.Config{
				BufferSize:    -1, // Invalid
				BatchSize:     10,
				FlushInterval: 100 * time.Millisecond,
				MaxRetries:    3,
			}

			var err error
			store, err = audit.NewBufferedStore(mockClient, config, "test-service", logger)

			// Should not error, but use defaults instead
			Expect(err).ToNot(HaveOccurred())
			Expect(store).ToNot(BeNil())
		})
	})

	Describe("StoreAudit", func() {
		BeforeEach(func() {
			config := audit.Config{
				BufferSize:    100,
				BatchSize:     10,
				FlushInterval: 10 * time.Second, // Long interval to prevent buffer drain during tests
				MaxRetries:    3,
			}
			var err error
			store, err = audit.NewBufferedStore(mockClient, config, "test-service", logger)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should buffer event successfully", func() {
			event := createTestEvent()

			err := store.StoreAudit(ctx, event)

			Expect(err).ToNot(HaveOccurred())
		})

		It("should validate event before buffering", func() {
			event := &ogenclient.AuditEventRequest{} // Invalid (missing required fields)

			err := store.StoreAudit(ctx, event)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid audit event"))
		})

		It("should not block when buffer has space", func() {
			event := createTestEvent()

			start := time.Now()
			err := store.StoreAudit(ctx, event)
			duration := time.Since(start)

			Expect(err).ToNot(HaveOccurred())
			Expect(duration).To(BeNumerically("<", 10*time.Millisecond)) // Should be instant
		})

		// GAP-9: ADR-032 requires callers to know about dropped events
		// so they can implement DLQ fallback
		It("should return error when buffer is full (ADR-032 compliance)", func() {
			// Slow down the background writer so buffer fills up
			mockClient.SetWriteDelay(1 * time.Second)
			defer mockClient.SetWriteDelay(0)

			// Create store with small buffer
			smallBufferConfig := audit.Config{
				BufferSize:    5,                // Very small buffer
				BatchSize:     5,                // Small batch
				FlushInterval: 10 * time.Second, // Long flush interval
				MaxRetries:    1,
			}
			smallStore, err := audit.NewBufferedStore(mockClient, smallBufferConfig, "test-service", logger)
			Expect(err).ToNot(HaveOccurred())
			defer smallStore.Close()

			// Rapidly fill the small buffer (background worker is slow)
			var bufferFullErr error
			for i := 0; i < 20; i++ { // Try more events than buffer can hold
				event := createTestEvent()
				if err := smallStore.StoreAudit(ctx, event); err != nil {
					bufferFullErr = err
					break
				}
			}

			// GAP-9: Should have received buffer full error
			// ADR-032: Caller MUST know event was dropped to implement DLQ fallback
			Expect(bufferFullErr).To(HaveOccurred())
			Expect(bufferFullErr.Error()).To(ContainSubstring("buffer full"))
		})
	})

	Describe("Batching", func() {
		BeforeEach(func() {
			config := audit.Config{
				BufferSize:    100,
				BatchSize:     10,
				FlushInterval: 1 * time.Second, // Long interval to test batch size trigger
				MaxRetries:    3,
			}
			var err error
			store, err = audit.NewBufferedStore(mockClient, config, "test-service", logger)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should batch events when batch size is reached", func() {
			// Store 10 events (batch size)
			for i := 0; i < 10; i++ {
				event := createTestEvent()
				_ = store.StoreAudit(ctx, event) // Intentionally ignore errors in test setup
			}

			// Wait for batch to be written
			Eventually(func() int {
				return mockClient.BatchCount()
			}, "2s").Should(Equal(1))

			Expect(mockClient.LastBatchSize()).To(Equal(10))
		})

		It("should flush partial batch after flush interval", func() {
			config := audit.Config{
				BufferSize:    100,
				BatchSize:     10,
				FlushInterval: 200 * time.Millisecond, // Short interval
				MaxRetries:    3,
			}
			var err error
			store, err = audit.NewBufferedStore(mockClient, config, "test-service", logger)
			Expect(err).ToNot(HaveOccurred())

			// Store 5 events (less than batch size)
			for i := 0; i < 5; i++ {
				event := createTestEvent()
				_ = store.StoreAudit(ctx, event) // Intentionally ignore errors in test setup
			}

			// Wait for flush interval
			Eventually(func() int {
				return mockClient.BatchCount()
			}, "1s").Should(Equal(1))

			Expect(mockClient.LastBatchSize()).To(Equal(5))
		})
	})

	Describe("Retry Logic", func() {
		BeforeEach(func() {
			config := audit.Config{
				BufferSize:    100,
				BatchSize:     10,
				FlushInterval: 100 * time.Millisecond,
				MaxRetries:    3,
			}
			var err error
			store, err = audit.NewBufferedStore(mockClient, config, "test-service", logger)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should retry on transient failure", func() {
			mockClient.SetFailUntilCall(2) // Fail first 2 attempts, succeed on 3rd

			// Store 10 events to trigger batch write
			for i := 0; i < 10; i++ {
				event := createTestEvent()
				_ = store.StoreAudit(ctx, event) // Intentionally ignore errors in test setup
			}

			// Wait for batch to be written (after retries)
			Eventually(func() int {
				return mockClient.BatchCount()
			}, "10s").Should(Equal(1))

			Expect(mockClient.AttemptCount()).To(BeNumerically(">=", 3))
			Expect(mockClient.FailureCount()).To(Equal(2))
		})

		It("should drop batch after max retries", func() {
			mockClient.SetShouldFail(true) // Fail all attempts

			// Store 10 events to trigger batch write
			for i := 0; i < 10; i++ {
				event := createTestEvent()
				_ = store.StoreAudit(ctx, event) // Intentionally ignore errors in test setup
			}

			// Wait for max retries
			Eventually(func() int {
				return mockClient.AttemptCount()
			}, "15s").Should(BeNumerically(">=", 3))

			Expect(mockClient.BatchCount()).To(Equal(0)) // Batch dropped
			Expect(mockClient.FailureCount()).To(BeNumerically(">=", 3))
		})

		// GAP-10: 4xx errors should NOT be retried (non-retryable)
		// BEHAVIOR: Client errors indicate invalid data, retry won't help
		// CORRECTNESS: Only 1 attempt should be made for 4xx errors
		It("should NOT retry on 4xx client error (GAP-10)", func() {
			// Set a 400 Bad Request error (non-retryable)
			mockClient.SetCustomError(audit.NewHTTPError(400, "Bad Request"))

			// Store 10 events to trigger batch write
			for i := 0; i < 10; i++ {
				event := createTestEvent()
				_ = store.StoreAudit(ctx, event)
			}

			// Wait for batch to be processed
			Eventually(func() int {
				return mockClient.AttemptCount()
			}, "5s").Should(BeNumerically(">=", 1))

			// GAP-10: Should NOT retry 4xx errors - only 1 attempt
			Expect(mockClient.AttemptCount()).To(Equal(1), "4xx errors should NOT be retried")
			Expect(mockClient.BatchCount()).To(Equal(0), "Batch should be dropped (not written)")
		})

		// GAP-10: 5xx errors SHOULD be retried (server errors)
		// BEHAVIOR: Server errors may be transient, retry may succeed
		// CORRECTNESS: Multiple attempts should be made for 5xx errors
		It("should retry on 5xx server error (GAP-10)", func() {
			// Set a 503 Service Unavailable error (retryable)
			mockClient.SetCustomError(audit.NewHTTPError(503, "Service Unavailable"))

			// Store 10 events to trigger batch write
			for i := 0; i < 10; i++ {
				event := createTestEvent()
				_ = store.StoreAudit(ctx, event)
			}

			// Wait for max retries
			Eventually(func() int {
				return mockClient.AttemptCount()
			}, "15s").Should(BeNumerically(">=", 3))

			// GAP-10: Should retry 5xx errors up to MaxRetries
			Expect(mockClient.AttemptCount()).To(BeNumerically(">=", 3), "5xx errors SHOULD be retried")
			Expect(mockClient.BatchCount()).To(Equal(0), "Batch should be dropped after max retries")
		})
	})

	// GAP-10: DLQ Fallback Tests (DD-009, ADR-032 "No Audit Loss")
	// Authority: DD-009 (Audit Write Error Recovery - Dead Letter Queue Pattern)
	// Business Requirement: BR-AUDIT-001 (Complete audit trail with no data loss)
	//
	// NOTE: Client-side DLQ removed in DD-AUDIT-002 V3.0 (see pkg/audit/store.go:101)
	// Server-side DLQ (in DataStorage service) handles persistence failures
	// These tests are PENDING until updated to test server-side DLQ behavior
	PDescribe("DLQ Fallback (GAP-10)", func() {
		var (
			mockDLQ *MockDLQClient
		)

		BeforeEach(func() {
			mockDLQ = NewMockDLQClient()
		})

		// BEHAVIOR: When primary write fails after max retries, events go to DLQ
		// CORRECTNESS: All events in batch should be enqueued to DLQ
		It("should enqueue batch to DLQ after max retries (GAP-10, DD-009)", func() {
			config := audit.Config{
				BufferSize:    100,
				BatchSize:     10,
				FlushInterval: 100 * time.Millisecond,
				MaxRetries:    3,
			}

			// Create store with DLQ client
			var err error
			store, err = audit.NewBufferedStore(mockClient, config, "test-service", logger)
			Expect(err).ToNot(HaveOccurred())

			// Make primary write fail permanently
			mockClient.SetShouldFail(true)

			// Store 10 events to trigger batch write
			for i := 0; i < 10; i++ {
				event := createTestEvent()
				_ = store.StoreAudit(ctx, event)
			}

			// Wait for max retries + DLQ fallback
			Eventually(func() int {
				return mockDLQ.EventCount()
			}, "20s").Should(Equal(10))

			// GAP-10: All 10 events should be in DLQ
			Expect(mockDLQ.EventCount()).To(Equal(10), "All events should be enqueued to DLQ")
			Expect(mockClient.BatchCount()).To(Equal(0), "No batches written to primary")
			Expect(mockClient.AttemptCount()).To(BeNumerically(">=", 3), "Should have retried")
		})

		// BEHAVIOR: DLQ receives the original error that caused failure
		// CORRECTNESS: Original error is preserved for debugging
		It("should include original error in DLQ entry (GAP-10)", func() {
			config := audit.Config{
				BufferSize:    100,
				BatchSize:     10,
				FlushInterval: 100 * time.Millisecond,
				MaxRetries:    3,
			}

			// Create store with DLQ client
			var err error
			store, err = audit.NewBufferedStore(mockClient, config, "test-service", logger)
			Expect(err).ToNot(HaveOccurred())

			// Set specific error
			mockClient.SetCustomError(audit.NewHTTPError(503, "Service Unavailable"))

			// Store events
			for i := 0; i < 10; i++ {
				event := createTestEvent()
				_ = store.StoreAudit(ctx, event)
			}

			// Wait for DLQ fallback
			Eventually(func() int {
				return mockDLQ.EventCount()
			}, "20s").Should(Equal(10))

			// Verify original errors are captured
			originalErrors := mockDLQ.GetOriginalErrors()
			Expect(len(originalErrors)).To(Equal(10))
			for _, origErr := range originalErrors {
				Expect(origErr).ToNot(BeNil())
				Expect(origErr.Error()).To(ContainSubstring("503"))
			}
		})

		// BEHAVIOR: Without DLQ, events are dropped with warning
		// CORRECTNESS: Log message indicates ADR-032 violation
		It("should log ADR-032 violation when no DLQ configured (GAP-10)", func() {
			config := audit.Config{
				BufferSize:    100,
				BatchSize:     10,
				FlushInterval: 100 * time.Millisecond,
				MaxRetries:    3,
			}

			// Create store WITHOUT DLQ
			var err error
			store, err = audit.NewBufferedStore(mockClient, config, "test-service", logger)
			Expect(err).ToNot(HaveOccurred())

			// Make primary write fail
			mockClient.SetShouldFail(true)

			// Store events
			for i := 0; i < 10; i++ {
				event := createTestEvent()
				_ = store.StoreAudit(ctx, event)
			}

			// Wait for max retries
			Eventually(func() int {
				return mockClient.AttemptCount()
			}, "15s").Should(BeNumerically(">=", 3))

			// Events should be dropped (no DLQ)
			Expect(mockClient.BatchCount()).To(Equal(0))
			// Note: In production, this would log "AUDIT DATA LOSS" warning
		})

		// BEHAVIOR: Partial DLQ success is better than total loss
		// CORRECTNESS: If some DLQ writes fail, others still succeed
		It("should continue DLQ writes even if some fail (GAP-10)", func() {
			config := audit.Config{
				BufferSize:    100,
				BatchSize:     5, // Smaller batch
				FlushInterval: 100 * time.Millisecond,
				MaxRetries:    3,
			}

			// Create a DLQ that fails after first 3 events
		failingDLQ := NewMockDLQClient()

		// Create store (DLQ functionality is internal)
		var err error
		store, err = audit.NewBufferedStore(mockClient, config, "test-service", logger)
		Expect(err).ToNot(HaveOccurred())

			// Make primary write fail
			mockClient.SetShouldFail(true)

			// Store 5 events
			for i := 0; i < 5; i++ {
				event := createTestEvent()
				_ = store.StoreAudit(ctx, event)
			}

			// Wait for DLQ fallback
			Eventually(func() int {
				return failingDLQ.EnqueueCount()
			}, "20s").Should(Equal(5))

			// All 5 events should have been attempted for DLQ
			Expect(failingDLQ.EnqueueCount()).To(Equal(5))
			Expect(failingDLQ.EventCount()).To(Equal(5)) // All succeeded
		})
	})

	Describe("Graceful Shutdown", func() {
		BeforeEach(func() {
			config := audit.Config{
				BufferSize:    100,
				BatchSize:     10,
				FlushInterval: 1 * time.Second, // Long interval
				MaxRetries:    3,
			}
			var err error
			store, err = audit.NewBufferedStore(mockClient, config, "test-service", logger)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should flush remaining events on close", func() {
			// Store 5 events (less than batch size)
			for i := 0; i < 5; i++ {
				event := createTestEvent()
				_ = store.StoreAudit(ctx, event) // Intentionally ignore errors in test setup
			}

			// Close immediately (before flush interval)
			err := store.Close()

			Expect(err).ToNot(HaveOccurred())
			Expect(mockClient.BatchCount()).To(Equal(1))
			Expect(mockClient.LastBatchSize()).To(Equal(5))
		})

		It("should not timeout on close with empty buffer", func() {
			err := store.Close()

			Expect(err).ToNot(HaveOccurred())
		})

		// GAP-12: ADR-032 requires Close() to report if events were lost
		It("should return error on close if events were dropped (ADR-032)", func() {
			// Configure store to fail all writes
			mockClient.SetShouldFail(true)

			// Store events (will fail after retries and be dropped)
			for i := 0; i < 20; i++ {
				event := createTestEvent()
				_ = store.StoreAudit(ctx, event)
			}

			// Wait for retries to complete and batches to fail
			Eventually(func() int {
				return mockClient.FailureCount()
			}, "20s").Should(BeNumerically(">=", 3)) // At least one batch retried

			// GAP-12: Close should report failure when events were lost
			err := store.Close()

			// ADR-032: Caller MUST know events were lost
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Or(
				ContainSubstring("failed batches"),
				ContainSubstring("dropped"),
			))
		})
	})

	Describe("Concurrent Access", func() {
		BeforeEach(func() {
			config := audit.Config{
				BufferSize:    1000,
				BatchSize:     100,
				FlushInterval: 100 * time.Millisecond,
				MaxRetries:    3,
			}
			var err error
			store, err = audit.NewBufferedStore(mockClient, config, "test-service", logger)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle concurrent writes", func() {
			var wg sync.WaitGroup
			numGoroutines := 10
			eventsPerGoroutine := 10

			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < eventsPerGoroutine; j++ {
						event := createTestEvent()
						_ = store.StoreAudit(ctx, event) // Intentionally ignore errors in test setup
					}
				}()
			}

			wg.Wait()

			// Wait for all events to be written
			Eventually(func() int {
				return mockClient.TotalEventsWritten()
			}, "5s").Should(Equal(numGoroutines * eventsPerGoroutine))
		})
	})
})
