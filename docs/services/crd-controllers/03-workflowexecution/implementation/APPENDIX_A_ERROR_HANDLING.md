# WorkflowExecution - Error Handling Philosophy

**Parent Document**: [IMPLEMENTATION_PLAN_V3.0.md](./IMPLEMENTATION_PLAN_V3.0.md)
**Version**: v1.0
**Last Updated**: 2025-12-03
**Status**: ✅ Ready for Implementation

---

## Document Purpose

This appendix defines the error handling philosophy for the WorkflowExecution Controller, aligned with [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](../../../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md) Error Handling Philosophy section.

---

## 🎯 Error Categories

WorkflowExecution handles 5 distinct error categories:

### Category A: Validation Errors (User Recoverable)

**When**: Invalid spec fields, malformed targetResource, missing required fields
**Action**: Fail immediately with ConfigurationError, no retry
**HTTP Equivalent**: 400 Bad Request
**Retry Strategy**: None - user must fix and resubmit

```go
// Example: Invalid targetResource format
func (r *WorkflowExecutionReconciler) validateSpec(wfe *workflowexecutionv1.WorkflowExecution) error {
    if !validTargetResourceFormat(wfe.Spec.TargetResource) {
        r.Recorder.Event(wfe, corev1.EventTypeWarning, "InvalidSpec",
            "targetResource must be format: namespace/kind/name")
        return r.markFailed(ctx, wfe, &FailureDetails{
            Reason:  "ConfigurationError",
            Message: "Invalid targetResource format",
            WasExecutionFailure: false,  // Pre-execution failure
        })
    }
    return nil
}
```

**Error Codes**:
| Code | Description |
|------|-------------|
| `ConfigurationError` | Invalid spec fields |
| `InvalidTargetResource` | Malformed target resource string |
| `MissingWorkflowRef` | WorkflowRef not specified |
| `InvalidContainerImage` | OCI bundle reference malformed |

---

### Category B: External Dependency Errors (Transient)

**When**: Tekton API unavailable, PipelineRun creation fails, network errors
**Action**: Retry with exponential backoff
**HTTP Equivalent**: 503 Service Unavailable
**Retry Strategy**: Exponential backoff up to 5 attempts

```go
// Example: Tekton PipelineRun creation failure
func (r *WorkflowExecutionReconciler) createPipelineRun(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
    pr := r.buildPipelineRun(wfe)

    if err := r.Create(ctx, pr); err != nil {
        if apierrors.IsAlreadyExists(err) {
            // Race condition caught - mark as skipped
            return r.markSkipped(ctx, wfe, "ResourceBusy", "PipelineRun already exists")
        }

        // Transient error - requeue with backoff
        log.Error(err, "Failed to create PipelineRun",
            "pipelinerun", pr.Name,
            "namespace", r.ExecutionNamespace)

        r.Recorder.Event(wfe, corev1.EventTypeWarning, "CreateFailed",
            fmt.Sprintf("Failed to create PipelineRun: %v", err))

        return ctrl.Result{RequeueAfter: r.calculateBackoff(wfe)}, nil
    }

    return r.markRunning(ctx, wfe, pr.Name)
}

func (r *WorkflowExecutionReconciler) calculateBackoff(wfe *workflowexecutionv1.WorkflowExecution) time.Duration {
    // Exponential backoff: 1s, 2s, 4s, 8s, 16s (max)
    attempts := wfe.Status.RetryCount
    if attempts > 4 {
        attempts = 4
    }
    return time.Duration(1<<attempts) * time.Second
}
```

**Error Codes**:
| Code | Description |
|------|-------------|
| `TektonUnavailable` | Cannot reach Tekton API |
| `PipelineRunCreateFailed` | Failed to create PipelineRun |
| `NetworkError` | Network connectivity issue |

---

### Category C: Permission Errors (Pre-Execution Failure)

**When**: RBAC denies PipelineRun creation, ServiceAccount missing
**Action**: Fail with PermissionDenied, no auto-retry (manual intervention needed)
**HTTP Equivalent**: 403 Forbidden
**Retry Strategy**: None - requires RBAC fix

```go
// Example: RBAC error during PipelineRun creation
func (r *WorkflowExecutionReconciler) handleCreateError(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
    err error,
) (ctrl.Result, error) {
    if apierrors.IsForbidden(err) {
        log.Error(err, "Permission denied creating PipelineRun",
            "serviceaccount", r.ServiceAccountName,
            "namespace", r.ExecutionNamespace)

        return r.markFailed(ctx, wfe, &FailureDetails{
            Reason:  "PermissionDenied",
            Message: fmt.Sprintf("ServiceAccount %s lacks permission: %v", r.ServiceAccountName, err),
            NaturalLanguageSummary: fmt.Sprintf(
                "The workflow could not start because the service account '%s' does not have "+
                "permission to create PipelineRuns in namespace '%s'. "+
                "Check RBAC configuration and ensure the ClusterRoleBinding exists.",
                r.ServiceAccountName, r.ExecutionNamespace),
            WasExecutionFailure: false,  // Pre-execution - safe to retry after RBAC fix
        })
    }
    return ctrl.Result{}, err
}
```

**Error Codes**:
| Code | Description |
|------|-------------|
| `PermissionDenied` | RBAC forbids operation |
| `ServiceAccountMissing` | ServiceAccount not found |
| `ClusterRoleMissing` | Required ClusterRole not found |

---

### Category D: Execution Errors (During-Execution Failure)

**When**: PipelineRun fails after starting, TaskRun fails
**Action**: Mark as Failed with detailed failure info, NO auto-retry (cluster state unknown)
**HTTP Equivalent**: 500 Internal Server Error
**Retry Strategy**: None - manual review required

