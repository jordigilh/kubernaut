# Handoff Request: Label Detection and Extraction

**From**: AIAnalysis Service Team
**To**: SignalProcessing Service Team
**Date**: November 30, 2025
**Priority**: P1 (Required for workflow matching)
**Status**: 🔴 PENDING
**Scope**: V1.0 (DetectedLabels) + V1.1 (CustomLabels)

---

## Document Version

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| **2.1** | Nov 30, 2025 | AIAnalysis Team | Added 7 Rego transformation examples |
| 2.0 | Nov 30, 2025 | AIAnalysis Team | Added DetectedLabels (V1.0), restructured document |
| 1.0 | Nov 29, 2025 | AIAnalysis Team | Initial Rego policy label extraction (CustomLabels) |

---

## 📢 Changelog (v2.1)

### ✨ New in v2.1

| Addition | Description |
|----------|-------------|
| **Rego Capabilities Table** | Overview of extraction, transformation, validation |
| **Example 2: business-owner** | Derive from namespace patterns + lookup tables |
| **Example 3: escalation-path** | Multi-factor derivation (team + env + severity) |
| **Example 4: cost-center** | Priority chain with fallbacks |
| **Example 5: workflow-constraints** | Safety constraints (no-delete, max-replicas) |
| **Example 6: Validation** | Security allowlists and deny lists |
| **Example 7: External Data** | OPA bundle data for org structure lookup |
| **Extended Input Schema** | Full input including signal, cluster, detected_labels |

### 🎯 Key Message

**Rego is NOT just a filter** - it's a full transformation engine that can:
- Derive new labels from multiple inputs
- Use lookup tables for organizational mappings
- Apply conditional business logic
- Validate and constrain values for security
- Load external data for complex derivations

---

## 📢 Changelog (v2.0)

### ⚠️ BREAKING CHANGES

1. **Document Restructured** - Now split into Part 1 (V1.0) and Part 2 (V1.1)
2. **New V1.0 Requirement** - `DetectedLabels` auto-detection is now **V1.0 priority**
3. **CustomLabels Deferred** - Rego policy extraction moved to V1.1

### ✨ New in v2.0

| Addition | Description |
|----------|-------------|
| `DetectedLabels` struct | Strongly-typed, auto-detected cluster characteristics |
| Detection logic | Complete Go implementation for 9 label types |
| K8s API queries | PDB, HPA, NetworkPolicy detection via API |
| Kubebuilder validation | Enum validation for `GitOpsTool`, `PodSecurityLevel`, `ServiceMesh` |
| Label taxonomy table | Clarifies relationship to 6 mandatory labels |

### 🔄 What's Required for V1.0

**SignalProcessing must now implement**:
- `DetectedLabels` auto-detection (NO config required)
- K8s API queries for PDB, HPA, NetworkPolicy
- Integration with existing enrichment flow

**CustomLabels (Rego)** remains in scope for **V1.1** (unchanged from v1.0).

### 📊 Updated Timeline

| Scope | Estimate |
|-------|----------|
| **V1.0 DetectedLabels** | 4-5 days |
| V1.1 CustomLabels | 5-7 days |

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

## Rego Capabilities: Beyond Simple Extraction

Rego is a **full policy language**, not just a label filter. It supports:

| Capability | Description | Use Case |
|------------|-------------|----------|
| **Extraction** | Copy labels/annotations | `team` from namespace label |
| **Transformation** | Derive new labels from multiple inputs | `business-owner` from namespace pattern |
| **Lookup Tables** | Map values to other values | `cost-center` from team |
| **Conditional Logic** | Complex business rules | `escalation-path` based on env + severity |
| **Validation** | Constrain/sanitize values | Only allow known team names |
| **External Data** | Load org structure from bundles | VP owner lookup |

---

## Example 1: Simple Label Extraction

```rego
# signal-processing-policies ConfigMap
# Basic label copying

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

## Example 2: Derive `business-owner` from Namespace (Transformation)

```rego
package signalprocessing.labels

# Lookup table: namespace pattern → business owner
# Platform admins define this mapping
namespace_owners := {
    "payments-prod": "payments-team@company.com",
    "payments-staging": "payments-team@company.com",
    "checkout-prod": "commerce-team@company.com",
    "checkout-staging": "commerce-team@company.com",
    "platform-prod": "platform-sre@company.com",
}

