# DD-AUTH-012: ose-oauth-proxy for SAR-Based REST API Authorization

**Date**: January 26, 2026
**Status**: ✅ **IMPLEMENTED**
**Version**: 1.0
**Authority**: AUTHORITATIVE - Technical Architecture Decision
**Related**: DD-AUTH-009 (oauth-proxy migration), DD-AUTH-011 (Granular RBAC), DD-AUTH-004 (DataStorage auth), DD-AUTH-006 (HAPI auth)

---

## 📋 **EXECUTIVE SUMMARY**

**Decision**: Use OpenShift `ose-oauth-proxy` instead of CNCF `oauth2-proxy` for REST API endpoint authorization because **oauth2-proxy cannot perform Kubernetes SubjectAccessReview (SAR) checks**.

**Problem**: CNCF `oauth2-proxy` authenticates ServiceAccount tokens but **cannot enforce RBAC authorization** for REST API endpoints.

**Solution**: OpenShift `ose-oauth-proxy` supports `--openshift-sar` flag, enabling Kubernetes RBAC enforcement at the API gateway layer.

---

## 🚨 **CRITICAL REQUIREMENT: SAR for REST API Endpoints**

### **Why SAR is Required**

Kubernetes RBAC natively protects:
- ✅ **CRD controllers**: K8s API enforces RBAC on CRD operations (CREATE, GET, UPDATE, DELETE)
- ✅ **K8s resources**: Pods, Services, ConfigMaps all protected by K8s RBAC
- ❌ **REST API endpoints**: HTTP endpoints on stateless services (DataStorage, HAPI) are **NOT** protected by K8s RBAC

**Gap**: DataStorage `/api/v1/workflows/*` endpoints are REST APIs, not K8s resources. Without SAR, **any authenticated user can access any endpoint** regardless of RBAC permissions.

### **SOC2 Requirement**

**BR-SOC2-CC8.1**: Track user identity for workflow catalog operations (create, search, update).

**Problem**: Without SAR enforcement, audit logs show "who accessed" but don't enforce "who can access".

**Solution**: ose-oauth-proxy performs SAR before proxying requests, ensuring RBAC is enforced.

---

## 🔍 **TECHNICAL COMPARISON**

### **CNCF oauth2-proxy Limitations**

```yaml
# oauth2-proxy CAN authenticate tokens
args:
  - --provider=oidc
  - --oidc-issuer-url=https://kubernetes.default.svc
  
# oauth2-proxy CANNOT perform SAR
# ❌ --openshift-sar flag does not exist in oauth2-proxy
# ❌ No RBAC enforcement for REST API endpoints
```

**Result**: Authenticated users can access all endpoints (no authorization enforcement).

---

### **OpenShift ose-oauth-proxy Solution**

```yaml
# ose-oauth-proxy CAN authenticate tokens
args:
  - --provider=openshift
  - --openshift-service-account=data-storage-sa
  
# ose-oauth-proxy CAN perform SAR (RBAC enforcement)
# ✅ --openshift-sar flag available
  - --openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"create"}
```

**Flow**:
1. Client sends request with ServiceAccount Bearer token
2. ose-oauth-proxy validates token (authentication)
3. ose-oauth-proxy performs SAR: "Can this ServiceAccount CREATE service/data-storage-service?" (authorization)
4. If SAR passes: Inject `X-Auth-Request-User` header, proxy to DataStorage
5. If SAR fails: Return 403 Forbidden

**Result**: RBAC permissions enforced at API gateway layer.

---

## 🎯 **SERVICES USING ose-oauth-proxy**

### **DataStorage** (Production + E2E)

**Production**: `deploy/data-storage/deployment.yaml`

```yaml
containers:
  - name: oauth-proxy
    image: quay.io/openshift/oauth-proxy:latest  # Official OpenShift image (amd64)
    args:
      - --provider=openshift
      - --openshift-service-account=data-storage-sa
      # DD-AUTH-011: SAR with verb:"create" (all services need audit write)
      - --openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"create"}
      # DD-AUTH-009: Inject X-Auth-Request-User for workflow catalog attribution
      - --set-xauthrequest=true
```

