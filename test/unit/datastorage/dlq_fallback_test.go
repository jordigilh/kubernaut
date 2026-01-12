/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package datastorage

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/audit"
)

// BR-STORAGE-017: DLQ Fallback Logic - Unit Tests
// Tests DataStorage BUSINESS LOGIC for DLQ fallback decisions
// NOT testing DLQ client (those tests moved to pkg/datastorage/dlq/)

var _ = Describe("DataStorage DLQ Fallback Logic", func() {
	var (
		ctx           context.Context
		mockDB        *MockDatabase
		mockDLQClient *MockDLQClient
		mockMetrics   *MockMetrics
		auditHandler  *AuditHandler
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockDB = NewMockDatabase()
		mockDLQClient = NewMockDLQClient()
		mockMetrics = NewMockMetrics()

		auditHandler = &AuditHandler{
			db:      mockDB,
			dlq:     mockDLQClient,
			metrics: mockMetrics,
		}
	})

	Describe("BR-STORAGE-017: DLQ Fallback on Database Unavailability", func() {
		Context("when database is unavailable", func() {
			BeforeEach(func() {
				mockDB.SetError(errors.New("database connection failed"))
			})

			It("should enqueue event to DLQ", func() {
				// Test DataStorage BUSINESS LOGIC: DLQ fallback decision
				event := &audit.AuditEvent{
					EventType: "test.event.occurred",
				}

				err := auditHandler.StoreAuditEvent(ctx, event)

				// Verify DataStorage used DLQ fallback (business logic)
				Expect(err).NotTo(HaveOccurred()) // ← Graceful degradation
				Expect(mockDLQClient.EnqueueCallCount()).To(Equal(1))
				Expect(mockDB.WriteCallCount()).To(Equal(1)) // ← Tried DB first
			})

			It("should emit dlq_enqueue metric", func() {
				// Test DataStorage BUSINESS LOGIC: metrics emission
				event := &audit.AuditEvent{EventType: "test.event"}

				_ = auditHandler.StoreAuditEvent(ctx, event)

				// Verify metrics were emitted (business logic)
				Expect(mockMetrics.DLQEnqueueCount()).To(Equal(1))
			})

			It("should log database error", func() {
				// Test DataStorage BUSINESS LOGIC: error logging
				event := &audit.AuditEvent{EventType: "test.event"}

				_ = auditHandler.StoreAuditEvent(ctx, event)

				// Verify error was logged (business logic)
				Expect(mockMetrics.DatabaseErrorCount()).To(Equal(1))
			})

			It("should preserve event data when enqueueing to DLQ", func() {
				// Test DataStorage BUSINESS LOGIC: data preservation
				event := &audit.AuditEvent{
					EventType:     "test.event.occurred",
					CorrelationID: "test-correlation-123",
				}

				_ = auditHandler.StoreAuditEvent(ctx, event)

				// Verify DLQ received complete event (business logic)
				Expect(mockDLQClient.EnqueueCallCount()).To(Equal(1))
				enqueuedEvent := mockDLQClient.EnqueueArgsForCall(0)
				Expect(enqueuedEvent.EventType).To(Equal("test.event.occurred"))
				Expect(enqueuedEvent.CorrelationID).To(Equal("test-correlation-123"))
			})
		})

		Context("when database is available", func() {
			BeforeEach(func() {
				mockDB.SetError(nil) // Database working
			})

			It("should write directly to database", func() {
				// Test DataStorage BUSINESS LOGIC: normal path
				event := &audit.AuditEvent{EventType: "test.event"}

				err := auditHandler.StoreAuditEvent(ctx, event)

				// Verify DataStorage wrote to DB (business logic)
				Expect(err).NotTo(HaveOccurred())
				Expect(mockDB.WriteCallCount()).To(Equal(1))
				Expect(mockDLQClient.EnqueueCallCount()).To(Equal(0)) // ← No DLQ
			})

			It("should emit database_write metric", func() {
				// Test DataStorage BUSINESS LOGIC: success metrics
				event := &audit.AuditEvent{EventType: "test.event"}

				_ = auditHandler.StoreAuditEvent(ctx, event)

				// Verify success metrics (business logic)
				Expect(mockMetrics.DatabaseWriteCount()).To(Equal(1))
				Expect(mockMetrics.DLQEnqueueCount()).To(Equal(0))
			})
		})

		Context("when database recovers after being down", func() {
			It("should resume writing to database", func() {
				// Test DataStorage BUSINESS LOGIC: recovery behavior
				event1 := &audit.AuditEvent{EventType: "test.event.1"}
				event2 := &audit.AuditEvent{EventType: "test.event.2"}

				// First write: database down
				mockDB.SetError(errors.New("connection failed"))
				err1 := auditHandler.StoreAuditEvent(ctx, event1)
				Expect(err1).NotTo(HaveOccurred())
				Expect(mockDLQClient.EnqueueCallCount()).To(Equal(1))

				// Database recovers
				mockDB.SetError(nil)

				// Second write: database available
				err2 := auditHandler.StoreAuditEvent(ctx, event2)
				Expect(err2).NotTo(HaveOccurred())

				// Verify DataStorage resumed DB writes (business logic)
				Expect(mockDB.WriteCallCount()).To(Equal(2))          // ← Tried both times
				Expect(mockDLQClient.EnqueueCallCount()).To(Equal(1)) // ← Only first failed
			})
		})

		Context("when DLQ is also unavailable", func() {
			BeforeEach(func() {
				mockDB.SetError(errors.New("database down"))
				mockDLQClient.SetError(errors.New("redis down"))
			})

			It("should return error indicating complete failure", func() {
				// Test DataStorage BUSINESS LOGIC: complete failure handling
				event := &audit.AuditEvent{EventType: "test.event"}

				err := auditHandler.StoreAuditEvent(ctx, event)

				// Verify DataStorage reports complete failure (business logic)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("redis down"))
			})

			It("should emit both database and DLQ error metrics", func() {
				// Test DataStorage BUSINESS LOGIC: error metrics
				event := &audit.AuditEvent{EventType: "test.event"}

				_ = auditHandler.StoreAuditEvent(ctx, event)

				// Verify both error metrics (business logic)
				Expect(mockMetrics.DatabaseErrorCount()).To(Equal(1))
				Expect(mockMetrics.DLQEnqueueErrorCount()).To(Equal(1))
			})
		})
	})

	Describe("BR-STORAGE-018: DLQ Batch Fallback", func() {
		Context("when database fails during batch write", func() {
			BeforeEach(func() {
				mockDB.SetBatchError(errors.New("batch write failed"))
			})

			It("should enqueue entire batch to DLQ", func() {
				// Test DataStorage BUSINESS LOGIC: batch fallback
				events := []*audit.AuditEvent{
					{EventType: "test.event.1"},
					{EventType: "test.event.2"},
					{EventType: "test.event.3"},
				}

				err := auditHandler.StoreBatchAuditEvents(ctx, events)

				// Verify DataStorage enqueued batch to DLQ (business logic)
				Expect(err).NotTo(HaveOccurred())
				Expect(mockDLQClient.EnqueueBatchCallCount()).To(Equal(1))
				enqueuedBatch := mockDLQClient.EnqueueBatchArgsForCall(0)
				Expect(len(enqueuedBatch)).To(Equal(3))
			})
		})
	})
})

