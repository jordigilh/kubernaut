# WorkflowExecution - Test Scenarios

**Parent Document**: [IMPLEMENTATION_PLAN_V3.0.md](./IMPLEMENTATION_PLAN_V3.0.md)
**Version**: v1.0
**Last Updated**: 2025-12-03
**Status**: ✅ Ready for Implementation

---

## Document Purpose

This appendix defines all test scenarios for the WorkflowExecution Controller, organized by component and test type. Aligned with [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](../../../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md) Test Scenarios section.

---

## 🎯 Test Count Summary

| Component | Happy Path | Edge Cases | Error Handling | **Total** |
|-----------|------------|------------|----------------|-----------|
| pipelineRunName | 3 | 2 | 0 | **5** |
| buildPipelineRun | 4 | 3 | 1 | **8** |
| checkResourceLock | 3 | 4 | 1 | **8** |
| reconcilePending | 3 | 5 | 3 | **11** |
| reconcileRunning | 3 | 3 | 2 | **8** |
| reconcileTerminal | 2 | 3 | 1 | **6** |
| reconcileDelete | 2 | 2 | 1 | **5** |
| extractFailureDetails | 3 | 2 | 1 | **6** |
| validateSpec | 2 | 4 | 2 | **8** |
| **Total Unit Tests** | **25** | **28** | **12** | **65** |
| **Integration Tests** | | | | **~25** |
| **E2E Tests** | | | | **~10** |
| **Grand Total** | | | | **~100** |

---

## 📋 Unit Test Scenarios by Component

### Component: pipelineRunName (`test/unit/workflowexecution/naming_test.go`)

