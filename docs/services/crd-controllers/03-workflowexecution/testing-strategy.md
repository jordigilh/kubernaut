## Testing Strategy

**Version**: 5.3
**Last Updated**: 2025-12-08
**CRD API Group**: `workflowexecution.kubernaut.ai/v1alpha1`
**Status**: ✅ COMPLIANT - Defense-in-Depth Testing Strategy

---

## Changelog

### Version 5.2 (2025-12-08)
- ✅ **MAJOR**: Integration tests now run WITH controller (EnvTest + Tekton CRDs)
- ✅ **ACHIEVED**: 60.5% integration coverage (target: >50%)
- ✅ **ACHIEVED**: 71.7% unit coverage (target: 70%+)
- ✅ **Added**: Full reconciliation tests with PipelineRun creation
- ✅ **Added**: Status sync tests with simulated PipelineRun completion
- ✅ **Added**: Resource locking tests with live controller
- ✅ **Fixed**: All tests use Eventually patterns for controller race conditions

### Version 5.1 (2025-12-06)
- ✅ **Fixed**: Aligned integration coverage with [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc) microservices mandate
- ✅ **Changed**: Integration test coverage from ~20% to >50% (microservices architecture)
- ✅ **Added**: Rationale for higher integration coverage (CRD-based coordination, watch patterns)

### Version 5.0 (2025-12-03)
- ✅ **Fixed**: Aligned with [TESTING_GUIDELINES.md](../../../../development/business-requirements/TESTING_GUIDELINES.md)
- ✅ **Separated**: Business Requirement Tests (BR-*) from Unit Tests
- ✅ **Added**: True business outcome tests (SLAs, efficiency, reliability)
- ✅ **Renamed**: Implementation-focused tests no longer use BR-* prefix

### Version 4.0 (2025-12-02)
- Rewritten for Tekton PipelineRun architecture

---

**Testing Framework References**:
- [TESTING_GUIDELINES.md](../../../../development/business-requirements/TESTING_GUIDELINES.md) - When to use BR vs Unit tests
- [.cursor/rules/03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc) - Defense-in-depth strategy

---

## 🎯 Test Type Decision Framework

Per [TESTING_GUIDELINES.md](../../../../development/business-requirements/TESTING_GUIDELINES.md):

```
📝 QUESTION: What are you trying to validate?

├─ 💼 "Does it solve the business problem?"
│  └─ ► BUSINESS REQUIREMENT TEST (BR-WE-*)
│
└─ 🔧 "Does the code work correctly?"
   └─ ► UNIT TEST (no BR prefix)
```

### Test Type Comparison

| Aspect | Business Requirement Tests | Unit Tests |
|--------|----------------------------|------------|
| **Purpose** | Validate business value delivery | Validate implementation correctness |
| **Focus** | External behavior & outcomes | Internal code mechanics |
| **Naming** | BR-WE-XXX prefix | Function/method name |
| **Audience** | Business stakeholders + developers | Developers only |
| **Metrics** | SLAs, efficiency, reliability | Coverage, edge cases |

---

### Testing Pyramid

Following Kubernaut's defense-in-depth testing strategy per [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc):

| Test Type | Target Coverage | Actual Coverage | Test Count | Focus | Status |
|-----------|----------------|-----------------|------------|-------|--------|
| **Unit Tests** | 70%+ | **71.7%** | 173 | Controller logic, PipelineRun building, resource locking | ✅ COMPLIANT |
| **Integration Tests** | >50% | **60.5%** | 41 | CRD interactions, Tekton PipelineRun creation, status sync, **audit events** | ✅ COMPLIANT |
| **E2E / BR Tests** | 10-15% | ~9 tests | 9 | Complete workflow execution, business SLAs | ✅ COMPLIANT |

**Rationale for >50% Integration Coverage** (microservices mandate):
- CRD-based coordination between WorkflowExecution and Tekton
- Watch-based status propagation (difficult to unit test)
- Cross-namespace PipelineRun lifecycle (requires real K8s API)
- Owner reference and finalizer lifecycle management
- **Audit event emission during reconciliation** (requires running controller)

**WorkflowExecution Focus Areas**:
1. **PipelineRun creation** - Bundle resolver, parameter passing
2. **Status synchronization** - Mapping Tekton conditions to WE phases
3. **Resource locking** - Parallel execution prevention, cooldown

---

## 💼 Business Requirement Tests (BR-WE-*)

