# RESPONSE: RO Team Acknowledges AIAnalysis Workflow Resolution Failure Notice

**Date**: 2025-12-06
**From**: RemediationOrchestrator Team
**To**: AIAnalysis Team
**CC**: Platform Team, Observability Team
**In Response To**: [NOTICE_AIANALYSIS_WORKFLOW_RESOLUTION_FAILURE.md](./NOTICE_AIANALYSIS_WORKFLOW_RESOLUTION_FAILURE.md)

---

## 📬 **AIAnalysis Team Response** (2025-12-07)

**Status**: ✅ **ALL QUESTIONS RESOLVED**

Thank you for the thorough analysis. All questions have been answered, including HAPI team clarifications (Q18, Q19).

### ✅ HAPI Dependencies Resolved (2025-12-07)

| HAPI Question | Topic | Resolution | Reference |
|---------------|-------|------------|-----------|
| **Q18** | Confidence threshold | **70%** - AIAnalysis owns threshold (HAPI is stateless) | [AIANALYSIS_TO_HOLMESGPT_API_TEAM.md](./AIANALYSIS_TO_HOLMESGPT_API_TEAM.md) |
| **Q19** | In-session retry | HAPI exhausts 3 retries → **RO should NOT retry** `LLMParsingError` | [AIANALYSIS_TO_HOLMESGPT_API_TEAM.md](./AIANALYSIS_TO_HOLMESGPT_API_TEAM.md) |

### 🆕 BR-HAPI-200 Update (2025-12-07)

**New outcomes added** - RO must also handle:

| Outcome | AIAnalysis Status | RO Action | BR |
|---------|-------------------|-----------|-----|
| **Problem Resolved** | `Phase=Completed`, `Reason=WorkflowNotNeeded`, `SubReason=ProblemResolved` | Skip WE, mark RR `Completed` | BR-ORCH-037 |
| **Investigation Inconclusive** | `Phase=Failed`, `Reason=WorkflowResolutionFailed`, `SubReason=InvestigationInconclusive` | Create `manual-review` notification | BR-ORCH-036 v2.0 |

**Reference**: [NOTICE_INVESTIGATION_INCONCLUSIVE_BR_HAPI_200.md](./NOTICE_INVESTIGATION_INCONCLUSIVE_BR_HAPI_200.md)

**RO Action**: Implementation can proceed - all blockers resolved.

---

## ✅ Acknowledgment

| Item | Status |
|------|--------|
| We understand the new `WorkflowResolutionFailed` failure mode | ✅ |
| We understand the `SubReason` values and their meanings | ✅ |
| We will implement handling in RO | ✅ |
| We understand AIAnalysis will NOT retry (terminal failure) | ✅ |
| We understand partial data is preserved for context | ✅ |

---

## 📊 Impact Analysis on RO Implementation

### Day 4 Impact (NotificationCreator)

The notice introduces a **new notification scenario** distinct from what we planned:

| Scenario | BR | Trigger | Notification Type | Description |
|----------|------|---------|-------------------|-------------|
| **Approval Required** | BR-ORCH-001 | `AIAnalysis.Status.ApprovalRequired=true` | `approval` | AI found workflow, confidence 60-79%, operator decides |
| **Manual Review Required** | **NEW** | `AIAnalysis.Status.Phase=Failed` + `Reason=WorkflowResolutionFailed` | `manual-review` | AI couldn't produce valid workflow, human investigates |

### Key Distinction

```
┌────────────────────────────────────────────────────────────────────┐
│  APPROVAL (BR-ORCH-001)                                           │
│  ─────────────────────                                            │
│  • AI DID find a workflow                                         │
│  • Confidence is borderline (60-79%)                              │
│  • Operator APPROVES or REJECTS the AI's recommendation           │
│  • If approved → proceed to WorkflowExecution                     │
│                                                                   │
│  MANUAL REVIEW (NEW - per this notice)                            │
│  ─────────────────────────────────────                            │
│  • AI FAILED to find/validate a workflow                          │
│  • SubReason explains why (WorkflowNotFound, LowConfidence, etc.) │
│  • RCA data is available for context                              │
│  • Operator must INVESTIGATE and DECIDE next steps                │
│  • NO automatic path to WorkflowExecution                         │
└────────────────────────────────────────────────────────────────────┘
```

---

## ❓ Questions for AIAnalysis Team

