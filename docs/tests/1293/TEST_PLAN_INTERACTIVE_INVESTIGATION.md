# Test Plan: Interactive Investigation Architecture (#1293)

## 1. Test Plan Identifier

**TP-1293-INTERACTIVE-001**

| Field | Value |
|-------|-------|
| Version | 1.1 |
| Date | 2026-05-26 |
| Author | AI Agent |
| Status | Implementation Complete |
| Business Requirement | BR-INTERACTIVE-010 |

## 2. Introduction

This test plan covers the Interactive Investigation Architecture feature (#1293), which uses the InvestigationSession (IS) CRD as a universal signal for interactive mode across KA, AA, RO, and AF components.

### 2.1 Scope

- KA: API schema, session pending state, investigator interactive hold, MCP start, context reconstruction, discover_workflows enrichment fix
- AA: IS watch, field index, cancel client, dynamic takeover, IS deletion handling, cancelled poll status
- ~~RO: IS field index, cooldown bypass for IS-backed RRs~~ (SC-4 CANCELLED BY DESIGN — AF inherently bypasses RO cooldown)
- AF: SA detection, IS creation guard, field selector migration, prompt rewrite, KA readiness tool

### 2.2 Out of Scope

- v1.6 autonomous restart after IS deletion
- `maxUserDrivingDuration` for AA
- Multi-replica session affinity
- Full observability dashboard

## 3. Test Items

| ID | Component | Item | Version |
|----|-----------|------|---------|
| TI-01 | KA | `IncidentRequest.Interactive` field | v1alpha1 |
| TI-02 | KA | `session.Manager` pending state logic | 1.5 |
| TI-03 | KA | `investigator.Investigate()` interactive hold | 1.5 |
| TI-04 | KA | MCP `handleStart` awaiting session detection | 1.5 |
| TI-05 | KA | `ReconstructionSpawner` chained session context | 1.5 |
| TI-06 | KA | MCP `discover_workflows` Phase 2 enrichment | 1.5 |
| TI-07 | AA | IS field index + query | 1.5 |
| TI-08 | AA | `handleSessionSubmit` IS check | 1.5 |
| TI-09 | AA | IS watch + dynamic takeover (cancel + re-submit) | 1.5 |
| TI-10 | AA | IS deletion → cancel | 1.5 |
| TI-11 | AA | `"cancelled"` poll status handling | 1.5 |
| TI-12 | AA | `CancelSession` client method | 1.5 |
| ~~TI-13~~ | ~~RO~~ | ~~IS field index + query~~ | ~~CANCELLED (SC-4)~~ |
| ~~TI-14~~ | ~~RO~~ | ~~Cooldown bypass for IS-backed RRs~~ | ~~CANCELLED (SC-4)~~ |
| TI-15 | AF | `UserIdentity.IsServiceAccount` detection | 1.5 |
| TI-16 | AF | `MaterializeCRD` SA guard + field selector migration | 1.5 |
| TI-17 | AF | KA readiness tool (AIAnalysis watch) | 1.5 |
| TI-18 | AF | Prompt rewrite validation | 1.5 |

## 4. Features to be Tested

### 4.1 Functional Features

| Feature | Priority | Risk |
|---------|----------|------|
| IS CRD as interactive signal (AA detection) | High | Medium |
| KA pending session (no goroutine launch) | High | Low |
| KA interactive hold after RCA | High | Low |
| KA context reconstruction from DS audit trail | High | High |
| AA cancel + re-submit on IS creation | High | Medium |
| AA cancel on IS deletion | High | Medium |
| ~~RO cooldown bypass~~ | ~~CANCELLED~~ | ~~SC-4 cancelled by design~~ |
| AF SA detection + IS guard | Medium | Low |
| AF field selector migration | Medium | Low |
| discover_workflows Phase 2 enrichment | Medium | Medium |

### 4.2 Non-Functional Features

| Feature | Priority | Status |
|---------|----------|--------|
| No token overflow on context reconstruction | High | Covered by UT-KA-1293-008..010 |
| Cancel idempotency (race-safe) | High | Covered by UT-AA-1293-007/008 |
| Field index performance (O(1) lookup) | Medium | Covered by IT-AA-1293-001 |
| Audit trail completeness (FedRAMP AU-2) | Medium | **Known gap**: IT tests do not assert on audit events emitted during interactive transitions. Audit store is wired but not queried in assertions. Future work: query `auditStore` after IT-AA-1293-003 (PhaseFailed transition) and assert `analysis.failed` event is present with `ReasonInteractiveCancelled`. |

## 5. Features Not to be Tested

- LLM response quality (non-deterministic)
- Multi-replica failover (single replica default)
- DS availability (mocked in unit/integration)
- Network partitions between services

## 6. Approach

### 6.1 Test Pyramid (Invariant)

```
         /  E2E  \          (Kind cluster, real services)
        /----------\
       / Integration \      (envtest, real K8s API, mocked external)
      /----------------\
     /    Unit Tests     \  (pure logic, no I/O)
    /____________________\
```

### 6.2 TDD Methodology

Each test item follows RED → GREEN → REFACTOR:
1. **RED**: Write failing tests defining behavioral contract
2. **GREEN**: Minimal implementation to pass
3. **REFACTOR**: Quality improvements, 100-go-mistakes validation

### 6.3 Coverage Target

- Unit: ≥80% of unit-testable code per component
- Integration: ≥80% of integration-testable code
- E2E: ≥80% of full-stack interactive flow

## 7. Test Scenarios

### 7.1 KA Unit Tests

> **Layered ID Scheme**: Tests that exercise the same TI from different internal components
> use a scope prefix to avoid ID collisions (e.g., `SS-` for Session Store, `SH-` for
> Session Hold). The canonical plan IDs (UT-KA-1293-NNN) live in the file closest to the
> public API surface; supplementary tests carry the prefix.

#### 7.1.1 Signal Handler (`server/interactive_signal_test.go`)

| ID | Scenario | Expected | TI |
|----|----------|----------|-----|
| UT-KA-1293-001 | `MapIncidentRequestToSignal` maps `interactive=true` | `SignalContext.Interactive=true` | TI-01 |
| UT-KA-1293-002 | `MapIncidentRequestToSignal` defaults `interactive=false` | `SignalContext.Interactive=false` | TI-01 |
| UT-KA-1293-003 | `MapIncidentRequestToSignal` maps explicit `interactive=false` | `SignalContext.Interactive=false` | TI-01 |
| UT-KA-1293-004 | Handler creates interactive session in pending state | Session `StatusPending`, no goroutine | TI-02 |
| UT-KA-1293-005 | Handler launches investigation normally for non-interactive | Session `StatusRunning`, goroutine active | TI-02 |
| UT-KA-1293-012 | `mapSessionStatusToAPI` returns `"pending"` for `StatusPending` | `"pending"` string | TI-02 |

#### 7.1.2 MCP Tools — handleStart & discover_workflows (`mcp/tools/investigate_test.go`)

| ID | Scenario | Expected | TI |
|----|----------|----------|-----|
| UT-KA-1293-007 | `handleStart` rejects reconnected session | Error: session already active | TI-04 |
| UT-KA-1293-011 | `discover_workflows` calls `ResolvePostRCAEnrichment` | `enricher.Enrich` invoked before workflow selection | TI-06 |

#### 7.1.3 MCP Tools — Start Action (`mcp/tools/interactive_start_test.go`)

| ID | Scenario | Expected | TI |
|----|----------|----------|-----|
| UT-KA-1293-013 | `action=start` detects pending session and launches deferred investigation | `LaunchDeferredInvestigation` called, returns `session_id` | TI-04 |
| UT-KA-1293-014 | `action=start` skips pending check when no pending session | Proceeds without launching | TI-04 |
| UT-KA-1293-015 | `action=start` handles `LaunchDeferredInvestigation` failure | Graceful error propagated | TI-04 |

#### 7.1.4 Context Reconstruction (`mcp/tools/reconstruction_test.go`)

| ID | Scenario | Expected | TI |
|----|----------|----------|-----|
| UT-KA-1293-008 | RCA summary available → single assistant turn | Stores RCA-prefixed turn, skips DS call | TI-05 |
| UT-KA-1293-009 | No RCA, DS returns turns → multiple turns stored | Calls `Reconstruct`, stores all turns | TI-05 |
| UT-KA-1293-010 | No RCA, DS error → empty context | Falls back to empty context | TI-05 |

#### 7.1.5 Investigator Hold (`investigator/interactive_hold_test.go`)

| ID | Scenario | Expected | TI |
|----|----------|----------|-----|
| UT-KA-1293-016 | `Investigate` with `Interactive=true` → `InteractiveHold=true` after RCA | Holds after RCA, no workflow selection | TI-03 |
| UT-KA-1293-017 | `Investigate` with `Interactive=false` → full pipeline | Full result with workflows | TI-03 |

#### 7.1.6 Session Store — Supplementary (`session/interactive_session_test.go`, SS prefix)

| ID | Scenario | Expected | TI |
|----|----------|----------|-----|
| UT-KA-1293-SS-006 | `StartInteractiveSession` creates session in `StatusPending` | Pending session stored | TI-02 |
| UT-KA-1293-SS-007 | `LaunchDeferredInvestigation` transitions pending → running | Status changes, goroutine active | TI-02 |
| UT-KA-1293-SS-008 | `LaunchDeferredInvestigation` fails for non-pending session | Error returned | TI-02 |
| UT-KA-1293-SS-009 | `LaunchDeferredInvestigation` fails for non-existent session | Error: not found | TI-02 |
| UT-KA-1293-SS-010 | `StartInteractiveSession` stores metadata and `created_by` | Metadata persisted | TI-02 |
| UT-KA-1293-018 | `GetLatestRCASummaryByRemediationID` returns RCA from completed session | Latest RCA summary | TI-05 |
| UT-KA-1293-019 | `FindPendingByRemediationID` on real Manager | Finds pending session | TI-02 |

#### 7.1.7 Session Hold — Supplementary (`session/interactive_hold_test.go`, SH prefix)

| ID | Scenario | Expected | TI |
|----|----------|----------|-----|
| UT-KA-1293-SH-011 | Investigation returning `InteractiveHold=true` → `StatusUserDriving` | Session transitions | TI-02 |
| UT-KA-1293-SH-012 | Investigation returning `InteractiveHold=false` → `StatusCompleted` | Session transitions | TI-02 |

### 7.2 AA Unit Tests

| ID | Scenario | Input | Expected | TI |
|----|----------|-------|----------|-----|
| UT-AA-1293-001 | IS found before submit → interactive=true | IS CRD exists with Active phase | `IncidentRequest.Interactive=true` | TI-08 |
| UT-AA-1293-002 | No IS before submit → interactive=false | No IS CRD for RR | `IncidentRequest.Interactive=false` | TI-08 |
| UT-AA-1293-003 | IS with non-Active phase ignored | IS CRD with phase=Completed | `IncidentRequest.Interactive=false` | TI-08 |
| UT-AA-1293-004 | Poll status "cancelled" + IS exists → re-submit | Status="cancelled", IS Active | Re-submit with interactive=true | TI-11 |
| UT-AA-1293-005 | Poll status "cancelled" + IS deleted → PhaseFailed | Status="cancelled", no IS | Transition to PhaseFailed + ReasonInteractiveCancelled | TI-11 |
| UT-AA-1293-006 | CancelSession calls correct endpoint | session_id="sess-123" | POST /api/v1/incident/session/sess-123/cancel | TI-12 |
| UT-AA-1293-007 | CancelSession handles 404 gracefully | KA returns 404 | No error (session already gone) | TI-12 |
| UT-AA-1293-008 | CancelSession handles 409 gracefully | KA returns 409 (already terminal) | No error (already cancelled) | TI-12 |

### 7.3 AA Integration Tests

| ID | Scenario | Input | Expected | TI |
|----|----------|-------|----------|-----|
| IT-AA-1293-001 | Field index returns IS by RR name | IS CRD with remediationRequestRef.name="rr-test" | List with MatchingFields returns IS | TI-07 |
| IT-AA-1293-002 | Watch triggers on IS creation | New IS CRD created for active investigation | Reconcile triggered, cancel + re-submit | TI-09 |
| IT-AA-1293-003 | Watch triggers on IS deletion | IS CRD deleted | Reconcile triggered, cancel KA session | TI-10 |
| IT-AA-1293-004 | Full takeover flow: IS create → cancel → re-submit | IS created while session "investigating" | Old session cancelled, new session with interactive=true | TI-09 |

### ~~7.4 RO Unit Tests~~ — CANCELLED (SC-4)

> SC-4 (RO cooldown bypass) was CANCELLED BY DESIGN. AF inherently bypasses RO cooldown.
> See BR-INTERACTIVE-010.md for rationale. All UT-RO-1293 and IT-RO-1293 tests are obsolete.

### ~~7.5 RO Integration Tests~~ — CANCELLED (SC-4)

### 7.6 AF Unit Tests

> **Layered ID Scheme**: Decorator-layer tests use `DEC-` prefix, `kubernaut_await_session`
> tests use `SC8-` prefix to avoid ID collisions with the canonical plan IDs.

#### 7.6.1 SA Detection (`auth/sa_detection_test.go`)

| ID | Scenario | Expected | TI |
|----|----------|----------|-----|
| UT-AF-1293-001 | SA token sets `IsServiceAccount=true` | `UserIdentity.IsServiceAccount=true` | TI-15 |
| UT-AF-1293-002 | Unauthenticated token returns error, not false positive | Error: "not authenticated" | TI-15 |

#### 7.6.2 SA Guard & Single-Driver (`session/sa_guard_test.go`)

| ID | Scenario | Expected | TI |
|----|----------|----------|-----|
| UT-AF-1293-003 | SA caller rejected by `MaterializeCRD` | Error: SA cannot create IS | TI-16 |
| UT-AF-1293-004 | Human caller allowed by `MaterializeCRD` | IS CRD created | TI-16 |
| UT-AF-1293-005 | Field selector detects conflicting active session | Error: active session exists | TI-16 |

#### 7.6.3 Decorator Layer — Supplementary (`session/decorator_test.go`, DEC prefix)

| ID | Scenario | Expected | TI |
|----|----------|----------|-----|
| UT-AF-1293-DEC-001 | Decorator-level SA guard rejects SA caller | Error propagated from MaterializeCRD | TI-16 |
| UT-AF-1293-DEC-002 | Decorator-level human caller allowed | IS CRD created via decorator | TI-16 |

#### 7.6.4 KA Readiness Tool — Supplementary (`tools/kubernaut_await_session_test.go`, SC8 prefix)

| ID | Scenario | Expected | TI |
|----|----------|----------|-----|
| UT-AF-1293-SC8-003 | `kubernaut_await_session` returns error when client is nil | Error returned | TI-17 |
| UT-AF-1293-SC8-004 | `kubernaut_await_session` returns error when namespace is empty | Error returned | TI-17 |
| UT-AF-1293-SC8-005 | `kubernaut_await_session` returns error when `rr_name` is empty | Error returned | TI-17 |

### 7.7 E2E Tests

| ID | Scenario | Components | Expected | Status |
|----|----------|------------|----------|--------|
| E2E-1293-001 | Interactive from start: full flow | AF → RO → AA → KA | IS created → KA session pending → MCP start → RCA → user_driving | Implemented |
| E2E-1293-002 | Dynamic takeover: autonomous → interactive | AF → AA → KA | IS created mid-investigation → cancel → re-submit → pending | Implemented |
| E2E-1293-003 | IS deletion cancels investigation | AF → AA → KA | IS deleted → KA cancelled → AIAnalysis PhaseFailed + ReasonInteractiveCancelled | Implemented |
| E2E-1293-004 | SA caller blocked from IS creation | AF | SA token → af_create_rr → MaterializeCRD rejected | Implemented |
| ~~E2E-1293-005~~ | ~~Cooldown bypass with IS~~ | ~~CANCELLED~~ | ~~SC-4 cancelled by design — AF bypasses RO cooldown~~ | Cancelled |
| E2E-1293-006 | Context reconstruction from prior session | KA → DS | Prior completed session → new interactive session → context pre-loaded | Gap: needs mock LLM scenario |

## 8. Pass/Fail Criteria

### 8.1 Per-Scenario

- All assertions pass (Gomega matchers)
- No race conditions detected (`-race` flag)
- No goroutine leaks (GolangCI-Lint)

### 8.2 Per-Tier

- Unit: ≥80% coverage of unit-testable code
- Integration: ≥80% coverage of integration-testable code
- E2E: All scenarios pass in Kind cluster

### 8.3 Overall

- All test tiers pass
- No new lint errors
- Build succeeds (`go build ./...`)
- No 100-go-mistakes violations in new code

## 9. Test Environment

### 9.1 Unit Tests

- Go 1.23+
- Ginkgo/Gomega BDD framework
- Mocked external dependencies (DS, K8s API for unit-only)
- `fake.NewClientBuilder()` for K8s client

### 9.2 Integration Tests

- envtest (real K8s API server, no kubelet)
- controller-runtime test framework
- Field indexes registered in test setup
- Mocked KA REST client (httptest)

### 9.3 E2E Tests

- Kind cluster
- Real KA, AA, RO, AF deployments
- Mock LLM service
- Real DS service

## 10. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| Test plan | `docs/tests/1293/TEST_PLAN_INTERACTIVE_INVESTIGATION.md` |
| KA session store tests (SS) | `internal/kubernautagent/session/interactive_session_test.go` |
| KA session hold tests (SH) | `internal/kubernautagent/session/interactive_hold_test.go` |
| KA investigator tests | `internal/kubernautagent/investigator/interactive_hold_test.go` |
| KA signal handler tests | `internal/kubernautagent/server/interactive_signal_test.go` |
| KA MCP start tests | `internal/kubernautagent/mcp/tools/interactive_start_test.go` |
| KA MCP investigate tests | `internal/kubernautagent/mcp/tools/investigate_test.go` (007, 011) |
| KA reconstruction tests | `internal/kubernautagent/mcp/tools/reconstruction_test.go` |
| AA handler tests | `pkg/aianalysis/investigating_handler_session_test.go` |
| AA integration tests | `test/integration/aianalysis/interactive_watch_test.go` |
| AA cancel client tests | `pkg/aianalysis/agentclient_session_test.go` |
| ~~RO unit tests~~ | ~~CANCELLED (SC-4)~~ |
| ~~RO integration tests~~ | ~~CANCELLED (SC-4)~~ |
| AF SA detection tests | `pkg/apifrontend/auth/sa_detection_test.go` |
| AF SA guard tests | `pkg/apifrontend/session/sa_guard_test.go` |
| AF decorator guard tests (DEC) | `pkg/apifrontend/session/decorator_test.go` (DEC-001, DEC-002) |
| AF await session tests (SC8) | `pkg/apifrontend/tools/kubernaut_await_session_test.go` (SC8-003..005) |
| AF E2E tests (E2E-1293-004) | `test/e2e/apifrontend/interactive_investigation_test.go` |
| Fullpipeline E2E tests (E2E-1293-001..003) | `test/e2e/fullpipeline/10_interactive_investigation_test.go` |

## 11. Schedule

| Phase | Scope | Estimated Duration |
|-------|-------|-------------------|
| Phase 1 | KA: API + session + investigator | TDD Red → Green → Refactor |
| Phase 2 | KA: MCP start + reconstruction + discover enrichment | TDD Red → Green → Refactor |
| Phase 3 | AA: RBAC + field index + IS watch + cancel + poll | TDD Red → Green → Refactor |
| ~~Phase 4~~ | ~~RO: RBAC + field index + cooldown bypass~~ | ~~CANCELLED (SC-4)~~ |
| Phase 5 | AF: SA detection + IS guard + prompt + readiness tool | TDD Red → Green → Refactor |
| Checkpoint | GA readiness audit | Audit + fix |

## 12. Risks and Mitigations

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|-----------|
| Token overflow on reconstruction | Medium | High | Truncation strategy, use summary when available |
| Race between IS watch and AA reconcile | Medium | Medium | Defensive idempotency, re-check IS on submit |
| AA "cancelled" poll → infinite loop | Low | High | Explicit case with IS existence check |
| KA session stuck in pending | Low | Medium | AA 25m cap provides safety net |
| LLM ignores sequential tool instructions | Medium | Medium | Tool design prevents parallel misuse (rr_id dependency) |

## 13. Approvals

| Role | Name | Date | Signature |
|------|------|------|-----------|
| Developer | - | - | - |
| Reviewer | - | - | - |