**Purpose**: Validate business value delivery - SLAs, efficiency, reliability
**Audience**: Business stakeholders + developers
**Directory**: `test/e2e/workflowexecution/` (alongside E2E tests)
**Execution**: `make test-e2e-workflowexecution`

### ✅ BR Tests Focus On:
- User-facing outcomes (remediation completes)
- Performance SLAs (execution time)
- Business efficiency (no wasted executions)
- Reliability (failures reported correctly)

### BR Test Examples

```go
// test/e2e/workflowexecution/business_requirements_test.go
package workflowexecution_e2e

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "time"
)

var _ = Describe("BR-WE-001: Workflow Remediation Completes Successfully", func() {
    // BUSINESS VALUE: System can automatically remediate issues
    // STAKEHOLDER: Operations team needs automated issue resolution

    It("should complete remediation within 30-second SLA", func() {
        // Given: A pod with memory pressure
        createPodWithMemoryPressure("test-pod", "default")

        // When: WorkflowExecution is created for memory increase
        startTime := time.Now()
        wfe := createWorkflowExecution("increase-memory", "default/pod/test-pod")

        // Then: Workflow completes within SLA
        Eventually(func() string {
            return getWFEPhase(wfe.Name)
        }, 30*time.Second).Should(Equal("Completed"))

        duration := time.Since(startTime)
        Expect(duration).To(BeNumerically("<", 30*time.Second))

        // Business outcome: Pod memory increased
        Expect(getPodMemoryLimit("test-pod")).To(BeNumerically(">", originalMemory))
    })
})

var _ = Describe("BR-WE-009: Prevent Wasteful Duplicate Remediations", func() {
    // BUSINESS VALUE: No wasted compute/time on redundant operations
    // STAKEHOLDER: Finance team cares about operational efficiency

    It("should prevent parallel remediations on same resource (cost savings)", func() {
        // Given: 10 identical remediation requests arrive simultaneously
        target := "production/deployment/payment-service"
        var wfes []string
        for i := 0; i < 10; i++ {
            wfes = append(wfes, createWorkflowExecution("disk-cleanup", target).Name)
        }

        // Then: Only 1 should execute, 9 should be skipped
        executedCount := 0
        skippedCount := 0
        for _, name := range wfes {
            phase := getWFEPhase(name)
            if phase == "Running" || phase == "Completed" {
                executedCount++
            } else if phase == "Skipped" {
                skippedCount++
            }
        }

        // Business outcome: 90% cost reduction in duplicate scenarios
        Expect(executedCount).To(Equal(1))
        Expect(skippedCount).To(Equal(9))
    })

    It("should prevent redundant sequential remediations (cooldown)", func() {
        // Given: Workflow just completed on target
        target := "production/deployment/app"
        wfe1 := createWorkflowExecution("restart-pod", target)
        Eventually(getWFEPhase(wfe1.Name)).Should(Equal("Completed"))

        // When: Same workflow requested again immediately
        wfe2 := createWorkflowExecution("restart-pod", target)

        // Then: Second is skipped (within cooldown)
        Eventually(getWFEPhase(wfe2.Name)).Should(Equal("Skipped"))
        Expect(getWFESkipReason(wfe2.Name)).To(Equal("RecentlyRemediated"))
    })
})

var _ = Describe("BR-WE-004: Failure Details Enable Recovery Decisions", func() {
    // BUSINESS VALUE: Operations can understand and act on failures
    // STAKEHOLDER: On-call engineers need actionable failure information

    It("should provide human-readable failure explanation", func() {
        // Given: A workflow that will fail (insufficient permissions)
        wfe := createWorkflowExecution("scale-deployment", "restricted/deployment/app")

        // When: Workflow fails
        Eventually(getWFEPhase(wfe.Name)).Should(Equal("Failed"))

        // Then: Failure details are actionable
        details := getWFEFailureDetails(wfe.Name)
        Expect(details.Reason).To(Equal("PermissionDenied"))
        Expect(details.NaturalLanguageSummary).To(ContainSubstring("ServiceAccount"))
        Expect(details.NaturalLanguageSummary).To(ContainSubstring("RBAC"))

        // Business outcome: On-call can take immediate action
    })
})
```

---

## 🔧 Unit Tests (Implementation Correctness)

**Purpose**: Validate internal code mechanics work correctly
**Audience**: Developers only
**Directory**: `test/unit/workflowexecution/`
**Coverage Target**: 70%
**Execution**: `make test-unit-workflowexecution`

