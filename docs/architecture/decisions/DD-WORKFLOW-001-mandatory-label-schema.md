# DD-WORKFLOW-001: Mandatory Workflow Label Schema

> **Note**: V1.0 uses label-only matching per DD-WORKFLOW-015. Semantic search references in this document are historical context for V1.1+.

**Date**: November 14, 2025
**Status**: ✅ **APPROVED** (V2.7 - Action-Type Primary Matching)
**Decision Maker**: Kubernaut Architecture Team
**Authority**: ⭐ **AUTHORITATIVE** - This document is the single source of truth for workflow label schema
**Affects**: Data Storage Service V1.0, Workflow Catalog, Signal Processing, HolmesGPT API
**Related**: DD-LLM-001 (MCP Search Taxonomy), DD-STORAGE-008 (Workflow Catalog Schema), ADR-041 (LLM Prompt Contract), DD-WORKFLOW-012 (Workflow Immutability), DD-WORKFLOW-004 v1.6 (Multi-Environment Queries), DD-WORKFLOW-016 (Action-Type Workflow Indexing)
**Version**: 2.7

---

## 🔗 **Workflow Immutability Reference**

**CRITICAL**: Workflow labels are immutable once created.

**Authority**: **DD-WORKFLOW-012: Workflow Immutability Constraints**
- Labels are immutable (cannot change after workflow creation)
- Labels are used for semantic search embeddings
- To change labels, create a new workflow version

**Cross-Reference**: All label schema definitions in this DD are subject to DD-WORKFLOW-012 immutability constraints.

---

---

## 📋 **Status**

**✅ APPROVED** (2025-11-14)
**Last Reviewed**: 2025-11-16
**Confidence**: 95%

---

## 📝 **Changelog**

### Version 2.7 (2026-02-15) **← CURRENT**
**BREAKING**: Severity changed to always-array (like environment)

**Changes**:
- **BREAKING**: `severity` in MandatoryLabels changed from `string` to `[]string`
  - Before: `severity: "critical"` (single string with `"*"` wildcard)
  - After: `severity: ["critical"]` or `["critical", "high"]` (array, no wildcard)
- **REMOVED**: `"*"` wildcard for severity. To match any severity, list all four levels: `["critical", "high", "medium", "low"]`
- **SQL Pattern**: `labels->'severity' ? $N` (JSONB containment, same as environment)

**Rationale**: Aligns severity with environment (both now arrays). A single workflow can handle multiple severity levels without needing a wildcard. Pre-release only, no backwards compatibility required.

**Breaking Impact**: Pre-release only. All workflow schemas must use array format for severity.

### Version 2.6 (2026-02-05)
**BREAKING**: Action-type primary matching (DD-WORKFLOW-016)

**Changes**:
- **NEW**: `action_type` added as **mandatory label** (Group C: Workflow-Defined)
  - Enforced taxonomy (10 types): `ScaleReplicas`, `RestartPod`, `RestartDeployment`, `IncreaseCPULimits`, `IncreaseMemoryLimits`, `RollbackDeployment`, `DrainNode`, `CordonNode`, `CleanupNode`, `DeletePod`
  - Validated against DD-WORKFLOW-016 authoritative taxonomy
  - Becomes the **primary matching key** for workflow catalog search (replaces `signal_type`)
- **CHANGED**: `signal_type` demoted from **mandatory primary key** to **optional metadata**
  - No longer used as the primary catalog search filter
  - Retained on workflow entries as documentation/metadata
  - SP continues to populate `signal_type` for signal classification (ADR-054)
- **NEW**: `ListAvailableActions` context-aware HAPI tool filters available action types by severity/component/environment
- **NEW**: LLM two-step workflow discovery protocol (list actions -> RCA -> search catalog)

**Rationale**: Different source adapters (Prometheus, K8s Events) produce incompatible signal type vocabularies. The same signal can require different remediations depending on root cause. Action-type indexing decouples workflows from source adapter vocabularies and leverages the LLM's RCA to select the appropriate remediation action.

**Authority**: DD-WORKFLOW-016 (Action-Type Workflow Catalog Indexing)

**Breaking Impact**: Pre-release only. Workflow registration now requires `action_type` from the enforced taxonomy. Catalog search primary key changes from `signal_type` to `action_type`.

### Version 2.5 (2026-01-28) **[SUPERSEDED by v2.6]**
**FEATURE**: Multi-environment workflow capability (workflows declare target environments)

**Breaking Changes**:
- **Storage Model**: `MandatoryLabels.environment` changed from `string` to `[]string`
  - Before: `environment: "production"` (single string)
  - After: `environment: ["production"]` or `["staging", "production"]` (array)
