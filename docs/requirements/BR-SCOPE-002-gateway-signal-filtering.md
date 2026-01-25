# BR-SCOPE-002: Gateway Signal Filtering

**Business Requirement ID**: BR-SCOPE-002
**Title**: Gateway Signal Filtering for Unmanaged Resources
**Category**: Signal Processing / Resource Management
**Priority**: P0 (Critical - Performance & Security)
**Status**: ✅ APPROVED
**Created**: 2026-01-20
**Last Updated**: 2026-01-20
**Owner**: Gateway Team
**Parent BR**: BR-SCOPE-001 (Resource Scope Management)

---

## 📋 Executive Summary

Gateway MUST filter signals from unmanaged resources at ingestion time to prevent unnecessary CRD creation, reduce Kubernetes API load, and provide immediate feedback to signal sources.

**Core Principle**: **Fail fast - reject unmanaged signals before downstream processing.**

---

## 🎯 Business Need

### Problem Statement

Without Gateway-level filtering:
- ❌ Unnecessary RemediationRequest CRDs created for unmanaged resources
- ❌ Increased Kubernetes API load (CRD creation + deletion)
- ❌ Delayed feedback (signal processed through multiple services before rejection)
- ❌ Observability noise (audit events, metrics for unwanted signals)
- ❌ Resource waste (CPU, memory for unnecessary processing)

### Current Limitation

If Gateway only validates signal format but not resource scope:
- All signals create RemediationRequests (regardless of scope)
- RemediationOrchestrator must validate scope later (after SP, AI processing)
- Wasted compute for processing signals that will eventually be rejected

---

## ✅ Requirements

### FR-SCOPE-002-1: Signal Source Validation (V1.0)

**Requirement**: Gateway MUST validate that the signal source resource is managed by Kubernaut before creating a RemediationRequest CRD.

**Validation Logic** (2-Level Hierarchy):
```go
// Pseudocode
func isManaged(signal Signal) bool {
    // Level 1: Check signal source resource
    if signal.ResourceLabels["kubernaut.ai/managed"] == "true" {
        return true
    }
    if signal.ResourceLabels["kubernaut.ai/managed"] == "false" {
        return false
    }

    // Level 2: Check namespace (for namespaced resources)
    if signal.Namespace != "" {
        if ns.Labels["kubernaut.ai/managed"] == "true" {
            return true
        }
        if ns.Labels["kubernaut.ai/managed"] == "false" {
            return false
        }
    }

    // Default: Unmanaged (safe default)
    return false
}
```

**Implementation**:
- **Package**: `pkg/shared/scope/manager.go` (shared with RO)
- **Entry Point**: `pkg/gateway/server.go` (`ProcessSignal()`)
- **K8s Client**: Use cached client (controller-runtime metadata-only cache)

---

### FR-SCOPE-002-2: Rejection Response (V1.0)

**Requirement**: Gateway MUST return a clear, actionable HTTP response when rejecting signals from unmanaged resources.

**HTTP Response**:
```json
{
  "status": "rejected",
  "reason": "unmanaged_resource",
  "message": "Resource production/deployment/payment-api is not managed by Kubernaut. Add label 'kubernaut.ai/managed=true' to namespace 'production' or the resource to enable remediation.",
  "resource": {
    "namespace": "production",
    "kind": "Deployment",
    "name": "payment-api"
  },
  "action": "Add label kubernaut.ai/managed=true to namespace or resource"
}
```

**HTTP Status Code**: `200 OK` (not `400 Bad Request` - this is a validation decision, not a client error)

**Rationale**:
- Clear user feedback (no debugging required)
- Actionable instruction (exact label to add)
- Not treated as HTTP error (signal format was valid)

---

### FR-SCOPE-002-3: Observability (V1.0)

**Requirement**: Gateway MUST provide comprehensive observability for scope validation decisions.

**Prometheus Metrics**:
```promql
# Total signals rejected due to unmanaged resource
gateway_signals_rejected_total{reason="unmanaged_resource"}

# By namespace
gateway_signals_rejected_total{reason="unmanaged_resource", namespace="kube-system"}

# By signal type
gateway_signals_rejected_total{reason="unmanaged_resource", signal_type="prometheus"}
```

