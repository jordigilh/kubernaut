# Test Plan: SP Multi-Dimension Readiness Audit (#1110)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> per-tier coverage policy, and 9-category quality gate checkpoints.

**Test Plan Identifier**: TP-1110-v1
**Feature**: SP Multi-Dimension Readiness Audit — 36 findings across 8 dimensions
**Version**: 1.0
**Created**: 2026-05-12
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/1110-sp-readiness-audit`

---

## 1. Introduction

### 1.1 Purpose

This test plan addresses 36 findings identified by the SP Multi-Dimension Readiness Audit,
which concluded the Signal Processing service is "NOT READY" for GA with 1 critical bug,
9 high-severity findings, and 13 medium findings. A previous implementation attempt was
reverted for violating TDD methodology. This plan restarts with strict TDD (Red/Green/Refactor)
and 9-category quality gate checkpoints at every phase boundary.

### 1.2 Objectives

1. **Fix E1 (Critical)**: ConsecutiveFailures counter never resets on successful phase transitions
2. **Fix 8 High-severity findings**: isTransientError wrapping (E2), enricher degraded mode (BLAST-A4/A5), observability gaps (O1-O3), state machine (S1), startup validation (O5)
3. **Fix 13 Medium findings**: Error logging (E5), audit handling (E6), Rego timeouts (E7), cluster-scoped guards (BLAST-A3), metrics gaps (O4-O7), concurrency (CONC-C1/C3), conditions (S2-S4)
4. **Fix 14 Low/Tech-debt findings**: Degraded context (BLAST-B1/B2), cache bounds, documentation, test quality, API surface
5. **Achieve >=80% per-tier testable code coverage** for all modified SP packages

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/signalprocessing/... -count=1` |
| Integration test pass rate | 100% | `go test ./test/integration/signalprocessing/... -count=1` |
| Build clean | 0 errors | `go build ./internal/controller/signalprocessing/... ./pkg/signalprocessing/...` |
| Lint clean | 0 new warnings | `golangci-lint run --timeout=5m` |
| Race detector clean | 0 races | `go test -race ./test/unit/signalprocessing/...` |
| Regression check | 0 regressions | All existing tests pass |
| 9-category checkpoint | All 9 satisfied | Per-phase audit documented below |

---

## 2. References

### 2.1 Authority (Governing Documents)

