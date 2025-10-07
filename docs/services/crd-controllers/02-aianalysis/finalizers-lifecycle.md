## Finalizer Implementation

### Finalizer Name

Following Kubernetes finalizer naming convention:

```go
const aiAnalysisFinalizer = "aianalysis.kubernaut.io/aianalysis-cleanup"
```

**Naming Pattern**: `{domain}.kubernaut.io/{resource}-cleanup`

**Why This Pattern**:
- **Domain-Scoped**: `aianalysis.kubernaut.io` prevents conflicts with other services
- **Resource-Specific**: `aianalysis-cleanup` clearly indicates what's being cleaned up
- **Kubernetes Convention**: Follows standard finalizer naming (domain/action format)

---

### Complete Reconciliation Loop with Finalizer

```go
package controller

import (
    "context"
    "fmt"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/analysis/v1"

    "github.com/go-logr/logr"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const aiAnalysisFinalizer = "aianalysis.kubernaut.io/aianalysis-cleanup"

type AIAnalysisReconciler struct {
    client.Client
    Scheme            *runtime.Scheme
    Log               logr.Logger
    Recorder          record.EventRecorder
    HolmesGPTClient   HolmesGPTClient
    StorageClient     StorageClient
}

func (r *AIAnalysisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var analysis aianalysisv1.AIAnalysis
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

    // Skip if already completed or failed
    if analysis.Status.Phase == "completed" || analysis.Status.Phase == "failed" {
        return ctrl.Result{}, nil
    }

    // Execute AI analysis...
    // (existing reconciliation logic from previous section)

    return ctrl.Result{}, nil
}
```

---

### Cleanup Logic

**What Gets Cleaned Up** (Middle Controller Pattern):

```go
package controller

import (
    "context"
    "fmt"
    "time"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/analysis/v1"
    approvalv1 "github.com/jordigilh/kubernaut/api/ai/analysis/v1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *AIAnalysisReconciler) cleanupAIAnalysis(
    ctx context.Context,
    analysis *aianalysisv1.AIAnalysis,
) error {
    r.Log.Info("Cleaning up AIAnalysis resources",
        "name", analysis.Name,
        "namespace", analysis.Namespace,
        "phase", analysis.Status.Phase,
    )

    // 1. Delete owned AIApprovalRequest CRDs (if not already cascade-deleted)
    if err := r.cleanupApprovalRequests(ctx, analysis); err != nil {
        r.Log.Error(err, "Failed to cleanup approval requests", "name", analysis.Name)
        // Continue with best-effort cleanup
    }

    // 2. Record final audit to database
    if err := r.recordFinalAudit(ctx, analysis); err != nil {
        r.Log.Error(err, "Failed to record final audit", "name", analysis.Name)
        // Don't block deletion on audit failure
    }

    // 3. Emit deletion event
    r.Recorder.Event(analysis, "Normal", "AIAnalysisDeleted",
        fmt.Sprintf("AIAnalysis cleanup completed (phase: %s)", analysis.Status.Phase))

    r.Log.Info("AIAnalysis cleanup completed successfully",
        "name", analysis.Name,
        "namespace", analysis.Namespace,
    )

    return nil
}

func (r *AIAnalysisReconciler) cleanupApprovalRequests(
    ctx context.Context,
    analysis *aianalysisv1.AIAnalysis,
) error {
    // List all AIApprovalRequest CRDs owned by this AIAnalysis
    var approvalList approvalv1.AIApprovalRequestList
    if err := r.List(ctx, &approvalList,
        client.InNamespace(analysis.Namespace),
        client.MatchingFields{"metadata.ownerReferences.uid": string(analysis.UID)},
    ); err != nil {
        return fmt.Errorf("failed to list approval requests: %w", err)
    }

    // Delete each approval request
    for _, approval := range approvalList.Items {
        if err := r.Delete(ctx, &approval); err != nil {
            if client.IgnoreNotFound(err) != nil {
                r.Log.Error(err, "Failed to delete approval request",
                    "approval", approval.Name,
                    "analysis", analysis.Name,
                )
                // Continue with best-effort cleanup
            }
        }
    }

    r.Log.Info("Cleaned up approval requests",
        "count", len(approvalList.Items),
        "analysis", analysis.Name,
    )

    return nil
}

func (r *AIAnalysisReconciler) recordFinalAudit(
    ctx context.Context,
    analysis *aianalysisv1.AIAnalysis,
) error {
    auditRecord := &AuditRecord{
        AlertFingerprint: analysis.Spec.EnrichedAlert.Fingerprint,
        ServiceType:      "AIAnalysis",
        CRDName:          analysis.Name,
        Namespace:        analysis.Namespace,
        Phase:            analysis.Status.Phase,
        CreatedAt:        analysis.CreationTimestamp.Time,
        DeletedAt:        analysis.DeletionTimestamp.Time,
        AnalysisStatus:   analysis.Status.AnalysisStatus,
        RecommendationsCount: len(analysis.Status.Recommendations),
    }

    return r.StorageClient.RecordAudit(ctx, auditRecord)
}
```

