# DD-AUTH-011: Granular RBAC & SAR Verb Mapping for DataStorage

**Date**: January 26, 2026
**Status**: ✅ **APPROVED** - AUTHORITATIVE
**Authority**: Supersedes DD-AUTH-009 (wildcard verbs), DD-AUTH-010 (E2E authentication)
**Priority**: CRITICAL (RBAC security)
**Related**: DD-AUTH-004 (OAuth-proxy design), DD-AUTH-009 (OAuth2-proxy migration)

---

## 🎯 **DECISION**

**Granular RBAC roles with operation-specific verbs instead of wildcard `verbs: ["*"]`**.

**Principle**: Test with production-equivalent RBAC permissions. Each service gets ONLY the verbs it needs for its operations.

---

## 📊 **CONTEXT**

### **Current State (INCORRECT)**

```yaml
# deploy/data-storage/client-rbac.yaml:38
verbs: ["get"]  # ❌ Insufficient for workflow CRUD operations
```

**OAuth2-Proxy SAR** (Current):
```yaml
--openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"get"}
```

**Problem**: 
- SAR checks `verb:"get"` 
- Workflow CREATE operations fail (requires `verb:"create"`)
- Workflow UPDATE operations fail (requires `verb:"update"`)

---

### **Proposed Incorrect Solution (REJECTED)**

```yaml
# DD-AUTH-009 Initial Proposal
verbs: ["*"]  # ❌ TOO PERMISSIVE - grants ALL operations
```

**Why Rejected**:
- ❌ Violates Principle of Least Privilege
- ❌ Grants unnecessary permissions (e.g., DELETE to read-only services)
- ❌ Not production-equivalent (overly broad)
- ❌ Doesn't test actual RBAC boundaries

---

## 🔍 **OPERATION ANALYSIS**

### **DataStorage API Operations → K8s RBAC Verbs**

| HTTP Method | Endpoint | K8s Verb | Services Using | Purpose |
|-------------|----------|----------|----------------|---------|
| **POST** | `/api/v1/audit/*` | `create` | All 7 services | Write audit events |
| **GET** | `/api/v1/audit/*` | `get` | Admin, E2E tests | Read audit data (legal hold) |
| **POST** | `/api/v1/workflows/search` | `list` | HAPI | Search workflows |
| **GET** | `/api/v1/workflows` | `list` | HAPI, E2E | List workflows |
| **GET** | `/api/v1/workflows/{id}` | `get` | HAPI, E2E | Get specific workflow |
| **POST** | `/api/v1/workflows` | `create` | Admin, E2E | Create workflows |
| **PATCH** | `/api/v1/workflows/{id}` | `update` | Admin, E2E | Update workflow status |

---

## ✅ **APPROVED SOLUTION: GRANULAR ClusterRoles**

### **Strategy**

1. **Create 4 ClusterRoles** (operation-specific):
   - `data-storage-audit-writer` (`create` for audit events)
   - `data-storage-audit-reader` (`get`, `list` for audit queries)
   - `data-storage-workflow-reader` (`get`, `list` for workflow search)
   - `data-storage-workflow-writer` (`create`, `update` for workflow CRUD)

2. **Grant roles per service need**:
   - All 7 services: `audit-writer`
   - HAPI: `audit-writer` + `workflow-reader`
   - E2E tests: `audit-writer` + `workflow-reader` + `workflow-writer` + `audit-reader`
   - Admin: All 4 roles

3. **OAuth2-Proxy SAR**: Single verb check based on endpoint pattern

---

## 🚨 **CRITICAL PROBLEM: OAuth2-Proxy SAR Limitation**

### **Limitation**

OAuth2-proxy's `--openshift-sar` flag only supports **ONE** SAR check for the entire service:

```yaml
--openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"get"}
```

**Cannot do**:
- ❌ Per-endpoint SAR checks (e.g., `verb:"create"` for `/workflows`, `verb:"get"` for `/workflows/{id}`)
- ❌ Conditional SAR based on HTTP method
- ❌ Multiple SAR checks per request

