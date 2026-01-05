# Triage: SOC2 Extension - Operator Actions Audit Events

**Status**: ✅ **APPROVED - OPTION B (WEEK 2-3 EXTENSION)**
**Date**: January 4, 2026
**Authority**: User request for SOC2 scope expansion
**Related**: SOC2_AUDIT_IMPLEMENTATION_PLAN.md, DD-WEBHOOK-001
**Confidence**: 85%

---

## 🎯 **Context**

**User Request**: Extend SOC2 audit coverage to include 3 additional operator actions:
1. **Operator cancels a workflow** (NotificationRequest deletion)
2. **Operator approves a RAR** (RemediationApprovalRequest approval)
3. **Operator creates/disables a workflow catalog** (Workflow CRUD operations)

**Authority Documents**:
- DD-WEBHOOK-001: CRD Webhook Requirements Matrix
- DD-WORKFLOW-009: Workflow Catalog Storage
- WEBHOOK_IMPLEMENTATION_WORKLOG_NEXT_SPRINT.md
- USER-GUIDE-NOTIFICATION-CANCELLATION.md

**Business Requirement**: BR-AUDIT-005 v2.0 (Enterprise-Grade Audit Integrity) - SOC2 CC8.1 Attribution

---

## 📋 **Discovery: What Operations Need Auditing?**

### **Operation 1: WorkflowExecution Block Clearance** ⭐ **CRITICAL P0**

**What It Is**:
- When a workflow execution fails, BR-WE-012 blocks future executions to prevent cascading failures
- Operator manually clears the block after investigating and fixing the root cause
- User guide: BR-WE-013 (P0 CRITICAL for SOC2 v1.0)
- Command: `kubectl patch workflowexecution <wfe-name> --subresource=status -p '{"status":{"blockClearanceRequest":{...}}}'`

**Current State**:
- ✅ **REQUIRED** for SOC2 v1.0 (BR-WE-013 P0 CRITICAL)
- ✅ Specified in DD-WEBHOOK-001 (WorkflowExecution webhook, line 29)
- ⚠️ Planned in WEBHOOK_IMPLEMENTATION_WORKLOG_NEXT_SPRINT.md but **NOT in our Week 2-3 plan**

**SOC2 Requirement**: CC8.1 Attribution (WHO cleared the block to allow retry?)

**Event Type Needed**: `workflowexecution.block.cleared`

**Event Data Fields**:
```json
{
  "workflow_execution_id": "wfe-restart-pod-123",
  "remediation_request_id": "rr-oomkill-abc123",
  "cleared_by": {
    "username": "operator@example.com",
    "uid": "k8s-user-uuid",
    "groups": ["platform-admins"]
  },
  "clear_reason": "Fixed RBAC permissions in target namespace",
  "previous_failure_reason": "insufficient permissions",
  "block_duration": "3h15m"
}
```

**Why Critical**:
- Prevents cascading failures from non-idempotent operations
- SOC2 CC8.1: Must record WHO made operational decision to unblock
- SOC2 CC7.3: Preserves failed execution history (no deletion required)
- SOC2 CC7.4: Complete audit trail of failure → fix → clearance → retry

---

### **Operation 2: Workflow Cancellation (NotificationRequest Deletion)**

**What It Is**:
- Operator deletes a `NotificationRequest` CRD to cancel notification delivery
- User guide: [USER-GUIDE-NOTIFICATION-CANCELLATION.md](../../services/crd-controllers/05-remediationorchestrator/USER-GUIDE-NOTIFICATION-CANCELLATION.md)
- Command: `kubectl delete notificationrequest <nr-name> -n <namespace>`

**Current State**:
- ❌ **NOT** captured in audit trail
- ❓ Kubernetes API Server audit logs capture DELETE operation (if enabled)
- ❓ Requires webhook to capture operator identity

**SOC2 Requirement**: CC8.1 Attribution (WHO cancelled the notification?)

**Event Type Needed**: `notification.request.cancelled`

**Event Data Fields**:
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

---

### **Operation 3: RAR Approval (RemediationApprovalRequest Status Update)**

**What It Is**:
- Operator approves/rejects a `RemediationApprovalRequest` CRD via webhook
- Requires authenticated user identity (DD-WEBHOOK-001)
- Implementation: Next sprint (WEBHOOK_IMPLEMENTATION_WORKLOG_NEXT_SPRINT.md)

