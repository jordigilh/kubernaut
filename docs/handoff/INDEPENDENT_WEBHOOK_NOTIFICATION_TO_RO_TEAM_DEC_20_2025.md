# 📢 Notification to RemediationOrchestrator (RO) Team: Independent Webhook Using Operator-SDK

**Date**: December 20, 2025
**From**: WorkflowExecution (WE) Team
**To**: RemediationOrchestrator (RO) Team
**Subject**: ✅ **Independent Authentication Webhook for User Identity - Operator-SDK Scaffolding**
**Priority**: **HIGH** - SOC2 v1.0 Requirement

---

## 🎯 **TL;DR**

✅ **Good News**: RO will implement its **own independent webhook** using operator-sdk scaffolding to solve the "WHO approved/rejected this remediation?" problem.

**What This Means for RO**:
- ✅ Real user authentication from Kubernetes auth context
- ✅ SOC2 compliant (CC8.1 Attribution requirement)
- ✅ **RO team owns the webhook** (independent deployment lifecycle)
- ✅ **Operator-SDK scaffolding** (production-ready code generated)
- ✅ **Shared authentication library** (`pkg/authwebhook`) for code reuse
- ✅ **Standard Kubernetes pattern** (each operator owns its webhooks)

**Timeline**: **3-4 days** (independent implementation)

---

## 🔍 **Background**

### **The Problem You're Solving**

**RemediationApprovalRequest (ADR-040)** requires tracking:
- **WHO** approved/rejected the remediation?
- **WHEN** was the decision made?
- **WHY** (decision message)?

**SOC2 Requirement**:
- **CC8.1 (Attribution)**: Must capture **authenticated** user identity
- **CC4.2 (Change Tracking)**: Must record WHO made changes in audit trail

### **Why NOT Annotations?**

❌ **Annotations are NOT authenticated**:
- Anyone can write arbitrary values
- No way to verify the user identity is real
- SOC2 non-compliant (fails CC8.1 Attribution)

### **The Solution: Independent Webhook with Shared Library**

✅ **RO will create its own webhook** using operator-sdk scaffolding:
- Extracts **real** user identity from Kubernetes authentication context
- Populates authenticated fields in CRD status
- Emits audit events with authenticated actor
- Uses **shared `pkg/authwebhook` library** for consistency with WE

**Why Independent Webhooks?**
- ✅ **Team autonomy**: RO team owns deployment lifecycle
- ✅ **Standard K8s pattern**: Each operator owns its webhooks
- ✅ **Fault isolation**: RO webhook independent from WE
- ✅ **Simpler RBAC**: Minimal permissions per webhook

---

## 🏗️ **Architecture**

### **Independent Webhooks Pattern**

```
┌─────────────────────────────────────────────────────────┐
│  kubernaut-workflowexecution-webhook (WE Team owns)     │
│  └── /mutate-kubernaut-ai-v1alpha1-workflowexecution    │
│      ↓ imports pkg/authwebhook                          │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│  pkg/authwebhook (Shared Library - ~200 LOC)           │
│  ├── authenticator.go (Extract user from K8s auth)     │
│  ├── validator.go (Validate requests)                  │
│  └── audit.go (Emit audit events)                      │
└─────────────────────────────────────────────────────────┘
                              ↑
┌─────────────────────────────────────────────────────────┐
│  kubernaut-remediationorchestrator-webhook (RO Team)   │
│  └── /mutate-kubernaut-ai-v1alpha1-remediationapproval │
│      ← YOU IMPLEMENT THIS! ← Scaffolded by operator-sdk│
└─────────────────────────────────────────────────────────┘
```

---

## 🔧 **How RO Service Will Use This**

### **CRD Schema Pattern**

**Your RemediationApprovalRequest CRD will have**:

```go
type RemediationApprovalRequestStatus struct {
    // ... existing fields ...

    // ApprovalRequest: Operator creates this (unauthenticated input)
    // +optional
    ApprovalRequest *ApprovalRequest `json:"approvalRequest,omitempty"`

    // Decision: Webhook populates these (AUTHENTICATED output)
    Decision        ApprovalDecision `json:"decision,omitempty"`        // "Approved" | "Rejected"
    DecidedBy       string          `json:"decidedBy,omitempty"`       // AUTHENTICATED by webhook
    DecidedAt       *metav1.Time    `json:"decidedAt,omitempty"`
    DecisionMessage string          `json:"decisionMessage,omitempty"` // From request
}

type ApprovalRequest struct {
    Decision        ApprovalDecision `json:"decision"`        // "Approved" | "Rejected"
    DecisionMessage string          `json:"decisionMessage"` // User-provided reason
    RequestedAt     metav1.Time     `json:"requestedAt"`
}

type ApprovalDecision string

const (
    ApprovalDecisionApproved ApprovalDecision = "Approved"
    ApprovalDecisionRejected ApprovalDecision = "Rejected"
)
```

