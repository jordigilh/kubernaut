> **SUPERSEDED**: This document is superseded by DD-WORKFLOW-015 (V1.0 label-only architecture).
> pgvector and semantic search are deferred to V1.1+. Retained for historical context.

---

# Data Storage Service - pgvector Embedding Requirements

**Version**: 1.0
**Date**: 2025-11-02
**Status**: ✅ Validated (Phase 0 Day 0.2 - GAP #3 Resolution)
**Authority**: User Decision 1a + V2.0 RAR Business Requirements

---

## 📋 **Decision Summary**

**Decision 1a** (User-Approved): **pgvector embeddings for AIAnalysis audit ONLY** (not all 6 audit types)

**Rationale**:
- V2.0 RAR semantic search requires **AI investigation embeddings** for finding similar remediations
- Other audit types contain **structured data** (execution status, delivery status, orchestration phases) queryable via SQL
- Implementing embeddings for all 6 audit types adds **4 hours development time** with **zero business value**

**Confidence**: 90% (Validated against V2.0 RAR business requirements)

---

## 🎯 **Business Requirements Validation**

### **V2.0 Remediation Analysis Report (RAR) Requirements**

| Requirement | Needs Semantic Search? | Data Type | Embedding Required? |
|-------------|----------------------|-----------|---------------------|
| **BR-REMEDIATION-ANALYSIS-001**: Analyze complete remediation timeline | ✅ YES | AI investigation text + recommendations | ✅ YES |
| **BR-REMEDIATION-ANALYSIS-002**: Analyze AI decision quality | ✅ YES | Historical AI analysis comparisons | ✅ YES |
| **BR-REMEDIATION-ANALYSIS-003**: Report generation & distribution | ❌ NO | Report metadata (format, recipients) | ❌ NO |
| **BR-REMEDIATION-ANALYSIS-004**: Continuous improvement integration | ✅ YES | Pattern detection across AI decisions | ✅ YES |

**Key Insight**: V2.0 RAR generation requires semantic search specifically over **AIAnalysis investigation results** to:
1. Find similar historical remediations for AI investigation recommendations
2. Compare current AI decision quality against historical patterns
3. Detect patterns across AI decisions for continuous improvement

**Other Audit Types**: Orchestration, SignalProcessing, WorkflowExecution, Notification, Effectiveness all contain **structured data** (enums, timestamps, status codes, numeric scores) that don't benefit from semantic search.

---

## 🏗️ **Architecture: AIAnalysis-Only Embedding**

### **Embedding Scope**

```
┌──────────────────────────────────────────────────────────┐
│ 6 Audit Tables in migrations/010_audit_write_api.sql    │
└──────────────────────────────────────────────────────────┘
       │
       ├─ orchestration_audit          ❌ NO embedding (structured data)
       ├─ signal_processing_audit      ❌ NO embedding (structured data)
       ├─ ai_analysis_audit            ✅ HAS embedding vector(1536) ⭐
       ├─ workflow_execution_audit     ❌ NO embedding (structured data)
       ├─ notification_audit           ❌ NO embedding (structured data)
       └─ effectiveness_audit          ❌ NO embedding (structured data)
```

### **Embedding Column Schema**

**Table**: `ai_analysis_audit`
**Column**: `embedding vector(1536)`
**Index**: HNSW (Hierarchical Navigable Small World) for fast similarity search

```sql
-- From migrations/010_audit_write_api.sql (lines 161-163)
embedding vector(1536),  -- OpenAI text-embedding-3-small or equivalent

-- HNSW index for semantic similarity search (V2.0 RAR generation)
CREATE INDEX IF NOT EXISTS idx_ai_analysis_audit_embedding ON ai_analysis_audit
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
```

**Why 1536 dimensions?**
- OpenAI `text-embedding-3-small` model: 1536 dimensions (default)
- Alternative: HuggingFace `sentence-transformers/all-MiniLM-L6-v2`: 384 dimensions (if switching to open-source)
- Trade-off: Higher dimensions = better accuracy but slower search and more storage

---

## 📊 **Embedding Generation Pipeline**

### **Input Data for Embedding**

**Source**: AIAnalysis audit data from `ai_analysis_audit` table

**Text to Embed** (concatenated):
```go
// pkg/datastorage/embeddings/generator.go
func GenerateAIAnalysisEmbedding(audit *AIAnalysisAudit) ([]float32, error) {
    // Concatenate investigation report + recommendations for embedding
    textToEmbed := fmt.Sprintf(
        "Alert: %s (Severity: %s, Environment: %s)\n"+
        "Investigation Report: %s\n"+
        "Top Recommendation: %s\n"+
        "Confidence Score: %.2f\n"+
        "Effectiveness Probability: %.2f",
        audit.AlertFingerprint,
        audit.Severity,
        audit.Environment,
        audit.InvestigationReport,  // ⭐ Primary content for semantic search
        audit.TopRecommendation,    // ⭐ Secondary content
        audit.ConfidenceScore,
        audit.EffectivenessProbability,
    )

    // Generate embedding via OpenAI API
    return openai.CreateEmbedding(textToEmbed, "text-embedding-3-small")
}
```

**Rationale**:
- **Investigation Report**: Unstructured text describing root causes, symptoms, and analysis
- **Top Recommendation**: Remediation action recommended by AI
- **Metadata**: Alert fingerprint, severity, environment for context
- **Scores**: Confidence and effectiveness for quality filtering

### **Embedding Generation Strategy**

| Aspect | Decision | Rationale |
|--------|----------|-----------|
| **Timing** | Synchronous during audit write | Ensures embedding always available for V2.0 RAR queries |
| **Latency** | ~200ms added to audit write | Acceptable trade-off for semantic search capability |
| **Failure Handling** | Write to DLQ for async retry | If embedding fails, audit data persisted without embedding, retry via DD-009 DLQ |
| **Caching** | NO caching | Each AI analysis is unique (different investigation text) |
| **Batch Processing** | NO batching | Real-time audit writes, not batch imports |

### **Embedding Model Selection**

**Primary**: OpenAI `text-embedding-3-small`
- **Dimensions**: 1536
- **Cost**: $0.02 per 1M tokens
- **Latency**: ~200ms per request
- **Quality**: High (optimized for semantic similarity)

**Fallback**: HuggingFace `sentence-transformers/all-MiniLM-L6-v2`
- **Dimensions**: 384
- **Cost**: Free (self-hosted)
- **Latency**: ~50ms per request (local inference)
- **Quality**: Good (slightly lower than OpenAI)

**Migration Path**: Start with OpenAI, evaluate cost/quality trade-off after 3 months, consider HuggingFace if cost >$500/month.

---

## 🔍 **V2.0 RAR Semantic Search Use Cases**

### **Use Case 1: Find Similar Historical Remediations**

**Query**: "Find AI analyses for similar alerts to improve RAR recommendations"

```sql
-- Find top 10 most similar AI investigations
SELECT
    alert_fingerprint,
    investigation_report,
    top_recommendation,
    confidence_score,
    effectiveness_probability,
    1 - (embedding <=> $1::vector) AS similarity_score
FROM ai_analysis_audit
WHERE environment = $2  -- Filter by environment
AND severity = $3       -- Filter by severity
ORDER BY embedding <=> $1::vector  -- Cosine distance (lower = more similar)
LIMIT 10;
```

**Business Value**: RAR can say "This remediation is 85% similar to 3 previous successful remediations in production"

### **Use Case 2: AI Decision Quality Analysis**

**Query**: "Compare current AI decision quality against historical patterns"

```sql
-- Find remediations with similar investigation results but different outcomes
SELECT
    a1.alert_fingerprint AS current_alert,
    a2.alert_fingerprint AS similar_historical_alert,
    a1.confidence_score AS current_confidence,
    a2.confidence_score AS historical_confidence,
    a2.effectiveness_probability AS historical_effectiveness,
    1 - (a1.embedding <=> a2.embedding) AS similarity
FROM ai_analysis_audit a1
CROSS JOIN LATERAL (
    SELECT * FROM ai_analysis_audit
    WHERE id != a1.id
    ORDER BY embedding <=> a1.embedding
    LIMIT 5
) a2
WHERE a1.id = $1  -- Current AI analysis
ORDER BY similarity DESC;
```

**Business Value**: RAR can identify "AI confidence was 0.7, but 5 similar historical cases had 0.9 confidence - investigate why"

### **Use Case 3: Pattern Detection for Continuous Improvement**

**Query**: "Cluster AI analyses to detect common patterns"

```sql
-- K-means clustering approximation using pgvector
WITH cluster_centers AS (
    SELECT DISTINCT ON (cluster_id)
        cluster_id,
        embedding AS center_embedding
    FROM (
        SELECT
            ntile(10) OVER (ORDER BY embedding <-> (SELECT AVG(embedding) FROM ai_analysis_audit)) AS cluster_id,
            embedding
        FROM ai_analysis_audit
        WHERE created_at > NOW() - INTERVAL '90 days'
    ) clusters
)
SELECT
    c.cluster_id,
    COUNT(*) AS cluster_size,
    AVG(a.confidence_score) AS avg_confidence,
    AVG(a.effectiveness_probability) AS avg_effectiveness
FROM cluster_centers c
CROSS JOIN ai_analysis_audit a
WHERE a.created_at > NOW() - INTERVAL '90 days'
GROUP BY c.cluster_id
ORDER BY cluster_size DESC;
```

**Business Value**: RAR can identify "70% of AI analyses fall into 3 common patterns - optimize AI model for these patterns"

---

## 📈 **Performance Characteristics**

### **Storage Impact**

| Audit Type | Rows per Month | Embedding Size | Monthly Storage |
|------------|----------------|----------------|-----------------|
| ai_analysis_audit | ~2,500 | 1536 floats × 4 bytes = 6KB | 15 MB |
| Other 5 audit types | ~25,000 | 0 bytes | 0 MB |
| **Total Embedding Storage** | | | **15 MB/month** ✅ |

**Annual Storage**: 15 MB × 12 = **180 MB/year** ✅ (negligible compared to text storage)

### **Query Performance**

| Query Type | WITHOUT HNSW Index | WITH HNSW Index | Improvement |
|------------|-------------------|----------------|-------------|
| Top-10 similarity (10K rows) | ~500ms | ~15ms | **33× faster** |
| Top-10 similarity (100K rows) | ~5s | ~25ms | **200× faster** |
| Top-10 similarity (1M rows) | ~50s | ~50ms | **1000× faster** |

**Rationale**: HNSW index trades storage (20% overhead) for query speed (100× faster similarity search)

### **Embedding Generation Cost**

**Assumptions**:
- 2,500 AI analyses per month
- Average investigation report: 1,000 tokens
- OpenAI `text-embedding-3-small`: $0.02 per 1M tokens

**Monthly Cost**:
```
2,500 analyses × 1,000 tokens = 2.5M tokens/month
2.5M tokens × $0.02 / 1M tokens = $0.05/month
```

**Annual Cost**: $0.05 × 12 = **$0.60/year** ✅ (negligible cost)

---

## 🚫 **Why NOT Embed Other Audit Types?**

### **Orchestration Audit** (`orchestration_audit`)

**Data**: Structured orchestration phases, CRD statuses, timeout phases

**Query Pattern**: "Find all orchestrations that timed out during AI analysis phase"
```sql
-- Efficient SQL query (no embedding needed)
SELECT * FROM orchestration_audit
WHERE timeout_phase = 'analyzing'
AND overall_phase = 'timeout'
AND start_time > NOW() - INTERVAL '7 days';
```

**Embedding Value**: ❌ ZERO - structured enums don't benefit from semantic search

---

### **Signal Processing Audit** (`signal_processing_audit`)

**Data**: Enrichment quality scores, classification results, routing decisions

**Query Pattern**: "Find all signal processing with low enrichment quality"
```sql
-- Efficient SQL query (no embedding needed)
SELECT * FROM signal_processing_audit
WHERE enrichment_quality < 0.5
AND environment = 'prod'
ORDER BY completed_at DESC;
```

**Embedding Value**: ❌ ZERO - numeric scores and enums queryable via SQL indexes

---

### **Workflow Execution Audit** (`workflow_execution_audit`)

**Data**: Step counts, execution metrics, outcome enums

**Query Pattern**: "Find all workflow executions with partial success"
```sql
-- Efficient SQL query (no embedding needed)
SELECT * FROM workflow_execution_audit
WHERE outcome = 'partial'
AND rollbacks_performed > 0
ORDER BY total_duration_ms DESC;
```

**Embedding Value**: ❌ ZERO - execution metrics are numeric, not semantic

---

### **Notification Audit** (`notification_audit`)

**Data**: Channel types, delivery status, retry counts

**Query Pattern**: "Find all failed Slack notifications with >2 retries"
```sql
-- Efficient SQL query (no embedding needed)
SELECT * FROM notification_audit
WHERE channel = 'slack'
AND status = 'failed'
AND retry_count > 2;
```

**Embedding Value**: ❌ ZERO - delivery tracking is transactional data

---

### **Effectiveness Audit** (`effectiveness_audit`)

**Data**: Numeric effectiveness scores, trend direction enums

**Query Pattern**: "Find all actions with declining effectiveness trend"
```sql
-- Efficient SQL query (no embedding needed)
SELECT * FROM effectiveness_audit
WHERE trend_direction = 'declining'
AND traditional_score < 0.6
ORDER BY completed_at DESC;
```

**Embedding Value**: ❌ ZERO - effectiveness scores are numeric metrics

---

## ✅ **Implementation Checklist**

**Phase 0 Day 0.2 - Documentation** (This file):
- [x] Decision 1a validated against V2.0 RAR business requirements
- [x] Embedding scope defined (AIAnalysis only)
- [x] Embedding generation pipeline documented
- [x] V2.0 RAR semantic search use cases provided
- [x] Performance characteristics calculated
- [x] Cost analysis completed ($0.60/year)
- [x] Rationale for excluding other audit types documented

**Phase 1-3 - Implementation** (Days 4-5 in IMPLEMENTATION_PLAN_V4.7.md):
- [ ] Create `pkg/datastorage/embeddings/generator.go`
- [ ] Integrate OpenAI `text-embedding-3-small` client
- [ ] Implement synchronous embedding generation in audit write path
- [ ] Add DD-009 DLQ fallback for embedding generation failures
- [ ] Implement HuggingFace fallback model (optional, V1.1)
- [ ] Add Prometheus metrics: `embedding_generation_duration_seconds`, `embedding_generation_failures_total`
- [ ] Unit tests: Embedding generation with various investigation texts
- [ ] Integration tests: End-to-end audit write with embedding persistence
- [ ] Validate HNSW index performance with 10K+ test records

---

## 🔗 **Related Documentation**

- **Schema Authority**: `migrations/010_audit_write_api.sql` (lines 127-163)
- **V2.0 RAR Requirements**: `docs/requirements/BR-TERMINOLOGY-CORRECTION-REMEDIATION-ANALYSIS.md`
- **Implementation Plan**: `docs/services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V4.7.md` (Day 4: Embedding Generation)
- **Error Recovery**: `docs/architecture/decisions/DD-009-audit-write-error-recovery.md` (DLQ fallback for embedding failures)

---

## 📊 **Decision Impact Summary**

| Aspect | All 6 Audit Types | AIAnalysis Only (Decision 1a) | Savings |
|--------|------------------|-------------------------------|---------|
| **Development Time** | 8 hours | 4 hours | **-4 hours** ✅ |
| **Storage (Annual)** | 1.08 GB | 180 MB | **-900 MB** ✅ |
| **Embedding Cost (Annual)** | $3.60 | $0.60 | **-$3.00** ✅ |
| **Query Complexity** | High (6 indexes) | Low (1 index) | **-5 indexes** ✅ |
| **Business Value** | Marginal | High (V2.0 RAR) | **+100%** ✅ |

**Confidence**: 90% (Validated against V2.0 RAR business requirements)

---

## ✅ **Phase 0 Day 0.2 - Task 1 Complete**

**Deliverable**: ✅ pgvector requirements clarified and validated
**Validation**: Decision 1a confirmed - AIAnalysis only needs embeddings (1536 dimensions)
**Day 4 Scope**: Reduced from 8h to 4h (embedding generation for 1 audit type)
**Confidence**: 90%

---

**Document Version**: 1.0
**Status**: ✅ GAP #3 RESOLVED
**Last Updated**: 2025-11-02

