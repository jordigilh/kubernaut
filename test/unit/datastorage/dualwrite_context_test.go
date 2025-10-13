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

var _ = Describe("BR-STORAGE-016: Context Propagation", func() {
	var (
		coordinator  *dualwrite.Coordinator
		mockDB       *MockDBWithContext
		mockVectorDB *MockVectorDB
		logger       *zap.Logger
		testAudit    *models.RemediationAudit
	)

	BeforeEach(func() {
		logger, _ = zap.NewDevelopment()
		mockDB = NewMockDBWithContext()
		mockVectorDB = NewMockVectorDB()

		coordinator = dualwrite.NewCoordinator(mockDB, mockVectorDB, logger)

		testAudit = &models.RemediationAudit{
			Name:                 "context-test",
			Namespace:            "default",
			Phase:                "pending",
			ActionType:           "scale_deployment",
			Status:               "success",
			StartTime:            time.Now(),
			RemediationRequestID: "req-ctx-123",
			AlertFingerprint:     "alert-ctx",
			Severity:             "high",
			Environment:          "production",
			ClusterName:          "prod-cluster",
			TargetResource:       "deployment/ctx-app",
			Metadata:             "{}",
		}
	})

	// ⭐ TABLE-DRIVEN: Context signal handling
	DescribeTable("should respect context signals",
		func(ctxSetup func() context.Context, expectedErr error, description string) {
			ctx := ctxSetup()
			embedding := make([]float32, 384)
			for i := range embedding {
				embedding[i] = 0.1
			}

			_, err := coordinator.Write(ctx, testAudit, embedding)

			Expect(err).To(HaveOccurred(), description)
			Expect(errors.Is(err, expectedErr)).To(BeTrue(),
				"expected %v, got %v", expectedErr, err)
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

	Context("WriteWithFallback context propagation", func() {
		It("BR-STORAGE-016.4: should respect cancelled context in fallback path", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel before fallback

			embedding := make([]float32, 384)
			for i := range embedding {
				embedding[i] = 0.1
			}
			mockVectorDB.shouldFail = true // Force fallback

			_, err := coordinator.WriteWithFallback(ctx, testAudit, embedding)

			// Fallback should also respect cancellation
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		})
	})

	Context("context deadline during transaction", func() {
		It("BR-STORAGE-016.5: should timeout if transaction takes too long", func() {
			// Very short timeout
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()

			// Simulate slow database
			mockDB.slowMode = true
			mockDB.delay = 100 * time.Millisecond

			embedding := make([]float32, 384)
			for i := range embedding {
				embedding[i] = 0.1
			}

			_, err := coordinator.Write(ctx, testAudit, embedding)

			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
		})
	})

	Context("context used correctly", func() {
		It("BR-STORAGE-016.6: should call BeginTx with context (not Begin)", func() {
			ctx := context.Background()
			embedding := make([]float32, 384)
			for i := range embedding {
				embedding[i] = 0.1
			}

			_, err := coordinator.Write(ctx, testAudit, embedding)

			Expect(err).ToNot(HaveOccurred())
			Expect(mockDB.beginTxCalled).To(BeTrue(), "BeginTx should be called")
			Expect(mockDB.beginCalled).To(BeFalse(), "Begin() should NOT be called (legacy API)")
		})
	})
})

// MockDBWithContext - Context-aware mock that tracks Begin() vs BeginTx() calls
type MockDBWithContext struct {
	shouldFail     bool
	failOnCommit   bool
	slowMode       bool
	delay          time.Duration
	beginCalled    bool // Legacy Begin() called (BAD)
	beginTxCalled  bool // Modern BeginTx() called (GOOD)
	beginTxContext context.Context
	commitCalls    int
	rollbackCalls  int
	mu             sync.Mutex
}

func NewMockDBWithContext() *MockDBWithContext {
	return &MockDBWithContext{}
}

// BeginTx - Context-aware transaction start (CORRECT API)
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

	return &MockTxContext{dbWithContext: m}, nil
}

// Begin - Legacy API (INCORRECT - should not be called after fix)
func (m *MockDBWithContext) Begin() (dualwrite.Tx, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.beginCalled = true

	if m.shouldFail {
		return nil, errors.New("begin transaction failed")
	}

	// Return transaction, but this is the WRONG API to use
	return &MockTxContext{dbWithContext: m}, nil
}

// MockTxContext - Transaction mock for context-aware tests
type MockTxContext struct {
	dbWithContext *MockDBWithContext
	committed     bool
	rolledBack    bool
}

func (m *MockTxContext) Commit() error {
	m.dbWithContext.mu.Lock()
	defer m.dbWithContext.mu.Unlock()

	if m.dbWithContext.failOnCommit {
		return errors.New("commit failed")
	}
	m.committed = true
	m.dbWithContext.commitCalls++
	return nil
}

func (m *MockTxContext) Rollback() error {
	m.dbWithContext.mu.Lock()
	defer m.dbWithContext.mu.Unlock()

	m.rolledBack = true
	m.dbWithContext.rollbackCalls++
	return nil
}

func (m *MockTxContext) Exec(query string, args ...interface{}) (sql.Result, error) {
	if m.dbWithContext.shouldFail {
		return nil, errors.New("exec failed")
	}
	return &MockResultContext{lastInsertID: 123}, nil
}

func (m *MockTxContext) QueryRow(query string, args ...interface{}) dualwrite.Row {
	return &MockRowContext{id: 123, shouldFail: m.dbWithContext.shouldFail}
}

// MockRowContext simulates a row result for scanning
type MockRowContext struct {
	id         int64
	shouldFail bool
}

func (m *MockRowContext) Scan(dest ...interface{}) error {
	if m.shouldFail {
		return errors.New("scan failed")
	}
	if len(dest) > 0 {
		if idPtr, ok := dest[0].(*int64); ok {
			*idPtr = m.id
		}
	}
	return nil
}

// MockResultContext - SQL result mock
type MockResultContext struct {
	lastInsertID int64
}

func (m *MockResultContext) LastInsertId() (int64, error) {
	return m.lastInsertID, nil
}

func (m *MockResultContext) RowsAffected() (int64, error) {
	return 1, nil
}