**E2E**: `test/infrastructure/datastorage.go` (line 1074)

```yaml
containers:
  - name: oauth2-proxy  # Named oauth2-proxy but uses ose-oauth-proxy image
    image: quay.io/jordigilh/ose-oauth-proxy:latest  # Custom multi-arch: arm64+amd64
    args:
      - --provider=oidc
      - --oidc-issuer-url=https://kubernetes.default.svc.cluster.local
      # DD-AUTH-011: SAR with verb:"create"
      - --openshift-sar={"namespace":"<namespace>","resource":"services","resourceName":"data-storage-service","verb":"create"}
      - --set-xauthrequest=true
```

**Why custom image for E2E**: OpenShift `origin-oauth-proxy` is amd64-only. Custom build adds arm64 support for local development on Apple Silicon.

---

### **HolmesGPT API** (Production)

**Production**: `deploy/holmesgpt-api/06-deployment.yaml`

```yaml
containers:
  - name: oauth-proxy
    image: quay.io/openshift/oauth-proxy:latest
    args:
      - --provider=openshift
      - --openshift-service-account=holmesgpt-api
      # DD-AUTH-006: SAR with verb:"get" (HAPI protects its own endpoints)
      - --openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"holmesgpt-api","verb":"get"}
      - --set-xauthrequest=true  # For LLM cost tracking
```

**Note**: HAPI uses `verb:"get"` because it's protecting **its own REST API endpoints** (not DataStorage). Only Gateway should access HAPI.

---

## 📊 **HTTP HEADER ALIGNMENT**

### **ose-oauth-proxy Header Injection**

Both `ose-oauth-proxy` (OpenShift) and `oauth2-proxy` (CNCF) inject the same header:

```
X-Auth-Request-User: system:serviceaccount:kubernaut-system:gateway-sa
```

**Format**: `system:serviceaccount:<namespace>:<serviceaccount-name>`

**Alignment**: DataStorage code extracts this header consistently across all endpoints.

---

### **DataStorage Header Extraction**

**Workflow Catalog Endpoints**: `pkg/datastorage/server/workflow_handlers.go`

```go
// NOTE: X-Auth-Request-User header extracted by audit event builder
// NewWorkflowCreatedAuditEvent() captures user attribution automatically
// Authority: DD-AUTH-009 (workflow catalog user attribution)
```

**Legal Hold Operations**: `pkg/datastorage/server/legal_hold_handler.go`

```go
// Extract X-Auth-Request-User header (placed_by) - REQUIRED for SOC2
// DD-AUTH-004: OAuth-proxy injects this header after validating JWT + SAR
placedBy := r.Header.Get("X-Auth-Request-User")
if placedBy == "" {
    return 401 Unauthorized  // Missing authentication
}
```

**Audit Export**: `pkg/datastorage/server/audit_export_handler.go`

```go
// Authentication: Require X-Auth-Request-User header (SOC2 CC8.1)
exportedBy := r.Header.Get("X-Auth-Request-User")
if exportedBy == "" {
    return 401 Unauthorized
}
```

**Consistency**: All DataStorage handlers use `X-Auth-Request-User` header, which is injected by **both** `ose-oauth-proxy` and `oauth2-proxy`.

---

## 🔒 **WORKFLOW CATALOG AUDIT TRACKING**

### **Implementation**

**File**: `pkg/datastorage/audit/workflow_catalog_event.go`

```go
// NewWorkflowCreatedAuditEvent creates audit event for workflow creation
// BR-STORAGE-183: Audit workflow catalog operations
// DD-AUDIT-002 V2.0.1: Workflow catalog = business logic (not pure CRUD)
//
// User Attribution: Extracted from X-Auth-Request-User header (DD-AUTH-009)
func NewWorkflowCreatedAuditEvent(workflow *models.Workflow) (*audit.AuditEvent, error) {
    // ... event creation ...
    // ActorID captured from workflow.CreatedBy (populated from header)
}
```

**File**: `pkg/datastorage/server/workflow_handlers.go`