# Prefix patterns for wildcard matching
prefix_owners := {
    "infra-": "infra-team@company.com",
    "data-": "data-platform@company.com",
    "ml-": "ml-engineering@company.com",
}

# Priority 1: Direct namespace match
labels["business-owner"] = owner {
    owner := namespace_owners[input.namespace.name]
}

# Priority 2: Prefix match (e.g., "infra-monitoring" → "infra-team@company.com")
labels["business-owner"] = owner {
    not namespace_owners[input.namespace.name]
    some prefix
    owner := prefix_owners[prefix]
    startswith(input.namespace.name, prefix)
}

# Priority 3: Derive from team label
labels["business-owner"] = owner {
    not labels["business-owner"]
    team := input.namespace.labels["kubernaut.io/team"]
    owner := sprintf("%s-team@company.com", [team])
}

# Priority 4: Default fallback
labels["business-owner"] = "platform-oncall@company.com" {
    not labels["business-owner"]
}
```

---

## Example 3: Derive `escalation-path` from Multiple Factors

```rego
package signalprocessing.labels

# Complex derivation: escalation path based on team + environment + severity
# This determines how alerts are routed

# Production + critical = PagerDuty page
labels["escalation-path"] = path {
    team := input.namespace.labels["kubernaut.io/team"]
    env := input.namespace.labels["environment"]
    severity := input.signal.severity

    env == "production"
    severity == "critical"
    path := sprintf("pagerduty:%s-prod-critical", [team])
}

# Production + warning = Slack channel
labels["escalation-path"] = path {
    team := input.namespace.labels["kubernaut.io/team"]
    env := input.namespace.labels["environment"]
    severity := input.signal.severity

    env == "production"
    severity == "warning"
    path := sprintf("slack:#%s-alerts", [team])
}

# Production + info = Just log
labels["escalation-path"] = path {
    team := input.namespace.labels["kubernaut.io/team"]
    env := input.namespace.labels["environment"]
    severity := input.signal.severity

    env == "production"
    severity == "info"
    path := sprintf("log:%s-info", [team])
}

# Non-production = Slack only
labels["escalation-path"] = path {
    team := input.namespace.labels["kubernaut.io/team"]
    env := input.namespace.labels["environment"]

    env != "production"
    path := sprintf("slack:#%s-staging", [team])
}
```

---

## Example 4: Derive `cost-center` with Priority Chain

```rego
package signalprocessing.labels

# Priority order for cost center derivation:
# 1. Explicit namespace label (highest priority)
# 2. Derive from team label
# 3. Derive from namespace naming convention
# 4. Default (lowest priority)

team_cost_centers := {
    "payments": "CC-1001-REVENUE",
    "checkout": "CC-1001-REVENUE",
    "platform": "CC-2001-INFRA",
    "data": "CC-3001-ANALYTICS",
    "ml": "CC-3001-ANALYTICS",
}

# Priority 1: Explicit label wins
labels["cost-center"] = cc {
    cc := input.namespace.labels["kubernaut.io/cost-center"]
    cc != ""
}

# Priority 2: Derive from team
labels["cost-center"] = cc {
    not input.namespace.labels["kubernaut.io/cost-center"]
    team := input.namespace.labels["kubernaut.io/team"]
    cc := team_cost_centers[team]
}

# Priority 3: Derive from namespace name pattern "<project>-<env>"
labels["cost-center"] = cc {
    not input.namespace.labels["kubernaut.io/cost-center"]
    not team_cost_centers[input.namespace.labels["kubernaut.io/team"]]

    parts := split(input.namespace.name, "-")
    count(parts) >= 2
    project := parts[0]
    cc := team_cost_centers[project]
}

# Priority 4: Default for unallocated
labels["cost-center"] = "CC-9999-UNALLOCATED" {
    not labels["cost-center"]
}
```

---

## Example 5: Derive `workflow-constraints` for Safety

```rego
package signalprocessing.labels

import future.keywords.in

# Derive workflow constraints that guide/restrict remediation
# These are passed to HolmesGPT to influence workflow selection

# Constraint: no-delete (immutable infrastructure)
labels["constraint-no-delete"] = "true" {
    input.namespace.labels["kubernaut.io/immutable"] == "true"
}

labels["constraint-no-delete"] = "true" {
    # GitOps managed = no direct deletes (use GitOps instead)
    input.detected_labels.gitOpsManaged == true
}

