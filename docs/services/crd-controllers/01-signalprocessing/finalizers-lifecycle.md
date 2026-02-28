## Finalizer Implementation

> **📋 Changelog**
> | Version | Date | Changes | Reference |
> |---------|------|---------|-----------|
> | v1.2 | 2025-11-28 | API group standardized to kubernaut.io/v1alpha1, graceful shutdown added | [DD-007](../../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md) |
> | v1.1 | 2025-11-27 | Service rename: RemediationProcessing → SignalProcessing | [DD-SIGNAL-PROCESSING-001](../../../architecture/decisions/DD-SIGNAL-PROCESSING-001-service-rename.md) |
> | v1.1 | 2025-11-27 | Data access via Data Storage Service REST API | [ADR-032](../../../architecture/decisions/ADR-032-data-access-layer-isolation.md) |
> | v1.0 | 2025-01-15 | Initial finalizer implementation | - |

### Finalizer Name

Following Kubernetes finalizer naming convention:

```go
const signalProcessingFinalizer = "signalprocessing.kubernaut.io/finalizer"
```

**Naming Pattern**: `{resource}.kubernaut.io/finalizer`

**Why This Pattern**:
- **Domain-Scoped**: `signalprocessing.kubernaut.io` prevents conflicts with other services
- **Resource-Specific**: Clearly indicates what's being cleaned up
- **Kubernetes Convention**: Follows standard finalizer naming (domain/action format)

---

### Complete Reconciliation Loop with Finalizer

```go
package controller

import (
    "context"
    "fmt"

    kubernautv1alpha1 "github.com/jordigilh/kubernaut/api/kubernaut.io/v1alpha1"

    "github.com/go-logr/logr"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const signalProcessingFinalizer = "signalprocessing.kubernaut.io/finalizer"

type SignalProcessingReconciler struct {
    client.Client
    Scheme            *runtime.Scheme
    Log               logr.Logger
    Recorder          record.EventRecorder
    EnrichmentService EnrichmentService
    DataStorageClient DataStorageClient  // REST API client (ADR-032)
}

func (r *SignalProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var sp signalprocessingv1.SignalProcessing
    if err := r.Get(ctx, req.NamespacedName, &sp); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // ========================================
    // DELETION HANDLING WITH FINALIZER
    // ========================================
    if !sp.ObjectMeta.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(&sp, signalProcessingFinalizer) {
            // Perform cleanup before deletion
            if err := r.cleanupSignalProcessing(ctx, &sp); err != nil {
                r.Log.Error(err, "Failed to cleanup SignalProcessing resources",
                    "name", sp.Name,
                    "namespace", sp.Namespace,
                )
                return ctrl.Result{}, err
            }

            // Remove finalizer to allow deletion
            controllerutil.RemoveFinalizer(&sp, signalProcessingFinalizer)
            if err := r.Update(ctx, &sp); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // ========================================
    // ADD FINALIZER IF NOT PRESENT
    // ========================================
    if !controllerutil.ContainsFinalizer(&sp, signalProcessingFinalizer) {
        controllerutil.AddFinalizer(&sp, signalProcessingFinalizer)
        if err := r.Update(ctx, &sp); err != nil {
            return ctrl.Result{}, err
        }
    }

    // ========================================
    // NORMAL RECONCILIATION LOGIC
    // ========================================

    // Skip if already completed or failed
    if sp.Status.Phase == "completed" || sp.Status.Phase == "failed" {
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

    kubernautv1alpha1 "github.com/jordigilh/kubernaut/api/kubernaut.io/v1alpha1"
)

func (r *SignalProcessingReconciler) cleanupSignalProcessing(
    ctx context.Context,
    sp *signalprocessingv1.SignalProcessing,
) error {
    r.Log.Info("Cleaning up SignalProcessing resources",
        "name", sp.Name,
        "namespace", sp.Namespace,
        "phase", sp.Status.Phase,
    )

    // 1. Record final audit to Data Storage Service (ADR-032)
    if err := r.recordFinalAudit(ctx, sp); err != nil {
        r.Log.Error(err, "Failed to record final audit", "name", sp.Name)
        // Don't block deletion on audit failure
        // Audit is best-effort during cleanup
    }

    // 2. Emit deletion event
    r.Recorder.Event(sp, "Normal", "SignalProcessingDeleted",
        fmt.Sprintf("SignalProcessing cleanup completed (phase: %s)", sp.Status.Phase))

    r.Log.Info("SignalProcessing cleanup completed successfully",
        "name", sp.Name,
        "namespace", sp.Namespace,
    )

    return nil
}

func (r *SignalProcessingReconciler) recordFinalAudit(
    ctx context.Context,
    sp *signalprocessingv1.SignalProcessing,
) error {
    auditRecord := &SignalProcessingAudit{
        SignalFingerprint: sp.Spec.Signal.Fingerprint,
        ServiceType:       "SignalProcessing",
        CRDName:           sp.Name,
        Namespace:         sp.Namespace,
        Phase:             sp.Status.Phase,
        CreatedAt:         sp.CreationTimestamp.Time,
        DeletedAt:         sp.DeletionTimestamp.Time,
        DegradedMode:      sp.Status.EnrichmentResults.EnrichmentQuality < 0.8,
    }

    // Send to Data Storage Service via REST API (ADR-032)
    return r.DataStorageClient.CreateAuditRecord(ctx, auditRecord)
}
```

