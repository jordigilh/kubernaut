# Shared Authentication Webhook - Cross-Service User Identity Extraction

**Date**: December 19, 2025
**Status**: 📋 **TRIAGE COMPLETE** - Ready for Architecture Decision
**Purpose**: Evaluate shared webhook service for extracting authenticated user identity
**Priority**: P0 (Required for BR-WE-013 SOC2 compliance + RemediationApproval UX)

---

## 🎯 **Executive Summary**

**Question**: Should we create a **shared authentication webhook** service instead of service-specific webhooks?

**Answer**: ✅ **YES** - Strong architectural benefits for multiple services requiring authenticated user identity.

**Primary Use Cases Identified**:
1. **WorkflowExecution** (BR-WE-013): Block clearance authentication
2. **RemediationApprovalRequest** (ADR-040): Approval decision authentication
3. **Future**: Any CRD requiring operator identity tracking

**Recommendation**: Create `kubernaut-auth-webhook` as a **shared library + deployment** for centralized user authentication.

---

## 📋 **Current Requirements Analysis**

### **Use Case 1: WorkflowExecution Block Clearance (BR-WE-013)**

**Requirement**: Track WHO cleared a `PreviousExecutionFailed` block for SOC2 compliance

**Current Gap**:
- Operators can delete WFE CRDs (loses audit trail)
- Need authenticated clearance mechanism

**Required Authentication**:
- Real user identity from K8s auth context
- Populate `status.blockClearance.ClearedBy` with authenticated user
- Audit event with authenticated actor

**CRD Fields**:
```go
type BlockClearanceDetails struct {
    ClearedBy   string      `json:"clearedBy"`   // NEEDS AUTHENTICATION
    ClearReason string      `json:"clearReason"` // User-provided
    ClearedAt   metav1.Time `json:"clearedAt"`
    ClearMethod string      `json:"clearMethod"` // "WebhookValidated"
}
```

**Operator Workflow**:
```bash
# Step 1: User creates clearance request
kubectl patch workflowexecution wfe-failed \
  --type=merge \
  --subresource=status \
  -p '{"status":{"blockClearanceRequest":{"clearReason":"investigation complete"}}}'

# Step 2: Webhook intercepts, extracts REAL user from K8s auth context
# Result: status.blockClearance.ClearedBy = "admin@example.com (UID: abc-123)"
```

---

### **Use Case 2: RemediationApprovalRequest Decision (ADR-040)**

**Requirement**: Track WHO approved/rejected a remediation for accountability

**Current Gap**:
- `status.decidedBy` is manually populated (string field)
- No authentication validation
- Users can impersonate others by writing arbitrary values

**Required Authentication**:
- Real user identity from K8s auth context
- Populate `status.decidedBy` with authenticated user
- Audit event: `orchestrator.approval.approved` with authenticated actor

**CRD Fields** (already exist):
```go
type RemediationApprovalRequestStatus struct {
    Decision        ApprovalDecision `json:"decision,omitempty"`        // "Approved" | "Rejected"
    DecidedBy       string          `json:"decidedBy,omitempty"`       // NEEDS AUTHENTICATION
    DecidedAt       *metav1.Time    `json:"decidedAt,omitempty"`
    DecisionMessage string          `json:"decisionMessage,omitempty"` // User-provided
}
```

**Operator Workflow**:
```bash
# Step 1: User creates approval decision
kubectl patch remediationapprovalrequest rar-test \
  --type=merge \
  --subresource=status \
  -p '{"status":{"approvalRequest":{"decision":"Approved","decisionMessage":"looks good"}}}'

# Step 2: Webhook intercepts, extracts REAL user from K8s auth context
# Result: status.decidedBy = "operator@example.com (UID: xyz-789)"
```

---

### **Use Case 3: Future Requirements**

**Potential Future Use Cases**:
- **SignalProcessing**: Manual signal triage (who dismissed/escalated)
- **AIAnalysis**: Manual workflow override (who changed LLM selection)
- **Notification**: Manual notification acknowledgment (who marked as read)
- **Any CRD**: Operator actions requiring identity tracking for compliance

---

## 🏗️ **Shared Webhook Architecture**

### **Option A: Shared Webhook Service** ✅ **RECOMMENDED**

