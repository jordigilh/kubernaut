# BR-HAPI-193: Execution Outcome Reporting

**Business Requirement ID**: BR-HAPI-193
**Category**: HAPI (HolmesGPT-API)
**Status**: ✅ APPROVED
**Created**: December 2, 2025
**Last Updated**: December 2, 2025

---

## Summary

HolmesGPT-API MUST report execution outcomes without implementing retry logic. The Remediation Orchestrator (RO) is solely responsible for retry decisions.

---

## Business Need

Clear separation of concerns ensures:

1. **Single Responsibility** - HolmesGPT-API focuses on analysis, not orchestration
2. **Centralized Retry Policy** - RO owns all retry logic, preventing conflicting behaviors
3. **Auditable Flow** - Each component's responsibility is clearly defined
4. **Flexibility** - RO can adjust retry policies without HAPI changes

---

## Requirements

### BR-HAPI-193-001: No Retry Logic in HAPI

**MUST NOT**: HolmesGPT-API MUST NOT implement any retry logic for workflow recommendations.

**Rationale**: If HAPI recommended a workflow and it failed, HAPI reports the outcome. RO decides whether to retry.

### BR-HAPI-193-002: Outcome Reporting Structure

**MUST**: HolmesGPT-API MUST include clear outcome information in responses.

**Response Schema**:
```json
{
  "investigationId": "inv-2025-12-02-abc123",
  "status": "completed",
  "selectedWorkflow": {
    "workflowId": "wf-memory-increase-001",
    "confidence": 0.90,
    "rationale": "Selected based on 90% semantic similarity..."
  },
  "analysisMetadata": {
    "processingTimeMs": 1234,
    "llmTokensUsed": 2500,
    "workflowCandidatesEvaluated": 5
  }
}
```

### BR-HAPI-193-003: Recovery Context Consumption

**MUST**: When processing a recovery request (retry scenario), HolmesGPT-API MUST:
1. Accept the `recoveryContext` from the request
2. Include previous failure context in LLM analysis
3. Return a new recommendation (may be same or different workflow)

**Flow**:
```
1. RO detects workflow failure
2. RO decides to retry (based on RO's retry policy)
3. RO triggers new AIAnalysis with recoveryContext
4. AIAnalysis calls HolmesGPT-API with recoveryContext
5. HolmesGPT-API returns new recommendation
6. AIAnalysis/RO proceeds with new recommendation
```

### BR-HAPI-193-004: Stateless Analysis

**MUST**: Each HolmesGPT-API request is stateless. HAPI does not track:
- Previous recommendations for the same incident
- Workflow execution results
- Retry counts

All context needed for recovery analysis MUST be provided in the request via `recoveryContext`.

---

## Retry Decision Matrix

| Scenario | HAPI Action | RO Action |
|----------|-------------|-----------|
| First analysis | Return recommendation | Create WorkflowExecution |
| Workflow fails (transient) | N/A (not called) | Retry same workflow |
| Workflow fails (persistent) | Re-analyze with recoveryContext | Call AIAnalysis → HAPI |
| Max retries exceeded | N/A (not called) | Mark as failed, alert |

**Key**: HAPI is only called when RO decides a **new analysis** is needed, not for simple retries.

---

## Acceptance Criteria

- [ ] HolmesGPT-API has NO retry logic
- [ ] Each request is stateless (no session tracking)
- [ ] `recoveryContext` is properly consumed when provided
- [ ] Response includes `analysisMetadata` for observability
- [ ] Documentation clearly states retry ownership

---

## Related Documents

- **BR-HAPI-192**: Recovery context consumption
- **DD-RECOVERY-002**: Recovery flow design
- **DECISIONS_HAPI_EXECUTION_RESPONSIBILITIES.md**: Cross-team decision record

---

## Version History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-12-02 | HAPI Team | Initial creation |

