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

### 4. Database Integration: Data Storage Service

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
}
```

### 5. Dependencies Summary

**Upstream Services**:
- **Gateway Service** - Creates RemediationRequest CRD with duplicate detection already performed (BR-WH-008)
- **RemediationRequest Controller** - Creates RemediationProcessing CRD when workflow starts

**Downstream Services**:
- **RemediationRequest Controller** - Watches RemediationProcessing status and creates AIAnalysis CRD upon completion

**External Services**:
- **Context Service** - HTTP GET for Kubernetes context enrichment
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

