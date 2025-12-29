# Finalizers & Lifecycle Management

**Version**: v2.0
**Status**: ✅ Complete - V1.0 Aligned
**Last Updated**: 2025-11-30

---

## 📋 Changelog

| Version | Date | Changes |
|---------|------|---------|
| v2.0 | 2025-11-30 | **V1.0 ALIGNMENT**: Removed AIApprovalRequest references (V1.1+); Updated to SignalProcessing naming; Aligned with 4-phase flow; Updated import paths |
| v1.0 | 2025-10-15 | Initial specification |

---

## Finalizer Implementation

### Finalizer Name

Following Kubernetes finalizer naming convention:

```go
const aiAnalysisFinalizer = "kubernaut.ai/cleanup"
```

**Naming Pattern**: `{api-group}/{resource}-cleanup`

**Why This Pattern**:
- **Domain-Scoped**: `aianalysis.kubernaut.io` matches CRD API group
- **Resource-Specific**: `cleanup` clearly indicates cleanup action
- **Kubernetes Convention**: Follows standard finalizer naming (domain/action format)

---

### Complete Reconciliation Loop with Finalizer

```go
package controller

import (
    "context"
    "fmt"

    aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"

    "github.com/go-logr/logr"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const aiAnalysisFinalizer = "kubernaut.ai/cleanup"

type AIAnalysisReconciler struct {
    client.Client
    Scheme            *runtime.Scheme
    Log               logr.Logger
    Recorder          record.EventRecorder
    HolmesGPTClient   HolmesGPTClient
    StorageClient     StorageClient
}

func (r *AIAnalysisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var analysis aianalysisv1alpha1.AIAnalysis
    if err := r.Get(ctx, req.NamespacedName, &analysis); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // ========================================
    // DELETION HANDLING WITH FINALIZER
    // ========================================
    if !analysis.ObjectMeta.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(&analysis, aiAnalysisFinalizer) {
            // Perform cleanup before deletion
            if err := r.cleanupAIAnalysis(ctx, &analysis); err != nil {
                r.Log.Error(err, "Failed to cleanup AIAnalysis resources",
                    "name", analysis.Name,
                    "namespace", analysis.Namespace,
                )
                return ctrl.Result{}, err
            }

            // Remove finalizer to allow deletion
            controllerutil.RemoveFinalizer(&analysis, aiAnalysisFinalizer)
            if err := r.Update(ctx, &analysis); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // ========================================
    // ADD FINALIZER IF NOT PRESENT
    // ========================================
    if !controllerutil.ContainsFinalizer(&analysis, aiAnalysisFinalizer) {
        controllerutil.AddFinalizer(&analysis, aiAnalysisFinalizer)
        if err := r.Update(ctx, &analysis); err != nil {
            return ctrl.Result{}, err
        }
    }

    // ========================================
    // NORMAL RECONCILIATION LOGIC
    // ========================================

    // Skip if already completed or failed (terminal states)
    if analysis.Status.Phase == aianalysisv1alpha1.PhaseReady ||
       analysis.Status.Phase == aianalysisv1alpha1.PhaseFailed {
        return ctrl.Result{}, nil
    }

    // Execute AI analysis reconciliation...
    // (reconciliation logic in controller-implementation.md)

    return ctrl.Result{}, nil
}
```

---

### Cleanup Logic

**What Gets Cleaned Up** (V1.0 - No Child CRDs):

```go
package controller

import (
    "context"
    "fmt"
    "time"

    aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *AIAnalysisReconciler) cleanupAIAnalysis(
    ctx context.Context,
    analysis *aianalysisv1alpha1.AIAnalysis,
) error {
    r.Log.Info("Cleaning up AIAnalysis resources",
        "name", analysis.Name,
        "namespace", analysis.Namespace,
        "phase", analysis.Status.Phase,
    )

    // V1.0: AIAnalysis does NOT create child CRDs
    // - AIApprovalRequest is V1.1+ (not in V1.0)
    // - WorkflowExecution is created by RO, not AIAnalysis

    // 1. Record final audit to database (best-effort)
    if err := r.recordFinalAudit(ctx, analysis); err != nil {
        r.Log.Error(err, "Failed to record final audit", "name", analysis.Name)
        // Don't block deletion on audit failure
    }

    // 2. Emit deletion event
    r.Recorder.Event(analysis, "Normal", "AIAnalysisDeleted",
        fmt.Sprintf("AIAnalysis cleanup completed (phase: %s)", analysis.Status.Phase))

    r.Log.Info("AIAnalysis cleanup completed successfully",
        "name", analysis.Name,
        "namespace", analysis.Namespace,
    )

    return nil
}

func (r *AIAnalysisReconciler) recordFinalAudit(
    ctx context.Context,
    analysis *aianalysisv1alpha1.AIAnalysis,
) error {
    auditRecord := &AuditRecord{
        AlertFingerprint: analysis.Spec.AnalysisRequest.SignalContext.Fingerprint,
        ServiceType:      "AIAnalysis",
        CRDName:          analysis.Name,
        Namespace:        analysis.Namespace,
        Phase:            string(analysis.Status.Phase),
        CreatedAt:        analysis.CreationTimestamp.Time,
        DeletedAt:        analysis.DeletionTimestamp.Time,
        ApprovalRequired: analysis.Status.ApprovalRequired,
        WorkflowID:       analysis.Status.SelectedWorkflow.WorkflowID,
    }

    return r.StorageClient.RecordAudit(ctx, auditRecord)
}
```