### **Operator Workflow**

**Step 1: Operator creates approval request** (unauthenticated):

```bash
kubectl patch remediationapprovalrequest rar-test \
  --type=merge \
  --subresource=status \
  -p '{"status":{"approvalRequest":{"decision":"Approved","decisionMessage":"verified cluster state, safe to proceed","requestedAt":"2025-12-20T10:00:00Z"}}}' \
  -n kubernaut-system
```

**Step 2: Webhook intercepts and authenticates** (automatic):

1. K8s API Server authenticates user via OIDC/certs/SA token
2. Sends request to MutatingWebhook with `req.UserInfo`
3. Webhook extracts **REAL** user identity
4. Populates authenticated fields in status

**Step 3: Result in CRD status**:

```yaml
status:
  decision: "Approved"
  decidedBy: "operator@example.com (UID: xyz-789)"  # ← AUTHENTICATED
  decidedAt: "2025-12-20T10:00:05Z"
  decisionMessage: "verified cluster state, safe to proceed"
  approvalRequest: null  # ← Cleared after processing
```

---

## 📊 **SOC2 Compliance**

| SOC2 Requirement | Implementation | RO Benefit |
|------------------|----------------|------------|
| **CC8.1** - Attribution | User from K8s auth context (`req.UserInfo`) | Track WHO approved/rejected |
| **CC7.3** - Immutability | Original RAR preserved (not deleted) | Complete audit trail |
| **CC7.4** - Completeness | No gaps in audit trail | All decisions recorded |
| **CC4.2** - Change Tracking | Authenticated actor in audit event | Compliance-ready |

---

## 🚀 **Implementation Timeline**

**Total**: **3-4 days** (RO team owns this!)

| Day | Focus | Owner | Deliverables |
|-----|-------|-------|--------------|
| **Day 1** (4h) | Shared library (`pkg/authwebhook`) | **WE Team** | Reusable authentication library ✅ |
| **Day 2** (8h) | WFE webhook scaffolding + implementation | **WE Team** | Reference implementation for RO |
| **Day 3** (8h) | **RAR webhook scaffolding + implementation** | **RO Team** ← **YOU!** | **YOUR WEBHOOK** |
| **Day 4** (4h) | Integration + E2E tests | **RO Team** | Test coverage for RAR webhook |

**RO-Specific Work** (Day 3-4):
- **Scaffolding**: `kubebuilder create webhook --group remediationorchestrator --version v1alpha1 --kind RemediationApprovalRequest --defaulting --programmatic-validation`
- **Implementation**: `api/remediationorchestrator/v1alpha1/remediationapprovalrequest_webhook.go`
- **Logic**: Populate `decidedBy` with authenticated user using `pkg/authwebhook`
- **Tests**: 8 unit tests + 3 integration tests + 2 E2E tests
- **Audit**: Emit `remediationapprovalrequest.decision` event

---

## 📚 **Authoritative References**

### **MUST READ** ⭐
1. **[ADR-WEBHOOK-001: Operator-SDK Webhook Scaffolding Pattern](../architecture/decisions/ADR-WEBHOOK-001-operator-sdk-webhook-scaffolding.md)** - **AUTHORITATIVE** design decision
   - Complete webhook scaffolding process using operator-sdk
   - Shared library pattern for code reuse
   - WE and RO webhook examples
   - Best practices and RBAC requirements
2. **[BR-WE-013: Audit-Tracked Block Clearing](../requirements/BR-WE-013-audit-tracked-block-clearing.md)** - WE use case example

### **Supporting Documents**
3. [WEBHOOK_ARCHITECTURE_TRIAGE_OPERATOR_SDK_VS_SHARED_DEC_20_2025.md](./WEBHOOK_ARCHITECTURE_TRIAGE_OPERATOR_SDK_VS_SHARED_DEC_20_2025.md) - Architecture analysis
4. [ADR-040: RemediationApprovalRequest Architecture](../architecture/decisions/ADR-040-remediation-approval-request-architecture.md) - RO approval workflow (if exists)

