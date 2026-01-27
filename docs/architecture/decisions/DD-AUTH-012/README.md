# DD-AUTH-012: ose-oauth-proxy for SAR-Based REST API Authorization - Document Index

**Status**: ✅ AUTHORITATIVE  
**Last Updated**: January 26, 2026  
**Category**: Authentication & Authorization

## Quick Links

### Core DD Document
- **[DD-AUTH-012: ose-oauth-proxy SAR for REST API Endpoints](DD-AUTH-012-ose-oauth-proxy-sar-rest-api-endpoints.md)** ⭐ **AUTHORITATIVE** - Technical decision to use OpenShift `ose-oauth-proxy` instead of CNCF `oauth2-proxy`

---

## Directory Structure

```
DD-AUTH-012/
├── README.md (this file)
├── DD-AUTH-012-ose-oauth-proxy-sar-rest-api-endpoints.md (AUTHORITATIVE)
└── DD-AUTH-012-IMPLEMENTATION-SUMMARY.md (Implementation status)
```

---

## Document Categories

### 📋 Core DD Document (1)
**AUTHORITATIVE** - Technical architecture decision.

- **DD-AUTH-012-ose-oauth-proxy-sar-rest-api-endpoints.md** - Authoritative document explaining why `ose-oauth-proxy` is used instead of CNCF `oauth2-proxy` for REST API authorization

### 📊 Implementation Summary (1)
Implementation status and completion summary.

- **DD-AUTH-012-IMPLEMENTATION-SUMMARY.md** - Concise summary of completed SAR implementation tasks

---

## Executive Summary

### Decision
Use OpenShift `ose-oauth-proxy` instead of CNCF `oauth2-proxy` for REST API endpoint authorization.

### Problem
CNCF `oauth2-proxy` **authenticates** ServiceAccount tokens but **cannot enforce Kubernetes RBAC authorization** for REST API endpoints because it lacks the `--openshift-sar` flag for SubjectAccessReview (SAR) checks.

### Solution
OpenShift `ose-oauth-proxy` supports `--openshift-sar` flag, enabling Kubernetes RBAC enforcement at the API gateway (sidecar) layer.

---

## Scope

### Services Covered
- **DataStorage Service**: REST API with `ose-oauth-proxy` sidecar (`verb:"create"`)
- **HolmesGPT API**: REST API with `ose-oauth-proxy` sidecar (`verb:"get"`)

### Key Differences: oauth2-proxy vs ose-oauth-proxy

| Feature | CNCF oauth2-proxy | OpenShift ose-oauth-proxy |
|---------|-------------------|---------------------------|
| **Authentication** | ✅ Yes (validates tokens) | ✅ Yes (validates tokens) |
| **Authorization (SAR)** | ❌ No (`--openshift-sar` flag missing) | ✅ Yes (`--openshift-sar` supported) |
| **RBAC Enforcement** | ❌ No (cannot check K8s RBAC) | ✅ Yes (performs K8s SAR checks) |
| **X-Auth-Request-User** | ✅ Yes (injects user identity) | ✅ Yes (injects user identity) |
| **Multi-Arch Support** | ✅ Yes (amd64, arm64) | ⚠️ Custom build required (arm64) |
| **Use Case** | Web apps with basic auth | REST APIs needing K8s RBAC |

---

## Technical Comparison

### CNCF oauth2-proxy Configuration (Attempted)
```yaml
# oauth2-proxy CAN authenticate tokens
args:
  - --provider=oidc
  - --oidc-issuer-url=https://kubernetes.default.svc
  - --pass-authorization-header=true
  - --set-xauthrequest=true
  # ❌ MISSING: --openshift-sar flag does NOT exist in oauth2-proxy
```

**Limitation**: oauth2-proxy stops at authentication. It cannot enforce RBAC authorization because it has no SAR capability.

### OpenShift ose-oauth-proxy Configuration (Implemented)
```yaml
# ose-oauth-proxy CAN authenticate AND authorize
args:
  - --provider=openshift
  - --upstream=http://localhost:8080
  - --pass-authorization-header=true
  - --set-xauthrequest=true
  # ✅ PRESENT: --openshift-sar performs K8s SubjectAccessReview
  - --openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"create"}
```

**Capability**: ose-oauth-proxy performs SAR check **before** proxying request. If SAR fails, returns **403 Forbidden**.

---

## Implementation Status

