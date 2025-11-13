# Data Storage Service - Audit Persistence & Semantic Search Implementation Plan

**Version**: v1.0
**Date**: November 13, 2025
**Branch**: `feature/datastorage-audit-semantic-search`
**Status**: 📋 **PLANNING PHASE**
**Estimated Effort**: 3-4 days (24-32 hours)

---

## 📋 **Executive Summary**

This plan addresses the two remaining features for Data Storage Service V1.0:

1. **Real Embedding Generation** (BR-STORAGE-012) - Replace mock embeddings with sentence-transformers model
2. **Semantic Search Integration** (BR-STORAGE-013) - Integrate pgvector for vector similarity search

**Current State**:
- ✅ Notification audit persistence fully implemented
- ✅ Dual-write coordinator architecture complete
- ✅ pgvector schema and indexes ready
- ⏸️ Embedding generation using mock data (TODO on line 227, 333)
- ⏸️ Semantic search not integrated with real Vector DB
- ⏸️ Aggregation methods have TODO comments (line 30)

**Target State**:
- ✅ Real embedding generation using sentence-transformers/all-MiniLM-L6-v2
- ✅ Semantic search fully integrated with pgvector
- ✅ Dual-write coordinator using real Vector DB client
- ✅ Integration tests for embedding pipeline and semantic search
- ✅ Aggregation methods using real PostgreSQL queries (already implemented, just remove TODO)

---

## 🎯 **Business Requirements**

### **Primary Requirements**

#### **BR-STORAGE-012: Embedding Generation**
- **Priority**: P0
- **Description**: Generate 384-dimensional embeddings from audit text for semantic search
- **Model**: sentence-transformers/all-MiniLM-L6-v2
- **Current Status**: Mock implementation (generateMockEmbedding)
- **Target**: Real embedding generation with caching

#### **BR-STORAGE-013: Query Performance Metrics**
- **Priority**: P1
- **Description**: Track query performance metrics for semantic search and vector operations
- **Current Status**: Metrics structure exists, not tracking real queries
- **Target**: Full metrics integration with pgvector queries

#### **BR-STORAGE-014: Atomic Dual-Write Operations**
- **Priority**: P0
- **Description**: Perform atomic writes to both PostgreSQL and Redis Vector DB
- **Current Status**: Architecture complete, using mock Vector DB
- **Target**: Real Vector DB client (pgvector via PostgreSQL)

#### **BR-STORAGE-015: Graceful Degradation**
- **Priority**: P0
- **Description**: Continue operation with PostgreSQL-only writes if Vector DB is unavailable
- **Current Status**: Logic implemented, needs real Vector DB integration
- **Target**: Tested graceful degradation with real Vector DB

---

## 📊 **Current Implementation Analysis**

### **What's Already Implemented** ✅

1. **Dual-Write Architecture** (`pkg/datastorage/dualwrite/`)
   - ✅ Coordinator pattern
   - ✅ Error handling and typed errors
   - ✅ Context propagation
   - ✅ Graceful degradation logic
   - ✅ Comprehensive unit tests (100% coverage)

2. **Embedding Pipeline Structure** (`pkg/datastorage/embedding/`)
   - ✅ Pipeline interface
   - ✅ Caching layer (Redis)
   - ✅ Error handling
   - ⏸️ Mock embedding API (needs real implementation)

3. **Query Service** (`pkg/datastorage/query/`)
   - ✅ Query builder
   - ✅ Semantic search method signature
   - ⏸️ Mock embedding generation (line 227, 333)
   - ⏸️ Not integrated with real Vector DB

4. **Database Schema**
   - ✅ pgvector extension enabled
   - ✅ Vector column in resource_action_traces table
   - ✅ HNSW index for vector similarity search
   - ✅ Query planner hints for index usage

5. **Aggregation Methods** (`pkg/datastorage/adapter/aggregations.go`)
   - ✅ Real PostgreSQL queries already implemented
   - ⚠️ TODO comment on line 30 is misleading (REFACTOR phase already done)
   - ✅ All tests passing