```go
// Example: PipelineRun failed during execution
func (r *WorkflowExecutionReconciler) handlePipelineRunFailure(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
    pr *tektonv1.PipelineRun,
) (ctrl.Result, error) {
    failureDetails := r.extractFailureDetails(pr)
    failureDetails.WasExecutionFailure = true  // CRITICAL: Mark as execution failure

    log.Info("PipelineRun failed during execution",
        "pipelinerun", pr.Name,
        "reason", failureDetails.Reason,
        "message", failureDetails.Message)

    r.Recorder.Event(wfe, corev1.EventTypeWarning, "ExecutionFailed",
        fmt.Sprintf("Workflow execution failed: %s", failureDetails.Message))

    // Do NOT retry - cluster state is unknown
    return r.markFailed(ctx, wfe, failureDetails)
}

func (r *WorkflowExecutionReconciler) extractFailureDetails(
    pr *tektonv1.PipelineRun,
) *workflowexecutionv1.FailureDetails {
    cond := pr.Status.GetCondition(apis.ConditionSucceeded)

    details := &workflowexecutionv1.FailureDetails{
        Reason:  cond.Reason,
        Message: cond.Message,
    }

    // Find failed TaskRun for more details
    if failedTask := findFailedTaskRun(pr); failedTask != nil {
        details.FailedTaskName = failedTask.Name
        details.NaturalLanguageSummary = generateNLSummary(failedTask, pr)
    }

    return details
}

func generateNLSummary(task *tektonv1.TaskRun, pr *tektonv1.PipelineRun) string {
    return fmt.Sprintf(
        "The workflow '%s' failed during task '%s'. "+
        "Reason: %s. "+
        "The cluster state may have been partially modified. "+
        "Review the PipelineRun logs before retrying.",
        pr.Name, task.Name, task.Status.GetCondition(apis.ConditionSucceeded).Message)
}
```

**Error Codes**:
| Code | Description |
|------|-------------|
| `TaskRunFailed` | A TaskRun within the Pipeline failed |
| `PipelineTimeout` | Pipeline exceeded timeout |
| `CouldntGetTask` | Task reference not found |
| `ResourceCreationFailed` | Failed to create resources within task |

---

### Category E: System Errors (Unexpected)

**When**: Unexpected errors, panics, internal bugs
**Action**: Log extensively, emit metric, requeue with long delay
**HTTP Equivalent**: 500 Internal Server Error
**Retry Strategy**: Long delay (30s) to avoid tight loops

```go
// Example: Unexpected error handling
func (r *WorkflowExecutionReconciler) Reconcile(
    ctx context.Context,
    req ctrl.Request,
) (ctrl.Result, error) {
    defer func() {
        if r := recover(); r != nil {
            log.Error(fmt.Errorf("%v", r), "Panic in reconcile loop",
                "request", req.NamespacedName,
                "stacktrace", string(debug.Stack()))

            metrics.WorkflowExecutionPanicsTotal.Inc()
        }
    }()

    // Normal reconcile logic...

    // Catch unexpected errors at top level
    if err != nil && !isKnownError(err) {
        log.Error(err, "Unexpected error in reconcile",
            "request", req.NamespacedName)

        metrics.WorkflowExecutionUnexpectedErrorsTotal.Inc()

        // Long requeue to prevent tight error loops
        return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
    }

    return result, err
}
```

**Error Codes**:
| Code | Description |
|------|-------------|
| `InternalError` | Unexpected internal error |
| `PanicRecovered` | Recovered from panic |

---

## 🔄 Error Recovery Matrix

| Error Category | Auto-Retry | Max Attempts | Backoff | Manual Action Required |
|----------------|------------|--------------|---------|------------------------|
| A: Validation | ❌ No | 0 | N/A | Fix spec and resubmit |
| B: External Dependency | ✅ Yes | 5 | Exponential | Wait or check Tekton |
| C: Permission | ❌ No | 0 | N/A | Fix RBAC configuration |
| D: Execution | ❌ No | 0 | N/A | Review logs, cluster state |
| E: System | ✅ Yes | ∞ | Fixed 30s | Report bug if persistent |

---

## 📊 Error Metrics

```go
// pkg/workflowexecution/metrics/error_metrics.go
var (
    WorkflowExecutionErrorsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflowexecution_errors_total",
            Help: "Total errors by category and reason",
        },
        []string{"category", "reason"},
    )

    WorkflowExecutionRetryTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflowexecution_retry_total",
            Help: "Total retry attempts by reason",
        },
        []string{"reason"},
    )

    WorkflowExecutionFailureRecoveryDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "workflowexecution_failure_recovery_duration_seconds",
            Help:    "Time from failure to next successful execution",
            Buckets: prometheus.ExponentialBuckets(1, 2, 10),
        },
    )
)
```

---

## 🧪 Error Testing Strategy

| Error Category | Test Type | Test File |
|----------------|-----------|-----------|
| A: Validation | Unit | `test/unit/workflowexecution/validation_test.go` |
| B: External Dependency | Unit + Integration | `test/unit/workflowexecution/retry_test.go` |
| C: Permission | Integration | `test/integration/workflowexecution/rbac_test.go` |
| D: Execution | Integration | `test/integration/workflowexecution/failure_test.go` |
| E: System | Unit | `test/unit/workflowexecution/panic_recovery_test.go` |

---

## References

- [Error Handling Philosophy Template](../../../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md#-error-handling-philosophy-template--v20)
- [DD-WE-001: Resource Locking Safety](../../../../architecture/decisions/DD-WE-001-resource-locking-safety.md)
- [crd-schema.md: FailureDetails](../crd-schema.md)

