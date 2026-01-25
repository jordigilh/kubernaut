# Migration to Ogen for DataStorage OpenAPI Client

**Date**: January 8, 2026
**Status**: 📋 **PROPOSED** - Migration plan for eliminating unstructured data
**Confidence**: **95%** - Ogen already in use, proven technology

---

## 🎯 **Problem Statement**

**Current State**:
- DataStorage client uses `oapi-codegen` which generates `json.RawMessage` for `oneOf` types
- Requires manual marshaling/unmarshaling in helpers: `pkg/audit/helpers.go`
- Python client requires manual conversion in `buffered_store.py` lines 434-435
- `event_data` treated as unstructured data despite having 37 typed schemas

**Desired State**:
- Use `ogen` (already in codebase) which generates proper Go interfaces for `oneOf`
- Direct struct assignment without marshaling: `event.SetEventData(&LLMRequestPayload{...})`
- Python uses Pydantic models directly (already supported)
- **ZERO unstructured data** in both Go and Python

---

## ✅ **Why Ogen**

### Already in Codebase
```bash
# From go.mod
github.com/ogen-go/ogen v1.18.0

# From Makefile
generate-holmesgpt-client: ogen ## Generate HolmesGPT-API client from OpenAPI spec
```

**Proven**: Already generates HolmesGPT-API client successfully
**Location**: `pkg/clients/holmesgpt/oas_*_gen.go`

### Better oneOf Support

**`oapi-codegen` (current)**:
```go
type AuditEventRequest_EventData struct {
    union json.RawMessage  // ❌ Requires manual marshaling
}

// Usage
jsonBytes, _ := json.Marshal(payload)
event.EventData = AuditEventRequest_EventData{union: jsonBytes}
```

**`ogen` (proposed)**:
```go
type AuditEventRequest_EventData interface {
    isAuditEventRequest_EventData()
}

type LLMRequestPayload struct { ... }
func (LLMRequestPayload) isAuditEventRequest_EventData() {}

// Usage
event.SetEventData(&LLMRequestPayload{...})  // ✅ Direct assignment!
```

---

## 📋 **Migration Steps**

### Step 1: Create DataStorage Client Package Structure ✅

```bash
mkdir -p pkg/datastorage/ogen-client
```

### Step 2: Add ogen Generation Config ✅

**File**: `pkg/datastorage/ogen-client/gen.go`
```go
package ogenclient

//go:generate go run github.com/ogen-go/ogen/cmd/ogen@v1.18.0 \
//  --target . \
//  --clean \
//  ../../../api/openapi/data-storage-v1.yaml
```

### Step 3: Update Makefile ✅

```makefile
.PHONY: generate-datastorage-client
generate-datastorage-client: ogen ## Generate DataStorage OpenAPI client from spec (DD-API-001)
	@echo "📋 Generating DataStorage clients (Go + Python) from api/openapi/data-storage-v1.yaml..."
	@echo ""
	@echo "🔧 [1/2] Generating Go client with ogen..."
	@go generate ./pkg/datastorage/ogen-client/...
	@echo "✅ Go client generated: pkg/datastorage/ogen-client/oas_*_gen.go"
	@echo ""
	@echo "🔧 [2/2] Generating Python client..."
	@rm -rf holmesgpt-api/src/clients/datastorage
	@podman run --rm -v "$(PWD)":/local:z openapitools/openapi-generator-cli:v7.2.0 generate \
		-i /local/api/openapi/data-storage-v1.yaml \
		-g python \
		-o /local/holmesgpt-api/src/clients/datastorage \
		--package-name datastorage \
		--additional-properties=packageVersion=1.0.0
	@echo "✅ Python client generated: holmesgpt-api/src/clients/datastorage/"
```

### Step 4: Regenerate Client ✅

```bash
make generate-datastorage-client
```

### Step 5: Update Import Paths ✅

**Find and replace across codebase**:
```bash
# Old import
github.com/jordigilh/kubernaut/pkg/datastorage/client

# New import
github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client
```

**Files to update** (~20-30 files):
- All service audit managers
- Integration tests
- Helper functions

### Step 6: Refactor Helper Functions ✅

**File**: `pkg/audit/helpers.go`

**Before** (with oapi-codegen):
```go
func SetEventData(e *dsgen.AuditEventRequest, data interface{}) {
    jsonBytes, err := json.Marshal(data)
    if err != nil {
        return
    }
    e.EventData = dsgen.AuditEventRequest_EventData{union: jsonBytes}
}
```

**After** (with ogen):
```go
func SetEventData(e *ogenclient.AuditEventRequest, data ogenclient.AuditEventRequest_EventData) {
    e.SetEventData(data)  // ✅ Direct setter - no marshaling!
}

// Or even better - eliminate the helper entirely:
// event.SetEventData(&ogenclient.LLMRequestPayload{...})
```

### Step 7: Update Service Audit Code ✅

**Example**: `pkg/workflowexecution/audit/manager.go`

**Before**:
```go
payload := &workflowexecution.WorkflowExecutionAuditPayload{...}
audit.SetEventData(event, payload)  // Marshals to json.RawMessage
```

**After**:
```go
payload := &ogenclient.WorkflowExecutionAuditPayload{...}
event.SetEventData(payload)  // ✅ Direct assignment via interface!
```

### Step 8: Test Go Changes ✅

```bash
# Run all integration tests
make test-integration

# Expected: All tests pass, no changes in behavior
```

### Step 9: Delete Old Client ✅

```bash
rm -rf pkg/datastorage/client/
```

---

## 📊 **Impact Analysis**

### Go Codebase

