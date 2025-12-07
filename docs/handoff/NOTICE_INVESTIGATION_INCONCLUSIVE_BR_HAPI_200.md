# NOTICE: New Investigation Outcome Types (BR-HAPI-200)

**Date**: December 7, 2025
**From**: HolmesGPT-API Team
**To**: AIAnalysis Team, Remediation Orchestrator Team, Notification Team
**Priority**: 🔴 **HIGH - V1.0 SCOPE** (Confirmed by HAPI Team)
**Status**: ✅ ALL TEAMS ACKNOWLEDGED FOR V1.0

---

## 🎉 HAPI V1.0 IMPLEMENTATION COMPLETE

**Date**: December 7, 2025

The **HolmesGPT-API team has completed all V1.0 work** for BR-HAPI-200:

| Component | Status | Details |
|-----------|--------|---------|
| `INVESTIGATION_INCONCLUSIVE` enum | ✅ Complete | `incident_models.py` |
| LLM prompt guidance | ✅ Complete | `incident.py` (Outcome A/B handling) |
| Response parsing | ✅ Complete | `investigation_outcome` field |
| Unit tests | ✅ Complete | 474 tests passing |
| BR-HAPI-200 documentation | ✅ Complete | Aligned with implementation |
| Q4 API contract clarification | ✅ Complete | See answers below |

**Other teams can now proceed with their implementations.**

---

## ⚠️ SCOPE CLARIFICATION

**This is V1.0 scope, NOT V1.1.**

The original notice incorrectly marked this as V1.1. HAPI Team confirmed (2025-12-07):

> "This is not an enhancement, it's covering a real scenario that can happen in production."

All teams must implement this in V1.0.

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

## 🎯 Team-Specific Guidance (V1.0)

### AIAnalysis Team (V1.0)

**Required Changes**:

1. **Handle new subReason**:
   ```yaml
   status:
     phase: Failed
     reason: WorkflowResolutionFailed
     subReason: InvestigationInconclusive
     message: "Investigation inconclusive - human review recommended"
   ```

2. **Handle confident resolution**:
   ```yaml
   status:
     phase: Completed
     reason: WorkflowNotNeeded
     subReason: ProblemResolved
     message: "Problem self-resolved. No remediation required."
   ```

3. **Add to Failure Reason Taxonomy** (per `RESPONSE_AIANALYSIS_NEEDS_HUMAN_REVIEW.md`):
   | `human_review_reason` | `reason` | `subReason` |
   |----------------------|----------|-------------|
   | `investigation_inconclusive` | `WorkflowResolutionFailed` | `InvestigationInconclusive` |

---

### Remediation Orchestrator Team (V1.0)

**Required Changes**:

1. **Handle `WorkflowNotNeeded` from AIAnalysis**:
   - Update RemediationRequest status to reflect no action needed
   - Do NOT create WorkflowExecution

2. **Metric**:
   ```prometheus
   kubernaut_remediationorchestrator_no_action_needed_total{
     reason="problem_resolved|investigation_inconclusive"
   }
   ```

---

### Notification Team (V1.0)

**Required Changes**:

1. **Add routing label constant**:
   ```go
   // pkg/notification/routing/labels.go
   LabelInvestigationOutcome = "kubernaut.ai/investigation-outcome"
   ```

2. **Configure routing for outcomes**:
   ```yaml
   route:
     routes:
       # Self-resolved: Skip notification by default
       - match:
           kubernaut.ai/investigation-outcome: resolved
         receiver: null-receiver

       # Inconclusive: Route to ops for review
       - match:
           kubernaut.ai/investigation-outcome: inconclusive
         receiver: slack-ops
   ```

---

## 📅 Timeline (V1.0)

| Phase | Target | Teams Affected | Status |
|-------|--------|----------------|--------|
| Enum added | ✅ Complete | HolmesGPT-API | ✅ Done |
| BR-HAPI-200 documented | ✅ Complete | All | ✅ Done |
| LLM prompt update | ✅ Complete | HolmesGPT-API | ✅ Done |
| Response parsing | ✅ Complete | HolmesGPT-API | ✅ Done |
| Unit tests (474 total) | ✅ Complete | HolmesGPT-API | ✅ Done |
| **HAPI V1.0 Complete** | ✅ **2025-12-07** | **HolmesGPT-API** | ✅ **ALL DONE** |
| **AIAnalysis handler** | ✅ **2025-12-07** | **AIAnalysis** | ✅ **ALL DONE** |
| RO handler | V1.0 | RO | ⏳ Day 7 |
| Notification routing | V1.0 | Notification | ⏳ Day 15 |

---

## ✅ Acknowledgment Status

