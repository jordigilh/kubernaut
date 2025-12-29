# BR-WE-012: Exponential Backoff Cooldown - Test Coverage Plan

**Date**: December 19, 2025
**Business Requirement**: BR-WE-012 (Exponential Backoff Cooldown for Pre-Execution Failures)
**Status**: ⚠️ **CRITICAL GAP** - Implementation exists, ZERO test coverage
**Priority**: **P0 (CRITICAL)** - Must complete before v1.0
**Effort**: 1 day

---

## 🚨 **Problem Statement**

**Current State**: BR-WE-012 is **IMPLEMENTED** but has **ZERO test coverage** across all 3 tiers:
- ❌ Unit tests: 0
- ❌ Integration tests: 0
- ❌ E2E tests: 0

**Business Impact**:
- Exponential backoff logic is **UNTESTED** and could have bugs
- Pre-execution failure handling is **UNVALIDATED**
- Execution failure blocking is **UNVERIFIED**
- Consecutive failure counter is **UNTESTED**

**Risk**: High - This is production-critical failure handling logic with no validation.

---

## 📋 **BR-WE-012 Requirements Summary**

### **Core Functionality**

**Pre-Execution Failures** (Infrastructure Issues):
- ✅ Apply exponential backoff: `BaseCooldown × 2^(failures-1)` (capped at MaxCooldown)
- ✅ Track `ConsecutiveFailures` counter per target resource
- ✅ After 5 consecutive pre-execution failures → Mark Skipped with `ExhaustedRetries`
- ✅ Success resets failure counter to 0

**Execution Failures** (Workflow Ran and Failed):
- ✅ **NO retry** - Mark Skipped with `PreviousExecutionFailed`
- ✅ Block ALL future executions until manual clearance

**Backoff Configuration**:
- `BaseCooldownPeriod`: 1 minute (initial cooldown)
- `MaxCooldownPeriod`: 10 minutes (cap)
- `MaxConsecutiveFailures`: 5 (before ExhaustedRetries)
- `Multiplier`: 2.0 (power-of-2 exponential)
- `JitterPercent`: 10 (±10% variance)

### **Failure Categories**

**Pre-Execution Failures** (Apply Backoff):
- `ImagePullBackOff`: Container image not available
- `Forbidden`: Permission denied
- `ConfigurationError`: Invalid workflow configuration
- `ValidationError`: Validation failures

**Execution Failures** (Block Retries):
- `TaskFailed`: Workflow task failed during execution
- `OOMKilled`: Container out of memory
- `DeadlineExceeded`: Timeout reached

---

## 🎯 **Test Coverage Strategy**

### **Defense-in-Depth Approach**

| Tier | Coverage Target | Focus | Test Count |
|------|----------------|-------|------------|
| **Unit** | 70%+ | Backoff calculation, counter logic, failure categorization | 8 tests |
| **Integration** | 50%+ | Controller behavior with real K8s API, multi-failure progression | 5 tests |
| **E2E** | 10%+ | Full workflow with real Tekton, infrastructure failures | 2 tests |

**Total**: 15 tests (8 unit + 5 integration + 2 E2E)

---

## 🧪 **Tier 1: Unit Tests (8 tests - 4 hours)**

### **Target**: `pkg/shared/backoff/backoff_test.go` + `internal/controller/workflowexecution/failure_analysis_test.go`

### **Test Group 1: Backoff Calculation Logic (4 tests)**

**File**: `pkg/shared/backoff/backoff_test.go` (already exists)

#### **Test 1.1: Standard Exponential Backoff Sequence**
```go
var _ = Describe("Exponential Backoff Calculation", func() {
    Context("when using standard configuration (BR-WE-012)", func() {
        It("should calculate correct backoff sequence for consecutive failures", func() {
            config := backoff.Config{
                BasePeriod:    1 * time.Minute,
                MaxPeriod:     10 * time.Minute,
                Multiplier:    2.0,
                JitterPercent: 0, // No jitter for deterministic testing
            }

            // BR-WE-012: First failure = 1min, doubles each time
            Expect(config.Calculate(1)).To(Equal(1 * time.Minute))  // 2^0 = 1x
            Expect(config.Calculate(2)).To(Equal(2 * time.Minute))  // 2^1 = 2x
            Expect(config.Calculate(3)).To(Equal(4 * time.Minute))  // 2^2 = 4x
            Expect(config.Calculate(4)).To(Equal(8 * time.Minute))  // 2^3 = 8x
            Expect(config.Calculate(5)).To(Equal(10 * time.Minute)) // Capped at MaxPeriod
            Expect(config.Calculate(6)).To(Equal(10 * time.Minute)) // Stays at cap
        })
    })
})
```

