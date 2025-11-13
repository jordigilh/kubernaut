# Context API: Playbook Query for HolmesGPT API

**Version**: v1.0
**Last Updated**: November 11, 2025
**Status**: Design
**Target Version**: V1.0
**Related Decision**: [DD-CONTEXT-005: Minimal LLM Response Schema](../../../architecture/decisions/DD-CONTEXT-005-minimal-llm-response-schema.md)

---

## Purpose

Context API provides a playbook query endpoint for HolmesGPT API to retrieve relevant remediation playbooks during AI investigation. The LLM uses this list to recommend playbooks based on the incident context.

**Design Rationale**: See [DD-CONTEXT-005](../../../architecture/decisions/DD-CONTEXT-005-minimal-llm-response-schema.md) for the architectural decision to keep the response schema minimal (4 fields) and perform all filtering via query parameters.

---

## Endpoint Specification

```http
GET /api/v1/playbooks
```

### Query Parameters

| Parameter | Type | Required | Description | Example |
|-----------|------|----------|-------------|---------|
| `incident_type` | string | Yes | Incident type from alert | `pod-oom-killer` |
| `description` | string | No | Incident description for semantic search | `High memory usage causing pod restarts` |
| `labels` | string | No | Label filters (repeatable) | `labels=kubernaut.io/environment:prod-us-east&labels=kubernaut.io/priority:P0` |
| `max_results` | int | No | Maximum playbooks to return (default: 10) | `10` |

**Notes**:
- `incident_type` is mandatory - primary filter
- `description` enables semantic search if provided
- `labels` filter by playbook labels (Gateway environment/priority + Signal Processing business categorization)
- Always returns `status=active` playbooks only
- Always includes new playbooks (0 executions) with exploration bonus

**Label Examples**:
- Environment (from Gateway): `kubernaut.io/environment:prod-us-east`, `kubernaut.io/environment:staging`
- Priority (from Gateway): `kubernaut.io/priority:P0`, `kubernaut.io/priority:P1`
- Business Category (from Signal Processing): `kubernaut.io/business-category:payment-service`

---

## Response Schema

```json
{
  "playbooks": [
    {
      "playbook_id": "pod-oom-recovery",
      "version": "v1.2",
      "description": "Increases memory limits and restarts pod",
      "total_executions": 150,
      "success_rate": 0.95,
      "confidence": 0.92,
      "llm_summary": "This playbook has been executed 150 times with 95% success rate. Recently successful (last success: 1 day ago). Lower success rate in production (78%) vs staging (95%).",
      "key_insights": [
        "Lower success rate in production (78%) vs staging (95%)",
        "Recently successful (last success: 1 day ago)",
        "Success rate improving (+12% over last 30 days)"
      ],
      "quality_score": 0.95,
      "quality_factors": {
        "sample_size": "excellent",
        "recency": "good",
        "consistency": "excellent",
        "environment_match": "high"
      }
    },
    {
      "playbook_id": "pod-oom-recovery-ml-optimized",
      "version": "v1.0",
      "description": "ML-based memory optimization",
      "total_executions": 0,
      "success_rate": null,
      "confidence": 0.78,
      "llm_summary": "New playbook with no execution history. Consider for non-critical environments first.",
      "key_insights": [
        "🆕 New playbook - no historical data available",
        "⚠️ Recommend testing in staging before production"
      ],
      "quality_score": 0.50,
      "quality_factors": {
        "sample_size": "none",
        "recency": "n/a",
        "consistency": "n/a",
        "environment_match": "unknown"
      }
    }
  ],
  "total_results": 2
}
```

### Playbook Object Fields

| Field | Type | Description |
|-------|------|-------------|
| `playbook_id` | string | Unique playbook identifier |
| `version` | string | Playbook version (semantic versioning) |
| `description` | string | Human-readable description of what the playbook does |
| `total_executions` | int | Historical execution count (0 for new playbooks) |
| `success_rate` | float \| null | Historical success rate 0.0-1.0 (null for new playbooks) |
| `confidence` | float | Ranking score 0.0-1.0 (semantic + labels + performance + exploration bonus) |

**Rationale**: Minimal fields for LLM decision-making. LLM sees description, historical performance, and composite ranking.

---

## Composite Score Calculation

```
confidence = (
    0.3 * semantic_similarity +      // How well description matches incident
    0.4 * label_match_score +         // How well labels match filters
    0.3 * performance_score           // Historical success rate (0.5 default for new)
) + exploration_bonus                 // +0.05 for new playbooks
```

**Performance Score**:
- New playbooks (0 executions): `0.5` (neutral)
- Existing playbooks: `success_rate` (0.0-1.0)

**Exploration Bonus**:
- New playbooks: `+0.05` to encourage trying new solutions
- Existing playbooks: `0.0`

**Label Match Score**:
- All labels match: `1.0`
- Partial match: `matched_labels / total_labels`
- No labels: `0.5` (neutral)

---

## Implementation

### V1.0: Query Data Storage Service

```go
func (s *ContextAPIServer) handleGetPlaybooksForHolmesGPT(w http.ResponseWriter, r *http.Request) {
    incidentType := r.URL.Query().Get("incident_type")
    if incidentType == "" {
        s.respondError(w, http.StatusBadRequest, "incident_type is required")
        return
    }

    description := r.URL.Query().Get("description")
    labels := r.URL.Query()["labels"] // Repeatable parameter
    maxResults := parseIntOrDefault(r.URL.Query().Get("max_results"), 10)

    // Parse labels into map
    labelMap := parseLabels(labels)

    // Query Data Storage Service REST API
    playbooks, err := s.dataStorageClient.QueryPlaybooks(r.Context(), &datastorage.PlaybookQuery{
        IncidentType: incidentType,
        Labels:       labelMap,
        Status:       "active", // Only active playbooks
    })
    if err != nil {
        s.logger.Error("failed to query playbooks from Data Storage Service", zap.Error(err))
        s.respondError(w, http.StatusInternalServerError, "failed to query playbooks")
        return
    }

    // Semantic search (if description provided)
    if description != "" {
        playbooks, err = s.enhanceWithSemanticSearch(r.Context(), playbooks, description)
        if err != nil {
            s.logger.Warn("semantic search failed, continuing with label matches only", zap.Error(err))
        }
    }

    // Enrich with LLM-friendly summaries and quality scores
    enriched := s.enrichPlaybooks(playbooks)

    // Rank by composite score
    ranked := s.rankByCompositeScore(enriched, labelMap, description)

    // Limit results
    if len(ranked) > maxResults {
        ranked = ranked[:maxResults]
    }

    s.respondJSON(w, http.StatusOK, map[string]interface{}{
        "playbooks":     ranked,
        "total_results": len(ranked),
    })
}

func parseLabels(labelParams []string) map[string]string {
    labels := make(map[string]string)
    for _, param := range labelParams {
        parts := strings.SplitN(param, ":", 2)
        if len(parts) == 2 {
            labels[parts[0]] = parts[1]
        }
    }
    return labels
}

func (s *ContextAPIServer) enhanceWithSemanticSearch(ctx context.Context, playbooks []*Playbook, description string) ([]*Playbook, error) {
    // Generate embedding for incident description
    embedding, err := s.embedder.Generate(ctx, description)
    if err != nil {
        return playbooks, err
    }

    // Query Data Storage Service for semantic matches
    semanticMatches, err := s.dataStorageClient.SemanticSearchPlaybooks(ctx, &datastorage.SemanticSearchQuery{
        Embedding:  embedding,
        MaxResults: 20,
    })
    if err != nil {
        return playbooks, err
    }

    // Merge results (combine label matches with semantic matches)
    merged := s.mergePlaybooks(playbooks, semanticMatches)
    return merged, nil
}

func (s *ContextAPIServer) enrichPlaybooks(playbooks []*Playbook) []*EnrichedPlaybook {
    enriched := make([]*EnrichedPlaybook, len(playbooks))
    for i, pb := range playbooks {
        enriched[i] = &EnrichedPlaybook{
            Playbook:       pb,
            LLMSummary:     s.generateLLMSummary(pb),      // BR-CONTEXT-011
            KeyInsights:    s.generateKeyInsights(pb),      // BR-CONTEXT-011
            QualityScore:   s.calculateQualityScore(pb),   // BR-CONTEXT-012
            QualityFactors: s.calculateQualityFactors(pb), // BR-CONTEXT-012
        }
    }
    return enriched
}

// BR-CONTEXT-011: LLM-Friendly Summarization
func (s *ContextAPIServer) generateLLMSummary(pb *Playbook) string {
    if pb.TotalExecutions == 0 {
        return fmt.Sprintf("New playbook with no execution history. Consider for non-critical environments first.")
    }

    summary := fmt.Sprintf("This playbook has been executed %d times with %.0f%% success rate.",
        pb.TotalExecutions, pb.SuccessRate*100)

    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince < 7 {
            summary += fmt.Sprintf(" Recently successful (last success: %.0f days ago).", daysSince)
        }
    }

    return summary
}

// BR-CONTEXT-011: Key Insights Generation
func (s *ContextAPIServer) generateKeyInsights(pb *Playbook) []string {
    insights := []string{}

    if pb.TotalExecutions == 0 {
        insights = append(insights, "🆕 New playbook - no historical data available")
        insights = append(insights, "⚠️ Recommend testing in staging before production")
        return insights
    }

    // Environment-specific performance
    if pb.EnvironmentPerformance != nil {
        prodRate := pb.EnvironmentPerformance["production"]
        stagingRate := pb.EnvironmentPerformance["staging"]
        if prodRate > 0 && stagingRate > 0 && math.Abs(prodRate-stagingRate) > 0.1 {
            if prodRate < stagingRate {
                insights = append(insights, fmt.Sprintf("⚠️ Lower success rate in production (%.0f%%) vs staging (%.0f%%)",
                    prodRate*100, stagingRate*100))
            }
        }
    }

    // Recency
    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince < 7 {
            insights = append(insights, fmt.Sprintf("✅ Recently successful (last success: %.0f days ago)", daysSince))
        } else if daysSince > 30 {
            insights = append(insights, fmt.Sprintf("⚠️ Not used recently (last success: %.0f days ago)", daysSince))
        }
    }

    // Trend
    if pb.TrendDirection != "" {
        if pb.TrendDirection == "improving" {
            insights = append(insights, fmt.Sprintf("📈 Success rate improving (+%.0f%% over last 30 days)", pb.TrendPercent))
        } else if pb.TrendDirection == "declining" {
            insights = append(insights, fmt.Sprintf("📉 Success rate declining (%.0f%% over last 30 days)", pb.TrendPercent))
        }
    }

    return insights
}

// BR-CONTEXT-012: Quality Score Calculation
func (s *ContextAPIServer) calculateQualityScore(pb *Playbook) float64 {
    if pb.TotalExecutions == 0 {
        return 0.50 // Neutral for new playbooks
    }

    // Sample size score (40% weight)
    sampleScore := 0.0
    if pb.TotalExecutions >= 100 {
        sampleScore = 1.0
    } else if pb.TotalExecutions >= 50 {
        sampleScore = 0.8
    } else if pb.TotalExecutions >= 10 {
        sampleScore = 0.6
    } else {
        sampleScore = 0.3
    }

    // Recency score (30% weight)
    recencyScore := 0.5
    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince <= 7 {
            recencyScore = 1.0
        } else if daysSince <= 30 {
            recencyScore = 0.8
        } else if daysSince <= 90 {
            recencyScore = 0.6
        } else {
            recencyScore = 0.3
        }
    }

    // Consistency score (20% weight)
    consistencyScore := 1.0
    if pb.SuccessRateStdDev > 0.3 {
        consistencyScore = 0.3
    } else if pb.SuccessRateStdDev > 0.2 {
        consistencyScore = 0.6
    } else if pb.SuccessRateStdDev > 0.1 {
        consistencyScore = 0.8
    }

    // Environment match score (10% weight)
    envScore := 0.5 // Default neutral

    // Weighted average
    qualityScore := (0.4 * sampleScore) + (0.3 * recencyScore) + (0.2 * consistencyScore) + (0.1 * envScore)
    return qualityScore
}

// BR-CONTEXT-012: Quality Factors
func (s *ContextAPIServer) calculateQualityFactors(pb *Playbook) map[string]string {
    factors := make(map[string]string)

    if pb.TotalExecutions == 0 {
        factors["sample_size"] = "none"
        factors["recency"] = "n/a"
        factors["consistency"] = "n/a"
        factors["environment_match"] = "unknown"
        return factors
    }

    // Sample size
    if pb.TotalExecutions >= 100 {
        factors["sample_size"] = "excellent"
    } else if pb.TotalExecutions >= 50 {
        factors["sample_size"] = "good"
    } else if pb.TotalExecutions >= 10 {
        factors["sample_size"] = "fair"
    } else {
        factors["sample_size"] = "poor"
    }

    // Recency
    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince <= 7 {
            factors["recency"] = "excellent"
        } else if daysSince <= 30 {
            factors["recency"] = "good"
        } else if daysSince <= 90 {
            factors["recency"] = "fair"
        } else {
            factors["recency"] = "poor"
        }
    } else {
        factors["recency"] = "unknown"
    }

    // Consistency
    if pb.SuccessRateStdDev < 0.1 {
        factors["consistency"] = "excellent"
    } else if pb.SuccessRateStdDev < 0.2 {
        factors["consistency"] = "good"
    } else if pb.SuccessRateStdDev < 0.3 {
        factors["consistency"] = "fair"
    } else {
        factors["consistency"] = "poor"
    }

    factors["environment_match"] = "high" // Default

    return factors
}
```

---

## Database Schema (V1.0)

**Note**: Context API queries Data Storage Service REST API, which accesses these tables.

