# Context API - Operations Guide

**Version**: 1.0
**Last Updated**: October 20, 2025
**Service**: Context API (kubernaut-system)

---

## 📋 **Overview**

The Context API provides historical context and incident data for HolmesGPT AI analysis. This guide covers operational procedures, health monitoring, troubleshooting, and incident response.

**Service Details**:
- **Namespace**: `kubernaut-system`
- **Deployment**: `context-api`
- **Service Port**: 8091 (HTTP)
- **Metrics Port**: 9090 (Prometheus)

---

## 🏥 **Health Checks**

### **Endpoints**

| Endpoint | Purpose | Expected Response | Probe Type |
|----------|---------|-------------------|------------|
| `/health` | Basic health | `200 OK` | Liveness |
| `/health/ready` | Readiness check | `200 OK` with JSON | Readiness |
| `/metrics` | Prometheus metrics | `200 OK` | Monitoring |

### **Health Check Interpretation**

#### **1. Liveness Probe (`/health`)**

**Purpose**: Verify service is alive and responding

**Response**:
```json
{
  "status": "healthy"
}
```

**Interpretation**:
- ✅ **200 OK**: Service is running
- ❌ **Non-200**: Service is unresponsive (Kubernetes will restart pod)

**Action on Failure**:
```bash
# Check pod status
kubectl get pods -n kubernaut-system -l app=context-api

# Check pod events
kubectl describe pod <pod-name> -n kubernaut-system

# Check logs
kubectl logs <pod-name> -n kubernaut-system --tail=100
```

---

#### **2. Readiness Probe (`/health/ready`)**

**Purpose**: Verify service can accept traffic

**Response**:
```json
{
  "status": "ready",
  "cache": "ready",
  "database": "ready"
}
```

**Interpretation**:
- ✅ **200 OK + all "ready"**: Service ready for traffic
- ⚠️ **200 OK + "cache": "degraded"**: Redis unavailable (using LRU fallback)
- ❌ **500**: Database unavailable (service cannot function)

**Degraded State (Cache)**:
```json
{
  "status": "degraded",
  "cache": "degraded",
  "database": "ready",
  "message": "Redis unavailable (using LRU L2 only)"
}
```

**Action**: Check Redis connectivity
```bash
# Verify Redis pod
kubectl get pods -n kubernaut-system -l app=redis

# Test Redis connection
kubectl exec -n kubernaut-system <context-api-pod> -- redis-cli -h redis.kubernaut-system.svc.cluster.local ping
```

**Database Failure**:
```json
{
  "status": "unhealthy",
  "cache": "ready",
  "database": "unavailable",
  "error": "failed to connect to PostgreSQL"
}
```

**Action**: Check PostgreSQL connectivity
```bash
# Verify PostgreSQL pod
kubectl get pods -n kubernaut-system -l app=postgres

# Check database service
kubectl get svc postgres -n kubernaut-system
```

---

## 📊 **Metrics Guide**

### **Accessing Metrics**

**Endpoint**: `http://context-api.kubernaut-system.svc.cluster.local:8091/metrics`

**Prometheus Scrape Configuration**:
```yaml
- job_name: 'context-api'
  kubernetes_sd_configs:
    - role: pod
      namespaces:
        names:
          - kubernaut-system
  relabel_configs:
    - source_labels: [__meta_kubernetes_pod_label_app]
      regex: context-api
      action: keep
```

---

### **Key Metrics**

#### **Query Metrics**

| Metric | Type | Description | Normal Range |
|--------|------|-------------|--------------|
| `contextapi_queries_total` | Counter | Total API queries by type/status | Monotonic increase |
| `contextapi_query_duration_seconds` | Histogram | Query latency distribution | p95 < 200ms |

**Query by Type**:
```promql
# Query rate by type
rate(contextapi_queries_total{type="list"}[5m])

# Failed queries
contextapi_queries_total{status="error"}
```

**Query Latency**:
```promql
# p95 latency
histogram_quantile(0.95, rate(contextapi_query_duration_seconds_bucket[5m]))

# p99 latency
histogram_quantile(0.99, rate(contextapi_query_duration_seconds_bucket[5m]))
```

