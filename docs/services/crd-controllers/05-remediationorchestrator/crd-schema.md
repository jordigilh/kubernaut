## CRD Schema

### 📋 Authoritative Schema Reference

**IMPORTANT**: The authoritative CRD schema is defined in [`docs/architecture/CRD_SCHEMAS.md`](../../../../architecture/CRD_SCHEMAS.md)

**Gateway Service is the source of truth** for `RemediationRequest` spec fields because:
- Gateway creates the CRD
- Gateway performs deduplication, priority assignment, storm detection
- Gateway has complete signal context

This document shows how Remediation Orchestrator **consumes** the CRD (what fields it expects and uses).

### RemediationRequest Spec (Gateway Creates)

Remediation Orchestrator expects Gateway to populate these fields:

```go
// pkg/apis/remediation/v1/remediationrequest_types.go
package v1

import (
    "encoding/json"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RemediationRequestSpec struct {
    // ========================================
    // UNIVERSAL FIELDS (ALL SIGNALS)
    // These fields are populated for EVERY signal regardless of provider
    // ========================================

    // Core Signal Identification (REQUIRED)
<<<<<<< HEAD
    AlertFingerprint string `json:"alertFingerprint"` // Unique fingerprint for deduplication
    AlertName        string `json:"alertName"`        // Human-readable signal name
=======
    SignalFingerprint string `json:"signalFingerprint"` // Unique fingerprint for deduplication
    SignalName        string `json:"signalName"`        // Human-readable signal name
>>>>>>> crd_implementation

    // Signal Classification (REQUIRED)
    Severity     string `json:"severity"`      // "critical", "warning", "info"
    Environment  string `json:"environment"`   // "prod", "staging", "dev"
    Priority     string `json:"priority"`      // P0/P1/P2 assigned by Gateway
    SignalType   string `json:"signalType"`    // "prometheus", "kubernetes-event", "aws-cloudwatch", etc.
    SignalSource string `json:"signalSource,omitempty"` // Adapter name (e.g., "prometheus-adapter")
    TargetType   string `json:"targetType"`    // "kubernetes", "aws", "azure", "gcp", "datadog"

    // Temporal Data (REQUIRED)
    FiringTime   metav1.Time `json:"firingTime"`   // When signal started firing
    ReceivedTime metav1.Time `json:"receivedTime"` // When Gateway received signal

    // Deduplication Metadata (REQUIRED)
    Deduplication DeduplicationInfo `json:"deduplication"`

    // Storm Detection (OPTIONAL)
    IsStorm         bool   `json:"isStorm,omitempty"`
    StormType       string `json:"stormType,omitempty"`       // "rate" or "pattern"
    StormWindow     string `json:"stormWindow,omitempty"`     // e.g., "5m"
    StormAlertCount int    `json:"stormAlertCount,omitempty"` // Number of alerts in storm

    // ========================================
    // PROVIDER-SPECIFIC DATA
    // All provider-specific fields (INCLUDING Kubernetes) go here
    // ========================================

    // Provider-specific fields in raw JSON format
    // Controllers unmarshal this based on targetType/signalType
    //
    // For Kubernetes (targetType="kubernetes"):
    //   {"namespace": "...", "resource": {"kind": "...", "name": "..."}, "alertmanagerURL": "...", "grafanaURL": "..."}
    //
    // For AWS (targetType="aws"):
    //   {"region": "...", "accountId": "...", "instanceId": "...", "resourceType": "..."}
    //
    // See docs/architecture/CRD_SCHEMAS.md for complete provider schemas
    ProviderData json.RawMessage `json:"providerData,omitempty"`

    // ========================================
    // AUDIT/DEBUG
    // ========================================

    // Complete original webhook payload for debugging and audit
    OriginalPayload []byte `json:"originalPayload,omitempty"`

    // ========================================
    // WORKFLOW CONFIGURATION
    // ========================================

    // Optional timeout overrides for this specific remediation
    // Remediation Orchestrator provides defaults if not specified
    TimeoutConfig *TimeoutConfig `json:"timeoutConfig,omitempty"`
}

// DeduplicationInfo tracks duplicate signal suppression
type DeduplicationInfo struct {
    IsDuplicate                   bool        `json:"isDuplicate"`
    FirstSeen                     metav1.Time `json:"firstSeen"`
    LastSeen                      metav1.Time `json:"lastSeen"`
    OccurrenceCount               int         `json:"occurrenceCount"`
    PreviousRemediationRequestRef string      `json:"previousRemediationRequestRef,omitempty"`
}

// TimeoutConfig allows per-remediation timeout customization
type TimeoutConfig struct {
    RemediationProcessingTimeout metav1.Duration `json:"remediationProcessingTimeout,omitempty"` // Default: 5m
    AIAnalysisTimeout            metav1.Duration `json:"aiAnalysisTimeout,omitempty"`            // Default: 10m
    WorkflowExecutionTimeout     metav1.Duration `json:"workflowExecutionTimeout,omitempty"`     // Default: 20m
    OverallWorkflowTimeout       metav1.Duration `json:"overallWorkflowTimeout,omitempty"`       // Default: 1h
}
```

