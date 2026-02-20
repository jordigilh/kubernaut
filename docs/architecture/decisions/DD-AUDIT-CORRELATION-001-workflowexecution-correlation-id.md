# DD-AUDIT-CORRELATION-001: WorkflowExecution Correlation ID Source

**Status**: ✅ APPROVED
**Date**: 2026-01-05
**Priority**: P0 - Foundational (Audit Trail Integrity)
**Scope**: WorkflowExecution audit trail correlation
**Related**: BR-AUDIT-005 (Gap 5-6), DD-AUDIT-002 (Audit Shared Library), ADR-032 (Data Access Layer)

---

## Context & Problem

During Day 3 SOC2 audit trail implementation (BR-AUDIT-005 Gap 5-6), a critical issue was discovered in how WorkflowExecution audit events retrieve correlation IDs:

### **Issue**:
The audit manager was using `wfe.Labels["kubernaut.ai/correlation-id"]` to retrieve correlation IDs, but:
1. **RemediationOrchestrator does NOT set this label** on WorkflowExecution CRDs
2. The label-based approach was based on an **unimplemented pattern** from `LOG_CORRELATION_ID_STANDARD.md`
3. This resulted in using `wfe.Name` as correlation ID, **breaking audit trail continuity**

### **Impact**:
- ❌ **Broken Request-Response Reconstruction**: Audit events tied to WFE name instead of parent RemediationRequest
- ❌ **Lost Cross-Service Tracing**: Cannot trace flow from Gateway → RR → AIAnalysis → WFE
- ❌ **SOC2 Compliance Risk**: Incomplete audit trail violates non-repudiation requirements

### **Root Cause**:
Documentation (`LOG_CORRELATION_ID_STANDARD.md`) suggested a label-based pattern that was **never implemented** in RemediationOrchestrator's CRD creator.

---

## Decision

**WorkflowExecution audit events MUST use `wfe.Spec.RemediationRequestRef.Name` as the correlation ID.**

### **Rationale**:
1. **Spec Field is Authoritative**:
   - `RemediationRequestRef` is a **REQUIRED** field in WFE spec (per ADR-044)
   - Set by RemediationOrchestrator during CRD creation (verified in `pkg/remediationorchestrator/creator/workflowexecution.go:109-115`)
   - Cannot be empty (CRD validation enforces this)

2. **Parent RR Name is Root Correlation ID**:
   - Gateway generates RemediationRequest with unique name
   - RR name is the **root correlation ID** for entire remediation flow
   - All child CRDs (AIAnalysis, WorkflowExecution) reference parent RR

3. **No Label Dependency**:
   - Labels are optional metadata (can be stripped/modified)
   - Spec fields are part of the API contract (immutable)
   - Avoids implementation drift between docs and code

4. **Consistent with Existing Pattern**:
   - AIAnalysis controller uses same pattern (parent RR reference)
   - Notification service uses RR name for correlation
   - Gateway service uses RR name for status updates

---

## Alternatives Considered

### **Alternative 1: Label-Based Correlation ID** (REJECTED)
**Pattern**: Use `wfe.Labels["kubernaut.ai/correlation-id"]` with fallback to `wfe.Name`

❌ **Rejected**:
- RemediationOrchestrator does NOT set this label (code inspection confirms)
- Would require RO changes + migration for existing CRDs
- Labels are not part of API contract (can be stripped)
- Adds unnecessary coupling to label keys

### **Alternative 2: Generate New Correlation ID** (REJECTED)
**Pattern**: Generate new UUID for WFE audit events

❌ **Rejected**:
- Breaks audit trail continuity (cannot link WFE events to parent RR)
- Violates Request-Response Reconstruction (BR-AUDIT-005)
- Makes cross-service debugging impossible

### **Alternative 3: Use `wfe.Spec.RemediationRequestRef.Name`** (APPROVED)
**Pattern**: Use parent RemediationRequest name as correlation ID

✅ **APPROVED**:
- ✅ Authoritative source (required spec field)
- ✅ Guaranteed to exist (CRD validation)
- ✅ Maintains audit trail continuity
- ✅ No code changes in RemediationOrchestrator needed
- ✅ Consistent with existing patterns