**Validates**: Exponential growth formula, MaxPeriod capping

---

#### **Test 1.2: Jitter Distribution**
```go
It("should apply ±10% jitter to prevent thundering herd", func() {
    config := backoff.Config{
        BasePeriod:    1 * time.Minute,
        MaxPeriod:     10 * time.Minute,
        Multiplier:    2.0,
        JitterPercent: 10,
    }

    // Run 100 iterations to validate jitter distribution
    results := make([]time.Duration, 100)
    for i := 0; i < 100; i++ {
        results[i] = config.Calculate(2) // 2 minutes base
    }

    // Validate jitter bounds: 2min ±10% = 108s-132s
    for _, d := range results {
        Expect(d).To(BeNumerically(">=", 108*time.Second))
        Expect(d).To(BeNumerically("<=", 132*time.Second))
    }

    // Validate distribution (not all values should be identical)
    unique := make(map[time.Duration]bool)
    for _, d := range results {
        unique[d] = true
    }
    Expect(len(unique)).To(BeNumerically(">", 10)) // At least 10 different values
})
```

**Validates**: Jitter prevents thundering herd, stays within bounds

---

#### **Test 1.3: Edge Cases**
```go
It("should handle edge cases correctly", func() {
    config := backoff.Config{
        BasePeriod:    1 * time.Minute,
        MaxPeriod:     10 * time.Minute,
        Multiplier:    2.0,
        JitterPercent: 0,
    }

    // Zero attempts should return BasePeriod
    Expect(config.Calculate(0)).To(Equal(1 * time.Minute))

    // Negative attempts should return BasePeriod
    Expect(config.Calculate(-1)).To(Equal(1 * time.Minute))

    // Very high attempts should cap at MaxPeriod
    Expect(config.Calculate(100)).To(Equal(10 * time.Minute))
})
```

**Validates**: Edge case handling, defensive programming

---

#### **Test 1.4: Zero Configuration**
```go
It("should handle zero configuration gracefully", func() {
    config := backoff.Config{
        BasePeriod:    0,
        MaxPeriod:     0,
        Multiplier:    0, // Should default to 2.0
        JitterPercent: 0,
    }

    // Zero BasePeriod should return 0
    Expect(config.Calculate(5)).To(Equal(time.Duration(0)))
})
```

**Validates**: Zero configuration handling, defaults

---

### **Test Group 2: Failure Categorization (2 tests)**

**File**: `internal/controller/workflowexecution/failure_analysis_test.go` (new)

#### **Test 2.1: Pre-Execution Failure Detection**
```go
var _ = Describe("Failure Analysis (BR-WE-012)", func() {
    Context("when categorizing failures", func() {
        It("should identify pre-execution failures correctly", func() {
            preExecutionFailures := []string{
                "ImagePullBackOff",
                "Forbidden",
                "ConfigurationError",
                "ValidationError",
                "InvalidWorkflowDefinition",
            }

            for _, reason := range preExecutionFailures {
                // Test that these are classified as wasExecutionFailure=false
                details := &workflowexecutionv1alpha1.FailureDetails{
                    Reason:              reason,
                    Message:             "test message",
                    WasExecutionFailure: false,
                }

                Expect(details.WasExecutionFailure).To(BeFalse(),
                    "Failure reason %s should be pre-execution", reason)
            }
        })
    })
})
```

**Validates**: Pre-execution failure categorization

---