### How Remediation Orchestrator Accesses Provider Data

Since Kubernetes-specific fields (namespace, resource, alertmanagerURL, grafanaURL) are now in `ProviderData`, Remediation Orchestrator must unmarshal them:

```go
// pkg/remediationorchestrator/reconcile.go
package remediationorchestrator

import (
    "context"
    "encoding/json"
    "fmt"

    remediationv1 "github.com/jordigilh/kubernaut/pkg/apis/remediation/v1"
    ctrl "sigs.k8s.io/controller-runtime"
)

func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var remediation remediationv1.RemediationRequest
    if err := r.Get(ctx, req.NamespacedName, &remediation); err != nil {
        return ctrl.Result{}, err
    }

    // Unmarshal Kubernetes provider data
    if remediation.Spec.TargetType == "kubernetes" {
        var k8sData remediationv1.KubernetesProviderData
        if err := json.Unmarshal(remediation.Spec.ProviderData, &k8sData); err != nil {
            return ctrl.Result{}, fmt.Errorf("failed to unmarshal K8s provider data: %w", err)
        }

        // Now you can access:
        // - k8sData.Namespace (string)
        // - k8sData.Resource (K8sResourceIdentifier with Kind, Name, Namespace)
        // - k8sData.AlertmanagerURL (string)
        // - k8sData.GrafanaURL (string)
        // - k8sData.PrometheusQuery (string)

        log.Info("Processing Kubernetes signal",
            "namespace", k8sData.Namespace,
            "resource", k8sData.Resource.Kind+"/"+k8sData.Resource.Name,
        )
    }

    // Continue with reconciliation...
    return ctrl.Result{}, nil
}
```

**Key Points**:
- ✅ No top-level `Namespace` or `Resource` fields - they're in `ProviderData`
- ✅ No top-level `AlertmanagerURL` or `GrafanaURL` - they're in `ProviderData`
- ✅ Always check `TargetType` before unmarshaling provider-specific data
- ✅ Use strongly-typed `KubernetesProviderData` struct for type safety

See [`docs/architecture/CRD_SCHEMAS.md`](../../../../architecture/CRD_SCHEMAS.md) for complete `KubernetesProviderData` schema.

---

### RemediationRequest Status

