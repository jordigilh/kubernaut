# TRIAGE: Audit Coverage Across All Services

**From**: HAPI Team (Audit Infrastructure Analysis)
**To**: ALL Service Teams
**Date**: December 8, 2025
**Priority**: 🔴 P0 (CRITICAL) - Foundational Compliance Issue
**Status**: 🔴 **CRITICAL FINDINGS**

---

## 🚨 Executive Summary

The AIAnalysis team discovered a **fundamental API contract mismatch** between the shared audit library (`pkg/audit/`) and the Data Storage Service. This triage reveals:

1. **Only 1 service** (Data Storage) has fully working audit E2E tests
2. **5 services** have audit tests that **cannot work** due to the batch API mismatch
3. **The entire audit infrastructure is untestable** outside of unit tests with mocks

---

## 🔍 Root Cause Analysis

### API Contract Mismatch

| Component | Expected Format | Actual Format | Status |
|-----------|-----------------|---------------|--------|
| **pkg/audit/http_client.go** | Array `[{event1}, {event2}]` | Sends array | ✅ Per DD-AUDIT-002 |
| **Data Storage Handler** | Single `{event}` | Expects single | ❌ **NON-COMPLIANT** |

**Authoritative Documents**:
- **DD-AUDIT-002**: `StoreBatch(ctx, events []*AuditEvent)` - **BATCH API**
- **ADR-038**: "Batches events (1000 events)" - **BATCH PROCESSING**
- **ADR-034**: Unified audit table design - **SINGLE EVENT SCHEMA** (but API should accept batch)

### Error Message
```
json: cannot unmarshal array into Go value of type map[string]interface {}
```

---

## 📊 Service-by-Service Audit Coverage

### Services MUST Audit (per DD-AUDIT-003)

| # | Service | Unit Tests | Integration Tests | E2E Tests | Audit Status |
|---|---------|------------|-------------------|-----------|--------------|
| 1 | **Gateway** | ✅ Mock | ⚠️ Can't verify | ❌ None | 🟡 PARTIAL |
| 2 | **AIAnalysis** | ✅ Mock | ❌ Failing | ❌ None | 🔴 BLOCKED |
| 3 | **WorkflowExecution** | ✅ Mock | ⚠️ Not verified | ❌ None | 🟡 PARTIAL |
| 4 | **Notification** | ✅ 5 tests | ✅ 2 integration | ✅ 2 E2E | 🟢 WORKING* |
| 5 | **Data Storage** | ✅ 756 specs | ✅ 163 tests | ✅ 13 tests | 🟢 WORKING |
| 6 | **Effectiveness Monitor** | ⚠️ Unknown | ❌ None | ❌ None | 🔴 UNKNOWN |

*Notification uses internal audit client (bypasses HTTP API issue)

### Services SHOULD Audit (per DD-AUDIT-003)

| # | Service | Unit Tests | Integration Tests | E2E Tests | Audit Status |
|---|---------|------------|-------------------|-----------|--------------|
| 7 | **SignalProcessing** | ⚠️ Unknown | ❌ None | ❌ None | 🔴 UNKNOWN |
| 8 | **RemediationOrchestrator** | ⚠️ Unknown | ❌ None | ❌ None | 🔴 UNKNOWN |

### Services NO Audit (per DD-AUDIT-003)

| # | Service | Audit Status |
|---|---------|--------------|
| 9 | **Context API** | ✅ N/A (read-only) |
| 10 | **HolmesGPT-API** | ✅ N/A (per DD-AUDIT-003)* |
| 11 | **Dynamic Toolset** | ✅ N/A (configuration) |

*Note: HAPI has internal audit for debugging but NOT per DD-AUDIT-003 requirements

---

## 📁 Audit Test File Inventory

### Integration Tests (test/integration/)

