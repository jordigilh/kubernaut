# DD-WEBHOOK-003: Webhook-Complete Audit Pattern for Operator Actions

**Status**: ✅ **Approved** (2026-01-05)
**Confidence**: 95% (based on SOC2 requirements and operational simplicity)
**Related**: DD-WEBHOOK-001 (Consolidated Webhook), BR-AUTH-001 (SOC2 CC8.1)

---

## Context & Problem

**Requirement**: Capture WHO performed WHAT action for SOC2 CC8.1 compliance.

**Key Insight**: Webhooks have complete context for operator-initiated actions:
- **WHO**: `req.UserInfo.Username` (authenticated operator)
- **WHAT**: `req.Operation` (CREATE, UPDATE, DELETE)
- **DETAILS**: `req.Object` (new state) and `req.OldObject` (previous state)

**Previous Anti-Pattern**: Dual audit pattern (webhook + controller) was over-engineered.

---

## Decision

**Webhooks write ONE complete audit event** capturing WHO, WHAT, and ACTION details.

### Pattern: Webhook-Complete Audit

```go
func (h *WebhookHandler) Handle(ctx context.Context, req admission.Request) {
    // 1. Extract WHO
    authCtx := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)

    // 2. Decode WHAT (object being modified)
    obj := decode(req.Object)
    oldObj := decode(req.OldObject)

    // 3. Write COMPLETE audit event (WHO + WHAT + ACTION)
    eventData := map[string]interface{}{
        "operation":     req.Operation,        // CREATE/UPDATE/DELETE
        "resource_name": obj.Name,
        "namespace":     obj.Namespace,
        "action_details": extractActionDetails(obj, oldObj), // What changed
    }

    h.auditManager.RecordEvent(ctx, audit.Event{
        EventType:     formatEventType(req.Operation, obj),
        ActorID:       authCtx.Username,
        EventCategory: audit.EventCategoryWorkflow,
        EventAction:   string(req.Operation),
        EventOutcome:  audit.OutcomeSuccess,
        EventData:     marshalJSON(eventData),
    })

    // 4. Optionally populate CRD status fields (for UI/API queries)
    if shouldPopulateStatus(req.Operation) {
        obj.Status.PerformedBy = authCtx.Username
        obj.Status.PerformedAt = metav1.Now()
        return admission.PatchResponseFromRaw(req.Object.Raw, marshal(obj))
    }

    return admission.Allowed("audit recorded")
}
```

---

## Alternatives Considered

### Alternative 1: Dual Audit Pattern (Rejected)

**Approach**: Webhook writes attribution, controller writes business context.

**Why Rejected**:
- ❌ Over-engineered: Webhook already has all context
- ❌ Requires coordination between webhook and controller
- ❌ Two audit events where one suffices
- ❌ Controller must read webhook-populated fields

**Confidence**: 40% (rejected - unnecessary complexity)

---

### Alternative 2: Annotation-Based Attribution (Rejected)

**Approach**: Webhook adds `annotations["kubernaut.ai/created-by"]`, controller reads it.

**Why Rejected**:
- ❌ **Annotations are not traceable in Kubernaut** (per project standards)
- ❌ Annotations can be modified by users
- ❌ No immutability guarantees
- ❌ Requires controller coordination

**Confidence**: 20% (rejected - violates project standards)

---

### Alternative 3: Webhook-Complete Audit (APPROVED)

**Approach**: Webhook writes one complete audit event with all context.

**Pros**:
- ✅ Simple: One audit event per operator action
- ✅ Complete: WHO + WHAT + ACTION in single event
- ✅ No coordination: Webhook is self-contained
- ✅ Traceable: Audit table is source of truth
- ✅ Status fields optional: Only for UI/API convenience

**Cons**:
- ⚠️ Webhook has some business logic (extracting action details)

**Confidence**: 95% (approved)

---

## Implementation by Operation Type

### CREATE Operations

**Webhook Audit Event**:
```go
func (h *CreateHandler) Handle(ctx context.Context, req admission.Request) {
    authCtx := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
    obj := decode(req.Object)

    h.auditManager.RecordEvent(ctx, audit.Event{
        EventType:     "resource.created",
        ActorID:       authCtx.Username,
        EventAction:   "create",
        EventData:     marshalJSON({
            "resource_name": obj.Name,
            "namespace":     obj.Namespace,
            "spec":          obj.Spec,  // What was created
        }),
    })

    // No CRD mutation needed (annotations not used)
    return admission.Allowed("audit recorded")
}
```

**Status Fields**: Not needed for CREATE (controller manages status).

---

### UPDATE Operations

