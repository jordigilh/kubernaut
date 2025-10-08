# Option B: Context API Integration in Remediation Orchestrator

**Original Status**: ✅ Phase 1 Critical Fix (C7) - Updated for Option B
**Current Status**: 🔄 **SUPERSEDED BY ALTERNATIVE 2**
**Reference**: [`docs/architecture/OPTION_B_IMPLEMENTATION_SUMMARY.md`](../../../architecture/OPTION_B_IMPLEMENTATION_SUMMARY.md)
**Business Requirement**: BR-WF-RECOVERY-011

---

## 🚨 **DOCUMENT STATUS: SUPERSEDED**

**This document describes Option B (Remediation Orchestrator queries Context API and embeds in AIAnalysis).**

**Option B has been superseded by Alternative 2 (RemediationProcessing queries Context API during enrichment).**

### **Why Alternative 2 Supersedes Option B:**

1. ✅ **Temporal Consistency**: All contexts (monitoring + business + recovery) captured at same timestamp
2. ✅ **Fresh Contexts**: Recovery sees CURRENT cluster state (not stale from initial attempt)
3. ✅ **Immutable Audit Trail**: Each RemediationProcessing CRD is separate and auditable
4. ✅ **Architectural Consistency**: ALL enrichment in RemediationProcessing (not split between RP and RR)
5. ✅ **Pattern Reuse**: Recovery follows same flow as initial (watch → enrich → complete)

### **Current Implementation Reference:**

**See**: [`PROPOSED_FAILURE_RECOVERY_SEQUENCE.md`](../../../architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md) (Version 1.2 - Alternative 2)

**Key Difference**:
- **Option B**: RR queries Context API → embeds in AIAnalysis
- **Alternative 2**: RR creates RP #2 (recovery) → RP queries Context API → RR creates AIAnalysis

**Context API Integration Now In**: `docs/services/crd-controllers/01-remediationprocessor/controller-implementation.md`

---

## 📜 **Historical Context (Option B Design - For Reference Only)**

**This section preserved for historical reference.**

---

## 🎯 **Overview (Option B - Superseded)**

In Option B architecture, the **Remediation Orchestrator** was responsible for querying the Context API and embedding historical context in the AIAnalysis CRD spec. This approach was superseded by Alternative 2.

---

## 🔧 **Context API Client Interface**

```go
// pkg/remediationorchestrator/context_client.go
package remediationorchestrator

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    aiv1 "github.com/jordigilh/kubernaut/api/ai/v1"
)

// ContextAPIClient queries the Context API for historical remediation context
type ContextAPIClient interface {
    // GetRemediationContext retrieves historical context for a remediation request
    GetRemediationContext(ctx context.Context, remediationRequestID string) (*ContextAPIResponse, error)
}

type ContextAPIClientImpl struct {
    BaseURL    string
    HTTPClient *http.Client
    Timeout    time.Duration
}

// ContextAPIResponse is the raw response from Context API
type ContextAPIResponse struct {
    RemediationRequestID string                    `json:"remediationRequestId"`
    CurrentAttempt       int                       `json:"currentAttempt"`
    PreviousFailures     []PreviousFailureDTO      `json:"previousFailures"`
    RelatedAlerts        []RelatedAlertDTO         `json:"relatedAlerts"`
    HistoricalPatterns   []HistoricalPatternDTO    `json:"historicalPatterns"`
    SuccessfulStrategies []SuccessfulStrategyDTO   `json:"successfulStrategies"`
    ContextQuality       string                    `json:"contextQuality"` // "complete", "partial", "minimal"
}

// DTOs for Context API response
type PreviousFailureDTO struct {
    WorkflowRef      string                 `json:"workflowRef"`
    AttemptNumber    int                    `json:"attemptNumber"`
    FailedStep       int                    `json:"failedStep"`
    Action           string                 `json:"action"`
    ErrorType        string                 `json:"errorType"`
    FailureReason    string                 `json:"failureReason"`
    Duration         string                 `json:"duration"`
    ClusterState     map[string]interface{} `json:"clusterState"`
    ResourceSnapshot map[string]interface{} `json:"resourceSnapshot"`
    Timestamp        time.Time              `json:"timestamp"`
}

type RelatedAlertDTO struct {
    AlertFingerprint string    `json:"alertFingerprint"`
    AlertName        string    `json:"alertName"`
    Correlation      float64   `json:"correlation"`
    Timestamp        time.Time `json:"timestamp"`
}

type HistoricalPatternDTO struct {
    Pattern             string  `json:"pattern"`
    Occurrences         int     `json:"occurrences"`
    SuccessRate         float64 `json:"successRate"`
    AverageRecoveryTime string  `json:"averageRecoveryTime"`
}

type SuccessfulStrategyDTO struct {
    Strategy     string    `json:"strategy"`
    Description  string    `json:"description"`
    SuccessCount int       `json:"successCount"`
    LastUsed     time.Time `json:"lastUsed"`
    Confidence   float64   `json:"confidence"`
}

func (c *ContextAPIClientImpl) GetRemediationContext(
    ctx context.Context,
    remediationRequestID string,
) (*ContextAPIResponse, error) {

    url := fmt.Sprintf("%s/context/remediation/%s", c.BaseURL, remediationRequestID)

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("User-Agent", "kubernaut-remediation-orchestrator/1.0")

    client := c.HTTPClient
    if client == nil {
        client = &http.Client{Timeout: c.Timeout}
    }

    resp, err := client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("context API request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("context API returned status %d", resp.StatusCode)
    }

    var response ContextAPIResponse
    if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
        return nil, fmt.Errorf("failed to decode response: %w", err)
    }

    return &response, nil
}
```

