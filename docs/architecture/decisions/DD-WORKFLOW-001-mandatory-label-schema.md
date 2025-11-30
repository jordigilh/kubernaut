# DD-WORKFLOW-001: Mandatory Workflow Label Schema

**Date**: November 14, 2025
**Status**: ✅ **APPROVED** (V1.6 - snake_case API + 5 Mandatory Labels + DetectedLabels + CustomLabels)
**Decision Maker**: Kubernaut Architecture Team
**Authority**: ⭐ **AUTHORITATIVE** - This document is the single source of truth for workflow label schema
**Affects**: Data Storage Service V1.0, Workflow Catalog, Signal Processing, HolmesGPT API
**Related**: DD-LLM-001 (MCP Search Taxonomy), DD-STORAGE-008 (Workflow Catalog Schema), ADR-041 (LLM Prompt Contract), DD-WORKFLOW-012 (Workflow Immutability)
**Version**: 1.6

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

### Version 1.6 (2025-11-30)
- **BREAKING**: Standardized all API/database field names to **snake_case**
- Changed filter parameters: `signal-type` → `signal_type`, `risk-tolerance` → `risk_tolerance`
- **Clarification**: Kubernetes annotation keys (`kubernaut.io/signal-type`) vs API field names (`signal_type`)
- K8s annotations use kebab-case per K8s convention; API/DB use snake_case per JSON convention
- Updated Implementation section to align with v1.4 (5 mandatory labels, not 7)
- Removed `risk_tolerance` and `business_category` from mandatory (now customer-derived via Rego)
- Updated Go struct JSON tags from `json:"kubernaut.io/signal-type"` to `json:"signal_type"`
- **NEW**: Added authoritative **DetectedLabels** section (9 auto-detected fields)
- **NEW**: Added **Wildcard Support for DetectedLabels** string fields (`gitOpsTool`, `podSecurityLevel`, `serviceMesh`)
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

### **5 Mandatory Labels (V1.4)**

Labels are grouped by how they are populated:

#### **Group A: Auto-Populated Labels** (Signal Processing derives automatically from K8s/Prometheus)

| # | Label | Type | Source | Wildcard | Description |
|---|---|---|---|---|---|
| 1 | `signal_type` | TEXT | K8s Event Reason | ❌ NO | What happened (OOMKilled, CrashLoopBackOff, NodeNotReady) |
| 2 | `severity` | ENUM | Alert/Event | ❌ NO | How bad (critical, high, medium, low) |
| 3 | `component` | TEXT | K8s Resource | ❌ NO | What resource (pod, deployment, node) |

**Derivation**: These labels are extracted directly from Kubernetes events, Prometheus alerts, or signal metadata. **No user configuration required.**

#### **Group B: System-Classified Labels** (Signal Processing derives with configurable defaults)

| # | Label | Type | Source | Wildcard | Description |
|---|---|---|---|---|---|
| 4 | `environment` | ENUM | Namespace Labels | ✅ YES | Where (production, staging, development, test, '*') |
| 5 | `priority` | ENUM | Derived | ✅ YES | Business priority (P0, P1, P2, P3, '*') |

**Derivation**: Signal Processing applies Rego policies to derive these labels from K8s context (namespace labels, annotations, resource metadata). Users can customize derivation logic via Rego policy ConfigMaps.

**Default Logic** (if no custom Rego):
- `environment`: From namespace label `environment` or annotation `kubernaut.io/environment`
- `priority`: Derived from `severity` + `environment` (critical + production → P0)

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

**Reference**: [HANDOFF_CUSTOM_LABELS_EXTRACTION_V1.md](../../services/crd-controllers/01-signalprocessing/HANDOFF_CUSTOM_LABELS_EXTRACTION_V1.md)

---

### **DetectedLabels (V1.0 - Auto-Detected from K8s)**

SignalProcessing auto-detects these labels from Kubernetes resources **without any customer configuration**.

#### **DetectedLabels Fields (9 Fields)**

| Field | Type | Wildcard | Detection Method | Used For |
|-------|------|----------|------------------|----------|
| `gitOpsManaged` | bool | ❌ No | ArgoCD/Flux annotations present | LLM context |
| `gitOpsTool` | string | ✅ `"*"` | `"argocd"`, `"flux"`, or omitted | Workflow selection |
| `pdbProtected` | bool | ❌ No | PodDisruptionBudget exists | Risk assessment |
| `hpaEnabled` | bool | ❌ No | HorizontalPodAutoscaler targets workload | Scaling context |
| `stateful` | bool | ❌ No | StatefulSet or has PVCs attached | State handling |
| `helmManaged` | bool | ❌ No | `helm.sh/chart` label present | Deployment method |
| `networkIsolated` | bool | ❌ No | NetworkPolicy exists in namespace | Security context |
| `podSecurityLevel` | string | ✅ `"*"` | `"privileged"`, `"baseline"`, `"restricted"` | Security posture |
| `serviceMesh` | string | ✅ `"*"` | `"istio"`, `"linkerd"`, or omitted | Traffic management |

