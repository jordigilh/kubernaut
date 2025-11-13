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
	"database/sql"
	"errors"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/dualwrite"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// MockDBWithContext - Context-aware mock that verifies BeginTx was called with context
type MockDBWithContext struct {
	shouldFail     bool
	slowMode       bool
	delay          time.Duration
	beginTxCalled  bool
	beginTxContext context.Context
	beginCalled    bool
	commitCalls    int
	rollbackCalls  int
	mu             sync.Mutex
}

func NewMockDBWithContext() *MockDBWithContext {
	return &MockDBWithContext{}
}

// BeginTx - Context-aware transaction start (SHOULD be called)
func (m *MockDBWithContext) BeginTx(ctx context.Context, opts *sql.TxOptions) (dualwrite.Tx, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.beginTxCalled = true
	m.beginTxContext = ctx

	// Check if context is already cancelled/expired
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Simulate slow database if enabled
	if m.slowMode && m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if m.shouldFail {
		return nil, errors.New("begin transaction failed")
	}

	return &MockTxWithContext{db: m}, nil
}

// Begin - Legacy API (should NOT be called after fix)
func (m *MockDBWithContext) Begin() (dualwrite.Tx, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.beginCalled = true

	// This should NOT be called after the fix - fail the test if it is
	Fail("Begin() called instead of BeginTx() - context not propagated!")
	return nil, errors.New("Begin() should not be called - use BeginTx(ctx, nil) instead")
}

// MockTxWithContext simulates transaction operations with context tracking
type MockTxWithContext struct {
	db         *MockDBWithContext
	committed  bool
	rolledBack bool
}

func (m *MockTxWithContext) Commit() error {
	m.db.mu.Lock()
	defer m.db.mu.Unlock()

	m.committed = true
	m.db.commitCalls++
	return nil
}

func (m *MockTxWithContext) Rollback() error {
	m.db.mu.Lock()
	defer m.db.mu.Unlock()

	m.rolledBack = true
	m.db.rollbackCalls++
	return nil
}

func (m *MockTxWithContext) Exec(query string, args ...interface{}) (sql.Result, error) {
	// Note: This doesn't receive context directly, but BeginTx already checked context
	return nil, nil
}

func (m *MockTxWithContext) QueryRow(query string, args ...interface{}) dualwrite.Row {
	return &MockRow{}
}

// MockVectorDBForContext simulates vector database operations
type MockVectorDBForContext struct {
	shouldFail bool
	mu         sync.Mutex
}

func NewMockVectorDBForContext() *MockVectorDBForContext {
	return &MockVectorDBForContext{}
}

func (m *MockVectorDBForContext) Insert(ctx context.Context, id int64, embedding []float32, metadata map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check context during vector insert
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if m.shouldFail {
		return errors.New("vector insert failed")
	}
	return nil
}

func (m *MockVectorDBForContext) Close() error {
	return nil
}

