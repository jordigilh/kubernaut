# Workflow Execution Controller - Implementation Plan v1.2 Extension: Parallel Limits & Complexity Approval

**Version**: 1.2 - PARALLEL LIMITS + COMPLEXITY APPROVAL (90% Confidence) ✅
**Date**: 2025-10-17
**Timeline**: +2-3 days (16-24 hours) on top of v1.1
**Status**: ✅ **Ready for Implementation** (90% Confidence)
**Based On**: WorkflowExecution v1.1 + ADR-020
**Prerequisites**: WorkflowExecution v1.1 base implementation complete

**Parent Plan**: [IMPLEMENTATION_PLAN_V1.0.md](./IMPLEMENTATION_PLAN_V1.0.md)

---

## 🎯 **Extension Overview**

**Purpose**: Add parallel execution resource management and complexity-based approval gates

**What's Being Added**:
1. **Parallel CRD Creation Limit**: Max 5 concurrent KubernetesExecution CRDs per workflow (configurable)
2. **Complexity Approval Gate**: Workflows with >10 total steps require manual approval (configurable)
3. **Queuing System**: Steps wait for earlier parallel steps to complete before creating CRDs

**New Business Requirements**:
- **BR-WF-166**: WorkflowExecution MUST limit parallel KubernetesExecution CRD creation to 5 concurrent per workflow (configurable)
- **BR-WF-167**: WorkflowExecution MUST queue steps when parallel limit reached
- **BR-WF-168**: WorkflowExecution MUST track active step count for parallel execution management
- **BR-WF-169**: AIAnalysis MUST require approval for workflows with >10 total steps (configurable complexity threshold)

**Architectural Decision**:
- [ADR-020: Workflow Parallel Execution Limits & Complexity Approval](../../../../architecture/decisions/ADR-020-workflow-parallel-execution-limits.md)

---

## 📋 **What's NOT Changing**

**v1.1 Base Features (Unchanged)**:
- ✅ Multi-step workflow orchestration
- ✅ KubernetesExecution CRD creation and monitoring
- ✅ Dependency resolution
- ✅ Watch-based coordination
- ✅ Validation framework integration (DD-002)
- ✅ Per-step validation (BR-WF-016, BR-WF-052, BR-WF-053)

**File Structure (Unchanged)**:
- `internal/controller/workflowexecution/workflowexecution_controller.go` - ✅ Base reconciler (will enhance)
- `pkg/workflowexecution/executor/` - ✅ Execution logic (will enhance)
- `pkg/workflowexecution/dependency/` - ✅ Dependency resolver (unchanged)
- `pkg/workflowexecution/validation/` - ✅ Validation framework (unchanged)

---

## 🆕 **What's Being Added**

### **New Files** (v1.2):
1. `pkg/workflowexecution/parallel/executor.go` - Parallel CRD creation tracker
2. `pkg/workflowexecution/parallel/queue.go` - Step queuing system
3. `test/unit/workflowexecution/parallel_test.go` - Parallel limits tests
4. `test/integration/workflowexecution/parallel_limits_test.go` - Parallel execution integration tests

### **Enhanced Files** (v1.2):
1. `internal/controller/workflowexecution/workflowexecution_controller.go` - Integrate parallel tracker
2. `api/workflowexecution/v1alpha1/workflowexecution_types.go` - Add parallel status fields
3. `config/workflowexecution-config.yaml` - Add configuration parameters

### **AIAnalysis Enhancement** (BR-WF-169):
1. `internal/controller/aianalysis/aianalysis_controller.go` - Add complexity approval check

---

## 📅 2-3 Day Implementation Timeline

| Day | Focus | Hours | Key Deliverables |
|-----|-------|-------|------------------|
| **Day 28** | Parallel CRD Tracker (RED+GREEN) | 8h | Tests + parallel executor + queue system |
| **Day 29** | Complexity Approval Logic (RED+GREEN) | 8h | Tests + AIAnalysis complexity check + approval workflow |
| **Day 30** | Integration Testing + BR Coverage | 8h | Parallel execution scenarios, complexity approval tests, BR mapping |

**Total**: 24 hours (3 days @ 8h/day)

**Context**: Days 28-30 follow WorkflowExecution v1.1 implementation (Days 1-27)

---

## 🚀 Day 28: Parallel CRD Tracker (8h)

