# Response: Label Passthrough from Gateway to SignalProcessing

**From**: Gateway Service Team
**To**: SignalProcessing Service Team
**Date**: November 30, 2025
**Re**: [HANDOFF_REQUEST_GATEWAY_LABEL_PASSTHROUGH.md](HANDOFF_REQUEST_GATEWAY_LABEL_PASSTHROUGH.md)
**Status**: ✅ CONFIRMED

---

## Document Version

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| **1.0** | Nov 30, 2025 | Gateway Team | Response to handoff request |

---

## Summary

**Your assumptions are correct.** Gateway provides signal metadata and resource identifiers. SignalProcessing must query K8s API for namespace/pod labels. Below are detailed answers to each question.

---

## Q1: What labels does Gateway currently extract from webhooks?

### ✅ Confirmed: Gateway extracts ALL alert labels and passes them through

| Label Source | Stored In | Available to SignalProcessing |
|--------------|-----------|-------------------------------|
| `alert.Labels` | `RemediationRequest.Spec.SignalLabels` | ✅ Yes |
| `webhook.CommonLabels` | Merged into `SignalLabels` | ✅ Yes |
| `alert.Annotations` | `RemediationRequest.Spec.SignalAnnotations` | ✅ Yes |
| `webhook.CommonAnnotations` | Merged into `SignalAnnotations` | ✅ Yes |

### Label Table Correction

| Label | Source | Set by Gateway? | Notes |
|-------|--------|-----------------|-------|
| `signal_type` | Alert labels | ✅ Yes | Stored as `Spec.SignalType` |
| `severity` | Alert labels | ✅ Yes | Stored as `Spec.Severity` |
| `component` | Alert labels | ✅ Yes | In `SignalLabels` if present |
| `priority` | Derived | ✅ Yes | Placeholder P2 → refined by SignalProcessing |
| `namespace` | Alert labels | ✅ Yes | In `SignalLabels["namespace"]` and `Spec.Namespace` |
| **Namespace labels** | K8s API | ❌ **NO** | SignalProcessing must fetch |
| **Pod labels** | K8s API | ❌ **NO** | SignalProcessing must fetch |

### Implementation Reference

```go
// pkg/gateway/adapters/prometheus_adapter.go:142-143
labels := MergeLabels(alert.Labels, webhook.CommonLabels)
annotations := MergeAnnotations(alert.Annotations, webhook.CommonAnnotations)

// pkg/gateway/processing/crd_creator.go:343-344
SignalLabels:      c.truncateLabelValues(signal.Labels),
SignalAnnotations: c.truncateAnnotationValues(signal.Annotations),
```

**Note**: Label values are truncated to 63 characters (K8s label value max length).

---

## Q2: Does Gateway pass through alert annotations?

### ✅ YES - All annotations are passed through

Gateway merges and stores all annotations:

```yaml
# Example RemediationRequest created by Gateway
spec:
  signalAnnotations:
    summary: "Pod OOMKilled"
    description: "Container exceeded memory limits"
    runbook_url: "https://runbooks.company.com/oom"
    kubernaut.io/team: "payments"  # Custom annotations preserved!
```

**Implementation**: `MergeAnnotations()` in `pkg/gateway/adapters/prometheus_adapter.go:359`

---

## Q3: Is the current interface sufficient for label extraction?

### ✅ CONFIRMED: SignalProcessing should query K8s API for resource labels

| Field | Source | Available in RemediationRequest? |
|-------|--------|----------------------------------|
| `namespace.name` | RemediationRequest | ✅ `Spec.SignalLabels["namespace"]` |
| `namespace.labels` | K8s API query | ❌ SignalProcessing must fetch |
| `pod.name` | RemediationRequest | ✅ `Spec.SignalLabels["pod"]` or `AffectedResources` |
| `pod.labels` | K8s API query | ❌ SignalProcessing must fetch |
| `signal.type` | RemediationRequest | ✅ `Spec.SignalType` |
| `signal.severity` | RemediationRequest | ✅ `Spec.Severity` |
| `alert.labels` | RemediationRequest | ✅ `Spec.SignalLabels` (all of them) |
| `alert.annotations` | RemediationRequest | ✅ `Spec.SignalAnnotations` (all of them) |

