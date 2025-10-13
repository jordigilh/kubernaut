# DD-STORAGE-002: Hybrid Database Approach - sqlx for Queries, database/sql for Dual-Write

> **Historical Note**: This document references implementation checkpoint files (phase0/XX-dayX-complete.md)
> that were removed during documentation cleanup on October 13, 2025. These references show when
> the decision was implemented. The referenced files are available in git history (commit d6de6702) if needed.

## Status
**✅ APPROVED** (2025-10-12)
**Last Reviewed**: 2025-10-12
**Confidence**: 85%

---

## Context & Problem

Day 6 (Query API) requires extensive row scanning for audit retrieval with:
- **4 audit types** (RemediationAudit, AIAnalysisAudit, WorkflowAudit, ExecutionAudit)
- **18-20 fields per audit type**
- **Multiple query patterns** (filtering, pagination, semantic search, sorting)

**Pain Point**: Manual `rows.Scan()` with `database/sql` requires ~20 lines of boilerplate per query:
```go
for rows.Next() {
    audit := &models.RemediationAudit{}
    err := rows.Scan(
        &audit.ID, &audit.Name, &audit.Namespace, &audit.Phase,
        &audit.ActionType, &audit.Status, &audit.StartTime, &audit.EndTime,
        &audit.Duration, &audit.RemediationRequestID, &audit.AlertFingerprint,
        &audit.Severity, &audit.Environment, &audit.ClusterName,
        &audit.TargetResource, &audit.ErrorMessage, &audit.Metadata,
        &audit.Embedding, &audit.CreatedAt, &audit.UpdatedAt,
    ) // 18 fields - error-prone, hard to maintain
}
```

**Decision Point**: How to reduce query boilerplate for Day 6 without compromising Day 5's explicit transaction control?

---

## Alternatives Considered

### Alternative 1: Pure database/sql (Consistent)

**Approach**: Continue with `database/sql` for all operations (Day 5 + Day 6).

**Pros**:
- ✅ **Single API**: Consistency across entire service
- ✅ **Zero Dependencies**: No new libraries
- ✅ **Explicit Control**: Full visibility into all database operations
- ✅ **DD-STORAGE-001 Alignment**: Matches previous design decision

**Cons**:
- ❌ **High Boilerplate**: ~20 lines per query × 4 audit types × 10+ query methods = 800+ lines
- ❌ **Error-Prone**: Easy to mismatch column order with struct fields
- ❌ **Maintenance Burden**: Adding/removing fields requires updating 10+ `Scan()` calls
- ❌ **Code Duplication**: Similar scanning logic repeated across query methods

**Estimated Work**: 8 hours (2h RED + 4h GREEN + 2h REFACTOR)

**Confidence**: 60% (rejected due to high boilerplate cost)

---

### Alternative 2: Hybrid Approach (sqlx for queries, database/sql for dual-write) ✅ APPROVED

**Approach**: Use `sqlx` for Day 6 Query API, preserve `database/sql` for Day 5 Dual-Write Coordinator.

**Pros**:
- ✅ **80% Less Boilerplate**: `sqlx.Select()` replaces manual `Scan()` loops (20 lines → 1 line)
- ✅ **Type Safety**: Struct tags (`db:"field_name"`) ensure correct field mapping
- ✅ **Maintainability**: Add/remove fields in one place (struct definition)
- ✅ **Clean Separation**: Writes need precision (database/sql), reads benefit from convenience (sqlx)
- ✅ **Preserves Day 5**: No changes to working, tested dual-write logic
- ✅ **Minimal Dependency**: `sqlx` extends `database/sql`, doesn't replace it
- ✅ **pgvector Compatible**: Raw SQL still accessible for vector similarity queries

**Cons**:
- ⚠️ **Mixed APIs**: Two database libraries in codebase (mitigated by clear separation)
- ⚠️ **Learning Curve**: Team needs to understand both (minimal, sqlx is very similar)
- ⚠️ **Migration Time**: 1-2 hours to add struct tags and update mocks

**Estimated Work**: 6-7 hours (2h RED + 3h GREEN with sqlx + 2h REFACTOR)
**Time Savings**: 1-2 hours vs. Alternative 1