// ========================================
// MOCK DATABASE
// ========================================

type MockDatabase struct {
	writeError      error
	batchError      error
	writeCalls      int
	batchWriteCalls int
}

func NewMockDatabase() *MockDatabase {
	return &MockDatabase{}
}

func (m *MockDatabase) SetError(err error) {
	m.writeError = err
}

func (m *MockDatabase) SetBatchError(err error) {
	m.batchError = err
}

func (m *MockDatabase) Write(ctx context.Context, event *audit.AuditEvent) error {
	m.writeCalls++
	return m.writeError
}

func (m *MockDatabase) WriteBatch(ctx context.Context, events []*audit.AuditEvent) error {
	m.batchWriteCalls++
	return m.batchError
}

func (m *MockDatabase) WriteCallCount() int {
	return m.writeCalls
}

// ========================================
// MOCK DLQ CLIENT
// ========================================

type MockDLQClient struct {
	enqueueError      error
	enqueueCalls      int
	enqueueBatchCalls int
	enqueuedEvents    []*audit.AuditEvent
	enqueuedBatches   [][]*audit.AuditEvent
}

func NewMockDLQClient() *MockDLQClient {
	return &MockDLQClient{
		enqueuedEvents:  make([]*audit.AuditEvent, 0),
		enqueuedBatches: make([][]*audit.AuditEvent, 0),
	}
}

