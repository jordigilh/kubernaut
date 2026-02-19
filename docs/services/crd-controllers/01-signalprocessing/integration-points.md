## Integration Points

> **📋 Changelog**
> | Version | Date | Changes | Reference |
> |---------|------|---------|-----------|
> | v1.3 | 2025-11-30 | Added OwnerChain (**ADR-055: removed**), DetectedLabels (**ADR-056: removed, now in PostRCAContext**), CustomLabels to downstream flow (DD-WORKFLOW-001 v1.8) | [DD-WORKFLOW-001 v1.8](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md), [HANDOFF v3.2](HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md) |
> | v1.2 | 2025-11-28 | API group standardized to kubernaut.io/v1alpha1, async audit (ADR-038) | [ADR-038](../../../architecture/decisions/ADR-038-async-buffered-audit-ingestion.md) |
> | v1.1 | 2025-11-27 | Service rename: RemediationProcessing → SignalProcessing | [DD-SIGNAL-PROCESSING-001](../../../architecture/decisions/DD-SIGNAL-PROCESSING-001-service-rename.md) |
> | v1.1 | 2025-11-27 | Context API removed (deprecated) | [DD-CONTEXT-006](../../../architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md) |
> | v1.1 | 2025-11-27 | Categorization consolidated in Signal Processing | [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md) |
> | v1.1 | 2025-11-27 | Data access via Data Storage Service REST API | [ADR-032](../../../architecture/decisions/ADR-032-data-access-layer-isolation.md) |
> | v1.0 | 2025-01-15 | Initial integration points | - |

**Updated**: November 27, 2025
**Downstream Impact**: Ultra-Compact JSON (DD-HOLMESGPT-009)

**Note**: Signal Processing prepares enriched context that is consumed by AIAnalysis Controller and formatted as **self-documenting JSON** for HolmesGPT API calls. While this service doesn't directly interact with HolmesGPT, its enrichment quality enables 60% token reduction in downstream AI analysis.

### 1. Upstream Integration: RemediationRequest Controller

**Integration Pattern**: Watch-based status coordination

**How SignalProcessing is Created**:
```go
// In RemediationRequestReconciler (Remediation Orchestrator)
func (r *RemediationRequestReconciler) reconcileSignalProcessing(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
) error {
    // When RemediationRequest is first created, create SignalProcessing
    if remediation.Status.SignalProcessingRef == nil {
        signalProcessing := &signalprocessingv1.SignalProcessing{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("%s-processing", remediation.Name),
                Namespace: remediation.Namespace,
                OwnerReferences: []metav1.OwnerReference{
                    *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
                },
            },
            Spec: signalprocessingv1.SignalProcessingSpec{
                RemediationRequestRef: signalprocessingv1.RemediationRequestReference{
                    Name:      remediation.Name,
                    Namespace: remediation.Namespace,
                },
                // Copy original signal data
                Signal: signalprocessingv1.Signal{
                    Fingerprint: remediation.Spec.SignalFingerprint,
                    Payload:     remediation.Spec.OriginalPayload,
                    Severity:    remediation.Spec.Severity,
                },
            },
        }

        return r.Create(ctx, signalProcessing)
    }

    return nil
}
```