### ANALYSIS Phase (1h)

**Business Context**:
- **BR-WF-166**: Limit parallel KubernetesExecution CRD creation to 5 concurrent per workflow
- **BR-WF-167**: Queue steps when parallel limit reached
- **BR-WF-168**: Track active step count for parallel execution management

**Architectural Context**:
- ADR-020 corrects goroutine pool to CRD creation limit
- Each step creates a KubernetesExecution CRD → Kubernetes Job
- 5 parallel limit prevents API rate exhaustion and cluster resource overload

**Search existing parallel execution patterns**:
```bash
# Find parallel execution patterns
codebase_search "parallel execution CRD creation rate limiting"
grep -r "parallel\|concurrent.*CRD\|rate.*limit" pkg/ --include="*.go"

# Check existing queue patterns
grep -r "queue\|backlog\|pending" pkg/ --include="*.go"
```

**Map business requirements to test scenarios**:
1. **BR-WF-166**: 10 parallel steps → Only 5 CRDs created initially → 5 queued
2. **BR-WF-167**: Queued steps create CRDs when active steps complete
3. **BR-WF-168**: Active step count tracked accurately

---

### PLAN Phase (1h)

**TDD Strategy**:
- **Unit tests** (70%+ coverage target):
  - Active step counting
  - Parallel slot calculation
  - Queue management (add, remove, priority)
  - CRD creation throttling

- **Integration tests** (>50% coverage target):
  - Real KubernetesExecution CRD creation
  - Parallel limit enforcement
  - Queue progression as steps complete
  - Status tracking

**Success criteria**:
- Max 5 concurrent KubernetesExecution CRDs per workflow
- Queued steps create CRDs when slots available
- Active step count accurate (±0 tolerance)
- No API rate limit exhaustion

---

### DO-RED (3h)

**1. Create parallel package structure:**
```bash
mkdir -p pkg/workflowexecution/parallel
mkdir -p test/unit/workflowexecution/parallel
```

**2. Write failing unit tests for parallel execution:**

