# CRD Schema: Raw JSON vs Strongly Typed - Analysis

**Date**: October 5, 2025
**Question**: Why not use raw JSON field to capture provider-specific signal data?

---

## 🤔 The Question

Instead of creating V2 for non-K8s signals, why not add a flexible field like:

```go
type RemediationRequestSpec struct {
    // ... existing fields ...

    // Provider-specific data (flexible)
    SourceSpecificData json.RawMessage `json:"sourceSpecificData,omitempty"`
}
```

**Benefits**:
- ✅ No V2 needed
- ✅ AWS, Datadog, Azure all work in V1
- ✅ Gateway adapters can include any data
- ✅ Simple schema evolution

---

## 📊 Option Comparison

### Option A: Current Approach (Strongly Typed, K8s-Only)

```go
type RemediationRequestSpec struct {
    AlertFingerprint string
    AlertName        string
    Severity         string
    Environment      string
    Priority         string
    SignalType       string  // "prometheus", "kubernetes-event"

    // K8s-specific (strongly typed)
    Namespace string
    Resource  ResourceIdentifier

    // Raw payload (for audit/debug)
    OriginalPayload []byte
}

type ResourceIdentifier struct {
    Kind      string // "Pod", "Deployment"
    Name      string
    Namespace string
}
```

**Pros**:
- ✅ Type-safe: Controllers know exact fields
- ✅ Queryable: Can filter CRDs by resource kind/name
- ✅ Validated: Kubernetes validates schema
- ✅ Self-documenting: Field names show purpose
- ✅ Compile-time safety: Go compiler catches errors

**Cons**:
- ❌ K8s-only: Doesn't work for AWS/Datadog
- ❌ Requires V2: Schema change needed for new providers
- ❌ Rigid: Can't extend without API version change

---

### Option B: Hybrid (Typed Core + Raw Provider Data)

```go
type RemediationRequestSpec struct {
    // Core fields (ALL signals, strongly typed)
    AlertFingerprint string
    AlertName        string
    Severity         string
    Environment      string
    Priority         string
    SignalType       string  // "prometheus", "kubernetes-event", "aws-cloudwatch", "datadog"
    SignalSource     string

    // Generic target (loosely typed)
    TargetType       string  // "kubernetes", "aws", "azure", "datadog"

    // K8s-specific (when TargetType="kubernetes")
    Namespace        string                 `json:"namespace,omitempty"`
    Resource         *ResourceIdentifier    `json:"resource,omitempty"`

    // Provider-specific data (flexible, untyped)
    ProviderData     json.RawMessage        `json:"providerData,omitempty"`

    // Raw payload (complete original)
    OriginalPayload  []byte                 `json:"originalPayload,omitempty"`
}
```

**Example for Prometheus (K8s)**:
```json
{
  "alertFingerprint": "abc123",
  "signalType": "prometheus",
  "targetType": "kubernetes",
  "namespace": "production",
  "resource": {
    "kind": "Pod",
    "name": "api-server-xyz"
  },
  "providerData": null
}
```

**Example for AWS CloudWatch**:
```json
{
  "alertFingerprint": "def456",
  "signalType": "aws-cloudwatch",
  "targetType": "aws",
  "namespace": "",
  "resource": null,
  "providerData": {
    "region": "us-east-1",
    "accountId": "123456789012",
    "resourceType": "ec2",
    "instanceId": "i-abc123def456",
    "metricName": "CPUUtilization",
    "threshold": 80,
    "evaluationPeriods": 2
  }
}
```

**Example for Datadog**:
```json
{
  "alertFingerprint": "ghi789",
  "signalType": "datadog-monitor",
  "targetType": "datadog",
  "namespace": "",
  "resource": null,
  "providerData": {
    "monitorId": 12345,
    "monitorName": "High Memory Usage",
    "host": "prod-web-01",
    "tags": ["env:production", "service:api", "team:platform"],
    "metricQuery": "avg:system.mem.used{host:prod-web-01}",
    "threshold": 90
  }
}
```

**Pros**:
- ✅ Flexible: Works for ANY provider without V2
- ✅ Type-safe core: Common fields strongly typed
- ✅ Extensible: New providers just add to `providerData`
- ✅ No migration: V1 supports everything

**Cons**:
- ⚠️ Mixed safety: Core typed, provider data untyped
- ⚠️ Documentation burden: Must document provider schemas
- ⚠️ Controller complexity: Must parse JSON for provider data
- ⚠️ No validation: K8s can't validate `providerData` structure
- ⚠️ Query limitations: Can't filter by AWS region, Datadog tags, etc.

---

### Option C: Pure Raw JSON (Maximum Flexibility)

```go
type RemediationRequestSpec struct {
    // Minimal core (only common fields)
    AlertFingerprint string
    SignalType       string

    // Everything else is raw
    SignalData       json.RawMessage `json:"signalData"`

    // Full payload
    OriginalPayload  []byte          `json:"originalPayload,omitempty"`
}
```

**Pros**:
- ✅ Ultimate flexibility
- ✅ Zero schema changes ever needed