### **What Needs Implementation** ⏸️

1. **Real Embedding Generation**
   - Location: `pkg/datastorage/embedding/pipeline.go`
   - Replace: `mockEmbeddingAPI` with real sentence-transformers client
   - Integration: HTTP API to Python embedding service OR Go binding

2. **Vector DB Client**
   - Location: `pkg/datastorage/dualwrite/coordinator.go`
   - Replace: `mockVectorDB` with pgvector client
   - Integration: Use PostgreSQL connection with vector operations

3. **Semantic Search Integration**
   - Location: `pkg/datastorage/query/service.go`
   - Replace: Mock embedding generation (lines 227-229)
   - Integration: Call real embedding pipeline + pgvector query

4. **Integration Tests**
   - Location: `test/integration/datastorage/`
   - Add: Embedding pipeline integration tests
   - Add: Semantic search end-to-end tests
   - Add: Dual-write with real Vector DB tests

---

## 🏗️ **Implementation Strategy**

### **Phase 1: Embedding Service Integration** (Day 1, 8 hours)

**Goal**: Replace mock embedding generation with real sentence-transformers model

#### **Option A: Python Embedding Service** (Recommended)
**Pros**:
- ✅ Native sentence-transformers support
- ✅ Well-tested ecosystem
- ✅ Easy model loading and caching
- ✅ Separate service = independent scaling

**Cons**:
- ❌ Additional service to deploy
- ❌ Network latency for embedding generation
- ❌ Need to manage Python dependencies

**Implementation**:
```go
// pkg/datastorage/embedding/client.go
type EmbeddingClient struct {
    httpClient *http.Client
    baseURL    string
    logger     *zap.Logger
}

func (c *EmbeddingClient) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
    req := &EmbeddingRequest{Text: text}
    resp, err := c.httpClient.Post(c.baseURL+"/embed", "application/json", req)
    // ... handle response
}
```

**Python Service** (separate repository/deployment):
```python
from sentence_transformers import SentenceTransformer
from flask import Flask, request, jsonify

app = Flask(__name__)
model = SentenceTransformer('sentence-transformers/all-MiniLM-L6-v2')

@app.route('/embed', methods=['POST'])
def embed():
    text = request.json['text']
    embedding = model.encode(text).tolist()
    return jsonify({'embedding': embedding})
```

#### **Option B: Go Bindings to Python** (Alternative)
**Pros**:
- ✅ No separate service
- ✅ Lower latency

**Cons**:
- ❌ Complex CGo integration
- ❌ Deployment complexity (Python runtime in Go container)
- ❌ Harder to test and maintain

**Decision**: **Option A** (Python Embedding Service)
- Cleaner separation of concerns
- Easier to scale independently
- Standard microservices pattern

---

### **Phase 2: Vector DB Integration** (Day 2, 8 hours)

**Goal**: Integrate pgvector for vector storage and similarity search

#### **Implementation**

**Vector DB Client**:
```go
// pkg/datastorage/vectordb/pgvector_client.go
type PgVectorClient struct {
    db     *sql.DB
    logger *zap.Logger
}

func (c *PgVectorClient) StoreVector(ctx context.Context, id string, embedding []float32) error {
    // Update resource_action_traces with vector embedding
    query := `
        UPDATE resource_action_traces
        SET embedding = $1
        WHERE action_id = $2
    `
    _, err := c.db.ExecContext(ctx, query, pgvector.NewVector(embedding), id)
    return err
}

func (c *PgVectorClient) SimilaritySearch(ctx context.Context, queryEmbedding []float32, limit int) ([]string, error) {
    // Use pgvector <=> operator for cosine similarity
    query := `
        SELECT action_id, 1 - (embedding <=> $1) AS similarity
        FROM resource_action_traces
        WHERE embedding IS NOT NULL
        ORDER BY embedding <=> $1
        LIMIT $2
    `
    rows, err := c.db.QueryContext(ctx, query, pgvector.NewVector(queryEmbedding), limit)
    // ... parse results
}
```