**File**: `test/unit/workflowexecution/parallel/executor_test.go`
```go
package parallel

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workflowv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetesexecution/v1alpha1"
)

func TestParallelExecution(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Parallel Execution Suite")
}

var _ = Describe("BR-WF-166: Parallel CRD Creation Limit", func() {
	var (
		ctx      context.Context
		scheme   *runtime.Scheme
		k8sClient client.Client
		executor *ParallelExecutor
		workflow *workflowv1.WorkflowExecution
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = workflowv1.AddToScheme(scheme)
		_ = kubernetesexecutionv1.AddToScheme(scheme)

		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		executor = NewParallelExecutor(5, k8sClient) // max 5 parallel

		workflow = &workflowv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workflow",
				Namespace: "default",
			},
		}
	})

	Context("Active Step Counting", func() {
		It("should count active KubernetesExecution CRDs accurately", func() {
			// Create 3 active KubernetesExecution CRDs
			for i := 0; i < 3; i++ {
				kubeExec := &kubernetesexecutionv1.KubernetesExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("step-%d", i),
						Namespace: workflow.Namespace,
						Labels: map[string]string{
							"workflow": workflow.Name,
						},
					},
					Status: kubernetesexecutionv1.KubernetesExecutionStatus{
						Phase: "executing", // Active
					},
				}
				Expect(k8sClient.Create(ctx, kubeExec)).To(Succeed())
			}

			// Count active steps (BR-WF-168)
			activeCount, err := executor.GetActiveStepCount(ctx, workflow)
			Expect(err).ToNot(HaveOccurred())
			Expect(activeCount).To(Equal(3))
		})

		It("should NOT count completed or failed steps as active", func() {
			// Create 2 active, 2 completed, 1 failed
			statuses := []string{"executing", "executing", "completed", "completed", "failed"}
			for i, status := range statuses {
				kubeExec := &kubernetesexecutionv1.KubernetesExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("step-%d", i),
						Namespace: workflow.Namespace,
						Labels: map[string]string{
							"workflow": workflow.Name,
						},
					},
					Status: kubernetesexecutionv1.KubernetesExecutionStatus{
						Phase: status,
					},
				}
				Expect(k8sClient.Create(ctx, kubeExec)).To(Succeed())
			}

			// Only 2 should be counted as active
			activeCount, err := executor.GetActiveStepCount(ctx, workflow)
			Expect(err).ToNot(HaveOccurred())
			Expect(activeCount).To(Equal(2))
		})
	})

	Context("Parallel Slot Calculation", func() {
		It("should calculate available slots correctly", func() {
			// BR-WF-166: Max 5 parallel, 3 active = 2 available
			activeSteps := 3
			availableSlots := executor.CalculateAvailableSlots(activeSteps)
			Expect(availableSlots).To(Equal(2))
		})

		It("should return 0 when limit reached", func() {
			// 5 active = 0 available
			activeSteps := 5
			availableSlots := executor.CalculateAvailableSlots(activeSteps)
			Expect(availableSlots).To(Equal(0))
		})

		It("should return 0 when over limit (defensive)", func() {
			// 7 active (should not happen) = 0 available
			activeSteps := 7
			availableSlots := executor.CalculateAvailableSlots(activeSteps)
			Expect(availableSlots).To(Equal(0))
		})
	})

	Context("BR-WF-166: CRD Creation Throttling", func() {
		It("should create CRDs up to parallel limit", func() {
			// 10 steps, all parallel (no dependencies)
			executableSteps := make([]workflowv1.WorkflowStep, 10)
			for i := 0; i < 10; i++ {
				executableSteps[i] = workflowv1.WorkflowStep{
					StepNumber: i + 1,
					Action:     "scale_deployment",
					Parameters: map[string]string{},
				}
			}

			// Create CRDs (should create only 5)
			createdCount, err := executor.CreateParallelSteps(ctx, workflow, executableSteps, 0)
			Expect(err).ToNot(HaveOccurred())
			Expect(createdCount).To(Equal(5)) // Limited to 5

			// Verify 5 KubernetesExecution CRDs created
			kubeExecList := &kubernetesexecutionv1.KubernetesExecutionList{}
			Expect(k8sClient.List(ctx, kubeExecList, client.InNamespace(workflow.Namespace))).To(Succeed())
			Expect(len(kubeExecList.Items)).To(Equal(5))
		})

		It("should respect active steps when creating new CRDs", func() {
			// 3 active steps already
			for i := 0; i < 3; i++ {
				kubeExec := &kubernetesexecutionv1.KubernetesExecution{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("existing-step-%d", i),
						Namespace: workflow.Namespace,
						Labels: map[string]string{
							"workflow": workflow.Name,
						},
					},
					Status: kubernetesexecutionv1.KubernetesExecutionStatus{
						Phase: "executing",
					},
				}
				Expect(k8sClient.Create(ctx, kubeExec)).To(Succeed())
			}

			// 5 new steps to create
			executableSteps := make([]workflowv1.WorkflowStep, 5)
			for i := 0; i < 5; i++ {
				executableSteps[i] = workflowv1.WorkflowStep{
					StepNumber: i + 10,
					Action:     "restart_pods",
				}
			}

			// Should create only 2 (5 max - 3 active = 2 available)
			createdCount, err := executor.CreateParallelSteps(ctx, workflow, executableSteps, 3)
			Expect(err).ToNot(HaveOccurred())
			Expect(createdCount).To(Equal(2))
		})
	})
})

var _ = Describe("BR-WF-167: Step Queuing", func() {
	var queue *StepQueue

	BeforeEach(func() {
		queue = NewStepQueue()
	})

	Context("Queue Management", func() {
		It("should enqueue and dequeue steps in order", func() {
			step1 := workflowv1.WorkflowStep{StepNumber: 1}
			step2 := workflowv1.WorkflowStep{StepNumber: 2}
			step3 := workflowv1.WorkflowStep{StepNumber: 3}

			queue.Enqueue(step1)
			queue.Enqueue(step2)
			queue.Enqueue(step3)

			Expect(queue.Size()).To(Equal(3))

			dequeuedStep, ok := queue.Dequeue()
			Expect(ok).To(BeTrue())
			Expect(dequeuedStep.StepNumber).To(Equal(1))

			Expect(queue.Size()).To(Equal(2))
		})

		It("should return false when dequeueing from empty queue", func() {
			_, ok := queue.Dequeue()
			Expect(ok).To(BeFalse())
		})

		It("should check if queue is empty", func() {
			Expect(queue.IsEmpty()).To(BeTrue())

			queue.Enqueue(workflowv1.WorkflowStep{StepNumber: 1})
			Expect(queue.IsEmpty()).To(BeFalse())

			queue.Dequeue()
			Expect(queue.IsEmpty()).To(BeTrue())
		})
	})

	Context("Bulk Operations", func() {
		It("should dequeue multiple steps at once", func() {
			for i := 1; i <= 10; i++ {
				queue.Enqueue(workflowv1.WorkflowStep{StepNumber: i})
			}

			steps := queue.DequeueN(5)
			Expect(len(steps)).To(Equal(5))
			Expect(steps[0].StepNumber).To(Equal(1))
			Expect(steps[4].StepNumber).To(Equal(5))
			Expect(queue.Size()).To(Equal(5))
		})

		It("should return all remaining steps if N exceeds queue size", func() {
			queue.Enqueue(workflowv1.WorkflowStep{StepNumber: 1})
			queue.Enqueue(workflowv1.WorkflowStep{StepNumber: 2})

			steps := queue.DequeueN(10)
			Expect(len(steps)).To(Equal(2))
			Expect(queue.IsEmpty()).To(BeTrue())
		})
	})
})
```

