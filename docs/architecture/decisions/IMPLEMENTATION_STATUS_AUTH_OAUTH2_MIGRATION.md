# OAuth2-Proxy Migration & Secret Management - Implementation Status

**Date**: January 26, 2026
**Status**: 📋 **PLANNING COMPLETE** - Ready for Implementation
**Authority**: Implementation tracking document

---

## ✅ **COMPLETED: Authoritative Documentation**

### **Design Decisions Created**

| Document | Purpose | Status | Path |
|----------|---------|--------|------|
| **DD-AUTH-008** | Secret Management Strategy (Kustomize + Helm) | ✅ Approved | `docs/architecture/decisions/DD-AUTH-008-secret-management-kustomize-helm.md` |
| **DD-AUTH-009** | OAuth2-Proxy Migration & Workflow Attribution Implementation Plan | ✅ Approved | `docs/architecture/decisions/DD-AUTH-009-oauth2-proxy-workflow-attribution-implementation.md` |

### **Implementation Files Created**

| File | Purpose | Status |
|------|---------|--------|
| `deploy/secrets/kustomization.yaml` | Kustomize secret generation (dev) | ✅ Created |
| `deploy/secrets/README.md` | Secret management usage guide | ✅ Created |
| `deploy/secrets/production/kustomization.yaml` | File-based secrets (production) | ✅ Created |
| `deploy/secrets/production/README.md` | Production deployment guide | ✅ Created |

---

## 📋 **KEY DECISIONS DOCUMENTED**

### **1. Secret Management Strategy (DD-AUTH-008)**

**Decision**: Use Kustomize for secret generation, Helm for application deployment

**Rationale**:
- ✅ Secrets NEVER visible in Git or Helm templates
- ✅ Works with `kubectl apply -k` (no standalone kustomize needed)
- ✅ Industry-standard separation of concerns
- ✅ Verified: kubectl v1.35.0 does NOT support `--enable-helm` flag

**Evidence**: Triaged upstream kubernetes/kubernetes v1.35.0 source code

---

### **2. Kustomize vs Helm Integration (DD-AUTH-008)**

**Alternatives Considered**:
1. ❌ **Helm `randAlphaNum` + `lookup`** - Rejected (secrets visible in `helm template`)
2. ❌ **Kustomize `helmCharts` field** - Rejected (requires `--enable-helm` flag not in kubectl)
3. ✅ **Separate Deployment** - Approved (Kustomize for secrets, Helm for app)

**Deployment Model**:
```bash
kubectl apply -k deploy/secrets/           # Step 1: Create secrets
helm upgrade --install kubernaut ./helm/kubernaut  # Step 2: Deploy app
```

---

### **3. OAuth2-Proxy Configuration (DD-AUTH-009)**

**Target Configuration**:
- **Image**: `quay.io/oauth2-proxy/oauth2-proxy:v7.5.1`
- **Provider**: OpenShift/K8s OAuth (OIDC)
- **SAR Verb**: `*` (all operations - read audit + CRUD workflow)
- **User Header**: `X-Auth-Request-User` (unchanged)

**Services Affected**:
1. DataStorage (`kubernaut-system`)
2. HolmesGPT API (`kubernaut-system`)

---

### **4. Workflow Catalog User Attribution (DD-AUTH-009)**

**Implementation**:
- Extract user from `X-Auth-Request-User` header (oauth2-proxy injected)
- Set `created_by` field on workflow creation
- Set `updated_by` field on workflow update
- Return 401 if header missing

**Business Requirement**: BR-SOC2-CC8.1 (User Attribution)

---

## 🚀 **NEXT STEPS: IMPLEMENTATION**

### **Phase 1: Secret Management Setup** (30 minutes)

```bash
# Deploy secrets
kubectl apply -k deploy/secrets/

# Verify
kubectl get secrets -n kubernaut-system | grep oauth-proxy
```

**Files to Implement**: ✅ Already created
- `deploy/secrets/kustomization.yaml`
- `deploy/secrets/production/kustomization.yaml`

---

### **Phase 2: DataStorage OAuth2-Proxy Migration** (1 hour)

**File to Update**: `deploy/data-storage/deployment.yaml`

**Changes Required**:
1. Update image: `quay.io/oauth2-proxy/oauth2-proxy:v7.5.1`
2. Update args for oauth2-proxy (OIDC provider, SAR, etc.)
3. Keep volumeMount unchanged (references Kustomize-generated secret)

**Status**: ⏳ Pending implementation

---

### **Phase 3: HolmesGPT API OAuth2-Proxy Migration** (1 hour)

**File to Update**: `deploy/holmesgpt-api/06-deployment.yaml`

**Changes Required**: Same as DataStorage

**Status**: ⏳ Pending implementation

---

### **Phase 4: Workflow Catalog User Attribution** (1.5 hours)

**Files to Update**:
1. `pkg/datastorage/server/workflow_handlers.go` - Add user extraction
2. `test/integration/datastorage/workflow_integration_test.go` - Add tests

**Status**: ⏳ Pending implementation

---

### **Phase 5: NetworkPolicy Update** (30 minutes)

**File to Update**: `deploy/data-storage/networkpolicy.yaml`

