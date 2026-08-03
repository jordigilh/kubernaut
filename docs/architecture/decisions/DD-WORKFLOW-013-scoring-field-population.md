# DD-WORKFLOW-013: Scoring Field Population and Data Flow

**Date**: 2025-11-27
**Status**: ⚠️ **PARTIALLY SUPERSEDED** — see correction notice below
**Version**: 1.1
**Authority**: Technical Reference
**Related**: DD-WORKFLOW-004 (Hybrid Scoring — superseded), DD-WORKFLOW-012 (Immutability), DD-WORKFLOW-002 (MCP Architecture — superseded), [DD-WORKFLOW-016](DD-WORKFLOW-016-action-type-workflow-indexing.md) (current scoring authority), [DD-WORKFLOW-019](DD-WORKFLOW-019-ka-owned-workflow-discovery.md) (current ownership)

---

## 📋 Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.1 | 2026-08-02 | Architecture Team (#1806 correction) | **Partial correction pass**: this is not a blanket-supersede case. The scoring *concept* (a numeric field used internally for sorting, kept out of the LLM's view) survives; the specific transport ("HolmesGPT API → Data Storage Service" as two separate network hops) and the specific formula (pgvector `base_similarity` + SQL-computed `label_boost`/`label_penalty`) do not. See "⚠️ Reading This Document" below for the full breakdown, verified against `internal/kubernautagent/workflowcatalog/cache_filter.go` and `pkg/datastorage/models/workflow.go`. | [#1806](https://github.com/jordigilh/kubernaut/issues/1806) |
| 1.0 | 2025-11-27 | Architecture Team | Initial version. | — |

---

## ⚠️ Reading This Document (2026-08-02 Correction Notice)

This document was written against the Nov-2025 Python HolmesGPT-API / PostgreSQL-pgvector architecture. Treat it accordingly:

- **What is still current**: `final_score` still exists, is still computed once per discovery call, still drives `ORDER BY final_score DESC` (now a Go in-memory sort, not SQL), and is still stripped before rendering results to the LLM — the general "keep a scoring breakdown internally, hide the score itself from the LLM" pattern this document argues for is directionally correct. Per [DD-WORKFLOW-016](DD-WORKFLOW-016-action-type-workflow-indexing.md) (`final_score -- Internal: used by KA for sorting, stripped before LLM rendering`), the LLM today sees **zero** score fields at all (not even a minimal `confidence`) — the current design goes further than this document's "PROPOSED (minimal): only `confidence`" conclusion, stripping scores entirely rather than reducing them to one field.
- **What is superseded (actor/transport)**: Step 1/2 below describe "LLM Query → HolmesGPT API → Data Storage Service" as two separate Python/Go services connected over HTTP (`kubernaut-agent/src/toolsets/workflow_catalog.py` calling `POST http://data-storage:8080/api/v1/workflows/search`). Per [DD-WORKFLOW-019](DD-WORKFLOW-019-ka-owned-workflow-discovery.md), that Python file and that Data Storage REST search endpoint are both retired dead code — discovery is a single in-process call from Kubernaut Agent (KA, Go) into its own informer-backed cache (`internal/kubernautagent/workflowcatalog/discovery.go`'s `ListWorkflowsByActionType` / `discovery_cache.go`'s `filterAndScoreCachedWorkflows`), with no second service and no network hop for this step.
- **What is superseded (formula)**: Steps 3-4 describe a PostgreSQL SQL query computing `base_similarity` from pgvector cosine similarity (`1 - (embedding <=> $1)`), plus SQL `CASE`-statement `label_boost`/`label_penalty`. There is no embedding and no `base_similarity` field anywhere in the current implementation (confirmed via grep: zero `pgvector`/embedding references in `internal/kubernautagent/workflowcatalog/`). The real, current formula lives in `internal/kubernautagent/workflowcatalog/cache_filter.go`'s `finalScore(detectedBoost, customBoost, penalty)`: `(5.0 + detectedBoost + customBoost - penalty) / 10.0`, capped at 1.0 — a label-match/conflict scoring model with a flat 0.5 baseline (no semantic component at all), computed in Go over the informer cache rather than in SQL.
- **What partially survives as a shared type**: `pkg/datastorage/models/workflow.go`'s `WorkflowSearchResult` struct (fields `Confidence`, `LabelBoost`, `LabelPenalty`, `FinalScore`, `Rank`, ~lines 355-420) still exists and is explicitly annotated in-code as implementing **"DD-WORKFLOW-004 v1.5 (Label-Only Scoring with Wildcard Weighting)"** — i.e. a *label-only, no-embedding* scoring model. This confirms DD-WORKFLOW-004 itself was silently revised past v1.0 (semantic pgvector query construction, its own document text never updated to reflect this) all the way to a label-only v1.5 design before KA took ownership; this document's Step 3/4 SQL, however, reflects neither v1.0 nor v1.5 exactly.

---

