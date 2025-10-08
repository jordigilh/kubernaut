## Integration Points

### 1. Upstream Integration: RemediationRequest Controller

**Integration Pattern**: Watch-based status coordination

**How RemediationProcessing is Created**:
```go
// In RemediationRequestReconciler (Remediation Coordinator)
func (r *RemediationRequestReconciler) reconcileRemediationProcessing(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
) error {
    // When RemediationRequest is first created, create RemediationProcessing
    if remediation.Status.RemediationProcessingRef == nil {
        alertProcessing := &processingv1.RemediationProcessing{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("%s-processing", remediation.Name),
                Namespace: remediation.Namespace,
                OwnerReferences: []metav1.OwnerReference{
                    *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
                },
            },
            Spec: processingv1.RemediationProcessingSpec{
                RemediationRequestRef: processingv1.RemediationRequestReference{
                    Name:      remediation.Name,
                    Namespace: remediation.Namespace,
                },
                // Copy original alert data
                Alert: processingv1.Alert{
                    Fingerprint: remediation.Spec.AlertFingerprint,
                    Payload:     remediation.Spec.OriginalPayload,
                    Severity:    remediation.Spec.Severity,
                },
            },
        }

        return r.Create(ctx, alertProcessing)
    }

    return nil
}
```

**Note**: RemediationRequest controller creates RemediationProcessing CRD when remediation workflow starts.

### 2. Downstream Integration: RemediationRequest Watches RemediationProcessing Status

**Integration Pattern**: Data snapshot - RemediationRequest creates AIAnalysis after RemediationProcessing completes

**How RemediationRequest Responds to Completion**:
```go
// In RemediationRequestReconciler (Remediation Coordinator)
func (r *RemediationRequestReconciler) reconcileAIAnalysis(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    alertProcessing *processingv1.RemediationProcessing,
) error {
    // When RemediationProcessing completes, create AIAnalysis with enriched data
    if alertProcessing.Status.Phase == "completed" && remediation.Status.AIAnalysisRef == nil {
        aiAnalysis := &aianalysisv1.AIAnalysis{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("%s-analysis", remediation.Name),
                Namespace: remediation.Namespace,
                OwnerReferences: []metav1.OwnerReference{
                    *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
                },
            },
            Spec: aianalysisv1.AIAnalysisSpec{
                RemediationRequestRef: aianalysisv1.RemediationRequestReference{
                    Name:      remediation.Name,
                    Namespace: remediation.Namespace,
                },
                // COPY enriched alert data (data snapshot pattern - TARGETING DATA ONLY)
                AnalysisRequest: aianalysisv1.AnalysisRequest{
                    AlertContext: aianalysisv1.AlertContext{
                        Fingerprint:      alertProcessing.Status.EnrichedAlert.Fingerprint,
                        Severity:         alertProcessing.Status.EnrichedAlert.Severity,
                        Environment:      alertProcessing.Status.EnrichedAlert.Environment,
                        BusinessPriority: alertProcessing.Status.EnrichedAlert.BusinessPriority,

                        // Resource targeting for HolmesGPT (NOT logs/metrics - toolsets fetch those)
                        Namespace:    alertProcessing.Status.EnrichedAlert.Namespace,
                        ResourceKind: alertProcessing.Status.EnrichedAlert.ResourceKind,
                        ResourceName: alertProcessing.Status.EnrichedAlert.ResourceName,

                        // Kubernetes context (small data ~8KB)
                        KubernetesContext: alertProcessing.Status.EnrichedAlert.KubernetesContext,
                    },
                },
            },
        }

        return r.Create(ctx, aiAnalysis)
    }

    return nil
}
```

**Note**: RemediationProcessing does NOT create AIAnalysis CRD. RemediationRequest controller watches RemediationProcessing status and creates AIAnalysis when processing completes.

### 3. External Service Integration: Context Service

**Integration Pattern**: Synchronous HTTP REST API call

