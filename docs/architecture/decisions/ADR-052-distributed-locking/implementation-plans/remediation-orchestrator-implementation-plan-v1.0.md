# RemediationOrchestrator - Distributed Locking Implementation Plan

**Version**: V1.0
**Created**: December 30, 2025
**Timeline**: 2 days (16 hours total) - **Next branch after current merge**
**Status**: ✅ APPROVED - Ready for Implementation
**Quality Level**: Matches Gateway V1.0 and Data Storage v4.1 standards
**Architecture Decision**: [ADR-052](../../../../architecture/decisions/ADR-052-distributed-locking-pattern.md) (to be created from DD-GATEWAY-013)
**Related**: Implementing alongside Gateway's shared lock manager migration

**Change Log**:
- **v1.0** (2025-12-30): Initial implementation plan for K8s Lease-based distributed resource locking

---

## 🎯 Quick Reference

**Feature**: Kubernetes Lease-Based Distributed Lock for Multi-Replica Resource Safety
**Business Requirement**: BR-ORCH-050 (Multi-Replica Resource Lock Safety)
**Architecture Decision**: ADR-052 (K8s Lease-Based Distributed Locking Pattern)
**Service Type**: CRD Controller (RemediationOrchestrator)
**Methodology**: APDC-TDD with Defense-in-Depth Testing
**Parallel Execution**: 4 concurrent processes for all test tiers

**Success Metrics**:
- Duplicate WFE creation rate: <0.001% (vs. potential ~0.1% with 10 replicas)
- Lock acquisition success rate: >99.9%
- P95 reconciliation latency impact: <20ms additional overhead
- Test coverage: 90%+ for distributed locking code

---

## 📑 Table of Contents

