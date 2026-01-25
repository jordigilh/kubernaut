# HolmesGPT API OAuth-Proxy Triage - Phase 9

**Date**: January 7, 2026
**Requestor**: User
**Triager**: AI Assistant
**Status**: Analysis Complete - Awaiting Decision

---

## 📋 **Executive Summary**

HolmesGPT API **already implements ServiceAccount token authentication** via custom Python middleware. Adding oauth-proxy would provide consistency with DataStorage but introduces trade-offs.

**Recommendation**: **Option B** - Keep existing auth, enhance with X-Auth-Request-User header support for consistency.

---

## 🔍 **Current State Analysis**

### **Existing Authentication**
Location: `holmesgpt-api/src/middleware/auth.py`

**Current Flow:**
```
Client → Service:8080 → holmesgpt-api:8080 (Python middleware validates token)
```

**Features:**
- ✅ Validates Kubernetes ServiceAccount tokens via TokenReviewer API
- ✅ Returns HTTP 401/403 for unauthorized requests
- ✅ Supports dev mode (test tokens)
- ✅ Retry logic with exponential backoff
- ✅ Metrics recording (auth successes/failures)
- ✅ Role-based access control (RBAC)

**Public Endpoints** (no auth required):
- `/health`, `/ready`, `/metrics`, `/docs`, `/redoc`, `/openapi.json`

**Protected Endpoints** (auth required):
- `POST /incident/*` - Incident analysis
- `POST /recovery/*` - Recovery suggestions
- `POST /postexec/*` - Post-execution analysis

### **Deployment Configuration**
Location: `deploy/holmesgpt-api/06-deployment.yaml`

**Current Setup:**
- Single container (no sidecar)
- Port 8080 (HTTP)
- ServiceAccount: `holmesgpt-api`
- Image: `quay.io/jordigilh/kubernaut-holmesgpt-api:v1.0.4-amd64`

---

## ⚖️ **OAuth-Proxy Options**

### **Option A: Add OAuth-Proxy Sidecar (Full Migration)**

**Implementation:**
1. Add oauth-proxy sidecar to deployment (similar to DataStorage)
2. **Remove** existing Python authentication middleware
3. Update port routing: Service:8080 → oauth-proxy:8080 → holmesgpt-api:8081
4. Update endpoints to extract `X-Auth-Request-User` header

**Pros:**
- ✅ Consistency with DataStorage authentication pattern
- ✅ Centralized authentication (no custom middleware needed)
- ✅ Simplified Python codebase (remove ~400 lines of auth code)
- ✅ Unified authentication pattern across all services
- ✅ OAuth-proxy handles token caching and validation efficiently

**Cons:**
- ❌ **Deployment complexity**: Adds sidecar container (+200MB memory, 50m CPU)
- ❌ **Latency increase**: Extra hop (5-10ms per request)
- ❌ **Migration effort**: 8 hours (deployment + middleware removal + testing)
- ❌ **Test changes required**: Integration tests need mock transport updates
- ❌ **Redundant with existing solution**: Current auth works well
- ❌ **Breaking change**: Existing clients may be affected

**Effort Estimate**: 8-10 hours
- Deployment updates: 2 hours
- Middleware removal: 3 hours
- Integration test fixes: 2 hours
- E2E testing: 1 hour

---

### **Option B: Keep Existing Auth, Enhance for Consistency (Recommended)**

**Implementation:**
1. **Keep** existing Python authentication middleware
2. **Add** `X-Auth-Request-User` header injection to middleware (for consistency)
3. **Document** authentication pattern in OpenAPI spec
4. **Update** integration tests to use consistent header format

**Pros:**
- ✅ **Zero deployment changes**: No sidecar, no port changes
- ✅ **Minimal effort**: ~2 hours of work
- ✅ **Existing solution works**: No need to fix what isn't broken
- ✅ **Consistent header format**: Aligns with DataStorage pattern
- ✅ **No breaking changes**: Backward compatible
- ✅ **Internal service**: Network policies already handle access control

**Cons:**
- ❌ **Not identical to DataStorage**: Different auth implementation
- ❌ **Custom middleware maintenance**: Continued ownership of auth code
- ❌ **Potential drift**: Two auth patterns in the codebase

**Effort Estimate**: 2 hours
- Middleware enhancement: 30 minutes
- OpenAPI documentation: 30 minutes
- Integration test updates: 30 minutes
- Testing: 30 minutes

