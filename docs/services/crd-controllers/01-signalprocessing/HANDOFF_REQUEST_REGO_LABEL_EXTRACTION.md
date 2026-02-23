# Handoff Request: Label Detection and Extraction

**From**: AIAnalysis Service Team
**To**: SignalProcessing Service Team
**Date**: November 30, 2025
**Priority**: P1 (Required for workflow matching)
**Status**: 🔴 PENDING
**Scope**: V1.0 (DetectedLabels + CustomLabels)

---

## Document Version

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| **3.2** | Nov 30, 2025 | AIAnalysis Team | **NEW**: Added `OwnerChain` requirement (DD-WORKFLOW-001 v1.8) for DetectedLabels validation |
| 3.1 | Nov 30, 2025 | SignalProcessing Team | DD-WORKFLOW-001 v1.8 alignment: naming convention, risk_tolerance now customer-defined |
| 3.0 | Nov 30, 2025 | AIAnalysis Team | **MAJOR**: CustomLabels (Rego) pulled into V1.0, unified naming |
| 2.1 | Nov 30, 2025 | AIAnalysis Team | Added 7 Rego transformation examples |
| 2.0 | Nov 30, 2025 | AIAnalysis Team | Added DetectedLabels (V1.0), restructured document |
| 1.0 | Nov 29, 2025 | AIAnalysis Team | Initial Rego policy label extraction (CustomLabels) |

---

## 📢 Changelog (v3.0)

### ⚠️ SCOPE CHANGE

**CustomLabels (Rego) is now V1.0**, not V1.1.

Rationale: Customers need to define their own environment interpretation (e.g., what "uat" means for risk). DetectedLabels alone cannot provide this business context.

### ✨ Key Changes in v3.0

| Change | Description |
|--------|-------------|
| **Rego in V1.0** | CustomLabels pulled from V1.1 into V1.0 |
| **Unified naming** | `*.kubernaut.io/*` convention from V1.0 (no migration needed) |
| **Empty default policy** | No shipped policy - customers define their own |
| **Security via Rego** | Deny list enforced via Rego wrapper, not Go code |
| **Simplified examples** | Focus on risk-tolerance, team, region, constraints |
| **V1.0 input schema** | namespace, pod, deployment, node, signal, detected_labels |

### 📊 Updated Timeline

| Component | Estimate |
|-----------|----------|
| DetectedLabels (auto-detect) | 4-5 days |
| CustomLabels (Rego) | 3-4 days |
| Security wrapper + docs | 1 day |
| **Total V1.0** | **8-10 days** |

### 🏷️ Unified Label Naming (V1.0 = V1.1)

All labels use the same naming convention from V1.0 to avoid migration:

| Subdomain | Purpose | Example |
|-----------|---------|---------|
| `kubernaut.io/*` | Standard labels | `kubernaut.io/team`, `kubernaut.io/risk-tolerance` |
| `constraint.kubernaut.io/*` | Workflow constraints | `constraint.kubernaut.io/cost-constrained` |
| `custom.kubernaut.io/*` | Customer-defined | `custom.kubernaut.io/business-unit` |

### 📛 Naming Convention (DD-WORKFLOW-001 v1.8)

| Context | Convention | Example |
|---------|------------|---------|
| **K8s Labels/Annotations** | kebab-case | `kubernaut.io/signal-type`, `kubernaut.io/risk-tolerance` |
| **API Fields** | snake_case | `signal_type`, `risk_tolerance`, `custom_labels` |
| **DB Columns** | snake_case | `signal_type`, `severity`, `custom_labels` |
| **Rego Output Keys** | kebab-case (K8s label keys) | `"kubernaut.io/risk-tolerance"` |

**Key Points**:
- K8s labels use kebab-case (Kubernetes convention)
- API/DB fields use snake_case (REST/SQL convention)
- Rego policies output K8s label keys (kebab-case)
- SignalProcessing extracts subdomain, HolmesGPT-API passes to Data Storage

### 📋 DetectedLabels Wildcard Support (NEW)

Workflow blueprints can use wildcards for DetectedLabels:

```yaml
# Workflow matches ANY GitOps tool
detectedLabels:
  gitOpsTool: "*"  # Matches argocd, flux, helm, etc.
  gitOpsManaged: true

# Workflow for non-GitOps workloads
detectedLabels:
  gitOpsManaged: false  # Or omit field (same meaning)
```

**Boolean Normalization**: DetectedLabels booleans are only included when `true`, omitted when `false`.

---

## Summary

SignalProcessing must provide two types of labels for workflow filtering:

| Label Type | Source | Config Required | V1.0 |
|------------|--------|-----------------|------|
| **DetectedLabels** | Auto-detection from K8s resources | ❌ No config | ✅ V1.0 |
| **CustomLabels** | Customer Rego policies | ✅ Requires config | ✅ V1.0 |

Both are passed to AIAnalysis → HolmesGPT-API → LLM prompt + MCP workflow filter.

---

## 🏗️ **Core Design Principle: Abstraction Over Infrastructure**

> **📋 ACTION REQUIRED**: SignalProcessing team must create an ADR documenting this design principle.

### The Abstraction Layer