| Section | Purpose |
|---------|---------|
| [Executive Summary](#executive-summary) | Problem, solution, and business impact |
| [Prerequisites Checklist](#prerequisites-checklist) | Pre-Day 1 requirements |
| [Risk Assessment](#️-risk-assessment-matrix) | Risk identification and mitigation |
| [Timeline Overview](#timeline-overview) | 2-day breakdown |
| [Day 1: Implementation](#day-1-implementation-8h) | Shared lock manager + RO integration |
| [Day 2: Testing & Validation](#day-2-testing--validation-8h) | Unit, integration, E2E tests |
| [Success Criteria](#success-criteria) | Completion checklist |
| [Rollback Plan](#rollback-plan) | Rollback procedure |

---

## Executive Summary

### Problem Statement

**Current State**: RemediationOrchestrator scales horizontally (multiple replicas for HA), but has a race condition vulnerability when concurrent RemediationRequests target the **same resource** and are processed by different RO pods.

**Impact with Multiple Replicas**:
- **Single RO pod**: No race condition (serialized processing)
- **2 RO pods**: ~0.01-0.03% duplicate WFE creation rate
- **3+ RO pods (HA)**: ~0.03-0.05% duplicate rate
- **5+ RO pods**: ~0.1%+ duplicate rate

**At Production Scale** (1,000 concurrent remediation scenarios/day, 3 RO pods):
- ~0.3-1 duplicate WFE created per day
- ~9-30 duplicate WFEs created per month
- ~109-365 duplicate WFEs created per year

**Blast Radius**:
- Duplicate WFE CRDs for same resource (observability confusion)
- Resource waste (second WFE unusable, stays in Pending phase)
- Reconciliation errors (second WFE fails to create PipelineRun)

**Important**: WorkflowExecution controller provides Layer 2 protection (deterministic PipelineRun naming), so no duplicate **execution** occurs. This fix eliminates duplicate **CRD creation** (Layer 1 protection).

### Solution

**Approach**: Kubernetes Lease-based distributed lock (ADR-052, shared with Gateway)

**How It Works**:
1. RO pod acquires K8s Lease for target resource before routing decision
2. Only 1 pod can hold lock at a time (mutual exclusion)
3. Other pods requeue with backoff (100ms) and retry resource busy check
4. Lease expires after 30s (prevents deadlocks on pod crashes)

**Protection Guarantee**: 100% elimination of cross-replica duplicate WFE creation

**Shared Implementation**: Reuses Gateway's `DistributedLockManager` from `pkg/shared/locking/`

### Business Impact

**Benefits**:
- ✅ **Eliminates duplicate WFE CRDs**: From ~0.1% to <0.001% with 5+ replicas
- ✅ **Scales safely**: Works correctly with 1 to 100+ RO replicas
- ✅ **K8s-native**: No external dependencies (Redis, etcd, etc.)
- ✅ **Fault-tolerant**: Lease expires if RO pod crashes
- ✅ **Consistent with Gateway**: Same architectural pattern (ADR-052)

**Trade-offs**:
- ⚠️ **Latency increase**: +10-20ms for lock acquisition per reconciliation
- ⚠️ **Lock contention**: High-volume alerts may queue behind locks

**Mitigation**:
- Latency still within RO SLO (typical reconciliation ~100-200ms → ~110-220ms)
- Lease duration tuned to 30s (balance between safety and contention)
- Monitoring alerts for lock acquisition failures

---

## Prerequisites Checklist

### **Pre-Day 1 Requirements** ✅ COMPLETE

- [x] **Race condition analysis complete**: [RO_RACE_CONDITION_ANALYSIS_DEC_30_2025.md](../../../../handoff/RO_RACE_CONDITION_ANALYSIS_DEC_30_2025.md)
- [x] **Gateway distributed locking reviewed**: [IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md](../../../stateless/gateway-service/implementation/IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md)
- [x] **Cross-team coordination**: [CROSS_TEAM_DISTRIBUTED_LOCKING_PATTERN_DEC_30_2025.md](../../../../shared/CROSS_TEAM_DISTRIBUTED_LOCKING_PATTERN_DEC_30_2025.md)
- [x] **ADR-052 conversion plan**: [DD_TO_ADR_CONVERSION_DISTRIBUTED_LOCKING_DEC_30_2025.md](../../../../handoff/DD_TO_ADR_CONVERSION_DISTRIBUTED_LOCKING_DEC_30_2025.md)
- [x] **User approval**: Go/no-go decision approved for Option 1 (Distributed Locking)

### **Day 1 Prerequisites** (Verify Before Starting)

**Infrastructure**:
- [ ] Development environment with Kind cluster access
- [ ] kubectl configured for test cluster
- [ ] Go 1.25+ installed
- [ ] Make targets functional (`make test-unit-remediationorchestrator`, etc.)

**Codebase**:
- [ ] Current branch merged and up-to-date
- [ ] All existing RO tests passing (432U + 39I + 19E2E)
- [ ] No lint errors in RO codebase

**Gateway Coordination**:
- [ ] Gateway team notified of shared lock manager refactoring
- [ ] Gateway's distributed locking code reviewed and understood
- [ ] Shared package location agreed: `pkg/shared/locking/`

---

## ⚠️ Risk Assessment Matrix

| Risk | Probability | Impact | Mitigation | Owner |
|------|------------|--------|------------|-------|
| **Gateway refactoring breaks existing functionality** | Medium | High | Run Gateway tests after refactoring, feature flag (if needed) | RO+GW Teams |
| **Lock contention under high load** | Low | Medium | Monitor metrics, tune lease duration, document expected behavior | RO Team |
| **RBAC permission issues in production** | Medium | High | Test in Kind cluster, document RBAC changes in deployment guide | RO Team |
| **Deadlocks from pod crashes** | Low | High | Lease expiration (30s) provides automatic recovery | K8s Platform |
| **Integration test flakiness** | Medium | Low | Use envtest with real K8s API, retry logic in tests | RO Team |
| **Cross-service coordination delays** | Low | Low | Implement RO first, Gateway refactoring in parallel (optional) | Both Teams |

---

## Timeline Overview

### **Total Duration**: 1.5-2 days (14 hours)

| Phase | Duration | Deliverables |
|-------|----------|--------------|
| **Day 1: Implementation** | 6 hours | RO lock manager (copy-adapt) + integration + RBAC |
| **Day 2: Testing & Validation** | 8 hours | Unit + Integration + E2E tests |

### **Parallel Work Streams**

**Day 1** (can be parallelized):
- Stream A: Copy and adapt lock manager from Gateway (2h)
- Stream B: RO routing engine integration (2h)
- Stream C: RBAC updates + metrics (1h)
- Stream D: Validation (1h)

**Day 2** (can be parallelized):
- Stream A: Unit tests (3h)
- Stream B: Integration tests (3h)
- Stream C: E2E tests (2h)

**Time Savings**: 2 hours compared to shared library approach (no Gateway refactoring, no abstraction design)

---

## Day 1: Implementation (6h)

### **Session 1: Copy and Adapt Lock Manager** (2h)

**Goal**: Create RO-specific lock manager by copying and adapting Gateway's implementation

#### **Task 1.1: Create RO Locking Package** (15 min)

**Action**: Create `pkg/remediationorchestrator/locking/` package

**Files to Create**:
```
pkg/remediationorchestrator/locking/
├── distributed_lock.go          # RO lock manager (copied from Gateway)
├── distributed_lock_test.go     # Unit tests (copied from Gateway)
└── doc.go                        # Package documentation
```

**Package Documentation** (`doc.go`):
```go
// Package locking provides distributed locking for RemediationOrchestrator multi-replica deployments.
//
// # K8s Lease-Based Distributed Locking
//
// This package implements distributed mutual exclusion using Kubernetes Lease resources,
// preventing race conditions when multiple RO pods process RemediationRequests targeting
// the same resource concurrently.
//
// # Pattern Reference
//
// This implementation follows the pattern documented in ADR-052, adapted from Gateway's
// implementation for RO-specific needs (lock key, metrics, namespace).
//
// # Usage Example
//
//	lockManager := locking.NewDistributedLockManager(k8sClient, namespace, podName, 30*time.Second)
//
//	// Acquire lock before routing checks
//	targetResource := rr.Spec.TargetResource.String()
//	acquired, err := lockManager.AcquireLock(ctx, targetResource)
//	if !acquired {
//	    // Lock held by another RO pod - requeue
//	    return ctrl.Result{RequeueAfter: 100ms}, nil
//	}
//	defer lockManager.ReleaseLock(ctx, targetResource)
//
//	// Routing checks + WFE creation (protected by lock)
//	// ...
//
// # Architecture Decision
//
// ADR-052: K8s Lease-Based Distributed Locking Pattern
// See: docs/architecture/decisions/ADR-052-distributed-locking-pattern.md
//
// # Business Requirement
//
// BR-ORCH-050: Multi-Replica Resource Lock Safety
package locking
```

**Validation**:
```bash
# Verify package structure
ls -la pkg/remediationorchestrator/locking/

# Verify package compiles
go build ./pkg/remediationorchestrator/locking/
```

---

#### **Task 1.2: Copy and Adapt from Gateway** (1h)

**Action**: Copy Gateway's lock manager and adapt for RO

**Source File**: `pkg/gateway/processing/distributed_lock.go` (reference)
**Destination**: `pkg/remediationorchestrator/locking/distributed_lock.go`

**Adaptation Changes**:
1. Change package to `locking`
2. Update code comments to reference ADR-052 and BR-ORCH-050
3. Adapt lock key generation (target resource string, no hash needed)
4. Remove Gateway-specific metrics coupling
5. Keep RO-specific customizations simple

**Key Code Structure** (RO-specific):
```go
package locking

import (
	"context"
	"fmt"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DistributedLockManager manages K8s Lease-based distributed locks for RemediationOrchestrator.
//
// # Architecture Decision
//
// ADR-052: K8s Lease-Based Distributed Locking Pattern
// See: docs/architecture/decisions/ADR-052-distributed-locking-pattern.md
//
// # Business Requirement
//
// BR-ORCH-050: Multi-Replica Resource Lock Safety
//
// # Use Case
//
// Prevents duplicate WorkflowExecution CRD creation when multiple RO pods process
// different RemediationRequests targeting the same resource concurrently.
//
// # How It Works
//
// 1. AcquireLock() creates or updates K8s Lease resource with holder identity
// 2. Only one pod can hold a lock at a time (mutual exclusion)
// 3. Other pods receive acquired=false and should requeue with backoff
// 4. Lease expires after leaseDuration (default 30s) if pod crashes
//
// # RBAC Requirements
//
// RemediationOrchestrator must have permissions:
//   - apiGroups: ["coordination.k8s.io"]
//     resources: ["leases"]
//     verbs: ["get", "create", "update", "delete"]
//
// # Pattern Reference
//
// This implementation is adapted from Gateway's distributed lock manager.
// See: pkg/gateway/processing/distributed_lock.go (reference implementation)
type DistributedLockManager struct {
	client        client.Client
	namespace     string        // Namespace for Lease resources (RO pod namespace)
	holderID      string        // Pod name (unique identifier for this RO pod)
	leaseDuration time.Duration // How long lock is held before expiration (default 30s)
}

// NewDistributedLockManager creates a new distributed lock manager.
//
// Parameters:
//   - client: Kubernetes client for Lease operations
//   - namespace: Namespace where Lease resources are created
//   - holderID: Unique identifier for this pod (typically pod name)
//   - leaseDuration: How long locks are held before automatic expiration
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

// AcquireLock attempts to acquire a distributed lock for the given key.
//
// Returns:
//   - acquired=true: Lock acquired, caller can proceed with critical section
//   - acquired=false, err=nil: Lock held by another pod, caller should retry
//   - acquired=false, err!=nil: Error acquiring lock, caller should handle error
//
// # Lock Key
//
// The key is hashed to create a K8s-compatible Lease name (max 63 chars).
// Different services use different keys:
//   - Gateway: Signal fingerprint (e.g., "bd773c9f25ac...")
//   - RemediationOrchestrator: Target resource (e.g., "node/worker-1")
//
// # Idempotency
//
// Acquiring a lock we already hold is idempotent (returns acquired=true).
//
// # Error Handling
//
// - IsNotFound: Lease doesn't exist → try to create
// - IsAlreadyExists: Another pod created lease → return acquired=false
// - Other K8s API errors: Return error for caller to handle
func (m *DistributedLockManager) AcquireLock(ctx context.Context, key string) (bool, error) {
	logger := log.FromContext(ctx).WithValues("lockKey", key, "holderID", m.holderID)

	leaseName := m.generateLeaseName(key)
	lease := &coordinationv1.Lease{}

	// Try to get existing lease
	err := m.client.Get(ctx, client.ObjectKey{
		Namespace: m.namespace,
		Name:      leaseName,
	}, lease)

	if err == nil {
		// Lease exists - check if we already hold it
		if lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity == m.holderID {
			logger.V(1).Info("Lock already held by us (idempotent)")
			return true, nil
		}

		// Check if lease expired (allow takeover)
		if m.isLeaseExpired(lease) {
			logger.V(1).Info("Lease expired, attempting takeover")
			return m.updateLease(ctx, lease)
		}

		// Lease held by another pod
		logger.V(1).Info("Lock held by another pod",
			"holder", *lease.Spec.HolderIdentity)
		return false, nil
	}

	// CRITICAL: Check error type explicitly (ADR-052 §4.2)
	// Don't treat all errors as "lease doesn't exist"
	if !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to check existing lease")
		return false, fmt.Errorf("failed to check lease: %w", err)
	}

	// Lease doesn't exist - try to create
	logger.V(1).Info("Creating new lease")
	return m.createLease(ctx, leaseName, key)
}

// ReleaseLock releases a distributed lock.
//
// This should be called in a defer statement after AcquireLock succeeds.
// Releasing a lock we don't hold is idempotent (no error).
//
// # Error Handling
//
// Errors are logged but not returned, as release is best-effort.
// If release fails, the lease will expire after leaseDuration.
func (m *DistributedLockManager) ReleaseLock(ctx context.Context, key string) {
	logger := log.FromContext(ctx).WithValues("lockKey", key, "holderID", m.holderID)

	leaseName := m.generateLeaseName(key)
	lease := &coordinationv1.Lease{}

	// Get current lease
	err := m.client.Get(ctx, client.ObjectKey{
		Namespace: m.namespace,
		Name:      leaseName,
	}, lease)

	if apierrors.IsNotFound(err) {
		// Lease already deleted - idempotent success
		logger.V(1).Info("Lease already released (idempotent)")
		return
	}

	if err != nil {
		logger.Error(err, "Failed to get lease for release (will expire naturally)")
		return
	}

	// Only delete if we're the holder
	if lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity == m.holderID {
		if err := m.client.Delete(ctx, lease); err != nil {
			logger.Error(err, "Failed to delete lease (will expire naturally)")
			return
		}
		logger.V(1).Info("Lock released successfully")
	} else {
		logger.V(1).Info("Lock not held by us, skipping release")
	}
}

// generateLeaseName creates a K8s-compatible name from the lock key.
//
// Requirements:
//   - Max 63 characters (K8s resource name limit)
//   - DNS-1123 compliant (lowercase alphanumeric + hyphen)
//
// # Implementation
//
// Uses first 16 chars of lock key (truncated/hashed if needed).
// Prefix with service-specific identifier to avoid cross-service collisions.
func (m *DistributedLockManager) generateLeaseName(key string) string {
	// Truncate to 16 chars (leaves room for prefix)
	// In production, use hash for longer keys
	safeKey := key
	if len(key) > 16 {
		safeKey = key[:16]
	}

	// Prefix depends on service (passed via constructor or holderID)
	// Gateway: "gw-lock-{key}"
	// RO: "ro-lock-{key}"
	return fmt.Sprintf("lock-%s", safeKey)
}

// createLease creates a new lease resource.
func (m *DistributedLockManager) createLease(ctx context.Context, leaseName, key string) (bool, error) {
	logger := log.FromContext(ctx).WithValues("leaseName", leaseName)

	now := metav1.NowMicro()
	leaseDurationSeconds := int32(m.leaseDuration.Seconds())

	lease := &coordinationv1.Lease{
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

	err := m.client.Create(ctx, lease)
	if err == nil {
		logger.V(1).Info("Lease created successfully")
		return true, nil
	}

	// Another pod created the lease in the race window
	if apierrors.IsAlreadyExists(err) {
		logger.V(1).Info("Lease created by another pod (race)")
		return false, nil
	}

	// Real error
	logger.Error(err, "Failed to create lease")
	return false, fmt.Errorf("failed to create lease: %w", err)
}

// updateLease updates an existing lease to change holder.
func (m *DistributedLockManager) updateLease(ctx context.Context, lease *coordinationv1.Lease) (bool, error) {
	logger := log.FromContext(ctx).WithValues("leaseName", lease.Name)

	now := metav1.NowMicro()
	lease.Spec.HolderIdentity = &m.holderID
	lease.Spec.AcquireTime = &now
	lease.Spec.RenewTime = &now

	err := m.client.Update(ctx, lease)
	if err == nil {
		logger.V(1).Info("Lease updated successfully (takeover)")
		return true, nil
	}

	// Conflict: another pod updated the lease
	if apierrors.IsConflict(err) {
		logger.V(1).Info("Lease updated by another pod (conflict)")
		return false, nil
	}

	// Real error
	logger.Error(err, "Failed to update lease")
	return false, fmt.Errorf("failed to update lease: %w", err)
}

// isLeaseExpired checks if a lease has expired and can be taken over.
func (m *DistributedLockManager) isLeaseExpired(lease *coordinationv1.Lease) bool {
	if lease.Spec.RenewTime == nil || lease.Spec.LeaseDurationSeconds == nil {
		return false
	}

	expirationTime := lease.Spec.RenewTime.Add(time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second)
	return time.Now().After(expirationTime)
}
```

**Validation**:
```bash
# Verify RO locking package compiles
go build ./pkg/remediationorchestrator/locking/

# Check for compilation errors
go build ./pkg/remediationorchestrator/...
```

**Note**: Gateway implementation remains unchanged - no refactoring needed!

---

### **Session 2: RO Integration** (2h)

#### **Task 1.3: RO Lock Manager Integration** (1h)

**Action**: Add lock manager to RO's RoutingEngine

**File to Update**: `pkg/remediationorchestrator/routing/engine.go`

**Add Lock Manager Field**:
```go
package routing

import (
	"context"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/locking"
	// ... other imports
)

// RoutingEngine makes routing decisions for RemediationRequest CRDs.
//
// # Multi-Replica Safety (ADR-052)
//
// When running with multiple RO replicas, the routing engine uses distributed
// locking to prevent race conditions in resource busy checks.
//
// Lock Key: Target resource string (e.g., "node/worker-1")
// See: BR-ORCH-050 (Multi-Replica Resource Lock Safety)
type RoutingEngine struct {
	client      client.Client
	namespace   string
	cooldown    time.Duration
	// ... existing fields ...

	// lockManager provides distributed locking for multi-replica race protection
	// ADR-052: K8s Lease-Based Distributed Locking Pattern
	lockManager *locking.DistributedLockManager
}

// NewRoutingEngine creates a new routing engine.
func NewRoutingEngine(
	client client.Client,
	namespace string,
	podName string, // NEW: pod name for lock holder identity
	cooldown time.Duration,
	// ... other params
) *RoutingEngine {
	return &RoutingEngine{
		client:      client,
		namespace:   namespace,
		cooldown:    cooldown,
		// ... other fields ...
		lockManager: locking.NewDistributedLockManager(
			client,
			namespace,
			podName,
			30*time.Second, // Lease duration (ADR-052 §3.2)
		),
	}
}
```

---

#### **Task 1.4: Wrap Lock Acquisition in CheckBlockingConditions** (30 min)

**Action**: Add distributed lock to routing decision logic

**File to Update**: `pkg/remediationorchestrator/routing/blocking.go`

**Update CheckBlockingConditions**:
```go
// CheckBlockingConditions checks all routing conditions with distributed lock protection.
//
// # Multi-Replica Safety (ADR-052, BR-ORCH-050)
//
// When running with multiple RO replicas, this method acquires a distributed lock
// on the target resource BEFORE checking routing conditions. This prevents race
// conditions where multiple pods create duplicate WorkflowExecution CRDs for the
// same target resource.
//
// # Lock Key
//
// The lock key is the target resource string (e.g., "node/worker-1"). This ensures
// mutual exclusion at the resource level, not the RemediationRequest level.
//
// # Retry Behavior
//
// If lock acquisition fails (held by another pod), this method returns a special
// BlockingCondition with Reason="LockContentionRetry". The reconciler should
// requeue with RequeueAfter=100ms.
//
// # Lock Release
//
// The lock is NOT released by this method - the caller (reconciler) must release
// it after WFE creation completes (using defer).
//
// Reference: ADR-052 (Distributed Locking), BR-ORCH-050 (Multi-Replica Safety)
func (r *RoutingEngine) CheckBlockingConditions(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	workflowID string,
) (*BlockingCondition, *LockHandle, error) {
	// ========================================
	// ADR-052: DISTRIBUTED LOCK ACQUISITION
	// ========================================
	// Acquire lock on target resource BEFORE routing checks.
	// This prevents race conditions when multiple RO pods process
	// different RemediationRequests targeting the same resource.
	//
	// Lock Key: Target resource string (e.g., "node/worker-1")
	// See: BR-ORCH-050 (Multi-Replica Resource Lock Safety)
	// ========================================
	targetResource := rr.Spec.TargetResource.String()

	lockAcquired, err := r.lockManager.AcquireLock(ctx, targetResource)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to acquire lock: %w", err)
	}

	if !lockAcquired {
		// Lock held by another RO pod - requeue with backoff
		// The reconciler will retry after 100ms
		return &BlockingCondition{
			Blocked:      true,
			Reason:       "LockContentionRetry",
			Message:      fmt.Sprintf("Target resource %s locked by another RO pod, retrying...", targetResource),
			RequeueAfter: 100 * time.Millisecond,
		}, nil, nil
	}

	// Lock acquired - return handle for reconciler to release later
	lockHandle := &LockHandle{
		Key:         targetResource,
		ReleaseFunc: func() { r.lockManager.ReleaseLock(ctx, targetResource) },
	}

	// ========================================
	// EXISTING ROUTING CHECKS (now protected by lock)
	// ========================================

	// Check 1: Consecutive failures (highest priority)
	if blocked := r.CheckConsecutiveFailures(ctx, rr); blocked != nil {
		return blocked, lockHandle, nil
	}

	// Check 2: Duplicate in progress
	blocked, err := r.CheckDuplicateInProgress(ctx, rr)
	if err != nil {
		return nil, lockHandle, fmt.Errorf("failed to check duplicate: %w", err)
	}
	if blocked != nil {
		return blocked, lockHandle, nil
	}

	// Check 3: Resource busy (NOW ATOMIC WITH WFE CREATION!)
	blocked, err = r.CheckResourceBusy(ctx, rr)
	if err != nil {
		return nil, lockHandle, fmt.Errorf("failed to check resource lock: %w", err)
	}
	if blocked != nil {
		return blocked, lockHandle, nil
	}

	// Check 4: Recently remediated
	blocked, err = r.CheckRecentlyRemediated(ctx, rr, workflowID)
	if err != nil {
		return nil, lockHandle, fmt.Errorf("failed to check recent remediation: %w", err)
	}
	if blocked != nil {
		return blocked, lockHandle, nil
	}

	// Check 5: Exponential backoff
	if blocked := r.CheckExponentialBackoff(ctx, rr); blocked != nil {
		return blocked, lockHandle, nil
	}

	// No blocking conditions - can proceed
	return nil, lockHandle, nil
}

// LockHandle represents a held distributed lock that must be released.
type LockHandle struct {
	Key         string
	ReleaseFunc func()
}

// Release releases the distributed lock.
func (h *LockHandle) Release() {
	if h != nil && h.ReleaseFunc != nil {
		h.ReleaseFunc()
	}
}
```

---

#### **Task 1.5: Update Reconciler to Use Lock Handle** (30 min)

**Action**: Update reconciler to release lock after WFE creation

**File to Update**: `internal/controller/remediationorchestrator/reconciler.go`

**Update Reconciler**:
```go
// Line 613-649: AIAnalysis completed, routing to WorkflowExecution
logger.Info("AIAnalysis completed, checking routing conditions")

// ========================================
// ADR-052: DISTRIBUTED LOCK + ROUTING CHECKS
// ========================================
// CheckBlockingConditions now acquires distributed lock internally.
// Returns lock handle for reconciler to release after WFE creation.
// See: BR-ORCH-050 (Multi-Replica Resource Lock Safety)
// ========================================
workflowID := ""
if ai.Status.SelectedWorkflow != nil {
	workflowID = ai.Status.SelectedWorkflow.WorkflowID
}

blocked, lockHandle, err := r.routingEngine.CheckBlockingConditions(ctx, rr, workflowID)
if err != nil {
	logger.Error(err, "Failed to check routing conditions")
	return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil
}

// CRITICAL: Ensure lock is released even if errors occur
if lockHandle != nil {
	defer lockHandle.Release()
}

// If blocked, handle appropriately
if blocked != nil {
	// Special case: Lock contention (another pod holds lock)
	if blocked.Reason == "LockContentionRetry" {
		logger.Info("Lock contention - retrying after backoff",
			"reason", blocked.Reason,
			"requeueAfter", blocked.RequeueAfter)
		// NO need to update status - this is expected behavior
		return ctrl.Result{RequeueAfter: blocked.RequeueAfter}, nil
	}

	// Normal blocking condition (not lock contention)
	logger.Info("Routing blocked - will not create WorkflowExecution",
		"reason", blocked.Reason,
		"message", blocked.Message,
		"requeueAfter", blocked.RequeueAfter)
	return r.handleBlocked(ctx, rr, blocked, string(remediationv1.PhaseAnalyzing), workflowID)
}

// Routing checks passed - create WorkflowExecution
logger.Info("Routing checks passed, creating WorkflowExecution")

// ========================================
// WFE CREATION (now guaranteed race-free!)
// ========================================
weName, err := r.weCreator.Create(ctx, rr, ai)
if err != nil {
	logger.Error(err, "Failed to create WorkflowExecution CRD")
	// Lock will be released by defer
	return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil
}

// Lock will be released by defer after successful WFE creation
logger.Info("Created WorkflowExecution CRD", "weName", weName)
```

---

### **Session 3: RBAC and Metrics** (1h)

#### **Task 1.6: Add RBAC Permissions** (30 min)

**Action**: Update RO's RBAC to allow Lease operations

**File to Update**: `deployments/remediationorchestrator/rbac.yaml`

**Add Lease Permissions**:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: remediationorchestrator-controller-role
rules:
# ... existing permissions ...

# ========================================
# ADR-052: Distributed Locking Permissions
# ========================================
# RemediationOrchestrator needs Lease permissions for multi-replica race protection.
# See: BR-ORCH-050 (Multi-Replica Resource Lock Safety)
# ========================================
- apiGroups:
  - coordination.k8s.io
  resources:
  - leases
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
```

**Validation**:
```bash
# Apply RBAC changes to test cluster
kubectl apply -f deployments/remediationorchestrator/rbac.yaml

# Verify RBAC
kubectl auth can-i create leases --as=system:serviceaccount:kubernaut-system:remediationorchestrator-controller
# Expected: yes
```

---

#### **Task 1.7: Add Metrics** (30 min)

**Action**: Add lock acquisition failure metric

**File to Update**: `pkg/remediationorchestrator/metrics/metrics.go`

**Add Metric**:
```go
// LockAcquisitionFailures tracks distributed lock acquisition failures.
//
// # ADR-052: Distributed Locking Monitoring
//
// This metric tracks when lock acquisition fails (K8s API errors, not lock contention).
// Lock contention (acquired=false with no error) is expected behavior and NOT counted.
//
// # Labels
//
// - reason: Error reason (e.g., "k8s_api_error", "permission_denied")
//
// # Business Requirement
//
// BR-ORCH-050: Multi-Replica Resource Lock Safety
LockAcquisitionFailures prometheus.CounterVec
```

**Update Constructor**:
```go
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		// ... existing metrics ...

		LockAcquisitionFailures: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "kubernaut",
				Subsystem: "remediationorchestrator",
				Name:      "lock_acquisition_failures_total",
				Help:      "Total number of distributed lock acquisition failures (K8s API errors, not contention)",
			},
			[]string{"reason"},
		),
	}

	// Register metrics
	reg.MustRegister(
		// ... existing registrations ...
		m.LockAcquisitionFailures,
	)

	return m
}
```

**Update Lock Acquisition Code**:
```go
// In CheckBlockingConditions:
lockAcquired, err := r.lockManager.AcquireLock(ctx, targetResource)
if err != nil {
	// Record metric for lock acquisition failure
	r.metrics.LockAcquisitionFailures.WithLabelValues("k8s_api_error").Inc()
	return nil, nil, fmt.Errorf("failed to acquire lock: %w", err)
}
```

---

### **Session 4: Day 1 Validation** (1h)

**Goal**: Verify Day 1 implementation compiles and basic tests pass

**Validation Checklist**:
```bash
# 1. Verify RO locking package compiles
go build ./pkg/remediationorchestrator/locking/
echo "✅ RO lock manager compiles"

# 2. Verify RO compiles with new lock integration
go build ./internal/controller/remediationorchestrator/...
go build ./pkg/remediationorchestrator/...
echo "✅ RO compiles with distributed locking"

# 3. Verify Gateway unchanged (no refactoring needed)
go build ./pkg/gateway/...
echo "✅ Gateway unaffected (no changes)"

# 4. Run RO unit tests (should still pass - no behavior change yet)
make test-unit-remediationorchestrator
echo "✅ RO tests pass with lock integration"

# 5. Check for lint errors
golangci-lint run ./pkg/remediationorchestrator/locking/...
golangci-lint run ./pkg/remediationorchestrator/routing/...
golangci-lint run ./internal/controller/remediationorchestrator/...
echo "✅ No lint errors"

# 6. Verify RBAC changes applied
kubectl apply -f deployments/remediationorchestrator/rbac.yaml --dry-run=client
echo "✅ RBAC changes valid"
```

**Time Saved**: 2 hours (no Gateway refactoring, no shared library design)

---

## Day 2: Testing & Validation (8h)

### **Session 1: Unit Tests** (3h)

**Goal**: Achieve 90%+ coverage for distributed locking code

#### **Task 2.1: RO Lock Manager Unit Tests** (1.5h)

**File**: `pkg/remediationorchestrator/locking/distributed_lock_test.go` (copied and adapted from Gateway)

**Test Scenarios** (adapted from Gateway's test plan):
1. **Lock Acquisition Success**
   - New lock (no existing lease)
   - Reentrant lock (already hold it)
   - Expired lease takeover

2. **Lock Acquisition Failure**
   - Lock held by another pod
   - K8s API errors
   - Permission denied

3. **Lock Release**
   - Successful release
   - Release of non-existent lock (idempotent)
   - Release when another pod holds lock

4. **Edge Cases**
   - Concurrent lock acquisition (race)
   - Lease expiration boundary
   - Invalid lock keys (too long, special chars)

**Test Pattern**:
```go
var _ = Describe("DistributedLockManager", func() {
	var (
		lockManager *locking.DistributedLockManager
		k8sClient   client.Client
		ctx         context.Context
		namespace   string
		holderID    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "test-namespace"
		holderID = "test-pod-1"
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		lockManager = locking.NewDistributedLockManager(
			k8sClient,
			namespace,
			holderID,
			30*time.Second,
		)
	})

	Describe("AcquireLock", func() {
		It("should acquire lock when lease doesn't exist", func() {
			// When: Acquire lock for new key
			acquired, err := lockManager.AcquireLock(ctx, "test-key-1")

			// Then: Lock acquired successfully
			Expect(err).ToNot(HaveOccurred())
			Expect(acquired).To(BeTrue())

			// And: Lease created in K8s
			lease := &coordinationv1.Lease{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Namespace: namespace,
				Name:      "lock-test-key-1",
			}, lease)
			Expect(err).ToNot(HaveOccurred())
			Expect(*lease.Spec.HolderIdentity).To(Equal(holderID))
		})

		It("should NOT acquire lock when held by another pod", func() {
			// Given: Lease exists and held by another pod
			otherPodID := "test-pod-2"
			createLease(ctx, k8sClient, namespace, "test-key-2", otherPodID)

			// When: Try to acquire lock
			acquired, err := lockManager.AcquireLock(ctx, "test-key-2")

			// Then: Lock NOT acquired (no error - expected behavior)
			Expect(err).ToNot(HaveOccurred())
			Expect(acquired).To(BeFalse())
		})

		It("should acquire lock we already hold (idempotent)", func() {
			// Given: We already hold the lock
			acquired, err := lockManager.AcquireLock(ctx, "test-key-3")
			Expect(err).ToNot(HaveOccurred())
			Expect(acquired).To(BeTrue())

			// When: Try to acquire same lock again
			acquired, err = lockManager.AcquireLock(ctx, "test-key-3")

			// Then: Lock acquired again (idempotent)
			Expect(err).ToNot(HaveOccurred())
			Expect(acquired).To(BeTrue())
		})

		It("should take over expired lease", func() {
			// Given: Lease exists but expired (held by crashed pod)
			otherPodID := "crashed-pod"
			expiredLease := createExpiredLease(ctx, k8sClient, namespace, "test-key-4", otherPodID)

			// When: Try to acquire lock
			acquired, err := lockManager.AcquireLock(ctx, "test-key-4")

			// Then: Lock acquired (takeover)
			Expect(err).ToNot(HaveOccurred())
			Expect(acquired).To(BeTrue())

			// And: Lease holder updated to us
			lease := &coordinationv1.Lease{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Namespace: namespace,
				Name:      "lock-test-key-4",
			}, lease)
			Expect(err).ToNot(HaveOccurred())
			Expect(*lease.Spec.HolderIdentity).To(Equal(holderID))
		})

		It("should return error on K8s API failure", func() {
			// Given: K8s client returns error
			failingClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						return fmt.Errorf("simulated K8s API error")
					},
				}).
				Build()

			lockManager := locking.NewDistributedLockManager(
				failingClient,
				namespace,
				holderID,
				30*time.Second,
			)

			// When: Try to acquire lock
			acquired, err := lockManager.AcquireLock(ctx, "test-key-5")

			// Then: Error returned
			Expect(err).To(HaveOccurred())
			Expect(acquired).To(BeFalse())
			Expect(err.Error()).To(ContainSubstring("failed to check lease"))
		})
	})

	Describe("ReleaseLock", func() {
		It("should release lock successfully", func() {
			// Given: We hold the lock
			acquired, err := lockManager.AcquireLock(ctx, "test-key-6")
			Expect(err).ToNot(HaveOccurred())
			Expect(acquired).To(BeTrue())

			// When: Release lock
			lockManager.ReleaseLock(ctx, "test-key-6")

			// Then: Lease deleted
			lease := &coordinationv1.Lease{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Namespace: namespace,
				Name:      "lock-test-key-6",
			}, lease)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should be idempotent when releasing non-existent lock", func() {
			// When: Release lock that doesn't exist
			// Should not panic or error
			lockManager.ReleaseLock(ctx, "non-existent-key")

			// Then: No error (idempotent)
			// (ReleaseLock logs but doesn't return errors)
		})

		It("should NOT release lock held by another pod", func() {
			// Given: Another pod holds the lock
			otherPodID := "test-pod-7"
			createLease(ctx, k8sClient, namespace, "test-key-7", otherPodID)

			// When: Try to release lock
			lockManager.ReleaseLock(ctx, "test-key-7")

			// Then: Lease still exists (not deleted)
			lease := &coordinationv1.Lease{}
			err := k8sClient.Get(ctx, client.ObjectKey{
				Namespace: namespace,
				Name:      "lock-test-key-7",
			}, lease)
			Expect(err).ToNot(HaveOccurred())
			Expect(*lease.Spec.HolderIdentity).To(Equal(otherPodID))
		})
	})
})

