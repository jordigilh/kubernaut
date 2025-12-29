# DataStorage V1.0: Workflow Labels Structured Types - IN PROGRESS

**Date**: December 17, 2025
**Status**: 🔄 **IN PROGRESS** - Phase 1 Complete (Models), Phase 2 Starting (Repository/Usage)
**Confidence**: 85%

---

## 🎯 **Objective**

**Goal**: Eliminate ALL unstructured data (`json.RawMessage`, `map[string]interface{}`) from workflow labels

**Scope**: V1.0 includes **all 3 label types**:
1. **MandatoryLabels** - 5 required fields (signal_type, severity, component, environment, priority)
2. **CustomLabels** - Subdomain-based map[string][]string
3. **DetectedLabels** - 8 auto-detected boolean/string fields

**Authority**: DD-WORKFLOW-001 v2.3 (Mandatory Label Schema)

---

## ✅ **Phase 1: Models - COMPLETE** (3/3 tasks)

### 1. Created Structured Label Types ✅

**File**: `pkg/datastorage/models/workflow_labels.go` (NEW)

**Types Created**:
```go
// MandatoryLabels - 5 required fields
type MandatoryLabels struct {
    SignalType  string `json:"signal_type" validate:"required"`
    Severity    string `json:"severity" validate:"required,oneof=critical high medium low"`
    Component   string `json:"component" validate:"required"`
    Environment string `json:"environment" validate:"required"`
    Priority    string `json:"priority" validate:"required"`
}

// CustomLabels - Subdomain-based extraction
type CustomLabels map[string][]string

// DetectedLabels - 8 auto-detected fields
type DetectedLabels struct {
    FailedDetections []string `json:"failedDetections,omitempty"`
    GitOpsManaged    bool     `json:"gitOpsManaged,omitempty"`
    GitOpsTool       string   `json:"gitOpsTool,omitempty"`
    PDBProtected     bool     `json:"pdbProtected,omitempty"`
    HPAEnabled       bool     `json:"hpaEnabled,omitempty"`
    Stateful         bool     `json:"stateful,omitempty"`
    HelmManaged      bool     `json:"helmManaged,omitempty"`
    NetworkIsolated  bool     `json:"networkIsolated,omitempty"`
    ServiceMesh      string   `json:"serviceMesh,omitempty"`
}
```

**Helper Functions**: NewMandatoryLabels(), NewCustomLabels(), NewDetectedLabels(), IsEmpty()
**Validation**: ValidDetectedLabelFields variable for FailedDetections validation

---

### 2. Updated RemediationWorkflow Struct ✅

**File**: `pkg/datastorage/models/workflow.go`

**Before V1.0** - Unstructured:
```go
Labels         json.RawMessage `json:"labels" db:"labels"`
CustomLabels   json.RawMessage `json:"custom_labels,omitempty" db:"custom_labels"`
DetectedLabels json.RawMessage `json:"detected_labels,omitempty" db:"detected_labels"`
```

**After V1.0** - Structured:
```go
Labels         MandatoryLabels  `json:"labels" db:"labels" validate:"required"`
CustomLabels   CustomLabels     `json:"custom_labels,omitempty" db:"custom_labels"`
DetectedLabels *DetectedLabels  `json:"detected_labels,omitempty" db:"detected_labels"`
```

---

### 3. Removed Unstructured Helper Methods ✅

**Removed**:
- `GetLabelsMap() (map[string]interface{}, error)` - Line 490-497
- `SetLabelsFromMap(labels map[string]interface{}) error` - Line 499-507
- Old `DetectedLabels` type definition (with pointer types) - Lines 240-298
- `ValidFailedDetectionFields` constants - Lines 305-339

**Reason**: With structured types, direct field access is preferred:
- Old: `map["signal_type"]`
- New: `w.Labels.SignalType`

---

## 🔄 **Phase 2: Repository/Usage - IN PROGRESS** (0/5 tasks)

### Remaining Tasks:

**4. Update Workflow Repository** ⏳ NEXT
- File: `pkg/datastorage/repository/workflow/*.go`
- Scan/marshaling logic for structured types
- Database JSONB → struct conversion

**5. Update Audit Events** ⏳ PENDING
- Files using workflow labels in audit event data
- Ensure structured types used in audit payloads

**6. Update Workflow Search** ⏳ PENDING
- File: `pkg/datastorage/models/workflow.go` (WorkflowSearchRequest)
- Label filtering with structured types
- Query building with type-safe access