**Current State**:
- ⚠️ **PLANNED** but NOT IMPLEMENTED (webhook in next sprint)
- DD-WEBHOOK-001 specifies RAR requires webhook for SOC2 CC8.1
- RO Team owns implementation (P0 priority)

**SOC2 Requirement**: CC8.1 Attribution (WHO approved the remediation?)

**Event Type Needed** (from DD-AUDIT-003 v1.2):
- ✅ `orchestrator.approval.approved` (ALREADY EXISTS in DD-AUDIT-003 v1.2, line 347)
- ✅ `orchestrator.approval.rejected` (ALREADY EXISTS in DD-AUDIT-003 v1.2, line 348)

**Event Data Fields** (from DD-AUDIT-003 v1.2):
```json
{
  "remediation_request_id": "rr-oomkill-abc123",
  "approved_by": {
    "username": "operator@example.com",
    "uid": "k8s-user-uuid",
    "groups": ["platform-admins"]
  },
  "approval_reason": "Manually reviewed - safe to proceed",
  "risk_level": "high",
  "approval_timestamp": "2026-01-04T10:30:00Z"
}
```

**Status**:
- ✅ Event types **ALREADY DEFINED** in DD-AUDIT-003 v1.2
- ⏳ **IMPLEMENTATION DEFERRED** to next sprint (webhook prerequisite)

---

### **Operation 4: Workflow Catalog CRUD (Create/Disable)**

**What It Is**:
- Operator creates workflow: `kubectl apply -f restart-pod-workflow.yaml` (RemediationWorkflow CRD)
- Operator disables workflow: Data Storage API `POST /api/v1/workflows/{id}/disable`
- Storage: PostgreSQL via Data Storage Service (DD-WORKFLOW-009)

**Current State**:
- ✅ **PARTIAL** - Workflow creation/update audit events **ALREADY DEFINED** in DD-AUDIT-003 v1.2:
  - `datastorage.workflow.created` (line 252)
  - `datastorage.workflow.updated` (line 253)
- ❌ **MISSING** - Operator identity NOT captured (no webhook on RemediationWorkflow CRD)

**SOC2 Requirement**: CC8.1 Attribution (WHO created/disabled the workflow?)

**Event Type Status**:
- ✅ `datastorage.workflow.created` - **ALREADY EXISTS** in DD-AUDIT-003 v1.2
- ✅ `datastorage.workflow.updated` - **ALREADY EXISTS** in DD-AUDIT-003 v1.2 (covers disable operation)

**Event Data Fields** (from DD-AUDIT-003 v1.2):
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

**Status**:
- ✅ Event types **ALREADY DEFINED** in DD-AUDIT-003 v1.2
- ❌ **OPERATOR IDENTITY MISSING** - requires webhook on RemediationWorkflow CRD (not in DD-WEBHOOK-001)

---

## 🔍 **Gap Analysis**

### **Gaps Summary**

| Operation | Event Type | DD-AUDIT-003 Status | Implementation Status | Operator Identity | Gap |
|---|---|---|---|---|---|
| **Block Clearance** ⭐ | `workflowexecution.block.cleared` | ❌ **MISSING** | ❌ Not implemented | ❌ No webhook | **NEW EVENT + WEBHOOK** |
| **Workflow Cancellation** | `notification.request.cancelled` | ❌ **MISSING** | ❌ Not implemented | ❌ No webhook | **NEW EVENT + WEBHOOK** |
| **RAR Approval** | `orchestrator.approval.*` | ✅ **EXISTS** (v1.2) | ⏳ Next sprint (webhook) | ⏳ Webhook planned | **WEBHOOK ONLY** |
| **Workflow Create** | `datastorage.workflow.created` | ✅ **EXISTS** (v1.2) | ✅ Implemented | ❌ No webhook | **WEBHOOK ONLY** |
| **Workflow Disable** | `datastorage.workflow.updated` | ✅ **EXISTS** (v1.2) | ✅ Implemented | ❌ No webhook | **WEBHOOK ONLY** |

---

## 📊 **Effort Analysis**

### **Option A: Add to Current SOC2 Sprint (Week 1)** ❌

**NOT RECOMMENDED** - Requires webhook implementation which is deferred to next sprint.

