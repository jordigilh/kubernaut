# ADR-052: Kubernetes Lease-Based Distributed Locking Pattern

**Status**: ✅ **APPROVED**
**Date**: December 30, 2025
**Last Updated**: December 30, 2025
**Applies To**: Multi-replica services with check-then-create operations
**Business Requirements**:
- [BR-GATEWAY-190](../../requirements/BR-GATEWAY-190.md) - Multi-Replica Deduplication Safety (Gateway)
- [BR-ORCH-050](../../requirements/BR-ORCH-050.md) - Multi-Replica Resource Lock Safety (RemediationOrchestrator)

---

## Context

### The Multi-Replica Race Condition Problem

**Scenario**: Services designed to scale horizontally (multiple replicas) encounter race conditions in check-then-create operations when multiple pods process concurrent requests.

**Example - Gateway Service**:
```
3 Gateway Pods Running in Production
═══════════════════════════════════════

Alert Storm: 100 concurrent requests with SAME fingerprint
  ├─ 33 requests → gateway-pod-1
  ├─ 34 requests → gateway-pod-2
  └─ 33 requests → gateway-pod-3

Each pod independently:
  1. Checks if RemediationRequest exists → false (race window)
  2. Generates CRD name with timestamp
  3. Creates RemediationRequest

Problem: If requests span second boundary, multiple RRs created! ❌
```

**Example - RemediationOrchestrator Service**:
```
2 RO Pods Processing Different RemediationRequests
═══════════════════════════════════════════════════

RO Pod 1: Processes RR-1 (target: Node/worker-1)
RO Pod 2: Processes RR-2 (target: Node/worker-1)

Both pods concurrently:
  1. Check for active WorkflowExecution → none found (race window ~10-50ms)
  2. Validate no resource lock → passes
  3. Create WorkflowExecution CRD

Problem: Two WorkflowExecutions targeting same resource! ❌
```

### Why Current Protection Mechanisms Fail

**Layer 1 (K8s Query Check)**: ❌ **NOT SUFFICIENT**
- Each pod queries K8s independently
- K8s API latency varies per pod (~10-50ms)
- No coordination between pods
- Race window: Both pods query before either creates resource

**Layer 2 (Optimistic Concurrency)**: ✅ **WORKS** (for status updates)
- Handles concurrent status updates correctly
- But doesn't prevent duplicate CRD creation

**Layer 3 (K8s Atomic Creation)**: ⚠️ **PARTIAL PROTECTION**
- Only prevents same-second races (same CRD name)
- Cross-second races have different names → both succeed

### Business Impact

**Duplicate Creation Rate** (estimated):
- Single pod: ~0.01% of concurrent requests
- 3 replicas: ~0.03% of concurrent requests
- 10 replicas: ~0.1% of concurrent requests

**At Scale** (10,000 alerts/day with 10% concurrent):
- 1,000 concurrent scenarios/day
- ~1 duplicate created/day (0.1% of 1,000)
- ~30 duplicates created/month
- ~365 duplicates created/year

**Impact by Operation Type**:

| Operation Type | Idempotent? | Duplicate Impact | Severity |
|---|---|---|---|
| **Restart Pod** | ✅ Yes | Multiple restarts (minor disruption) | Low |
| **Scale Deployment** | ✅ Yes | Multiple scale operations (eventual consistency) | Low |
| **Delete/Create Resource** | ❌ No | Resource thrashing, data loss | **High** |
| **Data Migration** | ❌ No | Duplicate migration, corruption | **Critical** |
| **Failover** | ❌ No | Cascading failures | **Critical** |

### Why Not a Shared Library?

**YAGNI Principle Applied**: While distributed locking is a common pattern, creating a shared library adds complexity that isn't justified for 2 services:

