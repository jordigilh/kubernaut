# DD-AUTH-014: E2E Test Regression Analysis

**Date**: January 26, 2026  
**Authority**: DD-AUTH-014 (Middleware-based Authentication)  
**Status**: Test Migration Required  
**Impact**: 32 E2E tests require authentication updates

---

## Executive Summary

✅ **Auth Middleware**: Production-ready and working perfectly  
✅ **Zero Trust**: Correctly enforced across all DataStorage endpoints  
✅ **Integration Tests**: 111/111 passing (mock auth working)  
✅ **SAR Tests**: 6/6 passing (100% - real auth validated)  
❌ **32 E2E Tests**: Expected failures - tests using unauthenticated HTTP clients

**Root Cause**: NOT a regression - tests need migration to use authenticated clients.

---

## Analysis Summary

### What We Found

From must-gather logs (`/tmp/datastorage-e2e-logs-20260126-141234`):

**DataStorage Pod Logs** (`datastorage-e2e_datastorage-599d477564-sd2vn`):
- ✅ Auth middleware initialized: `Auth middleware enabled (DD-AUTH-014)`
- ✅ Namespace correctly detected: `auth_namespace: "datastorage-e2e"`
- ✅ Successfully authenticating authorized requests (201 Created)
- ✅ Successfully rejecting unauthorized requests (403 Forbidden)
- ✅ Zero 401 errors logged in DataStorage pod

**E2E Test Logs** (`/tmp/ds-e2e-full-suite.log`):
- ❌ 32 tests failing with `401 Unauthorized: Missing Authorization header`
- Pattern: Tests create their own `http.Client` without Bearer tokens
- Examples:
  - `test/e2e/datastorage/15_http_api_test.go:53` - Plain `http.Client{}`
  - `test/e2e/datastorage/20_legal_hold_api_test.go` - No auth
  - `test/e2e/datastorage/12_audit_write_api_test.go` - No auth

### Why This Is Correct Behavior

**Zero Trust Mandate**: User explicitly stated "All interaction needs to be secured to avoid damaging actors from attempting to interact with the audit endpoint without permission."

**Auth Middleware Working As Designed**:
1. Validates Bearer tokens via Kubernetes TokenReview API
2. Authorizes via SubjectAccessReview API
3. Injects `X-Auth-Request-User` header for audit attribution
4. Returns RFC 7807 JSON errors for 401/403 responses

### Test Results Breakdown

| Test Category | Status | Details |
|--------------|--------|---------|
| SAR Access Control (New) | ✅ 6/6 PASS | Auth middleware validated |
| Integration Tests | ✅ 111/111 PASS | Mock auth working |
| Unit Tests | ✅ 22/22 PASS | Auth components validated |
| Legacy E2E Tests | ❌ 32 FAIL | Need client migration |
| Other E2E Tests | ✅ 120 PASS | Already authenticated or no auth needed |

---

## Failing Test Patterns

### Pattern 1: Direct HTTP Client (Most Common)

**Example**: `test/e2e/datastorage/15_http_api_test.go:41-54`

```go
var _ = Describe("HTTP API Integration - POST /api/v1/audit/notifications", Ordered, func() {
    var (
        client     *http.Client  // ❌ Unauthenticated
        validAudit *models.NotificationAudit
    )

    BeforeAll(func() {
        client = &http.Client{Timeout: 10 * time.Second}  // ❌ No auth!
    })
```

**Fix Required**: Use authenticated client from suite setup or create `ServiceAccountTransport`

### Pattern 2: Custom HTTP Requests

**Example**: Tests making direct `http.NewRequest()` calls without Authorization header

**Fix Required**: Add `Authorization: Bearer <token>` header

### Affected Test Files

**High Priority** (Audit & Workflow Catalog):
1. `test/e2e/datastorage/12_audit_write_api_test.go` - Audit write API (BR-STORAGE-033)
2. `test/e2e/datastorage/13_audit_query_api_test.go` - Audit query API
3. `test/e2e/datastorage/14_audit_batch_write_api_test.go` - Batch audit write (DD-AUDIT-002)
4. `test/e2e/datastorage/15_http_api_test.go` - HTTP API integration
5. `test/e2e/datastorage/20_legal_hold_api_test.go` - Legal hold (BR-AUDIT-006)
6. `test/e2e/datastorage/04_workflow_search_test.go` - Workflow search (BR-DS-003)
7. `test/e2e/datastorage/07_workflow_version_management_test.go` - Workflow versioning

**Medium Priority** (SOC2 & Aggregation):
8. `test/e2e/datastorage/05_soc2_compliance_test.go` - SOC2 compliance features
9. `test/e2e/datastorage/16_aggregation_api_test.go` - Success rate aggregation (ADR-033)
10. `test/e2e/datastorage/17_metrics_api_test.go` - Prometheus metrics (BR-STORAGE-019)

**Lower Priority** (Edge Cases):
11. `test/e2e/datastorage/18_workflow_duplicate_api_test.go` - Duplicate detection (DS-BUG-001)
12. `test/e2e/datastorage/22_audit_validation_helper_test.go` - Test helpers
13. `test/e2e/datastorage/11_connection_pool_exhaustion_test.go` - Performance tests
14. `test/e2e/datastorage/06_workflow_search_audit_test.go` - Search audit trail

---

## Migration Strategy

### Phase 1: Update Suite-Level Client (RECOMMENDED FIRST)

**Goal**: Make the shared `dsClient` available to all tests

**Files to Update**:
- `test/e2e/datastorage/datastorage_e2e_suite_test.go`

