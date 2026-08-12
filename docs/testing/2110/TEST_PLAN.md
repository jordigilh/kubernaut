# Test Plan: #2110 — `present_decision` SSE Artifact Crashes gob `DeepCopy` (`v1.5.6-rc4` Regression)

## 1. Purpose

`v1.5.6-rc4` introduced a release-blocking regression: **every**
`kubernaut_present_decision` call with grounded RCA content crashed the a2a
task, permanently breaking SSE delivery of the `investigation_summary`
artifact for the entire interactive approve/decline/dismiss flow.

Reported by QE ([#2110](https://github.com/jordigilh/kubernaut/issues/2110)),
confirmed via the `kubernaut-console` live E2E suite against a verified
`v1.5.6-rc4` deployment, and independently reproduced live against this
project's own dev cluster (`apifrontend` pod running the exact digest QE
reported): 7/7 grounded `present_decision` attempts crashed identically over
the pod's lifetime, 0 successful grounded completions.

### Root Cause Detail

`a2aproject/a2a-go@v0.3.15`'s task manager deep-copies every
`TaskArtifactUpdateEvent`'s artifact via a gob encode/decode round-trip
before fanning it out to subscriber goroutines
(`internal/utils/utils.go:28`'s `DeepCopy`, called from
`internal/taskupdate/manager.go`). `encoding/gob` requires any concrete type
stored behind an `interface{}` to be registered via `gob.Register` before it
can be encoded/decoded.

`pkg/apifrontend/agent/phase_guard.go`'s `canonicalGroundedRCA` returned a
raw `*tools.RCAData` struct pointer, which `enforceGroundingGuard` assigned
directly into `args["rca"]` (a `map[string]any`). `tools.RCAData` is never
`gob.Register`'d anywhere in this repo (confirmed: zero results for
`gob.Register` prior to this fix), so every grounded
`kubernaut_present_decision` call crashed with `gob: type not registered for
interface: tools.RCAData`, cascading into: RBAC guard denial (`SAR
authorization check failed: ... context canceled`), `failed to call model:
context canceled`, and a silently-dead task that never emits its artifact.

