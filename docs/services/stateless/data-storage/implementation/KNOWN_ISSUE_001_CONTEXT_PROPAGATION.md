# KNOWN ISSUE 001: Context Parameter Not Propagated in Dual-Write Coordinator

**Date Identified**: October 12, 2025
**Severity**: MEDIUM
**Status**: 🔴 **OPEN** - To be fixed via TDD in Day 9
**Affects**: Day 5 (Dual-Write Engine)

---

## Issue Description

### Problem Statement

The `Coordinator.Write()` and `Coordinator.writePostgreSQLOnly()` methods accept `ctx context.Context` parameters but **do not propagate them to database operations**.

**Affected Code** (`pkg/datastorage/dualwrite/coordinator.go`):

```go
// Line 71 - Write() method
func (c *Coordinator) Write(ctx context.Context, audit *models.RemediationAudit, embedding []float32) (*WriteResult, error) {
    // ...
    tx, err := c.db.Begin()  // ❌ Should be: c.db.BeginTx(ctx, nil)
    // ...
}

// Line 234 - writePostgreSQLOnly() method
func (c *Coordinator) writePostgreSQLOnly(ctx context.Context, audit *models.RemediationAudit, embedding []float32) (int64, error) {
    tx, err := c.db.Begin()  // ❌ Should be: c.db.BeginTx(ctx, nil)
    // ...
}
```

### Impact

