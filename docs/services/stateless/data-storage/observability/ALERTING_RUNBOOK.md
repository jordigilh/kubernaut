# Data Storage Service - Alerting Runbook

**Date**: October 13, 2025
**Service**: Data Storage Service
**BR Coverage**: BR-STORAGE-019 (Logging and metrics)

---

## Overview

This runbook provides step-by-step troubleshooting procedures for Data Storage Service alerts. Each alert includes symptoms, impact, diagnosis steps, and remediation actions.

---

## Table of Contents

1. [Critical Alerts](#critical-alerts)
2. [Warning Alerts](#warning-alerts)
3. [General Troubleshooting](#general-troubleshooting)
4. [Escalation Procedures](#escalation-procedures)

---

## Critical Alerts

### 🚨 DataStorageHighWriteErrorRate

**Alert Query**:
```promql
100 * (
  sum(rate(datastorage_write_total{status="failure"}[5m]))
  /
  sum(rate(datastorage_write_total[5m]))
) > 5
```

**Threshold**: > 5% error rate for 5 minutes
**Severity**: Critical
**BR Coverage**: BR-STORAGE-001, BR-STORAGE-002

#### Symptoms
- Write operations failing at high rate
- Users unable to create audit records
- Data loss risk for remediation workflows

#### Impact
- **User Impact**: High - Users cannot save remediation data
- **Data Impact**: High - Risk of data loss
- **System Impact**: Medium - Service degraded but queries still work

#### Diagnosis

1. **Check failure reasons**:
```bash
# View failure breakdown
kubectl logs -n kubernaut deployment/data-storage-service | grep "write failed"

# Check Prometheus for failure patterns
# Query: rate(datastorage_dualwrite_failure_total[5m]) by (reason)
```

2. **Identify root cause**:
```promql
# PostgreSQL failures?
rate(datastorage_dualwrite_failure_total{reason="postgresql_failure"}[5m])

# Vector DB failures?
rate(datastorage_dualwrite_failure_total{reason="vectordb_failure"}[5m])

# Validation failures?
rate(datastorage_dualwrite_failure_total{reason="validation_failure"}[5m])
```

3. **Check database health**:
```bash
# PostgreSQL connection test
kubectl exec -it deployment/data-storage-service -- \
  psql -h postgres-service -U db_user -d action_history -c "SELECT 1;"

# Check PostgreSQL logs
kubectl logs -n kubernaut statefulset/postgresql
```

#### Remediation

**If PostgreSQL is down**:
```bash
# Restart PostgreSQL
kubectl rollout restart statefulset/postgresql -n kubernaut

# Wait for healthy state
kubectl wait --for=condition=ready pod -l app=postgresql -n kubernaut --timeout=300s
```

**If Vector DB is down**:
```bash
# Service will auto-fallback to PostgreSQL-only mode (BR-STORAGE-015)
# Check fallback mode is active
# Query: rate(datastorage_fallback_mode_total[5m])

# Restart Vector DB if needed
kubectl rollout restart deployment/vector-db -n kubernaut
```

**If validation failures are high**:
```bash
# Investigate validation patterns
# Query: rate(datastorage_validation_failures_total[5m]) by (field, reason)

# Check for malicious input patterns (XSS/SQL injection)
# Query: rate(datastorage_validation_failures_total{reason=~"xss_detected|sql_injection_detected"}[5m])

# If attacks detected, enable rate limiting or block IPs
```

**If database connections exhausted**:
```bash
# Check active connections
kubectl exec -it deployment/data-storage-service -- \
  psql -h postgres-service -U db_user -d action_history -c \
  "SELECT count(*) FROM pg_stat_activity WHERE datname = 'action_history';"

# Increase max_connections if needed (requires PostgreSQL restart)
kubectl edit configmap postgresql-config -n kubernaut
```

#### Recovery Verification

```bash
# Verify write error rate dropped
# Query: rate(datastorage_write_total{status="failure"}[5m])

# Should be < 1 failure/sec

# Check write success rate
# Query: 100 * (sum(rate(datastorage_write_total{status="success"}[5m])) / sum(rate(datastorage_write_total[5m])))

# Should be > 99%
```

---

### 🚨 DataStoragePostgreSQLFailure

**Alert Query**:
```promql
rate(datastorage_dualwrite_failure_total{reason="postgresql_failure"}[5m]) > 0
```

**Threshold**: Any PostgreSQL write failure
**Severity**: Critical
**BR Coverage**: BR-STORAGE-014

#### Symptoms
- PostgreSQL write operations failing
- Dual-write coordinator reporting postgresql_failure
- Fallback mode may activate

#### Impact
- **User Impact**: High - No audit trail being created
- **Data Impact**: Critical - Data loss risk
- **System Impact**: High - Core database unavailable

#### Diagnosis

1. **Check PostgreSQL health**:
```bash
# Pod status
kubectl get pods -n kubernaut -l app=postgresql

# Check for crashes/restarts
kubectl describe pod -n kubernaut -l app=postgresql | grep -i "restart"

# Check PostgreSQL logs
kubectl logs -n kubernaut statefulset/postgresql --tail=100
```

2. **Test database connectivity**:
```bash
# Connection test from data-storage pod
kubectl exec -it deployment/data-storage-service -n kubernaut -- \
  psql -h postgres-service -U db_user -d action_history -c "SELECT version();"
```

3. **Check disk space**:
```bash
# PostgreSQL disk usage
kubectl exec -it statefulset/postgresql-0 -n kubernaut -- df -h /var/lib/postgresql/data
```

4. **Check connection pool**:
```bash
# Active connections
kubectl exec -it deployment/data-storage-service -n kubernaut -- \
  psql -h postgres-service -U db_user -d action_history -c \
  "SELECT count(*), state FROM pg_stat_activity GROUP BY state;"
```

#### Remediation

**If PostgreSQL is crashed**:
```bash
# Restart PostgreSQL
kubectl rollout restart statefulset/postgresql -n kubernaut

# Wait for ready
kubectl wait --for=condition=ready pod/postgresql-0 -n kubernaut --timeout=300s

# Verify health
kubectl exec -it postgresql-0 -n kubernaut -- pg_isready
```

**If disk is full**:
```bash
# Check disk usage
kubectl exec -it postgresql-0 -n kubernaut -- df -h

# If full, increase PVC size (requires downtime)
kubectl edit pvc postgresql-data-postgresql-0 -n kubernaut

# Or clean up old data (use with caution!)
kubectl exec -it postgresql-0 -n kubernaut -- \
  psql -U postgres -c "VACUUM FULL;"
```

**If connection pool exhausted**:
```bash
# Terminate idle connections
kubectl exec -it postgresql-0 -n kubernaut -- \
  psql -U postgres -d action_history -c \
  "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE state = 'idle' AND state_change < now() - interval '10 minutes';"

# Increase max_connections in PostgreSQL config
kubectl edit configmap postgresql-config -n kubernaut
# Set max_connections = 200 (requires restart)

kubectl rollout restart statefulset/postgresql -n kubernaut
```

**If network issues**:
```bash
# Test network connectivity
kubectl exec -it deployment/data-storage-service -n kubernaut -- \
  nc -zv postgres-service 5432

# Check service endpoints
kubectl get endpoints postgres-service -n kubernaut

# Check network policies
kubectl get networkpolicies -n kubernaut
```

#### Recovery Verification

```bash
# Verify PostgreSQL failures stopped
# Query: rate(datastorage_dualwrite_failure_total{reason="postgresql_failure"}[5m])

# Should be 0

# Verify dual-write success rate recovered
# Query: rate(datastorage_dualwrite_success_total[5m])

# Should be > 0 and increasing
```

---

### 🚨 DataStorageHighQueryErrorRate

**Alert Query**:
```promql
100 * (
  sum(rate(datastorage_query_total{status="failure"}[5m]))
  /
  sum(rate(datastorage_query_total[5m]))
) > 5
```

**Threshold**: > 5% query error rate for 5 minutes
**Severity**: Critical
**BR Coverage**: BR-STORAGE-007, BR-STORAGE-012, BR-STORAGE-013

#### Symptoms
- Query operations failing at high rate
- Users unable to retrieve audit records
- Semantic search unavailable

#### Impact
- **User Impact**: High - Users cannot view historical data
- **Data Impact**: Low - No data loss (read-only)
- **System Impact**: Medium - Service degraded but writes still work

#### Diagnosis

1. **Check query failure breakdown**:
```promql
# Failures by operation type
rate(datastorage_query_total{status="failure"}[5m]) by (operation)

# Identify which queries are failing:
# - list: Pagination queries
# - get: Single record retrieval
# - semantic_search: Vector similarity search
# - filter: Filtered queries
```

2. **Check database query performance**:
```bash
# Slow query log
kubectl exec -it postgresql-0 -n kubernaut -- \
  psql -U postgres -d action_history -c \
  "SELECT query, mean_exec_time, calls FROM pg_stat_statements ORDER BY mean_exec_time DESC LIMIT 10;"

# Active queries
kubectl exec -it postgresql-0 -n kubernaut -- \
  psql -U postgres -d action_history -c \
  "SELECT pid, now() - query_start AS duration, query FROM pg_stat_activity WHERE state = 'active' ORDER BY duration DESC;"
```

3. **Check HNSW index health (if semantic_search failing)**:
```bash
# Verify HNSW index exists
kubectl exec -it postgresql-0 -n kubernaut -- \
  psql -U postgres -d action_history -c \
  "SELECT indexname, indexdef FROM pg_indexes WHERE tablename = 'remediation_audit' AND indexdef LIKE '%hnsw%';"

# Check index size
kubectl exec -it postgresql-0 -n kubernaut -- \
  psql -U postgres -d action_history -c \
  "SELECT pg_size_pretty(pg_relation_size('idx_remediation_audit_embedding'));"
```

#### Remediation

**If semantic search is slow/failing**:
```bash
# Rebuild HNSW index (WARNING: blocking operation)
kubectl exec -it postgresql-0 -n kubernaut -- \
  psql -U postgres -d action_history -c \
  "REINDEX INDEX CONCURRENTLY idx_remediation_audit_embedding;"

# Or increase shared_buffers for better HNSW performance
kubectl edit configmap postgresql-config -n kubernaut
# Set shared_buffers = 2GB (requires restart)
```

**If database connection issues**:
```bash
# Same as PostgreSQL failure remediation above
```

**If query timeouts**:
```bash
# Increase query timeout in application
kubectl set env deployment/data-storage-service -n kubernaut \
  QUERY_TIMEOUT=30s

# Or add missing indexes
kubectl exec -it postgresql-0 -n kubernaut -- \
  psql -U postgres -d action_history -c \
  "CREATE INDEX CONCURRENTLY idx_remediation_audit_namespace_phase ON remediation_audit(namespace, phase);"
```

#### Recovery Verification

```bash
# Verify query error rate dropped
# Query: rate(datastorage_query_total{status="failure"}[5m])

# Should be < 0.1 failures/sec

# Verify query success rate
# Query: 100 * (sum(rate(datastorage_query_total{status="success"}[5m])) / sum(rate(datastorage_query_total[5m])))

# Should be > 99%

# Check semantic search latency
# Query: histogram_quantile(0.95, rate(datastorage_query_duration_seconds_bucket{operation="semantic_search"}[5m]))

# Should be < 0.05 (50ms)
```

---

## Warning Alerts

### ⚠️ DataStorageVectorDBDegraded

**Alert Query**:
```promql
rate(datastorage_fallback_mode_total[5m]) > 0
```

**Threshold**: Any fallback operations
**Severity**: Warning
**BR Coverage**: BR-STORAGE-015

#### Symptoms
- Writes succeeding but only to PostgreSQL
- Vector DB unavailable
- Semantic search may be degraded or unavailable

#### Impact
- **User Impact**: Low - Writes still work
- **Data Impact**: Low - Data saved to PostgreSQL
- **System Impact**: Medium - Vector search unavailable

#### Diagnosis

1. **Check Vector DB health**:
```bash
# Pod status
kubectl get pods -n kubernaut -l app=vector-db

# Check logs
kubectl logs -n kubernaut deployment/vector-db --tail=100
```

2. **Test Vector DB connectivity**:
```bash
# Connection test
kubectl exec -it deployment/data-storage-service -n kubernaut -- \
  curl -f http://vector-db-service:8080/health || echo "Vector DB unreachable"
```

#### Remediation

**Restart Vector DB**:
```bash
# Restart Vector DB service
kubectl rollout restart deployment/vector-db -n kubernaut

# Wait for ready
kubectl wait --for=condition=ready pod -l app=vector-db -n kubernaut --timeout=300s

# Verify health
kubectl exec -it deployment/vector-db-0 -n kubernaut -- curl -f http://localhost:8080/health
```

**Backfill Vector DB** (if needed):
```bash
# Trigger backfill job to sync missing embeddings from PostgreSQL
kubectl create job vector-backfill --from=cronjob/vector-backfill-cron -n kubernaut

# Monitor backfill progress
kubectl logs -n kubernaut job/vector-backfill --follow
```

#### Recovery Verification

```bash
# Verify fallback mode stopped
# Query: rate(datastorage_fallback_mode_total[5m])

# Should be 0

# Verify dual-write success resumed
# Query: rate(datastorage_dualwrite_success_total[5m])

# Should be > 0
```

---

### ⚠️ DataStorageLowCacheHitRate

**Alert Query**:
```promql
rate(datastorage_cache_hits_total[5m])
/
(rate(datastorage_cache_hits_total[5m]) + rate(datastorage_cache_misses_total[5m]))
< 0.5
```

**Threshold**: < 50% cache hit rate for 15 minutes
**Severity**: Warning
**BR Coverage**: BR-STORAGE-009

#### Symptoms
- High embedding generation load
- Increased write latency
- High cache miss rate

#### Impact
- **User Impact**: Low - Service still functional
- **Data Impact**: None
- **System Impact**: Medium - Increased load on embedding service

#### Diagnosis

1. **Check cache size and eviction rate**:
```bash
# Check Redis (if used for caching) metrics
kubectl exec -it deployment/redis-cache -n kubernaut -- \
  redis-cli INFO stats | grep evicted_keys

# High eviction rate indicates cache is too small
```

2. **Check embedding generation rate**:
```promql
# Embedding generation throughput
rate(datastorage_embedding_generation_duration_seconds_count[5m])

# If very high, cache is undersized
```

#### Remediation

**Increase cache size** (if using Redis):
```bash
# Edit Redis config
kubectl edit configmap redis-config -n kubernaut

# Increase maxmemory
# maxmemory: "2gb"  # Increase from 1gb

# Restart Redis
kubectl rollout restart deployment/redis-cache -n kubernaut
```

**Adjust cache eviction policy**:
```bash
# Use LRU eviction policy
kubectl exec -it deployment/redis-cache -n kubernaut -- \
  redis-cli CONFIG SET maxmemory-policy allkeys-lru
```

#### Recovery Verification

```bash
# Verify cache hit rate improved
# Query: rate(datastorage_cache_hits_total[5m]) / (rate(datastorage_cache_hits_total[5m]) + rate(datastorage_cache_misses_total[5m]))

# Should be > 0.8 (80%)

# Verify embedding generation rate decreased
# Query: rate(datastorage_embedding_generation_duration_seconds_count[5m])

# Should decrease as cache hit rate improves
```

---

### ⚠️ DataStorageSlowSemanticSearch

**Alert Query**:
```promql
histogram_quantile(0.95, rate(datastorage_query_duration_seconds_bucket{operation="semantic_search"}[5m]))
> 0.1
```

**Threshold**: p95 latency > 100ms for 10 minutes
**Severity**: Warning
**BR Coverage**: BR-STORAGE-012

#### Symptoms
- Semantic search taking longer than expected
- Users experiencing slow search results
- HNSW index not performing optimally

#### Impact
- **User Impact**: Medium - Slow search experience
- **Data Impact**: None
- **System Impact**: Low - Increased query latency

#### Diagnosis

1. **Check HNSW index configuration**:
```bash
# Verify HNSW parameters
kubectl exec -it postgresql-0 -n kubernaut -- \
  psql -U postgres -d action_history -c \
  "SELECT indexrelid::regclass, indoption FROM pg_index WHERE indexrelid = 'idx_remediation_audit_embedding'::regclass;"

# HNSW should have m=16, ef_construction=64 for optimal performance
```

2. **Check shared_buffers size**:
```bash
# Check PostgreSQL shared_buffers
kubectl exec -it postgresql-0 -n kubernaut -- \
  psql -U postgres -c "SHOW shared_buffers;"

# Should be >= 1GB for optimal HNSW performance (BR-STORAGE-012)
```

3. **Check table size and index bloat**:
```bash
# Table size
kubectl exec -it postgresql-0 -n kubernaut -- \
  psql -U postgres -d action_history -c \
  "SELECT pg_size_pretty(pg_total_relation_size('remediation_audit'));"

# Index size
kubectl exec -it postgresql-0 -n kubernaut -- \
  psql -U postgres -d action_history -c \
  "SELECT pg_size_pretty(pg_relation_size('idx_remediation_audit_embedding'));"
```

#### Remediation

**Increase shared_buffers**:
```bash
# Edit PostgreSQL config
kubectl edit configmap postgresql-config -n kubernaut

# Set shared_buffers = 2GB (from 1GB)
# Requires PostgreSQL restart

kubectl rollout restart statefulset/postgresql -n kubernaut
```

**Rebuild HNSW index** (if bloated):
```bash
# Rebuild index concurrently (non-blocking)
kubectl exec -it postgresql-0 -n kubernaut -- \
  psql -U postgres -d action_history -c \
  "REINDEX INDEX CONCURRENTLY idx_remediation_audit_embedding;"
```

**Vacuum table** (if bloated):
```bash
# Vacuum full (WARNING: blocking operation, do during maintenance window)
kubectl exec -it postgresql-0 -n kubernaut -- \
  psql -U postgres -d action_history -c \
  "VACUUM FULL ANALYZE remediation_audit;"
```

#### Recovery Verification

```bash
# Verify semantic search latency improved
# Query: histogram_quantile(0.95, rate(datastorage_query_duration_seconds_bucket{operation="semantic_search"}[5m]))

# Should be < 0.05 (50ms)

# Verify query success rate maintained
# Query: rate(datastorage_query_total{operation="semantic_search",status="success"}[5m])

# Should remain high
```

---

## General Troubleshooting

### Check Service Health

```bash
# Pod status
kubectl get pods -n kubernaut -l app=data-storage-service

# Recent logs
kubectl logs -n kubernaut deployment/data-storage-service --tail=100

# Check resource usage
kubectl top pod -n kubernaut -l app=data-storage-service
```

### Check Metrics Availability

```bash
# Check if metrics endpoint is accessible
kubectl exec -it deployment/data-storage-service -n kubernaut -- \
  curl -f http://localhost:9090/metrics | grep datastorage

# If metrics missing, check Prometheus scrape config
kubectl get servicemonitor -n kubernaut data-storage-service -o yaml
```

### Check Database Connectivity

```bash
# PostgreSQL test
kubectl exec -it deployment/data-storage-service -n kubernaut -- \
  psql -h postgres-service -U db_user -d action_history -c "SELECT 1;"

# Vector DB test (if used)
kubectl exec -it deployment/data-storage-service -n kubernaut -- \
  curl -f http://vector-db-service:8080/health
```

---

## Escalation Procedures

### Level 1: On-Call Engineer (First Responder)

**Actions**:
1. Acknowledge alert in PagerDuty/AlertManager
2. Follow runbook procedures above
3. Attempt basic remediation (restarts, connection tests)
4. Escalate to Level 2 if issue persists > 15 minutes

**Time Limit**: 15 minutes for initial triage and basic remediation

### Level 2: Database/Storage Team

**Actions**:
1. Deep dive into database performance issues
2. Analyze query plans and index usage
3. Perform database tuning and optimization
4. Escalate to Level 3 if infrastructure issues suspected

**Time Limit**: 30 minutes for advanced remediation

### Level 3: Infrastructure/SRE Team

**Actions**:
1. Investigate infrastructure issues (network, storage, compute)
2. Coordinate with cloud provider if needed
3. Implement disaster recovery procedures if necessary
4. Post-incident review and root cause analysis

**Time Limit**: 1 hour for critical incidents

---

## Post-Incident Actions

1. **Document incident**:
   - Symptoms observed
   - Root cause identified
   - Remediation steps taken
   - Time to resolution

2. **Update runbook** if new issues discovered

3. **Implement preventive measures**:
   - Add monitoring for new failure modes
   - Update alert thresholds if false positives
   - Improve automation for common fixes

4. **Conduct post-mortem** for critical incidents

---

## Contact Information

- **On-Call Engineer**: PagerDuty rotation
- **Database Team**: #database-team Slack channel
- **SRE Team**: #sre-team Slack channel
- **Emergency Escalation**: [escalation contact]

---

**Document Version**: 1.0
**Last Updated**: October 13, 2025
**Next Review**: January 13, 2026

