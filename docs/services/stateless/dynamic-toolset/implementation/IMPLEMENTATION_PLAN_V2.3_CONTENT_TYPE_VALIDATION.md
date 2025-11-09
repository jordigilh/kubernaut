# Dynamic Toolset Service - Implementation Plan v2.3

**Version**: v2.3 (Content-Type Validation - API Security & Standards)
**Date**: 2025-11-09
**Timeline**: 4 hours (Day 16)
**Status**: ⏸️ **PENDING APPROVAL**
**Based On**: IMPLEMENTATION_PLAN_V2.2_RFC7807_GRACEFUL_SHUTDOWN.md
**Parent Plan**: IMPLEMENTATION_PLAN_ENHANCED.md (Days 1-15 complete)

---

## 📋 Version History & Changelog

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v2.0** | 2025-10-11 | Enhanced plan with Gateway learnings (Days 1-13) | ✅ **COMPLETE** |
| **v2.1** | 2025-11-09 | RFC 7807 Error Responses extension (Day 14) | ✅ **COMPLETE** |
| **v2.2** | 2025-11-09 | RFC 7807 + Graceful Shutdown (Days 14-15) | ✅ **COMPLETE** |
| **v2.3** | 2025-11-09 | Content-Type Validation (Day 16) | ⏸️ **PENDING APPROVAL** |

### v2.3 Changelog (2025-11-09)

**Added**:
- ✅ **BR-TOOLSET-043**: Content-Type Validation Middleware (NEW)
- ✅ **Day 16**: Content-Type validation following TDD methodology
- ✅ Unit tests for Content-Type middleware (8 tests)
- ✅ Integration tests for Content-Type validation (4 tests)
- ✅ RFC 7807 error responses for invalid Content-Type (415 Unsupported Media Type)
- ✅ Support for `application/json` with optional `charset` parameter

**Modified**:
- Updated BR_MAPPING.md to include BR-TOOLSET-043
- Updated BUSINESS_REQUIREMENTS.md with Content-Type validation requirement

**Rationale**:
- **Security**: Prevent MIME confusion attacks and malformed request processing
- **API Contract**: Enforce strict Content-Type requirements per RFC 7231
- **Consistency**: Gateway and Context API already implement Content-Type validation
- **Standards**: Follow REST API best practices for Content-Type enforcement

**Dependencies**:
- ✅ DD-004: RFC 7807 Error Response Standard (approved)
- ✅ Gateway Service Content-Type middleware (reference)
- ✅ Context API Content-Type middleware (reference)
- ✅ Days 1-15 complete (service operational with RFC 7807)

---

## 🎯 Overview

This plan extends the Dynamic Toolset Service with **Content-Type validation middleware** to enforce API contract compliance and prevent security vulnerabilities.

**Why This Extension?**
1. **Security**: Prevent MIME confusion attacks and malformed request processing
2. **API Contract**: Enforce strict `application/json` requirement for POST/PUT/PATCH endpoints
3. **Consistency**: Gateway and Context API already implement this pattern
4. **Standards**: Follow RFC 7231 and REST API best practices

**Scope**:
- ✅ Content-Type validation middleware for POST/PUT/PATCH requests
- ✅ RFC 7807 error responses for invalid Content-Type (415 Unsupported Media Type)
- ✅ Support for `application/json` with optional `charset` parameter
- ✅ 8 unit tests for middleware behavior
- ✅ 4 integration tests for endpoint validation
- ✅ BR documentation and mapping
- ❌ No changes to business logic (service already operational)

---

## 📊 New Business Requirement

### BR-TOOLSET-043: Content-Type Validation Middleware

**Priority**: P1 (Security & API Standards)
**Status**: ⏸️ Pending Implementation
**Category**: API Security & Standards Compliance

**Description**:
All POST, PUT, and PATCH endpoints in the Dynamic Toolset Service MUST validate the `Content-Type` header and reject requests with invalid or missing Content-Type with a 415 Unsupported Media Type error in RFC 7807 format.

**Business Value**:
- **Security**: Prevent MIME confusion attacks and malformed request processing
- **API Contract**: Enforce strict Content-Type requirements per RFC 7231
- **Client Clarity**: Clear error messages when Content-Type is incorrect
- **Standards Compliance**: Follow REST API best practices

**Acceptance Criteria**:
1. ✅ POST, PUT, PATCH endpoints validate `Content-Type` header
2. ✅ Accept `application/json` (with or without `charset` parameter)
3. ✅ Reject requests with missing or invalid Content-Type (415 error)
4. ✅ Return RFC 7807 error response for invalid Content-Type
5. ✅ GET, DELETE, HEAD, OPTIONS requests are not affected
6. ✅ Unit tests cover all Content-Type scenarios
7. ✅ Integration tests validate endpoint behavior

**Use Cases**:

