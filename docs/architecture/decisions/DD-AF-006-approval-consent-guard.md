# DD-AF-006: Approval Consent Guard — A2A Tool Removal

**Status**: Accepted
**Date**: 2026-06-12
**Issue**: [#1415](https://github.com/jordigilh/kubernaut/issues/1415)
**Related**: ADR-022 (AF SA unified security model), ADR-040 (RAR architecture)

## Context

The `kubernaut_approve` tool was available to the A2A LLM agent, meaning the LLM could autonomously approve Remediation Approval Requests (RARs) without genuine human consent. This violated the principle of least privilege (AC-6) — the LLM should not hold the capability to make irreversible approval decisions on behalf of a human operator.

Observed risk: A sufficiently persuasive prompt or adversarial user message could cause the LLM to call `kubernaut_approve` without authentic human review, bypassing the consent gate that RARs are designed to enforce.

## Decision

### Structural Removal from A2A Agent

Remove `kubernaut_approve` from the A2A agent's tool constructors in `pkg/apifrontend/agent/root.go`. The tool is never registered in the ADK tool list, making it structurally impossible for the LLM to invoke it regardless of prompt injection or adversarial input.

### Retain on MCP Bridge for Console

`kubernaut_approve` remains registered in the MCP bridge (`pkg/apifrontend/handler/mcp_bridge.go`) because:

1. The Console UI routes Approve/Reject button clicks through the MCP `tools/call` endpoint
2. MCP requests carry authenticated user identity (OIDC token → SAR authorization)
3. The approval is attributed to the human user, not the LLM

### Prompt Reinforcement (Defense-in-Depth)

The LLM system prompt (`prompt.txt`) explicitly states:

```
NOTE: kubernaut_approve is NOT available to you. Approval and rejection actions
are handled exclusively by the console UI. When approval is needed, instruct
the user to click the Approve or Reject button.
```

This is a secondary defense; the primary control is tool absence.

### Console Flow

```
User clicks "Approve" in Console UI
  → Console sends MCP tools/call { name: "kubernaut_approve", ... }
  → AF MCP bridge authenticates user via OIDC token
  → SAR check: user must have "use" on kubernaut.ai/tools/kubernaut_approve
  → HandleApprove executes with username attribution
  → Audit event emitted with user identity
  → Console receives structured approval_request_resolved event
  → Console auto-sends "The remediation has been approved" to A2A to continue flow
```

## Defense-in-Depth Layers

| Layer | Mechanism | Failure mode it prevents |
|-------|-----------|--------------------------|
| 1. Tool absence | Not in `buildToolList()` | LLM cannot call what doesn't exist |
| 2. Prompt instruction | Explicit "NOT available to you" | Prevents hallucinated tool calls |
| 3. RBAC (MCP only) | SAR check on MCP path | Even if somehow reached, unauthorized users blocked |
| 4. Audit trail | User attribution on approval | Tamper-evident record of who approved |

## Alternatives Considered

### A: RBAC-only (keep tool in A2A, deny via SAR)

Rejected: The LLM would still attempt to call the tool, receive a "permission denied" error, and potentially retry or confuse the user. Structural absence is cleaner.

### B: Prompt-only ("do not call kubernaut_approve")

Rejected: Prompt instructions can be circumvented by adversarial prompts or prompt injection. Not sufficient as a sole control.

### C: Remove from both A2A and MCP (Selected: remove from A2A only)

Rejected: Console needs the MCP path for legitimate human-driven approvals. Removing from MCP would break the Console workflow.

## Consequences

### Positive

- Eliminates the entire class of LLM auto-approval vulnerabilities
- Console retains full approval capability with proper attribution
- Audit trail always shows a human user, not the LLM
- Adversarial tests (ADV-AF-1415-001/002) prove the tool cannot re-appear

### Negative

- A2A tool count drops from 24 to 23 (test assertions updated)
- Users cannot approve via chat — must use Console UI buttons
- Console team must implement MCP routing for Approve/Reject (tracked in `jordigilh/kubernaut-demo-console#2`)

### Stale Documentation Identified

- ADR-022 SEC-09: Still implies any path can call `kubernaut_approve` — needs update
- `ARCHITECTURE.md` Flow 3: Still shows A2A approval path — needs update
- `prompt.go` roleGuidanceMap: SRE/approver guidance still mentions "approve" capability — needs alignment

## Test Coverage

| Tier | IDs | Validates |
|------|-----|-----------|
| UT (adversarial) | ADV-AF-1415-001, 002 | Tool not in agent toolset under any condition |
| UT | UT-AF-100-002 (updated) | Tool count = 23 (was 24) |
| E2E | TC-E2E-A2A-T07 (refactored) | Approval attempt yields guidance text, not tool call |
| E2E | TC-E2E-MCP-FULL-02+04 | Console path still works via MCP |

## FedRAMP Controls

| Control | Application | Evidence |
|---------|-------------|----------|
| AC-6 (Least Privilege) | LLM agent does not hold approval capability; humans approve via Console | ADV-AF-1415-001: tool absent from toolset |
| AU-2 (Audit Events) | MCP approval path emits audit event with authenticated user identity | TC-E2E-MCP-FULL-04: audit event with username |
| SI-10 (Information Input Validation) | Structural absence prevents adversarial prompt from triggering approval | ADV-AF-1415-002: adversarial scenario blocked |