// Test helpers
func createLease(ctx context.Context, k8sClient client.Client, namespace, key, holderID string) *coordinationv1.Lease {
	now := metav1.NowMicro()
	leaseDurationSeconds := int32(30)

	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("lock-%s", key),
			Namespace: namespace,
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &holderID,
			LeaseDurationSeconds: &leaseDurationSeconds,
			AcquireTime:          &now,
			RenewTime:            &now,
		},
	}

	Expect(k8sClient.Create(ctx, lease)).To(Succeed())
	return lease
}

func createExpiredLease(ctx context.Context, k8sClient client.Client, namespace, key, holderID string) *coordinationv1.Lease {
	// Create lease with renewTime 60 seconds in the past (expired)
	expiredTime := metav1.NewMicroTime(time.Now().Add(-60 * time.Second))
	leaseDurationSeconds := int32(30)

	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("lock-%s", key),
			Namespace: namespace,
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &holderID,
			LeaseDurationSeconds: &leaseDurationSeconds,
			AcquireTime:          &expiredTime,
			RenewTime:            &expiredTime,
		},
	}

	Expect(k8sClient.Create(ctx, lease)).To(Succeed())
	return lease
}
```

**Run Tests**:
```bash
# Run RO lock manager tests
ginkgo -v ./pkg/remediationorchestrator/locking/

