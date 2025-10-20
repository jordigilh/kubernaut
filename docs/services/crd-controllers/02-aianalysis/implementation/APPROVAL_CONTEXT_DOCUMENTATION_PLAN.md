# AIAnalysis Controller - Approval Context Documentation Plan

**Date**: October 20, 2025
**Status**: 📋 **DOCUMENTATION PLAN** (Not Yet Implemented)
**Related**: ADR-018 (Approval Notification V1.0 Integration)
**Target Version**: v1.0.5 (incremental update to existing plan)

---

## 🎯 **Purpose**

Document how to integrate **BR-AI-059** (Approval Context Capture) and **BR-AI-060** (Approval Decision Tracking) into the existing AIAnalysis Implementation Plan v1.0.4.

**These are NEW requirements** from ADR-018 that extend the existing approval workflow with rich context for notifications.

---

## 📋 **Current Status**

### **Existing Coverage** (v1.0.4):
- ✅ **BR-AI-031 to BR-AI-046**: Basic approval workflow (16 BRs)
- ✅ **Day 6**: Approval Workflow (AIApprovalRequest child CRD creation)
- ✅ **Category D**: Approval workflow error handling
- ✅ Confidence evaluation (< 60% reject, 60-79% approve, ≥80% auto-approve)
- ✅ AIApprovalRequest CRD watch and status synchronization

### **Missing Coverage** (NEW from ADR-018):
- ❌ **BR-AI-059**: Approval context capture (why approval needed, evidence, alternatives)
- ❌ **BR-AI-060**: Approval decision tracking (who, when, how, why)
- ❌ Populate `AIAnalysis.status.approvalContext` with rich context
- ❌ Update `AIAnalysis.status` with approval decision metadata

---

## 📝 **Proposed Documentation Updates**

### **Update 1: Add to Business Requirements Section**

**Location**: `IMPLEMENTATION_PLAN_V1.0.md` - Business Requirements section

**Add After Line ~119** (after "BR-AI-031 to BR-AI-046"):

```markdown
### **Approval Context & Decision Tracking (BR-AI-059, BR-AI-060) - 2 NEW BRs**

**BR-AI-059**: AIAnalysis Approval Context Capture (P0)
- **Requirement**: Capture comprehensive approval context for rich notifications
- **Fields**:
  - `status.approvalContext.reason` - Why approval is required
  - `status.approvalContext.confidenceScore` - AI confidence (0.0-1.0)
  - `status.approvalContext.confidenceLevel` - "low" | "medium" | "high"
  - `status.approvalContext.investigationSummary` - Root cause description
  - `status.approvalContext.evidenceCollected[]` - Evidence items (min 1, rec 3-5)
  - `status.approvalContext.recommendedActions[]` - Actions with rationale
  - `status.approvalContext.alternativesConsidered[]` - Alternatives with pros/cons
  - `status.approvalContext.whyApprovalRequired` - Policy justification
- **Validation**: Unit tests verify all fields populated when `phase = "approving"`
- **Integration**: Used by RemediationOrchestrator to create rich notifications

**BR-AI-060**: AIAnalysis Approval Decision Tracking (P0)
- **Requirement**: Track comprehensive approval decision metadata for audit trail
- **Fields**:
  - `status.approvalStatus` - "Approved" | "Rejected" | "Pending"
  - `status.approvedBy` / `status.rejectedBy` - Email/username of decision maker
  - `status.approvalTime` - Timestamp of approval
  - `status.approvalDuration` - Time from request to decision (e.g., "2m15s")
  - `status.approvalMethod` - "kubectl" | "dashboard" | "slack-button" | "email-link"
  - `status.approvalJustification` - Optional operator comment
  - `status.rejectionReason` - Why approval was rejected
- **Validation**: Integration tests verify metadata captured after approval
- **Audit**: Full decision trail for compliance and UX improvement tracking

**Related**: ADR-018 (Approval Notification Integration)
**Reference**: [APPROVAL_NOTIFICATION_BUSINESS_REQUIREMENTS.md](../../../../requirements/APPROVAL_NOTIFICATION_BUSINESS_REQUIREMENTS.md)
```

