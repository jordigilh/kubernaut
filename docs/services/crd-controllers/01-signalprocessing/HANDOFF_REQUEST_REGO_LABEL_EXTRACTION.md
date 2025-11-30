# Handoff Request: Label Detection and Extraction

**From**: AIAnalysis Service Team
**To**: SignalProcessing Service Team
**Date**: November 30, 2025
**Priority**: P1 (Required for workflow matching)
**Status**: 🔴 PENDING
**Scope**: V1.0 (DetectedLabels) + V1.1 (CustomLabels)

---

## Summary

SignalProcessing must provide two types of labels for workflow filtering:

| Label Type | Source | Config Required | V1.0 |
|------------|--------|-----------------|------|
| **DetectedLabels** | Auto-detection from K8s resources | ❌ No config | ✅ V1.0 |
| **CustomLabels** | Customer Rego policies | ✅ Requires config | V1.1 |

Both are passed to AIAnalysis → HolmesGPT-API → LLM prompt + MCP workflow filter.

---

## 📋 **Label Taxonomy Clarification (DD-WORKFLOW-001 v1.3)**

| Label Category | Source | Examples | Purpose |
|----------------|--------|----------|---------|
| **6 Mandatory Labels** (DD-WORKFLOW-001 v1.3) | Signal Processing core | `signal_type`, `severity`, `component`, `environment`, `priority`, `risk_tolerance` | Required for workflow matching |
| **DetectedLabels** (this handoff) | Auto-detection from K8s | `GitOpsManaged`, `PDBProtected`, `HPAEnabled`, `Stateful` | Additional context for LLM |
| **CustomLabels** (this handoff) | Rego policies | `team`, `region`, `business_category` | User-defined workflow filters |

**Key Point**: `DetectedLabels` and `CustomLabels` are **supplementary** to the 6 mandatory labels, not replacements.

---

## Part 1: DetectedLabels (V1.0 - PRIORITY)

### Overview

`DetectedLabels` are **automatically detected** from Kubernetes resources without any customer configuration.
These are deterministic facts about the cluster environment.

### DetectedLabels Struct (Already Added to Types)

```go
type DetectedLabels struct {
    // GitOps Management
    GitOpsManaged bool   `json:"gitOpsManaged"`
    GitOpsTool    string `json:"gitOpsTool,omitempty"` // "argocd", "flux", ""

    // Workload Protection
    PDBProtected bool `json:"pdbProtected"`
    HPAEnabled   bool `json:"hpaEnabled"`

    // Workload Characteristics
    Stateful    bool `json:"stateful"`
    HelmManaged bool `json:"helmManaged"`

    // Security Posture
    NetworkIsolated  bool   `json:"networkIsolated"`
    PodSecurityLevel string `json:"podSecurityLevel,omitempty"` // "privileged", "baseline", "restricted"
    ServiceMesh      string `json:"serviceMesh,omitempty"`      // "istio", "linkerd", ""
}
```

### Detection Logic

| Field | Detection Method | Confidence |
|-------|-----------------|------------|
| `GitOpsManaged` | ArgoCD annotation or Flux label present | 100% |
| `GitOpsTool` | Check specific annotations: `argocd.argoproj.io/instance` → "argocd", `flux.fluxcd.io/*` → "flux" | 100% |
| `PDBProtected` | Query PDBs in namespace matching workload labels | 100% |
| `HPAEnabled` | Query HPAs in namespace targeting workload | 100% |
| `Stateful` | Resource kind is StatefulSet OR has PVCs | 100% |
| `HelmManaged` | Label `app.kubernetes.io/managed-by: Helm` OR `helm.sh/chart` | 100% |
| `NetworkIsolated` | NetworkPolicy exists in namespace | 100% |
| `PodSecurityLevel` | Namespace label `pod-security.kubernetes.io/enforce` | 100% |
| `ServiceMesh` | Istio sidecar annotation OR Linkerd annotation present | 95% |

### Implementation