var _ = Describe("BR-STORAGE-016: Context Propagation", func() {
	var (
		coordinator  *dualwrite.Coordinator
		mockDB       *MockDBWithContext
		mockVectorDB *MockVectorDBForContext
		logger       *zap.Logger
		testAudit    *models.RemediationAudit
	)

	BeforeEach(func() {
		var err error
		logger, err = zap.NewDevelopment()
		Expect(err).ToNot(HaveOccurred())

		mockDB = NewMockDBWithContext()
		mockVectorDB = NewMockVectorDBForContext()

		coordinator = dualwrite.NewCoordinator(mockDB, mockVectorDB, logger)

		testAudit = &models.RemediationAudit{
			Name:                 "context-test",
			Namespace:            "default",
			Phase:                "pending",
			ActionType:           "scale_deployment",
			Status:               "success",
			StartTime:            time.Now(),
			RemediationRequestID: "req-ctx-123",
			SignalFingerprint:    "alert-ctx",
			Severity:             "high",
			Environment:          "production",
			ClusterName:          "prod-cluster",
			TargetResource:       "deployment/ctx-app",
			Metadata:             "{}",
		}
	})

	Context("Write() method context propagation", func() {
		// ⭐ TABLE-DRIVEN: Context signal handling
		DescribeTable("should respect context signals",
			func(ctxSetup func() context.Context, expectedErr error, description string) {
				ctx := ctxSetup()
				embedding := make([]float32, 384)

				_, err := coordinator.Write(ctx, testAudit, embedding)

				Expect(err).To(HaveOccurred(), description)
				Expect(errors.Is(err, expectedErr)).To(BeTrue(),
					"expected %v, got %v", expectedErr, err)

				// Verify BeginTx was called (not Begin)
				Expect(mockDB.beginTxCalled).To(BeTrue(),
					"BeginTx(ctx, nil) should be called")
				Expect(mockDB.beginCalled).To(BeFalse(),
					"Begin() should NOT be called - context would be ignored")
			},

			Entry("BR-STORAGE-016.1: cancelled context should fail fast",
				func() context.Context {
					ctx, cancel := context.WithCancel(context.Background())
					cancel() // Cancel immediately
					return ctx
				},
				context.Canceled,
				"Write() should detect cancelled context and fail"),

			Entry("BR-STORAGE-016.2: expired deadline should fail fast",
				func() context.Context {
					// Deadline already passed
					ctx, cancel := context.WithDeadline(context.Background(),
						time.Now().Add(-1*time.Second))
					defer cancel()
					return ctx
				},
				context.DeadlineExceeded,
				"Write() should detect expired deadline and fail"),

			Entry("BR-STORAGE-016.3: zero timeout should fail fast",
				func() context.Context {
					ctx, cancel := context.WithTimeout(context.Background(), 0)
					defer cancel()
					return ctx
				},
				context.DeadlineExceeded,
				"Write() should detect zero timeout and fail"),
		)

		It("should propagate context to BeginTx", func() {
			ctx := context.Background()
			embedding := make([]float32, 384)

			_, err := coordinator.Write(ctx, testAudit, embedding)

			Expect(err).ToNot(HaveOccurred())

			// Verify BeginTx was called with the provided context
			Expect(mockDB.beginTxCalled).To(BeTrue(),
				"BeginTx should be called")
			Expect(mockDB.beginTxContext).To(Equal(ctx),
				"BeginTx should receive the same context passed to Write()")
			Expect(mockDB.beginCalled).To(BeFalse(),
				"Begin() should NOT be called")
		})

		It("should timeout if transaction takes too long", func() {
			// Very short timeout
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()

			// Simulate slow database
			mockDB.slowMode = true
			mockDB.delay = 100 * time.Millisecond

			embedding := make([]float32, 384)

			_, err := coordinator.Write(ctx, testAudit, embedding)

			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue(),
				"Write should fail with DeadlineExceeded when transaction takes too long")
		})
	})

	Context("WriteWithFallback() method context propagation", func() {
		It("should respect cancelled context in fallback path", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel before fallback

			embedding := make([]float32, 384)
			mockVectorDB.shouldFail = true // Force fallback

			_, err := coordinator.WriteWithFallback(ctx, testAudit, embedding)

			// Fallback should also respect cancellation
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, context.Canceled)).To(BeTrue(),
				"WriteWithFallback should respect context cancellation")

			// Verify BeginTx was called (not Begin)
			Expect(mockDB.beginTxCalled).To(BeTrue())
			Expect(mockDB.beginCalled).To(BeFalse())
		})

		It("should propagate context to PostgreSQL-only fallback", func() {
			ctx := context.Background()
			embedding := make([]float32, 384)
			mockVectorDB.shouldFail = true // Force PostgreSQL-only fallback

			_, err := coordinator.WriteWithFallback(ctx, testAudit, embedding)

			Expect(err).ToNot(HaveOccurred())

			// Verify context was propagated to fallback path
			Expect(mockDB.beginTxCalled).To(BeTrue(),
				"Fallback should call BeginTx(ctx, nil)")
			Expect(mockDB.beginTxContext).To(Equal(ctx),
				"Fallback should use the same context")
		})
	})

	Context("Concurrent writes with context cancellation", func() {
		It("should handle concurrent writes with mixed context states", func() {
			var wg sync.WaitGroup
			successCount := 0
			cancelledCount := 0
			var mu sync.Mutex

			// Launch 10 concurrent writes with different context states
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func(index int) {
					defer GinkgoRecover()
					defer wg.Done()

					var ctx context.Context
					var cancel context.CancelFunc

					// Half with valid contexts, half with cancelled contexts
					if index%2 == 0 {
						ctx = context.Background()
					} else {
						ctx, cancel = context.WithCancel(context.Background())
						cancel() // Cancel immediately
					}

					audit := &models.RemediationAudit{
						Name:                 testAudit.Name,
						Namespace:            testAudit.Namespace,
						Phase:                testAudit.Phase,
						ActionType:           testAudit.ActionType,
						Status:               testAudit.Status,
						StartTime:            time.Now(),
						RemediationRequestID: testAudit.RemediationRequestID + string(rune(index)),
						SignalFingerprint:    testAudit.SignalFingerprint,
						Severity:             testAudit.Severity,
						Environment:          testAudit.Environment,
						ClusterName:          testAudit.ClusterName,
						TargetResource:       testAudit.TargetResource,
						Metadata:             testAudit.Metadata,
					}

					embedding := make([]float32, 384)
					_, err := coordinator.Write(ctx, audit, embedding)

					mu.Lock()
					if err != nil && errors.Is(err, context.Canceled) {
						cancelledCount++
					} else if err == nil {
						successCount++
					}
					mu.Unlock()
				}(i)
			}

			wg.Wait()

			// Verify results
			Expect(successCount).To(Equal(5), "Half should succeed")
			Expect(cancelledCount).To(Equal(5), "Half should be cancelled")
		})
	})

	Context("Context deadline during operations", func() {
		It("should fail when deadline expires during write", func() {
			// Create context with very short deadline
			ctx, cancel := context.WithDeadline(context.Background(),
				time.Now().Add(5*time.Millisecond))
			defer cancel()

			// Simulate slow database
			mockDB.slowMode = true
			mockDB.delay = 50 * time.Millisecond

			embedding := make([]float32, 384)
			_, err := coordinator.Write(ctx, testAudit, embedding)

			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue(),
				"Should fail with DeadlineExceeded when deadline expires")
		})
	})

	Context("Context values preservation", func() {
		It("should preserve context values through call chain", func() {
			type contextKey string
			const requestIDKey contextKey = "request-id"

			ctx := context.WithValue(context.Background(), requestIDKey, "req-12345")
			embedding := make([]float32, 384)

			// ACT: Write with context containing request ID
			_, err := coordinator.Write(ctx, testAudit, embedding)

			// CORRECTNESS: Write succeeds
			Expect(err).ToNot(HaveOccurred(), "Write should succeed")

			// CORRECTNESS: Context was passed through with original values preserved
			value := mockDB.beginTxContext.Value(requestIDKey)
			Expect(value).To(Equal("req-12345"),
				"Context values should be preserved through call chain (not replaced with background context)")
		})
	})
})
