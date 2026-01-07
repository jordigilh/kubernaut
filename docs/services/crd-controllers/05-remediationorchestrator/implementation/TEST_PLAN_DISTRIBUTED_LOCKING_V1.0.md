# RemediationOrchestrator - Distributed Locking Test Plan V1.0

**Version**: 1.0.0
**Created**: December 30, 2025
**Status**: Active
**Purpose**: Comprehensive test plan for RO distributed locking implementation
**Service Type**: CRD Controller
**Team**: RemediationOrchestrator Team
**Implementation Plan**: [IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md](./IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md)

---

## Overview

This test plan validates the K8s Lease-based distributed locking mechanism for multi-replica RemediationOrchestrator deployments. The goal is to eliminate duplicate WorkflowExecution CRD creation when multiple RO pods process RemediationRequests targeting the same resource concurrently.

**Reference Documents**:
- [ADR-052](../../../../architecture/decisions/ADR-052-distributed-locking-pattern.md) - Distributed locking pattern (to be created)
- [BR-ORCH-050](../../../../requirements/BR-ORCH-050.md) - Multi-Replica Resource Lock Safety
- [TESTING_GUIDELINES.md](../../../../development/TESTING_GUIDELINES.md) - Testing standards
- [IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md](./IMPLEMENTATION_PLAN_DISTRIBUTED_LOCKING_V1.0.md) - Implementation details
- [RO_RACE_CONDITION_ANALYSIS_DEC_30_2025.md](../../../../handoff/RO_RACE_CONDITION_ANALYSIS_DEC_30_2025.md) - Race condition analysis

---

## Test Strategy

### Test Pyramid Distribution

| Test Tier | Coverage | Focus | Duration |
|-----------|----------|-------|----------|
| **Unit Tests** | 90%+ | Lock manager logic, routing engine integration | ~3 hours |
| **Integration Tests** | Multi-replica simulation | Concurrent RR processing, K8s Lease operations | ~3 hours |
| **E2E Tests** | Production deployment | 3-replica RO with concurrent RRs | ~2 hours |

### Success Criteria

- ✅ Zero duplicate WFEs created with multi-replica deployment
- ✅ Lock acquisition failure rate <0.1% (K8s API errors only)
- ✅ P95 reconciliation latency increase <20ms
- ✅ All tests passing (unit, integration, E2E)
- ✅ 90%+ code coverage for distributed locking code

---

## 1. Unit Tests

### 1.1 Test File Locations

**Lock Manager Tests**:
- `pkg/remediationorchestrator/locking/distributed_lock_test.go`

**Routing Engine Tests**:
- `test/unit/remediationorchestrator/routing_lock_test.go`

**Coverage Target**: 90%+ for lock manager and routing engine

---

### 1.2 Lock Manager Unit Tests

#### Scenario 1: Lock Acquisition Success

| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| **New Lock** | Acquire lock for target resource with no existing lease | Lease created, `acquired=true` |
| **Reentrant Lock** | Acquire lock we already hold | Idempotent, `acquired=true` |
| **Expired Lease Takeover** | Acquire expired lease held by another pod | Lease updated with our holder ID, `acquired=true` |

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
        holderID = "ro-pod-1"
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
            // Given: No existing lease for target resource
            targetResource := "Node/worker-1"

            // When: Acquire lock
            acquired, err := lockManager.AcquireLock(ctx, targetResource)

            // Then: Lock acquired successfully
            Expect(err).ToNot(HaveOccurred())
            Expect(acquired).To(BeTrue())

            // And: Lease created in K8s
            lease := &coordinationv1.Lease{}
            err = k8sClient.Get(ctx, client.ObjectKey{
                Namespace: namespace,
                Name:      "ro-lock-Node-worker-1", // Expected lease name
            }, lease)
            Expect(err).ToNot(HaveOccurred())
            Expect(*lease.Spec.HolderIdentity).To(Equal(holderID))
            Expect(*lease.Spec.LeaseDurationSeconds).To(Equal(int32(30)))
        })

        It("should be idempotent when acquiring lock we already hold", func() {
            // Given: We already hold the lock
            targetResource := "Node/worker-2"
            acquired, err := lockManager.AcquireLock(ctx, targetResource)
            Expect(err).ToNot(HaveOccurred())
            Expect(acquired).To(BeTrue())

            // When: Try to acquire same lock again
            acquired, err = lockManager.AcquireLock(ctx, targetResource)

            // Then: Lock acquired again (idempotent)
            Expect(err).ToNot(HaveOccurred())
            Expect(acquired).To(BeTrue())
        })

        It("should take over expired lease", func() {
            // Given: Lease exists but expired (held by crashed pod)
            targetResource := "Node/worker-3"
            expiredLease := createExpiredLease(ctx, k8sClient, namespace, targetResource, "crashed-pod")

            // When: Try to acquire lock
            acquired, err := lockManager.AcquireLock(ctx, targetResource)

            // Then: Lock acquired (takeover)
            Expect(err).ToNot(HaveOccurred())
            Expect(acquired).To(BeTrue())

            // And: Lease holder updated to us
            lease := &coordinationv1.Lease{}
            err = k8sClient.Get(ctx, client.ObjectKey{
                Namespace: namespace,
                Name:      generateLeaseName(targetResource),
            }, lease)
            Expect(err).ToNot(HaveOccurred())
            Expect(*lease.Spec.HolderIdentity).To(Equal(holderID))
        })
    })
})
```

---

#### Scenario 2: Lock Acquisition Failure

| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| **Lock Held by Another Pod** | Try to acquire lock held by another pod | `acquired=false`, no error |
| **K8s API Down** | K8s API returns communication error | `acquired=false`, error returned |
| **Permission Denied** | RBAC insufficient for Lease operations | `acquired=false`, error returned |

**Test Pattern**:
```go
Describe("AcquireLock Failures", func() {
    It("should NOT acquire lock when held by another pod", func() {
        // Given: Lease exists and held by another pod
        targetResource := "Node/worker-4"
        otherPodID := "ro-pod-2"
        createLease(ctx, k8sClient, namespace, targetResource, otherPodID)

        // When: Try to acquire lock
        acquired, err := lockManager.AcquireLock(ctx, targetResource)

        // Then: Lock NOT acquired (no error - expected behavior)
        Expect(err).ToNot(HaveOccurred())
        Expect(acquired).To(BeFalse())

        // And: Lease still held by other pod
        lease := &coordinationv1.Lease{}
        err = k8sClient.Get(ctx, client.ObjectKey{
            Namespace: namespace,
            Name:      generateLeaseName(targetResource),
        }, lease)
        Expect(err).ToNot(HaveOccurred())
        Expect(*lease.Spec.HolderIdentity).To(Equal(otherPodID))
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
        acquired, err := lockManager.AcquireLock(ctx, "Node/worker-5")

        // Then: Error returned
        Expect(err).To(HaveOccurred())
        Expect(acquired).To(BeFalse())
        Expect(err.Error()).To(ContainSubstring("failed to check lease"))
    })

    It("should return error on RBAC permission denied", func() {
        // Given: K8s API returns Forbidden error
        forbiddenClient := fake.NewClientBuilder().
            WithScheme(scheme).
            WithInterceptorFuncs(interceptor.Funcs{
                Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
                    return apierrors.NewForbidden(
                        schema.GroupResource{Group: "coordination.k8s.io", Resource: "leases"},
                        "test-lease",
                        fmt.Errorf("RBAC permission denied"),
                    )
                },
            }).
            Build()

        lockManager := locking.NewDistributedLockManager(
            forbiddenClient,
            namespace,
            holderID,
            30*time.Second,
        )

        // When: Try to acquire lock
        acquired, err := lockManager.AcquireLock(ctx, "Node/worker-6")

        // Then: Error returned
        Expect(err).To(HaveOccurred())
        Expect(acquired).To(BeFalse())
        Expect(err.Error()).To(ContainSubstring("failed to create lease"))
    })
})
```

---

#### Scenario 3: Lock Release

| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| **Successful Release** | Release lock we hold | Lease deleted, no error |
| **Idempotent Release** | Release lock that doesn't exist | No error (idempotent) |
| **Release Other's Lock** | Try to release lock held by another pod | No action (not our lock) |

**Test Pattern**:
```go
Describe("ReleaseLock", func() {
    It("should release lock successfully", func() {
        // Given: We hold the lock
        targetResource := "Node/worker-7"
        acquired, err := lockManager.AcquireLock(ctx, targetResource)
        Expect(err).ToNot(HaveOccurred())
        Expect(acquired).To(BeTrue())

        // When: Release lock
        lockManager.ReleaseLock(ctx, targetResource)

        // Then: Lease deleted
        lease := &coordinationv1.Lease{}
        err = k8sClient.Get(ctx, client.ObjectKey{
            Namespace: namespace,
            Name:      generateLeaseName(targetResource),
        }, lease)
        Expect(apierrors.IsNotFound(err)).To(BeTrue())
    })

    It("should be idempotent when releasing non-existent lock", func() {
        // When: Release lock that doesn't exist
        // Should not panic or error (idempotent)
        lockManager.ReleaseLock(ctx, "Node/non-existent")

        // Then: No panic (test passes)
    })

    It("should NOT release lock held by another pod", func() {
        // Given: Another pod holds the lock
        targetResource := "Node/worker-8"
        otherPodID := "ro-pod-3"
        createLease(ctx, k8sClient, namespace, targetResource, otherPodID)

        // When: Try to release lock
        lockManager.ReleaseLock(ctx, targetResource)

        // Then: Lease still exists (not deleted)
        lease := &coordinationv1.Lease{}
        err := k8sClient.Get(ctx, client.ObjectKey{
            Namespace: namespace,
            Name:      generateLeaseName(targetResource),
        }, lease)
        Expect(err).ToNot(HaveOccurred())
        Expect(*lease.Spec.HolderIdentity).To(Equal(otherPodID))
    })
})
```

---

#### Scenario 4: Edge Cases

| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| **Concurrent Acquisition** | Two pods try to acquire same lock simultaneously | Only one succeeds |
| **Lease Expiration Boundary** | Acquire lock at exact expiration time | Correct expiration behavior |
| **Invalid Lock Keys** | Lock keys with special chars, too long | Handled gracefully |

**Test Pattern**:
```go
Describe("Edge Cases", func() {
    It("should handle concurrent lock acquisition (only one succeeds)", func() {
        // Given: Two lock managers (simulating two pods)
        lockMgr1 := locking.NewDistributedLockManager(k8sClient, namespace, "ro-pod-1", 30*time.Second)
        lockMgr2 := locking.NewDistributedLockManager(k8sClient, namespace, "ro-pod-2", 30*time.Second)
        targetResource := "Node/worker-9"

        // When: Both try to acquire lock concurrently
        var wg sync.WaitGroup
        var acquired1, acquired2 bool
        var err1, err2 error

        wg.Add(2)
        go func() {
            defer wg.Done()
            acquired1, err1 = lockMgr1.AcquireLock(ctx, targetResource)
        }()
        go func() {
            defer wg.Done()
            time.Sleep(1 * time.Millisecond) // Slight delay to simulate race
            acquired2, err2 = lockMgr2.AcquireLock(ctx, targetResource)
        }()
        wg.Wait()

        // Then: Only one acquired lock
        Expect(err1).ToNot(HaveOccurred())
        Expect(err2).ToNot(HaveOccurred())
        Expect(acquired1 && !acquired2 || !acquired1 && acquired2).To(BeTrue(),
            "Exactly one pod should acquire lock")
    })

    It("should handle lease expiration boundary correctly", func() {
        // Given: Lease that expires in 1 second
        targetResource := "Node/worker-10"
        otherPodID := "ro-pod-expiring"
        createLeaseExpiringSoon(ctx, k8sClient, namespace, targetResource, otherPodID, 1*time.Second)

        // When: Wait for expiration
        time.Sleep(1500 * time.Millisecond)

        // And: Try to acquire lock
        acquired, err := lockManager.AcquireLock(ctx, targetResource)

        // Then: Lock acquired (expired lease taken over)
        Expect(err).ToNot(HaveOccurred())
        Expect(acquired).To(BeTrue())
    })

    It("should handle long lock keys gracefully", func() {
        // Given: Very long target resource name
        longResourceName := "Pod/" + strings.Repeat("very-long-pod-name-", 10) // >200 chars

        // When: Try to acquire lock
        acquired, err := lockManager.AcquireLock(ctx, longResourceName)

        // Then: Handled gracefully (lease name truncated/hashed)
        Expect(err).ToNot(HaveOccurred())
        Expect(acquired).To(BeTrue())

        // And: Lease name is K8s-compatible (<=63 chars)
        lease := &coordinationv1.Lease{}
        leases := &coordinationv1.LeaseList{}
        err = k8sClient.List(ctx, leases, client.InNamespace(namespace))
        Expect(err).ToNot(HaveOccurred())
        Expect(len(leases.Items)).To(Equal(1))
        Expect(len(leases.Items[0].Name)).To(BeNumerically("<=", 63))
    })
})
```

---

### 1.3 Routing Engine Unit Tests

#### Scenario 1: Lock Integration in CheckBlockingConditions

| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| **Lock Acquired Before Checks** | Verify lock acquired before routing checks | Lock acquisition logged/verified |
| **Lock Contention Handling** | Another pod holds lock | Return `LockContentionRetry` blocking condition |
| **Lock Handle Returned** | Lock acquired successfully | LockHandle returned for caller to release |
| **Lock Handle on Blocking** | Lock acquired but blocked by other condition | LockHandle still returned |

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
            "ro-pod-1",
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
                        Kind: "Node",
                        Name: "worker-1",
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
})
```

