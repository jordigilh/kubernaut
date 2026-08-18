# DD-FLEET-006: AF's LLM-Facing `list_clusters`/`cluster_id` Is Not an Asymmetry with DD-FLEET-005

**Status**: ✅ Accepted (documentation-only, no code change)
**Date**: 2026-07-29
**Author**: AI Assistant
**Related**: Issue #1768 (Track 3), DD-FLEET-005, ADR-068

---

## Context

Issue #1768 (Track 3) flagged an apparent asymmetry: APIFrontend's (AF) own
ADK agent still registers `list_clusters` as an LLM-callable tool
(`pkg/apifrontend/agent/root.go`), and its `kubectl_get` tool takes an
LLM-supplied `cluster_id` argument (`pkg/apifrontend/tools/kubectl_get.go`).
DD-FLEET-005 made the opposite choice for KubernautAgent (KA): it removed
`list_clusters` and `list_tools_for_cluster` from KA's LLM-facing RCA tool
list entirely, replacing LLM-driven cluster discovery with server-side
pre-scoping (`prescopeFleetOverlay`) keyed off `signal.ClusterID`, which is
always already known before an RCA investigation starts.

The question: is AF's design a regression against DD-FLEET-005's stated
principle ("the LLM is currently the one deciding which cluster to
investigate, and it shouldn't be"), or a deliberate, orthogonal design point
that principle doesn't apply to?

## Analysis

DD-FLEET-005's argument rests entirely on one premise, stated directly in
its Context section:

> A single RCA investigation is always scoped to exactly one target cluster
> — `RemediationRequest.Spec.ClusterID` ... There is never a "pick a
> cluster" decision to make during a single investigation.

That premise is true for KA's RCA phase specifically, and false for AF's
own tool-calling surface in general:

- **KA's RCA investigation** is always invoked with a specific, already-known
  `RemediationRequest`/signal in hand (`Investigator.Investigate(ctx, signal)`,
  `signal.ClusterID` resolved before the call). There is no user-facing
  "browse my fleet" entry point into KA's RCA tool loop at all — cluster
  identity is an input to the phase, never an output the LLM discovers.
- **AF's agent**, by contrast, is a general-purpose, multi-turn, user-facing
  interactive/autonomous surface (`pkg/apifrontend/agent/root.go`) that
  fields ad-hoc SRE requests before any specific `RemediationRequest` or
  target cluster necessarily exists — e.g. "what's failing across my fleet
  right now?" or "check cluster prod-east for X" as a first message with no
  prior RR. `list_clusters` exists precisely because there is a genuine
  "pick a cluster" decision at this layer that DD-FLEET-005's premise says
  never happens in KA's RCA phase.

In other words, DD-FLEET-005's fix targets a *specific, narrow* mismatch
(RCA always has exactly one implicit target; the tool surface pretended
otherwise). AF's tool surface never made that claim to begin with —
`cluster_id` is explicitly parameterized on every fleet-scoped AF tool
(`kubectl_get(cluster_id)`) precisely because AF, unlike a single RCA
investigation, routinely needs to address more than one cluster across the
lifetime of a session.

## Alternatives Considered

### Alternative A — Not a real asymmetry; document and close — ✅ CHOSEN

Treat AF's `list_clusters`/`cluster_id` exposure as correct-as-is. The two
services solve different problems: KA removes an LLM decision that
shouldn't exist; AF exposes an LLM decision that legitimately does exist.
No code change.

**Pros**: Zero implementation risk. Matches the actual shape of AF's use
cases (ad-hoc, no pre-known target). Avoids incorrectly narrowing AF's
fleet-browsing capability to "match" a KA design principle that doesn't
transfer.

**Cons**: `cluster_id` remains a free-form, LLM-suppliable argument on AF's
fleet tools, which is a larger prompt-injection/scope-escape surface than
KA's zero-LLM-choice model — mitigated by AF's existing per-request
authn/authz (every AF tool call is already scoped to the calling user's
own RBAC-visible clusters at the MCP-gateway/ClusterRegistry layer,
independent of this decision).

### Alternative B — Hybrid: hide `list_clusters`/`cluster_id` once a target is known — ❌ REJECTED (for now)

Pre-scope and hide `list_clusters`/`cluster_id` only when AF is continuing
an existing RR/takeover session with an already-resolved target cluster;
keep them LLM-callable only for fresh ad-hoc sessions with no known target.

**Rejected because**: real implementation cost (session-state-dependent
tool-list construction in AF's agent, mirroring the refactor DD-FLEET-005
took on for KA) for a benefit that is speculative absent a concrete
incident of an LLM mis-targeting a cluster it shouldn't have. Revisit if
Alternative A's residual risk (see Consequences) materializes in practice.

### Alternative C — Harden `cluster_id` with explicit RBAC/audit, keep as-is otherwise — ❌ REJECTED (for now)

Keep `list_clusters` LLM-callable, but add an explicit per-call
authorization check (calling user's RBAC must cover the requested
`cluster_id`) plus stronger audit logging specifically for cross-cluster
tool calls.

**Rejected because**: preflight confirms this defense-in-depth layer likely
already exists structurally — AF's fleet tools route through the same
MCP-gateway-backed `FleetReaderFactory`/`ClusterRegistry` path that already
enforces cluster-scoped access and emits per-call audit events (AU-2/AU-3)
regardless of which cluster the LLM names. Verifying and, if needed,
strengthening this is worth a dedicated audit pass, but is independent of
the list_clusters-exposure question this DD resolves — tracked as a
follow-up if a gap is found (not a reason to withhold this DD's
documentation-only conclusion).

## Decision

**Alternative A**: No code change. AF's `list_clusters` and
`cluster_id`-parameterized fleet tools remain LLM-callable. This is not an
asymmetry with DD-FLEET-005 — it is DD-FLEET-005's own premise (no cluster
decision exists) correctly evaluated as false for AF's use case and true
for KA's RCA phase, yielding the correct opposite outcome for each.

## Consequences

**Positive**:
- Closes issue #1768 Track 3 with no implementation risk or behavior
  change.
- Prevents an incorrect "fix" that would have removed a genuine, needed
  capability from AF (ad-hoc multi-cluster browsing) to chase surface-level
  parity with a KA design that solves a different problem.

**Negative**:
- The residual prompt-injection/scope-escape surface DD-FLEET-005 called
  out for KA's old design is knowingly still present on AF's fleet tools
  (bounded by existing RBAC/audit, per Alternative A's Cons above).

**Neutral**:
- Alternatives B and C remain available follow-ups if a concrete incident
  or audit finding changes the risk calculus.

## Authority

Issue #1768 (Track 3), DD-FLEET-005, ADR-068.