```sql
-- Playbooks table
CREATE TABLE remediation_playbooks (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    description TEXT NOT NULL,
    incident_types TEXT[] NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    steps JSONB NOT NULL,

    PRIMARY KEY (playbook_id, version)
);

-- Playbook labels table
CREATE TABLE playbook_labels (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    label_key VARCHAR(100) NOT NULL,
    label_value VARCHAR(200) NOT NULL,

    PRIMARY KEY (playbook_id, version, label_key),
    FOREIGN KEY (playbook_id, version) REFERENCES remediation_playbooks(playbook_id, version)
);

-- Playbook embeddings table (for semantic search)
CREATE TABLE playbook_embeddings (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    embedding vector(1536),  -- OpenAI text-embedding-3-small

    PRIMARY KEY (playbook_id, version),
    FOREIGN KEY (playbook_id, version) REFERENCES remediation_playbooks(playbook_id, version)
);

CREATE INDEX idx_playbook_embeddings_vector ON playbook_embeddings
USING ivfflat (embedding vector_cosine_ops);

-- Multi-dimensional success tracking (from ADR-033)
CREATE TABLE playbook_success_rates (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    dimension_type VARCHAR(50) NOT NULL,  -- 'environment', 'priority', 'business_category'
    dimension_value VARCHAR(100) NOT NULL,
    total_executions INT NOT NULL DEFAULT 0,
    successful_executions INT NOT NULL DEFAULT 0,
    failed_executions INT NOT NULL DEFAULT 0,
    success_rate DECIMAL(5,4) NOT NULL DEFAULT 0.0,
    last_executed_at TIMESTAMP,

    PRIMARY KEY (playbook_id, version, dimension_type, dimension_value),
    FOREIGN KEY (playbook_id, version) REFERENCES remediation_playbooks(playbook_id, version)
);

CREATE INDEX idx_playbook_success_rates_lookup ON playbook_success_rates(playbook_id, version, dimension_type);
```

---

## V1.1 Changes

**No changes to Context API query logic.**

V1.1 adds `PlaybookRegistration` CRD controller (separate service) that:
1. Watches `PlaybookRegistration` CRDs
2. Syncs to `remediation_playbooks` table
3. Generates embeddings on CRD creation
4. Updates database when CRD changes

**Note**: V1.1 Playbook Catalog is a **pure CRD controller** with NO REST API. Context API continues querying Data Storage Service REST API, which accesses the same database tables.

---

## Caching Strategy (BR-CONTEXT-013)

### Multi-Level Caching

**Level 1: HolmesGPT API Session Cache**
- In-memory cache within HolmesGPT API
- TTL: 5 minutes
- Scope: Per investigation session

**Level 2: Context API Redis Cache**
- Distributed cache in Redis
- TTL: 15 minutes
- Scope: All Context API instances

**Level 3: Semantic Similarity Cache**
- Cache for similar incident descriptions
- TTL: 15 minutes
- Invalidated when new playbook executions added

### Cache Invalidation

Context API listens for remediation completion events:
1. Effectiveness Monitor writes to `effectiveness_results` table
2. Data Storage Service writes to `remediation_audit` table
3. Context API subscribes to PostgreSQL NOTIFY/LISTEN for changes
4. Invalidates cache for affected playbooks

---

## Related Documents

- [BR-CONTEXT-011: LLM-Friendly Context Summarization](../../requirements/BR-CONTEXT-011-llm-friendly-summarization.md)
- [BR-CONTEXT-012: Context Quality Scoring](../../requirements/BR-CONTEXT-012-quality-scoring.md)
- [BR-CONTEXT-013: Context Caching Strategy](../../requirements/BR-CONTEXT-013-caching-strategy.md)
- [BR-PLAYBOOK-001: Playbook Registry Management](../../requirements/BR-PLAYBOOK-001-playbook-registry-management.md)
- [ADR-033: Remediation Playbook Catalog](../../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-035: Remediation Execution Engine](../../architecture/decisions/ADR-035-remediation-execution-engine.md)
- [HolmesGPT API Overview](../holmesgpt-api/overview.md)
- [Data Storage Service API](../data-storage/api-specification.md)

---

## Architecture Evolution

### V1.0 (Current)
```
HolmesGPT API
    ↓ (tool call)
Context API
    ↓ (REST API)
Data Storage Service
    ↓ (SQL)
PostgreSQL (remediation_playbooks, playbook_labels, playbook_embeddings)
```

### V1.1 (Next Release)
```
kubectl apply -f playbook.yaml
    ↓
PlaybookRegistration CRD
    ↓
Playbook Catalog CRD Controller (watches CRDs)
    ↓ (syncs to database)
PostgreSQL (remediation_playbooks, playbook_labels, playbook_embeddings)
    ↑ (queries via REST API)
Data Storage Service
    ↑ (REST API)
Context API
    ↑ (tool call)
HolmesGPT API
```

**Key Points**:
- V1.0: Playbooks managed via SQL
- V1.1: Playbooks managed via CRDs (Playbook Catalog CRD controller syncs to database)
- Context API always queries Data Storage Service REST API (not database directly)
- No REST API for Playbook Catalog Service in V1.1 (pure CRD controller)

---

**Document Version**: 1.1
**Last Updated**: November 11, 2025
**Status**: ✅ **READY FOR IMPLEMENTATION**

**Last Updated**: November 11, 2025
**Status**: Design
**Target Version**: V1.0

---

## Purpose

Context API provides a playbook query endpoint for HolmesGPT API to retrieve relevant remediation playbooks during AI investigation. The LLM uses this list to recommend playbooks based on the incident context.

---

## Endpoint Specification

```http
GET /api/v1/playbooks
```

### Query Parameters

| Parameter | Type | Required | Description | Example |
|-----------|------|----------|-------------|---------|
| `incident_type` | string | Yes | Incident type from alert | `pod-oom-killer` |
| `description` | string | No | Incident description for semantic search | `High memory usage causing pod restarts` |
| `labels` | string | No | Label filters (repeatable) | `labels=kubernaut.io/environment:prod-us-east&labels=kubernaut.io/priority:P0` |
| `max_results` | int | No | Maximum playbooks to return (default: 10) | `10` |

**Notes**:
- `incident_type` is mandatory - primary filter
- `description` enables semantic search if provided
- `labels` filter by playbook labels (Gateway environment/priority + Signal Processing business categorization)
- Always returns `status=active` playbooks only
- Always includes new playbooks (0 executions) with exploration bonus

**Label Examples**:
- Environment (from Gateway): `kubernaut.io/environment:prod-us-east`, `kubernaut.io/environment:staging`
- Priority (from Gateway): `kubernaut.io/priority:P0`, `kubernaut.io/priority:P1`
- Business Category (from Signal Processing): `kubernaut.io/business-category:payment-service`

---

## Response Schema

```json
{
  "playbooks": [
    {
      "playbook_id": "pod-oom-recovery",
      "version": "v1.2",
      "description": "Increases memory limits and restarts pod",
      "confidence": 0.92
    },
    {
      "playbook_id": "pod-oom-recovery-ml-optimized",
      "version": "v1.0",
      "description": "ML-based memory optimization",
      "confidence": 0.78
    }
  ],
  "total_results": 2
}
```

### Playbook Object Fields

| Field | Type | Description | BR Reference |
|-------|------|-------------|--------------|
| `playbook_id` | string | Unique playbook identifier | Core |
| `version` | string | Playbook version (semantic versioning) | Core |
| `description` | string | Human-readable description of what the playbook does | Core |
| `total_executions` | int | Historical execution count (0 for new playbooks) | Core |
| `success_rate` | float \| null | Historical success rate 0.0-1.0 (null for new playbooks) | Core |
| `confidence` | float | Ranking score 0.0-1.0 (semantic + labels + performance + exploration bonus) | Core |
| `llm_summary` | string | Natural language summary for LLM consumption | BR-CONTEXT-011 |
| `key_insights` | string[] | 3-5 key insights with emoji indicators | BR-CONTEXT-011 |
| `quality_score` | float | Data quality score 0.0-1.0 | BR-CONTEXT-012 |
| `quality_factors` | object | Quality score breakdown | BR-CONTEXT-012 |

**Rationale**: Minimal core fields + LLM-friendly enhancements. LLM sees description, historical performance, composite ranking, and actionable insights.

---

## Composite Score Calculation

```
confidence = (
    0.3 * semantic_similarity +      // How well description matches incident
    0.4 * label_match_score +         // How well labels match filters
    0.3 * performance_score           // Historical success rate (0.5 default for new)
) + exploration_bonus                 // +0.05 for new playbooks
```

**Performance Score**:
- New playbooks (0 executions): `0.5` (neutral)
- Existing playbooks: `success_rate` (0.0-1.0)

**Exploration Bonus**:
- New playbooks: `+0.05` to encourage trying new solutions
- Existing playbooks: `0.0`

**Label Match Score**:
- All labels match: `1.0`
- Partial match: `matched_labels / total_labels`
- No labels: `0.5` (neutral)

---

## Implementation

### V1.0: Query Data Storage Service

```go
func (s *ContextAPIServer) handleGetPlaybooksForHolmesGPT(w http.ResponseWriter, r *http.Request) {
    incidentType := r.URL.Query().Get("incident_type")
    if incidentType == "" {
        s.respondError(w, http.StatusBadRequest, "incident_type is required")
        return
    }

    description := r.URL.Query().Get("description")
    labels := r.URL.Query()["labels"] // Repeatable parameter
    maxResults := parseIntOrDefault(r.URL.Query().Get("max_results"), 10)

    // Parse labels into map
    labelMap := parseLabels(labels)

    // Query Data Storage Service REST API
    playbooks, err := s.dataStorageClient.QueryPlaybooks(r.Context(), &datastorage.PlaybookQuery{
        IncidentType: incidentType,
        Labels:       labelMap,
        Status:       "active", // Only active playbooks
    })
    if err != nil {
        s.logger.Error("failed to query playbooks from Data Storage Service", zap.Error(err))
        s.respondError(w, http.StatusInternalServerError, "failed to query playbooks")
        return
    }

    // Semantic search (if description provided)
    if description != "" {
        playbooks, err = s.enhanceWithSemanticSearch(r.Context(), playbooks, description)
        if err != nil {
            s.logger.Warn("semantic search failed, continuing with label matches only", zap.Error(err))
        }
    }

    // Enrich with LLM-friendly summaries and quality scores
    enriched := s.enrichPlaybooks(playbooks)

    // Rank by composite score
    ranked := s.rankByCompositeScore(enriched, labelMap, description)

    // Limit results
    if len(ranked) > maxResults {
        ranked = ranked[:maxResults]
    }

    s.respondJSON(w, http.StatusOK, map[string]interface{}{
        "playbooks":     ranked,
        "total_results": len(ranked),
    })
}

func parseLabels(labelParams []string) map[string]string {
    labels := make(map[string]string)
    for _, param := range labelParams {
        parts := strings.SplitN(param, ":", 2)
        if len(parts) == 2 {
            labels[parts[0]] = parts[1]
        }
    }
    return labels
}

func (s *ContextAPIServer) enhanceWithSemanticSearch(ctx context.Context, playbooks []*Playbook, description string) ([]*Playbook, error) {
    // Generate embedding for incident description
    embedding, err := s.embedder.Generate(ctx, description)
    if err != nil {
        return playbooks, err
    }

    // Query Data Storage Service for semantic matches
    semanticMatches, err := s.dataStorageClient.SemanticSearchPlaybooks(ctx, &datastorage.SemanticSearchQuery{
        Embedding:  embedding,
        MaxResults: 20,
    })
    if err != nil {
        return playbooks, err
    }

    // Merge results (combine label matches with semantic matches)
    merged := s.mergePlaybooks(playbooks, semanticMatches)
    return merged, nil
}

func (s *ContextAPIServer) enrichPlaybooks(playbooks []*Playbook) []*EnrichedPlaybook {
    enriched := make([]*EnrichedPlaybook, len(playbooks))
    for i, pb := range playbooks {
        enriched[i] = &EnrichedPlaybook{
            Playbook:       pb,
            LLMSummary:     s.generateLLMSummary(pb),      // BR-CONTEXT-011
            KeyInsights:    s.generateKeyInsights(pb),      // BR-CONTEXT-011
            QualityScore:   s.calculateQualityScore(pb),   // BR-CONTEXT-012
            QualityFactors: s.calculateQualityFactors(pb), // BR-CONTEXT-012
        }
    }
    return enriched
}

// BR-CONTEXT-011: LLM-Friendly Summarization
func (s *ContextAPIServer) generateLLMSummary(pb *Playbook) string {
    if pb.TotalExecutions == 0 {
        return fmt.Sprintf("New playbook with no execution history. Consider for non-critical environments first.")
    }

    summary := fmt.Sprintf("This playbook has been executed %d times with %.0f%% success rate.",
        pb.TotalExecutions, pb.SuccessRate*100)

    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince < 7 {
            summary += fmt.Sprintf(" Recently successful (last success: %.0f days ago).", daysSince)
        }
    }

    return summary
}

// BR-CONTEXT-011: Key Insights Generation
func (s *ContextAPIServer) generateKeyInsights(pb *Playbook) []string {
    insights := []string{}

    if pb.TotalExecutions == 0 {
        insights = append(insights, "🆕 New playbook - no historical data available")
        insights = append(insights, "⚠️ Recommend testing in staging before production")
        return insights
    }

    // Environment-specific performance
    if pb.EnvironmentPerformance != nil {
        prodRate := pb.EnvironmentPerformance["production"]
        stagingRate := pb.EnvironmentPerformance["staging"]
        if prodRate > 0 && stagingRate > 0 && math.Abs(prodRate-stagingRate) > 0.1 {
            if prodRate < stagingRate {
                insights = append(insights, fmt.Sprintf("⚠️ Lower success rate in production (%.0f%%) vs staging (%.0f%%)",
                    prodRate*100, stagingRate*100))
            }
        }
    }

    // Recency
    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince < 7 {
            insights = append(insights, fmt.Sprintf("✅ Recently successful (last success: %.0f days ago)", daysSince))
        } else if daysSince > 30 {
            insights = append(insights, fmt.Sprintf("⚠️ Not used recently (last success: %.0f days ago)", daysSince))
        }
    }

    // Trend
    if pb.TrendDirection != "" {
        if pb.TrendDirection == "improving" {
            insights = append(insights, fmt.Sprintf("📈 Success rate improving (+%.0f%% over last 30 days)", pb.TrendPercent))
        } else if pb.TrendDirection == "declining" {
            insights = append(insights, fmt.Sprintf("📉 Success rate declining (%.0f%% over last 30 days)", pb.TrendPercent))
        }
    }

    return insights
}

// BR-CONTEXT-012: Quality Score Calculation
func (s *ContextAPIServer) calculateQualityScore(pb *Playbook) float64 {
    if pb.TotalExecutions == 0 {
        return 0.50 // Neutral for new playbooks
    }

    // Sample size score (40% weight)
    sampleScore := 0.0
    if pb.TotalExecutions >= 100 {
        sampleScore = 1.0
    } else if pb.TotalExecutions >= 50 {
        sampleScore = 0.8
    } else if pb.TotalExecutions >= 10 {
        sampleScore = 0.6
    } else {
        sampleScore = 0.3
    }

    // Recency score (30% weight)
    recencyScore := 0.5
    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince <= 7 {
            recencyScore = 1.0
        } else if daysSince <= 30 {
            recencyScore = 0.8
        } else if daysSince <= 90 {
            recencyScore = 0.6
        } else {
            recencyScore = 0.3
        }
    }

    // Consistency score (20% weight)
    consistencyScore := 1.0
    if pb.SuccessRateStdDev > 0.3 {
        consistencyScore = 0.3
    } else if pb.SuccessRateStdDev > 0.2 {
        consistencyScore = 0.6
    } else if pb.SuccessRateStdDev > 0.1 {
        consistencyScore = 0.8
    }

    // Environment match score (10% weight)
    envScore := 0.5 // Default neutral

    // Weighted average
    qualityScore := (0.4 * sampleScore) + (0.3 * recencyScore) + (0.2 * consistencyScore) + (0.1 * envScore)
    return qualityScore
}

// BR-CONTEXT-012: Quality Factors
func (s *ContextAPIServer) calculateQualityFactors(pb *Playbook) map[string]string {
    factors := make(map[string]string)

    if pb.TotalExecutions == 0 {
        factors["sample_size"] = "none"
        factors["recency"] = "n/a"
        factors["consistency"] = "n/a"
        factors["environment_match"] = "unknown"
        return factors
    }

    // Sample size
    if pb.TotalExecutions >= 100 {
        factors["sample_size"] = "excellent"
    } else if pb.TotalExecutions >= 50 {
        factors["sample_size"] = "good"
    } else if pb.TotalExecutions >= 10 {
        factors["sample_size"] = "fair"
    } else {
        factors["sample_size"] = "poor"
    }

    // Recency
    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince <= 7 {
            factors["recency"] = "excellent"
        } else if daysSince <= 30 {
            factors["recency"] = "good"
        } else if daysSince <= 90 {
            factors["recency"] = "fair"
        } else {
            factors["recency"] = "poor"
        }
    } else {
        factors["recency"] = "unknown"
    }

    // Consistency
    if pb.SuccessRateStdDev < 0.1 {
        factors["consistency"] = "excellent"
    } else if pb.SuccessRateStdDev < 0.2 {
        factors["consistency"] = "good"
    } else if pb.SuccessRateStdDev < 0.3 {
        factors["consistency"] = "fair"
    } else {
        factors["consistency"] = "poor"
    }

    factors["environment_match"] = "high" // Default

    return factors
}
```