### Q1: NotificationType Enum Value

The notice references `type ManualReviewRequired` for notifications, but our NotificationRequest API has:

```go
type NotificationType string
const (
    NotificationTypeEscalation   NotificationType = "escalation"
    NotificationTypeSimple       NotificationType = "simple"
    NotificationTypeStatusUpdate NotificationType = "status-update"
    NotificationTypeApproval     NotificationType = "approval"  // Just added for BR-ORCH-001
)
```

**Question**: Should we add `NotificationTypeManualReview` as a distinct enum value, or does `approval` cover both scenarios?

**Our Recommendation**: Add `NotificationTypeManualReview` because:
- Semantically different: "approve my recommendation" vs "I have no recommendation, help"
- Different operator workflows
- Different notification content/actions
- Clearer metrics tracking

**Awaiting your input before proceeding.**

> ### ✅ **AIAnalysis Team Answer (Q1)**
>
> **Approved: Add `NotificationTypeManualReview`**
>
> Your analysis is correct. These are semantically distinct scenarios:
>
> | Type | Scenario | Operator Action | Has Workflow? |
> |------|----------|-----------------|---------------|
> | `approval` | AI confident but needs human sign-off | Approve/Reject | ✅ Yes |
> | `manual-review` | AI failed to produce recommendation | Investigate, decide | ❌ No (or invalid) |
>
> Please add:
> ```go
> NotificationTypeManualReview NotificationType = "manual-review"
> ```
>
> **Note**: We'll also need to notify the Notification Service team via a shared document (similar to what RO did for `approval`).

---

### Q2: Relationship Between ApprovalRequired and WorkflowResolutionFailed

Can both conditions be true simultaneously?

| Status | ApprovalRequired | Phase | Reason | Expected? |
|--------|------------------|-------|--------|-----------|
| Case A | `true` | `Completed` | - | ✅ Normal approval flow |
| Case B | `false` | `Failed` | `WorkflowResolutionFailed` | ✅ New failure mode |
| Case C | `true` | `Failed` | `WorkflowResolutionFailed` | ❓ **Possible?** |

**Question**: If `ApprovalRequired=true` but then HAPI returns `needs_human_review=true`, what happens?

- Does AIAnalysis set `Phase=Failed` overriding `ApprovalRequired`?
- Or can we have `ApprovalRequired=true` AND `Phase=Failed`?

**Our Assumption**: `Phase=Failed` takes precedence, meaning RO should check `Phase` first.

> ### ✅ **AIAnalysis Team Answer (Q2)**
>
> **Your assumption is correct: `Phase=Failed` takes precedence.**
>
> The conditions are **mutually exclusive** in the state machine:
>
> ```
> HAPI Response Processing:
>
> 1. Check needs_human_review FIRST
>    └─ If true → Phase=Failed, Reason=WorkflowResolutionFailed
>                 ApprovalRequired is NOT set (irrelevant)
>
> 2. If needs_human_review=false, check confidence
>    └─ If 60-79% → Phase=Completed, ApprovalRequired=true
>    └─ If ≥80%   → Phase=Completed, ApprovalRequired=false
> ```
>
> **Case C is impossible** in our implementation. `ApprovalRequired` is only evaluated AFTER `needs_human_review` check passes.
>
> **RO Detection Logic** (confirmed):
> ```go
> // Check failure FIRST
> if ai.Status.Phase == "Failed" {
>     if ai.Status.Reason == "WorkflowResolutionFailed" {
>         // Handle manual review scenario
>         return c.CreateManualReviewNotification(ctx, rr, ai)
>     }
>     // Handle other failures (APIError, Timeout, etc.)
>     return c.handleOtherFailure(ctx, rr, ai)
> }
>
> // Only check approval if Phase=Completed
> if ai.Status.ApprovalRequired {
>     return c.CreateApprovalNotification(ctx, rr, ai)
> }
>
> // Auto-execute (high confidence)
> return c.CreateWorkflowExecution(ctx, rr, ai)
> ```

---

### Q3: SubReason = "LowConfidence" vs ApprovalRequired

The notice lists `LowConfidence` as a `SubReason` for `WorkflowResolutionFailed`:

> `LowConfidence` | AI uncertainty | Confidence below 70% threshold

But `ApprovalRequired` is also triggered by low confidence (60-79% per BR-ORCH-001).

**Question**: What's the threshold difference?

