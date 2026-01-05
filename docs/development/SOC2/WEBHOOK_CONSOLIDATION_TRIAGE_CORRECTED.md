# Webhook Consolidation Triage - SOC2 Compliance (CORRECTED)

**Date**: January 6, 2026  
**Status**: ✅ **ANALYSIS COMPLETE - CORRECTED**  
**Purpose**: Evaluate consolidating multiple admission webhooks into a single implementation  
**Correction**: Initial triage missed 2 CRDs - **4 CRDs total**, not 2  
**Authoritative Sources**:
- `DD-AUTH-001`: Shared Authentication Webhook (AUTHORITATIVE)
- `DD-WEBHOOK-001`: CRD Webhook Requirements Matrix (December 2025 - needs v1.1 update)
- `TRIAGE_OPERATOR_ACTIONS_SOC2_EXTENSION.md`: SOC2 Extension (January 2026)
- `ADR-051`: Operator-SDK Webhook Scaffolding

---

## 🚨 **CORRECTION: 4 CRDs Require Webhooks, Not 2**

### **Initial Error**

❌ **INCORRECT STATEMENT**: "Only 2 CRDs - Small scope, easy implementation"

**Root Cause**: I only read DD-WEBHOOK-001 (December 20, 2025) which lists 2 CRDs. However, `TRIAGE_OPERATOR_ACTIONS_SOC2_EXTENSION.md` (January 4, 2026) identified 2 additional CRDs requiring webhooks for Week 2-3 SOC2 extension.

---

## 📋 **CORRECTED: CRD Webhook Requirements Matrix**

### **CRDs Requiring Webhooks** ✅ (4 TOTAL)

| CRD | Use Case | Operation Type | SOC2 Control | DD-WEBHOOK-001 Status | Priority |
|-----|----------|----------------|--------------|----------------------|----------|
| **WorkflowExecution** | Block Clearance | Status Update (manual) | CC8.1 (Attribution) | ✅ v1.0 (Dec 2025) | P0 |
| **RemediationApprovalRequest** | Approval Decisions | Status Update (manual) | CC8.1 (Attribution) | ✅ v1.0 (Dec 2025) | P0 |
| **NotificationRequest** | Cancellation Attribution | DELETE operation | CC8.1 (Attribution) | ⚠️  v1.1 (pending) | P0 |
| **RemediationWorkflow** | Catalog CRUD Attribution | CREATE/UPDATE operations | CC8.1 (Attribution) | ⚠️  v1.1 (pending) | P0 |

### **CRDs NOT Requiring Webhooks** ❌

| CRD | Reason |
|-----|--------|
| **SignalProcessing** | Controller-only status updates |
| **AIAnalysis** | Controller-only AI investigation results |
| **RemediationRequest** | Controller-only routing logic |

---

## 🔍 **Detailed CRD Analysis**

### **CRD 1: WorkflowExecution** ✅ (Already in DD-WEBHOOK-001 v1.0)

**Use Case**: Block Clearance (BR-WE-013)

**Why Webhook Required**:
1. ✅ **Manual Intervention**: Operator manually patches `status.blockClearanceRequest`
2. ✅ **SOC2 CC8.1**: Must record WHO cleared the block
3. ✅ **Override Action**: Operator overrides controller's failure block
4. ✅ **Operational Decision**: Clearance requires human judgment

**Status Fields**:
- **Operator Input** (unauthenticated): `status.blockClearanceRequest`
  - `clearReason`: Operator's explanation
  - `requestedAt`: Request timestamp
- **Webhook Output** (authenticated): `status.blockClearance`
  - `clearedBy`: Authenticated user from K8s auth context
  - `clearedAt`: Server-side timestamp
  - `clearReason`: Copied from request
  - `clearMethod`: "KubernetesAdmissionWebhook"

**Audit Event**: `workflowexecution.block.cleared`

**Webhook Type**: **MutatingWebhookConfiguration** (Status Update)

