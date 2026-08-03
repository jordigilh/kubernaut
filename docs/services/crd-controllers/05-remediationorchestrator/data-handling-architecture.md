## Data Handling Architecture: Targeting Data Pattern

### Overview

**Architectural Principle**: Remediation Coordinator provides targeting data only (~8KB). Kubernaut Agent (KA)
(formerly "HolmesGPT" — renamed in the Go rewrite, v1.3, issue [#433](https://github.com/jordigilh/kubernaut/issues/433))
fetches logs/metrics dynamically using built-in tools.

**Why This Matters**: Understanding this pattern ensures compliance with Kubernetes etcd limits and provides fresh data for KA investigations.

---

### Why CRDs Don't Store Logs/Metrics

**Kubernetes etcd Constraints**:
- **Typical limit**: 1.5MB per object
- **Recommended limit**: 1MB (safety margin)
- **Combined RemediationProcessing + AIAnalysis**: Must stay well below limits

**Data Freshness**:
- Logs stored in CRDs become stale immediately
- Kubernaut Agent (KA) needs real-time data for accurate investigations
- Kubernetes API provides fresh pod logs on demand

**Kubernaut Agent (KA) Design**:
- Built-in tools fetch data from live sources (see `pkg/kubernautagent/tools/` — `k8s`, `logs`, `prometheus`,
  `alertmanager`; full list in
  [AI Analysis Service — KA Toolsets](../02-aianalysis/ka-approval.md#ka-toolsets))
- `k8s`/`logs` tools: Pod logs, events, resource describe
- `prometheus` tool: Metrics queries, PromQL generation

---

### What Remediation Coordinator Provides to AIAnalysis

**Targeting Data** (~8-10KB total):

```yaml
spec:
  analysisRequest:
    alertContext:
      # Alert identification
      fingerprint: "abc123def456"
      severity: "critical"
      environment: "production"

      # Resource targeting for Kubernaut Agent (KA)
      namespace: "production-app"
      resourceKind: "Pod"
      resourceName: "web-app-789"

      # Kubernetes context (small metadata)
      kubernetesContext:
        clusterName: "prod-cluster-east"
        namespace: "production-app"
        resourceKind: "Pod"
        resourceName: "web-app-789"
        podDetails:
          name: "web-app-789"
          status: "CrashLoopBackOff"
          containerNames: ["app", "sidecar"]
          restartCount: 47
        deploymentDetails:
          name: "web-app"
          replicas: 3
        nodeDetails:
          name: "node-1"
          cpuCapacity: "16"
          memoryCapacity: "64Gi"
```

**What Is NOT Stored**:
- ❌ Pod logs (KA `k8s`/`logs` tools fetch)
- ❌ Metrics data (KA `prometheus` tool fetches)
- ❌ Events (KA fetches dynamically)
- ❌ kubectl describe output (KA generates on demand)

---

### How Kubernaut Agent (KA) Uses Targeting Data

**AIAnalysis Controller → Kubernaut Agent (KA) REST API** (via `pkg/agentclient`, OpenAPI-generated):

> **⚠️ CORRECTED (2026-08-02, [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806))**: The snippet
> below is illustrative pseudocode only — it does not reflect a real Go API. The actual integration is the
> AIAnalysis controller calling KA's REST API through the generated `pkg/agentclient` client; see
> `test/integration/aianalysis/agentclient_integration_test.go` for the real call pattern and
> [AI Analysis Service — KA Toolsets](../02-aianalysis/ka-approval.md#ka-toolsets) for KA's tool implementation.

```text
# Conceptual flow — KA uses targeting data to fetch fresh logs/metrics:
#   1. AIAnalysis controller calls KA's investigation API with namespace/resource_name (from SignalContext)
#   2. KA's built-in tools automatically:
#      - fetch pod logs (tail ~500 lines)
#      - describe the target resource
#      - list recent events in the namespace
#      - query Prometheus (e.g. container_memory_usage_bytes{pod="web-app-789"})
```

**Result**: Fresh, real-time data for investigation (not stale CRD snapshots)

---

### CRD-Level Validation (API Server Enforcement)

**Kubernetes API Server Validation**: All size and field constraints are enforced at the CRD schema level using OpenAPI v3 validation.

**SignalProcessing CRD** (where KubernetesContext originates):
```yaml
kubernetesContext:
  type: object
  x-kubernetes-validations:
  - rule: "self.size() <= 10240"  # 10KB CEL validation
    message: "kubernetesContext exceeds 10KB. Store targeting data only."
  properties:
    namespace:
      type: string
      maxLength: 63  # RFC 1123 DNS label
      description: "Target namespace for Kubernaut Agent (KA) investigation"
    resourceKind:
      type: string
      maxLength: 100  # Kubernetes resource kind max
      pattern: "^[A-Z][a-zA-Z0-9]*$"
      description: "Resource kind (Pod, Deployment, etc.)"
    resourceName:
      type: string
      maxLength: 253  # RFC 1123 DNS subdomain
      description: "Resource name for Kubernaut Agent (KA) targeting"
    labels:
      type: object
      maxProperties: 20
      additionalProperties:
        type: string
        maxLength: 63  # Label value max per Kubernetes
    annotations:
      type: object
      maxProperties: 10  # Limit to prevent bloat
      x-kubernetes-validations:
      - rule: "self.all(k, size(k) <= 253)"
        message: "Annotation keys must be 253 characters or less"
      - rule: "self.all(k, size(self[k]) <= 8192)"
        message: "Annotation values must be 8KB or less"
```

**AIAnalysis CRD** (where KubernetesContext is copied):
```yaml
kubernetesContext:
  type: object
  x-kubernetes-validations:
  - rule: "self.size() <= 10240"  # 10KB CEL validation
    message: "kubernetesContext exceeds 10KB. RemediationProcessing provided too much data."
  # Inherits same field constraints from SignalProcessing CRD
```

**Validation Flow**:
1. **RemediationProcessing Controller** → Tries to update `status.enrichmentResults.kubernetesContext`
2. **API Server** → Validates against SignalProcessing CRD schema
3. **If validation fails** → API server rejects update, returns error to controller
4. **If validation passes** → Written to etcd, Remediation Coordinator sees valid object

**Result**: Remediation Coordinator **NEVER sees invalid data** because API server blocks it.

---

### Error Handling (Not Validation)

**Remediation Coordinator does NOT validate** - it handles API server validation errors:

```go
// In RemediationRequestReconciler.createAIAnalysis()
func (r *RemediationRequestReconciler) createAIAnalysis(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    alertProcessing *processingv1.RemediationProcessing,
) error {

    // No validation needed - API server enforces CRD schema

    aiAnalysis := &aiv1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-analysis", remediation.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: aiv1.AIAnalysisSpec{
            RemediationRequestRef: aiv1.RemediationRequestReference{
                Name:      remediation.Name,
                Namespace: remediation.Namespace,
            },
            AnalysisRequest: aiv1.AnalysisRequest{
                AlertContext: aiv1.SignalContext{
                    // Data snapshot from RemediationProcessing (already validated by API server)
                    Fingerprint:       alertProcessing.Status.EnrichedSignal.Fingerprint,
                    Severity:          alertProcessing.Status.EnrichedSignal.Severity,
                    Environment:       alertProcessing.Status.EnrichedSignal.Environment,
                    BusinessPriority:  alertProcessing.Status.EnrichedSignal.BusinessPriority,

                    // Resource targeting for Kubernaut Agent (KA) tools
                    Namespace:    alertProcessing.Status.EnrichedSignal.Namespace,
                    ResourceKind: alertProcessing.Status.EnrichedSignal.ResourceKind,
                    ResourceName: alertProcessing.Status.EnrichedSignal.ResourceName,

                    // Kubernetes context (validated by API server: size <= 10KB)
                    KubernetesContext: alertProcessing.Status.EnrichedSignal.KubernetesContext,
                },
                AnalysisTypes: []string{"investigation", "root-cause", "recovery-analysis"},
                InvestigationScope: aiv1.InvestigationScope{
                    TimeWindow: "24h",
                    ResourceScope: []aiv1.ResourceScopeItem{
                        {
                            Kind:      alertProcessing.Status.EnrichedSignal.ResourceKind,
                            Namespace: alertProcessing.Status.EnrichedSignal.Namespace,
                            Name:      alertProcessing.Status.EnrichedSignal.ResourceName,
                        },
                    },
                    CorrelationDepth:          "detailed",
                    IncludeHistoricalPatterns: true,
                },
            },
        },
    }

    // Create AIAnalysis - API server validates against AIAnalysis CRD schema
    if err := r.Create(ctx, aiAnalysis); err != nil {
        if apierrors.IsInvalid(err) {
            // API server rejected due to CRD validation
            r.Log.Error(err, "AIAnalysis validation failed",
                "remediation", remediation.Name,
                "alertProcessing", alertProcessing.Name,
                "hint", "RemediationProcessing provided data that violates AIAnalysis CRD schema",
            )
        } else if apierrors.IsAlreadyExists(err) {
            // AIAnalysis already exists, this is fine (idempotency)
            r.Log.V(1).Info("AIAnalysis already exists (idempotent)",
                "remediation", remediation.Name,
                "aiAnalysis", aiAnalysis.Name,
            )
            return nil
        } else {
            // Other errors (network, RBAC, etc.)
            r.Log.Error(err, "Failed to create AIAnalysis",
                "remediation", remediation.Name,
            )
        }

        // Update RemediationRequest status to failed
        remediation.Status.OverallPhase = "failed"
        failureReason := fmt.Sprintf("Failed to create AIAnalysis: %v", err)
        remediation.Status.FailureReason = &failureReason

        if updateErr := r.Status().Update(ctx, remediation); updateErr != nil {
            return updateErr
        }

        return err
    }

    r.Log.Info("AIAnalysis created successfully",
        "remediation", remediation.Name,
        "aiAnalysis", aiAnalysis.Name,
    )

    return nil
}
```

**What This Catches**:
- ✅ Implementation bugs in RemediationProcessing (accidentally includes logs)
- ✅ Architectural violations (team forgets "targeting data only" pattern)
- ✅ Edge cases (pods with abnormally large annotations)

**Error Message Example** (from API server):
```
AIAnalysis.aianalysis.kubernaut.io "my-analysis" is invalid:
spec.analysisRequest.alertContext.kubernetesContext:
Invalid value: <object>: kubernetesContext exceeds 10KB.
RemediationProcessing provided too much data.
```

**Clear and actionable** - points to the problem (RemediationProcessing) without controller validation code.

---

### Size Budget Guidelines

| Component | Typical Size | Max (CRD Enforced) |
|-----------|-------------|--------------------|
| Alert fingerprint + metadata | ~500 bytes | N/A |
| Resource targeting (namespace, kind, name) | ~200 bytes | N/A |
| KubernetesContext (pod/deploy/node metadata) | 6-8KB | **10KB (API server enforced)** |
| Investigation scope | ~1KB | N/A |
| **Total AIAnalysis.spec** | **~8-10KB** | **~15KB** |

**Safety Margin**: Leaves >985KB for AIAnalysis.status (investigation results, recommendations)

---

### Field Length Constraints (Kubernetes Standards)

All field constraints match Kubernetes object specifications:

| Field | Max Length | Standard | Enforced By |
|-------|-----------|----------|-------------|
| `namespace` | 63 chars | RFC 1123 DNS label | CRD schema |
| `resourceKind` | 100 chars | Kubernetes resource kind | CRD schema |
| `resourceName` | 253 chars | RFC 1123 DNS subdomain | CRD schema |
| `label keys` | 253 chars | DNS subdomain | CRD schema |
| `label values` | 63 chars | RFC 1123 label | CRD schema |
| `annotation keys` | 253 chars | DNS subdomain | CRD schema |
| `annotation values` | 8KB each | Constrained for size | CRD CEL validation |

**Reference**: These constraints are verified in `internal/validation/validators.go` and `internal/testutil/assertions.go`.

---

### Reference

For detailed Kubernaut Agent (KA) tool capabilities and CRD schemas:
- [SignalProcessing CRD Schema](../../design/CRD/02_REMEDIATION_PROCESSING_CRD.md)
- [AIAnalysis CRD Schema](../02-aianalysis/crd-schema.md)
- [AI Analysis Service Spec - KA Toolsets](../02-aianalysis/ka-approval.md#ka-toolsets)

---