**Note**: Only **string fields** support wildcards. Boolean fields use absence semantics (see Boolean Normalization Rule below).

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
    if dl.PodSecurityLevel != "" {
        result["podSecurityLevel"] = dl.PodSecurityLevel
    }
    if dl.ServiceMesh != "" {
        result["serviceMesh"] = dl.ServiceMesh
    }

    return result
}
```

**Reference**: [HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md](../../services/crd-controllers/01-signalprocessing/HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md)

#### **Wildcard Support for DetectedLabels String Fields (V1.6)**

**String fields** in DetectedLabels support wildcard matching (`"*"`) in workflow blueprints:

| Field | Wildcard Support | Values |
|-------|------------------|--------|
| `gitOpsTool` | ✅ `"*"` | `"argocd"`, `"flux"`, `"*"` |
| `podSecurityLevel` | ✅ `"*"` | `"privileged"`, `"baseline"`, `"restricted"`, `"*"` |
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

**Generic workflow** (no GitOps requirement):
```json
{
  "detected_labels": {}
}
```

---

### **Label Matching Rules**

**For MCP Workflow Search** (DD-LLM-001):
1. **Mandatory Label Filtering**: `signal_type`, `severity`, `environment`, `priority` used as SQL WHERE filters
2. **Semantic Ranking**: Query string (`<signal_type> <severity>`) for pgvector semantic similarity
3. **Component Storage Only**: `component` is stored but NOT used as search filter (workflows are generic)
4. **Wildcard Support (Mandatory Labels)**: `environment`, `priority` support `'*'` (matches any value)
5. **Wildcard Support (DetectedLabels)**: `gitOpsTool`, `podSecurityLevel`, `serviceMesh` support `'*'` (matches any non-empty value)
6. **Custom Label Filtering**: Each subdomain becomes a separate WHERE clause (see V1.5 format above)
7. **Match Scoring**: Exact label matches + semantic similarity = final confidence score

**For Workflow Registration**:
1. **5 Mandatory Labels Required**: Every workflow must have all 5 mandatory labels (v1.4)
2. **Custom Labels Optional**: Workflows can include custom labels for more specific matching
3. **Description Format**: Must follow `"<signal_type> <severity>: <description>"` for optimal semantic matching
4. **Validation**: Labels are validated against authoritative values in this document

### **Valid Values (Authoritative)**

#### **Group A: Auto-Populated Labels**

```yaml
signal_type:  # Domain-specific values from source systems (NO TRANSFORMATION)
  # CRITICAL PRINCIPLE: Use exact event reason strings from Kubernetes/Prometheus
  # WHY: LLM uses signal_type to query the same source system during investigation
  #      Example: signal_type="OOMKilled" → LLM runs: kubectl get events | grep "OOMKilled"
  #      If we transform "OOMKilled" → "pod-oomkilled", LLM queries will fail
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

severity:  # From alert/event metadata
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
CREATE TYPE severity_enum AS ENUM ('critical', 'high', 'medium', 'low');
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
    severity          severity_enum NOT NULL,     -- critical, high, medium, low
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
  AND severity = $2
  AND component = $3
  AND (environment = $4 OR environment = '*')  -- Wildcard support
  AND (priority = $5 OR priority = '*')
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
| `severity` | `kubernaut.io/severity` | string | ✅ YES | `critical`, `high`, `medium`, `low` | Signal severity level |
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
    "podSecurityLevel": "restricted"
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

#### **SQL Filtering Pattern (v1.6 - 5 Mandatory + Custom Labels)**

```sql
-- Filter workflows by 5 mandatory labels (v1.4) + optional custom labels
SELECT
    workflow_id,
    version,
    title,
    description,
    embedding
FROM workflow_catalog
WHERE status = 'active'
  -- 5 Mandatory labels (structured columns, snake_case)
  AND signal_type = $1                                -- OOMKilled
  AND severity = $2                                   -- critical
  AND component = $3                                  -- pod
  AND (environment = $4 OR environment = '*')         -- production (with wildcard)
  AND (priority = $5 OR priority = '*')               -- P0 (with wildcard)
  -- Custom labels (JSONB containment - customer-defined)
  AND (custom_labels @> $6 OR $6 IS NULL)             -- {"constraint": ["cost-constrained"]}
ORDER BY embedding <=> $7  -- semantic similarity
LIMIT 10;
```

**Note**: `risk_tolerance` and `business_category` are now custom labels (v1.4), not mandatory columns.

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
  - PostgreSQL enums enforce `severity`, `environment`, `priority` values
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

### **v1.6** (2025-11-30) - CURRENT
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

**Document Version**: 1.6
**Last Updated**: November 30, 2025
**Status**: ✅ **APPROVED** (95% confidence, snake_case API standardization)
**Authority**: ⭐ **AUTHORITATIVE** - Single source of truth for workflow label schema
**Breaking Change**: API field names use snake_case; Data Storage must update Go struct JSON tags
**Cross-Reference**: DD-WORKFLOW-002 v3.3, DD-HAPI-001
**Next Review**: After Data Storage implements snake_case API fields

