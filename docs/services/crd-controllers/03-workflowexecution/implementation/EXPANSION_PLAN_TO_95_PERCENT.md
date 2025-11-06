# Workflow Execution - Expansion Plan to 95% Confidence

**Current Status**: 1,104 lines, 70% confidence
**Target Status**: 5,000 lines, 95% confidence
**Gap**: +3,896 lines
**Effort**: 10-13 hours (Days 2-3 already expanded!)

---

## Current State Analysis

### Existing Content (1,104 lines)
- ✅ Day 1 outline (foundation)
- ✅ **Day 2 ALREADY EXPANDED** ✅ (DAY_02_EXPANDED.md, 800 lines) - Dependency resolution
- ✅ **Day 3 ALREADY EXPANDED** ✅ (DAY_03_EXPANDED.md, 950 lines) - Parallel execution
- ✅ Days 4-12 brief outlines (~50-100 lines each)
- ✅ Basic BR Coverage Matrix (180 lines, needs expansion)
- ⚠️ Missing: Day 5 expansion, integration tests, EOD docs, error philosophy

### What's Missing vs Notification Standard
1. **APDC Phase Expansions**: 2/3 complete (Day 2 ✅, Day 3 ✅, Day 5 needed)
2. **Integration Tests**: 0/3 complete tests
3. **EOD Documentation**: 0/3 milestone docs
4. **Error Handling Philosophy**: 0/1 document
5. **Table-Driven Test Examples**: Some in Days 2-3, need more
6. **Production Deployment**: Brief only

### Advantage: 1,750 Lines Already Complete!
- Day 2 Expanded: Kahn's algorithm, controller integration, metrics
- Day 3 Expanded: Parent-child CRD pattern, watch-based coordination, concurrency

---

## Expansion Phase 1: APDC Day Expansion (+850 lines)

### Day 5: Rollback Testing (~850 lines)

**Location**: After current Day 5 outline, replace with full expansion

**Structure**:
```markdown
## 📅 Day 5: Rollback Logic and Testing (8h)

### ANALYSIS Phase (1h)
- Search existing rollback patterns in workflow engines
- Review Kubernetes Deployment rollback mechanisms
- Map BR-REMEDIATION-013 (Rollback on Failure) requirements
- Identify dependencies (KubernetesExecution controller)

### PLAN Phase (1h)
- TDD Strategy: Unit tests for rollback logic (70%), Integration for cascade (>50%)
- Integration points: KubernetesExecution status watching, rollback triggers
- Success criteria: Rollback cascade working, no orphaned resources
- Timeline: RED (2h) → GREEN (3h) → REFACTOR (2h)

### DO-RED: Rollback Tests (2h)
**File**: `test/unit/workflowexecution/rollback_test.go`
**BR Coverage**: BR-REMEDIATION-013, BR-REMEDIATION-014

[~350 lines of complete Ginkgo test code]
```go
var _ = Describe("BR-REMEDIATION-013: Rollback Logic", func() {
    var (
        rollbackManager *rollback.Manager
        workflow        *workflowv1alpha1.WorkflowExecution
    )

    BeforeEach(func() {
        rollbackManager = rollback.NewManager(fakeClient, scheme)
        workflow = createTestWorkflow()
    })

    Context("when a step fails", func() {
        It("should trigger rollback for completed steps in reverse order", func() {
            // Step 1 succeeded
            workflow.Status.Steps[0].Phase = workflowv1alpha1.StepPhaseCompleted

            // Step 2 failed
            workflow.Status.Steps[1].Phase = workflowv1alpha1.StepPhaseFailed

            err := rollbackManager.TriggerRollback(ctx, workflow)
            Expect(err).ToNot(HaveOccurred())

            // Verify rollback triggered for Step 1 only (Step 2 never completed)
            Expect(workflow.Status.RollbackStatus).ToNot(BeNil())
            Expect(workflow.Status.RollbackStatus.StepsToRollback).To(HaveLen(1))
            Expect(workflow.Status.RollbackStatus.StepsToRollback[0]).To(Equal("step-1"))
        })
    })

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
        Entry("Custom → Custom (user-defined rollback)",
            workflowv1alpha1.ActionTypeCustom,
            workflowv1alpha1.ActionTypeCustom),
    )

    Context("rollback parameter capture", func() {
        It("should capture original state before executing action", func() {
            step := &workflowv1alpha1.WorkflowStep{
                Name:   "scale-deployment",
                Action: workflowv1alpha1.ActionTypeScaleDeployment,
                Parameters: workflowv1alpha1.StepParameters{
                    TargetResource: "deployment/web-app",
                    ScaleReplicas:  ptrInt32(5), // Scale to 5
                },
            }

            // Simulate capturing original state (was 3 replicas)
            capturedState := &workflowv1alpha1.CapturedState{
                ResourceType: "Deployment",
                ResourceName: "web-app",
                OriginalReplicas: ptrInt32(3),
            }

            rollbackParams := rollbackManager.GenerateRollbackParameters(step, capturedState)

            Expect(rollbackParams.ScaleReplicas).To(Equal(ptrInt32(3)))
            Expect(rollbackParams.TargetResource).To(Equal("deployment/web-app"))
        })
    })
})
```

**Expected Result**: Tests fail - RollbackManager doesn't exist

### DO-GREEN: Minimal Rollback Manager (3h)
**File**: `pkg/workflowexecution/rollback/manager.go`
**BR Coverage**: BR-REMEDIATION-013

[~350 lines of complete implementation code]
```go
package rollback