**Cleanup Philosophy for SignalProcessing**:
- ✅ **Record final audit**: Capture that processing occurred (best-effort via Data Storage Service)
- ✅ **Emit deletion event**: Operational visibility
- ❌ **No external cleanup needed**: SignalProcessing is a leaf CRD (owns nothing)
- ❌ **No child CRD cleanup**: SignalProcessing doesn't create child CRDs
- ✅ **Non-blocking**: Audit failures don't block deletion (best-effort)

---

### Finalizer Testing

**Unit Test Pattern**:

```go
package controller_test

import (
    "context"
    "fmt"

    kubernautv1alpha1 "github.com/jordigilh/kubernaut/api/kubernaut.io/v1alpha1"
    "github.com/jordigilh/kubernaut/internal/controller/signalprocessing"

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

var _ = Describe("SignalProcessing Finalizer", func() {
    var (
        ctx       context.Context
        k8sClient client.Client
        reconciler *controller.SignalProcessingReconciler
    )

    BeforeEach(func() {
        ctx = context.Background()
        k8sClient = fake.NewClientBuilder().
            WithScheme(scheme.Scheme).
            Build()

        reconciler = &controller.SignalProcessingReconciler{
            Client:            k8sClient,
            DataStorageClient: &mockDataStorageClient{},
        }
    })

    Context("when SignalProcessing is created", func() {
        It("should add finalizer on first reconcile", func() {
            sp := &signalprocessingv1.SignalProcessing{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-processing",
                    Namespace: "default",
                },
                Spec: signalprocessingv1.SignalProcessingSpec{
                    Signal: signalprocessingv1.Signal{
                        Fingerprint: "abc123",
                    },
                },
            }
            Expect(k8sClient.Create(ctx, sp)).To(Succeed())

            // First reconcile should add finalizer
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(sp),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer added
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), sp)).To(Succeed())
            Expect(controllerutil.ContainsFinalizer(sp, signalProcessingFinalizer)).To(BeTrue())
        })
    })

    Context("when SignalProcessing is deleted", func() {
        It("should execute cleanup and remove finalizer", func() {
            sp := &signalprocessingv1.SignalProcessing{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-processing",
                    Namespace:  "default",
                    Finalizers: []string{signalProcessingFinalizer},
                },
                Spec: signalprocessingv1.SignalProcessingSpec{
                    Signal: signalprocessingv1.Signal{
                        Fingerprint: "abc123",
                    },
                },
                Status: signalprocessingv1.SignalProcessingStatus{
                    Phase: "completed",
                },
            }
            Expect(k8sClient.Create(ctx, sp)).To(Succeed())

            // Delete SignalProcessing
            Expect(k8sClient.Delete(ctx, sp)).To(Succeed())

            // Reconcile should execute cleanup
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(sp),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed (CRD will be deleted by Kubernetes)
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), sp)
            Expect(err).To(HaveOccurred())
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })

        It("should not block deletion if audit fails", func() {
            // Mock data storage client to return error
            reconciler.DataStorageClient = &mockDataStorageClient{
                createAuditError: fmt.Errorf("data storage unavailable"),
            }

            sp := &signalprocessingv1.SignalProcessing{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-processing",
                    Namespace:  "default",
                    Finalizers: []string{signalProcessingFinalizer},
                },
            }
            Expect(k8sClient.Create(ctx, sp)).To(Succeed())
            Expect(k8sClient.Delete(ctx, sp)).To(Succeed())

            // Cleanup should succeed even if audit fails
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(sp),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed despite audit failure
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), sp)
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
    ↓ (with owner reference, embeds failureData for recovery)
SignalProcessing Controller reconciles (this controller)
    ↓
SignalProcessing.status.phase = "completed"
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
    kubernautv1alpha1 "github.com/jordigilh/kubernaut/api/kubernaut.io/v1alpha1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// In RemediationRequestReconciler
func (r *RemediationRequestReconciler) createSignalProcessing(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
) error {
    signalProcessing := &signalprocessingv1.SignalProcessing{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-processing", remediation.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation,
                    remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: signalprocessingv1.SignalProcessingSpec{
            RemediationRequestRef: signalprocessingv1.RemediationRequestReference{
                Name:      remediation.Name,
                Namespace: remediation.Namespace,
            },
            Signal: signalprocessingv1.Signal{
                Fingerprint: remediation.Spec.SignalFingerprint,
                Payload:     remediation.Spec.OriginalPayload,
            },
        },
    }

    return r.Create(ctx, signalProcessing)
}
```

