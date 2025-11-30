# DD-NOT-005: NotificationRequest Spec Immutability

**Status**: 📋 **PROPOSAL** (2025-11-28)
**Last Reviewed**: 2025-11-28
**Confidence**: 95%
**Scope**: NotificationRequest CRD only

---

## Context & Problem

### **Problem Statement**

During DD-NOT-003 V2.0 integration test planning, we identified potential race conditions and complexity from CRD spec updates during notification delivery:

**User Feedback** (2025-11-28):
> "We need to avoid users overwriting the specs of our CRDs because that will cause havoc in the flow."

### **Specific NotificationRequest Risks**

If NotificationRequest spec is mutable:

1. **Status Corruption**: User updates `subject` mid-delivery → status reflects old subject, spec shows new subject
2. **Race Conditions**: User updates `channels` while controller is iterating through them
3. **Audit Trail Gap**: Delivered notification doesn't match current spec
4. **Sanitization Bypass**: User could update `body` after sanitization but before delivery
5. **Test Explosion**: +18 integration tests just for spec mutation edge cases

**Current Behavior**: NotificationRequestSpec has NO immutability validation - users can update any field anytime.

---

## Key Requirements

1. **Data Integrity**: Prevent race conditions during delivery
2. **Auditability**: Spec must match what was actually delivered
3. **User Control**: Allow users to cancel notifications
4. **Simplicity**: Reduce controller complexity
5. **Test Reduction**: Eliminate spec mutation test scenarios

---

## Alternatives Considered

### Alternative 1: Fully Mutable Spec (Current Behavior)

**Approach**: No validation, users can update any spec field anytime.

**Pros**:
- ✅ Flexible - users can "fix typos" in subject/body

**Cons**:
- ❌ **Race conditions**: Spec update during delivery causes inconsistent state
- ❌ **Status corruption**: Status may reflect old spec values
- ❌ **Audit trail gap**: Cannot determine what was actually delivered
- ❌ **Test explosion**: +9 integration tests for spec mutation scenarios
- ❌ **Controller complexity**: Need observedGeneration tracking
- ❌ **Production risk**: HIGH - Race conditions, data corruption

**Confidence**: 10% (rejected - too risky)

---

### Alternative 2: Fully Immutable Spec (RECOMMENDED) ⭐

**Approach**: Make ALL NotificationRequestSpec fields immutable after CRD creation.

**Design**:
```go
// NotificationRequestSpec defines the desired state of NotificationRequest
//
// DD-NOT-005: Spec Immutability
// ALL spec fields are immutable after CRD creation. Users cannot update
// notification content once created. To change a notification, delete
// and recreate the CRD.
//
// Rationale: Notifications are immutable events, not mutable resources.
// This prevents race conditions, simplifies controller logic, and provides
// perfect audit trail.
//
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation (DD-NOT-005)"
type NotificationRequestSpec struct {
    // Type of notification (escalation, simple, status-update)
    // +kubebuilder:validation:Required
    Type NotificationType `json:"type"`

    // Priority of notification (critical, high, medium, low)
    // +kubebuilder:validation:Required
    // +kubebuilder:default=medium
    Priority NotificationPriority `json:"priority"`

    // List of recipients for this notification
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MinItems=1
    Recipients []Recipient `json:"recipients"`

    // Subject line for notification
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=500
    Subject string `json:"subject"`

    // Notification body content
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MinLength=1
    Body string `json:"body"`

    // Delivery channels to use
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MinItems=1
    Channels []Channel `json:"channels"`

    // All other fields also immutable...
}
```

**User Operations**:
```bash
# ✅ ALLOWED: Create notification
kubectl apply -f notification.yaml

# ✅ ALLOWED: Delete notification (cancellation)
kubectl delete notificationrequest notification-123

# ❌ FORBIDDEN: Update spec
kubectl edit notificationrequest notification-123
# Error: spec is immutable after creation (DD-NOT-005)

# ✅ WORKAROUND: Delete + recreate
kubectl delete notificationrequest notification-123
kubectl apply -f notification-corrected.yaml
```

