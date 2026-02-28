# Authoritative CRD Schemas

**Version**: v1.1
**Last Updated**: December 7, 2025
**Status**: ✅ Authoritative Reference

---

## 📋 Overview

This document defines the **authoritative CRD schemas** for the Kubernaut project. All services must reference and implement these schemas exactly as specified.

**Schema Authority** (Updated per [ADR-049](decisions/ADR-049-remediationrequest-crd-ownership.md)):
- **Remediation Orchestrator (RO)** owns the RemediationRequest CRD schema definition
- Gateway Service creates RR instances by importing RO's types
- All other services consume CRDs according to these specifications

---

## 🎯 RemediationRequest CRD

### Metadata

**API Group**: `remediation.kubernaut.ai/v1alpha1`
**Kind**: `RemediationRequest`
**Schema Owner**: Remediation Orchestrator (RO) - per [ADR-049](decisions/ADR-049-remediationrequest-crd-ownership.md)
**Instances Created By**: Gateway Service
**Scope**: Namespaced

### Purpose

Entry point for the remediation workflow. Gateway Service creates one RemediationRequest CRD per unique signal (Prometheus alert, Kubernetes event). Remediation Orchestrator owns the schema and orchestrates downstream service CRDs based on this request.

### Ownership Clarification (ADR-049)

| Aspect | Owner |
|--------|-------|
| **Schema Definition** | Remediation Orchestrator (RO) |
| **Instance Creation** | Gateway Service |
| **Reconciliation** | Remediation Orchestrator (RO) |
| **Status (Deduplication, Storm)** | Gateway Service (per DD-GATEWAY-011) |
| **Status (Lifecycle)** | Remediation Orchestrator (RO) |

**Why RO Owns the Schema**:
- RO is the controller that reconciles RR (K8s pattern: reconciler owns schema)
- RR represents remediation lifecycle, which is RO's domain
- Clean dependency direction: Gateway → RO types (not reverse)
- RO can evolve schema based on orchestration needs

**Gateway's Role**:
- Creates RR instances using types imported from RO
- Owns `status.deduplication` and `status.stormAggregation` (per DD-GATEWAY-011)
- Performs deduplication and storm detection before instance creation

### Multi-Provider Architecture

**V1 Design Decision**: **Alternative 1 - Raw JSON Provider Data** (90% confidence)

**V1 Scope** (Current Implementation):
- ✅ **Kubernetes ONLY**: Prometheus alerts, Kubernetes events
- ⏸️ AWS: CloudWatch alerts (**V2 - Planned, structures preserved**)
- ⏸️ Azure: Azure Monitor alerts (**V2 - Planned, structures preserved**)
- ⏸️ Datadog: Datadog monitors (**V2 - Planned, structures preserved**)
- ⏸️ GCP: Cloud Monitoring alerts (**V2 - Planned, structures preserved**)
- ⏸️ Custom: Any custom monitoring platform (**V2 - Planned**)

**Architecture**: Universal typed fields + raw JSON provider data
- Core routing fields (priority, severity, environment) are strongly typed
- Provider-specific data stored in `providerData` JSON field
- All providers treated consistently (including Kubernetes)
- **V1 Implementation**: `targetType="kubernetes"` only
- **V2 Expansion**: AWS, Azure, Datadog, GCP support (schema ready, implementation pending)

**Schema Design**: No V2 version bump required - schema supports all providers

**🚨 IMPORTANT FOR CODE TRIAGES**:
- AWS, Azure, Datadog, GCP provider structures are **VALID FOR V2**
- **DO NOT DELETE** non-Kubernetes provider code during unused code cleanup
- These structures are intentionally forward-looking and preserved for V2

See: `docs/architecture/MULTI_PROVIDER_CRD_ALTERNATIVES.md` for alternatives analysis

---

## 📐 RemediationRequest Spec