**Metrics Coupling Complexity**:
```go
// Shared library would need:
type DistributedLockManager struct {
    client  client.Client
    metrics MetricsRecorder  // ← Abstract interface needed
}

// Each service implements:
type MetricsRecorder interface {
    RecordLockFailure(reason string)
    RecordLockLatency(duration time.Duration)
}

// Problems:
// 1. Metrics registry differs per service
// 2. Metric names/labels differ per service
// 3. Optional vs. required metrics (awkward API)
// 4. Testing complexity (mock metrics in shared tests)
// 5. Versioning (changes affect both services)
```

**Service-Specific Customization**:
- **Gateway**: Lock key = signal fingerprint (hash/truncate for long keys)
- **RO**: Lock key = target resource (already short, K8s-compatible)
- **Different logic**: Shared library would need pluggable key generation

**Team Coordination Overhead**:
- Shared library means cross-team code review for any changes
- Gateway team blocked on RO team (or vice versa) for updates
- More overhead than value for ~200 lines of code

**Decision**: Each service implements the pattern independently. If 3+ services adopt this pattern, revisit shared library decision.

---

## Decision

**APPROVED: Kubernetes Lease-Based Distributed Locking Pattern** (independent implementations)

### Pattern Overview

**Mechanism**: Use Kubernetes Lease resources (`coordination.k8s.io/v1`) for distributed mutual exclusion.

**Key Concepts**:
1. **Acquire Lock**: Pod attempts to create/update Lease with its identity as holder
2. **Hold Lock**: Pod holds lock for operation duration (protected by lease timeout)
3. **Release Lock**: Pod deletes Lease after operation completes

**Architecture**:
```
Pod 1                          Pod 2
─────────────────              ─────────────────
Request arrives                Request arrives
  ↓                              ↓
Acquire Lease on key           Acquire Lease on key
  ├─ Lease: "lock-key-abc123"   ├─ Lease: "lock-key-abc123"
  ├─ Holder: "pod-1"             ├─ Holder: ??? (conflict!)
  ├─ Duration: 30s               ├─ ERROR: Lease held by pod-1
  └─ SUCCESS ✅                  └─ RETRY (backoff 100ms) ⏳
    ↓                              ↓
Perform operation              Wait for lock
  ↓                              ↓
Release Lease                  Retry: Acquire Lease
  ↓                              ├─ Resource now exists
HTTP 201/202                    └─ Update status ✅
```

### Implementation Approach

**Pattern**: Each service copies and adapts the pattern for its specific needs.

**Reference Implementations**:
- **Gateway**: [`pkg/gateway/processing/distributed_lock.go`](../../pkg/gateway/processing/distributed_lock.go)
- **RemediationOrchestrator**: [`pkg/remediationorchestrator/locking/distributed_lock.go`](../../pkg/remediationorchestrator/locking/distributed_lock.go) *(to be implemented)*

### Core Pattern Components

#### 1. Lock Manager Structure

```go
// Service-specific package (e.g., pkg/gateway/processing/ or pkg/remediationorchestrator/locking/)
package processing

import (
    coordinationv1 "k8s.io/api/coordination/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// DistributedLockManager manages K8s Lease-based distributed locks
type DistributedLockManager struct {
    client        client.Client
    namespace     string        // Namespace for Lease resources
    holderID      string        // Pod name or unique identifier
    leaseDuration time.Duration // Default: 30 seconds
}

// NewDistributedLockManager creates a new lock manager
func NewDistributedLockManager(
    client client.Client,
    namespace string,
    holderID string,
    leaseDuration time.Duration,
) *DistributedLockManager {
    return &DistributedLockManager{
        client:        client,
        namespace:     namespace,
        holderID:      holderID,
        leaseDuration: leaseDuration,
    }
}
```

#### 2. Lock Acquisition

