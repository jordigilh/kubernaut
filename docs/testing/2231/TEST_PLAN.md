# Test Plan: KA AgentSession Dispatcher — Reconciler + Finalizer Migration

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2231-v1
**Feature**: Migrate KA's `agentsession.Dispatcher` from a raw `crclient.WithWatch` watch loop
to a controller-runtime `Reconciler` + `dispatchCleanupFinalizer`, closing the unreliable
`watch.Deleted` at-most-once delivery gap flagged in DD-AA-KA-001's 2026-08-21 cross-check note.
**Version**: 1.0
**Created**: 2026-08-22
**Status**: Active
**Branch**: `fix/2231-ka-agentsession-reconciler`

---

## 1. Introduction

### 1.1 Purpose

Issue [#2231](https://github.com/jordigilh/kubernaut/issues/2231) tracks a confirmed-live
reliability gap in KA's `Dispatcher`: a raw `watch.Deleted` event for `AgentSession` is
delivered at-most-once, outside controller-runtime's workqueue retry machinery, and can be
silently dropped under load (proven for AF's structurally-identical prior design, DD-AA-KA-001's
"Post-merge correction" amendment, PR #2222). A dropped delete leaves KA investigating a
remediation nothing will ever read the result of, burning LLM/tool budget for up to
`session.Store`'s 60-minute `maxSessionAge` backstop window. This plan covers the tests proving
the fix: adopting the same `Manager` + `Reconciler` + finalizer pattern already proven for AF's
`AgentSessionTerminalCloseReconciler` (DD-AA-KA-001 Amendment, #2231).

### 1.2 Objectives

1. **Delete-reliability**: an `AgentSession` deletion is observed and its in-memory
   investigation goroutine stopped via the same workqueue-backed, retried `Reconcile` path used
   for every other `AgentSession` event — not a raw, droppable watch event.
2. **Zero behavioral regression**: every existing dispatch, Lease-race, capacity-rejection,
   interactive-detection, timeout-enforcement, and terminal-status-write scenario
   (`dispatcher_test.go`'s 15 pre-existing scenarios) continues to pass unchanged in outcome.
3. **Wiring proof**: the new `Reconciler` is registered against a real `ctrl.Manager` started
   from `cmd/kubernautagent`, provable end-to-end against a real API server (envtest), not just
   a fake client.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./internal/kubernautagent/agentsession/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/...` |
| Backward compatibility | 0 regressions | All 15 pre-existing `dispatcher_test.go` scenarios pass after mechanical migration to direct `Reconcile()` calls |
| Wiring | 1/1 Wiring Manifest rows proven | See §14 |

---

## 2. References

### 2.1 Authority

- [DD-AA-KA-001](../../architecture/decisions/DD-AA-KA-001-agentsession-crd-http-removal.md), Amendment: #2231 (2026-08-22) — the design decision this plan implements
- [BR-AA-KA-065](../../requirements/BR-AA-KA-065-agentsession-watch-design.md) — AgentSession watch/dispatch design
- Issue [#2231](https://github.com/jordigilh/kubernaut/issues/2231)

### 2.2 Cross-References

- `internal/controller/apifrontend/agentsession_close.go` / `agentsession_close_test.go` — the proven Reconciler+finalizer+direct-`Reconcile()`-UT pattern this plan mirrors
- `cmd/apifrontend/session_infra.go` — the proven `ctrl.Manager` construction template this plan mirrors
- `test/integration/apifrontend/agentsessionclose/agentsession_close_wiring_test.go` — the proven envtest bootstrap template this plan mirrors

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `Reconcile` blocks on `tryDispatch`/`cancelOnTimeout` synchronously, serializing dispatch behind controller-runtime's default single worker | Dispatch throughput regression under real fleet load | Medium (easy to get wrong mechanically) | UT-AA-2231-003, all migrated dispatch-outcome tests | `Reconcile` launches both via `go` exactly as `considerAgentSession` always did; UT-AA-2231-003 asserts `Reconcile` returns before a slow investigation completes |
| R2 | Finalizer add/remove races a concurrent Status write (dispatch-Lease renewal, terminal-status write) on the same `resourceVersion` | Spurious `Conflict` errors, or a lost Status update | Low (existing `updateStatus` already retries on Conflict) | UT-AA-2231-002, IT-AA-2231-001 | `Reconcile`'s finalizer `Update` and `updateStatus`'s retry-on-Conflict loop both re-`Get` before writing; no shared mutation path |
| R3 | Test-double fake client's finalizer/deletion semantics diverge from a real API server's | UT passes, real cluster behaves differently | Medium | IT-AA-2231-001/002 (envtest, real API server) | envtest IT is the authoritative proof; UT is fast feedback only |
| R4 | Migrated tests silently lose coverage of a scenario during the `go d.Start(ctx)` → direct-`Reconcile()` mechanical conversion | Regression escapes to production undetected | Low (15 known scenarios, each ported 1:1) | All 15 migrated scenarios in `dispatcher_test.go` | §15 tracks each scenario's required change explicitly; none deleted, only the trigger mechanism changes |

### 3.1 Risk-to-Test Traceability

R1 → UT-AA-2231-003. R2 → UT-AA-2231-002, IT-AA-2231-001. R3 → IT-AA-2231-001, IT-AA-2231-002.
R4 → §15 (exhaustive per-scenario migration table).

---

## 4. Scope

### 4.1 Features to be Tested

- **`Dispatcher.Reconcile`/`reconcileDelete`/`SetupWithManager`** (`internal/kubernautagent/agentsession/dispatcher.go`): the new controller-runtime entry point, finalizer lifecycle, and Manager registration.
- **`cmd/kubernautagent/agentsession_wiring.go`**: the new `ctrl.Manager` construction and `mgr.Start(ctx)` wiring, replacing `go dispatcher.Start(ctx)`.
- **RBAC** (`charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml`, `test/infrastructure/kubernautagent.go`): the `update`/`patch` grant on `agentsessions` the finalizer requires.

### 4.2 Features Not to be Tested

- **`session.Manager`'s internal investigation lifecycle** (start/complete/cancel semantics): unchanged by this fix, already covered by `internal/kubernautagent/session/*_test.go`.
- **AF's `AgentSessionTerminalCloseReconciler`**: unrelated, already-merged prior work (#2214) — referenced only as a design precedent.
- **E2E**: no new SOC2/FedRAMP control objective is introduced (this fix strengthens an existing control's implementation reliability, AU-2/CC7.2, rather than adding a new one) and the existing `E2E-AA-065`/`E2E-AA-2170` suites already exercise the full `AgentSession` lifecycle including deletion end-to-end; no new E2E scenario is warranted for a delivery-reliability fix that IT-tier envtest (a real API server) already proves.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Direct-`Reconcile()`-call UT pattern (no fake watch/informer in unit tests) | Matches AF's proven `agentsession_close_test.go` convention; deterministic, no `Eventually`-on-background-watch flakiness for the trigger mechanism itself (dispatch's own async completion still needs `Eventually`, unchanged) |
| Keep `Dispatcher.client` as the same raw, uncached `crclient.WithWatch` for all API reads/writes; Manager's cache used only to drive `Reconcile` dispatch | Preserves existing dispatch-Lease race consistency semantics unchanged — see DD-AA-KA-001 Amendment #2231 |
| No `+kubebuilder:rbac` marker added | `make manifests`'s `controller-gen` only scans `./internal/controller/...`, not `internal/kubernautagent/...` — consistent with KA's pre-existing hand-maintained RBAC pattern |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: 100% of `Reconcile`/`reconcileDelete`/finalizer logic (pure control flow over a fake client — no real I/O)
- **Integration**: 100% of the Wiring Manifest row (§14) — a real envtest API server proving delete-driven `Reconcile` actually stops the in-memory investigation

### 5.2 Two-Tier Minimum

Both new scenarios (finalizer add-on-create, delete stops investigation) are covered by UT (fake client, fast) and IT (envtest, real API server, proves wiring) — see BR Coverage Matrix (§7).

### 5.3 Business Outcome Quality Bar

Every test asserts an observable `AgentSession.Status` transition or in-memory session-state
outcome (`session.Manager.GetSession`), never an internal call count alone.

### 5.4 Pass/Fail Criteria

**PASS**: all UT-AA-2231-* and IT-AA-2231-* tests pass; all 15 pre-existing `dispatcher_test.go`
scenarios pass unchanged in outcome after migration; `go build ./...` and
`golangci-lint run --timeout=5m` are clean.

**FAIL**: any P0 test fails, any pre-existing scenario's outcome changes, or the finalizer is
observed to block/delay a genuine `AgentSession` deletion beyond one `Reconcile` cycle.

### 5.5 Suspension & Resumption Criteria

Suspend if `KUBEBUILDER_ASSETS` is unavailable for envtest (`make setup-envtest` not run) —
resume once set. No other blocking dependency identified.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/agentsession/dispatcher.go` | `Reconcile`, `reconcileDelete`, `considerAgentSession` | ~90 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/agentsession/dispatcher.go` | `SetupWithManager` | ~15 |
| `cmd/kubernautagent/agentsession_wiring.go` | `newAgentSessionManager`, `startAgentSessionDispatcher` | ~40 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/2231-ka-agentsession-reconciler` HEAD | Branched from `origin/main` |
| controller-runtime | v0.24.1 (`go.mod`) | Confirmed supports `Reconciler`/`Manager`/`controllerutil` APIs used |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AA-KA-065.3 | KA dispatches exactly once per AgentSession via the dispatch Lease | P0 | Unit | UT-AA-KA-065-020..026 (migrated, unchanged outcome) | Pass |
| BR-AA-KA-065.9 | KA is the sole writer of AgentSession.Status | P0 | Unit | UT-AA-2231-002 | Pass |
| BR-AA-KA-065 (delete-reliability, #2231) | An AgentSession deletion reliably stops the in-memory investigation via the workqueue-retried Reconcile path, not an at-most-once raw watch event | P0 | Unit | UT-AA-2231-001, UT-AA-2231-004 | Pass |
| BR-AA-KA-065 (delete-reliability, #2231) | Same, proven against a real API server | P0 | Integration | IT-AA-2231-001 | Pass |
| BR-AA-KA-065 (concurrency, #2231) | Reconcile does not serialize dispatch attempts behind a single worker | P1 | Unit | UT-AA-2231-003 | Pass |
| BR-AA-KA-065 (negative control, #2231) | A not-found AgentSession (already fully deleted) is a benign Reconcile no-op | P1 | Unit | UT-AA-2231-005 | Pass |
| BR-AA-KA-065 (wiring, #2231) | The Reconciler is actually registered and dispatched to from `cmd/kubernautagent` production wiring | P0 | Integration | IT-AA-2231-002 | Pass |

---

## 8. Test Scenarios

### Test ID Naming Convention

`{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}` (SERVICE=AA for this KA-internal-package plan, matching
this codebase's existing `dispatcher_test.go` convention of using `AA` for `AgentSession`-scoped
tests regardless of which service package they live in).

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `UT-AA-2231-001` | `Reconcile` on a `DeletionTimestamp`-set AgentSession stops the in-memory investigation and removes the finalizer | Pass |
| `UT-AA-2231-002` | `Reconcile` on a fresh (no-finalizer) AgentSession adds `dispatchCleanupFinalizer` before attempting dispatch | Pass |
| `UT-AA-2231-003` | `Reconcile` returns promptly (does not block on a slow investigation) — dispatch happens via goroutine | Pass |
| `UT-AA-2231-004` | An AgentSession without the finalizer (pre-upgrade transitional gap) falls through `reconcileDelete` untouched, no panic, no error | Pass |
| `UT-AA-2231-005` | `Reconcile` on a not-found (fully deleted) AgentSession is a benign no-op | Pass |

### Tier 2: Integration Tests

**Testable code scope**: `SetupWithManager`, real `ctrl.Manager` wiring, real envtest API server.

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `IT-AA-2231-001` | Deleting a dispatched AgentSession against a real API server reliably stops the in-memory investigation via the finalizer-driven Reconcile path | Pass |
| `IT-AA-2231-002` | The Dispatcher Reconciler, registered via `SetupWithManager` on a real Manager, actually dispatches a freshly-Created AgentSession (wiring proof, not just unit-level `Reconcile()` calls) | Pass |

### Tier Skip Rationale

- **E2E**: no new control objective introduced; see §4.2.

---

## 9. Test Cases (P0 detail)

### UT-AA-2231-001: Delete stops in-memory investigation via finalizer

**BR**: BR-AA-KA-065 (delete-reliability, #2231)
**Priority**: P0 | **Type**: Unit
**File**: `internal/kubernautagent/agentsession/dispatcher_test.go`

**Test Steps**:
1. **Given**: a Dispatcher and a dispatched, `Investigating` AgentSession carrying `dispatchCleanupFinalizer`, with a real in-memory investigation running in `session.Manager`.
2. **When**: `Reconcile` is called with the object's `DeletionTimestamp` set.
3. **Then**: the in-memory session transitions to `StatusCancelled`, the finalizer is removed, and no error is returned.

### IT-AA-2231-001: Real-API-server delete reliability

**BR**: BR-AA-KA-065 (delete-reliability, #2231)
**Priority**: P0 | **Type**: Integration
**File**: `test/integration/kubernautagent/agentsession_dispatcher_test.go`

**Preconditions**: envtest environment running (`KUBEBUILDER_ASSETS` set), a real `ctrl.Manager` with the Dispatcher registered via `SetupWithManager`.

**Test Steps**:
1. **Given**: a Create'd AgentSession dispatched by the running Manager (Investigating).
2. **When**: the AgentSession is deleted against the real envtest API server.
3. **Then**: the in-memory investigation is cancelled and the AgentSession is fully removed (finalizer released) within the test's `Eventually` window.

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: `fakeInvestigationRunner` (test double for the LLM-bound `InvestigationRunner` interface — the only external dependency); `sigs.k8s.io/controller-runtime/pkg/client/fake` for the K8s API
- **Location**: `internal/kubernautagent/agentsession/dispatcher_test.go`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: ZERO — real envtest API server, real `ctrl.Manager`
- **Infrastructure**: `envtest.Environment` (`KUBEBUILDER_ASSETS`, `make setup-envtest`)
- **Location**: `test/integration/kubernautagent/agentsession_dispatcher_test.go`

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | per `go.mod` | Build and test |
| controller-runtime | v0.24.1 | Manager/Reconciler/envtest |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None. All referenced patterns (AF's `AgentSessionTerminalCloseReconciler`, `session_infra.go`, `agentsession_close_wiring_test.go`) already exist and are merged on `main`.

### 11.2 Execution Order

1. **Phase 1 (RED)**: migrate `dispatcher_test.go`'s 15 existing scenarios to direct-`Reconcile()` calls (mechanical, no new assertions) → confirm they now fail to compile/run against the not-yet-existing `Reconcile` method; write UT-AA-2231-001..005 (failing); add envtest bootstrap and IT-AA-2231-001/002 (failing).
2. **Phase 2 (GREEN)**: implement `Reconcile`/`reconcileDelete`/`SetupWithManager`/`dispatchCleanupFinalizer`; wire `cmd/kubernautagent/agentsession_wiring.go`'s new `ctrl.Manager`; add RBAC `update`/`patch` grants.
3. **Phase 3 (REFACTOR)**: remove dead `Start`/`watchLoop`/`handleEvent`/`resync`/`listOpts` code; tighten comments.
4. **Phase 4 (WIRING VERIFICATION)**: IT-AA-2231-002 proves `cmd/kubernautagent`'s production wiring.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/testing/2231/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `internal/kubernautagent/agentsession/dispatcher_test.go` | Migrated + new Ginkgo BDD tests |
| Integration test suite | `test/integration/kubernautagent/agentsession_dispatcher_test.go` | New Ginkgo BDD envtest suite |

---

## 13. Execution

```bash
# Unit tests
go test ./internal/kubernautagent/agentsession/... -ginkgo.v

# Integration tests (requires: make setup-envtest)
go test ./test/integration/kubernautagent/... -ginkgo.v

# Specific test by ID
go test ./internal/kubernautagent/agentsession/... -args -ginkgo.focus="UT-AA-2231"
```

---

## 14. Wiring Verification (TDD Phase 4)

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|------------------------|------------------------|------------|
| `Dispatcher` (as `reconcile.Reconciler`) | KA process startup (`main.go`'s `startAgentSessionDispatcher` call) | `cmd/kubernautagent/agentsession_wiring.go`: `newAgentSessionManager` + `dispatcher.SetupWithManager(mgr)` + `go mgr.Start(ctx)` | IT-AA-2231-002 |

---

## 15. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|---------------------|--------------------|-------------------|--------|
| All 15 `Describe` blocks in `dispatcher_test.go` (UT-AA-KA-065-020..026, IT-AA-2170-DISPATCH-LEASE-NAME, IT-AA-2170-DELETE-001, IT-AA-2170-TIMEOUT-001/002, UT-INTERACTIVE-010-030, UT-AA-KA-065-024, IT-AA-KA-2233-001) | `go d.Start(ctx)` launches the background watch loop; `Eventually` polls the fake client for the resulting Status | Replace `go d.Start(ctx)` with an explicit `mustReconcile(d, as)` call immediately after each `Create`/mutating `Update`/`Delete` that previously relied on a watch event; `Eventually` polling for the resulting async Status write is unchanged (dispatch itself remains goroutine-driven) | `Start`/`watchLoop` are retired (DD-AA-KA-001 Amendment #2231); the test must now drive `Reconcile` explicitly at each point a watch event previously fired it |
| `UT-AA-2170-DELETE-001` specifically | Deletes the AgentSession and waits for cancellation via the watch's `Deleted` handler | Add a second `mustReconcile(d, as)` call after `cli.Delete` to drive `reconcileDelete` (deletion now defers via `dispatchCleanupFinalizer` until `Reconcile` observes `DeletionTimestamp`) | Finalizer semantics: the fake client's tracker sets `DeletionTimestamp` (not actual removal) on `Delete` while a finalizer is present, matching real API server behavior |

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-08-22 | Initial test plan |
