# Data Storage Service - Documentation Hub

**Version**: 2.2 (Test Count Correction)
**Last Updated**: December 15, 2025
**Service Type**: Stateless HTTP API (Write & Query + Analytics)
**Status**: ⚠️ **TEST COUNTS CORRECTED** - See DS_V1.0_TRIAGE_2025-12-15.md
**Actual Tests**: 221 verified (38 E2E + 164 API E2E + 15 Integration + 4 Perf) + ~551 Unit (unverified)
**HTTP Port**: 8080 (REST API + Health)
**Metrics Port**: 9090 (`/metrics`)

---

## 🗂️ Documentation Index

| Document | Purpose | Lines | Status |
|----------|---------|-------|--------|
| **[Overview](./overview.md)** | Service purpose, architecture, design decisions | ~594 | ✅ Complete |
| **[API Specification](./api-specification.md)** | REST API endpoints, schemas, examples | ~1,249 | ✅ Complete |
| **[Testing Strategy](./testing-strategy.md)** | Unit/Integration/E2E tests, defense-in-depth | ~1,365 | ✅ Complete |
| **[Security Configuration](./security-configuration.md)** | RBAC, validation, container security | ~629 | ✅ Complete |
| **[Observability & Logging](./observability-logging.md)** | Structured logging, correlation IDs | ~436 | ✅ Complete |
| **[Metrics & SLOs](./metrics-slos.md)** | SLIs, SLOs, Prometheus metrics, alerts | ~400 | ✅ Complete |
| **[Integration Points](./integration-points.md)** | Service coordination, PostgreSQL, Vector DB | ~1,143 | ✅ Complete |
| **[Implementation Checklist](./implementation-checklist.md)** | APDC-TDD phases, validation steps | ~378 | ✅ Complete |
| **[Business Requirements](./BUSINESS_REQUIREMENTS.md)** | 31 BRs with acceptance criteria | ~701 | ✅ Complete |
| **[BR Mapping](./BR_MAPPING.md)** | BR-to-test traceability matrix | ~288 | ✅ Complete |
| **[Performance Requirements](./performance-requirements.md)** | Latency targets, throughput, scaling | ~440 | ✅ Complete |
| **[Embedding Requirements](./embedding-requirements.md)** | Vector embeddings, semantic search | ~417 | ✅ Complete |

**Total**: ~8,040 lines across 12 core specification documents
**Status**: ✅ **100% Complete** - Production-ready HTTP API with comprehensive documentation

**Additional Documentation**:
- **[Observability Runbooks](./observability/)** - ALERTING_RUNBOOK.md, PROMETHEUS_QUERIES.md
- **[OpenAPI Specifications](./openapi/)** - v1.yaml, v2.yaml, v3.yaml
- **[Event Data Schemas](./schemas/)** - PostgreSQL audit schemas
- **[Implementation Guides](./implementation/)** - Phase-by-phase implementation plans

---

## 📁 File Organization

```
data-storage/
├── 📄 README.md (you are here)              - Service index & navigation
├── 📘 overview.md                           - High-level architecture ✅ (594 lines)
├── 🔧 api-specification.md                  - REST API definitions ✅ (1,249 lines)
├── 🧪 testing-strategy.md                   - Test patterns ✅ (1,365 lines)
├── 🔒 security-configuration.md             - Security & validation ✅ (629 lines)
├── 📊 observability-logging.md              - Logging & debugging ✅ (436 lines)
├── 📈 metrics-slos.md                       - SLIs/SLOs & alerts ✅ (400 lines)
├── 🔗 integration-points.md                 - Service coordination ✅ (1,143 lines)
├── ✅ implementation-checklist.md           - APDC-TDD phases ✅ (378 lines)
├── 📋 BUSINESS_REQUIREMENTS.md              - 31 BRs with test mapping ✅ (701 lines)
├── 📋 BR_MAPPING.md                         - BR-to-test traceability ✅ (288 lines)
├── ⚡ performance-requirements.md           - Latency & throughput ✅ (440 lines)
├── 🔍 embedding-requirements.md             - Vector embeddings ✅ (417 lines)
├── 📚 observability/                        - Production operational guides
│   ├── ALERTING_RUNBOOK.md                 - Alert troubleshooting (750+ lines)
│   ├── PROMETHEUS_QUERIES.md               - 50+ query examples (730+ lines)
│   ├── DEPLOYMENT_CONFIGURATION.md         - Observability setup (680+ lines)
│   └── grafana-dashboard.json              - Pre-built Grafana dashboard
├── 🔧 openapi/                              - REST API specifications
│   ├── v1.yaml                             - OpenAPI 3.0 spec (legacy)
│   ├── v2.yaml                             - OpenAPI 3.0 spec (current)
│   └── v3.yaml                             - OpenAPI 3.0 spec (latest)
├── 📊 schemas/                              - Database schemas
│   └── event_data/                          - Audit event schemas
│       └── README.md                        - Schema documentation
├── 🧪 api/                                  - OpenAPI documentation
│   └── audit-write-api.openapi.yaml        - Audit write API spec
└── 📁 implementation/                       - Implementation phase guides
    ├── 00-GETTING-STARTED.md               - Developer onboarding
    ├── sessions/                            - Session artifacts
    ├── testing/                             - Test documentation
    │   └── BR-COVERAGE-MATRIX.md           - BR-to-test traceability
    ├── archive/                             - Versioned implementation plans
    └── design/                              - Design documents (DD-STORAGE-XXX)
```

