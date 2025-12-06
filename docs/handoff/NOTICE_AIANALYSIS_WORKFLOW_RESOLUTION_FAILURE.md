# NOTICE: AIAnalysis Workflow Resolution Failure Status Contract

**Date**: 2025-12-06
**Version**: 1.2
**From**: AIAnalysis Team
**To**: RemediationOrchestrator Team
**CC**: Platform Team, Observability Team
**Status**: 📋 **INFORMATIONAL - ACTION REQUIRED BY RO**
**Related BR**: [BR-HAPI-197](../requirements/BR-HAPI-197-needs-human-review-field.md)

### Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.2 | 2025-12-06 | Q18/Q19 resolved: threshold ownership (AIAnalysis), retry matrix finalized, `validation_attempts_history` field |
| 1.1 | 2025-12-06 | Added DD-AIANALYSIS-003 proposal, pending HAPI questions (Q18, Q19), threshold clarification |
| 1.0 | 2025-12-06 | Initial notice |

---

## 📋 **Summary**

This document describes a **new failure mode** in the AIAnalysis CRD status when HolmesGPT-API returns `needs_human_review=true`.

**Purpose**: Inform RO team of the new AIAnalysis status contract so they can implement appropriate handling logic.

**RO Action Required**: Update RO reconciliation logic to detect and handle this new failure mode.

---

## 🔔 **What Changed**

### Before (Prior to BR-HAPI-197)

AIAnalysis could only fail with generic reasons:
- `APIError` - HolmesGPT-API returned an HTTP error
- `MaxRetriesExceeded` - Transient errors persisted
- `Timeout` - Investigation timed out

### After (BR-HAPI-197 Implementation)

AIAnalysis can now fail with **structured workflow resolution failures**:
- `WorkflowResolutionFailed` - HAPI returned `needs_human_review=true`
- With **granular `SubReason`** explaining the specific cause

---

## 📊 **AIAnalysis Status Contract**

### New Failure Mode: `WorkflowResolutionFailed`

When HolmesGPT-API returns `needs_human_review=true`, AIAnalysis will set:

```yaml
status:
  phase: Failed
  reason: WorkflowResolutionFailed
  subReason: <specific_cause>  # NEW FIELD
  message: "<human-readable warning from HAPI>"
  warnings:
    - "<warning 1 from HAPI>"
    - "<warning 2 from HAPI>"

  # Partial data preserved for operator context
  selectedWorkflow:       # May be present (invalid but preserved)
    workflowId: "..."
    confidence: 0.XX
    rationale: "..."
  rootCauseAnalysis:      # Always present
    summary: "..."
    severity: "..."
```

### SubReason Values

| SubReason | HAPI Trigger | Description |
|-----------|--------------|-------------|
| `WorkflowNotFound` | LLM hallucinated workflow | Selected `workflow_id` doesn't exist in catalog |
| `ImageMismatch` | LLM provided wrong image | Container image doesn't match catalog |
| `ParameterValidationFailed` | Invalid parameters | Parameters don't conform to workflow schema |
| `NoMatchingWorkflows` | No workflows available | Catalog search returned no results |
| `LowConfidence` | AI uncertainty | Confidence below 70% threshold |
| `LLMParsingError` | AI failure | Cannot parse LLM response |

---

## 🔄 **How to Detect This Failure Mode**

### Detection Logic

RO should watch for AIAnalysis resources and check for this specific failure pattern:

```go
// Detection: Check if AIAnalysis failed due to workflow resolution
if analysis.Status.Phase == "Failed" && analysis.Status.Reason == "WorkflowResolutionFailed" {
    // This is the new failure mode from BR-HAPI-197
    // SubReason contains the specific cause
    subReason := analysis.Status.SubReason

    // Handle according to RO's own logic
}
```

### Key Fields to Read

| Field | Type | Description |
|-------|------|-------------|
| `status.phase` | `string` | Will be `"Failed"` |
| `status.reason` | `string` | Will be `"WorkflowResolutionFailed"` |
| `status.subReason` | `string` | Specific cause (see table above) |
| `status.message` | `string` | Human-readable explanation |
| `status.warnings` | `[]string` | Array of warning messages from HAPI |
| `status.validationAttemptsHistory` | `[]ValidationAttempt` | **NEW**: Complete history of all 3 HAPI retry attempts |
| `status.selectedWorkflow` | `*SelectedWorkflow` | May be present (partial data for context) |
| `status.rootCauseAnalysis` | `*RootCauseAnalysis` | Always present (RCA completed) |

