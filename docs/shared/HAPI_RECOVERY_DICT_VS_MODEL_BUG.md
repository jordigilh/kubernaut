# HAPI Recovery Endpoint - Dict vs Pydantic Model Bug

**Date**: December 30, 2025
**Team**: AA Team → HAPI Team
**Status**: 🐛 **BUG IDENTIFIED**
**Severity**: **P0 - Blocks BR-HAPI-197 Integration Tests**

---

## 🚨 **Bug Summary**

**HAPI's recovery endpoint (`/api/v1/recovery/analyze`) is returning HTTP 500** because the mock response logic returns a **dict** instead of a **RecoveryResponse Pydantic model**.

### **Error**
```
ERROR: "'dict' object has no attribute 'needs_human_review'"
AttributeError: 'dict' object has no attribute 'needs_human_review'
```

### **Impact**
- ❌ All recovery endpoint calls fail with HTTP 500
- ❌ BR-HAPI-197 integration tests cannot run
- ❌ AIAnalysis controller cannot perform recovery attempts
- ✅ Investigation endpoint works fine (not affected)

---

## 🔍 **Root Cause Analysis**

### **Evidence from HAPI Logs**

```
ERROR:src.middleware.metrics:{'event': 'request_failed', 'method': 'POST',
  'endpoint': '/api/v1/recovery/analyze',
  'error': "'dict' object has no attribute 'needs_human_review'"}

ERROR:src.middleware.rfc7807:{'event': 'unexpected_error',
  'path': '/api/v1/recovery/analyze',
  'error_type': 'AttributeError',
  'error': "'dict' object has no attribute 'needs_human_review'"}
```

### **Debug Logs Show**
```
INFO:src.extensions.recovery.endpoint:🔍 DEBUG: Recovery request received - signal_type=None
INFO:src.extensions.recovery.endpoint:🔍 DEBUG: Request dict - signal_type=None,
  is_recovery_attempt=True, recovery_attempt_number=1
INFO:src.extensions.recovery.llm_integration:{'event': 'mock_mode_active',
  'incident_id': 'test-attempt-tracking',
  'message': 'Returning deterministic mock response with audit (MOCK_LLM_MODE=true)'}
```

**Then HTTP 500 occurs when trying to access the response.**

---

## 🐛 **The Problem**

### **Current Code (Broken)**
```python
# holmesgpt-api/src/mock_responses.py
def _generate_no_recovery_workflow_response(...) -> dict:
    """Returns a dict"""
    return {
        "incident_id": incident_id,
        "can_recover": False,
        "needs_human_review": True,  # ← Dict key
        "human_review_reason": "no_matching_workflows",
        ...
    }

# holmesgpt-api/src/extensions/recovery/endpoint.py
async def recovery_analyze_endpoint(request: RecoveryRequest) -> RecoveryResponse:
    result = await analyze_recovery(request_data)  # Returns dict

    # DEBUG logging tries to access as attribute
    logger.info(f"Response - needs_human_review={result.needs_human_review}")
    # ❌ FAILS: dict has no attribute 'needs_human_review'

    return result  # FastAPI tries to validate dict → RecoveryResponse
```

### **Why It Fails**
1. `analyze_recovery()` returns a **dict** from `_generate_no_recovery_workflow_response()`
2. The debug logging code tries to access `result.needs_human_review` (attribute access)
3. **AttributeError**: dict uses `result['needs_human_review']` (key access)
4. Exception propagates → HTTP 500

---

## ✅ **The Fix**

### **Option A: Convert Dict to Pydantic Model** (Recommended)

```python
# holmesgpt-api/src/extensions/recovery/endpoint.py
async def recovery_analyze_endpoint(request: RecoveryRequest) -> RecoveryResponse:
    request_data = request.model_dump() if hasattr(request, 'model_dump') else request.dict()
    result_dict = await analyze_recovery(request_data)

    # Convert dict to Pydantic model
    result = RecoveryResponse(**result_dict)

    # Now this works
    logger.info(f"Response - needs_human_review={result.needs_human_review}")

    return result
```

### **Option B: Fix Mock Functions to Return Models**

```python
# holmesgpt-api/src/mock_responses.py
from src.models.recovery_models import RecoveryResponse

def _generate_no_recovery_workflow_response(...) -> RecoveryResponse:
    """Returns a RecoveryResponse model"""
    return RecoveryResponse(
        incident_id=incident_id,
        can_recover=False,
        needs_human_review=True,
        human_review_reason="no_matching_workflows",
        ...
    )
```