**Structured Logs**:
```json
{
  "level": "info",
  "service": "gateway",
  "msg": "Rejecting signal from unmanaged resource",
  "signal_name": "HighMemoryUsage",
  "namespace": "kube-system",
  "resource_kind": "Deployment",
  "resource_name": "coredns",
  "reason": "unmanaged_resource",
  "label_found": false
}
```

**Audit Events**: **NO** audit event emitted (per parent BR-SCOPE-001)
- Rationale: These are expected validation decisions, not business events
- Reduces audit noise for unmanaged signals
- Gateway logs + Prometheus metrics provide sufficient visibility

---

### FR-SCOPE-002-4: Performance (V1.0)

**Requirement**: Scope validation MUST NOT add significant latency to signal processing.

**Target SLA**:
- < 10ms per scope validation (P95)
- < 5ms per cached lookup (P50)

**Implementation Strategy**:
- Use controller-runtime cached client (metadata-only)
- Cache K8s namespace metadata in-memory (watch API)
- No direct API calls for scope validation (read from cache)

**API Call Cost** (Gateway):
- **0 API calls** per signal (reads from controller-runtime cache)
- **Watches**: Namespace metadata stream (one-time setup)

---

### NFR-SCOPE-002-1: No Duplicate CRD Creation

**Requirement**: Gateway MUST NOT create RemediationRequest CRDs for unmanaged resources.

**Rationale**:
- Prevents Kubernetes API pollution
- Reduces CRD count (less etcd storage)
- Avoids unnecessary downstream processing

**Validation**:
```bash
# Before scope filtering
kubectl get remediationrequests --all-namespaces | wc -l
# Output: 10,000 RRs (including 5,000 from unmanaged namespaces)

# After scope filtering
kubectl get remediationrequests --all-namespaces | wc -l
# Output: 5,000 RRs (only managed namespaces)
```

---

### NFR-SCOPE-002-2: Controller-Runtime Metadata Cache (V1.0)

**Requirement**: Gateway MUST use controller-runtime's metadata-only cache for scope validation to minimize memory footprint and API load.

**Implementation**:
```go
import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/cache"
)

// Configure cache for metadata-only
cacheOpts := cache.Options{
    DefaultUnsafeDisableDeepCopy: ptr.To(true), // Metadata is read-only
}

mgr, err := ctrl.NewManager(cfg, ctrl.Options{
    Cache: cacheOpts,
})

// Use PartialObjectMetadata for namespace lookups
ns := &metav1.PartialObjectMetadata{}
ns.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Namespace"))
err := cachedClient.Get(ctx, client.ObjectKey{Name: "production"}, ns)
// ns.Labels["kubernaut.ai/managed"] → "true"
```

**Benefits**:
- ✅ Minimal memory (only ObjectMeta, no spec/status)
- ✅ Zero API calls (reads from in-memory cache)
- ✅ Sub-millisecond latency (map lookup)
- ✅ Standard Kubernetes pattern (used by all controllers)

---

## 🔄 Signal Filtering Flow

### Flow Diagram

```
┌─────────────────────────────────────────────────────┐
│ Gateway Signal Processing with Scope Filtering     │
└─────────────────────────────────────────────────────┘

1. Signal arrives (Prometheus, K8s Event, Custom)
   ├─ Parse signal payload
   └─ Extract: namespace, resource kind, resource name

2. Format validation
   ├─ Valid? → Continue
   └─ Invalid? → Reject (HTTP 400)

3. Scope validation (NEW - FR-SCOPE-002-1)
   ├─ Check resource label (Level 1)
   │   ├─ kubernaut.ai/managed=true → MANAGED (go to 4)
   │   └─ kubernaut.ai/managed=false → UNMANAGED (go to 5)
   │
   ├─ Check namespace label (Level 2)
   │   ├─ kubernaut.ai/managed=true → MANAGED (go to 4)
   │   └─ kubernaut.ai/managed=false → UNMANAGED (go to 5)
   │
   └─ No label → UNMANAGED (go to 5)

4. Signal is MANAGED
   ├─ Create RemediationRequest CRD
   ├─ Emit audit: gateway.signal.received
   ├─ Increment: gateway_signals_received_total
   └─ Return HTTP 200 (success)

5. Signal is UNMANAGED (NEW - FR-SCOPE-002-2)
   ├─ Log: INFO "Rejecting signal from unmanaged resource"
   ├─ Increment: gateway_signals_rejected_total{reason="unmanaged_resource"}
   └─ Return HTTP 200 (rejection response with instructions)
```

