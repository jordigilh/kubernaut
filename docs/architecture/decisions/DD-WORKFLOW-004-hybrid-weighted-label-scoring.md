# DD-WORKFLOW-004: Hybrid Weighted Label Scoring for Workflow Selection

**Date**: November 22, 2025
**Status**: ✅ **APPROVED**
**Confidence**: 90%
**Purpose**: Define the hybrid weighted scoring strategy for workflow catalog semantic search that combines strict filtering for mandatory labels with weighted scoring for optional, dynamically detected labels.

**Note**: Renumbered from DD-WORKFLOW-003 to DD-WORKFLOW-004 to resolve conflict with DD-WORKFLOW-003-parameterized-actions.md (created November 15, 2025).

---

## Executive Summary

**Decision**: Implement a **Hybrid Weighted Scoring** approach for workflow selection that combines:
1. **Strict Filtering** for mandatory labels (`signal-type`, `severity`) - mismatch = exclude workflow
2. **Weighted Scoring** for optional labels (`resource-management`, `gitops-tool`, `environment`, `business-category`, `priority`, `risk-tolerance`) - match = boost score
3. **Mismatch Penalties** for conflicting optional labels - workflow has different value = penalty

**Key Insight**: Labels are **dynamically detected** from Kubernetes context (namespace labels, deployment annotations, cluster metadata), so workflows should score higher when optional labels match, but still be selectable as generic fallbacks when labels are absent.

---

## Problem Statement

### The Challenge: Dynamic and Optional Labels

**Scenario**: A signal is detected for a payment service in production that is managed by GitOps (ArgoCD).

**Labels Detected**:
- `signal-type`: "OOMKilled" (from LLM RCA)
- `severity`: "critical" (from LLM RCA)
- `environment`: "production" (from namespace label)
- `business-category`: "revenue-critical" (from namespace label)
- `resource-management`: "gitops" (from deployment annotation)
- `gitops-tool`: "argocd" (from deployment annotation)
- `priority`: "P0" (derived from business-category + severity)
- `risk-tolerance`: "low" (derived from environment)

**Problem**: Not all signals will have all labels. Some environments may not have:
- GitOps annotations
- Business category labels
- Explicit priority labels

**Question**: How should workflows be scored when:
1. A query provides optional labels (e.g., `resource-management=gitops`)
2. A workflow has matching optional labels (e.g., `resource-management=gitops`) → **BOOST**
3. A workflow lacks optional labels (generic fallback) → **NEUTRAL**
4. A workflow has conflicting optional labels (e.g., `resource-management=manual`) → **PENALTY**

---