**Recovery Flow (with embedded failureData)**:
```go
// In RemediationRequestReconciler - creating recovery SignalProcessing
func (r *RemediationRequestReconciler) reconcileRecoverySignalProcessing(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    failedWorkflow *workflowv1.WorkflowExecution,
) error {
    signalProcessing := &signalprocessingv1.SignalProcessing{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-recovery-%d", remediation.Name, remediation.Status.RecoveryAttemptNumber),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: signalprocessingv1.SignalProcessingSpec{
            RemediationRequestRef: signalprocessingv1.RemediationRequestReference{
                Name:      remediation.Name,
                Namespace: remediation.Namespace,
            },
            Signal: signalprocessingv1.Signal{
                Fingerprint: remediation.Spec.SignalFingerprint,
                Payload:     remediation.Spec.OriginalPayload,
                Severity:    remediation.Spec.Severity,
            },
            // Recovery-specific fields
            IsRecoveryAttempt:     true,
            RecoveryAttemptNumber: remediation.Status.RecoveryAttemptNumber,
            FailedWorkflowRef:     &corev1.LocalObjectReference{Name: failedWorkflow.Name},
            FailedStep:            &failedWorkflow.Status.FailedStep,
            FailureReason:         &failedWorkflow.Status.FailureReason,

            // Embedded failure data (replaces Context API per DD-CONTEXT-006)
            FailureData: &signalprocessingv1.FailureData{
                WorkflowRef:   failedWorkflow.Name,
                AttemptNumber: remediation.Status.RecoveryAttemptNumber - 1,
                FailedStep:    failedWorkflow.Status.FailedStep,
                Action:        failedWorkflow.Status.FailedAction,
                ErrorType:     failedWorkflow.Status.ErrorType,
                FailureReason: failedWorkflow.Status.FailureReason,
                Duration:      failedWorkflow.Status.Duration,
                FailedAt:      failedWorkflow.Status.CompletionTime,
                ResourceState: failedWorkflow.Status.ResourceSnapshot,
            },
        },
    }

    return r.Create(ctx, signalProcessing)
}
```

**Note**: RemediationRequest controller creates SignalProcessing CRD when remediation workflow starts. For recovery attempts, it embeds `failureData` from the failed WorkflowExecution CRD (Context API deprecated per DD-CONTEXT-006).

### 2. Downstream Integration: RemediationRequest Watches SignalProcessing Status

**Integration Pattern**: Data snapshot - RemediationRequest creates AIAnalysis after SignalProcessing completes

**How RemediationRequest Responds to Completion**:
```go
// In RemediationRequestReconciler (Remediation Orchestrator)
// Updated per DD-WORKFLOW-001 v1.8: Includes OwnerChain, DetectedLabels, CustomLabels
func (r *RemediationRequestReconciler) reconcileAIAnalysis(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    signalProcessing *signalprocessingv1.SignalProcessing,
) error {
    // When SignalProcessing completes, create AIAnalysis with enriched data
    if signalProcessing.Status.Phase == "completed" && remediation.Status.AIAnalysisRef == nil {
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
                // COPY enriched signal data (data snapshot pattern - TARGETING DATA ONLY)
                AnalysisRequest: aianalysisv1.AnalysisRequest{
                    SignalContext: aianalysisv1.SignalContext{
                        Fingerprint:         signalProcessing.Status.EnrichmentResults.Fingerprint,
                        Severity:            signalProcessing.Spec.Signal.Severity,
                        Environment:         signalProcessing.Status.EnvironmentClassification.Environment,
                        BusinessCriticality: signalProcessing.Status.EnvironmentClassification.BusinessCriticality,
                        Priority:            signalProcessing.Status.Categorization.Priority,

                        // Resource targeting for HolmesGPT (NOT logs/metrics - toolsets fetch those)
                        Namespace:    signalProcessing.Spec.Signal.Namespace,
                        ResourceKind: signalProcessing.Status.EnrichmentResults.KubernetesContext.ResourceKind,
                        ResourceName: signalProcessing.Status.EnrichmentResults.KubernetesContext.ResourceName,

                        // Kubernetes context (small data ~8KB)
                        KubernetesContext: signalProcessing.Status.EnrichmentResults.KubernetesContext,

                        // Recovery context if present
                        RecoveryContext: signalProcessing.Status.EnrichmentResults.RecoveryContext,

                        // ========================================
                        // LABEL DETECTION (DD-WORKFLOW-001 v1.8)
                        // ========================================

                        // Owner chain for DetectedLabels validation (**ADR-055: removed**)
                        // HolmesGPT-API validates RCA resource is in this chain
                        // If not in chain → DetectedLabels excluded from workflow filtering
                        OwnerChain: signalProcessing.Status.EnrichmentResults.OwnerChain,

                        // Auto-detected cluster characteristics (V1.0) (**ADR-056: removed, now computed by HAPI post-RCA**)
                        // Dual-use: Always in LLM prompt, conditionally in workflow filter
                        DetectedLabels: signalProcessing.Status.EnrichmentResults.DetectedLabels,

                        // Custom labels from Rego policies (V1.0)
                        // Format: map[string][]string (subdomain → list of values)
                        // Passed to Data Storage for workflow filtering
                        CustomLabels: signalProcessing.Status.EnrichmentResults.CustomLabels,
                    },
                },
            },
        }

        return r.Create(ctx, aiAnalysis)
    }

    return nil
}
```