| Team | Acknowledged | Scope | Status |
|------|--------------|-------|--------|
| AIAnalysis | ✅ 2025-12-07 | **V1.0** | ✅ **IMPLEMENTATION COMPLETE** |
| Remediation Orchestrator | ✅ 2025-12-07 | **V1.0** | BR-ORCH-036 v2.0, BR-ORCH-037 created |
| Notification | ✅ 2025-12-07 | **V1.0** | Day 15 (~1 hour) |

---

## 📝 Team Acknowledgments

### AIAnalysis Team (2025-12-07)

**Status**: ✅ **IMPLEMENTATION COMPLETE**

#### Implementation Status

| Task | Status | Notes |
|------|--------|-------|
| Add `investigation_inconclusive` → `InvestigationInconclusive` mapping | ✅ Complete | `pkg/aianalysis/handlers/investigating.go` |
| Handle "Resolved" outcome (`WorkflowNotNeeded`) | ✅ **Complete** | `handleProblemResolved()` method |
| Unit tests for new mapping | ✅ **Complete** | 163 tests, 87.6% coverage |
| CRD SubReason enum update | ✅ **Complete** | Added `InvestigationInconclusive`, `ProblemResolved` |

#### Taxonomy Decision (Implemented)

- `Reason: WorkflowNotNeeded` + `SubReason: ProblemResolved` for self-resolved ✅
- `Reason: WorkflowResolutionFailed` + `SubReason: InvestigationInconclusive` for inconclusive ✅

#### Detection Pattern (Implemented)

```go
// Outcome A: Problem Resolved (high confidence, no workflow)
if resp.SelectedWorkflow == nil && resp.Confidence >= 0.7 && !resp.NeedsHumanReview {
    // Phase=Completed, Reason=WorkflowNotNeeded, SubReason=ProblemResolved
}

// Outcome B: Investigation Inconclusive (human review required)
if resp.NeedsHumanReview && *resp.HumanReviewReason == "investigation_inconclusive" {
    // Phase=Failed, Reason=WorkflowResolutionFailed, SubReason=InvestigationInconclusive
}
```

---

### HAPI Team Response (2025-12-07)

**Confirmed V1.0 scope** and response structure:

```json
{
  "needs_human_review": false,
  "human_review_reason": null,
  "selected_workflow": null,
  "confidence": 0.92,
  "investigation_summary": "Pod recovered automatically..."
}
```

`selected_workflow: null` + high confidence + `needs_human_review: false` = problem resolved.

---

### RO Team (2025-12-07)

**Status**: ✅ **ACKNOWLEDGED - V1.0 SCOPE**

| Outcome | BR | RO Action | Status |
|---------|------|-----------|--------|
| **Problem Resolved** | BR-ORCH-037 | Skip WE, mark RR `Completed` | ⏳ Day 7 |
| **Investigation Inconclusive** | BR-ORCH-036 v2.0 | Create `manual-review` notification | ⏳ Day 4 |

**Metric**: `kubernaut_remediationorchestrator_no_action_needed_total{reason="problem_resolved"}`

**Self-resolved notification**: No notification by default.

---

### Notification Team (2025-12-07)

**Status**: ✅ **ACKNOWLEDGED - V1.0 SCOPE**

| Task | Status |
|------|--------|
| Add `LabelInvestigationOutcome` constant | ⏳ Day 15 |
| Add routing configuration example | ⏳ Day 15 |
| Add 2-3 unit tests | ⏳ Day 15 |

**Self-resolved notification**: No notification by default (agreed with RO).

---

## ❓ Questions

| Q# | Question | Answer | By |
|----|----------|--------|-----|
| Q1 | Should `ProblemResolved` be a new subReason? | Yes, with `Reason: WorkflowNotNeeded` | AIAnalysis |
| Q2 | Response structure for "Resolved"? | Confirmed (see HAPI response above) | HAPI |
| Q3 | Self-resolved notification by default? | No | RO + Notification |
| **Q4** | **API CONTRACT: SubReason Naming** | ✅ **RESOLVED** | **HAPI** |

---

### Q4: API Contract Clarification (RESOLVED 2025-12-07)

**From**: AIAnalysis Team
**To**: HAPI Team

**Original Issue**: Conflicting SubReason values in documentation:

| Document | SubReason Value |
|----------|-----------------|
| BR-HAPI-200 (line 182) | `ProblemNotReproducible` |
| NOTICE (line 117) | `ProblemResolved` |
| BR-ORCH-037 (line 48) | `ProblemResolved` |

---

#### ✅ HAPI Team Response (Based on Authoritative Code)

**Q1: Is `problem_not_reproducible` a valid HAPI enum value?**

**Answer: NO.**

**Authoritative Source**: `holmesgpt-api/src/models/incident_models.py` lines 36-53

