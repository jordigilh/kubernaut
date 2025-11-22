package audit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	audit "github.com/jordigilh/kubernaut/pkg/audit"
)

// MockDataStorageClient is a mock implementation of audit.DataStorageClient for testing
type MockDataStorageClient struct {
	mu            sync.Mutex
	batches       [][]*audit.AuditEvent
	failureCount  int32
	attemptCount  int32
	shouldFail    bool
	failUntilCall int32
}

func NewMockDataStorageClient() *MockDataStorageClient {
	return &MockDataStorageClient{
		batches: make([][]*audit.AuditEvent, 0),
	}
}

func (m *MockDataStorageClient) StoreBatch(ctx context.Context, events []*audit.AuditEvent) error {
	atomic.AddInt32(&m.attemptCount, 1)

	m.mu.Lock()
	defer m.mu.Unlock()

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
	batch := make([]*audit.AuditEvent, len(events))
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

func (m *MockDataStorageClient) SetFailUntilCall(failUntilCall int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failUntilCall = int32(failUntilCall)
}

func (m *MockDataStorageClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batches = make([][]*audit.AuditEvent, 0)
	atomic.StoreInt32(&m.failureCount, 0)
	atomic.StoreInt32(&m.attemptCount, 0)
	m.shouldFail = false
	m.failUntilCall = 0
}

// Helper function to create a test event
func createTestEvent() *audit.AuditEvent {
	payload := map[string]interface{}{"test": "data"}
	eventData := audit.NewEventData("test-service", "test_operation", "success", payload)
	eventDataJSON, _ := eventData.ToJSON()

	event := audit.NewAuditEvent()
	event.EventType = "test.event.created"
	event.EventCategory = "test"
	event.EventAction = "created"
	event.EventOutcome = "success"
	event.ActorType = "service"
	event.ActorID = "test-service"
	event.ResourceType = "TestResource"
	event.ResourceID = "test-123"
	event.CorrelationID = "corr-123"
	event.EventData = eventDataJSON

	return event
}

var _ = Describe("BufferedAuditStore", func() {
	var (
		store      audit.AuditStore
		mockClient *MockDataStorageClient
		logger     *zap.Logger
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockClient = NewMockDataStorageClient()
		// Use zap logger per DD-005 (all services use zap)
		logger, _ = zap.NewDevelopment()
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

		It("should return error if logger is nil", func() {
			config := audit.DefaultConfig()

			_, err := audit.NewBufferedStore(mockClient, config, "test-service", nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("logger cannot be nil"))
		})

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
				FlushInterval: 100 * time.Millisecond,
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
			event := &audit.AuditEvent{} // Invalid (missing required fields)

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

		It("should drop event when buffer is full (graceful degradation)", func() {
			// Fill buffer
			for i := 0; i < 100; i++ {
				event := createTestEvent()
				_ = store.StoreAudit(ctx, event) // Intentionally ignore errors in test setup
			}

			// Next event should be dropped (but not error)
			event := createTestEvent()
			err := store.StoreAudit(ctx, event)

			Expect(err).ToNot(HaveOccurred()) // Graceful degradation - no error
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