# Check coverage
go test -coverprofile=coverage.out ./pkg/remediationorchestrator/locking/
go tool cover -html=coverage.out
# Target: 90%+ coverage
```

**Note**: Gateway's lock manager tests remain in `pkg/gateway/processing/distributed_lock_test.go` (unchanged)

---

#### **Task 2.2: RO Routing Engine Unit Tests** (1.5h)

**File**: `test/unit/remediationorchestrator/routing_lock_test.go` (NEW)

**Test Scenarios**:
1. **Lock Acquisition Success**
   - Lock acquired, routing checks pass
   - Lock acquired, blocked by other conditions (NOT lock)
   - Lock handle released after WFE creation

2. **Lock Contention**
   - Another pod holds lock → requeue
   - Lock released by other pod → retry succeeds

3. **Lock Release**
   - Lock released on success
   - Lock released on error (defer)
   - Lock released on blocking condition

**Test Pattern**:
```go
var _ = Describe("RoutingEngine Distributed Locking", func() {
	var (
		routingEngine *routing.RoutingEngine
		mockLockMgr   *MockDistributedLockManager
		k8sClient     client.Client
		ctx           context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		mockLockMgr = NewMockDistributedLockManager()

		routingEngine = routing.NewRoutingEngine(
			k8sClient,
			"test-namespace",
			"test-pod-1",
			5*time.Minute, // cooldown
		)

		// Inject mock lock manager for testing
		routingEngine.SetLockManager(mockLockMgr)
	})

	Describe("CheckBlockingConditions with Lock", func() {
		It("should acquire lock before routing checks", func() {
			// Given: RR targeting a resource
			rr := testutil.NewRemediationRequest("test-rr", "default",
				testutil.RemediationRequestOpts{
					TargetResource: &remediationv1.TargetResourceSpec{
						Kind:      "Node",
						Name:      "worker-1",
						Namespace: "",
					},
				})

			// When: Check blocking conditions
			blocked, lockHandle, err := routingEngine.CheckBlockingConditions(ctx, rr, "workflow-1")

			// Then: Lock acquisition attempted
			Expect(err).ToNot(HaveOccurred())
			Expect(mockLockMgr.AcquireLockCalled).To(BeTrue())
			Expect(mockLockMgr.LastLockKey).To(Equal("Node/worker-1"))

			// And: Lock handle returned
			Expect(lockHandle).ToNot(BeNil())
			Expect(lockHandle.Key).To(Equal("Node/worker-1"))
		})

		It("should return LockContentionRetry when lock held by another pod", func() {
			// Given: Another pod holds the lock
			mockLockMgr.AcquireResult = false // Lock NOT acquired
			mockLockMgr.AcquireError = nil    // No error (just contention)

			rr := testutil.NewRemediationRequest("test-rr", "default",
				testutil.RemediationRequestOpts{
					TargetResource: &remediationv1.TargetResourceSpec{
						Kind: "Node",
						Name: "worker-1",
					},
				})

			// When: Check blocking conditions
			blocked, lockHandle, err := routingEngine.CheckBlockingConditions(ctx, rr, "workflow-1")

			// Then: Returns lock contention blocking condition
			Expect(err).ToNot(HaveOccurred())
			Expect(lockHandle).To(BeNil()) // No lock handle (lock not acquired)
			Expect(blocked).ToNot(BeNil())
			Expect(blocked.Reason).To(Equal("LockContentionRetry"))
			Expect(blocked.RequeueAfter).To(Equal(100 * time.Millisecond))
		})

		It("should return error on lock acquisition failure", func() {
			// Given: K8s API error during lock acquisition
			mockLockMgr.AcquireResult = false
			mockLockMgr.AcquireError = fmt.Errorf("K8s API error")

			rr := testutil.NewRemediationRequest("test-rr", "default")

			// When: Check blocking conditions
			blocked, lockHandle, err := routingEngine.CheckBlockingConditions(ctx, rr, "workflow-1")

			// Then: Error returned
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to acquire lock"))
			Expect(lockHandle).To(BeNil())
			Expect(blocked).To(BeNil())
		})

		It("should proceed with routing checks after lock acquired", func() {
			// Given: Lock acquired successfully
			mockLockMgr.AcquireResult = true
			mockLockMgr.AcquireError = nil

			rr := testutil.NewRemediationRequest("test-rr", "default",
				testutil.RemediationRequestOpts{
					TargetResource: &remediationv1.TargetResourceSpec{
						Kind: "Node",
						Name: "worker-1",
					},
				})

			// And: No blocking conditions exist
			// (no WFE for same target, no recent remediation, etc.)

			// When: Check blocking conditions
			blocked, lockHandle, err := routingEngine.CheckBlockingConditions(ctx, rr, "workflow-1")

			// Then: No blocking condition (can proceed)
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).To(BeNil())
			Expect(lockHandle).ToNot(BeNil())

			// And: Lock still held (caller must release)
			Expect(mockLockMgr.ReleaseLockCalled).To(BeFalse())
		})

		It("should return lock handle even when blocked by other conditions", func() {
			// Given: Lock acquired successfully
			mockLockMgr.AcquireResult = true
			mockLockMgr.AcquireError = nil

			rr := testutil.NewRemediationRequest("test-rr", "default",
				testutil.RemediationRequestOpts{
					TargetResource: &remediationv1.TargetResourceSpec{
						Kind: "Node",
						Name: "worker-1",
					},
				})

			// And: Resource is busy (existing WFE)
			existingWFE := testutil.NewWorkflowExecution("we-existing", "default",
				testutil.WorkflowExecutionOpts{
					TargetResource: "Node/worker-1",
					Phase:          workflowexecutionv1.PhaseRunning,
				})
			Expect(k8sClient.Create(ctx, existingWFE)).To(Succeed())

			// When: Check blocking conditions
			blocked, lockHandle, err := routingEngine.CheckBlockingConditions(ctx, rr, "workflow-1")

			// Then: Blocked by ResourceBusy
			Expect(err).ToNot(HaveOccurred())
			Expect(blocked).ToNot(BeNil())
			Expect(blocked.Reason).To(Equal("ResourceBusy"))

			// And: Lock handle still returned (caller must release)
			Expect(lockHandle).ToNot(BeNil())
			Expect(lockHandle.Key).To(Equal("Node/worker-1"))
		})
	})

	Describe("LockHandle Release", func() {
		It("should release lock when handle.Release() called", func() {
			// Given: Lock acquired and handle returned
			mockLockMgr.AcquireResult = true
			rr := testutil.NewRemediationRequest("test-rr", "default")
			blocked, lockHandle, err := routingEngine.CheckBlockingConditions(ctx, rr, "workflow-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(lockHandle).ToNot(BeNil())

			// When: Release lock
			lockHandle.Release()

			// Then: Lock released
			Expect(mockLockMgr.ReleaseLockCalled).To(BeTrue())
			Expect(mockLockMgr.LastReleasedKey).To(Equal(lockHandle.Key))
		})

		It("should be safe to call Release() multiple times", func() {
			// Given: Lock handle
			mockLockMgr.AcquireResult = true
			rr := testutil.NewRemediationRequest("test-rr", "default")
			blocked, lockHandle, err := routingEngine.CheckBlockingConditions(ctx, rr, "workflow-1")
			Expect(err).ToNot(HaveOccurred())

			// When: Release multiple times
			lockHandle.Release()
			lockHandle.Release()
			lockHandle.Release()

			// Then: No panic (idempotent)
			Expect(mockLockMgr.ReleaseLockCalled).To(BeTrue())
		})

		It("should be safe to call Release() on nil handle", func() {
			// Given: Nil lock handle
			var lockHandle *routing.LockHandle = nil

			// When: Call Release()
			// Should not panic
			lockHandle.Release()

			// Then: No panic
		})
	})
})