### ✅ Unit Tests Focus On:
- Function/method behavior
- Error handling & edge cases
- Internal logic validation
- Interface compliance

### ❌ Unit Tests Should NOT:
- Use BR-* prefixes (those are for business tests)
- Test business outcomes
- Validate SLAs

### Unit Test Examples

```go
// test/unit/workflowexecution/controller_test.go
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

// ============================================================
// PipelineRun Building Tests (Implementation Correctness)
// ============================================================

var _ = Describe("buildPipelineRun", func() {
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
            Client:             fakeK8sClient,
            Scheme:             scheme,
            ExecutionNamespace: "kubernaut-workflows",
        }
    })

    Context("bundle resolver configuration", func() {
        It("should use bundles resolver with OCI image", func() {
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
                },
            }

            pr := reconciler.BuildPipelineRun(wfe)

            Expect(pr.Spec.PipelineRef.ResolverRef.Resolver).To(Equal("bundles"))
            Expect(pr.Spec.PipelineRef.ResolverRef.Params).To(ContainElement(
                HaveField("Name", "bundle"),
            ))
        })
    })

    Context("parameter conversion", func() {
        It("should convert map to Tekton params", func() {
            wfe := testutil.NewTestWorkflowExecution("wfe-params-test")
            wfe.Spec.Parameters = map[string]string{
                "NAMESPACE":           "production",
                "MEMORY_INCREMENT_MB": "256",
            }

            pr := reconciler.BuildPipelineRun(wfe)

            paramMap := make(map[string]string)
            for _, p := range pr.Spec.Params {
                paramMap[p.Name] = p.Value.StringVal
            }
            Expect(paramMap["NAMESPACE"]).To(Equal("production"))
            Expect(paramMap["MEMORY_INCREMENT_MB"]).To(Equal("256"))
        })
    })

    Context("execution namespace", func() {
        It("should create PipelineRun in configured namespace", func() {
            wfe := testutil.NewTestWorkflowExecution("wfe-ns-test")
            wfe.Namespace = "source-namespace"

            pr := reconciler.BuildPipelineRun(wfe)

            Expect(pr.Namespace).To(Equal("kubernaut-workflows"))
            Expect(pr.Namespace).ToNot(Equal(wfe.Namespace))
        })
    })
})

// ============================================================
// Resource Lock Checking Tests (Internal Logic)
// ============================================================

var _ = Describe("checkResourceLock", func() {
    var (
        fakeK8sClient client.Client
        reconciler    *controller.WorkflowExecutionReconciler
        ctx           context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        // ... setup with indexed field
    })

    Context("parallel execution detection", func() {
        It("should return blocked when Running WFE exists for target", func() {
            // Create running WFE
            wfe1 := testutil.NewTestWorkflowExecution("wfe-running")
            wfe1.Spec.TargetResource = "production/deployment/payment-service"
            wfe1.Status.Phase = workflowexecutionv1.PhaseRunning
            Expect(fakeK8sClient.Create(ctx, wfe1)).To(Succeed())

            // Check lock
            blocked, reason := reconciler.CheckResourceLock(ctx, "production/deployment/payment-service")

            Expect(blocked).To(BeTrue())
            Expect(reason).To(Equal("ResourceBusy"))
        })

        It("should return not blocked when no Running WFE for target", func() {
            blocked, _ := reconciler.CheckResourceLock(ctx, "other/deployment/app")

            Expect(blocked).To(BeFalse())
        })
    })

    Context("cooldown detection", func() {
        It("should detect recent completion within cooldown period", func() {
            // Create completed WFE 1 minute ago
            wfe := testutil.NewTestWorkflowExecution("wfe-completed")
            wfe.Spec.TargetResource = "production/deployment/payment-service"
            wfe.Spec.WorkflowRef.WorkflowID = "increase-memory"
            wfe.Status.Phase = workflowexecutionv1.PhaseCompleted
            wfe.Status.CompletionTime = &metav1.Time{Time: time.Now().Add(-1 * time.Minute)}
            Expect(fakeK8sClient.Create(ctx, wfe)).To(Succeed())

            blocked, reason := reconciler.CheckCooldown(ctx, "production/deployment/payment-service", "increase-memory")

            Expect(blocked).To(BeTrue())
            Expect(reason).To(Equal("RecentlyRemediated"))
        })

        It("should allow execution after cooldown expires", func() {
            // Create completed WFE 10 minutes ago (default cooldown is 5 min)
            wfe := testutil.NewTestWorkflowExecution("wfe-old")
            wfe.Spec.TargetResource = "production/deployment/payment-service"
            wfe.Status.CompletionTime = &metav1.Time{Time: time.Now().Add(-10 * time.Minute)}
            Expect(fakeK8sClient.Create(ctx, wfe)).To(Succeed())

            blocked, _ := reconciler.CheckCooldown(ctx, "production/deployment/payment-service", "any-workflow")

            Expect(blocked).To(BeFalse())
        })
    })
})

// ============================================================
// Status Mapping Tests (Internal Logic)
// ============================================================

var _ = Describe("mapPipelineRunStatus", func() {
    Context("Tekton condition mapping", func() {
        It("should map Succeeded=True to Completed phase", func() {
            pr := &tektonv1.PipelineRun{
                Status: tektonv1.PipelineRunStatus{
                    Status: duckv1.Status{
                        Conditions: duckv1.Conditions{
                            {Type: "Succeeded", Status: "True"},
                        },
                    },
                },
            }

            phase, outcome := controller.MapPipelineRunStatus(pr)

            Expect(phase).To(Equal(workflowexecutionv1.PhaseCompleted))
            Expect(outcome).To(Equal(workflowexecutionv1.OutcomeSuccess))
        })

        It("should map Succeeded=False to Failed phase", func() {
            pr := &tektonv1.PipelineRun{
                Status: tektonv1.PipelineRunStatus{
                    Status: duckv1.Status{
                        Conditions: duckv1.Conditions{
                            {Type: "Succeeded", Status: "False", Reason: "TaskRunFailed"},
                        },
                    },
                },
            }

            phase, outcome := controller.MapPipelineRunStatus(pr)

            Expect(phase).To(Equal(workflowexecutionv1.PhaseFailed))
            Expect(outcome).To(Equal(workflowexecutionv1.OutcomeFailure))
        })

        It("should map Succeeded=Unknown to Running phase", func() {
            pr := &tektonv1.PipelineRun{
                Status: tektonv1.PipelineRunStatus{
                    Status: duckv1.Status{
                        Conditions: duckv1.Conditions{
                            {Type: "Succeeded", Status: "Unknown"},
                        },
                    },
                },
            }

            phase, _ := controller.MapPipelineRunStatus(pr)

            Expect(phase).To(Equal(workflowexecutionv1.PhaseRunning))
        })
    })
})

// ============================================================
// Failure Details Extraction Tests (Internal Logic)
// ============================================================

var _ = Describe("extractFailureDetails", func() {
    It("should extract reason and message from PipelineRun", func() {
        pr := &tektonv1.PipelineRun{
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

        details := controller.ExtractFailureDetails(pr)

        Expect(details.Reason).To(Equal("TaskRunFailed"))
        Expect(details.Message).To(ContainSubstring("exit code 1"))
        Expect(details.WasExecutionFailure).To(BeTrue())
    })

    It("should generate natural language summary", func() {
        pr := createFailedPipelineRunWithReason("PermissionDenied", "ServiceAccount lacks RBAC")

        details := controller.ExtractFailureDetails(pr)

        Expect(details.NaturalLanguageSummary).To(ContainSubstring("permission"))
    })
})
```