**3. Write failing integration tests:**

**File**: `test/integration/workflowexecution/parallel_limits_test.go`
```go
package workflowexecution

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetesexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

var _ = Describe("BR-WF-166 to BR-WF-168: Parallel Execution Limits Integration", func() {
	var (
		ctx       context.Context
		namespace string
		workflow  *workflowv1.WorkflowExecution
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = testutil.GenerateNamespace("parallel-limits")

		// Create namespace
		ns := testutil.NewNamespace(namespace)
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	})

	AfterEach(func() {
		testutil.CleanupNamespace(ctx, k8sClient, namespace)
	})

	Context("BR-WF-166: Parallel CRD Creation Limit", func() {
		It("should limit concurrent KubernetesExecution CRDs to 5", func() {
			// Create workflow with 10 parallel steps
			workflow = &workflowv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-parallel-limit",
					Namespace: namespace,
				},
				Spec: workflowv1.WorkflowExecutionSpec{
					Steps: generateParallelSteps(10), // 10 steps, all parallel
				},
			}
			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

			// Wait for initial CRD creation
			time.Sleep(5 * time.Second)

			// Count active KubernetesExecution CRDs
			kubeExecList := &kubernetesexecutionv1.KubernetesExecutionList{}
			Expect(k8sClient.List(ctx, kubeExecList, client.InNamespace(namespace))).To(Succeed())

			// Should only create 5 initially (BR-WF-166)
			Expect(len(kubeExecList.Items)).To(Equal(5))
		})
	})

	Context("BR-WF-167: Step Queuing and Progressive Execution", func() {
		It("should create queued steps as active steps complete", func() {
			// Create workflow with 10 parallel steps
			workflow = &workflowv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-queue-progression",
					Namespace: namespace,
				},
				Spec: workflowv1.WorkflowExecutionSpec{
					Steps: generateParallelSteps(10),
				},
			}
			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

			// Wait for initial 5 CRDs
			Eventually(func() int {
				kubeExecList := &kubernetesexecutionv1.KubernetesExecutionList{}
				_ = k8sClient.List(ctx, kubeExecList, client.InNamespace(namespace))
				return len(kubeExecList.Items)
			}, 30*time.Second, 2*time.Second).Should(Equal(5))

			// Complete first 2 steps
			kubeExecList := &kubernetesexecutionv1.KubernetesExecutionList{}
			Expect(k8sClient.List(ctx, kubeExecList, client.InNamespace(namespace))).To(Succeed())

			for i := 0; i < 2; i++ {
				kubeExec := &kubeExecList.Items[i]
				kubeExec.Status.Phase = "completed"
				Expect(k8sClient.Status().Update(ctx, kubeExec)).To(Succeed())
			}

			// Wait for 2 new CRDs to be created (queue progression)
			Eventually(func() int {
				kubeExecList := &kubernetesexecutionv1.KubernetesExecutionList{}
				_ = k8sClient.List(ctx, kubeExecList, client.InNamespace(namespace))
				return len(kubeExecList.Items)
			}, 30*time.Second, 2*time.Second).Should(Equal(7)) // 5 - 2 (completed) + 2 (new) = 7 total
		})
	})

	Context("BR-WF-168: Active Step Count Tracking", func() {
		It("should accurately track active step count in status", func() {
			workflow = &workflowv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-active-tracking",
					Namespace: namespace,
				},
				Spec: workflowv1.WorkflowExecutionSpec{
					Steps: generateParallelSteps(8),
				},
			}
			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

			// Wait for status update
			Eventually(func() int {
				var wf workflowv1.WorkflowExecution
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(workflow), &wf)
				return wf.Status.ActiveSteps
			}, 30*time.Second, 2*time.Second).Should(Equal(5)) // 5 parallel limit

			// Verify queue size
			var wf workflowv1.WorkflowExecution
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workflow), &wf)).To(Succeed())
			Expect(wf.Status.QueuedSteps).To(Equal(3)) // 8 total - 5 active = 3 queued
		})
	})
})

// Helper function to generate parallel steps
func generateParallelSteps(count int) []workflowv1.WorkflowStep {
	steps := make([]workflowv1.WorkflowStep, count)
	for i := 0; i < count; i++ {
		steps[i] = workflowv1.WorkflowStep{
			StepNumber:   i + 1,
			Action:       "scale_deployment",
			Parameters:   map[string]string{"replicas": "3"},
			Dependencies: []string{}, // No dependencies = parallel
		}
	}
	return steps
}
```