## 🎯 **Purpose**

Document how scoring fields (`base_similarity`, `label_boost`, `label_penalty`, `final_score`) are calculated and populated through the system, and which fields are exposed to the LLM.

---

## 📊 **Complete Data Flow**

### **Step 1: LLM Query → HolmesGPT API**

```python
# LLM calls search_workflow_catalog tool
{
  "query": "OOMKilled critical",
  "filters": {
    "resource-management": "gitops",
    "gitops-tool": "argocd",
    "environment": "production"
  },
  "top_k": 3
}
```

---

### **Step 2: HolmesGPT API → Data Storage Service**

```python
# kubernaut-agent/src/toolsets/workflow_catalog.py
# Line 338: POST request to Data Storage Service

POST http://data-storage:8080/api/v1/workflows/search
{
  "query": "OOMKilled critical",
  "filters": {
    "signal-type": "OOMKilled",        # Mandatory
    "severity": "critical",            # Mandatory
    "resource-management": "gitops",   # Optional
    "gitops-tool": "argocd",          # Optional
    "environment": "production"        # Optional
  },
  "top_k": 3,
  "min_similarity": 0.7
}
```

---

### **Step 3: Data Storage Service → PostgreSQL**

```go
// pkg/datastorage/repository/workflow_repository.go
// Lines 529-541: SQL query with hybrid scoring

SELECT
    *,
    -- CALCULATED FIELD 1: Base Similarity
    (1 - (embedding <=> $1)) AS base_similarity,

    -- CALCULATED FIELD 2: Label Boost
    (
        CASE WHEN labels->>'resource-management' = $2 THEN 0.10 ELSE 0.0 END +
        CASE WHEN labels->>'gitops-tool' = $3 THEN 0.10 ELSE 0.0 END +
        CASE WHEN labels->>'environment' = $4 THEN 0.08 ELSE 0.0 END
    ) AS label_boost,

    -- CALCULATED FIELD 3: Label Penalty
    (
        CASE WHEN labels->>'resource-management' IS NOT NULL
             AND labels->>'resource-management' != $2 THEN 0.10 ELSE 0.0 END +
        CASE WHEN labels->>'gitops-tool' IS NOT NULL
             AND labels->>'gitops-tool' != $3 THEN 0.10 ELSE 0.0 END
    ) AS label_penalty,

    -- CALCULATED FIELD 4: Final Score
    LEAST((1 - (embedding <=> $1)) + (label_boost) - (label_penalty), 1.0) AS final_score

FROM remediation_workflow_catalog
WHERE labels->>'signal-type' = 'OOMKilled'
  AND labels->>'severity' = 'critical'
  AND status = 'active'
  AND is_latest_version = true
  AND (1 - (embedding <=> $1)) >= 0.7
ORDER BY final_score DESC
LIMIT 3
```

**Parameters**:
- `$1`: Query embedding vector (768 dimensions)
- `$2`: "gitops" (resource-management filter)
- `$3`: "argocd" (gitops-tool filter)
- `$4`: "production" (environment filter)

---

### **Step 4: PostgreSQL Results → Go Struct**

```go
// pkg/datastorage/repository/workflow_repository.go
// Lines 546-576: Scan results into Go struct

type workflowWithScore struct {
    models.RemediationWorkflow        // All workflow fields
    BaseSimilarity  float64 `db:"base_similarity"`   // ← From SQL calculation
    LabelBoost      float64 `db:"label_boost"`       // ← From SQL calculation
    LabelPenalty    float64 `db:"label_penalty"`     // ← From SQL calculation
    FinalScore      float64 `db:"final_score"`       // ← From SQL calculation
    SimilarityScore float64 `db:"similarity_score"`  // ← Deprecated
}

// Example result:
{
    Workflow: {
        WorkflowID: "pod-oom-gitops",
        Version: "v1.0.0",
        Description: "Increase memory limits for GitOps-managed pods",
        Labels: {"signal-type": "OOMKilled", "severity": "critical", "resource-management": "gitops"}
    },
    BaseSimilarity: 0.88,   // Calculated by PostgreSQL
    LabelBoost: 0.18,       // Calculated by PostgreSQL (0.10 + 0.08)
    LabelPenalty: 0.0,      // Calculated by PostgreSQL
    FinalScore: 1.0,        // Calculated by PostgreSQL (0.88 + 0.18 = 1.06 → capped to 1.0)
    Rank: 1
}
```

---

### **Step 5: Go Struct → JSON Response**

```go
// pkg/datastorage/server/workflow_handlers.go
// Returns WorkflowSearchResponse as JSON

{
  "workflows": [
    {
      "workflow": {
        "workflow_id": "pod-oom-gitops",
        "version": "v1.0.0",
        "title": "Pod OOM GitOps Recovery",
        "description": "Increase memory limits for GitOps-managed pods",
        "labels": {
          "signal-type": "OOMKilled",
          "severity": "critical",
          "resource-management": "gitops",
          "gitops-tool": "argocd"
        }
      },
      "base_similarity": 0.88,   // ← From PostgreSQL
      "label_boost": 0.18,       // ← From PostgreSQL
      "label_penalty": 0.0,      // ← From PostgreSQL
      "final_score": 1.0,        // ← From PostgreSQL
      "rank": 1
    }
  ],
  "total_results": 1
}
```