---

### **Update 2: Modify Day 6 (Approval Workflow)**

**Location**: `IMPLEMENTATION_PLAN_V1.0.md` - Timeline section, Day 6

**Current** (Line ~148):
```markdown
| **Day 6** | Approval Workflow (AIApprovalRequest) | 8h | Child CRD creation, approval status tracking, watch-based coordination |
```

**Proposed**:
```markdown
| **Day 6** | Approval Workflow + Context Capture | 10h | Child CRD creation, approval status tracking, watch-based coordination, BR-AI-059 context population, BR-AI-060 decision tracking |
```

**Effort Impact**: +2 hours (8h → 10h) for context population logic

---

### **Update 3: Add to Day 6 Implementation Details**

**Location**: `IMPLEMENTATION_PLAN_V1.0.md` - Day 6: Approval Workflow section

**Add New Section** (after existing approval workflow implementation):

#### **Day 6.5: Approval Context Population (BR-AI-059)**

**Goal**: Populate `AIAnalysis.status.approvalContext` with rich context for notifications

**Implementation Steps**:

1. **Extract Context from HolmesGPT Response**:
   ```go
   // In reconcileApproving phase
   func (r *AIAnalysisReconciler) populateApprovalContext(
       ctx context.Context,
       aiAnalysis *aianalysisv1alpha1.AIAnalysis,
       holmesResponse *HolmesGPTResponse,
   ) error {
       // Build ApprovalContext from HolmesGPT analysis
       approvalContext := &aianalysisv1alpha1.ApprovalContext{
           Reason: fmt.Sprintf("Medium confidence (%.1f%%) - requires human review",
               holmesResponse.Confidence * 100),
           ConfidenceScore: holmesResponse.Confidence,
           ConfidenceLevel: r.getConfidenceLevel(holmesResponse.Confidence),
           InvestigationSummary: holmesResponse.RootCause,
           EvidenceCollected: holmesResponse.Evidence,
           WhyApprovalRequired: fmt.Sprintf(
               "Confidence %.1f%% is below auto-approve threshold (80%%) per policy",
               holmesResponse.Confidence * 100,
           ),
       }

       // Map HolmesGPT recommendations to structured actions
       for _, rec := range holmesResponse.Recommendations {
           approvalContext.RecommendedActions = append(
               approvalContext.RecommendedActions,
               aianalysisv1alpha1.RecommendedAction{
                   Action: rec.Action,
                   Rationale: rec.Rationale,
               },
           )
       }

       // Extract alternatives from HolmesGPT response
       if holmesResponse.Alternatives != nil {
           for _, alt := range holmesResponse.Alternatives {
               approvalContext.AlternativesConsidered = append(
                   approvalContext.AlternativesConsidered,
                   aianalysisv1alpha1.AlternativeApproach{
                       Approach: alt.Approach,
                       ProsCons: alt.ProsCons,
                   },
               )
           }
       }

       // Update status
       aiAnalysis.Status.ApprovalContext = approvalContext
       return r.Status().Update(ctx, aiAnalysis)
   }
   ```

2. **Validate Context Completeness**:
   - MUST have `investigationSummary`
   - MUST have at least 1 `recommendedAction`
   - MUST have at least 1 `evidenceCollected` item
   - SHOULD have at least 1 `alternativeConsidered`

3. **Test Scenarios**:
   - Unit test: Verify all approval context fields populated
   - Unit test: Verify context validation rejects incomplete data
   - Integration test: Verify context used by RemediationOrchestrator for notifications

**Effort**: 2 hours (context extraction + validation)

---

#### **Day 6.6: Approval Decision Tracking (BR-AI-060)**

**Goal**: Track approval decision metadata when AIApprovalRequest is approved/rejected

**Implementation Steps**:

1. **Watch AIApprovalRequest for Decisions**:
   ```go
   // Already implemented in Day 6, extend to capture metadata
   func (r *AIAnalysisReconciler) syncApprovalDecision(
       ctx context.Context,
       aiAnalysis *aianalysisv1alpha1.AIAnalysis,
       approvalRequest *approvalv1alpha1.AIApprovalRequest,
   ) error {
       log := ctrl.LoggerFrom(ctx)

       // Capture approval metadata (BR-AI-060)
       if approvalRequest.Spec.Decision == "Approved" {
           aiAnalysis.Status.ApprovalStatus = "Approved"
           aiAnalysis.Status.ApprovedBy = approvalRequest.Spec.DecidedBy
           aiAnalysis.Status.ApprovalTime = &metav1.Time{Time: time.Now()}
           aiAnalysis.Status.ApprovalMethod = approvalRequest.Spec.DecisionMethod
           aiAnalysis.Status.ApprovalJustification = approvalRequest.Spec.Justification

           // Calculate duration
           if aiAnalysis.Status.ApprovalRequestedAt != nil {
               duration := time.Since(aiAnalysis.Status.ApprovalRequestedAt.Time)
               aiAnalysis.Status.ApprovalDuration = duration.String()
           }

           log.Info("Approval decision captured",
               "approvedBy", aiAnalysis.Status.ApprovedBy,
               "duration", aiAnalysis.Status.ApprovalDuration)
       } else if approvalRequest.Spec.Decision == "Rejected" {
           aiAnalysis.Status.ApprovalStatus = "Rejected"
           aiAnalysis.Status.RejectedBy = approvalRequest.Spec.DecidedBy
           aiAnalysis.Status.ApprovalTime = &metav1.Time{Time: time.Now()}
           aiAnalysis.Status.RejectionReason = approvalRequest.Spec.Justification
           aiAnalysis.Status.ApprovalMethod = approvalRequest.Spec.DecisionMethod

           // Calculate duration
           if aiAnalysis.Status.ApprovalRequestedAt != nil {
               duration := time.Since(aiAnalysis.Status.ApprovalRequestedAt.Time)
               aiAnalysis.Status.ApprovalDuration = duration.String()
           }

           log.Info("Rejection decision captured",
               "rejectedBy", aiAnalysis.Status.RejectedBy,
               "reason", aiAnalysis.Status.RejectionReason)
       }

       return r.Status().Update(ctx, aiAnalysis)
   }
   ```

2. **Test Scenarios**:
   - Unit test: Verify approval metadata captured correctly
   - Unit test: Verify rejection metadata captured correctly
   - Unit test: Verify duration calculation accurate
   - Integration test: Verify metadata persists across reconcile cycles
   - E2E test: Verify audit trail completeness

**Effort**: Minimal (extend existing watch logic)

---

### **Update 4: Add to Edge Case Testing**

**Location**: `IMPLEMENTATION_PLAN_V1.0.md` - Edge Case Testing section (Day 11)

**Add to "Approval race conditions" edge case**:

```markdown
#### **Edge Case 4.2: Approval Context Incomplete**

**Scenario**: HolmesGPT returns response without alternatives or limited evidence

**Test**:
```go
It("should handle incomplete approval context gracefully", func() {
    // HolmesGPT response with minimal data
    holmesResponse := &HolmesGPTResponse{
        Confidence: 0.65,
        RootCause: "Memory leak detected",
        Evidence: []string{"Single evidence item"},
        Recommendations: []Recommendation{
            {Action: "increase_resources", Rationale: "Increase memory"},
        },
        Alternatives: nil, // No alternatives provided
    }

    // Should still populate approval context with available data
    Expect(aiAnalysis.Status.ApprovalContext).ToNot(BeNil())
    Expect(aiAnalysis.Status.ApprovalContext.EvidenceCollected).To(HaveLen(1))
    Expect(aiAnalysis.Status.ApprovalContext.AlternativesConsidered).To(BeEmpty())

    // Should include default "why approval required" message
    Expect(aiAnalysis.Status.ApprovalContext.WhyApprovalRequired).To(ContainSubstring("65"))
})
```

**Expected Behavior**:
- Populate available fields
- Use sensible defaults for missing fields
- Do NOT block approval workflow due to incomplete context
- Log warning about incomplete context for observability

---

#### **Edge Case 4.3: Approval Decision Method Tracking**

**Scenario**: Verify different approval methods captured correctly

**Test**:
```go
DescribeTable("should track approval method correctly",
    func(decisionMethod string) {
        approvalRequest.Spec.Decision = "Approved"
        approvalRequest.Spec.DecisionMethod = decisionMethod
        approvalRequest.Spec.DecidedBy = "ops-engineer@company.com"

        // Trigger sync
        Eventually(func() string {
            aiAnalysis := &aianalysisv1alpha1.AIAnalysis{}
            k8sClient.Get(ctx, aiAnalysisKey, aiAnalysis)
            return aiAnalysis.Status.ApprovalMethod
        }, timeout, interval).Should(Equal(decisionMethod))
    },
    Entry("kubectl approval", "kubectl"),
    Entry("dashboard approval", "dashboard"),
    Entry("slack button approval", "slack-button"),
    Entry("email link approval", "email-link"),
)
```

**Validation**: All 4 approval methods tracked correctly
```