**Note**: SignalProcessing does NOT create AIAnalysis CRD. RemediationRequest controller watches SignalProcessing status and creates AIAnalysis when processing completes.

### 3. External Service Integration: Enrichment Service

**Integration Pattern**: Synchronous HTTP REST API call

**Endpoint**: Kubernetes API (via controller-runtime client)

**Purpose**: Retrieve Kubernetes resource context for signal enrichment

**Request**:
```go
type EnrichmentRequest struct {
    Namespace     string            `json:"namespace"`
    ResourceKind  string            `json:"resourceKind"`
    ResourceName  string            `json:"resourceName"`
    SignalLabels  map[string]string `json:"signalLabels"`
}
```

**Response**:
```go
type EnrichmentResponse struct {
    PodDetails        PodDetails        `json:"podDetails"`
    DeploymentDetails DeploymentDetails `json:"deploymentDetails"`
    NodeDetails       NodeDetails       `json:"nodeDetails"`
    RelatedResources  []RelatedResource `json:"relatedResources"`
}
```

**Degraded Mode Fallback**:
```go
// If enrichment service unavailable, extract minimal context from signal labels
func (e *Enricher) DegradedModeEnrich(signal Signal) EnrichmentResults {
    return EnrichmentResults{
        KubernetesContext: &KubernetesContext{
            Namespace: signal.Labels["namespace"],
            PodDetails: &PodDetails{
                Name: signal.Labels["pod"],
            },
        },
        EnrichmentQuality: 0.5, // Low quality fallback
    }
}
```

### 4. Context API Integration - DEPRECATED

> **⚠️ DEPRECATED per [DD-CONTEXT-006](../../../architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md)**
>
> Signal Processing NO LONGER queries Context API for recovery context.
> Recovery context is now embedded by Remediation Orchestrator in `spec.failureData`.
>
> **Rationale**:
> - Eliminates external dependency for recovery
> - Simplifies architecture
> - Remediation Orchestrator has direct access to WorkflowExecution CRD
> - All failure data available at CRD creation time

**Previous Integration** (removed):
```go
// DEPRECATED - DO NOT USE
// Context API queries are no longer performed
// See: DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md
```

**New Integration Pattern**:
```go
// Recovery context now read from spec.failureData (embedded by Remediation Orchestrator)
func (r *SignalProcessingReconciler) reconcileEnriching(ctx context.Context, sp *signalprocessingv1.SignalProcessing) {
    if sp.Spec.IsRecoveryAttempt && sp.Spec.FailureData != nil {
        // Build recovery context from embedded data
        recoveryCtx := &signalprocessingv1.RecoveryContext{
            ContextQuality: "complete",
            PreviousFailure: &signalprocessingv1.PreviousFailure{
                WorkflowRef:   sp.Spec.FailureData.WorkflowRef,
                AttemptNumber: sp.Spec.FailureData.AttemptNumber,
                FailedStep:    sp.Spec.FailureData.FailedStep,
                Action:        sp.Spec.FailureData.Action,
                ErrorType:     sp.Spec.FailureData.ErrorType,
                FailureReason: sp.Spec.FailureData.FailureReason,
                Duration:      sp.Spec.FailureData.Duration,
                Timestamp:     sp.Spec.FailureData.FailedAt,
                ResourceState: sp.Spec.FailureData.ResourceState,
            },
            ProcessedAt: metav1.Now(),
        }
        sp.Status.EnrichmentResults.RecoveryContext = recoveryCtx
    }
}
```

