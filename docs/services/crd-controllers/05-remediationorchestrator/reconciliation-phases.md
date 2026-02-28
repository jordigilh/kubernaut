## Reconciliation Architecture

### Phase Transitions

**Sequential Service CRD Creation Flow**:

```
Gateway creates RemediationRequest
         ↓
   (watch triggers)
         ↓
RemediationRequest creates RemediationProcessing
         ↓
   RemediationProcessing.status.phase = "completed"
         ↓
   (watch triggers)
         ↓
RemediationRequest creates AIAnalysis
         ↓
   AIAnalysis.status.phase = "completed"
         ↓
   (watch triggers)
         ↓
RemediationRequest creates WorkflowExecution
         ↓
   WorkflowExecution.status.phase = "completed"
         ↓
   (watch triggers)
         ↓
RemediationRequest creates KubernetesExecution (DEPRECATED - ADR-025)
         ↓
   KubernetesExecution.status.phase = "completed" (DEPRECATED - ADR-025)
         ↓
   (watch triggers)
         ↓
RemediationRequest.status.overallPhase = "completed"
         ↓
   (24-hour retention begins)

   === ALTERNATIVE PATH A: WE Resource Lock - DUPLICATE (DD-RO-001) ===
RemediationRequest creates WorkflowExecution
         ↓
   WorkflowExecution.status.phase = "Skipped"
     (ResourceBusy OR RecentlyRemediated)
         ↓
   (watch triggers)
         ↓
RemediationRequest.status.overallPhase = "Skipped"
RemediationRequest.status.skipReason = "ResourceBusy" | "RecentlyRemediated"
RemediationRequest.status.duplicateOf = "parent-rr-name"
         ↓
   (Parent RR tracks duplicates, bulk notification on completion)
   (Requeue: ResourceBusy=30s, RecentlyRemediated=NextAllowedExecution)

   === ALTERNATIVE PATH B: WE Failure - MANUAL REVIEW (DD-WE-004) ===
RemediationRequest creates WorkflowExecution
         ↓
   WorkflowExecution.status.phase = "Skipped"
     (ExhaustedRetries OR PreviousExecutionFailed)
         ↓
   (watch triggers)
         ↓
RemediationRequest.status.overallPhase = "Failed"  ← NOT Skipped
RemediationRequest.status.skipReason = "ExhaustedRetries" | "PreviousExecutionFailed"
RemediationRequest.status.requiresManualReview = true
RemediationRequest.status.duplicateOf = ""  ← NOT a duplicate
         ↓
   (Individual escalation notification created)
   (NO requeue - manual intervention required)

   === ALTERNATIVE PATH C: WE Execution Failure ===
RemediationRequest creates WorkflowExecution
         ↓
   WorkflowExecution.status.phase = "Failed"
   WorkflowExecution.status.failureDetails.wasExecutionFailure = true
         ↓
   (watch triggers)
         ↓
RemediationRequest.status.overallPhase = "Failed"
RemediationRequest.status.requiresManualReview = true
RemediationRequest.status.message = failureDetails.naturalLanguageSummary
         ↓
   (Individual escalation notification with naturalLanguageSummary)
   (NO requeue - cluster state may be inconsistent)
```

**Overall Phase States**:
- `pending` → `processing` → `analyzing` → `executing` → `completed` / `failed` / `timeout` / `skipped`
- `failed` → `recovering` → `analyzing` → `executing` → `completed` / `failed`

**Note on Skipped vs Failed**:
- `skipped` is for **duplicates** (`ResourceBusy`, `RecentlyRemediated`) - requeued, tracked on parent
- `failed` is for **manual review required** (`ExhaustedRetries`, `PreviousExecutionFailed`, execution failures) - no requeue

**Reference**: DD-RO-001 (Resource Lock Deduplication), DD-WE-004 (Exponential Backoff)

### Reconciliation Flow

#### 1. **pending** Phase (Initial State)

**Purpose**: RemediationRequest CRD created by Gateway Service, awaiting controller reconciliation

**Trigger**: Gateway Service creates RemediationRequest CRD with original alert payload

**Actions**:
- Validate RemediationRequest spec (fingerprint, payload, severity)
- Initialize status fields
- Transition to `processing` phase
- **Create SignalProcessing CRD** with data snapshot