```go
// pkg/signalprocessing/detection/labels.go

func (d *Detector) DetectLabels(ctx context.Context, k8sCtx *v1alpha1.KubernetesContext) *v1alpha1.DetectedLabels {
    labels := &v1alpha1.DetectedLabels{}

    // GitOps detection
    labels.GitOpsManaged, labels.GitOpsTool = d.detectGitOps(k8sCtx)

    // Protection detection
    labels.PDBProtected = d.detectPDB(ctx, k8sCtx)
    labels.HPAEnabled = d.detectHPA(ctx, k8sCtx)

    // Workload type
    labels.Stateful = d.detectStateful(k8sCtx)
    labels.HelmManaged = d.detectHelm(k8sCtx)

    // Security
    labels.NetworkIsolated = d.detectNetworkPolicy(ctx, k8sCtx)
    labels.PodSecurityLevel = d.detectPodSecurityStandard(k8sCtx)
    labels.ServiceMesh = d.detectServiceMesh(k8sCtx)

    return labels
}

func (d *Detector) detectGitOps(k8sCtx *v1alpha1.KubernetesContext) (bool, string) {
    // Check deployment annotations
    if k8sCtx.DeploymentDetails != nil {
        // ArgoCD
        if _, ok := k8sCtx.DeploymentDetails.Annotations["argocd.argoproj.io/instance"]; ok {
            return true, "argocd"
        }
        // Flux
        if _, ok := k8sCtx.DeploymentDetails.Annotations["flux.fluxcd.io/sync-checksum"]; ok {
            return true, "flux"
        }
    }

    // Check namespace labels
    if k8sCtx.NamespaceLabels != nil {
        if _, ok := k8sCtx.NamespaceLabels["argocd.argoproj.io/managed-by"]; ok {
            return true, "argocd"
        }
    }

    return false, ""
}

func (d *Detector) detectPDB(ctx context.Context, k8sCtx *v1alpha1.KubernetesContext) bool {
    // List PDBs in namespace
    pdbList := &policyv1.PodDisruptionBudgetList{}
    if err := d.client.List(ctx, pdbList, client.InNamespace(k8sCtx.Namespace)); err != nil {
        return false
    }

    // Check if any PDB selector matches workload labels
    for _, pdb := range pdbList.Items {
        if selectorMatches(pdb.Spec.Selector, k8sCtx.PodDetails.Labels) {
            return true
        }
    }
    return false
}

func (d *Detector) detectHPA(ctx context.Context, k8sCtx *v1alpha1.KubernetesContext) bool {
    hpaList := &autoscalingv2.HorizontalPodAutoscalerList{}
    if err := d.client.List(ctx, hpaList, client.InNamespace(k8sCtx.Namespace)); err != nil {
        return false
    }

    for _, hpa := range hpaList.Items {
        if hpa.Spec.ScaleTargetRef.Name == k8sCtx.DeploymentDetails.Name {
            return true
        }
    }
    return false
}

func (d *Detector) detectStateful(k8sCtx *v1alpha1.KubernetesContext) bool {
    // Check if StatefulSet (would be in different context field)
    // OR check if PodDetails has PVC mounts
    // For now, check pod labels for StatefulSet pattern
    if k8sCtx.PodDetails != nil {
        if _, ok := k8sCtx.PodDetails.Labels["statefulset.kubernetes.io/pod-name"]; ok {
            return true
        }
    }
    return false
}

func (d *Detector) detectHelm(k8sCtx *v1alpha1.KubernetesContext) bool {
    if k8sCtx.DeploymentDetails != nil {
        if k8sCtx.DeploymentDetails.Labels["app.kubernetes.io/managed-by"] == "Helm" {
            return true
        }
        if _, ok := k8sCtx.DeploymentDetails.Labels["helm.sh/chart"]; ok {
            return true
        }
    }
    return false
}

func (d *Detector) detectNetworkPolicy(ctx context.Context, k8sCtx *v1alpha1.KubernetesContext) bool {
    npList := &networkingv1.NetworkPolicyList{}
    if err := d.client.List(ctx, npList, client.InNamespace(k8sCtx.Namespace)); err != nil {
        return false
    }
    return len(npList.Items) > 0
}

func (d *Detector) detectPodSecurityStandard(k8sCtx *v1alpha1.KubernetesContext) string {
    if k8sCtx.NamespaceLabels != nil {
        if level, ok := k8sCtx.NamespaceLabels["pod-security.kubernetes.io/enforce"]; ok {
            return level // "privileged", "baseline", "restricted"
        }
    }
    return ""
}

func (d *Detector) detectServiceMesh(k8sCtx *v1alpha1.KubernetesContext) string {
    if k8sCtx.PodDetails != nil {
        // Istio
        if _, ok := k8sCtx.PodDetails.Annotations["sidecar.istio.io/status"]; ok {
            return "istio"
        }
        // Linkerd
        if _, ok := k8sCtx.PodDetails.Annotations["linkerd.io/proxy-version"]; ok {
            return "linkerd"
        }
    }
    return ""
}
```

