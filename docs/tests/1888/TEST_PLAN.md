# Test Plan: RESTMapper cache self-heals on lookup failure (#1888)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1888-v1.0
**Feature**: `ResolveGVKForKind`'s REST-mapper fallback retries once via `Reset()` when a lookup
fails or comes back empty, so a CRD kind that missed a single discovery round during AF's lazy
mapper init self-heals on the next lookup instead of staying unresolvable until pod restart.
**Version**: 1.0
**Created**: 2026-08-03
**Author**: AI Assistant
**Status**: Complete
**Branch**: `fix/1888-restmapper-cache-invalidation`

---

## 1. Introduction

### 1.1 Purpose

Issue #1888: AF's `kubectl_get`/`kubectl_list` MCP tools intermittently fail to resolve valid
CRD kinds (e.g., ACM's `search.open-cluster-management.io/Search`) with "cannot resolve GVK for
kind". Root cause (confirmed against live cluster + the exact vendored `k8s.io/client-go@v0.35.7`
source): `cmd/apifrontend/main.go` builds a `restmapper.NewDeferredDiscoveryRESTMapper` wrapping a
`memory.MemCacheClient`. That mapper's built-in self-heal (`!d.cl.Fresh()` gate) only fires
**once**, before the very first successful discovery populate. `memCacheClient.Fresh()` reports
`cacheValid`, which flips to `true` permanently after the first `refreshLocked()` succeeds and is
never invalidated on any TTL. If a CRD's API group is missing from that very first discovery
round (a `*discovery.ErrGroupDiscoveryFailed` for one group, silently discarded by
`restmapper.GetAPIGroupResources` whenever `gs`/`rs` are non-nil), the Kind is permanently
unresolvable until the pod restarts.

This test plan covers the fix: `ResolveGVKForKind`'s REST-mapper fallback now retries once via
`Reset()` on failure/empty result, mirroring the already-shipped `resettableMapper` pattern in
`pkg/kubernautagent/tools/k8s/resolver.go`.

### 1.2 Objectives

1. **Self-heal proven**: A mapper whose first `KindsFor` call returns no match, but whose
   second call (post-`Reset()`) returns the correct GVK, causes `ResolveGVKForKind` to succeed —
   proving the retry path exists and is exercised.
2. **No regression**: All existing `ResolveGVKForKind`/`ResolveGVKWithAPIVersion` behavior
   (static-table kinds, ambiguous-Kind disambiguation, nil-mapper handling, genuine not-found
   errors) is unchanged.
3. **Scope discipline**: The fix touches only `ResolveGVKForKind`'s fallback — not
   `ResolveGVKWithAPIVersion`, whose only production callers (RemediationOrchestrator,
   EffectivenessMonitor) use controller-runtime's `apiutil.NewDynamicRESTMapper`, a different
   implementation not subject to this bug.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `make test-unit-shared` (or targeted `go test ./pkg/shared/k8s/...` via make) |
| Backward compatibility | 0 regressions | All pre-existing `gvk_test.go` / `gvk_apiversion_test.go` cases pass unmodified |
| Retry bounded | No infinite loop / no more than 1 extra discovery round-trip per failed lookup | Code inspection + test asserting `KindsFor` called exactly twice on the failure path |

---

## 2. References

### 2.1 Authority (governing documents)

- Issue #1888: `kubectl_get`/`kubectl_list` fail to resolve GVK for valid CRD kinds
- [DD-K8S-001](../../architecture/decisions/DD-K8S-001-restmapper-cache-invalidation-on-lookup-failure.md): RESTMapper cache invalidation on lookup failure
- Precedent: `pkg/kubernautagent/tools/k8s/resolver.go` (`resettableMapper` retry pattern)

### 2.2 Cross-References

- `pkg/shared/k8s/gvk.go` — code under test
- `pkg/shared/k8s/gvk_test.go` — existing suite, extended here
- Prior related fix: #1275 (kubernaut.ai CRD static-table kinds), #310 (Node ambiguity), #1040
  (`ResolveGVKWithAPIVersion` disambiguation)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|-----------------|------------|
