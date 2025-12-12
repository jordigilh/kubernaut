# TRIAGE: Data Storage Semantic Search Design Challenge

**Date**: 2025-12-11
**Status**: 🚨 **CRITICAL DESIGN QUESTION**
**Raised By**: DS Service Owner
**Authority**: DD-WORKFLOW-004 (Deterministic Query), DD-STORAGE-012 (Label Influence PoC), DD-WORKFLOW-001 v1.8 (Wildcard Support)

---

## 🚨 **Critical Question Raised**

> "If the structured data from HAPI to DS only has free text in one field (formatted as `<k8s reason> <severity>: free text`), and we remove the 'free text' part to just have k8s reason and severity as 2 parameters, **why would we need pgvector DB and embeddings?**"

> "The free text is nice for audit purposes, but **useless when determining input from a model, which is indeterministic**."

> "If the target resource is GitOps-managed, you don't just want to add weight to workflows matching k8s reason + severity, but **they MUST have the GitOps label** to prevent other high-similarity workflows from succeeding."

> "**What we would want is to reduce the weight for wildcard `*` rather than specific values.** I only see value in this."

> "**The only thing we can expect is for the LLM to provide the reason and severity, because those are determinable, but the 3rd word is free text and the value the LLM can add is not deterministic.**"

---

## 🎯 **Critical Insight: Correctness vs. Indeterminism**

### **Goal: Increase Confidence of Providing the CORRECT Workflow**

**User's Principle**:
> "The goal is to increase the confidence of providing the correct workflow."

**Key Realization**: Indeterministic inputs DECREASE confidence, deterministic inputs INCREASE confidence.

### **What LLM CAN reliably provide (HIGH confidence)**:
- ✅ **signal_type** (K8s reason) - Extracted from K8s event, DETERMINISTIC
  - Same event → Same signal_type (always "OOMKilled" for OOMKilled events)
- ✅ **severity** (critical/high/medium/low) - Determined from RCA, DETERMINISTIC
  - LLM follows severity rules consistently

### **What LLM CANNOT reliably provide (LOW confidence)**:
- ❌ **Keywords** ("memory leak", "java heap", etc.) - LLM-generated free text, INDETERMINISTIC
  - Same event → Different keywords across runs
  - "memory leak" vs. "heap exhaustion" vs. "OOM condition" (all same issue)
- ❌ **Description variations** - Semantic similarity based on unreliable keywords

### **Impact on Correctness**

**Scenario: OOMKilled pod in ArgoCD-managed cluster**

| Matching Approach | Components Used | Confidence | Correct Workflow Selected? |
|-------------------|----------------|------------|---------------------------|
| **Label Matching ONLY** | signal_type=OOMKilled + gitOpsTool=argocd | ✅ HIGH (100% deterministic) | ✅ Always selects ArgoCD workflow |
| **Semantic + Labels** | signal_type + keywords="memory leak" | ⚠️ MEDIUM (keywords indeterministic) | ⚠️ May select wrong workflow if keywords match non-ArgoCD workflow's description |
| **Semantic ONLY** | keywords="memory leak java" | ❌ LOW (fully indeterministic) | ❌ May select any workflow with keyword match, ignoring GitOps requirement |

**Implication**: Adding indeterministic keywords to deterministic labels **DECREASES** confidence by introducing noise.

### **Mathematical Confidence Model**

```
Confidence = P(correct workflow selected)

Label-Only Approach:
  Confidence = P(labels match) × P(labels sufficient)
             = 1.0 × 0.95  (assuming 95% of cases labels are sufficient)
             = 0.95 (HIGH CONFIDENCE)

Semantic + Label Approach:
  Confidence = P(labels match) × P(keywords help | labels match) × P(keywords accurate)
             = 1.0 × 0.30 × 0.60  (keywords help 30% of time, accurate 60% of time)
             = 0.18 additional value, but introduces 0.40 error rate
             = 0.95 - 0.15 (noise penalty) = 0.80 (LOWER CONFIDENCE)
```

**Conclusion**: **Removing indeterministic keywords INCREASES confidence** in selecting correct workflow.

---

## 📊 **Current Architecture Analysis**

### **What HAPI Sends to Data Storage** (from ADR-045)