**Webhook Audit Event**:
```go
func (h *UpdateHandler) Handle(ctx context.Context, req admission.Request) {
    authCtx := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
    obj := decode(req.Object)
    oldObj := decode(req.OldObject)

    // Extract WHAT changed
    changes := detectChanges(obj, oldObj)

    h.auditManager.RecordEvent(ctx, audit.Event{
        EventType:     "resource.updated",
        ActorID:       authCtx.Username,
        EventAction:   "update",
        EventData:     marshalJSON({
            "resource_name": obj.Name,
            "namespace":     obj.Namespace,
            "changes":       changes,  // What changed
            "new_state":     obj.Status,
            "old_state":     oldObj.Status,
        }),
    })

    // ALWAYS populate status fields for UPDATE operations (MANDATORY)
    obj.Status.LastModifiedBy = authCtx.Username
    obj.Status.LastModifiedAt = metav1.Now()

    return admission.PatchResponseFromRaw(req.Object.Raw, marshal(obj))
}
```

**Status Fields**: Optional, for UI/API queries (audit table is source of truth).

---

### DELETE Operations

**Webhook Audit Event**:
```go
func (h *DeleteHandler) Handle(ctx context.Context, req admission.Request) {
    authCtx := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
    oldObj := decode(req.OldObject)  // Object being deleted

    h.auditManager.RecordEvent(ctx, audit.Event{
        EventType:     "resource.deleted",
        ActorID:       authCtx.Username,
        EventAction:   "delete",
        EventData:     marshalJSON({
            "resource_name": oldObj.Name,
            "namespace":     oldObj.Namespace,
            "final_state":   oldObj.Status,  // State at deletion
            "spec":          oldObj.Spec,     // What was deleted
        }),
    })

    // Cannot mutate during DELETE (K8s limitation)
    // No annotations used (not traceable)
    return admission.Allowed("audit recorded")
}
```

**Status Fields**: Cannot be populated (K8s prevents mutation during DELETE).

---

## Controller Responsibilities

**Controllers do NOT write attribution audits**. They focus on business logic:

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) {
    obj := fetch(req)

    // Business logic only (no attribution audit)
    if r.shouldProcessWorkflow(obj) {
        r.processWorkflow(ctx, obj)

        // Controller writes business events (optional)
        r.auditManager.RecordEvent(ctx, audit.Event{
            EventType: "workflow.executed",
            ActorID:   "workflowexecution-controller",  // System actor
            EventData: marshalJSON({
                "workflow_id": obj.Spec.WorkflowID,
                "result":      obj.Status.Result,
            }),
        })
    }
}
```

**Key Difference**:
- **Webhook audits**: Operator actions (WHO did WHAT)
- **Controller audits**: System events (WHAT happened in business logic)

---

## Status Field Usage

### Purpose
Status fields (e.g., `PerformedBy`, `PerformedAt`) are **MANDATORY when possible**:
- ✅ **Always populate status fields** when operation allows mutation
- ✅ Provide immediate access to "who did what" without audit queries
- ✅ Enable UI display, API filtering, and debugging
- ✅ Immutable once set (operators cannot modify status fields)

### Audit Table Relationship
- ✅ **Audit table is the compliance source of truth**
- ✅ **Status fields are the operational convenience layer**
- ⚠️ Status fields may not exist for DELETE operations (K8s limitation)

### When to Populate Status Fields

| Operation | Can Mutate? | Status Fields | Audit Event |
|-----------|-------------|---------------|-------------|
| CREATE | ✅ Yes | ❌ Not needed (status empty) | ✅ Write audit |
| UPDATE | ✅ Yes | ✅ **MANDATORY** | ✅ Write audit |
| DELETE | ❌ No | ❌ Cannot mutate | ✅ Write audit |

### Usage Pattern
```go
// Webhook ALWAYS populates status fields for UPDATE operations
obj.Status.LastModifiedBy = authCtx.Username
obj.Status.LastModifiedAt = metav1.Now()

// UI queries status field (fast, no audit table query needed)
GET /api/v1/resources?modifiedBy=admin@example.com

// Compliance queries audit table (source of truth for SOC2)
SELECT * FROM audit_events WHERE actor_id = 'admin@example.com';
```

---

## Examples by Resource Type

### WorkflowExecution Block Clearance (UPDATE)

**Webhook**:
```go
// 1. Write complete audit event (WHO + WHAT + ACTION)
h.auditManager.RecordEvent(ctx, audit.Event{
    EventType: "workflowexecution.block.cleared",
    ActorID:   authCtx.Username,
    EventData: marshalJSON({
        "workflow_name": wfe.Name,
        "clear_reason":  wfe.Status.BlockClearance.ClearReason,
        "previous_state": oldWFE.Status.Phase,
        "new_state":      wfe.Status.Phase,
    }),
})