**Integration with Dual-Write Coordinator**:
```go
// pkg/datastorage/dualwrite/coordinator.go
func (c *Coordinator) Write(ctx context.Context, audit *models.NotificationAudit, embedding []float32) error {
    // 1. Write to PostgreSQL (primary)
    if err := c.db.Write(ctx, audit); err != nil {
        return err
    }

    // 2. Write vector to pgvector (secondary)
    if err := c.vectorDB.StoreVector(ctx, audit.NotificationID, embedding); err != nil {
        c.logger.Warn("Vector DB write failed, continuing with PostgreSQL-only",
            zap.Error(err))
        c.metrics.DegradedMode.Inc()
        // Graceful degradation: continue without vector
    }

    return nil
}
```

---

### **Phase 3: Semantic Search Implementation** (Day 3, 8 hours)

**Goal**: Implement end-to-end semantic search with real embeddings and pgvector

**⚠️ CRITICAL ALIGNMENT**: This implementation must align with **DD-CONTEXT-005: Minimal LLM Response Schema**

#### **Architecture Alignment**

**DD-CONTEXT-005 Pattern**: Filter Before LLM
- **Semantic Search**: Find similar incidents by embedding similarity
- **Label Matching**: Filter by environment, priority, risk_tolerance, business_category
- **Response**: Minimal schema (playbook_id, version, description, confidence)

**Data Storage Role**: Provide semantic search capability for Context API
- Data Storage stores embeddings for notification/action audits
- Context API queries Data Storage for similar incidents
- Context API uses semantic similarity + label matching to build confidence score

#### **Implementation**

**Update Query Service**:
```go
// pkg/datastorage/query/service.go

// SemanticSearchParams defines query parameters for semantic search
// Aligns with DD-CONTEXT-005: Filter Before LLM pattern
type SemanticSearchParams struct {
    QueryText      string            // Text to search for (generates embedding)
    Labels         map[string]string // Label filters (environment, priority, etc.)
    MinConfidence  float64           // Minimum similarity threshold (0.0-1.0)
    MaxResults     int               // Limit number of results
}

// SemanticSearchResult represents a search result with confidence score
// Confidence = semantic similarity (cosine distance from pgvector)
type SemanticSearchResult struct {
    ActionID    string                 // Unique action identifier
    Audit       *models.NotificationAudit // Full audit record
    Confidence  float64                // Semantic similarity score (0.0-1.0)
    Labels      map[string]string      // Labels for filtering
}

func (s *QueryService) SemanticSearch(ctx context.Context, params SemanticSearchParams) ([]*SemanticSearchResult, error) {
    // Validate parameters
    if params.QueryText == "" {
        return nil, fmt.Errorf("query text cannot be empty")
    }
    if params.MaxResults <= 0 {
        params.MaxResults = 10 // Default
    }
    if params.MinConfidence < 0 || params.MinConfidence > 1 {
        params.MinConfidence = 0.7 // Default threshold
    }

    // 1. Generate embedding for query text
    queryEmbedding, err := s.embeddingPipeline.Generate(ctx, params.QueryText)
    if err != nil {
        s.metrics.EmbeddingErrors.Inc()
        return nil, fmt.Errorf("failed to generate embedding: %w", err)
    }

    // 2. Perform vector similarity search with label filtering
    // DD-CONTEXT-005: All filtering happens at query time, not in LLM
    query := `
        SELECT 
            action_id,
            1 - (embedding <=> $1) AS similarity,
            -- Fetch audit data for response
            notification_id,
            message_summary,
            status,
            sent_at,
            -- Fetch labels for filtering
            labels
        FROM resource_action_traces
        WHERE embedding IS NOT NULL
          AND 1 - (embedding <=> $1) >= $2  -- Min confidence threshold
    `

    // Add label filters (DD-CONTEXT-005: Filter Before LLM)
    labelFilters := []string{}
    args := []interface{}{pgvector.NewVector(queryEmbedding), params.MinConfidence}
    argIndex := 3
    
    for key, value := range params.Labels {
        labelFilters = append(labelFilters, fmt.Sprintf("labels->>'%s' = $%d", key, argIndex))
        args = append(args, value)
        argIndex++
    }
    
    if len(labelFilters) > 0 {
        query += " AND " + strings.Join(labelFilters, " AND ")
    }

    // Order by similarity (highest first) and limit results
    query += fmt.Sprintf(`
        ORDER BY embedding <=> $1
        LIMIT $%d
    `, argIndex)
    args = append(args, params.MaxResults)

    // 3. Execute query
    rows, err := s.db.QueryContext(ctx, query, args...)
    if err != nil {
        s.metrics.QueryErrors.Inc()
        return nil, fmt.Errorf("failed to perform similarity search: %w", err)
    }
    defer rows.Close()

    // 4. Parse results
    results := make([]*SemanticSearchResult, 0, params.MaxResults)
    for rows.Next() {
        var (
            actionID       string
            similarity     float64
            notificationID string
            messageSummary string
            status         string
            sentAt         time.Time
            labelsJSON     []byte
        )

        err := rows.Scan(&actionID, &similarity, &notificationID, &messageSummary, 
                        &status, &sentAt, &labelsJSON)
        if err != nil {
            s.logger.Warn("Failed to scan search result", zap.Error(err))
            continue
        }

        // Parse labels
        labels := make(map[string]string)
        if len(labelsJSON) > 0 {
            json.Unmarshal(labelsJSON, &labels)
        }

        // Build result
        result := &SemanticSearchResult{
            ActionID: actionID,
            Audit: &models.NotificationAudit{
                NotificationID: notificationID,
                MessageSummary: messageSummary,
                Status:         status,
                SentAt:         sentAt,
            },
            Confidence: similarity,
            Labels:     labels,
        }
        results = append(results, result)
    }

    // Track metrics
    s.metrics.SemanticSearchTotal.Inc()
    s.metrics.SemanticSearchResults.Observe(float64(len(results)))

    s.logger.Info("Semantic search completed",
        zap.Int("results", len(results)),
        zap.Float64("min_confidence", params.MinConfidence),
        zap.Int("label_filters", len(params.Labels)))

    return results, nil
}
```

