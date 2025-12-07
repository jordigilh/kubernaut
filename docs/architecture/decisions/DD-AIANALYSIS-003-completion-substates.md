# DD-AIANALYSIS-003: AIAnalysis Completion Substates

**Date**: 2025-12-06
**Status**: 📋 **PROPOSED** - Awaiting Review
**Deciders**: AIAnalysis Team, RemediationOrchestrator Team, Platform Team
**Version**: 1.0
**Related BR**: BR-HAPI-197, BR-ORCH-001, BR-ORCH-036

---

## Context & Problem

### Current Model (Problematic)

The AIAnalysis CRD status currently uses **two separate mechanisms** to indicate completion outcomes:

```yaml
# Current: Two separate fields that are mutually exclusive
status:
  phase: Completed       # OR Failed
  approvalRequired: true # Only meaningful if phase=Completed
  reason: WorkflowResolutionFailed  # Only meaningful if phase=Failed
  subReason: LowConfidence
```

### Problem Statement

This design has issues:

1. **Implicit Mutual Exclusivity**: `ApprovalRequired` and `WorkflowResolutionFailed` cannot both be true, but this isn't enforced by the schema
2. **Confusing for Consumers**: RO must check `Phase` first, then conditionally check other fields
3. **Schema Drift Risk**: Future additions could accidentally create invalid combinations
4. **Inconsistent Semantics**: `ApprovalRequired` is a boolean, but `WorkflowResolutionFailed` is encoded as `Reason`

### Actual Outcomes

In reality, AIAnalysis has **exactly 4 terminal outcomes**:

| Outcome | Current Encoding | Meaning |
|---------|------------------|---------|
| **Auto-Executable** | `Phase=Completed`, `ApprovalRequired=false` | High confidence, proceed automatically |
| **Approval Required** | `Phase=Completed`, `ApprovalRequired=true` | Medium confidence, needs human sign-off |
| **Workflow Resolution Failed** | `Phase=Failed`, `Reason=WorkflowResolutionFailed` | AI couldn't produce valid workflow |
| **Other Failure** | `Phase=Failed`, `Reason=<other>` | API error, timeout, etc. |

---

## Alternatives Considered

### Alternative 1: Keep Current Model (Separate Fields)

**Approach**: Document the mutual exclusivity rules and rely on code to enforce them.

**Pros**:
- ✅ No schema change required
- ✅ Already implemented
- ✅ No migration needed

**Cons**:
- ❌ Mutual exclusivity not enforced by schema
- ❌ Consumers must understand implicit rules
- ❌ Easy to create invalid states in future development

**Confidence**: 60% (current implementation, but architecturally fragile)

---

### Alternative 2: Unified CompletionReason Enum

**Approach**: Replace `ApprovalRequired` boolean and `Reason` string with a single `CompletionReason` enum.

```yaml
status:
  phase: Completed  # Still have phase for lifecycle
  completionReason: AutoExecutable | ApprovalRequired | WorkflowResolutionFailed
  # subReason only used when completionReason=WorkflowResolutionFailed
  subReason: WorkflowNotFound | LowConfidence | ...
  message: "Human-readable detail"
```

**Go Schema**:
```go
type AIAnalysisStatus struct {
    // Phase tracks lifecycle: Pending → Investigating → Analyzing → Completed
    // +kubebuilder:validation:Enum=Pending;Investigating;Analyzing;Completed
    Phase string `json:"phase"`

    // CompletionReason indicates the terminal outcome (only set when Phase=Completed)
    // +kubebuilder:validation:Enum=AutoExecutable;ApprovalRequired;WorkflowResolutionFailed;APIError;Timeout
    // +optional
    CompletionReason string `json:"completionReason,omitempty"`

    // SubReason provides granular detail for WorkflowResolutionFailed
    // +kubebuilder:validation:Enum=WorkflowNotFound;ImageMismatch;ParameterValidationFailed;NoMatchingWorkflows;LowConfidence;LLMParsingError
    // +optional
    SubReason string `json:"subReason,omitempty"`

    Message string `json:"message,omitempty"`
    // ... other fields unchanged
}
```

**Pros**:
- ✅ Explicit enum prevents invalid combinations
- ✅ Clearer semantics for consumers
- ✅ Schema enforces valid states
- ✅ Single field to check for outcome routing

**Cons**:
- ❌ Breaking schema change
- ❌ Requires migration for existing CRDs
- ❌ RO must update detection logic
- ❌ `Failed` phase removed - all terminal states are `Completed`

**Confidence**: 75% (cleaner architecture, but breaking change)

---

### Alternative 3: Keep Phase=Failed, Add OutcomeReason Enum

**Approach**: Keep `Failed` phase for error cases, but unify the "success with conditions" case.

```yaml
status:
  # Phase: Completed = success (may need approval), Failed = error/resolution failure
  phase: Completed | Failed

  # OutcomeReason: Only set when terminal
  # For Completed: AutoExecutable, ApprovalRequired
  # For Failed: WorkflowResolutionFailed, APIError, Timeout
  outcomeReason: AutoExecutable | ApprovalRequired | WorkflowResolutionFailed | APIError | Timeout

  # SubReason: Granular detail (for WorkflowResolutionFailed)
  subReason: WorkflowNotFound | LowConfidence | ...
```