## Decision: Hybrid Weighted Scoring

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│ Phase 1: Strict Filtering (Mandatory Labels)                   │
│                                                                  │
│ WHERE labels->>'signal-type' = $signal_type                    │
│   AND labels->>'severity' = $severity                          │
│   AND (1 - (embedding <=> $embedding)) >= $min_similarity      │
│                                                                  │
│ Result: Workflows that MUST match signal-type + severity       │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│ Phase 2: Weighted Scoring (Optional Labels)                    │
│                                                                  │
│ Base Score: (1 - (embedding <=> $embedding))                   │
│                                                                  │
│ Boosts (if query label matches workflow label):                │
│   + 0.10 if resource-management matches                        │
│   + 0.05 if gitops-tool matches                                │
│   + 0.05 if environment matches                                │
│   + 0.05 if business-category matches                          │
│   + 0.05 if priority matches                                   │
│   + 0.05 if risk-tolerance matches                             │
│                                                                  │
│ Penalties (if query label conflicts with workflow label):      │
│   - 0.10 if resource-management conflicts                      │
│   - 0.05 if gitops-tool conflicts                              │
│   - 0.05 if environment conflicts                              │
│   - 0.05 if business-category conflicts                        │
│   - 0.05 if priority conflicts                                 │
│   - 0.05 if risk-tolerance conflicts                           │
│                                                                  │
│ Final Score: LEAST(base_score + boosts - penalties, 1.0)       │
└─────────────────────────────────────────────────────────────────┘
```

---

## Label Classification

### Mandatory Labels (Strict Filtering)

| Label | Source | Rationale |
|-------|--------|-----------|
| `signal-type` | LLM RCA | Core signal identification (e.g., "OOMKilled", "CrashLoopBackOff") |
| `severity` | LLM RCA | Criticality assessment (e.g., "critical", "high", "medium", "low") |

**Behavior**: If a workflow's `signal-type` or `severity` doesn't match the query, it is **EXCLUDED** from results.

### Optional Labels (Weighted Scoring)

| Label | Source | Boost | Penalty | Rationale |
|-------|--------|-------|---------|-----------|
| `resource-management` | Deployment annotation | +0.10 | -0.10 | High impact: GitOps vs manual remediation is fundamentally different |
| `gitops-tool` | Deployment annotation | +0.05 | -0.05 | Medium impact: ArgoCD vs Flux have different APIs |
| `environment` | Namespace label | +0.05 | -0.05 | Medium impact: Production vs staging may have different risk tolerance |
| `business-category` | Namespace label | +0.05 | -0.05 | Medium impact: Revenue-critical services need faster remediation |
| `priority` | Derived (Rego policy) | +0.05 | -0.05 | Medium impact: P0 vs P3 affects remediation urgency |
| `risk-tolerance` | Derived (environment) | +0.05 | -0.05 | Medium impact: Low risk tolerance = conservative remediation |

**Behavior**:
- **Match**: Workflow has the same label value → **BOOST** score
- **Absent**: Workflow doesn't have the label → **NEUTRAL** (no boost, no penalty) - workflow is a generic fallback
- **Conflict**: Workflow has a different label value → **PENALTY** (workflow is inappropriate for this context)

---

## SQL Implementation

### Query Structure

```sql
WITH label_scores AS (
    SELECT
        id,
        workflow_id,
        version,
        name,
        description,
        labels,
        embedding,
        -- Base similarity score (pgvector cosine similarity)
        (1 - (embedding <=> $embedding)) AS base_similarity,

        -- Calculate label boost (matches increase score)
        (
            -- resource-management boost (+0.10 if matches)
            CASE
                WHEN $resource_management IS NOT NULL
                     AND labels->>'resource-management' = $resource_management
                THEN 0.10
                ELSE 0
            END +

            -- gitops-tool boost (+0.05 if matches)
            CASE
                WHEN $gitops_tool IS NOT NULL
                     AND labels->>'gitops-tool' = $gitops_tool
                THEN 0.05
                ELSE 0
            END +

            -- environment boost (+0.05 if matches)
            CASE
                WHEN $environment IS NOT NULL
                     AND labels->>'environment' = $environment
                THEN 0.05
                ELSE 0
            END +

            -- business-category boost (+0.05 if matches)
            CASE
                WHEN $business_category IS NOT NULL
                     AND labels->>'business-category' = $business_category
                THEN 0.05
                ELSE 0
            END +

            -- priority boost (+0.05 if matches)
            CASE
                WHEN $priority IS NOT NULL
                     AND labels->>'priority' = $priority
                THEN 0.05
                ELSE 0
            END +

            -- risk-tolerance boost (+0.05 if matches)
            CASE
                WHEN $risk_tolerance IS NOT NULL
                     AND labels->>'risk-tolerance' = $risk_tolerance
                THEN 0.05
                ELSE 0
            END
        ) AS label_boost,

        -- Calculate label penalty (conflicts decrease score)
        (
            -- resource-management penalty (-0.10 if conflicts)
            CASE
                WHEN $resource_management IS NOT NULL
                     AND labels->>'resource-management' IS NOT NULL
                     AND labels->>'resource-management' != $resource_management
                THEN 0.10
                ELSE 0
            END +

            -- gitops-tool penalty (-0.05 if conflicts)
            CASE
                WHEN $gitops_tool IS NOT NULL
                     AND labels->>'gitops-tool' IS NOT NULL
                     AND labels->>'gitops-tool' != $gitops_tool
                THEN 0.05
                ELSE 0
            END +

            -- environment penalty (-0.05 if conflicts)
            CASE
                WHEN $environment IS NOT NULL
                     AND labels->>'environment' IS NOT NULL
                     AND labels->>'environment' != $environment
                THEN 0.05
                ELSE 0
            END +

            -- business-category penalty (-0.05 if conflicts)
            CASE
                WHEN $business_category IS NOT NULL
                     AND labels->>'business-category' IS NOT NULL
                     AND labels->>'business-category' != $business_category
                THEN 0.05
                ELSE 0
            END +

            -- priority penalty (-0.05 if conflicts)
            CASE
                WHEN $priority IS NOT NULL
                     AND labels->>'priority' IS NOT NULL
                     AND labels->>'priority' != $priority
                THEN 0.05
                ELSE 0
            END +

            -- risk-tolerance penalty (-0.05 if conflicts)
            CASE
                WHEN $risk_tolerance IS NOT NULL
                     AND labels->>'risk-tolerance' IS NOT NULL
                     AND labels->>'risk-tolerance' != $risk_tolerance
                THEN 0.05
                ELSE 0
            END
        ) AS label_penalty

    FROM remediation_workflow_catalog
    WHERE status = 'active'
      AND is_latest_version = true
      -- STRICT FILTERING: Mandatory labels MUST match
      AND labels->>'signal-type' = $signal_type
      AND labels->>'severity' = $severity
      -- Minimum semantic similarity threshold
      AND (1 - (embedding <=> $embedding)) >= $min_similarity
)
SELECT
    id,
    workflow_id,
    version,
    name,
    description,
    labels,
    base_similarity,
    label_boost,
    label_penalty,
    -- Final score: base + boosts - penalties, capped at 1.0
    LEAST(base_similarity + label_boost - label_penalty, 1.0) AS final_score