### DataStorage Service ✅ COMPLETE
| Component | Status | Configuration |
|---|---|---|
| **OAuth-Proxy Sidecar** | ✅ Deployed | `deploy/data-storage/deployment.yaml` |
| **SAR Configuration** | ✅ Configured | `verb:"create"` on `data-storage-service` resource |
| **X-Auth-Request-User** | ✅ Enabled | `--set-xauthrequest=true` |
| **ClusterRole** | ✅ Deployed | `deploy/data-storage/client-rbac-v2.yaml` |
| **E2E Tests** | 🚧 In Progress | `test/e2e/datastorage/23_sar_access_control_test.go` |

### HolmesGPT API ✅ COMPLETE
| Component | Status | Configuration |
|---|---|---|
| **OAuth-Proxy Sidecar** | ✅ Deployed | `deploy/holmesgpt-api/06-deployment.yaml` |
| **SAR Configuration** | ✅ Configured | `verb:"get"` on `holmesgpt-api` resource |
| **X-Auth-Request-User** | ✅ Enabled | `--set-xauthrequest=true` |
| **ClusterRole** | ✅ Deployed | `deploy/holmesgpt-api/03-rbac.yaml` |
| **E2E Tests** | 🚧 Pending | To be created |

---

## Related Design Decisions

### Parent DD
- **[DD-AUTH-011: Granular RBAC & SAR Verb Mapping](../DD-AUTH-011/DD-AUTH-011-granular-rbac-sar-verb-mapping.md)** - RBAC verb mapping strategy (led to creation of DD-AUTH-012)

### Child DD
- **[DD-AUTH-013: HTTP Status Codes for OAuth-Proxy](../DD-AUTH-013/DD-AUTH-013-http-status-codes-oauth-proxy.md)** - HTTP status codes returned by ose-oauth-proxy sidecar

### Related DDs
- **[DD-AUTH-009: OAuth2-Proxy Workflow Attribution](../DD-AUTH-009-oauth2-proxy-workflow-attribution-implementation.md)** - X-Auth-Request-User header injection for audit tracking
- **[DD-AUTH-006: HAPI OAuth-Proxy Configuration](../DD-AUTH-006-holmesgpt-api-oauth-proxy-config.md)** - HolmesGPT API sidecar config
- **[DD-AUTH-004: DataStorage Client Authentication](../DD-AUTH-004-datastorage-client-authentication-pattern.md)** - ServiceAccount authentication pattern

### Related ADRs
- **ADR-036**: Externalized Auth/Authz Sidecar Strategy

---

## Business Requirements

### DataStorage Service
- **BR-DATA-STORAGE-040**: RESTful API for audit events with RBAC
- **BR-DATA-STORAGE-041**: RESTful API for workflow catalog with RBAC
- **BR-DATA-STORAGE-050**: User attribution for SOC2 compliance
- **BR-SOC2-CC8.1**: Track user identity for workflow catalog operations

### HolmesGPT API
- **BR-HAPI-197**: RESTful API for incident analysis with RBAC
- **BR-HAPI-198**: RESTful API for recovery analysis with RBAC

---

## HTTP Headers Alignment

### X-Auth-Request-User Header

**Purpose**: Pass authenticated user/ServiceAccount identity to backend service.

**Format**:
```
X-Auth-Request-User: system:serviceaccount:kubernaut-system:aianalysis-controller
```

**Configuration**:
```yaml
args:
  - --set-xauthrequest=true  # Injects X-Auth-Request-User header
```

**Backend Usage**:
```go
// DataStorage extracts user identity from header
userID := r.Header.Get("X-Auth-Request-User")
// Store in audit log for SOC2 compliance
```

**Related**: DD-AUTH-009 (Workflow Attribution Implementation)

---

## SOC2 Compliance

### Workflow Catalog Attribution

**Requirement**: Track "who" accessed workflow catalog operations (create, search, update).

**Implementation**:
1. **Authentication**: ose-oauth-proxy validates ServiceAccount token
2. **Authorization**: ose-oauth-proxy performs K8s SAR check
3. **Attribution**: ose-oauth-proxy injects `X-Auth-Request-User` header
4. **Audit**: DataStorage extracts header and stores user identity in audit log

**Compliance**: Meets SOC2 Type II CC8.1 (logical access controls with attribution).

---

## Migration Path

### From oauth2-proxy to ose-oauth-proxy

