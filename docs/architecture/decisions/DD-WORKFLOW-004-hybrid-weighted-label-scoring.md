# DD-WORKFLOW-004: Hybrid Weighted Label Scoring for Workflow Selection

**Date**: November 22, 2025
**Status**: ✅ **APPROVED** (V1.6 IMPLEMENTED)
**Confidence**: 95%
**Purpose**: Define the hybrid weighted scoring strategy for workflow catalog semantic search that combines strict filtering for mandatory labels with semantic similarity ranking.
**Related**: DD-WORKFLOW-012 (Workflow Immutability), DD-WORKFLOW-001 v2.4 (Mandatory Label Schema + Multi-Environment), DD-HAPI-001 (Custom Labels Auto-Append)
**Version**: 1.6 (CURRENT), 2.2 (SPECIFICATION)

---

## 📝 **Changelog**

### Version 2.0 (2026-01-28) **← CURRENT IMPLEMENTATION**
**FEATURE**: Multi-environment workflow capability with JSONB array containment.

**Breaking Changes**:
- ✅ **Storage Model**: Workflows declare `environment: []string`
  - Example: `["staging", "production"]` (workflow works in BOTH environments)
  - Example: `["*"]` (workflow works in ALL environments - wildcard)
  - Example: `["production"]` (workflow only for production)