FROM label_scores
ORDER BY final_score DESC
LIMIT $top_k;
```

---

## Scoring Examples

### Example 1: Perfect Match (GitOps + Production + Revenue-Critical)

**Query**:
```json
{
  "query": "OOMKilled critical",
  "label.signal-type": "OOMKilled",
  "label.severity": "critical",
  "label.resource-management": "gitops",
  "label.gitops-tool": "argocd",
  "label.environment": "production",
  "label.business-category": "revenue-critical",
  "label.priority": "P0",
  "label.risk-tolerance": "low"
}
```

**Workflow A** (Specific GitOps workflow):
```json
{
  "name": "increase-memory-gitops-conservative-prod",
  "labels": {
    "signal-type": "OOMKilled",
    "severity": "critical",
    "resource-management": "gitops",
    "gitops-tool": "argocd",
    "environment": "production",
    "business-category": "revenue-critical",
    "priority": "P0",
    "risk-tolerance": "low"
  }
}
```

**Score Calculation**:
- Base similarity: 0.87
- Boosts: +0.10 (resource-management) +0.05 (gitops-tool) +0.05 (environment) +0.05 (business-category) +0.05 (priority) +0.05 (risk-tolerance) = **+0.35**
- Penalties: 0.00 (no conflicts)
- **Final Score: 0.87 + 0.35 = 1.22 → capped at 1.0**

---

### Example 2: Generic Fallback (No Optional Labels)

**Query**:
```json
{
  "query": "OOMKilled critical",
  "label.signal-type": "OOMKilled",
  "label.severity": "critical"
}
```

**Workflow B** (Generic workflow):
```json
{
  "name": "increase-memory-generic",
  "labels": {
    "signal-type": "OOMKilled",
    "severity": "critical"
  }
}
```

**Score Calculation**:
- Base similarity: 0.85
- Boosts: 0.00 (no optional labels to match)
- Penalties: 0.00 (no conflicts)
- **Final Score: 0.85**

**Result**: Workflow B is still selectable as a generic fallback, but scores lower than specific workflows.

---

### Example 3: Conflicting Labels (Manual vs GitOps)

**Query**:
```json
{
  "query": "OOMKilled critical",
  "label.signal-type": "OOMKilled",
  "label.severity": "critical",
  "label.resource-management": "gitops",
  "label.gitops-tool": "argocd"
}
```

**Workflow C** (Manual workflow):
```json
{
  "name": "increase-memory-manual-kubectl",
  "labels": {
    "signal-type": "OOMKilled",
    "severity": "critical",
    "resource-management": "manual"
  }
}
```

**Score Calculation**:
- Base similarity: 0.88
- Boosts: 0.00 (no matching optional labels)
- Penalties: -0.10 (resource-management conflict: "manual" vs "gitops")
- **Final Score: 0.88 - 0.10 = 0.78**

**Result**: Workflow C scores lower due to conflicting `resource-management` label, making it less likely to be selected.

---

## Dynamic Label Detection Strategy

### Label Sources

Labels are **dynamically detected** by the **Signal Processing Service** (per DD-CATEGORIZATION-001) using **Rego policies**.

**Why Signal Processing?**
- ✅ **Single Responsibility Principle**: Gateway focuses on fast ingestion (<50ms), Signal Processing on enrichment + categorization (~3s)
- ✅ **Reduces Gateway Response Time**: Removing categorization from Gateway improves ingestion latency by ~10ms
- ✅ **Full Context Availability**: Signal Processing has complete K8s context (~8KB) after enrichment
- ✅ **Sophisticated Categorization**: Can leverage deployment criticality, resource quotas, node health, business context (SLA, historical failures)

**Detection Sources**:

| Label | Detection Source | Detection Method |
|-------|------------------|------------------|
| `environment` | Namespace label | `namespace.labels["environment"]` |
| `business-category` | Namespace label | `namespace.labels["business-category"]` |
| `resource-management` | Deployment annotation | `deployment.annotations["argocd.argoproj.io/instance"]` (ArgoCD), `deployment.annotations["flux.fluxcd.io/sync-checksum"]` (Flux), `deployment.labels["helm.sh/chart"]` (Helm) |
| `gitops-tool` | Deployment annotation | `"argocd"` if ArgoCD annotation exists, `"flux"` if Flux annotation exists |
| `priority` | Derived (Rego policy) | Calculated from `business-category` + `severity` + `environment` |
| `risk-tolerance` | Derived (environment) | `"low"` for production, `"medium"` for staging, `"high"` for development |

### Rego Policy Integration

**File**: `pkg/signalprocessing/policies/label_detection.rego`

```rego
package signalprocessing.labels

