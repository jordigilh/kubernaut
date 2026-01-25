# DD-AUTH-007: Multi-arch origin-oauth-proxy Solution - FINAL

**Date**: January 8, 2026
**Status**: ✅ READY TO BUILD
**Authority**: AUTHORITATIVE - Final OAuth proxy solution
**Related**: DD-AUTH-004 (DataStorage), DD-AUTH-006 (HAPI)

---

## 📋 **EXECUTIVE SUMMARY**

**Problem**:
- `origin-oauth-proxy` (OpenShift) has NO ARM64 build
- Cannot run E2E tests on ARM64 Mac
- `oauth2-proxy` (CNCF) does NOT support Kubernetes ServiceAccount tokens

**Solution**:
- Build multi-arch `origin-oauth-proxy` from source (add ARM64 support)
- Push to `quay.io/jordigilh/ose-oauth-proxy:latest`
- Use in all environments (E2E, CI/CD, Production)

---

## ✅ **WHY origin-oauth-proxy?**

| Feature | origin-oauth-proxy | oauth2-proxy | kube-rbac-proxy |
|---------|-------------------|--------------|-----------------|
| **SA Token Validation** | ✅ TokenReview | ❌ OIDC only | ✅ TokenReview |
| **SAR (Authorization)** | ✅ YES | ❌ NO | ✅ YES |
| **User Header Injection** | ✅ X-Forwarded-User | ✅ X-Forwarded-User | ❌ NO |
| **Multi-arch (ARM64)** | ❌ NO (until we build it) | ✅ YES | ✅ YES |
| **Production Parity** | ✅ OpenShift standard | ⚠️ Different auth | ⚠️ No header injection |

**Winner**: origin-oauth-proxy (with custom ARM64 build)

---

## 🏗️ **BUILD STRATEGY**

### **Components Created**

1. **Dockerfile** (`build/ose-oauth-proxy/Dockerfile`)
   - Builds origin-oauth-proxy from source
   - Multi-stage build (golang:1.21 → distroless)
   - Supports ARM64 + AMD64

2. **Build Script** (`build/ose-oauth-proxy/build-and-push.sh`)
   - Pulls AMD64 from upstream (reuse existing)
   - Builds ARM64 from source (adds ARM64 support)
   - Creates multi-arch manifest
   - Pushes to `quay.io/jordigilh/ose-oauth-proxy:latest`

3. **E2E Infrastructure** (`test/infrastructure/datastorage.go`)
   - Uses `quay.io/jordigilh/ose-oauth-proxy:latest`
   - Configured for SA token validation
   - No code changes to DataStorage needed

---

## 🚀 **IMPLEMENTATION STEPS**

### **Step 1: Build Multi-arch Image**

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut/build/ose-oauth-proxy

# Login to quay.io
podman login quay.io

# Build and push multi-arch image
./build-and-push.sh
```

**What this does**:
1. Pulls AMD64 image from `quay.io/openshift/origin-oauth-proxy:latest`
2. Builds ARM64 from source (native build on ARM64 Mac)
3. Tags both: `quay.io/jordigilh/ose-oauth-proxy:latest-{amd64,arm64}`
4. Creates manifest: `quay.io/jordigilh/ose-oauth-proxy:latest` (multi-arch)
5. Pushes to quay.io

---

### **Step 2: Run E2E Tests**

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Run DataStorage E2E tests
make test-e2e-datastorage
```

**Expected Results**:
- ✅ OAuth-proxy pod runs successfully on ARM64 Mac
- ✅ OAuth-proxy validates SA tokens (TokenReview + SAR)
- ✅ OAuth-proxy injects `X-Forwarded-User` header
- ✅ Audit events have `actor_id` from SA token
- ✅ All E2E tests pass

---

## 🔧 **CONFIGURATION**

### **E2E Mode (Kind)**

```yaml
# origin-oauth-proxy sidecar
Image: quay.io/jordigilh/ose-oauth-proxy:latest
ImagePullPolicy: Always  # Pull latest multi-arch build

Args:
- --provider=openshift
- --openshift-service-account=datastorage
- --http-address=0.0.0.0:4180
- --upstream=http://127.0.0.1:8080
- --set-xauthrequest=true  # Inject X-Forwarded-User header
- --skip-auth-regex=^/health$  # Allow health checks
```

**E2E Test Flow**:
```
1. E2E test gets SA token from /var/run/secrets/kubernetes.io/serviceaccount/token
2. E2E test sends: Authorization: Bearer <SA token>
3. origin-oauth-proxy validates token (TokenReview API)
4. origin-oauth-proxy performs SAR (authorization check)
5. origin-oauth-proxy extracts user from token
6. origin-oauth-proxy injects: X-Forwarded-User: system:serviceaccount:ns:sa
7. DataStorage reads X-Forwarded-User → actor_id in audit events
```

---

### **Production Mode**

**Same image, same configuration!**

```yaml
# deploy/datastorage/06-deployment.yaml
Image: quay.io/jordigilh/ose-oauth-proxy:latest
# All other config identical to E2E
```

