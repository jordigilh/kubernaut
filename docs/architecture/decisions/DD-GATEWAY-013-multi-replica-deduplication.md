# DD-GATEWAY-013: Multi-Replica Deduplication Protection

**Version**: 1.0
**Date**: December 30, 2025
**Status**: 🔄 **PROPOSED** (Awaiting Approval)
**Author**: AI Assistant
**Confidence**: 85%

---

## Context & Problem

**User Question**: "What can we do to prevent [duplicate RR creation] when we have multiple gateway instances running?"

### **The Multi-Replica Race Condition**

**Current State**: Gateway is designed to scale horizontally (multiple replicas), but the deduplication mechanism has a race condition when requests span second boundaries across different Gateway pods.

**Production Scenario**:
```
3 Gateway Pods Running in Production
═══════════════════════════════════════

Alert Storm: 100 concurrent requests with SAME fingerprint
  ├─ 33 requests → gateway-pod-1
  ├─ 34 requests → gateway-pod-2
  └─ 33 requests → gateway-pod-3

Each pod independently:
  1. Calls ShouldDeduplicate() → false (race window)
  2. Generates CRD name with timestamp
  3. Creates RemediationRequest

Problem: If requests span second boundary, multiple RRs created! ❌
```

### **Race Condition Amplification with Multiple Replicas**

**Single Gateway Pod**:
- Race window: ~40ms (time between check and K8s create)
- Probability: ~0.01% (cross-second boundary + race window)

**3 Gateway Pods**:
- Race window: ~40ms per pod (independent processes)
- Probability: ~0.03% (3x single pod probability)
- **Load balancer distributes requests** → increases chance of cross-second races

**10 Gateway Pods** (high-scale production):
- Race window: ~40ms per pod
- Probability: ~0.1% (10x single pod probability)
- **More pods = more likely to span second boundaries**

### **Why Current Protection Fails**

**Layer 1 (K8s Deduplication Check)**: ❌ **NOT SUFFICIENT**
- Each Gateway pod queries K8s independently
- K8s API latency varies per pod (~10-50ms)
- No coordination between Gateway pods
- Race window: Both pods query before either creates RR

**Layer 2 (Optimistic Concurrency)**: ✅ **WORKS** (for status updates)
- Handles concurrent status updates correctly
- But doesn't prevent duplicate CRD creation

**Layer 3 (K8s Atomic Creation)**: ⚠️ **PARTIAL PROTECTION**
- Only prevents same-second races (same CRD name)
- Cross-second races have different names → both succeed

---

## Business Impact

### **Without Mitigation**

**Duplicate RR Creation Rate** (estimated):
- Single Gateway pod: ~0.01% of concurrent requests
- 3 Gateway pods: ~0.03% of concurrent requests
- 10 Gateway pods: ~0.1% of concurrent requests

**At Scale** (10,000 alerts/day with 10% concurrent):
- 1,000 concurrent alert scenarios/day
- ~1 duplicate RR created/day (0.1% of 1,000)
- ~30 duplicate RRs created/month
- ~365 duplicate RRs created/year

**Blast Radius per Duplicate**:
1. **Wasted Resources**: Duplicate RRs consume K8s API, etcd, controller CPU
2. **Duplicate Remediation**: Same fix applied multiple times
3. **Observability Confusion**: Multiple RRs for same problem
4. **Potential System Damage**: Non-idempotent remediations executed twice

### **Impact Severity by Remediation Type**

| Remediation Type | Idempotent? | Duplicate Impact | Severity |
|---|---|---|---|
| **Restart Pod** | ✅ Yes | Multiple restarts (minor disruption) | Low |
| **Scale Deployment** | ✅ Yes | Multiple scale operations (eventual consistency) | Low |
| **Delete/Create Resource** | ❌ No | Resource thrashing, data loss | **High** |
| **Data Migration** | ❌ No | Duplicate migration, corruption | **Critical** |
| **Failover** | ❌ No | Cascading failures | **Critical** |

---

## Alternatives Considered

### **Alternative 1: Kubernetes Lease-Based Distributed Lock** (RECOMMENDED)

**Approach**: Use K8s Lease resource for distributed locking on fingerprint