**Cons**:
- ❌ No type safety anywhere
- ❌ Controllers must parse everything
- ❌ No K8s validation
- ❌ Can't query/filter CRDs
- ❌ Hard to understand what's in CRDs

---

## 🎯 Recommendation: Hybrid Approach (Option B)

### Why Hybrid is Best

**The hybrid approach balances**:
1. **Type safety** for fields ALL controllers need
2. **Flexibility** for provider-specific details

### What Goes Where?

**Strongly Typed (All Controllers Need)**:
- `alertFingerprint` - Deduplication
- `alertName` - Display
- `severity` - Decision-making
- `environment` - Safety policies
- `priority` - Rego routing
- `signalType` - Provider identification
- `targetType` - Target system type

**Optional Strongly Typed (K8s-specific)**:
- `namespace` - When `targetType="kubernetes"`
- `resource` - When `targetType="kubernetes"`

**Raw JSON (Provider-specific)**:
- `providerData` - Everything else

---

## 📐 Updated Schema Proposal

```go
// pkg/apis/remediation/v1/remediationrequest_types.go
package v1

import (
    "encoding/json"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RemediationRequestSpec struct {
    // Core Signal Identification (REQUIRED, ALL PROVIDERS)
    AlertFingerprint string `json:"alertFingerprint"`
    AlertName        string `json:"alertName"`

    // Signal Classification (REQUIRED, ALL PROVIDERS)
    Severity     string `json:"severity"`      // "critical", "warning", "info"
    Environment  string `json:"environment"`   // "prod", "staging", "dev"
    Priority     string `json:"priority"`      // "P0", "P1", "P2"
    SignalType   string `json:"signalType"`    // "prometheus", "kubernetes-event", "aws-cloudwatch", "datadog"
    SignalSource string `json:"signalSource"`  // Adapter name

    // Target System (REQUIRED, ALL PROVIDERS)
    TargetType string `json:"targetType"` // "kubernetes", "aws", "azure", "gcp", "datadog"

    // Kubernetes-Specific Fields (OPTIONAL, when targetType="kubernetes")
    Namespace string              `json:"namespace,omitempty"`
    Resource  *ResourceIdentifier `json:"resource,omitempty"`

    // Temporal Data (REQUIRED, ALL PROVIDERS)
    FiringTime   metav1.Time `json:"firingTime"`
    ReceivedTime metav1.Time `json:"receivedTime"`

    // Deduplication (REQUIRED, ALL PROVIDERS)
    Deduplication DeduplicationInfo `json:"deduplication"`

    // Provider-Specific Data (OPTIONAL, flexible)
    // Gateway adapters populate this with provider-specific fields
    // Controllers parse this based on signalType/targetType
    ProviderData json.RawMessage `json:"providerData,omitempty"`

    // Storm Detection (OPTIONAL, ALL PROVIDERS)
    IsStorm         bool   `json:"isStorm,omitempty"`
    StormType       string `json:"stormType,omitempty"`
    StormWindow     string `json:"stormWindow,omitempty"`
    StormAlertCount int    `json:"stormAlertCount,omitempty"`

    // Original Payload (OPTIONAL, for audit/debug)
    OriginalPayload []byte `json:"originalPayload,omitempty"`

    // Workflow Configuration (OPTIONAL)
    TimeoutConfig *TimeoutConfig `json:"timeoutConfig,omitempty"`
}

type ResourceIdentifier struct {
    Kind      string `json:"kind"`
    Name      string `json:"name"`
    Namespace string `json:"namespace"`
}

// ... rest of types ...
```

---

## 📝 Provider Data Schemas

### Kubernetes (Prometheus/K8s Events)

**`targetType="kubernetes"`**

```json
{
  "targetType": "kubernetes",
  "namespace": "production",
  "resource": {
    "kind": "Pod",
    "name": "api-server-xyz"
  },
  "providerData": {
    "alertmanagerURL": "https://alertmanager.example.com/#/alerts",
    "grafanaURL": "https://grafana.example.com/d/abc123",
    "prometheusQuery": "rate(http_requests_total[5m]) > 1000"
  }
}
```

---

### AWS CloudWatch

**`targetType="aws"`**

```json
{
  "targetType": "aws",
  "providerData": {
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
}
```

---

### Datadog

**`targetType="datadog"`**

```json
{
  "targetType": "datadog",
  "providerData": {
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
}
```

---

### Azure Monitor

**`targetType="azure"`**

```json
{
  "targetType": "azure",
  "providerData": {
    "subscriptionId": "12345678-1234-1234-1234-123456789012",
    "resourceGroup": "production-rg",
    "resourceType": "Microsoft.Compute/virtualMachines",
    "resourceId": "/subscriptions/.../vm-prod-01",
    "resourceName": "vm-prod-01",
    "location": "eastus",
    "tags": {
      "Environment": "Production",
      "CostCenter": "Engineering"
    },
    "metricName": "Percentage CPU",
    "threshold": 85,
    "azurePortalURL": "https://portal.azure.com/#@.../resource/..."
  }
}
```

---