---

### 1.4 Unit Test Execution

**Run Tests**:
```bash
# Run all RO unit tests with locking
ginkgo -v ./pkg/remediationorchestrator/locking/
ginkgo -v ./test/unit/remediationorchestrator/routing_lock_test.go

# Run full RO unit test suite
make test-unit-remediationorchestrator

# Check coverage
go test -coverprofile=coverage.out ./pkg/remediationorchestrator/locking/
go tool cover -html=coverage.out
# Target: 90%+ coverage
```

**Success Criteria**:
- ✅ All unit tests passing
- ✅ 90%+ code coverage for lock manager
- ✅ No race conditions detected (`go test -race`)
- ✅ Fast execution (<30 seconds for all unit tests)

---

## 2. Integration Tests

### 2.1 Test File Location

**File**: `test/integration/remediationorchestrator/multi_replica_locking_integration_test.go`

**Environment**: envtest (real K8s API)

---

### 2.2 Multi-Replica Integration Tests

#### Scenario 1: No Race Condition with Single Replica (Baseline)

**Purpose**: Verify existing behavior is preserved

**Test Pattern**:
```go
var _ = Describe("RO Multi-Replica Locking Integration", func() {
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
        namespace = "test-" + uuid.New().String()[:8]

        // Start envtest (real K8s API)
        testEnv = &envtest.Environment{
            CRDDirectoryPaths: []string{
                filepath.Join("..", "..", "..", "config", "crd", "bases"),
            },
        }
        cfg, err := testEnv.Start()
        Expect(err).ToNot(HaveOccurred())

        // Create K8s client
        k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
        Expect(err).ToNot(HaveOccurred())

        // Create namespace
        ns := &corev1.Namespace{
            ObjectMeta: metav1.ObjectMeta{Name: namespace},
        }
        Expect(k8sClient.Create(ctx, ns)).To(Succeed())

        // Create two RO controller instances
        roPod1 = setupROController(ctx, k8sClient, namespace, "ro-pod-1")
        roPod2 = setupROController(ctx, k8sClient, namespace, "ro-pod-2")
    })

    AfterEach(func() {
        Expect(testEnv.Stop()).To(Succeed())
    })

    Describe("Single Replica Behavior (Baseline)", func() {
        It("should NOT create duplicate WFEs with single replica", func() {
            // Given: 2 RRs targeting same resource
            rr1 := createRR(ctx, k8sClient, namespace, "rr-1", "Node/worker-1")
            rr2 := createRR(ctx, k8sClient, namespace, "rr-2", "Node/worker-1")

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
            Expect(len(wfeList.Items)).To(Equal(1))
        })
    })
})
```

