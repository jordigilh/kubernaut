# DD-HAPI-003: Mandatory OpenAPI Client Usage

## Status
**✅ Approved** (2025-12-29)
**Last Reviewed**: 2025-12-29
**Confidence**: 95%
**Priority**: P0 - BLOCKER

## Context & Problem

**Problem**: The AIAnalysis service was using a manual HTTP client wrapper (`pkg/holmesgpt/client/holmesgpt.go`) to call HolmesGPT-API endpoints, instead of using the auto-generated OpenAPI client. This caused:

1. **E2E Test Failures**: HTTP 500 errors in E2E tests due to request format mismatches
2. **Type Safety Violations**: Manual JSON marshaling bypassed compile-time type checking
3. **Contract Drift**: No guarantee that requests match the HAPI OpenAPI specification
4. **Maintenance Burden**: Manual updates required when HAPI API changes

**Key Requirements**:
- **BR-AI-006**: HolmesGPT-API integration with type-safe requests/responses
- **ADR-031**: OpenAPI Specification Standard for REST APIs
- **DD-AUDIT-004**: Type-safe audit event data structures

**Root Cause**: The manual HTTP client was created before the OpenAPI client generation tooling was established, and was never migrated.

## Alternatives Considered

### Alternative 1: Continue Using Manual HTTP Client (Status Quo)
**Approach**: Keep the manual HTTP client wrapper in `pkg/holmesgpt/client/holmesgpt.go`

**Pros**:
- ✅ Already implemented
- ✅ Simple, easy to understand code
- ✅ No migration effort required

**Cons**:
- ❌ **BLOCKER**: Causing E2E test failures (HTTP 500 errors)
- ❌ No compile-time type safety
- ❌ Request format can drift from HAPI OpenAPI spec
- ❌ Violates ADR-031 (OpenAPI Standard)
- ❌ Requires manual updates when HAPI API changes
- ❌ Inconsistent with Data Storage client (which uses generated OpenAPI)

**Confidence**: 0% (rejected - causes production failures)

---

### Alternative 2: Use Generated OpenAPI Client (RECOMMENDED)
**Approach**: Migrate all HAPI client code to use the auto-generated OpenAPI client from `pkg/holmesgpt/client/oas_*_gen.go`

**Pros**:
- ✅ **Compile-time type safety** - Invalid requests caught at build time
- ✅ **Contract compliance** - Guaranteed to match HAPI OpenAPI spec
- ✅ **Auto-regeneration** - Run `go generate` when HAPI spec changes
- ✅ **Better error handling** - OpenAPI-defined error types (422 validation errors)
- ✅ **Consistent with Data Storage** - Same pattern across all OpenAPI services
- ✅ **Fixes E2E failures** - Proper request formatting resolves HTTP 500 errors
- ✅ **ADR-031 compliance** - Aligns with architectural standard

**Cons**:
- ⚠️ **One-time migration effort** - Update production code and tests (~2-3 hours)
- ⚠️ **Generated code complexity** - Generated code is verbose (but machine-maintained)
- ⚠️ **Response type assertions** - Must type-assert interface responses to concrete types

**Confidence**: 95% (approved)

---

### Alternative 3: Dual Client Support (Manual + OpenAPI)
**Approach**: Support both manual and generated clients, gradually migrate

**Pros**:
- ✅ Incremental migration possible
- ✅ Backward compatibility during transition

**Cons**:
- ❌ Confusing for developers - which client should I use?
- ❌ Maintenance burden - two codepaths to maintain
- ❌ Test complexity - must test both client types
- ❌ Technical debt - manual client eventually removed anyway

**Confidence**: 10% (rejected - adds complexity without benefit)

---

## Decision

**APPROVED: Alternative 2** - Use Generated OpenAPI Client (Mandatory)

**Rationale**:
1. **Fixes E2E Test Failures**: The HTTP 500 errors in E2E tests are caused by request format mismatches between the manual client and HAPI's expectations. Using the generated OpenAPI client guarantees contract compliance.

2. **Type Safety is Non-Negotiable**: Per ADR-031 and DD-AUDIT-004, type safety is a core architectural principle. Manual HTTP clients bypass compile-time validation and introduce runtime errors.