---

### **CRD 2: RemediationApprovalRequest** ✅ (Already in DD-WEBHOOK-001 v1.0)

**Use Case**: Approval Decisions (ADR-040)

**Why Webhook Required**:
1. ✅ **Manual Intervention**: Operator manually patches `status.approvalRequest`
2. ✅ **SOC2 CC8.1**: Must record WHO approved/rejected remediation
3. ✅ **Approval Workflow**: Explicit human approval required for high-risk actions
4. ✅ **Operational Decision**: Approval requires human risk assessment

**Status Fields**:
- **Operator Input** (unauthenticated): `status.approvalRequest`
  - `decision`: "Approved" | "Rejected"
  - `decisionMessage`: Operator's rationale
  - `requestedAt`: Request timestamp
- **Webhook Output** (authenticated):
  - `decision`: Copied from request
  - `decidedBy`: Authenticated user from K8s auth context
  - `decidedAt`: Server-side timestamp
  - `decisionMessage`: Copied from request

**Audit Events**: 
- `orchestrator.approval.approved` (already in DD-AUDIT-003 v1.2)
- `orchestrator.approval.rejected` (already in DD-AUDIT-003 v1.2)

**Webhook Type**: **MutatingWebhookConfiguration** (Status Update)

---

### **CRD 3: NotificationRequest** ⚠️  (Missing from DD-WEBHOOK-001 - needs v1.1)

**Use Case**: Notification Cancellation Attribution

**Why Webhook Required**:
1. ✅ **Manual Intervention**: Operator manually deletes NotificationRequest CRD
2. ✅ **SOC2 CC8.1**: Must record WHO cancelled the notification
3. ✅ **Override Action**: Operator cancels automated notification delivery
4. ✅ **Operational Decision**: Cancellation requires human judgment

**Operation**: `kubectl delete notificationrequest <nr-name> -n <namespace>`

**Metadata Fields**:
- `metadata.deletionTimestamp`: Kubernetes sets this on DELETE
- `metadata.finalizers`: Webhook must capture identity before allowing deletion

**Audit Event**: `notification.request.cancelled` (NEW - needs DD-AUDIT-003 v1.4)

**Event Data**:
```json
{
  "notification_request_id": "nr-approval-rr-123",
  "remediation_request_id": "rr-oomkill-abc123",
  "notification_type": "approval",
  "cancelled_by": {
    "username": "operator@example.com",
    "uid": "k8s-user-uuid",
    "groups": ["platform-admins"]
  },
  "cancellation_reason": "Issue manually resolved",
  "notification_phase": "Pending"
}
```

**Webhook Type**: **ValidatingWebhookConfiguration** (DELETE operation with finalizer)

**Implementation Pattern**:
1. Webhook intercepts DELETE operation
2. Extract authenticated user from `req.UserInfo`
3. Emit `notification.request.cancelled` audit event with authenticated actor
4. Allow DELETE to proceed (remove finalizer)

---

### **CRD 4: RemediationWorkflow** ⚠️  (Missing from DD-WEBHOOK-001 - needs v1.1)

**Use Case**: Workflow Catalog CRUD Attribution

**Why Webhook Required**:
1. ✅ **Manual Intervention**: Operator creates/updates RemediationWorkflow CRD
2. ✅ **SOC2 CC8.1**: Must record WHO created/modified the workflow
3. ✅ **Operational Decision**: Workflow CRUD affects system behavior
4. ✅ **Compliance**: Track workflow lineage for audit purposes

**Operations**:
- CREATE: `kubectl apply -f restart-pod-workflow.yaml`
- UPDATE: `kubectl apply -f restart-pod-workflow.yaml` (modified)
- DISABLE: `POST /api/v1/workflows/{id}/disable` (via Data Storage API)