**Transition Criteria**:
```go
if alertRemediation.Spec.AlertFingerprint != "" && alertRemediation.Spec.OriginalPayload != nil {
    phase = "processing"
    // Create SignalProcessing CRD
    createRemediationProcessing(ctx, alertRemediation)
} else {
    phase = "failed"
    reason = "invalid_alert_data"
}
```

**Timeout**: 30 seconds (initialization should be immediate)

---

#### 2. **processing** Phase (Alert Enrichment & Classification)

**Purpose**: Wait for SignalProcessing CRD completion, then create AIAnalysis CRD

**Trigger**: RemediationProcessing.status.phase = "completed" (watch event)

**Actions**:
- **Watch** SignalProcessing CRD status
- When `status.phase = "completed"`:
  - Extract enriched alert data from RemediationProcessing.status
  - **Create AIAnalysis CRD** with data snapshot (enriched context)
  - Transition to `analyzing` phase
- **Timeout Detection**: If RemediationProcessing exceeds timeout threshold, escalate

**Transition Criteria**:
```go
if alertProcessing.Status.Phase == "completed" {
    phase = "analyzing"
    // Copy enriched data and create AIAnalysis CRD
    createAIAnalysis(ctx, alertRemediation, alertProcessing.Status)
} else if alertProcessing.Status.Phase == "failed" {
    phase = "failed"
    reason = "alert_processing_failed"
} else if timeoutExceeded(alertProcessing) {
    phase = "timeout"
    escalate("alert_processing_timeout")
}
```

**Timeout**: 5 minutes (default for Alert Processing phase)

**Watch Pattern**:
```go
// In controller setup
err = c.Watch(
    &source.Kind{Type: &processingv1.RemediationProcessing{}},
    handler.EnqueueRequestsFromMapFunc(r.alertProcessingToRemediation),
)
```

---

#### 3. **analyzing** Phase (AI Analysis & Recommendations)

**Purpose**: Wait for AIAnalysis CRD completion, then create WorkflowExecution CRD

**Trigger**: AIAnalysis.status.phase = "completed" (watch event)

**Actions**:
- **Watch** AIAnalysis CRD status
- When `status.phase = "completed"`:
  - Extract AI recommendations from AIAnalysis.status
  - **Create WorkflowExecution CRD** with data snapshot (recommendations, workflow steps)
  - Transition to `executing` phase
- **Timeout Detection**: If AIAnalysis exceeds timeout threshold, escalate

**Transition Criteria**:
```go
if aiAnalysis.Status.Phase == "completed" {
    phase = "executing"
    // Copy recommendations and create WorkflowExecution CRD
    createWorkflowExecution(ctx, alertRemediation, aiAnalysis.Status)
} else if aiAnalysis.Status.Phase == "failed" {
    phase = "failed"
    reason = "ai_analysis_failed"
} else if timeoutExceeded(aiAnalysis) {
    phase = "timeout"
    escalate("ai_analysis_timeout")
}
```

**Timeout**: 10 minutes (default for AI Analysis phase - HolmesGPT investigation can be long-running)

**Watch Pattern**:
```go
err = c.Watch(
    &source.Kind{Type: &aiv1.AIAnalysis{}},
    handler.EnqueueRequestsFromMapFunc(r.aiAnalysisToRemediation),
)
```

---

#### 4. **executing** Phase (Workflow Execution & Kubernetes Operations)

**Purpose**: Wait for WorkflowExecution CRD completion, then create KubernetesExecution (DEPRECATED - ADR-025) CRD

**Trigger**: WorkflowExecution.status.phase = "completed" (watch event)

**Actions**:
- **Watch** WorkflowExecution CRD status
- When `status.phase = "completed"`:
  - Extract workflow results from WorkflowExecution.status
  - **Create KubernetesExecution (DEPRECATED - ADR-025) CRD** with data snapshot (operations to execute)
  - Wait for KubernetesExecution completion
- **Timeout Detection**: If WorkflowExecution exceeds timeout threshold, escalate