| R1 | Retry fires on every genuinely-invalid Kind, adding a discovery round-trip per bad lookup | Minor extra API-server load on malformed/typo'd Kind requests | Medium | UT-K8S-1888-002 | Bounded to exactly 1 retry (no loop); accepted trade-off, same as the existing `resolver.go` precedent already shipped in this codebase |
| R2 | Retry masks a persistently-broken mapper (discovery permanently down) behind repeated `Reset()` calls | Wasted work, but no correctness risk — final error is still surfaced | Low | UT-K8S-1888-003 | `Reset()` result is still checked; error propagates normally if the retry also fails |
| R3 | Fix applied to wrong function (`ResolveGVKWithAPIVersion` instead of/also) | Fix doesn't address the reported bug, or scope creep into reconciler-only code path | Low | UT-K8S-1888-001 | Confirmed via `Grep`: AF's `kubectl_get.go`/`scope_helpers.go` call only `ResolveGVKForKind`; `ResolveGVKWithAPIVersion` callers use a different, unaffected mapper implementation |

### 3.1 Risk-to-Test Traceability

R1/R2 are behavioral trade-offs inherent to the chosen design (DD-K8S-001), not defects;
UT-K8S-1888-002/003 prove the retry is bounded and errors still propagate correctly.

---

## 4. Scope

### 4.1 Features to be Tested

- **`ResolveGVKForKind` REST-mapper fallback** (`pkg/shared/k8s/gvk.go`): retries once via
  `Reset()` when the initial `KindsFor` call fails or returns no match, then re-resolves.

### 4.2 Features Not to be Tested

- **`ResolveGVKWithAPIVersion`'s explicit-`apiVersion` path**: out of scope — its only production
  callers use a RESTMapper implementation (`apiutil.NewDynamicRESTMapper`) not subject to this
  bug (see DD-K8S-001 §Context for verification).
- **`restmapper.DeferredDiscoveryRESTMapper` itself**: vendored `client-go` code, not modifiable;
  this fix works around its one-shot `Fresh()` gate rather than patching it.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Reactive `Reset()`-on-failure (not periodic TTL) | Mirrors an already-shipped, precedented pattern (`resolver.go`); smallest diff; self-heals on the very next lookup rather than waiting for a timer. See DD-K8S-001 for full alternatives analysis. |
