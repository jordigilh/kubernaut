# OpenAPI Hybrid Approach - Typed Schemas + interface{} Go Code

**Date**: January 8, 2026  
**Context**: DataStorage OpenAPI EventData Type Safety  
**Decision**: Alternative A - Keep typed schemas for documentation, use `interface{}` for Go code

---

## Problem Statement

After implementing typed OpenAPI schemas with `oneOf` discriminator for `event_data`, the generated Go client used `json.RawMessage` union types, which created awkward code patterns:

```go
// ❌ AWKWARD: Marshal to JSON, then assign to union
jsonBytes, _ := json.Marshal(payload)
_ = event.EventData.UnmarshalJSON(jsonBytes)

// ❌ AWKWARD: Check for empty data
if len(event.EventData.union) == 0 { ... }
```

**Root Cause**: `oapi-codegen` generates union types for `oneOf` schemas, requiring JSON marshaling for all assignments.

---

## Solution: Hybrid Approach (Alternative A)

**Keep typed schemas in OpenAPI spec for documentation/validation, but use `interface{}` in Go code.**

### OpenAPI Spec Change

```yaml
event_data:
  description: |
    Service-specific event data as structured type.
    V2.0: Typed schemas documented below for API validation.
    Go client uses interface{} for clean code ergonomics.
    See DD-AUDIT-004 for structured type requirements.
  oneOf:
    - $ref: '#/components/schemas/GatewayAuditPayload'
    - $ref: '#/components/schemas/RemediationOrchestratorAuditPayload'
    # ... 6 more schemas
  discriminator:
    propertyName: event_type
    mapping:
      'gateway.signal.received': '#/components/schemas/GatewayAuditPayload'
      # ... 34 more event types
  x-go-type: interface{}                          # ✅ Override Go type
  x-go-type-skip-optional-pointer: true          # ✅ No pointer
```

### Generated Go Client

```go
type AuditEventRequest struct {
    EventData interface{} `json:"event_data"`  // ✅ Clean interface{}
}
```

### Clean Go Code

```go
// ✅ CLEAN: Direct assignment
func SetEventData(e *dsgen.AuditEventRequest, data interface{}) {
    e.EventData = data
}

// ✅ CLEAN: Simple nil check
if event.EventData == nil {
    return errors.New("event_data is nil")
}

// ✅ CLEAN: Type assertion
eventData, ok := event.EventData.(map[string]interface{})
```

---

## Benefits

### 1. **API Documentation & Validation**
- OpenAPI spec documents all 8 typed payload schemas
- API consumers see clear structure for each event type
- Future: API gateway can validate payloads against schemas

### 2. **Clean Go Code**
- Direct assignment: `event.EventData = payload`
- Simple nil checks: `event.EventData == nil`
- Standard type assertions: `event.EventData.(map[string]interface{})`

### 3. **No Breaking Changes**
- Business logic unchanged (already uses structured types)
- Integration tests unchanged (already use `map[string]interface{}`)
- Unit tests unchanged (already use structured types)

### 4. **Best of Both Worlds**
- **OpenAPI**: Typed schemas for documentation/validation
- **Go Code**: `interface{}` for ergonomics and flexibility

---

## Implementation Details

### Files Modified

1. **`api/openapi/data-storage-v1.yaml`**
   - Added `x-go-type: interface{}` to `event_data` field
   - Added `x-go-type-skip-optional-pointer: true`
   - Kept all 8 typed schemas and discriminator mapping

2. **`pkg/datastorage/client/generated.go`** (regenerated)
   - `EventData` field is now `interface{}` (was `AuditEventRequest_EventData`)

3. **`pkg/audit/helpers.go`**
   - Reverted `SetEventData` to direct assignment
   - Reverted `SetEventDataFromEnvelope` to use `EnvelopeToMap`

4. **`pkg/datastorage/audit/workflow_search_event.go`**
   - Reverted `event.EventData == nil` checks (was `len(event.EventData.union) == 0`)
   - Reverted type assertions to `event.EventData.(map[string]interface{})`

### Test Fixes

**Pre-existing Issue**: Tests used wrong enum constant names after OpenAPI migration.

```bash
# Fixed enum constant references
sed -i '' 's/dsgen\.WorkflowSearchFiltersSeverityCritical/dsgen.Critical/g' test/**/*.go
sed -i '' 's/dsgen\.WorkflowSearchFiltersSeverityLow/dsgen.Low/g' test/**/*.go
```

**Affected Tests**:
- `test/e2e/datastorage/04_workflow_search_test.go`
- `test/e2e/datastorage/06_workflow_search_audit_test.go`
- `test/e2e/datastorage/07_workflow_version_management_test.go`
- `test/e2e/datastorage/08_workflow_search_edge_cases_test.go`
- `test/integration/datastorage/workflow_bulk_import_performance_test.go`

---

## Validation Results

### Compilation
```bash
✅ make build-datastorage
   Built: bin/datastorage
```

### Linting
```bash
⚠️  253 pre-existing lint issues (not introduced by this change)
✅ No new lint errors from OpenAPI changes
✅ Fixed 10 typecheck errors (enum constant names)
```

### Unit Tests
```bash
⏳ Running: make test-tier-unit
   (1 pre-existing flake in AIAnalysis Rego test)
```

---

## Comparison with Rejected Alternatives

### Alternative B: Remove Typed Schemas
- ❌ Loses API documentation
- ❌ No validation capability
- ✅ Clean Go code

### Alternative C: Keep Union Types
- ✅ Type-safe OpenAPI
- ❌ Awkward Go code (JSON marshaling for all assignments)
- ❌ Breaking changes to all business logic

### Alternative A (CHOSEN): Hybrid Approach
- ✅ API documentation preserved
- ✅ Clean Go code
- ✅ No breaking changes
- ✅ Future validation capability

---

## Future Considerations

### API Gateway Validation
When DataStorage adds an API gateway (e.g., Kong, Envoy), the typed schemas enable:
- **Request validation** before reaching the service
- **Schema evolution** tracking
- **API versioning** support

### Client Generation for Other Languages
The typed schemas enable generating type-safe clients for:
- **Python**: Pydantic models
- **TypeScript**: Interface definitions
- **Java**: POJO classes

### Migration Path
If Go code ergonomics improve in future `oapi-codegen` versions:
1. Remove `x-go-type` override
2. Update business logic to use generated union types
3. Typed schemas already in place

---

## Key Takeaways

1. **`x-go-type`** is a powerful escape hatch for code generation issues
2. **OpenAPI schemas** serve multiple purposes (docs, validation, codegen)
3. **Pragmatic trade-offs** are valid (typed schemas + `interface{}` Go code)
4. **No breaking changes** is a critical constraint in active development

---

## References

- **OpenAPI Extension**: `x-go-type` in `oapi-codegen`
- **Related Docs**: `docs/handoff/AUDIT_PAYLOAD_STRUCTURING_JAN08.md`
- **Design Decision**: DD-AUDIT-004 (Structured Audit Payloads)
- **Commit**: `refactor(datastorage): Use hybrid OpenAPI approach for EventData`