```python
class HumanReviewReason(str, Enum):
    WORKFLOW_NOT_FOUND = "workflow_not_found"
    IMAGE_MISMATCH = "image_mismatch"
    PARAMETER_VALIDATION_FAILED = "parameter_validation_failed"
    NO_MATCHING_WORKFLOWS = "no_matching_workflows"
    LOW_CONFIDENCE = "low_confidence"
    LLM_PARSING_ERROR = "llm_parsing_error"
    # BR-HAPI-200: LLM investigation did not yield conclusive results
    INVESTIGATION_INCONCLUSIVE = "investigation_inconclusive"  # ← ONLY new value
```

`problem_not_reproducible` appears in BR-HAPI-200 documentation (drafted before implementation) but was **never implemented**. BR-HAPI-200 will be updated to align with the authoritative implementation.

---

**Q2: For Outcome A (confident resolution), what should HAPI return?**

**Answer: Option (A) - `human_review_reason: null`**

**Authoritative Source**: `holmesgpt-api/src/extensions/incident.py` lines 1278-1302

```python
# BR-HAPI-200: Handle special investigation outcomes
investigation_outcome = json_data.get("investigation_outcome") if json_data else None

# BR-HAPI-200: Outcome A - Problem self-resolved (high confidence, no workflow needed)
if investigation_outcome == "resolved":
    warnings.append("Problem self-resolved - no remediation required")
    needs_human_review = False
    human_review_reason = None  # ← NULL by design

# BR-HAPI-200: Outcome B - Investigation inconclusive (human review required)
elif investigation_outcome == "inconclusive":
    warnings.append("Investigation inconclusive - human review recommended")
    needs_human_review = True
    human_review_reason = "investigation_inconclusive"  # ← Uses enum
```

**Authoritative Test Source**: `holmesgpt-api/tests/unit/test_resolved_signals_br_hapi_200.py` lines 111-159

| Outcome | `needs_human_review` | `human_review_reason` | `confidence` | AIAnalysis SubReason |
|---------|---------------------|----------------------|--------------|---------------------|
| **A: Resolved** | `false` | `null` | `≥ 0.7` | `ProblemResolved` (derived) |
| **B: Inconclusive** | `true` | `investigation_inconclusive` | `< 0.5` | `InvestigationInconclusive` |

**Rationale**: When problem is **confidently resolved**, no human review is needed → no enum value returned. AIAnalysis derives `SubReason: ProblemResolved` from the response pattern.

**✅ AIAnalysis Recommendation is correct.**

---

**Status**: ✅ RESOLVED - AIAnalysis team's Option (A) confirmed per authoritative implementation

---

## 🔗 Related Documents

| Document | Purpose |
|----------|---------|
| [BR-HAPI-200](../requirements/BR-HAPI-200-resolved-stale-signals.md) | Business requirement |
| [BR-HAPI-197](../requirements/BR-HAPI-197-needs-human-review-field.md) | Parent: `needs_human_review` field |
| [BR-ORCH-036 v2.0](../requirements/BR-ORCH-036-manual-review-notification.md) | RO manual review handling |
| [BR-ORCH-037](../requirements/BR-ORCH-037-workflow-not-needed.md) | RO workflow-not-needed handling |
| [RESPONSE_AIANALYSIS_NEEDS_HUMAN_REVIEW.md](./RESPONSE_AIANALYSIS_NEEDS_HUMAN_REVIEW.md) | AIAnalysis failure taxonomy |

---

## 📝 Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| v1.0 | 2025-12-07 | HAPI Team | Initial notice (incorrectly marked V1.1) |
| v1.1 | 2025-12-07 | AIAnalysis Team | Scope clarification: This is V1.0 |
| v1.2 | 2025-12-07 | HAPI Team | Confirmed V1.0 scope, answered Q1 & Q2 |
| v1.3 | 2025-12-07 | RO Team | Acknowledged V1.0, created BR-ORCH-037 |
| v1.4 | 2025-12-07 | Notification Team | Acknowledged V1.0 (revised from V1.1) |
| v2.0 | 2025-12-07 | Architecture Team | Cleaned up document, clarified V1.0 throughout |
| v2.1 | 2025-12-07 | HAPI Team | Answered Q4: Confirmed Option (A), clarified enum naming |
| v2.2 | 2025-12-07 | HAPI Team | **V1.0 COMPLETE**: All HAPI implementation done |
| v2.3 | 2025-12-07 | AIAnalysis Team | **V1.0 COMPLETE**: `handleProblemResolved()`, 163 tests, 87.6% coverage |

---

**Maintained By**: HolmesGPT-API Team
**Last Updated**: December 7, 2025
