## Integration Points

**Version**: 4.1
**Last Updated**: 2025-12-07
**CRD API Group**: `kubernaut.ai/v1alpha1`
**Status**: ✅ Updated for Tekton Architecture (ADR-044)

---

## Changelog

### Version 4.2 (2026-02-18)
- ✅ **Issue #91**: Removed `kubernaut.ai/remediation-request` label from WorkflowExecution creation; use `spec.remediationRequestRef`. `kubernaut.ai/workflow-execution` KEPT on PipelineRun (external resource).

### Version 4.1 (2025-12-07)
- ✅ **Fixed**: RemediationRequestRef type corrected to `corev1.ObjectReference` (was incorrectly documented as custom type)

### Version 4.0 (2025-12-02)
- ✅ **Updated**: Tekton PipelineRun integration patterns
- ✅ **Added**: Resource locking integration with RO

---

### 1. Upstream Integration: RemediationOrchestrator

**Integration Pattern**: RO creates WorkflowExecution after AIAnalysis completes

```go
// In RemediationOrchestrator
func (r *RemediationOrchestratorReconciler) createWorkflowExecution(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
    aiAnalysis *aianalysisv1.AIAnalysis,
) error {
    wfe := &workflowexecutionv1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-wfe", rr.Name),
            Namespace: rr.Namespace,
            // Issue #91: kubernaut.ai/remediation-request removed; use spec.remediationRequestRef
            Labels: map[string]string{
                "kubernaut.ai/correlation-id": rr.Labels["kubernaut.ai/correlation-id"],
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(rr, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: workflowexecutionv1.WorkflowExecutionSpec{
            // RemediationRequestRef uses corev1.ObjectReference (not custom type)
            RemediationRequestRef: corev1.ObjectReference{
                APIVersion: remediationv1.GroupVersion.String(),
                Kind:       "RemediationRequest",
                Name:       rr.Name,
                Namespace:  rr.Namespace,
                UID:        rr.UID,
            },
            WorkflowRef: workflowexecutionv1.WorkflowReference{
                WorkflowID:     aiAnalysis.Status.SelectedWorkflow.WorkflowID,
                ContainerImage: aiAnalysis.Status.SelectedWorkflow.ContainerImage,
                Version:        aiAnalysis.Status.SelectedWorkflow.Version,
            },
            TargetResource: rr.Spec.TargetResource,
            Parameters:     aiAnalysis.Status.SelectedWorkflow.Parameters,
        },
    }

    return r.Create(ctx, wfe)
}
```

**Data Flow**:
```
AIAnalysis.Status.SelectedWorkflow
    ↓
WorkflowExecution.Spec.WorkflowRef
    - workflowId: "increase-memory-conservative"
    - containerImage: "ghcr.io/kubernaut/workflows/increase-memory@sha256:..."
    - parameters: {NAMESPACE: "production", DEPLOYMENT_NAME: "payment-service"}
    ↓
WorkflowExecution.Spec.TargetResource (from RemediationRequest)
    - "production/deployment/payment-service"
```

---

### 2. Downstream Integration: Tekton Pipelines

**Integration Pattern**: WorkflowExecution creates PipelineRun with bundle resolver

