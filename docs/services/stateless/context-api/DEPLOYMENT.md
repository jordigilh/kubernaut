# Context API - Deployment Guide

**Version**: 1.0
**Last Updated**: October 20, 2025
**Service**: Context API (kubernaut-system)

---

## 📋 **Overview**

This guide provides step-by-step instructions for deploying the Context API service to a Kubernetes cluster. The Context API provides historical context and incident data for HolmesGPT AI analysis.

**Deployment Target**: `kubernaut-system` namespace

---

## 🎯 **Prerequisites**

### **Required Infrastructure**

| Component | Version | Purpose | Status Check |
|-----------|---------|---------|--------------|
| **Kubernetes** | 1.24+ | Container orchestration | `kubectl version` |
| **PostgreSQL** | 14+ with pgvector | Data storage | `kubectl get pods -n kubernaut-system -l app=postgres` |
| **Redis** | 6.0+ | L1 cache | `kubectl get pods -n kubernaut-system -l app=redis` |
| **kubectl** | 1.24+ | Kubernetes CLI | `kubectl version --client` |

### **PostgreSQL Requirements**

**Database**: `action_history`
**Extension**: `pgvector` (for vector embeddings)
**Schema**: `remediation_audit` table (managed by Data Storage Service)

**Verify PostgreSQL**:
```bash
kubectl exec -n kubernaut-system postgres-0 -- psql -U slm_user -d action_history -c "SELECT extname FROM pg_extension WHERE extname = 'vector';"
```

Expected output: `vector`

### **Redis Requirements**

**Configuration**: Standard Redis 6.0+
**Persistence**: Optional (cache can rebuild)
**Database**: DB 0 (default)

**Verify Redis**:
```bash
kubectl exec -n kubernaut-system redis-0 -- redis-cli PING
```

Expected output: `PONG`

---

## 📦 **Installation**

### **Step 1: Verify Namespace**

```bash
# Check if kubernaut-system namespace exists
kubectl get namespace kubernaut-system

# If not exists, create it
kubectl create namespace kubernaut-system
```

---

### **Step 2: Create Database Secret**

Create secret for PostgreSQL credentials:

```bash
kubectl create secret generic context-api-db-secret \
  --from-literal=password='<your-database-password>' \
  -n kubernaut-system \
  --dry-run=client -o yaml | kubectl apply -f -
```

**Verify secret**:
```bash
kubectl get secret context-api-db-secret -n kubernaut-system
```

---

### **Step 3: Review Configuration**

**Deployment manifest location**: `deploy/context-api-deployment.yaml`

**Key configuration items to review**:

```yaml
# Database connection
- name: DB_HOST
  value: "postgres.kubernaut-system.svc.cluster.local"
- name: DB_PORT
  value: "5432"
- name: DB_NAME
  value: "action_history"
- name: DB_USER
  value: "slm_user"

# Redis connection
- name: REDIS_ADDR
  value: "redis.kubernaut-system.svc.cluster.local:6379"

# Performance tuning
resources:
  requests:
    memory: "256Mi"
    cpu: "100m"
  limits:
    memory: "512Mi"
    cpu: "500m"
```

**Customize if needed**:
- Adjust resource limits based on expected load
- Update database credentials if different
- Modify replica count (default: 2 for HA)

---

### **Step 4: Apply Deployment**

```bash
# Apply the deployment manifest
kubectl apply -f deploy/context-api-deployment.yaml

# Verify deployment
kubectl get deployment context-api -n kubernaut-system
```

**Expected output**:
```
NAME          READY   UP-TO-DATE   AVAILABLE   AGE
context-api   2/2     2            2           30s
```

---

### **Step 5: Verify Pods**

```bash
# Check pod status
kubectl get pods -n kubernaut-system -l app=context-api

# Watch pods come up
kubectl get pods -n kubernaut-system -l app=context-api -w
```