# Constraint: approval-required (high-risk namespaces)
labels["constraint-approval-required"] = "true" {
    input.namespace.labels["environment"] == "production"
    input.namespace.labels["kubernaut.io/tier"] in {"critical", "tier-1"}
}

# Constraint: max-replicas (prevent runaway scaling)
labels["constraint-max-replicas"] = replicas {
    tier := input.namespace.labels["kubernaut.io/tier"]
    tier == "critical"
    replicas := "50"
}

labels["constraint-max-replicas"] = replicas {
    tier := input.namespace.labels["kubernaut.io/tier"]
    tier != "critical"
    replicas := "20"
}

# Constraint: read-only (investigation only, no remediation)
labels["constraint-read-only"] = "true" {
    input.namespace.labels["kubernaut.io/mode"] == "observe"
}
```

---

## Example 6: Validation + Allowlist (Security)

```rego
package signalprocessing.labels

import future.keywords.in

# SECURITY: Only allow known team values
valid_teams := {"platform", "payments", "checkout", "data", "ml", "sre", "frontend", "backend"}

labels["team"] = team {
    team := input.namespace.labels["kubernaut.io/team"]
    team in valid_teams  # SECURITY: Reject unknown teams
}

# SECURITY: Validate region format
labels["region"] = region {
    region := input.namespace.labels["kubernaut.io/region"]
    regex.match(`^[a-z]{2}-[a-z]+-[0-9]$`, region)  # e.g., us-east-1
}

# SECURITY: Never extract these system labels
deny_labels := {
    "kubernaut.io/priority",      # System-controlled
    "kubernaut.io/severity",      # System-controlled
    "kubernaut.io/risk-tolerance" # System-controlled
}

# Allow custom- prefixed labels (user-safe)
labels[key] = value {
    some label_key
    value := input.namespace.labels[label_key]
    startswith(label_key, "kubernaut.io/custom-")
    not label_key in deny_labels
    key := trim_prefix(label_key, "kubernaut.io/custom-")
}
```

---

## Example 7: External Data Lookup (Advanced)

```rego
package signalprocessing.labels

# Load organizational structure from OPA bundle data
# data.org_structure is loaded from a JSON file

labels["business-unit"] = bu {
    team := input.namespace.labels["kubernaut.io/team"]
    bu := data.org_structure.teams[team].business_unit
}

labels["vp-owner"] = vp {
    team := input.namespace.labels["kubernaut.io/team"]
    vp := data.org_structure.teams[team].vp
}

labels["slack-channel"] = channel {
    team := input.namespace.labels["kubernaut.io/team"]
    channel := data.org_structure.teams[team].slack_channel
}