**Alert Thresholds**:
- ⚠️ **Warning**: p95 > 200ms
- 🚨 **Critical**: p95 > 500ms OR error rate > 5%

---

#### **Cache Metrics**

| Metric | Type | Description | Target |
|--------|------|-------------|--------|
| `contextapi_cache_hits_total{tier="redis"}` | Counter | Redis L1 cache hits | >50% of requests |
| `contextapi_cache_hits_total{tier="lru"}` | Counter | LRU L2 cache hits | 10-30% of requests |
| `contextapi_cache_misses_total` | Counter | Cache misses (DB queries) | <30% of requests |

**Cache Hit Rate**:
```promql
# Overall cache hit rate (L1 + L2)
sum(rate(contextapi_cache_hits_total[5m])) /
(sum(rate(contextapi_cache_hits_total[5m])) + rate(contextapi_cache_misses_total[5m]))
```

**Expected**:
- ✅ **Healthy**: Cache hit rate > 70%
- ⚠️ **Warning**: Cache hit rate 50-70%
- 🚨 **Critical**: Cache hit rate < 50% (check Redis)

---

#### **HTTP Metrics**

| Metric | Type | Description | Target |
|--------|------|-------------|--------|
| `contextapi_http_requests_total` | Counter | HTTP requests by endpoint/status | Monitor 4xx/5xx |
| `contextapi_http_request_duration_seconds` | Histogram | HTTP request latency | p95 < 200ms |

**HTTP Errors**:
```promql
# 5xx error rate
rate(contextapi_http_requests_total{status=~"5.."}[5m])

# 4xx client errors
rate(contextapi_http_requests_total{status=~"4.."}[5m])
```

**Alert Thresholds**:
- ⚠️ **Warning**: 5xx rate > 1%
- 🚨 **Critical**: 5xx rate > 5%

---

## 🔍 **Troubleshooting**

### **Common Issues**

#### **Issue 1: High Query Latency**

**Symptoms**:
- p95 latency > 200ms
- Slow API responses

**Diagnosis**:
```bash
# Check cache hit rate
kubectl exec -n kubernaut-system <pod> -- curl localhost:8091/metrics | grep cache_hits

# Check database connections
kubectl exec -n kubernaut-system <pod> -- psql -h postgres.kubernaut-system.svc.cluster.local -U slm_user -d action_history -c "SELECT count(*) FROM pg_stat_activity WHERE state = 'active';"
```

**Possible Causes**:
1. **Low cache hit rate** → Check Redis availability
2. **Database slow** → Check PostgreSQL performance
3. **Large result sets** → Review query parameters (limit/offset)

**Resolution**:
```bash
# Scale up replicas if needed
kubectl scale deployment context-api -n kubernaut-system --replicas=3

# Check Redis memory
kubectl exec -n kubernaut-system redis-0 -- redis-cli INFO memory

# Check PostgreSQL query performance
kubectl exec -n kubernaut-system postgres-0 -- psql -U slm_user -d action_history -c "SELECT query, calls, mean_exec_time FROM pg_stat_statements ORDER BY mean_exec_time DESC LIMIT 10;"
```

---

#### **Issue 2: Cache Degradation**

**Symptoms**:
- Readiness probe shows "degraded"
- High cache miss rate
- All requests hitting database

**Diagnosis**:
```bash
# Check Redis pod status
kubectl get pods -n kubernaut-system -l app=redis

# Test Redis connectivity from Context API pod
kubectl exec -n kubernaut-system <context-api-pod> -- redis-cli -h redis.kubernaut-system.svc.cluster.local PING

# Check Redis logs
kubectl logs -n kubernaut-system redis-0 --tail=100
```

**Resolution**:
```bash
# Restart Redis if needed
kubectl rollout restart statefulset redis -n kubernaut-system

# Verify Redis is ready
kubectl wait --for=condition=ready pod -l app=redis -n kubernaut-system --timeout=60s

# Verify Context API reconnects
kubectl logs -n kubernaut-system <context-api-pod> --tail=50 | grep "cache"
```

