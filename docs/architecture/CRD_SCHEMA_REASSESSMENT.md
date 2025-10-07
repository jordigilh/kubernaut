# CRD Schema Reassessment - No Backwards Compatibility

**Date**: October 5, 2025
**Context**: User feedback - "No need for backwards compatibility"
**Question**: Should K8s-specific typed fields stay if they're empty for non-K8s signals?

---

## 🎯 The Core Issue

**Current Hybrid Proposal** has this asymmetry:

```go
type RemediationRequestSpec struct {
    // Universal fields
    AlertFingerprint string
    Priority         string
    SignalType       string

    // K8s-specific TYPED fields
    Namespace string              `json:"namespace,omitempty"`
    Resource  *ResourceIdentifier `json:"resource,omitempty"`

    // Everything else: RAW JSON
    ProviderData json.RawMessage `json:"providerData,omitempty"`
}
```

**Problem**: Why special treatment for K8s?
- AWS data goes in `providerData` (raw)
- K8s data gets typed fields (special)
- **Inconsistent**: K8s is just another provider

---

## 📊 Three Options Reconsidered

### **Option 1: Pure Universal + Provider Data** ⭐ **Most Consistent**

```go
type RemediationRequestSpec struct {
    // ========================================
    // UNIVERSAL FIELDS (ALL SIGNALS)
    // ========================================

    // Signal Identification
    AlertFingerprint string      `json:"alertFingerprint"`
    AlertName        string      `json:"alertName"`

    // Signal Classification
    Severity         string      `json:"severity"`
    Environment      string      `json:"environment"`
    Priority         string      `json:"priority"`
    SignalType       string      `json:"signalType"`   // "prometheus", "kubernetes-event", "aws-cloudwatch"
    SignalSource     string      `json:"signalSource"` // Adapter name

    // Target System Type
    TargetType       string      `json:"targetType"`   // "kubernetes", "aws", "azure", "datadog"

    // Temporal Data
    FiringTime       metav1.Time `json:"firingTime"`
    ReceivedTime     metav1.Time `json:"receivedTime"`

    // Deduplication
    Deduplication    DeduplicationInfo `json:"deduplication"`

    // Storm Detection
    IsStorm          bool        `json:"isStorm,omitempty"`
    StormType        string      `json:"stormType,omitempty"`
    StormWindow      string      `json:"stormWindow,omitempty"`
    StormAlertCount  int         `json:"stormAlertCount,omitempty"`

    // ========================================
    // PROVIDER-SPECIFIC DATA (ALL PROVIDERS)
    // ========================================

    // Provider-specific fields (INCLUDING K8s)
    // This is where K8s namespace, resource, etc. go
    ProviderData     json.RawMessage `json:"providerData,omitempty"`

    // ========================================
    // AUDIT/DEBUG
    // ========================================

    // Complete original payload
    OriginalPayload  []byte          `json:"originalPayload,omitempty"`

    // Workflow configuration
    TimeoutConfig    *TimeoutConfig  `json:"timeoutConfig,omitempty"`
}
```

**K8s Example**:
```json
{
  "alertFingerprint": "abc123",
  "priority": "P0",
  "signalType": "prometheus",
  "targetType": "kubernetes",
  "providerData": {
    "namespace": "production",
    "resource": {
      "kind": "Pod",
      "name": "api-server-xyz"
    },
    "alertmanagerURL": "https://alertmanager.example.com",
    "grafanaURL": "https://grafana.example.com/d/abc123"
  }
}
```

**AWS Example**:
```json
{
  "alertFingerprint": "def456",
  "priority": "P1",
  "signalType": "aws-cloudwatch",
  "targetType": "aws",
  "providerData": {
    "region": "us-east-1",
    "accountId": "123456789012",
    "resourceType": "ec2",
    "instanceId": "i-abc123",
    "metricName": "CPUUtilization"
  }
}
```

**Pros**:
- ✅ **Consistent**: ALL provider data in same place
- ✅ **Clean**: No empty fields for different providers
- ✅ **Scalable**: Add new providers without schema changes
- ✅ **Symmetric**: K8s treated like any other provider

**Cons**:
- ⚠️ K8s fields not queryable: Can't do `kubectl get remediationrequests --field-selector spec.namespace=production`
- ⚠️ Controllers must parse JSON even for K8s
- ⚠️ No K8s validation of provider data structure

