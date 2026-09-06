# DD-AF-014: Business-Outcome Completion Coordinator

**Status**: Accepted
**Date**: 2026-09-05
**Issue**: [#2365](https://github.com/jordigilh/kubernaut/issues/2365)
**Related**: [DD-AF-011](DD-AF-011-phase-transition-consent-guard.md), [DD-AF-007](DD-AF-007-escalation-routing.md), BR-INTERACTIVE-010, BR-SESS-013, BR-AUDIT-005

## Context

API Frontend currently uses ADK turn completion as an implicit proxy for business-lifecycle completion. This is unsafe because an LLM can return free-form narration after a successful workflow discovery without calling `kubernaut_present_decision`.

Issue #2365 exposed the resulting failure:

1. Kubernaut Agent completes workflow selection and AF completes `kubernaut_discover_workflows`.
2. The AF model narrates that it will submit the result but does not call `kubernaut_present_decision`.
3. The A2A stream closes normally without an `investigation_summary` artifact.
4. The Console cannot render workflow options and the RemediationRequest remains in `Analyzing`.

The attempted recovery in PR #2361 coupled this obligation to the generic reinvocation loop. That loop cannot safely distinguish a missing presentation artifact from a legitimate consent boundary or an autonomous workflow chain. In particular, `full_remediation` must present options and wait before execution, while `full_remediation_autonomous` must continue through selection and execution in the same turn.

This is a lifecycle-completion problem, not a prompt problem and not a generic retry problem.

## Decision

Introduce a typed **business-outcome completion coordinator** at the AF A2A pre-finalization boundary. The coordinator verifies that every turn has reached a valid business outcome before the A2A executor publishes the final status.

This is a declarative lifecycle state machine with a mostly forward, DAG-shaped path. It is not a pure DAG because reconnect, cancellation, failure, timeout, and disconnection are valid side transitions.

### Responsibilities

The coordinator owns:

- Investigation completion.
- Workflow discovery completion.
- Required decision-artifact completion.
- Mode-specific continuation policy.
- Structured incomplete-data outcomes.
- Idempotent recovery and escalation.

The existing phase guard remains responsible for authorization and ordering. The generic reinvocation loop remains responsible only for legitimate stalled investigation continuation. Neither component decides whether a business artifact obligation has been satisfied.

### Lifecycle State

AF session state must distinguish these independent dimensions:

- `interaction_mode`.
- `investigation_status`.
- `discovery_status`.
- `discovery_result`.
- `decision_artifact_status`.
- `execution_authorization`.
- `terminal_outcome`.

The critical invariant is that presentation and authorization are independent:

```text
decision_artifact_status = required
execution_authorization = blocked_until_user_confirmation
```

This represents `full_remediation` correctly: workflow options are shown, but `select_workflow` remains blocked until a genuine user turn.

### Mode Policy

`interactive`:

- User confirmation is required before discovery and execution.
- If discovery occurs after confirmation, a decision artifact is required before the turn can complete.

`full_remediation`:

- Investigation and discovery may proceed automatically.
- A decision artifact is required after successful discovery.
- Execution remains blocked until a genuine user selection.

`full_remediation_autonomous`:

- Investigation, discovery, selection, and execution may chain in one turn.
- Presentation recovery must not intercept or alter the chain.

### Discovery Normalization

All recovery logic must consume one canonical discovery representation. It must reuse `ka.ParseDiscoverWorkflowsResponse`, which supports:

- Direct AF responses containing `workflows`.
- KA envelope responses containing `status` and an encoded `response` payload.

The parser must recognize a direct response even when `workflows` is empty so that target metadata is not lost. Malformed or incomplete entries are normalization failures and must not become executable options.

The coordinator maps canonical discovery data to `WorkflowOption` values without inventing RCA facts or workflow identity. Existing parser behavior that preserves an absent workflow name must remain intact.

### Decision Artifact Recovery

At the pre-finalization boundary:

1. Read authoritative RCA state from `StateKeyGroundedRCA`.
2. Read the successful discovery `FunctionResponse` from the ADK session event history.
3. Determine whether a `present_decision` artifact was emitted by scanning the model `FunctionCall` history.
4. If the artifact exists, finish normally.
5. If presentation is required and the artifact is missing, emit a deterministic `investigation_summary` artifact through `EventBridge.EmitArtifact`.
6. Mark the artifact with recovery provenance:

```text
source: af_completion_recovery
recovery_reason: missing_present_decision
requires_user_selection: true|false
```

7. Preserve consent state and `driverActive`; never invoke `select_workflow` as recovery.

The recovered artifact is presentation-only. It is not an LLM authorization, workflow selection, or execution approval.

### Incomplete Authoritative Data

If RCA or discovery data is incomplete, AF must emit a truthful structured failure artifact when possible. It may use the existing schema-required shape:

```json
{
  "session_id": "...",
  "summary": "Structured workflow decision could not be completed.",
  "rca": {
    "explanation": "Authoritative investigation details were unavailable.",
    "tool_calls_count": 0,
    "llm_turns": 0
  },
  "options": [],
  "status": "failure",
  "failure_reason": "missing_authoritative_data"
}
```

The coordinator must not fabricate severity, confidence, causal chains, targets, workflow names, or execution decisions.

If a required outcome still cannot be produced after bounded recovery, AF must use the existing KA `complete_no_action` escalation path with a fixed system-generated escalation reason. This produces `HumanReviewReason=operator_escalation`, a `ManualReviewRequired` outcome, and notification routing. It must not use ordinary dismissal and must be idempotent.

### Idempotency

Recovery must be keyed by the session, RemediationRequest, and discovery event identity. Repeated finalization, reconnect, or transport retries must not emit duplicate decision artifacts or duplicate escalation outcomes.

## Alternatives Considered

### Generic reinvocation

Rejected. It reruns model behavior, may repeat discovery, can lose authoritative context, and conflicts with consent checkpoints. PR #2361 demonstrated that coupling this recovery to the reinvocation loop can terminate the phase-3 consent journey before the genuine user selection turn.

### Prompt-only correction

Rejected as the control mechanism. Provider behavior, model drift, and prompt loss can reproduce the issue. Prompt guidance remains useful as defense-in-depth only.

### Forced `present_decision` tool choice

Rejected as the primary mechanism. Tool-choice behavior varies by provider and still leaves the system without a deterministic outcome when the provider ignores the constraint.

### Unmarked server-side synthesis

Rejected. An unmarked artifact could be mistaken for an LLM-authored decision. Deterministic recovery is accepted only as an explicitly attributed, presentation-only artifact that cannot authorize execution.

### New AF terminal phase

Rejected. AF already has a valid operator-escalation contract spanning KA, AA, RO, notification, and audit. Adding a new terminal phase would duplicate outcome semantics and require CRD/API changes.

## Wiring Manifest

| Component | Production Entry Point | Wiring Location | Integration Test |
|---|---|---|---|
| Canonical discovery normalization | `kubernaut_discover_workflows` response and completion recovery | `pkg/apifrontend/ka/config.go` | IT-AF-2365-004 |
| Lifecycle state/obligation tracking | AF phase callbacks and session events | `pkg/apifrontend/agent/phase_guard.go`, `pkg/apifrontend/session/` | IT-AF-2365-005 |
| Completion coordinator | A2A execution before final status | `pkg/apifrontend/launcher/streaming_executor.go` / coordinator package | IT-AF-2365-006 |
| Recovered decision artifact | A2A EventBridge | `pkg/apifrontend/launcher/event_bridge.go` | IT-AF-2365-007 |
| Bounded escalation | AF to KA MCP bridge | `pkg/apifrontend/tools/ka_tools.go` | IT-AF-2365-008 |

## Security and Compliance

The design supports:

- **FedRAMP AC-6**: recovered presentation cannot bypass workflow-selection consent.
- **FedRAMP SI-10**: discovery data is normalized and validated before becoming a Console option.
- **FedRAMP AU-3/AU-12**: recovered artifacts and escalation carry provenance, correlation, and outcome data.
- **FedRAMP SI-4**: missing artifact and bounded recovery are observable events.
- **OWASP ASVS 5.1**: untrusted model/provider fields are schema-validated and malformed options are rejected.
- **OWASP ASVS 5.5.2**: structured output is encoded through the existing sanitized A2A artifact boundary.

## Consequences

### Positive

- A2A transport completion no longer implies remediation lifecycle completion.
- Consent and presentation obligations cannot accidentally bypass one another.
- Autonomous chaining remains unchanged.
- Recovery is deterministic, auditable, and provider-independent.
- The coordinator can later enforce other business-output obligations without expanding reinvocation logic.

### Negative

- AF session state becomes a typed lifecycle contract rather than a collection of independent booleans.
- Discovery responses need one canonical normalization path and explicit malformed-data handling.
- A new completion coordinator requires integration tests through the real A2A path.
- The incomplete-data fallback escalates to human review and therefore creates an operator notification rather than silently completing.

## Verification Requirements

Implementation must follow the test plan at `docs/tests/2365/IMPLEMENTATION_PLAN.md`. The pyramid invariant is mandatory:

- Unit tests prove normalization, state transitions, artifact construction, and idempotency.
- Integration tests prove production A2A wiring, consent preservation, artifact delivery, and escalation.
- E2E tests prove the Console-visible workflow-card journey and the autonomous regression path.

No implementation is complete with unit tests alone.
