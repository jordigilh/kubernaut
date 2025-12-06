# BR-HAPI-197: Human Review Required Flag

**ID**: BR-HAPI-197
**Title**: Human Review Required Flag for AI Reliability Issues
**Category**: HAPI (HolmesGPT-API)
**Priority**: 🔴 HIGH
**Version**: V1.0
**Status**: ✅ APPROVED
**Date**: December 6, 2025

---

## Business Context

### Problem Statement

When HolmesGPT-API analyzes an incident, there are scenarios where the AI cannot produce a reliable result:

1. **Workflow validation failures**: LLM selects a workflow that doesn't exist or has invalid parameters
2. **No suitable workflows**: Search returns no matching workflows for the incident type
3. **Low confidence**: AI has low confidence in its recommendation
4. **Parsing failures**: LLM response cannot be parsed into structured format

In these cases, **automatic remediation should NOT proceed**. A human operator must review the situation and decide the appropriate action.

### Business Value

| Benefit | Impact |
|---------|--------|
| **Safety** | Prevents execution of unreliable AI recommendations |
| **Transparency** | Clear signal to operators when human intervention is needed |
| **Audit Trail** | Documented reason for manual review requirement |
| **Cost Savings** | Avoids failed workflow executions from bad AI recommendations |

---

## Requirements

### BR-HAPI-197.1: Field Definition

**MUST**: HolmesGPT-API SHALL provide a `needs_human_review` boolean field in the `IncidentResponse` schema.

```json
{
  "needs_human_review": {
    "type": "boolean",
    "default": false
  }
}
```

### BR-HAPI-197.2: Field Semantics

**MUST**: The `needs_human_review` field SHALL be `true` when ANY of the following conditions occur:

| Condition | Trigger |
|-----------|---------|
| **Workflow Not Found** | Selected `workflow_id` does not exist in catalog |
| **Container Image Mismatch** | LLM-provided image doesn't match catalog |
| **Parameter Validation Failed** | Parameters don't conform to workflow schema |
| **No Workflows Matched** | Workflow search returned no results |
| **Low Confidence** | Overall confidence is below threshold (threshold owned by AIAnalysis) |
| **LLM Parsing Error** | Cannot extract structured data from LLM response |

### BR-HAPI-197.3: Warning Correlation

**MUST**: When `needs_human_review` is `true`, the `warnings` field SHALL contain at least one message explaining why human review is required.

**Example**:
```json
{
  "needs_human_review": true,
  "warnings": [
    "Workflow validation failed: workflow 'restart-pod-v1' not found in catalog",
    "Please select a valid workflow from the catalog"
  ]
}
```

### BR-HAPI-197.4: Workflow Preservation

**SHOULD**: When `needs_human_review` is `true` due to validation failures, the `selected_workflow` field SHOULD still contain the LLM's recommendation for operator context.

**Rationale**: Operators may want to see what the AI attempted to recommend, even if it was invalid.

### BR-HAPI-197.5: Validation Errors

**SHOULD**: When workflow validation fails, the `selected_workflow` object SHOULD contain a `validation_errors` array with specific error messages.

```json
{
  "selected_workflow": {
    "workflow_id": "restart-pod-v1",
    "confidence": 0.85,
    "validation_errors": [
      "Missing required parameter: 'namespace'",
      "Parameter 'delay': must be >= 0, got -5"
    ]
  },
  "needs_human_review": true
}
```

---

## Consumer Behavior Requirements

### BR-HAPI-197.6: AIAnalysis MUST NOT Create WorkflowExecution

**MUST**: When `needs_human_review` is `true`, the AIAnalysis controller MUST NOT create a WorkflowExecution CRD.

**Rationale**: Automatic execution of unreliable AI recommendations could cause harm to the cluster or workloads.

### BR-HAPI-197.7: AIAnalysis Phase Transition

**SHOULD**: When `needs_human_review` is `true`, the AIAnalysis controller SHOULD transition to a `ManualReviewRequired` phase (or equivalent).

**Example Status**:
```yaml
status:
  phase: ManualReviewRequired
  failureReason: AIReviewRequired
  message: "Workflow validation failed: workflow 'restart-pod-v1' not found in catalog"
```

### BR-HAPI-197.8: Audit Trail

**MUST**: When `needs_human_review` is `true`, the consuming service MUST log the event for audit purposes.

**Required Log Fields**:
- `incident_id`
- `remediation_id`
- `reason` (from warnings)
- `timestamp`