| Use Case | Content-Type Header | Expected Behavior |
|----------|---------------------|-------------------|
| **Valid JSON** | `application/json` | ✅ Request processed |
| **Valid JSON with charset** | `application/json; charset=utf-8` | ✅ Request processed |
| **Invalid: text/plain** | `text/plain` | ❌ 415 Unsupported Media Type |
| **Invalid: text/html** | `text/html` | ❌ 415 Unsupported Media Type |
| **Invalid: application/xml** | `application/xml` | ❌ 415 Unsupported Media Type |
| **Invalid: multipart/form-data** | `multipart/form-data` | ❌ 415 Unsupported Media Type |
| **Missing Content-Type** | (none) | ❌ 415 Unsupported Media Type |
| **GET request** | (any or none) | ✅ Not validated (GET doesn't require Content-Type) |

**Error Response Format** (RFC 7807):
```json
{
  "type": "https://kubernaut.io/errors/unsupported-media-type",
  "title": "Unsupported Media Type",
  "detail": "Content-Type must be 'application/json', got 'text/plain'",
  "status": 415,
  "instance": "/api/v1/discovery/trigger",
  "request_id": "req-123"
}
```

**Endpoints Affected**:
- POST `/api/v1/discovery/trigger` - Manual discovery trigger
- (Future: Any POST/PUT/PATCH endpoints added to the service)

---

## 🧪 Testing Strategy

### Unit Tests (8 tests)
**File**: `pkg/toolset/server/middleware/content_type_test.go`

**Test Scenarios**:
1. ✅ Valid `application/json` - request processed
2. ✅ Valid `application/json; charset=utf-8` - request processed
3. ✅ Invalid `text/plain` - 415 error
4. ✅ Invalid `text/html` - 415 error
5. ✅ Invalid `application/xml` - 415 error
6. ✅ Invalid `multipart/form-data` - 415 error
7. ✅ Missing Content-Type - 415 error
8. ✅ GET request - not validated (passes through)

**Test Behavior Validation**:
- Verify 415 status code for invalid Content-Type
- Verify RFC 7807 error response structure
- Verify request ID propagation in error response
- Verify GET/DELETE/HEAD/OPTIONS are not affected
- Verify `charset` parameter is handled correctly

### Integration Tests (4 tests)
**File**: `test/integration/toolset/content_type_validation_test.go`

**Test Scenarios**:
1. ✅ POST `/api/v1/discovery/trigger` with valid `application/json` - 200 OK
2. ✅ POST `/api/v1/discovery/trigger` with invalid `text/plain` - 415 error
3. ✅ POST `/api/v1/discovery/trigger` with missing Content-Type - 415 error
4. ✅ GET `/api/v1/health` without Content-Type - 200 OK (not validated)

**Test Behavior Validation**:
- Verify actual HTTP endpoint behavior
- Verify RFC 7807 error response from real server
- Verify request ID in error response
- Verify GET endpoints are not affected

---

## 📝 Implementation Plan (Day 16 - 4 hours)

### Phase 1: TDD RED - Unit Tests (1 hour)

**Step 1.1**: Create middleware test file
```bash
touch pkg/toolset/server/middleware/content_type_test.go
```

**Step 1.2**: Write 8 failing unit tests
- Test valid `application/json`
- Test valid `application/json; charset=utf-8`
- Test invalid Content-Type values (text/plain, text/html, application/xml, multipart/form-data)
- Test missing Content-Type
- Test GET request (should not validate)

**Step 1.3**: Run tests and verify RED phase
```bash
go test ./pkg/toolset/server/middleware/... -v
# Expected: 8 tests FAIL (middleware not yet implemented)
```

**Success Criteria**:
- ✅ 8 unit tests created
- ✅ All tests fail with "undefined: ValidateContentType"
- ✅ Test behavior validates functional correctness (not just structure)

---

### Phase 2: TDD GREEN - Minimal Implementation (1 hour)

**Step 2.1**: Create middleware implementation
```bash
touch pkg/toolset/server/middleware/content_type.go
```

**Step 2.2**: Implement `ValidateContentType` middleware
- Check HTTP method (only POST, PUT, PATCH)
- Extract Content-Type header
- Validate `application/json` (with optional `charset`)
- Return 415 RFC 7807 error for invalid Content-Type
- Propagate request ID

**Step 2.3**: Register middleware in server
```go
// pkg/toolset/server/server.go
r.Use(middleware.ValidateContentType)
```

**Step 2.4**: Run tests and verify GREEN phase
```bash
go test ./pkg/toolset/server/middleware/... -v
# Expected: 8 tests PASS
```

**Success Criteria**:
- ✅ 8 unit tests pass
- ✅ Middleware returns 415 for invalid Content-Type
- ✅ Middleware allows valid `application/json`
- ✅ GET requests are not affected

---

### Phase 3: Integration Tests (1 hour)

**Step 3.1**: Create integration test file
```bash
touch test/integration/toolset/content_type_validation_test.go
```

**Step 3.2**: Write 4 failing integration tests
- Test POST with valid Content-Type
- Test POST with invalid Content-Type
- Test POST with missing Content-Type
- Test GET without Content-Type (should pass)

**Step 3.3**: Run integration tests
```bash
go test ./test/integration/toolset/... -v -run TestContentType
# Expected: 4 tests PASS (middleware already implemented)
```

**Success Criteria**:
- ✅ 4 integration tests pass
- ✅ Real HTTP endpoints validate Content-Type
- ✅ RFC 7807 error responses returned
- ✅ GET endpoints unaffected

---

### Phase 4: REFACTOR & Documentation (1 hour)

**Step 4.1**: REFACTOR enhancements
- Add Prometheus metrics for Content-Type validation failures
- Enhance error messages with specific Content-Type received
- Add structured logging for invalid Content-Type attempts

**Step 4.2**: Update BR documentation
```bash
# Update BUSINESS_REQUIREMENTS.md
- Add BR-TOOLSET-043 to summary
- Add Content-Type validation section
- Update test coverage statistics
- Update version history
```

**Step 4.3**: Run full test suite
```bash
make test
make test-integration
# Expected: All tests pass (194 unit + 52 integration + 13 E2E = 259 + 12 new = 271 total)
```

**Step 4.4**: Commit changes
```bash
git add pkg/toolset/server/middleware/content_type*
git add test/integration/toolset/content_type_validation_test.go
git add docs/services/stateless/dynamic-toolset/BUSINESS_REQUIREMENTS.md
git commit -m "feat(middleware): Add Content-Type validation middleware (BR-TOOLSET-043)"
```

**Success Criteria**:
- ✅ All tests pass (271 total)
- ✅ BR documentation updated
- ✅ Prometheus metrics added
- ✅ Changes committed

---

## 🎯 Success Metrics

**Test Coverage**:
- Unit Tests: 202 specs (194 existing + 8 new)
- Integration Tests: 56 scenarios (52 existing + 4 new)
- E2E Tests: 13 scenarios (unchanged)
- **Total**: 271 test specs

**BR Coverage**:
- Total BRs: 11 (10 existing + 1 new)
- Unit Test Coverage: 82% (9/11 BRs)
- Integration Test Coverage: 100% (11/11 BRs)
- E2E Test Coverage: 55% (6/11 BRs)
- **Overall Coverage**: 100%

**Confidence Assessment**: 95%
- Content-Type validation is straightforward middleware
- Gateway and Context API provide proven reference implementations
- Unit and integration tests provide comprehensive coverage
- RFC 7807 integration already complete

---

## 🔗 Reference Implementations

### Gateway Service
- **File**: `pkg/gateway/middleware/content_type.go`
- **Tests**: `pkg/gateway/middleware/content_type_test.go`
- **Integration**: `test/integration/gateway/priority1_error_propagation_test.go`

### Context API Service
- **File**: `pkg/contextapi/middleware/content_type.go`
- **Tests**: `pkg/contextapi/middleware/content_type_test.go`
- **Integration**: `test/integration/contextapi/09_rfc7807_compliance_test.go`

---

## 📋 Testing Do's and Don'ts

### ✅ DO
- **Validate Behavior**: Test that 415 errors are returned for invalid Content-Type
- **Test Functional Correctness**: Verify actual HTTP status codes and error messages
- **Test Edge Cases**: Missing Content-Type, charset parameters, case sensitivity
- **Verify RFC 7807**: Ensure error responses follow RFC 7807 format
- **Test Request ID**: Verify request ID propagation in error responses

### ❌ DON'T
- **Don't Test Structure Only**: Avoid tests that only check "not nil" or "length > 0"
- **Don't Skip Behavior**: Always verify the actual business outcome (415 status, error message)
- **Don't Test Implementation**: Focus on "what" (behavior) not "how" (implementation)
- **Don't Over-Mock**: Use real HTTP requests in integration tests

---

## 🚀 Deployment Considerations

**No Deployment Changes Required**:
- Middleware is backward compatible (only validates POST/PUT/PATCH)
- GET endpoints unaffected (no breaking changes)
- RFC 7807 error format already deployed
- No configuration changes needed

**Rollout Strategy**:
- Deploy as part of normal rolling update
- Monitor for 415 errors (indicates clients with invalid Content-Type)
- No rollback needed (backward compatible)

---

**Plan Status**: ⏸️ **PENDING USER APPROVAL**
**Next Step**: User approval to proceed with Day 16 implementation
**Timeline**: 4 hours (TDD RED → GREEN → Integration → REFACTOR)
**Confidence**: 95% (straightforward middleware with proven patterns)

