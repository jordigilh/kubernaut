# Spike S9 — ScopeChecker Interface Design

## Objective

Validate backward-compatible approaches for making `ScopeChecker` cluster-aware
without breaking existing callers in GW and RO.

## Options Evaluated

### Option A: Context-Based Cluster Routing

```go
// ClusterID carried in context — existing callers unchanged
func ContextWithClusterID(ctx context.Context, clusterID string) context.Context
func ClusterIDFromContext(ctx context.Context) string

type ContextAwareFederatedChecker struct { ... }
func (c *ContextAwareFederatedChecker) IsManaged(ctx, ns, kind, name) (bool, error)
// Extracts clusterID from ctx; routes accordingly
```

**Pros:**
- Zero signature change to `ScopeChecker` interface
- Zero change to existing mock implementations in tests
- Callers just add one line: `ctx = ContextWithClusterID(ctx, signal.ClusterID)`
- Drop-in replacement: `var _ ScopeChecker = (*ContextAwareFederatedChecker)(nil)`

**Cons:**
- Implicit routing (cluster info is "hidden" in context)
- Easy to forget to set context — fails silently to local (safe default, but masks bugs)
- Harder to grep for cluster-aware call sites

### Option B: Extended Interface (IsManagedOnCluster)

```go
type FederatedScopeChecker interface {
    ScopeChecker
    IsManagedOnCluster(ctx, clusterID, ns, kind, name) (bool, error)
}
```

**Pros:**
- Explicit cluster parameter — clear at call site
- Type assertion makes cluster-aware callers self-documenting
- Harder to forget (compiler won't help, but code review will)

**Cons:**
- Callers must type-assert: `federated, ok := checker.(FederatedScopeChecker)`
- Or change their dependency type from `ScopeChecker` to `FederatedScopeChecker`
- Two code paths: some callers use `IsManaged()`, some use `IsManagedOnCluster()`

## Test Results — All PASS

| Test | Option | Scenario | Result |
|------|--------|----------|--------|
| S9-001 | A | Empty context → local | PASS |
| S9-002 | A | Context with clusterID → remote | PASS |
| S9-003 | A | Existing callers backward-compatible | PASS |
| S9-004 | A | Satisfies ScopeChecker interface | PASS |
| S9-005 | B | IsManaged → local (backward-compatible) | PASS |
| S9-006 | B | IsManagedOnCluster → remote | PASS |
| S9-007 | B | IsManagedOnCluster empty → local | PASS |
| S9-008 | B | Satisfies ScopeChecker interface | PASS |
| S9-009 | B | Type assertion pattern works | PASS |
| S9-010 | A | GW caller pattern | PASS |
| S9-011 | B | GW caller pattern | PASS |
| S9-012 | A | RO caller pattern | PASS |
| S9-013 | A | Error propagation | PASS |

## Recommendation: Option A (Context-Based)

**Rationale:**

1. **Minimal diff**: GW and RO already have `signal.ClusterID` and `rr.Spec.ClusterID`
   available at the call site. They just need one extra line to inject it into context
   before calling the unchanged `IsManaged()`.

2. **No interface change**: The `ScopeChecker` interface stays as-is. All 3 mock
   implementations in `test/shared/mocks/` remain valid. No refactoring wave.

3. **Safe default**: If caller forgets to set context, empty clusterID → local check.
   This is the correct behavior for the hub cluster (matches current behavior).

4. **Already precedented**: The project already uses context for request-scoped data
   (auth info, trace IDs, request metadata). ClusterID is the same category.

5. **Aligns with existing `FederatedScopeChecker`**: The current implementation in
   `pkg/fleet/scopecache/federated_checker.go` already has `IsManagedOnCluster` as
   a separate method. Option A replaces that with context extraction inside `IsManaged`,
   unifying the two code paths.

### Migration Pattern for Callers

**GW (server.go):**
```go
// Before:
managed, err := s.scopeChecker.IsManaged(ctx, signal.Namespace, signal.Resource.Kind, signal.Resource.Name)

// After:
scopeCtx := scope.ContextWithClusterID(ctx, signal.ClusterID)
managed, err := s.scopeChecker.IsManaged(scopeCtx, signal.Namespace, signal.Resource.Kind, signal.Resource.Name)
```

**RO (blocking.go):**
```go
// Before:
managed, err := r.scopeChecker.IsManaged(ctx, rr.Spec.TargetResource.Namespace, ...)

// After:
scopeCtx := scope.ContextWithClusterID(ctx, rr.Spec.ClusterID)
managed, err := r.scopeChecker.IsManaged(scopeCtx, rr.Spec.TargetResource.Namespace, ...)
```

### Implementation Plan

1. Add `ContextWithClusterID` / `ClusterIDFromContext` to `pkg/shared/scope/context.go`
2. Modify `FederatedScopeChecker.IsManaged()` to extract clusterID from context
3. Remove `IsManagedOnCluster` method (unified into `IsManaged` via context)
4. Update GW and RO callers to inject context (2 lines total)
5. No mock changes needed — mocks ignore context anyway
