## Testing Strategy

**Version**: 4.0
**Last Updated**: 2025-12-02
**CRD API Group**: `workflowexecution.kubernaut.ai/v1alpha1`
**Status**: ✅ Updated for Tekton Architecture

---

## Changelog

### Version 4.0 (2025-12-02)
- ✅ **Rewritten**: Complete rewrite for Tekton PipelineRun architecture
- ✅ **Removed**: All KubernetesExecution/step orchestration test patterns
- ✅ **Updated**: Tests focus on PipelineRun creation and resource locking

---

**Testing Framework Reference**: [.cursor/rules/03-testing-strategy.mdc](../../../.cursor/rules/03-testing-strategy.mdc)

### Testing Pyramid

Following Kubernaut's defense-in-depth testing strategy:

| Test Type | Target Coverage | Focus | Confidence |
|-----------|----------------|-------|------------|
| **Unit Tests** | 70%+ | Controller logic, PipelineRun building, resource locking | 85-90% |
| **Integration Tests** | >50% | CRD interactions, Tekton PipelineRun creation, status sync | 80-85% |
| **E2E Tests** | 10-15% | Complete workflow execution with Tekton, resource locking | 90-95% |

**Rationale**: WorkflowExecution delegates step orchestration to Tekton, so tests focus on:
1. **PipelineRun creation** - Bundle resolver, parameter passing
2. **Status synchronization** - Mapping Tekton conditions to WE phases
3. **Resource locking** - Parallel execution prevention, cooldown

---

### Unit Tests (Primary Coverage Layer)

**Test Directory**: [test/unit/workflowexecution/](../../../test/unit/workflowexecution/)
**Coverage Target**: 70%
**Confidence**: 85-90%
**Execution**: `make test-unit-workflowexecution`

**Testing Strategy**: Use fake K8s client for compile-time API safety. Mock external Tekton APIs.

**Core Test Patterns**:

```go
package workflowexecution

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "context"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1"
    tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
    "github.com/jordigilh/kubernaut/internal/controller"
    "github.com/jordigilh/kubernaut/pkg/testutil"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("BR-WE-001: PipelineRun Creation", func() {
    var (
        fakeK8sClient client.Client
        scheme        *runtime.Scheme
        reconciler    *controller.WorkflowExecutionReconciler
        ctx           context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        scheme = testutil.NewTestScheme()
        fakeK8sClient = fake.NewClientBuilder().
            WithScheme(scheme).
            WithStatusSubresource(&workflowexecutionv1.WorkflowExecution{}).
            Build()

        reconciler = &controller.WorkflowExecutionReconciler{
            Client: fakeK8sClient,
            Scheme: scheme,
        }
    })

    Context("PipelineRun Building", func() {
        It("should create PipelineRun with bundle resolver", func() {
            wfe := &workflowexecutionv1.WorkflowExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "wfe-test",
                    Namespace: "default",
                },
                Spec: workflowexecutionv1.WorkflowExecutionSpec{
                    WorkflowRef: workflowexecutionv1.WorkflowReference{
                        WorkflowID:     "increase-memory-conservative",
                        ContainerImage: "ghcr.io/kubernaut/workflows/increase-memory@sha256:abc123",
                    },
                    TargetResource: "production/deployment/payment-service",
                    Parameters: map[string]string{
                        "NAMESPACE":       "production",
                        "DEPLOYMENT_NAME": "payment-service",
                    },
                },
            }
            Expect(fakeK8sClient.Create(ctx, wfe)).To(Succeed())

            // Reconcile should create PipelineRun
            _, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest(wfe))
            Expect(err).ToNot(HaveOccurred())

            // Verify PipelineRun created with bundle resolver
            var prList tektonv1.PipelineRunList
            Expect(fakeK8sClient.List(ctx, &prList)).To(Succeed())
            Expect(prList.Items).To(HaveLen(1))

            pr := prList.Items[0]
            Expect(pr.Spec.PipelineRef.ResolverRef.Resolver).To(Equal("bundles"))
        })

        It("should pass parameters to PipelineRun", func() {
            wfe := testutil.NewTestWorkflowExecution("wfe-params-test")
            wfe.Spec.Parameters = map[string]string{
                "NAMESPACE":          "production",
                "MEMORY_INCREMENT_MB": "256",
            }
            Expect(fakeK8sClient.Create(ctx, wfe)).To(Succeed())

            _, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest(wfe))
            Expect(err).ToNot(HaveOccurred())

            // Verify parameters passed
            var pr tektonv1.PipelineRun
            Expect(fakeK8sClient.Get(ctx, client.ObjectKey{
                Name:      wfe.Name,
                Namespace: wfe.Namespace,
            }, &pr)).To(Succeed())

            paramMap := make(map[string]string)
            for _, p := range pr.Spec.Params {
                paramMap[p.Name] = p.Value.StringVal
            }
            Expect(paramMap["NAMESPACE"]).To(Equal("production"))
            Expect(paramMap["MEMORY_INCREMENT_MB"]).To(Equal("256"))
        })
    })
})

var _ = Describe("BR-WE-009: Resource Locking", func() {
    var (
        fakeK8sClient client.Client
        reconciler    *controller.WorkflowExecutionReconciler
        ctx           context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        // ... setup
    })

    Context("Parallel Execution Prevention", func() {
        It("should skip when another workflow targets same resource", func() {
            // Create first WFE (running)
            wfe1 := testutil.NewTestWorkflowExecution("wfe-running")
            wfe1.Spec.TargetResource = "production/deployment/payment-service"
            wfe1.Status.Phase = workflowexecutionv1.PhaseRunning
            Expect(fakeK8sClient.Create(ctx, wfe1)).To(Succeed())

            // Create second WFE targeting same resource
            wfe2 := testutil.NewTestWorkflowExecution("wfe-blocked")
            wfe2.Spec.TargetResource = "production/deployment/payment-service"
            Expect(fakeK8sClient.Create(ctx, wfe2)).To(Succeed())

            // Reconcile second WFE - should be skipped
            _, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest(wfe2))
            Expect(err).ToNot(HaveOccurred())

            // Verify skipped with ResourceBusy reason
            var updated workflowexecutionv1.WorkflowExecution
            Expect(fakeK8sClient.Get(ctx, client.ObjectKeyFromObject(wfe2), &updated)).To(Succeed())
            Expect(updated.Status.Phase).To(Equal(workflowexecutionv1.PhaseSkipped))
            Expect(updated.Status.SkipDetails.Reason).To(Equal("ResourceBusy"))
        })
    })

    Context("Cooldown Prevention", func() {
        It("should skip when same workflow ran recently", func() {
            // Create completed WFE
            wfe1 := testutil.NewTestWorkflowExecution("wfe-completed")
            wfe1.Spec.TargetResource = "production/deployment/payment-service"
            wfe1.Spec.WorkflowRef.WorkflowID = "increase-memory"
            wfe1.Status.Phase = workflowexecutionv1.PhaseCompleted
            wfe1.Status.CompletionTime = &metav1.Time{Time: time.Now().Add(-1 * time.Minute)}
            Expect(fakeK8sClient.Create(ctx, wfe1)).To(Succeed())

            // Create new WFE with same workflow + target
            wfe2 := testutil.NewTestWorkflowExecution("wfe-cooldown")
            wfe2.Spec.TargetResource = "production/deployment/payment-service"
            wfe2.Spec.WorkflowRef.WorkflowID = "increase-memory"
            Expect(fakeK8sClient.Create(ctx, wfe2)).To(Succeed())

            // Reconcile - should be skipped (cooldown)
            _, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest(wfe2))
            Expect(err).ToNot(HaveOccurred())

            var updated workflowexecutionv1.WorkflowExecution
            Expect(fakeK8sClient.Get(ctx, client.ObjectKeyFromObject(wfe2), &updated)).To(Succeed())
            Expect(updated.Status.Phase).To(Equal(workflowexecutionv1.PhaseSkipped))
            Expect(updated.Status.SkipDetails.Reason).To(Equal("RecentlyRemediated"))
        })
    })
})

var _ = Describe("BR-WE-003: Status Synchronization", func() {
    Context("PipelineRun Status Mapping", func() {
        It("should map PipelineRun success to Completed phase", func() {
            wfe := testutil.NewTestWorkflowExecution("wfe-success")
            wfe.Status.Phase = workflowexecutionv1.PhaseRunning
            Expect(fakeK8sClient.Create(ctx, wfe)).To(Succeed())

            // Create completed PipelineRun
            pr := &tektonv1.PipelineRun{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      wfe.Name,
                    Namespace: wfe.Namespace,
                },
                Status: tektonv1.PipelineRunStatus{
                    Status: duckv1.Status{
                        Conditions: duckv1.Conditions{
                            {Type: "Succeeded", Status: "True"},
                        },
                    },
                },
            }
            Expect(fakeK8sClient.Create(ctx, pr)).To(Succeed())

            // Reconcile - should update to Completed
            _, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest(wfe))
            Expect(err).ToNot(HaveOccurred())

            var updated workflowexecutionv1.WorkflowExecution
            Expect(fakeK8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), &updated)).To(Succeed())
            Expect(updated.Status.Phase).To(Equal(workflowexecutionv1.PhaseCompleted))
            Expect(updated.Status.Outcome).To(Equal(workflowexecutionv1.OutcomeSuccess))
        })

        It("should map PipelineRun failure to Failed phase with details", func() {
            wfe := testutil.NewTestWorkflowExecution("wfe-failure")
            wfe.Status.Phase = workflowexecutionv1.PhaseRunning
            Expect(fakeK8sClient.Create(ctx, wfe)).To(Succeed())

            // Create failed PipelineRun
            pr := &tektonv1.PipelineRun{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      wfe.Name,
                    Namespace: wfe.Namespace,
                },
                Status: tektonv1.PipelineRunStatus{
                    Status: duckv1.Status{
                        Conditions: duckv1.Conditions{
                            {
                                Type:    "Succeeded",
                                Status:  "False",
                                Reason:  "TaskRunFailed",
                                Message: "Task increase-memory failed: exit code 1",
                            },
                        },
                    },
                },
            }
            Expect(fakeK8sClient.Create(ctx, pr)).To(Succeed())

            _, err := reconciler.Reconcile(ctx, testutil.NewReconcileRequest(wfe))
            Expect(err).ToNot(HaveOccurred())

            var updated workflowexecutionv1.WorkflowExecution
            Expect(fakeK8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), &updated)).To(Succeed())
            Expect(updated.Status.Phase).To(Equal(workflowexecutionv1.PhaseFailed))
            Expect(updated.Status.FailureDetails).ToNot(BeNil())
            Expect(updated.Status.FailureDetails.Reason).To(Equal("TaskRunFailed"))
        })
    })
})
```

