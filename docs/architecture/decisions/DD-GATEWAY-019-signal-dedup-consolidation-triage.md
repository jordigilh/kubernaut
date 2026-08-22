# DD-GATEWAY-019: Signal Fingerprint/Deduplication Consolidation — Triage & Recommendation

**Version**: 1.0
**Created**: 2026-08-22
**Status**: 🔄 **PROPOSED** (triage complete, alternative approved by @jordigilh, implementation not yet started)
**Confidence**: 90% (triage conclusion) / 70% (specific recommended alternative)
**Related DD**: [DD-GATEWAY-011: Shared Status Ownership for Deduplication & Storm Aggregation](DD-GATEWAY-011-shared-status-deduplication.md)
**Related ADR**: [ADR-001: Gateway ↔ RO Deduplication Communication](ADR-001-gateway-ro-deduplication-communication.md)
**Tracking Issue**: (see `docs/architecture/decisions/README.md` entry / linked GitHub issue)

---

## Executive Summary

A request was made to triage whether the signal fingerprint and deduplication
logic duplicated across the **Gateway (GW)** and **ApiFrontend (AF)** services
could be consolidated into **AuthWebhook (AW)** as a single point of entry,
to reduce duplicated effort.

**Conclusion**: Moving this logic into AW is **not recommended**. AW is an
admission webhook (Allow/Deny semantics, `failurePolicy: Fail`) and would
become a hard availability dependency for the entire signal-ingestion
critical path if it took on this responsibility — a significant blast-radius
regression compared to its current narrow scope (TimeoutConfig audit,
RemediationWorkflow/AgentSession validation). It would also conflict with
the already-approved [DD-GATEWAY-011](DD-GATEWAY-011-shared-status-deduplication.md),
which established Gateway as the **exclusive writer** of
`RemediationRequest.status.deduplication`.

The triage did surface a genuine, previously-undocumented gap: Gateway's
distributed-lock protection against concurrent RR creation only covers races
**between Gateway's own replicas** — it provides no protection against a race
between **Gateway and ApiFrontend** creating an RR for the same fingerprint
concurrently. The recommended remediation is to extract the shared
dedup-check + lock orchestration into a common package that both GW and AF
call **in-process**, rather than centralizing it behind any HTTP/webhook
boundary (AW or otherwise).

---

## Context & Problem

### Current State (as of 2026-08-22, `main`)

| | Gateway (GW) | ApiFrontend (AF) | AuthWebhook (AW) |
|---|---|---|---|
| **Fingerprint algorithm** | `CalculateClusterAwareFingerprint` — `SHA256(clusterID:namespace:kind:name)` (`pkg/gateway/types/fingerprint.go`) | **Calls GW's function directly** via `gwtypes.CalculateClusterAwareFingerprint` (`pkg/apifrontend/tools/af_create_rr.go`) | none |
| **Owner-chain resolution** | Yes — `OwnerResolver.ResolveTopLevelOwner` before hashing | No — hashes the LLM-supplied `namespace/kind/name` as-is | n/a |
| **Dedup check** | `PhaseBasedDeduplicationChecker.ShouldDeduplicate` (`pkg/gateway/processing/phase_checker.go`) — lists RRs by `spec.signalFingerprint` field index, checks terminal phase / backoff / `ManualReviewRequired` | `HandleCheckExistingRR` / `checkExistingRRByFingerprint` (`pkg/apifrontend/tools/af_check_existing_rr.go`, `af_create_rr.go`) — own copy of the same field-index list + `IsTerminalPhase` check | none |
| **Concurrency control** | Cluster-wide K8s `Lease` distributed lock (`pkg/gateway/processing/distributed_lock.go`) | In-process `singleflight.Group` only (`rrCreateGroup` in `af_create_rr.go`) | none |
| **Status ownership** | Sole writer of `status.deduplication` (occurrence count, first/last seen), per DD-GATEWAY-011 | Read-only; never writes `status.deduplication` | none |
| **Trigger** | HTTP webhook handler (Alertmanager/K8s Event POST) | LLM/agent MCP tool call (`kubernaut_remediate`, `kubernaut_investigate`) | K8s admission webhook (invoked synchronously by the API server) |

