# ogenx Utility Refactor Plan

**Date**: February 3, 2026  
**Status**: Phase 1 Complete - Ready for Rollout  
**Authority**: OGEN_ERROR_HANDLING_INVESTIGATION_FEB_03_2026.md (SME-validated)

---

## Overview

The `pkg/ogenx` utility provides a **generic, SME-validated pattern** for converting ogen-generated client responses to Go errors. This eliminates boilerplate and provides consistent error handling across all services.

### Key Benefits

- ✅ **Single point of maintenance** for ogen error handling
- ✅ **Works for all ogen clients** (DataStorage, HAPI, future services)
- ✅ **Handles both patterns**: undefined status codes (error strings) and defined status codes (typed responses)
- ✅ **SME-validated** community-standard approach
- ✅ **Preserves type safety** - original response available for detailed inspection

---

## Implementation Status

### ✅ Phase 1: Core Utility (COMPLETE)

**Created**:
- `pkg/ogenx/error.go` - Core `ToError()` function
- `pkg/ogenx/error_test.go` - Comprehensive unit tests (11 tests, all passing)

**Functionality**:
- ✅ Network error passthrough
- ✅ Undefined status code parsing (`"unexpected status code: 503"` → `HTTPError{StatusCode: 503}`)
- ✅ Typed response status extraction (`GetStatus() int32`)
- ✅ RFC 7807 title extraction (`GetTitle() string`)
- ✅ Message extraction (`GetMessage() string`)
- ✅ HTTP error identification (`IsHTTPError()`, `GetHTTPError()`)
- ✅ Error formatting (`"HTTP 400: Validation Error"`)

**Known Limitation**:
- RFC 7807 "detail" field extraction needs enhancement (see Future Enhancements)
- Currently preserves typed response for manual inspection
- Title + status code extraction works perfectly

---

## Rollout Plan

### Phase 2: HAPI E2E Tests (NEXT - Estimated: 30-45 min)

**Goal**: Replace endpoint-specific wrappers with `ogenx.ToError()`

**Files to Update**:
1. `test/e2e/holmesgpt-api/incident_analysis_test.go`
   - Find: Manual `CheckIncidentAnalyzeError()` calls
   - Replace with: `ogenx.ToError(resp, err)`
   - Affected tests: E2E-HAPI-007, 008, others

2. `test/e2e/holmesgpt-api/recovery_analysis_test.go`
   - Find: Manual `CheckRecoveryAnalyzeError()` calls  
   - Replace with: `ogenx.ToError(resp, err)`
   - Affected tests: E2E-HAPI-018, others

3. **Remove**: `test/e2e/holmesgpt-api/ogen_error_helper.go`
   - No longer needed - replaced by generic utility

**Testing**:
```bash
go test ./test/e2e/holmesgpt-api/... -v
```

**Expected Result**: Same pass rate (33/40), but with generic utility

**Success Criteria**:
- ✅ All HAPI E2E tests pass at current rate
- ✅ Error messages remain useful
- ✅ Less boilerplate code

---

### Phase 3: DataStorage Audit Client (Estimated: 20-30 min)

**Goal**: Replace `parseOgenError()` with `ogenx.ToError()`

**Files to Update**:
1. `pkg/audit/openapi_client_adapter.go`
   - Find: `parseOgenError(err)` calls
   - Replace with: `ogenx.ToError(nil, err)` (for undefined status codes)
   - Note: Keep `OpenAPIClientAdapter` wrapper - just change internal implementation

**Code Changes**:
```go
// OLD (custom parser)
resp, err := a.client.CreateAuditEventsBatch(ctx, valueEvents)
if err != nil {
    return parseOgenError(err)  // ← Custom implementation
}

// NEW (generic utility)
resp, err := a.client.CreateAuditEventsBatch(ctx, valueEvents)
err = ogenx.ToError(resp, err)  // ← Generic utility
if err != nil {
    return err
}
```

**Remove**: `parseOgenError()` function - no longer needed

**Testing**:
```bash
go test ./pkg/audit/... -v
go test ./test/integration/*/audit*.go -v
```

**Success Criteria**:
- ✅ All audit client tests pass
- ✅ Error types remain compatible (HTTPError vs custom error)
- ✅ Integration tests pass

---

### Phase 4: Service-Wide Audit (Estimated: 1-2 hours)

**Goal**: Find and update all ogen client usages across services

**Search Strategy**:
```bash
# Find all ogen client imports
grep -r "datastorage/ogen-client\|holmesgpt/client" --include="*.go" pkg/ cmd/

# Find CreateAuditEventsBatch calls
grep -r "CreateAuditEventsBatch" --include="*.go" pkg/ cmd/

# Find HAPI client calls  
grep -r "IncidentAnalyze\|RecoveryAnalyze" --include="*.go" pkg/ cmd/
```

