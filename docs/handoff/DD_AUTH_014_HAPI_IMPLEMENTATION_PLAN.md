# DD-AUTH-014: HAPI SAR Implementation Plan

**Date**: 2026-01-26
**Status**: Planning Phase
**Authority**: DD-AUTH-014 (Middleware-Based SAR Authentication)
**POC**: DataStorage (100% pass, Zero Trust enforced)

---

## 🎯 **Objective**

Extend DD-AUTH-014 middleware-based SAR authentication to HolmesGPT API (HAPI), completing the original scope (DS + HAPI).

---

## 📋 **Current State Analysis**

### **HAPI Auth Middleware Status**

**Existing Implementation** (`holmesgpt-api/src/middleware/auth.py`):
- ✅ AuthenticationMiddleware exists
- ✅ TokenReview API integration working
- ❌ **Marked as DEPRECATED** (line 20: "DEPRECATED in favor of oauth-proxy")
- ❌ **Disabled** (auth_enabled=False)
- ❌ **No SAR authorization** (line 424: `return True` - no RBAC check)
- ❌ **No X-Auth-Request-User injection**

**Key Finding**: HAPI has TokenReview but missing SAR + user attribution

---

## 🚀 **Implementation Phases**

### **Phase 1: Python Auth Interfaces** (TDD RED)

Create Python equivalents of Go auth interfaces for dependency injection.

**Files to Create**:
```
holmesgpt-api/src/auth/
├── __init__.py
├── interfaces.py       # Authenticator, Authorizer protocols
├── k8s_auth.py        # Production: K8s TokenReview + SAR
└── mock_auth.py       # Testing: Mock implementations
```

**`interfaces.py`** (Python Protocols):
```python
from typing import Protocol

class Authenticator(Protocol):
    """Validates Bearer tokens using K8s TokenReview API"""
    async def validate_token(self, token: str) -> str:
        """Returns username if valid, raises HTTPException if not"""
        ...

class Authorizer(Protocol):
    """Checks authorization using K8s SubjectAccessReview API"""
    async def check_access(
        self, 
        user: str, 
        namespace: str, 
        resource: str, 
        resource_name: str, 
        verb: str
    ) -> bool:
        """Returns True if authorized, False otherwise"""
        ...
```

**Authority**: DD-AUTH-014 Go implementation (proven pattern)

---

### **Phase 2: Update Auth Middleware** (TDD GREEN)

Modify `holmesgpt-api/src/middleware/auth.py` to:
1. Accept `Authenticator` and `Authorizer` via dependency injection
2. Call SAR after TokenReview
3. Inject `X-Auth-Request-User` header
4. Return RFC 7807 Problem Details for 401/403/500

**Changes**:
```python
class AuthenticationMiddleware(BaseHTTPMiddleware):
    def __init__(self, app, authenticator: Authenticator, authorizer: Authorizer, config: Dict[str, Any]):
        super().__init__(app)
        self.authenticator = authenticator
        self.authorizer = authorizer
        self.config = config
        # ... rest of initialization
    
    async def dispatch(self, request: Request, call_next):
        # 1. Authenticate (TokenReview via authenticator)
        user = await self.authenticator.validate_token(token)
        
        # 2. Authorize (SAR via authorizer)
        allowed = await self.authorizer.check_access(
            user, 
            self.config["namespace"], 
            self.config["resource"], 
            self.config["resource_name"], 
            self.config["verb"]
        )
        if not allowed:
            return JSONResponse(
                status_code=403,
                content={
                    "type": "about:blank",
                    "title": "Forbidden",
                    "status": 403,
                    "detail": f"Insufficient RBAC permissions"
                }
            )
        
        # 3. Inject X-Auth-Request-User header (for user attribution)
        request.headers.__dict__["_list"].append(
            (b"x-auth-request-user", user.encode("utf-8"))
        )
        
        # 4. Continue to handler
        return await call_next(request)
```

---

### **Phase 3: Main App Integration** (TDD GREEN)

Update `holmesgpt-api/src/main.py` to inject auth components.

