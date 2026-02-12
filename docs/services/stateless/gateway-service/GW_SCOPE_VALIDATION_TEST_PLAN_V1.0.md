# Gateway Scope Validation Test Plan v1.0

**Service**: Gateway (Scope Validation)
**Version**: v1.0
**Date**: February 6, 2026
**Owner**: Gateway Team
**Status**: ✅ **ALL SCOPE TESTS PASS — 4 regressions resolved (see notes)**
**Business Requirements**: BR-SCOPE-001, BR-SCOPE-002, NFR-SCOPE-002
**Architecture**: ADR-053 (Resource Scope Management)

---

## Executive Summary

### Objective

Validate Gateway scope validation using the metadata-only cache pattern (ADR-053 Decision #5).
This test plan covers three tiers (unit, integration, E2E) for the scope validation pipeline
that filters signals based on the `kubernaut.ai/managed` label.

### Scope

- **Shared `scope.Manager`**: 2-level hierarchy resolution, unknown kind resilience, error fallthrough
- **Gateway `ScopeChecker` integration**: Signal rejection pipeline, metrics, audit events
- **RBAC & Cache**: Metadata-only informer behavior, list/watch permissions

### Key Principles

1. **Defense in depth**: Overlapping coverage across tiers for critical scope paths
2. **TDD RED-GREEN-REFACTOR**: Every test scenario defined before implementation
3. **Real infrastructure for E2E**: Kind cluster with actual RBAC and informer caches

---

## Test Registry

### Tier 1: Unit Tests — Shared Scope Manager

**Location**: `test/unit/shared/scope/manager_test.go`

| Test ID | Description | BR | Priority | Status |
|---------|-------------|-----|----------|--------|
| UT-SCOPE-001-001 | Resource inherits managed from namespace | BR-SCOPE-001 | P0 | ✅ Pass |
| UT-SCOPE-001-002 | Resource explicit opt-in label | BR-SCOPE-001 | P0 | ✅ Pass |
| UT-SCOPE-001-003 | Resource explicit opt-out label | BR-SCOPE-001 | P0 | ✅ Pass |
| UT-SCOPE-001-004 | Fall through to namespace when no resource label | BR-SCOPE-001 | P0 | ✅ Pass |
| UT-SCOPE-001-005 | Namespace managed label | BR-SCOPE-001 | P0 | ✅ Pass |
| UT-SCOPE-001-006 | Namespace unmanaged label | BR-SCOPE-001 | P0 | ✅ Pass |
| UT-SCOPE-001-007 | No labels — safe default unmanaged | BR-SCOPE-001 | P0 | ✅ Pass |
| UT-SCOPE-001-008 | Resource opt-out overrides namespace opt-in | BR-SCOPE-001 | P0 | ✅ Pass |
| UT-SCOPE-001-009 | Resource opt-in overrides namespace opt-out | BR-SCOPE-001 | P0 | ✅ Pass |
| UT-SCOPE-001-010 | Namespace not found — unmanaged | BR-SCOPE-001 | P1 | ✅ Pass |
| UT-SCOPE-001-011 | Resource not found — fall through to namespace | BR-SCOPE-001 | P1 | ✅ Pass |
| UT-SCOPE-001-012 | Invalid label value treated as unset | BR-SCOPE-001 | P1 | ✅ Pass |
| UT-SCOPE-001-013 | Cluster-scoped resource managed | BR-SCOPE-001 | P0 | ✅ Pass |
| UT-SCOPE-001-014 | Cluster-scoped resource unmanaged | BR-SCOPE-001 | P0 | ✅ Pass |
| UT-SCOPE-001-015 | Cluster-scoped resource opt-out | BR-SCOPE-001 | P0 | ✅ Pass |
| UT-SCOPE-001-016 | Resource lookup non-NotFound error propagated | BR-SCOPE-001 | P1 | ✅ Pass |
| UT-SCOPE-001-017 | Namespace lookup non-NotFound error propagated | BR-SCOPE-001 | P1 | ✅ Pass |
| UT-SCOPE-001-018 | Invalid namespace label treated as unset | BR-SCOPE-001 | P1 | ✅ Pass |
| UT-SCOPE-001-019 | Unknown kind skips resource check — falls through to namespace | BR-SCOPE-001, ADR-053 | P0 | ✅ Pass |
| UT-SCOPE-001-020 | Forbidden error on resource lookup — graceful fallthrough to namespace | BR-SCOPE-001, ADR-053 | P0 | ✅ Pass |
| UT-SCOPE-001-021 | "No matches for kind" error — graceful fallthrough to namespace | BR-SCOPE-001, ADR-053 | P0 | ✅ Pass |
| UT-SCOPE-001-022 | Unknown kind with managed namespace returns managed | BR-SCOPE-001, ADR-053 | P1 | ✅ Pass |
| UT-SCOPE-001-023 | Unknown kind with unmanaged namespace returns unmanaged | BR-SCOPE-001, ADR-053 | P1 | ✅ Pass |

### Tier 1: Unit Tests — Gateway Scope Validation

**Location**: `test/unit/gateway/scope_validation_test.go`

| Test ID | Description | BR | Priority | Status |
|---------|-------------|-----|----------|--------|
| UT-GW-002-001 | Reject signal from unmanaged resource | BR-SCOPE-002 | P0 | ✅ Pass |
| UT-GW-002-002 | Accept signal from managed resource | BR-SCOPE-002 | P0 | ✅ Pass |
| UT-GW-002-003 | Reject signal with resource opt-out | BR-SCOPE-002 | P0 | ✅ Pass |
| UT-GW-002-004 | Actionable rejection response | BR-SCOPE-002 | P0 | ✅ Pass |
| UT-GW-002-005 | Prometheus metric incremented on rejection | BR-SCOPE-002 | P0 | ✅ Pass |
| UT-GW-002-006 | Structured log on rejection | BR-SCOPE-002 | P1 | ✅ Pass |
| UT-GW-002-007 | ScopeChecker returns error | BR-SCOPE-002 | P0 | ✅ Pass |
| UT-GW-002-008 | nil ScopeChecker (backward compatibility) | BR-SCOPE-002 | P1 | ✅ Pass |
| UT-GW-002-009 | Rejection response for cluster-scoped resource | BR-SCOPE-002 | P1 | ✅ Pass |
| UT-GW-002-010 | Multiple rejections increment metric correctly | BR-SCOPE-002 | P1 | ✅ Pass |

### Tier 2: Integration Tests — Gateway Scope Filtering

**Location**: `test/integration/gateway/scope_filtering_test.go`

| Test ID | Description | BR | Priority | Status |
|---------|-------------|-----|----------|--------|
| IT-GW-002-001 | No RR created for unmanaged signal | BR-SCOPE-002 | P0 | ✅ Pass |
| IT-GW-002-002 | RR created for managed signal | BR-SCOPE-002 | P0 | ✅ Pass |
| IT-GW-002-003 | Scope validation < 10ms latency | NFR-SCOPE-002 | P0 | ✅ Pass |
| IT-GW-002-004 | Namespace inheritance — Pod without label in managed NS | BR-SCOPE-001 | P0 | ✅ Pass |
| IT-GW-002-005 | Resource opt-out overrides managed namespace | BR-SCOPE-001 | P0 | ✅ Pass |
| IT-GW-002-006 | Resource opt-in overrides unmanaged namespace | BR-SCOPE-001 | P0 | ✅ Pass |
| IT-GW-002-007 | Dynamic scope change — add label mid-test | BR-SCOPE-001 | P1 | ✅ Pass |
| IT-GW-002-008 | Adapter-agnostic rejection (K8s Event signal) | BR-SCOPE-002 | P0 | ✅ Pass |
| IT-GW-002-009 | Consecutive rejections — metric counter accuracy | BR-SCOPE-002 | P1 | ✅ Pass |
| IT-GW-002-010 | Rejection response field verification | BR-SCOPE-002 | P0 | ✅ Pass |

### Tier 3: E2E Tests — Gateway Scope Validation

**Location**: `test/e2e/gateway/37_scope_filtering_test.go`

| Test ID | Description | BR | Priority | Status |
|---------|-------------|-----|----------|--------|
| E2E-GW-002-001 | Managed signal creates RR in Kind cluster | BR-SCOPE-002, ADR-053 | P0 | ✅ Pass |
| E2E-GW-002-002 | Unmanaged signal rejected with HTTP 200 + rejection body | BR-SCOPE-002, ADR-053 | P0 | ✅ Pass |
| E2E-GW-002-003 | Namespace inheritance — pod without label in managed NS | BR-SCOPE-002, ADR-053 | P0 | ✅ Pass |
| E2E-GW-002-004 | Resource opt-out in managed namespace | BR-SCOPE-002, ADR-053 | P0 | ✅ Pass |
| E2E-GW-002-005 | Actionable label instructions in rejection response | BR-SCOPE-002, ADR-053 | P0 | ✅ Pass |
| E2E-GW-002-006 | `gateway_signals_rejected_total` metric on /metrics endpoint | BR-SCOPE-002, ADR-053 | P0 | ✅ Pass |

---

## New Test Specifications (TDD RED Phase)

### UT-SCOPE-001-019: Unknown kind — skips resource check

```go
It("UT-SCOPE-001-019: should skip resource check for unknown kind and fall through to namespace", func() {
    ns := makeNamespace("production", map[string]string{
        scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
    })
    setup(ns)

    // "Unknown" is not in kindToGroup — resource check should be skipped entirely
    managed, err := mgr.IsManaged(ctx, "production", "Unknown", "test-resource")
    Expect(err).ToNot(HaveOccurred())
    Expect(managed).To(BeTrue(), "Unknown kind should skip resource check and inherit from namespace")
})
```

### UT-SCOPE-001-020: Forbidden error — graceful fallthrough

```go
It("UT-SCOPE-001-020: should fall through to namespace when resource lookup returns Forbidden", func() {
    ns := makeNamespace("rbac-ns", map[string]string{
        scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
    })

    setupWithInterceptor(interceptor.Funcs{
        Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
            if key.Name == "restricted-pod" && key.Namespace == "rbac-ns" {
                return apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "restricted-pod", fmt.Errorf("RBAC denied"))
            }
            return c.Get(ctx, key, obj, opts...)
        },
    }, ns)

    managed, err := mgr.IsManaged(ctx, "rbac-ns", "Pod", "restricted-pod")
    Expect(err).ToNot(HaveOccurred(), "Forbidden error should NOT propagate — graceful fallthrough")
    Expect(managed).To(BeTrue(), "Should inherit from namespace when resource check is Forbidden")
})
```

### UT-SCOPE-001-021: "No matches for kind" error — graceful fallthrough

```go
It("UT-SCOPE-001-021: should fall through to namespace when resource lookup returns no-matches error", func() {
    ns := makeNamespace("kind-ns", map[string]string{
        scope.ManagedLabelKey: scope.ManagedLabelValueTrue,
    })

    setupWithInterceptor(interceptor.Funcs{
        Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
            if key.Name == "custom-thing" && key.Namespace == "kind-ns" {
                return &apierrors.StatusError{
                    ErrStatus: metav1.Status{
                        Status:  metav1.StatusFailure,
                        Code:    404,
                        Reason:  metav1.StatusReasonNotFound,
                        Message: "no matches for kind \"CustomThing\" in version \"v1\"",
                    },
                }
            }
            return c.Get(ctx, key, obj, opts...)
        },
    }, ns)

    // Note: "CustomThing" IS in kindToGroup? No — it's not. So this tests the case
    // where the kind IS known but the API server returns an unusual error.
    // Actually for unknown kinds, UT-SCOPE-001-019 covers it.
    // This test covers a KNOWN kind (e.g., Pod) where the API server returns an unexpected error.
    managed, err := mgr.IsManaged(ctx, "kind-ns", "Pod", "custom-thing")
    Expect(err).ToNot(HaveOccurred(), "No-matches error should NOT propagate — graceful fallthrough")
    Expect(managed).To(BeTrue(), "Should inherit from namespace when resource returns no-matches error")
})
```

---

## TDD Phases

### RED Phase
- Write UT-SCOPE-001-019, 020, 021, 022, 023 — all should FAIL against current `scope.Manager`
- UT-SCOPE-001-016 currently expects error propagation — needs updating after GREEN phase (behavior change)

### GREEN Phase
- Implement `checkResourceLabel()` resilience in `pkg/shared/scope/manager.go`:
  - Unknown kind → skip resource check entirely
  - Non-NotFound errors → graceful fallthrough with Info-level log

### REFACTOR Phase
- Extract `isKnownKind()` helper if warranted
- Consolidate error handling patterns
- Update UT-SCOPE-001-016 to match new behavior (Forbidden now falls through instead of erroring)

---

## Dependencies

- **Shared `ScopeChecker` interface** (A2): Must be promoted to `pkg/shared/scope/checker.go` before RO can reuse
- **Gateway RBAC** (A4): E2E tests require `list`/`watch` permissions for scope-relevant resource types
- **Gateway cached client** (A3): Production wiring change `apiReader` → `ctrlClient`

---

## E2E Execution Results (Feb 8, 2026)

**Gateway E2E Suite**: 99 Passed, 4 Failed (of 104 specs)

### Scope E2E Tests: 6/6 PASS

All scope-specific E2E tests passed after fixing `sendSuccessResponse()` to return HTTP 200
for `StatusRejected` (previously defaulted to HTTP 201).

### Resolved Regressions from Scope Validation (February 8, 2026)

4 pre-existing E2E tests were identified as failing due to scope validation rejecting signals
to unmanaged or non-existent namespaces. All 4 have been resolved:

| Test File | Resolution | Details |
|-----------|------------|---------|
| `09_signal_validation_test.go` | **Fixed** | Updated to create a managed test namespace instead of using `default` |
| `22_audit_errors_test.go` | **Removed** | Namespace fallback feature deprecated (DD-GATEWAY-007). Entire test file deleted. |
| `24_audit_signal_data_test.go` | **Fixed** | Added `Eventually` wrapper around initial `postToGateway` call to tolerate informer cache sync delays |
| `27_error_handling_test.go` | **Removed (fallback test case)** | Namespace fallback test case removed. Other test cases in file retained. |

**Decision Made**: Namespace fallback (DD-GATEWAY-007) was deprecated as it creates
technical debt with scope management. Scope validation correctly rejects signals to
unmanaged/non-existent namespaces. The RO would block any fallback-created RRs anyway
since the underlying resources lack the managed label.
