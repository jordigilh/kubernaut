# DD-AF-007: Escalation Routing — `kubernaut_complete_no_action` Extension

**Status**: Accepted
**Date**: 2026-06-13
**Issue**: [#1418](https://github.com/jordigilh/kubernaut/issues/1418)
**Related**: ADR-022 (AF SA unified security model), DD-AF-004 (investigation tool split), DD-AF-006 (approval consent guard)

## Context

After investigating an alert, the KA LLM agent may determine that no automated remediation workflow is appropriate but the alert still requires human attention. Prior to this change, `kubernaut_complete_no_action` only supported a "dismiss" path (mark alert as not actionable), leaving no structured mechanism for the agent to escalate to a human team.

Observed gap: The agent would either force-fit an inappropriate workflow or dismiss genuine alerts that needed human expertise, with no middle ground for operator escalation.

## Decision

### Dual-Path Branching in `kubernaut_complete_no_action`

Extend the existing `kubernaut_complete_no_action` tool to support two mutually exclusive paths based on input:

| Condition | Path | InvestigationResult Effect | RR Outcome (via RO) |
|-----------|------|---------------------------|---------------------|
| `escalation_reason` absent or empty | **Dismiss** | `IsActionable=false`, `HumanReviewNeeded=false` | `WorkflowNotNeeded` |
| `escalation_reason` present (non-empty, non-whitespace) | **Escalate** | `HumanReviewNeeded=true`, `HumanReviewReason="operator_escalation"` | `ManualReviewRequired` + NotificationRequest |

### Dismiss Path Invariant

When dismissing, explicitly clear `HumanReviewNeeded=false` and `HumanReviewReason=""` to prevent inherited RCA signals from leaking into the dismiss outcome. This ensures AA's precedence check (`needs_human_review` before `hasNotActionableSignal`) never routes a dismiss to `ManualReviewRequired`.

### Escalation Semantics

The `operator_escalation` reason is a new `HumanReviewReason` enum value in the KA OpenAPI spec. It signals that the escalation was explicitly requested by the investigating agent (acting on behalf of the operator), not inferred from RCA ambiguity.

### MCP-Only Registration (Console Path)

`kubernaut_complete_no_action` is registered in the MCP bridge only — NOT in the A2A agent tool list. This follows the same pattern as `kubernaut_approve` (DD-AF-006):

1. Console UI presents Dismiss/Escalate buttons after investigation completes
2. Console sends MCP `tools/call` with appropriate arguments
3. AF MCP bridge authenticates user, performs SAR check, and proxies to KA
4. The A2A LLM agent does NOT have access to this tool (structural absence)

Rationale: The decision to dismiss or escalate is a human judgment call. The LLM investigates and presents findings; the human (via Console) decides disposition.

### Session Finalization

Both paths finalize the IS CRD session phase to `Completed`. The distinction between dismiss and escalation is carried by `InvestigationResult` fields, not by session phase. The Reconciliation Operator (RO) uses `HumanReviewNeeded` to determine the final RR outcome.

### Input Validation

| Field | Constraints |
|-------|-------------|
| `rr_id` | Required; validated via `validate.RRID` (accepts `namespace/name` format) |
| `escalation_reason` | Max 1024 chars; no control characters (except `\n`, `\t`); must not be whitespace-only |
| `reason` | Optional dismiss reason; max 1024 chars |

## Data Flow

```
Console "Escalate" button click
  → MCP tools/call { name: "kubernaut_complete_no_action", arguments: { rr_id, escalation_reason } }
  → AF MCP bridge: authenticate user (OIDC) → SAR check → HandleCompleteNoAction
  → validate.RRID + validate.EscalationReason
  → ka.MCPClient.CompleteNoAction → KA HTTP → Handle()
  → KA sets InvestigationResult { HumanReviewNeeded: true, HumanReviewReason: "operator_escalation" }
  → KA calls CompleteHTTPSession → KA HTTP response → RO reconciles RR
  → RO sees HumanReviewNeeded → sets Outcome = ManualReviewRequired
  → RO creates NotificationRequest with HumanReviewReason = "operator_escalation"
  → Console receives structured response { status: "escalated" }
```

## Audit Trail (AU-2)

`HandleCompleteNoAction` emits `EventKAResultReceived` with detail fields:

| Field | Value |
|-------|-------|
| `rr_id` | The remediation request ID |
| `status` | `"completed_no_action"` or `"escalated"` |
| `result_type` | `"completed"` or `"escalated"` |
| `delegation_type` | `"interactive"` |
| `tool_outcome` | `"success"` |
| `reason` | Dismiss reason (if provided) |
| `escalation_reason` | Escalation reason (escalate path only) |

## Defense-in-Depth Layers

| Layer | Mechanism | Failure mode it prevents |
|-------|-----------|--------------------------|
| 1. Tool absence (A2A) | Not in `buildToolList()` | LLM cannot dismiss/escalate without human |
| 2. RBAC (MCP) | SAR check on `kubernaut.ai/tools/kubernaut_complete_no_action` | Unauthorized users blocked |
| 3. Input validation | Whitespace rejection, length limits, control char filter | Content-free or malformed escalations |
| 4. Dismiss path clearing | Explicit `HumanReviewNeeded=false` on dismiss | Inherited RCA signals don't leak |
| 5. Audit trail | Full detail fields on EventKAResultReceived | Tamper-evident record |

## Alternatives Considered

### A: Separate `kubernaut_escalate` tool

Rejected: Would require a new tool registration, new RBAC resource, and duplicated session finalization logic. Single tool with branching is simpler and matches the Console UX (one screen, two buttons).

### B: Escalation via A2A agent (let LLM decide)

Rejected: Same reasoning as DD-AF-006 — escalation is a human judgment. The LLM presents findings; the human chooses disposition. Structural absence prevents prompt-driven escalation.

### C: Use existing `HumanReviewReason` values for escalation

Rejected: Existing values (`rca_incomplete`, `investigation_inconclusive`, etc.) describe AI-detected conditions. Operator escalation is semantically different — it represents explicit human intent to route to a team. A new enum value (`operator_escalation`) preserves this distinction.

## Consequences

### Positive

- Operators have a structured path to escalate without force-fitting workflows
- NotificationRequest creation provides automated alerting to on-call teams
- Audit trail distinguishes AI-detected reviews from operator-initiated escalations
- Dismiss path is defensively coded (clears inherited signals)

### Negative

- OpenAPI enum extended (requires client regeneration on spec updates)
- Console must implement escalation UI (Escalate button + reason text field)
- E2E test coverage for CRD transitions depends on Kind cluster infrastructure

## Test Coverage

| Tier | IDs | Validates |
|------|-----|-----------|
| UT (KA) | UT-KA-1418-001 to 005 | Dismiss/escalation branching, field clearing, validation |
| UT (AF) | validate.EscalationReason boundary tests | Length, whitespace, control char rejection |
| IT (AF) | IT-AF-1418-001 to 004 | MCP proxy wiring, RBAC denial, error propagation |
| E2E | E2E-KA-1418-001, 002 | Full dismiss/escalate journey through live binary |

## FedRAMP Controls

| Control | Application | Evidence |
|---------|-------------|----------|
| AC-6 (Least Privilege) | Tool absent from A2A agent; human-only via Console MCP | Tool not in `buildToolList()` |
| AU-2 (Audit Events) | Full audit detail emitted with result_type, delegation_type | IT-AF-1418-002 audit assertion |
| SI-4(5) (System Alerts) | Escalation triggers NotificationRequest via RO | E2E-KA-1418-002 |
| IR-4(1) (Incident Response) | Automated routing to ManualReviewRequired outcome | RO reconciliation of HumanReviewNeeded |
| IR-5 (Incident Monitoring) | NotificationRequest creation for on-call notification | RO + NotificationRequest CRD |
| SI-10 (Input Validation) | Whitespace rejection, length limits, control char filter | UT validate.EscalationReason |
