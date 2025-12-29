# DataStorage RFC 7807 URL Pattern - Authoritative Decision

**Date**: December 16, 2025
**Status**: ✅ **CONCLUSIVE** - Authoritative documentation found
**Authority**: DD-004-RFC7807-ERROR-RESPONSES.md (Production Standard)

---

## 🎯 **Executive Summary**

**Question**: Which RFC 7807 URL pattern is correct?
- `https://kubernaut.io/errors/*` (validation package)
- `https://api.kubernaut.io/problems/*` (response package)

**Answer**: ✅ **`https://kubernaut.io/errors/*`** is the authoritative standard

**Authority**: DD-004 (RFC 7807 Error Response Standard) - Production Standard since October 30, 2025

**Action**: Fix response package to use `https://kubernaut.io/errors/*` pattern

---

## 📚 **Authoritative Documentation**

### **DD-004: RFC 7807 Error Response Standard**

**File**: `docs/architecture/decisions/DD-004-RFC7807-ERROR-RESPONSES.md`

**Status**: ✅ **APPROVED** (Production Standard)
**Date**: October 30, 2025
**Confidence**: 95%

---

### **Exact Specification** (Lines 210-228)

```markdown
### **Error Type URI Convention**

**Format**: `https://kubernaut.io/errors/{error-type}`

**Standard Error Types**:

| HTTP Status | Error Type | Title | Use Case |
|-------------|-----------|-------|----------|
| **400** | `validation-error` | Bad Request | Invalid request format, missing fields |
| **405** | `method-not-allowed` | Method Not Allowed | Wrong HTTP method (GET instead of POST) |
| **415** | `unsupported-media-type` | Unsupported Media Type | Wrong Content-Type header |
| **500** | `internal-error` | Internal Server Error | Unexpected server errors |
| **503** | `service-unavailable` | Service Unavailable | Dependencies down, graceful shutdown |
| **504** | `gateway-timeout` | Gateway Timeout | Upstream service timeout |

**Example URIs**:
- `https://kubernaut.io/errors/validation-error`
- `https://kubernaut.io/errors/service-unavailable`
- `https://kubernaut.io/errors/internal-error`
```

---

## 🚨 **Context API Lesson Learned**

### **Historical Precedent: This Exact Problem Happened Before!**

**File**: `docs/services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V4.8.md:934-935`

**Warning in Code Comment**:
```go
// Error type URI constants (use kubernaut.io domain, NOT api.kubernaut.io)
// Context API Lesson: Wrong domain caused 6 test failures
const (
	ErrorTypeValidationError      = "https://kubernaut.io/errors/validation-error"
	ErrorTypeNotFound             = "https://kubernaut.io/errors/not-found"
	ErrorTypeMethodNotAllowed     = "https://kubernaut.io/errors/method-not-allowed"
	ErrorTypeUnsupportedMediaType = "https://kubernaut.io/errors/unsupported-media-type"
	ErrorTypeInternalError        = "https://kubernaut.io/errors/internal-error"
	ErrorTypeServiceUnavailable   = "https://kubernaut.io/errors/service-unavailable"
	ErrorTypeDualWriteFailure     = "https://kubernaut.io/errors/dual-write-failure"
	ErrorTypeEmbeddingFailure     = "https://kubernaut.io/errors/embedding-failure"
)
```

**Key Insight**: The Context API team encountered this exact issue and documented it as a lesson learned. Using `api.kubernaut.io` instead of `kubernaut.io` caused 6 test failures.

---

## 📊 **Supporting Evidence**

### **1. Toolset Package** (Shared Library)

**File**: `pkg/toolset/errors/rfc7807.go:26-32`

```go
// Error type URI constants
// BR-TOOLSET-039: RFC 7807 error format
// These URIs identify the problem type and can link to documentation
const (
	ErrorTypeValidationError      = "https://kubernaut.io/errors/validation-error"
	ErrorTypeUnsupportedMediaType = "https://kubernaut.io/errors/unsupported-media-type"
	ErrorTypeMethodNotAllowed     = "https://kubernaut.io/errors/method-not-allowed"
	ErrorTypeInternalError        = "https://kubernaut.io/errors/internal-error"
	ErrorTypeServiceUnavailable   = "https://kubernaut.io/errors/service-unavailable"
	ErrorTypeUnknown              = "https://kubernaut.io/errors/unknown"
)
```

**Authority**: Shared toolset package used across multiple services

---

### **2. DataStorage Validation Package**

**File**: `pkg/datastorage/validation/errors.go:74,180,195,210,223,236`

```go
Type: "https://kubernaut.io/errors/validation-error"
Type: "https://kubernaut.io/errors/validation-error"
Type: "https://kubernaut.io/errors/not-found"
Type: "https://kubernaut.io/errors/internal-error"
Type: "https://kubernaut.io/errors/service-unavailable"
Type: "https://kubernaut.io/errors/conflict"
```

**Status**: ✅ **CORRECT** - Already compliant with DD-004

---

### **3. Gateway Service**

**File**: `pkg/gateway/errors/rfc7807.go`

Uses `https://kubernaut.io/errors/*` pattern (referenced as authoritative example in DD-004)

---

