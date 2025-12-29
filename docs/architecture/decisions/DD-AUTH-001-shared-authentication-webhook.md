# DD-AUTH-001: Shared Authentication Webhook for User Identity Extraction

**Date**: 2025-12-19
**Status**: ✅ **APPROVED**
**Version**: 1.0
**Authority**: ⭐ **AUTHORITATIVE** - Single source of truth for authenticated user identity extraction

---

## 📋 **Status**

**✅ APPROVED** (2025-12-19)
**Confidence**: 95%
**Purpose**: Define shared webhook service for extracting authenticated user identity from Kubernetes authentication context

**THIS IS THE AUTHORITATIVE SOURCE FOR USER AUTHENTICATION IN KUBERNAUT.**
**ALL CRDs REQUIRING USER IDENTITY TRACKING MUST USE THIS WEBHOOK.**

---

## 🎯 **Context & Problem**

### **Problem Statement**

Multiple Kubernaut CRDs require tracking **WHO** performed critical operator actions:

1. **WorkflowExecution (BR-WE-013)**: WHO cleared a `PreviousExecutionFailed` block?
2. **RemediationApprovalRequest (ADR-040)**: WHO approved/rejected a remediation?
3. **Future CRDs**: Any operator action requiring accountability and audit trail

**Authentication Challenge**:
- **Annotations**: ❌ Anyone can write arbitrary values (not authenticated)
- **Direct Status Update**: ❌ No authentication layer
- **Custom API Endpoint**: ⚠️ Requires separate HTTP service (operational overhead)

**SOC2 Requirement**:
- **CC8.1 (Attribution)**: Must capture **authenticated** user identity
- **CC4.2 (Change Tracking)**: Must record WHO made changes in audit trail
- **CC7.3 (Immutability)**: Audit trail must be tamper-proof

### **Key Requirements**

1. **Real User Authentication**: Extract user identity from Kubernetes authentication context
2. **Reusability**: Support multiple CRDs with consistent authentication
3. **SOC2 Compliance**: Meet attribution and change tracking requirements
4. **K8s-Native**: Use existing K8s RBAC and authentication infrastructure
5. **Low Operational Overhead**: Single deployment, not per-service

---

## ✅ **Decision**

**APPROVED**: Create **`kubernaut-auth-webhook`** as a **shared Kubernetes admission webhook service** for authenticated user identity extraction.

### **Architecture**

```
┌────────────────────────────────────────────────────────────┐
│                    kubernaut-auth-webhook                   │
│                  (Single Deployment + Service)              │
├────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────────────────────────────────────────────┐  │
│  │  MutatingWebhookConfiguration                       │  │
│  ├─────────────────────────────────────────────────────┤  │
│  │  1. workflowexecutions.kubernaut.ai/status          │  │
│  │     → /authenticate/workflowexecution               │  │
│  │  2. remediationapprovalrequests.kubernaut.ai/status │  │
│  │     → /authenticate/remediationapproval             │  │
│  │  3. [future CRDs...]                                │  │
│  └─────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐  │
│  │  Shared Authentication Logic (pkg/authwebhook)      │  │
│  ├─────────────────────────────────────────────────────┤  │
│  │  - Extract user from req.UserInfo                   │  │
│  │  - Format: "username (UID: uid)"                    │  │
│  │  - Populate authenticated fields in status          │  │
│  │  - Emit audit events with authenticated actor       │  │
│  └─────────────────────────────────────────────────────┘  │
│                                                             │
└────────────────────────────────────────────────────────────┘
```

### **Implementation Components**

**1. Shared Library**: `pkg/authwebhook/`
```go
// Shared authentication logic
func ExtractAuthenticatedUser(req admission.AdmissionRequest) string
func EmitAuthenticatedAuditEvent(ctx context.Context, event AuditEvent)
```

**2. CRD-Specific Handlers**: `internal/webhook/`
```go
// WorkflowExecution handler
type WorkflowExecutionAuthHandler struct {}
func (h *WorkflowExecutionAuthHandler) Handle(ctx, req) admission.Response

// RemediationApprovalRequest handler
type RemediationApprovalAuthHandler struct {}
func (h *RemediationApprovalAuthHandler) Handle(ctx, req) admission.Response
```