---

## Database Schema (V1.0)

**Note**: Context API queries Data Storage Service REST API, which accesses these tables.

```sql
-- Playbooks table
CREATE TABLE remediation_playbooks (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    description TEXT NOT NULL,
    incident_types TEXT[] NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    steps JSONB NOT NULL,

    PRIMARY KEY (playbook_id, version)
);

-- Playbook labels table
CREATE TABLE playbook_labels (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    label_key VARCHAR(100) NOT NULL,
    label_value VARCHAR(200) NOT NULL,

    PRIMARY KEY (playbook_id, version, label_key),
    FOREIGN KEY (playbook_id, version) REFERENCES remediation_playbooks(playbook_id, version)
);

-- Playbook embeddings table (for semantic search)
CREATE TABLE playbook_embeddings (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    embedding vector(1536),  -- OpenAI text-embedding-3-small

    PRIMARY KEY (playbook_id, version),
    FOREIGN KEY (playbook_id, version) REFERENCES remediation_playbooks(playbook_id, version)
);

CREATE INDEX idx_playbook_embeddings_vector ON playbook_embeddings
USING ivfflat (embedding vector_cosine_ops);

-- Multi-dimensional success tracking (from ADR-033)
CREATE TABLE playbook_success_rates (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    dimension_type VARCHAR(50) NOT NULL,  -- 'environment', 'priority', 'business_category'
    dimension_value VARCHAR(100) NOT NULL,
    total_executions INT NOT NULL DEFAULT 0,
    successful_executions INT NOT NULL DEFAULT 0,
    failed_executions INT NOT NULL DEFAULT 0,
    success_rate DECIMAL(5,4) NOT NULL DEFAULT 0.0,
    last_executed_at TIMESTAMP,

    PRIMARY KEY (playbook_id, version, dimension_type, dimension_value),
    FOREIGN KEY (playbook_id, version) REFERENCES remediation_playbooks(playbook_id, version)
);

CREATE INDEX idx_playbook_success_rates_lookup ON playbook_success_rates(playbook_id, version, dimension_type);
```

---

## V1.1 Changes

**No changes to Context API query logic.**

V1.1 adds `PlaybookRegistration` CRD controller (separate service) that:
1. Watches `PlaybookRegistration` CRDs
2. Syncs to `remediation_playbooks` table
3. Generates embeddings on CRD creation
4. Updates database when CRD changes

**Note**: V1.1 Playbook Catalog is a **pure CRD controller** with NO REST API. Context API continues querying Data Storage Service REST API, which accesses the same database tables.

---

## Caching Strategy (BR-CONTEXT-013)

### Multi-Level Caching

**Level 1: HolmesGPT API Session Cache**
- In-memory cache within HolmesGPT API
- TTL: 5 minutes
- Scope: Per investigation session

**Level 2: Context API Redis Cache**
- Distributed cache in Redis
- TTL: 15 minutes
- Scope: All Context API instances

**Level 3: Semantic Similarity Cache**
- Cache for similar incident descriptions
- TTL: 15 minutes
- Invalidated when new playbook executions added

### Cache Invalidation

Context API listens for remediation completion events:
1. Effectiveness Monitor writes to `effectiveness_results` table
2. Data Storage Service writes to `remediation_audit` table
3. Context API subscribes to PostgreSQL NOTIFY/LISTEN for changes
4. Invalidates cache for affected playbooks

---

## Related Documents

- [BR-CONTEXT-011: LLM-Friendly Context Summarization](../../requirements/BR-CONTEXT-011-llm-friendly-summarization.md)
- [BR-CONTEXT-012: Context Quality Scoring](../../requirements/BR-CONTEXT-012-quality-scoring.md)
- [BR-CONTEXT-013: Context Caching Strategy](../../requirements/BR-CONTEXT-013-caching-strategy.md)
- [BR-PLAYBOOK-001: Playbook Registry Management](../../requirements/BR-PLAYBOOK-001-playbook-registry-management.md)
- [ADR-033: Remediation Playbook Catalog](../../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-035: Remediation Execution Engine](../../architecture/decisions/ADR-035-remediation-execution-engine.md)
- [HolmesGPT API Overview](../holmesgpt-api/overview.md)
- [Data Storage Service API](../data-storage/api-specification.md)

---

## Architecture Evolution

### V1.0 (Current)
```
HolmesGPT API
    ↓ (tool call)
Context API
    ↓ (REST API)
Data Storage Service
    ↓ (SQL)
PostgreSQL (remediation_playbooks, playbook_labels, playbook_embeddings)
```

### V1.1 (Next Release)
```
kubectl apply -f playbook.yaml
    ↓
PlaybookRegistration CRD
    ↓
Playbook Catalog CRD Controller (watches CRDs)
    ↓ (syncs to database)
PostgreSQL (remediation_playbooks, playbook_labels, playbook_embeddings)
    ↑ (queries via REST API)
Data Storage Service
    ↑ (REST API)
Context API
    ↑ (tool call)
HolmesGPT API
```

**Key Points**:
- V1.0: Playbooks managed via SQL
- V1.1: Playbooks managed via CRDs (Playbook Catalog CRD controller syncs to database)
- Context API always queries Data Storage Service REST API (not database directly)
- No REST API for Playbook Catalog Service in V1.1 (pure CRD controller)

---

**Document Version**: 1.1
**Last Updated**: November 11, 2025
**Status**: ✅ **READY FOR IMPLEMENTATION**

**Last Updated**: November 11, 2025
**Status**: Design
**Target Version**: V1.0

---

## Purpose

Context API provides a playbook query endpoint for HolmesGPT API to retrieve relevant remediation playbooks during AI investigation. The LLM uses this list to recommend playbooks based on the incident context.

---

## Endpoint Specification

```http
GET /api/v1/playbooks
```

### Query Parameters

| Parameter | Type | Required | Description | Example |
|-----------|------|----------|-------------|---------|
| `incident_type` | string | Yes | Incident type from alert | `pod-oom-killer` |
| `description` | string | No | Incident description for semantic search | `High memory usage causing pod restarts` |
| `labels` | string | No | Label filters (repeatable) | `labels=kubernaut.io/environment:prod-us-east&labels=kubernaut.io/priority:P0` |
| `max_results` | int | No | Maximum playbooks to return (default: 10) | `10` |

**Notes**:
- `incident_type` is mandatory - primary filter
- `description` enables semantic search if provided
- `labels` filter by playbook labels (Gateway environment/priority + Signal Processing business categorization)
- Always returns `status=active` playbooks only
- Always includes new playbooks (0 executions) with exploration bonus

**Label Examples**:
- Environment (from Gateway): `kubernaut.io/environment:prod-us-east`, `kubernaut.io/environment:staging`
- Priority (from Gateway): `kubernaut.io/priority:P0`, `kubernaut.io/priority:P1`
- Business Category (from Signal Processing): `kubernaut.io/business-category:payment-service`

---

## Response Schema

```json
{
  "playbooks": [
    {
      "playbook_id": "pod-oom-recovery",
      "version": "v1.2",
      "description": "Increases memory limits and restarts pod",
      "total_executions": 150,
      "success_rate": 0.95,
      "confidence": 0.92,
      "llm_summary": "This playbook has been executed 150 times with 95% success rate.",
      "quality_score": 0.95,
      "quality_factors": {
        "sample_size": "excellent",
        "recency": "good",
        "consistency": "excellent",
        "environment_match": "high"
      }
    },
    {
      "playbook_id": "pod-oom-recovery-ml-optimized",
      "version": "v1.0",
      "description": "ML-based memory optimization",
      "total_executions": 0,
      "success_rate": null,
      "confidence": 0.78,
      "llm_summary": "New playbook with no execution history. Consider for non-critical environments first.",
      "key_insights": [
        "🆕 New playbook - no historical data available",
        "⚠️ Recommend testing in staging before production"
      ],
      "quality_score": 0.50,
      "quality_factors": {
        "sample_size": "none",
        "recency": "n/a",
        "consistency": "n/a",
        "environment_match": "unknown"
      }
    }
  ],
  "total_results": 2
}
```

### Playbook Object Fields

| Field | Type | Description | BR Reference |
|-------|------|-------------|--------------|
| `playbook_id` | string | Unique playbook identifier | Core |
| `version` | string | Playbook version (semantic versioning) | Core |
| `description` | string | Human-readable description of what the playbook does | Core |
| `total_executions` | int | Historical execution count (0 for new playbooks) | Core |
| `success_rate` | float \| null | Historical success rate 0.0-1.0 (null for new playbooks) | Core |
| `confidence` | float | Ranking score 0.0-1.0 (semantic + labels + performance + exploration bonus) | Core |
| `llm_summary` | string | Natural language summary for LLM consumption | BR-CONTEXT-011 |
| `key_insights` | string[] | 3-5 key insights with emoji indicators | BR-CONTEXT-011 |
| `quality_score` | float | Data quality score 0.0-1.0 | BR-CONTEXT-012 |
| `quality_factors` | object | Quality score breakdown | BR-CONTEXT-012 |

**Rationale**: Minimal core fields + LLM-friendly enhancements. LLM sees description, historical performance, composite ranking, and actionable insights.

---

## Composite Score Calculation

```
confidence = (
    0.3 * semantic_similarity +      // How well description matches incident
    0.4 * label_match_score +         // How well labels match filters
    0.3 * performance_score           // Historical success rate (0.5 default for new)
) + exploration_bonus                 // +0.05 for new playbooks
```

**Performance Score**:
- New playbooks (0 executions): `0.5` (neutral)
- Existing playbooks: `success_rate` (0.0-1.0)

**Exploration Bonus**:
- New playbooks: `+0.05` to encourage trying new solutions
- Existing playbooks: `0.0`

**Label Match Score**:
- All labels match: `1.0`
- Partial match: `matched_labels / total_labels`
- No labels: `0.5` (neutral)

---

## Implementation

### V1.0: Query Data Storage Service