```go
// pkg/apis/remediation/v1/remediationrequest_types.go
package v1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RemediationRequestStatus struct {
    // Overall remediation state
    // UPDATED: Added "recovering" phase for failure recovery coordination
    OverallPhase string      `json:"overallPhase"` // pending, processing, analyzing, executing, recovering, completed, failed, timeout
    StartTime    metav1.Time `json:"startTime"`
    CompletionTime *metav1.Time `json:"completionTime,omitempty"`

    // ========================================
    // RECOVERY TRACKING (Phase 1 Critical Fix)
    // See: docs/architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md
    // ========================================

    // Recovery attempt tracking
    RecoveryAttempts        int         `json:"recoveryAttempts"`                  // Current recovery attempt count (0-3)
    MaxRecoveryAttempts     int         `json:"maxRecoveryAttempts"`               // Maximum allowed (default: 3)
    LastFailureTime         *metav1.Time `json:"lastFailureTime,omitempty"`        // Timestamp of most recent workflow failure
    EscalatedToManualReview bool        `json:"escalatedToManualReview"`           // True when recovery limits exceeded
    RecoveryReason          *string     `json:"recoveryReason,omitempty"`          // Why recovery was needed (e.g., "workflow_timeout", "step_failure")

    // ========================================
    // ALTERNATIVE 2: Multiple RemediationProcessing CRD references
    // See: docs/architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md (Alternative 2)
    // ========================================

    // RemediationProcessingRefs tracks ALL RemediationProcessing CRDs (initial + recovery attempts)
    RemediationProcessingRefs []RemediationProcessingReference `json:"remediationProcessingRefs,omitempty"` // Array: initial + recovery

    // CurrentProcessingRef points to the RemediationProcessing currently being processed
    // Used by orchestratePhase() to watch for completion
    CurrentProcessingRef *corev1.LocalObjectReference `json:"currentProcessingRef,omitempty"` // Current RP being watched

    // Multiple CRD references for recovery attempts (changed from single ref to array)
    AIAnalysisRefs               []AIAnalysisReference               `json:"aiAnalysisRefs,omitempty"`     // CHANGED: Array for initial + recovery analyses
    WorkflowExecutionRefs        []WorkflowExecutionReferenceWithOutcome `json:"workflowExecutionRefs,omitempty"` // CHANGED: Array with outcomes
    CurrentWorkflowExecutionRef  *string                             `json:"currentWorkflowExecutionRef,omitempty"` // Name of currently executing workflow

    // Legacy: Kept for backward compatibility, use arrays above for recovery tracking
    // DEPRECATED: Use remediationProcessingRefs, aiAnalysisRefs, and workflowExecutionRefs arrays instead
<<<<<<< HEAD
    RemediationProcessingRef *RemediationProcessingReference     `json:"alertProcessingRef,omitempty"` // Deprecated: Use remediationProcessingRefs[0]
=======
    RemediationProcessingRef *RemediationProcessingReference     `json:"remediationProcessingRef,omitempty"` // Deprecated: Use remediationProcessingRefs[0]
>>>>>>> crd_implementation
    AIAnalysisRef          *AIAnalysisReference          `json:"aiAnalysisRef,omitempty"`          // Deprecated: Use aiAnalysisRefs
    WorkflowExecutionRef   *WorkflowExecutionReference   `json:"workflowExecutionRef,omitempty"`   // Deprecated: Use workflowExecutionRefs
    KubernetesExecutionRef *KubernetesExecutionReference `json:"kubernetesExecutionRef,omitempty"` // Unchanged (single executor per workflow)

    // Aggregated status from service CRDs
<<<<<<< HEAD
    RemediationProcessingStatus     *RemediationProcessingStatusSummary     `json:"alertProcessingStatus,omitempty"`
=======
    RemediationProcessingStatus     *RemediationProcessingStatusSummary     `json:"remediationProcessingStatus,omitempty"`
>>>>>>> crd_implementation
    AIAnalysisStatus          *AIAnalysisStatusSummary          `json:"aiAnalysisStatus,omitempty"`
    WorkflowExecutionStatus   *WorkflowExecutionStatusSummary   `json:"workflowExecutionStatus,omitempty"`
    KubernetesExecutionStatus *KubernetesExecutionStatusSummary `json:"kubernetesExecutionStatus,omitempty"`

    // Timeout tracking
    TimeoutPhase *string      `json:"timeoutPhase,omitempty"` // Which phase timed out
    TimeoutTime  *metav1.Time `json:"timeoutTime,omitempty"`

    // Failure tracking
    FailurePhase  *string `json:"failurePhase,omitempty"`  // Which phase failed
    FailureReason *string `json:"failureReason,omitempty"` // Detailed failure reason

    // Retention tracking
    RetentionExpiryTime *metav1.Time `json:"retentionExpiryTime,omitempty"` // When 24h retention expires

    // Duplicate tracking (from Gateway Service)
    DuplicateCount int      `json:"duplicateCount"` // Number of duplicate alerts suppressed
    LastDuplicateTime *metav1.Time `json:"lastDuplicateTime,omitempty"`
}

// Reference types
// RemediationProcessingReference tracks RemediationProcessing CRD references
// Enhanced for Alternative 2 to track both initial and recovery attempts
type RemediationProcessingReference struct {
    Name          string      `json:"name"`
    Namespace     string      `json:"namespace"`
    Type          string      `json:"type,omitempty"`          // "initial" | "recovery" (Alternative 2)
    AttemptNumber int         `json:"attemptNumber,omitempty"` // 0 for initial, 1-3 for recovery (Alternative 2)
    Phase         string      `json:"phase,omitempty"`         // Current phase of this RP (Alternative 2)
    CreatedAt     metav1.Time `json:"createdAt,omitempty"`     // When this RP was created (Alternative 2)
}

type AIAnalysisReference struct {
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
}

type WorkflowExecutionReference struct {
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
}

// NEW: WorkflowExecutionReferenceWithOutcome tracks workflow execution attempts with outcomes
// Used in WorkflowExecutionRefs array for recovery tracking
type WorkflowExecutionReferenceWithOutcome struct {
    Name           string       `json:"name"`
    Namespace      string       `json:"namespace"`
    Outcome        string       `json:"outcome"`        // "in-progress", "completed", "failed"
    FailedStep     *int         `json:"failedStep,omitempty"`     // Which step failed (if outcome=failed)
    FailureReason  *string      `json:"failureReason,omitempty"`  // Why it failed (if outcome=failed)
    CompletionTime *metav1.Time `json:"completionTime,omitempty"` // When it finished
    AttemptNumber  int          `json:"attemptNumber"`  // 1 for initial, 2+ for recovery attempts
}

type KubernetesExecutionReference struct {
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
}

// Status summary types (lightweight aggregation)
type RemediationProcessingStatusSummary struct {
    Phase          string      `json:"phase"`
    CompletionTime *metav1.Time `json:"completionTime,omitempty"`
    Environment    string      `json:"environment,omitempty"`
    DegradedMode   bool        `json:"degradedMode"`
}

type AIAnalysisStatusSummary struct {
    Phase              string      `json:"phase"`
    CompletionTime     *metav1.Time `json:"completionTime,omitempty"`
    RecommendationCount int        `json:"recommendationCount"`
    TopRecommendation  string      `json:"topRecommendation,omitempty"`
}

type WorkflowExecutionStatusSummary struct {
    Phase          string      `json:"phase"`
    CompletionTime *metav1.Time `json:"completionTime,omitempty"`
    TotalSteps     int         `json:"totalSteps"`
    CompletedSteps int         `json:"completedSteps"`
}

type KubernetesExecutionStatusSummary struct {
    Phase           string      `json:"phase"`
    CompletionTime  *metav1.Time `json:"completionTime,omitempty"`
    OperationsTotal int         `json:"operationsTotal"`
    OperationsSuccess int       `json:"operationsSuccess"`
}
```