**Changes Required**:
- Allow ALL pods in `kubernaut-system` namespace
- Allow Notification controller from `kubernaut-notifications` namespace

**Status**: ⏳ Pending implementation

---

## 📊 **IMPLEMENTATION TRACKING**

### **Progress Summary**

| Phase | Task | Estimated Time | Status |
|-------|------|----------------|--------|
| **Phase 0** | Documentation & Planning | 2 hours | ✅ Complete |
| **Phase 0.1** | RBAC Update (verbs:["*"]) | 15 min | ⏳ Ready |
| **Phase 1** | Secret Management Setup | 30 min | ⏳ Ready |
| **Phase 2** | DataStorage Migration | 1 hour | ⏳ Ready |
| **Phase 3** | HAPI Migration | 1 hour | ⏳ Ready |
| **Phase 4** | Workflow Attribution | 1.5 hours | ⏳ Ready |
| **Phase 5** | E2E Real Authentication (DD-AUTH-010) | 3 hours | ⏳ Ready |
| **Phase 6** | NetworkPolicy Update | 30 min | ⏳ Ready |
| **Phase 7** | Testing & Validation | 2 hours | ⏳ Pending |

**Total Estimated Effort**: 10-12 hours (revised with DD-AUTH-010 real authentication)

---

## 🔍 **UPSTREAM RESEARCH FINDINGS**

### **kubectl Kustomize Version**

**Researched**: kubernetes/kubernetes v1.35.0 (latest stable)

**Findings**:
- kubectl v1.35.0 embeds Kustomize v5.7.1
- Kustomize v5.7.1 HAS `helmCharts` API field
- kubectl does NOT expose `--enable-helm` flag
- Therefore: Cannot use `helmCharts` with `kubectl apply -k`

**Conclusion**: Separate Kustomize (secrets) and Helm (app) deployment is the correct approach

**Evidence URLs**:
- https://github.com/kubernetes/kubernetes/tree/v1.35.0
- https://github.com/kubernetes-sigs/kustomize/tree/kustomize/v5.7.1

---

## 📚 **AUTHORITATIVE DOCUMENTATION INDEX**

### **Design Decisions**

1. **[DD-AUTH-008](./DD-AUTH-008-secret-management-kustomize-helm.md)** - Secret Management Strategy
   - Kustomize for secrets, Helm for app
   - Dynamic secret generation (not in Git)
   - Production vs development strategies

2. **[DD-AUTH-009](./DD-AUTH-009-oauth2-proxy-workflow-attribution-implementation.md)** - Implementation Plan
   - OAuth2-proxy migration steps
   - Workflow catalog user attribution
   - NetworkPolicy updates
   - Testing strategy

3. **[DD-AUTH-007](../../development/SOC2/DD-AUTH-007_OAUTH_PROXY_MIGRATION.md)** - OAuth2-Proxy Migration
   - Configuration comparison (origin-oauth-proxy vs oauth2-proxy)
   - Header compatibility analysis
   - E2E test migration

4. **[DD-AUTH-004](./DD-AUTH-004-openshift-oauth-proxy-legal-hold.md)** - DataStorage OAuth Pattern
   - Legal hold operations
   - SAR configuration
   - User header injection

---

### **Implementation Guides**

1. **[Secrets README](../../../deploy/secrets/README.md)** - Secret management usage
2. **[Production Secrets README](../../../deploy/secrets/production/README.md)** - Production deployment
3. **[DD-AUTH-009 Implementation Plan](./DD-AUTH-009-oauth2-proxy-workflow-attribution-implementation.md)** - Step-by-step guide

---

## ✅ **VALIDATION CHECKLIST**

Before marking complete, verify:

- [ ] All authoritative documentation created
- [ ] Kustomize secret configurations created
- [ ] Production secret strategy documented
- [ ] Deployment order clearly defined
- [ ] Upstream kubectl/kustomize versions researched
- [ ] Implementation plan detailed with time estimates
- [ ] Testing strategy defined
- [ ] Rollback plan documented

**Current Status**: ✅ All documentation complete, ready for implementation

---

## 🎯 **QUICK REFERENCE**

### **Deploy Secrets (Development)**

```bash
kubectl apply -k deploy/secrets/
```

### **Deploy Secrets (Production)**

```bash
# One-time setup
openssl rand -base64 32 > /vault/secrets/ds-cookie-secret.txt
openssl rand -base64 32 > /vault/secrets/hapi-cookie-secret.txt

# Deploy
kubectl apply -k deploy/secrets/production/
```

### **Deploy Application (Helm)**

```bash
helm upgrade --install kubernaut ./helm/kubernaut \
  --namespace kubernaut-system \
  --create-namespace
```

---

## 📝 **CHANGELOG**

| Date | Version | Changes |
|------|---------|---------|
| 2026-01-26 | v1.0 | Initial documentation complete |

---

**Next Action**: Begin Phase 1 implementation (Secret Management Setup)

**Estimated Time to Complete**: 4-6 hours of implementation work

**Authority**: DD-AUTH-008, DD-AUTH-009

---

**Document Version**: 1.0  
**Last Updated**: January 26, 2026  
**Status**: ✅ **PLANNING COMPLETE** - Ready for Implementation