---

## Integration Tests

**Test Directory**: `test/integration/workflowexecution/`
**Coverage Target**: >50% (microservices mandate per [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc))
**Confidence**: 80-85%
**Execution**: `make test-integration-workflowexecution` (4 parallel procs by default)

**Focus Areas**:
- Real Tekton PipelineRun creation with EnvTest
- Status synchronization from PipelineRun to WorkflowExecution
- Resource locking with concurrent reconciliations
- CRD lifecycle (create, update, finalize)

```go
// test/integration/workflowexecution/lifecycle_test.go
var _ = Describe("WorkflowExecution CRD Lifecycle", func() {
    Context("PipelineRun creation", func() {
        It("should create PipelineRun in dedicated namespace", func() {
            wfe := createWFE("wfe-integration", "default/deployment/app")

            Eventually(func() bool {
                return pipelineRunExists("kubernaut-workflows", pipelineRunName(wfe.Spec.TargetResource))
            }, 10*time.Second).Should(BeTrue())
        })
    })

    Context("Status synchronization", func() {
        It("should update WFE status when PipelineRun completes", func() {
            wfe := createWFE("wfe-sync", "default/deployment/app")
            Eventually(getWFEPhase(wfe.Name)).Should(Equal("Running"))

            // Complete the PipelineRun
            completePipelineRun(pipelineRunName(wfe.Spec.TargetResource))

            Eventually(getWFEPhase(wfe.Name)).Should(Equal("Completed"))
        })
    })
})
```

