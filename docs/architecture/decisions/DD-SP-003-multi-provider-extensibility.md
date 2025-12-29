# DD-SP-003: Multi-Provider Extensibility Design

**Status**: ✅ APPROVED
**Date**: 2025-12-16
**Author**: SignalProcessing Team
**Stakeholders**: All Teams

---

## 📋 **Decision Summary**

**Decision**: Use the existing `spec.signal.targetType` CRD field as the routing discriminator for future multi-provider enrichment support.

**Approach Selected**: **Path A - Use Existing Field**

---

## 🎯 **Context**

When evolving Kubernaut to a full-stack AIOps platform supporting multiple infrastructure providers (AWS, Azure, GCP, Datadog, etc.), the SignalProcessing controller needs a mechanism to route signals to the appropriate enrichment logic.

### Options Evaluated

| Option | Description | Complexity | Backward Compatible |
|--------|-------------|------------|---------------------|
| **A: Use existing `targetType`** | Route based on `spec.signal.targetType` enum | Low | ✅ Yes |
| B: Add new `provider` field | Separate field for provider identification | Medium | ✅ Yes |
| C: Derive from `type`/`source` | Infer provider from signal metadata | High | ✅ Yes |

---

## ✅ **Decision: Path A - Use Existing `targetType` Field**

### Rationale

1. **Field Already Exists**: `spec.signal.targetType` is already defined in the CRD schema with proper enum validation (`kubernetes|aws|azure|gcp|datadog`)

2. **Gateway Integration Ready**: Gateway's `crd_creator.go` already sets this field (currently hardcoded to `"kubernetes"`)

3. **No Schema Changes Required**: V1.0 CRDs are already compatible with multi-provider routing

4. **Clear Semantics**: `targetType` explicitly identifies the target infrastructure system for enrichment

### Current State

```yaml
# api/signalprocessing/v1alpha1/signalprocessing_types.go
spec:
  signal:
    targetType: "kubernetes"  # enum: kubernetes|aws|azure|gcp|datadog
```

### V2.0 Extensibility Pattern

```go
// internal/controller/signalprocessing/signalprocessing_controller.go
func (r *SignalProcessingReconciler) reconcileEnriching(ctx context.Context, sp *spv1.SignalProcessing, ...) (ctrl.Result, error) {
    switch sp.Spec.Signal.TargetType {
    case "kubernetes":
        return r.enrichKubernetesContext(ctx, sp, logger)  // Current implementation
    case "aws":
        return r.enrichAWSContext(ctx, sp, logger)         // CloudWatch, EC2, EKS context
    case "azure":
        return r.enrichAzureContext(ctx, sp, logger)       // Azure Monitor, AKS context
    case "gcp":
        return r.enrichGCPContext(ctx, sp, logger)         // Cloud Monitoring, GKE context
    case "datadog":
        return r.enrichDatadogContext(ctx, sp, logger)     // Datadog API context
    default:
        return ctrl.Result{}, fmt.Errorf("unsupported targetType: %s", sp.Spec.Signal.TargetType)
    }
}
```

---

## 📌 **Implementation Tasks for V2.0**

When implementing multi-provider support:

### Phase 1: Gateway Extension
1. [ ] Parse provider from incoming signal source
2. [ ] Dynamically set `targetType` based on signal origin
3. [ ] Add new `type` enum values (`aws-cloudwatch`, `azure-monitor`, etc.)

### Phase 2: SP Controller Extension
1. [ ] Add provider-specific enrichment functions
2. [ ] Extend `KubernetesContext` → `EnrichmentContext` (generic)
3. [ ] Add provider-specific conditions and error reasons
4. [ ] Implement degraded mode per provider

### Phase 3: CRD Status Extension
1. [ ] Add `AWSContext`, `AzureContext`, etc. to status
2. [ ] Or: Use generic `ProviderContext` with flexible schema
3. [ ] Update conditions for provider-specific failures

---

## 🔍 **Related Files**

| File | Purpose |
|------|---------|
| `api/signalprocessing/v1alpha1/signalprocessing_types.go` | CRD schema with `targetType` enum |
| `internal/controller/signalprocessing/signalprocessing_controller.go` | Extensibility comments added |
| `pkg/gateway/processing/crd_creator.go` | Gateway sets `targetType` (currently hardcoded) |

---

## 📊 **Impact Assessment**

| Aspect | V1.0 Impact | V2.0 Effort |
|--------|-------------|-------------|
| CRD Schema | ✅ None - field exists | Medium - status extension |
| SP Controller | ✅ Comments added | Medium - switch routing |
| Gateway | ✅ None - hardcoded | Low - dynamic detection |
| Tests | ✅ None | High - provider mocks |
| Documentation | ✅ This DD | Medium - provider guides |

---

## ✅ **Approval**

- [x] **SignalProcessing Team** - @jgil - 2025-12-16 - Approved

---

## 📚 **References**

- BR-SP-001: K8s Context Enrichment
- BR-SP-100: Owner Chain Traversal
- BR-SP-101: Detected Labels
- `reconcileEnriching` function in `signalprocessing_controller.go` (contains extensibility roadmap)



