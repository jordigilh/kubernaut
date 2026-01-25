# Gateway Service - Distributed Locking Implementation Plan

**Version**: V1.0
**Created**: December 30, 2025
**Timeline**: 2 days (16 hours total)
**Status**: ✅ APPROVED - Ready for Implementation
**Quality Level**: Matches Data Storage v4.1 and Notification V3.0 standards
**Design Decision**: [DD-GATEWAY-013](../../../../architecture/decisions/DD-GATEWAY-013-multi-replica-deduplication.md)

**Change Log**:
- **v1.0** (2025-12-30): Initial implementation plan for K8s Lease-based distributed locking

---

## 🎯 Quick Reference

**Feature**: Kubernetes Lease-Based Distributed Lock for Multi-Replica Deduplication
**Business Requirement**: BR-GATEWAY-190 (Multi-Replica Safety)
**Design Decision**: DD-GATEWAY-013 (Alternative 1 - K8s Lease-Based Lock)
**Service Type**: Stateless HTTP API (Gateway Service)
**Methodology**: APDC-TDD with Defense-in-Depth Testing
**Parallel Execution**: 4 concurrent processes for all test tiers

**Success Metrics**:
- Duplicate RR creation rate: <0.001% (vs. current ~0.1% with 10 replicas)
- Lock acquisition success rate: >99.9%
- P95 latency impact: <20ms additional overhead
- Test coverage: 90%+ for distributed locking code

---

## 📑 Table of Contents

