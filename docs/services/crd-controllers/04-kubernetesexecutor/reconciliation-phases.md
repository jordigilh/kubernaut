## Reconciliation Architecture

> **DEPRECATED**: KubernetesExecution CRD and KubernetesExecutor service were eliminated by ADR-025 and replaced by Tekton TaskRun via WorkflowExecution. This documentation is retained for historical reference only. API types and CRD manifests have been removed from the codebase.

### Phase Transitions

**Multi-Phase Asynchronous Processing** (Job-based execution requires async handling):

```
"" (new) → validating → validated → waiting_approval → executing → rollback_ready → completed/failed
              ↓            ↓            ↓                  ↓             ↓
          (5-30s)      (instant)    (0s-∞)            (1-15min)     (1-5min)
```

**Rationale**: Kubernetes Job execution is inherently asynchronous:
- Job creation: ~500ms
- Pod scheduling: ~2-5s
- Action execution: ~10s-15min (depends on action type)
- Job cleanup: ~5-10s
- **Total**: Variable based on action complexity

### Reconciliation Flow

#### 1. **validating** Phase (BR-EXEC-001 to BR-EXEC-015, BR-SAFETY-001 to BR-SAFETY-015)

**Purpose**: Comprehensive validation before any real execution

**Actions** (executed synchronously):

**Step 1: Parameter Validation** (BR-EXEC-001 to BR-EXEC-005)
- Validate action type is predefined and supported
- Validate required parameters present and type-safe
- Validate target resource identifiers (namespace, name, kind)
- Check action-specific parameter constraints

**Step 2: RBAC Validation** (BR-SECURITY-010 to BR-SECURITY-015)
- Verify ServiceAccount exists for action type
- Check ServiceAccount has required permissions (dry-run auth check)
- Validate namespace access permissions

**Step 3: Resource Existence Check** (BR-EXEC-010)
- Verify target resource exists in cluster
- Check resource is in expected state for action
- Validate resource is not protected (e.g., kube-system namespace restrictions)

**Step 4: Rego Policy Validation** (BR-REGO-001 to BR-REGO-010, BR-SAFETY-010 to BR-SAFETY-020)
- Load Rego policy from ConfigMap
- Evaluate policy against action request
- Check safety constraints (environment, resource type, action type)
- Determine if dry-run is required for this action

**Step 5: Dry-Run Execution** (BR-SAFETY-015, if required by policy)
- Create validation Job with `--dry-run=server` flag
- Wait for Job completion (timeout: 30s)
- Parse dry-run results and potential impacts
- Detect any validation errors or warnings

**Step 6: Precondition Evaluation** (BR-EXEC-016) [NEW - DD-002]
- Evaluate all `spec.preConditions[]` using Rego policy engine
- Query current cluster state for condition input (e.g., deployment status, node capacity, RBAC permissions)
- For each precondition:
  - Execute Rego policy with cluster state as input
  - If `condition.required=true` and evaluation fails: Block Job creation, mark execution as "failed", update `status.validationResults.preConditionResults`, do NOT proceed to Job creation
  - If `condition.required=false` and evaluation fails: Log warning, update `status.validationResults.preConditionResults`, continue execution
- Wait up to `condition.timeout` for async precondition checks
- Record all precondition results in `status.validationResults.preConditionResults[]`

**Transition Criteria**:
```go
if allValidationsPassed && dryRunSuccessful {
    phase = "validated"
} else if validationFailed {
    phase = "failed"
    reason = "validation_error"
} else if dryRunFailed {
    phase = "failed"
    reason = "dry_run_failed"
}
```

**Example Status Update**:
```yaml
status:
  phase: validated
  validationResults:
    parameterValidation: true
    rbacValidation: true
    resourceExists: true
    policyValidation:
      policyName: "safety-policy-v1"
      allowed: true
      requiredApproval: true  # Policy determined manual approval needed
    dryRunResults:
      performed: true
      success: true
      estimatedImpact:
        resourcesAffected: 3
        replicasChanged: "3 -> 5"
      warnings: []
  validationTime: "2025-01-15T10:05:23Z"
```

#### 2. **validated** Phase (Approval Gate)

**Purpose**: Wait for approval if required by policy