- ✅ **Search Model**: Filters use `environment: string` (reverted from v1.6's `[]string`)
  - Signal Processing sends: `"production"` (single value)
  - HAPI passes through: `"production"` (single value)
- ✅ **SQL Pattern**: `labels->'environment' ? $N OR labels->'environment' ? '*'`
  - PostgreSQL JSONB `?` operator checks if array contains value
  - Wildcard check ensures `["*"]` workflows match all searches
- ✅ **Performance**: Uses existing GIN index on `labels` JSONB column (optimized for `?` operator)

**SQL Example**:
```sql
-- Workflow Storage (JSONB):
-- Workflow A: {"environment": ["staging", "production"], ...}
-- Workflow B: {"environment": ["production"], ...}
-- Workflow C: {"environment": ["*"], ...}

-- Search from Production Signal:
WHERE labels->'environment' ? 'production' OR labels->'environment' ? '*'
-- Matches: Workflow A, B, and C

-- Search from Staging Signal:
WHERE labels->'environment' ? 'staging' OR labels->'environment' ? '*'
-- Matches: Workflow A and C (NOT B)
```

**Use Case**: DevOps creates one workflow for multiple environments, reducing duplication and maintenance burden.

**Authority**: DD-WORKFLOW-001 v2.5, BR-STORAGE-040 v2.0

### Version 1.6 (2026-01-26) **[SUPERSEDED by v2.0]**
**REVERTED**: Incorrect implementation - search-side arrays instead of storage-side arrays

**SQL Example**:
```sql
-- Before (v1.5): Single environment
WHERE labels->>'environment' = 'production'

-- After (v1.6): Multiple environments
WHERE labels->>'environment' = ANY(ARRAY['staging', 'production'])
-- PostgreSQL expands to: WHERE labels->>'environment' IN ('staging', 'production')
```

**Authority**: DD-WORKFLOW-001 v2.4, BR-STORAGE-040 v1.5

### Version 1.5 (2025-12-11)
**IMPLEMENTATION**: Fixed weights for DetectedLabels (no configuration).

**Changes**:
- ✅ **Implemented hybrid scoring** with fixed DetectedLabel weights
- ✅ **Package created**: `pkg/datastorage/scoring` with weight definitions
- ✅ **SQL updated**: Dynamic boost/penalty calculation in `workflow_repository.go`
- ✅ **Comprehensive tests**: 16 unit tests + 4 integration tests
- ✅ **Security**: Enum sanitization prevents SQL injection
- ✅ **Fixed weights based on correctness impact**:
  - HIGH (0.10): `gitOpsManaged`, `gitOpsTool` (apply penalty on conflict)
  - MEDIUM (0.05): `pdbProtected`, `serviceMesh`
  - LOW (0.02-0.03): `networkIsolated`, `helmManaged`, `stateful`, `hpaEnabled`

**Rationale**:
- Fixed weights enable faster delivery (1-2 days vs. 1 week for ConfigMap)
- DetectedLabels are universal across K8s clusters (predictable impact)
- ConfigMap configuration deferred to V2.0+ after customer validation

**Implementation Authority**: `docs/services/stateless/data-storage/V1.5_HYBRID_SCORING_IMPLEMENTATION.md`

### Version 2.2 (2025-11-30) **← SPECIFICATION (Not Yet Implemented)**
**ALIGNMENT**: Updated to DD-WORKFLOW-001 v1.8 (snake_case API fields + DetectedLabels wildcards).

**Changes**:
- ✅ **snake_case API Fields**: All label field names use snake_case (`signal_type`, `severity`, etc.)
- ✅ **Structured Columns**: 5 mandatory labels are structured columns (not JSONB)
- ✅ **CustomLabels JSONB**: `risk_tolerance`, `business_category` stored in `custom_labels` JSONB
- ✅ **Updated SQL Queries**: Use structured column references, not `labels->>'signal-type'`
- ✅ **DetectedLabels Wildcard Support**: String fields (`gitOpsTool`, `podSecurityLevel`, `serviceMesh`) support `"*"`
- ✅ **Matching Semantics**: `"*"` = "requires SOME value", *(absent)* = "no requirement"
- ✅ **Cross-reference**: DD-WORKFLOW-001 v1.8, DD-HAPI-001

**Breaking Changes**:
- All examples updated to snake_case field names
- SQL queries use structured columns for mandatory labels
- CustomLabels use subdomain format: `map[subdomain][]string`
- DetectedLabels string fields now support wildcard matching

### Version 2.1 (2025-11-30)
**ALIGNMENT**: Updated to DD-WORKFLOW-001 v1.8 (5 mandatory labels, snake_case API fields, risk_tolerance customer-derived).

**Changes**:
- ✅ **5 Mandatory Labels**: Reduced from 6 to 5 (removed `risk_tolerance` from mandatory)
- ✅ **Label Taxonomy Clarified**: Group A (auto-populated) + Group B (Rego-configurable)
- ✅ **DetectedLabels (V1.0)**: Auto-detected cluster characteristics (GitOps, PDB, HPA, etc.)
- ✅ **CustomLabels (V1.0)**: Customer-defined via Rego policies (includes `risk_tolerance`, `business_category`)
- ✅ **Pass-Through Principle**: Kubernaut doesn't validate DetectedLabels/CustomLabels values
- ✅ **Cross-reference**: HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md v3.0

**Rationale**:
- `risk_tolerance` and `business_category` are organization-specific, customer-derived via Rego
- DetectedLabels provide auto-detected context without configuration
- CustomLabels are examples only - customers define their own keys and values

### Version 2.0 (2025-11-27)
**CRITICAL REVISION**: Removed hardcoded boost/penalty logic for optional labels.

**Changes**:
- ✅ **V1.0 Simplified**: Base semantic similarity only (no boost/penalty)
- ✅ **Removed hardcoded labels**: `resource-management`, `gitops-tool`, etc. are NOT Kubernaut-enforced
- ✅ **Clarified label architecture**: Only mandatory labels are Kubernaut-defined; custom labels are customer-defined
- ✅ **Future roadmap**: Configurable label weights deferred to V2.0+

**Rationale**:
- Custom labels (keys AND values) are defined by customers via Rego policies
- Customers match their Rego-defined labels against workflow labels
- Kubernaut cannot hardcode label names that vary per customer environment
- V1.0 uses base semantic similarity with mandatory label filtering

**Breaking Changes**:
- Removed `label_boost` and `label_penalty` from V1.0 scoring
- Removed hardcoded boost/penalty weights from implementation
- `confidence` score now equals `base_similarity` in V1.0

### Version 1.1 (2025-11-22)
- Renumbered from DD-WORKFLOW-003 to DD-WORKFLOW-004

### Version 1.0 (2025-11-22)
- Initial version with hardcoded boost/penalty logic (SUPERSEDED)

---

## 🔗 **Workflow Immutability Reference**

**CRITICAL**: Workflow labels used in scoring are immutable.

**Authority**: **DD-WORKFLOW-012: Workflow Immutability Constraints**
- Workflow labels are immutable (cannot change after creation)
- Label scoring relies on stable label values
- To change labels, create a new workflow version

**Cross-Reference**: All hybrid scoring logic assumes workflow label immutability per DD-WORKFLOW-012.

---

**Note**: Renumbered from DD-WORKFLOW-003 to DD-WORKFLOW-004 to resolve conflict with DD-WORKFLOW-003-parameterized-actions.md (created November 15, 2025).

---

## Executive Summary

**Decision**: Implement a **Two-Phase Semantic Search** approach for workflow selection:

### V1.5 (Current - Fixed DetectedLabel Weights) **← IMPLEMENTED 2025-12-11**
1. **Phase 1: Strict Filtering** for mandatory labels (`signal_type`, `severity`, + other mandatory labels) - mismatch = exclude workflow
2. **Phase 2: Hybrid Scoring** using pgvector cosine similarity + fixed DetectedLabel boost/penalty
   - `confidence = LEAST(base_similarity + label_boost - label_penalty, 1.0)`
   - **Fixed weights**: gitOpsManaged (0.10), pdbProtected (0.05), serviceMesh (0.05), etc.
   - **No configuration required**: Weights hard-coded based on correctness impact

### V2.0+ (Future - Configurable Label Weights)
3. **Configurable Boost/Penalty** via ConfigMap - customers can customize weights per their environment

**Key Insight**: Custom labels (both keys AND values) are **customer-defined** via Rego policies and matched against workflow labels. Kubernaut enforces only **5 mandatory labels** (DD-WORKFLOW-001 v1.8). Additionally, **DetectedLabels** are auto-populated (V1.0) and **CustomLabels** (including `risk_tolerance`) are user-defined via Rego (V1.0). Any boost/penalty logic for custom labels requires customer configuration, which is deferred to V2.0+.

---

## 🏗️ **Label Architecture (DD-WORKFLOW-001 v1.8)**

### Label Taxonomy Overview

| Category | Source | Config Required | Examples |
|----------|--------|-----------------|----------|
| **5 Mandatory Labels** | Signal Processing | No (auto/Rego defaults) | `signal_type`, `severity`, `component`, `environment`, `priority` |
| **DetectedLabels** | Auto-detection from K8s | ❌ No config | `GitOpsManaged`, `PDBProtected`, `HPAEnabled`, `ServiceMesh` |
| **CustomLabels** | Customer Rego policies | ✅ User-defined | `risk_tolerance`, `business_category`, `team`, `region` |

**Pass-Through Principle**: Kubernaut does NOT validate DetectedLabels or CustomLabels values - they are passed through to Data Storage for workflow matching. Workflow blueprints define which labels they accept.

### Kubernaut-Enforced Labels (5 Mandatory)

Per **DD-WORKFLOW-001 v1.8**, these labels are Kubernaut-defined with fixed keys:

#### Group A: Auto-Populated (from K8s/Prometheus)

| # | Label | Type | Wildcard | Description |
|---|-------|------|----------|-------------|
| 1 | `signal_type` | TEXT | ❌ NO | What happened (OOMKilled, CrashLoopBackOff) |
| 2 | `severity` | ENUM | ❌ NO | How bad (critical, high, medium, low) |
| 3 | `component` | TEXT | ❌ NO | What resource (pod, deployment, node) |

#### Group B: Rego-Configurable (users can customize derivation)

| # | Label | Type | Wildcard | Description |
|---|-------|------|----------|-------------|
| 4 | `environment` | ENUM | ✅ YES | Where (production, staging, development, test, '*') |
| 5 | `priority` | ENUM | ✅ YES | Business priority (P0, P1, P2, P3, '*') |

### DetectedLabels (V1.0 - Auto-Detected)

SignalProcessing auto-detects these from K8s resources (NO config required):

| Field | Type | Wildcard | Detection Method | Used For |
|-------|------|----------|------------------|----------|
| `gitOpsManaged` | bool | ❌ No | ArgoCD/Flux annotations | LLM context |
| `gitOpsTool` | string | ✅ `"*"` | Specific annotation patterns | Workflow selection |
| `pdbProtected` | bool | ❌ No | PDB exists for workload | Risk assessment |
| `hpaEnabled` | bool | ❌ No | HPA targets workload | Scaling context |
| `stateful` | bool | ❌ No | StatefulSet or PVC | State handling |
| `helmManaged` | bool | ❌ No | Helm labels present | Deployment method |
| `networkIsolated` | bool | ❌ No | NetworkPolicy exists | Security context |
| `podSecurityLevel` | string | ✅ `"*"` | Namespace PSS label | Security posture |
| `serviceMesh` | string | ✅ `"*"` | Istio/Linkerd sidecar | Traffic management |

**Wildcard Matching for String Fields** (per DD-WORKFLOW-001 v1.8):

| Workflow Specifies | Signal Has Value | Signal Absent | Meaning |
|--------------------|------------------|---------------|---------|
| `"argocd"` | ✅ if `argocd` | ❌ No | "I only support ArgoCD" |
| `"*"` | ✅ Any value | ❌ No | "I support any GitOps tool (but require one)" |
| *(absent)* | ✅ Any value | ✅ Yes | "I have no GitOps requirement" (generic) |

### CustomLabels (V1.0 - Subdomain-Based)

- **Format**: `<subdomain>.kubernaut.io/<key>[:<value>]`
- **Extraction**: Subdomain becomes filter key; value is boolean key or `key=value`
- **Storage**: `map[string][]string` (subdomain → list of values)
- **Matching**: Each subdomain becomes a separate WHERE clause in Data Storage
- **Kubernaut Role**: Pass-through conduit; do NOT define label names or weights

**Reference**: [HANDOFF_CUSTOM_LABELS_EXTRACTION_V1.md](../../services/crd-controllers/01-signalprocessing/HANDOFF_CUSTOM_LABELS_EXTRACTION_V1.md)

### Why No Hardcoded Boost/Penalty in V1.0

| Aspect | V1.0 (DD-WORKFLOW-004 v1.x) | V2.0+ (Future) |
|--------|------------------------------|------------------------------|
| **Label Keys** | Hardcoded (`resource-management`, `gitops-tool`) | Customer-defined via Rego |
| **Weights** | Fixed (0.10, 0.05) | Customer-configurable |
| **Problem** | Labels vary per customer environment | Requires configuration system |
| **Solution** | Remove boost/penalty | Defer to V2.0+ |

---

## Problem Statement

### The Challenge: Multi-Tier Label Architecture

**Scenario**: A signal is detected for a payment service in production that is managed by GitOps (ArgoCD).

**Labels Detected** (multi-tier):
- `signal_type`: "OOMKilled" (from LLM RCA) - **Mandatory (Group A)**
- `severity`: "critical" (from LLM RCA) - **Mandatory (Group A)**
- `component`: "pod" (from K8s) - **Mandatory (Group A)**
- `environment`: "production" (from Rego) - **Mandatory (Group B)**
- `priority`: "P0" (from Rego) - **Mandatory (Group B)**
- `GitOpsManaged`: true (auto-detected) - **DetectedLabels**
- `GitOpsTool`: "argocd" (auto-detected) - **DetectedLabels**
- `risk_tolerance`: "low" (from Rego) - **CustomLabels (customer-derived)**
- `PDBProtected`: true (auto-detected) - **DetectedLabels**
- `business_category`: "payment-service" (from Rego) - **CustomLabels**
- `region`: "us-east-1" (from Rego) - **CustomLabels**
- `team`: "platform" (from Rego) - **CustomLabels**

**Key Insight**:
- The **5 mandatory labels** are Kubernaut-enforced (DD-WORKFLOW-001 v1.8)
- **DetectedLabels** are auto-detected from K8s without configuration (V1.0 priority)
- **CustomLabels** like `risk_tolerance`, `business_category`, `region`, `team` are **user-defined** via Rego (V1.0)
- Both label **keys** and **values** for CustomLabels are customer-controlled
- **Naming convention**: `kubernaut.io/*`, `constraint.kubernaut.io/*`, `custom.kubernaut.io/*`
- Kubernaut cannot hardcode boost/penalty weights for labels that vary per customer

**V1.0 Solution**: Use base semantic similarity with mandatory label filtering + DetectedLabels context.

**V2.0+ Solution**: Allow customers to configure label weights via configuration.

---

## Decision: Two-Phase Semantic Search (V1.0)

### Architecture Overview (V1.0 - Base Similarity Only)

```
┌─────────────────────────────────────────────────────────────────┐
│ Phase 1: Strict Filtering (Mandatory Labels)                   │
│                                                                  │
│ WHERE signal_type = $signal_type                               │
│   AND severity = $severity                                     │
│   AND (environment = $environment OR environment = '*')        │
│   AND (1 - (embedding <=> $embedding)) >= $min_similarity      │
│                                                                  │
│ Result: Workflows matching mandatory labels                     │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ Phase 2: Semantic Ranking (Base Similarity)                    │
│                                                                  │
│ confidence = (1 - (embedding <=> $embedding))                  │
│                                                                  │
│ V1.0: NO boost/penalty for custom labels                       │
│ V2.0+: Configurable boost/penalty (future)                     │
│                                                                  │
│ ORDER BY confidence DESC                                        │
└─────────────────────────────────────────────────────────────────┘
```

---

## Label Classification (V1.0)

### Mandatory Labels (Strict Filtering)

| Label | Source | Behavior |
|-------|--------|----------|
| `signal_type` | LLM RCA | **EXCLUDE** if mismatch |
| `severity` | LLM RCA | **EXCLUDE** if mismatch |
| `component` | Signal Processing | Stored but NOT used as filter |
| `environment` | Signal Processing | **EXCLUDE** if mismatch (wildcard '*' matches any) |
| `priority` | Signal Processing | **EXCLUDE** if mismatch (wildcard '*' matches any) |

**Note**: `risk_tolerance` and `business_category` are now **CustomLabels** (per DD-WORKFLOW-001 v1.8).

**Behavior**: 5 mandatory labels provide strict pre-filtering before semantic ranking.

### Custom Labels (V1.0 - No Scoring Impact)

| Aspect | V1.0 Behavior |
|--------|---------------|
| **Matching** | Labels matched via Rego policy → workflow labels |
| **Boost** | ❌ No boost (deferred to V2.0+) |
| **Penalty** | ❌ No penalty (deferred to V2.0+) |
| **Scoring** | `confidence = base_similarity` only |

**Rationale**: Custom label keys are customer-defined; Kubernaut cannot assign weights to unknown labels.

---

## SQL Implementation (V1.0 - Base Similarity Only)

### Query Structure

```sql
-- V1.0: Two-Phase Semantic Search (Base Similarity Only)
-- Authority: DD-WORKFLOW-004 v2.1, DD-WORKFLOW-001 v1.8
--
-- Phase 1: Strict filtering on 5 mandatory labels (structured columns)
-- Phase 2: Semantic ranking by cosine similarity
--
-- NOTE: No boost/penalty logic in V1.0
--       Custom labels (risk_tolerance, business_category) are filtered via JSONB containment
--       Configurable weights deferred to V2.0+

SELECT
    id,
    workflow_id,
    version,
    name,
    description,
    embedding,
    -- Confidence = base semantic similarity (V1.0)
    (1 - (embedding <=> $embedding)) AS confidence
FROM remediation_workflow_catalog
WHERE status = 'active'
  AND is_latest_version = true
  -- PHASE 1: Strict Filtering (5 Mandatory Labels - structured columns)
  AND signal_type = $signal_type
  AND severity = $severity
  AND component = $component
  AND (environment = $environment OR environment = '*')
  AND (priority = $priority OR priority = '*')
  -- Custom labels (JSONB containment - includes risk_tolerance, business_category)
  AND (custom_labels @> $custom_labels OR $custom_labels IS NULL)
  -- Minimum semantic similarity threshold
  AND (1 - (embedding <=> $embedding)) >= $min_similarity
-- PHASE 2: Semantic Ranking
ORDER BY confidence DESC
LIMIT $top_k;
```

### V1.0 Scoring Formula

```
confidence = (1 - (embedding <=> query_embedding))
```

- **Range**: 0.0 to 1.0
- **Meaning**: Cosine similarity between query and workflow embeddings
- **No boost/penalty**: Custom labels do not affect scoring in V1.0

---

## Scoring Examples (V1.0)

### Example 1: Multiple Workflows with Same Mandatory Labels

**Query** (signal with auto-detected GitOps and PDB protection):
```json
{
  "query": "OOMKilled critical memory increase production",
  "filters": {
    "signal_type": "OOMKilled",
    "severity": "critical",
    "component": "pod",
    "environment": "production",
    "priority": "P0"
  },
  "detected_labels": {
    "gitOpsManaged": true,
    "gitOpsTool": "argocd",
    "pdbProtected": true,
    "helmManaged": true
  },
  "custom_labels": {
    "risk_tolerance": ["low"],
    "business_category": ["revenue-critical"]
  }
}
```

**Note**: `detected_labels` only includes booleans when `true` (per v1.5 Boolean Normalization). Fields like `hpaEnabled: false`, `stateful: false` are **omitted**.

**Workflow A** (GitOps workflow - better semantic match):
```json
{
  "name": "increase-memory-gitops-conservative-prod",
  "description": "OOMKilled critical: Increases memory limits via GitOps PR for production workloads",
  "signal_type": "OOMKilled",
  "severity": "critical",
  "component": "pod",
  "environment": "production",
  "priority": "P0",
  "custom_labels": {
    "risk_tolerance": ["low"],
    "business_category": ["revenue-critical"],
    "constraint": ["gitops-only"]
  }
}
```

**Workflow B** (Manual workflow - less semantic match):
```json
{
  "name": "increase-memory-manual-kubectl",
  "description": "OOMKilled critical: Increases memory limits via kubectl patch",
  "signal_type": "OOMKilled",
  "severity": "critical",
  "component": "pod",
  "environment": "production",
  "priority": "P0",
  "custom_labels": {
    "risk_tolerance": ["low"],
    "business_category": ["revenue-critical"]
  }
}
```

**V1.0 Score Calculation**:
- **Workflow A**: confidence = 0.92 (higher semantic similarity to query)
- **Workflow B**: confidence = 0.88 (lower semantic similarity)
- **Result**: Workflow A ranked first based on semantic similarity alone

**Note**: In V1.0, the `detected_labels` and `custom_labels` do NOT affect scoring. Ranking is purely by semantic similarity. Future V2.0+ may add configurable boost/penalty based on these labels.

---

### Example 2: Wildcard Matching

**Query**:
```json
{
  "filters": {
    "signal_type": "OOMKilled",
    "severity": "critical",
    "environment": "staging"
  }
}
```

**Workflow C** (Environment-specific):
```json
{
  "signal_type": "OOMKilled",
  "severity": "critical",
  "component": "pod",
  "environment": "staging",
  "priority": "P1"
}
```

**Workflow D** (Wildcard environment):
```json
{
  "signal_type": "OOMKilled",
  "severity": "critical",
  "component": "pod",
  "environment": "*",
  "priority": "*"
}
```

**V1.0 Behavior**:
- Both workflows pass mandatory label filtering (Workflow D via wildcard)
- Ranking determined by semantic similarity only
- Workflow C may rank higher if its description better matches the query

---

## V2.0+ Roadmap: Configurable Label Weights

### Future Enhancement (Not Implemented in V1.0)

**Problem**: Customers want to influence workflow ranking based on their custom labels.

**Solution**: Customer-configurable label weights via ConfigMap.

```yaml
# Example: Customer configuration (V2.0+)
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-label-weights
  namespace: kubernaut-system
data:
  label-weights.yaml: |
    # Customer-defined boost/penalty weights
    custom_labels:
      gitops-tool:
        boost: 0.10      # Matching gitops-tool adds 0.10
        penalty: 0.10    # Conflicting gitops-tool subtracts 0.10
      region:
        boost: 0.05      # Matching region adds 0.05
        penalty: 0.0     # No penalty for region mismatch
      team:
        boost: 0.05      # Matching team adds 0.05
        penalty: 0.0     # No penalty for team mismatch
```

**V2.0+ Scoring Formula**:
```
confidence = LEAST(base_similarity + label_boost - label_penalty, 1.0)

where:
  label_boost = SUM(weight for each matching custom label)
  label_penalty = SUM(penalty for each conflicting custom label)
```

**Implementation Status**:
- ⏳ **Pending customer feedback** on V1.0 base similarity approach
- ⏳ **Pending design** of configuration schema and validation
- ⏳ **Pending implementation** of dynamic SQL generation from configuration

---

## Label Detection Strategy

### Customer-Defined Labels via Rego Policies

Labels are **customer-defined** and **detected** by the **Signal Processing Service** using **Rego policies**.

**Key Principle**: Customers define BOTH the label keys AND values in their Rego policies. Kubernaut matches these labels against workflow labels but does NOT define them.

### Mandatory Labels (Kubernaut-Enforced)

These **5 mandatory labels** have fixed keys defined by Kubernaut (DD-WORKFLOW-001 v1.8):

| Label | Detection Source | Detection Method |
|-------|------------------|------------------|
| `signal_type` | LLM RCA | LLM determines from investigation |
| `severity` | LLM RCA | LLM assesses from impact analysis |
| `component` | Resource type | Kubernetes resource kind |
| `environment` | Namespace label | `namespace.labels["environment"]` |
| `priority` | Derived (Rego policy) | Calculated from context |

**Note**: `risk_tolerance` and `business_category` are now **CustomLabels** (customer-defined via Rego).

### Custom Labels (Customer-Defined)

Customers define additional labels via Rego policies. Examples:

| Label Key (Customer-Defined) | Detection Source | Example Value |
|------------------------------|------------------|---------------|
| `risk_tolerance` | Derived (Rego policy) | `["low"]`, `["medium"]`, `["high"]` |
| `business_category` | Namespace label | `["payment-service"]`, `["analytics"]` |
| `gitops_tool` | Deployment annotation | `["argocd"]`, `["flux"]` |
| `region` | Namespace label | `["us-east-1"]`, `["eu-west-1"]` |
| `team` | Namespace label | `["platform"]`, `["payments"]` |

**Note**: CustomLabels use the subdomain format per DD-WORKFLOW-001 v1.8: `map[subdomain][]string`

### Example Rego Policy (Customer-Provided)

```rego
package signalprocessing.labels

# Mandatory labels (Kubernaut-enforced keys)
environment = env {
    env := input.namespace.labels["environment"]
    env != ""
}

priority = "P0" {
    input.severity == "critical"
    environment == "production"
}

# Custom labels (customer-defined keys - stored in custom_labels JSONB)
# Format: map[subdomain][]string per DD-WORKFLOW-001 v1.5

custom_labels["risk_tolerance"] = ["low"] {
    priority == "P0"
    environment == "production"
}

custom_labels["risk_tolerance"] = ["high"] {
    environment in ["staging", "development", "test"]
}

custom_labels["gitops_tool"] = ["argocd"] {
    input.deployment.annotations["argocd.argoproj.io/instance"]
}

custom_labels["gitops_tool"] = ["flux"] {
    input.deployment.annotations["flux.fluxcd.io/sync-checksum"]
}

custom_labels["region"] = [input.namespace.labels["region"]] {
    input.namespace.labels["region"]
}

custom_labels["team"] = [input.namespace.labels["team"]] {
    input.namespace.labels["team"]
}
```

**Note**: The Rego policy structure is customer-defined. Kubernaut provides examples but customers customize for their environment.

---

## Integration Points

### 1. Signal Processing Service (Label Detection)

**Responsibility**: Detect all optional labels using Rego policies after K8s context enrichment.

**Flow**:
1. Signal Processing enriches K8s context (~2s)
2. Rego policy evaluates enriched context → detects labels
3. Labels stored in `RemediationProcessing.Status.DetectedLabels`

**Implementation**: `pkg/signalprocessing/enrichment/label_detector.go`

---

### 2. AIAnalysis Service (Label Pass-Through)

**Responsibility**: Pass detected labels from Signal Processing to HolmesGPT API for workflow search.

**Flow**:
1. AIAnalysis receives `RemediationProcessing` CRD with detected labels
2. LLM performs RCA → determines `signal_type` and `severity`
3. AIAnalysis combines LLM-determined labels + detected labels
4. Calls Data Storage workflow search with ALL labels (custom_labels auto-appended per DD-HAPI-001)

**Implementation**: `holmesgpt-api/src/extensions/recovery.py`

```python
def search_workflows(context, rca_findings):
    # LLM-determined mandatory labels (snake_case per DD-WORKFLOW-001 v1.8)
    mandatory_labels = {
        "signal_type": rca_findings["signal_type"],
        "severity": rca_findings["severity"],
        "component": context.get("component", "pod"),
        "environment": context.get("environment", "production"),
        "priority": context.get("priority", "P0")
    }

    # Custom labels (from Signal Processing enrichment_results)
    # Auto-appended to workflow search per DD-HAPI-001
    custom_labels = context.get("enrichment_results", {}).get("customLabels", {})

    # Combine for semantic query
    search_params = {
        "query": f"{mandatory_labels['signal_type']} {mandatory_labels['severity']}",
        "filters": mandatory_labels,
        "custom_labels": custom_labels  # Auto-appended by WorkflowCatalogToolset
    }

    return mcp_client.search_workflow_catalog(**search_params)
```

---

### 3. Data Storage Service (Weighted Scoring)

**Responsibility**: Execute hybrid weighted scoring SQL query.

**Flow**:
1. Receive workflow search request with mandatory + optional labels
2. Execute Phase 1: Strict filtering (mandatory labels)
3. Execute Phase 2: Weighted scoring (optional labels)
4. Return top-K workflows ordered by final score

**Implementation**: `pkg/datastorage/repository/workflow_repository.go`

```go
func (r *WorkflowRepository) SearchWorkflows(ctx context.Context, filters *WorkflowSearchFilters) ([]*models.Workflow, error) {
    // V1.0: Base similarity only (no boost/penalty)
    // Authority: DD-WORKFLOW-004 v2.1, DD-WORKFLOW-001 v1.8
    query := `
        SELECT
            id,
            workflow_id,
            version,
            name,
            description,
            embedding,
            (1 - (embedding <=> $1)) AS confidence
        FROM remediation_workflow_catalog
        WHERE status = 'active'
          AND is_latest_version = true
          -- 5 Mandatory labels (structured columns, snake_case)
          AND signal_type = $2
          AND severity = $3
          AND component = $4
          AND (environment = $5 OR environment = '*')
          AND (priority = $6 OR priority = '*')
          -- Custom labels (JSONB containment)
          AND (custom_labels @> $7 OR $7 IS NULL)
          AND (1 - (embedding <=> $1)) >= $8
        ORDER BY confidence DESC
        LIMIT $5
    `

    // Execute query with all parameters
    rows, err := r.db.QueryContext(ctx, query,
        filters.Embedding,
        filters.SignalType,
        filters.Severity,
        filters.MinSimilarity,
        filters.TopK)

    // ... parse results
}
```

---

## Benefits

### 1. Context-Aware Workflow Selection ✅

**Problem**: Generic workflows may not be appropriate for specific contexts (e.g., GitOps-managed resources).

**Solution**: Workflows with matching optional labels score higher, ensuring context-appropriate selection.

**Example**: A GitOps workflow scores 1.0 for a GitOps-managed resource, while a manual workflow scores 0.78 (penalty for conflict).

---

### 2. Graceful Degradation ✅

**Problem**: Not all environments have rich metadata (e.g., no GitOps annotations).

**Solution**: Generic workflows (no optional labels) are still selectable as fallbacks, scoring based on semantic similarity only.

**Example**: A generic "increase-memory" workflow scores 0.85 when no optional labels are provided.

---

### 3. Conflict Avoidance ✅

**Problem**: Selecting an inappropriate workflow (e.g., manual remediation for GitOps-managed resource) causes failures.

**Solution**: Conflicting optional labels apply penalties, reducing the likelihood of inappropriate workflow selection.

**Example**: A manual workflow scores 0.78 (penalty) vs. a GitOps workflow scoring 1.0 (boost).

---

### 4. Dynamic Adaptability ✅

**Problem**: Kubernetes environments are dynamic (labels change, new annotations added).

**Solution**: Rego policies detect labels dynamically at runtime, no code changes needed.

**Example**: Adding a new `sla-tier` label to namespaces automatically affects workflow scoring via Rego policy updates.

---

## Confidence Assessment

**Confidence**: 95% (V1.0 Base Similarity Approach)

**Evidence**:
- ✅ Aligns with DD-WORKFLOW-001 v1.8 (5 mandatory labels + DetectedLabels + CustomLabels, snake_case)
- ✅ Aligns with DD-LLM-001 (structured query format with exact labels)
- ✅ Respects customer-defined labels (no hardcoded assumptions)
- ✅ Simple implementation (base similarity only)
- ✅ Graceful degradation (generic workflows as fallbacks)
- ✅ Clear upgrade path (V2.0+ configurable weights)

**V1.0 Approach Benefits**:
- ✅ **Simplicity**: No complex boost/penalty logic
- ✅ **Flexibility**: Works with any customer label conventions
- ✅ **Maintainability**: Less code to maintain and debug
- ✅ **Correctness**: No hardcoded labels that may not exist in customer environment

**V2.0+ Considerations**:
- ⏳ **Customer Feedback**: Collect feedback on V1.0 base similarity approach
- ⏳ **Configuration Design**: Design customer-configurable weight schema
- ⏳ **Dynamic SQL**: Implement dynamic SQL generation from configuration

---

## Implementation Plan (V1.0)

### Phase 1: Remove Hardcoded Boost/Penalty Logic (1 hour)

**Tasks**:
1. Remove boost/penalty calculation from `workflow_repository.go`
2. Simplify SQL to use base similarity only
3. Update `WorkflowSearchResult` to remove `LabelBoost` and `LabelPenalty` fields
4. Update E2E tests to reflect base similarity scoring

**Files**:
- `pkg/datastorage/repository/workflow_repository.go`
- `pkg/datastorage/models/workflow.go`
- `test/e2e/datastorage/04_workflow_search_test.go`

---

### Phase 2: Update Documentation (30 minutes)

**Tasks**:
1. Update DD-WORKFLOW-004 (this document) ✅
2. Update DD-WORKFLOW-002 to reference V1.0 approach
3. Update implementation plan for audit trail

**Files**:
- `docs/architecture/decisions/DD-WORKFLOW-004-hybrid-weighted-label-scoring.md` ✅
- `docs/architecture/decisions/DD-WORKFLOW-002-MCP-WORKFLOW-CATALOG-ARCHITECTURE.md`
- `docs/services/stateless/data-storage/DD-AUDIT-023-030-WORKFLOW-SEARCH-AUDIT-IMPLEMENTATION-PLAN-V1.0.md`

---

### Phase 3: Update Audit Trail (30 minutes)

**Tasks**:
1. Update audit event to capture `confidence` only (no breakdown)
2. Remove `boost_breakdown` and `penalty_breakdown` from audit schema
3. Update implementation plan to reflect V1.0 approach

**Files**:
- `docs/services/stateless/data-storage/DD-AUDIT-023-030-WORKFLOW-SEARCH-AUDIT-IMPLEMENTATION-PLAN-V1.0.md`

---

## Cross-References

**Related Design Decisions**:
- **DD-CATEGORIZATION-001**: Gateway → Signal Processing categorization consolidation
- **DD-LLM-001**: MCP Workflow Search Parameter Taxonomy (structured query format)
- **DD-WORKFLOW-002 v2.0**: MCP Workflow Catalog Architecture (two-phase filtering)

**Related Business Requirements**:
- **BR-SP-051 to 053**: Signal Processing environment classification
- **BR-SP-070 to 072**: Signal Processing priority assignment (NEW)
- **BR-WORKFLOW-001**: Workflow catalog semantic search

**Related Implementation Documents**:
- `/tmp/rego-based-label-detection.md`: Complete Rego policy implementation
- `/tmp/label-filtering-strategy.md`: Weighted scoring analysis
- `/tmp/dynamic-label-detection-strategy.md`: Dynamic label detection strategy

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 2.0 | 2025-11-27 | **CRITICAL REVISION**: Removed hardcoded boost/penalty logic. V1.0 = base similarity only. Custom labels are customer-defined via Rego. Configurable weights deferred to V2.0+. |
| 1.1 | 2025-11-22 | Renumbered from DD-WORKFLOW-003 to DD-WORKFLOW-004 to resolve conflict with parameterized-actions DD |
| 1.0 | 2025-11-22 | Initial version: Hybrid weighted scoring strategy (SUPERSEDED) |

---

**Status**: ✅ **APPROVED** (V2.0)
**Confidence**: 95%
**Next Steps**:
1. Remove hardcoded boost/penalty logic from `workflow_repository.go`
2. Update E2E tests to reflect base similarity scoring
3. Update audit trail to capture `confidence` only (no breakdown)
4. Collect customer feedback on V1.0 approach for V2.0+ planning

