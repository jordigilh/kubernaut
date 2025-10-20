# Forced Recommendation V2 Feature - Documentation Summary

**Date**: October 20, 2025
**Status**: ✅ APPROVED FOR V2
**Decision**: Defer to V2 (after V1 validation)

---

## 📋 **EXECUTIVE SUMMARY**

The **Forced Recommendation and Manual Override** feature has been **approved for V2** implementation. This feature will allow operators to execute specific remediation actions while maintaining Kubernaut's audit trail and effectiveness tracking.

**Current V1 Limitation**: When operators reject AI recommendations, they must use manual `kubectl` commands, which bypasses Kubernaut's tracking.

**V2 Solution**: Add `forcedRecommendation` and `bypassAIAnalysis` fields to `RemediationRequest` CRD.

---

## 📄 **CREATED DOCUMENTS**

### **1. Business Requirement Document**

**File**: `docs/requirements/BR-RR-001-FORCED-RECOMMENDATION-MANUAL-OVERRIDE.md`

**Contents**:
- ✅ Business need and problem statement
- ✅ 3 detailed use cases (alternative recommendation, expert override, postmortem learning)
- ✅ 5 functional requirements (CRD schema, bypass flag, orchestrator logic, audit trail, safety validation)
- ✅ Non-functional requirements (performance, security, compliance)
- ✅ Implementation phases (6 weeks total effort)
- ✅ Success metrics (adoption, effectiveness, time savings, audit compliance)
- ✅ Alternatives considered (3 options, all rejected)

