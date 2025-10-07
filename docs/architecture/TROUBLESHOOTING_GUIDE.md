# Troubleshooting Guide - Kubernaut Services

**Version**: v1.0
**Last Updated**: October 6, 2025
**Status**: ✅ **COMPREHENSIVE**
**Scope**: All 11 Services + Infrastructure

---

## 📋 **Table of Contents**

1. [Common Issues](#common-issues)
2. [Service-Specific Issues](#service-specific-issues)
3. [Infrastructure Issues](#infrastructure-issues)
4. [Debugging Tools](#debugging-tools)
5. [Emergency Procedures](#emergency-procedures)

---

## 🚨 **Common Issues**

### **Issue 1: Service Not Ready**

**Symptoms**:
- Pod status: `Running` but not `Ready`
- Health check failures in logs

**Diagnosis**:
```bash
# Check pod readiness
kubectl get pods -n kubernaut-system

# View readiness probe logs
kubectl logs -n kubernaut-system {pod-name} --tail=50 | grep readyz

# Check dependency health
kubectl exec -n kubernaut-system {pod-name} -- curl localhost:8080/readyz | jq
```

**Common Causes**:
1. Database unavailable
2. Redis connection failed
3. Kubernetes API unreachable

**Resolution**:
```bash
# Check PostgreSQL
kubectl get pods -n kubernaut-system -l app=postgresql

# Check Redis
kubectl get pods -n kubernaut-system -l app=redis

# Check K8s API access
kubectl auth can-i get pods --as=system:serviceaccount:kubernaut-system:{sa-name}
```

---

### **Issue 2: High Error Rate (5xx)**

**Symptoms**:
- Prometheus alert: `HighErrorRate`
- Many 500/503 responses in logs

**Diagnosis**:
```bash
# Check error rate
kubectl logs -n kubernaut-system {pod-name} | grep '"code":"500"' | wc -l

# View error details
kubectl logs -n kubernaut-system {pod-name} --tail=100 | grep error
```

**Common Causes**:
1. Database connection pool exhausted
2. Circuit breaker open
3. External API timeout

**Resolution**:
```bash
# Scale up service
kubectl scale deployment/{service} --replicas=5 -n kubernaut-system

# Check database connections
kubectl exec -n kubernaut-system postgresql-0 -- psql -U kubernaut -c "SELECT count(*) FROM pg_stat_activity;"

# Check circuit breaker state
curl http://{service}:9090/metrics | grep circuit_breaker_state
```

---

### **Issue 3: Rate Limit Exceeded**

**Symptoms**:
- 429 Too Many Requests responses
- Clients reporting throttling

**Diagnosis**:
```bash
# Check rate limit metrics
curl http://{service}:9090/metrics | grep rate_limit_exceeded

# View rate limit logs
kubectl logs -n kubernaut-system {pod-name} | grep "Rate limit exceeded"
```

**Resolution**:
```bash
# Identify top clients
kubectl logs -n kubernaut-system {pod-name} | grep "Rate limit exceeded" | jq -r '.client' | sort | uniq -c | sort -rn

# Option 1: Scale service (increases global limit)
kubectl scale deployment/{service} --replicas=5 -n kubernaut-system

# Option 2: Adjust per-client limits (config change)
# Edit service ConfigMap to increase rate limits
```

---

### **Issue 4: Memory Leak**

**Symptoms**:
- Pod OOMKilled repeatedly
- Memory usage steadily increasing

**Diagnosis**:
```bash
# Check memory usage
kubectl top pod -n kubernaut-system {pod-name}

# View OOMKill events
kubectl get events -n kubernaut-system --field-selector reason=OOMKilling

# Check memory metrics
curl http://{service}:9090/metrics | grep process_resident_memory_bytes
```

**Resolution**:
```bash
# Increase memory limit (temporary)
kubectl set resources deployment/{service} --limits=memory=2Gi -n kubernaut-system

# Restart pods to clear leaked memory
kubectl rollout restart deployment/{service} -n kubernaut-system

# Investigate memory leak (long-term)
# - Check for goroutine leaks
# - Review connection pooling
# - Analyze heap dumps
```

---

## 🔧 **Service-Specific Issues**

### **Gateway Service**

#### **Issue: CRD Creation Failed**

**Symptoms**:
```
Failed to create RemediationRequest CRD: unauthorized
```

**Resolution**:
```bash
# Check RBAC permissions
kubectl auth can-i create remediationrequests.remediation.kubernaut.io --as=system:serviceaccount:kubernaut-system:gateway

# Verify ClusterRole
kubectl get clusterrole gateway-writer -o yaml

# Reapply RBAC if needed
kubectl apply -f deploy/gateway-rbac.yaml
```

---

#### **Issue: Redis Deduplication Failure**

**Symptoms**:
```
Deduplication failed: redis timeout
```

**Resolution**:
```bash
# Check Redis connectivity
kubectl exec -n kubernaut-system gateway-{pod} -- redis-cli -h redis ping

# Check Redis performance
kubectl exec -n kubernaut-system redis-0 -- redis-cli INFO stats | grep instantaneous_ops_per_sec

# Scale Redis if needed
kubectl scale statefulset/redis --replicas=3 -n kubernaut-system
```

---

### **Context API Service**

#### **Issue: Vector Search Timeout**

**Symptoms**:
```
Vector search timeout after 15s
```

**Resolution**:
```bash
# Check pgvector index status
kubectl exec -n kubernaut-system postgresql-0 -- psql -U kubernaut -c "SELECT * FROM pg_indexes WHERE tablename='incident_embeddings';"

# Tune HNSW ef_search parameter
# In application config:
SET hnsw.ef_search = 100;  # Increase for better recall

# Rebuild index if corrupted
kubectl exec -n kubernaut-system postgresql-0 -- psql -U kubernaut -c "REINDEX INDEX CONCURRENTLY idx_incident_embeddings_vector;"
```

---

#### **Issue: Database Connection Pool Exhausted**

**Symptoms**:
```
Database error: connection pool exhausted (100/100)
```

**Resolution**:
```bash
# Check active connections
kubectl exec -n kubernaut-system postgresql-0 -- psql -U kubernaut -c "SELECT count(*), state FROM pg_stat_activity GROUP BY state;"

# Increase connection limit (PostgreSQL)
kubectl exec -n kubernaut-system postgresql-0 -- psql -U postgres -c "ALTER SYSTEM SET max_connections = 200;"
kubectl rollout restart statefulset/postgresql -n kubernaut-system

# Increase pool size (application)
# Edit deployment environment:
- name: DB_MAX_CONNECTIONS
  value: "150"
```

---

### **HolmesGPT API Service**

#### **Issue: LLM Rate Limit Exceeded**

**Symptoms**:
```
Investigation rate limit exceeded (5/min)
```

**Resolution**:
```bash
# Check current rate
curl http://holmesgpt-api:9090/metrics | grep holmesgpt_rate_limit_exceeded_total

# Option 1: Increase rate limit (config)
# Edit ConfigMap to increase from 5/min to 10/min

# Option 2: Scale replicas (increases global capacity)
kubectl scale deployment/holmesgpt-api --replicas=4 -n kubernaut-system

# Option 3: Identify top consumers
kubectl logs -n kubernaut-system holmesgpt-api-{pod} | grep "Rate limit exceeded" | jq -r '.client' | sort | uniq -c | sort -rn
```

---

#### **Issue: Toolset Configuration Not Found**

**Symptoms**:
```
Toolset 'prometheus' not found in ConfigMap
```

**Resolution**:
```bash
# Check ConfigMap exists
kubectl get configmap kubernaut-toolset-config -n kubernaut-system

# View ConfigMap content
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o yaml

# Trigger Dynamic Toolset Service discovery
curl -X POST http://dynamic-toolset:8080/api/v1/discover

# Wait for ConfigMap sync (60s)
sleep 60

# Verify toolset appears
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o yaml | grep prometheus
```

---

### **Notification Service**

#### **Issue: Slack Webhook Timeout**

**Symptoms**:
```
Notification send failed: slack timeout after 10s
```

**Resolution**:
```bash
# Test Slack connectivity
kubectl exec -n kubernaut-system notification-{pod} -- curl -s -o /dev/null -w "%{http_code}" https://slack.com/api/api.test

# Check circuit breaker state
curl http://notification:9090/metrics | grep 'notification_circuit_breaker_state{breaker="slack"}'

# Manual circuit breaker reset (if stuck open)
# Restart service to reset circuit breaker
kubectl rollout restart deployment/notification -n kubernaut-system
```

---

## 🗄️ **Infrastructure Issues**

### **PostgreSQL Issues**

#### **High CPU Usage**

**Diagnosis**:
```bash
# Check slow queries
kubectl exec -n kubernaut-system postgresql-0 -- psql -U kubernaut -c "SELECT query, mean_exec_time, calls FROM pg_stat_statements ORDER BY mean_exec_time DESC LIMIT 10;"

# Check table bloat
kubectl exec -n kubernaut-system postgresql-0 -- psql -U kubernaut -c "SELECT schemaname, tablename, pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) FROM pg_tables WHERE schemaname='public' ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;"
```

**Resolution**:
```bash
# VACUUM ANALYZE
kubectl exec -n kubernaut-system postgresql-0 -- psql -U kubernaut -c "VACUUM ANALYZE;"

# Tune PostgreSQL
kubectl exec -n kubernaut-system postgresql-0 -- psql -U postgres -c "ALTER SYSTEM SET shared_buffers = '2GB';"
kubectl exec -n kubernaut-system postgresql-0 -- psql -U postgres -c "ALTER SYSTEM SET effective_cache_size = '6GB';"
kubectl rollout restart statefulset/postgresql -n kubernaut-system
```

---

### **Redis Issues**

#### **Memory Full**

**Diagnosis**:
```bash
# Check memory usage
kubectl exec -n kubernaut-system redis-0 -- redis-cli INFO memory | grep used_memory_human

# Check eviction policy
kubectl exec -n kubernaut-system redis-0 -- redis-cli CONFIG GET maxmemory-policy
```

**Resolution**:
```bash
# Set eviction policy (LRU)
kubectl exec -n kubernaut-system redis-0 -- redis-cli CONFIG SET maxmemory-policy allkeys-lru

# Increase memory limit
kubectl set resources statefulset/redis --limits=memory=4Gi -n kubernaut-system

# Flush expired keys
kubectl exec -n kubernaut-system redis-0 -- redis-cli --scan --pattern "*" | xargs redis-cli DEL
```

---

## 🛠️ **Debugging Tools**

### **1. Log Analysis**

```bash
# Search for errors across all services
kubectl logs -n kubernaut-system -l app.kubernetes.io/part-of=kubernaut --tail=1000 | grep -i error

# Follow logs with correlation ID
kubectl logs -n kubernaut-system -l app=gateway -f | grep "req-20251006101530-abc123"

# Export logs for analysis
kubectl logs -n kubernaut-system gateway-{pod} --since=1h > gateway-logs.txt
```

---

### **2. Metrics Investigation**

```bash
# Query Prometheus directly
kubectl port-forward -n monitoring svc/prometheus 9090:9090

# Open http://localhost:9090 and run queries:
rate(gateway_http_requests_total{code=~"5.."}[5m])
histogram_quantile(0.95, rate(contextapi_http_request_duration_seconds_bucket[5m]))
```

---

### **3. Network Debugging**

```bash
# Test connectivity between services
kubectl exec -n kubernaut-system gateway-{pod} -- curl -s http://context-api:8080/healthz

# Check DNS resolution
kubectl exec -n kubernaut-system gateway-{pod} -- nslookup postgresql.kubernaut-system.svc.cluster.local

# Trace network path
kubectl exec -n kubernaut-system gateway-{pod} -- traceroute postgresql.kubernaut-system
```

---

## 🚑 **Emergency Procedures**

### **1. Service Down - Emergency Recovery**

```bash
# Step 1: Identify failing service
kubectl get pods -n kubernaut-system | grep -v Running

# Step 2: Restart pods
kubectl rollout restart deployment/{service} -n kubernaut-system

# Step 3: Scale to zero and back (if restart fails)
kubectl scale deployment/{service} --replicas=0 -n kubernaut-system
sleep 10
kubectl scale deployment/{service} --replicas=2 -n kubernaut-system

# Step 4: Check logs
kubectl logs -n kubernaut-system {pod-name} --tail=100
```

---

### **2. Database Corruption**

```bash
# Step 1: Stop all services writing to database
kubectl scale deployment/data-storage --replicas=0 -n kubernaut-system

# Step 2: Backup database
kubectl exec -n kubernaut-system postgresql-0 -- pg_dump -U kubernaut > backup-$(date +%Y%m%d%H%M%S).sql

# Step 3: Check database integrity
kubectl exec -n kubernaut-system postgresql-0 -- psql -U kubernaut -c "SELECT * FROM pg_database;"

# Step 4: Restore from backup (if needed)
kubectl exec -n kubernaut-system postgresql-0 -- psql -U kubernaut < backup-20251006120000.sql

# Step 5: Restart services
kubectl scale deployment/data-storage --replicas=2 -n kubernaut-system
```

---

### **3. Complete System Outage**

```bash
# Step 1: Check Kubernetes cluster health
kubectl get nodes
kubectl get componentstatuses

# Step 2: Check critical pods
kubectl get pods -n kube-system

# Step 3: Restart all Kubernaut services in order
kubectl rollout restart statefulset/postgresql -n kubernaut-system
kubectl rollout restart statefulset/redis -n kubernaut-system
sleep 30
kubectl rollout restart deployment/dynamic-toolset -n kubernaut-system
kubectl rollout restart deployment/data-storage -n kubernaut-system
kubectl rollout restart deployment/context-api -n kubernaut-system
kubectl rollout restart deployment/holmesgpt-api -n kubernaut-system
kubectl rollout restart deployment/gateway -n kubernaut-system
kubectl rollout restart deployment/notification -n kubernaut-system

# Step 4: Verify all services healthy
kubectl get pods -n kubernaut-system
kubectl rollout status deployment/gateway -n kubernaut-system
```

---

## 📚 **Related Documentation**

- [ERROR_RESPONSE_STANDARD.md](./ERROR_RESPONSE_STANDARD.md) - Error code reference
- [HEALTH_CHECK_STANDARD.md](./HEALTH_CHECK_STANDARD.md) - Health check details
- [OPERATIONAL_STANDARDS.md](./OPERATIONAL_STANDARDS.md) - Timeouts, circuit breakers
- [LOG_CORRELATION_ID_STANDARD.md](./LOG_CORRELATION_ID_STANDARD.md) - Log tracing

---

**Document Status**: ✅ Complete
**Last Updated**: October 6, 2025
**Version**: 1.0