---

## 📊 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| **CRD Reduction** | > 50% fewer RRs for unmanaged signals | Compare RR count before/after |
| **Latency Impact** | < 10ms added latency (P95) | Prometheus histogram |
| **False Rejections** | < 0.1% managed signals rejected | `signals_rejected_total` / `signals_received_total` |
| **Cache Hit Rate** | > 99% scope lookups from cache | controller-runtime metrics |
| **User Clarity** | > 90% operators understand rejection reason | Survey after 3 months |

---

## 🔗 Dependencies

### Upstream Dependencies

- **BR-SCOPE-001**: Resource Scope Management (parent BR)
- **Kubernetes API**: Namespace label watches
- **controller-runtime**: Cached client for metadata

### Downstream Impact

- **SignalProcessing**: Fewer SignalProcessing CRDs created (indirect)
- **AIAnalysis**: Fewer AIAnalysis CRDs created (indirect)
- **DataStorage**: Fewer audit events for rejected signals
- **Prometheus**: New metric `gateway_signals_rejected_total`

---

## 🚫 Out of Scope (V1.0)

1. ❌ **Signal Source Rewriting**: Gateway does not modify signal payload to add scope info
2. ❌ **Cluster-Scoped Resource Filtering**: V1.0 focuses on namespaced resources only
3. ❌ **Dynamic Scope Policies**: Rego policies for scope decisions (static labels only)
4. ❌ **Scope Change Notifications**: Proactive alerts when namespace labels change (reactive only)

---

## 🎯 Related Business Requirements

| BR ID | Title | Relationship |
|-------|-------|--------------|
| BR-SCOPE-001 | Resource Scope Management | Parent BR (defines opt-in model) |
| BR-SCOPE-010 | RO Routing Scope Validation | Defense-in-depth (RO validates again) |
| BR-GATEWAY-001 | Signal Ingestion | Gateway scope validation extends signal validation |
| BR-PLATFORM-001 | Kubernetes-Native | Uses native labels and controller-runtime |

---

## 📝 Implementation References

| Component | Implementation | Status |
|-----------|---------------|--------|
| **Shared Scope Manager** | `pkg/shared/scope/manager.go` | ⚠️ TODO |
| **Gateway Integration** | `pkg/gateway/server.go` (`ProcessSignal()`) | ⚠️ TODO |
| **Metadata Cache Setup** | `cmd/gateway/main.go` (controller-runtime config) | ⚠️ TODO |
| **Prometheus Metrics** | `pkg/gateway/metrics.go` (`signals_rejected_total`) | ⚠️ TODO |
| **Unit Tests** | `test/unit/gateway/scope_validation_test.go` | ⚠️ TODO |
| **Integration Tests** | `test/integration/gateway/scope_filtering_test.go` | ⚠️ TODO |

---

## ✅ Approval

**Approved By**: Gateway Team, Platform Team
**Date**: 2026-01-20
**Confidence**: 95%

**Approval Rationale**:
- ✅ Fail-fast principle (early rejection)
- ✅ Kubernetes-native (controller-runtime cache)
- ✅ Performance-conscious (no additional API calls)
- ✅ Clear user feedback (actionable error messages)
- ✅ Observable (metrics, logs)
- ✅ Defense-in-depth (RO validates again per BR-SCOPE-010)

**Next Steps**:
1. Implement shared scope manager (`pkg/shared/scope/manager.go`)
2. Integrate scope validation in Gateway `ProcessSignal()`
3. Configure controller-runtime metadata cache in `cmd/gateway/main.go`
4. Add Prometheus metric `gateway_signals_rejected_total`
5. Add unit and integration tests
6. Update Gateway user documentation

---

**Document Version**: 1.0
**Last Updated**: 2026-01-20
**Next Review**: 2026-04-20 (3 months)