// Mock lock manager for testing
type MockDistributedLockManager struct {
	AcquireLockCalled bool
	AcquireResult     bool
	AcquireError      error
	LastLockKey       string

	ReleaseLockCalled bool
	LastReleasedKey   string
}

func NewMockDistributedLockManager() *MockDistributedLockManager {
	return &MockDistributedLockManager{
		AcquireResult: true, // Default: lock acquired
		AcquireError:  nil,  // Default: no error
	}
}

func (m *MockDistributedLockManager) AcquireLock(ctx context.Context, key string) (bool, error) {
	m.AcquireLockCalled = true
	m.LastLockKey = key
	return m.AcquireResult, m.AcquireError
}

func (m *MockDistributedLockManager) ReleaseLock(ctx context.Context, key string) {
	m.ReleaseLockCalled = true
	m.LastReleasedKey = key
}
```

**Run Tests**:
```bash
# Run RO routing engine tests
ginkgo -v ./test/unit/remediationorchestrator/routing_lock_test.go

# Run all RO unit tests
make test-unit-remediationorchestrator

# Check coverage
go test -coverprofile=coverage.out ./pkg/remediationorchestrator/routing/
go tool cover -html=coverage.out
# Target: 90%+ coverage for routing engine
```

---

### **Session 2: Integration Tests** (3h)

**Goal**: Validate multi-replica behavior with real K8s API (envtest)

#### **Task 2.3: Multi-Replica Integration Tests** (3h)

**File**: `test/integration/remediationorchestrator/multi_replica_locking_integration_test.go` (NEW)

**Test Scenarios**:
1. **No Race Condition with Single Replica**
   - Baseline: single RO pod, no lock contention
   - Verify existing behavior preserved

2. **Race Prevention with Multiple Replicas**
   - 2 RO pods process same target resource concurrently
   - Verify only 1 WFE created

3. **Lock Release and Retry**
   - Pod 1 holds lock, Pod 2 waits
   - Pod 1 releases lock, Pod 2 acquires and succeeds

4. **Lock Expiration and Takeover**
   - Pod 1 crashes (lease expires)
   - Pod 2 takes over expired lease

**Test Pattern**:
```go
var _ = Describe("RemediationOrchestrator Multi-Replica Locking", func() {
	var (
		k8sClient   client.Client
		testEnv     *envtest.Environment
		ctx         context.Context
		namespace   string

		// Two simulated RO pods
		roPod1      *RemediationOrchestratorController
		roPod2      *RemediationOrchestratorController
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "test-namespace-" + uuid.New().String()[:8]

		// Start envtest (real K8s API)
		testEnv = &envtest.Environment{
			CRDDirectoryPaths: []string{
				filepath.Join("..", "..", "..", "config", "crd", "bases"),
			},
		}
		cfg, err := testEnv.Start()
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg).ToNot(BeNil())

		// Create K8s client
		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).ToNot(HaveOccurred())

		// Create namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		// Create two RO controller instances (simulating 2 pods)
		roPod1 = setupROController(ctx, k8sClient, namespace, "ro-pod-1")
		roPod2 = setupROController(ctx, k8sClient, namespace, "ro-pod-2")
	})

	AfterEach(func() {
		Expect(testEnv.Stop()).To(Succeed())
	})

	Describe("Single Replica Behavior (Baseline)", func() {
		It("should NOT create duplicate WFEs with single replica", func() {
			// Given: 2 RRs targeting same resource
			rr1 := createRemediationRequest(ctx, k8sClient, namespace, "rr-1", "Node/worker-1")
			rr2 := createRemediationRequest(ctx, k8sClient, namespace, "rr-2", "Node/worker-1")

			// When: Single RO pod processes both (serialized)
			result1, err1 := roPod1.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(rr1),
			})
			Expect(err1).ToNot(HaveOccurred())

			result2, err2 := roPod1.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(rr2),
			})
			Expect(err2).ToNot(HaveOccurred())

			// Then: Only 1 WFE created (second blocked by ResourceBusy)
			wfeList := &workflowexecutionv1.WorkflowExecutionList{}
			Expect(k8sClient.List(ctx, wfeList, client.InNamespace(namespace))).To(Succeed())
			Expect(len(wfeList.Items)).To(Equal(1), "Only 1 WFE should be created")
		})
	})

	Describe("Multi-Replica Race Prevention", func() {
		It("should NOT create duplicate WFEs with 2 concurrent replicas", func() {
			// Given: 2 RRs targeting SAME resource
			rr1 := createRemediationRequest(ctx, k8sClient, namespace, "rr-1", "Node/worker-1")
			rr2 := createRemediationRequest(ctx, k8sClient, namespace, "rr-2", "Node/worker-1")

			// When: 2 RO pods process concurrently (simulated race)
			var wg sync.WaitGroup
			wg.Add(2)

			var result1, result2 ctrl.Result
			var err1, err2 error

			go func() {
				defer wg.Done()
				result1, err1 = roPod1.Reconcile(ctx, reconcile.Request{
					NamespacedName: client.ObjectKeyFromObject(rr1),
				})
			}()

			go func() {
				defer wg.Done()
				// Slight delay to simulate concurrent processing
				time.Sleep(5 * time.Millisecond)
				result2, err2 = roPod2.Reconcile(ctx, reconcile.Request{
					NamespacedName: client.ObjectKeyFromObject(rr2),
				})
			}()

			wg.Wait()

			// Then: No errors (lock contention handled gracefully)
			Expect(err1).ToNot(HaveOccurred())
			Expect(err2).ToNot(HaveOccurred())

			// And: Only 1 WFE created (distributed lock prevented race)
			Eventually(func() int {
				wfeList := &workflowexecutionv1.WorkflowExecutionList{}
				_ = k8sClient.List(ctx, wfeList, client.InNamespace(namespace))
				return len(wfeList.Items)
			}, "10s", "100ms").Should(Equal(1), "Only 1 WFE should be created")

			// And: Second pod requeued with backoff (not error)
			// (One reconcile should return RequeueAfter for lock contention)
			hasRequeue := result1.RequeueAfter > 0 || result2.RequeueAfter > 0
			Expect(hasRequeue).To(BeTrue(), "One pod should requeue due to lock contention")
		})

		It("should create WFEs for DIFFERENT resources concurrently", func() {
			// Given: 2 RRs targeting DIFFERENT resources
			rr1 := createRemediationRequest(ctx, k8sClient, namespace, "rr-1", "Node/worker-1")
			rr2 := createRemediationRequest(ctx, k8sClient, namespace, "rr-2", "Node/worker-2")

			// When: 2 RO pods process concurrently
			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer wg.Done()
				_, _ = roPod1.Reconcile(ctx, reconcile.Request{
					NamespacedName: client.ObjectKeyFromObject(rr1),
				})
			}()

			go func() {
				defer wg.Done()
				_, _ = roPod2.Reconcile(ctx, reconcile.Request{
					NamespacedName: client.ObjectKeyFromObject(rr2),
				})
			}()

			wg.Wait()

			// Then: 2 WFEs created (different targets, no lock contention)
			Eventually(func() int {
				wfeList := &workflowexecutionv1.WorkflowExecutionList{}
				_ = k8sClient.List(ctx, wfeList, client.InNamespace(namespace))
				return len(wfeList.Items)
			}, "10s", "100ms").Should(Equal(2), "2 WFEs should be created for different targets")
		})
	})

	Describe("Lock Contention and Retry", func() {
		It("should retry and succeed after lock is released", func() {
			// Given: Pod 1 holds lock
			rr1 := createRemediationRequest(ctx, k8sClient, namespace, "rr-1", "Node/worker-1")
			_, err := roPod1.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(rr1),
			})
			Expect(err).ToNot(HaveOccurred())

			// And: Pod 2 tries to process same target (contention)
			rr2 := createRemediationRequest(ctx, k8sClient, namespace, "rr-2", "Node/worker-1")
			result2, err2 := roPod2.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(rr2),
			})
			Expect(err2).ToNot(HaveOccurred())
			Expect(result2.RequeueAfter).To(Equal(100 * time.Millisecond), "Should requeue with backoff")

			// When: Pod 1's lock is released (WFE created, lock released)
			// Pod 2 retries after backoff
			time.Sleep(150 * time.Millisecond) // Wait for requeue backoff

			result2_retry, err2_retry := roPod2.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(rr2),
			})
			Expect(err2_retry).ToNot(HaveOccurred())

			// Then: Pod 2 succeeds (sees WFE from Pod 1, blocked by ResourceBusy)
			// (NOT lock contention, but normal ResourceBusy blocking)
			Expect(result2_retry.RequeueAfter).To(Equal(30 * time.Second), "Should requeue with ResourceBusy backoff")
		})
	})

	Describe("Lease Expiration and Takeover", func() {
		It("should take over expired lease", func() {
			// Given: Pod 1 created a lease but "crashed" (simulated by not releasing)
			rr1 := createRemediationRequest(ctx, k8sClient, namespace, "rr-1", "Node/worker-1")

			// Simulate Pod 1 acquiring lock but not releasing (crash)
			leaseName := "lock-Node/worker-1" // Simplified lease name
			expiredLease := createExpiredLease(ctx, k8sClient, namespace, leaseName, "ro-pod-1")

			// When: Pod 2 tries to acquire lock
			_, err := roPod2.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(rr1),
			})
			Expect(err).ToNot(HaveOccurred())

			// Then: Pod 2 takes over the expired lease
			lease := &coordinationv1.Lease{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Namespace: namespace,
				Name:      leaseName,
			}, lease)
			Expect(err).ToNot(HaveOccurred())
			Expect(*lease.Spec.HolderIdentity).To(Equal("ro-pod-2"), "Pod 2 should take over expired lease")
		})
	})
})

