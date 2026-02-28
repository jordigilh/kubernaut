## Finalizer Implementation

> **DEPRECATED**: KubernetesExecution CRD and KubernetesExecutor service were eliminated by ADR-025 and replaced by Tekton TaskRun via WorkflowExecution. This documentation is retained for historical reference only. API types and CRD manifests have been removed from the codebase.

### Finalizer Name

Following Kubernetes finalizer naming convention:

```go
const kubernetesExecutionFinalizer = "kubernetesexecution.kubernaut.io/kubernetesexecution-cleanup"
```

**Naming Pattern**: `{domain}.kubernaut.io/{resource}-cleanup`

**Why This Pattern**:
- **Domain-Scoped**: `kubernetesexecution.kubernaut.io` prevents conflicts with other services
- **Resource-Specific**: `kubernetesexecution-cleanup` clearly indicates what's being cleaned up
- **Kubernetes Convention**: Follows standard finalizer naming (domain/action format)

---

### Complete Reconciliation Loop with Finalizer

```go
package controller

import (
    "context"
    "fmt"

    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetes/execution/v1"

    "github.com/go-logr/logr"
    batchv1 "k8s.io/api/batch/v1"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const kubernetesExecutionFinalizer = "kubernetesexecution.kubernaut.io/kubernetesexecution-cleanup"

type KubernetesExecutionReconciler struct {
    client.Client
    Scheme            *runtime.Scheme
    Log               logr.Logger
    Recorder          record.EventRecorder
    ActionHandlers    map[string]ActionHandler
    PolicyEvaluator   PolicyEvaluator
    StorageClient     StorageClient
}

func (r *KubernetesExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var ke kubernetesexecutionv1.KubernetesExecution
    if err := r.Get(ctx, req.NamespacedName, &ke); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // ========================================
    // DELETION HANDLING WITH FINALIZER
    // ========================================
    if !ke.ObjectMeta.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(&ke, kubernetesExecutionFinalizer) {
            // Perform cleanup before deletion
            if err := r.cleanupKubernetesExecution(ctx, &ke); err != nil {
                r.Log.Error(err, "Failed to cleanup KubernetesExecution resources",
                    "name", ke.Name,
                    "namespace", ke.Namespace,
                )
                return ctrl.Result{}, err
            }

            // Remove finalizer to allow deletion
            controllerutil.RemoveFinalizer(&ke, kubernetesExecutionFinalizer)
            if err := r.Update(ctx, &ke); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // ========================================
    // ADD FINALIZER IF NOT PRESENT
    // ========================================
    if !controllerutil.ContainsFinalizer(&ke, kubernetesExecutionFinalizer) {
        controllerutil.AddFinalizer(&ke, kubernetesExecutionFinalizer)
        if err := r.Update(ctx, &ke); err != nil {
            return ctrl.Result{}, err
        }
    }

    // ========================================
    // NORMAL RECONCILIATION LOGIC
    // ========================================

    // Skip if already completed or failed
    if ke.Status.Phase == "completed" || ke.Status.Phase == "failed" {
        return ctrl.Result{}, nil
    }

    // Execute phases (validating, creating_job, executing, validating_results, rollback_prepared, completed)...
    // (existing reconciliation logic from previous section)

    return ctrl.Result{}, nil
}
```

---

### Cleanup Logic

**What Gets Cleaned Up** (Leaf Controller Pattern):

