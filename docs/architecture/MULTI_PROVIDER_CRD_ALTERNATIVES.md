# Multi-Provider CRD Architecture - Alternative Approaches

**Date**: October 5, 2025
**Challenge**: Support K8s + Non-K8s (AWS, Datadog, Azure) signals in single CRD schema
**Constraint**: No backwards compatibility required
**Minimum Confidence**: 80%

---

## 🎯 High-Confidence Alternatives (≥80%)

Five approaches that solve the multi-provider challenge with different trade-offs.

---

## 📊 Alternative 1: Pure Raw JSON Provider Data ⭐

**Confidence**: 90%

### Schema

```go
type RemediationRequestSpec struct {
    // ========================================
    // UNIVERSAL FIELDS (strongly typed)
    // ========================================
    AlertFingerprint string      `json:"alertFingerprint"`
    AlertName        string      `json:"alertName"`
    Severity         string      `json:"severity"`
    Environment      string      `json:"environment"`
    Priority         string      `json:"priority"`
    SignalType       string      `json:"signalType"`
    TargetType       string      `json:"targetType"` // "kubernetes", "aws", "datadog"

    FiringTime       metav1.Time `json:"firingTime"`
    ReceivedTime     metav1.Time `json:"receivedTime"`
    Deduplication    DeduplicationInfo `json:"deduplication"`

    // ========================================
    // PROVIDER DATA (raw JSON for flexibility)
    // ========================================
    ProviderData     json.RawMessage `json:"providerData,omitempty"`

    OriginalPayload  []byte          `json:"originalPayload,omitempty"`
    TimeoutConfig    *TimeoutConfig  `json:"timeoutConfig,omitempty"`
}
```

### Examples

**Kubernetes**:
```json
{
  "targetType": "kubernetes",
  "providerData": {
    "namespace": "production",
    "resource": {"kind": "Pod", "name": "api-xyz"},
    "alertmanagerURL": "https://..."
  }
}
```

**AWS**:
```json
{
  "targetType": "aws",
  "providerData": {
    "region": "us-east-1",
    "instanceId": "i-abc123",
    "resourceType": "ec2"
  }
}
```

### Controller Usage

```go
func (r *Reconciler) processRemediation(remediation *remediationv1.RemediationRequest) error {
    switch remediation.Spec.TargetType {
    case "kubernetes":
        var k8s KubernetesProviderData
        json.Unmarshal(remediation.Spec.ProviderData, &k8s)
        return r.processKubernetes(k8s)

    case "aws":
        var aws AWSProviderData
        json.Unmarshal(remediation.Spec.ProviderData, &aws)
        return r.processAWS(aws)
    }
}
```

### Pros & Cons

**Pros**:
- ✅ **Simplicity**: One field for all provider data
- ✅ **Consistency**: All providers treated equally
- ✅ **Scalability**: Add providers without schema changes
- ✅ **No Empty Fields**: Clean for all providers

**Cons**:
- ⚠️ **No K8s Validation**: Can't validate provider data structure
- ⚠️ **No Field Queries**: Can't query by provider-specific fields
- ⚠️ **JSON Parsing**: Controllers must parse for every access
- ⚠️ **Documentation Critical**: Provider schemas must be well-documented

### Mitigation Strategies

**For Queryability**: Use labels
```go
Labels: map[string]string{
    "namespace": extractFromProviderData(signal.ProviderData, "namespace"),
    "region":    extractFromProviderData(signal.ProviderData, "region"),
}
```

**For Validation**: Gateway adapter validates before CRD creation
```go
func (a *PrometheusAdapter) Validate(signal *NormalizedSignal) error {
    var k8s KubernetesProviderData
    if err := json.Unmarshal(signal.ProviderData, &k8s); err != nil {
        return fmt.Errorf("invalid K8s provider data: %w", err)
    }
    if k8s.Namespace == "" {
        return errors.New("namespace is required")
    }
    return nil
}
```