---

## 🔄 **Updated Reconciler Struct**

```go
type RemediationRequestReconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder

    // NEW: Context API client for recovery
    ContextAPIClient ContextAPIClient

    // Existing clients
    NotificationClient NotificationClient
    StorageClient      StorageClient

    // Metrics
    metricsRecorder MetricsRecorder
}
```

---

## 📥 **Context Embedding Logic in `initiateRecovery()`**

```go
func (r *RemediationRequestReconciler) initiateRecovery(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    failedWorkflow *workflowv1.WorkflowExecution,
) (ctrl.Result, error) {

    log := ctrl.LoggerFrom(ctx)

    // ========================================
    // STEP 1: Query Context API
    // ========================================
    var embeddedContext *aiv1.HistoricalContext
    contextQuality := "minimal"

    log.Info("Fetching historical context from Context API",
        "remediationRequest", remediation.Name)

    contextAPIResponse, err := r.ContextAPIClient.GetRemediationContext(ctx, remediation.Name)

    if err != nil {
        // GRACEFUL DEGRADATION: Create minimal context from WorkflowExecutionRefs
        log.Error(err, "Failed to fetch context from Context API, using fallback")
        r.Recorder.Event(remediation, corev1.EventTypeWarning,
            "ContextAPIUnavailable",
            fmt.Sprintf("Context API unavailable: %v - using fallback context", err))

        embeddedContext = r.buildFallbackContext(remediation, failedWorkflow)
        contextQuality = "degraded"

        r.metricsRecorder.RecordContextAPIFailure()
    } else {
        // Context API success - convert response to CRD-embeddable format
        log.Info("Historical context retrieved successfully from Context API",
            "previousFailures", len(contextAPIResponse.PreviousFailures),
            "relatedAlerts", len(contextAPIResponse.RelatedAlerts),
            "contextQuality", contextAPIResponse.ContextQuality)

        embeddedContext = r.convertContextAPIResponseToEmbeddable(contextAPIResponse)
        contextQuality = contextAPIResponse.ContextQuality

        r.metricsRecorder.RecordContextAPISuccess(len(contextAPIResponse.PreviousFailures))
    }

    // ========================================
    // STEP 2: Update RemediationRequest Phase
    // ========================================
    remediation.Status.OverallPhase = "recovering"
    remediation.Status.RecoveryAttempts++
    remediation.Status.LastFailureTime = &metav1.Time{Time: time.Now()}
    reason := fmt.Sprintf("workflow_%s_step_%d",
        *failedWorkflow.Status.ErrorType,
        *failedWorkflow.Status.FailedStep)
    remediation.Status.RecoveryReason = &reason

    // ========================================
    // STEP 3: Create AIAnalysis with Embedded Context
    // ========================================
    aiAnalysis := &aiv1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-recovery-%d", remediation.Name, remediation.Status.RecoveryAttempts),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: aiv1.AIAnalysisSpec{
            // Signal context
            AlertContext:          remediation.Spec.AlertContext,
            RemediationRequestRef: corev1.LocalObjectReference{Name: remediation.Name},

            // Recovery metadata
            IsRecoveryAttempt:      true,
            RecoveryAttemptNumber:  remediation.Status.RecoveryAttempts,
            FailedWorkflowRef:      &corev1.LocalObjectReference{Name: failedWorkflow.Name},
            FailedStep:             failedWorkflow.Status.FailedStep,
            FailureReason:          failedWorkflow.Status.FailureReason,
            PreviousAIAnalysisRefs: copyAIAnalysisRefs(remediation.Status.AIAnalysisRefs),

            // EMBEDDED HISTORICAL CONTEXT FROM CONTEXT API
            HistoricalContext: embeddedContext,
        },
    }

    if err := r.Create(ctx, aiAnalysis); err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to create recovery AIAnalysis: %w", err)
    }

    log.Info("Recovery AIAnalysis created with embedded context",
        "aiAnalysis", aiAnalysis.Name,
        "contextQuality", contextQuality,
        "contextSize", estimateContextSize(embeddedContext))

    // ========================================
    // STEP 4: Update RemediationRequest Status
    // ========================================
    remediation.Status.AIAnalysisRefs = append(
        remediation.Status.AIAnalysisRefs,
        remediationv1.AIAnalysisReference{
            Name:      aiAnalysis.Name,
            Namespace: aiAnalysis.Namespace,
        },
    )

    remediation.Status.WorkflowExecutionRefs = append(
        remediation.Status.WorkflowExecutionRefs,
        remediationv1.WorkflowExecutionReferenceWithOutcome{
            Name:           failedWorkflow.Name,
            Namespace:      failedWorkflow.Namespace,
            Outcome:        "failed",
            FailedStep:     failedWorkflow.Status.FailedStep,
            FailureReason:  failedWorkflow.Status.FailureReason,
            CompletionTime: failedWorkflow.Status.CompletionTime,
            AttemptNumber:  remediation.Status.RecoveryAttempts,
        },
    )

    remediation.Status.CurrentWorkflowExecutionRef = nil

    if err := r.Status().Update(ctx, remediation); err != nil {
        return ctrl.Result{}, err
    }

    r.Recorder.Event(remediation, corev1.EventTypeNormal, "RecoveryInitiated",
        fmt.Sprintf("Recovery attempt %d/%d initiated with %s context",
            remediation.Status.RecoveryAttempts,
            remediation.Status.MaxRecoveryAttempts,
            contextQuality))

    return ctrl.Result{}, nil
}
```