---

### ✅ **TYPE SAFETY COMPLIANCE**

This CRD specification uses **fully structured types** for all status aggregation and service CRD references:

| Type | Previous (Anti-Pattern) | Current (Type-Safe) | Benefit |
|------|------------------------|---------------------|---------|
| **RemediationProcessingStatusSummary** | `map[string]interface{}` | Structured type with phase, timestamp, environment | Compile-time safety for aggregation |
| **AIAnalysisStatusSummary** | `map[string]interface{}` | Structured type with phase, recommendation count | Type-safe AI status aggregation |
| **WorkflowExecutionStatusSummary** | `map[string]interface{}` | Structured type with step progress | Type-safe workflow status tracking |
| **KubernetesExecutionStatusSummary** | `map[string]interface{}` | Structured type with operation counts | Type-safe execution result aggregation |
| **Service CRD References** | `map[string]interface{}` | 4 structured reference types | Clear ownership and lifecycle management |

**Design Principle**: RemediationRequest aggregates status from 4 service CRDs. All aggregation uses lightweight structured types, not full data copies.

**Key Type-Safe Components**:
- ✅ All service CRD references use `corev1.ObjectReference` (Kubernetes-native type)
- ✅ Status summaries are lightweight structured types (not full service CRD status copies)
- ✅ No `map[string]interface{}` usage anywhere in aggregation logic
- ✅ Each service CRD provides its own type-safe status, RemediationRequest aggregates safely