**For Type Safety**: Create typed helper structs
```go
// pkg/apis/remediation/v1/provider_types.go
type KubernetesProviderData struct {
    Namespace       string             `json:"namespace"`
    Resource        ResourceIdentifier `json:"resource"`
    AlertmanagerURL string             `json:"alertmanagerURL,omitempty"`
}

type AWSProviderData struct {
    Region       string `json:"region"`
    AccountID    string `json:"accountId"`
    InstanceID   string `json:"instanceId"`
    ResourceType string `json:"resourceType"`
}
```

### Confidence Justification

**90% confidence** because:
- ✅ Proven pattern in Kubernetes (ConfigMaps use similar approach)
- ✅ Simple to implement
- ✅ Highly flexible
- ✅ Used successfully in other projects (Tekton, ArgoCD)
- ⚠️ Requires strong documentation discipline

---

## 📊 Alternative 2: Typed Union with OneOf Validation

**Confidence**: 85%

### Schema

```go
type RemediationRequestSpec struct {
    // Universal
    AlertFingerprint string
    Priority         string
    TargetType       string

    // Typed provider data (only ONE populated)
    Kubernetes       *KubernetesTarget `json:"kubernetes,omitempty"`
    AWS              *AWSTarget        `json:"aws,omitempty"`
    Azure            *AzureTarget      `json:"azure,omitempty"`
    Datadog          *DatadogTarget    `json:"datadog,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="self.targetType == 'kubernetes' ? has(self.kubernetes) : true"
// +kubebuilder:validation:XValidation:rule="self.targetType == 'aws' ? has(self.aws) : true"
```

### Provider Types

```go
type KubernetesTarget struct {
    Namespace       string             `json:"namespace"`
    Resource        ResourceIdentifier `json:"resource"`
    AlertmanagerURL string             `json:"alertmanagerURL,omitempty"`
    GrafanaURL      string             `json:"grafanaURL,omitempty"`
}

type AWSTarget struct {
    Region          string            `json:"region"`
    AccountID       string            `json:"accountId"`
    ResourceType    string            `json:"resourceType"` // "ec2", "rds", "lambda"
    ResourceID      string            `json:"resourceId"`
    Tags            map[string]string `json:"tags,omitempty"`
    CloudWatchURL   string            `json:"cloudWatchURL,omitempty"`
}

type AzureTarget struct {
    SubscriptionID  string            `json:"subscriptionId"`
    ResourceGroup   string            `json:"resourceGroup"`
    ResourceType    string            `json:"resourceType"`
    ResourceName    string            `json:"resourceName"`
    Location        string            `json:"location"`
    Tags            map[string]string `json:"tags,omitempty"`
    AzurePortalURL  string            `json:"azurePortalURL,omitempty"`
}

