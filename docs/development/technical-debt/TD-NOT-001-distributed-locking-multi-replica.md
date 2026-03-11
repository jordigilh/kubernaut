# TD-NOT-001: Distributed Locking for Multi-Replica Notification Controller

**Created**: January 11, 2026
**Priority**: P2 (DD-NOT-008 handles single-cluster multi-replica)
**Effort**: 2-3 days
**Owner**: Notification Team
**Status**: Documented - Not Yet Required
**Aligns With**: [TD-GW-001 (Gateway)](../handoff/GW_DISTRIBUTED_LOCKING_READY_FOR_IMPLEMENTATION_DEC_30_2025.md), TD-RO-001 (Remediation Orchestrator)

---

## 🎯 PROBLEM STATEMENT

### Current State: DD-NOT-008 (Production-Ready for Single Cluster)

**Implemented**: `singleflight` + in-flight tracking + successful delivery tracking
**Status**: ✅ Handles multi-replica deployments within single Kubernetes cluster
**Coverage**: Prevents duplicate deliveries across 2+ controller pods
**Limitation**: In-memory only (no cross-cluster coordination)

**What DD-NOT-008 Solves**:
```
Pod A (Reconciliation 1) → In-flight tracking → Prevents duplicate
Pod B (Reconciliation 2) → Checks in-flight   → Blocks duplicate delivery ✅
```

**When DD-NOT-008 is Sufficient**:
- ✅ Single Kubernetes cluster
- ✅ 2-10 replicas of NotificationRequest controller
- ✅ Leader election enabled (default)
- ✅ No external rate limits (Slack, email APIs)

---

### Gap: Multi-Cluster / Shared External Resources

**When Distributed Locking Becomes Necessary**:
1. **Multi-Cluster Deployments**: Federation scenarios with shared external systems
2. **Shared Rate-Limited APIs**: Slack workspace rate limits (1 msg/sec/workspace)
3. **Leader Election Disabled**: High availability scenarios requiring active-active
4. **Cross-Service Coordination**: NotificationRequest triggered by multiple sources

**Example Scenario**:
```
Cluster A (Pod 1) → Slack message #1 ──┐
                                         ├─→ Slack API (rate limit: 1 msg/sec)
Cluster B (Pod 2) → Slack message #2 ──┘    ❌ Rate limit exceeded
```

**Impact Without Distributed Locking**:
- Duplicate Slack messages (user confusion)
- Rate limit violations (Slack API throttling)
- Duplicate email notifications (compliance issues)
- Audit trail inconsistencies (multiple delivery attempts recorded)

---

## ✅ PROPOSED SOLUTION: Kubernetes Lease-Based Distributed Lock

### Architecture (Aligns with Gateway/RO Pattern)

```go
// pkg/notification/processing/distributed_lock.go
type DistributedLock struct {
    client          client.Client
    leaseDuration   time.Duration
    namespace       string // From POD_NAMESPACE env var
    podName         string // From POD_NAME env var
    logger          logr.Logger
}

// Acquire lock before delivery to external channels
func (d *DistributedLock) AcquireLock(ctx context.Context, resourceKey string) error {
    leaseName := fmt.Sprintf("notification-delivery-%s", resourceKey)

    // Try to get existing lease
    lease := &coordinationv1.Lease{}
    err := d.client.Get(ctx, client.ObjectKey{
        Name:      leaseName,
        Namespace: d.namespace,
    }, lease)

    if err != nil {
        if !apierrors.IsNotFound(err) {
            // Propagate ALL errors except NotFound
            // DD-GATEWAY-013: User feedback applied
            return fmt.Errorf("failed to get lease: %w", err)
        }

        // Lease doesn't exist, create it
        return d.createLease(ctx, leaseName)
    }

    // Lease exists, check if expired
    if d.isLeaseExpired(lease) {
        return d.renewLease(ctx, lease)
    }

    // Lease held by another pod
    return fmt.Errorf("lock held by %s", *lease.Spec.HolderIdentity)
}
```

### Integration with DD-NOT-008

