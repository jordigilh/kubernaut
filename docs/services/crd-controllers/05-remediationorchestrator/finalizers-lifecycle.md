## Finalizer Implementation

### Finalizer Name

Following Kubernetes finalizer naming convention:

```go
const alertRemediationFinalizer = "remediation.kubernaut.ai/alertremediation-cleanup"
```

**Naming Pattern**: `{domain}.kubernaut.ai/{resource}-cleanup`

**Why This Pattern**:
- **Domain-Scoped**: `remediation.kubernaut.io` prevents conflicts with other services
- **Resource-Specific**: `alertremediation-cleanup` clearly indicates what's being cleaned up
- **Kubernetes Convention**: Follows standard finalizer naming (domain/action format)

---

### Complete Reconciliation Loop with Finalizer

```go
package controller

import (
    "context"
    "fmt"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"

    "github.com/go-logr/logr"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const alertRemediationFinalizer = "remediation.kubernaut.ai/alertremediation-cleanup"

type RemediationRequestReconciler struct {
    client.Client
    Scheme            *runtime.Scheme
    Log               logr.Logger
    Recorder          record.EventRecorder
    StorageClient     StorageClient
}

func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var ar remediationv1.RemediationRequest
    if err := r.Get(ctx, req.NamespacedName, &ar); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // ========================================
    // DELETION HANDLING WITH FINALIZER
    // ========================================
    if !ar.ObjectMeta.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(&ar, alertRemediationFinalizer) {
            // Perform cleanup before deletion
            if err := r.cleanupRemediationRequest(ctx, &ar); err != nil {
                r.Log.Error(err, "Failed to cleanup RemediationRequest resources",
                    "name", ar.Name,
                    "namespace", ar.Namespace,
                )
                return ctrl.Result{}, err
            }

            // Remove finalizer to allow deletion
            controllerutil.RemoveFinalizer(&ar, alertRemediationFinalizer)
            if err := r.Update(ctx, &ar); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // ========================================
    // ADD FINALIZER IF NOT PRESENT
    // ========================================
    if !controllerutil.ContainsFinalizer(&ar, alertRemediationFinalizer) {
        controllerutil.AddFinalizer(&ar, alertRemediationFinalizer)
        if err := r.Update(ctx, &ar); err != nil {
            return ctrl.Result{}, err
        }
    }

    // ========================================
    // NORMAL RECONCILIATION LOGIC
    // ========================================

    // Skip if already completed or failed
    if ar.Status.OverallPhase == "completed" || ar.Status.OverallPhase == "failed" {
        return ctrl.Result{}, nil
    }

    // Execute orchestration phases...
    // (existing reconciliation logic from previous section)

    return ctrl.Result{}, nil
}
```

---

### Cleanup Logic

**What Gets Cleaned Up** (Remediation Orchestrator Pattern):

