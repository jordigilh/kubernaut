# Webhook Consolidation Triage - SOC2 Compliance

**Date**: January 6, 2026
**Status**: ✅ **ANALYSIS COMPLETE**
**Purpose**: Evaluate consolidating multiple admission webhooks into a single implementation
**Authoritative Sources**:
- `DD-AUTH-001`: Shared Authentication Webhook (AUTHORITATIVE)
- `DD-WEBHOOK-001`: CRD Webhook Requirements Matrix (AUTHORITATIVE)
- `ADR-051`: Operator-SDK Webhook Scaffolding

---

## 🎯 **Executive Summary**

**Recommendation**: ✅ **CONSOLIDATED APPROACH** (Already Decided)

**Current Status**: ✅ **Shared webhook library exists** (`pkg/authwebhook/`)
**Implementation Status**: ⚠️  **NOT YET IMPLEMENTED** (no `*_webhook.go` files found)

### **Key Findings**

1. ✅ **DD-AUTH-001 is AUTHORITATIVE** - Already mandates shared webhook approach
2. ✅ **Shared library exists** - `pkg/authwebhook` provides common authentication logic
3. ✅ **Only 2 CRDs require webhooks** - WorkflowExecution + RemediationApprovalRequest
4. ✅ **Consolidation already decided** - Architecture documents specify shared service

---

## 📋 **CRD Webhook Requirements Analysis**

### **CRDs Requiring Webhooks** (per DD-WEBHOOK-001)

| CRD | Use Case | Status Fields | SOC2 Control | Priority |
|-----|----------|---------------|--------------|----------|
| **WorkflowExecution** | Block Clearance | `status.blockClearanceRequest` → `status.blockClearance` | CC8.1 (Attribution) | P0 |
| **RemediationApprovalRequest** | Approval Decisions | `status.approvalRequest` → `status.decision` | CC8.1 (Attribution) | P0 |

### **CRDs NOT Requiring Webhooks** (per DD-WEBHOOK-001)

| CRD | Reason |
|-----|--------|
| **SignalProcessing** | Controller-only status updates |
| **AIAnalysis** | Controller-only AI investigation results |
| **RemediationRequest** | Controller-only routing logic |
| **NotificationRequest** | Controller-only notification delivery |

**Result**: **ONLY 2 CRDs** require webhooks for SOC2 compliance.

---

## 🏗️ **Architecture Comparison**

### **Option A: Independent Webhooks** (Rejected)

<details>
<summary><b>❌ Multiple Separate Webhook Deployments</b> (Click to expand)</summary>

```
┌──────────────────────────────────────────────┐
│  WorkflowExecution Webhook                    │
│  (Separate deployment)                        │
│  - Port: 9443                                 │
│  - TLS: workflowexecution-webhook-certs      │
│  - MutatingWebhookConfiguration: wfe-webhook │
└──────────────────────────────────────────────┘

┌──────────────────────────────────────────────┐
│  RemediationApprovalRequest Webhook           │
│  (Separate deployment)                        │
│  - Port: 9444                                 │
│  - TLS: rar-webhook-certs                    │
│  - MutatingWebhookConfiguration: rar-webhook │
└──────────────────────────────────────────────┘
```

**Cons**:
- ❌ **Operational Overhead**: 2 separate deployments, services, TLS certs
- ❌ **Resource Duplication**: Each webhook consumes CPU/memory
- ❌ **Code Duplication**: Authentication logic duplicated
- ❌ **Inconsistent Behavior**: Different implementations may diverge
- ❌ **Maintenance Burden**: Updates required in multiple places
- ❌ **Violates DD-AUTH-001**: Architecture decision already rejected this

</details>

---

### **Option B: Consolidated Webhook** ✅ (Approved in DD-AUTH-001)

```
┌────────────────────────────────────────────────────────────┐
│           kubernaut-auth-webhook (Single Deployment)        │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐  │
│  │  MutatingWebhookConfiguration                       │  │
│  ├─────────────────────────────────────────────────────┤  │
│  │  1. workflowexecutions.kubernaut.ai/status          │  │
│  │     → /authenticate/workflowexecution               │  │
│  │  2. remediationapprovalrequests.kubernaut.ai/status │  │
│  │     → /authenticate/remediationapproval             │  │
│  └─────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐  │
│  │  Shared Authentication Logic (pkg/authwebhook)      │  │
│  ├─────────────────────────────────────────────────────┤  │
│  │  ✅ ExtractAuthenticatedUser(req)                   │  │
│  │  ✅ ValidateReason(reason, minLength)               │  │
│  │  ✅ ValidateTimestamp(ts)                           │  │
│  │  ✅ EmitAuthenticatedAuditEvent(ctx, event)         │  │
│  └─────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐  │
│  │  CRD-Specific Handlers                              │  │
│  ├─────────────────────────────────────────────────────┤  │
│  │  - WorkflowExecutionAuthHandler                     │  │
│  │  - RemediationApprovalAuthHandler                   │  │
│  │  → Future handlers extensible                       │  │
│  └─────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────┘
```