```go
func (r *WorkflowExecutionReconciler) buildPipelineRun(
    wfe *workflowexecutionv1.WorkflowExecution,
) *tektonv1.PipelineRun {
    // Convert parameters to Tekton format
    params := make([]tektonv1.Param, 0, len(wfe.Spec.Parameters))
    for key, value := range wfe.Spec.Parameters {
        params = append(params, tektonv1.Param{
            Name:  key,
            Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: value},
        })
    }

    return &tektonv1.PipelineRun{
        ObjectMeta: metav1.ObjectMeta{
            Name:      wfe.Name,
            Namespace: r.ExecutionNamespace,  // "kubernaut-workflows" (DD-WE-002)
            Labels: map[string]string{
                // Issue #91: KEPT - label on external K8s resource (PipelineRun) for WE-to-PipelineRun correlation
                "kubernaut.ai/workflow-execution": wfe.Name,
                "kubernaut.ai/source-namespace":   wfe.Namespace,  // Track source for cleanup
                "kubernaut.ai/workflow-id":        wfe.Spec.WorkflowRef.WorkflowID,
                "kubernaut.ai/target-resource":    wfe.Spec.TargetResource,
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(wfe, workflowexecutionv1.GroupVersion.WithKind("WorkflowExecution")),
            },
        },
        Spec: tektonv1.PipelineRunSpec{
            PipelineRef: &tektonv1.PipelineRef{
                ResolverRef: tektonv1.ResolverRef{
                    Resolver: "bundles",
                    Params: []tektonv1.Param{
                        {Name: "bundle", Value: tektonv1.ParamValue{StringVal: wfe.Spec.WorkflowRef.ContainerImage}},
                        {Name: "name", Value: tektonv1.ParamValue{StringVal: "workflow"}},
                    },
                },
            },
            Params: params,
            TaskRunTemplate: tektonv1.PipelineTaskRunTemplate{
                ServiceAccountName: wfe.Spec.ExecutionConfig.ServiceAccountName,
            },
        },
    }
}
```

**Tekton Integration Flow**:
```
WorkflowExecution.Spec.WorkflowRef.ContainerImage
    ↓
Tekton Bundle Resolver fetches OCI bundle
    ↓
Tekton extracts Pipeline definition from bundle
    ↓
Tekton creates TaskRuns for each Pipeline task
    ↓
Tekton executes tasks (step orchestration handled by Tekton)
    ↓
PipelineRun.Status.Conditions[Succeeded]
    ↓
WorkflowExecution syncs status
```

---

### 3. Status Synchronization

**Pattern**: WorkflowExecution watches PipelineRun status via reconciliation

```go
func (r *WorkflowExecutionReconciler) syncStatusFromPipelineRun(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
    pr *tektonv1.PipelineRun,
) error {
    // Map Tekton status to WFE status
    for _, cond := range pr.Status.Conditions {
        if cond.Type != "Succeeded" {
            continue
        }

        switch cond.Status {
        case "True":
            wfe.Status.Phase = workflowexecutionv1.PhaseCompleted
            wfe.Status.Outcome = workflowexecutionv1.OutcomeSuccess
            wfe.Status.CompletionTime = pr.Status.CompletionTime
            wfe.Status.Message = "Workflow completed successfully"

        case "False":
            wfe.Status.Phase = workflowexecutionv1.PhaseFailed
            wfe.Status.Outcome = workflowexecutionv1.OutcomeFailed
            wfe.Status.CompletionTime = pr.Status.CompletionTime
            wfe.Status.FailureDetails = r.extractFailureDetails(pr, cond)

        default:
            // Still running
            wfe.Status.Message = fmt.Sprintf("Running: %s", cond.Reason)
        }
    }

    return r.Status().Update(ctx, wfe)
}

func (r *WorkflowExecutionReconciler) extractFailureDetails(
    pr *tektonv1.PipelineRun,
    cond duckv1.Condition,
) *workflowexecutionv1.FailureDetails {
    fd := &workflowexecutionv1.FailureDetails{
        Reason:            cond.Reason,
        Message:           cond.Message,
        FailedAt:          metav1.Now(),
        WasExecutionFailure: pr.Status.StartTime != nil,
    }

    // Extract failed task info from TaskRuns
    for _, childRef := range pr.Status.ChildReferences {
        if childRef.Kind == "TaskRun" {
            // Get TaskRun to check for failure
            var tr tektonv1.TaskRun
            if err := r.Get(ctx, client.ObjectKey{
                Name:      childRef.Name,
                Namespace: pr.Namespace,
            }, &tr); err == nil {
                if taskFailed(&tr) {
                    fd.FailedTaskName = tr.Spec.TaskRef.Name
                    fd.FailedStepName = getFailedStepName(&tr)
                    break
                }
            }
        }
    }

    // Generate natural language summary
    fd.NaturalLanguageSummary = generateFailureSummary(fd)

    return fd
}
```

