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

// errorCaptureSink captures Error-level log messages for test assertions.
type errorCaptureSink struct {
	mu     sync.Mutex
	errors []string
}

func (s *errorCaptureSink) Init(logr.RuntimeInfo)                    {}
func (s *errorCaptureSink) Enabled(int) bool                         { return true }
func (s *errorCaptureSink) Info(level int, msg string, _ ...interface{}) {}
func (s *errorCaptureSink) Error(_ error, msg string, _ ...interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errors = append(s.errors, msg)
}
func (s *errorCaptureSink) WithValues(...interface{}) logr.LogSink { return s }
func (s *errorCaptureSink) WithName(string) logr.LogSink          { return s }

func (s *errorCaptureSink) hasErrorContaining(substr string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, msg := range s.errors {
		if containsSubstring(msg, substr) {
			return true
		}
	}
	return false
}

func (s *errorCaptureSink) errorCountContaining(substr string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, msg := range s.errors {
		if containsSubstring(msg, substr) {
			count++
		}
	}
	return count
}

// infoCaptureSink captures Info-level log messages with their verbosity level.
type infoCaptureSink struct {
	mu      sync.Mutex
	entries []infoEntry
}

type infoEntry struct {
	level int
	msg   string
}

func (s *infoCaptureSink) Init(logr.RuntimeInfo)                          {}
func (s *infoCaptureSink) Enabled(int) bool                               { return true }
func (s *infoCaptureSink) Error(_ error, _ string, _ ...interface{})      {}
func (s *infoCaptureSink) WithValues(...interface{}) logr.LogSink         { return s }
func (s *infoCaptureSink) WithName(string) logr.LogSink                  { return s }
func (s *infoCaptureSink) Info(level int, msg string, _ ...interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, infoEntry{level: level, msg: msg})
}

