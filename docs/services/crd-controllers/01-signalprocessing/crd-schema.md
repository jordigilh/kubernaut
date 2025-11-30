## CRD Schema Specification

> **📋 Changelog**
> | Version | Date | Changes | Reference |
> |---------|------|---------|-----------|
> | v1.4 | 2025-11-30 | Updated to DD-WORKFLOW-001 v1.4 (5 mandatory labels, risk_tolerance customer-derived) | [HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md](HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md) v3.0, [DD-WORKFLOW-001 v1.4](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) |
> | v1.3 | 2025-11-30 | Added DetectedLabels (V1.0) and CustomLabels (V1.0) to EnrichmentResults | [HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md](HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md) v2.0 |
> | v1.2 | 2025-11-28 | API group standardized to kubernaut.io/v1alpha1, file location updated | [001-crd-api-group-rationale.md](../../../architecture/decisions/001-crd-api-group-rationale.md) |
> | v1.1 | 2025-11-27 | Type rename: RemediationProcessing* → SignalProcessing* | [DD-SIGNAL-PROCESSING-001](../../../architecture/decisions/DD-SIGNAL-PROCESSING-001-service-rename.md) |
> | v1.1 | 2025-11-27 | Terminology: Alert → Signal | [ADR-015](../../../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md) |
> | v1.1 | 2025-11-27 | Recovery context: Now embedded by Remediation Orchestrator in spec.failureData | [DD-CONTEXT-006](../../../architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md) |
> | v1.1 | 2025-11-27 | Categorization: Priority fields added (consolidated from Gateway) | [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md) |
> | v1.0 | 2025-01-15 | Initial CRD schema | - |

**Full Schema**: See [docs/design/CRD/02_SIGNAL_PROCESSING_CRD.md](../../design/CRD/02_SIGNAL_PROCESSING_CRD.md)

**Note**: The examples below show the conceptual structure. The authoritative OpenAPI v3 schema is defined in `02_SIGNAL_PROCESSING_CRD.md`.

**Location**: `api/kubernaut.io/v1alpha1/signalprocessing_types.go`

**API Group**: `kubernaut.io/v1alpha1` (unified API group for all Kubernaut CRDs)

### ✅ **TYPE SAFETY COMPLIANCE**

This CRD specification uses **fully structured types** and eliminates all `map[string]interface{}` anti-patterns:

| Type | Previous (Anti-Pattern) | Current (Type-Safe) | Benefit |
|------|------------------------|---------------------|---------|
| **KubernetesContext** | `map[string]interface{}` | 14 structured types | Compile-time safety, OpenAPI validation |
| **HistoricalContext** | `map[string]interface{}` | Structured type | Clear data contract |
| **ProcessingPhase.Results** | `map[string]interface{}` | 3 phase-specific types | Database query performance |

**Related Triage**: See `SIGNAL_PROCESSING_TYPE_SAFETY_TRIAGE.md` for detailed analysis and remediation plan.