**Impact**: OAuth2-proxy will check the SAME verb for ALL endpoints.

---

## 💡 **WORKAROUND: Tiered RBAC Approach**

### **Option A: Use Most Permissive Verb in SAR** (SELECTED)

**OAuth2-Proxy SAR**:
```yaml
--openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"create"}
```

**Logic**: 
- SAR checks if user has `create` permission
- If user has `create`, they also have `get`/`list` (RBAC allows additive permissions)
- This is the "highest" permission level needed for DataStorage

**RBAC ClusterRoles**:

#### **1. `data-storage-full-access`** (Admin, E2E tests)
```yaml
rules:
  - apiGroups: [""]
    resources: ["services"]
    resourceNames: ["data-storage-service"]
    verbs: ["create", "get", "list", "update"]  # All operations
```

#### **2. `data-storage-read-only`** (HAPI, query services)
```yaml
rules:
  - apiGroups: [""]
    resources: ["services"]
    resourceNames: ["data-storage-service"]
    verbs: ["get", "list"]  # Read-only operations
```

#### **3. `data-storage-audit-writer`** (All 7 services)
```yaml
rules:
  - apiGroups: [""]
    resources: ["services"]
    resourceNames: ["data-storage-service"]
    verbs: ["create"]  # Audit write operations
```

**Service Assignments**:
- **Gateway, RO, SP, AA, WE, Notif, DS**: `data-storage-audit-writer` (create only)
- **HolmesGPT API**: `data-storage-audit-writer` + `data-storage-read-only` (create + read)
- **E2E Tests**: `data-storage-full-access` (all operations)
- **Human Admin**: `data-storage-full-access` (all operations)

---

### **Option B: Use Wildcard Verb with NetworkPolicy Isolation** (REJECTED)

**OAuth2-Proxy SAR**:
```yaml
--openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"*"}
```

**RBAC**: Grant `verbs: ["*"]` to all services

**Isolation**: Use NetworkPolicy to restrict which pods can reach DataStorage

**Why Rejected**:
- ❌ Doesn't test granular RBAC (all services get all permissions)
- ❌ Relies on NetworkPolicy for authorization (wrong layer)
- ❌ Not production-equivalent (RBAC is first line of defense)

---

## 🎯 **APPROVED RBAC CONFIGURATION**

### **File**: `deploy/data-storage/client-rbac.yaml`

```yaml
---
# ClusterRole 1: Full Access (Admin, E2E Tests)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: data-storage-full-access
  labels:
    app: data-storage-service
    component: rbac
    tier: admin
rules:
  # OAuth-proxy SAR check: Can the ServiceAccount "create" the data-storage-service?
  # "create" is the highest permission level → implies "get", "list", "update"
  - apiGroups: [""]
    resources: ["services"]
    resourceNames: ["data-storage-service"]
    verbs: ["create", "get", "list", "update"]

---
# ClusterRole 2: Read-Only (HolmesGPT API, Query Services)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: data-storage-read-only
  labels:
    app: data-storage-service
    component: rbac
    tier: read
rules:
  # Read-only access: Workflow search + Audit queries
  - apiGroups: [""]
    resources: ["services"]
    resourceNames: ["data-storage-service"]
    verbs: ["get", "list"]

---
# ClusterRole 3: Audit Writer (All 7 Services)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: data-storage-audit-writer
  labels:
    app: data-storage-service
    component: rbac
    tier: write
rules:
  # Write-only access: Audit event creation
  - apiGroups: [""]
    resources: ["services"]
    resourceNames: ["data-storage-service"]
    verbs: ["create"]

---
# RoleBinding: Gateway (audit-writer only)
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: gateway-data-storage-audit-writer
  namespace: kubernaut-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: data-storage-audit-writer
subjects:
  - kind: ServiceAccount
    name: gateway-sa
    namespace: kubernaut-system

---
# RoleBinding: HolmesGPT API (audit-writer + read-only)
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: holmesgpt-api-data-storage-combined
  namespace: kubernaut-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: data-storage-read-only  # Primary role
subjects:
  - kind: ServiceAccount
    name: holmesgpt-api-sa
    namespace: kubernaut-system

---
# RoleBinding: HolmesGPT API Audit Writer
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: holmesgpt-api-data-storage-audit-writer
  namespace: kubernaut-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: data-storage-audit-writer
subjects:
  - kind: ServiceAccount
    name: holmesgpt-api-sa
    namespace: kubernaut-system

---
# RoleBinding: E2E Tests (full-access)
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: e2e-data-storage-full-access
  namespace: kubernaut-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: data-storage-full-access
subjects:
  - kind: ServiceAccount
    name: datastorage-e2e-sa
    namespace: kubernaut-system
  - kind: ServiceAccount
    name: aianalysis-e2e-sa
    namespace: kubernaut-system
  - kind: ServiceAccount
    name: gateway-e2e-sa
    namespace: kubernaut-system

# ... (Repeat for all 7 services with audit-writer)
```