**V1.0 Cleanup Philosophy**:
- ✅ **Record final audit**: Capture analysis outcomes (best-effort)
- ✅ **Emit deletion event**: Operational visibility
- ✅ **Non-blocking**: Audit failures don't block deletion
- ❌ **No child CRD cleanup**: AIAnalysis doesn't create child CRDs in V1.0

**V1.1+ Additions** (Future):
- AIApprovalRequest CRD cleanup (when approval orchestration via CRD is implemented)

---

### Finalizer Testing

**Unit Test Pattern**:

```go
package controller_test

import (
    "context"

    aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/internal/controller/aianalysis"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/types"
    "k8s.io/client-go/kubernetes/scheme"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const aiAnalysisFinalizer = "kubernaut.ai/cleanup"

var _ = Describe("AIAnalysis Finalizer", func() {
    var (
        ctx       context.Context
        k8sClient client.Client
        reconciler *aianalysis.AIAnalysisReconciler
    )

    BeforeEach(func() {
        ctx = context.Background()
        k8sClient = fake.NewClientBuilder().
            WithScheme(scheme.Scheme).
            Build()

        reconciler = &aianalysis.AIAnalysisReconciler{
            Client:          k8sClient,
            HolmesGPTClient: &mockHolmesGPTClient{},
            StorageClient:   &mockStorageClient{},
        }
    })

    Context("when AIAnalysis is created", func() {
        It("should add finalizer on first reconcile", func() {
            analysis := &aianalysisv1alpha1.AIAnalysis{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-analysis",
                    Namespace: "default",
                },
                Spec: aianalysisv1alpha1.AIAnalysisSpec{
                    RemediationID: "rem-123",
                    AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
                        SignalContext: aianalysisv1alpha1.SignalContextInput{
                            Fingerprint: "abc123",
                        },
                    },
                },
            }
            Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

            // First reconcile should add finalizer
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(analysis),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer added
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
            Expect(controllerutil.ContainsFinalizer(analysis, aiAnalysisFinalizer)).To(BeTrue())
        })
    })

    Context("when AIAnalysis is deleted", func() {
        It("should record audit and remove finalizer", func() {
            analysis := &aianalysisv1alpha1.AIAnalysis{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-analysis",
                    Namespace:  "default",
                    UID:        types.UID("analysis-uid-123"),
                    Finalizers: []string{aiAnalysisFinalizer},
                },
                Status: aianalysisv1alpha1.AIAnalysisStatus{
                    Phase: aianalysisv1alpha1.PhaseReady,
                },
            }
            Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

            // Delete AIAnalysis
            Expect(k8sClient.Delete(ctx, analysis)).To(Succeed())

            // Reconcile should execute cleanup
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(analysis),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed (CRD will be deleted by Kubernetes)
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })

        It("should not block deletion if audit fails", func() {
            // Mock storage client to return error
            reconciler.StorageClient = &mockStorageClient{
                recordAuditError: fmt.Errorf("database unavailable"),
            }

            analysis := &aianalysisv1alpha1.AIAnalysis{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-analysis",
                    Namespace:  "default",
                    Finalizers: []string{aiAnalysisFinalizer},
                },
            }
            Expect(k8sClient.Create(ctx, analysis)).To(Succeed())
            Expect(k8sClient.Delete(ctx, analysis)).To(Succeed())

            // Cleanup should succeed even if audit fails
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(analysis),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed despite audit failure
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })
    })
})
```

---

## CRD Lifecycle Management

### Creation Lifecycle

**Created By**: Remediation Orchestrator (RO)

**Creation Trigger**: SignalProcessing completion