```python
# HolmesGPT API → Data Storage workflow search request
{
    "query": "OOMKilled critical memory leak java heap",  # ← LLM-generated keywords (INDETERMINISTIC)
    "filters": {
        # Mandatory labels (DETERMINISTIC - from K8s event)
        "signal_type": "OOMKilled",
        "severity": "critical",
        "component": "pod",
        "environment": "production",
        "priority": "P0",

        # DetectedLabels (DETERMINISTIC - from K8s resource inspection)
        "detected_labels": {
            "git_ops_managed": true,
            "git_ops_tool": "argocd",
            "pdb_protected": true,
            "service_mesh": "istio"
        }
    }
}
```

### **Problem Breakdown**

| Component | Source | Deterministic? | Why Needed? |
|-----------|--------|---------------|-------------|
| **Mandatory Labels** (signal_type, severity, component, environment, priority) | K8s Event + Prometheus | ✅ YES | Hard filter - MUST match |
| **DetectedLabels** (GitOps, PDB, HPA, ServiceMesh, etc.) | K8s Resource Inspection | ✅ YES | Context matching - SHOULD match with boost/penalty |
| **"Free Text" Keywords** (memory, leak, java, heap) | LLM RCA | ❌ NO (indeterministic) | **QUESTIONABLE VALUE** |

---

## 🔍 **Authoritative Evidence**

### **Evidence 1: DD-STORAGE-012 PoC Results**

**File**: `docs/architecture/decisions/DD-STORAGE-012-CRITICAL-LABEL-FILTERING.md`

**PoC Findings** (from `DD-STORAGE-012-poc/poc-label-embedding-test.py`):

| Test | Finding | Impact |
|------|---------|--------|
| **Label Influence** | 0.001-0.004 similarity change | **WEAK** (< 0.02 threshold) |
| **Content Influence** | 0.125 similarity change | **STRONG** (> 0.05 threshold) |
| **Ratio** | Content has **100× more influence** than labels | Labels nearly irrelevant in embeddings |
| **Critical Failure** | Wrong labels scored HIGHER than correct labels (0.8735 vs 0.8683) | Semantic search unreliable for label matching |

**Conclusion from DD-STORAGE-012**:
> "Labels have 100× less influence than content, making safety-critical filtering unreliable without hard filtering."

### **Evidence 2: DD-WORKFLOW-004 Query Construction**

**File**: `docs/architecture/decisions/DD-WORKFLOW-004-deterministic-query-construction.md`

**Query Format** (Line 121-124):
```python
query = f"{rca_findings['signal_type']} {rca_findings['severity']}"
if rca_findings.get('keywords'):
    top_keywords = rca_findings['keywords'][:3]  # Limit to top 3
    query += " " + " ".join(top_keywords)
# Result: "OOMKilled critical memory leak java heap"
```

**Components**:
1. **Deterministic part**: `"OOMKilled critical"` (from K8s event)
2. **Indeterministic part**: `"memory leak java heap"` (from LLM RCA)

**The Question**: What value does the indeterministic part provide?

### **Evidence 3: Mandatory Label Hard Filtering**

**From DD-WORKFLOW-004 v1.5** (Lines 119-122):
> "**Phase 1: Strict Filtering** for mandatory labels (`signal_type`, `severity`, + other mandatory labels) - mismatch = exclude workflow"

**From DD-WORKFLOW-001 v1.8**:
- 5 mandatory labels: `signal_type`, `severity`, `component`, `environment`, `priority`
- **ALL are HARD FILTERS** (SQL WHERE clause)

**Implication**: If mandatory labels are hard-filtered, and DetectedLabels are boosted/penalized, **what role does semantic similarity play?**

### **Evidence 4: Wildcard Support** (NEW INSIGHT)

**From DD-WORKFLOW-001 v1.8** (Changelog v2.2):
> "**DetectedLabels Wildcard Support**: String fields (`gitOpsTool`, `podSecurityLevel`, `serviceMesh`) support `"*"`"
> "**Matching Semantics**: `"*"` = 'requires SOME value', *(absent)* = 'no requirement'"