**Metadata Fields**:
- `metadata.creationTimestamp`: Kubernetes sets this on CREATE
- `metadata.annotations["kubernaut.ai/created-by"]`: Webhook populates from `req.UserInfo`
- `metadata.annotations["kubernaut.ai/modified-by"]`: Webhook populates on UPDATE

**Audit Events** (already in DD-AUDIT-003 v1.2):
- `datastorage.workflow.created`
- `datastorage.workflow.updated` (includes disable operation)

**Event Data**:
```json
{
  "workflow_id": "restart-pod-workflow",
  "workflow_version": "v1.2.3",
  "operation": "created", // or "disabled"
  "created_by": {
    "username": "operator@example.com",
    "uid": "k8s-user-uuid",
    "groups": ["workflow-admins"]
  },
  "workflow_metadata": {
    "title": "Restart Pod Workflow",
    "labels": ["pod", "restart", "oomkill"]
  }
}
```

**Webhook Type**: **MutatingWebhookConfiguration** (CREATE/UPDATE operations)

**Implementation Pattern**:
1. Webhook intercepts CREATE/UPDATE operations
2. Extract authenticated user from `req.UserInfo`
3. Populate `metadata.annotations["kubernaut.ai/created-by"]` or `["modified-by"]`
4. Controller emits audit event using annotation value

---

## 🏗️ **CORRECTED: Consolidated Architecture (4 CRDs)**

```
┌────────────────────────────────────────────────────────────┐
│           kubernaut-auth-webhook (Single Deployment)        │
│                                                             │
│  MutatingWebhookConfiguration (3 rules):                   │
│    1. workflowexecutions.kubernaut.ai/status               │
│       → /authenticate/workflowexecution                    │
│    2. remediationapprovalrequests.kubernaut.ai/status      │
│       → /authenticate/remediationapproval                  │
│    3. remediationworkflows.kubernaut.ai                    │
│       → /authenticate/workflow                             │
│                                                             │
│  ValidatingWebhookConfiguration (1 rule):                  │
│    1. notificationrequests.kubernaut.ai (DELETE)           │
│       → /authenticate/notification-delete                  │
│                                                             │
│  Shared Authentication (pkg/authwebhook/):                 │
│    ✅ ExtractUser() - Extract K8s authenticated user       │
│    ✅ ValidateReason() - Validate operator reason          │
│    ✅ ValidateTimestamp() - Validate request timestamp     │
│                                                             │
│  CRD-Specific Handlers (internal/webhook/handlers/):      │
│    - WorkflowExecutionAuthHandler (mutating)               │
│    - RemediationApprovalAuthHandler (mutating)             │
│    - RemediationWorkflowAuthHandler (mutating)             │
│    - NotificationRequestDeleteHandler (validating)         │
└────────────────────────────────────────────────────────────┘
```

**Key Differences from Initial Analysis**:
- ✅ **4 handlers** instead of 2
- ✅ **Both Mutating AND Validating** webhook configurations
- ✅ **3 mutating rules** (WFE, RAR, RW) + **1 validating rule** (NR DELETE)

---

## 📊 **CORRECTED: Comparison Matrix**

| Aspect | Independent Webhooks (4) | Consolidated Webhook | Winner |
|--------|--------------------------|----------------------|--------|
| **Deployments** | 4 separate | 1 shared | ✅ |
| **Services** | 4 separate | 1 shared | ✅ |
| **TLS Certificates** | 4 separate | 1 shared | ✅ |
| **Ports** | 9443-9446 | 9443 only | ✅ |
| **WebhookConfigurations** | 4 separate | 2 shared | ✅ |
| **Code Duplication** | Very High | None | ✅ |
| **Resource Usage** | 4x pods/CPU/memory | 1x | ✅ |
| **Operational Overhead** | Very High | Low | ✅ |
| **Maintenance** | 4 codebases | 1 codebase | ✅ |
| **Consistency** | High risk divergence | Guaranteed | ✅ |
| **Extensibility** | New deployment per CRD | Add handler only | ✅ |
| **DD-AUTH-001 Compliance** | ❌ Violates | ✅ Mandated | ✅ |