---

## 🔄 **Conversion Functions**

```go
// convertContextAPIResponseToEmbeddable converts Context API response to CRD-embeddable format
func (r *RemediationRequestReconciler) convertContextAPIResponseToEmbeddable(
    response *ContextAPIResponse,
) *aiv1.HistoricalContext {

    historicalContext := &aiv1.HistoricalContext{
        ContextQuality:       response.ContextQuality,
        PreviousFailures:     make([]aiv1.PreviousFailure, len(response.PreviousFailures)),
        RelatedAlerts:        make([]aiv1.RelatedAlert, len(response.RelatedAlerts)),
        HistoricalPatterns:   make([]aiv1.HistoricalPattern, len(response.HistoricalPatterns)),
        SuccessfulStrategies: make([]aiv1.SuccessfulStrategy, len(response.SuccessfulStrategies)),
        RetrievedAt:          metav1.Now(),
    }

    // Convert previous failures
    for i, failure := range response.PreviousFailures {
        historicalContext.PreviousFailures[i] = aiv1.PreviousFailure{
            WorkflowRef:      failure.WorkflowRef,
            AttemptNumber:    failure.AttemptNumber,
            FailedStep:       failure.FailedStep,
            Action:           failure.Action,
            ErrorType:        failure.ErrorType,
            FailureReason:    failure.FailureReason,
            Duration:         failure.Duration,
            ClusterState:     failure.ClusterState,
            ResourceSnapshot: failure.ResourceSnapshot,
            Timestamp:        metav1.NewTime(failure.Timestamp),
        }
    }

    // Convert related alerts
    for i, alert := range response.RelatedAlerts {
        historicalContext.RelatedAlerts[i] = aiv1.RelatedAlert{
            AlertFingerprint: alert.AlertFingerprint,
            AlertName:        alert.AlertName,
            Correlation:      alert.Correlation,
            Timestamp:        metav1.NewTime(alert.Timestamp),
        }
    }

    // Convert historical patterns
    for i, pattern := range response.HistoricalPatterns {
        historicalContext.HistoricalPatterns[i] = aiv1.HistoricalPattern{
            Pattern:             pattern.Pattern,
            Occurrences:         pattern.Occurrences,
            SuccessRate:         pattern.SuccessRate,
            AverageRecoveryTime: pattern.AverageRecoveryTime,
        }
    }

    // Convert successful strategies
    for i, strategy := range response.SuccessfulStrategies {
        historicalContext.SuccessfulStrategies[i] = aiv1.SuccessfulStrategy{
            Strategy:     strategy.Strategy,
            Description:  strategy.Description,
            SuccessCount: strategy.SuccessCount,
            LastUsed:     metav1.NewTime(strategy.LastUsed),
            Confidence:   strategy.Confidence,
        }
    }

    return historicalContext
}
```

