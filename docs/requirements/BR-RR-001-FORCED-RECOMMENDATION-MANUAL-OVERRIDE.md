# BR-RR-001: Forced Recommendation and Manual Override

**Business Requirement ID**: BR-RR-001
**Category**: RemediationRequest Enhancement
**Priority**: Medium
**Target Version**: V2
**Status**: ✅ Approved for V2
**Date**: October 20, 2025

---

## 📋 **Business Need**

### **Problem Statement**

When an operator **rejects** an AI-generated remediation recommendation (medium confidence, 60-79%), they currently have **no way** to execute an alternative recommendation while maintaining Kubernaut's audit trail and effectiveness tracking.

**Current Limitations**:
1. ❌ Cannot force a specific remediation action
2. ❌ Cannot bypass AI analysis when operator knows the correct action
3. ❌ Must use manual `kubectl` commands (bypassing Kubernaut tracking)
4. ❌ No audit trail for operator-initiated remediations

**Impact**:
- Lost effectiveness tracking data (manual actions not recorded)
- Incomplete audit trail (compliance risk)
- Operator frustration (cannot execute alternatives within system)
- Learning system gap (no feedback on operator-chosen alternatives)

---

## 🎯 **Business Objective**

**Enable operators to force specific remediation actions while maintaining Kubernaut's audit trail, effectiveness tracking, and learning capabilities.**

### **Success Criteria**

1. ✅ Operators can create `RemediationRequest` with forced recommendation
2. ✅ AI analysis can be bypassed when operator specifies action
3. ✅ Forced remediations are tracked in effectiveness monitoring
4. ✅ Audit trail captures operator-initiated actions
5. ✅ System learns from operator-chosen alternatives

---

## 📊 **Use Cases**

### **Use Case 1: Alternative Recommendation After Rejection**

**Scenario**: AI recommends "scale to 5 replicas" (confidence: 65%), operator rejects due to resource constraints, wants to try "scale to 3 replicas" instead.

**Current Flow**:
```
1. AI recommends: scale to 5 replicas (65% confidence)
2. Operator rejects (resource constraints)
3. RemediationRequest fails
4. Operator executes manually: kubectl scale deployment webapp --replicas=3
5. ❌ No audit trail, no effectiveness tracking, no learning
```

**Desired Flow with BR-RR-001**:
```
1. AI recommends: scale to 5 replicas (65% confidence)
2. Operator rejects
3. Operator creates new RemediationRequest with forced recommendation:
   forcedRecommendation:
     action: "scale-deployment"
     parameters:
       deployment: "webapp"
       targetReplicas: 3
4. ✅ Kubernaut executes, tracks, and learns from operator's choice
```

---

### **Use Case 2: Expert Operator Override**

**Scenario**: Production incident, experienced operator knows exact fix, wants to bypass AI analysis time (saves 1-2 minutes).

**Current Flow**:
```
1. Alert fires
2. Wait for AI analysis (1-2 minutes)
3. AI recommendation may not match operator's known fix
4. Operator executes manually: kubectl rollout restart deployment/webapp
5. ❌ Wasted AI analysis time, no tracking
```

**Desired Flow with BR-RR-001**:
```
1. Alert fires
2. Operator creates RemediationRequest with forced recommendation:
   forcedRecommendation:
     action: "restart-deployment"
     parameters:
       deployment: "webapp"
   bypassAIAnalysis: true
3. ✅ Immediate execution, full audit trail, effectiveness tracking
```

---

### **Use Case 3: Incident Postmortem Learning**

**Scenario**: After incident resolution, team wants to test different remediation approaches to improve AI training.

**Current Flow**:
```
1. Incident resolved manually
2. Team wants to test "what would have worked better"
3. Cannot replay with different actions in Kubernaut
4. ❌ No systematic learning from postmortem analysis
```

**Desired Flow with BR-RR-001**:
```
1. Incident resolved
2. Team creates test RemediationRequests with forced recommendations
3. Compare effectiveness of different approaches
4. ✅ System learns optimal actions for similar future incidents
```

---

## 🔧 **Functional Requirements**

### **FR-RR-001-01: Forced Recommendation Field**

**Requirement**: `RemediationRequest` CRD SHALL support an optional `forcedRecommendation` field.

**CRD Schema**:
```yaml
apiVersion: remediation.kubernaut.io/v1alpha1
kind: RemediationRequest
spec:
  # ... existing fields ...

  # NEW: Forced recommendation (optional)
  forcedRecommendation:
    # Action type (from 29 canonical actions)
    action: string  # e.g., "scale-deployment", "restart-pod", "rollback-deployment"

    # Action-specific parameters
    parameters: map[string]interface{}  # e.g., {"deployment": "webapp", "targetReplicas": 3}

    # Operator justification (for audit trail)
    justification: string  # e.g., "Resource constraints - scaling to 3 instead of AI's 5"

    # Operator who forced the recommendation
    forcedBy: string  # e.g., "user@company.com"
```

