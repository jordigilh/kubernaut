# DD-AUTH-007: OAuth Proxy Migration (origin-oauth-proxy → oauth2-proxy)

**Date**: January 8, 2026
**Status**: 🚧 IN PROGRESS
**Authority**: AUTHORITATIVE - Supersedes previous oauth-proxy implementations
**Related**: DD-AUTH-004 (DataStorage), DD-AUTH-006 (HAPI)

---

## 📋 **EXECUTIVE SUMMARY**

**Objective**: Migrate from OpenShift `origin-oauth-proxy` to CNCF `oauth2-proxy` for multi-architecture support (ARM64 + AMD64).

**Reason**:
- ❌ `ose-oauth-proxy` (OpenShift): No ARM64 support, Red Hat registry auth required
- ✅ `oauth2-proxy` (CNCF): Multi-arch, public registry (quay.io)

**Impact**: 2 services, 2 E2E suites, production deployments

---

## 🎯 **SCOPE**

### **Services Affected**

| Service | Current | Target | Production Deploy | E2E Tests |
|---------|---------|--------|-------------------|-----------|
| **DataStorage** | `ose-oauth-proxy` | `oauth2-proxy` | `deploy/datastorage/` | `test/e2e/datastorage/` |
| **HAPI** | `ose-oauth-proxy` | `oauth2-proxy` | `deploy/holmesgpt-api/` | `test/e2e/aianalysis/` |

### **Header Names (UNCHANGED)**

| Service | Current Header | New Header | Notes |
|---------|---------------|------------|-------|
| **DataStorage** | `X-Forwarded-User` | `X-Forwarded-User` | ✅ No change |
| **HAPI** | `X-Auth-Request-User` | `X-Auth-Request-User` | ✅ No change |

**Rationale**: Both oauth-proxy implementations support the same headers via `--set-xauthrequest=true`.

---

## 🔧 **CONFIGURATION COMPARISON**

### **origin-oauth-proxy (OLD)**

```yaml
args:
- --config=/etc/oauth-proxy/oauth-proxy.cfg
- --upstream=http://localhost:8080
- --http-address=0.0.0.0:4180
- --pass-user-bearer-token=false
- --pass-access-token=false
- --set-xauthrequest=true
- --skip-auth-regex=^/health
```

**ConfigMap** (origin-oauth-proxy):
```toml
provider = "static"           # E2E only
email_domains = ["*"]
upstreams = ["http://localhost:8080/"]
http_address = "0.0.0.0:4180"
static_user = "test-operator@kubernaut.ai"  # E2E only
pass_user_headers = true
```

---

### **oauth2-proxy (NEW)**

```yaml
args:
- --http-address=0.0.0.0:4180
- --upstream=http://localhost:8080
- --email-domain=*
- --cookie-secret=CHANGEME1234567890123456  # Must be 32 chars
# E2E: Use htpasswd for static user
- --htpasswd-file=/etc/oauth-proxy/htpasswd  # E2E only
# Production: Use OIDC provider
- --provider=oidc                            # Production only
- --oidc-issuer-url=https://keycloak.example.com  # Production only
- --skip-auth-route=^/health$                # Allow health checks
- --set-xauthrequest=true                    # Inject user header
- --set-authorization-header=true
- --pass-user-headers=true
- --pass-access-token=false
```

**ConfigMap** (oauth2-proxy E2E):
```yaml
# htpasswd format (bcrypt)
# User: test-operator@kubernaut.ai
# Password: e2e-test-password
htpasswd: |
  test-operator@kubernaut.ai:$2y$05$FKilXbhBhKH1WAaL9IVzqOYpuVh3QRwEVBhZKGN3KVR6xZHLp.lfu
```

**ConfigMap** (oauth2-proxy Production):
```yaml
# Production uses OIDC provider (Keycloak, etc.)
# No htpasswd file needed
```

---

## 🚧 **MIGRATION STRATEGY**

### **Phase 1: DataStorage Migration** ⏱️ 2-3 hours