Rego policies define **abstract categories** based on characteristics, NOT per-namespace rules.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    ONE Rego Policy (Cluster-wide)                       │
│                                                                         │
│  Defines CATEGORIES based on characteristics, NOT per-namespace rules   │
│                                                                         │
│  Input: Namespace labels, pod/deployment details, detected labels       │
│  Output: Abstract categories (risk-tolerance, constraints)              │
└─────────────────────────────────────────────────────────────────────────┘
                              │
          ┌───────────────────┼───────────────────┐
          ▼                   ▼                   ▼
    ┌──────────┐        ┌──────────┐        ┌──────────┐
    │ ns: foo  │        │ ns: bar  │        │ ns: baz  │
    │ env=prod │        │ env=uat  │        │ env=dev  │
    └──────────┘        └──────────┘        └──────────┘
          │                   │                   │
          ▼                   ▼                   ▼
    risk=critical       risk=high            risk=low
```

### Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| **Single cluster-wide policy** | One source of truth for categorization logic |
| **Characteristic-based rules** | Policy operates on labels/characteristics, never hardcoded namespace names |
| **Auto-categorization** | New namespaces automatically categorized by existing rules |
| **Infrastructure agnostic** | Same policy works across clusters with consistent labeling |

### What This Means

```rego
# ✅ CORRECT: Characteristic-based (abstract)
labels["kubernaut.io/risk-tolerance"] = "critical" {
    input.namespace.labels["environment"] == "production"
}

# ❌ WRONG: Hardcoded namespace names (infrastructure-specific)
labels["kubernaut.io/risk-tolerance"] = "critical" {
    input.namespace.name == "payments-prod"
}
```

### Benefits

| Benefit | Description |
|---------|-------------|
| **Scalability** | Adding namespaces requires zero policy changes |
| **Maintainability** | Single policy to audit, update, version control |
| **Portability** | Policy works across environments with consistent labeling conventions |
| **Separation of concerns** | Platform team defines categories; app teams label namespaces |

### ADR Requirements

The SignalProcessing team ADR should document:

1. **Title**: "Characteristic-Based Label Categorization"
2. **Context**: Why abstraction over infrastructure
3. **Decision**: Single cluster-wide Rego policy, characteristic-based rules
4. **Consequences**: Auto-categorization, policy portability, labeling conventions required

---

## 📦 Dedicated Package Namespace: `signalprocessing.customlabels`

Custom label Rego policies use the `signalprocessing.customlabels` package, separate from the 5 mandatory label classifiers (environment, priority, severity, business, signal type) that each have their own Rego packages.

| Package | Purpose |
|---------|---------|
| `signalprocessing.environment` | Mandatory environment label classifier |
| `signalprocessing.priority` | Mandatory priority label classifier |
| `signalprocessing.severity` | Mandatory severity label classifier |
| `signalprocessing.business` | Mandatory business category classifier |
| **`signalprocessing.customlabels`** | **User-defined custom labels (this document)** |

**Rationale**: Keeping custom labels in a dedicated package (`customlabels`) rather than a generic `labels` package:

1. **Clarity**: Operators can immediately distinguish user-defined labels from the 5 system-managed mandatory labels
2. **Consistency**: Follows the same one-package-per-classifier pattern used by all other SP classifiers
3. **Safety**: The Rego engine queries `data.signalprocessing.customlabels.labels`, so policies in other packages cannot accidentally inject custom labels
4. **Discoverability**: All custom label policies live under a predictable, documented package name

**Engine query path**: `data.signalprocessing.customlabels.labels`

---

## 🎯 V1.0 Rego: Focused Scope

### V1.0 CustomLabels Examples (Documented)

| Label | Purpose | Use Case |
|-------|---------|----------|
| `kubernaut.io/risk-tolerance` | Customer defines environment meaning | "uat" → "high" vs "low" |
| `kubernaut.io/team` | Extract from namespace | Workflow filtering |
| `kubernaut.io/region` | Extract from namespace | Workflow filtering |
| `constraint.kubernaut.io/cost-constrained` | Budget-limited namespace | Prefer cost-efficient workflows |
| `constraint.kubernaut.io/high-availability` | HA-required workload | Maintain availability during remediation |

### V1.0 Default Policy

**No default policy shipped**. Customers must define their own. Documentation provides examples.

### V1.0 Input Schema

> **📋 Updated per Gateway Team Response** ([RESPONSE_GATEWAY_LABEL_PASSTHROUGH.md](RESPONSE_GATEWAY_LABEL_PASSTHROUGH.md))
> Gateway provides ALL alert labels/annotations via `SignalLabels` and `SignalAnnotations`.
> K8s resource labels (namespace.labels, pod.labels) are fetched by SignalProcessing.

```json
{
  "namespace": {
    "name": "payments-uat",
    "labels": { "environment": "uat", "kubernaut.io/team": "payments" },
    "annotations": {}
  },
  "pod": {
    "name": "api-server-xyz",
    "labels": { "app": "payment-api" },
    "annotations": {}
  },
  "deployment": {
    "name": "api-server",
    "labels": {},
    "annotations": {},
    "replicas": 3
  },
  "node": {
    "name": "node-1",
    "labels": {
      "topology.kubernetes.io/zone": "us-east-1a",
      "node.kubernetes.io/instance-type": "m5.xlarge"
    }
  },
  "signal": {
    "type": "OOMKilled",
    "severity": "critical",
    "source": "prometheus"
  },
  // NEW: Alert labels/annotations from Gateway (via RemediationRequest)
  "signal_labels": {
    "alertname": "HighMemoryUsage",
    "namespace": "payments-uat",
    "pod": "api-server-xyz",
    "container": "api",
    "severity": "critical",
    "cluster": "prod-us-west"
  },
  "signal_annotations": {
    "summary": "Container memory usage > 90%",
    "description": "Pod api-server-xyz memory at 94%",
    "runbook_url": "https://runbooks.company.com/oom"
  },
  "detected_labels": {
    // Only include boolean fields when TRUE (omit when false)
    "gitOpsManaged": true,
    "gitOpsTool": "argocd",
    "hpaEnabled": true
    // pdbProtected: omitted (false)
    // stateful: omitted (false)
    // String fields include if detected: podSecurityLevel, serviceMesh
  }
}
```

### Input Data Sources

| Field | Source | Notes |
|-------|--------|-------|
| `namespace.*` | K8s API query | SignalProcessing fetches |
| `pod.*` | K8s API query | SignalProcessing fetches |
| `deployment.*` | K8s API query | SignalProcessing fetches |
| `node.*` | K8s API query | SignalProcessing fetches |
| `signal.*` | RemediationRequest.Spec | Gateway provides |
| `signal_labels` | RemediationRequest.Spec.SignalLabels | Gateway provides (ALL alert labels) |
| `signal_annotations` | RemediationRequest.Spec.SignalAnnotations | Gateway provides (ALL alert annotations) |
| `detected_labels` | V1.0 DetectedLabels | SignalProcessing computes |
| `cluster` | **V1.1** | Deferred |

**V1.1**: Add `cluster` (name, region)

### DetectedLabels Convention: Only Include When True

**Boolean fields**: Only include in `detected_labels` when `true`. Omit when `false`.

```go
// In detection logic
if gitOpsManaged {
    detectedLabels["gitOpsManaged"] = true
    detectedLabels["gitOpsTool"] = gitOpsTool
}
// Don't add: detectedLabels["gitOpsManaged"] = false
```

**Rego usage**:
```rego
# Positive check: field exists and is true
labels["constraint.kubernaut.io/gitops-only"] = "true" {
    input.detected_labels.gitOpsManaged
}