```go
// POST /api/v1/workflows - Create workflow
func (h *WorkflowHandler) HandleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
    // 1. Create workflow in database
    workflow, err := h.catalogRepo.CreateWorkflow(ctx, &req)
    
    // 2. Audit workflow creation (async)
    if h.auditStore != nil {
        go func() {
            auditEvent, _ := dsaudit.NewWorkflowCreatedAuditEvent(&workflow)
            h.auditStore.StoreAudit(ctx, auditEvent)
        }()
    }
}
```

**Audit Events Captured**:
- `workflow.catalog.created` - Workflow created (captures `created_by` from header)
- `workflow.catalog.search_completed` - Workflow search (captures user performing search)
- `workflow.catalog.updated` - Workflow updated (captures `updated_by` from header)

---

### **User Attribution Flow**

```
1. Client (Gateway) → Request with Bearer token
2. ose-oauth-proxy → Validates token (authentication)
3. ose-oauth-proxy → Performs SAR (authorization)
4. ose-oauth-proxy → Injects X-Auth-Request-User: system:serviceaccount:kubernaut-system:gateway-sa
5. DataStorage → Extracts header, populates workflow.CreatedBy
6. DataStorage → Creates audit event with ActorID = gateway-sa
7. PostgreSQL → Stores audit event with user attribution
```

**SOC2 Compliance**: Audit logs now show:
- **Who** accessed the workflow catalog (via `actor_id` field)
- **What** operation was performed (`workflow.catalog.created`)
- **When** it occurred (`event_timestamp`)
- **Result** (`event_outcome`: success/failure)

---

## 🚧 **MIGRATION PATH (oauth2-proxy → ose-oauth-proxy)**

### **Phase 1: Integration Tests** ✅ COMPLETE

**Removed**: `oauth2-proxy` authentication middleware from services
- Caused K8s API throttling (TokenReview/SubjectAccessReview on every request)
- Integration tests hitting 503 errors due to rate limiting

**Authority**: `test/integration/gateway/K8S_API_THROTTLING_SOLUTIONS.md`

---

### **Phase 2: E2E Tests with Pass-Through** ✅ COMPLETE

**Added**: `oauth2-proxy` sidecar to E2E DataStorage deployments
- Used `--skip-auth-regex=.*` (pass-through mode)
- Validated sidecar architecture without SAR enforcement

**Authority**: TD-E2E-001 Phase 1

---

### **Phase 3: Real Authentication** ✅ COMPLETE

**Migrated**: `oauth2-proxy` → `ose-oauth-proxy` (custom multi-arch build)
- Reason: `oauth2-proxy` does **NOT** support `--openshift-sar` flag
- Removed pass-through mode (`--skip-auth-regex`)
- Enabled real ServiceAccount token validation + SAR enforcement

**Authority**: DD-AUTH-010 (E2E Real Authentication Mandate), DD-AUTH-011 (Granular RBAC)

---

### **Phase 4: Production Deployment** ✅ COMPLETE

**Updated**: Production deployments to use `ose-oauth-proxy`
- DataStorage: `deploy/data-storage/deployment.yaml` (updated today)
- HolmesGPT API: `deploy/holmesgpt-api/06-deployment.yaml` (already using ose-oauth-proxy)

**Authority**: DD-AUTH-004 (DataStorage), DD-AUTH-006 (HAPI), DD-AUTH-012 (this document)

---

## 📋 **VALIDATION COMMANDS**

### **Verify ose-oauth-proxy SAR Enforcement**

```bash
# 1. Check DataStorage deployment uses ose-oauth-proxy
kubectl get deployment data-storage-service -n kubernaut-system -o yaml | grep -A5 oauth-proxy

# Expected:
# - name: oauth-proxy
#   image: quay.io/openshift/oauth-proxy:latest
#   args:
#   - --openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"create"}

# 2. Test SAR enforcement (should get 403 without proper RBAC)
kubectl run test-pod --rm -it --image=curlimages/curl -- \
  curl -H "Authorization: Bearer $(kubectl create token test-sa)" \
  http://data-storage-service.kubernaut-system.svc:8080/api/v1/workflows

# Expected: 403 Forbidden (test-sa has no RBAC permissions)

# 3. Test with authorized ServiceAccount (should succeed)
kubectl run test-pod --rm -it --image=curlimages/curl -- \
  curl -H "Authorization: Bearer $(kubectl create token gateway-sa -n kubernaut-system)" \
  http://data-storage-service.kubernaut-system.svc:8080/api/v1/workflows

# Expected: 200 OK (gateway-sa has data-storage-client ClusterRole)
```