**Pros**:
- ✅ **Zero race conditions**: Spec never changes after creation
- ✅ **Perfect audit trail**: Spec matches what was delivered
- ✅ **Controller simplicity**: No observedGeneration tracking needed
- ✅ **Test reduction**: -9 integration tests (DD-NOT-003 V2.0: 122 → 113 tests)
- ✅ **Clear semantics**: One NotificationRequest = one immutable notification event
- ✅ **Kubernetes native**: Deletion is standard cancellation mechanism
- ✅ **Production risk**: LOW - No spec mutation bugs possible

**Cons**:
- ⚠️ **No in-place edits**: Users must delete + recreate to fix typos
  - **Mitigation**: Delete + recreate is < 1 second operation
- ⚠️ **Learning curve**: Users may expect mutability
  - **Mitigation**: Clear validation error message, user documentation

**Confidence**: 95% (approved - simple, safe, user-validated)

---

### Alternative 3: Selective Immutability with Cancellation Toggle

**Approach**: Make notification content immutable, but add `spec.cancelled: bool` toggle.

**Design**:
```go
type NotificationRequestSpec struct {
    // Immutable fields
    Subject string `json:"subject"` // +kubebuilder:validation:XValidation:rule="self == oldSelf"
    Body    string `json:"body"`    // +kubebuilder:validation:XValidation:rule="self == oldSelf"

    // Mutable toggle
    // +kubebuilder:validation:Optional
    // +kubebuilder:default=false
    Cancelled bool `json:"cancelled,omitempty"` // User can set to true
}
```

**Pros**:
- ✅ Explicit cancellation (vs deletion)

**Cons**:
- ❌ **More complex**: Still need observedGeneration for `cancelled` field
- ❌ **Confusing**: "Why can I update `cancelled` but not `subject`?"
- ❌ **Edge case**: What if `cancelled=true` after already `Sent`?
- ❌ **Not simpler**: Doesn't reduce test count (still need toggle mutation tests)

**Confidence**: 40% (rejected - complexity without benefit)

---

## Decision

### **APPROVED: Alternative 2 - Fully Immutable Spec** ⭐

**Rationale**:

1. **User Validation**: User explicitly said "avoid users overwriting specs"
2. **Simplicity**: Zero spec mutation → zero spec mutation bugs
3. **Audit Trail**: Spec always matches what was delivered
4. **Test Reduction**: -9 integration tests from DD-NOT-003 V2.0
5. **Controller Simplicity**: No observedGeneration tracking needed
6. **No Toggle Fields Needed**: NotificationRequest has no enable/disable requirements

**Key Insight**: **Notifications are immutable events, not mutable resources.**

---

## Implementation

### **1. CRD Schema Update**

**File**: `api/notification/v1alpha1/notificationrequest_types.go`

**Add immutability validation**:

```go
// NotificationRequestSpec defines the desired state of NotificationRequest
//
// DD-NOT-005: Spec Immutability
// ALL spec fields are immutable after CRD creation. Users cannot update
// notification content once created. To change a notification, delete
// and recreate the CRD.
//
// Rationale: Notifications are immutable events, not mutable resources.
// This prevents race conditions, simplifies controller logic, and provides
// perfect audit trail.
//
// Cancellation: Delete the NotificationRequest CRD to cancel delivery.
//
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation (DD-NOT-005)"
type NotificationRequestSpec struct {
    // ... existing fields unchanged
}
```

**Validation Behavior**:
```bash
# User tries to update spec
$ kubectl patch notificationrequest notif-123 --type merge -p '{"spec":{"subject":"New Subject"}}'

# Kubernetes API rejects with validation error:
Error from server: admission webhook denied: spec is immutable after creation (DD-NOT-005)
```

---

### **2. Controller Simplification**

**File**: `internal/controller/notification/notificationrequest_controller.go`

**Current Code (line 98)**:
```go
notification.Status.ObservedGeneration = notification.Generation
```

**Assessment**: Check if observedGeneration is actually used for spec change detection.