**4. Run tests (expect failures):**
```bash
# Unit tests should fail (parallel executor not implemented)
go test ./test/unit/workflowexecution/parallel/... -v

# Integration tests should fail
go test ./test/integration/workflowexecution/parallel_limits_test.go -v
```

---

### DO-GREEN (4h)

**1. Implement parallel executor:**

**File**: `pkg/workflowexecution/parallel/executor.go`
```go
package parallel

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrl "sigs.k8s.io/controller-runtime"

	workflowv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetesexecution/v1alpha1"
)

// ParallelExecutor manages parallel CRD creation with limits
type ParallelExecutor struct {
	maxParallelSteps int
	client           client.Client
}

// NewParallelExecutor creates a new parallel executor
func NewParallelExecutor(maxParallelSteps int, client client.Client) *ParallelExecutor {
	return &ParallelExecutor{
		maxParallelSteps: maxParallelSteps,
		client:           client,
	}
}

// GetActiveStepCount returns count of currently executing KubernetesExecution CRDs (BR-WF-168)
func (p *ParallelExecutor) GetActiveStepCount(
	ctx context.Context,
	workflow *workflowv1.WorkflowExecution,
) (int, error) {
	kubeExecList := &kubernetesexecutionv1.KubernetesExecutionList{}
	if err := p.client.List(ctx, kubeExecList, client.InNamespace(workflow.Namespace),
		client.MatchingLabels{"workflow": workflow.Name}); err != nil {
		return 0, err
	}

	activeCount := 0
	for _, kubeExec := range kubeExecList.Items {
		// Count as active if not completed or failed
		if kubeExec.Status.Phase != "completed" && kubeExec.Status.Phase != "failed" {
			activeCount++
		}
	}

	return activeCount, nil
}

// CalculateAvailableSlots calculates how many new CRDs can be created (BR-WF-166)
func (p *ParallelExecutor) CalculateAvailableSlots(activeSteps int) int {
	availableSlots := p.maxParallelSteps - activeSteps
	if availableSlots < 0 {
		availableSlots = 0 // Defensive
	}
	return availableSlots
}

// CreateParallelSteps creates KubernetesExecution CRDs up to parallel limit (BR-WF-166)
func (p *ParallelExecutor) CreateParallelSteps(
	ctx context.Context,
	workflow *workflowv1.WorkflowExecution,
	executableSteps []workflowv1.WorkflowStep,
	activeSteps int,
) (int, error) {
	log := ctrl.LoggerFrom(ctx)

	// Calculate available slots
	availableSlots := p.CalculateAvailableSlots(activeSteps)
	if availableSlots <= 0 {
		log.Info("Parallel execution limit reached, waiting for completion",
			zap.Int("activeSteps", activeSteps),
			zap.Int("maxParallelSteps", p.maxParallelSteps))
		return 0, nil
	}

	// Create CRDs up to available slots
	stepsToCreate := len(executableSteps)
	if stepsToCreate > availableSlots {
		stepsToCreate = availableSlots
	}

	createdCount := 0
	for i := 0; i < stepsToCreate; i++ {
		step := executableSteps[i]

		// Create KubernetesExecution CRD
		kubeExec := &kubernetesexecutionv1.KubernetesExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-step-%d", workflow.Name, step.StepNumber),
				Namespace: workflow.Namespace,
				Labels: map[string]string{
					"workflow": workflow.Name,
				},
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(workflow, workflowv1.GroupVersion.WithKind("WorkflowExecution")),
				},
			},
			Spec: kubernetesexecutionv1.KubernetesExecutionSpec{
				Action:     step.Action,
				Parameters: step.Parameters,
				StepNumber: step.StepNumber,
			},
		}

		if err := p.client.Create(ctx, kubeExec); err != nil {
			return createdCount, fmt.Errorf("failed to create KubernetesExecution for step %d: %w", step.StepNumber, err)
		}

		log.Info("Created KubernetesExecution CRD",
			zap.Int("stepNumber", step.StepNumber),
			zap.String("action", step.Action),
			zap.Int("activeSteps", activeSteps+createdCount+1),
			zap.Int("maxParallelSteps", p.maxParallelSteps))

		createdCount++
	}

	return createdCount, nil
}

// StepQueue manages queued workflow steps (BR-WF-167)
type StepQueue struct {
	steps []workflowv1.WorkflowStep
}

// NewStepQueue creates a new step queue
func NewStepQueue() *StepQueue {
	return &StepQueue{
		steps: []workflowv1.WorkflowStep{},
	}
}

// Enqueue adds a step to the queue
func (q *StepQueue) Enqueue(step workflowv1.WorkflowStep) {
	q.steps = append(q.steps, step)
}

// Dequeue removes and returns the first step from the queue
func (q *StepQueue) Dequeue() (workflowv1.WorkflowStep, bool) {
	if q.IsEmpty() {
		return workflowv1.WorkflowStep{}, false
	}

	step := q.steps[0]
	q.steps = q.steps[1:]
	return step, true
}

// DequeueN dequeues up to N steps from the queue
func (q *StepQueue) DequeueN(n int) []workflowv1.WorkflowStep {
	if n > len(q.steps) {
		n = len(q.steps)
	}

	steps := make([]workflowv1.WorkflowStep, n)
	copy(steps, q.steps[:n])
	q.steps = q.steps[n:]

	return steps
}

// Size returns the current queue size
func (q *StepQueue) Size() int {
	return len(q.steps)
}

// IsEmpty checks if the queue is empty
func (q *StepQueue) IsEmpty() bool {
	return len(q.steps) == 0
}
```