---

### **Step 6: JSON Response → Python Transformation**

```python
# kubernaut-agent/src/toolsets/workflow_catalog.py
# _transform_api_response() method

# CURRENT (with breakdown):
{
    "workflow_id": "pod-oom-gitops",
    "confidence": 1.0,          # ← Mapped from api_wf["final_score"]
    "base_similarity": 0.88,    # ← Passed through from api_wf["base_similarity"]
    "label_boost": 0.18         # ← Passed through from api_wf["label_boost"]
}

# PROPOSED (minimal):
{
    "workflow_id": "pod-oom-gitops",
    "version": "v1.0.0",
    "title": "Pod OOM GitOps Recovery",
    "description": "Increase memory limits for GitOps-managed pods",
    "signal_type": "OOMKilled",
    "confidence": 1.0           # ← ONLY score field (mapped from final_score)
}
```

---

## 🎯 **Why Fields Exist at Each Layer**

### **PostgreSQL Layer** (Lines 529-541)

**Purpose**: Calculate scoring components in SQL for performance

**Why Calculate in SQL**:
- ✅ **Performance**: Single query vs multiple round-trips
- ✅ **Accuracy**: Database calculates scores atomically
- ✅ **Indexing**: Can use pgvector HNSW index efficiently
- ✅ **Sorting**: ORDER BY final_score DESC in database

**Fields Calculated**:
- `base_similarity`: pgvector cosine similarity
- `label_boost`: Sum of CASE statements for matching labels
- `label_penalty`: Sum of CASE statements for conflicting labels
- `final_score`: LEAST(base + boost - penalty, 1.0)

---

### **Go Model Layer** (Lines 546-576)

**Purpose**: Type-safe representation of database results

**Why Keep All Fields**:
- ✅ **Debugging**: Operators can inspect scoring breakdown
- ✅ **Metrics**: Prometheus metrics for scoring distribution
- ✅ **Logging**: Detailed logs for troubleshooting
- ✅ **Testing**: Unit tests verify scoring calculations
- ✅ **Tuning**: Analyze boost/penalty effectiveness

**Fields Stored**:
```go
BaseSimilarity  float64  // For debugging: "Why is semantic match low?"
LabelBoost      float64  // For metrics: "How often do label boosts help?"
LabelPenalty    float64  // For debugging: "Why did this workflow score lower?"
FinalScore      float64  // For primary ranking
```

---

### **Python/LLM Layer** (Minimal Response)

**Purpose**: Provide only decision-relevant information to LLM

**Why Remove Breakdown**:
- ✅ **Simplicity**: 1 field (`confidence`) vs 3 fields
- ✅ **Cognitive Load**: LLM doesn't need scoring formula
- ✅ **Decision Focus**: `confidence` is sufficient
- ✅ **Clean Abstraction**: Hide implementation details

**Fields Exposed**:
```python
"confidence": 1.0  # ONLY score field
```

---

## 📋 **Summary**

### **Field Population Flow**

```
PostgreSQL (SQL CASE statements)
    ↓ Calculates base_similarity, label_boost, label_penalty, final_score
Go Model (WorkflowSearchResult)
    ↓ Stores all fields for debugging/metrics
JSON API Response (Data Storage → HolmesGPT)
    ↓ Returns all fields in JSON
Python Transformation (_transform_api_response)
    ↓ Maps final_score → confidence, DROPS breakdown fields
LLM Response (Minimal)
    ↓ Only confidence field
```

### **Why Keep in Go, Remove from LLM**

**Go (Internal)**:
- ✅ Debugging: "Why did this workflow score X?"
- ✅ Metrics: Track scoring distribution
- ✅ Tuning: Analyze boost/penalty effectiveness

**LLM (External)**:
- ✅ Decision-making: `confidence` is sufficient
- ✅ Simplicity: Fewer fields = clearer reasoning
- ✅ Abstraction: Hide implementation details

---

**Status**: ⚠️ **PARTIALLY SUPERSEDED** (2026-08-02, #1806) — the "keep detail internal, hide score from LLM" principle holds; the transport (single in-process KA call, not HolmesGPT-API-to-Data-Storage HTTP) and formula (label-only, no pgvector `base_similarity`) do not. See correction notice near the top of this document.
**Confidence**: 95% (original, pre-correction assessment)
**Implementation (current)**: `internal/kubernautagent/workflowcatalog/cache_filter.go`'s `finalScore` computes the score in Go over the informer cache; the LLM sees no score field at all (not even `confidence`) per DD-WORKFLOW-016.