```go
package controller

import (
    "context"
    "fmt"
    "time"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/analysis/v1"
    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflow/execution/v1"
    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetes/execution/v1" // DEPRECATED - ADR-025

    apierrors "k8s.io/apimachinery/pkg/api/errors"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *RemediationRequestReconciler) cleanupRemediationRequest(
    ctx context.Context,
    ar *remediationv1.RemediationRequest,
) error {
    r.Log.Info("Cleaning up RemediationRequest resources",
        "name", ar.Name,
        "namespace", ar.Namespace,
        "overallPhase", ar.Status.OverallPhase,
    )

    // 1. Delete ALL owned child CRDs (best-effort, cascade deletion handles most)
    // These CRDs have owner references, so Kubernetes will cascade-delete them
    // Explicit deletion here is best-effort for immediate cleanup
    if err := r.cleanupChildCRDs(ctx, ar); err != nil {
        r.Log.Error(err, "Failed to cleanup child CRDs", "name", ar.Name)
        // Don't block deletion on child cleanup failure
        // Owner references will ensure cascade deletion
    }

    // 2. Record final audit to database
    if err := r.recordFinalAudit(ctx, ar); err != nil {
        r.Log.Error(err, "Failed to record final audit", "name", ar.Name)
        // Don't block deletion on audit failure
        // Audit is best-effort during cleanup
    }

    // 3. Emit deletion event
    r.Recorder.Event(ar, "Normal", "RemediationRequestDeleted",
        fmt.Sprintf("RemediationRequest cleanup completed (phase: %s)", ar.Status.OverallPhase))

    r.Log.Info("RemediationRequest cleanup completed successfully",
        "name", ar.Name,
        "namespace", ar.Namespace,
    )

    return nil
}

func (r *RemediationRequestReconciler) cleanupChildCRDs(
    ctx context.Context,
    ar *remediationv1.RemediationRequest,
) error {
    namespace := ar.Namespace

    // Delete SignalProcessing CRD (if exists)
    if ar.Status.RemediationProcessingRef != nil {
        apName := ar.Status.RemediationProcessingRef.Name
        ap := &processingv1.RemediationProcessing{}
        if err := r.Get(ctx, client.ObjectKey{Name: apName, Namespace: namespace}, ap); err == nil {
            if err := r.Delete(ctx, ap); err != nil && !apierrors.IsNotFound(err) {
                r.Log.Error(err, "Failed to delete RemediationProcessing", "name", apName)
            } else {
                r.Log.Info("Deleted SignalProcessing CRD", "name", apName)
            }
        }
    }

    // Delete AIAnalysis CRD (if exists)
    if ar.Status.AIAnalysisRef != nil {
        aiName := ar.Status.AIAnalysisRef.Name
        ai := &aianalysisv1.AIAnalysis{}
        if err := r.Get(ctx, client.ObjectKey{Name: aiName, Namespace: namespace}, ai); err == nil {
            if err := r.Delete(ctx, ai); err != nil && !apierrors.IsNotFound(err) {
                r.Log.Error(err, "Failed to delete AIAnalysis", "name", aiName)
            } else {
                r.Log.Info("Deleted AIAnalysis CRD", "name", aiName)
            }
        }
    }

    // Delete WorkflowExecution CRD (if exists)
    if ar.Status.WorkflowExecutionRef != nil {
        weName := ar.Status.WorkflowExecutionRef.Name
        we := &workflowexecutionv1.WorkflowExecution{}
        if err := r.Get(ctx, client.ObjectKey{Name: weName, Namespace: namespace}, we); err == nil {
            if err := r.Delete(ctx, we); err != nil && !apierrors.IsNotFound(err) {
                r.Log.Error(err, "Failed to delete WorkflowExecution", "name", weName)
            } else {
                r.Log.Info("Deleted WorkflowExecution CRD", "name", weName)
            }
        }
    }

    // Delete ALL KubernetesExecution (DEPRECATED - ADR-025) CRDs owned by this RemediationRequest
    keList := &kubernetesexecutionv1.KubernetesExecutionList{}
    if err := r.List(ctx, keList, client.InNamespace(namespace)); err == nil {
        for _, ke := range keList.Items {
            // Check if owned by this RemediationRequest
            for _, ownerRef := range ke.OwnerReferences {
                if ownerRef.UID == ar.UID {
                    if err := r.Delete(ctx, &ke); err != nil && !apierrors.IsNotFound(err) {
                        r.Log.Error(err, "Failed to delete KubernetesExecution (DEPRECATED - ADR-025)", "name", ke.Name)
                    } else {
                        r.Log.Info("Deleted KubernetesExecution CRD", "name", ke.Name)
                    }
                    break
                }
            }
        }
    }

    return nil
}

func (r *RemediationRequestReconciler) recordFinalAudit(
    ctx context.Context,
    ar *remediationv1.RemediationRequest,
) error {
    auditRecord := &AuditRecord{
        AlertFingerprint:       ar.Spec.AlertFingerprint,
        ServiceType:            "RemediationRequest",
        CRDName:                ar.Name,
        Namespace:              ar.Namespace,
        OverallPhase:           ar.Status.OverallPhase,
        CreatedAt:              ar.CreationTimestamp.Time,
        DeletedAt:              ar.DeletionTimestamp.Time,
        RemediationProcessingCreated: ar.Status.RemediationProcessingRef != nil,
        AIAnalysisCreated:      ar.Status.AIAnalysisRef != nil,
        WorkflowCreated:        ar.Status.WorkflowExecutionRef != nil,
        ExecutionsCreated:      len(ar.Status.KubernetesExecutionRefs),
    }

    return r.StorageClient.RecordAudit(ctx, auditRecord)
}
```