| Confidence | Outcome |
|------------|---------|
| 80-100% | Auto-execute (no approval needed) |
| 60-79% | `ApprovalRequired=true`, operator approves, then execute |
| **Below 60%?** | `WorkflowResolutionFailed` + `SubReason=LowConfidence`? |

**Our Assumption**:
- 60-79% → Approval flow
- Below 60% → Manual review (too uncertain to even offer for approval)

Please confirm.

> ### ✅ **AIAnalysis Team Answer (Q3)** - RESOLVED (2025-12-07)
>
> **HAPI Q18 Resolved**: The threshold is **70%**, owned by **AIAnalysis** (not HAPI).
>
> **Key Clarification** (per [AIANALYSIS_TO_HOLMESGPT_API_TEAM.md](./AIANALYSIS_TO_HOLMESGPT_API_TEAM.md) Q18):
> - **HAPI is stateless and threshold-agnostic** - returns raw `confidence: 0.XX`
> - **AIAnalysis owns threshold logic** - applies rules to determine status
>
> **✅ Confirmed Decision Tree**:
>
> | Confidence | AIAnalysis Status | RO Action |
> |------------|-------------------|-----------|
> | ≥80% | `Phase=Completed`, `ApprovalRequired=false` | Create WorkflowExecution |
> | **70-79%** | `Phase=Completed`, `ApprovalRequired=true` | Create Approval Notification |
> | **<70%** | `Phase=Failed`, `SubReason=LowConfidence` | Create ManualReview Notification |
>
> **V1.0**: AIAnalysis uses global 70% threshold
> **V1.1**: Operator-tunable thresholds per context (new BR to be created)
>
> **Key Distinction** (confirmed):
> - **Approval flow (70-79%)**: AI is "somewhat confident" - workflow is valid, just needs human sign-off
> - **Manual review (<70%)**: AI is "not confident enough to recommend" - human must investigate

---

### Q4: Retry Strategy Ownership

The notice states:

> If RO wants to retry, it must create a **new** AIAnalysis CRD.

**Questions**:
1. Should RO automatically retry for transient SubReasons (e.g., `LLMParsingError`)?
2. For `WorkflowNotFound` or `NoMatchingWorkflows`, should we just notify (no automatic retry)?
3. Should retry create a new `AIAnalysis` CRD under the same `RemediationRequest`, or a completely new remediation flow?

**Our Proposed Approach**:

| SubReason | Auto-Retry? | Notification | Notes |
|-----------|-------------|--------------|-------|
| `LLMParsingError` | ✅ Once | If retry fails | Transient |
| `WorkflowNotFound` | ❌ | ✅ Immediately | Catalog issue |
| `ImageMismatch` | ❌ | ✅ Immediately | Catalog issue |
| `ParameterValidationFailed` | ❌ | ✅ Immediately | Schema issue |
| `NoMatchingWorkflows` | ❌ | ✅ Immediately | Catalog gap |
| `LowConfidence` | ❌ | ✅ Immediately | AI uncertainty |

**Awaiting confirmation before implementing.**

> ### ✅ **AIAnalysis Team Answer (Q4)** - RESOLVED (2025-12-07)
>
> **HAPI Q19 Resolved**: HAPI **does in-session retries** (3 attempts with LLM self-correction).
>
> **Key Clarification** (per [AIANALYSIS_TO_HOLMESGPT_API_TEAM.md](./AIANALYSIS_TO_HOLMESGPT_API_TEAM.md) Q19):
> - If `LLMParsingError` reaches AIAnalysis, HAPI has **already exhausted 3 retries**
> - `validation_attempts_history` field provides audit trail of all attempts
> - **RO should NOT retry** `LLMParsingError` - retries are exhausted
>
> **✅ Confirmed Retry Strategy (ALL SubReasons)**:
>
> | SubReason | Auto-Retry? | Notification | Rationale |
> |-----------|-------------|--------------|-----------|
> | `LLMParsingError` | ❌ **NO** | ✅ Immediate | HAPI exhausted 3 retries |
> | `WorkflowNotFound` | ❌ | ✅ Immediate | Catalog issue - won't resolve without human action |
> | `ImageMismatch` | ❌ | ✅ Immediate | Catalog issue - won't resolve without human action |
> | `ParameterValidationFailed` | ❌ | ✅ Immediate | Schema issue - won't resolve without human action |
> | `NoMatchingWorkflows` | ❌ | ✅ Immediate | Catalog gap - won't resolve without human action |
> | `LowConfidence` | ❌ | ✅ Immediate | AI uncertainty - retry unlikely to help |
> | `InvestigationInconclusive` | ❌ | ✅ Immediate | **NEW (BR-HAPI-200)** - LLM couldn't determine state |
>
> **V1.0 Implementation** (simplified - no retry logic needed):
>
> ```go
> // V1.0: Simple approach - always notify for WorkflowResolutionFailed
> if ai.Status.Phase == "Failed" && ai.Status.Reason == "WorkflowResolutionFailed" {
>     return c.CreateManualReviewNotification(ctx, rr, ai)
> }
>
> // NEW (BR-HAPI-200): Handle WorkflowNotNeeded (problem self-resolved)
> if ai.Status.Phase == "Completed" && ai.Status.Reason == "WorkflowNotNeeded" {
>     // No notification needed by default (agreed with Notification team)
>     return c.MarkRemediationCompleted(ctx, rr, "Problem self-resolved")
> }
> ```

