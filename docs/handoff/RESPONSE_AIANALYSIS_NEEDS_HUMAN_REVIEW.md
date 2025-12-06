# RESPONSE: AIAnalysis Integration of `needs_human_review` Field

**Date**: 2025-12-06
**From**: AIAnalysis Team
**To**: HolmesGPT-API Team
**In Response To**: [NOTICE_NEEDS_HUMAN_REVIEW_FIELD.md](./NOTICE_NEEDS_HUMAN_REVIEW_FIELD.md)
**Status**: ✅ **ACKNOWLEDGED - Integration Planned**

---

## 📋 **Summary**

AIAnalysis team acknowledges BR-HAPI-197 and will integrate the `needs_human_review` field. This response documents our integration approach and clarifies architectural differences from the notice.

---

## ⚠️ **Architecture Clarification**

The notice references `RecommendingHandler` which **no longer exists** in AIAnalysis.

| Notice Reference | Current AIAnalysis (v1.8+) |
|------------------|----------------------------|
| `RecommendingHandler` | ❌ **REMOVED** in v1.8 |
| 5-phase flow | ✅ 4-phase: `Pending → Investigating → Analyzing → Completed` |
| `PhaseManualReviewRequired` | ❌ Will NOT add new phase (see below) |

**Reason**: We removed `Recommending` because workflow data is captured in `InvestigatingHandler` when calling HolmesGPT-API. The `AnalyzingHandler` only evaluates Rego policies.

---

## ✅ **Integration Approach**

### **Failure Reason Taxonomy**

We will use `Failed` phase with structured reasons:

| Field | Purpose | Example |
|-------|---------|---------|
| `status.phase` | Terminal state | `Failed` |
| `status.reason` | Umbrella category | `WorkflowResolutionFailed` |
| `status.subReason` | Specific cause | `WorkflowNotFound` |
| `status.message` | Human-readable detail | `"Workflow 'restart-pod-v1' not found in catalog"` |

### **Sub-Reason Mapping**

| BR-HAPI-197 Trigger | `reason` | `subReason` |
|---------------------|----------|-------------|
| Workflow Not Found | `WorkflowResolutionFailed` | `WorkflowNotFound` |
| Container Image Mismatch | `WorkflowResolutionFailed` | `ImageMismatch` |
| Parameter Validation Failed | `WorkflowResolutionFailed` | `ParameterValidationFailed` |
| No Workflows Matched | `WorkflowResolutionFailed` | `NoMatchingWorkflows` |
| Low Confidence | `WorkflowResolutionFailed` | `LowConfidence` |
| LLM Parsing Error | `WorkflowResolutionFailed` | `LLMParsingError` |

### **Handler Location**

The check will happen in `InvestigatingHandler` (where HolmesGPT-API is called):

```go
// pkg/aianalysis/handlers/investigating.go

func (h *InvestigatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    response, err := h.client.AnalyzeIncident(ctx, request)
    if err != nil {
        return h.handleError(ctx, analysis, err)
    }

    // BR-HAPI-197: Check if workflow resolution failed
    if response.NeedsHumanReview {
        analysis.Status.Phase = aianalysis.PhaseFailed
        analysis.Status.Reason = "WorkflowResolutionFailed"
        analysis.Status.SubReason = h.mapWarningsToSubReason(response.Warnings)
        analysis.Status.Message = strings.Join(response.Warnings, "; ")

        // Store partial response for operator context
        h.capturePartialResponse(analysis, response)

        // Emit metric with sub-reason
        metrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", analysis.Status.SubReason).Inc()

        return ctrl.Result{}, nil  // Terminal - no requeue
    }

    // Continue normal flow...
}
```

---

## 📊 **CRD Schema Changes**

### **Remove `Recommending` Phase**

```go
// BEFORE
// +kubebuilder:validation:Enum=Pending;Investigating;Analyzing;Recommending;Completed;Failed

// AFTER
// +kubebuilder:validation:Enum=Pending;Investigating;Analyzing;Completed;Failed
```

