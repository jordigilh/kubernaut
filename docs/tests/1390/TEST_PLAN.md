# IEEE 829 Test Plan — Fix #1390: Jump-In Session Upgrade + Defense-in-Depth

| Field                | Value                                                                              |
|----------------------|------------------------------------------------------------------------------------|
| **Test Plan ID**     | TP-1390                                                                            |
| **Revision**         | 1.0                                                                                |
| **Author**           | Kubernaut AI Agent                                                                 |
| **Date**             | 2026-06-09                                                                         |
| **Status**           | Active                                                                             |
| **Business Req**     | BR-INTERACTIVE-004, BR-INTERACTIVE-005, BR-INTERACTIVE-010, BR-SESSION-002, BR-REL-014, BR-ORCH-027, BR-MCP-002, BR-AUDIT-005 |
| **FedRAMP Controls** | SC-24, SI-10, SI-13, AC-12, AU-12, SC-8, SI-4                                     |

## 1. Introduction

This test plan covers the verification of fixes for GitHub issue #1390, which
describes two production failure modes in the AA-KA session lifecycle:

- **Bug A (20-minute 409 loop)**: AA polls `GET /result` on a completed session
  with `nil` result; KA returns 409 Conflict; AA treats it as transient and
  retries indefinitely (violates BR-ORCH-027, BR-SESSION-002).

- **Bug B (Wasteful cancel+recreate)**: When an InvestigationSession (IS) CRD
  appears after an autonomous submit, AA cancels the running KA session and
  re-submits with `interactive=true`, discarding in-flight RCA work and wasting
  LLM tokens (violates BR-INTERACTIVE-004 efficiency).

Fix approach: "Jump In" upgrades autonomous sessions to interactive in-place via
an `atomic.Bool` flag on the Session struct, plus defense-in-depth nil-result
handling and AA retry cap.

## 2. Scope

### In Scope

| Area                              | Description                                                     |
|-----------------------------------|-----------------------------------------------------------------|
| KA session manager                | `UpgradeToInteractive`, `interactiveUpgrade` atomic flag        |
| KA store deterministic upgrade    | `store.Update` checks flag under mu for race-free guarantee     |
| KA context helpers                | `WithInteractiveUpgrade`, `InteractiveUpgradeFromContext`       |
| KA investigator                   | `InteractiveHold` decision respects runtime upgrade flag        |
| KA MCP tools                      | `handleStart` wiring + EventLogBridge activation for upgrade    |
| AA InvestigatingHandler           | Takeover simplification (no cancel), `SetActivePhase` call      |
| KA HTTP handler                   | Structured result for nil-result completed/failed/cancelled     |
| AA retry cap                      | `ConsecutiveGetResultErrors` counter, caps at 3                 |
| Session ID context propagation    | Autonomous path carries session_id for audit trail              |
| E2E upgrade journey               | Full pipeline: AA -> IS -> MCP -> upgrade -> result             |
| E2E nil-result resilience         | Timeout -> nil result -> structured 200 -> AA terminal          |

### Out of Scope

- AF session service (upstream consumer, no changes)
- MCP transport layer (WebSocket/SSE framing)
- LLM provider integrations
- BR-INTERACTIVE-010 SC-1 wording update (docs-only, separate PR)

## 3. Business Requirements Traceability

| BR                    | Description                                                         | Violated By |
|-----------------------|---------------------------------------------------------------------|-------------|
| BR-SESSION-002        | Session store accurately reflects terminal state + result on query  | Bug A       |
| BR-ORCH-027           | All remediations MUST reach terminal state                          | Bug A       |
| BR-INTERACTIVE-004    | Dynamic takeover preserves in-flight work                           | Bug B       |
| BR-INTERACTIVE-005    | Session lifecycle (timeout, inactivity, disconnect)                 | Bug A       |
| BR-INTERACTIVE-010    | IS-driven interactive flow, AA poll handling                        | Bug B       |
| BR-REL-014            | Idempotent operations for retry scenarios                           | Bug A       |
| BR-MCP-002            | `start_autonomous` idempotency when investigation running           | Bug B       |
| BR-AUDIT-005          | Enterprise audit integrity; session lifecycle auditable             | Bug A, B    |

## 4. FedRAMP / NIST 800-53 Rev 5 Control Verification

Tests verify **business-level behavior** tied to each control objective.