**Type-Safe Aggregation Pattern**:
```go
// ✅ TYPE SAFE - Lightweight status aggregation
type RemediationProcessingStatusSummary struct {
    Phase          string       `json:"phase"`
    CompletionTime *metav1.Time `json:"completionTime,omitempty"`
    Environment    string       `json:"environment,omitempty"`
    DegradedMode   bool         `json:"degradedMode"`
}

// NOT this anti-pattern:
// ProcessingStatus map[string]interface{} `json:"processingStatus"` // ❌ WRONG
```

**Why Lightweight Summaries**:
- **Performance**: Don't copy entire service CRD status (can be large)
- **Clarity**: Only essential fields for coordination (phase, completion time)
- **Decoupling**: Service CRDs own their detailed status
- **Scalability**: RemediationRequest status stays small even with complex service CRDs

**Full Status Available When Needed**:
```go
// When RemediationRequest needs detailed status, it queries the service CRD:
var aiAnalysis aiv1.AIAnalysis
if err := r.Get(ctx, client.ObjectKey{
    Name:      remediation.Status.AIAnalysisRef.Name,
    Namespace: remediation.Status.AIAnalysisRef.Namespace,
}, &aiAnalysis); err != nil {
    return err
}

// Full status available here: aiAnalysis.Status
// Remediation only stores summary: remediation.Status.AIAnalysisStatus
```

**Related Documents**:
- `ALERT_PROCESSOR_TYPE_SAFETY_TRIAGE.md` - Original type safety remediation (archived)
- [Owner Reference Architecture](../../../architecture/decisions/005-owner-reference-architecture.md) - Service CRD lifecycle and references

---

### 🔄 **RECOVERY TRACKING & "recovering" PHASE**

**Status**: ✅ Phase 1 Critical Fix Complete (C2, C4)
**Reference**: [`PROPOSED_FAILURE_RECOVERY_SEQUENCE.md`](../../../architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md)

#### Phase Lifecycle with Recovery

```ascii
RemediationRequest Phase Progression:

  pending → processing → analyzing → executing → [SUCCESS] → completed ✅
                                         ↓
                                    [FAILURE]
                                         ↓
                                   recovering ← NEW PHASE
                                         ↓
                          [Recovery Success] → executing → completed ✅
                                         ↓
                      [Max Attempts/Pattern] → failed (escalate) ❌
```

#### Recovery Phase Trigger

The "recovering" phase is entered when:
1. WorkflowExecution CRD status changes to "failed"
2. Remediation Orchestrator watches this failure event
3. Recovery viability evaluation passes (see below)
4. New AIAnalysis CRD is created for recovery analysis

#### Recovery Viability Evaluation

Before entering "recovering" phase, Remediation Orchestrator checks:

```go
// Pseudo-code for recovery viability evaluation
func (r *RemediationRequestReconciler) evaluateRecoveryViability(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
) (canRecover bool, reason string) {

    // Check 1: Recovery attempts limit
    if remediation.Status.RecoveryAttempts >= remediation.Status.MaxRecoveryAttempts {
        return false, "max_recovery_attempts_exceeded"
    }

    // Check 2: Pattern detection (same failure twice)
    if r.hasRepeatedFailurePattern(remediation) {
        return false, "repeated_failure_pattern"
    }

    // Check 3: Termination rate (BR-WF-541: <10%)
    terminationRate := r.calculateTerminationRate(ctx)
    if terminationRate >= 0.10 {
        return false, "termination_rate_exceeded"
    }

    return true, ""
}
```

#### Recovery Tracking Fields Usage

