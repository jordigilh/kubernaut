# Resource Scope Divergence: Production Scenarios

**Status**: Authoritative Reference
**Version**: 1.0
**Date**: 2026-02-07
**Cross-references**: [DD-HAPI-006](../decisions/DD-HAPI-006-affectedResource-in-rca.md), [BR-SCOPE-001](../../requirements/BR-SCOPE-001-resource-scope-management.md), [BR-SCOPE-010](../../requirements/BR-SCOPE-010-ro-routing-validation.md)
**Audience**: Internal (feeds into user-facing documentation)

---

## Overview

In Kubernaut, an **alert signal** fires on the resource where the symptom is observed (the **signal target**). The AI-driven Root Cause Analysis (RCA) may determine that the actual cause -- and therefore the correct remediation target -- is a **different resource** (the **RCA target**).

This divergence is expected and common. Kubernetes workloads form ownership hierarchies (Pod -> ReplicaSet -> Deployment) and dependency graphs (Pod -> ConfigMap, Service -> Deployment). Alerts typically fire at the leaf of these graphs (a crashing Pod, a latent Service), while the fix lives higher up or in a related resource.

### Why this matters

Remediating the signal target (the symptom) instead of the RCA target (the cause) is at best ineffective and at worst dangerous:

- **Ineffective**: Restarting a Pod that will crash again because its Deployment specifies insufficient memory
- **Dangerous**: Draining a Node when only one workload is misbehaving, causing unnecessary disruption to all co-located workloads

Kubernaut's architecture addresses this through the `affectedResource` field in the HAPI RCA response, extracted into `AIAnalysis.Status.RootCauseAnalysis.TargetResource` and used by the RemediationOrchestrator for scope validation and workflow execution (see [DD-HAPI-006](../decisions/DD-HAPI-006-affectedResource-in-rca.md)).

---

## Production Scenarios

### Scenario 1: Pod OOMKilled -- Deployment memory limits insufficient

| Field | Value |
|-------|-------|
| **Alert** | Kubernetes event: OOMKilled |
| **Signal target** | `Pod/payment-api-7f8b9c-xk2z` |
| **RCA target** | `Deployment/payment-api` |
| **Root cause** | Container `resources.limits.memory` set too low for actual workload demand |
| **Remediation** | Patch Deployment to increase memory limits |

**Why the targets diverge**: The Pod is ephemeral -- it was created by a ReplicaSet owned by the Deployment. Even if the Pod were restarted, the new Pod would inherit the same insufficient memory limit from the Deployment spec and crash again. The owner chain traversal (`Pod -> ReplicaSet -> Deployment`) guides the AI to the resource whose spec must change.

**Owner chain**:
```
Pod/payment-api-7f8b9c-xk2z
  └── ReplicaSet/payment-api-7f8b9c
        └── Deployment/payment-api  ← RCA target
```

---

### Scenario 2: Pod CrashLoopBackOff -- ConfigMap misconfiguration

| Field | Value |
|-------|-------|
| **Alert** | Kubernetes event: CrashLoopBackOff |
| **Signal target** | `Pod/order-service-5d9f4b-abc12` |
| **RCA target** | `ConfigMap/order-service-config` |
| **Root cause** | Invalid database connection string in the ConfigMap mounted by the Pod |
| **Remediation** | Patch ConfigMap to correct the connection string, then restart the Deployment |

**Why the targets diverge**: The Pod crashes at startup because it reads an invalid configuration value. Restarting the Pod or scaling the Deployment will not help -- every new Pod will read the same bad config and crash. The fix must target the ConfigMap that provides the configuration.

**Dependency graph**:
```
Pod/order-service-5d9f4b-abc12
  ├── ReplicaSet/order-service-5d9f4b
  │     └── Deployment/order-service
  └── mounts ConfigMap/order-service-config  ← RCA target
```

> **Note**: This scenario crosses the ownership boundary. The ConfigMap is not in the Pod's owner chain; it is a dependency discovered through volume mount analysis. The AI identifies this through Kubernetes context enrichment provided by the SignalProcessing controller.

---

### Scenario 3: Service 5xx spike -- HPA maxReplicas ceiling reached

| Field | Value |
|-------|-------|
| **Alert** | Prometheus: `http_requests_errors_total` rate > 5% for `catalog-api` |
| **Signal target** | `Service/catalog-api` |
| **RCA target** | `HorizontalPodAutoscaler/catalog-api` |
| **Root cause** | HPA `maxReplicas` already reached; existing Pods are saturated under load |
| **Remediation** | Increase `maxReplicas` on the HPA to allow further scaling |