```go
// pkg/apis/remediation/v1/remediationrequest_types.go
package v1

import (
    "encoding/json"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RemediationRequestSpec defines the desired state of RemediationRequest
type RemediationRequestSpec struct {
    // ========================================
    // UNIVERSAL FIELDS (ALL SIGNALS)
    // These fields are populated for EVERY signal regardless of provider
    // ========================================

    // Core Signal Identification
    // Unique fingerprint for deduplication (SHA256 of alert/event key fields)
    // +kubebuilder:validation:MaxLength=64
    // +kubebuilder:validation:Pattern="^[a-f0-9]{64}$"
    SignalFingerprint string `json:"signalFingerprint"`

    // Human-readable signal name (e.g., "HighMemoryUsage", "CrashLoopBackOff")
    // +kubebuilder:validation:MaxLength=253
    SignalName string `json:"signalName"`

    // Signal Classification
    // Severity level: "critical", "warning", "info"
    // +kubebuilder:validation:Enum=critical;warning;info
    Severity string `json:"severity"`

    // Environment: "prod", "staging", "dev"
    // +kubebuilder:validation:Enum=prod;staging;dev
    Environment string `json:"environment"`

    // Priority assigned by Gateway (P0=critical, P1=high, P2=normal)
    // Used by downstream Rego policies for remediation decisions
    // +kubebuilder:validation:Enum=P0;P1;P2
    // +kubebuilder:validation:Pattern="^P[0-2]$"
    Priority string `json:"priority"`

    // Signal type: "prometheus", "kubernetes-event", "aws-cloudwatch", "datadog-monitor", etc.
    // Used for signal-aware remediation strategies
    SignalType string `json:"signalType"`

    // Adapter that ingested the signal (e.g., "prometheus-adapter", "k8s-event-adapter")
    // +kubebuilder:validation:MaxLength=63
    SignalSource string `json:"signalSource,omitempty"`

    // Target system type: "kubernetes", "aws", "azure", "gcp", "datadog"
    // Indicates which infrastructure system the signal targets
    // +kubebuilder:validation:Enum=kubernetes;aws;azure;gcp;datadog
    TargetType string `json:"targetType"`

    // Temporal Data
    // When the signal first started firing (from upstream source)
    FiringTime metav1.Time `json:"firingTime"`

    // When Gateway received the signal
    ReceivedTime metav1.Time `json:"receivedTime"`

    // Deduplication Metadata
    // Tracking information for duplicate signal suppression
    Deduplication DeduplicationInfo `json:"deduplication"`

    // Storm Detection
    // True if this signal is part of a detected alert storm
    IsStorm bool `json:"isStorm,omitempty"`

    // Storm type: "rate" (frequency-based) or "pattern" (similar alerts)
    StormType string `json:"stormType,omitempty"`

    // Time window for storm detection (e.g., "5m")
    StormWindow string `json:"stormWindow,omitempty"`

    // Number of alerts in the storm
    StormAlertCount int `json:"stormAlertCount,omitempty"`

    // ========================================
    // SIGNAL METADATA (PHASE 1 ADDITION)
    // Structured metadata extracted from provider-specific data
    // ========================================

    // Signal labels extracted from provider-specific data
    // For Prometheus: alert.Labels (e.g., {"alertname": "HighMemory", "namespace": "prod", "severity": "critical"})
    // For Kubernetes events: Event labels (e.g., {"reason": "CrashLoopBackOff", "kind": "Pod"})
    // For AWS CloudWatch: Alarm tags (e.g., {"AlarmName": "HighCPU", "Environment": "production"})
    //
    // Purpose: Provide structured access to signal metadata without parsing providerData
    // Gateway Service populates this using pkg/gateway/signal_extraction.go helpers
    // RemediationProcessing and downstream services use this for decision-making
    //
    // Note: Labels are sanitized for Kubernetes compliance (63 char max per value)
    SignalLabels map[string]string `json:"signalLabels,omitempty"`

    // Signal annotations extracted from provider-specific data
    // For Prometheus: alert.Annotations (e.g., {"summary": "Memory usage above 90%", "description": "Pod xyz..."})
    // For Kubernetes events: Event message/reason (e.g., {"message": "Container crashed", "reason": "Error"})
    // For AWS CloudWatch: Alarm description (e.g., {"AlarmDescription": "CPU threshold exceeded"})
    //
    // Purpose: Provide human-readable descriptions and context without parsing providerData
    // Gateway Service populates this using pkg/gateway/signal_extraction.go helpers
    // Used by notification services, AI analysis, and audit logging
    //
    // Note: Annotations have practical size limit (256KB) for storage efficiency
    SignalAnnotations map[string]string `json:"signalAnnotations,omitempty"`

    // ========================================
    // PROVIDER-SPECIFIC DATA
    // All provider-specific fields go here (INCLUDING Kubernetes)
    // ========================================

    // Provider-specific fields in raw JSON format
    // Gateway adapter populates this based on signal source
    // Controllers parse this based on targetType/signalType
    //
    // For Kubernetes (targetType="kubernetes"):
    //   {"namespace": "...", "resource": {"kind": "...", "name": "..."}, "alertmanagerURL": "...", ...}
    //
    // For AWS (targetType="aws"):
    //   {"region": "...", "accountId": "...", "instanceId": "...", "resourceType": "...", ...}
    //
    // For Datadog (targetType="datadog"):
    //   {"monitorId": 123, "host": "...", "tags": [...], "metricQuery": "...", ...}
    //
    // See below for complete provider data schemas
    ProviderData json.RawMessage `json:"providerData,omitempty"`

    // ========================================
    // AUDIT/DEBUG
    // ========================================

    // Complete original webhook payload for debugging and audit
    // Stored as []byte to preserve exact format
    OriginalPayload []byte `json:"originalPayload,omitempty"`

    // ========================================
    // WORKFLOW CONFIGURATION
    // ========================================

    // Optional timeout overrides for this specific remediation
    TimeoutConfig *TimeoutConfig `json:"timeoutConfig,omitempty"`
}

// DeduplicationInfo tracks duplicate signal suppression
type DeduplicationInfo struct {
    // True if this signal is a duplicate of an active remediation
    IsDuplicate bool `json:"isDuplicate"`

    // Timestamp when this signal fingerprint was first seen
    FirstSeen metav1.Time `json:"firstSeen"`

    // Timestamp when this signal fingerprint was last seen
    LastSeen metav1.Time `json:"lastSeen"`

    // Total count of occurrences of this signal
    OccurrenceCount int `json:"occurrenceCount"`

    // Reference to previous RemediationRequest CRD (if duplicate)
    PreviousRemediationRequestRef string `json:"previousRemediationRequestRef,omitempty"`
}

// TimeoutConfig allows per-remediation timeout customization
type TimeoutConfig struct {
    // Timeout for RemediationProcessing phase (default: 5m)
    RemediationProcessingTimeout metav1.Duration `json:"remediationProcessingTimeout,omitempty"`

    // Timeout for AIAnalysis phase (default: 10m)
    AIAnalysisTimeout metav1.Duration `json:"aiAnalysisTimeout,omitempty"`

    // Timeout for WorkflowExecution phase (default: 20m)
    WorkflowExecutionTimeout metav1.Duration `json:"workflowExecutionTimeout,omitempty"`

    // Overall workflow timeout (default: 1h)
    OverallWorkflowTimeout metav1.Duration `json:"overallWorkflowTimeout,omitempty"`
}
```

---

## 📊 RemediationRequest Status

```go
// RemediationRequestStatus defines the observed state of RemediationRequest
type RemediationRequestStatus struct {
    // Overall Workflow State
    // Phase: "pending", "processing", "analyzing", "executing", "completed", "failed", "timeout"
    OverallPhase string `json:"overallPhase"`

    // When workflow started
    StartTime metav1.Time `json:"startTime"`

    // When workflow completed (if completed/failed/timeout)
    CompletionTime *metav1.Time `json:"completionTime,omitempty"`

    // Service CRD References
    // References to child CRDs created by Central Controller
    RemediationProcessingRef *RemediationProcessingReference `json:"remediationProcessingRef,omitempty"`
    AIAnalysisRef            *AIAnalysisReference            `json:"aiAnalysisRef,omitempty"`
    WorkflowExecutionRef     *WorkflowExecutionReference     `json:"workflowExecutionRef,omitempty"`

    // Aggregated Status Summaries
    // Lightweight status summaries from service CRDs (not full copies)
    RemediationProcessingStatus *RemediationProcessingStatusSummary `json:"remediationProcessingStatus,omitempty"`
    AIAnalysisStatus            *AIAnalysisStatusSummary            `json:"aiAnalysisStatus,omitempty"`
    WorkflowExecutionStatus     *WorkflowExecutionStatusSummary     `json:"workflowExecutionStatus,omitempty"`

    // Timeout Tracking
    // Which phase timed out (if timeout occurred)
    TimeoutPhase *string `json:"timeoutPhase,omitempty"`

    // When timeout occurred
    TimeoutTime *metav1.Time `json:"timeoutTime,omitempty"`

    // Failure Tracking
    // Which phase failed (if failure occurred)
    FailurePhase *string `json:"failurePhase,omitempty"`

    // Detailed failure reason
    FailureReason *string `json:"failureReason,omitempty"`

    // Retention Tracking
    // When 24-hour retention window expires (for cleanup)
    RetentionExpiryTime *metav1.Time `json:"retentionExpiryTime,omitempty"`

    // Duplicate Signal Tracking
    // Number of duplicate signals suppressed while this remediation is active
    DuplicateCount int `json:"duplicateCount"`

    // Last time a duplicate signal was received
    LastDuplicateTime *metav1.Time `json:"lastDuplicateTime,omitempty"`
}

// Service CRD Reference Types
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

// Status Summary Types (Lightweight Aggregation)
type RemediationProcessingStatusSummary struct {
    Phase          string       `json:"phase"`
    CompletionTime *metav1.Time `json:"completionTime,omitempty"`
    Environment    string       `json:"environment,omitempty"`
    DegradedMode   bool         `json:"degradedMode"`
}

type AIAnalysisStatusSummary struct {
    Phase               string       `json:"phase"`
    CompletionTime      *metav1.Time `json:"completionTime,omitempty"`
    RecommendationCount int          `json:"recommendationCount"`
    TopRecommendation   string       `json:"topRecommendation,omitempty"`
}

type WorkflowExecutionStatusSummary struct {
    Phase          string       `json:"phase"`
    CompletionTime *metav1.Time `json:"completionTime,omitempty"`
    TotalSteps     int          `json:"totalSteps"`
    CompletedSteps int          `json:"completedSteps"`
}
```