**Rationale**:
- ❌ Webhooks are **P0 next sprint** work (8-10 days effort)
- ❌ Current SOC2 sprint focuses on **RR reconstruction** (no webhook dependency)
- ❌ Adding webhook work would **BLOCK current SOC2 completion**
- ❌ Webhooks require shared library development (WE Team, Days 1-2)

**Impact**:
- Would increase current sprint from **48-49 hours → 128-139 hours** (+80 hours webhook work)
- Would delay RR reconstruction completion by 2+ weeks

---

### **Option B: Create SOC2 Week 2-3 Extension** ⭐

**RECOMMENDED** - Separate sprint for operator action auditing after RR reconstruction is complete.

**Rationale**:
- ✅ **Dependencies managed**: RR reconstruction completes first (Week 1)
- ✅ **Aligns with existing plans**: Webhook work already scheduled for next sprint
- ✅ **Clear scope separation**: RR reconstruction vs. Operator attribution
- ✅ **Incremental compliance**: 100% RR reconstruction → Operator audit trail

**Effort Breakdown**:

#### **Week 2-3: Operator Action Auditing** (8-10 days)

**Prerequisites**:
- ✅ Week 1 SOC2 complete (RR reconstruction)
- ✅ DD-WEBHOOK-001 merged to main (already done)

**Phase 1: Webhook Infrastructure** (4 days, WE Team):
- Shared library development (`pkg/authwebhook`)
- WorkflowExecution webhook (block clearance)
- RemediationApprovalRequest webhook (approval)
- RemediationWorkflow webhook (catalog CRUD)

**Phase 2: Audit Event Integration** (2-3 days, Cross-team):
- Implement `notification.request.cancelled` event (Notification Team)
- Wire RAR approval events to webhook (RO Team)
- Wire workflow CRUD events to webhook (Data Storage Team)

**Phase 3: Testing & Validation** (2-3 days):
- Integration tests for all 3 operator actions
- E2E tests with authenticated users
- SOC2 CC8.1 compliance validation

**Total Effort**: 8-10 days (64-80 hours)

---

### **Option C: Defer to V1.1** ❌

**NOT RECOMMENDED** - Operator attribution is SOC2 CC8.1 requirement, not optional.

**Rationale**:
- ❌ SOC2 Type II requires operator attribution (CC8.1)
- ❌ "100% RR reconstruction" is incomplete without operator actions
- ❌ Auditors will flag missing operator identity in compliance review

---

## 💡 **Recommendation**

### **APPROVE: Option B (SOC2 Week 2-3 Extension)** ⭐

**Implementation Plan**:

#### **Week 1 (Current Sprint - Days 1-6)**: RR Reconstruction
- Focus: 100% RR CRD reconstruction from audit traces
- Deliverable: All 8 critical fields captured
- Timeline: January 6-13, 2026 (48-49 hours)

#### **Week 2-3 (Next Sprint - Days 7-16)**: Operator Action Auditing
- Focus: Webhook implementation + operator attribution
- Deliverable: 4 operator actions with authenticated user identity
- Timeline: January 14-27, 2026 (68-84 hours)

**Total SOC2 Implementation**: **16 days (116-133 hours)** across 2 sprints

---

## 📋 **Detailed Implementation Plan (Week 2-3)**

**Updated Effort Breakdown**:

| Days | Focus | Owner | Hours |
|------|-------|-------|-------|
| Days 7-8 | Shared library + WE block clearance | WE Team | 20h (+4h) |
| Days 9-10 | RAR approval webhook | RO Team | 16h |
| Days 11-12 | Workflow catalog webhook | DS Team | 16h |
| Days 13-14 | Notification cancellation webhook | NT Team | 16h |
| Days 15-16 | E2E testing + SOC2 compliance | All Teams | 16h |

**Total Week 2-3**: 84 hours (was 80 hours, +4 hours for WE block clearance)

---

### **Day 7-8: Shared Webhook Library + WE Block Clearance** (WE Team)

**Tasks**:

**Part 1: Shared Library** (12 hours):
1. Create `pkg/authwebhook/` shared library
2. Implement `ExtractUser(ctx, req)` for user identity extraction
3. Implement `AuditClient` wrapper for authenticated events
4. Write 18 unit tests for shared library