```go
func (s *ContextAPIServer) handleGetPlaybooksForHolmesGPT(w http.ResponseWriter, r *http.Request) {
    incidentType := r.URL.Query().Get("incident_type")
    if incidentType == "" {
        s.respondError(w, http.StatusBadRequest, "incident_type is required")
        return
    }

    description := r.URL.Query().Get("description")
    labels := r.URL.Query()["labels"] // Repeatable parameter
    maxResults := parseIntOrDefault(r.URL.Query().Get("max_results"), 10)

    // Parse labels into map
    labelMap := parseLabels(labels)

    // Query Data Storage Service REST API
    playbooks, err := s.dataStorageClient.QueryPlaybooks(r.Context(), &datastorage.PlaybookQuery{
        IncidentType: incidentType,
        Labels:       labelMap,
        Status:       "active", // Only active playbooks
    })
    if err != nil {
        s.logger.Error("failed to query playbooks from Data Storage Service", zap.Error(err))
        s.respondError(w, http.StatusInternalServerError, "failed to query playbooks")
        return
    }

    // Semantic search (if description provided)
    if description != "" {
        playbooks, err = s.enhanceWithSemanticSearch(r.Context(), playbooks, description)
        if err != nil {
            s.logger.Warn("semantic search failed, continuing with label matches only", zap.Error(err))
        }
    }

    // Enrich with LLM-friendly summaries and quality scores
    enriched := s.enrichPlaybooks(playbooks)

    // Rank by composite score
    ranked := s.rankByCompositeScore(enriched, labelMap, description)

    // Limit results
    if len(ranked) > maxResults {
        ranked = ranked[:maxResults]
    }

    s.respondJSON(w, http.StatusOK, map[string]interface{}{
        "playbooks":     ranked,
        "total_results": len(ranked),
    })
}

func parseLabels(labelParams []string) map[string]string {
    labels := make(map[string]string)
    for _, param := range labelParams {
        parts := strings.SplitN(param, ":", 2)
        if len(parts) == 2 {
            labels[parts[0]] = parts[1]
        }
    }
    return labels
}

func (s *ContextAPIServer) enhanceWithSemanticSearch(ctx context.Context, playbooks []*Playbook, description string) ([]*Playbook, error) {
    // Generate embedding for incident description
    embedding, err := s.embedder.Generate(ctx, description)
    if err != nil {
        return playbooks, err
    }

    // Query Data Storage Service for semantic matches
    semanticMatches, err := s.dataStorageClient.SemanticSearchPlaybooks(ctx, &datastorage.SemanticSearchQuery{
        Embedding:  embedding,
        MaxResults: 20,
    })
    if err != nil {
        return playbooks, err
    }

    // Merge results (combine label matches with semantic matches)
    merged := s.mergePlaybooks(playbooks, semanticMatches)
    return merged, nil
}

func (s *ContextAPIServer) enrichPlaybooks(playbooks []*Playbook) []*EnrichedPlaybook {
    enriched := make([]*EnrichedPlaybook, len(playbooks))
    for i, pb := range playbooks {
        enriched[i] = &EnrichedPlaybook{
            Playbook:       pb,
            LLMSummary:     s.generateLLMSummary(pb),      // BR-CONTEXT-011
            KeyInsights:    s.generateKeyInsights(pb),      // BR-CONTEXT-011
            QualityScore:   s.calculateQualityScore(pb),   // BR-CONTEXT-012
            QualityFactors: s.calculateQualityFactors(pb), // BR-CONTEXT-012
        }
    }
    return enriched
}

// BR-CONTEXT-011: LLM-Friendly Summarization
func (s *ContextAPIServer) generateLLMSummary(pb *Playbook) string {
    if pb.TotalExecutions == 0 {
        return fmt.Sprintf("New playbook with no execution history. Consider for non-critical environments first.")
    }

    summary := fmt.Sprintf("This playbook has been executed %d times with %.0f%% success rate.",
        pb.TotalExecutions, pb.SuccessRate*100)

    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince < 7 {
            summary += fmt.Sprintf(" Recently successful (last success: %.0f days ago).", daysSince)
        }
    }

    return summary
}

// BR-CONTEXT-011: Key Insights Generation
func (s *ContextAPIServer) generateKeyInsights(pb *Playbook) []string {
    insights := []string{}

    if pb.TotalExecutions == 0 {
        insights = append(insights, "🆕 New playbook - no historical data available")
        insights = append(insights, "⚠️ Recommend testing in staging before production")
        return insights
    }

    // Environment-specific performance
    if pb.EnvironmentPerformance != nil {
        prodRate := pb.EnvironmentPerformance["production"]
        stagingRate := pb.EnvironmentPerformance["staging"]
        if prodRate > 0 && stagingRate > 0 && math.Abs(prodRate-stagingRate) > 0.1 {
            if prodRate < stagingRate {
                insights = append(insights, fmt.Sprintf("⚠️ Lower success rate in production (%.0f%%) vs staging (%.0f%%)",
                    prodRate*100, stagingRate*100))
            }
        }
    }

    // Recency
    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince < 7 {
            insights = append(insights, fmt.Sprintf("✅ Recently successful (last success: %.0f days ago)", daysSince))
        } else if daysSince > 30 {
            insights = append(insights, fmt.Sprintf("⚠️ Not used recently (last success: %.0f days ago)", daysSince))
        }
    }

    // Trend
    if pb.TrendDirection != "" {
        if pb.TrendDirection == "improving" {
            insights = append(insights, fmt.Sprintf("📈 Success rate improving (+%.0f%% over last 30 days)", pb.TrendPercent))
        } else if pb.TrendDirection == "declining" {
            insights = append(insights, fmt.Sprintf("📉 Success rate declining (%.0f%% over last 30 days)", pb.TrendPercent))
        }
    }

    return insights
}

// BR-CONTEXT-012: Quality Score Calculation
func (s *ContextAPIServer) calculateQualityScore(pb *Playbook) float64 {
    if pb.TotalExecutions == 0 {
        return 0.50 // Neutral for new playbooks
    }

    // Sample size score (40% weight)
    sampleScore := 0.0
    if pb.TotalExecutions >= 100 {
        sampleScore = 1.0
    } else if pb.TotalExecutions >= 50 {
        sampleScore = 0.8
    } else if pb.TotalExecutions >= 10 {
        sampleScore = 0.6
    } else {
        sampleScore = 0.3
    }

    // Recency score (30% weight)
    recencyScore := 0.5
    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince <= 7 {
            recencyScore = 1.0
        } else if daysSince <= 30 {
            recencyScore = 0.8
        } else if daysSince <= 90 {
            recencyScore = 0.6
        } else {
            recencyScore = 0.3
        }
    }

    // Consistency score (20% weight)
    consistencyScore := 1.0
    if pb.SuccessRateStdDev > 0.3 {
        consistencyScore = 0.3
    } else if pb.SuccessRateStdDev > 0.2 {
        consistencyScore = 0.6
    } else if pb.SuccessRateStdDev > 0.1 {
        consistencyScore = 0.8
    }

    // Environment match score (10% weight)
    envScore := 0.5 // Default neutral

    // Weighted average
    qualityScore := (0.4 * sampleScore) + (0.3 * recencyScore) + (0.2 * consistencyScore) + (0.1 * envScore)
    return qualityScore
}

// BR-CONTEXT-012: Quality Factors
func (s *ContextAPIServer) calculateQualityFactors(pb *Playbook) map[string]string {
    factors := make(map[string]string)

    if pb.TotalExecutions == 0 {
        factors["sample_size"] = "none"
        factors["recency"] = "n/a"
        factors["consistency"] = "n/a"
        factors["environment_match"] = "unknown"
        return factors
    }

    // Sample size
    if pb.TotalExecutions >= 100 {
        factors["sample_size"] = "excellent"
    } else if pb.TotalExecutions >= 50 {
        factors["sample_size"] = "good"
    } else if pb.TotalExecutions >= 10 {
        factors["sample_size"] = "fair"
    } else {
        factors["sample_size"] = "poor"
    }

    // Recency
    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince <= 7 {
            factors["recency"] = "excellent"
        } else if daysSince <= 30 {
            factors["recency"] = "good"
        } else if daysSince <= 90 {
            factors["recency"] = "fair"
        } else {
            factors["recency"] = "poor"
        }
    } else {
        factors["recency"] = "unknown"
    }

    // Consistency
    if pb.SuccessRateStdDev < 0.1 {
        factors["consistency"] = "excellent"
    } else if pb.SuccessRateStdDev < 0.2 {
        factors["consistency"] = "good"
    } else if pb.SuccessRateStdDev < 0.3 {
        factors["consistency"] = "fair"
    } else {
        factors["consistency"] = "poor"
    }

    factors["environment_match"] = "high" // Default

    return factors
}
```

---

## Database Schema (V1.0)

**Note**: Context API queries Data Storage Service REST API, which accesses these tables.

```sql
-- Playbooks table
CREATE TABLE remediation_playbooks (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    description TEXT NOT NULL,
    incident_types TEXT[] NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    steps JSONB NOT NULL,

    PRIMARY KEY (playbook_id, version)
);

-- Playbook labels table
CREATE TABLE playbook_labels (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    label_key VARCHAR(100) NOT NULL,
    label_value VARCHAR(200) NOT NULL,

    PRIMARY KEY (playbook_id, version, label_key),
    FOREIGN KEY (playbook_id, version) REFERENCES remediation_playbooks(playbook_id, version)
);

-- Playbook embeddings table (for semantic search)
CREATE TABLE playbook_embeddings (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    embedding vector(1536),  -- OpenAI text-embedding-3-small

    PRIMARY KEY (playbook_id, version),
    FOREIGN KEY (playbook_id, version) REFERENCES remediation_playbooks(playbook_id, version)
);

CREATE INDEX idx_playbook_embeddings_vector ON playbook_embeddings
USING ivfflat (embedding vector_cosine_ops);

-- Multi-dimensional success tracking (from ADR-033)
CREATE TABLE playbook_success_rates (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    dimension_type VARCHAR(50) NOT NULL,  -- 'environment', 'priority', 'business_category'
    dimension_value VARCHAR(100) NOT NULL,
    total_executions INT NOT NULL DEFAULT 0,
    successful_executions INT NOT NULL DEFAULT 0,
    failed_executions INT NOT NULL DEFAULT 0,
    success_rate DECIMAL(5,4) NOT NULL DEFAULT 0.0,
    last_executed_at TIMESTAMP,

    PRIMARY KEY (playbook_id, version, dimension_type, dimension_value),
    FOREIGN KEY (playbook_id, version) REFERENCES remediation_playbooks(playbook_id, version)
);

CREATE INDEX idx_playbook_success_rates_lookup ON playbook_success_rates(playbook_id, version, dimension_type);
```

---

## V1.1 Changes

**No changes to Context API query logic.**

V1.1 adds `PlaybookRegistration` CRD controller (separate service) that:
1. Watches `PlaybookRegistration` CRDs
2. Syncs to `remediation_playbooks` table
3. Generates embeddings on CRD creation
4. Updates database when CRD changes

**Note**: V1.1 Playbook Catalog is a **pure CRD controller** with NO REST API. Context API continues querying Data Storage Service REST API, which accesses the same database tables.

---

## Caching Strategy (BR-CONTEXT-013)

### Multi-Level Caching

**Level 1: HolmesGPT API Session Cache**
- In-memory cache within HolmesGPT API
- TTL: 5 minutes
- Scope: Per investigation session

**Level 2: Context API Redis Cache**
- Distributed cache in Redis
- TTL: 15 minutes
- Scope: All Context API instances

**Level 3: Semantic Similarity Cache**
- Cache for similar incident descriptions
- TTL: 15 minutes
- Invalidated when new playbook executions added

### Cache Invalidation

Context API listens for remediation completion events:
1. Effectiveness Monitor writes to `effectiveness_results` table
2. Data Storage Service writes to `remediation_audit` table
3. Context API subscribes to PostgreSQL NOTIFY/LISTEN for changes
4. Invalidates cache for affected playbooks

---

## Related Documents

- [BR-CONTEXT-011: LLM-Friendly Context Summarization](../../requirements/BR-CONTEXT-011-llm-friendly-summarization.md)
- [BR-CONTEXT-012: Context Quality Scoring](../../requirements/BR-CONTEXT-012-quality-scoring.md)
- [BR-CONTEXT-013: Context Caching Strategy](../../requirements/BR-CONTEXT-013-caching-strategy.md)
- [BR-PLAYBOOK-001: Playbook Registry Management](../../requirements/BR-PLAYBOOK-001-playbook-registry-management.md)
- [ADR-033: Remediation Playbook Catalog](../../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-035: Remediation Execution Engine](../../architecture/decisions/ADR-035-remediation-execution-engine.md)
- [HolmesGPT API Overview](../holmesgpt-api/overview.md)
- [Data Storage Service API](../data-storage/api-specification.md)

---

## Architecture Evolution

### V1.0 (Current)
```
HolmesGPT API
    ↓ (tool call)
Context API
    ↓ (REST API)
Data Storage Service
    ↓ (SQL)
PostgreSQL (remediation_playbooks, playbook_labels, playbook_embeddings)
```

### V1.1 (Next Release)
```
kubectl apply -f playbook.yaml
    ↓
PlaybookRegistration CRD
    ↓
Playbook Catalog CRD Controller (watches CRDs)
    ↓ (syncs to database)
PostgreSQL (remediation_playbooks, playbook_labels, playbook_embeddings)
    ↑ (queries via REST API)
Data Storage Service
    ↑ (REST API)
Context API
    ↑ (tool call)
HolmesGPT API
```

**Key Points**:
- V1.0: Playbooks managed via SQL
- V1.1: Playbooks managed via CRDs (Playbook Catalog CRD controller syncs to database)
- Context API always queries Data Storage Service REST API (not database directly)
- No REST API for Playbook Catalog Service in V1.1 (pure CRD controller)

---

**Document Version**: 1.1
**Last Updated**: November 11, 2025
**Status**: ✅ **READY FOR IMPLEMENTATION**

**Last Updated**: November 11, 2025
**Status**: Design
**Target Version**: V1.0

---