type DatadogTarget struct {
    MonitorID       int64             `json:"monitorId"`
    MonitorName     string            `json:"monitorName"`
    Host            string            `json:"host,omitempty"`
    Tags            []string          `json:"tags,omitempty"`
    MetricQuery     string            `json:"metricQuery"`
    DatadogURL      string            `json:"datadogURL,omitempty"`
}
```

### Examples

**Kubernetes**:
```json
{
  "targetType": "kubernetes",
  "kubernetes": {
    "namespace": "production",
    "resource": {"kind": "Pod", "name": "api-xyz"}
  },
  "aws": null,
  "azure": null,
  "datadog": null
}
```

**AWS**:
```json
{
  "targetType": "aws",
  "kubernetes": null,
  "aws": {
    "region": "us-east-1",
    "resourceType": "ec2",
    "resourceId": "i-abc123"
  },
  "azure": null,
  "datadog": null
}
```

### Controller Usage

```go
func (r *Reconciler) processRemediation(remediation *remediationv1.RemediationRequest) error {
    switch remediation.Spec.TargetType {
    case "kubernetes":
        if remediation.Spec.Kubernetes == nil {
            return errors.New("kubernetes target data missing")
        }
        return r.processKubernetes(remediation.Spec.Kubernetes)

    case "aws":
        if remediation.Spec.AWS == nil {
            return errors.New("aws target data missing")
        }
        return r.processAWS(remediation.Spec.AWS)
    }
}
```

### Pros & Cons

**Pros**:
- ✅ **Fully Typed**: Complete type safety for all providers
- ✅ **K8s Validation**: CRD validation validates structure
- ✅ **Self-Documenting**: Types show what's expected
- ✅ **IDE Support**: Auto-complete works perfectly
- ✅ **Queryable**: Can query K8s-specific fields

**Cons**:
- ⚠️ **Schema Updates**: Every new provider requires CRD schema change
- ⚠️ **Multiple Nil Pointers**: 3-4 nil pointers per CRD
- ⚠️ **Validation Complexity**: OneOf validation can be tricky
- ⚠️ **CRD Size**: Schema gets large with many providers

### Mitigation Strategies

**For Schema Updates**: Use CRD versioning
```go
// When adding new provider, bump version
// v1 -> v2 with conversion webhook
```

**For Nil Pointers**: Document clearly which field applies
```go
// Only ONE of kubernetes, aws, azure, datadog is populated
// Determined by targetType field
```

**For Validation**: Use CEL validation (Kubernetes 1.25+)
```go
// +kubebuilder:validation:XValidation:rule="(self.targetType == 'kubernetes' && has(self.kubernetes)) || (self.targetType == 'aws' && has(self.aws))"
```

### Confidence Justification

**85% confidence** because:
- ✅ Strong type safety
- ✅ K8s native validation
- ✅ Clear schema
- ⚠️ Requires schema updates for new providers (acceptable if infrequent)
- ⚠️ More complex than raw JSON

---

## 📊 Alternative 3: Separate CRD Types per Provider

**Confidence**: 80%

### Schema

**Base CRD** (abstract):
```go
// Common fields only
type RemediationRequestBase struct {
    AlertFingerprint string
    Priority         string
    Severity         string
    Environment      string
    TargetType       string // Set automatically by CRD kind
}
```

**K8s-specific CRD**:
```go
// KubernetesRemediationRequest CRD
type KubernetesRemediationRequestSpec struct {
    RemediationRequestBase

    // K8s-specific (strongly typed)
    Namespace       string
    Resource        ResourceIdentifier
    AlertmanagerURL string
    GrafanaURL      string
}
```

**AWS-specific CRD**:
```go
// AWSRemediationRequest CRD
type AWSRemediationRequestSpec struct {
    RemediationRequestBase

    // AWS-specific (strongly typed)
    Region          string
    AccountID       string
    ResourceType    string
    ResourceID      string
    CloudWatchURL   string
}
```

### Examples

**Kubernetes CRD**:
```yaml
apiVersion: remediation.kubernaut.io/v1
kind: KubernetesRemediationRequest
metadata:
  name: remediation-abc123
spec:
  alertFingerprint: abc123
  priority: P0
  namespace: production
  resource:
    kind: Pod
    name: api-xyz
```

**AWS CRD**:
```yaml
apiVersion: remediation.kubernaut.io/v1
kind: AWSRemediationRequest
metadata:
  name: remediation-def456
spec:
  alertFingerprint: def456
  priority: P1
  region: us-east-1
  resourceType: ec2
  resourceId: i-abc123
```

### Controller Architecture

**Separate controllers per provider**:
```go
// KubernetesRemediationReconciler
func (r *KubernetesRemediationReconciler) Reconcile(ctx context.Context, req ctrl.Request) {
    var k8sRemediation remediationv1.KubernetesRemediationRequest
    // ... process K8s remediation
}