This assignment pattern existed since `canonicalGroundedRCA` was introduced,
but only became reachable by the SSE artifact pipeline once
`sanitizePresentDecisionResponse` (#2105, v1.6 clone #2106) started applying
`enforceGroundingGuard`'s mutation to the model's raw `FunctionCall.Args`
*before* ADK yields that response as an SSE event — previously,
`part_converter.go`'s `emitDecisionEvent` built the artifact from the
model's own raw, JSON-native args, so the struct-typed substitution never
reached the gob-encoded pipeline. #2105's own verification
(`E2E-AF-1396-001`, mock-LLM E2E harness) never caught this because that
harness doesn't exercise the real `a2a-go` task manager's gob-based artifact
fan-out — confirmed by a spike (below) showing gob's failure mode depends on
which concrete types are pre-registered by `a2a-go`'s own
`internal/taskstore/store.go` `init()` (`map[string]any` and `[]any` are;
`tools.RCAData` never was).

### Why `map[string]any` Is Safe But `*tools.RCAData` Is Not (Spike Evidence)

A local spike (`/tmp/gobspike`, not committed) confirmed the precise
mechanics before implementing the fix:

- Encoding a struct with a `map[string]any` field whose value is a
  `*RCAData`-shaped struct pointer fails with exactly the reported error,
  regardless of nesting depth.
- Encoding the same structure with a plain nested `map[string]any` value
  instead **also fails** unless `map[string]any`/`[]any` are pre-registered
  — which is exactly what `a2a-go@v0.3.15`'s
  `internal/taskstore/store.go:68-69` does at `init()`:
  `gob.Register(map[string]any{})`, `gob.Register([]any{})`. This package is
  already transitively imported via `pkg/apifrontend/launcher`'s own use of
  `a2a-go` types, so by the time `EventBridge.EmitArtifact` runs in
  production, `map[string]any` is already safe to gob-encode — only
  `tools.RCAData` (a type local to this repo, never registered by anyone)
  was the actual gap.

## 2. Fix Design

**Rejected alternative**: `gob.Register(&tools.RCAData{})` alone, without
changing the assignment site. Rejected as the *sole* fix (though added
below as defense-in-depth) because it doesn't address the underlying
type-safety gap: any other struct type introduced in the future by a
similar pass-through pattern would reintroduce the exact same crash class
one call site at a time.

**Chosen fix**: convert `canonicalGroundedRCA`'s return type from
`*tools.RCAData` to a plain `map[string]any`, matching the pattern already
used elsewhere in this same package by `emitFallbackInvestigationArtifact`
(`pkg/apifrontend/tools/ka_investigate_mcp.go`):

```go
func canonicalGroundedRCA(rca *tools.InvestigateRCA) map[string]any {
    if rca == nil || rca.Severity == "" {
        return nil
    }
    return map[string]any{
        "severity":         rca.Severity,
        "confidence":       rca.Confidence,
        "causal_chain":     rca.CausalChain,
        "target":           rca.Target,
        "tool_calls_count": rca.TotalToolCalls,
        "llm_turns":        rca.TotalLLMTurns,
    }
}
```

Because both branches of `enforceGroundingGuard`'s grounded-content handling
now produce the same concrete type (`map[string]any`), the pre-existing
`_, isRCAData := args["rca"].(*tools.RCAData)` type-assertion (used to skip
the #2073 zero-backfill branch when the canonical substitution already ran)
is no longer able to distinguish the two cases by Go type. Replaced with an
explicit `canonicalSubstituted bool` flag set at the point of substitution.

**Defense-in-depth** (per QE's own suggestion in #2110): also added
`gob.Register(&RCAData{})` in an `init()` in `pkg/apifrontend/tools/ka_tools.go`
(where `RCAData` is defined), so that if some future code path violates the
`map[string]any` convention and reintroduces a raw struct pointer into an
SSE artifact, it fails safe (a working gob round-trip) rather than crashing
the a2a task outright. Documented explicitly as *not* a substitute for the
type-conversion fix itself.

## 3. FedRAMP Control Mapping

| Control | Title | Relevance |
|---|---|---|
| **SI-10** | Information Input/Output Validation | The core control: the SSE artifact pipeline must be able to actually deliver the structured `investigation_summary` artifact it constructs — a type that can't survive the transport's own serialization step is a data-integrity failure at the point of delivery. |
| **AU-3** | Content of Audit Records | `kubernaut_present_decision`'s SSE artifact is this system's audit-visible decision record (#1408's structured-artifact mandate); this bug silently dropped that record for every grounded decision in `v1.5.6-rc4`, defeating AU-3's reconstruction guarantee for the entire interactive approve/decline/dismiss flow. |
| **SI-11** | Error Handling | The cascading failure (task moved to failed state → RBAC denial → context canceled → stream closed) surfaced no actionable error to the Console client — the task simply died silently from the user's perspective. |

## 4. Pyramid Invariant — Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | Control | Test File |
|---|---|---|---|---|
| UT-AF-2110-001 (regression) | Unit | The `rca` value `enforceGroundingGuard`'s `BeforeToolCallback` path substitutes for a fully-grounded investigate result is gob-encodable in the exact `a2a.DataPart.Data` shape `EventBridge.EmitArtifact` produces | SI-10 | `pkg/apifrontend/agent/present_decision_rca_gob_2110_test.go` |
| UT-AF-2110-002 (regression) | Unit | The same is true through `sanitizePresentDecisionResponse`'s `AfterModelCallback` path — the actual runtime path that crashed in production (closes the exact gap #2105's own tests missed: they never populated `session.StateKeyGroundedRCA` with a genuine `*tools.InvestigateRCA`) | SI-10, AU-3 | `pkg/apifrontend/agent/present_decision_rca_gob_2110_test.go` |
| UT-AF-2110-003 | Unit | `enforceGroundingGuard`'s grounded `rca` substitution is a `map[string]any`, not a `*tools.RCAData` struct pointer, and preserves all six substituted fields' values | SI-10 | `pkg/apifrontend/agent/present_decision_rca_gob_2110_test.go` |
| UT-AF-2023-010 (regression guard, updated) | Unit | "Overwrites `present_decision`'s `rca` argument with KA's own reported RCA" — updated to assert the gob-safe `map[string]any` type instead of the buggy `*tools.RCAData` type it previously (incorrectly) required | SI-10 | `pkg/apifrontend/agent/phase_guard_test.go` |
| UT-AF-2068-004 (regression guard, updated) | Unit | "Still overwrites `present_decision`'s `rca` when genuinely KA-reported" — same type-assertion update as above | SI-10 | `pkg/apifrontend/agent/phase_guard_test.go` |

### Tier Coverage Rationale

- **UT** covers this fix completely. The actual crash is a `encoding/gob`
  stdlib behavior triggered by a concrete Go type reaching an
  `interface{}`-typed field — this is precisely a unit-testable property of
  the data this package produces, independent of `a2a-go`'s specific
  internal call sites (`internal/taskupdate.Manager`'s `DeepCopy` is an
  unexported, unimportable dependency-internal type, so it cannot be
  invoked directly from an external test; the tests instead gob-encode the
  identical `map[string]any` shape `EventBridge.EmitArtifact`
  (`pkg/apifrontend/launcher/event_bridge.go`) hands to `a2a.DataPart.Data`,
  which is the literal operation that crashed in production, using the same
  `gob.Register(map[string]any{})`/`gob.Register([]any{})` calls `a2a-go`'s
  own `internal/taskstore/store.go` performs at `init()`).
- **IT**: not added net-new. This bug is not a wiring gap (the
  `BeforeToolCallback`/`AfterModelCallback` registration wiring itself was
  already correct and already IT/UT-covered by #2098/#2105) — it is a
  data-shape bug in an existing, already-wired code path. The existing
  `present_decision_ordering_2098_it_test.go`/`present_decision_response_sanitize_2105_test.go`
  integration-style tests continue to pass unmodified and implicitly cover
  the wiring; UT-AF-2110-001/002 above close the specific type-safety gap
  they didn't.
- **E2E**: not added net-new in this repo, per the root-cause analysis
  above — this failure mode is only observable against a real `a2a-go` task
  manager + real client, which is exactly what the reporting
  `kubernaut-console` live E2E suite already is. QE's next acceptance run
  against this branch's images is the E2E confirmation path.

## 5. Wiring Manifest

Not applicable — this is a bug fix to existing, already-wired production
code (`enforceGroundingGuard`, `canonicalGroundedRCA`, both already called
from `phaseGuardBefore` and `sanitizePresentDecisionResponse`), not a new
component. No new production entry points are introduced.

## 6. CHECKPOINT W Evidence

Not applicable (see Section 5) — no new wiring to verify. Confirmed instead
via `go build ./... && go vet ./...` (below) that both existing call sites of
`canonicalGroundedRCA` (`enforceGroundingGuard`'s grounded branch,
`internal/utils/utils.go`'s `DeepCopy`'s two-caller build) still compile
cleanly against the new `map[string]any` return type.

## 7. Build Validation

Executed 2026-08-11:

```bash
$ go build ./pkg/apifrontend/...                                          # PASS
$ go vet ./pkg/apifrontend/...                                            # PASS (no output)
$ go test ./pkg/apifrontend/agent/...                                     # PASS: 245/245 specs
$ go test ./pkg/apifrontend/tools/...                                     # PASS
$ go test ./pkg/apifrontend/launcher/...                                  # PASS
$ go test ./pkg/apifrontend/...                                           # PASS: all 24 packages
```

## 8. Coverage Summary

- Unit: 245/245 specs passed in `pkg/apifrontend/agent` (includes 3 new
  #2110 regression specs plus 2 updated pre-existing regression guards that
  previously (incorrectly) asserted the buggy `*tools.RCAData` type).
- No regressions introduced in any pre-existing test across the full
  `pkg/apifrontend/...` tree (24/24 packages passing).
- Live-cluster confirmation: reproduced the exact reported crash signature
  (`gob: type not registered for interface: tools.RCAData`) against this
  project's dev cluster prior to the fix (7/7 occurrences over the running
  pod's lifetime, matching QE's reported digest exactly); this Test Plan's
  fix removes the code path that produced it.

## 9. Out of Scope

- Standing up a full `a2asrv` server (`a2a-go`'s exported request-handling
  stack) purely to exercise the real, unexported `internal/taskupdate.Manager`'s
  `DeepCopy` end-to-end. Rejected as disproportionate: the actual crashing
  operation is a stdlib `encoding/gob` behavior fully reproducible (and
  already reproduced, see UT-AF-2110-001/002) without that dependency's
  internal machinery, and the reporting `kubernaut-console` live E2E suite
  already provides real end-to-end coverage of the full stack.
- Auditing the rest of this repo for other structs assigned into `any`-typed
  fields that might reach the same SSE artifact pipeline. The
  `gob.Register(&RCAData{})` defense-in-depth (Section 2) mitigates the
  immediate risk class; a broader audit is a separate, larger effort not
  required to close this specific release-blocking regression.

## 10. Sign-off

| Role | Name | Date | Signature |
|---|---|---|---|
| Author | AI Assistant | 2026-08-11 | pending |
| Reviewer | Jordi Gil | | pending |
| Approver | | | pending |
