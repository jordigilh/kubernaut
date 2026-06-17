# IEEE 829 Test Plan — Fix #1440: IS CRD Co-Creation + KA Session Robustness

| Field                | Value                                                                              |
|----------------------|------------------------------------------------------------------------------------|
| **Test Plan ID**     | TP-1440                                                                            |
| **Revision**         | 1.0                                                                                |
| **Author**           | Kubernaut AI Agent                                                                 |
| **Date**             | 2026-06-16                                                                         |
| **Status**           | Active                                                                             |
| **Business Req**     | BR-INTERACTIVE-010, BR-INTERACTIVE-004                                             |
| **FedRAMP Controls** | AU-12, SI-4, AC-3, SC-24                                                           |

## 1. Introduction

This test plan covers the verification of fixes for GitHub issue #1440, which
describes a failure in interactive takeover of an autonomous investigation where
the user is "kicked from their own session."

Root cause: two-pronged failure mode in the AF-AA-KA session lifecycle:

- **Bug A (IS CRD temporal gap)**: `kubernaut_investigate_alert` creates the RR
  in one tool call, but the IS CRD is deferred to a subsequent
  `kubernaut_investigate` call. During the ~2 second LLM inference gap, the
  RO+SP pipeline creates the AIA, and AA submits to KA as autonomous (no IS CRD
  found by `HasActiveSession`). Violates BR-INTERACTIVE-010.

- **Bug B (KA session-not-found failure)**: When AF sends `action=start` after
  the autonomous session has already completed (or hasn't been submitted by AA
  yet), `handleStart` fails to create a usable interactive session. The user
  acquires the MCP lease but has no investigation to drive. Violates SC-24.