# Negative check: field absent or false
labels["kubernaut.io/safe-to-restart"] = "true" {
    not input.detected_labels.stateful
    not input.detected_labels.pdbProtected
}
```

**String fields**: Include if detected (non-empty value).

---

## 🔐 Security: Rego Wrapper Policy

SignalProcessing enforces a security wrapper that strips system labels from customer policy output:

```rego
package signalprocessing.security

import data.signalprocessing.customlabels as customer_labels

# System labels that cannot be overridden by customer policies
# These are the 5 mandatory labels from DD-WORKFLOW-001 v1.8
# K8s label keys use kebab-case; API/DB fields use snake_case
system_labels := {
    "kubernaut.io/signal-type",   # API: signal_type
    "kubernaut.io/severity",      # API: severity
    "kubernaut.io/component",     # API: component
    "kubernaut.io/environment",   # API: environment
    "kubernaut.io/priority"       # API: priority
}
# NOTE: The following are ALLOWED (customer-defined via Rego, per DD-WORKFLOW-001 v1.8):
#   - kubernaut.io/risk-tolerance  → stored in custom_labels JSONB
#   - kubernaut.io/business-category → stored in custom_labels JSONB (optional)

# Final output: customer labels minus system labels
labels[key] = value {
    customer_labels.labels[key] = value
    not startswith_any(key, system_labels)
}

# Helper: check if key starts with any system label
startswith_any(key, prefixes) {
    some prefix
    prefixes[prefix]
    startswith(key, prefix)
}
```

**Implementation**: SignalProcessing loads customer policy, wraps with security policy, evaluates combined.

---

## 📝 V1.0 Customer Policy Example

```rego
package signalprocessing.customlabels

# ============================================
# RISK TOLERANCE
# Customers define what their environments mean
# ============================================

labels["kubernaut.io/risk-tolerance"] = "critical" {
    input.namespace.labels["environment"] == "production"
}

labels["kubernaut.io/risk-tolerance"] = "high" {
    input.namespace.labels["environment"] == "staging"
}

# ACME Corp: Our UAT is customer-facing demos, treat as high risk
labels["kubernaut.io/risk-tolerance"] = "high" {
    input.namespace.labels["environment"] == "uat"
}

labels["kubernaut.io/risk-tolerance"] = "low" {
    input.namespace.labels["environment"] == "dev"
}

# Fallback
labels["kubernaut.io/risk-tolerance"] = "medium" {
    not labels["kubernaut.io/risk-tolerance"]
}

# ============================================
# SIMPLE EXTRACTION
# ============================================

labels["kubernaut.io/team"] = team {
    team := input.namespace.labels["kubernaut.io/team"]
    team != ""
}

labels["kubernaut.io/region"] = region {
    region := input.namespace.labels["kubernaut.io/region"]
    region != ""
}

# ============================================
# CONSTRAINTS
# ============================================