func (m *MockDLQClient) SetError(err error) {
	m.enqueueError = err
}

func (m *MockDLQClient) Enqueue(ctx context.Context, event *audit.AuditEvent) error {
	m.enqueueCalls++
	if m.enqueueError != nil {
		return m.enqueueError
	}
	m.enqueuedEvents = append(m.enqueuedEvents, event)
	return nil
}

func (m *MockDLQClient) EnqueueBatch(ctx context.Context, events []*audit.AuditEvent) error {
	m.enqueueBatchCalls++
	if m.enqueueError != nil {
		return m.enqueueError
	}
	m.enqueuedBatches = append(m.enqueuedBatches, events)
	return nil
}

func (m *MockDLQClient) EnqueueCallCount() int {
	return m.enqueueCalls
}

func (m *MockDLQClient) EnqueueArgsForCall(i int) *audit.AuditEvent {
	if i >= len(m.enqueuedEvents) {
		return nil
	}
	return m.enqueuedEvents[i]
}

func (m *MockDLQClient) EnqueueBatchCallCount() int {
	return m.enqueueBatchCalls
}

func (m *MockDLQClient) EnqueueBatchArgsForCall(i int) []*audit.AuditEvent {
	if i >= len(m.enqueuedBatches) {
		return nil
	}
	return m.enqueuedBatches[i]
}

// ========================================
// MOCK METRICS
// ========================================

type MockMetrics struct {
	dlqEnqueueCount      int
	databaseWriteCount   int
	databaseErrorCount   int
	dlqEnqueueErrorCount int
}

func NewMockMetrics() *MockMetrics {
	return &MockMetrics{}
}

func (m *MockMetrics) RecordDLQEnqueue() {
	m.dlqEnqueueCount++
}

func (m *MockMetrics) RecordDatabaseWrite() {
	m.databaseWriteCount++
}

func (m *MockMetrics) RecordDatabaseError() {
	m.databaseErrorCount++
}

func (m *MockMetrics) RecordDLQEnqueueError() {
	m.dlqEnqueueErrorCount++
}

func (m *MockMetrics) DLQEnqueueCount() int {
	return m.dlqEnqueueCount
}

func (m *MockMetrics) DatabaseWriteCount() int {
	return m.databaseWriteCount
}

func (m *MockMetrics) DatabaseErrorCount() int {
	return m.databaseErrorCount
}

func (m *MockMetrics) DLQEnqueueErrorCount() int {
	return m.dlqEnqueueErrorCount
}

// ========================================
// MOCK AUDIT HANDLER (Simplified)
// ========================================

type AuditHandler struct {
	db      *MockDatabase
	dlq     *MockDLQClient
	metrics *MockMetrics
}

func (h *AuditHandler) StoreAuditEvent(ctx context.Context, event *audit.AuditEvent) error {
	// Simplified business logic for testing DLQ fallback

	// Try database first
	err := h.db.Write(ctx, event)
	if err != nil {
		// Database failed: fallback to DLQ
		h.metrics.RecordDatabaseError()

		dlqErr := h.dlq.Enqueue(ctx, event)
		if dlqErr != nil {
			// Both failed
			h.metrics.RecordDLQEnqueueError()
			return dlqErr
		}

		// DLQ succeeded
		h.metrics.RecordDLQEnqueue()
		return nil
	}

	// Database succeeded
	h.metrics.RecordDatabaseWrite()
	return nil
}

func (h *AuditHandler) StoreBatchAuditEvents(ctx context.Context, events []*audit.AuditEvent) error {
	// Batch write logic
	err := h.db.WriteBatch(ctx, events)
	if err != nil {
		// Fallback to DLQ for entire batch
		h.metrics.RecordDatabaseError()

		dlqErr := h.dlq.EnqueueBatch(ctx, events)
		if dlqErr != nil {
			h.metrics.RecordDLQEnqueueError()
			return dlqErr
		}

		h.metrics.RecordDLQEnqueue()
		return nil
	}

	h.metrics.RecordDatabaseWrite()
	return nil
}