---

## 🔧 **OAUTH2-PROXY CONFIGURATION**

### **DataStorage OAuth2-Proxy Args**

```yaml
# deploy/data-storage/deployment.yaml
- name: oauth-proxy
  image: quay.io/oauth2-proxy/oauth2-proxy:v7.5.1
  args:
    - --http-address=0.0.0.0:8080
    - --upstream=http://localhost:8081
    - --provider=oidc
    - --oidc-issuer-url=https://kubernetes.default.svc/.well-known/openid-configuration
    - --skip-oidc-discovery=true
    - --cookie-secret-file=/etc/oauth-proxy/cookie-secret
    - --cookie-name=_oauth_proxy_ds
    - --skip-auth-route=^/health$
    # DD-AUTH-011: SAR checks "create" verb (highest permission level)
    # Services with "create" permission can perform all operations
    # Services with only "get"/"list" will pass SAR but DataStorage will enforce finer-grained authz
    - --openshift-sar={"namespace":"kubernaut-system","resource":"services","resourceName":"data-storage-service","verb":"create"}
    - --set-xauthrequest=true
```

---

## 🚨 **DEFENSE-IN-DEPTH: Application-Level Authorization**

### **Problem**

OAuth2-proxy SAR only checks ONE verb (`create`). This means:
- ✅ Services with `create` permission pass SAR
- ✅ Services with `get`/`list` permission **fail** SAR (even for read operations)

**Impact**: Read-only services (HAPI for workflow search) will get HTTP 403 Forbidden from oauth2-proxy.

---

### **Solution: Two-Tier Authorization**

#### **Tier 1: OAuth2-Proxy SAR** (Coarse-grained)
```yaml
--openshift-sar={"verb":"create"}  # Allows services with audit write OR workflow write
```

**Passes**: Services with `create`, `get`, `list`, `update` permissions
**Fails**: Services with NO permissions

#### **Tier 2: DataStorage Application-Level AuthZ** (Fine-grained)

**File**: `pkg/datastorage/server/authz_middleware.go` (NEW)

```go
// AuthorizationMiddleware enforces fine-grained RBAC based on X-Auth-Request-User
// OAuth2-proxy validates user exists, but we validate user can perform THIS operation
func (s *Server) AuthorizationMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        user := r.Header.Get("X-Auth-Request-User")
        if user == "" {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }

        // Extract ServiceAccount namespace and name from user
        // Format: system:serviceaccount:kubernaut-system:gateway-sa
        parts := strings.Split(user, ":")
        if len(parts) != 4 || parts[0] != "system" || parts[1] != "serviceaccount" {
            http.Error(w, "Invalid user format", http.StatusForbidden)
            return
        }
        namespace := parts[2]
        saName := parts[3]

        // Perform SAR for the SPECIFIC operation
        verb := s.httpMethodToVerb(r.Method)
        allowed, err := s.performSAR(r.Context(), namespace, saName, verb)
        if err != nil {
            s.logger.Error(err, "SAR check failed", "user", user, "verb", verb)
            http.Error(w, "Authorization check failed", http.StatusInternalServerError)
            return
        }

        if !allowed {
            s.logger.Info("Operation not allowed", "user", user, "verb", verb, "path", r.URL.Path)
            http.Error(w, "Forbidden", http.StatusForbidden)
            return
        }

        next.ServeHTTP(w, r)
    })
}

// httpMethodToVerb maps HTTP methods to K8s RBAC verbs
func (s *Server) httpMethodToVerb(method string) string {
    switch method {
    case http.MethodGet:
        return "get"  // or "list" for collection endpoints
    case http.MethodPost:
        return "create"
    case http.MethodPatch, http.MethodPut:
        return "update"
    case http.MethodDelete:
        return "delete"
    default:
        return "get"
    }
}
```

