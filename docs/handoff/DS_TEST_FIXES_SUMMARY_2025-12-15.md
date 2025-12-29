# Data Storage Service - Test Fixes Summary

**Date**: 2025-12-15
**Status**: ✅ 2/3 P0 E2E Fixes Complete + Diagnosis for Remaining Issues

---

## ✅ **COMPLETED FIXES** (2/3 P0 E2E)

### **Fix 1: RFC 7807 Error Response Format** ✅

**Issue**: Service returned HTTP 201 instead of HTTP 400 for missing required fields.

**Root Cause**: JSON unmarshaling in Go doesn't validate required fields - it just sets zero values (empty strings). The validation function assumed "OpenAPI already validated required fields", but this was incorrect.

**Fix**: Added explicit validation for required fields in `pkg/datastorage/server/helpers/openapi_conversion.go`:

```go
// Validate required fields
if req.EventType == "" {
    return fmt.Errorf("event_type is required")
}
if req.Version == "" {
    return fmt.Errorf("version is required")
}
if req.CorrelationId == "" {
    return fmt.Errorf("correlation_id is required")
}
if req.EventAction == "" {
    return fmt.Errorf("event_action is required")
}
if req.EventCategory == "" {
    return fmt.Errorf("event_category is required")
}
if string(req.EventOutcome) == "" {
    return fmt.Errorf("event_outcome is required")
}

// Validate event_outcome enum
validOutcomes := map[string]bool{
    "success": true,
    "failure": true,
    "pending": true,
}
if !validOutcomes[string(req.EventOutcome)] {
    return fmt.Errorf("event_outcome must be one of: success, failure, pending (got: %s)", req.EventOutcome)
}
```

**File**: `pkg/datastorage/server/helpers/openapi_conversion.go`
**Test**: `test/e2e/datastorage/10_malformed_event_rejection_test.go:108`
**Status**: ✅ FIXED

---

### **Fix 2: Multi-Filter Query API** ✅

**Issue**: Query by `event_category=gateway` returned 0 results instead of 4.

**Root Cause**: E2E test was using old ADR-034 field names (`service`, `outcome`, `operation`) instead of new names (`event_category`, `event_outcome`, `event_action`).

**Fix**: Updated test to use correct ADR-034 field names in `test/e2e/datastorage/03_query_api_timeline_test.go`:

**Before**:
```go
event := map[string]interface{}{
    "service": "gateway",     // OLD
    "outcome": "success",      // OLD
    "operation": "gateway_op", // OLD
    // ...
}
```

**After**:
```go
event := map[string]interface{}{
    "event_category": "gateway", // ADR-034
    "event_action": "gateway_op", // ADR-034
    "event_outcome": "success",   // ADR-034
    // ...
}
```

**Files Changed**:
- `test/e2e/datastorage/03_query_api_timeline_test.go` (3 locations: Gateway, AIAnalysis, Workflow events)

**Test**: `test/e2e/datastorage/03_query_api_timeline_test.go:254`
**Status**: ✅ FIXED

---

## ⏸️ **REMAINING ISSUES** (1/3 P0 E2E + 7 Integration + Performance)

### **Fix 3: Workflow Search Audit Metadata** ⏸️ NOT FIXED

**Issue**: No audit event created after workflow search operation.

**Root Cause**: The `HandleWorkflowSearch` method doesn't generate audit events.

**Expected Behavior**: Per BR-AUDIT-023 through BR-AUDIT-028, workflow search should generate audit events with:
- correlation_id (for request tracing)
- remediation_id (search context)
- search_filters (labels used)
- result_count (workflows found)
- duration_ms (search latency)

**Diagnosis**: The workflow search handler is missing audit event generation code similar to `HandleCreateWorkflow` and `HandleUpdateWorkflow`.

**Required Fix**:
1. Locate `HandleWorkflowSearch` in `pkg/datastorage/server/handler.go` or `workflow_handlers.go`
2. Add audit event generation similar to:

```go
// BR-AUDIT-023: Audit workflow search (business logic operation)
if h.auditStore != nil {
    go func() {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()

        auditEvent, err := dsaudit.NewWorkflowSearchAuditEvent(
            remediationID,    // from query params or headers
            searchFilters,    // labels used in search
            len(workflows),   // result count
            searchDuration,   // actual search latency
        )
        if err != nil {
            h.logger.Error(err, "Failed to create workflow search audit event")
            return
        }

        if err := h.auditStore.StoreAudit(ctx, auditEvent); err != nil {
            h.logger.Error(err, "Failed to audit workflow search")
        }
    }()
}
```

3. Create `NewWorkflowSearchAuditEvent` in `pkg/datastorage/audit/workflow_catalog_event.go` if it doesn't exist

**Test**: `test/e2e/datastorage/06_workflow_search_audit_test.go:290`
**Status**: ⏸️ **NOT FIXED** (requires implementation)

---

### **Fix 4: Workflow Repository Test Isolation** ⏸️ NOT FIXED

**Issue**: 7 integration tests failing with test isolation problems.

**Root Cause**: Tests see 50 workflows from other tests instead of expected 2-3.

**Failing Tests** (all in `test/integration/datastorage/workflow_repository_integration_test.go`):
1. List with no filters (line 346)
2. List with status filter (line 371)
3. List with pagination (line 388)
4-7. Additional List tests