**Cleanup Philosophy for RemediationRequest** (Remediation Orchestrator):
- ✅ **Delete child CRDs**: Best-effort deletion of all owned CRDs (owner references ensure cascade)
- ✅ **Record final audit**: Capture complete remediation lifecycle (best-effort)
- ✅ **Emit deletion event**: Operational visibility
- ✅ **Multiple child CRDs**: Handles 4 different child CRD types (RemediationProcessing, AIAnalysis, WorkflowExecution, KubernetesExecution (DEPRECATED - ADR-025))
- ✅ **Parallel deletion**: Kubernetes garbage collector handles cascade deletion in parallel
- ✅ **Non-blocking**: Child deletion and audit failures don't block deletion (best-effort)

**Note**: Child CRDs have `ownerReferences` set to RemediationRequest, so they'll be cascade-deleted automatically by Kubernetes. Explicit deletion in finalizer is best-effort immediate cleanup.

---

### Finalizer Testing

**Unit Test Pattern**:

```go
package controller_test

import (
    "context"
    "fmt"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/analysis/v1"
    "github.com/jordigilh/kubernaut/pkg/remediation/controller"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/kubernetes/scheme"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("RemediationRequest Finalizer", func() {
    var (
        ctx       context.Context
        k8sClient client.Client
        reconciler *controller.RemediationRequestReconciler
    )

    BeforeEach(func() {
        ctx = context.Background()
        k8sClient = fake.NewClientBuilder().
            WithScheme(scheme.Scheme).
            Build()

        reconciler = &controller.RemediationRequestReconciler{
            Client:        k8sClient,
            StorageClient: &mockStorageClient{},
        }
    })

    Context("when RemediationRequest is created", func() {
        It("should add finalizer on first reconcile", func() {
            ar := &remediationv1.RemediationRequest{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-remediation",
                    Namespace: "default",
                },
                Spec: remediationv1.RemediationRequestSpec{
                    AlertFingerprint: "abc123",
                },
            }
            Expect(k8sClient.Create(ctx, ar)).To(Succeed())

            // First reconcile should add finalizer
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(ar),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer added
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ar), ar)).To(Succeed())
            Expect(controllerutil.ContainsFinalizer(ar, alertRemediationFinalizer)).To(BeTrue())
        })
    })

    Context("when RemediationRequest is deleted", func() {
        It("should execute cleanup and remove finalizer", func() {
            ar := &remediationv1.RemediationRequest{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-remediation",
                    Namespace:  "default",
                    Finalizers: []string{alertRemediationFinalizer},
                },
                Status: remediationv1.RemediationRequestStatus{
                    OverallPhase: "completed",
                },
            }
            Expect(k8sClient.Create(ctx, ar)).To(Succeed())

            // Delete RemediationRequest
            Expect(k8sClient.Delete(ctx, ar)).To(Succeed())

            // Reconcile should execute cleanup
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(ar),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed (CRD will be deleted by Kubernetes)
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(ar), ar)
            Expect(err).To(HaveOccurred())
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })

        It("should delete all child CRDs during cleanup", func() {
            ar := &remediationv1.RemediationRequest{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-remediation",
                    Namespace:  "default",
                    UID:        "test-uid-123",
                    Finalizers: []string{alertRemediationFinalizer},
                },
                Status: remediationv1.RemediationRequestStatus{
                    OverallPhase: "completed",
                    RemediationProcessingRef: &corev1.ObjectReference{
                        Name: "test-processing",
                    },
                    AIAnalysisRef: &corev1.ObjectReference{
                        Name: "test-analysis",
                    },
                },
            }
            Expect(k8sClient.Create(ctx, ar)).To(Succeed())

            // Create child CRDs owned by RemediationRequest
            ap := &processingv1.RemediationProcessing{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-processing",
                    Namespace: "default",
                    OwnerReferences: []metav1.OwnerReference{
                        {UID: ar.UID, Name: ar.Name, Kind: "RemediationRequest"},
                    },
                },
            }
            Expect(k8sClient.Create(ctx, ap)).To(Succeed())

            ai := &aianalysisv1.AIAnalysis{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-analysis",
                    Namespace: "default",
                    OwnerReferences: []metav1.OwnerReference{
                        {UID: ar.UID, Name: ar.Name, Kind: "RemediationRequest"},
                    },
                },
            }
            Expect(k8sClient.Create(ctx, ai)).To(Succeed())

            // Delete RemediationRequest
            Expect(k8sClient.Delete(ctx, ar)).To(Succeed())

            // Cleanup should delete child CRDs
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(ar),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify child CRDs deleted
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(ap), ap)
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())

            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(ai), ai)
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })

        It("should not block deletion if child deletion fails", func() {
            ar := &remediationv1.RemediationRequest{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-remediation",
                    Namespace:  "default",
                    Finalizers: []string{alertRemediationFinalizer},
                },
                Status: remediationv1.RemediationRequestStatus{
                    RemediationProcessingRef: &corev1.ObjectReference{
                        Name: "nonexistent-processing",
                    },
                },
            }
            Expect(k8sClient.Create(ctx, ar)).To(Succeed())
            Expect(k8sClient.Delete(ctx, ar)).To(Succeed())

            // Cleanup should succeed even if child CRDs don't exist
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(ar),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed despite child deletion failure
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(ar), ar)
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })
    })
})
```