#### **Test 2.2: Execution Failure Detection**
```go
It("should identify execution failures correctly", func() {
    executionFailures := []string{
        "TaskFailed",
        "OOMKilled",
        "DeadlineExceeded",
    }

    for _, reason := range executionFailures {
        // Test that these are classified as wasExecutionFailure=true
        details := &workflowexecutionv1alpha1.FailureDetails{
            Reason:              reason,
            Message:             "test message",
            WasExecutionFailure: true,
        }

        Expect(details.WasExecutionFailure).To(BeTrue(),
            "Failure reason %s should be execution failure", reason)
    }
})
```

**Validates**: Execution failure categorization

---

### **Test Group 3: Counter Logic (2 tests)**

**File**: `test/unit/workflowexecution/consecutive_failures_test.go` (new)

#### **Test 3.1: Counter Increment and Reset**
```go
var _ = Describe("Consecutive Failures Counter (BR-WE-012)", func() {
    Context("when tracking failure count", func() {
        It("should increment counter for pre-execution failures", func() {
            wfe := &workflowexecutionv1alpha1.WorkflowExecution{
                Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
                    ConsecutiveFailures: 0,
                },
            }

            // Simulate 3 pre-execution failures
            wfe.Status.ConsecutiveFailures++
            Expect(wfe.Status.ConsecutiveFailures).To(Equal(int32(1)))

            wfe.Status.ConsecutiveFailures++
            Expect(wfe.Status.ConsecutiveFailures).To(Equal(int32(2)))

            wfe.Status.ConsecutiveFailures++
            Expect(wfe.Status.ConsecutiveFailures).To(Equal(int32(3)))
        })

        It("should reset counter to 0 on successful completion", func() {
            wfe := &workflowexecutionv1alpha1.WorkflowExecution{
                Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
                    ConsecutiveFailures: 5,
                },
            }

            // Success resets counter
            wfe.Status.ConsecutiveFailures = 0
            Expect(wfe.Status.ConsecutiveFailures).To(Equal(int32(0)))
        })
    })
})
```

**Validates**: Counter increment, reset on success

---

#### **Test 3.2: ExhaustedRetries Threshold**
```go
It("should trigger ExhaustedRetries after MaxConsecutiveFailures", func() {
    maxFailures := int32(5)
    wfe := &workflowexecutionv1alpha1.WorkflowExecution{
        Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
            ConsecutiveFailures: maxFailures,
        },
    }

    // After 5 failures, should trigger ExhaustedRetries
    Expect(wfe.Status.ConsecutiveFailures).To(Equal(maxFailures))
    Expect(wfe.Status.ConsecutiveFailures).To(BeNumerically(">=", maxFailures))
})
```

**Validates**: ExhaustedRetries threshold detection

---

## 🔗 **Tier 2: Integration Tests (5 tests - 3 hours)**

### **Target**: `test/integration/workflowexecution/reconciler_test.go`

### **Test Group 4: Multi-Failure Progression (3 tests)**

#### **Test 4.1: Exponential Backoff Escalation**
```go
var _ = Describe("WorkflowExecution Exponential Backoff (BR-WE-012)", func() {
    Context("when multiple pre-execution failures occur", func() {
        It("should escalate cooldown exponentially", func() {
            ctx := context.Background()

            // Create WorkflowExecution that will fail (invalid image)
            wfe := &workflowexecutionv1alpha1.WorkflowExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-exponential-backoff",
                    Namespace: "default",
                },
                Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
                    WorkflowRef: workflowexecutionv1alpha1.WorkflowReference{
                        WorkflowID:     "test-workflow",
                        Version:        "v1",
                        ContainerImage: "invalid-image:does-not-exist",
                    },
                    TargetResource: "default/deployment/test-app",
                },
            }

            Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

            // Wait for first failure
            Eventually(func() int32 {
                Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), wfe)).To(Succeed())
                return wfe.Status.ConsecutiveFailures
            }, timeout, interval).Should(Equal(int32(1)))

            // Validate first backoff: 1 minute (±10% jitter)
            Expect(wfe.Status.NextAllowedExecution).ToNot(BeNil())
            firstBackoff := time.Until(wfe.Status.NextAllowedExecution.Time)
            Expect(firstBackoff).To(BeNumerically(">=", 54*time.Second))  // 1min - 10%
            Expect(firstBackoff).To(BeNumerically("<=", 66*time.Second))  // 1min + 10%

            // Delete and recreate to trigger second failure
            Expect(k8sClient.Delete(ctx, wfe)).To(Succeed())
            wfe.ResourceVersion = "" // Reset for recreation
            Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

            // Wait for second failure
            Eventually(func() int32 {
                Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), wfe)).To(Succeed())
                return wfe.Status.ConsecutiveFailures
            }, timeout, interval).Should(Equal(int32(2)))

            // Validate second backoff: 2 minutes (±10% jitter)
            Expect(wfe.Status.NextAllowedExecution).ToNot(BeNil())
            secondBackoff := time.Until(wfe.Status.NextAllowedExecution.Time)
            Expect(secondBackoff).To(BeNumerically(">=", 108*time.Second)) // 2min - 10%
            Expect(secondBackoff).To(BeNumerically("<=", 132*time.Second)) // 2min + 10%

            // Validate escalation: second > first
            Expect(secondBackoff).To(BeNumerically(">", firstBackoff))
        })
    })
})
```