---

## 🛡️ **Graceful Degradation: Fallback Context**

```go
// buildFallbackContext creates minimal context from WorkflowExecutionRefs when Context API fails
func (r *RemediationRequestReconciler) buildFallbackContext(
    remediation *remediationv1.RemediationRequest,
    currentFailure *workflowv1.WorkflowExecution,
) *aiv1.HistoricalContext {

    // Extract previous failures from WorkflowExecutionRefs
    previousFailures := make([]aiv1.PreviousFailure, 0, len(remediation.Status.WorkflowExecutionRefs))

    for _, ref := range remediation.Status.WorkflowExecutionRefs {
        if ref.Outcome == "failed" && ref.FailedStep != nil && ref.FailureReason != nil {
            previousFailures = append(previousFailures, aiv1.PreviousFailure{
                WorkflowRef:   ref.Name,
                AttemptNumber: ref.AttemptNumber,
                FailedStep:    *ref.FailedStep,
                FailureReason: *ref.FailureReason,
                Timestamp:     *ref.CompletionTime,
            })
        }
    }

    // Add current failure
    if currentFailure != nil && currentFailure.Status.FailedStep != nil {
        previousFailures = append(previousFailures, aiv1.PreviousFailure{
            WorkflowRef:   currentFailure.Name,
            AttemptNumber: remediation.Status.RecoveryAttempts + 1,
            FailedStep:    *currentFailure.Status.FailedStep,
            Action:        coalesce(currentFailure.Status.FailedAction, ""),
            ErrorType:     coalesce(currentFailure.Status.ErrorType, ""),
            FailureReason: coalesce(currentFailure.Status.FailureReason, ""),
            Timestamp:     *currentFailure.Status.CompletionTime,
        })
    }

    return &aiv1.HistoricalContext{
        ContextQuality:       "degraded",
        PreviousFailures:     previousFailures,
        RelatedAlerts:        []aiv1.RelatedAlert{},
        HistoricalPatterns:   []aiv1.HistoricalPattern{},
        SuccessfulStrategies: []aiv1.SuccessfulStrategy{},
        RetrievedAt:          metav1.Now(),
    }
}

func coalesce(ptr *string, fallback string) string {
    if ptr != nil {
        return *ptr
    }
    return fallback
}
```

---

## 📊 **Metrics**

```go
// MetricsRecorder interface - extended for Context API
type MetricsRecorder interface {
    // Existing metrics...
    RecordRecoveryViabilityAllowed()
    RecordRecoveryViabilityDenied(reason string)

    // NEW: Context API metrics
    RecordContextAPISuccess(failureCount int)
    RecordContextAPIFailure()
    UpdateContextAPILatency(duration time.Duration)
}

// Prometheus metrics
var (
    contextAPIRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubernaut_remediation_orchestrator_context_api_requests_total",
            Help: "Total Context API requests from Remediation Orchestrator",
        },
        []string{"success"}, // "true", "false"
    )

    contextAPILatency = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "kubernaut_remediation_orchestrator_context_api_latency_seconds",
            Help:    "Context API request latency",
            Buckets: []float64{0.1, 0.5, 1.0, 2.0, 5.0},
        },
    )

    embeddedContextHistoricalFailures = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "kubernaut_embedded_context_historical_failures_count",
            Help:    "Number of historical failures embedded in AIAnalysis CRDs",
            Buckets: []float64{0, 1, 2, 3, 5, 10},
        },
    )
)
```

---

## ✅ **Testing Strategy**