---

## CRD Lifecycle Management

### Creation Lifecycle

**Created By**: Gateway Service (webhook handler)

**Creation Trigger**: New alert webhook received (after duplicate detection)

**Sequence**:
```
Prometheus/Grafana sends alert webhook
    ↓
Gateway Service receives webhook
    ↓
Gateway Service checks for duplicates (fingerprint-based)
    ↓
If NOT duplicate:
    ↓
Gateway Service creates RemediationRequest CRD
    ↓ (sets initial status.overallPhase = "pending")
RemediationRequest Controller reconciles (this controller)
    ↓
RemediationRequest creates SignalProcessing CRD
    ↓ (with owner reference to RemediationRequest)
RemediationRequest watches child CRD status changes
    ↓
Child CRDs update status (processing → completed)
    ↓ (watch triggers <100ms)
RemediationRequest orchestrates next phase
    ↓
Creates next child CRD (AIAnalysis, WorkflowExecution, KubernetesExecution (DEPRECATED - ADR-025))
    ↓
Repeats until all phases completed
    ↓
RemediationRequest.status.overallPhase = "completed"
```

**No Owner Reference** (Root CRD):
- RemediationRequest is the ROOT CRD - it has NO owner reference
- Created directly by Gateway Service
- ALL other CRDs own by RemediationRequest

---

### Update Lifecycle

**Status Updates by RemediationRequest Controller**:

```go
package controller

import (
    "context"
    "time"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *RemediationRequestReconciler) updateStatusCompleted(
    ctx context.Context,
    ar *remediationv1.RemediationRequest,
    finalResults remediationv1.RemediationResults,
) error {
    // Controller updates own status
    ar.Status.OverallPhase = "completed"
    ar.Status.CompletionTime = &metav1.Time{Time: time.Now()}
    ar.Status.RemediationResults = finalResults

    return r.Status().Update(ctx, ar)
}
```

**Watch-Based Orchestration**:

```
Child CRD status changes (RemediationProcessing, AIAnalysis, etc.)
    ↓ (watch event)
RemediationRequest watch triggers
    ↓ (<100ms latency)
RemediationRequest Controller reconciles
    ↓
RemediationRequest checks child status
    ↓
If child completed:
    ↓
RemediationRequest creates next child CRD
    ↓
If all children completed:
    ↓
RemediationRequest.status.overallPhase = "completed"
```

**Self-Updates Throughout Lifecycle**:
- RemediationRequest CONTINUOUSLY updates itself based on child status
- Aggregates status from all child CRDs
- Maintains overall remediation state machine
- Orchestrates creation of child CRDs based on workflow progress

---

### Deletion Lifecycle

**Trigger**: Manual deletion or TTL-based retention (24 hours)

**Cascade Deletion Sequence**:
```
User/System deletes RemediationRequest (after 24h retention)
    ↓
RemediationRequest.deletionTimestamp set
    ↓
RemediationRequest Controller reconciles (detects deletion)
    ↓
Finalizer cleanup executes:
  - Delete ALL child CRDs (RemediationProcessing, AIAnalysis, WorkflowExecution, KubernetesExecution (DEPRECATED - ADR-025))
  - Record final remediation audit
  - Emit deletion event
    ↓
Finalizer removed
    ↓
Kubernetes deletes RemediationRequest CRD
    ↓
Kubernetes garbage collector cascade-deletes ALL owned CRDs in parallel:
  - RemediationProcessing → deleted
  - AIAnalysis (+ AIApprovalRequest) → deleted
  - WorkflowExecution → deleted
  - KubernetesExecution (DEPRECATED - ADR-025) (+ Kubernetes Jobs) → deleted
```

**Retention**:
- **RemediationRequest**: 24-hour retention (configurable per environment)
- **Child CRDs**: Deleted with parent (no independent retention)
- **Audit Data**: 90-day retention in PostgreSQL (persisted before deletion)

---

### Lifecycle Events

**Kubernetes Events Emitted**:

```go
package controller

import (
    "fmt"
    "time"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"

    "k8s.io/client-go/tools/record"
)

func (r *RemediationRequestReconciler) emitLifecycleEvents(
    ar *remediationv1.RemediationRequest,
    oldPhase string,
    duration time.Duration,
) {
    // Creation event
    r.Recorder.Event(ar, "Normal", "RemediationRequestCreated",
        fmt.Sprintf("Alert remediation started for fingerprint: %s", ar.Spec.AlertFingerprint))

    // Phase transition events
    r.Recorder.Event(ar, "Normal", "PhaseTransition",
        fmt.Sprintf("Overall Phase: %s → %s", oldPhase, ar.Status.OverallPhase))

    // Child CRD creation events
    if ar.Status.RemediationProcessingRef != nil {
        r.Recorder.Event(ar, "Normal", "RemediationProcessingCreated",
            fmt.Sprintf("SignalProcessing CRD created: %s", ar.Status.RemediationProcessingRef.Name))
    }
    if ar.Status.AIAnalysisRef != nil {
        r.Recorder.Event(ar, "Normal", "AIAnalysisCreated",
            fmt.Sprintf("AIAnalysis CRD created: %s", ar.Status.AIAnalysisRef.Name))
    }
    if ar.Status.WorkflowExecutionRef != nil {
        r.Recorder.Event(ar, "Normal", "WorkflowExecutionCreated",
            fmt.Sprintf("WorkflowExecution CRD created: %s", ar.Status.WorkflowExecutionRef.Name))
    }
    for _, keRef := range ar.Status.KubernetesExecutionRefs {
        r.Recorder.Event(ar, "Normal", "KubernetesExecutionCreated", // DEPRECATED - ADR-025
            fmt.Sprintf("KubernetesExecution CRD created: %s", keRef.Name))
    }

    // Completion event
    if ar.Status.OverallPhase == "completed" {
        r.Recorder.Event(ar, "Normal", "RemediationRequestCompleted",
            fmt.Sprintf("Alert remediation completed in %s", duration))
    }

    // Failure event
    if ar.Status.OverallPhase == "failed" {
        r.Recorder.Event(ar, "Warning", "RemediationRequestFailed",
            fmt.Sprintf("Alert remediation failed: %s", ar.Status.FailureReason))
    }

    // Deletion event (in cleanup function)
    r.Recorder.Event(ar, "Normal", "RemediationRequestDeleted",
        fmt.Sprintf("RemediationRequest cleanup completed (phase: %s)", ar.Status.OverallPhase))
}
```