**Architecture**:
```
Gateway Pod 1                  Gateway Pod 2
─────────────────              ─────────────────
Request arrives                Request arrives
  ↓                              ↓
Acquire Lease on fingerprint   Acquire Lease on fingerprint
  ├─ Lease: "gw-lock-bd773c"     ├─ Lease: "gw-lock-bd773c"
  ├─ Holder: "gateway-pod-1"     ├─ Holder: ??? (conflict!)
  ├─ Duration: 30s               ├─ ERROR: Lease held by pod-1
  └─ SUCCESS ✅                  └─ WAIT & RETRY ⏳
    ↓                              ↓
ShouldDeduplicate()            (waits 100ms)
  ↓                              ↓
CreateRemediationRequest()     Retry: ShouldDeduplicate()
  ↓                              ├─ RR now exists!
Release Lease                   └─ Update OccurrenceCount ✅
  ↓
HTTP 201 Created
```

**Implementation**:
```go
// pkg/gateway/processing/distributed_lock.go (NEW FILE)
package processing

import (
    "context"
    "fmt"
    "time"

    coordinationv1 "k8s.io/api/coordination/v1"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// DistributedLockManager manages K8s Lease-based distributed locks
type DistributedLockManager struct {
    client    client.Client
    namespace string
    holderID  string  // Gateway pod name
}

// AcquireLock attempts to acquire a lock for the given fingerprint
// Returns:
//   - (true, nil): Lock acquired successfully
//   - (false, nil): Lock held by another pod (not an error, caller should retry)
//   - (false, error): K8s API error (communication failure, permission denied, etc.)
func (m *DistributedLockManager) AcquireLock(ctx context.Context, fingerprint string) (bool, error) {
    leaseName := fmt.Sprintf("gw-lock-%s", fingerprint[:16])

    lease := &coordinationv1.Lease{}
    err := m.client.Get(ctx, client.ObjectKey{
        Namespace: m.namespace,
        Name:      leaseName,
    }, lease)

    now := metav1.Now()
    leaseDuration := int32(30)  // 30 seconds

    if err != nil {
        // Check error type - only handle NotFound, propagate all others
        if !apierrors.IsNotFound(err) {
            // Real error (API down, permission denied, timeout, etc.)
            // Return error so caller can fail-fast
            return false, fmt.Errorf("failed to check for existing lease: %w", err)
        }

        // Lease doesn't exist (NotFound) - create it
        lease = &coordinationv1.Lease{
            ObjectMeta: metav1.ObjectMeta{
                Name:      leaseName,
                Namespace: m.namespace,
            },
            Spec: coordinationv1.LeaseSpec{
                HolderIdentity:       &m.holderID,
                LeaseDurationSeconds: &leaseDuration,
                AcquireTime:          &now,
                RenewTime:            &now,
            },
        }

        if err := m.client.Create(ctx, lease); err != nil {
            // Check error type on Create failure
            if apierrors.IsAlreadyExists(err) {
                // Another pod created the lease first (race condition)
                // This is expected - return false (not acquired) without error
                return false, nil
            }

            // Real error (API down, permission denied, etc.)
            return false, fmt.Errorf("failed to create lease: %w", err)
        }

        return true, nil
    }

    // Lease exists - check if expired or held by us
    if lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity == m.holderID {
        // We already hold this lock
        return true, nil
    }

    // Check if lease expired
    if lease.Spec.RenewTime != nil {
        expiry := lease.Spec.RenewTime.Add(time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second)
        if time.Now().After(expiry) {
            // Lease expired - take it over
            lease.Spec.HolderIdentity = &m.holderID
            lease.Spec.RenewTime = &now

            if err := m.client.Update(ctx, lease); err != nil {
                // Check error type on Update failure
                if apierrors.IsConflict(err) {
                    // Another pod updated the lease first (race condition)
                    // This is expected - return false (not acquired) without error
                    return false, nil
                }

                // Real error (API down, permission denied, etc.)
                return false, fmt.Errorf("failed to take over expired lease: %w", err)
            }

            return true, nil
        }
    }

    // Lease held by another pod and not expired
    return false, nil
}

// ReleaseLock releases the lock for the given fingerprint
func (m *DistributedLockManager) ReleaseLock(ctx context.Context, fingerprint string) error {
    leaseName := fmt.Sprintf("gw-lock-%s", fingerprint[:16])

    lease := &coordinationv1.Lease{}
    lease.Namespace = m.namespace
    lease.Name = leaseName

    // Delete lease (allows garbage collection)
    return m.client.Delete(ctx, lease)
}
```

