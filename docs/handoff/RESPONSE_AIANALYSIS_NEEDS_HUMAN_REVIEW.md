# RESPONSE: AIAnalysis Integration of `needs_human_review` Field

**Date**: 2025-12-06
**Updated**: 2025-12-07
**From**: AIAnalysis Team
**To**: HolmesGPT-API Team
**In Response To**: [NOTICE_NEEDS_HUMAN_REVIEW_FIELD.md](./NOTICE_NEEDS_HUMAN_REVIEW_FIELD.md)
**Status**: ✅ **INTEGRATION COMPLETE**

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
| Update CRD schema | Day 5 | ✅ **Complete** (removed Recommending, added SubReason enum) |
| Update HolmesGPT client | Day 5 | ✅ **Complete** (NeedsHumanReview, HumanReviewReason fields) |
| Update InvestigatingHandler | Day 5-8 | ✅ **Complete** (mapEnumToSubReason, handleWorkflowResolutionFailure, handleProblemResolved) |
| Add metrics | Day 5 | ✅ **Complete** (aianalysis_failures_total with sub_reason label) |
| Unit tests | Day 6-8 | ✅ **Complete** (163 tests, 87.6% coverage) |

---

## ✅ **HAPI Team Answers** (2025-12-06)

**Reference**: [RESPONSE_HAPI_TO_AIANALYSIS_NEEDS_HUMAN_REVIEW.md](./RESPONSE_HAPI_TO_AIANALYSIS_NEEDS_HUMAN_REVIEW.md)

### A1: Warning-to-SubReason Mapping

**HAPI Answer**: ✅ **Option B APPROVED** - `human_review_reason` enum field added

```python
class HumanReviewReason(str, Enum):
    WORKFLOW_NOT_FOUND = "workflow_not_found"
    IMAGE_MISMATCH = "image_mismatch"
    PARAMETER_VALIDATION_FAILED = "parameter_validation_failed"
    NO_MATCHING_WORKFLOWS = "no_matching_workflows"
    LOW_CONFIDENCE = "low_confidence"
    LLM_PARSING_ERROR = "llm_parsing_error"
    INVESTIGATION_INCONCLUSIVE = "investigation_inconclusive"  # BR-HAPI-200
```

**AIAnalysis Implementation** (complete):
```go
// pkg/aianalysis/handlers/investigating.go - mapEnumToSubReason()
mapping := map[string]string{
    "workflow_not_found":          "WorkflowNotFound",
    "image_mismatch":              "ImageMismatch",
    "parameter_validation_failed": "ParameterValidationFailed",
    "no_matching_workflows":       "NoMatchingWorkflows",
    "low_confidence":              "LowConfidence",
    "llm_parsing_error":           "LLMParsingError",
    "investigation_inconclusive":  "InvestigationInconclusive",
}
```

### A2: Partial Response Preservation

**HAPI Answer**: ✅ **CONFIRMED** - Store all available data for operator context

| Field | Availability | Purpose |
|-------|--------------|---------|
| `selected_workflow` | ✅ If LLM provided one | Operator can see what AI attempted |
| `root_cause_analysis` | ✅ Always | RCA is still valuable |
| `alternative_workflows` | ✅ If available | Additional context |
| `confidence` | ✅ Always | Even if low |
| `warnings` | ✅ Always | Human-readable details |

**AIAnalysis Implementation** (complete): `handleWorkflowResolutionFailure()` preserves all partial data.

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
| 1 | Update CRD schema (remove `Recommending`, add `SubReason`) | AIAnalysis | ✅ **Complete** |
| 2 | Update HolmesGPT client to include `NeedsHumanReview` | AIAnalysis | ✅ **Complete** |
| 3 | Update InvestigatingHandler | AIAnalysis | ✅ **Complete** |
| 4 | Add failure metrics with sub-reason | AIAnalysis | ✅ **Complete** |
| 5 | [HAPI] Add `human_review_reason` enum | HAPI Team | ✅ **Complete** (Dec 6, 2025) |
| 6 | [BR-HAPI-200] Add `investigation_inconclusive` enum | HAPI Team | ✅ **Complete** (Dec 7, 2025) |
| 7 | [BR-HAPI-200] Handle "Problem Resolved" outcome | AIAnalysis | ✅ **Complete** (Dec 7, 2025) |

---

**Acknowledged By**: AIAnalysis Team
**Date**: 2025-12-06
**Integration Complete**: 2025-12-07

---

## 📝 Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| v1.0 | 2025-12-06 | AIAnalysis Team | Initial acknowledgment, questions Q1/Q2 |
| v1.1 | 2025-12-06 | HAPI Team | Answered Q1 (enum approved), Q2 (preserve partial) |
| v2.0 | 2025-12-07 | AIAnalysis Team | All action items complete, BR-HAPI-200 integration |

