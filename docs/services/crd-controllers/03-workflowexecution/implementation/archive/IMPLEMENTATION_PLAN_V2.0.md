# WorkflowExecution Controller - Implementation Plan v2.0

**Version**: 2.0 - ENGINE DELEGATION ARCHITECTURE
**Date**: 2025-11-28
**Timeline**: 12 days (96 hours)
**Status**: ✅ **Ready for Implementation** (98% Confidence)
**Architecture**: [ADR-044: Workflow Execution Engine Delegation](../../../../architecture/decisions/ADR-044-workflow-execution-engine-delegation.md)

---

## ⚠️ **Breaking Change from v1.x**

**This plan supersedes v1.3** due to architectural simplification:

| Aspect | v1.3 (Old) | v2.0 (New) |
|--------|------------|------------|
| **Architecture** | Complex orchestration | Engine delegation |
| **Timeline** | 30-33 days | **12 days** |
| **Step handling** | Controller logic | Tekton handles |
| **Rollback** | Controller logic | Not our concern |
| **BRs** | 38 BRs | **~12 BRs** |

**Decision**: [ADR-044](../../../../architecture/decisions/ADR-044-workflow-execution-engine-delegation.md)

---

## 🎯 **Service Overview**

**Purpose**: Create Tekton PipelineRuns from OCI workflow bundles and monitor execution status.

**Core Responsibilities**:
1. **CRD Reconciliation** - Watch and reconcile WorkflowExecution CRDs
2. **PipelineRun Creation** - Create Tekton PipelineRun from OCI bundle
3. **Status Monitoring** - Watch PipelineRun status and update CRD
4. **Audit Trail** - Record execution outcomes for compliance

**What We DON'T Do**:
- ❌ Step orchestration (Tekton handles)
- ❌ Dependency resolution (Tekton handles)
- ❌ Rollback (Tekton `finally` tasks or N/A)
- ❌ Workflow transformation (direct OCI bundle usage)
- ❌ Per-step status tracking (overall status only)

---

## 📋 **Business Requirements**

### Core BRs (Simplified)

| BR ID | Description | Priority |
|-------|-------------|----------|
| **BR-WE-001** | Create Tekton PipelineRun from WorkflowExecution CRD | P0 |
| **BR-WE-002** | Pass parameters to PipelineRun from spec | P0 |
| **BR-WE-003** | Monitor PipelineRun status (Running/Completed/Failed) | P0 |
| **BR-WE-004** | Update WorkflowExecution status based on PipelineRun | P0 |
| **BR-WE-005** | Set owner reference for cascade deletion | P0 |
| **BR-WE-006** | Support configurable ServiceAccount for PipelineRun | P1 |
| **BR-WE-007** | Emit Prometheus metrics for execution outcomes | P1 |
| **BR-WE-008** | Write audit event on completion (ADR-034) | P0 |
| **BR-WE-009** | Resource locking - prevent parallel execution | P0 |
| **BR-WE-010** | Cooldown - prevent redundant sequential execution | P0 |
| **BR-WE-011** | Target resource identification and validation | P0 |

**Total**: 12 BRs (vs 38 in v1.3)

---

## 🏗️ **CRD Schema**

