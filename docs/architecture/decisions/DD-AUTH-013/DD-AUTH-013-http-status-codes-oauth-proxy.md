# DD-AUTH-013: HTTP Status Codes for ose-oauth-proxy Authentication & Authorization

**Date**: January 26, 2026  
**Status**: ✅ **AUTHORITATIVE**  
**Version**: 1.0  
**Authority**: Defines HTTP status codes returned by ose-oauth-proxy sidecar

---

## 🎯 **OBJECTIVE**

Document all HTTP status codes returned by `ose-oauth-proxy` sidecar for DataStorage and HolmesGPT API REST endpoints, ensuring OpenAPI spec accuracy and E2E test coverage.

---

## 📋 **HTTP STATUS CODES**

### **🟢 Success Responses (2xx)**

These are returned by **DataStorage/HAPI**, not the proxy:

| Code | Name | Source | Description |
|------|------|--------|-------------|
| **200 OK** | Success | DataStorage/HAPI | Request succeeded (GET, PUT, DELETE) |
| **201 Created** | Created | DataStorage/HAPI | Resource created successfully (POST) |
| **202 Accepted** | Accepted | DataStorage | Async processing (DLQ fallback per DD-009) |

---

### **🔴 Client Error Responses (4xx)**

#### **401 Unauthorized** ⚠️ **AUTHENTICATION FAILURE**

**Source**: `ose-oauth-proxy` sidecar  
**Cause**: Invalid or missing Bearer token

**Scenarios**:
1. **No Authorization header**:
   ```bash
   curl http://datastorage:8080/api/v1/workflows
   # Response: 401 Unauthorized
   ```

2. **Invalid Bearer token**:
   ```bash
   curl -H "Authorization: Bearer invalid-token" http://datastorage:8080/api/v1/workflows
   # Response: 401 Unauthorized
   ```

3. **Expired token**:
   ```bash
   curl -H "Authorization: Bearer expired-jwt" http://datastorage:8080/api/v1/workflows
   # Response: 401 Unauthorized
   ```

4. **Malformed token**:
   ```bash
   curl -H "Authorization: Bearer not-a-jwt" http://datastorage:8080/api/v1/workflows
   # Response: 401 Unauthorized
   ```

**OAuth-Proxy Behavior**:
- Validates Bearer token via Kubernetes `TokenReview` API
- If validation fails, returns 401 **before** checking authorization
- **Does not** reach DataStorage/HAPI application code

**Response Format**:
```html
HTTP/1.1 401 Unauthorized
Content-Type: text/html

<html>
<head><title>401 Authorization Required</title></head>
<body>
<center><h1>401 Authorization Required</h1></center>
</body>
</html>
```

**Authority**: OAuth-proxy standard behavior, OpenShift OAuth documentation

---

#### **403 Forbidden** ⚠️ **AUTHORIZATION FAILURE**

**Source**: `ose-oauth-proxy` sidecar  
**Cause**: Valid token but failed Kubernetes SubjectAccessReview (SAR)

**Scenarios**:
1. **ServiceAccount lacks RBAC permission**:
   ```bash
   # ServiceAccount has no RoleBinding to data-storage-client ClusterRole
   TOKEN=$(kubectl create token test-sa -n kubernaut-system)
   curl -H "Authorization: Bearer $TOKEN" http://datastorage:8080/api/v1/workflows
   # Response: 403 Forbidden
   ```

2. **Insufficient RBAC verb**:
   ```bash
   # ServiceAccount has "get" permission but SAR checks for "create"
   TOKEN=$(kubectl create token readonly-sa -n kubernaut-system)
   curl -H "Authorization: Bearer $TOKEN" http://datastorage:8080/api/v1/workflows
   # Response: 403 Forbidden
   ```

3. **Wrong namespace**:
   ```bash
   # ServiceAccount in different namespace without cross-namespace RBAC
   TOKEN=$(kubectl create token gateway-sa -n other-namespace)
   curl -H "Authorization: Bearer $TOKEN" http://datastorage:8080/api/v1/workflows
   # Response: 403 Forbidden
   ```

**OAuth-Proxy SAR Check**:
```yaml
--openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"create"}
```

**Translates to**:
```bash
kubectl auth can-i create services/data-storage-service \
  --as=system:serviceaccount:kubernaut-system:gateway-sa \
  -n kubernaut-system
# If "no", returns 403 Forbidden
```