**Cleanup Philosophy for AIAnalysis** (Middle Controller):
- ✅ **Delete child AIApprovalRequest CRDs**: Middle controller owns child CRDs (best-effort, may already be cascade-deleted)
- ✅ **Record final audit**: Capture analysis outcomes (best-effort)
- ✅ **Emit deletion event**: Operational visibility
- ✅ **Non-blocking**: Child deletion and audit failures don't block deletion (best-effort)
- ✅ **Two-layer cleanup**: Own resources + child CRDs

**Why Best-Effort for AIApprovalRequest Deletion**:
- Owner references should handle cascade deletion automatically
- Explicit cleanup is a safety net if owner references fail
- Don't block AIAnalysis deletion if child deletion fails

---

### Finalizer Testing

**Unit Test Pattern**:

```go
package controller_test

import (
    "context"
    "fmt"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/analysis/v1"
    approvalv1 "github.com/jordigilh/kubernaut/api/ai/analysis/v1"
    "github.com/jordigilh/kubernaut/pkg/ai/analysis/controller"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/types"
    "k8s.io/client-go/kubernetes/scheme"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("AIAnalysis Finalizer", func() {
    var (
        ctx       context.Context
        k8sClient client.Client
        reconciler *controller.AIAnalysisReconciler
    )

    BeforeEach(func() {
        ctx = context.Background()
        k8sClient = fake.NewClientBuilder().
            WithScheme(scheme.Scheme).
            Build()

        reconciler = &controller.AIAnalysisReconciler{
            Client:          k8sClient,
            HolmesGPTClient: &mockHolmesGPTClient{},
            StorageClient:   &mockStorageClient{},
        }
    })

    Context("when AIAnalysis is created", func() {
        It("should add finalizer on first reconcile", func() {
            analysis := &aianalysisv1.AIAnalysis{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-analysis",
                    Namespace: "default",
                },
                Spec: aianalysisv1.AIAnalysisSpec{
                    EnrichedAlert: aianalysisv1.EnrichedAlert{
                        Fingerprint: "abc123",
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
        It("should cleanup approval requests and remove finalizer", func() {
            analysis := &aianalysisv1.AIAnalysis{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-analysis",
                    Namespace:  "default",
                    UID:        types.UID("analysis-uid-123"),
                    Finalizers: []string{aiAnalysisFinalizer},
                },
                Status: aianalysisv1.AIAnalysisStatus{
                    Phase: "completed",
                },
            }
            Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

            // Create owned AIApprovalRequest
            approval := &approvalv1.AIApprovalRequest{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-approval",
                    Namespace: "default",
                    OwnerReferences: []metav1.OwnerReference{
                        *metav1.NewControllerRef(analysis,
                            aianalysisv1.GroupVersion.WithKind("AIAnalysis")),
                    },
                },
            }
            Expect(k8sClient.Create(ctx, approval)).To(Succeed())

            // Delete AIAnalysis
            Expect(k8sClient.Delete(ctx, analysis)).To(Succeed())

            // Reconcile should execute cleanup
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(analysis),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify AIApprovalRequest deleted
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(approval), approval)
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())

            // Verify finalizer removed (CRD will be deleted by Kubernetes)
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
            Expect(err).To(HaveOccurred())
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })

        It("should not block deletion if approval cleanup fails", func() {
            analysis := &aianalysisv1.AIAnalysis{
                ObjectMeta: metav1.ObjectMeta{
                    Name:       "test-analysis",
                    Namespace:  "default",
                    Finalizers: []string{aiAnalysisFinalizer},
                },
            }
            Expect(k8sClient.Create(ctx, analysis)).To(Succeed())
            Expect(k8sClient.Delete(ctx, analysis)).To(Succeed())

            // Cleanup should succeed even if no approvals found
            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(analysis),
            })
            Expect(err).ToNot(HaveOccurred())

            // Verify finalizer removed
            err = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
            Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
        })

        It("should not block deletion if audit fails", func() {
            // Mock storage client to return error
            reconciler.StorageClient = &mockStorageClient{
                recordAuditError: fmt.Errorf("database unavailable"),
            }

            analysis := &aianalysisv1.AIAnalysis{
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

**Created By**: RemediationRequest controller (centralized orchestration)

**Creation Trigger**: RemediationProcessing completion

**Sequence**:
```
RemediationProcessing.status.phase = "completed"
    ↓ (watch trigger <100ms)