3. **Consistency Across Services**: The Data Storage service already uses a generated OpenAPI client. HAPI client should follow the same pattern for consistency.

4. **Architectural Compliance**: ADR-031 mandates OpenAPI specification for REST APIs. Using the generated client enforces this standard.

**Key Insight**: The manual HTTP client was a technical debt artifact from early development. The generated OpenAPI client is production-ready and provides superior type safety, contract compliance, and maintainability.

## Implementation

### Primary Implementation Files

**Production Code**:
- `pkg/holmesgpt/client/holmesgpt.go` - Migrate to wrapper around generated `oas_client_gen.go`
- `pkg/holmesgpt/client/oas_client_gen.go` - Auto-generated OpenAPI client (ogen)
- `pkg/holmesgpt/client/oas_schemas_gen.go` - Auto-generated types
- `pkg/holmesgpt/client/oas_interfaces_gen.go` - Response type interfaces

**Test Code**:
- `test/unit/aianalysis/investigating_handler_test.go` - Mock generated client
- `test/integration/aianalysis/audit_flow_integration_test.go` - Use generated client
- `test/integration/aianalysis/recovery_integration_test.go` - Use generated client
- `test/e2e/aianalysis/*.go` - Use generated client

**Validation**:
- `scripts/validate-openapi-client-usage.sh` - NEW: Linter script to detect manual HTTP clients

### Migration Pattern

**Before (Manual Client)**:
```go
// FORBIDDEN: Manual HTTP client
func (c *HolmesGPTClient) Investigate(ctx context.Context, req *IncidentRequest) (*IncidentResponse, error) {
    body, err := json.Marshal(req)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal request: %w", err)
    }
    
    url := fmt.Sprintf("%s/api/v1/incident/analyze", c.baseURL)
    httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
    // ... manual HTTP handling
}
```

**After (OpenAPI Client)**:
```go
// ✅ CORRECT: Generated OpenAPI client
func (c *HolmesGPTClient) Investigate(ctx context.Context, req *IncidentRequest) (*IncidentResponse, error) {
    // Use generated client method (compile-time type safety)
    res, err := c.client.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("HolmesGPT-API call failed: %w", err)
    }
    
    // Type-assert response interface to concrete type
    response, ok := res.(*IncidentResponse)
    if !ok {
        return nil, fmt.Errorf("unexpected response type: %T", res)
    }
    
    return response, nil
}
```

### Data Flow

1. **Client Initialization**: `NewHolmesGPTClient(baseURL)` creates wrapper around generated client
2. **Request Construction**: Business code creates typed `*IncidentRequest` struct
3. **API Call**: Wrapper delegates to `client.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(ctx, req)`
4. **Response Handling**: Type-assert interface response to `*IncidentResponse`
5. **Error Handling**: Generated client returns `*HTTPValidationError` for 422 responses

### Validation Script

**File**: `scripts/validate-openapi-client-usage.sh`

```bash
#!/bin/bash
# Validate that all HAPI client code uses generated OpenAPI client

echo "🔍 Validating OpenAPI client usage..."

# Check for forbidden manual HTTP client patterns
VIOLATIONS=$(grep -r "http.NewRequestWithContext.*holmesgpt\|json.Marshal.*IncidentRequest" \
    pkg/aianalysis \
    internal/controller/aianalysis \
    test/unit/aianalysis \
    test/integration/aianalysis \
    test/e2e/aianalysis \
    --include="*.go" \
    --exclude="*_gen.go" \
    --exclude="holmesgpt.go" | wc -l)

if [ "$VIOLATIONS" -gt 0 ]; then
    echo "❌ VIOLATION: Found manual HTTP client usage for HAPI"
    echo "   All HAPI clients MUST use generated OpenAPI client (DD-HAPI-003)"
    grep -r "http.NewRequestWithContext.*holmesgpt\|json.Marshal.*IncidentRequest" \
        pkg/aianalysis \
        internal/controller/aianalysis \
        test/unit/aianalysis \
        test/integration/aianalysis \
        test/e2e/aianalysis \
        --include="*.go" \
        --exclude="*_gen.go" \
        --exclude="holmesgpt.go"
    exit 1
fi

echo "✅ All HAPI client code uses generated OpenAPI client"
```