```go
// api/workflowexecution/v1alpha1/types.go
package v1alpha1

import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Outcome",type=string,JSONPath=`.status.outcome`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type WorkflowExecution struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   WorkflowExecutionSpec   `json:"spec,omitempty"`
    Status WorkflowExecutionStatus `json:"status,omitempty"`
}

// WorkflowExecutionSpec defines the desired state
type WorkflowExecutionSpec struct {
    // RemediationRequestRef - parent reference for audit trail
    // +kubebuilder:validation:Required
    RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

    // WorkflowRef - catalog-resolved workflow reference
    // +kubebuilder:validation:Required
    WorkflowRef WorkflowRef `json:"workflowRef"`

    // Parameters - from LLM selection (UPPER_SNAKE_CASE per DD-WORKFLOW-003)
    Parameters map[string]string `json:"parameters,omitempty"`

    // Confidence - from AIAnalysis (for audit)
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=1
    Confidence float64 `json:"confidence,omitempty"`

    // Rationale - from AIAnalysis (for audit)
    Rationale string `json:"rationale,omitempty"`

    // ExecutionConfig - minimal execution settings
    ExecutionConfig ExecutionConfig `json:"executionConfig,omitempty"`
}

// WorkflowRef - reference to OCI bundle
type WorkflowRef struct {
    // WorkflowID - catalog lookup key
    // +kubebuilder:validation:Required
    WorkflowID string `json:"workflowId"`

    // Version - workflow version
    // +kubebuilder:validation:Required
    Version string `json:"version"`

    // ContainerImage - OCI bundle URL (e.g., quay.io/kubernaut/workflow:v1.0.0)
    // +kubebuilder:validation:Required
    ContainerImage string `json:"containerImage"`

    // ContainerDigest - for audit trail and reproducibility
    ContainerDigest string `json:"containerDigest,omitempty"`
}

// ExecutionConfig - minimal config (engine handles the rest)
type ExecutionConfig struct {
    // ServiceAccountName for PipelineRun (default: workflow-execution-sa)
    ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// WorkflowExecutionStatus defines the observed state
type WorkflowExecutionStatus struct {
    // Phase: Pending → Running → Completed/Failed
    // +kubebuilder:validation:Enum=Pending;Running;Completed;Failed
    Phase string `json:"phase,omitempty"`

    // PipelineRunRef - reference to created Tekton PipelineRun
    PipelineRunRef *corev1.ObjectReference `json:"pipelineRunRef,omitempty"`

    // StartTime - when PipelineRun was created
    StartTime *metav1.Time `json:"startTime,omitempty"`

    // CompletionTime - when PipelineRun finished
    CompletionTime *metav1.Time `json:"completionTime,omitempty"`

    // Outcome: Success, Failed
    Outcome string `json:"outcome,omitempty"`

    // Message - human-readable status message
    Message string `json:"message,omitempty"`

    // Conditions - standard K8s conditions
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

---

## 📅 **Day-by-Day Implementation**

### **Day 1-2: Project Setup & CRD**

**Goals**:
- Scaffold controller with kubebuilder
- Define CRD types
- Basic reconciler shell

**Deliverables**:
```bash
# Day 1
kubebuilder init --domain kubernaut.ai --repo github.com/jordigilh/kubernaut
kubebuilder create api --group workflowexecution --version v1alpha1 --kind WorkflowExecution

# Day 2
make manifests
make generate
```

**Files Created**:
- `api/workflowexecution/v1alpha1/types.go`
- `api/workflowexecution/v1alpha1/zz_generated.deepcopy.go`
- `config/crd/bases/workflowexecution.kubernaut.ai_workflowexecutions.yaml`
- `internal/controller/workflowexecution/workflowexecution_controller.go`

---

### **Day 3-4: PipelineRun Creation**

**Goals**:
- Implement `handlePending` to create Tekton PipelineRun
- Convert parameters to Tekton format
- Set owner reference

**Implementation**:
```go
// internal/controller/workflowexecution/reconciler.go
package workflowexecution

import (
    "context"
    "fmt"

    tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
    workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/types"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
    DefaultServiceAccount = "workflow-execution-sa"
    PipelineName          = "workflow" // Name in OCI bundle
)

type WorkflowExecutionReconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

