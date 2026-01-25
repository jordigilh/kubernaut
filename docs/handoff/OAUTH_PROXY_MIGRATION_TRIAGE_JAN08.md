# OAuth Proxy Migration - Triage and Implementation Plan - January 8, 2026

**Authority**: DD-AUTH-007
**Status**: 🚧 PLANNING
**Effort**: 4-6 hours total

---

## 📋 **TRIAGE SUMMARY**

### **Services Requiring Migration**

| # | Service | Environment | Current State | Impact Level | Effort |
|---|---------|-------------|---------------|--------------|--------|
| 1 | **DataStorage** | E2E Tests | ❌ Broken (ARM64 `ImagePullBackOff`) | 🔴 CRITICAL | 2h |
| 2 | **DataStorage** | Production | ✅ Working (`ose-oauth-proxy` amd64) | 🟡 MEDIUM | 1h |
| 3 | **HAPI** | E2E Tests | ⚠️ Unknown (need to check) | 🟡 MEDIUM | 1-2h |
| 4 | **HAPI** | Production | ✅ Working (`ose-oauth-proxy` amd64) | 🟡 MEDIUM | 1h |

**Total Estimated Effort**: 4-6 hours

---

## 🔍 **DETAILED TRIAGE**

### **1. DataStorage E2E (CRITICAL - BLOCKING PR)**

**Current State**:
- File: `test/infrastructure/datastorage.go`
- Image: Attempted multiple oauth-proxy options, all failed ARM64
- Status: ❌ `ImagePullBackOff` on ARM64 Mac
- Blocker: Cannot run E2E tests, cannot raise PR

**Root Cause**:
- `quay.io/openshift/origin-oauth-proxy:latest` → ❌ No ARM64 build
- `registry.access.redhat.com/openshift4/ose-oauth-proxy:latest` → ❌ Requires auth

**Solution**: Migrate to `quay.io/oauth2-proxy/oauth2-proxy:v7.5.1` (multi-arch)

**Files to Modify**:
1. `test/infrastructure/datastorage.go` - Change image, args, ConfigMap
2. `test/e2e/datastorage/*_test.go` - Verify auth headers (likely no changes)

**Validation**:
```bash
make test-e2e-datastorage  # Must pass on ARM64 Mac
```

---

### **2. DataStorage Production (MEDIUM - NOT BLOCKING)**

**Current State**:
- File: `deploy/datastorage/06-deployment.yaml`
- Image: `quay.io/openshift/origin-oauth-proxy:latest`
- Status: ✅ Working (deployed on AMD64 clusters only)
- Impact: No immediate issue, but tech debt

**Why Migrate**:
- Future ARM64 cluster support
- Public registry (no Red Hat auth)
- Better maintained (CNCF project)

**Files to Modify**:
1. `deploy/datastorage/06-deployment.yaml` - Change image, args
2. `deploy/datastorage/13-oauth-proxy-secret.yaml` - Update ConfigMap format

**Validation**:
```bash
# Deploy to staging cluster
kubectl apply -k deploy/datastorage/
# Verify audit events have correct actor_id
```

---

### **3. HAPI E2E (MEDIUM - NEED TO INVESTIGATE)**

**Current State**: ⚠️ **UNKNOWN** - Need to check if AIAnalysis E2E tests use HAPI with oauth-proxy

**Investigation Needed**:
```bash
# Check if HAPI E2E infrastructure exists
ls -la test/infrastructure/holmesgpt_api*.go
ls -la test/e2e/aianalysis/

# Check if oauth-proxy is deployed in AIAnalysis E2E
grep -r "oauth-proxy" test/e2e/aianalysis/
grep -r "oauth-proxy" test/infrastructure/*aianalysis*
```

**Possible Outcomes**:

#### **Outcome A: HAPI E2E Has oauth-proxy**
- **Action**: Migrate (same as DataStorage E2E)
- **Effort**: 1-2 hours
- **Priority**: HIGH (same as DataStorage E2E)

####  **Outcome B: HAPI E2E Has NO oauth-proxy**
- **Action**: Integration tests use mock headers (already working)
- **Effort**: 0 hours (no changes needed)
- **Priority**: N/A

---

### **4. HAPI Production (MEDIUM - NOT BLOCKING)**

**Current State**:
- File: `deploy/holmesgpt-api/06-deployment.yaml`
- Image: `quay.io/openshift/origin-oauth-proxy:latest`
- Status: ✅ Working (deployed on AMD64 clusters only)
- Impact: Same as DataStorage Production