// AWSRemediationReconciler
func (r *AWSRemediationReconciler) Reconcile(ctx context.Context, req ctrl.Request) {
    var awsRemediation remediationv1.AWSRemediationRequest
    // ... process AWS remediation
}
```

**OR Unified controller with type switching**:
```go
func (r *RemediationReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&remediationv1.KubernetesRemediationRequest{}).
        For(&remediationv1.AWSRemediationRequest{}).
        For(&remediationv1.AzureRemediationRequest{}).
        Complete(r)
}
```

### Pros & Cons

**Pros**:
- ✅ **Perfect Type Safety**: Each CRD has exact fields needed
- ✅ **No Empty Fields**: Zero waste
- ✅ **Clear Separation**: K8s vs AWS vs Azure is explicit
- ✅ **Independent Evolution**: Each CRD can evolve independently
- ✅ **Simple Queries**: No confusion about which fields exist

**Cons**:
- ❌ **Multiple CRDs**: More API complexity
- ❌ **Gateway Complexity**: Must choose correct CRD type
- ❌ **Controller Complexity**: Must watch multiple CRD types
- ❌ **Code Duplication**: Common logic repeated across controllers
- ❌ **Listing Complexity**: Can't `kubectl get remediationrequests` (must query each type)

### Mitigation Strategies

**For Gateway Complexity**: Factory pattern
```go
func (g *Gateway) createRemediationCRD(signal *NormalizedSignal) (client.Object, error) {
    switch signal.TargetType {
    case "kubernetes":
        return g.createKubernetesRemediationCRD(signal)
    case "aws":
        return g.createAWSRemediationCRD(signal)
    }
}
```

**For Listing**: Use labels
```go
// All CRDs share common labels
Labels: map[string]string{
    "kubernaut.io/remediation": "true",
    "kubernaut.io/priority": signal.Priority,
}

// List all remediations across types
kubectl get kubernetesremediationrequests,awsremediationrequests -l kubernaut.io/remediation=true
```

**For Code Duplication**: Shared base reconciler
```go
type BaseRemediationReconciler struct {
    // Common logic
}