**Remove Mock Implementations**:
- Delete `generateMockEmbedding()` function (line 333)
- Remove TODO comments (lines 227, 333)
- Update tests to use real embedding pipeline

**Add HTTP API Endpoint** (for Context API integration):
```go
// pkg/datastorage/server/handlers.go

// HandleSemanticSearch handles semantic search queries from Context API
// DD-CONTEXT-005: Provides semantic similarity for playbook filtering
func (s *Server) HandleSemanticSearch(w http.ResponseWriter, r *http.Request) {
    // Parse query parameters
    queryText := r.URL.Query().Get("query")
    if queryText == "" {
        http.Error(w, "query parameter required", http.StatusBadRequest)
        return
    }

    // Parse label filters (DD-CONTEXT-005: Filter Before LLM)
    labels := make(map[string]string)
    for key, values := range r.URL.Query() {
        if strings.HasPrefix(key, "label.") {
            labelKey := strings.TrimPrefix(key, "label.")
            if len(values) > 0 {
                labels[labelKey] = values[0]
            }
        }
    }

    // Parse optional parameters
    minConfidence := 0.7
    if conf := r.URL.Query().Get("min_confidence"); conf != "" {
        if parsed, err := strconv.ParseFloat(conf, 64); err == nil {
            minConfidence = parsed
        }
    }

    maxResults := 10
    if max := r.URL.Query().Get("max_results"); max != "" {
        if parsed, err := strconv.Atoi(max); err == nil {
            maxResults = parsed
        }
    }

    // Execute semantic search
    params := query.SemanticSearchParams{
        QueryText:     queryText,
        Labels:        labels,
        MinConfidence: minConfidence,
        MaxResults:    maxResults,
    }

    results, err := s.queryService.SemanticSearch(r.Context(), params)
    if err != nil {
        s.logger.Error("Semantic search failed", zap.Error(err))
        http.Error(w, "search failed", http.StatusInternalServerError)
        return
    }

    // Return results (Context API will use these to build playbook response)
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "results":       results,
        "total_results": len(results),
        "query":         queryText,
        "filters":       labels,
    })
}
```