## Purpose

Context API provides a playbook query endpoint for HolmesGPT API to retrieve relevant remediation playbooks during AI investigation. The LLM uses this list to recommend playbooks based on the incident context.

---

## Endpoint Specification

```http
GET /api/v1/playbooks
```

### Query Parameters

| Parameter | Type | Required | Description | Example |
|-----------|------|----------|-------------|---------|
| `incident_type` | string | Yes | Incident type from alert | `pod-oom-killer` |
| `description` | string | No | Incident description for semantic search | `High memory usage causing pod restarts` |
| `labels` | string | No | Label filters (repeatable) | `labels=kubernaut.io/environment:prod-us-east&labels=kubernaut.io/priority:P0` |
| `max_results` | int | No | Maximum playbooks to return (default: 10) | `10` |

**Notes**:
- `incident_type` is mandatory - primary filter
- `description` enables semantic search if provided
- `labels` filter by playbook labels (key:value pairs from Gateway and Signal Processing categorization)
- Always returns `status=active` playbooks only
- Always includes new playbooks (0 executions) with exploration bonus

---

## Response Schema

```json
{
  "playbooks": [
    {
      "playbook_id": "pod-oom-recovery",
      "version": "v1.2",
      "description": "Increases memory limits and restarts pod",
      "total_executions": 150,
      "success_rate": 0.95,
      "confidence": 0.92,
      "llm_summary": "This playbook has been executed 150 times with 95% success rate. Recently successful (last success: 1 day ago). Lower success rate in production (78%) vs staging (95%).",
      "key_insights": [
        "Lower success rate in production (78%) vs staging (95%)",
        "Recently successful (last success: 1 day ago)",
        "Success rate improving (+12% over last 30 days)"
      ],
      "quality_score": 0.95,
      "quality_factors": {
        "sample_size": "excellent",
        "recency": "good",
        "consistency": "excellent",
        "environment_match": "high"
      }
    },
    {
      "playbook_id": "pod-oom-recovery-ml-optimized",
      "version": "v1.0",
      "description": "ML-based memory optimization",
      "total_executions": 0,
      "success_rate": null,
      "confidence": 0.78,
      "llm_summary": "New playbook with no execution history. Consider for non-critical environments first.",
      "key_insights": [
        "🆕 New playbook - no historical data available",
        "⚠️ Recommend testing in staging before production"
      ],
      "quality_score": 0.50,
      "quality_factors": {
        "sample_size": "none",
        "recency": "n/a",
        "consistency": "n/a",
        "environment_match": "unknown"
      }
    }
  ],
  "total_results": 2
}
```

### Playbook Object Fields

| Field | Type | Description | BR Reference |
|-------|------|-------------|--------------|
| `playbook_id` | string | Unique playbook identifier | Core |
| `version` | string | Playbook version (semantic versioning) | Core |
| `description` | string | Human-readable description of what the playbook does | Core |
| `total_executions` | int | Historical execution count (0 for new playbooks) | Core |
| `success_rate` | float \| null | Historical success rate 0.0-1.0 (null for new playbooks) | Core |
| `confidence` | float | Ranking score 0.0-1.0 (semantic + labels + performance + exploration bonus) | Core |
| `llm_summary` | string | Natural language summary for LLM consumption | BR-CONTEXT-011 |
| `key_insights` | string[] | 3-5 key insights with emoji indicators | BR-CONTEXT-011 |
| `quality_score` | float | Data quality score 0.0-1.0 | BR-CONTEXT-012 |
| `quality_factors` | object | Quality score breakdown | BR-CONTEXT-012 |

**Rationale**: Minimal core fields + LLM-friendly enhancements. LLM sees description, historical performance, composite ranking, and actionable insights.

---

## Composite Score Calculation

```
confidence = (
    0.3 * semantic_similarity +      // How well description matches incident
    0.4 * label_match_score +         // How well labels match filters
    0.3 * performance_score           // Historical success rate (0.5 default for new)
) + exploration_bonus                 // +0.05 for new playbooks
```

**Performance Score**:
- New playbooks (0 executions): `0.5` (neutral)
- Existing playbooks: `success_rate` (0.0-1.0)

**Exploration Bonus**:
- New playbooks: `+0.05` to encourage trying new solutions
- Existing playbooks: `0.0`

**Label Match Score**:
- All labels match: `1.0`
- Partial match: `matched_labels / total_labels`
- No labels: `0.5` (neutral)

---

## Implementation

### V1.0: Query Data Storage Service

```go
func (s *ContextAPIServer) handleGetPlaybooksForHolmesGPT(w http.ResponseWriter, r *http.Request) {
    incidentType := r.URL.Query().Get("incident_type")
    if incidentType == "" {
        s.respondError(w, http.StatusBadRequest, "incident_type is required")
        return
    }

    description := r.URL.Query().Get("description")
    labels := r.URL.Query()["labels"] // Repeatable parameter
    maxResults := parseIntOrDefault(r.URL.Query().Get("max_results"), 10)

    // Parse labels into map
    labelMap := parseLabels(labels)

    // Query Data Storage Service REST API
    playbooks, err := s.dataStorageClient.QueryPlaybooks(r.Context(), &datastorage.PlaybookQuery{
        IncidentType: incidentType,
        Labels:       labelMap,
        Status:       "active", // Only active playbooks
    })
    if err != nil {
        s.logger.Error("failed to query playbooks from Data Storage Service", zap.Error(err))
        s.respondError(w, http.StatusInternalServerError, "failed to query playbooks")
        return
    }

    // Semantic search (if description provided)
    if description != "" {
        playbooks, err = s.enhanceWithSemanticSearch(r.Context(), playbooks, description)
        if err != nil {
            s.logger.Warn("semantic search failed, continuing with label matches only", zap.Error(err))
        }
    }

    // Enrich with LLM-friendly summaries and quality scores
    enriched := s.enrichPlaybooks(playbooks)

    // Rank by composite score
    ranked := s.rankByCompositeScore(enriched, labelMap, description)

    // Limit results
    if len(ranked) > maxResults {
        ranked = ranked[:maxResults]
    }

    s.respondJSON(w, http.StatusOK, map[string]interface{}{
        "playbooks":     ranked,
        "total_results": len(ranked),
    })
}

func parseLabels(labelParams []string) map[string]string {
    labels := make(map[string]string)
    for _, param := range labelParams {
        parts := strings.SplitN(param, ":", 2)
        if len(parts) == 2 {
            labels[parts[0]] = parts[1]
        }
    }
    return labels
}

func (s *ContextAPIServer) enhanceWithSemanticSearch(ctx context.Context, playbooks []*Playbook, description string) ([]*Playbook, error) {
    // Generate embedding for incident description
    embedding, err := s.embedder.Generate(ctx, description)
    if err != nil {
        return playbooks, err
    }

    // Query Data Storage Service for semantic matches
    semanticMatches, err := s.dataStorageClient.SemanticSearchPlaybooks(ctx, &datastorage.SemanticSearchQuery{
        Embedding:  embedding,
        MaxResults: 20,
    })
    if err != nil {
        return playbooks, err
    }

    // Merge results (combine label matches with semantic matches)
    merged := s.mergePlaybooks(playbooks, semanticMatches)
    return merged, nil
}

func (s *ContextAPIServer) enrichPlaybooks(playbooks []*Playbook) []*EnrichedPlaybook {
    enriched := make([]*EnrichedPlaybook, len(playbooks))
    for i, pb := range playbooks {
        enriched[i] = &EnrichedPlaybook{
            Playbook:       pb,
            LLMSummary:     s.generateLLMSummary(pb),      // BR-CONTEXT-011
            KeyInsights:    s.generateKeyInsights(pb),      // BR-CONTEXT-011
            QualityScore:   s.calculateQualityScore(pb),   // BR-CONTEXT-012
            QualityFactors: s.calculateQualityFactors(pb), // BR-CONTEXT-012
        }
    }
    return enriched
}

// BR-CONTEXT-011: LLM-Friendly Summarization
func (s *ContextAPIServer) generateLLMSummary(pb *Playbook) string {
    if pb.TotalExecutions == 0 {
        return fmt.Sprintf("New playbook with no execution history. Consider for non-critical environments first.")
    }

    summary := fmt.Sprintf("This playbook has been executed %d times with %.0f%% success rate.",
        pb.TotalExecutions, pb.SuccessRate*100)

    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince < 7 {
            summary += fmt.Sprintf(" Recently successful (last success: %.0f days ago).", daysSince)
        }
    }

    return summary
}

// BR-CONTEXT-011: Key Insights Generation
func (s *ContextAPIServer) generateKeyInsights(pb *Playbook) []string {
    insights := []string{}

    if pb.TotalExecutions == 0 {
        insights = append(insights, "🆕 New playbook - no historical data available")
        insights = append(insights, "⚠️ Recommend testing in staging before production")
        return insights
    }

    // Environment-specific performance
    if pb.EnvironmentPerformance != nil {
        prodRate := pb.EnvironmentPerformance["production"]
        stagingRate := pb.EnvironmentPerformance["staging"]
        if prodRate > 0 && stagingRate > 0 && math.Abs(prodRate-stagingRate) > 0.1 {
            if prodRate < stagingRate {
                insights = append(insights, fmt.Sprintf("⚠️ Lower success rate in production (%.0f%%) vs staging (%.0f%%)",
                    prodRate*100, stagingRate*100))
            }
        }
    }

    // Recency
    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince < 7 {
            insights = append(insights, fmt.Sprintf("✅ Recently successful (last success: %.0f days ago)", daysSince))
        } else if daysSince > 30 {
            insights = append(insights, fmt.Sprintf("⚠️ Not used recently (last success: %.0f days ago)", daysSince))
        }
    }

    // Trend
    if pb.TrendDirection != "" {
        if pb.TrendDirection == "improving" {
            insights = append(insights, fmt.Sprintf("📈 Success rate improving (+%.0f%% over last 30 days)", pb.TrendPercent))
        } else if pb.TrendDirection == "declining" {
            insights = append(insights, fmt.Sprintf("📉 Success rate declining (%.0f%% over last 30 days)", pb.TrendPercent))
        }
    }

    return insights
}

// BR-CONTEXT-012: Quality Score Calculation
func (s *ContextAPIServer) calculateQualityScore(pb *Playbook) float64 {
    if pb.TotalExecutions == 0 {
        return 0.50 // Neutral for new playbooks
    }

    // Sample size score (40% weight)
    sampleScore := 0.0
    if pb.TotalExecutions >= 100 {
        sampleScore = 1.0
    } else if pb.TotalExecutions >= 50 {
        sampleScore = 0.8
    } else if pb.TotalExecutions >= 10 {
        sampleScore = 0.6
    } else {
        sampleScore = 0.3
    }

    // Recency score (30% weight)
    recencyScore := 0.5
    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince <= 7 {
            recencyScore = 1.0
        } else if daysSince <= 30 {
            recencyScore = 0.8
        } else if daysSince <= 90 {
            recencyScore = 0.6
        } else {
            recencyScore = 0.3
        }
    }

    // Consistency score (20% weight)
    consistencyScore := 1.0
    if pb.SuccessRateStdDev > 0.3 {
        consistencyScore = 0.3
    } else if pb.SuccessRateStdDev > 0.2 {
        consistencyScore = 0.6
    } else if pb.SuccessRateStdDev > 0.1 {
        consistencyScore = 0.8
    }

    // Environment match score (10% weight)
    envScore := 0.5 // Default neutral

    // Weighted average
    qualityScore := (0.4 * sampleScore) + (0.3 * recencyScore) + (0.2 * consistencyScore) + (0.1 * envScore)
    return qualityScore
}

// BR-CONTEXT-012: Quality Factors
func (s *ContextAPIServer) calculateQualityFactors(pb *Playbook) map[string]string {
    factors := make(map[string]string)

    if pb.TotalExecutions == 0 {
        factors["sample_size"] = "none"
        factors["recency"] = "n/a"
        factors["consistency"] = "n/a"
        factors["environment_match"] = "unknown"
        return factors
    }

    // Sample size
    if pb.TotalExecutions >= 100 {
        factors["sample_size"] = "excellent"
    } else if pb.TotalExecutions >= 50 {
        factors["sample_size"] = "good"
    } else if pb.TotalExecutions >= 10 {
        factors["sample_size"] = "fair"
    } else {
        factors["sample_size"] = "poor"
    }

    // Recency
    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince <= 7 {
            factors["recency"] = "excellent"
        } else if daysSince <= 30 {
            factors["recency"] = "good"
        } else if daysSince <= 90 {
            factors["recency"] = "fair"
        } else {
            factors["recency"] = "poor"
        }
    } else {
        factors["recency"] = "unknown"
    }

    // Consistency
    if pb.SuccessRateStdDev < 0.1 {
        factors["consistency"] = "excellent"
    } else if pb.SuccessRateStdDev < 0.2 {
        factors["consistency"] = "good"
    } else if pb.SuccessRateStdDev < 0.3 {
        factors["consistency"] = "fair"
    } else {
        factors["consistency"] = "poor"
    }

    factors["environment_match"] = "high" // Default

    return factors
}
```

---

## Database Schema (V1.0)

**Note**: Context API queries Data Storage Service REST API, which accesses these tables.