**V1.0 Sequence**:
```
SignalProcessing.status.phase = "Completed"
    ↓ (watch trigger <100ms)
Remediation Orchestrator reconciles
    ↓
RO extracts EnrichmentResults from SignalProcessing status
RO copies DetectedLabels, CustomLabels, OwnerChain
    ↓
RO creates AIAnalysis CRD
    ↓ (with owner reference to RemediationRequest)
AIAnalysis Controller reconciles (this controller)
    ↓
AIAnalysis calls HolmesGPT-API for investigation
    ↓
HolmesGPT-API returns: RCA + Workflow Selection
    ↓
AIAnalysis evaluates Rego approval policies
    ↓
AIAnalysis.status.phase = "Ready"
AIAnalysis.status.approvalRequired = true/false
    ↓ (watch trigger <100ms)
Remediation Orchestrator detects completion
    ↓
If approvalRequired = true:
    → RO sends notification, STOPS
If approvalRequired = false:
    → RO creates WorkflowExecution CRD
```

**Owner Reference Set at Creation** (by RO):
```go
// In Remediation Orchestrator
func (r *RemediationOrchestratorReconciler) createAIAnalysis(
    ctx context.Context,
    remediation *remediationv1alpha1.RemediationRequest,
    signalProcessing *signalprocessingv1alpha1.SignalProcessing,
) error {
    analysis := &aianalysisv1alpha1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-analysis", remediation.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation,
                    remediationv1alpha1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: aianalysisv1alpha1.AIAnalysisSpec{
            RemediationRequestRef: corev1.ObjectReference{
                Name:      remediation.Name,
                Namespace: remediation.Namespace,
            },
            RemediationID: remediation.Status.RemediationID,
            AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
                SignalContext: buildSignalContext(signalProcessing),
                AnalysisTypes: []string{"root_cause", "workflow_selection"},
            },
            // Recovery fields (if applicable)
            IsRecoveryAttempt:  remediation.Spec.IsRecovery,
            PreviousExecutions: remediation.Spec.PreviousExecutions,
        },
    }

    return r.Create(ctx, analysis)
}
```

**Result**: AIAnalysis is owned by RemediationRequest (cascade deletion applies)

---

### Update Lifecycle

**Status Updates by AIAnalysis Controller**:

```go
package controller

import (
    "context"
    "time"

    aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *AIAnalysisReconciler) updateStatusInvestigating(
    ctx context.Context,
    analysis *aianalysisv1alpha1.AIAnalysis,
) error {
    analysis.Status.Phase = aianalysisv1alpha1.PhaseInvestigating
    analysis.Status.LastUpdated = &metav1.Time{Time: time.Now()}

    return r.Status().Update(ctx, analysis)
}

func (r *AIAnalysisReconciler) updateStatusReady(
    ctx context.Context,
    analysis *aianalysisv1alpha1.AIAnalysis,
    result *HolmesGPTResult,
    approvalRequired bool,
) error {
    analysis.Status.Phase = aianalysisv1alpha1.PhaseReady
    analysis.Status.CompletionTime = &metav1.Time{Time: time.Now()}
    analysis.Status.ApprovalRequired = approvalRequired
    analysis.Status.SelectedWorkflow = result.SelectedWorkflow
    analysis.Status.RootCauseAnalysis = result.RCA

    return r.Status().Update(ctx, analysis)
}
```

**Watch Triggers RO Reconciliation**:
```
AIAnalysis.status.phase = "Ready"
    ↓ (watch event)
Remediation Orchestrator watch triggers
    ↓ (<100ms latency)
RO reconciles
    ↓
RO checks approvalRequired flag
    ↓
If true: RO sends notification → STOP
If false: RO creates WorkflowExecution CRD
```

---

### Deletion Lifecycle

**Trigger**: RemediationRequest deletion (cascade)

**V1.0 Cascade Deletion Sequence**:
```
User/System deletes RemediationRequest
    ↓
Kubernetes garbage collector detects owner reference
    ↓ (parallel deletion of all owned CRDs)
AIAnalysis.deletionTimestamp set
    ↓
AIAnalysis Controller reconciles (detects deletion)
    ↓
Finalizer cleanup executes:
  - Record final audit (best-effort)
  - Emit deletion event
    ↓
Finalizer removed
    ↓
Kubernetes deletes AIAnalysis CRD
```

**V1.0 Cascade Pattern** (Single-Layer):
- **Layer 1**: RemediationRequest → AIAnalysis (parent-child)
- **Layer 1**: RemediationRequest → SignalProcessing (parent-child)
- **Layer 1**: RemediationRequest → WorkflowExecution (parent-child)

**V1.1+ Cascade Pattern** (Two-Layer for AIAnalysis):
- **Layer 1**: RemediationRequest → AIAnalysis
- **Layer 2**: AIAnalysis → AIApprovalRequest (when implemented)

**Retention**:
- **AIAnalysis**: No independent retention (deleted with parent)
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

    aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"

    "k8s.io/client-go/tools/record"
)

