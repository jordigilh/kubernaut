# Context API - Database Schema

**Version**: v1.0
**Last Updated**: October 6, 2025
**Database**: PostgreSQL 15+ with pgvector extension
**Purpose**: Read-only queries for historical intelligence

---

## Table of Contents

1. [Overview](#overview)
2. [Core Tables](#core-tables)
3. [Indexes](#indexes)
4. [Partitioning Strategy](#partitioning-strategy)
5. [Common Queries](#common-queries)
6. [Performance Optimization](#performance-optimization)

---

## Overview

### Database Architecture

Context API performs **read-only operations** against two data stores:
1. **PostgreSQL**: Structured audit trail and remediation history
2. **Vector DB (pgvector)**: Embedding storage for semantic search

**Note**: All write operations are handled by Data Storage Service.

---

### Key Tables

| Table | Purpose | Partitioned | Indexes |
|-------|---------|-------------|---------|
| `remediation_requests` | Remediation request history | Yes (monthly) | 5 |
| `remediation_processing` | Processing details | Yes (monthly) | 4 |
| `ai_analysis` | AI analysis results | Yes (monthly) | 4 |
| `workflow_executions` | Workflow execution history | Yes (monthly) | 5 |
| `kubernetes_executions` | Kubernetes action audit | Yes (monthly) | 4 |
| `incident_embeddings` | Vector embeddings | No | 2 |
| `environment_stats` | Pre-aggregated statistics | No | 3 |

---

## Core Tables

### 1. remediation_requests

**Purpose**: Stores all RemediationRequest CRD lifecycle history

```sql
CREATE TABLE remediation_requests (
    id BIGSERIAL,
    name VARCHAR(255) NOT NULL,                  -- CRD name
    namespace VARCHAR(255) NOT NULL,

    -- Signal details
    signal_type VARCHAR(50) NOT NULL,            -- 'prometheus', 'kubernetes-event'
    signal_name VARCHAR(255) NOT NULL,
    signal_namespace VARCHAR(255) NOT NULL,

    -- Target details
    target_type VARCHAR(100) NOT NULL,           -- 'deployment', 'statefulset', etc.
    target_name VARCHAR(255) NOT NULL,
    target_namespace VARCHAR(255) NOT NULL,

    -- Context
    environment VARCHAR(50),                     -- 'production', 'staging', 'dev'
    priority VARCHAR(10),                        -- 'P0', 'P1', 'P2'
    fingerprint VARCHAR(255),                    -- SHA256 hash

    -- Status
    phase VARCHAR(50),                           -- 'Pending', 'Processing', 'Completed', 'Failed'
    message TEXT,
    reason VARCHAR(255),

    -- Timestamps
    created_at TIMESTAMP NOT NULL,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,

    -- Metadata
    correlation_id VARCHAR(255),
    labels JSONB,

    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Monthly partitions
CREATE TABLE remediation_requests_2025_10
    PARTITION OF remediation_requests
    FOR VALUES FROM ('2025-10-01') TO ('2025-11-01');

CREATE TABLE remediation_requests_2025_11
    PARTITION OF remediation_requests
    FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');
```

---

### 2. remediation_processing

**Purpose**: Stores remediation processing (enrichment) details

```sql
CREATE TABLE remediation_processing (
    id BIGSERIAL,
    name VARCHAR(255) NOT NULL,
    namespace VARCHAR(255) NOT NULL,

    -- Parent reference
    remediation_request_ref VARCHAR(255) NOT NULL,

    -- Enrichment data
    enriched_context JSONB,                      -- Key-value context
    context_quality NUMERIC(5,2),                -- 0.0-1.0
    missing_data_fields TEXT[],

    -- Status
    phase VARCHAR(50),
    message TEXT,

    -- Timestamps
    created_at TIMESTAMP NOT NULL,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,

    -- Metadata
    correlation_id VARCHAR(255),

    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);
```

---

### 3. ai_analysis

**Purpose**: Stores AI analysis results and recommendations

```sql
CREATE TABLE ai_analysis (
    id BIGSERIAL,
    name VARCHAR(255) NOT NULL,
    namespace VARCHAR(255) NOT NULL,

    -- Parent reference
    remediation_request_ref VARCHAR(255) NOT NULL,

    -- Analysis input
    signal_type VARCHAR(50),
    signal_context JSONB,

    -- LLM configuration
    llm_provider VARCHAR(50),                    -- 'openai', 'anthropic', 'local'
    llm_model VARCHAR(100),
    temperature NUMERIC(3,2),

    -- Analysis results
    root_cause TEXT,
    confidence NUMERIC(5,2),                     -- 0.0-1.0
    recommended_action TEXT,
    requires_approval BOOLEAN DEFAULT false,

    -- Investigation details
    investigation_id VARCHAR(255),
    tokens_used INT,
    investigation_time_seconds INT,

    -- Status
    phase VARCHAR(50),
    message TEXT,

    -- Timestamps
    created_at TIMESTAMP NOT NULL,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,

    -- Metadata
    correlation_id VARCHAR(255),

    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);
```

---

### 4. workflow_executions

**Purpose**: Stores workflow execution history and step details

```sql
CREATE TABLE workflow_executions (
    id BIGSERIAL,
    name VARCHAR(255) NOT NULL,
    namespace VARCHAR(255) NOT NULL,

    -- Parent reference
    remediation_request_ref VARCHAR(255) NOT NULL,
    ai_analysis_ref VARCHAR(255),

    -- Workflow definition
    workflow_name VARCHAR(255) NOT NULL,
    is_auto_approved BOOLEAN DEFAULT false,
    requires_approval BOOLEAN DEFAULT false,

    -- Execution progress
    current_step INT DEFAULT 0,
    step_count INT NOT NULL,
    completed_count INT DEFAULT 0,
    failed_count INT DEFAULT 0,
    skipped_count INT DEFAULT 0,

    -- Status
    phase VARCHAR(50),                           -- 'Pending', 'Executing', 'Completed', 'Failed', 'Paused'
    message TEXT,

    -- Timestamps
    created_at TIMESTAMP NOT NULL,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    paused_at TIMESTAMP,

    -- Metadata
    correlation_id VARCHAR(255),

    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);
```

---

### 5. kubernetes_executions

**Purpose**: Stores Kubernetes action execution audit trail

```sql
CREATE TABLE kubernetes_executions (
    id BIGSERIAL,
    name VARCHAR(255) NOT NULL,
    namespace VARCHAR(255) NOT NULL,

    -- Parent reference
    workflow_execution_ref VARCHAR(255) NOT NULL,
    step_name VARCHAR(255),

    -- Action details
    action_type VARCHAR(50) NOT NULL,            -- 'apply', 'patch', 'scale', 'delete'
    action_data JSONB,

    -- Target resource
    target_type VARCHAR(100) NOT NULL,
    target_name VARCHAR(255) NOT NULL,
    target_namespace VARCHAR(255) NOT NULL,

    -- Safety configuration
    enable_dry_run BOOLEAN DEFAULT true,
    enable_validation BOOLEAN DEFAULT true,
    enable_rollback BOOLEAN DEFAULT true,

    -- Execution results
    action_result VARCHAR(20),                   -- 'success', 'failure'
    resource_version VARCHAR(100),
    previous_version VARCHAR(100),

    -- Safety validation
    dry_run_result TEXT,
    validation_result TEXT,
    validation_warnings TEXT[],

    -- Rollback
    is_rolled_back BOOLEAN DEFAULT false,
    rollback_reason TEXT,

    -- Status
    phase VARCHAR(50),
    message TEXT,

    -- Timestamps
    created_at TIMESTAMP NOT NULL,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    rolled_back_at TIMESTAMP,

    -- Metadata
    correlation_id VARCHAR(255),

    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);
```

---

### 6. incident_embeddings (Vector DB)

**Purpose**: Stores vector embeddings for semantic search

**Vector Database**: PostgreSQL with **pgvector extension**

#### pgvector Extension Setup

**Installation** (Run once per database):
```sql
-- Create pgvector extension (requires superuser)
CREATE EXTENSION IF NOT EXISTS vector;

-- Verify installation
SELECT * FROM pg_extension WHERE extname = 'vector';
```

**Version Requirements**:
- PostgreSQL: 12+ (14+ recommended)
- pgvector: 0.5.0+ (0.7.0+ recommended for better HNSW performance)

---

#### Vector Column Configuration

**Dimension Selection**: 1536 dimensions (OpenAI text-embedding-ada-002 standard)

**Distance Metrics Supported**:
- `vector_cosine_ops` - **Cosine similarity** (RECOMMENDED for embeddings)
- `vector_l2_ops` - Euclidean distance (L2)
- `vector_ip_ops` - Inner product

**Why Cosine Similarity?**
- ✅ Normalized embeddings (OpenAI ada-002 outputs are normalized)
- ✅ Scale-invariant (focuses on direction, not magnitude)
- ✅ Standard for semantic search in NLP
- ✅ Better performance for high-dimensional data

---

#### Table Schema

```sql
CREATE TABLE incident_embeddings (
    id BIGSERIAL PRIMARY KEY,

    -- Incident reference
    remediation_request_id BIGINT NOT NULL,
    namespace VARCHAR(255) NOT NULL,
    target_type VARCHAR(100) NOT NULL,
    target_name VARCHAR(255) NOT NULL,

    -- Incident details
    signal_type VARCHAR(50),
    root_cause TEXT,
    remediation_action TEXT,
    phase VARCHAR(50),

    -- Vector embedding (1536 dimensions for OpenAI ada-002)
    -- pgvector type: vector(dimensions)
    embedding vector(1536) NOT NULL,

    -- Metadata
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

---

#### HNSW Index Configuration

**HNSW** (Hierarchical Navigable Small World): High-performance approximate nearest neighbor search

```sql
-- Create HNSW index with optimized parameters
CREATE INDEX idx_incident_embeddings_vector ON incident_embeddings
    USING hnsw (embedding vector_cosine_ops)
    WITH (
        m = 16,              -- Max connections per node (default: 16)
        ef_construction = 64 -- Construction-time search depth (default: 64)
    );

-- Namespace filter index (for pre-filtering)
CREATE INDEX idx_incident_embeddings_namespace ON incident_embeddings(namespace);

-- Composite index for common queries
CREATE INDEX idx_incident_embeddings_namespace_phase ON incident_embeddings(namespace, phase);
```

---

#### HNSW Parameters Explained

| Parameter | Default | Recommended | Impact |
|-----------|---------|-------------|--------|
| **m** | 16 | 16-32 | Higher = better recall, more memory |
| **ef_construction** | 64 | 64-128 | Higher = better index quality, slower build |
| **ef_search** | 40 | 40-100 | Higher = better recall, slower search (query-time) |

**Tuning Guidelines**:
- **Small dataset** (< 100K rows): `m=16, ef_construction=64` (defaults)
- **Medium dataset** (100K-1M rows): `m=16, ef_construction=128`
- **Large dataset** (> 1M rows): `m=32, ef_construction=128`

**Query-Time Parameter** (set per query):
```sql
-- Adjust ef_search for query accuracy vs speed tradeoff
SET hnsw.ef_search = 100;  -- Higher = more accurate, slower

SELECT * FROM incident_embeddings
ORDER BY embedding <-> '[0.1, 0.2, ...]'::vector
LIMIT 10;
```

---

#### Distance Operators

| Operator | Distance Metric | Use Case |
|----------|----------------|----------|
| `<->` | Euclidean (L2) | Absolute distance |
| `<#>` | Inner product | Dot product similarity |
| `<=>` | **Cosine distance** | **Semantic search (RECOMMENDED)** |

**Cosine Distance vs Similarity**:
```sql
-- Cosine distance: 0 = identical, 2 = opposite
-- Cosine similarity: 1 - cosine_distance

SELECT
    embedding <=> $1::vector AS cosine_distance,
    1 - (embedding <=> $1::vector) AS cosine_similarity
FROM incident_embeddings;
```

---

#### Embedding Model Specifications

**OpenAI text-embedding-ada-002** (V1 Default):
- **Dimensions**: 1536
- **Normalization**: Output is normalized (L2 norm = 1)
- **Distance Metric**: Cosine similarity recommended
- **Cost**: $0.0001 per 1000 tokens
- **Performance**: ~1ms latency per embedding

**Future Models** (V2 Considerations):
- **text-embedding-3-small**: 512 or 1536 dimensions
- **text-embedding-3-large**: 256, 1024, or 3072 dimensions
- **Migration**: Requires re-embedding or dual-column strategy

---

#### Storage Considerations

**Vector Storage Size**:
```
Size per embedding = dimensions × 4 bytes (float32)
1536 dimensions = 1536 × 4 = 6,144 bytes (~6 KB)
```

**Table Size Estimation**:
```
1,000 incidents:     ~6 MB (vectors) + ~2 MB (metadata) = ~8 MB
10,000 incidents:    ~60 MB (vectors) + ~20 MB (metadata) = ~80 MB
100,000 incidents:   ~600 MB (vectors) + ~200 MB (metadata) = ~800 MB
1,000,000 incidents: ~6 GB (vectors) + ~2 GB (metadata) = ~8 GB
```

**HNSW Index Size** (approximate):
```
Index size ≈ vector_size × (m + ef_construction) / 8
For 100K incidents: ~600 MB × 80 / 8 ≈ 6 GB
```

---

#### Performance Optimization

##### 1. Pre-Filtering Strategy

**Problem**: Filtering after vector search is slow
**Solution**: Use composite indexes for pre-filtering

```sql
-- Good: Pre-filter with index, then vector search
SELECT * FROM incident_embeddings
WHERE namespace = 'production'  -- Index scan
  AND embedding <=> $1::vector < 0.5
ORDER BY embedding <=> $1::vector
LIMIT 10;

-- Better: Use CTE for explicit pre-filter
WITH filtered AS (
  SELECT * FROM incident_embeddings
  WHERE namespace = 'production'
    AND phase = 'Completed'
)
SELECT * FROM filtered
ORDER BY embedding <=> $1::vector
LIMIT 10;
```

---

##### 2. Query-Time Tuning

```sql
-- Set ef_search based on accuracy requirements
SET hnsw.ef_search = 40;   -- Faster, less accurate
SET hnsw.ef_search = 100;  -- Slower, more accurate
SET hnsw.ef_search = 200;  -- Best accuracy, slowest

-- Use EXPLAIN to verify index usage
EXPLAIN (ANALYZE, BUFFERS)
SELECT * FROM incident_embeddings
ORDER BY embedding <=> $1::vector
LIMIT 10;
```

---

##### 3. Batch Operations

**Bulk Insert Optimization**:
```sql
-- Disable index during bulk insert
DROP INDEX IF EXISTS idx_incident_embeddings_vector;

-- Bulk insert embeddings
COPY incident_embeddings FROM '/path/to/embeddings.csv' CSV;

-- Rebuild index (parallelized)
CREATE INDEX CONCURRENTLY idx_incident_embeddings_vector
    ON incident_embeddings
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
```

---

#### Sample Queries

##### Semantic Search (Top 10 Similar Incidents)

```sql
-- Find similar incidents using cosine similarity
SELECT
    id,
    namespace,
    target_name,
    root_cause,
    remediation_action,
    1 - (embedding <=> $1::vector) AS similarity_score
FROM incident_embeddings
WHERE namespace = 'production'
  AND 1 - (embedding <=> $1::vector) > 0.7  -- Similarity threshold
ORDER BY embedding <=> $1::vector
LIMIT 10;
```

---

##### Semantic Search with Multiple Filters

```sql
-- Complex query with namespace, phase, and time filters
WITH recent_incidents AS (
  SELECT * FROM incident_embeddings
  WHERE namespace = 'production'
    AND phase = 'Completed'
    AND created_at > NOW() - INTERVAL '30 days'
)
SELECT
    namespace,
    target_name,
    root_cause,
    1 - (embedding <=> $1::vector) AS similarity
FROM recent_incidents
WHERE 1 - (embedding <=> $1::vector) > 0.75
ORDER BY embedding <=> $1::vector
LIMIT 10;
```

---

##### Find Duplicate Embeddings

```sql
-- Find near-duplicate incidents (very high similarity)
SELECT
    a.id AS incident1_id,
    b.id AS incident2_id,
    a.target_name AS incident1_target,
    b.target_name AS incident2_target,
    1 - (a.embedding <=> b.embedding) AS similarity
FROM incident_embeddings a
CROSS JOIN LATERAL (
    SELECT id, target_name, embedding
    FROM incident_embeddings
    WHERE id > a.id
    ORDER BY embedding <=> a.embedding
    LIMIT 5
) b
WHERE 1 - (a.embedding <=> b.embedding) > 0.95
ORDER BY similarity DESC;
```

---

#### Migration Strategy

**Adding pgvector to Existing Database**:

```sql
-- Step 1: Install extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Step 2: Add vector column to existing table
ALTER TABLE incident_embeddings
    ADD COLUMN IF NOT EXISTS embedding vector(1536);

-- Step 3: Backfill embeddings (application-side)
-- Data Storage Service generates embeddings for existing incidents

-- Step 4: Create HNSW index (after backfill)
CREATE INDEX CONCURRENTLY idx_incident_embeddings_vector
    ON incident_embeddings
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- Step 5: Add NOT NULL constraint (after validation)
ALTER TABLE incident_embeddings
    ALTER COLUMN embedding SET NOT NULL;
```

---

#### Monitoring & Maintenance

**Index Health Check**:
```sql
-- Check index size
SELECT
    pg_size_pretty(pg_relation_size('idx_incident_embeddings_vector')) AS index_size;

-- Check index usage
SELECT
    schemaname,
    tablename,
    indexname,
    idx_scan AS index_scans,
    idx_tup_read AS tuples_read,
    idx_tup_fetch AS tuples_fetched
FROM pg_stat_user_indexes
WHERE indexname = 'idx_incident_embeddings_vector';
```

**Vacuum & Analyze**:
```sql
-- Regular maintenance
VACUUM ANALYZE incident_embeddings;

-- Check bloat
SELECT
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size
FROM pg_tables
WHERE tablename = 'incident_embeddings';
```

---

#### Troubleshooting

**Common Issues**:

1. **Slow Queries**:
   ```sql
   -- Increase ef_search
   SET hnsw.ef_search = 100;

   -- Verify index usage
   EXPLAIN (ANALYZE) SELECT * FROM incident_embeddings
   ORDER BY embedding <=> $1::vector LIMIT 10;
   ```

2. **High Memory Usage**:
   ```sql
   -- Reduce HNSW parameters
   DROP INDEX idx_incident_embeddings_vector;
   CREATE INDEX idx_incident_embeddings_vector
       ON incident_embeddings
       USING hnsw (embedding vector_cosine_ops)
       WITH (m = 8, ef_construction = 32);
   ```

3. **Index Build Fails**:
   ```sql
   -- Increase maintenance_work_mem temporarily
   SET maintenance_work_mem = '2GB';
   CREATE INDEX ...;
   RESET maintenance_work_mem;
   ```

---

**Vector DB Documentation**: ✅ **COMPREHENSIVE**

---

### 7. environment_stats

**Purpose**: Pre-aggregated statistics for fast environment context queries

```sql
CREATE TABLE environment_stats (
    id BIGSERIAL PRIMARY KEY,

    -- Target identification
    namespace VARCHAR(255) NOT NULL,
    target_type VARCHAR(100) NOT NULL,
    target_name VARCHAR(255) NOT NULL,
    environment VARCHAR(50),

    -- Time period
    period_start TIMESTAMP NOT NULL,
    period_end TIMESTAMP NOT NULL,

    -- Statistics
    total_remediations INT NOT NULL DEFAULT 0,
    successful_remediations INT NOT NULL DEFAULT 0,
    failed_remediations INT NOT NULL DEFAULT 0,
    avg_remediation_duration_seconds NUMERIC(10,2),

    -- Priority distribution
    priority_p0_count INT NOT NULL DEFAULT 0,
    priority_p1_count INT NOT NULL DEFAULT 0,
    priority_p2_count INT NOT NULL DEFAULT 0,

    -- Common failures (JSON array)
    common_failure_reasons JSONB,

    -- Metadata
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (namespace, target_type, target_name, period_start)
);

-- Composite index for fast lookups
CREATE INDEX idx_environment_stats_lookup ON environment_stats
    (namespace, target_type, target_name, period_start DESC);

-- Environment filter index
CREATE INDEX idx_environment_stats_environment ON environment_stats(environment);
```

---

## Indexes

### remediation_requests Indexes

```sql
-- Primary lookup by target
CREATE INDEX idx_remediation_requests_target ON remediation_requests
    (namespace, target_type, target_name, created_at DESC);

-- Lookup by correlation ID
CREATE INDEX idx_remediation_requests_correlation ON remediation_requests(correlation_id);

-- Lookup by fingerprint (deduplication)
CREATE INDEX idx_remediation_requests_fingerprint ON remediation_requests(fingerprint);

-- Environment + priority filtering
CREATE INDEX idx_remediation_requests_env_priority ON remediation_requests
    (environment, priority, created_at DESC);

-- JSONB labels GIN index
CREATE INDEX idx_remediation_requests_labels ON remediation_requests
    USING gin(labels);
```

---

### workflow_executions Indexes

```sql
-- Primary lookup by target
CREATE INDEX idx_workflow_executions_target ON workflow_executions
    (namespace, workflow_name, created_at DESC);

-- Parent reference
CREATE INDEX idx_workflow_executions_parent ON workflow_executions
    (remediation_request_ref);

-- Success rate queries
CREATE INDEX idx_workflow_executions_phase ON workflow_executions
    (workflow_name, phase, completed_at DESC);
```

---

### ai_analysis Indexes

```sql
-- Parent reference
CREATE INDEX idx_ai_analysis_parent ON ai_analysis(remediation_request_ref);

-- Confidence-based queries
CREATE INDEX idx_ai_analysis_confidence ON ai_analysis
    (confidence DESC, created_at DESC)
    WHERE phase = 'Completed';

-- LLM provider analytics
CREATE INDEX idx_ai_analysis_provider ON ai_analysis
    (llm_provider, llm_model, created_at DESC);
```

---

## Partitioning Strategy

### Monthly Partitioning

All audit trail tables are partitioned by month for:
- **Query Performance**: Recent queries use partition pruning
- **Data Management**: Old partitions can be archived
- **Maintenance**: Partition-level vacuum and analyze

---

### Automatic Partition Creation

```sql
-- Function to create next month's partition
CREATE OR REPLACE FUNCTION create_next_month_partition()
RETURNS void AS $$
DECLARE
    next_month_start DATE;
    next_month_end DATE;
    partition_name TEXT;
BEGIN
    next_month_start := DATE_TRUNC('month', CURRENT_DATE + INTERVAL '1 month');
    next_month_end := next_month_start + INTERVAL '1 month';
    partition_name := 'remediation_requests_' || TO_CHAR(next_month_start, 'YYYY_MM');

    EXECUTE format('
        CREATE TABLE IF NOT EXISTS %I
        PARTITION OF remediation_requests
        FOR VALUES FROM (%L) TO (%L)',
        partition_name, next_month_start, next_month_end
    );
END;
$$ LANGUAGE plpgsql;

-- Cron job to create partitions (runs monthly)
SELECT cron.schedule('create-partitions', '0 0 1 * *',
    'SELECT create_next_month_partition()');
```

---

### Partition Cleanup

```sql
-- Archive partitions older than 1 year
CREATE OR REPLACE FUNCTION archive_old_partitions()
RETURNS void AS $$
DECLARE
    partition_name TEXT;
BEGIN
    FOR partition_name IN
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
          AND tablename LIKE 'remediation_requests_%'
          AND tablename < 'remediation_requests_' ||
              TO_CHAR(CURRENT_DATE - INTERVAL '1 year', 'YYYY_MM')
    LOOP
        EXECUTE format('DROP TABLE IF EXISTS %I', partition_name);
        RAISE NOTICE 'Dropped partition: %', partition_name;
    END LOOP;
END;
$$ LANGUAGE plpgsql;
```

---

## Common Queries

### Query 1: Environment Context

```sql
-- Fast query using environment_stats table
SELECT
    namespace,
    target_type,
    target_name,
    environment,
    SUM(total_remediations) as total_remediations,
    SUM(successful_remediations) as successful_remediations,
    AVG(avg_remediation_duration_seconds) as avg_duration,
    (SUM(priority_p0_count)::float / SUM(total_remediations)) as p0_ratio
FROM environment_stats
WHERE namespace = 'production'
  AND target_type = 'deployment'
  AND target_name = 'api-server'
  AND period_start > CURRENT_DATE - INTERVAL '7 days'
GROUP BY namespace, target_type, target_name, environment;
```

---

### Query 2: Success Rates by Workflow

```sql
-- Success rate calculation
SELECT
    workflow_name,
    COUNT(*) as total_executions,
    SUM(CASE WHEN phase = 'Completed' THEN 1 ELSE 0 END) as successful,
    SUM(CASE WHEN phase = 'Failed' THEN 1 ELSE 0 END) as failed,
    (SUM(CASE WHEN phase = 'Completed' THEN 1 ELSE 0 END)::float / COUNT(*)) as success_rate,
    AVG(EXTRACT(EPOCH FROM (completed_at - started_at))) as avg_duration_seconds,
    PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (completed_at - started_at))) as p50_duration,
    PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (completed_at - started_at))) as p95_duration
FROM workflow_executions
WHERE namespace = 'production'
  AND created_at > CURRENT_DATE - INTERVAL '30 days'
  AND completed_at IS NOT NULL
GROUP BY workflow_name
ORDER BY total_executions DESC;
```

---

### Query 3: Historical Pattern Matching

```sql
-- Pattern matching with aggregation
SELECT
    signal_name,
    COUNT(*) as occurrences,
    MODE() WITHIN GROUP (ORDER BY phase) as most_common_phase,
    AVG(CASE WHEN phase = 'Completed' THEN 1 ELSE 0 END) as success_rate,
    ARRAY_AGG(DISTINCT workflow_name) as common_workflows
FROM remediation_requests rr
JOIN workflow_executions we ON we.remediation_request_ref = rr.name
WHERE rr.namespace = 'production'
  AND rr.target_type = 'deployment'
  AND rr.created_at > CURRENT_DATE - INTERVAL '30 days'
GROUP BY signal_name
HAVING COUNT(*) >= 3
ORDER BY occurrences DESC;
```

---

### Query 4: Vector Similarity Search

```sql
-- Find similar incidents using cosine similarity
SELECT
    ie.namespace,
    ie.target_name,
    ie.root_cause,
    ie.remediation_action,
    ie.phase,
    rr.created_at,
    1 - (ie.embedding <=> $1::vector) as similarity_score
FROM incident_embeddings ie
JOIN remediation_requests rr ON rr.id = ie.remediation_request_id
WHERE ie.namespace = 'production'
  AND 1 - (ie.embedding <=> $1::vector) > 0.7  -- Similarity threshold
ORDER BY ie.embedding <=> $1::vector
LIMIT 10;
```

---

## Performance Optimization

### 1. Connection Pooling

```go
// Database connection pool configuration
db, err := sql.Open("postgres", connString)
db.SetMaxOpenConns(25)         // Max connections
db.SetMaxIdleConns(5)          // Idle connections
db.SetConnMaxLifetime(5 * time.Minute)
db.SetConnMaxIdleTime(1 * time.Minute)
```

---

### 2. Query Optimization

**Use prepared statements**:
```go
stmt, err := db.Prepare(`
    SELECT namespace, target_type, target_name, avg_remediation_duration_seconds
    FROM environment_stats
    WHERE namespace = $1 AND target_type = $2 AND target_name = $3
        AND period_start > $4
`)
defer stmt.Close()

rows, err := stmt.Query("production", "deployment", "api-server", sevenDaysAgo)
```

---

### 3. Materialized Views

```sql
-- Pre-aggregated workflow success rates
CREATE MATERIALIZED VIEW workflow_success_rates AS
SELECT
    workflow_name,
    namespace,
    COUNT(*) as total_executions,
    AVG(CASE WHEN phase = 'Completed' THEN 1.0 ELSE 0.0 END) as success_rate,
    AVG(EXTRACT(EPOCH FROM (completed_at - started_at))) as avg_duration_seconds,
    MAX(created_at) as last_execution
FROM workflow_executions
WHERE created_at > CURRENT_DATE - INTERVAL '30 days'
GROUP BY workflow_name, namespace;

-- Refresh daily
CREATE INDEX idx_workflow_success_rates ON workflow_success_rates(workflow_name, namespace);
REFRESH MATERIALIZED VIEW workflow_success_rates;
```

---

### 4. Cache-Friendly Queries

**Design queries for caching**:
- Consistent parameter order
- Deterministic results
- Time-based bucketing (not absolute timestamps)

```go
// Good: Cache-friendly query
func GetEnvironmentContext(namespace, targetType, targetName string, daysAgo int) {...}

// Bad: Not cache-friendly (absolute timestamp changes every second)
func GetEnvironmentContext(namespace, targetType, targetName string, since time.Time) {...}
```

---

## Database Maintenance

### Vacuum Schedule

```sql
-- Aggressive vacuum for partitioned tables
VACUUM ANALYZE remediation_requests;
VACUUM ANALYZE workflow_executions;
VACUUM ANALYZE ai_analysis;

-- Auto-vacuum configuration
ALTER TABLE remediation_requests SET (
    autovacuum_vacuum_scale_factor = 0.05,
    autovacuum_analyze_scale_factor = 0.02
);
```

---

### Statistics Update

```sql
-- Update statistics after bulk inserts
ANALYZE remediation_requests;
ANALYZE environment_stats;
```

---

**Document Status**: ✅ Complete
**Last Updated**: October 6, 2025
**Version**: 1.0