# Cost-constrained: namespace has budget limits
labels["constraint.kubernaut.io/cost-constrained"] = "true" {
    input.namespace.labels["kubernaut.io/budget-tier"] == "limited"
}

# High-availability: workload requires HA-aware remediation
labels["constraint.kubernaut.io/high-availability"] = "true" {
    input.deployment.replicas >= 3
}

labels["constraint.kubernaut.io/high-availability"] = "true" {
    input.namespace.labels["kubernaut.io/sla-tier"] == "gold"
}
```

---

## 📋 **Label Taxonomy Clarification (DD-WORKFLOW-001 v1.8)**

| Label Category | Source | Examples | Purpose |
|----------------|--------|----------|---------|
| **5 Mandatory Labels** (DD-WORKFLOW-001 v1.8) | Signal Processing core | `signal_type`, `severity`, `component`, `environment`, `priority` | Required for workflow matching |
| **Customer-Derived Labels** (Rego) | Rego policies | `risk_tolerance`, `business_category`, `team`, `region` | Customer-defined via Rego |
| **DetectedLabels** (this handoff) | Auto-detection from K8s | `GitOpsManaged`, `PDBProtected`, `HPAEnabled`, `Stateful` | Additional context for LLM |

**Key Point**: `DetectedLabels` and `CustomLabels` are **supplementary** to the 5 mandatory labels, not replacements.

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
│  Filters by: 5 mandatory labels (DD-WORKFLOW-001) + Custom Labels           │
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

    // Custom labels for workflow catalog filtering (DD-WORKFLOW-001 v1.8)
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

---

# V1.1+ Advanced Examples (Reference Only)

The following examples demonstrate advanced Rego capabilities. These are **documented for reference** but not the focus of V1.0 implementation.

## Rego Capabilities: Beyond Simple Extraction

Rego is a **full policy language**, not just a label filter. It supports:

| Capability | Description | Use Case | V1.0? |
|------------|-------------|----------|-------|
| **Extraction** | Copy labels/annotations | `team` from namespace label | ✅ |
| **Transformation** | Derive new labels from multiple inputs | `business-owner` from namespace pattern | V1.1+ |
| **Lookup Tables** | Map values to other values | `cost-center` from team | V1.1+ |
| **Conditional Logic** | Complex business rules | `escalation-path` based on env + severity | V1.1+ |
| **Validation** | Constrain/sanitize values | Only allow known team names | V1.1+ |
| **External Data** | Load org structure from bundles | VP owner lookup | V2.0+ |

---

## Example 1: Simple Label Extraction (V1.0)

```rego
# signal-processing-policies ConfigMap
# Basic label copying

package signalprocessing.customlabels

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

## Example 2: Derive `business-owner` from Namespace (V1.1+ - Transformation)

```rego
package signalprocessing.customlabels

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

## Example 3: Derive `escalation-path` from Multiple Factors (V1.1+)

```rego
package signalprocessing.customlabels

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

## Example 4: Derive `cost-center` with Priority Chain (V1.1+)

```rego
package signalprocessing.customlabels

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

## Example 5: Derive `workflow-constraints` for Safety (V1.1+)

```rego
package signalprocessing.customlabels

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

## Example 6: Validation + Allowlist (V1.1+ - Security)

