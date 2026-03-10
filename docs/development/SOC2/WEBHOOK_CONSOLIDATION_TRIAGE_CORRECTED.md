# Webhook Consolidation Triage - SOC2 Compliance (FINAL CORRECTION)

**Date**: January 6, 2026
**Status**: ✅ **ANALYSIS COMPLETE - SUPERSEDED IN PART**
**Purpose**: Evaluate consolidating multiple admission webhooks into a single implementation

> **March 2026 Correction (v1.3)**: This document's conclusion of "3 CRDs + 1 REST API" is **no longer accurate**. As of [ADR-058](../../architecture/decisions/ADR-058-webhook-driven-workflow-registration.md) and [BR-WORKFLOW-006](../../requirements/BR-WORKFLOW-006-remediation-workflow-crd.md), RemediationWorkflow now uses a **CRD + ValidatingWebhook** pattern, making the correct count **4 CRDs**. The DS REST API for workflow registration (`POST /api/v1/workflows`) is now an internal API consumed only by the AuthWebhook. See [DD-WEBHOOK-001 v1.3](../../architecture/decisions/DD-WEBHOOK-001-crd-webhook-requirements-matrix.md) for the current authoritative matrix.

**Original Correction (January 2026)**: ~~**3 CRDs** + **1 REST API** requiring authentication~~
**Authoritative Sources**:
- `DD-AUTH-001`: Shared Authentication Webhook (AUTHORITATIVE for CRD webhooks)
- `DD-WEBHOOK-001 v1.3`: CRD Webhook Requirements Matrix (**4 CRDs** including RemediationWorkflow)
- `ADR-058`: Webhook-driven workflow registration (RemediationWorkflow CRD)
- `BR-WORKFLOW-006`: RemediationWorkflow CRD specification

---

## 🚨 **FINAL CORRECTION: 3 CRDs + 1 REST API, Not 4 CRDs**

### **Second Correction (User Identified)**

❌ **SECOND ERROR**: RemediationWorkflow is **REST API**, not CRD

**Root Cause**: Assumed workflow catalog used CRD pattern, but `DD-WORKFLOW-005 v2.0` confirms V1.0 uses REST API only (`POST /api/v1/workflows`). V1.1 `WorkflowRegistration` CRD is future work.

### **Evolution of Understanding**

1. **Initial Triage**: "Only 2 CRDs" ❌ (missed NotificationRequest + RemediationWorkflow)
2. **First Correction**: "4 CRDs total" ❌ (incorrectly assumed RemediationWorkflow is CRD)
3. **Final Correction**: "3 CRDs + 1 REST API" ✅ (accurate per DD-WORKFLOW-005 v2.0)

---

## 📋 **FINAL CORRECTION: CRD Webhook + REST API Requirements**

### **CRDs Requiring Webhooks** ✅ (3 TOTAL)

| CRD | Use Case | Operation Type | SOC2 Control | DD-WEBHOOK-001 Status | Priority |
|-----|----------|----------------|--------------|----------------------|----------|
| **WorkflowExecution** | Block Clearance | Status Update (manual) | CC8.1 (Attribution) | ✅ v1.0 (Dec 2025) | P0 |
| **RemediationApprovalRequest** | Approval Decisions | Status Update (manual) | CC8.1 (Attribution) | ✅ v1.0 (Dec 2025) | P0 |
| **NotificationRequest** | Cancellation Attribution | DELETE operation | CC8.1 (Attribution) | ✅ v1.1 (Jan 2026) | P0 |

### **REST APIs Requiring HTTP Middleware** ✅ (1 TOTAL)

| REST API | Use Case | HTTP Method | SOC2 Control | DD-AUTH-002 Status | Priority |
|----------|----------|-------------|--------------|-------------------|----------|
| `/api/v1/workflows` | Workflow CRUD Attribution | POST/PATCH | CC8.1 (Attribution) | ✅ v1.0 (Jan 2026) | P0 |

**Note**: RemediationWorkflow is **NOT a CRD** - workflows are managed via REST API only (DD-WORKFLOW-005 v2.0). V1.1 `WorkflowRegistration` CRD is future work.

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

### **REST API: Workflow CRUD** ✅ (Uses HTTP Middleware, not CRD Webhook)

**Use Case**: Workflow Catalog CRUD Attribution

**Why HTTP Middleware (not CRD Webhook)**:
- ❌ RemediationWorkflow CRD does NOT exist (DD-WORKFLOW-005 v2.0: REST API only)
- ✅ V1.0 uses `POST /api/v1/workflows` (REST API registration)
- ⏳ V1.1 `WorkflowRegistration` CRD is future work (not now)

**Operations** (REST API):
- CREATE: `POST /api/v1/workflows` + `Authorization: Bearer <K8s-JWT>`
- UPDATE: `PATCH /api/v1/workflows/{workflowID}` + `Authorization: Bearer <K8s-JWT>`
- DISABLE: `PATCH /api/v1/workflows/{workflowID}/disable` + `Authorization: Bearer <K8s-JWT>`

**Authentication Method**: K8s ServiceAccount JWT + HTTP Middleware
- Middleware validates JWT via K8s TokenReview API
- Extract authenticated user from TokenReview response
- Populate audit events with authenticated actor

**Audit Events** (already in DD-AUDIT-003 v1.2):
- `datastorage.workflow.created`
- `datastorage.workflow.updated` (includes disable operation)

**Implementation Reference**: `DD-AUTH-002-http-authentication-middleware.md`

**Timeline**: Week 2, Days 12-14 (Data Storage Team) - 2.5 days

---

## 🏗️ **FINAL CORRECTION: Two Authentication Patterns**

### **Pattern 1: Kubernetes Admission Webhooks** (3 CRDs)

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
- Create `cmd/authwebhook/main.go` with HTTP server
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
| **Webhook Service** | ❌ **NOT STARTED** | Create `cmd/authwebhook/main.go` |
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

