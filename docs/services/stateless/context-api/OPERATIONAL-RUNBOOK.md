# Context API Operational Runbook

**Version**: 1.0  
**Last Updated**: 2025-11-02  
**Status**: Production-Ready (Migration Phase)

---

## 🎯 **Quick Reference**

| Issue | Symptom | First Action | Runbook Section |
|-------|---------|--------------|-----------------|
| **High Latency** | p95 > 200ms | Check cache hit rate | [Performance Issues](#performance-issues) |
| **Circuit Breaker Open** | "circuit breaker open" logs | Check Data Storage health | [Circuit Breaker](#circuit-breaker-scenarios) |
| **Cache Misses** | High cache_misses metric | Verify Redis connectivity | [Cache Issues](#cache-issues) |
| **Data Storage 5xx** | Failed API calls | Check service logs | [Data Storage Connectivity](#data-storage-connectivity) |
| **Memory Growth** | OOMKilled pods | Check cache size limits | [Memory Issues](#memory-issues) |

---

## 📊 **Health Monitoring**

### Key Metrics (Prometheus)

```promql
# Request rate
rate(contextapi_requests_total[5m])

# Error rate
rate(contextapi_requests_total{status=~"5.."}[5m]) / rate(contextapi_requests_total[5m])

# Latency (p95)
histogram_quantile(0.95, rate(contextapi_request_duration_seconds_bucket[5m]))

# Cache hit rate
rate(contextapi_cache_hits_total[5m]) / (rate(contextapi_cache_hits_total[5m]) + rate(contextapi_cache_misses_total[5m]))

# Circuit breaker status
contextapi_circuit_breaker_open
```

### Healthy Baselines

| Metric | Healthy Range | Warning Threshold | Critical Threshold |
|--------|---------------|-------------------|-------------------|
| **Error Rate** | < 1% | > 5% | > 10% |
| **Latency p95** | < 200ms | > 500ms | > 1s |
| **Cache Hit Rate** | > 80% | < 50% | < 20% |
| **Circuit Breaker** | Closed (0) | N/A | Open (1) |

---

## 🚨 **Common Issues**

### Performance Issues

#### **Symptom**: High response latency (p95 > 500ms)

**Diagnosis**:
```bash
# 1. Check cache hit rate
kubectl logs -n kubernaut deployment/context-api | grep "cache_hit" | tail -20

# 2. Check Data Storage latency
kubectl logs -n kubernaut deployment/context-api | grep "Data Storage query" | tail -20

# 3. Check Redis connectivity
kubectl exec -n kubernaut deployment/context-api -- redis-cli -h redis-service ping
```

**Root Causes & Solutions**:

| Root Cause | Evidence | Solution |
|------------|----------|----------|
| **Low Cache Hit Rate** | `cache_misses` > 80% | See [Cache Issues](#cache-issues) |
| **Data Storage Slow** | DS query > 200ms | Scale Data Storage pods |
| **Redis Down** | Connection refused | Check Redis health |
| **High Request Volume** | QPS > 1000 | Scale Context API pods |

**Example Fix** (Scale Context API):
```bash
kubectl scale -n kubernaut deployment/context-api --replicas=5
```

---

### Circuit Breaker Scenarios

#### **Symptom**: "circuit breaker open, falling back to cache" logs

**Diagnosis**:
```bash
# 1. Check circuit breaker status
kubectl logs -n kubernaut deployment/context-api | grep "circuit breaker" | tail -10

# 2. Check Data Storage health
kubectl get pods -n kubernaut -l app=data-storage

# 3. Check recent errors
kubectl logs -n kubernaut deployment/context-api | grep "Data Storage query failed" | tail -20
```

**Circuit Breaker Behavior**:
- **Threshold**: Opens after 3 consecutive failures
- **Timeout**: Closes (half-open) after 60 seconds
- **Fallback**: Returns cached data while open

**Root Causes & Solutions**:

| Root Cause | Evidence | Solution | Recovery Time |
|------------|----------|----------|---------------|
| **Data Storage Down** | Pods not ready | Fix Data Storage | 60s (auto) |
| **Network Issue** | DNS resolution fails | Check network policies | 60s (auto) |
| **Data Storage Overload** | 503 errors | Scale Data Storage | 60s (auto) |
| **Bad Config** | Connection refused | Fix BaseURL config | Restart pods |

**Manual Circuit Breaker Reset** (if needed):
```bash
# Circuit breaker auto-closes after 60s, but you can restart pods:
kubectl rollout restart -n kubernaut deployment/context-api
```

**Validation**:
```bash
# Circuit should close after 60s
kubectl logs -n kubernaut deployment/context-api | grep "circuit breaker closing"
```

---

### Cache Issues

#### **Symptom**: High cache miss rate (< 50% hits)

**Diagnosis**:
```bash
# 1. Check Redis connectivity
kubectl exec -n kubernaut deployment/context-api -- redis-cli -h redis-service INFO

# 2. Check cache metrics
kubectl logs -n kubernaut deployment/context-api | grep "cache_hit\|cache_miss" | tail -50

# 3. Check Redis memory usage
kubectl exec -n kubernaut deployment/redis -- redis-cli INFO memory
```

**Root Causes & Solutions**:

| Root Cause | Evidence | Solution |
|------------|----------|----------|
| **Redis Down** | Connection refused | Restart Redis pods |
| **Redis Memory Full** | Eviction policy active | Increase Redis memory |
| **TTL Too Short** | Frequent cache expirations | Increase TTL in config |
| **Cache Stampede** | Concurrent misses | Already mitigated (single-flight) |

**Example Fix** (Increase Redis memory):
```yaml
# redis-deployment.yaml
resources:
  limits:
    memory: "2Gi"  # Increase from 1Gi
```

**Cache Fallback Validation**:
```bash
# Verify graceful degradation (Redis down → LRU fallback)
kubectl logs -n kubernaut deployment/context-api | grep "falling back to LRU"
```

---

### Data Storage Connectivity

#### **Symptom**: "Data Storage Service unavailable" errors

**Diagnosis**:
```bash
# 1. Check Data Storage pods
kubectl get pods -n kubernaut -l app=data-storage

# 2. Check Data Storage logs
kubectl logs -n kubernaut deployment/data-storage | tail -50

# 3. Test connectivity from Context API pod
kubectl exec -n kubernaut deployment/context-api -- curl http://data-storage-service:8080/health
```

**Root Causes & Solutions**:

| Root Cause | Evidence | Solution | Impact |
|------------|----------|----------|--------|
| **Data Storage Down** | Pods CrashLoopBackOff | Fix Data Storage | Cache fallback active |
| **Network Policy** | Connection timeout | Update NetworkPolicy | New data unavailable |
| **DNS Issue** | Name resolution failed | Check CoreDNS | New data unavailable |
| **Wrong BaseURL** | Connection refused | Update ConfigMap | Restart pods needed |

**Example Fix** (Update BaseURL):
```yaml
# context-api-config.yaml
data:
  config.yaml: |
    dataStorage:
      baseURL: "http://data-storage-service:8080"  # Fix typo
```

**Retry Behavior**:
- **Attempts**: 3 retries with exponential backoff (100ms, 200ms, 400ms)
- **Total time**: ~700ms before fallback
- **Fallback**: Returns cached data if available

---

### Memory Issues

#### **Symptom**: Context API pods OOMKilled

**Diagnosis**:
```bash
# 1. Check pod memory usage
kubectl top pods -n kubernaut -l app=context-api

# 2. Check OOM events
kubectl describe pods -n kubernaut -l app=context-api | grep -A 5 "OOMKilled"

# 3. Check cache size
kubectl logs -n kubernaut deployment/context-api | grep "cache_size\|lru_size"
```

**Root Causes & Solutions**:

| Root Cause | Evidence | Solution |
|------------|----------|----------|
| **LRU Cache Too Large** | LRU size > 10,000 | Reduce LRU size in config |
| **Memory Leak** | Steady growth over time | Report bug (enable pprof) |
| **Insufficient Limits** | Memory limit < 512Mi | Increase pod memory limit |
| **High Request Volume** | Many concurrent requests | Scale horizontally |

**Example Fix** (Reduce LRU cache size):
```yaml
# context-api-config.yaml
data:
  config.yaml: |
    cache:
      lruSize: 5000  # Reduce from 10000
      defaultTTL: 5m
```

---

## 🔧 **Configuration Management**

### Config File Location
```bash
kubectl get configmap -n kubernaut context-api-config -o yaml
```

### Key Configuration Parameters

| Parameter | Default | Production Recommendation | Impact |
|-----------|---------|---------------------------|--------|
| `cache.lruSize` | 10000 | 5000-10000 (depends on memory) | Memory usage |
| `cache.defaultTTL` | 5m | 5m-15m (depends on data freshness) | Cache hit rate |
| `dataStorage.baseURL` | N/A | `http://data-storage-service:8080` | Critical |
| `server.port` | 8080 | 8080 (standard) | N/A |
| `logging.level` | info | info (debug for troubleshooting) | Log volume |

### Configuration Update Procedure
```bash
# 1. Edit ConfigMap
kubectl edit configmap -n kubernaut context-api-config

# 2. Restart pods to pick up changes
kubectl rollout restart -n kubernaut deployment/context-api

# 3. Verify new config loaded
kubectl logs -n kubernaut deployment/context-api | grep "config loaded"
```

---

## 🔍 **Debugging**

### Enable Debug Logging
```bash
# Temporarily enable debug logs
kubectl set env -n kubernaut deployment/context-api LOG_LEVEL=debug

# Disable debug logs (production)
kubectl set env -n kubernaut deployment/context-api LOG_LEVEL=info
```

### Useful Log Queries

```bash
# Data Storage query latency
kubectl logs -n kubernaut deployment/context-api | grep "Data Storage query" | jq '.duration_ms'

# Circuit breaker events
kubectl logs -n kubernaut deployment/context-api | grep "circuit breaker"

# Cache operations
kubectl logs -n kubernaut deployment/context-api | grep "cache_hit\|cache_miss"

# RFC 7807 errors from Data Storage
kubectl logs -n kubernaut deployment/context-api | grep "RFC7807Error"

# Retry attempts
kubectl logs -n kubernaut deployment/context-api | grep "retrying after backoff"
```

### Profiling (Performance Investigation)

**Enable pprof**:
```yaml
# context-api-deployment.yaml
env:
  - name: ENABLE_PPROF
    value: "true"
```

**Collect heap profile**:
```bash
kubectl port-forward -n kubernaut deployment/context-api 6060:6060
curl http://localhost:6060/debug/pprof/heap > heap.prof
go tool pprof heap.prof
```

---

## 📈 **Capacity Planning**

### Scaling Guidelines

| Metric | Scale Up Trigger | Scale Down Trigger |
|--------|------------------|-------------------|
| **CPU** | > 70% sustained | < 30% sustained |
| **Memory** | > 80% sustained | < 40% sustained |
| **QPS** | > 800 per pod | < 200 per pod |
| **Latency p95** | > 500ms | < 100ms |

### Horizontal Scaling
```bash
# Scale up
kubectl scale -n kubernaut deployment/context-api --replicas=5

# Autoscaling (recommended)
kubectl autoscale -n kubernaut deployment/context-api --min=3 --max=10 --cpu-percent=70
```

### Resource Recommendations

| Deployment Size | Replicas | CPU Request | Memory Request | CPU Limit | Memory Limit |
|-----------------|----------|-------------|----------------|-----------|--------------|
| **Small** (< 100 QPS) | 2 | 100m | 256Mi | 500m | 512Mi |
| **Medium** (100-500 QPS) | 3-5 | 200m | 512Mi | 1000m | 1Gi |
| **Large** (> 500 QPS) | 5-10 | 500m | 1Gi | 2000m | 2Gi |

---

## 🚨 **Incident Response**

### Severity Levels

| Severity | Definition | Response Time | Example |
|----------|-----------|---------------|---------|
| **P0** | Service down, no fallback | < 15 min | All pods CrashLoopBackOff |
| **P1** | Degraded performance, fallback active | < 1 hour | Circuit breaker open |
| **P2** | Minor issues, no user impact | < 4 hours | Low cache hit rate |

### P0: Service Down

**Immediate Actions**:
1. Check pod status: `kubectl get pods -n kubernaut -l app=context-api`
2. Check logs: `kubectl logs -n kubernaut deployment/context-api --tail=100`
3. Check dependencies: Data Storage, Redis, PostgreSQL
4. Rollback if recent deploy: `kubectl rollout undo -n kubernaut deployment/context-api`

**Escalation**:
- If not resolved in 15 min → Page on-call engineer
- If Data Storage down → See Data Storage runbook

### P1: Degraded Performance

**Immediate Actions**:
1. Check circuit breaker status (see [Circuit Breaker](#circuit-breaker-scenarios))
2. Check cache hit rate (see [Cache Issues](#cache-issues))
3. Scale pods if needed: `kubectl scale -n kubernaut deployment/context-api --replicas=5`
4. Monitor for auto-recovery (circuit breaker closes in 60s)

**Escalation**:
- If not recovered in 1 hour → Investigate Data Storage health

---

## 📚 **Related Documentation**

- [Data Storage Operational Runbook](../data-storage/OPERATIONAL-RUNBOOK.md)
- [Context API Implementation Plan](implementation/IMPLEMENTATION_PLAN_V2.7.md)
- [Testing Strategy](../../../../rules/03-testing-strategy.mdc)
- [DD-007: Graceful Shutdown](../../../architecture/decisions/DD-007-graceful-shutdown.md)

---

## 📞 **Support Contacts**

| Component | Team | Slack Channel |
|-----------|------|---------------|
| **Context API** | Platform Team | #kubernaut-context-api |
| **Data Storage** | Platform Team | #kubernaut-data-storage |
| **Redis** | Infrastructure Team | #infrastructure |
| **On-Call** | Platform Team | Page via PagerDuty |

---

**Document Status**: ✅ Production-Ready  
**Confidence**: 90% - Covers 90%+ of production scenarios  
**Last Validated**: 2025-11-02