**Key Sections**:
- **Use Case 1**: Alternative recommendation after rejection (scale to 3 instead of AI's 5)
- **Use Case 2**: Expert operator override (bypass AI analysis for known fixes)
- **Use Case 3**: Incident postmortem learning (test alternative approaches)

---

### **2. Architecture Decision Record**

**File**: `docs/architecture/decisions/ADR-026-forced-recommendation-manual-override.md`

**Contents**:
- ✅ Context and problem statement
- ✅ Decision drivers (audit trail, operator autonomy, system maturity, architectural complexity)
- ✅ Decision: Defer to V2 with detailed rationale
- ✅ V2 implementation design (CRD schema, orchestrator logic, Rego validation)
- ✅ Complete code examples (Go controller logic, Rego policies, SQL schema)
- ✅ Implementation timeline (6 weeks, phased approach)
- ✅ Consequences (positive and negative)
- ✅ Alternatives considered (3 options, all rejected)

**Key Decisions**:
- **V1 Scope**: AI-driven flow only (validate core value proposition)
- **V2 Scope**: Add forced recommendations (informed by V1 usage data)
- **Rationale**: V1 success metrics first, then add manual override based on observed needs

---

### **3. Updated Approval Rejection Documentation**

**File**: `docs/APPROVAL_REJECTION_BEHAVIOR_DETAILED.md`

**Updates**:
- ✅ Corrected to show `forcedRecommendation` does NOT exist in V1
- ✅ Added "Feature Gap - Approved for V2" section
- ✅ Referenced BR-RR-001 and ADR-026
- ✅ Updated FAQs to reflect V2 approval
- ✅ Clarified current V1 workarounds (manual kubectl recommended)

---

## 🎯 **KEY DECISIONS**

### **Decision 1: Defer to V2, Not V1**

**Rationale**:
1. ✅ V1 must validate core AI-driven remediation flow first
2. ✅ Gather real-world usage patterns before adding manual override
3. ✅ Simplify V1 implementation (lower risk, faster to production)
4. ✅ V2 design informed by V1 metrics (not speculation)

**Risk Mitigation**: Document as known V1 limitation, gather operator feedback

---

### **Decision 2: Full Audit Trail Required**

**Requirements**:
- ✅ Operator identity captured (`forcedBy` field)
- ✅ Justification required (`justification` field)
- ✅ Database schema tracks forced recommendations
- ✅ Effectiveness metrics separate AI vs operator-initiated

---

### **Decision 3: Rego Policy Validation**

**Safety**: Forced recommendations validated by Rego policies

**Examples**:
- ✅ Production restrictions (require approval for P0/P1)
- ✅ Dangerous action denial (no delete-deployment in production)
- ✅ Role-based validation (storage actions require storage admin role)

---

## 🔧 **V2 IMPLEMENTATION DESIGN**

### **CRD Schema**

```yaml
# V2: RemediationRequest with forced recommendation
apiVersion: remediation.kubernaut.io/v1alpha1
kind: RemediationRequest
spec:
  # NEW: Forced recommendation (optional)
  forcedRecommendation:
    action: "scale-deployment"
    parameters:
      deployment: "webapp"
      targetReplicas: 3
    justification: "Resource constraints - scaling to 3 instead of AI's 5"
    forcedBy: "ops-engineer@company.com"

  # NEW: Skip AI analysis (optional, default: false)
  bypassAIAnalysis: true
```

---

### **RemediationOrchestrator Logic**

```go
func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) {
    // V2: Check for forced recommendation
    if rr.Spec.ForcedRecommendation != nil {
        // Validate with Rego policy
        if err := r.validateForcedRecommendation(ctx, &rr); err != nil {
            return r.handlePolicyDenial(ctx, &rr, err)
        }

        if rr.Spec.BypassAIAnalysis {
            // Skip AIAnalysis, create WorkflowExecution directly
            return r.createWorkflowFromForcedRecommendation(ctx, &rr)
        }
    }

    // V1: Normal flow
    return r.handleNormalFlow(ctx, &rr)
}
```

---

### **Database Schema**

```sql
-- V2: Track forced recommendations
ALTER TABLE action_history ADD COLUMN forced_by VARCHAR(255);
ALTER TABLE action_history ADD COLUMN forced_justification TEXT;
ALTER TABLE action_history ADD COLUMN bypassed_ai_analysis BOOLEAN DEFAULT false;

-- Query effectiveness: AI vs Operator
SELECT
    CASE
        WHEN forced_by IS NULL THEN 'AI-Generated'
        ELSE 'Operator-Forced'
    END AS source,
    action_type,
    COUNT(*) as total,
    ROUND(100.0 * SUM(CASE WHEN success THEN 1 ELSE 0 END) / COUNT(*), 2) as success_rate
FROM action_history
GROUP BY source, action_type;
```

---

## 📊 **SUCCESS METRICS**

### **V1 (Establish Baseline)**

**Metrics to Measure**:
- AI recommendation accuracy (baseline: 71-86% from HolmesGPT)
- Approval vs auto-execute ratio
- Rejection frequency and reasons
- Manual kubectl usage patterns

**Target**: 90% operator satisfaction with AI recommendations

---

### **V2 (Measure Enhancement)**

**Adoption Metrics**:
- **Target**: 20% of rejected approvals → forced recommendation retry
- **Measure**: Track forced recommendation usage after rejections

**Effectiveness Metrics**:
- **Target**: 85% success rate for forced recommendations
- **Measure**: Compare forced vs AI-generated effectiveness

**Time Savings Metrics**:
- **Target**: Average 1.5 minute time savings per bypassed AI analysis
- **Measure**: Track `bypassAIAnalysis=true` execution times

**Audit Compliance**:
- **Target**: 100% of forced recommendations have complete audit trail
- **Measure**: Audit log completeness validation

---

## 📅 **IMPLEMENTATION TIMELINE**

### **V1 (Q4 2025)** ✅

- ✅ AI-driven remediation flow only
- ✅ Document forced recommendation as known limitation
- ✅ Create BR-RR-001 and ADR-026
- ✅ Gather operator feedback on rejection patterns

### **V2 (Q1-Q2 2026)** 📅

**Phase 1: CRD Schema** (1 week)
- Add `forcedRecommendation` and `bypassAIAnalysis` fields
- Update CRD validation webhooks

**Phase 2: RemediationOrchestrator** (2 weeks)
- Implement bypass logic
- Add forced workflow creation
- Add audit logging

**Phase 3: Safety & Validation** (1 week)
- Implement Rego policy validation
- Add policy denial handling
- Add security audit logging

**Phase 4: Data Storage** (1 week)
- Update database schema
- Add effectiveness tracking queries
- Add forced recommendation reports

**Phase 5: Testing & Documentation** (1 week)
- Unit tests (forced recommendation flow)
- Integration tests (bypass flow, policy validation)
- E2E tests (operator scenarios)
- User documentation updates

**Total V2 Effort**: 6 weeks

---

## ✅ **BENEFITS**

### **Operator Experience**

- ✅ Execute alternative recommendations within Kubernaut
- ✅ Bypass AI analysis for known fixes (save 1-2 minutes)
- ✅ Test postmortem alternatives systematically

### **Audit & Compliance**

- ✅ Complete audit trail for all remediations (AI + operator)
- ✅ Operator identity and justification captured
- ✅ Compliance-ready audit reports

### **System Learning**

- ✅ Effectiveness tracking for forced recommendations
- ✅ Compare AI vs operator decision quality
- ✅ Improve AI training with operator feedback

### **Safety**

- ✅ Rego policy validation for forced actions
- ✅ Production restrictions enforced
- ✅ Role-based access control

---

## 🔗 **RELATED DOCUMENTATION**

### **V1 Documentation**

- `docs/APPROVAL_REJECTION_BEHAVIOR_DETAILED.md`: Current rejection behavior
- `api/remediation/v1alpha1/remediationrequest_types.go`: V1 CRD schema
- `docs/architecture/decisions/ADR-018-approval-notification-v1-integration.md`: Approval workflow

### **V2 Documentation**

- `docs/requirements/BR-RR-001-FORCED-RECOMMENDATION-MANUAL-OVERRIDE.md`: Business requirements
- `docs/architecture/decisions/ADR-026-forced-recommendation-manual-override.md`: Architecture decision
- `docs/design/CANONICAL_ACTION_TYPES.md`: 29 canonical action types

---

## 📋 **NEXT STEPS**

### **V1 (Before Launch)**

1. ✅ Complete V1 implementation and testing
2. ✅ Document known limitation in user docs
3. ✅ Create operator feedback mechanism
4. ✅ Track rejection patterns and manual kubectl usage

### **V2 (After V1 Launch)**

1. 📅 Review V1 operator feedback (Q1 2026)
2. 📅 Analyze rejection patterns and manual usage
3. 📅 Refine V2 design based on V1 data
4. 📅 Implement V2 forced recommendation feature (6 weeks)
5. 📅 Launch V2 with forced recommendation support

---

## ✅ **APPROVAL STATUS**

**Business Requirement**: ✅ **APPROVED** (BR-RR-001)
**Architecture Decision**: ✅ **APPROVED** (ADR-026)
**Target Version**: V2 (Q1-Q2 2026)
**Implementation Effort**: 6 weeks
**Priority**: Medium
**Date**: October 20, 2025

---

## 📞 **CONTACT**

**Questions**: Refer to BR-RR-001 and ADR-026 for complete details
**Feedback**: Track operator feedback during V1 deployment
**Updates**: Will be reviewed after V1 production deployment

---

**Document Version**: 1.0
**Last Updated**: October 20, 2025
**Status**: ✅ V2 Feature Approved - Ready for V1 Launch