// 2. ALWAYS populate status fields (MANDATORY for UPDATE)
wfe.Status.BlockClearance.ClearedBy = authCtx.Username
wfe.Status.BlockClearance.ClearedAt = metav1.Now()
```

---

### RemediationApprovalRequest Decision (UPDATE)

**Webhook**:
```go
// 1. Write complete audit event (WHO + WHAT + ACTION)
h.auditManager.RecordEvent(ctx, audit.Event{
    EventType: "remediationapproval.decision.made",
    ActorID:   authCtx.Username,
    EventData: marshalJSON({
        "request_name":     rar.Name,
        "decision":         rar.Status.Decision,
        "decision_message": rar.Status.DecisionMessage,
        "ai_analysis_ref":  rar.Spec.AIAnalysisRef.Name,
    }),
})

// 2. ALWAYS populate status fields (MANDATORY for UPDATE)
rar.Status.DecidedBy = authCtx.Username
rar.Status.DecidedAt = &metav1.Time{Time: time.Now()}
```

---

### NotificationRequest Cancellation (DELETE)

**Webhook**:
```go
h.auditManager.RecordEvent(ctx, audit.Event{
    EventType: "notification.request.deleted",
    ActorID:   authCtx.Username,
    EventData: marshalJSON({
        "notification_name": nr.Name,
        "notification_type": nr.Spec.Type,
        "final_status":      nr.Status.Phase,
        "recipients":        nr.Spec.Recipients,
    }),
})
// No CRD mutation (DELETE limitation + no annotations)
```

---

## Validation & Testing

### Integration Test Pattern

```go
It("should record complete audit event for operator action", func() {
    By("Operator performs action (business operation)")
    obj.Status.Decision = "Approved"
    Expect(k8sClient.Status().Update(ctx, obj)).To(Succeed())

    By("Verifying webhook recorded audit (side effect)")
    Eventually(func() bool {
        events := mockAuditManager.GetEvents()
        for _, event := range events {
            if event.EventType == "resource.updated" &&
               event.ActorID == "admin" &&
               event.EventData contains "Approved" {
                return true
            }
        }
        return false
    }).Should(BeTrue())
})
```

---

## Consequences

### Positive

- ✅ **Simple**: One audit event per operator action
- ✅ **Complete**: WHO + WHAT + ACTION in single event
- ✅ **Traceable**: Audit table is source of truth (no annotations)
- ✅ **Self-Contained**: No webhook-controller coordination needed
- ✅ **SOC2 Compliant**: All operator actions captured

### Negative

- ⚠️ **Business Logic in Webhook**: Webhook must extract action details
- ⚠️ **Duplicate Context**: EventData may duplicate CRD fields

### Neutral

- 🔄 **Status Fields Mandatory**: Always populate for UPDATE operations (UI + API convenience)
- 🔄 **Audit Table Source of Truth**: Status fields supplement, not replace audit events
- 🔄 **Controller Audits**: Still needed for system events (not operator attribution)

---

## Migration from Dual Audit Pattern

### Before (Dual Audit - DEPRECATED)
```go
// Webhook (incomplete)
wfe.Status.ClearedBy = authCtx.Username
h.auditManager.RecordEvent(ctx, audit.Event{
    EventType: "block.clearance.attributed",
    ActorID:   authCtx.Username,
})

// Controller (reads webhook field)
clearedBy := wfe.Status.ClearedBy
h.auditManager.RecordEvent(ctx, audit.Event{
    EventType: "block.clearance.processed",
    ActorID:   clearedBy,  // From webhook
    EventData: {...business context...},
})
```

### After (Webhook-Complete - APPROVED)
```go
// Webhook (complete)
h.auditManager.RecordEvent(ctx, audit.Event{
    EventType: "workflowexecution.block.cleared",
    ActorID:   authCtx.Username,
    EventData: marshalJSON({
        "clear_reason": wfe.Status.BlockClearance.ClearReason,
        "previous_phase": oldWFE.Status.Phase,
        "new_phase": wfe.Status.Phase,
        // All context in one event
    }),
})
wfe.Status.ClearedBy = authCtx.Username  // MANDATORY (always populate for UPDATE)
wfe.Status.ClearedAt = metav1.Now()

// Controller (no attribution audit)
// Only system events if needed
```

---

## Decision Matrix

| Operation | Webhook Audit | Status Fields | Controller Audit |
|-----------|---------------|---------------|------------------|
| CREATE | ✅ One complete event | ❌ Not applicable | ❌ No attribution |
| UPDATE | ✅ One complete event | ✅ **MANDATORY** | ❌ No attribution |
| DELETE | ✅ One complete event | ❌ Cannot mutate | ❌ No attribution |

---

## References

- **BR-AUTH-001**: SOC2 CC8.1 Operator Attribution
- **DD-WEBHOOK-001**: Consolidated Webhook Architecture
- **SOC2 CC8.1**: Change Management - Attribution Requirements
- **Project Standard**: Annotations not used (not traceable)

---

**Review Schedule**: Quarterly or when new operator actions are added
**Success Metrics**: 100% operator attribution coverage, <10ms audit latency