### Controller Integration

```go
// pkg/signalprocessing/controller/reconciler.go

func (r *Reconciler) enrichContext(ctx context.Context, sp *v1alpha1.SignalProcessing) error {
    // Step 1: K8s context enrichment
    k8sCtx, err := r.enrichKubernetesContext(ctx, sp)
    if err != nil {
        return err
    }

    // Step 2: Auto-detect labels (V1.0 - NO CONFIG NEEDED)
    detectedLabels := r.detector.DetectLabels(ctx, k8sCtx)

    // Step 3: Rego policy evaluation for custom labels (V1.1)
    var customLabels map[string]string
    if r.regoEngine != nil {
        customLabels, _ = r.evaluateRegoPolicy(ctx, k8sCtx)
    }

    // Populate results
    sp.Status.EnrichmentResults = v1alpha1.EnrichmentResults{
        KubernetesContext: k8sCtx,
        DetectedLabels:    detectedLabels,
        CustomLabels:      customLabels,
        EnrichmentQuality: calculateQuality(k8sCtx, detectedLabels, customLabels),
    }

    return nil
}
```

---

## Part 2: CustomLabels (V1.1)

## Business Justification

Per DD-WORKFLOW-004:
> **Signal Processing Service (Label Detection)**
> Responsibility: Detect all optional labels using Rego policies after K8s context enrichment.

Without extracted labels, HolmesGPT-API cannot filter workflows by customer-specific criteria (GitOps status, team ownership, region, etc.).

---

## Data Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         SignalProcessing Enrichment                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. Enrich Kubernetes Context                                               │
│     ├── Namespace labels (e.g., kubernaut.io/team: platform)                │
│     ├── Pod labels/annotations                                              │
│     └── Deployment labels/annotations                                        │
│                                                                              │
│  2. Evaluate Rego Policy (NEW)                                              │
│     ├── Input: KubernetesContext (namespace, pod, deployment labels)        │
│     ├── Policy: ConfigMap `signal-processing-policies` in kubernaut-system  │
│     └── Output: ExtractedLabels map[string]string                           │
│                                                                              │
│  3. Populate EnrichmentResults.ExtractedLabels                              │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              AIAnalysis                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│  Receives ExtractedLabels in spec.analysisRequest.signalContext             │
│  Passes to HolmesGPT-API for workflow filtering                             │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            HolmesGPT-API                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│  Uses ExtractedLabels in workflow catalog search query                      │
│  Filters by: 6 mandatory labels (DD-WORKFLOW-001 v1.3) + Custom Labels      │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Required Change

### 1. Field Already Added

`CustomLabels` has been added to `EnrichmentResults`:

```go
// api/signalprocessing/v1alpha1/signalprocessing_types.go
type EnrichmentResults struct {
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`

    // Custom labels for workflow catalog filtering (DD-WORKFLOW-001 v1.3)
    // These are user-defined labels extracted via Rego policies
    CustomLabels map[string]string `json:"customLabels,omitempty"`

    EnrichmentQuality float64 `json:"enrichmentQuality,omitempty"`
}
```

### 2. Implement Rego Policy Evaluation

**Rego Engine**: Use OPA/Rego (same as Gateway priority engine)

**Policy Source**: ConfigMap `signal-processing-policies` in `kubernaut-system`

**Input Schema**:
```json
{
  "namespace": {
    "name": "production",
    "labels": {
      "kubernaut.io/team": "platform-engineering",
      "kubernaut.io/region": "us-east-1",
      "environment": "production"
    }
  },
  "pod": {
    "name": "payment-api-7d8f9c6b5-xyz",
    "labels": { "app": "payment-api" },
    "annotations": { "argocd.argoproj.io/instance": "payment" }
  },
  "deployment": {
    "name": "payment-api",
    "labels": { "app": "payment-api" },
    "annotations": { "flux.fluxcd.io/sync-checksum": "abc123" }
  }
}
```

**Output Schema**:
```json
{
  "labels": {
    "team": "platform-engineering",
    "region": "us-east-1",
    "gitops-tool": "argocd"
  }
}
```

---

## Example Rego Policy (Customer-Defined)

```rego
# signal-processing-policies ConfigMap
# Customers define their own label extraction logic

package signalprocessing.labels

# Extract team from namespace label
labels["team"] = team {
    team := input.namespace.labels["kubernaut.io/team"]
    team != ""
}