**Pros**:
- ✅ **Single Deployment**: One service, one TLS cert, one port
- ✅ **Shared Authentication Logic**: Consistent behavior across all CRDs
- ✅ **Lower Resource Usage**: Single pod vs. multiple pods
- ✅ **Easier Maintenance**: Single codebase for authentication
- ✅ **SOC2 Compliance**: Centralized audit trail for all operator actions
- ✅ **Extensible**: Add new CRD handlers without new deployments
- ✅ **DD-AUTH-001 Compliant**: Follows approved architecture

---

## 📊 **Architectural Decision Analysis**

### **DD-AUTH-001: Shared Authentication Webhook**

**Status**: ✅ **AUTHORITATIVE** (Approved 2025-12-19)

**Key Points**:
1. ✅ **MANDATES** shared webhook service (`kubernaut-auth-webhook`)
2. ✅ **REQUIRES** MutatingWebhookConfiguration with multiple rules
3. ✅ **SPECIFIES** shared library pattern (`pkg/authwebhook/`)
4. ✅ **ENFORCES** consistent authentication across all CRDs

**Direct Quote from DD-AUTH-001**:
> **THIS IS THE AUTHORITATIVE SOURCE FOR USER AUTHENTICATION IN KUBERNAUT.**
> **ALL CRDs REQUIRING USER IDENTITY TRACKING MUST USE THIS WEBHOOK.**

**Result**: ✅ **Consolidation is NOT optional - it's MANDATORY per DD-AUTH-001**

---

### **DD-WEBHOOK-001: CRD Webhook Requirements Matrix**

**Status**: ✅ **AUTHORITATIVE** (2025-12-20)

**Key Points**:
1. ✅ Defines **WHEN** webhooks are required (4 criteria)
2. ✅ Only **2 CRDs** currently meet criteria
3. ✅ References DD-AUTH-001 for shared webhook pattern
4. ✅ Specifies "shared library" in implementation timeline

**Decision Criteria** (per DD-WEBHOOK-001):
- Manual Intervention Required? → **YES** (both CRDs)
- SOC2 Attribution Required? → **YES** (CC8.1)
- Approval Workflows? → **YES** (RAR)
- Override Actions? → **YES** (WFE block clearance)

**Result**: ✅ **Both CRDs meet ALL 4 criteria - consolidation makes sense**

---

## 🔍 **Current Implementation Status**

### **Shared Library** ✅ **IMPLEMENTED**

**Location**: `pkg/authwebhook/`

**Files**:
1. ✅ `types.go` - `AuthContext` struct with `String()` method
2. ✅ `authenticator.go` - `ExtractUser()` from admission request
3. ✅ `validator.go` - `ValidateReason()` + `ValidateTimestamp()`
4. ✅ `authenticator_test.go` - Unit tests for authenticator
5. ✅ `validator_test.go` - Unit tests for validators

**API**:
```go
// pkg/authwebhook/authenticator.go
func (a *Authenticator) ExtractUser(ctx context.Context, req *admissionv1.AdmissionRequest) (*AuthContext, error)

// pkg/authwebhook/validator.go
func ValidateReason(reason string, minLength int) error
func ValidateTimestamp(ts time.Time) error

// pkg/authwebhook/types.go
type AuthContext struct {
    Username string
    UID      string
    Groups   []string
    Extra    map[string]authenticationv1.ExtraValue
}
func (a *AuthContext) String() string // Returns "username (UID: uid)"
```

**Status**: ✅ **PRODUCTION-READY SHARED LIBRARY EXISTS**

---

### **Webhook Implementations** ❌ **NOT YET IMPLEMENTED**

**Expected Files** (per operator-sdk pattern):
- `api/workflowexecution/v1alpha1/workflowexecution_webhook.go`
- `api/remediationapprovalrequest/v1alpha1/remediationapprovalrequest_webhook.go`

**Actual Files**: **ZERO** webhook files found

**Search Results**:
```bash
$ find . -name "*_webhook.go" -not -path "./vendor/*"
# No results

$ find . -name "webhook*.go" -not -path "./vendor/*" -not -path "./test/*"
# No results
```

**Status**: ⚠️  **WEBHOOKS NOT IMPLEMENTED YET**