func (r *WorkflowExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    var wfe workflowexecutionv1alpha1.WorkflowExecution
    if err := r.Get(ctx, req.NamespacedName, &wfe); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Skip if terminal
    if wfe.Status.Phase == "Completed" || wfe.Status.Phase == "Failed" {
        return ctrl.Result{}, nil
    }

    switch wfe.Status.Phase {
    case "", "Pending":
        return r.handlePending(ctx, &wfe)
    case "Running":
        return r.handleRunning(ctx, &wfe)
    default:
        log.Info("Unknown phase", "phase", wfe.Status.Phase)
        return ctrl.Result{}, nil
    }
}

func (r *WorkflowExecutionReconciler) handlePending(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Validate spec
    if err := r.validateSpec(wfe); err != nil {
        return r.setFailed(ctx, wfe, "ValidationFailed", err.Error())
    }

    // Build PipelineRun
    pipelineRun := r.buildPipelineRun(wfe)

    // Create PipelineRun
    if err := r.Create(ctx, pipelineRun); err != nil {
        log.Error(err, "Failed to create PipelineRun")
        return ctrl.Result{}, err
    }

    log.Info("Created PipelineRun", "name", pipelineRun.Name)

    // Update status
    now := metav1.Now()
    wfe.Status.Phase = "Running"
    wfe.Status.PipelineRunRef = &corev1.ObjectReference{
        APIVersion: tektonv1.SchemeGroupVersion.String(),
        Kind:       "PipelineRun",
        Name:       pipelineRun.Name,
        Namespace:  pipelineRun.Namespace,
    }
    wfe.Status.StartTime = &now

    if err := r.Status().Update(ctx, wfe); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *WorkflowExecutionReconciler) buildPipelineRun(wfe *workflowexecutionv1alpha1.WorkflowExecution) *tektonv1.PipelineRun {
    serviceAccount := wfe.Spec.ExecutionConfig.ServiceAccountName
    if serviceAccount == "" {
        serviceAccount = DefaultServiceAccount
    }

    return &tektonv1.PipelineRun{
        ObjectMeta: metav1.ObjectMeta{
            GenerateName: fmt.Sprintf("%s-", wfe.Name),
            Namespace:    wfe.Namespace,
            Labels: map[string]string{
                "kubernaut.ai/workflow-execution": wfe.Name,
                "kubernaut.ai/workflow-id":        wfe.Spec.WorkflowRef.WorkflowID,
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(wfe, workflowexecutionv1alpha1.GroupVersion.WithKind("WorkflowExecution")),
            },
        },
        Spec: tektonv1.PipelineRunSpec{
            PipelineRef: &tektonv1.PipelineRef{
                ResolverRef: tektonv1.ResolverRef{
                    Resolver: "bundles",
                    Params: []tektonv1.Param{
                        {
                            Name:  "bundle",
                            Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: wfe.Spec.WorkflowRef.ContainerImage},
                        },
                        {
                            Name:  "name",
                            Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: PipelineName},
                        },
                    },
                },
            },
            Params:             r.convertParameters(wfe.Spec.Parameters),
            ServiceAccountName: serviceAccount,
        },
    }
}

func (r *WorkflowExecutionReconciler) convertParameters(params map[string]string) []tektonv1.Param {
    result := make([]tektonv1.Param, 0, len(params))
    for name, value := range params {
        result = append(result, tektonv1.Param{
            Name:  name,
            Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: value},
        })
    }
    return result
}

func (r *WorkflowExecutionReconciler) validateSpec(wfe *workflowexecutionv1alpha1.WorkflowExecution) error {
    if wfe.Spec.WorkflowRef.ContainerImage == "" {
        return fmt.Errorf("workflowRef.containerImage is required")
    }
    if wfe.Spec.WorkflowRef.WorkflowID == "" {
        return fmt.Errorf("workflowRef.workflowId is required")
    }
    return nil
}
```

---

### **Day 5-6: Status Monitoring**

**Goals**:
- Implement `handleRunning` to watch PipelineRun status
- Update WorkflowExecution status on completion
- Handle Success/Failed outcomes

**Implementation**:
```go
// internal/controller/workflowexecution/reconciler.go (continued)