**Expected output**:
```
NAME                          READY   STATUS    RESTARTS   AGE
context-api-7d4b5c8f9-abc12   1/1     Running   0          1m
context-api-7d4b5c8f9-def34   1/1     Running   0          1m
```

**If pods not ready, check logs**:
```bash
kubectl logs -n kubernaut-system -l app=context-api --tail=50
```

---

### **Step 6: Verify Service**

```bash
# Check service
kubectl get svc context-api -n kubernaut-system

# Test service connectivity
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -n kubernaut-system -- curl http://context-api.kubernaut-system.svc.cluster.local:8091/health
```

**Expected response**:
```json
{
  "status": "healthy"
}
```

---

### **Step 7: Verify Health Checks**

```bash
# Test liveness probe
kubectl exec -n kubernaut-system <pod-name> -- curl localhost:8091/health

# Test readiness probe
kubectl exec -n kubernaut-system <pod-name> -- curl localhost:8091/health/ready
```

**Expected readiness response**:
```json
{
  "status": "ready",
  "cache": "ready",
  "database": "ready"
}
```

**Note**: If cache shows "degraded", verify Redis connectivity.

---

### **Step 8: Verify API Endpoints**

```bash
# Test query endpoint (from within cluster)
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -n kubernaut-system -- \
  curl "http://context-api.kubernaut-system.svc.cluster.local:8091/api/v1/context/query?limit=5"
```

**Expected response** (if data exists):
```json
{
  "incidents": [...],
  "total": 10,
  "limit": 5,
  "offset": 0
}
```

---

### **Step 9: Verify Metrics**

```bash
# Check metrics endpoint
kubectl exec -n kubernaut-system <pod-name> -- curl localhost:8091/metrics | head -20
```

**Expected output** (Prometheus format):
```
# HELP contextapi_queries_total Total number of API queries
# TYPE contextapi_queries_total counter
contextapi_queries_total{type="list",status="success"} 0
...
```

---

### **Step 10: Configure Prometheus (Optional)**

If using Prometheus for monitoring:

```yaml
# prometheus-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-config
  namespace: monitoring
data:
  prometheus.yml: |
    scrape_configs:
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
          - source_labels: [__meta_kubernetes_pod_name]
            target_label: pod
          - source_labels: [__meta_kubernetes_namespace]
            target_label: namespace
```

Apply and reload Prometheus configuration.

---

## ⚙️ **Configuration**

### **Environment Variables**

**Database Configuration**:
- `DB_HOST`: PostgreSQL hostname (default: `postgres.kubernaut-system.svc.cluster.local`)
- `DB_PORT`: PostgreSQL port (default: `5432`)
- `DB_NAME`: Database name (default: `action_history`)
- `DB_USER`: Database user (default: `slm_user`)
- `DB_PASSWORD`: Database password (from secret: `context-api-db-secret`)
- `DB_SSLMODE`: SSL mode (default: `disable`)

**Redis Configuration**:
- `REDIS_ADDR`: Redis address (default: `redis.kubernaut-system.svc.cluster.local:6379`)
- `REDIS_DB`: Redis database number (default: `0`)

**Service Configuration**:
- `LOG_LEVEL`: Logging level (default: `info`, options: `debug|info|warn|error`)
- `CACHE_TTL_DEFAULT`: Default cache TTL (default: `5m`)
- `CACHE_TTL_MAX`: Maximum cache TTL (default: `10m`)

**Prometheus Configuration**:
- `PROMETHEUS_ENDPOINT`: Prometheus server endpoint (optional)

### **ConfigMap (Optional)**

For advanced configuration, create a ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: context-api-config
  namespace: kubernaut-system
data:
  config.yaml: |
    server:
      port: 8091
      host: "0.0.0.0"
      read_timeout: "30s"
      write_timeout: "30s"

    logging:
      level: "info"
      format: "json"

    cache:
      type: "memory"
      ttl: "10m"
      max_size: 1000
      cleanup_interval: "5m"

    performance:
      max_concurrent_requests: 50
      request_timeout: "60s"
      graceful_shutdown_timeout: "10s"