---

### **Phase 4: Integration Testing** (Day 4, 8 hours)

**Goal**: Comprehensive integration tests for embedding and semantic search

#### **Test Coverage**

**Embedding Pipeline Integration Tests**:
```go
// test/integration/datastorage/embedding_pipeline_test.go
var _ = Describe("Embedding Pipeline Integration", func() {
    It("should generate real embeddings from text", func() {
        text := "Kubernetes pod crash loop detected"
        embedding, err := embeddingPipeline.Generate(ctx, text)
        Expect(err).ToNot(HaveOccurred())
        Expect(embedding).To(HaveLen(384))
        Expect(embedding[0]).ToNot(BeZero())
    })

    It("should cache embeddings in Redis", func() {
        text := "Memory limit exceeded"
        // First call - cache miss
        _, err := embeddingPipeline.Generate(ctx, text)
        Expect(err).ToNot(HaveOccurred())

        // Second call - cache hit
        start := time.Now()
        _, err = embeddingPipeline.Generate(ctx, text)
        duration := time.Since(start)
        Expect(err).ToNot(HaveOccurred())
        Expect(duration).To(BeNumerically("<", 10*time.Millisecond))
    })
})
```

**Semantic Search Integration Tests**:
```go
// test/integration/datastorage/semantic_search_test.go
var _ = Describe("Semantic Search Integration", func() {
    BeforeEach(func() {
        // Insert test audits with embeddings
        audits := []string{
            "Kubernetes pod crash loop detected in production",
            "Memory limit exceeded for container",
            "Disk space running low on node",
        }
        for _, text := range audits {
            embedding, _ := embeddingPipeline.Generate(ctx, text)
            audit := &models.NotificationAudit{
                NotificationID: uuid.New().String(),
                MessageSummary: text,
            }
            _ = dualwrite.Write(ctx, audit, embedding)
        }
    })

    It("should find semantically similar audits", func() {
        query := "pod keeps restarting"
        results, err := queryService.SemanticSearch(ctx, query, 3)
        Expect(err).ToNot(HaveOccurred())
        Expect(results).To(HaveLen(3))
        Expect(results[0].MessageSummary).To(ContainSubstring("crash loop"))
    })
})
```

**Dual-Write Integration Tests**:
```go
// test/integration/datastorage/dualwrite_vectordb_test.go
var _ = Describe("Dual-Write with Vector DB", func() {
    It("should write to both PostgreSQL and pgvector", func() {
        audit := &models.NotificationAudit{
            NotificationID: uuid.New().String(),
            MessageSummary: "Test notification",
        }
        embedding := []float32{0.1, 0.2, 0.3, ...} // 384 dimensions

        err := coordinator.Write(ctx, audit, embedding)
        Expect(err).ToNot(HaveOccurred())

        // Verify PostgreSQL write
        fetched, err := repository.GetByNotificationID(ctx, audit.NotificationID)
        Expect(err).ToNot(HaveOccurred())
        Expect(fetched.MessageSummary).To(Equal(audit.MessageSummary))

        // Verify pgvector write
        results, err := vectorDB.SimilaritySearch(ctx, embedding, 1)
        Expect(err).ToNot(HaveOccurred())
        Expect(results).To(ContainElement(audit.NotificationID))
    })

    It("should gracefully degrade when Vector DB is unavailable", func() {
        // Simulate Vector DB failure
        vectorDB.SimulateFailure()

        audit := &models.NotificationAudit{
            NotificationID: uuid.New().String(),
            MessageSummary: "Test notification",
        }
        embedding := []float32{0.1, 0.2, 0.3, ...}

        err := coordinator.Write(ctx, audit, embedding)
        Expect(err).ToNot(HaveOccurred()) // Should succeed with PostgreSQL only

        // Verify PostgreSQL write succeeded
        fetched, err := repository.GetByNotificationID(ctx, audit.NotificationID)
        Expect(err).ToNot(HaveOccurred())

        // Verify degraded mode metric incremented
        metric := testutil.GetMetric("datastorage_degraded_mode_total")
        Expect(metric).To(BeNumerically(">", 0))
    })
})
```