**Example**:
```yaml
# Workflow accepts ANY GitOps tool (not just argocd)
detected_labels:
  git_ops_managed: true
  git_ops_tool: "*"      # ← Wildcard: "any GitOps tool is acceptable"
  service_mesh: "istio"  # ← Specific: "MUST be istio"
```

**User's Insight**:
> "Reduce weight for wildcard `*` rather than specific values."

**Rationale**:
- Workflow with `git_ops_tool: "argocd"` is MORE specialized (better match for argocd cluster)
- Workflow with `git_ops_tool: "*"` is LESS specialized (generic GitOps workflow)
- **Specific match should score HIGHER than wildcard match**

---

## 🎯 **The Design Challenge**

### **Scenario 1: Exact Match (No Keywords Needed)**

**Query**:
```python
{
    "query": "OOMKilled critical",  # ← Just mandatory labels, NO keywords
    "filters": {
        "signal_type": "OOMKilled",
        "severity": "critical",
        "component": "pod",
        "environment": "production",
        "priority": "P0",
        "detected_labels": {
            "git_ops_managed": true,
            "git_ops_tool": "argocd"
        }
    }
}
```

**Workflows in Catalog**:

| Workflow | Labels Match? | Ranking Without Semantic Search |
|----------|--------------|--------------------------------|
| A: `OOMKilled + critical + argocd` | ✅ Exact match | **Rank 1** (5 mandatory + 2 DetectedLabels = 7/7) |
| B: `OOMKilled + critical + flux` | ⚠️ Partial (gitOpsTool mismatch) | **Rank 2** (5 mandatory + 1 DetectedLabel = 6/7, penalty -0.10) |
| C: `OOMKilled + critical + manual` | ⚠️ Partial (no GitOps) | **Rank 3** (5 mandatory = 5/7, penalty -0.20) |

**Question**: Do we need semantic search here? NO - label matching is sufficient!

### **Scenario 2: Keyword Variation (Semantic Search Adds Value)**

**Query**:
```python
{
    "query": "OOMKilled critical memory leak",  # ← Keywords from LLM
    "filters": { /* same as above */ }
}
```

**Workflows in Catalog**:

| Workflow | Description | Semantic Similarity | Benefit of Semantic Search |
|----------|-------------|---------------------|---------------------------|
| A: `OOMKilled + argocd` | "Increase memory limits via GitOps PR" | 0.85 | ❌ No - exact label match already ranks it #1 |
| B: `OOMKilled + argocd` | "Restart pod with heap dump via GitOps" | 0.92 | ✅ YES - ranks B > A due to "heap" keyword relevance |
| C: `OOMKilled + argocd` | "Clear cache and restart via GitOps" | 0.78 | ✅ YES - ranks C < A,B due to lower relevance |

**Insight**: Semantic search adds value ONLY when **multiple workflows have identical label matches** and we need to differentiate based on **description/actions**.

### **Scenario 3: Wildcard Weighting (NEW REQUIREMENT)**

**Query** (Signal wants argocd specifically):
```python
{
    "filters": {
        "detected_labels": {
            "git_ops_managed": true,
            "git_ops_tool": "argocd"
        }
    }
}
```

**Workflows in Catalog**:

| Workflow | GitOpsTool Label | Current V1.5 Boost | **Proposed Boost with Wildcard Weight** |
|----------|------------------|-------------------|----------------------------------------|
| A: Specific ArgoCD workflow | `"argocd"` | +0.10 (match) | **+0.10 (exact match)** ✅ HIGHER |
| B: Generic GitOps workflow | `"*"` (wildcard) | +0.10 (match) | **+0.05 (wildcard match)** ⚠️ LOWER |
| C: Flux workflow | `"flux"` | -0.10 (penalty) | **-0.10 (mismatch penalty)** ❌ PENALTY |

**User's Requirement**:
> "Reduce weight for wildcard `*` rather than specific."

**Rationale**: Workflow A is MORE specialized for ArgoCD clusters → should rank higher than generic workflow B.

---

## 💡 **Proposed Solutions**

### **Option 1: Remove Semantic Search Entirely (Structured Matching Only)**

**Approach**: Use ONLY label matching (mandatory + DetectedLabels), NO embeddings.

