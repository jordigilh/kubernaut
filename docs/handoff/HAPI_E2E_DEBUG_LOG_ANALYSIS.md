# HAPI E2E Debug Log Analysis Guide

**Date**: January 29, 2026  
**Purpose**: Diagnose body parsing + 30s hang issue in HAPI E2E tests

---

## 🔧 **Changes Made**

### 1. Incident Endpoint Debug Logging

**File**: `holmesgpt-api/src/extensions/incident/endpoint.py`

**Added** (lines 76-85):
```python
# 🔍 DEBUG: Log raw request details for body parsing investigation
try:
    body_bytes = await request.body()
    logger.info({
        "event": "incident_endpoint_request_received",
        "body_length": len(body_bytes),
        "content_type": request.headers.get('content-type'),
        "body_preview": body_bytes[:300].decode('utf-8', errors='replace'),
        "headers": dict(request.headers),
    })
except Exception as e:
    logger.error(f"🔍 DEBUG: Failed to read request body for debugging: {e}")
```

### 2. RFC7807 Response Creation Logging

**File**: `holmesgpt-api/src/middleware/rfc7807.py`

**Added** (lines 130-137):
```python
# 🔍 DEBUG: Log response creation (troubleshooting 400→401 conversion)
logger.info({
    "event": "rfc7807_response_created",
    "request_id": request_id,
    "status_code": status_code,
    "response_type": type(response).__name__,
    "detail": detail[:100],
})
```

### 3. Verification Checks

**Auth Middleware**: ✅ Clean - No `request.body()` consumption  
**FastAPI Config**: ✅ Default - No body size limits  
**OpenAPI Client**: ✅ Valid JSON (423 bytes, all fields present)

---

## 📋 **How to Run Debug Test**

