# DD-STORAGE-001: Database Abstraction Choice - database/sql vs. ORM Framework

## Status
**✅ APPROVED** (2025-10-12)
**Last Reviewed**: 2025-10-12
**Confidence**: 90%

---

## Context & Problem

The Data Storage Service requires database operations for:
- Audit trail persistence (PostgreSQL)
- Vector embeddings storage (pgvector extension)
- Atomic dual-write coordination (PostgreSQL + Vector DB)
- High-throughput writes (<250ms p95 latency)

**Decision Point**: At 90% completion of Day 5 (Dual-Write Engine), we evaluated whether to:
- Continue with Go's standard `database/sql` package
- Switch to an ORM framework (GORM, sqlx)

---

## Alternatives Considered

### Alternative 1: Continue with `database/sql` ✅ APPROVED

**Approach**: Use Go's standard library `database/sql` with manual transaction management.

**Pros**:
- ✅ **Zero Migration Cost**: Already 90% complete with Day 5 implementation
- ✅ **Full Transaction Control**: Explicit `Begin()`, `Commit()`, `Rollback()` for BR-STORAGE-014 (atomic dual-write)
- ✅ **pgvector Compatible**: Direct SQL access for custom vector queries (`ORDER BY embedding <=> $1`)
- ✅ **Zero Overhead**: No ORM layer, direct database driver performance
- ✅ **Battle-Tested**: Standard library stability and community support
- ✅ **Testing**: Clean mock interfaces already working (`MockDB`, `MockTx`)
- ✅ **Transparent Behavior**: No hidden magic, explicit control flow

**Cons**:
- ❌ **Manual SQL**: Must write SQL query strings by hand
- ❌ **Boilerplate**: More code for query building and result scanning
- ❌ **Migration Management**: Requires separate tool (e.g., `golang-migrate`)

**Estimated Work**: 1-2 hours to complete Day 5

**Confidence**: 90%

---

### Alternative 2: Switch to GORM (Popular ORM)

**Approach**: Use `gorm.io/gorm` for automatic migrations and query building.

**Pros**:
- ✅ **Auto Migrations**: `db.AutoMigrate(&RemediationAudit{})` handles schema
- ✅ **Less Boilerplate**: `db.Create(&audit)` vs. manual INSERT
- ✅ **Query Builder**: Fluent API for complex queries
- ✅ **Hooks**: BeforeCreate, AfterCreate for timestamps

**Cons**:
- ❌ **pgvector Incompatibility**: No official pgvector support, must use raw SQL anyway
- ❌ **Dual-Write Complexity**: Closure-based `db.Transaction()` harder to coordinate with Vector DB
- ❌ **Migration Cost**: 16-24 hours to refactor all code, models, tests
- ❌ **Hidden Behavior**: Automatic timestamps, soft deletes can obscure bugs
- ❌ **Performance Overhead**: ORM layer adds 2-3x latency for bulk writes
- ❌ **Sunk Cost**: Wastes 6-8 hours of completed work

**Estimated Work**: 16-24 hours (2-3 days) to migrate + test

**Confidence**: 40% (rejected)

---

### Alternative 3: Switch to sqlx (Lightweight Extension)

**Approach**: Use `github.com/jmoiron/sqlx` for struct scanning convenience.

**Pros**:
- ✅ **Minimal Change**: Extends `database/sql`, not a replacement
- ✅ **Struct Scanning**: `db.Get(&audit, query, args...)` reduces boilerplate
- ✅ **Named Parameters**: `db.NamedExec()` for cleaner queries
- ✅ **Same Transaction API**: Compatible with existing `Begin()`/`Commit()`/`Rollback()`
- ✅ **No Magic**: Transparent behavior, just convenience methods

**Cons**:
- ❌ **Refactoring Cost**: 4-8 hours to switch interfaces and rewrite mocks
- ❌ **Still Manual SQL**: Query construction not automated
- ❌ **Moderate Benefit**: Only 30-40% less boilerplate vs. current approach
- ❌ **Sunk Cost**: Day 5 work must be refactored