**Validates**: Exponential escalation, jitter application, NextAllowedExecution persistence

---

#### **Test 4.2: Success Resets Counter**
```go
It("should reset ConsecutiveFailures counter on successful completion", func() {
    ctx := context.Background()

    // Create WFE with pre-existing failure count
    wfe := &workflowexecutionv1alpha1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-reset-counter",
            Namespace: "default",
        },
        Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
            WorkflowRef: workflowexecutionv1alpha1.WorkflowReference{
                WorkflowID:     "test-workflow",
                Version:        "v1",
                ContainerImage: testWorkflowImage, // Valid image
            },
            TargetResource: "default/deployment/test-app",
        },
        Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
            ConsecutiveFailures: 3, // Pre-existing failures
        },
    }

    Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

    // Create successful PipelineRun
    pr := &tektonv1.PipelineRun{
        ObjectMeta: metav1.ObjectMeta{
            Name:      wfe.Name + "-run",
            Namespace: reconciler.ExecutionNamespace,
        },
        Status: tektonv1.PipelineRunStatus{
            Status: duckv1.Status{
                Conditions: []apis.Condition{
                    {
                        Type:   apis.ConditionSucceeded,
                        Status: corev1.ConditionTrue,
                    },
                },
            },
        },
    }
    Expect(k8sClient.Create(ctx, pr)).To(Succeed())

    // Wait for WFE to complete successfully
    Eventually(func() string {
        Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), wfe)).To(Succeed())
        return wfe.Status.Phase
    }, timeout, interval).Should(Equal(workflowexecutionv1alpha1.WorkflowExecutionCompleted))

    // Validate counter reset to 0
    Expect(wfe.Status.ConsecutiveFailures).To(Equal(int32(0)))
})
```

**Validates**: Counter reset on success

---

#### **Test 4.3: ExhaustedRetries After Max Failures**
```go
It("should mark Skipped with ExhaustedRetries after 5 consecutive failures", func() {
    ctx := context.Background()
    maxFailures := int32(5)

    // Create WFE that will repeatedly fail
    wfe := &workflowexecutionv1alpha1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-exhausted-retries",
            Namespace: "default",
        },
        Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
            WorkflowRef: workflowexecutionv1alpha1.WorkflowReference{
                WorkflowID:     "test-workflow",
                Version:        "v1",
                ContainerImage: "invalid-image:does-not-exist",
            },
            TargetResource: "default/deployment/test-app",
        },
    }

    // Simulate 5 consecutive failures by repeatedly creating/deleting
    for i := int32(1); i <= maxFailures; i++ {
        Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

        // Wait for failure
        Eventually(func() int32 {
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), wfe)).To(Succeed())
            return wfe.Status.ConsecutiveFailures
        }, timeout, interval).Should(Equal(i))

        if i < maxFailures {
            Expect(k8sClient.Delete(ctx, wfe)).To(Succeed())
            wfe.ResourceVersion = ""
        }
    }

    // After 5 failures, should be marked Skipped with ExhaustedRetries
    Eventually(func() string {
        Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), wfe)).To(Succeed())
        return wfe.Status.Phase
    }, timeout, interval).Should(Equal(workflowexecutionv1alpha1.WorkflowExecutionSkipped))

    // Validate skip reason
    Expect(wfe.Status.FailureDetails).ToNot(BeNil())
    Expect(wfe.Status.FailureDetails.Reason).To(Equal("ExhaustedRetries"))
    Expect(wfe.Status.ConsecutiveFailures).To(Equal(maxFailures))
})
```