---

## 🎯 **Alignment with DD-CONTEXT-005**

### **Critical Architectural Alignment**

This implementation **MUST** align with **DD-CONTEXT-005: Minimal LLM Response Schema** and the "Filter Before LLM" pattern.

#### **Data Storage's Role in the Pattern**

```
┌─────────────────────────────────────────────────────────────┐
│ Gateway/Signal Processing                                    │
│ - Categorizes signal (environment, priority, risk)          │
│ - Extracts incident description                             │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       v
┌─────────────────────────────────────────────────────────────┐
│ Context API (HolmesGPT API consumer)                        │
│ - Receives categorized signal                               │
│ - Queries Data Storage for similar incidents                │
│ - Applies label filters (environment, priority, risk)       │
│ - Builds playbook response with confidence scores           │
└──────────────────────┬──────────────────────────────────────┘
                       │ HTTP Query
                       v
┌─────────────────────────────────────────────────────────────┐
│ Data Storage Service (THIS IMPLEMENTATION)                  │
│ - Generates embedding for incident description              │
│ - Performs semantic search (pgvector)                       │
│ - Filters by labels (environment, priority, etc.)           │
│ - Returns results sorted by confidence (similarity)         │
└─────────────────────────────────────────────────────────────┘
```

#### **Key Principles from DD-CONTEXT-005**

1. **Filter Before LLM** ✅
   - Data Storage performs **deterministic filtering** (label matching)
   - LLM only receives **pre-filtered** results
   - No filtering decisions delegated to LLM

2. **Confidence = Semantic Similarity** ✅
   - Data Storage calculates confidence as **cosine similarity** (1 - distance)
   - Context API uses this confidence directly in playbook response
   - LLM picks highest confidence playbook

3. **Label-Based Filtering** ✅
   - Data Storage supports label filters: `label.environment=production`
   - Filters applied at query time (SQL WHERE clause)
   - Only matching records returned

4. **Minimal Response** ✅
   - Data Storage returns: action_id, audit data, confidence, labels
   - Context API transforms to: playbook_id, version, description, confidence
   - LLM receives only 4 fields (DD-CONTEXT-005 schema)

#### **Example Query Flow**

**1. Signal Processing categorizes incident**:
```json
{
  "incident_description": "Pod crash loop due to OOM",
  "environment": "production",
  "priority": "P0",
  "risk_tolerance": "low",
  "business_category": "payment-service"
}
```

**2. Context API queries Data Storage**:
```http
GET /api/v1/semantic-search?query=Pod+crash+loop+due+to+OOM
    &label.environment=production
    &label.priority=P0
    &label.risk_tolerance=low
    &label.business_category=payment-service
    &min_confidence=0.7
    &max_results=10
```

**3. Data Storage returns filtered results**:
```json
{
  "results": [
    {
      "action_id": "action-123",
      "audit": { "notification_id": "...", "message_summary": "..." },
      "confidence": 0.92,
      "labels": {
        "environment": "production",
        "priority": "P0",
        "risk_tolerance": "low"
      }
    }
  ],
  "total_results": 1
}
```

**4. Context API transforms to playbook response**:
```json
{
  "playbooks": [
    {
      "playbook_id": "pod-oom-recovery",
      "version": "v1.2",
      "description": "Increases memory limits and restarts pod",
      "confidence": 0.92
    }
  ],
  "total_results": 1
}
```

**5. LLM receives minimal schema** (DD-CONTEXT-005):
- Only 4 fields per playbook
- All filtering already done
- Task: Pick highest confidence

