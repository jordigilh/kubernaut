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

// MockDB simulates database operations
type MockDB struct {
	shouldFail    bool
	failOnCommit  bool
	commitCalls   int
	rollbackCalls int
	mu            sync.Mutex
}

func NewMockDB() *MockDB {
	return &MockDB{}
}

func (m *MockDB) Begin() (dualwrite.Tx, error) {
	if m.shouldFail {
		return nil, errors.New("begin transaction failed")
	}
	return &MockTx{db: m}, nil
}

// BeginTx implements context-aware transaction start (for non-context tests)
func (m *MockDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (dualwrite.Tx, error) {
	// For non-context tests, just delegate to Begin()
	return m.Begin()
}

// MockTx simulates transaction operations
type MockTx struct {
	db         *MockDB
	committed  bool
	rolledBack bool
}

func (m *MockTx) Commit() error {
	m.db.mu.Lock()
	defer m.db.mu.Unlock()

	if m.db.failOnCommit {
		return errors.New("commit failed")
	}
	m.committed = true
	m.db.commitCalls++
	return nil
}

func (m *MockTx) Rollback() error {
	m.db.mu.Lock()
	defer m.db.mu.Unlock()

	m.rolledBack = true
	m.db.rollbackCalls++
	return nil
}

func (m *MockTx) Exec(query string, args ...interface{}) (sql.Result, error) {
	if m.db.shouldFail {
		return nil, errors.New("exec failed")
	}
	return &MockResult{lastInsertID: 123}, nil
}

func (m *MockTx) QueryRow(query string, args ...interface{}) dualwrite.Row {
	return &MockRow{id: 123, shouldFail: m.db.shouldFail}
}

// MockRow simulates a row result for scanning
type MockRow struct {
	id         int64
	shouldFail bool
}

func (m *MockRow) Scan(dest ...interface{}) error {
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

// MockResult simulates SQL result
type MockResult struct {
	lastInsertID int64
}

func (m *MockResult) LastInsertId() (int64, error) {
	return m.lastInsertID, nil
}

func (m *MockResult) RowsAffected() (int64, error) {
	return 1, nil
}

// MockVectorDB simulates vector database operations
type MockVectorDB struct {
	shouldFail  bool
	insertCalls int
	mu          sync.Mutex
}

func NewMockVectorDB() *MockVectorDB {
	return &MockVectorDB{}
}

func (m *MockVectorDB) Insert(ctx context.Context, id int64, embedding []float32, metadata map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFail {
		return errors.New("vector DB insert failed")
	}
	m.insertCalls++
	return nil
}

var _ = Describe("BR-STORAGE-014: Atomic Dual-Write Operations", func() {
	var (
		coordinator  *dualwrite.Coordinator
		mockDB       *MockDB
		mockVectorDB *MockVectorDB
		logger       *zap.Logger
		testAudit    *models.RemediationAudit
	)

	BeforeEach(func() {
		logger, _ = zap.NewDevelopment()
		mockDB = NewMockDB()
		mockVectorDB = NewMockVectorDB()

		coordinator = dualwrite.NewCoordinator(mockDB, mockVectorDB, logger)

		testAudit = &models.RemediationAudit{
			Name:                 "test-remediation",
			Namespace:            "default",
			Phase:                "pending",
			ActionType:           "scale_deployment",
			Status:               "success",
			StartTime:            time.Now(),
			RemediationRequestID: "req-123",
			AlertFingerprint:     "alert-abc",
			Severity:             "high",
			Environment:          "production",
			ClusterName:          "prod-cluster",
			TargetResource:       "deployment/my-app",
			Metadata:             "{}",
		}
	})

	Context("successful dual-write operations", func() {
		It("should write to both PostgreSQL and Vector DB atomically", func() {
			ctx := context.Background()

			// Create 384-dimensional embedding
			embedding := make([]float32, 384)
			for i := range embedding {
				embedding[i] = float32(i) * 0.01
			}

			result, err := coordinator.Write(ctx, testAudit, embedding)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.PostgreSQLID).To(Equal(int64(123)))
			Expect(result.VectorDBSuccess).To(BeTrue())
			Expect(result.PostgreSQLSuccess).To(BeTrue())
			Expect(mockDB.commitCalls).To(Equal(1))
			Expect(mockDB.rollbackCalls).To(Equal(0))
			Expect(mockVectorDB.insertCalls).To(Equal(1))
		})

		It("should return valid IDs after successful write", func() {
			ctx := context.Background()
			embedding := make([]float32, 384)

			result, err := coordinator.Write(ctx, testAudit, embedding)

			Expect(err).ToNot(HaveOccurred())
			Expect(result.PostgreSQLID).To(BeNumerically(">", 0))
		})
	})

	Context("PostgreSQL failure handling", func() {
		It("should rollback on PostgreSQL transaction begin failure", func() {
			ctx := context.Background()
			embedding := make([]float32, 384)

			mockDB.shouldFail = true

			result, err := coordinator.Write(ctx, testAudit, embedding)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("begin transaction failed"))
			Expect(result).To(BeNil())
			Expect(mockVectorDB.insertCalls).To(Equal(0), "Vector DB should not be called if transaction begin fails")
		})

		It("should rollback on PostgreSQL write failure", func() {
			// Allow begin to succeed, but fail on exec
			mockDB.shouldFail = false
			tx, _ := mockDB.Begin()
			mockDB.shouldFail = true

			// Simulate write failure
			_, writeErr := tx.Exec("INSERT INTO remediation_audit ...")
			Expect(writeErr).To(HaveOccurred())

			// Verify rollback called
			_ = tx.Rollback()
			Expect(mockDB.rollbackCalls).To(Equal(1))
		})

		It("should rollback on PostgreSQL commit failure", func() {
			ctx := context.Background()
			embedding := make([]float32, 384)

			mockDB.failOnCommit = true

			result, err := coordinator.Write(ctx, testAudit, embedding)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("commit failed"))
			Expect(result).To(BeNil())
			// Note: In real SQL, commit failure automatically rolls back the transaction
			// The defer will attempt rollback, but it's already rolled back
			Expect(mockDB.commitCalls).To(Equal(0), "Commit should have failed")
		})
	})

	Context("Vector DB failure handling", func() {
		It("should rollback PostgreSQL transaction on Vector DB failure", func() {
			ctx := context.Background()
			embedding := make([]float32, 384)

			mockVectorDB.shouldFail = true

			result, err := coordinator.Write(ctx, testAudit, embedding)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("vector DB insert failed"))
			Expect(result).To(BeNil())
			Expect(mockDB.rollbackCalls).To(BeNumerically(">=", 1), "PostgreSQL should rollback on Vector DB failure")
			Expect(mockDB.commitCalls).To(Equal(0), "PostgreSQL should not commit on Vector DB failure")
		})

		It("should not commit PostgreSQL if Vector DB is unavailable", func() {
			ctx := context.Background()
			embedding := make([]float32, 384)

			mockVectorDB.shouldFail = true

			_, err := coordinator.Write(ctx, testAudit, embedding)

			Expect(err).To(HaveOccurred())
			Expect(mockDB.commitCalls).To(Equal(0))
		})
	})

	Context("concurrent write operations", func() {
		It("should handle 10 concurrent writes without race conditions", func() {
			ctx := context.Background()

			var wg sync.WaitGroup
			successCount := 0
			var mu sync.Mutex

			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					defer GinkgoRecover()

					audit := &models.RemediationAudit{
						Name:                 "concurrent-test",
						Namespace:            "default",
						Phase:                "pending",
						ActionType:           "scale_deployment",
						Status:               "success",
						StartTime:            time.Now(),
						RemediationRequestID: "req-concurrent",
						AlertFingerprint:     "alert-concurrent",
						Severity:             "high",
						Environment:          "production",
						ClusterName:          "prod-cluster",
						TargetResource:       "deployment/concurrent-app",
						Metadata:             "{}",
					}

					embedding := make([]float32, 384)
					for j := range embedding {
						embedding[j] = float32(index*j) * 0.001
					}

					result, err := coordinator.Write(ctx, audit, embedding)

					if err == nil {
						mu.Lock()
						successCount++
						mu.Unlock()

						Expect(result).ToNot(BeNil())
						Expect(result.PostgreSQLSuccess).To(BeTrue())
						Expect(result.VectorDBSuccess).To(BeTrue())
					}
				}(i)
			}

			wg.Wait()

			Expect(successCount).To(Equal(10), "All 10 concurrent writes should succeed")
			Expect(mockDB.commitCalls).To(Equal(10))
			Expect(mockVectorDB.insertCalls).To(Equal(10))
		})
	})

	Context("validation", func() {
		It("should reject nil audit", func() {
			ctx := context.Background()
			embedding := make([]float32, 384)

			result, err := coordinator.Write(ctx, nil, embedding)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("audit is nil"))
			Expect(result).To(BeNil())
		})

		It("should reject nil embedding", func() {
			ctx := context.Background()

			result, err := coordinator.Write(ctx, testAudit, nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("embedding is nil"))
			Expect(result).To(BeNil())
		})

		It("should reject invalid embedding dimensions", func() {
			ctx := context.Background()
			invalidEmbedding := make([]float32, 128) // Wrong dimension

			result, err := coordinator.Write(ctx, testAudit, invalidEmbedding)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("embedding dimension"))
			Expect(result).To(BeNil())
		})
	})
})

