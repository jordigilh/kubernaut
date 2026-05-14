# Test Plan: Session Lifecycle Audit Trail for SOC2 Compliance

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-823-AUDIT-v1.0
**Feature**: Session lifecycle audit trail — emit SOC2-compliant audit events for session started/cancelled/completed/failed
**Version**: 1.0
**Created**: 2026-04-24
**Author**: AI Assistant + Jordi Gil
**Status**: Active
**Branch**: `feature/pr1.5-session-audit-trail`

---

## 1. Introduction

### 1.1 Purpose

PR 1 (#823) added session cancellation infrastructure but emitted zero audit events for
session lifecycle transitions. This violates SOC2 CC8.1 (operator attribution) and
BR-AUDIT-005 v2.0 (100% RR reconstruction from audit traces). This test plan covers
the addition of 4 `aiagent.session.*` audit event types to the session Manager, ensuring
every investigation lifecycle transition is audited fire-and-forget per ADR-038.

### 1.2 Objectives

1. **Audit completeness**: Every session lifecycle transition (started, cancelled, completed, failed) emits exactly one audit event
2. **SOC2 traceability**: Audit events carry `remediation_id` as CorrelationID, `session_id`, and `initiated_by` for operator attribution
3. **Fire-and-forget resilience**: Audit store failures never abort investigations (ADR-038)
4. **Nil safety**: A nil AuditStore in NewManager defaults to NopAuditStore without panic
5. **Zero regression**: All existing v1.4 and PR1 tests pass without modification (other than NewManager signature update)

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/session/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/session/...` |
| Unit-testable code coverage (emitter.go) | >=80% | `go test -coverprofile -coverpkg=.../audit` |
| Integration-testable code coverage (manager.go) | >=80% | `go test -coverprofile -coverpkg=.../session` |
| Backward compatibility | 0 regressions | All existing tests pass with NewManager signature update |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-AUDIT-005 v2.0**: SOC2 CC8.1 — all session lifecycle events must be audited for RR reconstruction
- **ADR-038**: Async Buffered Audit Ingestion — fire-and-forget semantics, audit failures never block business logic
- **DD-AUDIT-003 v1.8**: Service Audit Trace Requirements — authoritative event registry for all Kubernaut services
- **Issue #823**: Session Store Cancellation Infrastructure (parent issue)
- **TP-823-v1.0**: Companion test plan for PR1 cancellation infrastructure

### 2.2 Business Requirements — Working Definitions

| BR ID | Definition |
|-------|-----------|
| BR-AUDIT-005 v2.0 | Every business-critical state transition MUST be auditable for 100% RR CRD reconstruction |
| BR-SESSION-001 | A session MUST be uniquely identifiable (session_id as UUID) |
| BR-SESSION-003 | Lifecycle transitions (pending→running→terminal) MUST be traceable |
| SOC2 CC8.1 | Operator attribution: who started, who cancelled, when, with what correlation |

### 2.3 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- [DD-AUDIT-003](../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md)
- [ADR-038](../../architecture/decisions/ADR-038-async-buffered-audit-ingestion.md)
- [PR1 Test Plan](TEST_PLAN.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Nil AuditStore causes panic in Manager | Investigation crashes, availability loss | Medium | IT-KA-823-A07 | NopAuditStore guard in NewManager constructor |
| R2 | Phantom audit events for rejected state transitions | SOC2 reconstructs non-existent sessions | Medium | IT-KA-823-A05 | Emit audit AFTER successful state transition |
| R3 | Missing CorrelationID prevents RR reconstruction | SOC2 audit gap | High | IT-KA-823-A06 | Use `remediation_id` from session metadata as CorrelationID |
| R4 | Audit emission blocks investigation goroutine | Performance degradation under audit backpressure | Medium | IT-KA-823-A08 | Fire-and-forget via `StoreBestEffort` (ADR-038) |
| R5 | Existing tests break from NewManager signature change | False CI failures, regression | High | Checkpoint 0 | Update all 8 callsites; existing tests pass `nil` (guarded by R1 mitigation) |
| R6 | Data race on AuditStore access from background goroutine | Undefined behavior | Low | Checkpoint 3 (race detector) | `StoreBestEffort` uses AuditStore interface which is goroutine-safe by contract |
| R7 | CancelInvestigation lacks actor identity (no ctx parameter) | Incomplete SOC2 CC8.1 attribution for cancel events | Low | N/A (deferred) | Deferred to PR 2 when cancel HTTP endpoint is wired; PR 1.5 uses ActorID="kubernaut-agent" |

### 3.1 Risk-to-Test Traceability

- **R1** → IT-KA-823-A07 (nil AuditStore guard)
- **R2** → IT-KA-823-A05 (no phantom audit after cancel)
- **R3** → IT-KA-823-A06 (correlation fields present)
- **R4** → IT-KA-823-A08 (audit error is fire-and-forget)
- **R5** → Checkpoint 0 (build + existing test regression)
- **R6** → Checkpoint 3 (race detector)
- **R7** → Documented deferral; no test (cancel endpoint not yet wired)

---

## 4. Scope

### 4.1 Features to be Tested

- **Audit event constants** (`internal/kubernautagent/audit/emitter.go`): 4 new `aiagent.session.*` event types and 4 action constants registered in `AllEventTypes`
- **Manager audit emission** (`internal/kubernautagent/session/manager.go`): AuditStore injection, audit event emission at all 4 lifecycle transitions
- **Nil AuditStore guard**: NewManager defaults nil to NopAuditStore
- **Fire-and-forget semantics**: StoreBestEffort wrapping for all audit calls
- **Handler metadata enrichment** (`internal/kubernautagent/server/handler.go`): `remediation_id` added to metadata, user identity extraction from context

### 4.2 Features Not to be Tested

- **DataStorage OpenAPI typed payloads**: Deferred to follow-up issue (new `AIAgentSessionPayload` schema needed in DS OpenAPI spec). `buildEventData` returns untyped fallback.
- **CancelInvestigation actor plumbing**: Deferred to PR 2 when the cancel HTTP endpoint is wired. Current CancelInvestigation has no context parameter.
- **Conversation/SSE audit emission**: Out of scope (different audit category, covered by existing `aiagent.conversation.turn` events)

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Emit audit at Manager level, not Store level | Manager owns lifecycle semantics; Store is a dumb data structure |
| Use `StoreBestEffort` for all audit calls | ADR-038 fire-and-forget; audit failures never block investigations |
| Actor for goroutine events = "kubernaut-agent" | Background goroutine outlives HTTP request; `initiated_by` in event data provides human traceability |
| `buildEventData` returns `(empty, false)` for session events | No typed OpenAPI payload exists yet; DS persists raw event_data as JSONB |
| `CancelInvestigation` keeps `(id string)` signature | No cancel endpoint wired yet; adding ctx would create dead parameter |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (`internal/kubernautagent/audit/emitter.go` — constant registration)
- **Integration**: >=80% of integration-testable code (`internal/kubernautagent/session/manager.go` — audit emission paths)

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers:
- **Unit tests**: Validate audit event type constants, action strings, and registry completeness
- **Integration tests**: Validate Manager emits correct audit events at lifecycle transitions with correct metadata

### 5.3 Business Outcome Quality Bar

Tests validate **business outcomes** — "does the SOC2 auditor see the right events with the right correlation data?" — not just "does the function call succeed?"

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. Per-tier code coverage meets >=80% threshold
3. No regressions in existing test suites (PR1 #823 tests, v1.4 #433 tests)
4. Race detector reports no new races in session package
5. DD-AUDIT-003 updated to v1.9 with 4 new session event types

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80%
3. Existing tests regress
4. Race detected in audit emission path

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- Build broken — code does not compile
- Circular import — audit package cannot be imported from session
- Pre-existing race failures obscure new races

**Resume testing when**:
- Build restored
- Import path resolved
- Pre-existing races documented and excluded

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/audit/emitter.go` | Event type constants, Action constants, `AllEventTypes` slice | ~15 new lines |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/session/manager.go` | `NewManager`, `StartInvestigation`, `CancelInvestigation`, goroutine completion/failure | ~40 new/changed lines |
| `internal/kubernautagent/server/handler.go` | `IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost` (metadata enrichment) | ~5 changed lines |
| `cmd/kubernautagent/main.go` | `NewManager` callsite wiring | ~2 changed lines |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `feature/pr1.5-session-audit-trail` HEAD | Branched from `feature/pr1-session-cancel-infra` |
| Dependency: PR1 cancellation infra | `fd15319b6` | Must be present (parent commit) |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AUDIT-005 v2.0 | Session started emits audit | P0 | Integration | IT-KA-823-A01 | Pending |
| BR-AUDIT-005 v2.0 | Session completed emits audit | P0 | Integration | IT-KA-823-A02 | Pending |
| BR-AUDIT-005 v2.0 | Session failed emits audit | P0 | Integration | IT-KA-823-A03 | Pending |
| BR-AUDIT-005 v2.0 | Session cancelled emits audit | P0 | Integration | IT-KA-823-A04 | Pending |
| BR-AUDIT-005 v2.0 | No phantom audit after cancel | P0 | Integration | IT-KA-823-A05 | Pending |
| SOC2 CC8.1 | Correlation fields present (remediation_id, session_id, initiated_by) | P0 | Integration | IT-KA-823-A06 | Pending |
| ADR-038 | Nil AuditStore guard (NopAuditStore default) | P0 | Integration | IT-KA-823-A07 | Pending |
| ADR-038 | Fire-and-forget on audit error | P0 | Integration | IT-KA-823-A08 | Pending |
| BR-AUDIT-005 v2.0 | Event types registered in AllEventTypes | P0 | Unit | UT-KA-823-A01 | Pending |
| BR-AUDIT-005 v2.0 | Action constants map to event types | P0 | Unit | UT-KA-823-A02 | Pending |
| BR-AUDIT-005 v2.0 | buildEventData returns untyped for session events | P1 | Unit | UT-KA-823-A03 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-KA-823-A{SEQUENCE}` (A prefix distinguishes audit scenarios from PR1 cancellation scenarios)

### Tier 1: Unit Tests

**Testable code scope**: `internal/kubernautagent/audit/emitter.go` — constant definitions and `AllEventTypes` registry.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-KA-823-A01 | SOC2 auditors can discover all 4 session event types via the AllEventTypes registry | Pending |
| UT-KA-823-A02 | Each session lifecycle transition has a dedicated action constant for structured querying | Pending |
| UT-KA-823-A03 | buildEventData gracefully falls through for session events (no panic, returns false) | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `internal/kubernautagent/session/manager.go` — audit emission at lifecycle transitions.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| IT-KA-823-A01 | When StartInvestigation is called, a `session.started` audit event is emitted with session_id and remediation_id | Pending |
| IT-KA-823-A02 | When an investigation completes successfully, a `session.completed` audit event is emitted | Pending |
| IT-KA-823-A03 | When an investigation fails, a `session.failed` audit event is emitted with the error message | Pending |
| IT-KA-823-A04 | When CancelInvestigation is called, a `session.cancelled` audit event is emitted | Pending |
| IT-KA-823-A05 | A cancelled investigation does NOT emit a session.completed or session.failed event (no phantom audit) | Pending |
| IT-KA-823-A06 | All audit events carry remediation_id as CorrelationID, session_id and initiated_by in event data, and EventCategory="aiagent" | Pending |
| IT-KA-823-A07 | NewManager with nil AuditStore defaults to NopAuditStore; StartInvestigation succeeds without panic | Pending |
| IT-KA-823-A08 | When AuditStore.StoreAudit returns an error, the investigation continues and completes successfully (fire-and-forget) | Pending |

### Tier Skip Rationale

- **E2E**: Not applicable for PR 1.5 — session audit emission is infrastructure-level; E2E coverage deferred to full v1.5 streaming integration.

---

## 9. Test Cases

### UT-KA-823-A01: Session event types registered in AllEventTypes

**BR**: BR-AUDIT-005 v2.0
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/audit/emitter_test.go`

**Test Steps**:
1. **Given**: The 4 session event type constants are defined
2. **When**: AllEventTypes is inspected
3. **Then**: Each of `aiagent.session.started`, `aiagent.session.cancelled`, `aiagent.session.completed`, `aiagent.session.failed` appears exactly once

**Acceptance Criteria**:
- All 4 session event types are in AllEventTypes
- No duplicates in AllEventTypes

### UT-KA-823-A02: Session action constants exist

**BR**: BR-AUDIT-005 v2.0
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/audit/emitter_test.go`

**Test Steps**:
1. **Given**: Action constants for session lifecycle transitions
2. **When**: Constants are referenced
3. **Then**: `ActionSessionStarted`, `ActionSessionCancelled`, `ActionSessionCompleted`, `ActionSessionFailed` are non-empty strings

### UT-KA-823-A03: buildEventData returns untyped for session events

**BR**: BR-AUDIT-005 v2.0
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/audit/ds_store_test.go`

**Test Steps**:
1. **Given**: An AuditEvent with EventType = `aiagent.session.started`
2. **When**: `buildEventData` is called
3. **Then**: Returns `(empty, false)` — no typed payload, graceful fallback

### IT-KA-823-A01: StartInvestigation emits session.started audit event

**BR**: BR-AUDIT-005 v2.0
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_audit_test.go`

**Preconditions**:
- Manager created with spy AuditStore
- Metadata includes `remediation_id`

**Test Steps**:
1. **Given**: A Manager with a spy AuditStore
2. **When**: `StartInvestigation` is called with metadata `{"remediation_id": "rr-123"}`
3. **Then**: Spy contains exactly 1 event with EventType=`aiagent.session.started`, CorrelationID=`rr-123`, Data["session_id"] is non-empty

### IT-KA-823-A02: Successful investigation emits session.completed

**BR**: BR-AUDIT-005 v2.0
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_audit_test.go`

**Preconditions**:
- Manager with spy AuditStore
- Investigation function returns success

**Test Steps**:
1. **Given**: An investigation that returns `("result", nil)`
2. **When**: The goroutine completes
3. **Then**: Spy contains a `session.completed` event with `EventOutcome="success"`

### IT-KA-823-A03: Failed investigation emits session.failed

**BR**: BR-AUDIT-005 v2.0
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_audit_test.go`

**Preconditions**:
- Manager with spy AuditStore
- Investigation function returns error

**Test Steps**:
1. **Given**: An investigation that returns `(nil, errors.New("timeout"))`
2. **When**: The goroutine completes
3. **Then**: Spy contains a `session.failed` event with `EventOutcome="failure"` and Data["error"]="timeout"

### IT-KA-823-A04: CancelInvestigation emits session.cancelled

**BR**: BR-AUDIT-005 v2.0
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_audit_test.go`

**Preconditions**:
- Manager with spy AuditStore
- Session is in StatusRunning

**Test Steps**:
1. **Given**: A running investigation
2. **When**: `CancelInvestigation(id)` is called
3. **Then**: Spy contains a `session.cancelled` event with `EventOutcome="success"` and matching session_id

### IT-KA-823-A05: Cancelled session does not emit phantom completed/failed

**BR**: BR-AUDIT-005 v2.0
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_audit_test.go`

**Preconditions**:
- Manager with spy AuditStore
- Session started, then cancelled before goroutine finishes

**Test Steps**:
1. **Given**: A running investigation blocked on a channel
2. **When**: `CancelInvestigation` is called, then the goroutine is released
3. **Then**: Spy contains `session.started` and `session.cancelled` events only; NO `session.completed` or `session.failed`

### IT-KA-823-A06: Audit events carry SOC2 correlation fields

**BR**: SOC2 CC8.1
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_audit_test.go`

**Test Steps**:
1. **Given**: Manager with spy, metadata=`{"remediation_id": "rr-456", "incident_id": "inc-789"}`
2. **When**: Investigation completes
3. **Then**: All emitted events have `CorrelationID="rr-456"`, `Data["session_id"]` matches the returned session ID, `EventCategory="aiagent"`

### IT-KA-823-A07: Nil AuditStore defaults to NopAuditStore

**BR**: ADR-038
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_audit_test.go`

**Test Steps**:
1. **Given**: `NewManager(store, logger, nil)` is called
2. **When**: `StartInvestigation` runs and investigation completes
3. **Then**: No panic; session reaches StatusCompleted normally

### IT-KA-823-A08: Audit store error does not abort investigation

**BR**: ADR-038
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/session/manager_audit_test.go`

**Preconditions**:
- Manager with a failing AuditStore (always returns error)

**Test Steps**:
1. **Given**: AuditStore that returns `errors.New("ds unavailable")`
2. **When**: Investigation completes successfully
3. **Then**: Session status is StatusCompleted (not StatusFailed); audit failure logged but investigation not aborted

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: None — testing constants and pure functions
- **Location**: `test/unit/kubernautagent/audit/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: `spyAuditStore` (implements `audit.AuditStore` interface, records events) — this is a test double for the AuditStore dependency, not a mock of the system under test
- **Infrastructure**: No external infra needed (in-process Manager + Store)
- **Location**: `test/integration/kubernautagent/session/`

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| PR1 cancellation infra (fd15319b6) | Code | Merged to feature branch | All tests blocked | N/A — parent commit |

### 11.2 Execution Order (TDD Phases)

1. **CHECKPOINT 0**: Clean-state verification (build, existing tests, git)
2. **Phase 1 (RED)**: Write all failing tests (UT-KA-823-A01..A03, IT-KA-823-A01..A08) with minimal stubs
3. **CHECKPOINT 1A**: Test plan quality gate
4. **CHECKPOINT 2A**: RED quality gate — verify tests fail for the right reasons
5. **Phase 2 (GREEN)**: Implement minimal code to pass all tests
6. **CHECKPOINT 3A**: GREEN quality gate — race detector, regression, adversarial audit
7. **Phase 3 (REFACTOR)**: GoDoc, logging, documentation (DD-AUDIT-003 v1.9)
8. **CHECKPOINT 4A**: Final gate — coverage, full audit, commit

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/823/TP-823-AUDIT.md` | Strategy and test design |
| Unit test suite | `test/unit/kubernautagent/audit/emitter_test.go` | Ginkgo BDD audit constant tests |
| Integration test suite | `test/integration/kubernautagent/session/manager_audit_test.go` | Ginkgo BDD audit emission tests |
| DD-AUDIT-003 v1.9 | `docs/architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md` | Updated event registry |

---

## 13. Execution

```bash
# Unit tests (audit constants)
go test ./test/unit/kubernautagent/audit/... -ginkgo.v

# Integration tests (manager audit emission)
go test ./test/integration/kubernautagent/session/... -ginkgo.v

# Focus on audit scenarios only
go test ./test/integration/kubernautagent/session/... -ginkgo.focus="Audit"

# Coverage (manager.go)
go test ./test/integration/kubernautagent/session/... -coverprofile=cover.out -coverpkg=github.com/jordigilh/kubernaut/internal/kubernautagent/session
go tool cover -func=cover.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/integration/kubernautagent/session/manager_test.go` (all) | `session.NewManager(store, slog.Default())` | `session.NewManager(store, slog.Default(), audit.NopAuditStore{})` | NewManager signature gains `auditStore` parameter |
| `test/unit/kubernautagent/session/metadata_test.go` (3 sites) | `session.NewManager(store, slog.Default())` | `session.NewManager(store, slog.Default(), nil)` | Nil is safe (F1 guard); minimal change to unrelated tests |
| `test/unit/kubernautagent/server/adversarial_http_test.go` | `session.NewManager(store, logger)` | `session.NewManager(store, logger, nil)` | Same |
| `test/unit/kubernautagent/server/response_mapper_test.go` | `session.NewManager(store, logger)` | `session.NewManager(store, logger, nil)` | Same |
| `cmd/kubernautagent/main.go:267` | `session.NewManager(store, slogger)` | `session.NewManager(store, slogger, auditStore)` | Production wiring |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-24 | Initial test plan for session lifecycle audit trail |