**Diagnosis**: Database not cleaned between tests OR tests running in parallel without isolation.

**Required Fix**:
1. **Option A**: Add proper cleanup in `AfterEach`:
   ```go
   AfterEach(func() {
       // Delete all workflows created by this test
       _, err := db.Exec("DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE 'wf-repo-test-%'")
       Expect(err).ToNot(HaveOccurred())
   })
   ```

2. **Option B**: Use unique schema per test run:
   ```go
   BeforeEach(func() {
       schemaName := fmt.Sprintf("test_%d", time.Now().UnixNano())
       db.Exec(fmt.Sprintf("CREATE SCHEMA %s", schemaName))
       db.Exec(fmt.Sprintf("SET search_path TO %s", schemaName))
   })
   ```

3. **Option C**: Use test fixtures with known IDs and query by those IDs only

**Files**: `test/integration/datastorage/workflow_repository_integration_test.go`
**Status**: ⏸️ **NOT FIXED** (test infrastructure issue, not production bug)

---

### **Fix 5: Performance Tests** ⏸️ BUILD VERIFICATION ONLY

**Issue**: Performance tests skipped because service not running on `localhost:8080`.

**Root Cause**: Tests expect service on localhost, but it's deployed in Kind cluster on different port.

**Required Fix**:
1. **Verify tests build**:
   ```bash
   go build ./test/performance/datastorage/...
   ```

2. **Update tests to use Kind cluster URL** (future work):
   - Add environment variable `DATA_STORAGE_URL`
   - Default to `localhost:8080` for local development
   - Use Kind cluster NodePort for CI/E2E runs

**Files**: `test/performance/datastorage/*.go`
**Status**: ⏸️ **BUILD VERIFICATION ONLY** (user requested to skip for now)

---

## 📊 **Summary of Fixes**

| Issue | Status | Impact | Effort | Priority |
|-------|--------|--------|--------|----------|
| RFC 7807 Error Format | ✅ FIXED | HIGH (API compliance) | 30 min | P0 |
| Multi-Filter Query API | ✅ FIXED | HIGH (query correctness) | 15 min | P0 |
| Workflow Search Audit | ⏸️ NOT FIXED | HIGH (audit compliance) | 1-2 hours | P0 |
| Test Isolation (7 tests) | ⏸️ NOT FIXED | LOW (test infra) | 30 min | P1 |
| Performance Tests | ⏸️ BUILD ONLY | LOW (can run separately) | 15 min | P1 |

---

## 🎯 **Next Steps**

### **Immediate (P0)**

1. **Fix Workflow Search Audit** (1-2 hours):
   - Add audit event generation to `HandleWorkflowSearch`
   - Create `NewWorkflowSearchAuditEvent` builder
   - Verify test passes

### **Short-Term (P1)**

2. **Fix Test Isolation** (30 min):
   - Add cleanup in `AfterEach` for workflow integration tests
   - Re-run integration tests

3. **Verify Performance Tests Build** (15 min):
   - Run `go build ./test/performance/datastorage/...`
   - Document any build errors

### **Medium-Term (P2)**

4. **Make Performance Tests CI-Ready**:
   - Add `DATA_STORAGE_URL` environment variable
   - Update tests to use Kind cluster NodePort
   - Add to CI pipeline

---

## ✅ **Verification Commands**

### **Test Fixed Changes**

```bash
# Verify code builds
go build ./pkg/datastorage/server/...

# Run specific E2E tests that were fixed
cd test/e2e/datastorage
ginkgo --focus="RFC 7807|Multi-Filter" -v

# Run all Data Storage tests
make test-datastorage-all
```

### **Check Remaining Issues**

```bash
# Workflow search audit test (expected to still fail)
cd test/e2e/datastorage
ginkgo --focus="Workflow Search Audit" -v

# Integration tests (expected to still have 7 failures)
make test-integration-datastorage

# Performance tests build check
go build ./test/performance/datastorage/...
```

---

## 🎓 **Lessons Learned**

### **1. Go JSON Unmarshaling Doesn't Validate Required Fields**

**Problem**: Assumed OpenAPI validation was automatic.
**Reality**: Go sets zero values (empty strings) for missing fields.
**Solution**: Always add explicit validation for required fields.

### **2. ADR-034 Field Name Migration**

**Problem**: Tests used old field names (`service`, `outcome`, `operation`).
**Reality**: ADR-034 renamed to (`event_category`, `event_outcome`, `event_action`).
**Solution**: Update all tests to use current ADR-034 schema.

### **3. Test Isolation is Critical**

**Problem**: Integration tests see data from other tests.
**Reality**: Shared database without cleanup causes flaky tests.
**Solution**: Always clean up test data in `AfterEach` or use unique schemas.

---

## 📋 **Related Documents**

- [DS_V1.0_ACTUAL_TEST_RESULTS_2025-12-15.md](./DS_V1.0_ACTUAL_TEST_RESULTS_2025-12-15.md) - Complete test execution results
- [DS_V1.0_TRIAGE_2025-12-15.md](./DS_V1.0_TRIAGE_2025-12-15.md) - V1.0 triage against authoritative docs
- [DS_P0_CORRECTIONS_COMPLETE.md](./DS_P0_CORRECTIONS_COMPLETE.md) - Documentation corrections

---

**Document Version**: 1.0
**Created**: 2025-12-15
**Status**: ✅ 2/3 P0 Fixes Complete





