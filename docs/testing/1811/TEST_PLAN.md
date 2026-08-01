# Test Plan: Late-Subscribe Event Replay for AA/AF Interactive-Upgrade Race

**Issue**: [#1811](https://github.com/jordigilh/kubernaut/issues/1811)
**Authority**: BR-INTERACTIVE-004 (dynamic takeover / in-place upgrade), BR-INTERACTIVE-010 (deferred interactive sessions)
**Business Requirements**: BR-INTERACTIVE-004, BR-INTERACTIVE-010
**Branch**: `fix/1811-late-subscribe-event-replay`
**Created**: 2026-08-01
**Status**: Complete — all scenarios in Section 4 pass; see Section 7 for final results

---

## 1. Purpose

Issue #1811 identified that `E2E-FP-1189-005` fails 0/3 in CI: when AA submits an
investigation non-interactively first (`Manager.StartInvestigation`), and KA's RCA loop
resolves fast enough to hit `checkRCAEarlyReturn`'s `InteractiveHold` short-circuit before
AF's `kubernaut_investigate` call reaches `Manager.Subscribe`, every event emitted during
that window (`emitToSink` reasoning/tool events, `emitCompleteEvent`'s terminal event) is
permanently dropped — `LazySink.Get()` returns `nil` and the emitter silently no-ops. AF's
`bridgeEventsCollectSummary` then blocks for the full inactivity timeout and returns
`summary_len=0`.

### Root cause (confirmed via code read, not just log inference)

`Manager.handleInvestigationSuccess` (`internal/kubernautagent/session/manager.go:284`)
already anticipates a late `Subscribe` after `InteractiveHold`: it sets
`StatusUserDriving` (non-terminal) and deliberately skips `closeEventChan` for that status,
specifically so a later `Subscribe` call still succeeds and returns a live channel. **The
missing piece is that nothing buffers what was emitted before that channel existed** —
`LazySink.Get()` (`internal/kubernautagent/session/event_sink.go:44`) either returns the
channel or `nil`; there is no in-between state. Three call sites independently do
"get channel, non-blocking-send, else silently drop" with no memory of drops:
`emitToSink` (`internal/kubernautagent/investigator/investigator_loop.go:464`),
`emitCompleteEvent` and `emitTerminalEvent`
(`internal/kubernautagent/session/manager_events.go:65,105`).

This is a **completion of an existing, partially-built mechanism**, not a new architecture:
the kept-open-channel design for `StatusUserDriving` already exists; it just never got the
event-buffering half needed to make a late `Subscribe` actually deliver anything.

An existing spike (`internal/kubernautagent/session/production_flow_spike_test.go`,
`H_TIMING_PROD`) proves the sibling "pending session" path (`StartInteractiveSession` →
`LaunchDeferredInvestigation` → `Subscribe`, all triggered back-to-back by the same MCP
`action=start` call) tolerates this drop because the race window is microseconds. #1811's
path (`StartInvestigation` runs synchronously to completion, `UpgradeToInteractive` and
`Subscribe` are two independent, unsynchronized calls from a completely different code path)
has no such back-to-back guarantee — the window is the investigation's entire first RCA turn.

## 2. Fix Design

Add a bounded replay buffer to `LazySink` (`internal/kubernautagent/session/event_sink.go`):

- `LazySink.Emit(event) bool` — single critical section that appends to a capped
  ring buffer (cap matches `eventChannelBuffer`=64) when no channel is attached yet, or
  attempts the existing non-blocking send when one is. Replaces the "Get() then the caller
  manually selects" pattern at all three call sites.
- `LazySink.Set(ch)` — on activation, replay the buffered events (oldest-first,
  non-blocking, respecting the new channel's own buffer) before accepting new live sends.
- New `session.EmitEvent(ctx, event) bool` context-based entry point so
  `investigator`'s `emitToSink` dispatcher (and `manager_events.go`'s two call sites) route
  through the same buffering logic.

No behavior change for the already-working case (sink active before emission — the common,
already-well-tested interactive path): `Emit` sends immediately exactly as today. Only the
"sink was nil at emit time" case changes, from "permanently lost" to "delivered on first
subsequent `Subscribe`". Buffer is bounded and freed with the session; purely autonomous
investigations that are never subscribed to pay a small bounded append cost per emission
(no unbounded growth — oldest entries evicted past cap).

Out of scope (separate, harder problem, not what #1811 describes): a session that reaches a
**truly terminal** status (`StatusCompleted`/`Failed`/`Cancelled`, channel already closed via
`closeEventChan`) before any `Subscribe` call. `Subscribe` already fails fast with
`ErrSessionTerminal` in that case (`manager_query.go:152`) and callers fall back to
`ForceTransitionToUserDriving`/`createFallbackSession` — that fallback path is unchanged.

## 3. FedRAMP Control Mapping

| Control | Title | Relevance |
|---|---|---|
| **SI-4** | Information System Monitoring | An operator's live interactive session must not silently lose all investigation-progress visibility due to an internal scheduling race — this is the monitoring-completeness guarantee the fix restores. |
| **AU-3** | Content of Audit Records | `emitCompleteEvent`'s replayed payload carries `MarshalRCASubset(result)` (the bounded RCA subset) so a late subscriber's terminal event still carries real content, not an empty placeholder (mirrors the #1794 fix's intent for the terminal-event content guarantee). |

This is a live-progress-stream reliability fix (BR-INTERACTIVE-004/010), not a change to the
`audit.AuditStore` compliance trail — the two are separate mechanisms in this codebase.

## 4. Pyramid Invariant — Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | Control | BR | Test File |
|---|---|---|---|---|---|
| UT-KA-1811-001 | Unit | `LazySink.Emit` buffers events (bounded, oldest-evicted) when no channel is attached, and `Set` replays them in order to a newly-activated channel | SI-4 | BR-INTERACTIVE-004 | `internal/kubernautagent/session/event_sink_test.go` |
| UT-KA-1811-002 | Unit | A full `Manager.StartInvestigation` → synchronous `InteractiveHold` completion → `UpgradeToInteractive` → `Subscribe` sequence (the exact #1811 ordering) delivers the RCA-phase events and the terminal `EventTypeComplete` (with non-empty RCA data) to the late subscriber, where today's code delivers zero | SI-4, AU-3 | BR-INTERACTIVE-004 | `internal/kubernautagent/session/late_subscribe_replay_1811_test.go` |
| UT-KA-1811-003 | Unit | The already-working case (sink active before emission) is unaffected: no double-delivery, no reordering | SI-4 | BR-INTERACTIVE-004 | `internal/kubernautagent/session/late_subscribe_replay_1811_test.go` |
| IT-KA-1811-001 | Integration | The real `kubernaut_investigate` MCP tool path (`InvestigateTool.Handle` action=start → `upgradeOrCreateInteractiveSession` → `UpgradeToInteractive`, then `InvestigateTool.SubscribeEvents` — the exact `registration.go` `wireInvestigationEventBridge` call chain) exercised against a real `Manager`, proving the production wiring point (not just direct `Manager` calls) delivers buffered events after a late `Subscribe` | SI-4 | BR-INTERACTIVE-004 | `internal/kubernautagent/mcp/tools/late_subscribe_replay_1811_it_test.go` |
| E2E-FP-1189-005 | E2E | Existing test (`test/e2e/fullpipeline/15_af_a2a_interactive_streaming_test.go`) — attempted to harden the `TODO(#1795)`-softened RCA-content assertion; CI surfaced a **distinct** race (#1818, see Section 6) that this fix does not address, so the assertion remains softened pending #1818 | SI-4 | BR-INTERACTIVE-004 | `test/e2e/fullpipeline/15_af_a2a_interactive_streaming_test.go` |

## 5. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| `LazySink.Emit`/buffered replay | Investigation goroutine emission path | `internal/kubernautagent/investigator/investigator_loop.go` (`emitToSink` dispatcher), `internal/kubernautagent/session/manager_events.go` (`emitCompleteEvent`, `emitTerminalEvent`) | UT-KA-1811-001/002/003 |
| `session.EmitEvent` (new) | Same as above | `internal/kubernautagent/session/event_sink.go` | UT-KA-1811-001/002 |
| Late-subscribe delivery through `kubernaut_investigate` | `internal/kubernautagent/mcp/tools/investigate_start.go` (`handleStart`/`upgradeOrCreateInteractiveSession`), `registration.go` (`wireInvestigationEventBridge` → `SubscribeEvents`) | Pre-existing; newly exercised by IT-KA-1811-001 | IT-KA-1811-001 |

No new production components — this closes a gap in existing, already-wired machinery.

## 6. Out of Scope / Tracked Separately

- The "genuinely fully-completed-before-any-Subscribe" case (`ErrSessionTerminal` path) —
  unaffected by this fix, already has its own fallback (`ForceTransitionToUserDriving`).
- The `discovery_required` symptom mentioned in #1811 as "possibly related" — treated as a
  separate, unconfirmed issue per the original report; not addressed here unless the E2E
  hardening in Section 4 surfaces it as still-failing, in which case it will be filed
  separately rather than silently folded into this fix.
- `release/v1.5` applicability — assessed separately after `main` fix lands (same
  `LazySink`/`InteractiveHold` architecture is present on `v1.5`, confirmed during #1811
  triage; backport tracked as a follow-up once the `main` fix is verified in CI).
- **[#1818](https://github.com/jordigilh/kubernaut/issues/1818)** (filed during this fix's CI
  verification): attempting to harden `E2E-FP-1189-005`'s RCA-content assertion surfaced a
  *third*, architecturally distinct race, confirmed via CI must-gather KA logs — AA's
  `RequestBuilder.BuildIncidentRequest` never sets `IncidentRequest.Interactive`, so KA always
  takes the autonomous/immediate path; when that autonomous investigation is fast enough to
  reach `StatusCompleted` before AF's `kubernaut_investigate` arrives, KA can no longer
  reattach to it (`LaunchDeferredInvestigation` rejects non-`Pending`, `FindByRemediationID`
  only matches `StatusRunning`), so `createFallbackSession` creates a fresh, RCA-less session
  and the real RCA is orphaned. This fix's `LazySink` buffering is verified working correctly
  in the same CI logs (`sink_nil=6, dropped=0` for the original session) — #1818 is about the
  original session never being found again, not about its buffered events being lost. Left
  the E2E assertion in its pre-existing softened form (now referencing #1818 instead of
  #1795) rather than folding an unrelated architectural fix into this PR.

## 7. Final Results

All scenarios pass locally against `origin/main` (`go build ./...`, `go vet ./...`,
`golangci-lint run` all clean):

| ID | Result |
|---|---|
| UT-KA-1811-001 | PASS |
| UT-KA-1811-002 | PASS |
| UT-KA-1811-003 | PASS |
| IT-KA-1811-001 | PASS |
| UT-KA-1438-001/002 (pre-existing, updated) | PASS — updated to drain the now-correctly-replayed `EventTypeComplete` event ahead of the `session_ended` assertion; the fix means this event is no longer silently lost, which these tests' setups happen to exercise as a side effect |
| Full `internal/kubernautagent/...` unit suite | PASS (0 regressions) |
| `test/e2e/fullpipeline` (`go vet`) | PASS (compiles; full E2E run is CI-gated, not run locally) |

Three call sites now route through the buffering `Emit`/`EmitEvent` path:
`investigator_loop.go`'s `emitToSink`, and `manager_events.go`'s `emitCompleteEvent` and
`emitTerminalEvent`.
