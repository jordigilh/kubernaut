# Kubernetes Executor - Expansion Plan to 95% Confidence

> **DEPRECATED**: KubernetesExecution CRD and KubernetesExecutor service were eliminated by ADR-025 and replaced by Tekton TaskRun via WorkflowExecution. This documentation is retained for historical reference only. API types and CRD manifests have been removed from the codebase.

**Current Status**: 1,303 lines, 70% confidence
**Target Status**: 5,100 lines, 95% confidence
**Gap**: +3,797 lines
**Effort**: 14-17 hours

---

## Current State Analysis

### Existing Content (1,303 lines)
- ✅ Day 1 outline (foundation)
- ✅ Days 2-11 brief outlines (~70-120 lines each)
- ✅ **Rego Policy Integration ALREADY COMPLETE** ✅ (REGO_POLICY_INTEGRATION.md, 600 lines)
- ✅ Basic BR Coverage Matrix (170 lines, needs expansion)
- ⚠️ Missing: APDC detail, integration tests, EOD docs, error philosophy

### What's Missing vs Notification Standard
1. **APDC Phase Expansions**: 0/3 days fully detailed
2. **Integration Tests**: 0/3 complete tests
3. **EOD Documentation**: 0/3 milestone docs
4. **Error Handling Philosophy**: 0/1 document
5. **Table-Driven Test Examples**: 0 examples
6. **Production Deployment**: Brief only

### Advantage: 600 Lines Already Complete!
- Rego Policy Integration: 8 production-ready safety policies with unit tests
- Policy validation framework with test examples
- Comprehensive policy catalog

---

## Expansion Phase 1: APDC Day Expansions (+2,500 lines)

### Day 2: Rego Policy Tests & Job Creation (~850 lines)

**Location**: After current Day 2 outline, replace with full expansion

