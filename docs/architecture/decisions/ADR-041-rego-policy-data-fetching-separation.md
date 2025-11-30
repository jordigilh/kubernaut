# ADR-041: Rego Policy Engine Data Fetching Separation

## Status

**APPROVED** - 2025-11-27

## Context

Kubernaut uses the Open Policy Agent (OPA) Rego policy engine for customer-configurable classification logic, including:

- **Environment Classification**: Determining production/staging/development based on K8s context
- **Priority Assignment**: Calculating signal priority based on business rules
- **Business Classification**: Categorizing signals based on customer-specific criteria

A key architectural decision is whether Rego policies should:
- **Option A**: Receive pre-fetched Kubernetes data as input (data fetching separated from policy evaluation)
- **Option B**: Fetch Kubernetes data directly via `http.send` during policy evaluation

This decision affects security, performance, maintainability, and the overall separation of concerns in the system.

## Decision

**We adopt Option A: Separate data fetching (Go/client-go) from policy evaluation (Rego).**

All Kubernetes API calls are made by Go code using client-go, and the resulting data is passed to Rego policies as input. Rego policies contain **only classification logic** and never make external API calls.

## Architecture

### Component Separation

```
┌─────────────────────────────────────────────────────────────────┐
│ SignalProcessing CRD (input: signal metadata only)              │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│ K8s Enricher (Go code, client-go)                               │
│  • Fetches K8s objects via authenticated K8s API calls          │
│  • Caches results (e.g., namespace labels don't change often)   │
│  • Handles errors, retries, rate limiting                       │
│  • Uses controller-runtime client with ServiceAccount RBAC      │
│                                                                 │
│  OUTPUT: KubernetesContext struct (raw data, no interpretation) │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│ Rego Policy Engine (OPA)                                        │
│                                                                 │
│  INPUT: {                                                       │
│    "signal": {                                                  │
│      "source": "prometheus",                                    │
│      "name": "HighMemoryUsage",                                 │
│      "labels": { "namespace": "prod-api", "pod": "api-xyz" }    │
│    },                                                           │
│    "kubernetesContext": {        ← Pre-fetched by K8s Enricher  │
│      "namespace": {                                             │
│        "name": "prod-api",                                      │
│        "labels": { "env": "production", "team": "platform" },   │
│        "annotations": { "owner": "sre-team" }                   │
│      },                                                         │
│      "deployment": {                                            │
│        "name": "api-server",                                    │
│        "replicas": 3,                                           │
│        "availableReplicas": 2                                   │
│      },                                                         │
│      "node": {                                                  │
│        "name": "worker-1",                                      │
│        "conditions": [...]                                      │
│      }                                                          │
│    }                                                            │
│  }                                                              │
│                                                                 │
│  EVALUATES: Customer-defined Rego policies (pure business logic)│
│                                                                 │
│  OUTPUT: Classifications with confidence scores                 │
└─────────────────────────────────────────────────────────────────┘
```

### Responsibility Matrix

| Concern | K8s Enricher (Go) | Rego Policy Engine |
|---------|-------------------|-------------------|
| **Data Fetching** | ✅ Fetches K8s objects via client-go | ❌ Never fetches data |
| **Authentication** | ✅ Uses ServiceAccount + RBAC | ❌ No auth concerns |
| **Caching** | ✅ TTL cache for repeated lookups | ❌ Stateless evaluation |
| **Error Handling** | ✅ Handles K8s API errors gracefully | ❌ Only policy errors |
| **Rate Limiting** | ✅ Respects K8s API rate limits | ❌ Not applicable |
| **Retries** | ✅ Exponential backoff on failures | ❌ Not applicable |
| **Policy Evaluation** | ❌ No business logic | ✅ All classification rules |
| **Customer Configuration** | ❌ Fixed implementation | ✅ Customer-defined policies |

## Alternatives Considered

### Option B: Rego Direct API Access (REJECTED)

Rego/OPA CAN make HTTP calls via the `http.send` built-in function:

```rego
# REJECTED APPROACH - Direct K8s API access from Rego
package kubernaut.classification

namespace_data := http.send({
    "method": "GET",
    "url": sprintf("https://kubernetes.default.svc/api/v1/namespaces/%s", [input.signal.labels.namespace]),
    "headers": {"Authorization": concat(" ", ["Bearer", data.serviceAccountToken])},
    "tls_ca_cert": data.kubernetesCACert,
    "timeout": "5s"
})

environment := "production" {
    namespace_data.status_code == 200
    namespace_data.body.metadata.labels.env == "production"
}
```

#### Why This Was Rejected

| Issue | Impact | Severity |
|-------|--------|----------|
| **Security Risk** | Customer-provided Rego policies would have raw K8s API access. Malicious or buggy policies could read secrets, list all namespaces, or cause API server load. | **Critical** |
| **Auth Token Management** | ServiceAccount tokens must be embedded in policy input or stored in ConfigMaps accessible to OPA. Token rotation becomes complex. | **High** |
| **No client-go Features** | Lose caching, retries, rate limiting, watch/informer patterns, and all optimizations built into client-go. | **High** |
| **Blocking Calls** | `http.send` blocks policy evaluation. Slow K8s API responses directly impact signal processing latency. | **High** |
| **Error Handling Complexity** | HTTP errors in Rego return as response objects, requiring complex error checking in every policy. | **Medium** |
| **TLS Certificate Management** | Must manage Kubernetes CA certificates for in-cluster API calls. | **Medium** |
| **Testing Complexity** | Cannot easily mock K8s API responses in Rego policy tests. | **Medium** |
| **Performance Degradation** | N classifications × M signals = N×M K8s API calls vs M calls with enricher. | **High** |

