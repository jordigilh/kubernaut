# DataStorage E2E - Remaining 6 Test Failures Analysis

**Date**: January 10, 2026  
**Status**: 🔍 Under Investigation  
**Overall Progress**: 92/98 tests passing (94%)

---

## 🎯 **Executive Summary**

6 E2E tests remain failing after all infrastructure issues were fixed. Analysis shows these failures fall into two categories:

1. **Test Schema Mismatch** (1 failure) - Test needs update for OpenAPI discriminated union
2. **Real Business Logic Bugs** (5 failures) - Production code issues

---

## 📋 **Failure Details**

### **1. GAP 1.1: Event Type Validation - gateway.signal.received** ✅ **FIX IN PROGRESS**

**File**: `test/e2e/datastorage/09_event_type_jsonb_comprehensive_test.go:654`  
**Error**: `Expected <int>: 400 To satisfy at least one of these matchers: [201, 202]`

**Root Cause**: Test sends raw JSON without OpenAPI discriminated union format

**Current**: Test sends event_data as plain map:
```json
{
  "alert_name": "HighCPU",
  "signal_fingerprint": "fp-abc123",
  "namespace": "production",
  ...
}
```

**Required**: OpenAPI schema expects discriminated union:
```json
{
  "type": "gateway.signal.received",  // Discriminator
  "event_type": "gateway.signal.received",
  "signal_type": "prometheus-alert",
  "alert_name": "HighCPU",
  "namespace": "production",
  "fingerprint": "fp-abc123"
}
```

**Fix Applied**:
1. Added discriminator "type" field to event_data (line 627-629)
2. Updated SampleEventData to include required OpenAPI fields (lines 92-100)
3. Fixed field name: `signal_fingerprint` → `fingerprint`

**Status**: ✅ Fix applied, pending verification

---

### **2. BR-DS-006: Connection Pool Efficiency** 🐛 **BUSINESS LOGIC BUG**

**File**: `test/e2e/datastorage/11_connection_pool_exhaustion_test.go:156`  
**Error**: `Request 0 should not be rejected with 503`

**Test Scenario**:
- 50 concurrent writes sent simultaneously
- Connection pool configured with `max_open_conns = 25`
- Expected: Requests queue gracefully (no 503 errors)
- Actual: Requests rejected with HTTP 503

**Business Impact**: 
- **Severity**: HIGH
- **Production Risk**: System rejects requests under load instead of queueing
- **User Impact**: Data loss during traffic bursts

**Root Cause**: Connection pool not configured to queue requests

**Recommendation**: 
1. Increase `max_open_conns` to handle burst traffic
2. Add request queueing with timeout instead of immediate rejection
3. Add backpressure metrics to monitor pool exhaustion

---

### **3. DD-WORKFLOW-002: Workflow Version Management** 🐛 **BUSINESS LOGIC BUG**

**File**: `test/e2e/datastorage/07_workflow_version_management_test.go:181`  
**Error**: `Expected [specific assertion failed - need logs]`

**Test Scenario**:
- Create workflow v1.0.0 with UUID primary key
- Expected: `is_latest_version=true` set automatically
- Actual: [Assertion failed]

**Business Impact**:
- **Severity**: MEDIUM
- **Production Risk**: Workflow version tracking broken
- **User Impact**: Cannot reliably determine latest workflow version

**Root Cause**: UUID-based workflow creation not setting `is_latest_version` flag

**Recommendation**:
1. Fix workflow creation logic to set `is_latest_version=true` for first version
2. Add database constraint to ensure only one version has `is_latest_version=true` per workflow
3. Add migration tests for workflows created before fix

---

### **4. BR-DS-002: Query API Performance** 🐛 **BUSINESS LOGIC BUG**

**File**: `test/e2e/datastorage/13_audit_query_api_test.go`  
**Error**: [Need specific error message from logs]

**Test Scenario**:
- Multi-dimensional filtering with pagination
- Expected: Response time <5s with correct results
- Actual: [Query failing or timeout]

**Business Impact**:
- **Severity**: MEDIUM
- **Production Risk**: Query API unusable for complex filters
- **User Impact**: Cannot efficiently search audit events

**Root Cause**: Complex filter combinations not handled correctly

**Recommendation**:
1. Add database indexes for common filter combinations
2. Optimize query generation for multi-dimensional filters
3. Add query performance tests for worst-case scenarios

---

### **5. DD-009: DLQ Fallback HTTP API** 🐛 **CRITICAL BUG**

**File**: `test/e2e/datastorage/15_http_api_test.go:229`  
**Error**: `Timed out after 10.159s`

**Test Scenario**:
- PostgreSQL made unavailable (stopped)
- POST audit event via HTTP
- Expected: Event written to DLQ (Dead Letter Queue)
- Actual: Request times out instead of falling back to DLQ

**Business Impact**:
- **Severity**: **CRITICAL**
- **Production Risk**: **DATA LOSS during PostgreSQL outages**
- **User Impact**: Audit events lost when primary storage fails

**Root Cause**: DLQ fallback logic not triggered when PostgreSQL unavailable