**Services to Check**:
1. **AIAnalysis** (`pkg/aianalysis/`, `cmd/aianalysis/`)
   - Uses DataStorage for audit
   - May use HAPI client for production integration

2. **Gateway** (`pkg/gateway/`, `cmd/gateway/`)
   - Uses DataStorage for audit
   - Heavy audit usage

3. **RemediationOrchestrator** (`pkg/remediationorchestrator/`, `cmd/remediationorchestrator/`)
   - Uses DataStorage for audit
   - Workflow catalog operations

4. **WorkflowExecution** (`pkg/workflowexecution/`, `cmd/workflowexecution/`)
   - Uses DataStorage for audit

5. **SignalProcessing** (`pkg/signalprocessing/`, `cmd/signalprocessing/`)
   - Uses DataStorage for audit

6. **Notification** (`pkg/notification/`, `cmd/notification/`)
   - Uses DataStorage for audit

**Pattern to Apply**:
```go
// For services using DataStorage audit client
// Most should already use OpenAPIClientAdapter, which we'll update in Phase 3
// Verify they're using the adapter, not direct ogen client

// For any direct ogen client usage:
resp, err := client.SomeEndpoint(ctx, req)
err = ogenx.ToError(resp, err)  // ← Add this
if err != nil {
    // Handle error
}
```

**Testing Strategy**:
- Run integration tests for each service
- Run E2E tests for affected workflows
- Verify audit events still flow correctly

**Success Criteria**:
- ✅ All service integration tests pass
- ✅ No regression in error handling
- ✅ Consistent error messages across services

---

### Phase 5: Documentation & Cleanup (Estimated: 30 min)

**Documentation Updates**:
1. **Update**: `docs/development/project guidelines.md`
   - Add ogenx usage guidelines
   - Example code snippets
   - Best practices

2. **Update**: Testing guidelines
   - How to test ogen client interactions
   - Error handling patterns

3. **Create**: `pkg/ogenx/README.md`
   - Quick start guide
   - Common patterns
   - Troubleshooting

**Cleanup**:
1. Remove deprecated error helpers:
   - `test/e2e/holmesgpt-api/ogen_error_helper.go` (after Phase 2)
   - `parseOgenError()` in audit client (after Phase 3)

2. Update imports:
   - Add `github.com/jordigilh/kubernaut/pkg/ogenx` where needed
   - Remove custom error helper imports

**Code Review Checklist**:
- [ ] All ogen client calls use `ogenx.ToError()`
- [ ] No custom error parsers remaining
- [ ] Error messages remain useful
- [ ] Tests pass at same or better rate
- [ ] Documentation updated

---

## Future Enhancements

### Enhancement 1: Improve RFC 7807 Detail Extraction

**Current Limitation**: Can't easily access OptString.Value field via interfaces

**Options**:
1. **Use reflection** to access struct fields
2. **Add extension methods** to real ogen types (via generate directives)
3. **Create typed wrappers** for common ogen patterns
4. **Wait for ogen feature**: File feature request for GetValue() methods on OptString types

**Priority**: Low - current implementation works for most use cases

**Estimated Effort**: 2-4 hours

---

### Enhancement 2: Structured Error Details

**Goal**: Extract field-level validation errors from RFC 7807 responses

**Use Case**:
```go
err := ogenx.ToError(resp, origErr)
if httpErr := ogenx.GetHTTPError(err); httpErr != nil {
    // Access field errors: map[string]string
    fieldErrors := httpErr.FieldErrors
    if emailErr, ok := fieldErrors["email"]; ok {
        fmt.Printf("Email validation failed: %s\n", emailErr)
    }
}
```

**Implementation**:
- Add `FieldErrors map[string]string` to `HTTPError`
- Extract from RFC 7807 `field_errors` field
- Use similar interface pattern as detail extraction

**Priority**: Medium - useful for validation error debugging

**Estimated Effort**: 2-3 hours

---

### Enhancement 3: Custom Error Types per Service

**Goal**: Allow services to define custom error types wrapping HTTPError

**Use Case**:
```go
type DataStorageError struct {
    *ogenx.HTTPError
    RetryAfter time.Duration
    IsRetryable bool
}

func (e *DataStorageError) Unwrap() error {
    return e.HTTPError
}
```

**Implementation**:
- Define service-specific error types
- Wrap ogenx.HTTPError
- Add service-specific fields
- Maintain error chain for `errors.Is()` / `errors.As()`

