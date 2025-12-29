# TRIAGE: Data Storage Audit Event Unstructured Filters Violation

**Date**: 2025-12-11
**Service**: Data Storage
**Severity**: HIGH
**Type**: Type Safety Violation
**Reporter**: User (jordigilh)

---

## 🚨 **VIOLATION DETECTED**

### **Location**
- **File**: `pkg/datastorage/audit/workflow_search_event.go`
- **Struct**: `QueryMetadata`
- **Field**: `Filters map[string]interface{}`

### **Violation Details**
```go
// CURRENT (VIOLATES TYPE SAFETY GUIDELINES):
type QueryMetadata struct {
    TopK     int                    `json:"top_k"`
    MinScore float64                `json:"min_score,omitempty"`
    Filters  map[string]interface{} `json:"filters"` // ❌ UNSTRUCTURED
}
```

### **Project Guidelines Violated**
From `00-project-guidelines.mdc`:
```
Type System Guidelines:
- **AVOID** using `any` or `interface{}` unless absolutely necessary
- **ALWAYS** use structured field values with specific types
- **AVOID** local type definitions to resolve import cycles
- Use shared types from pkg/shared/types/ instead
```

**Authority**: Rule priority 1 (FOUNDATIONAL) - mandatory compliance

---

## 📊 **IMPACT ASSESSMENT**

### **Current State**
1. **Audit Events**: Using `map[string]interface{}` for filters
2. **Test Code**: Using unstructured data access (`eventData.Query.Filters["signal_type"]`)
3. **Type Safety**: NO compile-time validation of filter fields
4. **Maintainability**: Risk of field name typos and incorrect type usage

### **Files Affected**
| File | Issue | Impact |
|------|-------|--------|
| `pkg/datastorage/audit/workflow_search_event.go` | `QueryMetadata.Filters` definition | HIGH - Core audit structure |
| `test/unit/datastorage/workflow_search_audit_test.go` | Test assertions using unstructured access | MEDIUM - Tests use string literals |

### **Risk Analysis**
| Risk | Severity | Likelihood | Mitigation Required |
|------|----------|------------|---------------------|
| Runtime panics from invalid type assertions | HIGH | MEDIUM | Use structured types |
| Field name typos undetected at compile time | HIGH | HIGH | Use structured types |
| Inconsistent filter representation | MEDIUM | HIGH | Use structured types |
| Harder to maintain and refactor | MEDIUM | CERTAIN | Use structured types |

---

## 🎯 **ROOT CAUSE ANALYSIS**

### **Why This Happened**
During the embedding removal refactoring, the `QueryMetadata` struct was updated to use `Filters` instead of `Query.Text`. The implementation chose `map[string]interface{}` for quick compatibility with `models.WorkflowSearchFilters`, but **forgot to use the actual structured type**.

### **Correct Approach**
We **already have** a structured type: `models.WorkflowSearchFilters`

```go
// EXISTING STRUCTURED TYPE in pkg/datastorage/models/workflow.go:
type WorkflowSearchFilters struct {
    // Mandatory labels (BR-WORKFLOW-001)
    SignalType  string `json:"signal_type" validate:"required"`
    Severity    string `json:"severity" validate:"required"`
    Component   string `json:"component" validate:"required"`
    Environment string `json:"environment" validate:"required"`
    Priority    string `json:"priority" validate:"required"`

    // Optional DetectedLabels
    DetectedLabels *DetectedLabels `json:"detected_labels,omitempty"`
}
```

**We should be using THIS type directly!**

---

## ✅ **RECOMMENDED FIX**

### **Priority**: **P0 - IMMEDIATE** (Type safety is non-negotiable)

### **Solution**: Use Structured Type Directly

```go
// AFTER (CORRECT - TYPE SAFE):
type QueryMetadata struct {
    TopK     int                       `json:"top_k"`
    MinScore float64                   `json:"min_score,omitempty"`
    Filters  *models.WorkflowSearchFilters `json:"filters"` // ✅ STRUCTURED
}
```