# org_structure.json (loaded as OPA bundle):
# {
#   "teams": {
#     "payments": {
#       "business_unit": "Commerce",
#       "vp": "Jane Smith",
#       "slack_channel": "#payments-alerts"
#     },
#     "platform": {
#       "business_unit": "Engineering",
#       "vp": "John Doe",
#       "slack_channel": "#platform-oncall"
#     }
#   }
# }
```

---

## Extended Input Schema

To enable powerful transformations, the Rego input includes:

```json
{
  "namespace": {
    "name": "payments-prod",
    "labels": {
      "kubernaut.io/team": "payments",
      "kubernaut.io/tier": "critical",
      "environment": "production"
    },
    "annotations": {
      "compliance.company.com/pci": "true"
    }
  },
  "pod": {
    "name": "api-server-7d8f9c6b5-xyz",
    "labels": { "app": "payment-api" },
    "annotations": {}
  },
  "deployment": {
    "name": "api-server",
    "replicas": 5,
    "labels": { "app": "payment-api" },
    "annotations": { "argocd.argoproj.io/instance": "payment" }
  },
  "node": {
    "name": "node-1",
    "labels": {
      "topology.kubernetes.io/zone": "us-east-1a",
      "node.kubernetes.io/instance-type": "m5.xlarge"
    }
  },
  "cluster": {
    "name": "prod-us-east-1",
    "region": "us-east-1"
  },
  "signal": {
    "type": "OOMKilled",
    "severity": "critical",
    "source": "prometheus"
  },
  "detected_labels": {
    "gitOpsManaged": true,
    "gitOpsTool": "argocd",
    "pdbProtected": true,
    "hpaEnabled": false,
    "stateful": false,
    "helmManaged": true,
    "networkIsolated": true,
    "podSecurityLevel": "restricted",
    "serviceMesh": "istio"
  }
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

---

## ❓ Questions from SignalProcessing Team (Pending AI Analysis Review)

**Date**: November 30, 2025
**Status**: 🟡 AWAITING RESPONSE

### Q1: Extended Input Schema - Data Availability

The new input schema (v2.1) includes:

```json
{
  "node": { "name": "...", "labels": {...} },
  "cluster": { "name": "...", "region": "..." },
  "signal": { "type": "...", "severity": "...", "source": "..." },
  "detected_labels": { "gitOpsManaged": true, ... }
}
```

**Question**: Are `node`, `cluster`, and `signal` fields currently available in `KubernetesContext`, or does SignalProcessing need to add new enrichment logic?

**Current State**:
- ✅ `NodeDetails` - partial (name, labels, capacity)
- ❓ `cluster` - **NOT** in current `KubernetesContext` struct
- ❓ `signal` - Available in `SignalProcessingSpec.Signal`, but not in `KubernetesContext`

**Proposal**: Should SignalProcessing build a new "RegoInput" struct that combines `KubernetesContext`, `Signal`, `DetectedLabels`, and a new `ClusterInfo`?

---

### Q2: Example 7 (External Data) - Infrastructure Scope

```rego
labels["vp-owner"] = vp {
    vp := data.org_structure.teams[team].vp
}
```

**Question**: OPA bundles require either:
- A bundle server
- File mounting in the controller pod

Is this expected in **V1.1**, or is it a **V2.0+** feature?

**Impact**: Significant increase in implementation complexity if V1.1.

**Proposal**: Mark external data lookup as V2.0+ and document it as an advanced use case for enterprises with existing OPA infrastructure.

---

### Q3: Example 5 (Workflow Constraints) - Downstream Consumption

The constraint labels are powerful:
- `constraint-no-delete: "true"`
- `constraint-approval-required: "true"`
- `constraint-max-replicas: "50"`
- `constraint-read-only: "true"`

**Question**: How are these consumed downstream?

| Option | Description |
|--------|-------------|
| **A** | HolmesGPT-API uses them to filter workflows (exclude workflows that delete if `constraint-no-delete=true`) |
| **B** | HolmesGPT-API passes them to the LLM prompt as context (LLM respects constraints) |
| **C** | Workflow Execution validates constraints before running |

**Impact**: This affects whether constraints should be `CustomLabels` or a separate field (e.g., `EnrichmentResults.WorkflowConstraints`).

---

### Q4: Example 6 (Security) - Intent Clarification

```rego
# SECURITY: Never extract these system labels
deny_labels := {
    "kubernaut.io/priority",      # System-controlled
    "kubernaut.io/severity",      # System-controlled
    "kubernaut.io/risk-tolerance" # System-controlled
}
```

**Question**: Is the intent to:
- **A)** Prevent users from accidentally overriding system-controlled mandatory labels via Rego?
- **B)** Document the separation between system labels (6 mandatory) and user labels (CustomLabels)?

**Clarification needed**: Should SignalProcessing enforce this deny list programmatically, or is it purely documentation for Rego policy authors?

---

### Q5: DetectedLabels as Rego Input - Ordering Dependency

The extended input schema shows `detected_labels` being passed to Rego.

**Question**: This implies CustomLabels (V1.1) Rego policies can reference DetectedLabels (V1.0) output:

```
V1.0: K8s Context → DetectedLabels (auto)
V1.1: K8s Context + DetectedLabels → CustomLabels (Rego)
```

**Confirmation needed**: Is this ordering correct? V1.1 depends on V1.0 being complete first.

---

### Question Summary

| # | Question | Impact | Blocking? |
|---|----------|--------|-----------|
| Q1 | Extended input data availability | Implementation scope | ⚠️ Medium |
| Q2 | External data (OPA bundles) scope | V1.1 vs V2.0 decision | 🔴 High |
| Q3 | Workflow constraints consumption | Architecture decision | ⚠️ Medium |
| Q4 | Security deny list enforcement | Implementation detail | 🟢 Low |
| Q5 | V1.0 → V1.1 ordering | Confirmation | 🟢 Low |

---

**Response requested by**: AI Analysis Team
**Deadline**: Before V1.1 implementation begins

