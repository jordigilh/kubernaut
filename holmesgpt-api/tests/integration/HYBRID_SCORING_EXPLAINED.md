# Hybrid Scoring Deep Dive - Query to Score Flow

**Design Decision**: DD-WORKFLOW-004 v1.1 - Hybrid Weighted Label Scoring
**Business Requirement**: BR-STORAGE-013 - Semantic Search for Remediation Workflows

---

## 🎯 **Complete Flow: Query → Score**

### **Example Query**
```
Query: "CrashLoopBackOff high severity gitops"
```

This query goes through multiple stages to produce the final score.

---

## 📊 **Stage 1: Query Transformation (HolmesGPT API)**

### **Input (from LLM)**
```
"CrashLoopBackOff high severity gitops"
```

### **Transformation Logic** (`workflow_catalog.py:_build_filters`)

The tool parses the natural language query into structured filters:

```python
def _build_filters(self, query: str, filters: Dict) -> Dict:
    """
    Transform LLM query into Data Storage API filters

    Parsing Rules (DD-LLM-001):
    - Extract signal-type (OOMKilled, CrashLoopBackOff, etc.)
    - Extract severity (critical, high, medium, low)
    - Extract resource-management (gitops, manual, automated)
    """
    search_filters = {}

    # Parse signal-type from query
    if "CrashLoopBackOff" in query:
        search_filters["signal-type"] = "CrashLoopBackOff"

    # Parse severity from query
    if "high" in query.lower():
        search_filters["severity"] = "high"
    elif "critical" in query.lower():
        search_filters["severity"] = "critical"

    # Parse resource-management from query
    if "gitops" in query.lower():
        search_filters["resource-management"] = "gitops"

    return search_filters
```

### **Output (API Request)**
```json
{
  "query": "CrashLoopBackOff high severity gitops",
  "filters": {
    "signal-type": "CrashLoopBackOff",
    "severity": "high",
    "resource-management": "gitops"
  },
  "top_k": 3,
  "min_similarity": 0.7
}
```

---

## 📊 **Stage 2: Embedding Generation (Embedding Service)**

### **Input**
```
"CrashLoopBackOff high severity gitops"
```

### **Process**
1. **Model**: `sentence-transformers/all-mpnet-base-v2`
2. **Tokenization**: Text → tokens
3. **Encoding**: Tokens → 768-dimensional vector

### **Output (Query Embedding)**
```
[0.023, -0.145, 0.892, ..., 0.234]  // 768 dimensions
```

---

## 📊 **Stage 3: Semantic Search (Data Storage Service)**

### **SQL Query Structure**

The Data Storage Service executes a complex PostgreSQL query with pgvector:

```sql
SELECT
    *,
    -- Base Similarity: Cosine similarity from pgvector
    (1 - (embedding <=> $1)) AS base_similarity,

    -- Label Boost: Sum of matching optional labels
    (
        CASE WHEN labels->>'resource-management' = $2 THEN 0.10 ELSE 0.0 END +
        CASE WHEN labels->>'gitops-tool' = $3 THEN 0.10 ELSE 0.0 END +
        CASE WHEN labels->>'environment' = $4 THEN 0.08 ELSE 0.0 END +
        CASE WHEN labels->>'business-category' = $5 THEN 0.08 ELSE 0.0 END +
        CASE WHEN labels->>'priority' = $6 THEN 0.05 ELSE 0.0 END +
        CASE WHEN labels->>'risk-tolerance' = $7 THEN 0.05 ELSE 0.0 END
    ) AS label_boost,

    -- Label Penalty: Sum of conflicting optional labels
    (
        CASE WHEN labels->>'resource-management' IS NOT NULL
             AND labels->>'resource-management' != $2 THEN 0.10 ELSE 0.0 END +
        CASE WHEN labels->>'gitops-tool' IS NOT NULL
             AND labels->>'gitops-tool' != $3 THEN 0.10 ELSE 0.0 END
    ) AS label_penalty,

    -- Final Score: Capped at 1.0
    LEAST(
        (1 - (embedding <=> $1)) + label_boost - label_penalty,
        1.0
    ) AS final_score
FROM remediation_workflow_catalog
WHERE
    labels->>'signal-type' = 'CrashLoopBackOff'  -- Mandatory filter
    AND labels->>'severity' = 'high'              -- Mandatory filter
ORDER BY final_score DESC
LIMIT 3
```

### **Parameters**
- `$1`: Query embedding vector `[0.023, -0.145, ...]`
- `$2`: `"gitops"` (resource-management filter)
- `$3`: `NULL` (no gitops-tool filter)
- `$4-$7`: `NULL` (no other filters)

---

## 📊 **Stage 4: Score Calculation (Per Workflow)**

### **Example Workflow: crashloop-fix-configuration**