---

## Implementation

### **Code Pattern (MANDATORY)**

```go
// ✅ CORRECT: Use parent RemediationRequest name
correlationID := wfe.Spec.RemediationRequestRef.Name
audit.SetCorrelationID(event, correlationID)
```

```go
// ❌ INCORRECT: Label-based with fallback (DO NOT USE)
correlationID := wfe.Name
if wfe.Labels != nil {
    if corrID, ok := wfe.Labels["kubernaut.ai/correlation-id"]; ok {
        correlationID = corrID
    }
}
```

### **Applied in Files**:
- ✅ `pkg/workflowexecution/audit/manager.go` (3 methods):
  - `RecordWorkflowSelectionCompleted()` (Gap #5)
  - `RecordExecutionWorkflowStarted()` (Gap #6)
  - `recordAuditEvent()` (shared helper)

### **Code Comment Pattern**:
All correlation ID retrieval sites MUST include this comment:
```go
// Correlation ID: Use parent RemediationRequest name (BR-AUDIT-005)
// Per DD-AUDIT-CORRELATION-001: WFE.Spec.RemediationRequestRef.Name is the authoritative source
// Labels are NOT set by RemediationOrchestrator (verified in creator implementation)
correlationID := wfe.Spec.RemediationRequestRef.Name
```

---

## Verification

### **How to Verify**:

1. **Check RemediationOrchestrator Creator**:
```bash
# Confirm RO does NOT set correlation-id label
grep -A 10 "Labels: map\[string\]string{" \
  pkg/remediationorchestrator/creator/workflowexecution.go
```
**Expected**: No `kubernaut.ai/*` labels on CRDs (Issue #91: migrated to spec fields; `spec.remediationRequestRef` used instead)

2. **Check WFE Spec Field**:
```bash
# Confirm RemediationRequestRef is set
grep -A 5 "RemediationRequestRef:" \
  pkg/remediationorchestrator/creator/workflowexecution.go
```
**Expected**: `Name: rr.Name` (parent RR name)

3. **Query Audit Events**:
```sql
-- Verify correlation ID matches parent RR
SELECT
  event_type,
  correlation_id,
  event_data->>'execution_name' as wfe_name
FROM audit_events
WHERE event_category = 'workflow'
  AND event_type IN ('workflow.selection.completed', 'execution.workflow.started')
ORDER BY event_timestamp DESC
LIMIT 10;
```
**Expected**: `correlation_id` should match RemediationRequest name, NOT WorkflowExecution name

---

## Audit Trail Flow

### **Correct Flow (DD-AUDIT-CORRELATION-001)**:

```
Gateway → RemediationRequest (Name: "rr-abc123")
  ↓
  correlation_id = "rr-abc123"
  ↓
RO → WorkflowExecution (Name: "we-rr-abc123", Spec.RemediationRequestRef.Name: "rr-abc123")
  ↓
  correlation_id = "rr-abc123" (from Spec field!)
  ↓
WFE Audit Events:
  - workflow.selection.completed (correlation_id: "rr-abc123")
  - execution.workflow.started (correlation_id: "rr-abc123")
  - workflow.started (correlation_id: "rr-abc123")
```

### **Incorrect Flow (Pre-DD-AUDIT-CORRELATION-001)**:

```
Gateway → RemediationRequest (Name: "rr-abc123")
  ↓
  correlation_id = "rr-abc123"
  ↓
RO → WorkflowExecution (Name: "we-rr-abc123")
  ↓
  correlation_id = "we-rr-abc123" ❌ (WRONG - uses WFE name!)
  ↓
WFE Audit Events (BROKEN):
  - workflow.selection.completed (correlation_id: "we-rr-abc123") ❌
  - execution.workflow.started (correlation_id: "we-rr-abc123") ❌
  - workflow.started (correlation_id: "we-rr-abc123") ❌
```

**Result**: Cannot reconstruct Request-Response flow!

---

## Related Documentation

### **Update LOG_CORRELATION_ID_STANDARD.md**:
The `LOG_CORRELATION_ID_STANDARD.md` document suggests using labels but does not reflect actual implementation. **Action Required**:
- Add note: "WorkflowExecution uses `Spec.RemediationRequestRef.Name` instead of labels (per DD-AUDIT-CORRELATION-001)"
- Document that label-based pattern is NOT implemented in RO

### **Update DD-AUDIT-002**:
Add WorkflowExecution-specific guidance referencing this decision.

---

## Testing Strategy

### **Unit Tests**:
```go
// Verify correlation ID uses RemediationRequestRef.Name
wfe := &workflowexecutionv1alpha1.WorkflowExecution{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "we-test-123",
        Namespace: "default",
    },
    Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
        RemediationRequestRef: corev1.ObjectReference{
            Name: "rr-parent-456", // This should be correlation ID
        },
    },
}

auditSpy := &AuditStoreSpy{}
manager := audit.NewManager(auditSpy, logger)
err := manager.RecordWorkflowSelectionCompleted(ctx, wfe)

events := auditSpy.GetStoredEvents()
Expect(events[0].CorrelationId).To(Equal("rr-parent-456")) // NOT "we-test-123"!
```

### **Integration Tests**:
Verify existing Gap 5-6 tests query events by RR name:
```go
correlationID := wfe.Spec.RemediationRequestRef.Name // Use parent RR
events, err := queryAuditEvents(dsClient, correlationID, nil)
Expect(events).To(HaveLen(2)) // Both Gap 5-6 events found
```

---

## Migration

### **No Migration Required**:
- This is a **Day 3 implementation** (Gap 5-6 is new functionality)
- No existing WFE audit events to migrate
- `workflow.started`, `workflow.completed`, `workflow.failed` already use this pattern (verified in existing code)

### **Future-Proof**:
If RemediationOrchestrator ever implements label-based correlation ID:
- Spec field remains authoritative source
- Labels become optional metadata for debugging
- No code changes needed in audit manager

---

## Success Criteria

✅ **Compliance**:
- All WFE audit events use `RemediationRequestRef.Name` as correlation ID
- No fallback to `wfe.Name` or label-based retrieval
- Audit trail continuity maintained across Gateway → RR → WFE

✅ **Code Quality**:
- All correlation ID retrieval sites include DD-AUDIT-CORRELATION-001 comment
- No label-based patterns in audit manager code
- Linter passes with no unused label checks

✅ **Testing**:
- Unit tests verify correlation ID source
- Integration tests query by parent RR name
- Gap 5-6 tests pass with proper correlation

---

## Lessons Learned

### **Documentation vs. Implementation Drift**:
- **Problem**: `LOG_CORRELATION_ID_STANDARD.md` documented a label pattern that was never implemented
- **Solution**: Always verify docs against actual code during triage
- **Prevention**: Link design decisions to implementation PRs

### **Defensive Programming**:
- **Problem**: Assumed labels would exist without checking RO creator
- **Solution**: Always inspect CRD creator code to verify metadata
- **Prevention**: Mandate code inspection during audit implementation

### **Spec vs. Labels**:
- **Problem**: Used labels (optional metadata) instead of spec (API contract)
- **Solution**: Prefer spec fields for required data
- **Prevention**: Document "spec field is authoritative" pattern

---

## Approval

**Approved By**: Auto-approved (P0 Foundational - Audit Trail Integrity)
**Date**: 2026-01-05
**Implementation**: Day 3 SOC2 Gap 5-6 (BR-AUDIT-005)
**Compliance**: SOC2 CC6.8 (Non-Repudiation), CC7.2 (Monitoring Activities)

---

## References

- **BR-AUDIT-005**: Hybrid Provider Data Capture (Gap 5-6: Workflow References)
- **DD-AUDIT-002**: Audit Shared Library Design
- **ADR-032**: Data Access Layer Isolation
- **ADR-044**: Workflow Execution Engine Delegation
- **LOG_CORRELATION_ID_STANDARD.md**: Original (incorrect) label-based pattern
- **`pkg/remediationorchestrator/creator/workflowexecution.go`**: RO CRD creator (authoritative implementation)

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-01-05 | Initial decision (Day 3 SOC2 implementation) |