---

### **Option C: Do Nothing (Not Recommended)**

**Implementation:**
- No changes

**Pros:**
- ✅ Zero effort
- ✅ Zero risk

**Cons:**
- ❌ **Inconsistent with DataStorage**: Different authentication patterns
- ❌ **Missed opportunity**: Could simplify codebase
- ❌ **Future tech debt**: Two auth patterns to maintain

---

## 🎯 **Recommendation: Option B**

**Rationale:**

1. **Existing auth is robust**: The current Python middleware is production-ready with retry logic, metrics, and proper error handling.

2. **Internal service**: HolmesGPT API is internal-only (not exposed externally), so network policies already provide access control.

3. **No SOC2 compliance needs**: Unlike DataStorage (which needs user attribution for legal hold), HolmesGPT API doesn't have SOC2 compliance requirements for user tracking.

4. **Cost-benefit**: Adding oauth-proxy provides consistency but at the cost of deployment complexity, resource usage, and migration effort.

5. **Header consistency**: By adding `X-Auth-Request-User` header injection to the existing middleware, we achieve consistency with DataStorage without the overhead of oauth-proxy.

---

## 📊 **Comparison Matrix**

| Criteria | Option A (OAuth-Proxy) | Option B (Enhance Existing) | Option C (Do Nothing) |
|----------|------------------------|-----------------------------|-----------------------|
| **Effort** | High (8-10 hours) | Low (2 hours) | None |
| **Deployment Complexity** | High (sidecar) | Low (none) | None |
| **Resource Usage** | High (+200MB, +50m CPU) | Low (none) | None |
| **Consistency with DS** | Full | Partial (header format) | None |
| **Breaking Changes** | Yes | No | No |
| **Test Changes** | Extensive | Minimal | None |
| **Maintenance** | OAuth-proxy team | Internal | Internal |
| **Security Posture** | Centralized | Custom (robust) | Custom (robust) |

---

## 🚀 **Next Steps (If Option B Approved)**

### **Phase 9.1: Enhance Middleware (30 minutes)**
1. Update `holmesgpt-api/src/middleware/auth.py`:
   - After successful token validation, inject `X-Auth-Request-User` header
   - Format: `system:serviceaccount:<namespace>:<name>`
   - Example: `system:serviceaccount:kubernaut-system:gateway-service`

2. Update endpoints to extract user from `request.state.user.username` or header

### **Phase 9.2: Document Authentication (30 minutes)**
1. Update `holmesgpt-api/api/openapi.json`:
   - Add comprehensive authentication documentation
   - Document `X-Auth-Request-User` header usage
   - Link to DD-AUTH-005 for consistency

2. Create DD-AUTH-006 design decision document (optional)

### **Phase 9.3: Update Integration Tests (30 minutes)**
1. Update test clients to use consistent header format
2. Verify authentication failures return proper HTTP 401/403

### **Phase 9.4: Testing (30 minutes)**
1. Run integration tests
2. Run E2E tests
3. Verify metrics recording

---

## 📚 **Related Documentation**

- **DataStorage OAuth-Proxy**: `deploy/data-storage/deployment.yaml`
- **DD-AUTH-004**: OAuth-proxy sidecar design for DataStorage
- **DD-AUTH-005**: Client authentication pattern (7 Go services + 1 Python)
- **Current Auth Middleware**: `holmesgpt-api/src/middleware/auth.py`

---

## 🤔 **Questions for User**

1. **SOC2 Compliance**: Does HolmesGPT API need user attribution for compliance like DataStorage?
2. **Consistency vs. Simplicity**: Do you prefer full consistency (Option A) or pragmatic consistency (Option B)?
3. **Resource Constraints**: Are sidecar resource costs (+200MB, +50m CPU per pod) acceptable?
4. **Migration Timing**: If Option A, can we defer to v1.1 after initial release?
5. **Breaking Changes**: Are breaking changes acceptable at this stage (pre-release)?

---

## ✅ **Triage Complete**

**Status**: Awaiting user decision
**Recommended**: Option B (Enhance existing auth)
**Alternative**: Option A (OAuth-proxy sidecar) if full consistency is required
**Priority**: Low (not blocking, existing auth works)

**Ready for**: User decision and approval to proceed