**Transition Criteria**:
```go
switch workflowExecution.Status.Phase {
case "Completed":
    // Create KubernetesExecution (DEPRECATED - ADR-025) CRD
    createKubernetesExecution(ctx, alertRemediation, workflowExecution.Status)

    // Wait for KubernetesExecution to complete before final transition
    if kubernetesExecution.Status.Phase == "completed" {
        phase = "completed"
        completionTime = metav1.Now()
    }

case "Skipped":
    // DD-RO-001: Handle resource lock deduplication (BR-ORCH-032)
    handleWorkflowExecutionSkipped(ctx, alertRemediation, workflowExecution)
    phase = "Skipped"
    // Note: Individual notification deferred to bulk notification on parent completion

case "Failed":
    phase = "failed"
    reason = "workflow_execution_failed"

default:
    if timeoutExceeded(workflowExecution) {
        phase = "timeout"
        escalate("workflow_execution_timeout")
    }
}
```

**Timeout**: 30 minutes (default for Workflow + Kubernetes Execution phases)

**Watch Patterns**:
```go
// Watch WorkflowExecution
err = c.Watch(
    &source.Kind{Type: &workflowv1.WorkflowExecution{}},
    handler.EnqueueRequestsFromMapFunc(r.workflowExecutionToRemediation),
)

// Watch KubernetesExecution (DEPRECATED - ADR-025)
err = c.Watch(
    &source.Kind{Type: &executorv1.KubernetesExecution{}},
    handler.EnqueueRequestsFromMapFunc(r.kubernetesExecutionToRemediation),
)
```

---

#### 5. **completed** Phase (Terminal State - Success)

**Business Requirements**: BR-ORCH-045 (Completion Notification), BR-ORCH-034 (Bulk Duplicate Notification)

**Purpose**: All service CRDs completed successfully, notify operators, begin 24-hour retention

**Actions**:
- Record completion timestamp
- Emit Kubernetes event: `RemediationCompleted`
- Record audit trail to PostgreSQL
- **Create completion NotificationRequest** (BR-ORCH-045): Notify operators of successful remediation with signal name, root cause, workflow executed, duration, and outcome
- **Create bulk duplicate NotificationRequest** (BR-ORCH-034): If `DuplicateCount > 0`, notify operators of suppressed duplicate remediations
- **Start 24-hour retention timer** (finalizer prevents immediate deletion)
- After 24 hours: Remove finalizer and allow garbage collection

**Completion Notification** (BR-ORCH-045):
```yaml
kind: NotificationRequest
spec:
  type: "completion"
  priority: "low"
  subject: "Remediation Completed: {signalName}"
  body: |
    Remediation Completed Successfully

    Signal: {signalName}
    Severity: {severity}

    Root Cause Analysis:
    {rootCauseAnalysis}

    Workflow Executed: {workflowId}
    Duration: {duration}
    Outcome: {outcome}
  channels: [slack, file]
  metadata:
    remediationRequest: "{rrName}"
    workflowId: "{workflowId}"
    rootCause: "{rootCauseAnalysis}"
    duration: "{duration}"
    outcome: "{outcome}"
```

**Cleanup Process**:
```go
// Finalizer pattern for 24-hour retention
const remediationFinalizerName = "kubernaut.ai/remediation-retention"

func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var remediation remediationv1.RemediationRequest

    // Check if being deleted
    if !remediation.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(&remediation, remediationFinalizerName) {
            // Perform cleanup
            if err := r.finalizeRemediation(ctx, &remediation); err != nil {
                return ctrl.Result{}, err
            }

            // Remove finalizer
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

    // Check for 24-hour retention expiry
    if remediation.Status.OverallPhase == "completed" {
        retentionExpiry := remediation.Status.CompletionTime.Add(24 * time.Hour)
        if time.Now().After(retentionExpiry) {
            // Delete CRD (finalizer cleanup will be triggered)
            return ctrl.Result{}, r.Delete(ctx, &remediation)
        }

        // Requeue to check expiry later
        requeueAfter := time.Until(retentionExpiry)
        return ctrl.Result{RequeueAfter: requeueAfter}, nil
    }

    // Continue reconciliation...
}
```

**No Timeout** (terminal state)

**Cascade Deletion**: All service CRDs (RemediationProcessing, AIAnalysis, WorkflowExecution, KubernetesExecution (DEPRECATED - ADR-025)) are deleted automatically via owner references.