**Response Format**:
```html
HTTP/1.1 403 Forbidden
Content-Type: text/html

<html>
<head><title>403 Forbidden</title></head>
<body>
<center><h1>403 Forbidden</h1></center>
</body>
</html>
```

**Authority**: DD-AUTH-011 (Granular RBAC), DD-AUTH-012 (ose-oauth-proxy SAR)

---

#### **400 Bad Request** ⚠️ **VALIDATION ERROR**

**Source**: **DataStorage/HAPI** (application code)  
**Cause**: Invalid request payload or parameters

**Response Format** (RFC 7807 Problem Details):
```json
{
  "type": "about:blank",
  "title": "Bad Request",
  "status": 400,
  "detail": "Invalid workflow: missing required field 'workflow_name'"
}
```

**Note**: This is **after** auth/authz succeeds (proxy passes request through).

---

#### **404 Not Found**

**Source**: **DataStorage/HAPI** (application code)  
**Cause**: Resource not found

**Example**:
```bash
curl -H "Authorization: Bearer $VALID_TOKEN" \
  http://datastorage:8080/api/v1/workflows/nonexistent-id
# Response: 404 Not Found
```

---

### **🔴 Server Error Responses (5xx)**

#### **500 Internal Server Error**

**Source**: **DataStorage/HAPI** (application code)  
**Cause**: Unexpected server error (after auth/authz succeeds)

**Response Format** (RFC 7807):
```json
{
  "type": "about:blank",
  "title": "Internal Server Error",
  "status": 500,
  "detail": "Database connection failed"
}
```

---

#### **503 Service Unavailable**

**Source**: **DataStorage/HAPI** (application code)  
**Cause**: Service temporarily unavailable (health check failing)

**Example**: Database connection pool exhausted, Redis unavailable

---

## 📊 **AUTHENTICATION & AUTHORIZATION FLOW**

```
┌─────────┐     ┌──────────────────┐     ┌─────────────┐
│ Client  │────▶│ ose-oauth-proxy  │────▶│ DataStorage │
└─────────┘     └──────────────────┘     └─────────────┘
                        │
                        ├─ Step 1: TokenReview (Authentication)
                        │  ├─ Valid → Continue
                        │  └─ Invalid → Return 401 Unauthorized
                        │
                        ├─ Step 2: SubjectAccessReview (Authorization)
                        │  ├─ Allowed → Inject X-Auth-Request-User, proxy request
                        │  └─ Denied → Return 403 Forbidden
                        │
                        └─ Step 3: Proxy to backend
                           └─ Backend returns 200/201/400/500 (application logic)
```

---

## 🔧 **OPENAPI SPEC REQUIREMENTS**

### **All Protected Endpoints Must Document:**

1. **401 Unauthorized** (authentication failure)
2. **403 Forbidden** (authorization failure - SAR denied)
3. **400 Bad Request** (validation error - application)
4. **500 Internal Server Error** (server error - application)

### **Example (Audit Write API)**:

```yaml
/api/v1/audit/events:
  post:
    responses:
      '201':
        description: Audit event created successfully
      '401':
        description: Authentication failed (invalid/missing Bearer token)
        content:
          text/html:
            schema:
              type: string
      '403':
        description: Authorization failed (Kubernetes SAR denied access)
        content:
          text/html:
            schema:
              type: string
      '400':
        description: Validation error
        content:
          application/problem+json:
            schema:
              $ref: '#/components/schemas/RFC7807Problem'
      '500':
        description: Internal server error
        content:
          application/problem+json:
            schema:
              $ref: '#/components/schemas/RFC7807Problem'
```

---

## 🧪 **E2E TEST COVERAGE REQUIREMENTS**

### **Mandatory Test Scenarios:**

| Scenario | Expected Status | Validates |
|----------|----------------|-----------|
| **No Bearer token** | 401 Unauthorized | Authentication required |
| **Invalid Bearer token** | 401 Unauthorized | Token validation |
| **Valid token, no RBAC** | 403 Forbidden | SAR enforcement |
| **Valid token, insufficient verb** | 403 Forbidden | Granular RBAC (verb:"create") |
| **Valid token, correct RBAC** | 200/201 | Successful auth + authz |
| **Invalid payload** | 400 Bad Request | Application validation |

**Authority**: DD-AUTH-010 (E2E Real Authentication), DD-AUTH-011-E2E-TESTING-GUIDE