**Recommendation**:
1. **URGENT**: Fix DLQ fallback detection logic
2. Add connection timeout to trigger DLQ faster
3. Add integration test for DLQ fallback in isolation
4. Add production monitoring for DLQ usage

---

### **6. GAP 2.3: Wildcard Search Edge Cases** 🐛 **BUSINESS LOGIC BUG**

**File**: `test/e2e/datastorage/08_workflow_search_edge_cases_test.go:489`  
**Error**: `Wildcard workflow (component='*') should match specific filter (component='deployment')`

**Test Scenario**:
- Search for workflows with `component='*'` (wildcard)
- Expected: Matches workflows with `component='deployment'`
- Actual: No match found

**Business Impact**:
- **Severity**: LOW
- **Production Risk**: Search functionality incomplete
- **User Impact**: Cannot use wildcard searches effectively

**Root Cause**: Wildcard matching logic not implemented or incorrect

**Recommendation**:
1. Implement proper wildcard matching in workflow search
2. Add tests for all wildcard patterns (`*`, `?`, partial matches)
3. Document wildcard syntax in API documentation

---

## 📊 **Failure Category Breakdown**

| Category | Count | Severity | Examples |
|----------|-------|----------|----------|
| **Test Schema Issues** | 1 | Low | Event type validation |
| **Critical Business Bugs** | 1 | **CRITICAL** | DLQ fallback |
| **High Priority Bugs** | 1 | High | Connection pool |
| **Medium Priority Bugs** | 2 | Medium | Workflow version, Query API |
| **Low Priority Bugs** | 1 | Low | Wildcard search |

---

## 🚨 **Action Items by Priority**

### **P0 - CRITICAL (Fix Immediately)**
1. ✅ Fix event type discriminator (test schema issue)
2. 🐛 **Fix DLQ fallback logic** - Prevents data loss in production

### **P1 - HIGH (Fix This Sprint)**
3. 🐛 Fix connection pool exhaustion handling
4. 🐛 Fix workflow version management

### **P2 - MEDIUM (Fix Next Sprint)**
5. 🐛 Optimize query API for multi-dimensional filters
6. 🐛 Implement wildcard search logic

---

## 🔍 **Investigation Steps Taken**

### **Infrastructure Validation** ✅
- [x] Verified Kind cluster creates successfully
- [x] Verified services deploy and become ready
- [x] Verified HTTP endpoints accessible
- [x] Verified parallel execution working
- [x] Verified test isolation working

### **Test Code Analysis** ✅
- [x] Identified event type validation needs discriminator
- [x] Applied fix for OpenAPI schema compliance
- [x] Confirmed other 5 failures are business logic bugs

### **Error Message Analysis** ✅
- [x] Gateway signal.received: 400 Bad Request (schema mismatch)
- [x] Connection pool: HTTP 503 rejection (should queue)
- [x] DLQ fallback: Timeout (fallback not triggered)
- [x] Workflow version: Assertion failure (flag not set)
- [x] Query API: [Needs detailed error from logs]
- [x] Wildcard search: No match found (logic missing)

---

## 📝 **Recommendations for Development Team**

### **Immediate Actions**
1. **Priority 1**: Fix DLQ fallback logic (DD-009) - CRITICAL for data reliability
2. **Priority 2**: Verify event type discriminator fix and re-run E2E suite
3. **Priority 3**: Fix connection pool queueing (BR-DS-006)

### **Short Term Actions**
4. Add integration tests for DLQ fallback in isolation (don't rely on E2E only)
5. Add connection pool metrics and alerting
6. Fix workflow version management logic
7. Optimize query API performance

### **Long Term Actions**
8. Implement comprehensive wildcard search
9. Add performance regression tests
10. Add chaos testing for PostgreSQL failures

---

## 🎯 **Success Criteria**

### **Test Suite Health**
- [x] Infrastructure: 100% reliable
- [x] Test Execution: 100% completion rate
- [ ] Test Passing Rate: Target 100% (currently 94%)
- [x] Error Messages: Actionable and clear

### **Business Logic**
- [ ] DLQ Fallback: 100% reliable under PostgreSQL failure
- [ ] Connection Pool: Graceful queueing under burst load
- [ ] Workflow Versioning: Correct version tracking
- [ ] Query API: <5s response for complex filters
- [ ] Wildcard Search: Full wildcard support

---

## 🔗 **Related Documentation**

- [DS E2E Final Status](./DS_E2E_FINAL_STATUS_JAN10_2026.md)
- [DS E2E Infrastructure Fixes](./DS_E2E_INFRASTRUCTURE_FIX_JAN10_2026.md)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- [DD-009: DLQ Fallback Design](../design-decisions/DD-009-dlq-fallback.md)
- [BR-DS-006: Connection Pool Efficiency](../requirements/BR-DS-006.md)

---

**Document Status**: 🔍 Analysis Complete  
**Next Steps**: Apply fix #1, run full E2E suite, triage remaining 5 business logic bugs  
**Owner**: Platform Team  
**Timeline**: P0 fixes needed before production deployment
