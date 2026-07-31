# Test Plan: Target-Resource-Scoped Remediation History Matching (release/v1.5 Backport)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1802-v1.5-BACKPORT
**Feature**: Backport of [#1802](https://github.com/jordigilh/kubernaut/issues/1802)'s fix to DataStorage's remediation-history query, consumed by RO's ineffective-chain blocking (KA's LLM-prompt enrichment inherits the fix transparently — see Section 4.3, D0)
**Version**: 1.0
**Created**: 2026-07-31
**Author**: AI Agent (Cursor)
**Status**: Active
**Branch**: `fix/1802-backport-v1.5`

---

## 1. Introduction

### 1.1 Purpose

[Issue #1802](https://github.com/jordigilh/kubernaut/issues/1802) reports that `RoutingEngine.CheckIneffectiveRemediationChain` blocks `WorkflowExecution` creation for a target that has never been remediated before, because DataStorage's remediation-history query (`QueryROEventsBySpecHash`) matches purely on `pre_remediation_spec_hash`/`post_remediation_spec_hash` equality with no target-resource scoping. Two unrelated `Deployment`s with identical Pod specs (a common pattern — templated manifests, Helm charts, GitOps repos) collide and are treated as the same remediation target. The fix landed on `main` via [PR #1809](https://github.com/jordigilh/kubernaut/pull/1809). This test plan governs the **target-resource-only backport** of that fix to `release/v1.5`, which has no `cluster_id` concept and therefore excludes the fleet-scoping dimension of the `main` fix entirely (not deferred — genuinely inapplicable to this branch).

### 1.2 Objectives

1. **DS query fix**: `QueryROEventsBySpecHash` scopes both Tier 1 (24h) and Tier 2 (90d) queries by `target_resource` in addition to `spec_hash`, using the existing `target_resource` column/index — no new migration.
2. **Format bug fix**: The handler's cluster-scoped (namespace-less) target-resource string is unified to the same 3-part canonical format (`{namespace}/{kind}/{name}`, empty namespace producing a leading `/`) that RO's audit emission unconditionally writes — closing a latent format-mismatch bug that would otherwise silently defeat the new target-resource filter for cluster-scoped resources (e.g. `Node`).
3. **Zero regressions**: All existing DS/RO suites continue to pass with only signature-arity changes (no assertion changes) required.
4. **No KA code changes**: Confirmed in preflight (Section 4.3, D0) — KA already sends `targetKind`/`targetName`/`targetNamespace` to the same DS endpoint (Issue #214); the DS-side fix applies transparently. Only a proving IT is added, no wiring change.

### 1.3 Out of Scope (genuinely inapplicable to this branch, not deferred)

- `cluster_id` scoping (`main`-only; `release/v1.5`'s `RemediationRequest`/`SignalContext` have no `ClusterID` field)
- The `clusterId` OpenAPI query parameter and its `ogen` regeneration
- `pkg/remediationorchestrator/routing/{types,ds_history_adapter,blocking}.go` changes — confirmed byte-identical to `main`'s pre-#1802 baseline and require **no changes** for the target-resource-only fix (Section 4.3, D0)
- The `idx_audit_events_cluster_id` migration — the `cluster_id` column does not exist in this branch's schema at all (confirmed via migration grep)
- `internal/kubernautagent/**` wiring changes — confirmed unnecessary (Section 4.3, D0)
- `DD-HAPI-016` v1.5 documentation update — that doc's `main`-authored v1.5 changelog entry already documents both the target-resource fix (all branches) and `cluster_id` scoping (`main` only) with explicit `release/v1.5` callouts; no separate branch-specific doc fork needed

### 1.4 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/datastorage/...` |
| Integration test pass rate | 100% | `go test ./test/integration/{datastorage,remediationorchestrator,kubernautagent}/...` |
| E2E test pass rate | 100% | Live Kind-cluster run of `test/e2e/remediationorchestrator/...` |
| Backward compatibility | 0 regressions | `remediation_history_query_fix_integration_test.go` (#616) and `ds_due_diligence_integration_test.go` (F1) pass unmodified in assertions |
| Pyramid Invariant compliance | 100% of Wiring Manifest rows | Section 9 — no pure-logic component ships IT/E2E-only without documented rationale |

---

## 2. References

### 2.1 Authority (governing documents)

- [BR-ORCH-042.5: Ineffective Remediation Chain Detection](../../requirements/BR-ORCH-042-consecutive-failure-blocking.md) — primary BR; amended (v1.6 -> v1.7) alongside this backport
- [DD-HAPI-016: Remediation History Context Enrichment](../../architecture/decisions/DD-HAPI-016-remediation-history-context.md) — authored on `main` at v1.5; already documents this branch's scope explicitly (no fork needed)
- Issue #1802: Cross-namespace false-positive ineffective-remediation blocking
- [PR #1809](https://github.com/jordigilh/kubernaut/pull/1809): `main` fix (merged basis for this backport)
- Issue #586: Prior change that removed target-resource scoping (the actual root cause origin)
- Issue #616: Prior fix adding post-remediation-hash OR-branch to the same query (regression-risk surface for this fix)

### 2.2 Cross-References

- `AGENTS.md` — Pre-Implementation Workflow, Pyramid Invariant, TDD Anti-Patterns

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|-----------------|------------|
| R1 | `release/v1.5`'s `remediation_history_handler.go` has a different (older, pre-Wave-3-refactor) code shape than `main`, causing a raw cherry-pick to conflict | Low — resolved during GREEN, not a functional risk | Confirmed (materialized) | All DS handler tests | Hand-resolved the merge conflict; confirmed `repository.go`/`remediation_history_adapter.go` are byte-identical to `main`'s pre-#1802 baseline (clean auto-merge), only the handler's endpoint function needed manual re-integration into its simpler, non-decomposed shape |
| R2 | `cluster_id` column does not exist in this branch's schema; a naive cherry-pick of `main`'s SQL (which references `cluster_id`) would compile but fail at runtime with "column does not exist" | High if unnoticed — silent runtime failure, not caught by `go build` | Confirmed (materialized) | `UT-RH-*`, `IT-DS-1802-001` | Confirmed via `git grep cluster_id migrations/` (zero hits) before writing any SQL; stripped `clusterID` param and predicate entirely from the repository function rather than defaulting it to `""` |
| R3 | RO or KA requires code changes to benefit from the DS-side fix, expanding backport scope unexpectedly | Medium if true — would require porting `routing/*.go` and `internal/kubernautagent/**` changes too | Low (verified false) | N/A | Confirmed via `git diff` that `routing/types.go` and `routing/ds_history_adapter.go` are byte-identical between `release/v1.5` and `main`'s pre-#1802 baseline — RO already forwards `targetKind`/`targetName`/`targetNamespace` to DS (Issue #214); the WHERE-clause fix alone closes the gap for both RO and KA with zero client-side changes |
| R4 | E2E suite (real Kind cluster) is flaky, masking a real regression as infra noise | Medium | Medium | `E2E-RO-1802-001` | Test added to the already-stable, already-provisioned `blocking_e2e_test.go` suite infra; re-run once before treating a failure as signal |

### 3.1 Risk-to-Test Traceability

All four risks have direct test/verification coverage per the table above — no coverage gap.

---

## 4. Scope

### 4.1 Features to be Tested

- **DS query WHERE clause** (`pkg/datastorage/repository/remediation_history_repository.go`, `QueryROEventsBySpecHash`): target-resource scoping added to both the direct `pre_remediation_spec_hash` match and the `post_remediation_spec_hash` correlation subquery. No `cluster_id` parameter.
- **DS handler target-resource format** (`pkg/datastorage/server/remediation_history_handler.go`): unify the cluster-scoped 2-part/3-part format inconsistency to match RO's unconditional 3-part audit-emission format.
- **KA transparent inheritance**: a proving integration test confirms KA's real enrichment path (`Enricher.Enrich` -> `DSAdapter` -> real DS -> real Postgres) does not surface cross-namespace history for an identically-specced target, with zero KA code changes.
- **End-to-end journey**: the actual #1802 cross-namespace repro through the real RO controller (Kind cluster).

### 4.2 Features Not to be Tested

See Section 1.3 (Out of Scope) — `cluster_id`/fleet scoping, OpenAPI/ogen changes, the `cluster_id` migration, and RO/KA wiring changes are genuinely inapplicable to this branch, not silently dropped.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| D0: No RO or KA production code changes in this backport | Verified via `git diff` that `pkg/remediationorchestrator/routing/{types,ds_history_adapter}.go` are byte-identical to `main`'s pre-#1802 baseline; both already forward `targetKind`/`targetName`/`targetNamespace` to DS (Issue #214). The fix is entirely a DS-side WHERE-clause change; RO and KA inherit it transparently through the existing HTTP contract. `blocking.go` differs from `main` only in unrelated post-#1802 refactors — no `parseTargetResource`/`ClusterID`-shaped change is needed here. |
| D1: Scope by `target_resource AND spec_hash` (not spec-hash-only cross-namespace correlation) | Identical Pod specs across different targets don't guarantee identical remediation outcomes (resource quotas, network policy, node placement all differ) — a spec-hash-only cross-namespace match would trade one false-positive class for another |
| D2: Reuse the existing `target_resource` composite string column/index (`idx_audit_events_target_resource`) rather than add new structured columns/migration | Confirmed present on this branch; avoids an unnecessary schema migration for a filtering-logic fix |
| D3: No `cluster_id` parameter anywhere in the ported signature (not an optional/unused parameter) | The `cluster_id` column does not exist in this branch's schema (confirmed via migration grep) — an unused parameter would be dead code and a Go anti-pattern; the parameter is omitted entirely rather than defaulted |
| D4: Public API response field name for `targetResource` is unchanged; only the internal matching key is standardized to 3-part format | Preserves backward compatibility for existing API consumers while fixing the internal bug |

---

## 5. Approach

### 5.1 Coverage Policy

Per AGENTS.md Testing Requirements: Unit tier targets 100% of unit-testable business logic (structural coverage); Integration tier targets 100% of Wiring Manifest rows (requirements-based); E2E tier targets 100% of the user-facing journeys in scope.

### 5.2 Business Outcome Quality Bar

Every test answers a business question, not a code-path question: "Does an unrelated `Deployment` in a different namespace get incorrectly blocked?" (not "is `target_resource` passed as a parameter?").

### 5.3 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. `IT-RO-1802-001` and `E2E-RO-1802-001` (the literal #1802 repro) prove the second target's `WorkflowExecution` is NOT blocked.
2. `IT-KA-1802-001` proves KA's real enrichment path does not surface a different namespace's history for an identically-specced target.
3. Zero regressions: `remediation_history_query_fix_integration_test.go` (#616) and `ds_due_diligence_integration_test.go` (F1) continue passing unmodified in assertions.
4. `go build ./...` and `golangci-lint run --timeout=5m` are clean.
5. `BR-ORCH-042.5` document text matches enforced behavior (amended, not just code-fixed).

**FAIL** — any primary repro test fails after GREEN, or a previously-passing regression test now fails.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods |
|------|--------------------|
| `pkg/datastorage/repository/remediation_history_repository.go` | `QueryROEventsBySpecHash` (WHERE-clause predicate construction, tested via `sqlmock`) |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods |
|------|--------------------|
| `pkg/datastorage/server/remediation_history_handler.go` | `HandleGetRemediationHistoryContext` (real Postgres) |
| `pkg/remediationorchestrator/routing/{blocking,ds_history_adapter}.go` | `CheckIneffectiveRemediationChain`, `GetRemediationHistory` (unchanged code, proving IT only) |
| `internal/kubernautagent/enrichment/*.go` | `Enricher.Enrich` (unchanged code, proving IT only) |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-ORCH-042.5 | Ineffective chain detection must not cross target-resource boundaries | P0 | Unit | `UT-DS-1802-001` | Pending |
| BR-ORCH-042.5 | " | P0 | Integration | `IT-DS-1802-001`, `IT-RO-1802-001` | Pending |
| BR-ORCH-042.5 | " | P0 | E2E | `E2E-RO-1802-001` | Pending |
| BR-INS-001/002 | Remediation-effectiveness correlation must not cross target boundaries in LLM enrichment | P1 | Integration | `IT-KA-1802-001` | Pending |
| (structural) | Cluster-scoped target-resource string format must be internally consistent | P1 | Unit + Integration | `UT-DS-1802-002`, `IT-DS-1802-002` | Pending |

---

## 8. FedRAMP/SOC2 Control Mapping

**N/A — verified, not asserted.** Identical rationale to the `main` fix ([PR #1809](https://github.com/jordigilh/kubernaut/pull/1809)): `QueryROEventsBySpecHash`'s code path has zero overlap with the CC8.1 reconstruction endpoint (`pkg/datastorage/reconstruction/query.go`), which queries by `correlation_id` against a fixed lifecycle-event-type allowlist unrelated to `remediation.workflow_created`/`effectiveness` filtering. This is a consumption-side business-logic correctness fix, not an audit-emission or reconstruction-endpoint change, and carries no control mapping in DD-AUDIT-003.

---

## 9. Test Scenarios (Pyramid Invariant Inventory)

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `UT-DS-1802-001` | Given a same-hash row belonging to a *different* target resource, the query predicate excludes it | Pending |
| `UT-DS-1802-002` | Given a cluster-scoped (non-namespaced) resource, the handler's target-resource string uses the same unconditional 3-part canonical format used for namespaced resources | Pending |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `IT-DS-1802-001` | Against a real Postgres instance, `QueryROEventsBySpecHash` (via the full handler stack) returns only same-target rows for a given spec hash | Pending |
| `IT-DS-1802-002` | The handler's target-resource string, as sent to the repository layer, is 3-part for cluster-scoped resources (end-to-end through the real HTTP handler) | Pending |
| `IT-RO-1802-001` | The exact #1802 repro: two `Deployment`s, identical spec hash, different namespaces — the second target's `WorkflowExecution` is NOT blocked, proven through RO's real `CheckIneffectiveRemediationChain` call chain against a real DS/Postgres | Pending |
| `IT-KA-1802-001` | KA's real enrichment path (`Enricher.Enrich` -> `DSAdapter` -> real DS/Postgres) does not surface a different namespace's identically-specced-resource history — zero KA code changes, proving transparent inheritance (D0) | Pending |

### Tier 3: E2E Tests

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `E2E-RO-1802-001` | Through the real deployed RO controller (Kind cluster, real Postgres, real DataStorage), creating two `Deployment`s with identical spec hash in different namespaces does NOT cause the second `WorkflowExecution` to be blocked as "ineffective" | Pending |

### Tier Skip Rationale

| Row | Has decision logic? | Tier(s) | Rationale |
|-----|----------------------|---------|-----------|
| KA transparent inheritance | No — zero code change, proving existing wiring | IT only (`IT-KA-1802-001`) | The matching/filtering decision is exhaustively unit-tested at the DS layer (`UT-DS-1802-001`); no KA-specific E2E is added because no KA code changed — an E2E would duplicate `IT-KA-1802-001` without proving anything additional (D0) |
| RO E2E | N/A (journey, not unit) | E2E only, backed by IT | E2E proves the production entry point (real controller); UT+IT already cover the underlying decision logic exhaustively |

---

## 10. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT/E2E Test ID |
|-----------|--------------------------|------------------------|-----------------|
| Target-resource scoped query | `QueryROEventsBySpecHash` | `pkg/datastorage/repository/remediation_history_repository.go` (WHERE clause) | `UT-DS-1802-001`, `IT-DS-1802-001` |
| Unified 3-part target format | `HandleGetRemediationHistoryContext` | `pkg/datastorage/server/remediation_history_handler.go` | `UT-DS-1802-002`, `IT-DS-1802-002` |
| RO ineffective-chain repro (no code change; proving IT) | `CheckIneffectiveRemediationChain` | `pkg/remediationorchestrator/routing/blocking.go` (unchanged) | `IT-RO-1802-001` |
| RO E2E journey | Real RO controller reconcile loop | `test/e2e/remediationorchestrator/blocking_e2e_test.go` | `E2E-RO-1802-001` |
| KA transparent inheritance (no code change; proving IT) | `Enricher.Enrich` | `internal/kubernautagent/enrichment/enricher.go` (unchanged) | `IT-KA-1802-001` |
| `BR-ORCH-042.5` text amendment | n/a (doc) | `docs/requirements/BR-ORCH-042-consecutive-failure-blocking.md` | n/a |

---

## 11. Environmental Needs

- **Unit**: Ginkgo/Gomega BDD, `sqlmock` (DB-only mocking per AGENTS.md Mock Strategy), `pkg/datastorage/`
- **Integration**: Ginkgo/Gomega BDD, zero mocks (real Postgres, real DS server), `test/integration/{datastorage,remediationorchestrator,kubernautagent}/`
- **E2E**: Ginkgo/Gomega BDD, Kind cluster + real RO controller + real Postgres + real DataStorage (existing `suite_test.go` infra, no new infra), `test/e2e/remediationorchestrator/`

---

## 12. Execution Order

1. **GREEN — DS query**: target-resource scoping added (cherry-picked and hand-adapted from `main`'s [PR #1809](https://github.com/jordigilh/kubernaut/pull/1809), `cluster_id` stripped); `UT-DS-1802-001`/`IT-DS-1802-001` pass.
2. **GREEN — DS format**: 3-part format unified; `UT-DS-1802-002`/`IT-DS-1802-002` pass.
3. **GREEN — RO proving test**: `IT-RO-1802-001`, `E2E-RO-1802-001` pass against unchanged RO code (D0).
4. **GREEN — KA proving test**: `IT-KA-1802-001` passes against unchanged KA code (D0).
5. **REFACTOR**: `BR-ORCH-042.5` amendment (v1.6 -> v1.7).
6. **Verification**: full build/lint/test, PR to `release/v1.5`.

---

## 13. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|----------------------|----------------------|-------------------|--------|
| `remediation_history_query_fix_integration_test.go` (#616) | Query returns rows matching spec hash across pre/post branches | None (assertion preserved) — call sites updated to pass `targetResource` (new required param) | Confirmed: each scenario uses a single consistent target, so target-scoping doesn't change its result set |
| `ds_due_diligence_integration_test.go` (F1) | EM subquery is time-unbounded, matches by hash | None (assertion preserved) — same call-site update | Confirmed: single-target scenario |
| `remediation_history_handler_test.go`, `remediation_history_repository_test.go` mocks | N-arg mock signatures | Signature-arity update only (add `targetResource` param) — no assertion logic change | New required parameter on the interface |

---

## 14. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-07-31 | Initial backport test plan, written before backport implementation per AGENTS.md Pre-Implementation Workflow. Adapted from `main`'s TP-1802-v1.0 (this same document, ported wholesale by `git cherry-pick` then rewritten to accurately scope this branch — see Section 1.3 for what was dropped and why). |