```rego
package signalprocessing.customlabels

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
    "kubernaut.io/severity"       # System-controlled
    # NOTE: risk_tolerance is now CUSTOMER-DEFINED (DD-WORKFLOW-001 v1.8)
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

## Example 7: External Data Lookup (V2.0+ - Advanced)

```rego
package signalprocessing.customlabels

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
  // "cluster": { ... } - DEFERRED TO V1.1
  "signal": {
    "type": "OOMKilled",
    "severity": "critical",
    "source": "prometheus"
  },
  "detected_labels": {
    // CONVENTION: Only include boolean fields when TRUE
    // Omit when false - Rego uses `not input.detected_labels.X` for negative checks
    "gitOpsManaged": true,
    "gitOpsTool": "argocd",
    "pdbProtected": true,
    "helmManaged": true,
    "networkIsolated": true,
    "podSecurityLevel": "restricted",
    "serviceMesh": "istio"
    // hpaEnabled: omitted (false)
    // stateful: omitted (false)
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
    results, err := r.regoEngine.Evaluate(ctx, "data.signalprocessing.customlabels.labels", input)
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
        rego.Query("data.signalprocessing.customlabels.labels"),
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
    package signalprocessing.customlabels

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
| `pkg/signalprocessing/rego/engine.go` | NEW - Rego evaluation engine | **V1.0** |
| `pkg/signalprocessing/rego/security.go` | NEW - Security wrapper policy | **V1.0** |
| `pkg/signalprocessing/controller/reconciler.go` | Add `DetectLabels()` + `EvaluateRego()` calls | **V1.0** |
| `config/configmap/signal-processing-policies.yaml` | Customer policy ConfigMap (empty default) | **V1.0** |
| `deploy/charts/signalprocessing/templates/configmap.yaml` | Include policy ConfigMap | **V1.0** |

---

## Verification Checklist

### V1.0 - DetectedLabels

- [ ] `DetectedLabels` populated in `EnrichmentResults`
- [ ] **Convention**: Boolean fields only included when `true` (omit when `false`)
- [ ] **Convention**: String fields included if non-empty
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

### V1.0 - CustomLabels (Rego)

- [ ] `CustomLabels` populated in `EnrichmentResults`
- [ ] Rego engine initialized at controller startup
- [ ] Customer policy loaded from ConfigMap
- [ ] Security wrapper strips system labels
- [ ] Empty map returned when no policy or no labels match (not nil)
- [ ] Policy evaluation failure doesn't block enrichment (log warning)
- [ ] Input schema includes: namespace, pod, deployment, node, signal, detected_labels
- [ ] Unit tests pass

### V1.0 - Documentation

- [ ] **ADR created**: "Characteristic-Based Label Categorization" (see Core Design Principle section)
- [ ] ADR documents: single cluster-wide policy, characteristic-based rules, auto-categorization

---

## Related Documents

| Document | Relevance |
|----------|-----------|
| [DD-WORKFLOW-004](../../../architecture/decisions/DD-WORKFLOW-004-hybrid-weighted-label-scoring.md) | Defines SignalProcessing responsibility |
| [Gateway Priority Engine](../../../pkg/gateway/processing/priority.go) | Reference Rego implementation |
| [ADR-041](../../../architecture/decisions/adr-041-llm-contract/) | Workflow filtering contract |

---

## Timeline

### V1.0 - Complete (DetectedLabels + CustomLabels)

| Milestone | Target |
|-----------|--------|
| DetectedLabels: Detection logic | 2 days |
| DetectedLabels: K8s API queries (PDB, HPA, NP) | 1 day |
| CustomLabels: Rego engine + security wrapper | 3 days |
| Controller integration (both) | 1 day |
| Testing | 1-2 days |
| **Total V1.0** | **8-10 days** |

### V1.1 - Extended Features

| Milestone | Target |
|-----------|--------|
| Add `cluster` to input schema | 0.5 day |
| Complex transformation examples | Documentation |
| Validation/allowlist examples | Documentation |

---

**Contact**: AIAnalysis Service Team
**Questions**: Create issue or reach out on #kubernaut-dev

---

## ❓ Questions from SignalProcessing Team - ANSWERED

**Date**: November 30, 2025
**Status**: ✅ ANSWERED

---

### Q1: Extended Input Schema - Data Availability

**Question**: Are `node`, `cluster`, and `signal` fields available?

**✅ ANSWER**:

| Field | V1.0 | Source |
|-------|------|--------|
| `namespace` | ✅ | `KubernetesContext` |
| `pod` | ✅ | `KubernetesContext.PodDetails` |
| `deployment` | ✅ | `KubernetesContext.DeploymentDetails` |
| `node` | ✅ | `KubernetesContext.NodeDetails` |
| `signal` | ✅ | `SignalProcessingSpec` fields |
| `detected_labels` | ✅ | V1.0 DetectedLabels output |
| `cluster` | ❌ V1.1 | Defer - derive from env var |

**Action**: Yes, build a `RegoInput` struct that combines these sources.

---

### Q2: Example 7 (External Data) - Infrastructure Scope

**Question**: Is external data (OPA bundles) V1.1 or V2.0+?

**✅ ANSWER**: **V2.0+**

External data lookup is deferred to V2.0. For V1.0/V1.1, use inline lookup tables in Rego:

```rego
# V1.0: Inline lookup (no external data)
team_owners := {
    "payments": "payments-team@company.com",
    "platform": "platform-sre@company.com",
}
```

---

### Q3: Example 5 (Workflow Constraints) - Downstream Consumption

**Question**: How are constraint labels consumed?

**✅ ANSWER**: Keep constraints in `CustomLabels` with naming convention.

| Stage | Consumption |
|-------|-------------|
| **Data Storage** | LLM passes labels in MCP call, Data Storage filters workflows |
| **LLM Prompt** | Natural language context for investigation |
| **Workflow Execution** | ❌ Not V1.0 |

**Naming convention**: `constraint.kubernaut.io/*` prefix identifies constraints.

---

### Q4: Example 6 (Security) - Intent Clarification

**Question**: Programmatic enforcement or documentation only?

**✅ ANSWER**: **Programmatic enforcement via Rego security wrapper**

SignalProcessing loads customer policy, wraps with security policy, evaluates combined. See "🔐 Security: Rego Wrapper Policy" section above.

The wrapper strips system labels from output - defense in depth.

---

### Q5: DetectedLabels as Rego Input - Ordering Dependency

**Question**: Is this ordering correct?

```
K8s Context → DetectedLabels (auto)
K8s Context + DetectedLabels → CustomLabels (Rego)
```

**✅ ANSWER**: **Confirmed**

DetectedLabels must be computed BEFORE Rego evaluation. Both are V1.0.

```go
func (r *Reconciler) enrichContext(ctx context.Context, sp *v1alpha1.SignalProcessing) error {
    // Step 1: K8s context
    k8sCtx := r.enrichKubernetesContext(ctx, sp)

    // Step 2: DetectedLabels (auto-detect)
    detectedLabels := r.detector.DetectLabels(ctx, k8sCtx)

    // Step 3: CustomLabels (Rego) - receives DetectedLabels as input
    customLabels := r.evaluateRegoPolicy(ctx, k8sCtx, sp.Spec, detectedLabels)

    // Step 4: Populate results
    sp.Status.EnrichmentResults = v1alpha1.EnrichmentResults{
        KubernetesContext: k8sCtx,
        DetectedLabels:    detectedLabels,
        CustomLabels:      customLabels,
    }
    return nil
}
```

---

### Question Summary - RESOLVED

| # | Question | Answer | Status |
|---|----------|--------|--------|
| Q1 | Input data availability | Build RegoInput struct, defer cluster to V1.1 | ✅ |
| Q2 | External data scope | V2.0+ (use inline tables for V1.0) | ✅ |
| Q3 | Constraints consumption | CustomLabels with `constraint.kubernaut.io/*` prefix | ✅ |
| Q4 | Security enforcement | Rego wrapper policy (programmatic) | ✅ |
| Q5 | Ordering dependency | Confirmed: DetectedLabels → then Rego | ✅ |

---

## ❓ Follow-up Questions from SignalProcessing Team (v3.0)

**Date**: November 30, 2025
**Status**: 🟡 AWAITING RESPONSE

---

### F1: `risk_tolerance` vs `risk-tolerance` Naming

**To**: AI Analysis Team
**Blocking**: ⚠️ Medium

The security wrapper shows:

```rego
system_labels := {
    "kubernaut.io/risk_tolerance"  # Note: risk-tolerance (with hyphen) IS allowed
}
```

**Current understanding**:

| Label | Status | Who sets it? |
|-------|--------|--------------|
| `kubernaut.io/risk-tolerance` (hyphen) | ALLOWED | Customer Rego (DD-WORKFLOW-001 v1.8) |

**Question**: Is this intentional dual-naming? The customer's Rego-derived `risk-tolerance` (hyphen) is supplementary to the system's `risk_tolerance` (underscore)?

**Concern**: This could confuse users. Which approach should we take?

| Option | Description |
|--------|-------------|
| **A** | Document clearly that these are different labels with different purposes |
| **B** | Rename customer label to `kubernaut.io/custom-risk-tolerance` to avoid confusion |
| **C** | Keep as-is with clear documentation |

---

**✅ ANSWER FROM AI ANALYSIS TEAM**:

Good catch! This was an error in the security wrapper example. Here's the clarification:

**The 5 Mandatory Labels** (system-controlled) are:
1. `signal_type`
2. `severity`
3. `component`
4. `environment`
5. `priority`

**Customer-Derived Labels** (via Rego):
- `kubernaut.io/risk-tolerance` - customer interprets environment risk
- `kubernaut.io/business-category` - optional, not all workloads have one (e.g., infra pods)

**Corrected Design**:

| Label | Source | Blocked? |
|-------|--------|----------|
| 5 mandatory labels | System (Gateway/SignalProcessing) | ✅ Blocked |
| `risk-tolerance` | Customer Rego | ❌ Allowed |
| `business-category` | Customer Rego (optional) | ❌ Allowed |

**Decision**: Use **consistent hyphen naming** for all CustomLabels (Kubernetes convention).

**Updated Security Wrapper**:

```rego
system_labels := {
    "kubernaut.io/signal_type",
    "kubernaut.io/severity",
    "kubernaut.io/component",
    "kubernaut.io/environment",
    "kubernaut.io/priority"
}
# ALLOWED (customer-defined):
#   - kubernaut.io/risk-tolerance
#   - kubernaut.io/business-category
```

**Action**: Update the security wrapper section in this document to reflect correct blocked labels.

---

### F2: Constraint Flow to Data Storage

**To**: Data Storage Team / Gateway Team
**Blocking**: ⚠️ Medium

The answer to Q3 states:
> **Data Storage**: LLM passes labels in MCP call, Data Storage filters workflows

**Current understanding of flow**:
```
SignalProcessing → AIAnalysis → HolmesGPT-API → MCP Tool Call → Data Storage API
                                      ↓
                               LLM decides which
                               constraint labels to
                               include in search
```

**Questions for Data Storage Team**:

1. Does the Data Storage workflow search API support filtering by `constraint.kubernaut.io/*` prefixed labels?
2. Are constraint labels stored as workflow metadata in Data Storage?
3. Does the search API support arbitrary label filtering (not just the 5 mandatory)?
4. Or does HolmesGPT-API handle constraint filtering locally after search results return?

**Impact**: Need to understand if constraints are:
- **A)** Pre-filter (Data Storage filters workflows by constraints before returning)
- **B)** Post-filter (HolmesGPT-API filters results after search)
- **C)** LLM context only (constraints inform LLM reasoning, not explicit filtering)