---

## 📝 Provider Data Schemas

**V1 vs V2 Scope**:
- ✅ **V1 (Current)**: Kubernetes provider only
- ⏸️ **V2 (Future)**: AWS, Azure, Datadog, GCP providers

---

### Kubernetes Provider Data ✅ **V1**

**When `targetType="kubernetes"`**

**Status**: ✅ **V1 Implementation** - Active in current version

```json
{
  "namespace": "production",
  "resource": {
    "kind": "Pod",
    "name": "api-server-xyz-abc123",
    "namespace": "production"
  },
  "alertmanagerURL": "https://alertmanager.example.com/#/alerts?receiver=kubernaut",
  "grafanaURL": "https://grafana.example.com/d/abc123/pod-dashboard",
  "prometheusQuery": "rate(http_requests_total{pod=\"api-server-xyz-abc123\"}[5m]) > 1000"
}
```

**TypeScript/Go Helper Type**:
```go
type KubernetesProviderData struct {
    Namespace       string             `json:"namespace"`
    Resource        ResourceIdentifier `json:"resource"`
    AlertmanagerURL string             `json:"alertmanagerURL,omitempty"`
    GrafanaURL      string             `json:"grafanaURL,omitempty"`
    PrometheusQuery string             `json:"prometheusQuery,omitempty"`
}

type ResourceIdentifier struct {
    Kind      string `json:"kind"`
    Name      string `json:"name"`
    // +optional — empty for cluster-scoped resources (e.g., Node, PersistentVolume)
    Namespace string `json:"namespace,omitempty"`
}
```

**Fields**:
- `namespace` (string, **optional**): Kubernetes namespace where signal originated. Empty for cluster-scoped resources (e.g., Node, PersistentVolume)
- `resource` (object, **required**): Target Kubernetes resource
  - `kind` (string, **required**): Resource kind (Pod, Deployment, StatefulSet, etc.)
  - `name` (string, **required**): Resource name
  - `namespace` (string, **optional**): Resource namespace. Empty for cluster-scoped resources (e.g., Node, PersistentVolume)
- `alertmanagerURL` (string, optional): Link to Alertmanager alert view
- `grafanaURL` (string, optional): Link to Grafana dashboard
- `prometheusQuery` (string, optional): Prometheus query that triggered the alert

---

### AWS Provider Data ⏸️ **V2 - Planned**

**When `targetType="aws"`**

**Status**: ⏸️ **V2 Implementation** - Schema complete, implementation pending

**🚨 DO NOT DELETE**: This structure is valid and preserved for V2. Not unused code.

```json
{
  "region": "us-east-1",
  "accountId": "123456789012",
  "resourceType": "ec2",
  "instanceId": "i-abc123def456",
  "instanceType": "t3.large",
  "availabilityZone": "us-east-1a",
  "tags": {
    "Environment": "production",
    "Service": "api",
    "Team": "platform"
  },
  "metricName": "CPUUtilization",
  "metricNamespace": "AWS/EC2",
  "threshold": 80,
  "evaluationPeriods": 2,
  "cloudWatchURL": "https://console.aws.amazon.com/cloudwatch/home?region=us-east-1#alarmsV2:alarm/HighCPU"
}
```

**Go Helper Type**:
```go
type AWSProviderData struct {
    Region             string            `json:"region"`
    AccountID          string            `json:"accountId"`
    ResourceType       string            `json:"resourceType"`
    InstanceID         string            `json:"instanceId,omitempty"`
    InstanceType       string            `json:"instanceType,omitempty"`
    AvailabilityZone   string            `json:"availabilityZone,omitempty"`
    Tags               map[string]string `json:"tags,omitempty"`
    MetricName         string            `json:"metricName"`
    MetricNamespace    string            `json:"metricNamespace,omitempty"`
    Threshold          float64           `json:"threshold,omitempty"`
    EvaluationPeriods  int               `json:"evaluationPeriods,omitempty"`
    CloudWatchURL      string            `json:"cloudWatchURL,omitempty"`
}
```

**Fields**:
- `region` (string, **required**): AWS region (e.g., "us-east-1")
- `accountId` (string, **required**): AWS account ID
- `resourceType` (string, **required**): AWS resource type ("ec2", "rds", "lambda", "ecs", etc.)
- `instanceId` (string, required for EC2): EC2 instance ID
- `tags` (object, optional): AWS resource tags
- `metricName` (string, **required**): CloudWatch metric name
- `threshold` (number, optional): Alert threshold value
- `cloudWatchURL` (string, optional): Link to CloudWatch console

---

### Azure Provider Data ⏸️ **V2 - Planned**

**When `targetType="azure"`**

**Status**: ⏸️ **V2 Implementation** - Schema complete, implementation pending

**🚨 DO NOT DELETE**: This structure is valid and preserved for V2. Not unused code.

```json
{
  "subscriptionId": "12345678-1234-1234-1234-123456789012",
  "resourceGroup": "production-rg",
  "resourceType": "Microsoft.Compute/virtualMachines",
  "resourceId": "/subscriptions/.../resourceGroups/production-rg/providers/Microsoft.Compute/virtualMachines/vm-prod-01",
  "resourceName": "vm-prod-01",
  "location": "eastus",
  "tags": {
    "Environment": "Production",
    "CostCenter": "Engineering"
  },
  "metricName": "Percentage CPU",
  "threshold": 85,
  "azurePortalURL": "https://portal.azure.com/#@.../resource/.../overview"
}
```

**Go Helper Type**:
```go
type AzureProviderData struct {
    SubscriptionID  string            `json:"subscriptionId"`
    ResourceGroup   string            `json:"resourceGroup"`
    ResourceType    string            `json:"resourceType"`
    ResourceID      string            `json:"resourceId"`
    ResourceName    string            `json:"resourceName"`
    Location        string            `json:"location"`
    Tags            map[string]string `json:"tags,omitempty"`
    MetricName      string            `json:"metricName"`
    Threshold       float64           `json:"threshold,omitempty"`
    AzurePortalURL  string            `json:"azurePortalURL,omitempty"`
}
```

**Fields**:
- `subscriptionId` (string, **required**): Azure subscription ID
- `resourceGroup` (string, **required**): Azure resource group name
- `resourceType` (string, **required**): Azure resource type
- `resourceId` (string, **required**): Full Azure resource ID
- `resourceName` (string, **required**): Resource name
- `location` (string, **required**): Azure region/location
- `metricName` (string, **required**): Azure Monitor metric name
- `azurePortalURL` (string, optional): Link to Azure Portal

---

### Datadog Provider Data ⏸️ **V2 - Planned**

**When `targetType="datadog"`**

**Status**: ⏸️ **V2 Implementation** - Schema complete, implementation pending

**🚨 DO NOT DELETE**: This structure is valid and preserved for V2. Not unused code.

```json
{
  "monitorId": 12345,
  "monitorName": "High Memory Usage",
  "monitorType": "metric alert",
  "host": "prod-web-01.example.com",
  "tags": [
    "env:production",
    "service:api",
    "team:platform"
  ],
  "metricQuery": "avg:system.mem.used{host:prod-web-01}",
  "threshold": 90,
  "thresholdWindows": ["5m", "10m"],
  "datadogURL": "https://app.datadoghq.com/monitors/12345"
}
```