### **Benefits**
1. ✅ **Compile-time validation** - Invalid field names cause build errors
2. ✅ **Type safety** - No runtime type assertion failures
3. ✅ **IDE support** - Autocomplete and refactoring tools work correctly
4. ✅ **Consistency** - Same type used across request, response, and audit
5. ✅ **Maintainability** - Single source of truth for filter structure

---

## 🔧 **IMPLEMENTATION PLAN**

### **Phase 1: Fix Audit Event Structure (IMMEDIATE)**

#### **Step 1.1: Update QueryMetadata**
**File**: `pkg/datastorage/audit/workflow_search_event.go`

```go
// Change line ~63:
type QueryMetadata struct {
    TopK     int                       `json:"top_k"`
    MinScore float64                   `json:"min_score,omitempty"`
    Filters  *models.WorkflowSearchFilters `json:"filters"` // STRUCTURED TYPE
}
```

#### **Step 1.2: Update buildQueryMetadata Function**
**File**: `pkg/datastorage/audit/workflow_search_event.go` (lines ~220-245)

```go
func buildQueryMetadata(request *models.WorkflowSearchRequest) QueryMetadata {
    metadata := QueryMetadata{
        TopK:     request.TopK,
        MinScore: request.MinScore,
        Filters:  request.Filters, // DIRECT ASSIGNMENT - NO CONVERSION NEEDED
    }
    return metadata
}
```

#### **Step 1.3: Update generateQueryHash Function**
**File**: `pkg/datastorage/audit/workflow_search_event.go` (lines ~340-360)

```go
func generateQueryHash(request *models.WorkflowSearchRequest) string {
    if request == nil || request.Filters == nil {
        return "00000000000000"
    }

    // Create hash from filter values (deterministic label-based search)
    hashInput := fmt.Sprintf("%s-%s-%s-%s-%s",
        request.Filters.SignalType,  // STRUCTURED ACCESS
        request.Filters.Severity,
        request.Filters.Component,
        request.Filters.Environment,
        request.Filters.Priority,
    )

    hash := sha256.Sum256([]byte(hashInput))
    return hex.EncodeToString(hash[:])[:16]
}
```

### **Phase 2: Update Unit Tests (IMMEDIATE)**

#### **Step 2.1: Fix Test Assertions**
**File**: `test/unit/datastorage/workflow_search_audit_test.go`

```go
// BEFORE (UNSTRUCTURED):
Expect(eventData.Query.Filters["signal_type"]).To(Equal("OOMKilled"))

// AFTER (STRUCTURED):
Expect(eventData.Query.Filters.SignalType).To(Equal("OOMKilled"))
```

**Lines to update**:
- Line ~148: `eventData.Query.Filters["signal_type"]` → `eventData.Query.Filters.SignalType`
- Line ~276: `eventData.Query.Filters["signal_type"]` → `eventData.Query.Filters.SignalType`

#### **Step 2.2: Fix Nil Checks**
```go
// BEFORE:
Expect(eventData.Query.Filters).NotTo(BeEmpty())

// AFTER:
Expect(eventData.Query.Filters).NotTo(BeNil())
```

---

## 🧪 **VALIDATION PLAN**

### **Build Validation**
```bash
make build-datastorage
```
**Expected**: No compilation errors

### **Unit Test Validation**
```bash
make test-unit-datastorage
```
**Expected**: All audit tests pass with structured types

### **Type Safety Verification**
```bash
# Verify no map[string]interface{} in audit code
grep -n "map\[string\]interface{}" pkg/datastorage/audit/*.go
```
**Expected**: Zero results (or only in comments)

---

## 📋 **ACCEPTANCE CRITERIA**

- [ ] `QueryMetadata.Filters` uses `*models.WorkflowSearchFilters`
- [ ] `buildQueryMetadata` directly assigns structured filters
- [ ] `generateQueryHash` uses structured field access
- [ ] Unit tests use structured field access (`.SignalType`, `.Severity`, etc.)
- [ ] Build succeeds without errors
- [ ] Unit tests pass without errors
- [ ] No `map[string]interface{}` remains in audit code (except justified exceptions)

