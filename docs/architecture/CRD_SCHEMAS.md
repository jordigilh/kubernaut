# Authoritative CRD Schemas

**Version**: v1.0
**Last Updated**: October 5, 2025
**Status**: ✅ Authoritative Reference

---

## 📋 Overview

This document defines the **authoritative CRD schemas** for the Kubernaut project. All services must reference and implement these schemas exactly as specified.

**Schema Authority**:
- Gateway Service creates CRDs and is the **source of truth** for field definitions
- Central Controller orchestrates CRDs but **follows Gateway's schema**
- All other services consume CRDs according to these specifications

---

## 🎯 RemediationRequest CRD

### Metadata

**API Group**: `remediation.kubernaut.io/v1`
**Kind**: `RemediationRequest`
**Owner**: Central Controller Service
**Created By**: Gateway Service
**Scope**: Namespaced

### Purpose

Entry point for the remediation workflow. Gateway Service creates one RemediationRequest CRD per unique signal (Prometheus alert, Kubernetes event). Central Controller orchestrates downstream service CRDs based on this request.

### Source of Truth

**Gateway Service** creates RemediationRequest CRDs and populates all fields based on ingested signals. The schema below reflects Gateway's comprehensive data collection.

**Why Gateway is Authoritative**:
- Gateway performs deduplication and storm detection
- Gateway assigns priority based on Rego policies
- Gateway enriches signal with environment classification
- Gateway captures all temporal and source metadata
- Central Controller was designed before Gateway's full capabilities were known

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
    AlertFingerprint string `json:"alertFingerprint"`

    // Human-readable signal name (e.g., "HighMemoryUsage", "CrashLoopBackOff")
    AlertName string `json:"alertName"`

    // Signal Classification
    // Severity level: "critical", "warning", "info"
    Severity string `json:"severity"`

    // Environment: "prod", "staging", "dev"
    Environment string `json:"environment"`

    // Priority assigned by Gateway (P0=critical, P1=high, P2=normal)
    // Used by downstream Rego policies for remediation decisions
    Priority string `json:"priority"`

    // Signal type: "prometheus", "kubernetes-event", "aws-cloudwatch", "datadog-monitor", etc.
    // Used for signal-aware remediation strategies
    SignalType string `json:"signalType"`

    // Adapter that ingested the signal (e.g., "prometheus-adapter", "k8s-event-adapter")
    SignalSource string `json:"signalSource,omitempty"`

    // Target system type: "kubernetes", "aws", "azure", "gcp", "datadog"
    // Indicates which infrastructure system the signal targets
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
    Namespace string `json:"namespace"`
}
```

**Fields**:
- `namespace` (string, **required**): Kubernetes namespace where signal originated
- `resource` (object, **required**): Target Kubernetes resource
  - `kind` (string, **required**): Resource kind (Pod, Deployment, StatefulSet, etc.)
  - `name` (string, **required**): Resource name
  - `namespace` (string, **required**): Resource namespace (may differ from signal namespace)
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
| `alertFingerprint` | string | Unique fingerprint for deduplication |
| `alertName` | string | Human-readable signal name |
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
| `alertFingerprint` | ✅ Creates | ✅ Uses for tracking | ✅ Uses for correlation | ✅ Uses for audit |
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
    AlertFingerprint string
    Namespace        string              // K8s-specific, typed
    Resource         ResourceIdentifier  // K8s-specific, typed
    AlertmanagerURL  string              // K8s-specific, typed
    GrafanaURL       string              // K8s-specific, typed
}
```

**Current Design** (Multi-provider, Alternative 1):
```go
type RemediationRequestSpec struct {
    AlertFingerprint string              // Universal
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
- **RemediationProcessing**: See `01-remediationprocessor/crd-schema.md`
- **AIAnalysis**: See `02-aianalysis/crd-schema.md`
- **WorkflowExecution**: See `03-workflowexecution/crd-schema.md`
- **KubernetesExecution**: See `04-kubernetesexecutor/crd-schema.md`

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