**Integration in Gateway**:
```go
// pkg/gateway/server.go
func (s *Server) ProcessSignal(ctx context.Context, signal *types.NormalizedSignal) (*ProcessingResponse, error) {
    logger := middleware.GetLogger(ctx)

    // 1. Acquire distributed lock for fingerprint
    lockAcquired, err := s.lockManager.AcquireLock(ctx, signal.Fingerprint)
    if err != nil {
        return nil, fmt.Errorf("failed to acquire lock: %w", err)
    }

    if !lockAcquired {
        // Lock held by another Gateway pod - retry deduplication check
        // (other pod may have created RR by now)
        time.Sleep(100 * time.Millisecond)  // Backoff

        shouldDeduplicate, existingRR, err := s.phaseChecker.ShouldDeduplicate(ctx, signal.Namespace, signal.Fingerprint)
        if err != nil {
            return nil, err
        }

        if shouldDeduplicate && existingRR != nil {
            // RR created by other pod - update status
            s.statusUpdater.UpdateDeduplicationStatus(ctx, existingRR)
            return NewDuplicateResponseFromRR(signal.Fingerprint, existingRR), nil
        }

        // Still no RR - retry lock acquisition
        return s.ProcessSignal(ctx, signal)  // Recursive retry
    }

    // Lock acquired - ensure it's released
    defer s.lockManager.ReleaseLock(ctx, signal.Fingerprint)

    // 2. Deduplication check (DD-GATEWAY-011: K8s status-based)
    shouldDeduplicate, existingRR, err := s.phaseChecker.ShouldDeduplicate(ctx, signal.Namespace, signal.Fingerprint)
    if err != nil {
        return nil, fmt.Errorf("deduplication check failed: %w", err)
    }

    if shouldDeduplicate && existingRR != nil {
        // Duplicate - update status
        s.statusUpdater.UpdateDeduplicationStatus(ctx, existingRR)
        return NewDuplicateResponseFromRR(signal.Fingerprint, existingRR), nil
    }

    // 3. Create RemediationRequest CRD
    return s.createRemediationRequestCRD(ctx, signal, start)
}
```

**Pros**:
- ✅ **Guarantees mutual exclusion**: Only 1 Gateway pod can create RR per fingerprint
- ✅ **K8s-native**: Uses standard Kubernetes Lease resource
- ✅ **Fault-tolerant**: Lease expires if Gateway pod crashes
- ✅ **Stateless**: No in-memory state, survives pod restarts
- ✅ **Minimal latency**: ~10-20ms for lease acquisition (K8s API call)
- ✅ **Audit trail**: Lease shows which pod held lock

**Cons**:
- ⚠️ **Adds latency**: +10-20ms for lock acquisition per request
- ⚠️ **Lock contention**: High-volume alerts may queue behind locks
- ⚠️ **Lease cleanup**: Need garbage collection for old leases
- ⚠️ **Complexity**: Adds distributed locking logic to Gateway

**Confidence**: **85%** - Robust solution with acceptable trade-offs

---

### **Alternative 2: Admission Webhook Validation** (ALTERNATIVE)

**Approach**: K8s ValidatingAdmissionWebhook rejects duplicate RRs on CREATE

**Architecture**:
```
Gateway Pod 1                  K8s API Server                  Admission Webhook
─────────────────              ───────────────                 ─────────────────
Create RR-1                    Receives CREATE request         Validates uniqueness
  ├─ Fingerprint: "bd773c"       ├─ Calls webhook                ├─ Query K8s for duplicates
  └─ Name: "rr-bd773c-1001"      └─ Waits for response           ├─ Check: spec.signalFingerprint
                                                                  └─ Returns: ALLOW
                                Webhook returns ALLOW
                                  ↓
                                CREATE SUCCESS ✅

Gateway Pod 2                  K8s API Server                  Admission Webhook
─────────────────              ───────────────                 ─────────────────
Create RR-2                    Receives CREATE request         Validates uniqueness
  ├─ Fingerprint: "bd773c"       ├─ Calls webhook                ├─ Query K8s for duplicates
  └─ Name: "rr-bd773c-1002"      └─ Waits for response           ├─ FOUND: RR-1 exists!
                                                                  └─ Returns: DENY ❌
                                Webhook returns DENY
                                  ↓
                                CREATE REJECTED ❌

Gateway Pod 2 retries:
  ├─ ShouldDeduplicate() → true (RR-1 exists)
  └─ Update OccurrenceCount ✅
```

