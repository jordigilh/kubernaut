# HAPI LiteLLM Custom Endpoint Bug - Root Cause & Fix

**Date**: January 29, 2026  
**Issue**: Mock LLM received ZERO HTTP requests during HAPI E2E tests  
**Impact**: 11/14 HAPI E2E test failures, AIAnalysis integration tests don't validate LLM responses

---

## Executive Summary

**TWO bugs prevented HAPI from calling Mock LLM**:

1. **SDK Bug**: HolmesGPT SDK used `base_url` parameter instead of `api_base` (LiteLLM doesn't recognize `base_url`)
2. **HAPI Bug**: HAPI didn't pass `api_key` when creating SDK Config (LiteLLM requires API key for ALL requests, even custom endpoints)

**Fixes Applied**:
1. SDK: Changed `base_url=self.api_base` → `api_base=self.api_base` in `holmes/core/llm.py`
2. HAPI: Added `api_key=os.getenv("OPENAI_API_KEY")` when creating Config in both incident and recovery endpoints

---

## Root Cause Analysis

### The Bug

**File**: `holmesgpt-api/src/extensions/llm_config.py`  
**Lines**: 217-218

```python
if is_custom_endpoint:
    return f"openai/{model_name}"  # ❌ BUG! Returns "openai/mock-model"
```

**Comment was misleading**:
```python
# These are OpenAI-compatible endpoints that need the "openai/" prefix
```

### The Chain of Events

1. **HAPI Configuration** (correct):
   ```yaml
   LLM_ENDPOINT=http://mock-llm:8080
   LLM_MODEL=mock-model
   LLM_PROVIDER=openai
   ```

2. **HAPI's `llm_config.py`** (buggy):
   ```python
   formatted_model = format_model_name_for_litellm(
       model_name="mock-model",
       provider="openai", 
       llm_endpoint="http://mock-llm:8080"
   )
   # Returns: "openai/mock-model"  ❌
   ```

3. **HolmesGPT SDK** passes to LiteLLM:
   ```python
   litellm.completion(
       model="openai/mock-model",          # ❌ With prefix
       base_url="http://mock-llm:8080"     # Ignored!
   )
   ```

4. **LiteLLM behavior**:
   - Sees `"openai/"` prefix
   - Interprets as: "Use official OpenAI provider"
   - **IGNORES** the `base_url` parameter
   - Routes request to `api.openai.com` (fails silently or uses OpenAI fallback)

5. **Result**: Mock LLM received **ZERO HTTP requests**

### Evidence

**Mock LLM Logs** (`/tmp/holmesgpt-api-e2e-logs-*/mock-llm/0.log`):
```
✅ Mock LLM running at http://0.0.0.0:8080
❌ NO HTTP requests logged (no POST/GET)
```

**HAPI Logs**:
```
LiteLLM completion() model= mock-model; provider = openai
                           ^^^^^^^^^^^^
                           Stripped "openai/" prefix!
```

**LiteLLM Warning**:
```
WARNING - Couldn't find model openai/mock-model in litellm's model list
```

This warning indicates LiteLLM tried to look up "openai/mock-model" as an official model, confirming it's treating it as OpenAI provider instead of using the custom endpoint.

---

## The Fix

**File**: `holmesgpt-api/src/extensions/llm_config.py`  
**Change**: Remove the `"openai/"` prefix for custom endpoints

```python
if is_custom_endpoint:
    # Return model name WITHOUT prefix for custom endpoints
    # LiteLLM will then use the provided base_url parameter
    return model_name  # ✅ Returns "mock-model" (no prefix)
```

### Why This Works

Without the `"openai/"` prefix:
- LiteLLM sees model name: `"mock-model"`
- Recognizes it's not an official OpenAI model
- **USES the provided `base_url` parameter**
- Makes HTTP request to `http://mock-llm:8080` ✅

---

## Impact Assessment

### HAPI E2E Tests

**Before Fix**: 26/40 passing (14 failures)
- 11 failures: Mock LLM not called (wrong workflows, missing alternatives, Python dict responses)
- 3 failures: Expected validation errors (deferred to V1.1)

**After Fix**: Expected 37/40 passing (3 validation failures remain)

### AIAnalysis Integration Tests

**Current Status**: Tests PASS but don't validate LLM responses!

**Issue Found**: `test/integration/aianalysis/recovery_integration_test.go` calls real HAPI + Mock LLM but only validates HTTP contract:

```go
resp, err := realHGClient.InvestigateRecovery(testCtx, recoveryReq)

// Only validates HTTP contract, NOT LLM response content
Expect(err).ToNot(HaveOccurred())
Expect(resp).ToNot(BeNil())
Expect(resp.IncidentID).ToNot(BeEmpty())
// Note: With mock LLM, response may vary but should be valid JSON
```

**Recommendation**: Add validation for:
- Confidence values
- Selected workflow UUIDs
- Alternative workflows
- RCA summary content

**Task Created**: `fix-aa-e2e-llm-validation` to address this gap

---

## Comparison: AIAnalysis vs HAPI E2E Tests

| Service | Test Type | Uses Real HAPI | Uses Mock LLM | Validates LLM Responses | Result |
|---------|-----------|----------------|---------------|------------------------|--------|
| AIAnalysis | Integration (holmesgpt_integration_test.go) | ❌ (in-memory Go mock) | ❌ | ✅ (fixtures) | ✅ Pass |
| AIAnalysis | Integration (recovery_integration_test.go) | ✅ | ✅ | ❌ (HTTP only) | ✅ Pass (bug hidden) |
| HAPI | E2E | ✅ | ✅ | ✅ (full validation) | ❌ Fail (bug exposed) |

**Key Insight**: HAPI E2E tests exposed the bug because they validate **exact LLM response content** (confidence, workflows, UUIDs), not just HTTP contract compliance.

---

## Testing Strategy

### Verification Steps

1. **Apply Fix**:
   ```bash
   # Fix already applied in holmesgpt-api/src/extensions/llm_config.py
   ```

2. **Rebuild HAPI Image**:
   ```bash
   make test-e2e-holmesgpt-api  # Will rebuild with fix
   ```

3. **Check Mock LLM Logs** (during test run):
   ```bash
   # Should see HTTP POST requests to /v1/chat/completions
   kubectl logs -n holmesgpt-api-e2e mock-llm-xxx -f
   ```

4. **Validate Test Results**:
   ```bash
   # Expected: 37/40 passing (3 validation errors deferred to V1.1)
   ```

### Future Prevention

**Recommendation**: Add explicit Mock LLM request count validation to test infrastructure:

```go
// In test teardown
mockLLMRequestCount := GetMockLLMRequestCount()
Expect(mockLLMRequestCount).To(BeNumerically(">", 0), 
    "Mock LLM must receive at least one request (sanity check)")
```

---

## Related Documentation

- **Business Requirements**:
  - `BR-HAPI-197`: `needs_human_review` field
  - `BR-HAPI-198`: Configurable confidence thresholds
  
- **Test Plans**:
  - `docs/development/testing/HAPI_E2E_TEST_PLAN.md`: HAPI E2E test scenarios
  
- **Handoff Documents**:
  - `docs/handoff/HAPI_E2E_CRITICAL_FINDING_MOCK_LLM_NOT_CALLED.md`: Initial discovery
  - `docs/handoff/HAPI_E2E_COMPLETE_TRIAGE_JAN_29_2026.md`: Full triage

---

## Lessons Learned

1. **Don't Trust Comments**: The comment said "need the prefix" but the opposite was true.

2. **Validate External Dependencies**: Mock LLM receiving zero requests was a critical clue.

3. **Test Different Levels**: AIAnalysis integration tests passed because they don't validate LLM content deeply enough.

4. **LiteLLM Behavior**: Provider prefixes (`openai/`, `anthropic/`) affect routing logic, not just model identification.

5. **Configuration is Complex**: The chain HAPI → SDK → LiteLLM → Mock LLM has multiple points where configuration can break silently.

---

## Action Items

- [x] Fix `llm_config.py` to remove prefix for custom endpoints
- [ ] Verify fix with HAPI E2E test run (expected 37/40 passing)
- [ ] Add LLM response validation to AIAnalysis recovery tests
- [ ] Add Mock LLM request count sanity check to test infrastructure
- [ ] Document LiteLLM provider prefix behavior in development docs