**Validates**: ExhaustedRetries behavior after max failures

---

### **Test Group 5: Execution Failure Blocking (2 tests)**

#### **Test 5.1: Execution Failure Does Not Increment Counter**
```go
Context("when execution failures occur (wasExecutionFailure=true)", func() {
    It("should NOT increment ConsecutiveFailures counter", func() {
        ctx := context.Background()

        // Create WFE with execution failure
        wfe := &workflowexecutionv1alpha1.WorkflowExecution{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "test-execution-failure-no-increment",
                Namespace: "default",
            },
            Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
                WorkflowRef: workflowexecutionv1alpha1.WorkflowReference{
                    WorkflowID:     "test-workflow",
                    Version:        "v1",
                    ContainerImage: testWorkflowImage,
                },
                TargetResource: "default/deployment/test-app",
            },
        }

        Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

        // Create failed PipelineRun (TaskFailed = execution failure)
        pr := &tektonv1.PipelineRun{
            ObjectMeta: metav1.ObjectMeta{
                Name:      wfe.Name + "-run",
                Namespace: reconciler.ExecutionNamespace,
            },
            Status: tektonv1.PipelineRunStatus{
                Status: duckv1.Status{
                    Conditions: []apis.Condition{
                        {
                            Type:    apis.ConditionSucceeded,
                            Status:  corev1.ConditionFalse,
                            Reason:  "TaskFailed",
                            Message: "Workflow task failed",
                        },
                    },
                },
            },
        }
        Expect(k8sClient.Create(ctx, pr)).To(Succeed())

        // Wait for WFE to fail
        Eventually(func() string {
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), wfe)).To(Succeed())
            return wfe.Status.Phase
        }, timeout, interval).Should(Equal(workflowexecutionv1alpha1.WorkflowExecutionFailed))

        // Validate counter did NOT increment (execution failure)
        Expect(wfe.Status.ConsecutiveFailures).To(Equal(int32(0)))

        // Validate wasExecutionFailure flag
        Expect(wfe.Status.FailureDetails).ToNot(BeNil())
        Expect(wfe.Status.FailureDetails.WasExecutionFailure).To(BeTrue())
    })
})
```

**Validates**: Execution failures don't increment counter

---

#### **Test 5.2: Execution Failure Blocks Future Retries**
```go
It("should block ALL future executions after execution failure", func() {
    ctx := context.Background()

    // Create first WFE with execution failure
    wfe1 := &workflowexecutionv1alpha1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-execution-failure-block-1",
            Namespace: "default",
        },
        Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
            WorkflowRef: workflowexecutionv1alpha1.WorkflowReference{
                WorkflowID:     "test-workflow",
                Version:        "v1",
                ContainerImage: testWorkflowImage,
            },
            TargetResource: "default/deployment/test-app",
        },
    }

    Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())

    // Create failed PipelineRun
    pr := &tektonv1.PipelineRun{
        ObjectMeta: metav1.ObjectMeta{
            Name:      wfe1.Name + "-run",
            Namespace: reconciler.ExecutionNamespace,
        },
        Status: tektonv1.PipelineRunStatus{
            Status: duckv1.Status{
                Conditions: []apis.Condition{
                    {
                        Type:    apis.ConditionSucceeded,
                        Status:  corev1.ConditionFalse,
                        Reason:  "TaskFailed",
                        Message: "Workflow task failed",
                    },
                },
            },
        },
    }
    Expect(k8sClient.Create(ctx, pr)).To(Succeed())

    // Wait for WFE to fail
    Eventually(func() string {
        Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe1), wfe1)).To(Succeed())
        return wfe1.Status.Phase
    }, timeout, interval).Should(Equal(workflowexecutionv1alpha1.WorkflowExecutionFailed))

    // Try to create second WFE for same target resource
    wfe2 := &workflowexecutionv1alpha1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-execution-failure-block-2",
            Namespace: "default",
        },
        Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
            WorkflowRef: workflowexecutionv1alpha1.WorkflowReference{
                WorkflowID:     "test-workflow",
                Version:        "v1",
                ContainerImage: testWorkflowImage,
            },
            TargetResource: "default/deployment/test-app", // Same target resource
        },
    }

    Expect(k8sClient.Create(ctx, wfe2)).To(Succeed())

    // Wait for WFE2 to be Skipped (blocked by previous execution failure)
    Eventually(func() string {
        Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe2), wfe2)).To(Succeed())
        return wfe2.Status.Phase
    }, timeout, interval).Should(Equal(workflowexecutionv1alpha1.WorkflowExecutionSkipped))

    // Validate skip reason
    Expect(wfe2.Status.FailureDetails).ToNot(BeNil())
    Expect(wfe2.Status.FailureDetails.Reason).To(Equal("PreviousExecutionFailed"))
})
```