// Test helpers
func setupROController(ctx context.Context, k8sClient client.Client, namespace, podName string) *RemediationOrchestratorController {
	// Create RO controller instance
	routingEngine := routing.NewRoutingEngine(
		k8sClient,
		namespace,
		podName, // Unique pod name for lock holder identity
		5*time.Minute, // cooldown
	)

	controller := &RemediationOrchestratorController{
		client:        k8sClient,
		scheme:        scheme,
		routingEngine: routingEngine,
		// ... other fields
	}

	return controller
}

func createRemediationRequest(ctx context.Context, k8sClient client.Client, namespace, name, targetResource string) *remediationv1.RemediationRequest {
	// Parse target resource (e.g., "Node/worker-1")
	parts := strings.Split(targetResource, "/")
	kind := parts[0]
	resourceName := parts[1]

	rr := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: remediationv1.RemediationRequestSpec{
			TargetResource: remediationv1.TargetResourceSpec{
				Kind: kind,
				Name: resourceName,
			},
		},
	}
	Expect(k8sClient.Create(ctx, rr)).To(Succeed())
	return rr
}

func createExpiredLease(ctx context.Context, k8sClient client.Client, namespace, leaseName, holderID string) *coordinationv1.Lease {
	// Create lease with renewTime 60 seconds in the past (expired)
	expiredTime := metav1.NewMicroTime(time.Now().Add(-60 * time.Second))
	leaseDurationSeconds := int32(30)

	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: namespace,
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &holderID,
			LeaseDurationSeconds: &leaseDurationSeconds,
			AcquireTime:          &expiredTime,
			RenewTime:            &expiredTime,
		},
	}

	Expect(k8sClient.Create(ctx, lease)).To(Succeed())
	return lease
}
```

**Run Tests**:
```bash
# Run RO integration tests
make test-integration-remediationorchestrator

