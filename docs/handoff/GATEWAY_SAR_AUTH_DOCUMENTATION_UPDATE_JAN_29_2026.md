# Gateway SAR Authentication - Documentation Updates

**Date**: January 29, 2026  
**Status**: ✅ **DOCUMENTATION COMPLETE**  
**Next**: Implementation Phase

---

## 📋 **Executive Summary**

Updated all authoritative documentation to enable SAR authentication for Gateway service. Gateway now follows the same pattern as DataStorage and HolmesGPT API (DD-AUTH-014).

**Key Decision**: Gateway requires application-level authentication due to being an **external-facing entry point**, superseding the original network-only security approach (DD-GATEWAY-006).

---

## 📝 **Documents Updated**

### **1. DD-AUTH-014 (Primary Decision Document)**

**File**: `docs/architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md`

**Changes**:
- ✅ **Bumped version**: 1.0 → 2.0
- ✅ **Added changelog** section (Version 2.0)
- ✅ **Updated status**: "Proposed (POC)" → "Approved (Gateway in progress)"
- ✅ **Updated affected services**:
  - Phase 2: DataStorage ✅ Complete
  - Phase 3: HAPI ✅ Complete
  - Phase 4: Gateway 🚧 In Progress (NEW)
- ✅ **Added Phase 4 details**:
  - Rationale for Gateway SAR auth (security, SOC2, zero-trust)
  - Performance considerations (no caching, low throughput)
  - Implementation tasks (middleware, server, BRs, tests, docs)
  - Decision: APPROVED
- ✅ **Updated success metrics**: Added Gateway status
- ✅ **Updated related documents**: Added DD-GATEWAY-006 (superseded), ADR-036 (exception), new BRs

**Rationale Added**:
1. Gateway is external-facing (different threat model)
2. Zero-trust architecture (defense-in-depth)
3. SOC2 compliance (operator attribution)
4. Webhook compatibility (AlertManager/K8s Events support Bearer tokens)

---

### **2. Gateway Business Requirements**

**File**: `docs/services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md`

**Changes**:
- ✅ **Bumped version**: v1.6 → v1.7
- ✅ **Updated total BRs**: 75 → 77
- ✅ **Added changelog entry** (v1.7)
- ✅ **Added BR-GATEWAY-182**: ServiceAccount Authentication (TokenReview)
  - Description: Validate ServiceAccount tokens using Kubernetes TokenReview API
  - Priority: P0 (Critical)
  - Status: In Progress
  - Includes: Authentication flow, user identity format, error handling
- ✅ **Added BR-GATEWAY-183**: SubjectAccessReview Authorization
  - Description: Authorize requests using Kubernetes SAR API
  - Priority: P0 (Critical)
  - Status: In Progress
  - Includes: Authorization check, SAR parameters, RBAC example, error handling

**New Section**: Authentication & Authorization (BR-GATEWAY-182 to BR-GATEWAY-183)

---

### **3. DD-GATEWAY-006 (Superseded)**

**File**: `docs/architecture/decisions/DD-GATEWAY-006-authentication-strategy.md`

**Changes**:
- ✅ **Updated status**: "Approved" → "⛔ SUPERSEDED by DD-AUTH-014 V2.0"
- ✅ **Added superseded notice** at top:
  - Reason for change
  - New approach (DD-AUTH-014 V2.0)
  - Migration path
- ✅ **Marked original content** as "For Historical Reference"

**Key Message**: Network Policies alone insufficient for external-facing services

---

### **4. ADR-036 (Architectural-Level Exception)**

**File**: `docs/architecture/decisions/ADR-036-authentication-authorization-strategy.md`

**Changes**:
- ✅ **Bumped version**: 1.0 → 1.1
- ✅ **Updated status**: Added "(with Gateway exception)"
- ✅ **Added important update section**:
  - Gateway exception explained
  - Rationale (external-facing, zero-trust, SOC2)
  - Updated service status list
- ✅ **Updated services affected table**:
  - Gateway: ⚠️ Exception - SAR Auth Required
  - DataStorage: ⚠️ Exception - SAR Auth Complete
  - HAPI: ⚠️ Exception - SAR Auth Complete
  - Others: ✅ Follows ADR (Network Policies + TLS)
- ✅ **Added note**: ADR-036 applies to internal-only services

**Key Clarification**: External-facing services require SAR (DD-AUTH-014), internal services use Network Policies (ADR-036)

---

## 🎯 **New Business Requirements**

### **BR-GATEWAY-182: ServiceAccount Authentication**

**Purpose**: Validate webhook requests using Kubernetes TokenReview API

**Key Points**:
- Extract Bearer token from Authorization header
- Call TokenReview API to validate token
- Extract user identity (e.g., `system:serviceaccount:monitoring:prometheus-sa`)
- Inject user into request context for audit logging
- Return 401 if token invalid

**User Identity Formats**:
- ServiceAccount: `system:serviceaccount:<namespace>:<sa-name>`
- User: `<username>@<domain>`
- System: `system:<component-name>`

---

### **BR-GATEWAY-183: SubjectAccessReview Authorization**

**Purpose**: Authorize webhook requests using Kubernetes SAR API

**Key Points**:
- Authorization check: `Can <ServiceAccount> CREATE remediationrequests.kubernaut.ai IN <namespace>?`
- SAR Parameters:
  - User: Authenticated ServiceAccount name
  - Resource: `remediationrequests.kubernaut.ai`
  - Verb: `create`
  - Namespace: Target namespace from signal
- Return 403 if SAR denies access
- Return 500 if SAR API fails (fail-closed)

**RBAC Example**:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: gateway-signal-sender
rules:
- apiGroups: ["kubernaut.ai"]
  resources: ["remediationrequests"]
  verbs: ["create"]