### **Add `SubReason` Field**

```go
// AIAnalysisStatus
type AIAnalysisStatus struct {
    Phase     string `json:"phase"`
    Message   string `json:"message,omitempty"`
    Reason    string `json:"reason,omitempty"`
    SubReason string `json:"subReason,omitempty"`  // NEW: Specific failure cause
    // ...
}
```

### **SubReason Enum**

```go
// +kubebuilder:validation:Enum=WorkflowNotFound;ImageMismatch;ParameterValidationFailed;NoMatchingWorkflows;LowConfidence;LLMParsingError
SubReason string `json:"subReason,omitempty"`
```

---

## 📈 **Metrics**

Per DD-005 naming convention:

```prometheus
# Counter with sub-reason label for granularity
aianalysis_failures_total{reason="WorkflowResolutionFailed", sub_reason="WorkflowNotFound"} 1
aianalysis_failures_total{reason="WorkflowResolutionFailed", sub_reason="LowConfidence"} 1
aianalysis_failures_total{reason="WorkflowResolutionFailed", sub_reason="NoMatchingWorkflows"} 1
```

---

## 🗓️ **Timeline**

| Milestone | Target | Status |
|-----------|--------|--------|
| Acknowledge notice | 2025-12-06 | ✅ Complete |
| Update CRD schema | Day 5 | ⏳ Pending |
| Update HolmesGPT client | Day 5 | ⏳ Pending |
| Update InvestigatingHandler | Day 5 | ⏳ Pending |
| Add metrics | Day 5 | ⏳ Pending |
| Unit tests | Day 5 | ⏳ Pending |

---

## ❓ **Questions for HAPI Team**

### Q1: Warning-to-SubReason Mapping

How should we map `warnings` to `subReason`? Options:

**Option A**: Parse warning text (fragile)
```go
if strings.Contains(warning, "not found") {
    return "WorkflowNotFound"
}
```

**Option B**: HAPI provides structured error code (preferred)
```json
{
  "needs_human_review": true,
  "human_review_reason": "workflow_not_found",  // NEW FIELD
  "warnings": ["Workflow 'restart-pod-v1' not found in catalog"]
}
```

**Recommendation**: Option B is more reliable. Can HAPI add a `human_review_reason` enum field?

### Q2: Partial Response Preservation

When `needs_human_review=true`, should we still store:
- `selected_workflow` (if present)?
- `root_cause_analysis`?
- `alternative_workflows`?

**Our assumption**: YES, store everything for operator context.

---

## 📚 **References**

| Document | Purpose |
|----------|---------|
| [BR-HAPI-197](../requirements/BR-HAPI-197-needs-human-review-field.md) | Business requirement |
| [DD-HAPI-002](../architecture/decisions/DD-HAPI-002-workflow-parameter-validation.md) | Design decision |
| [NOTICE_NEEDS_HUMAN_REVIEW_FIELD.md](./NOTICE_NEEDS_HUMAN_REVIEW_FIELD.md) | Original notice |
| [reconciliation-phases.md](../services/crd-controllers/02-aianalysis/reconciliation-phases.md) | AIAnalysis phases |

---

## ✅ **Action Items**

| # | Action | Owner | Status |
|---|--------|-------|--------|
| 1 | Update CRD schema (remove `Recommending`, add `SubReason`) | AIAnalysis | ⏳ Pending |
| 2 | Update HolmesGPT client to include `NeedsHumanReview` | AIAnalysis | ⏳ Pending |
| 3 | Update InvestigatingHandler | AIAnalysis | ⏳ Pending |
| 4 | Add failure metrics with sub-reason | AIAnalysis | ⏳ Pending |
| 5 | [HAPI] Consider adding `human_review_reason` enum | HAPI Team | ❓ Pending Response |

---

**Acknowledged By**: AIAnalysis Team
**Date**: 2025-12-06

