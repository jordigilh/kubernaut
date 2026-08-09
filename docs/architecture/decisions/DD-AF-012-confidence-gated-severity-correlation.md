# DD-AF-012: Confidence-Gated Severity Correlation with Agent Clarification

**Status**: 📋 PROPOSED — design agreed, implementation plan (Wiring Manifest + confidence score) not yet written
**Date**: 2026-08-08
**Issue**: [#2027](https://github.com/jordigilh/kubernaut/issues/2027) (`release/v1.5`), [#2028](https://github.com/jordigilh/kubernaut/issues/2028) (`main`/v1.6 clone)
**Related**: [DD-AF-001](DD-AF-001-pod-based-alert-correlation.md) (Tier 1 correlation this decision extends), DD-AF-010 (referenced in `pkg/apifrontend/severity/triage.go`/`af_create_rr.go` for the fail-closed-over-guessing philosophy this decision continues, no standalone doc file exists yet), [#2018](https://github.com/jordigilh/kubernaut/issues/2018)/[#2021](https://github.com/jordigilh/kubernaut/issues/2021) (specificity-vs-state ranking fix — confirmed correct, not the cause of this recurrence), [#2022](https://github.com/jordigilh/kubernaut/issues/2022) (`kubernaut.ai/managed` scope pattern referenced below), [#2023](https://github.com/jordigilh/kubernaut/issues/2023) (harness-signal + prompt-instruction dual pattern this decision reuses)

## Context

QE reported a recurrence of #2018's symptom (a `RemediationRequest` bound to an unrelated,
persistently-firing cluster-scoped alert instead of the actual crashing resource's own signal) on a
build that already contains #2018's fix. Live evidence: RR `rr-9307edea5938-57afb3e7`,
`console-e2e-lifecycle-3-633843/Deployment/worker`, bound to `CDIDefaultStorageClassDegraded`
(cluster-scoped, unrelated) instead of `KubePodCrashLooping` (the target's own alert).

### Root cause — live-cluster-verified, not a #2018 regression

`severity.Triager.bestOverallMatch` (`pkg/apifrontend/severity/triage.go`) was traced line-by-line
through its final selection switch:

```go
switch {
case resourceFiring.found:  return resourceFiring.result(SourceFiringAlert), true
case resourcePending.found: return resourcePending.result(SourcePendingAlert), true
case nsFiring.found:        return nsFiring.result(SourceNSFiringAlert), true
case nsPending.found:       return nsPending.result(SourceNSPendingAlert), true
case clusterFiring.found:   return clusterFiring.result(SourceClusterFiringAlert), true
case clusterPending.found:  return clusterPending.result(SourceClusterPendingAlert), true
}
```

Specificity (resource > namespace > cluster) is correctly ranked strictly ahead of state
(firing > pending) — `resourcePending`/`nsPending` are checked **before** `clusterFiring`. Two
already-passing tests assert exactly the scenario the reporter believed should have won:
`UT-AF-2018-001` (pending resource-specific alert beats firing cluster-scoped) and `UT-AF-2018-002`
(pending namespace-scoped alert beats firing cluster-scoped), both in
`pkg/apifrontend/severity/triage_test.go`. **This refutes the issue's original hypothesis** that
pending alerts are still filtered out of candidacy before ranking runs.

The actual root cause was confirmed against **live Prometheus data**, not inference. Querying
`prometheus-k8s` (`openshift-monitoring` — the `console-e2e-alerts` `PrometheusRule` is evaluated
there, not by `prometheus-user-workload`) for the historical `ALERTS` series:

| Event | Timestamp (UTC) | Delta from trigger |
|---|---|---|
| AF's Tier 1 correlation runs (RR `firingTime`/creation) | `23:14:27Z` | T+0 |
| `KubePodCrashLooping{pod=worker-7986b99df8-gpjgw}` goes **pending** | `23:15:00Z` | **T+33s** |
| Same alert instance transitions to **firing** (`for: 30s` elapsed) | `23:15:30Z` | T+63s |
| `CDIDefaultStorageClassDegraded` (cluster-scoped, no `namespace` label) | firing continuously the entire window | — |

At the exact moment Tier 1 took its single Prometheus snapshot, the target's own alert did not exist
yet — not pending, not firing, zero instances. There was no higher-specificity candidate at all for
`bestOverallMatch` to rank; it correctly fell back to the only candidate present.

Cross-checked against apifrontend logs for the initiating call:

```
23:14:25.109Z  tool call started: kubernaut_investigate  →  FAILED: "api_version must not be empty"
23:14:27.740Z  tool call started: kubernaut_investigate  →  succeeded (retry, 2.6s later)
```

No alert was named in the request at all — a plain resource-based call
(`kubernaut_investigate(namespace=console-e2e-lifecycle-3-633843, kind=Deployment, name=worker)`).
The alert identity is invented entirely by `severity.Triager` — **deterministic Go code inside
`HandleCreateRR`**, not an LLM choice, made before any KA/AA session or LLM reasoning ever sees the
resource. (Minor, unrelated observation: the agent's first tool-call attempt omitted `api_version`
and failed validation, self-corrected 2.6s later — not connected to the mis-correlation.)

**Root cause**: Tier 1 takes a single, non-retrying Prometheus snapshot with no tolerance for the
inherent latency of Prometheus's own alerting pipeline (metric scrape interval + rule evaluation
interval + the rule's `for` duration — ~33-63s in this fixture). When an agent-initiated
investigation is triggered near-instantly after a failure begins, Tier 1 has zero resource/
namespace-scoped candidates and, under the current design, treats a completely unrelated
cluster-scoped alert as equally valid evidence — with no notion of confidence to say otherwise. This
is a **new, distinct bug class from #2018**: an absence of a candidate at correlation time, not a
misranking of candidates that do exist. #2021's own original triage had already flagged the
alternative not chosen at the time ("gate the cluster-scoped fallback out of the automatic/
interactive investigation path entirely, pending a product decision") — #2027 is the evidence that
deferred decision needs resolving now.

### Options considered

| # | Option | Why superseded |
|---|---|---|
| 1 | Gate cluster-scoped-only matches as unconditionally insufficient (fail through Tier 1.5/2/2.5 → `ErrSeverityUndetermined`) | Would need to be ripped out once a planned (not-yet-scoped) proper cluster-scoped-alert-handling feature ships — a throwaway patch, not a durable mechanism |
| 2 | Bounded retry/wait in Tier 1 before accepting a cluster-scoped-only match | Adds latency to every cluster-scoped-only case (not just buggy ones); the observed gap (33-63s) is too large for a cheap bounded wait to reliably close |
| 3 | Confidence-score the match tiers, route low-confidence to the existing `ManualReviewRequired`/`HumanReviewReason` mechanism | Correct direction, but discovery happens too late — a *different* human (a reviewer, asynchronously) resolves the ambiguity instead of the actual user who is live, mid-conversation, right now |
| 4 | **Chosen**: surface the ambiguity back through the tool result to the calling agent, in the same conversational turn, and instruct it to ask the live user for clarification | Reuses the existing conversational session (no new synchronous-pause plumbing); asks the person who actually has the context, in real time; naturally extensible as future correlation signals are added |

Key clarifying facts established during discussion:
- The decision is made by deterministic Go code (`bestOverallMatch`), not an LLM — there is nothing
  a smarter model could have done differently; the ambiguity must be surfaced structurally.
- The **signal-initiated path** (Alertmanager → Gateway → RO) is out of scope: Gateway already
  rejects cluster-scoped alerts outright today, so this race cannot manifest there.
- Existing precedent for "the caller supplies a definitive signal identity instead of letting
  Triager guess" already exists: `CreateRRArgs.SignalNameOverride`
  (`pkg/apifrontend/tools/af_create_rr.go`), used today by `kubernaut_investigate_alert`. The
  re-call-with-confirmation flow below extends this pattern rather than inventing a parallel one.

## Decision

1. `severity.Triager`/`bestOverallMatch` gains an explicit confidence notion per tier (the
   `TriageResult.Confidence float64` field already exists but is currently only populated by the
   Tier 2.5 LLM path) — resource-scoped ≈ high, namespace-scoped ≈ medium, cluster-scoped-with-zero-
   correlation ≈ low/ambiguous.
2. When Tier 1's *only* candidate is cluster-scoped (no resource/namespace-scoped candidate found at
   all), `Triage()`/`HandleCreateRR` does **not** silently write that candidate into the RR's
   `severity`/`signalName` as fact. It returns a structured "ambiguous — needs clarification" result
   — same shape as #2022's `Managed: false` and #2023's grounding-guard pattern (a typed signal, not
   a generic error), naming the weak candidate(s) found so the result is informative.
3. `kubernaut_investigate`/`kubernaut_remediate`/`kubernaut_investigate_alert`'s tool result surfaces
   this to the agent (no RR created yet). `prompt.txt` gains a new instruction: when severity/signal
   correlation comes back ambiguous, the agent MUST ask the user to confirm/clarify before
   proceeding — mirroring the dual harness-signal + prompt-instruction pattern already built for
   #2023 — never silently pick for them, never silently drop the ambiguity.
4. Once the user answers, the agent re-calls the tool with an explicit confirmation carried through
   (extending the existing `SignalNameOverride` mechanism, or a sibling field for "proceed without
   attributing any alert" — exact shape TBD in the implementation plan), and Tier 1's ambiguity
   check is bypassed for that explicit, user-confirmed call.

### Scope boundaries

- **In scope**: `kubernaut_investigate`, `kubernaut_remediate`, `kubernaut_investigate_alert` —
  agent-initiated paths where a live conversational session exists.
- **Out of scope**: the fully automatic signal-initiated path (Alertmanager → Gateway → RO). Gateway
  already rejects cluster-scoped signals before they ever reach RO, so this specific race cannot
  occur there today.
- **Out of scope**: the broader "how should Kubernaut properly support cluster-scoped alerts as
  legitimate correlation evidence" feature — tracked as a separate, not-yet-scoped issue. This
  design is intentionally durable against that future work: it extends via new ways to *earn*
  confidence (e.g. a future resource-to-cluster-component dependency check), not via removing a
  hardcoded gate.

### Preliminary FedRAMP control mapping

| Control | Application |
|---|---|
| SI-10 (Information Input Validation) | A correlation with zero verified relationship to the target resource is no longer accepted as validated input to severity/signal attribution |
| AU-3 / AU-12 (Audit Content / Generation) | The RR's `signalName`/`severity` — which becomes part of the permanent audit trail once created — will no longer misattribute an investigation to an unrelated alert; an ambiguous outcome is now explicitly recorded and resolved by the user, not silently guessed |
| AC-6 (Least Privilege) / CM-3 | Mirrors #2019's principle: a decision presented-but-unconfirmed (here, a *candidate* signal identity) must never be treated as user-approved fact |

## Next Steps

- [ ] Write the ephemeral implementation plan: exact field/type changes to `TriageResult`,
      `CreateRRArgs`, `InvestigateMCPArgs`/`RemediateArgs`/`InvestigateAlertArgs`, the three tool
      Result types, and the new `prompt.txt` instruction — with a Wiring Manifest table and
      confidence score, per project methodology, before implementation begins.
- [ ] TDD RED/GREEN/REFACTOR per the pyramid invariant (UT for `bestOverallMatch` confidence + the
      ambiguous-result path; IT for the full tool-call → ambiguous-result → re-call-with-confirmation
      round trip).

## References

- **Issue**: [#2027](https://github.com/jordigilh/kubernaut/issues/2027), comment with this
  triage summary: [issuecomment-5229204437](https://github.com/jordigilh/kubernaut/issues/2027#issuecomment-5229204437)