Key findings from the codebase triage:

1. **The fingerprint algorithm itself is already not duplicated.** AF imports
   `pkg/gateway/types` and calls Gateway's exact hash function — the code
   comment states the intent explicitly:

   ```go
   // rrFingerprintWithCluster generates a dedup fingerprint that includes the cluster
   // context. Delegates to gwtypes.CalculateClusterAwareFingerprint to ensure GW and
   // AF produce identical fingerprints for the same resource (CC4.2: audit trail consistency).
   ```

   What *is* duplicated is the dedup-check **orchestration** (list-by-fingerprint
   + terminal-phase logic, ~2 near-identical copies of `IsTerminalPhase`) and,
   more importantly, the **absence of shared concurrency control** between GW
   and AF.

2. **AuthWebhook has zero signal-fingerprint/dedup logic today.** Its only
   superficially similar mechanism is `computeRWContentHash` in
   `pkg/authwebhook/remediationworkflow_handler.go`, which content-hashes
   `RemediationWorkflow` catalog entries via the shared `pkg/shared/contenthash`
   package for change detection — a different domain object, unrelated to
   signal identity.

3. **AW's existing RemediationRequest webhook is scoped to status updates
   only, and explicitly no-ops on CREATE.**

   ```yaml
   # charts/kubernaut/templates/authwebhook/webhooks.yaml
   - name: remediationrequest.mutate.kubernaut.ai
     rules:
       - apiGroups: ["kubernaut.ai"]
         apiVersions: ["v1alpha1"]
         operations: ["UPDATE"]
         resources: ["remediationrequests/status"]
   ```

   ```go
   // pkg/authwebhook/remediationrequest_handler.go
   } else {
       // No old object (creation) - allow without modification
       return admission.Allowed("creation allowed")
   }
   ```

   Becoming a single point of entry for dedup would require expanding this
   rule to `CREATE` on the main `remediationrequests` resource — a scope
   increase, not a refactor of existing responsibility.

---

## Alternatives Considered

### Alternative 1: Move fingerprint + dedup logic into AuthWebhook (Rejected)

**Approach**: Expand AW's `RemediationRequest` mutating webhook to intercept
`CREATE` on the main resource (not just `/status`), and perform fingerprint
canonicalization + dedup lookup (List by `spec.signalFingerprint` field
index + terminal-phase check) before admitting the object, denying creation
when an active duplicate exists.

**Pros**:
- ✅ Every creator of `RemediationRequest` (GW, AF, and any future caller —
  including `kubectl`/tests) passes through the same enforcement point,
  since admission webhooks fire on all CREATE requests regardless of caller.
- ✅ Would close the GW↔AF concurrent-create race (see below), since a single
  synchronous gate serializes all creators.

**Cons**:
- ❌ **Availability blast radius**: AW's webhooks use `failurePolicy: Fail`.
  Today an AW outage blocks TimeoutConfig edits, RemediationWorkflow
  validation, and AgentSession creation — a narrow surface. Adding CREATE-time
  dedup authority over `remediationrequests` would make an AW outage a
  **complete signal-ingestion outage for both GW and AF, cluster-wide**.
- ❌ **Wrong response contract**: Admission webhooks can only Allow or Deny.
  GW's HTTP handler must return **202 + existing RR details** on a duplicate;
  AF's tool must return `already_exists: true` + the existing `rr_id`. Under
  a Deny-based model, both callers still need to independently re-fetch the
  existing RR after the denial to build their response — the "existing-RR
  lookup" code is relocated into the error path, not eliminated.
- ❌ **Conflicts with DD-GATEWAY-011** (95% confidence, approved), which
  established Gateway as the exclusive writer of `status.deduplication`
  specifically to keep dedup state visible/auditable on the RR, following
  the K8s "shared status ownership" pattern (Node/Pod/Ingress precedent).
  `ADR-001-gateway-ro-deduplication-communication.md` evaluated 6 alternatives
  for *where dedup state should live* (SignalIngestion CRD, RawSignal
  pipeline, Redis, K8s Events, Kafka/NATS) — an admission-webhook-as-orchestrator
  was never one of them, so this is a new departure from settled architecture,
  not a cleanup of existing debt.