**Production Flow**: Identical to E2E (production parity)

---

## 📋 **FILES MODIFIED**

### **1. Build Infrastructure** (NEW)
- `build/ose-oauth-proxy/Dockerfile` - Multi-stage build from source
- `build/ose-oauth-proxy/build-and-push.sh` - Build + push script

### **2. E2E Infrastructure** (UPDATED)
- `test/infrastructure/datastorage.go`:
  - Changed image: `origin-oauth-proxy:latest` → `quay.io/jordigilh/ose-oauth-proxy:latest`
  - Updated args for SA token validation
  - Updated ConfigMap (placeholder, origin-oauth-proxy uses K8s APIs)

### **3. Documentation** (NEW)
- `docs/development/SOC2/DD-AUTH-007_FINAL_SOLUTION.md` (this file)

---

## 🧪 **TESTING STRATEGY**

### **E2E Tests** (Kind - All Platforms)
- Use ServiceAccount tokens (DD-AUTH-006 pattern)
- Test TokenReview + SAR validation
- Verify X-Forwarded-User header injection
- Validate audit event actor_id

### **Integration Tests** (No oauth-proxy)
- Direct business logic calls
- Mock headers if needed (optional)
- No oauth-proxy deployment

### **Unit Tests** (No oauth-proxy)
- Pure business logic
- Mock data structures
- No infrastructure

---

## ✅ **BENEFITS**

1. **ARM64 Support**: E2E tests work on ARM64 Mac
2. **Production Parity**: Same oauth-proxy in all environments
3. **SA Token Validation**: Tests real K8s authentication (TokenReview + SAR)
4. **Clean Architecture**: DataStorage doesn't parse tokens (DD-AUTH-003)
5. **No DataStorage Changes**: Auth stays externalized
6. **Multi-arch**: Works on ARM64 Mac + AMD64 CI/production

---

## ⚠️ **MAINTENANCE**

### **Updating the Image**

```bash
# Pull latest upstream changes
cd build/ose-oauth-proxy
./build-and-push.sh

# This will:
# 1. Pull latest AMD64 from quay.io/openshift/origin-oauth-proxy
# 2. Rebuild ARM64 from latest source
# 3. Update multi-arch manifest
```

**Frequency**:
- On-demand (when upstream updates)
- Quarterly (recommended)
- When security patches released

---

## 📚 **REFERENCES**

- **DD-AUTH-004**: DataStorage oauth-proxy pattern (original)
- **DD-AUTH-006**: HAPI oauth-proxy + ServiceAccount tokens
- **DD-AUTH-003**: Externalized Authorization Sidecar pattern
- **SOC2 CC8.1**: User attribution requirement
- **origin-oauth-proxy source**: https://github.com/openshift/oauth-proxy

---

## 🎯 **SUCCESS CRITERIA**

- [x] Multi-arch ose-oauth-proxy Dockerfile created
- [x] Build and push script created
- [x] E2E infrastructure updated to use multi-arch image
- [x] No DataStorage code changes
- [ ] **Multi-arch image built and pushed** (USER ACTION REQUIRED)
- [ ] **E2E tests pass on ARM64 Mac** (USER ACTION REQUIRED)
- [ ] **Audit events have correct actor_id** (USER ACTION REQUIRED)

---

## 🚀 **NEXT STEPS**

### **Immediate (TODAY)**:
1. Build multi-arch image:
   ```bash
   cd build/ose-oauth-proxy
   podman login quay.io
   ./build-and-push.sh
   ```

2. Test E2E on ARM64 Mac:
   ```bash
   make test-e2e-datastorage
   ```

3. Verify audit events:
   ```bash
   kubectl logs -n kubernaut-datastorage-e2e deploy/datastorage -c datastorage | grep actor_id
   ```

### **This Week**:
- Update HAPI E2E (if needed - likely not, based on investigation)
- Raise PR for SOC2 work

### **Next Week**:
- Update production deployments to use multi-arch image
- Deploy to staging → production

---

## 📊 **COMPARISON TABLE**

| Approach | E2E ARM64 | SA Tokens | Header Injection | DataStorage Changes | Effort |
|----------|-----------|-----------|------------------|---------------------|--------|
| **Multi-arch ose-oauth-proxy** ⭐ | ✅ | ✅ | ✅ | ❌ No | 30min |
| oauth2-proxy | ✅ | ❌ | ✅ | ❌ No | N/A (incompatible) |
| kube-rbac-proxy | ✅ | ✅ | ❌ | ✅ Yes | 2h |
| Skip oauth-proxy (hybrid) | ✅ | ❌ | ✅ | ❌ No | 10min |

**Winner**: Multi-arch ose-oauth-proxy (this solution)

---

## ✅ **FINAL VERDICT**

This is the **optimal solution** that satisfies all requirements:
- ✅ ARM64 support for local development
- ✅ ServiceAccount token validation (E2E requirement)
- ✅ Production parity (same auth flow)
- ✅ Clean architecture (no auth in business code)
- ✅ Minimal implementation effort (30 minutes)

**Ready to build!** 🚀