---

#### **Issue 3: Database Connection Failures**

**Symptoms**:
- Readiness probe returns 500
- Errors: "failed to connect to PostgreSQL"
- All queries failing

**Diagnosis**:
```bash
# Check PostgreSQL pod status
kubectl get pods -n kubernaut-system -l app=postgres

# Check PostgreSQL service
kubectl get svc postgres -n kubernaut-system

# Test database connectivity
kubectl exec -n kubernaut-system <context-api-pod> -- psql -h postgres.kubernaut-system.svc.cluster.local -U slm_user -d action_history -c "SELECT 1;"
```

**Resolution**:
```bash
# Check PostgreSQL logs
kubectl logs -n kubernaut-system postgres-0 --tail=100

# Verify database credentials secret
kubectl get secret context-api-db-secret -n kubernaut-system -o jsonpath='{.data.password}' | base64 -d

# Restart Context API pods to reconnect
kubectl rollout restart deployment context-api -n kubernaut-system
```

---

#### **Issue 4: Out of Memory (OOM)**

**Symptoms**:
- Pods restarting frequently
- OOMKilled in pod status
- High memory usage in metrics

**Diagnosis**:
```bash
# Check pod resource usage
kubectl top pods -n kubernaut-system -l app=context-api

# Check pod events for OOM
kubectl get events -n kubernaut-system --field-selector involvedObject.name=<pod-name> | grep OOM
```

**Resolution**:
```bash
# Increase memory limits in deployment
kubectl edit deployment context-api -n kubernaut-system
# Update:
# resources:
#   limits:
#     memory: "1Gi"  # Increase from 512Mi

# Or patch directly
kubectl patch deployment context-api -n kubernaut-system -p '{"spec":{"template":{"spec":{"containers":[{"name":"context-api","resources":{"limits":{"memory":"1Gi"}}}]}}}}'

# Monitor after change
kubectl logs -n kubernaut-system -l app=context-api --tail=100 -f
```

---

## 🚨 **Incident Response**

### **Severity Levels**

| Severity | Definition | Response Time | Escalation |
|----------|------------|---------------|------------|
| **P1 - Critical** | Service down, all requests failing | 15 minutes | Immediate |
| **P2 - High** | Degraded (cache down, high latency) | 1 hour | If not resolved in 2h |
| **P3 - Medium** | Intermittent issues, some failures | 4 hours | If not resolved in 8h |
| **P4 - Low** | Performance degradation, non-critical | 1 business day | As needed |

---

### **Incident Response Procedures**

#### **P1: Service Down**

**Indicators**:
- All health checks failing
- 100% request failure rate
- No response from service

**Immediate Actions**:
1. **Verify pod status**:
   ```bash
   kubectl get pods -n kubernaut-system -l app=context-api
   ```

2. **Check recent events**:
   ```bash
   kubectl get events -n kubernaut-system --sort-by='.lastTimestamp' | grep context-api | tail -20
   ```

3. **Review logs**:
   ```bash
   kubectl logs -n kubernaut-system -l app=context-api --tail=200 --all-containers=true
   ```

4. **Check dependencies**:
   ```bash
   # PostgreSQL
   kubectl get pods -n kubernaut-system -l app=postgres

   # Redis
   kubectl get pods -n kubernaut-system -l app=redis
   ```

5. **Emergency restart**:
   ```bash
   kubectl rollout restart deployment context-api -n kubernaut-system
   kubectl rollout status deployment context-api -n kubernaut-system
   ```

**Escalation**: If not resolved in 15 minutes, escalate to platform team

---

#### **P2: Degraded Performance**

**Indicators**:
- Readiness shows "degraded"
- High latency (p95 > 500ms)
- Cache hit rate < 50%

**Response**:
1. **Assess impact**:
   ```bash
   # Check error rate
   kubectl exec -n kubernaut-system <pod> -- curl localhost:8091/metrics | grep http_requests_total
   ```