```go
package v1alpha1

import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=sp
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.status.environmentClassification.environment`
// +kubebuilder:printcolumn:name="Priority",type=string,JSONPath=`.status.categorization.priority`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SignalProcessingSpec defines the desired state of SignalProcessing
type SignalProcessingSpec struct {
    // RemediationRequestRef references the parent RemediationRequest CRD
    RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

    // Signal contains the raw signal payload from webhook
    Signal Signal `json:"signal"`

    // EnrichmentConfig specifies enrichment sources and depth
    EnrichmentConfig EnrichmentConfig `json:"enrichmentConfig,omitempty"`

    // EnvironmentClassification config for namespace classification
    EnvironmentClassification EnvironmentClassificationConfig `json:"environmentClassification,omitempty"`

    // ========================================
    // RECOVERY FIELDS
    // 📋 Design Decision: DD-001 | ✅ Approved Design
    // UPDATE (2025-11-11): Context API DEPRECATED per DD-CONTEXT-006
    // Recovery context now embedded by Remediation Orchestrator in failureData
    // See: docs/architecture/decisions/DD-001-recovery-context-enrichment.md
    // See: docs/architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md
    // ========================================

    // IsRecoveryAttempt indicates this is a recovery attempt (not initial processing)
    // When true, Signal Processing reads recovery context from FailureData (embedded by Remediation Orchestrator)
    IsRecoveryAttempt bool `json:"isRecoveryAttempt,omitempty"`

    // RecoveryAttemptNumber tracks which recovery attempt this is (1, 2, 3)
    RecoveryAttemptNumber int `json:"recoveryAttemptNumber,omitempty"`

    // FailedWorkflowRef references the WorkflowExecution that failed
    FailedWorkflowRef *corev1.LocalObjectReference `json:"failedWorkflowRef,omitempty"`

    // FailedStep indicates which workflow step failed (0-based index)
    FailedStep *int `json:"failedStep,omitempty"`

    // FailureReason contains the human-readable failure reason
    FailureReason *string `json:"failureReason,omitempty"`

    // OriginalProcessingRef references the initial SignalProcessing CRD
    // (for audit trail - links recovery attempts back to original)
    OriginalProcessingRef *corev1.LocalObjectReference `json:"originalProcessingRef,omitempty"`

    // FailureData contains embedded failure context from Remediation Orchestrator
    // Populated when IsRecoveryAttempt = true (replaces Context API queries per DD-CONTEXT-006)
    // This data comes from the WorkflowExecution CRD that failed
    FailureData *FailureData `json:"failureData,omitempty"`
}

// Signal represents the signal data from Prometheus/Grafana
// (Renamed from Alert per ADR-015: alert-to-signal-naming-migration)
type Signal struct {
    Fingerprint string            `json:"fingerprint"`
    Payload     map[string]string `json:"payload"`
    Severity    string            `json:"severity"`
    Namespace   string            `json:"namespace"`
    Labels      map[string]string `json:"labels"`
    Annotations map[string]string `json:"annotations"`
}

// FailureData contains failure context embedded by Remediation Orchestrator
// This replaces Context API queries per DD-CONTEXT-006
type FailureData struct {
    // WorkflowRef is the name of the failed WorkflowExecution
    WorkflowRef string `json:"workflowRef"`

    // AttemptNumber is which attempt failed (1, 2, 3)
    AttemptNumber int `json:"attemptNumber"`

    // FailedStep is which step failed (0-based index)
    FailedStep int `json:"failedStep"`

    // Action is the action type that failed (e.g., "scale-deployment")
    Action string `json:"action"`

    // ErrorType is the classified error type ("timeout", "permission_denied", etc.)
    ErrorType string `json:"errorType"`

    // FailureReason is a human-readable failure reason
    FailureReason string `json:"failureReason"`

    // Duration is how long the step ran before failure
    Duration string `json:"duration"`

    // FailedAt is when the failure occurred
    FailedAt metav1.Time `json:"failedAt"`

    // ResourceState contains target resource state at failure time
    ResourceState map[string]string `json:"resourceState,omitempty"`
}

// EnrichmentConfig specifies context enrichment parameters
type EnrichmentConfig struct {
    ContextSources     []string `json:"contextSources,omitempty"`     // ["kubernetes", "historical"]
    ContextDepth       string   `json:"contextDepth,omitempty"`       // "basic", "detailed", "comprehensive"
    HistoricalLookback string   `json:"historicalLookback,omitempty"` // "1h", "24h", "7d"
}

// EnvironmentClassificationConfig for namespace environment detection
type EnvironmentClassificationConfig struct {
    ClassificationSources []string          `json:"classificationSources,omitempty"` // ["labels", "annotations", "configmap", "patterns"]
    ConfidenceThreshold   float64           `json:"confidenceThreshold,omitempty"`   // 0.8
    BusinessRules         map[string]string `json:"businessRules,omitempty"`
}

