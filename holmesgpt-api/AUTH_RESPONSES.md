# HolmesGPT API - Authentication & Authorization Responses

**Authority**: DD-AUTH-014 (Middleware-Based SAR Authentication)

This document describes the HTTP status codes and responses returned by the authentication/authorization middleware protecting all HolmesGPT API endpoints.

---

## 🔒 **Protected Endpoints**

All API endpoints except the following are protected by authentication and authorization:

- `/health` - Health check (liveness probe)
- `/ready` - Readiness check
- `/metrics` - Prometheus metrics
- `/docs` - OpenAPI documentation (dev mode only)
- `/redoc` - ReDoc documentation (dev mode only)
- `/openapi.json` - OpenAPI spec (dev mode only)

---

## 📋 **HTTP Status Codes**

### **401 Unauthorized**

Returned when the Bearer token is missing, invalid, or cannot be authenticated.

**Trigger Conditions**:
- Missing `Authorization` header
- `Authorization` header does not start with "Bearer "
- Token validation fails (Kubernetes TokenReview API returns `authenticated: false`)
- Kubernetes TokenReview API call fails

**Response Format** (RFC 7807 Problem Details):
```json
{
  "type": "about:blank",
  "title": "Unauthorized",
  "status": 401,
  "detail": "Missing Authorization header with Bearer token"
}
```

**Content-Type**: `application/problem+json`

**Example Error Messages**:
- `"Missing Authorization header with Bearer token"`
- `"Token not authenticated: <error from K8s>"`
- `"Token authenticated but user identity is empty"`

---

### **403 Forbidden**

Returned when the Bearer token is valid but the user lacks the required RBAC permissions.

**Trigger Conditions**:
- Token is authenticated (via TokenReview)
- Kubernetes SubjectAccessReview (SAR) API returns `allowed: false`
- User does not have the required RBAC verb permission on the service resource

**Response Format** (RFC 7807 Problem Details):
```json
{
  "type": "about:blank",
  "title": "Forbidden",
  "status": 403,
  "detail": "Insufficient RBAC permissions for POST /api/v1/recovery"
}
```

**Content-Type**: `application/problem+json`

**Example Error Messages**:
- `"Insufficient RBAC permissions for POST /api/v1/recovery"`
- `"Insufficient RBAC permissions for GET /api/v1/incident"`

**Required Permissions**:
See [client-rbac.yaml](../deploy/holmesgpt-api/client-rbac.yaml) for the ClusterRole definition.

---

### **500 Internal Server Error**

Returned when the Kubernetes TokenReview or SubjectAccessReview API calls fail unexpectedly.

**Trigger Conditions**:
- Kubernetes API server is unavailable
- Network timeout during TokenReview/SAR calls
- Unexpected error in auth middleware

**Response Format** (RFC 7807 Problem Details):
```json
{
  "type": "about:blank",
  "title": "Internal Server Error",
  "status": 500,
  "detail": "Authorization check failed: <error message>"
}
```

**Content-Type**: `application/problem+json`

**Example Error Messages**:
- `"Token validation failed: <K8s API error>"`
- `"Authorization check failed: <K8s API error>"`
- `"Unexpected error during token validation: <error>"`

---

## 🔑 **Authentication Flow**

1. **Client** sends request with `Authorization: Bearer <token>` header
2. **Middleware** extracts Bearer token from header
3. **TokenReview** validates token via Kubernetes TokenReview API
   - Returns user identity (e.g., `system:serviceaccount:namespace:sa-name`)
4. **SAR** checks authorization via Kubernetes SubjectAccessReview API
   - Checks if user can perform verb on `services/holmesgpt-api-service`
   - Verb is mapped from HTTP method (GET→get, POST→create, etc.)
5. **User Attribution** injected into `request.state.user` for audit logging
6. **Request** forwarded to handler if authentication and authorization succeed

---

## 📊 **HTTP Method to RBAC Verb Mapping**

Authority: DD-AUTH-014 (Granular RBAC SAR Verb Mapping)

| HTTP Method | RBAC Verb | Example Endpoint |
|-------------|-----------|------------------|
| GET         | `get`     | GET /api/v1/recovery |
| POST        | `create`  | POST /api/v1/recovery |
| PUT/PATCH   | `update`  | PUT /api/v1/recovery (if exists) |
| DELETE      | `delete`  | DELETE /api/v1/recovery (if exists) |

---

## 🧪 **Testing**

### **Integration Tests**

Integration tests use **mock implementations** that do not make real Kubernetes API calls:

```python
from src.auth import MockAuthenticator, MockAuthorizer

app.add_middleware(
    AuthenticationMiddleware,
    authenticator=MockAuthenticator(
        valid_users={"test-token": "system:serviceaccount:test:sa"}
    ),
    authorizer=MockAuthorizer(default_allow=True),
    config={...}
)
```

Set `ENV_MODE=integration` to enable mock auth.

### **E2E Tests**

E2E tests use **real Kubernetes APIs** in Kind clusters:

1. Create ServiceAccount for E2E client
2. Grant `holmesgpt-api-client` ClusterRole permissions
3. Get ServiceAccount Bearer token
4. Configure HTTP client with `Authorization: Bearer <token>` header

See [client-rbac.yaml](../deploy/holmesgpt-api/client-rbac.yaml) for required permissions.

---

## 🔗 **Related Documentation**

- [DD-AUTH-014: Middleware-Based SAR Authentication](../docs/architecture/decisions/DD-AUTH-014/)
- [Service RBAC](../deploy/holmesgpt-api/service-rbac.yaml) - ServiceAccount + permissions for middleware
- [Client RBAC](../deploy/holmesgpt-api/client-rbac.yaml) - Permissions for E2E test clients
- [Auth Interfaces](src/auth/interfaces.py) - Python Protocol definitions
- [Auth Middleware](src/middleware/auth.py) - Middleware implementation

---

## 🛡️ **Security**

**Zero Trust**: All endpoints (except public health/metrics) require authentication and authorization.

**No Runtime Disable**: Auth cannot be disabled via config flags (security risk). Mock implementations are injected via `ENV_MODE` for testing.

**SOC2 Compliance**: User attribution is tracked via `request.state.user` for audit logging (CC8.1). Handlers can access the authenticated user via `get_authenticated_user(request)` from `src.middleware.user_context`.

---

**Authority**: DD-AUTH-014, BR-HAPI-067, BR-HAPI-068, BR-HAPI-200 (RFC 7807)
