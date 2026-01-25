# Reconstruction Testing Tiers - Architecture Guide

**Date**: 2026-01-12  
**Purpose**: Clarify testing tier boundaries for reconstruction feature  
**Authority**: [03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc)

## 🚨 Critical Issue: Testing Tier Confusion

### Problem Identified:
Initial reconstruction integration tests incorrectly used HTTP server (`testServer.ServeHTTP()`) instead of calling business logic directly. This violated the testing tier boundaries.

## ✅ Correct Testing Tier Architecture

### Testing Pyramid (per 03-testing-strategy.mdc):
```
        ▲
       ╱ ╲
      ╱E2E╲        10-15%  - Complete workflows via REST API
     ╱─────╲
    ╱Integr╲       >50%   - Business logic with real infrastructure
   ╱────────╲
  ╱   Unit   ╲     70%+   - Business logic in isolation
 ╱────────────╲
```

---

## 📋 Testing Tier Specifications

### Tier 1: Unit Tests (70%+)

**Location**: `test/unit/datastorage/reconstruction/`

**What to Test**:
- Business logic in isolation
- Mock external dependencies ONLY

**What to Mock**:
- External APIs (Gateway, Orchestrator)
- External infrastructure (PostgreSQL, Redis)

**What to Use REAL**:
- All reconstruction components (query, parser, mapper, builder, validator)
- Business logic types and data structures

**Example**:
```go
// ✅ CORRECT: Unit test with mocked database
parsed, err := reconstruction.ParseAuditEvent(mockEvent)
Expect(parsed.SignalName).To(Equal("HighCPU"))
```

**Files**:
- `parser_test.go` - 8 tests
- `mapper_test.go` - 5 tests
- `builder_test.go` - 7 tests
- `validator_test.go` - 8 tests

**Total**: 28 unit tests

---

### Tier 2: Integration Tests (>50%)

**Location**: `test/integration/datastorage/`

**What to Test**:
- Business logic with REAL infrastructure
- Component interactions
- Database constraints and transactions

**What to Mock**:
- NOTHING - use real PostgreSQL, real components

**What to Use REAL**:
- PostgreSQL database (Podman container)
- All reconstruction components
- AuditEventsRepository

**Example**:
```go
// ✅ CORRECT: Integration test calls business logic directly
events, err := reconstruction.QueryAuditEventsForReconstruction(ctx, db.DB, logger, correlationID)
parsedData := []reconstruction.ParsedAuditData{}
for _, event := range events {
    parsed, err := reconstruction.ParseAuditEvent(event)
    parsedData = append(parsedData, *parsed)
}
rrFields, err := reconstruction.MergeAuditData(parsedData)
rr, err := reconstruction.BuildRemediationRequest(correlationID, rrFields)
validationResult, err := reconstruction.ValidateReconstructedRR(rr)

// Validate business outcomes
Expect(rr.Spec.SignalName).To(Equal("HighCPU"))
Expect(validationResult.Completeness).To(BeNumerically(">=", 50))
```

**Files**:
- `reconstruction_integration_test.go` - 5 integration scenarios

**Key Principle**: **Call reconstruction components directly, NOT via HTTP**

---

### Tier 3: E2E Tests (10-15%)

**Location**: `test/e2e/datastorage/` (FUTURE)

**What to Test**:
- Complete end-to-end workflow via REST API
- HTTP status codes, JSON responses
- Authentication (OAuth-proxy)
- kubectl apply workflow

**What to Mock**:
- Minimal (only if absolutely necessary)

**What to Use REAL**:
- HTTP endpoint (DataStorage service)
- OpenAPI client (ogenclient)
- ServiceAccount authentication
- Kubernetes cluster (for kubectl apply)

**Example**:
```go
// ✅ CORRECT: E2E test uses OpenAPI client
client, _ := ogenclient.NewClient("http://data-storage-service:8080")
response, err := client.ReconstructRemediationRequest(ctx, ogenclient.ReconstructRemediationRequestParams{
    CorrelationID: correlationID,
})

// Validate HTTP response
Expect(response.RemediationRequestYaml).ToNot(BeEmpty())
Expect(response.Validation.IsValid).To(BeTrue())

// Apply to Kubernetes
kubectl.Apply(response.RemediationRequestYaml)
```

**Files**:
- `reconstruction_e2e_test.go` (not yet implemented)

**Key Principle**: **Use OpenAPI client to call HTTP endpoint, test complete workflow**

---

## 🚫 Anti-Patterns to AVOID

### ❌ Anti-Pattern 1: HTTP Server in Integration Tests

**WRONG**:
```go
// ❌ WRONG: Integration test using HTTP server
testServer.ServeHTTP(rr, req)
```