**Endpoint**: Context Service - see [README: Context Service](../../README.md#-9-context-service)

**Request**:
```go
type ContextRequest struct {
    Namespace     string            `json:"namespace"`
    ResourceKind  string            `json:"resourceKind"`
    ResourceName  string            `json:"resourceName"`
    AlertLabels   map[string]string `json:"alertLabels"`
}
```

**Response**:
```go
type ContextResponse struct {
    PodDetails        PodDetails        `json:"podDetails"`
    DeploymentDetails DeploymentDetails `json:"deploymentDetails"`
    NodeDetails       NodeDetails       `json:"nodeDetails"`
    RelatedResources  []RelatedResource `json:"relatedResources"`
}
```

**Degraded Mode Fallback**:
```go
// If Context Service unavailable, extract minimal context from alert labels
func (e *Enricher) DegradedModeEnrich(alert Alert) EnrichedAlert {
    return EnrichedAlert{
        Fingerprint: alert.Fingerprint,
        Severity:    alert.Severity,
        Environment: extractEnvironmentFromLabels(alert.Labels),
        KubernetesContext: KubernetesContext{
            Namespace:    alert.Labels["namespace"],
            PodName:      alert.Labels["pod"],
            // Minimal context from alert labels only
        },
    }
}
```

### 4. External Service Integration: Context API (Recovery Enrichment - DD-001: Alternative 2)

> **📋 Design Decision: DD-001 | ✅ Approved Design**
> **See**: [DD-001](../../../architecture/DESIGN_DECISIONS.md#dd-001-recovery-context-enrichment-alternative-2)

**Integration Pattern**: Conditional synchronous HTTP REST API call (ONLY for recovery attempts)
**Business Requirement**: BR-WF-RECOVERY-011
**Reference**: [`PROPOSED_FAILURE_RECOVERY_SEQUENCE.md`](../../../architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md) (Version 1.2 - Alternative 2)

**When Used**: Only when `spec.isRecoveryAttempt = true`

**Endpoint**: Context API `/api/v1/context/remediation/{remediationRequestId}`

**Purpose**: Retrieve historical failure context to enable AI to generate alternative recovery strategies

**Request**:
```go
// GET /api/v1/context/remediation/{remediationRequestId}
// Query parameter: remediationRequestId from spec.remediationRequestRef.name

type ContextAPIClient interface {
    GetRemediationContext(ctx context.Context, remediationRequestID string) (*ContextAPIResponse, error)
}
```

**Response**:
```go
type ContextAPIResponse struct {
    RemediationRequestID string                    `json:"remediationRequestId"`
    CurrentAttempt       int                       `json:"currentAttempt"`
    PreviousFailures     []PreviousFailureDTO      `json:"previousFailures"`
    RelatedAlerts        []RelatedAlertDTO         `json:"relatedAlerts"`
    HistoricalPatterns   []HistoricalPatternDTO    `json:"historicalPatterns"`
    SuccessfulStrategies []SuccessfulStrategyDTO   `json:"successfulStrategies"`
    ContextQuality       string                    `json:"contextQuality"` // "complete", "partial", "minimal"
}

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
```

**Integration Flow (Alternative 2)**:
```go
// In RemediationProcessingReconciler.reconcileEnriching()
func (r *RemediationProcessingReconciler) reconcileEnriching(
    ctx context.Context,
    rp *processingv1.RemediationProcessing,
) (ctrl.Result, error) {

    log := ctrl.LoggerFrom(ctx)

    // ALWAYS: Enrich monitoring + business context (gets FRESH data)
    enrichmentResults, err := r.ContextService.GetContext(ctx, rp.Spec.Alert)
    if err != nil {
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    rp.Status.EnrichmentResults = enrichmentResults

    // IF RECOVERY: ALSO query Context API for recovery context (Alternative 2)
    if rp.Spec.IsRecoveryAttempt {
        log.Info("Recovery attempt detected - querying Context API for historical context",
            "attemptNumber", rp.Spec.RecoveryAttemptNumber,
            "remediationRequestID", rp.Spec.RemediationRequestRef.Name)

        // Query Context API
        recoveryCtx, err := r.enrichRecoveryContext(ctx, rp)
        if err != nil {
            // Graceful degradation: Use fallback context
            log.Warn("Context API unavailable, using fallback recovery context",
                "error", err,
                "attemptNumber", rp.Spec.RecoveryAttemptNumber)

            r.Recorder.Event(rp, "Warning", "ContextAPIUnavailable",
                fmt.Sprintf("Context API query failed: %v. Using fallback context.", err))

            recoveryCtx = r.buildFallbackRecoveryContext(rp)
        }

        // Add recovery context to enrichment results
        rp.Status.EnrichmentResults.RecoveryContext = recoveryCtx

        log.Info("Recovery context enrichment completed",
            "contextQuality", recoveryCtx.ContextQuality,
            "previousFailuresCount", len(recoveryCtx.PreviousFailures),
            "relatedAlertsCount", len(recoveryCtx.RelatedAlerts))
    }

    rp.Status.Phase = "classifying"
    return ctrl.Result{Requeue: true}, r.Status().Update(ctx, rp)
}

// Helper function to query Context API
func (r *RemediationProcessingReconciler) enrichRecoveryContext(
    ctx context.Context,
    rp *processingv1.RemediationProcessing,
) (*processingv1.RecoveryContext, error) {

    // Query Context API
    contextResp, err := r.ContextAPIClient.GetRemediationContext(
        ctx,
        rp.Spec.RemediationRequestRef.Name,
    )

    if err != nil {
        return nil, fmt.Errorf("Context API query failed: %w", err)
    }

    // Convert DTO to CRD type
    return convertToRecoveryContext(contextResp), nil
}

// Graceful degradation fallback
func (r *RemediationProcessingReconciler) buildFallbackRecoveryContext(
    rp *processingv1.RemediationProcessing,
) *processingv1.RecoveryContext {

    return &processingv1.RecoveryContext{
        ContextQuality: "degraded",
        PreviousFailures: []processingv1.PreviousFailure{
            {
                WorkflowRef:   rp.Spec.FailedWorkflowRef.Name,
                FailedStep:    *rp.Spec.FailedStep,
                FailureReason: *rp.Spec.FailureReason,
                AttemptNumber: rp.Spec.RecoveryAttemptNumber - 1,
            },
        },
        RetrievedAt: metav1.Now(),
    }
}
```

**Key Benefits (Alternative 2)**:
- ✅ **Temporal Consistency**: Recovery context retrieved at same time as monitoring/business contexts
- ✅ **Fresh Data**: All contexts (monitoring + business + recovery) captured at same timestamp
- ✅ **Graceful Degradation**: Falls back to minimal context from `failedWorkflowRef` if API unavailable
- ✅ **Architectural Consistency**: ALL enrichment happens in RemediationProcessing controller
- ✅ **Immutable Audit Trail**: Each RemediationProcessing CRD contains complete snapshot

**Graceful Degradation Example**:
```yaml
# Context API unavailable - fallback context created
status:
  enrichmentResults:
    recoveryContext:
      contextQuality: "degraded"
      previousFailures:
      - workflowRef: "workflow-001"
        failedStep: 3
        failureReason: "timeout"
        attemptNumber: 1
      retrievedAt: "2025-01-15T10:00:00Z"
```

**Success Example**:
```yaml
# Context API available - complete recovery context
status:
  enrichmentResults:
    recoveryContext:
      contextQuality: "complete"
      previousFailures:
      - workflowRef: "workflow-001"
        attemptNumber: 1
        failedStep: 3
        action: "scale-deployment"
        errorType: "timeout"
        failureReason: "Operation timed out after 5m"
        duration: "5m 3s"
        timestamp: "2025-01-15T09:50:00Z"
      relatedAlerts:
      - alertFingerprint: "related-123"
        alertName: "HighMemoryUsage"
        correlation: 0.85
      historicalPatterns:
      - pattern: "scale_timeout_high_memory"
        occurrences: 12
        successRate: 0.73
      successfulStrategies:
      - strategy: "force-delete-pods-then-scale"
        confidence: 0.88
        successCount: 8
      retrievedAt: "2025-01-15T10:00:00Z"
```

---

### 5. Database Integration: Data Storage Service

**Integration Pattern**: Audit trail persistence

**Endpoint**: Data Storage Service HTTP POST `/api/v1/audit/alert-processing`

**Audit Record**:
```go
type RemediationProcessingAudit struct {
    AlertFingerprint    string                     `json:"alertFingerprint"`
    ProcessingStartTime time.Time                  `json:"processingStartTime"`
    ProcessingEndTime   time.Time                  `json:"processingEndTime"`
    EnrichmentResult    EnrichedAlert              `json:"enrichmentResult"`
    ClassificationResult EnvironmentClassification `json:"classificationResult"`
    DegradedMode        bool                       `json:"degradedMode"`
    // Alternative 2: Recovery context audit
    RecoveryContext     *RecoveryContext           `json:"recoveryContext,omitempty"`
    IsRecoveryAttempt   bool                       `json:"isRecoveryAttempt"`
}
```

### 6. Dependencies Summary

**Upstream Services**:
- **Gateway Service** - Creates RemediationRequest CRD with duplicate detection already performed (BR-WH-008)
- **RemediationRequest Controller** - Creates RemediationProcessing CRD when workflow starts (initial & recovery)

**Downstream Services**:
- **RemediationRequest Controller** - Watches RemediationProcessing status and creates AIAnalysis CRD upon completion

**External Services**:
- **Context Service** - HTTP GET for Kubernetes context enrichment (monitoring + business contexts)
- **Context API** - HTTP GET for recovery context enrichment (ONLY for recovery attempts - Alternative 2)
- **Data Storage Service** - HTTP POST for audit trail persistence

**Database**:
- PostgreSQL - `alert_processing_audit` table for long-term audit storage
- Vector DB (optional) - Enrichment context embeddings for ML analysis

**Existing Code to Leverage** (after migration to `pkg/remediationprocessing/`):
- `pkg/remediationprocessing/` (migrated from `pkg/alert/`) - Alert processing business logic (1,103 lines)
  - `service.go` - AlertProcessorService interface (to be renamed)
  - `implementation.go` - Service implementation
  - `components.go` - Processing components (AlertProcessorImpl, AlertEnricherImpl, etc.)
- `pkg/processor/environment/classifier.go` - Environment classification
- `pkg/ai/context/` - Context gathering functions
- `pkg/storage/` - Database client and audit storage (to be created)

**Code to Move to Gateway Service**:
- `pkg/alert/components.go` → Gateway Service - `AlertDeduplicatorImpl` (fingerprint generation logic reusable)

