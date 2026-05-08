# Test Plan: K8sAdapter Multi-Group Kind Resolution Fallback

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1062-v1
**Feature**: K8sAdapter resolves ambiguous kinds by trying all API groups when `apiVersion` is empty
**Version**: 1.0
**Created**: 2026-05-08
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/1061-1062-signal-target-and-ambiguous-kind`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

When a Kind exists in multiple API groups (e.g., `Subscription` in `operators.coreos.com` and `messaging.knative.dev`), and `apiVersion` is not provided, the K8sAdapter resolves to a single API group. If the resource does not exist in that group, it returns NotFound without trying alternate groups, causing enrichment to fail and the investigation pipeline to use incorrect target data.

### 1.2 Objectives

1. **Multi-group fallback**: When `apiVersion` is empty and the first resolved API group returns NotFound, try alternate groups before failing.
2. **Explicit apiVersion bypass**: When `apiVersion` is provided, use only the specified group (no fallback).
3. **Non-ambiguous preservation**: Single-group kinds continue to work identically.
4. **Error classification preservation**: When all groups return NotFound, the error wrapping must preserve `IsNotFoundError` classification for `TargetResourceDeleted` handling in `Enrich()`.
5. **Audit trail (FedRAMP AU-6)**: Each fallback attempt is logged with structured context for SRE analysis.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/enrichment/... --ginkgo.focus="1062"` |
| Unit-testable code coverage | >=80% | Coverage on `resolveMappingsAll`, `getResourceWithFallback`, modified `GetOwnerChain`/`GetSpecHash` |
| Backward compatibility | 0 regressions | Full enrichment suite passes |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority

- Issue #1062: k8s adapter resolves ambiguous kind 'Subscription' to wrong API group
- FedRAMP AU-6: Audit Review, Analysis, and Reporting

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- Issue #1040: apiVersion disambiguation (prior art)
- Issue #1044: apiVersionValidationGate (related gate)

---

## 3. Risks & Mitigations

> **IEEE 829 §5** — Software risk issues that drive test design.

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Wrong API group selected for ambiguous kind | Enrichment fails, wrong workflow params | High | UT-KA-1062-001 | Multi-group fallback via `ResourcesFor` |
| R2 | Error classification broken by new wrapping | `HardFail` instead of `TargetResourceDeleted` | Medium | UT-KA-1062-003 | Wrap last `StatusError` via `%w` |
| R3 | Performance regression from extra API calls | Increased enrichment latency | Low | N/A | Fallback only on NotFound/Forbidden, max N-1 extra calls (N typically 2-3) |
| R4 | Test ordering non-determinism from map iteration | Flaky tests | Low | All | `DefaultRESTMapper` sorts by `defaultGroupVersions` preference |
| R5 | `ResourcesFor` behavior differs between DefaultRESTMapper and production DeferredDiscoveryRESTMapper | Production-only bugs | Low | N/A | Dual fallback handles both AmbiguousResourceError and NotFound-after-resolve |

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- `K8sAdapter.resolveMappingsAll()` — new method returning all mappings for a kind
- `K8sAdapter.getResourceWithFallback()` — new method centralizing multi-group-try logic
- `K8sAdapter.GetOwnerChain()` — modified to use `getResourceWithFallback`
- `K8sAdapter.GetSpecHash()` — modified to use `getResourceWithFallback`

### 4.2 Features Not to be Tested

- `Enricher.Enrich()` — tested by separate enricher tests
- `resolveOwnerMapping()` — uses explicit `apiVersion` from ownerReference (never ambiguous)
- `resolveMappingWithAPIVersion()` with non-empty `apiVersion` — unchanged behavior

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Unit-only tier | Adapter uses fake dynamic client + mapper; no real cluster needed |
| `ResourcesFor` over `ResourceFor` | Returns all candidates; `ResourceFor` errors on ambiguity |
| Fallback on NotFound and Forbidden | Both indicate wrong API group (Forbidden = no RBAC for that group) |
| Wrap last error via `%w` | Preserves `errors.As` chain for `IsNotFoundError` |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of `resolveMappingsAll`, `getResourceWithFallback`, modified `GetOwnerChain`/`GetSpecHash`
- **Integration**: Skipped — adapter wraps fake/dynamic client, no real cluster needed
- **E2E**: Skipped — enrichment-level behavior, not system-boundary observable