**Why the targets diverge**: The alert fires on the Service-level error rate metric, which is the user-visible symptom. But the Service itself is functioning correctly -- it's routing traffic to all available Pods. The underlying issue is that the HPA has hit its scaling ceiling and cannot create additional Pods to handle the load. The fix targets the HPA, not the Service.

**Relationship graph**:
```
Service/catalog-api  ← Signal target (error rate alert)
  └── selects Pods managed by
        Deployment/catalog-api
          └── scaled by HorizontalPodAutoscaler/catalog-api  ← RCA target
```

---

### Scenario 4: Node memory pressure -- Resource-intensive Deployment

| Field | Value |
|-------|-------|
| **Alert** | Kubernetes condition: `MemoryPressure=True` on node |
| **Signal target** | `Node/worker-03` |
| **RCA target** | `Deployment/batch-processor` |
| **Root cause** | Memory leak in the batch-processor workload, consuming 80% of node memory |
| **Remediation** | Restart the batch-processor Deployment (to clear the leaked memory) and optionally apply resource limits |

**Why the targets diverge**: The Kubernetes event fires at the Node level -- the node is running low on memory. However, the cause is a single workload monopolizing resources. Draining the entire node would disrupt all co-located workloads unnecessarily. The AI correlates per-Pod resource consumption metrics with the node pressure event to identify the specific culprit.

**Relationship graph**:
```
Node/worker-03  ← Signal target (memory pressure)
  └── hosts Pod/batch-processor-8c7d2a-zz9p1 (using 80% node memory)
        └── ReplicaSet/batch-processor-8c7d2a
              └── Deployment/batch-processor  ← RCA target
```

> **Note**: This scenario involves a cluster-scoped resource (Node) as the signal target and a namespaced resource (Deployment) as the RCA target. Scope validation in the RemediationOrchestrator must handle this cross-scope transition.

---

### Scenario 5: Ingress latency spike -- Under-scaled backend Deployment

| Field | Value |
|-------|-------|
| **Alert** | Prometheus: `nginx_ingress_request_duration_seconds` p99 > 2s |
| **Signal target** | `Ingress/api-gateway` |
| **RCA target** | `Deployment/user-service` |
| **Root cause** | `user-service` has only 1 replica, saturated under load; all other backends are healthy |
| **Remediation** | Scale up the `user-service` Deployment or configure an HPA |

**Why the targets diverge**: The latency is observed at the Ingress, which fans out to multiple backend Services. The AI traces the request path -- correlating per-backend latency metrics -- and identifies that `user-service` is the bottleneck while other backends respond normally. Remediating the Ingress itself (e.g., adjusting timeouts) would mask the symptom without resolving the capacity issue.

**Relationship graph**:
```
Ingress/api-gateway  ← Signal target (latency alert)
  ├── routes /users/* → Service/user-service
  │                        └── Deployment/user-service  ← RCA target (1 replica, saturated)
  ├── routes /orders/* → Service/order-service (healthy)
  └── routes /catalog/* → Service/catalog-service (healthy)
```

---

## Patterns and Observations

### Common divergence patterns

| Pattern | Signal target | RCA target | Traversal |
|---------|--------------|------------|-----------|
| **Owner chain** | Pod | Deployment/StatefulSet | Up the ownership hierarchy |
| **Configuration dependency** | Pod | ConfigMap/Secret | Volume mount analysis |
| **Scaling constraint** | Service | HPA | Resource relationship discovery |
| **Node-to-workload** | Node | Deployment | Per-Pod resource correlation |
| **Ingress-to-backend** | Ingress | Deployment | Request path tracing |

### Escalation: when the RCA target cannot be determined

If the AI identifies a remediation workflow but cannot determine the `affectedResource`, Kubernaut does **not** fall back to the signal target. Instead, it sets `needs_human_review=true` with `human_review_reason=rca_incomplete` and creates a NotificationRequest for human investigation (per [DD-HAPI-006](../decisions/DD-HAPI-006-affectedResource-in-rca.md)).

**Rationale**: Remediating the symptom resource without confirming the root cause risks:
- Masking the real problem (allowing it to recur or worsen)
- Disrupting the wrong workload (if the signal target is a shared resource like a Node)
- Creating a false sense of resolution in the audit trail

### Scope validation

The RemediationOrchestrator validates the RCA target against the `kubernaut.ai/managed` label before proceeding with remediation (per [BR-SCOPE-010](../../requirements/BR-SCOPE-010-ro-routing-validation.md)). If the RCA target is not managed, the remediation is blocked and escalated.

---

## Revision History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-02-07 | Initial version with 5 production scenarios |