**Files to Modify**:
1. `deploy/holmesgpt-api/06-deployment.yaml` - Change image, args
2. `deploy/holmesgpt-api/13-oauth-proxy-secret.yaml` - Update ConfigMap format

**Validation**:
```bash
# Deploy to staging cluster
kubectl apply -k deploy/holmesgpt-api/
# Verify Gateway can still call HAPI
# Verify HAPI logs show correct user identity
```

---

## 🎯 **IMPLEMENTATION PHASES**

### **Phase 1: DataStorage E2E (TODAY - BLOCKING PR)** ⏱️ 2 hours

**Objective**: Fix ARM64 `ImagePullBackOff`, enable E2E tests on Mac

**Tasks**:
1. [ ] Update `test/infrastructure/datastorage.go`:
   - Change image to `quay.io/oauth2-proxy/oauth2-proxy:v7.5.1`
   - Update args to oauth2-proxy format (per DD-AUTH-007)
   - Update ConfigMap to htpasswd format
   - Remove unused volumes/mounts

2. [ ] Run E2E tests:
   ```bash
   make test-e2e-datastorage
   ```

3. [ ] Verify oauth-proxy pod:
   ```bash
   kubectl get pods -n kubernaut-datastorage-e2e
   kubectl logs -n kubernaut-datastorage-e2e deploy/datastorage -c oauth-proxy
   ```

4. [ ] Verify audit events:
   ```bash
   # Check actor_id in audit events
   kubectl logs -n kubernaut-datastorage-e2e deploy/datastorage -c datastorage | grep actor_id
   ```

**Success Criteria**:
- ✅ OAuth-proxy pod runs on ARM64 Mac
- ✅ All E2E tests pass
- ✅ Audit events have `actor_id: "test-operator@kubernaut.ai"`

**Deliverables**:
- Modified: `test/infrastructure/datastorage.go`
- Documentation: Update DD-AUTH-007 with implementation notes

---

### **Phase 2: HAPI E2E Investigation (TODAY)** ⏱️ 30 min

**Objective**: Determine if HAPI E2E needs oauth-proxy migration

**Tasks**:
1. [ ] Search for HAPI E2E oauth-proxy usage:
   ```bash
   find test/ -name "*hapi*" -o -name "*aianalysis*" -o -name "*holmes*"
   grep -r "oauth-proxy" test/e2e/aianalysis/ test/infrastructure/
   ```

2. [ ] Check AIAnalysis E2E test patterns:
   ```bash
   cat test/e2e/aianalysis/*_test.go | grep -A 10 "HolmesGPT\|HAPI"
   ```

3. [ ] Document findings in DD-AUTH-007

**Decision Matrix**:
| Finding | Action | Priority |
|---------|--------|----------|
| HAPI E2E has oauth-proxy | Proceed to Phase 3 | HIGH |
| HAPI E2E has NO oauth-proxy | Skip Phase 3, proceed to Phase 4 | N/A |
| Unclear | Manual inspection of test code | MEDIUM |

---

### **Phase 3: HAPI E2E Migration (IF NEEDED)** ⏱️ 1-2 hours

**Objective**: Fix HAPI E2E oauth-proxy for ARM64

**Tasks** (Only if Phase 2 finds oauth-proxy usage):
1. [ ] Update HAPI E2E infrastructure file (TBD based on Phase 2)
2. [ ] Run AIAnalysis E2E tests:
   ```bash
   make test-e2e-aianalysis
   ```
3. [ ] Verify HAPI receives correct headers

**Success Criteria**:
- ✅ AIAnalysis E2E tests pass on ARM64
- ✅ HAPI logs show correct user identity

---

### **Phase 4: Production Migrations (NEXT WEEK)** ⏱️ 2 hours

**Objective**: Migrate production deployments to oauth2-proxy

**Priority**: LOW (not blocking PR, AMD64 works fine)

**Tasks**:
1. [ ] DataStorage Production:
   - Update `deploy/datastorage/06-deployment.yaml`
   - Update ConfigMap for OIDC provider (not htpasswd)
   - Test on staging cluster
   - Deploy to production

2. [ ] HAPI Production:
   - Update `deploy/holmesgpt-api/06-deployment.yaml`
   - Update ConfigMap for OIDC provider
   - Test on staging cluster
   - Deploy to production

**Success Criteria**:
- ✅ Production services work with oauth2-proxy
- ✅ Real OAuth provider integration (Keycloak/OpenShift OAuth)
- ✅ User headers injected correctly

---

## 📋 **PRIORITY MATRIX**

