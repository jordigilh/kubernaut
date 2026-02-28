# Edge Case Testing Guide for Kubernaut

**Version**: 1.0
**Last Updated**: 2025-10-14
**Status**: Approved - Mandatory for all edge case tests
**Complementary To**: [TEST_STYLE_GUIDE.md](TEST_STYLE_GUIDE.md)

---

## Purpose

This guide provides comprehensive patterns and examples for implementing edge case tests across all Kubernaut services. Edge case tests validate boundary conditions, error paths, and failure scenarios that could cause production issues.

---

## Table of Contents

1. [What are Edge Cases?](#what-are-edge-cases)
2. [Edge Case Categories](#edge-case-categories)
3. [Testing Patterns](#testing-patterns)
4. [Ginkgo DescribeTable Examples](#ginkgo-describetable-examples)
5. [Anti-Flaky Patterns for Edge Cases](#anti-flaky-patterns-for-edge-cases)
6. [Service-Specific Edge Cases](#service-specific-edge-cases)
7. [Coverage Validation](#coverage-validation)

---

## 1. What are Edge Cases?

**Edge cases** are scenarios that test boundary conditions, error paths, and exceptional situations that may not occur during normal operation but could cause failures in production.

### Characteristics of Good Edge Case Tests

✅ **Test boundary conditions** (zero, max, exactly at threshold)
✅ **Test error paths** (failures, timeouts, connection loss)
✅ **Test concurrent scenarios** (race conditions, deadlocks)
✅ **Test resource exhaustion** (quotas, limits, capacity)
✅ **Test invalid inputs** (malformed data, unexpected types)
✅ **Test recovery scenarios** (retries, reconnections, graceful degradation)

### Edge Cases vs Normal Cases

| Aspect | Normal Case | Edge Case |
|--------|-------------|-----------|
| **Frequency** | Common, expected | Rare, exceptional |
| **Input** | Valid, typical | Boundary, invalid, extreme |
| **Purpose** | Validate happy path | Validate error handling |
| **Example** | Alert with 5 historical matches | Alert with zero historical matches |

---

## 2. Edge Case Categories

### Category 1: Boundary Conditions

**Definition**: Values at the extreme ends of valid ranges

**Examples**:
- Zero replicas (scale to zero)
- Maximum replicas (scale to limit)
- Empty arrays/maps
- Exactly at threshold (50% confidence)
- First/last element in collection

**Test Pattern**:
```go
DescribeTable("boundary conditions",
    func(value int, expectedBehavior string) {
        result := processValue(value)
        Expect(result).To(MatchBehavior(expectedBehavior))
    },
    Entry("zero value", 0, "special_handling"),
    Entry("minimum value", 1, "normal_processing"),
    Entry("maximum value", 10000, "capacity_limit"),
    Entry("exactly at threshold", 50, "threshold_boundary"),
)
```

---

### Category 2: Resource Exhaustion

**Definition**: System resources depleted or limits reached

**Examples**:
- Kubernetes quota exceeded
- Database connection pool exhausted
- Memory/CPU limits reached
- Disk space full
- Network bandwidth saturated

**Test Pattern**:
```go
It("should handle resource quota exceeded", func() {
    // Setup: Create ResourceQuota limiting pods to 5
    quota := createResourceQuota("test-quota", 5)
    Expect(k8sClient.Create(ctx, quota)).To(Succeed())

    // Act: Try to create 6th pod
    pod := createTestPod("pod-6")
    err := executor.ExecuteAction(ctx, createPodAction(pod))

    // Assert: Graceful failure with specific error
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("quota exceeded"))
})
```

---

### Category 3: Timing & Concurrency

**Definition**: Race conditions, timeouts, and concurrent access scenarios

**Examples**:
- Concurrent status updates
- Watch connection interrupted
- Timeout exactly at deadline
- Reconciliation loop race conditions
- Multiple goroutines updating same resource

**Test Pattern (using anti-flaky patterns)**:
```go
It("should handle concurrent status updates", func() {
    syncPoint := timing.NewSyncPoint()
    var updateCount atomic.Int32

    // Start 3 concurrent updaters
    for i := 0; i < 3; i++ {
        go func(id int) {
            defer GinkgoRecover()

            // Wait for sync point
            Expect(syncPoint.WaitForReady(ctx)).To(Succeed())

            // All update simultaneously
            err := controller.UpdateStatus(ctx, resource, fmt.Sprintf("update-%d", id))
            if err == nil {
                updateCount.Add(1)
            }
        }(i)
    }

    // Signal all to proceed
    <-syncPoint.Signal()
    syncPoint.Proceed()

    // Verify: Exactly one succeeded (optimistic locking)
    Eventually(func() int32 {
        return updateCount.Load()
    }, 2*time.Second).Should(Equal(int32(1)))
})
```

---

### Category 4: Invalid / Malformed Input

**Definition**: Data that violates format, type, or schema expectations

**Examples**:
- Malformed embedding vectors (wrong dimensions)
- Invalid image references
- Missing required fields
- Incorrect data types
- Out-of-range values

**Test Pattern**:
```go
DescribeTable("invalid input handling",
    func(input InvalidInput, expectedError string) {
        err := validator.Validate(input)

        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring(expectedError))
    },
    Entry("malformed embedding",
        InvalidInput{Embedding: []float32{1, 2}}, // Expected 384 dimensions
        "dimension mismatch"),
    Entry("invalid image reference",
        InvalidInput{Image: "invalid::image"},
        "invalid image format"),
    Entry("missing required field",
        InvalidInput{Name: ""},
        "name is required"),
)
```

---

### Category 5: External Dependency Failures

**Definition**: External services unavailable, slow, or returning errors

**Examples**:
- Database connection failure
- LLM API timeout
- Kubernetes API unavailable
- Network partition
- DNS resolution failure

**Test Pattern (with fault injection)**:
```go
It("should handle database connection failure with retry", func() {
    // Setup: Inject transient connection error
    mockDB := testutil.NewMockDatabase()
    injector := mocks.NewFaultInjector()
    injector.SetFailureRate(0.5) // 50% failure rate
    mockDB.SetFaultInjector(injector)

    // Act: Operation should eventually succeed
    timing.EventuallyWithRetry(func() error {
        return enricher.EnrichAlert(ctx, alert)
    }, 5, 1*time.Second).Should(Succeed())

    // Assert: Retry logic worked
    Expect(mockDB.GetAttemptCount()).To(BeNumerically(">", 1))
})
```

---

### Category 6: State Transitions

**Definition**: Unexpected or invalid state transitions

**Examples**:
- Phase transition from non-adjacent state
- Completed resource receiving new updates
- Deleted resource still being reconciled
- Status update on failed resource

**Test Pattern**:
```go
Context("invalid state transitions", func() {
    It("should reject transition from Pending to Completed (skipping Running)", func() {
        workflow.Status.Phase = "Pending"
        Expect(k8sClient.Status().Update(ctx, workflow)).To(Succeed())

        // Try invalid transition
        workflow.Status.Phase = "Completed" // Skipped "Running"
        err := reconciler.Reconcile(ctx, workflow)

        Expect(err).To(MatchError(ContainSubstring("invalid phase transition")))
        Expect(workflow.Status.Phase).To(Equal("Pending")) // Unchanged
    })
})
```

---

## 3. Testing Patterns

### Pattern 1: Table-Driven Edge Cases (Ginkgo DescribeTable)

**When to Use**: Multiple similar edge cases with different inputs

**Benefits**:
- ✅ Single test function for many scenarios
- ✅ Clear test matrix visibility
- ✅ Easy to add new cases
- ✅ Lower maintenance cost

**Example**:
```go
var _ = Describe("BR-SP-001: Context Enrichment Edge Cases", func() {
    var enricher *ContextEnricher

    BeforeEach(func() {
        enricher = NewContextEnricher(mockStorage, mockEmbedding)
    })

    DescribeTable("historical data variations",
        func(historicalData []Record, expectedClassification string) {
            alert := createTestAlert("deployment-failure")
            mockStorage.SetHistoricalData(historicalData)

            result, err := enricher.Enrich(ctx, alert)

            Expect(err).ToNot(HaveOccurred())
            Expect(result.Classification).To(Equal(expectedClassification))
        },
        Entry("empty history → AIRequired",
            []Record{}, "AIRequired"),
        Entry("high similarity (>0.95) → Automated",
            []Record{{Similarity: 0.97, SuccessRate: 0.9}}, "Automated"),
        Entry("low similarity (<0.70) → AIRequired",
            []Record{{Similarity: 0.65, SuccessRate: 0.9}}, "AIRequired"),
        Entry("zero success rate → AIRequired",
            []Record{{Similarity: 0.95, SuccessRate: 0.0}}, "AIRequired"),
        Entry("exactly threshold (0.80) → Automated",
            []Record{{Similarity: 0.95, SuccessRate: 0.80}}, "Automated"),
    )
})
```

---

### Pattern 2: Concurrent Edge Cases (Anti-Flaky Patterns)

**When to Use**: Testing race conditions, concurrent updates, parallel execution

**Required Tool**: `pkg/testutil/timing/anti_flaky_patterns.go`

**Example**:
```go
var _ = Describe("BR-WF-023: Parallel Execution Edge Cases", func() {
    It("should respect concurrency limit under load", func() {
        harness := parallel.NewExecutionHarness(3) // Max 3 concurrent
        ctx := context.Background()

        // Submit 10 tasks
        for i := 0; i < 10; i++ {
            taskID := fmt.Sprintf("task-%d", i)
            go func(id string) {
                defer GinkgoRecover()
                err := harness.ExecuteTask(ctx, id, 100*time.Millisecond)
                Expect(err).NotTo(HaveOccurred())
            }(taskID)
        }

        // Wait for all tasks
        Expect(harness.WaitForAllTasks(ctx, 10)).To(Succeed())

        // Verify concurrency limit never exceeded
        Expect(harness.GetMaxConcurrency()).To(BeNumerically("<=", 3))
    })
})
```

---

### Pattern 3: Timeout Edge Cases (Deadline Testing)

**When to Use**: Testing timeout boundaries, deadline enforcement

**Required Tool**: `timing.WaitForConditionWithDeadline`

**Example**:
```go
It("should timeout Job exactly at activeDeadlineSeconds", func() {
    ctx := context.Background()

    // Create Job with 5 second timeout
    ke := createKubernetesExecution("timeout-test", withTimeout(5))  // DEPRECATED - ADR-025
    Expect(k8sClient.Create(ctx, ke)).To(Succeed())

    // Wait for timeout (with buffer)
    err := timing.WaitForConditionWithDeadline(
        ctx,
        func() bool {
            updated := &kubernetesexecutionv1alpha1.KubernetesExecution{}
            k8sClient.Get(ctx, client.ObjectKeyFromObject(ke), updated)
            return updated.Status.Phase == "Failed"
        },
        500*time.Millisecond,
        10*time.Second, // Timeout + buffer
    )

    Expect(err).NotTo(HaveOccurred())

    // Verify timeout reason
    updated := &kubernetesexecutionv1alpha1.KubernetesExecution{}
    Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ke), updated)).To(Succeed())
    Expect(updated.Status.Reason).To(ContainSubstring("timeout"))
})
```

---

### Pattern 4: Resource Exhaustion Edge Cases

**When to Use**: Testing quota limits, capacity constraints

**Example**:
```go
It("should handle database connection pool exhaustion", func() {
    // Setup: Create pool with 5 connections
    pool := testutil.NewConnectionPool(5)
    enricher := NewEnricher(pool)

    // Exhaust pool
    var conns []Connection
    for i := 0; i < 5; i++ {
        conn, _ := pool.Acquire()
        conns = append(conns, conn)
    }

    // Act: 6th request should wait and retry
    done := make(chan error)
    go func() {
        _, err := enricher.EnrichAlert(ctx, alert)
        done <- err
    }()

    // Release one connection after delay
    time.Sleep(100 * time.Millisecond)
    pool.Release(conns[0])

    // Assert: Request succeeded after retry
    Eventually(done, 2*time.Second).Should(Receive(BeNil()))
})
```

---

## 4. Ginkgo DescribeTable Examples

### Example 1: Exit Code Interpretation

```go
DescribeTable("non-standard exit codes",
    func(exitCode int32, expectedStatus string, expectedReason string) {
        result := interpreter.InterpretExitCode(exitCode)

        Expect(result.Status).To(Equal(expectedStatus))
        Expect(result.Reason).To(ContainSubstring(expectedReason))
    },
    Entry("SIGKILL (137) → Killed", int32(137), "Failed", "killed by signal"),
    Entry("SIGTERM (143) → Terminated", int32(143), "Failed", "terminated by signal"),
    Entry("SIGINT (130) → Interrupted", int32(130), "Failed", "interrupted"),
    Entry("OOMKilled (255) → Out of Memory", int32(255), "Failed", "out of memory"),
    Entry("Exit 0 → Success", int32(0), "Succeeded", "completed successfully"),
)
```

---

### Example 2: Classification Thresholds

```go
DescribeTable("environment-based classification thresholds",
    func(namespace string, labels map[string]string, expectedEnv string, expectedThreshold float64) {
        alert := createAlert(namespace, labels)
        classification := classifier.Classify(alert)

        Expect(classification.Environment).To(Equal(expectedEnv))
        Expect(classification.Threshold).To(BeNumerically("==", expectedThreshold))
    },
    Entry("production namespace → 0.90 threshold",
        "prod-webapp", map[string]string{}, "production", 0.90),
    Entry("staging namespace → 0.70 threshold",
        "staging-api", map[string]string{}, "staging", 0.70),
    Entry("dev namespace → 0.60 threshold",
        "dev-test", map[string]string{}, "dev", 0.60),
    Entry("explicit label overrides namespace",
        "unknown", map[string]string{"environment": "production"}, "production", 0.90),
)
```

---

### Example 3: Rollback Action Mapping

```go
DescribeTable("rollback action mapping",
    func(originalAction, expectedRollbackAction workflowv1alpha1.ActionType) {
        step := &workflowv1alpha1.WorkflowStep{
            Name:   "test-step",
            Action: originalAction,
        }

        rollbackAction := rollbackManager.GetRollbackAction(step)
        Expect(rollbackAction).To(Equal(expectedRollbackAction))
    },
    Entry("ScaleDeployment → ScaleDeployment (original replicas)",
        workflowv1alpha1.ActionTypeScaleDeployment,
        workflowv1alpha1.ActionTypeScaleDeployment),
    Entry("UpdateImage → UpdateImage (original image)",
        workflowv1alpha1.ActionTypeUpdateImage,
        workflowv1alpha1.ActionTypeUpdateImage),
    Entry("DeletePod → No rollback (irreversible)",
        workflowv1alpha1.ActionTypeDeletePod,
        workflowv1alpha1.ActionTypeNone),
    Entry("CordonNode → UncordonNode",
        workflowv1alpha1.ActionTypeCordonNode,
        workflowv1alpha1.ActionTypeUncordonNode),
)
```

---

## 5. Anti-Flaky Patterns for Edge Cases

### When to Use Anti-Flaky Patterns

✅ **Concurrent status updates** (race conditions)
✅ **Watch-based coordination** (network interruptions)
✅ **Timeout scenarios** (exact deadline testing)
✅ **Retry logic** (transient failures)
✅ **Parallel execution** (concurrency limits)

### Pattern: SyncPoint for Race Conditions

```go
It("should handle concurrent CRD creation", func() {
    syncPoint := timing.NewSyncPoint()
    var created atomic.Int32

    // Start 3 concurrent creators
    for i := 0; i < 3; i++ {
        go func(id int) {
            defer GinkgoRecover()
            Expect(syncPoint.WaitForReady(ctx)).To(Succeed())

            ke := createKubernetesExecution(fmt.Sprintf("test-%d", id))
            if err := k8sClient.Create(ctx, ke); err == nil {
                created.Add(1)
            }
        }(i)
    }

    // Signal all to proceed simultaneously
    <-syncPoint.Signal()
    syncPoint.Proceed()

    // Wait for completion
    Eventually(func() int32 { return created.Load() }, 2*time.Second).Should(Equal(int32(3)))
})
```

---

### Pattern: Barrier for N-Way Synchronization

```go
It("should execute parallel steps simultaneously", func() {
    barrier := timing.NewBarrier(5) // 5 steps
    var startTimes []time.Time
    var mu sync.Mutex

    for i := 0; i < 5; i++ {
        go func(id int) {
            defer GinkgoRecover()

            // All wait at barrier
            Expect(barrier.Wait(ctx)).To(Succeed())

            // Record start time (all should be very close)
            mu.Lock()
            startTimes = append(startTimes, time.Now())
            mu.Unlock()
        }(i)
    }

    Eventually(func() int {
        mu.Lock()
        defer mu.Unlock()
        return len(startTimes)
    }, 2*time.Second).Should(Equal(5))

    // Verify all started within 100ms window
    minTime := startTimes[0]
    maxTime := startTimes[0]
    for _, t := range startTimes {
        if t.Before(minTime) {
            minTime = t
        }
        if t.After(maxTime) {
            maxTime = t
        }
    }
    Expect(maxTime.Sub(minTime)).To(BeNumerically("<", 100*time.Millisecond))
})
```

---

## 6. Service-Specific Edge Cases

### Remediation Processor Edge Cases

**Common Scenarios**:
- Empty historical context (zero matches)
- Malformed embedding vectors
- pgvector query timeout
- Zero historical attempts (divide-by-zero)
- Ambiguous classification (exactly 50% confidence)
- Concurrent phase transitions

**Example**:
```go
It("should handle empty historical context gracefully", func() {
    // No historical data available
    mockStorage.SetHistoricalData([]Record{})

    alert := createTestAlert("novel-signal")
    result, err := enricher.Enrich(ctx, alert)

    // Should not fail, should classify as AIRequired
    Expect(err).NotTo(HaveOccurred())
    Expect(result.Classification).To(Equal("AIRequired"))
    Expect(result.HistoricalMatches).To(BeEmpty())
    Expect(result.Reason).To(ContainSubstring("no historical data"))
})
```

---

### Workflow Execution Edge Cases

**Common Scenarios**:
- Circular dependency detection
- Orphaned KubernetesExecution (DEPRECATED - ADR-025) CRDs
- Watch connection interrupted
- Concurrent step completion
- Rollback cascade failures

**Example**:
```go
It("should detect circular dependencies in workflow", func() {
    workflow := &workflowv1alpha1.WorkflowExecution{
        Spec: workflowv1alpha1.WorkflowExecutionSpec{
            Definition: workflowv1alpha1.WorkflowDefinition{
                Steps: []workflowv1alpha1.WorkflowStep{
                    {Name: "step-1", DependsOn: []string{"step-2"}},
                    {Name: "step-2", DependsOn: []string{"step-1"}}, // Circular!
                },
            },
        },
    }

    err := orchestrator.ValidateWorkflow(workflow)
    Expect(err).To(MatchError(ContainSubstring("circular dependency")))
})
```

---

### Kubernetes Executor Edge Cases

**Common Scenarios**:
- Job timeout exactly at deadline
- Pod eviction (node pressure)
- RBAC permission denied
- Rego policy compilation error
- Non-standard exit codes (SIGKILL, SIGTERM)

**Example**:
```go
It("should handle RBAC permission denied gracefully", func() {
    // Create ServiceAccount without required permissions
    sa := createServiceAccount("insufficient-sa", []string{}) // No permissions

    ke := createKubernetesExecution("rbac-test")
    ke.Spec.ServiceAccountName = "insufficient-sa"
    Expect(k8sClient.Create(ctx, ke)).To(Succeed())

    // Wait for failure
    timing.EventuallyWithRetry(func() error {
        updated := &kubernetesexecutionv1alpha1.KubernetesExecution{}
        if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ke), updated); err != nil {
            return err
        }
        if updated.Status.Phase != "Failed" {
            return fmt.Errorf("expected Failed, got %s", updated.Status.Phase)
        }
        return nil
    }, 5, 1*time.Second).Should(Succeed())

    // Verify specific error
    updated := &kubernetesexecutionv1alpha1.KubernetesExecution{}
    Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ke), updated)).To(Succeed())
    Expect(updated.Status.Reason).To(ContainSubstring("permission denied"))
})
```

---

## 7. Coverage Validation

### Automated Coverage Check

Use the edge case coverage validator:

```bash
./test/scripts/validate_edge_case_coverage.sh remediationprocessor
./test/scripts/validate_edge_case_coverage.sh workflowexecution
./test/scripts/validate_edge_case_coverage.sh kubernetesexecutor
```

**Output**:
```
================================================
Edge Case Coverage Report: remediationprocessor
================================================

BR-SP-001: Historical Alert Enrichment
  ✅ Empty historical context
  ✅ Malformed embedding vectors
  ✅ pgvector query timeout
  ✅ Zero historical attempts
  Coverage: 4/4

Summary:
Total Edge Cases: 12
Covered: 12
Missing: 0
✅ 100% Coverage - All edge cases tested!
```

---

### Manual Review Checklist

Before merging edge case tests:

- [ ] All edge cases documented in BR Coverage Matrix
- [ ] Each edge case has explicit test name
- [ ] Table-driven tests used where appropriate
- [ ] Anti-flaky patterns used for concurrent tests
- [ ] Tests follow [TEST_STYLE_GUIDE.md](TEST_STYLE_GUIDE.md) naming
- [ ] Coverage validator passes (100%)
- [ ] No flaky tests (>99% pass rate over 50 runs)
- [ ] Edge cases reference specific BR-XXX-YYY-EC# identifiers

---

## Quick Reference Card

### Edge Case Test Checklist

- [ ] Boundary conditions (zero, max, threshold)
- [ ] Resource exhaustion (quota, pool, capacity)
- [ ] Timing issues (timeout, race, deadline)
- [ ] Invalid input (malformed, missing, wrong type)
- [ ] External failures (connection, timeout, unavailable)
- [ ] State transitions (invalid, unexpected)

### Anti-Flaky Pattern Selection

| Scenario | Use This Pattern |
|----------|-----------------|
| Race condition | `SyncPoint` |
| N-way coordination | `Barrier` |
| Retry with backoff | `EventuallyWithRetry` |
| Timeout testing | `WaitForConditionWithDeadline` |
| Parallel execution | `ConcurrentExecutor` or `ExecutionHarness` |
| Transient failures | `RetryWithBackoff` |

### Common Assertions for Edge Cases

```go
// Errors
Expect(err).To(HaveOccurred())
Expect(err.Error()).To(ContainSubstring("expected error"))
Expect(err).To(MatchError(ContainSubstring("partial match")))

// Boundary values
Expect(value).To(BeNumerically(">=", 0))
Expect(value).To(BeNumerically("<=", max))
Expect(value).To(BeNumerically("==", threshold))

// Collections
Expect(items).To(BeEmpty())
Expect(items).To(HaveLen(1))
Expect(items).To(ContainElement(expectedItem))

// Status
Expect(status.Phase).To(Equal("Failed"))
Expect(status.Reason).To(ContainSubstring("timeout"))
```

---

**Version**: 1.0
**Last Updated**: 2025-10-14
**Compliance**: Mandatory for all edge case tests
**Review Cycle**: Quarterly