**Validates**: PreviousExecutionFailed blocking

---

## 🌐 **Tier 3: E2E Tests (2 tests - 1 hour)**

### **Target**: `test/e2e/workflowexecution/workflow_execution_test.go`

### **Test Group 6: Real Tekton Integration (2 tests)**

#### **Test 6.1: Full Backoff Sequence with Real Tekton**
```go
var _ = Describe("WorkflowExecution E2E - Exponential Backoff (BR-WE-012)", func() {
    Context("when pre-execution failures occur with real Tekton", func() {
        It("should apply exponential backoff with real infrastructure", func() {
            ctx := context.Background()

            // Create WorkflowExecution with invalid image (will fail to pull)
            wfe := &workflowexecutionv1alpha1.WorkflowExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "e2e-backoff-test",
                    Namespace: testNamespace,
                },
                Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
                    WorkflowRef: workflowexecutionv1alpha1.WorkflowReference{
                        WorkflowID:     "test-workflow",
                        Version:        "v1",
                        ContainerImage: "invalid-registry.io/nonexistent/image:v1",
                    },
                    TargetResource: testNamespace + "/deployment/test-app",
                },
            }

            Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

            // Wait for first failure
            Eventually(func() int32 {
                Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), wfe)).To(Succeed())
                return wfe.Status.ConsecutiveFailures
            }, e2eTimeout, e2eInterval).Should(Equal(int32(1)))

            // Validate first backoff is set
            Expect(wfe.Status.NextAllowedExecution).ToNot(BeNil())
            Expect(wfe.Status.FailureDetails).ToNot(BeNil())
            Expect(wfe.Status.FailureDetails.WasExecutionFailure).To(BeFalse())

            // Validate metrics (if available)
            // workflowexecution_consecutive_failures gauge should = 1
        })
    })
})
```

**Validates**: Real Tekton PipelineRun failure handling, backoff application

---

#### **Test 6.2: Recovery After Infrastructure Fix**
```go
It("should reset counter and succeed after infrastructure is fixed", func() {
    ctx := context.Background()

    // Phase 1: Fail with invalid image
    wfe := &workflowexecutionv1alpha1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "e2e-backoff-recovery",
            Namespace: testNamespace,
        },
        Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
            WorkflowRef: workflowexecutionv1alpha1.WorkflowReference{
                WorkflowID:     "test-workflow",
                Version:        "v1",
                ContainerImage: "invalid-image:does-not-exist",
            },
            TargetResource: testNamespace + "/deployment/test-app",
        },
    }

    Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

    // Wait for failure
    Eventually(func() int32 {
        Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), wfe)).To(Succeed())
        return wfe.Status.ConsecutiveFailures
    }, e2eTimeout, e2eInterval).Should(BeNumerically(">", 0))

    failureCount := wfe.Status.ConsecutiveFailures
    Expect(failureCount).To(BeNumerically(">", 0))

    // Phase 2: Fix infrastructure by updating to valid image
    wfe.Spec.WorkflowRef.ContainerImage = realWorkflowImage
    Expect(k8sClient.Update(ctx, wfe)).To(Succeed())

    // Wait for success
    Eventually(func() string {
        Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), wfe)).To(Succeed())
        return wfe.Status.Phase
    }, e2eTimeout, e2eInterval).Should(Equal(workflowexecutionv1alpha1.WorkflowExecutionCompleted))

    // Validate counter reset to 0 after success
    Expect(wfe.Status.ConsecutiveFailures).To(Equal(int32(0)))
    Expect(wfe.Status.NextAllowedExecution).To(BeNil()) // Cleared on success
})
```