**Go Helper Type**:
```go
type DatadogProviderData struct {
    MonitorID         int64    `json:"monitorId"`
    MonitorName       string   `json:"monitorName"`
    MonitorType       string   `json:"monitorType"`
    Host              string   `json:"host,omitempty"`
    Tags              []string `json:"tags,omitempty"`
    MetricQuery       string   `json:"metricQuery"`
    Threshold         float64  `json:"threshold,omitempty"`
    ThresholdWindows  []string `json:"thresholdWindows,omitempty"`
    DatadogURL        string   `json:"datadogURL,omitempty"`
}
```

**Fields**:
- `monitorId` (number, **required**): Datadog monitor ID
- `monitorName` (string, **required**): Monitor name
- `host` (string, optional): Affected host
- `tags` (array, optional): Datadog tags
- `metricQuery` (string, **required**): Datadog metric query
- `datadogURL` (string, optional): Link to Datadog monitor

---

### GCP Provider Data ⏸️ **V2 - Planned**

**When `targetType="gcp"`**

**Status**: ⏸️ **V2 Implementation** - Schema complete, implementation pending

**🚨 DO NOT DELETE**: This structure is valid and preserved for V2. Not unused code.

```json
{
  "projectId": "my-project-123456",
  "resourceType": "gce_instance",
  "instanceId": "1234567890123456789",
  "zone": "us-central1-a",
  "labels": {
    "environment": "production",
    "service": "api"
  },
  "metricType": "compute.googleapis.com/instance/cpu/utilization",
  "threshold": 0.8,
  "cloudConsoleURL": "https://console.cloud.google.com/monitoring/alerting/policies/..."
}
```

**Go Helper Type**:
```go
type GCPProviderData struct {
    ProjectID       string            `json:"projectId"`
    ResourceType    string            `json:"resourceType"`
    InstanceID      string            `json:"instanceId,omitempty"`
    Zone            string            `json:"zone,omitempty"`
    Labels          map[string]string `json:"labels,omitempty"`
    MetricType      string            `json:"metricType"`
    Threshold       float64           `json:"threshold,omitempty"`
    CloudConsoleURL string            `json:"cloudConsoleURL,omitempty"`
}
```

**Fields**:
- `projectId` (string, **required**): GCP project ID
- `resourceType` (string, **required**): GCP resource type
- `zone` (string, optional): GCP zone
- `labels` (object, optional): GCP resource labels
- `metricType` (string, **required**): Cloud Monitoring metric type
- `cloudConsoleURL` (string, optional): Link to GCP Console

---

## 📋 Field Reference Guide

### Universal Fields (Required for ALL Signals)

These fields are **required** regardless of provider:

| Field | Type | Description |
|-------|------|-------------|
| `signalFingerprint` | string | Unique fingerprint for deduplication |
| `signalName` | string | Human-readable signal name |
| `severity` | string | "critical", "warning", "info" |
| `environment` | string | "prod", "staging", "dev" |
| `priority` | string | "P0", "P1", "P2" |
| `signalType` | string | Provider signal type |
| `targetType` | string | Infrastructure system type |
| `firingTime` | metav1.Time | When signal started firing |
| `receivedTime` | metav1.Time | When Gateway received signal |
| `deduplication` | DeduplicationInfo | Deduplication metadata |

### Universal Fields (Optional for ALL Signals)

These fields are **optional** regardless of provider:

| Field | Type | Description |
|-------|------|-------------|
| `signalSource` | string | Adapter name |
| `isStorm` | bool | Storm detection flag |
| `stormType` | string | "rate" or "pattern" |
| `stormWindow` | string | Time window (e.g., "5m") |
| `stormAlertCount` | int | Number of alerts in storm |
| `originalPayload` | []byte | Raw webhook payload |
| `timeoutConfig` | TimeoutConfig | Custom timeout overrides |

### Provider-Specific Fields

**Required**: `providerData` (json.RawMessage) - Structure varies by `targetType`

See provider data schemas above for each `targetType`

### Field Usage by Service

| Field | Gateway | RemediationProcessor | AIAnalysis | WorkflowExecution |
|-------|---------|---------------------|------------|------------------|
| `signalFingerprint` | ✅ Creates | ✅ Uses for tracking | ✅ Uses for correlation | ✅ Uses for audit |
| `priority` | ✅ Assigns | ❌ N/A | ✅ Uses in Rego | ✅ Uses for ordering |
| `signalType` | ✅ Detects | ❌ N/A | ✅ Uses in Rego | ✅ Uses for strategy |
| `targetType` | ✅ Sets | ✅ Uses for routing | ✅ Uses for toolset selection | ✅ Uses for execution |
| `environment` | ✅ Classifies | ✅ Validates | ✅ Uses in Rego | ✅ Uses for safety |
| `providerData` | ✅ Populates | ✅ Parses for enrichment | ✅ Parses for context | ✅ Parses for targeting |
| `originalPayload` | ✅ Stores | ❌ N/A | ✅ Uses for AI input | ❌ N/A |
| `deduplication` | ✅ Computes | ❌ N/A | ❌ N/A | ❌ N/A |

**Note**: `providerData` parsing depends on `targetType`:
- Controllers use Go helper structs (e.g., `KubernetesProviderData`, `AWSProviderData`)
- Parsing is type-safe at runtime via `json.Unmarshal()`

---

## 🔄 Comparison with Previous Schemas

### Evolution from K8s-Only to Multi-Provider Ready

**V1 Implementation**: Kubernetes only (Prometheus alerts, K8s events)
**V2 Expansion**: AWS, Azure, Datadog, GCP (schema ready, code preserved)

**Previous Design** (K8s-only, typed):
```go
type RemediationRequestSpec struct {
    SignalFingerprint string
    Namespace        string              // K8s-specific, typed
    Resource         ResourceIdentifier  // K8s-specific, typed
    AlertmanagerURL  string              // K8s-specific, typed
    GrafanaURL       string              // K8s-specific, typed
}
```

**Current Design** (Multi-provider, Alternative 1):
```go
type RemediationRequestSpec struct {
    SignalFingerprint string              // Universal
    TargetType       string              // NEW: "kubernetes", "aws", "datadog", etc.
    ProviderData     json.RawMessage     // NEW: All provider-specific data
}
```

### Key Changes

**Fields Added**:
1. ✅ `targetType` - Infrastructure system type (kubernetes, aws, azure, datadog, gcp)
2. ✅ `providerData` - Raw JSON for all provider-specific fields (INCLUDING K8s)
3. ✅ `priority` - Priority assignment (P0/P1/P2) for Rego decisions
4. ✅ `signalType` - Signal type (prometheus, kubernetes-event, aws-cloudwatch, etc.)

**Fields Moved** (K8s fields → providerData):
1. ✅ `namespace` - Now in `providerData` when `targetType="kubernetes"`
2. ✅ `resource` - Now in `providerData` when `targetType="kubernetes"`
3. ✅ `alertmanagerURL` - Now in `providerData` when `targetType="kubernetes"`
4. ✅ `grafanaURL` - Now in `providerData` when `targetType="kubernetes"`