## 🔍 How Controllers Use This

### RemediationProcessor

```go
func (r *RemediationProcessorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var remediation remediationv1.RemediationRequest
    if err := r.Get(ctx, req.NamespacedName, &remediation); err != nil {
        return ctrl.Result{}, err
    }

    // Always available: typed fields
    priority := remediation.Spec.Priority
    targetType := remediation.Spec.TargetType

    // K8s-specific: strongly typed (optional)
    if targetType == "kubernetes" && remediation.Spec.Resource != nil {
        namespace := remediation.Spec.Namespace
        kind := remediation.Spec.Resource.Kind
        name := remediation.Spec.Resource.Name

        // Enrich with K8s context
        ctx, err := r.contextService.EnrichKubernetesContext(ctx, namespace, kind, name)
    }

    // Provider-specific: parse raw JSON
    if remediation.Spec.ProviderData != nil {
        switch remediation.Spec.SignalType {
        case "aws-cloudwatch":
            var awsData AWSCloudWatchData
            if err := json.Unmarshal(remediation.Spec.ProviderData, &awsData); err != nil {
                // Handle error
            }
            // Use awsData.Region, awsData.InstanceID, etc.

        case "datadog-monitor":
            var ddData DatadogMonitorData
            if err := json.Unmarshal(remediation.Spec.ProviderData, &ddData); err != nil {
                // Handle error
            }
            // Use ddData.Host, ddData.Tags, etc.
        }
    }

    return ctrl.Result{}, nil
}
```

---

### AIAnalysis Controller

```go
func (r *AIAnalysisReconciler) prepareHolmesGPTInput(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
) (*holmesgpt.InvestigationRequest, error) {

    input := &holmesgpt.InvestigationRequest{
        AlertName: remediation.Spec.AlertName,
        Severity:  remediation.Spec.Severity,
    }

    // K8s-native: Use HolmesGPT kubernetes toolset
    if remediation.Spec.TargetType == "kubernetes" {
        input.Toolsets = []string{"kubernetes", "prometheus"}
        input.Namespace = remediation.Spec.Namespace
        input.ResourceKind = remediation.Spec.Resource.Kind
        input.ResourceName = remediation.Spec.Resource.Name
    }

    // AWS: Different context strategy
    if remediation.Spec.TargetType == "aws" {
        var awsData AWSCloudWatchData
        json.Unmarshal(remediation.Spec.ProviderData, &awsData)

        // Use AWS-specific investigation context
        input.Context = fmt.Sprintf(
            "AWS EC2 instance %s in region %s is experiencing high CPU",
            awsData.InstanceID,
            awsData.Region,
        )
        // Note: HolmesGPT might not have AWS toolset, might escalate
    }

    return input, nil
}
```

---

## ✅ Benefits of Hybrid Approach

### 1. **No V2 Needed**
- AWS, Datadog, Azure all work in V1
- Just add adapters, no schema changes

### 2. **Type Safety Where It Matters**
- Core routing fields (priority, severity) are typed
- Controllers don't break on bad data

### 3. **Query/Filter Core Fields**
```bash
# Can still query by core fields
kubectl get remediationrequests -l priority=P0
kubectl get remediationrequests --field-selector spec.targetType=kubernetes
```

### 4. **Flexible Provider Data**
- AWS adapter adds AWS-specific fields
- Datadog adapter adds Datadog-specific fields
- No schema changes needed

### 5. **Documentation Driven**
- Provider data schemas documented here
- Adapters follow documented schemas
- Controllers parse based on documentation

---

## ⚠️ Trade-offs to Accept

### 1. **No Validation of Provider Data**
Kubernetes can't validate `providerData` structure. Must validate in adapter.

### 2. **Documentation is Critical**
Provider data schemas must be well-documented. This file becomes authoritative.

### 3. **Controller Complexity**
Controllers must handle different provider data formats. More code paths.

### 4. **Can't Query Provider Fields**
Can't do: `kubectl get remediationrequests --field-selector providerData.region=us-east-1`

---

## 🎯 Recommendation

**Use Hybrid Approach (Option B) - Update V1 Schema Now**

**Why**:
1. ✅ Supports AWS/Datadog/Azure without V2
2. ✅ Maintains type safety for routing decisions
3. ✅ Minimal migration (just add `providerData`, keep existing fields)
4. ✅ Kubernetes philosophy: typed core + unstructured extension

**Action**:
1. Update `docs/architecture/CRD_SCHEMAS.md` with hybrid approach
2. Add `providerData json.RawMessage` field
3. Add `targetType` field
4. Document provider data schemas (AWS, Datadog, Azure)
5. Keep existing K8s fields (backward compatible)

---

## 🤔 Your Decision

**Should we**:
- **Option A**: Keep current K8s-only schema, plan V2 later
- **Option B**: Adopt hybrid approach NOW (add `providerData`)
- **Option C**: Different approach?

**If Option B**, I'll:
1. Update authoritative CRD schema
2. Add provider data schema documentation
3. Update Gateway CRD integration examples
4. Update controller integration docs

**What's your preference?**
