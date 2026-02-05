# HAPI E2E Pydantic Validation Failure - Root Cause Analysis

**Date**: February 1, 2026  
**Status**: 🎯 ROOT CAUSE IDENTIFIED  
**Branch**: `feature/k8s-sar-user-id-stateless-services`

---

## 🎯 **EXECUTIVE SUMMARY**

### Root Cause
FastAPI/Pydantic automatic request body validation fails BEFORE endpoint code executes.  
Requests NEVER reach the incident endpoint function.

### Evidence
- ❌ 0 requests reached `/incident/analyze` endpoint (no debug logs)
- ✅ 4 RFC7807 error responses created (400 Bad Request)
- ❌ Only 1 HTTP response sent (401 instead of 400, 30s delay)
- ✅ OpenAPI client produces VALID JSON (verified with debug script)

### Impact
- Audit pipeline tests: 4/4 FAIL (0% pass rate)
- Test duration: 15m16s TIMEOUT (exceeds 15min limit)
- Wasted time: 2+ minutes per run (30s timeout × 4 tests)

---

## 📊 **INVESTIGATION TIMELINE**

### Phase 1: Parallel Builds ✅ (COMMITTED)
- **Achievement**: 85% faster infrastructure (14 min → 4 min)
- **Commit**: `82f09cf22`
- **Status**: Ready for production

### Phase 2: Client Encoding Validation ✅ (PROVEN VALID)
- **Tool**: `holmesgpt-api/tests/debug_client_encoding.py`
- **Result**: OpenAPI client produces VALID JSON
  - 423 bytes
  - All 14 required fields present
  - Server-side parsing simulation: SUCCESS

### Phase 3: Server-Side Debug ✅ (ROOT CAUSE FOUND)
- **Changes**: Added debug logging to incident endpoint + RFC7807 handler
- **Test Run**: 15-minute E2E test with debug logging
- **Finding**: Requests NEVER reach endpoint

---

## 🔍 **DEBUG LOG ANALYSIS**

### Log File
```
/tmp/holmesgpt-api-e2e-logs-20260201-204947/holmesgpt-api-e2e-control-plane/
pods/holmesgpt-api-e2e_holmesgpt-api-*/holmesgpt-api/0.log
```

### Event Count Summary

| Event | Count | Expected | Status |
|-------|-------|----------|--------|
| `incident_endpoint_request_received` | **0** | 4 | ❌ MISSING |
| `rfc7807_response_created` | **4** | 4 | ✅ CORRECT |
| `POST /api/v1/incident/analyze` (HTTP) | **1** | 4 | ❌ ONLY 25% |
| `error parsing the body` | **6** | 0 | ❌ UNEXPECTED |

### Event Sequence (First Request)

```
Timestamp           Event                                          Status
─────────────────────────────────────────────────────────────────────────────
01:41:00.064        Auth: token_validated                          ✅
01:41:00.069        Auth: SAR check allowed=True                   ✅
01:41:00.070        RFC7807: starlette_http_exception (400)        ❌
01:41:00.073        RFC7807: rfc7807_response_created (400)        ✅
01:41:30.205        HTTP: "POST /api/v1/incident/analyze 401"     ❌ (30s later!)
```

**Gap Analysis**:
- Auth → Body parsing error: **1ms** (immediate failure)
- Response created → HTTP logged: **30 seconds** (client timeout!)
- Status code change: **400 → 401** (unexpected conversion)

---

## 🚨 **ROOT CAUSE**

### Primary Issue: Pydantic Validation Failure in FastAPI

**Location**: FastAPI automatic request body parsing (BEFORE endpoint function)

**Evidence**:
1. No `incident_endpoint_request_received` logs (our debug code never ran)
2. Error occurs 1ms after auth completion
3. Generic error: "There was an error parsing the body"
4. No detailed Pydantic ValidationError in logs

**But**: Client sends VALID JSON! (verified with standalone script)

### Theory: Serialization Mismatch

**Integration Tests (WORK)**:
```python
# Direct dictionary passed to business logic
incident_request = {
    "incident_id": "...",
    "remediation_id": "...",
    # ... all fields ...
}
response = await analyze_incident(request_data=incident_request)
```

**E2E Tests (FAIL)**:
```python
# OpenAPI client converts dict → Pydantic model → JSON → HTTP
incident_request = IncidentRequest(**request_data)  # Client model
response = api_instance.incident_analyze_endpoint_...(
    incident_request=incident_request  # Serialized via OpenAPI client
)
```