| File | Service | Status | Issue |
|------|---------|--------|-------|
| `aianalysis/audit_integration_test.go` | AIAnalysis | ❌ **FAILING** | Batch API mismatch |
| `notification/audit_integration_test.go` | Notification | ✅ Working | Uses internal client |
| `datastorage/audit_events_write_api_test.go` | Data Storage | ✅ Working | Tests single event API |
| `datastorage/audit_events_query_api_test.go` | Data Storage | ✅ Working | Query API tests |
| `datastorage/audit_events_schema_test.go` | Data Storage | ✅ Working | Schema validation |
| `datastorage/audit_self_auditing_test.go` | Data Storage | ✅ Working | Self-audit tests |
| `datastorage/workflow_search_audit_test.go` | Data Storage | ✅ Working | Workflow audit |

### E2E Tests (test/e2e/)

| File | Service | Status | Issue |
|------|---------|--------|-------|
| `notification/01_notification_lifecycle_audit_test.go` | Notification | ✅ Working | Uses internal client |
| `notification/02_audit_correlation_test.go` | Notification | ✅ Working | Uses internal client |
| `datastorage/01_happy_path_test.go` | Data Storage | ✅ Working | Tests full audit trail |
| `datastorage/06_workflow_search_audit_test.go` | Data Storage | ✅ Working | Workflow audit E2E |

### HAPI Tests (holmesgpt-api/tests/)

| File | Type | Status | Issue |
|------|------|--------|-------|
| `e2e/test_audit_pipeline_e2e.py` | E2E | ⚠️ **PARTIAL** | Uses Go infrastructure with fixed single-event writes |
| `unit/test_audit_event_structure.py` | Unit | ✅ Working | ADR-034 structure tests |
| `unit/test_llm_audit_integration.py` | Unit | ✅ Working | Mock LLM audit tests |

---

## 🔴 Critical Findings

### Finding 1: Shared Audit Library Cannot Be Tested

**Impact**: ALL external services using `pkg/audit/HTTPDataStorageClient` cannot verify audit persistence

**Affected Services**:
- Gateway Service
- AIAnalysis Controller
- WorkflowExecution Controller
- Effectiveness Monitor

**Root Cause**: `HTTPDataStorageClient.StoreBatch()` sends arrays, Data Storage expects single objects

### Finding 2: HAPI Implemented Workaround (Not Documented)

The HAPI team modified `holmesgpt-api/src/audit/buffered_store.py` to send **individual events** instead of batches:

```python
# HAPI workaround (not aligned with DD-AUDIT-002)
async def _write_single_event_with_retry(self, event: dict) -> bool:
    response = await self._session.post(
        f"{self._url}/api/v1/audit/events",  # Single event endpoint
        json=event  # Single event, not array
    )
```

This works but:
- ❌ Does not align with DD-AUDIT-002 (batch API)
- ❌ Less efficient (N HTTP calls instead of 1)
- ❌ Not documented as a deviation

### Finding 3: Missing E2E Audit Tests for Most Services

| Service | E2E Audit Tests | Compliance |
|---------|-----------------|------------|
| Gateway | ❌ 0 tests | 🔴 Non-compliant with DD-AUDIT-003 |
| AIAnalysis | ❌ 0 tests | 🔴 Non-compliant with DD-AUDIT-003 |
| WorkflowExecution | ❌ 0 tests | 🔴 Non-compliant with DD-AUDIT-003 |
| Notification | ✅ 2 tests | ✅ Compliant |
| Data Storage | ✅ 4+ tests | ✅ Compliant |
| Effectiveness Monitor | ❌ 0 tests | 🔴 Non-compliant with DD-AUDIT-003 |

---

## ✅ Recommended Actions

### Immediate (P0) - Data Storage Team

| # | Action | Effort | Impact |
|---|--------|--------|--------|
| 1 | Add batch endpoint `POST /api/v1/audit/events/batch` | 2-4 hours | Unblocks all services |
| 2 | Update OpenAPI spec with batch endpoint | 1 hour | Documentation |
| 3 | Run AIAnalysis audit integration tests | 30 min | Verify fix |

### Short-term (P1) - Each Service Team

| # | Action | Effort | Impact |
|---|--------|--------|--------|
| 4 | Gateway: Add E2E audit tests | 4 hours | DD-AUDIT-003 compliance |
| 5 | AIAnalysis: Verify audit integration after batch fix | 2 hours | Unblock E2E |
| 6 | WorkflowExecution: Add audit integration tests | 4 hours | DD-AUDIT-003 compliance |
| 7 | Effectiveness Monitor: Add audit tests | 4 hours | DD-AUDIT-003 compliance |

