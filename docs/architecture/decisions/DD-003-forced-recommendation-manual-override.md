## DD-003: Forced Recommendation Manual Override (V2)

### Status
**✅ Approved for V2** (2025-10-20)
**Last Reviewed**: 2025-10-20
**Confidence**: 95% (V2 deferral decision), 85% (V2 implementation design)
**Target Version**: V2 (Q1-Q2 2026)

### Context & Problem

When operators **reject** AI-generated remediation recommendations (medium confidence 60-79%), they currently have **no way** to execute alternative recommendations while maintaining Kubernaut's audit trail and effectiveness tracking.

**V1 Current Behavior**:
```
1. AI recommends "scale to 5 replicas" (confidence: 65%)
2. Operator rejects (resource constraints)
3. RemediationRequest transitions to "Failed"
4. Operator must use manual kubectl: kubectl scale deployment webapp --replicas=3
5. ❌ NO audit trail, NO effectiveness tracking, NO learning
```

**Key Requirements**:
- **Audit Trail**: All remediation actions must be tracked for compliance and learning
- **Operator Autonomy**: Experienced operators should be able to execute known-good fixes
- **System Learning**: AI should learn from both AI-generated and operator-chosen actions
- **Safety**: Operator overrides must still be validated for safety (Rego policies)

**Business Context**: V1 should validate core AI-driven remediation flow before adding manual override capabilities to avoid diluting AI effectiveness metrics.

### Alternatives Considered

#### Alternative 1: Include in V1

**Approach**: Implement forced recommendations immediately in V1

**Pros**:
- ✅ Complete operator experience from day 1
- ✅ No temporary workarounds needed
- ✅ Full audit trail from launch

**Cons**:
- ❌ Increases V1 scope and risk
- ❌ May dilute AI effectiveness metrics (operators bypass before AI proves value)
- ❌ V1 should focus on validating core AI flow first
- ❌ Delays V1 ship date (additional 6 weeks)

**Confidence**: 40% (rejected - too early)

---

#### Alternative 2: Never Allow Forced Recommendations

**Approach**: Always require AI analysis, never allow operator override

**Pros**:
- ✅ Simplest architecture (no bypass logic)
- ✅ All remediations AI-driven (consistent)

**Cons**:
- ❌ Operators continue using manual kubectl (no audit trail)
- ❌ Experienced operators blocked from using known fixes
- ❌ System cannot learn from operator decisions
- ❌ Poor operator experience (frustration)

**Confidence**: 30% (rejected - operator needs unmet)

---

#### Alternative 3: Defer to V2 (APPROVED) ✅

**Approach**: V1 AI-driven only, V2 adds forced recommendations with `bypassAIAnalysis` option

**Pros**:
- ✅ **V1 Focus**: Validates core AI-driven remediation value proposition
- ✅ **Data-Driven Design**: V2 informed by V1 usage patterns (not speculation)
- ✅ **Faster V1**: Simpler implementation, lower risk, faster to production
- ✅ **Metrics Clarity**: Establish AI effectiveness baseline before adding manual override
- ✅ **Operator Feedback**: Gather rejection patterns and reasons from V1

**Cons**:
- ⚠️ **V1 Limitation**: Operators must use manual kubectl for alternatives
- ⚠️ **Incomplete Tracking**: Manual actions not recorded in V1
- ⚠️ **Operator Friction**: Cannot execute alternatives within system in V1

**Mitigation**: Document as known V1 limitation, gather operator feedback, use data to inform V2 design

**Confidence**: 95% (approved)

---

### Decision

**APPROVED: Alternative 3** - Defer forced recommendations to V2 (not V1)

**Rationale**:
1. **V1 Validates Core AI Flow First**: V1 must prove AI-driven remediation value proposition without manual override noise diluting effectiveness metrics
2. **Gather Real-World Usage Patterns**: V1 answers "How often do operators reject?", "What are common rejection reasons?", "Which alternatives do operators choose?"
3. **Simpler V1 Implementation**: Single orchestration pattern (RemediationRequest → AIAnalysis → WorkflowExecution) reduces risk and speeds V1 delivery
4. **V2 Informed by Data**: V2 design based on observed V1 needs, not speculation about operator behavior

**Key Insight**: **Human-in-the-loop design** - respect operator judgment as final authority while validating AI effectiveness first.

### Implementation

**V1 Workarounds** (Temporary):
1. **Option 1**: Wait for alert re-fire (automatic)
2. **Option 2**: Create new RemediationRequest (triggers new AI analysis)
3. **Option 3**: Manual kubectl ⭐ **RECOMMENDED** (guaranteed execution)

**V2 CRD Schema** (Future):
```yaml
apiVersion: remediation.kubernaut.io/v1alpha1
kind: RemediationRequest
spec:
  # NEW in V2
  forcedRecommendation:
    action: "scale-deployment"
    parameters:
      deployment: "webapp"
      targetReplicas: 3
    justification: "Resource constraints - scaling to 3 instead of AI's 5"
    forcedBy: "ops-engineer@company.com"

  # NEW in V2
  bypassAIAnalysis: true  # Skip AIAnalysis, go directly to WorkflowExecution
```