---

## 🎯 **Action Items for RO Team**

### **Immediate (This Week)**

1. **Review ADR-WEBHOOK-001** (1 hour) ⭐ **MOST IMPORTANT**
   - Understand operator-sdk webhook scaffolding process
   - Review shared `pkg/authwebhook` library usage
   - Study WE webhook as reference implementation

2. **Update RemediationApprovalRequest CRD Schema** (2 hours)
   - Add `approvalRequest` field (operator input)
   - Add authenticated fields: `decidedBy`, `decidedAt`, `decision`, `decisionMessage`
   - Regenerate CRD manifests

3. **Review Shared Library** (30 minutes)
   - Check `pkg/authwebhook/` implementation
   - Understand authenticator, validator, audit interfaces
   - Ask WE team for clarifications if needed

### **Next Week (Implementation)**

4. **Scaffold RO Webhook** (Day 3, 2 hours)
   - Run `kubebuilder create webhook` command
   - Review generated files

5. **Implement Authentication Logic** (Day 3, 4 hours)
   - Implement `Default()` method (use `pkg/authwebhook`)
   - Implement `ValidateUpdate()` method
   - 8 unit tests

6. **Integration + E2E Testing** (Day 4, 4 hours)
   - 3 integration tests (envtest)
   - 2 E2E tests (Kind cluster)
   - Verify audit events

7. **Update RO Documentation**
   - Operator runbooks (how to approve/reject remediations)
   - SOC2 compliance documentation
   - Audit trail examples

---

## 💡 **Example: Complete Approval Flow**

### **Scenario**: Operator approves a remediation for critical pod restart

**Step 1: Operator action**:
```bash
kubectl patch remediationapprovalrequest rar-critical-pod-restart \
  --type=merge \
  --subresource=status \
  -p '{"status":{"approvalRequest":{"decision":"Approved","decisionMessage":"incident commander approval - critical service down","requestedAt":"2025-12-20T14:30:00Z"}}}' \
  -n production
```

**Step 2: Webhook authentication** (automatic):
- K8s authenticates user: `incident-commander@example.com`
- Webhook extracts user identity
- Populates authenticated fields

**Step 3: Result in RAR status**:
```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationApprovalRequest
metadata:
  name: rar-critical-pod-restart
  namespace: production
status:
  decision: "Approved"
  decidedBy: "incident-commander@example.com (UID: abc-123)"
  decidedAt: "2025-12-20T14:30:05Z"
  decisionMessage: "incident commander approval - critical service down"
```

**Step 4: Audit event emitted**:
```json
{
  "event_type": "remediationapprovalrequest.decision",
  "event_category": "remediation",
  "event_action": "decision.approved",
  "event_outcome": "success",
  "actor_type": "user",
  "actor_id": "incident-commander@example.com (UID: abc-123)",
  "resource_type": "RemediationApprovalRequest",
  "resource_name": "rar-critical-pod-restart",
  "event_data": {
    "decision": "Approved",
    "decided_by": "incident-commander@example.com (UID: abc-123)",
    "decision_message": "incident commander approval - critical service down"
  }
}
```

**Step 5: RO Controller reacts**:
- Detects `status.decision = "Approved"`
- Proceeds with remediation execution
- Tracks `decidedBy` in remediation history

---

## 🔐 **RBAC Requirements**

### **Operator Permissions** (RO Team to configure)

Operators need `update` permission on `/status` subresource:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: remediation-operator
  namespace: production
rules:
- apiGroups: ["kubernaut.ai"]
  resources: ["remediationapprovalrequests/status"]
  verbs: ["get", "update", "patch"]
```

### **Webhook ServiceAccount** (RO Team configures - auto-generated by operator-sdk)

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut-remediationorchestrator-webhook
rules:
# Read RARs to validate requests
- apiGroups: ["kubernaut.ai"]
  resources: ["remediationapprovalrequests"]
  verbs: ["get", "list", "watch"]

# Update status with authenticated data
- apiGroups: ["kubernaut.ai"]
  resources: ["remediationapprovalrequests/status"]
  verbs: ["update", "patch"]
```

---

## 🎁 **Benefits for RO Service**

