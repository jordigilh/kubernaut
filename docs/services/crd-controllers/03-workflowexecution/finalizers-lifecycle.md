## Finalizer Implementation

**Version**: 3.1
**Last Updated**: 2025-12-02
**CRD API Group**: `kubernaut.ai/v1alpha1`
**Status**: ✅ Updated for Tekton Architecture

---

## Changelog

### Version 3.1 (2025-12-02)
- ✅ **Removed**: All KubernetesExecution (DEPRECATED - ADR-025) references
- ✅ **Updated**: Lifecycle documentation for Tekton-based architecture

---

### Finalizer Name

Following Kubernetes finalizer naming convention:

```go
const workflowExecutionFinalizer = "kubernaut.ai/workflowexecution-cleanup"
```

**Naming Pattern**: `{domain}.kubernaut.ai/{resource}-cleanup`

**Why This Pattern**:
- **Domain-Scoped**: `kubernaut.ai` prevents conflicts with other services
- **Resource-Specific**: `workflowexecution-cleanup` clearly indicates what's being cleaned up
- **Kubernetes Convention**: Follows standard finalizer naming (domain/action format)

---

### Complete Reconciliation Loop with Finalizer

```go
package controller

import (
    "context"
    "fmt"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflow/execution/v1"

    "github.com/go-logr/logr"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const workflowExecutionFinalizer = "kubernaut.ai/workflowexecution-cleanup"

type WorkflowExecutionReconciler struct {
    client.Client
    Scheme            *runtime.Scheme
    Log               logr.Logger
    Recorder          record.EventRecorder
    WorkflowBuilder   WorkflowBuilder
    StorageClient     StorageClient
}

func (r *WorkflowExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var we workflowexecutionv1.WorkflowExecution
    if err := r.Get(ctx, req.NamespacedName, &we); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // ========================================
    // DELETION HANDLING WITH FINALIZER
    // ========================================
    if !we.ObjectMeta.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(&we, workflowExecutionFinalizer) {
            // Perform cleanup before deletion
            if err := r.cleanupWorkflowExecution(ctx, &we); err != nil {
                r.Log.Error(err, "Failed to cleanup WorkflowExecution resources",
                    "name", we.Name,
                    "namespace", we.Namespace,
                )
                return ctrl.Result{}, err
            }

            // Remove finalizer to allow deletion
            controllerutil.RemoveFinalizer(&we, workflowExecutionFinalizer)
            if err := r.Update(ctx, &we); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // ========================================
    // ADD FINALIZER IF NOT PRESENT
    // ========================================
    if !controllerutil.ContainsFinalizer(&we, workflowExecutionFinalizer) {
        controllerutil.AddFinalizer(&we, workflowExecutionFinalizer)
        if err := r.Update(ctx, &we); err != nil {
            return ctrl.Result{}, err
        }
    }

    // ========================================
    // NORMAL RECONCILIATION LOGIC
    // ========================================

    // Skip if already completed or failed
    if we.Status.Phase == "completed" || we.Status.Phase == "failed" {
        return ctrl.Result{}, nil
    }

    // Execute workflow building and validation...
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

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflow/execution/v1"
)

func (r *WorkflowExecutionReconciler) cleanupWorkflowExecution(
    ctx context.Context,
    we *workflowexecutionv1.WorkflowExecution,
) error {
    r.Log.Info("Cleaning up WorkflowExecution resources",
        "name", we.Name,
        "namespace", we.Namespace,
        "phase", we.Status.Phase,
    )

    // 1. Record final audit to database
    if err := r.recordFinalAudit(ctx, we); err != nil {
        r.Log.Error(err, "Failed to record final audit", "name", we.Name)
        // Don't block deletion on audit failure
        // Audit is best-effort during cleanup
    }

    // 2. Emit deletion event
    r.Recorder.Event(we, "Normal", "WorkflowExecutionDeleted",
        fmt.Sprintf("WorkflowExecution cleanup completed (phase: %s)", we.Status.Phase))

    r.Log.Info("WorkflowExecution cleanup completed successfully",
        "name", we.Name,
        "namespace", we.Namespace,
    )

    return nil
}

func (r *WorkflowExecutionReconciler) recordFinalAudit(
    ctx context.Context,
    we *workflowexecutionv1.WorkflowExecution,
) error {
    auditRecord := &AuditRecord{
        AlertFingerprint: we.Spec.SignalContext.Fingerprint,
        ServiceType:      "WorkflowExecution",
        CRDName:          we.Name,
        Namespace:        we.Namespace,
        Phase:            we.Status.Phase,
        CreatedAt:        we.CreationTimestamp.Time,
        DeletedAt:        we.DeletionTimestamp.Time,
        WorkflowSteps:    len(we.Status.Workflow.Steps),
        ValidationStatus: we.Status.ValidationStatus,
    }

    return r.StorageClient.RecordAudit(ctx, auditRecord)
}
```