**Fields Removed from Spec**:
1. ❌ `createdAt` - **MOVED TO METADATA** (Kubernetes standard `.metadata.creationTimestamp`)

**Benefits**:
- ✅ **Scalable**: Add AWS, Datadog, Azure without schema changes
- ✅ **Consistent**: All providers treated equally
- ✅ **Clean**: No empty fields for different providers
- ✅ **No V2 Required**: Schema supports all future providers

### Migration Notes

**For Controllers**:
```go
// Before (typed K8s fields)
namespace := remediation.Spec.Namespace
kind := remediation.Spec.Resource.Kind

// After (parse providerData)
if remediation.Spec.TargetType == "kubernetes" {
    var k8sData KubernetesProviderData
    json.Unmarshal(remediation.Spec.ProviderData, &k8sData)
    namespace := k8sData.Namespace
    kind := k8sData.Resource.Kind
}
```

**For Queryability**:
```go
// Use labels for queries (not spec fields)
Labels: map[string]string{
    "namespace":  extractNamespaceFromProviderData(signal),
    "targetType": signal.TargetType,
    "priority":   signal.Priority,
}

// Query:
kubectl get remediationrequests -l namespace=production,priority=P0
```

---

## 📚 Related CRD Schemas

### Other Service CRDs

For schemas of CRDs created by Central Controller:
- **RemediationProcessing**: See `01-signalprocessing/crd-schema.md`
- **AIAnalysis**: See `02-aianalysis/crd-schema.md`
- **WorkflowExecution**: See `03-workflowexecution/crd-schema.md`
- **KubernetesExecution** (DEPRECATED - ADR-025): See `04-kubernetesexecutor/crd-schema.md`

---

## 🔗 References

**Design Documents**:
- Gateway Service Specification: `docs/services/stateless/gateway-service/`
- Central Controller Specification: `docs/services/crd-controllers/05-centralcontroller/`
- CRD Integration: `docs/services/stateless/gateway-service/crd-integration.md`

**Architecture Decisions**:
- Gateway is CRD source of truth: This document
- Priority as Rego Modifier: `gateway-service/PRIORITY_AS_REGO_MODIFIER_TRIAGE.md`
- Signal Type Architecture: `gateway-service/DESIGN_B_IMPLEMENTATION_SUMMARY.md`

---

## ✅ Validation

**This schema is complete and ready for implementation when**:
- [ ] All service documentation references this authoritative schema
- [ ] Gateway CRD creation code matches this spec exactly
- [ ] Central Controller expects these fields in spec
- [ ] Downstream services reference correct fields
- [ ] CRD generation (`controller-gen`) produces matching YAML

---

**Document Status**: ✅ Authoritative Reference
**Schema Version**: v1.0
**K8s API Version**: `remediation.kubernaut.io/v1`
**Confidence**: 100% (Gateway as source of truth)
# CRD Schemas Extension - AIAnalysis, WorkflowExecution, KubernetesExecution (DEPRECATED - ADR-025)

**Purpose**: Temporary file containing schema extensions to be appended to `CRD_SCHEMAS.md`

---

## 🎯 AIAnalysis CRD

### Metadata

**API Group**: `aianalysis.kubernaut.io/v1alpha1`
**Kind**: `AIAnalysis`
**Owner**: AIAnalysis Controller
**Created By**: RemediationOrchestrator (RR Controller)
**Scope**: Namespaced

### Purpose

Performs AI-powered root cause analysis using LLM providers (HolmesGPT, OpenAI, Anthropic). Analyzes signal context and generates remediation recommendations.

---

## 📐 AIAnalysisSpec

```go
// api/aianalysis/v1alpha1/aianalysis_types.go
package v1alpha1

import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AIAnalysisSpec defines the desired state of AIAnalysis
type AIAnalysisSpec struct {
    // Parent reference for audit/lineage
    RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

    // Signal context for analysis
    SignalType string `json:"signalType"`
    SignalContext map[string]string `json:"signalContext"`

    // LLM Configuration
    // +kubebuilder:validation:Enum=openai;anthropic;local;holmesgpt
    LLMProvider string `json:"llmProvider"`

    // +kubebuilder:validation:MaxLength=253
    LLMModel string `json:"llmModel"`

    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=100000
    MaxTokens int `json:"maxTokens"`

    // +kubebuilder:validation:Minimum=0.0
    // +kubebuilder:validation:Maximum=1.0
    Temperature float64 `json:"temperature"`

    IncludeHistory bool `json:"includeHistory,omitempty"`
}
```

---

## 📊 AIAnalysisStatus

```go
// AIAnalysisStatus defines the observed state of AIAnalysis
type AIAnalysisStatus struct {
    // +kubebuilder:validation:Enum=Pending;Investigating;Analyzing;Recommending;Completed;Failed
    Phase string `json:"phase"`

    Message string `json:"message,omitempty"`
    Reason string `json:"reason,omitempty"`

    StartedAt *metav1.Time `json:"startedAt,omitempty"`
    CompletedAt *metav1.Time `json:"completedAt,omitempty"`

    // Analysis Results
    RootCause string `json:"rootCause,omitempty"`

    // +kubebuilder:validation:Minimum=0.0
    // +kubebuilder:validation:Maximum=1.0
    Confidence float64 `json:"confidence,omitempty"`

    RecommendedAction string `json:"recommendedAction,omitempty"`
    RequiresApproval bool `json:"requiresApproval,omitempty"`

    // LLM Metrics
    // +kubebuilder:validation:MaxLength=253
    InvestigationID string `json:"investigationId,omitempty"`

    // NOTE: TokensUsed REMOVED (Dec 2025) - HAPI owns LLM cost observability
    // Use InvestigationID to correlate with HAPI's holmesgpt_llm_token_usage_total metric

    // +kubebuilder:validation:Minimum=0
    InvestigationTime int64 `json:"investigationTime,omitempty"` // milliseconds

    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

**Phase Transitions**:
1. `Pending` → Initial state
2. `Investigating` → Gathering context from RemediationProcessing
3. `Analyzing` → LLM analysis in progress
4. `Recommending` → Generating actionable recommendations
5. `Completed` → Analysis finished, results available
6. `Failed` → Analysis failed (with reason)

---

## 🎯 WorkflowExecution CRD

### Metadata

**API Group**: `workflowexecution.kubernaut.io/v1alpha1`
**Kind**: `WorkflowExecution`
**Owner**: WorkflowExecution Controller
**Created By**: RemediationOrchestrator (RR Controller)
**Scope**: Namespaced

### Purpose

Orchestrates multi-step remediation workflows with safety checks, rollback strategies, and adaptive optimization.

---

## 📐 WorkflowExecutionSpec

```go
// api/workflowexecution/v1alpha1/workflowexecution_types.go
package v1alpha1

import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkflowExecutionSpec defines the desired state of WorkflowExecution
type WorkflowExecutionSpec struct {
    // Parent reference for audit/lineage
    RemediationRequestRef corev1.ObjectReference `json:"remediationRequestRef"`

    // Workflow definition from AIAnalysis
    WorkflowDefinition WorkflowDefinition `json:"workflowDefinition"`

    // Execution strategy
    ExecutionStrategy ExecutionStrategy `json:"executionStrategy"`

    // Adaptive orchestration configuration
    AdaptiveOrchestration *AdaptiveOrchestration `json:"adaptiveOrchestration,omitempty"`
}

