## Controller Implementation

### Core Reconciliation Logic

```go
package controller

import (
    "context"
    "fmt"
    "time"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
    aiv1 "github.com/jordigilh/kubernaut/api/ai/v1"
    workflowv1 "github.com/jordigilh/kubernaut/api/workflow/v1"
    executorv1 "github.com/jordigilh/kubernaut/api/executor/v1"

    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type RemediationRequestReconciler struct {
    client.Client
    Scheme *runtime.Scheme

    NotificationClient NotificationClient
    StorageClient      StorageClient
}

func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var remediation remediationv1.RemediationRequest
    if err := r.Get(ctx, req.NamespacedName, &remediation); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Handle finalizer for 24-hour retention
    if !remediation.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(&remediation, remediationFinalizerName) {
            if err := r.finalizeRemediation(ctx, &remediation); err != nil {
                return ctrl.Result{}, err
            }

            controllerutil.RemoveFinalizer(&remediation, remediationFinalizerName)
            if err := r.Update(ctx, &remediation); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // Add finalizer if not present
    if !controllerutil.ContainsFinalizer(&remediation, remediationFinalizerName) {
        controllerutil.AddFinalizer(&remediation, remediationFinalizerName)
        if err := r.Update(ctx, &remediation); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Initialize if new
    if remediation.Status.OverallPhase == "" {
        remediation.Status.OverallPhase = "pending"
        remediation.Status.StartTime = metav1.Now()
        if err := r.Status().Update(ctx, &remediation); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Handle terminal states
    if remediation.Status.OverallPhase == "completed" ||
       remediation.Status.OverallPhase == "failed" ||
       remediation.Status.OverallPhase == "timeout" {
        return r.handleTerminalState(ctx, &remediation)
    }

    // Orchestrate service CRDs based on phase
    return r.orchestratePhase(ctx, &remediation)
}

// Orchestrate service CRD creation based on current phase
func (r *RemediationRequestReconciler) orchestratePhase(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
) (ctrl.Result, error) {

    switch remediation.Status.OverallPhase {
    case "pending":
        // Create RemediationProcessing CRD
        if remediation.Status.RemediationProcessingRef == nil {
            if err := r.createRemediationProcessing(ctx, remediation); err != nil {
                return ctrl.Result{}, err
            }
            remediation.Status.OverallPhase = "processing"
            return ctrl.Result{}, r.Status().Update(ctx, remediation)
        }

    case "processing":
        // Wait for RemediationProcessing completion, then create AIAnalysis
        var alertProcessing processingv1.RemediationProcessing
        if err := r.Get(ctx, client.ObjectKey{
            Name:      remediation.Status.RemediationProcessingRef.Name,
            Namespace: remediation.Status.RemediationProcessingRef.Namespace,
        }, &alertProcessing); err != nil {
            return ctrl.Result{}, err
        }

        // Check timeout
        if r.isPhaseTimedOut(&alertProcessing, remediation.Spec.TimeoutConfig) {
            return r.handleTimeout(ctx, remediation, "alert_processing")
        }

        if alertProcessing.Status.Phase == "completed" {
            if remediation.Status.AIAnalysisRef == nil {
                if err := r.createAIAnalysis(ctx, remediation, &alertProcessing); err != nil {
                    return ctrl.Result{}, err
                }
                remediation.Status.OverallPhase = "analyzing"
                return ctrl.Result{}, r.Status().Update(ctx, remediation)
            }
        } else if alertProcessing.Status.Phase == "failed" {
            return r.handleFailure(ctx, remediation, "alert_processing", "Alert processing failed")
        }

    case "analyzing":
        // Wait for AIAnalysis completion, then create WorkflowExecution
        var aiAnalysis aiv1.AIAnalysis
        if err := r.Get(ctx, client.ObjectKey{
            Name:      remediation.Status.AIAnalysisRef.Name,
            Namespace: remediation.Status.AIAnalysisRef.Namespace,
        }, &aiAnalysis); err != nil {
            return ctrl.Result{}, err
        }

        // Check timeout
        if r.isPhaseTimedOut(&aiAnalysis, remediation.Spec.TimeoutConfig) {
            return r.handleTimeout(ctx, remediation, "ai_analysis")
        }

        if aiAnalysis.Status.Phase == "completed" {
            if remediation.Status.WorkflowExecutionRef == nil {
                if err := r.createWorkflowExecution(ctx, remediation, &aiAnalysis); err != nil {
                    return ctrl.Result{}, err
                }
                remediation.Status.OverallPhase = "executing"
                return ctrl.Result{}, r.Status().Update(ctx, remediation)
            }
        } else if aiAnalysis.Status.Phase == "failed" {
            return r.handleFailure(ctx, remediation, "ai_analysis", "AI analysis failed")
        }

    case "executing":
        // Wait for WorkflowExecution completion, then create KubernetesExecution
        var workflowExecution workflowv1.WorkflowExecution
        if err := r.Get(ctx, client.ObjectKey{
            Name:      remediation.Status.WorkflowExecutionRef.Name,
            Namespace: remediation.Status.WorkflowExecutionRef.Namespace,
        }, &workflowExecution); err != nil {
            return ctrl.Result{}, err
        }

        // Check timeout
        if r.isPhaseTimedOut(&workflowExecution, remediation.Spec.TimeoutConfig) {
            return r.handleTimeout(ctx, remediation, "workflow_execution")
        }

        if workflowExecution.Status.Phase == "completed" {
            if remediation.Status.KubernetesExecutionRef == nil {
                if err := r.createKubernetesExecution(ctx, remediation, &workflowExecution); err != nil {
                    return ctrl.Result{}, err
                }

                // Wait for KubernetesExecution to complete
                var kubernetesExecution executorv1.KubernetesExecution
                if err := r.Get(ctx, client.ObjectKey{
                    Name:      remediation.Status.KubernetesExecutionRef.Name,
                    Namespace: remediation.Status.KubernetesExecutionRef.Namespace,
                }, &kubernetesExecution); err != nil {
                    return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
                }

                if kubernetesExecution.Status.Phase == "completed" {
                    remediation.Status.OverallPhase = "completed"
                    remediation.Status.CompletionTime = &metav1.Time{Time: time.Now()}
                    remediation.Status.RetentionExpiryTime = &metav1.Time{Time: time.Now().Add(24 * time.Hour)}
                    return ctrl.Result{}, r.Status().Update(ctx, remediation)
                } else if kubernetesExecution.Status.Phase == "failed" {
                    return r.handleFailure(ctx, remediation, "kubernetes_execution", "Kubernetes execution failed")
                }
            }
        } else if workflowExecution.Status.Phase == "failed" {
            return r.handleFailure(ctx, remediation, "workflow_execution", "Workflow execution failed")
        }
    }

    // Requeue to check progress
    return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// Handle terminal state (completed, failed, timeout)
func (r *RemediationRequestReconciler) handleTerminalState(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
) (ctrl.Result, error) {

    // Check if 24-hour retention has expired
    if remediation.Status.RetentionExpiryTime != nil {
        if time.Now().After(remediation.Status.RetentionExpiryTime.Time) {
            // Delete CRD (finalizer cleanup will be triggered)
            return ctrl.Result{}, r.Delete(ctx, remediation)
        }

        // Requeue to check expiry later
        requeueAfter := time.Until(remediation.Status.RetentionExpiryTime.Time)
        return ctrl.Result{RequeueAfter: requeueAfter}, nil
    }

    return ctrl.Result{}, nil
}

// Handle timeout
func (r *RemediationRequestReconciler) handleTimeout(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    phase string,
) (ctrl.Result, error) {

    remediation.Status.OverallPhase = "timeout"
    remediation.Status.TimeoutPhase = &phase
    remediation.Status.TimeoutTime = &metav1.Time{Time: time.Now()}
    remediation.Status.RetentionExpiryTime = &metav1.Time{Time: time.Now().Add(24 * time.Hour)}

    // Escalate timeout
    if err := r.escalateTimeout(ctx, remediation, phase); err != nil {
        return ctrl.Result{}, err
    }

    // Record audit
    if err := r.recordAudit(ctx, remediation); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, r.Status().Update(ctx, remediation)
}

// Handle failure
func (r *RemediationRequestReconciler) handleFailure(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    phase string,
    reason string,
) (ctrl.Result, error) {

    remediation.Status.OverallPhase = "failed"
    remediation.Status.FailurePhase = &phase
    remediation.Status.FailureReason = &reason
    remediation.Status.CompletionTime = &metav1.Time{Time: time.Now()}
    remediation.Status.RetentionExpiryTime = &metav1.Time{Time: time.Now().Add(24 * time.Hour)}

    // Record audit
    if err := r.recordAudit(ctx, remediation); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, r.Status().Update(ctx, remediation)
}

// Finalizer cleanup
func (r *RemediationRequestReconciler) finalizeRemediation(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
) error {

    // Record final audit before deletion
    return r.recordAudit(ctx, remediation)
}

const remediationFinalizerName = "kubernaut.io/remediation-retention"
```

---