**Estimated Work**: 4-8 hours to migrate + test

**Confidence**: 70% (considered but rejected for Day 5)

---

## Decision

**APPROVED: Alternative 1** - Continue with `database/sql`

**Rationale**:

1. **Sunk Cost is Minimal**: 90% complete with Day 5, switching wastes 6-8 hours of working code

2. **pgvector Requires Raw SQL**:
   - BR-STORAGE-012 (vector embeddings) needs custom SQL: `ORDER BY embedding <=> $1`
   - ORMs provide no benefit for semantic search queries
   - **Critical Insight**: Most complex queries can't be simplified by frameworks

3. **Dual-Write Atomicity is Critical**:
   - BR-STORAGE-014 requires precise transaction control
   - Current `shouldRollback` flag approach gives explicit control
   - GORM's closure-based transactions complicate Vector DB coordination

4. **Testing is Already Working**:
   - Mock interfaces clean and functional
   - 12/14 tests passing (2 updated for transaction semantics)
   - Framework switch requires rewriting all test mocks

5. **Performance Requirements**:
   - <250ms p95 latency for audit writes (BR-STORAGE-020)
   - `database/sql` has zero ORM overhead
   - 2-3x faster than GORM for high-throughput operations

6. **Time-to-Completion**:
   - Current approach: 1-2 hours to finish Day 5
   - sqlx migration: 4-8 hours
   - GORM migration: 16-24 hours

**Key Insight**: **"Don't switch horses mid-race"** - We're one sprint away from Day 5 completion with a clean, testable, performant solution.

---

## Implementation

### Primary Implementation Files

**Dual-Write Coordinator**:
- `pkg/datastorage/dualwrite/coordinator.go` - Atomic dual-write with explicit transaction control
- `pkg/datastorage/dualwrite/interfaces.go` - Clean abstractions (`DB`, `Tx`, `VectorDBClient`)

**Transaction Pattern**:
```go
// Begin transaction
tx, err := c.db.Begin()
if err != nil {
    return nil, fmt.Errorf("begin transaction failed: %w", err)
}

// Track rollback with explicit flag
shouldRollback := true
defer func() {
    if shouldRollback {
        _ = tx.Rollback()
    }
}()

// Write to PostgreSQL
pgID, err := c.writeToPostgreSQL(tx, audit, embedding)
if err != nil {
    return nil, err // rollback via defer
}

// Write to Vector DB
if err := c.vectorDB.Insert(ctx, pgID, embedding, metadata); err != nil {
    return nil, err // rollback via defer
}

// Commit
if err := tx.Commit(); err != nil {
    return nil, err // rollback via defer
}

// Success - disable rollback
shouldRollback = false
```

**Testing Pattern**:
```go
// Mock interfaces match database/sql behavior
type MockDB struct {
    shouldFail    bool
    commitCalls   int
    rollbackCalls int
}

func (m *MockDB) Begin() (dualwrite.Tx, error) {
    if m.shouldFail {
        return nil, errors.New("begin failed")
    }
    return &MockTx{db: m}, nil
}
```

---

## Consequences

### Positive

- ✅ **Fast Completion**: Day 5 finishes in 1-2 hours, not 4-24 hours
- ✅ **Full Control**: Explicit transaction management for complex dual-write atomicity
- ✅ **pgvector Ready**: Direct SQL access for vector similarity queries
- ✅ **Production Performance**: Zero ORM overhead, <250ms p95 latency achievable
- ✅ **Stable Foundation**: Standard library reliability and long-term support
- ✅ **Clean Testing**: Mock interfaces work seamlessly without framework magic

### Negative

- ⚠️ **Manual SQL** - Must write query strings by hand
  - **Mitigation**: Use SQL files in `internal/database/schema/` for complex queries
  - **Mitigation**: Consider `sqlx` for Day 6 (Query API) if boilerplate becomes painful