---

### 4. Resource Locking Integration

**Pattern**: WorkflowExecution checks for active executions on same target (DD-WE-001)

```go
func (r *WorkflowExecutionReconciler) checkResourceLock(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) (blocked bool, conflicting *workflowexecutionv1.WorkflowExecution) {
    // List all WorkflowExecutions in namespace
    var wfeList workflowexecutionv1.WorkflowExecutionList
    if err := r.List(ctx, &wfeList, client.InNamespace(wfe.Namespace)); err != nil {
        return false, nil
    }

    for _, other := range wfeList.Items {
        // Skip self
        if other.Name == wfe.Name {
            continue
        }

        // Check if same target resource
        if other.Spec.TargetResource != wfe.Spec.TargetResource {
            continue
        }

        // Check if running or pending
        if other.Status.Phase == workflowexecutionv1.PhaseRunning ||
           other.Status.Phase == workflowexecutionv1.PhasePending {
            return true, &other
        }
    }

    return false, nil
}

func (r *WorkflowExecutionReconciler) checkCooldown(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) *workflowexecutionv1.WorkflowExecution {
    cooldownPeriod := 5 * time.Minute // Configurable

    var wfeList workflowexecutionv1.WorkflowExecutionList
    if err := r.List(ctx, &wfeList, client.InNamespace(wfe.Namespace)); err != nil {
        return nil
    }

    for _, other := range wfeList.Items {
        if other.Name == wfe.Name {
            continue
        }

        // Check same target + workflow
        if other.Spec.TargetResource != wfe.Spec.TargetResource ||
           other.Spec.WorkflowRef.WorkflowID != wfe.Spec.WorkflowRef.WorkflowID {
            continue
        }

        // Check completed within cooldown
        if other.Status.Phase == workflowexecutionv1.PhaseCompleted &&
           other.Status.CompletionTime != nil &&
           time.Since(other.Status.CompletionTime.Time) < cooldownPeriod {
            return &other
        }
    }

    return nil
}
```

---

### 5. Audit Trail Integration

**Pattern**: WorkflowExecution records execution events to Data Storage

```go
func (r *WorkflowExecutionReconciler) recordAuditEvent(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
    eventType string,
) error {
    record := &datastorage.AuditRecord{
        EventType:           eventType,
        WorkflowExecutionID: wfe.Name,
        WorkflowID:          wfe.Spec.WorkflowRef.WorkflowID,
        TargetResource:      wfe.Spec.TargetResource,
        Phase:               string(wfe.Status.Phase),
        Outcome:             string(wfe.Status.Outcome),
        Timestamp:           time.Now(),
        CorrelationID:       wfe.Labels["kubernaut.ai/correlation-id"],
    }

    if wfe.Status.FailureDetails != nil {
        record.FailureReason = wfe.Status.FailureDetails.Reason
        record.FailureMessage = wfe.Status.FailureDetails.Message
    }

    return r.DataStorageClient.RecordAudit(ctx, record)
}
```

---

### Integration Summary

| Integration | Direction | Mechanism | Data Flow |
|-------------|-----------|-----------|-----------|
| **RemediationOrchestrator** | Upstream | CRD creation | AIAnalysis → WFE Spec |
| **Tekton Pipelines** | Downstream | PipelineRun creation | WFE Spec → PipelineRun |
| **Status Sync** | Bidirectional | Reconciliation | PipelineRun Status → WFE Status |
| **Resource Locking** | Internal | CRD list/filter | WFE → WFE |
| **Data Storage** | Downstream | API call | WFE Status → Audit |

---