**Legend**:
- ✅ = Complete documentation
- 📋 = Core specification document
- 🧪 = Test-related documentation
- 📚 = Operational documentation
- 🔧 = API/Schema documentation

---

## 🏗️ Implementation Structure

### **Binary Location**
- **Directory**: `cmd/datastorage/`
- **Entry Point**: `cmd/datastorage/main.go`
- **Build Command**: `go build -o bin/data-storage-service ./cmd/datastorage`

### **Business Logic**
- **Package**: `pkg/datastorage/`
  - `repository/` - PostgreSQL repository implementations
  - `server/` - HTTP server and handlers
  - `models/` - Request/response types
  - `validation/` - Request validation
  - `dlq/` - Dead letter queue (Redis)
  - `audit/` - Self-auditing (DD-STORAGE-012)
  - `embedding/` - Vector embedding generation
- **Shared Types**: `pkg/shared/types/` - Shared types across services

### **Tests** ⚠️ CORRECTED (2025-12-15)
- `test/unit/datastorage/` - **~551 unit tests** (70%+ coverage, not verified)
- `test/integration/datastorage/` - **164 API E2E tests** (PostgreSQL + Podman + HTTP) ⚠️ Misclassified
- `test/integration/datastorage/*_repository_*` - **15 integration tests** (Real DB, created 2025-12-15)
- `test/e2e/datastorage/` - **38 E2E tests** (Kind cluster)
- `test/performance/datastorage/` - **4 performance tests** (Load testing)

**Note**: Tests in `test/integration/datastorage/` are actually E2E tests (deploy containers, HTTP calls).
See [DS_V1.0_TRIAGE_2025-12-15.md](../../../handoff/DS_V1.0_TRIAGE_2025-12-15.md) for analysis.

### **Infrastructure**
- `migrations/` - PostgreSQL schema migrations (Goose)
- `docker/data-storage-service.Dockerfile` - Container image
- `deploy/data-storage/` - Kubernetes manifests

**See Also**: [cmd/ directory structure](../../../../cmd/README.md) for complete binary organization.

---

## 🎯 Purpose

**Centralized audit storage and analytics for all Kubernaut remediation activities.**

The Data Storage Service provides:
- **Persistent audit trail** for remediation workflows
- **Multi-dimensional success tracking** (ADR-033) for AI learning
- **Success rate analytics** by incident type and workflow
- **AI execution mode tracking** (catalog/chained/manual)
- **Dual-write coordination** (PostgreSQL + Vector DB)
- **Semantic search** via vector embeddings
- **Query API** for historical data retrieval
- **Self-auditing** (DD-STORAGE-012) for internal audit write monitoring
- **Comprehensive observability** with Prometheus metrics

### **Self-Auditing (DD-STORAGE-012)**

Data Storage Service audits its own operations using the **InternalAuditClient** pattern to avoid circular dependencies:

**Three Audit Points**:
1. ✅ `datastorage.audit.written` - Successful audit event writes
2. ✅ `datastorage.audit.failed` - Write failures (before DLQ fallback)
3. ✅ `datastorage.dlq.fallback` - DLQ fallback success

**Key Design**: Uses direct PostgreSQL writes (bypasses REST API) to prevent infinite recursion.

**Documentation**: [DD-STORAGE-012-HANDOFF.md](./DD-STORAGE-012-HANDOFF.md)

---

## 🚀 Quick Start

### Prerequisites

**Required**:
- PostgreSQL 16+ with pgvector 0.5.1+ extension
- Go 1.21+
- Kubernetes 1.23+ (for deployment)

**Optional**:
- Vector DB (for semantic search)
- Redis (for embedding cache)

### Local Development

```bash
# 1. Start PostgreSQL with pgvector
make test-integration-datastorage  # Starts PostgreSQL 16 via Podman

# 2. Run unit tests
make test-unit-datastorage

# 3. Run integration tests
make test-integration-datastorage

# 4. Build service
go build -o bin/data-storage-service cmd/datastorage/main.go

# 5. Run service locally
export DB_HOST=localhost
export DB_PORT=5432
export DB_USER=postgres
export DB_PASSWORD=postgres
export DB_NAME=action_history
./bin/data-storage-service
```

### Docker/Podman

```bash
# Build container image
docker build -f docker/data-storage-service.Dockerfile -t kubernaut/data-storage:latest .

# Run with PostgreSQL
docker run -d \
  -e DB_HOST=postgres \
  -e DB_PORT=5432 \
  -e DB_USER=db_user \
  -e DB_PASSWORD=slm_password \
  -e DB_NAME=action_history \
  -p 8080:8080 \
  -p 9090:9090 \
  kubernaut/data-storage:latest
```

### Kubernetes Deployment

```bash
# Deploy to Kubernetes
kubectl apply -f deploy/data-storage/

# Verify deployment
kubectl get pods -n kubernaut -l app=data-storage-service

# Check logs
kubectl logs -n kubernaut -l app=data-storage-service --tail=100

# Access metrics
kubectl port-forward -n kubernaut svc/data-storage-service 9090:9090
curl http://localhost:9090/metrics | grep datastorage
```

---

## 🔌 Service Configuration

### Basic Configuration

| Aspect | Value | Purpose |
|--------|-------|---------|
| **HTTP Port** | 8080 | REST API, `/health`, `/ready` |
| **Metrics Port** | 9090 | Prometheus `/metrics` |
| **Namespace** | `kubernaut` | Kubernetes namespace |
| **ServiceAccount** | `data-storage-sa` | RBAC permissions |

### Environment Variables

#### Database Configuration

```bash
# PostgreSQL (Required)
DB_HOST=postgres-service              # PostgreSQL hostname
DB_PORT=5432                          # PostgreSQL port
DB_USER=db_user                       # Database user
DB_PASSWORD=slm_password              # Database password
DB_NAME=action_history                # Database name
DB_MAX_CONNECTIONS=50                 # Max connection pool size
DB_SSL_MODE=disable                   # SSL mode (disable/require/verify-full)

# PostgreSQL Requirements
# - PostgreSQL 16.x or higher
# - pgvector 0.5.1 or higher
# - shared_buffers >= 1GB (recommended for HNSW performance)
```

#### Optional Configuration

```bash
# Vector DB (Optional - for semantic search)
VECTOR_DB_ENABLED=false               # Enable Vector DB dual-write
VECTOR_DB_HOST=vector-db-service      # Vector DB hostname
VECTOR_DB_PORT=8080                   # Vector DB port

# Embedding Generation (Optional)
EMBEDDING_ENABLED=false               # Enable embedding generation
EMBEDDING_MODEL=text-embedding-ada-002  # OpenAI model
EMBEDDING_API_KEY=sk-...              # OpenAI API key

# Cache Configuration (Optional)
CACHE_ENABLED=false                   # Enable embedding cache
CACHE_TYPE=redis                      # Cache backend (redis/memory)
CACHE_HOST=redis-service              # Redis hostname
CACHE_TTL=5m                          # Cache TTL

# Logging
LOG_LEVEL=info                        # Logging level (debug/info/warn/error)
LOG_FORMAT=json                       # Log format (json/console)

# Performance Tuning
QUERY_TIMEOUT=30s                     # Query timeout
WRITE_TIMEOUT=10s                     # Write timeout
CONTEXT_TIMEOUT=60s                   # Overall request timeout
```