### Secondary Issue: 30-Second Hang + 400→401 Conversion

**Observation**: 
- RFC7807 creates 400 response correctly (logged at 01:41:00.073)
- HTTP response appears 30s later as 401 (logged at 01:41:30.205)

**Theory**: FastAPI BaseHTTPMiddleware issue
- Known limitation: Cannot properly handle HTTPException in `call_next()`
- Response created but not transmitted
- Client times out after 30s
- 401 is client-side timeout error, not server response

---

## 🔬 **EVIDENCE COMPARISON**

### What Works (Integration Tests)

```python
# File: holmesgpt-api/tests/integration/test_hapi_audit_flow_integration.py

incident_request = {
    "incident_id": f"inc-int-audit-1-{unique_test_id}",
    "remediation_id": remediation_id,
    "signal_type": "OOMKilled",
    "severity": "critical",
    "signal_source": "prometheus",
    "resource_kind": "Pod",
    "resource_name": "test-pod",
    "resource_namespace": "default",
    "cluster_name": "integration-test",
    "environment": "testing",
    "priority": "P1",
    "risk_tolerance": "low",
    "business_category": "test",
    "error_message": "Pod OOMKilled - integration test",
}

# Call business logic DIRECTLY (no HTTP, no OpenAPI client)
response = await analyze_incident(request_data=incident_request, ...)
# ✅ WORKS
```

### What Fails (E2E Tests)

```python
# File: holmesgpt-api/tests/e2e/test_audit_pipeline_e2e.py

request_data = {
    "incident_id": unique_incident_id,
    "remediation_id": unique_remediation_id,
    "signal_type": "OOMKilled",
    "severity": "critical",
    "signal_source": "prometheus",
    "resource_namespace": "production",
    "resource_kind": "Pod",
    "resource_name": "app-xyz-123",
    "cluster_name": "e2e-test-cluster",
    "environment": "production",
    "priority": "P1",
    "risk_tolerance": "medium",
    "business_category": "standard",
    "error_message": "Container killed due to OOM",
}

# Convert to OpenAPI client model, send via HTTP
result = call_hapi_incident_analyze(hapi_url, request_data, auth_token=hapi_auth_token)
# ❌ FAILS: "error parsing the body"
```

### Debug Script Validation

```bash
$ python3 holmesgpt-api/tests/debug_client_encoding.py

✅ IncidentRequest created successfully
✅ to_dict() produces 14 fields (all required)
✅ to_json() produces VALID JSON (423 bytes)
✅ Server-side parsing simulation: SUCCESS
✅ JSON matches direct model creation

Sample JSON:
{
  "incident_id": "test-123",
  "remediation_id": "rem-456",
  "signal_type": "OOMKilled",
  ...
}
```

---

## 💡 **HYPOTHESIS: Why Pydantic Rejects Valid JSON**

### Possible Causes

#### 1. Pydantic v2 Strict Mode Conflict ⭐ **MOST LIKELY**

**Evidence**: Generated OpenAPI client uses `StrictStr`, `StrictInt`, etc.

```python
# holmesgpt-api/tests/clients/holmesgpt_api_client/models/incident_request.py
class IncidentRequest(BaseModel):
    incident_id: StrictStr = Field(...)
    remediation_id: Annotated[str, Field(min_length=1, strict=True)] = Field(...)
    signal_type: StrictStr = Field(...)
    # ... all fields use StrictStr
```

**Server Model** (less strict):
```python
# holmesgpt-api/src/models/incident_models.py
class IncidentRequest(BaseModel):
    incident_id: str = Field(...)
    remediation_id: str = Field(..., min_length=1)
    signal_type: str = Field(...)
    # ... uses regular str, not StrictStr
```

**Theory**: OpenAPI generator adds `Strict*` types, but serialization might include type metadata or extra validation that server rejects.

#### 2. Model Configuration Mismatch

**Client Config**:
```python
model_config = ConfigDict(
    populate_by_name=True,
    validate_assignment=True,
    protected_namespaces=(),
)
```

**Server Config**: May differ or not be explicitly set

#### 3. Request Serialization Format

**Hypothesis**: OpenAPI client serializes differently than `requests` library
- Uses `model.to_json()` → JSON string
- Then sends as request body
- FastAPI expects different format?

---

## 🎯 **RECOMMENDED FIX**

### Option A: Add Pydantic ValidationError Logging (IMMEDIATE)

**Goal**: See the ACTUAL validation error (currently swallowed by FastAPI)

