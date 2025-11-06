## Finalizer Implementation

### Finalizer Name

Following Kubernetes finalizer naming convention:

```go
const alertProcessingFinalizer = "signalprocessing.kubernaut.io/alertprocessing-cleanup"
```

**Naming Pattern**: `{domain}.kubernaut.io/{resource}-cleanup`

**Why This Pattern**:
- **Domain-Scoped**: `signalprocessing.kubernaut.io` prevents conflicts with other services
- **Resource-Specific**: `alertprocessing-cleanup` clearly indicates what's being cleaned up
- **Kubernetes Convention**: Follows standard finalizer naming (domain/action format)

---

### Complete Reconciliation Loop with Finalizer

```go
package controller

import (
    "context"
    "fmt"

    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"

    "github.com/go-logr/logr"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const alertProcessingFinalizer = "signalprocessing.kubernaut.io/alertprocessing-cleanup"

type RemediationProcessingReconciler struct {
    client.Client
    Scheme            *runtime.Scheme
    Log               logr.Logger
    Recorder          record.EventRecorder
    ContextClient     ContextClient
    StorageClient     StorageClient
}

func (r *RemediationProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var ap processingv1.RemediationProcessing
    if err := r.Get(ctx, req.NamespacedName, &ap); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // ========================================
    // DELETION HANDLING WITH FINALIZER
    // ========================================
    if !ap.ObjectMeta.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(&ap, alertProcessingFinalizer) {
            // Perform cleanup before deletion
            if err := r.cleanupRemediationProcessing(ctx, &ap); err != nil {
                r.Log.Error(err, "Failed to cleanup RemediationProcessing resources",
                    "name", ap.Name,
                    "namespace", ap.Namespace,
                )
                return ctrl.Result{}, err
            }

            // Remove finalizer to allow deletion
            controllerutil.RemoveFinalizer(&ap, alertProcessingFinalizer)
            if err := r.Update(ctx, &ap); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // ========================================
    // ADD FINALIZER IF NOT PRESENT
    // ========================================
    if !controllerutil.ContainsFinalizer(&ap, alertProcessingFinalizer) {
        controllerutil.AddFinalizer(&ap, alertProcessingFinalizer)
        if err := r.Update(ctx, &ap); err != nil {
            return ctrl.Result{}, err
        }
    }

    // ========================================
    // NORMAL RECONCILIATION LOGIC
    // ========================================

    // Skip if already completed or failed
    if ap.Status.Phase == "completed" || ap.Status.Phase == "failed" {
        return ctrl.Result{}, nil
    }

    // Execute processing...
    // (existing reconciliation logic from previous section)

    return ctrl.Result{}, nil
}
```

---

### Cleanup Logic

**What Gets Cleaned Up**:

```go
package controller

import (
    "context"
    "fmt"
    "time"

    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
)

func (r *RemediationProcessingReconciler) cleanupRemediationProcessing(
    ctx context.Context,
    ap *processingv1.RemediationProcessing,
) error {
    r.Log.Info("Cleaning up RemediationProcessing resources",
        "name", ap.Name,
        "namespace", ap.Namespace,
        "phase", ap.Status.Phase,
    )

    // 1. Record final audit to database
    if err := r.recordFinalAudit(ctx, ap); err != nil {
        r.Log.Error(err, "Failed to record final audit", "name", ap.Name)
        // Don't block deletion on audit failure
        // Audit is best-effort during cleanup
    }

    // 2. Emit deletion event
    r.Recorder.Event(ap, "Normal", "RemediationProcessingDeleted",
        fmt.Sprintf("RemediationProcessing cleanup completed (phase: %s)", ap.Status.Phase))

    r.Log.Info("RemediationProcessing cleanup completed successfully",
        "name", ap.Name,
        "namespace", ap.Namespace,
    )

    return nil
}

func (r *RemediationProcessingReconciler) recordFinalAudit(
    ctx context.Context,
    ap *processingv1.RemediationProcessing,
) error {
    auditRecord := &AuditRecord{
        AlertFingerprint: ap.Spec.Signal.Fingerprint,
        ServiceType:      "RemediationProcessing",
        CRDName:          ap.Name,
        Namespace:        ap.Namespace,
        Phase:            ap.Status.Phase,
        CreatedAt:        ap.CreationTimestamp.Time,
        DeletedAt:        ap.DeletionTimestamp.Time,
        DegradedMode:     ap.Status.DegradedMode,
    }

    return r.StorageClient.RecordAudit(ctx, auditRecord)
}
```

**Cleanup Philosophy for RemediationProcessing**:
- ✅ **Record final audit**: Capture that processing occurred (best-effort)
- ✅ **Emit deletion event**: Operational visibility
- ❌ **No external cleanup needed**: RemediationProcessing is a leaf CRD (owns nothing)
- ❌ **No child CRD cleanup**: RemediationProcessing doesn't create child CRDs
- ✅ **Non-blocking**: Audit failures don't block deletion (best-effort)

---

### Finalizer Testing

**Unit Test Pattern**:

```go
package controller_test

import (
    "context"
    "fmt"

    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
    "github.com/jordigilh/kubernaut/pkg/remediationprocessing/controller"

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

var _ = Describe("RemediationProcessing Finalizer", func() {
    var (
        ctx       context.Context
        k8sClient client.Client
        reconciler *controller.RemediationProcessingReconciler
    )

    BeforeEach(func() {
        ctx = context.Background()
        k8sClient = fake.NewClientBuilder().
            WithScheme(scheme.Scheme).
            Build()

        reconciler = &controller.RemediationProcessingReconciler{
            Client:        k8sClient,
            StorageClient: &mockStorageClient{},
        }
    })

    Context("when RemediationProcessing is created", func() {
        It("should add finalizer on first reconcile", func() {
            ap := &processingv1.RemediationProcessing{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-processing",
                    Namespace: "default",
                },
                Spec: processingv1.RemediationProcessingSpec{
                    Alert: processingv1.Alert{
                        Fingerprint: "abc123",
                    },
                },
            }
            Expect(k8sClient.Create(ctx, ap)).To(Succeed())

            // First reconcile should add finalizer
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(ap),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer added
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ap), ap)).To(Succeed())
            Expect(controllerutil.ContainsFinalizer(ap, alertProcessingFinalizer)).To(BeTrue())
        })
    })

    Context("when RemediationProcessing is deleted", func() {
        It("should execute cleanup and remove finalizer", func() {
            ap := &processingv1.RemediationProcessing{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-processing",
                    Namespace:  "default",
                    Finalizers: []string{alertProcessingFinalizer},
                },
                Spec: processingv1.RemediationProcessingSpec{
                    Alert: processingv1.Alert{
                        Fingerprint: "abc123",
                    },
                },
                Status: processingv1.RemediationProcessingStatus{
                    Phase: "completed",
                },
            }
            Expect(k8sClient.Create(ctx, ap)).To(Succeed())

            // Delete RemediationProcessing
            Expect(k8sClient.Delete(ctx, ap)).To(Succeed())

            // Reconcile should execute cleanup
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(ap),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed (CRD will be deleted by Kubernetes)
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(ap), ap)
            Expect(err).To(HaveOccurred())
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })

        It("should not block deletion if audit fails", func() {
            // Mock storage client to return error
            reconciler.StorageClient = &mockStorageClient{
                recordAuditError: fmt.Errorf("database unavailable"),
            }

            ap := &processingv1.RemediationProcessing{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-processing",
                    Namespace:  "default",
                    Finalizers: []string{alertProcessingFinalizer},
                },
            }
            Expect(k8sClient.Create(ctx, ap)).To(Succeed())
            Expect(k8sClient.Delete(ctx, ap)).To(Succeed())

            // Cleanup should succeed even if audit fails
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(ap),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed despite audit failure
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(ap), ap)
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })
    })
})
```

---

## CRD Lifecycle Management

### Creation Lifecycle

**Created By**: RemediationRequest controller (centralized orchestration)

**Creation Trigger**: RemediationRequest transitions to `pending` phase

**Sequence**:
```
Gateway Service creates RemediationRequest CRD
    ↓
RemediationRequest.status.overallPhase = "pending"
    ↓
RemediationRequest Controller reconciles
    ↓
RemediationRequest Controller creates SignalProcessing CRD
    ↓ (with owner reference)
RemediationProcessing Controller reconciles (this controller)
    ↓
RemediationProcessing.status.phase = "completed"
    ↓ (watch trigger <100ms)
RemediationRequest Controller detects completion
    ↓
RemediationRequest Controller creates AIAnalysis CRD
```

**Owner Reference Set at Creation**:
```go
package controller

import (
    "context"
    "fmt"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// In RemediationRequestReconciler
func (r *RemediationRequestReconciler) createRemediationProcessing(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
) error {
    alertProcessing := &processingv1.RemediationProcessing{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-processing", remediation.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation,
                    remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: processingv1.RemediationProcessingSpec{
            RemediationRequestRef: processingv1.RemediationRequestReference{
                Name:      remediation.Name,
                Namespace: remediation.Namespace,
            },
            Alert: processingv1.Alert{
                Fingerprint: remediation.Spec.SignalFingerprint,
                Payload:     remediation.Spec.OriginalPayload,
            },
        },
    }

    return r.Create(ctx, alertProcessing)
}
```

**Result**: RemediationProcessing is owned by RemediationRequest (cascade deletion applies)

---

### Update Lifecycle

**Status Updates by RemediationProcessing Controller**:

```go
package controller

import (
    "context"
    "time"

    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *RemediationProcessingReconciler) updateStatusCompleted(
    ctx context.Context,
    ap *processingv1.RemediationProcessing,
    enriched processingv1.EnrichedAlert,
    classification string,
) error {
    // Controller updates own status
    ap.Status.Phase = "completed"
    ap.Status.CompletionTime = &metav1.Time{Time: time.Now()}
    ap.Status.EnrichedAlert = enriched
    ap.Status.EnvironmentClassification = classification

    return r.Status().Update(ctx, ap)
}
```