## Consequences

### Positive
- ✅ **E2E Tests Pass**: Fixes HTTP 500 errors by ensuring request format compliance
- ✅ **Compile-Time Type Safety**: Invalid requests caught at build time, not runtime
- ✅ **Contract Compliance**: Guaranteed to match HAPI OpenAPI specification
- ✅ **Automated Regeneration**: `go generate` updates client when HAPI spec changes
- ✅ **Better Error Handling**: OpenAPI-defined error types (422 validation errors)
- ✅ **Consistent Architecture**: Same pattern as Data Storage OpenAPI client
- ✅ **Reduced Maintenance**: No manual request/response handling code

### Negative
- ⚠️ **Migration Effort**: One-time ~2-3 hour effort to update production code and tests
  - **Mitigation**: Clear migration pattern, single Pull Request
- ⚠️ **Generated Code Complexity**: Generated code is verbose and harder to read
  - **Mitigation**: Generated code is machine-maintained, developers only interact with wrapper
- ⚠️ **Response Type Assertions**: Must type-assert interface responses to concrete types
  - **Mitigation**: Add helper methods in wrapper to handle assertions

### Neutral
- 🔄 **Build Dependency**: Requires `ogen` tool for regeneration (`go install github.com/ogen-go/ogen/cmd/ogen@latest`)
- 🔄 **Testing Pattern**: Tests must use generated client types for mocking

## Validation Results

**Confidence Assessment Progression**:
- Initial assessment: 85% confidence (before E2E test failures)
- After E2E failure analysis: 95% confidence (root cause identified)
- After migration: 100% confidence (tests passing, type safety enforced)

**Key Validation Points**:
- ✅ **E2E Tests Pass**: HTTP 500 errors resolved
- ✅ **Build Success**: No compilation errors with generated client
- ✅ **Type Safety**: Compile-time validation of request/response structures
- ✅ **Contract Compliance**: Requests match HAPI OpenAPI spec exactly
- ✅ **Test Coverage**: Unit, integration, and E2E tests all use generated client

## Related Decisions
- **Supports**: ADR-031 (OpenAPI Specification Standard for REST APIs)
- **Supports**: DD-AUDIT-004 (Type-Safe Audit Event Data)
- **Consistent With**: Data Storage OpenAPI client pattern
- **Supersedes**: Manual HTTP client in `pkg/holmesgpt/client/holmesgpt.go`

## Review & Evolution

**When to Revisit**:
- If HAPI migrates to gRPC (unlikely - ADR-031 mandates OpenAPI)
- If ogen generator is deprecated (monitor community support)
- If generated code size becomes a build performance issue

**Success Metrics**:
- **E2E Test Pass Rate**: Target 100% (resolved HTTP 500 errors)
- **Type Safety Violations**: Target 0 (compile-time enforcement)
- **HAPI API Changes**: Auto-regeneration with `go generate` (< 5 minutes)
- **Developer Onboarding**: Clear pattern for HAPI client usage

---

## Enforcement

### Linter Integration
Add to `.golangci.yml`:
```yaml
linters-settings:
  custom:
    holmesgpt-openapi-client:
      description: "Enforce generated OpenAPI client usage for HolmesGPT-API"
      path: scripts/validate-openapi-client-usage.sh
```

### CI Pipeline
Add to GitHub Actions `.github/workflows/ci.yml`:
```yaml
- name: Validate OpenAPI Client Usage
  run: ./scripts/validate-openapi-client-usage.sh
```

### Code Review Checklist
- [ ] All HAPI client code uses generated OpenAPI client
- [ ] No manual `http.NewRequestWithContext` calls to HolmesGPT-API
- [ ] No manual `json.Marshal` of HAPI request types
- [ ] Tests use generated client types for mocking

---

**PRIORITY**: P0 - BLOCKER
**ENFORCEMENT**: MANDATORY - All HAPI client code MUST use generated OpenAPI client
**VALIDATION**: Automated via `scripts/validate-openapi-client-usage.sh`