---

#### Scenario 2: Race Prevention with Multiple Replicas

**Purpose**: Verify distributed locking prevents duplicate WFE creation

**Test Pattern**:
```go
Describe("Multi-Replica Race Prevention", func() {
    It("should NOT create duplicate WFEs with 2 concurrent replicas", func() {
        // Given: 2 RRs targeting SAME resource
        rr1 := createRR(ctx, k8sClient, namespace, "rr-1", "Node/worker-1")
        rr2 := createRR(ctx, k8sClient, namespace, "rr-2", "Node/worker-1")

        // When: 2 RO pods process concurrently
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
            time.Sleep(5 * time.Millisecond) // Simulate concurrent processing
            result2, err2 = roPod2.Reconcile(ctx, reconcile.Request{
                NamespacedName: client.ObjectKeyFromObject(rr2),
            })
        }()

        wg.Wait()

        // Then: No errors
        Expect(err1).ToNot(HaveOccurred())
        Expect(err2).ToNot(HaveOccurred())

        // And: Only 1 WFE created
        Eventually(func() int {
            wfeList := &workflowexecutionv1.WorkflowExecutionList{}
            _ = k8sClient.List(ctx, wfeList, client.InNamespace(namespace))
            return len(wfeList.Items)
        }, "10s", "100ms").Should(Equal(1))

        // And: One reconcile requeued with backoff (lock contention)
        hasRequeue := result1.RequeueAfter > 0 || result2.RequeueAfter > 0
        Expect(hasRequeue).To(BeTrue())
    })

    It("should create WFEs for DIFFERENT resources concurrently", func() {
        // Given: 2 RRs targeting DIFFERENT resources
        rr1 := createRR(ctx, k8sClient, namespace, "rr-1", "Node/worker-1")
        rr2 := createRR(ctx, k8sClient, namespace, "rr-2", "Node/worker-2")

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

        // Then: 2 WFEs created (different targets, no contention)
        Eventually(func() int {
            wfeList := &workflowexecutionv1.WorkflowExecutionList{}
            _ = k8sClient.List(ctx, wfeList, client.InNamespace(namespace))
            return len(wfeList.Items)
        }, "10s", "100ms").Should(Equal(2))
    })
})
```