| Document | Relevance |
|----------|-----------|
| BR-SP-111 | Shared exponential backoff: reset failure counter on successful phase transition |
| BR-SP-090 | Categorization audit trail: audit on signal processing start, enrichment, classification, completion |
| BR-SP-110 | Kubernetes Conditions for operator visibility: failure reasons by stage |
| BR-SP-001 | K8s context enrichment with cache and configurable TTL |
| BR-SP-070 | Priority assignment (Issue #437 guard) |
| BR-SP-080 | Environment classification; signal labels forbidden for classification (security) |
| ADR-032 | Data access layer isolation: no silent skips for audit failures |
| ADR-034 | Unified audit table design: event naming convention `{service}.{domain}.{action}` |
| DD-SP-002 | Kubernetes conditions specification: lifecycle state machine, failure paths |
| DD-017 | Enrichment architecture: Principle 3 graceful degradation with partial context |
| DD-005 | Observability standards: structured logging, metrics shape |
| DD-EVENT-001 | Controller Kubernetes Event Registry: constants mandatory, no raw strings |
| DD-PERF-001 | Atomic status updates mandate for SP |
| DD-AUDIT-003 | Service audit trace requirements: SP is SHOULD (P1) |
| DD-TEST-006 | Test plan policy: formal test plans, ID convention |
| OpenAPI spec | `api/openapi/data-storage-v1.yaml`: SP event_type enum (6 values) |

### 2.2 Cross-References

- [Testing Guidelines](../../../docs/development/business-requirements/TESTING_GUIDELINES.md)
- [100 Go Mistakes](https://github.com/teivah/100-go-mistakes)
- [Issue #1110](https://github.com/jordigilh/kubernaut/issues/1110)
- [Issue #1116](https://github.com/jordigilh/kubernaut/issues/1116) — AA cross-cutting (out of scope)
- [SP Readiness Audit Plan](../../../.cursor/plans/sp_multi-dimension_readiness_audit_a53a2da0.plan.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Mitigation |
|----|------|--------|-------------|------------|
| R1 | Phase 2 BR creation surfaces tension with BR-SP-080 (signal label prohibition) | Scope creep | Medium | BR wording distinguishes "workload labels" (safe) from "signal labels" (forbidden per BR-SP-080) |
| R2 | CONC-C2 TOCTOU test is flaky under `-race` | False positives | Medium | Use deterministic synchronization (channels/barriers) rather than timing-dependent races |
| R3 | `isClusterScoped` is unexported in `ownerchain/builder.go` | Test access | Low | Exercise through `Reconcile()` with shaped inputs; export only if strictly needed |
| R4 | Phase 6 breadth (13 items) risks overlooking a test | Incomplete coverage | Low | Systematic checklist with per-item sign-off |
| R5 | Existing test drift: `controller_error_handling_test.go` has local `isTransientError` duplicate | Test/prod divergence | High | RED test must exercise production code, not local duplicate. Delete duplicate or align. |

---

## 4. Scope

### 4.1 Findings In Scope (36 total)

All 36 findings from the multi-dimension readiness audit are in scope. No exclusions.

### 4.2 Finding Inventory

#### Dimension 2: Error Handling and Resilience

| ID | Severity | Type | Description | Authority |
|----|----------|------|-------------|-----------|
| E1 | Critical | Bug | ConsecutiveFailures never resets — guard checks `Requeue` but success paths use `RequeueAfter` | BR-SP-111 |
| E2 | High | Bug | `isTransientError`: `==` instead of `errors.Is` for wrapped context errors | Go coding standards |
| E5 | Medium | Gap | 12+ error returns without `logger.Error` | 00-kubernaut-core-rules + DD-005 |
| E6 | Medium | Inconsistency | `RecordError` audit return ignored with `_ =` | ADR-032 |
| E7 | Medium | Gap | No Rego timeout for `EvaluateEnvironment` / `EvaluateSeverity` | Consistency with `EvaluatePriority` pattern |

#### Dimension 4: Observability (reclassified O5)

| ID | Severity | Type | Description | Authority |
|----|----------|------|-------------|-----------|
| O5 | High | Gap | PolicyEvaluator nil should prevent SP startup (fail-fast at SetupWithManager + Reconcile) | Defense in depth |

#### Dimension 1: Cluster-Scoped Resource Handling

| ID | Severity | Type | Description | Authority |
|----|----------|------|-------------|-----------|
| BLAST-A1 | High | Bug | `classifyBusiness` loses workload labels when Namespace nil | New BR-SP-XXX (prerequisite) |
| BLAST-A2 | High | Bug | CustomLabels fallback skips workload labels when Namespace nil | New BR-SP-XXX (prerequisite) |
| BLAST-A3 | Medium | Bug | #437 guard treats nil Namespace as incomplete context | Issue #437 -> BR-SP-070 |
| BLAST-A4 | High | Bug | `enrichNodeSignal` no degraded mode on Node NotFound | DD-017 Principle 3 |
| BLAST-A5 | High | Bug | `enrichNamespaceOnly` fails for cluster-scoped kinds with empty namespace | DD-017 Principle 3 |
| BLAST-B1 | Low | Inconsistency | `BuildDegradedContext` always sets non-nil Namespace (even for cluster-scoped) | DD-SP-002 |
| BLAST-B2 | Low | Inconsistency | Log format `Kind /name` with empty namespace | Hardening |
| D4 | Low | Spec test | Enricher intentional panic on nil metrics — document fail-fast contract | Checkpoint 5 |

#### Dimension 4: Observability Gaps

| ID | Severity | Type | Description | Authority |
|----|----------|------|-------------|-----------|
| O1 | High | Gap | Classification/severity failures don't emit `signalprocessing.error.occurred` audit | BR-SP-090 + OpenAPI enum |
| O2 | High | Gap | No phase transition audit at first reconcile (`""` -> `Pending`) | BR-SP-090 via `RecordPhaseTransition` |
| O3 | High | Gap | PhaseFailed doesn't record `completed/failure` metrics | DD-005 pattern |
| O4 | High | Gap | Early exits (#437 requeue, FreshGet failure) skip metrics | Hardening |
| O6 | Medium | Gap | K8s events missing for hard enrichment failure | DD-EVENT-001 |
| O7 | Medium | Inconsistency | `UnsupportedTargetType` event uses string literals vs package constants | DD-EVENT-001 |

#### Dimension 3: Concurrency and Resource Bounds

| ID | Severity | Type | Description | Authority |
|----|----------|------|-------------|-----------|
| CONC-C1 | Medium | Gap | `MaxConcurrentReconciles` not explicitly set (runtime default is 1) | Documentation/defense |
| CONC-C2 | Medium | Gap | TOCTOU between reads and `AtomicStatusUpdate` | Concurrency checkpoint |
| CONC-C3 | Medium | Gap | TTLCache unbounded growth — no max entries, no eviction | BR-SP-001 (cache) |
| CONC-C4 | Low | Regression | AtomicStatusUpdate serialization — regression guard | DD-PERF-001 |

#### Dimension 8: Conditions / Phase State Machine

| ID | Severity | Type | Description | Authority |
|----|----------|------|-------------|-----------|
| S1 | High | Gap | PhaseFailed doesn't set `CategorizationComplete` / `ProcessingComplete` conditions | DD-SP-002 |
| S2 | Medium | Gap | Env/priority failure loops in Classifying without terminal failure | DD-SP-002 lifecycle |
| S3 | Medium | Dead code | `ReasonAuditWriteFailed` defined in DD-SP-002 but never used in production | DD-SP-002 |
| S4 | Medium | Gap | `SetCategorizationComplete(false)` never called in production | DD-SP-002 + BR-SP-110 |

#### Dimension 6: Test Quality and Coverage

| ID | Severity | Type | Description | Authority |
|----|----------|------|-------------|-----------|
| T1 | High | Gap | Integration test over-claims BR coverage vs weak assertions | TESTING_GUIDELINES |
| T2 | High | Gap | Zero cluster-scoped / Node-as-target tests | Phase 2 dependency |
| T3 | High | Tech debt | `time.Sleep` in tests vs forbidden policy | TESTING_GUIDELINES v2.0 |
| T4 | Medium | Tech debt | Duplicate assertion in reconciliation test | Test hygiene |
| T5 | Medium | Tech debt | Test ID naming inconsistency (`CTRL-*` vs `UT-SP-*`) | DD-TEST-006 |
| T6 | Medium | Tech debt | Test plan doc drift | DD-TEST-006 |

#### Dimension 5: API Surface and CRD Hygiene

| ID | Severity | Type | Description | Authority |
|----|----------|------|-------------|-----------|
| API-A1 | Medium | Tech debt | `EnrichmentConfig` on CRD not consumed by controller | Documentation |
| API-A2 | Low | Tech debt | `SignalData.Type` deprecation comment-only | Documentation |
| API-A3 | Low | Spec test | CRD validation — regression guard for kubebuilder enum | Checkpoint 8 |

#### Dimension 7: Spec / Documentation Compliance

| ID | Severity | Type | Description | Authority |
|----|----------|------|-------------|-----------|
| D1 | Medium | Tech debt | DD-SP-003 TODO contradicts existing file | Documentation accuracy |
| D2 | Medium | Tech debt | `BR-SP-XXX` placeholder in StatusManager comment | Documentation accuracy |
| D3 | Medium | Inconsistency | Integration suite podman-compose vs guideline preference | TESTING_GUIDELINES v2.5.2 |

#### Moved / Reclassified

| ID | Severity | Type | Description | Authority |
|----|----------|------|-------------|-----------|
| E3 | Medium | Gap | Recorder nil check — testing ergonomics (prod always wired) | Moved to Phase 6 |

### 4.3 Descoped (1 finding)

| ID | Reason |
|----|--------|
| E4 | CRD kubebuilder enum prevents invalid phases at K8s admission. User decision. |

### 4.4 Design Decisions

| Decision | Rationale |
|----------|-----------|
| O2 uses existing `RecordPhaseTransition` | `signalprocessing.signal.received` is NOT in the OpenAPI enum. Phase transition `"" -> Pending` satisfies BR-SP-090 per ADR-034. |
| O5 enforced at SetupWithManager + Reconcile | Defense in depth. AA has same bug (Issue #1116). |
| BLAST-A1/A2 require new BR | "Capture and expose all labels, let Rego decide" — BR creation is Phase 2 prerequisite. |
| S3 corrected from audit | Original audit referenced `ReasonAuditWriteNever` which does not exist. Actual constant is `ReasonAuditWriteFailed`. |
| Private functions tested via Reconcile | `classifyBusiness`, `isTransientError` are unexported — tested through shaped inputs. |
| Finding ID disambiguation | Prefix per dimension: `BLAST-` (Dim 1), `CONC-` (Dim 3), `API-` (Dim 5) to avoid collisions. |

---

## 5. Approach

### 5.1 Coverage Policy

| Tier | Code Subset | Coverage Target |
|------|-------------|-----------------|
| Unit | Unit-testable SP code (pure logic) | >=80% |
| Integration | Integration-testable SP code (I/O) | >=80% |
| E2E | Full SP service | >=80% (existing) |
| All Tiers | Merged line-by-line dedup | >=80% |

### 5.2 TDD Methodology

Every finding follows strict **RED -> GREEN -> REFACTOR**:

1. **TDD RED**: Write failing test(s) that define the expected behavior. Tests MUST fail because the production code does not yet implement the behavior.
2. **TDD GREEN**: Write minimal production code to make the test(s) pass. No sophistication.
3. **TDD REFACTOR**: Improve code quality. Validate against [100 Go Mistakes](https://github.com/teivah/100-go-mistakes).

### 5.3 Pass/Fail Criteria

**PASS**: All of the following:
1. All tests pass (0 failures)
2. `go build ./...` succeeds
3. `go test -race ./test/unit/signalprocessing/...` passes
4. No regressions in existing test suites
5. All 9-category checkpoints satisfied at each phase boundary

**FAIL**: Any of the following:
1. Any test fails
2. Build errors introduced
3. Existing tests regress
4. Any checkpoint category unsatisfied without documented escalation

### 5.4 Suspension & Resumption Criteria

**Suspend**: BLAST-A1/A2 implementation if BR creation reveals design conflicts with BR-SP-080
**Resume**: When BR wording is finalized and approved

---

## 6. Test Items

### 6.1 Production Files Under Test

| File | Key Functions/Methods | Findings |
|------|----------------------|----------|
| `internal/controller/signalprocessing/signalprocessing_controller.go` | `Reconcile`, `reconcileEnriching`, `reconcileClassifying`, `reconcileCategorizing`, `classifyBusiness`, `isTransientError`, `resetConsecutiveFailures`, `SetupWithManager` | E1, E2, E3, E5, E6, O1-O4, O5, O6, O7, BLAST-A1, BLAST-A2, BLAST-A3, BLAST-B2, CONC-C1, S1, S2 |
| `pkg/signalprocessing/enricher/k8s_enricher.go` | `Enrich`, `enrichNodeSignal`, `enrichNamespaceOnly` | BLAST-A4, BLAST-A5 |
| `pkg/signalprocessing/enricher/degraded.go` | `BuildDegradedContext` | BLAST-B1, D4 |
| `pkg/signalprocessing/evaluator/evaluator.go` | `EvaluateEnvironment`, `EvaluateSeverity` | E7 |
| `pkg/signalprocessing/cache/cache.go` | `Set`, `Get`, `Delete`, `Len` | CONC-C3 |
| `pkg/signalprocessing/audit/client.go` | Event type constants, `RecordClassificationDecision`, `RecordError` | O1, O5 |
| `pkg/signalprocessing/audit/manager.go` | `RecordPhaseTransition`, `RecordError`, `RecordCompletion` | O2 |
| `pkg/signalprocessing/conditions.go` | `SetCategorizationComplete`, `SetProcessingComplete`, `SetClassificationComplete` | S1, S3, S4 |
| `pkg/signalprocessing/ownerchain/builder.go` | `isClusterScoped` (unexported) | BLAST-A3, BLAST-A5 |
| `pkg/shared/events/reasons.go` | `EventReasonEnrichmentDegraded`, etc. | O7 |
| `pkg/signalprocessing/metrics/metrics.go` | `IncrementProcessingTotal`, `ObserveProcessingDuration`, `RecordEnrichmentError` | O3, O4 |
| `pkg/signalprocessing/status/manager.go` | `AtomicStatusUpdate` | CONC-C4, D2 |

### 6.2 Test Files to Extend

| Test File | Findings Covered |
|-----------|-----------------|
| `test/unit/signalprocessing/controller_error_handling_test.go` | E1, E2, E5, E6, O5 |
| `test/unit/signalprocessing/controller_reconciliation_test.go` | BLAST-A1, BLAST-A2, BLAST-A3, BLAST-B2, O2, O4, S1, S2 |
| `test/unit/signalprocessing/enricher_resource_types_test.go` | BLAST-A4, BLAST-A5, BLAST-B1 |
| `test/unit/signalprocessing/degraded_test.go` | BLAST-B1, D4 |
| `test/unit/signalprocessing/evaluator/evaluator_test.go` | E7 |
| `test/unit/signalprocessing/audit_client_test.go` | O1, O5 |
| `test/unit/signalprocessing/metrics_test.go` | O3, O4 |
| `test/unit/signalprocessing/controller_events_test.go` | O6, O7, E3 |
| `test/unit/signalprocessing/cache_test.go` | CONC-C3, CONC-C4 |
| `test/unit/signalprocessing/conditions_test.go` | S1, S3, S4 |
| `test/integration/signalprocessing/reconciler_integration_test.go` | CONC-C1, CONC-C2 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/1110-sp-readiness-audit` HEAD (`72b84b2b6`) | Rebased on origin/main |
| Base commit | `72b84b2b6` | Last #1111 commit |

---

## 7. BR Coverage Matrix

| BR / Authority | Description | Finding(s) | Test ID(s) | Status |
|----------------|-------------|------------|------------|--------|
| BR-SP-111 | Reset failure counter on successful phase transition | E1 | UT-SP-1110-001 to 003 | Pending |
| Go standards | `errors.Is` for wrapped sentinel errors | E2 | UT-SP-1110-004 to 006 | Pending |
| 00-core-rules + DD-005 | Every error must be logged | E5 | UT-SP-1110-007 to 009 | Pending |
| ADR-032 | No silent audit failure skips | E6 | UT-SP-1110-010, 011 | Pending |
| Consistency | Rego eval timeout parity | E7 | UT-SP-1110-012, 013 | Pending |
| Defense in depth | PolicyEvaluator nil = fail-fast | O5 | UT-SP-1110-014 to 016 | Pending |
| New BR-SP-XXX | Workload label exposure for Rego | BLAST-A1, A2 | UT-SP-1110-017 to 022 | Pending |
| BR-SP-070 / #437 | Cluster-scoped guard | BLAST-A3 | UT-SP-1110-023, 024 | Pending |
| DD-017 P3 | Enricher degraded for Node NotFound | BLAST-A4 | UT-SP-1110-025, 026 | Pending |
| DD-017 P3 | Enricher cluster-scoped kind handling | BLAST-A5 | UT-SP-1110-027, 028 | Pending |
| DD-SP-002 | BuildDegradedContext cluster-scoped semantics | BLAST-B1 | UT-SP-1110-029 | Pending |
| Hardening | Log format for empty namespace | BLAST-B2 | UT-SP-1110-030 | Pending |
| Checkpoint 5 | Nil metrics panic spec test | D4 | UT-SP-1110-031 | Pending |
| BR-SP-090 + OpenAPI | Classification error audit | O1 | UT-SP-1110-032, 033 | Pending |
| BR-SP-090 | Phase transition "" -> Pending | O2 | UT-SP-1110-034 | Pending |
| DD-005 | PhaseFailed completed/failure metrics | O3 | UT-SP-1110-035, 036 | Pending |
| Hardening | Early exit metrics | O4 | UT-SP-1110-037 to 039 | Pending |
| DD-EVENT-001 | K8s events on enrichment failure | O6 | UT-SP-1110-040, 041 | Pending |
| DD-EVENT-001 | Constants vs string literals | O7 | UT-SP-1110-042 | Pending |
| Documentation | MaxConcurrentReconciles explicit | CONC-C1 | IT-SP-1110-001 | Pending |
| Concurrency | TOCTOU race demonstration | CONC-C2 | IT-SP-1110-002 | Pending |
| BR-SP-001 | TTLCache bounded growth | CONC-C3 | UT-SP-1110-043 to 045 | Pending |
| DD-PERF-001 | AtomicStatusUpdate serialization | CONC-C4 | UT-SP-1110-046 | Pending |
| DD-SP-002 | PhaseFailed all conditions set | S1 | UT-SP-1110-047 to 049 | Pending |
| DD-SP-002 | Persistent classification -> PhaseFailed | S2 | UT-SP-1110-050, 051 | Pending |
| DD-SP-002 | ReasonAuditWriteFailed used in production | S3 | UT-SP-1110-052 | Pending |
| DD-SP-002 + BR-SP-110 | SetCategorizationComplete(false) called | S4 | UT-SP-1110-053 | Pending |
| TESTING_GUIDELINES | Test assertions strengthened | T1 | T1-REFACTOR | Pending |
| Phase 2 dep | Cluster-scoped tests exist | T2 | T2-REFACTOR | Pending |
| TESTING_GUIDELINES v2.0 | time.Sleep removed | T3 | T3-REFACTOR | Pending |
| Test hygiene | Duplicate assertion removed | T4 | T4-REFACTOR | Pending |
| DD-TEST-006 | Test ID naming consistency | T5 | T5-REFACTOR | Pending |
| DD-TEST-006 | Test plan doc alignment | T6 | T6-REFACTOR | Pending |
| Testing ergonomics | Recorder nil check | E3 | UT-SP-1110-054 | Pending |
| Documentation | EnrichmentConfig unused | API-A1 | API-A1-DOC | Pending |
| Documentation | SignalData.Type deprecation | API-A2 | API-A2-DOC | Pending |
| Checkpoint 8 | CRD enum regression guard | API-A3 | UT-SP-1110-055 | Pending |
| Documentation | DD-SP-003 TODO removal | D1 | D1-DOC | Pending |
| Documentation | BR-SP-XXX placeholder fix | D2 | D2-DOC | Pending |
| TESTING_GUIDELINES | podman-compose note | D3 | D3-DOC | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: `SP` (Signal Processing)
- **ISSUE**: `1110`
- **SEQUENCE**: Zero-padded 3-digit

---

## 9. Implementation Phases

### Phase 1: Error Handling, Resilience, and Startup Validation

**Findings**: E1, E2, E5, E6, E7, O5
**Dependencies**: None (foundational)

#### Phase 1 — TDD RED (Write Failing Tests)

| Test ID | Business Outcome Under Test | File | Status |
|---------|---------------------------|------|--------|
| `UT-SP-1110-001` | After successful phase transition with `RequeueAfter > 0`, `ConsecutiveFailures` resets to 0 | `controller_error_handling_test.go` | Pending |
| `UT-SP-1110-002` | After successful phase transition with `Requeue == true`, `ConsecutiveFailures` resets to 0 | `controller_error_handling_test.go` | Pending |
| `UT-SP-1110-003` | After failed reconcile (err != nil), `ConsecutiveFailures` does NOT reset | `controller_error_handling_test.go` | Pending |
| `UT-SP-1110-004` | `isTransientError(fmt.Errorf("wrapped: %w", context.DeadlineExceeded))` returns true | `controller_error_handling_test.go` | Pending |
| `UT-SP-1110-005` | `isTransientError(fmt.Errorf("wrapped: %w", context.Canceled))` returns true | `controller_error_handling_test.go` | Pending |
| `UT-SP-1110-006` | `isTransientError(context.DeadlineExceeded)` returns true (direct sentinel) | `controller_error_handling_test.go` | Pending |
| `UT-SP-1110-007` | Every error return in `reconcileEnriching` includes structured log with resource name, namespace, phase | `controller_error_handling_test.go` | Pending |
| `UT-SP-1110-008` | Every error return in `reconcileClassifying` includes structured log with resource name, namespace, phase | `controller_error_handling_test.go` | Pending |
| `UT-SP-1110-009` | Every error return in `reconcileCategorizing` includes structured log with resource name, namespace, phase | `controller_error_handling_test.go` | Pending |
| `UT-SP-1110-010` | `RecordError` return value is checked (not `_ =`) | `controller_error_handling_test.go` | Pending |
| `UT-SP-1110-011` | When `RecordError` fails, error is logged with context | `controller_error_handling_test.go` | Pending |
| `UT-SP-1110-012` | `EvaluateEnvironment` respects context timeout (cancels within `regoEvalTimeout`) | `evaluator_test.go` | Pending |
| `UT-SP-1110-013` | `EvaluateSeverity` respects context timeout (cancels within `regoEvalTimeout`) | `evaluator_test.go` | Pending |
| `UT-SP-1110-014` | `SetupWithManager` returns error when `PolicyEvaluator` is nil | `controller_error_handling_test.go` | Pending |
| `UT-SP-1110-015` | `Reconcile` returns permanent error when `PolicyEvaluator` is nil | `controller_error_handling_test.go` | Pending |
| `UT-SP-1110-016` | `SetupWithManager` succeeds when `PolicyEvaluator` is non-nil | `controller_error_handling_test.go` | Pending |

#### Phase 1 — TDD GREEN (Implement to Pass)

Production files to modify:
- `internal/controller/signalprocessing/signalprocessing_controller.go`: Fix E1 (line 211), E2 (line 1051), E5 (multiple), E6 (multiple), O5 (SetupWithManager + Reconcile guard)
- `pkg/signalprocessing/evaluator/evaluator.go`: Fix E7 (add `context.WithTimeout` to `EvaluateEnvironment` and `EvaluateSeverity`)

#### Phase 1 — TDD REFACTOR

100 Go Mistakes checklist:
- **#1 Variable shadowing**: Check `err` reuse in `reconcileClassifying` and `reconcileEnriching`
- **#53 Not handling defer errors**: Check `AtomicStatusUpdate` closures
- **#77 Not using errors.Is/As**: Verify all error comparisons use `errors.Is` (E2 fix)
- **#79 Not wrapping errors**: Ensure all returned errors include context via `fmt.Errorf("...: %w", err)`

#### Checkpoint 1 (after Phase 1 Refactor)

| # | Category | Satisfied By | Status |
|---|----------|-------------|--------|
| 1 | Observability wiring | UT-SP-1110-010/011 prove `RecordError` is called and return checked; existing `metrics_test.go` covers processing_total | Pending |
| 2 | Adversarial inputs | UT-SP-1110-006 tests direct sentinel; 004/005 test wrapped errors; E5 tests verify log context on errors with empty/nil fields | Pending |
| 3 | Resource bounds | Deferred to Phase 4 checkpoint (CONC-C3) | N/A |
| 4 | Concurrency | Deferred to Phase 4 checkpoint (CONC-C2/C4) | N/A |
| 5 | Nil/zero edge cases | UT-SP-1110-014/015 test nil PolicyEvaluator; E5 tests exercise nil KubernetesContext error paths | Pending |
| 6 | Error-path observability | UT-SP-1110-007/008/009 verify every error return has structured log with resource name, namespace, phase | Pending |
| 7 | Cross-phase integration | N/A (Phase 1 is foundational) | N/A |
| 8 | Spec compliance | UT-SP-1110-012/013 verify Rego eval timeout matches `regoEvalTimeout` constant | Pending |
| 9 | API surface hygiene | Verify no test helpers exported from `pkg/signalprocessing/evaluator/` | Pending |

---

### Phase 2: Cluster-Scoped Resource Handling

**Findings**: BLAST-A1, BLAST-A2, BLAST-A3, BLAST-A4, BLAST-A5, BLAST-B1, BLAST-B2, D4
**Dependencies**: Phase 1 (E2 fix for `isTransientError`)
**Prerequisite**: Write BR-SP-XXX for label exposure strategy

#### Phase 2 — Prerequisite: BR Creation

Before TDD begins, create BR-SP-XXX in `docs/services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md`:
- Title: "Workload Label Exposure for Cluster-Scoped Resources"
- Contract: "SP controller MUST populate `KubernetesContext` with all available label sources (namespace labels AND workload labels). Rego policies decide classification priority. When namespace is nil (cluster-scoped resources), workload labels are the sole label source."
- Distinguishes workload labels (safe) from signal labels (forbidden per BR-SP-080).

#### Phase 2 — TDD RED (Write Failing Tests)

| Test ID | Business Outcome Under Test | File | Status |
|---------|---------------------------|------|--------|
| `UT-SP-1110-017` | `classifyBusiness` extracts BusinessUnit from workload labels when Namespace is nil | `controller_reconciliation_test.go` | Pending |
| `UT-SP-1110-018` | `classifyBusiness` extracts ServiceOwner from workload labels when Namespace is nil | `controller_reconciliation_test.go` | Pending |
| `UT-SP-1110-019` | `classifyBusiness` prefers namespace labels over workload labels when both exist | `controller_reconciliation_test.go` | Pending |
| `UT-SP-1110-020` | CustomLabels fallback extracts `team`/`tier`/`cost-center`/`region` from workload labels when Namespace nil | `controller_reconciliation_test.go` | Pending |
| `UT-SP-1110-021` | CustomLabels fallback prefers Rego evaluator result over any label fallback | `controller_reconciliation_test.go` | Pending |
| `UT-SP-1110-022` | CustomLabels fallback prefers namespace labels over workload labels | `controller_reconciliation_test.go` | Pending |
| `UT-SP-1110-023` | #437 guard does not requeue for cluster-scoped resources with nil Namespace and EnrichmentComplete=True | `controller_reconciliation_test.go` | Pending |
| `UT-SP-1110-024` | #437 guard still requeues for namespaced resources with nil Namespace and EnrichmentComplete=False | `controller_reconciliation_test.go` | Pending |
| `UT-SP-1110-025` | `enrichNodeSignal` returns degraded context (not error) when Node is NotFound | `enricher_resource_types_test.go` | Pending |
| `UT-SP-1110-026` | `enrichNodeSignal` returns error for non-NotFound errors (e.g., RBAC denied) | `enricher_resource_types_test.go` | Pending |
| `UT-SP-1110-027` | `Enrich` for cluster-scoped kind (e.g., PersistentVolume) with empty namespace succeeds with partial context | `enricher_resource_types_test.go` | Pending |
| `UT-SP-1110-028` | `Enrich` for cluster-scoped kind does NOT call `enrichNamespaceOnly` | `enricher_resource_types_test.go` | Pending |
| `UT-SP-1110-029` | `BuildDegradedContext` for cluster-scoped signal sets Namespace to nil (not empty name) | `degraded_test.go` | Pending |
| `UT-SP-1110-030` | Log message for enrichment includes `Kind Name` format (no leading slash) when namespace is empty | `controller_reconciliation_test.go` | Pending |
| `UT-SP-1110-031` | `NewK8sEnricher` with nil metrics panics (spec test documenting fail-fast) | `degraded_test.go` | Pending |

#### Phase 2 — TDD GREEN (Implement to Pass)

Production files to modify:
- `internal/controller/signalprocessing/signalprocessing_controller.go`: Fix BLAST-A1 (line 903), BLAST-A2 (line 384), BLAST-A3 (line 527), BLAST-B2 (log format)
- `pkg/signalprocessing/enricher/k8s_enricher.go`: Fix BLAST-A4 (line 353), BLAST-A5 (line 369)
- `pkg/signalprocessing/enricher/degraded.go`: Fix BLAST-B1 (line 41)
- `pkg/signalprocessing/ownerchain/builder.go`: Export `IsClusterScoped` if needed for BLAST-A3/A5

#### Phase 2 — TDD REFACTOR

100 Go Mistakes checklist:
- **#5 Interface pollution**: Verify enricher interfaces are minimal
- **#26 Slices and memory leaks**: Check label map copies in `classifyBusiness`
- **#88 Not properly initializing maps**: Verify `BuildDegradedContext` initializes maps before use

#### Checkpoint 2 (after Phase 2 Refactor)

| # | Category | Satisfied By | Status |
|---|----------|-------------|--------|
| 1 | Observability wiring | UT-SP-1110-025 verifies enrichment error metric fires on Node NotFound | Pending |
| 2 | Adversarial inputs | UT-SP-1110-027 tests empty namespace string; 029 tests nil Namespace pointer; 031 tests nil metrics | Pending |
| 3 | Resource bounds | Deferred to Phase 4 | N/A |
| 4 | Concurrency | Deferred to Phase 4 | N/A |
| 5 | Nil/zero edge cases | UT-SP-1110-017/018 (nil Namespace), 025 (NotFound node), 029 (nil Namespace in degraded), 031 (nil metrics panic) | Pending |
| 6 | Error-path observability | UT-SP-1110-026 verifies RBAC error includes resource name/kind in log; 030 verifies log format | Pending |
| 7 | Cross-phase integration | UT-SP-1110-023/024 prove Phase 1 `isTransientError` fix (E2) is wired into Phase 2 enrichment error handling | Pending |
| 8 | Spec compliance | UT-SP-1110-029 verifies `DegradedMode=true` per DD-SP-002; 027 verifies cluster-scoped kinds match `ownerchain.IsClusterScoped` | Pending |
| 9 | API surface hygiene | Verify `isClusterScoped` export decision (export only if test access requires it) | Pending |

---

### Phase 3: Observability Gaps

**Findings**: O1, O2, O3, O4, O6, O7
**Dependencies**: Phase 1 (E6 fix for audit error handling), Phase 2 (enrichment paths for O6)

#### Phase 3 — TDD RED (Write Failing Tests)

| Test ID | Business Outcome Under Test | File | Status |
|---------|---------------------------|------|--------|
| `UT-SP-1110-032` | `EvaluateEnvironment` failure emits `signalprocessing.error.occurred` audit event | `audit_client_test.go` | Pending |
| `UT-SP-1110-033` | `EvaluateSeverity` failure emits `signalprocessing.error.occurred` audit event with phase="Classifying" | `audit_client_test.go` | Pending |
| `UT-SP-1110-034` | First reconcile (`phase=""`) emits `RecordPhaseTransition` with `from=""`, `to="Pending"` | `controller_reconciliation_test.go` | Pending |
| `UT-SP-1110-035` | Transition to `PhaseFailed` increments `signalprocessing_processing_total{phase="completed",result="failure"}` | `metrics_test.go` | Pending |
| `UT-SP-1110-036` | Transition to `PhaseFailed` records `ObserveProcessingDuration("completed", ...)` | `metrics_test.go` | Pending |
| `UT-SP-1110-037` | `#437 requeue` path increments `signalprocessing_processing_total{phase="classifying",result="requeue"}` | `metrics_test.go` | Pending |
| `UT-SP-1110-038` | `FreshGet` failure path increments `signalprocessing_enrichment_errors_total{error_type="fresh_get_failed"}` | `metrics_test.go` | Pending |
| `UT-SP-1110-039` | Duplicate reconcile (ObservedGeneration match) records metric | `metrics_test.go` | Pending |
| `UT-SP-1110-040` | Hard enrichment failure emits K8s Warning event with `EventReasonEnrichmentDegraded` | `controller_events_test.go` | Pending |
| `UT-SP-1110-041` | Transient enrichment error emits K8s Warning event | `controller_events_test.go` | Pending |
| `UT-SP-1110-042` | `UnsupportedTargetType` event uses `corev1.EventTypeWarning` and constant from `pkg/shared/events/reasons.go` | `controller_events_test.go` | Pending |

#### Phase 3 — TDD GREEN (Implement to Pass)

Production files to modify:
- `internal/controller/signalprocessing/signalprocessing_controller.go`: Add `RecordError` calls for O1, `RecordPhaseTransition` for O2, `IncrementProcessingTotal("completed","failure")` for O3, early-exit metrics for O4, events for O6, constants for O7
- `pkg/signalprocessing/audit/manager.go`: No changes needed (existing `RecordPhaseTransition` suffices for O2)

#### Phase 3 — TDD REFACTOR

100 Go Mistakes checklist:
- **#54 Not handling errors in goroutines**: Verify audit `RecordError` calls in goroutine paths
- **#84 Not using time.Duration**: Ensure duration calculations use `time.Since` not manual subtraction

#### Checkpoint 3 (after Phase 3 Refactor)

| # | Category | Satisfied By | Status |
|---|----------|-------------|--------|
| 1 | Observability wiring | UT-SP-1110-035/036 prove PhaseFailed fires `processing_total` + `processing_duration`; 037-039 prove early exits fire metrics; 032/033 prove error audit fires | Pending |
| 2 | Adversarial inputs | Covered by Phase 1/2 checkpoints | N/A |
| 3 | Resource bounds | Deferred to Phase 4 | N/A |
| 4 | Concurrency | Deferred to Phase 4 | N/A |
| 5 | Nil/zero edge cases | UT-SP-1110-034 exercises empty-string phase (""); 040/041 exercise nil enrichment result | Pending |
| 6 | Error-path observability | UT-SP-1110-032/033 verify error audit includes phase string; 040/041 verify K8s event includes error message | Pending |
| 7 | Cross-phase integration | UT-SP-1110-035 proves Phase 1 PhaseFailed path (S1 dependency) wires to Phase 3 metrics; UT-SP-1110-040 proves Phase 2 enrichment failure triggers Phase 3 K8s event | Pending |
| 8 | Spec compliance | UT-SP-1110-032/033 verify event_type matches OpenAPI enum `signalprocessing.error.occurred`; 042 verifies DD-EVENT-001 constants | Pending |
| 9 | API surface hygiene | No new exports added in Phase 3 | Pending |

---

### Phase 4: Concurrency and Resource Bounds

**Findings**: CONC-C1, CONC-C2, CONC-C3, CONC-C4
**Dependencies**: Phase 1 (O5 SetupWithManager validation)

#### Phase 4 — TDD RED (Write Failing Tests)

| Test ID | Business Outcome Under Test | File | Status |
|---------|---------------------------|------|--------|
| `UT-SP-1110-043` | TTLCache with 50+ Set/Delete cycles does not grow beyond max entries | `cache_test.go` | Pending |
| `UT-SP-1110-044` | TTLCache evicts expired entries during Set when at capacity | `cache_test.go` | Pending |
| `UT-SP-1110-045` | TTLCache Get/Set/Delete under 10+ concurrent goroutines with `-race` produces no races | `cache_test.go` | Pending |
| `UT-SP-1110-046` | `AtomicStatusUpdate` with 10+ concurrent goroutines serializes writes (no data loss) | `cache_test.go` | Pending |
| `IT-SP-1110-001` | `SetupWithManager` sets `MaxConcurrentReconciles` explicitly (verified via controller options) | `reconciler_integration_test.go` | Pending |
| `IT-SP-1110-002` | Two concurrent reconciles on the same SP CR demonstrate TOCTOU: read stale -> AtomicStatusUpdate refetches | `reconciler_integration_test.go` | Pending |

#### Phase 4 — TDD GREEN (Implement to Pass)

Production files to modify:
- `pkg/signalprocessing/cache/cache.go`: Add `maxEntries` field, `DefaultMaxEntries` constant, eviction in `Set`
- `internal/controller/signalprocessing/signalprocessing_controller.go`: Add `WithOptions(controller.Options{MaxConcurrentReconciles: 1})` in `SetupWithManager`

#### Phase 4 — TDD REFACTOR

100 Go Mistakes checklist:
- **#58 Not understanding race conditions**: Verify TTLCache lock discipline
- **#9 Generics**: Consider `TTLCache[V any]` instead of `interface{}` (document decision if deferred)
- **#26 Slices and memory leaks**: Verify evicted entries are GC-eligible
- **#88 Map initialization**: Ensure `NewTTLCache` pre-allocates map with capacity hint

#### Checkpoint 4 (after Phase 4 Refactor)

| # | Category | Satisfied By | Status |
|---|----------|-------------|--------|
| 1 | Observability wiring | Covered by Phase 3 | N/A |
| 2 | Adversarial inputs | UT-SP-1110-043 tests 50+ lifecycle cycles (stress); 045 tests concurrent adversarial access | Pending |
| 3 | Resource bounds | **UT-SP-1110-043**: 50+ create/delete cycles assert `Len() <= maxEntries`; **044**: expired entries evicted on capacity | Pending |
| 4 | Concurrency | **UT-SP-1110-045**: 10+ goroutines on TTLCache under `-race`; **046**: 10+ goroutines on AtomicStatusUpdate; **IT-SP-1110-002**: TOCTOU race demonstration | Pending |
| 5 | Nil/zero edge cases | Covered by Phase 1/2 | N/A |
| 6 | Error-path observability | Covered by Phase 1/3 | N/A |
| 7 | Cross-phase integration | IT-SP-1110-001 proves Phase 1 SetupWithManager (O5) also wires MaxConcurrentReconciles | Pending |
| 8 | Spec compliance | Covered by Phase 1/3 | N/A |
| 9 | API surface hygiene | Verify `DefaultMaxEntries` is appropriately exported (needed by tests); no debug functions exported | Pending |

---

### Phase 5: State Machine and Conditions

**Findings**: S1, S2, S3, S4
**Dependencies**: Phase 3 (O3 metrics for PhaseFailed)

#### Phase 5 — TDD RED (Write Failing Tests)

| Test ID | Business Outcome Under Test | File | Status |
|---------|---------------------------|------|--------|
| `UT-SP-1110-047` | Severity failure -> PhaseFailed sets `ProcessingComplete=False` with `ReasonProcessingFailed` | `conditions_test.go` | Pending |
| `UT-SP-1110-048` | Severity failure -> PhaseFailed sets `CategorizationComplete=False` with `ReasonCategorizationFailed` | `conditions_test.go` | Pending |
| `UT-SP-1110-049` | Severity failure -> PhaseFailed sets `ClassificationComplete=False`, `Ready=False` (existing), AND `ProcessingComplete=False` + `CategorizationComplete=False` (new) | `controller_reconciliation_test.go` | Pending |
| `UT-SP-1110-050` | Non-transient `EvaluateEnvironment` error transitions from Classifying to PhaseFailed (not infinite requeue) | `controller_reconciliation_test.go` | Pending |
| `UT-SP-1110-051` | Non-transient `EvaluatePriority` error transitions from Classifying to PhaseFailed | `controller_reconciliation_test.go` | Pending |
| `UT-SP-1110-052` | `ReasonAuditWriteFailed` is used in production when audit write fails during `reconcileCategorizing` | `controller_reconciliation_test.go` | Pending |
| `UT-SP-1110-053` | Categorization failure calls `SetCategorizationComplete(sp, false, ReasonCategorizationFailed, ...)` | `controller_reconciliation_test.go` | Pending |

#### Phase 5 — TDD GREEN (Implement to Pass)

Production files to modify:
- `internal/controller/signalprocessing/signalprocessing_controller.go`: Add `SetCategorizationComplete(false)` and `SetProcessingComplete(false)` to PhaseFailed path (S1); add terminal transition for non-transient classification errors (S2); use `ReasonAuditWriteFailed` on audit failure (S3); call `SetCategorizationComplete(false)` on categorization failure (S4)

#### Phase 5 — TDD REFACTOR

100 Go Mistakes checklist:
- **#2 Unnecessary nested code**: Flatten PhaseFailed condition-setting into helper function if 4+ conditions
- **#1 Variable shadowing**: Check `status` variable in `SetCategorizationComplete` (the source shows `:=` shadow at line 226)

#### Checkpoint 5 (after Phase 5 Refactor)

| # | Category | Satisfied By | Status |
|---|----------|-------------|--------|
| 1 | Observability wiring | UT-SP-1110-052 proves audit write failure triggers `ReasonAuditWriteFailed` condition (ties audit to conditions) | Pending |
| 2 | Adversarial inputs | Covered by Phase 1/2 | N/A |
| 3 | Resource bounds | Covered by Phase 4 | N/A |
| 4 | Concurrency | Covered by Phase 4 | N/A |
| 5 | Nil/zero edge cases | UT-SP-1110-050/051 exercise nil PolicyEvaluator result paths | Pending |
| 6 | Error-path observability | UT-SP-1110-050/051 verify classification failure includes error context in Status.Error; 052 verifies audit failure reason propagated | Pending |
| 7 | Cross-phase integration | UT-SP-1110-049 proves Phase 3 PhaseFailed metrics (O3) fire WITH Phase 5 all-conditions-set (S1); UT-SP-1110-050 proves Phase 1 isTransientError (E2) used to distinguish transient vs permanent | Pending |
| 8 | Spec compliance | UT-SP-1110-047/048 verify DD-SP-002 lifecycle: FAILED state sets ProcessingComplete=False + phase-specific condition False | Pending |
| 9 | API surface hygiene | No new exports | Pending |

---

### Phase 6: Test Quality, API Surface, Documentation

**Findings**: T1-T6, E3, API-A1, API-A2, API-A3, D1-D3
**Dependencies**: Phase 2 (T2 depends on cluster-scoped tests), Phase 5 (all conditions defined)

#### Phase 6 — TDD RED (Write Failing Tests)

| Test ID | Business Outcome Under Test | File | Status |
|---------|---------------------------|------|--------|
| `UT-SP-1110-054` | Reconcile with nil Recorder does not panic on `UnsupportedTargetType` path | `controller_events_test.go` | Pending |
| `UT-SP-1110-055` | CRD phase enum matches `Pending;Enriching;Classifying;Categorizing;Completed;Failed` (regression guard) | `conditions_test.go` | Pending |

#### Phase 6 — TDD GREEN (Implement to Pass)

Production files to modify:
- `internal/controller/signalprocessing/signalprocessing_controller.go`: Add `if r.Recorder != nil` guard (E3)
- `pkg/signalprocessing/status/manager.go`: Replace `BR-SP-XXX` with `BR-SP-110` (D2)

#### Phase 6 — Refactoring Tasks (No New Tests)

| Task ID | Description | File | Status |
|---------|------------|------|--------|
| T1-REFACTOR | Strengthen integration test assertions to match claimed BR coverage | `reconciler_integration_test.go` | Pending |
| T2-REFACTOR | Confirm cluster-scoped tests exist (from Phase 2) | Various | Pending |
| T3-REFACTOR | Replace all `time.Sleep` with `Eventually` per TESTING_GUIDELINES v2.0 | Various test files | Pending |
| T4-REFACTOR | Remove duplicate `Expect(result.RequeueAfter).To(BeZero())` assertion | `controller_reconciliation_test.go` | Pending |
| T5-REFACTOR | Align test IDs to `UT-SP-*` / `IT-SP-*` convention per DD-TEST-006 | Various test files | Pending |
| T6-REFACTOR | Update test plan doc references for consistency | Various docs | Pending |
| API-A1-DOC | Add comment documenting `EnrichmentConfig` is reserved for future use | CRD types file | Pending |
| API-A2-DOC | Add deprecation annotation to `SignalData.Type` | CRD types file | Pending |
| D1-DOC | Remove stale DD-SP-003 TODO that contradicts existing file | Controller | Pending |
| D2-DOC | Replace `BR-SP-XXX` placeholder with `BR-SP-110` | Status manager | Pending |
| D3-DOC | Add note about programmatic Go vs podman-compose per TESTING_GUIDELINES v2.5.2 | Integration suite | Pending |

#### Checkpoint 6 (Final — after Phase 6 Refactor)

| # | Category | Satisfied By | Status |
|---|----------|-------------|--------|
| 1 | Observability wiring | All 3 SP metrics have tests asserting value changes (from Phases 1-3) | Pending |
| 2 | Adversarial inputs | Empty string, max-length+1, path traversal, Unicode tested for signal IDs (Phase 1), namespace (Phase 2) | Pending |
| 3 | Resource bounds | TTLCache 50+ cycle test (Phase 4, UT-SP-1110-043) | Pending |
| 4 | Concurrency | TTLCache 10+ goroutines (045), AtomicStatusUpdate 10+ goroutines (046), TOCTOU race (IT-002) | Pending |
| 5 | Nil/zero edge cases | nil Recorder (054), nil PolicyEvaluator (014/015), nil Namespace (017/018/025/029), nil Workload, zero Phase (034), nil metrics (031) | Pending |
| 6 | Error-path observability | Every error return has structured log (007/008/009), audit errors logged (010/011), classification errors audited (032/033) | Pending |
| 7 | Cross-phase integration | Phase 3 metrics prove Phase 1 error paths fire counters; Phase 5 conditions prove Phase 2 enrichment wired; Phase 6 confirms all phases integrated | Pending |
| 8 | Spec compliance | CRD enum regression (055), OpenAPI event_type enum (032/033), DD-EVENT-001 constants (042), K8s RFC 1123 (existing) | Pending |
| 9 | API surface hygiene | No test helpers exported from production `pkg/`; `isClusterScoped` export decision documented; `ReasonAuditWriteFailed` only export if production uses it (S3) | Pending |

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Fake K8s client (`fake.NewClientBuilder()`), mock AuditStore, mock PolicyEvaluator, fake EventRecorder
- **Metrics**: `prometheus.NewPedanticRegistry()` for isolated metric assertions
- **Race detector**: `go test -race` mandatory for Phase 4
- **Location**: `test/unit/signalprocessing/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: envtest for K8s API server
- **Location**: `test/integration/signalprocessing/`

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.23 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| golangci-lint | latest | Lint validation |

---

## 11. Dependencies & Schedule

### 11.1 Phase Dependency Graph

```
Phase 1 (Error Handling) ──┬──> Phase 2 (Cluster-Scoped) ──> Phase 3 (Observability)
                           │                                       │
                           └──> Phase 4 (Concurrency) <────────────┘
                                       │
                                       v
                                Phase 5 (State Machine) ──> Phase 6 (Quality/Docs)
```

### 11.2 Execution Order

1. **Phase 1**: TDD RED -> GREEN -> REFACTOR -> **Checkpoint 1**
2. **Phase 2**: BR prerequisite -> TDD RED -> GREEN -> REFACTOR -> **Checkpoint 2**
3. **Phase 3**: TDD RED -> GREEN -> REFACTOR -> **Checkpoint 3**
4. **Phase 4**: TDD RED -> GREEN -> REFACTOR -> **Checkpoint 4**
5. **Phase 5**: TDD RED -> GREEN -> REFACTOR -> **Checkpoint 5**
6. **Phase 6**: TDD RED -> GREEN -> REFACTOR -> **Checkpoint 6 (Final)**

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/1110/TEST_PLAN.md` | IEEE 829 strategy and test design |
| BR-SP-XXX | `docs/services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md` | Workload label exposure BR |
| Phase 1 tests | `test/unit/signalprocessing/controller_error_handling_test.go`, `evaluator_test.go` | E1-E7, O5 |
| Phase 2 tests | `test/unit/signalprocessing/enricher_resource_types_test.go`, `controller_reconciliation_test.go`, `degraded_test.go` | BLAST-A1 to B2, D4 |
| Phase 3 tests | `test/unit/signalprocessing/audit_client_test.go`, `metrics_test.go`, `controller_events_test.go` | O1-O7 |
| Phase 4 tests | `test/unit/signalprocessing/cache_test.go`, `test/integration/signalprocessing/reconciler_integration_test.go` | CONC-C1 to C4 |
| Phase 5 tests | `test/unit/signalprocessing/conditions_test.go`, `controller_reconciliation_test.go` | S1-S4 |
| Phase 6 tests | `test/unit/signalprocessing/controller_events_test.go`, `conditions_test.go` | E3, API-A3 |

---

## 13. Execution

```bash
# Unit tests (all phases)
go test ./test/unit/signalprocessing/... -ginkgo.v -count=1

# Specific phase tests by ID pattern
go test ./test/unit/signalprocessing/... -ginkgo.focus="UT-SP-1110" -count=1

# Integration tests (Phase 4)
go test ./test/integration/signalprocessing/... -ginkgo.focus="IT-SP-1110" -count=1

# Race detector (Phase 4 mandatory)
go test -race ./test/unit/signalprocessing/... -ginkgo.focus="UT-SP-1110-04[3456]" -count=1

# Build validation
go build ./internal/controller/signalprocessing/... ./pkg/signalprocessing/...

# Lint validation
golangci-lint run --timeout=5m ./internal/controller/signalprocessing/... ./pkg/signalprocessing/...
```

---

## 14. 100 Go Mistakes Refactor Checklist

Validated during each phase's REFACTOR step against the SP codebase:

| # | Mistake | Relevance to SP | Phase |
|---|---------|-----------------|-------|
| 1 | Variable shadowing | `err` reuse in reconcile methods | 1 |
| 2 | Unnecessary nested code | PhaseFailed condition-setting | 5 |
| 5 | Interface pollution | Manager/AuditClient separation | 1 |
| 9 | Generics confusion | TTLCache uses `interface{}` | 4 |
| 26 | Slices/memory leaks | TTLCache expired entry cleanup | 4 |
| 53 | Not handling defer errors | AtomicStatusUpdate closures | 1 |
| 54 | Not handling goroutine errors | Audit RecordError in goroutine paths | 3 |
| 56 | Concurrency not always faster | MaxConcurrentReconciles=1 rationale | 4 |
| 58 | Race conditions | TTLCache, TOCTOU | 4 |
| 73 | Testing utility packages | httptest, fake clients | 6 |
| 77 | Not using errors.Is/As | E2 fix | 1 |
| 79 | Not wrapping errors | Error context via `%w` | 1 |
| 84 | time.Duration misuse | Duration calculations | 3 |
| 88 | Map initialization | TTLCache constructor, BuildDegradedContext | 2, 4 |

---

## 15. Compliance Sign-Off

### Test Execution Summary

| Phase | Tests Written | Tests Passing | Coverage |
|-------|--------------|---------------|----------|
| Phase 1: Error Handling | 0 / 16 | 0 | 0% |
| Phase 2: Cluster-Scoped | 0 / 15 | 0 | 0% |
| Phase 3: Observability | 0 / 11 | 0 | 0% |
| Phase 4: Concurrency | 0 / 6 | 0 | 0% |
| Phase 5: State Machine | 0 / 7 | 0 | 0% |
| Phase 6: Quality/Docs | 0 / 2 | 0 | 0% |
| **Total** | **0 / 57** | **0** | **0%** |

### Checkpoint Sign-Off

| Checkpoint | All 9 Categories | Sign-Off |
|------------|-----------------|----------|
| Checkpoint 1 (Phase 1) | [ ] | Pending |
| Checkpoint 2 (Phase 2) | [ ] | Pending |
| Checkpoint 3 (Phase 3) | [ ] | Pending |
| Checkpoint 4 (Phase 4) | [ ] | Pending |
| Checkpoint 5 (Phase 5) | [ ] | Pending |
| Checkpoint 6 (Final) | [ ] | Pending |

### Approval

| Role | Name | Date | Signature |
|------|------|------|-----------|
| Developer | | | [ ] |
| Reviewer | | | [ ] |
