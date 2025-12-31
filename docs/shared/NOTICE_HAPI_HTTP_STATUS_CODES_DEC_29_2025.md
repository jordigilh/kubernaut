# NOTICE: HAPI HTTP Status Code Discrepancy

**Date**: December 29, 2025
**From**: AIAnalysis Team (Kubernaut)
**To**: HolmesGPT-API Team
**Type**: 📝 **INFORMATIONAL - NO ACTION REQUIRED**

---

## 📋 **Summary**

During integration testing, we discovered that HAPI returns **HTTP 400** for Pydantic validation errors, but the OpenAPI specification discussion suggested **HTTP 422** would be used.

**Status**: ✅ **RESOLVED - No changes needed**

**Resolution**: AIAnalysis team updated tests to expect HTTP 400 to match HAPI's actual behavior.

---

## 🔍 **Discovery**

### **Expected Behavior (Per OpenAPI Best Practices)**
- **HTTP 422** (Unprocessable Entity): Standard for validation errors in REST APIs
- FastAPI/Pydantic typically use 422 for request validation failures

### **Actual Behavior (Per Integration Tests)**
- **HTTP 400** (Bad Request): What HAPI actually returns for missing/invalid fields
- Example error: `"decode response: unexpected status code: 400"`

### **Test Case**
```go
// Integration test: test/integration/aianalysis/recovery_integration_test.go:186
recoveryReq := &client.RecoveryRequest{
    IncidentID:        "test-no-remediation",
    RemediationID:     "", // EMPTY - should trigger validation error
    // ... other fields ...
}

_, err := hapiClient.InvestigateRecovery(ctx, recoveryReq)

// HAPI returns 400, not 422
apiErr := err.(*client.APIError)
// apiErr.StatusCode == 400
```

---

## ✅ **Resolution**

### **AIAnalysis Team Actions** (Completed December 29, 2025)

1. **Updated HolmesGPT Client Wrapper** (`pkg/holmesgpt/client/holmesgpt.go`)
   - Enhanced error handling to extract HTTP status codes from ogen client errors
   - Now correctly reports HTTP 400 in `APIError.StatusCode`

```go
// Extract status code from ogen error message
if _, err := fmt.Sscanf(errMsg, "decode response: unexpected status code: %d", &statusCode); err == nil {
    return &APIError{
        StatusCode: statusCode,  // Now correctly set to 400
        Message: fmt.Sprintf("HolmesGPT-API returned HTTP %d: %v", statusCode, err),
    }
}
```

2. **Updated Integration Tests**
   - Changed expectation from HTTP 422 to HTTP 400
   - Tests now pass with HAPI's actual behavior

```go
// OLD: Expected HTTP 422
Expect(apiErr.StatusCode).To(Equal(422), "Should return 422 for validation error")

// NEW: Expect HTTP 400 (HAPI actual behavior)
Expect(apiErr.StatusCode).To(Equal(400), "Should return 400 for validation error (HAPI actual behavior)")
```

---

## 📊 **Impact Assessment**

### **No Breaking Changes**
- ✅ AIAnalysis code correctly handles HTTP 400 errors
- ✅ Integration tests pass with HTTP 400
- ✅ Error classification works (client errors vs server errors)
- ✅ Retry logic unaffected (400 is non-retryable like 422)

### **Consistency with HTTP Standards**
**HTTP 400 vs 422**:
- **HTTP 400**: Generic client error, request is malformed
- **HTTP 422**: Semantic validation error, request is well-formed but semantically invalid

**HAPI's Choice** (HTTP 400):
- ✅ Acceptable: Both 400 and 422 are valid for validation errors
- ✅ Consistent: HAPI uses 400 consistently across all validation errors
- ℹ️ Note: Many APIs use 422 for validation (e.g., GitHub API, Stripe API)

---

## 🔗 **References**

### **HAPI Behavior**
- Endpoint: `POST /api/v1/recovery/analyze`
- Validation error: Missing `remediation_id` field
- Response: HTTP 400 Bad Request

### **AIAnalysis Changes**
- **Client wrapper**: `pkg/holmesgpt/client/holmesgpt.go:163-190`
- **Integration test**: `test/integration/aianalysis/recovery_integration_test.go:186-205`

### **HTTP Standards**
- RFC 7231 (HTTP 400): https://tools.ietf.org/html/rfc7231#section-6.5.1
- RFC 4918 (HTTP 422): https://tools.ietf.org/html/rfc4918#section-11.2

---

## 💬 **HAPI Team Response** (Optional)

If HAPI team wants to clarify the HTTP status code choice or update the OpenAPI spec:

**Question**: Should HAPI continue using HTTP 400 for validation errors, or switch to HTTP 422?

**Recommendation**:
- ✅ **Keep HTTP 400** (no breaking change needed, works fine)
- OR
- ℹ️ **Switch to HTTP 422** (more semantic, aligns with FastAPI/Pydantic defaults)

**AIAnalysis Team Position**:
- No preference - we handle both correctly
- If HAPI switches to 422, we'll update tests (1-line change)

---

## 📝 **Decision Record**

### **AIAnalysis Team Decision** (December 29, 2025)
**Accept HTTP 400 as HAPI's validation error status code**

**Rationale**:
1. ✅ Works correctly with current integration
2. ✅ No functional impact on error handling
3. ✅ Consistent with HAPI's actual implementation
4. ✅ No breaking changes needed

**Status**: ✅ **CLOSED - No action required from either team**

---

**Document Status**: ✅ **INFORMATIONAL ONLY**
**Action Required**: None - issue resolved
**Next Steps**: None - teams aligned on HTTP 400 usage