**Architecture**:
```
┌────────────────────────────────────────────────────────────┐
│                    kubernaut-auth-webhook                   │
│                  (Single Deployment + Service)              │
├────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────────────────────────────────────────────┐  │
│  │  MutatingWebhookConfiguration                       │  │
│  ├─────────────────────────────────────────────────────┤  │
│  │  1. workflowexecutions.kubernaut.ai                 │  │
│  │     → /authenticate/workflowexecution               │  │
│  │  2. remediationapprovalrequests.kubernaut.ai        │  │
│  │     → /authenticate/remediationapproval             │  │
│  │  3. [future CRDs...]                                │  │
│  └─────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐  │
│  │  Shared Authentication Logic                        │  │
│  ├─────────────────────────────────────────────────────┤  │
│  │  - Extract user from req.UserInfo                   │  │
│  │  - Format: "username (UID: uid)"                    │  │
│  │  - Populate authenticated fields                    │  │
│  │  - Emit audit events                                │  │
│  └─────────────────────────────────────────────────────┘  │
│                                                             │
└────────────────────────────────────────────────────────────┘
```

**Implementation**:
```go
// pkg/authwebhook/shared.go
package authwebhook

import (
    "context"
    "fmt"
    admission "k8s.io/api/admission/v1"
    authenticationv1 "k8s.io/api/authentication/v1"
)

// ExtractAuthenticatedUser extracts the authenticated user from admission request
// Returns formatted user string: "username (UID: uid)" or "serviceaccount:namespace:name (UID: uid)"
func ExtractAuthenticatedUser(req admission.AdmissionRequest) string {
    userInfo := req.UserInfo

    // Format: "username (UID: uid)"
    if userInfo.Username != "" {
        return fmt.Sprintf("%s (UID: %s)", userInfo.Username, userInfo.UID)
    }

    // Fallback: UID only
    return fmt.Sprintf("unknown (UID: %s)", userInfo.UID)
}

// AuthenticatedUpdate represents an update with authenticated user
type AuthenticatedUpdate struct {
    AuthenticatedBy string
    Timestamp       metav1.Time
    Reason          string // User-provided reason
}

// WorkflowExecutionAuthHandler handles WFE block clearance authentication
type WorkflowExecutionAuthHandler struct {
    decoder     *admission.Decoder
    auditStore  audit.AuditStore
    clusterName string
}

func (h *WorkflowExecutionAuthHandler) Handle(ctx context.Context, req admission.AdmissionRequest) admission.AdmissionResponse {
    wfe := &workflowexecutionv1alpha1.WorkflowExecution{}
    if err := h.decoder.Decode(req, wfe); err != nil {
        return admission.Errored(http.StatusBadRequest, err)
    }

    // Check if clearance request is present
    if wfe.Status.BlockClearanceRequest != nil && wfe.Status.BlockClearance == nil {
        // Extract AUTHENTICATED user
        authenticatedUser := ExtractAuthenticatedUser(req)

        // Populate authenticated clearance
        wfe.Status.BlockClearance = &workflowexecutionv1alpha1.BlockClearanceDetails{
            ClearedAt:   metav1.Now(),
            ClearedBy:   authenticatedUser, // AUTHENTICATED
            ClearReason: wfe.Status.BlockClearanceRequest.ClearReason,
            ClearMethod: "WebhookValidated",
        }

        // Clear the request (one-time operation)
        wfe.Status.BlockClearanceRequest = nil

        // Emit audit event
        h.emitBlockClearanceAudit(ctx, wfe, req.UserInfo)
    }

    return admission.PatchResponseFromRaw(req.Object.Raw, marshalWFE(wfe))
}

// RemediationApprovalAuthHandler handles RAR approval decision authentication
type RemediationApprovalAuthHandler struct {
    decoder     *admission.Decoder
    auditStore  audit.AuditStore
    clusterName string
}

func (h *RemediationApprovalAuthHandler) Handle(ctx context.Context, req admission.AdmissionRequest) admission.AdmissionResponse {
    rar := &remediationv1alpha1.RemediationApprovalRequest{}
    if err := h.decoder.Decode(req, rar); err != nil {
        return admission.Errored(http.StatusBadRequest, err)
    }

    // Check if approval decision is present
    if rar.Status.ApprovalRequest != nil && rar.Status.DecidedBy == "" {
        // Extract AUTHENTICATED user
        authenticatedUser := ExtractAuthenticatedUser(req)

        // Populate authenticated decision
        rar.Status.Decision = rar.Status.ApprovalRequest.Decision
        rar.Status.DecidedBy = authenticatedUser // AUTHENTICATED
        rar.Status.DecidedAt = &metav1.Time{Time: time.Now()}
        rar.Status.DecisionMessage = rar.Status.ApprovalRequest.DecisionMessage

        // Clear the request (one-time operation)
        rar.Status.ApprovalRequest = nil

        // Emit audit event
        h.emitApprovalDecisionAudit(ctx, rar, req.UserInfo)
    }

    return admission.PatchResponseFromRaw(req.Object.Raw, marshalRAR(rar))
}
```