**Acceptance Criteria**:
- ✅ CRD validates `action` is one of 29 canonical action types
- ✅ CRD validates required parameters for each action type
- ✅ Justification is required (min 10 chars, max 500 chars)
- ✅ `forcedBy` defaults to authenticated user (from kubectl context)

---

### **FR-RR-001-02: Bypass AI Analysis Flag**

**Requirement**: `RemediationRequest` CRD SHALL support an optional `bypassAIAnalysis` boolean flag.

**CRD Schema**:
```yaml
apiVersion: remediation.kubernaut.io/v1alpha1
kind: RemediationRequest
spec:
  # ... existing fields ...

  # NEW: Skip AI analysis (optional, default: false)
  bypassAIAnalysis: boolean  # true = skip AIAnalysis, go directly to WorkflowExecution
```

**Acceptance Criteria**:
- ✅ Default value: `false` (normal AI analysis flow)
- ✅ When `true` AND `forcedRecommendation` is set: skip AIAnalysis CRD creation
- ✅ When `true` BUT `forcedRecommendation` is NOT set: ERROR (invalid configuration)
- ✅ RemediationOrchestrator creates WorkflowExecution directly from forced recommendation

---

### **FR-RR-001-03: RemediationOrchestrator Logic**

**Requirement**: RemediationOrchestrator controller SHALL handle forced recommendations.

**Logic**:
```go
func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var rr remediationv1alpha1.RemediationRequest
    r.Get(ctx, req.NamespacedName, &rr)

    // Check for forced recommendation
    if rr.Spec.ForcedRecommendation != nil {
        if rr.Spec.BypassAIAnalysis {
            // Skip AIAnalysis, create WorkflowExecution directly
            workflow := r.createWorkflowFromForcedRecommendation(ctx, &rr)
            return r.createWorkflowExecution(ctx, &rr, workflow)
        } else {
            // Create AIAnalysis with forced recommendation hint
            // AI can validate/enhance but cannot override
            return r.createAIAnalysisWithHint(ctx, &rr)
        }
    }

    // Normal flow (no forced recommendation)
    return r.handleNormalFlow(ctx, &rr)
}
```

**Acceptance Criteria**:
- ✅ Forced recommendation bypasses AIAnalysis when `bypassAIAnalysis: true`
- ✅ Forced recommendation creates WorkflowExecution directly
- ✅ Audit logs capture operator identity and justification
- ✅ Metrics track forced vs AI-generated remediations

---

### **FR-RR-001-04: Audit Trail Enhancement**

**Requirement**: Data Storage Service SHALL track forced remediations with operator metadata.

**Database Schema**:
```sql
ALTER TABLE action_history ADD COLUMN forced_by VARCHAR(255);
ALTER TABLE action_history ADD COLUMN forced_justification TEXT;
ALTER TABLE action_history ADD COLUMN bypassed_ai_analysis BOOLEAN DEFAULT false;

CREATE INDEX idx_action_history_forced_by ON action_history(forced_by) WHERE forced_by IS NOT NULL;
```

**Acceptance Criteria**:
- ✅ Action history records capture `forced_by` field
- ✅ Forced justification stored in audit trail
- ✅ Queries can filter for operator-initiated remediations
- ✅ Effectiveness tracking distinguishes AI vs operator-initiated

---

### **FR-RR-001-05: Safety Validation**

**Requirement**: Forced recommendations SHALL be validated by Rego policies before execution.

**Validation Flow**:
```go
// WorkflowExecution Controller validates forced recommendation
func (r *WorkflowExecutionReconciler) validateForcedRecommendation(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
    regoResult, err := r.regoEvaluator.Evaluate(ctx, "kubernaut.remediation.validate_forced_action", map[string]interface{}{
        "environment":   rr.Spec.Environment,
        "priority":      rr.Spec.Priority,
        "action":        rr.Spec.ForcedRecommendation.Action,
        "parameters":    rr.Spec.ForcedRecommendation.Parameters,
        "forced_by":     rr.Spec.ForcedRecommendation.ForcedBy,
    })

    if regoResult.Decision == "deny" {
        return fmt.Errorf("forced recommendation denied by policy: %s", regoResult.Reason)
    }

    return nil
}
```

**Acceptance Criteria**:
- ✅ Rego policies can deny forced recommendations (e.g., production restrictions)
- ✅ Denied forced recommendations escalate to manual review
- ✅ Policy violations are logged with operator identity
- ✅ Audit trail captures policy denial reasons

