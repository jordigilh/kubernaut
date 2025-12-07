# NOTICE: New Investigation Outcome Types (BR-HAPI-200)

**Date**: December 7, 2025
**From**: HolmesGPT-API Team
**To**: AIAnalysis Team, Remediation Orchestrator Team, Notification Team
**Priority**: 🟠 HIGH (V1.0 - Per AIAnalysis scope clarification)
**Status**: ✅ ALL TEAMS ACKNOWLEDGED (Clarification pending from HAPI)

---

## 📋 Summary

BR-HAPI-200 introduces two distinct investigation outcomes when the LLM cannot recommend a workflow:

| Outcome | Confidence | `needs_human_review` | `human_review_reason` | Action |
|---------|------------|---------------------|----------------------|--------|
| **A: Resolved** | High (≥0.7) | `false` | `null` | No action - problem self-resolved |
| **B: Inconclusive** | Low (<0.5) | `true` | `investigation_inconclusive` | Human review required |

---

## 🆕 API Changes

### New Enum Value: `INVESTIGATION_INCONCLUSIVE`

```python
class HumanReviewReason(str, Enum):
    # ... existing values ...
    INVESTIGATION_INCONCLUSIVE = "investigation_inconclusive"
```

**Meaning**: LLM investigation did not yield conclusive results - could not determine root cause or current state.

**When Used**:
- LLM cannot determine root cause
- Resource state is ambiguous
- Metrics/events unavailable or conflicting
- Investigation yields no clear answer

**When NOT Used**:
- Problem is confirmed resolved → `needs_human_review=false`
- Workflow validation fails → `WORKFLOW_NOT_FOUND`, etc.
- RCA found but confidence low → `LOW_CONFIDENCE`

---

## 📊 Response Examples

### Outcome A: Problem Confirmed Resolved (No Human Review)

```json
{
  "incident_id": "inc-123",
  "needs_human_review": false,
  "human_review_reason": null,
  "selected_workflow": null,
  "confidence": 0.92,
  "investigation_summary": "Investigated OOMKilled signal. Pod 'myapp' recovered automatically. Status: Running, memory at 45% of limit. No remediation required.",
  "warnings": ["Problem self-resolved - no remediation required"]
}
```

**AIAnalysis Action**: Mark as `Completed` with reason `WorkflowNotNeeded`. No workflow execution.

---

### Outcome B: Investigation Inconclusive (Human Review Required)

```json
{
  "incident_id": "inc-123",
  "needs_human_review": true,
  "human_review_reason": "investigation_inconclusive",
  "selected_workflow": null,
  "confidence": 0.35,
  "investigation_summary": "Unable to determine root cause. Pod status ambiguous, events conflicting, metrics unavailable.",
  "warnings": ["Investigation inconclusive - human review recommended"]
}
```

**AIAnalysis Action**: Mark as `Failed` with subReason `InvestigationInconclusive`. Notify operators.

---

## 🎯 Team-Specific Guidance

### AIAnalysis Team

**Required Changes (V1.1)**:

1. **Handle new subReason**:
   ```yaml
   status:
     phase: Failed
     reason: WorkflowResolutionFailed
     subReason: InvestigationInconclusive  # NEW
     message: "Investigation inconclusive - human review recommended"
   ```

2. **Handle confident resolution**:
   ```yaml
   status:
     phase: Completed
     reason: WorkflowNotNeeded
     subReason: ProblemResolved  # Optional
     message: "Problem self-resolved. No remediation required."
   ```

3. **Add to Failure Reason Taxonomy** (per `RESPONSE_AIANALYSIS_NEEDS_HUMAN_REVIEW.md`):
   | `human_review_reason` | `reason` | `subReason` |
   |----------------------|----------|-------------|
   | `investigation_inconclusive` | `WorkflowResolutionFailed` | `InvestigationInconclusive` |

---

### Remediation Orchestrator Team

**Required Changes (V1.1)**:

1. **Handle `WorkflowNotNeeded` from AIAnalysis**:
   - Update RemediationRequest status to reflect no action needed
   - Do NOT create WorkflowExecution

2. **Optional Metric**:
   ```prometheus
   kubernaut_remediation_no_action_needed_total{
     reason="problem_resolved|investigation_inconclusive"
   }
   ```

---

### Notification Team

**Required Changes (V1.1)**:

1. **Optional: Configure routing for self-resolved incidents**:
   | Notification Type | Use Case |
   |-------------------|----------|
   | **Skip** | Most self-resolutions don't need notification |
   | **Informational** | "FYI: Incident auto-resolved" for audit |
   | **Warning** | Pattern of repeated self-resolutions (flapping) |