**3. Webhook Deployment**: `config/webhook/`
- Single deployment with multiple handlers
- cert-manager integration for TLS
- High availability (2+ replicas)

### **Authentication Flow**

```
┌─────────────────────────────────────────────────────────────────┐
│  1. Operator creates request (unauthenticated)                  │
│     kubectl patch <crd> --subresource=status                    │
│     -p '{"status":{"<requestField>":{"reason":"..."}}'          │
└──────────────────────┬──────────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────────┐
│  2. K8s API Server intercepts request                           │
│     - Authenticates user via K8s auth (OIDC, certs, SA token)   │
│     - Authorizes via RBAC (user has update permission?)         │
│     - Sends to MutatingWebhook with req.UserInfo                │
└──────────────────────┬──────────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────────┐
│  3. kubernaut-auth-webhook processes request                    │
│     - Extracts user from req.UserInfo (AUTHENTICATED)           │
│     - Formats: "username (UID: uid)"                            │
│     - Populates <authenticatedField> in status                  │
│     - Emits audit event with authenticated actor                │
└──────────────────────┬──────────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────────┐
│  4. K8s API Server persists updated CRD                         │
│     - status.<authenticatedField> = "user@domain (UID: abc)"    │
│     - Audit event sent to DataStorage                           │
└─────────────────────────────────────────────────────────────────┘
```

---

## 🔧 **Implementation Details**

### **CRD Schema Pattern**

All CRDs follow this pattern:

```go
type CRDStatus struct {
    // ... existing fields ...

    // RequestField: Operator's unauthenticated input
    // Populated by operator, validated by webhook
    // +optional
    RequestField *RequestDetails `json:"requestField,omitempty"`

    // AuthenticatedField: Webhook's authenticated output
    // Populated ONLY by webhook with real user identity
    // +optional
    AuthenticatedField *AuthenticatedDetails `json:"authenticatedField,omitempty"`
}

type RequestDetails struct {
    Reason      string      `json:"reason"`      // User-provided
    RequestedAt metav1.Time `json:"requestedAt"` // Timestamp
}

type AuthenticatedDetails struct {
    AuthenticatedBy string      `json:"authenticatedBy"` // From req.UserInfo (AUTHENTICATED)
    Reason          string      `json:"reason"`          // From request
    AuthenticatedAt metav1.Time `json:"authenticatedAt"` // Timestamp
    Method          string      `json:"method"`          // "WebhookValidated"
}
```

### **Example 1: WorkflowExecution Block Clearance**

**CRD Schema**:
```go
type WorkflowExecutionStatus struct {
    // ... existing fields ...

    // BlockClearanceRequest: Operator's request (unauthenticated)
    BlockClearanceRequest *BlockClearanceRequest `json:"blockClearanceRequest,omitempty"`

    // BlockClearance: Webhook's authenticated result
    BlockClearance *BlockClearanceDetails `json:"blockClearance,omitempty"`
}

type BlockClearanceRequest struct {
    ClearReason string      `json:"clearReason"`
    RequestedAt metav1.Time `json:"requestedAt"`
}

type BlockClearanceDetails struct {
    ClearedBy   string      `json:"clearedBy"`   // AUTHENTICATED by webhook
    ClearReason string      `json:"clearReason"` // From request
    ClearedAt   metav1.Time `json:"clearedAt"`
    ClearMethod string      `json:"clearMethod"` // "WebhookValidated"
}
```

**Operator Workflow**:
```bash
# Step 1: Create request
kubectl patch workflowexecution wfe-failed \
  --type=merge \
  --subresource=status \
  -p '{"status":{"blockClearanceRequest":{"clearReason":"investigation complete","requestedAt":"2025-12-19T10:00:00Z"}}}'

# Step 2: Webhook authenticates (automatic)
# Result in status:
# blockClearance:
#   clearedBy: "admin@example.com (UID: abc-123)"
#   clearReason: "investigation complete"
#   clearedAt: "2025-12-19T10:00:05Z"
#   clearMethod: "WebhookValidated"
```

### **Example 2: RemediationApprovalRequest Decision**