**Deployment**:
```yaml
# config/auth-webhook/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubernaut-auth-webhook
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: kubernaut-auth-webhook
  template:
    metadata:
      labels:
        app: kubernaut-auth-webhook
    spec:
      serviceAccountName: kubernaut-auth-webhook
      containers:
      - name: webhook
        image: quay.io/kubernaut/auth-webhook:v1.0.0
        ports:
        - containerPort: 9443
          name: webhook
        volumeMounts:
        - name: cert
          mountPath: /tmp/k8s-webhook-server/serving-certs
          readOnly: true
      volumes:
      - name: cert
        secret:
          secretName: kubernaut-auth-webhook-cert
---
apiVersion: v1
kind: Service
metadata:
  name: kubernaut-auth-webhook
  namespace: kubernaut-system
spec:
  selector:
    app: kubernaut-auth-webhook
  ports:
  - port: 443
    targetPort: 9443
---
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: kubernaut-auth-webhook
webhooks:
- name: workflowexecution.kubernaut.ai
  clientConfig:
    service:
      name: kubernaut-auth-webhook
      namespace: kubernaut-system
      path: /authenticate/workflowexecution
    caBundle: ${CA_BUNDLE}
  rules:
  - operations: ["UPDATE"]
    apiGroups: ["kubernaut.ai"]
    apiVersions: ["v1alpha1"]
    resources: ["workflowexecutions/status"]
  admissionReviewVersions: ["v1"]
  sideEffects: None

- name: remediationapproval.kubernaut.ai
  clientConfig:
    service:
      name: kubernaut-auth-webhook
      namespace: kubernaut-system
      path: /authenticate/remediationapproval
    caBundle: ${CA_BUNDLE}
  rules:
  - operations: ["UPDATE"]
    apiGroups: ["kubernaut.ai"]
    apiVersions: ["v1alpha1"]
    resources: ["remediationapprovalrequests/status"]
  admissionReviewVersions: ["v1"]
  sideEffects: None
```

**Pros**:
- ✅ **Single deployment** (shared operational overhead)
- ✅ **Consistent authentication** (one source of truth)
- ✅ **Reusable library** (`pkg/authwebhook`)
- ✅ **Easy to extend** (add new CRDs by adding handlers)
- ✅ **Centralized audit** (consistent event format)

**Cons**:
- ⚠️ **Single point of failure** (mitigated by replicas)
- ⚠️ **Slightly higher complexity** (multiple handlers)

**Effort**: 5 days
- Day 1: Shared library (`pkg/authwebhook`)
- Day 2: WFE handler + tests
- Day 3: RAR handler + tests
- Day 4: Deployment + certificate management
- Day 5: Integration + E2E tests

---

### **Option B: Service-Specific Webhooks** ⚠️ **NOT RECOMMENDED**

**Architecture**:
```
┌─────────────────────────────────┐
│ workflowexecution-webhook       │  (Separate deployment)
│ → /validate-wfe-clearance       │
└─────────────────────────────────┘

┌─────────────────────────────────┐
│ remediationapproval-webhook     │  (Separate deployment)
│ → /validate-rar-decision        │
└─────────────────────────────────┘
```