```sql
-- Playbooks table
CREATE TABLE remediation_playbooks (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    description TEXT NOT NULL,
    incident_types TEXT[] NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    steps JSONB NOT NULL,

    PRIMARY KEY (playbook_id, version)
);

-- Playbook labels table
CREATE TABLE playbook_labels (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    label_key VARCHAR(100) NOT NULL,
    label_value VARCHAR(200) NOT NULL,

    PRIMARY KEY (playbook_id, version, label_key),
    FOREIGN KEY (playbook_id, version) REFERENCES remediation_playbooks(playbook_id, version)
);

-- Playbook embeddings table (for semantic search)
CREATE TABLE playbook_embeddings (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    embedding vector(1536),  -- OpenAI text-embedding-3-small

    PRIMARY KEY (playbook_id, version),
    FOREIGN KEY (playbook_id, version) REFERENCES remediation_playbooks(playbook_id, version)
);

CREATE INDEX idx_playbook_embeddings_vector ON playbook_embeddings
USING ivfflat (embedding vector_cosine_ops);

-- Multi-dimensional success tracking (from ADR-033)
CREATE TABLE playbook_success_rates (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    dimension_type VARCHAR(50) NOT NULL,  -- 'environment', 'priority', 'business_category'
    dimension_value VARCHAR(100) NOT NULL,
    total_executions INT NOT NULL DEFAULT 0,
    successful_executions INT NOT NULL DEFAULT 0,
    failed_executions INT NOT NULL DEFAULT 0,
    success_rate DECIMAL(5,4) NOT NULL DEFAULT 0.0,
    last_executed_at TIMESTAMP,

    PRIMARY KEY (playbook_id, version, dimension_type, dimension_value),
    FOREIGN KEY (playbook_id, version) REFERENCES remediation_playbooks(playbook_id, version)
);

CREATE INDEX idx_playbook_success_rates_lookup ON playbook_success_rates(playbook_id, version, dimension_type);
```

---

## V1.1 Changes

**No changes to Context API query logic.**

V1.1 adds `PlaybookRegistration` CRD controller (separate service) that:
1. Watches `PlaybookRegistration` CRDs
2. Syncs to `remediation_playbooks` table
3. Generates embeddings on CRD creation
4. Updates database when CRD changes

**Note**: V1.1 Playbook Catalog is a **pure CRD controller** with NO REST API. Context API continues querying Data Storage Service REST API, which accesses the same database tables.

---

## Caching Strategy (BR-CONTEXT-013)

### Multi-Level Caching

**Level 1: HolmesGPT API Session Cache**
- In-memory cache within HolmesGPT API
- TTL: 5 minutes
- Scope: Per investigation session

**Level 2: Context API Redis Cache**
- Distributed cache in Redis
- TTL: 15 minutes
- Scope: All Context API instances

**Level 3: Semantic Similarity Cache**
- Cache for similar incident descriptions
- TTL: 15 minutes
- Invalidated when new playbook executions added

### Cache Invalidation

Context API listens for remediation completion events:
1. Effectiveness Monitor writes to `effectiveness_results` table
2. Data Storage Service writes to `remediation_audit` table
3. Context API subscribes to PostgreSQL NOTIFY/LISTEN for changes
4. Invalidates cache for affected playbooks

---

## Related Documents

- [BR-CONTEXT-011: LLM-Friendly Context Summarization](../../requirements/BR-CONTEXT-011-llm-friendly-summarization.md)
- [BR-CONTEXT-012: Context Quality Scoring](../../requirements/BR-CONTEXT-012-quality-scoring.md)
- [BR-CONTEXT-013: Context Caching Strategy](../../requirements/BR-CONTEXT-013-caching-strategy.md)
- [BR-PLAYBOOK-001: Playbook Registry Management](../../requirements/BR-PLAYBOOK-001-playbook-registry-management.md)
- [ADR-033: Remediation Playbook Catalog](../../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-035: Remediation Execution Engine](../../architecture/decisions/ADR-035-remediation-execution-engine.md)
- [HolmesGPT API Overview](../holmesgpt-api/overview.md)
- [Data Storage Service API](../data-storage/api-specification.md)

---

## Architecture Evolution

### V1.0 (Current)
```
HolmesGPT API
    ↓ (tool call)
Context API
    ↓ (REST API)
Data Storage Service
    ↓ (SQL)
PostgreSQL (remediation_playbooks, playbook_labels, playbook_embeddings)
```

### V1.1 (Next Release)
```
kubectl apply -f playbook.yaml
    ↓
PlaybookRegistration CRD
    ↓
Playbook Catalog CRD Controller (watches CRDs)
    ↓ (syncs to database)
PostgreSQL (remediation_playbooks, playbook_labels, playbook_embeddings)
    ↑ (queries via REST API)
Data Storage Service
    ↑ (REST API)
Context API
    ↑ (tool call)
HolmesGPT API
```

**Key Points**:
- V1.0: Playbooks managed via SQL
- V1.1: Playbooks managed via CRDs (Playbook Catalog CRD controller syncs to database)
- Context API always queries Data Storage Service REST API (not database directly)
- No REST API for Playbook Catalog Service in V1.1 (pure CRD controller)

---

**Document Version**: 1.1
**Last Updated**: November 11, 2025
**Status**: ✅ **READY FOR IMPLEMENTATION**

**Last Updated**: November 11, 2025
**Status**: Design
**Target Version**: V1.0

---

## Purpose

Context API provides a playbook query endpoint for HolmesGPT API to retrieve relevant remediation playbooks during AI investigation. The LLM uses this list to recommend playbooks based on the incident context.

---

## Endpoint Specification

```http
GET /api/v1/playbooks
```

### Query Parameters

| Parameter | Type | Required | Description | Example |
|-----------|------|----------|-------------|---------|
| `incident_type` | string | Yes | Incident type from alert | `pod-oom-killer` |
| `description` | string | No | Incident description for semantic search | `High memory usage causing pod restarts` |
| `labels` | string | No | Label filters (repeatable) | `labels=kubernaut.io/environment:prod-us-east&labels=kubernaut.io/priority:P0` |
| `max_results` | int | No | Maximum playbooks to return (default: 10) | `10` |

**Notes**:
- `incident_type` is mandatory - primary filter
- `description` enables semantic search if provided
- `labels` filter by playbook labels (Gateway environment/priority + Signal Processing business categorization)
- Always returns `status=active` playbooks only
- Always includes new playbooks (0 executions) with exploration bonus

**Label Examples**:
- Environment (from Gateway): `kubernaut.io/environment:prod-us-east`, `kubernaut.io/environment:staging`
- Priority (from Gateway): `kubernaut.io/priority:P0`, `kubernaut.io/priority:P1`
- Business Category (from Signal Processing): `kubernaut.io/business-category:payment-service`

---

## Response Schema

```json
{
  "playbooks": [
    {
      "playbook_id": "pod-oom-recovery",
      "version": "v1.2",
      "description": "Increases memory limits and restarts pod",
      "total_executions": 150,
      "success_rate": 0.95,
      "confidence": 0.92
    },
    {
      "playbook_id": "pod-oom-recovery-ml-optimized",
      "version": "v1.0",
      "description": "ML-based memory optimization",
      "total_executions": 0,
      "success_rate": null,
      "confidence": 0.78
    }
  ],
  "total_results": 2
}
```

### Playbook Object Fields

| Field | Type | Description | BR Reference |
|-------|------|-------------|--------------|
| `playbook_id` | string | Unique playbook identifier | Core |
| `version` | string | Playbook version (semantic versioning) | Core |
| `description` | string | Human-readable description of what the playbook does | Core |
| `total_executions` | int | Historical execution count (0 for new playbooks) | Core |
| `success_rate` | float \| null | Historical success rate 0.0-1.0 (null for new playbooks) | Core |
| `confidence` | float | Ranking score 0.0-1.0 (semantic + labels + performance + exploration bonus) | Core |
| `llm_summary` | string | Natural language summary for LLM consumption | BR-CONTEXT-011 |
| `key_insights` | string[] | 3-5 key insights with emoji indicators | BR-CONTEXT-011 |
| `quality_score` | float | Data quality score 0.0-1.0 | BR-CONTEXT-012 |
| `quality_factors` | object | Quality score breakdown | BR-CONTEXT-012 |

**Rationale**: Minimal core fields + LLM-friendly enhancements. LLM sees description, historical performance, composite ranking, and actionable insights.

---

## Composite Score Calculation

```
confidence = (
    0.3 * semantic_similarity +      // How well description matches incident
    0.4 * label_match_score +         // How well labels match filters
    0.3 * performance_score           // Historical success rate (0.5 default for new)
) + exploration_bonus                 // +0.05 for new playbooks
```

**Performance Score**:
- New playbooks (0 executions): `0.5` (neutral)
- Existing playbooks: `success_rate` (0.0-1.0)

**Exploration Bonus**:
- New playbooks: `+0.05` to encourage trying new solutions
- Existing playbooks: `0.0`

**Label Match Score**:
- All labels match: `1.0`
- Partial match: `matched_labels / total_labels`
- No labels: `0.5` (neutral)

---

## Implementation

### V1.0: Query Data Storage Service

```go
func (s *ContextAPIServer) handleGetPlaybooksForHolmesGPT(w http.ResponseWriter, r *http.Request) {
    incidentType := r.URL.Query().Get("incident_type")
    if incidentType == "" {
        s.respondError(w, http.StatusBadRequest, "incident_type is required")
        return
    }

    description := r.URL.Query().Get("description")
    labels := r.URL.Query()["labels"] // Repeatable parameter
    maxResults := parseIntOrDefault(r.URL.Query().Get("max_results"), 10)

    // Parse labels into map
    labelMap := parseLabels(labels)

    // Query Data Storage Service REST API
    playbooks, err := s.dataStorageClient.QueryPlaybooks(r.Context(), &datastorage.PlaybookQuery{
        IncidentType: incidentType,
        Labels:       labelMap,
        Status:       "active", // Only active playbooks
    })
    if err != nil {
        s.logger.Error("failed to query playbooks from Data Storage Service", zap.Error(err))
        s.respondError(w, http.StatusInternalServerError, "failed to query playbooks")
        return
    }

    // Semantic search (if description provided)
    if description != "" {
        playbooks, err = s.enhanceWithSemanticSearch(r.Context(), playbooks, description)
        if err != nil {
            s.logger.Warn("semantic search failed, continuing with label matches only", zap.Error(err))
        }
    }

    // Enrich with LLM-friendly summaries and quality scores
    enriched := s.enrichPlaybooks(playbooks)

    // Rank by composite score
    ranked := s.rankByCompositeScore(enriched, labelMap, description)

    // Limit results
    if len(ranked) > maxResults {
        ranked = ranked[:maxResults]
    }

    s.respondJSON(w, http.StatusOK, map[string]interface{}{
        "playbooks":     ranked,
        "total_results": len(ranked),
    })
}

func parseLabels(labelParams []string) map[string]string {
    labels := make(map[string]string)
    for _, param := range labelParams {
        parts := strings.SplitN(param, ":", 2)
        if len(parts) == 2 {
            labels[parts[0]] = parts[1]
        }
    }
    return labels
}

func (s *ContextAPIServer) enhanceWithSemanticSearch(ctx context.Context, playbooks []*Playbook, description string) ([]*Playbook, error) {
    // Generate embedding for incident description
    embedding, err := s.embedder.Generate(ctx, description)
    if err != nil {
        return playbooks, err
    }

    // Query Data Storage Service for semantic matches
    semanticMatches, err := s.dataStorageClient.SemanticSearchPlaybooks(ctx, &datastorage.SemanticSearchQuery{
        Embedding:  embedding,
        MaxResults: 20,
    })
    if err != nil {
        return playbooks, err
    }

    // Merge results (combine label matches with semantic matches)
    merged := s.mergePlaybooks(playbooks, semanticMatches)
    return merged, nil
}

func (s *ContextAPIServer) enrichPlaybooks(playbooks []*Playbook) []*EnrichedPlaybook {
    enriched := make([]*EnrichedPlaybook, len(playbooks))
    for i, pb := range playbooks {
        enriched[i] = &EnrichedPlaybook{
            Playbook:       pb,
            LLMSummary:     s.generateLLMSummary(pb),      // BR-CONTEXT-011
            KeyInsights:    s.generateKeyInsights(pb),      // BR-CONTEXT-011
            QualityScore:   s.calculateQualityScore(pb),   // BR-CONTEXT-012
            QualityFactors: s.calculateQualityFactors(pb), // BR-CONTEXT-012
        }
    }
    return enriched
}

// BR-CONTEXT-011: LLM-Friendly Summarization
func (s *ContextAPIServer) generateLLMSummary(pb *Playbook) string {
    if pb.TotalExecutions == 0 {
        return fmt.Sprintf("New playbook with no execution history. Consider for non-critical environments first.")
    }

    summary := fmt.Sprintf("This playbook has been executed %d times with %.0f%% success rate.",
        pb.TotalExecutions, pb.SuccessRate*100)

    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince < 7 {
            summary += fmt.Sprintf(" Recently successful (last success: %.0f days ago).", daysSince)
        }
    }

    return summary
}

// BR-CONTEXT-011: Key Insights Generation
func (s *ContextAPIServer) generateKeyInsights(pb *Playbook) []string {
    insights := []string{}

    if pb.TotalExecutions == 0 {
        insights = append(insights, "🆕 New playbook - no historical data available")
        insights = append(insights, "⚠️ Recommend testing in staging before production")
        return insights
    }

    // Environment-specific performance
    if pb.EnvironmentPerformance != nil {
        prodRate := pb.EnvironmentPerformance["production"]
        stagingRate := pb.EnvironmentPerformance["staging"]
        if prodRate > 0 && stagingRate > 0 && math.Abs(prodRate-stagingRate) > 0.1 {
            if prodRate < stagingRate {
                insights = append(insights, fmt.Sprintf("⚠️ Lower success rate in production (%.0f%%) vs staging (%.0f%%)",
                    prodRate*100, stagingRate*100))
            }
        }
    }

    // Recency
    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince < 7 {
            insights = append(insights, fmt.Sprintf("✅ Recently successful (last success: %.0f days ago)", daysSince))
        } else if daysSince > 30 {
            insights = append(insights, fmt.Sprintf("⚠️ Not used recently (last success: %.0f days ago)", daysSince))
        }
    }

    // Trend
    if pb.TrendDirection != "" {
        if pb.TrendDirection == "improving" {
            insights = append(insights, fmt.Sprintf("📈 Success rate improving (+%.0f%% over last 30 days)", pb.TrendPercent))
        } else if pb.TrendDirection == "declining" {
            insights = append(insights, fmt.Sprintf("📉 Success rate declining (%.0f%% over last 30 days)", pb.TrendPercent))
        }
    }

    return insights
}

// BR-CONTEXT-012: Quality Score Calculation
func (s *ContextAPIServer) calculateQualityScore(pb *Playbook) float64 {
    if pb.TotalExecutions == 0 {
        return 0.50 // Neutral for new playbooks
    }

    // Sample size score (40% weight)
    sampleScore := 0.0
    if pb.TotalExecutions >= 100 {
        sampleScore = 1.0
    } else if pb.TotalExecutions >= 50 {
        sampleScore = 0.8
    } else if pb.TotalExecutions >= 10 {
        sampleScore = 0.6
    } else {
        sampleScore = 0.3
    }

    // Recency score (30% weight)
    recencyScore := 0.5
    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince <= 7 {
            recencyScore = 1.0
        } else if daysSince <= 30 {
            recencyScore = 0.8
        } else if daysSince <= 90 {
            recencyScore = 0.6
        } else {
            recencyScore = 0.3
        }
    }

    // Consistency score (20% weight)
    consistencyScore := 1.0
    if pb.SuccessRateStdDev > 0.3 {
        consistencyScore = 0.3
    } else if pb.SuccessRateStdDev > 0.2 {
        consistencyScore = 0.6
    } else if pb.SuccessRateStdDev > 0.1 {
        consistencyScore = 0.8
    }

    // Environment match score (10% weight)
    envScore := 0.5 // Default neutral

    // Weighted average
    qualityScore := (0.4 * sampleScore) + (0.3 * recencyScore) + (0.2 * consistencyScore) + (0.1 * envScore)
    return qualityScore
}

// BR-CONTEXT-012: Quality Factors
func (s *ContextAPIServer) calculateQualityFactors(pb *Playbook) map[string]string {
    factors := make(map[string]string)

    if pb.TotalExecutions == 0 {
        factors["sample_size"] = "none"
        factors["recency"] = "n/a"
        factors["consistency"] = "n/a"
        factors["environment_match"] = "unknown"
        return factors
    }

    // Sample size
    if pb.TotalExecutions >= 100 {
        factors["sample_size"] = "excellent"
    } else if pb.TotalExecutions >= 50 {
        factors["sample_size"] = "good"
    } else if pb.TotalExecutions >= 10 {
        factors["sample_size"] = "fair"
    } else {
        factors["sample_size"] = "poor"
    }

    // Recency
    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince <= 7 {
            factors["recency"] = "excellent"
        } else if daysSince <= 30 {
            factors["recency"] = "good"
        } else if daysSince <= 90 {
            factors["recency"] = "fair"
        } else {
            factors["recency"] = "poor"
        }
    } else {
        factors["recency"] = "unknown"
    }

    // Consistency
    if pb.SuccessRateStdDev < 0.1 {
        factors["consistency"] = "excellent"
    } else if pb.SuccessRateStdDev < 0.2 {
        factors["consistency"] = "good"
    } else if pb.SuccessRateStdDev < 0.3 {
        factors["consistency"] = "fair"
    } else {
        factors["consistency"] = "poor"
    }

    factors["environment_match"] = "high" // Default

    return factors
}
```