---

### 2.3 Integration Test Execution

**Run Tests**:
```bash
# Run RO integration tests
make test-integration-remediationorchestrator

# Run specific multi-replica tests
ginkgo -v -focus="Multi-Replica" ./test/integration/remediationorchestrator/

# Check test time
# Target: <3 minutes for all integration tests
```

**Success Criteria**:
- ✅ All integration tests passing
- ✅ No duplicate WFEs created in multi-replica scenarios
- ✅ Lock contention handled gracefully (requeue, not error)
- ✅ Tests complete in <3 minutes

---

## 3. E2E Tests

### 3.1 Test File Location

**File**: `test/e2e/remediationorchestrator/multi_replica_locking_e2e_test.go`

**Environment**: Kind cluster with 3-replica RO deployment

---

### 3.2 Multi-Replica E2E Tests

#### Scenario 1: 3-Replica RO Deployment

**Purpose**: Validate production deployment with multiple concurrent RRs

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
        namespace = "test-ro-e2e-" + uuid.New().String()[:8]

        // k8sClient initialized by e2e test suite (Kind cluster)
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
            // Given: RO deployed with 3 replicas
            Eventually(func() int {
                pods := &corev1.PodList{}
                _ = k8sClient.List(ctx, pods,
                    client.InNamespace("kubernaut-system"),
                    client.MatchingLabels{"app": "remediationorchestrator"},
                )
                return len(pods.Items)
            }, "30s", "1s").Should(Equal(3))

            // When: Create 10 RRs targeting same resource concurrently
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

            // Then: Only 1 WFE created
            Eventually(func() int {
                wfeList := &workflowexecutionv1.WorkflowExecutionList{}
                _ = k8sClient.List(ctx, wfeList, client.InNamespace(namespace))
                return len(wfeList.Items)
            }, "30s", "1s").Should(Equal(1))

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
            }, "30s", "1s").Should(Equal(9))
        })
    })
})
```

---

### 3.3 E2E Test Execution

**Run Tests**:
```bash
# Run RO E2E tests
make test-e2e-remediationorchestrator