---

## 📈 **Non-Functional Requirements**

### **NFR-RR-001-01: Performance**

- ✅ Bypassed AI analysis saves 1-2 minutes per remediation
- ✅ No performance degradation for normal flow (when not using forced recommendation)

### **NFR-RR-001-02: Security**

- ✅ Forced recommendations require authenticated user (RBAC enforced)
- ✅ Audit trail immutable (operator cannot tamper with forced_by)
- ✅ Rego policies can restrict forced recommendations by role/user

### **NFR-RR-001-03: Compliance**

- ✅ Complete audit trail for all forced remediations
- ✅ Operator identity captured from Kubernetes authentication
- ✅ Justification required for all forced recommendations

---

## 🔗 **Dependencies**

### **Upstream Dependencies**

- ✅ CRD schema updates (RemediationRequest API)
- ✅ RemediationOrchestrator controller logic
- ✅ WorkflowExecution controller validation
- ✅ Data Storage Service schema updates

### **Downstream Impacts**

- ✅ Effectiveness Monitor: Distinguish AI vs operator-initiated
- ✅ Notification Service: Include forced recommendation context
- ✅ Dashboard: Display forced vs AI-generated remediations

---

## 🚀 **Implementation Phases**

### **Phase 1: CRD Schema** (1 week)

- Add `forcedRecommendation` field to RemediationRequest CRD
- Add `bypassAIAnalysis` flag
- Update CRD validation webhooks
- Update API documentation

### **Phase 2: RemediationOrchestrator Logic** (2 weeks)

- Implement forced recommendation detection
- Add bypass logic for AIAnalysis
- Add direct WorkflowExecution creation
- Add audit logging

### **Phase 3: Safety Validation** (1 week)

- Implement Rego policy validation
- Add policy denial handling
- Add security audit logging

### **Phase 4: Data Storage Integration** (1 week)

- Update database schema
- Add forced recommendation tracking
- Update effectiveness queries

### **Phase 5: Testing & Documentation** (1 week)

- Unit tests for forced recommendation flow
- Integration tests for bypass flow
- E2E tests for operator scenarios
- Update user documentation

**Total Estimated Effort**: 6 weeks

---

## 📊 **Success Metrics**

### **Adoption Metrics**

- **Target**: 20% of rejected approvals result in forced recommendation retry
- **Measure**: Track forced recommendation usage after rejections

### **Effectiveness Metrics**

- **Target**: 85% success rate for forced recommendations
- **Measure**: Compare forced vs AI-generated effectiveness

### **Time Savings Metrics**

- **Target**: Average 1.5 minute time savings per bypassed AI analysis
- **Measure**: Track `bypassAIAnalysis=true` execution times

### **Audit Compliance**

- **Target**: 100% of forced recommendations have complete audit trail
- **Measure**: Audit log completeness for forced actions

---

## 🔄 **Alternatives Considered**

### **Alternative 1: No Forced Recommendation (V1 Current State)**

**Approach**: Operators use manual `kubectl` commands

**Rejected Because**:
- ❌ No audit trail
- ❌ No effectiveness tracking
- ❌ No learning for AI improvement

---

### **Alternative 2: AI Always Required (No Bypass)**

**Approach**: Force recommendation but still require AI analysis

**Rejected Because**:
- ❌ Wastes 1-2 minutes for experienced operators
- ❌ AI analysis may contradict operator's known-good fix
- ❌ Not useful in time-critical production incidents

---

### **Alternative 3: Separate "Manual Remediation" CRD**

**Approach**: Create new CRD type for manual remediations

**Rejected Because**:
- ❌ Duplicates RemediationRequest fields
- ❌ Increases controller complexity
- ❌ Fragments audit trail across CRD types

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V2**
**Date**: October 20, 2025
**Decision**: Defer to V2 (after V1 validation)
**Rationale**: V1 should validate core AI-driven flow before adding manual override capabilities

**Approved By**: Architecture Team
**Related ADR**: [ADR-026: Forced Recommendation Manual Override](../architecture/decisions/ADR-026-forced-recommendation-manual-override.md)

---

## 📚 **References**

### **Related Business Requirements**

- BR-AI-059: Approval context for operators
- BR-AI-060: Approval decision metadata
- BR-ORCH-001: NotificationRequest creation

### **Related Documents**

- `docs/APPROVAL_REJECTION_BEHAVIOR_DETAILED.md`: Current limitation documentation
- `api/remediation/v1alpha1/remediationrequest_types.go`: CRD schema
- `docs/design/CANONICAL_ACTION_TYPES.md`: 29 canonical action types

---

**Document Version**: 1.0
**Last Updated**: October 20, 2025
**Status**: ✅ Approved for V2 Implementation