```go
package controller

import (
    "context"
    "fmt"
    "time"

    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetes/execution/v1"

    batchv1 "k8s.io/api/batch/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *KubernetesExecutionReconciler) cleanupKubernetesExecution(
    ctx context.Context,
    ke *kubernetesexecutionv1.KubernetesExecution,
) error {
    r.Log.Info("Cleaning up KubernetesExecution resources",
        "name", ke.Name,
        "namespace", ke.Namespace,
        "phase", ke.Status.Phase,
    )

    // 1. Delete Kubernetes Job if still running (best-effort)
    if ke.Status.JobName != "" {
        job := &batchv1.Job{}
        jobKey := client.ObjectKey{
            Name:      ke.Status.JobName,
            Namespace: "kubernaut-executor",
        }

        if err := r.Get(ctx, jobKey, job); err == nil {
            // Job still exists, delete it
            if err := r.Delete(ctx, job); err != nil {
                r.Log.Error(err, "Failed to delete Kubernetes Job", "jobName", ke.Status.JobName)
                // Don't block cleanup on job deletion failure
            } else {
                r.Log.Info("Deleted Kubernetes Job", "jobName", ke.Status.JobName)
            }
        }
    }

    // 2. Record final audit to database
    if err := r.recordFinalAudit(ctx, ke); err != nil {
        r.Log.Error(err, "Failed to record final audit", "name", ke.Name)
        // Don't block deletion on audit failure
        // Audit is best-effort during cleanup
    }

    // 3. Emit deletion event
    r.Recorder.Event(ke, "Normal", "KubernetesExecutionDeleted",
        fmt.Sprintf("KubernetesExecution cleanup completed (phase: %s, action: %s)",
            ke.Status.Phase, ke.Spec.Action))

    r.Log.Info("KubernetesExecution cleanup completed successfully",
        "name", ke.Name,
        "namespace", ke.Namespace,
    )

    return nil
}

func (r *KubernetesExecutionReconciler) recordFinalAudit(
    ctx context.Context,
    ke *kubernetesexecutionv1.KubernetesExecution,
) error {
    auditRecord := &AuditRecord{
        AlertFingerprint: ke.Spec.SignalContext.Fingerprint,
        ServiceType:      "KubernetesExecution",
        CRDName:          ke.Name,
        Namespace:        ke.Namespace,
        Phase:            ke.Status.Phase,
        CreatedAt:        ke.CreationTimestamp.Time,
        DeletedAt:        ke.DeletionTimestamp.Time,
        Action:           ke.Spec.Action,
        JobName:          ke.Status.JobName,
        ExecutionStatus:  ke.Status.ExecutionResult.Status,
        RollbackPrepared: ke.Status.RollbackInfo != nil,
    }

    return r.StorageClient.RecordAudit(ctx, auditRecord)
}
```

**Cleanup Philosophy for KubernetesExecution** (Leaf Controller):
- ✅ **Delete Kubernetes Job**: Best-effort cleanup of running Job (prevent resource leaks)
- ✅ **Record final audit**: Capture execution results (best-effort)
- ✅ **Emit deletion event**: Operational visibility
- ❌ **No external cleanup needed**: KubernetesExecution is a leaf CRD (owns nothing except Jobs)
- ❌ **No child CRD cleanup**: KubernetesExecution doesn't create child CRDs
- ✅ **Non-blocking**: Job deletion and audit failures don't block deletion (best-effort)

**Note**: Kubernetes Jobs have `ownerReferences` set to KubernetesExecution, so they'll be cascade-deleted automatically. Explicit deletion in finalizer is best-effort cleanup for running Jobs.

---

### Finalizer Testing

**Unit Test Pattern**:

```go
package controller_test

import (
    "context"
    "fmt"

    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetes/execution/v1"
    "github.com/jordigilh/kubernaut/pkg/kubernetesexecution/controller"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    batchv1 "k8s.io/api/batch/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/kubernetes/scheme"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("KubernetesExecution Finalizer", func() {
    var (
        ctx       context.Context
        k8sClient client.Client
        reconciler *controller.KubernetesExecutionReconciler
    )

    BeforeEach(func() {
        ctx = context.Background()
        k8sClient = fake.NewClientBuilder().
            WithScheme(scheme.Scheme).
            Build()

        reconciler = &controller.KubernetesExecutionReconciler{
            Client:        k8sClient,
            StorageClient: &mockStorageClient{},
        }
    })

    Context("when KubernetesExecution is created", func() {
        It("should add finalizer on first reconcile", func() {
            ke := &kubernetesexecutionv1.KubernetesExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-execution",
                    Namespace: "default",
                },
                Spec: kubernetesexecutionv1.KubernetesExecutionSpec{
                    Action: "scale_deployment",
                    Parameters: map[string]string{
                        "deployment": "webapp",
                        "replicas":   "5",
                    },
                },
            }
            Expect(k8sClient.Create(ctx, ke)).To(Succeed())

            // First reconcile should add finalizer
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(ke),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer added
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ke), ke)).To(Succeed())
            Expect(controllerutil.ContainsFinalizer(ke, kubernetesExecutionFinalizer)).To(BeTrue())
        })
    })

    Context("when KubernetesExecution is deleted", func() {
        It("should execute cleanup and remove finalizer", func() {
            ke := &kubernetesexecutionv1.KubernetesExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-execution",
                    Namespace:  "default",
                    Finalizers: []string{kubernetesExecutionFinalizer},
                },
                Status: kubernetesexecutionv1.KubernetesExecutionStatus{
                    Phase:   "completed",
                    JobName: "test-job-123",
                },
            }
            Expect(k8sClient.Create(ctx, ke)).To(Succeed())

            // Delete KubernetesExecution
            Expect(k8sClient.Delete(ctx, ke)).To(Succeed())

            // Reconcile should execute cleanup
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(ke),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed (CRD will be deleted by Kubernetes)
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(ke), ke)
            Expect(err).To(HaveOccurred())
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })

        It("should delete running Kubernetes Job during cleanup", func() {
            // Create a Job that KubernetesExecution owns
            job := &batchv1.Job{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-job-123",
                    Namespace: "kubernaut-executor",
                },
            }
            Expect(k8sClient.Create(ctx, job)).To(Succeed())

            ke := &kubernetesexecutionv1.KubernetesExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-execution",
                    Namespace:  "default",
                    Finalizers: []string{kubernetesExecutionFinalizer},
                },
                Status: kubernetesexecutionv1.KubernetesExecutionStatus{
                    Phase:   "executing",
                    JobName: "test-job-123",
                },
            }
            Expect(k8sClient.Create(ctx, ke)).To(Succeed())
            Expect(k8sClient.Delete(ctx, ke)).To(Succeed())

            // Cleanup should delete Job
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(ke),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify Job deleted
            err = k8sClient.Get(ctx, client.ObjectKey{
                Name:      "test-job-123",
                Namespace: "kubernaut-executor",
            }, job)
            Expect(err).To(HaveOccurred())
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })

        It("should not block deletion if job deletion fails", func() {
            ke := &kubernetesexecutionv1.KubernetesExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-execution",
                    Namespace:  "default",
                    Finalizers: []string{kubernetesExecutionFinalizer},
                },
                Status: kubernetesexecutionv1.KubernetesExecutionStatus{
                    Phase:   "executing",
                    JobName: "nonexistent-job",
                },
            }
            Expect(k8sClient.Create(ctx, ke)).To(Succeed())
            Expect(k8sClient.Delete(ctx, ke)).To(Succeed())

            // Cleanup should succeed even if Job doesn't exist
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(ke),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed despite job deletion failure
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(ke), ke)
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })
    })
})
```