**Confidence**: 85% (approved)

---

### Alternative 3: Full Migration to sqlx

**Approach**: Migrate both Day 5 (Dual-Write) and Day 6 (Query) to `sqlx`.

**Pros**:
- ✅ **Single API**: Consistency across entire service
- ✅ **Less Boilerplate**: Benefits for both write and read operations

**Cons**:
- ❌ **Sunk Cost**: Must refactor completed Day 5 code (285 lines)
- ❌ **Contradicts DD-STORAGE-001**: User-approved design decision for `database/sql`
- ❌ **High Risk**: Introduces changes to working, tested dual-write coordinator
- ❌ **Migration Time**: 4-6 hours to refactor Day 5 + tests + mocks
- ❌ **No Added Value**: Day 5 dual-write doesn't suffer from boilerplate (single INSERT)

**Estimated Work**: 10-12 hours (4-6h refactor Day 5 + 6h Day 6)
**Time Penalty**: 2-4 hours vs. Alternative 1

**Confidence**: 40% (rejected due to high cost, low benefit)

---

## Decision

**APPROVED: Alternative 2** - Hybrid Approach (sqlx for queries, database/sql for dual-write)

**Rationale**:

1. **Addresses Real Pain Point**: Day 6 queries have 18-20 fields × 4 audit types × 10+ methods = significant boilerplate
   - **Problem**: Manual scanning is error-prone and hard to maintain
   - **Solution**: `sqlx.Select()` reduces 20 lines to 1 line per query

2. **Preserves Day 5 Design**: Dual-write coordinator keeps explicit transaction control
   - **Insight**: Dual-write has only 1 INSERT statement, not suffering from boilerplate
   - **Benefit**: No need to refactor working, tested code

3. **Clean Architectural Separation**:
   - **Writes (Day 5)**: Need explicit control → `database/sql` with `shouldRollback` flag
   - **Reads (Day 6)**: Need convenience → `sqlx` with struct scanning
   - **Result**: Best tool for each job

4. **Time Efficiency**: Saves 1-2 hours vs. pure `database/sql`
   - Alternative 1: 8 hours
   - Alternative 2: 6-7 hours ✅
   - Alternative 3: 10-12 hours

5. **Risk Mitigation**: Zero changes to Day 5 (no regression risk)

6. **pgvector Compatible**: Raw SQL still accessible for semantic search
   - `sqlx` doesn't hide SQL, just automates scanning
   - Custom queries like `ORDER BY embedding <=> $1` still work

**Key Insight**: **"Use the right tool for the job"** - explicit control for writes, convenience for reads.

---

## Implementation

### Hybrid Architecture

```
pkg/datastorage/
├── dualwrite/          # Day 5 - database/sql
│   ├── coordinator.go  # Explicit transaction control
│   └── interfaces.go   # DB, Tx, VectorDBClient interfaces
├── query/              # Day 6 - sqlx (NEW)
│   ├── service.go      # Struct scanning convenience
│   └── interfaces.go   # QueryService interface
```

### Struct Tags for sqlx

**File**: `pkg/datastorage/models/audit.go`

```go
type RemediationAudit struct {
    ID                   int64      `json:"id" db:"id"`
    Name                 string     `json:"name" db:"name"`
    Namespace            string     `json:"namespace" db:"namespace"`
    Phase                string     `json:"phase" db:"phase"`
    ActionType           string     `json:"action_type" db:"action_type"`
    Status               string     `json:"status" db:"status"`
    StartTime            time.Time  `json:"start_time" db:"start_time"`
    EndTime              *time.Time `json:"end_time,omitempty" db:"end_time"`
    Duration             *int64     `json:"duration,omitempty" db:"duration"`
    RemediationRequestID string     `json:"remediation_request_id" db:"remediation_request_id"`
    AlertFingerprint     string     `json:"alert_fingerprint" db:"alert_fingerprint"`
    Severity             string     `json:"severity" db:"severity"`
    Environment          string     `json:"environment" db:"environment"`
    ClusterName          string     `json:"cluster_name" db:"cluster_name"`
    TargetResource       string     `json:"target_resource" db:"target_resource"`
    ErrorMessage         *string    `json:"error_message,omitempty" db:"error_message"`
    Metadata             string     `json:"metadata" db:"metadata"`
    Embedding            []float32  `json:"embedding,omitempty" db:"embedding"`
    CreatedAt            time.Time  `json:"created_at" db:"created_at"`
    UpdatedAt            time.Time  `json:"updated_at" db:"updated_at"`
}
```