### Step 1: Rebuild HAPI Image

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Build with updated debug logging
make test-e2e-holmesgpt-api 2>&1 | tee /tmp/hapi-debug-run.log
```

### Step 2: Let Test Run (Will Timeout - Expected)

- **Duration**: 15 minutes (suite timeout)
- **Expected**: Same 4 failures in audit pipeline tests
- **Goal**: Capture debug logs, not pass tests (yet)

### Step 3: Collect Must-Gather

```bash
# Must-gather is auto-collected in /tmp/holmesgpt-api-e2e-logs-*
ls -la /tmp/holmesgpt-api-e2e-logs-*
```

---

## 🔍 **What to Look For in Logs**

### Log File Location

```bash
HAPI_POD_LOG="/tmp/holmesgpt-api-e2e-logs-*/holmesgpt-api-e2e-control-plane/pods/holmesgpt-api-e2e_holmesgpt-api-*/holmesgpt-api/0.log"
```

### Critical Log Patterns

#### Pattern 1: Request Received at Endpoint?

**Search**:
```bash
grep "incident_endpoint_request_received" $HAPI_POD_LOG
```

**Expect**:
- ✅ **YES (4 times)**: Request reached endpoint, check body_length and content_type
- ❌ **NO (0 times)**: Request didn't reach endpoint (middleware blocking?)

**If YES, check**:
```json
{
  "event": "incident_endpoint_request_received",
  "body_length": 423,  // <-- Should be ~400-500 bytes
  "content_type": "application/json",  // <-- Should be JSON
  "body_preview": "{\"incident_id\": \"test-123\", ...}"  // <-- Should be valid JSON
}
```

#### Pattern 2: RFC7807 Response Created?

**Search**:
```bash
grep "rfc7807_response_created" $HAPI_POD_LOG
```

**Expect**:
- ✅ **YES (4 times)**: Response created, check timing vs HTTP log
- ❌ **NO (0 times)**: Exception handler not executing (framework issue?)

**If YES, check timing**:
```bash
# Compare: Response created → HTTP logged
grep -E "rfc7807_response_created|POST /api/v1/incident/analyze" $HAPI_POD_LOG
```

**Expected Pattern**:
```
00:17:29.046: rfc7807_response_created (status_code: 400)
00:17:29.046: POST /api/v1/incident/analyze 400 Bad Request  // <-- Should be IMMEDIATE
```

**Current Problem Pattern**:
```
00:17:29.046: rfc7807_response_created (status_code: 400)
00:17:59.198: POST /api/v1/incident/analyze 401 Unauthorized  // <-- 30s delay! Wrong status!
```

#### Pattern 3: Body Parsing Error Details

**Search**:
```bash
grep "error parsing the body" $HAPI_POD_LOG -B 10 -A 5
```

**Check**:
- What's the last log before "error parsing the body"?
- Is "incident_endpoint_request_received" logged?
- Any exceptions between request received and parsing error?

---

## 🎯 **Diagnosis Decision Tree**

### Scenario A: Request NOT Reaching Endpoint

**Evidence**:
```bash
grep "incident_endpoint_request_received" $HAPI_POD_LOG
# Output: (empty)
```

**Diagnosis**: Middleware blocking request before endpoint  
**Action**: Check auth middleware execution order

---

### Scenario B: Request Reaches Endpoint, Body is Empty/Invalid

**Evidence**:
```json
{
  "event": "incident_endpoint_request_received",
  "body_length": 0,  // <-- PROBLEM
  "content_type": "application/json"
}
```

**Diagnosis**: Body consumed before endpoint (impossible - we checked!)  
**Action**: Check for duplicate request.body() calls in middleware stack

---

### Scenario C: Request Reaches Endpoint, Body is Valid, Still Fails

**Evidence**:
```json
{
  "event": "incident_endpoint_request_received",
  "body_length": 423,
  "content_type": "application/json",
  "body_preview": "{\"incident_id\": \"test-123\", ...}"  // Valid JSON
}
```
+ "error parsing the body" still occurs

**Diagnosis**: Pydantic validation issue (field type mismatch?)  
**Action**: Check IncidentRequest Pydantic model for strict type validation

---

### Scenario D: Response Created but Not Sent (30s Hang)

**Evidence**:
```bash
00:17:29.046: rfc7807_response_created (status_code: 400)
# ... 30 seconds of silence ...
00:17:59.198: POST /api/v1/incident/analyze 401 Unauthorized
```

**Diagnosis**: Response creation successful, but transmission blocked  
**Action**: 
1. Check for blocking I/O in middleware after response creation
2. Investigate BaseHTTPMiddleware async handling
3. Consider switching to pure ASGI middleware

---

## 📊 **Expected Debug Output Examples**

### Good Request (Working)

```
2026-02-02 00:22:08,123 - incident.endpoint - INFO - {
  "event": "incident_endpoint_request_received",
  "body_length": 456,
  "content_type": "application/json",
  "body_preview": "{\"incident_id\": \"rec-123\", \"remediation_id\": \"rem-456\", ...}"
}
2026-02-02 00:22:08,124 - incident.endpoint - INFO - {
  "event": "incident_analysis_requested",
  "user": "system:serviceaccount:...",
  "endpoint": "/incident/analyze"
}
2026-02-02 00:22:10,567 - INFO - POST /api/v1/incident/analyze 200 OK
```

### Bad Request (Body Parsing Error)

```
2026-02-02 00:17:29,042 - auth.middleware - INFO - token_validated
2026-02-02 00:17:29,044 - auth.middleware - INFO - sar_check_completed allowed=True
2026-02-02 00:17:29,045 - incident.endpoint - INFO - {
  "event": "incident_endpoint_request_received",
  "body_length": 423,
  "content_type": "application/json",
  "body_preview": "{\"incident_id\": \"inc-123\", ...}"
}
2026-02-02 00:17:29,046 - rfc7807 - WARNING - {
  "event": "starlette_http_exception",
  "status_code": 400,
  "detail": "There was an error parsing the body"
}
2026-02-02 00:17:29,046 - rfc7807 - INFO - {
  "event": "rfc7807_response_created",
  "request_id": "...",
  "status_code": 400,
  "response_type": "JSONResponse"
}
# ⚠️ 30-SECOND GAP HERE - WHY?
2026-02-02 00:17:59,198 - INFO - POST /api/v1/incident/analyze 401 Unauthorized
```

---

## ✅ **Success Criteria**

After analyzing debug logs, we should be able to answer:

1. ✅ Does request reach `/incident/analyze` endpoint?
2. ✅ What is body_length and content_type?
3. ✅ Is body content valid JSON?
4. ✅ Is RFC7807 response created with correct status_code?
5. ✅ What's the time gap between response creation and HTTP log?
6. ✅ Why does 400 become 401?

---

## 🚀 **Next Steps After Log Analysis**

### If Request Body is Valid → Pydantic Issue
- Check for Pydantic v2 strict mode conflicts
- Verify field types match OpenAPI spec exactly
- Consider adding Pydantic ValidationError logging

### If Response Created but Not Sent → Middleware Issue
- Remove BaseHTTPMiddleware (use pure ASGI)
- Check for blocking sync calls in async context
- Profile response transmission path

### If 400→401 Conversion → Framework Bug
- Verify Starlette/FastAPI versions
- Check for known issues with BaseHTTPMiddleware + exception handlers
- Consider workaround: Custom exception handler for 400 errors

---

## 📝 **Quick Command Reference**

```bash
# Run test
make test-e2e-holmesgpt-api 2>&1 | tee /tmp/hapi-debug-run.log

# Find latest must-gather
ls -lt /tmp/holmesgpt-api-e2e-logs-* | head -1

# Set log path
export HAPI_LOG="/tmp/holmesgpt-api-e2e-logs-$(date +%Y%m%d)*/holmesgpt-api-e2e-control-plane/pods/holmesgpt-api-e2e_holmesgpt-api-*/holmesgpt-api/0.log"

# Search debug logs
grep "incident_endpoint_request_received" $HAPI_LOG
grep "rfc7807_response_created" $HAPI_LOG
grep -E "rfc7807_response_created|POST /api/v1/incident" $HAPI_LOG

# Check timing
grep "00:17:29" $HAPI_LOG | grep -E "request_received|response_created|POST"
```