### BR-HAPI-197.9: Metrics Emission

**SHOULD**: Consuming services SHOULD emit metrics when `needs_human_review` is `true`.

```prometheus
kubernaut_aianalysis_human_review_required_total{
  reason="workflow_validation_failed|no_workflows_matched|low_confidence|parsing_error"
}
```

---

## Operator Actions

### BR-HAPI-197.10: Manual Review Options

When `needs_human_review` is `true`, operators SHALL have the following options:

| Action | Description | Outcome |
|--------|-------------|---------|
| **Approve with modifications** | Review AI recommendation, fix issues, manually create WorkflowExecution | Remediation proceeds with corrected parameters |
| **Reject** | Determine AI recommendation is not appropriate | Incident marked as rejected, no remediation |
| **Retry** | Trigger re-analysis (useful for transient LLM issues) | New analysis attempt |
| **Escalate** | Forward to human remediation team | Manual investigation and remediation |

---

## Non-Requirements

### What This BR Does NOT Cover

1. **LLM Self-Correction Loop**: In-session retry mechanism (future enhancement)
2. **Automatic Retry**: System automatically retrying analysis (requires separate BR)
3. **Approval Workflow UI**: User interface for manual review (separate product feature)

---

## Design Decision Reference

**DD-HAPI-002 v1.2**: Workflow Response Validation Architecture

This BR implements the business behavior for the `needs_human_review` flag defined in DD-HAPI-002 v1.2.

---

## Acceptance Criteria

### AC-1: Field Present in API Response

```gherkin
Given an IncidentRequest is submitted to HolmesGPT-API
When the API returns an IncidentResponse
Then the response SHALL contain a "needs_human_review" boolean field
```

### AC-2: True When Workflow Validation Fails

```gherkin
Given an IncidentRequest is submitted
And the LLM selects a workflow_id that doesn't exist in the catalog
When the API validates the workflow
Then "needs_human_review" SHALL be true
And "warnings" SHALL contain "Workflow validation failed"
```

### AC-3: True When No Workflows Match

```gherkin
Given an IncidentRequest is submitted
And the workflow catalog search returns no results
When the API returns a response
Then "needs_human_review" SHALL be true
And "warnings" SHALL contain "No workflows matched"
```

### AC-4: Confidence Returned for Consumer Decision

```gherkin
Given an IncidentRequest is submitted
When the API returns a response
Then the "selected_workflow.confidence" field SHALL contain the AI confidence score (0.0-1.0)
And the consuming service (AIAnalysis) SHALL apply its configured threshold
```

**Note**: HAPI returns `confidence` but does NOT enforce thresholds. AIAnalysis owns the threshold logic (V1.0: global 70% default, V1.1: operator-configurable).

### AC-5: False When Validation Passes

```gherkin
Given an IncidentRequest is submitted
And workflow validation passes (exists, image matches, parameters valid)
When the API returns a response
Then "needs_human_review" SHALL be false
And "warnings" SHALL be empty or contain only informational messages
```

**Note**: `needs_human_review` is only set by HAPI for validation failures, not confidence thresholds.

---

## Test Coverage

| Test Category | Test Count | Coverage |
|---------------|------------|----------|
| Unit Tests (HolmesGPT-API) | 21 | Validation logic |
| Integration Tests | TBD | End-to-end flow |
| E2E Tests | TBD | Full system behavior |

---

## Implementation Status

| Component | Status | Notes |
|-----------|--------|-------|
| HolmesGPT-API Model | ✅ Complete | `IncidentResponse.needs_human_review` |
| HolmesGPT-API Logic | ✅ Complete | Set based on validation/confidence |
| OpenAPI Spec | ✅ Complete | 17 schemas |
| AIAnalysis Handler | ⏳ Pending | Requires AIAnalysis team |
| Metrics | ⏳ Pending | Requires AIAnalysis team |

---

## Related Documents

- [DD-HAPI-002 v1.2: Workflow Response Validation Architecture](../architecture/decisions/DD-HAPI-002-workflow-parameter-validation.md)
- [NOTICE_NEEDS_HUMAN_REVIEW_FIELD.md](../handoff/NOTICE_NEEDS_HUMAN_REVIEW_FIELD.md)
- [AIANALYSIS_TO_HOLMESGPT_API_TEAM.md](../handoff/AIANALYSIS_TO_HOLMESGPT_API_TEAM.md)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-06 | Initial business requirement |