### SC-24 -- Fail in Known State

**Business behavior**: A completed session MUST always be pollable with a
deterministic result. A nil-result completed session must return a structured
"completed without result" response, not an error loop. The store-level upgrade
guarantee ensures sessions deterministically reach `user_driving` when upgraded.

**Tests**: UT-KA-1390-007, UT-KA-1390-018..021, UT-KA-1390-027..029, IT-KA-1390-W01, IT-KA-1390-W02, IT-KA-1390-W05, E2E-KA-1390-001

### SI-10 -- Information Input Validation / Deduplication

**Business behavior**: `UpgradeToInteractive` MUST validate session state before
modifying. Terminal and non-existent sessions MUST be rejected with appropriate
errors.

**Tests**: UT-KA-1390-003, UT-KA-1390-004

### SI-13 -- Predictable Failure Prevention

**Business behavior**: AA MUST NOT retry indefinitely on data-integrity errors.
After N consecutive 409s on GetResult, AA triggers session regeneration to break
the loop.

**Tests**: UT-AA-1390-022..024, E2E-KA-1390-001

### AC-12 -- Session Termination

**Business behavior**: Every investigation MUST reach a terminal AA phase. AA
takeover MUST NOT leave sessions in limbo. The upgrade path MUST preserve session
reachability through all state transitions.

**Tests**: UT-AA-1390-014..017, IT-AA-1390-W04, E2E-FP-1390-001

### AU-12 -- Audit Record Generation

**Business behavior**: All session lifecycle events (upgrade, nil-result
synthesis, completion) MUST be auditable. `session_id` MUST appear in all
LLM/audit logs for forensic traceability.

**Tests**: UT-KA-1390-002, UT-KA-1390-021, UT-KA-1390-025..026

### SC-8 -- Transmission / Session Integrity

**Business behavior**: Session identity (`session_id`) MUST be propagated through
the investigation context. Ghost `session_id:""` events violate audit chain
integrity.

**Tests**: UT-KA-1390-025, UT-KA-1390-026

### SI-4 -- System Monitoring / Event Streaming

**Business behavior**: After upgrade, the EventLogBridge MUST stream investigation
events to the MCP client. LazySink activation MUST be picked up by the running
goroutine on its next event emission.

**Tests**: UT-KA-1390-008, UT-KA-1390-009, UT-KA-1390-013, IT-KA-1390-W03

## 5. Test Tiers and Coverage Targets

| Tier              | Testable Code                                                       | Target |
|-------------------|---------------------------------------------------------------------|--------|
| Unit Tests        | Session manager, store, investigator, HTTP handler, AA handlers     | >=80%  |
| Integration Tests | HTTP dispatch, session wiring, controller reconcile, EventLogBridge | >=80%  |
| E2E Tests         | Full pipeline upgrade journey, nil-result resilience journey        | >=80%  |

## 6. Wiring Manifest (Pyramid Invariant)

> UT proves logic. IT proves wiring. E2E proves the journey.

| Component                      | Production Entry Point                      | Wiring Location              | UT (logic)               | IT/E2E (wiring)       |
|--------------------------------|---------------------------------------------|------------------------------|--------------------------|----------------------|
| `UpgradeToInteractive`         | `handleStart` in `investigate.go:~424`      | `manager.go`                 | UT-KA-1390-001..004      | IT-KA-1390-W01       |
| `interactiveUpgrade` flag      | `launchInvestigation` context               | `manager.go:~161`            | UT-KA-1390-005..009      | IT-KA-1390-W01       |
| Store deterministic upgrade    | `store.Update` flag check under mu          | `store.go:~241`              | UT-KA-1390-027..029      | IT-KA-1390-W01       |
| Investigator upgrade check     | `Investigate()` line 519                    | `investigator.go`            | UT-KA-1390-010..011      | IT-KA-1390-W01       |
| `handleStart` wiring + bridge  | `InvestigateRegistration`                   | `investigate.go + reg.go`    | UT-KA-1390-012..013      | IT-KA-1390-W03       |
| AA takeover simplification     | `checkISMismatchAndCancel`                  | `investigating.go:~407`      | UT-AA-1390-014..017      | IT-AA-1390-W04       |
| Nil-result structured response | `GetResult` handler                         | `handler.go:~249`            | UT-KA-1390-018..021      | IT-KA-1390-W02       |
| AA 409 retry cap               | `handleSessionGetResultError`               | `investigating.go:~921`      | UT-AA-1390-022..024      | E2E-KA-1390-001      |
| session_id propagation         | `launchInvestigation` context               | `manager.go`                 | UT-KA-1390-025..026      | E2E-FP-1390-001      |
| Upgrade journey (cross-svc)    | AA + KA + MCP + IS CRD                      | Full pipeline                | --                       | E2E-FP-1390-001      |
| Nil-result resilience journey  | KA timeout + AA poll                        | KA + AA                      | --                       | E2E-KA-1390-001      |