**CRD Schema**:
```go
type RemediationApprovalRequestStatus struct {
    // ... existing fields ...

    // ApprovalRequest: Operator's request (unauthenticated)
    ApprovalRequest *ApprovalRequest `json:"approvalRequest,omitempty"`

    // Decision: Webhook's authenticated result
    Decision        ApprovalDecision `json:"decision,omitempty"`        // Populated by webhook
    DecidedBy       string          `json:"decidedBy,omitempty"`       // AUTHENTICATED by webhook
    DecidedAt       *metav1.Time    `json:"decidedAt,omitempty"`
    DecisionMessage string          `json:"decisionMessage,omitempty"` // From request
}

type ApprovalRequest struct {
    Decision        ApprovalDecision `json:"decision"`        // "Approved" | "Rejected"
    DecisionMessage string          `json:"decisionMessage"` // User-provided
    RequestedAt     metav1.Time     `json:"requestedAt"`
}
```

**Operator Workflow**:
```bash
# Step 1: Create request
kubectl patch remediationapprovalrequest rar-test \
  --type=merge \
  --subresource=status \
  -p '{"status":{"approvalRequest":{"decision":"Approved","decisionMessage":"looks good","requestedAt":"2025-12-19T10:00:00Z"}}}'

# Step 2: Webhook authenticates (automatic)
# Result in status:
# decision: "Approved"
# decidedBy: "operator@example.com (UID: xyz-789)"
# decidedAt: "2025-12-19T10:00:05Z"
# decisionMessage: "looks good"
```

---

## 🔐 **Security & RBAC**

### **Webhook ServiceAccount Permissions**

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut-auth-webhook
rules:
# Read CRDs to validate requests
- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions", "remediationapprovalrequests"]
  verbs: ["get", "list", "watch"]

# Update status with authenticated data
- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions/status", "remediationapprovalrequests/status"]
  verbs: ["update", "patch"]

# Create audit events
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create"]
```

### **Operator Permissions**

Operators need `update` permission on `/status` subresource:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: kubernaut-operator
  namespace: kubernaut-system
rules:
- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions/status", "remediationapprovalrequests/status"]
  verbs: ["get", "update", "patch"]
```

---

## 📊 **SOC2 Compliance**

### **Requirement Mapping**

| SOC2 Requirement | Implementation | Validation |
|------------------|----------------|------------|
| **CC8.1** - Attribution | User identity from K8s auth context (`req.UserInfo`) | Webhook extracts authenticated user |
| **CC7.3** - Immutability | Original CRDs preserved (no deletion) | Status updates only |
| **CC7.4** - Completeness | No gaps in audit trail | All actions recorded |
| **CC4.2** - Change Tracking | Audit events with authenticated actor | DataStorage persistence |

### **Audit Trail**

**Audit Event Schema**:
```json
{
  "event_type": "workflowexecution.block.cleared",
  "event_category": "workflow",
  "event_action": "block.cleared",
  "event_outcome": "success",
  "actor_type": "user",
  "actor_id": "admin@example.com (UID: abc-123)",
  "resource_type": "WorkflowExecution",
  "resource_name": "wfe-failed",
  "event_data": {
    "cleared_by": "admin@example.com (UID: abc-123)",
    "clear_reason": "investigation complete",
    "clear_method": "WebhookValidated"
  }
}
```

---

## 📦 **Deployment**

### **High Availability**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubernaut-auth-webhook
  namespace: kubernaut-system
spec:
  replicas: 2
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
  selector:
    matchLabels:
      app: kubernaut-auth-webhook
  template:
    metadata:
      labels:
        app: kubernaut-auth-webhook
    spec:
      serviceAccountName: kubernaut-auth-webhook
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels:
                  app: kubernaut-auth-webhook
              topologyKey: kubernetes.io/hostname
      containers:
      - name: webhook
        image: quay.io/kubernaut/auth-webhook:v1.0.0
        ports:
        - containerPort: 9443
          name: webhook
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 256Mi
        volumeMounts:
        - name: cert
          mountPath: /tmp/k8s-webhook-server/serving-certs
          readOnly: true
      volumes:
      - name: cert
        secret:
          secretName: kubernaut-auth-webhook-cert