type WorkflowDefinition struct {
    Steps []WorkflowStep `json:"steps"`

    // +kubebuilder:validation:Enum=automatic;manual;none
    RollbackStrategy string `json:"rollbackStrategy"`

    SafetyChecks []SafetyCheck `json:"safetyChecks"`
}

type WorkflowStep struct {
    // +kubebuilder:validation:Minimum=1
    StepNumber int `json:"stepNumber"`

    Action string `json:"action"`
    Parameters StepParameters `json:"parameters"`

    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=10
    MaxRetries int `json:"maxRetries,omitempty"`

    // +kubebuilder:validation:Pattern="^[0-9]+(s|m|h)$"
    Timeout string `json:"timeout,omitempty"`

    RollbackSpec *RollbackSpec `json:"rollbackSpec,omitempty"`
}

type ExecutionStrategy struct {
    // +kubebuilder:validation:Enum=sequential;parallel;sequential-with-parallel
    Strategy string `json:"strategy"`

    EstimatedDuration string `json:"estimatedDuration"`
    RollbackStrategy string `json:"rollbackStrategy"`
    SafetyChecks []SafetyCheck `json:"safetyChecks"`
}
```

---

## 📊 WorkflowExecutionStatus

```go
// WorkflowExecutionStatus defines the observed state of WorkflowExecution
type WorkflowExecutionStatus struct {
    // +kubebuilder:validation:Enum=planning;validating;executing;monitoring;completed;failed;paused
    Phase string `json:"phase"`

    // +kubebuilder:validation:Minimum=0
    CurrentStep int `json:"currentStep"`

    // +kubebuilder:validation:Minimum=0
    TotalSteps int `json:"totalSteps"`

    // Execution tracking
    ExecutionPlan *ExecutionPlan `json:"executionPlan,omitempty"`
    ValidationResults *ValidationResults `json:"validationResults,omitempty"`
    StepStatuses []StepStatus `json:"stepStatuses,omitempty"`

    // Metrics
    ExecutionMetrics ExecutionMetrics `json:"executionMetrics,omitempty"`
    AdaptiveAdjustments []AdaptiveAdjustment `json:"adaptiveAdjustments,omitempty"`

    // Final result
    WorkflowResult *WorkflowResult `json:"workflowResult,omitempty"`

    // Phase timestamps
    PlanningStartedAt *metav1.Time `json:"planningStartedAt,omitempty"`
    ExecutionStartedAt *metav1.Time `json:"executionStartedAt,omitempty"`
    CompletedAt *metav1.Time `json:"completedAt,omitempty"`

    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type StepStatus struct {
    // +kubebuilder:validation:Minimum=1
    StepNumber int `json:"stepNumber"`

    Action string `json:"action"`

    // +kubebuilder:validation:Enum=pending;executing;completed;failed;rolled_back;skipped
    Status string `json:"status"`

    StartTime *metav1.Time `json:"startTime,omitempty"`
    EndTime *metav1.Time `json:"endTime,omitempty"`
    Result *StepExecutionResult `json:"result,omitempty"`
    ErrorMessage string `json:"errorMessage,omitempty"`

    // +kubebuilder:validation:Minimum=0
    RetriesAttempted int `json:"retriesAttempted,omitempty"`

    RollbackStatus *RollbackStatus `json:"rollbackStatus,omitempty"`
}

type ExecutionMetrics struct {
    // +kubebuilder:validation:Minimum=0.0
    // +kubebuilder:validation:Maximum=1.0
    StepSuccessRate float64 `json:"stepSuccessRate"`

    // +kubebuilder:validation:Minimum=0.0
    // +kubebuilder:validation:Maximum=1.0
    OverallConfidence float64 `json:"overallConfidence"`

    // +kubebuilder:validation:Minimum=0.0
    // +kubebuilder:validation:Maximum=1.0
    SimilarWorkflowSuccessRate float64 `json:"similarWorkflowSuccessRate"`

    TotalExecutionTime int64 `json:"totalExecutionTime"` // milliseconds
    ResourceModifications int `json:"resourceModifications"`
}

type WorkflowResult struct {
    // +kubebuilder:validation:Enum=success;partial_success;failed;unknown
    Outcome string `json:"outcome"`

    // +kubebuilder:validation:Minimum=0.0
    // +kubebuilder:validation:Maximum=1.0
    EffectivenessScore float64 `json:"effectivenessScore"`

    // +kubebuilder:validation:Enum=healthy;degraded;unhealthy
    ResourceHealth string `json:"resourceHealth"`

    RollbacksExecuted int `json:"rollbacksExecuted"`
    Message string `json:"message,omitempty"`
}
```

**Phase Transitions**:
1. `planning` → Creating execution plan
2. `validating` → Running safety checks and Rego policies
3. `executing` → Executing workflow steps (creates KubernetesExecution CRDs) (DEPRECATED - ADR-025: replaced by Tekton TaskRun)
4. `monitoring` → Monitoring step results and health
5. `completed` → Workflow finished successfully
6. `failed` → Workflow failed (with rollback if configured)
7. `paused` → Manual approval required

---

## 🎯 KubernetesExecution CRD (DEPRECATED - ADR-025)

**⚠️ DEPRECATED**: KubernetesExecution CRD and KubernetesExecutor service eliminated by ADR-025. Replaced by Tekton TaskRun. API types and CRD manifests have been deleted.

### Metadata

**API Group**: `kubernetesexecution.kubernaut.io/v1alpha1`
**Kind**: `KubernetesExecution`
**Owner**: KubernetesExecutor Controller
**Created By**: WorkflowExecution Controller
**Scope**: Namespaced

### Purpose

Executes individual Kubernetes actions (scale, restart, patch) with Job-based isolation, safety validation, and rollback support.

---

## 📐 KubernetesExecutionSpec

```go
// api/kubernetesexecution/v1alpha1/kubernetesexecution_types.go
package v1alpha1

import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubernetesExecutionSpec defines the desired state of KubernetesExecution
type KubernetesExecutionSpec struct {
    // Parent reference for audit/lineage
    WorkflowExecutionRef corev1.ObjectReference `json:"workflowExecutionRef"`

    // +kubebuilder:validation:Minimum=1
    StepNumber int `json:"stepNumber"`

    // +kubebuilder:validation:Enum=scale_deployment;rollout_restart;delete_pod;patch_deployment;cordon_node;drain_node;uncordon_node;update_configmap;update_secret;apply_manifest
    Action string `json:"action"`

    Parameters *ActionParameters `json:"parameters"`

    TargetCluster string `json:"targetCluster,omitempty"` // V2: Multi-cluster support

    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=5
    MaxRetries int `json:"maxRetries,omitempty"`

    Timeout metav1.Duration `json:"timeout,omitempty"`
    ApprovalReceived bool `json:"approvalReceived,omitempty"`
}

type ActionParameters struct {
    ScaleDeployment *ScaleDeploymentParams `json:"scaleDeployment,omitempty"`
    RolloutRestart *RolloutRestartParams `json:"rolloutRestart,omitempty"`
    DeletePod *DeletePodParams `json:"deletePod,omitempty"`
    PatchDeployment *PatchDeploymentParams `json:"patchDeployment,omitempty"`
    CordonNode *CordonNodeParams `json:"cordonNode,omitempty"`
    DrainNode *DrainNodeParams `json:"drainNode,omitempty"`
    UncordonNode *UncordonNodeParams `json:"uncordonNode,omitempty"`
    UpdateConfigMap *UpdateConfigMapParams `json:"updateConfigMap,omitempty"`
    UpdateSecret *UpdateSecretParams `json:"updateSecret,omitempty"`
    ApplyManifest *ApplyManifestParams `json:"applyManifest,omitempty"`
}

type ScaleDeploymentParams struct {
    // +kubebuilder:validation:MaxLength=253
    Deployment string `json:"deployment"`

    // +kubebuilder:validation:MaxLength=63
    Namespace string `json:"namespace"`

    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=1000
    Replicas int32 `json:"replicas"`
}

type DeletePodParams struct {
    // +kubebuilder:validation:MaxLength=253
    Pod string `json:"pod"`

    // +kubebuilder:validation:MaxLength=63
    Namespace string `json:"namespace"`

    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=3600
    GracePeriodSeconds *int64 `json:"gracePeriodSeconds,omitempty"`
}

type PatchDeploymentParams struct {
    // +kubebuilder:validation:MaxLength=253
    Deployment string `json:"deployment"`

    // +kubebuilder:validation:MaxLength=63
    Namespace string `json:"namespace"`

    // +kubebuilder:validation:Enum=strategic;merge;json
    PatchType string `json:"patchType"`

    Patch string `json:"patch"` // JSON/YAML patch content
}

type DrainNodeParams struct {
    // +kubebuilder:validation:MaxLength=253
    Node string `json:"node"`

    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=3600
    GracePeriodSeconds int64 `json:"gracePeriodSeconds,omitempty"`

    Force bool `json:"force,omitempty"`
    DeleteLocalData bool `json:"deleteLocalData,omitempty"`
    IgnoreDaemonSets bool `json:"ignoreDaemonSets,omitempty"`
}
```

---

## 📊 KubernetesExecutionStatus

```go
// KubernetesExecutionStatus defines the observed state of KubernetesExecution
type KubernetesExecutionStatus struct {
    // +kubebuilder:validation:Enum=validating;validated;waiting_approval;executing;rollback_ready;completed;failed
    Phase string `json:"phase"`

    // Validation results
    ValidationResults *ValidationResults `json:"validationResults,omitempty"`

    // Execution results
    ExecutionResults *ExecutionResults `json:"executionResults,omitempty"`

    // Rollback information
    RollbackInformation *RollbackInfo `json:"rollbackInformation,omitempty"`

    // Job reference
    JobName string `json:"jobName,omitempty"`

    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type ExecutionResults struct {
    Success bool `json:"success"`
    JobName string `json:"jobName"`
    StartTime *metav1.Time `json:"startTime,omitempty"`
    EndTime *metav1.Time `json:"endTime,omitempty"`
    Duration string `json:"duration,omitempty"`
    ResourcesAffected []AffectedResource `json:"resourcesAffected,omitempty"`
    PodLogs string `json:"podLogs,omitempty"`

    // +kubebuilder:validation:Minimum=0
    RetriesAttempted int `json:"retriesAttempted"`

    ErrorMessage string `json:"errorMessage,omitempty"`
}
```

**Phase Transitions**:
1. `validating` → Running Rego policy checks, dry-run validation
2. `validated` → Validation passed, ready for execution
3. `waiting_approval` → Manual approval required (high-risk actions)
4. `executing` → Job executing Kubernetes action
5. `rollback_ready` → Rollback parameters captured
6. `completed` → Action completed successfully
7. `failed` → Action failed (with rollback if configured)

---

## 🔔 NotificationRequest CRD

### Metadata

**API Group**: `notification.kubernaut.io/v1`
**Kind**: `NotificationRequest`
**Owner**: Notification Controller Service
**Created By**: RemediationOrchestrator Service
**Scope**: Namespaced
**Documentation**: [06-notification/](../services/crd-controllers/06-notification/)

### Purpose

CRD-based notification delivery with zero data loss guarantee. Replaces the previous stateless HTTP API design. Provides automatic retry, complete audit trail, and at-least-once delivery semantics through etcd persistence.

**Architecture Change (2025-10-12)**: Migrated from stateless HTTP API to CRD Controller for:
- **BR-NOT-050**: Zero data loss (etcd persistence)
- **BR-NOT-051**: Complete audit trail
- **BR-NOT-052**: Automatic retry
- **BR-NOT-053**: At-least-once delivery
- **BR-NOT-054**: Real-time observability

### Source of Truth

**RemediationOrchestrator Service** creates NotificationRequest CRDs after remediation actions complete. The schema below reflects the declarative notification design.

**Why CRD-Based**:
- Durable state survives pod restarts (etcd)
- Controller reconciliation provides automatic retry
- CRD status tracks all delivery attempts (audit trail)
- At-least-once delivery guarantee
- Zero data loss on system failures

## 📐 NotificationRequest Spec

```go
// api/notification/v1alpha1/notificationrequest_types.go
package v1alpha1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NotificationRequestSpec defines the desired state of NotificationRequest
type NotificationRequestSpec struct {
    // ========================================
    // NOTIFICATION METADATA
    // ========================================

    // Subject line for the notification
    // +kubebuilder:validation:MaxLength=255
    // +kubebuilder:validation:MinLength=1
    Subject string `json:"subject"`

    // Human-readable message body
    // +kubebuilder:validation:MaxLength=4096
    // +kubebuilder:validation:MinLength=1
    Message string `json:"message"`

    // Priority: "critical", "high", "normal", "low"
    // Critical notifications are delivered first
    // +kubebuilder:validation:Enum=critical;high;normal;low
    Priority string `json:"priority"`

    // ========================================
    // DELIVERY CHANNELS
    // ========================================

    // List of delivery channels to use
    // At least one channel must be specified
    // +kubebuilder:validation:MinItems=1
    Channels []DeliveryChannel `json:"channels"`

    // ========================================
    // CONTEXT & LINKING
    // ========================================

    // Reference to originating RemediationRequest
    RemediationRequestName string `json:"remediationRequestName,omitempty"`

    // Reference to related WorkflowExecution
    WorkflowExecutionName string `json:"workflowExecutionName,omitempty"`

    // Namespace of remediation context
    RemediationNamespace string `json:"remediationNamespace,omitempty"`

    // Direct action links for external services
    // +kubebuilder:validation:MaxItems=10
    ActionLinks []ActionLink `json:"actionLinks,omitempty"`

    // ========================================
    // RETRY CONFIGURATION
    // ========================================

    // Maximum retry attempts per channel (default: 3)
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=10
    MaxRetries int `json:"maxRetries,omitempty"`

    // Base backoff duration for retry (default: "30s")
    // Exponential backoff: 30s, 1m, 2m, 4m, 8m
    RetryBackoff string `json:"retryBackoff,omitempty"`

    // ========================================
    // SENSITIVE DATA HANDLING
    // ========================================

    // True if message contains potentially sensitive data
    // Controller will sanitize before delivery
    ContainsSensitiveData bool `json:"containsSensitiveData,omitempty"`

    // Sanitization rules to apply
    SanitizationRules []string `json:"sanitizationRules,omitempty"`
}

// DeliveryChannel defines a notification delivery target
type DeliveryChannel struct {
    // Channel type: "email", "slack", "teams", "sms", "webhook"
    // +kubebuilder:validation:Enum=email;slack;teams;sms;webhook
    Type string `json:"type"`

    // Destination (email address, Slack channel, Teams webhook URL, etc.)
    // +kubebuilder:validation:MaxLength=512
    // +kubebuilder:validation:MinLength=1
    Destination string `json:"destination"`

    // Optional custom configuration per channel (JSON)
    Config map[string]string `json:"config,omitempty"`
}

// ActionLink defines an external service action link
type ActionLink struct {
    // Link label (e.g., "View Logs in Grafana")
    // +kubebuilder:validation:MaxLength=100
    // +kubebuilder:validation:MinLength=1
    Label string `json:"label"`

    // Target URL
    // +kubebuilder:validation:MaxLength=2048
    // +kubebuilder:validation:MinLength=1
    URL string `json:"url"`

    // Link type: "grafana", "prometheus", "github", "k8s-dashboard", "custom"
    // +kubebuilder:validation:Enum=grafana;prometheus;github;k8s-dashboard;custom
    Type string `json:"type"`
}
```

## 📊 NotificationRequest Status

```go
// NotificationRequestStatus defines the observed state of NotificationRequest
type NotificationRequestStatus struct {
    // ========================================
    // DELIVERY STATE
    // ========================================

    // Overall phase: "pending", "sending", "sent", "failed"
    // +kubebuilder:validation:Enum=pending;sending;sent;failed
    Phase string `json:"phase"`

    // Per-channel delivery status
    // +kubebuilder:validation:MinItems=1
    ChannelStatus []ChannelDeliveryStatus `json:"channelStatus,omitempty"`

    // ========================================
    // AUDIT TRAIL
    // ========================================

    // All delivery attempts (complete audit trail)
    DeliveryAttempts []DeliveryAttempt `json:"deliveryAttempts,omitempty"`

    // Total retry count across all channels
    // +kubebuilder:validation:Minimum=0
    TotalRetries int `json:"totalRetries"`

    // ========================================
    // TIMESTAMPS
    // ========================================

    // When notification was created
    CreatedAt *metav1.Time `json:"createdAt,omitempty"`

    // When first delivery attempt started
    FirstAttemptAt *metav1.Time `json:"firstAttemptAt,omitempty"`

    // When final delivery completed or failed
    CompletedAt *metav1.Time `json:"completedAt,omitempty"`

    // ========================================
    // ERROR TRACKING
    // ========================================

    // Last error message (if any)
    LastError string `json:"lastError,omitempty"`

    // Per-channel error details
    ChannelErrors map[string]string `json:"channelErrors,omitempty"`
}

// ChannelDeliveryStatus tracks delivery for a single channel
type ChannelDeliveryStatus struct {
    // Channel type
    Type string `json:"type"`

    // Destination
    Destination string `json:"destination"`

    // Status: "pending", "sending", "sent", "failed"
    // +kubebuilder:validation:Enum=pending;sending;sent;failed
    Status string `json:"status"`

    // Retry count for this specific channel
    // +kubebuilder:validation:Minimum=0
    Retries int `json:"retries"`

    // Last delivery attempt timestamp
    LastAttempt *metav1.Time `json:"lastAttempt,omitempty"`

    // Error message (if failed)
    ErrorMessage string `json:"errorMessage,omitempty"`
}

// DeliveryAttempt tracks a single delivery attempt (audit record)
type DeliveryAttempt struct {
    // Attempt number
    // +kubebuilder:validation:Minimum=1
    AttemptNumber int `json:"attemptNumber"`

    // Channel type
    Type string `json:"type"`

    // Destination
    Destination string `json:"destination"`

    // Success or failure
    Success bool `json:"success"`

    // Timestamp
    Timestamp *metav1.Time `json:"timestamp"`

    // Error message (if failed)
    ErrorMessage string `json:"errorMessage,omitempty"`

    // Response code/ID from external service
    ResponseCode string `json:"responseCode,omitempty"`
}
```

**Phase Transitions**:
1. `pending` → NotificationRequest created, waiting for controller pickup
2. `sending` → Controller actively attempting delivery
3. `sent` → All channels delivered successfully
4. `failed` → All retry attempts exhausted, at least one channel failed

**Retry Behavior**:
- Exponential backoff per channel: 30s, 1m, 2m, 4m, 8m (default)
- Controller requeues NotificationRequest after backoff duration
- Per-channel graceful degradation (one channel failure doesn't block others)

**Audit Trail**:
- All delivery attempts stored in `status.deliveryAttempts[]`
- Long-term audit data persisted to Data Storage service (>90 days)
- CRD status provides real-time delivery observability

---

## 📝 Validation Markers Summary

### RemediationRequest / RemediationProcessing
- **Enum**: Severity, Environment, Priority, TargetType
- **Pattern**: SignalFingerprint (SHA256), Priority (P0-P2)
- **MaxLength**: SignalFingerprint (64), SignalName (253), SignalSource (63)

### AIAnalysis
- **Enum**: LLMProvider, Phase
- **Numeric**: MaxTokens (1-100000), Temperature (0.0-1.0), Confidence (0.0-1.0)
- **MaxLength**: LLMModel (253), InvestigationID (253)
- **Minimum**: InvestigationTime (≥0)

### WorkflowExecution
- **Enum**: Phase, RollbackStrategy, ExecutionStrategy, StepStatus, Outcome, ResourceHealth
- **Numeric**: StepNumber (≥1), MaxRetries (0-10), Confidence scores (0.0-1.0)
- **Pattern**: Timeout (duration format)

### KubernetesExecution (DEPRECATED - ADR-025)
- **Enum**: Action, PatchType, Phase
- **Numeric**: StepNumber (≥1), Replicas (0-1000), GracePeriodSeconds (0-3600), MaxRetries (0-5)
- **MaxLength**: Deployment/Pod/Node names (253), Namespace (63)

### NotificationRequest
- **Enum**: Priority, ChannelType, Phase, Status, LinkType
- **Numeric**: MaxRetries (0-10), AttemptNumber (≥1), TotalRetries (≥0)
- **MaxLength**: Subject (255), Message (4096), Destination (512), Label (100), URL (2048)
- **MinLength**: Subject (1), Message (1), Destination (1), Label (1), URL (1)
- **MinItems**: Channels (≥1), ChannelStatus (≥1)
- **MaxItems**: ActionLinks (≤10)

---

**Generated**: January 10, 2025
**Last Updated**: October 12, 2025 (Added NotificationRequest CRD)
**Validates Against**: Kubebuilder v3.x, Kubernetes 1.28+
**Confidence**: 95% - All validations tested and verified in generated CRD manifests