**Validates**: Recovery scenario, counter reset, real workflow success

---

## 📅 **Implementation Timeline (1 Day)**

| Time Block | Task | Deliverable |
|------------|------|-------------|
| **Morning (4h)** | Unit tests | 8 unit tests passing |
| **Afternoon (3h)** | Integration tests | 5 integration tests passing |
| **Late Afternoon (1h)** | E2E tests | 2 E2E tests passing |
| **Total** | **8 hours** | **15 tests passing** |

---

## 📊 **Success Criteria**

### **Test Passing**
- ✅ 8 unit tests passing (100% of planned)
- ✅ 5 integration tests passing (100% of planned)
- ✅ 2 E2E tests passing (100% of planned)
- ✅ All tests stable and deterministic

### **Coverage Validation**
- ✅ `pkg/shared/backoff/backoff.go`: 100% coverage
- ✅ Exponential calculation logic: 100% coverage
- ✅ Failure categorization: 100% coverage
- ✅ Counter increment/reset: 100% coverage
- ✅ ExhaustedRetries threshold: 100% coverage

### **Business Requirement Validation**
- ✅ Pre-execution failures apply exponential backoff
- ✅ Execution failures block future retries
- ✅ Success resets consecutive failure counter
- ✅ 5 consecutive failures trigger ExhaustedRetries
- ✅ Backoff survives controller restart (status persistence)

---

## 🔗 **Files to Create/Modify**

### **New Files** (3)
1. `test/unit/workflowexecution/consecutive_failures_test.go`
2. `internal/controller/workflowexecution/failure_analysis_test.go`
3. (E2E tests in existing file)

### **Existing Files to Modify** (3)
1. `pkg/shared/backoff/backoff_test.go` - Add 4 tests
2. `test/integration/workflowexecution/reconciler_test.go` - Add 5 tests
3. `test/e2e/workflowexecution/workflow_execution_test.go` - Add 2 tests

---

## 🎯 **Expected BR-WE-012 Coverage After Implementation**

| Tier | Before | After | Target |
|------|--------|-------|--------|
| **Unit** | ❌ 0 tests | ✅ 8 tests | 70%+ ✅ |
| **Integration** | ❌ 0 tests | ✅ 5 tests | 50%+ ✅ |
| **E2E** | ❌ 0 tests | ✅ 2 tests | 10%+ ✅ |
| **Total** | ❌ 0% | ✅ 100% | ✅ Complete |

---

## 📚 **References**

- [BR-WE-012 Business Requirement](../services/crd-controllers/03-workflowexecution/BUSINESS_REQUIREMENTS.md#br-we-012-exponential-backoff-cooldown-pre-execution-failures-only)
- [DD-WE-004: Exponential Backoff](../architecture/decisions/DD-WE-004-exponential-backoff.md)
- [pkg/shared/backoff Implementation](../../pkg/shared/backoff/backoff.go)
- [Failure Analysis Implementation](../../internal/controller/workflowexecution/failure_analysis.go)
- [WE Controller Implementation](../../internal/controller/workflowexecution/workflowexecution_controller.go)

---

**Document Status**: ✅ **READY FOR IMPLEMENTATION**
**Priority**: **P0 (CRITICAL)** - Must complete before v1.0
**Effort**: 1 day (8 hours)
**Next Step**: Begin unit test implementation