```

### **Certificate Management**

**Using cert-manager** (recommended):
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

---

## 📈 **Monitoring & Observability**

### **Prometheus Metrics**

```go
// Webhook request metrics
kubernaut_auth_webhook_requests_total{handler="wfe",status="success|error"}
kubernaut_auth_webhook_requests_duration_seconds{handler="wfe",quantile="0.5|0.95|0.99"}

// Authentication metrics
kubernaut_auth_webhook_authentications_total{handler="wfe",status="success|error"}
kubernaut_auth_webhook_auth_failures_total{handler="wfe",reason="missing_user|invalid_request"}

// Audit event metrics
kubernaut_auth_webhook_audit_events_total{handler="wfe",status="success|error"}
```

### **Alerts**

```yaml
- alert: AuthWebhookHighErrorRate
  expr: rate(kubernaut_auth_webhook_requests_total{status="error"}[5m]) > 0.05
  annotations:
    summary: "Auth webhook error rate > 5%"

- alert: AuthWebhookHighLatency
  expr: histogram_quantile(0.95, kubernaut_auth_webhook_requests_duration_seconds) > 0.1
  annotations:
    summary: "Auth webhook p95 latency > 100ms"

- alert: AuthWebhookCertExpiring
  expr: (kubernaut_auth_webhook_cert_expiry_seconds - time()) < (30 * 24 * 3600)
  annotations:
    summary: "Auth webhook certificate expires in < 30 days"
```

---

## 🚀 **Implementation Timeline**

**Total Effort**: 5 days

**Phase 1: Shared Library** (Day 1)
- `pkg/authwebhook/shared.go`: User extraction logic
- `pkg/authwebhook/audit.go`: Audit event helpers
- Unit tests (10 tests)

**Phase 2: WFE Handler** (Day 2)
- `internal/webhook/workflowexecution/handler.go`
- Block clearance authentication logic
- Unit tests (8 tests)

**Phase 3: RAR Handler** (Day 3)
- `internal/webhook/remediationapproval/handler.go`
- Approval decision authentication logic
- Unit tests (8 tests)

**Phase 4: Deployment** (Day 4)
- `config/webhook/deployment.yaml`
- `config/webhook/service.yaml`
- `config/webhook/mutatingwebhook.yaml`
- cert-manager Certificate
- RBAC manifests

**Phase 5: Testing** (Day 5)
- Integration tests (6 tests)
- E2E tests (4 tests)
- Documentation

---

## 🎯 **Benefits**

### **vs Service-Specific Webhooks**

| Criteria | Shared Webhook | Service-Specific |
|----------|----------------|------------------|
| **Total Effort** | ✅ 5 days | ❌ 6+ days (3 days per service) |
| **Code Reuse** | ✅ Single library | ❌ Duplicated code |
| **Operational Overhead** | ✅ 1 deployment | ❌ N deployments |
| **Consistency** | ✅ Single source of truth | ⚠️ Can drift over time |
| **Extensibility** | ✅ Add handler (< 1 day) | ❌ New deployment (3 days) |
| **Resource Usage** | ✅ 2 pods | ❌ 2N pods |
| **Monitoring** | ✅ Single dashboard | ❌ N dashboards |

### **vs Annotations**

| Criteria | Webhook | Annotations |
|----------|---------|-------------|
| **Authentication** | ✅ Real K8s user | ❌ None (anyone can write) |
| **SOC2 Compliance** | ✅ CC8.1 compliant | ❌ Non-compliant |
| **Audit Trail** | ✅ Authenticated actor | ❌ Unauthenticated |
| **Tamper-Proof** | ✅ Webhook-only writes | ❌ Anyone with edit access |

### **vs Custom API Endpoint**

| Criteria | Webhook | Custom API |
|----------|---------|------------|
| **K8s Integration** | ✅ Native (kubectl) | ⚠️ Requires curl/client |
| **Authentication** | ✅ K8s auth context | ⚠️ Token validation required |
| **Authorization** | ✅ K8s RBAC | ⚠️ Custom authz logic |
| **Operational Overhead** | ✅ Single deployment | ⚠️ Separate HTTP service |
| **UX** | ✅ Standard kubectl | ⚠️ Custom CLI tool |

---

## 🔄 **Extensibility**

### **Adding New CRDs**

To add a new CRD requiring authentication:

**Step 1: Update CRD Schema** (< 1 hour)
```go
type NewCRDStatus struct {
    RequestField       *RequestDetails       `json:"requestField,omitempty"`
    AuthenticatedField *AuthenticatedDetails `json:"authenticatedField,omitempty"`
}
```

**Step 2: Create Handler** (< 4 hours)
```go
type NewCRDAuthHandler struct {
    decoder    *admission.Decoder
    auditStore audit.AuditStore
}