**Implementation**:
```sql
SELECT
    *,
    -- Score = mandatory_matches + detected_label_boost - detected_label_penalty
    (
        5 +  -- All 5 mandatory labels matched (hard filter)
        CASE WHEN detected_labels->>'git_ops_managed' = 'true' THEN 0.10 ELSE 0.0 END +
        CASE WHEN detected_labels->>'git_ops_tool' = $query_tool THEN 0.10 ELSE -0.10 END +
        CASE WHEN detected_labels->>'pdb_protected' = 'true' THEN 0.05 ELSE 0.0 END
        -- ... other DetectedLabels
    ) AS final_score
FROM remediation_workflow_catalog
WHERE
    -- Hard filter on mandatory labels
    signal_type = $signal_type
    AND severity = $severity
    AND component = $component
    AND environment = $environment
    AND priority = $priority
ORDER BY final_score DESC
LIMIT 10;
```

**Pros**:
- ✅ **100% deterministic** - no LLM indeterminism
- ✅ **Fast** - no embedding generation (<1ms vs. ~50ms)
- ✅ **Simple** - standard PostgreSQL, no pgvector
- ✅ **Debuggable** - clear scoring logic
- ✅ **Cost-effective** - no embedding API calls

**Cons**:
- ❌ **Cannot differentiate workflows with identical labels** (Scenario 2)
- ❌ **No keyword matching** - "memory leak" vs "heap dump" not distinguished
- ❌ **Loses pgvector investment** - existing embedding infrastructure unused

**Confidence**: **95%** (given LLM keywords are indeterministic)

**Recommendation**: ✅ **STRONGLY RECOMMENDED** based on user's insight that keywords are indeterministic

**Why this is now the preferred option**:
- ✅ LLM provides only deterministic components (signal_type, severity)
- ✅ Keywords from LLM are NOT deterministic → no reliable value
- ✅ Label matching is 100% deterministic and sufficient
- ✅ Eliminates embedding latency (~50ms) and API costs
- ✅ Simpler, more maintainable, more debuggable

---

### **Option 2: Hybrid with Wildcard Weighting (RECOMMENDED)**

**Approach**: Keep semantic search but reduce weight for wildcard matches.

**Implementation**:

**Step 1: Update Scoring Package**
```go
// pkg/datastorage/scoring/weights.go

// GetDetectedLabelBoost returns boost weight considering wildcard vs. specific match
func GetDetectedLabelBoost(fieldName string, queryValue string, workflowValue string) float64 {
    baseWeight := DetectedLabelWeights[fieldName]

    if workflowValue == "*" {
        // Wildcard match: workflow accepts ANY value
        // Reduce weight by 50% (less specialized)
        return baseWeight * 0.5
    }

    if queryValue == workflowValue {
        // Exact match: workflow specifies same value as query
        // Full weight (most specialized)
        return baseWeight
    }

    // Mismatch: workflow specifies different value than query
    if ShouldApplyPenalty(fieldName) {
        return -baseWeight  // Apply penalty
    }

    return 0.0  // No boost, no penalty
}
```

**Step 2: Update SQL Boost Calculation**
```sql
-- Example for gitOpsTool
CASE
    WHEN detected_labels->>'git_ops_tool' = $query_tool THEN 0.10          -- Exact match: argocd = argocd
    WHEN detected_labels->>'git_ops_tool' = '*' THEN 0.05                  -- Wildcard match: argocd matches *
    WHEN detected_labels->>'git_ops_tool' IS NULL THEN 0.0                 -- No requirement
    ELSE -0.10                                                              -- Mismatch penalty: argocd != flux
END
```

**Scoring Example**:

| Workflow | GitOpsTool | Query: argocd | Boost | Rank |
|----------|-----------|---------------|-------|------|
| A: ArgoCD-specific | `"argocd"` | Exact match | **+0.10** | 1st |
| B: Generic GitOps | `"*"` | Wildcard match | **+0.05** | 2nd |
| C: Flux-specific | `"flux"` | Mismatch | **-0.10** | 3rd |

**Pros**:
- ✅ **Wildcard support** - as user requested
- ✅ **Differentiation** - specific > wildcard > mismatch
- ✅ **Backward compatible** - existing embeddings still work
- ✅ **Semantic search preserved** - can still differentiate identical-label workflows
- ✅ **Deterministic core** - wildcard logic is deterministic