### Option B Variant: OPA Bundle with K8s Data (REJECTED)

Another approach is to periodically sync K8s data to OPA as bundles:

```yaml
# REJECTED - OPA bundle approach
services:
  kubernetes-sync:
    url: http://k8s-sync-service:8080
bundles:
  kubernetes:
    service: kubernetes-sync
    resource: /kubernetes/data
    polling:
      min_delay_seconds: 30
```

#### Why This Was Rejected

| Issue | Impact |
|-------|--------|
| **Stale Data** | 30-second polling means classification uses outdated K8s state |
| **Additional Service** | Requires separate sync service to maintain |
| **Memory Overhead** | OPA must cache all relevant K8s objects in memory |
| **Complexity** | Bundle management adds operational complexity |

## Consequences

### Positive

1. **Security**: Customer Rego policies have no direct cluster access - they only see what we explicitly pass
2. **Performance**: Single K8s API call per signal, results reused across all classification policies
3. **Testability**: Easy to unit test Rego policies with mock input data (no HTTP mocking needed)
4. **Separation of Concerns**: Go handles infrastructure concerns, Rego handles business logic
5. **Caching**: K8s Enricher can cache namespace/node data across signals (TTL-based)
6. **Error Isolation**: K8s API errors handled in Go, Rego policies always receive valid input or skip enrichment
7. **Observability**: Clear metrics separation - K8s API latency vs policy evaluation time

### Negative

1. **Additional Component**: K8s Enricher is a separate concern to implement and maintain
2. **Data Freshness**: Cached data may be slightly stale (mitigated by short TTL)
3. **Schema Coupling**: Changes to K8s Enricher output require policy updates

### Mitigations

| Negative | Mitigation |
|----------|------------|
| Additional Component | K8s Enricher is simple, well-tested, and reusable across services |
| Data Freshness | 30-second TTL for namespace labels, 5-second TTL for pod status |
| Schema Coupling | Versioned schema with backward compatibility; policy input schema documented |

## Implementation

### K8s Enricher Interface

```go
package enricher

import (
    "context"

    "github.com/jordigilh/kubernaut/pkg/types"
)

// KubernetesEnricher fetches Kubernetes context for signal classification.
// This component handles all K8s API interactions, caching, and error handling.
// Rego policies receive the output as input - they never make K8s API calls.
type KubernetesEnricher interface {
    // EnrichSignal fetches Kubernetes context for the given signal.
    // Returns KubernetesContext with namespace, deployment, pod, and node data.
    // On K8s API errors, returns partial context with available data.
    EnrichSignal(ctx context.Context, signal *types.Signal) (*types.KubernetesContext, error)
}
```

### Rego Policy Input Schema

```rego
# Customer Rego policies receive this input structure
# They NEVER make http.send calls - all data is pre-fetched

package kubernaut.classification.environment

# Input structure (provided by K8s Enricher)
# input.signal - Original signal data
# input.kubernetesContext - Pre-fetched K8s data (may be partial on API errors)
# input.kubernetesContext.namespace - Namespace object with labels/annotations
# input.kubernetesContext.deployment - Deployment object (if applicable)
# input.kubernetesContext.pod - Pod object (if applicable)
# input.kubernetesContext.node - Node object (if applicable)

# Example: Environment classification based on namespace labels
default environment := "unknown"

environment := "production" {
    input.kubernetesContext.namespace.labels.env == "production"
}

environment := "production" {
    startswith(input.kubernetesContext.namespace.name, "prod-")
}

environment := "staging" {
    input.kubernetesContext.namespace.labels.env == "staging"
}

environment := "development" {
    input.kubernetesContext.namespace.labels.env in ["dev", "development"]
}
```

## Industry Precedent

This separation pattern is standard in policy-based systems:

| System | Data Fetching | Policy Evaluation |
|--------|--------------|-------------------|
| **OPA/Gatekeeper** | K8s Admission Webhook fetches resource | OPA evaluates policy on `input` |
| **Kyverno** | Controller fetches context | CEL/Rego evaluates rules |
| **Istio AuthZ** | Envoy fetches request metadata | OPA evaluates authorization |
| **AWS IAM** | AWS service fetches request context | Policy engine evaluates JSON policies |
| **HashiCorp Sentinel** | Terraform fetches plan data | Sentinel evaluates policies |

## Related Decisions

- [ADR-015: Alert to Signal Naming Migration](ADR-015-alert-to-signal-naming-migration.md) - Terminology standards
- [DD-CATEGORIZATION-001: Gateway/Signal Processing Split](DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md) - Categorization responsibility
- [DD-017: K8s Enrichment Depth Strategy](DD-017-k8s-enrichment-depth-strategy.md) - Signal-driven enrichment with standard depth

## References

- [OPA http.send Documentation](https://www.openpolicyagent.org/docs/latest/policy-reference/#http)
- [OPA Best Practices: External Data](https://www.openpolicyagent.org/docs/latest/external-data/)
- [Kubernetes client-go](https://github.com/kubernetes/client-go)

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-11-27 | AI Assistant | Initial ADR creation |