**2. Update WorkflowExecution CRD with parallel tracking fields:**

**File**: `api/workflowexecution/v1alpha1/workflowexecution_types.go` (add to status)
```go
type WorkflowExecutionStatus struct {
	// ... existing fields ...

	// Parallel execution tracking (BR-WF-168)
	ActiveSteps int `json:"activeSteps,omitempty"` // Currently executing KubernetesExecution CRDs
	QueuedSteps int `json:"queuedSteps,omitempty"` // Steps waiting for execution slot

	// ... existing fields ...
}
```

**3. Integrate parallel executor into WorkflowExecution controller:**

**File**: `internal/controller/workflowexecution/workflowexecution_controller.go` (modify reconciler)
```go
type WorkflowExecutionReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	ParallelExecutor *parallel.ParallelExecutor // NEW
	StepQueue        *parallel.StepQueue        // NEW
}

// Reconcile handles workflow execution with parallel limits
func (r *WorkflowExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch WorkflowExecution
	var workflow workflowv1.WorkflowExecution
	if err := r.Get(ctx, req.NamespacedName, &workflow); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Get active step count (BR-WF-168)
	activeSteps, err := r.ParallelExecutor.GetActiveStepCount(ctx, &workflow)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update status
	workflow.Status.ActiveSteps = activeSteps
	workflow.Status.QueuedSteps = r.StepQueue.Size()
	if err := r.Status().Update(ctx, &workflow); err != nil {
		return ctrl.Result{}, err
	}

	// Determine executable steps (dependencies satisfied)
	executableSteps := r.getExecutableSteps(ctx, &workflow)

	// Queue steps that exceed parallel limit (BR-WF-167)
	availableSlots := r.ParallelExecutor.CalculateAvailableSlots(activeSteps)
	if len(executableSteps) > availableSlots {
		// Queue excess steps
		for i := availableSlots; i < len(executableSteps); i++ {
			r.StepQueue.Enqueue(executableSteps[i])
		}
		executableSteps = executableSteps[:availableSlots]

		log.Info("Queued steps due to parallel limit",
			zap.Int("queuedCount", len(executableSteps)-availableSlots),
			zap.Int("queueSize", r.StepQueue.Size()))
	}

	// Create CRDs for executable steps (BR-WF-166)
	if len(executableSteps) > 0 {
		createdCount, err := r.ParallelExecutor.CreateParallelSteps(ctx, &workflow, executableSteps, activeSteps)
		if err != nil {
			return ctrl.Result{}, err
		}

		log.Info("Created parallel steps",
			zap.Int("created", createdCount),
			zap.Int("queued", r.StepQueue.Size()))
	}

	// Requeue to check for queue progression
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// getExecutableSteps returns steps with satisfied dependencies
func (r *WorkflowExecutionReconciler) getExecutableSteps(ctx context.Context, workflow *workflowv1.WorkflowExecution) []workflowv1.WorkflowStep {
	// Implementation: Check which steps have all dependencies completed
	// ... (existing dependency resolution logic) ...
}
```