---

## Database Schema (V1.0)

**Note**: Context API queries Data Storage Service REST API, which accesses these tables.

```sql
-- Playbooks table
CREATE TABLE remediation_playbooks (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    description TEXT NOT NULL,
    incident_types TEXT[] NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    steps JSONB NOT NULL,

    PRIMARY KEY (playbook_id, version)
);

-- Playbook labels table
CREATE TABLE playbook_labels (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    label_key VARCHAR(100) NOT NULL,
    label_value VARCHAR(200) NOT NULL,

    PRIMARY KEY (playbook_id, version, label_key),
    FOREIGN KEY (playbook_id, version) REFERENCES remediation_playbooks(playbook_id, version)
);

-- Playbook embeddings table (for semantic search)
CREATE TABLE playbook_embeddings (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    embedding vector(1536),  -- OpenAI text-embedding-3-small

    PRIMARY KEY (playbook_id, version),
    FOREIGN KEY (playbook_id, version) REFERENCES remediation_playbooks(playbook_id, version)
);

CREATE INDEX idx_playbook_embeddings_vector ON playbook_embeddings
USING ivfflat (embedding vector_cosine_ops);

-- Multi-dimensional success tracking (from ADR-033)
CREATE TABLE playbook_success_rates (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    dimension_type VARCHAR(50) NOT NULL,  -- 'environment', 'priority', 'business_category'
    dimension_value VARCHAR(100) NOT NULL,
    total_executions INT NOT NULL DEFAULT 0,
    successful_executions INT NOT NULL DEFAULT 0,
    failed_executions INT NOT NULL DEFAULT 0,
    success_rate DECIMAL(5,4) NOT NULL DEFAULT 0.0,
    last_executed_at TIMESTAMP,

    PRIMARY KEY (playbook_id, version, dimension_type, dimension_value),
    FOREIGN KEY (playbook_id, version) REFERENCES remediation_playbooks(playbook_id, version)
);

CREATE INDEX idx_playbook_success_rates_lookup ON playbook_success_rates(playbook_id, version, dimension_type);
```

---

## V1.1 Changes

**No changes to Context API query logic.**

V1.1 adds `PlaybookRegistration` CRD controller (separate service) that:
1. Watches `PlaybookRegistration` CRDs
2. Syncs to `remediation_playbooks` table
3. Generates embeddings on CRD creation
4. Updates database when CRD changes

**Note**: V1.1 Playbook Catalog is a **pure CRD controller** with NO REST API. Context API continues querying Data Storage Service REST API, which accesses the same database tables.

---

## Caching Strategy (BR-CONTEXT-013)

### Multi-Level Caching

**Level 1: HolmesGPT API Session Cache**
- In-memory cache within HolmesGPT API
- TTL: 5 minutes
- Scope: Per investigation session

**Level 2: Context API Redis Cache**
- Distributed cache in Redis
- TTL: 15 minutes
- Scope: All Context API instances

**Level 3: Semantic Similarity Cache**
- Cache for similar incident descriptions
- TTL: 15 minutes
- Invalidated when new playbook executions added

### Cache Invalidation

Context API listens for remediation completion events:
1. Effectiveness Monitor writes to `effectiveness_results` table
2. Data Storage Service writes to `remediation_audit` table
3. Context API subscribes to PostgreSQL NOTIFY/LISTEN for changes
4. Invalidates cache for affected playbooks

---

## Related Documents

- [BR-CONTEXT-011: LLM-Friendly Context Summarization](../../requirements/BR-CONTEXT-011-llm-friendly-summarization.md)
- [BR-CONTEXT-012: Context Quality Scoring](../../requirements/BR-CONTEXT-012-quality-scoring.md)
- [BR-CONTEXT-013: Context Caching Strategy](../../requirements/BR-CONTEXT-013-caching-strategy.md)
- [BR-PLAYBOOK-001: Playbook Registry Management](../../requirements/BR-PLAYBOOK-001-playbook-registry-management.md)
- [ADR-033: Remediation Playbook Catalog](../../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-035: Remediation Execution Engine](../../architecture/decisions/ADR-035-remediation-execution-engine.md)
- [HolmesGPT API Overview](../holmesgpt-api/overview.md)
- [Data Storage Service API](../data-storage/api-specification.md)

---

## Architecture Evolution

### V1.0 (Current)
```
HolmesGPT API
    ↓ (tool call)
Context API
    ↓ (REST API)
Data Storage Service
    ↓ (SQL)
PostgreSQL (remediation_playbooks, playbook_labels, playbook_embeddings)
```

### V1.1 (Next Release)
```
kubectl apply -f playbook.yaml
    ↓
PlaybookRegistration CRD
    ↓
Playbook Catalog CRD Controller (watches CRDs)
    ↓ (syncs to database)
PostgreSQL (remediation_playbooks, playbook_labels, playbook_embeddings)
    ↑ (queries via REST API)
Data Storage Service
    ↑ (REST API)
Context API
    ↑ (tool call)
HolmesGPT API
```

**Key Points**:
- V1.0: Playbooks managed via SQL
- V1.1: Playbooks managed via CRDs (Playbook Catalog CRD controller syncs to database)
- Context API always queries Data Storage Service REST API (not database directly)
- No REST API for Playbook Catalog Service in V1.1 (pure CRD controller)

---

**Document Version**: 1.1
**Last Updated**: November 11, 2025
**Status**: ✅ **READY FOR IMPLEMENTATION**

**Last Updated**: November 11, 2025
**Status**: Design
**Target Version**: V1.0

---

## Purpose

Context API provides a playbook query endpoint for HolmesGPT API to retrieve relevant remediation playbooks during AI investigation. The LLM uses this list to recommend playbooks based on the incident context.

---

## Endpoint Specification

```http
GET /api/v1/playbooks
```

### Query Parameters

| Parameter | Type | Required | Description | Example |
|-----------|------|----------|-------------|---------|
| `incident_type` | string | Yes | Incident type from alert | `pod-oom-killer` |
| `description` | string | No | Incident description for semantic search | `High memory usage causing pod restarts` |
| `labels` | string | No | Label filters (repeatable, key:value format) | `labels=environment:production&labels=priority:P0` |
| `max_results` | int | No | Maximum playbooks to return (default: 10) | `10` |

**Notes**:
- `incident_type` is mandatory - primary filter
- `description` enables semantic search if provided
- `labels` filter by playbook labels (Gateway environment/priority + Signal Processing business categorization)
- Always returns `status=active` playbooks only
- Always includes new playbooks (0 executions) with exploration bonus

**Label Examples**:
- Environment (from Gateway): `kubernaut.io/environment:prod-us-east`, `kubernaut.io/environment:staging`
- Priority (from Gateway): `kubernaut.io/priority:P0`, `kubernaut.io/priority:P1`
- Business Category (from Signal Processing): `kubernaut.io/business-category:payment-service`

---

## Response Schema

```json
{
  "playbooks": [
    {
      "playbook_id": "pod-oom-recovery",
      "version": "v1.2",
      "description": "Increases memory limits and restarts pod",
      "total_executions": 150,
      "success_rate": 0.95,
      "confidence": 0.92,
      "llm_summary": "This playbook has been executed 150 times with 95% success rate. Recently successful (last success: 1 day ago). Lower success rate in production (78%) vs staging (95%).",
      "key_insights": [
        "Lower success rate in production (78%) vs staging (95%)",
        "Recently successful (last success: 1 day ago)",
        "Success rate improving (+12% over last 30 days)"
      ],
      "quality_score": 0.95,
      "quality_factors": {
        "sample_size": "excellent",
        "recency": "good",
        "consistency": "excellent",
        "environment_match": "high"
      }
    },
    {
      "playbook_id": "pod-oom-recovery-ml-optimized",
      "version": "v1.0",
      "description": "ML-based memory optimization",
      "total_executions": 0,
      "success_rate": null,
      "confidence": 0.78,
      "llm_summary": "New playbook with no execution history. Consider for non-critical environments first.",
      "key_insights": [
        "🆕 New playbook - no historical data available",
        "⚠️ Recommend testing in staging before production"
      ],
      "quality_score": 0.50,
      "quality_factors": {
        "sample_size": "none",
        "recency": "n/a",
        "consistency": "n/a",
        "environment_match": "unknown"
      }
    }
  ],
  "total_results": 2
}
```

### Playbook Object Fields

| Field | Type | Description | BR Reference |
|-------|------|-------------|--------------|
| `playbook_id` | string | Unique playbook identifier | Core |
| `version` | string | Playbook version (semantic versioning) | Core |
| `description` | string | Human-readable description of what the playbook does | Core |
| `total_executions` | int | Historical execution count (0 for new playbooks) | Core |
| `success_rate` | float \| null | Historical success rate 0.0-1.0 (null for new playbooks) | Core |
| `confidence` | float | Ranking score 0.0-1.0 (semantic + labels + performance + exploration bonus) | Core |
| `llm_summary` | string | Natural language summary for LLM consumption | BR-CONTEXT-011 |
| `key_insights` | string[] | 3-5 key insights with emoji indicators | BR-CONTEXT-011 |
| `quality_score` | float | Data quality score 0.0-1.0 | BR-CONTEXT-012 |
| `quality_factors` | object | Quality score breakdown | BR-CONTEXT-012 |

**Rationale**: Minimal core fields + LLM-friendly enhancements. LLM sees description, historical performance, composite ranking, and actionable insights.

---

## Composite Score Calculation

```
confidence = (
    0.3 * semantic_similarity +      // How well description matches incident
    0.4 * label_match_score +         // How well labels match filters
    0.3 * performance_score           // Historical success rate (0.5 default for new)
) + exploration_bonus                 // +0.05 for new playbooks
```

**Performance Score**:
- New playbooks (0 executions): `0.5` (neutral)
- Existing playbooks: `success_rate` (0.0-1.0)

**Exploration Bonus**:
- New playbooks: `+0.05` to encourage trying new solutions
- Existing playbooks: `0.0`

**Label Match Score**:
- All labels match: `1.0`
- Partial match: `matched_labels / total_labels`
- No labels: `0.5` (neutral)

---

## Implementation

### V1.0: Query Data Storage Service