### Medium-term (P2) - HAPI Team

| # | Action | Effort | Impact |
|---|--------|--------|--------|
| 8 | Update HAPI to use batch endpoint when available | 2 hours | DD-AUDIT-002 alignment |
| 9 | Document HAPI audit architecture | 1 hour | Clarity |

---

## 📋 Acceptance Criteria for Resolution

- [ ] Data Storage accepts batch writes (array of events)
- [ ] All P0 services (Gateway, AIAnalysis, WE, Notification, DataStorage, EffMon) have passing audit E2E tests
- [ ] `pkg/audit/http_client.go` successfully writes batches to Data Storage
- [ ] HAPI audit uses batch endpoint (optional, can keep single-event as fallback)
- [ ] All authoritative documents (DD-AUDIT-002, ADR-038) match implementation

---

## 🔗 Related Documents

| Document | Purpose |
|----------|---------|
| [DD-AUDIT-002](../../architecture/decisions/DD-AUDIT-002-audit-shared-library-design.md) | Batch API specification |
| [DD-AUDIT-003](../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) | Which services MUST audit |
| [ADR-034](../../architecture/decisions/ADR-034-unified-audit-table-design.md) | Unified audit schema |
| [ADR-038](../../architecture/decisions/ADR-038-async-buffered-audit-ingestion.md) | Async buffered ingestion |
| [NOTICE_DATASTORAGE_AUDIT_BATCH_ENDPOINT_MISSING](./NOTICE_DATASTORAGE_AUDIT_BATCH_ENDPOINT_MISSING.md) | Original issue report |

---

## 📞 Response Section

### Data Storage Team Response

```
⏳ AWAITING RESPONSE

Please confirm:
1. Root cause acknowledged
2. Solution approach (batch endpoint vs. dual-mode handler)
3. Estimated timeline
```

### Gateway Team Response

```
⏳ AWAITING RESPONSE

Please confirm:
1. Current audit test coverage
2. Plan for E2E audit tests
```

### AIAnalysis Team Response

```
⏳ AWAITING RESPONSE

Please confirm:
1. Integration tests blocked by batch API issue
2. Ready to verify once Data Storage fixed
```

### WorkflowExecution Team Response

```
⏳ AWAITING RESPONSE

Please confirm:
1. Current audit implementation status
2. Plan for audit E2E tests
```

### Effectiveness Monitor Team Response

```
⏳ AWAITING RESPONSE

Please confirm:
1. Current audit implementation status
2. Plan for audit tests
```

---

## 🚨 TESTING_GUIDELINES.md VIOLATIONS TRIAGE

### Summary of Violations

Per `docs/development/business-requirements/TESTING_GUIDELINES.md`:

> **E2E tests must use all real services EXCEPT the LLM.**
> **If Data Storage is unavailable, E2E tests should FAIL, not skip.**

### Violations Found

| File | Type | Violation | Severity |
|------|------|-----------|----------|
| `test/e2e/notification/01_notification_lifecycle_audit_test.go` | E2E | Uses `httptest.Server` mock for Data Storage | 🔴 **CRITICAL** |
| `test/e2e/notification/02_audit_correlation_test.go` | E2E | Uses `httptest.Server` mock for Data Storage | 🔴 **CRITICAL** |
| `test/integration/notification/audit_integration_test.go` | Integration | Uses `httptest.Server` mock | 🟡 **ACCEPTABLE** (see note) |

**Note on Integration Tests**: Per TESTING_GUIDELINES.md, integration tests CAN use mocks (`Mock ✅`), but they should use `podman-compose.test.yml` for real services when testing audit persistence.

### Compliant E2E Tests

| File | Type | Status | Notes |
|------|------|--------|-------|
| `test/e2e/datastorage/01_happy_path_test.go` | E2E | ✅ **COMPLIANT** | Uses real DB, real HTTP API |
| `test/e2e/datastorage/02_dlq_fallback_test.go` | E2E | ✅ **COMPLIANT** | Uses real infrastructure |
| `test/e2e/datastorage/03_query_api_timeline_test.go` | E2E | ✅ **COMPLIANT** | Uses real infrastructure |
| `test/e2e/datastorage/06_workflow_search_audit_test.go` | E2E | ✅ **COMPLIANT** | Uses real infrastructure |