**Cleanup Philosophy for WorkflowExecution** (Leaf Controller):
- ✅ **Record final audit**: Capture workflow definition (best-effort)
- ✅ **Emit deletion event**: Operational visibility
- ✅ **PipelineRun cleanup**: Owned PipelineRun deleted via garbage collection
- ✅ **Non-blocking**: Audit failures don't block deletion (best-effort)

**Note**: WorkflowExecution creates Tekton PipelineRun to execute workflows. RemediationOrchestrator orchestrates the overall remediation flow.

---

### Finalizer Testing

**Unit Test Pattern**:

```go
package controller_test

import (
    "context"
    "fmt"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflow/execution/v1"
    "github.com/jordigilh/kubernaut/pkg/workflow/execution/controller"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/kubernetes/scheme"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("WorkflowExecution Finalizer", func() {
    var (
        ctx       context.Context
        k8sClient client.Client
        reconciler *controller.WorkflowExecutionReconciler
    )

    BeforeEach(func() {
        ctx = context.Background()
        k8sClient = fake.NewClientBuilder().
            WithScheme(scheme.Scheme).
            Build()

        reconciler = &controller.WorkflowExecutionReconciler{
            Client:        k8sClient,
            StorageClient: &mockStorageClient{},
        }
    })

    Context("when WorkflowExecution is created", func() {
        It("should add finalizer on first reconcile", func() {
            we := &workflowexecutionv1.WorkflowExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-workflow",
                    Namespace: "default",
                },
                Spec: workflowexecutionv1.WorkflowExecutionSpec{
                    Recommendations: []workflowexecutionv1.RemediationRecommendation{
                        {Action: "restart_pod", Confidence: 0.95},
                    },
                },
            }
            Expect(k8sClient.Create(ctx, we)).To(Succeed())

            // First reconcile should add finalizer
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(we),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer added
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(we), we)).To(Succeed())
            Expect(controllerutil.ContainsFinalizer(we, workflowExecutionFinalizer)).To(BeTrue())
        })
    })

    Context("when WorkflowExecution is deleted", func() {
        It("should execute cleanup and remove finalizer", func() {
            we := &workflowexecutionv1.WorkflowExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-workflow",
                    Namespace:  "default",
                    Finalizers: []string{workflowExecutionFinalizer},
                },
                Status: workflowexecutionv1.WorkflowExecutionStatus{
                    Phase: "completed",
                },
            }
            Expect(k8sClient.Create(ctx, we)).To(Succeed())

            // Delete WorkflowExecution
            Expect(k8sClient.Delete(ctx, we)).To(Succeed())

            // Reconcile should execute cleanup
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(we),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed (CRD will be deleted by Kubernetes)
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(we), we)
            Expect(err).To(HaveOccurred())
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })

        It("should not block deletion if audit fails", func() {
            // Mock storage client to return error
            reconciler.StorageClient = &mockStorageClient{
                recordAuditError: fmt.Errorf("database unavailable"),
            }

            we := &workflowexecutionv1.WorkflowExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-workflow",
                    Namespace:  "default",
                    Finalizers: []string{workflowExecutionFinalizer},
                },
            }
            Expect(k8sClient.Create(ctx, we)).To(Succeed())
            Expect(k8sClient.Delete(ctx, we)).To(Succeed())

            // Cleanup should succeed even if audit fails
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(we),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed despite audit failure
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(we), we)
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })
    })
})
```

---

## CRD Lifecycle Management

### Creation Lifecycle

**Created By**: RemediationRequest controller (centralized orchestration)

**Creation Trigger**: AIAnalysis completion (with approved recommendations)

