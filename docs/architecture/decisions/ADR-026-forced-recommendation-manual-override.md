# ADR-026: Forced Recommendation and Manual Override (V2 Feature)

**Status**: ✅ Approved
**Date**: 2025-10-20
**Deciders**: Architecture Team
**Priority**: MEDIUM
**Target Version**: V2
**Related BR**: [BR-RR-001](../../requirements/BR-RR-001-FORCED-RECOMMENDATION-MANUAL-OVERRIDE.md)

---

## Context and Problem Statement

When an operator **rejects** an AI-generated remediation recommendation (medium confidence 60-79%), they currently have **no way** to execute an alternative recommendation while maintaining Kubernaut's audit trail and effectiveness tracking.

**Current V1 Behavior**:
```
1. AI recommends "scale to 5 replicas" (confidence: 65%)
2. Operator rejects (resource constraints)
3. RemediationRequest transitions to "Failed"
4. Operator must use manual kubectl: kubectl scale deployment webapp --replicas=3
5. ❌ NO audit trail, NO effectiveness tracking, NO learning
```

**Key Questions**:
1. Should operators be able to force specific remediation actions?
2. Should operators be able to bypass AI analysis when they know the correct fix?
3. How do we maintain audit trail and safety for operator-initiated actions?
4. Should this be in V1 or V2?

---

## Decision Drivers

### **1. Audit Trail Completeness** 🔍

**Requirement**: All remediation actions must be tracked for compliance and learning

**Current Gap**: Manual `kubectl` commands bypass Kubernaut's audit trail

**Impact**:
- ❌ Incomplete compliance audit trail
- ❌ Missing effectiveness tracking data
- ❌ Lost learning opportunities for AI improvement

---

### **2. Operator Autonomy** 🙋

**Requirement**: Experienced operators should be able to execute known-good fixes

**Current Gap**: Operators cannot override AI recommendations within Kubernaut

**Impact**:
- ❌ Operator frustration (forced to work outside system)
- ❌ Time wasted on AI analysis for known fixes
- ❌ Production incidents delayed by AI analysis time

---

### **3. System Maturity** 🎯

**Consideration**: V1 should validate core AI-driven flow before adding manual overrides

**Risk**: Adding manual override too early may:
- ⚠️ Reduce AI usage (operators bypass before AI matures)
- ⚠️ Mask AI quality issues (operators work around instead of reporting)
- ⚠️ Dilute effectiveness metrics (hard to separate AI vs operator performance)

---

### **4. Architectural Complexity** 🏗️

**V1 Focus**: Validate core orchestration pattern (RemediationRequest → AIAnalysis → WorkflowExecution)

**V2 Enhancement**: Add optional bypass flow (RemediationRequest → WorkflowExecution)

**Rationale**: Clean V1/V2 separation reduces implementation risk

---

## Decision

**APPROVED: Defer forced recommendation and manual override to V2**

### **V1 Scope (Current)**
- ✅ AI-driven remediation flow only
- ✅ Approval workflow for medium confidence (60-79%)
- ✅ Operator can reject, but must use manual `kubectl` for alternatives
- ❌ NO forced recommendations
- ❌ NO bypass of AI analysis

### **V2 Scope (Approved for Future)**
- ✅ Add `forcedRecommendation` field to RemediationRequest CRD
- ✅ Add `bypassAIAnalysis` boolean flag
- ✅ RemediationOrchestrator skips AIAnalysis when bypass is enabled
- ✅ Complete audit trail for operator-initiated actions
- ✅ Effectiveness tracking for forced recommendations
- ✅ Rego policy validation for forced actions

---

## Rationale

### **Why Defer to V2?**

#### **Reason 1: Validate Core AI Flow First** 🎯

**Priority**: V1 must prove the AI-driven remediation value proposition

**Risk of V1 Inclusion**: Operators may bypass AI before it proves itself, making it impossible to validate AI effectiveness

**Mitigation**: Launch V1 with AI-only flow, collect metrics, then add manual override in V2 based on observed needs

---

#### **Reason 2: Gather Real-World Usage Patterns** 📊

**Questions to Answer in V1**:
- How often do operators reject AI recommendations?
- What are the common reasons for rejection?
- Which action types are most frequently rejected?
- What alternatives do operators execute manually?

**Value**: V2 design will be informed by actual V1 usage data, not speculation

---

#### **Reason 3: Simplify V1 Implementation** 🚀

**V1 Focus**: Core orchestration pattern
- RemediationRequest → RemediationProcessing → AIAnalysis → WorkflowExecution