**Expected Change**:
```go
// ❌ REMOVE: observedGeneration tracking (not needed for immutable spec)
// notification.Status.ObservedGeneration = notification.Generation

// ❌ REMOVE: Spec change detection (if it exists)
// if notification.Status.ObservedGeneration < notification.Generation {
//     log.Info("Spec changed, re-evaluating...")
// }
```

**Keep**:
```go
// ✅ KEEP: Generation field for K8s metadata (standard)
// Not used for spec change detection with immutable specs
```

---

### **3. Integration Test Updates**

**File**: `DD-NOT-003-EXPANDED-INTEGRATION-TEST-PLAN-V2.0.md`

**Tests to REMOVE** (9 tests):

**Category 1B: CRD Update Scenarios** - REMOVED (6 tests)
- ~~Test 11~~: Update spec during Pending phase
- ~~Test 12~~: Update spec during Sending phase
- ~~Test 13~~: Update spec during Sent phase
- ~~Test 14~~: Update status manually
- ~~Test 15~~: Conflicting observedGeneration
- ~~Test 16~~: Update labels during reconciliation

**Category 1D: Generation Tracking** - REMOVED (3 tests)
- ~~Test 23~~: ObservedGeneration lag
- ~~Test 24~~: ObservedGeneration = 0 initially
- ~~Test 25~~: Rapid successive updates (generation storm)

**Updated Test Count**: 122 → **113 tests** (-7%)

---

### **4. User Documentation**

**File**: `docs/services/crd-controllers/06-notification/user-guide.md` (new)

```markdown
# NotificationRequest User Guide

## Spec Immutability (DD-NOT-005)

**ALL NotificationRequest spec fields are immutable after creation.**

### Why Immutability?

Notifications represent **events** (something that happened), not resources (something that exists). Events should be immutable to maintain:

1. **Audit trail integrity**: Spec matches what was delivered
2. **Race condition prevention**: No concurrent updates during delivery
3. **Status consistency**: Status always reflects operations on current spec

### How to Cancel a Notification

**Delete the CRD**:

```bash
# Cancel in-flight notification
kubectl delete notificationrequest notification-123

# NotificationController stops delivery and cleans up
```

### How to Fix a Mistake

**Delete and recreate**:

```bash
# Oops, wrong subject
kubectl delete notificationrequest notification-789

# Create corrected notification
kubectl apply -f notification-corrected.yaml
```

### What You CANNOT Do

```bash
# ❌ FORBIDDEN: Update spec
kubectl edit notificationrequest notification-123
# Error: spec is immutable after creation (DD-NOT-005)

# ❌ FORBIDDEN: Patch spec
kubectl patch notificationrequest notification-456 --type merge -p '{"spec":{"subject":"new"}}'
# Error: spec is immutable after creation (DD-NOT-005)
```

### What You CAN Do

```bash
# ✅ View spec (read-only)
kubectl get notificationrequest notification-123 -o yaml

# ✅ View status
kubectl get notificationrequest notification-123 -o jsonpath='{.status.phase}'

# ✅ Delete (cancellation)
kubectl delete notificationrequest notification-123

# ✅ Create new
kubectl apply -f notification.yaml
```
```

---

## Consequences

### **Positive**

- ✅ **Zero spec mutation bugs**: Immutability enforced by Kubernetes validation
- ✅ **Perfect audit trail**: Spec never changes, status always consistent
- ✅ **Controller simplicity**: No observedGeneration tracking needed (~20 lines removed)
- ✅ **Test reduction**: -9 integration tests from DD-NOT-003 V2.0
- ✅ **Clear semantics**: One CRD = one notification event
- ✅ **Kubernetes native**: Deletion as cancellation is standard pattern
- ✅ **Production risk**: LOW - Entire class of bugs eliminated

### **Negative**

- ⚠️ **No in-place edits**: Users must delete + recreate to fix mistakes
  - **Mitigation**: Delete + recreate is < 1 second, acceptable trade-off
  - **Documentation**: Clear examples in user guide
- ⚠️ **Learning curve**: Users may expect mutability initially
  - **Mitigation**: Validation error message references DD-NOT-005
  - **Mitigation**: User documentation explains rationale