---

### **Update 5: Version Bump & Changelog**

**Location**: `IMPLEMENTATION_PLAN_V1.0.md` - Version History section (top of file)

**Proposed Version**: **v1.0.5** - Approval Context & Decision Tracking Documentation

**Add to Version History**:

```markdown
- **v1.0.5** (2025-10-20): 📋 **Approval Context & Decision Tracking Documented**
  - **BR-AI-059**: AIAnalysis approval context capture (rich notifications)
    - Populate `status.approvalContext` with investigation summary, evidence, alternatives
    - Enable informed operator decisions via comprehensive context
    - Apply to Day 6.5 (new: Approval Context Population, +2h)
  - **BR-AI-060**: AIAnalysis approval decision tracking (audit trail)
    - Capture decision metadata: who, when, how, why
    - Enable compliance and UX improvement tracking
    - Apply to Day 6.6 (extend existing watch logic, minimal effort)
  - **Documentation**: [APPROVAL_CONTEXT_DOCUMENTATION_PLAN.md](./APPROVAL_CONTEXT_DOCUMENTATION_PLAN.md)
  - **Timeline**: Day 6 extended from 8h → 10h (total: 18-20 days)
  - **Confidence**: 95% (no change - straightforward extension)
  - **Expected Impact**: Approval timeout rate -50% (better operator context)
```

---

## 📊 **Impact Summary**

### **Timeline Impact**:
- **Day 6 Extended**: 8h → 10h (+2 hours for context population)
- **Total Timeline**: 18-19 days → **18-19 days** (within rounding)

### **Effort Breakdown**:
| Task | Effort | Notes |
|---|---|---|
| Day 6.5: Approval Context Population | 2h | Extract from HolmesGPT, validate |
| Day 6.6: Approval Decision Tracking | 0.5h | Extend existing watch logic |
| Edge Case Testing Updates | 0.5h | Add 2 new edge case scenarios |
| **Total** | **3h** | Absorbed in Day 6 extension |

### **Confidence**:
- **95%** - Straightforward extension of existing approval workflow
- No new external dependencies
- Leverages existing CRD watch patterns

---

## ✅ **Next Steps (When Ready for Implementation)**

1. **Review** this documentation plan
2. **Update** `IMPLEMENTATION_PLAN_V1.0.md` with above changes
3. **Bump version** to v1.0.5 with changelog
4. **Update** `api/aianalysis/v1alpha1/aianalysis_types.go` with new fields (already done in previous session)
5. **Implement** when AIAnalysis controller development begins

---

## 📚 **References**

- [ADR-018: Approval Notification V1.0 Integration](../../../../architecture/decisions/ADR-018-approval-notification-v1-integration.md)
- [BR-AI-059, BR-AI-060: Approval Context & Decision Tracking](../../../../requirements/APPROVAL_NOTIFICATION_BUSINESS_REQUIREMENTS.md)
- [AIAnalysis CRD Types](../../../../../../api/aianalysis/v1alpha1/aianalysis_types.go) - Fields already added
- [AIAnalysis Implementation Plan v1.0.4](./IMPLEMENTATION_PLAN_V1.0.md) - Base plan to extend