**Step 1: Image Change**
```yaml
# BEFORE (oauth2-proxy - authentication only)
image: quay.io/oauth2-proxy/oauth2-proxy:v7.5.1

# AFTER (ose-oauth-proxy - authentication + authorization)
image: quay.io/jordigilh/ose-oauth-proxy:latest  # Dev/E2E (multi-arch)
image: quay.io/openshift/origin-oauth-proxy:latest  # Production (amd64)
```

**Step 2: Add SAR Configuration**
```yaml
args:
  # Existing args (kept)
  - --provider=openshift
  - --upstream=http://localhost:8080
  - --set-xauthrequest=true
  
  # NEW: Add SAR enforcement
  - --openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"create"}
```

**Step 3: Deploy ClusterRole**
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: data-storage-client
rules:
- apiGroups: [""]
  resources: ["services"]
  resourceNames: ["data-storage-service"]
  verbs: ["create"]
```

**Step 4: Bind ServiceAccounts**
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: aianalysis-data-storage-client
  namespace: kubernaut-system
subjects:
- kind: ServiceAccount
  name: aianalysis-controller
  namespace: kubernaut-system
roleRef:
  kind: ClusterRole
  name: data-storage-client
  apiGroup: rbac.authorization.k8s.io
```

---

## Testing

### E2E Test Scenarios

**DataStorage Service**:
- ✅ Test 1: ServiceAccount with `data-storage-client` role → 201 Created
- ✅ Test 2: ServiceAccount without role → 403 Forbidden
- ✅ Test 3: Workflow API with `data-storage-client` role → 201 Created
- 🚧 Test 4: Workflow API without role → 403 Forbidden (workflow types pending)
- 🚧 Test 5: Invalid token → 401 Unauthorized (pending)

**HolmesGPT API**:
- 🚧 Test 1: ServiceAccount with `holmesgpt-api-client` role → 200 OK
- 🚧 Test 2: ServiceAccount without role → 403 Forbidden
- 🚧 Test 3: Invalid token → 401 Unauthorized

---

## Key Learnings

### Critical Finding: oauth2-proxy Cannot Perform SAR

**Discovery**: During DD-AUTH-011 investigation (January 2026), we attempted to use CNCF `oauth2-proxy` for REST API authorization.

**Problem**:
- ❌ `oauth2-proxy:v7.5.1` lacks `--openshift-sar` flag
- ❌ Cannot enforce Kubernetes RBAC on REST API endpoints
- ❌ Only authenticates tokens, does not authorize access

**Solution**:
- ✅ Migrate to OpenShift `ose-oauth-proxy`
- ✅ Custom multi-arch build for dev/E2E: `quay.io/jordigilh/ose-oauth-proxy:latest`
- ✅ Production: `quay.io/openshift/origin-oauth-proxy:latest`

**Impact**: This finding led to the creation of DD-AUTH-012 as an authoritative document.

---

## Authority

**Status**: ✅ **AUTHORITATIVE**

This design decision is the **canonical reference** for the technical choice to use OpenShift `ose-oauth-proxy` instead of CNCF `oauth2-proxy` for REST API authorization in Kubernaut services.

**Supersedes**: No previous authoritative document existed for oauth-proxy selection.

**Referenced By**:
- DataStorage deployment (`deploy/data-storage/deployment.yaml`)
- HolmesGPT API deployment (`deploy/holmesgpt-api/06-deployment.yaml`)
- DD-AUTH-011 (Granular RBAC strategy)
- DD-AUTH-013 (HTTP Status Codes)

---

## Next Steps

### Pending Tasks
1. 🚧 Complete DataStorage E2E tests (workflow types fix)
2. 🚧 Add 401 Unauthorized test scenarios
3. 🚧 Create HolmesGPT API E2E auth validation tests
4. 🚧 Add NetworkPolicy examples to DD-AUTH-012
5. 🚧 Create production troubleshooting guide for SAR failures

### Future Enhancements (V1.1+)
1. Prometheus metrics for 401/403 rates
2. Helm chart configuration for production
3. Per-endpoint granular verb mapping (requires Envoy + Lua filters)
4. Alert rules for SAR failure rates

---

**Maintained By**: Platform Team  
**Contact**: See DD-AUTH-012 main document for author information  
**Last Review**: January 26, 2026  
**Next Review**: After E2E test completion