### **Neutral**

- 🔄 **Test Plan Update**: DD-NOT-003 V2.0 reduced from 122 → 113 tests
- 🔄 **Controller Update**: Remove observedGeneration tracking (simplification)
- 🔄 **Documentation**: User guide explains immutability pattern

---

## Integration with RemediationOrchestrator

### **Deletion Handling by RO (Future Implementation)**

When RemediationOrchestrator (RO) creates NotificationRequest CRDs, it must handle user-initiated deletion gracefully.

#### **Owner Reference Pattern**

```yaml
apiVersion: notification.kubernaut.io/v1alpha1
kind: NotificationRequest
metadata:
  name: notif-for-rr-123
  ownerReferences:
    - apiVersion: remediation.kubernaut.io/v1alpha1
      kind: RemediationRequest
      name: rr-123
      uid: <uuid>
      controller: true
      blockOwnerDeletion: false  # Allow independent user deletion
```

**Behavior**:
- ✅ Deleting RemediationRequest → cascades to NotificationRequest (automatic cleanup)
- ✅ Deleting NotificationRequest independently → RO detects via watch (user cancellation)

#### **RO Detection Logic (Expected Behavior)**

```go
// RO watches NotificationRequest status
if apierrors.IsNotFound(err) {
    if rr.DeletionTimestamp != nil {
        // Case 1: RR being deleted → expected cascade
        log.Info("NotificationRequest deleted as part of RR cleanup")
        return nil
    } else {
        // Case 2: User-initiated cancellation
        log.Warn("NotificationRequest deleted by user")

        // DO NOT mark RR as Failed (user cancellation ≠ system failure)
        rr.Status.Phase = RemediationPhaseCompleted
        rr.Status.NotificationStatus = "Cancelled"

        // Set condition indicating user cancellation
        meta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
            Type:    "NotificationDelivered",
            Status:  metav1.ConditionFalse,
            Reason:  "UserCancelled",
            Message: "NotificationRequest deleted before delivery completed",
        })

        // DO NOT trigger escalation (respect user intent)
        return r.Status().Update(ctx, rr)
    }
}
```

#### **Design Principles**

1. **User Cancellation ≠ System Failure**: RO should mark RemediationRequest as `Completed` (not `Failed`) when NotificationRequest is deleted by users
2. **No Automatic Re-creation**: RO should NOT recreate deleted NotificationRequest (respect user intent)
3. **No Escalation**: User cancellation should NOT trigger escalation workflows
4. **Clear Audit Trail**: Condition `NotificationDelivered=False` with reason `UserCancelled` provides audit trail
5. **Status Propagation**: RO sets `rr.status.notificationStatus = "Cancelled"` for observability

#### **Related Design Decision**

See **DD-RO-001: Notification Cancellation Handling** for comprehensive RemediationOrchestrator behavior when NotificationRequest is deleted.

---

## Validation Results

### **Confidence Assessment**

- Initial assessment: 85% (analyzing alternatives)
- After user feedback: 95% (user validated system-wide concern)
- After implementation planning: 95% (clear path, low risk)

### **Key Validation Points**

- ✅ **User Validated**: User explicitly stated need to avoid spec overwrites
- ✅ **Reduces Complexity**: -9 integration tests, simpler controller
- ✅ **Kubernetes Convention**: Deletion as cancellation is standard
- ✅ **No Toggle Fields**: NotificationRequest has no enable/disable requirements
- ✅ **Production Safe**: Zero spec mutation bugs possible

---

## Related Decisions

- **Builds On**: ADR-034 (Unified Audit Table) - immutable audit events
- **Aligns With**: ADR-001 (CRD Microservices) - CRD best practices (updated with immutability principle)
- **Supports**: BR-NOT-050 (Data Loss Prevention), BR-NOT-051 (Audit Trail), BR-NOT-053 (At-Least-Once Delivery)
- **Simplifies**: DD-NOT-003 V2.0 (Integration Tests) - reduces from 122 → 113 tests
- **Informs**: DD-RO-001 (Notification Cancellation Handling) - RO behavior when NotificationRequest deleted
- **Pattern For**: Future CRD immutability decisions (DD-REM-XXX, DD-AI-XXX, etc.)