**Sequence**:
```
AIAnalysis.status.phase = "completed"
    ↓ (watch trigger <100ms)
RemediationRequest Controller reconciles
    ↓
RemediationRequest extracts approved recommendations
    ↓
RemediationRequest Controller creates WorkflowExecution CRD
    ↓ (with owner reference)
WorkflowExecution Controller reconciles (this controller)
    ↓
WorkflowExecution builds multi-step workflow
    ↓
WorkflowExecution validates workflow steps
    ↓
WorkflowExecution.status.phase = "completed"
    ↓ (watch trigger <100ms)
RemediationRequest Controller detects completion
    ↓
WorkflowExecution Controller creates Tekton PipelineRun
```

**Owner Reference Set at Creation**:
```go
package controller

import (
    "context"
    "fmt"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflow/execution/v1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// In RemediationRequestReconciler
func (r *RemediationRequestReconciler) createWorkflowExecution(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    recommendations []workflowexecutionv1.RemediationRecommendation,
) error {
    workflowExecution := &workflowexecutionv1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-workflow", remediation.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation,
                    remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: workflowexecutionv1.WorkflowExecutionSpec{
            RemediationRequestRef: workflowexecutionv1.RemediationRequestReference{
                Name:      remediation.Name,
                Namespace: remediation.Namespace,
            },
            Recommendations: recommendations,
            AlertContext: workflowexecutionv1.SignalContext{
                Fingerprint: remediation.Spec.SignalFingerprint,
                Environment: remediation.Status.Environment,
            },
        },
    }

    return r.Create(ctx, workflowExecution)
}
```

**Result**: WorkflowExecution is owned by RemediationRequest (cascade deletion applies)

---

### Update Lifecycle

**Status Updates by WorkflowExecution Controller**:

```go
package controller

import (
    "context"
    "time"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflow/execution/v1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *WorkflowExecutionReconciler) updateStatusCompleted(
    ctx context.Context,
    we *workflowexecutionv1.WorkflowExecution,
    workflow workflowexecutionv1.Workflow,
    validationStatus string,
) error {
    // Controller updates own status
    we.Status.Phase = "completed"
    we.Status.CompletionTime = &metav1.Time{Time: time.Now()}
    we.Status.Workflow = workflow
    we.Status.ValidationStatus = validationStatus

    return r.Status().Update(ctx, we)
}
```

**Watch Triggers RemediationOrchestrator Reconciliation**:

```
WorkflowExecution.status.phase = "Completed"
    ↓ (watch event)
RemediationOrchestrator watch triggers
    ↓ (<100ms latency)
RemediationOrchestrator Controller reconciles
    ↓
RemediationOrchestrator updates RemediationRequest status
```

**Architecture Notes**:
- WorkflowExecution does NOT update itself after terminal phase
- WorkflowExecution owns Tekton PipelineRun (cascade deletion)
- WorkflowExecution watches PipelineRun for status sync

---

### Deletion Lifecycle

**Trigger**: RemediationRequest deletion (cascade)

**Cascade Deletion Sequence**:
```
User/System deletes RemediationRequest
    ↓
Kubernetes garbage collector detects owner reference
    ↓ (parallel deletion of all owned CRDs)
WorkflowExecution.deletionTimestamp set
    ↓
WorkflowExecution Controller reconciles (detects deletion)
    ↓
Finalizer cleanup executes:
  - Record final workflow audit
  - Emit deletion event
    ↓
Finalizer removed
    ↓
Kubernetes deletes WorkflowExecution CRD
```

**Parallel Deletion**: All service CRDs (RemediationProcessing, AIAnalysis, WorkflowExecution) and PipelineRuns deleted in parallel when RemediationRequest is deleted.

**Retention**:
- **WorkflowExecution**: No independent retention (deleted with parent)
- **RemediationRequest**: 24-hour retention (parent CRD manages retention)
- **Audit Data**: 90-day retention in PostgreSQL (persisted before deletion)

---

### Lifecycle Events

**Kubernetes Events Emitted**:

```go
package controller

import (
    "fmt"
    "time"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflow/execution/v1"

    "k8s.io/client-go/tools/record"
)

func (r *WorkflowExecutionReconciler) emitLifecycleEvents(
    we *workflowexecutionv1.WorkflowExecution,
    oldPhase string,
    duration time.Duration,
) {
    // Creation event
    r.Recorder.Event(we, "Normal", "WorkflowExecutionCreated",
        fmt.Sprintf("Workflow building started for %d recommendations", len(we.Spec.Recommendations)))

    // Phase transition events
    r.Recorder.Event(we, "Normal", "PhaseTransition",
        fmt.Sprintf("Phase: %s → %s", oldPhase, we.Status.Phase))

    // Workflow generated
    if we.Status.Phase == "completed" && we.Status.Workflow.Steps != nil {
        r.Recorder.Event(we, "Normal", "WorkflowGenerated",
            fmt.Sprintf("Generated workflow with %d steps", len(we.Status.Workflow.Steps)))
    }

    // Validation events
    if we.Status.ValidationStatus == "passed" {
        r.Recorder.Event(we, "Normal", "WorkflowValidated",
            "Workflow validation passed")
    } else if we.Status.ValidationStatus == "failed" {
        r.Recorder.Event(we, "Warning", "WorkflowValidationFailed",
            "Workflow validation failed - review workflow definition")
    }

    // Completion event
    r.Recorder.Event(we, "Normal", "WorkflowExecutionCompleted",
        fmt.Sprintf("Workflow building completed in %s", duration))

    // Deletion event (in cleanup function)
    r.Recorder.Event(we, "Normal", "WorkflowExecutionDeleted",
        fmt.Sprintf("WorkflowExecution cleanup completed (phase: %s)", we.Status.Phase))
}
```

**Event Visibility**:
```bash
kubectl describe workflowexecution <name>
# Shows all events in chronological order

kubectl get events --field-selector involvedObject.name=<name>
# Filter events for specific WorkflowExecution
```

---

### Lifecycle Monitoring

**Prometheus Metrics**:

```promql
# CRD creation rate
rate(workflowexecution_created_total[5m])

# CRD completion time (end-to-end)
histogram_quantile(0.95, workflowexecution_lifecycle_duration_seconds)

# Active WorkflowExecution CRDs
workflowexecution_active_total

# CRD deletion rate
rate(workflowexecution_deleted_total[5m])

# Workflow validation failure rate
rate(workflowexecution_validation_failures_total[5m])

# Workflow step count distribution
histogram_quantile(0.95, workflowexecution_workflow_steps_total)
```

**Grafana Dashboard**:
```yaml
panels:
  - title: "WorkflowExecution Lifecycle"
    targets:
      - expr: workflowexecution_active_total
        legendFormat: "Active CRDs"
      - expr: rate(workflowexecution_created_total[5m])
        legendFormat: "Creation Rate"
      - expr: rate(workflowexecution_deleted_total[5m])
        legendFormat: "Deletion Rate"

  - title: "Workflow Building Latency (P95)"
    targets:
      - expr: histogram_quantile(0.95, workflowexecution_lifecycle_duration_seconds)
        legendFormat: "P95 Duration"

  - title: "Workflow Validation Status"
    targets:
      - expr: sum(workflowexecution_validation_status{status="passed"})
        legendFormat: "Passed"
      - expr: sum(workflowexecution_validation_status{status="failed"})
        legendFormat: "Failed"
```

**Alert Rules**:

```yaml
groups:
- name: workflowexecution-lifecycle
  rules:
  - alert: WorkflowExecutionStuckInPhase
    expr: time() - workflowexecution_phase_start_timestamp > 300
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "WorkflowExecution stuck in phase for >5 minutes"
      description: "WorkflowExecution {{ $labels.name }} has been in phase {{ $labels.phase }} for over 5 minutes"

  - alert: WorkflowExecutionHighValidationFailureRate
    expr: rate(workflowexecution_validation_failures_total[5m]) > 0.1
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "High workflow validation failure rate"
      description: "Workflow validation failing for >10% of executions"

  - alert: WorkflowExecutionHighDeletionRate
    expr: rate(workflowexecution_deleted_total[5m]) > rate(workflowexecution_created_total[5m]) * 1.5
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "WorkflowExecution deletion rate exceeds creation rate"
      description: "More WorkflowExecution CRDs being deleted than created (possible cascade deletion issue)"

  - alert: WorkflowExecutionComplexWorkflows
    expr: histogram_quantile(0.95, workflowexecution_workflow_steps_total) > 10
    for: 10m
    labels:
      severity: info
    annotations:
      summary: "P95 workflow complexity exceeds 10 steps"
      description: "Generated workflows are becoming complex - review recommendation quality"
```

---