- ⚠️ **Boilerplate Scanning** - Manual `Scan()` for query results
  - **Mitigation**: Extract helper functions (e.g., `scanRemediationAudit()`)
  - **Mitigation**: Reassess at Day 6 (Query API) for potential `sqlx` adoption

- ⚠️ **Migration Management** - Need separate schema versioning tool
  - **Mitigation**: Already using SQL files + schema initializer (Day 2)
  - **Mitigation**: Add `golang-migrate` in Day 12 (Production Readiness)

### Neutral

- 🔄 **Hybrid Approach Possible**: Can add `sqlx` for query service (Day 6) while keeping `database/sql` for dual-write
- 🔄 **Learning Curve**: Team already proficient with `database/sql` (evidenced by Day 5 progress)
- 🔄 **Ecosystem Maturity**: Both `database/sql` and potential alternatives have strong communities

---

## Validation Results

### Confidence Assessment Progression

- **Initial Assessment (90% Day 5 completion)**: 85% confidence
- **After Framework Analysis**: 90% confidence
- **After User Approval**: 90% confidence (approved 2025-10-12)

### Key Validation Points

- ✅ **Day 5 Tests**: 12/14 passing (2 updated for transaction semantics)
- ✅ **Build Status**: Clean compilation, zero lint errors
- ✅ **Mock Testing**: Interface-based mocks work seamlessly
- ✅ **Transaction Control**: `shouldRollback` flag provides precise rollback handling
- ✅ **pgvector Queries**: Raw SQL access confirmed compatible

---

## Related Decisions

**Builds On**:
- Day 2 Schema Design: Idempotent DDL with SQL files
- Day 4 Embedding Pipeline: 384-dimensional vectors for pgvector

**Supports Business Requirements**:
- **BR-STORAGE-014**: Atomic dual-write operations (PostgreSQL + Vector DB)
- **BR-STORAGE-015**: Graceful degradation (PostgreSQL-only fallback)
- **BR-STORAGE-012**: Vector embeddings for semantic search
- **BR-STORAGE-020**: Performance targets (<250ms p95 latency)

**Future Decisions**:
- Day 6 (Query API): May revisit `sqlx` for query boilerplate reduction
- Day 12 (Production Readiness): Add `golang-migrate` for schema versioning

---

## Review & Evolution

### When to Revisit

- **Day 6 (Query API)**: If query result scanning becomes excessively verbose
- **Day 9 (BR Coverage Matrix)**: If test complexity increases with manual SQL
- **Performance Testing**: If latency exceeds targets (unlikely with `database/sql`)

### Success Metrics

- **Day 5 Completion**: ≤2 hours remaining work (Target: 1-2h)
- **Test Pass Rate**: 14/14 tests passing (Target: 100%)
- **Build Time**: <10s for dual-write package (Target: <30s)
- **Code Coverage**: ≥90% for coordinator (Target: 85%+)
- **Latency**: <250ms p95 for dual-write operations (Target: <250ms)

### Hybrid Approach (Future Consideration)

If query boilerplate becomes painful in Day 6+:

**Option**: Add `sqlx` for query service only
- **Keep**: `database/sql` for dual-write coordinator (complex transaction control)
- **Add**: `sqlx` for query API (struct scanning convenience)
- **Confidence**: 75% (adds complexity, mixing two APIs)

---

## Related Documentation

- [Implementation Plan V4.1](../IMPLEMENTATION_PLAN_V4.1.md) - Overall service implementation plan
- [Day 2 Complete](../phase0/02-day2-complete.md) - Schema design with SQL files
- [Day 4 Midpoint](../phase0/04-day4-midpoint.md) - Embedding pipeline with pgvector
- [Day 5 WIP](../phase0/05-day5-wip.md) - Dual-write coordinator implementation

---

**Decision Owner**: Jordi Gil
**Approved By**: User (2025-10-12)
**Implementation Status**: ✅ **IN PROGRESS** (Day 5: 90% complete)
**Next Review**: Day 6 (Query API) - Reassess query boilerplate