#### **Why This Alignment Matters**

**Without Alignment** ❌:
- Data Storage returns unfiltered results
- Context API must filter in-memory (inefficient)
- Risk of exposing wrong playbooks to LLM
- Confidence calculation inconsistent

**With Alignment** ✅:
- Data Storage filters at query time (SQL WHERE clause)
- Context API receives only matching results
- Confidence is semantic similarity (deterministic)
- LLM task is simple (pick highest confidence)

---

## 🔧 **Technical Decisions**

### **Decision 1: Embedding Service Architecture**

**Options**:
1. **Python Microservice** (HTTP API)
2. **Go with CGo bindings to Python**
3. **Pure Go implementation** (limited model support)

**Chosen**: **Option 1 - Python Microservice**

**Rationale**:
- ✅ Native sentence-transformers support
- ✅ Easier to scale independently
- ✅ Cleaner separation of concerns
- ✅ Standard microservices pattern
- ✅ Simpler testing and deployment

**Trade-offs**:
- ⚠️ Network latency (~10-50ms per embedding)
- ⚠️ Additional service to manage
- **Mitigation**: Redis caching reduces embedding generation frequency

---

### **Decision 2: Vector DB Strategy**

**Options**:
1. **pgvector** (PostgreSQL extension)
2. **Separate Vector DB** (Milvus, Weaviate, Pinecone)

**Chosen**: **Option 1 - pgvector**

**Rationale**:
- ✅ Already integrated (schema exists)
- ✅ No additional infrastructure
- ✅ ACID guarantees with PostgreSQL
- ✅ Simpler dual-write coordination
- ✅ Lower operational complexity

**Trade-offs**:
- ⚠️ Limited to PostgreSQL performance
- ⚠️ Less specialized than dedicated vector DBs
- **Mitigation**: HNSW index provides good performance for our scale

---

### **Decision 3: Embedding Caching Strategy**

**Strategy**: Redis cache with TTL

**Implementation**:
- **Cache Key**: `embedding:<sha256(text)>`
- **TTL**: 7 days (configurable)
- **Eviction**: LRU policy

**Rationale**:
- ✅ Reduces embedding generation latency
- ✅ Reduces load on embedding service
- ✅ Cost-effective for repeated queries

---

## 📊 **Success Metrics**

### **Performance Targets**

| Metric | Target | Current | Gap |
|---|---|---|---|
| **Embedding Generation** | <100ms (cached) | N/A (mock) | Implement caching |
| **Embedding Generation** | <500ms (uncached) | N/A (mock) | Implement real service |
| **Semantic Search** | <200ms (10 results) | N/A (mock) | Implement pgvector |
| **Dual-Write Success** | >99.9% | 100% (mock) | Test with real Vector DB |
| **Graceful Degradation** | 100% PostgreSQL writes | 100% (mock) | Verify with real Vector DB |

### **Test Coverage Targets**

| Component | Target | Current | Gap |
|---|---|---|---|
| **Embedding Pipeline** | >80% unit + integration | 80% unit, 0% integration | Add integration tests |
| **Semantic Search** | >80% unit + integration | 80% unit, 0% integration | Add integration tests |
| **Dual-Write Vector DB** | >80% unit + integration | 80% unit, 0% integration | Add integration tests |

---

## 🚀 **Deployment Plan**

### **Prerequisites**

1. **Python Embedding Service**
   - Deploy as separate microservice
   - Expose HTTP API on port 8081
   - Load sentence-transformers/all-MiniLM-L6-v2 model
   - Health check endpoint: `/health`

2. **PostgreSQL pgvector**
   - ✅ Already enabled (migration 001)
   - ✅ HNSW index created
   - Verify extension: `SELECT * FROM pg_extension WHERE extname = 'vector';`

3. **Redis Cache**
   - ✅ Already deployed for DLQ
   - Configure TTL: 7 days
   - Monitor cache hit rate

### **Configuration**