---

### **Verify X-Auth-Request-User Header Injection**

```bash
# Deploy a test pod that logs all headers
kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: header-logger
  namespace: kubernaut-system
spec:
  containers:
  - name: nginx
    image: nginx:alpine
    command: ["/bin/sh", "-c", "apk add --no-cache tcpdump && tcpdump -A -s 0 'tcp port 8081 and (((ip[2:2] - ((ip[0]&0xf)<<2)) - ((tcp[12]&0xf0)>>2)) != 0)' > /tmp/headers.log"]
EOF

# Send request through oauth-proxy
kubectl exec -n kubernaut-system gateway-pod -- \
  curl -H "Authorization: Bearer $(kubectl create token gateway-sa -n kubernaut-system)" \
  http://data-storage-service:8080/health

# Check captured headers
kubectl exec -n kubernaut-system header-logger -- cat /tmp/headers.log | grep X-Auth-Request-User

# Expected: X-Auth-Request-User: system:serviceaccount:kubernaut-system:gateway-sa
```

---

## 🎯 **SUCCESS CRITERIA**

- [x] DataStorage deployment uses `ose-oauth-proxy` with SAR enforcement
- [x] HAPI deployment uses `ose-oauth-proxy` with SAR enforcement
- [x] E2E tests use custom `ose-oauth-proxy` image (multi-arch)
- [x] SAR enforces `verb:"create"` for DataStorage (DD-AUTH-011)
- [x] SAR enforces `verb:"get"` for HAPI (DD-AUTH-006)
- [x] `X-Auth-Request-User` header injected consistently
- [x] Workflow catalog audit events capture user attribution
- [x] No `oauth2-proxy` references in production deployments

---

## 📚 **REFERENCES**

### **Internal Documents**
- **DD-AUTH-004**: DataStorage OAuth-Proxy Architecture (Legal Hold)
- **DD-AUTH-006**: HolmesGPT API OAuth-Proxy Integration
- **DD-AUTH-009**: OpenShift OAuth-Proxy Migration (v2.0 - ose-oauth-proxy)
- **DD-AUTH-010**: E2E Real Authentication Mandate
- **DD-AUTH-011**: Granular RBAC & SAR Verb Mapping
- **BR-SOC2-CC8.1**: User Attribution Requirements

### **External References**
- [OpenShift OAuth Proxy Documentation](https://github.com/openshift/oauth-proxy)
- [Kubernetes SubjectAccessReview API](https://kubernetes.io/docs/reference/access-authn-authz/authorization/#checking-api-access)
- [oauth2-proxy Limitations](https://oauth2-proxy.github.io/oauth2-proxy/docs/) (no SAR support)

---

## 🔗 **RELATED DESIGN DECISIONS**

| Decision | Relationship | Impact |
|----------|--------------|--------|
| **DD-AUTH-009** | Superseded by this document | Documented oauth2-proxy → ose-oauth-proxy migration |
| **DD-AUTH-011** | Implements RBAC verbs | Defines SAR verb mappings (`verb:"create"`) |
| **DD-AUTH-004** | DataStorage implementation | First service to use ose-oauth-proxy |
| **DD-AUTH-006** | HAPI implementation | Second service to use ose-oauth-proxy |
| **ADR-036** | High-level strategy | Defines network-level security approach |

---

**Document Version**: 1.0  
**Last Updated**: January 26, 2026  
**Status**: ✅ IMPLEMENTED - DataStorage and HAPI deployments updated  
**Next Review**: After V1.0 release (February 2026)
