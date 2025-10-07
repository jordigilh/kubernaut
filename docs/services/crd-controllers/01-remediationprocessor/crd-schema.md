## CRD Schema Specification

**Full Schema**: See [docs/design/CRD/02_ALERT_PROCESSING_CRD.md](../../design/CRD/02_ALERT_PROCESSING_CRD.md)

**Note**: The examples below show the conceptual structure. The authoritative OpenAPI v3 schema is defined in `02_ALERT_PROCESSING_CRD.md`.

**Location**: `api/remediationprocessing/v1/alertprocessing_types.go`

### ✅ **TYPE SAFETY COMPLIANCE**

This CRD specification uses **fully structured types** and eliminates all `map[string]interface{}` anti-patterns:

| Type | Previous (Anti-Pattern) | Current (Type-Safe) | Benefit |
|------|------------------------|---------------------|---------|
| **KubernetesContext** | `map[string]interface{}` | 14 structured types | Compile-time safety, OpenAPI validation |
| **HistoricalContext** | `map[string]interface{}` | Structured type | Clear data contract |
| **ProcessingPhase.Results** | `map[string]interface{}` | 3 phase-specific types | Database query performance |

**Related Triage**: See `ALERT_PROCESSOR_TYPE_SAFETY_TRIAGE.md` for detailed analysis and remediation plan.

```go
package v1

import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RemediationProcessingSpec defines the desired state of RemediationProcessing
type RemediationProcessingSpec struct {
    // RemediationRequestRef references the parent RemediationRequest CRD
    RemediationRequestRef corev1.ObjectReference `json:"alertRemediationRef"`

    // Alert contains the raw alert payload from webhook
    Alert Alert `json:"alert"`

    // EnrichmentConfig specifies enrichment sources and depth
    EnrichmentConfig EnrichmentConfig `json:"enrichmentConfig,omitempty"`

    // EnvironmentClassification config for namespace classification
    EnvironmentClassification EnvironmentClassificationConfig `json:"environmentClassification,omitempty"`
}

// Alert represents the alert data from Prometheus/Grafana
type Alert struct {
    Fingerprint string            `json:"fingerprint"`
    Payload     map[string]string `json:"payload"`
    Severity    string            `json:"severity"`
    Namespace   string            `json:"namespace"`
    Labels      map[string]string `json:"labels"`
    Annotations map[string]string `json:"annotations"`
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

// RemediationProcessingStatus defines the observed state
type RemediationProcessingStatus struct {
    // Phase tracks current processing stage
    Phase string `json:"phase"` // "enriching", "classifying", "routing", "completed"

    // EnrichmentResults contains context data gathered
    EnrichmentResults EnrichmentResults `json:"enrichmentResults,omitempty"`

    // EnvironmentClassification result with confidence
    EnvironmentClassification EnvironmentClassification `json:"environmentClassification,omitempty"`

    // RoutingDecision for next service
    RoutingDecision RoutingDecision `json:"routingDecision,omitempty"`

    // ProcessingTime duration for metrics
    ProcessingTime string `json:"processingTime,omitempty"`

    // Conditions for status tracking
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// EnrichmentResults from context gathering
type EnrichmentResults struct {
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    HistoricalContext *HistoricalContext `json:"historicalContext,omitempty"`
    EnrichmentQuality float64            `json:"enrichmentQuality,omitempty"` // 0.0-1.0
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
    // Historical alert patterns
    PreviousAlerts     int     `json:"previousAlerts"`
    LastAlertTimestamp string  `json:"lastAlertTimestamp,omitempty"`
    AlertFrequency     float64 `json:"alertFrequency"` // alerts per hour

    // Historical resource usage
    AverageMemoryUsage string `json:"averageMemoryUsage,omitempty"` // e.g., "3.2Gi"
    AverageCPUUsage    string `json:"averageCPUUsage,omitempty"`    // e.g., "1.5 cores"

    // Historical success rate
    LastSuccessfulResolution string  `json:"lastSuccessfulResolution,omitempty"`
    ResolutionSuccessRate    float64 `json:"resolutionSuccessRate"` // 0.0-1.0
}

// EnvironmentClassification result
type EnvironmentClassification struct {
    Environment      string  `json:"environment"`      // "production", "staging", "development", "testing"
    Confidence       float64 `json:"confidence"`       // 0.0-1.0
    BusinessPriority string  `json:"businessPriority"` // "P0", "P1", "P2", "P3"
    SLARequirement   string  `json:"slaRequirement"`   // "5m", "15m", "30m"
}

// RoutingDecision for workflow continuation
type RoutingDecision struct {
    NextService string `json:"nextService"` // "ai-analysis"
    RoutingKey  string `json:"routingKey"`  // Alert fingerprint
    Priority    int    `json:"priority"`    // 0-10
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// RemediationProcessing is the Schema for the alertprocessings API
type RemediationProcessing struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   RemediationProcessingSpec   `json:"spec,omitempty"`
    Status RemediationProcessingStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RemediationProcessingList contains a list of RemediationProcessing
type RemediationProcessingList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []RemediationProcessing `json:"items"`
}

func init() {
    SchemeBuilder.Register(&RemediationProcessing{}, &RemediationProcessingList{})
}
```