**7. Update OpenAPI Spec** ⏳ PENDING
- File: `api/openapi/data-storage-v1.yaml`
- Define structured schemas for labels
- Regenerate Go/Python clients

**8. Verify Compilation & Tests** ⏳ PENDING
- Build all datastorage packages
- Run unit + integration tests
- Verify zero regressions

---

## 📊 **Progress Summary**

| Phase | Tasks Complete | Status |
|-------|---------------|--------|
| **Phase 1: Models** | 3/3 (100%) | ✅ **COMPLETE** |
| **Phase 2: Repository/Usage** | 0/5 (0%) | ⏳ **IN PROGRESS** |
| **Overall** | 3/8 (38%) | 🔄 **IN PROGRESS** |

---

## 🔧 **Technical Achievements**

### Type Safety Improvements

| Aspect | Before | After |
|--------|--------|-------|
| **Mandatory Labels** | `json.RawMessage` | `MandatoryLabels` struct |
| **Custom Labels** | `json.RawMessage` | `map[string][]string` |
| **Detected Labels** | `json.RawMessage` | `*DetectedLabels` struct |
| **Field Access** | `map["key"]` (runtime) | `.Field` (compile-time) |
| **Validation** | Runtime JSON parsing | Compile-time + `validate` tags |

### Code Quality

- **Type Safety**: ✅ Compile-time validation for all label fields
- **Documentation**: ✅ Comprehensive DD-WORKFLOW-001 v2.3 compliance
- **Helper Functions**: ✅ Constructors and utility methods
- **Validation**: ✅ `go-playground/validator` integration

---

## ⚠️ **Known Dependencies - Must Update**

Based on search results, these areas use workflow labels and need updates:

### High Priority (V1.0 Blockers):
1. **Workflow Repository** - Database scan/marshal operations
2. **Workflow Search** - Label filtering and query building
3. **OpenAPI Spec** - API contract definition
4. **Audit Events** - Workflow label serialization in audit traces

### Files Likely Affected:
```
pkg/datastorage/repository/workflow/*.go
pkg/datastorage/server/*_handlers.go
api/openapi/data-storage-v1.yaml
pkg/datastorage/client/generated.go (after regeneration)
holmesgpt-api/src/clients/datastorage/*.py (after regeneration)
```

---

## 📋 **Next Steps**

### Immediate (Phase 2):
1. ✅ Search for `json.Unmarshal.*Labels` usages → Update to structured types
2. ✅ Search for `GetLabelsMap()`/`SetLabelsFromMap()` calls → Use direct access
3. ✅ Update workflow repository scan/marshal logic
4. ✅ Update OpenAPI spec with structured schemas
5. ✅ Regenerate clients (Go + Python)
6. ✅ Run full test suite

### Validation:
```bash
# Find label usage patterns
grep -r "json.Unmarshal.*Labels" pkg/datastorage/
grep -r "GetLabelsMap\|SetLabelsFromMap" pkg/datastorage/
grep -r "workflow\.Labels" pkg/datastorage/ --include="*.go"

# Build + test
go build ./pkg/datastorage/...
go test ./pkg/datastorage/...
```

---

## 🎯 **Success Criteria**

| Criterion | Target | Current | Status |
|-----------|--------|---------|--------|
| **Zero `json.RawMessage` for labels** | 0 usages | 0 in models | ✅ Models |
| **Zero `map[string]interface{}` for labels** | 0 usages | Unknown | ⏳ TBD |
| **Type-safe field access** | 100% | 100% in models | ✅ Models |
| **Build success** | Pass | Pass (models only) | ⏳ Full build TBD |
| **Test pass rate** | 100% | TBD | ⏳ Pending |

---

## 🏆 **V1.0 Impact**

**When Complete**:
- ✅ Zero unstructured data in workflow labels
- ✅ Compile-time type safety for all label access
- ✅ DD-WORKFLOW-001 v2.3 full compliance
- ✅ V1.0 zero technical debt mandate achieved

**Final V1.0 Blocker Status**:
- ✅ V2.2 Audit Pattern: Complete (6/6 services)
- ✅ DB Adapter Structured Types: Complete
- ⏳ **Workflow Labels**: **IN PROGRESS** (38% complete)

---

**Authority**: DD-WORKFLOW-001 v2.3, DS_V1_0_ZERO_TECHNICAL_DEBT_COMPLETE.md
**Related**: DS_V1_0_DB_ADAPTER_STRUCTURED_TYPES_COMPLETE.md, TRIAGE_V2_2_FINAL_STATUS_DEC_17_2025.md