---

#### 6. **failed** Phase (Terminal State - Failure)

**Purpose**: One or more service CRDs failed, record failure and begin retention

**Actions**:
- Record failure timestamp and reason
- Emit Kubernetes event: `RemediationFailed` with failure details
- Record failure audit to PostgreSQL
- **Start 24-hour retention timer** (same as completed)
- Trigger notification via Notification Service

**No Requeue** (terminal state - requires manual intervention or alert retry)

---

#### 7. **timeout** Phase (Terminal State - Timeout)

**Purpose**: Service CRD exceeded timeout threshold, escalate and record

**Actions**:
- Record timeout timestamp and phase that timed out
- Emit Kubernetes event: `RemediationTimeout`
- **Escalate** via Notification Service (severity-based channels)
- Record timeout audit to PostgreSQL
- **Start 24-hour retention timer**

**Escalation Criteria** (BR-SP-062 (RemediationProcessor)):

| Phase | Default Timeout | Escalation Channel |
|-------|----------------|-------------------|
| **Alert Processing** | 5 minutes | Slack: #platform-ops |
| **AI Analysis** | 10 minutes | Slack: #ai-team, Email: ai-oncall |
| **Workflow Execution** | 20 minutes | Slack: #sre-team |
| **Kubernetes Execution** (DEPRECATED - ADR-025) | 10 minutes | Slack: #platform-oncall, PagerDuty |
| **Overall Workflow** | 1 hour | Slack: #incident-response, PagerDuty: P1 |

**No Requeue** (terminal state)

---

#### 8. **skipped** Phase (Terminal State - Duplicate/Resource Lock)

**Business Requirements**: BR-ORCH-032, BR-ORCH-033, BR-ORCH-034
**Design Decision Reference**: DD-RO-001 (Resource Lock Deduplication Handling)

**Purpose**: Remediation was skipped because WorkflowExecution detected a resource lock (another workflow executing on the same target resource).

**Trigger**: WorkflowExecution.status.phase = "Skipped" (with skipDetails)

**Actions**:
- Mark RemediationRequest as skipped with reason
- Track relationship to parent (first/active) remediation
- Update parent RR's duplicate tracking (count, refs)
- **NO individual notification** (bulk notification on parent completion per BR-ORCH-034)

**Skip Reasons** (from WorkflowExecution):
| Reason | Description | Parent Reference |
|--------|-------------|------------------|
| `ResourceBusy` | Another workflow executing on same target | `skipDetails.conflictingWorkflow.remediationRequestRef` |
| `RecentlyRemediated` | Target resource recently remediated (cooldown) | `skipDetails.recentRemediation.remediationRequestRef` |

**Status Update**:
```go
// Update skipped RR status
remediation.Status.OverallPhase = "Skipped"
remediation.Status.SkipReason = we.Status.SkipDetails.Reason    // "ResourceBusy" | "RecentlyRemediated"
remediation.Status.DuplicateOf = parentRRName                    // Reference to parent RR
remediation.Status.Message = fmt.Sprintf("Skipped: %s - see %s", skipReason, parentRRName)

// Update parent RR's duplicate tracking
parentRR.Status.DuplicateCount++
parentRR.Status.DuplicateRefs = append(parentRR.Status.DuplicateRefs, duplicateRRName)
```

**Bulk Notification on Parent Completion** (BR-ORCH-034):
When the parent (first/active) RemediationRequest completes (success OR failure):
```yaml
kind: NotificationRequest
spec:
  eventType: "RemediationCompleted"
  subject: "Remediation Completed: {workflowId}"
  body: |
    Target: {targetResource}
    Result: ✅ Successful / ❌ Failed
    Duration: {duration}

    Duplicates Suppressed: {duplicateCount}
    ├─ ResourceBusy: {resourceBusyCount}
    └─ RecentlyRemediated: {recentlyRemediatedCount}
  metadata:
    duplicateCount: "{N}"
    duplicateRefs: ["rr-002", "rr-003", ...]
```

**No Requeue** (terminal state)

**Note**: This differs from Gateway-level deduplication. Gateway deduplicates signals with the same fingerprint. WE resource locking is a safety net for different fingerprints targeting the same resource (Layer 3 per DD-RO-001).

