# DD-K8S-001: RESTMapper Cache Invalidation on Lookup Failure

## Status
**✅ Approved** (2026-08-03)
**Last Reviewed**: 2026-08-03
**Confidence**: 96%

---

## Context & Problem

**Problem** (Issue #1888): AF's `kubectl_get`/`kubectl_list` MCP tools intermittently fail to
resolve valid CRD kinds (e.g., Red Hat ACM's `search.open-cluster-management.io/Search`) with
"cannot resolve GVK for kind", even though the CRD is genuinely installed and RBAC-accessible.
The failure is permanent for the life of the pod — a restart fixes it.

**Root cause** (confirmed against live-cluster testing and the exact vendored
`k8s.io/client-go@v0.35.7` source, not just generic recollection):

1. `cmd/apifrontend/main.go` constructs `restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(disc))`.
2. `pkg/shared/k8s/gvk.go`'s `ResolveGVKForKind` fallback calls `mapper.KindsFor(...)` on it for
   any Kind not in the static table (CRDs, third-party resources).
3. `DeferredDiscoveryRESTMapper.KindsFor` (client-go source):
   ```go
   gvks, err = del.KindsFor(resource)
   if len(gvks) == 0 && !d.cl.Fresh() {
       d.Reset()
       gvks, err = d.KindsFor(resource)
   }
   ```
   The self-heal retry only fires when `!d.cl.Fresh()`.
4. `memCacheClient.Fresh()` returns `d.cacheValid`, which is set `true` once by `refreshLocked()`
   on the first successful discovery populate and is **never** invalidated on any timer — only an
   explicit `Invalidate()`/`Reset()` call flips it back to `false`. There is no periodic TTL
   anywhere in this client.
5. `restmapper.GetAPIGroupResources` additionally discards the error from
   `ServerGroupsAndResources()` whenever `gs`/`rs` come back non-nil — so a single group's
   transient `*discovery.ErrGroupDiscoveryFailed` during that first populate is silently dropped,
   with no log line anywhere in the call chain.

**Net effect**: if a CRD's API group is missing from the pod's very first discovery round (a
transient API-server hiccup, an aggregated-API-server slow to register, or the CRD being
installed a few seconds after AF's mapper does its first lazy fetch), that Kind is **permanently**
unresolvable until the pod restarts — the built-in self-heal only exists for the window before the
first successful populate, never after.

**Blast radius**: confirmed to affect only mappers built via `client-go`'s
`restmapper.DeferredDiscoveryRESTMapper` directly. RemediationOrchestrator and
EffectivenessMonitor call `ResolveGVKWithAPIVersion`/`ResolveGVKForKind` with
`k8sManager.GetRESTMapper()` (controller-runtime's `apiutil.NewDynamicRESTMapper`), a different
implementation that reactively invalidates per-request on `NoKindMatchError` — not subject to
this one-shot-`Fresh()` bug. The fix is therefore scoped to `ResolveGVKForKind`'s fallback, the
only path AF's `kubectl_get`/`kubectl_list` tools use.

**Existing precedent**: `pkg/kubernautagent/tools/k8s/resolver.go` (KA's own dynamic resolver,
added independently, unrelated code path) already has this exact fix pattern:

```go
type resettableMapper interface {
    Reset()
}
...
gvrs, err := r.mapper.ResourcesFor(schema.GroupVersionResource{Resource: resource})
if err != nil {
    if rm, ok := r.mapper.(resettableMapper); ok {
        rm.Reset()
        gvrs, err = r.mapper.ResourcesFor(schema.GroupVersionResource{Resource: resource})
    }
    ...
}
```

`gvk.go`'s `ResolveGVKForKind` never got the same treatment.

---

## Alternatives Considered

### Alternative 1: Reactive `Reset()`-on-failure retry ⭐ **SELECTED**

**Approach**: In `ResolveGVKForKind`'s fallback, on `KindsFor` failure/empty result, type-assert
the mapper to `meta.ResettableRESTMapper` (apimachinery's own interface for exactly this purpose)
and retry once after `Reset()`.

```go
gvks, err := mapper.KindsFor(schema.GroupVersionResource{Resource: pluralGVR.Resource})
if (err != nil || len(gvks) == 0) {
    if rm, ok := mapper.(meta.ResettableRESTMapper); ok {
        rm.Reset()
        gvks, err = mapper.KindsFor(schema.GroupVersionResource{Resource: pluralGVR.Resource})
    }
}
```

**Pros**:
- ✅ Mirrors an already-shipped, already-accepted pattern in this codebase (`resolver.go`) —
  zero new design risk
- ✅ Self-heals on the very next lookup, not after a timer window
- ✅ Smallest possible diff (~6 lines), no new goroutines, no new config/interval to tune
- ✅ `apimachinery`'s own `meta.ResettableRESTMapper` doc comment explicitly recommends this
  check for delegating mappers

**Cons**:
- ⚠️ A genuinely-invalid/typo'd Kind still costs one extra discovery round-trip per failed
  lookup (no caching of "this Kind is definitely bad")
  - **Mitigation**: Same accepted trade-off as the existing `resolver.go` precedent; bounded to
    exactly one retry, not a loop

**Confidence**: 96% (approved — precedented, minimal, directly addresses the reported symptom)

---

### Alternative 2: Periodic TTL-based `Reset()` background goroutine

**Approach**: Start a background goroutine at mapper-construction time (`cmd/apifrontend/main.go`)
that calls `mapper.Reset()` on a fixed interval (e.g., every 10 minutes), mirroring the
`StartAnomalyDetectorCleanupLoop`/`store.StartCleanupLoop` idiom used elsewhere in this codebase
(#1892).

**Pros**:
- ✅ Proactively self-heals without waiting for a failed lookup
- ✅ Consistent with an existing background-sweep idiom in this codebase

**Cons**:
- ❌ New moving part: an interval to choose and justify (too short = wasted full-discovery
  refreshes on every AF pod; too long = long recovery window)
- ❌ Doesn't cover the case of a CRD installed after the pod's first successful discovery *and*
  before the next lookup attempt if no lookup ever fails to trigger a refresh in between —
  correctness still ultimately depends on a failed lookup happening at some point
- ❌ Adds recurring full-discovery-refresh cost (`ServerGroupsAndResources()` across all API
  groups) independent of actual usage

**Confidence**: 70% (rejected as primary fix — solves a superset of the problem at higher
ongoing cost, without being strictly necessary given Alternative 1 already self-heals on demand)

---

### Alternative 3: Combined (Alternative 1 + Alternative 2 as defense-in-depth)

**Approach**: Ship the reactive retry now, and separately consider a periodic backstop later if
telemetry shows kinds still going unresolved for extended periods in production.

**Pros**:
- ✅ Highest robustness ceiling

**Cons**:
- ❌ Adds Alternative 2's complexity/cost for a benefit not yet demonstrated to be needed
- ❌ Scope creep relative to the reported, reproduced bug

**Confidence**: 65% (deferred — revisit only if Alternative 1 proves insufficient in production)

---

## Decision

**APPROVED: Alternative 1** — Reactive `Reset()`-on-failure retry in `ResolveGVKForKind`'s
fallback, using apimachinery's `meta.ResettableRESTMapper` interface.

**Rationale**:
1. Directly fixes the reported, reproduced symptom (permanent post-first-miss unresolvability)
   with the smallest possible change.
2. Mirrors a pattern already shipped and accepted in this exact codebase
   (`pkg/kubernautagent/tools/k8s/resolver.go`), eliminating design novelty/risk.
3. No new background goroutine, interval, or Helm value to introduce, document, and maintain.
4. Alternative 2/3's extra robustness is not justified without evidence that reactive-only
   self-heal is insufficient in production; can be revisited later (see Review & Evolution).

---

## Implementation

### Package Location

```
pkg/shared/k8s/gvk.go          # ResolveGVKForKind fallback (modified)
pkg/shared/k8s/gvk_test.go     # UT-K8S-1888-* (new)
```

### Core Change

```go
if mapper != nil {
    pluralGVR, _ := meta.UnsafeGuessKindToResource(schema.GroupVersionKind{Kind: kind})
    resource := schema.GroupVersionResource{Resource: pluralGVR.Resource}
    gvks, err := mapper.KindsFor(resource)
    if err != nil || len(gvks) == 0 {
        // #1888: DeferredDiscoveryRESTMapper's own self-heal only fires once, before its
        // first successful discovery populate (see DD-K8S-001). A CRD group missing from
        // that first round stays unresolvable forever without this explicit retry.
        if rm, ok := mapper.(meta.ResettableRESTMapper); ok {
            rm.Reset()
            gvks, err = mapper.KindsFor(resource)
        }
    }
    if err == nil && len(gvks) > 0 {
        return gvks[0], nil
    }
}
```

---

## Consequences

### Positive
- ✅ CRD kinds that miss AF's first discovery round now resolve on the very next lookup —
  no pod restart required
- ✅ Zero new infrastructure, config, or Helm values
- ✅ Consistent pattern now exists in both `pkg/shared/k8s/gvk.go` and
  `pkg/kubernautagent/tools/k8s/resolver.go`

### Negative
- ⚠️ One extra discovery round-trip per genuinely-failed lookup (typo'd Kind, truly
  unregistered resource)
  - **Mitigation**: Bounded to exactly one retry; accepted trade-off already present in
    `resolver.go`

### Neutral
- 🔄 `ResolveGVKWithAPIVersion`'s explicit-`apiVersion` path is intentionally left unchanged —
  its only callers use a different, unaffected RESTMapper implementation (see Context)

---

## Related Documents

| Document | Relationship |
|----------|--------------|
| `docs/tests/1888/TEST_PLAN.md` | Test plan for this fix |
| `pkg/kubernautagent/tools/k8s/resolver.go` | Existing precedent for the same retry pattern |
| Issue #310 / #1275 / #1040 | Prior `gvk.go` fixes (ambiguity, static-table CRDs, apiVersion disambiguation) |

---

## Review & Evolution

**When to Revisit**:
- If production telemetry shows kinds still failing to resolve across multiple consecutive
  lookups (would indicate Alternative 2/3's proactive backstop is needed)
- If `client-go` changes `DeferredDiscoveryRESTMapper`'s `Fresh()` semantics in a future version

---

**Last Updated**: August 3, 2026

## Change Log

| Date | Change | Author |
|------|--------|--------|
| 2026-08-03 | Initial approval | AI Assistant |