```go
// AcquireLock attempts to acquire a lock for the given key
// Returns:
//   - (true, nil): Lock acquired successfully
//   - (false, nil): Lock held by another pod (not an error, caller should retry)
//   - (false, error): K8s API error (communication failure, permission denied, etc.)
func (m *DistributedLockManager) AcquireLock(ctx context.Context, lockKey string) (bool, error) {
    leaseName := generateLeaseName(lockKey) // Service-specific key generation

    lease := &coordinationv1.Lease{}
    err := m.client.Get(ctx, client.ObjectKey{
        Namespace: m.namespace,
        Name:      leaseName,
    }, lease)

    now := metav1.Now()
    leaseDurationSeconds := int32(m.leaseDuration.Seconds())

    if err != nil {
        // IMPORTANT: Check error type explicitly
        if !apierrors.IsNotFound(err) {
            // Real error (API down, permission denied, etc.)
            return false, fmt.Errorf("failed to check lease: %w", err)
        }

        // Lease doesn't exist - create it
        lease = &coordinationv1.Lease{
            ObjectMeta: metav1.ObjectMeta{
                Name:      leaseName,
                Namespace: m.namespace,
            },
            Spec: coordinationv1.LeaseSpec{
                HolderIdentity:       &m.holderID,
                LeaseDurationSeconds: &leaseDurationSeconds,
                AcquireTime:          &now,
                RenewTime:            &now,
            },
        }

        if err := m.client.Create(ctx, lease); err != nil {
            if apierrors.IsAlreadyExists(err) {
                // Another pod created lease first (expected race)
                return false, nil
            }
            return false, fmt.Errorf("failed to create lease: %w", err)
        }

        return true, nil
    }

    // Lease exists - check if we hold it or if it's expired
    if lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity == m.holderID {
        // We already hold this lock (idempotent)
        return true, nil
    }

    // Check if lease expired (allow takeover)
    if isLeaseExpired(lease) {
        lease.Spec.HolderIdentity = &m.holderID
        lease.Spec.RenewTime = &now

        if err := m.client.Update(ctx, lease); err != nil {
            if apierrors.IsConflict(err) {
                // Another pod took over first (expected race)
                return false, nil
            }
            return false, fmt.Errorf("failed to takeover lease: %w", err)
        }

        return true, nil
    }

    // Lease held by another pod and not expired
    return false, nil
}
```

#### 3. Lock Release

```go
// ReleaseLock releases the lock for the given key
// Idempotent: Safe to call even if lock not held
func (m *DistributedLockManager) ReleaseLock(ctx context.Context, lockKey string) error {
    leaseName := generateLeaseName(lockKey)

    lease := &coordinationv1.Lease{}
    lease.Namespace = m.namespace
    lease.Name = leaseName

    // Delete lease (allows garbage collection)
    err := m.client.Delete(ctx, lease)
    if err != nil && !apierrors.IsNotFound(err) {
        return fmt.Errorf("failed to delete lease: %w", err)
    }

    return nil
}
```

#### 4. Service Integration

**Gateway Example**:
```go
// pkg/gateway/server.go
func (s *Server) ProcessSignal(ctx context.Context, signal *types.NormalizedSignal) (*ProcessingResponse, error) {
    // 1. Acquire distributed lock
    lockAcquired, err := s.lockManager.AcquireLock(ctx, signal.Fingerprint)
    if err != nil {
        return nil, fmt.Errorf("lock acquisition failed: %w", err)
    }

    if !lockAcquired {
        // Lock held by another pod - retry with backoff
        time.Sleep(100 * time.Millisecond)
        return s.ProcessSignal(ctx, signal) // Retry
    }

    // Ensure lock released
    defer s.lockManager.ReleaseLock(ctx, signal.Fingerprint)

    // 2. Check for duplicates (protected by lock)
    shouldDeduplicate, existingRR, err := s.phaseChecker.ShouldDeduplicate(ctx, signal.Namespace, signal.Fingerprint)
    if shouldDeduplicate && existingRR != nil {
        s.statusUpdater.UpdateDeduplicationStatus(ctx, existingRR)
        return NewDuplicateResponseFromRR(signal.Fingerprint, existingRR), nil
    }

    // 3. Create resource (no race condition possible)
    return s.createRemediationRequestCRD(ctx, signal)
}
```