---

### **Option 2: Hybrid (Typed K8s + Raw Others)**

```go
type RemediationRequestSpec struct {
    // Universal
    AlertFingerprint string
    Priority         string
    TargetType       string

    // K8s-specific (typed)
    Namespace        string              `json:"namespace,omitempty"`
    Resource         *ResourceIdentifier `json:"resource,omitempty"`

    // All other providers (raw)
    ProviderData     json.RawMessage     `json:"providerData,omitempty"`
}
```

**Pros**:
- ✅ K8s fields queryable
- ✅ K8s validation by Kubernetes

**Cons**:
- ❌ **Inconsistent**: K8s special treatment
- ❌ **Empty fields**: `namespace`, `resource` empty for AWS/Datadog
- ❌ **Confusing**: Why is K8s different?

---

### **Option 3: Typed Union (Go-style)**

```go
type RemediationRequestSpec struct {
    // Universal
    AlertFingerprint string
    Priority         string
    TargetType       string

    // Provider-specific (typed unions)
    Kubernetes       *KubernetesTarget `json:"kubernetes,omitempty"`
    AWS              *AWSTarget        `json:"aws,omitempty"`
    Azure            *AzureTarget      `json:"azure,omitempty"`
    Datadog          *DatadogTarget    `json:"datadog,omitempty"`
}

type KubernetesTarget struct {
    Namespace        string             `json:"namespace"`
    Resource         ResourceIdentifier `json:"resource"`
    AlertmanagerURL  string             `json:"alertmanagerURL,omitempty"`
    GrafanaURL       string             `json:"grafanaURL,omitempty"`
}

type AWSTarget struct {
    Region           string `json:"region"`
    AccountID        string `json:"accountId"`
    ResourceType     string `json:"resourceType"`
    InstanceID       string `json:"instanceId"`
    CloudWatchURL    string `json:"cloudWatchURL,omitempty"`
}
```

**Pros**:
- ✅ Fully typed (all providers)
- ✅ K8s validation
- ✅ Queryable (K8s fields)
- ✅ Self-documenting

**Cons**:
- ❌ **Schema changes**: Every new provider requires schema update
- ❌ **Empty fields**: Multiple nil pointers for unused providers
- ❌ **Not scalable**: Can't add providers without code changes

---

## 🎯 Recommendation: Option 1 (Pure Universal + Provider Data)

### Why Option 1 is Best

**With no backwards compatibility constraint**, we should optimize for:

1. **Consistency** - Treat all providers equally
2. **Scalability** - Add providers without schema changes
3. **Simplicity** - One place for provider data

### What This Means

**NO typed K8s fields**:
- Remove `namespace` from spec
- Remove `resource` from spec
- Move both into `providerData`

**Why This is OK**:
- Controllers already parse `providerData` for AWS/Datadog
- Controllers can parse `providerData` for K8s too
- No special case logic

### Trade-off to Accept

**Can't query by K8s namespace**:
```bash
# ❌ This won't work
kubectl get remediationrequests --field-selector spec.namespace=production

# ✅ But labels still work
kubectl get remediationrequests -l namespace=production
```

**Solution**: Gateway sets labels:
```go
Labels: map[string]string{
    "namespace":   signal.Namespace,
    "environment": signal.Environment,
    "priority":    signal.Priority,
}
```

---