---

**✅ ANSWER FROM AI ANALYSIS TEAM**:

This question is correctly directed to **Data Storage team** for definitive answer. However, here's the intended design:

**Intended Flow**: **Option A (Pre-filter)** + **Option C (LLM context)**

1. **LLM receives** constraint labels as natural language context (for investigation reasoning)
2. **LLM includes** constraint labels in MCP `search_workflow_catalog` call
3. **Data Storage filters** workflows by constraints BEFORE returning results
4. **LLM selects** from pre-filtered results

**Data Storage Requirements** (to be confirmed by DS team):

| Requirement | Description |
|-------------|-------------|
| Arbitrary label filtering | Support filtering by any `*.kubernaut.io/*` label, not just 5 mandatory |
| Workflow metadata | Workflows must have constraint-compatible tags (e.g., `gitops_aware: true`) |
| Semantic matching | Match constraint labels to workflow capabilities |

**Example MCP Call**:
```json
{
  "query": "OOMKilled critical",
  "labels": {
    "kubernaut.io/risk-tolerance": "high",
    "constraint.kubernaut.io/cost-constrained": "true"
  }
}
```

**Data Storage should**:
- Return only workflows compatible with `cost-constrained=true`
- Exclude expensive remediation workflows

**Action**: Data Storage team to confirm API supports arbitrary label filtering.