**Actions**:
- Check if approval is required (from Rego policy evaluation)
- If no approval needed: transition immediately to `executing`
- If approval required: wait for `spec.approvalReceived = true`
- Monitor approval timeout (default: 1 hour, configurable)

**Transition Criteria**:
```go
if !requiresApproval || spec.approvalReceived {
    phase = "executing"
} else if approvalTimedOut {
    phase = "failed"
    reason = "approval_timeout"
}
```

#### 3. **executing** Phase (BR-EXEC-020 to BR-EXEC-035)

**Purpose**: Execute action through Kubernetes Job

**Actions**:

**Step 1: ServiceAccount Selection** (BR-SECURITY-012)
- Select appropriate ServiceAccount for action type
- Examples: `scale-deployment-sa`, `restart-pod-sa`, `patch-configmap-sa`

**Step 2: Job Creation** (BR-EXEC-025)
```go
job := &batchv1.Job{
    ObjectMeta: metav1.ObjectMeta{
        Name:      fmt.Sprintf("exec-%s-%s", ke.Spec.Action, ke.Name),
        Namespace: "kubernaut-executor",
        Labels: map[string]string{
            "kubernaut.io/execution-id": ke.Name,
            "kubernaut.io/action":       ke.Spec.Action,
        },
    },
    Spec: batchv1.JobSpec{
        TTLSecondsAfterFinished: ptr.To(int32(300)), // 5min cleanup
        BackoffLimit:            ptr.To(int32(ke.Spec.MaxRetries)),
        ActiveDeadlineSeconds:   ptr.To(int64(ke.Spec.Timeout.Seconds())),
        Template: corev1.PodTemplateSpec{
            Spec: corev1.PodSpec{
                ServiceAccountName: getServiceAccountForAction(ke.Spec.Action),
                RestartPolicy:      corev1.RestartPolicyNever,
                Containers: []corev1.Container{
                    {
                        Name:  "kubectl-executor",
                        Image: "bitnami/kubectl:1.28",
                        Command: buildActionCommand(ke.Spec.Action, ke.Spec.Parameters),
                        Env: []corev1.EnvVar{
                            {Name: "TARGET_NAMESPACE", Value: ke.Spec.TargetNamespace},
                            {Name: "RESOURCE_NAME", Value: ke.Spec.ResourceName},
                        },
                    },
                },
            },
        },
    },
}
```

**Step 3: Job Monitoring** (BR-EXEC-030)
- Watch Job status for completion
- Track Pod logs for execution details
- Monitor for timeout (action-specific timeout, 5m default)
- Handle Job failures and retries

**Step 4: Result Capture** (BR-EXEC-035)
- Parse Job completion status
- Extract execution results from Pod logs
- Record resources affected by action
- Calculate execution duration

**Step 5: Postcondition Verification** (BR-EXEC-036) [NEW - DD-002]
- After `Job.status.succeeded = 1`, evaluate all `spec.postConditions[]`
- Query cluster state to validate intended outcome achieved
- For each postcondition:
  - Execute Rego policy with post-execution cluster state as input
  - Wait up to `condition.timeout` for async verification (e.g., pods starting, deployment Available=true)
  - If `condition.required=true` and verification fails: Mark execution as "failed", update `status.validationResults.postConditionResults`, capture rollback information, transition to "rollback_ready"
  - If `condition.required=false` and verification fails: Log warning, update `status.validationResults.postConditionResults`, mark as partial success
- Record all postcondition results in `status.validationResults.postConditionResults[]`
- If any required postcondition fails, populate `status.rollbackInformation` with details for WorkflowExecution to use

**Transition Criteria**:
```go
if jobSuccessful {
    phase = "rollback_ready"  // or "completed" if no rollback needed
    captureExecutionResults()
} else if jobFailed && retriesExhausted {
    phase = "failed"
    reason = "execution_failed"
} else if executionTimedOut {
    phase = "failed"
    reason = "execution_timeout"
}
```