// SignalProcessingStatus defines the observed state
type SignalProcessingStatus struct {
    // Phase tracks current processing stage
    Phase string `json:"phase"` // "enriching", "classifying", "categorizing", "completed"

    // EnrichmentResults contains context data gathered
    EnrichmentResults EnrichmentResults `json:"enrichmentResults,omitempty"`

    // EnvironmentClassification result with confidence
    EnvironmentClassification EnvironmentClassification `json:"environmentClassification,omitempty"`

    // Categorization result with priority assignment (DD-CATEGORIZATION-001)
    Categorization Categorization `json:"categorization,omitempty"`

    // RoutingDecision for next service
    RoutingDecision RoutingDecision `json:"routingDecision,omitempty"`

    // ProcessingTime duration for metrics
    ProcessingTime string `json:"processingTime,omitempty"`

    // Conditions for status tracking
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// EnrichmentResults from context gathering
// Updated v1.3: Added DetectedLabels (V1.0) and CustomLabels (V1.1)
type EnrichmentResults struct {
    // Kubernetes resource context (from cluster API queries)
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`

    // Historical context (past signals, resource usage)
    HistoricalContext *HistoricalContext `json:"historicalContext,omitempty"`

    // ========================================
    // LABEL DETECTION (HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md v2.0)
    // ========================================

    // Auto-detected cluster characteristics - NO CONFIG NEEDED (V1.0)
    // SignalProcessing auto-detects these from K8s resources
    // Flow: SignalProcessing → AIAnalysis → HolmesGPT-API → LLM prompt + MCP workflow filter
    DetectedLabels *DetectedLabels `json:"detectedLabels,omitempty"`

    // Custom labels from Rego policies - USER DEFINED (V1.1)
    // For labels we can't auto-detect (business_category, team, region)
    // Extracted via Rego policies during enrichment
    CustomLabels map[string]string `json:"customLabels,omitempty"`

    // Overall enrichment quality score (0.0-1.0)
    // 1.0 = all enrichments successful, 0.0 = all failed
    EnrichmentQuality float64 `json:"enrichmentQuality,omitempty"`

    // RecoveryContext contains failure context for recovery attempts
    // Populated from spec.failureData when IsRecoveryAttempt = true
    // Note: Context API queries deprecated per DD-CONTEXT-006
    RecoveryContext *RecoveryContext `json:"recoveryContext,omitempty"`
}

// DetectedLabels contains auto-detected cluster characteristics (V1.0)
// SignalProcessing populates these automatically from K8s resources
// HolmesGPT-API uses for:
//   - Natural language in LLM prompt (context)
//   - LLM instructs model to include in MCP workflow search request (filtering)
type DetectedLabels struct {
    // ========================================
    // GITOPS MANAGEMENT
    // ========================================
    // True if namespace/deployment is managed by GitOps controller
    // Detection: ArgoCD annotations, Flux labels
    GitOpsManaged bool `json:"gitOpsManaged"`
    // GitOps tool managing this resource
    // +kubebuilder:validation:Enum=argocd;flux;""
    GitOpsTool string `json:"gitOpsTool,omitempty"`

    // ========================================
    // WORKLOAD PROTECTION
    // ========================================
    // True if PodDisruptionBudget exists for this workload
    PDBProtected bool `json:"pdbProtected"`
    // True if HorizontalPodAutoscaler targets this workload
    HPAEnabled bool `json:"hpaEnabled"`

    // ========================================
    // WORKLOAD CHARACTERISTICS
    // ========================================
    // True if StatefulSet or has PVCs attached
    Stateful bool `json:"stateful"`
    // True if managed by Helm (has helm.sh/chart label)
    HelmManaged bool `json:"helmManaged"`

    // ========================================
    // SECURITY POSTURE
    // ========================================
    // True if NetworkPolicy exists in namespace
    NetworkIsolated bool `json:"networkIsolated"`
    // Pod Security Standard level from namespace label
    // +kubebuilder:validation:Enum=privileged;baseline;restricted;""
    PodSecurityLevel string `json:"podSecurityLevel,omitempty"`
    // Service mesh if detected (from sidecar or namespace labels)
    // +kubebuilder:validation:Enum=istio;linkerd;""
    ServiceMesh string `json:"serviceMesh,omitempty"`
}

// KubernetesContext contains Kubernetes resource context (~8KB typical size)
// ✅ TYPE SAFE - Replaces map[string]interface{} anti-pattern
type KubernetesContext struct {
    // Namespace information
    Namespace       string            `json:"namespace"`
    NamespaceLabels map[string]string `json:"namespaceLabels,omitempty"`

    // Pod context
    PodDetails *PodDetails `json:"podDetails,omitempty"`

    // Deployment/workload context
    DeploymentDetails *DeploymentDetails `json:"deploymentDetails,omitempty"`

    // Node context
    NodeDetails *NodeDetails `json:"nodeDetails,omitempty"`

    // Related resources (targeting data only)
    RelatedServices   []ServiceSummary   `json:"relatedServices,omitempty"`
    RelatedIngresses  []IngressSummary   `json:"relatedIngresses,omitempty"`
    RelatedConfigMaps []ConfigMapSummary `json:"relatedConfigMaps,omitempty"`
}

type PodDetails struct {
    Name              string            `json:"name"`
    Phase             string            `json:"phase"` // Running, Pending, Failed
    Labels            map[string]string `json:"labels,omitempty"`
    Annotations       map[string]string `json:"annotations,omitempty"`
    Containers        []ContainerStatus `json:"containers,omitempty"`
    RestartCount      int32             `json:"restartCount"`
    CreationTimestamp string            `json:"creationTimestamp"`
}

type ContainerStatus struct {
    Name         string `json:"name"`
    Image        string `json:"image"`
    Ready        bool   `json:"ready"`
    RestartCount int32  `json:"restartCount"`
    State        string `json:"state"` // running, waiting, terminated
}

type DeploymentDetails struct {
    Name              string            `json:"name"`
    Replicas          int32             `json:"replicas"`
    ReadyReplicas     int32             `json:"readyReplicas"`
    AvailableReplicas int32             `json:"availableReplicas"`
    Strategy          string            `json:"strategy"` // RollingUpdate, Recreate
    Labels            map[string]string `json:"labels,omitempty"`
}

type NodeDetails struct {
    Name        string            `json:"name"`
    Labels      map[string]string `json:"labels,omitempty"`
    Capacity    ResourceList      `json:"capacity"`
    Allocatable ResourceList      `json:"allocatable"`
    Conditions  []NodeCondition   `json:"conditions,omitempty"`
}

type ResourceList struct {
    CPU    string `json:"cpu"`    // e.g., "4000m"
    Memory string `json:"memory"` // e.g., "16Gi"
}

type NodeCondition struct {
    Type   string `json:"type"`   // Ready, MemoryPressure, DiskPressure
    Status string `json:"status"` // True, False, Unknown
}

type ServiceSummary struct {
    Name      string        `json:"name"`
    Type      string        `json:"type"` // ClusterIP, NodePort, LoadBalancer
    ClusterIP string        `json:"clusterIP"`
    Ports     []ServicePort `json:"ports,omitempty"`
}

type ServicePort struct {
    Name       string `json:"name"`
    Port       int32  `json:"port"`
    TargetPort string `json:"targetPort"`
    Protocol   string `json:"protocol"` // TCP, UDP
}

type IngressSummary struct {
    Name  string        `json:"name"`
    Hosts []string      `json:"hosts"`
    Rules []IngressRule `json:"rules,omitempty"`
}

type IngressRule struct {
    Host string `json:"host"`
    Path string `json:"path"`
}

type ConfigMapSummary struct {
    Name string   `json:"name"`
    Keys []string `json:"keys"` // ConfigMap key names (not full data)
}

type HistoricalContext struct {
    // Historical signal patterns
    PreviousSignals     int     `json:"previousSignals"`
    LastSignalTimestamp string  `json:"lastSignalTimestamp,omitempty"`
    SignalFrequency     float64 `json:"signalFrequency"` // signals per hour

    // Historical resource usage
    AverageMemoryUsage string `json:"averageMemoryUsage,omitempty"` // e.g., "3.2Gi"
    AverageCPUUsage    string `json:"averageCPUUsage,omitempty"`    // e.g., "1.5 cores"

    // Historical success rate
    LastSuccessfulResolution string  `json:"lastSuccessfulResolution,omitempty"`
    ResolutionSuccessRate    float64 `json:"resolutionSuccessRate"` // 0.0-1.0
}

// RecoveryContext contains failure context for recovery attempts
// Populated from spec.failureData when IsRecoveryAttempt = true
// Note: Context API queries deprecated per DD-CONTEXT-006
type RecoveryContext struct {
    // Context quality indicator
    ContextQuality string `json:"contextQuality"` // "complete", "partial", "minimal", "degraded"

    // Previous workflow failure (from spec.failureData)
    PreviousFailure *PreviousFailure `json:"previousFailure,omitempty"`

    // When this context was processed
    ProcessedAt metav1.Time `json:"processedAt"`
}

// PreviousFailure describes the workflow execution failure
// Data comes from spec.failureData (embedded by Remediation Orchestrator)
type PreviousFailure struct {
    WorkflowRef    string      `json:"workflowRef"`              // Failed WorkflowExecution name
    AttemptNumber  int         `json:"attemptNumber"`            // 1, 2, 3
    FailedStep     int         `json:"failedStep"`               // Which step failed (0-based)
    Action         string      `json:"action"`                   // Action type (e.g., "scale-deployment")
    ErrorType      string      `json:"errorType"`                // Classified error ("timeout", "permission_denied", etc.)
    FailureReason  string      `json:"failureReason"`            // Human-readable reason
    Duration       string      `json:"duration"`                 // How long before failure (e.g., "5m3s")
    Timestamp      metav1.Time `json:"timestamp"`                // When it failed
    ResourceState  map[string]string `json:"resourceState,omitempty"` // Target resource state at failure
}

// EnvironmentClassification result
type EnvironmentClassification struct {
    Environment      string  `json:"environment"`      // "production", "staging", "development", "testing"
    Confidence       float64 `json:"confidence"`       // 0.0-1.0
    BusinessCriticality string `json:"businessCriticality"` // "critical", "high", "medium", "low"
    SLARequirement   string  `json:"slaRequirement"`   // "5m", "15m", "30m"
}

// Categorization contains priority assignment results
// Added per DD-CATEGORIZATION-001: all categorization consolidated in Signal Processing
type Categorization struct {
    // Priority is the final priority level (P0-P3)
    Priority string `json:"priority"` // "P0", "P1", "P2", "P3"

    // PriorityScore is the numeric score (0-100) used for ranking
    PriorityScore int `json:"priorityScore"`

    // CategorizationFactors lists what influenced the priority
    CategorizationFactors []CategorizationFactor `json:"categorizationFactors,omitempty"`

    // CategorizationSource indicates how priority was determined
    CategorizationSource string `json:"categorizationSource"` // "enriched_context", "fallback_labels", "default"

    // CategorizationTime is when categorization was performed
    CategorizationTime metav1.Time `json:"categorizationTime"`
}

// CategorizationFactor describes a factor that influenced priority
type CategorizationFactor struct {
    Factor      string  `json:"factor"`      // e.g., "namespace_labels", "workload_type", "environment"
    Value       string  `json:"value"`       // e.g., "production", "deployment", "critical"
    Weight      float64 `json:"weight"`      // 0.0-1.0
    Contribution int    `json:"contribution"` // Points contributed to priority score
}

// RoutingDecision for workflow continuation
type RoutingDecision struct {
    NextService string `json:"nextService"` // "ai-analysis"
    RoutingKey  string `json:"routingKey"`  // Signal fingerprint
    Priority    int    `json:"priority"`    // 0-10 (derived from Categorization)
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// SignalProcessing is the Schema for the signalprocessings API
type SignalProcessing struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   SignalProcessingSpec   `json:"spec,omitempty"`
    Status SignalProcessingStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SignalProcessingList contains a list of SignalProcessing
type SignalProcessingList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []SignalProcessing `json:"items"`
}

func init() {
    SchemeBuilder.Register(&SignalProcessing{}, &SignalProcessingList{})
}
```

---

## Key Changes from Previous Schema

### Service Rename (DD-SIGNAL-PROCESSING-001)
- `RemediationProcessing` → `SignalProcessing`
- `RemediationProcessingSpec` → `SignalProcessingSpec`
- `RemediationProcessingStatus` → `SignalProcessingStatus`
- `RemediationProcessingReconciler` → `SignalProcessingReconciler`

### Terminology Migration (ADR-015)
- `Alert` type → `Signal` type
- `alert` field names → `signal` field names
- `alertFingerprint` → `signalFingerprint`
- `DuplicateAlerts` → `DuplicateSignals`

### Context API Deprecation (DD-CONTEXT-006)
- Removed: `RecoveryContext` populated from Context API queries
- Added: `FailureData` field in spec (embedded by Remediation Orchestrator)
- `RecoveryContext` now populated from `spec.failureData`
- Simplified architecture: no external Context API dependency for recovery

### Categorization Consolidation (DD-CATEGORIZATION-001)
- Added: `Categorization` type in status
- Added: `CategorizationFactor` type
- Added: `categorizing` phase in reconciliation
- Priority now assigned by Signal Processing (not Gateway)

### Data Access Layer (ADR-032)
- Audit writes via Data Storage Service REST API
- No direct PostgreSQL access from Signal Processing

### Label Detection (HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md v2.0)
- Added: `DetectedLabels` struct - auto-detected cluster characteristics (V1.0 priority)
- Added: `CustomLabels` field - user-defined via Rego policies (V1.1)
- Reference: DD-WORKFLOW-001 v1.4 (5 mandatory labels), DD-WORKFLOW-004 v2.1

#### Label Taxonomy (DD-WORKFLOW-001 v1.4)

| Category | Source | Config Required | Examples |
|----------|--------|-----------------|----------|
| **5 Mandatory Labels** | Signal Processing | No (auto/system) | `signal_type`, `severity`, `component`, `environment`, `priority` |
| **Customer-Derived** | Rego policies | Yes | `risk_tolerance`, `business_category`, `team`, `region` |
| **DetectedLabels** | Auto-detection from K8s | ❌ No config | `GitOpsManaged`, `PDBProtected`, `HPAEnabled` |
| **CustomLabels** | Rego policies | ✅ User-defined | `business_category`, `team`, `region` |

#### DetectedLabels (V1.0 - PRIORITY)

| Field | Detection Method | Used For |
|-------|------------------|----------|
| `GitOpsManaged` | ArgoCD/Flux annotations | LLM context + workflow filtering |
| `GitOpsTool` | Specific annotation patterns | Workflow selection preference |
| `PDBProtected` | PDB exists for workload | Risk assessment |
| `HPAEnabled` | HPA targets workload | Scaling context |
| `Stateful` | StatefulSet or PVC | State handling |
| `HelmManaged` | Helm labels present | Deployment method |
| `NetworkIsolated` | NetworkPolicy exists | Security context |
| `PodSecurityLevel` | Namespace PSS label | Security posture |
| `ServiceMesh` | Istio/Linkerd sidecar | Traffic management |

#### CustomLabels (V1.1)

- **Keys**: Defined by customer in Rego policies (e.g., `business_category`, `team`, `region`)
- **Values**: Derived from K8s labels/annotations via Rego policies
- **Matching**: Customer's Rego labels matched against workflow labels
- **Reference**: `signal-processing-policies` ConfigMap in `kubernaut-system` namespace

