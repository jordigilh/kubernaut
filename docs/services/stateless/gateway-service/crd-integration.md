# Gateway Service - CRD Integration

**Version**: v1.1
**Last Updated**: December 7, 2025
**Status**: ✅ Design Complete

---

## 📋 CRD Schema Ownership (ADR-049)

**IMPORTANT**: **Remediation Orchestrator (RO) owns the RemediationRequest CRD schema definition.**

Gateway Service **creates instances** of RemediationRequest but **imports types from RO**.

**Authoritative Schema**: Owned by RO - see [`api/remediation/v1alpha1/`](../../../../api/remediation/v1alpha1/)
**Reference**: [ADR-049](../../../architecture/decisions/ADR-049-remediationrequest-crd-ownership.md)

### Ownership Model (DD-GATEWAY-011)

| Aspect | Owner |
|--------|-------|
| **Schema Definition** | Remediation Orchestrator |
| **Instance Creation** | Gateway Service |
| **`status.deduplication`** | Gateway Service (exclusive write) |
| **`status.stormAggregation`** | Gateway Service (exclusive write) |
| **`status.overallPhase`** | Remediation Orchestrator |

---

## RemediationRequest CRD Creation

### Purpose

Gateway Service creates `RemediationRequest` CRDs that serve as the entry point for the Remediation Orchestrator orchestration workflow. Each CRD contains comprehensive signal data (alert metadata, deduplication info, priority assignment, storm detection) that Gateway collects during signal ingestion.

### CRD Creation Flow

```go
// pkg/gateway/crd.go
package gateway

import (
    "context"
    "fmt"
    "time"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// createRemediationRequestCRD creates RemediationRequest CRD from normalized signal
func (s *Server) createRemediationRequestCRD(
    ctx context.Context,
    signal *NormalizedSignal,
    isStorm bool,
    stormMetadata *processing.StormMetadata,
) (*remediationv1.RemediationRequest, error) {
    // Generate unique name
    name := fmt.Sprintf("remediation-%s", generateShortID())

    cr := &remediationv1.RemediationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      name,
            Namespace: "kubernaut-system",
            Labels: map[string]string{
                "alertName":   signal.AlertName,
                "environment": signal.Environment,
                "priority":    signal.Priority,
                "signalType":  signal.SignalType,
            },
        },
        Spec: remediationv1.RemediationRequestSpec{
            // Core identification
            AlertFingerprint: signal.Fingerprint,
            AlertName:        signal.AlertName,

            // Signal classification
            Severity:     signal.Severity,
            Environment:  signal.Environment,
            Priority:     signal.Priority,
            SignalType:   signal.SignalType,
            SignalSource: signal.SignalSource,
            TargetType:   signal.TargetType, // "kubernetes", "aws", "datadog", etc.

            // Temporal data
            FiringTime:   metav1.NewTime(signal.FiringTime),
            ReceivedTime: metav1.NewTime(signal.ReceivedTime),

            // Deduplication metadata
            Deduplication: remediationv1.DeduplicationInfo{
                IsDuplicate:                   false,
                FirstSeen:                     metav1.NewTime(signal.FiringTime),
                LastSeen:                      metav1.NewTime(signal.ReceivedTime),
                OccurrenceCount:               1,
                PreviousRemediationRequestRef: "",
            },

            // Provider-specific data (ALL providers, including K8s)
            ProviderData: buildProviderData(signal),

            // Raw payload (as []byte for exact preservation)
            OriginalPayload: signal.RawPayload,
        },
    }

    // Add storm detection metadata if detected
    if isStorm && stormMetadata != nil {
        cr.Spec.IsStorm = true
        cr.Spec.StormType = stormMetadata.StormType
        cr.Spec.StormWindow = stormMetadata.Window
        cr.Spec.StormAlertCount = stormMetadata.AlertCount

        // Add affected resources (max 100)
        if len(stormMetadata.AffectedResources) > 0 {
            maxResources := 100
            if len(stormMetadata.AffectedResources) > maxResources {
                cr.Spec.AffectedResources = convertResourceIdentifiers(
                    stormMetadata.AffectedResources[:maxResources],
                )
                cr.Spec.TotalAffectedResources = len(stormMetadata.AffectedResources)
            } else {
                cr.Spec.AffectedResources = convertResourceIdentifiers(
                    stormMetadata.AffectedResources,
                )
            }
        }
    }

    // Create CRD
    if err := s.k8sClient.Create(ctx, cr); err != nil {
        return nil, fmt.Errorf("failed to create RemediationRequest CRD: %w", err)
    }

    alertRemediationCreatedTotal.WithLabelValues(alert.Environment, alert.Priority).Inc()

    return cr, nil
}

// buildProviderData creates provider-specific JSON data based on target type
// V1: Only "kubernetes" is implemented
// V2: "aws", "azure", "datadog", "gcp" will be activated (structures preserved)
func buildProviderData(signal *NormalizedSignal) json.RawMessage {
    switch signal.TargetType {
    case "kubernetes":
        return buildKubernetesProviderData(signal)  // ✅ V1 Active
    case "aws":
        return buildAWSProviderData(signal)          // ⏸️ V2 Planned
    case "datadog":
        return buildDatadogProviderData(signal)      // ⏸️ V2 Planned
    case "azure":
        return buildAzureProviderData(signal)        // ⏸️ V2 Planned (if implemented)
    case "gcp":
        return buildGCPProviderData(signal)          // ⏸️ V2 Planned (if implemented)
    default:
        return nil
    }
}

// buildKubernetesProviderData creates Kubernetes-specific provider data
// Status: ✅ V1 Active - Implemented for current version
func buildKubernetesProviderData(signal *NormalizedSignal) json.RawMessage {
    data := map[string]interface{}{
        "namespace": signal.Namespace,
        "resource": map[string]string{
            "kind":      signal.Resource.Kind,
            "name":      signal.Resource.Name,
            "namespace": signal.Resource.Namespace,
        },
    }

    // Add optional fields if present
    if signal.AlertmanagerURL != "" {
        data["alertmanagerURL"] = signal.AlertmanagerURL
    }
    if signal.GrafanaURL != "" {
        data["grafanaURL"] = signal.GrafanaURL
    }
    if signal.PrometheusQuery != "" {
        data["prometheusQuery"] = signal.PrometheusQuery
    }

    jsonData, _ := json.Marshal(data)
    return jsonData
}

// buildAWSProviderData creates AWS-specific provider data
// Status: ⏸️ V2 Planned - Structure preserved for V2 implementation
// 🚨 DO NOT DELETE: Valid V2 code, not unused
func buildAWSProviderData(signal *NormalizedSignal) json.RawMessage {
    data := map[string]interface{}{
        "region":       signal.AWSRegion,
        "accountId":    signal.AWSAccountID,
        "resourceType": signal.AWSResourceType,
        "instanceId":   signal.AWSInstanceID,
        "metricName":   signal.AWSMetricName,
    }

    // Add optional fields
    if signal.AWSCloudWatchURL != "" {
        data["cloudWatchURL"] = signal.AWSCloudWatchURL
    }
    if signal.AWSTags != nil {
        data["tags"] = signal.AWSTags
    }

    jsonData, _ := json.Marshal(data)
    return jsonData
}

// buildDatadogProviderData creates Datadog-specific provider data
// Status: ⏸️ V2 Planned - Structure preserved for V2 implementation
// 🚨 DO NOT DELETE: Valid V2 code, not unused
func buildDatadogProviderData(signal *NormalizedSignal) json.RawMessage {
    data := map[string]interface{}{
        "monitorId":   signal.DatadogMonitorID,
        "monitorName": signal.DatadogMonitorName,
        "metricQuery": signal.DatadogMetricQuery,
    }

    // Add optional fields
    if signal.DatadogHost != "" {
        data["host"] = signal.DatadogHost
    }
    if signal.DatadogTags != nil {
        data["tags"] = signal.DatadogTags
    }
    if signal.DatadogURL != "" {
        data["datadogURL"] = signal.DatadogURL
    }

    jsonData, _ := json.Marshal(data)
    return jsonData
}
```

