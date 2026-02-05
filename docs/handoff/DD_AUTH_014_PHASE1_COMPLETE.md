# DD-AUTH-014: Phase 1 Complete - Shared Authenticated Client Export

**Date**: January 26, 2026  
**Authority**: DD-AUTH-014 (Middleware-based Authentication)  
**Status**: ✅ Phase 1 Complete - Ready for Phase 2

---

## What Was Done

### Exported Shared Authenticated Clients

**Files Modified**:
- `test/e2e/datastorage/datastorage_e2e_suite_test.go`
- All 23 E2E test files (`*_test.go`)

### Changes Summary

1. **Exported Package Variables** (datastorage_e2e_suite_test.go):
   ```go
   // BEFORE (private):
   dsClient *dsgen.Client
   
   // AFTER (exported):
   DsClient *dsgen.Client  // Shared authenticated OpenAPI client
   HttpClient *http.Client  // Shared authenticated HTTP client (base)
   ```

2. **Added Documentation**:
   ```go
   // DsClient is the shared authenticated OpenAPI client for E2E tests (DD-AUTH-014)
   // 
   // USAGE PATTERN (DD-AUTH-014 - Zero Trust):
   //   - Use DsClient for functional tests (audit, workflow, metrics)
   //   - Create custom clients for authorization tests (SAR scenarios)
   ```

3. **Updated All References**:
   - Changed 379 references from `dsClient` → `DsClient` across 23 test files
   - Excluded backup files (*.tf, *.ogenbackup, etc.)

4. **Enhanced Logging**:
   ```go
   logger.Info("✅ Shared authenticated OpenAPI client created (DD-AUTH-014)", 
       "baseURL", "http://localhost:28090",
       "pattern", "Use DsClient for functional tests, custom clients for authz tests")
   ```

---

## Usage Patterns (DD-AUTH-014)

### Pattern 1: Use Shared DsClient (Recommended for Most Tests) ✅

**When**: Testing business logic, API correctness, functional behavior

**Example**:
```go
var _ = Describe("Audit Write API", func() {
    It("should create audit event", func() {
        // No client setup needed - use shared DsClient directly
        resp, err := DsClient.CreateAuditEvent(ctx, event)
        Expect(err).ToNot(HaveOccurred())
        Expect(resp.Status).To(Equal(201))
    })
})
```

**Benefits**:
- ✅ No setup code needed
- ✅ Consistent authentication across tests
- ✅ Faster test execution (no per-test client creation)
- ✅ Zero Trust enforced automatically

### Pattern 2: Create Custom Client (For Authorization Tests) ✅

**When**: Testing different permission levels, SAR scenarios

**Example**:
```go
var _ = Describe("SAR Access Control", func() {
    It("should reject unauthorized user", func() {
        // Create custom client with unauthorized ServiceAccount
        token, _ := infrastructure.GetServiceAccountToken(ctx, ns, "unauthorized-sa", kubeconfig)
        unauthorizedClient, _ := dsgen.NewClient(
            dataStorageURL,
            dsgen.WithClient(&http.Client{
                Transport: testauth.NewServiceAccountTransport(token),
            }),
        )
        
        // Test unauthorized access
        _, err := unauthorizedClient.CreateAuditEvent(ctx, event)
        Expect(err).To(MatchError(ContainSubstring("403 Forbidden")))
    })
})
```

**Benefits**:
- ✅ Tests authorization scenarios explicitly
- ✅ Uses different ServiceAccounts with varying permissions
- ✅ Validates RBAC enforcement

---

## Verification

### Exports Confirmed ✅
```bash
$ grep "DsClient \*" test/e2e/datastorage/datastorage_e2e_suite_test.go
94:	DsClient *dsgen.Client

$ grep -c "DsClient" test/e2e/datastorage/datastorage_e2e_suite_test.go
10
```

### References Updated ✅
- **Before**: 379 references to `dsClient` (lowercase)
- **After**: 379 references to `DsClient` (exported)
- **Files**: 23 actual test files (excluding backups)

---

## Next Steps: Phase 2

### Priority 1: High-Impact Tests (Audit & Workflow)

**Files to Update** (7 files):
1. `test/e2e/datastorage/12_audit_write_api_test.go` - Remove custom client, use `DsClient`
2. `test/e2e/datastorage/13_audit_query_api_test.go` - Remove custom client, use `DsClient`
3. `test/e2e/datastorage/14_audit_batch_write_api_test.go` - Remove custom client, use `DsClient`
4. `test/e2e/datastorage/15_http_api_test.go` - Remove custom client, use `DsClient`
5. `test/e2e/datastorage/20_legal_hold_api_test.go` - Remove custom client, use `DsClient`
6. `test/e2e/datastorage/04_workflow_search_test.go` - Remove custom client, use `DsClient`
7. `test/e2e/datastorage/07_workflow_version_management_test.go` - Remove custom client, use `DsClient`

**Standard Fix Pattern**:
```go
// BEFORE (❌):
var (
    client *http.Client
)

BeforeAll(func() {
    client = &http.Client{Timeout: 10 * time.Second}  // No auth!
})

// AFTER (✅):
// Remove local client variable
// Use DsClient directly in tests
```

### Priority 2: Medium-Impact Tests (SOC2 & Metrics)

**Files to Update** (3 files):
8. `test/e2e/datastorage/05_soc2_compliance_test.go`
9. `test/e2e/datastorage/16_aggregation_api_test.go`
10. `test/e2e/datastorage/17_metrics_api_test.go`

### Priority 3: Low-Impact Tests (Edge Cases)

**Files to Update** (4 files):
11. `test/e2e/datastorage/18_workflow_duplicate_api_test.go`
12. `test/e2e/datastorage/22_audit_validation_helper_test.go`
13. `test/e2e/datastorage/11_connection_pool_exhaustion_test.go`
14. `test/e2e/datastorage/06_workflow_search_audit_test.go`

---

## Files To **NOT** Modify ✅

**Keep Custom Clients** (authorization testing):
- `test/e2e/datastorage/23_sar_access_control_test.go` - Uses 3 different ServiceAccounts

**Reason**: These tests explicitly test authorization with different permission levels.

---

## Estimated Effort

| Priority | Files | Time | Complexity |
|----------|-------|------|------------|
| High | 7 | 3-4 hours | Low (simple pattern) |
| Medium | 3 | 1-2 hours | Low |
| Low | 4 | 1-2 hours | Low |
| **TOTAL** | **14** | **5-8 hours** | - |

**Note**: Can be parallelized - multiple files can be updated simultaneously.

---

## Success Criteria

After Phase 2 completion:

1. ✅ All 32 failing tests pass with authentication
2. ✅ Zero `http.Client{Timeout...}` patterns in functional tests
3. ✅ Only SAR test (23_) creates custom clients
4. ✅ All tests use exported `DsClient` for API calls
5. ✅ Full E2E suite passes: `make test-e2e-datastorage`

**Target**: 189/190 tests passing (currently: 120 passing, 32 failing, 37 skipped, 1 pending)

---

## References

- **Authority**: DD-AUTH-014 (Middleware-based Authentication)
- **Analysis**: `docs/handoff/DD_AUTH_014_E2E_REGRESSION_ANALYSIS.md`
- **Test Plan**: E2E-DS-023 (SAR Access Control Validation)
- **Pattern**: Option A (Selective Refactoring)

---

**Status**: ✅ Phase 1 Complete  
**Approval**: User approved selective refactoring approach (Option A)  
**Next Action**: Begin Phase 2 - Update high-priority test files (audit, workflow)