**Result**: SignalProcessing is owned by RemediationRequest (cascade deletion applies)

---

### Update Lifecycle

**Status Updates by SignalProcessing Controller**:

```go
package controller

import (
    "context"
    "time"

    kubernautv1alpha1 "github.com/jordigilh/kubernaut/api/kubernaut.io/v1alpha1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *SignalProcessingReconciler) updateStatusCompleted(
    ctx context.Context,
    sp *signalprocessingv1.SignalProcessing,
    enrichment signalprocessingv1.EnrichmentResults,
    classification signalprocessingv1.EnvironmentClassification,
    categorization signalprocessingv1.Categorization,
) error {
    // Controller updates own status
    sp.Status.Phase = "completed"
    sp.Status.ProcessingTime = time.Since(sp.CreationTimestamp.Time).String()
    sp.Status.EnrichmentResults = enrichment
    sp.Status.EnvironmentClassification = classification
    sp.Status.Categorization = categorization

    return r.Status().Update(ctx, sp)
}
```

**Watch Triggers RemediationRequest Reconciliation**:

```
SignalProcessing.status.phase = "completed"
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
- SignalProcessing does NOT update itself after `phase = "completed"`
- SignalProcessing does NOT create other CRDs (leaf controller)
- SignalProcessing does NOT watch other CRDs

---

### Deletion Lifecycle

**Trigger**: RemediationRequest deletion (cascade)

**Cascade Deletion Sequence**:
```
User/System deletes RemediationRequest
    ↓
Kubernetes garbage collector detects owner reference
    ↓ (parallel deletion of all owned CRDs)
SignalProcessing.deletionTimestamp set
    ↓
SignalProcessing Controller reconciles (detects deletion)
    ↓
Finalizer cleanup executes:
  - Record final audit to Data Storage Service
  - Emit deletion event
    ↓
Finalizer removed
    ↓
Kubernetes deletes SignalProcessing CRD
```

**Parallel Deletion**: All service CRDs (SignalProcessing, AIAnalysis, WorkflowExecution, KubernetesExecution (DEPRECATED - ADR-025)) deleted in parallel when RemediationRequest is deleted.

**Retention**:
- **SignalProcessing**: No independent retention (deleted with parent)
- **RemediationRequest**: 24-hour retention (parent CRD manages retention)
- **Audit Data**: 90-day retention in PostgreSQL (persisted via Data Storage Service before deletion)

---

### Lifecycle Events

**Kubernetes Events Emitted**:

```go
package controller