**Functional Impact**:
- ❌ Context cancellation is **ignored** (operations continue after `ctx.Done()`)
- ❌ Context deadlines are **not respected** (transactions don't timeout)
- ❌ Graceful shutdown is **incomplete** (in-flight writes don't stop on SIGTERM)

**Production Implications**:
- **Medium Severity**: System works, but graceful shutdown is impaired
- **No Data Loss**: Transactions still commit/rollback correctly
- **Performance**: No immediate performance impact
- **Observability**: Context tracing is broken (distributed tracing incomplete)

---

## Root Cause

### Why It Wasn't Caught

**Unit Tests (Day 5 - Current)**:
- ✅ 14 tests pass for dual-write logic
- ❌ **No context cancellation tests**
- ❌ **No context timeout tests**
- ❌ Mock `Begin()` doesn't verify context usage

**Test Gap**:
```go
// Current mock doesn't check context usage
type MockDB struct {
    // ...
}

func (m *MockDB) Begin() (dualwrite.Tx, error) {
    // ❌ No way to verify BeginTx(ctx, nil) was called
    return &MockTx{db: m}, nil
}
```

### Why It Exists

**Historical Context**:
- Day 5 TDD focused on **transaction atomicity** (Begin/Commit/Rollback)
- Context propagation was **assumed** but not tested
- `database/sql` has two APIs:
  - Legacy: `Begin()` (no context)
  - Modern: `BeginTx(ctx, opts)` (context-aware)

**Lesson Learned**: "Passing tests != correct behavior" - Test what you assume!

---

## TDD Fix Strategy (Day 9)

### Phase 1: Write Failing Tests (DO-RED)

**File**: `test/unit/datastorage/dualwrite_context_test.go` (NEW)

```go
package datastorage

import (
    "context"
    "errors"
    "testing"
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
        mockDB       *MockDBWithContext  // NEW: context-aware mock
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
        It("should respect cancelled context in fallback path", func() {
            ctx, cancel := context.WithCancel(context.Background())
            cancel() // Cancel before fallback

            embedding := make([]float32, 384)
            mockVectorDB.shouldFail = true // Force fallback

            _, err := coordinator.WriteWithFallback(ctx, testAudit, embedding)

            // Fallback should also respect cancellation
            Expect(err).To(HaveOccurred())
            Expect(errors.Is(err, context.Canceled)).To(BeTrue())
        })
    })

    Context("context deadline during long transaction", func() {
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
            Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
        })
    })
})

// MockDBWithContext - NEW: Verifies BeginTx was called with context
type MockDBWithContext struct {
    *MockDB
    slowMode          bool
    delay             time.Duration
    beginTxCalled     bool
    beginTxContext    context.Context
    mu                sync.Mutex
}

func NewMockDBWithContext() *MockDBWithContext {
    return &MockDBWithContext{
        MockDB: NewMockDB(),
    }
}

// BeginTx - Context-aware transaction start
func (m *MockDBWithContext) BeginTx(ctx context.Context, opts *sql.TxOptions) (dualwrite.Tx, error) {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.beginTxCalled = true
    m.beginTxContext = ctx

    // Check if context is already cancelled
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

    return &MockTx{db: m.MockDB}, nil
}

// Begin - Legacy API (should NOT be called after fix)
func (m *MockDBWithContext) Begin() (dualwrite.Tx, error) {
    // This should NOT be called after the fix
    panic("Begin() called instead of BeginTx() - context not propagated!")
}
```

**Expected Outcome**: ❌ **All 3+ tests FAIL** (exposing the bug)

---

### Phase 2: Fix Implementation (DO-GREEN)

**File**: `pkg/datastorage/dualwrite/interfaces.go`

```go
// Add BeginTx to DB interface
type DB interface {
    // Begin starts a new transaction (legacy - deprecated)
    Begin() (Tx, error)

    // BeginTx starts a new transaction with context (preferred)
    BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error)
}
```

**File**: `pkg/datastorage/dualwrite/coordinator.go`

```go
// Line 71 - Fix Write() method
func (c *Coordinator) Write(ctx context.Context, audit *models.RemediationAudit, embedding []float32) (*WriteResult, error) {
    // ... validation ...

    // ✅ FIX: Use BeginTx instead of Begin
    tx, err := c.db.BeginTx(ctx, nil)  // nil = default isolation level
    if err != nil {
        c.logger.Error("failed to begin transaction",
            zap.Error(err),
            zap.String("name", audit.Name))
        return nil, fmt.Errorf("begin transaction failed: %w", err)
    }

    // ... rest unchanged ...
}

// Line 234 - Fix writePostgreSQLOnly() method
func (c *Coordinator) writePostgreSQLOnly(ctx context.Context, audit *models.RemediationAudit, embedding []float32) (int64, error) {
    // ✅ FIX: Use BeginTx instead of Begin
    tx, err := c.db.BeginTx(ctx, nil)
    if err != nil {
        return 0, fmt.Errorf("begin transaction failed: %w", err)
    }

    // ... rest unchanged ...
}
```

**Expected Outcome**: ✅ **All 3+ tests PASS**

---

### Phase 3: Integration Stress Test (Day 7)

**File**: `test/integration/datastorage/context_cancellation_integration_test.go` (NEW)

```go
var _ = Describe("BR-STORAGE-016: Context Cancellation Stress Test", func() {
    It("should handle context cancellation during concurrent writes", func() {
        ctx, cancel := context.WithCancel(suite.Context)

        var wg sync.WaitGroup
        cancelledCount := 0
        successCount := 0
        var mu sync.Mutex

        // Start 20 concurrent writes
        for i := 0; i < 20; i++ {
            wg.Add(1)
            go func(index int) {
                defer GinkgoRecover()
                defer wg.Done()

                // Cancel context mid-flight (after 10 goroutines started)
                if index == 10 {
                    time.Sleep(50 * time.Millisecond)
                    cancel()
                }

                audit := &models.RemediationAudit{
                    Name:                 fmt.Sprintf("stress-test-%d", index),
                    Namespace:            "default",
                    Phase:                "processing",
                    ActionType:           "scale_deployment",
                    Status:               "pending",
                    StartTime:            time.Now(),
                    RemediationRequestID: fmt.Sprintf("req-stress-%d", index),
                    AlertFingerprint:     fmt.Sprintf("alert-stress-%d", index),
                    Severity:             "high",
                    Environment:          "production",
                    ClusterName:          "test-cluster",
                    TargetResource:       fmt.Sprintf("deployment/app-%d", index),
                    Metadata:             "{}",
                }

                embedding := make([]float32, 384)
                for j := range embedding {
                    embedding[j] = float32(index*j) * 0.001
                }

                _, err := client.CreateRemediationAudit(ctx, audit)

                mu.Lock()
                if err != nil {
                    if errors.Is(err, context.Canceled) {
                        cancelledCount++
                    }
                } else {
                    successCount++
                }
                mu.Unlock()
            }(i)
        }

        wg.Wait()

        GinkgoWriter.Printf("✅ Success: %d, ❌ Cancelled: %d\n", successCount, cancelledCount)

        // Assertions
        Expect(cancelledCount).To(BeNumerically(">", 0),
            "Some writes should be cancelled")
        Expect(successCount).To(BeNumerically(">", 0),
            "Some writes should succeed before cancellation")
        Expect(successCount + cancelledCount).To(Equal(20),
            "All goroutines should complete")

        // Verify database consistency (no partial writes)
        audits, err := client.ListRemediationAudits(suite.Context, &datastorage.ListOptions{
            Limit: 50,
        })
        Expect(err).ToNot(HaveOccurred())

        // Only successful writes should be persisted
        Expect(len(audits)).To(Equal(successCount),
            "Database should only contain successful writes (no partial writes)")
    })

    It("should respect server shutdown timeout", func() {
        // Simulate server shutdown with 5 second timeout
        ctx, cancel := context.WithTimeout(suite.Context, 5*time.Second)
        defer cancel()

        // Start long-running writes
        var wg sync.WaitGroup
        for i := 0; i < 5; i++ {
            wg.Add(1)
            go func(index int) {
                defer wg.Done()

                audit := &models.RemediationAudit{/* ... */}
                embedding := make([]float32, 384)

                // Should complete or cancel within timeout
                _, _ = client.CreateRemediationAudit(ctx, audit)
            }(i)
        }

        // Wait for timeout
        done := make(chan struct{})
        go func() {
            wg.Wait()
            close(done)
        }()

        // All writes should complete before timeout + 1 second grace period
        Eventually(done, 6*time.Second).Should(BeClosed(),
            "All writes should complete or cancel within shutdown timeout")
    })
})
```

---

## Documentation Updates

### Day 7 Implementation Plan Update

**Add to Integration Test Plan**:
```markdown
### Integration Test 6: Context Cancellation Stress Test (NEW - 30 min)

**Business Requirement**: BR-STORAGE-016 (Context cancellation handling)

**Test File**: `test/integration/datastorage/context_cancellation_integration_test.go`

**Test Scenarios**:
1. Concurrent writes with mid-flight cancellation
2. Server shutdown timeout compliance
3. No partial writes after cancellation

**Expected Outcome**:
- ❌ FAIL before fix (context ignored)
- ✅ PASS after fix (context respected)
```

---

### Day 9 Implementation Plan Update

**Add to Unit Test Plan**:
```markdown
### Unit Test Set 4: Context Propagation (NEW - 45 min)

**Business Requirement**: BR-STORAGE-016 (Context cancellation handling)

**Test File**: `test/unit/datastorage/dualwrite_context_test.go`

**Table-Driven Tests**:
- 3 DescribeTable entries (cancelled, deadline, timeout)
- MockDBWithContext for context verification
- Fallback path context tests

**TDD Workflow**:
1. DO-RED (20 min): Write failing tests exposing unused ctx
2. DO-GREEN (15 min): Fix Begin() → BeginTx(ctx, nil)
3. DO-REFACTOR (10 min): Update all tests to verify context

**Expected Outcome**:
- ❌ RED: 3+ tests fail (bug exposed)
- ✅ GREEN: All tests pass (bug fixed)
```

---

## Business Requirement Impact

### BR-STORAGE-016: Context Cancellation Handling (NEW)

**Requirement**: MUST respect context cancellation signals for graceful shutdown

**Acceptance Criteria**:
- ✅ Cancelled contexts fail fast without starting transactions
- ✅ Expired deadlines prevent transaction start
- ✅ In-flight transactions respect context timeouts
- ✅ No partial writes after cancellation

**Test Coverage**:
- Unit Tests: 3+ table-driven tests (Day 9)
- Integration Tests: 2 stress tests (Day 7)
- Coverage: 100%

---

## Success Metrics

### Before Fix (Current State)
- ❌ Context cancellation ignored: 100%
- ❌ Graceful shutdown incomplete: 100%
- ✅ Transaction atomicity preserved: 100%
- ✅ No data loss: 100%

### After Fix (Target State)
- ✅ Context cancellation respected: 100%
- ✅ Graceful shutdown complete: 100%
- ✅ Transaction atomicity preserved: 100%
- ✅ No data loss: 100%

---

## Related Issues

- **Day 5**: Dual-write coordinator implemented without context tests
- **Day 7**: Integration tests planned but missing context cancellation
- **Day 9**: Unit test coverage incomplete for context propagation

---

## Lesson Learned

**TDD Principle Reinforced**: "Write the test you wish you had"

**What Went Wrong**:
- Assumed context propagation without testing it
- Mock interface too simple (didn't verify BeginTx usage)
- Missing test category: Context lifecycle tests

**Prevention for Future**:
- ✅ Add "Context Propagation" to standard test checklist
- ✅ Always write cancellation/timeout tests for long operations
- ✅ Mock interfaces should verify method signatures match expectations

---

**Status**: 🔴 **OPEN** - To be fixed following TDD in Day 9
**Severity**: MEDIUM (graceful shutdown impaired, but no data loss)
**ETA**: Day 9 (45 minutes: 20min RED + 15min GREEN + 10min REFACTOR)

---

**Sign-off**: Jordi Gil
**Date**: October 12, 2025
**Next Action**: Add context tests to Day 9 plan, implement TDD fix