**Winner**: ✅ **Consolidated Webhook** (12/12 advantages)

**Impact of 4 CRDs vs. 2**:
- Independent approach would require **4 deployments** (2x worse than initially thought)
- Consolidated approach becomes **even more compelling** with more CRDs

---

## 📈 **Effort Analysis - CORRECTED**

### **Initial Estimate (2 CRDs)**: 6 days

### **Corrected Estimate (4 CRDs)**: 8-10 days

**Breakdown**:

#### **Phase 1: Webhook Service Scaffolding** (1.5 days)
- Create `cmd/webhooks/main.go` with HTTP server
- Support both Mutating AND Validating webhook configurations
- Register 4 endpoint handlers
- **+0.5 days** for ValidatingWebhookConfiguration support

#### **Phase 2: CRD Handlers** (4 days)
- `internal/webhook/handlers/workflowexecution_handler.go` (1 day)
- `internal/webhook/handlers/remediationapproval_handler.go` (1 day)
- `internal/webhook/handlers/remediationworkflow_handler.go` (1 day)
- `internal/webhook/handlers/notificationrequest_delete_handler.go` (1 day)
- **+2 days** for 2 additional handlers

#### **Phase 3: Kubernetes Configuration** (1.5 days)
- `config/webhook/manifests.yaml` - Deployment, Service, 2 WebhookConfigurations
- `config/webhook/kustomization.yaml` - cert-manager integration
- **+0.5 days** for dual webhook configuration (Mutating + Validating)

#### **Phase 4: Testing** (3 days)
- 72 unit tests (18 per CRD × 4 CRDs)
- 12 integration tests (3 per CRD × 4 CRDs)
- 8 E2E tests (2 per CRD × 4 CRDs)
- **+1 day** for 2 additional CRD test suites

**Total**: **10 days** (2 weeks) - **+4 days** from initial estimate

---

## 🔍 **Timeline vs. SOC2 Phases**

### **Week 1 (Current): RR Reconstruction** (Jan 6-13, 2026)
- Day 1-2: Gateway audit (✅ COMPLETE)
- Day 3-5: AI Analysis audit
- Day 6-7: Workflow Selection & Execution audit
- **NO webhooks** - focusing on RR reconstruction only

### **Week 2-3: Operator Attribution** (Jan 14-27, 2026)
- Days 7-8: Shared library (`pkg/authwebhook`) (already ✅ COMPLETE)
- Days 9-14: **4 webhook handlers** (corrected from 2)
- Days 15-17: Kubernetes config + testing
- Days 18-19: Integration + E2E tests

**Impact of Correction**:
- Week 2-3 effort: **64-80 hours** (original estimate was accurate)
- Webhook implementation: **8-10 days** (corrected from 6 days)

---

## ✅ **CORRECTED Recommendation**

### **✅ STILL RECOMMEND: Consolidated Webhook Approach**

**Rationale** (even stronger with 4 CRDs):
1. ✅ **DD-AUTH-001 is AUTHORITATIVE** - Consolidation is **MANDATORY**
2. ✅ **Shared library exists** - `pkg/authwebhook/` ready for use
3. ✅ **4 CRDs** - Consolidation **EVEN MORE CRITICAL** with more webhooks
4. ✅ **Superior architecture** - 4 independent webhooks would be **operational nightmare**
5. ✅ **SOC2 compliant** - Centralized audit trail for all operator actions

**Why Consolidation is MORE Important with 4 CRDs**:
- Independent approach: **4 deployments, 4 services, 4 TLS certs**
- Consolidated approach: **1 deployment, 1 service, 1 TLS cert**
- **Operational savings**: 4x → 1x (75% reduction in infrastructure)
- **Maintenance savings**: 4 codebases → 1 codebase

