# DD-KA-2520-2521: Typed Investigation Outcomes and Decision Ledger

**Status**: Approved for implementation
**Date**: 2026-10-16
**Issues**: #2520, #2521
**Related**: #2518, BR-AUDIT-005, BR-KA-016, BR-KA-197, BR-KA-197.2, BR-KA-261, DD-KA-2496, DD-KA-2511

## Decision

Kubernaut Agent will carry an explicit, server-owned investigation outcome through
the existing Investigator → AgentSession → AIAnalysis result boundary. Terminal
routing must never infer `no_matching_workflows` from an absent selected workflow.

The canonical typed state contains:

- `TerminalOutcome`: `workflow_selected`, `problem_resolved`,
  `evidence_transport_unavailable`, `history_enrichment_unavailable`,
  `rca_incomplete`, `no_matching_workflows`, `workflow_discovery_failed`, and
  `cancelled` where applicable;
- `WorkflowDiscoveryStatus`: `not_started`, `started`, `completed_selected`,
  `completed_no_match`, `failed`, and `skipped_due_to_failure`;
- `HistoryStatus`: `available`, `empty`, `transport_unavailable`, `schema_error`,
  and `not_requested`.

Wire values use lower snake case. `TerminalOutcome` is authoritative. Existing
`Reason`, `SubReason`, and human-review fields remain compatibility projections for
current consumers and are not used to reconstruct the authoritative state.

Issue #2518 remains the owner of MCP retry/reconnect mechanics. When that path
returns a typed transport exhaustion result, #2521 consumes it as an evidence or
history-unavailable terminal outcome; #2521 must not implement a second retry loop.
The confirmed #2518 failure mode is not an LLM reasoning failure: a valid RCA can
be preserved while target-scoped history is unavailable.

The existing immutable audit store is the persistence path. Kubernaut Agent emits a
typed `aiagent.investigation.decision` event after each meaningful investigation
turn or state transition. No new CRD or database table is introduced.

## Classification rules

1. A missing workflow ID alone never produces `no_matching_workflows`.
2. `no_matching_workflows` requires an explicit completed discovery result proving
   zero acceptable candidates.
3. Parser, catalog, validation, provider, or workflow-LLM exhaustion after
   discovery starts produces `workflow_discovery_failed` with gate status `failed`.
4. #2518 MCP/history transport or schema exhaustion before discovery produces
   `evidence_transport_unavailable` or the source-specific
   `history_enrichment_unavailable`, with `HistoryStatus=transport_unavailable` or
   `schema_error` and discovery status `skipped_due_to_failure`.
5. `rca_incomplete` is reserved for an RCA path that has valid required evidence
   but explicitly establishes insufficient causal evidence, or an RCA-specific
   parsing/validation exhaustion that is not an evidence transport failure.
6. A successful #2518 retry/reconnect preserves the original logical correlation
   and continues normally; intermediate retries do not create terminal outcomes.
7. Target-scoped history already acquired for an earlier target is preserved, but
   a required post-RCA target re-enrichment failure remains explicitly unavailable;
   it must not be represented as empty history.

## Decision-ledger event

`aiagent.investigation.decision` uses the existing audit envelope and contains only
bounded, allow-listed data:

- investigation/session/turn identifiers, phase, and `turn_kind`;
- provider/model and cumulative token totals;
- evidence-source, history, target, and causal-mechanism statuses;
- continuation/stop reason, terminal outcome, and workflow-discovery status;
- human-review/failure class and retry attempt count;
- the logical correlation ID and bounded related event/tool identifiers.

The event excludes prompts, hidden reasoning, complete tool arguments/results,
credentials, raw transport errors, and unredacted model rationale. Existing final
completion/failure audit projections gain only the typed terminal outcome and
workflow-discovery status needed to join the final result to the ledger timeline.

All enrichment and investigation audit events for one logical remediation request
must preserve the same top-level correlation ID. A source-specific incident ID may
be retained only as a bounded related identifier; it must not become the only bridge
between lifecycle phases.

## Alternatives considered

### Alternative A — Explicit typed state plus dedicated ledger event (selected)

This directly satisfies #2520 and #2521, prevents fabricated no-match routing,
preserves legacy consumers through projections, and provides a durable correlated
timeline without a second persistence model.

### Alternative B — Extend only existing RCA/response events (rejected)

The existing events cannot distinguish tool-only turns, discovery-not-started,
completed-no-match, and transport-skipped discovery without inference. Extending
them alone would preserve the ambiguity exposed by #2518.

### Alternative C — Add a decision-ledger CRD or database table (rejected)

This duplicates the immutable audit store and adds lifecycle, retention,
authorization, and a second correlation path without improving the typed result
boundary.

## Consequences and implementation constraints

- Shared enums and result fields must be propagated through KA, AgentSession, and
  AIAnalysis, including generated OpenAPI/CRD/client artifacts.
- Legacy results without typed state use a conservative investigation-failure path;
  they never infer no-match.
- The existing #2518 source owns transport retry, session resume, and exhaustion
  details. The decision ledger records only the bounded typed result.
- Audit and CRD payloads must not copy existing raw RCA narratives, prompts, or
  unrestricted tool data into the new ledger event.
- Deterministic turn/state identity is required to avoid duplicate ledger events on
  reconciliation and retry.

## Controls

- FedRAMP AU-2/AU-3: decision checkpoints are attributable, typed, and correlated.
- FedRAMP AU-9/AU-11: the existing immutable, retained audit store is reused and
  payloads remain bounded.
- FedRAMP SI-10: enum and phase validation prevents ambiguous terminal routing.
- FedRAMP AC-4/AC-6: target-scoped history is preserved and absent workflows never
  authorize execution.
- SOC 2 CC7.2: correlation queries reconstruct selected, genuine no-match, and
  transport-failure journeys. CC8.1 applies to the controlled implementation
  process, not event-completeness semantics.
- OWASP ASVS V5.1/V5.5.2 and V7.1.1/V7.2.1: validate typed bounded data and make
  security-relevant investigation decisions observable without secrets or private
  reasoning.

Production comments and tests implementing this decision must reference
`DD-KA-2520-2521`.