**Pros**:
- ✅ Isolated failures (one webhook failure doesn't affect others)
- ✅ Independent deployment cycles

**Cons**:
- ❌ **Duplicated code** (same auth logic in multiple places)
- ❌ **More operational overhead** (N deployments vs 1)
- ❌ **Inconsistent implementation** (drift over time)
- ❌ **Higher resource usage** (N pods vs 1 replicated pod)

**Effort**: 3 days per service = 6 days total
- Higher total effort due to duplication

---

## 🎯 **Recommendation: Option A (Shared Webhook)**

### **Why Shared Webhook is Better**

| Criteria | Shared Webhook | Service-Specific |
|----------|----------------|------------------|
| **Code Reuse** | ✅ Single library | ❌ Duplicated |
| **Operational Overhead** | ✅ 1 deployment | ❌ N deployments |
| **Consistency** | ✅ Single source | ⚠️ Can drift |
| **Extensibility** | ✅ Add handler | ❌ New deployment |
| **Resource Usage** | ✅ Minimal | ❌ N pods |
| **Total Effort** | ✅ 5 days | ❌ 6+ days |

### **Implementation Phases**

**Phase 1 (v1.0)**: Core Shared Webhook
- Shared library: `pkg/authwebhook`
- WFE block clearance handler (BR-WE-013)
- RAR approval decision handler (ADR-040)
- Deployment + cert management
- Integration + E2E tests

**Phase 2 (v1.1+)**: Additional Handlers
- SignalProcessing: Manual triage authentication
- AIAnalysis: Manual override authentication
- Notification: Acknowledgment tracking

---

## 📐 **CRD Schema Updates Required**

### **WorkflowExecution CRD** ✅ **Already Updated**

```go
type WorkflowExecutionStatus struct {
    // ... existing fields ...

    // BlockClearanceRequest is populated by operator (unauthenticated)
    // Webhook validates and moves to BlockClearance with authenticated user
    // +optional
    BlockClearanceRequest *BlockClearanceRequest `json:"blockClearanceRequest,omitempty"`

    // BlockClearance is populated by webhook (authenticated)
    // +optional
    BlockClearance *BlockClearanceDetails `json:"blockClearance,omitempty"`
}

type BlockClearanceRequest struct {
    ClearReason string      `json:"clearReason"`
    RequestedAt metav1.Time `json:"requestedAt"`
}

type BlockClearanceDetails struct {
    ClearedBy   string      `json:"clearedBy"`   // Authenticated by webhook
    ClearReason string      `json:"clearReason"` // From request
    ClearedAt   metav1.Time `json:"clearedAt"`
    ClearMethod string      `json:"clearMethod"` // "WebhookValidated"
}
```

### **RemediationApprovalRequest CRD** ⏳ **Needs Update**

**Current State**:
```go
type RemediationApprovalRequestStatus struct {
    Decision        ApprovalDecision `json:"decision,omitempty"`
    DecidedBy       string          `json:"decidedBy,omitempty"`       // ⚠️ NO AUTHENTICATION
    DecidedAt       *metav1.Time    `json:"decidedAt,omitempty"`
    DecisionMessage string          `json:"decisionMessage,omitempty"`
}
```

**Proposed Update**:
```go
type RemediationApprovalRequestStatus struct {
    // ApprovalRequest is populated by operator (unauthenticated)
    // Webhook validates and moves to Decision/DecidedBy with authenticated user
    // +optional
    ApprovalRequest *ApprovalRequest `json:"approvalRequest,omitempty"`

    // Decision is populated by webhook (authenticated)
    // +optional
    Decision ApprovalDecision `json:"decision,omitempty"`

    // DecidedBy is populated by webhook with authenticated user identity
    // Format: "username (UID: uid)" or "serviceaccount:namespace:name (UID: uid)"
    // +optional
    DecidedBy string `json:"decidedBy,omitempty"`

    // DecidedAt is when the decision was made
    // +optional
    DecidedAt *metav1.Time `json:"decidedAt,omitempty"`

    // DecisionMessage is optional message from decision maker
    // +optional
    DecisionMessage string `json:"decisionMessage,omitempty"`
}

type ApprovalRequest struct {
    Decision        ApprovalDecision `json:"decision"`        // "Approved" | "Rejected"
    DecisionMessage string          `json:"decisionMessage"` // User-provided
    RequestedAt     metav1.Time     `json:"requestedAt"`
}
```

---

## 🔐 **RBAC Requirements**

### **Operator Permissions**

**For WFE Block Clearance**:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: kubernaut-operator
  namespace: kubernaut-system
rules:
- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions/status"]
  verbs: ["get", "update", "patch"]
```

**For RAR Approval Decision**:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: kubernaut-operator
  namespace: kubernaut-system
rules:
- apiGroups: ["kubernaut.ai"]
  resources: ["remediationapprovalrequests/status"]
  verbs: ["get", "update", "patch"]
```

### **Webhook ServiceAccount Permissions**

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut-auth-webhook
rules:
- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions", "remediationapprovalrequests"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions/status", "remediationapprovalrequests/status"]
  verbs: ["update", "patch"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create"]
```

---

## 📊 **SOC2 Compliance Validation**

### **WorkflowExecution (BR-WE-013)**

| SOC2 Requirement | Shared Webhook Implementation | Validation |
|------------------|-------------------------------|------------|
| **CC8.1** - Attribution | ✅ Real user from K8s auth context | Webhook extracts `req.UserInfo` |
| **CC7.3** - Immutability | ✅ Failed WFE preserved | No deletion, only status update |
| **CC7.4** - Completeness | ✅ No gaps in history | Clearance adds to audit trail |
| **CC4.2** - Change Tracking | ✅ Audit event emitted | `workflowexecution.block.cleared` |

### **RemediationApprovalRequest (ADR-040)**

| SOC2 Requirement | Shared Webhook Implementation | Validation |
|------------------|-------------------------------|------------|
| **CC8.1** - Attribution | ✅ Real user from K8s auth context | Webhook extracts `req.UserInfo` |
| **CC4.2** - Change Tracking | ✅ Audit event emitted | `orchestrator.approval.approved` |
| **Accountability** | ✅ Decision cannot be forged | Only webhook can set `decidedBy` |

---

## ⚡ **Operational Considerations**

### **High Availability**

```yaml
spec:
  replicas: 2
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
  affinity:
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchLabels:
              app: kubernaut-auth-webhook
          topologyKey: kubernetes.io/hostname
```

### **Certificate Management**

**Option A**: cert-manager (recommended)
```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: kubernaut-auth-webhook-cert
  namespace: kubernaut-system
spec:
  secretName: kubernaut-auth-webhook-cert
  dnsNames:
  - kubernaut-auth-webhook.kubernaut-system.svc
  - kubernaut-auth-webhook.kubernaut-system.svc.cluster.local
  issuerRef:
    name: selfsigned-issuer
    kind: ClusterIssuer
```

**Option B**: Manual cert generation (not recommended for production)

### **Monitoring**

**Metrics**:
- `kubernaut_auth_webhook_requests_total{handler="wfe",status="success|error"}`
- `kubernaut_auth_webhook_auth_failures_total{handler="wfe",reason="..."}`
- `kubernaut_auth_webhook_latency_seconds{handler="wfe",quantile="..."}`

**Alerts**:
- High error rate (>5% auth failures)
- High latency (>100ms p95)
- Certificate expiration (<30 days)

---

## 🚀 **Next Steps**

### **Immediate Actions**

1. **Create Design Decision**: `DD-AUTH-001-shared-authentication-webhook.md`
2. **Update BR-WE-013 Plan**: Replace annotation approach with webhook
3. **Create ADR for RAR**: Approval decision authentication
4. **Update Parent Plan**: `IMPLEMENTATION_PLAN_V3.8.md` addendum

### **Implementation Timeline**

**Week 1 (Days 1-5)**: Shared Webhook Implementation
- Day 1: Shared library (`pkg/authwebhook`)
- Day 2: WFE handler + unit tests
- Day 3: RAR handler + unit tests
- Day 4: Deployment + cert management
- Day 5: Integration + E2E tests

**Week 2 (Days 6-7)**: Documentation + Rollout
- Day 6: Operator runbooks + docs
- Day 7: Staging deployment + validation

### **Dependencies**

- ✅ **CRD Schema**: WFE already updated, RAR needs update
- ✅ **DataStorage Service**: Running and healthy
- ✅ **BufferedAuditStore**: Already implemented
- ⏳ **cert-manager**: Required for certificate management

---

## 📝 **Decision Required**

**Question**: Approve shared authentication webhook for v1.0?

**Options**:
- **A)** ✅ **Approve shared webhook** (5 days, reusable for multiple services)
- **B)** ⚠️ **Service-specific webhooks** (6+ days, duplicated code)

**Recommendation**: **Option A** (shared webhook)

**Justification**:
- ✅ Lower total effort (5 days vs 6+ days)
- ✅ Better code reuse and consistency
- ✅ Lower operational overhead (1 deployment vs N)
- ✅ Easier to extend for future use cases
- ✅ Meets SOC2 requirements for both services

---

## 📚 **Related Documents**

### **Primary References**
- [BR-WE-013](../services/crd-controllers/03-workflowexecution/BUSINESS_REQUIREMENTS.md) - Block clearance requirement
- [ADR-040](../architecture/decisions/ADR-040-remediation-approval-request-architecture.md) - Approval request architecture
- [WE_BR_WE_013_SOC2_COMPLIANCE_TRIAGE_DEC_19_2025.md](./WE_BR_WE_013_SOC2_COMPLIANCE_TRIAGE_DEC_19_2025.md) - SOC2 analysis

### **Technical References**
- [BR_WE_013_BLOCK_CLEARANCE_ADDENDUM_V1.0.md](../services/crd-controllers/03-workflowexecution/implementation/BR_WE_013_BLOCK_CLEARANCE_ADDENDUM_V1.0.md) - Implementation plan
- [AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md](./AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md) - SOC2 v1.0 approval

---

**Document Status**: ✅ **TRIAGE COMPLETE**
**Last Updated**: December 19, 2025
**Awaiting Decision**: User approval for Option A (shared webhook)