Fix approach:
1. Co-create IS CRD alongside RR in `HandleInvestigateAlert` (closes temporal gap)
2. Make KA `handleStart` robust for all session states (completed, missing, GC'd)

## 2. Scope

### In Scope

| Area                                   | Description                                                         |
|----------------------------------------|---------------------------------------------------------------------|
| AF HandleInvestigateAlert              | ISSignaler field addition, IS CRD co-creation logic                 |
| AF agent root wiring                   | Pass `buildAgentISSignaler(cfg)` to InvestigateAlertConfig          |
| KA handleStart fallback                | Create fresh interactive session when no viable session exists       |
| KA AutonomousSessionManager interface  | Extend with session creation method for MCP-only path               |
| KA signal resolver                     | Resolve signal context for MCP-initiated sessions                   |

### Out of Scope

- HandleAwaitSession short-circuit logic (separate issue)
- MCP session lifecycle / GC (separate issue)
- KA action=start/takeover unification (proven unnecessary by spike)
- AF `kubernaut_investigate` signaler path (already works correctly)

## 3. Business Requirements Traceability

| BR                    | Description                                                         | Violated By |
|-----------------------|---------------------------------------------------------------------|-------------|
| BR-INTERACTIVE-010    | IS CRD as universal interactive signal; AA must detect IS before autonomous submit | Bug A |
| BR-INTERACTIVE-004    | Dynamic takeover preserves in-flight work; KA must handle all states | Bug B       |

## 4. FedRAMP / NIST 800-53 Rev 5 Control Verification

Tests verify **business-level behavior** tied to each control objective.

### AU-12 -- Audit Record Generation

**Business behavior**: IS CRD creation MUST be auditable. The signaler emits an
audit event (`EventSessionCreated`) with user identity, task ID, and RR
reference when creating the IS CRD in `HandleInvestigateAlert`.

**Tests**: UT-AF-1440-005, IT-AF-1440-001

### SI-4 -- Information System Monitoring

**Business behavior**: Interactive intent signal (IS CRD) MUST be visible to AA
before the autonomous submit decision. AA uses a direct API reader (not informer
cache) to check for IS CRDs. The IS must exist before AA's
`handleSessionSubmit` calls `HasActiveSession`.

**Tests**: UT-AF-1440-001, UT-AF-1440-002, IT-AF-1440-002

### AC-3 -- Access Enforcement

**Business behavior**: IS CRD carries user identity (username, groups) for
session RBAC. The signaler receives the authenticated user from
`auth.UserIdentityFromContext` and writes it to `spec.userIdentity` on the IS
CRD.

**Tests**: UT-AF-1440-005, IT-AF-1440-001

### SC-24 -- Fail in Known State

**Business behavior**: KA `handleStart` MUST produce a usable interactive
session regardless of prior session state. When the autonomous session has
completed, been GC'd, or was never submitted by AA, KA creates a fresh
interactive session with signal context resolved from the RR CRD. The user is
never left with a lease but no session to drive.

**Tests**: UT-KA-1440-010, UT-KA-1440-011, UT-KA-1440-012, IT-KA-1440-010

## 5. Test Tiers and Coverage Targets

| Tier              | Testable Code                                                       | Target |
|-------------------|---------------------------------------------------------------------|--------|
| Unit Tests        | HandleInvestigateAlert signaler logic, KA handleStart fallback      | >=80%  |
| Integration Tests | ADK tool -> signaler wiring, is_checker visibility, MCP dispatch    | >=80%  |
| E2E Tests         | Full pipeline: AF -> RR+IS -> RO/SP/AIA -> AA interactive -> KA    | >=80%  |

## 6. Wiring Manifest (Pyramid Invariant)

> UT proves logic. IT proves wiring. E2E proves the journey.

| Component                              | Production Entry Point                      | Wiring Location                              | UT (logic)               | IT/E2E (wiring)       |
|----------------------------------------|---------------------------------------------|----------------------------------------------|--------------------------|----------------------|
| `ISSignaler` on InvestigateAlertConfig | `kubernaut_investigate_alert` ADK tool call | `pkg/apifrontend/agent/root.go:~153`         | UT-AF-1440-001..005      | IT-AF-1440-001       |
| IS co-creation in HandleInvestigateAlert| HandleCreateRR success path                | `pkg/apifrontend/tools/af_investigate_alert.go` | UT-AF-1440-001..004   | IT-AF-1440-002       |
| KA fallback session creation           | `handleStart` no-viable-session branch      | `internal/kubernautagent/mcp/tools/investigate.go` | UT-KA-1440-010..012 | IT-KA-1440-010       |
| Full interactive journey               | AF -> AA -> KA pipeline                     | Cross-service                                | --                       | E2E-AF-1440-001      |

## 7. Unit Test Scenarios

### Layer 1: AF IS CRD Co-Creation

| ID              | FedRAMP | Business Behavior Verified                                                                  | Acceptance Criteria                                                              |
|-----------------|---------|---------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------|
| UT-AF-1440-001  | SI-4    | `HandleInvestigateAlert` calls Signaler.SignalInteractive when Signaler is provided and RR is new | `recorder.signalCalls` has length 1; `rrName == result.RRID`                    |
| UT-AF-1440-002  | SI-4    | `HandleInvestigateAlert` calls Signaler when RR AlreadyExists (user intent is interactive)   | Second call with same args: `recorder.signalCalls` has length 1                  |
| UT-AF-1440-003  | SC-24   | `HandleInvestigateAlert` succeeds when Signaler is nil (backward compatibility)              | No panic, `result.RRID != ""`                                                    |
| UT-AF-1440-004  | SC-24   | `HandleInvestigateAlert` succeeds when Signaler returns error (best-effort, non-blocking)    | `err == nil`, `result.RRID != ""`                                                |
| UT-AF-1440-005  | AC-3,AU-12 | Signaler receives correct taskID (`a2a-{RRID}`), username, groups from auth context       | `call.taskID == "a2a-"+RRID`, `call.username == identity.Username`, `call.groups == identity.Groups` |

### Layer 2: KA handleStart Robustness

| ID              | FedRAMP | Business Behavior Verified                                                                  | Acceptance Criteria                                                              |
|-----------------|---------|---------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------|
| UT-KA-1440-010  | SC-24   | `handleStart` creates fresh interactive session when no session exists for RR                | `output.SessionID != ""`, `output.Status == "started"`, no error                 |
| UT-KA-1440-011  | SC-24   | `handleStart` creates fresh interactive session when prior session is terminal (completed)   | `output.SessionID != ""`, session created despite existing terminal session      |
| UT-KA-1440-012  | SC-24   | `handleStart` preserves RCA context from completed autonomous session via storeReconstructedContext | `reconHistory` contains prior RCA summary from completed session              |

## 8. Integration Test Scenarios

| ID              | FedRAMP    | Business Behavior Verified                                          | Acceptance Criteria                                                                     | Location                                       |
|-----------------|------------|---------------------------------------------------------------------|-----------------------------------------------------------------------------------------|-------------------------------------------------|
| IT-AF-1440-001  | SI-4,AU-12 | `kubernaut_investigate_alert` ADK tool creates IS CRD via production signaler wiring | IS CRD exists in fake K8s client after tool call; audit event emitted                   | `pkg/apifrontend/tools/`                        |
| IT-AF-1440-002  | SI-4       | IS CRD created by investigate_alert is findable by `is_checker.HasActiveSession`    | `HasActiveSession(rrName) == true` using direct API reader on fake client               | `pkg/apifrontend/tools/`                        |
| IT-KA-1440-010  | SC-24      | MCP `action=start` with no prior session creates interactive session and returns valid session_id | `output.SessionID != ""`, session findable by `FindByRemediationID` or `FindPending`  | `internal/kubernautagent/mcp/tools/`            |

## 9. E2E Test Scenarios

| ID              | FedRAMP      | Business Behavior Verified                                                                                | Acceptance Criteria                                                                      | Location                       |
|-----------------|--------------|-----------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------|--------------------------------|
| E2E-AF-1440-001 | SI-4,SC-24   | Full pipeline: kubernaut_investigate_alert creates RR+IS, AA submits with interactive=true, AF action=start launches deferred | AA AIA status shows `Interactive==true`; KA session reaches `user_driving`; user can message | `test/e2e/fullpipeline/`       |

## 10. TDD Execution Phases

| Phase | Type     | Scope                                                       | Tests                                           |
|-------|----------|-------------------------------------------------------------|-------------------------------------------------|
| 1     | RED      | Layer 1: AF IS co-creation                                  | UT-AF-1440-001..005; IT-AF-1440-001..002        |
| 2     | GREEN    | Layer 1: ISSignaler field, co-creation logic, agent wiring  | All Phase 1 tests pass                          |
| 3     | REFACTOR | Layer 1: 100-go-mistakes, lint, race                        | All tests remain green                          |
| 4     | RED      | Layer 2: KA handleStart robustness                          | UT-KA-1440-010..012; IT-KA-1440-010             |
| 5     | GREEN    | Layer 2: Interface extension, fallback creation, signal resolver | All Phase 4 tests pass                       |
| 6     | REFACTOR | Layer 2: 100-go-mistakes, concurrency, lint                 | All tests remain green                          |
| 7     | RED      | E2E journey                                                 | E2E-AF-1440-001                                 |
| 8     | GREEN    | E2E wiring (requires Kind cluster or httptest stack)        | All tests pass                                  |
| 9     | REFACTOR | Final: full checklist, coverage, lint, race                 | All 12 scenarios green                          |

## 11. Checkpoints

| Checkpoint | Gate                                                                                     |
|------------|------------------------------------------------------------------------------------------|
| CP-1       | Post-Layer 1: build, lint, race, CHECKPOINT W, BDD, test IDs                             |
| CP-2       | Post-Layer 2: all L1+L2 tests, wiring, BR alignment                                     |
| CP-3       | Final: all scenarios, >=80%/tier coverage, FedRAMP, wiring manifest, BR traceability     |

## 12. 100 Go Mistakes Validation Checklist

Applied during each TDD REFACTOR phase.

### Error Management (#48-#54) -- P0

- [ ] #49: Wrap errors with `fmt.Errorf("context: %w", err)` in signaler paths
- [ ] #50: Use `errors.Is()` / `errors.As()` not `==` for sentinel errors
- [ ] #52: Don't log and return same error (signaler best-effort ignores error)
- [ ] #54: Every error path returns or logs

### Concurrency (#55-#74) -- P0

- [ ] #58: Run `-race` on `pkg/apifrontend/tools/` and `internal/kubernautagent/mcp/tools/` tests
- [ ] #62: No goroutine leak from fallback session creation
- [ ] #63: No loop variable capture in goroutines
- [ ] #74: No copy of sync primitives

### Interface Design (#5-#8) -- P2

- [ ] #5: No interface pollution -- extend existing `AutonomousSessionManager` minimally
- [ ] #7: Return concrete types from constructors

### Testing (#86-#91) -- P1

- [ ] #87: `-race` flag enabled for all test targets
- [ ] #90: No `time.Sleep` -- use `Eventually()` with polling
- [ ] #91: `DeferCleanup()` for resource cleanup in lifecycle tests

## 13. Risk Assessment

| Risk                                    | Severity | Mitigation                                                   |
|-----------------------------------------|----------|--------------------------------------------------------------|
| AutonomousSessionManager interface change breaks mocks | Medium | Minimal extension; update all implementors in same PR      |
| Signal resolver thin context for MCP-only sessions | Low | RR CRD fallback sufficient for interactive; enrichment happens in KA investigator |
| Race between MCP start and AA submit (duplicate sessions) | Low | LeaseSessionManager enforces one driver; GetLatest* picks newest |
| E2E flakiness in Kind cluster           | Medium   | `Eventually()` with generous timeouts; fallback to IT level  |

## 14. Status

| Test ID          | Status  |
|------------------|---------|
| UT-AF-1440-001   | Pending |
| UT-AF-1440-002   | Pending |
| UT-AF-1440-003   | Pending |
| UT-AF-1440-004   | Pending |
| UT-AF-1440-005   | Pending |
| UT-KA-1440-010   | Pending |
| UT-KA-1440-011   | Pending |
| UT-KA-1440-012   | Pending |
| IT-AF-1440-001   | Pending |
| IT-AF-1440-002   | Pending |
| IT-KA-1440-010   | Pending |
| E2E-AF-1440-001  | Pending |