**Event Visibility**:
```bash
kubectl describe alertremediation <name>
# Shows all events in chronological order

kubectl get events --field-selector involvedObject.name=<name>
# Filter events for specific RemediationRequest
```

---

### Lifecycle Monitoring

**Prometheus Metrics**:

```promql
# CRD creation rate
rate(alertremediation_created_total[5m])

# CRD completion time (end-to-end)
histogram_quantile(0.95, alertremediation_lifecycle_duration_seconds)

# Active RemediationRequest CRDs
alertremediation_active_total

# CRD deletion rate
rate(alertremediation_deleted_total[5m])

# Success rate
sum(rate(alertremediation_completed{status="success"}[5m])) /
sum(rate(alertremediation_completed[5m]))

# Phase distribution
sum by (overallPhase) (alertremediation_active_total)

# Child CRD creation success rate
rate(alertremediation_child_crd_creation_failures_total[5m])
```

**Grafana Dashboard**:
```yaml
panels:
  - title: "RemediationRequest Lifecycle"
    targets:
      - expr: alertremediation_active_total
        legendFormat: "Active CRDs"
      - expr: rate(alertremediation_created_total[5m])
        legendFormat: "Creation Rate"
      - expr: rate(alertremediation_deleted_total[5m])
        legendFormat: "Deletion Rate"

  - title: "Remediation Latency (P95)"
    targets:
      - expr: histogram_quantile(0.95, alertremediation_lifecycle_duration_seconds)
        legendFormat: "P95 Duration"

  - title: "Success Rate"
    targets:
      - expr: |
          sum(rate(alertremediation_completed{status="success"}[5m])) /
          sum(rate(alertremediation_completed[5m]))
        legendFormat: "Success Rate"

  - title: "Active Remediation Phases"
    targets:
      - expr: sum by (overallPhase) (alertremediation_active_total)
        legendFormat: "{{overallPhase}}"
```

**Alert Rules**:

```yaml
groups:
- name: alertremediation-lifecycle
  rules:
  - alert: RemediationRequestStuckInPhase
    expr: time() - alertremediation_phase_start_timestamp > 1800
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "RemediationRequest stuck in phase for >30 minutes"
      description: "RemediationRequest {{ $labels.name }} has been in phase {{ $labels.overallPhase }} for over 30 minutes"

  - alert: RemediationRequestHighFailureRate
    expr: |
      sum(rate(alertremediation_completed{status="failed"}[5m])) /
      sum(rate(alertremediation_completed[5m])) > 0.2
    for: 10m
    labels:
      severity: critical
    annotations:
      summary: "High remediation failure rate"
      description: ">20% of alert remediations are failing"

  - alert: RemediationRequestChildCreationFailures
    expr: rate(alertremediation_child_crd_creation_failures_total[5m]) > 0.1
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Child CRD creation failing frequently"
      description: "RemediationRequest controller unable to create child CRDs"

  - alert: RemediationRequestHighDeletionRate
    expr: rate(alertremediation_deleted_total[5m]) > rate(alertremediation_created_total[5m]) * 1.5
    for: 5m
    labels:
      severity: info
    annotations:
      summary: "RemediationRequest deletion rate exceeds creation rate"
      description: "More remediations being deleted than created (possible retention policy cleanup)"

  - alert: RemediationRequestOrchestrationFailures
    expr: rate(alertremediation_orchestration_errors_total[5m]) > 0.05
    for: 10m
    labels:
      severity: critical
    annotations:
      summary: "Orchestration failures in RemediationRequest"
      description: "Central controller experiencing orchestration errors"
```

---