2. **Identify root cause**:
   - Redis down? → Follow "Issue 2: Cache Degradation"
   - Database slow? → Follow "Issue 1: High Query Latency"

3. **Implement mitigation**:
   ```bash
   # Scale up if needed
   kubectl scale deployment context-api -n kubernaut-system --replicas=3
   ```

4. **Monitor recovery**:
   ```bash
   kubectl logs -n kubernaut-system -l app=context-api --tail=50 -f | grep -E "(error|warn|degraded)"
   ```

---

### **Rollback Procedure**

If a deployment causes issues:

```bash
# View deployment history
kubectl rollout history deployment context-api -n kubernaut-system

# Rollback to previous version
kubectl rollout undo deployment context-api -n kubernaut-system

# Rollback to specific revision
kubectl rollout undo deployment context-api -n kubernaut-system --to-revision=<revision>

# Verify rollback
kubectl rollout status deployment context-api -n kubernaut-system

# Check health
kubectl exec -n kubernaut-system <new-pod> -- curl localhost:8091/health
```

---

## 📞 **Escalation Paths**

| Issue Type | Primary Contact | Secondary Contact | Notes |
|------------|----------------|-------------------|-------|
| Service Down | Platform Team | SRE On-Call | Immediate escalation for P1 |
| Database Issues | Database Team | Platform Team | PostgreSQL expertise needed |
| Performance | Platform Team | Development Team | May require code changes |
| Security | Security Team | Platform Team | Immediate for security incidents |

---

## 🔐 **Security Operations**

### **Access Control**

**RBAC Permissions**:
- ClusterRole: `context-api`
- ServiceAccount: `context-api`
- Namespace: `kubernaut-system`

**Verify RBAC**:
```bash
# Check ClusterRole
kubectl get clusterrole context-api -o yaml

# Check ServiceAccount
kubectl get serviceaccount context-api -n kubernaut-system

# Test permissions
kubectl auth can-i get pods --as=system:serviceaccount:kubernaut-system:context-api -n kubernaut-system
```

### **Secret Management**

**Database Credentials**:
```bash
# View secret (without revealing password)
kubectl describe secret context-api-db-secret -n kubernaut-system

# Rotate password (if needed)
kubectl create secret generic context-api-db-secret \
  --from-literal=password='<new-password>' \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart pods to pick up new secret
kubectl rollout restart deployment context-api -n kubernaut-system
```

---

## 📋 **Maintenance Procedures**

### **Routine Maintenance**

**Weekly**:
- Review error rates and performance trends
- Check resource utilization trends
- Verify backup procedures (PostgreSQL)

**Monthly**:
- Review and update resource limits if needed
- Audit RBAC permissions
- Review and archive old logs

**Quarterly**:
- Conduct disaster recovery drill
- Review and update runbooks
- Performance capacity planning

---

## 🎯 **SLOs & Monitoring**

### **Service Level Objectives**

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Availability** | 99.9% | Uptime over 30 days |
| **Latency (p95)** | < 200ms | Query response time |
| **Latency (p99)** | < 500ms | Query response time |
| **Error Rate** | < 1% | Failed requests / total requests |
| **Cache Hit Rate** | > 70% | (L1 + L2) hits / total requests |

### **Monitoring Dashboard**

**Recommended Grafana Panels**:
1. Request rate (requests/second)
2. Latency percentiles (p50, p95, p99)
3. Error rate by status code
4. Cache hit rate (L1 + L2)
5. Pod count and resource usage
6. Database connection pool metrics

---

## 📚 **Additional Resources**

- **API Documentation**: [api-specification.md](./api-specification.md)
- **Deployment Guide**: [DEPLOYMENT.md](./DEPLOYMENT.md)
- **Architecture**: [implementation/IMPLEMENTATION_PLAN_V2.6.md](./implementation/IMPLEMENTATION_PLAN_V2.6.md)
- **Troubleshooting**: This document (OPERATIONS.md)

---

**Last Updated**: October 20, 2025
**Version**: 1.0
**Maintained By**: Platform Team