## ❌ **The Incorrect Pattern**

### **Response Package - DOES NOT COMPLY WITH DD-004**

**File**: `pkg/datastorage/server/response/rfc7807.go:55`

```go
Type: fmt.Sprintf("https://api.kubernaut.io/problems/%s", errorType)
```

**Status**: ❌ **NON-COMPLIANT** - Violates DD-004 standard

**Origin**: Introduced during Phase 2.1 refactoring (not based on authoritative documentation)

---

## 🔧 **Required Fix**

### **Update Response Package to DD-004 Standard**

**File**: `pkg/datastorage/server/response/rfc7807.go:55`

**Change**:
```go
// BEFORE (NON-COMPLIANT):
Type: fmt.Sprintf("https://api.kubernaut.io/problems/%s", errorType)

// AFTER (DD-004 COMPLIANT):
Type: fmt.Sprintf("https://kubernaut.io/errors/%s", errorType)
```

**Impact**: 1 line change, fixes 3 integration test failures

---

## 📊 **Why `kubernaut.io` NOT `api.kubernaut.io`**

### **Rationale from DD-004**

1. **RFC 7807 Best Practice**: Error type URIs should be documentation links
   - `https://kubernaut.io/errors/validation-error` → can host error docs
   - `https://api.kubernaut.io/problems/validation-error` → implies API endpoint (wrong)

2. **Clarity**: `errors` subdomain clearly indicates documentation resource
   - `kubernaut.io/errors/*` = documentation
   - `api.kubernaut.io/*` = API endpoints

3. **Consistency**: All Kubernaut services use `kubernaut.io/errors/*`
   - Gateway ✅
   - Toolset ✅
   - DataStorage validation package ✅
   - Context API ✅ (after lesson learned)

4. **Historical Precedent**: Context API already learned this lesson
   - Using `api.kubernaut.io` caused 6 test failures
   - Documented as anti-pattern

---

## ✅ **Decision**

**Authoritative Standard**: `https://kubernaut.io/errors/{error-type}`

**Authority Hierarchy**:
1. ✅ DD-004 (Production Standard) - October 30, 2025
2. ✅ Context API Lesson Learned - Documented in code
3. ✅ Toolset Package (Shared Library) - Uses correct pattern
4. ✅ Gateway Service (Reference Implementation) - Uses correct pattern
5. ✅ DataStorage Validation Package - Already correct
6. ❌ DataStorage Response Package - **NEEDS FIX**

**Confidence**: 100% (authoritative documentation found, precedent established)

---

## 🎯 **Implementation**

### **Option 1: Fix Response Package** (Recommended)

**File**: `pkg/datastorage/server/response/rfc7807.go:55`

**Change**:
```go
func WriteRFC7807Error(w http.ResponseWriter, status int, errorType, title, detail string, logger logr.Logger) {
	problem := RFC7807Problem{
		Type:   fmt.Sprintf("https://kubernaut.io/errors/%s", errorType), // CHANGED: api.kubernaut.io/problems → kubernaut.io/errors
		Title:  title,
		Status: status,
		Detail: detail,
	}
	// ... rest of function ...
}
```

**Pros**:
- ✅ Makes response package DD-004 compliant
- ✅ Aligns with all other Kubernaut services
- ✅ Fixes 3 integration test failures
- ✅ Future-proof (if response package is used elsewhere)

**Cons**:
- None (this is the correct fix)

**Effort**: 1 minute (1 line change)

---

### **Option 2: Preserve Original in Helper** (Also Valid, but Band-aid)

**File**: `pkg/datastorage/server/audit_handlers.go:209-219`

**Change**: Make helper write validation package's URL directly without transformation

**Pros**:
- ✅ Quick fix for immediate test failures
- ✅ Respects validation package authority

**Cons**:
- ❌ Leaves response package non-compliant with DD-004
- ❌ Duplicates encoding logic
- ❌ Future risk if response package is used elsewhere

**Effort**: 5 minutes

---

## 🚀 **Recommended Action**

**Fix Option 1** (Response Package) - This is the root cause and should be fixed at the source

**Steps**:
1. Update `pkg/datastorage/server/response/rfc7807.go:55`
2. Change `https://api.kubernaut.io/problems/%s` to `https://kubernaut.io/errors/%s`
3. Run integration tests → expect 158/158 passing
4. Add code comment referencing DD-004 and Context API lesson

**Expected Result**:
```bash
make test-integration-datastorage
# Ran 158 of 158 Specs in ~240 seconds
# SUCCESS! -- 158 Passed | 0 Failed | 0 Pending | 0 Skipped
```

---

## 📋 **Summary**

**Question**: Which URL pattern is correct?

**Answer**: ✅ `https://kubernaut.io/errors/*`

**Authority**:
- DD-004 (Production Standard)
- Context API lesson learned
- Toolset package implementation
- Gateway service implementation
- All existing test expectations

**Action**: Update response package line 55 to use `https://kubernaut.io/errors/*`

**Confidence**: 100% (conclusive authoritative documentation)

---

**Document Status**: ✅ Complete
**Authority**: DD-004-RFC7807-ERROR-RESPONSES.md
**Last Updated**: December 16, 2025, 9:00 PM