**Go Schema**:
```go
type AIAnalysisStatus struct {
    // Phase: Completed for success outcomes, Failed for error outcomes
    // +kubebuilder:validation:Enum=Pending;Investigating;Analyzing;Completed;Failed
    Phase string `json:"phase"`

    // OutcomeReason indicates the specific terminal outcome
    // +kubebuilder:validation:Enum=AutoExecutable;ApprovalRequired;WorkflowResolutionFailed;APIError;Timeout;MaxRetriesExceeded
    // +optional
    OutcomeReason string `json:"outcomeReason,omitempty"`

    // SubReason provides granular detail for WorkflowResolutionFailed
    // +optional
    SubReason string `json:"subReason,omitempty"`

    // Deprecated: Use OutcomeReason instead
    // +optional
    ApprovalRequired bool `json:"approvalRequired,omitempty"`

    // Deprecated: Use OutcomeReason instead
    // +optional
    Reason string `json:"reason,omitempty"`
}
```

**Pros**:
- ✅ Backward compatible (deprecated fields still work)
- ✅ Clear distinction: `Completed` = AI succeeded, `Failed` = AI couldn't help
- ✅ Single `OutcomeReason` enum for routing
- ✅ Gradual migration path

**Cons**:
- ❌ Redundant fields during transition
- ❌ Schema has deprecated + new fields
- ❌ Validation must check consistency

**Confidence**: 85% (best balance of clarity and backward compatibility)

---

## Decision

**PROPOSED: Alternative 3** - Keep Phase=Failed, Add OutcomeReason Enum

### Rationale

1. **Backward Compatibility**: Existing consumers continue to work with `ApprovalRequired` and `Reason`
2. **Clear Semantics**: `OutcomeReason` enum makes all outcomes explicit
3. **Phase Preserves Intent**: `Completed` = AI produced a workflow, `Failed` = AI couldn't
4. **Gradual Migration**: Consumers can migrate to `OutcomeReason` at their own pace

### Key Insight

The fundamental question is: **"Did AI produce a usable workflow recommendation?"**

- **Completed + ApprovalRequired**: Yes, but needs human sign-off
- **Completed + AutoExecutable**: Yes, high confidence
- **Failed + WorkflowResolutionFailed**: No, AI couldn't produce one
- **Failed + APIError/Timeout**: Unknown, infrastructure failed

---

## Implementation

### Phase 1: Add OutcomeReason (Non-Breaking)

1. Add `OutcomeReason` field to CRD schema
2. Update AIAnalysis handlers to populate both old and new fields
3. Update documentation

```go
// InvestigatingHandler - when HAPI returns needs_human_review=true
status.Phase = aianalysis.PhaseFailed
status.Reason = "WorkflowResolutionFailed"           // Old (deprecated)
status.OutcomeReason = "WorkflowResolutionFailed"    // New
status.SubReason = mapEnumToSubReason(resp.HumanReviewReason)

// AnalyzingHandler - when approval required
status.Phase = aianalysis.PhaseCompleted
status.ApprovalRequired = true                       // Old (deprecated)
status.OutcomeReason = "ApprovalRequired"            // New
```

### Phase 2: Consumer Migration

1. RO migrates to check `OutcomeReason` instead of `Phase` + `ApprovalRequired`/`Reason`
2. Other consumers migrate
3. Update all documentation

### Phase 3: Deprecation (V2.0)

1. Remove deprecated `ApprovalRequired` and `Reason` fields
2. `OutcomeReason` becomes the sole indicator

---

## Consequences

### Positive

- ✅ Explicit enum prevents invalid state combinations
- ✅ Single field for outcome routing simplifies consumer logic
- ✅ Backward compatible migration path
- ✅ Schema-enforced valid states

### Negative

- ⚠️ Temporary schema bloat during transition (deprecated + new fields)
- ⚠️ Documentation must explain both patterns during transition
- **Mitigation**: Clear deprecation notices and migration guide

### Neutral

- 🔄 Requires coordination with RO team for migration timing
- 🔄 May influence Notification Service API design

---

## Validation

### Detection Logic (New)

```go
// RO detection with OutcomeReason (cleaner)
switch ai.Status.OutcomeReason {
case "AutoExecutable":
    return c.CreateWorkflowExecution(ctx, rr, ai)
case "ApprovalRequired":
    return c.CreateApprovalNotification(ctx, rr, ai)
case "WorkflowResolutionFailed":
    return c.CreateManualReviewNotification(ctx, rr, ai)
case "APIError", "Timeout", "MaxRetriesExceeded":
    return c.HandleInfrastructureFailure(ctx, rr, ai)
}
```

### Detection Logic (Current - for comparison)

```go
// Current: Multiple checks required
if ai.Status.Phase == "Failed" {
    if ai.Status.Reason == "WorkflowResolutionFailed" {
        return c.CreateManualReviewNotification(ctx, rr, ai)
    }
    return c.HandleOtherFailure(ctx, rr, ai)
}
if ai.Status.ApprovalRequired {
    return c.CreateApprovalNotification(ctx, rr, ai)
}
return c.CreateWorkflowExecution(ctx, rr, ai)
```

---

## Timeline

| Phase | Target | Status |
|-------|--------|--------|
| DD Approval | TBD | 📋 Proposed |
| Phase 1: Add OutcomeReason | V1.1 | ⏳ Pending |
| Phase 2: Consumer Migration | V1.2 | ⏳ Pending |
| Phase 3: Deprecation | V2.0 | ⏳ Pending |

---

## Related Documents

| Document | Relationship |
|----------|--------------|
| [BR-HAPI-197](../../requirements/BR-HAPI-197-needs-human-review-field.md) | Triggered this design discussion |
| [NOTICE_AIANALYSIS_WORKFLOW_RESOLUTION_FAILURE.md](../../handoff/NOTICE_AIANALYSIS_WORKFLOW_RESOLUTION_FAILURE.md) | Current RO coordination |
| [reconciliation-phases.md](../../services/crd-controllers/02-aianalysis/reconciliation-phases.md) | AIAnalysis phase definitions |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-06 | Initial proposal |