### **Option C: Use Dict Access in Debug Logging**

```python
# holmesgpt-api/src/extensions/recovery/endpoint.py
async def recovery_analyze_endpoint(request: RecoveryRequest) -> RecoveryResponse:
    result = await analyze_recovery(request_data)

    # Use dict access for debug logging
    needs_human_review = result.get('needs_human_review', False) if isinstance(result, dict) else result.needs_human_review
    logger.info(f"Response - needs_human_review={needs_human_review}")

    return result  # FastAPI will convert dict → RecoveryResponse
```

---

## 🎯 **Recommended Approach**

**Option A is best** because:
1. ✅ **Type Safety**: Pydantic validates the response structure
2. ✅ **Consistency**: All code can use attribute access (`result.needs_human_review`)
3. ✅ **Error Detection**: Pydantic catches missing/invalid fields early
4. ✅ **Minimal Changes**: Only modify the endpoint, not all mock functions

---

## 🔧 **Implementation Steps**

### **Step 1: Add Model Conversion**
```python
# File: holmesgpt-api/src/extensions/recovery/endpoint.py

from src.models.recovery_models import RecoveryResponse

@router.post(
    "/recovery/analyze",
    status_code=status.HTTP_200_OK,
    response_model=RecoveryResponse,
    response_model_exclude_unset=False
)
async def recovery_analyze_endpoint(request: RecoveryRequest) -> RecoveryResponse:
    # ... existing code ...

    result_dict = await analyze_recovery(request_data)

    # Convert dict to Pydantic model (ADD THIS)
    if isinstance(result_dict, dict):
        result = RecoveryResponse(**result_dict)
    else:
        result = result_dict

    # Now debug logging works
    logger.info(f"🔍 DEBUG: Response - needs_human_review={result.needs_human_review}, "
                f"human_review_reason={result.human_review_reason!r}")

    return result
```

### **Step 2: Test the Fix**
```bash
# Start integration tests
cd kubernaut
PRESERVE_CONTAINERS=true make test-integration-aianalysis FOCUS="BR-HAPI-197"

# While running, test HAPI directly
curl -X POST http://localhost:18120/api/v1/recovery/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "incident_id": "test",
    "remediation_id": "test",
    "signal_type": "MOCK_NO_WORKFLOW_FOUND",
    "is_recovery_attempt": true,
    "recovery_attempt_number": 1
  }' | jq '.'

# Should return HTTP 200 with needs_human_review=true
```

---

## 📊 **Verification**

### **Before Fix** ❌
```bash
$ curl -X POST http://localhost:18120/api/v1/recovery/analyze ...
HTTP/1.1 500 Internal Server Error
{
  "detail": "Internal server error"
}
```

### **After Fix** ✅
```bash
$ curl -X POST http://localhost:18120/api/v1/recovery/analyze ...
HTTP/1.1 200 OK
{
  "incident_id": "test",
  "can_recover": false,
  "needs_human_review": true,
  "human_review_reason": "no_matching_workflows",
  ...
}
```

---

## 🔗 **Related Documents**

- **HTTP 500 Investigation**: `docs/handoff/AA_INTEGRATION_TEST_HAPI_500_ERROR_DEC_30_2025.md`
- **Container Preservation**: `docs/handoff/AA_PRESERVE_CONTAINERS_ON_FAILURE_DEC_30_2025.md`
- **HAPI Mock Request**: `docs/shared/HAPI_RECOVERY_MOCK_EDGE_CASES_REQUEST.md`
- **BR-HAPI-197 Implementation**: `docs/handoff/AA_RECOVERY_HUMAN_REVIEW_COMPLETE_DEC_30_2025.md`

---

## ✅ **Action Items**

### **HAPI Team** (P0 - Immediate)
- [ ] Implement Option A (dict → Pydantic model conversion) in `recovery/endpoint.py`
- [ ] Test with `curl` to verify HTTP 200 response
- [ ] Notify AA team when fixed

### **AA Team** (After HAPI fix)
- [ ] Re-run integration tests: `make test-integration-aianalysis FOCUS="BR-HAPI-197"`
- [ ] Verify all 3 BR-HAPI-197 tests pass
- [ ] Clean up preserved containers

---

**Status**: 🐛 Bug identified and documented
**Blocker Resolved**: Yes - root cause found
**Next Action**: HAPI team to implement Option A fix

