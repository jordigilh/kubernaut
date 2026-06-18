# Test Plan: #1418 Escalation Routing (`kubernaut_complete_no_action`)

**Version**: 1.0
**Date**: 2026-06-13
**Issue**: [#1418](https://github.com/jordigilh/kubernaut/issues/1418)
**Design**: [DD-AF-007](../../architecture/decisions/DD-AF-007-escalation-routing.md)
**Business Requirements**: BR-WORKFLOW-1418

## FedRAMP Control Mapping

| Control | Requirement | Verified By |
|---------|-------------|-------------|
| AC-6 (Least Privilege) | Tool not available to A2A LLM agent | document-mcp-only-decision |
| AU-2 (Audit Events) | Audit event emitted with full detail fields | IT-AF-1418-002, UT audit assertions |
| SI-4(5) (System Alerts) | Escalation triggers NotificationRequest | E2E-KA-1418-002 |
| IR-4(1) (Incident Response) | Escalation routes to ManualReviewRequired | UT-KA-1418-002, E2E-KA-1418-002 |
| IR-5 (Incident Monitoring) | NotificationRequest with operator_escalation reason | RO reconciliation path |
| SI-10 (Input Validation) | escalation_reason validated (length, whitespace, control chars) | UT-VALIDATE-1418-001 to 004 |

## Test Scenarios

### Unit Tests — KA Tool Logic

| ID | Scenario | Input | Expected | FedRAMP |
|----|----------|-------|----------|---------|
| UT-KA-1418-001 | Dismiss path sets IsActionable=false | `{ rr_id, reason }` | status="completed_no_action", IsActionable=false | — |
| UT-KA-1418-002 | Escalation path sets HumanReviewNeeded=true | `{ rr_id, escalation_reason }` | status="escalated", HumanReviewNeeded=true, HumanReviewReason="operator_escalation" | IR-4(1) |
| UT-KA-1418-003 | Escalation does NOT set IsActionable=false | `{ rr_id, escalation_reason }` | IsActionable=nil (unset) | AC-6 |
| UT-KA-1418-004 | Output includes status and escalation_reason | `{ rr_id, escalation_reason }` | Output.Status="escalated", Output.EscalationReason=input value | AU-2 |
| UT-KA-1418-005 | Dismiss preserves RCA when present | `{ rr_id }` (with prior RCA) | RCASummary preserved, IsActionable=false | — |
| UT-KA-1418-006 | Dismiss clears inherited HumanReviewNeeded | `{ rr_id }` (RCA has HumanReviewNeeded=true) | HumanReviewNeeded=false, HumanReviewReason="" | IR-4(1) |
| UT-KA-1418-007 | Whitespace-only escalation_reason rejected | `{ rr_id, escalation_reason: "   " }` | Error: "must not be whitespace-only" | SI-10 |
| UT-KA-1418-008 | escalation_reason > 1024 chars rejected | `{ rr_id, escalation_reason: 1025 chars }` | Error: "exceeds maximum length" | SI-10 |

### Unit Tests — AF Input Validation

| ID | Scenario | Input | Expected | FedRAMP |
|----|----------|-------|----------|---------|
| UT-VALIDATE-1418-001 | Valid escalation_reason (1024 chars) | 1024 character string | No error | SI-10 |
| UT-VALIDATE-1418-002 | Over-limit (1025 chars) rejected | 1025 character string | Error with length info | SI-10 |
| UT-VALIDATE-1418-003 | Whitespace-only rejected | `"   \t  "` | Error: whitespace-only | SI-10 |
| UT-VALIDATE-1418-004 | Control character rejected | `"reason\x01bad"` | Error: control character at position | SI-10 |
| UT-VALIDATE-1418-005 | Newline and tab allowed | `"line1\nline2\tok"` | No error | SI-10 |

### Integration Tests — AF MCP Bridge

| ID | Scenario | Method | Expected | FedRAMP |
|----|----------|--------|----------|---------|
| IT-AF-1418-001 | Dismiss path through MCP proxy | MCP tools/call (no escalation_reason) | status="completed_no_action" | — |
| IT-AF-1418-002 | Escalation path through MCP proxy (with audit) | MCP tools/call (with escalation_reason) | status="escalated", audit event emitted with result_type="operator_escalation" | AU-2 |
| IT-AF-1418-003 | RBAC denial for unauthorized user | MCP tools/call (no SAR grant) | MCP error response | AC-6 |
| IT-AF-1418-004 | mcpClient error propagation | MCP tools/call (mock returns error) | Error propagated, no audit event | — |

### Integration Tests — SessionFinalizer

| ID | Scenario | Method | Expected | FedRAMP |
|----|----------|--------|----------|---------|
| IT-AF-1418-005 | SessionFinalizer called on success | MCP tools/call → success | FinalizeSessionByRR invoked with SessionPhaseCompleted | — |

### E2E Tests — Full Journey

| ID | Scenario | Journey | Expected | FedRAMP |
|----|----------|---------|----------|---------|
| E2E-KA-1418-001 | Dismiss journey | Create RR → Investigate → Complete No Action (dismiss) | MCP response status="completed_no_action" | — |
| E2E-KA-1418-002 | Escalation journey | Create RR → Investigate → Complete No Action (escalate) | MCP response status="escalated" | SI-4(5), IR-5 |

### A2A Exclusion Tests

| ID | Scenario | Method | Expected | FedRAMP |
|----|----------|--------|----------|---------|
| ADV-AF-1418-001 | Tool not in A2A agent toolset | Inspect agent tool list | `kubernaut_complete_no_action` absent | AC-6 |

## Acceptance Criteria

1. All UT-KA-1418 tests pass (dismiss/escalation branching correct)
2. All UT-VALIDATE-1418 tests pass (input validation boundaries)
3. All IT-AF-1418 tests pass (MCP bridge wiring, RBAC, error handling)
4. All E2E-KA-1418 tests pass (full journey through live binary)
5. Audit events contain `result_type`, `delegation_type`, `tool_outcome` per AU-2
6. `operator_escalation` enum value in OpenAPI spec and `mapHumanReviewReason`
7. No session leak in PooledMCPClient on unmarshal failure

## Coverage Targets

| Tier | Target | Measured Against |
|------|--------|-----------------|
| UT | ≥80% of `complete_no_action.go` and `validate.EscalationReason` | Line coverage |
| IT | ≥80% of `HandleCompleteNoAction` and MCP bridge registration | Line coverage |
| E2E | Both dismiss and escalation journeys exercised | Scenario coverage |
