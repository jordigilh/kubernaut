# AIAnalysis Integration Test Audit Coverage Triage

**Date**: December 17, 2025
**Triage By**: AIAnalysis Team
**Document**: `test/integration/aianalysis/audit_integration_test.go`
**Status**: ✅ **AUDIT EVENT COVERAGE COMPLETE** | 🚨 **ARCHITECTURAL VIOLATION - MANDATORY FIX**

---

## 🎯 **Triage Summary**

**Audit Event Coverage**: ✅ **100% COMPLETE** (All 6 event types tested)
**Field Coverage**: ✅ **100% COMPLETE** (All payload fields validated)
**Test Architecture**: 🚨 **WRONG** (Direct DB queries violate service boundaries)
**V1.0 Readiness**: 🚨 **BLOCKING** (Must refactor to use Data Storage REST API)

---

## 🚨 **CRITICAL ARCHITECTURAL VIOLATION**

### **Current Implementation** ❌

**AIAnalysis integration tests directly query Data Storage's PostgreSQL database**:

```go
// ❌ WRONG: Direct DB queries couple to Data Storage internal schema
// From audit_integration_test.go (lines 104-119)
pgHost := os.Getenv("POSTGRES_HOST")
if pgHost == "" {
    pgHost = "localhost"
}
pgPort := os.Getenv("POSTGRES_PORT")
if pgPort == "" {
    pgPort = "15434" // AIAnalysis integration test PostgreSQL port
}

connStr := fmt.Sprintf("host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable", pgHost, pgPort)
db, dbErr = sql.Open("pgx", connStr)

// Later in tests:
query := `
    SELECT event_type, resource_type, resource_id, correlation_id, event_outcome
    FROM audit_events
    WHERE correlation_id = $1
    AND event_type = $2
    ORDER BY event_timestamp DESC
    LIMIT 1
`
err := db.QueryRow(query, testAnalysis.Spec.RemediationID, aiaudit.EventTypeAnalysisCompleted).
    Scan(&eventType, &resourceType, &resourceID, &correlationID, &eventOutcome)
```

---

## 🚨 **Why This Is Wrong**

### **Architectural Violation: Service Boundary Breach**

**Problem Statement**:
> "when the DB schema changes, these tests will fail"
> "These DB queries are not necessary unless it's the DS service doing them"
> — User feedback (December 17, 2025)

**Service Boundaries Violated**:
1. ❌ **AIAnalysis knows Data Storage internals** (PostgreSQL schema)
2. ❌ **AIAnalysis depends on Data Storage implementation** (DB table structure)
3. ❌ **Tests break when Data Storage refactors** (schema evolution)
4. ❌ **Bypasses Data Storage API contract** (REST API is the contract)

### **Consequences of Current Architecture**

| Scenario | Impact |
|----------|--------|
| Data Storage changes `audit_events` table name | ❌ AIAnalysis integration tests break |
| Data Storage changes column names/types | ❌ AIAnalysis integration tests break |
| Data Storage migrates to different database | ❌ AIAnalysis integration tests break |
| Data Storage optimizes schema for performance | ❌ AIAnalysis integration tests break |

**Result**: AIAnalysis integration tests are **tightly coupled** to Data Storage internal implementation.

---

## ✅ **Correct Architecture: Service Contract Testing**

### **Service Contracts (APIs)**

```
AIAnalysis Service
    ↓ (writes audit events via)
Data Storage REST API (POST /api/v1/audit/events)
    ↓ (stores in)
PostgreSQL Database ← ONLY Data Storage knows this schema
    ↑ (reads via)
Data Storage REST API (GET /api/v1/audit/events)
    ↑ (queries from)
AIAnalysis Integration/E2E Tests
```

### **What AIAnalysis Should Know**

| Layer | AIAnalysis Should Know | AIAnalysis Should NOT Know |
|-------|----------------------|---------------------------|
| **Write** | ✅ Data Storage REST API endpoint | ❌ PostgreSQL connection string |
| **Write** | ✅ Audit event JSON schema | ❌ Database table schema |
| **Read** | ✅ Query parameters (correlation_id, event_type) | ❌ SQL queries |
| **Read** | ✅ REST API response format | ❌ Database row structure |

### **Correct Integration Test Pattern**