**Watch Triggers RemediationRequest Reconciliation**:

```
RemediationProcessing.status.phase = "completed"
    ↓ (watch event)
RemediationRequest watch triggers
    ↓ (<100ms latency)
RemediationRequest Controller reconciles
    ↓
RemediationRequest extracts enriched data
    ↓
RemediationRequest creates AIAnalysis CRD
```

**No Self-Updates After Completion**:
- RemediationProcessing does NOT update itself after `phase = "completed"`
- RemediationProcessing does NOT create other CRDs (leaf controller)
- RemediationProcessing does NOT watch other CRDs

---

### Deletion Lifecycle

**Trigger**: RemediationRequest deletion (cascade)

**Cascade Deletion Sequence**:
```
User/System deletes RemediationRequest
    ↓
Kubernetes garbage collector detects owner reference
    ↓ (parallel deletion of all owned CRDs)
RemediationProcessing.deletionTimestamp set
    ↓
RemediationProcessing Controller reconciles (detects deletion)
    ↓
Finalizer cleanup executes:
  - Record final audit
  - Emit deletion event
    ↓
Finalizer removed
    ↓
Kubernetes deletes SignalProcessing CRD
```

**Parallel Deletion**: All service CRDs (RemediationProcessing, AIAnalysis, WorkflowExecution, KubernetesExecution) deleted in parallel when RemediationRequest is deleted.

**Retention**:
- **RemediationProcessing**: No independent retention (deleted with parent)
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

    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"

    "k8s.io/client-go/tools/record"
)

func (r *RemediationProcessingReconciler) emitLifecycleEvents(
    ap *processingv1.RemediationProcessing,
    oldPhase string,
    duration time.Duration,
) {
    // Creation event
    r.Recorder.Event(ap, "Normal", "RemediationProcessingCreated",
        fmt.Sprintf("Alert processing started for %s", ap.Spec.Signal.Fingerprint))

    // Phase transition events
    r.Recorder.Event(ap, "Normal", "PhaseTransition",
        fmt.Sprintf("Phase: %s → %s", oldPhase, ap.Status.Phase))

    // Degraded mode event
    if ap.Status.DegradedMode {
        r.Recorder.Event(ap, "Warning", "DegradedMode",
            "Context Service unavailable, using minimal context from alert labels")
    }

    // Completion event
    r.Recorder.Event(ap, "Normal", "RemediationProcessingCompleted",
        fmt.Sprintf("Alert processing completed in %s", duration))

    // Deletion event (in cleanup function)
    r.Recorder.Event(ap, "Normal", "RemediationProcessingDeleted",
        fmt.Sprintf("RemediationProcessing cleanup completed (phase: %s)", ap.Status.Phase))
}
```

**Event Visibility**:
```bash
kubectl describe alertprocessing <name>
# Shows all events in chronological order

kubectl get events --field-selector involvedObject.name=<name>
# Filter events for specific RemediationProcessing
```

---

### Lifecycle Monitoring

**Prometheus Metrics**:

```promql
# CRD creation rate
rate(alertprocessing_created_total[5m])

# CRD completion time (end-to-end)
histogram_quantile(0.95, alertprocessing_lifecycle_duration_seconds)

# Active SignalProcessing CRDs
alertprocessing_active_total

# CRD deletion rate
rate(alertprocessing_deleted_total[5m])

# Degraded mode percentage
sum(alertprocessing_active_total{degraded_mode="true"}) / sum(alertprocessing_active_total)
```

**Grafana Dashboard**:
```yaml
panels:
  - title: "RemediationProcessing Lifecycle"
    targets:
      - expr: alertprocessing_active_total
        legendFormat: "Active CRDs"
      - expr: rate(alertprocessing_created_total[5m])
        legendFormat: "Creation Rate"
      - expr: rate(alertprocessing_deleted_total[5m])
        legendFormat: "Deletion Rate"

  - title: "Processing Latency (P95)"
    targets:
      - expr: histogram_quantile(0.95, alertprocessing_lifecycle_duration_seconds)
        legendFormat: "P95 Duration"
```

**Alert Rules**:

```yaml
groups:
- name: alertprocessing-lifecycle
  rules:
  - alert: RemediationProcessingStuckInPhase
    expr: time() - alertprocessing_phase_start_timestamp > 600
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "RemediationProcessing stuck in phase for >10 minutes"
      description: "RemediationProcessing {{ $labels.name }} has been in phase {{ $labels.phase }} for over 10 minutes"

  - alert: RemediationProcessingHighDeletionRate
    expr: rate(alertprocessing_deleted_total[5m]) > rate(alertprocessing_created_total[5m]) * 1.5
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "RemediationProcessing deletion rate exceeds creation rate"
      description: "More SignalProcessing CRDs being deleted than created (possible cascade deletion issue)"

  - alert: RemediationProcessingHighDegradedMode
    expr: sum(alertprocessing_active_total{degraded_mode="true"}) / sum(alertprocessing_active_total) > 0.5
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: ">50% of SignalProcessing CRDs in degraded mode"
      description: "Context Service may be unavailable"
```

---