```

Mount as volume in deployment:
```yaml
volumeMounts:
  - name: config
    mountPath: /etc/context-api
volumes:
  - name: config
    configMap:
      name: context-api-config
```

---

## 🔧 **Post-Deployment Validation**

### **Validation Checklist**

| Check | Command | Expected Result |
|-------|---------|----------------|
| **Pods Running** | `kubectl get pods -n kubernaut-system -l app=context-api` | All pods READY 1/1 |
| **Health OK** | `curl http://context-api:8091/health` | `{"status":"healthy"}` |
| **Readiness OK** | `curl http://context-api:8091/health/ready` | `{"status":"ready",...}` |
| **Database Connection** | Check readiness `"database":"ready"` | Ready status |
| **Redis Connection** | Check readiness `"cache":"ready"` | Ready or degraded OK |
| **Metrics Exposed** | `curl http://context-api:8091/metrics` | Prometheus format |
| **API Responsive** | `curl http://context-api:8091/api/v1/context/query` | JSON response |

### **Validation Script**

```bash
#!/bin/bash
# validate-deployment.sh

NAMESPACE="kubernaut-system"
SERVICE="context-api"

echo "🔍 Validating Context API deployment..."

# Check pods
echo "1. Checking pods..."
READY=$(kubectl get pods -n $NAMESPACE -l app=$SERVICE -o jsonpath='{.items[*].status.containerStatuses[*].ready}')
if [[ "$READY" == *"false"* ]]; then
    echo "❌ Some pods not ready"
    exit 1
fi
echo "✅ All pods ready"

# Check health
echo "2. Checking health endpoint..."
HEALTH=$(kubectl exec -n $NAMESPACE $(kubectl get pods -n $NAMESPACE -l app=$SERVICE -o jsonpath='{.items[0].metadata.name}') -- curl -s localhost:8091/health)
if [[ "$HEALTH" == *"healthy"* ]]; then
    echo "✅ Health check passed"
else
    echo "❌ Health check failed"
    exit 1
fi

# Check readiness
echo "3. Checking readiness endpoint..."
READY=$(kubectl exec -n $NAMESPACE $(kubectl get pods -n $NAMESPACE -l app=$SERVICE -o jsonpath='{.items[0].metadata.name}') -- curl -s localhost:8091/health/ready)
if [[ "$READY" == *"ready"* ]] || [[ "$READY" == *"degraded"* ]]; then
    echo "✅ Readiness check passed"
else
    echo "❌ Readiness check failed"
    exit 1
fi

# Check metrics
echo "4. Checking metrics endpoint..."
METRICS=$(kubectl exec -n $NAMESPACE $(kubectl get pods -n $NAMESPACE -l app=$SERVICE -o jsonpath='{.items[0].metadata.name}') -- curl -s localhost:8091/metrics | head -1)
if [[ "$METRICS" == *"# HELP"* ]]; then
    echo "✅ Metrics endpoint working"
else
    echo "❌ Metrics endpoint failed"
    exit 1
fi

echo ""
echo "🎉 All validation checks passed!"
echo ""
echo "Service URL: http://context-api.kubernaut-system.svc.cluster.local:8091"
echo "Health: http://context-api.kubernaut-system.svc.cluster.local:8091/health"
echo "Metrics: http://context-api.kubernaut-system.svc.cluster.local:8091/metrics"
```

Run validation:
```bash
chmod +x validate-deployment.sh
./validate-deployment.sh
```

---

## 🔄 **Updates & Rollouts**

### **Rolling Update**

```bash
# Update deployment (e.g., new image version)
kubectl set image deployment/context-api \
  context-api=kubernaut/context-api:v1.1.0 \
  -n kubernaut-system

# Watch rollout
kubectl rollout status deployment/context-api -n kubernaut-system

# Verify new version
kubectl get pods -n kubernaut-system -l app=context-api -o jsonpath='{.items[*].spec.containers[*].image}'
```