#### **1.1: E2E Tests** (PRIORITY)
**File**: `test/infrastructure/datastorage.go`

**Changes**:
- [ ] Change image: `quay.io/openshift/origin-oauth-proxy:latest` → `quay.io/oauth2-proxy/oauth2-proxy:v7.5.1`
- [ ] Update args to oauth2-proxy format
- [ ] Update ConfigMap to htpasswd format
- [ ] Test E2E: `make test-e2e-datastorage`
- [ ] Verify audit events have `actor_id: "test-operator@kubernaut.ai"`

**Success Criteria**:
- ✅ DataStorage E2E tests pass
- ✅ OAuth-proxy pod runs on ARM64 Mac
- ✅ Audit events have correct user attribution

---

#### **1.2: Production Deployment**
**File**: `deploy/datastorage/06-deployment.yaml`

**Changes**:
- [ ] Change image to `oauth2-proxy:v7.5.1`
- [ ] Update args (use OIDC provider, not htpasswd)
- [ ] Update ConfigMap (remove static user config)
- [ ] Test on staging cluster

**Success Criteria**:
- ✅ DataStorage production deployment works
- ✅ Real OAuth provider integration (Keycloak/OpenShift OAuth)
- ✅ User headers injected correctly

---

### **Phase 2: HAPI Migration** ⏱️ 2-3 hours

#### **2.1: E2E Tests** (if HAPI has E2E with oauth-proxy)
**File**: `test/infrastructure/holmesgpt_api.go` (check if exists)

**Changes**:
- [ ] Same as DataStorage E2E migration
- [ ] Test E2E: `make test-e2e-aianalysis`
- [ ] Verify HAPI logs show correct user

**Success Criteria**:
- ✅ AIAnalysis E2E tests pass
- ✅ HAPI receives `X-Auth-Request-User` header

---

#### **2.2: Production Deployment**
**File**: `deploy/holmesgpt-api/06-deployment.yaml`

**Changes**:
- [ ] Change image to `oauth2-proxy:v7.5.1`
- [ ] Update args (use OIDC provider)
- [ ] Header name stays: `X-Auth-Request-User` (no code changes needed)
- [ ] Test on staging cluster

**Success Criteria**:
- ✅ HAPI production deployment works
- ✅ AIAnalysis service can still call HAPI
- ✅ User headers logged correctly

---

## 🔍 **AUTHORIZATION CONFIGURATION**

### **Selective Endpoint Protection**

**Requirement**: Only enforce auth on endpoints that need it, not ALL endpoints.

**oauth2-proxy Configuration**:
```yaml
# Allow health checks without auth
- --skip-auth-route=^/health$

# Production: Allow metrics without auth (if needed)
- --skip-auth-route=^/(health|metrics)$

# All other routes require authentication
```

**DataStorage Endpoints**:
| Endpoint | Auth Required | Reason |
|----------|--------------|--------|
| `/health` | ❌ No | Kubernetes liveness probes |
| `/metrics` | ⚠️ TBD | Prometheus scraping (discuss) |
| `/api/v1/audit/events` | ✅ Yes | SOC2 CC8.1 requirement |
| `/api/v1/audit/export` | ✅ Yes | SOC2 CC8.1 requirement |

**HAPI Endpoints**:
| Endpoint | Auth Required | Reason |
|----------|--------------|--------|
| `/health` | ❌ No | Kubernetes liveness probes |
| `/metrics` | ⚠️ TBD | Prometheus scraping |
| `/api/v1/incident/analyze` | ✅ Yes | LLM cost tracking |
| `/api/v1/recovery/suggest` | ✅ Yes | LLM cost tracking |

---

## 📋 **E2E TEST AUTHENTICATION PATTERN**

### **Current State (Production)**

**ServiceAccount Token Flow** (DD-AUTH-006):
```
1. Client (AIAnalysis, Gateway) reads SA token from /var/run/secrets/kubernetes.io/serviceaccount/token
2. Client sends: Authorization: Bearer <token>
3. oauth-proxy validates token with TokenReview API
4. oauth-proxy performs SubjectAccessReview (SAR)
5. oauth-proxy injects X-Auth-Request-User header
6. Service receives authenticated request
```

