# BR-KA-200: Handling Inconclusive Investigations

**ID**: BR-KA-200
**Title**: Handling Inconclusive LLM Investigations and Self-Resolved Signals
**Category**: Kubernaut Agent (KA)
**Priority**: 🔴 P0 (V1.0 BLOCKER)
**Version**: V1.0
**Status**: ✅ Implemented (KA/AIAnalysis complete; see Implementation Status)
**Date**: December 7, 2025
**Last Updated**: 2026-08-01 (Renamed `BR-HAPI-200` → `BR-KA-200`, terminology corrected to KA, `HumanReviewReason` snippet corrected from Python to the current Go/OpenAPI enum — [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806))

---

## Business Context

### Problem Statement

In production Kubernetes environments, signals (alerts/events) can **self-resolve** before or during LLM investigation:

| Scenario | Example |
|----------|---------|
| **Pod Restart** | OOMKilled pod automatically restarts and becomes healthy |
| **Transient Network** | Network partition resolves before investigation |
| **Auto-scaling** | HPA scales up, resolving resource pressure |
| **Self-Healing** | Application recovers from temporary failure |
| **Race Condition** | Signal created, but issue resolves before KA processes it |

**Current Gap**: When the LLM investigates and finds **no reproducible problem**, there is:
1. No explicit `HumanReviewReason` for this case
2. No prompt guidance telling the LLM how to respond
3. No clear signal to downstream services (AIAnalysis, RO, Notification)

### Business Impact

| Impact | Description |
|--------|-------------|
| **Unnecessary Workflows** | LLM may recommend remediation for non-existent problems |
| **Operator Confusion** | No clear indication that problem self-resolved |
| **Audit Gap** | Cannot track "problem resolved before remediation" events |
| **Resource Waste** | Executing workflows against healthy resources |

### Business Value

| Benefit | Impact |
|---------|--------|
| **Safety** | Prevents unnecessary remediation of healthy resources |
| **Clarity** | Clear signal to operators: "Problem resolved, no action needed" |
| **Audit Trail** | Track self-resolution events for operational insights |
| **Efficiency** | Avoid wasting workflow execution resources |

---

## Requirements

### BR-KA-200.1: New HumanReviewReason Value

**MUST**: Add `investigation_inconclusive` to the `HumanReviewReason` enum.

**Implemented** as an `ogen`-generated OpenAPI enum (`pkg/agentclient/oas_json_gen.go`), not a hand-written Go type:

```go
// Generated from KA's OpenAPI spec (internal/kubernautagent/api/openapi.json)
const (
	HumanReviewReasonWorkflowNotFound            HumanReviewReason = "workflow_not_found"
	HumanReviewReasonImageMismatch                HumanReviewReason = "image_mismatch"
	HumanReviewReasonParameterValidationFailed    HumanReviewReason = "parameter_validation_failed"
	HumanReviewReasonNoMatchingWorkflows          HumanReviewReason = "no_matching_workflows"
	HumanReviewReasonLowConfidence                HumanReviewReason = "low_confidence"
	HumanReviewReasonLlmParsingError              HumanReviewReason = "llm_parsing_error"
	HumanReviewReasonInvestigationInconclusive    HumanReviewReason = "investigation_inconclusive"
	HumanReviewReasonRcaIncomplete                HumanReviewReason = "rca_incomplete"       // BR-KA-212
	HumanReviewReasonAlignmentCheckFailed         HumanReviewReason = "alignment_check_failed" // shadow-agent
	HumanReviewReasonOperatorEscalation           HumanReviewReason = "operator_escalation"
)
```

**Semantics**: LLM investigation did not yield conclusive results - could not determine root cause or current state.

**Important Distinction**:
- `INVESTIGATION_INCONCLUSIVE` = LLM **uncertain**, needs human judgment → `needs_human_review=true`
- Problem **confirmed resolved** = LLM **confident** no action needed → `needs_human_review=false`, `selected_workflow=null`

---

### BR-KA-200.2: LLM Prompt Guidance

**MUST**: The LLM investigation prompt SHALL include explicit guidance for this scenario:

```
IMPORTANT: If you cannot verify the reported problem exists (e.g., the resource
is now healthy, events show a past issue but current state is normal, or the
symptoms described in the signal cannot be reproduced), you MUST:

1. Set "needs_human_review": true
2. Set "human_review_reason": "investigation_inconclusive"
3. Provide a detailed "investigation_summary" explaining:
   - What you investigated
   - What the current state is
   - Why the problem could not be reproduced
4. Set "selected_workflow": null (no workflow needed)
5. Add a warning: "Problem could not be reproduced - resource appears healthy"

This is a VALID outcome - not all signals require remediation.
```