import (
    "knative.dev/pkg/apis"
)

func (r *WorkflowExecutionReconciler) handleRunning(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    if wfe.Status.PipelineRunRef == nil {
        return r.setFailed(ctx, wfe, "MissingPipelineRun", "PipelineRunRef is nil in Running phase")
    }

    // Get PipelineRun
    var pipelineRun tektonv1.PipelineRun
    if err := r.Get(ctx, types.NamespacedName{
        Name:      wfe.Status.PipelineRunRef.Name,
        Namespace: wfe.Status.PipelineRunRef.Namespace,
    }, &pipelineRun); err != nil {
        if client.IgnoreNotFound(err) == nil {
            // PipelineRun deleted - mark as failed
            return r.setFailed(ctx, wfe, "PipelineRunDeleted", "PipelineRun was deleted")
        }
        return ctrl.Result{}, err
    }

    // Check if done
    if !pipelineRun.IsDone() {
        log.V(1).Info("PipelineRun still running", "name", pipelineRun.Name)
        return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
    }

    // PipelineRun completed - update status
    now := metav1.Now()
    wfe.Status.CompletionTime = &now

    condition := pipelineRun.Status.GetCondition(apis.ConditionSucceeded)
    if condition == nil {
        return r.setFailed(ctx, wfe, "UnknownStatus", "PipelineRun has no Succeeded condition")
    }

    if condition.IsTrue() {
        wfe.Status.Phase = "Completed"
        wfe.Status.Outcome = "Success"
        wfe.Status.Message = "Workflow executed successfully"
        log.Info("Workflow completed successfully", "name", wfe.Name)
    } else {
        wfe.Status.Phase = "Failed"
        wfe.Status.Outcome = "Failed"
        wfe.Status.Message = condition.Message
        log.Info("Workflow failed", "name", wfe.Name, "message", condition.Message)
    }

    // Update condition
    wfe.Status.Conditions = []metav1.Condition{
        {
            Type:               "Ready",
            Status:             metav1.ConditionTrue,
            Reason:             wfe.Status.Outcome,
            Message:            wfe.Status.Message,
            LastTransitionTime: now,
        },
    }

    if err := r.Status().Update(ctx, wfe); err != nil {
        return ctrl.Result{}, err
    }

    // Write audit event (BR-WE-008)
    r.writeAuditEvent(ctx, wfe)

    return ctrl.Result{}, nil
}

func (r *WorkflowExecutionReconciler) setFailed(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, reason, message string) (ctrl.Result, error) {
    now := metav1.Now()
    wfe.Status.Phase = "Failed"
    wfe.Status.Outcome = "Failed"
    wfe.Status.Message = message
    wfe.Status.CompletionTime = &now
    wfe.Status.Conditions = []metav1.Condition{
        {
            Type:               "Ready",
            Status:             metav1.ConditionFalse,
            Reason:             reason,
            Message:            message,
            LastTransitionTime: now,
        },
    }

    if err := r.Status().Update(ctx, wfe); err != nil {
        return ctrl.Result{}, err
    }

    r.writeAuditEvent(ctx, wfe)
    return ctrl.Result{}, nil
}

