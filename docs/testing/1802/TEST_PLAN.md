# Test Plan: Target-Resource-Scoped Remediation History Matching

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1802-v1.0
**Feature**: Fix cross-namespace/cross-cluster false-positive matching in DataStorage's remediation-history query, consumed by RO's ineffective-chain blocking and KA's LLM-prompt enrichment
**Version**: 1.0
**Created**: 2026-07-31
**Author**: AI Agent (Cursor)
**Status**: Active
**Branch**: `fix/1802-target-resource-scoping`

---

## 1. Introduction

### 1.1 Purpose

[Issue #1802](https://github.com/jordigilh/kubernaut/issues/1802) reports that `RoutingEngine.CheckIneffectiveRemediationChain` blocks `WorkflowExecution` creation for a target that has never been remediated before, because DataStorage's remediation-history query (`QueryROEventsBySpecHash`) matches purely on `pre_remediation_spec_hash`/`post_remediation_spec_hash` equality with no target-resource scoping. Two unrelated `Deployment`s with identical Pod specs (a common pattern — templated manifests, Helm charts, GitOps repos) collide and are treated as the same remediation target. This test plan provides failing-test evidence of the bug (RED), then proves the fix (GREEN) at all three tiers for both production consumers of the affected endpoint.

### 1.2 Objectives

1. **Repro proof**: A failing UT/IT/E2E set exists that reproduces the exact #1802 symptom against the current (pre-fix) code, before any fix code is written.
2. **DS query fix**: `QueryROEventsBySpecHash` scopes both Tier 1 (24h) and Tier 2 (90d) queries by `target_resource` in addition to `spec_hash`, using the existing `target_resource` column/index — no new migration for this part.
3. **Format bug fix**: `parseRemediationHistoryRequest`'s cluster-scoped target-resource string is unified to the same 3-part canonical format (`{namespace}/{kind}/{name}` or `{kind}/{name}`... see Design Decision D2) used everywhere else, closing a latent format-mismatch bug that would otherwise silently defeat the new target-resource filter for cluster-scoped resources.
4. **`cluster_id` scoping (main only)**: Both RO's blocking check and KA's enrichment path additionally scope by `cluster_id` on `main`, since fleet deployments can have identically-named/namespaced resources across clusters.
5. **Zero regressions**: All existing DS/RO/KA suites continue to pass with only signature-arity changes (no assertion changes) required.
6. **Backport**: A target-resource-only subset (no `cluster_id`, which doesn't exist on `release/v1.5`) lands on `release/v1.5`.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| RED evidence exists before GREEN | 100% of new test IDs | Each new test ID's first run is a failure, captured before its corresponding GREEN commit |
| Unit test pass rate | 100% | `go test ./test/unit/datastorage/...` |
| Integration test pass rate | 100% | `go test ./test/integration/{datastorage,remediationorchestrator,kubernautagent}/...` |
| E2E test pass rate | 100% | Live Kind-cluster run of `test/e2e/{remediationorchestrator,kubernautagent}/...` |
| Backward compatibility | 0 regressions | `remediation_history_query_fix_integration_test.go` (#616) and `ds_due_diligence_integration_test.go` (F1) pass unmodified in assertions |
| Pyramid Invariant compliance | 100% of Wiring Manifest rows | Section 9 Tier Skip Rationale — no pure-logic component ships IT/E2E-only |

---

## 2. References

### 2.1 Authority (governing documents)

- [BR-ORCH-042.5: Ineffective Remediation Chain Detection](../../requirements/BR-ORCH-042-consecutive-failure-blocking.md) — primary BR; will be amended alongside this fix to make target/cluster scoping an explicit requirement
- [BR-INS-001 / BR-INS-002](../../architecture/decisions/DD-HAPI-016-remediation-history-context.md) — secondary BRs (KA enrichment accuracy)
- [DD-HAPI-016: Remediation History Context Enrichment](../../architecture/decisions/DD-HAPI-016-remediation-history-context.md) — will be updated to v1.5
- Issue #1802: Cross-namespace false-positive ineffective-remediation blocking
- Issue #586: Prior change that removed target-resource scoping (the actual root cause origin)
- Issue #616: Prior fix adding post-remediation-hash OR-branch to the same query (regression-risk surface for this fix)
- [ISSUE-214/TEST_PLAN.md](../ISSUE-214/TEST_PLAN.md) — original BR-ORCH-042.5 test plan; this plan's new test IDs use the `1802` sequence per the same naming convention

### 2.2 Cross-References

- [Test Plan Template](../TEST_PLAN_TEMPLATE.md)
- [Test Case Specification Template](../TEST_CASE_SPECIFICATION_TEMPLATE.md)
- `AGENTS.md` — Pre-Implementation Workflow, Pyramid Invariant, TDD Anti-Patterns

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|-----------------|------------|
| R1 | Hidden production caller of `QueryROEventsBySpecHash`/`GetRemediationHistory` missed, leaving a third consumer unfixed | Medium — a third consumer would continue to see the bug | Low | All IT/E2E rows | `gopls references` (type-checked, not grep) confirmed exactly one production caller each, on both the DS repository side and the RO/KA client side |
| R2 | `release/v1.5` handler diff conflicts during backport (different code shape from `main`) | Low — backport delay, not a main-branch risk | Medium | Backport IT/E2E | `repository.go`/`ds_history_adapter.go`/`routing/types.go` confirmed byte-identical across branches via `git diff`; only `handler.go` needs hand-porting into its older (pre-Wave-3-refactor) shape, already inspected |
| R3 | `Enrich()`'s 8th parameter (`clusterID`) trips the AGENTS.md 8-param anti-pattern threshold | Low — lint/review finding, not a functional bug | High (certain, if left as a positional param) | N/A (structural) | Addressed explicitly as a REFACTOR step (request struct), not deferred |
| R4 | KA scope expansion (threading `ClusterID` through 6 call sites) introduces an unforeseen regression in investigation flow | Medium — KA's investigation path is complex | Low | `IT-KA-1802-001`, `E2E-KA-1802-001`, full existing KA suite | New IT/E2E are added at RED before any wiring change, so any regression is caught before REFACTOR; full existing KA suite re-run at Section 13 verification |
| R5 | E2E suites are flaky/slow (real Kind cluster), masking a real regression as infra noise | Medium | Medium | `E2E-RO-1802-001`, `E2E-KA-1802-001` | Both new E2E tests are added to already-stable, already-provisioned suites (`suite_test.go` infra reused, not new); if either is flaky on first run, re-run once before treating as signal |
| R6 | The stale `"pending"`-labeled stub in `blocking_e2e_test.go` misleads a future reader into thinking `BR-ORCH-042.1` (consecutive failure counting, a **different** sub-requirement) is also covered by this plan | Low | Medium | N/A (documentation risk) | Section 4.2 explicitly scopes this plan to `BR-ORCH-042.5` only; the `BR-ORCH-042.1` stub remains a separate, pre-existing gap not addressed here |

### 3.1 Risk-to-Test Traceability

R1, R2, R4, R5 all have direct test coverage per the table above. R3 and R6 are structural/documentation risks with process mitigations rather than test mitigations — no coverage gap.

---

## 4. Scope

### 4.1 Features to be Tested

- **DS query WHERE clause** (`pkg/datastorage/repository/remediation_history_repository.go`, `QueryROEventsBySpecHash`): target-resource (+ `cluster_id` on `main`) scoping added to both the direct `pre_remediation_spec_hash` match and the `post_remediation_spec_hash` correlation subquery.
- **DS handler target-resource format** (`pkg/datastorage/server/remediation_history_handler.go`, `parseRemediationHistoryRequest`): unify the cluster-scoped 2-part/3-part format inconsistency.
- **DS OpenAPI contract (`main` only)**: optional `clusterId` query parameter on `GET /api/v1/remediation-history/context`.
- **RO blocking wiring** (`pkg/remediationorchestrator/routing/{types,ds_history_adapter,blocking}.go`): `TargetResource.ClusterID` field, threaded from `RemediationRequest.Spec.ClusterID` (`main` only).
- **KA enrichment wiring** (`internal/kubernautagent/{investigator,enrichment}/*.go`): `signal.ClusterID` threaded from `Investigate`/`RunWorkflowDiscoveryFromRCA` through to the DS adapter (`main` only).
- **`cluster_id` btree index migration (`main` only)**: new migration under `pkg/shared/assets/migrations/`.
- **End-to-end journeys**: the actual #1802 cross-namespace repro through the real RO controller, and the analogous cross-cluster repro through the real KA server.

### 4.2 Features Not to be Tested

- **`BR-ORCH-042.1`** (consecutive failure counting, a different sub-requirement sharing the same `BR-ORCH-042` parent and the same stale E2E stub file) — pre-existing gap, unrelated to #1802, out of scope for this plan.
- **HAPI → Kubernaut Agent (KA) terminology rename** across documentation — tracked as a separate, dedicated GitHub issue per user direction, not implemented as part of this fix.
- **`release/v1.5` `cluster_id` scoping** — `release/v1.5` has no `cluster_id` concept on `RemediationRequest`/`SignalContext`; the backport ports the target-resource-only subset (see Section 17).
- **CC8.1 audit-trace reconstruction** (`pkg/datastorage/reconstruction/*`) — verified architecturally independent of this fix (see Section 8); not touched, not tested here.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| D1: Scope by `target_resource AND spec_hash` on **both** Tier 1 and Tier 2 (not spec-hash-only cross-namespace correlation) | Identical Pod specs across different targets/environments don't guarantee identical remediation outcomes (resource quotas, network policy, node placement, egress rules all differ) — a spec-hash-only cross-namespace match would trade one false-positive class for another |
| D2: Reuse the existing `target_resource` composite string column/index (`idx_audit_events_target_resource`) rather than add new structured columns/migration | Confirmed present on both `main` and `release/v1.5`; avoids an unnecessary schema migration for a filtering-logic fix |
| D3: `cluster_id` scoping is `main`-only, optional (`$N = '' OR cluster_id = $N`) | `release/v1.5` has no `cluster_id` field on `RemediationRequest`/`SignalContext`; making the filter optional (not `NOT NULL`-enforced) preserves fail-open behavior when the caller omits it |
| D4: Public API response field name for `targetResource` is unchanged; only the internal matching key is standardized to 3-part format | Preserves backward compatibility for existing API consumers while fixing the internal bug |
| D5: `Enrich()`'s new `clusterID` parameter is introduced via a request struct, not a 8th positional parameter | AGENTS.md Go Anti-Pattern Checklist: 8+ parameters must use Options pattern or config struct |

---

## 5. Approach

### 5.1 Coverage Policy

Per AGENTS.md Testing Requirements: Unit tier targets 100% of unit-testable business logic (structural coverage); Integration tier targets 100% of Wiring Manifest rows (requirements-based); E2E tier targets 100% of the user-facing journeys in scope (requirements-based, not line %).

### 5.2 Three-Tier Minimum (extends the template's Two-Tier baseline)

Both production consumers (RO, KA) get UT (where genuine branching/decision logic exists) + IT (wiring proof) + E2E (journey proof) — see Section 9 for the full Pyramid Invariant audit, including explicit rationale for every row that is not UT+IT+E2E.

### 5.3 Business Outcome Quality Bar

Every test answers a business question, not a code-path question:
- "Does an unrelated `Deployment` in a different namespace get incorrectly blocked?" (not "is `target_resource` passed as a parameter?")
- "Does a fleet investigation for cluster A leak cluster B's remediation history into an LLM prompt?" (not "is `ClusterID` threaded through 6 function calls?")

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. `IT-RO-1802-001` and `E2E-RO-1802-001` (the literal #1802 repro) prove the second target's `WorkflowExecution` is NOT blocked.
2. `IT-KA-1802-001` and `E2E-KA-1802-001` prove a fleet investigation for cluster A's resource does not surface cluster B's identically-named-resource history.
3. Zero regressions: `remediation_history_query_fix_integration_test.go` (#616) and `ds_due_diligence_integration_test.go` (F1) continue passing unmodified in assertions.
4. `go build ./...` and `golangci-lint run --timeout=5m` are clean on both the `main` PR and the `release/v1.5` backport PR.
5. `BR-ORCH-042.5` document text matches enforced behavior (amended, not just code-fixed).
6. Pyramid Invariant holds per Section 9: no pure-logic component ships IT/E2E-only, and every IT/E2E-only row has a documented (not silent) rationale.

**FAIL** — any of the following:

1. Any of the 4 primary repro tests (`IT-RO-1802-001`, `E2E-RO-1802-001`, `IT-KA-1802-001`, `E2E-KA-1802-001`) fails after GREEN.
2. A previously-passing test in `remediation_history_query_fix_integration_test.go` or `ds_due_diligence_integration_test.go` now fails (regression).
3. `Enrich()` ships with 8 positional parameters (anti-pattern not addressed).

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- Build is broken (`go build ./...` fails) — unit tests cannot execute.
- A Kind cluster cannot be provisioned for E2E (infra unavailable).
- More than 2 unrelated existing tests fail for the same root cause during Section 13 verification — stop and investigate before continuing the TDD sequence.

**Resume testing when**:
- Build is fixed and green.
- Kind cluster infra restored.
- Root cause identified and isolated (either fixed, or confirmed pre-existing/unrelated and explicitly called out).

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|--------------------|-----------------|
| `pkg/datastorage/repository/remediation_history_repository.go` | `QueryROEventsBySpecHash` (WHERE-clause predicate construction, tested via mocked DB per AGENTS.md Mock Strategy) | ~60 |
| `pkg/datastorage/server/remediation_history_handler.go` | `parseRemediationHistoryRequest` (target-resource string formatting, pure `fmt.Sprintf` branching) | ~30 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|--------------------|-----------------|
| `pkg/datastorage/server/remediation_history_handler.go` | `queryTier1History`, `queryTier2History` (real Postgres) | ~40 |
| `pkg/remediationorchestrator/routing/{blocking,ds_history_adapter,types}.go` | `CheckIneffectiveRemediationChain`, `GetRemediationHistory`, `TargetResource.ClusterID` | ~50 |
| `internal/kubernautagent/{investigator,enrichment}/*.go` | `Investigate`, `resolveEnrichmentCached`, `resolveEnrichment`, `Enrich`, `DSAdapter.GetRemediationHistory` | ~40 (signature changes only) |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|-----------------|-------|
| Code under test | `main` HEAD at branch creation | `fix/1802-target-resource-scoping`, branched from `origin/main` |
| Backport target | `release/v1.5` HEAD at backport branch creation | Post-`main`-merge |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-ORCH-042.5 | Ineffective chain detection must not cross target-resource boundaries | P0 | Unit | `UT-DS-1802-001` | Pending |
| BR-ORCH-042.5 | " | P0 | Integration | `IT-DS-1802-001`, `IT-RO-1802-001`, `IT-RO-1802-002` | Pending |
| BR-ORCH-042.5 | " | P0 | E2E | `E2E-RO-1802-001` | Pending |
| BR-INS-001/002 | Remediation-effectiveness correlation must not cross target/cluster boundaries in LLM enrichment | P0 | Integration | `IT-KA-1802-001` | Pending |
| BR-INS-001/002 | " | P0 | E2E | `E2E-KA-1802-001` | Pending |
| (structural) | Cluster-scoped target-resource string format must be internally consistent | P1 | Unit | `UT-DS-1802-002` | Pending |
| (structural) | " | P1 | Integration | `IT-DS-1802-002` | Pending |
| BR-ORCH-042.5 (main) | Fleet deployments must not cross-match across clusters | P1 | Integration | `IT-DS-1802-003` | Pending |

### Status Legend

Pending / RED / GREEN / REFACTORED / Pass (per template).

---

## 8. FedRAMP/SOC2 Control Mapping

**N/A — verified, not asserted.** This fix's code path (`QueryROEventsBySpecHash`, filtering `event_type = 'remediation.workflow_created'` and `event_category = 'effectiveness'`) was traced against the codebase's actual CC8.1 reconstruction implementation (`pkg/datastorage/reconstruction/query.go`, `QueryAuditEventsForReconstruction`), which queries `audit_events` by `correlation_id` only, against a fixed lifecycle-event-type allowlist (`IsReconstructionRelevant`: `gateway.signal.received`, `aianalysis.analysis.completed`, `workflowexecution.selection.completed`/`execution.started`, `orchestrator.lifecycle.*`). Zero code or event-type overlap exists between the two paths — confirmed via cross-reference search (no hits). This fix's business requirements (BR-ORCH-042.5, BR-INS-001/002) are consumption-side business-logic correctness fixes for RO's blocking decision and KA's LLM enrichment, not audit-emission or reconstruction-endpoint changes, and carry no control mapping in [DD-AUDIT-003](../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md)'s per-service catalog. No test in this plan claims FedRAMP/SOC2 control-objective coverage.

---

## 9. Test Scenarios (Pyramid Invariant Inventory)

### Test ID Naming Convention

`{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}` — e.g. `UT-DS-1802-001`. Per `TEST_PLAN_TEMPLATE.md` §8, using the issue number (`1802`) as the sequence-number component, consistent with `ISSUE-214`'s precedent for this same BR family.

### Tier 1: Unit Tests

**Testable code scope**: `remediation_history_repository.go` WHERE-clause construction, `remediation_history_handler.go` target-format branching.

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `UT-DS-1802-001` | Given a same-hash row belonging to a *different* target resource, the query predicate excludes it — proves the SQL predicate logic in isolation (mocked DB) | Pending |
| `UT-DS-1802-002` | Given a cluster-scoped (non-namespaced) resource, `parseRemediationHistoryRequest` emits the same 3-part canonical format used for namespaced resources — proves the format-unification logic | Pending |

### Tier 2: Integration Tests

**Testable code scope**: DS handler + real Postgres; RO blocking wiring; KA enrichment wiring.

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `IT-DS-1802-001` | Against a real Postgres instance, `QueryROEventsBySpecHash` (via the full handler stack) returns only same-target rows for a given spec hash | Pending |
| `IT-DS-1802-002` | The handler's target-resource string, as sent to the repository layer, is 3-part for cluster-scoped resources (end-to-end through the real HTTP handler) | Pending |
| `IT-DS-1802-003` (`main` only) | An optional `clusterId` query parameter further restricts results to the requesting cluster | Pending |
| `IT-RO-1802-001` | The exact #1802 repro: two `Deployment`s, identical spec hash, different namespaces — the second target's `WorkflowExecution` is NOT blocked, proven through RO's real `CheckIneffectiveRemediationChain` call chain against a real DS/Postgres | Pending |
| `IT-RO-1802-002` (`main` only) | RO's `TargetResource.ClusterID` (sourced from `RemediationRequest.Spec.ClusterID`) reaches the DS HTTP call | Pending |
| `IT-KA-1802-001` (`main` only) | A fleet investigation for cluster A's resource does not surface cluster B's identically-named-resource history in the enrichment result returned to the LLM prompt builder | Pending |

### Tier 3: E2E Tests

**Testable code scope**: Full production entry point — real RO controller reconcile loop; real KA server investigation session.

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `E2E-RO-1802-001` | Through the real deployed RO controller (Kind cluster, real Postgres, real DataStorage), creating two `Deployment`s with identical spec hash in different namespaces does NOT cause the second `WorkflowExecution` to be blocked as "ineffective" | Pending |
| `E2E-KA-1802-001` | Through a real running KA server (`sessionClient.Investigate`), a fleet investigation for one cluster's resource does not return another cluster's identically-named-resource remediation history in the enrichment result | Pending |

### Tier Skip Rationale

No tier is silently skipped. The following rows are intentionally IT/E2E-only (no independent UT), each with an explicit rationale rather than an omission:

| Row | Has decision logic? | Tier(s) | Rationale |
|-----|----------------------|---------|-----------|
| Handler param passthrough (`queryTier1History`/`queryTier2History`) | No — glue code | IT only | Categorized as integration-testable by the project's coverage-pattern taxonomy (HTTP handlers); no independent branching beyond what `UT-DS-1802-001/002` already cover |
| RO `ClusterID` threading | No — struct field + direct passthrough, no branching | IT only (`IT-RO-1802-002`) | The actual matching/filtering decision is exhaustively unit-tested at the DS layer (`UT-DS-1802-001`); RO's change is proven correct by IT showing the right value reaches the HTTP call |
| KA `ClusterID` threading | No — pure passthrough through 6 call sites | IT only (`IT-KA-1802-001`) | Same rationale as RO |
| `clusterId` OpenAPI param + ogen regen | No — generated code | IT only (`IT-DS-1802-003`) | Nothing hand-written to unit-test |
| `cluster_id` btree index migration | No | Migration/IT test only | Infra-only, no business logic |
| Both E2E rows | N/A (journey, not unit) | E2E only, backed by IT | E2E proves the production entry point (real controller / real server); UT+IT already cover the underlying decision logic exhaustively — E2E's value is proving the wiring reaches end-to-end, not re-proving the logic |

---

## 10. Test Cases (P0 detail)

### UT-DS-1802-001: Target-resource-scoped WHERE clause excludes cross-target matches

**BR**: BR-ORCH-042.5
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/repository/remediation_history_repository_test.go`

**Preconditions**: Mocked DB (`sqlmock` or repository's existing mock pattern) with two rows sharing `pre_remediation_spec_hash`, differing in `target_resource`.

**Test Steps**:
1. **Given**: Two `remediation.workflow_created` audit rows with identical `pre_remediation_spec_hash` but `target_resource = "ns-a/Deployment/app"` and `target_resource = "ns-b/Deployment/app"` respectively.
2. **When**: `QueryROEventsBySpecHash(ctx, "ns-a/Deployment/app", "", specHash, since, until)` is called.
3. **Then**: Only the `ns-a` row is returned.

**Expected Results**: Result set length 1, matching `ns-a`'s row.

**Acceptance Criteria**:
- **Behavior**: Query predicate includes `target_resource = $N`.
- **Correctness**: Cross-target row excluded.
- **Accuracy**: Same-target row's fields unchanged.

**Dependencies**: None (pure unit test against mocked DB).

---

### IT-RO-1802-001: Exact #1802 repro — cross-namespace false positive resolved

**BR**: BR-ORCH-042.5
**Priority**: P0
**Type**: Integration
**File**: `test/integration/remediationorchestrator/routing/ineffective_chain_test.go`

**Preconditions**: Real Postgres (DS schema applied), real DS server, RO routing engine wired to a real `DSHistoryAdapter`.

**Test Steps**:
1. **Given**: 3 consecutive ineffective remediation entries for `Deployment ns-a/app` (spec hash `H`).
2. **When**: A new `RemediationRequest` targets `Deployment ns-b/app` with the same spec hash `H`.
3. **Then**: `CheckIneffectiveRemediationChain` returns `nil` (not blocked) for the `ns-b` target.

**Expected Results**: No `BlockReasonIneffectiveChain` for the `ns-b/app` target; `ns-a/app`'s own chain-blocking behavior (regression check) is unaffected by the same test run.

**Acceptance Criteria**:
- **Behavior**: `ns-b/app`'s `WorkflowExecution` creation proceeds.
- **Correctness**: `ns-a/app`'s chain still blocks correctly if re-queried (no accidental exclusion of legitimate same-target chains).
- **Accuracy**: DS query result set for `ns-b` is empty.

**Dependencies**: `IT-DS-1802-001` (repository-level scoping) must pass first.

---

### E2E-RO-1802-001: Real controller journey — cross-namespace false positive resolved

**BR**: BR-ORCH-042.5
**Priority**: P0
**Type**: E2E
**File**: `test/e2e/remediationorchestrator/blocking_e2e_test.go` (replaces the stale stub `It` in the `BR-ORCH-042` `Describe` block)

**Preconditions**: Kind cluster with real RO controller, PostgreSQL, Redis, DataStorage deployed (`SetupROInfrastructureHybridWithCoverage`, existing `suite_test.go` infra — no new infra needed).

**Test Steps**:
1. **Given**: A `Deployment` `ns-a/app` has 3 consecutive ineffective remediations recorded via real audit events.
2. **When**: A `RemediationRequest` CR is created targeting `Deployment` `ns-b/app` with an identical Pod spec (same spec hash).
3. **Then**: The RO controller's reconcile loop does NOT block the resulting `WorkflowExecution`.

**Expected Results**: `WorkflowExecution` for `ns-b/app` reaches a non-blocked phase; RR status shows no `ManualReviewRequired` outcome tied to `BlockReasonIneffectiveChain`.

**Acceptance Criteria**:
- **Behavior**: Full production reconcile path (not a mocked/injected shortcut) exercises the fix.
- **Correctness**: Matches `IT-RO-1802-001`'s outcome end-to-end.
- **Accuracy**: Real Postgres-backed audit trail reflects the correct, unblocked lifecycle.

**Dependencies**: `IT-RO-1802-001` passing first (faster feedback loop); `green-ro-wiring` GREEN commit.

---

*(Remaining P0/P1 cases — `UT-DS-1802-002`, `IT-DS-1802-001/002/003`, `IT-RO-1802-002`, `IT-KA-1802-001`, `E2E-KA-1802-001` — follow the same Given/When/Then structure; summarized in Sections 7 and 9 rather than fully detailed here per the template's "detailed for P0, summarized for P1/P2" guidance.)*

---

## 11. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT/E2E Test ID |
|-----------|--------------------------|------------------------|-----------------|
| Target-resource scoped query | `QueryROEventsBySpecHash` | `pkg/datastorage/repository/remediation_history_repository.go` (WHERE clause) | `UT-DS-1802-001`, `IT-DS-1802-001` |
| Unified 3-part target format | `parseRemediationHistoryRequest` | `pkg/datastorage/server/remediation_history_handler.go` | `UT-DS-1802-002`, `IT-DS-1802-002` |
| Handler passthrough | `queryTier1History`, `queryTier2History` | same file | `IT-DS-1802-001` |
| `clusterId` OpenAPI param (`main` only) | `GetRemediationHistoryContext` | `api/openapi/data-storage-v1.yaml` + regenerated `pkg/datastorage/ogen-client` | `IT-DS-1802-003` |
| RO `ClusterID` threading (`main` only) | `RoutingEngine.CheckIneffectiveRemediationChain` | `pkg/remediationorchestrator/routing/types.go` (`TargetResource.ClusterID`), `ds_history_adapter.go` | `IT-RO-1802-002` |
| RO ineffective-chain repro | `CheckIneffectiveRemediationChain` | `pkg/remediationorchestrator/routing/blocking.go` | `IT-RO-1802-001` |
| RO E2E journey | Real RO controller reconcile loop | `test/e2e/remediationorchestrator/blocking_e2e_test.go` | `E2E-RO-1802-001` |
| KA `ClusterID` threading (`main` only) | `Investigator.Investigate` / `RunWorkflowDiscoveryFromRCA` | `internal/kubernautagent/investigator/{investigator,investigator_phases,investigator_discovery}.go` | `IT-KA-1802-001` |
| KA Enrich/DSAdapter `ClusterID` param (`main` only) | `Enricher.Enrich`, `DSAdapter.GetRemediationHistory` | `internal/kubernautagent/enrichment/{enricher,ds_adapter}.go` | `IT-KA-1802-001` |
| KA E2E journey | Real KA server session | `test/e2e/kubernautagent/incident_analysis_test.go` | `E2E-KA-1802-001` |
| `cluster_id` btree index (`main` only) | n/a (perf) | new migration in `pkg/shared/assets/migrations/` | Migration test |
| `BR-ORCH-042.5` text amendment | n/a (doc) | `docs/requirements/BR-ORCH-042-consecutive-failure-blocking.md` | n/a |

---

## 12. Environmental Needs

### 12.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: DB only (`sqlmock` or the repository's existing mock pattern) — per AGENTS.md Mock Strategy
- **Location**: `test/unit/datastorage/repository/`, `test/unit/datastorage/server/`

### 12.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks — real Postgres, real DS server
- **Infrastructure**: Postgres test container/instance, real DS binary or in-process server
- **Location**: `test/integration/datastorage/`, `test/integration/remediationorchestrator/routing/`, `test/integration/kubernautagent/enrichment/`

### 12.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Kind cluster, real RO controller / real KA server, real Postgres, real DataStorage (existing `suite_test.go` infra for both suites — no new infra)
- **Location**: `test/e2e/remediationorchestrator/`, `test/e2e/kubernautagent/`

### 12.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-------------------|---------|
| Go | per `go.mod` | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| Kind | per `test/infrastructure` pinned version | E2E cluster |
| `ogen` | per `Makefile` `generate-datastorage-client` | OpenAPI client regen (`main` only) |

---

## 13. Dependencies & Schedule

### 13.1 Blocking Dependencies

None — this fix is self-contained within `main` and does not depend on any other open issue/PR.

### 13.2 Execution Order

1. **RED — DS query**: `UT-DS-1802-001` + `IT-DS-1802-001`, fail against current code.
2. **GREEN — DS query**: target-resource (+ `cluster_id`) scoping added; tests pass.
3. **RED — DS format**: `UT-DS-1802-002`, fails against current code.
4. **GREEN/REFACTOR — DS format**: 3-part format unified; `UT-DS-1802-002`/`IT-DS-1802-002` pass.
5. **RED/GREEN — OpenAPI (`main` only)**: `clusterId` param + ogen regen; `IT-DS-1802-003`.
6. **RED — RO wiring**: `IT-RO-1802-001`, `E2E-RO-1802-001`, both fail against current code.
7. **GREEN — RO wiring**: `ClusterID` threaded; both pass.
8. **RED — KA wiring**: `IT-KA-1802-001`, `E2E-KA-1802-001`, both fail against current code.
9. **GREEN — KA wiring**: `signal.ClusterID` threaded end-to-end; both pass.
10. **REFACTOR**: `Enrich()` request struct; `DD-HAPI-016` -> v1.5; `BR-ORCH-042.5` amendment.
11. **GREEN — migration (`main` only)**: `cluster_id` btree index.
12. **Verification**: full build/lint/test, PR to `main`.
13. **Backport**: target-resource-only subset to `release/v1.5`.

---

## 14. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|--------------|
| This test plan | `docs/testing/1802/TEST_PLAN.md` | Strategy and test design |
| Unit test additions | `test/unit/datastorage/{repository,server}/` | Ginkgo BDD |
| Integration test additions | `test/integration/{datastorage,remediationorchestrator,kubernautagent}/...` | Ginkgo BDD |
| E2E test additions | `test/e2e/{remediationorchestrator,kubernautagent}/...` | Ginkgo BDD |
| `BR-ORCH-042.5` amendment | `docs/requirements/BR-ORCH-042-consecutive-failure-blocking.md` | Text + changelog |
| `DD-HAPI-016` v1.5 | `docs/architecture/decisions/DD-HAPI-016-remediation-history-context.md` | Scope update + changelog |

---

## 15. Execution

```bash
# Unit tests
go test ./test/unit/datastorage/... -ginkgo.v

# Integration tests
go test ./test/integration/datastorage/... ./test/integration/remediationorchestrator/... ./test/integration/kubernautagent/... -ginkgo.v

# Specific test by ID
go test ./test/unit/datastorage/... -ginkgo.focus="UT-DS-1802"

# E2E (requires Kind + Docker)
go test ./test/e2e/remediationorchestrator/... -ginkgo.focus="E2E-RO-1802" -timeout=30m
go test ./test/e2e/kubernautagent/... -ginkgo.focus="E2E-KA-1802" -timeout=30m
```

---

## 16. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|----------------------|----------------------|-------------------|--------|
| `remediation_history_query_fix_integration_test.go` (#616) | Query returns rows matching spec hash across pre/post branches | None (assertion preserved) — verify it uses a single consistent target so target-scoping doesn't change its result set | Confirmed in preflight: uses a single target per scenario |
| `ds_due_diligence_integration_test.go` (F1) | EM subquery is time-unbounded, matches by hash | None (assertion preserved) — same target-consistency verification | Confirmed in preflight |
| `RemediationHistoryQuerier` interface implementers (mocks in RO/KA unit tests) | N-arg mock signatures | Signature-arity update only (add `targetResource`/`clusterID` params) — no assertion logic change | New required parameters on the interface |

---

## 17. Backport Scope (`release/v1.5`)

`release/v1.5` has the identical bug and the identical two E2E suites (confirmed: `blocking_e2e_test.go` has the same stale `"pending"`-labeled stub; `incident_analysis_test.go` exists with its own working tests). The backport ports:
- `UT-DS-1802-001`, `IT-DS-1802-001` (target-resource scoping, no `cluster_id` dimension)
- `UT-DS-1802-002`, `IT-DS-1802-002` (format fix — identical bug present)
- `IT-RO-1802-001`, `E2E-RO-1802-001` (target-resource-only variant)
- `IT-KA-1802-001` target-resource-only variant, and an `E2E-KA-1802-001` variant **only if** `release/v1.5`'s KA E2E suite exercises the same enrichment path; otherwise the KA E2E gap on this branch is documented rather than silently skipped.

Not backported: `cluster_id` scoping, OpenAPI/ogen changes, the `cluster_id` migration, `IT-DS-1802-003`, `IT-RO-1802-002`.

---

## 18. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-07-31 | Initial test plan, written before any branch/code changes per AGENTS.md Pre-Implementation Workflow |