### Configuration File

Create `config/data-storage.yaml`:

```yaml
database:
  host: postgres-service
  port: 5432
  user: db_user
  password: slm_password  # Use Kubernetes Secret in production
  name: action_history
  maxConnections: 50
  sslMode: require

vectorDB:
  enabled: false  # Enable for semantic search
  host: vector-db-service
  port: 8080

embedding:
  enabled: false  # Enable for embedding generation
  model: text-embedding-ada-002
  apiKey: ${OPENAI_API_KEY}  # From environment

cache:
  enabled: false  # Enable for better performance
  type: redis
  host: redis-service
  ttl: 5m

server:
  httpPort: 8080
  metricsPort: 9090
  queryTimeout: 30s
  writeTimeout: 10s

logging:
  level: info
  format: json
```

---

## 📊 API Reference

### Write Endpoints (4)

#### 1. Create Remediation Audit

```http
POST /api/v1/store/remediation
Content-Type: application/json

{
  "name": "pod-restart-fix-001",
  "namespace": "production",
  "phase": "completed",
  "action_type": "restart_pod",
  "status": "success",
  "start_time": "2025-10-13T10:00:00Z",
  "end_time": "2025-10-13T10:02:30Z",
  "duration": 150000,
  "remediation_request_id": "req-12345",
  "alert_fingerprint": "alert-abc",
  "severity": "high",
  "environment": "production",
  "cluster_name": "prod-cluster-01",
  "target_resource": "pod/app-server-xyz",
  "error_message": null,
  "metadata": {}
}
```

**Response** (201 Created):
```json
{
  "id": 12345,
  "created_at": "2025-10-13T10:02:31Z"
}
```

#### 2. Create AI Analysis Audit

```http
POST /api/v1/store/aianalysis
Content-Type: application/json

{
  "name": "ai-analysis-001",
  "namespace": "production",
  "analysis_type": "root_cause",
  "status": "completed",
  "start_time": "2025-10-13T10:00:00Z",
  "end_time": "2025-10-13T10:00:15Z",
  "duration": 15000,
  "confidence_score": 0.92,
  "findings": {...},
  "metadata": {}
}
```

#### 3. Create Workflow Execution Audit

```http
POST /api/v1/store/workflow
Content-Type: application/json

{
  "name": "workflow-exec-001",
  "namespace": "production",
  "workflow_type": "remediation",
  "status": "completed",
  "start_time": "2025-10-13T10:00:00Z",
  "end_time": "2025-10-13T10:05:00Z",
  "duration": 300000,
  "steps_completed": 5,
  "steps_total": 5,
  "metadata": {}
}
```

#### 4. Create Execution Audit

```http
POST /api/v1/store/execution
Content-Type: application/json

{
  "name": "k8s-exec-001",
  "namespace": "production",
  "action": "kubectl scale deployment",
  "status": "success",
  "start_time": "2025-10-13T10:01:00Z",
  "end_time": "2025-10-13T10:01:02Z",
  "duration": 2000,
  "resource": "deployment/app-server",
  "metadata": {}
}
```

### Query Endpoints (3)

#### 1. List Remediation Audits

```http
GET /api/v1/query/remediation?limit=10&offset=0&namespace=production&phase=completed
```

**Response** (200 OK):
```json
{
  "audits": [
    {
      "id": 12345,
      "name": "pod-restart-fix-001",
      "namespace": "production",
      "phase": "completed",
      ...
    }
  ],
  "total": 100,
  "limit": 10,
  "offset": 0
}
```

#### 2. Get Remediation Audit by ID

```http
GET /api/v1/query/remediation/12345
```

**Response** (200 OK):
```json
{
  "id": 12345,
  "name": "pod-restart-fix-001",
  ...
}
```

#### 3. Semantic Search

```http
POST /api/v1/query/semantic
Content-Type: application/json

{
  "query": "pod restart failures in production",
  "limit": 10
}
```

**Response** (200 OK):
```json
{
  "results": [
    {
      "audit": {...},
      "similarity": 0.92
    }
  ]
}
```

