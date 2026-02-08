> **SUPERSEDED**: This document is superseded by DD-WORKFLOW-015 (V1.0 label-only architecture).
> pgvector and semantic search are deferred to V1.1+. Retained for historical context.

---

# Vector Database Setup Guide

This guide explains how to set up and configure the vector database features for AI-driven pattern recognition in kubernaut.

## Overview

The vector database integration enables:
- **AI-driven pattern recognition** for action recommendations
- **Semantic search** across historical actions and patterns
- **Similarity-based matching** for improved decision making
- **Pattern discovery** and effectiveness analysis

## Quick Start

### 1. Enable pgvector in PostgreSQL

```sql
-- Connect to your PostgreSQL database and enable the pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;
```

### 2. Run Database Schema

Apply the vector database schema to your existing database:

```bash
# Execute the vector schema SQL file
psql -d prometheus_alerts -f migrations/005_vector_schema.sql
```

### 3. Update Configuration

Add vector database configuration to your config file:

```yaml
# config/production.yaml
database:
  enabled: true
  host: "localhost"
  port: "5432"
  database: "prometheus_alerts"
  # ... other database settings

vectordb:
  enabled: true
  backend: "postgresql"

  embedding_service:
    service: "local"      # Local embedding generation (no external dependencies)
    dimension: 384        # Standard dimension for sentence transformers
    model: "all-MiniLM-L6-v2"

  postgresql:
    use_main_db: true     # Use same database as main application
    index_lists: 100      # Good for up to 100K patterns

  cache:
    enabled: true
    ttl: "1h"            # Cache embeddings for 1 hour
    max_size: 5000       # Max cached embeddings
```

### 4. Verify Setup

Run the application with vector database enabled:

```bash
# Start the application
./kubernaut --config config/production.yaml

# Check logs for vector database initialization
# You should see: "Using PostgreSQL vector database with pgvector"
```

## Configuration Options

### Backend Types

| Backend | Status | Use Case | Dependencies |
|---------|--------|----------|--------------|
| `postgresql` | ✅ Ready | **Recommended** for production | PostgreSQL + pgvector |
| `memory` | ✅ Ready | Development/testing | None (data lost on restart) |
| `pinecone` | 🚧 Planned | Managed cloud solution | Pinecone API key |
| `weaviate` | 🚧 Planned | Self-hosted vector DB | Weaviate instance |

### Embedding Services

| Service | Status | Description | Dependencies |
|---------|--------|-------------|--------------|
| `local` | ✅ Ready | **Recommended** - No external deps | None |
| `hybrid` | ✅ Ready | Local with external fallback | None |
| `openai` | 🚧 Planned | OpenAI embeddings | OpenAI API key |
| `huggingface` | 🚧 Planned | HuggingFace models | HF API key |

## PostgreSQL Setup Details

### Requirements

- PostgreSQL 12+ (recommended: PostgreSQL 14+)
- pgvector extension installed
- Minimum 512MB RAM for vector operations
- SSD storage recommended for performance

### Installation

#### Ubuntu/Debian
```bash
# Install pgvector extension
sudo apt install postgresql-14-pgvector

# Or build from source:
git clone https://github.com/pgvector/pgvector.git
cd pgvector
make
sudo make install
```

#### Docker
```bash
# Use official PostgreSQL image with pgvector
docker run -d --name postgres-vector \
  -e POSTGRES_PASSWORD=password \
  -p 5432:5432 \
  ankane/pgvector
```

### Performance Tuning

For optimal vector search performance, adjust PostgreSQL settings:

```sql
-- postgresql.conf adjustments for vector operations
shared_preload_libraries = 'vector'
max_connections = 100
shared_buffers = 256MB
effective_cache_size = 1GB
work_mem = 64MB

-- For vector-intensive workloads
maintenance_work_mem = 512MB
```

## Usage Examples