**Example Status Update**:
```yaml
status:
  phase: rollback_ready
  executionResults:
    success: true
    jobName: "exec-scale-deployment-abc123"
    startTime: "2025-01-15T10:10:00Z"
    endTime: "2025-01-15T10:10:15Z"
    duration: "15s"
    resourcesAffected:
      - kind: "Deployment"
        namespace: "production"
        name: "webapp"
        action: "scaled"
        before: "replicas: 3"
        after: "replicas: 5"
    podLogs: |
      deployment.apps/webapp scaled
    retriesAttempted: 0
```

#### 4. **rollback_ready** Phase (Terminal State with Rollback Capability)

**Purpose**: Execution complete, rollback information preserved

**Actions**:
- Record rollback parameters to CRD status
- Store execution results for audit
- Emit Kubernetes event: `ExecutionCompleted`
- Wait for WorkflowExecution controller to mark as final

**Rollback Capability**:
```yaml
status:
  phase: rollback_ready
  rollbackInformation:
    available: true
    rollbackAction: "scale_deployment"
    rollbackParameters:
      deployment: "webapp"
      namespace: "production"
      replicas: 3  # Original value before action
    estimatedRollbackDuration: "10s"
```

**Note**: Rollback is NOT automatic. WorkflowExecution controller decides if/when to trigger rollback.

#### 5. **completed** Phase (Terminal State - Success)

**Purpose**: Final success state, no rollback needed

**Actions**:
- Record execution to audit database
- Emit Kubernetes event: `ExecutionCompletedPermanent`
- Clean up Job resources (via TTL)

**No Timeout** (terminal state)

#### 6. **failed** Phase (Terminal State - Failure)

**Purpose**: Record failure for debugging and retry at workflow level

**Actions**:
- Log failure reason and context
- Emit Kubernetes event: `ExecutionFailed`
- Record failure to audit database
- Preserve Job for debugging (TTL still applies)

**No Requeue** (terminal state - WorkflowExecution decides retry strategy)

---

### CRD-Based Coordination Patterns

#### Event-Driven Coordination

This service uses **CRD-based reconciliation** for coordination with RemediationRequest controller:

1. **Created By**: RemediationRequest controller creates KubernetesExecution CRD (with owner reference)
2. **Watch Pattern (Upstream)**: RemediationRequest watches KubernetesExecution status for completion
3. **Watch Pattern (Downstream)**: KubernetesExecution watches Kubernetes Jobs for action execution
4. **Status Propagation**: Status updates trigger RemediationRequest reconciliation automatically (<1s latency)
5. **Event Emission**: Emit Kubernetes events for operational visibility

**Coordination Flow (Two Layers)**:
```
Layer 1: RemediationRequest → KubernetesExecution
    RemediationRequest.status.overallPhase = "executing"
        ↓
    RemediationRequest Controller creates KubernetesExecution CRD
        ↓
    KubernetesExecution Controller reconciles (this controller)
        ↓
    KubernetesExecution.status.phase = "completed"
        ↓ (watch trigger in RemediationRequest)
    RemediationRequest Controller reconciles (detects completion)
        ↓
    RemediationRequest.status.overallPhase = "completed"

Layer 2: KubernetesExecution → Kubernetes Jobs (Native Resources)
    KubernetesExecution.status.phase = "validating"
        ↓ (validation passes)
    KubernetesExecution Controller creates Kubernetes Job (owned)
        ↓ (Job executes kubectl/oc with per-action RBAC)
    Job.status.succeeded = 1
        ↓ (watch trigger in KubernetesExecution)
    KubernetesExecution Controller reconciles
        ↓
    KubernetesExecution.status.phase = "completed"
```

---

#### Owner Reference Management

**This CRD (KubernetesExecution)**:
- **Owned By**: RemediationRequest (parent CRD)
- **Owner Reference**: Set at creation by RemediationRequest controller
- **Cascade Deletion**: Deleted automatically when RemediationRequest is deleted
- **Owns**: Kubernetes Jobs (native resources for action execution)
- **Watches**: Kubernetes Jobs (for action completion status)

**Job-Based Execution Pattern**:

KubernetesExecution is a **leaf controller with native resource coordination**:
- ✅ **CRD Layer**: Owned by RemediationRequest (no service CRD children)
- ✅ **Native Resource Layer**: Creates Kubernetes Jobs for action execution
- ✅ **Watch Jobs**: Event-driven Job status monitoring
- ✅ **RBAC Isolation**: Per-action ServiceAccount in each Job