---

### F3: Core Design Principle ADR

**To**: SignalProcessing Team (Self)
**Blocking**: 🟢 Low (Acknowledgment)

**Acknowledgment**: Will create ADR for "Characteristic-Based Label Categorization" covering:

1. Single cluster-wide policy (not per-namespace)
2. Characteristic-based rules (not hardcoded namespace names)
3. Auto-categorization of new namespaces
4. Policy portability across clusters

**No question** - confirming this is understood and will be created as part of V1.0.

---

### F4: DetectedLabels Convention Verification

**To**: AI Analysis Team
**Blocking**: 🟢 Low

The convention states:
> Boolean fields only included when `true`. Omit when `false`.

**Verification Question**: Is this the correct Go implementation?

```go
func buildDetectedLabelsForRego(dl *v1alpha1.DetectedLabels) map[string]interface{} {
    result := make(map[string]interface{})

    // Only include booleans when true
    if dl.GitOpsManaged {
        result["gitOpsManaged"] = true
        result["gitOpsTool"] = dl.GitOpsTool
    }
    if dl.PDBProtected {
        result["pdbProtected"] = true
    }
    if dl.HPAEnabled {
        result["hpaEnabled"] = true
    }
    if dl.Stateful {
        result["stateful"] = true
    }
    if dl.HelmManaged {
        result["helmManaged"] = true
    }
    if dl.NetworkIsolated {
        result["networkIsolated"] = true
    }

    // Always include non-empty strings
    if dl.PodSecurityLevel != "" {
        result["podSecurityLevel"] = dl.PodSecurityLevel
    }
    if dl.ServiceMesh != "" {
        result["serviceMesh"] = dl.ServiceMesh
    }

    return result
}
```

**Specific Question**: If `GitOpsManaged` is `false`, should `GitOpsTool` also be omitted?

Current implementation: Yes, both omitted together.

Please confirm this is correct behavior.

---

**✅ ANSWER FROM AI ANALYSIS TEAM**:

**Confirmed: Implementation is correct.**

| Condition | `gitOpsManaged` | `gitOpsTool` | Rationale |
|-----------|-----------------|--------------|-----------|
| GitOps detected | ✅ `true` | ✅ `"argocd"` | Both present - tool is meaningful |
| GitOps not detected | ❌ omitted | ❌ omitted | Both omitted - no tool to report |

**Why this is correct**:
1. **Data consistency**: `gitOpsTool` only makes sense when `gitOpsManaged` is true
2. **Rego simplicity**: Checking `input.detected_labels.gitOpsManaged` implicitly means tool is available
3. **Payload cleanliness**: No misleading empty strings

**Rego usage**:
```rego
# This works correctly with your implementation
labels["constraint.kubernaut.io/gitops-only"] = "true" {
    input.detected_labels.gitOpsManaged
    # gitOpsTool is guaranteed to exist here
}

labels["kubernaut.io/gitops-tool"] = tool {
    tool := input.detected_labels.gitOpsTool
    # Only evaluates when gitOpsManaged is true (and thus gitOpsTool exists)
}
```

**✅ Approved**: Your implementation is correct.

---

### Follow-up Summary

| # | Question | To Team | Blocking? | Status |
|---|----------|---------|-----------|--------|
| F1 | `risk_tolerance` vs `risk-tolerance` naming | AI Analysis | ⚠️ Medium | ✅ **Answered** - risk-tolerance is customer-defined, not system |
| F2 | Constraint flow to Data Storage | Data Storage | ⚠️ Medium | ✅ **Answered** - See [RESPONSE_CONSTRAINT_FILTERING.md](../../stateless/datastorage/RESPONSE_CONSTRAINT_FILTERING.md) |
| F2b | Gateway label passthrough | Gateway | 🟢 Low | ✅ **Answered** - See [RESPONSE_GATEWAY_LABEL_PASSTHROUGH.md](RESPONSE_GATEWAY_LABEL_PASSTHROUGH.md) |
| F3 | ADR creation acknowledgment | Self (SP Team) | 🟢 Low | ✅ Acknowledged |
| F4 | DetectedLabels convention verification | AI Analysis | 🟢 Low | ✅ **Answered** - Implementation is correct |

---

## ✅ Data Storage Team Response (F2) - Constraint Filtering

**Response Document**: [RESPONSE_CONSTRAINT_FILTERING.md](../../stateless/datastorage/RESPONSE_CONSTRAINT_FILTERING.md)

### Key Findings

| Question | Answer |
|----------|--------|
| **Q1**: Constraint label filtering | ✅ YES - JSONB supports any key; needs `custom_labels` filter parameter |
| **Q2**: Constraint labels stored | ✅ YES - JSONB column accepts any valid JSON object |
| **Q3**: Arbitrary label filtering | 🟡 PARTIAL - needs `custom_labels` filter enhancement (~2-4h) |
| **Q4**: Filtering strategy | ✅ Option A (Pre-filter) - GIN index already exists |