**Layered Approach**:
```
1. DD-NOT-008 (In-Memory)   → Prevents intra-pod duplicates
2. Distributed Lock (Lease)  → Prevents cross-cluster duplicates
```

**Modified Orchestrator** (`pkg/notification/delivery/orchestrator.go`):
```go
func (o *Orchestrator) DeliverToChannel(ctx context.Context, notification *v1alpha1.NotificationRequest, channel v1alpha1.Channel) error {
    key := fmt.Sprintf("%s:%s", notification.UID, channel)

    // Layer 1: DD-NOT-008 singleflight (in-memory deduplication)
    result, err, shared := o.deliveryGroup.Do(key, func() (interface{}, error) {
        // Layer 2: Distributed lock (cross-cluster deduplication)
        if o.shouldUseDistributedLock(channel) {
            lockKey := fmt.Sprintf("%s-%s", notification.Name, channel)
            if err := o.distributedLock.AcquireLock(ctx, lockKey); err != nil {
                o.logger.Info("TD-NOT-001: Lock acquisition failed, skipping duplicate delivery")
                return nil, nil // Not an error - another pod is delivering
            }
            defer o.distributedLock.ReleaseLock(ctx, lockKey)
        }

        return nil, o.doDelivery(ctx, notification, channel)
    })

    if shared {
        o.logger.Info("DD-NOT-008: Concurrent delivery deduplicated (in-memory)")
    }

    return err
}

func (o *Orchestrator) shouldUseDistributedLock(channel v1alpha1.Channel) bool {
    // Only for external channels with shared rate limits
    return channel == v1alpha1.ChannelSlack ||
           channel == v1alpha1.ChannelEmail ||
           channel == v1alpha1.ChannelWebhook
}
```

---

## 📋 IMPLEMENTATION PLAN

### Phase 1: Distributed Lock Manager (8 hours)

**Deliverable**: `pkg/notification/processing/distributed_lock.go`

**API Surface**:
```go
type DistributedLock interface {
    AcquireLock(ctx context.Context, resourceKey string) error
    ReleaseLock(ctx context.Context, resourceKey string) error
    IsLockHeld(ctx context.Context, resourceKey string) (bool, string, error)
}

// Configuration (hardcoded for simplicity)
const (
    LeaseDuration   = 30 * time.Second
    RenewInterval   = 10 * time.Second
    LeaseNamePrefix = "notification-delivery-"
)
```

**Key Methods**:
1. `AcquireLock()`: Create or renew lease
2. `ReleaseLock()`: Delete lease
3. `IsLockHeld()`: Check lease status (for debugging)
4. `isLeaseExpired()`: Check expiration (private)
5. `createLease()`: Create new lease (private)
6. `renewLease()`: Update existing lease (private)

---

### Phase 2: Orchestrator Integration (4 hours)

**File**: `pkg/notification/delivery/orchestrator.go`

**Changes**:
```go
type Orchestrator struct {
    // ... existing fields ...

    // TD-NOT-001: Distributed locking (optional)
    distributedLock *DistributedLock // nil if not configured
    useDistributedLocking bool
}

// Modified constructor
func NewOrchestrator(client client.Client, logger logr.Logger, useDistributedLocking bool) *Orchestrator {
    o := &Orchestrator{
        // ... existing initialization ...
        useDistributedLocking: useDistributedLocking,
    }

    if useDistributedLocking {
        o.distributedLock = NewDistributedLock(client, logger)
    }

    return o
}
```

---

### Phase 3: RBAC and Deployment (2 hours)

**File**: `deploy/notification/rbac.yaml`

**Required Permissions**:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: notification-controller-lease-manager
rules:
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "create", "update", "delete"]
```

**File**: `deploy/notification/deployment.yaml`

**Environment Variables**:
```yaml
env:
- name: POD_NAMESPACE
  valueFrom:
    fieldRef:
      fieldPath: metadata.namespace
- name: POD_NAME
  valueFrom:
    fieldRef:
      fieldPath: metadata.name
- name: USE_DISTRIBUTED_LOCKING
  value: "false" # Default: DD-NOT-008 only