**Lifecycle**:
```
RemediationRequest Controller
    ↓ (creates with owner reference)
KubernetesExecution CRD
    ↓ (validates actions)
KubernetesExecution Controller creates Kubernetes Job (owned)
    ↓ (Job executes: kubectl scale deployment/web-app --replicas=5)
Job.status.succeeded = 1
    ↓ (watch trigger)
KubernetesExecution.status.phase = "completed"
    ↓ (watch trigger in RemediationRequest)
RemediationRequest Controller (remediation complete)
```

---

#### Native Resource Coordination (Kubernetes Jobs)

**Special Pattern**: KubernetesExecution creates **native Kubernetes resources** (Jobs), not CRDs

**Why Kubernetes Jobs**:
- **Process Isolation**: Each action runs in separate container
- **RBAC Isolation**: Each Job uses action-specific ServiceAccount
- **Resource Limits**: CPU/memory limits per action
- **TTL Cleanup**: Jobs auto-deleted after completion (ttlSecondsAfterFinished: 300)
- **Retry Logic**: Kubernetes built-in backoff for failed Jobs

**Job Creation Pattern**:
```go
// In KubernetesExecutionReconciler
func (r *KubernetesExecutionReconciler) createJobForAction(
    ctx context.Context,
    execution *kubernetesexecutionv1.KubernetesExecution,
    action Action,
) error {
    job := &batchv1.Job{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-action-%d", execution.Name, action.Index),
            Namespace: execution.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(execution,
                    kubernetesexecutionv1.GroupVersion.WithKind("KubernetesExecution")),
            },
        },
        Spec: batchv1.JobSpec{
            TTLSecondsAfterFinished: ptr.To[int32](300), // 5min cleanup
            Template: corev1.PodTemplateSpec{
                Spec: corev1.PodSpec{
                    ServiceAccountName: fmt.Sprintf("kubernaut-action-%s", action.Type),
                    Containers: []corev1.Container{
                        {
                            Name:  "kubectl",
                            Image: "bitnami/kubectl:1.28",
                            Command: []string{"kubectl"},
                            Args: action.CommandArgs, // e.g., ["scale", "deployment/web-app", "--replicas=5"]
                        },
                    },
                    RestartPolicy: corev1.RestartPolicyNever,
                },
            },
        },
    }

    return r.Create(ctx, job)
}
```

**Job Watch Pattern**:
```go
// In KubernetesExecutionReconciler.SetupWithManager()
err = c.Watch(
    &source.Kind{Type: &batchv1.Job{}},
    handler.EnqueueRequestsFromMapFunc(r.jobToExecution),
)

// Mapping function
func (r *KubernetesExecutionReconciler) jobToExecution(obj client.Object) []ctrl.Request {
    job := obj.(*batchv1.Job)

    // Extract KubernetesExecution from owner reference
    for _, owner := range job.GetOwnerReferences() {
        if owner.Kind == "KubernetesExecution" {
            return []ctrl.Request{
                {
                    NamespacedName: types.NamespacedName{
                        Name:      owner.Name,
                        Namespace: job.Namespace,
                    },
                },
            }
        }
    }
    return nil
}
```

**Result**: Job status changes trigger KubernetesExecution reconciliation within ~100ms.

---

#### No Direct HTTP Calls Between Controllers

**Anti-Pattern (Avoided)**: ❌ KubernetesExecution calling other controllers via HTTP

**Correct Pattern (Used)**: ✅ CRD status update + RemediationRequest watch-based coordination

**Why This Matters**:
- **Reliability**: CRD status persists in etcd (HTTP calls can fail silently)
- **Observability**: Status visible via `kubectl get kubernetesexecution` (HTTP calls are opaque)
- **Kubernetes-Native**: Leverages built-in watch/reconcile patterns (no custom HTTP infrastructure)
- **Decoupling**: KubernetesExecution doesn't need to know about other services
- **Job Coordination**: Native Kubernetes Job watching (no custom polling)

