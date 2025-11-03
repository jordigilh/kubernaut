# RemediationProcessor Controller - Operations Guide

**Version**: 1.0.0
**Last Updated**: October 21, 2025
**On-Call Contact**: kubernaut-ops@jordigilh.com

---

## Table of Contents

1. [Overview](#overview)
2. [Health Checks](#health-checks)
3. [Metrics and Monitoring](#metrics-and-monitoring)
4. [Operational Runbooks](#operational-runbooks)
5. [Incident Response](#incident-response)
6. [Performance Tuning](#performance-tuning)
7. [Maintenance](#maintenance)

---

## Overview

### Service Description

The RemediationProcessor controller manages remediation request processing with:

- **Primary Function**: Context enrichment, semantic classification, and deduplication of remediation requests
- **CRD**: `RemediationProcessing.remediation.kubernaut.io/v1alpha1`
- **Namespace**: `kubernaut-system`
- **Deployment**: Kubernetes Deployment with leader election (1-3 replicas)

### Key Components

| Component | Purpose | Dependencies |
|-----------|---------|--------------|
| **Controller Manager** | Main reconciliation loop | Kubernetes API Server |
| **Context Enrichment** | Historical pattern analysis | Context API (port 8080) |
| **Classification Engine** | Semantic similarity analysis | PostgreSQL (port 5432) |
| **Deduplication Service** | Time-window duplicate detection | PostgreSQL (port 5432) |
| **Metrics Server** | Prometheus metrics | Port 8080 |
| **Health Probes** | Liveness/readiness checks | Port 8081 |
| **Leader Election** | HA coordination | ConfigMap-based leases |

### Service Dependencies

| Dependency | Critical | Failure Impact |
|------------|----------|----------------|
| **PostgreSQL** | Yes | Cannot store/query remediation history |
| **Context API** | Yes | Cannot enrich with historical patterns |
| **Kubernetes API** | Yes | Cannot reconcile CRDs |

---

## Health Checks

### Liveness Probe

**Endpoint**: `GET /healthz`
**Port**: 8081
**Purpose**: Detects if controller process is alive

```bash
# Manual check
curl http://remediationprocessor-pod:8081/healthz

# Expected: 200 OK, body: "ok"
```

**Kubernetes Configuration**:
```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8081
  initialDelaySeconds: 15
  periodSeconds: 20
  timeoutSeconds: 3
  failureThreshold: 3
```

**Failure Actions**:
- Pod is restarted after 3 consecutive failures (60 seconds)
- Leader election automatically transfers to healthy replica

### Readiness Probe

**Endpoint**: `GET /readyz`
**Port**: 8081
**Purpose**: Determines if controller is ready to process requests

```bash
# Manual check
curl http://remediationprocessor-pod:8081/readyz

# Expected: 200 OK, body: "ok"
```

**Kubernetes Configuration**:
```yaml
readinessProbe:
  httpGet:
    path: /readyz
    port: 8081
  initialDelaySeconds: 5
  periodSeconds: 10
  timeoutSeconds: 3
  failureThreshold: 3
```

**Readiness Criteria**:
- PostgreSQL connection is healthy
- Context API is reachable
- Kubernetes API client is initialized
- Leader election (if enabled) is stable

### Health Check Troubleshooting

#### Issue: Liveness probe failing repeatedly
```bash
# Check pod logs for panics or deadlocks
kubectl logs -n kubernaut-system -l app=remediationprocessor --tail=200

# Check recent events
kubectl get events -n kubernaut-system --field-selector involvedObject.name=remediationprocessor --sort-by='.lastTimestamp'

# Common causes:
# 1. Goroutine leak (check go_goroutines metric)
# 2. Memory exhaustion (check process_resident_memory_bytes)
# 3. Unhandled panic (check logs for stack traces)
```

#### Issue: Readiness probe failing
```bash
# Check PostgreSQL connectivity
kubectl exec -n kubernaut-system deployment/remediationprocessor -- \
  psql -h postgres-service -U remediation_user -d kubernaut_remediation -c "SELECT 1;"

# Check Context API connectivity
kubectl exec -n kubernaut-system deployment/remediationprocessor -- \
  curl -f http://context-api:8080/health

# Check service logs
kubectl logs -n kubernaut-system -l app=remediationprocessor --tail=100 | grep -i "ready\|health"
```

---

## Metrics and Monitoring

### Prometheus Metrics Endpoint

**Endpoint**: `GET /metrics`
**Port**: 8080

```bash
# Scrape metrics
curl http://remediationprocessor-pod:8080/metrics

# Filter RemediationProcessor-specific metrics
curl http://remediationprocessor-pod:8080/metrics | grep remediationprocessor
```

### Key Metrics

#### Controller Runtime Metrics

| Metric | Type | Description | Alert Threshold |
|--------|------|-------------|-----------------|
| `controller_runtime_reconcile_total{controller="remediationprocessing"}` | Counter | Total reconciliation attempts | - |
| `controller_runtime_reconcile_errors_total{controller="remediationprocessing"}` | Counter | Failed reconciliations | >100/5min |
| `controller_runtime_reconcile_time_seconds{controller="remediationprocessing"}` | Histogram | Reconciliation duration | p99 >30s |
| `workqueue_depth{name="remediationprocessing"}` | Gauge | Items waiting in queue | >1000 |
| `workqueue_retries_total{name="remediationprocessing"}` | Counter | Retried items | >50/5min |

#### Resource Metrics

| Metric | Type | Description | Alert Threshold |
|--------|------|-------------|-----------------|
| `process_resident_memory_bytes` | Gauge | Memory usage | >2GB |
| `process_cpu_seconds_total` | Counter | CPU usage | >80% sustained |
| `go_goroutines` | Gauge | Active goroutines | >10000 |
| `go_memstats_alloc_bytes` | Gauge | Allocated heap memory | >1.5GB |

#### RemediationProcessor-Specific Metrics

| Metric | Type | Description | Alert Threshold |
|--------|------|-------------|-----------------|
| `remediationprocessor_processing_duration_seconds` | Histogram | End-to-end processing time | p99 >60s |
| `remediationprocessor_context_enrichment_latency_seconds` | Histogram | Context API call duration | p99 >5s |
| `remediationprocessor_classification_errors_total` | Counter | Semantic classification failures | >10/5min |
| `remediationprocessor_deduplication_matches_total` | Counter | Duplicate remediations detected | - |
| `remediationprocessor_postgres_connection_pool_size` | Gauge | Active PostgreSQL connections | >20 |
| `remediationprocessor_context_api_requests_total` | Counter | Context API requests made | - |
| `remediationprocessor_semantic_similarity_score` | Histogram | Similarity scores distribution | - |
| `remediationprocessor_time_window_matches` | Counter | Matches within time window | - |

### ServiceMonitor Configuration

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: remediationprocessor
  namespace: kubernaut-system
  labels:
    app: remediationprocessor
spec:
  selector:
    matchLabels:
      app: remediationprocessor
  endpoints:
    - port: metrics
      interval: 30s
      path: /metrics
      scrapeTimeout: 10s
```

### Grafana Dashboards

**Dashboard Location**: `config/grafana/remediationprocessor-dashboard.json`

**Key Panels**:
- Processing Duration (p50, p95, p99)
- Context Enrichment Latency
- Classification Error Rate
- Deduplication Match Rate
- PostgreSQL Connection Pool
- Memory and CPU Usage
- Reconciliation Queue Depth

---

## Operational Runbooks

### Runbook 1: High Context Enrichment Latency

**Symptom**: `remediationprocessor_context_enrichment_latency_seconds` p99 > 5s

**Impact**: Slow remediation processing, queue backlog

**Diagnosis**:
```bash
# Check Context API health
kubectl logs -n kubernaut-system -l app=context-api --tail=100

# Check Context API metrics
curl http://context-api:8080/metrics | grep -E "(request_duration|error)"

# Check network latency
kubectl exec -n kubernaut-system deployment/remediationprocessor -- \
  time curl -f http://context-api:8080/health
```

**Resolution**:
1. **If Context API is slow**:
   ```bash
   # Scale Context API
   kubectl scale deployment/context-api -n kubernaut-system --replicas=3
   
   # Check Context API database
   kubectl logs -n kubernaut-system -l app=context-api | grep -i "database\|postgres"
   ```

2. **If network issues**:
   ```bash
   # Check network policies
   kubectl get networkpolicy -n kubernaut-system
   
   # Check service endpoints
   kubectl get endpoints context-api -n kubernaut-system
   ```

3. **If overload**:
   ```bash
   # Increase timeout temporarily
   kubectl set env deployment/remediationprocessor -n kubernaut-system \
     CONTEXT_API_TIMEOUT=60
   
   # Reduce concurrency
   kubectl set env deployment/remediationprocessor -n kubernaut-system \
     MAX_CONCURRENCY=5
   ```

**Prevention**:
- Monitor Context API capacity proactively
- Implement caching for frequently accessed patterns
- Set up auto-scaling for Context API

---

### Runbook 2: High Classification Error Rate

**Symptom**: `remediationprocessor_classification_errors_total` > 10 errors/5min

**Impact**: Remediation requests not properly classified, potential duplicates missed

**Diagnosis**:
```bash
# Check classification error logs
kubectl logs -n kubernaut-system -l app=remediationprocessor | grep -i "classification error"

# Check semantic threshold configuration
kubectl get configmap remediationprocessor-config -n kubernaut-system -o yaml | grep semantic_threshold

# Check PostgreSQL connectivity
kubectl exec -n kubernaut-system deployment/remediationprocessor -- \
  psql -h $POSTGRES_HOST -U $POSTGRES_USER -d $POSTGRES_DATABASE -c "SELECT count(*) FROM remediation_history;"
```

**Resolution**:
1. **If PostgreSQL issues**:
   ```bash
   # Check PostgreSQL health
   kubectl logs -n kubernaut-system -l app=postgres
   
   # Check connection pool exhaustion
   kubectl logs -n kubernaut-system -l app=remediationprocessor | grep "connection pool"
   
   # Increase pool size temporarily
   kubectl set env deployment/remediationprocessor -n kubernaut-system \
     POSTGRES_MAX_CONNECTIONS=50
   ```

2. **If data quality issues**:
   ```bash
   # Check for null/invalid embeddings
   kubectl exec -n kubernaut-system deployment/postgres -- \
     psql -U remediation_user -d kubernaut_remediation -c \
     "SELECT count(*) FROM remediation_history WHERE embedding IS NULL;"
   
   # Reduce semantic threshold temporarily
   kubectl set env deployment/remediationprocessor -n kubernaut-system \
     SEMANTIC_THRESHOLD=0.75
   ```

3. **If similarity engine issues**:
   ```bash
   # Check similarity engine configuration
   kubectl get configmap remediationprocessor-config -n kubernaut-system -o yaml | grep similarity_engine
   
   # Switch to fallback engine
   kubectl set env deployment/remediationprocessor -n kubernaut-system \
     SIMILARITY_ENGINE=euclidean
   ```

**Prevention**:
- Validate embedding quality during ingestion
- Monitor classification accuracy metrics
- Implement data quality checks in pipeline

---

### Runbook 3: PostgreSQL Connection Issues

**Symptom**: `remediationprocessor_postgres_connection_pool_size` drops to 0, errors in logs

**Impact**: Cannot store or query remediation history, classification fails

**Diagnosis**:
```bash
# Check PostgreSQL pod status
kubectl get pods -n kubernaut-system -l app=postgres

# Check PostgreSQL logs
kubectl logs -n kubernaut-system -l app=postgres --tail=100

# Test connection from RemediationProcessor
kubectl exec -n kubernaut-system deployment/remediationprocessor -- \
  psql -h $POSTGRES_HOST -U $POSTGRES_USER -d $POSTGRES_DATABASE -c "SELECT version();"

# Check connection string configuration
kubectl get secret remediationprocessor-secret -n kubernaut-system -o yaml
```

**Resolution**:
1. **If PostgreSQL is down**:
   ```bash
   # Restart PostgreSQL
   kubectl rollout restart statefulset/postgres -n kubernaut-system
   
   # Wait for ready
   kubectl rollout status statefulset/postgres -n kubernaut-system
   ```

2. **If credentials invalid**:
   ```bash
   # Verify credentials
   kubectl get secret remediationprocessor-secret -n kubernaut-system -o jsonpath='{.data.postgres_password}' | base64 -d
   
   # Update secret if needed
   kubectl create secret generic remediationprocessor-secret \
     --from-literal=postgres_password=NEW_PASSWORD \
     --dry-run=client -o yaml | kubectl apply -f -
   
   # Restart RemediationProcessor to pick up new secret
   kubectl rollout restart deployment/remediationprocessor -n kubernaut-system
   ```

3. **If connection pool exhausted**:
   ```bash
   # Check current connections
   kubectl exec -n kubernaut-system statefulset/postgres -- \
     psql -U postgres -c "SELECT count(*) FROM pg_stat_activity WHERE datname='kubernaut_remediation';"
   
   # Increase max connections in PostgreSQL
   kubectl exec -n kubernaut-system statefulset/postgres -- \
     psql -U postgres -c "ALTER SYSTEM SET max_connections = 200;"
   
   # Restart PostgreSQL
   kubectl rollout restart statefulset/postgres -n kubernaut-system
   ```

**Prevention**:
- Monitor PostgreSQL connection pool usage
- Set up connection pool alerts (<20% available)
- Implement connection pooling with pgBouncer

---

### Runbook 4: Context API Unreachable

**Symptom**: `remediationprocessor_context_api_requests_total` errors spiking, timeout logs

**Impact**: Cannot enrich remediations with historical context

**Diagnosis**:
```bash
# Check Context API pod status
kubectl get pods -n kubernaut-system -l app=context-api

# Test Context API endpoint
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
  curl -v http://context-api.kubernaut-system.svc.cluster.local:8080/health

# Check DNS resolution
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nslookup context-api.kubernaut-system.svc.cluster.local

# Check network policies
kubectl get networkpolicy -n kubernaut-system
```

**Resolution**:
1. **If Context API pods are down**:
   ```bash
   # Check pod events
   kubectl describe pods -n kubernaut-system -l app=context-api
   
   # Restart Context API
   kubectl rollout restart deployment/context-api -n kubernaut-system
   
   # Scale up if needed
   kubectl scale deployment/context-api -n kubernaut-system --replicas=2
   ```

2. **If DNS issues**:
   ```bash
   # Check CoreDNS
   kubectl get pods -n kube-system -l k8s-app=kube-dns
   
   # Restart CoreDNS if needed
   kubectl rollout restart deployment/coredns -n kube-system
   ```

3. **If network policy blocking**:
   ```bash
   # Check network policies
   kubectl get networkpolicy -n kubernaut-system -o yaml
   
   # Temporarily allow all (for testing only!)
   kubectl label namespace kubernaut-system network-policy=allow-all
   ```

**Prevention**:
- Monitor Context API uptime
- Implement retry logic with exponential backoff
- Set up Context API high availability (3+ replicas)

---

### Runbook 5: Deduplication Failure

**Symptom**: `remediationprocessor_deduplication_matches_total` drops to 0, duplicate remediations created

**Impact**: Multiple remediation attempts for same issue, wasted resources

**Diagnosis**:
```bash
# Check deduplication configuration
kubectl get configmap remediationprocessor-config -n kubernaut-system -o yaml | grep -A 5 "classification:"

# Check time window configuration
kubectl logs -n kubernaut-system -l app=remediationprocessor | grep "time_window"

# Query PostgreSQL for recent remediations
kubectl exec -n kubernaut-system statefulset/postgres -- \
  psql -U remediation_user -d kubernaut_remediation -c \
  "SELECT id, created_at, similarity_score FROM remediation_history ORDER BY created_at DESC LIMIT 20;"
```

**Resolution**:
1. **If time window too narrow**:
   ```bash
   # Increase time window
   kubectl set env deployment/remediationprocessor -n kubernaut-system \
     TIME_WINDOW_MINUTES=120
   
   # Restart to apply
   kubectl rollout restart deployment/remediationprocessor -n kubernaut-system
   ```

2. **If semantic threshold too high**:
   ```bash
   # Lower threshold to catch more duplicates
   kubectl set env deployment/remediationprocessor -n kubernaut-system \
     SEMANTIC_THRESHOLD=0.80
   
   # Monitor impact
   kubectl logs -n kubernaut-system -l app=remediationprocessor -f | grep "deduplication"
   ```

3. **If PostgreSQL query issues**:
   ```bash
   # Check query performance
   kubectl exec -n kubernaut-system statefulset/postgres -- \
     psql -U remediation_user -d kubernaut_remediation -c \
     "EXPLAIN ANALYZE SELECT * FROM remediation_history WHERE created_at > NOW() - INTERVAL '1 hour';"
   
   # Add index if missing
   kubectl exec -n kubernaut-system statefulset/postgres -- \
     psql -U remediation_user -d kubernaut_remediation -c \
     "CREATE INDEX IF NOT EXISTS idx_remediation_created_at ON remediation_history(created_at DESC);"
   ```

**Prevention**:
- Monitor deduplication match rate (should be 5-15%)
- Tune semantic threshold based on domain
- Implement periodic cleanup of old remediation history

---

### Runbook 6: Memory Pressure

**Symptom**: `process_resident_memory_bytes` > 2GB, OOMKilled events

**Impact**: Pod restarts, processing interruptions, queue backlog

**Diagnosis**:
```bash
# Check memory usage
kubectl top pod -n kubernaut-system -l app=remediationprocessor

# Check memory limits
kubectl get deployment remediationprocessor -n kubernaut-system -o yaml | grep -A 2 "resources:"

# Check Go heap profile
kubectl exec -n kubernaut-system deployment/remediationprocessor -- \
  curl http://localhost:8080/debug/pprof/heap > heap.prof

# Analyze with pprof
go tool pprof -top heap.prof
```

**Resolution**:
1. **If memory leak**:
   ```bash
   # Restart pod immediately
   kubectl delete pod -n kubernaut-system -l app=remediationprocessor
   
   # Reduce concurrency to lower memory footprint
   kubectl set env deployment/remediationprocessor -n kubernaut-system \
     MAX_CONCURRENCY=5
   
   # Reduce batch size
   kubectl set env deployment/remediationprocessor -n kubernaut-system \
     CLASSIFICATION_BATCH_SIZE=50
   ```

2. **If legitimate high memory usage**:
   ```bash
   # Increase memory limits
   kubectl set resources deployment/remediationprocessor -n kubernaut-system \
     --limits=memory=4Gi --requests=memory=2Gi
   
   # Enable memory ballast (Go 1.19+)
   kubectl set env deployment/remediationprocessor -n kubernaut-system \
     GOMEMLIMIT=3GiB
   ```

3. **If goroutine leak**:
   ```bash
   # Check goroutine count
   kubectl exec -n kubernaut-system deployment/remediationprocessor -- \
     curl http://localhost:8080/metrics | grep go_goroutines
   
   # Get goroutine profile
   kubectl exec -n kubernaut-system deployment/remediationprocessor -- \
     curl http://localhost:8080/debug/pprof/goroutine?debug=2 > goroutines.txt
   
   # Analyze for stuck goroutines
   grep "goroutine" goroutines.txt | wc -l
   ```

**Prevention**:
- Set appropriate resource limits and requests
- Monitor memory trends over time
- Implement periodic profiling in staging environment

---

### Runbook 7: Leader Election Failure

**Symptom**: Multiple pods reconciling simultaneously, duplicate processing

**Impact**: Race conditions, inconsistent state, resource waste

**Diagnosis**:
```bash
# Check leader election status
kubectl get lease -n kubernaut-system

# Check which pod is leader
kubectl logs -n kubernaut-system -l app=remediationprocessor | grep "leader election"

# Check for split-brain
kubectl get pods -n kubernaut-system -l app=remediationprocessor -o wide

# Check ConfigMap locks
kubectl get configmap -n kubernaut-system | grep remediationprocessor-lock
```

**Resolution**:
1. **If lease expired**:
   ```bash
   # Delete stale lease
   kubectl delete lease remediationprocessor-lease -n kubernaut-system
   
   # Restart all pods to re-elect
   kubectl rollout restart deployment/remediationprocessor -n kubernaut-system
   ```

2. **If network partition**:
   ```bash
   # Check pod-to-pod connectivity
   kubectl exec -n kubernaut-system deployment/remediationprocessor -- \
     ping -c 3 <other-pod-ip>
   
   # Check kube-apiserver connectivity
   kubectl get --raw /healthz
   ```

3. **If RBAC issues**:
   ```bash
   # Check RBAC permissions
   kubectl auth can-i create leases --as=system:serviceaccount:kubernaut-system:remediationprocessor -n kubernaut-system
   
   # Verify ServiceAccount
   kubectl get sa remediationprocessor -n kubernaut-system
   
   # Check RoleBinding
   kubectl get rolebinding -n kubernaut-system | grep remediationprocessor
   ```

**Prevention**:
- Monitor lease renewal frequency
- Set appropriate lease duration (default 15s)
- Implement proper RBAC for leader election

---

### Runbook 8: High Semantic Threshold Miss Rate

**Symptom**: Very few `remediationprocessor_deduplication_matches_total`, many unique classifications

**Impact**: Over-classification, missed duplicates, storage bloat

**Diagnosis**:
```bash
# Check classification distribution
kubectl exec -n kubernaut-system statefulset/postgres -- \
  psql -U remediation_user -d kubernaut_remediation -c \
  "SELECT similarity_score, count(*) FROM remediation_history GROUP BY similarity_score ORDER BY similarity_score;"

# Check semantic threshold
kubectl get configmap remediationprocessor-config -n kubernaut-system -o yaml | grep semantic_threshold

# Check embedding quality
kubectl exec -n kubernaut-system statefulset/postgres -- \
  psql -U remediation_user -d kubernaut_remediation -c \
  "SELECT count(*), AVG(array_length(embedding, 1)) FROM remediation_history WHERE embedding IS NOT NULL;"
```

**Resolution**:
1. **If threshold too high**:
   ```bash
   # Lower threshold incrementally
   kubectl set env deployment/remediationprocessor -n kubernaut-system \
     SEMANTIC_THRESHOLD=0.80
   
   # Monitor for 1 hour
   watch -n 60 'kubectl exec -n kubernaut-system deployment/remediationprocessor -- curl -s http://localhost:8080/metrics | grep deduplication_matches'
   
   # Adjust further if needed
   kubectl set env deployment/remediationprocessor -n kubernaut-system \
     SEMANTIC_THRESHOLD=0.75
   ```

2. **If similarity engine mismatch**:
   ```bash
   # Try different similarity engine
   kubectl set env deployment/remediationprocessor -n kubernaut-system \
     SIMILARITY_ENGINE=euclidean
   
   # Or cosine (default)
   kubectl set env deployment/remediationprocessor -n kubernaut-system \
     SIMILARITY_ENGINE=cosine
   ```

3. **If data normalization issues**:
   ```bash
   # Check for unnormalized embeddings
   kubectl exec -n kubernaut-system statefulset/postgres -- \
     psql -U remediation_user -d kubernaut_remediation -c \
     "SELECT id, sqrt(sum(e^2)) as magnitude FROM remediation_history, unnest(embedding) e GROUP BY id HAVING sqrt(sum(e^2)) > 1.1;"
   ```

**Prevention**:
- Baseline semantic threshold against historical data
- Periodically review classification accuracy
- Implement A/B testing for threshold tuning

---

## Incident Response

### Severity Levels

| Level | Definition | Response Time | Examples |
|-------|------------|---------------|----------|
| **P0 - Critical** | Service completely down | Immediate | All pods CrashLooping, PostgreSQL unavailable |
| **P1 - High** | Major functionality impaired | <15 minutes | Context API unreachable, high error rate (>50%) |
| **P2 - Medium** | Degraded performance | <1 hour | High latency, memory pressure |
| **P3 - Low** | Minor issues, no impact | <4 hours | Single pod restart, isolated errors |

### Escalation Path

1. **On-Call Engineer** → Check runbooks, attempt resolution
2. **Senior Engineer** (if >30min) → Complex diagnosis, code changes
3. **Engineering Manager** (if >1hr) → Resource allocation, customer communication
4. **CTO** (if P0 >2hr) → Executive escalation

### Emergency Contacts

- **On-Call Slack**: `#kubernaut-oncall`
- **PagerDuty**: Integration key in K8s secret
- **Engineering Lead**: See internal wiki

---

## Performance Tuning

### Concurrency Settings

```bash
# Default: 10 concurrent reconciliations
MAX_CONCURRENCY=10

# For high-throughput environments
MAX_CONCURRENCY=20

# For resource-constrained environments
MAX_CONCURRENCY=5
```

### Batch Size Tuning

```bash
# Default: 100 items per classification batch
CLASSIFICATION_BATCH_SIZE=100

# For low-latency requirements
CLASSIFICATION_BATCH_SIZE=50

# For high-throughput batch processing
CLASSIFICATION_BATCH_SIZE=200
```

### Timeout Configuration

```bash
# Context API timeout (default: 30s)
CONTEXT_API_TIMEOUT=30

# PostgreSQL query timeout (default: 10s)
POSTGRES_QUERY_TIMEOUT=10

# Increase for slow environments
CONTEXT_API_TIMEOUT=60
POSTGRES_QUERY_TIMEOUT=30
```

### Resource Allocation

**Recommended**:
```yaml
resources:
  requests:
    memory: "512Mi"
    cpu: "250m"
  limits:
    memory: "2Gi"
    cpu: "1000m"
```

**High-throughput**:
```yaml
resources:
  requests:
    memory: "1Gi"
    cpu: "500m"
  limits:
    memory: "4Gi"
    cpu: "2000m"
```

---

## Maintenance

### Regular Maintenance Tasks

| Task | Frequency | Duration | Impact |
|------|-----------|----------|--------|
| **PostgreSQL vacuum** | Weekly | 10-30min | Minor latency increase |
| **Index rebuild** | Monthly | 30-60min | Classification slower |
| **Log rotation** | Daily | <5min | None |
| **Metrics retention** | Weekly | <5min | None |

### Maintenance Windows

**Preferred**: Sunday 02:00-04:00 UTC (low traffic)

**Procedure**:
1. Announce maintenance in Slack #kubernaut-ops (24h notice)
2. Scale to 1 replica during maintenance
3. Perform maintenance tasks
4. Validate with smoke tests
5. Scale back to normal (2-3 replicas)
6. Monitor for 30 minutes post-maintenance

### Backup and Recovery

**PostgreSQL Backups**:
```bash
# Daily automated backups via CronJob
kubectl get cronjob postgres-backup -n kubernaut-system

# Manual backup
kubectl exec -n kubernaut-system statefulset/postgres -- \
  pg_dump -U remediation_user kubernaut_remediation > remediation_backup_$(date +%Y%m%d).sql
```

**Configuration Backups**:
```bash
# Backup ConfigMap
kubectl get configmap remediationprocessor-config -n kubernaut-system -o yaml > config_backup.yaml

# Backup Secret
kubectl get secret remediationprocessor-secret -n kubernaut-system -o yaml > secret_backup.yaml
```

---

**End of Operations Guide**










