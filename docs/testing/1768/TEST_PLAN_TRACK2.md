# Test Plan: AF↔KA Interactive Bridge Fleet Cluster-Scoping (Gap D)

**Issue**: [#1768](https://github.com/jordigilh/kubernaut/issues/1768) (Track 2 — Gap D)
**Authority**: [DD-FLEET-004](../../architecture/decisions/DD-FLEET-004-cluster-transparent-tool-exposure.md), [spike findings](../../spikes/1768-af-ka-interactive-fleet-scoping/README.md)
**Business Requirements**: BR-INTEGRATION-054, BR-FLEET-054
**Branch**: `fix/1768-track2-interactive-fleet-scoping`
**Created**: 2026-07-30
**Status**: Active

---

## 1. Purpose

The [spike](../../spikes/1768-af-ka-interactive-fleet-scoping/README.md) confirmed (90%
confidence, now re-verified against current `main` at 95%+) that KA's interactive
investigation path — `InvestigateTool.handleMessage` → `Investigator.RunInteractiveTurn` —
never applies `prescopeFleetOverlay`, the same DD-FLEET-004 server-side cluster pre-scoping
`Investigate()` applies for autonomous RCA. An interactive investigation (opened via AF's
takeover/message bridge) for a non-hub `cluster_id` therefore silently resolves tool calls
against the **hub** cluster's tools instead of the target remote cluster's — with no error,
no audit trace, and no indication to the operator that their query landed on the wrong
cluster.

## 2. Design refinement since the spike (re-verified against current `main`)

The spike estimated the fix would touch three handler files (`handleStart`, `handleTakeover`,
`handleMessage`). Fresh call-graph verification against current `main` narrows this:

- `RunInteractiveTurn` has exactly **one** production call site:
  `internal/kubernautagent/mcp/tools/investigate_takeover.go:133`, inside `handleMessage`
  (`handleStart`/`handleTakeover` never call it directly — `handleStart`'s fallback session
  uses a synthetic no-op `InvestigateFunc`, and `handleTakeover` only transitions session
  ownership before the *next* `message` call actually runs a turn).
- `handleMessage` **already** resolves the target `ClusterID` on every turn (#1374/F9,
  `investigate_takeover.go:110-114`): `t.signalResolver.ResolveSignalContext(ctx, input.RRID)`
  → `ctx = katypes.WithSignalContext(ctx, *resolved)`. `SessionSignalContextResolver`
  (`internal/kubernautagent/mcp/adapters/signal_resolver.go:68-92`) returns a real
  `ClusterID` for **both** the "jump in on an autonomous session" case (from the persisted AA
  payload) and the "fresh ad-hoc interactive session" case (CRD fallback to
  `RemediationRequest.Spec.ClusterID`) — so this single resolution point already covers both
  of the spike's two problematic code paths uniformly.
- `Investigator.prescopeFleetOverlay` is a private method **in the same package**
  (`internal/kubernautagent/investigator`) as `RunInteractiveTurn` — no exported-visibility
  change is needed to call it from there, unlike calling it cross-package from `mcp/tools`.
- Tool resolution (`toolDefinitionsForPhase`, `executeResolved`) already reads
  `FleetOverlayFromContext(ctx)` generically — it does not care whether the overlay was set by
  `Investigate()` or `RunInteractiveTurn()`.

**Net effect**: the fix is a single-file change to `RunInteractiveTurn` (extract
`SignalContext` from `ctx`, call the existing `prescopeFleetOverlay` before `runLLMLoop`),
not three handler files. This is a pure refinement of Alt A (same resolver, same overlay
mechanism, same fail-open semantics) discovered by re-tracing the call graph at
implementation time — no change to the previously-approved design intent.

## 3. FedRAMP Control Mapping

| Control | Title | Relevance |
|---|---|---|
| **AC-4** | Information Flow Enforcement | Primary. Ensures an interactive investigation's tool calls are enforced against the *operator's actual target cluster*, not silently misrouted to the hub — closing the exact boundary DD-FLEET-004 established for the autonomous path. |
| **AC-6** | Least Privilege | The LLM's tool schema stays byte-identical regardless of which cluster backs it (existing `toolDefinitionsForPhase` guarantee) — this fix extends that guarantee to interactive turns, so an interactive session's LLM never gains visibility into cluster topology it shouldn't reason about. |
| **AU-3** | Content of Audit Records | `prescopeFleetOverlay` failure path already emits `EventTypeFleetOverlayFailed` (AU-3) with `cluster_id` + `correlation_id`; this fix makes that audit path reachable from interactive sessions too (previously unreachable, since the resolver was never invoked there). |

## 4. Pyramid Invariant — Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | Control | BR | Test File |
|---|---|---|---|---|---|
| UT-KA-FLEET-026 | UT | `RunInteractiveTurn`, given a `ctx` carrying `SignalContext{ClusterID: "remote-cluster"}` and a `FleetOverlayResolver` that maps `kubectl_get` to a remote-cluster tool double, executes the LLM's `kubectl_get` call via the overlay tool, not the local registry's tool of the same name | AC-4, AC-6 | BR-FLEET-054 | `internal/kubernautagent/investigator/interactive_fleet_overlay_test.go` |
| UT-KA-FLEET-027 | UT | `RunInteractiveTurn`, given a `ctx` with **no** `SignalContext` (or `ClusterID == ""`) — the hub-local/regression case — behaves identically to pre-fix: no overlay applied, local registry tool executes, zero behavior change | AC-4 | BR-FLEET-054 | `internal/kubernautagent/investigator/interactive_fleet_overlay_test.go` |
| IT-KA-FLEET-022 | IT | Full wiring: `InvestigateTool.handleMessage` (real `signalResolver` resolving a non-hub `cluster_id` via CRD fallback) → real `Investigator.RunInteractiveTurn` → LLM calls `kubectl_get` → resolves through a fake `FleetOverlayResolver`'s remote tool, proving the production dispatch path (not just the isolated investigator method) | AC-4 | BR-INTEGRATION-054, BR-FLEET-054 | `internal/kubernautagent/mcp/tools/interactive_fleet_overlay_wiring_test.go` |

## 5. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|---|---|---|---|
| Fleet-overlay pre-scoping for interactive turns | `InvestigateTool.handleMessage` (MCP `investigate` tool, `action=message`) | `internal/kubernautagent/investigator/investigator.go` (`RunInteractiveTurn`, calling existing `prescopeFleetOverlay`) | IT-KA-FLEET-022 |

No new production types are introduced. `FleetOverlayResolver`, `prescopeFleetOverlay`,
`WithFleetOverlay`/`FleetOverlayFromContext`, `SessionSignalContextResolver`, and
`katypes.WithSignalContext`/`SignalContextFromContext` all already exist and are reused
unchanged.

## 6. Out of Scope

- `ReconRunnerAdapter.RunReconTurn` (reconstruction-turn replay after session
  handoff/reconnect, #1384/#1389) also calls `RunInteractiveTurn` but does not currently set
  `SignalContext` on its `ctx`. This fix fails open for that path (identical to today's
  behavior — no regression), but does not extend fleet scoping to reconstruction turns.
  Tracked as a follow-up if reconstruction ever needs live remote-cluster tool execution
  (today it replays already-completed history, not new tool calls).
- Track 4 / issue #1729 (KA Helm-chart parity) remains a separate prerequisite issue for
  testing this fix through full production Helm wiring; this plan's IT proves the Go-level
  wiring only.

## 7. Coverage Target

UT + IT tiers only (E2E deferred until #1729 unblocks production Helm wiring for KA's fleet
config in interactive scenarios). Both control objectives (AC-4, AC-6) get at least one UT
proving journey; AC-4's wiring gets an IT proving journey per the Wiring Manifest above.