## 7. Unit Test Scenarios

### Layer 1: KA Session Upgrade Mechanism

| ID              | FedRAMP | Business Behavior Verified                                                                  | Acceptance Criteria                                                              |
|-----------------|---------|---------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------|
| UT-KA-1390-001  | SC-24   | `UpgradeToInteractive` sets atomic flag on running session without cancelling goroutine      | `interactiveUpgrade.Load()==true`, cancel not called, `Status==StatusRunning`     |
| UT-KA-1390-002  | AU-12   | `UpgradeToInteractive` writes acting_user and acting_user_groups to session metadata         | `Metadata["acting_user"]=="testuser"`, groups JSON matches                       |
| UT-KA-1390-003  | SI-10   | `UpgradeToInteractive` on terminal session returns `ErrSessionTerminal`                      | Error is `ErrSessionTerminal`, no state change                                   |
| UT-KA-1390-004  | SI-10   | `UpgradeToInteractive` on non-existent session returns `ErrSessionNotFound`                  | Error is `ErrSessionNotFound`                                                    |
| UT-KA-1390-005  | SC-24   | `InteractiveUpgradeFromContext` returns false when no flag in context                        | Returns false, no panic                                                          |
| UT-KA-1390-006  | SC-24   | `InteractiveUpgradeFromContext` reads true after `Store(true)` on atomic                     | Returns true after concurrent Set                                                |
| UT-KA-1390-007  | SC-24   | Goroutine with upgrade flag stores result via `Update(running->user_driving)`                | `Status==StatusUserDriving`, `Result!=nil`, `Result.InteractiveHold==true`        |
| UT-KA-1390-008  | SI-4    | Event channel stays open after upgrade + InteractiveHold completion                          | `eventChan` not nil, not closed                                                  |
| UT-KA-1390-009  | SI-4    | LazySink activation post-upgrade delivers events to subscriber                               | Event arrives on channel within 100ms                                            |

### Layer 1: Store-Level Deterministic Upgrade

| ID              | FedRAMP | Business Behavior Verified                                                                  | Acceptance Criteria                                                              |
|-----------------|---------|---------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------|
| UT-KA-1390-027  | SC-24   | `store.Update(StatusCompleted)` + `interactiveUpgrade=true` forces `StatusUserDriving`       | `Status==StatusUserDriving`, `InteractiveHold==true`, result preserved            |
| UT-KA-1390-028  | SC-24   | `store.Update(StatusCompleted)` + `interactiveUpgrade=false` remains `StatusCompleted`       | `Status==StatusCompleted`, `InteractiveHold` unchanged                           |
| UT-KA-1390-029  | SC-24   | `UpgradeToInteractive` after `Update(completed)` returns `ErrSessionTerminal`                | Error is `ErrSessionTerminal`, fallback to `ForceTransitionToUserDriving`         |

### Layer 1c: Investigator

| ID              | FedRAMP | Business Behavior Verified                                                                  | Acceptance Criteria                                                              |
|-----------------|---------|---------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------|
| UT-KA-1390-010  | SC-24   | Investigator sets `InteractiveHold=true` when upgrade flag is true + `signal.Interactive=false` | Result has `InteractiveHold==true`, Phase 2/3 skipped                           |
| UT-KA-1390-011  | SC-24   | Investigator still respects `signal.Interactive=true` (no regression)                         | Same behavior as before for originally-interactive sessions                      |

### Layer 1d: MCP Wiring

