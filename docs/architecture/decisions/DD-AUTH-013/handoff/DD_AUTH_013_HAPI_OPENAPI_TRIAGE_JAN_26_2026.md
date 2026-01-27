# DD-AUTH-013: HolmesGPT API OpenAPI Spec Triage Report

**Date**: January 26, 2026  
**Status**: 🚨 **ACTION REQUIRED**  
**Version**: 1.0  
**Authority**: Extends DD-AUTH-013 to HolmesGPT API

---

## 🎯 **OBJECTIVE**

Triage HolmesGPT API (HAPI) OpenAPI spec to identify missing HTTP status codes from ose-oauth-proxy authentication/authorization flow.

---

## 📋 **CURRENT STATE**

### **OpenAPI Spec Location**
```
holmesgpt-api/api/openapi.json
```

### **Current HTTP Status Codes**
```json
{
  "200": "Successful Response",
  "422": "Validation Error"  // FastAPI convention
}
```

### **OAuth-Proxy Configuration** (Per DD-AUTH-006)
```yaml
# deploy/holmesgpt-api/06-deployment.yaml
- --openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"holmesgpt-api","verb":"get"}
- --set-xauthrequest=true
```

**SAR Check**: `verb:"get"` on `holmesgpt-api` resource  
**Purpose**: Only Gateway can call HAPI (not "create" like DataStorage)

---

## 🚨 **MISSING HTTP STATUS CODES**

### **❌ 401 Unauthorized** - MISSING

**Source**: ose-oauth-proxy (authentication failure)  
**Scenarios**:
1. No Authorization header
2. Invalid Bearer token
3. Expired token
4. Malformed token

**Impact**: Clients cannot distinguish authentication vs authorization failures

---

### **❌ 403 Forbidden** - MISSING

**Source**: ose-oauth-proxy (authorization failure - SAR denied)  
**Scenarios**:
1. ServiceAccount lacks "get" permission on holmesgpt-api resource
2. Wrong namespace
3. ServiceAccount not in `holmesgpt-api-client` ClusterRole

**Impact**: Clients receive generic error instead of clear authorization failure

---

### **❌ 500 Internal Server Error** - MISSING

**Source**: HAPI application (server error)  
**Scenarios**:
1. LLM provider API failure
2. Database connection error
3. Unhandled exception

**Impact**: No documentation for server-side errors

---

## 📊 **ENDPOINT ANALYSIS**

### **Protected Endpoints** (Behind OAuth-Proxy)

| Endpoint | Method | Auth Required | Current Status Codes | Missing Codes |
|----------|--------|---------------|---------------------|---------------|
| `/api/v1/incident/analyze` | POST | ✅ YES | 200, 422 | ❌ 401, 403, 500 |
| `/api/v1/recovery/analyze` | POST | ✅ YES | 200, 422 | ❌ 401, 403, 500 |

---

### **Unprotected Endpoints** (Health Checks)

| Endpoint | Method | Auth Required | Current Status Codes | Status |
|----------|--------|---------------|---------------------|--------|
| `/health` | GET | ❌ NO | 200 | ✅ OK |
| `/ready` | GET | ❌ NO | 200 | ✅ OK |
| `/config` | GET | ❌ NO | 200 | ✅ OK |