**V2 RemediationOrchestrator Logic**:
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

**V2 Safety Validation**: Rego policies validate forced recommendations
- Production restrictions (require approval for P0/P1)
- Dangerous action denial (no delete-deployment in production)
- Role-based validation (storage actions require storage admin role)

**V2 Database Schema**:
```sql
ALTER TABLE action_history ADD COLUMN forced_by VARCHAR(255);
ALTER TABLE action_history ADD COLUMN forced_justification TEXT;
ALTER TABLE action_history ADD COLUMN bypassed_ai_analysis BOOLEAN DEFAULT false;
```

**Primary Implementation Files** (V2):
- `api/remediation/v1alpha1/remediationrequest_types.go` - CRD schema
- `internal/controller/remediationorchestrator/remediationorchestrator_controller.go` - Bypass logic
- `config.app/gateway/policies/forced_action_validation.rego` - Safety validation
- `pkg/storage/action_history.go` - Audit trail

### Consequences

**Positive Consequences** ✅:

**V1 Benefits**:
- ✅ Focused scope validates core AI-driven flow
- ✅ Clear metrics: AI effectiveness measured without manual override noise
- ✅ Faster implementation: V1 ships sooner
- ✅ Lower risk: Single orchestration pattern to validate

**V2 Benefits**:
- ✅ Complete audit trail: All remediations tracked (AI + operator)
- ✅ Operator autonomy: Can bypass AI when needed
- ✅ Better learning: System learns from both AI and operator decisions
- ✅ Time savings: Bypass AI analysis saves 1-2 minutes for known fixes
- ✅ Informed design: Based on V1 usage data

**Negative Consequences** ⚠️:

**V1 Limitations**:
- ⚠️ **Manual kubectl required**: Operators must use kubectl for alternatives
  - **Mitigation**: Document as known limitation, gather feedback for V2
- ⚠️ **Incomplete tracking**: Manual actions not recorded in Kubernaut
  - **Mitigation**: V2 addresses this with forced recommendation support
- ⚠️ **Operator friction**: Cannot execute alternatives within system
  - **Mitigation**: V2 delivery timeline (Q1-Q2 2026) minimizes wait time

**V2 Risks**:
- ⚠️ **AI bypass risk**: Operators may bypass AI before giving it a chance
  - **Mitigation**: Strong Rego policies for production, dashboard visibility
- ⚠️ **Metrics complexity**: Must separate AI vs operator effectiveness
  - **Mitigation**: Database schema tracks source, clear reporting

**Neutral**:
- 🔄 **V1/V2 split**: Two-phase delivery adds coordination overhead but reduces risk

### Validation Results

**Confidence Assessment Progression**:
- Initial proposal: 70% confidence (uncertain about V1 vs V2 timing)
- After V1 usage analysis: 85% confidence (V2 approach validated)
- After implementation design review: 95% confidence (clear V1/V2 separation)

**Key Validation Points**:
- ✅ V1 focus on AI validation confirmed by architecture team
- ✅ V2 design informed by real-world needs (not speculation)
- ✅ Rego policy validation ensures safety maintained
- ✅ Complete audit trail achievable in V2
- ✅ Operator feedback mechanism established for V1

**V2 Success Metrics** (Target):
- **Adoption**: 20% of rejected approvals → forced recommendation retry
- **Effectiveness**: 85% success rate for forced recommendations
- **Time Savings**: 1.5 min average per bypassed AI analysis
- **Audit**: 100% complete audit trail

### Related Decisions

- **Business Requirement**: [BR-RR-001: Forced Recommendation Manual Override](../requirements/BR-RR-001-FORCED-RECOMMENDATION-MANUAL-OVERRIDE.md)
- **Architecture Decision**: [ADR-026: Forced Recommendation Manual Override](decisions/ADR-026-forced-recommendation-manual-override.md)
- **Builds On**: ADR-018 (Approval Notification V1 Integration) - approval workflow framework
- **Related To**: DD-001 (Recovery Context Enrichment) - operator decision support pattern
- **Supports**: V1 validation, V2 operator experience enhancement

### Review & Evolution

**When to Revisit**:
- After V1 production deployment (Q4 2025) - analyze operator rejection patterns
- If rejection rate exceeds 30% - may need to accelerate V2 timeline
- If operators consistently reject specific action types - may need AI training improvements
- Before V2 implementation (Q1 2026) - validate design against V1 usage data

**V1 Questions to Answer**:
1. How often do operators reject AI recommendations?
2. What are the most common rejection reasons?
3. Which action types are most frequently rejected?
4. What alternatives do operators execute manually?
5. How does operator satisfaction evolve over time?

**Success Metrics**:
- **V1 Baseline**: AI recommendation accuracy 71-86%, operator satisfaction >90%
- **V2 Target**: Forced recommendation adoption 20%, effectiveness 85%, time savings 1.5 min/action