---

## CRD Lifecycle Management

### Creation Lifecycle

**Created By**: RemediationRequest controller (centralized orchestration)

**Creation Trigger**: WorkflowExecution completion (with validated workflow)

**Sequence**:
```
WorkflowExecution.status.phase = "completed"
    ↓ (watch trigger <100ms)
RemediationRequest Controller reconciles
    ↓
RemediationRequest extracts workflow definition
    ↓
RemediationRequest Controller creates KubernetesExecution CRD
    ↓ (with owner reference)
KubernetesExecution Controller reconciles (this controller)
    ↓
KubernetesExecution validates action
    ↓
KubernetesExecution creates Kubernetes Job
    ↓
KubernetesExecution monitors Job execution
    ↓
KubernetesExecution.status.phase = "completed"
    ↓ (watch trigger <100ms)
RemediationRequest Controller detects completion
    ↓
RemediationRequest marks remediation complete
```

**Owner Reference Set at Creation**:
```go
package controller

import (
    "context"
    "fmt"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetes/execution/v1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// In RemediationRequestReconciler
func (r *RemediationRequestReconciler) createKubernetesExecution(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    workflowStep workflowexecutionv1.WorkflowStep,
) error {
    kubernetesExecution := &kubernetesexecutionv1.KubernetesExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-exec-%d", remediation.Name, workflowStep.Order),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation,
                    remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: kubernetesexecutionv1.KubernetesExecutionSpec{
            RemediationRequestRef: kubernetesexecutionv1.RemediationRequestReference{
                Name:      remediation.Name,
                Namespace: remediation.Namespace,
            },
            Action:     workflowStep.Action,
            Parameters: workflowStep.Parameters,
            AlertContext: kubernetesexecutionv1.AlertContext{
                Fingerprint: remediation.Spec.SignalFingerprint,
                Environment: remediation.Status.Environment,
            },
        },
    }

    return r.Create(ctx, kubernetesExecution)
}
```

**Result**: KubernetesExecution is owned by RemediationRequest (cascade deletion applies)

---

### Update Lifecycle

**Status Updates by KubernetesExecution Controller**:

```go
package controller

import (
    "context"
    "time"

    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetes/execution/v1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *KubernetesExecutionReconciler) updateStatusCompleted(
    ctx context.Context,
    ke *kubernetesexecutionv1.KubernetesExecution,
    executionResult kubernetesexecutionv1.ExecutionResult,
    rollbackInfo *kubernetesexecutionv1.RollbackInfo,
) error {
    // Controller updates own status
    ke.Status.Phase = "completed"
    ke.Status.CompletionTime = &metav1.Time{Time: time.Now()}
    ke.Status.ExecutionResult = executionResult
    ke.Status.RollbackInfo = rollbackInfo

    return r.Status().Update(ctx, ke)
}
```