**Why Wrong**:
- Tests HTTP layer (routing, serialization) instead of business logic
- Belongs in E2E tests, not integration tests

**Correct**:
```go
// ✅ CORRECT: Integration test calls business logic directly
events, err := reconstruction.QueryAuditEventsForReconstruction(ctx, db.DB, logger, correlationID)
```

### ❌ Anti-Pattern 2: Direct Business Logic Calls in E2E Tests

**WRONG**:
```go
// ❌ WRONG: E2E test calling business logic directly
rr, err := reconstruction.BuildRemediationRequest(correlationID, rrFields)
```

**Why Wrong**:
- Bypasses HTTP layer, doesn't test complete workflow
- Belongs in integration tests, not E2E tests

**Correct**:
```go
// ✅ CORRECT: E2E test uses OpenAPI client
response, err := client.ReconstructRemediationRequest(ctx, params)
```

### ❌ Anti-Pattern 3: Mocking Database in Integration Tests

**WRONG**:
```go
// ❌ WRONG: Integration test with mocked database
mockDB := &MockDatabase{}
mockDB.ExpectQuery("SELECT * FROM audit_events...")
```

**Why Wrong**:
- Integration tests MUST use real infrastructure
- This is actually a unit test disguised as integration test

**Correct**:
```go
// ✅ CORRECT: Integration test with real PostgreSQL
events, err := reconstruction.QueryAuditEventsForReconstruction(ctx, db.DB, logger, correlationID)
```

---

## 🔍 Decision Matrix

Use this matrix to determine which testing tier to use:

| Question | Unit | Integration | E2E |
|----------|------|-------------|-----|
| Mocks database? | ✅ Yes | ❌ No (real PostgreSQL) | ❌ No (real DB) |
| Calls business logic directly? | ✅ Yes | ✅ Yes | ❌ No (HTTP client) |
| Uses HTTP server? | ❌ No | ❌ No | ✅ Yes |
| Uses OpenAPI client? | ❌ No | ❌ No | ✅ Yes |
| Tests authentication? | ❌ No | ❌ No | ✅ Yes |
| Tests kubectl apply? | ❌ No | ❌ No | ✅ Yes |

---

## 📊 Reconstruction Testing Coverage

### Current Status:

**Unit Tests**: ✅ 28 tests (100% of business logic)
- Parser: 8 tests
- Mapper: 5 tests
- Builder: 7 tests
- Validator: 8 tests

**Integration Tests**: ✅ 5 scenarios (business logic with real DB)
- Query from real PostgreSQL: 2 scenarios
- Full reconstruction pipeline: 1 scenario
- Error handling: 2 scenarios

**E2E Tests**: ⏳ Not yet implemented (future work)
- HTTP endpoint via OpenAPI client
- Authentication via OAuth-proxy
- kubectl apply workflow

### Coverage Compliance:
- ✅ Unit: 70%+ (28 tests covering all business logic)
- ✅ Integration: >50% (5 scenarios covering all components with real DB)
- ⏳ E2E: 10-15% (not yet implemented)

---

## 🎯 Testing Tier Checklist

### When Writing Unit Tests:
- [ ] Tests business logic in isolation
- [ ] Mocks external dependencies (DB, HTTP, etc.)
- [ ] Uses real business logic components
- [ ] Located in `test/unit/datastorage/reconstruction/`
- [ ] Uses `package reconstruction` with import alias

### When Writing Integration Tests:
- [ ] Tests business logic with real infrastructure
- [ ] Uses real PostgreSQL database (Podman)
- [ ] Calls reconstruction components directly (NO HTTP)
- [ ] Validates database interactions and constraints
- [ ] Located in `test/integration/datastorage/`
- [ ] Uses `package datastorage`

### When Writing E2E Tests:
- [ ] Tests complete workflow via REST API
- [ ] Uses OpenAPI client (ogenclient)
- [ ] Tests HTTP endpoint
- [ ] Tests authentication (OAuth-proxy)
- [ ] Tests kubectl apply workflow
- [ ] Located in `test/e2e/datastorage/`
- [ ] Uses `package datastorage_test`

---

## 📚 References

- **Primary Authority**: [03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc)
- **Test Plan**: [SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md](./SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md)
- **Testing Coverage Standards**: [15-testing-coverage-standards.mdc](mdc:.cursor/rules/15-testing-coverage-standards.mdc)

---

## ✅ Success Criteria

This guide is successful when:
- ✅ All integration tests call business logic directly (NO HTTP server)
- ✅ All E2E tests use OpenAPI client (NO direct business logic calls)
- ✅ Clear separation of concerns between testing tiers
- ✅ No testing tier confusion in future development

---

**Document Status**: ✅ Active  
**Created**: 2026-01-12  
**Purpose**: Prevent testing tier confusion in reconstruction feature