### Basic Pattern Storage
```go
// Initialize vector database
factory := vector.NewVectorDatabaseFactory(&config.VectorDB, db, logger)
vectorDB, err := factory.CreateVectorDatabase()

// Store an action pattern
pattern := &vector.ActionPattern{
    ID: "scale-deployment-pattern-1",
    ActionType: "scale_deployment",
    AlertName: "HighMemoryUsage",
    AlertSeverity: "warning",
    // ... other fields
}

err = vectorDB.StoreActionPattern(ctx, pattern)
```

### Semantic Search
```go
// Search for similar patterns
patterns, err := vectorDB.SearchBySemantics(ctx, "high memory usage scale deployment", 10)

// Find patterns similar to current situation
similarPatterns, err := vectorDB.FindSimilarPatterns(ctx, currentPattern, 5, 0.8)
```

### Analytics
```go
// Get pattern analytics
analytics, err := vectorDB.GetPatternAnalytics(ctx)
fmt.Printf("Total patterns: %d\n", analytics.TotalPatterns)
fmt.Printf("Average effectiveness: %.2f\n", analytics.AverageEffectiveness)
```

## Integration with Existing Features

### Effectiveness Assessment
Vector database automatically integrates with effectiveness assessment:
- Stores effectiveness scores with patterns
- Uses similarity search for better predictions
- Enables cross-pattern learning

### Workflow Orchestration
Enhances workflow recommendations:
- AI-driven workflow generation based on similar patterns
- Historical pattern analysis for workflow optimization
- Context-aware action suggestions

### Analytics Engine
Powers advanced analytics:
- Pattern discovery across action history
- Trend analysis using vector similarity
- Anomaly detection in action patterns

## Monitoring and Maintenance

### Health Checks
```bash
# Check vector database health
curl http://localhost:8080/health/vector
```

### Performance Monitoring
```sql
-- Monitor vector index performance
SELECT schemaname, tablename, indexname, idx_scan, idx_tup_read
FROM pg_stat_user_indexes
WHERE indexname LIKE '%embedding%';

-- Check vector table size
SELECT pg_size_pretty(pg_total_relation_size('action_patterns'));
```

### Backup Considerations
```bash
# Standard PostgreSQL backup includes vector data
pg_dump prometheus_alerts > backup.sql

# Restore includes vector extension and data
psql -d prometheus_alerts < backup.sql
```

## Troubleshooting

### Common Issues

**Error: extension "vector" does not exist**
```bash
# Install pgvector extension
sudo apt install postgresql-14-pgvector
# Then restart PostgreSQL
sudo systemctl restart postgresql
```

**Error: vector index creation failed**
```sql
-- Check if pgvector is loaded
SHOW shared_preload_libraries;
-- Should include 'vector'
```

**Poor search performance**
```sql
-- Check index usage
EXPLAIN ANALYZE SELECT * FROM action_patterns
ORDER BY embedding <-> '[0.1,0.2,...]'::vector LIMIT 10;
-- Should use index scan
```

### Performance Optimization

1. **Index Tuning**: Adjust `lists` parameter based on data size
2. **Memory Settings**: Increase `work_mem` for large vector operations
3. **Connection Pooling**: Use pgbouncer for high-concurrency workloads
4. **Regular VACUUM**: Vector data benefits from regular maintenance

### Debug Logging

Enable debug logging for vector operations:
```yaml
logging:
  level: "debug"
```

Look for log entries:
- `Vector database initialization completed`
- `Stored action pattern with embedding`
- `Found N similar patterns`

## Next Steps

- Review [VECTOR_DATABASE_ANALYSIS.md](./VECTOR_DATABASE_ANALYSIS.md) for architecture details
- See [config/vector-database-example.yaml](../config/vector-database-example.yaml) for full configuration examples
- Check [Vector Database Tests](../pkg/effectiveness/vector/) for usage patterns

## Support

For issues or questions:
1. Check the troubleshooting section above
2. Review existing GitHub issues
3. Enable debug logging to identify problems
4. Open a new issue with log output and configuration