**RemediationOrchestrator Example**:
```go
// internal/controller/remediationorchestrator/reconciler.go
func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... load RemediationRequest ...

    targetResource := rr.Spec.TargetResource.String() // "Node/worker-1"

    // Routing decision protected by lock
    blocked, lockHandle, err := r.routingEngine.CheckBlockingConditions(ctx, rr, workflowID)
    if err != nil {
        return ctrl.Result{}, err
    }

    // If lock not acquired, requeue
    if lockHandle == nil {
        return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
    }

    // Ensure lock released
    defer lockHandle.Release(ctx)

    if blocked != nil {
        // ... handle blocking condition ...
        return ctrl.Result{RequeueAfter: blocked.RequeueAfter}, nil
    }

    // Create WorkflowExecution (protected by lock)
    weName, err := r.weCreator.Create(ctx, rr, ai)
    // ...
}
```

---

## Implementation Guidance

### When to Use This Pattern

**Use distributed locking when ALL of these apply**:
1. ✅ Service runs with multiple replicas (horizontal scaling)
2. ✅ Service performs check-then-create operations
3. ✅ Duplicate creation causes business problems (not just inefficiency)
4. ✅ K8s CRD name generation doesn't prevent duplicates (e.g., timestamp-based)

**Services Currently Using This Pattern**:
- ✅ **Gateway** (`pkg/gateway/processing/distributed_lock.go`)
  - Lock key: Signal fingerprint
  - Prevents duplicate RemediationRequest CRDs
  - BR-GATEWAY-190

- 🚧 **RemediationOrchestrator** (`pkg/remediationorchestrator/locking/distributed_lock.go`) *(planned)*
  - Lock key: Target resource (`Node/worker-1`)
  - Prevents duplicate WorkflowExecution CRDs
  - BR-ORCH-050

### How to Adapt for Your Service

**Step 1: Copy Reference Implementation**
- Copy Gateway's `distributed_lock.go` as starting point
- Place in service-specific package (not shared)

**Step 2: Customize Lock Key Generation**
```go
// Service-specific lock key generation
func generateLeaseName(key string) string {
    // Gateway: Hash and truncate fingerprint (can be long)
    // RO: Use target resource directly (already short)
    // Your service: Adapt as needed

    // Example for short keys:
    safeName := strings.ReplaceAll(key, "/", "-")
    return "service-lock-" + safeName

    // Example for long keys:
    hash := sha256.Sum256([]byte(key))
    return "service-lock-" + hex.EncodeToString(hash[:8])
}
```

**Step 3: Integrate with Service Metrics**
```go
// Service-specific metrics (NOT shared interface)
func (m *DistributedLockManager) AcquireLock(ctx context.Context, key string) (bool, error) {
    start := time.Now()
    acquired, err := m.acquireLockImpl(ctx, key)

    // Record metrics using YOUR service's metrics package
    if err != nil {
        yourservice.Metrics.LockAcquisitionFailures.Inc()
    } else if acquired {
        yourservice.Metrics.LockAcquisitionLatency.Observe(time.Since(start).Seconds())
    }

    return acquired, err
}
```

**Step 4: Add RBAC Permissions**
```yaml
# deployments/<service>/rbac.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: <service>-role
rules:
  # Existing permissions...

  # NEW: Lease resource permissions
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "create", "update", "delete"]
```

**Step 5: Configure Lease Duration**
```go
// Typical: 30 seconds (short enough to avoid long waits, long enough to complete operation)
lockManager := NewDistributedLockManager(
    k8sClient,
    namespace,
    podName,
    30*time.Second, // Lease duration
)
```

### Configuration Best Practices

**Lease Duration**:
- **Too short** (<10s): Increased K8s API load, potential lock loss during operation
- **Too long** (>60s): Longer wait times for lock contention
- **Recommended**: **30 seconds** (works for both Gateway and RO)