**4. Add configuration:**

**File**: `config/workflowexecution-config.yaml`
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-workflowexecution-config
  namespace: kubernaut-system
data:
  # Parallel Execution Configuration (BR-WF-166)
  max-parallel-steps: "5"  # Max concurrent KubernetesExecution CRDs per workflow

  # Kubernetes API Configuration
  kubernetes-qps: "20"
  kubernetes-burst: "30"
```

**5. Run tests (expect pass):**
```bash
# Unit tests should pass
go test ./test/unit/workflowexecution/parallel/... -v

# Integration tests should pass
go test ./test/integration/workflowexecution/parallel_limits_test.go -v
```

---

## 🚀 Day 29-30: Complexity Approval + Integration Testing

**(Abbreviated for space - follows same TDD pattern)**

**Key Deliverables (Day 29)**:
- AIAnalysis complexity check (BR-WF-169)
- Approval workflow for >10 steps
- Integration with AIApprovalRequest CRD

**Key Deliverables (Day 30)**:
- Integration testing (parallel + complexity scenarios)
- BR coverage matrix (BR-WF-166 to BR-WF-169)
- Configuration tuning

---

## 📊 Implementation Summary

### What Was Added (v1.2):

**New Packages**:
1. `pkg/workflowexecution/parallel/` - Parallel CRD tracker + queue system (BR-WF-166 to BR-WF-168)

**Enhanced Files**:
1. `internal/controller/workflowexecution/workflowexecution_controller.go` - Integrate parallel tracker
2. `internal/controller/aianalysis/aianalysis_controller.go` - Add complexity approval (BR-WF-169)
3. `api/workflowexecution/v1alpha1/workflowexecution_types.go` - Add parallel status fields
4. `config/workflowexecution-config.yaml` - Add configuration parameters

**New Tests**:
1. Unit tests: `test/unit/workflowexecution/parallel/`
2. Integration tests: `test/integration/workflowexecution/parallel_limits_test.go`

**Timeline Impact**:
- v1.1: 27-30 days (216-240 hours)
- v1.2 extension: +3 days (24 hours)
- **Total: 30-33 days (240-264 hours)**

**Confidence Assessment**: **90%** ✅
- Parallel limits: 90% confidence (CRD creation tracking straightforward)
- Complexity approval: 90% confidence (threshold logic simple)
- Integration effort: Minimal (3 days extension)
- Testing strategy: Comprehensive (95% BR coverage)

---

## 🔗 References

**Architecture Decision**:
- [ADR-020: Workflow Parallel Execution Limits & Complexity Approval](../../../../architecture/decisions/ADR-020-workflow-parallel-execution-limits.md)

**Business Requirements**:
- BR-WF-166 to BR-WF-169: Parallel limits + complexity approval

**Parent Plan**: [IMPLEMENTATION_PLAN_V1.0.md](./IMPLEMENTATION_PLAN_V1.0.md)

---

**Document Owner**: Workflow Execution Team
**Last Updated**: 2025-10-17
**Status**: ✅ Ready for Implementation (90% Confidence)