# Extract region from namespace label
labels["region"] = region {
    region := input.namespace.labels["kubernaut.io/region"]
    region != ""
}

# Detect GitOps tool from deployment annotations
labels["gitops-tool"] = "argocd" {
    input.deployment.annotations["argocd.argoproj.io/instance"]
}

labels["gitops-tool"] = "flux" {
    input.deployment.annotations["flux.fluxcd.io/sync-checksum"]
}

# Detect cost tier from namespace label
labels["cost-tier"] = tier {
    tier := input.namespace.labels["kubernaut.io/cost-tier"]
    tier != ""
}
```

---

## Implementation

### Controller Changes

```go
// pkg/signalprocessing/controller/reconciler.go

func (r *Reconciler) enrichContext(ctx context.Context, sp *v1alpha1.SignalProcessing) error {
    // Step 1: K8s context enrichment
    k8sCtx, err := r.enrichKubernetesContext(ctx, sp)
    if err != nil {
        return err
    }

    // Step 2: Rego policy evaluation for custom labels
    customLabels, err := r.evaluateRegoPolicy(ctx, k8sCtx)
    if err != nil {
        // Log error but don't fail - labels are optional
        log.Error(err, "failed to evaluate rego policy for label extraction")
        customLabels = nil
    }

    // Populate results
    sp.Status.EnrichmentResults = v1alpha1.EnrichmentResults{
        KubernetesContext: k8sCtx,
        CustomLabels:      customLabels,
        EnrichmentQuality: calculateQuality(k8sCtx, customLabels),
    }

    return nil
}

func (r *Reconciler) evaluateRegoPolicy(ctx context.Context, k8sCtx *v1alpha1.KubernetesContext) (map[string]string, error) {
    // Build Rego input from K8s context
    input := map[string]interface{}{
        "namespace": map[string]interface{}{
            "name":   k8sCtx.Namespace,
            "labels": k8sCtx.NamespaceLabels,
        },
    }

    if k8sCtx.PodDetails != nil {
        input["pod"] = map[string]interface{}{
            "name":        k8sCtx.PodDetails.Name,
            "labels":      k8sCtx.PodDetails.Labels,
            "annotations": k8sCtx.PodDetails.Annotations,
        }
    }

    // Evaluate policy
    results, err := r.regoEngine.Evaluate(ctx, "data.signalprocessing.labels.labels", input)
    if err != nil {
        return nil, err
    }

    // Convert to map[string]string
    labels := make(map[string]string)
    if labelsMap, ok := results.(map[string]interface{}); ok {
        for k, v := range labelsMap {
            if strVal, ok := v.(string); ok {
                labels[k] = strVal
            }
        }
    }

    return labels, nil
}
```

### Rego Engine Setup

Follow the Gateway pattern (`pkg/gateway/processing/priority.go`):

```go
// pkg/signalprocessing/rego/engine.go

type RegoEngine struct {
    query rego.PreparedEvalQuery
}

func NewRegoEngine(policyPath string) (*RegoEngine, error) {
    policy, err := os.ReadFile(policyPath)
    if err != nil {
        return nil, err
    }

    query, err := rego.New(
        rego.Query("data.signalprocessing.labels.labels"),
        rego.Module("policy.rego", string(policy)),
    ).PrepareForEval(context.Background())
    if err != nil {
        return nil, err
    }

    return &RegoEngine{query: query}, nil
}

func (e *RegoEngine) Evaluate(ctx context.Context, input interface{}) (interface{}, error) {
    results, err := e.query.Eval(ctx, rego.EvalInput(input))
    if err != nil {
        return nil, err
    }

    if len(results) == 0 || len(results[0].Expressions) == 0 {
        return map[string]string{}, nil
    }

    return results[0].Expressions[0].Value, nil
}
```

---

## ConfigMap Deployment

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: signal-processing-policies
  namespace: kubernaut-system
data:
  labels.rego: |
    package signalprocessing.labels

    # Default: extract kubernaut.io/* labels from namespace
    labels[key] = value {
        some label_key
        value := input.namespace.labels[label_key]
        startswith(label_key, "kubernaut.io/")
        key := trim_prefix(label_key, "kubernaut.io/")
    }

    # Detect ArgoCD
    labels["gitops-tool"] = "argocd" {
        input.deployment.annotations["argocd.argoproj.io/instance"]
    }

    # Detect Flux
    labels["gitops-tool"] = "flux" {
        input.deployment.annotations["flux.fluxcd.io/sync-checksum"]
    }
```