---

## Review & Evolution

### **When to Revisit**

- If users strongly request in-place editing (collect feedback for 6 months)
- If cancellation via deletion causes operational issues (monitor for 3 months)
- If notification patterns emerge requiring spec mutation (unlikely)

### **Success Metrics**

- **Spec Mutation Bugs**: 0 (entire class eliminated)
- **Integration Test Stability**: Flakiness < 1% (simplified tests)
- **User Satisfaction**: Feedback on immutability UX (collect for 6 months)
- **Production Incidents**: Spec-related incidents = 0

---

## Implementation Timeline

### **Phase 1: CRD Schema Update** (2 hours)

**File**: `api/notification/v1alpha1/notificationrequest_types.go`

**Changes**:
1. Add DD-NOT-005 comment block above `NotificationRequestSpec`
2. Add XValidation rule: `+kubebuilder:validation:XValidation:rule="self == oldSelf"`
3. Regenerate CRD manifests: `make manifests`
4. Reinstall CRDs: `make install`

---

### **Phase 2: Controller Analysis** (1 hour)

**File**: `internal/controller/notification/notificationrequest_controller.go`

**Tasks**:
1. Search for observedGeneration usage
2. Search for spec change detection logic
3. Determine if simplification is possible

**Current Usage** (line 98):
```go
notification.Status.ObservedGeneration = notification.Generation
```

**Analysis Needed**: Is observedGeneration actually used for decisions, or just informational?

---

### **Phase 3: Test Plan Update** (1 hour)

**File**: `DD-NOT-003-EXPANDED-INTEGRATION-TEST-PLAN-V2.0.md`

**Changes**:
1. Remove 9 spec mutation tests from Category 1
2. Update test count: 122 → 113
3. Update timeline: Redistribute 6 hours saved
4. Add note referencing DD-NOT-005

---

### **Phase 4: Documentation** (2 hours)

**New Files**:
1. `docs/services/crd-controllers/06-notification/user-guide.md` - User-facing immutability guide
2. `docs/services/crd-controllers/06-notification/DD-NOT-005-MIGRATION-GUIDE.md` - For existing deployments (if any)

**Updated Files**:
1. `docs/services/crd-controllers/06-notification/README.md` - Add immutability note
2. `docs/architecture/decisions/ADR-001-crd-microservices-architecture.md` - Add immutability principle

---

## Migration Considerations

### **Existing Deployments** (if any)

**Risk Assessment**: LOW (NotificationController is new, likely no production deployments)

**Migration Steps** (if needed):
1. Update CRD schema with immutability validation
2. Existing NotificationRequest CRDs are grandfathered (not affected)
3. New NotificationRequest CRDs enforce immutability
4. No breaking changes for existing CRDs

**Rollback**: Remove XValidation rule, reinstall CRDs

---

## Future Pattern

### **Template for Future CRD Immutability Decisions**

When building new CRD controllers, teams should create DD-XXX-YYY decision following this template:

**DD-REM-XXX: RemediationRequest Spec Immutability** (future example)
- Workflow data fields: Immutable
- Toggle fields: `spec.rejected: bool` (mutable)
- Rationale: [Specific to RemediationRequest]

**DD-AI-XXX: AIAnalysis Spec Immutability** (future example)
- All fields immutable (AI analysis context shouldn't change)
- No toggle fields needed
- Rationale: [Specific to AIAnalysis]

**Pattern Established**: Start with DD-NOT-005, each new CRD creates its own DD-XXX-YYY

---

**Prepared By**: AI Assistant (DD-NOT-005: NotificationRequest Spec Immutability)
**Date**: 2025-11-28
**User Input**: "We need to avoid users overwriting the specs of our CRDs because that will cause havoc in the flow"
**Status**: PROPOSAL - Awaiting user approval
**Confidence**: 95% (user-validated, focused scope, immediate value)
**Implementation Effort**: 6 hours (CRD update + controller analysis + test plan + docs)