```go
func TestContextAPIIntegration_Success(t *testing.T) {
    // Mock Context API client
    mockClient := &MockContextAPIClient{
        GetRemediationContextFunc: func(ctx context.Context, id string) (*ContextAPIResponse, error) {
            return &ContextAPIResponse{
                RemediationRequestID: id,
                CurrentAttempt:       2,
                PreviousFailures: []PreviousFailureDTO{
                    {
                        WorkflowRef:   "workflow-001",
                        AttemptNumber: 1,
                        FailedStep:    3,
                        FailureReason: "timeout",
                    },
                },
                ContextQuality: "complete",
            }, nil
        },
    }

    reconciler := &RemediationRequestReconciler{
        ContextAPIClient: mockClient,
    }

    remediation := createTestRemediationRequest()
    failedWorkflow := createTestFailedWorkflow()

    result, err := reconciler.initiateRecovery(context.Background(), remediation, failedWorkflow)

    assert.NoError(t, err)
    assert.Equal(t, "recovering", remediation.Status.OverallPhase)
    assert.Len(t, remediation.Status.AIAnalysisRefs, 2) // Initial + recovery

    // Verify embedded context
    var aiAnalysis aiv1.AIAnalysis
    err = reconciler.Get(context.Background(), client.ObjectKey{
        Name:      remediation.Status.AIAnalysisRefs[1].Name,
        Namespace: remediation.Namespace,
    }, &aiAnalysis)

    assert.NoError(t, err)
    assert.NotNil(t, aiAnalysis.Spec.HistoricalContext)
    assert.Equal(t, "complete", aiAnalysis.Spec.HistoricalContext.ContextQuality)
    assert.Len(t, aiAnalysis.Spec.HistoricalContext.PreviousFailures, 1)
}

func TestContextAPIIntegration_GracefulDegradation(t *testing.T) {
    // Mock Context API client that fails
    mockClient := &MockContextAPIClient{
        GetRemediationContextFunc: func(ctx context.Context, id string) (*ContextAPIResponse, error) {
            return nil, fmt.Errorf("context API unavailable")
        },
    }

    reconciler := &RemediationRequestReconciler{
        ContextAPIClient: mockClient,
    }

    remediation := createTestRemediationRequest()
    remediation.Status.WorkflowExecutionRefs = []remediationv1.WorkflowExecutionReferenceWithOutcome{
        {
            Name:          "workflow-001",
            Outcome:       "failed",
            FailedStep:    ptr.To(2),
            FailureReason: ptr.To("resource not found"),
            AttemptNumber: 1,
        },
    }
    failedWorkflow := createTestFailedWorkflow()

    result, err := reconciler.initiateRecovery(context.Background(), remediation, failedWorkflow)

    // Should NOT fail - graceful degradation
    assert.NoError(t, err)
    assert.Equal(t, "recovering", remediation.Status.OverallPhase)

    // Verify fallback context was created
    var aiAnalysis aiv1.AIAnalysis
    err = reconciler.Get(context.Background(), client.ObjectKey{
        Name:      remediation.Status.AIAnalysisRefs[1].Name,
        Namespace: remediation.Namespace,
    }, &aiAnalysis)

    assert.NoError(t, err)
    assert.NotNil(t, aiAnalysis.Spec.HistoricalContext)
    assert.Equal(t, "degraded", aiAnalysis.Spec.HistoricalContext.ContextQuality)
    assert.Len(t, aiAnalysis.Spec.HistoricalContext.PreviousFailures, 2) // From WorkflowExecutionRefs + current
}
```

---

## 📋 **Implementation Checklist**

- [ ] Implement ContextAPIClient interface
- [ ] Implement ContextAPIClientImpl with HTTP client
- [ ] Add ContextAPIClient to RemediationRequestReconciler struct
- [ ] Implement convertContextAPIResponseToEmbeddable()
- [ ] Implement buildFallbackContext() for graceful degradation
- [ ] Update initiateRecovery() to query Context API and embed result
- [ ] Add Context API metrics
- [ ] Write unit tests for Context API success
- [ ] Write unit tests for graceful degradation
- [ ] Write integration tests with real Context API
- [ ] Add circuit breaker for Context API failures
- [ ] Document Context API endpoint specification

---

## 🔗 **Related Documentation**

- **Design Decision**: [`OPTION_B_IMPLEMENTATION_SUMMARY.md`](../../../architecture/OPTION_B_IMPLEMENTATION_SUMMARY.md)
- **Architecture**: [`PROPOSED_FAILURE_RECOVERY_SEQUENCE.md`](../../../architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md)
- **AIAnalysis Controller** (reads embedded context): `docs/services/crd-controllers/02-aianalysis/controller-implementation.md` (C5)
- **Business Requirements**: BR-WF-RECOVERY-011 in `docs/requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md`

---

**Document Version**: 1.0
**Last Updated**: October 8, 2025
**Status**: Implementation Ready

