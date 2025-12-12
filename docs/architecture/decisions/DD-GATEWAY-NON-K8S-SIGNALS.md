# DD-GATEWAY-NON-K8S-SIGNALS: Non-Kubernetes Signal Support

## Status
⏳ **PROPOSED** (December 2, 2025)
**Target Release**: V1.x or V2.0 (based on V1.0 feedback)
**Last Reviewed**: December 2, 2025
**Confidence**: 70%

---

## Context & Problem

V1.0 of Kubernaut only supports Kubernetes-based signals (Prometheus alerts from K8s workloads, Kubernetes events). The Gateway currently validates that all signals have valid Kubernetes resource info (`Kind`, `Name`, `Namespace`).

**Current V1.0 Behavior**:
- Gateway rejects signals without resource info with HTTP 400
- Error message: `"V1.0 requires valid Kubernetes resource info"`
- `TargetType` field is always set to `"kubernetes"`
- Metrics track rejected signals: `gateway_signals_rejected_total{reason="missing_resource_*"}`

**Future Requirements**:
- Support non-Kubernetes signals (AWS CloudWatch, Datadog, Azure Monitor, GCP Cloud Monitoring)
- These signals won't have Kubernetes `Kind/Name/Namespace`
- Need alternative resource identification (ARN, Datadog host, etc.)

---

## Key Requirements

1. **Backward Compatibility**: Existing Kubernetes signals must continue to work
2. **Clear Resource Identification**: Each target type must have appropriate resource identification
3. **Validation Per Target Type**: Different validation rules for different target types
4. **Downstream Compatibility**: SignalProcessing, AIAnalysis, and WE must handle non-K8s targets

---

## Alternatives Considered

### Alternative 1: Polymorphic TargetResource (Complex)

**Approach**: Make `TargetResource` a union type with different schemas per target type.

```go
type TargetResource struct {
    // Kubernetes (existing)
    Kubernetes *KubernetesResource `json:"kubernetes,omitempty"`
    // AWS
    AWS *AWSResource `json:"aws,omitempty"`
    // Azure
    Azure *AzureResource `json:"azure,omitempty"`
    // GCP
    GCP *GCPResource `json:"gcp,omitempty"`
    // Datadog
    Datadog *DatadogResource `json:"datadog,omitempty"`
}

type KubernetesResource struct {
    Kind      string `json:"kind"`
    Name      string `json:"name"`
    Namespace string `json:"namespace,omitempty"`
}

type AWSResource struct {
    ARN       string `json:"arn"`
    AccountID string `json:"accountId"`
    Region    string `json:"region"`
}
```

**Pros**:
- ✅ Type-safe schemas per target type
- ✅ Clear validation rules per type
- ✅ Extensible for future targets

**Cons**:
- ❌ Breaking CRD schema change
- ❌ Complex migration path
- ❌ Downstream services need significant updates

**Confidence**: 60% (rejected for V1.x due to complexity)

---

### Alternative 2: String-Based Resource ID (Simple) ✅ RECOMMENDED

**Approach**: Add a flexible `resourceId` string field alongside Kubernetes-specific fields.

```go
type RemediationRequestSpec struct {
    // Existing Kubernetes resource identification (required for TargetType=kubernetes)
    TargetResource ResourceIdentifier `json:"targetResource"`

    // Generic resource identifier for non-Kubernetes targets (V2.0)
    // Examples:
    //   AWS: "arn:aws:ec2:us-east-1:123456789:instance/i-1234567890abcdef0"
    //   Datadog: "host:web-server-01"
    //   Azure: "/subscriptions/.../resourceGroups/.../providers/Microsoft.Compute/virtualMachines/vm-01"
    // +kubebuilder:validation:MaxLength=2048
    ResourceID string `json:"resourceId,omitempty"`

    // Target system type (already exists)
    // +kubebuilder:validation:Enum=kubernetes;aws;azure;gcp;datadog
    TargetType string `json:"targetType"`
}
```