### `ValidationAttempt` Structure (NEW)

```go
type ValidationAttempt struct {
    Attempt    int       `json:"attempt"`     // 1, 2, or 3
    WorkflowID string    `json:"workflowId"`  // What LLM tried
    IsValid    bool      `json:"isValid"`     // Always false for failed attempts
    Errors     []string  `json:"errors"`      // Validation errors
    Timestamp  time.Time `json:"timestamp"`   // When attempt occurred
}
```

### Data Available for RO Decision-Making

When `WorkflowResolutionFailed` occurs, RO has access to:

1. **SubReason** - Categorizes the failure for routing/handling
2. **Message** - Human-readable detail for notifications
3. **RootCauseAnalysis** - Full RCA even though workflow selection failed
4. **SelectedWorkflow** (if present) - What AI attempted (may be invalid)
5. **Warnings** - Full array of HAPI warnings
6. **ValidationAttemptsHistory** - **NEW**: Complete audit trail of all 3 HAPI retry attempts (useful for operator notifications)

---

## 📈 **Metrics Coordination**

### AIAnalysis Metrics (Already Implemented)

```prometheus
# Counter for workflow resolution failures
aianalysis_failures_total{
  reason="WorkflowResolutionFailed",
  sub_reason="WorkflowNotFound|ImageMismatch|ParameterValidationFailed|NoMatchingWorkflows|LowConfidence|LLMParsingError"
}
```

### RO Metrics (Requested)

Please implement these metrics in RO:

```prometheus
# Counter for manual review notifications sent
ro_manual_review_notifications_total{
  sub_reason="WorkflowNotFound|ImageMismatch|..."
}

# Gauge for pending manual reviews
ro_pending_manual_reviews{
  sub_reason="WorkflowNotFound|ImageMismatch|..."
}
```

---

## 🔍 **Example Scenarios**

### Scenario 1: Workflow Not Found

**HAPI Response**:
```json
{
  "needs_human_review": true,
  "human_review_reason": "workflow_not_found",
  "warnings": ["Workflow 'restart-pod-v99' not found in catalog"],
  "selected_workflow": {
    "workflow_id": "restart-pod-v99",
    "confidence": 0.85
  }
}
```

**AIAnalysis Status**:
```yaml
status:
  phase: Failed
  reason: WorkflowResolutionFailed
  subReason: WorkflowNotFound
  message: "Workflow 'restart-pod-v99' not found in catalog"
  selectedWorkflow:
    workflowId: restart-pod-v99
    confidence: 0.85
```

**Expected RO Action**:
1. Create NotificationRequest with type `ManualReviewRequired`
2. Include the invalid workflow for operator context
3. Suggest checking workflow catalog

### Scenario 2: Low Confidence

**HAPI Response**:
```json
{
  "needs_human_review": true,
  "human_review_reason": "low_confidence",
  "warnings": ["Confidence (0.55) below threshold (0.70)"],
  "selected_workflow": {
    "workflow_id": "scale-deployment-v1",
    "confidence": 0.55
  }
}
```

**AIAnalysis Status**:
```yaml
status:
  phase: Failed
  reason: WorkflowResolutionFailed
  subReason: LowConfidence
  message: "Confidence (0.55) below threshold (0.70)"
  selectedWorkflow:
    workflowId: scale-deployment-v1
    confidence: 0.55
```

**Expected RO Action**:
1. Create NotificationRequest with type `ManualReviewRequired`
2. Include the workflow (valid but low confidence)
3. Offer manual approval option

### Scenario 3: No Matching Workflows

**HAPI Response**:
```json
{
  "needs_human_review": true,
  "human_review_reason": "no_matching_workflows",
  "warnings": ["No workflows in catalog match the incident type 'CustomResourceDegraded'"]
}
```

**AIAnalysis Status**:
```yaml
status:
  phase: Failed
  reason: WorkflowResolutionFailed
  subReason: NoMatchingWorkflows
  message: "No workflows in catalog match the incident type 'CustomResourceDegraded'"
  # selectedWorkflow is nil - nothing to show
```