**Structure**:
```markdown
## 📅 Day 2: Rego Policy Integration & Job Creation (8h)

### ANALYSIS Phase (1h)
- Review existing Rego policies (REGO_POLICY_INTEGRATION.md ✅ complete)
- Search Kubernetes Job creation patterns
- Map BR-EXECUTOR-002 (Safety Validation) requirements
- Identify dependencies (Rego policy engine, Job API)

### PLAN Phase (1h)
- TDD Strategy: Unit tests for Rego evaluation (70%), Integration for Job execution (>50%)
- Integration points: Rego policy engine, Kubernetes Job API
- Success criteria: All safety policies enforced, Jobs created successfully
- Timeline: RED (2h) → GREEN (3h) → REFACTOR (2h)

### DO-RED: Rego Policy Validation Tests (2h)
**File**: `test/unit/kubernetesexecution/policy_validator_test.go`
**BR Coverage**: BR-EXECUTOR-002, BR-EXECUTOR-008

[~350 lines of complete Ginkgo test code]
```go
var _ = Describe("BR-EXECUTOR-002: Rego Policy Validation", func() {
    var (
        validator *policy.Validator
        executor  *kubernetesexecutionv1alpha1.KubernetesExecution
    )

    BeforeEach(func() {
        // Load production Rego policies from REGO_POLICY_INTEGRATION.md
        validator = policy.NewValidator(loadProductionPolicies())
        executor = createTestExecutor()
    })

    Context("when validating ScaleDeployment action", func() {
        It("should allow scaling within max replicas limit", func() {
            executor.Spec.Action = kubernetesexecutionv1alpha1.ActionTypeScaleDeployment
            executor.Spec.Parameters.ScaleReplicas = ptrInt32(5) // Max is 10

            result, err := validator.Validate(ctx, executor)
            Expect(err).ToNot(HaveOccurred())
            Expect(result.Allowed).To(BeTrue())
        })

        It("should deny scaling beyond max replicas limit", func() {
            executor.Spec.Action = kubernetesexecutionv1alpha1.ActionTypeScaleDeployment
            executor.Spec.Parameters.ScaleReplicas = ptrInt32(15) // Exceeds max of 10

            result, err := validator.Validate(ctx, executor)
            Expect(err).ToNot(HaveOccurred())
            Expect(result.Allowed).To(BeFalse())
            Expect(result.Reason).To(ContainSubstring("exceeds maximum allowed replicas"))
            Expect(result.ViolatedPolicy).To(Equal("scale-deployment-max-replicas"))
        })
    })

    DescribeTable("action safety validation",
        func(action kubernetesexecutionv1alpha1.ActionType, params kubernetesexecutionv1alpha1.ActionParameters, expectedAllowed bool, expectedReason string) {
            executor.Spec.Action = action
            executor.Spec.Parameters = params

            result, err := validator.Validate(ctx, executor)
            Expect(err).ToNot(HaveOccurred())
            Expect(result.Allowed).To(Equal(expectedAllowed))
            if !expectedAllowed {
                Expect(result.Reason).To(ContainSubstring(expectedReason))
            }
        },
        Entry("delete pod in production namespace - denied",
            kubernetesexecutionv1alpha1.ActionTypeDeletePod,
            kubernetesexecutionv1alpha1.ActionParameters{
                TargetResource: "pod/critical-app",
                Namespace:      "production",
            },
            false,
            "deletion in production namespace requires approval"),

        Entry("delete pod in dev namespace - allowed",
            kubernetesexecutionv1alpha1.ActionTypeDeletePod,
            kubernetesexecutionv1alpha1.ActionParameters{
                TargetResource: "pod/test-pod",
                Namespace:      "dev",
            },
            true,
            ""),

        Entry("cordon node without approval - denied",
            kubernetesexecutionv1alpha1.ActionTypeCordonNode,
            kubernetesexecutionv1alpha1.ActionParameters{
                TargetResource: "node/worker-1",
            },
            false,
            "cordoning nodes requires manual approval"),

        Entry("scale deployment to 0 replicas - denied",
            kubernetesexecutionv1alpha1.ActionTypeScaleDeployment,
            kubernetesexecutionv1alpha1.ActionParameters{
                TargetResource: "deployment/api-server",
                ScaleReplicas:  ptrInt32(0),
            },
            false,
            "scaling to 0 replicas not allowed"),

        Entry("update image with approved registry - allowed",
            kubernetesexecutionv1alpha1.ActionTypeUpdateImage,
            kubernetesexecutionv1alpha1.ActionParameters{
                TargetResource: "deployment/web-app",
                ImageName:      "registry.company.com/web-app:v2.0",
            },
            true,
            ""),

        Entry("update image with untrusted registry - denied",
            kubernetesexecutionv1alpha1.ActionTypeUpdateImage,
            kubernetesexecutionv1alpha1.ActionParameters{
                TargetResource: "deployment/web-app",
                ImageName:      "unknown-registry.io/suspicious:latest",
            },
            false,
            "image must be from approved registry"),

        // Additional 10+ entries covering all actions and policies
    )

    Context("custom Rego policy loading", func() {
        It("should load custom policies from ConfigMap", func() {
            customPolicy := `
package custom_validation

deny[msg] {
    input.spec.action == "CustomAction"
    not input.spec.approved_by
    msg := "CustomAction requires approval"
}
`
            validator.AddCustomPolicy("custom-policy", customPolicy)

            executor.Spec.Action = kubernetesexecutionv1alpha1.ActionTypeCustom
            // Missing approved_by field

            result, err := validator.Validate(ctx, executor)
            Expect(err).ToNot(HaveOccurred())
            Expect(result.Allowed).To(BeFalse())
            Expect(result.Reason).To(ContainSubstring("CustomAction requires approval"))
        })
    })
})