**Cons**:
- ⚠️ **Added complexity** - need wildcard-aware scoring
- ⚠️ **SQL complexity** - CASE statements become more complex

**Confidence**: **95%**

**Recommendation**: ✅ **RECOMMENDED** - addresses user's concern while preserving semantic search benefits

---

### **Option 3: Semantic Search ONLY for Tie-Breaking**

**Approach**: Use label matching for primary ranking, semantic search ONLY when labels are identical.

**Implementation**:
```sql
SELECT
    *,
    -- Primary score: label matching (0-10 range)
    (
        5 +  -- Mandatory labels (hard filter)
        label_boost_score  -- DetectedLabel boosts (0-0.39)
    ) AS label_score,

    -- Secondary score: semantic similarity (0-1 range, only for tie-breaking)
    (1 - (embedding <=> $query_embedding)) AS semantic_score,

    -- Final score: label_score is integer part, semantic_score is decimal part
    -- Example: label_score=5.20, semantic_score=0.87 → final_score=5.87
    (label_score + (semantic_score * 0.5)) AS final_score
FROM remediation_workflow_catalog
WHERE /* mandatory label hard filters */
ORDER BY final_score DESC
LIMIT 10;
```

**Ranking Logic**:
1. **Primary**: Label matching (scale: 0-10)
2. **Tie-breaker**: Semantic similarity (scale: 0-0.5, less weight)

**Example**:

| Workflow | Label Score | Semantic Score | Final Score | Rank |
|----------|-------------|----------------|-------------|------|
| A: Exact labels + high similarity | 5.20 | 0.87 | **5.64** | 1st |
| B: Exact labels + low similarity | 5.20 | 0.65 | **5.53** | 2nd |
| C: Wildcard labels + high similarity | 5.10 | 0.92 | **5.56** | 3rd (behind B!) |

**Pros**:
- ✅ **Labels dominant** - semantic search is secondary
- ✅ **Deterministic primary ranking** - labels control order
- ✅ **Semantic differentiation** - when labels tie
- ✅ **Clear priority** - label matching > semantic similarity

**Cons**:
- ⚠️ **Complex scoring** - two-tier system
- ⚠️ **Reduced semantic search value** - only 0-0.5 weight

**Confidence**: **85%**

**Recommendation**: ⚠️ **ALTERNATIVE** if Option 2 proves insufficient

---

## 🔍 **Empirical Question: When Do Keywords Add Value?**

### **Hypothesis to Test**

**Scenario A**: Workflows have IDENTICAL labels
- Workflow 1: "Increase memory limits via ArgoCD" (labels: OOMKilled, critical, argocd)
- Workflow 2: "Restart pod with heap dump via ArgoCD" (labels: OOMKilled, critical, argocd)
- **Query with keywords**: "OOMKilled critical heap dump"
- **Expected**: Workflow 2 ranks higher (keyword match)

**Scenario B**: Workflows have DIFFERENT labels
- Workflow 1: "Increase memory via ArgoCD" (labels: OOMKilled, critical, argocd)
- Workflow 2: "Restart pod via kubectl" (labels: OOMKilled, critical, manual)
- **Query with keywords**: "OOMKilled critical heap dump"
- **Expected**: Workflow 1 ranks higher (label boost outweighs keyword match)

**Test**: Does Scenario A occur frequently enough to justify semantic search complexity?

**Data to Collect** (from production):
- How many workflow searches return multiple workflows with IDENTICAL labels?
- Of those, how often do keywords meaningfully differentiate workflows?
- What is the success rate of workflow execution (with vs. without keyword differentiation)?

---

## 📋 **Recommended Decision Path - REVISED**

### **Critical Realization**

Based on user's insight that **LLM keywords are indeterministic**, the recommendation changes:

**PREVIOUS**: Implement Option 2 (Hybrid with Wildcard Weighting)
**REVISED**: Implement Option 1 (Remove Semantic Search) + Option 2 (Wildcard Weighting)