### Health Endpoints (2)

#### Liveness Probe

```http
GET /health
```

**Response** (200 OK):
```json
{
  "status": "healthy",
  "timestamp": "2025-10-13T10:00:00Z"
}
```

#### Readiness Probe

```http
GET /ready
```

**Response** (200 OK):
```json
{
  "status": "ready",
  "checks": {
    "database": "ok",
    "vector_db": "ok"
  },
  "timestamp": "2025-10-13T10:00:00Z"
}
```

---

## 🗄️ Data Storage

### Database Architecture

**PostgreSQL** (Primary):
- Audit records with full ACID guarantees
- HNSW vector indexes for semantic search
- Partitioned tables for performance
- Transaction-consistent dual-write

**Vector DB** (Optional):
- Embedding storage for semantic search
- Graceful degradation if unavailable
- PostgreSQL fallback mode

### Database Schema

#### remediation_audit Table

```sql
CREATE TABLE remediation_audit (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    namespace VARCHAR(255) NOT NULL,
    phase VARCHAR(50) NOT NULL,
    action_type VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL,
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE,
    duration BIGINT,
    remediation_request_id VARCHAR(255) NOT NULL UNIQUE,
    alert_fingerprint VARCHAR(255) NOT NULL,
    severity VARCHAR(50) NOT NULL,
    environment VARCHAR(50) NOT NULL,
    cluster_name VARCHAR(255) NOT NULL,
    target_resource VARCHAR(512) NOT NULL,
    error_message TEXT,
    metadata TEXT NOT NULL DEFAULT '{}',
    embedding vector(384),  -- pgvector for semantic search
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- HNSW index for fast semantic search (PostgreSQL 16+ only)
CREATE INDEX idx_remediation_audit_embedding ON remediation_audit
USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);
```

### Dual-Write Pattern

The service implements atomic dual-write to PostgreSQL and Vector DB:

1. **Write Phase 1**: Write to PostgreSQL (with embedding)
2. **Write Phase 2**: Write to Vector DB
3. **Commit/Rollback**: Both succeed or both fail

**Fallback Mode** (BR-STORAGE-015):
- If Vector DB is unavailable, write to PostgreSQL only
- Semantic search continues via PostgreSQL's HNSW index
- Automatic recovery when Vector DB becomes available

---

## 📊 Performance

### Latency Targets

| Operation | p50 | p95 | p99 | Target |
|-----------|-----|-----|-----|--------|
| Write (simple) | 15ms | 25ms | 50ms | < 50ms |
| Write (with embedding) | 50ms | 150ms | 250ms | < 250ms |
| Query (list) | 5ms | 10ms | 20ms | < 50ms |
| Query (get by ID) | 2ms | 5ms | 10ms | < 20ms |
| Semantic search | 20ms | 50ms | 100ms | < 100ms |

### Throughput

- **Write Operations**: 500+ writes/second
- **Query Operations**: 1000+ queries/second
- **Concurrent Clients**: 10+ services
- **Connection Pool**: 50 connections

### Caching

- **Embedding Cache**: 60-70% hit rate (target)
- **Cache TTL**: 5 minutes
- **Cache Backend**: Redis (recommended) or in-memory

---

## 📈 Observability

### Prometheus Metrics (11 metrics)

**Write Operations**:
- `datastorage_write_total{table, status}`
- `datastorage_write_duration_seconds{table}`

**Dual-Write Coordination**:
- `datastorage_dualwrite_success_total`
- `datastorage_dualwrite_failure_total{reason}`
- `datastorage_fallback_mode_total`

**Embedding & Caching**:
- `datastorage_cache_hits_total`
- `datastorage_cache_misses_total`
- `datastorage_embedding_generation_duration_seconds`

**Validation**:
- `datastorage_validation_failures_total{field, reason}`

**Query Operations**:
- `datastorage_query_total{operation, status}`
- `datastorage_query_duration_seconds{operation}`

### Grafana Dashboard

Import the pre-built dashboard:

```bash
# Dashboard JSON location
docs/services/stateless/data-storage/observability/grafana-dashboard.json

# Import via Grafana UI:
# 1. Navigate to Grafana → Dashboards → Import
# 2. Upload grafana-dashboard.json
# 3. Select Prometheus data source
# 4. Click "Import"
```

