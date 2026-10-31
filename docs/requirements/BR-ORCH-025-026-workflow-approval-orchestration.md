# BR-ORCH-025/026: Workflow Data Pass-Through and Approval Orchestration

> **Proposed approval-path amendment (#2570; pending ADR-076/DD-AF-017 review and merge):**
> For human-approved execution, RO consumes the immutable selection and target
> snapshot recorded in the RAR, with only admitted overrides to existing
> parameter values. Omitted override keys retain their original values; no
> workflow switch or live catalog/AIA re-resolution is permitted. Every WE,
> including automatic non-approval execution, performs the trusted-namespace,
> point-in-time workflow-ID and content-identity check before dispatch; absent,
> ambiguous or mismatched workflows fail closed without creating an external
> execution resource. A WE CR can already exist when this dispatch check fails.
> The check does not replace frozen snapshot values with live catalog data. RO's
> AIA selection pass-through and routing remain unchanged for the non-approval
> path, which nevertheless gains this WE runtime check. This amendment is proposed, not an assertion that the
> current implementation or compliance evidence already satisfies it.

**Service**: RemediationOrchestrator Controller
**Category**: Workflow Data Pass-Through
**Priority**: P0 (CRITICAL)
**Version**: 1.0
**Date**: 2025-12-02
**Status**: 🚧 Planned
**Design Decision**: [DD-CONTRACT-001-aianalysis-workflowexecution-alignment.md](../architecture/decisions/DD-CONTRACT-001-aianalysis-workflowexecution-alignment.md)

---

## Overview

This document consolidates two related business requirements for workflow data handling in RemediationOrchestrator:
1. **BR-ORCH-025**: Pass through workflow data from AIAnalysis to WorkflowExecution (no catalog lookup)
2. **BR-ORCH-026**: Orchestrate approval workflow when AIAnalysis requires human approval

**Key Design Decision**: RO does NOT perform catalog lookups. Kubernaut Agent (KA) resolves `workflow_id → executionBundle` during workflow discovery. RO passes through from `AIAnalysis.status.rcaResult.selectedWorkflow`.

---

## BR-ORCH-025: Workflow Data Pass-Through to WorkflowExecution

### Description

RemediationOrchestrator MUST pass through workflow data (including `executionBundle` and `executionBundleDigest`) from `AIAnalysis.status.rcaResult.selectedWorkflow` to `WorkflowExecution.spec.workflowRef` without performing catalog lookups.

### Priority

**P0 (CRITICAL)** - Core data flow for remediation execution

### Rationale

Per DD-CONTRACT-001 v1.2:
- Kubernaut Agent resolves `workflow_id → executionBundle` during workflow discovery
- `AIAnalysis.status.rcaResult.selectedWorkflow` contains the fully resolved workflow reference
- RO's responsibility is to pass this data through to WorkflowExecution
- RO should NOT duplicate catalog lookup logic (separation of concerns)

### Implementation

1. Read `AIAnalysis.status.rcaResult.selectedWorkflow` when phase = "Completed"
2. Pass through all fields to `WorkflowExecution.spec.workflowRef`:
   - `workflowId` ← `selectedWorkflow.workflowId`
   - `executionBundle` ← `selectedWorkflow.executionBundle` (resolved by Kubernaut Agent)
   - `executionBundleDigest` ← `selectedWorkflow.executionBundleDigest` (resolved by Kubernaut Agent)
3. Pass through `parameters` unchanged (UPPER_SNAKE_CASE keys)
4. Pass through `confidence` and `rationale` for audit trail
5. Fail if `selectedWorkflow` is nil or missing required fields

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-025-1 | RO does NOT call Data Storage API for workflow resolution | Unit |
| AC-025-2 | `executionBundle` and `executionBundleDigest` passed through from AIAnalysis.status | Unit, Integration |
| AC-025-3 | All workflow parameters passed through unchanged | Unit |
| AC-025-4 | Missing selectedWorkflow → RemediationRequest marked as Failed | Unit |
| AC-025-5 | Audit trail contains complete workflow selection context | Integration |

### Test Scenarios

```gherkin
Scenario: Workflow data pass-through
  Given AIAnalysis "aia-1" has status.rcaResult.selectedWorkflow with:
    | workflowId | executionBundle | executionBundleDigest |
    | pod-restart | registry.io/workflows/pod-restart:v1.2 | sha256:abc123 |
  And AIAnalysis "aia-1" phase is "Completed"
  When RemediationOrchestrator creates WorkflowExecution
  Then WorkflowExecution.spec.workflowRef should have:
    | workflowId | pod-restart |
    | executionBundle | registry.io/workflows/pod-restart:v1.2 |
    | executionBundleDigest | sha256:abc123 |
  And NO calls to Data Storage API should be made

Scenario: Missing selectedWorkflow fails remediation
  Given AIAnalysis "aia-1" has status.rcaResult.selectedWorkflow = nil
  And AIAnalysis "aia-1" phase is "Completed"
  When RemediationOrchestrator attempts to create WorkflowExecution
  Then RemediationRequest should transition to phase "Failed"
  And failure reason should be "missing_workflow_selection"
```

---

## BR-ORCH-026: Approval Orchestration

### Description

RemediationOrchestrator MUST create RemediationApprovalRequest CRD and NotificationRequest CRD when AIAnalysis signals approval is required, enabling operator review before workflow execution.

### Priority

**P0 (CRITICAL)** - Core approval workflow for medium-confidence remediations

### Rationale

When AIAnalysis confidence is below threshold (80%), human approval is required:
- Prevents automated execution of uncertain remediations
- Enables operator oversight for edge cases
- Provides audit trail for approval decisions
- Supports configurable approval timeout (default: 15 minutes per ADR-040)

### Implementation

1. Watch `AIAnalysis.status.approvalRequired`
2. When true:
   - Create RemediationApprovalRequest (per ADR-040)
   - Create NotificationRequest (per ADR-017/ADR-018, see BR-ORCH-001)
3. Wait for `RemediationApprovalRequest.status.decision`
4. On Approved: Create WorkflowExecution
5. On Rejected/Expired: Mark RemediationRequest as Failed with an approval failure reason

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-026-1 | RemediationApprovalRequest created when approvalRequired = true | Unit, Integration |
| AC-026-2 | NotificationRequest created for operator alerting | Unit, Integration |
| AC-026-3 | Approval decision watched and acted upon | Integration |
| AC-026-4 | Timeout handled via RemediationApprovalRequest controller | Integration |
| AC-026-5 | Rejected approval → RemediationRequest phase = Failed with a rejection reason | Unit |
| AC-026-6 | Approved → WorkflowExecution created | Integration, E2E |

### Test Scenarios

```gherkin
Scenario: Approval orchestration - approved
  Given AIAnalysis "aia-1" has approvalRequired = true
  When RemediationOrchestrator reconciles
  Then RemediationApprovalRequest should be created
  And NotificationRequest should be created
  When RemediationApprovalRequest decision is "Approved"
  Then WorkflowExecution should be created
  And RemediationRequest phase should be "executing"

Scenario: Approval orchestration - rejected
  Given AIAnalysis "aia-1" has approvalRequired = true
  When RemediationOrchestrator reconciles
  Then RemediationApprovalRequest should be created
  When RemediationApprovalRequest decision is "Rejected"
  And the rejection has no decision message
  Then NO WorkflowExecution should be created
  And RemediationRequest phase should be "Failed"
  And RemediationRequest failure reason should be "Rejected by operator"

Scenario: Selected workflow is unavailable at dispatch (proposed #2570)
  Given an approved RemediationApprovalRequest has produced a WorkflowExecution CR
  And the previously selected workflow ID has no match in the trusted catalog
  When WorkflowExecution checks the workflow before dispatch
  Then WorkflowExecution fails closed without a retry for the no-match
  And no external execution resource is created

Scenario: Approval timeout
  Given AIAnalysis "aia-1" has approvalRequired = true
  And RemediationApprovalRequest exists with no decision for 15 minutes
  When approval timeout is triggered
  Then RemediationRequest phase should be "Failed"
  And timeout reason should be "approval_timeout"
```

---

## Related Documents

- [DD-CONTRACT-001: AIAnalysis ↔ WorkflowExecution Alignment](../architecture/decisions/DD-CONTRACT-001-aianalysis-workflowexecution-alignment.md)
- [DD-CONTRACT-002: Service Integration Contracts](../architecture/decisions/DD-CONTRACT-002-service-integration-contracts.md)
- [ADR-040: RemediationApprovalRequest Architecture](../architecture/decisions/ADR-040-remediation-approval-request-architecture.md)
- [ADR-043: Workflow Schema Definition](../architecture/decisions/ADR-043-workflow-schema-definition-standard.md)

---

**Document Version**: 1.0
**Last Updated**: December 2, 2025
**Maintained By**: Kubernaut Architecture Team