### SignalProcessing Answers to Data Storage Questions

| DS Question | SignalProcessing Answer |
|-------------|------------------------|
| **1. Validate `constraint.kubernaut.io/` prefix?** | ✅ YES - Validate prefix for security; reject arbitrary keys |
| **2. Boost scoring for constraints?** | ❌ NO - Hard filters only (presence/absence match) |
| **3. Absence handling?** | ✅ Option A - Absence = no constraint (workflow eligible) |

### Impact on SignalProcessing V1.0

- ✅ **Proceed**: Emit `constraint.kubernaut.io/*` labels in `CustomLabels`
- ✅ **Convention**: Constraints only present when `"true"`, absence = no constraint
- ⏳ **Dependency**: Data Storage adds `custom_labels` filter (~2-4h)

---

## ✅ Gateway Team Response (F2b) - Additional Detail

**Response Document**: [RESPONSE_GATEWAY_LABEL_PASSTHROUGH.md](RESPONSE_GATEWAY_LABEL_PASSTHROUGH.md)

### Key Findings

| Question | Answer |
|----------|--------|
| **Q1**: What labels does Gateway extract? | ✅ ALL alert labels + annotations passed through via `SignalLabels` and `SignalAnnotations` |
| **Q2**: Does Gateway pass annotations? | ✅ YES - all annotations preserved |
| **Q3**: Interface sufficient? | ✅ YES - SignalProcessing queries K8s API for resource labels |
| **Q4**: Plans for Gateway label extraction? | ❌ NO - per DD-CATEGORIZATION-001 (fast ingestion layer) |

### Impact on V1.0 Input Schema

Gateway provides **MORE** data than originally assumed. V1.0 input schema now includes:

| New Input Field | Source | Use Case |
|-----------------|--------|----------|
| `signal_labels` | `RemediationRequest.Spec.SignalLabels` | Alert labels (e.g., `cluster`, `container`) |
| `signal_annotations` | `RemediationRequest.Spec.SignalAnnotations` | Alert annotations (e.g., `runbook_url`, `summary`) |

### Optimization Opportunity

SignalProcessing can skip redundant queries:
- Pod/namespace **names** available from `SignalLabels` (no API call needed)
- Pod/namespace **labels** require K8s API query (for Rego input)


---

## 🔧 ADDENDUM v3.2: OwnerChain Schema Correction

**Date**: November 30, 2025
**Reference**: DD-WORKFLOW-001 v1.8

### ⚠️ Schema Correction Required

Your `crd-schema.md` v1.6 has an incorrect `OwnerReference` type. Please update to match DD-WORKFLOW-001 v1.8.

#### ❌ Current (Incorrect)

```go
// From crd-schema.md lines 228-239
type OwnerReference struct {
    APIVersion string `json:"apiVersion"`  // ❌ NOT NEEDED
    Kind string `json:"kind"`
    Name string `json:"name"`
    UID string `json:"uid,omitempty"`      // ❌ NOT NEEDED
    // ❌ MISSING: Namespace
}
```

#### ✅ Correct (DD-WORKFLOW-001 v1.8)

```go
// OwnerChainEntry represents a single entry in the K8s ownership chain
// HolmesGPT-API uses for DetectedLabels validation
type OwnerChainEntry struct {
    // Namespace of the owner resource
    // Empty for cluster-scoped resources (e.g., Node)
    // REQUIRED for namespaced resources
    Namespace string `json:"namespace,omitempty"`

    // Kind of the owner resource
    // Examples: "ReplicaSet", "Deployment", "StatefulSet", "DaemonSet"
    // REQUIRED
    Kind string `json:"kind"`

    // Name of the owner resource
    // REQUIRED
    Name string `json:"name"`
}
```

### Why This Matters

| Field | HolmesGPT-API Uses? | Why |
|-------|---------------------|-----|
| `namespace` | ✅ **YES** | Required for `same_namespace_and_kind()` validation |
| `kind` | ✅ **YES** | Required for `resources_match()` |
| `name` | ✅ **YES** | Required for `resources_match()` |
| `apiVersion` | ❌ **NO** | Not used in validation logic |
| `uid` | ❌ **NO** | Not used in validation logic |

### Validation Logic (from DD-WORKFLOW-001 v1.8)

```python
def resources_match(a, b):
    """Match by namespace + kind + name (NOT apiVersion or uid)"""
    return (a.namespace == b.namespace and
            a.kind == b.kind and
            a.name == b.name)
```

### Action Required

1. Update `crd-schema.md` type name: `OwnerReference` → `OwnerChainEntry`
2. Add `Namespace` field
3. Remove `APIVersion` field
4. Remove `UID` field
5. Update Go types in `api/signalprocessing/v1alpha1/signalprocessing_types.go`

### Traversal Algorithm

See DD-WORKFLOW-001 v1.8 for the complete Go pseudocode for traversing `ownerReferences`.

**Key points**:
- Use the first `controller: true` owner reference
- Inherit namespace from current resource (for namespaced owners)
- Stop when no more owners or owner not found

---
