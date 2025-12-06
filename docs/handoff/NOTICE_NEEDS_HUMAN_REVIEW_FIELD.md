# 🔔 NOTICE: New `needs_human_review` Field in IncidentResponse

**Date**: December 6, 2025
**From**: HolmesGPT-API Team
**To**: AIAnalysis Team
**Priority**: 🔴 HIGH - API Contract Change
**Version**: v1.0

---

## Summary

A new top-level field `needs_human_review: bool` has been added to the `IncidentResponse` schema. This field indicates when the AI analysis could not produce a reliable result and requires human intervention.

**Business Requirement**: [BR-HAPI-197](../requirements/BR-HAPI-197-needs-human-review-field.md)

---

## API Contract Change

### New Field

```json
{
  "needs_human_review": {
    "type": "boolean",
    "default": false,
    "description": "True when AI analysis could not produce a reliable result. Reasons include: workflow validation failures after retries, LLM parsing errors, no suitable workflow found, or other AI reliability issues. When true, AIAnalysis should NOT create WorkflowExecution - requires human intervention. Check 'warnings' field for specific reasons."
  }
}
```

### Updated IncidentResponse Schema

```json
{
  "incident_id": "inc-123",
  "analysis": "...",
  "root_cause_analysis": {...},
  "selected_workflow": {...},
  "confidence": 0.85,
  "timestamp": "2025-12-06T10:00:00Z",
  "target_in_owner_chain": true,
  "warnings": ["..."],
  "needs_human_review": false,         // ← NEW FIELD
  "alternative_workflows": [...]
}
```

---

## When `needs_human_review` is `true`

| Condition | `needs_human_review` | `warnings` Contains |
|-----------|---------------------|---------------------|
| Workflow validation failed | `true` | "Workflow validation failed: [errors]" |
| No workflows matched search | `true` | "No workflows matched the search criteria" |
| Low confidence (<70%) | `true` | "Low confidence selection (X%) - manual review recommended" |
| LLM response parsing failed | `true` | "Failed to parse LLM response" |
| Everything OK | `false` | (may have minor warnings) |

---

## AIAnalysis Required Actions

### 1. Update Go Client

Regenerate the Go client to include the new field:

```bash
# Install ogen if not already installed
go install github.com/ogen-go/ogen/cmd/ogen@latest

# Regenerate Go client
ogen -package holmesgpt -target pkg/clients/holmesgpt \
    holmesgpt-api/api/openapi.json
```

### 2. Update Recommending Handler

In the AIAnalysis Recommending phase handler, check this field **before** creating WorkflowExecution:

```go
// pkg/aianalysis/handlers/recommending.go

func (h *RecommendingHandler) Handle(ctx context.Context, analysis *v1alpha1.AIAnalysis) (ctrl.Result, error) {
    // ... call HolmesGPT-API ...
    hapiResponse, err := h.holmesgptClient.AnalyzeIncident(ctx, request)
    if err != nil {
        return h.handleError(ctx, analysis, err)
    }

    // BR-HAPI-197: Check if human review is required
    if hapiResponse.NeedsHumanReview {
        // Do NOT create WorkflowExecution - requires human intervention
        analysis.Status.Phase = v1alpha1.PhaseManualReviewRequired
        analysis.Status.FailureReason = "AIReviewRequired"
        analysis.Status.Message = strings.Join(hapiResponse.Warnings, "; ")

        // Record event for observability
        h.recorder.Event(analysis, corev1.EventTypeWarning, "HumanReviewRequired",
            fmt.Sprintf("AI analysis requires human review: %s", analysis.Status.Message))

        return ctrl.Result{}, h.updateStatus(ctx, analysis)
    }

    // Continue with normal flow - create WorkflowExecution
    // ...
}
```

### 3. Update AIAnalysis CRD Status (Optional)

Consider adding a new phase to handle this state:

```go
// api/aianalysis/v1alpha1/aianalysis_types.go

const (
    PhaseValidating    Phase = "Validating"
    PhaseInvestigating Phase = "Investigating"
    PhaseRecommending  Phase = "Recommending"
    PhaseCompleted     Phase = "Completed"
    PhaseFailed        Phase = "Failed"
    PhaseManualReviewRequired Phase = "ManualReviewRequired"  // ← NEW PHASE
)
```

---

## Business Behavior (BR-HAPI-197)

### What Happens When `needs_human_review` is `true`

1. **AIAnalysis MUST NOT** create a WorkflowExecution CRD
2. **AIAnalysis SHOULD** transition to `ManualReviewRequired` phase
3. **AIAnalysis SHOULD** preserve `selected_workflow` for operator context (if present)
4. **AIAnalysis SHOULD** log/record warnings for audit trail
5. **Operator MUST** manually review and decide next action

### What Operators Can Do

| Action | Description |
|--------|-------------|
| **Approve with modifications** | Operator reviews, fixes parameters, manually creates WorkflowExecution |
| **Reject** | Operator determines AI recommendation is incorrect, closes incident |
| **Retry** | Operator triggers re-analysis (if transient LLM issue) |
| **Escalate** | Operator escalates to human remediation team |

### Metrics & Observability

When `needs_human_review` is `true`, AIAnalysis should emit:

```prometheus
# Counter for human review required events
kubernaut_aianalysis_human_review_required_total{reason="workflow_validation_failed"} 1
kubernaut_aianalysis_human_review_required_total{reason="no_workflows_matched"} 1
kubernaut_aianalysis_human_review_required_total{reason="low_confidence"} 1
```

---

## Migration Guide

### Before (No `needs_human_review` field)

```go
// Old code - assumed all responses were reliable
if hapiResponse.SelectedWorkflow == nil {
    // Handle no workflow case
}
```

### After (With `needs_human_review` field)

```go
// New code - check reliability first
if hapiResponse.NeedsHumanReview {
    // AI couldn't produce reliable result - require human review
    return h.transitionToManualReview(ctx, analysis, hapiResponse.Warnings)
}

if hapiResponse.SelectedWorkflow == nil {
    // Normal case - no workflow matched (different from unreliable AI)
}
```

---

## Testing Recommendations

### Unit Tests

```go
func TestRecommendingHandler_NeedsHumanReview(t *testing.T) {
    tests := []struct {
        name           string
        hapiResponse   *holmesgpt.IncidentResponse
        expectedPhase  v1alpha1.Phase
    }{
        {
            name: "human review required - validation failed",
            hapiResponse: &holmesgpt.IncidentResponse{
                NeedsHumanReview: true,
                Warnings:         []string{"Workflow validation failed: workflow not found"},
            },
            expectedPhase: v1alpha1.PhaseManualReviewRequired,
        },
        {
            name: "normal flow - no human review needed",
            hapiResponse: &holmesgpt.IncidentResponse{
                NeedsHumanReview: false,
                SelectedWorkflow: &holmesgpt.SelectedWorkflow{...},
            },
            expectedPhase: v1alpha1.PhaseCompleted,
        },
    }
    // ... test implementation
}
```

---

## Timeline

| Milestone | Date | Status |
|-----------|------|--------|
| Field added to HolmesGPT-API | Dec 6, 2025 | ✅ Complete |
| OpenAPI spec regenerated | Dec 6, 2025 | ✅ Complete |
| BR-HAPI-197 documented | Dec 6, 2025 | ✅ Complete |
| AIAnalysis integration | TBD | ⏳ Pending |

---

## Questions?

Contact HolmesGPT-API team or respond in this document with questions.

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-06 | Initial notification |