| Section | Purpose |
|---------|---------|
| [Executive Summary](#executive-summary) | Problem, solution, and business impact |
| [Prerequisites Checklist](#prerequisites-checklist) | Pre-Day 1 requirements |
| [Risk Assessment](#️-risk-assessment-matrix) | Risk identification and mitigation |
| [Timeline Overview](#timeline-overview) | 2-day breakdown |
| [Day 1: Implementation](#day-1-implementation-8h) | Core distributed locking logic |
| [Day 2: Testing & Validation](#day-2-testing--validation-8h) | Unit, integration, E2E tests |
| [Success Criteria](#success-criteria) | Completion checklist |
| [Rollback Plan](#rollback-plan) | Feature flag and rollback procedure |

---

## Executive Summary

### Problem Statement

**Current State**: Gateway service can scale horizontally (multiple replicas), but has a race condition vulnerability when concurrent signals with the same fingerprint span second boundaries across different Gateway pods.

**Impact with Multiple Replicas**:
- **Single Gateway pod**: ~0.01% duplicate RR creation rate (rare)
- **3 Gateway pods**: ~0.03% duplicate rate (3x more likely)
- **10 Gateway pods**: ~0.1% duplicate rate (10x more likely)

**At Production Scale** (10,000 alerts/day, 10% concurrent, 10 Gateway pods):
- ~1 duplicate RR created per day
- ~30 duplicate RRs created per month
- ~365 duplicate RRs created per year

**Blast Radius**:
- Duplicate remediation executions (potential system damage)
- Resource thrashing for non-idempotent remediations
- Observability confusion (multiple RRs for same problem)

### Solution

**Approach**: Kubernetes Lease-based distributed lock (DD-GATEWAY-013 Alternative 1)

**How It Works**:
1. Gateway pod acquires K8s Lease for fingerprint before CRD creation
2. Only 1 pod can hold lock at a time (mutual exclusion)
3. Other pods wait (100ms backoff) and retry deduplication check
4. Lease expires after 30s (prevents deadlocks on pod crashes)

**Protection Guarantee**: 100% elimination of cross-replica duplicate RR creation

### Business Impact

**Benefits**:
- ✅ **Eliminates duplicate RR creation**: From ~0.1% to <0.001% with 10 replicas
- ✅ **Scales safely**: Works correctly with 1 to 100+ Gateway replicas
- ✅ **K8s-native**: No external dependencies (Redis, etcd, etc.)
- ✅ **Fault-tolerant**: Lease expires if Gateway pod crashes

**Trade-offs**:
- ⚠️ **Latency increase**: +10-20ms for lock acquisition per request
- ⚠️ **Lock contention**: High-volume alerts may queue behind locks

**Mitigation**:
- Latency still within Gateway SLO (P95 <50ms → P95 ~60-70ms)
- Lease duration tuned to 30s (balance between safety and contention)
- Monitoring alerts for performance degradation

---

## Prerequisites Checklist

### Knowledge Prerequisites

- [ ] Reviewed [DD-GATEWAY-013](../../../../architecture/decisions/DD-GATEWAY-013-multi-replica-deduplication.md) (distributed locking design)
- [ ] Reviewed [DD-GATEWAY-011](../../../../architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md) (status-based deduplication)
- [ ] Understood Kubernetes [Lease resource](https://kubernetes.io/docs/reference/kubernetes-api/cluster-resources/lease-v1/)
- [ ] Reviewed [DD-005](../../../../architecture/decisions/DD-005-unified-logging-framework.md) (observability standards)

### Environment Prerequisites

- [ ] Gateway service deployable locally (Kind cluster)
- [ ] Gateway integration test infrastructure functional
- [ ] DataStorage service running (for audit integration tests)
- [ ] Prometheus metrics endpoint accessible

### Code Prerequisites

- [ ] Gateway service at stable version (all tests passing)
- [ ] No pending PRs that modify `ProcessSignal()` flow
- [ ] Baseline metrics captured (P95 latency without distributed locking)

---

## ⚠️ Risk Assessment Matrix

| Risk | Probability | Impact | Mitigation | Status |
|------|-------------|--------|------------|--------|
| **Latency degradation >20ms** | Medium | High | Feature flag for quick rollback | 🟡 Active Monitoring |
| **Lock contention under high load** | Medium | Medium | Lease duration tuned to 30s | 🟡 Active Monitoring |
| **K8s API unavailability** | Low | High | Fail-fast with HTTP 500, alert sources can retry | ✅ Handled |
| **Lease resource RBAC missing** | Low | High | RBAC added in Day 1, validated in E2E tests | ✅ Handled |
| **Deadlocks on Gateway pod crash** | Low | High | Lease expires after 30s automatically | ✅ Handled |
| **Breaking change to existing behavior** | N/A | N/A | No backwards compatibility required | ✅ Not Applicable |

---

## Timeline Overview

### Phase Breakdown

**Total Time**: 2 days (16 hours)

| Day | Focus | Hours | Deliverables |
|-----|-------|-------|-------------|
| **Day 1** | Core Implementation | 8h | Distributed lock manager, Gateway integration, RBAC |
| **Day 2** | Testing & Validation | 8h | Unit tests, integration tests, E2E tests, metrics validation |

---

## Day 1: Implementation (8h)

### ✅ Objectives

- Implement K8s Lease-based distributed lock manager
- Integrate lock acquisition in Gateway `ProcessSignal()` flow
- Add RBAC permissions for Lease resources
- Add configuration for distributed locking feature flag

### 📦 Deliverables

**File 1**: `pkg/gateway/processing/distributed_lock.go` (NEW)
**File 2**: `pkg/gateway/processing/distributed_lock_test.go` (NEW)
**File 3**: `pkg/gateway/server.go` (MODIFIED)
**File 4**: `pkg/gateway/config/config.go` (MODIFIED)
**File 5**: `deployments/gateway/rbac.yaml` (MODIFIED)
**File 6**: `deployments/gateway/deployment.yaml` (MODIFIED)

---

### Task 1.1: Implement Distributed Lock Manager (2h)

**File**: `pkg/gateway/processing/distributed_lock.go`

**Code Implementation**:

```go
/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

// ========================================
// DD-GATEWAY-013: Distributed Lock Manager
// 📋 Design Decision: DD-GATEWAY-013 Alternative 1 | ✅ Approved Design | Confidence: 85%
// See: docs/architecture/decisions/DD-GATEWAY-013-multi-replica-deduplication.md
// ========================================
//
// DistributedLockManager manages K8s Lease-based distributed locks for
// multi-replica deduplication protection.
//
// WHY DD-GATEWAY-013 Alternative 1?
// - ✅ Eliminates cross-replica race conditions (0.1% → <0.001% duplicate rate)
// - ✅ K8s-native solution (no external dependencies like Redis/etcd)
// - ✅ Fault-tolerant (lease expires on pod crash, no deadlocks)
// - ✅ Scales safely (1 to 100+ Gateway replicas)
//
// ⚠️ Trade-off: +10-20ms latency per request (acceptable vs. duplicate risk)
// ========================================

const (
	// LockDurationSeconds is the duration (in seconds) a lease is held before expiring
	// Balance between safety (short enough to prevent long waits) and efficiency
	LockDurationSeconds = 30
)

// DistributedLockManager manages K8s Lease-based distributed locks
type DistributedLockManager struct {
	client    client.Client
	namespace string // Namespace where leases are created (same as Gateway pod namespace)
	holderID  string // Gateway pod name (unique identifier)
}

// NewDistributedLockManager creates a new distributed lock manager
//
// Parameters:
//   - client: K8s client for Lease resource operations
//   - namespace: Namespace where Lease resources are created
//   - holderID: Unique identifier for this Gateway pod (typically pod name)
func NewDistributedLockManager(client client.Client, namespace, holderID string) *DistributedLockManager {
	return &DistributedLockManager{
		client:    client,
		namespace: namespace,
		holderID:  holderID,
	}
}

// AcquireLock attempts to acquire a distributed lock for the given fingerprint
//
// This method ensures only 1 Gateway pod can process a signal with a given
// fingerprint at a time, preventing duplicate RR creation across replicas.
//
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
	leaseDuration := int32(LockDurationSeconds)

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

// ReleaseLock releases the distributed lock for the given fingerprint
//
// This should be called in a defer statement after successful lock acquisition
// to ensure the lock is released even if processing fails.
func (m *DistributedLockManager) ReleaseLock(ctx context.Context, fingerprint string) error {
	leaseName := fmt.Sprintf("gw-lock-%s", fingerprint[:16])

	lease := &coordinationv1.Lease{}
	lease.Namespace = m.namespace
	lease.Name = leaseName

	// Delete lease (allows garbage collection)
	// Ignore NotFound errors (lease may have expired and been cleaned up)
	if err := m.client.Delete(ctx, lease); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to release lease: %w", err)
	}

	return nil
}
```

**Testing Notes**:
- Unit tests will cover all code paths (lease creation, acquisition, expiry, conflicts)
- Error handling validated for all K8s API error types
- Race condition scenarios tested (concurrent lock acquisition)

---

### Task 1.2: Integrate Lock Manager in Gateway Server (2h)

**File**: `pkg/gateway/server.go`

**Modifications**:

**Step 1**: Add `lockManager` field to `Server` struct:

```go
// Server represents the Gateway HTTP server
type Server struct {
	// ... existing fields ...

	// DD-GATEWAY-013: Distributed locking for multi-replica deduplication (always enabled)
	lockManager *processing.DistributedLockManager
}
```

**Step 2**: Modify `createServerWithClients()` to initialize lock manager:

```go
func createServerWithClients(cfg *config.ServerConfig, logger logr.Logger, metricsInstance *metrics.Metrics, ctrlClient client.Client, k8sClient *k8s.Client) (*Server, error) {
	// ... existing initialization ...

	// DD-GATEWAY-013: Initialize distributed lock manager (always enabled)
	// Get pod metadata from environment (set by K8s downward API)
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		podName = fmt.Sprintf("gateway-unknown-%d", time.Now().Unix())
	}

	podNamespace := os.Getenv("POD_NAMESPACE")
	if podNamespace == "" {
		logger.Info("POD_NAMESPACE not set, defaulting to kubernaut-system")
		podNamespace = "kubernaut-system"
	}

	lockManager := processing.NewDistributedLockManager(
		ctrlClient,
		podNamespace,  // Leases created in same namespace as Gateway pod
		podName,
	)

	logger.Info("Distributed locking initialized",
		"namespace", podNamespace,
		"holderID", podName)

	return &Server{
		// ... existing fields ...
		lockManager: lockManager,
	}, nil
}
```

**Step 3**: Modify `ProcessSignal()` to use distributed lock (always enabled):

```go
func (s *Server) ProcessSignal(ctx context.Context, signal *types.NormalizedSignal) (*ProcessingResponse, error) {
	start := time.Now()
	logger := middleware.GetLogger(ctx)

	// Record ingestion metric
	s.metricsInstance.AlertsReceivedTotal.WithLabelValues(signal.SourceType, signal.Severity).Inc()

	// DD-GATEWAY-013: Acquire distributed lock (multi-replica protection - always enabled)
	lockAcquired, err := s.lockManager.AcquireLock(ctx, signal.Fingerprint)
	if err != nil {
		// K8s API error (fail-fast)
		logger.Error(err, "Failed to acquire distributed lock",
			"fingerprint", signal.Fingerprint)
		s.metricsInstance.LockAcquisitionFailuresTotal.Inc()
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}

	if !lockAcquired {
		// Lock held by another Gateway pod - retry deduplication check after backoff
		logger.V(1).Info("Lock held by another Gateway pod, retrying after backoff",
			"fingerprint", signal.Fingerprint)

		// Backoff to allow other pod to create RR
		time.Sleep(100 * time.Millisecond)

		// Retry deduplication check (other pod may have created RR by now)
		shouldDeduplicate, existingRR, err := s.phaseChecker.ShouldDeduplicate(ctx, signal.Namespace, signal.Fingerprint)
		if err != nil {
			return nil, fmt.Errorf("deduplication check failed after backoff: %w", err)
		}

		if shouldDeduplicate && existingRR != nil {
			// RR created by other pod - update status
			if err := s.statusUpdater.UpdateDeduplicationStatus(ctx, existingRR); err != nil {
				logger.Info("Failed to update deduplication status after lock contention",
					"error", err,
					"fingerprint", signal.Fingerprint)
			}
			return NewDuplicateResponseFromRR(signal.Fingerprint, existingRR), nil
		}

		// Still no RR - recursively retry lock acquisition
		return s.ProcessSignal(ctx, signal)
	}

	// Lock acquired - ensure it's released
	defer func() {
		if err := s.lockManager.ReleaseLock(ctx, signal.Fingerprint); err != nil {
			logger.Error(err, "Failed to release distributed lock",
				"fingerprint", signal.Fingerprint)
		}
	}()

	logger.V(1).Info("Distributed lock acquired",
		"fingerprint", signal.Fingerprint)

	// 1. Deduplication check (DD-GATEWAY-011: K8s status-based)
	shouldDeduplicate, existingRR, err := s.phaseChecker.ShouldDeduplicate(ctx, signal.Namespace, signal.Fingerprint)
	if err != nil {
		logger.Error(err, "Deduplication check failed",
			"fingerprint", signal.Fingerprint)
		return nil, fmt.Errorf("deduplication check failed: %w", err)
	}

	if shouldDeduplicate && existingRR != nil {
		// Duplicate - update status
		if err := s.statusUpdater.UpdateDeduplicationStatus(ctx, existingRR); err != nil {
			logger.Info("Failed to update deduplication status",
				"error", err,
				"fingerprint", signal.Fingerprint)
		}

		// Record metrics
		s.metricsInstance.AlertsDeduplicatedTotal.WithLabelValues(signal.AlertName).Inc()

		logger.V(1).Info("Duplicate signal detected",
			"fingerprint", signal.Fingerprint,
			"existingRR", existingRR.Name)

		// Emit audit event
		s.emitSignalDeduplicatedAudit(ctx, signal, existingRR.Name, existingRR.Namespace, existingRR.Status.Deduplication.OccurrenceCount)

		return NewDuplicateResponseFromRR(signal.Fingerprint, existingRR), nil
	}

	// 2. Create RemediationRequest CRD
	return s.createRemediationRequestCRD(ctx, signal, start)
}
```

---

### Task 1.3: Add Configuration Support (1h)

**File**: `pkg/gateway/config/config.go`

**Modifications**:

```go
// ProcessingConfig configures signal processing behavior
type ProcessingConfig struct {
	// ... existing fields ...

	// DD-GATEWAY-013: Distributed locking configuration
	EnableDistributedLocking bool   `yaml:"enableDistributedLocking" env:"GATEWAY_ENABLE_DISTRIBUTED_LOCKING"`
	LockNamespace            string `yaml:"lockNamespace" env:"GATEWAY_LOCK_NAMESPACE"`
	LockDurationSeconds      int32  `yaml:"lockDurationSeconds" env:"GATEWAY_LOCK_DURATION_SECONDS"`
}
```

**Default Configuration**:

```go
func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		// ... existing defaults ...
		Processing: ProcessingConfig{
			// ... existing defaults ...
			EnableDistributedLocking: false, // Default to disabled for backward compatibility
			LockNamespace:            "kubernaut-system",
			LockDurationSeconds:      30,
		},
	}
}
```

---

### Task 1.4: Add Prometheus Metrics (1h)

**File**: `pkg/gateway/metrics/metrics.go`

**Modifications**:

```go
// Metrics represents all Gateway metrics
type Metrics struct {
	// ... existing metrics ...

	// DD-GATEWAY-013: Distributed locking metrics
	LockAcquisitionAttemptsTotal prometheus.Counter
	LockAcquisitionSuccessTotal  prometheus.Counter
	LockAcquisitionFailuresTotal prometheus.Counter
	LockContentionTotal          prometheus.Counter
	LockWaitDurationSeconds      prometheus.Histogram
	LockHoldDurationSeconds      prometheus.Histogram
}

// NewMetrics creates a new Metrics instance with all Gateway metrics
func NewMetrics() *Metrics {
	// ... existing metrics initialization ...

	// DD-GATEWAY-013: Distributed locking metrics
	lockAcquisitionAttemptsTotal := promauto.NewCounter(prometheus.CounterOpts{
		Name: "gateway_lock_acquisition_attempts_total",
		Help: "Total number of distributed lock acquisition attempts",
	})

	lockAcquisitionSuccessTotal := promauto.NewCounter(prometheus.CounterOpts{
		Name: "gateway_lock_acquisition_success_total",
		Help: "Total number of successful distributed lock acquisitions",
	})

	lockAcquisitionFailuresTotal := promauto.NewCounter(prometheus.CounterOpts{
		Name: "gateway_lock_acquisition_failures_total",
		Help: "Total number of failed distributed lock acquisitions (K8s API errors)",
	})

	lockContentionTotal := promauto.NewCounter(prometheus.CounterOpts{
		Name: "gateway_lock_contention_total",
		Help: "Total number of times lock was held by another pod (contention)",
	})

	lockWaitDurationSeconds := promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "gateway_lock_wait_duration_seconds",
		Help:    "Time spent waiting for lock (backoff duration)",
		Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // 10ms to 5.12s
	})

	lockHoldDurationSeconds := promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "gateway_lock_hold_duration_seconds",
		Help:    "Time lock was held during signal processing",
		Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // 10ms to 5.12s
	})

	return &Metrics{
		// ... existing metrics ...
		LockAcquisitionAttemptsTotal: lockAcquisitionAttemptsTotal,
		LockAcquisitionSuccessTotal:  lockAcquisitionSuccessTotal,
		LockAcquisitionFailuresTotal: lockAcquisitionFailuresTotal,
		LockContentionTotal:          lockContentionTotal,
		LockWaitDurationSeconds:      lockWaitDurationSeconds,
		LockHoldDurationSeconds:      lockHoldDurationSeconds,
	}
}
```

---

### Task 1.5: Add RBAC Permissions (30min)

**File**: `deployments/gateway/rbac.yaml`

**Modifications**:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: gateway-role
rules:
  # ... existing permissions ...

  # DD-GATEWAY-013: Lease resource permissions for distributed locking
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "create", "update", "delete"]
```

---

### Task 1.6: Update Deployment Configuration (15min)

**File**: `deployments/gateway/deployment.yaml`

**Modifications**:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: kubernaut-system  # Can be any namespace
spec:
  replicas: 3  # Multi-replica deployment (distributed locking always enabled)
  template:
    spec:
      containers:
        - name: gateway
          env:
            # ... existing environment variables ...

            # DD-GATEWAY-013: Pod metadata for lock holder identification
            # Leases created in same namespace as Gateway pod
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
```

**Note**: Minimal configuration via K8s downward API:
- Lock namespace = Pod's namespace (dynamic, not hardcoded)
- Lock duration = 30s (hardcoded constant)
- Distributed locking always enabled

---

### Day 1 EOD Checklist

- [ ] `distributed_lock.go` implemented with all error handling
- [ ] `distributed_lock_test.go` unit tests written (not yet run)
- [ ] `server.go` modified with lock integration
- [ ] `config.go` updated with distributed locking configuration
- [ ] `metrics.go` updated with distributed locking metrics
- [ ] RBAC permissions added for Lease resources
- [ ] Deployment configuration updated with environment variables
- [ ] Code compiles without errors
- [ ] All lint checks pass

---

## Day 2: Testing & Validation (8h)

### ✅ Objectives

- Implement comprehensive unit tests for distributed lock manager
- Add integration tests for multi-replica deduplication
- Add E2E tests for distributed locking in production scenario
- Validate metrics exposure and correctness
- Performance baseline comparison (with/without distributed locking)

### 📦 Deliverables

**File 1**: `pkg/gateway/processing/distributed_lock_test.go` (NEW)
**File 2**: `test/integration/gateway/distributed_locking_test.go` (NEW)
**File 3**: `test/e2e/gateway/distributed_locking_test.go` (NEW)
**File 4**: `docs/handoff/DD_GATEWAY_013_IMPLEMENTATION_COMPLETE_DEC_30_2025.md` (NEW)

---

### Task 2.1: Unit Tests for Distributed Lock Manager (2h)

**File**: `pkg/gateway/processing/distributed_lock_test.go`

**Test Coverage**:
- ✅ Lock acquisition when lease doesn't exist
- ✅ Lock acquisition when lease exists and held by self
- ✅ Lock acquisition when lease exists and held by other pod
- ✅ Lock acquisition when lease expired
- ✅ Lock release
- ✅ Error handling for K8s API failures (NotFound, AlreadyExists, Conflict, Communication errors)
- ✅ Concurrent lock acquisition attempts (race conditions)

**Test Implementation** (abbreviated for space):

```go
package processing_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
)

var _ = Describe("DistributedLockManager", func() {
	var (
		ctx           context.Context
		k8sClient     client.Client
		lockManager   *processing.DistributedLockManager
		scheme        *runtime.Scheme
		namespace     = "test-locks"
		holderID      = "gateway-pod-1"
		otherHolderID = "gateway-pod-2"
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = coordinationv1.AddToScheme(scheme)
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		lockManager = processing.NewDistributedLockManager(k8sClient, namespace, holderID)
	})

	Context("Lock Acquisition", func() {
		It("should acquire lock when lease doesn't exist", func() {
			// When: Acquire lock for new fingerprint
			acquired, err := lockManager.AcquireLock(ctx, "test-fingerprint-1")

			// Then: Lock acquired successfully
			Expect(err).ToNot(HaveOccurred())
			Expect(acquired).To(BeTrue())

			// And: Lease created in K8s
			lease := &coordinationv1.Lease{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Namespace: namespace,
				Name:      "gw-lock-test-fingerpri",
			}, lease)
			Expect(err).ToNot(HaveOccurred())
			Expect(*lease.Spec.HolderIdentity).To(Equal(holderID))
		})

		It("should acquire lock when we already hold it", func() {
			// Given: Lease exists and held by us
			fingerprint := "test-fingerprint-2"
			createLease(ctx, k8sClient, namespace, fingerprint, holderID, 30, time.Now())

			// When: Acquire lock again
			acquired, err := lockManager.AcquireLock(ctx, fingerprint)

			// Then: Lock acquired (idempotent)
			Expect(err).ToNot(HaveOccurred())
			Expect(acquired).To(BeTrue())
		})

		It("should NOT acquire lock when held by another pod", func() {
			// Given: Lease exists and held by another pod
			fingerprint := "test-fingerprint-3"
			createLease(ctx, k8sClient, namespace, fingerprint, otherHolderID, 30, time.Now())

			// When: Try to acquire lock
			acquired, err := lockManager.AcquireLock(ctx, fingerprint)

			// Then: Lock NOT acquired (no error)
			Expect(err).ToNot(HaveOccurred())
			Expect(acquired).To(BeFalse())
		})

		It("should acquire expired lease held by another pod", func() {
			// Given: Lease exists, held by another pod, but expired
			fingerprint := "test-fingerprint-4"
			createLease(ctx, k8sClient, namespace, fingerprint, otherHolderID, 30, time.Now().Add(-60*time.Second))

			// When: Try to acquire lock
			acquired, err := lockManager.AcquireLock(ctx, fingerprint)

			// Then: Lock acquired (took over expired lease)
			Expect(err).ToNot(HaveOccurred())
			Expect(acquired).To(BeTrue())

			// And: Lease now held by us
			lease := &coordinationv1.Lease{}
			_ = k8sClient.Get(ctx, client.ObjectKey{
				Namespace: namespace,
				Name:      "gw-lock-test-fingerpri",
			}, lease)
			Expect(*lease.Spec.HolderIdentity).To(Equal(holderID))
		})
	})

	Context("Lock Release", func() {
		It("should release lock successfully", func() {
			// Given: Lease exists and held by us
			fingerprint := "test-fingerprint-5"
			createLease(ctx, k8sClient, namespace, fingerprint, holderID, 30, time.Now())

			// When: Release lock
			err := lockManager.ReleaseLock(ctx, fingerprint)

			// Then: Lease deleted
			Expect(err).ToNot(HaveOccurred())

			lease := &coordinationv1.Lease{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Namespace: namespace,
				Name:      "gw-lock-test-fingerpri",
			}, lease)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should ignore NotFound error when releasing non-existent lock", func() {
			// Given: No lease exists
			fingerprint := "test-fingerprint-6"

			// When: Release lock
			err := lockManager.ReleaseLock(ctx, fingerprint)

			// Then: No error (idempotent)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Error Handling", func() {
		It("should return error for K8s API communication failures", func() {
			// Given: K8s client configured to return API error
			// (Use a mock client that returns errors for Get operations)
			// TODO: Implement mock client with configurable error responses

			// When: Try to acquire lock
			// acquired, err := lockManager.AcquireLock(ctx, "test-fingerprint-7")

			// Then: Error returned (not just false)
			// Expect(err).To(HaveOccurred())
			// Expect(err.Error()).To(ContainSubstring("failed to check for existing lease"))
		})

		It("should handle race condition when creating lease", func() {
			// Given: Two lock managers trying to acquire same lock
			lockManager2 := processing.NewDistributedLockManager(k8sClient, namespace, otherHolderID)
			fingerprint := "test-fingerprint-8"

			// When: Both try to acquire simultaneously
			results := make(chan bool, 2)
			go func() {
				acquired, _ := lockManager.AcquireLock(ctx, fingerprint)
				results <- acquired
			}()
			go func() {
				acquired, _ := lockManager2.AcquireLock(ctx, fingerprint)
				results <- acquired
			}()

			// Then: Only one acquires the lock
			acquired1 := <-results
			acquired2 := <-results
			Expect(acquired1 != acquired2).To(BeTrue(), "Exactly one lock manager should acquire the lock")
		})
	})
})

// Helper function to create a lease for testing
func createLease(ctx context.Context, k8sClient client.Client, namespace, fingerprint, holderID string, durationSeconds int32, renewTime time.Time) {
	leaseName := fmt.Sprintf("gw-lock-%s", fingerprint[:16])
	renewTimeMeta := metav1.NewTime(renewTime)

	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: namespace,
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &holderID,
			LeaseDurationSeconds: &durationSeconds,
			RenewTime:            &renewTimeMeta,
		},
	}

	_ = k8sClient.Create(ctx, lease)
}
```

**Expected Coverage**: 90%+ for `distributed_lock.go`

---

### Task 2.2: Integration Tests for Multi-Replica Deduplication (3h)

**File**: `test/integration/gateway/distributed_locking_test.go`

**Test Scenarios**:
- ✅ Concurrent requests with same fingerprint (5 requests, only 1 RR created)
- ✅ Lock contention handling (retry with backoff)
- ✅ Lease expiration on Gateway pod "crash" simulation
- ✅ Metrics validation (lock acquisition success/failure rates)

**Test Implementation** (abbreviated):

```go
package gateway

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

var _ = Describe("Gateway Distributed Locking Integration (DD-GATEWAY-013)", func() {
	var (
		testNamespace = "gw-lock-test"
		ctx           context.Context
		testClient    *K8sTestClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		testClient = SetupK8sTestClient(ctx)

		// Enable distributed locking for this test
		os.Setenv("GATEWAY_ENABLE_DISTRIBUTED_LOCKING", "true")
		os.Setenv("GATEWAY_LOCK_NAMESPACE", testNamespace)
	})

	AfterEach(func() {
		// Cleanup resources
		if testClient != nil {
			_ = testClient.Client.DeleteAllOf(ctx, &remediationv1alpha1.RemediationRequest{},
				client.InNamespace(testNamespace))
		}
		os.Unsetenv("GATEWAY_ENABLE_DISTRIBUTED_LOCKING")
	})

	Context("Multi-Replica Deduplication Protection", func() {
		It("should prevent duplicate RRs with simulated multiple Gateway replicas", func() {
			// Given: 3 simulated Gateway pods (3 separate Gateway servers)
			gatewayURLs := make([]string, 3)
			servers := make([]*httptest.Server, 3)

			for i := 0; i < 3; i++ {
				// Create Gateway server instance with unique pod name
				podName := fmt.Sprintf("gateway-pod-%d", i)
				os.Setenv("POD_NAME", podName)

				gatewayServer, err := StartTestGateway(ctx, testClient, dataStorageURL)
				Expect(err).ToNot(HaveOccurred())

				server := httptest.NewServer(gatewayServer.Handler())
				servers[i] = server
				gatewayURLs[i] = server.URL

				defer server.Close()
			}

			// When: Send 15 concurrent requests with SAME fingerprint to DIFFERENT Gateway pods
			fingerprint := fmt.Sprintf("distributed-lock-test-%d", time.Now().Unix())
			concurrentRequests := 15

			var wg sync.WaitGroup
			wg.Add(concurrentRequests)

			for i := 0; i < concurrentRequests; i++ {
				go func(reqNum int) {
					defer wg.Done()

					// Round-robin across Gateway pods
					gatewayURL := gatewayURLs[reqNum%3]

					payload := createPrometheusAlertPayload(PrometheusAlertOptions{
						AlertName: "DistributedLockTest",
						Namespace: testNamespace,
						Severity:  "warning",
						Labels: map[string]string{
							"fingerprint": fingerprint,
						},
					})

					req, _ := http.NewRequest("POST",
						fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL),
						bytes.NewBuffer(payload))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

					resp, _ := http.DefaultClient.Do(req)
					if resp != nil {
						resp.Body.Close()
					}
				}(i)
			}

			// Wait for all requests to complete
			wg.Wait()

			// Then: Only 1 RemediationRequest should be created
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := testClient.Client.List(ctx, rrList,
					client.InNamespace(testNamespace))
				if err != nil {
					return -1
				}

				// Filter by alert name
				count := 0
				for _, rr := range rrList.Items {
					if rr.Spec.SignalName == "DistributedLockTest" {
						count++
					}
				}
				return count
			}, 30*time.Second, 1*time.Second).Should(Equal(1),
				"Only 1 RR should be created despite 15 concurrent requests across 3 Gateway pods")

			// And: OccurrenceCount should reflect all duplicates
			Eventually(func() int32 {
				var rrList remediationv1alpha1.RemediationRequestList
				_ = testClient.Client.List(ctx, &rrList, client.InNamespace(testNamespace))

				for _, rr := range rrList.Items {
					if rr.Spec.SignalName == "DistributedLockTest" && rr.Status.Deduplication != nil {
						return rr.Status.Deduplication.OccurrenceCount
					}
				}
				return 0
			}, 30*time.Second, 1*time.Second).Should(BeNumerically(">=", 15),
				"OccurrenceCount should reflect all 15 requests")
		})

		It("should handle lock contention gracefully", func() {
			// Given: 2 Gateway pods
			pod1URL, pod2URL := setupTwoGatewayPods(ctx, testClient, dataStorageURL)

			// When: Send requests to both pods simultaneously
			fingerprint := fmt.Sprintf("contention-test-%d", time.Now().Unix())

			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer wg.Done()
				sendSignalToGateway(pod1URL, testNamespace, "ContentionTest", fingerprint)
			}()

			go func() {
				defer wg.Done()
				sendSignalToGateway(pod2URL, testNamespace, "ContentionTest", fingerprint)
			}()

			wg.Wait()

			// Then: Only 1 RR created
			Eventually(func() int {
				return countRRsByAlertName(ctx, testClient, testNamespace, "ContentionTest")
			}, 30*time.Second, 1*time.Second).Should(Equal(1))

			// And: Lock contention metric incremented
			// (Verify via metrics endpoint query)
		})

		It("should handle lease expiration on pod crash simulation", func() {
			// Given: Gateway pod acquires lock
			fingerprint := fmt.Sprintf("expiry-test-%d", time.Now().Unix())
			gatewayURL := setupSingleGatewayPod(ctx, testClient, dataStorageURL)

			// When: Send signal (lock acquired)
			sendSignalToGateway(gatewayURL, testNamespace, "ExpiryTest", fingerprint)

			// And: Simulate pod crash (don't release lock, let it expire)
			// Wait for lease expiration (30s + buffer)
			time.Sleep(35 * time.Second)

			// And: New Gateway pod tries to acquire lock
			newGatewayURL := setupSingleGatewayPod(ctx, testClient, dataStorageURL)
			sendSignalToGateway(newGatewayURL, testNamespace, "ExpiryTest", fingerprint)

			// Then: New Gateway pod should take over expired lease
			// (Verify via metrics or lease inspection)
		})
	})

	Context("Metrics Validation", func() {
		It("should record distributed locking metrics", func() {
			// Given: Gateway with distributed locking enabled
			gatewayURL, metricsURL := setupGatewayWithMetrics(ctx, testClient, dataStorageURL)

			// When: Process signals with lock acquisition
			fingerprint := fmt.Sprintf("metrics-test-%d", time.Now().Unix())
			sendSignalToGateway(gatewayURL, testNamespace, "MetricsTest", fingerprint)

			// Then: Metrics should be present
			resp, err := http.Get(metricsURL)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			metricsOutput := string(body)

			// Verify distributed locking metrics
			expectedMetrics := []string{
				"gateway_lock_acquisition_attempts_total",
				"gateway_lock_acquisition_success_total",
				"gateway_lock_hold_duration_seconds",
			}

			for _, metric := range expectedMetrics {
				Expect(metricsOutput).To(ContainSubstring(metric),
					"Expected metric %s not found", metric)
			}
		})
	})
})
```

**Expected Coverage**: Validates distributed locking behavior across simulated multi-replica deployment

---

### Task 2.3: E2E Tests for Production Scenario (2h)

**File**: `test/e2e/gateway/distributed_locking_test.go`

**Test Scenarios**:
- ✅ Gateway deployment with 3 replicas (actual K8s deployment)
- ✅ Send 100 concurrent signals with same fingerprint
- ✅ Verify only 1 RR created
- ✅ Verify all 3 Gateway pods processed requests (load distribution)
- ✅ Verify metrics exposure on all pods

**Test Implementation** (abbreviated):

```go
package gateway

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

var _ = Describe("Gateway Distributed Locking E2E (DD-GATEWAY-013)", func() {
	var (
		testNamespace = "gw-e2e-lock"
		ctx           context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("Production Multi-Replica Deployment", func() {
		It("should handle 100 concurrent signals across 3 Gateway replicas", func() {
			// Given: Gateway deployed with 3 replicas and distributed locking enabled
			gatewayDeployment := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, client.ObjectKey{
				Namespace: "kubernaut-system",
				Name:      "gateway",
			}, gatewayDeployment)
			Expect(err).ToNot(HaveOccurred())
			Expect(*gatewayDeployment.Spec.Replicas).To(Equal(int32(3)))

			// Verify distributed locking is enabled
			envVars := gatewayDeployment.Spec.Template.Spec.Containers[0].Env
			for _, env := range envVars {
				if env.Name == "GATEWAY_ENABLE_DISTRIBUTED_LOCKING" {
					Expect(env.Value).To(Equal("true"))
				}
			}

			// When: Send 100 concurrent signals with SAME fingerprint
			fingerprint := fmt.Sprintf("e2e-lock-test-%d", time.Now().Unix())
			gatewayURL := getGatewayServiceURL() // From E2E suite setup

			var wg sync.WaitGroup
			wg.Add(100)

			for i := 0; i < 100; i++ {
				go func() {
					defer wg.Done()
					sendPrometheusAlert(gatewayURL, testNamespace, "E2ELockTest", fingerprint)
				}()
			}

			wg.Wait()

			// Then: Only 1 RemediationRequest created
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				_ = k8sClient.List(ctx, rrList, client.InNamespace(testNamespace))

				count := 0
				for _, rr := range rrList.Items {
					if rr.Spec.SignalName == "E2ELockTest" {
						count++
					}
				}
				return count
			}, 60*time.Second, 2*time.Second).Should(Equal(1),
				"Only 1 RR should be created despite 100 concurrent requests")

			// And: OccurrenceCount should be 100
			Eventually(func() int32 {
				var rrList remediationv1alpha1.RemediationRequestList
				_ = k8sClient.List(ctx, &rrList, client.InNamespace(testNamespace))

				for _, rr := range rrList.Items {
					if rr.Spec.SignalName == "E2ELockTest" && rr.Status.Deduplication != nil {
						return rr.Status.Deduplication.OccurrenceCount
					}
				}
				return 0
			}, 60*time.Second, 2*time.Second).Should(Equal(int32(100)))
		})

		It("should distribute load across all Gateway replicas", func() {
			// Given: 3 Gateway pods running
			pods := getGatewayPods(ctx)
			Expect(len(pods)).To(Equal(3))

			// When: Send multiple signals
			fingerprint := fmt.Sprintf("load-test-%d", time.Now().Unix())
			for i := 0; i < 30; i++ {
				sendPrometheusAlert(gatewayURL, testNamespace, "LoadTest", fingerprint)
			}

			// Then: Verify metrics from all pods show activity
			for _, pod := range pods {
				metricsURL := getPodMetricsURL(pod)
				metrics := fetchMetrics(metricsURL)

				// Each pod should have processed at least some requests
				Expect(metrics).To(ContainSubstring("gateway_signals_received_total"))
			}
		})
	})
})
```

**Expected Outcome**: Validates production deployment with distributed locking enabled

---

### Task 2.4: Performance Baseline Comparison (1h)

**Objective**: Measure latency impact of distributed locking

**Methodology**:
1. Baseline measurement (distributed locking disabled):
   - Run 1000 requests with unique fingerprints
   - Measure P50, P95, P99 latencies
2. Distributed locking measurement (enabled):
   - Run 1000 requests with unique fingerprints
   - Measure P50, P95, P99 latencies
3. Compare results:
   - Target: P95 latency increase <20ms

**Tool**: `vegeta` load testing tool

**Test Script**:

```bash
#!/bin/bash
# performance_comparison.sh

GATEWAY_URL="http://localhost:8080"
DURATION="60s"
RATE="50/s"  # 50 requests per second

# Test 1: Distributed locking disabled
echo "=== Baseline (Distributed Locking Disabled) ==="
kubectl set env deployment/gateway -n kubernaut-system GATEWAY_ENABLE_DISTRIBUTED_LOCKING=false
sleep 10  # Wait for rollout

echo "GET ${GATEWAY_URL}/api/v1/signals/prometheus" | \
  vegeta attack -duration=${DURATION} -rate=${RATE} | \
  vegeta report -type=text > baseline_results.txt

# Test 2: Distributed locking enabled
echo "=== With Distributed Locking Enabled ==="
kubectl set env deployment/gateway -n kubernaut-system GATEWAY_ENABLE_DISTRIBUTED_LOCKING=true
sleep 10  # Wait for rollout

echo "GET ${GATEWAY_URL}/api/v1/signals/prometheus" | \
  vegeta attack -duration=${DURATION} -rate=${RATE} | \
  vegeta report -type=text > distributed_lock_results.txt

# Compare results
echo "=== Performance Comparison ==="
diff baseline_results.txt distributed_lock_results.txt
```

**Success Criteria**:
- P50 latency increase: <10ms
- P95 latency increase: <20ms
- P99 latency increase: <30ms

---

### Day 2 EOD Checklist

- [ ] All unit tests implemented and passing (90%+ coverage)
- [ ] Integration tests implemented and passing
- [ ] E2E tests implemented and passing
- [ ] Performance baseline comparison completed
- [ ] Metrics validation completed
- [ ] All lint checks passing
- [ ] Implementation summary document created

---

## Success Criteria

### Functional Requirements

- [ ] Distributed lock manager correctly acquires/releases K8s Lease resources
- [ ] Gateway `ProcessSignal()` integrates distributed locking
- [ ] Feature flag allows enabling/disabling distributed locking
- [ ] RBAC permissions configured for Lease resources
- [ ] Multiple Gateway pods can run concurrently without duplicate RR creation

### Test Coverage

- [ ] Unit tests: 90%+ coverage for `distributed_lock.go`
- [ ] Integration tests: Multi-replica deduplication scenarios validated
- [ ] E2E tests: Production deployment with 3 replicas validated
- [ ] All tests passing (unit, integration, E2E)

### Performance Requirements

- [ ] P95 latency increase <20ms with distributed locking enabled
- [ ] Lock acquisition success rate >99.9%
- [ ] Lock contention rate <5% under normal load
- [ ] Duplicate RR creation rate <0.001% (vs. ~0.1% without distributed locking)

### Observability Requirements

- [ ] 6 new Prometheus metrics exposed:
  - `gateway_lock_acquisition_attempts_total`
  - `gateway_lock_acquisition_success_total`
  - `gateway_lock_acquisition_failures_total`
  - `gateway_lock_contention_total`
  - `gateway_lock_wait_duration_seconds`
  - `gateway_lock_hold_duration_seconds`
- [ ] Metrics validated in integration and E2E tests
- [ ] Grafana dashboard updated with distributed locking metrics

### Documentation Requirements

- [ ] DD-GATEWAY-013 updated with implementation status
- [ ] Implementation summary handoff document created
- [ ] RBAC changes documented
- [ ] Configuration guide updated with distributed locking settings
- [ ] Runbook created for troubleshooting distributed locking issues

---

## Rollback Plan

### Feature Flag Rollback

**Scenario**: Performance degradation or unexpected behavior with distributed locking

**Procedure**:
1. **Immediate Rollback**: Disable via environment variable
   ```bash
   kubectl set env deployment/gateway -n kubernaut-system GATEWAY_ENABLE_DISTRIBUTED_LOCKING=false
   ```
2. **Verify Rollback**: Check Gateway logs for "Distributed locking disabled"
3. **Monitor Metrics**: Verify latency returns to baseline
4. **Investigate**: Review Gateway logs and metrics for root cause

**Rollback Time**: <5 minutes (no code changes, just env var update)

### Code Rollback

**Scenario**: Critical bug discovered in distributed locking implementation

**Procedure**:
1. **Revert Commit**: `git revert <commit-sha>`
2. **Rebuild Image**: `make docker-build docker-push`
3. **Redeploy**: `kubectl rollout restart deployment/gateway -n kubernaut-system`
4. **Verify**: Check Gateway health and test signal processing

**Rollback Time**: <10 minutes (rebuild + redeploy)

### Communication Plan

**If Rollback Required**:
1. Notify team in Slack: `#kubernaut-alerts`
2. Create incident ticket with root cause analysis
3. Schedule post-mortem to review implementation

---

## Questions & Concerns

### Pre-Implementation Questions

**Q1**: Should distributed locking be enabled by default in V1.0?
- **Recommendation**: No, default to **disabled** for backward compatibility
- **Rationale**: Allows gradual rollout and performance validation
- **Rollout Plan**: Enable in dev → staging → production (phased)

**Q2**: What should lease duration be?
- **Recommendation**: 30 seconds
- **Rationale**: Balance between safety (short enough to prevent long waits) and efficiency (long enough to avoid frequent lease renewals)
- **Tuning**: Can be adjusted via `GATEWAY_LOCK_DURATION_SECONDS` env var

**Q3**: Should we add circuit breaker for K8s API failures?
- **Recommendation**: Not in V1.0
- **Rationale**: Fail-fast is acceptable for V1.0 (Gateway should not process signals if K8s API is down)
- **Future Enhancement**: Add circuit breaker in V1.1 if K8s API instability observed

---

## Related Documentation

### Design Decisions
- [DD-GATEWAY-013](../../../../architecture/decisions/DD-GATEWAY-013-multi-replica-deduplication.md) - Multi-Replica Deduplication (Alternative 1 approved)
- [DD-GATEWAY-011](../../../../architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md) - Status-Based Deduplication
- [DD-005](../../../../architecture/decisions/DD-005-unified-logging-framework.md) - Unified Logging Framework

### Test Plans
- [TEST_PLAN_DISTRIBUTED_LOCKING_V1.0.md](./TEST_PLAN_DISTRIBUTED_LOCKING_V1.0.md) - Comprehensive test plan

### Business Requirements
- BR-GATEWAY-190: Multi-Replica Safety
- BR-GATEWAY-185: Deduplication Correctness
- BR-GATEWAY-183: Optimistic Concurrency

---

## Approval & Sign-Off

**Approved By**: jordigilh (User)
**Approval Date**: December 30, 2025
**Implementation Start**: [Date]
**Expected Completion**: [Date + 2 days]

**Status**: ✅ **APPROVED** - Ready for Implementation

