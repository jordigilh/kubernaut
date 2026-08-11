# Test Plan: #2103 — `aiagent_mcp_interactive_sessions_active` Gauge Drift

## 1. Purpose

`aiagent_mcp_interactive_sessions_active` (the Prometheus gauge tracking
concurrent MCP interactive sessions) only ever decrements for two of the
ways an interactive session can end — explicit `complete`/`cancel` — plus
two callback paths owned directly by `cmd/kubernautagent/main.go`
(disconnect-grace-period expiry, inactivity-timeout expiry via
`TimeoutManager`). Every other completion path releases the real session
(Lease + `LeaseSessionManager.activeCount`, both healthy) but never calls
the matching `RecordInteractiveSessionEnded()`, so the gauge drifts upward
and never reflects true concurrency. This is **not** a recurrence of
#2100's capacity leak — the real Lease/activeCount bookkeeping is
confirmed healthy; this is an observability-only bug.

Discovered live-monitoring QE's #2100 validation run (PR #2102, images
deployed with `SessionJanitor` wired).

### Root Cause Detail

`RecordInteractiveSessionEnded()` is not called from inside
`LeaseSessionManager.Release()` itself — it is the *caller's*
responsibility to call both `sessions.Release(...)` and the metrics
decrement separately, and most callers of `Release()` don't have
`agentMetrics` wired at all. Confirmed gaps (all verified directly against
current code, not just the issue's live evidence):

1. `CompleteNoActionTool.Handle` (`complete_no_action.go:194`, reason
   `"complete_no_action"`) — struct has no `metrics` field.
2. `SelectWorkflowTool`'s post-selection release goroutine
   (`select_workflow.go:372`, reason `"workflow_selected"`) — struct has
   no `metrics` field.
3. `InvestigateTool`'s `no_matching_workflows` auto-close goroutine
   (`investigate.go:1101-1138`, release at line 1133) — `InvestigateTool`
   *does* have `t.metrics` (used elsewhere at lines 583-584, 850, 1271),
   but this goroutine's closure doesn't capture or call it.
4. `SessionJanitor`'s `onExpire` callback, added by #2100's own fix
   (`session_manager.go:290-294`) — lives inside `LeaseSessionManager`, a
   package with zero awareness of `agentMetrics`. #2100's new backstop
   sweep path inherits this same gap.

Correctly-decrementing paths (for contrast): `InvestigateTool.handleComplete`
(investigate.go:850, `"complete"`), `handleCancel` (investigate.go:1271,
`"explicit"`), `TimeoutManager`'s inactivity-expiry callback
(`main.go:1356`, `"inactivity_timeout"`), `GracefulSessionClosedHandler`'s
disconnect callback (`main.go:1415`, `"disconnect"`), and
`WithSessionExpiredCallback` (`main.go:1312`, TTL/inactivity via
`GetDriver`'s lazy check).

## 2. Fix Design

**Rejected alternative**: patch each of the 4 (and growing) gap call sites
individually with their own `metrics ToolMetrics` field + option, mirroring
`InvestigateTool`'s existing pattern. Rejected because it reintroduces the
exact same gap for the *next* new completion path (this issue is itself
evidence that per-call-site wiring doesn't scale — #2100's new janitor path
inherited the gap on day one).

**Chosen fix**: centralize the decrement *inside*
`LeaseSessionManager.Release()` itself, paired with the existing
`activeCount.Add(-1)`, via a new optional callback wired the same way
`WithSessionExpiredCallback`/`WithSessionJanitor` already are:

```go
// SessionEndedMetrics is the minimal metrics surface LeaseSessionManager
// needs to keep aiagent_mcp_interactive_sessions_active accurate for every
// Release caller, current and future (#2103). Implemented structurally by
// *metrics.Metrics (see tools.ToolMetrics for the parallel interface at
// the tool-handler layer) -- defined locally here, rather than imported,
// to avoid mcp importing metrics or tools (tools already imports mcp).
type SessionEndedMetrics interface {
    RecordInteractiveSessionEnded()
}
```

`Release()` calls `m.sessionMetrics.RecordInteractiveSessionEnded()`
immediately after `m.activeCount.Add(-1)`, guarded by nil-check, and
**only** on the path that actually reaches that line (i.e. not on
`ErrSessionNotFound`, not on a hard Lease-delete error) — so a
double-`Release()` call never double-decrements.

This single change automatically closes gaps #1–#4 above, since all four
routes call `sessions.Release(...)` against the *same* shared
`LeaseSessionManager` instance in `buildMCPHandler`. No per-tool `metrics`
field is added to `CompleteNoActionTool` or `SelectWorkflowTool`, and
`InvestigateTool`'s `no_matching_workflows` goroutine needs no capture
change.

**Redundant-call cleanup** (required to avoid double-decrementing once
`Release()` handles it centrally):

- `investigate.go` `handleComplete` (was line 849-851) and `handleCancel`
  (was line 1270-1272): remove the explicit
  `t.metrics.RecordInteractiveSessionEnded()` call — both already call
  `t.sessions.Release(...)` just above.
- `main.go`'s `WithSessionExpiredCallback` (was line 1312), `timeoutMgr`'s
  inactivity callback (was line 1356), `disconnectHandler`'s callback (was
  line 1415): remove the explicit `agentMetrics.RecordInteractiveSessionEnded()`
  calls — all three already call `leaseMgr.Release(...)` (directly, or via
  `GetDriver`'s lazy TTL/inactivity check) just above.

**Started/Ended symmetry fix** (subtlety not called out in the original
issue): `handleStart`'s fallback-exhausted branch (`#2100`'s own fix,
`investigate.go:570`) calls `Release(sess.SessionID, "no_investigation_available")`
on a lease for which `RecordInteractiveSessionStarted()` was **never**
called (that call happens later, at the end of `handleStart`, only on the
success path). Once `Release()` centrally decrements, this branch would
decrement a gauge that was never incremented for that session, pushing it
negative. Fix: move `t.metrics.RecordInteractiveSessionStarted()` to fire
immediately after `Takeover()` confirms a genuinely new lease (i.e. after
the `sess.Reconnected` check, which correctly still returns early without
touching the gauge — no new lease was acquired for a reconnect), before any
of the fallback logic that might immediately release it. This keeps
Started/Ended paired with the lease's own lifecycle (`activeCount`'s own
`Add(1)`/`Add(-1)` pair), which is more accurate than the previous
behavior (a lease that lived for a few hundred ms before failing closed was
never counted as "started" at all).