**Lock Key Naming**:
- **Prefix**: Include service name (e.g., `gw-lock-`, `ro-lock-`)
- **Max length**: 63 characters (K8s resource name limit)
- **Safe characters**: Lowercase alphanumeric + hyphens only

**Retry Strategy**:
- **Backoff**: 100ms initial (controller-native `RequeueAfter`)
- **Max retries**: Let controller retry indefinitely
- **Exponential backoff**: Not needed (lock will be released within 30s)

### Testing Guidance

**Unit Tests**:
- Test lock acquisition success (new lease)
- Test lock acquisition failure (held by another pod)
- Test lock acquisition idempotency (reentrant)
- Test expired lease takeover
- Test K8s API errors (permission denied, API down)

**Integration Tests**:
- Simulate multiple pods (multiple lock manager instances)
- Verify only one pod acquires lock at a time
- Verify lock release on operation completion
- Verify lease expiration on pod crash

**E2E Tests**:
- Deploy service with 3+ replicas
- Send concurrent requests targeting same resource
- Verify only one resource created (no duplicates)
- Verify all requests eventually succeed

**Reference Test Plans**:
- **Gateway**: [`docs/services/stateless/gateway-service/implementation/TEST_PLAN_DISTRIBUTED_LOCKING_V1.0.md`](../../services/stateless/gateway-service/implementation/TEST_PLAN_DISTRIBUTED_LOCKING_V1.0.md)
- **RO**: [`docs/services/crd-controllers/05-remediationorchestrator/implementation/TEST_PLAN_DISTRIBUTED_LOCKING_V1.0.md`](../../services/crd-controllers/05-remediationorchestrator/implementation/TEST_PLAN_DISTRIBUTED_LOCKING_V1.0.md)

---

## Alternatives Considered

### Alternative 1: Shared Library for Distributed Locking (REJECTED)

**Approach**: Move lock manager to `pkg/shared/locking/` for reuse across services.

**Pros**:
- ✅ Single implementation to maintain
- ✅ Consistent behavior across services
- ✅ Shared bug fixes and improvements

**Cons**:
- ❌ **Metrics Coupling**: Abstract interface needed (complexity)
- ❌ **Service-Specific Logic**: Lock key generation differs per service
- ❌ **Team Coordination**: Cross-team code review overhead
- ❌ **YAGNI Violation**: Only 2 services need this pattern
- ❌ **Premature Abstraction**: ~200 lines of code, not worth abstracting yet

**Decision**: **REJECTED** - Copy and adapt pattern for each service. Revisit if 3+ services adopt.

### Alternative 2: Admission Webhook Validation (CONSIDERED)

**Approach**: K8s ValidatingAdmissionWebhook rejects duplicate CRDs on CREATE.

**Pros**:
- ✅ Centralized validation logic
- ✅ K8s-native admission control
- ✅ No service code changes

**Cons**:
- ❌ **High Complexity**: Separate webhook deployment
- ❌ **Single Point of Failure**: Webhook down = all creates blocked
- ❌ **Higher Latency**: +20-50ms per request
- ❌ **Development Effort**: New service, certificates, RBAC

**Decision**: **REJECTED** - Too complex for the problem. Lease-based locking is simpler.

### Alternative 3: CRD Schema Unique Constraint (IDEAL BUT NOT SUPPORTED)

**Approach**: Add unique constraint to CRD schema (hypothetical).

```yaml
# +kubebuilder:validation:UniqueConstraint:field=signalFingerprint
type RemediationRequestSpec struct {
    SignalFingerprint string `json:"signalFingerprint"`
}
```

**Pros**:
- ✅ K8s enforces uniqueness automatically
- ✅ Zero latency overhead
- ✅ Simplest solution (declarative)

**Cons**:
- ❌ **NOT SUPPORTED**: Kubernetes doesn't support unique constraints in CRDs
- ❌ **Requires K8s Enhancement**: Would need KEP approval