var _ = Describe("BR-EXECUTOR-003: Job Creation", func() {
    var (
        jobFactory *job.Factory
        executor   *kubernetesexecutionv1alpha1.KubernetesExecution
    )

    BeforeEach(func() {
        jobFactory = job.NewFactory(scheme)
        executor = createTestExecutor()
    })

    It("should create Job with correct kubectl command", func() {
        executor.Spec.Action = kubernetesexecutionv1alpha1.ActionTypeScaleDeployment
        executor.Spec.Parameters = kubernetesexecutionv1alpha1.ActionParameters{
            TargetResource: "deployment/web-app",
            Namespace:      "default",
            ScaleReplicas:  ptrInt32(5),
        }

        job, err := jobFactory.CreateJob(executor)
        Expect(err).ToNot(HaveOccurred())

        // Verify Job metadata
        Expect(job.Name).To(HavePrefix("executor-"))
        Expect(job.Namespace).To(Equal("kubernaut-executors"))
        Expect(job.Labels["executor-action"]).To(Equal("ScaleDeployment"))

        // Verify kubectl command
        container := job.Spec.Template.Spec.Containers[0]
        Expect(container.Image).To(Equal("bitnami/kubectl:latest"))
        Expect(container.Command).To(Equal([]string{"kubectl"}))
        Expect(container.Args).To(Equal([]string{
            "scale",
            "deployment/web-app",
            "--replicas=5",
            "-n", "default",
        }))
    })

    DescribeTable("kubectl command generation",
        func(action kubernetesexecutionv1alpha1.ActionType, params kubernetesexecutionv1alpha1.ActionParameters, expectedCommand []string) {
            executor.Spec.Action = action
            executor.Spec.Parameters = params

            job, err := jobFactory.CreateJob(executor)
            Expect(err).ToNot(HaveOccurred())

            container := job.Spec.Template.Spec.Containers[0]
            Expect(container.Args).To(Equal(expectedCommand))
        },
        Entry("ScaleDeployment",
            kubernetesexecutionv1alpha1.ActionTypeScaleDeployment,
            kubernetesexecutionv1alpha1.ActionParameters{
                TargetResource: "deployment/api",
                ScaleReplicas:  ptrInt32(3),
            },
            []string{"scale", "deployment/api", "--replicas=3"}),

        Entry("DeletePod",
            kubernetesexecutionv1alpha1.ActionTypeDeletePod,
            kubernetesexecutionv1alpha1.ActionParameters{
                TargetResource: "pod/crashlooping-pod",
                Namespace:      "default",
            },
            []string{"delete", "pod/crashlooping-pod", "-n", "default"}),

        Entry("CordonNode",
            kubernetesexecutionv1alpha1.ActionTypeCordonNode,
            kubernetesexecutionv1alpha1.ActionParameters{
                TargetResource: "node/worker-1",
            },
            []string{"cordon", "node/worker-1"}),

        // Additional 8+ entries for all action types
    )
})
```

**Expected Result**: Tests fail - PolicyValidator, JobFactory don't exist

### DO-GREEN: Minimal Policy Validator & Job Factory (3h)
**File**: `pkg/kubernetesexecution/policy/validator.go`
**BR Coverage**: BR-EXECUTOR-002

[~350 lines of implementation]
```go
package policy

import (
    "context"
    "fmt"

    "github.com/open-policy-agent/opa/rego"
    kubernetesexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/kubernetesexecution/v1alpha1"
)

// Validator validates KubernetesExecution against Rego policies
type Validator struct {
    policies map[string]*rego.PreparedEvalQuery
}

// ValidationResult contains policy evaluation result
type ValidationResult struct {
    Allowed        bool
    Reason         string
    ViolatedPolicy string
}

// NewValidator creates a new policy validator
func NewValidator(policies map[string]string) *Validator {
    validator := &Validator{
        policies: make(map[string]*rego.PreparedEvalQuery),
    }

    // Compile all Rego policies
    for name, policy := range policies {
        query, err := rego.New(
            rego.Query("data.kubernetes_executor.deny"),
            rego.Module(name, policy),
        ).PrepareForEval(context.Background())

        if err != nil {
            // Log error but continue with other policies
            continue
        }

        validator.policies[name] = &query
    }

    return validator
}

// Validate validates KubernetesExecution against all policies
func (v *Validator) Validate(ctx context.Context, executor *kubernetesexecutionv1alpha1.KubernetesExecution) (*ValidationResult, error) {
    // Evaluate each policy
    for policyName, query := range v.policies {
        results, err := query.Eval(ctx, rego.EvalInput(executor))
        if err != nil {
            return nil, fmt.Errorf("failed to evaluate policy %s: %w", policyName, err)
        }

        // Check for denials
        if len(results) > 0 && len(results[0].Expressions) > 0 {
            denials := results[0].Expressions[0].Value
            if denySlice, ok := denials.([]interface{}); ok && len(denySlice) > 0 {
                return &ValidationResult{
                    Allowed:        false,
                    Reason:         fmt.Sprintf("%v", denySlice[0]),
                    ViolatedPolicy: policyName,
                }, nil
            }
        }
    }

    // All policies passed
    return &ValidationResult{
        Allowed: true,
    }, nil
}

// AddCustomPolicy adds a custom Rego policy
func (v *Validator) AddCustomPolicy(name, policy string) error {
    query, err := rego.New(
        rego.Query("data.kubernetes_executor.deny"),
        rego.Module(name, policy),
    ).PrepareForEval(context.Background())

    if err != nil {
        return fmt.Errorf("failed to compile policy %s: %w", name, err)
    }

    v.policies[name] = &query
    return nil
}
```

**File**: `pkg/kubernetesexecution/job/factory.go`
**BR Coverage**: BR-EXECUTOR-003

[~250 lines of implementation]
- Job creation with kubectl commands
- Per-action command argument mapping
- ServiceAccount binding
- Owner reference setting

**Expected Result**: Tests pass - basic policy validation and Job creation working

### DO-REFACTOR: Advanced Features (2h)
[~150 lines of refactored code]
- Policy caching for performance
- Batch policy evaluation
- Job template customization
- RBAC verification

**Validation**:
- [ ] Tests passing
- [ ] All 8 Rego policies enforced
- [ ] Jobs created with correct kubectl commands
- [ ] Policy evaluation < 50ms
```

