# Test Plan: Interactive Investigation Architecture (#1293)

## 1. Test Plan Identifier

**TP-1293-INTERACTIVE-001**

| Field | Value |
|-------|-------|
| Version | 1.0 |
| Date | 2026-05-26 |
| Author | AI Agent |
| Status | Draft |
| Business Requirement | BR-INTERACTIVE-010 |

## 2. Introduction

This test plan covers the Interactive Investigation Architecture feature (#1293), which uses the InvestigationSession (IS) CRD as a universal signal for interactive mode across KA, AA, RO, and AF components.

### 2.1 Scope

- KA: API schema, session pending state, investigator interactive hold, MCP start, context reconstruction, discover_workflows enrichment fix
- AA: IS watch, field index, cancel client, dynamic takeover, IS deletion handling, cancelled poll status
- RO: IS field index, cooldown bypass for IS-backed RRs
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
| TI-13 | RO | IS field index + query | 1.5 |
| TI-14 | RO | Cooldown bypass for IS-backed RRs | 1.5 |
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
| RO cooldown bypass | Medium | Low |
| AF SA detection + IS guard | Medium | Low |
| AF field selector migration | Medium | Low |
| discover_workflows Phase 2 enrichment | Medium | Medium |

### 4.2 Non-Functional Features

| Feature | Priority |
|---------|----------|
| No token overflow on context reconstruction | High |
| Cancel idempotency (race-safe) | High |
| Field index performance (O(1) lookup) | Medium |
| Audit trail completeness | Medium |

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

| ID | Scenario | Input | Expected | TI |
|----|----------|-------|----------|-----|
| UT-KA-1293-001 | Session created in pending when interactive=true | `IncidentRequest{Interactive: true}` | Session with `StatusPending`, no goroutine | TI-02 |
| UT-KA-1293-002 | Session launches normally when interactive=false | `IncidentRequest{Interactive: false}` | Session with `StatusRunning`, goroutine active | TI-02 |
| UT-KA-1293-003 | Investigator skips Phase 2+3 when interactive | `SignalContext{Interactive: true}` | `InteractiveHold=true`, no workflow selection | TI-03 |
| UT-KA-1293-004 | Investigator runs full pipeline when not interactive | `SignalContext{Interactive: false}` | Full result with workflows | TI-03 |
| UT-KA-1293-005 | Session status → UserDriving on InteractiveHold | Result with `InteractiveHold=true` | `StatusUserDriving` | TI-02 |
| UT-KA-1293-006 | MCP handleStart detects pending session | Session in `StatusPending` | Launches Investigate(), returns session_id | TI-04 |
| UT-KA-1293-007 | MCP handleStart rejects if session already running | Session in `StatusRunning` | Error: session already active | TI-04 |
| UT-KA-1293-008 | Context reconstruction queries DS by correlation_id | `remediation_id=rr-123` | Calls `QueryAuditEvents(correlation_id=rr-123)` | TI-05 |
| UT-KA-1293-009 | Context reconstruction uses latest session (chained) | Multiple sessions for same RR | Uses most recent audit events | TI-05 |
| UT-KA-1293-010 | Context reconstruction handles empty DS response | No prior sessions | Starts with empty context (fresh) | TI-05 |
| UT-KA-1293-011 | discover_workflows includes Phase 2 enrichment | Active session with RCA result | Calls `ResolveEnrichmentTarget` + `enricher.Enrich` before workflow selection | TI-06 |
| UT-KA-1293-012 | mapSessionStatusToAPI returns "pending" for pending | `StatusPending` | `"pending"` string | TI-02 |

### 7.2 AA Unit Tests

| ID | Scenario | Input | Expected | TI |
|----|----------|-------|----------|-----|
| UT-AA-1293-001 | IS found before submit → interactive=true | IS CRD exists with Active phase | `IncidentRequest.Interactive=true` | TI-08 |
| UT-AA-1293-002 | No IS before submit → interactive=false | No IS CRD for RR | `IncidentRequest.Interactive=false` | TI-08 |
| UT-AA-1293-003 | IS with non-Active phase ignored | IS CRD with phase=Completed | `IncidentRequest.Interactive=false` | TI-08 |
| UT-AA-1293-004 | Poll status "cancelled" + IS exists → re-submit | Status="cancelled", IS Active | Re-submit with interactive=true | TI-11 |
| UT-AA-1293-005 | Poll status "cancelled" + IS deleted → PhaseCancelled | Status="cancelled", no IS | Transition to PhaseCancelled | TI-11 |
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

### 7.4 RO Unit Tests

| ID | Scenario | Input | Expected | TI |
|----|----------|-------|----------|-----|
| UT-RO-1293-001 | Cooldown bypassed when active IS exists | RR with IS (Active phase), consecutive failures exceeded | No blocking condition returned | TI-14 |
| UT-RO-1293-002 | Cooldown enforced when no IS exists | RR without IS, consecutive failures exceeded | BlockingCondition returned | TI-14 |
| UT-RO-1293-003 | Dedup NOT bypassed even with IS | RR with IS, duplicate in progress | BlockingCondition (duplicate) returned | TI-14 |
| UT-RO-1293-004 | Exponential backoff bypassed with IS | RR with IS, NextAllowedExecution in future | No blocking condition | TI-14 |

### 7.5 RO Integration Tests

| ID | Scenario | Input | Expected | TI |
|----|----------|-------|----------|-----|
| IT-RO-1293-001 | Field index returns IS by RR name | IS CRD with remediationRequestRef.name="rr-test" | List with MatchingFields returns IS | TI-13 |
| IT-RO-1293-002 | Full bypass flow: IS + cooldown → proceeds | RR created with IS, cooldown threshold exceeded | RR transitions to Processing (not Blocked) | TI-14 |

### 7.6 AF Unit Tests

| ID | Scenario | Input | Expected | TI |
|----|----------|-------|----------|-----|
| UT-AF-1293-001 | SA token sets IsServiceAccount=true | TokenReview with SA username | `UserIdentity.IsServiceAccount=true` | TI-15 |
| UT-AF-1293-002 | Human token sets IsServiceAccount=false | TokenReview with human username | `UserIdentity.IsServiceAccount=false` | TI-15 |
| UT-AF-1293-003 | MaterializeCRD rejects SA caller | IsServiceAccount=true, CreateConfig present | Error: SA cannot create IS | TI-16 |
| UT-AF-1293-004 | MaterializeCRD accepts human caller | IsServiceAccount=false, CreateConfig present | IS CRD created successfully | TI-16 |
| UT-AF-1293-005 | Single-driver guard uses field selector | Two IS CRDs for same RR, different users | Error: active session exists | TI-16 |

### 7.7 E2E Tests

| ID | Scenario | Components | Expected |
|----|----------|------------|----------|
| E2E-1293-001 | Interactive from start: full flow | AF → RO → AA → KA | IS created → KA session pending → MCP start → RCA → user_driving |
| E2E-1293-002 | Dynamic takeover: autonomous → interactive | AF → AA → KA | IS created mid-investigation → cancel → re-submit → pending |
| E2E-1293-003 | IS deletion cancels investigation | AF → AA → KA | IS deleted → KA cancelled → AIAnalysis PhaseCancelled |
| E2E-1293-004 | SA caller blocked from IS creation | AF | SA token → af_create_rr → MaterializeCRD rejected |
| E2E-1293-005 | Cooldown bypass with IS | AF → RO | Terminal RR + IS → new RR bypasses cooldown → Processing |
| E2E-1293-006 | Context reconstruction from prior session | KA → DS | Prior completed session → new interactive session → context pre-loaded |

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
| KA unit tests | `internal/kubernautagent/session/interactive_test.go` |
| KA investigator tests | `internal/kubernautagent/investigator/interactive_test.go` |
| KA MCP tests | `internal/kubernautagent/mcp/tools/interactive_start_test.go` |
| KA reconstruction tests | `internal/kubernautagent/mcp/reconstruction_interactive_test.go` |
| AA unit tests | `pkg/aianalysis/handlers/interactive_signal_test.go` |
| AA integration tests | `test/integration/aianalysis/interactive_watch_test.go` |
| AA cancel client tests | `pkg/agentclient/cancel_session_test.go` |
| RO unit tests | `pkg/remediationorchestrator/routing/interactive_bypass_test.go` |
| RO integration tests | `test/integration/remediationorchestrator/interactive_cooldown_test.go` |
| AF unit tests | `pkg/apifrontend/session/sa_guard_test.go` |
| AF auth tests | `pkg/apifrontend/auth/sa_detection_test.go` |
| E2E tests | `test/e2e/apifrontend/interactive_investigation_test.go` |

## 11. Schedule

| Phase | Scope | Estimated Duration |
|-------|-------|-------------------|
| Phase 1 | KA: API + session + investigator | TDD Red → Green → Refactor |
| Phase 2 | KA: MCP start + reconstruction + discover enrichment | TDD Red → Green → Refactor |
| Phase 3 | AA: RBAC + field index + IS watch + cancel + poll | TDD Red → Green → Refactor |
| Phase 4 | RO: RBAC + field index + cooldown bypass | TDD Red → Green → Refactor |
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