**Decision**: **NOT VIABLE** - Not implementable with current Kubernetes.

### Alternative 4: Optimistic Creation with Background Cleanup (REJECTED)

**Approach**: Allow duplicates to be created, clean up in background CronJob.

**Pros**:
- ✅ Zero latency impact
- ✅ Simple service code

**Cons**:
- ❌ **Window of Vulnerability**: Duplicates exist for up to 1 minute
- ❌ **Potential Duplicate Execution**: If operation triggers before cleanup
- ❌ **Cleanup Complexity**: Robust deletion logic needed
- ❌ **Too Risky**: Non-idempotent operations could cause damage

**Decision**: **REJECTED** - Too risky for production. Correctness trumps latency.

---

## Consequences

### Positive

- ✅ **Eliminates Multi-Replica Race Conditions**: Only 1 pod can create resource at a time
- ✅ **Scales Safely**: Works correctly with 1 to 100+ replicas
- ✅ **K8s-Native**: No external dependencies (Redis, etcd, etc.)
- ✅ **Fault-Tolerant**: Lease expires if pod crashes (no deadlocks)
- ✅ **Audit Trail**: Lease shows which pod processed each request
- ✅ **Stateless**: No in-memory state, survives pod restarts
- ✅ **Independent Services**: Each service owns its implementation

### Negative

- ⚠️ **Latency Increase**: +10-20ms for lock acquisition per request
  - **Mitigation**: Still within service SLO targets (acceptable trade-off for correctness)
- ⚠️ **Lock Contention**: High-volume concurrent requests may queue
  - **Mitigation**: Lease duration tuned to 30s (short enough to avoid long waits)
- ⚠️ **Increased K8s API Load**: +2 API calls per operation (create/delete lease)
  - **Mitigation**: K8s API can handle this easily (lightweight Lease resources)
- ⚠️ **Code Duplication**: ~200 lines per service
  - **Mitigation**: Well-documented pattern, easy to copy and adapt

### Neutral

- 🔄 **Additional RBAC**: Lease resource permissions required (standard K8s pattern)
- 🔄 **Lease Garbage Collection**: Cleanup of expired leases
  - **Mitigation**: Built-in K8s garbage collection for Lease resources

---

## Observability

### Metrics to Track

**Per-Service Metrics** (each service implements its own):

```go
// Lock acquisition metrics
service_lock_acquisition_attempts_total      // Counter
service_lock_acquisition_successes_total     // Counter
service_lock_acquisition_failures_total{reason=""}  // Counter (reason: contention, api_error)
service_lock_acquisition_duration_seconds    // Histogram

// Lock hold metrics
service_lock_hold_duration_seconds           // Histogram
service_lock_releases_total                  // Counter
```

**Example - Gateway**:
- `gateway_lock_acquisition_attempts_total`
- `gateway_lock_acquisition_failures_total{reason="contention"}`

**Example - RemediationOrchestrator**:
- `ro_lock_acquisition_attempts_total`
- `ro_lock_acquisition_failures_total{reason="api_error"}`

### Monitoring & Alerting

**Lock Acquisition Success Rate**:
```promql
# Target: >99.9% success rate
sum(rate(service_lock_acquisition_successes_total[5m])) /
sum(rate(service_lock_acquisition_attempts_total[5m]))
```

**Lock Contention Rate**:
```promql
# Target: <5% contention rate
sum(rate(service_lock_acquisition_failures_total{reason="contention"}[5m])) /
sum(rate(service_lock_acquisition_attempts_total[5m]))
```

**Latency Impact**:
```promql
# Target: P95 lock acquisition latency <20ms
histogram_quantile(0.95, rate(service_lock_acquisition_duration_seconds_bucket[5m]))
```

---

## Review & Evolution

### When to Revisit This Decision