---

## 📋 **CORRECTED: Implementation Status**

| Component | Status | Next Action |
|-----------|--------|-------------|
| **Shared Library** | ✅ **COMPLETE** | None - ready to use |
| **Webhook Service** | ❌ **NOT STARTED** | Create `cmd/webhooks/main.go` |
| **CRD Handlers** | ❌ **NOT STARTED** | Implement **4 handlers** (corrected) |
| **K8s Config** | ❌ **NOT STARTED** | Create **2 webhook configs** (Mutating + Validating) |
| **Tests** | ❌ **NOT STARTED** | **92 tests** (72 unit + 12 integration + 8 E2E) |

---

## 📚 **Authority Document Updates Needed**

### **1. DD-WEBHOOK-001 v1.1** (CRITICAL UPDATE)

**Add 2 new CRDs to "CRDs Requiring Webhooks" table**:

| CRD | Use Case | Status Fields Requiring Auth | SOC2 Control | Priority |
|-----|----------|------------------------------|--------------|----------|
| **NotificationRequest** | Cancellation Attribution | `metadata.deletionTimestamp` | CC8.1 (Attribution) | P0 |
| **RemediationWorkflow** | Catalog CRUD Attribution | `metadata.creationTimestamp` | CC8.1 (Attribution) | P0 |

**Remove from "CRDs NOT Requiring Webhooks" table**:
- ❌ **NotificationRequest** (MOVED to requiring webhooks)

---

### **2. DD-AUDIT-003 v1.4** (MINOR UPDATE)

**Add 1 new event type**:
- `notification.request.cancelled`

**Clarify 3 existing event types**:
- `datastorage.workflow.created` (add: "Captures authenticated operator identity")
- `datastorage.workflow.updated` (add: "Includes disable operation with operator identity")
- `orchestrator.approval.*` (add: "Captures authenticated operator identity")

---

## 🎯 **Final Comparison: 2 CRDs vs. 4 CRDs**

| Aspect | 2 CRDs (Initial) | 4 CRDs (Corrected) | Impact |
|--------|------------------|-------------------|--------|
| **Webhook Handlers** | 2 | 4 | +100% |
| **WebhookConfigurations** | 1 (Mutating) | 2 (Mutating + Validating) | +100% |
| **Implementation Time** | 6 days | 8-10 days | +4 days |
| **Test Suite** | 46 tests | 92 tests | +100% |
| **Consolidation Value** | ✅ Beneficial | ✅ **CRITICAL** | Higher |
| **Independent Overhead** | 2x deployments | **4x deployments** | Much worse |

**Conclusion**: ✅ **Consolidation is EVEN MORE IMPORTANT with 4 CRDs**

---

## ✅ **Corrected Final Status**

**CRDs Requiring Webhooks**: ✅ **4 CRDs** (corrected from 2)

1. ✅ **WorkflowExecution** (Status Update - Block Clearance)
2. ✅ **RemediationApprovalRequest** (Status Update - Approval/Rejection)
3. ✅ **NotificationRequest** (DELETE operation - Cancellation)
4. ✅ **RemediationWorkflow** (CREATE/UPDATE - CRUD Attribution)

**Recommendation**: ✅ **CONSOLIDATED WEBHOOK** (even more critical with 4 CRDs)

**Estimated Timeline**: **8-10 days** (corrected from 6 days)

**Next**: Update DD-WEBHOOK-001 to v1.1 + implement 4 CRD handlers

---

**Status**: ✅ **ANALYSIS COMPLETE - CORRECTED**  
**Correction Applied**: January 6, 2026  
**User Feedback**: ✅ Acknowledged - triage corrected  
**Recommendation**: ✅ **CONSOLIDATED WEBHOOK** (DD-AUTH-001 mandated)

**Compliance**: DD-AUTH-001, DD-WEBHOOK-001 v1.1, BR-WE-013, SOC2 CC8.1