### 5.2 Pass/Fail Criteria

**PASS**: All unit tests pass, no regressions, `IsNotFoundError` classification preserved.

**FAIL**: Any test failure, or `TargetResourceDeleted` classification broken for all-groups-NotFound case.

---

## 6. Test Items

| File | Functions/Methods | Lines (approx) |
|------|-------------------|----------------|
| `internal/kubernautagent/enrichment/k8s_adapter.go` | `resolveMappingsAll`, `getResourceWithFallback`, `GetOwnerChain`, `GetSpecHash` | ~80 added/modified |

---

## 7. BR Coverage Matrix

| BR / Issue ID | Description | Priority | Tier | Test ID | Status |
|---------------|-------------|----------|------|---------|--------|
| #1062 | Ambiguous kind fallback to second group | P0 | Unit | UT-KA-1062-001 | Pending |
| #1062 | Ambiguous kind, resource in first group | P0 | Unit | UT-KA-1062-002 | Pending |
| #1062 | Ambiguous kind, resource in no group | P0 | Unit | UT-KA-1062-003 | Pending |
| #1062 | Non-ambiguous kind preserved | P0 | Unit | UT-KA-1062-004 | Pending |
| #1062 | Explicit apiVersion bypasses fallback | P0 | Unit | UT-KA-1062-005 | Pending |
| #1062 | GetSpecHash same fallback | P0 | Unit | UT-KA-1062-006 | Pending |
| #1062 | Error preserves IsNotFoundError | P0 | Unit | UT-KA-1062-007 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `UT-KA-{ISSUE}-{SEQUENCE}`

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-KA-1062-001 | Ambiguous `Subscription` kind, resource in OLM (2nd group) — GetOwnerChain succeeds | Pending |
| UT-KA-1062-002 | Ambiguous `Subscription` kind, resource in Knative (1st group) — GetOwnerChain succeeds | Pending |
| UT-KA-1062-003 | Ambiguous kind, resource in neither group — error returned | Pending |
| UT-KA-1062-004 | Non-ambiguous `Pod` kind — existing behavior preserved | Pending |
| UT-KA-1062-005 | Ambiguous kind with explicit `apiVersion` — only specified group used | Pending |
| UT-KA-1062-006 | GetSpecHash fallback for ambiguous kind | Pending |
| UT-KA-1062-007 | All-groups-NotFound error preserves `IsNotFoundError` classification | Pending |

### Tier Skip Rationale

- **Integration**: Skipped. Adapter wraps `dynamic.Interface` + `meta.RESTMapper`; no real cluster I/O needed.
- **E2E**: Skipped. Enrichment-level behavior; system-boundary E2E tests already cover enrichment via mock cluster.

---

## 9. Environmental Needs

### 9.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: `fakedynamic.NewSimpleDynamicClient` + `meta.DefaultRESTMapper` with multi-group registration
- **Location**: `test/unit/kubernautagent/enrichment/k8s_adapter_test.go`

---

## 10. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| This test plan | `docs/tests/1062/TEST_PLAN.md` |
| Unit test additions | `test/unit/kubernautagent/enrichment/k8s_adapter_test.go` |

---

## 11. Execution

```bash
# Focused run
go test ./test/unit/kubernautagent/enrichment/... -count=1 --ginkgo.focus="1062" -v

# Full suite regression
go test ./test/unit/kubernautagent/enrichment/... -count=1 -race
```

---

## 12. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-08 | Initial test plan for Issue #1062 |