**Dashboard includes**:
- 13 panels covering all metrics
- Write/query performance graphs
- Error rate monitoring
- Cache hit rate gauge
- Semantic search latency

### Alerting

**6 production alerts configured**:

**Critical** (3):
1. `DataStorageHighWriteErrorRate` - Write errors > 5%
2. `DataStoragePostgreSQLFailure` - PostgreSQL unavailable
3. `DataStorageHighQueryErrorRate` - Query errors > 5%

**Warning** (3):
1. `DataStorageVectorDBDegraded` - Fallback mode active
2. `DataStorageLowCacheHitRate` - Cache hit rate < 50%
3. `DataStorageSlowSemanticSearch` - Search p95 > 100ms

See [observability/ALERTING_RUNBOOK.md](./observability/ALERTING_RUNBOOK.md) for troubleshooting procedures.

### Structured Logging

All logs use structured JSON format with zap:

```json
{
  "level": "info",
  "ts": "2025-10-13T10:00:00.000Z",
  "caller": "client.go:123",
  "msg": "audit created",
  "remediation_id": "req-12345",
  "id": 12345,
  "duration": "150ms",
  "namespace": "production"
}
```

---

## 🔧 Troubleshooting

### Common Issues

#### 1. PostgreSQL Connection Failed

**Symptom**: Service fails to start with "connection refused"

**Diagnosis**:
```bash
# Check PostgreSQL status
kubectl get pods -n kubernaut -l app=postgresql

# Test connection
kubectl exec -it deployment/data-storage-service -n kubernaut -- \
  psql -h postgres-service -U db_user -d action_history -c "SELECT 1;"
```

**Solutions**:
- Verify PostgreSQL is running: `kubectl get pods`
- Check credentials in ConfigMap/Secret
- Verify network connectivity: `kubectl exec ... -- nc -zv postgres-service 5432`
- Check PostgreSQL logs: `kubectl logs -l app=postgresql`

#### 2. HNSW Index Errors

**Symptom**: "HNSW index creation failed" or "HNSW validation failed"

**Diagnosis**:
```bash
# Check PostgreSQL version
kubectl exec postgresql-0 -- psql -U postgres -c "SELECT version();"

# Check pgvector version
kubectl exec postgresql-0 -- psql -U postgres -c "SELECT extversion FROM pg_extension WHERE extname = 'vector';"
```

**Solutions**:
- **Requires PostgreSQL 16+**: Upgrade if version < 16
- **Requires pgvector 0.5.1+**: Install latest pgvector
- Check shared_buffers >= 1GB for optimal HNSW performance

#### 3. High Write Latency

**Symptom**: Write p95 latency > 250ms

**Diagnosis**:
```bash
# Check metrics
curl http://localhost:9090/metrics | grep datastorage_write_duration

# Check slow queries
kubectl exec postgresql-0 -- \
  psql -U postgres -d action_history -c \
  "SELECT query, mean_exec_time FROM pg_stat_statements ORDER BY mean_exec_time DESC LIMIT 10;"
```

**Solutions**:
- Check PostgreSQL connection pool size (increase to 50+)
- Verify HNSW index is being used: `EXPLAIN ANALYZE SELECT ...`
- Increase shared_buffers to 2GB
- Enable connection pooling (PgBouncer)
- Scale to multiple replicas

#### 4. Low Cache Hit Rate

**Symptom**: Cache hit rate < 50%

**Diagnosis**:
```bash
# Check cache metrics
curl http://localhost:9090/metrics | grep datastorage_cache

# Check Redis (if used)
kubectl exec -it redis-cache-0 -- redis-cli INFO stats | grep evicted_keys
```

**Solutions**:
- Increase cache size (Redis maxmemory)
- Increase cache TTL (from 5m to 15m)
- Use LRU eviction policy: `maxmemory-policy allkeys-lru`

#### 5. Vector DB Fallback Mode

**Symptom**: `datastorage_fallback_mode_total` > 0

**Diagnosis**:
```bash
# Check Vector DB status
kubectl get pods -l app=vector-db

# Test Vector DB connectivity
kubectl exec -it deployment/data-storage-service -- \
  curl -f http://vector-db-service:8080/health
```