2. **Suggested Label**:
   ```yaml
   # On AIAnalysis CR
   labels:
     kubernaut.ai/investigation-outcome: "resolved" | "inconclusive" | "workflow-selected"
   ```

---

## 📅 Timeline

| Phase | Target | Teams Affected |
|-------|--------|----------------|
| Enum added | ✅ Complete | HolmesGPT-API |
| BR-HAPI-200 documented | ✅ Complete | All |
| LLM prompt update | V1.0 | HolmesGPT-API |
| AIAnalysis handler | V1.0 | AIAnalysis |
| RO handler | ✅ V1.0 (planned) | RO |
| Notification routing | ✅ V1.0 (Day 15) | Notification |

---

## ✅ Acknowledgment Required

Please acknowledge receipt and provide feedback:

| Team | Acknowledged | Notes |
|------|--------------|-------|
| AIAnalysis | ✅ 2025-12-07 | See below |
| Remediation Orchestrator | ✅ 2025-12-07 | See below |
| Notification | ✅ 2025-12-07 | See below |

---

### AIAnalysis Team Acknowledgment (2025-12-07)

**Status**: ✅ **ACKNOWLEDGED - V1.0 SCOPE**

We acknowledge receipt of BR-HAPI-200. **This is V1.0 scope, not V1.1.**

#### ⚠️ Scope Clarification Required

The notice marks this as "V1.1 - No immediate action required", but **BR-HAPI-197 is V1.0** and the `human_review_reason` enum infrastructure is already live. The `investigation_inconclusive` value **can be returned now** by HAPI.

**Question for HAPI Team**:

> **Q1**: Is `investigation_inconclusive` already being returned by the HAPI service in V1.0, or is the LLM prompt update that triggers this value truly deferred to V1.1?
>
> If HAPI can return this value today (even if rare), AIAnalysis must handle it in V1.0.

#### Implementation Status (V1.0)

| Task | Status | Notes |
|------|--------|-------|
| Add `investigation_inconclusive` → `InvestigationInconclusive` mapping | ✅ Complete | `pkg/aianalysis/handlers/investigating.go` |
| Handle "Resolved" outcome (`WorkflowNotNeeded`) | ⏳ Day 8 | Need to verify HAPI response structure |
| Unit tests for new mapping | ⏳ Day 8 | Following TDD |

#### Answer to Q1

> **Q1**: Should `ProblemResolved` be a new subReason, or use existing `WorkflowNotNeeded`?

**Answer**: Both.
- `Reason: WorkflowNotNeeded` (consistent with existing taxonomy)
- `SubReason: ProblemResolved` (provides specific context)

This maintains the `Reason + SubReason` pattern established for `WorkflowResolutionFailed`.

#### "Resolved" Outcome Handling

For Outcome A (problem self-resolved), we need clarification:

> **Q2**: When a problem is confirmed resolved (high confidence, no workflow needed), what is the exact response structure from HAPI?
>
> Expected (please confirm):
> ```json
> {
>   "needs_human_review": false,
>   "human_review_reason": null,
>   "selected_workflow": null,
>   "confidence": 0.92,
>   "investigation_summary": "Pod recovered automatically..."
> }
> ```
>
> Is `selected_workflow: null` the correct indicator, or is there a different field?

---

**Acknowledged By**: AIAnalysis Team
**Date**: December 7, 2025

---

### RO Team Acknowledgment (2025-12-07)

**Status**: ✅ **ACKNOWLEDGED - V1.0 SCOPE**

We acknowledge receipt of BR-HAPI-200 and will handle both scenarios in V1.0:

#### Implementation Scope

| Outcome | BR | RO Action | V1.0 Status |
|---------|------|-----------|-------------|
| **A: Problem Resolved** | BR-ORCH-037 | Skip WE, mark RR `Completed` with `Outcome=NoActionRequired` | ✅ Planned |
| **B: Investigation Inconclusive** | BR-ORCH-036 (v2.0) | Create `manual-review` notification | ✅ Planned |

#### Documentation Created

1. **BR-ORCH-036 v2.0**: Extended to include `InvestigationInconclusive` as a SubReason for manual review
2. **BR-ORCH-037 v1.0**: New BR for `WorkflowNotNeeded` handling

#### Answers to Questions

> **Q2: Do you need a new metric for self-resolved incidents?**