---

## Phase 3.5: Approval Notification Triggering (V1.0 - BR-ORCH-001)

**Business Requirement**: BR-ORCH-001 (RemediationOrchestrator Notification Creation)
**ADR Reference**: ADR-018 (Approval Notification V1.0 Integration)

**Trigger**: AIAnalysis.status.phase == "Approving"

**Purpose**: Create NotificationRequest CRD to notify operators when AIAnalysis requires manual approval (medium confidence 60-79%), reducing approval miss rate from 40-60% to <5%.

### Watch Configuration

RemediationOrchestrator watches AIAnalysis CRD for status changes:

```go
func (r *RemediationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&remediationv1alpha1.RemediationRequest{}).
        Watches(
            &source.Kind{Type: &aianalysisv1alpha1.AIAnalysis{}},
            handler.EnqueueRequestsFromMapFunc(r.findRemediationRequestsForAIAnalysis),
        ).
        Complete(r)
}
```

### Reconciliation Logic

**Step 1: Detect Approval Requirement**
1. Fetch AIAnalysis CRD referenced by `RemediationRequest.status.aiAnalysisRef`
2. Check if `AIAnalysis.status.phase == "Approving"`
3. Check if `RemediationRequest.status.approvalNotificationSent == false` (idempotency)

**Step 2: Create NotificationRequest CRD** (if both conditions met)
1. Extract approval context from `AIAnalysis.status.approvalContext`:
   - Investigation summary
   - Evidence collected
   - Recommended actions with rationales
   - Alternatives considered with pros/cons
   - Why approval is required
2. Create `NotificationRequest` CRD:
   - **Name**: `approval-notification-{remediationRequest}-{aiAnalysis}`
   - **Subject**: `"🚨 Approval Required: {reason}"`
   - **Body**: Formatted approval context (investigation summary, evidence, actions, alternatives)
   - **Priority**: High
   - **Channels**: Slack (#kubernaut-approvals), Console
   - **Metadata**: RemediationRequest name, AIAnalysis name, AIApprovalRequest name, confidence score
   - **OwnerReference**: RemediationRequest (for cascade deletion)
3. Set `RemediationRequest.status.approvalNotificationSent = true`

**Step 3: Notification Delivery**
- Notification Service watches NotificationRequest CRD
- Delivers formatted notification to Slack/Console
- Operators receive push notification with approval context

### Idempotency Pattern

The `approvalNotificationSent` flag ensures single notification per approval request:

```go
if aiAnalysis.Status.Phase == "Approving" && !remediation.Status.ApprovalNotificationSent {
    // Create notification
    createApprovalNotification(ctx, remediation, aiAnalysis)

    // Mark as sent (prevents duplicates on reconciliation retries)
    remediation.Status.ApprovalNotificationSent = true
    r.Status().Update(ctx, remediation)
}
```

**Why Needed**: RemediationOrchestrator may reconcile multiple times while AIAnalysis is in "Approving" phase (status updates, watch triggers, etc.). Without idempotency flag, this would create duplicate notifications.

### Performance Metrics

- **CRD Watch Latency**: <500ms from AIAnalysis status update to RemediationOrchestrator reconciliation
- **Notification Creation Time**: <2 seconds from approval phase detection to NotificationRequest creation
- **End-to-End Latency**: <5 seconds from AIAnalysis "Approving" to operator notification delivery
- **Approval Miss Rate**: Reduced from 40-60% (manual polling) to <5% (push notifications)

### Business Value

**Without Approval Notifications** (V0):
- Operators must manually poll: `kubectl get aiapprovalrequest --watch`
- 40-60% approval miss rate (operators miss pending approvals)
- 30-40% timeout rate (15-minute default approval timeout)
- MTTR degradation: 60+ minutes for manual intervention

**With Approval Notifications** (V1.0):
- Push notifications to Slack/Console (no polling required)
- <5% approval miss rate (operators receive immediate alerts)
- <10% timeout rate (operators notified promptly)
- MTTR improvement: 5 minutes average for approval-required incidents
- **Cost savings**: $392K per approval-required incident (large enterprise, $7K/min downtime cost)

---