---

### Q5: Metrics Naming Convention

The notice requests these metrics:

```prometheus
ro_manual_review_notifications_total{sub_reason="..."}
ro_pending_manual_reviews{sub_reason="..."}
```

**Question**: Per [DD-005 Metrics Naming Compliance](./NOTICE_DD005_METRICS_NAMING_COMPLIANCE.md), should these be:

```prometheus
# Option A (notice format)
ro_manual_review_notifications_total

# Option B (DD-005 compliant)
kubernaut_remediationorchestrator_manual_review_notifications_total
```

**Our Preference**: Option B for consistency with DD-005.

> ### ✅ **AIAnalysis Team Answer (Q5)**
>
> **Approved: Option B (DD-005 compliant)**
>
> The metrics in the notice were simplified examples. Please use DD-005 compliant naming:
>
> ```prometheus
> # Counter for manual review notifications sent
> kubernaut_remediationorchestrator_manual_review_notifications_total{
>   sub_reason="WorkflowNotFound|ImageMismatch|ParameterValidationFailed|NoMatchingWorkflows|LowConfidence|LLMParsingError",
>   namespace="<rr_namespace>"
> }
>
> # Gauge for pending manual reviews (optional - nice to have)
> kubernaut_remediationorchestrator_pending_manual_reviews{
>   sub_reason="...",
>   namespace="<rr_namespace>"
> }
> ```
>
> **Note**: The gauge metric (`pending_manual_reviews`) is **optional for V1.0**. The counter is sufficient for initial observability. The gauge requires tracking state transitions which adds complexity.

---

## 📋 Implementation Plan Update

### Additional Day 4 Work (Confirmed)

| Task | BR | Priority | Status |
|------|------|----------|--------|
| Detect `WorkflowResolutionFailed` in AIAnalysis status | BR-ORCH-036 | P0 | ✅ Approved |
| Add `NotificationTypeManualReview` to API | BR-ORCH-036 | P0 | ✅ Approved |
| Create `CreateManualReviewNotification` method | BR-ORCH-036 | P0 | ✅ Approved |
| Implement retry logic for `LLMParsingError` | BR-ORCH-036 | P2 | ⏳ Deferred to V1.1 |
| Add DD-005 compliant metrics | BR-ORCH-036 | P1 | ✅ Approved |

### New Business Requirement: BR-ORCH-036

**BR-ORCH-036: Handle AIAnalysis WorkflowResolutionFailed**

**Acceptance Criteria** (Confirmed):
1. ✅ RO detects `WorkflowResolutionFailed` failure mode (check `Phase=Failed` first)
2. ✅ RO creates `NotificationRequest` with `Type=manual-review`
3. ⏳ RO retries once for `LLMParsingError` (V1.1 - optional for V1.0)
4. ✅ RO tracks metrics: `kubernaut_remediationorchestrator_manual_review_notifications_total{sub_reason=...}`
5. ✅ RO preserves partial data (RCA, invalid workflow if available) in notification `Metadata`

**V1.0 Scope**:
- Detect failure mode and create notification ✅
- Metrics counter ✅
- Retry logic ⏳ (deferred)
- Gauge metric ⏳ (deferred)

---

## 🗓️ Updated Timeline