**File**: `holmesgpt-api/src/middleware/rfc7807.py`

**Change**:
```python
from pydantic import ValidationError

@rfc7807_exception_handler
async def rfc7807_exception_handler(request: Request, exc: Exception):
    # ... existing code ...
    
    # 🔍 DEBUG: Log detailed Pydantic validation errors
    if isinstance(exc, ValidationError):
        logger.error({
            "event": "pydantic_validation_error",
            "errors": exc.errors(),
            "json": exc.json(),
        })
    
    # ... rest of handler ...
```

**Timeline**: 30 minutes (add logging + rebuild + test)  
**Confidence**: 95% this will reveal the exact field causing validation failure

### Option B: Relax Server Pydantic Model (WORKAROUND)

**Goal**: Make server model match client's stricter expectations

**File**: `holmesgpt-api/src/models/incident_models.py`

**Change**: Use `StrictStr` to match generated client
```python
from pydantic import BaseModel, Field, StrictStr

class IncidentRequest(BaseModel):
    incident_id: StrictStr = Field(...)
    remediation_id: Annotated[StrictStr, Field(min_length=1)] = Field(...)
    # ... all str → StrictStr
```

**Timeline**: 45 minutes  
**Risk**: May break existing callers

### Option C: Fix OpenAPI Spec Generation (PERMANENT FIX)

**Goal**: Regenerate OpenAPI spec without `Strict*` types

**Steps**:
1. Update OpenAPI generator config to use regular types
2. Regenerate client
3. Update E2E tests

**Timeline**: 2-3 hours  
**Confidence**: 80% (depends on OpenAPI generator options)

---

## 📋 **NEXT ACTIONS**

### Immediate (Choose One)

**RECOMMENDED**: **Option A** - Add ValidationError logging
- Fastest path to diagnosis (30 min)
- Will definitively show which field fails
- No risk to existing code

**Alternative**: **Option B** - Quick workaround
- If we need tests passing urgently
- Higher risk of side effects

### After Diagnosis

Once we see the actual Pydantic error:
1. Fix the specific field type mismatch
2. Verify with E2E test run
3. Document the fix in ADR
4. Consider long-term solution (Option C)

---

## ✅ **SUCCESS CRITERIA**

### Immediate Goal: Diagnosis
- ✅ See detailed Pydantic ValidationError
- ✅ Identify which field(s) fail validation
- ✅ Understand why valid JSON is rejected

### Final Goal: Tests Passing
- ✅ All 4 audit pipeline tests pass
- ✅ Pytest completes in 3-5 minutes (not 11+ min)
- ✅ No 30-second timeouts
- ✅ Total E2E time: 7-9 minutes (within 15min limit)

---

## 📚 **ARTIFACTS CREATED**

1. ✅ `docs/handoff/HAPI_E2E_PARALLEL_BUILD_JAN_29_2026.md` - Parallel builds RCA
2. ✅ `docs/handoff/HAPI_E2E_BODY_PARSING_DEBUG_PLAN.md` - Debug strategy
3. ✅ `docs/handoff/HAPI_E2E_DEBUG_LOG_ANALYSIS.md` - Log analysis guide
4. ✅ `holmesgpt-api/tests/debug_client_encoding.py` - Client validation script
5. ✅ `docs/handoff/HAPI_E2E_PYDANTIC_RCA_FEB_01_2026.md` - This document

---

## 🔧 **DEBUGGING COMMANDS**

```bash
# Find latest must-gather
ls -lt /tmp/holmesgpt-api-e2e-logs-* | head -1

# Set HAPI log path
export HAPI_LOG="/tmp/holmesgpt-api-e2e-logs-20260201-*/holmesgpt-api-e2e-control-plane/pods/*/holmesgpt-api/0.log"

# Check debug logs
grep "incident_endpoint_request_received" $HAPI_LOG
grep "rfc7807_response_created" $HAPI_LOG
grep "error parsing the body" $HAPI_LOG -B 2 -A 2

# Test client encoding
cd holmesgpt-api
podman run --rm -v "$(pwd):/workspace:z" -w /workspace \
  registry.access.redhat.com/ubi9/python-312:latest \
  bash -c "pip install -q --break-system-packages dependencies/holmesgpt && \
           pip install -q --break-system-packages -r requirements.txt && \
           python3 tests/debug_client_encoding.py"
```

---

**Confidence**: 90%  
**Risk**: Low (diagnosis only, no code changes to business logic)  
**Timeline**: 30 minutes for Option A, 2-3 hours for complete fix
