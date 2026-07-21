# ADR-KA-002: Agent Security — Defense-in-Depth Model for Opaque OCI Agents

**Status**: ACCEPTED
**Date**: 2026-07-19
**Related**: [ADR-KA-001](ADR-KA-001-shadow-agent-alignment-check.md) (shadow agent alignment check — current Layer 4 implementation for KA-side investigations), [#1535](https://github.com/jordigilh/kubernaut/issues/1535) (AuthBridge audit relay), [#1681](https://github.com/jordigilh/kubernaut/issues/1681) (AuthBridge shadow-evaluator tee), [#1536](https://github.com/jordigilh/kubernaut/issues/1536) (opaque-OCI-agent runtime-agnostic direction), [PROPOSAL-EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) §16.7 (❌ superseded — original Goose/OAS-session-based version of this model)

## Context

Kubernaut is moving from a Kubernaut-known agent runtime (Goose sessions, then OAS Runtime + ACP) to a runtime-agnostic model where investigation/remediation agents are packaged as opaque OCI images and executed via their own entrypoint (#1536). Kubernaut has no visibility into an opaque agent's internal reasoning loop, prompt structure, or runtime framework — only the network traffic the agent produces (outbound LLM calls, outbound MCP tool calls) is observable, and only because `AuthBridge` intercepts that traffic for credential injection.

`PROPOSAL-EXT-003` §16.7 defined a five-layer defense-in-depth security model for the prior Goose/OAS-session architecture. The layer boundaries and rationale remain valid, but the implementation details assumed a Kubernaut-orchestrated session (shadow-agent-as-a-second-Goose-session, DG-20) that no longer exists once the agent runtime is opaque. This ADR restates the model in terms of the opaque-agent architecture and records what's decided vs. still open.

### Why defense-in-depth (not one layer)

Per the Lasso research cited in `PROPOSAL-EXT-003` (May 2026), infrastructure-level sandboxing alone can be bypassed through trusted tools and approved outbound connections. No single layer is sufficient; each layer below catches attacks the others miss.

## Decision

### The five layers (opaque-agent model)

```
Layer 5: Deterministic gate
         Remediation requires human/policy approval regardless of what
         the agent concludes. Unchanged from PROPOSAL-EXT-003.
         ─────────────────────────────────────────────────────────────
Layer 4: Semantic detection (pluggable evaluator behind AuthBridge's relay)
         LLM-based (or future NemoClaw-based) evaluation of the agent's
         full LLM+tool-call stream, fed via AuthBridge's interception —
         not a Kubernaut-orchestrated shadow session, since the agent's
         own runtime is opaque. Today: Kubernaut's own shadow-agent LLM
         per ADR-KA-001 evaluates KA's own in-process investigation flow
         only. Extending an equivalent feed to opaque-agent traffic is
         #1681 (open, v1.7) — until it ships, opaque-agent investigations
         have no Layer 4 coverage.
         ─────────────────────────────────────────────────────────────
Layer 3: Pattern detection (candidate: NemoClaw scanner)
         Regex-based fast-path pattern matching, zero LLM cost.
         NOT decided or implemented — PROPOSAL-EXT-003 DG-22 scoped
         this as a complementary pre-filter to Layer 4, never built.
         Remains open and unscheduled.
         ─────────────────────────────────────────────────────────────
Layer 2: Infrastructure isolation (OpenShell / Agent Sandbox)
         Sandbox isolation (Landlock/gVisor/Kata), network policy
         (egress restricted to approved endpoints), filesystem
         confinement. Coexists with AuthBridge as a sidecar in the
         same pod — does not replace it. Coexistence testing is still
         pending (SPIKE-OPENSHELL-OAS-RUNTIME.md §8).
         ─────────────────────────────────────────────────────────────
Layer 1: Identity and credential interception (AuthBridge)
         Zero-secret: opaque agents never hold credentials. AuthBridge
         intercepts every outbound LLM/MCP call for auth/authz and
         credential injection. This is the ONLY layer that observes
         100% of an opaque agent's traffic by construction — it is the
         durable foundation the other layers build on, regardless of
         agent runtime (Goose, OAS, or fully opaque) or sandbox choice
         (plain K8s pod or OpenShell).
```

### What's decided vs. open

| Layer | Status | Reference |
|---|---|---|
| L1 — AuthBridge interception | Decided; durable across runtime/sandbox choices | this ADR |
| L1 — AuthBridge → audit relay | Decided, scoped | #1535 |
| L1 — AuthBridge → shadow-evaluator tee | Open, scoped for v1.7 | #1681 |
| L2 — OpenShell / Agent Sandbox | Open (DG-21 carries over unchanged) | `PROPOSAL-EXT-003` DG-21 |
| L1/L2 — sidecar coexistence in one pod | Open, untested | `SPIKE-OPENSHELL-OAS-RUNTIME.md` §8 |
| L3 — NemoClaw pattern scanner | Open, undecided whether/when adopted | `PROPOSAL-EXT-003` DG-22 |
| L4 — evaluator identity for opaque agents | Open — Kubernaut's own shadow LLM vs. NemoClaw vs. hybrid | this ADR, #1681 |
| L5 — Deterministic gate | Decided, unchanged | ADR-KA-001, RO approval gating |

### Key architectural invariant

**Layer 1 (AuthBridge) is decoupled from the choice of Layer 3/4 evaluator.** Adopting NemoClaw, keeping Kubernaut's own bespoke LLM evaluator, or running both would all consume the same AuthBridge relay — none of them requires re-architecting the interception point. This was confirmed explicitly to avoid speculative rework: AuthBridge remains in place through the OpenShell migration (an L2 change) and through any future L3/L4 evaluator change.

## Consequences

### Positive

- The security model survives runtime churn (Goose → OAS → opaque) because it is defined in terms of network-observable layers, not Kubernaut-orchestrated session internals.
- L3/L4 evaluator decisions (NemoClaw vs. bespoke vs. hybrid) can proceed independently of L1/L2 decisions (AuthBridge, OpenShell), reducing coupling between unrelated workstreams.
- Today's `ADR-KA-001` shadow agent continues to function unmodified for KA-side (non-opaque-agent) investigations; this ADR only extends the model to cover the opaque-agent path.

### Negative / Risks

- For opaque agents, Layer 4 currently has **no evaluator feed at all** until #1681 ships — opaque-agent investigations run without semantic security evaluation in the interim. (`ADR-KA-001`'s shadow agent only covers KA's own in-process LLM calls, not traffic from an opaque OCI agent.)
- L1/L2 sidecar coexistence in the same pod is unvalidated; a real port/routing conflict discovered later would force a redesign of one of the two components.
- L3 (NemoClaw) and the L4 evaluator-identity question are both unscheduled; without a target milestone, opaque-agent semantic security coverage could remain a gap across multiple releases.

## References

- [PROPOSAL-EXT-003](../proposals/PROPOSAL-EXT-003-goose-runtime-evaluation.md) §16.7 (❌ superseded — original Goose/OAS-session version of this model)
- [ADR-KA-001](ADR-KA-001-shadow-agent-alignment-check.md) (shadow agent alignment check — current L4 implementation for KA-side investigations)
- [#1535](https://github.com/jordigilh/kubernaut/issues/1535) (AuthBridge audit relay)
- [#1681](https://github.com/jordigilh/kubernaut/issues/1681) (AuthBridge shadow-evaluator tee, v1.7)
- [#1536](https://github.com/jordigilh/kubernaut/issues/1536) (opaque-OCI-agent runtime-agnostic direction)
- [SPIKE-OPENSHELL-OAS-RUNTIME.md](../spikes/SPIKE-OPENSHELL-OAS-RUNTIME.md) §8 (AuthBridge/OpenShell coexistence, untested)