func (r *AIAnalysisReconciler) emitLifecycleEvents(
    analysis *aianalysisv1alpha1.AIAnalysis,
    oldPhase aianalysisv1alpha1.AIAnalysisPhase,
    duration time.Duration,
) {
    // Creation event
    r.Recorder.Event(analysis, "Normal", "AIAnalysisCreated",
        fmt.Sprintf("AI analysis started for signal %s",
            analysis.Spec.AnalysisRequest.SignalContext.Fingerprint))

    // Phase transition events
    r.Recorder.Event(analysis, "Normal", "PhaseTransition",
        fmt.Sprintf("Phase: %s → %s", oldPhase, analysis.Status.Phase))

    // HolmesGPT investigation events
    if analysis.Status.Phase == aianalysisv1alpha1.PhaseReady {
        r.Recorder.Event(analysis, "Normal", "InvestigationComplete",
            fmt.Sprintf("Selected workflow: %s (confidence: %.2f)",
                analysis.Status.SelectedWorkflow.WorkflowID,
                analysis.Status.SelectedWorkflow.Confidence))
    }

    // Approval signaling (V1.0)
    if analysis.Status.ApprovalRequired {
        r.Recorder.Event(analysis, "Normal", "ApprovalRequired",
            "Manual approval required - RO will send notification")
    }

    // Completion event
    r.Recorder.Event(analysis, "Normal", "AIAnalysisCompleted",
        fmt.Sprintf("AI analysis completed in %s", duration))

    // Deletion event (in cleanup function)
    r.Recorder.Event(analysis, "Normal", "AIAnalysisDeleted",
        fmt.Sprintf("AIAnalysis cleanup completed (phase: %s)", analysis.Status.Phase))
}
```

**Event Visibility**:
```bash
kubectl describe aianalysis <name>
# Shows all events in chronological order

kubectl get events --field-selector involvedObject.name=<name>
# Filter events for specific AIAnalysis
```

---

### Lifecycle Monitoring

**Prometheus Metrics**:

```promql
# CRD creation rate
rate(aianalysis_created_total[5m])

# CRD completion time (end-to-end)
histogram_quantile(0.95, aianalysis_lifecycle_duration_seconds)

# Active AIAnalysis CRDs
aianalysis_active_total

# CRD deletion rate
rate(aianalysis_deleted_total[5m])

# V1.0: Approval signaling rate
rate(aianalysis_approval_required_total[5m])

# HolmesGPT investigation failures
rate(aianalysis_holmesgpt_failures_total[5m])
```

**Grafana Dashboard**:
```yaml
panels:
  - title: "AIAnalysis Lifecycle"
    targets:
      - expr: aianalysis_active_total
        legendFormat: "Active CRDs"
      - expr: rate(aianalysis_created_total[5m])
        legendFormat: "Creation Rate"
      - expr: rate(aianalysis_deleted_total[5m])
        legendFormat: "Deletion Rate"

  - title: "Analysis Latency (P95)"
    targets:
      - expr: histogram_quantile(0.95, aianalysis_lifecycle_duration_seconds)
        legendFormat: "P95 Duration"

  - title: "Approval Signaling (V1.0)"
    targets:
      - expr: sum(aianalysis_approval_required_total{approval_required="true"})
        legendFormat: "Approval Required"
      - expr: sum(aianalysis_approval_required_total{approval_required="false"})
        legendFormat: "Auto-Approved"
```

**Alert Rules**:

```yaml
groups:
- name: aianalysis-lifecycle
  rules:
  - alert: AIAnalysisStuckInvestigating
    expr: time() - aianalysis_phase_start_timestamp{phase="Investigating"} > 300
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "AIAnalysis stuck investigating for >5 minutes"
      description: "AIAnalysis {{ $labels.name }} has been investigating for over 5 minutes"

  - alert: AIAnalysisHighFailureRate
    expr: rate(aianalysis_holmesgpt_failures_total[5m]) > 0.1
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "High HolmesGPT investigation failure rate"
      description: "HolmesGPT investigation failing for >10% of requests"

  - alert: AIAnalysisHighApprovalRate
    expr: |
      sum(rate(aianalysis_approval_required_total{approval_required="true"}[5m])) /
      sum(rate(aianalysis_approval_required_total[5m])) > 0.8
    for: 15m
    labels:
      severity: warning
    annotations:
      summary: ">80% of AI analyses require manual approval"
      description: "High approval rate may indicate Rego policies are too strict"

  - alert: AIAnalysisHighDeletionRate
    expr: rate(aianalysis_deleted_total[5m]) > rate(aianalysis_created_total[5m]) * 1.5
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "AIAnalysis deletion rate exceeds creation rate"
      description: "More AIAnalysis CRDs being deleted than created (possible cascade deletion issue)"
```

---

## References

- [Controller Implementation](./controller-implementation.md) - Reconciler logic
- [Reconciliation Phases](./reconciliation-phases.md) - 4-phase flow (V1.0)
- [DD-WORKFLOW-001 v1.8](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) - Label schema
- [Integration Points](./integration-points.md) - Service coordination
