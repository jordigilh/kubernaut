# Effectiveness Monitor - API Gateway Migration

**Related Decision**: [DD-ARCH-001: Alternative 2 (API Gateway Pattern)](../../../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md)
**Date**: November 2, 2025
**Status**: ✅ **APPROVED FOR IMPLEMENTATION**
**Service**: Effectiveness Monitor
**Timeline**: **2-3 Days** (Phase 3 of overall migration)
**Depends On**: [Data Storage Service Phase 1](../../data-storage/implementation/API-GATEWAY-MIGRATION.md) ✅ Must complete first

---

## 🎯 **WHAT THIS SERVICE NEEDS TO DO**

**Current State**: Effectiveness Monitor reads audit trail data directly from PostgreSQL

**New State**: Effectiveness Monitor queries Data Storage Service REST API for audit data (continues to write assessments directly)

**Changes Needed**:
1. ✅ Replace direct SQL queries for **audit trail reads** with HTTP client calls
2. ✅ **Keep direct writes** for effectiveness assessments (unchanged)
3. ✅ Update service specification
4. ✅ Update integration test infrastructure (start Data Storage Service in tests)

---

## 📋 **SPECIFICATION CHANGES**

### **1. Service Overview Update**

**File**: `overview.md`

**Current**:
> Effectiveness Monitor reads audit trail data from PostgreSQL.

**New**:
> Effectiveness Monitor queries audit trail data via **Data Storage Service REST API**.
>
> **Data Access Pattern**:
> - **Reads**: Audit trail data → **Data Storage Service REST API** (NEW)
> - **Writes**: Effectiveness assessments → **Direct PostgreSQL** (unchanged)
>
> **Design Decision**: [DD-ARCH-001 Alternative 2](../../../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md)

---

### **2. Integration Points Update**

**File**: `integration-points.md`

**Current**:
> **Downstream**:
> - PostgreSQL (Read audit data, Write assessments)

**New**:
> **Downstream**:
> - **Data Storage Service REST API** (Read audit trail data)
>   - Endpoint: `GET /api/v1/incidents?...`
>   - Client: `pkg/datastorage/client/http_client.go`
> - PostgreSQL (Write effectiveness assessments - unchanged)
>
> **Design Decision**: [DD-ARCH-001 Alternative 2](../../../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md)

---

## 🚀 **IMPLEMENTATION PLAN**

### **Phase 0: Documentation Updates** (1-1.5 hours)

**Status**: ✅ **Defined above**

**Tasks**:
1. Update `overview.md` (data access pattern)
2. Update `integration-points.md` (Data Storage Service client)

**Deliverables**:
- ✅ Service specification reflects new architecture

---

### **Day 1: HTTP Client Integration** (3-4 hours)

**Objective**: Integrate Data Storage Service HTTP client for audit reads

**Tasks**:
1. Reuse `pkg/datastorage/client/` package (already created for Context API)
2. Update Effectiveness Monitor's query logic to use HTTP client
3. Keep direct database writes unchanged

**Changes in `pkg/effectivenessmonitor/analyzer.go`** (example):

**Before**:
```go
type EffectivenessAnalyzer struct {
    db *sqlx.DB  // Direct SQL
}

func (a *EffectivenessAnalyzer) AnalyzeRemediation(ctx context.Context, remediationID int64) (*Assessment, error) {
    // Read audit trail directly from PostgreSQL
    var auditData []AuditEvent
    query := "SELECT * FROM resource_action_traces WHERE remediation_id = ?"
    a.db.SelectContext(ctx, &auditData, query, remediationID)

    // Analyze effectiveness
    assessment := a.calculateEffectiveness(auditData)

    // Write assessment directly to PostgreSQL (UNCHANGED)
    a.db.ExecContext(ctx, "INSERT INTO effectiveness_assessments (...) VALUES (...)", ...)

    return assessment, nil
}
```