---

## 🎯 **BUSINESS REQUIREMENTS ALIGNMENT**

### **BR-AUDIT-025: Query Metadata Capture**
- **Before**: Query metadata captured but NOT type-safe
- **After**: Query metadata captured WITH compile-time type safety

### **Technical Debt Reduction**
- **Debt Created**: Unstructured data in audit events during refactoring
- **Debt Eliminated**: Type-safe structured data following project guidelines

---

## 📊 **CONFIDENCE ASSESSMENT**

### **Fix Confidence: 98%**

**High Confidence Factors**:
1. ✅ Structured type already exists (`models.WorkflowSearchFilters`)
2. ✅ Simple type replacement (no logic changes)
3. ✅ Clear test coverage to validate correctness
4. ✅ Compile-time validation ensures correctness

**Minimal Risk Factors**:
1. ⚠️ JSON serialization must preserve field names (already correct with json tags)

---

## 🚀 **PRIORITY JUSTIFICATION**

### **Why P0 (Immediate)**
1. **Type Safety Violation**: Core project principle violated
2. **Maintenance Risk**: Runtime errors possible from unstructured access
3. **Simple Fix**: 5-minute change with clear path
4. **Blocking**: Prevents establishing bad patterns in codebase

### **Effort**: **15 minutes**
- 5 min: Update audit event structure
- 5 min: Update unit tests
- 5 min: Validation

---

## 📝 **NEXT STEPS**

1. **Implement Fix** (NOW)
   - Update `QueryMetadata.Filters` type
   - Update `buildQueryMetadata` function
   - Update `generateQueryHash` function

2. **Update Tests** (NOW)
   - Fix structured field access in assertions
   - Update nil/empty checks

3. **Validate** (NOW)
   - Build validation
   - Unit test execution
   - Grep verification

4. **Document** (AFTER FIX)
   - Update this triage with resolution status
   - Create follow-up if similar patterns found elsewhere

---

## 🔗 **RELATED DOCUMENTATION**

- **Authority**: `00-project-guidelines.mdc` - Type System Guidelines
- **Reference**: `pkg/datastorage/models/workflow.go` - `WorkflowSearchFilters` definition
- **Context**: `docs/handoff/API_IMPACT_REMOVE_EMBEDDINGS.md` - API changes that introduced this

---

## ✅ **RESOLUTION STATUS**

**Status**: 🟢 **RESOLVED** - Implementation completed and validated

**Implementation Summary**:
1. ✅ Updated `QueryMetadata.Filters` to use `*models.WorkflowSearchFilters`
2. ✅ Simplified `buildQueryMetadata()` to direct assignment (eliminated 70+ lines of manual map construction)
3. ✅ Verified `generateQueryHash()` already used structured access (no changes needed)
4. ✅ Updated unit test assertions to use structured field access
5. ✅ Build validation: SUCCESS
6. ✅ Unit test validation: ALL PASS
7. ✅ Type safety verification: CLEAN (remaining map[string]interface{} justified)

**Actual Resolution Time**: 12 minutes

**Code Changes**:
- `pkg/datastorage/audit/workflow_search_event.go`: 3 lines changed, 70+ lines deleted
- `test/unit/datastorage/workflow_search_audit_test.go`: 12 assertions updated

**Benefits Achieved**:
- ✅ Compile-time validation of filter field names
- ✅ Type-safe field access (no runtime type assertion failures)
- ✅ Simplified code (70+ lines of manual map construction eliminated)
- ✅ Consistent with project guidelines (00-project-guidelines.mdc)
- ✅ Single source of truth for filter structure

---

**Triage Completed By**: AI Assistant (Claude)
**Implemented By**: AI Assistant (Claude)
**Approved By**: User (jordigilh)
**Completion Date**: 2025-12-11