**Expected RO Action**:
1. Create NotificationRequest with type `ManualReviewRequired`
2. Suggest adding workflows to catalog for this incident type
3. Offer manual remediation option

---

## ✅ **CRD Schema Reference**

### AIAnalysisStatus Fields Used

```go
type AIAnalysisStatus struct {
    // Phase: "Failed" for workflow resolution failures
    // +kubebuilder:validation:Enum=Pending;Investigating;Analyzing;Completed;Failed
    Phase   string `json:"phase"`

    // Reason: "WorkflowResolutionFailed" umbrella category
    Reason  string `json:"reason,omitempty"`

    // SubReason: Specific cause (NEW in v1.10)
    // +kubebuilder:validation:Enum=WorkflowNotFound;ImageMismatch;ParameterValidationFailed;NoMatchingWorkflows;LowConfidence;LLMParsingError;...
    SubReason string `json:"subReason,omitempty"`

    // Message: Human-readable detail from HAPI warnings
    Message string `json:"message,omitempty"`

    // Warnings: Array of warning messages from HAPI
    Warnings []string `json:"warnings,omitempty"`

    // SelectedWorkflow: May be present even on failure (for context)
    SelectedWorkflow *SelectedWorkflow `json:"selectedWorkflow,omitempty"`

    // RootCauseAnalysis: Always present (RCA completed before failure)
    RootCauseAnalysis *RootCauseAnalysis `json:"rootCauseAnalysis,omitempty"`
}
```

---

## ⚠️ **Important Considerations for RO**

### This is NOT an Error in the Traditional Sense

`WorkflowResolutionFailed` means:
- ✅ HolmesGPT-API was successfully called
- ✅ AI analysis was performed
- ✅ Root Cause Analysis was completed
- ❌ **BUT** the AI could not produce a reliable workflow recommendation

### AIAnalysis Will NOT Retry

AIAnalysis treats this as a **terminal failure**. The handler:
- Does NOT requeue
- Does NOT increment retry count
- Sets phase to `Failed` permanently

If RO wants to retry, it must create a **new** AIAnalysis CRD.

### Partial Data is Preserved

Even though the failure is terminal, AIAnalysis preserves:
- Full `rootCauseAnalysis` (the RCA work is valid)
- Partial `selectedWorkflow` (if HAPI provided one, even if invalid)
- All `warnings` from HAPI

This data is available for operator context in notifications.

---

## 🗓️ **Timeline**

| Milestone | Owner | Status |
|-----------|-------|--------|
| AIAnalysis implementation | AIAnalysis | ✅ **DONE** |
| Unit tests (11 new) | AIAnalysis | ✅ **DONE** |
| RO acknowledges notice | RO Team | ⏳ **PENDING** |
| RO implements handling | RO Team | ⏳ **PENDING** |
| Integration testing | Both | ⏳ **PENDING** |

---

## 📚 **References**

| Document | Purpose |
|----------|---------|
| [BR-HAPI-197](../requirements/BR-HAPI-197-needs-human-review-field.md) | Business requirement |
| [RESPONSE_AIANALYSIS_NEEDS_HUMAN_REVIEW.md](./RESPONSE_AIANALYSIS_NEEDS_HUMAN_REVIEW.md) | AIAnalysis → HAPI response |
| [ACK_AIANALYSIS_HAPI_HUMAN_REVIEW.md](./ACK_AIANALYSIS_HAPI_HUMAN_REVIEW.md) | Integration acknowledgment |
| [reconciliation-phases.md](../services/crd-controllers/02-aianalysis/reconciliation-phases.md) | AIAnalysis phase spec |

---

## 🔔 **Pending Design Decisions & Open Items** (Dec 6, 2025)

### DD-AIANALYSIS-003: Completion Substates (Proposed)

**Status**: 📋 **PROPOSED** - Awaiting Review

**Issue Identified**: `ApprovalRequired` and `WorkflowResolutionFailed` are mutually exclusive, but this is implicit (not enforced by schema).

**Proposed Solution**: Introduce `OutcomeReason` enum to make all terminal outcomes explicit:

```yaml
# Current (problematic - mutual exclusivity not enforced)
status:
  phase: Completed | Failed
  approvalRequired: true        # Only if phase=Completed
  reason: WorkflowResolutionFailed  # Only if phase=Failed

# Proposed (explicit substates)
status:
  phase: Completed | Failed
  outcomeReason: AutoExecutable | ApprovalRequired | WorkflowResolutionFailed | APIError | Timeout
  subReason: WorkflowNotFound | LowConfidence | ...  # Granular detail
```

**Reference**: [DD-AIANALYSIS-003](../architecture/decisions/DD-AIANALYSIS-003-completion-substates.md)

**RO Impact**:
- V1.0: Continue using `Phase` + `ApprovalRequired`/`Reason` (current model)
- V1.1+: Migrate to `OutcomeReason` enum (simpler, schema-enforced)

---

### ✅ HAPI Clarifications Resolved (Q18, Q19)

**Status**: ✅ **RESOLVED** (Dec 6, 2025)

---

#### Q18: Confidence Threshold Ownership

**Resolution**: Threshold is **AIAnalysis's responsibility**, not HAPI's.

| Component | Role |
|-----------|------|
| **HAPI** | Returns raw `confidence: 0.XX` (stateless, threshold-agnostic) |
| **AIAnalysis** | Applies threshold rules, owns business logic |

**V1.0**: AIAnalysis uses global 70% threshold:
```yaml
# AIAnalysis ConfigMap
confidence_thresholds:
  manual_review: 0.70  # Below 70% → WorkflowResolutionFailed + SubReason=LowConfidence
```

**V1.1**: Operator-tunable thresholds per context (new BR to be created).

---

#### Q19: In-Session Retry - RO Guidance Finalized

**Resolution**: **HAPI exhausts 3 in-session retries** before returning `needs_human_review=true`.

**RO Retry Decision Matrix** (AUTHORITATIVE):

| SubReason | External Retry? | Rationale |
|-----------|-----------------|-----------|
| `LLMParsingError` | ❌ **NO** | HAPI already tried 3x with same context - will fail again |
| `WorkflowNotFound` | ⚠️ **Conditional** | Only if catalog was updated after failure |
| `ImageMismatch` | ⚠️ **Conditional** | Only if catalog was corrected |
| `ParameterValidationFailed` | ❌ **NO** | Schema issue - won't self-resolve |
| `NoMatchingWorkflows` | ⚠️ **Conditional** | Only if new workflows added to catalog |
| `LowConfidence` | ❌ **NO** | LLM uncertainty - human judgment required |

**New Field for Operator Notifications**: `validation_attempts_history` provides complete audit trail of all 3 attempts with errors.

---

#### Threshold Correction

**✅ RESOLVED**: The threshold for `LowConfidence` is **70%** (owned by AIAnalysis, not HAPI).

- ≥80% → Auto-execute (`ApprovalRequired=false`)
- 70-79% → Approval required (`ApprovalRequired=true`)
- <70% → Manual review (`Phase=Failed`, `SubReason=LowConfidence`)

---

## ✅ **Acknowledgment Tracking**

| Team | Acknowledged | Date | Notes |
|------|--------------|------|-------|
| RemediationOrchestrator | ⏳ | 2025-12-06 | [Response](./RESPONSE_RO_ACKNOWLEDGES_WORKFLOW_RESOLUTION_FAILURE.md) - Q1,Q2,Q5 answered; Q3,Q4 blocked on HAPI Q18/Q19 |
| Platform | ⏳ | | |
| Observability | ⏳ | | |

---

## 📝 **Response Template**

Please acknowledge this notice:

```markdown
# RESPONSE: RO Team Acknowledges AIAnalysis Workflow Resolution Failure Notice

**Date**: YYYY-MM-DD
**From**: RemediationOrchestrator Team
**To**: AIAnalysis Team

## Acknowledgment

- [ ] We understand the new `WorkflowResolutionFailed` failure mode
- [ ] We understand the `SubReason` values and their meanings
- [ ] We will implement handling in RO

## Implementation Plan
| Milestone | Target Date |
|-----------|-------------|
| Design RO handling | |
| Implement RO changes | |
| Integration testing | |

## Questions for AIAnalysis Team (if any)
[Any clarifications needed]
```

---

**Issued By**: AIAnalysis Team
**Date**: 2025-12-06