**Answer**: Yes. We'll add:
```prometheus
kubernaut_remediationorchestrator_no_action_needed_total{reason="problem_resolved"}
```

> **Q3: Should self-resolved incidents generate any notification by default?**

**Answer**: No notification by default. Optional informational notification if `notify_on_self_resolved=true` in config.

#### Implementation Timeline

| Task | Target Day |
|------|------------|
| Update RR API with `Outcome` field | Day 7 |
| Implement `WorkflowNotNeeded` handler | Day 7 |
| Add `InvestigationInconclusive` to manual review handler | Day 4 (already planned) |
| Add metrics | Day 8 |

---

**Acknowledged By**: RemediationOrchestrator Team
**Date**: December 7, 2025

---

### Notification Team Acknowledgment (2025-12-07)

**Status**: ✅ **ACKNOWLEDGED - V1.1 SCOPE**

We acknowledge receipt of BR-HAPI-200. This is a V1.1 enhancement that does NOT affect our V1.0-complete status.

#### Assessment

| Aspect | Status |
|--------|--------|
| **V1.0 Impact** | ❌ None - Notification V1.0 is complete |
| **Existing Support** | ✅ BR-NOT-065 routing infrastructure already supports label-based routing |
| **Implementation Effort** | ~2 hours (V1.1) |

#### Implementation Plan (V1.1)

The existing routing infrastructure in `pkg/notification/routing/` already supports:
- Alertmanager-compatible label-based routing
- Dynamic ConfigMap-based configuration (BR-NOT-067)
- Label constants in `pkg/notification/routing/labels.go`

**V1.1 Tasks**:
1. Add `LabelInvestigationOutcome` constant to `pkg/notification/routing/labels.go`
2. Add example routing rules to default config
3. Add 3-5 unit tests for new routing scenarios

#### Answer to Q3

> **Q3: Should self-resolved incidents generate any notification by default?**

**Answer**: **No notification by default**. We agree with RO's assessment.

**Rationale**:
- Self-resolved incidents require no human action
- Unnecessary notifications create alert fatigue
- Operators can opt-in via routing config if audit trail is desired

**Routing Configuration Example (V1.1)**:
```yaml
route:
  routes:
    # Self-resolved: Skip notification by default
    - match:
        kubernaut.ai/investigation-outcome: resolved
      receiver: null-receiver  # Or use continue: false with no receiver

    # Inconclusive: Route to ops for review
    - match:
        kubernaut.ai/investigation-outcome: inconclusive
      receiver: slack-ops
```

---

**Acknowledged By**: Notification Team
**Date**: December 7, 2025

---

## 🔗 Related Documents

| Document | Purpose |
|----------|---------|
| [BR-HAPI-200](../requirements/BR-HAPI-200-resolved-stale-signals.md) | Business requirement |
| [BR-HAPI-197](../requirements/BR-HAPI-197-needs-human-review-field.md) | Parent: `needs_human_review` field |
| [RESPONSE_AIANALYSIS_NEEDS_HUMAN_REVIEW.md](./RESPONSE_AIANALYSIS_NEEDS_HUMAN_REVIEW.md) | AIAnalysis failure taxonomy |

---

## ❓ Questions

Please add questions here or create a response document:

1. **AIAnalysis**: Should `ProblemResolved` be a new subReason, or use existing `WorkflowNotNeeded`?
2. **RO**: Do you need a new metric for self-resolved incidents?
3. **Notification**: Should self-resolved incidents generate any notification by default?

---

**Maintained By**: HolmesGPT-API Team
**Last Updated**: December 7, 2025

---

## 📢 Update Notification (2025-12-07) - REVISED

**To**: HolmesGPT-API Team
**From**: Notification Team

The Notification team has **REVISED** acknowledgment to align with RO's V1.0 scope:

- ✅ **Acknowledged**: **V1.0 scope** (revised from V1.1)
- ✅ **Aligned with RO**: RO creates `InvestigationInconclusive` notifications in V1.0
- ✅ **Q3 Answered**: No notification by default for self-resolved incidents
- ✅ **Implementation Plan**: ~1 hour (Day 15) - Add label constant + routing rules + tests
- ✅ **Existing Support**: BR-NOT-065 routing infrastructure already supports this pattern

**V1.0 Tasks Scheduled**:
1. Add `LabelInvestigationOutcome` constant
2. Add routing configuration example
3. Add 2-3 unit tests

**Status**: 2 of 3 teams acknowledged for V1.0 (AIAnalysis pending)