## 📐 Final Recommended Schema

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

    // Signal Identification
    AlertFingerprint string `json:"alertFingerprint"` // SHA256 for deduplication
    AlertName        string `json:"alertName"`        // Human-readable name

    // Signal Classification
    Severity         string `json:"severity"`         // "critical", "warning", "info"
    Environment      string `json:"environment"`      // "prod", "staging", "dev"
    Priority         string `json:"priority"`         // "P0", "P1", "P2"
    SignalType       string `json:"signalType"`       // "prometheus", "kubernetes-event", "aws-cloudwatch", "datadog"
    SignalSource     string `json:"signalSource"`     // Adapter name (e.g., "prometheus-adapter")

    // Target System
    TargetType       string `json:"targetType"`       // "kubernetes", "aws", "azure", "gcp", "datadog"

    // Temporal Data
    FiringTime       metav1.Time `json:"firingTime"`   // When signal started firing
    ReceivedTime     metav1.Time `json:"receivedTime"` // When Gateway received signal

    // Deduplication Metadata
    Deduplication    DeduplicationInfo `json:"deduplication"`

    // Storm Detection
    IsStorm          bool   `json:"isStorm,omitempty"`
    StormType        string `json:"stormType,omitempty"`       // "rate", "pattern"
    StormWindow      string `json:"stormWindow,omitempty"`     // "5m"
    StormAlertCount  int    `json:"stormAlertCount,omitempty"`

    // ========================================
    // PROVIDER-SPECIFIC DATA
    // All provider-specific fields go here (INCLUDING Kubernetes)
    // ========================================

    // Provider-specific fields in raw JSON format
    // Gateway adapter populates this based on signal source
    // Controllers parse this based on targetType/signalType
    //
    // For Kubernetes (targetType="kubernetes"):
    //   {"namespace": "...", "resource": {"kind": "...", "name": "..."}, ...}
    //
    // For AWS (targetType="aws"):
    //   {"region": "...", "accountId": "...", "instanceId": "...", ...}
    //
    // See docs/architecture/CRD_SCHEMA_RAW_JSON_ANALYSIS.md for schemas
    ProviderData     json.RawMessage `json:"providerData,omitempty"`

    // ========================================
    // AUDIT/DEBUG
    // ========================================

    // Complete original webhook payload for audit/debugging
    OriginalPayload  []byte `json:"originalPayload,omitempty"`

    // ========================================
    // WORKFLOW CONFIGURATION
    // ========================================

    // Optional timeout overrides for this specific remediation
    TimeoutConfig    *TimeoutConfig `json:"timeoutConfig,omitempty"`
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

---

## 📝 Provider Data Schemas (Documentation)

### Kubernetes Provider Data

**When `targetType="kubernetes"`**:

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
  "prometheusQuery": "rate(http_requests_total{pod=\"api-server-xyz-abc123\"}[5m])"
}
```

**Fields**:
- `namespace` (string, required): K8s namespace where signal originated
- `resource` (object, required): Target K8s resource
  - `kind` (string): Resource kind (Pod, Deployment, etc.)
  - `name` (string): Resource name
  - `namespace` (string): Resource namespace (may differ from signal namespace)
- `alertmanagerURL` (string, optional): Link to Alertmanager alert
- `grafanaURL` (string, optional): Link to Grafana dashboard
- `prometheusQuery` (string, optional): Prometheus query that triggered alert

---

### AWS Provider Data

**When `targetType="aws"`**:

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
  "cloudWatchURL": "https://console.aws.amazon.com/cloudwatch/..."
}
```

**Fields**:
- `region` (string, required): AWS region
- `accountId` (string, required): AWS account ID
- `resourceType` (string, required): AWS resource type (ec2, rds, lambda, etc.)
- `instanceId` (string, required for EC2): EC2 instance ID
- `tags` (object, optional): AWS resource tags
- `metricName` (string, required): CloudWatch metric name
- `threshold` (number, optional): Alert threshold value
- `cloudWatchURL` (string, optional): Link to CloudWatch console

---

### Datadog Provider Data