func (h *NewCRDAuthHandler) Handle(ctx context.Context, req admission.AdmissionRequest) admission.AdmissionResponse {
    // Use shared authwebhook.ExtractAuthenticatedUser()
    // Use shared authwebhook.EmitAuditEvent()
}
```

**Step 3: Register Handler** (< 1 hour)
```go
// cmd/auth-webhook/main.go
mgr.GetWebhookServer().Register("/authenticate/newcrd", &webhook.Admission{Handler: newCRDHandler})
```

**Step 4: Update MutatingWebhookConfiguration** (< 1 hour)
```yaml
- name: newcrd.kubernaut.ai
  clientConfig:
    service:
      path: /authenticate/newcrd
  rules:
  - operations: ["UPDATE"]
    apiGroups: ["kubernaut.ai"]
    resources: ["newcrds/status"]
```

**Total Time**: < 1 day per new CRD

---

## 📚 **References**

### **Business Requirements**
- [BR-WE-013](../../services/crd-controllers/03-workflowexecution/BUSINESS_REQUIREMENTS.md) - Block clearance authentication
- [ADR-040](./ADR-040-remediation-approval-request-architecture.md) - Approval request architecture

### **Compliance**
- [AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md](../../handoff/AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md) - SOC2 v1.0 approval
- [WE_BR_WE_013_SOC2_COMPLIANCE_TRIAGE_DEC_19_2025.md](../../handoff/WE_BR_WE_013_SOC2_COMPLIANCE_TRIAGE_DEC_19_2025.md) - SOC2 analysis

### **Implementation**
- [SHARED_AUTHENTICATION_WEBHOOK_TRIAGE_DEC_19_2025.md](../../handoff/SHARED_AUTHENTICATION_WEBHOOK_TRIAGE_DEC_19_2025.md) - Triage analysis
- [BR_WE_013_BLOCK_CLEARANCE_ADDENDUM_V1.0.md](../../services/crd-controllers/03-workflowexecution/implementation/BR_WE_013_BLOCK_CLEARANCE_ADDENDUM_V1.0.md) - WE implementation plan

---

## ✅ **Decision Rationale**

**Why Shared Webhook?**

1. **SOC2 Compliance**: Real user authentication from K8s auth context (CC8.1)
2. **Code Reuse**: Single library for consistent authentication across services
3. **Operational Efficiency**: One deployment vs N separate webhooks
4. **K8s-Native**: Leverages existing K8s RBAC and authentication
5. **Extensibility**: Easy to add new CRDs (< 1 day per CRD)
6. **Lower Total Cost**: 5 days vs 6+ days for service-specific webhooks

**Why NOT Alternatives?**

- **Annotations**: ❌ No authentication (SOC2 non-compliant)
- **Custom API**: ⚠️ Higher operational overhead, non-standard UX
- **Service-Specific Webhooks**: ❌ Code duplication, higher maintenance

---

## 📝 **Confidence Assessment**

**Overall Confidence**: 95%

**Justification**:
- ✅ **Well-established pattern**: K8s admission webhooks are battle-tested
- ✅ **Clear requirements**: SOC2 CC8.1 (Attribution) is well-defined
- ✅ **Proven architecture**: Used by cert-manager, OPA, Istio
- ✅ **Low risk**: K8s-native authentication is highly reliable

**Risks** (5%):
- **Certificate Management**: Mitigated by cert-manager integration
- **Webhook Availability**: Mitigated by HA deployment (2+ replicas)
- **K8s Version Compatibility**: Mitigated by using stable admission API v1

---

**Document Status**: ✅ **APPROVED**
**Last Updated**: December 19, 2025
**Version**: 1.0