**Changes**:
```python
from src.auth.k8s_auth import K8sAuthenticator, K8sAuthorizer
from src.auth.mock_auth import MockAuthenticator, MockAuthorizer

# Production: Real K8s APIs
if os.getenv("ENV") == "production":
    from kubernetes import client, config
    config.load_incluster_config()
    k8s_client = client.CoreV1Api()
    
    authenticator = K8sAuthenticator(k8s_client)
    authorizer = K8sAuthorizer(k8s_client)

# Integration/E2E: Mock implementations
else:
    authenticator = MockAuthenticator()
    authorizer = MockAuthorizer()

# Apply middleware with dependency injection
app.add_middleware(
    AuthenticationMiddleware,
    authenticator=authenticator,
    authorizer=authorizer,
    config={
        "namespace": os.getenv("POD_NAMESPACE", "kubernaut-system"),
        "resource": "services",
        "resource_name": "holmesgpt-api-service",
        "verb": "create",  # Adjust per endpoint if needed
    }
)
```

---

### **Phase 4: RBAC Provisioning**

Create RBAC for HAPI service + E2E client.

**Files to Create**:
```
deploy/holmesgpt-api/
├── service-rbac.yaml       # HAPI ServiceAccount + permissions
└── client-rbac.yaml        # E2E test client permissions
```

**`service-rbac.yaml`**:
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: holmesgpt-api-sa
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: holmesgpt-api-auth-middleware
rules:
- apiGroups: ["authentication.k8s.io"]
  resources: ["tokenreviews"]
  verbs: ["create"]