type KubernetesRemediationReconciler struct {
    BaseRemediationReconciler
    // K8s-specific logic
}
```

### Confidence Justification

**80% confidence** because:
- ✅ Perfect type safety
- ✅ Clear separation of concerns
- ⚠️ Adds operational complexity (multiple CRDs to manage)
- ⚠️ More code to maintain
- ⚠️ Listing/querying across types is awkward

---

## 📊 Alternative 4: Extensible Metadata Pattern

**Confidence**: 82%

### Schema

```go
type RemediationRequestSpec struct {
    // Universal fields
    AlertFingerprint string
    Priority         string
    TargetType       string

    // Core target info (minimal, typed)
    TargetIdentifier string // K8s: "namespace/kind/name", AWS: "region/resource-type/id"

    // Extensible metadata (key-value)
    TargetMetadata   map[string]string `json:"targetMetadata,omitempty"`

    // Structured data (when needed)
    TargetData       json.RawMessage   `json:"targetData,omitempty"`
}
```

### Examples

**Kubernetes**:
```json
{
  "targetType": "kubernetes",
  "targetIdentifier": "production/Pod/api-xyz",
  "targetMetadata": {
    "namespace": "production",
    "kind": "Pod",
    "name": "api-xyz",
    "alertmanagerURL": "https://..."
  },
  "targetData": {
    "labels": {"app": "api", "version": "v2"},
    "nodeSelector": {"disktype": "ssd"}
  }
}
```

**AWS**:
```json
{
  "targetType": "aws",
  "targetIdentifier": "us-east-1/ec2/i-abc123",
  "targetMetadata": {
    "region": "us-east-1",
    "resourceType": "ec2",
    "instanceId": "i-abc123",
    "accountId": "123456789012"
  },
  "targetData": {
    "instanceType": "t3.large",
    "availabilityZone": "us-east-1a",
    "tags": {...}
  }
}
```

### Controller Usage

```go
func (r *Reconciler) processRemediation(remediation *remediationv1.RemediationRequest) error {
    // Parse identifier
    parts := strings.Split(remediation.Spec.TargetIdentifier, "/")

    // Access metadata (flat, typed as strings)
    namespace := remediation.Spec.TargetMetadata["namespace"]
    region := remediation.Spec.TargetMetadata["region"]

    // Parse structured data when needed
    if remediation.Spec.TargetData != nil {
        var data map[string]interface{}
        json.Unmarshal(remediation.Spec.TargetData, &data)
    }
}
```

### Pros & Cons

**Pros**:
- ✅ **Queryable Metadata**: Can query by common metadata keys
- ✅ **Extensible**: Add metadata without schema changes
- ✅ **Identifier Pattern**: Standard format across providers
- ✅ **Flexible**: Structured data for complex cases

**Cons**:
- ⚠️ **String-Only Metadata**: All values are strings (no nested objects)
- ⚠️ **Parsing Required**: Must parse identifier string
- ⚠️ **Convention-Based**: Relies on metadata key conventions
- ⚠️ **Mixed Patterns**: Some data in metadata, some in targetData

### Mitigation Strategies

**For Type Safety**: Define metadata key constants
```go
const (
    MetadataNamespace    = "namespace"
    MetadataRegion       = "region"
    MetadataResourceType = "resourceType"
)
```

**For Identifier Parsing**: Helper functions
```go
func ParseKubernetesIdentifier(id string) (namespace, kind, name string, err error) {
    parts := strings.Split(id, "/")
    if len(parts) != 3 {
        return "", "", "", errors.New("invalid K8s identifier")
    }
    return parts[0], parts[1], parts[2], nil
}
```

### Confidence Justification

**82% confidence** because:
- ✅ Good balance of flexibility and structure
- ✅ Queryable metadata
- ⚠️ Requires conventions and documentation
- ⚠️ String-only metadata is limiting for complex data

---

## 📊 Alternative 5: Dynamic CRD with Schema Registry

**Confidence**: 80%

### Architecture

**Core CRD** (minimal, dynamic):
```go
type RemediationRequestSpec struct {
    // Minimal universal fields
    AlertFingerprint string
    Priority         string

    // Schema reference
    SchemaVersion    string          `json:"schemaVersion"` // "kubernetes-v1", "aws-v1"

    // Dynamic data (validated against schema)
    Data             json.RawMessage `json:"data"`
}
```

**Schema Registry** (ConfigMaps or CRDs):
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: remediation-schema-kubernetes-v1
data:
  schema: |
    {
      "type": "object",
      "required": ["namespace", "resource"],
      "properties": {
        "namespace": {"type": "string"},
        "resource": {
          "type": "object",
          "properties": {
            "kind": {"type": "string"},
            "name": {"type": "string"}
          }
        }
      }
    }
```

### Validation

**Gateway validates against schema**:
```go
func (g *Gateway) createRemediationCRD(signal *NormalizedSignal) error {
    // Load schema
    schema, err := g.schemaRegistry.GetSchema(signal.SchemaVersion)
    if err != nil {
        return fmt.Errorf("schema not found: %w", err)
    }

    // Validate data against schema
    if err := schema.Validate(signal.Data); err != nil {
        return fmt.Errorf("data validation failed: %w", err)
    }

    // Create CRD
    return g.k8sClient.Create(ctx, &remediationv1.RemediationRequest{
        Spec: remediationv1.RemediationRequestSpec{
            SchemaVersion: signal.SchemaVersion,
            Data:          signal.Data,
        },
    })
}
```

### Pros & Cons

**Pros**:
- ✅ **Ultimate Flexibility**: Schema evolves without CRD changes
- ✅ **Versioned Schemas**: Multiple versions can coexist
- ✅ **Validation**: JSON Schema validation ensures correctness
- ✅ **Runtime Evolution**: Add new schemas without redeployment