**Total Day 2 Lines**: ~850 lines (currently ~90)

---

### Day 4: Per-Action RBAC & Job Watching (~850 lines)

**Location**: After current Day 4 outline, replace with full expansion

**Structure**:
```markdown
## 📅 Day 4: Per-Action RBAC and Job Status Watching (8h)

### ANALYSIS Phase (1h)
- Search existing RBAC patterns for Kubernetes Jobs
- Review Job status watching mechanisms
- Map BR-EXECUTOR-004 (Per-Action RBAC) requirements
- Identify dependencies (ServiceAccount per action, Job informers)

### PLAN Phase (1h)
- TDD Strategy: Unit tests for RBAC generation, Integration for Job watching
- Integration points: Kubernetes RBAC API, Job informers
- Success criteria: ServiceAccounts created per action, Job status synced

### DO-RED: RBAC Tests (2h)
**File**: `test/unit/kubernetesexecution/rbac_test.go`
**BR Coverage**: BR-EXECUTOR-004

[~350 lines of test code]
```go
var _ = Describe("BR-EXECUTOR-004: Per-Action RBAC", func() {
    var rbacManager *rbac.Manager

    BeforeEach(func() {
        rbacManager = rbac.NewManager(fakeClient, scheme)
    })

    DescribeTable("ServiceAccount and Role generation",
        func(action kubernetesexecutionv1alpha1.ActionType, expectedVerbs, expectedResources []string) {
            sa, role := rbacManager.GenerateRBAC(action, "kubernaut-executors")

            // Verify ServiceAccount
            Expect(sa.Name).To(HavePrefix(fmt.Sprintf("executor-%s", action)))
            Expect(sa.Namespace).To(Equal("kubernaut-executors"))

            // Verify Role permissions
            Expect(role.Rules).To(HaveLen(1))
            Expect(role.Rules[0].Verbs).To(ConsistOf(expectedVerbs))
            Expect(role.Rules[0].Resources).To(ConsistOf(expectedResources))
        },
        Entry("ScaleDeployment - minimal permissions",
            kubernetesexecutionv1alpha1.ActionTypeScaleDeployment,
            []string{"get", "patch"},
            []string{"deployments", "deployments/scale"}),

        Entry("DeletePod - delete permission only",
            kubernetesexecutionv1alpha1.ActionTypeDeletePod,
            []string{"delete"},
            []string{"pods"}),

        Entry("UpdateImage - patch permission only",
            kubernetesexecutionv1alpha1.ActionTypeUpdateImage,
            []string{"get", "patch"},
            []string{"deployments"}),

        Entry("CordonNode - node permissions",
            kubernetesexecutionv1alpha1.ActionTypeCordonNode,
            []string{"get", "patch"},
            []string{"nodes"}),

        // Additional 6+ entries for all actions
    )

    It("should use least-privilege principle for permissions", func() {
        // ScaleDeployment should NOT have delete permission
        _, role := rbacManager.GenerateRBAC(
            kubernetesexecutionv1alpha1.ActionTypeScaleDeployment,
            "kubernaut-executors",
        )

        Expect(role.Rules[0].Verbs).ToNot(ContainElement("delete"))
        Expect(role.Rules[0].Verbs).ToNot(ContainElement("*"))
    })
})

var _ = Describe("Job Status Watching", func() {
    var watcher *job.Watcher

    BeforeEach(func() {
        watcher = job.NewWatcher(fakeClient)
    })

    It("should update KubernetesExecution status when Job completes", func() {
        // [Test Job completion → CRD status update]
    })

    It("should handle Job failure and capture exit code", func() {
        // [Test Job failure → CRD status update with error details]
    })
})
```

### DO-GREEN: RBAC Manager & Job Watcher (3h)
[~350 lines of implementation]

### DO-REFACTOR: RBAC Caching (2h)
[~150 lines]

**Validation**:
- [ ] Per-action ServiceAccounts created
- [ ] Least-privilege RBAC enforced
- [ ] Job status synced to CRD
```