| Phase | Service | Environment | Priority | Blocking PR | Effort | Timeline |
|-------|---------|-------------|----------|-------------|--------|----------|
| 1 | DataStorage | E2E | 🔴 CRITICAL | YES | 2h | TODAY |
| 2 | HAPI | E2E Investigation | 🟡 HIGH | Depends on Phase 2 | 30min | TODAY |
| 3 | HAPI | E2E Migration | 🟡 HIGH | Depends on Phase 2 | 1-2h | TODAY |
| 4a | DataStorage | Production | 🟢 LOW | NO | 1h | NEXT WEEK |
| 4b | HAPI | Production | 🟢 LOW | NO | 1h | NEXT WEEK |

---

## 🧪 **E2E TEST AUTHENTICATION PATTERNS**

### **Current State (DataStorage E2E)**

**Before Migration** (origin-oauth-proxy):
```go
// E2E test configuration (BROKEN on ARM64)
ConfigMap: "oauth-proxy-config"
Data:
  oauth-proxy.cfg: |
    provider = "static"
    static_user = "test-operator@kubernaut.ai"
    pass_user_headers = true
```

**After Migration** (oauth2-proxy):
```go
// E2E test configuration (WORKS on ARM64 + AMD64)
ConfigMap: "oauth-proxy-config"
Data:
  htpasswd: |
    test-operator@kubernaut.ai:$2y$05$...bcrypt_hash...
```

**E2E Test Flow**:
```go
// Option 1: Direct HTTP with Basic Auth (NEW - tests oauth-proxy)
client := &http.Client{}
req, _ := http.NewRequest("POST", dsURL+"/api/v1/audit/events", body)
req.SetBasicAuth("test-operator@kubernaut.ai", "e2e-test-password")
resp, _ := client.Do(req)

// Option 2: Direct HTTP with Mock Header (OLD - bypasses oauth-proxy)
req.Header.Set("X-Forwarded-User", "test-operator@kubernaut.ai")
resp, _ := client.Do(req)
```

**Recommendation**: Use Option 1 (tests full oauth-proxy flow)

---

## ⚠️ **RISKS AND MITIGATIONS**

### **Risk 1: E2E Tests Fail After Migration**

**Mitigation**:
- Thoroughly test locally on ARM64 Mac before committing
- Run full E2E suite: `make test-e2e-datastorage`
- Check oauth-proxy logs for auth errors
- Verify audit events have correct `actor_id`

### **Risk 2: Header Names Change**

**Mitigation**:
- DD-AUTH-007 documents headers are UNCHANGED
- Both implementations support same headers via `--set-xauthrequest=true`
- No client/service code changes needed

### **Risk 3: Production Breakage**

**Mitigation**:
- Production migration is LOW priority (Phase 4)
- Test on staging cluster first
- AMD64 production clusters work fine with origin-oauth-proxy
- No urgency to migrate production

---

## 📝 **DECISION LOG**

### **Decision 1: Prioritize DataStorage E2E First**

**Rationale**: Blocking PR, critical path for SOC2 work

### **Decision 2: Use oauth2-proxy (not kube-rbac-proxy)**

**Rationale**:
- kube-rbac-proxy does NOT inject user headers (authz only)
- oauth2-proxy provides full auth + user attribution
- See DD-AUTH-007 for detailed analysis

### **Decision 3: Use htpasswd for E2E**

**Rationale**:
- Tests full oauth-proxy flow (not just mock headers)
- Simpler than K8s ServiceAccount token setup
- Production parity without K8s auth complexity

---

## 🚀 **NEXT STEPS (IMMEDIATE)**

1. **NOW**: Execute Phase 1 (DataStorage E2E migration)
2. **AFTER Phase 1**: Execute Phase 2 (HAPI E2E investigation)
3. **IF Phase 2 finds oauth-proxy**: Execute Phase 3 (HAPI E2E migration)
4. **AFTER E2E TESTS PASS**: Raise PR for SOC2 work
5. **NEXT WEEK**: Execute Phase 4 (Production migrations)

**Blocker Removal**: Phase 1 completion unblocks PR

---

## 📚 **REFERENCES**

- **DD-AUTH-007**: OAuth Proxy Migration (AUTHORITATIVE)
- **DD-AUTH-004**: DataStorage oauth-proxy pattern
- **DD-AUTH-006**: HAPI oauth-proxy pattern
- **E2E_SOC2_IMPLEMENTATION_COMPLETE_JAN08.md**: Current E2E oauth-proxy state
- **HAPI_OAUTH_PROXY_COMPLETE_JAN07.md**: HAPI oauth-proxy implementation