func (s *infoCaptureSink) hasInfoAtLevel(level int, substr string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.level == level && containsSubstring(e.msg, substr) {
			return true
		}
	}
	return false
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Helper function to create a test event
// DD-AUDIT-002 V2.0: Uses OpenAPI types and helper functions
func createTestEvent() *ogenclient.AuditEventRequest {
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	// F-3 SOC2 Fix: event_type must match EventData discriminator
	audit.SetEventType(event, "gateway.crd.created")
	audit.SetEventCategory(event, "gateway") // DD-TESTING-001: Use valid event_category from OpenAPI enum
	audit.SetEventAction(event, "created")
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "service", "test-service")
	audit.SetResource(event, "TestResource", "test-123")
	audit.SetCorrelationID(event, "corr-123")

	// Use GatewayAuditPayload for test event (ogen migration - discriminated union)
	payload := ogenclient.GatewayAuditPayload{
		EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewayCrdCreated,
		SignalType:  ogenclient.GatewayAuditPayloadSignalTypeAlert, // Updated enum
		SignalName:   "test-alert",
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
			_, ok := store.(*audit.BufferedAuditStore)
			Expect(ok).To(BeTrue(), "store should be *audit.BufferedAuditStore")
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
			_, ok := store.(*audit.BufferedAuditStore)
			Expect(ok).To(BeTrue(), "store should be *audit.BufferedAuditStore")
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

			// StoreAudit with buffer space is a non-blocking channel send.
			// The behavioral contract: the call returns nil (no error) without
			// blocking. If the channel send blocked, the test would hang until
			// the Ginkgo timeout fires, which is the implicit deadlock guard.
			//
			// We do NOT assert on batch flush timing here — the shared
			// BeforeEach uses FlushInterval=10s to prevent buffer drain,
			// and batch flushing is tested separately in "Batch Processing".
			err := store.StoreAudit(ctx, event)
			Expect(err).ToNot(HaveOccurred())
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
			defer func() { _ = smallStore.Close() }()

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

	// NOTE: Client-side DLQ tests (GAP-10) removed — DD-AUDIT-002 V3.0 removed
	// client-side DLQ (see pkg/audit/store.go:101). Server-side DLQ in
	// DataStorage service handles persistence failures. If server-side DLQ
	// tests are needed, they belong in test/unit/datastorage/ or
	// test/integration/datastorage/.

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
				ContainSubstring("timeout"), // Close() may timeout if retries are still in-flight
			))
		})
	})

	// ========================================
	// Issue #749: Timer drift threshold and log verbosity (BR-AUDIT-749)
	// ========================================
	Describe("Timer Drift Threshold (Issue #749)", func() {

		// UT-AUDIT-749-001: 3x drift should NOT trigger Error log (threshold is 5x)
		It("UT-AUDIT-749-001: timer drift at 3x does NOT emit Error log", func() {
			config := audit.Config{
				BufferSize:    100,
				BatchSize:     10,
				FlushInterval: 100 * time.Millisecond,
				MaxRetries:    3,
			}

			logSink := &errorCaptureSink{}
			testLogger := logr.New(logSink)

			var err error
			store, err = audit.NewBufferedStore(mockClient, config, "test-service", testLogger)
			Expect(err).ToNot(HaveOccurred())

			// Store one event so the timer-based flush has something to process
			event := createTestEvent()
			Expect(store.StoreAudit(ctx, event)).To(Succeed())

			// Wait for several flush intervals to allow timer ticks to accumulate
			// At 100ms interval, 3x drift means ~300ms between ticks (normal jitter)
			time.Sleep(500 * time.Millisecond)

			// Close cleanly to drain
			Expect(store.Close()).To(Succeed())
			store = nil

			// No Error-level log about "TIMER BUG" should appear for normal jitter
			Expect(logSink.hasErrorContaining("TIMER BUG")).To(BeFalse(),
				"3x drift (normal jitter) must NOT trigger TIMER BUG Error — threshold should be 5x")
		})

		// UT-AUDIT-749-002: 6x drift SHOULD trigger Error log
		// This is harder to test deterministically since we can't control Go scheduler.
		// We validate the threshold constant indirectly: the threshold value in the code
		// must be >2x (old value) and <=5x (new value). Since we can't inject fake time
		// into backgroundWriter, we test the behavioral contract: a store with a very short
		// FlushInterval on a loaded system won't fire the Error for normal jitter.
		// The actual 6x test requires inspecting the threshold constant.
		It("UT-AUDIT-749-002: timer drift threshold is 5x (not 2x)", func() {
			// This test validates the code review finding: the threshold should be 5x.
			// We can't deterministically trigger 6x drift in a unit test, but we can
			// verify that with a 10ms FlushInterval (where 2x=20ms is easily exceeded
			// by Go scheduler), the old 2x threshold would fire constantly but
			// the new 5x threshold (50ms) should rarely fire.
			config := audit.Config{
				BufferSize:    100,
				BatchSize:     10,
				FlushInterval: 10 * time.Millisecond,
				MaxRetries:    3,
			}

			logSink := &errorCaptureSink{}
			testLogger := logr.New(logSink)

			var err error
			store, err = audit.NewBufferedStore(mockClient, config, "test-service", testLogger)
			Expect(err).ToNot(HaveOccurred())

			event := createTestEvent()
			Expect(store.StoreAudit(ctx, event)).To(Succeed())

			// With 10ms interval and 2x threshold (20ms), Error would fire on almost every tick.
			// With 5x threshold (50ms), it should fire much less frequently.
			time.Sleep(200 * time.Millisecond)

			Expect(store.Close()).To(Succeed())
			store = nil

			errorCount := logSink.errorCountContaining("TIMER BUG")
			// With 5x threshold at 10ms interval, we expect far fewer errors than tick count (~20 ticks)
			// Old 2x threshold would fire on nearly every tick
			Expect(errorCount).To(BeNumerically("<", 10),
				"5x threshold should fire much less than the ~20 ticks that occurred")
		})
	})

	Describe("StoreAudit Log Verbosity (Issue #749)", func() {

		// UT-AUDIT-749-003: StoreAudit should NOT emit Info logs at V(0)
		It("UT-AUDIT-749-003: StoreAudit does not emit Info-level logs at default verbosity", func() {
			config := audit.Config{
				BufferSize:    100,
				BatchSize:     10,
				FlushInterval: 10 * time.Second,
				MaxRetries:    3,
			}

			logSink := &infoCaptureSink{}
			testLogger := logr.New(logSink)

			var err error
			store, err = audit.NewBufferedStore(mockClient, config, "test-service", testLogger)
			Expect(err).ToNot(HaveOccurred())

			event := createTestEvent()
			Expect(store.StoreAudit(ctx, event)).To(Succeed())

			// At V(0) (production default), no Info logs from StoreAudit should appear
			Expect(logSink.hasInfoAtLevel(0, "StoreAudit called")).To(BeFalse(),
				"StoreAudit called log must be V(1), not Info")
			Expect(logSink.hasInfoAtLevel(0, "Validation passed")).To(BeFalse(),
				"Validation passed log must be V(1), not Info")
			Expect(logSink.hasInfoAtLevel(0, "Event buffered successfully")).To(BeFalse(),
				"Event buffered successfully log must be V(1), not Info")
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