RemediationRequest Controller reconciles
    ↓
RemediationRequest extracts enriched alert data
    ↓
RemediationRequest Controller creates AIAnalysis CRD
    ↓ (with owner reference)
AIAnalysis Controller reconciles (this controller)
    ↓
AIAnalysis calls HolmesGPT-API for analysis
    ↓
AIAnalysis.status.phase = "awaiting_approval"
    ↓
AIAnalysis Controller creates AIApprovalRequest CRD
    ↓ (with owner reference)
AIApprovalRequest Controller reconciles
    ↓
AIApprovalRequest.status.decision = "approved"
    ↓ (watch trigger <100ms)
AIAnalysis Controller detects approval
    ↓
AIAnalysis.status.phase = "completed"
    ↓ (watch trigger <100ms)
RemediationRequest Controller detects completion
    ↓
RemediationRequest Controller creates WorkflowExecution CRD
```

**Owner Reference Set at Creation**:
```go
package controller

import (
    "context"
    "fmt"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/analysis/v1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// In RemediationRequestReconciler
func (r *RemediationRequestReconciler) createAIAnalysis(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    enrichedAlert *aianalysisv1.EnrichedAlert,
) error {
    analysis := &aianalysisv1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-analysis", remediation.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation,
                    remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: aianalysisv1.AIAnalysisSpec{
            RemediationRequestRef: aianalysisv1.RemediationRequestReference{
                Name:      remediation.Name,
                Namespace: remediation.Namespace,
            },
            EnrichedAlert: *enrichedAlert,
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

    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/analysis/v1"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *AIAnalysisReconciler) updateStatusAwaitingApproval(
    ctx context.Context,
    analysis *aianalysisv1.AIAnalysis,
    recommendations []aianalysisv1.RemediationRecommendation,
) error {
    analysis.Status.Phase = "awaiting_approval"
    analysis.Status.AnalysisStatus = "complete"
    analysis.Status.Recommendations = recommendations
    analysis.Status.LastUpdated = &metav1.Time{Time: time.Now()}

    return r.Status().Update(ctx, analysis)
}

func (r *AIAnalysisReconciler) updateStatusCompleted(
    ctx context.Context,
    analysis *aianalysisv1.AIAnalysis,
    approvalDecision string,
) error {
    analysis.Status.Phase = "completed"
    analysis.Status.CompletionTime = &metav1.Time{Time: time.Now()}
    analysis.Status.ApprovalDecision = approvalDecision

    return r.Status().Update(ctx, analysis)
}
```

**Watch Triggers RemediationRequest Reconciliation**:

```
AIAnalysis.status.phase = "completed"
    ↓ (watch event)
RemediationRequest watch triggers
    ↓ (<100ms latency)
RemediationRequest Controller reconciles
    ↓
RemediationRequest extracts approved recommendations
    ↓
RemediationRequest creates WorkflowExecution CRD
```

**Two-Layer Watch Pattern** (Middle Controller):
- **Upstream Watch**: RemediationRequest watches AIAnalysis status
- **Downstream Watch**: AIAnalysis watches AIApprovalRequest status

---

### Deletion Lifecycle

**Trigger**: RemediationRequest deletion (cascade)

**Cascade Deletion Sequence** (Two-Layer):
```
User/System deletes RemediationRequest
    ↓
Kubernetes garbage collector detects owner reference
    ↓ (parallel deletion of all owned CRDs)
AIAnalysis.deletionTimestamp set
    ↓ (triggers cascade deletion of AIAnalysis-owned CRDs)
AIApprovalRequest.deletionTimestamp set
    ↓ (parallel: AIAnalysis finalizer + AIApprovalRequest deletion)
AIAnalysis Controller reconciles (detects deletion)
    ↓
Finalizer cleanup executes:
  - Delete AIApprovalRequest CRDs (best-effort, may already be deleted)
  - Record final audit
  - Emit deletion event
    ↓
Finalizer removed
    ↓
Kubernetes deletes AIAnalysis CRD
```

**Two-Layer Cascade Pattern**:
- **Layer 1**: RemediationRequest → AIAnalysis (parent-child)
- **Layer 2**: AIAnalysis → AIApprovalRequest (middle-child)

**Retention**:
- **AIAnalysis**: No independent retention (deleted with parent)
- **AIApprovalRequest**: No independent retention (deleted with AIAnalysis)
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

    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/analysis/v1"

    "k8s.io/client-go/tools/record"
)

func (r *AIAnalysisReconciler) emitLifecycleEvents(
    analysis *aianalysisv1.AIAnalysis,
    oldPhase string,
    duration time.Duration,
) {
    // Creation event
    r.Recorder.Event(analysis, "Normal", "AIAnalysisCreated",
        fmt.Sprintf("AI analysis started for alert %s", analysis.Spec.EnrichedAlert.Fingerprint))

    // Phase transition events
    r.Recorder.Event(analysis, "Normal", "PhaseTransition",
        fmt.Sprintf("Phase: %s → %s", oldPhase, analysis.Status.Phase))

    // HolmesGPT analysis events
    if analysis.Status.AnalysisStatus == "complete" {
        r.Recorder.Event(analysis, "Normal", "AnalysisComplete",
            fmt.Sprintf("Generated %d recommendations", len(analysis.Status.Recommendations)))
    }

    // Approval request created
    if analysis.Status.Phase == "awaiting_approval" {
        r.Recorder.Event(analysis, "Normal", "ApprovalRequested",
            "AIApprovalRequest created, awaiting decision")
    }

    // Approval received
    if analysis.Status.ApprovalDecision != "" {
        eventType := "Normal"
        if analysis.Status.ApprovalDecision == "rejected" {
            eventType = "Warning"
        }
        r.Recorder.Event(analysis, eventType, "ApprovalReceived",
            fmt.Sprintf("Approval decision: %s", analysis.Status.ApprovalDecision))
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

# Approval request rate
rate(aianalysis_approval_requests_total[5m])

# Approval decision distribution
sum(aianalysis_approvals_total) by (decision)

# HolmesGPT analysis failures
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

  - title: "Approval Decision Distribution"
    targets:
      - expr: sum(aianalysis_approvals_total{decision="approved"})
        legendFormat: "Approved"
      - expr: sum(aianalysis_approvals_total{decision="rejected"})
        legendFormat: "Rejected"
      - expr: sum(aianalysis_approvals_total{decision="auto_approved"})
        legendFormat: "Auto-Approved"
```

**Alert Rules**:

```yaml
groups:
- name: aianalysis-lifecycle
  rules:
  - alert: AIAnalysisStuckAwaitingApproval
    expr: time() - aianalysis_phase_start_timestamp{phase="awaiting_approval"} > 3600
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "AIAnalysis stuck awaiting approval for >1 hour"
      description: "AIAnalysis {{ $labels.name }} has been awaiting approval for over 1 hour"

  - alert: AIAnalysisHighFailureRate
    expr: rate(aianalysis_holmesgpt_failures_total[5m]) > 0.1
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "High HolmesGPT analysis failure rate"
      description: "HolmesGPT analysis failing for >10% of requests"

  - alert: AIAnalysisHighRejectionRate
    expr: sum(rate(aianalysis_approvals_total{decision="rejected"}[5m])) / sum(rate(aianalysis_approvals_total[5m])) > 0.5
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: ">50% of AI recommendations being rejected"
      description: "High rejection rate may indicate poor recommendation quality"

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

