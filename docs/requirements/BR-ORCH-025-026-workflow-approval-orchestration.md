# BR-ORCH-025/026: Workflow Data Pass-Through and Approval Orchestration

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

**Key Design Decision**: RO does NOT perform catalog lookups. HolmesGPT-API resolves `workflow_id → container_image` during MCP search. RO passes through from `AIAnalysis.status.selectedWorkflow`.

---

## BR-ORCH-025: Workflow Data Pass-Through to WorkflowExecution

### Description

RemediationOrchestrator MUST pass through workflow data (including `container_image` and `container_digest`) from `AIAnalysis.status.selectedWorkflow` to `WorkflowExecution.spec.workflowRef` without performing catalog lookups.

### Priority

**P0 (CRITICAL)** - Core data flow for remediation execution

### Rationale

Per DD-CONTRACT-001 v1.2:
- HolmesGPT-API resolves `workflow_id → container_image` during MCP search
- `AIAnalysis.status.selectedWorkflow` contains the fully resolved workflow reference
- RO's responsibility is to pass this data through to WorkflowExecution
- RO should NOT duplicate catalog lookup logic (separation of concerns)

### Implementation

1. Read `AIAnalysis.status.selectedWorkflow` when phase = "Completed"
2. Pass through all fields to `WorkflowExecution.spec.workflowRef`:
   - `workflowId` ← `selectedWorkflow.workflowId`
   - `containerImage` ← `selectedWorkflow.containerImage` (resolved by HolmesGPT-API)
   - `containerDigest` ← `selectedWorkflow.containerDigest` (resolved by HolmesGPT-API)
3. Pass through `parameters` unchanged (UPPER_SNAKE_CASE keys)
4. Pass through `confidence` and `rationale` for audit trail
5. Fail if `selectedWorkflow` is nil or missing required fields

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-025-1 | RO does NOT call Data Storage API for workflow resolution | Unit |
| AC-025-2 | `container_image` and `container_digest` passed through from AIAnalysis.status | Unit, Integration |
| AC-025-3 | All workflow parameters passed through unchanged | Unit |
| AC-025-4 | Missing selectedWorkflow → RemediationRequest marked as Failed | Unit |
| AC-025-5 | Audit trail contains complete workflow selection context | Integration |

### Test Scenarios

```gherkin
Scenario: Workflow data pass-through
  Given AIAnalysis "aia-1" has status.selectedWorkflow with:
    | workflowId | containerImage | containerDigest |
    | pod-restart | registry.io/workflows/pod-restart:v1.2 | sha256:abc123 |
  And AIAnalysis "aia-1" phase is "Completed"
  When RemediationOrchestrator creates WorkflowExecution
  Then WorkflowExecution.spec.workflowRef should have:
    | workflowId | pod-restart |
    | containerImage | registry.io/workflows/pod-restart:v1.2 |
    | containerDigest | sha256:abc123 |
  And NO calls to Data Storage API should be made

Scenario: Missing selectedWorkflow fails remediation
  Given AIAnalysis "aia-1" has status.selectedWorkflow = nil
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
5. On Rejected/Expired: Mark RemediationRequest as Rejected

### Acceptance Criteria

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-026-1 | RemediationApprovalRequest created when approvalRequired = true | Unit, Integration |
| AC-026-2 | NotificationRequest created for operator alerting | Unit, Integration |
| AC-026-3 | Approval decision watched and acted upon | Integration |
| AC-026-4 | Timeout handled via RemediationApprovalRequest controller | Integration |
| AC-026-5 | Rejected approval → RemediationRequest phase = Rejected | Unit |
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
  Then NO WorkflowExecution should be created
  And RemediationRequest phase should be "Rejected"

Scenario: Approval timeout
  Given AIAnalysis "aia-1" has approvalRequired = true
  And RemediationApprovalRequest exists with no decision for 15 minutes
  When approval timeout is triggered
  Then RemediationRequest phase should be "Timeout"
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


