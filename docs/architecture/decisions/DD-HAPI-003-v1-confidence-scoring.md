# DD-HAPI-003: V1.0 Confidence Scoring Methodology

**Design Decision ID**: DD-HAPI-003
**Status**: ✅ APPROVED
**Version**: 1.0
**Created**: December 2, 2025
**Last Updated**: December 2, 2025

---

## Context & Problem

HolmesGPT-API needs to return a confidence score for workflow recommendations. The question is: **what factors should contribute to this confidence score in V1.0?**

**Key Requirements**:
- Score must be meaningful for AIAnalysis Rego policy evaluation
- Score must be explainable in the `rationale` field
- Implementation must be achievable in V1.0 timeline

---

## Decision

### V1.0 Confidence Scoring: Semantic Similarity + Label Matching ONLY

**APPROVED**: V1.0 confidence scoring uses only two factors:

1. **Semantic Similarity** (primary factor)
   - Cosine similarity between incident embedding and workflow description embedding
   - Range: 0.0 to 1.0
   - Weight: 70% of final score

2. **Label Matching** (secondary factor)
   - Percentage of `DetectedLabels` that match workflow requirements
   - Range: 0.0 to 1.0
   - Weight: 30% of final score

**Formula**:
```
confidence = (semantic_similarity * 0.7) + (label_match_ratio * 0.3)
```

---

## What is NOT Included in V1.0

### Historical Success Rate

**DECISION**: Historical success rate is **NOT** used in V1.0 confidence scoring.

**Rationale**:
1. **No execution data yet** - V1.0 is initial deployment, no historical data exists
2. **Circular dependency** - Can't have success rates without executions
3. **Complexity** - Requires execution tracking, feedback loop, statistical significance
4. **Scope creep** - Adds significant implementation effort to V1.0

### Historical Success Rate: Human Consumption Only

**CLARIFICATION**: When historical data becomes available (post-V1.0), it will be:

- **Displayed to operators** in dashboards and approval notifications
- **NOT used by LLM** for workflow selection
- **NOT included in confidence score calculation**

**Why not use for scoring?**
- Past success doesn't guarantee future success (different conditions)
- Creates bias toward frequently-used workflows
- Newer (potentially better) workflows disadvantaged
- Semantic similarity is a better predictor of applicability

---

## Rationale Field Format

The `rationale` field in responses MUST reflect the V1.0 methodology:

**✅ CORRECT V1.0 Rationale**:
```json
{
  "rationale": "Selected based on 90% semantic similarity for OOMKilled signal pattern with matching GitOps and service mesh labels"
}
```

**❌ INCORRECT (references historical success)**:
```json
{
  "rationale": "Selected based on 92% historical success rate for OOMKilled signals"
}
```

---

## Confidence Thresholds

AIAnalysis uses confidence scores for approval decisions (via Rego policies):

| Confidence | AIAnalysis Behavior |
|------------|---------------------|
| ≥ 0.80 | `approvalRequired = false` (auto-execute in non-prod) |
| < 0.80 | `approvalRequired = true` (requires approval) |
| Production | **Always** `approvalRequired = true` (regardless of confidence) |

---

## Future Considerations (Post-V1.0)

For V1.1+, consider adding:
- **Workflow complexity score** - Simpler workflows may be safer
- **Parameter completeness** - Are all required parameters extractable?
- **Time-of-day factors** - Business hours vs. off-hours
- **Operator feedback integration** - Learn from approval/rejection patterns

**NOT planned**:
- Historical success rate in confidence scoring (see rationale above)

---

## Implementation

### Confidence Calculation (Python)

```python
def calculate_confidence(
    semantic_similarity: float,
    detected_labels: DetectedLabels,
    workflow_requirements: WorkflowRequirements
) -> float:
    """
    Calculate V1.0 confidence score.

    Args:
        semantic_similarity: Cosine similarity (0.0-1.0) from embedding comparison
        detected_labels: Auto-detected cluster characteristics
        workflow_requirements: Workflow's label requirements

    Returns:
        Confidence score (0.0-1.0)
    """
    # Semantic similarity: 70% weight
    semantic_component = semantic_similarity * 0.7

    # Label matching: 30% weight
    label_match_ratio = calculate_label_match_ratio(detected_labels, workflow_requirements)
    label_component = label_match_ratio * 0.3

    return semantic_component + label_component


def calculate_label_match_ratio(
    detected: DetectedLabels,
    requirements: WorkflowRequirements
) -> float:
    """Calculate percentage of required labels that match."""
    if not requirements.required_labels:
        return 1.0  # No requirements = 100% match

    matches = 0
    total = len(requirements.required_labels)

    for label, required_value in requirements.required_labels.items():
        detected_value = getattr(detected, label, None)
        if detected_value == required_value:
            matches += 1

    return matches / total if total > 0 else 1.0
```

---

## Alternatives Considered

### Alternative 1: Include Historical Success Rate

**Rejected** - No historical data in V1.0, adds complexity, creates workflow bias.

### Alternative 2: LLM Self-Assessed Confidence

**Rejected** - LLM confidence is unreliable, not reproducible, hard to threshold.

### Alternative 3: Static Confidence (Always 0.85)

**Rejected** - Not useful for Rego policies, doesn't differentiate recommendations.

---

## Consequences

**Positive**:
- ✅ Simple, explainable methodology
- ✅ Achievable in V1.0 timeline
- ✅ Provides meaningful differentiation for Rego policies
- ✅ No dependency on historical data

**Negative**:
- ⚠️ Doesn't leverage execution feedback (future enhancement)
- ⚠️ May over-weight semantic similarity

**Mitigation**: V1.1 can add additional factors once V1.0 is deployed and data is collected.

---

## Related Documents

- **BR-AI-050**: Confidence thresholds for approval
- **DD-WORKFLOW-001**: Mandatory label schema
- **RESPONSE_TO_AIANALYSIS_TEAM.md**: API contract details

---

## Version History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-12-02 | HAPI Team | Initial creation - V1.0 methodology defined |

