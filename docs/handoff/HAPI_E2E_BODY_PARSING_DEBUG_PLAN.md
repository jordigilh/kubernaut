# HAPI E2E Body Parsing Debug Plan - Jan 29, 2026

## 🔍 **CRITICAL FINDING: Only 1/4 Responses Logged**

### Evidence from Must-Gather

**RFC7807 Logs** (4 errors):
```
00:17:29.046: status_code: 400, detail: 'error parsing body'
00:18:29.320: status_code: 400, detail: 'error parsing body'
00:19:49.503: status_code: 400, detail: 'error parsing body'
(4th error timestamp not in sample)
```

**Uvicorn HTTP Logs** (only 1 response):
```
00:17:59.198: "POST /api/v1/incident/analyze HTTP/1.1" 401 Unauthorized
```

### ❗ THE SMOKING GUN

**Only 1 out of 4 requests got an HTTP response logged!**

This means:
1. ✅ All 4 requests reached HAPI (RFC7807 logged them)
2. ✅ All 4 hit body parsing errors (400)
3. ✅ RFC7807 handler executed for all 4
4. ❌ Only 1 response was sent back (or logged)
5. ❌ 3 requests timed out on client side (30s each = 90s wasted)

---

## 🚨 **ROOT CAUSE HYPOTHESIS**

### Theory: Response Not Being Sent

The RFC7807 handler creates a `JSONResponse(status_code=400)` but something prevents it from being sent:

**Possible Causes**:
1. **Async/await issue**: Response not being properly awaited
2. **Middleware interference**: Something blocking after exception handler
3. **Connection closed**: Client or server closing connection before send
4. **FastAPI/Starlette bug**: Known issue with BaseHTTPMiddleware + exception handlers

---

## 🎯 **TARGETED FIX: Add Request Body Logging**

### Step 1: Add Debug Logging to Endpoint

**File**: `holmesgpt-api/src/extensions/incident/endpoint.py`

**Change** (before validation):
```python
@router.post("/incident/analyze", ...)
async def incident_analyze_endpoint(incident_req: IncidentRequest, request: Request) -> IncidentResponse:
    """..."""
    # 🔍 DEBUG: Log raw request body
    try:
        body_bytes = await request.body()
        logger.info({
            "event": "incident_endpoint_raw_body",
            "body_length": len(body_bytes),
            "body_preview": body_bytes[:500].decode('utf-8', errors='replace'),
            "content_type": request.headers.get('content-type'),
        })
    except Exception as e:
        logger.error(f"Failed to read request body for debugging: {e}")
    
    # DD-AUTH-006: Extract authenticated user
    user = get_authenticated_user(request)
    ...
```

### Step 2: Add Logging to RFC7807 Handler

**File**: `holmesgpt-api/src/middleware/rfc7807.py`

**Change** (after creating JSONResponse):
```python
# Return JSON response with RFC 7807 format
response = JSONResponse(
    status_code=status_code,
    content=error.dict(),
    headers={
        "Content-Type": "application/problem+json",
        "X-Request-ID": request_id
    }
)

# 🔍 DEBUG: Log that we're returning the response
logger.info({
    "event": "rfc7807_response_created",
    "request_id": request_id,
    "status_code": status_code,
    "response_type": type(response).__name__,
})

return response
```

### Step 3: Rebuild and Rerun

```bash
# Rebuild HAPI image
make build-hapi  # or equivalent

# Rerun E2E test
make test-e2e-holmesgpt-api
```

### Step 4: Check New Logs

Look for:
1. **Raw body content**: What is pytest actually sending?
2. **Response created logs**: Are 4 responses created but 3 not sent?
3. **Timing**: Is there a delay between response creation and HTTP log?

---

## 🔧 **ALTERNATIVE: Simplified Client Test**

Instead of full E2E, create a minimal test to isolate the issue:

**File**: `holmesgpt-api/tests/debug_client_encoding.py`

```python
#!/usr/bin/env python3
"""
Debug script to test OpenAPI client encoding of IncidentRequest.
"""
import sys
sys.path.insert(0, 'tests/clients')

from holmesgpt_api_client.models import IncidentRequest
import json

# Same test data that's failing in E2E
test_data = {
    "incident_id": "test-123",
    "remediation_id": "rem-456",
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

# Create OpenAPI client model
incident_req = IncidentRequest(**test_data)

# Check serialization
print("✅ IncidentRequest created")
print(f"   Type: {type(incident_req)}")
print()

# Check to_dict()
dict_output = incident_req.to_dict()
print("📦 to_dict() output:")
print(f"   Fields: {len(dict_output)}")
print(f"   Keys: {list(dict_output.keys())[:5]}...")
print()

# Check model_dump()
dump_output = incident_req.model_dump()
print("📦 model_dump() output:")
print(f"   Fields: {len(dump_output)}")
print(f"   Keys: {list(dump_output.keys())[:5]}...")
print()

# Check JSON serialization
json_output = incident_req.to_json()
print("📦 to_json() output:")
print(f"   Length: {len(json_output)} bytes")
print(f"   Preview: {json_output[:200]}...")
print()

# Check for None values (excluded by to_dict but not model_dump)
none_fields = [k for k, v in dump_output.items() if v is None]
print(f"⚠️  None fields in model_dump: {len(none_fields)}")
if none_fields:
    print(f"   {none_fields[:10]}")
```

**Run it**:
```bash
cd holmesgpt-api
python3 tests/debug_client_encoding.py
```

This will show exactly how the OpenAPI client serializes the request.

---

## 📊 **EXPECTED OUTCOMES**

### Scenario A: Malformed JSON
- **Symptom**: `to_json()` output is invalid/corrupt
- **Fix**: Regenerate OpenAPI client with correct spec
- **Timeline**: 30 minutes

### Scenario B: Response Not Sent
- **Symptom**: RFC7807 logs "response_created" but uvicorn never logs HTTP response
- **Fix**: Remove BaseHTTPMiddleware, use pure ASGI middleware
- **Timeline**: 2-3 hours

### Scenario C: Content-Type Mismatch
- **Symptom**: Client sends `application/x-www-form-urlencoded` instead of `application/json`
- **Fix**: Force `Content-Type: application/json` in client
- **Timeline**: 15 minutes

---

## ✅ **SUCCESS CRITERIA**

After fix:
1. All 4 audit pipeline tests pass ✅
2. Pytest completes in 3-5 minutes (not 11+ min) ✅
3. No 30-second timeouts ✅
4. Total E2E time: 7-9 minutes (within 15min limit) ✅

---

## 🚀 **COMMIT STRATEGY**

### Commit Now (Proven Working):
- ✅ `test/infrastructure/holmesgpt_api.go` (parallel builds)
- ✅ `test/e2e/holmesgpt-api/holmesgpt_api_e2e_suite_test.go` (pip cache)

### Commit After Debug (Pending):
- ⏳ HAPI endpoint/middleware fixes (once body parsing resolved)
- ⏳ E2E test updates (if client changes needed)

**Recommendation**: Commit parallel builds NOW (they deliver 85% speedup), continue debugging body parsing separately.