### What Gateway Provides (Complete List)

```yaml
# RemediationRequest.Spec (created by Gateway)
spec:
  # Core identification
  signalFingerprint: "a1b2c3d4..."
  signalName: "HighMemoryUsage"
  signalType: "prometheus-alert"
  signalSource: "prometheus-adapter"

  # Classification (Gateway-assigned)
  severity: "critical"
  environment: "prod"           # Classified from namespace
  priority: "P2"                # Placeholder - SignalProcessing refines

  # Resource identification
  namespace: "prod-payment"     # Primary namespace
  targetType: "kubernetes"

  # ALL alert labels passed through
  signalLabels:
    alertname: "HighMemoryUsage"
    namespace: "prod-payment"
    pod: "payment-api-789"
    container: "api"
    severity: "critical"
    cluster: "prod-us-west"
    # ... ALL labels from alert + commonLabels

  # ALL alert annotations passed through
  signalAnnotations:
    summary: "Container memory usage > 90%"
    description: "Pod payment-api-789 memory at 94%"
    runbook_url: "https://..."
    # ... ALL annotations from alert + commonAnnotations

  # Original payload for audit
  originalPayload: <base64>

  # Storm detection (if applicable)
  isStorm: false
  affectedResources: []
```

---

## Q4: Any plans to add label extraction to Gateway?

### ❌ NO - Per DD-CATEGORIZATION-001

**Current design principle**: Gateway is a fast signal ingestion layer (target: <50ms p95). Adding K8s API calls for namespace/pod labels would:

1. **Increase latency** - Additional API calls per signal
2. **Add coupling** - Gateway would need K8s client permissions
3. **Duplicate work** - SignalProcessing already has enrichment infrastructure

**Recommendation**: SignalProcessing should continue to fetch K8s context. This aligns with the "Investigation vs. Execution" separation principle in DD-CATEGORIZATION-001.

### Future Consideration (Not Planned)

If we ever find that:
- Same K8s queries are made repeatedly for high-volume namespaces
- Gateway has <5ms headroom to spare

Then we could explore **optional caching** of namespace labels in Gateway. But this is not planned for V1.0.

---

## Optimization Opportunity

### Avoiding Duplicate API Calls

SignalProcessing can **skip redundant queries** by checking what's already in `SignalLabels`:

```go
// SignalProcessing enrichment (pseudo-code)
func EnrichSignal(rr *RemediationRequest) {
    // Pod name already available - no API call needed
    podName := rr.Spec.SignalLabels["pod"]

    // But pod LABELS need API call
    pod, _ := k8sClient.Get(ctx, podName, namespace)
    detectedLabels["gitops"] = hasGitOpsLabels(pod.Labels)

    // Namespace name already available - no API call needed
    nsName := rr.Spec.SignalLabels["namespace"]

    // But namespace LABELS need API call
    ns, _ := k8sClient.Get(ctx, nsName)
    customLabels = regoPolicy.Evaluate(ns.Labels)
}
```

---

## Confirmation Summary

| Your Assumption | Status | Notes |
|-----------------|--------|-------|
| Gateway provides signal metadata | ✅ Correct | All labels/annotations passed through |
| Gateway does NOT provide namespace labels | ✅ Correct | No K8s API queries |
| SignalProcessing fetches all K8s context | ✅ Correct | Via API queries during enrichment |
| No Gateway changes needed for V1.0 | ✅ Correct | Current interface is sufficient |

---

## Related Implementation Files

| File | Purpose |
|------|---------|
| `pkg/gateway/adapters/prometheus_adapter.go` | Label/annotation extraction |
| `pkg/gateway/processing/crd_creator.go` | RemediationRequest creation |
| `pkg/gateway/types/types.go` | NormalizedSignal definition |
| `api/remediation/v1alpha1/remediationrequest_types.go` | CRD spec with SignalLabels/SignalAnnotations |

---

**Contact**: Gateway Service Team
**Questions**: Create issue or reach out on #kubernaut-dev