**Watch Triggers RemediationRequest Reconciliation**:

```
KubernetesExecution.status.phase = "completed"
    ↓ (watch event)
RemediationRequest watch triggers
    ↓ (<100ms latency)
RemediationRequest Controller reconciles
    ↓
RemediationRequest checks if all workflow steps completed
    ↓
RemediationRequest marks overall remediation complete
```

**No Self-Updates After Completion**:
- KubernetesExecution does NOT update itself after `phase = "completed"`
- KubernetesExecution does NOT create other CRDs (leaf controller)
- KubernetesExecution does NOT watch other CRDs (except its owned Jobs)

---

### Deletion Lifecycle

**Trigger**: RemediationRequest deletion (cascade)

**Cascade Deletion Sequence**:
```
User/System deletes RemediationRequest
    ↓
Kubernetes garbage collector detects owner reference
    ↓ (parallel deletion of all owned CRDs)
KubernetesExecution.deletionTimestamp set
    ↓
KubernetesExecution Controller reconciles (detects deletion)
    ↓
Finalizer cleanup executes:
  - Delete running Kubernetes Job (best-effort)
  - Record final execution audit
  - Emit deletion event
    ↓
Finalizer removed
    ↓
Kubernetes deletes KubernetesExecution CRD
    ↓
Kubernetes Job cascade-deleted (owner reference)
```

**Parallel Deletion**: All service CRDs (RemediationProcessing, AIAnalysis, WorkflowExecution, KubernetesExecution) deleted in parallel when RemediationRequest is deleted.

**Retention**:
- **KubernetesExecution**: No independent retention (deleted with parent)
- **RemediationRequest**: 24-hour retention (parent CRD manages retention)
- **Kubernetes Jobs**: Cascade-deleted with KubernetesExecution (owner reference)
- **Audit Data**: 90-day retention in PostgreSQL (persisted before deletion)

---

### Lifecycle Events

**Kubernetes Events Emitted**:

```go
package controller

import (
    "fmt"
    "time"

    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetes/execution/v1"

    "k8s.io/client-go/tools/record"
)

func (r *KubernetesExecutionReconciler) emitLifecycleEvents(
    ke *kubernetesexecutionv1.KubernetesExecution,
    oldPhase string,
    duration time.Duration,
) {
    // Creation event
    r.Recorder.Event(ke, "Normal", "KubernetesExecutionCreated",
        fmt.Sprintf("Kubernetes execution started for action: %s", ke.Spec.Action))

    // Phase transition events
    r.Recorder.Event(ke, "Normal", "PhaseTransition",
        fmt.Sprintf("Phase: %s → %s", oldPhase, ke.Status.Phase))

    // Job creation
    if ke.Status.Phase == "executing" && ke.Status.JobName != "" {
        r.Recorder.Event(ke, "Normal", "JobCreated",
            fmt.Sprintf("Kubernetes Job created: %s", ke.Status.JobName))
    }

    // Validation events
    if ke.Status.Phase == "validating" {
        r.Recorder.Event(ke, "Normal", "ActionValidating",
            fmt.Sprintf("Validating action %s with Rego policy", ke.Spec.Action))
    }

    // Execution result events
    if ke.Status.Phase == "completed" {
        if ke.Status.ExecutionResult.Status == "success" {
            r.Recorder.Event(ke, "Normal", "ExecutionSucceeded",
                fmt.Sprintf("Action %s completed successfully", ke.Spec.Action))
        } else {
            r.Recorder.Event(ke, "Warning", "ExecutionFailed",
                fmt.Sprintf("Action %s failed: %s", ke.Spec.Action, ke.Status.ExecutionResult.Message))
        }
    }

    // Rollback preparation
    if ke.Status.RollbackInfo != nil {
        r.Recorder.Event(ke, "Normal", "RollbackPrepared",
            "Rollback information captured for potential revert")
    }

    // Completion event
    r.Recorder.Event(ke, "Normal", "KubernetesExecutionCompleted",
        fmt.Sprintf("Execution completed in %s", duration))

    // Deletion event (in cleanup function)
    r.Recorder.Event(ke, "Normal", "KubernetesExecutionDeleted",
        fmt.Sprintf("KubernetesExecution cleanup completed (phase: %s, action: %s)",
            ke.Status.Phase, ke.Spec.Action))
}
```

