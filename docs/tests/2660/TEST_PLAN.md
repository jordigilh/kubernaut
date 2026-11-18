# Test Plan: Deferred Minor/Nit Follow-Ups from #2593

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2660-v1.0
**Feature**: Staged-repair expiry, failure-code fidelity, deferred abandon context, readiness error lanes, dead-lane removal
**Version**: 1.2
**Created**: 2026-11-18
**Author**: Kubernaut Team
**Status**: Active
**Branch**: `work/2660-fmc-followup`

---

## 1. Introduction

### 1.1 Purpose

This plan covers the deferred Minor/Nit findings from the #2593 review (issue
#2660). It is a bounded polish series, not a redesign: the Lua staged-repair
protocol gains an explicit terminal-expiry transition, the FMC syncer stops
collapsing typed repair/commit failures into `source_incomplete`, the
begin-failure abandon survives request cancellation, the scope-cache client
keeps invalid readiness proofs distinguishable from unavailable partitions, and
two zero-caller boolean scope lanes (`IsManagedResource`, `ScopeResult.Managed`)
plus a dead `local` field are removed.

### 1.2 Objectives

1. **Terminal staged expiry**: a reconcile past the 60s staged lifetime cap
   fails typed as `stale_fence`, writes no staged orphans, and the next
   `BeginRepair` recovers in one cycle.
2. **Commit-lane fidelity**: a commit against a gone/expired staging reports
   `Failure.Code == stale_fence` and unwraps to `ErrStaleFence`.
3. **Syncer fidelity**: `MarkRepairUncertain` receives the real typed failure
   code (`count_mismatch`, `stale_fence`, `stale_registration`) and is not
   called at all after a clean commit whose readiness publish later fails.
4. **Deferred abandon**: the begin-failure abandon runs on a detached
   (`context.WithoutCancel`) 5s context.
5. **Readiness lanes**: `ReadinessInvalid` keeps its error and yields
   `partition_invalid`; `ReadinessUnavailable` stays a nil-error
   `partition_unavailable` result.
6. **Behavior-change record**: cluster-scoped targets stay `ScopeUnsupported`;
   the boolean scope lanes are gone with no wire change.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `make test-unit-fleetmetadatacache`, `make test-unit-shared-packages` |
| Integration test pass rate | 100% | `make test-integration-fleetmetadatacache` (envtest 1.37) |
| Build / lint | 0 errors, 0 new issues | `go build ./...`, `make lint` |
| Test-pattern gates | 0 violations | `make lint-test-patterns lint-business-integration lint-tdd-compliance` |
| Regressions | 0 | Existing suites pass unmodified |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-SCOPE-001: Resource Scope Management (FR-SCOPE-004 deferred target boundary)
- BR-INTEGRATION-065: Multi-cluster federation scope resolution
- BR-ORCH-025: RemediationRequest fixture factories
- BR-SECURITY-016: Fenced-read proof integrity
- DD-FLEET-015: Typed scope states (only state and provenance cross the wire)
- DD-FLEET-016 A2: Staged repair with atomic commit swap
- Issue #2660: Deferred Minor/Nit review findings from #2593 (PR #2637)
- `docs/tests/2593/TEST_PLAN.md`: parent protocol and remote-qualification protocol

### 2.2 Cross-References

- `pkg/fleet/scopecache/scripts/fmc_v2_atomic.lua`
- `pkg/fleet/scopecache/status_store.go`, `pkg/fleet/scopecache/client.go`
- `pkg/fleet/fmc/syncer.go`, `pkg/fleet/registry/types.go`
- `pkg/shared/scope/{checker,manager}.go`, `pkg/fleet/federated_checker.go`
- `test/shared/helpers/remediation.go`

---

## 3. Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Deleting `ScopeResult.Managed` changes a wire member | No wire change: the FMC strict decoder already admits only `state` + `provenance`; the field had no reader |
| Deleting `IsManagedResource` removes a lane used outside this repo | `grep` confirms zero production callers; only unit tests used it and they now assert the typed lanes |
| Lua change ships in the FMC image without remote AF re-qualification | Explicitly **deferred** here (see §4.2); flagged in the PR body |
| Detached abandon hides a real cancellation bug | The abandon is best-effort cleanup only; the begin error still propagates |