func (r *WorkflowExecutionReconciler) writeAuditEvent(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) {
    // Fire-and-forget audit write (per ADR-038)
    go func() {
        // Implementation per ADR-034 audit table
    }()
}
```

---

### **Day 7-8: Unit + Integration Tests**

**Goals**:
- Unit tests with fake client
- Integration tests with EnvTest + Tekton CRDs

**Unit Test Example**:
```go
// test/unit/workflowexecution/reconciler_test.go
package workflowexecution_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
    workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("WorkflowExecution Controller", func() {
    var (
        fakeClient client.Client
        reconciler *WorkflowExecutionReconciler
    )

    BeforeEach(func() {
        scheme := runtime.NewScheme()
        _ = workflowexecutionv1alpha1.AddToScheme(scheme)
        _ = tektonv1.AddToScheme(scheme)

        fakeClient = fake.NewClientBuilder().
            WithScheme(scheme).
            Build()

        reconciler = &WorkflowExecutionReconciler{
            Client: fakeClient,
            Scheme: scheme,
        }
    })

    Describe("handlePending", func() {
        It("should create PipelineRun from OCI bundle", func() {
            // Given
            wfe := &workflowexecutionv1alpha1.WorkflowExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-wfe",
                    Namespace: "default",
                },
                Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
                    WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
                        WorkflowID:     "oomkill-increase-memory",
                        Version:        "1.0.0",
                        ContainerImage: "quay.io/kubernaut/oomkill:v1.0.0",
                    },
                    Parameters: map[string]string{
                        "NAMESPACE":       "production",
                        "DEPLOYMENT_NAME": "my-app",
                    },
                },
            }
            Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

            // When
            result, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: types.NamespacedName{Name: "test-wfe", Namespace: "default"},
            })

            // Then
            Expect(err).ToNot(HaveOccurred())
            Expect(result.RequeueAfter).To(Equal(10 * time.Second))

            // Verify PipelineRun created
            var pipelineRunList tektonv1.PipelineRunList
            Expect(fakeClient.List(ctx, &pipelineRunList)).To(Succeed())
            Expect(pipelineRunList.Items).To(HaveLen(1))

            pr := pipelineRunList.Items[0]
            Expect(pr.Spec.PipelineRef.ResolverRef.Resolver).To(Equal("bundles"))

            // Verify bundle parameter
            bundleParam := findParam(pr.Spec.PipelineRef.ResolverRef.Params, "bundle")
            Expect(bundleParam.Value.StringVal).To(Equal("quay.io/kubernaut/oomkill:v1.0.0"))

            // Verify workflow parameters passed
            nsParam := findParam(pr.Spec.Params, "NAMESPACE")
            Expect(nsParam.Value.StringVal).To(Equal("production"))
        })

        It("should fail if containerImage is empty", func() {
            // Given
            wfe := &workflowexecutionv1alpha1.WorkflowExecution{
                ObjectMeta: metav1.ObjectMeta{Name: "test-wfe", Namespace: "default"},
                Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
                    WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
                        WorkflowID: "test",
                        // ContainerImage missing
                    },
                },
            }
            Expect(fakeClient.Create(ctx, wfe)).To(Succeed())

            // When
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: types.NamespacedName{Name: "test-wfe", Namespace: "default"},
            })

            // Then
            Expect(err).ToNot(HaveOccurred()) // Error handled gracefully

            var updated workflowexecutionv1alpha1.WorkflowExecution
            Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-wfe", Namespace: "default"}, &updated)).To(Succeed())
            Expect(updated.Status.Phase).To(Equal("Failed"))
            Expect(updated.Status.Message).To(ContainSubstring("containerImage is required"))
        })
    })

    Describe("handleRunning", func() {
        It("should mark Completed when PipelineRun succeeds", func() {
            // Given: WFE in Running phase with successful PipelineRun
            // ...
        })

        It("should mark Failed when PipelineRun fails", func() {
            // Given: WFE in Running phase with failed PipelineRun
            // ...
        })
    })
})
```

---

### **Day 9-10: E2E Tests**

**Goals**:
- E2E tests with Kind + Tekton
- Real PipelineRun execution

**E2E Test Setup**:
```yaml
# test/e2e/workflowexecution/kind-config.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    extraPortMappings:
      - containerPort: 30090
        hostPort: 28090  # E2E metrics (per DD-TEST-001)