**Priority**: Low - can be done incrementally by services

**Estimated Effort**: 1-2 hours per service

---

## Testing Strategy

### Unit Tests (Current: 11 tests, all passing)

**Coverage**:
- ✅ Network errors
- ✅ Undefined status codes (503, 429)
- ✅ Defined status codes (400, 422, 500)
- ✅ Success responses (200, 201)
- ✅ RFC 7807 extraction (title, message)
- ✅ Error formatting
- ✅ Helper functions (IsHTTPError, GetHTTPError)

**Run Tests**:
```bash
go test ./pkg/ogenx/... -v -cover
```

**Expected**: 100% pass rate, >90% coverage

---

### Integration Tests

**Phase 2** (HAPI E2E):
```bash
make test-e2e-holmesgpt-api
```
**Expected**: 33/40 passing (current baseline)

**Phase 3** (DataStorage Audit):
```bash
go test ./pkg/audit/... -v
go test ./test/integration/*/audit*.go -v
```
**Expected**: All passing

**Phase 4** (Service-Wide):
```bash
# Run integration tests for each service
make test-integration-aianalysis
make test-integration-gateway
make test-integration-remediationorchestrator
make test-integration-workflowexecution
make test-integration-signalprocessing
make test-integration-notification
```
**Expected**: No regression

---

### E2E Tests

**Full Suite**:
```bash
make test-e2e  # Runs all E2E tests
```

**Per-Service**:
```bash
make test-e2e-holmesgpt-api
make test-e2e-gateway
make test-e2e-aianalysis
# etc.
```

**Expected**: No regression in pass rates

---

## Risk Assessment

### Low Risk

- ✅ **SME-validated pattern** - this is community-standard
- ✅ **Well-tested utility** - 11 unit tests, all passing
- ✅ **Backward compatible** - can roll out incrementally
- ✅ **Type-safe** - preserves original responses

### Potential Issues

1. **Error Message Changes**
   - **Risk**: Tests checking exact error messages may break
   - **Mitigation**: Update test assertions to check substrings
   - **Example**: `Contains("HTTP 400")` instead of exact match

2. **Custom Error Type Dependencies**
   - **Risk**: Code relying on custom error types (e.g., `HTTPError` from audit client)
   - **Mitigation**: `ogenx.HTTPError` has same fields, use `errors.As()` for compatibility

3. **Performance**
   - **Risk**: Additional type assertions may add overhead
   - **Mitigation**: Negligible - interface checks are fast
   - **Validation**: Benchmark if needed

---

## Rollback Plan

### Per-Phase Rollback

**If Phase 2 (HAPI E2E) fails**:
1. Revert changes to test files
2. Keep `ogen_error_helper.go`
3. No impact on production code

**If Phase 3 (Audit Client) fails**:
1. Revert `openapi_client_adapter.go`
2. Restore `parseOgenError()` function
3. Run regression tests

**If Phase 4 (Service-Wide) fails**:
1. Revert per-service changes
2. Each service isolated - no cascading failures

**Nuclear Option**: Remove `pkg/ogenx` entirely
- No production dependencies yet
- Only test code affected
- Can return to endpoint-specific wrappers

---

## Success Metrics

### Code Quality
- [ ] Reduced boilerplate (estimate: -200 lines across all tests)
- [ ] Single point of maintenance
- [ ] Consistent error handling

### Test Pass Rates
- [ ] HAPI E2E: Maintain 33/40 (82.5%) or better
- [ ] Integration tests: 100% pass rate
- [ ] E2E tests: No regression

### Developer Experience
- [ ] Easier to add new ogen clients
- [ ] Clear error messages
- [ ] Simple API (`ogenx.ToError()`)

---

## Timeline

| Phase | Duration | Dependencies |
|-------|----------|--------------|
| Phase 1: Core Utility | ✅ Complete | None |
| Phase 2: HAPI E2E Tests | 30-45 min | Phase 1 |
| Phase 3: Audit Client | 20-30 min | Phase 1 |
| Phase 4: Service-Wide | 1-2 hours | Phases 2-3 |
| Phase 5: Documentation | 30 min | Phases 2-4 |
| **Total** | **2.5-4 hours** | Sequential |

**Recommended**: Execute phases sequentially, validate each before proceeding

---

## Next Steps

1. **Get user approval** for Phase 2 (HAPI E2E tests)
2. **Execute Phase 2**: Update HAPI E2E tests
3. **Validate**: Run E2E tests, verify no regression
4. **Proceed to Phase 3**: Update audit client
5. **Continue systematically** through remaining phases

**Ready to proceed?** Phase 2 is low-risk and provides immediate value.