---

### Integration Tests

**Test Directory**: [test/integration/workflowexecution/](../../../test/integration/workflowexecution/)
**Coverage Target**: 50%
**Confidence**: 80-85%
**Execution**: `make test-integration-workflowexecution`

**Focus Areas**:
- Real Tekton PipelineRun creation with EnvTest
- Status synchronization from PipelineRun to WorkflowExecution
- Resource locking with concurrent reconciliations

---

### E2E Tests

**Test Directory**: [test/e2e/workflowexecution/](../../../test/e2e/workflowexecution/)
**Coverage Target**: 15%
**Confidence**: 90-95%
**Execution**: `make test-e2e-workflowexecution`

**Test Scenarios**:
1. Complete workflow execution with Tekton (Kind + Tekton Pipelines)
2. Resource locking prevents parallel execution
3. Cooldown prevents redundant sequential execution
4. Failure details extraction from failed PipelineRun

---

### Test Level Selection

| Scenario | Test Level | Rationale |
|----------|-----------|-----------|
| PipelineRun building logic | Unit | Pure logic, no K8s API needed |
| Parameter conversion | Unit | Deterministic mapping |
| Resource lock checking | Unit | In-memory lock checks |
| PipelineRun creation | Integration | Requires real K8s API |
| Status sync from Tekton | Integration | Requires Tekton CRDs |
| Full workflow with Tekton | E2E | Requires running Tekton |

---