**MUST** (Issue #388): The prompt SHALL also include Outcome D guidance for benign alerts:

```
IMPORTANT: If your investigation determines the alert describes a BENIGN CONDITION
that does not warrant remediation or human review (e.g., orphaned PVCs from
completed batch jobs, completed Job artifacts, non-impactful resource drift), you MUST:

1. Set "actionable": false
2. Set "selected_workflow": null (no workflow needed)
3. Set confidence >= 0.7 (you are confident the alert is benign)
4. Provide root cause analysis describing the benign condition

The "actionable" field is SEPARATE from "investigation_outcome":
- "investigation_outcome": describes what happened (resolved, inconclusive)
- "actionable": describes whether the alert warrants action (true/false)

Outcome D (actionable: false) means: the condition IS STILL PRESENT but is HARMLESS.
This is distinct from Outcome A (resolved) where the problem WENT AWAY.
```

---

### BR-KA-200.3: Response Structure - Two Distinct Outcomes

#### Outcome A: Problem Confirmed Resolved (No Human Review)

**MUST**: When LLM **confidently** determines the problem is resolved, return:

```json
{
  "incident_id": "inc-123",
  "remediation_id": "rem-456",
  "investigation_summary": "Investigated OOMKilled signal for pod 'myapp-abc123'. Current status: Running (healthy). Pod events show OOMKilled at 10:15:00 UTC, but pod successfully restarted at 10:15:03 UTC. Current memory usage: 45% of limit. No remediation required.",
  "selected_workflow": null,
  "confidence": 0.92,
  "needs_human_review": false,
  "human_review_reason": null,
  "warnings": [
    "Problem self-resolved - no remediation required",
    "Pod automatically recovered from OOMKilled event"
  ],
  "target_in_owner_chain": true,
  "timestamp": "2025-12-07T10:20:00Z"
}
```

**Key**: `needs_human_review=false` because LLM is confident. This is a successful outcome.

#### Outcome B: Investigation Inconclusive (Human Review Required)

**MUST**: When LLM **cannot determine** root cause or current state, return:

```json
{
  "incident_id": "inc-123",
  "remediation_id": "rem-456",
  "investigation_summary": "Unable to determine root cause for reported OOMKilled signal. Pod status is ambiguous - events show multiple restarts but current state unclear. Metrics unavailable.",
  "selected_workflow": null,
  "confidence": 0.35,
  "needs_human_review": true,
  "human_review_reason": "investigation_inconclusive",
  "warnings": [
    "Investigation inconclusive - human review recommended",
    "Could not verify current resource state"
  ],
  "target_in_owner_chain": true,
  "timestamp": "2025-12-07T10:20:00Z"
}
```

**Key**: `needs_human_review=true` because LLM is uncertain. Requires human judgment.

---

### BR-KA-200.4: Confidence Semantics

**SHOULD**: The `confidence` field SHALL reflect the LLM's confidence in its assessment (not confidence in a workflow selection).

| Confidence | Meaning |
|------------|---------|
| 0.9-1.0 | Very confident problem is resolved |
| 0.7-0.9 | Reasonably confident, may warrant monitoring |
| < 0.7 | Uncertain - may need human verification |

---

### BR-KA-200.5: Audit Requirements

**MUST**: For both investigation outcomes, the audit trail SHALL capture:

| Field | Value |
|-------|-------|
| `event_type` | `aiagent.llm.response` |
| `outcome` | `resolved` or `inconclusive` |
| `investigation_summary` | Full summary from LLM |
| `original_signal_type` | From request |
| `current_resource_state` | From LLM investigation |

---

## Consumer Behavior Requirements

### BR-KA-200.6: AIAnalysis Handling

**MUST**: AIAnalysis SHALL handle both outcomes:

#### Decision Tree (Evaluation Order)

**CRITICAL**: The `needs_human_review` check MUST be evaluated **before** the "ProblemResolved" check.
Evaluating in the wrong order causes high-confidence inconclusive investigations to be misclassified as "ProblemResolved".

```
1. needs_human_review=true                              → WorkflowResolutionFailed (Layer 1)
2. !hasSelectedWorkflow && confidence >= 0.7
   && no inconclusive/no-match warning signals
   && (isResolved || !hasSubstantiveRCA)                → ProblemResolved (Outcome A)
3. !hasSelectedWorkflow && confidence >= 0.7
   && is_actionable=false && "alert not actionable"     → NotActionable (Outcome D, #388)
4. !hasSelectedWorkflow                                 → NoWorkflowTerminalFailure
5. hasSelectedWorkflow && confidence < 0.7              → LowConfidenceFailure
6. default (workflow selected, confidence >= 0.7)       → Proceed to Analyzing phase
```

**Defense-in-Depth (Layer 2)**: Even when `needs_human_review=false`, the "ProblemResolved" path
verifies that KA's warnings do not contain signals indicating an active problem. This catches
edge cases where the LLM incorrectly overrides `needs_human_review=false` but KA's
`result_parser` still appends diagnostic warnings from `investigation_outcome` processing.

Warning signals that block "ProblemResolved" classification:
- `"inconclusive"` - from KA when `investigation_outcome == "inconclusive"`
- `"no workflows matched"` - from KA when `selected_workflow` is null and outcome is not "resolved"
- `"human review recommended"` - general KA safety signal

#### Outcome A: Problem Resolved (`needs_human_review=false`, `human_review_reason=null`)

AIAnalysis detects this pattern: `needs_human_review=false` AND `selected_workflow=null` AND `confidence >= 0.7` AND no inconclusive/no-match warning signals

1. **NOT** create a WorkflowExecution CRD
2. Transition to `Completed` phase
3. Set `reason: WorkflowNotNeeded`
4. Set `subReason: ProblemResolved` (derived by AIAnalysis)

```yaml
status:
  phase: Completed
  reason: WorkflowNotNeeded
  subReason: ProblemResolved
  message: "Problem self-resolved. No remediation required."
```

#### Outcome B: Investigation Inconclusive (`needs_human_review=true`, `human_review_reason=investigation_inconclusive`)

1. **NOT** create a WorkflowExecution CRD
2. Transition to `Failed` phase
3. Set `reason: WorkflowResolutionFailed`
4. Set `subReason: InvestigationInconclusive` (from KA enum)

```yaml
status:
  phase: Failed
  reason: WorkflowResolutionFailed
  subReason: InvestigationInconclusive
  message: "Investigation inconclusive - human review recommended."
```

#### Outcome D: Alert Not Actionable (`is_actionable=false`, `needs_human_review=false`)

Added by Issue #388 Fix A. Labeled "Outcome D" to align with the LLM prompt contract
(Outcome C in the prompt is "Problem Identified, No Automated Remediation Available",
handled by the `NoWorkflowTerminalFailure` code path).

When KA returns `actionable: false` with confidence >= 0.7:

1. **NOT** create a WorkflowExecution CRD
2. Transition to `Completed` phase
3. Set `reason: WorkflowNotNeeded`
4. Set `subReason: NotActionable` (distinct from `ProblemResolved`)
5. Set `actionability: NotActionable` on the CRD status

```yaml
status:
  phase: Completed
  reason: WorkflowNotNeeded
  subReason: NotActionable
  actionability: NotActionable
  needsHumanReview: false
  message: "Alert not actionable. No remediation warranted."
```

**Distinction from Outcome A (ProblemResolved)**:
- **Outcome A**: The problem **existed but is no longer occurring** (transient condition that resolved)
- **Outcome D**: The condition **is still present but is harmless** (benign alert, no action needed)

**Examples**: Orphaned PVCs from completed batch jobs, completed Job artifacts, informational alerts describing expected states.

---

### BR-KA-200.7: Remediation Orchestrator Handling

**SHOULD**: When AIAnalysis completes with `reason: WorkflowNotNeeded`, RO SHALL:

1. Update RemediationRequest status to reflect no action needed
2. NOT create any workflow execution
3. Emit `kubernaut_remediationorchestrator_no_action_needed_total{reason="problem_resolved|investigation_inconclusive"}` metric

---

### BR-KA-200.8: Notification Handling

**SHOULD**: Operators MAY configure notification rules for self-resolved incidents:

| Notification Type | Use Case |
|-------------------|----------|
| **Skip** | Most self-resolutions don't need notification |
| **Informational** | "FYI: Incident auto-resolved" for audit |
| **Warning** | Pattern of repeated self-resolutions (flapping) |

---

## Detection Criteria

### BR-KA-200.9: When to Report Each Outcome

#### Outcome A: Report "Resolved" (High Confidence)

The LLM SHALL return `investigation_outcome: "resolved"` when **confident** (≥0.7):

| Condition | Example |
|-----------|---------|
| **Resource Healthy** | Pod status is `Running`, no error conditions |
| **Explicit Recovery** | Events show recovery after the issue |
| **Metrics Normal** | CPU/memory/disk returned to normal range |

#### Outcome D: Report "Not Actionable" (High Confidence, Benign) — Issue #388

The LLM SHALL return `actionable: false` when **confident** (>=0.7) that the alert is benign:

| Condition | Example |
|-----------|---------|
| **Orphaned Resources** | PVCs from completed batch jobs, not bound to any running pod |
| **Completed Job Artifacts** | Job resources still present after successful completion |
| **Non-Impactful Drift** | Configuration difference with no operational effect |
| **Informational Alerts** | Events describing expected states (e.g., scale-down events) |

**Key distinction**: The condition IS STILL PRESENT but is HARMLESS. This is different from Outcome A where the problem WENT AWAY.

#### Outcome B: Report "Inconclusive" (Low Confidence)

The LLM SHALL return `investigation_outcome: "inconclusive"` when **uncertain** (<0.5):

| Condition | Example |
|-----------|---------|
| **Ambiguous State** | Pod status unclear, conflicting events |
| **Missing Data** | Metrics/events unavailable |
| **Conflicting Signals** | Some indicators healthy, others not |

---

## Non-Requirements

### What This BR Does NOT Cover

1. **Flapping Detection** - Detecting patterns of repeated self-resolution (future BR)
2. **Signal Staleness Check** - Checking signal age before investigation (different layer)
3. **Automatic Suppression** - Auto-closing signals without investigation (not safe)

---

## Acceptance Criteria

### AC-1: HumanReviewReason Includes INVESTIGATION_INCONCLUSIVE

```gherkin
Given the HumanReviewReason enum
When I inspect the available values
Then "investigation_inconclusive" SHALL be a valid option
```

### AC-2: Outcome A - Problem Confidently Resolved

```gherkin
Given a signal for pod "myapp" with status "OOMKilled"
And the pod has since restarted and is now healthy
And the LLM is confident (≥0.7) the problem is resolved
When the LLM investigates via Kubernaut Agent (KA)
Then "needs_human_review" SHALL be false
And "human_review_reason" SHALL be null
And "selected_workflow" SHALL be null
And "investigation_summary" SHALL describe the current healthy state
And "confidence" SHALL be ≥0.7
```

### AC-3: Outcome B - Investigation Inconclusive

```gherkin
Given a signal for pod "myapp" with status "OOMKilled"
And the LLM cannot determine the current state
When the LLM investigates via Kubernaut Agent (KA)
Then "needs_human_review" SHALL be true
And "human_review_reason" SHALL be "investigation_inconclusive"
And "selected_workflow" SHALL be null
And "investigation_summary" SHALL explain the uncertainty
And "confidence" SHALL be <0.5
```

### AC-4: AIAnalysis Handles Resolved Outcome

```gherkin
Given Kubernaut Agent (KA) returns needs_human_review=false AND selected_workflow=null AND confidence≥0.7
When AIAnalysis processes the response
Then NO WorkflowExecution CRD SHALL be created
And AIAnalysis status.phase SHALL be "Completed"
And AIAnalysis status.reason SHALL be "WorkflowNotNeeded"
And AIAnalysis status.subReason SHALL be "ProblemResolved"
```

### AC-5: AIAnalysis Handles Inconclusive Outcome

```gherkin
Given Kubernaut Agent (KA) returns needs_human_review=true with human_review_reason="investigation_inconclusive"
When AIAnalysis processes the response
Then NO WorkflowExecution CRD SHALL be created
And AIAnalysis status.phase SHALL be "Failed"
And AIAnalysis status.reason SHALL be "WorkflowResolutionFailed"
And AIAnalysis status.subReason SHALL be "InvestigationInconclusive"
```

### AC-6: Audit Trail Captured

```gherkin
Given a signal investigation completes with either outcome
When Kubernaut Agent (KA) returns the result
Then an audit event SHALL be written with the investigation outcome
And the event SHALL include the LLM's investigation_summary
```

### AC-7: Alert Not Actionable - KA Response (#388)

```gherkin
Given the LLM determines an alert is benign (e.g., orphaned PVC from completed batch job)
And the LLM sets actionable=false with confidence >= 0.7
When Kubernaut Agent (KA) returns the result
Then "is_actionable" SHALL be false
And "needs_human_review" SHALL be false
And "selected_workflow" SHALL be null
And warnings SHALL include "Alert not actionable"
```

### AC-8: Alert Not Actionable - AIAnalysis Handling (#388)

```gherkin
Given Kubernaut Agent (KA) returns is_actionable=false AND confidence >= 0.7 AND "alert not actionable" warning
When AIAnalysis processes the response
Then NO WorkflowExecution CRD SHALL be created
And AIAnalysis status.phase SHALL be "Completed"
And AIAnalysis status.reason SHALL be "WorkflowNotNeeded"
And AIAnalysis status.subReason SHALL be "NotActionable"
And AIAnalysis status.actionability SHALL be "NotActionable"
And AIAnalysis status.needsHumanReview SHALL be false
```

---

## Test Coverage

| Test Category | Test Count | Coverage |
|---------------|------------|----------|
| Unit Tests (Kubernaut Agent (KA)) | 8 | Prompt, parsing, enum |
| Integration Tests | 4 | Mock LLM scenarios |
| E2E Tests | 2 | Full flow with mock LLM |

---

## Implementation Status

| Component | Status | Owner |
|-----------|--------|-------|
| HumanReviewReason Enum (`INVESTIGATION_INCONCLUSIVE`) | ✅ Complete | KA Team |
| LLM Prompt Update (investigation_outcome guidance) | ✅ Complete | KA Team |
| Response Parsing (handle "resolved" and "inconclusive") | ✅ Complete | KA Team |
| Unit Tests | ✅ Complete | KA Team |
| AIAnalysis Handler (BR-KA-200.6 decision tree + defense-in-depth) | ✅ Complete | AIAnalysis Team |
| #388 `actionable` field in prompt, parser, Pydantic model | ✅ Complete | KA Team |
| #388 `is_actionable` in OpenAPI spec + Go client regeneration | ✅ Complete | KA Team |
| #388 `Actionability` CRD field + NotActionable routing | ✅ Complete | AIAnalysis Team |
| #388 Unit Tests | ✅ Complete | Both |
| RO Handler (`AnalyzingCallbacks.HandleWorkflowNotNeeded`, `internal/controller/remediationorchestrator/analyzing_handler.go`) | ✅ Complete | RO Team |
| Notification Rules (BR-KA-200.8, operator-configurable) | 🟡 Not yet implemented as a dedicated rule set — no action currently means no notification by default | Notification Team |

---

## Related Documents

| Document | Relationship |
|----------|-------------|
| [BR-KA-197](BR-KA-197-needs-human-review-field.md) | Parent: needs_human_review field |
| [BR-AI-088](BR-AI-088-configurable-confidence-thresholds.md) | **Different gate, not a duplicate**: BR-AI-088 is AIAnalysis's downstream, operator-configurable approval threshold applied to an *already-selected* workflow's `confidence`. This BR's 0.5/0.7 bands are KA's own upstream, hardcoded, LLM-self-assessed investigation-quality gate that runs before any workflow is selected. |
| [DD-KA-001](../architecture/decisions/DD-KA-001-workflow-response-validation-architecture.md) | Design: Validation architecture (supersedes the retired DD-HAPI-002) |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-07 | Initial business requirement |
| 1.1 | 2025-12-07 | Aligned with authoritative implementation: replaced `problem_not_reproducible` with `investigation_inconclusive`, clarified two distinct outcomes (Resolved vs Inconclusive) |
| 1.2 | 2026-02-09 | BR-KA-200.6: Documented corrected decision tree evaluation order (needs_human_review BEFORE ProblemResolved), added defense-in-depth via warnings-based check. Fixed misclassification bug where high-confidence inconclusive investigations were routed to ProblemResolved. AIAnalysis Handler marked complete. |
| 1.3 | 2026-03-02 | Issue #388 Fix A: Added Outcome D (Alert Not Actionable) with new `actionable` boolean field, `is_actionable` in IncidentResponse, `Actionability` CRD enum field, `NotActionable` SubReason. Updated decision tree (step 3). Added AC-7, AC-8. Relabeled from "Outcome C" to "Outcome D" to align with prompt contract (Outcome C = No Automated Remediation). |
| 1.4 | 2026-08-01 | Renamed `BR-HAPI-200` → `BR-KA-200`, `HolmesGPT-API`/`HAPI` → `Kubernaut Agent (KA)`/`KA` ([Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806)). Replaced the Python `HumanReviewReason` enum snippet with the current Go/`ogen`-generated enum. Verified the decision tree, warnings-based defense-in-depth, and AIAnalysis routing logic against `pkg/aianalysis/handlers/response_processor.go` — unchanged, still accurate. Confirmed RO Handler is implemented (`AnalyzingCallbacks.HandleWorkflowNotNeeded`); corrected Notification Rules status to reflect it is not yet implemented as a dedicated rule set. Removed a dead link to a non-existent `docs/handoff/` notice. |
| 1.5 | 2026-08-01 | Added a Related Documents cross-reference to BR-AI-088 clarifying that BR-AI-088's operator-configurable approval threshold and this BR's hardcoded 0.5/0.7 investigation-outcome bands are two distinct, non-overlapping gates (Issue #1806 follow-up). |

---

**Maintained By**: Kubernaut Agent (KA) Team