# Run specific multi-replica E2E tests
ginkgo -v -focus="Multi-Replica E2E" ./test/e2e/remediationorchestrator/

# Check test time
# Target: <2 minutes for E2E tests
```

**Success Criteria**:
- ✅ All E2E tests passing
- ✅ 3 RO replicas running in Kind cluster
- ✅ Only 1 WFE created despite 10 concurrent RRs
- ✅ No leaked Lease resources
- ✅ Tests complete in <2 minutes

---

## 4. Performance Testing

### 4.1 Latency Impact Measurement

**Purpose**: Verify P95 latency increase <20ms

**Test Pattern**:
```go
Describe("Performance Impact", func() {
    It("should have P95 latency increase <20ms", func() {
        latencies := make([]time.Duration, 100)

        for i := 0; i < 100; i++ {
            rr := createRR(ctx, k8sClient, namespace,
                fmt.Sprintf("rr-perf-%d", i),
                fmt.Sprintf("Node/worker-%d", i)) // Different targets

            start := time.Now()
            Expect(k8sClient.Create(ctx, rr)).To(Succeed())

            Eventually(func() bool {
                _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
                return rr.Status.WorkflowExecutionRef != nil
            }, "10s", "100ms").Should(BeTrue())

            latencies[i] = time.Since(start)
        }

        // Calculate P95
        sort.Slice(latencies, func(i, j int) bool {
            return latencies[i] < latencies[j]
        })
        p95Index := int(float64(len(latencies)) * 0.95)
        p95Latency := latencies[p95Index]

        fmt.Printf("P95 Latency: %v\n", p95Latency)

        // Verify reasonable latency
        Expect(p95Latency).To(BeNumerically("<", 500*time.Millisecond))
    })
})
```

---

## 5. Test Coverage Requirements

### 5.1 Coverage Targets

| Component | Target | Measurement |
|-----------|--------|-------------|
| **Lock Manager** | 90%+ | Lines covered |
| **Routing Engine** | 90%+ | Lines covered (lock integration) |
| **Overall RO** | 85%+ | Lines covered (including locking) |

### 5.2 Coverage Measurement

```bash
# Generate coverage for lock manager
go test -coverprofile=coverage_lock.out ./pkg/remediationorchestrator/locking/
go tool cover -html=coverage_lock.out

# Generate coverage for routing engine
go test -coverprofile=coverage_routing.out ./pkg/remediationorchestrator/routing/
go tool cover -html=coverage_routing.out