#### **Workflow Data**
```json
{
  "workflow_id": "crashloop-fix-configuration",
  "title": "CrashLoopBackOff - Fix Configuration",
  "labels": {
    "signal-type": "CrashLoopBackOff",
    "severity": "high",
    "resource-management": "gitops",
    "gitops-tool": "flux",
    "environment": "production",
    "business-category": "application",
    "priority": "p1",
    "risk-tolerance": "low"
  },
  "embedding": [0.019, -0.152, 0.885, ..., 0.241]  // 768 dimensions
}
```

#### **Step 1: Base Similarity (Semantic Match)**

**Calculation**: Cosine similarity between query and workflow embeddings

```
Query Embedding:    [0.023, -0.145, 0.892, ..., 0.234]
Workflow Embedding: [0.019, -0.152, 0.885, ..., 0.241]

Cosine Distance = 1 - cosine_similarity(query_emb, workflow_emb)
                = 0.12  (example)

Base Similarity = 1 - Cosine Distance
                = 1 - 0.12
                = 0.88
```

**Result**: `base_similarity = 0.88` (88% semantic match)

#### **Step 2: Label Boost (Matching Optional Labels)**

**Boost Weights** (DD-WORKFLOW-004 v1.1):
```
resource-management: +0.10
gitops-tool:         +0.10
environment:         +0.08
business-category:   +0.08
priority:            +0.05
risk-tolerance:      +0.05
─────────────────────────────
Max Total Boost:     +0.46
```

**Query Filters**:
- `resource-management: "gitops"` ✅ Matches workflow label
- `gitops-tool: NULL` ❌ Not specified in query
- `environment: NULL` ❌ Not specified
- `business-category: NULL` ❌ Not specified
- `priority: NULL` ❌ Not specified
- `risk-tolerance: NULL` ❌ Not specified

**Calculation**:
```
Label Boost = 0.10 (resource-management match)
            + 0.00 (gitops-tool not in query)
            + 0.00 (environment not in query)
            + 0.00 (business-category not in query)
            + 0.00 (priority not in query)
            + 0.00 (risk-tolerance not in query)
            = 0.10
```

**Result**: `label_boost = 0.10` (10% boost for gitops match)

#### **Step 3: Label Penalty (Conflicting Optional Labels)**

**Penalty Weights**:
```
resource-management: -0.10 (if conflict)
gitops-tool:         -0.10 (if conflict)
─────────────────────────────
Max Total Penalty:   -0.20
```

**Query Filters vs Workflow Labels**:
- `resource-management: "gitops"` vs `"gitops"` ✅ Match (no penalty)
- `gitops-tool: NULL` vs `"flux"` ✅ Not specified (no penalty)

**Calculation**:
```
Label Penalty = 0.00 (resource-management matches)
              + 0.00 (gitops-tool not in query, so no conflict)
              = 0.00
```

**Result**: `label_penalty = 0.00` (no conflicting labels)

#### **Step 4: Final Score (Capped at 1.0)**

**Formula**:
```
final_score = LEAST(base_similarity + label_boost - label_penalty, 1.0)
```

**Calculation**:
```
final_score = LEAST(0.88 + 0.10 - 0.00, 1.0)
            = LEAST(0.98, 1.0)
            = 0.98
```

**Result**: `final_score = 0.98` (98% overall match)

---

## 📊 **Stage 5: Response Transformation (Data Storage Service)**

### **API Response**
```json
{
  "workflows": [
    {
      "workflow": {
        "workflow_id": "crashloop-fix-configuration",
        "version": "1.0.0",
        "title": "CrashLoopBackOff - Fix Configuration",
        "description": "Identifies and fixes configuration issues...",
        "labels": {
          "signal-type": "CrashLoopBackOff",
          "severity": "high",
          "resource-management": "gitops",
          "gitops-tool": "flux"
        },
        "estimated_duration": "15 minutes",
        "success_rate": 0.75
      },
      "base_similarity": 0.88,
      "label_boost": 0.10,
      "label_penalty": 0.00,
      "final_score": 0.98,
      "rank": 1
    }
  ],
  "total_results": 1,
  "query": "CrashLoopBackOff high severity gitops"
}
```

---

## 📊 **Stage 6: Tool Result Transformation (HolmesGPT API)**

### **Transformation** (`workflow_catalog.py:_transform_api_response`)