**Files Affected**: ~30 files
- `pkg/*/audit/manager.go` - All service audit managers (8 files)
- `pkg/audit/helpers.go` - Central helper functions (1 file)
- `test/integration/*/audit_*.go` - Integration tests (~15 files)
- `test/unit/*/audit_*.go` - Unit tests (~6 files)

**Changes Required**:
1. Update imports: `client` → `ogen-client`
2. Update type references: `dsgen.Type` → `ogenclient.Type`
3. Simplify `SetEventData` calls (remove marshaling)
4. Use ogen's generated interface types

**Confidence**: 95% - Mechanical refactoring, no logic changes

---

### Python Codebase

**Files Affected**: 3 files
- `holmesgpt-api/src/audit/events.py` - Event creation (5 functions)
- `holmesgpt-api/src/audit/buffered_store.py` - Buffering logic (1 file)
- `holmesgpt-api/src/models/audit_models.py` - Type imports (1 file)

**Changes Required**:
1. Return `AuditEventRequest` (Pydantic model) instead of `Dict[str, Any]`
2. Remove lines 434-435 in `buffered_store.py` (no conversion needed)
3. Update `store_audit()` signature to accept `AuditEventRequest`

**Confidence**: 90% - Requires refactoring dict-based code to Pydantic models

---

## 🎯 **Benefits**

### For Go

| Aspect | Before (oapi-codegen) | After (ogen) |
|--------|----------------------|--------------|
| **EventData Type** | `json.RawMessage` | `interface` with typed implementations |
| **Assignment** | Manual marshal/unmarshal | Direct struct assignment |
| **Type Safety** | Runtime (JSON validation) | Compile-time (Go type system) |
| **Helper Complexity** | Marshal→assign→unmarshal | Direct setter method |
| **Code Lines** | `pkg/audit/helpers.go` ~50 lines | ~10 lines (or eliminate entirely) |

### For Python

| Aspect | Before | After |
|--------|--------|-------|
| **Event Creation** | Returns `Dict[str, Any]` | Returns `AuditEventRequest` (Pydantic) |
| **Buffering** | Manual conversion (lines 434-435) | Direct Pydantic model usage |
| **Type Safety** | Runtime (dict access) | Compile-time (Pydantic validation) |
| **IDE Support** | No autocomplete for dicts | Full autocomplete for Pydantic models |

---

## ⚠️ **Risks & Mitigation**

### Risk 1: Ogen Generated Code Different from oapi-codegen

**Risk**: Ogen may generate different struct field names or types

**Mitigation**:
- Run `make generate-datastorage-client` and inspect `git diff`
- Update type references systematically
- Run full test suite before committing

**Confidence**: 90% - Ogen follows OpenAPI spec closely

---

### Risk 2: Integration Test Failures

**Risk**: Tests may fail due to type mismatches or missing imports

**Mitigation**:
- Fix compilation errors first (imports, types)
- Run integration tests service-by-service
- Use `git bisect` if regressions occur

**Confidence**: 95% - Tests use well-defined interfaces

---

### Risk 3: Python Client Behavior Change

**Risk**: Python generator may handle `oneOf` differently

**Mitigation**:
- Python generator unchanged (still `openapi-generator`)
- Only Python business code changes (dicts → Pydantic)
- HAPI integration tests validate behavior

**Confidence**: 95% - Python generator not affected by Go changes

---

## 📅 **Implementation Timeline**

| Step | Duration | Dependencies |
|------|----------|--------------|
| **1. Setup ogen package** | 10 min | None |
| **2. Generate client** | 5 min | Step 1 |
| **3. Update imports** | 20 min | Step 2 |
| **4. Refactor helpers** | 30 min | Step 3 |
| **5. Update service audit code** | 60 min | Step 4 |
| **6. Fix Go tests** | 30 min | Step 5 |
| **7. Refactor Python code** | 45 min | Step 2 |
| **8. Fix Python tests** | 30 min | Step 7 |
| **9. Full test validation** | 60 min | Steps 6, 8 |
| **10. Cleanup & docs** | 30 min | Step 9 |

**Total Estimated Time**: **5 hours**

---

## ✅ **Success Criteria**

- [ ] Ogen generates Go client from DataStorage OpenAPI spec
- [ ] All Go code uses ogen-generated types (no `json.RawMessage` for event_data)
- [ ] `pkg/audit/helpers.go` simplified or eliminated
- [ ] All Go integration tests pass (no regressions)
- [ ] Python code uses Pydantic models (no `Dict[str, Any]`)
- [ ] All HAPI integration tests pass (65/65)
- [ ] No unstructured data (`interface{}` or `Dict`) in audit event handling
- [ ] Build time not significantly increased
- [ ] Documentation updated

---

## 🎯 **Confidence Assessment**

| Metric | Confidence | Reasoning |
|--------|-----------|-----------|
| **Ogen Compatibility** | 95% | Already in use for HolmesGPT-API |
| **Go Migration** | 95% | Mechanical refactoring, well-defined interfaces |
| **Python Migration** | 90% | Requires dict→Pydantic refactoring |
| **Test Compatibility** | 90% | Some test updates needed |
| **Overall Success** | 93% | High confidence - proven technology, clear path |

---

## 📝 **Next Steps**

**Immediate Actions**:
1. ✅ Get user approval for ogen migration
2. ⏳ Create `pkg/datastorage/ogen-client/gen.go`
3. ⏳ Update Makefile to use ogen for DataStorage
4. ⏳ Generate client and inspect output
5. ⏳ Begin systematic import path updates

**Expected Outcome**: **ZERO unstructured data** in both Go and Python audit event handling, with significantly simpler code.