| Milestone | Target | Status | Notes |
|-----------|--------|--------|-------|
| Acknowledge notice | 2025-12-06 | ✅ **DONE** | - |
| Receive answers to questions | 2025-12-07 | ✅ **DONE** | All blockers resolved |
| Create BR-ORCH-036 v2.0 | 2025-12-07 | ✅ **DONE** | Updated for BR-HAPI-200 |
| Create BR-ORCH-037 | 2025-12-07 | ✅ **DONE** | Handle `WorkflowNotNeeded` |
| Add `NotificationTypeManualReview` to API | Day 4 | ⏳ Ready | No blockers |
| Notify Notification Service team | Day 4 | ⏳ Ready | No blockers |
| Implement NotificationCreator updates | Day 4 | ⏳ Ready | No blockers |
| Add `InvestigationInconclusive` SubReason handling | Day 4 | ⏳ Ready | BR-HAPI-200 |
| Add `WorkflowNotNeeded` handling | Day 7 | ⏳ Ready | BR-ORCH-037 |
| Integration testing | Day 14-15 | ⏳ | - |

### V1.0 Scope (No Blockers)

| Task | BR | Status |
|------|-----|--------|
| Create BR-ORCH-036 v2.0 (manual review) | BR-ORCH-036 | ✅ Created |
| Create BR-ORCH-037 (workflow not needed) | BR-ORCH-037 | ✅ Created |
| Add `NotificationTypeManualReview` | BR-ORCH-036 | ⏳ Ready |
| Handle `InvestigationInconclusive` SubReason | BR-HAPI-200 | ⏳ Ready |
| Handle `WorkflowNotNeeded` outcome | BR-ORCH-037 | ⏳ Ready |
| DD-005 compliant metrics | BR-ORCH-036 | ⏳ Ready |
| No retry logic needed (HAPI exhausts retries) | - | ✅ Confirmed |

---

## 📝 Summary

We acknowledge and understand the `WorkflowResolutionFailed` failure mode and new BR-HAPI-200 outcomes.

### ✅ All Questions Resolved

| Question | Status | Answer |
|----------|--------|--------|
| Q1: NotificationType enum | ✅ | Add `NotificationTypeManualReview` |
| Q2: Condition precedence | ✅ | `Phase=Failed` checked first, mutually exclusive |
| Q3: Confidence thresholds | ✅ | **70%** threshold (AIAnalysis owns, HAPI stateless) |
| Q4: Retry strategy | ✅ | **No retry** - HAPI exhausts 3 retries before returning failure |
| Q5: Metrics naming | ✅ | DD-005 compliant: `kubernaut_remediationorchestrator_*` |

### 🆕 BR-HAPI-200 Updates

| New Outcome | RO Action | BR |
|-------------|-----------|-----|
| `WorkflowNotNeeded` + `ProblemResolved` | Skip WE, mark RR Completed (no notification) | BR-ORCH-037 |
| `WorkflowResolutionFailed` + `InvestigationInconclusive` | Create ManualReview notification | BR-ORCH-036 v2.0 |

### V1.0 Implementation Ready

**All blockers resolved** - RO can proceed with full V1.0 implementation:

- ✅ Add `NotificationTypeManualReview` enum (Q1)
- ✅ Detect `WorkflowResolutionFailed` (Q2)
- ✅ Handle `InvestigationInconclusive` SubReason (BR-HAPI-200)
- ✅ Handle `WorkflowNotNeeded` outcome (BR-ORCH-037)
- ✅ Use 70% threshold (Q3)
- ✅ No retry logic needed - HAPI exhausts retries (Q4)
- ✅ DD-005 compliant metrics (Q5)

**No items deferred to V1.1** - retry logic confirmed unnecessary.

---

**Issued By**: RemediationOrchestrator Team
**Date**: 2025-12-06

**AIAnalysis Team Response**: 2025-12-07 - All questions answered ✅

---

## 📝 Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| v1.0 | 2025-12-06 | RO Team | Initial acknowledgment |
| v1.1 | 2025-12-06 | AIAnalysis Team | Answered Q1, Q2, Q5 |
| v2.0 | 2025-12-07 | AIAnalysis Team | Resolved Q3, Q4 per HAPI Q18/Q19 |
| v2.1 | 2025-12-07 | AIAnalysis Team | Added BR-HAPI-200 updates (InvestigationInconclusive, WorkflowNotNeeded) |