**Confidence**: 90% (rejected)

---

### Alternative 2: Extract shared dedup orchestration into a common package used by GW and AF in-process (Recommended)

**Approach**: Extract the dedup-check (`PhaseBasedDeduplicationChecker` /
`IsTerminalPhase`) and distributed-lock orchestration
(`DistributedLockManager`) currently in `pkg/gateway/processing/` into a
shared, service-agnostic package. Both GW and AF call it directly in-process
— AF already imports `pkg/gateway/types` for the hash function, so this
extends an existing dependency relationship rather than introducing a new
one.

**Pros**:
- ✅ Closes the real gap: AF acquiring the same K8s `Lease` as GW before
  creating an RR eliminates the GW↔AF concurrent-create race.
- ✅ Preserves DD-GATEWAY-011's status-ownership model unchanged — whichever
  caller creates/updates the RR still does so via the same shared code,
  writing `status.deduplication` the same way.
- ✅ No new availability dependency: no webhook boundary is introduced; each
  caller keeps its own failure characteristics.
- ✅ Matches existing caller response contracts — both GW's HTTP handler and
  AF's tool handler get the real existing-RR object back in-process, no
  Deny-response parsing needed.
- ✅ Actually *reduces* duplication more thoroughly than Alternative 1, since
  no per-caller "re-fetch after denial" fallback path is needed.

**Cons**:
- ⚠️ Requires deciding the exact target package location and the shape of a
  shared interface that accommodates GW's read+write dedup role vs. AF's
  read-only existence check.
- ⚠️ Touches both GW's and AF's test suites (11 GW unit/integration test
  files + 4 AF unit/integration test files reference this logic and would
  need to be re-pointed or partially relocated).

**Confidence**: 70% (approved as direction; package layout and migration
plan require follow-up design work before implementation)

---

## Decision

**APPROVED: Alternative 2** — Extract shared dedup-check + lock orchestration
into a common package consumed by GW and AF in-process. **Alternative 1
(AuthWebhook single point of entry) is rejected.**

No implementation has started as of this writing. This document exists to
capture the triage findings and motivate a tracked follow-up (see linked
GitHub issue) before any code changes are made, per this project's
CHECKPOINT DD requirement for architectural changes.

### Consequences

**Positive**:
- ✅ Closes an undocumented cross-service race condition (GW↔AF concurrent
  create for the same fingerprint) that exists today.
- ✅ Reduces genuine code duplication (dedup-check orchestration, terminal-phase
  classification) without introducing a new availability-critical component.
- ✅ Fully compatible with DD-GATEWAY-011; no changes to status ownership
  semantics.

**Negative**:
- ⚠️ Not yet scoped as an implementation plan (package location, interface
  design, test migration) — **Mitigation**: tracked as a follow-up issue;
  full DD-XXX-style alternatives-and-plan writeup expected before
  implementation begins, per CHECKPOINT DD.

**Neutral**:
- 🔄 AuthWebhook's scope and responsibilities are unchanged by this decision.

---

## Related Decisions

- **Builds On**: [DD-GATEWAY-011](DD-GATEWAY-011-shared-status-deduplication.md) — preserves its status-ownership model
- **Related**: [ADR-001](ADR-001-gateway-ro-deduplication-communication.md) — prior alternatives analysis for dedup state location (did not consider admission-webhook orchestration)
- **Related**: [DD-AUTH-001](DD-AUTH-001-shared-authentication-webhook.md) — AuthWebhook's existing scope and design rationale

### Review & Evolution

**When to Revisit**:
- If AuthWebhook's availability/reliability profile is substantially
  strengthened (e.g., HA posture equivalent to the API server itself) such
  that the blast-radius objection to Alternative 1 no longer applies.
- If a future caller of `RemediationRequest` creation emerges that cannot
  easily adopt the shared in-process package (favoring a re-evaluation of a
  centralized enforcement point).

**Success Metrics** (for the follow-up implementation):
- Zero divergence between GW's and AF's terminal-phase / dedup-check logic
  (single shared implementation).
- GW↔AF concurrent-create race demonstrably closed (integration test).
- No new hard dependency introduced on the signal-ingestion critical path.