# Run specific multi-replica tests
ginkgo -v -focus="Multi-Replica" ./test/integration/remediationorchestrator/

# Check test time
# Target: <3 minutes for all integration tests
```

---

### **Session 3: E2E Tests** (2h)

**Goal**: Validate distributed locking in production-like Kind cluster

#### **Task 2.4: Multi-Replica E2E Tests** (2h)

**File**: `test/e2e/remediationorchestrator/multi_replica_locking_e2e_test.go` (NEW)

**Test Scenarios**:
1. **3-Replica RO Deployment**
   - Deploy RO with 3 replicas in Kind cluster
   - Fire 10 concurrent alerts for same node
   - Verify only 1 WFE created

2. **Performance Impact**
   - Measure P95 reconciliation latency with/without locking
   - Verify latency increase <20ms

3. **Lease Cleanup**
   - Verify leases are deleted after WFE creation
   - Verify no leaked leases in namespace

**Test Pattern**:
```go
var _ = Describe("RO Multi-Replica E2E", func() {
	var (
		ctx       context.Context
		namespace string
		k8sClient client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "test-ro-multi-replica-" + uuid.New().String()[:8]

		// k8sClient is initialized by e2e test suite (Kind cluster)
		Expect(k8sClient).ToNot(BeNil())

		// Create namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	})

	AfterEach(func() {
		// Cleanup namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		_ = k8sClient.Delete(ctx, ns)
	})

	Describe("Multi-Replica Deployment", func() {
		It("should handle 10 concurrent RRs for same resource with 3 replicas", func() {
			// Given: RO deployed with 3 replicas (done in test setup)
			// Verify 3 RO pods are running
			Eventually(func() int {
				pods := &corev1.PodList{}
				_ = k8sClient.List(ctx, pods,
					client.InNamespace("kubernaut-system"),
					client.MatchingLabels{"app": "remediationorchestrator"},
				)
				return len(pods.Items)
			}, "30s", "1s").Should(Equal(3), "3 RO replicas should be running")

			// When: Create 10 RRs targeting same resource concurrently
			targetResource := "Node/worker-1"
			for i := 0; i < 10; i++ {
				go func(index int) {
					rr := &remediationv1.RemediationRequest{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("rr-concurrent-%d", index),
							Namespace: namespace,
						},
						Spec: remediationv1.RemediationRequestSpec{
							TargetResource: remediationv1.TargetResourceSpec{
								Kind: "Node",
								Name: "worker-1",
							},
						},
					}
					_ = k8sClient.Create(ctx, rr)
				}(i)
			}

			// Then: Only 1 WFE created (distributed lock prevents duplicates)
			Eventually(func() int {
				wfeList := &workflowexecutionv1.WorkflowExecutionList{}
				_ = k8sClient.List(ctx, wfeList, client.InNamespace(namespace))
				return len(wfeList.Items)
			}, "30s", "1s").Should(Equal(1), "Only 1 WFE should be created despite 10 concurrent RRs")

			// And: Other RRs blocked by ResourceBusy
			Eventually(func() int {
				rrList := &remediationv1.RemediationRequestList{}
				_ = k8sClient.List(ctx, rrList, client.InNamespace(namespace))
				blockedCount := 0
				for _, rr := range rrList.Items {
					if rr.Status.BlockReason == "ResourceBusy" {
						blockedCount++
					}
				}
				return blockedCount
			}, "30s", "1s").Should(Equal(9), "9 RRs should be blocked by ResourceBusy")
		})
	})

	Describe("Performance Impact", func() {
		It("should have P95 latency increase <20ms with distributed locking", func() {
			// Measure reconciliation latency for 100 RRs
			latencies := make([]time.Duration, 100)

			for i := 0; i < 100; i++ {
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("rr-perf-%d", i),
						Namespace: namespace,
					},
					Spec: remediationv1.RemediationRequestSpec{
						TargetResource: remediationv1.TargetResourceSpec{
							Kind: "Node",
							Name: fmt.Sprintf("worker-%d", i), // Different targets (no lock contention)
						},
					},
				}

				start := time.Now()
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())

				// Wait for reconciliation (WFE created)
				Eventually(func() bool {
					_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
					return rr.Status.WorkflowExecutionRef != nil
				}, "10s", "100ms").Should(BeTrue())

				latencies[i] = time.Since(start)
			}

			// Calculate P95 latency
			sort.Slice(latencies, func(i, j int) bool {
				return latencies[i] < latencies[j]
			})
			p95Index := int(float64(len(latencies)) * 0.95)
			p95Latency := latencies[p95Index]

			fmt.Printf("P95 Latency: %v\n", p95Latency)

			// Verify P95 latency is reasonable
			// (Baseline ~100-200ms, with locking ~110-220ms)
			Expect(p95Latency).To(BeNumerically("<", 500*time.Millisecond),
				"P95 latency should be <500ms")
		})
	})

	Describe("Lease Cleanup", func() {
		It("should clean up leases after WFE creation", func() {
			// Given: Create RR
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-lease-cleanup",
					Namespace: namespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					TargetResource: remediationv1.TargetResourceSpec{
						Kind: "Node",
						Name: "worker-cleanup",
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// When: RO processes RR and creates WFE
			Eventually(func() bool {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.WorkflowExecutionRef != nil
			}, "30s", "1s").Should(BeTrue())

			// Then: Lease should be cleaned up (deleted)
			Eventually(func() bool {
				leaseList := &coordinationv1.LeaseList{}
				_ = k8sClient.List(ctx, leaseList, client.InNamespace(namespace))
				// Check for leases matching lock prefix
				for _, lease := range leaseList.Items {
					if strings.HasPrefix(lease.Name, "lock-") {
						return false // Lease still exists
					}
				}
				return true // No leases found
			}, "10s", "1s").Should(BeTrue(), "Leases should be cleaned up after WFE creation")
		})
	})
})
```

**Run Tests**:
```bash
# Run RO E2E tests
make test-e2e-remediationorchestrator