### Query Service Pattern (sqlx)

**File**: `pkg/datastorage/query/service.go`

```go
package query

import (
    "context"
    "fmt"

    "github.com/jmoiron/sqlx"
    "github.com/jordigilh/kubernaut/pkg/datastorage/models"
    "go.uber.org/zap"
)

type Service struct {
    db     *sqlx.DB
    logger *zap.Logger
}

func NewService(db *sqlx.DB, logger *zap.Logger) *Service {
    return &Service{db: db, logger: logger}
}

// ListRemediationAudits - sqlx automatically scans all fields
func (s *Service) ListRemediationAudits(ctx context.Context, opts *ListOptions) ([]*models.RemediationAudit, error) {
    var audits []*models.RemediationAudit

    query, args := s.buildQuery(opts)

    // sqlx handles all 18 fields automatically via struct tags
    err := s.db.SelectContext(ctx, &audits, query, args...)
    if err != nil {
        return nil, fmt.Errorf("query failed: %w", err)
    }

    return audits, nil
}
```

### Dual-Write Coordinator Pattern (database/sql)

**File**: `pkg/datastorage/dualwrite/coordinator.go` (UNCHANGED)

```go
// Day 5 code remains unchanged - uses database/sql for explicit control
func (c *Coordinator) Write(ctx context.Context, audit *models.RemediationAudit, embedding []float32) (*WriteResult, error) {
    tx, err := c.db.Begin() // database/sql transaction
    if err != nil {
        return nil, fmt.Errorf("begin failed: %w", err)
    }

    shouldRollback := true
    defer func() {
        if shouldRollback {
            _ = tx.Rollback()
        }
    }()

    // ... explicit transaction control preserved
}
```

---

## Consequences

### Positive

- ✅ **80% Less Query Boilerplate**: 20 lines → 1 line per query method
- ✅ **Type-Safe Field Mapping**: Struct tags prevent column-field mismatches
- ✅ **Easy Maintenance**: Add/remove fields in struct definition only
- ✅ **Preserves Day 5 Quality**: Zero changes to tested dual-write logic
- ✅ **Time Savings**: 1-2 hours vs. pure `database/sql`
- ✅ **pgvector Compatible**: Raw SQL for semantic search still works

### Negative

- ⚠️ **Mixed APIs**: Two database libraries in codebase
  - **Mitigation**: Clear architectural separation (writes vs. reads)
  - **Documentation**: DD-STORAGE-002 explains why both exist
  - **Team Training**: Document sqlx usage patterns (minimal learning curve)

- ⚠️ **Struct Tag Management**: Need to keep `db:` tags in sync with schema
  - **Mitigation**: Schema changes require updating struct tags (same as manual Scan)
  - **Validation**: Integration tests catch tag mismatches early
  - **Benefit**: Centralized in struct definition, not scattered across 10+ Scan calls

- ⚠️ **Test Mock Updates**: Need to update mocks for sqlx API
  - **Mitigation**: `sqlx.DB` embeds `database/sql.DB`, mocks similar
  - **Effort**: ~1 hour to update test mocks
  - **One-Time Cost**: No ongoing maintenance burden

### Neutral

- 🔄 **Two Import Paths**: `database/sql` and `github.com/jmoiron/sqlx`
- 🔄 **Dependency Addition**: `sqlx` added to `go.mod` (~16k GitHub stars, stable)
- 🔄 **Documentation Overhead**: Need to explain hybrid approach (this document)

---

## Validation Results

### Confidence Assessment Progression

- **Initial Assessment (Day 6 planning)**: 75% confidence
- **After Boilerplate Analysis**: 85% confidence
- **After User Approval (Option B)**: 85% confidence (approved 2025-10-12)

