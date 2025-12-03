# HolmesGPT-API Execution Responsibility Decisions

**Date**: December 1, 2025
**From**: HolmesGPT-API Team
**To**: WorkflowExecution Team, RemediationOrchestrator Team
**Status**: ✅ DECIDED

---

## Summary

This document captures key architectural decisions about HolmesGPT-API's responsibilities in the execution and recovery flow. These decisions clarify the boundary between HAPI (intelligence) and RO/WE (orchestration/execution).

---

## Decision 1: naturalLanguageSummary Consumption ✅

**Decision**: HolmesGPT-API will consume `naturalLanguageSummary` from WE for recovery prompts.

### Responsibility Chain

| Service | Responsibility |
|---------|----------------|
| **WorkflowExecution** | Generates `naturalLanguageSummary` from PipelineRun/TaskRun failure details |
| **RemediationOrchestrator** | Passes to recovery AIAnalysis via `PreviousExecutions[].NaturalLanguageSummary` |
| **HolmesGPT-API** | Includes in LLM recovery prompt for failure context |

### Data Flow

```
WE.Status.FailureDetails.NaturalLanguageSummary
    ↓
RO extracts and passes to recovery AIAnalysis
    ↓
AIAnalysis.Spec.PreviousExecutions[].NaturalLanguageSummary
    ↓
HolmesGPT-API includes in recovery prompt
    ↓
LLM analyzes failure and recommends alternative workflow
```

### Example Usage in Recovery Prompt

```markdown
## Previous Execution Attempt

The previous remediation attempt failed:

{previousExecutions[0].naturalLanguageSummary}

Please analyze this failure and select an alternative workflow that avoids this issue.
```

---

## Decision 2: No Retry in HAPI - RO Decides ✅

**Decision**: HolmesGPT-API does NOT implement retry logic. RO decides all retry/recovery actions.

### Rationale

- **Separation of concerns**: HAPI focuses on intelligence (RCA, workflow selection)
- **Single orchestration point**: RO owns the remediation lifecycle
- **Policy flexibility**: Retry policy can change without modifying HAPI

### Responsibility Split

| Service | Responsibility |
|---------|----------------|
| **HolmesGPT-API** | RCA + Workflow Selection + Report results |
| **RemediationOrchestrator** | Orchestration + Retry/Recovery decisions + Approval flow |
| **WorkflowExecution** | Execution + Status reporting |

### Failure Flow

```
┌─────────────────────────────────────────────────────────────────┐
│ WorkflowExecution fails                                         │
│ Reports: wasExecutionFailure, requiresManualReview, summary     │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ RemediationOrchestrator receives failure                        │
│                                                                 │
│ IF wasExecutionFailure == false (pre-execution):                │
│   → May trigger recovery AIAnalysis                             │
│   → HAPI analyzes and selects alternative workflow              │
│                                                                 │
│ IF wasExecutionFailure == true (during-execution):              │
│   → May create notification for manual review                   │
│   → HAPI provides analysis if asked, but RO decides action      │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ HolmesGPT-API (if recovery AIAnalysis triggered)                │
│                                                                 │
│ - Receives failure context (including naturalLanguageSummary)   │
│ - Performs recovery RCA                                         │
│ - Selects alternative workflow                                  │
│ - Reports recommendation to AIAnalysis status                   │
│                                                                 │
│ DOES NOT:                                                       │
│ - Decide whether to retry                                       │
│ - Trigger execution directly                                    │
│ - Implement retry backoff logic                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Impact on WE

- **No change required**: WE continues to report `wasExecutionFailure` and other failure details
- **Confirms design**: WE's `wasExecutionFailure` flag is used by RO (not HAPI) for retry decisions

### Impact on RO

- **Confirmed responsibility**: RO owns all retry/recovery decisions
- **Uses `wasExecutionFailure`**:
  - `false` → Safe to trigger recovery analysis
  - `true` → Requires manual review (state unknown)

---

## Decision 3: Flexible Parameter Format ✅

**Decision**: HolmesGPT-API does NOT enforce a specific parameter casing format. Workflow schema defines parameters.

### Rationale

- **Multi-runtime support**: Different engines have different conventions
  - Tekton: `UPPER_SNAKE_CASE`
  - Ansible: `lower_snake_case`
  - Future engines: TBD
- **Workflow author control**: Parameter schema is part of workflow definition
- **No transformation needed**: Pass-through from LLM → HAPI → RO → WE → Runtime

### Contract

```
┌─────────────────────────────────────────────────────────────────┐
│ Workflow Schema (in Data Storage)                               │
│ - Defines parameter names, types, descriptions                  │
│ - Parameter names follow runtime engine convention              │
│ - Example Tekton: { "name": "TARGET_NAMESPACE", "type": "string" }
│ - Example Ansible: { "name": "target_namespace", "type": "string" }
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ HolmesGPT-API                                                   │
│ - LLM reads parameter schema from workflow search results       │
│ - Populates values based on RCA                                 │
│ - Passes through as-is (no casing transformation)               │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ WorkflowExecution                                               │
│ - Receives parameters as-is                                     │
│ - Passes to PipelineRun (Tekton) or Playbook (Ansible)          │
│ - No transformation                                             │
└─────────────────────────────────────────────────────────────────┘
```

### V1.0 State

- All V1.0 workflows are **Tekton-based**
- Current workflows use `UPPER_SNAKE_CASE` by convention
- No enforcement in HAPI - just follows workflow schema

### Future Considerations

When adding Ansible or other runtimes:
- Workflow schema specifies parameter names in engine's convention
- HAPI passes through without transformation
- WE routes to appropriate runtime based on workflow metadata

---

## Action Items

| Item | Owner | Status |
|------|-------|--------|
| WE continues generating `naturalLanguageSummary` | WE Team | ✅ No change |
| RO passes `naturalLanguageSummary` to recovery AIAnalysis | RO Team | ✅ **Confirmed** (Dec 2, 2025) |
| RO owns retry/recovery decisions | RO Team | ✅ Confirmed |
| HAPI consumes `naturalLanguageSummary` in recovery prompts | HAPI Team | ⏳ Implementation |
| Workflow schemas define parameter format | Data Storage Team | ✅ No change |

---

## RO Team Confirmation (December 2, 2025)

**Confirmed**: RO will pass `naturalLanguageSummary` to recovery AIAnalysis via `PreviousExecutions[].NaturalLanguageSummary`.

**Implementation** (per DD-RO-001 and WE→RO-003 response):
```go
// When creating recovery AIAnalysis after pre-execution failure
aiAnalysis.Spec.PreviousExecutions = []PreviousExecution{
    {
        WorkflowID:            failedWE.Spec.WorkflowRef.WorkflowID,
        Phase:                 failedWE.Status.Phase,
        FailureReason:         failedWE.Status.FailureDetails.Reason,
        WasExecutionFailure:   failedWE.Status.FailureDetails.WasExecutionFailure,
        NaturalLanguageSummary: failedWE.Status.FailureDetails.NaturalLanguageSummary, // ✅ Passed
        CompletedAt:           failedWE.Status.CompletionTime,
    },
}
```

**Note**: Recovery AIAnalysis is only triggered for pre-execution failures (`wasExecutionFailure: false`). During-execution failures require manual review.

---

## Questions?

Please respond in this document or reach out to the HolmesGPT-API team.

---

**Document Version**: 1.0
**Last Updated**: December 1, 2025

