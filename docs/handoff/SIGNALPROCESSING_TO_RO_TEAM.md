# Questions for Remediation Orchestrator Team

**From**: SignalProcessing Team
**To**: Remediation Orchestrator Team
**Date**: December 1, 2025
**Status**: ✅ **RESOLVED**

---

## Context

SignalProcessing implementation is ready to begin. We have a few clarifying questions to ensure correct integration with RO.

---

## Questions

### Q1: `correlationId` JSON Field Case Change

**Context**: The shared `DeduplicationInfo` type uses:
```go
CorrelationID string `json:"correlationId,omitempty"`  // lowercase 'd'
```

SignalProcessing previously used:
```go
CorrelationID string `json:"correlationID,omitempty"`  // uppercase 'D'
```

**Question**: Is this intentional? It's a breaking JSON change for any existing consumers.

---

### Q2: Recovery Scenario Data Flow

**Question**: Does RO populate `SignalProcessing.spec.failureData` from the failed WorkflowExecution? If so, which fields are copied?

**Current Understanding**:
```
Failed WorkflowExecution.status.error → SignalProcessing.spec.failureData
```

---

### Q3: Authoritative Sequence Diagram

**Question**: Is there an authoritative sequence diagram showing the full flow with field mappings?

```
RemediationRequest → SignalProcessing → AIAnalysis → WorkflowExecution
```

---

## RO Team Response

**Date**: December 1, 2025
**Respondent**: Remediation Orchestrator Team

**Q1 (correlationId case)**:
- [x] **Option A - Intentional** - Document as breaking JSON change

**Notes**: JSON camelCase convention (`correlationId`) vs Go initialism convention (`CorrelationID`). Migration action for SP: Update any existing JSON parsing to use lowercase `correlationId`.

---

**Q2 (Recovery data flow)**:
- [x] **Option A - Confirm understanding**

**RO populates these fields from WorkflowExecution.status when creating recovery SignalProcessing**:

| FailureData Field | Source |
|-------------------|--------|
| `workflowRef` | `we.Name` |
| `attemptNumber` | `we.Status.AttemptNumber` or tracked by RO |
| `failedStep` | `we.Status.FailedStep` |
| `action` | `we.Status.FailedAction` or step metadata |
| `errorType` | `we.Status.Error.Type` |
| `failureReason` | `we.Status.Error.Message` |
| `duration` | `we.Status.Duration` |
| `failedAt` | `we.Status.CompletionTime` |
| `resourceState` | `we.Status.ResourceSnapshot` (if available) |

**RO sets these SignalProcessing.spec fields for recovery**:
- `isRecoveryAttempt: true`
- `recoveryAttemptNumber: N`
- `failedWorkflowRef: {name: <we-name>}`
- `originalProcessingRef: {name: <original-sp-name>}`

---

**Q3 (Sequence diagram)**:
- [x] **Option A - Yes, see DD-CONTRACT-002**

**Authoritative sequence diagram**: `docs/architecture/decisions/DD-CONTRACT-002-service-integration-contracts.md` (v1.2)

**For detailed field mappings per contract**:
- **C1 (Gateway → RO)**: See `API_CONTRACT_TRIAGE.md` GAP-C1-* sections
- **C3 (RO → AIAnalysis)**: See DD-CONTRACT-002 "Contract 1" section
- **C5 (RO → WorkflowExecution)**: See DD-CONTRACT-002 "Contract 2" section
- **Recovery loop**: See [DD-001-recovery-context-enrichment.md](../architecture/decisions/DD-001-recovery-context-enrichment.md)

---

## ✅ Summary

| Question | Answer | Action |
|----------|--------|--------|
| Q1 | A - Intentional | SP updates JSON parsing to `correlationId` |
| Q2 | A - Confirmed | RO populates failureData from WE status |
| Q3 | A - DD-CONTRACT-002 | Review existing docs |

---

## SignalProcessing Team Acknowledgment

**Date**: December 1, 2025
**Respondent**: SignalProcessing Team

### ✅ All Answers Accepted

| Question | Answer | SP Action |
|----------|--------|-----------|
| Q1 | A - Intentional | ✅ **Already implemented** - Using shared type with `correlationId` (lowercase) |
| Q2 | A - Confirmed | ✅ **Noted** - Will implement `FailureData` population per field mapping table |
| Q3 | A - DD-CONTRACT-002 | ✅ **Will review** - No expansion needed at this time |

### Follow-Up Actions

| Action | Owner | Status |
|--------|-------|--------|
| Use shared `DeduplicationInfo` with `correlationId` | SP Team | ✅ Complete |
| Implement `FailureData` struct per RO field mapping | SP Team | 📋 Tracked in Implementation Plan |
| Review DD-CONTRACT-002 for integration patterns | SP Team | 📋 Pre-implementation |
| Review DD-001-recovery-context-enrichment for recovery loop | SP Team | 📋 Pre-implementation |

### Documents to Review

Per RO recommendations:
- [ ] `docs/architecture/decisions/DD-CONTRACT-002-service-integration-contracts.md` (v1.2)
- [ ] `docs/architecture/decisions/DD-001-recovery-context-enrichment.md`
- [ ] `docs/architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md`

### Questions Closed

No further questions for RO team at this time. Thank you for the detailed responses!

---

**Document Version**: 1.1
**Last Updated**: December 2, 2025
**Migrated From**: `docs/services/crd-controllers/01-signalprocessing/QUESTIONS_FOR_RO_TEAM.md`
**Changelog**:
- v1.1: Migrated to `docs/handoff/` as authoritative Q&A directory
- v1.0: Initial document with questions and responses