**When `targetType="datadog"`**:

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
  "datadogURL": "https://app.datadoghq.com/monitors/12345"
}
```

**Fields**:
- `monitorId` (number, required): Datadog monitor ID
- `monitorName` (string, required): Monitor name
- `host` (string, optional): Affected host
- `tags` (array, optional): Datadog tags
- `metricQuery` (string, required): Datadog metric query
- `datadogURL` (string, optional): Link to Datadog monitor

---

## 🔍 How Controllers Use This

### Example: RemediationProcessor

```go
func (r *RemediationProcessorReconciler) enrichContext(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
) error {

    targetType := remediation.Spec.TargetType

    switch targetType {
    case "kubernetes":
        // Parse K8s provider data
        var k8sData KubernetesProviderData
        if err := json.Unmarshal(remediation.Spec.ProviderData, &k8sData); err != nil {
            return fmt.Errorf("failed to parse K8s provider data: %w", err)
        }

        // Enrich with K8s context
        return r.contextService.EnrichKubernetesContext(
            ctx,
            k8sData.Namespace,
            k8sData.Resource.Kind,
            k8sData.Resource.Name,
        )

    case "aws":
        // Parse AWS provider data
        var awsData AWSProviderData
        if err := json.Unmarshal(remediation.Spec.ProviderData, &awsData); err != nil {
            return fmt.Errorf("failed to parse AWS provider data: %w", err)
        }

        // Enrich with AWS context (future)
        return r.contextService.EnrichAWSContext(
            ctx,
            awsData.Region,
            awsData.InstanceID,
        )

    default:
        return fmt.Errorf("unsupported target type: %s", targetType)
    }
}
```

### Example: AIAnalysis Controller

```go
func (r *AIAnalysisReconciler) prepareInvestigationRequest(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
) (*holmesgpt.InvestigationRequest, error) {

    req := &holmesgpt.InvestigationRequest{
        AlertName: remediation.Spec.AlertName,
        Severity:  remediation.Spec.Severity,
    }

    // K8s-specific: Use HolmesGPT kubernetes toolset
    if remediation.Spec.TargetType == "kubernetes" {
        var k8sData KubernetesProviderData
        json.Unmarshal(remediation.Spec.ProviderData, &k8sData)

        req.Toolsets = []string{"kubernetes", "prometheus"}
        req.Context = fmt.Sprintf(
            "Investigate %s/%s in namespace %s",
            k8sData.Resource.Kind,
            k8sData.Resource.Name,
            k8sData.Namespace,
        )
    }

    // AWS-specific: Different investigation approach
    if remediation.Spec.TargetType == "aws" {
        var awsData AWSProviderData
        json.Unmarshal(remediation.Spec.ProviderData, &awsData)

        req.Context = fmt.Sprintf(
            "Investigate AWS %s instance %s in region %s",
            awsData.ResourceType,
            awsData.InstanceID,
            awsData.Region,
        )
        // Note: Might escalate if no AWS investigation toolset available
    }

    return req, nil
}
```

---

## ✅ Benefits of This Approach

### 1. **Consistency**
- ALL provider data in `providerData` field
- K8s treated same as AWS, Datadog, Azure
- No special cases

### 2. **Scalability**
- Add Splunk? Just add adapter, no schema change
- Add Azure? Just add adapter, no schema change
- Add custom provider? Just add adapter, no schema change

### 3. **Simplicity**
- One place for provider data
- Controllers have one code path: parse `providerData`
- No "if K8s then use typed fields, else parse JSON"

### 4. **Clean Schema**
- No empty fields
- No nil pointers
- No confusion about which fields apply when

### 5. **Documentation Driven**
- Provider data schemas documented
- Adapters follow documented schemas
- Clear what each provider includes

---

## ⚠️ Trade-offs Accepted

### 1. **No Field-Level Querying**
Can't do: `kubectl get remediationrequests --field-selector spec.namespace=production`

**Mitigation**: Use labels
```go
Labels: map[string]string{
    "namespace":   k8sData.Namespace,
    "environment": signal.Environment,
    "priority":    signal.Priority,
}
```

Then: `kubectl get remediationrequests -l namespace=production` ✅

### 2. **No K8s Validation of Provider Data**
Kubernetes can't validate `providerData` structure

**Mitigation**: Gateway adapters validate before creating CRD

### 3. **Controllers Must Parse JSON**
Even for K8s, controllers parse JSON

**Mitigation**: Create helper types, code is clean:
```go
var k8sData KubernetesProviderData
json.Unmarshal(remediation.Spec.ProviderData, &k8sData)
```

---

## 🎯 Decision Time

**Should we adopt Option 1 (Pure Universal + Provider Data)?**

**This means**:
1. Remove `namespace`, `resource` from typed spec
2. Move them into `providerData` (K8s is just another provider)
3. Document provider data schemas for K8s, AWS, Datadog, etc.
4. Use labels for queryability

**Pros**:
- ✅ Consistent (no special K8s treatment)
- ✅ Scalable (add providers without schema changes)
- ✅ Clean (no empty fields)

**Cons**:
- ⚠️ Controllers parse JSON even for K8s
- ⚠️ Can't query by spec fields (use labels instead)

**Your decision?**