```go
// ✅ CORRECT: Test AIAnalysis → Data Storage contract (REST API)

// WRITE: AIAnalysis sends audit event to Data Storage
auditClient.RecordAnalysisComplete(ctx, testAnalysis)
time.Sleep(500 * time.Millisecond)
Expect(auditStore.Close()).To(Succeed()) // Flush buffered events

// READ: Query Data Storage via REST API (public contract)
resp, err := http.Get(fmt.Sprintf(
    "%s/api/v1/audit/events?correlation_id=%s&event_type=%s",
    datastorageURL,
    testAnalysis.Spec.RemediationID,
    aiaudit.EventTypeAnalysisCompleted,
))
Expect(err).ToNot(HaveOccurred())
defer resp.Body.Close()
Expect(resp.StatusCode).To(Equal(http.StatusOK))

var auditResponse struct {
    Data       []map[string]interface{} `json:"data"`
    Pagination map[string]int           `json:"pagination"`
}
Expect(json.NewDecoder(resp.Body).Decode(&auditResponse)).To(Succeed())
Expect(auditResponse.Data).To(HaveLen(1))

// Validate event data via REST API response
event := auditResponse.Data[0]
Expect(event["event_type"]).To(Equal(aiaudit.EventTypeAnalysisCompleted))
Expect(event["correlation_id"]).To(Equal(testAnalysis.Spec.RemediationID))
Expect(event["resource_type"]).To(Equal("AIAnalysis"))

eventData := event["event_data"].(map[string]interface{})
Expect(eventData["phase"]).To(Equal("Completed"))
Expect(eventData["approval_required"]).To(BeFalse())
```

---

## 🔧 **MANDATORY Refactoring Plan**

### **Task 1: Remove Direct PostgreSQL Access** (30 min)

**File**: `test/integration/aianalysis/audit_integration_test.go`

**Changes**:
1. Remove PostgreSQL connection setup (lines 104-119)
2. Remove `db *sql.DB` variable
3. Remove `_ "github.com/jackc/pgx/v5/stdlib"` import
4. Remove all `db.QueryRow()` calls

**Rationale**: AIAnalysis should not know Data Storage's database implementation.

---

### **Task 2: Add REST API Query Helper** (30 min)

**File**: `test/integration/aianalysis/audit_integration_test.go`

**Add Helper Function**:
```go
// queryAuditEventsViaAPI queries Data Storage REST API for audit events
// This is the CORRECT way for AIAnalysis to verify audit data - via public API contract
func queryAuditEventsViaAPI(datastorageURL, correlationID, eventType string) ([]map[string]interface{}, error) {
    url := fmt.Sprintf(
        "%s/api/v1/audit/events?correlation_id=%s",
        datastorageURL,
        correlationID,
    )
    if eventType != "" {
        url += fmt.Sprintf("&event_type=%s", eventType)
    }

    resp, err := http.Get(url)
    if err != nil {
        return nil, fmt.Errorf("failed to query audit API: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("audit API returned %d: %s", resp.StatusCode, string(body))
    }

    var auditResponse struct {
        Data       []map[string]interface{} `json:"data"`
        Pagination map[string]int           `json:"pagination"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&auditResponse); err != nil {
        return nil, fmt.Errorf("failed to decode audit response: %w", err)
    }

    return auditResponse.Data, nil
}
```

---

### **Task 3: Refactor All Test Specs** (2-3 hours)

**Pattern for Each Test**:

```go
// BEFORE (WRONG):
By("Verifying audit event was persisted to PostgreSQL")
query := `SELECT event_type FROM audit_events WHERE correlation_id = $1`
err := db.QueryRow(query, remediationID).Scan(&eventType)
Expect(err).ToNot(HaveOccurred())