**Validation Logic**:
```go
func (c *CRDCreator) validateResourceInfo(signal *types.NormalizedSignal) error {
    switch signal.TargetType {
    case "kubernetes":
        // V1.0 behavior - require Kind and Name
        if signal.Resource.Kind == "" || signal.Resource.Name == "" {
            return fmt.Errorf("kubernetes signals require Kind and Name")
        }
    case "aws", "azure", "gcp", "datadog":
        // V2.0 behavior - require ResourceID
        if signal.ResourceID == "" {
            return fmt.Errorf("%s signals require resourceId", signal.TargetType)
        }
    default:
        return fmt.Errorf("unsupported target type: %s", signal.TargetType)
    }
    return nil
}
```

**Pros**:
- ✅ Minimal CRD schema change (additive only)
- ✅ No breaking changes for V1.0 consumers
- ✅ Simple downstream adaptation
- ✅ Flexible for future target types

**Cons**:
- ⚠️ Less type-safety for non-K8s resources (mitigated by validation)
- ⚠️ Requires documentation of expected formats per target type

**Confidence**: 85% (recommended)

---

### Alternative 3: Defer to V3.0 (Wait and See)

**Approach**: Keep V1.0 Kubernetes-only, evaluate demand before designing.

**Pros**:
- ✅ No premature abstraction
- ✅ Learn from V1.0 production usage

**Cons**:
- ❌ May require breaking changes later
- ❌ Delays feature availability

**Confidence**: 50% (not recommended if there's known demand)

---

## Decision

**PROPOSED: Alternative 2** - String-Based Resource ID

**Rationale**:
1. **Minimal Disruption**: Additive change, no breaking modifications
2. **Flexibility**: String-based IDs work for any target type
3. **Simple Validation**: Per-target-type validation logic is straightforward
4. **Documentation-Driven**: Format expectations documented per target type

**Implementation Notes**:
- Add `resourceId` field as optional (empty for K8s signals)
- Validation checks `targetType` and requires appropriate fields
- Downstream services route based on `targetType`

---

## Implementation Plan

### Phase 1: V1.0 Foundation (DONE)
- [x] `TargetType` field exists with enum validation
- [x] `TargetType` populated as `"kubernetes"` for all V1.0 signals
- [x] Validation rejects signals without K8s resource info
- [x] Metrics track rejected signals

### Phase 2: V1.x/V2.0 Non-K8s Support (PROPOSED)
1. Add `resourceId` field to RemediationRequest CRD
2. Add adapters for non-K8s signal sources (CloudWatch, Datadog)
3. Update validation logic for per-target-type rules
4. Update SignalProcessing for non-K8s enrichment (or skip)
5. Update AIAnalysis/HolmesGPT for non-K8s RCA
6. Update WorkflowExecution for non-K8s remediation workflows

---

## Consequences

**Positive**:
- ✅ Clear path for non-Kubernetes signal support
- ✅ No breaking changes for V1.0 consumers
- ✅ Extensible architecture

**Negative**:
- ⚠️ Additional complexity in validation logic
- ⚠️ Requires documentation of resource ID formats

**Neutral**:
- 🔄 Timeline depends on V1.0 feedback and demand

---

## Success Metrics

- **Adoption**: Number of non-K8s signals processed per day
- **Error Rate**: Rejection rate for non-K8s signals (should be <5%)
- **Downstream Success**: SP/AIAnalysis/WE completion rate for non-K8s signals

---

## Related Documents

- **BR-GATEWAY-TARGET-RESOURCE-VALIDATION**: V1.0 resource validation requirement
- **DD-CATEGORIZATION-001**: Signal categorization split (Gateway vs SignalProcessing)
- **RO_TO_GATEWAY_CONTRACT_ALIGNMENT.md**: Q3 response about rejecting signals without resource info

---

## Review & Evolution

**When to Revisit**:
- After V1.0 production deployment
- When first non-K8s integration is requested
- Before V2.0 planning

**Success Criteria**:
- Non-K8s signals can be processed end-to-end
- Validation errors are clear and actionable
- Existing K8s functionality unchanged

---

**Document Version**: 1.0
**Created**: December 2, 2025
**Author**: Gateway Team (AI-assisted)





