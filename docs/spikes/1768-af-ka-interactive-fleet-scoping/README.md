# Spike: Does AF↔KA interactive session bridging apply fleet tool overlay pre-scoping?

**Issue**: #1768 (Gap D)
**Date**: 2026-07-29
**Type**: Static code-path tracing (no runnable prototype needed — question is
answerable directly from production call graphs)
**Decision needed**: YES/NO — does an interactive KA session opened via AF's
`message`/`start`/`takeover` bridge for a non-hub `cluster_id` get the same
`prescopeFleetOverlay` treatment DD-FLEET-004 gives autonomous investigations?

## Answer: NO — confirmed by direct call-graph tracing

Interactive sessions (both the "Jump In" case and the fresh ad-hoc case)
**never** apply `prescopeFleetOverlay`, regardless of the investigation's
target `cluster_id`. This is Gap D as hypothesized in #1768's option 3: an
interactive session's tool calls **only ever resolve against the hub
cluster's tools**, silently, with no error surfaced.

## Evidence chain

1. `prescopeFleetOverlay` (`internal/kubernautagent/investigator/fleet_overlay.go:97`)
   has **exactly one call site** in the entire repo:
   `internal/kubernautagent/investigator/investigator.go:359`, inside
   `Investigator.Investigate(ctx, signal)` — the autonomous RCA entry point.
   Verified via `grep -rn prescopeFleetOverlay` across `internal/` and `pkg/`.

2. The interactive protocol's LLM-turn executor,
   `Investigator.RunInteractiveTurn` (`internal/kubernautagent/investigator/investigator.go:300`),
   does **not** call `prescopeFleetOverlay` — it resolves the phase client/tools
   and calls `runLLMLoop` directly with whatever `ctx` its caller supplied.

3. KA's MCP-side dispatcher for the interactive protocol,
   `InvestigateTool.dispatch` (`internal/kubernautagent/mcp/tools/investigate.go:358`),
   routes `action=start/takeover/message/complete/cancel/status/reconnect` to
   handlers in `investigate_start.go`, `investigate_takeover.go`, and the
   `message` handler — **none of which reference `ClusterID`, `cluster_id`,
   or `prescopeFleetOverlay`** (verified by grep across all three files: zero
   matches).

4. Two distinct code paths converge on the same gap, for different reasons:

   - **"Jump In" on an already-running autonomous investigation**
     (`upgradeOrCreateInteractiveSession`, `investigate_start.go:157`, the
     `found=true` branch of `t.autoMgr.FindByRemediationID`): this *does*
     reuse the session that originally called `Investigate()` (so
     `prescopeFleetOverlay` ran once, at that investigation's start) — but
     the interactive turns that follow (`handleMessage` →
     `RunInteractiveTurn`) execute on a **fresh per-MCP-request `ctx`**
     supplied by the JSON-RPC handler, not the original goroutine's `ctx`.
     Go context values do not persist across independent request/response
     boundaries — the fleet-overlay context value set by `WithFleetOverlay`
     inside the autonomous goroutine's `ctx` chain is simply absent from the
     new per-message `ctx`. Confirmed by tracing `FleetOverlayFromContext`'s
     three production call sites (`cmd/kubernautagent/toolregistry.go`,
     `internal/kubernautagent/investigator/investigator_tools.go`,
     `fleet_overlay.go` itself) — none of them re-derive or persist the
     overlay onto a new context at message-turn time.

   - **Fresh ad-hoc interactive session, no backing autonomous investigation**
     (`createFallbackSession`, `internal/kubernautagent/mcp/tools/investigate_autonomous.go:137`,
     the `found=false` branch): this constructs a **synthetic**
     `session.InvestigateFunc` that never calls `Investigator.Investigate()`
     at all — it just returns a static placeholder
     (`RCASummary: "Interactive session — awaiting user direction", InteractiveHold: true`).
     `prescopeFleetOverlay` is architecturally unreachable on this path: there
     is no `Investigate()` call for it to hang off of.

5. KA's interactive session metadata (`internal/kubernautagent/session/*.go`)
   stores only `remediation_id`, `username`, and `mode` per session — **no
   `cluster_id`** is persisted anywhere in session state, confirmed by
   grepping `Metadata[...]` usage across the session package. This isn't
   fundamental: `SignalContextResolver.ResolveSignalContext(ctx, rrID)` (an
   existing `InvestigateTool` dependency, already used by
   `handleDiscoverWorkflows`) can resolve `RemediationRequest.Spec.ClusterID`
   from `rrID` on demand — so a fix has a natural seam, it's just not wired.

## Confidence

**90%** — every link in the call chain was verified by direct source
inspection (not inference from naming or comments); the one residual
10% is whether some other, non-obvious mechanism re-injects fleet scoping
at a layer this trace didn't cover (e.g. a gateway-side session pinning
keyed by `remediation_id` outside KA's Go code). Recommend a quick manual
E2E confirmation (drive `message` against a non-hub `cluster_id` signal and
assert `kubectl_get` returns hub, not remote, data) before committing to a
fix design, but the code-level evidence alone is already strong enough to
treat Gap D as real rather than speculative.

## Implication for a fix (not implemented by this spike)

`InvestigateTool` already carries `signalResolver SignalContextResolver` as
a dependency. The natural fix seam is: resolve `ClusterID` via
`signalResolver.ResolveSignalContext(ctx, input.RRID)` in `handleStart`,
`handleTakeover`, and `handleMessage`, and apply the same
`prescopeFleetOverlay`-equivalent context decoration `Investigate()` uses,
before calling `RunInteractiveTurn`. This is scoped and additive (no new
types), but touches three handler files plus the fleet_overlay helper's
visibility (currently a private method on `*Investigator`, would need an
exported equivalent or a shared helper) — a genuine implementation task,
not a one-line fix. Left for a follow-up decision/PR; not started here per
the "spike + present confidence" instruction for this track.
