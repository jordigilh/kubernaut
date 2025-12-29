# RESPONSE: HolmesGPT-API Team to AIAnalysis Questions

**Date**: 2025-12-06
**From**: HolmesGPT-API Team
**To**: AIAnalysis Team
**In Response To**: [RESPONSE_AIANALYSIS_NEEDS_HUMAN_REVIEW.md](./RESPONSE_AIANALYSIS_NEEDS_HUMAN_REVIEW.md)
**Status**: ✅ **IMPLEMENTATION COMPLETE - READY FOR AIANALYSIS INTEGRATION**

---

## 🔔 IMPLEMENTATION COMPLETE (Dec 6, 2025)

All requested changes have been implemented and are ready for AIAnalysis integration:

| Deliverable | Status |
|-------------|--------|
| `HumanReviewReason` enum | ✅ **DONE** |
| `human_review_reason` field in `IncidentResponse` | ✅ **DONE** |
| Logic to set reason in `incident.py` | ✅ **DONE** |
| OpenAPI spec regenerated | ✅ **18 schemas** |
| Unit tests passing | ✅ **406 tests** |

**AIAnalysis Next Step**: Regenerate Go client and integrate in `InvestigatingHandler`:

```bash
# Regenerate Go client with new HumanReviewReason enum
ogen -package holmesgpt -target pkg/clients/holmesgpt \
    holmesgpt-api/api/openapi.json

# Verify new types
grep -l "HumanReviewReason" pkg/clients/holmesgpt/*.go
```

---

## 📋 **Summary**

We acknowledge your integration approach and provide answers to your questions below.

---

## ✅ **Architecture Clarification Acknowledged**

We understand and accept your 4-phase architecture:

| Our Notice | Your Reality | Status |
|------------|--------------|--------|
| `RecommendingHandler` | `InvestigatingHandler` | ✅ Understood |
| `PhaseManualReviewRequired` | `Failed` + `SubReason` | ✅ Accepted |
| 5-phase flow | 4-phase flow | ✅ Understood |

Your approach using `Failed` phase with structured `reason` + `subReason` is cleaner than adding a new phase. We support this design.

---

## ❓ **Answers to Your Questions**

### A1: Warning-to-SubReason Mapping

**Your Preference**: Option B (structured error code)

**Our Answer**: ✅ **APPROVED - We will add `human_review_reason` field**

We agree that parsing warning text is fragile. We will add a new enum field to `IncidentResponse`:

```python
class HumanReviewReason(str, Enum):
    """Structured reason for needs_human_review=true"""
    WORKFLOW_NOT_FOUND = "workflow_not_found"
    IMAGE_MISMATCH = "image_mismatch"
    PARAMETER_VALIDATION_FAILED = "parameter_validation_failed"
    NO_MATCHING_WORKFLOWS = "no_matching_workflows"
    LOW_CONFIDENCE = "low_confidence"
    LLM_PARSING_ERROR = "llm_parsing_error"

class IncidentResponse(BaseModel):
    # ... existing fields ...
    needs_human_review: bool = Field(default=False)
    human_review_reason: Optional[HumanReviewReason] = Field(
        default=None,
        description="Structured reason when needs_human_review=true. "
                    "Use this for reliable subReason mapping instead of parsing warnings."
    )
```

**Updated Response Example**:
```json
{
  "needs_human_review": true,
  "human_review_reason": "workflow_not_found",
  "warnings": ["Workflow 'restart-pod-v1' not found in catalog"]
}
```

**Your Mapping**:
```go
func (h *InvestigatingHandler) mapToSubReason(reason string) string {
    mapping := map[string]string{
        "workflow_not_found":           "WorkflowNotFound",
        "image_mismatch":               "ImageMismatch",
        "parameter_validation_failed":  "ParameterValidationFailed",
        "no_matching_workflows":        "NoMatchingWorkflows",
        "low_confidence":               "LowConfidence",
        "llm_parsing_error":            "LLMParsingError",
    }
    return mapping[reason]
}
```

**Timeline**: We will implement this by EOD Dec 6, 2025.

---

### A2: Partial Response Preservation