```python
def _transform_api_response(self, api_workflows: List[Dict]) -> List[Dict]:
    """Transform API response to LLM-friendly format"""
    results = []

    for api_wf in api_workflows:
        workflow_data = api_wf.get("workflow", {})

        tool_workflow = {
            "workflow_id": workflow_data.get("workflow_id"),
            "title": workflow_data.get("title"),
            "description": workflow_data.get("description"),
            "signal_types": [workflow_data["labels"]["signal-type"]],
            "estimated_duration": workflow_data.get("estimated_duration"),
            "success_rate": workflow_data.get("success_rate"),  # 0.75

            # Hybrid scoring fields (DD-WORKFLOW-004)
            "similarity_score": api_wf.get("final_score"),      # 0.98
            "base_similarity": api_wf.get("base_similarity"),   # 0.88
            "label_boost": api_wf.get("label_boost")            # 0.10
        }

        results.append(tool_workflow)

    return results
```

### **Final Tool Result (to LLM)**
```json
{
  "workflows": [
    {
      "workflow_id": "crashloop-fix-configuration",
      "version": "1.0.0",
      "title": "CrashLoopBackOff - Fix Configuration",
      "description": "Identifies and fixes configuration issues causing CrashLoopBackOff...",
      "signal_types": ["CrashLoopBackOff"],
      "estimated_duration": "15 minutes",
      "success_rate": 0.75,
      "similarity_score": 0.98,
      "base_similarity": 0.88,
      "label_boost": 0.10
    }
  ]
}
```

---

## 🎓 **Score Interpretation**

### **What Each Score Means**

| Score | Value | Meaning |
|-------|-------|---------|
| **base_similarity** | 0.88 | 88% semantic match between query and workflow description |
| **label_boost** | 0.10 | +10% boost for matching `resource-management: gitops` |
| **label_penalty** | 0.00 | No penalty (no conflicting labels) |
| **final_score** | 0.98 | **98% overall match** (semantic + label matching) |
| **success_rate** | 0.98 | **98% confidence/match rate** (same as final_score) |

### **Why This Workflow Scored High**

1. **Strong Semantic Match (0.88)**:
   - Query mentions "CrashLoopBackOff" → Workflow title is "CrashLoopBackOff - Fix Configuration"
   - Query mentions "high severity" → Workflow has `severity: high`
   - Query embedding is very similar to workflow embedding

2. **Label Boost (+0.10)**:
   - Query mentions "gitops" → Workflow has `resource-management: gitops`
   - This adds +10% to the score

3. **No Penalties (0.00)**:
   - No conflicting labels
   - Query didn't specify gitops-tool, so no conflict with "flux"

4. **Final Score (0.98)**:
   - `0.88 (base) + 0.10 (boost) - 0.00 (penalty) = 0.98`
   - This is a **very high match** (98%)

5. **Success Rate = Final Score**:
   - The `success_rate` field represents the **confidence/match percentage**
   - It's the same as `final_score`: **98% match** from query + labels

---

## 🔬 **Example: Lower Score Scenario**

### **Query**
```
"CrashLoopBackOff high severity manual"
```

### **Workflow: crashloop-fix-configuration**
```json
{
  "labels": {
    "signal-type": "CrashLoopBackOff",
    "severity": "high",
    "resource-management": "gitops"  // ← Conflicts with "manual"
  }
}
```

### **Score Calculation**
```
base_similarity = 0.88  (same semantic match)
label_boost     = 0.00  (no matching optional labels)
label_penalty   = 0.10  (resource-management conflict: manual vs gitops)
final_score     = 0.88 + 0.00 - 0.10 = 0.78
```

**Result**: Score drops from **0.98** to **0.78** due to conflicting label!

---

## 📊 **Score Distribution Examples**

### **Perfect Match (1.00)**
```
base_similarity: 0.95
label_boost:     0.18  (multiple matching labels)
label_penalty:   0.00
final_score:     1.13 → capped at 1.00
```

### **Good Match (0.85)**
```
base_similarity: 0.80
label_boost:     0.10
label_penalty:   0.05
final_score:     0.85
```

### **Moderate Match (0.70)**
```
base_similarity: 0.75
label_boost:     0.05
label_penalty:   0.10
final_score:     0.70
```

### **Weak Match (0.50)**
```
base_similarity: 0.60
label_boost:     0.00
label_penalty:   0.10
final_score:     0.50
```

---

## 🎯 **Key Takeaways**

1. **Semantic Search First**: Base similarity (0.0-1.0) from pgvector cosine similarity
2. **Label Boosting**: Matching optional labels add up to +0.46
3. **Label Penalties**: Conflicting labels subtract up to -0.20
4. **Final Score**: Capped at 1.0 for consistency
5. **Success Rate**: Independent metric (historical performance, not search relevance)

---

## 🔗 **References**

- **DD-WORKFLOW-004 v1.1**: Hybrid Weighted Label Scoring
- **DD-LLM-001**: MCP Workflow Search Parameter Taxonomy
- **DD-STORAGE-008**: Workflow Catalog Schema
- **DD-EMBEDDING-001**: Model B (all-mpnet-base-v2, 768 dimensions)
- **BR-STORAGE-013**: Semantic Search for Remediation Workflows