// AFTER (CORRECT):
By("Verifying audit event is retrievable via Data Storage REST API")
events, err := queryAuditEventsViaAPI(datastorageURL, remediationID, aiaudit.EventTypeAnalysisCompleted)
Expect(err).ToNot(HaveOccurred())
Expect(events).To(HaveLen(1))
Expect(events[0]["event_type"]).To(Equal(aiaudit.EventTypeAnalysisCompleted))
```

**Tests to Refactor** (10 specs):
1. `RecordAnalysisComplete` - basic persistence (lines 194-223)
2. `RecordAnalysisComplete` - 100% field coverage (lines 225-279)
3. `RecordPhaseTransition` - 100% field coverage (lines 285-317)
4. `RecordHolmesGPTCall` - success case (lines 323-356)
5. `RecordHolmesGPTCall` - failure case (lines 358-381)
6. `RecordApprovalDecision` - 100% field coverage (lines 387-424)
7. `RecordRegoEvaluation` - allow case (lines 431-474)
8. `RecordRegoEvaluation` - degraded case (lines 476-509)
9. `RecordError` - phase context (lines 515-560)
10. `RecordError` - phase differentiation (lines 562-595)

---

### **Task 4: Update Test Infrastructure** (30 min)

**File**: `test/integration/aianalysis/audit_integration_test.go`

**Changes**:
1. Update `BeforeEach()` to remove PostgreSQL connection
2. Update `AfterEach()` to remove `db.Close()`
3. Add HTTP client to `BeforeEach()` for REST API calls
4. Update test documentation header

**New BeforeEach Pattern**:
```go
BeforeEach(func() {
    // Determine Data Storage URL
    datastorageURL = os.Getenv("DATASTORAGE_URL")
    if datastorageURL == "" {
        datastorageURL = "http://localhost:18091"
    }

    // Verify Data Storage is available (MANDATORY per TESTING_GUIDELINES.md)
    By("Verifying Data Storage is available via REST API")
    Eventually(func() error {
        resp, err := http.Get(datastorageURL + "/health")
        if err != nil {
            return err
        }
        defer resp.Body.Close()
        if resp.StatusCode != http.StatusOK {
            return fmt.Errorf("Data Storage health check failed with status %d", resp.StatusCode)
        }
        return nil
    }, "10s", "1s").Should(Succeed())

    // Create audit store (writes via REST API)
    httpClient := &http.Client{Timeout: 5 * time.Second}
    dsClient := audit.NewHTTPDataStorageClient(datastorageURL, httpClient)
    auditStore, err := audit.NewBufferedStore(dsClient, config, "aianalysis-integration-test", logger)
    Expect(err).ToNot(HaveOccurred())

    auditClient = aiaudit.NewAuditClient(auditStore, logger)
    // ... rest of setup
})
```

---

### **Task 5: Update E2E Tests for Consistency** (1 hour)

**File**: `test/e2e/aianalysis/05_audit_trail_test.go`

**Verify E2E Tests Already Use REST API**:
- ✅ E2E tests already use REST API for reads (confirmed in earlier review)
- ✅ E2E tests use audit client for writes (buffered store → REST API)
- ✅ No changes needed to E2E tests (already correct)

**Consistency Check**:
```bash
# Both integration and E2E should use REST API
grep -n "db.QueryRow\|sql.Open" test/e2e/aianalysis/*.go
# Expected: No matches (E2E doesn't use direct DB)

grep -n "db.QueryRow\|sql.Open" test/integration/aianalysis/*.go
# Expected: No matches after refactoring (integration also uses REST API)
```

---

## 📊 **Compliance Assessment**

### **Current State vs. Required for V1.0**

| Aspect | Current | Required | Status |
|--------|---------|----------|--------|
| **Audit Event Coverage** | ✅ 100% (6/6 event types) | ✅ 100% | ✅ COMPLIANT |
| **Field Coverage** | ✅ 100% (all payload fields) | ✅ 100% | ✅ COMPLIANT |
| **Write Integration** | ✅ Uses REST API (audit client) | ✅ Uses REST API | ✅ COMPLIANT |
| **Read Validation** | ❌ Direct DB queries | ✅ Uses REST API | 🚨 **VIOLATION** |
| **Service Boundaries** | ❌ Knows DS PostgreSQL schema | ✅ Only knows DS REST API | 🚨 **VIOLATION** |
| **Schema Independence** | ❌ Breaks on DB schema changes | ✅ Resilient to DS refactoring | 🚨 **VIOLATION** |

### **Severity**: 🚨 **P0 - V1.0 BLOCKER**

**Rationale**:
- Integration tests violate service boundary principles
- Direct DB access creates tight coupling to Data Storage internals
- Schema evolution in Data Storage will break AIAnalysis tests
- Only Data Storage service should know its database schema

---

## ✅ **Success Criteria**

### **Post-Refactoring Validation**

```bash
# 1. Verify no direct PostgreSQL access in AIAnalysis tests
grep -r "sql.Open\|pgx\|db.QueryRow" test/integration/aianalysis/ test/e2e/aianalysis/
# Expected: NO MATCHES (only Data Storage tests should access DB)

# 2. Verify REST API usage in integration tests
grep -r "http.Get.*audit/events" test/integration/aianalysis/
# Expected: Multiple matches (all tests use REST API)

# 3. Run integration tests
cd test/integration/aianalysis
podman-compose -f ../../podman-compose.test.yml up -d datastorage postgres redis
ginkgo -v --focus="Audit"
# Expected: All 10 tests pass using REST API

# 4. Run E2E tests
make test-e2e-aianalysis
# Expected: All 5 audit E2E tests pass using REST API

# 5. Simulate Data Storage schema change (verify resilience)
# Change audit_events table structure in Data Storage
# Expected: AIAnalysis tests still pass (only test REST API contract)
```

### **Metrics** ✅

- ✅ **0 direct DB queries** in AIAnalysis tests (integration + E2E)
- ✅ **100% REST API usage** for audit validation
- ✅ **Service boundary compliance** (AIAnalysis doesn't know DS internals)
- ✅ **Schema independence** (resilient to DS database refactoring)

---

## 📚 **Related Documents**

- [audit_integration_test.go](../../test/integration/aianalysis/audit_integration_test.go) - **REQUIRES REFACTORING**
- [05_audit_trail_test.go](../../test/e2e/aianalysis/05_audit_trail_test.go) - Already correct (uses REST API)
- [audit_events_handler.go](../../pkg/datastorage/server/audit_events_handler.go) - Data Storage REST API (public contract)
- [DD-AUDIT-003](../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) - Audit event types
- [DD-AUDIT-004](../architecture/decisions/DD-AUDIT-004-audit-type-safety-specification.md) - Payload specifications
- [ADR-032](../architecture/decisions/ADR-032-data-access-layer-isolation.md) - Mandatory audit requirements

---

## 🎯 **Recommendation**

### **MANDATORY FIX FOR V1.0: Refactor Integration Tests to Use REST API**

**Priority**: P0 - V1.0 blocking
**Effort**: 3.5-4.5 hours
**Owner**: AIAnalysis Team

**Timeline**:
- Task 1 (Remove PostgreSQL): 30 min
- Task 2 (Add REST API helper): 30 min
- Task 3 (Refactor 10 test specs): 2-3 hours
- Task 4 (Update infrastructure): 30 min
- Task 5 (Verify E2E consistency): 1 hour

**Critical Path**:
1. Remove all `db.QueryRow()` calls
2. Add `queryAuditEventsViaAPI()` helper
3. Refactor each test to use REST API
4. Verify 100% test pass rate
5. Confirm no direct DB access remains

---

## ✅ **Triage Conclusion**

### **Findings**

1. ✅ **CONFIRMED**: Audit event coverage is 100% complete (6/6 event types)
2. ✅ **CONFIRMED**: Field coverage is 100% complete (all payload fields)
3. 🚨 **IDENTIFIED**: Architectural violation - direct PostgreSQL access
4. 🚨 **ASSESSED**: Service boundary breach - AIAnalysis knows DS internals
5. 🚨 **DETERMINED**: P0 blocker for V1.0 - must refactor to use REST API

### **Recommendation**

**MANDATORY FIX FOR V1.0**: Refactor integration tests to use Data Storage REST API

**Rationale** (User feedback December 17, 2025):
- "when the DB schema changes, these tests will fail"
- "These DB queries are not necessary unless it's the DS service doing them"
- Only Data Storage service should know its database schema
- AIAnalysis should only know Data Storage's public API contract (REST API)

### **Architecture Principle**

**Service Boundaries**:
```
AIAnalysis ←→ Data Storage REST API (public contract)
                      ↓
              PostgreSQL (Data Storage internal implementation)
```

**AIAnalysis should**:
- ✅ Write audit events via Data Storage REST API
- ✅ Read audit events via Data Storage REST API
- ❌ NEVER access Data Storage PostgreSQL directly

### **V1.0 Status**

- **Audit Event Coverage**: ✅ **COMPLETE**
- **Field Coverage**: ✅ **COMPLETE**
- **Test Architecture**: 🚨 **REQUIRES FIX** (direct DB access violation)
- **V1.0 Readiness**: 🚨 **BLOCKING** until refactored to REST API

### **Acknowledgment**

- [ ] **AIAnalysis Team** - Triage acknowledged, refactoring plan accepted
- [ ] **Architecture Team** - Service boundary violation documented

---

**Document Status**: ✅ **TRIAGE COMPLETE**
**Next Action**: Refactor integration tests to use REST API (Tasks 1-5)
**Owner**: AIAnalysis Team
**Target**: V1.0 release (before PR merge)
**Priority**: P0 (BLOCKING)