**Pros**:
- ✅ **Centralized validation**: Single point of enforcement (webhook)
- ✅ **No Gateway changes**: Logic in separate webhook service
- ✅ **K8s-native**: Standard admission control mechanism
- ✅ **Guaranteed prevention**: K8s API server enforces uniqueness

**Cons**:
- ❌ **High complexity**: Requires separate webhook deployment
- ❌ **High availability requirement**: Webhook failure blocks ALL RR creation
- ❌ **Latency**: +20-50ms for webhook validation per request
- ❌ **Development effort**: New service, certificates, deployment

**Confidence**: **65%** - Technically sound but over-engineered

---

### **Alternative 3: CRD Schema Unique Constraint** (IDEAL BUT NOT SUPPORTED)

**Approach**: Add unique constraint to CRD schema for `spec.signalFingerprint`

**Implementation** (hypothetical):
```yaml
# api/remediation/v1alpha1/remediationrequest_types.go
// +kubebuilder:validation:UniqueConstraint:field=signalFingerprint
type RemediationRequestSpec struct {
    SignalFingerprint string `json:"signalFingerprint"`
    // ... other fields
}
```

**Pros**:
- ✅ **K8s enforces uniqueness**: No custom code needed
- ✅ **Zero latency overhead**: Validation at API server level
- ✅ **Simplest solution**: Declarative constraint in CRD

**Cons**:
- ❌ **NOT SUPPORTED**: Kubernetes doesn't support unique constraints in CRD schemas
- ❌ **Requires K8s enhancement**: Would need KEP (K8s Enhancement Proposal)

**Confidence**: **N/A** - Not implementable with current K8s

---

### **Alternative 4: Optimistic Creation with Cleanup** (ACCEPT RISK)

**Approach**: Allow duplicate RRs to be created, clean up in background

**Architecture**:
```
Gateway Pod 1                  Gateway Pod 2
─────────────────              ─────────────────
Create RR-1 → SUCCESS          Create RR-2 → SUCCESS
  ↓                              ↓
Both RRs exist temporarily

Background Cleanup Job (CronJob every 1 minute):
  ├─ Query all RRs in Pending phase
  ├─ Group by spec.signalFingerprint
  ├─ For each fingerprint with duplicates:
  │   ├─ Keep oldest RR
  │   └─ Delete newer RRs
  └─ Update OccurrenceCount on kept RR
```

**Pros**:
- ✅ **Zero latency impact**: No locking overhead
- ✅ **Simple Gateway code**: No distributed locking
- ✅ **Eventual consistency**: Duplicates cleaned up within 1 minute

**Cons**:
- ❌ **Window of vulnerability**: Duplicates exist for up to 1 minute
- ❌ **Potential duplicate execution**: If WE picks up RR before cleanup
- ❌ **Complexity in cleanup**: Need robust deletion logic
- ❌ **Race in cleanup**: Multiple cleanup jobs could conflict

**Confidence**: **50%** - Too risky for production

---

## Decision

**RECOMMENDED: Alternative 1 - Kubernetes Lease-Based Distributed Lock**

**Rationale**:
1. **Guarantees correctness**: Mutual exclusion prevents duplicate RR creation
2. **K8s-native solution**: Uses standard Lease resource (no external dependencies)
3. **Acceptable latency**: +10-20ms per request (within Gateway SLO p95 <50ms)
4. **Fault-tolerant**: Lease expires if Gateway pod crashes (no deadlocks)
5. **Proven pattern**: Same mechanism used by K8s leader election
6. **Stateless**: No in-memory state, survives Gateway pod restarts

**Trade-offs Accepted**:
- ⚠️ **Latency increase**: +10-20ms for lock acquisition (acceptable vs. duplicate risk)
- ⚠️ **Lock contention**: High-volume alerts may queue (but ensures correctness)