### Key Validation Points

- ✅ **Boilerplate Reduction**: 20 lines → 1 line confirmed in implementation plan
- ✅ **Day 5 Preservation**: Zero changes to dual-write coordinator
- ✅ **Time Savings**: 1-2 hours vs. Alternative 1
- ✅ **pgvector Compatible**: Raw SQL for `ORDER BY embedding <=> $1` confirmed
- ✅ **Risk Assessment**: LOW (no changes to tested Day 5 code)

---

## Related Decisions

**Builds On**:
- **DD-STORAGE-001**: Continue with `database/sql` for dual-write (Day 5)

**Supersedes**: None (extends DD-STORAGE-001, doesn't contradict)

**Supports Business Requirements**:
- **BR-STORAGE-005**: Query audit trails with filtering
- **BR-STORAGE-006**: Pagination for large result sets
- **BR-STORAGE-012**: Semantic search via vector embeddings
- **BR-STORAGE-020**: Performance targets (<250ms p95 latency)

**Future Decisions**:
- Day 7 (Integration Tests): May use testcontainers for real database testing
- Day 10 (Observability): Query performance monitoring with sqlx hooks

---

## Review & Evolution

### When to Revisit

- **Day 7 (Integration Tests)**: If integration tests reveal issues with hybrid approach
- **Day 9 (BR Coverage Matrix)**: If test complexity increases with mixed APIs
- **Performance Testing**: If query latency exceeds targets (unlikely with sqlx)
- **Team Feedback**: If developers find mixed APIs confusing

### Success Metrics

- **Day 6 Completion**: ≤7 hours (Target: 6-7h)
- **Test Pass Rate**: 100% for query tests (Target: 100%)
- **Code Reduction**: ≥70% less scanning boilerplate (Target: 80%)
- **Build Time**: <10s for query package (Target: <30s)
- **Query Latency**: <100ms p95 for list operations (Target: <250ms)

### Migration Path (If Needed)

If hybrid approach proves problematic:

**Option**: Consolidate to single API
- **Choice 1**: Full `sqlx` migration (refactor Day 5 dual-write)
- **Choice 2**: Full `database/sql` (accept query boilerplate)
- **Estimated Effort**: 4-6 hours for full migration

**Confidence on Migration**: 30% (low likelihood of needing this)

---

## Related Documentation

- [DD-STORAGE-001](./DD-STORAGE-001-DATABASE-SQL-VS-ORM.md) - Day 5 database/sql decision
- [Implementation Plan V4.1](../IMPLEMENTATION_PLAN_V4.1.md) - Overall service plan
- [Day 5 Complete](../phase0/05-day5-complete.md) - Dual-write coordinator completion
- [Day 6 WIP](../phase0/06-day6-wip.md) - Query API implementation (to be created)

---

## Code Examples

### Before (database/sql - Day 5)

```go
// 20 lines of manual scanning per query
for rows.Next() {
    audit := &models.RemediationAudit{}
    err := rows.Scan(
        &audit.ID, &audit.Name, &audit.Namespace, &audit.Phase,
        &audit.ActionType, &audit.Status, &audit.StartTime, &audit.EndTime,
        &audit.Duration, &audit.RemediationRequestID, &audit.AlertFingerprint,
        &audit.Severity, &audit.Environment, &audit.ClusterName,
        &audit.TargetResource, &audit.ErrorMessage, &audit.Metadata,
        &audit.Embedding, &audit.CreatedAt, &audit.UpdatedAt,
    )
    if err != nil {
        return nil, err
    }
    audits = append(audits, audit)
}
```

### After (sqlx - Day 6)

```go
// 1 line - sqlx handles all scanning
var audits []*models.RemediationAudit
err := s.db.SelectContext(ctx, &audits, query, args...)
```

**Result**: 95% less code for query operations

---

**Decision Owner**: Jordi Gil
**Approved By**: User (2025-10-12)
**Implementation Status**: ✅ **READY TO IMPLEMENT** (Day 6: Query API)
**Next Steps**: Add `sqlx` dependency, add struct tags, implement query service