**Your Assumption**: Store everything for operator context

**Our Answer**: ✅ **CORRECT - Store all available data**

When `needs_human_review=true`, we will still provide:

| Field | Availability | Purpose |
|-------|--------------|---------|
| `selected_workflow` | ✅ If LLM provided one | Operator can see what AI attempted |
| `selected_workflow.validation_errors` | ✅ If validation failed | Specific errors for debugging |
| `root_cause_analysis` | ✅ Always | RCA is still valuable |
| `alternative_workflows` | ✅ If available | Additional context |
| `confidence` | ✅ Always | Even if low |
| `warnings` | ✅ Always | Human-readable details |

**Example Response with Partial Data**:
```json
{
  "incident_id": "inc-123",
  "needs_human_review": true,
  "human_review_reason": "parameter_validation_failed",
  "warnings": [
    "Parameter validation failed: Missing required parameter 'namespace'"
  ],
  "root_cause_analysis": {
    "summary": "Pod OOMKilled due to memory limit exceeded",
    "severity": "high",
    "contributing_factors": ["memory_limit_too_low", "memory_leak"]
  },
  "selected_workflow": {
    "workflow_id": "restart-pod-v1",
    "confidence": 0.85,
    "parameters": {
      "delay_seconds": 30
    },
    "validation_errors": [
      "Missing required parameter: 'namespace'"
    ]
  },
  "confidence": 0.85,
  "alternative_workflows": [...]
}
```

**Your Status Mapping**:
```yaml
status:
  phase: Failed
  reason: WorkflowResolutionFailed
  subReason: ParameterValidationFailed
  message: "Parameter validation failed: Missing required parameter 'namespace'"
  # Store partial response in status for operator context
  selectedWorkflow:
    workflowId: restart-pod-v1
    confidence: 0.85
    validationErrors:
      - "Missing required parameter: 'namespace'"
  rootCauseAnalysis:
    summary: "Pod OOMKilled due to memory limit exceeded"
    severity: high
```

---

## 🔄 **Updated API Contract**

Based on your feedback, the updated `IncidentResponse` will be:

```json
{
  "incident_id": "string",
  "analysis": "string",
  "root_cause_analysis": {...},
  "selected_workflow": {...},
  "confidence": 0.85,
  "timestamp": "2025-12-06T10:00:00Z",
  "target_in_owner_chain": true,
  "warnings": ["..."],
  "needs_human_review": false,
  "human_review_reason": null,        // ← NEW FIELD (enum or null)
  "alternative_workflows": [...]
}
```

**OpenAPI Schema** (to be regenerated):
```yaml
HumanReviewReason:
  type: string
  enum:
    - workflow_not_found
    - image_mismatch
    - parameter_validation_failed
    - no_matching_workflows
    - low_confidence
    - llm_parsing_error
  nullable: true
```

---

## 📅 **Timeline**

| Action | Owner | Target | Status |
|--------|-------|--------|--------|
| Add `human_review_reason` enum | HAPI | Dec 6, 2025 | ✅ **DONE** |
| Regenerate OpenAPI spec | HAPI | Dec 6, 2025 | ✅ **DONE** (18 schemas) |
| Update BR-HAPI-197 | HAPI | Dec 6, 2025 | ✅ **DONE** |
| Regenerate Go client | AIAnalysis | After HAPI | ⏳ **READY** |
| Update InvestigatingHandler | AIAnalysis | TBD | ⏳ Pending |

---

## ✅ **Action Items**

| # | Action | Owner | Status |
|---|--------|-------|--------|
| 1 | Implement `human_review_reason` field | HAPI | ✅ **DONE** |
| 2 | Update `incident.py` to set reason | HAPI | ✅ **DONE** |
| 3 | Regenerate OpenAPI spec | HAPI | ✅ **DONE** |
| 4 | Update notice documents | HAPI | ✅ **DONE** |
| 5 | Regenerate Go client | AIAnalysis | ⏳ **READY** - HAPI complete |
| 6 | Update InvestigatingHandler | AIAnalysis | ⏳ Pending |

---

**Responded By**: HolmesGPT-API Team
**Date**: 2025-12-06

