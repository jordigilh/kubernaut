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
    AlertFingerprint string `json:"alertFingerprint"` // Unique fingerprint for deduplication
    AlertName        string `json:"alertName"`        // Human-readable signal name

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
    OverallPhase string      `json:"overallPhase"` // pending, processing, analyzing, executing, completed, failed, timeout
    StartTime    metav1.Time `json:"startTime"`
    CompletionTime *metav1.Time `json:"completionTime,omitempty"`

    // Service CRD references (created by this controller)
    RemediationProcessingRef     *RemediationProcessingReference     `json:"alertProcessingRef,omitempty"`
    AIAnalysisRef          *AIAnalysisReference          `json:"aiAnalysisRef,omitempty"`
    WorkflowExecutionRef   *WorkflowExecutionReference   `json:"workflowExecutionRef,omitempty"`
    KubernetesExecutionRef *KubernetesExecutionReference `json:"kubernetesExecutionRef,omitempty"`

    // Aggregated status from service CRDs
    RemediationProcessingStatus     *RemediationProcessingStatusSummary     `json:"alertProcessingStatus,omitempty"`
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
type RemediationProcessingReference struct {
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
}

type AIAnalysisReference struct {
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
}

type WorkflowExecutionReference struct {
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
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