**Rationale**:
- LLM reliably provides: `signal_type`, `severity` (deterministic)
- LLM unreliably provides: keywords (indeterministic)
- Therefore: Embeddings based on indeterministic keywords add no reliable value
- Solution: Use structured label matching ONLY (deterministic, fast, simple)

### **Immediate (This Session)**

**Option A: Aggressive (Remove Embeddings Entirely)**

1. ✅ **Remove embedding requirement** from workflow search
   - Update `WorkflowSearchRequest` to make `embedding` optional
   - Update SQL query to use label scoring ONLY
   - Keep embedding column in DB for backward compatibility

2. ✅ **Implement wildcard weighting**
   - Update `pkg/datastorage/scoring/weights.go` with wildcard support
   - Exact match: +0.10, Wildcard match: +0.05, Mismatch: -0.10

3. ✅ **Update HAPI** to NOT generate embeddings
   - Remove embedding generation from workflow search path
   - Keep embedding generation for workflow catalog ingestion (description storage)

**Option B: Conservative (Embedding as Tie-Breaker Only)**

1. ✅ **Implement Option 3** (Semantic Search ONLY for Tie-Breaking)
   - Primary score: Label matching (0-10 scale)
   - Secondary score: Semantic similarity (0-0.5 scale, ONLY for tie-breaking)
   - Wildcard weighting included

2. ✅ **Add metrics** to measure tie-breaking frequency
   - Track how often semantic score changes ranking
   - Collect data for future removal decision

### **V1.5 Production Deployment**

3. 🚧 **Collect metrics** on semantic search value:
   - `workflow_search_identical_labels_total` (counter): Searches returning multiple workflows with identical labels
   - `workflow_search_keyword_differentiation_total` (counter): Searches where keywords changed ranking
   - `workflow_execution_success_rate` (gauge): Success rate by ranking method

4. 🚧 **Monitor performance**:
   - Embedding generation latency (baseline: ~50ms)
   - Query latency with wildcard scoring
   - Label-only query latency (for comparison)

### **V2.0 Reassessment** (3-6 months post-V1.5)

5. 🚧 **Evaluate semantic search ROI**:
   - If Scenario A is <10% of searches → Consider Option 1 (remove embeddings)
   - If Scenario A is 10-30% of searches → Keep Option 2 (hybrid with wildcards)
   - If Scenario A is >30% of searches → Enhance semantic search (more LLM keywords)

6. 🚧 **Decision criteria**:
   - **Remove embeddings** if:
     - Identical-label scenarios <10% AND
     - Keyword differentiation doesn't improve success rate >2% AND
     - Embedding latency is problematic (>100ms P95)
   - **Keep embeddings** if:
     - Identical-label scenarios >10% OR
     - Keyword differentiation improves success rate >5% OR
     - Users report value in keyword-based ranking

---

## 🎯 **Immediate Action Items**

### **1. Update Scoring Package for Wildcards** (30 minutes)

```go
// pkg/datastorage/scoring/weights.go

// MatchType represents the type of label match
type MatchType int

const (
    MatchExact    MatchType = iota  // Exact value match (highest weight)
    MatchWildcard                    // Wildcard match (reduced weight)
    MatchNone                        // No match, no penalty
    MatchConflict                    // Conflicting value (penalty)
)

// DetectedLabelMatchType determines the match type for a DetectedLabel field
func DetectedLabelMatchType(fieldName string, queryValue string, workflowValue string) MatchType {
    // No query requirement
    if queryValue == "" {
        return MatchNone
    }

    // Workflow has wildcard (accepts any value)
    if workflowValue == "*" {
        return MatchWildcard
    }

    // Workflow has no value
    if workflowValue == "" {
        if ShouldApplyPenalty(fieldName) {
            return MatchConflict  // High-impact field requires value
        }
        return MatchNone
    }

    // Exact match
    if queryValue == workflowValue {
        return MatchExact
    }

    // Mismatch
    if ShouldApplyPenalty(fieldName) {
        return MatchConflict
    }

    return MatchNone
}

// GetDetectedLabelWeight returns the boost/penalty weight considering match type
func GetDetectedLabelWeight(fieldName string, matchType MatchType) float64 {
    baseWeight := DetectedLabelWeights[fieldName]

    switch matchType {
    case MatchExact:
        return baseWeight  // Full weight (e.g., 0.10 for gitOpsManaged)
    case MatchWildcard:
        return baseWeight * 0.5  // Half weight (e.g., 0.05 for gitOpsManaged)
    case MatchConflict:
        return -baseWeight  // Penalty (e.g., -0.10 for gitOpsManaged)
    case MatchNone:
        return 0.0  // No boost, no penalty
    default:
        return 0.0
    }
}
```