- apiGroups: ["authorization.k8s.io"]
  resources: ["subjectaccessreviews"]
  verbs: ["create"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: holmesgpt-api-auth-middleware
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: holmesgpt-api-auth-middleware
subjects:
- kind: ServiceAccount
  name: holmesgpt-api-sa
  namespace: kubernaut-system
```

**`client-rbac.yaml`**:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: holmesgpt-api-client
rules:
- apiGroups: [""]
  resources: ["services"]
  resourceNames: ["holmesgpt-api-service"]
  verbs: ["create", "get", "list", "update", "delete"]  # Full CRUD
```

**Authority**: DataStorage RBAC pattern (proven)

---

### **Phase 5: OpenAPI Spec Update**

Add 401/403 responses to HAPI OpenAPI spec (if exists).

**Pattern** (from DataStorage):
```yaml
responses:
  '200':
    description: Success
  '401':
    description: Unauthorized - Missing/invalid Bearer token
    content:
      application/problem+json:
        schema:
          $ref: '#/components/schemas/RFC7807Problem'
  '403':
    description: Forbidden - Insufficient RBAC permissions
    content:
      application/problem+json:
        schema:
          $ref: '#/components/schemas/RFC7807Problem'
  '500':
    description: Internal Server Error - TokenReview/SAR API failure
    content:
      application/problem+json:
        schema:
          $ref: '#/components/schemas/RFC7807Problem'
```

**Note**: If HAPI doesn't have OpenAPI spec, document HTTP status codes in README

---

### **Phase 6: E2E Test Refactoring**

Update HAPI E2E tests to use authenticated clients.

**Changes**:
1. **Create shared authenticated client** in E2E suite setup
2. **Provision ServiceAccount** with `holmesgpt-api-client` permissions
3. **Configure HTTP client** with ServiceAccount Bearer token
4. **Update all tests** to use shared authenticated client

**Pattern** (from DataStorage):
```python
# In E2E suite setup
from test.shared.auth.serviceaccount_transport import ServiceAccountTransport

# Provision ServiceAccount
sa_token = get_serviceaccount_token("holmesgpt-api-e2e-client")

# Create authenticated client
transport = ServiceAccountTransport(sa_token)
hapi_client = HolmesGPTAPIClient(
    base_url="http://localhost:28091",
    transport=transport
)

# Export for all tests
HAPI_CLIENT = hapi_client
```

---

## ✅ **Validation Checklist**

### **Code Quality**
- [ ] Python auth interfaces follow Go pattern
- [ ] Dependency injection working (production + test)
- [ ] X-Auth-Request-User header injected
- [ ] RFC 7807 error responses

### **Security**
- [ ] No runtime disable flags (security risk)
- [ ] TokenReview validates all tokens
- [ ] SAR checks all requests
- [ ] Zero Trust enforced on all endpoints

### **Testing**
- [ ] Unit tests for auth components
- [ ] Integration tests use mock auth
- [ ] E2E tests use real TokenReview + SAR
- [ ] 100% E2E pass rate

### **Operations**
- [ ] HAPI ServiceAccount has auth middleware permissions
- [ ] E2E client has full CRUD permissions
- [ ] API server load acceptable (monitor)
- [ ] Metrics for auth failures/successes

---

## 📊 **Effort Estimate**

| Phase | Effort | Risk |
|-------|--------|------|
| 1. Python Auth Interfaces | 4-6 hours | Low (copy from DS) |
| 2. Update Auth Middleware | 4-6 hours | Low (proven pattern) |
| 3. Main App Integration | 2-4 hours | Low (straightforward) |
| 4. RBAC Provisioning | 2-3 hours | Low (copy from DS) |
| 5. OpenAPI Spec Update | 2-3 hours | Low (if spec exists) |
| 6. E2E Test Refactoring | 6-8 hours | Medium (unknown test count) |
| **Total** | **2-3 days** | **Low** |

---

## ⚠️ **Risks & Mitigations**

### **Risk 1: Python asyncio vs Go sync**
**Impact**: Medium - Python HTTP is async, Go is sync
**Mitigation**: Use `aiohttp` for K8s API calls (already used in existing middleware)

### **Risk 2: HAPI E2E test count unknown**
**Impact**: Medium - May have many tests to refactor
**Mitigation**: Use same pattern as DS (global authenticated client)

### **Risk 3: HAPI doesn't have OpenAPI spec**
**Impact**: Low - Can document HTTP codes in README
**Mitigation**: Check for spec first, document if missing

### **Risk 4: Python type hints for dependency injection**
**Impact**: Low - Python Protocols work like Go interfaces
**Mitigation**: Use `typing.Protocol` (Python 3.8+)

---

## 🔄 **Rollback Plan**

If HAPI integration fails:
1. **Revert middleware changes** (git revert)
2. **Keep oauth-proxy deprecated state** (don't re-enable)
3. **Document lessons learned** in DD-AUTH-014
4. **Evaluate alternative** (OAuth-proxy re-enablement)

**Confidence**: 90% - Pattern proven in DataStorage

---

## 📝 **Success Criteria**

**HAPI integration is successful if**:
1. ✅ All E2E tests passing (100%)
2. ✅ Zero Trust enforced (0 unauthenticated requests)
3. ✅ TokenReview + SAR working in Python
4. ✅ X-Auth-Request-User header injection working
5. ✅ No regression in HAPI functionality

---

## 🔗 **Related Documentation**

- [DD-AUTH-014 Final Summary](./DD_AUTH_014_FINAL_SUMMARY.md)
- [DD-AUTH-014 Next Steps](./DD_AUTH_014_NEXT_STEPS.md)
- [DataStorage Auth Middleware](../../pkg/datastorage/server/middleware/auth.go)
- [DataStorage Auth Interfaces](../../pkg/shared/auth/interfaces.go)

---

## 🚦 **Decision Gates**

### **Gate 1: After Phase 3** (Main App Integration)
**Question**: Does HAPI compile and start with new middleware?
- **Yes** → Continue to Phase 4 (RBAC)
- **No** → Debug, escalate if needed

### **Gate 2: After Phase 4** (RBAC Provisioning)
**Question**: Can HAPI pod perform TokenReview + SAR?
- **Yes** → Continue to Phase 5 (OpenAPI)
- **No** → Fix RBAC, validate with `kubectl auth can-i`

### **Gate 3: After Phase 6** (E2E Tests)
**Question**: Do all HAPI E2E tests pass (100%)?
- **Yes** → **HAPI COMPLETE** ✅
- **No** → Triage failures (same process as DS)

---

## 📚 **Implementation Sequence**

1. **Day 1 Morning**: Phase 1-2 (Python auth interfaces + middleware update)
2. **Day 1 Afternoon**: Phase 3-4 (Main app integration + RBAC)
3. **Day 2 Morning**: Phase 5-6 (OpenAPI + E2E test refactoring)
4. **Day 2 Afternoon**: Validation + documentation
5. **Day 3**: Buffer for debugging + final validation

---

## 🎯 **Definition of Done**

- [ ] Python auth interfaces implemented
- [ ] HAPI middleware updated with SAR
- [ ] X-Auth-Request-User header injection
- [ ] RBAC provisioned (service + client)
- [ ] OpenAPI spec updated (or documented)
- [ ] E2E tests refactored to use authenticated clients
- [ ] **100% E2E pass rate**
- [ ] Zero Trust enforced
- [ ] Documentation updated

---

**Ready to proceed with Phase 1?**