---

### 5. Database Integration: Data Storage Service (ADR-032)

**Integration Pattern**: Audit trail persistence via REST API

> **📋 ADR-032: Data Access Layer Isolation**
> Only Data Storage Service connects directly to PostgreSQL.
> Signal Processing uses Data Storage Service REST API for audit writes.
> **See**: [ADR-032](../../../architecture/decisions/ADR-032-data-access-layer-isolation.md)

**Endpoint**: Data Storage Service `POST /api/v1/audit/signal-processing`

**Audit Record**:
```go
type SignalProcessingAudit struct {
    SignalFingerprint    string                     `json:"signalFingerprint"`
    ProcessingStartTime  time.Time                  `json:"processingStartTime"`
    ProcessingEndTime    time.Time                  `json:"processingEndTime"`
    EnrichmentResult     *EnrichmentResults         `json:"enrichmentResult"`
    ClassificationResult *EnvironmentClassification `json:"classificationResult"`
    Categorization       *Categorization            `json:"categorization"`
    DegradedMode         bool                       `json:"degradedMode"`
    IsRecoveryAttempt    bool                       `json:"isRecoveryAttempt"`
}
```

**Integration Code**:
```go
// DataStorageClient interface for audit trail persistence
type DataStorageClient interface {
    CreateAuditRecord(ctx context.Context, audit *SignalProcessingAudit) error
}

// Implementation using HTTP client
type dataStorageClientImpl struct {
    httpClient *http.Client
    baseURL    string
}

func (c *dataStorageClientImpl) CreateAuditRecord(ctx context.Context, audit *SignalProcessingAudit) error {
    url := fmt.Sprintf("%s/api/v1/audit/signal-processing", c.baseURL)
    body, _ := json.Marshal(audit)

    req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("data storage request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusCreated {
        return fmt.Errorf("data storage returned status %d", resp.StatusCode)
    }

    return nil
}
```

**Note**: Signal Processing does NOT connect directly to PostgreSQL. All audit persistence goes through Data Storage Service REST API per ADR-032.

### 6. Dependencies Summary

**Upstream Services**:
- **Gateway Service** - Creates RemediationRequest CRD with duplicate detection already performed (BR-WH-008), sets placeholder priority
- **RemediationRequest Controller** - Creates SignalProcessing CRD when workflow starts (initial & recovery), embeds `failureData` for recovery

**Downstream Services**:
- **RemediationRequest Controller** - Watches SignalProcessing status and creates AIAnalysis CRD upon completion

**External Services**:
- **Kubernetes API** - For resource context enrichment (pods, deployments, nodes)
- **Data Storage Service** - REST API for audit trail persistence (ADR-032)

**Deprecated Services**:
- ~~**Context API**~~ - Removed per DD-CONTEXT-006 (recovery context now embedded by Remediation Orchestrator)

**Database**:
- ~~PostgreSQL - Direct access~~ - **REMOVED** per ADR-032
- Data Storage Service REST API - Audit persistence

**Existing Code to Leverage** (after migration to `pkg/signalprocessing/`):
- `pkg/signalprocessing/` - Signal processing business logic
  - `service.go` - SignalProcessingService interface
  - `implementation.go` - Service implementation
  - `components.go` - Processing components (SignalEnricher, EnvironmentClassifier, PriorityCategorizer)
- `pkg/processor/environment/classifier.go` - Environment classification
- `pkg/processor/priority/categorizer.go` - Priority categorization (new per DD-CATEGORIZATION-001)

**Code to Move to Gateway Service**:
- `pkg/signal/components.go` → Gateway Service - `SignalDeduplicatorImpl` (fingerprint generation logic reusable)