**Solutions**:
- Restart Vector DB: `kubectl rollout restart deployment/vector-db`
- Check Vector DB logs: `kubectl logs -l app=vector-db`
- Fallback mode is safe (data persists to PostgreSQL)

### Debugging Commands

```bash
# View service logs
kubectl logs -n kubernaut -l app=data-storage-service --tail=100 -f

# Check metrics
kubectl port-forward -n kubernaut svc/data-storage-service 9090:9090
curl http://localhost:9090/metrics | grep datastorage

# Check database connectivity
kubectl exec -it deployment/data-storage-service -n kubernaut -- \
  psql -h postgres-service -U db_user -d action_history -c "SELECT count(*) FROM remediation_audit;"

# Check resource usage
kubectl top pod -n kubernaut -l app=data-storage-service

# Describe pod for events
kubectl describe pod -n kubernaut -l app=data-storage-service
```

---

## 🔗 Integration Points

### Upstream Services (Write to Data Storage)

1. **Remediation Processor** - Stores enriched remediation data
2. **AI Analysis Service** - Stores AI analysis results
3. **Workflow Execution Service** - Stores workflow execution history
4. **Kubernetes Executor** - Stores action execution logs

### Downstream Services (Read from Data Storage)

1. **Context API** - Queries historical audit data for context
2. **Analytics Service** - Aggregates metrics and trends
3. **UI/Dashboard** - Displays audit trail to users

### Infrastructure Sharing

**Context API Service** (Phase 2 - Intelligence Layer) shares Data Storage Service infrastructure for integration testing:

- **Shared Resource**: PostgreSQL 16+ with pgvector extension (localhost:5432)
- **Shared Schema**: `internal/database/schema/remediation_audit.sql` (authoritative schema)
- **Isolation Strategy**: Schema-based isolation (`contextapi_test_<timestamp>`)
- **Benefits**: Zero schema drift guarantee, faster test execution, reduced infrastructure overhead
- **Documentation**: See [../context-api/implementation/SCHEMA_ALIGNMENT.md](../context-api/implementation/SCHEMA_ALIGNMENT.md)

**Integration Test Compatibility**:
- Data Storage tests: Use `datastorage-postgres` container (port 5432)
- Context API tests: Reuse same PostgreSQL instance with separate schema
- No conflicts: Different test schemas ensure parallel test execution safety

### External Dependencies

1. **PostgreSQL 16+** (Required)
   - Primary data storage
   - ACID guarantees
   - pgvector for semantic search
   - **Shared with**: Context API integration tests

2. **Vector DB** (Optional)
   - Enhanced semantic search
   - Graceful degradation if unavailable

3. **Redis** (Optional)
   - Embedding cache
   - Improves performance

---

## 📚 Additional Documentation

### Implementation Documentation

- **[implementation/DAY10_OBSERVABILITY_COMPLETE.md](./implementation/DAY10_OBSERVABILITY_COMPLETE.md)** - Day 10 observability summary
- **[implementation/IMPLEMENTATION_PLAN_V4.1.md](./implementation/IMPLEMENTATION_PLAN_V4.1.md)** - Complete implementation plan
- **[implementation/testing/](./implementation/testing/)** - Testing strategy and results

### Observability Documentation

- **[observability/PROMETHEUS_QUERIES.md](./observability/PROMETHEUS_QUERIES.md)** - 50+ Prometheus query examples
- **[observability/ALERTING_RUNBOOK.md](./observability/ALERTING_RUNBOOK.md)** - Alert troubleshooting procedures
- **[observability/DEPLOYMENT_CONFIGURATION.md](./observability/DEPLOYMENT_CONFIGURATION.md)** - Deployment and monitoring setup
- **[observability/grafana-dashboard.json](./observability/grafana-dashboard.json)** - Grafana dashboard JSON

### Design Documentation

- **[overview.md](./overview.md)** - Architecture and design decisions
- **[api-specification.md](./api-specification.md)** - API contracts and schemas
- **[implementation/design/](./implementation/design/)** - Design decision documents (DD-STORAGE-XXX)

---

## 🎯 Quick Reference

### Make Targets