### 3.1 Risk-to-Test Traceability

- Lua expiry: UT-FMC-STAGED-011, UT-FMC-STAGED-013
- Syncer fidelity: UT-FMC-PRES-013, -014, -015, -016
- Readiness lanes: UT-FMC-DEFERRED-002
- Dead-lane removal: existing UT-SCOPE-* scenarios re-expressed on typed lanes

---

## 4. Scope

### 4.1 Features to be Tested

- Staged lifetime cap transition and `mark_uncertain` acceptance of the
  `expired` marker.
- Commit-lane error classification before failure derivation.
- Syncer repair/commit failure-code threading and post-commit publish handling.
- Begin-failure abandon context detachment.
- Scope-cache client readiness error discrimination (both `PartitionReady` sites).
- Fixture helper failing loudly on unsupported kinds.

### 4.2 Features Not to be Tested

- **Remote AF re-qualification of the embedded Lua script: DEFERRED.** The
  protocol in `docs/tests/2593/TEST_PLAN.md` applies (live kind cluster
  `af-fleet-2593-rca` on `helios08`, image `2593-a2-1fb6d74c0`). It is flagged
  in the PR body and is not run in this branch.
- New registry scenarios for the `addCluster` refactor (behavior-preserving;
  existing registry tests are reused per the user's condition).
- Any broader GA certification.

### 4.3 Design Decisions

- **`STAGED_EXPIRED` folds into `ErrStaleFence`.** No new Go sentinel and no
  new failure enum member: a terminal staged expiry means the writer must begin
  a new repair, exactly like a stale fence. The Lua guard also transitions the
  staged status to a TTL-bounded `state=expired` marker so `begin` reclaims it
  in one cycle (its gate blocks only `state=repairing`), and `mark_uncertain`
  accepts that marker so the syncer defer does not spurious-ERROR.
- **Repair-lane classification shift.** Post-write `INVALID_PTTL` /
  `TTL_MISMATCH` replies from `repair_reconcile` are genuinely ambiguous (the
  staged writes may already have landed): they now map to `ErrAmbiguousWrite`
  (failure code `ambiguous_write`) instead of the previous message-derived
  `ttl_invalid`. No test pinned the old value.
- **Malformed replies stay `ttl_invalid`.** A script reply that is not
  parseable as a reconcile proof (`parseRepairReconcile` failure) is a protocol
  defect in a successful reply, not an ambiguous write; it keeps
  `RepairFailureTTLInvalid`.
- **`currentRegistrationGuard` failures stay `source_incomplete`.** That path
  runs before a repair staging exists (no start token), so there is nothing to
  classify against a repair failure code.
- **Site-2 `ReadinessInvalid` is defensive.** Site 1 validates identity before
  the callback; an invalid proof inside the callback can only mean a corrupted
  snapshot.

---

## 5. Approach

### 5.1 Coverage Policy

Requirements-based (ISO/IEC/IEEE 29119-4): every in-scope finding maps to a
named scenario. Structural coverage is diagnostic only.

### 5.2 Two-Tier Minimum

- Unit: Ginkgo/Gomega in `pkg/fleet/scopecache`, `pkg/fleet/fmc`, `pkg/shared/scope`, `test/shared/helpers`.
- Integration: existing FMC envtest suite must stay green (no new IT is needed:
  the changed wiring is exercised through the unit-level fakes and the existing
  IT suite is the regression gate).

### 5.3 Pass/Fail Criteria

All scenarios below GREEN; build/lint/test-pattern gates clean; zero regressions.

### 5.4 Suspension & Resumption Criteria

Suspend if a RED failure is caused by a pre-existing defect outside the issue's
scope; record it separately and resume only after the user decides.

---

## 6. Test Items

### 6.1 Unit-Testable Code

`pkg/fleet/scopecache/{status_store,client}.go`, `pkg/fleet/fmc/syncer.go`,
`pkg/fleet/{registry,federated_checker}`, `pkg/shared/scope`, `test/shared/helpers`.

### 6.2 Integration-Testable Code

FMC sync/repair lifecycle through the real Valkey-backed store (existing IT).

### 6.3 Version Identification

Branch `work/2660-fmc-followup` based on `0e086e065` (origin/main).

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-SCOPE-001 | Terminal staged expiry fails typed and does not strand orphans | P0 | Unit | UT-FMC-STAGED-011 | Pass |
| BR-SCOPE-001 | Commit against gone staging keeps stale-fence fidelity | P0 | Unit | UT-FMC-STAGED-013 | Pass |
| BR-SCOPE-001 | Empty-partition fence/ttl split keeps failure fidelity | P0 | Unit | UT-FMC-STAGED-014 | Pass |
| BR-SCOPE-001 | Reserved proof slots stay unenforced (regression guard) | P1 | Unit | UT-FMC-STAGED-012 | Pass |
| BR-SCOPE-001 | Repair failure code reaches MarkRepairUncertain | P0 | Unit | UT-FMC-PRES-013 | Pass |
| BR-SCOPE-001 | Commit/registration failure codes reach MarkRepairUncertain | P0 | Unit | UT-FMC-PRES-014 | Pass |
| BR-SCOPE-001 | Clean commit is not re-marked uncertain on publish failure | P0 | Unit | UT-FMC-PRES-015 | Pass |
| BR-SCOPE-001 | Begin-failure abandon survives request cancellation | P1 | Unit | UT-FMC-PRES-016 | Pass |
| BR-SCOPE-001 | Invalid readiness proof is not flattened to unavailable | P0 | Unit | UT-FMC-DEFERRED-002 | Pass |
| BR-ORCH-025 | Fixture helper fails loudly on unsupported kinds | P1 | Unit | UT-HELPERS-2660-001..003 | Pass |
| BR-SCOPE-001 | Typed lanes replace boolean lanes (existing scenarios) | P1 | Unit | UT-SCOPE-001-* / RI-00* / 2592-LABEL-* | Pass |

Status legend: Pending / RED / GREEN / REFACTORED / Pass.

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-FMC-STAGED-011` | A reconcile 61s past `created_at` fails with `RepairFailureError.Code == stale_fence`, writes no new staged keys (set equality), and an immediate `BeginRepair` succeeds (the TTL-bounded `expired` marker reopens begin's gate) | Pass |
| `UT-FMC-STAGED-012` | Direct EVALSHA of the production script, two cases: (a) a non-empty commit with `ARGV[17]='0'` (record-TTL proof) is rejected `COMMIT_REJECTED`; (b) a commit with the reserved structural slot `ARGV[13]='0'` succeeds — the m1 guarantee that repair_reconcile success IS the structural proof (re-instating the deleted pre-validation loop would fail this case) | Pass |
| `UT-FMC-STAGED-013` | `CommitRepair` against a deleted/expired staging returns `Failure.Code == stale_fence` and `errors.Is(err, scopecache.ErrStaleFence)` | Pass |
| `UT-FMC-STAGED-014` | Empty-partition finalization: a gone staging reports `stale_fence` (was `commit_rejected`); a corrupted `status_expires_at_ms` reports `ttl_invalid` and does not unwrap to `ErrStaleFence` (pre-write rejection) | Pass |
| `UT-FMC-PRES-013` | `Repair` failing with `count_mismatch` makes `MarkRepairUncertain` receive `count_mismatch` | Pass |
| `UT-FMC-PRES-014` | (a) typed commit `Failure.Code == stale_fence` and (b) guard rejection via `registry.ErrRegistrationChanged` both reach `MarkRepairUncertain` as `stale_fence` / `stale_registration` | Pass |
| `UT-FMC-PRES-015` | A clean commit followed by a failed clean-partition publish does **not** call `MarkRepairUncertain` | Pass |
| `UT-FMC-PRES-016` | Cancelling the request context inside `BeginRepair` still gives `AbandonStaging` a non-cancelled context | Pass |
| `UT-FMC-DEFERRED-002` | Site 1: `ReadinessInvalid` returns an error + `State unavailable` + `Reason partition_invalid`. Site 2: a rotated readiness view inside `WithCurrent` yields `partition_unavailable` with a nil error | Pass |
| `UT-HELPERS-2660-001` | `ensureTargetFixture` returns a descriptive non-nil error naming an unsupported kind | Pass |

Notes:

- `UT-FMC-PRES-012` is intentionally unused (reserved during planning; no
  scenario was assigned to it).
- `STAGED_EXPIRED` guards only the "staged status survived past the cap" case.
  On a compliant TTL-honoring server the fence check reads the key first and
  reports `STALE_FENCE`. The `expired` marker is TTL-bounded at cap + 15s and
  `mark_uncertain` accepts it.
- m1/m6/m7/m8/m9/n1–n3/n5 are **RED: N/A** — they delete unreachable code,
  dead fields, or zero-caller lanes, or are behavior-preserving refactors. Their
  safety net is the existing suite.

### Tier 2: Integration Tests

Existing `make test-integration-fleetmetadatacache` remains the regression
gate; no new IT wiring point is introduced by this series.

### Tier Skip Rationale

No E2E: nothing crosses a new service boundary, and the Lua ensures change is
gated on the deferred remote re-qualification described in §4.2.

---

## 9. Test Cases

### UT-FMC-STAGED-011: past-cap reconcile is a terminal staged expiry

- **Given** a staging with one reconciled observation at `t0`
- **When** the server clock is set to `t0+61s` (no FastForward, so miniredis
  keeps the keys) and `Repair` runs
- **Then** it fails with `RepairFailureError`, `Failure.Code ==
  RepairFailureStaleFence`; server key set is unchanged; an immediate
  `BeginRepair` succeeds.

Expected RED: `failure.Failure.Code` is `ttl_invalid` (today the guard is
absent, so the post-write `INVALID_PTTL` reply is message-derived as
`ttl_invalid`) — assertion `Expected <stale_fence> to equal <ttl_invalid>`.

### UT-FMC-STAGED-013: commit-lane stale-fence fidelity

- **Given** a begun + reconciled staging
- **When** the staged keys are deleted (expired) and `CommitRepair` runs
- **Then** `Failure.Code == stale_fence` and `errors.Is(err, ErrStaleFence)`.

Expected RED: `CommitRepair` derives `Failure` from the raw server error, so the
code is `commit_rejected`, and `errors.Is` is false.

### UT-FMC-STAGED-014: empty-partition fence/ttl fidelity (review follow-up)

- **Given** a begun staging with **zero** observations whose staged status key
  has been reaped (FastForward past the 15s staged TTL)
- **When** `Repair` runs the empty-partition finalization
- **Then** it fails with `Failure.Code == RepairFailureStaleFence` (the raw
  `STALE_FENCE` reply previously fell through to `commit_rejected`).
- **Given** a second begun staging whose `status_expires_at_ms` is corrupted
- **When** `Repair` runs the empty-partition finalization
- **Then** it fails with `Failure.Code == RepairFailureTTLInvalid` and does
  NOT unwrap to `ErrStaleFence` (a pre-write TTL mismatch is not a fence loss).

Expected RED: `Expected <stale_fence> to equal <commit_rejected>`; the
TTL_MISMATCH case pins the deliberate fence/ttl split.

### UT-FMC-PRES-013: repair failure code fidelity

Expected RED: `MarkRepairUncertain` receives `source_incomplete` instead of
`count_mismatch`.

### UT-FMC-PRES-014: commit failure code fidelity

Expected RED: both sub-cases reach `MarkRepairUncertain` as `source_incomplete`.
Sub-case (a) must fake a typed `Failure` because the real store always populates
it; a bare error would legitimately be classified `lease_lost`.

### UT-FMC-PRES-015: no uncertain marking after a clean commit

Expected RED: `MarkRepairUncertain` is called after the publish guard fails.

### UT-FMC-PRES-016: detached abandon context

Expected RED: the abandon records a cancelled `ctx.Err()`.

### UT-FMC-DEFERRED-002: readiness error lanes

Expected RED: `Check` returns nil error + `partition_unavailable` for an invalid
proof (today every readiness error is flattened).

### UT-HELPERS-2660-001: helper fails loudly

Expected RED: `ensureTargetFixture` does not exist yet; the test does not
compile until the helper is extracted (the helper's error message names the
unsupported kind).

---

## 10. Environmental Needs

### 10.1 Unit Tests

Go toolchain, Ginkgo/Gomega, `miniredis` for the Lua/store fixtures.

### 10.2 Integration Tests

envtest 1.37 (`~/go/bin/setup-envtest use 1.37 -p path`) and a local Valkey
per the existing FMC IT harness.

### 10.3 Tools & Versions

`go build`, `golangci-lint --timeout=5m`, project make targets.

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

`docs/tests/2593/TEST_PLAN.md` protocol for the deferred remote qualification;
origin/main `0e086e065` as the rebase base.

### 11.2 Execution Order

docs → m2 (+n2's Lua comment fixes shipped in the m2 commit) → m1 → m3 → m4 → m5 → m6+n1 → m7 → m8 → m9+n3 → m10 → n4 → n5.

---

## 12. Test Deliverables

Commit log (one commit per finding with RED evidence in the body), this plan,
and the validation run output reported to the user.

---

## 13. Execution

```bash
go build ./...
make test-unit-fleetmetadatacache
make test-unit-shared-packages
make test-integration-fleetmetadatacache
make lint
make lint-test-patterns lint-business-integration lint-tdd-compliance
```

---

## 14. Wiring Verification

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|-----------|
| Lua `STAGED_EXPIRED` guard | `ValkeyStatusStore.Repair` | `pkg/fleet/scopecache/status_store.go` → `repairObservation` | existing FMC IT suite |
| Syncer failure threading | `Syncer.repairPartition` | `pkg/fleet/fmc/syncer.go` | existing FMC IT suite |
| Readiness discrimination | `scopecache.Client.Check` | `pkg/fleet/scopecache/client.go` → `checkFencedStatus` | existing FMC IT suite |
| `registry.ErrRegistrationChanged` | both registry `WithCurrent` guards | `kuadrant_registry.go`, `eaigw_registry.go` | existing FMC IT suite |

---

## 15. Existing Tests Requiring Updates

- `pkg/shared/scope/manager_test.go`: 26 `IsManagedResource` call sites move to
  `Resolve` / `Check` lanes (the three Node scenarios were later moved from
  `ResolveWithResourceLabels` to `Resolve` by the review follow-up, so no test
  uses the pre-fetched-labels lane; it keeps its production caller in
  `pkg/apifrontend/policyread/local_inventory.go`).
- `pkg/fleet/federated_checker_test.go`: `UT-FLEET-2592-GVK-001` and the
  `NewFederatedScopeChecker` call sites; `Managed` assertions deleted.
- `pkg/fleet/fmc/syncer_red_test.go`: fake guard returns the sentinel;
  `lifecycleStatusStore` gains fault injection and failure recording.

---

## 16. Changelog

| Version | Date | Change |
|---------|------|--------|
| 1.0 | 2026-11-18 | Initial plan for #2660 deferred findings |
| 1.1 | 2026-11-18 | Implementation complete: matrix and scenarios marked Pass; n4 gained the helper's positive Pod/Deployment and cluster-scoped no-op scenarios |
| 1.2 | 2026-11-18 | Post-implementation review follow-up: UT-FMC-STAGED-014 (empty-partition stale-fence fidelity, RED: `commit_rejected`), STAGED-012 reserved-slot acceptance case, STAGED-011 `observed_count` no-mutation assertion, Node scenarios moved to the `Resolve` lane |
| 1.3 | 2026-11-18 | Review round 2: STAGED-014 gains the TTL_MISMATCH pre-write case (fence/ttl split pinned); begin-lane typed failures preferred in `repairUncertainFailure`; STAGED-012 matrix row restated as its two real cases; version/matrix alignment |