---

## ❌ **HTTP CODES NOT USED**

### **402 Payment Required**

**Status**: Reserved, not used in Kubernetes auth flows  
**Reason**: Historical placeholder, no standard implementation  
**Not applicable** to ose-oauth-proxy authentication/authorization

---

## 📋 **VALIDATION COMMANDS**

### **Test 401 Unauthorized (No Token)**

```bash
curl -v http://localhost:28090/api/v1/workflows

# Expected:
# < HTTP/1.1 401 Unauthorized
# < WWW-Authenticate: Bearer realm="kubernetes"
```

---

### **Test 401 Unauthorized (Invalid Token)**

```bash
curl -v -H "Authorization: Bearer invalid-token" \
  http://localhost:28090/api/v1/workflows

# Expected:
# < HTTP/1.1 401 Unauthorized
```

---

### **Test 403 Forbidden (No RBAC)**

```bash
# Create ServiceAccount without RBAC
kubectl create sa test-unauthorized -n datastorage-e2e
TOKEN=$(kubectl create token test-unauthorized -n datastorage-e2e)

curl -v -H "Authorization: Bearer $TOKEN" \
  http://localhost:28090/api/v1/workflows

# Expected:
# < HTTP/1.1 403 Forbidden
```

---

### **Test 403 Forbidden (Insufficient RBAC Verb)**

```bash
# Create ServiceAccount with "get" permission (insufficient for "create" SAR check)
kubectl create sa test-readonly -n datastorage-e2e
kubectl create rolebinding test-readonly --clusterrole=data-storage-read-only -n datastorage-e2e --serviceaccount=datastorage-e2e:test-readonly

TOKEN=$(kubectl create token test-readonly -n datastorage-e2e)

curl -v -H "Authorization: Bearer $TOKEN" \
  -X POST -H "Content-Type: application/json" \
  -d '{"workflow_name":"test"}' \
  http://localhost:28090/api/v1/workflows

# Expected:
# < HTTP/1.1 403 Forbidden
```

---

### **Test 200 OK (Authorized)**

```bash
# Create ServiceAccount with correct RBAC
kubectl create sa test-authorized -n datastorage-e2e
kubectl create rolebinding test-authorized --clusterrole=data-storage-client -n datastorage-e2e --serviceaccount=datastorage-e2e:test-authorized

TOKEN=$(kubectl create token test-authorized -n datastorage-e2e)

curl -v -H "Authorization: Bearer $TOKEN" \
  http://localhost:28090/api/v1/workflows

# Expected:
# < HTTP/1.1 200 OK
```

---

## 📚 **RELATED DOCUMENTS**

### **Authentication & Authorization**
- **DD-AUTH-009**: OAuth-Proxy Workflow Attribution (v2.0 - ose-oauth-proxy migration)
- **DD-AUTH-010**: E2E Real Authentication Mandate
- **DD-AUTH-011**: Granular RBAC & SAR Verb Mapping
- **DD-AUTH-012**: ose-oauth-proxy for SAR REST API Endpoints

### **Testing**
- **DD-AUTH-011-E2E-TESTING-GUIDE**: E2E test execution guide
- **DD-AUTH-013**: HTTP Status Codes (this document)

### **Business Requirements**
- **BR-SECURITY-016**: Kubernetes RBAC Enforcement
- **BR-SOC2-CC8.1**: User Attribution Requirements

---

## ✅ **SUMMARY**

### **HTTP Status Codes to Document in OpenAPI Spec:**

| Code | Source | Meaning | OpenAPI Required |
|------|--------|---------|------------------|
| **401** | ose-oauth-proxy | Authentication failed | ✅ YES |
| **403** | ose-oauth-proxy | Authorization failed (SAR) | ✅ YES |
| **400** | Application | Validation error | ✅ YES |
| **404** | Application | Resource not found | ⚠️ Per endpoint |
| **500** | Application | Server error | ✅ YES |
| **402** | N/A | Not used | ❌ NO |

### **E2E Tests Must Cover:**
- ✅ 401 (no token, invalid token)
- ✅ 403 (no RBAC, insufficient verb)
- ✅ 200/201 (authorized)
- ✅ 400 (validation error)

---

**Document Version**: 1.0  
**Last Updated**: January 26, 2026  
**Status**: ✅ AUTHORITATIVE  
**Next Action**: Update OpenAPI spec with 401 responses