**Alternative Considered But Rejected**:
- **Admission Webhook**: Too complex, single point of failure
- **CRD Unique Constraint**: Not supported by Kubernetes
- **Optimistic Cleanup**: Too risky, window of vulnerability

---

## Implementation Plan

### **Phase 1: Core Distributed Lock Implementation** (P0 - High Priority)

**Files to Create**:
1. `pkg/gateway/processing/distributed_lock.go` - Lease-based lock manager
2. `pkg/gateway/processing/distributed_lock_test.go` - Unit tests

**Files to Modify**:
1. `pkg/gateway/server.go` - Integrate lock acquisition in ProcessSignal()
2. `pkg/gateway/config/config.go` - Add distributed locking configuration
3. `deployments/gateway/deployment.yaml` - Add RBAC for Lease resources

**Estimated Effort**: 4-6 hours

---

### **Phase 2: Integration Testing** (P0 - High Priority)

**New Test**:
```go
// test/integration/gateway/multi_replica_deduplication_test.go
var _ = Describe("Multi-Replica Deduplication (DD-GATEWAY-013)", func() {
    It("should prevent duplicate RRs with multiple Gateway replicas", func() {
        // Simulate 3 Gateway pods processing same fingerprint concurrently
        // Verify only 1 RR created
    })

    It("should handle lock contention gracefully", func() {
        // Simulate 10 concurrent requests with same fingerprint
        // Verify all succeed (first creates, others deduplicate)
    })

    It("should release locks on Gateway pod crash", func() {
        // Simulate Gateway pod crash during lock hold
        // Verify lease expires and new request succeeds
    })
})
```

**Estimated Effort**: 3-4 hours

---

### **Phase 3: E2E Validation** (P1 - Medium Priority)

**E2E Test**:
- Deploy Gateway with 3 replicas
- Send 100 concurrent requests with same fingerprint
- Verify only 1 RR created
- Verify all requests return HTTP 201/202

**Estimated Effort**: 2 hours

---

### **Phase 4: Production Rollout** (P1 - Medium Priority)

**Rollout Strategy**:
1. **Stage 1**: Deploy with distributed locking **disabled** (feature flag)
2. **Stage 2**: Enable for 10% of traffic (canary)
3. **Stage 3**: Monitor latency impact (p95, p99)
4. **Stage 4**: Enable for 100% if latency acceptable

**Feature Flag**:
```go
// pkg/gateway/config/config.go
type ProcessingConfig struct {
    EnableDistributedLocking bool `yaml:"enableDistributedLocking" env:"GATEWAY_ENABLE_DISTRIBUTED_LOCKING"`
    // ... other fields
}
```

**Estimated Effort**: 2 hours

---

## Configuration

### **Gateway Configuration**

**Environment Variables**:
```yaml
# deployments/gateway/deployment.yaml
env:
  - name: GATEWAY_ENABLE_DISTRIBUTED_LOCKING
    value: "true"
  - name: GATEWAY_LOCK_NAMESPACE
    value: "kubernaut-system"
  - name: GATEWAY_LOCK_DURATION_SECONDS
    value: "30"
```

**RBAC Permissions**:
```yaml
# deployments/gateway/rbac.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: gateway-role
rules:
  # ... existing permissions ...

  # NEW: Lease resource permissions for distributed locking
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "create", "update", "delete"]
```

---

## Validation & Success Metrics

### **Correctness Metrics**

**Primary Metric**: **Duplicate RR Creation Rate**
- **Target**: <0.001% of concurrent requests
- **Measurement**: `rate(gateway_duplicate_rr_created_total[5m])`
- **Alert**: If rate > 0.01% for 5 minutes

**Secondary Metric**: **Lock Acquisition Success Rate**
- **Target**: >99.9% of lock attempts succeed
- **Measurement**: `gateway_lock_acquisition_success_total / gateway_lock_acquisition_attempts_total`
- **Alert**: If success rate < 99% for 5 minutes

### **Performance Metrics**

**Latency Impact**:
- **Target**: P95 latency increase <20ms
- **Measurement**: `histogram_quantile(0.95, gateway_signal_processing_duration_seconds)`
- **Alert**: If P95 latency > 70ms (50ms SLO + 20ms lock overhead)