**V2 Addition**: Optional bypass pattern
- RemediationRequest → WorkflowExecution (skip AIAnalysis)

**Benefit**: V1 implementation is simpler, less risky, faster to production

---

#### **Reason 4: V1 Success Metrics First** 📈

**V1 Metrics to Establish**:
- AI recommendation accuracy (71-86% baseline from HolmesGPT benchmarks)
- Approval vs auto-execute ratio
- MTTR improvement (target: 60 min → 5 min)
- Operator satisfaction with AI recommendations

**V2 Enhancement**: Add forced recommendations based on observed gaps in V1 metrics

---

## V2 Implementation Design

### **CRD Schema Changes**

```yaml
# V2: RemediationRequest with forced recommendation
apiVersion: remediation.kubernaut.io/v1alpha1
kind: RemediationRequest
metadata:
  name: forced-scale-webapp-003
spec:
  # ... existing V1 fields (signalFingerprint, signalName, etc.) ...

  # NEW in V2: Forced recommendation (optional)
  forcedRecommendation:
    # Action type (from 29 canonical actions)
    action: "scale-deployment"  # e.g., "scale-deployment", "restart-pod"

    # Action-specific parameters
    parameters:
      deployment: "webapp"
      namespace: "production"
      targetReplicas: 3

    # Operator justification (required for audit trail)
    justification: "Resource constraints - scaling to 3 instead of AI's 5"

    # Operator who forced the recommendation (auto-populated from auth)
    forcedBy: "ops-engineer@company.com"

  # NEW in V2: Skip AI analysis (optional, default: false)
  bypassAIAnalysis: true  # true = skip AIAnalysis, go directly to WorkflowExecution
```

---

### **RemediationOrchestrator Logic (V2)**

```go
package remediationorchestrator

import (
    remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var rr remediationv1alpha1.RemediationRequest
    if err := r.Get(ctx, req.NamespacedName, &rr); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // V2: Check for forced recommendation
    if rr.Spec.ForcedRecommendation != nil {
        log.Info("Forced recommendation detected",
            "action", rr.Spec.ForcedRecommendation.Action,
            "forcedBy", rr.Spec.ForcedRecommendation.ForcedBy)

        // V2: Validate forced recommendation with Rego policy
        if err := r.validateForcedRecommendation(ctx, &rr); err != nil {
            return r.handlePolicyDenial(ctx, &rr, err)
        }

        if rr.Spec.BypassAIAnalysis {
            // V2: Skip AIAnalysis, create WorkflowExecution directly
            log.Info("Bypassing AI analysis (operator override)")
            return r.createWorkflowFromForcedRecommendation(ctx, &rr)
        } else {
            // V2: Create AIAnalysis with forced recommendation as hint
            // AI can validate/enhance but cannot override operator's choice
            log.Info("Creating AIAnalysis with forced recommendation hint")
            return r.createAIAnalysisWithHint(ctx, &rr)
        }
    }

    // V1: Normal flow (no forced recommendation)
    return r.handleNormalFlow(ctx, &rr)
}

// V2: Validate forced recommendation with Rego policy
func (r *RemediationRequestReconciler) validateForcedRecommendation(
    ctx context.Context,
    rr *remediationv1alpha1.RemediationRequest,
) error {
    regoResult, err := r.regoEvaluator.Evaluate(ctx, "kubernaut.remediation.validate_forced_action", map[string]interface{}{
        "environment":   rr.Spec.Environment,
        "priority":      rr.Spec.Priority,
        "action":        rr.Spec.ForcedRecommendation.Action,
        "parameters":    rr.Spec.ForcedRecommendation.Parameters,
        "forced_by":     rr.Spec.ForcedRecommendation.ForcedBy,
    })

    if err != nil {
        return fmt.Errorf("rego evaluation failed: %w", err)
    }

    if regoResult.Decision == "deny" {
        return fmt.Errorf("forced recommendation denied by policy: %s", regoResult.Reason)
    }

    return nil
}

// V2: Create WorkflowExecution directly from forced recommendation
func (r *RemediationRequestReconciler) createWorkflowFromForcedRecommendation(
    ctx context.Context,
    rr *remediationv1alpha1.RemediationRequest,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Build WorkflowExecution from forced recommendation
    workflow := &workflowexecutionv1alpha1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-forced-wf", rr.Name),
            Namespace: rr.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(rr, remediationv1alpha1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
            RemediationRequestRef: corev1.LocalObjectReference{Name: rr.Name},
            Steps: []workflowexecutionv1alpha1.WorkflowStep{
                {
                    Name:       "forced-action",
                    ActionType: rr.Spec.ForcedRecommendation.Action,
                    Parameters: rr.Spec.ForcedRecommendation.Parameters,
                    // Mark as operator-initiated
                    Metadata: map[string]string{
                        "forced_by":          rr.Spec.ForcedRecommendation.ForcedBy,
                        "forced_justification": rr.Spec.ForcedRecommendation.Justification,
                        "bypassed_ai_analysis": "true",
                    },
                },
            },
        },
    }

    // Create WorkflowExecution
    if err := r.Create(ctx, workflow); err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to create forced workflow: %w", err)
    }

    log.Info("Forced WorkflowExecution created",
        "workflow", workflow.Name,
        "action", rr.Spec.ForcedRecommendation.Action,
        "forcedBy", rr.Spec.ForcedRecommendation.ForcedBy)

    // Update RemediationRequest status
    rr.Status.OverallPhase = "executing"
    rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
        Name:      workflow.Name,
        Namespace: workflow.Namespace,
    }

    if err := r.Status().Update(ctx, rr); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}
```