---

## 🎯 **Recommendation**

### **✅ APPROVED: Consolidated Webhook Approach**

**Rationale**:
1. ✅ **DD-AUTH-001 is AUTHORITATIVE** - Already mandates this approach
2. ✅ **Shared library exists** - Infrastructure ready for implementation
3. ✅ **Only 2 CRDs** - Small scope makes consolidation straightforward
4. ✅ **SOC2 Compliance** - Centralized audit trail for operator actions
5. ✅ **Operational Simplicity** - Single deployment, service, TLS cert

---

## 📋 **Implementation Plan**

### **Phase 1: Webhook Service Scaffolding** (1 day)

**Deliverable**: Create `cmd/webhooks/main.go` with HTTP server

```go
// cmd/webhooks/main.go
package main

import (
    "net/http"

    "sigs.k8s.io/controller-runtime/pkg/webhook"
    "github.com/jordigilh/kubernaut/internal/webhook/handlers"
)

func main() {
    // Create webhook server
    webhookServer := webhook.NewServer(webhook.Options{
        Port: 9443,
        CertDir: "/tmp/k8s-webhook-server/serving-certs",
    })

    // Register handlers
    webhookServer.Register("/authenticate/workflowexecution",
        &handlers.WorkflowExecutionAuthHandler{})
    webhookServer.Register("/authenticate/remediationapproval",
        &handlers.RemediationApprovalAuthHandler{})

    // Start server
    http.ListenAndServe(":9443", webhookServer)
}
```

---

### **Phase 2: CRD-Specific Handlers** (2 days)

**Location**: `internal/webhook/handlers/`

**Files to Create**:
1. `workflowexecution_handler.go`
2. `remediationapproval_handler.go`

**Handler Interface**:
```go
type AuthHandler interface {
    Handle(ctx context.Context, req admission.Request) admission.Response
}
```

**Implementation Pattern**:
```go
// internal/webhook/handlers/workflowexecution_handler.go
type WorkflowExecutionAuthHandler struct {
    authenticator *authwebhook.Authenticator
    auditClient   AuditClient
}

func (h *WorkflowExecutionAuthHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
    // 1. Extract authenticated user (shared logic)
    authCtx, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
    if err != nil {
        return admission.Errored(http.StatusUnauthorized, err)
    }

    // 2. Parse WFE from request
    wfe := &workflowexecutionv1alpha1.WorkflowExecution{}
    if err := json.Unmarshal(req.Object.Raw, wfe); err != nil {
        return admission.Errored(http.StatusBadRequest, err)
    }

    // 3. Validate clearance request (shared logic)
    if err := authwebhook.ValidateReason(wfe.Status.BlockClearanceRequest.ClearReason, 20); err != nil {
        return admission.Denied(err.Error())
    }
    if err := authwebhook.ValidateTimestamp(wfe.Status.BlockClearanceRequest.RequestedAt.Time); err != nil {
        return admission.Denied(err.Error())
    }

    // 4. Populate authenticated fields
    wfe.Status.BlockClearance = &workflowexecutionv1alpha1.BlockClearance{
        ClearedBy: authCtx.String(), // "username (UID: uid)"
        ClearedAt: metav1.Now(),
        ClearReason: wfe.Status.BlockClearanceRequest.ClearReason,
        ClearMethod: "KubernetesAdmissionWebhook",
    }

    // 5. Emit audit event (shared logic)
    h.auditClient.EmitAuthenticatedEvent(ctx, "workflowexecution.block.cleared", authCtx, wfe)

    // 6. Return patched object
    return admission.Patched("authenticated", genPatch(wfe))
}
```

---

### **Phase 3: Kubernetes Configuration** (1 day)

**Files to Create**:
1. `config/webhook/manifests.yaml` - Deployment, Service, MutatingWebhookConfiguration
2. `config/webhook/kustomization.yaml` - cert-manager integration

**MutatingWebhookConfiguration**:
```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: kubernaut-auth-webhook
  annotations:
    cert-manager.io/inject-ca-from: kubernaut-system/kubernaut-webhook-cert
webhooks:
- name: workflowexecution.kubernaut.ai
  clientConfig:
    service:
      name: kubernaut-auth-webhook
      namespace: kubernaut-system
      path: /authenticate/workflowexecution
  rules:
  - operations: ["UPDATE"]
    apiGroups: ["kubernaut.ai"]
    apiVersions: ["v1alpha1"]
    resources: ["workflowexecutions/status"]
    scope: "Namespaced"
  admissionReviewVersions: ["v1"]
  sideEffects: None

- name: remediationapproval.kubernaut.ai
  clientConfig:
    service:
      name: kubernaut-auth-webhook
      namespace: kubernaut-system
      path: /authenticate/remediationapproval
  rules:
  - operations: ["UPDATE"]
    apiGroups: ["kubernaut.ai"]
    apiVersions: ["v1alpha1"]
    resources: ["remediationapprovalrequests/status"]
    scope: "Namespaced"
  admissionReviewVersions: ["v1"]
  sideEffects: None
```