```

---

## 📊 **Documentation Alignment Status**

| Document | Version | Status | Notes |
|----------|---------|--------|-------|
| DD-AUTH-014 | 2.0 | ✅ Updated | Gateway Phase 4 added |
| BR-GATEWAY | v1.7 | ✅ Updated | BR-182, BR-183 added |
| DD-GATEWAY-006 | - | ⛔ Superseded | Marked obsolete |
| ADR-036 | 1.1 | ✅ Updated | Gateway exception noted |

**Consistency Check**: ✅ All documents aligned and cross-referenced

---

## 🚀 **Next Steps: Implementation Phase**

### **Phase 1: Core Implementation** (2-3 hours)
1. Create `pkg/gateway/middleware/auth.go`
   - Follow DataStorage pattern exactly
   - Reuse `pkg/shared/auth` interfaces
2. Update `pkg/gateway/server.go`
   - Add k8sClient parameter
   - Instantiate AuthMiddleware
   - Extract user from context in audit events

### **Phase 2: Integration Tests** (2-3 hours)
1. Create `test/integration/gateway/auth_integration_test.go`
   - Use envtest (real Kubernetes)
   - Test authenticated requests (200 OK)
   - Test unauthenticated requests (401)
   - Test unauthorized requests (403)
   - Reference: DataStorage integration tests

### **Phase 3: E2E Tests** (2-3 hours)
1. Update `test/e2e/gateway/*_test.go`
   - Create E2E ServiceAccount with RBAC
   - Use authenticated HTTP clients
   - Verify audit events capture ServiceAccount
2. Update `test/infrastructure/gateway_e2e.go`
   - Deploy Gateway with k8sClient
   - Create Gateway ServiceAccount + RBAC

### **Phase 4: Deployment Documentation** (1 hour)
1. Update `deploy/gateway/README.md`
   - RBAC requirements
   - Webhook configuration examples
2. Create integration guide
   - Prometheus AlertManager configuration
   - Kubernetes Event forwarder setup

---

## ⚠️ **Key Decisions**

### **No Token Caching** (per user guidance)
**Rationale**:
- Low throughput: <100 signals/min expected
- NetworkPolicy reduces unauthorized traffic before auth layer
- Caching adds complexity/risk
- Can revisit post-v1.0 if needed

### **Supersedes DD-GATEWAY-006**
**Rationale**:
- Original decision made before DD-AUTH-014 pattern proven successful
- External-facing services have different threat model
- Zero-trust architecture requires app-level auth

### **Exception to ADR-036**
**Rationale**:
- ADR-036 intended for internal-only services
- Gateway is external-facing (different security requirements)
- Both decisions remain valid for their respective scopes

---

## 🎓 **Key Learnings**

1. **Threat Modeling Matters**: External-facing vs internal services require different auth strategies
2. **Pattern Reuse**: DD-AUTH-014 pattern proven successful in DS/HAPI → apply to Gateway
3. **SOC2 Compliance**: Operator attribution (ActorID) requires authentication, not just network isolation
4. **Webhook Compatibility**: AlertManager + K8s Events natively support Bearer tokens → no blocker

---

## ✅ **Documentation Review Checklist**

- [x] DD-AUTH-014 updated (version, changelog, phases, metrics, related docs)
- [x] BR-GATEWAY updated (version, changelog, BR-182, BR-183)
- [x] DD-GATEWAY-006 marked superseded (notice added, reasons explained)
- [x] ADR-036 updated (version, exception noted, services table updated)
- [x] All cross-references validated (DD-AUTH-014 ↔ BR-GATEWAY ↔ DD-GATEWAY-006 ↔ ADR-036)
- [x] Business rationale documented (security, SOC2, zero-trust)
- [x] Implementation approach defined (same pattern as DS/HAPI)
- [x] Related documents linked (DD-AUTH-013, DD-TEST-012, BR-SECURITY-016/017)

---

## 📚 **Related Documents**

### **Decision Documents**
- [DD-AUTH-014 V2.0](../architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md) - Primary decision
- [DD-GATEWAY-006](../architecture/decisions/DD-GATEWAY-006-authentication-strategy.md) - Superseded
- [ADR-036 V1.1](../architecture/decisions/ADR-036-authentication-authorization-strategy.md) - Gateway exception
- [DD-AUTH-013](../architecture/decisions/DD-AUTH-013-http-status-codes-oauth-proxy.md) - HTTP status codes
- [DD-TEST-012](../architecture/decisions/DD-TEST-012-envtest-real-authentication-pattern.md) - Test strategy

### **Business Requirements**
- [Gateway BUSINESS_REQUIREMENTS.md v1.7](../services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md)
- BR-GATEWAY-182: ServiceAccount Authentication
- BR-GATEWAY-183: SubjectAccessReview Authorization
- BR-SECURITY-016: Kubernetes RBAC enforcement
- BR-SECURITY-017: ServiceAccount token authentication

### **Reference Implementations**
- DataStorage: `pkg/datastorage/middleware/auth.go`
- DataStorage Integration: `test/integration/datastorage/auth_middleware_integration_test.go`
- DataStorage E2E: `test/e2e/datastorage/23_sar_access_control_test.go`
- Shared Auth: `pkg/shared/auth/interfaces.go`, `pkg/shared/auth/k8s_auth.go`

---

**Status**: ✅ **DOCUMENTATION PHASE COMPLETE**  
**Confidence**: 95% (high confidence - all documents aligned)  
**Next**: Proceed to implementation (core middleware + tests)  
**Estimated Effort**: 8-12 hours (2 days)

---

**Author**: AI Assistant  
**Date**: January 29, 2026  
**Session**: Gateway SAR Authentication - Documentation Updates