**Environment Variables**:
```bash
# Embedding Service
EMBEDDING_SERVICE_URL=http://embedding-service:8081
EMBEDDING_SERVICE_TIMEOUT=5s

# Vector DB (pgvector)
POSTGRES_VECTOR_ENABLED=true
POSTGRES_VECTOR_INDEX=hnsw

# Caching
REDIS_EMBEDDING_CACHE_TTL=168h  # 7 days
REDIS_EMBEDDING_CACHE_ENABLED=true
```

**ConfigMap** (`config/data-storage-config.yaml`):
```yaml
embedding:
  service_url: "http://embedding-service:8081"
  timeout: "5s"
  cache_ttl: "168h"
  model: "sentence-transformers/all-MiniLM-L6-v2"

vectordb:
  enabled: true
  type: "pgvector"
  index_type: "hnsw"
  similarity_metric: "cosine"
```

---

## 📋 **Implementation Checklist**

### **Phase 1: Embedding Service** ✅/⏸️

- [ ] Create Python embedding service (separate repo/deployment)
- [ ] Implement HTTP API for embedding generation
- [ ] Add health check and metrics endpoints
- [ ] Deploy embedding service to Kubernetes
- [ ] Create Go HTTP client for embedding service
- [ ] Integrate client with embedding pipeline
- [ ] Add Redis caching layer
- [ ] Unit tests for embedding client
- [ ] Integration tests for embedding pipeline

### **Phase 2: Vector DB Integration** ✅/⏸️

- [ ] Create pgvector client implementation
- [ ] Implement `StoreVector()` method
- [ ] Implement `SimilaritySearch()` method
- [ ] Integrate with dual-write coordinator
- [ ] Update graceful degradation logic
- [ ] Unit tests for pgvector client
- [ ] Integration tests for dual-write with Vector DB

### **Phase 3: Semantic Search** ✅/⏸️

- [ ] Update `SemanticSearch()` method in query service
- [ ] Remove mock embedding generation
- [ ] Integrate real embedding pipeline
- [ ] Integrate pgvector similarity search
- [ ] Add query performance metrics
- [ ] Unit tests for semantic search
- [ ] Integration tests for end-to-end semantic search

### **Phase 4: Testing & Documentation** ✅/⏸️

- [ ] Comprehensive integration tests
- [ ] Performance benchmarking
- [ ] Update OpenAPI specification
- [ ] Update BR documentation
- [ ] Update implementation plan
- [ ] Deployment guide for embedding service
- [ ] Runbook for troubleshooting

### **Phase 5: Cleanup** ✅/⏸️

- [ ] Remove TODO comments (lines 227, 333)
- [ ] Remove mock implementations
- [ ] Update aggregation.go TODO comment (line 30) - already implemented
- [ ] Final code review
- [ ] Merge to main

---

## 🎯 **Next Steps**

1. **Review and Approve Plan** (30 minutes)
   - User review of architecture decisions
   - Confirm embedding service deployment strategy
   - Approve implementation phases

2. **Start Phase 1: Embedding Service** (Day 1)
   - Create Python embedding service
   - Implement Go HTTP client
   - Add Redis caching

3. **Continue with Phases 2-5** (Days 2-4)
   - Follow implementation checklist
   - Run tests after each phase
   - Update documentation continuously

---

## 📚 **References**

- **BR-STORAGE-012**: Embedding Generation
- **BR-STORAGE-013**: Query Performance Metrics
- **BR-STORAGE-014**: Atomic Dual-Write Operations
- **BR-STORAGE-015**: Graceful Degradation
- **ADR-032**: Exclusive Database Access Layer
- **DD-010**: PostgreSQL pgx Driver Migration
- **Sentence Transformers**: https://www.sbert.net/
- **pgvector**: https://github.com/pgvector/pgvector

---

**Confidence Assessment**: 90%
- ✅ Architecture is well-defined
- ✅ Implementation path is clear
- ✅ Test strategy is comprehensive
- ⚠️ Embedding service deployment needs coordination
- ⚠️ Performance targets need validation with real data