---

### **Phase 4: Testing** (2 days)

**Test Tiers**:
1. ✅ **Unit Tests** (18 tests per CRD, 36 total)
   - Authentication extraction
   - Reason validation
   - Timestamp validation
   - Mutual exclusion (controller vs operator fields)
   - Error handling

2. ✅ **Integration Tests** (3 tests per CRD, 6 total)
   - Full webhook flow with envtest
   - Real K8s API server authentication
   - Audit event emission

3. ✅ **E2E Tests** (2 tests per CRD, 4 total)
   - End-to-end operator workflows in Kind cluster
   - SOC2 compliance validation

---

## 📊 **Comparison Matrix**

| Aspect | Independent Webhooks | Consolidated Webhook |
|--------|----------------------|----------------------|
| **Deployments** | 2 separate | 1 shared | ✅ |
| **Services** | 2 separate | 1 shared | ✅ |
| **TLS Certificates** | 2 separate | 1 shared | ✅ |
| **Ports** | 9443, 9444 | 9443 only | ✅ |
| **Code Duplication** | High | None | ✅ |
| **Resource Usage** | 2x pods/CPU/memory | 1x | ✅ |
| **Operational Overhead** | High | Low | ✅ |
| **Maintenance** | 2 codebases | 1 codebase | ✅ |
| **Consistency** | Risk of divergence | Guaranteed | ✅ |
| **Extensibility** | New deployment per CRD | Add handler only | ✅ |
| **SOC2 Compliance** | Same | Same | ✅ |
| **DD-AUTH-001 Compliance** | ❌ Violates | ✅ Mandated | ✅ |

**Winner**: ✅ **Consolidated Webhook** (11/11 advantages)

---

## ✅ **Conclusion**

### **Final Recommendation**

✅ **IMPLEMENT CONSOLIDATED WEBHOOK** (`kubernaut-auth-webhook`)

**Rationale**:
1. ✅ **DD-AUTH-001 is AUTHORITATIVE** - Already mandates consolidated approach
2. ✅ **Shared library exists** - `pkg/authwebhook/` ready for use
3. ✅ **Only 2 CRDs** - Small scope, easy consolidation
4. ✅ **Superior Architecture** - Lower overhead, better maintainability
5. ✅ **SOC2 Compliance** - Centralized audit trail for operator actions

### **Implementation Status**

| Component | Status | Next Action |
|-----------|--------|-------------|
| **Shared Library** | ✅ **COMPLETE** | None - ready to use |
| **Webhook Service** | ❌ **NOT STARTED** | Create `cmd/webhooks/main.go` |
| **CRD Handlers** | ❌ **NOT STARTED** | Implement 2 handlers |
| **K8s Config** | ❌ **NOT STARTED** | Create webhook manifests |
| **Tests** | ❌ **NOT STARTED** | 46 tests (36 unit + 6 integration + 4 E2E) |

### **Estimated Timeline**

- **Phase 1** (Scaffolding): 1 day
- **Phase 2** (Handlers): 2 days
- **Phase 3** (K8s Config): 1 day
- **Phase 4** (Testing): 2 days
- **Total**: **6 days** (1.2 weeks)

---

## 📚 **References**

### **Authoritative Documents**
- ✅ **DD-AUTH-001**: Shared Authentication Webhook (AUTHORITATIVE)
- ✅ **DD-WEBHOOK-001**: CRD Webhook Requirements Matrix (AUTHORITATIVE)
- ✅ **ADR-051**: Operator-SDK Webhook Scaffolding
- ✅ **BR-WE-013**: Audit-Tracked Block Clearing
- ✅ **ADR-040**: Remediation Approval Request Architecture

### **Implementation References**
- ✅ `pkg/authwebhook/` - Shared authentication library
- ✅ Controller-runtime webhook docs: https://book.kubebuilder.io/reference/webhook-overview.html

---

**Status**: ✅ **ANALYSIS COMPLETE**
**Recommendation**: ✅ **CONSOLIDATED WEBHOOK** (DD-AUTH-001 mandated)
**Next**: Implement `cmd/webhooks/main.go` + CRD handlers

**Compliance**: DD-AUTH-001, DD-WEBHOOK-001, BR-WE-013, SOC2 CC8.1