### Missing E2E Audit Tests (per DD-AUDIT-003)

| Service | DD-AUDIT-003 Status | E2E Audit Tests | Action Required |
|---------|---------------------|-----------------|-----------------|
| **Gateway** | MUST audit (P0) | ❌ **NONE** | Create E2E audit tests |
| **AIAnalysis** | MUST audit (P0) | ❌ **NONE** | Create E2E audit tests |
| **WorkflowExecution** | MUST audit (P0) | ❌ **NONE** (K8s events only) | Create E2E audit tests with Data Storage |
| **Effectiveness Monitor** | MUST audit (P0) | ❌ **NONE** | Create E2E audit tests |
| **SignalProcessing** | SHOULD audit (P1) | ❌ **NONE** | Create E2E audit tests |
| **RemediationOrchestrator** | SHOULD audit (P1) | ❌ **NONE** | Create E2E audit tests |

### Code Evidence of Violations

**test/e2e/notification/01_notification_lifecycle_audit_test.go** (Lines 71-77):
```go
// ❌ VIOLATION: E2E test using mock Data Storage
auditStore        audit.AuditStore
mockDataStorage   *httptest.Server  // ← MOCK IN E2E (FORBIDDEN)
receivedEvents    []*audit.AuditEvent
eventsMutex       sync.Mutex
```

**test/e2e/notification/02_audit_correlation_test.go** (Lines 72-74):
```go
// ❌ VIOLATION: E2E test using mock Data Storage
auditStore      audit.AuditStore
mockDataStorage *httptest.Server  // ← MOCK IN E2E (FORBIDDEN)
```

### Correct Pattern (from test/e2e/datastorage/)

**test/e2e/datastorage/01_happy_path_test.go** (Lines 66-74):
```go
// ✅ COMPLIANT: E2E test using real infrastructure
var (
    testCancel    context.CancelFunc
    testLogger    logr.Logger
    httpClient    *http.Client
    testNamespace string
    serviceURL    string  // ← REAL SERVICE URL
    db            *sql.DB // ← REAL DATABASE CONNECTION
    correlationID string
)
```

---

## 📋 Remediation Plan

### Phase 1: Fix Notification E2E Tests (P0)

**Objective**: Convert Notification E2E tests from mock to real Data Storage

**Files to Fix**:
1. `test/e2e/notification/01_notification_lifecycle_audit_test.go`
2. `test/e2e/notification/02_audit_correlation_test.go`

**Pattern to Follow**: `test/e2e/datastorage/01_happy_path_test.go`

**Changes Required**:
- Remove `httptest.Server` mock
- Use `test/infrastructure/notification.go` to deploy real Data Storage
- Connect to real Data Storage service URL
- Query real `audit_events` table for verification

### Phase 2: Add Missing E2E Audit Tests (P1)

| Service | New Test File | Dependencies |
|---------|---------------|--------------|
| Gateway | `test/e2e/gateway/XX_audit_trail_test.go` | Data Storage in Kind |
| AIAnalysis | `test/e2e/aianalysis/XX_audit_trail_test.go` | Data Storage in Kind |
| WorkflowExecution | `test/e2e/workflowexecution/XX_audit_trail_test.go` | Data Storage in Kind |
| Effectiveness Monitor | `test/e2e/effectivenessmonitor/XX_audit_trail_test.go` | Data Storage in Kind |

### Phase 3: Fix Batch API Mismatch (P0)

**Prerequisite for all audit E2E tests to work**

**Data Storage Team Action**:
- Add `POST /api/v1/audit/events/batch` endpoint
- Accept array of audit events: `[{event1}, {event2}, ...]`

---

**Document Version**: 1.1
**Created**: December 8, 2025
**Last Updated**: December 8, 2025
**Maintained By**: HAPI Team (Audit Infrastructure Analysis)