---

## Testing Requirements

### Unit Tests

```go
var _ = Describe("Rego Label Extraction", func() {
    It("should extract team from namespace label", func() {
        input := map[string]interface{}{
            "namespace": map[string]interface{}{
                "labels": map[string]string{
                    "kubernaut.io/team": "platform",
                },
            },
        }
        labels, err := engine.Evaluate(ctx, input)
        Expect(err).ToNot(HaveOccurred())
        Expect(labels["team"]).To(Equal("platform"))
    })

    It("should detect ArgoCD from deployment annotation", func() {
        input := map[string]interface{}{
            "deployment": map[string]interface{}{
                "annotations": map[string]string{
                    "argocd.argoproj.io/instance": "my-app",
                },
            },
        }
        labels, err := engine.Evaluate(ctx, input)
        Expect(err).ToNot(HaveOccurred())
        Expect(labels["gitops-tool"]).To(Equal("argocd"))
    })

    It("should return empty map when no labels match", func() {
        input := map[string]interface{}{
            "namespace": map[string]interface{}{
                "labels": map[string]string{},
            },
        }
        labels, err := engine.Evaluate(ctx, input)
        Expect(err).ToNot(HaveOccurred())
        Expect(labels).To(BeEmpty())
    })
})
```

---

## Files to Modify

| File | Change | Priority |
|------|--------|----------|
| `api/signalprocessing/v1alpha1/signalprocessing_types.go` | ✅ DONE - `DetectedLabels` + `CustomLabels` added | ✅ Done |
| `pkg/signalprocessing/detection/labels.go` | NEW - Auto-detection logic | **V1.0** |
| `pkg/signalprocessing/controller/reconciler.go` | Add `DetectLabels()` call | **V1.0** |
| `pkg/signalprocessing/rego/engine.go` | NEW - Rego evaluation engine | V1.1 |
| `config/configmap/signal-processing-policies.yaml` | NEW - Default Rego policy | V1.1 |
| `deploy/charts/signalprocessing/templates/configmap.yaml` | Include policy ConfigMap | V1.1 |

---

## Verification Checklist

### V1.0 - DetectedLabels (PRIORITY)

- [ ] `DetectedLabels` populated in `EnrichmentResults`
- [ ] GitOps detection works (ArgoCD annotations, Flux labels)
- [ ] PDB detection queries PDBs in namespace
- [ ] HPA detection queries HPAs targeting workload
- [ ] Stateful detection (StatefulSet or PVC)
- [ ] Helm detection (managed-by label or helm.sh/chart)
- [ ] NetworkPolicy detection (any NP in namespace)
- [ ] Pod Security Standard detection (namespace label)
- [ ] Service mesh detection (Istio/Linkerd sidecar annotations)
- [ ] No errors when resources don't exist (graceful defaults)
- [ ] Unit tests pass

### V1.1 - CustomLabels

- [ ] `CustomLabels` populated in `EnrichmentResults`
- [ ] Rego engine initialized at controller startup
- [ ] Policy loaded from ConfigMap
- [ ] Default policy extracts `kubernaut.io/*` labels
- [ ] Empty map returned when no labels match (not nil)
- [ ] Policy evaluation failure doesn't block enrichment
- [ ] Unit tests pass

---

## Related Documents

| Document | Relevance |
|----------|-----------|
| [DD-WORKFLOW-004](../../../architecture/decisions/DD-WORKFLOW-004-hybrid-weighted-label-scoring.md) | Defines SignalProcessing responsibility |
| [Gateway Priority Engine](../../../pkg/gateway/processing/priority.go) | Reference Rego implementation |
| [ADR-041](../../../architecture/decisions/adr-041-llm-contract/) | Workflow filtering contract |

---

## Timeline

### V1.0 - DetectedLabels (PRIORITY)

| Milestone | Target |
|-----------|--------|
| Detection logic implementation | 2 days |
| K8s API queries (PDB, HPA, NP) | 1 day |
| Controller integration | 0.5 day |
| Testing | 1 day |
| **Total V1.0** | **4-5 days** |

### V1.1 - CustomLabels

| Milestone | Target |
|-----------|--------|
| Rego engine implementation | 2-3 days |
| Default policy + ConfigMap | 1 day |
| Integration with enrichment | 1 day |
| Testing | 1-2 days |
| **Total V1.1** | **5-7 days** |

---

**Contact**: AIAnalysis Service Team
**Questions**: Create issue or reach out on #kubernaut-dev

