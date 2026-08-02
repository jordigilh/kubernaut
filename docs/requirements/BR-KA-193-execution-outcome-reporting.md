# BR-KA-193: Execution Outcome Reporting

**Business Requirement ID**: BR-KA-193
**Category**: Kubernaut Agent (KA)
**Status**: ✅ APPROVED
**Created**: December 2, 2025
**Last Updated**: December 2, 2025

---

## Summary

Kubernaut Agent (KA) MUST report execution outcomes without implementing retry logic. The Remediation Orchestrator (RO) is solely responsible for retry decisions.

---

## Business Need

Clear separation of concerns ensures:

1. **Single Responsibility** - Kubernaut Agent (KA) focuses on analysis, not orchestration
2. **Centralized Retry Policy** - RO owns all retry logic, preventing conflicting behaviors
3. **Auditable Flow** - Each component's responsibility is clearly defined
4. **Flexibility** - RO can adjust retry policies without KA changes

---

## Requirements

### BR-KA-193-001: No Retry Logic in KA

**MUST NOT**: Kubernaut Agent (KA) MUST NOT implement any retry logic for workflow recommendations.

**Rationale**: If KA recommended a workflow and it failed, KA reports the outcome. RO decides whether to retry.

### BR-KA-193-002: Outcome Reporting Structure

**MUST**: Kubernaut Agent (KA) MUST include clear outcome information in responses.

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

### BR-KA-193-003: Recovery Context Consumption

**MUST**: When processing a recovery request (retry scenario), Kubernaut Agent (KA) MUST:
1. Accept the `recoveryContext` from the request
2. Include previous failure context in LLM analysis
3. Return a new recommendation (may be same or different workflow)

**Flow**:
```
1. RO detects workflow failure
2. RO decides to retry (based on RO's retry policy)
3. RO triggers new AIAnalysis with recoveryContext
4. AIAnalysis calls Kubernaut Agent (KA) with recoveryContext
5. Kubernaut Agent (KA) returns new recommendation
6. AIAnalysis/RO proceeds with new recommendation
```

### BR-KA-193-004: Stateless Analysis

**MUST**: Each Kubernaut Agent (KA) request is stateless. KA does not track:
- Previous recommendations for the same incident
- Workflow execution results
- Retry counts

All context needed for recovery analysis MUST be provided in the request via `recoveryContext`.

---

## Retry Decision Matrix

| Scenario | KA Action | RO Action |
|----------|-------------|-----------|
| First analysis | Return recommendation | Create WorkflowExecution |
| Workflow fails (transient) | N/A (not called) | Retry same workflow |
| Workflow fails (persistent) | Re-analyze with recoveryContext | Call AIAnalysis → KA |
| Max retries exceeded | N/A (not called) | Mark as failed, alert |

**Key**: KA is only called when RO decides a **new analysis** is needed, not for simple retries.

---

## Acceptance Criteria

- [ ] Kubernaut Agent (KA) has NO retry logic
- [ ] Each request is stateless (no session tracking)
- [ ] `recoveryContext` is properly consumed when provided
- [ ] Response includes `analysisMetadata` for observability
- [ ] Documentation clearly states retry ownership

---

## Related Documents

- **BR-HAPI-192**: Recovery context consumption (no renamed KA-prefixed doc exists)
- **DD-RECOVERY-002**: Recovery flow design
- **DECISIONS_KA_EXECUTION_RESPONSIBILITIES.md**: Cross-team decision record

---

## Version History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-12-02 | KA Team | Initial creation |