**RecoveryAttempts**: Increments each time a new recovery AIAnalysis is created
```go
// Initial workflow fails → RecoveryAttempts = 0 → Create AIAnalysis #2 → RecoveryAttempts = 1
// Recovery workflow fails → RecoveryAttempts = 1 → Create AIAnalysis #3 → RecoveryAttempts = 2
// Recovery workflow fails → RecoveryAttempts = 2 → Create AIAnalysis #4 → RecoveryAttempts = 3
// Recovery workflow fails → RecoveryAttempts = 3 → MAX REACHED → Escalate
```

**AIAnalysisRefs**: Array tracking initial + all recovery analyses
```yaml
status:
  aiAnalysisRefs:
    - name: ai-analysis-001        # Initial analysis
      namespace: default
    - name: ai-analysis-002        # Recovery attempt 1
      namespace: default
    - name: ai-analysis-003        # Recovery attempt 2
      namespace: default
```

**WorkflowExecutionRefs**: Array tracking all workflow attempts with outcomes
```yaml
status:
  workflowExecutionRefs:
    - name: workflow-001
      namespace: default
      outcome: failed
      failedStep: 3
      failureReason: "timeout"
      attemptNumber: 1
    - name: workflow-002
      namespace: default
      outcome: in-progress
      attemptNumber: 2
  currentWorkflowExecutionRef: "workflow-002"
```

#### Recovery Phase Transitions

**Enter "recovering" Phase**:
```go
// When WorkflowExecution fails
if workflowExecution.Status.Phase == "failed" && canRecover {
    remediation.Status.OverallPhase = "recovering"
    remediation.Status.RecoveryAttempts++
    remediation.Status.LastFailureTime = &metav1.Time{Time: time.Now()}
    remediation.Status.RecoveryReason = &workflowExecution.Status.FailureReason

    // Create new AIAnalysis for recovery
    aiAnalysis := createRecoveryAIAnalysis(remediation, workflowExecution)

    // Update refs arrays
    remediation.Status.AIAnalysisRefs = append(
        remediation.Status.AIAnalysisRefs,
        AIAnalysisReference{Name: aiAnalysis.Name, Namespace: aiAnalysis.Namespace},
    )
    remediation.Status.WorkflowExecutionRefs = append(
        remediation.Status.WorkflowExecutionRefs,
        WorkflowExecutionReferenceWithOutcome{
            Name: workflowExecution.Name,
            Namespace: workflowExecution.Namespace,
            Outcome: "failed",
            FailedStep: &workflowExecution.Status.FailedStep,
            FailureReason: &workflowExecution.Status.FailureReason,
            AttemptNumber: remediation.Status.RecoveryAttempts,
        },
    )
}
```

**Exit "recovering" Phase**:
```go
// When recovery AIAnalysis completes, transition back to "executing"
if aiAnalysis.Status.Phase == "completed" && remediation.Status.OverallPhase == "recovering" {
    remediation.Status.OverallPhase = "executing"

    // Create new WorkflowExecution from recovery analysis
    workflow := createWorkflowFromAIAnalysis(remediation, aiAnalysis)
    remediation.Status.CurrentWorkflowExecutionRef = &workflow.Name
}
```

**Escalate to Manual Review**:
```go
// When recovery is not viable
if !canRecover {
    remediation.Status.OverallPhase = "failed"
    remediation.Status.EscalatedToManualReview = true
    remediation.Status.FailureReason = &reason  // "max_recovery_attempts_exceeded", etc.

    // Send notification to operations team
<<<<<<< HEAD
    r.NotificationClient.SendManualReviewAlert(remediation)
=======
    r.NotificationClient.SendManualReviewNotification(remediation)
>>>>>>> crd_implementation
}
```

#### Recovery Loop Prevention

**Max Attempts**: Default 3, configurable via RemediationRequest spec
```yaml
spec:
  recoveryConfig:
    maxAttempts: 3  # Optional override
```

**Pattern Detection**: Track failure signatures
```go
type FailurePattern struct {
    Action        string  // "scale-deployment"
    ErrorType     string  // "timeout"
    FailureStep   int     // 3
}

// If same pattern occurs twice, escalate
if countFailurePattern(remediation, currentPattern) >= 2 {
    escalateToManualReview(remediation, "repeated_failure_pattern")
}
```