**After**:
```go
type EffectivenessAnalyzer struct {
    storageClient datastorage.Client  // HTTP client
    db            *sqlx.DB             // For writes only
}

func (a *EffectivenessAnalyzer) AnalyzeRemediation(ctx context.Context, remediationID int64) (*Assessment, error) {
    // Read audit trail via Data Storage Service REST API
    response, err := a.storageClient.ListIncidents(ctx, &datastorage.ListParams{
        RemediationID: &remediationID,
    })
    if err != nil {
        return nil, fmt.Errorf("data storage query failed: %w", err)
    }

    // Analyze effectiveness (UNCHANGED)
    assessment := a.calculateEffectiveness(response.Incidents)

    // Write assessment directly to PostgreSQL (UNCHANGED)
    a.db.ExecContext(ctx, "INSERT INTO effectiveness_assessments (...) VALUES (...)", ...)

    return assessment, nil
}
```

**Code Changes**:
- Remove direct SQL queries for audit reads (~30 lines)
- Add HTTP client calls (~20 lines)
- Keep direct database writes unchanged (~0 lines)

**Deliverables**:
- ✅ Effectiveness Monitor uses Data Storage Service REST API for reads
- ✅ Direct database writes unchanged
- ✅ Unit tests updated

---

### **Day 2: Update Integration Test Infrastructure** (2-3 hours)

**Objective**: Integration tests now start Data Storage Service

**Tasks**:
1. Reuse test helper from Context API migration
2. Update `BeforeSuite()` to start Data Storage Service
3. Verify all integration tests pass

**Changes in `test/integration/effectivenessmonitor/effectiveness_monitor_suite_test.go`**:

**Before**:
```go
var _ = BeforeSuite(func() {
    // Start PostgreSQL
    db = testutil.StartPostgreSQL()
})
```

**After**:
```go
var (
    db            *sqlx.DB
    storageServer *datastorage.Server  // NEW
    storageClient datastorage.Client   // NEW
)

var _ = BeforeSuite(func() {
    // Start PostgreSQL
    db = testutil.StartPostgreSQL()

    // Start Data Storage Service (NEW - reuse from Context API)
    storageServer = datastorage.NewServer(&datastorage.Config{
        DBConnection: db,
        Port:        8082,  // Different port from Context API tests
    })
    go storageServer.Start()
    testutil.WaitForHTTP("http://localhost:8082/health")

    // Create client for Effectiveness Monitor to use
    storageClient = datastorage.NewHTTPClient("http://localhost:8082")
})

var _ = AfterSuite(func() {
    storageServer.Shutdown()  // NEW
    db.Close()
})
```

**Deliverables**:
- ✅ Integration tests updated
- ✅ All tests passing

---

### **Day 3: Validation & Documentation** (1-2 hours)

**Objective**: Final validation and documentation update

**Tasks**:
1. Run full test suite (unit + integration)
2. Manual testing with real Data Storage Service
3. Update service documentation

**Deliverables**:
- ✅ All tests passing
- ✅ Service specification updated
- ✅ **Effectiveness Monitor successfully migrated**

---

## ✅ **SUCCESS CRITERIA**

- ✅ HTTP client for Data Storage Service integrated
- ✅ Effectiveness Monitor reads audit data via REST API
- ✅ Direct database writes unchanged (effectiveness assessments)
- ✅ Integration tests updated (start Data Storage Service)
- ✅ All unit + integration tests passing
- ✅ Service specification updated
- ✅ **Effectiveness Monitor successfully migrated to API Gateway pattern**

---

## 📊 **CODE IMPACT SUMMARY**

| Component | Change | Lines |
|-----------|--------|-------|
| Direct SQL reads (audit data) | **REMOVED** | -30 |
| HTTP client (audit data) | **ADDED** | +20 |
| Direct SQL writes (assessments) | **UNCHANGED** | 0 |
| Integration test infra | **UPDATED** | +40 |
| **Net Change** | | **+30 lines** |

**Confidence**: 95% - Similar to Context API migration, straightforward HTTP client replacement

---

## 🔗 **RELATED DOCUMENTATION**

- [DD-ARCH-001 Final Decision](../../../../architecture/decisions/DD-ARCH-001-FINAL-DECISION.md) - Architecture decision
- [Data Storage Service Migration](../../data-storage/implementation/API-GATEWAY-MIGRATION.md) - Phase 1 (dependency)
- [Context API Migration](../../context-api/implementation/API-GATEWAY-MIGRATION.md) - Phase 2 (parallel)

---

**Status**: ✅ **APPROVED - Ready for implementation after Data Storage Service Phase 1**
**Dependencies**: Data Storage Service REST API must be implemented first
**Parallel Work**: Can be done in parallel with Context API migration