**Event Visibility**:
```bash
kubectl describe kubernetesexecution <name>
# Shows all events in chronological order

kubectl get events --field-selector involvedObject.name=<name>
# Filter events for specific KubernetesExecution
```

---

### Lifecycle Monitoring

**Prometheus Metrics**:

```promql
# CRD creation rate
rate(kubernetesexecution_created_total[5m])

# CRD completion time (end-to-end)
histogram_quantile(0.95, kubernetesexecution_lifecycle_duration_seconds)

# Active KubernetesExecution CRDs
kubernetesexecution_active_total

# CRD deletion rate
rate(kubernetesexecution_deleted_total[5m])

# Execution success rate by action
sum(rate(kubernetesexecution_execution_result{status="success"}[5m])) by (action) /
sum(rate(kubernetesexecution_execution_result[5m])) by (action)

# Rollback preparation rate
rate(kubernetesexecution_rollback_prepared_total[5m])

# Job failure rate
rate(kubernetesexecution_job_failures_total[5m])
```

**Grafana Dashboard**:
```yaml
panels:
  - title: "KubernetesExecution Lifecycle"
    targets:
      - expr: kubernetesexecution_active_total
        legendFormat: "Active CRDs"
      - expr: rate(kubernetesexecution_created_total[5m])
        legendFormat: "Creation Rate"
      - expr: rate(kubernetesexecution_deleted_total[5m])
        legendFormat: "Deletion Rate"

  - title: "Execution Latency by Action (P95)"
    targets:
      - expr: histogram_quantile(0.95, rate(kubernetesexecution_lifecycle_duration_seconds_bucket[5m]))
        legendFormat: "{{action}}"

  - title: "Execution Success Rate by Action"
    targets:
      - expr: |
          sum(rate(kubernetesexecution_execution_result{status="success"}[5m])) by (action) /
          sum(rate(kubernetesexecution_execution_result[5m])) by (action)
        legendFormat: "{{action}}"

  - title: "Rollback Preparation Rate"
    targets:
      - expr: rate(kubernetesexecution_rollback_prepared_total[5m])
        legendFormat: "Rollbacks Prepared"
```

**Alert Rules**:

```yaml
groups:
- name: kubernetesexecution-lifecycle
  rules:
  - alert: KubernetesExecutionStuckInPhase
    expr: time() - kubernetesexecution_phase_start_timestamp > 600
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "KubernetesExecution stuck in phase for >10 minutes"
      description: "KubernetesExecution {{ $labels.name }} (action: {{ $labels.action }}) has been in phase {{ $labels.phase }} for over 10 minutes"

  - alert: KubernetesExecutionHighFailureRate
    expr: |
      sum(rate(kubernetesexecution_execution_result{status="failed"}[5m])) by (action) /
      sum(rate(kubernetesexecution_execution_result[5m])) by (action) > 0.2
    for: 10m
    labels:
      severity: critical
    annotations:
      summary: "High execution failure rate for action {{ $labels.action }}"
      description: ">20% of {{ $labels.action }} executions are failing"

  - alert: KubernetesExecutionJobFailures
    expr: rate(kubernetesexecution_job_failures_total[5m]) > 0.1
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Kubernetes Jobs failing frequently"
      description: "Job failure rate exceeds 10%"

  - alert: KubernetesExecutionHighDeletionRate
    expr: rate(kubernetesexecution_deleted_total[5m]) > rate(kubernetesexecution_created_total[5m]) * 1.5
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "KubernetesExecution deletion rate exceeds creation rate"
      description: "More KubernetesExecution CRDs being deleted than created (possible cascade deletion issue)"

  - alert: KubernetesExecutionLowRollbackPreparation
    expr: |
      rate(kubernetesexecution_rollback_prepared_total[5m]) /
      rate(kubernetesexecution_completed_total[5m]) < 0.5
    for: 10m
    labels:
      severity: info
    annotations:
      summary: "Low rollback preparation rate"
      description: "<50% of executions are preparing rollback information"
```

---

*[Document continues with remaining sections: Prometheus Metrics, Testing Strategy, Performance Targets, Database Integration, Integration Points, RBAC Configuration, Implementation Checklist, Critical Architectural Patterns, Common Pitfalls, and Summary]*

---

**Design Specification Status**: 60% Complete (Core architecture defined, awaiting detailed sections)

**Next Steps**: Complete remaining sections following [CRD Service Specification Template](../../../development/templates/CRD_SERVICE_SPECIFICATION_TEMPLATE.md) structure.


---