**Cons**:
- ❌ **Complexity**: Schema registry adds significant complexity
- ❌ **Runtime Validation**: Can't use K8s CRD validation
- ❌ **Learning Curve**: Team must learn JSON Schema
- ❌ **Debugging**: Harder to debug schema issues
- ❌ **Tooling**: Requires custom tooling for schema management

### Confidence Justification

**80% confidence** (minimum threshold) because:
- ✅ Very flexible and powerful
- ⚠️ High complexity - only worth it for highly dynamic systems
- ⚠️ Requires schema registry infrastructure
- ⚠️ May be over-engineering for this use case

---

## 📊 Comparison Matrix

| Criteria | Alt 1: Raw JSON | Alt 2: Typed Union | Alt 3: Separate CRDs | Alt 4: Metadata | Alt 5: Schema Registry |
|----------|----------------|-------------------|---------------------|----------------|----------------------|
| **Confidence** | 90% | 85% | 80% | 82% | 80% |
| **Type Safety** | ⚠️ Medium | ✅ High | ✅ Highest | ⚠️ Medium | ⚠️ Low |
| **Simplicity** | ✅ Highest | ⚠️ Medium | ❌ Low | ⚠️ Medium | ❌ Lowest |
| **Scalability** | ✅ Highest | ⚠️ Medium | ⚠️ Medium | ✅ High | ✅ Highest |
| **Query Support** | ❌ Via Labels | ✅ Native | ✅ Native | ✅ Metadata | ❌ Limited |
| **K8s Validation** | ❌ No | ✅ Yes | ✅ Yes | ⚠️ Partial | ❌ No |
| **Schema Changes** | ✅ None | ❌ Per Provider | ❌ New CRD | ✅ None | ✅ None |
| **Implementation Time** | ✅ 1-2 days | ⚠️ 3-4 days | ❌ 5-7 days | ⚠️ 2-3 days | ❌ 7-10 days |
| **Maintenance** | ✅ Low | ⚠️ Medium | ❌ High | ⚠️ Medium | ❌ High |

---

## 🎯 Recommendations by Use Case

### **For V1 (K8s Only, Plan for Future)**
→ **Alternative 1** (Raw JSON) - 90% confidence
- Simple to implement now
- Easy to extend later
- Proven pattern

### **For Multi-Provider from Day 1**
→ **Alternative 2** (Typed Union) - 85% confidence
- Strong type safety
- Good developer experience
- Worth the complexity

### **For Provider-Specific Logic**
→ **Alternative 3** (Separate CRDs) - 80% confidence
- Clear separation
- Independent evolution
- Accept the operational complexity

### **For Hybrid Approach**
→ **Alternative 4** (Metadata Pattern) - 82% confidence
- Balance of structure and flexibility
- Queryable metadata
- Good for gradual adoption

### **For Highly Dynamic Systems**
→ **Alternative 5** (Schema Registry) - 80% confidence
- Only if you need extreme flexibility
- Only if you have schema management expertise

---

## 🎯 Final Recommendation

**For Kubernaut V1**: **Alternative 1 (Raw JSON Provider Data)**

**Why**:
1. ✅ **Highest Confidence** (90%)
2. ✅ **Fastest to Implement** (1-2 days)
3. ✅ **Most Flexible** (add providers without schema changes)
4. ✅ **Simplest** (one pattern for all providers)
5. ✅ **Proven** (used in Tekton, ArgoCD, Crossplane)

**Trade-offs Accepted**:
- No K8s validation (mitigate with Gateway adapter validation)
- No field queries (mitigate with labels)
- JSON parsing required (mitigate with typed helper structs)

**When to Reconsider**:
- If K8s validation is critical → Choose Alternative 2
- If provider-specific controllers needed → Choose Alternative 3
- If extreme type safety required → Choose Alternative 2

---

**What's your decision?** Which alternative aligns best with your requirements?