import (
    "fmt"
    "time"

    kubernautv1alpha1 "github.com/jordigilh/kubernaut/api/kubernaut.io/v1alpha1"

    "k8s.io/client-go/tools/record"
)

func (r *SignalProcessingReconciler) emitLifecycleEvents(
    sp *signalprocessingv1.SignalProcessing,
    oldPhase string,
    duration time.Duration,
) {
    // Creation event
    r.Recorder.Event(sp, "Normal", "SignalProcessingCreated",
        fmt.Sprintf("Signal processing started for %s", sp.Spec.Signal.Fingerprint))

    // Phase transition events
    r.Recorder.Event(sp, "Normal", "PhaseTransition",
        fmt.Sprintf("Phase: %s → %s", oldPhase, sp.Status.Phase))

    // Degraded mode event
    if sp.Status.EnrichmentResults.EnrichmentQuality < 0.8 {
        r.Recorder.Event(sp, "Warning", "DegradedMode",
            "Enrichment service unavailable, using minimal context from signal labels")
    }

    // Completion event
    r.Recorder.Event(sp, "Normal", "SignalProcessingCompleted",
        fmt.Sprintf("Signal processing completed in %s", duration))

    // Deletion event (in cleanup function)
    r.Recorder.Event(sp, "Normal", "SignalProcessingDeleted",
        fmt.Sprintf("SignalProcessing cleanup completed (phase: %s)", sp.Status.Phase))
}
```

**Event Visibility**:
```bash
kubectl describe signalprocessing <name>
# Shows all events in chronological order

kubectl get events --field-selector involvedObject.name=<name>
# Filter events for specific SignalProcessing
```

---

### Lifecycle Monitoring

**Prometheus Metrics**:

```promql
# CRD creation rate
rate(kubernaut_signal_processing_created_total[5m])

# CRD completion time (end-to-end)
histogram_quantile(0.95, kubernaut_signal_processing_lifecycle_duration_seconds)

# Active SignalProcessing CRDs
kubernaut_signal_processing_active_total

# CRD deletion rate
rate(kubernaut_signal_processing_deleted_total[5m])

# Degraded mode percentage
sum(kubernaut_signal_processing_active_total{degraded_mode="true"}) / sum(kubernaut_signal_processing_active_total)
```

**Grafana Dashboard**:
```yaml
panels:
  - title: "SignalProcessing Lifecycle"
    targets:
      - expr: kubernaut_signal_processing_active_total
        legendFormat: "Active CRDs"
      - expr: rate(kubernaut_signal_processing_created_total[5m])
        legendFormat: "Creation Rate"
      - expr: rate(kubernaut_signal_processing_deleted_total[5m])
        legendFormat: "Deletion Rate"

  - title: "Processing Latency (P95)"
    targets:
      - expr: histogram_quantile(0.95, kubernaut_signal_processing_lifecycle_duration_seconds)
        legendFormat: "P95 Duration"
```

**Alert Rules**:

```yaml
groups:
- name: signalprocessing-lifecycle
  rules:
  - alert: SignalProcessingStuckInPhase
    expr: time() - kubernaut_signal_processing_phase_start_timestamp > 600
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "SignalProcessing stuck in phase for >10 minutes"
      description: "SignalProcessing {{ $labels.name }} has been in phase {{ $labels.phase }} for over 10 minutes"

  - alert: SignalProcessingHighDeletionRate
    expr: rate(kubernaut_signal_processing_deleted_total[5m]) > rate(kubernaut_signal_processing_created_total[5m]) * 1.5
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "SignalProcessing deletion rate exceeds creation rate"
      description: "More SignalProcessing CRDs being deleted than created (possible cascade deletion issue)"

  - alert: SignalProcessingHighDegradedMode
    expr: sum(kubernaut_signal_processing_active_total{degraded_mode="true"}) / sum(kubernaut_signal_processing_active_total) > 0.5
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: ">50% of SignalProcessing CRDs in degraded mode"
      description: "Enrichment service may be unavailable"
```

---