**Apply Middleware**:
```go
// pkg/datastorage/server/server.go
r.Use(s.AuthorizationMiddleware)  // After authentication, before handlers
```

---

## ✅ **IMPLEMENTATION PHASES**

### **Phase 1: Update RBAC (30 min)**

1. Replace `deploy/data-storage/client-rbac.yaml` with granular ClusterRoles
2. Create 3 ClusterRoles: `full-access`, `read-only`, `audit-writer`
3. Assign roles to services based on operations
4. Apply RBAC: `kubectl apply -f deploy/data-storage/client-rbac.yaml`

### **Phase 2: Update OAuth2-Proxy SAR (15 min)**

1. Update `deploy/data-storage/deployment.yaml`
2. Change `--openshift-sar` from `verb:"get"` to `verb:"create"`
3. Deploy DataStorage

### **Phase 3: Add Application-Level AuthZ (2 hours)** (OPTIONAL - Phase 2)

1. Create `pkg/datastorage/server/authz_middleware.go`
2. Implement SAR client for runtime authorization
3. Apply middleware to all API endpoints
4. Add unit tests for middleware

### **Phase 4: E2E Tests (1 hour)**

1. Create E2E ServiceAccounts with `full-access` role
2. Test audit write (all services)
3. Test workflow search (HAPI)
4. Test workflow CRUD (E2E tests)
5. Test 403 Forbidden (services without permission)

---

## 🎯 **SUCCESS CRITERIA**

### **RBAC Validation**
- [ ] Gateway SA has `create` verb (audit write)
- [ ] HAPI SA has `create` + `get` + `list` verbs (audit + workflow search)
- [ ] E2E SA has all verbs (`create`, `get`, `list`, `update`)
- [ ] Services without permission get HTTP 403

### **OAuth2-Proxy SAR**
- [ ] SAR checks `verb:"create"` (highest permission level)
- [ ] Services with `create` permission pass SAR
- [ ] Services without permission fail SAR

### **E2E Tests**
- [ ] All services can write audit events
- [ ] HAPI can search workflows
- [ ] E2E tests can CRUD workflows
- [ ] Read-only operations work (GET workflows)

---

## 📚 **REFERENCES**

- **DD-AUTH-004**: OAuth-proxy sidecar design
- **DD-AUTH-009**: OAuth2-proxy migration
- **DD-AUTH-010**: E2E real authentication mandate
- **BR-SOC2-CC8.1**: User attribution requirements

---

## 🚨 **CRITICAL DECISION**

**User's Mandate**: "either we use a proper SA that we create programmatically that validates the correct RBAC roles or we use clusteradmin. I am more inclined to use the former and ensure we are testing the correct roles we'll use in production. That means we only enable the verbs that are used, not '*'."

**Decision**: ✅ Use granular RBAC with operation-specific verbs (no wildcards)

**Rationale**:
- ✅ Tests production-equivalent RBAC
- ✅ Validates Principle of Least Privilege
- ✅ Catches RBAC misconfigurations in E2E
- ✅ Enforces security boundaries

**Approved Verb Assignments**:
- Audit write services: `create`
- Workflow read services: `get`, `list`
- Workflow write services: `create`, `update`
- E2E tests: `create`, `get`, `list`, `update` (all operations)

---

**Document Version**: 1.0  
**Last Updated**: January 26, 2026  
**Estimated Implementation Effort**: 3-4 hours (RBAC + OAuth2-proxy + E2E tests)