### Normal Alert CRD Example

```yaml
apiVersion: remediation.kubernaut.ai/v1
kind: RemediationRequest
metadata:
  name: remediation-abc123
  namespace: kubernaut-system
  labels:
    alertName: HighMemoryUsage
    environment: prod
    priority: P0
    sourceType: prometheus
spec:
  alertFingerprint: "a1b2c3d4e5..."
  alertName: "HighMemoryUsage"
  severity: "critical"
  environment: "prod"
  priority: "P0"
  namespace: "prod-payment-service"
  resource:
    kind: Pod
    name: payment-api-789
    namespace: prod-payment-service
  firingTime: "2025-10-04T10:00:00Z"
  receivedTime: "2025-10-04T10:00:05Z"
  deduplication:
    isDuplicate: false
    firstSeen: "2025-10-04T10:00:00Z"
    lastSeen: "2025-10-04T10:00:00Z"
    occurrenceCount: 1
  sourceType: "prometheus"
  alertmanagerURL: "http://alertmanager:9093"
  rawPayload: |
    {"alerts":[{"status":"firing",...}]}
status:
  phase: "Pending"  # Remediation Orchestrator updates this
```

### Storm Alert CRD Example

```yaml
apiVersion: remediation.kubernaut.ai/v1
kind: RemediationRequest
metadata:
  name: remediation-storm-xyz
  namespace: kubernaut-system
  labels:
    alertName: PodOOMKilled
    environment: prod
    priority: P0
    sourceType: kubernetes-event
    isStorm: "true"
spec:
  isStorm: true
  stormType: "pattern"
  stormWindow: "5m"
  alertCount: 15
  affectedResources:
    - kind: Pod
      name: web-app-789
      namespace: prod-ns-1
    - kind: Pod
      name: api-456
      namespace: prod-ns-2
    # ... (max 100 resources shown)
  totalAffectedResources: 15
  # Standard fields
  alertFingerprint: "f1e2d3c4b5..."
  alertName: "PodOOMKilled"
  severity: "critical"
  environment: "prod"
  priority: "P0"
  # ... rest of normal fields
```

### Integration with Remediation Orchestrator

Gateway creates CRD → Remediation Orchestrator watches and orchestrates:

```
┌──────────────┐
│   Gateway    │
│   Service    │
└──────┬───────┘
       │ Create RemediationRequest CRD
       ▼
┌──────────────────────┐
│  Kubernetes API      │
│  (CRD Storage)       │
└──────┬───────────────┘
       │ Watch Event
       ▼
┌──────────────────────┐
│ RemediationRequest   │
│ Controller (Central) │
│                      │
│ Creates child CRDs:  │
│ 1. RemediationProcessing
│ 2. AIAnalysis        │
│ 3. WorkflowExecution │
│ 4. KubernetesExecution (DEPRECATED - ADR-025)
└──────────────────────┘
```

**Note**: Gateway's responsibility ends after CRD creation. Remediation Orchestrator orchestrates downstream workflow.

**Confidence**: 95%