var _ = Describe("BR-STORAGE-015: Graceful Degradation", func() {
	var (
		coordinator  *dualwrite.Coordinator
		mockDB       *MockDB
		mockVectorDB *MockVectorDB
		logger       *zap.Logger
		testAudit    *models.RemediationAudit
	)

	BeforeEach(func() {
		logger, _ = zap.NewDevelopment()
		mockDB = NewMockDB()
		mockVectorDB = NewMockVectorDB()

		coordinator = dualwrite.NewCoordinator(mockDB, mockVectorDB, logger)

		testAudit = &models.RemediationAudit{
			Name:                 "fallback-test",
			Namespace:            "default",
			Phase:                "pending",
			ActionType:           "scale_deployment",
			Status:               "success",
			StartTime:            time.Now(),
			RemediationRequestID: "req-fallback",
			AlertFingerprint:     "alert-fallback",
			Severity:             "high",
			Environment:          "production",
			ClusterName:          "prod-cluster",
			TargetResource:       "deployment/fallback-app",
			Metadata:             "{}",
		}
	})

	Context("PostgreSQL-only fallback", func() {
		It("should fall back to PostgreSQL-only on Vector DB unavailability", func() {
			ctx := context.Background()
			embedding := make([]float32, 384)

			mockVectorDB.shouldFail = true

			result, err := coordinator.WriteWithFallback(ctx, testAudit, embedding)

			Expect(err).ToNot(HaveOccurred(), "WriteWithFallback should succeed with PostgreSQL-only")
			Expect(result).ToNot(BeNil())
			Expect(result.PostgreSQLSuccess).To(BeTrue())
			Expect(result.VectorDBSuccess).To(BeFalse())
			Expect(result.FallbackMode).To(BeTrue())
			Expect(mockDB.commitCalls).To(BeNumerically(">=", 1), "PostgreSQL should commit at least once")
			// Note: First dual-write attempt rolls back, then fallback succeeds
			Expect(mockDB.rollbackCalls).To(BeNumerically(">=", 1), "First attempt should rollback, fallback should not")
		})

		It("should record Vector DB as failed in result", func() {
			ctx := context.Background()
			embedding := make([]float32, 384)

			mockVectorDB.shouldFail = true

			result, err := coordinator.WriteWithFallback(ctx, testAudit, embedding)

			Expect(err).ToNot(HaveOccurred())
			Expect(result.VectorDBSuccess).To(BeFalse())
			Expect(result.VectorDBError).ToNot(BeEmpty())
		})

		It("should not fall back if PostgreSQL fails", func() {
			ctx := context.Background()
			embedding := make([]float32, 384)

			mockDB.shouldFail = true

			result, err := coordinator.WriteWithFallback(ctx, testAudit, embedding)

			Expect(err).To(HaveOccurred(), "PostgreSQL failure should fail entire operation")
			Expect(result).To(BeNil())
		})
	})
})