```bash
# Testing
make test-unit-datastorage           # Run unit tests (< 1 minute)
make test-integration-datastorage    # Run integration tests (PostgreSQL via Podman, ~30s)

# Build
make build-datastorage               # Build service binary

# Development
make fmt                             # Format code
make lint                            # Run linters
```

### Environment Variables Quick Reference

```bash
# Minimal configuration (PostgreSQL only)
export DB_HOST=postgres-service
export DB_PORT=5432
export DB_USER=db_user
export DB_PASSWORD=slm_password
export DB_NAME=action_history
export LOG_LEVEL=info

# Full configuration (with Vector DB and caching)
export DB_HOST=postgres-service
export DB_PORT=5432
export DB_USER=db_user
export DB_PASSWORD=slm_password
export DB_NAME=action_history
export VECTOR_DB_ENABLED=true
export VECTOR_DB_HOST=vector-db-service
export CACHE_ENABLED=true
export CACHE_TYPE=redis
export CACHE_HOST=redis-service
export LOG_LEVEL=info
```

### Health Check URLs

```bash
# Liveness probe
curl http://localhost:8080/health

# Readiness probe
curl http://localhost:8080/ready

# Metrics
curl http://localhost:9090/metrics | grep datastorage
```

---

## 📞 Support

### Documentation Links

- **Parent**: [../README.md](../README.md) - All stateless services
- **Read Layer**: [../context-api/](../context-api/) - Complementary read service
- **Architecture**: [../../architecture/](../../architecture/)

### Contact

- **Team**: Kubernaut Data Storage Team
- **Slack**: #data-storage-team
- **Issue Tracker**: GitHub Issues
- **Runbook**: [observability/ALERTING_RUNBOOK.md](./observability/ALERTING_RUNBOOK.md)

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: December 4, 2025
**Status**: ✅ Production Ready
**Version**: 2.1

---

## Summary

- **Service**: Data Storage Service
- **Type**: Stateless HTTP API (Write & Query)
- **Status**: ⚠️ Test counts corrected (2025-12-15)
- **Test Coverage**: 221 verified tests (38 E2E + 164 API E2E + 15 Integration + 4 Perf) + ~551 Unit (unverified)
- **Observability**: 11 Prometheus metrics + Grafana dashboard
- **Performance**: < 0.01% metrics overhead
- **Dependencies**: PostgreSQL 16+ with pgvector 0.5.1+
- **Documentation**: 8,040+ lines across 12 core specification documents

**Note**: Original "727 tests" claim was inaccurate. See [DS_V1.0_TRIAGE_2025-12-15.md](../../../handoff/DS_V1.0_TRIAGE_2025-12-15.md).

---

## 📜 Version History

### **Version 2.2** (2025-12-15) - **CURRENT**
- ⚠️ **Test Count Correction**: Fixed inaccurate test counts (was "727 tests", actual: 221 verified + ~551 unverified)
- ⚠️ **Test Classification**: Identified 164 "integration" tests are actually E2E tests
- ✅ **Documentation Accuracy**: Updated README and Final Delivery doc with correct counts
- ✅ **Triage Document**: Created DS_V1.0_TRIAGE_2025-12-15.md with complete analysis
- 📋 **Reference**: See [DS_V1.0_TRIAGE_2025-12-15.md](../../../handoff/DS_V1.0_TRIAGE_2025-12-15.md)

### **Version 2.1** (2025-12-04)
- ✅ **Documentation Standardization**: README restructured to match ADR-039 template
- ✅ **Documentation Index**: Added comprehensive doc catalog with line counts (12 documents)
- ✅ **File Organization**: Visual tree showing OpenAPI, schemas, implementation docs
- ✅ **Implementation Structure**: Added binary/pkg/test location guide
- ✅ **Enhanced Navigation**: Consistent structure with all V1.0 services
- ✅ **New Documents**: Added `observability-logging.md` and `metrics-slos.md`
- ✅ **File Cleanup**: Moved 38 implementation artifacts to `implementation/` subdirectory

### **Version 2.0** (2025-11-05)
- ✅ **ADR-033 Compliance**: Multi-dimensional success tracking
- ✅ **Self-Auditing**: DD-STORAGE-012 internal audit monitoring
- ✅ **Production Ready**: Complete observability and alerting