```

**E2E Test**:
```go
// test/e2e/workflowexecution/workflow_execution_e2e_test.go
var _ = Describe("WorkflowExecution E2E", func() {
    It("should execute workflow from OCI bundle successfully", func() {
        // Given: Test workflow OCI bundle exists
        wfe := &workflowexecutionv1alpha1.WorkflowExecution{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "e2e-test-wfe",
                Namespace: "kubernaut-system",
            },
            Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
                WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
                    WorkflowID:     "test-echo",
                    Version:        "1.0.0",
                    ContainerImage: "ttl.sh/kubernaut-test-workflow:1h",
                },
                Parameters: map[string]string{
                    "MESSAGE": "Hello from E2E test",
                },
            },
        }

        // When
        Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

        // Then: Wait for completion
        Eventually(func() string {
            var updated workflowexecutionv1alpha1.WorkflowExecution
            k8sClient.Get(ctx, types.NamespacedName{Name: "e2e-test-wfe", Namespace: "kubernaut-system"}, &updated)
            return updated.Status.Phase
        }, 2*time.Minute, 5*time.Second).Should(Equal("Completed"))

        // Verify outcome
        var final workflowexecutionv1alpha1.WorkflowExecution
        Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "e2e-test-wfe", Namespace: "kubernaut-system"}, &final)).To(Succeed())
        Expect(final.Status.Outcome).To(Equal("Success"))
        Expect(final.Status.PipelineRunRef).ToNot(BeNil())
    })
})
```

---

### **Day 11-12: Polish & Documentation**

**Goals**:
- Prometheus metrics
- Structured logging
- Documentation updates

**Metrics**:
```go
// internal/controller/workflowexecution/metrics.go
package workflowexecution

import (
    "github.com/prometheus/client_golang/prometheus"
    "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
    workflowExecutionTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubernaut_workflow_execution_total",
            Help: "Total workflow executions by outcome",
        },
        []string{"workflow_id", "outcome"},
    )

    workflowExecutionDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "kubernaut_workflow_execution_duration_seconds",
            Help:    "Workflow execution duration",
            Buckets: []float64{30, 60, 120, 300, 600, 1800},
        },
        []string{"workflow_id"},
    )

    pipelineRunCreationErrors = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "kubernaut_pipelinerun_creation_errors_total",
            Help: "Total PipelineRun creation errors",
        },
    )
)

func init() {
    metrics.Registry.MustRegister(
        workflowExecutionTotal,
        workflowExecutionDuration,
        pipelineRunCreationErrors,
    )
}
```

---

## 📊 **Test Coverage Targets**

| Tier | Tests | Target |
|------|-------|--------|
| **Unit** | 15-20 | 70%+ coverage |
| **Integration** | 8-10 | EnvTest + Tekton CRDs |
| **E2E** | 3-5 | Kind + real Tekton |

---

## 🔗 **Related Documents**

| Document | Relationship |
|----------|--------------|
| **ADR-044** | Engine delegation architecture (this plan's foundation) |
| **DD-CONTRACT-001** | WorkflowRef schema, parameters format |
| **ADR-043** | Workflow schema in OCI bundle |
| **DD-TIMEOUT-001** | Global timeout (handled by RO) |
| **DD-TEST-001** | Port allocation (E2E: 28090) |
| **ADR-034** | Audit event format |

---

## ✅ **Definition of Done**

- [ ] CRD deployed and validated
- [ ] Controller creates PipelineRun from OCI bundle
- [ ] Status correctly reflects PipelineRun outcome
- [ ] Unit tests passing (70%+ coverage)
- [ ] Integration tests passing (EnvTest)
- [ ] E2E tests passing (Kind + Tekton)
- [ ] Metrics exposed on :9090
- [ ] Audit events written (ADR-034)
- [ ] Documentation updated

---

## 📝 **Changelog**

| Version | Date | Changes |
|---------|------|---------|
| 2.0 | 2025-11-28 | Complete rewrite: Engine delegation architecture (ADR-044), 12-day timeline, simplified CRD |

---

**Supersedes**: Implementation Plan v1.3 (complex orchestration)