```

---

### Phase 4: Metrics and Observability (2 hours)

**File**: `pkg/notification/metrics/metrics.go`

**New Metrics**:
```go
var (
    // TD-NOT-001: Distributed lock metrics
    notificationLockAcquisitionFailures = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "notification_lock_acquisition_failures_total",
            Help: "Total number of distributed lock acquisition failures",
        },
        []string{"channel", "reason"},
    )

    notificationLockAcquisitionDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "notification_lock_acquisition_duration_seconds",
            Help: "Duration of distributed lock acquisition",
            Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
        },
        []string{"channel"},
    )
)
```

---

## 🧪 TEST PLAN

### Unit Tests (4 hours)

**File**: `pkg/notification/processing/distributed_lock_test.go`

**Coverage**: 90%+

**Scenarios**:
1. ✅ Lock acquisition success (lease doesn't exist)
2. ✅ Lock acquisition failure (lease held by another pod)
3. ✅ Lock renewal success (lease expired)
4. ✅ Lock release success
5. ✅ Error handling (K8s API errors)
6. ✅ Lease expiration logic
7. ✅ Race condition prevention (concurrent acquisitions)

---

### Integration Tests (6 hours)

**File**: `test/integration/notification/distributed_locking_test.go`

**Setup**: 3 controller replicas in Kind cluster

**Scenarios**:
1. ✅ Multi-replica deduplication (3 pods, 1 delivery)
2. ✅ Lock contention (10 concurrent signals, 1 winner)
3. ✅ Lease expiration (pod crash, lease expires, another pod acquires)
4. ✅ RBAC validation (missing permissions → error)
5. ✅ Cross-namespace isolation (locks in different namespaces don't conflict)

**Validation**:
```go
It("should prevent duplicate Slack deliveries across replicas", func() {
    // Create NotificationRequest with Slack channel
    notification := &v1alpha1.NotificationRequest{
        Spec: v1alpha1.NotificationRequestSpec{
            Channels: []v1alpha1.Channel{v1alpha1.ChannelSlack},
        },
    }

    // All 3 replicas will reconcile
    // Only 1 should deliver (others blocked by distributed lock)
    Eventually(func() int {
        return getSlackMessageCount()
    }, 30*time.Second, 1*time.Second).Should(Equal(1))
})
```

---

### E2E Tests (4 hours)

**File**: `test/e2e/notification/distributed_locking_test.go`

**Setup**: 3 NotificationRequest controller replicas + real Slack webhook

**Scenarios**:
1. ✅ Production scenario: 100 concurrent signals → 100 unique Slack messages (no duplicates)
2. ✅ Rate limit compliance: Slack deliveries respect 1 msg/sec/workspace
3. ✅ Pod crash recovery: Leader pod crashes → follower acquires lock
4. ✅ Metrics validation: `notification_lock_acquisition_failures_total` increments

---

## 📊 ACCEPTANCE CRITERIA

### Functional Requirements
- ✅ No duplicate deliveries in multi-replica deployments
- ✅ No duplicate deliveries across clusters (if federation enabled)
- ✅ Lock acquisition latency <20ms (P95)
- ✅ Lock acquisition failure rate <0.1%

### Non-Functional Requirements
- ✅ Zero impact when `USE_DISTRIBUTED_LOCKING=false` (default)
- ✅ Backward compatible (no API changes)
- ✅ RBAC permissions minimal (only Lease resources)
- ✅ No configuration required (hardcoded settings)

### Testing Requirements
- ✅ Unit test coverage >90%
- ✅ Integration tests pass with 3 replicas
- ✅ E2E tests validate production scenarios
- ✅ Performance tests: P95 latency <20ms increase

---

## 🚀 IMPLEMENTATION TIMELINE

### Sprint 1 (Day 1): Foundation
- ✅ Create `pkg/notification/processing/distributed_lock.go`
- ✅ Implement lock acquisition/release
- ✅ Add RBAC permissions
- ✅ Add POD_NAMESPACE/POD_NAME env vars

### Sprint 2 (Day 2): Integration
- ✅ Integrate with `pkg/notification/delivery/orchestrator.go`
- ✅ Add metrics (`notification_lock_acquisition_failures_total`)
- ✅ Unit tests (90%+ coverage)

### Sprint 3 (Day 3): Testing
- ✅ Integration tests (multi-replica scenarios)
- ✅ E2E tests (production scenarios)
- ✅ Performance validation (P95 latency)

---

## 🔒 RISKS & MITIGATION

### Risk 1: Increased Latency
**Impact**: Distributed lock acquisition adds K8s API roundtrip (5-10ms)
**Mitigation**:
- Only for external channels (Slack, email, webhook)
- Internal channels (console, file, log) skip distributed lock
- Performance tests validate <20ms P95 increase

### Risk 2: K8s API Availability
**Impact**: K8s API outage prevents lock acquisition
**Mitigation**:
- DD-NOT-008 still provides in-cluster deduplication
- Metrics track lock acquisition failures
- Graceful degradation: log error, continue with DD-NOT-008 only

### Risk 3: Lease Expiration Edge Cases
**Impact**: Pod crashes before releasing lock → 30s wait for expiration
**Mitigation**:
- 30s lease duration is reasonable for notification delivery (typically <1s)
- Automatic lease renewal every 10s
- Metrics track lock contention duration

---

## 📚 RELATED DOCUMENTATION

### Gateway Service (Aligned Pattern)
- [GW_DISTRIBUTED_LOCKING_READY_FOR_IMPLEMENTATION_DEC_30_2025.md](../handoff/GW_DISTRIBUTED_LOCKING_READY_FOR_IMPLEMENTATION_DEC_30_2025.md)
- [DD-GATEWAY-013: Multi-Replica Deduplication](../architecture/decisions/DD-GATEWAY-013-multi-replica-deduplication.md)

### Notification Service
- [DD-NOT-008: Production-Grade Concurrent Delivery Deduplication](../handoff/DD_NOT_008_PRODUCTION_CONCURRENCY_FIX_JAN11_2026.md)
- [DD-NOT-007: Delivery Orchestrator Registration Pattern](../architecture/decisions/DD-NOT-007-DELIVERY-ORCHESTRATOR-REGISTRATION-PATTERN.md)

### Remediation Orchestrator (Aligned Pattern)
- [RO_DISTRIBUTED_LOCKING_PLANS_COMPLETE_DEC_30_2025.md](../handoff/RO_DISTRIBUTED_LOCKING_PLANS_COMPLETE_DEC_30_2025.md)

---

## ✅ SIGN-OFF

**Technical Debt Owner**: Notification Team
**Priority**: P2 (Not required for V1.0 - DD-NOT-008 sufficient)
**Effort**: 2-3 days (16-24 hours)
**Target Release**: V1.1 or V2.0 (when multi-cluster/federation required)
**Dependency**: None (DD-NOT-008 already production-ready)

---

## 📌 DECISION: When to Implement

### ✅ Implement When:
1. **Multi-cluster deployments** planned (federation, disaster recovery)
2. **Shared rate-limited APIs** causing throttling (Slack workspace limits)
3. **Leader election disabled** for high availability (active-active)
4. **Compliance requirements** mandate zero duplicate deliveries

### ⏸️ Defer When:
1. **Single cluster only** (DD-NOT-008 sufficient)
2. **Leader election enabled** (default, only 1 active controller)
3. **No rate limit issues** observed in production
4. **V1.0 release timeline** prioritizes other features

---

**Status**: 📋 **Documented** - Ready for implementation when business requirements justify effort

**Current State**: DD-NOT-008 provides production-ready deduplication for single-cluster multi-replica deployments. This technical debt tracks future enhancements for multi-cluster scenarios.

**Confidence Assessment**: 90%
- ✅ Proven pattern (Gateway, Remediation Orchestrator)
- ✅ Kubernetes-native (coordination.k8s.io/v1 Lease API)
- ✅ Clear acceptance criteria and test plan
- ⚠️ Risk: K8s API dependency (mitigated by graceful degradation)