| ID              | FedRAMP | Business Behavior Verified                                                                  | Acceptance Criteria                                                              |
|-----------------|---------|---------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------|
| UT-KA-1390-012  | SC-24   | `handleStart` calls `UpgradeToInteractive` (not `TransitionToUserDriving`) for running       | Mock `UpgradeToInteractive` called once, `TransitionToUserDriving` not called    |
| UT-KA-1390-013  | SI-4    | `handleStart` sets `InvestigationSessionID` for running sessions (enables EventLogBridge)    | `output.InvestigationSessionID != ""`                                            |

### Layer 2: AA Takeover Simplification

| ID              | FedRAMP | Business Behavior Verified                                                                  | Acceptance Criteria                                                              |
|-----------------|---------|---------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------|
| UT-AA-1390-014  | AC-12   | Takeover branch sets `Interactive=true` without calling `CancelSession`                      | `cancelCount==0`, `session.Interactive==true`                                    |
| UT-AA-1390-015  | AC-12   | Takeover branch calls `SetActivePhase` on IS phase updater                                   | `SetActivePhaseCallCount==1`                                                     |
| UT-AA-1390-016  | AC-12   | Next reconcile with `hasIS=true` + `Interactive=true` skips mismatch (no-op)                 | No cancel, no resubmit, returns `RequeueAfter`                                   |
| UT-AA-1390-017  | AC-12   | Poll `user_driving` after upgrade populates `InteractiveSession` info                        | `InteractiveSession.ActingUser != ""`                                            |

### Layer 3: Nil-Result 409 Loop Fix

| ID              | FedRAMP | Business Behavior Verified                                                                  | Acceptance Criteria                                                              |
|-----------------|---------|---------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------|
| UT-KA-1390-018  | SC-24   | `GetResult` on completed/nil-result returns HTTP 200 + synthetic result                      | Status 200, `summary` present, `isActionable==false`                             |
| UT-KA-1390-019  | SC-24   | `GetResult` on failed session returns HTTP 200 + error result                                | Status 200, `summary` with failure reason                                        |
| UT-KA-1390-020  | SC-24   | `GetResult` on cancelled session returns HTTP 200 + cancellation result                      | Status 200, `cancelled==true`                                                    |
| UT-KA-1390-021  | AU-12   | `GetResult` nil-result synthesis emits log event                                             | Logger captures `"nil_result_synthesized"` with session_id                       |
| UT-AA-1390-022  | SI-13   | Third consecutive GetResult 409 triggers session regeneration                                | After 3x 409, `session.ID` cleared, `result.Requeue`                            |
| UT-AA-1390-023  | SI-13   | Successful GetResult resets consecutive error counter                                        | After success, counter is 0                                                      |
| UT-AA-1390-024  | AC-12   | Regeneration clears session ID and requeues for re-submit                                    | `session.ID==""`, `result.Requeue==true`                                         |

### Layer 4: Observability

| ID              | FedRAMP   | Business Behavior Verified                                                                | Acceptance Criteria                                                              |
|-----------------|-----------|-------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------|
| UT-KA-1390-025  | SC-8,AU-12| Autonomous session `bgCtx` carries `session_id` via `WithSessionID`                       | `SessionIDFromContext(bgCtx) != ""`                                              |
| UT-KA-1390-026  | AU-12     | `UpgradeToInteractive` emits audit event with session_id and acting_user                  | Audit spy captures `EventTypeSessionUpgraded` with correct fields                |

## 8. Integration Test Scenarios

| ID              | FedRAMP | Business Behavior Verified                                          | Acceptance Criteria                                                                     | Location                                       |
|-----------------|---------|---------------------------------------------------------------------|-----------------------------------------------------------------------------------------|-------------------------------------------------|
| IT-KA-1390-W01  | SC-24   | Full upgrade flow: autonomous -> upgrade -> InteractiveHold -> result | HTTP poll returns `user_driving`, `result!=nil`, `acting_user` populated                | `test/integration/kubernautagent/session/`      |
| IT-KA-1390-W02  | SC-24   | Nil-result completed session returns structured result via HTTP      | `GET /result` returns 200 with synthetic body                                           | `test/integration/kubernautagent/`              |
| IT-KA-1390-W03  | SI-4    | EventLogBridge wired after upgrade; events stream to subscriber      | Events emitted by goroutine arrive on Subscribe channel                                 | `test/integration/kubernautagent/session/`      |
| IT-AA-1390-W04  | AC-12   | AA takeover without cancel -> polls user_driving -> InteractiveSession | No KA cancel, `Interactive==true`, `InteractiveSession.ActingUser!=""`                  | `test/integration/aianalysis/`                  |
| IT-KA-1390-W05  | SC-24   | MCP disconnect during upgrade window -> ForceComplete graceful       | Session transitions to `completed`, no panic, no orphan goroutine                       | `test/integration/kubernautagent/session/`      |