**Termination Rate**: System-wide metric (BR-WF-541)
```go
// Calculate across all RemediationRequests in last hour
terminationRate := (totalFailed / totalAttempted)
// If > 10%, stop creating new recovery workflows
```

#### Example: Complete Recovery Lifecycle

**Initial State**:
```yaml
status:
  overallPhase: executing
  recoveryAttempts: 0
  maxRecoveryAttempts: 3
  aiAnalysisRefs:
    - name: ai-analysis-001
  workflowExecutionRefs:
    - name: workflow-001
      outcome: in-progress
      attemptNumber: 1
```

**After First Failure**:
```yaml
status:
  overallPhase: recovering         # CHANGED
  recoveryAttempts: 1               # INCREMENTED
  maxRecoveryAttempts: 3
  lastFailureTime: "2025-10-08T..."  # NEW
  recoveryReason: "workflow_timeout" # NEW
  aiAnalysisRefs:
    - name: ai-analysis-001
    - name: ai-analysis-002          # ADDED
  workflowExecutionRefs:
    - name: workflow-001
      outcome: failed                # CHANGED
      failedStep: 3                  # ADDED
      failureReason: "timeout"       # ADDED
      attemptNumber: 1
    - name: workflow-002
      outcome: in-progress           # NEW
      attemptNumber: 2               # NEW
  currentWorkflowExecutionRef: "workflow-002"  # CHANGED
```

**After Recovery Success**:
```yaml
status:
  overallPhase: completed           # SUCCESS
  recoveryAttempts: 1
  maxRecoveryAttempts: 3
  lastFailureTime: "2025-10-08T..."
  recoveryReason: "workflow_timeout"
  aiAnalysisRefs:
    - name: ai-analysis-001
    - name: ai-analysis-002
  workflowExecutionRefs:
    - name: workflow-001
      outcome: failed
      failedStep: 3
      failureReason: "timeout"
      attemptNumber: 1
    - name: workflow-002
      outcome: completed             # SUCCESS
      completionTime: "2025-10-08T..." # ADDED
      attemptNumber: 2
```

**After Max Attempts Exceeded**:
```yaml
status:
  overallPhase: failed              # TERMINAL
  recoveryAttempts: 3               # MAX REACHED
  maxRecoveryAttempts: 3
  escalatedToManualReview: true     # ESCALATED
  failureReason: "max_recovery_attempts_exceeded"
  aiAnalysisRefs:
    - name: ai-analysis-001
    - name: ai-analysis-002
    - name: ai-analysis-003
    - name: ai-analysis-004
  workflowExecutionRefs:
    - name: workflow-001
      outcome: failed
      attemptNumber: 1
    - name: workflow-002
      outcome: failed
      attemptNumber: 2
    - name: workflow-003
      outcome: failed
      attemptNumber: 3
    - name: workflow-004
      outcome: failed
      attemptNumber: 4
```

#### Implementation Checklist

- [ ] Update RemediationRequest CRD API types (api/remediation/v1/)
- [ ] Implement recovery viability evaluation logic
- [ ] Implement pattern detection algorithm
- [ ] Add WorkflowExecution watch for failure events
- [ ] Implement recovery AIAnalysis creation logic
- [ ] Add "recovering" phase handler in reconciliation loop
- [ ] Implement escalation to manual review
- [ ] Add termination rate calculation
- [ ] Update status field population logic
- [ ] Add integration tests for recovery flow
- [ ] Add metrics for recovery attempts and success rate

#### Related Documentation

- **Architecture**: [`PROPOSED_FAILURE_RECOVERY_SEQUENCE.md`](../../../architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md)
- **Business Requirements**: See Section 5 "Recovery Orchestration" (to be added in C3)
- **Controller Implementation**: [`controller-implementation.md`](./controller-implementation.md) (to be updated in C7)
- **Integration Points**: [`integration-points.md`](./integration-points.md) (to be updated in C8)

---