**Part 2: WorkflowExecution Block Clearance** (8 hours):
5. Update WorkflowExecution CRD schema (add `blockClearanceRequest`, `blockClearance` fields)
6. Scaffold WorkflowExecution webhook using operator-sdk
7. Implement `ValidateUpdate()` for block clearance requests
8. Wire to NEW event: `workflowexecution.block.cleared`
9. Write integration tests with authenticated operators
10. Update WorkflowExecution controller to detect `blockClearance` and unblock

**Deliverable**:
- ✅ Reusable authentication library
- ✅ WE webhook with operator attribution for block clearance

**Reference**:
- ADR-051-operator-sdk-webhook-scaffolding.md
- BR-WE-013-audit-tracked-block-clearing.md (P0 CRITICAL)

---

### **Day 9-10: RemediationApprovalRequest Webhook** (RO Team)

**Tasks**:
1. Scaffold RAR webhook using operator-sdk
2. Implement `ValidateUpdate()` for approval/rejection
3. Extract operator identity using `pkg/authwebhook`
4. Wire to existing `orchestrator.approval.*` events
5. Write integration tests with authenticated users

**Deliverable**: ✅ RAR webhook with operator attribution

**Reference**: WEBHOOK_IMPLEMENTATION_WORKLOG_NEXT_SPRINT.md, Days 4-5

---

### **Day 11-12: RemediationWorkflow Webhook** (Data Storage Team)

**Tasks**:
1. Add RemediationWorkflow to DD-WEBHOOK-001 (currently missing)
2. Scaffold RemediationWorkflow webhook using operator-sdk
3. Implement `ValidateCreate()` for workflow creation
4. Wire to existing `datastorage.workflow.*` events
5. Write integration tests

**Deliverable**: ✅ Workflow catalog operations with operator attribution

**New Event**: Update DD-AUDIT-003 v1.4 to clarify operator identity capture

---

### **Day 13-14: NotificationRequest Cancellation Webhook** (Notification Team)

**Tasks**:
1. Add NotificationRequest to DD-WEBHOOK-001 (currently missing)
2. Scaffold NotificationRequest webhook using operator-sdk
3. Implement `ValidateDelete()` for cancellation
4. Create NEW event: `notification.request.cancelled`
5. Wire to audit system via `pkg/authwebhook`
6. Write integration tests

**Deliverable**: ✅ Workflow cancellation with operator attribution

**New Event**: Add to DD-AUDIT-003 v1.4

---

### **Day 15-16: E2E Testing & Compliance Validation** (All Teams)

**Tasks**:
1. E2E test: Operator clears WorkflowExecution block → audit event captured
2. E2E test: Operator approves RAR → audit event captured
3. E2E test: Operator cancels NotificationRequest → audit event captured
4. E2E test: Operator creates workflow → audit event captured
5. E2E test: Operator disables workflow → audit event captured
6. SOC2 CC8.1 compliance documentation

**Deliverable**: ✅ Full operator action audit trail

**Acceptance**: All 5 operator actions have authenticated user identity in audit events

---

## 🎯 **Updated SOC2 Roadmap**

### **Week 1: RR Reconstruction (Current Sprint)**
- **Goal**: 100% RR CRD reconstruction from audit traces
- **Events**: 4 critical event types (gateway, aianalysis, workflow selection, execution)
- **Outcome**: RR can be reconstructed after TTL expiration

### **Week 2-3: Operator Attribution (Next Sprint)**
- **Goal**: SOC2 CC8.1 operator attribution
- **Events**: 3 operator action types (approval, cancellation, workflow CRUD)
- **Outcome**: All operator actions captured with authenticated identity

### **Week 3: Full SOC2 Compliance Validation**
- **Goal**: 100% SOC2 Type II readiness
- **Coverage**: RR reconstruction + operator attribution
- **Compliance**: CC8.1 (attribution), CC7.3 (audit integrity)

---

## 📊 **DD-AUDIT-003 Extension Requirements**

### **Version 1.4 Changes Needed**

#### **New Event Type**:
```markdown
| `notification.request.cancelled` | Operator cancelled notification delivery | **P0** |
```

**Event Data**:
- `notification_request_id`
- `remediation_request_id`
- `cancelled_by.username`
- `cancelled_by.uid`
- `cancellation_reason`

#### **Clarifications for Existing Events**:

**`datastorage.workflow.created`**:
- Add: "Captures authenticated operator identity via RemediationWorkflow webhook"

**`datastorage.workflow.updated`**:
- Add: "Includes disable operation, captures authenticated operator identity"

**`orchestrator.approval.approved/rejected`**:
- Add: "Captures authenticated operator identity via RemediationApprovalRequest webhook"

---

## 🔗 **Authority Document Updates Required**

### **1. DD-WEBHOOK-001 v1.1**
**Add 2 new CRDs requiring webhooks**:

| CRD | Use Case | Status Fields Requiring Auth | Priority |
|-----|----------|------------------------------|----------|
| **RemediationWorkflow** | Catalog CRUD Attribution | `metadata.creationTimestamp` | P0 |
| **NotificationRequest** | Cancellation Attribution | `metadata.deletionTimestamp` | P0 |

### **2. DD-AUDIT-003 v1.4**
**Add 1 new event type**:
- `notification.request.cancelled`

**Clarify 3 existing event types**:
- `datastorage.workflow.created` (operator identity)
- `datastorage.workflow.updated` (operator identity)
- `orchestrator.approval.*` (operator identity)

### **3. SOC2_AUDIT_IMPLEMENTATION_PLAN.md**
**Extend timeline**:
- Add Week 2-3 work breakdown (64-80 hours)
- Add webhook implementation tasks
- Update total effort: 48-49h → 112-129h

---

## ✅ **Recommendation Summary**

**APPROVE**: **Option B - SOC2 Week 2-3 Extension**

**Justification**:
1. ✅ **Separation of Concerns**: RR reconstruction (Week 1) vs. Operator attribution (Week 2-3)
2. ✅ **Dependency Management**: Webhook work already scheduled for next sprint
3. ✅ **Incremental Compliance**: Deliver RR reconstruction first, then operator audit
4. ✅ **Lower Risk**: No scope creep in current sprint, clear milestone boundaries
5. ✅ **SOC2 Complete**: Achieves 100% CC8.1 compliance (attribution + reconstruction)

**Effort**:
- Week 1: 48-49 hours (RR reconstruction)
- Week 2-3: 64-80 hours (Operator attribution)
- **Total**: 112-129 hours (16 days)

**Timeline**:
- Week 1: January 6-13, 2026
- Week 2-3: January 14-27, 2026
- **SOC2 Complete**: January 27, 2026

---

## 🚨 **Critical Dependencies**

### **Week 1 → Week 2 Dependencies**:
1. ✅ DD-WEBHOOK-001 merged to main (already done)
2. ⏳ RR reconstruction complete (Week 1 deliverable)
3. ⏳ `pkg/authwebhook` shared library created (Week 2, Days 7-8)

### **Week 2 → Week 3 Dependencies**:
1. ⏳ All 3 webhooks implemented (Week 2, Days 9-14)
2. ⏳ Integration tests passing (Week 2, Days 13-14)
3. ⏳ DD-AUDIT-003 v1.4 approved (Week 2, Day 11)

---

## 📋 **Confidence Assessment**

**Confidence**: 85%

**Justification**:
- ✅ Clear requirements from DD-WEBHOOK-001 and DD-AUDIT-003
- ✅ Webhook implementation plan already documented (WEBHOOK_IMPLEMENTATION_WORKLOG_NEXT_SPRINT.md)
- ✅ Effort estimates based on existing webhook work (WE Team)
- ⚠️ Minor uncertainty on exact webhook validation logic (10-15% variance)
- ⚠️ Integration test complexity across 3 teams (RO, Notification, Data Storage)

**Risk Assessment**:
- **Risk**: Webhook implementation delays Week 2-3 work
- **Mitigation**: WE Team has detailed worklog, reusable shared library
- **Probability**: LOW (webhook design already approved)

---

**Document Status**: ✅ **APPROVED - OPTION B (WEEK 2-3 EXTENSION)**
**User Decision**: Option B approved - time is not a constraint
**Priority**: Quality and completeness over speed
**Timeline**:
- Week 1 (Jan 6-13): RR Reconstruction (48-49 hours)
- Week 2-3 (Jan 14-27): Operator Attribution (64-80 hours)
- **Total**: 112-129 hours (16 days)
**Next Action**: Proceed with Week 1 implementation per SOC2_AUDIT_IMPLEMENTATION_PLAN.md