```go
func (s *ContextAPIServer) handleGetPlaybooksForHolmesGPT(w http.ResponseWriter, r *http.Request) {
    incidentType := r.URL.Query().Get("incident_type")
    if incidentType == "" {
        s.respondError(w, http.StatusBadRequest, "incident_type is required")
        return
    }

    description := r.URL.Query().Get("description")
    labels := r.URL.Query()["labels"] // Repeatable parameter
    maxResults := parseIntOrDefault(r.URL.Query().Get("max_results"), 10)

    // Parse labels into map
    labelMap := parseLabels(labels)

    // Query Data Storage Service REST API
    playbooks, err := s.dataStorageClient.QueryPlaybooks(r.Context(), &datastorage.PlaybookQuery{
        IncidentType: incidentType,
        Labels:       labelMap,
        Status:       "active", // Only active playbooks
    })
    if err != nil {
        s.logger.Error("failed to query playbooks from Data Storage Service", zap.Error(err))
        s.respondError(w, http.StatusInternalServerError, "failed to query playbooks")
        return
    }

    // Semantic search (if description provided)
    if description != "" {
        playbooks, err = s.enhanceWithSemanticSearch(r.Context(), playbooks, description)
        if err != nil {
            s.logger.Warn("semantic search failed, continuing with label matches only", zap.Error(err))
        }
    }

    // Enrich with LLM-friendly summaries and quality scores
    enriched := s.enrichPlaybooks(playbooks)

    // Rank by composite score
    ranked := s.rankByCompositeScore(enriched, labelMap, description)

    // Limit results
    if len(ranked) > maxResults {
        ranked = ranked[:maxResults]
    }

    s.respondJSON(w, http.StatusOK, map[string]interface{}{
        "playbooks":     ranked,
        "total_results": len(ranked),
    })
}

func parseLabels(labelParams []string) map[string]string {
    labels := make(map[string]string)
    for _, param := range labelParams {
        parts := strings.SplitN(param, ":", 2)
        if len(parts) == 2 {
            labels[parts[0]] = parts[1]
        }
    }
    return labels
}

func (s *ContextAPIServer) enhanceWithSemanticSearch(ctx context.Context, playbooks []*Playbook, description string) ([]*Playbook, error) {
    // Generate embedding for incident description
    embedding, err := s.embedder.Generate(ctx, description)
    if err != nil {
        return playbooks, err
    }

    // Query Data Storage Service for semantic matches
    semanticMatches, err := s.dataStorageClient.SemanticSearchPlaybooks(ctx, &datastorage.SemanticSearchQuery{
        Embedding:  embedding,
        MaxResults: 20,
    })
    if err != nil {
        return playbooks, err
    }

    // Merge results (combine label matches with semantic matches)
    merged := s.mergePlaybooks(playbooks, semanticMatches)
    return merged, nil
}

func (s *ContextAPIServer) enrichPlaybooks(playbooks []*Playbook) []*EnrichedPlaybook {
    enriched := make([]*EnrichedPlaybook, len(playbooks))
    for i, pb := range playbooks {
        enriched[i] = &EnrichedPlaybook{
            Playbook:       pb,
            LLMSummary:     s.generateLLMSummary(pb),      // BR-CONTEXT-011
            KeyInsights:    s.generateKeyInsights(pb),      // BR-CONTEXT-011
            QualityScore:   s.calculateQualityScore(pb),   // BR-CONTEXT-012
            QualityFactors: s.calculateQualityFactors(pb), // BR-CONTEXT-012
        }
    }
    return enriched
}

// BR-CONTEXT-011: LLM-Friendly Summarization
func (s *ContextAPIServer) generateLLMSummary(pb *Playbook) string {
    if pb.TotalExecutions == 0 {
        return fmt.Sprintf("New playbook with no execution history. Consider for non-critical environments first.")
    }

    summary := fmt.Sprintf("This playbook has been executed %d times with %.0f%% success rate.",
        pb.TotalExecutions, pb.SuccessRate*100)

    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince < 7 {
            summary += fmt.Sprintf(" Recently successful (last success: %.0f days ago).", daysSince)
        }
    }

    return summary
}

// BR-CONTEXT-011: Key Insights Generation
func (s *ContextAPIServer) generateKeyInsights(pb *Playbook) []string {
    insights := []string{}

    if pb.TotalExecutions == 0 {
        insights = append(insights, "🆕 New playbook - no historical data available")
        insights = append(insights, "⚠️ Recommend testing in staging before production")
        return insights
    }

    // Environment-specific performance
    if pb.EnvironmentPerformance != nil {
        prodRate := pb.EnvironmentPerformance["production"]
        stagingRate := pb.EnvironmentPerformance["staging"]
        if prodRate > 0 && stagingRate > 0 && math.Abs(prodRate-stagingRate) > 0.1 {
            if prodRate < stagingRate {
                insights = append(insights, fmt.Sprintf("⚠️ Lower success rate in production (%.0f%%) vs staging (%.0f%%)",
                    prodRate*100, stagingRate*100))
            }
        }
    }

    // Recency
    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince < 7 {
            insights = append(insights, fmt.Sprintf("✅ Recently successful (last success: %.0f days ago)", daysSince))
        } else if daysSince > 30 {
            insights = append(insights, fmt.Sprintf("⚠️ Not used recently (last success: %.0f days ago)", daysSince))
        }
    }

    // Trend
    if pb.TrendDirection != "" {
        if pb.TrendDirection == "improving" {
            insights = append(insights, fmt.Sprintf("📈 Success rate improving (+%.0f%% over last 30 days)", pb.TrendPercent))
        } else if pb.TrendDirection == "declining" {
            insights = append(insights, fmt.Sprintf("📉 Success rate declining (%.0f%% over last 30 days)", pb.TrendPercent))
        }
    }

    return insights
}

// BR-CONTEXT-012: Quality Score Calculation
func (s *ContextAPIServer) calculateQualityScore(pb *Playbook) float64 {
    if pb.TotalExecutions == 0 {
        return 0.50 // Neutral for new playbooks
    }

    // Sample size score (40% weight)
    sampleScore := 0.0
    if pb.TotalExecutions >= 100 {
        sampleScore = 1.0
    } else if pb.TotalExecutions >= 50 {
        sampleScore = 0.8
    } else if pb.TotalExecutions >= 10 {
        sampleScore = 0.6
    } else {
        sampleScore = 0.3
    }

    // Recency score (30% weight)
    recencyScore := 0.5
    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince <= 7 {
            recencyScore = 1.0
        } else if daysSince <= 30 {
            recencyScore = 0.8
        } else if daysSince <= 90 {
            recencyScore = 0.6
        } else {
            recencyScore = 0.3
        }
    }

    // Consistency score (20% weight)
    consistencyScore := 1.0
    if pb.SuccessRateStdDev > 0.3 {
        consistencyScore = 0.3
    } else if pb.SuccessRateStdDev > 0.2 {
        consistencyScore = 0.6
    } else if pb.SuccessRateStdDev > 0.1 {
        consistencyScore = 0.8
    }

    // Environment match score (10% weight)
    envScore := 0.5 // Default neutral

    // Weighted average
    qualityScore := (0.4 * sampleScore) + (0.3 * recencyScore) + (0.2 * consistencyScore) + (0.1 * envScore)
    return qualityScore
}

// BR-CONTEXT-012: Quality Factors
func (s *ContextAPIServer) calculateQualityFactors(pb *Playbook) map[string]string {
    factors := make(map[string]string)

    if pb.TotalExecutions == 0 {
        factors["sample_size"] = "none"
        factors["recency"] = "n/a"
        factors["consistency"] = "n/a"
        factors["environment_match"] = "unknown"
        return factors
    }

    // Sample size
    if pb.TotalExecutions >= 100 {
        factors["sample_size"] = "excellent"
    } else if pb.TotalExecutions >= 50 {
        factors["sample_size"] = "good"
    } else if pb.TotalExecutions >= 10 {
        factors["sample_size"] = "fair"
    } else {
        factors["sample_size"] = "poor"
    }

    // Recency
    if pb.LastExecutedAt != nil {
        daysSince := time.Since(*pb.LastExecutedAt).Hours() / 24
        if daysSince <= 7 {
            factors["recency"] = "excellent"
        } else if daysSince <= 30 {
            factors["recency"] = "good"
        } else if daysSince <= 90 {
            factors["recency"] = "fair"
        } else {
            factors["recency"] = "poor"
        }
    } else {
        factors["recency"] = "unknown"
    }

    // Consistency
    if pb.SuccessRateStdDev < 0.1 {
        factors["consistency"] = "excellent"
    } else if pb.SuccessRateStdDev < 0.2 {
        factors["consistency"] = "good"
    } else if pb.SuccessRateStdDev < 0.3 {
        factors["consistency"] = "fair"
    } else {
        factors["consistency"] = "poor"
    }

    factors["environment_match"] = "high" // Default

    return factors
}
```

---

## Database Schema (V1.0)

**Note**: Context API queries Data Storage Service REST API, which accesses these tables.

```sql
-- Playbooks table
CREATE TABLE remediation_playbooks (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    description TEXT NOT NULL,
    incident_types TEXT[] NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    steps JSONB NOT NULL,

    PRIMARY KEY (playbook_id, version)
);

-- Playbook labels table
CREATE TABLE playbook_labels (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    label_key VARCHAR(100) NOT NULL,
    label_value VARCHAR(200) NOT NULL,

    PRIMARY KEY (playbook_id, version, label_key),
    FOREIGN KEY (playbook_id, version) REFERENCES remediation_playbooks(playbook_id, version)
);

-- Playbook embeddings table (for semantic search)
CREATE TABLE playbook_embeddings (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    embedding vector(1536),  -- OpenAI text-embedding-3-small

    PRIMARY KEY (playbook_id, version),
    FOREIGN KEY (playbook_id, version) REFERENCES remediation_playbooks(playbook_id, version)
);

CREATE INDEX idx_playbook_embeddings_vector ON playbook_embeddings
USING ivfflat (embedding vector_cosine_ops);

-- Multi-dimensional success tracking (from ADR-033)
CREATE TABLE playbook_success_rates (
    playbook_id VARCHAR(64) NOT NULL,
    version VARCHAR(20) NOT NULL,
    dimension_type VARCHAR(50) NOT NULL,  -- 'environment', 'priority', 'business_category'
    dimension_value VARCHAR(100) NOT NULL,
    total_executions INT NOT NULL DEFAULT 0,
    successful_executions INT NOT NULL DEFAULT 0,
    failed_executions INT NOT NULL DEFAULT 0,
    success_rate DECIMAL(5,4) NOT NULL DEFAULT 0.0,
    last_executed_at TIMESTAMP,

    PRIMARY KEY (playbook_id, version, dimension_type, dimension_value),
    FOREIGN KEY (playbook_id, version) REFERENCES remediation_playbooks(playbook_id, version)
);

CREATE INDEX idx_playbook_success_rates_lookup ON playbook_success_rates(playbook_id, version, dimension_type);
```

---

## V1.1 Changes

**No changes to Context API query logic.**

V1.1 adds `PlaybookRegistration` CRD controller (separate service) that:
1. Watches `PlaybookRegistration` CRDs
2. Syncs to `remediation_playbooks` table
3. Generates embeddings on CRD creation
4. Updates database when CRD changes

**Note**: V1.1 Playbook Catalog is a **pure CRD controller** with NO REST API. Context API continues querying Data Storage Service REST API, which accesses the same database tables.

---

## Caching Strategy (BR-CONTEXT-013)

### Multi-Level Caching

**Level 1: HolmesGPT API Session Cache**
- In-memory cache within HolmesGPT API
- TTL: 5 minutes
- Scope: Per investigation session

**Level 2: Context API Redis Cache**
- Distributed cache in Redis
- TTL: 15 minutes
- Scope: All Context API instances

**Level 3: Semantic Similarity Cache**
- Cache for similar incident descriptions
- TTL: 15 minutes
- Invalidated when new playbook executions added

### Cache Invalidation

Context API listens for remediation completion events:
1. Effectiveness Monitor writes to `effectiveness_results` table
2. Data Storage Service writes to `remediation_audit` table
3. Context API subscribes to PostgreSQL NOTIFY/LISTEN for changes
4. Invalidates cache for affected playbooks

---

## Related Documents

- [BR-CONTEXT-011: LLM-Friendly Context Summarization](../../requirements/BR-CONTEXT-011-llm-friendly-summarization.md)
- [BR-CONTEXT-012: Context Quality Scoring](../../requirements/BR-CONTEXT-012-quality-scoring.md)
- [BR-CONTEXT-013: Context Caching Strategy](../../requirements/BR-CONTEXT-013-caching-strategy.md)
- [BR-PLAYBOOK-001: Playbook Registry Management](../../requirements/BR-PLAYBOOK-001-playbook-registry-management.md)
- [ADR-033: Remediation Playbook Catalog](../../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-035: Remediation Execution Engine](../../architecture/decisions/ADR-035-remediation-execution-engine.md)
- [HolmesGPT API Overview](../holmesgpt-api/overview.md)
- [Data Storage Service API](../data-storage/api-specification.md)

---

## Architecture Evolution

### V1.0 (Current)
```
HolmesGPT API
    ↓ (tool call)
Context API
    ↓ (REST API)
Data Storage Service
    ↓ (SQL)
PostgreSQL (remediation_playbooks, playbook_labels, playbook_embeddings)
```

### V1.1 (Next Release)
```
kubectl apply -f playbook.yaml
    ↓
PlaybookRegistration CRD
    ↓
Playbook Catalog CRD Controller (watches CRDs)
    ↓ (syncs to database)
PostgreSQL (remediation_playbooks, playbook_labels, playbook_embeddings)
    ↑ (queries via REST API)
Data Storage Service
    ↑ (REST API)
Context API
    ↑ (tool call)
HolmesGPT API
```

**Key Points**:
- V1.0: Playbooks managed via SQL
- V1.1: Playbooks managed via CRDs (Playbook Catalog CRD controller syncs to database)
- Context API always queries Data Storage Service REST API (not database directly)
- No REST API for Playbook Catalog Service in V1.1 (pure CRD controller)

---

**Document Version**: 1.1
**Last Updated**: November 11, 2025
**Status**: ✅ **READY FOR IMPLEMENTATION**