**Total Day 4 Lines**: ~850 lines (currently ~110)

---

### Day 7: Complete Integration (~800 lines)

**Location**: After current Day 7 outline, replace with full expansion

**Structure**:
```markdown
## 📅 Day 7: Controller Integration + Metrics (8h)

[Similar structure to Remediation Processor Day 7]

### Morning: Manager Setup (3h)
[~300 lines of complete main.go]

### Afternoon: Prometheus Metrics (2h)
[~250 lines of metrics]
- executor_job_duration_seconds
- executor_policy_violations_total
- executor_rbac_creation_errors_total
- executor_action_success_rate
- executor_active_jobs

### Evening: Health Checks (1h)
[~150 lines]

### EOD Documentation (2h)
[~100 lines summary, full doc in Phase 3]
```

**Total Day 7 Lines**: ~800 lines (currently ~95)

---

## Expansion Phase 2: Integration Test Suite (+650 lines)

### Integration Test 1: Deployment Scaling with RBAC (~220 lines)

**File**: `test/integration/kubernetesexecution/deployment_scaling_test.go`

**Structure**:
```go
var _ = Describe("Integration Test 1: Deployment Scaling with RBAC Validation", func() {
    It("should scale deployment using Job with per-action ServiceAccount", func() {
        By("Creating test Deployment")
        deployment := createTestDeployment("test-app", 3)
        err := crClient.Create(ctx, deployment)
        Expect(err).ToNot(HaveOccurred())

        By("Creating KubernetesExecution to scale deployment")
        executor := &kubernetesexecutionv1alpha1.KubernetesExecution{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "scale-test-app",
                Namespace: "kubernaut-executors",
            },
            Spec: kubernetesexecutionv1alpha1.KubernetesExecutionSpec{
                Action: kubernetesexecutionv1alpha1.ActionTypeScaleDeployment,
                Parameters: kubernetesexecutionv1alpha1.ActionParameters{
                    TargetResource: "deployment/test-app",
                    Namespace:      "default",
                    ScaleReplicas:  ptrInt32(5),
                },
            },
        }

        err = crClient.Create(ctx, executor)
        Expect(err).ToNot(HaveOccurred())

        By("Waiting for Job to be created")
        Eventually(func() int {
            jobList := &batchv1.JobList{}
            err := crClient.List(ctx, jobList, client.InNamespace("kubernaut-executors"))
            if err != nil {
                return 0
            }
            return len(jobList.Items)
        }, 10*time.Second, 1*time.Second).Should(BeNumerically(">=", 1))

        By("Verifying Job uses correct ServiceAccount")
        jobList := &batchv1.JobList{}
        err = crClient.List(ctx, jobList, client.InNamespace("kubernaut-executors"))
        Expect(err).ToNot(HaveOccurred())
        Expect(jobList.Items).To(HaveLen(1))

        job := jobList.Items[0]
        Expect(job.Spec.Template.Spec.ServiceAccountName).To(Equal("executor-scaledeployment"))

        By("Waiting for Job to complete successfully")
        Eventually(func() batchv1.JobConditionType {
            updatedJob := &batchv1.Job{}
            err := crClient.Get(ctx, types.NamespacedName{
                Name:      job.Name,
                Namespace: job.Namespace,
            }, updatedJob)
            if err != nil {
                return ""
            }
            if len(updatedJob.Status.Conditions) == 0 {
                return ""
            }
            return updatedJob.Status.Conditions[0].Type
        }, 30*time.Second, 2*time.Second).Should(Equal(batchv1.JobComplete))

        By("Verifying Deployment scaled to 5 replicas")
        Eventually(func() int32 {
            updatedDeployment := &appsv1.Deployment{}
            err := crClient.Get(ctx, types.NamespacedName{
                Name:      "test-app",
                Namespace: "default",
            }, updatedDeployment)
            if err != nil {
                return 0
            }
            return *updatedDeployment.Spec.Replicas
        }, 20*time.Second, 2*time.Second).Should(Equal(int32(5)))

        By("Verifying KubernetesExecution status updated")
        updatedExecutor := &kubernetesexecutionv1alpha1.KubernetesExecution{}
        err = crClient.Get(ctx, types.NamespacedName{
            Name:      "scale-test-app",
            Namespace: "kubernaut-executors",
        }, updatedExecutor)
        Expect(err).ToNot(HaveOccurred())
        Expect(updatedExecutor.Status.Phase).To(Equal(kubernetesexecutionv1alpha1.ExecutionPhaseSucceeded))

        GinkgoWriter.Printf("✅ Deployment scaling validated: 3 → 5 replicas\n")
    })
})
```