# Environment detection (from namespace label)
environment = env {
    env := input.namespace.labels["environment"]
    env != ""
}

# Business category detection (from namespace label)
business_category = cat {
    cat := input.namespace.labels["business-category"]
    cat != ""
}

# Resource management detection (from deployment annotations)
resource_management = {"type": "gitops", "tool": "argocd", "confidence": 1.0} {
    input.deployment.annotations["argocd.argoproj.io/instance"]
}

resource_management = {"type": "gitops", "tool": "flux", "confidence": 1.0} {
    not input.deployment.annotations["argocd.argoproj.io/instance"]
    input.deployment.annotations["flux.fluxcd.io/sync-checksum"]
}

resource_management = {"type": "manual", "tool": null, "confidence": 0.8} {
    not input.deployment.annotations["argocd.argoproj.io/instance"]
    not input.deployment.annotations["flux.fluxcd.io/sync-checksum"]
    not input.deployment.labels["helm.sh/chart"]
}

# Priority derivation (from business context)
priority = "P0" {
    input.signal.severity == "critical"
    environment == "production"
    business_category == "revenue-critical"
}

# Risk tolerance derivation (from environment)
risk_tolerance = "low" {
    environment == "production"
}

# Aggregate all labels
labels = {
    "environment": environment,
    "business-category": business_category,
    "priority": priority,
    "risk-tolerance": risk_tolerance,
    "resource-management": resource_management.type,
    "gitops-tool": resource_management.tool
}
```

**Cross-Reference**: See `/tmp/rego-based-label-detection.md` for complete Rego policy implementation.

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
2. LLM performs RCA → determines `signal-type` and `severity`
3. AIAnalysis combines LLM-determined labels + detected labels
4. Calls Data Storage workflow search with ALL labels

**Implementation**: `holmesgpt-api/src/extensions/recovery.py`

```python
def search_workflows(context, rca_findings):
    # LLM-determined labels
    llm_labels = {
        "signal-type": rca_findings["signal_type"],
        "severity": rca_findings["severity"]
    }

    # Detected labels (from Signal Processing)
    detected_labels = context.get("detected_labels", {})

    # Combine ALL labels
    search_params = {
        "query": f"{llm_labels['signal-type']} {llm_labels['severity']}",
        **{f"label.{k}": v for k, v in llm_labels.items()},
        **{f"label.{k}": v for k, v in detected_labels.items()}
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
    query := `
        WITH label_scores AS (
            SELECT
                *,
                (1 - (embedding <=> $1)) AS base_similarity,
                (/* boost calculation */) AS label_boost,
                (/* penalty calculation */) AS label_penalty
            FROM remediation_workflow_catalog
            WHERE status = 'active'
              AND is_latest_version = true
              AND labels->>'signal-type' = $2
              AND labels->>'severity' = $3
              AND (1 - (embedding <=> $1)) >= $4
        )
        SELECT
            *,
            LEAST(base_similarity + label_boost - label_penalty, 1.0) AS final_score
        FROM label_scores
        ORDER BY final_score DESC
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

**Confidence**: 90%

**Evidence**:
- ✅ Aligns with DD-CATEGORIZATION-001 (Signal Processing owns categorization)
- ✅ Aligns with DD-LLM-001 (structured query format with exact labels)
- ✅ Solves user's GitOps example (manual workflow excluded, GitOps workflow selected)
- ✅ Supports dynamic label detection (no hardcoded assumptions)
- ✅ Graceful degradation (generic workflows as fallbacks)
- ✅ Conflict avoidance (penalties for mismatching labels)

**Risks**:
- ⚠️ **Complexity**: SQL query is more complex than strict filtering alone
- ⚠️ **Tuning**: Boost/penalty weights may need adjustment based on real-world usage
- ⚠️ **Performance**: Additional CASE statements in SQL query (mitigated by pgvector indexing)

**Mitigation**:
- Add metrics collection to monitor scoring distribution
- Implement A/B testing for boost/penalty weight tuning
- Profile SQL query performance with realistic data volumes

---

## Implementation Plan

### Phase 1: Update Label Schema (2-3 hours)

**Tasks**:
1. Update `WorkflowSearchFilters` model to include optional labels
2. Update `workflow_repository.go` to implement hybrid weighted scoring SQL
3. Update `workflow_handlers.go` to parse optional label parameters
4. Update migration `015_create_workflow_catalog_table.sql` to document label schema

**Files**:
- `pkg/datastorage/models/workflow.go`
- `pkg/datastorage/repository/workflow_repository.go`
- `pkg/datastorage/server/workflow_handlers.go`
- `migrations/015_create_workflow_catalog_table.sql`

---

### Phase 2: Implement Rego-Based Label Detection (3-4 hours)

**Tasks**:
1. Create `pkg/signalprocessing/policies/label_detection.rego`
2. Implement `pkg/signalprocessing/enrichment/label_detector.go`
3. Update `RemediationProcessing` CRD to include `DetectedLabels` field
4. Integrate label detection into Signal Processing enrichment flow

**Files**:
- `pkg/signalprocessing/policies/label_detection.rego` (NEW)
- `pkg/signalprocessing/enrichment/label_detector.go` (NEW)
- `api/remediationprocessing/v1alpha1/remediationprocessing_types.go`
- `pkg/signalprocessing/enrichment/enricher.go`

---

### Phase 3: Update AIAnalysis Integration (1-2 hours)

**Tasks**:
1. Update `holmesgpt-api/src/extensions/recovery.py` to pass detected labels
2. Update `holmesgpt-api/src/toolsets/workflow_catalog.py` to document optional labels
3. Add integration tests for label pass-through

**Files**:
- `holmesgpt-api/src/extensions/recovery.py`
- `holmesgpt-api/src/toolsets/workflow_catalog.py`
- `test/integration/datastorage/workflow_catalog_test.go`

---

### Phase 4: Metrics and Monitoring (1-2 hours)

**Tasks**:
1. Add Prometheus metrics for scoring distribution
2. Add logging for label detection confidence
3. Add dashboard for workflow selection analysis

**Metrics**:
- `workflow_search_label_boost_total{label="resource-management"}` (counter)
- `workflow_search_label_penalty_total{label="resource-management"}` (counter)
- `workflow_search_final_score` (histogram)
- `workflow_label_detection_confidence{label="environment"}` (gauge)

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
| 1.1 | 2025-11-22 | Renumbered from DD-WORKFLOW-003 to DD-WORKFLOW-004 to resolve conflict with parameterized-actions DD |
| 1.0 | 2025-11-22 | Initial version: Hybrid weighted scoring strategy |

---

**Status**: ✅ **APPROVED**
**Next Steps**: Proceed with Phase 1 implementation (update label schema in Data Storage)

