# BR-ORCH-027/028: Timeout Management

**Service**: RemediationOrchestrator Controller
**Category**: Timeout Management
**Priority**: P0/P1 (CRITICAL/HIGH)
**Version**: 1.0
**Date**: 2025-12-02
**Status**: 🚧 Planned
**Design Decision**: [DD-TIMEOUT-001-global-remediation-timeout.md](../architecture/decisions/DD-TIMEOUT-001-global-remediation-timeout.md)

---

## Overview

This document consolidates two related business requirements for timeout management in RemediationOrchestrator:
1. **BR-ORCH-027** (P0): Global remediation timeout (default: 1 hour)
2. **BR-ORCH-028** (P1): Per-phase timeouts for faster detection

**Key Design Decision**: All remediations MUST reach a terminal state. Timeout enforcement prevents stuck remediations from consuming resources indefinitely.

---

## BR-ORCH-027: Global Remediation Timeout

### Description

RemediationOrchestrator MUST enforce a global timeout (default: 1 hour) for the entire remediation lifecycle, preventing stuck remediations from consuming resources indefinitely.

### Priority

**P0 (CRITICAL)** - Without global timeout, stuck remediations never terminate

### Rationale

Stuck remediations can occur due to:
- Hung HolmesGPT investigations
- Unresponsive approvers (beyond approval timeout)
- Stuck Tekton pipelines
- Network partitions

Without global timeout:
- Resources consumed indefinitely
- Alert correlation confusion
- No clear termination signal for dependent systems
- Monitoring blind spots

### Implementation

1. Default global timeout: 1 hour (configurable via ConfigMap)
2. Check on every reconciliation: `time.Since(creationTimestamp) > globalTimeout`
3. On timeout:
   - Set `status.phase = "Timeout"`
   - Set `status.timeoutTime = now()`
   - Set `status.timeoutPhase = currentPhase`
   - Create NotificationRequest for escalation
4. Per-remediation override via `status.timeoutConfig.overallWorkflowTimeout`

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-027-1 | Remediations exceeding global timeout marked as Timeout | Unit, Integration |
| AC-027-2 | NotificationRequest created on timeout | Unit |
| AC-027-3 | Default timeout configurable via ConfigMap | Integration |
| AC-027-4 | Per-remediation override supported | Unit |
| AC-027-5 | Timeout phase tracked in status | Unit |

### Test Scenarios

```gherkin
Scenario: Global timeout triggers
  Given RemediationRequest "rr-1" was created 61 minutes ago
  And default global timeout is 60 minutes
  When RemediationOrchestrator reconciles "rr-1"
  Then RemediationRequest phase should be "Timeout"
  And status.timeoutTime should be set
  And NotificationRequest should be created for escalation

Scenario: Per-remediation timeout override
  Given RemediationRequest "rr-1" has status.timeoutConfig.overallWorkflowTimeout = 2h
  And RemediationRequest was created 90 minutes ago
  When RemediationOrchestrator reconciles "rr-1"
  Then RemediationRequest phase should NOT be "Timeout"
  And remediation should continue normally
```

---

## BR-ORCH-028: Per-Phase Timeouts

### Description

RemediationOrchestrator MUST enforce per-phase timeouts to detect stuck individual phases without waiting for global timeout, enabling faster detection of phase-specific issues.

### Priority

**P1 (HIGH)** - Improves MTTR by failing fast on phase-specific issues

### Rationale

Per-phase timeouts enable:
- Faster detection of phase-specific issues (e.g., hung AIAnalysis)
- More precise timeout reasons in audit trail
- Earlier escalation for specific components
- Granular SLO tracking per phase

### Implementation

Default phase timeouts:
| Phase | Default Timeout | Rationale |
|-------|-----------------|-----------|
| SignalProcessing | 5 minutes | Quick enrichment |
| AIAnalysis | 10 minutes | HolmesGPT investigation |
| Approval | 15 minutes | Per ADR-040 |
| WorkflowExecution | 30 minutes | Tekton pipeline execution |

Implementation:
1. Track phase start time in `status.phaseTransitions`
2. Check on reconciliation: `time.Since(phaseStart) > phaseTimeout`
3. On phase timeout:
   - Set `status.phase = "Timeout"`
   - Set `status.timeoutPhase = currentPhaseName`
   - Create targeted escalation notification

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-028-1 | Each phase has configurable timeout | Unit |
| AC-028-2 | Phase timeout triggers before global timeout | Integration |
| AC-028-3 | Phase start times tracked in status | Unit |
| AC-028-4 | Timeout reason indicates which phase timed out | Unit |
| AC-028-5 | Per-remediation phase timeout override supported | Unit |

### Test Scenarios

```gherkin
Scenario: AIAnalysis phase timeout
  Given RemediationRequest "rr-1" is in phase "analyzing"
  And AIAnalysis was created 11 minutes ago
  And default AIAnalysis timeout is 10 minutes
  When RemediationOrchestrator reconciles "rr-1"
  Then RemediationRequest phase should be "Timeout"
  And status.timeoutPhase should be "ai_analysis"
  And NotificationRequest should contain "AIAnalysis phase timed out"

Scenario: Phase timeout before global timeout
  Given RemediationRequest "rr-1" was created 15 minutes ago
  And RemediationRequest is stuck in "analyzing" for 12 minutes
  And AIAnalysis timeout is 10 minutes
  And global timeout is 60 minutes
  When RemediationOrchestrator reconciles "rr-1"
  Then RemediationRequest should timeout due to phase timeout
  And timeout should NOT wait for global timeout
```

---

## Escalation Channels

Per-phase escalation routing (from DD-TIMEOUT-001):

| Phase | Default Timeout | Escalation Channel |
|-------|----------------|-------------------|
| **SignalProcessing** | 5 minutes | Slack: #platform-ops |
| **AIAnalysis** | 10 minutes | Slack: #ai-team, Email: ai-oncall |
| **Approval** | 15 minutes | Slack: #approvers |
| **WorkflowExecution** | 30 minutes | Slack: #sre-team |
| **Overall Workflow** | 1 hour | Slack: #incident-response, PagerDuty: P1 |

---

## Related Documents

- [DD-TIMEOUT-001: Global Remediation Timeout Strategy](../architecture/decisions/DD-TIMEOUT-001-global-remediation-timeout.md)
- [ADR-040: RemediationApprovalRequest (approval timeout: 15 minutes)](../architecture/decisions/ADR-040-remediation-approval-request-architecture.md)

---

**Document Version**: 1.0
**Last Updated**: December 2, 2025
**Maintained By**: Kubernaut Architecture Team