---

### Integration Test 2: Pod Restart with Rego Policy (~220 lines)

**File**: `test/integration/kubernetesexecution/pod_restart_policy_test.go`

**Structure**: Test policy enforcement blocking unsafe pod deletion

---

### Integration Test 3: Job Completion Tracking (~210 lines)

**File**: `test/integration/kubernetesexecution/job_completion_test.go`

**Structure**: Test Job status → CRD status sync

**Total Integration Tests**: ~650 lines (currently 0)

---

## Expansion Phase 3: EOD Documentation (+800 lines)

### EOD 1: Day 4 Midpoint (~400 lines)

**File**: `phase0/02-day4-midpoint.md`

**Structure**: [Similar to other services, covering Days 1-4]

---

### EOD 2: Day 7 Complete (~400 lines)

**File**: `phase0/03-day7-complete.md`

**Structure**: [Comprehensive summary of Days 1-7]

---

## Expansion Phase 4: Error Handling Philosophy (+300 lines)

**File**: `design/ERROR_HANDLING_PHILOSOPHY.md`

**Structure**:
```markdown
# Error Handling Philosophy - Kubernetes Executor

## Executive Summary
Kubernetes-specific error handling for:
- BR-EXECUTOR-005: Job failure handling
- BR-EXECUTOR-008: Policy violation handling
- BR-EXECUTOR-010: RBAC error handling

## Error Classification Taxonomy

### 1. Transient Errors (RETRY)
| Error Type | Example | Retry | Max Attempts |
|-----------|---------|-------|--------------|
| **API Server Timeout** | kubectl timeout | ✅ Yes | 3 |
| **Job Pod Pending** | Image pull delay | ✅ Yes | 5 |

### 2. Permanent Errors (FAIL IMMEDIATELY)
| Error Type | Example | Retry | Action |
|-----------|---------|-------|--------|
| **Policy Violation** | Unsafe action blocked | ❌ No | Log violation, alert |
| **RBAC Denied** | Insufficient permissions | ❌ No | Fix RBAC, manual approval |
| **Invalid Action Parameters** | Malformed kubectl command | ❌ No | Validation error |

### 3. Job-Specific Errors
| Error Type | Detection | Action |
|-----------|-----------|--------|
| **Job Failed (Exit Code != 0)** | Job status | Capture logs, mark CRD failed |
| **Job Timeout** | ActiveDeadlineSeconds | Kill Job, mark timeout |
| **Job Backoff Limit** | Backoff exceeded | Mark permanent failure |

## Job Failure Decision Matrix
[When to retry Job vs mark CRD failed]

## RBAC Error Recovery
[How to handle missing permissions]

## Policy Violation Response
[Manual approval workflow]

## Operational Guidelines
[Monitoring, alerts, testing]
```

---

## Expansion Phase 5: Enhanced BR Coverage Matrix (+200 lines)

**Enhancement to existing**: `testing/BR_COVERAGE_MATRIX.md`

**Additions**: Same structure as other services
- Testing Infrastructure (Envtest for CRD, Kind for Job execution)
- Edge Case Coverage (15 scenarios including policy edge cases)
- Test Implementation Guidance with policy testing examples
- Defense-in-Depth coverage update

---

## Summary: Total Additions by Phase

| Phase | Component | Lines | Effort |
|-------|-----------|-------|--------|
| **Phase 1** | Day 2 APDC Expansion | 850 | 3h |
| **Phase 1** | Day 4 APDC Expansion | 850 | 3h |
| **Phase 1** | Day 7 APDC Expansion | 800 | 3h |
| **Phase 2** | Integration Test 1 | 220 | 1.5h |
| **Phase 2** | Integration Test 2 | 220 | 1.5h |
| **Phase 2** | Integration Test 3 | 210 | 1.5h |
| **Phase 3** | EOD: Day 4 Midpoint | 400 | 1h |
| **Phase 3** | EOD: Day 7 Complete | 400 | 1h |
| **Phase 4** | Error Handling Philosophy | 300 | 1h |
| **Phase 5** | BR Coverage Matrix Enhancement | 200 | 0.5h |
| **Total** | **All Additions** | **4,450** | **17h** |

**Note**: 600 lines already complete (Rego Policy Integration)!

**Confidence Increase**: 70% → 95%

---

## Approval Required

**This plan shows WHAT will be added but does NOT implement it yet.**

Please review and approve before implementation begins.