**Trigger Conditions**:
1. **If 3+ services adopt this pattern** → Consider shared library refactoring
2. **If service SLO violated** (e.g., P95 latency >SLO due to locking)
3. **If lock contention >10%** (high wait rate)
4. **If Kubernetes adds CRD unique constraints** (Alternative 3 becomes viable)
5. **If multi-region deployment requires cross-cluster locking**

### Future Enhancements

**Potential Improvements**:
1. **Adaptive Lease Duration**: Adjust based on request volume
2. **Lease Pooling**: Pre-acquire leases for common lock keys
3. **Circuit Breaker**: Temporarily disable locking if K8s API degraded
4. **Shared Library**: If 3+ services adopt, refactor to `pkg/shared/locking/`

**When to Create Shared Library**:
- ✅ 3+ services using the pattern
- ✅ Lock key generation logic stabilized (no service-specific variations)
- ✅ Metrics abstraction design agreed upon by all teams
- ✅ Cross-team coordination overhead justified by benefits

---

## Related Decisions

**Architecture Decisions**:
- **ADR-030**: Service Configuration Management (lease duration configuration)
- **ADR-015**: Alert-to-Signal Naming Migration (Gateway's signal fingerprint locking)
- **ADR-001**: CRD Microservices Architecture (multi-replica deployment patterns)

**Related Design Documents**:
- **DD-GATEWAY-011**: Status-Based Deduplication (complements distributed locking)
- **DD-015**: Timestamp-Based CRD Naming (why lock key != CRD name)
- **DD-RO-002**: Centralized Routing Responsibility (RO's resource lock checks)

**Cross-Team Coordination**:
- [CROSS_TEAM_DISTRIBUTED_LOCKING_PATTERN_DEC_30_2025.md](../../shared/CROSS_TEAM_DISTRIBUTED_LOCKING_PATTERN_DEC_30_2025.md)

---

## References

### Kubernetes Documentation
- [Lease Resource API](https://kubernetes.io/docs/reference/kubernetes-api/cluster-resources/lease-v1/)
- [Leader Election Pattern](https://kubernetes.io/blog/2016/01/simple-leader-election-with-kubernetes/)
- [Controller Runtime Client](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/client)

### Implementation References
- **Gateway**: [`pkg/gateway/processing/distributed_lock.go`](../../pkg/gateway/processing/distributed_lock.go)
- **Gateway Implementation Plan**: [`docs/services/stateless/gateway-service/implementation/IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md`](../../services/stateless/gateway-service/implementation/IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md)
- **RO Implementation Plan**: [`docs/services/crd-controllers/05-remediationorchestrator/implementation/IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md`](../../services/crd-controllers/05-remediationorchestrator/implementation/IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md)

### Test Plan References
- **Gateway Test Plan**: [`docs/services/stateless/gateway-service/implementation/TEST_PLAN_DISTRIBUTED_LOCKING_V1.0.md`](../../services/stateless/gateway-service/implementation/TEST_PLAN_DISTRIBUTED_LOCKING_V1.0.md)
- **RO Test Plan**: [`docs/services/crd-controllers/05-remediationorchestrator/implementation/TEST_PLAN_DISTRIBUTED_LOCKING_V1.0.md`](../../services/crd-controllers/05-remediationorchestrator/implementation/TEST_PLAN_DISTRIBUTED_LOCKING_V1.0.md)

### Handoff Documents
- [RO Race Condition Analysis](../../handoff/RO_RACE_CONDITION_ANALYSIS_DEC_30_2025.md)
- [Gateway Race Condition Gap Analysis](../../handoff/GW_RACE_CONDITION_GAP_ANALYSIS_DEC_30_2025.md)
- [DD to ADR Conversion](../../handoff/DD_TO_ADR_CONVERSION_DISTRIBUTED_LOCKING_DEC_30_2025.md)

---

**Status**: ✅ **APPROVED** - Pattern documented, services implementing independently

**Last Updated**: December 30, 2025
**Next Review**: After 3+ services adopt pattern (evaluate shared library)