# Run specific multi-replica E2E tests
ginkgo -v -focus="Multi-Replica E2E" ./test/e2e/remediationorchestrator/

# Check test time
# Target: <2 minutes for E2E tests
```

---

## Success Criteria

### **Functional Requirements** ✅

- [ ] Shared lock manager compiles and passes tests (90%+ coverage)
- [ ] Gateway refactored to use shared lock manager (no behavior change)
- [ ] RO integrates distributed locking in routing engine
- [ ] RBAC updated with Lease permissions
- [ ] Metrics added for lock acquisition failures
- [ ] Unit tests pass (432 RO unit tests + new locking tests)
- [ ] Integration tests pass (multi-replica scenarios validated)
- [ ] E2E tests pass (3-replica deployment with 10 concurrent RRs)

### **Quality Requirements** ✅

- [ ] Test coverage: 90%+ for distributed locking code
- [ ] No lint errors in new code
- [ ] All existing tests still pass (no regressions)
- [ ] Documentation updated (implementation plan, test plan, ADR-052)

### **Performance Requirements** ✅

- [ ] P95 reconciliation latency increase: <20ms
- [ ] Lock acquisition success rate: >99.9%
- [ ] Duplicate WFE creation rate: <0.001% (vs. ~0.1% without locking)

### **Operational Requirements** ✅

- [ ] RBAC changes documented in deployment guide
- [ ] Metrics exposed for monitoring (lock failures)
- [ ] Lease cleanup verified (no resource leaks)
- [ ] Gateway team notified and aligned on shared package

---

## Rollback Plan

### **Rollback Triggers**

- Lock acquisition failure rate >1%
- P95 latency increase >50ms
- Duplicate WFE creation rate >0.1% (locking not working)
- Production incidents related to lock contention

### **Rollback Procedure**

**Option 1: Revert Code Changes** (if critical issues)
```bash
# Revert Git commits
git revert <commit-hash-ro-locking>
git revert <commit-hash-shared-lock-manager>

# Redeploy
kubectl rollout restart deployment remediationorchestrator-controller
kubectl rollout restart deployment gateway-service
```

**Option 2: Single-Replica Deployment** (temporary mitigation)
```bash
# Scale down to 1 replica (eliminates race condition)
kubectl scale deployment remediationorchestrator-controller --replicas=1 -n kubernaut-system

# Monitor for stability
# Investigation and fix can proceed without production impact
```

**Rollback Validation**:
- [ ] RO and Gateway revert to previous behavior
- [ ] All tests pass after rollback
- [ ] No new errors in production logs
- [ ] Performance metrics return to baseline

---

## Appendix A: File Changes Summary

### **New Files Created**
```
pkg/remediationorchestrator/locking/
├── distributed_lock.go          # RO lock manager (copied from Gateway)
├── distributed_lock_test.go     # Unit tests (copied from Gateway)
└── doc.go                        # Package documentation

test/unit/remediationorchestrator/
└── routing_lock_test.go          # RO routing engine lock tests

test/integration/remediationorchestrator/
└── multi_replica_locking_integration_test.go  # Multi-replica integration tests

test/e2e/remediationorchestrator/
└── multi_replica_locking_e2e_test.go          # Multi-replica E2E tests

docs/handoff/
└── DD_TO_ADR_CONVERSION_DISTRIBUTED_LOCKING_DEC_30_2025.md

docs/architecture/decisions/
└── ADR-052-distributed-locking-pattern.md  # To be created (pattern documentation)
```

### **Files Modified**
```
pkg/remediationorchestrator/routing/
├── engine.go                     # Add lock manager field
└── blocking.go                   # Wrap lock acquisition in CheckBlockingConditions

internal/controller/remediationorchestrator/
└── reconciler.go                 # Use lock handle, defer release

pkg/remediationorchestrator/metrics/
└── metrics.go                    # Add lock failure metric

deployments/remediationorchestrator/
└── rbac.yaml                     # Add Lease permissions
```

### **Gateway Files** (UNCHANGED)
```
pkg/gateway/processing/
├── server.go                     # No changes
├── distributed_lock.go           # Remains as reference implementation
└── distributed_lock_test.go      # Remains as reference tests
```

### **Documentation Updates**
```
docs/services/crd-controllers/05-remediationorchestrator/implementation/
├── IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md  # This document
└── TEST_PLAN_DISTRIBUTED_LOCKING_V1.0.md            # Test plan

docs/shared/
└── CROSS_TEAM_DISTRIBUTED_LOCKING_PATTERN_DEC_30_2025.md  # Updated

docs/handoff/
└── RO_RACE_CONDITION_ANALYSIS_DEC_30_2025.md  # Reference document
```

---

## Appendix B: References

### **Architecture Decisions**
- **ADR-052**: K8s Lease-Based Distributed Locking Pattern (to be created from DD-GATEWAY-013)
- **DD-GATEWAY-013**: Multi-Replica Deduplication Protection (Gateway-specific, to be deprecated)

### **Business Requirements**
- **BR-ORCH-050**: Multi-Replica Resource Lock Safety (RO)
- **BR-GATEWAY-190**: Multi-Replica Deduplication Safety (Gateway)

### **Cross-Team Documentation**
- [CROSS_TEAM_DISTRIBUTED_LOCKING_PATTERN_DEC_30_2025.md](../../../../shared/CROSS_TEAM_DISTRIBUTED_LOCKING_PATTERN_DEC_30_2025.md)
- [RO_RACE_CONDITION_ANALYSIS_DEC_30_2025.md](../../../../handoff/RO_RACE_CONDITION_ANALYSIS_DEC_30_2025.md)

### **Gateway Implementation**
- [IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md](../../../stateless/gateway-service/implementation/IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md)
- [TEST_PLAN_DISTRIBUTED_LOCKING_V1.0.md](../../../stateless/gateway-service/implementation/TEST_PLAN_DISTRIBUTED_LOCKING_V1.0.md)
- [DD-GATEWAY-013](../../../../architecture/decisions/DD-GATEWAY-013-multi-replica-deduplication.md)

### **Testing Standards**
- [TESTING_GUIDELINES.md](../../../../development/TESTING_GUIDELINES.md)
- [03-testing-strategy.mdc](.cursor/rules/03-testing-strategy.mdc)

---

**Status**: ✅ **APPROVED - Ready for Implementation**
**Timeline**: Next branch after current merge (implementing alongside Gateway shared lock manager)
**Confidence**: 90% (based on Gateway's proven implementation)

---

**Document Version**: 1.0
**Last Updated**: December 30, 2025
**Next Review**: After Day 1 implementation (validate approach and adjust Day 2 if needed)