| Benefit | Details |
|---------|---------|
| **Team Autonomy** | RO team owns webhook deployment lifecycle |
| **SOC2 Compliant** | CC8.1 Attribution requirement satisfied |
| **Standard Pattern** | Follows Kubernetes operator best practices |
| **Code Reuse** | Shared `pkg/authwebhook` library (~200 LOC) |
| **Operator-SDK** | Production-ready scaffolding (33% less manual code) |
| **Fault Isolation** | RO webhook independent from WE |
| **Audit Trail** | Authenticated actor in all approval decision events |

---

## ❓ **FAQs**

### **Q1: Do we need to deploy our own webhook?**

✅ **Yes!** RO team owns the webhook deployment (standard operator pattern). Operator-SDK scaffolds all deployment configuration.

### **Q2: Can operators still use kubectl to approve remediations?**

✅ **Yes!** Standard kubectl command:
```bash
kubectl patch remediationapprovalrequest <name> --type=merge --subresource=status -p '...'
```

### **Q3: What if we want custom validation logic for RO?**

✅ **Fully supported!** The RO webhook can include RO-specific validation:
- Check remediation prerequisites
- Validate operator permissions
- Enforce approval windows

### **Q4: How do we test this in staging?**

✅ **Complete test scaffolding included**:
- Unit tests (operator-sdk generates test suite)
- Integration tests (envtest with real K8s API)
- E2E tests (Kind cluster)

### **Q5: What about code duplication with WE webhook?**

✅ **Minimal duplication via shared library**:
- Shared authentication logic in `pkg/authwebhook` (~200 LOC)
- Both webhooks import the same library
- Consistency enforced by shared code

### **Q6: How does this work with WE's webhook?**

✅ **Completely independent**:
- WE webhook handles WorkflowExecution CRDs
- RO webhook handles RemediationApprovalRequest CRDs
- No coordination needed between webhooks
- Each team deploys independently

---

## 📞 **Contact & Coordination**

### **WE Team Support**
- **Shared Library**: `pkg/authwebhook` (Day 1 - already completed)
- **Reference Implementation**: WE webhook (Day 2 - available for review)
- **Questions**: Review ADR-WEBHOOK-001 first, then reach out

### **RO Team Action Required**
1. Review ADR-WEBHOOK-001 (1 hour) ⭐
2. Update RAR CRD schema (2 hours)
3. Review `pkg/authwebhook` library (30 min)
4. Scaffold + implement RO webhook (Day 3-4)

### **Questions?**
- **Technical**: See ADR-WEBHOOK-001 (comprehensive guide)
- **Shared Library**: Check `pkg/authwebhook/README.md`
- **SOC2 Compliance**: See [SOC2_V1_0_MVP_WORK_TRIAGE_DEC_20_2025.md](./SOC2_V1_0_MVP_WORK_TRIAGE_DEC_20_2025.md)

---

## ✅ **Next Steps**

**For RO Team**:
1. ✅ **Read ADR-WEBHOOK-001** (AUTHORITATIVE design decision) ⭐ **START HERE**
2. ✅ **Review `pkg/authwebhook` library** (shared authentication code)
3. ✅ **Update RAR CRD schema** (add `approvalRequest` + authenticated fields)
4. ✅ **Scaffold RO webhook** (Day 3: `kubebuilder create webhook`)
5. ✅ **Implement authentication logic** (Day 3: use `pkg/authwebhook`)
6. ✅ **Test webhook** (Day 4: unit + integration + E2E)

**For WE Team**:
1. ✅ **Shared library complete** (`pkg/authwebhook`) - Day 1
2. ✅ **WE webhook implementation** (reference for RO) - Day 2
3. ✅ **Available for questions** (advisory support for RO)

---

## 🎯 **Summary**

✅ **Independent webhook pattern using operator-SDK scaffolding**
✅ **Solves SOC2 CC8.1 Attribution requirement for RO**
✅ **RO team owns deployment lifecycle (standard K8s pattern)**
✅ **Shared authentication library for code reuse**
✅ **3-4 day implementation timeline**
✅ **Production-ready scaffolding reduces manual work by 33%**

**Each team owns its webhook independently!**

---

**Document Status**: ✅ **READY FOR REVIEW**
**Notification Sent**: December 20, 2025
**Authoritative Reference**: [ADR-WEBHOOK-001](../architecture/decisions/ADR-WEBHOOK-001-operator-sdk-webhook-scaffolding.md) ⭐