---

### **Safety: Rego Policy Validation**

```rego
# config.app/gateway/policies/forced_action_validation.rego

package kubernaut.remediation

# Allow forced recommendations in non-production environments
allow_forced if {
    input.environment in ["development", "staging", "qa"]
}

# Require approval for forced recommendations in production
require_approval_for_forced if {
    input.environment == "production"
    input.priority in ["P0", "P1"]
}

# Deny dangerous actions even when forced
deny_forced[msg] {
    input.action in ["delete-deployment", "delete-statefulset"]
    input.environment == "production"
    msg := "Destructive actions cannot be forced in production"
}

# Validate forced action against operator role
deny_forced[msg] {
    input.action in ["modify-pvc", "delete-pv"]
    not has_storage_admin_role(input.forced_by)
    msg := sprintf("User %v does not have storage admin role", [input.forced_by])
}

has_storage_admin_role(user) {
    # Check if user has storage admin role (integration with RBAC)
    # Implementation depends on authentication system
}

# Final decision
decision = "allow" {
    not deny_forced[_]
    allow_forced
}

decision = "deny" {
    deny_forced[_]
}

decision = "require_approval" {
    not deny_forced[_]
    require_approval_for_forced
}
```

---

### **Audit Trail Enhancement**

```sql
-- V2: Database schema updates for forced recommendations

ALTER TABLE action_history ADD COLUMN forced_by VARCHAR(255);
ALTER TABLE action_history ADD COLUMN forced_justification TEXT;
ALTER TABLE action_history ADD COLUMN bypassed_ai_analysis BOOLEAN DEFAULT false;

CREATE INDEX idx_action_history_forced_by ON action_history(forced_by) WHERE forced_by IS NOT NULL;

-- Query forced recommendations
SELECT
    action_type,
    forced_by,
    forced_justification,
    executed_at,
    success
FROM action_history
WHERE forced_by IS NOT NULL
ORDER BY executed_at DESC;

-- Effectiveness comparison: AI vs Operator
SELECT
    CASE
        WHEN forced_by IS NULL THEN 'AI-Generated'
        ELSE 'Operator-Forced'
    END AS source,
    action_type,
    COUNT(*) as total_actions,
    SUM(CASE WHEN success THEN 1 ELSE 0 END) as successful_actions,
    ROUND(100.0 * SUM(CASE WHEN success THEN 1 ELSE 0 END) / COUNT(*), 2) as success_rate
FROM action_history
GROUP BY source, action_type
ORDER BY action_type, source;
```

---

## Consequences

### **Positive Consequences** ✅

#### **V1 (Current)**

1. **Focused Scope**: V1 validates core AI-driven remediation flow
2. **Clear Metrics**: AI effectiveness measured without manual override noise
3. **Faster Implementation**: V1 ships sooner without forced recommendation complexity
4. **Lower Risk**: Single orchestration pattern to validate

#### **V2 (Future)**

1. **Complete Audit Trail**: All remediations tracked (AI and operator-initiated)
2. **Operator Autonomy**: Experienced operators can bypass AI when needed
3. **Better Learning**: System learns from both AI and operator decisions
4. **Time Savings**: Bypass AI analysis saves 1-2 minutes for known fixes
5. **Informed Design**: V2 design based on V1 usage data