### **E2E Test Flow (Simpler)**

**Option 1: Direct HTTP with Mock Header** (Current DataStorage E2E)
```go
// E2E test bypasses oauth-proxy entirely
req, _ := http.NewRequest("POST", dsURL+"/api/v1/audit/events", body)
req.Header.Set("X-Forwarded-User", "test-operator@kubernaut.ai")
resp, _ := client.Do(req)
```

**❌ Problem**: Doesn't test oauth-proxy integration!

**Option 2: HTTP with htpasswd Auth** (New Recommendation)
```go
// E2E test authenticates through oauth-proxy
client := &http.Client{}
req, _ := http.NewRequest("POST", dsURL+"/api/v1/audit/events", body)
req.SetBasicAuth("test-operator@kubernaut.ai", "e2e-test-password")
resp, _ := client.Do(req)
// oauth-proxy validates htpasswd, injects X-Forwarded-User header
```

**✅ Benefit**: Tests full oauth-proxy flow without K8s ServiceAccount complexity!

---

## ⚠️ **BREAKING CHANGES**

### **None Expected**

**Headers**: Unchanged (`X-Forwarded-User`, `X-Auth-Request-User`)
**Client Code**: No changes needed (auth is transparent)
**Business Logic**: No changes needed (services read same headers)

### **Deployment Changes**

- ConfigMap format changes (toml → yaml)
- Container args change (different flags)
- Image registry changes (openshift → oauth2-proxy)

---

## 🧪 **VALIDATION CHECKLIST**

### **DataStorage Validation**

- [ ] E2E tests pass on ARM64 Mac
- [ ] E2E tests pass on AMD64 CI/CD
- [ ] OAuth-proxy pod starts successfully
- [ ] Audit events have correct `actor_id`
- [ ] Health checks work without auth
- [ ] Production staging deployment works

### **HAPI Validation**

- [ ] AIAnalysis E2E tests pass (if applicable)
- [ ] HAPI logs show correct user identity
- [ ] Gateway can still call HAPI
- [ ] LLM cost tracking logs work
- [ ] Production staging deployment works

---

## 📚 **REFERENCES**

- **DD-AUTH-004**: DataStorage oauth-proxy pattern (original)
- **DD-AUTH-006**: HAPI oauth-proxy pattern (ServiceAccount auth)
- **DD-AUTH-003**: Externalized Authorization Sidecar pattern
- **SOC2 CC8.1**: User attribution requirement
- **oauth2-proxy docs**: https://oauth2-proxy.github.io/oauth2-proxy/docs/

---

## 📝 **DECISION LOG**

### **Decision 1: Use oauth2-proxy (not kube-rbac-proxy)**

**Options Considered**:
- ❌ **kube-rbac-proxy**: K8s native, but NO user header injection (designed for authz only, not user attribution)
- ✅ **oauth2-proxy**: Full auth + user header injection

**Rationale**: We need user identity headers for audit logs, not just authorization checks.

---

### **Decision 2: Use htpasswd for E2E (not skip-auth)**

**Options Considered**:
- ❌ **skip-auth-route=.** *: Bypasses all auth (defeats purpose of testing oauth-proxy)
- ✅ **htpasswd**: Validates auth flow without K8s ServiceAccount complexity

**Rationale**: E2E tests should validate production patterns as much as possible.

---

### **Decision 3: Keep Same Header Names**

**Rationale**: Both oauth-proxy implementations support `X-Forwarded-User` and `X-Auth-Request-User` via `--set-xauthrequest=true`. No client/service code changes needed.

---

## 🚀 **NEXT STEPS**

1. **TODAY**: Complete DataStorage E2E migration
2. **THIS WEEK**: Complete HAPI E2E migration (if needed)
3. **NEXT WEEK**: Production staging validation
4. **WEEK AFTER**: Production rollout

**Estimated Total Effort**: 4-6 hours (both services, both environments)