import (
    "context"
    "fmt"

    workflowv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// Manager manages workflow rollback logic
type Manager struct {
    client client.Client
    scheme *runtime.Scheme
}

// NewManager creates a new rollback manager
func NewManager(client client.Client, scheme *runtime.Scheme) *Manager {
    return &Manager{
        client: client,
        scheme: scheme,
    }
}

// TriggerRollback initiates rollback for a failed workflow
func (m *Manager) TriggerRollback(ctx context.Context, workflow *workflowv1alpha1.WorkflowExecution) error {
    // Identify completed steps to rollback (in reverse order)
    stepsToRollback := m.identifyRollbackSteps(workflow)

    // Update workflow status with rollback plan
    workflow.Status.RollbackStatus = &workflowv1alpha1.RollbackStatus{
        Phase:            workflowv1alpha1.RollbackPhasePending,
        StepsToRollback:  stepsToRollback,
        RollbackStarted:  metav1.Now(),
    }

    return m.client.Status().Update(ctx, workflow)
}

// identifyRollbackSteps returns list of step names to rollback (reverse order)
func (m *Manager) identifyRollbackSteps(workflow *workflowv1alpha1.WorkflowExecution) []string {
    var stepsToRollback []string

    // Iterate steps in reverse order
    for i := len(workflow.Status.Steps) - 1; i >= 0; i-- {
        step := workflow.Status.Steps[i]

        // Only rollback completed steps
        if step.Phase == workflowv1alpha1.StepPhaseCompleted {
            // Check if action is reversible
            if m.isReversibleAction(step.Action) {
                stepsToRollback = append(stepsToRollback, step.Name)
            }
        }
    }

    return stepsToRollback
}

// GetRollbackAction returns the rollback action for a given action
func (m *Manager) GetRollbackAction(step *workflowv1alpha1.WorkflowStep) workflowv1alpha1.ActionType {
    rollbackActions := map[workflowv1alpha1.ActionType]workflowv1alpha1.ActionType{
        workflowv1alpha1.ActionTypeScaleDeployment:   workflowv1alpha1.ActionTypeScaleDeployment,
        workflowv1alpha1.ActionTypeUpdateImage:       workflowv1alpha1.ActionTypeUpdateImage,
        workflowv1alpha1.ActionTypeCordonNode:        workflowv1alpha1.ActionTypeUncordonNode,
        workflowv1alpha1.ActionTypeDrainNode:         workflowv1alpha1.ActionTypeUncordonNode,
        workflowv1alpha1.ActionTypeDeletePod:         workflowv1alpha1.ActionTypeNone, // Irreversible
        workflowv1alpha1.ActionTypeUpdateConfigMap:   workflowv1alpha1.ActionTypeUpdateConfigMap,
        workflowv1alpha1.ActionTypeUpdateSecret:      workflowv1alpha1.ActionTypeUpdateSecret,
        workflowv1alpha1.ActionTypeCustom:            workflowv1alpha1.ActionTypeCustom,
    }

    if rollbackAction, exists := rollbackActions[step.Action]; exists {
        return rollbackAction
    }

    return workflowv1alpha1.ActionTypeNone
}

// GenerateRollbackParameters creates rollback parameters from captured state
func (m *Manager) GenerateRollbackParameters(step *workflowv1alpha1.WorkflowStep, capturedState *workflowv1alpha1.CapturedState) *workflowv1alpha1.StepParameters {
    rollbackParams := &workflowv1alpha1.StepParameters{
        TargetResource: step.Parameters.TargetResource,
    }

    // Restore original values based on action type
    switch step.Action {
    case workflowv1alpha1.ActionTypeScaleDeployment:
        rollbackParams.ScaleReplicas = capturedState.OriginalReplicas
    case workflowv1alpha1.ActionTypeUpdateImage:
        rollbackParams.ImageName = capturedState.OriginalImage
    case workflowv1alpha1.ActionTypeUpdateConfigMap:
        rollbackParams.ConfigMapData = capturedState.OriginalConfigMapData
    case workflowv1alpha1.ActionTypeUpdateSecret:
        rollbackParams.SecretData = capturedState.OriginalSecretData
    }

    return rollbackParams
}

// isReversibleAction checks if an action can be rolled back
func (m *Manager) isReversibleAction(action workflowv1alpha1.ActionType) bool {
    irreversibleActions := []workflowv1alpha1.ActionType{
        workflowv1alpha1.ActionTypeDeletePod,
        workflowv1alpha1.ActionTypeRestartDeployment, // Can't "un-restart"
    }

    for _, irreversible := range irreversibleActions {
        if action == irreversible {
            return false
        }
    }

    return true
}
```

**Expected Result**: Tests pass - basic rollback logic working

### DO-REFACTOR: Cascade Rollback (2h)
**Goal**: Handle dependent step rollbacks, orphaned KubernetesExecution cleanup

[~150 lines of refactored code]
- Dependency graph traversal for rollback order
- KubernetesExecution deletion during rollback
- Partial rollback support (some steps succeed, others fail)
- Rollback timeout protection

**Validation**:
- [ ] Tests passing
- [ ] Rollback cascade working
- [ ] No orphaned KubernetesExecution CRDs
- [ ] Rollback timeout < 5 minutes
```

**Total Day 5 Lines**: ~850 lines (currently ~90)

---

## Expansion Phase 2: Integration Test Suite (+650 lines)

### Integration Test 1: Multi-Step Workflow with Dependencies (~220 lines)

**File**: `test/integration/workflowexecution/multi_step_workflow_test.go`

**Structure**:
```go
var _ = Describe("Integration Test 1: Multi-Step Workflow with Dependency Resolution", func() {
    var workflow *workflowv1alpha1.WorkflowExecution
    var workflowName string

    BeforeEach(func() {
        workflowName = "test-multi-step-" + time.Now().Format("20060102150405")
    })

    AfterEach(func() {
        if workflow != nil {
            _ = crClient.Delete(ctx, workflow)
        }
    })

    It("should execute steps in dependency order using Kahn's algorithm", func() {
        By("Creating WorkflowExecution with 4 steps and dependencies")
        workflow = &workflowv1alpha1.WorkflowExecution{
            ObjectMeta: metav1.ObjectMeta{
                Name:      workflowName,
                Namespace: "kubernaut-workflows",
            },
            Spec: workflowv1alpha1.WorkflowExecutionSpec{
                Steps: []workflowv1alpha1.WorkflowStep{
                    {
                        Name:   "step-1",
                        Action: workflowv1alpha1.ActionTypeScaleDeployment,
                        // No dependencies - should execute first
                    },
                    {
                        Name:   "step-2",
                        Action: workflowv1alpha1.ActionTypeUpdateImage,
                        DependsOn: []string{"step-1"}, // Depends on step-1
                    },
                    {
                        Name:   "step-3",
                        Action: workflowv1alpha1.ActionTypeRestartDeployment,
                        DependsOn: []string{"step-1"}, // Also depends on step-1
                    },
                    {
                        Name:   "step-4",
                        Action: workflowv1alpha1.ActionTypeCustom,
                        DependsOn: []string{"step-2", "step-3"}, // Depends on both
                    },
                },
            },
        }

        err := crClient.Create(ctx, workflow)
        Expect(err).ToNot(HaveOccurred())
        GinkgoWriter.Printf("✅ Created WorkflowExecution: %s\n", workflowName)

        By("Waiting for controller to resolve dependencies")
        Eventually(func() []string {
            updated := &workflowv1alpha1.WorkflowExecution{}
            err := crClient.Get(ctx, types.NamespacedName{
                Name:      workflowName,
                Namespace: "kubernaut-workflows",
            }, updated)
            if err != nil {
                return nil
            }
            return updated.Status.ExecutionOrder
        }, 10*time.Second, 1*time.Second).Should(Equal([]string{"step-1", "step-2", "step-3", "step-4"}))

        By("Verifying topological sort succeeded")
        final := &workflowv1alpha1.WorkflowExecution{}
        err = crClient.Get(ctx, types.NamespacedName{
            Name:      workflowName,
            Namespace: "kubernaut-workflows",
        }, final)
        Expect(err).ToNot(HaveOccurred())

        // Verify execution order respects dependencies
        Expect(final.Status.ExecutionOrder).To(HaveLen(4))

        // Step-1 must be first (no dependencies)
        Expect(final.Status.ExecutionOrder[0]).To(Equal("step-1"))

        // Step-4 must be last (depends on all others)
        Expect(final.Status.ExecutionOrder[3]).To(Equal("step-4"))

        // Step-2 and Step-3 can be parallel (both depend only on Step-1)
        middleSteps := final.Status.ExecutionOrder[1:3]
        Expect(middleSteps).To(ContainElements("step-2", "step-3"))

        By("Verifying child KubernetesExecution CRDs created")
        // [Validation that KubernetesExecution CRDs were created]

        GinkgoWriter.Printf("✅ Dependency resolution validated: %v\n", final.Status.ExecutionOrder)
    })
})
```

---

### Integration Test 2: Parallel Execution with Concurrency Limits (~220 lines)

**File**: `test/integration/workflowexecution/parallel_execution_test.go`

**Structure**:
```go
var _ = Describe("Integration Test 2: Parallel Execution with Concurrency Limits", func() {
    It("should enforce maxConcurrent limit for parallel steps", func() {
        By("Creating WorkflowExecution with 6 parallel steps and maxConcurrent=2")
        workflow = &workflowv1alpha1.WorkflowExecution{
            Spec: workflowv1alpha1.WorkflowExecutionSpec{
                MaxConcurrent: 2, // Only 2 steps at a time
                Steps: []workflowv1alpha1.WorkflowStep{
                    {Name: "step-1", Action: workflowv1alpha1.ActionTypeCustom},
                    {Name: "step-2", Action: workflowv1alpha1.ActionTypeCustom},
                    {Name: "step-3", Action: workflowv1alpha1.ActionTypeCustom},
                    {Name: "step-4", Action: workflowv1alpha1.ActionTypeCustom},
                    {Name: "step-5", Action: workflowv1alpha1.ActionTypeCustom},
                    {Name: "step-6", Action: workflowv1alpha1.ActionTypeCustom},
                },
            },
        }

        By("Monitoring concurrent execution count over time")
        // [Poll status and verify never more than 2 steps in Running phase]

        By("Verifying all steps eventually complete")
        // [Wait for all steps to reach Completed phase]

        GinkgoWriter.Printf("✅ Concurrency limit enforced: max 2 concurrent\n")
    })
})
```

---

### Integration Test 3: Rollback Cascade with Owner References (~210 lines)

**File**: `test/integration/workflowexecution/rollback_cascade_test.go`

**Structure**:
```go
var _ = Describe("Integration Test 3: Rollback Cascade with Parent Deletion", func() {
    It("should rollback completed steps when workflow is deleted", func() {
        By("Creating WorkflowExecution with 3 steps")
        // [Create workflow]

        By("Waiting for 2 steps to complete")
        // [Wait for step-1 and step-2 to complete]

        By("Failing step-3 to trigger rollback")
        // [Simulate step-3 failure]

        By("Deleting WorkflowExecution CRD")
        err := crClient.Delete(ctx, workflow)
        Expect(err).ToNot(HaveOccurred())

        By("Verifying child KubernetesExecution CRDs deleted via owner references")
        // [Verify cascade deletion]

        By("Verifying rollback KubernetesExecution CRDs created")
        // [Check rollback CRDs exist]

        GinkgoWriter.Printf("✅ Rollback cascade validated\n")
    })
})
```

**Total Integration Tests**: ~650 lines (currently 0)

---

## Expansion Phase 3: EOD Documentation (+800 lines)

### EOD 1: Day 3 Midpoint (~400 lines)

**File**: `phase0/02-day3-midpoint.md`

**Structure**:
```markdown
# Day 3 Midpoint: Dependency Resolution & Parallel Execution Complete

**Date**: [YYYY-MM-DD]
**Status**: Days 1-3 Complete (30% of implementation)
**Confidence**: 82%

## Accomplishments (Days 1-3)
### Day 1: Foundation ✅
- [Summary]

### Day 2: Dependency Resolution ✅ (EXPANDED)
- Kahn's algorithm working (800 lines detailed)
- Topological sort validated
- Cycle detection implemented
- Controller integration complete
- ...

### Day 3: Parallel Execution ✅ (EXPANDED)
- Parent-child CRD pattern working (950 lines detailed)
- Watch-based coordination functional
- Concurrency limits enforced
- KubernetesExecution owner references set
- ...

## Integration Status
[Working vs pending components]

## BR Progress Tracking
[18/35 BRs complete (51%)]

## Blockers
**None at this time** ✅

## Next Steps (Days 4-7)
- Day 4: Step parameter passing
- Day 5: Rollback logic (expansion planned)
- Day 6-7: Complete integration

## Confidence Assessment
**Current Confidence**: 82%
[Justification: Days 2-3 fully expanded give high confidence in architecture]
```

---

### EOD 2: Day 7 Complete (~400 lines)

**File**: `phase0/03-day7-complete.md`

**Structure**: Similar to Day 3 but covering Days 1-7

---

## Expansion Phase 4: Error Handling Philosophy (+300 lines)

**File**: `design/ERROR_HANDLING_PHILOSOPHY.md`

**Structure**:
```markdown
# Error Handling Philosophy - Workflow Execution

## Executive Summary
Workflow-specific error handling for:
- BR-REMEDIATION-013: Rollback on failure
- BR-REMEDIATION-016: Step timeout handling
- Operational excellence

## Error Classification Taxonomy

### 1. Transient Errors (RETRY)
| Error Type | Example | Retry | Rollback |
|-----------|---------|-------|----------|
| **KubernetesExecution Timeout** | Step exceeded timeout | ✅ Yes | ❌ No (retry first) |
| **API Server 503** | K8s API unavailable | ✅ Yes | ❌ No |

### 2. Permanent Errors (FAIL + ROLLBACK)
| Error Type | Example | Retry | Rollback |
|-----------|---------|-------|----------|
| **Step Dependency Cycle** | Circular dependencies | ❌ No | ❌ N/A (pre-execution) |
| **Invalid Action Parameters** | Missing required field | ❌ No | ❌ No (never started) |
| **Step Execution Failed** | kubectl command failed | ❌ No | ✅ Yes (rollback previous) |

### 3. Rollback-Specific Errors
| Error Type | Action |
|-----------|--------|
| **Rollback Step Failed** | Log warning, continue rollback |
| **Rollback Timeout** | Mark workflow as PartiallyRolledBack |

## Rollback Decision Matrix

**When to rollback**:
1. Step fails after completion of previous steps
2. Workflow deleted while in progress
3. Timeout exceeded for workflow (not individual step)

**When NOT to rollback**:
1. Validation failure (nothing executed yet)
2. First step fails (nothing to rollback)
3. Step marked as "critical" with continue-on-failure

## Operational Guidelines
[Monitoring, alerts, testing]
```

---

## Expansion Phase 5: Enhanced BR Coverage Matrix (+200 lines)

**Enhancement to existing**: `testing/BR_COVERAGE_MATRIX.md`

**Additions**: Same structure as Remediation Processor
- Testing Infrastructure section (Envtest specification)
- Edge Case Coverage section (10 additional scenarios)
- Test Implementation Guidance with DescribeTable examples
- Defense-in-Depth coverage update (overlapping percentages)

---

## Summary: Total Additions by Phase

| Phase | Component | Lines | Effort |
|-------|-----------|-------|--------|
| **Phase 1** | Day 5 APDC Expansion (Days 2-3 ✅ done) | 850 | 3h |
| **Phase 2** | Integration Test 1 | 220 | 1.5h |
| **Phase 2** | Integration Test 2 | 220 | 1.5h |
| **Phase 2** | Integration Test 3 | 210 | 1.5h |
| **Phase 3** | EOD: Day 3 Midpoint | 400 | 1h |
| **Phase 3** | EOD: Day 7 Complete | 400 | 1h |
| **Phase 4** | Error Handling Philosophy | 300 | 1h |
| **Phase 5** | BR Coverage Matrix Enhancement | 200 | 0.5h |
| **Total** | **All Additions** | **2,800** | **11h** |

**Note**: 1,750 lines already complete (Days 2-3 expansions)!

**Confidence Increase**: 70% → 95%

---

## Approval Required

**This plan shows WHAT will be added but does NOT implement it yet.**

Please review and approve before implementation begins.