| Type-assert to `meta.ResettableRESTMapper` (apimachinery's own interface) rather than a local interface | Avoids a redundant duplicate type; apimachinery's doc comment explicitly recommends this exact check for delegating mappers. |
| Test IDs reference the issue number (`UT-K8S-1888-*`), not a new BR-XXX doc | Matches established local convention for bug fixes in this file (`UT-K8S-1275-*` for issue #1275) rather than minting a new business requirement document for a defect fix. |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: 100% of the new retry branch (both the "retry succeeds" and "retry also fails" paths).
- **Integration**: N/A — no new wiring; `ResolveGVKForKind` is already called from AF's
  `kubectl_get.go`/`scope_helpers.go` in production. Existing UT coverage of the shared helper is
  the correctness proof; no new cross-component wiring is introduced by this fix.

### 5.2 Two-Tier Minimum — Tier Skip Rationale

Integration tier is skipped for this fix: it is a pure-logic change to an already-integrated,
already-unit-tested helper function with no I/O of its own (the `meta.RESTMapper` it receives is
already exercised via mocks in the existing suite, matching the file's established test style).
No new wiring point is introduced (CHECKPOINT W: not applicable — no new component).

### 5.3 Pass/Fail Criteria

**PASS**:
1. New `UT-K8S-1888-*` tests pass.
2. All pre-existing tests in `gvk_test.go` and `gvk_apiversion_test.go` continue to pass
   unmodified.
3. `go build ./...`, `golangci-lint run`, `go vet ./...` clean.

**FAIL**: any pre-existing test regresses, or the retry loops/recurses unboundedly.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|--------------------|-----------------|
| `pkg/shared/k8s/gvk.go` | `ResolveGVKForKind` (fallback branch) | ~10 (modified) |

### 6.2 Version Identification

| Item | Version/Commit | Notes |
|------|-----------------|-------|
| Code under test | `release/v1.5` HEAD (`fix/1888-restmapper-cache-invalidation`) | |
| `k8s.io/apimachinery` | v0.36.3 (go.mod) | `meta.ResettableRESTMapper` |
| `k8s.io/client-go` | v0.35.7 (go.mod) | `restmapper.DeferredDiscoveryRESTMapper` (root cause, verified against actual vendored source) |

---

## 7. BR Coverage Matrix

Bug fix to already-implemented, already-integrated functionality; tracked via issue #1888 test
IDs per local convention (see §4.3), not a new BR document.

| Reference | Description | Priority | Tier | Test ID | Status |
|-----------|--------------|----------|------|---------|--------|
| #1888 | Self-heal REST-mapper cache on lookup failure | P1 | Unit | UT-K8S-1888-001 | Pass |
| #1888 | Retry is bounded to exactly one extra attempt | P2 | Unit | UT-K8S-1888-002 | Pass |
| #1888 | Error still propagates when retry also fails | P2 | Unit | UT-K8S-1888-003 | Pass |
| #1888 | Non-resettable mappers are unaffected (no panic/no retry attempted) | P2 | Unit | UT-K8S-1888-004 | Pass |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: `pkg/shared/k8s/gvk.go` — 100% of the new retry branch.

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `UT-K8S-1888-001` | A CRD kind that misses the first discovery round resolves successfully on the very next lookup, without requiring a pod restart | Pass |
| `UT-K8S-1888-002` | The retry fires exactly once per failed lookup (no unbounded loop) | Pass |
| `UT-K8S-1888-003` | A mapper that still can't resolve the kind after `Reset()` returns the same "cannot resolve GVK" error as before (no silent success, no panic) | Pass |
| `UT-K8S-1888-004` | A `meta.RESTMapper` that does NOT implement `Reset()` (e.g., `meta.DefaultRESTMapper` used in most existing tests) behaves exactly as before — no retry attempted, no panic from a failed type assertion | Pass |

---

## 9. Test Cases

### UT-K8S-1888-001: Self-heals on next lookup after a missed discovery round

**Priority**: P1
**Type**: Unit
**File**: `pkg/shared/k8s/gvk_test.go`

**Test Steps**:
1. **Given**: A mock `meta.ResettableRESTMapper` whose `KindsFor("customwidgets")` returns
   `NoResourceMatchError` on the first call (simulating a CRD group missing from the pod's first
   discovery round), but returns the correct GVK on the second call (simulating the CRD becoming
   discoverable, e.g. after the operator's cache TTL or a later successful discovery round).
2. **When**: `ResolveGVKForKind(mapper, "CustomWidget")` is called.
3. **Then**: It returns the correct GVK with no error, and `Reset()` was called exactly once.

**Acceptance Criteria**: Behavior — resolves without error. Correctness — GVK matches the
second-call result. Accuracy — `Reset()` invoked exactly once, `KindsFor` invoked exactly twice.

### UT-K8S-1888-002 / 003 / 004

Summarized in §8; implemented as `DescribeTable`/`It` entries alongside UT-K8S-1888-001 in the
same `Describe("REST mapper self-heal on lookup failure (#1888)")` block.

---

## 10. Environmental Needs

- **Framework**: Ginkgo/Gomega BDD (existing suite convention)
- **Mocks**: In-package mock `meta.RESTMapper`/`meta.ResettableRESTMapper` implementations only
  (no external dependencies — this is a pure-logic unit)
- **Location**: `pkg/shared/k8s/gvk_test.go`

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None. Independent of PR #1895 (#1889/#1892 fix) — different subsystem (RESTMapper discovery
caching vs. anomaly-detector isolation), separate branch/PR by design.

### 11.2 Execution Order

1. **RED**: Add `UT-K8S-1888-001..004` with a new resettable mock mapper; confirm they fail
   against unmodified `gvk.go`.
2. **GREEN**: Add the `Reset()`-on-failure retry to `ResolveGVKForKind`'s fallback.
3. **REFACTOR**: Anti-pattern check (naming, error string, retry-bound clarity); N/A if GREEN
   leaves nothing to clean beyond the minimal retry itself.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|--------------|
| This test plan | `docs/tests/1888/TEST_PLAN.md` | Strategy and test design |
| Unit tests | `pkg/shared/k8s/gvk_test.go` | Ginkgo BDD, extended |
| Design decision | `docs/architecture/decisions/DD-K8S-001-restmapper-cache-invalidation-on-lookup-failure.md` | Alternatives + rationale |

---

## 13. Execution

```bash
make test-unit-shared   # or the make target covering pkg/shared/...
```

---

## 14. Wiring Verification

Not applicable — no new component or wiring point. `ResolveGVKForKind` is already called in
production by `pkg/apifrontend/tools/kubectl_get.go:91` and `pkg/apifrontend/tools/scope_helpers.go:37`.
This fix changes internal behavior of an already-wired function.

---

## 15. Existing Tests Requiring Updates

None. The fix only adds a new branch that activates on failure/empty-result; all existing
success-path assertions in `gvk_test.go`/`gvk_apiversion_test.go` are unaffected.

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-08-03 | Initial test plan |
| 1.1 | 2026-08-03 | RED→GREEN→REFACTOR complete. All 4 new tests (UT-K8S-1888-001..004) pass; full `make test-unit-shared-packages` suite (21 suites) passes with zero regressions; `go build ./...`, `go vet`, and `golangci-lint run` clean. |