**Happy Path (3 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| PN-HP-01 | Generate deterministic name | `production/deployment/app` | `wfe-<first 16 chars of sha256>` |
| PN-HP-02 | Same input = same output | Same targetResource twice | Identical names |
| PN-HP-03 | Different inputs = different names | Different targetResources | Different names |

**Edge Cases (2 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| PN-EC-01 | Very long targetResource | 500 char string | Valid 20-char name |
| PN-EC-02 | Special characters | `ns/deploy/app-v2.1` | Valid name (hashed) |

```go
var _ = Describe("pipelineRunName", func() {
    DescribeTable("should generate deterministic names",
        func(targetResource, expectedPrefix string) {
            name := pipelineRunName(targetResource)
            Expect(name).To(HavePrefix("wfe-"))
            Expect(len(name)).To(Equal(20)) // "wfe-" + 16 hex chars
        },
        Entry("standard format", "production/deployment/app", "wfe-"),
        Entry("with special chars", "ns/deploy/app-v2.1", "wfe-"),
        Entry("very long input", strings.Repeat("a", 500), "wfe-"),
    )

    It("should be deterministic", func() {
        name1 := pipelineRunName("production/deployment/app")
        name2 := pipelineRunName("production/deployment/app")
        Expect(name1).To(Equal(name2))
    })
})
```

---

### Component: buildPipelineRun (`test/unit/workflowexecution/pipelinerun_test.go`)

**Happy Path (4 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| PR-HP-01 | Build with bundle resolver | Valid WFE | PipelineRef uses bundles resolver |
| PR-HP-02 | Pass parameters | WFE with 3 params | All params in PipelineRun.Spec.Params |
| PR-HP-03 | Set execution namespace | WFE from any namespace | PR in `kubernaut-workflows` |
| PR-HP-04 | Set labels for tracking | Valid WFE | Labels include source WFE name/namespace |

**Edge Cases (3 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| PR-EC-01 | Empty parameters | WFE with no params | Empty Params slice |
| PR-EC-02 | Long parameter values | 10KB param value | Value preserved |
| PR-EC-03 | Unicode in params | Unicode param value | Correctly encoded |

**Error Handling (1 test)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| PR-ER-01 | Nil WFE | nil | Panic or error (defensive) |

```go
var _ = Describe("buildPipelineRun", func() {
    var reconciler *WorkflowExecutionReconciler

    BeforeEach(func() {
        reconciler = &WorkflowExecutionReconciler{
            ExecutionNamespace: "kubernaut-workflows",
            ServiceAccountName: "kubernaut-workflow-runner",
        }
    })

    Context("bundle resolver configuration", func() {
        It("should use bundles resolver", func() {
            wfe := newTestWFE("test", "production/deployment/app")
            wfe.Spec.WorkflowRef.ContainerImage = "ghcr.io/kubernaut/workflows/disk-cleanup@sha256:abc"

            pr := reconciler.buildPipelineRun(wfe)

            Expect(pr.Spec.PipelineRef.ResolverRef.Resolver).To(Equal("bundles"))
            Expect(pr.Spec.PipelineRef.ResolverRef.Params).To(ContainElement(
                HaveField("Name", "bundle"),
            ))
        })
    })

    DescribeTable("should handle parameters correctly",
        func(params map[string]string, expectedCount int) {
            wfe := newTestWFE("test", "production/deployment/app")
            wfe.Spec.Parameters = params

            pr := reconciler.buildPipelineRun(wfe)

            Expect(len(pr.Spec.Params)).To(Equal(expectedCount))
        },
        Entry("no parameters", map[string]string{}, 0),
        Entry("single parameter", map[string]string{"KEY": "value"}, 1),
        Entry("multiple parameters", map[string]string{"A": "1", "B": "2", "C": "3"}, 3),
    )

    It("should create in execution namespace", func() {
        wfe := newTestWFE("test", "production/deployment/app")
        wfe.Namespace = "source-namespace"

        pr := reconciler.buildPipelineRun(wfe)

        Expect(pr.Namespace).To(Equal("kubernaut-workflows"))
        Expect(pr.Namespace).ToNot(Equal(wfe.Namespace))
    })
})
```

---

### Component: checkResourceLock (`test/unit/workflowexecution/lock_test.go`)

**Happy Path (3 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| RL-HP-01 | No existing WFE for target | Unique targetResource | blocked=false |
| RL-HP-02 | Existing completed WFE | Past cooldown | blocked=false |
| RL-HP-03 | Different target | Different targetResource | blocked=false |

**Edge Cases (4 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| RL-EC-01 | Running WFE exists | Same targetResource | blocked=true, reason=ResourceBusy |
| RL-EC-02 | Within cooldown period | Same target, completed 1min ago | blocked=true, reason=RecentlyRemediated |
| RL-EC-03 | Exactly at cooldown boundary | 5min ago | blocked=false |
| RL-EC-04 | Same target, different workflow | Running WFE for same target | blocked=true (V1.0 strict) |

**Error Handling (1 test)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| RL-ER-01 | K8s API list fails | API error | Returns error, no decision |

```go
var _ = Describe("checkResourceLock", func() {
    var (
        fakeClient client.Client
        reconciler *WorkflowExecutionReconciler
        ctx        context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        scheme := testutil.NewTestScheme()
        fakeClient = fake.NewClientBuilder().
            WithScheme(scheme).
            WithIndex(&workflowexecutionv1.WorkflowExecution{},
                "spec.targetResource",
                targetResourceIndexer).
            Build()

        reconciler = &WorkflowExecutionReconciler{
            Client:         fakeClient,
            CooldownPeriod: 5 * time.Minute,
        }
    })

    Context("parallel execution prevention", func() {
        It("should block when Running WFE exists for target", func() {
            // Create running WFE
            wfe := newTestWFE("existing", "production/deployment/app")
            wfe.Status.Phase = workflowexecutionv1.PhaseRunning
            Expect(fakeClient.Create(ctx, wfe)).To(Succeed())
            Expect(fakeClient.Status().Update(ctx, wfe)).To(Succeed())

            blocked, reason := reconciler.checkResourceLock(ctx, "production/deployment/app")

            Expect(blocked).To(BeTrue())
            Expect(reason).To(Equal("ResourceBusy"))
        })

        It("should not block when no Running WFE exists", func() {
            blocked, _ := reconciler.checkResourceLock(ctx, "unique/deployment/app")

            Expect(blocked).To(BeFalse())
        })
    })

    Context("cooldown enforcement", func() {
        DescribeTable("should enforce cooldown period",
            func(completedAgo time.Duration, expectBlocked bool) {
                wfe := newTestWFE("completed", "production/deployment/app")
                wfe.Status.Phase = workflowexecutionv1.PhaseCompleted
                wfe.Status.CompletionTime = &metav1.Time{
                    Time: time.Now().Add(-completedAgo),
                }
                Expect(fakeClient.Create(ctx, wfe)).To(Succeed())
                Expect(fakeClient.Status().Update(ctx, wfe)).To(Succeed())

                blocked, _ := reconciler.checkResourceLock(ctx, "production/deployment/app")

                Expect(blocked).To(Equal(expectBlocked))
            },
            Entry("1 minute ago (within cooldown)", 1*time.Minute, true),
            Entry("4 minutes ago (within cooldown)", 4*time.Minute, true),
            Entry("5 minutes ago (at boundary)", 5*time.Minute, false),
            Entry("10 minutes ago (past cooldown)", 10*time.Minute, false),
        )
    })
})
```

---

### Component: reconcilePending (`test/unit/workflowexecution/reconcile_pending_test.go`)

**Happy Path (3 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| RP-HP-01 | Create PipelineRun | Valid WFE, no locks | Phase=Running, PR created |
| RP-HP-02 | Valid spec | All required fields | No validation error |
| RP-HP-03 | First reconcile | New WFE | Finalizer added |

**Edge Cases (5 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| RP-EC-01 | Lock exists | Running WFE for target | Phase=Skipped, reason=ResourceBusy |
| RP-EC-02 | Cooldown active | Recent completion | Phase=Skipped, reason=RecentlyRemediated |
| RP-EC-03 | PR already exists | Race condition | Phase=Skipped (AlreadyExists caught) |
| RP-EC-04 | Finalizer already present | Reconcile again | No duplicate finalizer |
| RP-EC-05 | Empty targetResource | Missing field | Phase=Failed, reason=ConfigurationError |

**Error Handling (3 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| RP-ER-01 | PR creation fails | Tekton API error | Requeue with backoff |
| RP-ER-02 | Status update fails | Conflict | Requeue |
| RP-ER-03 | RBAC error | Forbidden | Phase=Failed, reason=PermissionDenied |

---

### Component: reconcileRunning (`test/unit/workflowexecution/reconcile_running_test.go`)

**Happy Path (3 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| RR-HP-01 | PR succeeded | Succeeded=True | Phase=Completed |
| RR-HP-02 | PR failed | Succeeded=False | Phase=Failed with details |
| RR-HP-03 | PR still running | Succeeded=Unknown | Requeue after 10s |

**Edge Cases (3 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| RR-EC-01 | PR deleted externally | NotFound | Phase=Failed, reason=PipelineRunDeleted |
| RR-EC-02 | Multiple TaskRuns failed | Complex failure | FailureDetails has first failed task |
| RR-EC-03 | PR cancelled | Status=Cancelled | Phase=Failed, reason=Cancelled |

**Error Handling (2 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| RR-ER-01 | API error getting PR | Transient error | Requeue with backoff |
| RR-ER-02 | Status update conflict | Conflict | Requeue |

---

### Component: reconcileTerminal (`test/unit/workflowexecution/reconcile_terminal_test.go`)

**Happy Path (2 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| RT-HP-01 | After cooldown | 6min since completion | PR deleted, lock released |
| RT-HP-02 | Completed WFE | Phase=Completed | No requeue (terminal) |

**Edge Cases (3 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| RT-EC-01 | Within cooldown | 2min since completion | Requeue with remaining duration |
| RT-EC-02 | PR already deleted | NotFound | Success (idempotent) |
| RT-EC-03 | Failed WFE | Phase=Failed | Same cooldown behavior |

**Error Handling (1 test)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| RT-ER-01 | PR delete fails | Transient error | Requeue with backoff |

---

## 🔗 Integration Test Scenarios

**Location**: `test/integration/workflowexecution/`

### CRD Lifecycle (`lifecycle_test.go`)

| ID | Scenario | Expected Outcome |
|----|----------|------------------|
| IL-01 | Create WFE | PR created in kubernaut-workflows |
| IL-02 | Status sync | WFE status updated from PR status |
| IL-03 | Finalizer cleanup | PR deleted when WFE deleted |
| IL-04 | Phase transitions | Pending → Running → Completed |

### Resource Locking (`locking_test.go`)

| ID | Scenario | Expected Outcome |
|----|----------|------------------|
| IL-05 | Parallel block | Second WFE skipped when first running |
| IL-06 | Cooldown block | New WFE skipped within 5min |
| IL-07 | Race condition | AlreadyExists handled gracefully |
| IL-08 | Lock release | PR deleted after cooldown |

### Cross-Namespace (`namespace_test.go`)

| ID | Scenario | Expected Outcome |
|----|----------|------------------|
| IL-09 | Source != execution namespace | PR created in kubernaut-workflows |
| IL-10 | Watch triggers reconcile | WFE reconciled when PR changes |

---

## 🌐 E2E Test Scenarios

**Location**: `test/e2e/workflowexecution/`

### Complete Workflow (`workflow_test.go`)

| ID | Scenario | Expected Outcome |
|----|----------|------------------|
| E2E-01 | Happy path execution | WFE Completed, target remediated |
| E2E-02 | Failed workflow | WFE Failed, FailureDetails populated |
| E2E-03 | Resource locking | Concurrent requests handled |

### Business Requirements (`br_test.go`)

| ID | BR | Scenario | Expected Outcome |
|----|----|----|------------------|
| E2E-04 | BR-WE-001 | Complete within SLA | < 30s total time |
| E2E-05 | BR-WE-009 | Cost savings | 90% skipped in duplicate scenario |
| E2E-06 | BR-WE-004 | Actionable failures | Natural language summary present |

---

## 📊 Edge Case Categories

### Category 1: Timing and Race Conditions

| Scenario | Test Location | Mitigation |
|----------|---------------|------------|
| Concurrent PR creation | Unit: RP-EC-03 | AlreadyExists → Skipped |
| Status update conflict | Unit: RP-ER-02 | Requeue with fresh version |
| PR deleted during reconcile | Unit: RR-EC-01 | Mark as Failed |

### Category 2: Resource State

| Scenario | Test Location | Handling |
|----------|---------------|----------|
| Missing PR | Unit: RR-EC-01 | Failed state |
| Orphaned PR | Integration: IL-08 | Finalizer cleanup |
| Stale WFE | Unit: RL-EC-03 | Cooldown boundary check |

### Category 3: External Dependencies

| Scenario | Test Location | Handling |
|----------|---------------|----------|
| Tekton unavailable | Integration | Controller crash (ADR-030) |
| Network timeout | Unit: PR-ER-01 | Retry with backoff |
| RBAC denied | Unit: RP-ER-03 | PermissionDenied status |

---

## References

- [Test Scenarios Template](../../../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md#test-scenarios-by-component-define-upfront-per-tdd)
- [testing-strategy.md](../testing-strategy.md)
- [TESTING_GUIDELINES.md](../../../../development/business-requirements/TESTING_GUIDELINES.md)

