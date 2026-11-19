# ADR-053: Resource Scope Management Architecture

**Status**: ✅ APPROVED
**Date**: January 20, 2026
**Deciders**: Platform Team, Gateway Team, RemediationOrchestrator Team
**Confidence**: 95%

> **Approved pre-GA amendment (2026-11-16):**
> [DD-FLEET-015](DD-FLEET-015-owned-watch-and-enum-scope.md) supersedes this ADR's
> older boolean checker, missing-target/namespace fallback and invalid-label
> examples. `ScopeChecker.Check` returns enum state/provenance; only verified
> managed authorizes action. Missing observation evidence is unavailable, invalid
> labels cannot inherit, and cluster-scoped targets authorize through their own
> observed label (FR-SCOPE-004 Option A, #2670); only the non-labelable
> `ClusterScope` pseudo-target stays deferred.
> AF uses `ErrResourceNotManaged` only for verified unmanaged and
> `ErrScopeUnavailable` for missing/invalid/unsupported/denied authority or errors.
> Existing-RR takeover remains distinct from new-RR admission under ADR-068.

> **Note (2026-02-24)**: [ADR-057](ADR-057-crd-namespace-consolidation.md) supersedes this ADR's
> implicit assumption that CRDs are created in the signal/workload namespace. Per ADR-057, **all
> kubernaut CRDs are created in the controller namespace** (`kubernaut-system`). The target resource
> namespace is preserved in `Spec.TargetResource.Namespace`. Scope validation in this ADR remains
> authoritative — it now evaluates `TargetResource.Namespace` instead of the CRD's own namespace.

---

## Context

### Problem Statement

Kubernaut operates in multi-tenant Kubernetes clusters where not all resources should be automatically remediated. Operators need fine-grained control over which namespaces and resources Kubernaut manages to:

1. **Multi-Tenancy**: Different tenants may want independent control over auto-remediation
2. **Shared Infrastructure**: System namespaces (`kube-system`, `istio-system`) often require manual intervention
3. **Compliance Requirements**: Some workloads may require manual approval for all changes
4. **Gradual Rollout**: Operators want to enable Kubernaut incrementally (pilot namespaces first)
5. **Safety Boundary**: Prevent accidental remediation of critical resources

### Current Limitation

Without scope management, Kubernaut would:
- ❌ Process ALL signals from ALL namespaces (observability noise)
- ❌ Potentially remediate resources operators don't want automated
- ❌ Create RemediationRequest CRDs for unmanaged resources (wasted Kubernetes API load)
- ❌ Lack clear security boundary for auto-remediation scope

### Business Requirements

This ADR implements three Business Requirements:
- **BR-SCOPE-001**: Resource Scope Management (parent BR)
- **BR-SCOPE-002**: Gateway Signal Filtering
- **BR-SCOPE-010**: RemediationOrchestrator Routing Scope Validation

---

## Decision

### 1. Opt-In Model with Value-Based Labels

**Chosen**: Use `kubernaut.ai/managed="true"` label (value-based, not existence-only)

**Configuration**:
```yaml
# Namespace-level opt-in
apiVersion: v1
kind: Namespace
metadata:
  name: production
  labels:
    kubernaut.ai/managed: "true"  # Opt-in to Kubernaut remediation

---
# Resource-level override
apiVersion: apps/v1
kind: Deployment
metadata:
  name: legacy-app
  namespace: production
  labels:
    kubernaut.ai/managed: "false"  # Explicit opt-out (overrides namespace)
```

**Rationale**:
- **Label Domain**: `kubernaut.ai` matches API group domain (`kubernaut.ai/v1alpha1`)
- **Value-Based**: Explicit intent (true/false/unset) vs existence-only
- **Cluster Tool Compatibility**: Kyverno, OPA, admission webhooks expect values
- **Audit Trail**: Clear operator decision visible in K8s events
- **Future Extensibility**: Can add values like `"dry-run"`, `"audit-only"`

---

### 2. Two-Level Hierarchy (Resource → Namespace)

**Chosen**: Check resource label first, then namespace label (no owner chain traversal)

**Validation Logic**:
```
1. Is resource cluster-scoped (Node, PersistentVolume)?
   ├─ YES → Check resource label ONLY
   │   ├─ kubernaut.ai/managed="true" → MANAGED
   │   ├─ kubernaut.ai/managed="false" → UNMANAGED
   │   └─ No label → UNMANAGED (safe default)
   │
   └─ NO → Resource is namespaced (Pod, Deployment, Service)
           │
           ├─ Check resource label (Level 1)
           │   ├─ kubernaut.ai/managed="true" → MANAGED (explicit opt-in)
           │   ├─ kubernaut.ai/managed="false" → UNMANAGED (explicit opt-out)
           │   └─ No label → Continue to Level 2
           │
           └─ Check namespace label (Level 2)
               ├─ kubernaut.ai/managed="true" → MANAGED (inherited)
               ├─ kubernaut.ai/managed="false" → UNMANAGED (inherited)
               └─ No label → UNMANAGED (safe default)
```

**Rationale**:
- **Simplicity**: Only 2 levels (resource → namespace), not 5 (pod → replicaset → deployment → namespace)
- **Performance**: Max 2 API calls (resource + namespace)
- **Operator Control**: Resource-level override for exceptions (e.g., exclude legacy apps from managed namespace)

### 2a. Recognized kinds (issue #2670)

The scope lanes classify a target as cluster-scoped through a static kind map
(`pkg/shared/scope/manager.go` `clusterScopedKinds`) plus an empty namespace, and
resolve it at level 1 only. The map is a heuristic, not API discovery: an entry
that the cluster does not serve is simply unobservable. An identity without a
namespace is resolved at level 1 when its kind is unknown or known to be
cluster-scoped; a kind known to be namespaced (`Pod`, `Deployment`, `Service`,
`ConfigMap`, `Secret`, `PersistentVolumeClaim`, …) is rejected first as
`invalid` / `namespaced_target_without_namespace`, because such an identity can
never be satisfied and retrying it would loop. The inventory is pinned by
`UT-SCOPE-2670-012`, so adding or removing a kind requires changing this section
and the test together.

**Upstream Kubernetes, shipped by default (19):** `Node`, `PersistentVolume`,
`Namespace`, `ClusterRole`, `ClusterRoleBinding`, `StorageClass`,
`CustomResourceDefinition`, `IngressClass`, `PriorityClass`,
`ValidatingWebhookConfiguration`, `MutatingWebhookConfiguration`, `CSIDriver`,
`CSINode`, `VolumeAttachment`, `RuntimeClass`, `APIService`,
`CertificateSigningRequest`, `FlowSchema`, `PriorityLevelConfiguration`.
(`APIService`, the CSI kinds and the flowcontrol kinds can be disabled in a
stripped-down apiserver; absence only makes them unobservable.)

**OpenShift-only (4):** `ClusterOperator`, `ClusterVersion`, `Infrastructure`,
`ClusterNetwork`.

**Namespaced kinds recognized by the same map** (they keep the 2-level
hierarchy and are never treated as cluster-scoped): `Pod`, `Service`,
`ConfigMap`, `Secret`, `PersistentVolumeClaim`, `ReplicaSet`, `Deployment`,
`StatefulSet`, `DaemonSet`, `Job`, `CronJob`, `HorizontalPodAutoscaler`,
`Ingress`, `PodDisruptionBudget`.

A kind **outside** this map that carries no namespace stays ambiguous: it is
resolved at level 1 like a cluster-scoped CRD, because the static map cannot
prove it is namespaced. A cluster-scoped CRD that is never observed therefore
ends in retryable `unavailable` rather than a terminal rejection. Closing that
gap needs a RESTMapper-based kind resolver; both directions of the ambiguity are
tracked as one follow-up.

**Normalization and dedup.** The Gateway adapters drop a claimed namespace for a
cluster-scoped target before the fingerprint is computed, so the dedup key, the
persisted `spec.targetResource` and the scope identity agree (for cluster-scoped
kinds this supersedes #1371's "honor an explicit namespace label"). The
Prometheus adapter uses API discovery, so it also covers a discovered
cluster-scoped CRD; the Kubernetes-event adapter has no discovery client and
covers only the static kinds, which is acceptable because events for
cluster-scoped objects normally carry no namespace.

The residual case is a cluster-scoped kind that **neither the adapters nor the
Gateway's discovery snapshot** classifies, arriving with a namespace. That
identity is treated as *namespaced* and can be authorized by that namespace's
`kubernaut.ai/managed` label at level 2 rather than failing closed. When discovery
does know the kind, the Gate normalizes it to level 1 regardless of the adapter,
so the only remaining divergence there is the dedup key: the Kubernetes-event
adapter has no discovery client and keeps the claimed namespace in the hash,
while the RR target and the scope identity carry none. That can produce two RRs
for one cluster-scoped CRD (BR-GATEWAY-004), and the same RESTMapper-based
resolver follow-up closes both directions of the ambiguity.

**Direct ACM and an empty namespace.** `buildFilters` omits the namespace filter
when the identity has none, but `searchScopeItem` re-verifies the returned item
against the requested identity, namespace included. A namespaced object therefore
cannot satisfy a namespace-less identity: the query is rejected as a mismatched
identity and the lane returns `unavailable`. All lanes fail closed for that
identity; only a genuinely cluster-scoped object passes and resolves at level 1
from its own label.

**Reads are read-only.** The check performs a metadata-only `Get` on
`PartialObjectMetadata` (local lane), a cache read (fenced fleet lane), or an
ACM Search query; no mutating verb is required or performed.

**Read grants per lane** (nothing here is a mutation permission):
- Gateway and API Frontend read the shared target set, which includes
  `nodes`, `persistentvolumes` and `persistentvolumeclaims`.
- RemediationOrchestrator reads `pods`, `nodes`, `services`, `namespaces`,
  `persistentvolumes`, `persistentvolumeclaims` and `configmaps`, plus the `apps`
  workload kinds. The PVC grant landed in
  [kubernaut-operator#646](https://github.com/jordigilh/kubernaut-operator/pull/646)
  (issue #645) at the same time as this behaviour, because a PVC scope check at
  RO was otherwise unavailable.
- The Fenced Fleet Metadata Cache syncs Deployment, StatefulSet, DaemonSet, Pod,
  Service, Node and Namespace; for cluster-scoped **targets** that means `Node`
  and `Namespace`, with Namespace also serving as inheritance evidence.
- Direct ACM has no per-kind grant: visibility is bounded by ACM Search RBAC
  (see ADR-068 §ACM Search RBAC), which is broader than the fenced lane's list.

**Invalid-label observations on the fenced lane.** The FMC syncer rejects a
malformed `kubernaut.ai/managed` value before it stages a repair
(`pkg/fleet/fmc/syncer.go` `buildObservedResource`), so an invalid observation is
never committed and a previously committed record keeps serving until a later
repair replaces it or its TTL expires (45s by default). For a cluster-scoped
target that means a record committed as `true` whose label later becomes invalid
can still resolve `verified_managed` for that bounded window; the local lane,
which reads the object live, returns `invalid` / `invalid_resource_label`
immediately. Removing authorization on an invalid observation — instead of
retaining the last committed snapshot — changes the DD-FLEET-016 staged-repair
contract, applies to namespaced targets as well, and is a tracked follow-up
rather than part of #2670.

Failure states are fail-closed and lane-specific: an unreadable kind on the
cached Gateway/RO lanes ends in retryable `unavailable` after the scope-lookup
timeout, while the API Frontend's direct client surfaces the terminal
`unauthorized` state.

---

### 3. Defense-in-Depth: Gateway + RO Validation

**Chosen**: Validate scope at Gateway (fail-fast) AND RemediationOrchestrator (temporal drift protection)

**Validation Points**:

#### **Point 1: Gateway Signal Filtering (Fail-Fast)**
```
Signal arrives → Gateway validates scope → Unmanaged? → Reject (log, metric)
                                        → Managed? → Create RemediationRequest CRD
```

**Gateway Behavior**:
- Check signal source resource/namespace against `kubernaut.ai/managed` label
- Reject unmanaged signals with HTTP 200 response (clear instructions to add label)
- No RemediationRequest CRD created (prevent downstream processing waste)
- Log: `INFO "Rejecting signal from unmanaged resource"`
- Metric: `gateway_signals_rejected_total{reason="unmanaged_resource"}`
- **NO** audit event (reduce noise for expected validation decisions)

#### **Point 2: RO Routing Check #6 (Temporal Drift Protection)**
```
RR exists (created at T0) → RO validates scope at T60 → Unmanaged? → Block RR (retry)
                                                      → Managed? → Create WorkflowExecution
```

**RO Behavior**:
- Re-validate resource scope before creating WorkflowExecution (routing Check #6)
- Block RR if unmanaged: `Status.Phase = Blocked`, `Status.BlockReason = UnmanagedResource`
- Emit audit event: `orchestrator.routing.blocked` (triggers notification)
- Retry with exponential backoff (see Decision #4)

**Rationale**:
- **Gateway Fail-Fast**: Prevents unnecessary CRD creation (50% reduction in RR count for unmanaged signals)
- **RO Defense-in-Depth**: Handles temporal drift (scope can change during approval delays)
- **Temporal Drift Scenario**: Namespace labeled as managed at T0, label removed at T20, approval granted at T30 → RO blocks at T30

---

### 4. Exponential Backoff Retry (Until RR Timeout)

**Chosen**: RO retries scope validation with exponential backoff (5s → 5min max) until RR times out (60 minutes)

**Retry Schedule**:
```
T+0m:     RR blocked (UnmanagedResource)
          └─ Retry scheduled: 5 seconds

T+0m05s:  Retry #1 → Re-validate scope
          ├─ Still blocked? → Retry in 10 seconds
          └─ Unblocked? → Proceed with workflow ✅

T+0m15s:  Retry #2 → Re-validate scope
          ├─ Still blocked? → Retry in 20 seconds
          └─ Unblocked? → Proceed with workflow ✅

T+0m35s:  Retry #3 → Re-validate scope
          ├─ Still blocked? → Retry in 40 seconds
          └─ Unblocked? → Proceed with workflow ✅

T+1m15s:  Retry #4 → Re-validate scope
          ├─ Still blocked? → Retry in 80 seconds
          └─ Unblocked? → Proceed with workflow ✅

...

T+Xm:     Retry #N → Re-validate scope
          ├─ Still blocked? → Retry in 300s (5 min max)
          └─ Unblocked? → Proceed with workflow ✅

T+60m:    RR global timeout reached
          └─ Status: TimedOut (terminal, user action required)
```

**Configuration**:
```yaml
# RemediationOrchestrator Config
retryConfig:
  unmanagedResource:
    initialInterval: 5s         # First retry after 5 seconds
    maxInterval: 300s           # Cap at 5 minutes per retry
    multiplier: 2.0             # Double the interval each retry
```

**Rationale**:
- **Early Retries**: Catch quick fixes (5s, 10s, 20s) when operators are actively labeling resources
- **Graduated Backoff**: Reduce API load as retries continue (40s, 80s, 160s, 300s)
- **Max Cap**: 5 minutes per retry balances responsiveness and API efficiency
- **Global Timeout**: 60 minutes provides eventual failure (prevents infinite retry)
- **Automatic Unblocking**: No user intervention required (Kubernetes-native reconciliation)

---

### 5. Controller-Runtime Metadata-Only Cache (Gateway)

**Chosen**: Gateway uses controller-runtime metadata-only cache (0 API calls), RO uses direct API calls (2 calls per retry)

**Gateway Implementation**:
```go
import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/cache"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// Configure cache for metadata-only (in cmd/gateway/main.go)
cacheOpts := cache.Options{
    DefaultUnsafeDisableDeepCopy: ptr.To(true), // Metadata is read-only
}

mgr, err := ctrl.NewManager(cfg, ctrl.Options{
    Cache: cacheOpts,
})

// Use PartialObjectMetadata for lookups (in pkg/gateway/server.go)
ns := &metav1.PartialObjectMetadata{}
ns.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Namespace"))
err := cachedClient.Get(ctx, client.ObjectKey{Name: "production"}, ns)
// ns.Labels["kubernaut.ai/managed"] → "true"
```

**RO Implementation**:
```go
// Direct API calls (no caching in V1.0)
ns := &corev1.Namespace{}
err := kubeClient.Get(ctx, client.ObjectKey{Name: "production"}, ns)
// ns.Labels["kubernaut.ai/managed"] → "true"
```

**Rationale**:
- **Gateway**: High-frequency validation (thousands of signals per hour) → Cache is essential
- **RO**: Low-frequency validation (only for blocked RRs during retry) → Direct API acceptable
- **Minimal Memory**: Gateway caches only ObjectMeta (no spec/status), ~100 bytes per namespace
- **Zero API Calls**: Gateway reads from in-memory cache (sub-millisecond latency)

---

### 6. Non-Terminal Blocking Phase

**Chosen**: Use `Blocked` phase (non-terminal) for UnmanagedResource, not `Failed` phase (terminal)

**State Transition**:
```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationRequest
status:
  overallPhase: Blocked  # NON-terminal (Gateway can deduplicate against this)
  blockReason: UnmanagedResource
  blockMessage: "Resource production/deployment/payment-api is not managed by Kubernaut at routing time. Add label 'kubernaut.ai/managed=true' to namespace or resource to unblock."
  retryAttempts: 3
  nextRetryTime: "2026-01-20T10:31:00Z"
```

**Gateway Deduplication Behavior**:
```
Scenario: 10 duplicate signals arrive while RR is blocked (UnmanagedResource)

With Blocked (non-terminal):
- Gateway deduplicates against existing Blocked RR (fingerprint match) ✅
- Only 1 RR in system (efficient)
- Occurrence count incremented (preserves signal frequency data)

With Failed (terminal):
- Gateway creates 10 new RRs (flood) ❌
- Each RR blocked by RO → 10× validation overhead
- 10× API load, 10× audit events, 10× notifications
```

**Rationale**:
- **Gateway Deduplication**: Non-terminal state allows Gateway to deduplicate duplicate signals
- **Automatic Unblocking**: RO continues retrying, unblocks when label is added
- **Consistent with DD-RO-002-ADDENDUM**: UnmanagedResource is 6th blocking scenario (joins ConsecutiveFailures, ResourceBusy, RecentlyRemediated, ExponentialBackoff, DuplicateInProgress)

---

### 7. No Gateway Audit Events for Unmanaged Signals

**Chosen**: Gateway logs + Prometheus metrics only (NO audit events for rejected signals)

**Gateway Observability**:
```go
// Log rejection
logger.Info("Rejecting signal from unmanaged resource",
    "signal_name", signal.Name,
    "namespace", signal.Namespace,
    "resource_kind", signal.ResourceKind,
    "resource_name", signal.ResourceName,
    "reason", "unmanaged_resource",
)

// Increment Prometheus metric
gateway_signals_rejected_total.WithLabelValues(
    "unmanaged_resource",
    signal.Namespace,
    signal.Type,
).Inc()

// NO audit event emitted
```

**RO Audit Event** (for blocked RRs):
```json
{
  "event_type": "orchestrator.routing.blocked",
  "event_action": "blocked",
  "correlation_id": "rr-a1b2c3d4e5f6-12345678",
  "event_data": {
    "block_reason": "UnmanagedResource",
    "resource": {
      "namespace": "production",
      "kind": "Deployment",
      "name": "payment-api"
    }
  }
}
```

**Rationale**:
- **Reduce Audit Noise**: Unmanaged signals are expected validation decisions, not business events
- **Gateway Observability**: Prometheus metrics + structured logs provide sufficient visibility
- **RO Audit**: RO emits audit event for blocked RRs (triggers notification to user)

---

## Alternatives Considered

### Alternative 1: Owner Chain Traversal (Rejected)

**Proposed**: Check Pod → ReplicaSet → Deployment → Namespace (5 levels)

**Rejected Rationale**:
- **Complexity**: Requires traversing owner references (4-5 levels deep)
- **Performance**: Up to 5 API calls per validation vs 2
- **Edge Cases**: Orphaned resources (no owner), cross-namespace owners (StatefulSet + PVC)
- **Maintenance Burden**: Owner reference chain can break, requires error handling

**Decision**: Stick with 2-level hierarchy (Resource → Namespace)

---

### Alternative 2: Existence-Only Labels (Rejected)

**Proposed**: `kubernaut.ai/managed` (label existence = managed)

**Rejected Rationale**:
- **Less Explicit**: Can't distinguish "unset" from "explicitly false"
- **Cluster Tool Compatibility**: Kyverno, OPA, admission webhooks expect values
- **Audit Trail**: Value-based labels provide clearer operator intent
- **Future Extensibility**: Can't add values like `"dry-run"` with existence-only

**Decision**: Use value-based labels (`kubernaut.ai/managed="true"`)

---

### Alternative 3: Gateway-Only Validation (Rejected)

**Proposed**: Validate scope at Gateway only, no RO re-validation

**Rejected Rationale**:
- **Temporal Drift Risk**: Scope can change during approval delays (30+ minutes)
- **Real-World Scenario**:
  ```
  T+0m:  Namespace is managed → Gateway creates RR
  T+20m: Admin removes label: kubectl label ns production kubernaut.ai/managed-
  T+30m: Operator approves remediation
  T+30m: ❌ PROBLEM: RO would execute on unmanaged namespace
  ```
- **No Safety Net**: Single point of failure for scope validation

**Decision**: Implement defense-in-depth (Gateway + RO validation)

---

### Alternative 4: Fixed-Interval Retry (Rejected)

**Proposed**: Retry every 5 minutes (fixed interval)

**Rejected Rationale**:
- **Missed Quick Fixes**: Operators labeling resources actively must wait 5 minutes for first retry
- **API Waste**: 5 minutes every time, even when operators are done labeling after 10 seconds
- **Poor UX**: Long initial wait time frustrates operators fixing configuration

**Decision**: Use exponential backoff (5s → 10s → 20s → ... → 5min max)

---

### Alternative 5: Terminal Failure for Unmanaged Resources (Rejected)

**Proposed**: Set `Status.Phase = Failed` (terminal) for unmanaged resources

**Rejected Rationale**:
- **Gateway Deduplication Breaks**: Gateway can't deduplicate against terminal RRs
- **RR Flood**: 10 duplicate signals = 10 failed RRs (all created and failed)
- **API Load**: 10× CRD creation, 10× validation, 10× audit events
- **User Confusion**: 10 identical failed RRs instead of 1 blocked RR with occurrence count

**Decision**: Use non-terminal `Blocked` phase with `BlockReasonUnmanagedResource`

---

### Alternative 6: RemediationScope CRD (Deferred to V2.0)

**Proposed**: Create a `RemediationScope` CRD for complex scope rules

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationScope
metadata:
  name: cluster-scope-config
spec:
  namespaceSelector:
    matchLabels:
      environment: production
  clusterScoped:
    - resourceType: Node
      selector:
        matchLabels:
          node-role.kubernetes.io/worker: "true"
```

**Deferred Rationale**:
- **V1.0 Scope**: Namespace-level labels sufficient for 95% of use cases
- **Complexity**: CRD-based config adds complexity (CRUD API, validation, defaulting)
- **Future Extension**: Can add in V2.0 without breaking V1.0 label-based config

**Decision**: V1.0 uses labels only, V2.0 can add CRD-based config if needed

---

## Consequences

### Positive

- ✅ **Clear Security Boundary**: Opt-in model prevents accidental remediations
- ✅ **Fail-Fast**: Gateway rejects unmanaged signals early (50% reduction in RR count)
- ✅ **Temporal Drift Protection**: RO re-validates scope (handles label changes during approval)
- ✅ **Automatic Unblocking**: Exponential backoff retry unblocks when label is added
- ✅ **Gateway Deduplication Works**: Non-terminal Blocked phase prevents RR flood
- ✅ **Acceptable API Load**: 0.67 GET/second at scale (100 blocked RRs)
- ✅ **Kubernetes-Native**: Uses standard labels (no external config)
- ✅ **Operator Control**: Resource-level override for exceptions
- ✅ **Observable**: Prometheus metrics, structured logs, audit events

### Negative

- ⚠️ **2× Validation Overhead**: Scope checked at Gateway AND RO (defense-in-depth tradeoff)
- ⚠️ **More Complex State Machine**: 6 blocking scenarios (was 5), more reconciliation logic
- ⚠️ **Different Patterns**: Gateway uses cache (0 API calls), RO uses direct API (2 calls/retry)
- ⚠️ **Manual Labeling Required**: Operators must label namespaces/resources (not automatic)

### Neutral

- ℹ️ **Cluster-Scoped Resources (Option A, #2670)**: labelable cluster-scoped targets (Node, PersistentVolume, labeled cluster-scoped CRDs, Namespace objects) authorize through their own `kubernaut.ai/managed` label at level 1; the `RemediationScope` CRD (FR-SCOPE-004 Option B) remains deferred to V2.0
- ℹ️ **V1.0 Uses Direct API for RO**: V2.0 can add metadata-only cache if needed
- ℹ️ **V1.0 Uses Simple 2-Level Hierarchy**: V2.0 can add CRD-based config for complex rules

---

## Performance Analysis

### Gateway Scope Validation (Per Signal)

```
Namespaced Resource (e.g., Deployment/production/payment-api):
├─ Get resource metadata: 0 API calls (controller-runtime cache)
└─ Get namespace metadata: 0 API calls (controller-runtime cache)
Total: 0 API calls ✅
Latency: < 1ms (in-memory map lookup)

Cluster-Scoped Resource (e.g., Node/worker-01):
└─ Get resource metadata: 0 API calls (controller-runtime cache)
Total: 0 API calls ✅
Latency: < 1ms (in-memory map lookup)
```

**Gateway at Scale** (10,000 signals/hour):
- Total API calls: 0 (all reads from cache)
- Cache memory: ~100 bytes × 100 namespaces = 10 KB
- Latency impact: < 10ms per signal (P95 target: met ✅)

---

### RO Scope Validation (Per Retry)

```
Namespaced Resource (e.g., Deployment/production/payment-api):
├─ Get resource metadata: 1 GET call (direct API, V1.0)
└─ Get namespace metadata: 1 GET call (direct API, V1.0)
Total: 2 GET calls per retry

Cluster-Scoped Resource (e.g., Node/worker-01):
└─ Get resource metadata: 1 GET call (direct API, V1.0)
Total: 1 GET call per retry
```

**RO Retry Frequency** (Exponential Backoff):
```
Worst-case (1 blocked RR over 60 minutes):
- Retries: 12 attempts (5s, 10s, 20s, 40s, 80s, 160s, 300s × 7)
- API calls per retry: 2 (resource + namespace)
- Total: 24 GET calls over 60 minutes = 0.4 GET/minute

At scale (100 blocked RRs):
- Total: 2,400 GET calls over 60 minutes
- Rate: 40 GET/minute = 0.67 GET/second ✅
```

**Assessment**: Acceptable API load for defensive validation (< 1 GET/second)

---

## Implementation

### Components Affected

#### **1. Shared Scope Manager**
- **File**: `pkg/shared/scope/manager.go`
- **Purpose**: Reusable scope validation logic (shared by Gateway and RO)
- **Interface**:
  ```go
  type Manager struct {
      client client.Client  // Gateway uses cached client, RO uses direct client
  }

  func (m *Manager) IsManaged(ctx context.Context, namespace, kind, name string) (bool, error)
  ```

#### **2. Gateway Integration**
- **Files**:
  - `cmd/gateway/main.go`: Configure controller-runtime metadata-only cache
  - `pkg/gateway/server.go`: Integrate `scopeManager.IsManaged()` in `ProcessSignal()`
  - `pkg/gateway/metrics.go`: Add `gateway_signals_rejected_total` metric
- **Behavior**: Reject unmanaged signals before CRD creation

#### **3. RO Integration**
- **Files**:
  - `pkg/remediationorchestrator/routing/scope_validator.go`: RO-specific scope validator
  - `pkg/remediationorchestrator/routing/blocking.go`: Add Check #6 (scope validation)
  - `internal/controller/remediationorchestrator/reconciler.go`: Add exponential backoff retry logic
- **Behavior**: Block RRs for unmanaged resources, retry with exponential backoff

#### **4. API Types**
- **File**: `api/remediation/v1alpha1/remediationrequest_types.go`
- **Changes**:
  ```go
  const (
      // ... existing block reasons ...
      BlockReasonUnmanagedResource BlockReason = "UnmanagedResource"
  )

  // Updated PhaseBlocked comment (6 scenarios, was 5)
  PhaseBlocked RemediationPhase = "Blocked"  // 6 scenarios: ..., UnmanagedResource
  ```

---

### Testing Strategy

#### **Unit Tests**
- `test/unit/gateway/scope_validation_test.go` (10+ test cases)
  - Managed namespace (inherited)
  - Unmanaged namespace (rejected)
  - Resource-level override (opt-out)
  - Cluster-scoped resources
  - Cache miss scenarios
- `test/unit/remediationorchestrator/scope_validation_test.go` (15+ test cases)
  - Blocking for unmanaged resources
  - Exponential backoff retry schedule
  - Automatic unblocking when label is added
  - Timeout after 60 minutes

#### **Integration Tests**
- `test/integration/gateway/scope_filtering_test.go`
  - Signal rejection for unmanaged namespaces
  - CRD creation for managed namespaces
  - Prometheus metrics validation
- `test/integration/remediationorchestrator/scope_blocking_test.go`
  - Temporal drift scenario (scope changes during approval)
  - Exponential backoff retry behavior
  - Audit event emission

---

### Performance Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Gateway Latency** | < 10ms added (P95) | Prometheus histogram |
| **RO Latency** | < 10ms added (P95) | Prometheus histogram |
| **CRD Reduction** | > 50% fewer RRs for unmanaged signals | Compare RR count before/after |
| **False Rejections** | < 0.1% managed signals rejected | `signals_rejected_total` / `signals_received_total` |
| **Auto-Unblock Rate** | > 80% blocked RRs unblock before timeout | `blocked_duration` histogram |
| **API Load** | < 1 GET/second at scale | Kubernetes API server metrics |
| **Cache Hit Rate** | > 99% Gateway scope lookups from cache | controller-runtime metrics |

---

## Migration Plan

### Phase 1: Infrastructure (Week 1)
1. Implement `pkg/shared/scope/manager.go`
2. Add unit tests for scope manager
3. Create configuration structs for retry behavior

### Phase 2: Gateway Integration (Week 2)
1. Configure controller-runtime metadata-only cache in `cmd/gateway/main.go`
2. Integrate scope validation in `pkg/gateway/server.go`
3. Add Prometheus metric `gateway_signals_rejected_total`
4. Add unit and integration tests

### Phase 3: RO Integration (Week 3)
1. Implement `pkg/remediationorchestrator/routing/scope_validator.go`
2. Add Check #6 to `CheckBlockingConditions()`
3. Add exponential backoff retry logic to RO reconciler
4. Add unit and integration tests

### Phase 4: Documentation (Week 4)
1. Update DD-RO-002-ADDENDUM (6th blocking scenario)
2. Create user guide (`docs/user-guide/scope-management.md`)
3. Update service documentation (Gateway, RO)
4. Update API reference documentation

### Rollback Plan
- **Phase 2 Rollback**: Remove Gateway scope validation (all signals create RRs)
- **Phase 3 Rollback**: Remove RO Check #6 (no scope validation at routing)
- **No Data Loss**: Existing RRs unaffected by rollback

---

## References

### Business Requirements
- **BR-SCOPE-001**: `docs/requirements/BR-SCOPE-001-resource-scope-management.md`
- **BR-SCOPE-002**: `docs/requirements/BR-SCOPE-002-gateway-signal-filtering.md`
- **BR-SCOPE-010**: `docs/requirements/BR-SCOPE-010-ro-routing-validation.md`

### Related ADRs
- **ADR-001**: CRD Microservices Architecture (spec immutability)
- **DD-RO-002-ADDENDUM**: Blocked Phase Semantics (6 blocking scenarios)
- **DD-GATEWAY-009**: State-Based Deduplication (non-terminal Blocked phase)

### External Documentation
- **Kubernetes Labels**: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
- **Controller-Runtime Cache**: https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/cache
- **PartialObjectMetadata**: https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#PartialObjectMetadata

---

## Approval

**Approved By**: Platform Team, Gateway Team, RemediationOrchestrator Team
**Date**: January 20, 2026
**Confidence**: 95%

**Key Decisions Approved**:
1. ✅ Label domain: `kubernaut.ai/managed` (matches API group)
2. ✅ Value-based labels: `"true"` (not existence-only)
3. ✅ 2-level hierarchy: Resource → Namespace (no owner chain)
4. ✅ Defense-in-depth: Gateway + RO validation
5. ✅ Exponential backoff: 5s → 5min max, until RR timeout (60min)
6. ✅ Controller-runtime cache: Gateway uses metadata-only cache (0 API calls)
7. ✅ Non-terminal blocking: Enables Gateway deduplication
8. ✅ No Gateway audit events: Logs + metrics only

**Next Steps**:
1. Begin Phase 1 implementation (Shared Scope Manager)
2. Schedule design review with all affected teams
3. Create E2E test scenarios for temporal drift
4. Update user documentation with labeling guide

---

---

## Addendum: V1.0 Implementation Decisions (February 6, 2026)

### Cache Strategy Revision

The original V1.0 plan allowed the RO to use direct API calls for scope validation as a simplification.
This has been revised: **both Gateway and RO use metadata-only cached clients** (controller-runtime's
lazy informer mechanism for `PartialObjectMetadata`).

**Rationale**: The RO validates scope for every AA completion that is valid for forwarding to the
WorkflowExecution service, not just for blocked RR retries. Using direct API calls at this frequency
would create unnecessary load on the API server. The cached client provides consistent, low-latency
lookups for both services.

### ScopeChecker Interface (DI Pattern)

A shared `ScopeChecker` interface (`pkg/shared/scope/checker.go`) is used for dependency injection
in both Gateway and RO. This follows the same mandatory DI pattern as `processing.RetryObserver`:
- Production: `*scope.Manager` (backed by metadata-only cache)
- Tests: mock implementations (`AlwaysManagedScopeChecker`, `NeverManagedScopeChecker`)
- Constructor enforcement: `nil` panics (programming error in bootstrap)

### RO Check Priority (Check #1)

`CheckUnmanagedResource` is the **first** check in the RO's `CheckBlockingConditions()` pipeline,
not the last. If a resource is unmanaged, all other blocking checks (consecutive failures, rate limits,
circuit breakers, maintenance windows, exponential backoff) are irrelevant. This avoids wasted
computation and gives operators a clear, immediate signal about the blocking reason.

### Unknown Kind Resilience

The `scope.Manager.checkResourceLabel()` now handles unknown resource kinds and non-NotFound API
errors gracefully:
- Unknown kind (not in `kindToGroup`): skip resource-level check, fall through to namespace
- Forbidden / connection errors: graceful fallthrough to namespace with Info-level log
- This ensures scope validation degrades to namespace-level when resource-level is not possible

### Cache Pre-warming (Deferred)

Cache pre-warming (eagerly starting informers for known GVKs at startup) is deferred to v1.1/v2.0.
V1.0 accepts the lazy informer cold-start latency (200ms-2s per resource type on first access).
A `ScopeGVKs()` helper in `pkg/shared/scope/cache.go` exports the known GVKs for future pre-warming.

### Namespace Fallback Deprecation (DD-GATEWAY-007)

As a direct consequence of scope management, the Gateway's namespace fallback feature
(DD-GATEWAY-007) has been **deprecated and removed** from the codebase (February 2026).

**Background**: DD-GATEWAY-007 defined a fallback behavior where signals targeting non-existent
namespaces would have their RemediationRequest CRDs created in `kubernaut-system` with
origin-namespace labels. This was designed for cluster-scoped signals and deleted namespaces.

**Why Deprecated**: With ADR-053 scope validation running as the first step in the Gateway
pipeline, signals to unmanaged or non-existent namespaces are now rejected with HTTP 200
(informational rejection) before CRD creation is ever attempted. Additionally, the RO's
`CheckUnmanagedResource` (Check #1, BR-SCOPE-010) would block any fallback-created RRs
since the underlying resource would lack the `kubernaut.ai/managed=true` label.

**Decision**: Creating a RemediationRequest in a fallback namespace for a resource whose
original namespace no longer exists serves no purpose -- the RO cannot remediate it. Removing
the fallback eliminates technical debt and simplifies the CRD creation path.

**Removed Code**: `handleNamespaceNotFoundError()`, `isNamespaceNotFoundError()`,
`FallbackNamespace` config field, `GetPodNamespace()`, and all associated tests.

---

### Fleet 3-Level Scope Hierarchy (DD-FLEET-001, Issue #54 P1)

When fleet mode is enabled, `FederatedScopeChecker` adds a **cluster-level
precondition** to the scope check pipeline:

```
1. Cluster managed?  (ClusterLookup.IsKnownCluster)
2. Resource managed? (ScopeChecker.IsManagedResource — local or remote)
3. Namespace managed? (implicit in resource scope check)
```

> **Superseded (#2662):** the `IsManagedResource` boolean lanes are deleted; the typed `Check`/`Resolve` contracts carry this hierarchy.


The `ClusterLookup` interface is injected via `WithClusterLookup` option on
`NewFederatedScopeChecker`. The production implementation uses
`BackendInformerRegistry` (watching Backend CRDs with `kubernaut.ai/managed` label)
adapted via `ClusterLookupAdapter`.

If a signal arrives with a `clusterID` that is not registered in the cluster
registry, the resource is immediately classified as unmanaged without making
any remote calls. This prevents unnecessary MCP tool calls to unknown clusters
and provides fail-fast behavior.

See DD-FLEET-001 for full design details.

---

## Addendum: Point 3 — AF Tool-Layer Validation (Issue #2025, August 2026)

**Cloned from Issue #2022 (`release/v1.5`) for `main` tracking.** The gap and fix design below are
identical on both branches; only file paths/wiring differ where `main` has since diverged (notably:
`main`'s `ScopeChecker.IsManagedResource` already takes a `scope.ResourceIdentity{ClusterID, ...}`
rather than a bare namespace/kind/name triple, and `main` wires the checker through
`fleet.NewScopeChecker` rather than a bare `scope.Manager`, so this addendum gets Fleet's federated
local+remote checks for free with no extra AF-side code).

### Gap Identified

Decision #3 above established defense-in-depth at two points: Gateway (Point 1, fail-fast for
**signal-initiated** RRs) and RO (Point 2, temporal-drift re-validation for **all** RRs). Neither
point covers RRs created by API-frontend's agent tools (`kubernaut_investigate_alert`,
`kubernaut_investigate`, `kubernaut_remediate`) — these are **agent-initiated**, not
signal-initiated, so they never pass through Gateway's `ProcessSignal()` pipeline at all. The only
existing protection for this path was Point 2 (RO), which only fires *after* the RemediationRequest
CRD (and, for the interactive tools, an InvestigationSession CRD) has already been created — wasting
a tool call, a CRD, and an interactive session, then blocking silently with no signal surfaced back
to the calling agent (#2022/#2025).

### Point 3: AF Tool-Layer Validation (Fail-Fast for Agent-Initiated RRs)

**Chosen**: Apply the same `ScopeChecker.IsManagedResource()` check (shared `pkg/shared/scope`
package, Decision #4 "ScopeChecker Interface (DI Pattern)" above) at the AF tool layer, symmetric to
Gateway's Point 1, before any RR/session is created — not after, as a routing-time re-check.

```
Tool call arrives (kubernaut_investigate_alert / kubernaut_investigate / kubernaut_remediate)
  → AF validates scope → Unmanaged? → Reject: CreateRRResult error (ErrResourceNotManaged) + explanatory message, no CRD created
                        → Managed?   → Proceed to HandleCreateRR (existing behavior)
```

**AF Behavior**:
- Check target resource/namespace (as a `scope.ResourceIdentity`, including `ClusterID` for
  fleet-federated targets) against `kubernaut.ai/managed` before calling `HandleCreateRR`.
- Reject unmanaged targets with `ErrResourceNotManaged` wrapping an explanatory message mirroring
  RO's own block wording (`routing/blocking.go`), not a generic tool error — the calling agent gets
  an actionable, non-ambiguous signal instead of an empty session.
- No RemediationRequest or InvestigationSession CRD created (prevents the exact waste #2022/#2025
  reported: a CRD + interactive session created for a resource RO was always going to block).
- Audit event: `rr.scope_rejected` (`EventRRScopeRejected`) — parallel to RO's
  `orchestrator.routing.blocked`, giving agent-initiated rejections the same audit visibility
  signal-initiated ones lack at Gateway (Decision #7 above explicitly omits Gateway audit events
  for expected rejections; AF's is a genuinely new event type, not a gap-fill of an existing one).

**Rationale**:
- **Symmetry with Point 1**: AF's agent-initiated path deserved the same fail-fast treatment
  Gateway already gives the signal-initiated path; the gap was an omission, not a design choice.
- **No new validation logic**: reuses the existing `scope.ScopeChecker` interface and
  `IsManagedResource` contract unchanged — this is a new *caller*, not a new *validator*.
- **Fleet-aware for free**: `main`'s checker is constructed via `fleet.NewScopeChecker` (the same
  factory RO/Gateway use), which itself decides whether to wrap the local `scope.Manager` with a
  `fleet.FederatedScopeChecker` based on `cfg.Fleet.Enabled` — AF's tool layer never has to know
  whether the target is local or remote.
- **Client caching accepted as a trade-off**: unlike Gateway's metadata-only cache (Decision #5),
  the local `scope.Manager` layer underneath uses an uncached `crclient.WithWatch` (`TypedClient`),
  so each check is a live API read for local targets. This mirrors RO's V1.0 direct-API approach
  (before the cache-strategy revision in the addendum above) and is acceptable given AF's low
  interactive-tool-call volume relative to Gateway's signal volume.

### FedRAMP Controls

| Control | Application | Evidence |
|---|---|---|
| AC-6 (Least Privilege) | AF declines to exercise its elevated CRD-creation capability (AF's own ServiceAccount, not the caller's RBAC — see `agent/root.go` SA delegation comment) for resources outside its declared management scope | UT-AF-2025-001..040 |
| SI-10 (Information Input Validation) | Target namespace/kind/name (as a `scope.ResourceIdentity`) is validated against declared scope alongside the existing `validate.*` checks already present in these same tool handlers | UT-AF-2025-001..040 |
| AU-3 / AU-12 (Audit Content / Generation) | New `EventRRScopeRejected` audit event captures namespace/kind/name/user for every rejected attempt — closes an audit gap where nothing previously distinguished "rejected for scope" from silence at this layer | UT-AF-2025-004, -013, -023 |
| SI-11 (Error Handling) | Rejection returns `ErrResourceNotManaged` + explanatory message instead of a misleading empty/silent session, mirroring RO's own block wording | UT-AF-2025-001, -005, -010, -020 |

### Components Affected (`main`)

- `pkg/apifrontend/tools/scope_helpers.go`: shared `checkRRScope` helper (takes a
  `scope.ResourceIdentity`) + `ContextWithScopeChecker`/`ScopeCheckerFromContext`.
- `pkg/apifrontend/tools/af_create_rr.go`: single pre-triage scope gate in `HandleCreateRR`, reused
  by all three tools below (they all funnel through `HandleCreateRR`).
- `pkg/apifrontend/tools/af_investigate_alert.go`, `ka_remediate.go`, `ka_investigate_mcp.go`:
  `ScopeChecker` threaded into `ToolDeps`/`InvestigateConfig` and forwarded to `HandleCreateRR`.
- `pkg/apifrontend/agent/root.go`, `pkg/apifrontend/handler/mcp_bridge.go`: wiring for both AF
  transports (A2A/ADK and raw MCP bridge — `kubernaut_investigate` is registered in both,
  independently).
- `cmd/apifrontend/backend_deps.go`: `buildScopeCheckerDeps` constructs the checker via
  `fleet.NewScopeChecker(scope.NewManager(k8sTypedClient), cfg.Fleet, logger)`, degrading to a nil
  `ScopeChecker` (scope validation skipped) when the K8s typed client is unavailable.
- `pkg/apifrontend/audit/audit.go`: new `EventRRScopeRejected`.

---

**Document Version**: 1.3
**Last Updated**: June 25, 2026
**Next Review**: September 25, 2026 (3 months)