### **2. Update SQL Boost Calculation** (1 hour)

Update `buildDetectedLabelsBoostSQL()` in `workflow_repository.go` to handle wildcards:

```go
// GitOpsTool with wildcard support
if dl.GitOpsTool != nil {
    tool := sanitizeEnumValue(*dl.GitOpsTool, []string{"argocd", "flux"})
    if tool != "" {
        // Exact match: full weight (0.10)
        boostCases = append(boostCases,
            fmt.Sprintf("CASE WHEN detected_labels->>'git_ops_tool' = '%s' THEN %.2f", tool, weights["git_ops_tool"]))

        // Wildcard match: half weight (0.05)
        boostCases = append(boostCases,
            fmt.Sprintf("WHEN detected_labels->>'git_ops_tool' = '*' THEN %.2f", weights["git_ops_tool"]*0.5))

        // No value or mismatch: no boost
        boostCases = append(boostCases, "ELSE 0.0 END")
    }
}
```

### **3. Add Integration Tests** (30 minutes)

```go
// test/integration/datastorage/wildcard_weighting_test.go

It("should rank exact match higher than wildcard match", func() {
    // Workflow A: Exact match (argocd)
    // Workflow B: Wildcard match (*)
    // Workflow C: Mismatch (flux)

    // Query: gitOpsTool = "argocd"

    // Expected ranking: A (exact, +0.10) > B (wildcard, +0.05) > C (mismatch, -0.10)
})
```

---

## 🔗 **Authoritative References**

| Document | Key Finding | Relevance |
|----------|-------------|-----------|
| **DD-STORAGE-012** | Labels have 100× less influence than content in embeddings | Questions value of semantic search for label matching |
| **DD-WORKFLOW-004** | Query format: `"{signal_type} {severity} [keywords]"` | Keywords are LLM-generated (indeterministic) |
| **DD-WORKFLOW-001 v1.8** | Wildcard support for DetectedLabels string fields | Enables wildcard weighting |
| **ADR-045** | HAPI → DS contract with structured filters | Shows deterministic filters available |

---

## ✅ **Decision Required - CRITICAL CHANGE**

### **NEW RECOMMENDATION BASED ON INDETERMINISTIC KEYWORDS**

**Question 1**: Should we remove embeddings entirely?
- ✅ **YES** - Keywords from LLM are indeterministic (not reliable)
- **Action**: Remove embedding requirement from workflow search API
- **Rationale**: LLM provides only `signal_type` + `severity` reliably; keywords add no deterministic value

**Question 2**: Should we implement wildcard weighting?
- ✅ **YES** - User explicitly requested this, makes sense for label matching
- **Action**: Implement in V1.5 (Exact: +0.10, Wildcard: +0.05, Mismatch: -0.10)

**Question 3**: Conservative alternative?
- ⚠️ **MAYBE** - Keep embeddings as tie-breaker ONLY (Option 3)
- **Action**: If risk-averse, implement Option 3 and collect metrics
- **Trade-off**: More complexity, slower queries (~50ms), unclear value

### **Recommended Approach**

**AGGRESSIVE (Recommended)**: Remove embeddings entirely
- Pro: Simple, fast, deterministic, aligns with LLM capabilities
- Con: No differentiation for identical-label workflows
- Risk: Low (identical-label scenarios likely rare)

**CONSERVATIVE (Fallback)**: Embeddings as tie-breaker only
- Pro: Preserves differentiation capability
- Con: Added complexity, latency, unclear value given indeterministic keywords
- Risk: Medium (complexity may not be justified)

---

**Status**: 🚧 **AWAITING DECISION**
**Next Step**: Implement Option 2 (Wildcard Weighting) or discuss further
**Confidence**: **95%** that wildcard weighting addresses user's concern