- **Search Model**: `WorkflowSearchFilters.environment` **REVERTED** to `string` (from v2.4's `[]string`)
  - Signal Processing sends single value: `"production"`
  - HAPI passes through single value: `"production"`
  - DataStorage searches workflows where array contains value

**SQL Pattern**:
```sql
-- Before (v2.4): Search multiple environments (INCORRECT)
WHERE labels->>'environment' IN ('staging', 'production')

-- After (v2.5): Workflow declares multiple environments, search single (CORRECT)
WHERE labels->'environment' ? 'production' OR labels->'environment' ? '*'
```

**Use Cases**:
- **Workflow Creation**: DevOps creates workflow: `environment: ["staging", "production"]`
- **Signal from Production**: SP extracts `"production"` → HAPI searches `"production"` → finds workflows with `["staging", "production"]` or `["production"]`
- **Signal from Staging**: SP extracts `"staging"` → HAPI searches `"staging"` → finds workflows with `["staging", "production"]` or `["staging"]`
- **Universal Workflow**: `environment: ["*"]` matches ANY environment search

**Validation**: `validate:"required,min=1"` (explicit declaration required, no default)

**Rationale**: Single workflow can be reused across multiple environments without duplication. Workflows explicitly declare their scope, Signal Processing always sends single environment.

**Breaking Impact**: Pre-release only, no backwards compatibility required

**Authority**: BR-STORAGE-040 v2.0, DD-WORKFLOW-004 v2.0

### Version 2.4 (2026-01-26) **[SUPERSEDED by v2.5]**
- **REVERTED**: Incorrect implementation - implemented search-side arrays instead of storage-side arrays
- **ISSUE**: Allowed HAPI to search multiple environments, but workflows were still single-environment
- **CORRECTION**: v2.5 implements correct model (storage-side arrays, search-side single value)

### Version 2.3 (2025-12-06)
- **NEW**: Added **Detailed Detection Methods** section documenting specific annotations/labels for each detection type
- **CLARIFICATION**: `stateful` detection uses owner chain (no K8s API call), not direct StatefulSet lookup
- **CLARIFICATION**: `serviceMesh` detection uses pod annotations:
  - Istio: `sidecar.istio.io/status` (present after sidecar injection)
  - Linkerd: `linkerd.io/proxy-version` (present after proxy injection)
- **DOCUMENTATION**: Each detection field now has documented source, specific check, and API call requirements
- **RATIONALE**: Standardized detection methods for consistent implementation across services

### Version 2.2 (2025-12-03)
- **REMOVED**: `podSecurityLevel` field from DetectedLabels
- **RATIONALE**: PodSecurityPolicy (PSP) is deprecated since K8s 1.21, removed in 1.25. Pod Security Standards (PSS) are namespace-level, not pod-level, making detection inconsistent and unreliable.
- **IMPACT**: Field count reduced from 9 to 8 detected labels
- **MIGRATION**: Services should remove any references to `podSecurityLevel` in DetectedLabels
- **NOTIFICATION**: See `docs/handoff/NOTICE_PODSECURITYLEVEL_REMOVED.md`

### Version 2.1 (2025-12-02)
- **NEW**: Added **Detection Failure Handling** section documenting the December 2025 inter-team agreement
- **DESIGN**: Plain `bool` fields + `FailedDetections []string` array (avoids `*bool` anti-pattern)
- **VALIDATION**: Added `go-playground/validator` enum validation for `FailedDetections`
- **SCHEMA**: `FailedDetections` only accepts known field names (validated enum)
- **AUTHORITATIVE**: Go type definition with validation tags

### Version 2.0 (2025-12-02)
- **NEW**: Added **DetectedLabels End-to-End Architecture** section clarifying:
  - **Incident DetectedLabels** (SignalProcessing auto-detects from live K8s) vs
  - **Workflow Catalog detected_labels** (workflow author-defined metadata)
- **CLARIFICATION**: Data Storage does NOT auto-populate workflow detected_labels (they are author-defined)
- **NEW**: End-to-end flow diagram showing label flow from SignalProcessing → HolmesGPT-API → Data Storage
- **CLARIFICATION**: SignalProcessing V1.0 ✅ IMPLEMENTED for incident DetectedLabels auto-detection

### Version 1.9 (2025-12-02)
- **NEW**: Added **CustomLabels Validation Limits** section (max 10 keys, 5 values/key, 63 char keys, 100 char values)
- **NEW**: Added **Security Measures** section for Sandboxed OPA Runtime (no network, no filesystem, 5s timeout, 128MB memory)
- **NEW**: Added **Mandatory Label Protection** via Rego security wrapper (blocks 5 system labels)
- **FORMALIZED**: Validation limits previously discussed in handoff documents now authoritative

### Version 1.8 (2025-11-30)
- **NEW**: Added **authoritative `OwnerChainEntry` Go schema** with explicit field definitions
- **CLARIFICATION**: `OwnerChainEntry` requires ONLY: `namespace`, `kind`, `name`
- **CLARIFICATION**: Do NOT include `apiVersion` or `uid` - not used by HolmesGPT-API validation
- **NEW**: Added **traversal algorithm** (Go pseudocode) for SignalProcessing implementation
- **FIX**: Corrected SignalProcessing spec misinterpretation (was using K8s native ownerReference format)

### Version 1.7 (2025-11-30)
- **NEW**: 100% Safe DetectedLabels validation using owner chain from SignalProcessing
- **NEW**: `ownerChain` field in EnrichmentResults (K8s ownerReferences traversal)
- **NEW**: `rca_resource` required parameter in `search_workflow_catalog` tool
- **NEW**: Dual-use DetectedLabels architecture:
  - **LLM Prompt Context**: ALWAYS included (helps LLM understand environment)
  - **Workflow Filtering**: CONDITIONAL (only when relationship is PROVEN)
- **PRINCIPLE**: Default to EXCLUDE if relationship cannot be proven (100% safe)
- **PRINCIPLE**: Owner chain enables deterministic validation (no heuristics)
- Pod ↔ Deployment/StatefulSet/DaemonSet ownership relationships supported
- ReplicaSet ↔ Deployment ownership relationships supported
- Cluster-scoped (Node) vs namespaced resource validation
- Same namespace + same kind fallback when owner_chain provided but empty

### Version 1.6 (2025-11-30)
- **BREAKING**: Standardized all API/database field names to **snake_case**
- Changed filter parameters: `signal-type` → `signal_type`, `risk-tolerance` → `risk_tolerance`
- **Clarification**: Kubernetes annotation keys (`kubernaut.io/signal-type`) vs API field names (`signal_type`)
- K8s annotations use kebab-case per K8s convention; API/DB use snake_case per JSON convention
- Updated Implementation section to align with v1.4 (5 mandatory labels, not 7)
- Removed `risk_tolerance` and `business_category` from mandatory (now customer-derived via Rego)
- Updated Go struct JSON tags from `json:"kubernaut.io/signal-type"` to `json:"signal_type"`
- **NEW**: Added authoritative **DetectedLabels** section (8 auto-detected fields)
- **NEW**: Added **Wildcard Support for DetectedLabels** string fields (`gitOpsTool`, `serviceMesh`)
- **NEW**: Documented matching semantics: `"*"` = "requires SOME value", *(absent)* = "no requirement"
- **NEW**: Added complete examples showing all three label types (mandatory + detected + custom)
- **Documented**: Boolean Normalization Rule - booleans only included when `true`, omitted when `false`
- **Impact**: Data Storage must update Go struct JSON tags to snake_case and implement DetectedLabels wildcard matching
- **Cross-reference**: DD-WORKFLOW-002 v3.3, DD-HAPI-001, DD-WORKFLOW-004 v2.2

### Version 1.5 (2025-11-30)
- **Custom Labels**: Subdomain-based extraction design finalized
- **Format**: `<subdomain>.kubernaut.io/<key>[:<value>]` → `map[string][]string`
- **Pass-Through**: Kubernaut is a conduit, not transformer (labels flow unchanged)
- **Boolean Normalization**: Empty/true → key only; false → omitted
- **Industry Alignment**: Follows Kubernetes label propagation pattern
- **Reference**: HANDOFF_CUSTOM_LABELS_EXTRACTION_V1.md

### Version 1.4 (2025-11-30)
- 5 mandatory labels: `signal_type`, `severity`, `component`, `environment`, `priority`
- Customer-derived labels via Rego: `risk_tolerance`, `business_category`, `team`, `region`, etc.
- Rationale: Customers define environment meaning for risk (e.g., "uat" = high risk for one team, low for another)

---

## ⭐ **AUTHORITATIVE LABEL DEFINITIONS**

**This document is the single source of truth for workflow label schema.** All services MUST reference this document for label definitions.

### **Naming Convention Clarification (v1.6)**

There are TWO different naming contexts - do NOT confuse them:

| Context | Convention | Example | Used In |
|---------|------------|---------|---------|
| **Kubernetes annotations/labels** | kebab-case with prefix | `kubernaut.io/signal-type` | CRD metadata, K8s resources |
| **API/Database field names** | snake_case | `signal_type` | REST APIs, Go structs, SQL columns |

**Example**:
```yaml
# Kubernetes CRD annotation (kebab-case)
metadata:
  annotations:
    kubernaut.io/signal-type: "OOMKilled"

# API request body (snake_case)
{
  "filters": {
    "signal_type": "OOMKilled"
  }
}
```

**Rule**: When writing API code, always use `snake_case`. The K8s annotation format is only for CRD metadata.

### **6 Mandatory Labels (V2.6)**

Labels are grouped by how they are populated:

#### **Group A: Auto-Populated Labels** (Signal Processing derives automatically from K8s/Prometheus)

| # | Label | Type | Source | Wildcard | Description |
|---|---|---|---|---|---|
| 1 | `signal_type` | TEXT | K8s Event Reason | ❌ NO | What happened (OOMKilled, CrashLoopBackOff, NodeNotReady). **Optional for catalog matching** (v2.6: metadata only, not used as primary search key). SP continues to populate for signal classification. |
| 2 | `severity` | ENUM[] | Alert/Event | ❌ NO | How bad (critical, high, medium, low). Always array in JSONB. No wildcard. |
| 3 | `component` | TEXT | K8s Resource | ❌ NO | What resource (pod, deployment, node) |

**Derivation**: These labels are extracted directly from Kubernetes events, Prometheus alerts, or signal metadata. **No user configuration required.**

> **v2.6 Note**: `signal_type` remains auto-populated by SP for signal classification (ADR-054) and is available as metadata on workflow entries. However, it is **no longer the primary catalog search key**. See Group C below and DD-WORKFLOW-016.

#### **Group B: System-Classified Labels** (Signal Processing derives with configurable defaults)

| # | Label | Type | Source | Wildcard | Description |
|---|---|---|---|---|---|
| 4 | `environment` | ENUM[] | Namespace Labels | ✅ YES | Where (production, staging, development, test, '*'). Always array in JSONB (v2.5). |
| 5 | `priority` | ENUM | Derived | ✅ YES | Business priority (P0, P1, P2, P3, '*') |

**Derivation**: Signal Processing applies Rego policies to derive these labels from K8s context (namespace labels, annotations, resource metadata). Users can customize derivation logic via Rego policy ConfigMaps.

**Default Logic** (if no custom Rego):
- `environment`: From namespace label `kubernaut.ai/environment` (single authoritative source)
- `priority`: Derived from `severity` + `environment` (critical + production -> P0)

**Rationale**: Using only `kubernaut.ai/` prefixed labels prevents accidentally capturing labels from other systems.

#### **Group C: Workflow-Defined Labels (v2.6)** (Workflow author selects from enforced taxonomy)

| # | Label | Type | Source | Wildcard | Description |
|---|---|---|---|---|---|
| 6 | `action_type` | TEXT | Workflow Author | ❌ NO | **Primary catalog matching key.** What the workflow does (ScaleReplicas, RestartPod, IncreaseCPULimits, etc.). Selected from enforced taxonomy defined in DD-WORKFLOW-016. |

**Derivation**: Workflow authors select from the predefined action type taxonomy when registering workflows. The LLM selects an action type after RCA investigation via the `list_available_actions` tool (DD-WORKFLOW-016).

**Valid Values (V1.0 Taxonomy)**:
- `ScaleReplicas` - Horizontally scale a workload
- `RestartPod` - Kill and recreate specific pod(s)
- `RestartDeployment` - Rolling restart of all pods in a workload
- `IncreaseCPULimits` - Raise CPU resource limits
- `IncreaseMemoryLimits` - Raise memory resource limits
- `RollbackDeployment` - Revert to previous revision
- `DrainNode` - Drain and cordon a node (evict pods)
- `CordonNode` - Cordon a node (prevent scheduling, no eviction)
- `CleanupNode` - Reclaim disk space (purge temp files, old logs, unused images)
- `DeletePod` - Delete specific pod(s)

**Authority**: DD-WORKFLOW-016 governs the action type taxonomy. New types require a DD-WORKFLOW-016 amendment.

---

### **Custom Labels (V1.5 - Subdomain-Based)**

Operators define custom labels via Rego policies. Kubernaut extracts and passes them through unchanged.

#### **Label Format**

```
<subdomain>.kubernaut.io/<key>[:<value>]
```

| Component | Description | Example |
|-----------|-------------|---------|
| `subdomain` | Category/dimension (becomes filter key) | `constraint`, `team`, `region` |
| `.kubernaut.io/` | Namespace (hidden from downstream) | *(internal)* |
| `key` | Label identifier | `cost-constrained`, `name` |
| `value` | Optional (empty = boolean true) | `payments`, `us-east-1` |

#### **Extraction Rules**

| Input Value | Output | Example |
|-------------|--------|---------|
| Empty `""` | `key` only | `cost-constrained` |
| `"true"` | `key` only (normalized) | `stateful-safe` |
| `"false"` | *(omitted)* | — |
| Other value | `key=value` | `name=payments` |

#### **Storage Structure**

```go
// map[subdomain][]string
CustomLabels map[string][]string

// Example:
{
  "constraint": ["cost-constrained", "stateful-safe"],
  "team": ["name=payments"],
  "region": ["zone=us-east-1"]
}
```

#### **Query Behavior**

Each subdomain becomes a **hard filter** in Data Storage:

```sql
WHERE custom_labels->'constraint' ? 'cost-constrained'
  AND custom_labels->'team' ? 'name=payments'
```

#### **Operator Freedom**

Operators define their own subdomains. Kubernaut does NOT validate subdomain names.

**Recommended Conventions** (documentation only):

| Subdomain | Use Case | Example Values |
|-----------|----------|----------------|
| `constraint` | Workflow constraints | `cost-constrained`, `stateful-safe` |
| `team` | Ownership | `name=payments`, `name=platform` |
| `region` | Geographic | `zone=us-east-1` |
| `compliance` | Regulatory | `pci`, `hipaa` |

**Key Principle**: Kubernaut is a **conduit, not a transformer**. Custom labels flow unchanged from Rego → SignalProcessing → HolmesGPT-API → Data Storage.

#### **Validation Limits (V1.9)**

SignalProcessing enforces validation limits on CustomLabels output:

| Constraint | Limit | Rationale |
|------------|-------|-----------|
| Max keys (subdomains) | 10 | Prevent prompt bloat, reasonable filtering dimensions |
| Max values per key | 5 | Reasonable multi-value, prevent unbounded arrays |
| Max key length | 63 chars | K8s label key compatibility |
| Max value length | 100 chars | Prompt efficiency, reasonable constraint values |
| Allowed key chars | `[a-zA-Z0-9._-]` | K8s label key compatible |
| Allowed value chars | UTF-8 printable | Prompt safety, no control characters |
| Reserved key prefixes | `kubernaut.ai/`, `system/` | Prevent collision with system labels |

**Total max size**: 10 keys × 5 values × 100 chars ≈ **5KB** (well within HolmesGPT-API's 64k token limit)

**Validation Behavior**:
- Keys exceeding limits → **truncated** with warning log
- Values exceeding limits → **truncated** with warning log
- Reserved prefixes → **stripped** (security enforcement)
- Invalid characters → **rejected** with error log

#### **Security Measures (V1.9 - Sandboxed OPA Runtime)**

CustomLabels are extracted via Rego policies in a **sandboxed OPA runtime**:

| Measure | Setting | Rationale |
|---------|---------|-----------|
| Network access | ❌ Disabled | Prevent data exfiltration |
| Filesystem access | ❌ Disabled | Prevent local file access |
| Evaluation timeout | 5 seconds | Prevent infinite loops |
| Memory limit | 128 MB | Prevent memory exhaustion |
| External data | ❌ Disabled (V1.0) | Inline tables only, no `http.send()` |

**Mandatory Label Protection (Security Wrapper)**:

SignalProcessing wraps customer Rego policies with a security policy that **strips** attempts to override the 5 mandatory labels:

```rego
# Security wrapper - strips system labels from customer output
system_labels := {
    "kubernaut.io/signal_type",
    "kubernaut.io/severity",
    "kubernaut.io/component",
    "kubernaut.io/environment",
    "kubernaut.io/priority"
}

# Customer labels with system labels removed
final_labels[key] = value {
    customer_labels[key] = value
    not startswith(key, "kubernaut.io/signal")
    not startswith(key, "kubernaut.io/sever")
    not startswith(key, "kubernaut.io/compon")
    not startswith(key, "kubernaut.io/environ")
    not startswith(key, "kubernaut.io/prior")
}
```

**Defense in Depth**: Even if Rego policy attempts to set mandatory labels, they are stripped before output.

**Reference**: [HANDOFF_CUSTOM_LABELS_EXTRACTION_V1.md](../../services/crd-controllers/01-signalprocessing/HANDOFF_CUSTOM_LABELS_EXTRACTION_V1.md)

---

### **DetectedLabels (V1.0 - Auto-Detected from K8s)**

SignalProcessing auto-detects these labels from Kubernetes resources **without any customer configuration**.

#### **DetectedLabels Fields (8 Fields)**

| Field | Type | Wildcard | Detection Method | Used For |
|-------|------|----------|------------------|----------|
| `gitOpsManaged` | bool | ❌ No | ArgoCD/Flux annotations present | LLM context |
| `gitOpsTool` | string | ✅ `"*"` | `"argocd"`, `"flux"`, or omitted | Workflow selection |
| `pdbProtected` | bool | ❌ No | PodDisruptionBudget exists | Risk assessment |
| `hpaEnabled` | bool | ❌ No | HorizontalPodAutoscaler targets workload | Scaling context |
| `stateful` | bool | ❌ No | StatefulSet in owner chain | State handling |
| `helmManaged` | bool | ❌ No | `helm.sh/chart` label present | Deployment method |
| `networkIsolated` | bool | ❌ No | NetworkPolicy exists in namespace | Security context |
| `serviceMesh` | string | ✅ `"*"` | `"istio"`, `"linkerd"`, or omitted | Traffic management |

**Note**: Only **string fields** support wildcards. Boolean fields use absence semantics (see Boolean Normalization Rule below).

#### **Detailed Detection Methods (v2.3)**

> **Added**: 2025-12-06 - Specific annotations and labels for each detection type

| Field | Detection Source | Specific Check | API Call Required |
|-------|------------------|----------------|-------------------|
| `gitOpsManaged` | Deployment/Namespace annotations | ArgoCD: `argocd.argoproj.io/instance`<br>Flux: `fluxcd.io/sync-gc-mark` label | No (existing data) |
| `gitOpsTool` | Same as above | `"argocd"` or `"flux"` based on which is present | No (existing data) |
| `pdbProtected` | K8s API: PodDisruptionBudgets | List PDBs in namespace, check if selector matches pod labels | Yes |
| `hpaEnabled` | K8s API: HorizontalPodAutoscalers | List HPAs in namespace, check if `scaleTargetRef` matches deployment | Yes |
| `stateful` | Owner chain (from Day 7) | Check if any `OwnerChainEntry.Kind == "StatefulSet"` | No (owner chain param) |
| `helmManaged` | Deployment labels | `app.kubernetes.io/managed-by: Helm` or `helm.sh/chart` annotation | No (existing data) |
| `networkIsolated` | K8s API: NetworkPolicies | List NetworkPolicies in namespace (existence = isolated) | Yes |
| `serviceMesh` | Pod annotations | Istio: `sidecar.istio.io/status` (present after injection)<br>Linkerd: `linkerd.io/proxy-version` (present after injection) | No (existing data) |

**ServiceMesh Detection Rationale** (2025-12-06):
- Both Istio and Linkerd inject sidecars into pods, adding specific annotations post-injection
- Istio adds `sidecar.istio.io/status` with sidecar configuration JSON
- Linkerd adds `linkerd.io/proxy-version` with the proxy version string
- These annotations are reliable indicators that the pod is mesh-enabled (injection complete)

#### **Boolean Normalization Rule (V1.5)**

**CRITICAL**: Boolean fields are **only included when `true`**. Omit when `false`.

| Condition | Field Included? | Example |
|-----------|-----------------|---------|
| `gitOpsManaged = true` | ✅ Yes | `"gitOpsManaged": true` |
| `gitOpsManaged = false` | ❌ Omitted | *(field absent)* |
| `gitOpsTool = "argocd"` | ✅ Yes (non-empty) | `"gitOpsTool": "argocd"` |
| `gitOpsTool = ""` | ❌ Omitted | *(field absent)* |

**Rationale**:
1. **Payload cleanliness**: No misleading `false` values cluttering the data
2. **Rego simplicity**: Checking `input.detected_labels.gitOpsManaged` implicitly means `true`
3. **Data consistency**: `gitOpsTool` only makes sense when `gitOpsManaged` is `true`

#### **Go Implementation Pattern**

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
    if dl.ServiceMesh != "" {
        result["serviceMesh"] = dl.ServiceMesh
    }

    return result
}
```

**Reference**: [HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md](../../services/crd-controllers/01-signalprocessing/HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md)

#### **Detection Failure Handling (V2.1 - December 2025)**

**CRITICAL**: DetectedLabels uses plain `bool` fields with a separate `FailedDetections` array to track which detections failed. This avoids the `*bool` anti-pattern while providing explicit failure tracking.

| Scenario | `FailedDetections` | Field Value | Meaning |
|----------|-------------------|-------------|---------|
| Detection succeeds (found) | Field NOT in array | `true` | Feature confirmed present |
| Detection succeeds (not found) | Field NOT in array | `false` | Feature confirmed absent |
| Detection fails (RBAC, timeout) | Field IN array | `false` (ignore) | Unknown - detection failed |

**Concrete Example** (`pdbProtected` field):

| Scenario | `pdbProtected` | `FailedDetections` | Interpretation |
|----------|----------------|-------------------|----------------|
| PDB exists for pod | `true` | `[]` | ✅ Has PDB protection |
| No PDB for pod | `false` | `[]` | ✅ No PDB protection |
| RBAC denied querying PDBs | `false` | `["pdbProtected"]` | ⚠️ Unknown - skip filter |

**Key Distinction**: "Resource doesn't exist" is NOT a failure - it's a successful detection with result `false`.

**Design Decision (December 2025 - Inter-Team Agreement)**:

1. **Plain `bool` fields** - No `*bool` pointers (anti-pattern)
2. **`FailedDetections []string`** - Lists field names where detection failed
3. **Explicit failure tracking** - Consumers know exactly which detections failed
4. **Error logging** - Detection failures also emit error logs for observability

**Rationale**:

- **Avoids `*bool` anti-pattern**: `*bool` causes JSON ambiguity (`null` vs absent vs `false`)
- **Explicit about failures**: `FailedDetections` array clearly shows what couldn't be detected
- **Auditable**: Post-incident analysis can see exactly which detections failed
- **Cleaner Rego**: No need to handle `null` values, just check array membership
- **Smaller payload**: When all detections succeed, `FailedDetections` is omitted

**Consumer Handling**:

```go
// Check if detection succeeded before trusting the value
if !slices.Contains(labels.FailedDetections, "pdbProtected") {
    // labels.PDBProtected is reliable
    if labels.PDBProtected {
        // Has PDB protection
    }
} else {
    // Detection failed - don't trust the value
}
```

**Rego Policy Example**:
```rego
# Trust value only if detection succeeded
pdb_protected {
    not "pdbProtected" in input.detected_labels.failedDetections
    input.detected_labels.pdbProtected
}

# Require approval if critical detections failed
require_approval {
    "gitOpsManaged" in input.detected_labels.failedDetections
    input.environment == "production"
}
```

**Go Type Definition** (authoritative - `pkg/shared/types/enrichment.go`):
```go
// ValidDetectedLabelFields defines the allowed values for FailedDetections
// Used by go-playground/validator for enum validation
var ValidDetectedLabelFields = []string{
    "gitOpsManaged",
    "pdbProtected",
    "hpaEnabled",
    "stateful",
    "helmManaged",
    "networkIsolated",
    "serviceMesh",
}

type DetectedLabels struct {
    // Detection metadata - lists fields where detection failed (RBAC, timeout, etc.)
    // If a field name is in this array, its value should be ignored
    // If empty/nil, all detections succeeded
    // Validated: only accepts values from ValidDetectedLabelFields
    FailedDetections []string `json:"failedDetections,omitempty" validate:"omitempty,dive,oneof=gitOpsManaged pdbProtected hpaEnabled stateful helmManaged networkIsolated serviceMesh"`

    // GitOps Management
    GitOpsManaged bool   `json:"gitOpsManaged"`
    GitOpsTool    string `json:"gitOpsTool,omitempty" validate:"omitempty,oneof=argocd flux"`

    // Workload Protection
    PDBProtected bool `json:"pdbProtected"`
    HPAEnabled   bool `json:"hpaEnabled"`

    // Workload Characteristics
    Stateful    bool `json:"stateful"`
    HelmManaged bool `json:"helmManaged"`

    // Security Posture
    NetworkIsolated bool   `json:"networkIsolated"`
    ServiceMesh     string `json:"serviceMesh,omitempty" validate:"omitempty,oneof=istio linkerd"`
}
```

**Validation Setup** (in `pkg/shared/types/validation.go`):
```go
package types

import (
    "github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
    validate = validator.New()
}

// ValidateDetectedLabels validates the DetectedLabels struct
func ValidateDetectedLabels(dl *DetectedLabels) error {
    return validate.Struct(dl)
}
```

**Validation Behavior**:
- `FailedDetections` only accepts known field names (enum validation)
- `GitOpsTool` only accepts `"argocd"` or `"flux"` (or empty)
- `ServiceMesh` only accepts `"istio"` or `"linkerd"` (or empty)

**Example Validation Error**:
```go
labels := &DetectedLabels{
    FailedDetections: []string{"unknownField"}, // Invalid!
}
err := ValidateDetectedLabels(labels)
// err: "FailedDetections[0]" failed on "oneof" tag
```

**SignalProcessing Implementation**:
```go
func (d *LabelDetector) DetectLabels(ctx context.Context, k8sCtx *KubernetesContext) *DetectedLabels {
    labels := &DetectedLabels{}
    var failedDetections []string

    // PDB detection
    hasPDB, err := d.detectPDB(ctx, k8sCtx)
    if err != nil {
        log.Error(err, "Failed to detect PDB (RBAC?)")
        failedDetections = append(failedDetections, "pdbProtected")
        // labels.PDBProtected remains false (but ignored due to failedDetections)
    } else {
        labels.PDBProtected = hasPDB
    }

    // GitOps detection
    managed, tool, err := d.detectGitOps(ctx, k8sCtx)
    if err != nil {
        log.Error(err, "Failed to detect GitOps")
        failedDetections = append(failedDetections, "gitOpsManaged")
    } else {
        labels.GitOpsManaged = managed
        labels.GitOpsTool = tool
    }

    // ... other detections ...

    if len(failedDetections) > 0 {
        labels.FailedDetections = failedDetections
    }
    return labels
}
```

**JSON Examples**:

*All detections succeeded:*
```json
{
  "gitOpsManaged": true,
  "gitOpsTool": "argocd",
  "pdbProtected": true,
  "hpaEnabled": false,
  "stateful": false,
  "helmManaged": true,
  "networkIsolated": false
}
```

*Some detections failed (RBAC issues):*
```json
{
  "failedDetections": ["pdbProtected", "hpaEnabled"],
  "gitOpsManaged": true,
  "gitOpsTool": "argocd",
  "pdbProtected": false,
  "hpaEnabled": false,
  "stateful": false,
  "helmManaged": true,
  "networkIsolated": false
}
```
Note: `pdbProtected` and `hpaEnabled` values should be ignored because they're in `failedDetections`.

---

#### **Wildcard Support for DetectedLabels String Fields (V1.6)**

**String fields** in DetectedLabels support wildcard matching (`"*"`) in workflow blueprints:

| Field | Wildcard Support | Values |
|-------|------------------|--------|
| `gitOpsTool` | ✅ `"*"` | `"argocd"`, `"flux"`, `"*"` |
| `serviceMesh` | ✅ `"*"` | `"istio"`, `"linkerd"`, `"*"` |

**Boolean fields do NOT support wildcards** - absence means "no requirement".

#### **Matching Semantics**

**Key Principle**:
- **Signal** describes what the workload **IS** (auto-detected facts)
- **Workflow** describes what the workflow **SUPPORTS/REQUIRES**

| Workflow Specifies | Signal Has Value | Signal Absent | Meaning |
|--------------------|------------------|---------------|---------|
| `"argocd"` | ✅ if `argocd` | ❌ No | "I only support ArgoCD" |
| `"*"` | ✅ Any value | ❌ No | "I support any GitOps tool (but require one)" |
| *(absent)* | ✅ Any value | ✅ Yes | "I have no GitOps requirement" (generic) |

**Complete Matching Matrix for `gitOpsTool`**:

| Workflow Has | Signal: `argocd` | Signal: `flux` | Signal: *(absent)* |
|--------------|------------------|----------------|---------------------|
| `"argocd"` | ✅ Match | ❌ No | ❌ No |
| `"flux"` | ❌ No | ✅ Match | ❌ No |
| `"*"` | ✅ Match | ✅ Match | ❌ No |
| *(absent)* | ✅ Match | ✅ Match | ✅ Match |

**Important Distinction**:
- `"*"` = "I require SOME value" (any GitOps tool, but must have one)
- *(absent)* = "I have NO requirement" (matches anything including absent)

#### **SQL Implementation Pattern**

```sql
-- Workflow requires ArgoCD specifically
WHERE signal.detected_labels->>'gitOpsTool' = 'argocd'

-- Workflow requires ANY GitOps tool (wildcard "*")
WHERE signal.detected_labels->>'gitOpsTool' IS NOT NULL
  AND (workflow.detected_labels->>'gitOpsTool' = '*'
       OR workflow.detected_labels->>'gitOpsTool' = signal.detected_labels->>'gitOpsTool')

-- Workflow has no requirement (field absent in workflow) - no filter applied
-- Generic workflows match any signal
```

#### **Workflow Blueprint Examples**

**GitOps-specific workflow** (ArgoCD only):
```json
{
  "detected_labels": {
    "gitOpsTool": "argocd"
  }
}
```

**Any-GitOps workflow** (requires GitOps, any tool):
```json
{
  "detected_labels": {
    "gitOpsTool": "*"
  }
}
```

---

### **⭐ DetectedLabels End-to-End Architecture (V2.0)**

> ⚠️ **CRITICAL DISTINCTION**: There are TWO different contexts for "detected_labels" - do NOT confuse them.

#### **Two Different Contexts**

| Context | Owner | When Populated | Data Type | Purpose |
|---------|-------|----------------|-----------|---------|
| **Incident DetectedLabels** | SignalProcessing | At runtime (incident detection) | Auto-detected facts | Describes what the affected workload **IS** |
| **Workflow Catalog detected_labels** | Workflow Author | At workflow creation | Metadata constraints | Describes what environments the workflow **SUPPORTS** |

#### **End-to-End Flow Diagram**

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              INCIDENT OCCURS                                     │
│                     (OOMKilled in production namespace)                          │
└─────────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         SIGNAL PROCESSING (V1.0)                                 │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │ AUTO-DETECT from LIVE Kubernetes cluster:                                │   │
│  │   - Check ArgoCD/Flux annotations → gitOpsManaged: true, gitOpsTool: argocd │
│  │   - Query PodDisruptionBudget → pdbProtected: true                       │   │
│  │   - Query HorizontalPodAutoscaler → hpaEnabled: false (omitted)          │   │
│  │   - Check helm.sh/chart label → helmManaged: true                        │   │
│  │   - Check NetworkPolicy → networkIsolated: false (omitted)               │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                  │
│  OUTPUT: EnrichmentResults.DetectedLabels = { (**ADR-056: removed, now in PostRCAContext**) │
│    "gitOpsManaged": true,                                                        │
│    "gitOpsTool": "argocd",                                                       │
│    "pdbProtected": true,                                                         │
│    "helmManaged": true                                                           │
│  }                                                                               │
└─────────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                            HOLMESGPT-API                                         │
│  - Receives incident with DetectedLabels from SignalProcessing                   │
│  - Passes DetectedLabels as FILTERS to workflow catalog search                   │
│  - LLM also sees DetectedLabels for context understanding                        │
└─────────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         DATA STORAGE (Workflow Catalog)                          │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │ WORKFLOW METADATA (author-defined at creation time):                     │   │
│  │                                                                          │   │
│  │ Workflow A: "scale-horizontal-argocd"                                    │   │
│  │   detected_labels: { "gitOpsTool": "argocd" }  ← "I only support ArgoCD" │   │
│  │                                                                          │   │
│  │ Workflow B: "restart-pod-generic"                                        │   │
│  │   detected_labels: {}  ← "I have no requirements, generic workflow"      │   │
│  │                                                                          │   │
│  │ Workflow C: "scale-horizontal-gitops"                                    │   │
│  │   detected_labels: { "gitOpsTool": "*" }  ← "I support any GitOps tool"  │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                  │
│  MATCHING: Incident's gitOpsTool="argocd" matches:                               │
│    ✅ Workflow A (exact match)                                                   │
│    ✅ Workflow B (no requirement = matches anything)                             │
│    ✅ Workflow C (wildcard = matches any value)                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

#### **Key Principles**

1. **SignalProcessing AUTO-POPULATES** incident DetectedLabels from live K8s ✅ IMPLEMENTED (V1.0)
2. **Workflow authors MANUALLY DEFINE** workflow catalog detected_labels at creation time
3. **Data Storage does NOT auto-populate** workflow detected_labels - they are workflow metadata
4. **HolmesGPT-API passes incident DetectedLabels** as filters to Data Storage search
5. **Matching logic** is in Data Storage: incident labels match workflow metadata

#### **Common Misconception**

> ❌ "Data Storage should auto-populate detected_labels"

**Clarification**: Data Storage **receives** incident DetectedLabels as search filters. It does NOT populate them. The workflow catalog's `detected_labels` field is **workflow metadata** (author-defined), not auto-detected.

| Service | Populates | Consumes |
|---------|-----------|----------|
| SignalProcessing | Incident DetectedLabels (auto-detect from K8s) | — |
| HolmesGPT-API | — | Incident DetectedLabels (from SP), passes to DS as filters |
| Data Storage | — | Incident DetectedLabels (as filters), Workflow detected_labels (as metadata) |
| Workflow Author | Workflow detected_labels (manual, at creation) | — |

---
```

**Generic workflow** (no GitOps requirement):
```json
{
  "detected_labels": {}
}
```

---

### **DetectedLabels Validation Architecture (V1.7 - 100% Safe)**

**Problem**: DetectedLabels describe the **original signal's resource** (e.g., Pod). If RCA identifies a **different resource** (e.g., Node), those labels are **invalid** and could cause query failures.

**Solution**: 100% safe validation using owner chain from SignalProcessing.

#### **Dual-Use Architecture**

DetectedLabels serve **two distinct purposes** with different requirements:

| Use Case | When Included | Accuracy Requirement |
|----------|---------------|---------------------|
| **LLM Prompt Context** | ALWAYS | Good enough (LLM can reason) |
| **Workflow Filtering** | CONDITIONAL (proven relationship) | 100% (query fails otherwise) |

**For LLM Prompt**: DetectedLabels are **always included** in the prompt to help the LLM understand the environment (GitOps, PDB, service mesh, etc.), even if RCA identifies a different resource.

**For Workflow Filtering**: DetectedLabels are **only included** when the relationship between source resource and RCA resource is **proven**.

#### **Owner Chain from SignalProcessing**

SignalProcessing traverses K8s `ownerReferences` to build the ownership chain.

##### **OwnerChainEntry Schema (AUTHORITATIVE)**

```go
// OwnerChainEntry represents a single entry in the K8s ownership chain
// SignalProcessing traverses ownerReferences to build this chain
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

**⚠️ IMPORTANT**: Do NOT include `apiVersion` or `uid` - they are NOT used by HolmesGPT-API validation.

##### **Example**

```json
{
  "source_resource": {
    "namespace": "production",
    "kind": "Pod",
    "name": "payment-api-7d8f9c6b5-x2k4m"
  },
  "enrichment_results": {
    "detectedLabels": {"gitOpsManaged": true, "gitOpsTool": "argocd"},  // ADR-056: removed, now in PostRCAContext
    "ownerChain": [  // ADR-055: removed
      {"namespace": "production", "kind": "ReplicaSet", "name": "payment-api-7d8f9c6b5"},
      {"namespace": "production", "kind": "Deployment", "name": "payment-api"}
    ]
  }
}
```

##### **Traversal Algorithm**

```go
func buildOwnerChain(ctx context.Context, client client.Client, resource metav1.Object) []OwnerChainEntry {
    var chain []OwnerChainEntry
    current := resource

    for {
        owners := current.GetOwnerReferences()
        if len(owners) == 0 {
            break // No more owners - chain complete
        }

        // Use first controller owner (typical K8s pattern)
        var controllerOwner *metav1.OwnerReference
        for _, ref := range owners {
            if ref.Controller != nil && *ref.Controller {
                controllerOwner = &ref
                break
            }
        }
        if controllerOwner == nil {
            break // No controller owner
        }

        // Add to chain (namespace inherited from current resource for namespaced owners)
        entry := OwnerChainEntry{
            Namespace: current.GetNamespace(), // Empty for cluster-scoped
            Kind:      controllerOwner.Kind,
            Name:      controllerOwner.Name,
        }
        chain = append(chain, entry)

        // Fetch owner to continue traversal
        owner, err := getResource(ctx, client, controllerOwner.APIVersion, controllerOwner.Kind,
                                   current.GetNamespace(), controllerOwner.Name)
        if err != nil {
            break // Owner not found - chain ends here
        }
        current = owner
    }

    return chain
}
```

#### **Validation Logic (100% Safe)**

```python
def should_include_detected_labels(source_resource, rca_resource, owner_chain):
    """Include ONLY when relationship is PROVEN. Default: EXCLUDE."""

    # Gate 1: Required data
    if not source_resource or not rca_resource:
        return False  # Safe default

    # Gate 2: Exact match
    if resources_match(source_resource, rca_resource):
        return True

    # Gate 3: Owner chain match (PROVEN relationship)
    for owner in (owner_chain or []):
        if resources_match(owner, rca_resource):
            return True

    # Gate 4: Same namespace + kind (fallback when owner_chain provided)
    if owner_chain is not None:
        if same_namespace_and_kind(source_resource, rca_resource):
            return True

    # Default: Cannot prove → EXCLUDE (100% safe)
    return False
```

#### **Supported Relationships**

| Source | RCA | Include DetectedLabels? | Reason |
|--------|-----|-------------------------|--------|
| Pod/prod/api-xyz | Pod/prod/api-xyz | ✅ YES | Exact match |
| Pod/prod/api-xyz | Deployment/prod/api | ✅ YES | Owner chain match |
| Pod/prod/api-xyz | StatefulSet/prod/api | ✅ YES | Owner chain match |
| Pod/prod/api-xyz | ReplicaSet/prod/api-abc | ✅ YES | Owner chain match |
| Pod/prod/api-xyz | Node/worker-3 | ❌ NO | Different scope |
| Pod/prod/api-xyz | Pod/staging/api-xyz | ❌ NO | Different namespace |
| Pod/prod/api-xyz | Deployment/prod/other | ❌ NO | Not in owner chain |

#### **LLM Tool Parameter**

The `search_workflow_catalog` tool includes `rca_resource` for validation:

```json
{
  "query": "DiskPressure critical",
  "rca_resource": {
    "signal_type": "DiskPressure",
    "kind": "Node",
    "name": "worker-3"
  },
  "top_k": 3
}
```

#### **Safety Guarantee**

| Scenario | Result | Why Safe |
|----------|--------|----------|
| source_resource missing | EXCLUDE | Can't compare |
| rca_resource missing | EXCLUDE | LLM didn't provide |
| owner_chain missing | EXCLUDE | Can't verify |
| owner_chain empty (orphan) | Same ns/kind check | Conservative fallback |
| owner_chain has match | INCLUDE | **PROVEN** |
| No match found | EXCLUDE | Can't prove |

**Result**: 100% safety - we **never include wrong labels** that could cause query failures.

---

### **Label Matching Rules**

**For MCP Workflow Search** (DD-LLM-001, DD-WORKFLOW-016):
1. **Primary Key (v2.6)**: `action_type` is the required primary filter (replaces `signal_type` per DD-WORKFLOW-016)
2. **Mandatory Label Filtering**: `severity` (array containment `?`), `environment` (array containment), `priority` (wildcard) used as SQL WHERE filters
3. **Component Filtering**: `component` used as SQL WHERE filter with wildcard support
4. **Signal Type (v2.6)**: Optional metadata on workflow entries, NOT used as primary search filter
5. **Wildcard Support (Mandatory Labels)**: `environment`, `priority` support `'*'` (matches any value)
6. **Wildcard Support (DetectedLabels)**: `gitOpsTool`, `serviceMesh` support `'*'` (matches any non-empty value)
7. **Custom Label Filtering**: Each subdomain becomes a separate WHERE clause (see V1.5 format above)
8. **Match Scoring**: Exact label matches + semantic similarity = final confidence score
9. **Two-Step Discovery (v2.6)**: LLM calls `list_available_actions` first, then `search_workflow_catalog` (DD-WORKFLOW-016)

**For Workflow Registration**:
1. **6 Labels Required (v2.6)**: Every workflow must have `action_type` (from taxonomy) + `severity`, `component`, `environment`, `priority`. `signal_type` is optional metadata.
2. **Action Type Validation**: `action_type` must be from DD-WORKFLOW-016 enforced taxonomy
3. **Custom Labels Optional**: Workflows can include custom labels for more specific matching
4. **Description Format**: Must follow `"<action_type> <severity>: <description>"` for optimal semantic matching
5. **Validation**: Labels are validated against authoritative values in this document and DD-WORKFLOW-016

### **Valid Values (Authoritative)**

#### **Group A: Auto-Populated Labels**

```yaml
signal_type:  # Domain-specific values from source systems (NO TRANSFORMATION)
  # CRITICAL PRINCIPLE: Use exact event reason strings from Kubernetes/Prometheus
  # WHY: LLM uses signal_type to query the same source system during investigation
  #      Example: signal_type="OOMKilled" → LLM runs: kubectl get events | grep "OOMKilled"
  #      If we transform "OOMKilled" → "pod-oomkilled", LLM queries will fail
  #
  # v2.6 NOTE: signal_type is NO LONGER the primary catalog search key.
  # It remains auto-populated by SP for signal classification (ADR-054) and
  # is available as optional metadata on workflow entries.
  # Primary catalog matching now uses action_type (DD-WORKFLOW-016).
  #
  # SOURCE: Kubernetes API - kubectl describe pod → State.Reason field
  # SOURCE: Prometheus - kube_pod_container_status_terminated_reason{reason="..."}
  #
  # Examples (use exact K8s event reason strings):
  - OOMKilled              # Container killed due to out-of-memory
  - CrashLoopBackOff       # Container repeatedly crashing
  - ImagePullBackOff       # Failed to pull container image
  - ErrImagePull           # Image pull error
  - NodeNotReady           # Node is not ready
  - Evicted                # Pod evicted due to resource pressure
  - Error                  # Generic container error
  - Completed              # Container completed successfully
  #
  # RULE: Signal Processing MUST pass through domain-specific values unchanged
  # RULE: NO normalization, NO kebab-case conversion, NO transformation

severity:  # From alert/event metadata. Always stored as JSONB array in workflow labels. No '*' wildcard.
  - critical
  - high
  - medium
  - low

component:  # Kubernetes resource types (auto-detected from signal)
  - pod
  - deployment
  - statefulset
  - daemonset
  - node
  - service
  - pvc
  - configmap
  - secret
```

#### **Group B: Rego-Configurable Labels**

```yaml
environment:  # Derived from namespace labels/annotations
  - production
  - staging
  - development
  - test
  - '*'  # Wildcard: matches any environment

priority:  # Derived from severity + environment via Rego
  - P0   # Critical production issue (immediate response)
  - P1   # High-priority issue (response within 1 hour)
  - P2   # Medium-priority issue (response within 4 hours)
  - P3   # Low-priority issue (response within 24 hours)
  - '*'  # Wildcard: matches any priority

risk_tolerance:  # Derived from priority + environment via Rego
  - low      # Conservative remediation (e.g., 10% resource increase, no restart)
  - medium   # Balanced remediation (e.g., 25% resource increase, rolling restart)
  - high     # Aggressive remediation (e.g., 50% resource increase, immediate restart)
```

#### **Group C: Workflow-Defined Labels (v2.6)**

```yaml
action_type:  # PRIMARY CATALOG MATCHING KEY (DD-WORKFLOW-016)
  # Enforced taxonomy - workflow authors select from this list
  # LLM selects via list_available_actions tool after RCA investigation
  # VerbNoun naming convention
  #
  - ScaleReplicas          # Horizontally scale a workload
  - RestartPod             # Kill and recreate specific pod(s)
  - RestartDeployment      # Rolling restart of all pods in a workload
  - IncreaseCPULimits      # Raise CPU resource limits
  - IncreaseMemoryLimits   # Raise memory resource limits
  - RollbackDeployment     # Revert to previous revision
  - DrainNode              # Drain and cordon a node (evict pods)
  - CordonNode             # Cordon a node (prevent scheduling, no eviction)
  - CleanupNode            # Reclaim disk space (purge temp files, old logs, unused images)
  - DeletePod              # Delete specific pod(s)
  #
  # RULE: New action types require DD-WORKFLOW-016 amendment
  # RULE: Validation rejects unknown action types at registration time
  # AUTHORITY: DD-WORKFLOW-016
```

#### **Optional Custom Labels (User-Defined)**

```yaml
# These are EXAMPLES - users define their own custom labels via Rego policies
# Custom labels are stored in JSONB and matched if present

business_category:  # OPTIONAL - Business domain categorization
  - payment-service
  - analytics
  - api-gateway
  - database
  - infrastructure
  - general
  - '*'  # Wildcard: matches any category

gitops_tool:  # OPTIONAL - GitOps tooling preference
  - argocd
  - flux
  - helm

region:  # OPTIONAL - Geographic targeting
  - us-east-1
  - eu-west-1
  - ap-southeast-1

team:  # OPTIONAL - Team ownership
  - platform
  - sre
  - payments
  - infrastructure
```

---

## 🎯 **Context & Problem**

### **Problem Statement**

The Workflow Catalog requires a standardized label schema to enable deterministic filtering and semantic search. Labels are used to match incoming signals with appropriate remediation workflows based on signal characteristics (type, severity, component, etc.).

**Key Requirements**:
1. **V1.0 Scope**: Support mandatory labels only (no custom labels)
2. **Deterministic Filtering**: Labels must enable SQL-based filtering before semantic search
3. **Signal Matching**: Labels must align with Signal Processing categorization output
4. **Future Extensibility**: Schema must support custom labels in V1.1

### **Current State**

- ✅ **Schema defined**: `workflow_catalog.labels` column (JSONB)
- ✅ **GIN index**: Efficient JSONB querying
- ❌ **NO authoritative label list**: Multiple documents reference different labels
- ❌ **Inconsistent terminology**: "signal_type" vs "incident-type", "severity" vs "priority"

### **Decision Scope**

Define the **mandatory label schema for V1.0** that:
- Aligns with Signal Processing categorization output
- Enables deterministic workflow filtering
- Supports future custom label extension (V1.1)

---

## 🔍 **Alternatives Considered**

### **Alternative 1: Minimal Labels (3 Fields)**

**Approach**: Support only the most critical labels for basic matching.

**Schema**:
```json
{
  "kubernaut.io/signal-type": "pod-oomkilled",
  "kubernaut.io/severity": "critical",
  "kubernaut.io/component": "pod"
}
```

**Pros**:
- ✅ **Simplicity**: Minimal schema, easy to understand
- ✅ **Fast implementation**: Less validation logic
- ✅ **Low cognitive load**: Only 3 fields to remember

**Cons**:
- ❌ **Insufficient filtering**: Cannot distinguish environment, risk tolerance
- ❌ **Limited matching**: Cannot filter by business category or priority
- ❌ **Weak "Filter Before LLM"**: Too coarse-grained for effective pre-filtering

**Confidence**: 40% (rejected - insufficient for production use)

---

### **Alternative 2: Structured Columns (5 Fields - 1:1 Signal Matching)** ⭐ **RECOMMENDED**

**Approach**: Use structured database columns for mandatory labels that **exactly match** Signal Processing Rego output. Playbooks are filtered by exact 1:1 label matching before semantic search.

**Schema**:
```sql
-- Enums for type safety and validation
CREATE TYPE severity_enum AS ENUM ('critical', 'high', 'medium', 'low'); -- v2.7: severity now stored as JSONB array in labels column
CREATE TYPE environment_enum AS ENUM ('production', 'staging', 'development', 'test', '*');
CREATE TYPE priority_enum AS ENUM ('P0', 'P1', 'P2', 'P3', '*');

CREATE TABLE workflow_catalog (
    workflow_id       TEXT NOT NULL,
    version           TEXT NOT NULL,
    title             TEXT NOT NULL,
    description       TEXT,

    -- 5 Mandatory structured labels (V1.4) - 1:1 matching with wildcard support
    -- Group A: Auto-populated from K8s/Prometheus
    signal_type       TEXT NOT NULL,              -- OOMKilled, CrashLoopBackOff, NodeNotReady
    severity          severity_enum NOT NULL,     -- v2.7: now JSONB array in labels column
    component         TEXT NOT NULL,              -- pod, deployment, node, service, pvc
    -- Group B: Rego-configurable
    environment       environment_enum NOT NULL,  -- production, staging, development, test, '*'
    priority          priority_enum NOT NULL,     -- P0, P1, P2, P3, '*'

    -- Validation constraints
    CHECK (signal_type ~ '^[A-Za-z0-9-]+$'),  -- Exact K8s event reason (no transformation)
    CHECK (component ~ '^[a-z0-9-]+$'),

    -- Custom labels (user-defined via Rego, stored in JSONB)
    -- Format: map[subdomain][]string per V1.5
    -- Examples: risk_tolerance, business_category, team, region
    custom_labels     JSONB,

    embedding         vector(384),
    status            TEXT NOT NULL DEFAULT 'active',

    PRIMARY KEY (workflow_id, version)
);

-- Composite index for efficient label filtering (5 mandatory labels per v1.4)
CREATE INDEX idx_workflow_labels ON workflow_catalog (
    signal_type, severity, component, environment, priority
);

-- GIN index for custom label queries
CREATE INDEX idx_workflow_custom_labels ON workflow_catalog USING GIN (custom_labels);
```

**Rationale for 5 Mandatory Fields (V1.4)**:
- ✅ **1:1 Label Matching**: ALL 5 mandatory fields must match between signal and workflow
- ✅ **Wildcard Support**: Workflows can use `'*'` for `environment`, `priority` to match any value
- ✅ **Auto-Populated Labels**: Group A labels require no user configuration
- ✅ **Rego-Configurable Labels**: Group B labels can be customized via Rego policies
- ✅ **Custom Labels Optional**: Additional labels stored in JSONB (no enforcement)
- ✅ **Zero-Friction Default**: Works out-of-the-box without namespace→category mapping
- ✅ **Type Safety**: PostgreSQL enums prevent invalid values
- ✅ **Dual-Source Semantics** (via custom labels):
  - **Signal**: `custom_labels.risk_tolerance: ["low"]` = "I require a low-risk remediation"
  - **Workflow**: `custom_labels.risk_tolerance: ["low"]` = "I provide a low-risk remediation"
  - **Match**: Only when both agree (low matches low, high matches high)

**Wildcard Matching Logic**:
```sql
-- Signal: {environment: "production", priority: "P0"}
-- Matches workflows with:
--   1. Exact match: {environment: "production", priority: "P0"}
--   2. Wildcard match: {environment: "*", priority: "P0"}
--   3. Wildcard match: {environment: "production", priority: "*"}

WHERE signal_type = $1
  AND labels->'severity' ? $2                          -- JSONB array containment (v2.7)
  AND (labels->>'component' = $3 OR labels->>'component' = '*')
  AND (labels->'environment' ? $4 OR labels->'environment' ? '*')  -- JSONB array (v2.5)
  AND (labels->>'priority' = $5 OR labels->>'priority' = '*')
  -- Custom label matching via JSONB containment (includes risk_tolerance, business_category, etc.)
  AND (custom_labels @> $6 OR $6 IS NULL)
```

**Match Scoring (for LLM ranking)**:
- **Score 5**: All 5 mandatory labels exact match (most specific, per v1.4)
- **Score 5**: 5 exact + 1 wildcard
- **Score 4**: 4 exact + 2 wildcards (least specific)
- **Bonus**: +1 for each custom label match (if custom labels used)

Workflows are ranked by: `(match_score * 10) + semantic_similarity_score`

**Pros**:
- ✅ **Type safety**: Database enforces NOT NULL constraints
- ✅ **Query performance**: Direct column access for mandatory labels
- ✅ **Index efficiency**: B-tree index on 5 mandatory labels (v1.4)
- ✅ **Flexible custom labels**: JSONB with GIN index for user-defined labels
- ✅ **Zero-friction adoption**: No mandatory business_category configuration
- ✅ **Schema clarity**: Explicit columns for mandatory, JSONB for optional
- ✅ **Risk-aware**: Risk tolerance enables safe vs. aggressive workflows
- ✅ **Priority-based**: Priority enables P0 vs. P1 workflow selection
- ✅ **Strong "Filter Before LLM"**: Fine-grained pre-filtering reduces LLM context

**Cons**:
- ⚠️ **Schema migration**: Adding new mandatory fields requires ALTER TABLE
  - **Mitigation**: V1.1 custom labels use JSONB (no schema changes)
- ⚠️ **More columns**: 5 columns vs 1 JSONB column
  - **Mitigation**: Clearer schema, better performance

**Confidence**: 95% (approved - structured data is superior for mandatory fields)

---

### **Alternative 3: Flexible Labels (No Mandatory Fields)**

**Approach**: All labels are optional; playbooks define their own label requirements.

**Schema**:
```json
{
  "kubernaut.io/signal-type": "pod-oomkilled",  // optional
  "kubernaut.io/severity": "critical",           // optional
  "custom/my-label": "my-value"                  // optional
}
```

**Pros**:
- ✅ **Maximum flexibility**: Playbooks can define any labels
- ✅ **No validation burden**: No mandatory field validation

**Cons**:
- ❌ **No deterministic filtering**: Cannot guarantee label presence
- ❌ **Weak matching**: Cannot reliably filter playbooks
- ❌ **Inconsistent schema**: Different playbooks use different labels
- ❌ **Poor "Filter Before LLM"**: Cannot pre-filter without guaranteed labels

**Confidence**: 20% (rejected - too flexible for production reliability)

---

## ✅ **Decision**

**APPROVED: Alternative 2** - Structured Columns (5 Mandatory + DetectedLabels + CustomLabels)

**Rationale**:

1. **Type Safety & Performance**:
   - Database enforces NOT NULL constraints (no runtime validation needed)
   - Direct column access is faster than JSONB extraction (10-50x speedup)
   - Standard B-tree indexes on columns (better than GIN index on JSONB)
   - Query planner can optimize column-based queries more effectively

2. **Schema Clarity**:
   - Explicit columns make schema self-documenting
   - No need for "kubernaut.io/" prefix (clean field names)
   - Database schema tools (pg_dump, migrations) work naturally
   - IDE autocomplete works for column names

3. **Comprehensive Filtering**:
   - Environment-specific playbooks (production vs. staging)
   - Risk-aware playbooks (low vs. medium vs. high risk tolerance)
   - Business-aware playbooks (payment-service vs. analytics)

4. **Signal Processing Alignment**:
   - Signal Processing categorization outputs these fields
   - Direct mapping from signal → workflow columns
   - No transformation needed

5. **"Filter Before LLM" Pattern**:
   - Fine-grained pre-filtering reduces LLM context
   - SQL filtering is fast (< 5ms with column indexes)
   - Semantic search operates on pre-filtered subset

6. **Future-Proof**:
   - V1.0: Mandatory structured columns
   - V1.1: Add custom_labels JSONB column (optional, flexible)
   - Best of both worlds: structured + flexible

7. **Production-Ready**:
   - Comprehensive enough for real-world scenarios
   - Supports multi-environment deployments
   - Enables risk-aware remediation strategies

**Key Insight**: Structured columns for mandatory fields provide superior type safety, performance, and clarity compared to JSONB. V1.1 custom labels will use JSONB for flexibility, giving us the best of both worlds.

---

## 🏗️ **Implementation**

### **Mandatory Label Schema (V1.0)**

#### **Label Definitions (v1.6 - snake_case API fields)**

**5 Mandatory Labels** (per v1.4):

| API Field Name | K8s Annotation | Type | Required | Values | Description |
|---|---|---|---|---|---|
| `signal_type` | `kubernaut.io/signal-type` | string | ✅ YES | `OOMKilled`, `CrashLoopBackOff`, `NodeNotReady`, etc. | Signal type (exact K8s event reason) |
| `severity` | `kubernaut.io/severity` | string (search) / []string (storage) | ✅ YES | `critical`, `high`, `medium`, `low` | Signal severity level. Search sends single value; workflow labels store as JSONB array (v2.7). |
| `component` | `kubernaut.io/component` | string | ✅ YES | `pod`, `deployment`, `node`, `service`, `pvc`, etc. | Kubernetes resource type |
| `environment` | `kubernaut.io/environment` | string | ✅ YES | `production`, `staging`, `development`, `test`, `*` | Deployment environment |
| `priority` | `kubernaut.io/priority` | string | ✅ YES | `P0`, `P1`, `P2`, `P3`, `*` | Business priority level |

**Custom Labels** (customer-defined via Rego - stored in `custom_labels` JSONB):

| Example Subdomain | Example Values | Description |
|---|---|---|
| `constraint` | `["cost-constrained", "stateful-safe"]` | Workflow constraints |
| `team` | `["name=payments"]` | Team ownership |
| `risk_tolerance` | `["low"]`, `["medium"]`, `["high"]` | Risk tolerance (customer-derived) |
| `business_category` | `["payment-service"]` | Business domain (customer-derived) |

#### **Example Workflow Labels (v1.6 - Two Formats)**

**Example 1: Conservative OOMKilled Playbook (GitOps-managed, PDB-protected)**

*K8s CRD Metadata (annotations use kebab-case):*
```yaml
metadata:
  annotations:
    kubernaut.io/signal-type: "OOMKilled"
    kubernaut.io/severity: "critical"
    kubernaut.io/component: "pod"
    kubernaut.io/environment: "production"
    kubernaut.io/priority: "P0"
```

*Complete API Search Request (snake_case, all three label types):*
```json
{
  "signal_type": "OOMKilled",
  "severity": "critical",
  "component": "pod",
  "environment": "production",
  "priority": "P0",
  "detected_labels": {
    "gitOpsManaged": true,
    "gitOpsTool": "argocd",
    "pdbProtected": true,
    "helmManaged": true,
    "networkIsolated": true,
    "serviceMesh": "istio"
  },
  "custom_labels": {
    "risk_tolerance": ["low"],
    "constraint": ["cost-constrained"],
    "team": ["name=payments"]
  }
}
```

**Note**: `detected_labels` only includes booleans when `true` and strings when non-empty (per v1.5 Boolean Normalization rule). Fields like `hpaEnabled: false` and `stateful: false` are **omitted**.

**Use Case**: Payment service pods in production managed by ArgoCD with PDB protection and cost constraints → Conservative memory increase (10% bump, no restart)

---

**Example 2: Aggressive OOMKilled Playbook (Non-GitOps, no PDB)**

*API Search Request (snake_case):*
```json
{
  "signal_type": "OOMKilled",
  "severity": "high",
  "component": "pod",
  "environment": "staging",
  "priority": "P2",
  "detected_labels": {
    "hpaEnabled": true
  },
  "custom_labels": {
    "risk_tolerance": ["high"],
    "team": ["name=analytics"]
  }
}
```

**Note**: Only `hpaEnabled: true` appears in `detected_labels`. Fields like `gitOpsManaged`, `pdbProtected`, `stateful` are **omitted** because they are `false`.

**Use Case**: Analytics pods in staging with HPA (auto-scaling) but no GitOps or PDB protection → Aggressive memory increase (50% bump, immediate restart)

---

**Example 3: Node NotReady Playbook (Service mesh enabled)**

*API Search Request (snake_case):*
```json
{
  "signal_type": "NodeNotReady",
  "severity": "critical",
  "component": "node",
  "environment": "production",
  "priority": "P0",
  "detected_labels": {
    "serviceMesh": "istio"
  },
  "custom_labels": {
    "risk_tolerance": ["low"],
    "team": ["name=infrastructure"],
    "region": ["zone=us-east-1"]
  }
}
```

**Note**: Only `serviceMesh: "istio"` appears because it's a non-empty string. Boolean fields for nodes (like `gitOpsManaged`) are typically `false` and thus omitted.

**Use Case**: Node failures in production with Istio service mesh → Cordon node, drain pods with Istio awareness, investigate

---

### **Validation Rules**

#### **Schema Validation (Data Storage Service) - v1.6 snake_case**

```go
// pkg/datastorage/validation/workflow_labels.go

// WorkflowSearchFilters - API request filters (v1.6: snake_case)
type WorkflowSearchFilters struct {
    // 5 Mandatory labels (v1.4)
    SignalType  string `json:"signal_type" validate:"required"`
    Severity    string `json:"severity" validate:"required,oneof=critical high medium low"`
    Component   string `json:"component" validate:"required"`
    Environment string `json:"environment" validate:"required"`
    Priority    string `json:"priority" validate:"required"`

    // Custom labels (customer-defined via Rego, stored in JSONB)
    // Format: map[subdomain][]string
    CustomLabels map[string][]string `json:"custom_labels,omitempty"`
}

// ValidateMandatoryLabels validates that all 5 mandatory labels are present and valid (v1.4)
func ValidateMandatoryLabels(filters WorkflowSearchFilters) error {
    // Validate severity
    validSeverities := []string{"critical", "high", "medium", "low"}
    if !contains(validSeverities, filters.Severity) {
        return fmt.Errorf("invalid severity: %s (must be one of: %v)",
            filters.Severity, validSeverities)
    }

    // Validate environment (supports wildcard)
    validEnvironments := []string{"production", "staging", "development", "test", "*"}
    if !contains(validEnvironments, filters.Environment) {
        return fmt.Errorf("invalid environment: %s (must be one of: %v)",
            filters.Environment, validEnvironments)
    }

    // Validate priority (supports wildcard)
    validPriorities := []string{"P0", "P1", "P2", "P3", "*"}
    if !contains(validPriorities, filters.Priority) {
        return fmt.Errorf("invalid priority: %s (must be one of: %v)",
            filters.Priority, validPriorities)
    }

    // Note: risk_tolerance and business_category are now custom labels (v1.4)
    // They are NOT validated by Kubernaut - customers define their own values via Rego

    return nil
}
```

#### **SQL Filtering Pattern (v2.6 - Action-Type Primary + Mandatory + Custom Labels)**

```sql
-- Filter workflows by action_type (primary key, v2.6) + mandatory labels + optional custom labels
SELECT
    workflow_id,
    version,
    title,
    description,
    embedding
FROM workflow_catalog
WHERE status = 'active'
  -- Primary key (v2.6, DD-WORKFLOW-016)
  AND action_type = $1                                -- ScaleReplicas (from LLM selection)
  -- Mandatory labels (structured columns, snake_case)
  AND labels->'severity' ? $2                          -- critical (JSONB array containment)
  AND (labels->>'component' = $3 OR labels->>'component' = '*')   -- pod
  AND (labels->'environment' ? $4 OR labels->'environment' ? '*')  -- production (JSONB array, v2.5)
  AND (labels->>'priority' = $5 OR labels->>'priority' = '*')     -- P0 (with wildcard)
  -- Custom labels (JSONB containment - customer-defined)
  AND (custom_labels @> $6 OR $6 IS NULL)             -- {"constraint": ["cost-constrained"]}
ORDER BY embedding <=> $7  -- semantic similarity
LIMIT 10;
```

**Note (v2.6)**: `action_type` replaces `signal_type` as the primary search key (DD-WORKFLOW-016). `signal_type` is optional metadata on workflow entries, not a search filter. `risk_tolerance` and `business_category` are custom labels (v1.4), not mandatory columns.

---

### **Data Flow (v1.6)**

1. **Signal Processing categorizes signal**
   - Output: Signal with 5 mandatory labels (`signal_type`, `severity`, `component`, `environment`, `priority`)
   - Output: Optional custom labels (customer-defined via Rego, stored in `custom_labels`)

2. **HolmesGPT API receives signal**
   - Extracts labels from signal (snake_case format)
   - Auto-appends `custom_labels` to workflow search request (per DD-HAPI-001)
   - Calls Data Storage workflow search API

3. **Data Storage filters workflows**
   - **Step 1**: SQL filter by 5 mandatory labels (structured columns)
   - **Step 2**: JSONB containment filter by custom labels (if present)
   - **Step 3**: Semantic search on pre-filtered subset (similarity-based)
   - **Step 4**: Return top-k matching workflows

4. **HolmesGPT API selects playbook**
   - LLM reviews top-k playbooks
   - Selects best match based on signal context
   - Creates RemediationRequest CRD

---

### **V1.1 Extension: Custom Labels**

**V1.4 Schema**: 5 mandatory structured columns + optional custom labels in JSONB:

**Database Schema**:
```sql
-- V1.4: 5 Mandatory structured columns
signal_type       TEXT NOT NULL,      -- Group A: Auto-populated
severity          TEXT NOT NULL,      -- Group A: Auto-populated
component         TEXT NOT NULL,      -- Group A: Auto-populated
environment       TEXT NOT NULL,      -- Group B: Rego-configurable
priority          TEXT NOT NULL,      -- Group B: Rego-configurable

-- V1.5: JSONB for custom labels (user-defined via Rego)
-- Format: map[subdomain][]string
custom_labels     JSONB
```

**Example Custom Labels** (V1.5 subdomain format):
```json
{
  "risk_tolerance": ["low"],
  "constraint": ["cost-constrained", "stateful-safe"],
  "team": ["name=payments"],
  "region": ["zone=us-east-1"]
}
```

**Custom Label Keys**: Subdomain-based (e.g., `risk_tolerance`, `constraint`, `team`)

**V1.5 Filtering Strategy**:
- **Step 1**: Filter by 5 mandatory structured columns (fast, deterministic, per v1.4)
- **Step 2**: Filter by custom labels in JSONB if provided (subdomain-based, per v1.5)
- **Step 3**: Semantic search on pre-filtered subset

---

## 📊 **Consequences**

### **Positive**

- ✅ **Comprehensive Filtering**: Environment, risk, business context enable fine-grained matching
- ✅ **Production-Ready**: Schema supports real-world multi-environment deployments
- ✅ **Signal Processing Alignment**: Direct mapping from signal categorization output
- ✅ **"Filter Before LLM" Pattern**: Deterministic pre-filtering reduces LLM context
- ✅ **Future-Proof**: Extensible to custom labels in V1.1 without breaking changes
- ✅ **Risk-Aware**: Risk tolerance enables safe vs. aggressive remediation strategies

### **Negative**

- ⚠️ **Validation Complexity**: 5 mandatory fields require validation logic
  - **Mitigation**: Centralized validation function, comprehensive unit tests
- ⚠️ **Cognitive Load**: More fields to understand and maintain
  - **Mitigation**: Clear documentation, examples, validation error messages
- ⚠️ **Signal Processing Dependency**: Signal Processing must output all 5 mandatory labels
  - **Mitigation**: Signal Processing already categorizes signals; labels are natural output

### **Neutral**

- 🔄 **Schema Evolution**: V1.1 will add custom labels (backward compatible)
- 🔄 **Label Namespace**: `kubernaut.io/` prefix reserves namespace for mandatory labels
- 🔄 **JSONB Storage**: Supports both mandatory and custom labels without schema changes

---

## 🧪 **Validation Results**

### **Confidence Assessment Progression**

- **Initial assessment**: 70% confidence (label list unclear)
- **After Signal Processing alignment**: 80% confidence (labels match categorization output)
- **After "Filter Before LLM" analysis**: 85% confidence (comprehensive filtering validated)
- **After V1.1 extensibility review**: 90% confidence (expected after production deployment)

### **Key Validation Points**

- ✅ **Signal Processing Alignment**: Labels match Signal Processing categorization output
- ✅ **SQL Filtering**: GIN index supports efficient JSONB filtering (< 10ms)
- ✅ **Semantic Search**: Pre-filtering reduces search space (10x-100x speedup)
- ✅ **V1.1 Extensibility**: JSONB supports custom labels without schema migration

---

## 🔗 **Related Decisions**

- **Builds On**: DD-STORAGE-008 (Workflow Catalog Schema)
- **Builds On**: DD-STORAGE-012 (Critical Label Filtering)
- **Supports**: BR-STORAGE-012 (Playbook Semantic Search)
- **Supports**: AUDIT_TRACE_SEMANTIC_SEARCH_IMPLEMENTATION_PLAN_V1.4 (Day 3 implementation)
- **Supersedes**: None (new decision)
- **Related**: DD-EMBEDDING-001 (Embedding Service - semantic search after label filtering)

---

## 📋 **Review & Evolution**

### **When to Revisit**

- If **Signal Processing categorization changes**
  - **Action**: Update label schema to match new categorization output
- If **custom labels are needed before V1.1**
  - **Action**: Accelerate V1.1 custom label support
- If **label filtering becomes a bottleneck** (> 50ms)
  - **Action**: Optimize GIN index, add materialized views
- If **label validation becomes too strict**
  - **Action**: Relax validation rules, add default values

### **Success Metrics**

- **Filtering Performance**: p95 SQL filtering < 10ms
- **Match Accuracy**: 90%+ of signals match at least one playbook
- **False Positives**: < 5% of matches are irrelevant
- **Validation Errors**: < 1% of workflow creation requests fail validation

---

## 📝 **Business Requirements**

### **New BRs Created**

#### **BR-STORAGE-013: Mandatory Workflow Label Validation**
- **Category**: STORAGE
- **Priority**: P0 (blocking for Data Storage V1.0)
- **Description**: MUST validate that all workflows have 5 mandatory labels with valid values per DD-WORKFLOW-001 v1.6
- **Acceptance Criteria**:
  - Workflow creation fails if any mandatory label is missing
  - Workflow creation fails if any label has invalid value (not in authoritative list)
  - Wildcard validation: `environment`, `priority` accept `'*'`
  - Application-level validation enforces severity (JSONB array), environment (JSONB array), and priority values
  - CHECK constraints enforce `signal_type`, `component` format
  - Custom labels stored in JSONB (subdomain format per v1.5)
  - Validation errors include descriptive error messages
  - Unit tests cover all validation scenarios

#### **BR-STORAGE-014: Label-Based Workflow Filtering with Wildcards**
- **Category**: STORAGE
- **Priority**: P0 (blocking for Data Storage V1.0)
- **Description**: MUST support SQL-based filtering by mandatory labels with wildcard matching before semantic search
- **Acceptance Criteria**:
  - GET /api/v1/playbooks/search accepts 5 mandatory label filter parameters + custom_labels JSONB
  - SQL query supports wildcard matching: `(environment = $1 OR environment = '*')`
  - Composite index on all 5 mandatory labels for efficient filtering
  - JSONB containment filter for custom labels (includes risk_tolerance, business_category, etc.)
  - p95 filtering latency < 5ms
  - Returns playbooks ranked by match score (exact > wildcard)

#### **BR-STORAGE-015: Match Scoring and Ranking**
- **Category**: STORAGE
- **Priority**: P0 (blocking for Data Storage V1.0)
- **Description**: MUST rank playbooks by match specificity before semantic search
- **Acceptance Criteria**:
  - Calculate match score: 5 (all mandatory exact) + bonus for custom label matches
  - Rank playbooks by: `(match_score * 10) + semantic_similarity`
  - Return match score in API response for LLM decision-making
  - Unit tests validate scoring logic

#### **BR-SIGNAL-PROCESSING-001: Signal Label Enrichment (5 Mandatory Labels per v1.6)**
- **Category**: SIGNAL-PROCESSING
- **Priority**: P0 (blocking for Signal Processing V1.0)
- **Description**: MUST enrich signals with ALL 5 mandatory labels during categorization per DD-WORKFLOW-001 v1.6
- **Authority**: DD-WORKFLOW-001 v1.6 (authoritative label definitions)
- **Label Groups**:
  - **Auto-Populated** (Group A): `signal_type`, `severity`, `component`
  - **Rego-Configurable** (Group B): `environment`, `priority`
- **Acceptance Criteria**:
  - Signal categorization outputs all 5 mandatory labels (v1.6)
  - Group A labels extracted from K8s events/Prometheus alerts (no user config needed)
  - Group B labels derived via Rego policies (customizable by user)
  - Labels match DD-WORKFLOW-001 v1.6 authoritative values (snake_case API fields)
  - Labels are stored in RemediationRequest CRD spec
  - Labels are passed to HolmesGPT API for workflow matching
  - Custom labels (if any) are stored in `custom_labels` JSONB for optional matching

#### **BR-SIGNAL-PROCESSING-002: Custom Label Derivation (risk_tolerance, business_category, etc.)**
- **Category**: SIGNAL-PROCESSING
- **Priority**: P1 (optional enhancement)
- **Description**: MAY output custom labels (e.g., `risk_tolerance`, `business_category`) based on Rego policies
- **Authority**: DD-WORKFLOW-001 v1.6 (custom labels section)
- **Note**: `risk_tolerance` and `business_category` are now **custom labels**, not mandatory fields
- **Rego Policy Logic** (example for `risk_tolerance`):
  ```rego
  # risk_tolerance is a CUSTOM LABEL derived via Rego
  custom_labels["risk_tolerance"] = ["low"] {
      input.priority == "P0"
      input.environment == "production"
  }

  custom_labels["risk_tolerance"] = ["medium"] {
      input.priority == "P1"
      input.environment == "production"
  }

  custom_labels["risk_tolerance"] = ["high"] {
      input.priority in ["P2", "P3"]
  }

  custom_labels["risk_tolerance"] = ["high"] {
      input.environment in ["staging", "development", "test"]
  }

  custom_labels["risk_tolerance"] = ["medium"] {  # Fallback
      true
  }
  ```
- **Acceptance Criteria**:
  - Custom labels stored in `custom_labels` JSONB (subdomain format per v1.5)
  - Example: `{"risk_tolerance": ["low"], "business_category": ["payment-service"]}`
  - Unit tests cover derivation logic

#### **BR-SIGNAL-PROCESSING-003: Custom Label Support (OPTIONAL)**
- **Category**: SIGNAL-PROCESSING
- **Priority**: P2 (optional enhancement)
- **Description**: MAY support custom labels defined by users via Rego policies
- **Authority**: DD-WORKFLOW-001 v1.6 (customer-derived labels section)
- **Status**: ✅ **OPTIONAL** - Users configure if needed, no default required
- **Example Custom Labels**: `business_category`, `gitops_tool`, `region`, `team`
- **Rego Policy Logic** (user-defined):
  ```rego
  # EXAMPLE: User-defined business_category (OPTIONAL)
  custom_labels["business_category"] = data.namespace_categories[input.namespace] {
      data.namespace_categories[input.namespace]
  }

  custom_labels["business_category"] = "infrastructure" {
      input.resource.kind in ["Node", "PersistentVolume", "PersistentVolumeClaim"]
  }

  # EXAMPLE: User-defined gitops_tool (OPTIONAL)
  custom_labels["gitops_tool"] = annotation {
      annotation := input.deployment.annotations["argocd.argoproj.io/sync-wave"]
      annotation != ""
  }
  ```
- **Acceptance Criteria**:
  - Custom labels stored in JSONB column
  - Custom labels matched against workflow custom labels if present
  - No mandatory configuration required (zero-friction default)

---

## 🚀 **Next Steps**

1. ✅ **DD-WORKFLOW-001 v1.6 Approved** (this document - authoritative label schema)
2. ✅ **Simplified to 5 Mandatory Labels (v1.4)**: Removed `risk_tolerance` and `business_category` from mandatory (now custom labels via Rego)
3. ✅ **Standardized API Fields to snake_case (v1.6)**: All API/DB fields use snake_case
4. 🚧 **Update DD-STORAGE-008**: Reference DD-WORKFLOW-001 v1.6 for label schema
5. 🚧 **Implement Label Validation**: `pkg/datastorage/validation/workflow_labels.go` (5 mandatory per v1.6)
6. 🚧 **Update Workflow Schema Migration**: Add enums, CHECK constraints for 5 labels
7. 🚧 **Update Signal Processing Rego**: Implement Group A (auto-populate) + Group B (configurable)
8. 🚧 **Custom Label Support**: JSONB storage with subdomain format (per v1.5)
9. 🚧 **Integration Tests**: Validate label filtering, wildcard matching, and custom label matching

---

## 📋 **Changelog**

### **v2.7** (2026-02-15) - CURRENT
- ✅ **BREAKING**: `severity` in MandatoryLabels changed from `string` to `[]string` (always array, like environment)
- ✅ **REMOVED**: `"*"` wildcard for severity. To match any severity, list all four levels.
- ✅ **SQL Pattern**: `labels->'severity' ? $N` (JSONB containment)
- ✅ **Cross-reference**: DD-WORKFLOW-001 v2.7 changelog (top)

### **v2.6** (2026-02-05)
- ✅ **BREAKING**: `action_type` added as mandatory label (Group C: Workflow-Defined, 10 types)
- ✅ **BREAKING**: `signal_type` demoted from required primary key to optional metadata
- ✅ **NEW**: Enforced action type taxonomy (DD-WORKFLOW-016)
- ✅ **NEW**: `ListAvailableActions` context-aware HAPI tool
- ✅ **NEW**: LLM two-step workflow discovery protocol
- ✅ **Cross-reference**: DD-WORKFLOW-016, DD-HAPI-016

### **v1.6** (2025-11-30)
- ✅ **BREAKING**: Standardized all API/database field names to **snake_case**
- ✅ **Changed filter parameters**: `signal-type` → `signal_type`, etc.
- ✅ **Clarified naming convention**: K8s annotations (kebab-case) vs API fields (snake_case)
- ✅ **Updated Go struct JSON tags**: From `json:"kubernaut.io/signal-type"` to `json:"signal_type"`
- ✅ **Updated Business Requirements**: All BRs now reference v1.6
- ✅ **Cross-reference**: DD-WORKFLOW-002 v3.3, DD-HAPI-001

### **v1.5** (2025-11-30)
- ✅ **Custom Labels Subdomain Format**: `<subdomain>.kubernaut.io/<key>[:<value>]` → `map[string][]string`
- ✅ **Pass-Through Design**: Kubernaut is a conduit, not transformer
- ✅ **Boolean Normalization**: Empty/true → key only; false → omitted
- ✅ **Industry Alignment**: Follows Kubernetes label propagation pattern

### **v1.4** (2025-11-30)
- ✅ **BREAKING**: Reduced to 5 mandatory labels: `signal_type`, `severity`, `component`, `environment`, `priority`
- ✅ **Moved to custom labels**: `risk_tolerance`, `business_category`, `team`, `region`, etc.
- ✅ **Rationale**: Customers define environment meaning for risk via Rego policies

### **v1.3** (2025-11-30)
- ✅ **BREAKING**: Reduced from 7 to 6 mandatory labels
- ✅ **Removed `business_category` from mandatory**: Moved to optional custom labels
- ✅ **Added Label Grouping**: Auto-populated (Group A) vs Rego-configurable (Group B)
- ✅ **Added Custom Labels Section**: User-defined labels for organization-specific needs
- ✅ **Simplified Adoption**: No namespace→category mapping required by default

### **v1.2** (2025-11-16)
- ✅ **Clarified MCP Search Usage**: signal_type and severity for filtering + semantic ranking
- ✅ **Added Description Format**: `"<signal_type> <severity>: <description>"`
- ✅ **Cross-References**: DD-LLM-001, ADR-041

### **v1.1** (2025-11-14)
- ✅ **Added Wildcard Support**: `environment`, `priority` support `'*'`
- ✅ **Added Match Scoring**: Rank workflows by match specificity (exact > wildcard)
- ✅ **Added Type Safety**: PostgreSQL enums for `severity`, `environment`, `priority`, `risk_tolerance`
- ✅ **Added Validation Constraints**: CHECK constraints for `signal_type`, `component`
- ✅ **Added Authoritative Definitions**: Single source of truth for all label values
- ✅ **Added Signal Processing BRs**: BR-SIGNAL-PROCESSING-001, 002 with Rego policy logic
- ✅ **Added Data Storage BRs**: BR-STORAGE-013, 014, 015 for validation and filtering

- Initial mandatory label schema
- 1:1 signal-to-workflow matching
- Structured columns for mandatory labels

---

**Document Version**: 2.7
**Last Updated**: February 15, 2026
**Status**: ✅ **APPROVED** (95% confidence, action-type primary matching)
**Authority**: ⭐ **AUTHORITATIVE** - Single source of truth for workflow label schema
**Breaking Change (v2.7)**: Severity changed from string to JSONB array; no `*` wildcard. `action_type` replaces `signal_type` as primary catalog matching key (v2.6). Workflow registration now requires `action_type` from enforced taxonomy (DD-WORKFLOW-016).
**Cross-Reference**: DD-WORKFLOW-002 v3.3, DD-HAPI-001, DD-WORKFLOW-016, DD-HAPI-016
**Next Review**: After V1.0 implementation validates action-type matching accuracy