**Changes**:
1. Export `dsClient` as a package-level variable
2. Ensure all parallel processes have access to authenticated client
3. Document usage in suite README

**Example**:
```go
var (
    // Shared authenticated client for all E2E tests (DD-AUTH-014)
    DsClient *dsgen.Client
    httpClient *http.Client
)

// In SynchronizedBeforeSuite (all processes)
func(data []byte) {
    // ... existing setup ...
    saTransport := testauth.NewServiceAccountTransport(e2eToken)
    httpClient = &http.Client{
        Timeout:   10 * time.Second,
        Transport: saTransport,
    }
    DsClient, err = dsgen.NewClient(dataStorageURL, dsgen.WithClient(httpClient))
    // ...
}
```

### Phase 2: Update Individual Test Files

**Priority Order**: High → Medium → Low (as listed above)

**Standard Fix Pattern**:

**Before**:
```go
BeforeAll(func() {
    client = &http.Client{Timeout: 10 * time.Second}
})
```

**After**:
```go
BeforeAll(func() {
    client = httpClient  // Use authenticated client from suite
})
```

**Alternative** (if tests need custom client):
```go
BeforeAll(func() {
    token, err := infrastructure.GetServiceAccountToken(ctx, namespace, saName, kubeconfigPath)
    Expect(err).ToNot(HaveOccurred())
    
    saTransport := testauth.NewServiceAccountTransport(token)
    client = &http.Client{
        Timeout:   10 * time.Second,
        Transport: saTransport,
    }
})
```

### Phase 3: Validation

After each test file update:

1. **Run specific test**: `ginkgo --focus="TestName" test/e2e/datastorage/`
2. **Verify auth**: Check for 201/200 responses instead of 401
3. **Check must-gather**: Confirm auth middleware logs show successful authorization
4. **Run full suite**: Ensure no new regressions

---

## Estimated Effort

| Phase | Files | Effort | Priority |
|-------|-------|--------|----------|
| Phase 1: Suite Update | 1 file | 1-2 hours | **CRITICAL** |
| Phase 2: High Priority Tests | 7 files | 4-6 hours | **HIGH** |
| Phase 2: Medium Priority Tests | 3 files | 2-3 hours | MEDIUM |
| Phase 2: Low Priority Tests | 4 files | 2-3 hours | LOW |
| Phase 3: Validation | All | 2-3 hours | **CRITICAL** |
| **TOTAL** | **15 files** | **11-17 hours** | - |

**Note**: Parallelizable - multiple test files can be updated simultaneously by different developers.

---

## Success Criteria

1. ✅ All 32 failing tests pass with authentication
2. ✅ Zero 401 Unauthorized errors in E2E runs
3. ✅ Auth middleware logs show successful authorization for all requests
4. ✅ Full E2E suite passes: `make test-e2e-datastorage`
5. ✅ Must-gather logs confirm Zero Trust enforcement

**Target**: All 189 tests passing (currently: 120 passing, 32 failing, 37 skipped/pending)

---

## Implementation Tracking

### Completed ✅
- [x] DD-AUTH-014 implementation (Phase 1 & 2)
- [x] Auth middleware with dependency injection
- [x] Integration tests updated (111/111 passing)
- [x] SAR E2E tests created (6/6 passing)
- [x] RBAC manifests updated (CRUD permissions)
- [x] OpenAPI spec updated (RFC 7807 errors)
- [x] Root cause analysis completed

### In Progress 🔄
- [ ] Phase 1: Export shared authenticated client
- [ ] Phase 2: Update 32 failing test files
- [ ] Phase 3: Full E2E validation

### Next Steps 📋
1. **Immediate**: Update `datastorage_e2e_suite_test.go` to export `DsClient`
2. **Week 1**: Update high-priority test files (audit, workflow)
3. **Week 2**: Update medium and low-priority test files
4. **Week 2**: Final validation and must-gather analysis
5. **Week 3**: Document lessons learned, create migration guide for HAPI

---

## Lessons Learned

### What Went Well ✅
1. **Zero Trust**: Correctly identified unauthenticated tests
2. **Systematic Triage**: Must-gather logs provided clear evidence
3. **No Business Logic Issues**: Auth middleware working perfectly
4. **Mock Pattern**: Integration tests continue to work with mock auth

### What to Improve 🔄
1. **Test Patterns**: Establish standard authenticated client usage
2. **Documentation**: Create E2E auth testing guide
3. **Linting**: Add lint rule to detect unauthenticated `http.Client{}` in E2E tests

### Key Insights 💡
1. **Zero Regressions**: This is NOT a regression - it's correct Zero Trust enforcement
2. **Expected Behavior**: 401 errors are the *correct* response to unauthenticated requests
3. **Migration Path Clear**: Simple pattern to fix all 32 tests
4. **Validation Critical**: Must-gather logs essential for debugging

---

## References

- **Authority**: DD-AUTH-014 (Middleware-based Authentication)
- **Test Plan**: E2E-DS-023 (SAR Access Control Validation)
- **Must-Gather**: `/tmp/datastorage-e2e-logs-20260126-141234`
- **Test Logs**: `/tmp/ds-e2e-full-suite.log`
- **Related**: DD-AUTH-010 (E2E Real Authentication Mandate)
- **Related**: DD-AUTH-011 (Granular RBAC & SAR Verb Mapping)

---

**Status**: Ready for implementation  
**Approval**: User approved Option A (Update all 32 failing tests)  
**Next Action**: Begin Phase 1 - Export shared authenticated client
