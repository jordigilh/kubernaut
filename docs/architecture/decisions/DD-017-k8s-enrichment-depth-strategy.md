# DD-017: Kubernetes Enrichment Depth Strategy

## Status

**APPROVED** - 2025-11-27

## Context

The Signal Processing service enriches incoming signals with Kubernetes context to enable customer-defined Rego policies to make informed classification decisions. Per [ADR-041](ADR-041-rego-policy-data-fetching-separation.md), the K8s Enricher (Go code) fetches Kubernetes objects, and Rego policies evaluate classification rules on the fetched data.

A key design question is: **What objects should we fetch for each signal type, and how deep should we traverse the Kubernetes object graph?**

### Problem Statement

Different signal types reference different Kubernetes resources:
- A **Pod alert** references a specific pod, which runs on a node and is owned by a workload (Deployment/StatefulSet/DaemonSet)
- A **Deployment alert** references a deployment, which manages multiple pods
- A **Node alert** references a node, which has no namespace

Naive approaches have problems:
1. **Fetch everything**: Expensive, slow, unnecessary data
2. **Fetch only primary resource**: Missing context for classification (e.g., pod alert without node context can't distinguish app vs infra issues)
3. **Make it configurable**: Configuration complexity, testing burden, SRE confusion

## Decision

**We adopt signal-driven enrichment with hardcoded standard depth (no configuration).**

### Standard Depth Strategy (Hardcoded)

| Signal Type | Fetched Objects | Rationale |
|-------------|-----------------|-----------|
| **Pod** | Namespace + Pod + Node + Owner | Node context distinguishes app vs infra issues; Owner provides workload context |
| **Deployment** | Namespace + Deployment | Pods are ephemeral; deployment is source of truth |
| **StatefulSet** | Namespace + StatefulSet | Same as Deployment |
| **DaemonSet** | Namespace + DaemonSet | Same as Deployment |
| **ReplicaSet** | Namespace + ReplicaSet + Owner | Owner (Deployment) provides higher-level context |
| **Node** | Node only | Node is top-level; no namespace for node signals |
| **Service** | Namespace + Service | Endpoints rarely needed for classification |
| **Unknown** | Namespace only | Graceful fallback for unrecognized resource types |

### Key Design Principles

1. **Signal-Driven**: Enrichment adapts to `signal.Resource.Kind`, not fixed for all signals
2. **Standard Depth**: Predefined depth per resource type (no configuration)
3. **Graceful Degradation**: If related objects can't be fetched, continue with partial context
4. **No Configuration**: YAGNI - add configuration only if users demonstrate real need

## Alternatives Considered

### Alternative A: Fetch Everything (REJECTED)

Always fetch Namespace + Pod + Deployment + StatefulSet + DaemonSet + Node + Service.

| Pros | Cons |
|------|------|
| Complete context always available | 6-8 API calls per signal (expensive) |
| Simple implementation | Latency exceeds 2s P95 target |
| | Most data unused for most signals |

**Rejection Reason**: Performance impact unacceptable; violates BR-SP-001 (<2s P95 latency).

### Alternative B: Fetch Only Primary Resource (REJECTED)

Fetch only the resource referenced in the signal (e.g., Pod signal → Pod only).

| Pros | Cons |
|------|------|
| Fast (1-2 API calls) | Missing context for classification |
| Simple implementation | Can't distinguish app vs infra issues |
| | Pod alerts can't see node conditions |

**Rejection Reason**: Insufficient context for meaningful classification. A pod OOMKilled on a node with memory pressure requires node context to classify correctly.

### Alternative C: Configurable Depth (REJECTED)

Allow SREs to configure enrichment depth via ConfigMap:

```yaml
enrichment:
  depth: "standard"  # Options: minimal, standard, full
```

| Pros | Cons |
|------|------|
| Flexible for different use cases | Configuration complexity |
| Users can tune performance | Testing burden (3x permutations) |
| | Documentation overhead |
| | "What depth should I use?" support tickets |
| | Default still needed (what is it?) |

**Rejection Reason**: YAGNI. No demonstrated need for configurability. Standard depth is sensible for all known use cases. Add configuration only if users demonstrate real need through feedback.

## Implementation

### Signal-Driven Enrichment

```go
package enricher

import (
    "context"
    "fmt"

    "go.uber.org/zap"
    signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// Enrich fetches Kubernetes context based on signal type (standard depth).
// DD-017: Signal-driven enrichment with hardcoded standard depth.
func (e *K8sEnricher) Enrich(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error) {
    result := &signalprocessingv1alpha1.KubernetesContext{}

    switch signal.Resource.Kind {
    case "Pod":
        return e.enrichPodSignal(ctx, signal, result)
    case "Deployment":
        return e.enrichDeploymentSignal(ctx, signal, result)
    case "StatefulSet":
        return e.enrichStatefulSetSignal(ctx, signal, result)
    case "DaemonSet":
        return e.enrichDaemonSetSignal(ctx, signal, result)
    case "ReplicaSet":
        return e.enrichReplicaSetSignal(ctx, signal, result)
    case "Node":
        return e.enrichNodeSignal(ctx, signal, result)
    default:
        return e.enrichNamespaceOnly(ctx, signal, result)
    }
}
```

### Pod Signal Enrichment (Standard Depth)

```go
// enrichPodSignal fetches Namespace + Pod + Node + Owner.
// DD-017: Standard depth for pod signals includes node context for infra vs app classification.
func (e *K8sEnricher) enrichPodSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
    // 1. Namespace (always)
    ns, err := e.getNamespace(ctx, signal.Namespace)
    if err != nil {
        return nil, fmt.Errorf("failed to get namespace: %w", err)
    }
    result.Namespace = ns

    // 2. Pod (primary resource)
    pod, err := e.getPod(ctx, signal.Namespace, signal.Resource.Name)
    if err != nil {
        e.logger.Warn("Pod not found, continuing with partial context", zap.Error(err))
        return result, nil // Graceful degradation
    }
    result.Pod = pod

    // 3. Node where pod runs (standard depth - enables infra vs app classification)
    if pod.NodeName != "" {
        node, _ := e.getNode(ctx, pod.NodeName)
        result.Node = node
    }

    // 4. Owner workload (Deployment/StatefulSet/DaemonSet)
    owner, _ := e.getOwnerWorkload(ctx, signal.Namespace, pod.OwnerReferences)
    result.Owner = owner

    return result, nil
}
```

### Why Node Context for Pod Signals?

| Scenario | Without Node Context | With Node Context |
|----------|---------------------|-------------------|
| Pod OOMKilled | "Pod failed" | "Pod failed on node with memory pressure" |
| Pod CrashLooping | "Pod restarting" | "Pod restarting on node with disk pressure" |
| Pod Pending | "Pod not scheduled" | "Pod not scheduled, node has taints" |
| Pod Evicted | "Pod terminated" | "Pod evicted due to node resource exhaustion" |

Node context enables Rego policies to distinguish:
- **Application issues**: Bug in code, resource limits too low, bad configuration
- **Infrastructure issues**: Node unhealthy, resource exhaustion, hardware failure

This distinction is critical for routing alerts to the correct team (app developers vs SREs/platform team).

## Consequences

### Positive

1. **Appropriate Context**: Each signal type gets relevant context without over-fetching
2. **Performance**: 2-4 API calls per signal (within 2s P95 target)
3. **Simplicity**: No configuration to manage or document
4. **Testability**: Single code path per signal type
5. **Operational Clarity**: SREs don't need to decide on enrichment depth

### Negative

1. **Less Flexibility**: Can't tune enrichment per environment
2. **Hardcoded Assumptions**: Assumes standard depth is appropriate for all customers

### Mitigations

| Negative | Mitigation |
|----------|------------|
| Less Flexibility | Add configuration only if customers demonstrate real need via feedback |
| Hardcoded Assumptions | Standard depth covers all known use cases; revisit if gaps found |

## Future Considerations

If customers request configurable depth in the future:

1. **Collect Feedback**: Document specific use cases requiring different depth
2. **Evaluate Patterns**: Determine if requests cluster around specific needs
3. **Design Configuration**: If warranted, design simple configuration (2-3 options, not per-resource)
4. **Default to Standard**: Any configuration should default to current standard depth

## Related Decisions

- [ADR-041: Rego Policy Data Fetching Separation](ADR-041-rego-policy-data-fetching-separation.md) - Architecture for K8s Enricher + Rego
- [DD-CATEGORIZATION-001: Gateway/Signal Processing Split](DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md) - Signal Processing categorization ownership

## References

- [Signal Processing Implementation Plan](../../services/crd-controllers/01-signalprocessing/IMPLEMENTATION_PLAN_V1.2.md)
- [Kubernetes API Concepts](https://kubernetes.io/docs/concepts/overview/kubernetes-api/)

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-11-27 | AI Assistant | Initial DD creation |