---

## E2E Tests + Business Requirement Tests

**Test Directory**: `test/e2e/workflowexecution/`
**Coverage Target**: ~10%
**Confidence**: 90-95%
**Execution**: `make test-e2e-workflowexecution`

### Port Allocation (DD-TEST-001)

**Reference**: [DD-TEST-001-port-allocation-strategy.md](../../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md)

| Port Type | Value | Purpose |
|-----------|-------|---------|
| **NodePort** | 30085 | Service access in Kind cluster |
| **Metrics NodePort** | 30185 | Prometheus metrics endpoint |
| **Host Port** | 8085 | localhost access for tests |
| **Metrics Host Port** | 9185 | localhost metrics access |

**Kind Config**: `test/infrastructure/kind-workflowexecution-config.yaml`

```yaml
extraPortMappings:
- containerPort: 30085
  hostPort: 8085
  protocol: TCP
- containerPort: 30185
  hostPort: 9185
  protocol: TCP
```

**Note**: E2E tests in this service include **Business Requirement Tests** (BR-WE-*) because they validate end-to-end business outcomes.

**Test Scenarios** (BR-labeled = Business Requirement):
| Scenario | BR ID | Test Type |
|----------|-------|-----------|
| Complete remediation within SLA | BR-WE-001 | Business Requirement |
| Prevent parallel executions | BR-WE-009 | Business Requirement |
| Prevent redundant sequential executions | BR-WE-010 | Business Requirement |
| Failure details enable recovery | BR-WE-004 | Business Requirement |
| Full workflow with Tekton | - | Technical E2E |

---

## Test Level Selection Matrix

| What to Test | Test Type | Naming Convention | Why |
|--------------|-----------|-------------------|-----|
| Business outcomes (SLAs, efficiency) | BR Test (E2E) | `BR-WE-XXX: description` | Validates business value |
| PipelineRun building logic | Unit | `buildPipelineRun` | Pure logic, no K8s API |
| Parameter conversion | Unit | `convertParameters` | Deterministic mapping |
| Resource lock checking | Unit | `checkResourceLock` | In-memory checks |
| CRD creation in K8s API | Integration | `WorkflowExecution CRD Lifecycle` | Requires real K8s API |
| Status sync from Tekton | Integration | `Status synchronization` | Requires Tekton CRDs |
| Full workflow execution | E2E/BR | `BR-WE-001: Workflow Remediation` | Requires running Tekton |

---

## Summary: BR vs Unit Test Assignment

| BR ID | Description | Test Type | Rationale |
|-------|-------------|-----------|-----------|
| BR-WE-001 | PipelineRun creation | **Unit** + **BR E2E** | Unit tests implementation, BR tests business outcome |
| BR-WE-002 | Parameter passing | **Unit** | Implementation detail |
| BR-WE-003 | Status monitoring | **Unit** + **Integration** | Implementation + API interaction |
| BR-WE-004 | Failure details | **Unit** + **BR E2E** | Unit tests extraction, BR tests actionability |
| BR-WE-005 | **Audit events** | **Unit** + **Integration** | **Field validation + reconciliation emission** |
| BR-WE-006 | Phase updates | **Unit** | Implementation detail |
| BR-WE-007 | External PipelineRun deletion | **Unit** + **Integration** | Logic + API interaction |
| BR-WE-008 | Prometheus metrics | **Unit** + **Integration** + **E2E** | Logic + scrape validation |
| BR-WE-009 | Parallel prevention | **Unit** + **BR E2E** | Unit tests logic, BR tests cost savings |
| BR-WE-010 | Cooldown | **Unit** + **BR E2E** | Unit tests logic, BR tests efficiency |
| BR-WE-011 | Target resource | **Unit** | Validation logic |
| BR-WE-012 | Exponential backoff | **Unit** + **Integration** | State persistence + backoff calculation |

**Key Insight**: Each BR may have BOTH:
- **Unit tests**: Test the implementation mechanics (no BR prefix)
- **BR tests**: Test the business outcome (BR-WE-XXX prefix, in E2E suite)

---