**Note**: Health check endpoints should NOT have 401/403 (Kubernetes probes don't authenticate)

---

## 🔍 **DETAILED FINDINGS**

### **Finding 1: FastAPI Uses 422 Instead of 400**

**Current**: `422 Unprocessable Entity` for validation errors  
**DataStorage**: `400 Bad Request` for validation errors

**Recommendation**: Keep 422 for HAPI (FastAPI convention), but add note in documentation

**Rationale**:
- FastAPI distinguishes between:
  - **400**: Malformed request (invalid JSON)
  - **422**: Valid JSON but fails validation (Pydantic)
- This is a FastAPI standard, don't change it

---

### **Finding 2: No Error Response Schema**

**Current**: No `HTTPValidationError` schema examples in documentation  
**DataStorage**: Uses RFC 7807 Problem Details

**Recommendation**: Document FastAPI's HTTPValidationError format

---

### **Finding 3: Health Endpoints Are Unprotected**

**Current**: Health endpoints return `200` only  
**Status**: ✅ CORRECT - should NOT have auth errors

**Verification**:
```bash
# Health checks work without token (as expected)
curl http://holmesgpt-api:8081/health
curl http://holmesgpt-api:8081/ready
```

**Authority**: Kubernetes probes run without authentication

---

## ✅ **RECOMMENDED CHANGES**

### **Change 1: Add 401 Unauthorized**

**Affected Endpoints**:
- `POST /api/v1/incident/analyze`
- `POST /api/v1/recovery/analyze`

**Response Format**:
```json
{
  "401": {
    "description": "Authentication failed - Invalid or missing Bearer token.\n\n**Source**: ose-oauth-proxy sidecar (DD-AUTH-013)\n\n**Authority**: DD-AUTH-013",
    "content": {
      "text/html": {
        "schema": {
          "type": "string",
          "example": "<html><body><h1>401 Authorization Required</h1></body></html>"
        }
      }
    }
  }
}
```

---

### **Change 2: Add 403 Forbidden**

**Affected Endpoints**:
- `POST /api/v1/incident/analyze`
- `POST /api/v1/recovery/analyze`

**Response Format**:
```json
{
  "403": {
    "description": "Authorization failed - Kubernetes SubjectAccessReview (SAR) denied access.\n\n**Source**: ose-oauth-proxy sidecar (DD-AUTH-006, DD-AUTH-013)\n\n**Cause**: ServiceAccount lacks 'get' permission on holmesgpt-api resource.\n\n**Authority**: DD-AUTH-006, DD-AUTH-013",
    "content": {
      "text/html": {
        "schema": {
          "type": "string",
          "example": "<html><body><h1>403 Forbidden</h1></body></html>"
        }
      }
    }
  }
}
```

---

### **Change 3: Add 500 Internal Server Error**

**Affected Endpoints**:
- `POST /api/v1/incident/analyze`
- `POST /api/v1/recovery/analyze`

**Response Format**:
```json
{
  "500": {
    "description": "Internal server error - Unexpected failure in HAPI service.\n\n**Causes**: LLM provider API failure, database error, unhandled exception.\n\n**Authority**: DD-AUTH-013",
    "content": {
      "application/json": {
        "schema": {
          "$ref": "#/components/schemas/HTTPValidationError"
        }
      }
    }
  }
}
```

---

## 📊 **COMPARISON: DataStorage vs HAPI**

| Aspect | DataStorage | HAPI | Notes |
|--------|------------|------|-------|
| **OpenAPI Format** | YAML | JSON | Both valid |
| **SAR Verb** | `create` | `get` | Different use cases |
| **Validation Error** | 400 | 422 | FastAPI convention |
| **401 Response** | ❌ Missing (fixed) | ❌ Missing | Both need update |
| **403 Response** | ❌ Missing (fixed) | ❌ Missing | Both need update |
| **500 Response** | ✅ Documented | ❌ Missing | HAPI needs |
| **Error Format** | RFC 7807 | HTTPValidationError | Different frameworks |

---

## 🧪 **E2E TEST REQUIREMENTS**

### **Mandatory Test Scenarios** (Per DD-AUTH-013):

| Scenario | Expected Status | Test File | Status |
|----------|----------------|-----------|--------|
| **No Bearer token** | 401 Unauthorized | ⏳ TODO | ❌ Not implemented |
| **Invalid Bearer token** | 401 Unauthorized | ⏳ TODO | ❌ Not implemented |
| **Valid token, no RBAC** | 403 Forbidden | ⏳ TODO | ❌ Not implemented |
| **Valid token, wrong verb** | 403 Forbidden | ⏳ TODO | ❌ Not implemented |
| **Valid token, correct RBAC** | 200 | ⏳ TODO | ⚠️  May exist in integration tests |
| **Invalid payload** | 422 | ⏳ TODO | ⚠️  May exist in unit tests |

**E2E Coverage**: 0% for auth failure scenarios

---

## 📋 **VALIDATION COMMANDS**

### **Test 401 Unauthorized (No Token)**

```bash
curl -v http://localhost:28080/api/v1/incident/analyze \
  -X POST \
  -H "Content-Type: application/json" \
  -d '{}'

# Expected:
# < HTTP/1.1 401 Unauthorized
```

---

### **Test 403 Forbidden (No RBAC)**

```bash
# Create ServiceAccount without RBAC
kubectl create sa test-unauthorized -n kubernaut-system
TOKEN=$(kubectl create token test-unauthorized -n kubernaut-system)

curl -v -H "Authorization: Bearer $TOKEN" \
  -X POST \
  -H "Content-Type: application/json" \
  http://localhost:28080/api/v1/incident/analyze \
  -d '{}'

# Expected:
# < HTTP/1.1 403 Forbidden
```

---

### **Test 200 OK (Authorized - Gateway SA)**

```bash
# Gateway SA has holmesgpt-api-client ClusterRole
TOKEN=$(kubectl create token gateway-sa -n kubernaut-system)

curl -v -H "Authorization: Bearer $TOKEN" \
  -X POST \
  -H "Content-Type: application/json" \
  http://localhost:28080/api/v1/incident/analyze \
  -d '{"alert_name":"test","namespace":"default"}'

# Expected:
# < HTTP/1.1 200 OK
```

---

## ✅ **ACTION ITEMS**

### **Priority 1: Update OpenAPI Spec** ⏳ IN PROGRESS

- [ ] Add 401 response to `/api/v1/incident/analyze`
- [ ] Add 401 response to `/api/v1/recovery/analyze`
- [ ] Add 403 response to `/api/v1/incident/analyze`
- [ ] Add 403 response to `/api/v1/recovery/analyze`
- [ ] Add 500 response to `/api/v1/incident/analyze`
- [ ] Add 500 response to `/api/v1/recovery/analyze`
- [ ] Regenerate HAPI client (ogen)

---

### **Priority 2: Add E2E Tests** ⏳ TODO

- [ ] Create `test/e2e/holmesgpt-api/auth_validation_test.go`
- [ ] Test 401 scenarios (no token, invalid token)
- [ ] Test 403 scenarios (no RBAC, wrong verb)
- [ ] Test 200 scenarios (authorized Gateway SA)

---

### **Priority 3: Documentation** ⏳ TODO

- [ ] Add HAPI example to DD-AUTH-013
- [ ] Update DD-AUTH-006 with HTTP status code references
- [ ] Document FastAPI 422 vs REST 400 convention

---

## 📚 **RELATED DOCUMENTS**

### **Authentication & Authorization**
- **DD-AUTH-006**: HolmesGPT API OAuth-Proxy Integration
- **DD-AUTH-013**: HTTP Status Codes for OAuth-Proxy (authoritative)
- **DD-AUTH-011**: Granular RBAC & SAR Verb Mapping

### **HAPI Documentation**
- **BR-HAPI-001**: Recovery analysis endpoint
- **BR-HAPI-002**: Incident analysis endpoint
- **BR-HAPI-126/127/128**: Health check endpoints

---

## 🎯 **SUMMARY**

### **Current State**
- ✅ OAuth-proxy correctly configured (DD-AUTH-006)
- ❌ OpenAPI spec missing 401/403/500 responses
- ❌ No E2E tests for auth failure scenarios
- ⚠️  Uses FastAPI 422 convention (OK, but document it)

### **Impact**
- **Medium**: Clients lack proper error handling for auth failures
- **Low**: HAPI is internal (only Gateway calls it), not user-facing
- **High**: API contract incomplete for E2E validation

### **Next Action**
Update `holmesgpt-api/api/openapi.json` with 401/403/500 responses

---

**Document Version**: 1.0  
**Last Updated**: January 26, 2026  
**Status**: 🚨 ACTION REQUIRED  
**Confidence**: 100% (validated against deployment config)