### **Rollback**

```bash
# View rollout history
kubectl rollout history deployment/context-api -n kubernaut-system

# Rollback to previous version
kubectl rollout undo deployment/context-api -n kubernaut-system

# Verify rollback
kubectl rollout status deployment/context-api -n kubernaut-system
```

---

## 📊 **Scaling**

### **Horizontal Scaling**

```bash
# Scale to 3 replicas
kubectl scale deployment context-api -n kubernaut-system --replicas=3

# Verify scaling
kubectl get pods -n kubernaut-system -l app=context-api
```

### **Horizontal Pod Autoscaler (HPA)**

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: context-api-hpa
  namespace: kubernaut-system
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: context-api
  minReplicas: 2
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80
```

Apply HPA:
```bash
kubectl apply -f context-api-hpa.yaml

# Monitor HPA
kubectl get hpa context-api-hpa -n kubernaut-system -w
```

---

## 🗑️ **Uninstall**

### **Remove Deployment**

```bash
# Delete all Context API resources
kubectl delete -f deploy/context-api-deployment.yaml

# Verify removal
kubectl get all -n kubernaut-system -l app=context-api
```

### **Remove Secret**

```bash
# Delete database secret
kubectl delete secret context-api-db-secret -n kubernaut-system
```

### **Cleanup ConfigMap** (if created)

```bash
kubectl delete configmap context-api-config -n kubernaut-system
```

---

## 🐛 **Troubleshooting Deployment**

### **Pods Not Starting**

**Check pod events**:
```bash
kubectl describe pod <pod-name> -n kubernaut-system
```

**Common issues**:
1. **ImagePullBackOff**: Image not available
   ```bash
   # Check image name
   kubectl get deployment context-api -n kubernaut-system -o jsonpath='{.spec.template.spec.containers[0].image}'
   ```

2. **CrashLoopBackOff**: Application failing to start
   ```bash
   # Check logs
   kubectl logs <pod-name> -n kubernaut-system --previous
   ```

3. **Pending**: Insufficient resources
   ```bash
   # Check node resources
   kubectl describe nodes | grep -A 5 "Allocated resources"
   ```

### **Database Connection Issues**

**Verify database connectivity**:
```bash
# Test from Context API pod
kubectl exec -n kubernaut-system <pod-name> -- \
  psql -h postgres.kubernaut-system.svc.cluster.local -U slm_user -d action_history -c "SELECT 1;"
```

**Check database logs**:
```bash
kubectl logs -n kubernaut-system postgres-0 --tail=100
```

### **Redis Connection Issues**

**Test Redis connectivity**:
```bash
# From Context API pod
kubectl exec -n kubernaut-system <pod-name> -- \
  redis-cli -h redis.kubernaut-system.svc.cluster.local PING
```

**Note**: Service will function with degraded performance if Redis is unavailable (LRU fallback).

---

## 📚 **Additional Resources**

- **Operations Guide**: [OPERATIONS.md](./OPERATIONS.md)
- **API Documentation**: [api-specification.md](./api-specification.md)
- **Implementation Plan**: [implementation/IMPLEMENTATION_PLAN_V2.6.md](./implementation/IMPLEMENTATION_PLAN_V2.6.md)
- **Kubernetes Documentation**: https://kubernetes.io/docs/

---

## 🔐 **Security Considerations**

1. **Secret Management**: Use Kubernetes secrets or external secret managers (Vault, etc.)
2. **RBAC**: Service account has minimal required permissions
3. **Network Policies**: Consider implementing network policies to restrict traffic
4. **TLS**: For production, enable TLS for PostgreSQL and Redis connections
5. **Image Security**: Scan container images for vulnerabilities before deployment

---

**Last Updated**: October 20, 2025
**Version**: 1.0
**Maintained By**: Platform Team