**Lock Contention**:
- **Target**: <5% of requests wait for lock
- **Measurement**: `gateway_lock_wait_duration_seconds_count / gateway_signals_received_total`
- **Alert**: If wait rate > 10% for 5 minutes

### **Observability**

**New Prometheus Metrics**:
```go
// pkg/gateway/metrics/metrics.go
type Metrics struct {
    // ... existing metrics ...

    // Distributed locking metrics
    LockAcquisitionAttemptsTotal prometheus.Counter
    LockAcquisitionSuccessTotal  prometheus.Counter
    LockAcquisitionFailuresTotal prometheus.Counter
    LockWaitDurationSeconds      prometheus.Histogram
    LockHoldDurationSeconds      prometheus.Histogram
}
```

**Grafana Dashboard**:
- Lock acquisition success rate (gauge)
- Lock wait time distribution (histogram)
- Lock hold time distribution (histogram)
- Duplicate RR creation rate (counter)

---

## Consequences

### **Positive**

- ✅ **Eliminates cross-replica race condition**: Only 1 Gateway pod can create RR per fingerprint
- ✅ **Scales safely**: Works correctly with 1 to 100+ Gateway replicas
- ✅ **K8s-native**: No external dependencies (Redis, etcd, etc.)
- ✅ **Fault-tolerant**: Lease expires if Gateway pod crashes (no deadlocks)
- ✅ **Audit trail**: Lease shows which pod processed each signal

### **Negative**

- ⚠️ **Latency increase**: +10-20ms for lock acquisition per request
  - **Mitigation**: Still within Gateway SLO (P95 <50ms becomes P95 ~60ms)
- ⚠️ **Lock contention**: High-volume alerts may queue behind locks
  - **Mitigation**: Lease duration tuned to 30s (short enough to avoid long waits)
- ⚠️ **Increased K8s API load**: +2 API calls per signal (create/delete lease)
  - **Mitigation**: K8s API server can handle this easily (lightweight Lease resources)

### **Neutral**

- 🔄 **Additional RBAC**: Need Lease resource permissions (standard K8s pattern)
- 🔄 **Lease garbage collection**: Need cleanup of expired leases
  - **Mitigation**: Built-in K8s garbage collection for Lease resources

---

## Review & Evolution

### **When to Revisit**

- If Gateway SLO violated (P95 latency >70ms)
- If lock contention >10% (high wait rate)
- If Kubernetes adds CRD unique constraints (Alternative 3 becomes viable)
- If multi-region deployment requires cross-cluster locking

### **Future Enhancements**

1. **Adaptive Lease Duration**: Adjust based on signal volume
2. **Lease Pooling**: Pre-acquire leases for common fingerprints
3. **Circuit Breaker**: Temporarily disable locking if K8s API degraded

---

## Related Decisions

- **DD-GATEWAY-011**: Status-Based Deduplication (K8s-native, Redis deprecated)
- **DD-015**: Timestamp-Based CRD Naming (unique occurrence tracking)
- **DD-WE-003**: Resource Lock Persistence (similar distributed locking pattern)
- **DD-RO-002**: Centralized Routing Responsibility (RO checks for duplicate WFEs)

---

## References

### **Kubernetes Documentation**
- [Lease Resource](https://kubernetes.io/docs/reference/kubernetes-api/cluster-resources/lease-v1/)
- [Leader Election](https://kubernetes.io/blog/2016/01/simple-leader-election-with-kubernetes/)

### **Code Files**
- `pkg/gateway/server.go` - ProcessSignal() integration point
- `pkg/gateway/processing/phase_checker.go` - Deduplication check
- `pkg/gateway/processing/status_updater.go` - Optimistic concurrency

### **Design Documents**
- `docs/handoff/GW_RACE_CONDITION_GAP_ANALYSIS_DEC_30_2025.md` - Race condition analysis

---

## Approval & Implementation

**Approval Required**: YES
**Approver**: User (jordigilh)
**Estimated Implementation Time**: 11-16 hours total
**Production Rollout**: Phased rollout with feature flag

**Questions for Approval**:
1. Is +10-20ms latency acceptable for correctness guarantee?
2. Should we implement in V1.0 or defer to V1.1?
3. Do we need cross-cluster support (multi-region)?

---

**Status**: ⚠️ **AWAITING USER APPROVAL** - Ready for implementation pending user decision