## 3. FedRAMP Control Mapping

| Control | Title | Relevance |
|---|---|---|
| **SI-4** | System Monitoring | The core control: `aiagent_mcp_interactive_sessions_active` is an operational-monitoring signal (BR-KA-OBSERVABILITY-001); a monotonically-drifting gauge produces false-positive capacity alerts and masks the true concurrency the #2100 capacity guarantee depends on. |
| **SI-11** | Error Handling | The Started/Ended symmetry fix ensures a fail-closed lease release (#2100's `no_investigation_available` path) is still correctly accounted for in the gauge instead of silently under/over-counting. |
| **SC-5** | Denial of Service Protection | An operator/dashboard relying on this gauge to catch a *future* recurrence of #2100's capacity-exhaustion bug would be blind to it while the gauge is stuck climbing regardless of true load. |

## 4. Pyramid Invariant — Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | Control | Test File |
|---|---|---|---|---|
| UT-KA-2103-001 | Unit | `LeaseSessionManager.Release()` calls the wired `SessionEndedMetrics.RecordInteractiveSessionEnded()` exactly once on a successful release | SI-4 | `internal/kubernautagent/mcp/session_manager_metrics_2103_test.go` |
| UT-KA-2103-002 (regression guard) | Unit | `Release()` on an unknown/already-released session ID returns `ErrSessionNotFound` and does **not** call the metrics callback (no phantom decrement) | SI-4 | `internal/kubernautagent/mcp/session_manager_metrics_2103_test.go` |
| UT-KA-2103-003 (regression guard) | Unit | `Release()` with no `WithSessionEndedMetrics` wired (nil) does not panic — safe no-op, matching every pre-#2103 call site | SI-11 | `internal/kubernautagent/mcp/session_manager_metrics_2103_test.go` |
| UT-KA-2103-004 (regression guard) | Unit | `InvestigateTool.handleComplete` (`action=complete`) no longer calls `metrics.RecordInteractiveSessionEnded()` directly — proves the responsibility fully moved to `LeaseSessionManager.Release()`, preventing double-decrement once combined with the real manager | SI-4 | `internal/kubernautagent/mcp/tools/investigate_metrics_2103_test.go` |
| UT-KA-2103-005 (regression guard) | Unit | `InvestigateTool.handleCancel` (`action=cancel`) — same as UT-KA-2103-004 | SI-4 | `internal/kubernautagent/mcp/tools/investigate_metrics_2103_test.go` |
| UT-KA-2103-006 | Unit | `handleStart` records `RecordInteractiveSessionStarted()` immediately after a successful (non-reconnect) `Takeover`, even on the fallback-exhausted branch that immediately releases the lease — proves Started/Ended stay paired now that Ended is centralized (prevents the gauge going negative) | SI-4, SI-11 | `internal/kubernautagent/mcp/tools/investigate_metrics_2103_test.go` |
| UT-KA-2103-007 (regression guard) | Unit | `handleStart`'s `Reconnected` branch and `Takeover`-failure branches still do **not** call `RecordInteractiveSessionStarted()` — no new lease was actually acquired | SI-4 | `internal/kubernautagent/mcp/tools/investigate_metrics_2103_test.go` |
| IT-KA-2103-001 | Integration | An explicit `Release()` call (reasons `complete_no_action`, `workflow_selected`, `no_matching_workflows` in sequence) against a real envtest `LeaseSessionManager` wired with `WithSessionEndedMetrics` decrements the metrics recorder exactly once per release — proves gaps #1–#3 all collapse to the same fixed chokepoint, using the exact `NewLeaseSessionManagerConcrete(..., WithSessionEndedMetrics(...))` construction `buildMCPHandler` uses in production | SI-4 | `test/integration/kubernautagent/mcp/session_manager_metrics_2103_it_test.go` |
| IT-KA-2103-002 | Integration | `SessionJanitor`'s sweep-triggered `onExpire` → `Release(..., reason)` decrements the same wired metrics recorder against a real envtest API server (real `coordination.k8s.io/Lease` reclaimed) — closes gap #4 end-to-end, extending #2100's own `session_janitor_wiring_2100_test.go` IT pattern | SI-4, SC-5 | `test/integration/kubernautagent/mcp/session_manager_metrics_2103_it_test.go` |

### Tier Coverage Rationale

- **UT** covers the centralized decrement's logic in isolation (found/not-found/nil-metrics against a fake K8s client, mirroring #2100's own `session_manager_janitor_2100_test.go` precedent — no real API server needed since this fix touches zero K8s-API-specific behavior), plus the two `investigate.go` regression guards proving the old direct-call pattern is fully retired, plus the Started/Ended symmetry fix.
- **IT** proves the fix against a real envtest API server using the identical `NewLeaseSessionManagerConcrete(..., WithSessionEndedMetrics(...))` construction path `buildMCPHandler` uses in production, exercising both call shapes that existed pre-fix: an explicit `Release()` call (the shape shared verbatim by `CompleteNoActionTool.Handle`, `SelectWorkflowTool`'s async goroutine, and `InvestigateTool`'s `no_matching_workflows` auto-close — CHECKPOINT W's grep evidence confirms all three call the same `sessions.Release(sessionID, reason)` signature on the one shared `SessionManager` instance `buildMCPHandler` constructs), and the `SessionJanitor`'s real sweep-triggered `onExpire` callback (gap #4), mirroring #2100's own precedent of proving `LeaseSessionManager`-internal wiring directly against envtest rather than requiring a full MCP-protocol round trip per caller. A UT of `Release()` alone (already covered above) cannot prove the `WithSessionEndedMetrics` option is actually threaded through `buildMCPHandler`'s `leaseOpts` to the one shared instance in production — CHECKPOINT W's grep evidence plus this IT together close that gap.
- **E2E**: not added net-new. This is an observability-only fix (no functional/capacity behavior change); QE's next acceptance run against this branch's images is the confirmation path, not a new E2E suite in this repo.

## 5. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| `SessionEndedMetrics` interface + `sessionMetrics` field | `LeaseSessionManager.Release()` | `internal/kubernautagent/mcp/session_manager.go` | UT-KA-2103-001..003 |
| `WithSessionEndedMetrics` option | `buildMCPHandler`'s `leaseOpts` (`cmd/kubernautagent/main.go`), passed `agentMetrics` | `cmd/kubernautagent/main.go`, `internal/kubernautagent/mcp/session_manager.go` | IT-KA-2103-001, IT-KA-2103-002 |
| `handleStart`'s relocated `RecordInteractiveSessionStarted()` call | `InvestigateTool.Handle` (`action=start`), dispatched from the `kubernaut_investigate` MCP tool handler registered in `buildMCPHandler` | `internal/kubernautagent/mcp/tools/investigate.go` | UT-KA-2103-006, UT-KA-2103-007 |

## 6. CHECKPOINT W Evidence

```
=== WithSessionEndedMetrics production callers ===
cmd/kubernautagent/main.go:1324:		mcpkg.WithSessionEndedMetrics(agentMetrics),
internal/kubernautagent/mcp/session_manager.go:177:func WithSessionEndedMetrics(m SessionEndedMetrics) LeaseOption {

=== sessionMetrics.RecordInteractiveSessionEnded call site (the ONLY production call site) ===
internal/kubernautagent/mcp/session_manager.go:374:	if m.sessionMetrics != nil {
internal/kubernautagent/mcp/session_manager.go:375:		m.sessionMetrics.RecordInteractiveSessionEnded()

=== Confirm no orphaned direct RecordInteractiveSessionEnded calls remain in cmd/ or internal/ ===
(none outside session_manager.go:375 and metrics.go's own implementation — confirmed via
 grep -rn "RecordInteractiveSessionEnded" cmd/ internal/ --include="*.go" | grep -v _test.go)

=== All 4 original gap call sites now route through the one centralized Release() ===
internal/kubernautagent/mcp/tools/select_workflow.go:372:   sessions.Release(sessionID, "workflow_selected")
internal/kubernautagent/mcp/tools/investigate.go:582:      t.sessions.Release(sess.SessionID, "no_investigation_available")  (#2100 fail-closed path, unrelated reason string)
internal/kubernautagent/mcp/tools/investigate.go:1147:     sessions.Release(sessionID, "no_matching_workflows")
internal/kubernautagent/mcp/tools/complete_no_action.go:194: t.sessions.Release(driver.SessionID, "complete_no_action")
```

CHECKPOINT W: PASSED — `WithSessionEndedMetrics` has exactly one production caller
(`cmd/kubernautagent/main.go`, wired into the same `leaseOpts` slice that constructs the
one shared `LeaseSessionManager` instance `buildMCPHandler` hands to every tool), the
decrement itself has exactly one call site (`Release()` line 375), and all four originally-
reported gap paths (`complete_no_action`, `workflow_selected`, `no_matching_workflows`,
`SessionJanitor.onExpire`) call `Release()` on that same shared instance — no orphaned
`pkg`/`internal` code, no "TODO: wire later" deferred wiring.

## 7. Build Validation

Executed 2026-08-11:

```bash
$ go build ./...                                                          # PASS
$ go vet ./...                                                            # PASS (no output)
$ golangci-lint run --timeout=5m ./internal/kubernautagent/... \
    ./cmd/kubernautagent/...                                              # 0 issues
$ make test-unit-kubernautagent
  # Ran 97 of 97 Specs — SUCCESS! 97 Passed | 0 Failed | 0 Pending | 0 Skipped
  # (across all 29 KA unit suites; composite coverage 84.8%)
$ make test-integration-kubernautagent-interactive
  # Ran 27 of 103 Specs (label: interactive) — SUCCESS! 27 Passed | 0 Failed | 0 Pending
  # includes both new IT-KA-2103-001 and IT-KA-2103-002
```

## 8. Coverage Summary

- Unit: 97/97 specs passed across 29 KA unit suites; composite (merged) coverage 84.8%
  of unit-testable statements in `internal/kubernautagent/...` (per
  `make test-unit-kubernautagent`'s coverage report).
- Integration: 27/27 specs passed for the `interactive` label subset (full
  `test/integration/kubernautagent/mcp` package, envtest + Podman DataStorage + Mock LLM),
  including the two new #2103 IT tests exercising the real `Release()` chokepoint and the
  real `SessionJanitor` sweep against a live envtest API server.
- No regressions introduced in any pre-existing #2100/#2098/#2094/#2092/#2105 test in the
  same packages (all still passing in the same runs above).

## 9. Out of Scope

- Restructuring the pre-existing, seemingly-overlapping dual inactivity
  mechanisms (`LeaseSessionManager.inactivityTimeout`'s lazy `GetDriver`
  check vs. `TimeoutManager`'s proactive timer) — both already correctly
  route through `Release()` and are unaffected by this centralization;
  consolidating them is a separate, unrelated architectural question not
  raised by this issue.
- The `#1654`-style "transition to user-driving fails, not terminal"
  branch in `handleTakeover` (investigate.go:656-659) that returns an
  error without releasing the just-acquired lease — noticed while tracing
  this issue's call graph, but out of scope: it is a potential *capacity*
  leak (#2100's concern), not a *metrics* leak (#2103's concern), since
  `RecordInteractiveSessionStarted()` was never called for it either. Flag
  for separate triage if confirmed live.

## 10. Sign-off

| Role | Name | Date | Signature |
|---|---|---|---|
| Author | AI Assistant | 2026-08-11 | pending |
| Reviewer | Jordi Gil | | pending |
| Approver | | | pending |