## 9. E2E Test Scenarios

| ID              | FedRAMP    | Business Behavior Verified                                                                                | Acceptance Criteria                                                                      | Location                       |
|-----------------|------------|-----------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------|--------------------------------|
| E2E-FP-1390-001 | SC-24,AC-12| Upgrade journey: AA autonomous -> IS -> MCP connect -> upgrade -> RCA -> user interacts -> complete        | Full journey, AA terminal, session_id consistent, no cancel                              | `test/e2e/fullpipeline/`       |
| E2E-KA-1390-001 | SC-24,SI-13| Nil-result resilience: timeout -> complete(nil) -> AA polls -> structured 200 -> AA terminal               | AA exits loop in 2 cycles, no 409, AIAnalysis reaches terminal                           | `test/e2e/kubernautagent/`     |

## 10. Tests Modified (existing)

| Test              | Current Assertion                              | New Assertion                                           |
|-------------------|------------------------------------------------|---------------------------------------------------------|
| UT-AA-1293-SC1-005| `cancelCount=1`, `ID=""`                       | `cancelCount=0`, `Interactive=true`, ID preserved       |
| IT-AA-1293-002    | `GetCancelCallCount() >= 1`                    | No cancel, `Interactive=true`                           |
| IT-AA-1293-004    | Cancel + resubmit + new ID                     | No cancel, `Interactive=true`, `SetActivePhase` called  |
| UT-KA-1326-021    | Empty `InvestigationSessionID` for running     | Non-empty `InvestigationSessionID`                      |

## 11. TDD Execution Phases

| Phase | Type     | Scope                                                       | Tests                                                 |
|-------|----------|-------------------------------------------------------------|-------------------------------------------------------|
| 1     | RED      | Layer 1: KA session upgrade + store deterministic           | UT 001-013, 027-029; IT W01, W03, W05                 |
| 2     | GREEN    | Layer 1: Implement store, context, manager, investigator    | All Phase 1 tests pass                                |
| 3     | REFACTOR | Layer 1: 100-go-mistakes concurrency, lint, race            | All tests remain green                                |
| 4     | RED      | Layer 2: AA takeover simplification                         | UT 014-017; IT W04                                    |
| 5     | GREEN    | Layer 2: Implement investigating.go change, update tests    | All Phase 4 + existing tests pass                     |
| 6     | REFACTOR | Layer 2: 100-go-mistakes error handling, lint               | All tests remain green                                |
| 7     | RED      | Layer 3: Nil-result 409 loop fix                            | UT 018-024; IT W02                                    |
| 8     | GREEN    | Layer 3: Implement handler + AA retry cap                   | All Phase 7 tests pass                                |
| 9     | REFACTOR | Layer 3: DescribeTable, 100-go-mistakes nil handling        | All tests remain green                                |
| 10    | RED      | Layer 4 + E2E: Observability + journeys                     | UT 025-026; E2E-FP-001, E2E-KA-001                   |
| 11    | GREEN    | Layer 4 + E2E: session_id propagation, E2E wiring           | All tests pass (E2E requires Kind)                    |
| 12    | REFACTOR | Final: Full checklist, coverage, lint, race                  | All 36 scenarios green                                |

## 12. Checkpoints

| Checkpoint | Gate                                                                                     |
|------------|------------------------------------------------------------------------------------------|
| CP-1       | Post-Layer 1: build, lint, race, wiring (CHECKPOINT W), BDD, test IDs, 100-go-mistakes   |
| CP-2       | Post-Layer 2: all L1+L2 tests, wiring, BR alignment, updated existing tests green        |
| CP-3       | Post-Layer 3: all L1-L3, race, nil-result matrix coverage, AA retry cap verified         |
| CP-4       | Final: all 36 scenarios, >=80%/tier coverage, FedRAMP, wiring manifest, BR traceability  |

## 13. 100 Go Mistakes Validation Checklist

Applied during each TDD REFACTOR phase to both test code and any business code touched.

### Error Management (#48-#54) -- P0