**What KubernetesExecution Does NOT Do**:
- ❌ Call RemediationRequest controller via HTTP
- ❌ Create other service CRDs (terminal service)
- ❌ Poll Job status (uses watches instead)
- ❌ Execute kubectl commands directly (delegates to Jobs)

**What KubernetesExecution DOES Do**:
- ✅ Process its own KubernetesExecution CRD
- ✅ Create Kubernetes Jobs for action execution
- ✅ Watch Job status changes via Kubernetes API
- ✅ Update its own status to "completed"
- ✅ Trust RemediationRequest to handle post-execution flow

---

#### Watch Configuration

**1. RemediationRequest Watches KubernetesExecution (Upstream)**:

```go
// In RemediationRequestReconciler.SetupWithManager()
err = c.Watch(
    &source.Kind{Type: &kubernetesexecutionv1.KubernetesExecution{}},
    handler.EnqueueRequestsFromMapFunc(r.kubernetesExecutionToRemediation),
)

// Mapping function
func (r *RemediationRequestReconciler) kubernetesExecutionToRemediation(obj client.Object) []ctrl.Request {
    exec := obj.(*kubernetesexecutionv1.KubernetesExecution)
    return []ctrl.Request{
        {
            NamespacedName: types.NamespacedName{
                Name:      exec.Spec.RemediationRequestRef.Name,
                Namespace: exec.Spec.RemediationRequestRef.Namespace,
            },
        },
    }
}
```

**2. KubernetesExecution Watches Jobs (Downstream)**:

See "Native Resource Coordination" section above for Job watch configuration.

**Result**: Bi-directional event propagation with ~100ms latency:
- RemediationRequest detects KubernetesExecution completion within ~100ms
- KubernetesExecution detects Job completion within ~100ms

---

#### Per-Action RBAC Isolation

**Unique Pattern**: Each action type uses a dedicated ServiceAccount with minimal RBAC

**Why Per-Action ServiceAccounts**:
- **Least Privilege**: Each action only gets permissions it needs
- **Audit Trail**: RBAC logs show which action performed operation
- **Blast Radius**: Compromised action limited to its own permissions
- **Security Compliance**: Defense-in-depth RBAC strategy

**Example RBAC Setup**:
```yaml
# ServiceAccount for scale action
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubernaut-action-scale
  namespace: kubernaut-system

---
# ClusterRole for scale action (minimal permissions)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut-action-scale
rules:
- apiGroups: ["apps"]
  resources: ["deployments", "statefulsets"]
  verbs: ["get", "patch"] # ONLY get and patch, not delete

---
# ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernaut-action-scale
subjects:
- kind: ServiceAccount
  name: kubernaut-action-scale
  namespace: kubernaut-system
roleRef:
  kind: ClusterRole
  name: kubernaut-action-scale
  apiGroup: rbac.authorization.k8s.io
```

**Job Uses Action ServiceAccount**:
```go
job.Spec.Template.Spec.ServiceAccountName = "kubernaut-action-scale"
```

**Security Benefits**:
- ✅ Each action isolated to specific resources and verbs
- ✅ No single ServiceAccount with cluster-admin
- ✅ Audit logs show `kubernaut-action-scale` performed the operation
- ✅ Kubernetes enforces RBAC at Job execution time

---

#### Coordination Benefits

**For KubernetesExecution Controller**:
- ✅ **Focused**: Only handles Kubernetes action execution
- ✅ **RBAC Isolation**: Per-action ServiceAccounts
- ✅ **Job Management**: Kubernetes handles retry and cleanup
- ✅ **Testable**: Unit tests mock Jobs, integration tests use real K8s

**For RemediationRequest Controller**:
- ✅ **Visibility**: Can query KubernetesExecution status anytime
- ✅ **Control**: Knows when remediation completes
- ✅ **Timeout Detection**: Can detect if execution takes too long
- ✅ **Completion Certainty**: Terminal state after KubernetesExecution

**For Operations**:
- ✅ **Debuggable**: `kubectl get kubernetesexecution -o yaml` shows execution state
- ✅ **Job Visibility**: `kubectl get jobs -l owner=kubernetesexecution` shows action Jobs
- ✅ **Observable**: Kubernetes events show action progress
- ✅ **Security Auditable**: RBAC logs show which action executed operations

---