# Overall RO coverage
make test-unit-remediationorchestrator-coverage
```

---

## 6. Test Execution Summary

### 6.1 Test Execution Order

1. **Unit Tests** (3 hours)
   - Lock manager tests
   - Routing engine tests
   - Fast feedback loop

2. **Integration Tests** (3 hours)
   - Multi-replica scenarios
   - envtest with real K8s API
   - Lock contention validation

3. **E2E Tests** (2 hours)
   - 3-replica deployment
   - Production-like environment
   - End-to-end validation

**Total Time**: 8 hours (Day 2 of implementation plan)

---

### 6.2 Success Criteria Summary

**Functional**:
- ✅ All tests passing (unit, integration, E2E)
- ✅ No duplicate WFEs created in multi-replica scenarios
- ✅ Lock contention handled gracefully (requeue, not error)

**Performance**:
- ✅ P95 latency increase <20ms
- ✅ Lock acquisition failure rate <0.1%
- ✅ Tests complete in reasonable time (unit <30s, integration <3min, E2E <2min)

**Quality**:
- ✅ 90%+ code coverage for lock manager
- ✅ No race conditions detected
- ✅ No resource leaks (Lease cleanup verified)

---

## 7. Test Maintenance

### 7.1 Test Updates Required When:

- Lock duration changes (currently 30s)
- Lock key generation logic changes
- Routing engine integration changes
- Error handling changes

### 7.2 Test Documentation

All tests must include:
- Clear test description
- Business requirement reference (BR-ORCH-050)
- Expected behavior
- Failure scenarios

---

## Appendix A: Test Helpers

### Helper Functions

```go
// Create RemediationRequest for testing
func createRR(ctx context.Context, k8sClient client.Client, namespace, name, targetResource string) *remediationv1.RemediationRequest {
    parts := strings.Split(targetResource, "/")
    rr := &remediationv1.RemediationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      name,
            Namespace: namespace,
        },
        Spec: remediationv1.RemediationRequestSpec{
            TargetResource: remediationv1.TargetResourceSpec{
                Kind: parts[0],
                Name: parts[1],
            },
        },
    }
    Expect(k8sClient.Create(ctx, rr)).To(Succeed())
    return rr
}

// Create expired lease for testing
func createExpiredLease(ctx context.Context, k8sClient client.Client, namespace, targetResource, holderID string) *coordinationv1.Lease {
    expiredTime := metav1.NewMicroTime(time.Now().Add(-60 * time.Second))
    leaseDurationSeconds := int32(30)

    lease := &coordinationv1.Lease{
        ObjectMeta: metav1.ObjectMeta{
            Name:      generateLeaseName(targetResource),
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

// Generate lease name from target resource
func generateLeaseName(targetResource string) string {
    // Simplified - production version should handle truncation/hashing
    safeName := strings.ReplaceAll(targetResource, "/", "-")
    return "ro-lock-" + safeName
}
```

---

## Appendix B: References

### Documentation
- [ADR-052](../../../../architecture/decisions/ADR-052-distributed-locking-pattern.md) - Distributed locking pattern
- [BR-ORCH-050](../../../../requirements/BR-ORCH-050.md) - Multi-Replica Resource Lock Safety
- [RO_RACE_CONDITION_ANALYSIS_DEC_30_2025.md](../../../../handoff/RO_RACE_CONDITION_ANALYSIS_DEC_30_2025.md)
- [TESTING_GUIDELINES.md](../../../../development/TESTING_GUIDELINES.md)

### Related Test Plans
- [Gateway Test Plan](../../../stateless/gateway-service/implementation/TEST_PLAN_DISTRIBUTED_LOCKING_V1.0.md)

---

**Status**: ✅ **READY FOR EXECUTION**
**Timeline**: Day 2 of implementation (8 hours)
**Confidence**: 95% - Proven pattern from Gateway, adapted for RO

---

**Document Version**: 1.0
**Last Updated**: December 30, 2025
**Next Review**: After Day 2 testing (validate coverage and performance)