- [ ] #28: Nil vs empty: completed session with nil result vs "no workflow" semantic
- [ ] #49: Wrap errors with `fmt.Errorf("context: %w", err)` in upgrade/handler paths
- [ ] #50: Use `errors.Is()` / `errors.As()` not `==` for sentinel errors
- [ ] #52: Don't log and return same error
- [ ] #53: Verify all deferred error paths handled
- [ ] #54: Every error path returns or logs

### Concurrency (#55-#74) -- P0

- [ ] #57: `interactiveUpgrade` uses `atomic.Bool` (lock-free read), upgrade holds mu
- [ ] #58: Run `-race` on `session/`, `pkg/aianalysis/handlers/` tests
- [ ] #62: Goroutine stop plan -- upgrade does NOT cancel goroutine; no leak
- [ ] #63: No loop variable capture in goroutines
- [ ] #74: No copy of `sync.Mutex` / `atomic.Bool` (pointer-based on Session)

### Channel (#64-#68) -- P1

- [ ] #65: No `defer` in channel receive loops
- [ ] #67: Buffer sizing -- `eventChannelBuffer = 64` unchanged
- [ ] #68: Channel close ownership -- only `closeEventChan` closes

### Context (#59-#61) -- P1

- [ ] #60: `WithInteractiveUpgrade` follows `WithLazySink` pattern
- [ ] #61: `bgCtx` carries `session_id` for all autonomous paths

### Testing (#86-#91) -- P1

- [ ] #87: `-race` flag enabled for all test targets
- [ ] #89: `DescribeTable` for nil-result matrix
- [ ] #90: No `time.Sleep` -- use `Eventually()` with polling
- [ ] #91: `DeferCleanup()` for channel/session cleanup in lifecycle tests

### Interface Design (#5-#8) -- P2

- [ ] #5: No interface pollution -- method on existing `Manager`
- [ ] #7: Return concrete types from constructors

## 14. Risk Assessment

| Risk                                    | Severity | Mitigation                                                   |
|-----------------------------------------|----------|--------------------------------------------------------------|
| Upgrade-completion race                 | Eliminated | Store-level mu serialization (UT-KA-1390-027..029)         |
| SetActivePhase timing                   | Not new  | Existing K8s dependency; new flow is faster (1 vs 2 cycles) |
| E2E flakiness                           | Medium   | `Eventually()` with generous timeouts                        |
| ConsecutiveGetResultErrors CRD field    | Low      | May require API type update; verified in Phase 8             |
| BR-INTERACTIVE-010 SC-1 wording         | Low      | Docs-only update in separate PR                              |

## 15. Status

| Test ID          | Status  |
|------------------|---------|
| UT-KA-1390-001   | Pending |
| UT-KA-1390-002   | Pending |
| UT-KA-1390-003   | Pending |
| UT-KA-1390-004   | Pending |
| UT-KA-1390-005   | Pending |
| UT-KA-1390-006   | Pending |
| UT-KA-1390-007   | Pending |
| UT-KA-1390-008   | Pending |
| UT-KA-1390-009   | Pending |
| UT-KA-1390-010   | Pending |
| UT-KA-1390-011   | Pending |
| UT-KA-1390-012   | Pending |
| UT-KA-1390-013   | Pending |
| UT-AA-1390-014   | Pending |
| UT-AA-1390-015   | Pending |
| UT-AA-1390-016   | Pending |
| UT-AA-1390-017   | Pending |
| UT-KA-1390-018   | Pending |
| UT-KA-1390-019   | Pending |
| UT-KA-1390-020   | Pending |
| UT-KA-1390-021   | Pending |
| UT-AA-1390-022   | Pending |
| UT-AA-1390-023   | Pending |
| UT-AA-1390-024   | Pending |
| UT-KA-1390-025   | Pending |
| UT-KA-1390-026   | Pending |
| UT-KA-1390-027   | Pending |
| UT-KA-1390-028   | Pending |
| UT-KA-1390-029   | Pending |
| IT-KA-1390-W01   | Pending |
| IT-KA-1390-W02   | Pending |
| IT-KA-1390-W03   | Pending |
| IT-AA-1390-W04   | Pending |
| IT-KA-1390-W05   | Pending |
| E2E-FP-1390-001  | Pending |
| E2E-KA-1390-001  | Pending |
