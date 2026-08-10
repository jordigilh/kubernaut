# DD-AF-012: Confidence-Gated Severity Correlation with Agent Clarification

**Status**: ✅ IMPLEMENTED on `main` (2026-08-09) — ported from `release/v1.5` (Issue #2027, PR
[#2026](https://github.com/jordigilh/kubernaut/pull/2026)) as Issue #2028's `main`/v1.6 clone. See
"Implementation results (main port)" for the Wiring Manifest, TDD results, and test IDs for this
branch.
**Date**: 2026-08-08 (root-cause correction: 2026-08-09, `release/v1.5` implemented: 2026-08-09,
`main` ported: 2026-08-09)
**Issue**: [#2027](https://github.com/jordigilh/kubernaut/issues/2027) (`release/v1.5`),
[#2028](https://github.com/jordigilh/kubernaut/issues/2028) (`main`/v1.6 clone)
**Related**: [DD-AF-001](DD-AF-001-pod-based-alert-correlation.md) (Tier 1 correlation this decision
extends), DD-AF-010 (referenced in `pkg/apifrontend/severity/triage.go`/`af_create_rr.go` for the
fail-closed-over-guessing philosophy this decision continues, no standalone doc file exists yet),
[#2018](https://github.com/jordigilh/kubernaut/issues/2018)/[#2021](https://github.com/jordigilh/kubernaut/issues/2021)
(specificity-vs-state ranking fix — confirmed correct, not the cause of this recurrence),
[#2022](https://github.com/jordigilh/kubernaut/issues/2022)/[#2025](https://github.com/jordigilh/kubernaut/issues/2025)
(`kubernaut.ai/managed` scope pattern referenced below), [#2023](https://github.com/jordigilh/kubernaut/issues/2023)/[#2047](https://github.com/jordigilh/kubernaut/issues/2047)
(harness-signal + prompt-instruction dual pattern this decision reuses)

## Context

QE reported a recurrence of #2018's symptom (a `RemediationRequest` bound to an unrelated,
persistently-firing cluster-scoped alert instead of the actual crashing resource's own signal) on a
build that already contains #2018's fix. Live evidence: RR `rr-9307edea5938-57afb3e7`,
`console-e2e-lifecycle-3-633843/Deployment/worker`, bound to `CDIDefaultStorageClassDegraded`
(cluster-scoped, unrelated) instead of `KubePodCrashLooping` (the target's own alert).

Issue #2028 confirmed the identical root-cause hypothesis applies equally to `main`: the ported
#2018/#2021 fix (`b86294ab9`, "port #2018/#2019 fixes to main") is present, and `main`'s
`bestOverallMatch` has the same specificity-ranked-ahead-of-state selection switch described below.

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

The root cause was confirmed against **live Prometheus data**, not inference — but the first pass
below (RR `rr-9307edea5938-57afb3e7`) **understated the effective margin**, corrected in the next
subsection using three more live-cluster-verified reproductions.

Querying `prometheus-k8s` (`openshift-monitoring` — the `console-e2e-alerts` `PrometheusRule` is
evaluated there, not by `prometheus-user-workload`) for the historical `ALERTS` series:

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

From this first pass alone, the apparent root cause was: Tier 1 takes a single, non-retrying
Prometheus snapshot with no tolerance for the inherent latency of Prometheus's own alerting pipeline
(metric scrape interval + rule evaluation interval + the rule's `for` duration — ~33-63s in this
fixture). This is a **new, distinct bug class from #2018**: an absence of a candidate at correlation
time, not a misranking of candidates that do exist. #2021's own original triage had already flagged
the alternative not chosen at the time ("gate the cluster-scoped fallback out of the automatic/
interactive investigation path entirely, pending a product decision") — #2027/#2028 is the evidence
that deferred decision needs resolving now.

### Correction: the effective margin is larger and more variable than ~33-63s

Three additional live-cluster-verified reproductions (one follow-up, two from QE, all re-checked
directly against `prometheus-k8s`'s historical `ALERTS` series on 2026-08-09) show the ~33-63s
figure above is not a reliable upper bound:

| Case | Margin between the resource's own alert firing and RR creation | Outcome |
|---|---|---|
| Original (`rr-9307edea5938-57afb3e7`) | negative (alert didn't exist yet) | misattributed |
| `rr-a435f12273dd-6779aee4` | **+45s** (alert already firing) | **misattributed** |
| QE: `rr-00dd9198c6a5-89a9e536` / `rr-e2d75f86f062-7422e2e3` | negative (alert pending/firing 33-63s *after* RR creation) | misattributed |
| `rr-003721461ed4-ae9f674f` (correctly-attributed RR from the same test run, for contrast) | **+165s** | correctly attributed to `KubePodCrashLooping` |

A 45-second head start on the resource's own alert was **not enough**; roughly 165 seconds was. AF
queries Thanos Querier (`thanos-querier.openshift-monitoring.svc:9091`), not Prometheus directly, for
both Tier 1 (`GetAlerts`) and Tier 1.5 (`GetRules`) — how much of this larger, variable margin is the
underlying rule's own evaluation latency versus additional lag in Thanos's alert/rule federation view
has not been isolated. The practical conclusion holds regardless: the safe margin is meaningfully
larger than a single rule's `for:` duration and is not a fixed constant, which only strengthens the
case against Option 2 below (bounded wait) and for Option 4 (surface ambiguity, ask the user).

This is also confirmed broader than the original report, per QE's two follow-up reproductions:
- **Not model-specific**: reproduced under `claude-sonnet-4-6` in addition to the original
  `claude-sonnet-5` evidence.
- **Not fixture-specific**: reproduced against the `memory-eater`/OOMKilled fixture in addition to
  the original `worker`/bad-release-command fixture — same `severity_source: cluster_firing_alert`
  mechanism, different underlying fault type.
- Separately worth flagging (not this decision's scope): the two contract-compliance E2E specs that
  hit this didn't fail, because they only assert an investigation started against the right resource,
  not that the attributed alert/severity was correct — a real user hitting this gets a
  plausible-looking but factually wrong RCA with no visible failure signal today.

### Options considered

| # | Option | Why superseded |
|---|---|---|
| 1 | Gate cluster-scoped-only matches as unconditionally insufficient (fail through Tier 1.5/2/2.5 → `ErrSeverityUndetermined`) | Would need to be ripped out once a planned (not-yet-scoped) proper cluster-scoped-alert-handling feature ships — a throwaway patch, not a durable mechanism |
| 2 | Bounded retry/wait in Tier 1 before accepting a cluster-scoped-only match | Adds latency to every cluster-scoped-only case (not just buggy ones); the observed gap is larger and more variable than first measured (45s insufficient, ~165s sufficient in live reproductions) — too large and unpredictable for a cheap bounded wait to reliably close |
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
   — same shape as #2022/#2025's `Managed: false`/`ErrResourceNotManaged` and #2023/#2047's
   grounding-guard pattern (a typed signal, not a generic error), naming the weak candidate(s) found
   so the result is informative.
3. `kubernaut_investigate`/`kubernaut_remediate`/`kubernaut_investigate_alert`'s tool result surfaces
   this to the agent (no RR created yet). `prompt.txt` gains a new instruction: when severity/signal
   correlation comes back ambiguous, the agent MUST ask the user to confirm/clarify before
   proceeding — mirroring the dual harness-signal + prompt-instruction pattern already built for
   #2023/#2047 — never silently pick for them, never silently drop the ambiguity.
4. Once the user answers, the agent re-calls the tool with an explicit confirmation carried through
   (a `confirmed_signal_name` field on each tool's Args, mirroring the existing `SignalNameOverride`
   mechanism), and Tier 1's ambiguity check is bypassed for that explicit, user-confirmed call —
   fail-closed if the confirmed name does not exactly match the candidate that was surfaced.

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

- [x] Write the ephemeral implementation plan: exact field/type changes to `TriageResult`,
      `CreateRRArgs`, `InvestigateMCPArgs`/`RemediateArgs`/`InvestigateAlertArgs`, the three tool
      Result types, and the new `prompt.txt` instruction — with a Wiring Manifest table and
      confidence score, per project methodology, before implementation begins.
- [x] TDD RED/GREEN/REFACTOR per the pyramid invariant (UT for `bestOverallMatch` confidence + the
      ambiguous-result path; IT for the full tool-call → ambiguous-result → re-call-with-confirmation
      round trip).
- [x] Port to `main` (this document, Issue #2028).

### Implementation results — `release/v1.5` (2026-08-09)

All Go-side changes and the `prompt.txt` Behavioral Constraint 7 described above were implemented
on branch `fix/2022-af-scope-validation` (same branch as #2022/#2023/PR #2026, per the
resource-constraint decision to consolidate into one PR).

**Test blast-radius note**: `defaultTestTriager()` — a shared fixture used by ~46 pre-existing,
unrelated test cases across 7 files to get *any* successful (non-ambiguous) `Triager` result — was
reworked to accept `(namespace, kind, name string)` and return a resource-scoped alert with a
verified relationship to that specific target, so it continues to resolve confidently under the new
ambiguity gate. A separate `ambiguousTestTriager()` (cluster-scoped-only, no verified relationship)
was introduced for the tests that specifically exercise DD-AF-012's ambiguous path. All ~46
call sites were updated to pass the target's own `namespace`/`kind`/`name`; no test behavior outside
the new ambiguity assertions changed.

### Implementation results — `main` port (2026-08-09, Issue #2028)

Ported onto branch `fix/2025-2047-2028-2030-af-main-port` (bundled with the `main`-tracking clones
of #2022/#2023/#2029 per the same resource-constraint consolidation decision). `main`'s
`severity`/`tools` packages structurally match `release/v1.5` closely enough that the port is a
direct application of the same design, with `main`-specific adjustments limited to: `ToolDeps`
also carries a `ScopeChecker` (from the #2025 port, bundled in the same branch/PR) and
`InvestigateConfig`'s `resolveInvestigationRR`/`createRRForInvestigation` return the ambiguous
result as an explicit third return value rather than overloading the error return, to avoid
conflating "ambiguous, needs clarification" with a genuine Go error at the call site.

**Wiring Manifest — final state (`main`):**

| Component | Production Entry Point | Wiring Code Location | Test ID | Result |
|---|---|---|---|---|
| `TriageResult.Ambiguous` / `AmbiguousSeverityError` | `severity.Triager.Triage` (called from `HandleCreateRR`) | [pkg/apifrontend/severity/triage.go](../../../pkg/apifrontend/severity/triage.go) | `UT-AF-2027-001`..`002`, `UT-AF-1369-003/2027-001a` | ✅ PASS |
| `CreateRRResult.Ambiguous` translation | `HandleCreateRR` | [pkg/apifrontend/tools/af_create_rr.go](../../../pkg/apifrontend/tools/af_create_rr.go) | `UT-AF-2028-004`, `004b` | ✅ PASS |
| `RemediateResult.Ambiguous` + `confirmed_signal_name` round trip | `kubernaut_remediate` tool call | [pkg/apifrontend/tools/ka_remediate.go](../../../pkg/apifrontend/tools/ka_remediate.go), wired in `agent/root.go`/`handler/mcp_bridge.go` | `UT-AF-2028-005` | ✅ PASS |
| `InvestigateMCPResult.Ambiguous` + `confirmed_signal_name` round trip | `kubernaut_investigate` tool call | [pkg/apifrontend/tools/ka_investigate_mcp.go](../../../pkg/apifrontend/tools/ka_investigate_mcp.go), wired in `agent/root.go` and `handler/mcp_bridge.go` | `UT-AF-2028-006` | ✅ PASS |
| `InvestigateAlertResult.Ambiguous` | `kubernaut_investigate_alert` tool call | [pkg/apifrontend/tools/af_investigate_alert.go](../../../pkg/apifrontend/tools/af_investigate_alert.go), wired in `agent/root.go` | `UT-AF-2028-007` | ✅ PASS |
| `EventSeverityTriageAmbiguous` audit emission | `Triager.Triage` | [pkg/apifrontend/audit/audit.go](../../../pkg/apifrontend/audit/audit.go) constant + emission in `triage.go` | `UT-AF-2027-008` | ✅ PASS |
| Behavioral Constraint 7 prompt text | `BuildInstruction` (embeds `prompt.txt`) | [pkg/apifrontend/agent/prompt.txt](../../../pkg/apifrontend/agent/prompt.txt) | `UT-AF-2028-010`..`014` | ✅ PASS |

CHECKPOINT W verified: all seven rows above have a production caller (confirmed via `grep` against
`cmd/`/`agent/root.go`/`handler/mcp_bridge.go`, none of it new wiring — this change modifies
existing Args/Result shapes on already-wired tools) and a passing UT proving the field is reachable
through that production entry point, not just via direct Go function calls in unit tests.

**Full validation**: `go build ./...`, `go vet ./...`, and the full `pkg/apifrontend/...`/
`cmd/apifrontend/...` test suites (agent, audit, handler, severity, tools, and all other
subpackages) pass with zero failures.

## References

- **Issue**: [#2027](https://github.com/jordigilh/kubernaut/issues/2027) (`release/v1.5`),
  [#2028](https://github.com/jordigilh/kubernaut/issues/2028) (`main`), comment with the triage
  summary: [issuecomment-5229204437](https://github.com/jordigilh/kubernaut/issues/2027#issuecomment-5229204437)