---

### **Negative Consequences** ⚠️

#### **V1 (Current)**

1. **Manual kubectl Required**: Operators must use kubectl for alternatives
2. **Incomplete Tracking**: Manual actions not recorded in Kubernaut
3. **Operator Friction**: Cannot execute alternatives within system

**Mitigation**: Document this as known limitation, gather feedback for V2

#### **V2 (Future)**

1. **AI Bypass Risk**: Operators may bypass AI before giving it a chance
2. **Metrics Complexity**: Must separate AI vs operator effectiveness
3. **Policy Management**: Rego policies must prevent abuse of forced recommendations

**Mitigation**:
- Strong Rego policies for production environments
- Dashboard visibility into forced vs AI-generated ratio
- Alerts when forced recommendation usage exceeds thresholds

---

## Implementation Timeline

### **V1 (Q4 2025)** ✅

- ✅ AI-driven remediation flow only
- ✅ Document forced recommendation as V2 feature
- ✅ Create BR-RR-001 and ADR-026

### **V2 (Q1-Q2 2026)** 📅

**Phase 1: CRD Schema** (1 week)
- Add `forcedRecommendation` and `bypassAIAnalysis` fields
- Update CRD validation

**Phase 2: RemediationOrchestrator** (2 weeks)
- Implement bypass logic
- Add forced workflow creation
- Add audit logging

**Phase 3: Safety & Validation** (1 week)
- Implement Rego policy validation
- Add policy denial handling

**Phase 4: Data Storage** (1 week)
- Update database schema
- Add effectiveness tracking

**Phase 5: Testing** (1 week)
- Unit, integration, E2E tests
- Documentation updates

**Total V2 Effort**: 6 weeks

---

## Alternatives Considered

### **Alternative 1: Include in V1** ❌

**Approach**: Implement forced recommendations in V1

**Rejected Because**:
- ❌ Increases V1 scope and risk
- ❌ May dilute AI effectiveness metrics
- ❌ V1 should validate core AI flow first
- ❌ Delays V1 ship date

---

### **Alternative 2: Never Allow Forced Recommendations** ❌

**Approach**: Always require AI analysis, never allow operator override

**Rejected Because**:
- ❌ Operators will continue using manual kubectl (no audit trail)
- ❌ Experienced operators blocked from using known fixes
- ❌ System cannot learn from operator decisions
- ❌ Poor operator experience

---

### **Alternative 3: Separate "Manual Remediation" CRD** ❌

**Approach**: Create new CRD type for manual remediations

**Rejected Because**:
- ❌ Duplicates RemediationRequest fields
- ❌ Fragments audit trail across CRD types
- ❌ Increases controller complexity
- ❌ Harder to compare AI vs operator effectiveness

---

## Related Decisions

- **ADR-018**: Approval Notification V1 Integration (approval workflow design)
- **ADR-024**: Eliminate ActionExecution Layer (simplified execution model)
- **BR-AI-059**: Approval Context (approval decision framework)
- **BR-AI-060**: Approval Decision Metadata (audit trail requirements)

---

## References

### **V1 Documentation**

- `docs/APPROVAL_REJECTION_BEHAVIOR_DETAILED.md`: Current rejection behavior
- `api/remediation/v1alpha1/remediationrequest_types.go`: V1 CRD schema

### **V2 Requirements**

- `docs/requirements/BR-RR-001-FORCED-RECOMMENDATION-MANUAL-OVERRIDE.md`: Business requirements
- `docs/design/CANONICAL_ACTION_TYPES.md`: 29 canonical action types

---

## Decision Rationale Summary

**Why V2, Not V1?**

1. ✅ **Focus**: V1 validates core AI-driven value proposition
2. ✅ **Risk**: Avoid diluting AI metrics with operator overrides
3. ✅ **Data**: V2 design informed by V1 usage patterns
4. ✅ **Speed**: V1 ships faster without forced recommendation complexity

**Why Include in V2?**

1. ✅ **Audit Trail**: Capture all remediation actions (not just AI)
2. ✅ **Operator Experience**: Allow expert overrides within system
3. ✅ **Learning**: System learns from both AI and operator decisions
4. ✅ **Compliance**: Complete audit trail for all remediations

---

**Status**: ✅ **APPROVED - DEFERRED TO V2**
**Date**: October 20, 2025
**Next Review**: After V1 production deployment (Q1 2026)
**Confidence**: 95% (clear V1/V2 separation, informed by real-world V1 data)

