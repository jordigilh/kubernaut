# Data Storage Service - Kubernetes Deployment

**Version**: 1.0
**Date**: October 13, 2025
**Status**: ✅ Production Ready

---

## Quick Deploy

```bash
# Deploy everything
kubectl apply -k deploy/data-storage/

# Verify deployment
kubectl get pods -n kubernaut -l app=data-storage-service

# Check logs
kubectl logs -n kubernaut -l app=data-storage-service --tail=100 -f

# Test health
kubectl port-forward -n kubernaut svc/data-storage-service 8080:8080
curl http://localhost:8080/health
```

---

## Prerequisites

### Required

1. **Kubernetes 1.23+**
2. **PostgreSQL 16+ with pgvector 0.5.1+**
   - Deploy separately or use existing
   - Ensure `shared_buffers >= 1GB`
3. **Kustomize 4.0+** (or `kubectl apply -k`)

### Optional

4. **Vector DB** (for semantic search)
5. **Redis** (for embedding cache)
6. **Prometheus** (for metrics)

---

## Deployment Files

| File | Purpose |
|------|---------|
| `kustomization.yaml` | Kustomize configuration |
| `namespace.yaml` | Namespace definition |
| `serviceaccount.yaml` | ServiceAccount for RBAC |
| `role.yaml` | Role with minimal permissions |
| `rolebinding.yaml` | RoleBinding for ServiceAccount |
| `configmap.yaml` | Non-sensitive configuration |
| `secret.yaml` | Database credentials (change in production!) |
| `deployment.yaml` | Main service deployment (3 replicas) |
| `service.yaml` | ClusterIP service (ports 8080, 9090) |
| `servicemonitor.yaml` | Prometheus scraping config |
| `networkpolicy.yaml` | Network isolation rules |

---

## Configuration

### Edit Before Deploying

#### 1. Update `secret.yaml` (CRITICAL)

```yaml
# ⚠️ CHANGE THESE IN PRODUCTION!
stringData:
  DB_USER: "your-db-user"
  DB_PASSWORD: "your-secure-password"
  EMBEDDING_API_KEY: "sk-your-openai-key"  # Optional
```

#### 2. Update `configmap.yaml`

```yaml
# Update database hostname
  DB_HOST: "your-postgres-service"  # Change to your PostgreSQL service name

# Enable Vector DB (optional)
  VECTOR_DB_ENABLED: "true"
  VECTOR_DB_HOST: "your-vector-db-service"

# Enable embedding generation (optional)
  EMBEDDING_ENABLED: "true"
  EMBEDDING_API_KEY: "from-secret"

# Enable caching (optional)
  CACHE_ENABLED: "true"
  CACHE_HOST: "your-redis-service"
```

#### 3. Update `deployment.yaml` replicas

```yaml
spec:
  replicas: 3  # Adjust based on load
```

---

## Deployment Steps

### Step 1: Verify Prerequisites

```bash
# Check Kubernetes version
kubectl version --short

# Check PostgreSQL is running
kubectl get pods -n kubernaut -l app=postgres

# Verify pgvector extension
kubectl exec -it postgres-0 -n kubernaut -- \
  psql -U postgres -c "SELECT extversion FROM pg_extension WHERE extname = 'vector';"
```

### Step 2: Deploy Service

```bash
# Deploy with kustomize
kubectl apply -k deploy/data-storage/

# Or deploy individual files
kubectl apply -f deploy/data-storage/namespace.yaml
kubectl apply -f deploy/data-storage/serviceaccount.yaml
kubectl apply -f deploy/data-storage/role.yaml
kubectl apply -f deploy/data-storage/rolebinding.yaml
kubectl apply -f deploy/data-storage/configmap.yaml
kubectl apply -f deploy/data-storage/secret.yaml
kubectl apply -f deploy/data-storage/deployment.yaml
kubectl apply -f deploy/data-storage/service.yaml
kubectl apply -f deploy/data-storage/servicemonitor.yaml
kubectl apply -f deploy/data-storage/networkpolicy.yaml
```

### Step 3: Verify Deployment

```bash
# Check pods are running
kubectl get pods -n kubernaut -l app=data-storage-service

# Expected output:
# NAME                                     READY   STATUS    RESTARTS   AGE
# data-storage-service-xxxxx-xxxxx         1/1     Running   0          1m
# data-storage-service-xxxxx-xxxxx         1/1     Running   0          1m
# data-storage-service-xxxxx-xxxxx         1/1     Running   0          1m

# Check logs for errors
kubectl logs -n kubernaut -l app=data-storage-service --tail=50

# Verify health endpoints
kubectl port-forward -n kubernaut svc/data-storage-service 8080:8080 &
curl http://localhost:8080/health    # Should return {"status": "healthy"}
curl http://localhost:8080/ready     # Should return {"status": "ready"}
```

### Step 4: Verify Metrics

```bash
# Port-forward metrics endpoint
kubectl port-forward -n kubernaut svc/data-storage-service 9090:9090 &

# Check metrics are exposed
curl http://localhost:9090/metrics | grep datastorage

# Expected metrics:
# datastorage_write_total
# datastorage_write_duration_seconds
# datastorage_dualwrite_success_total
# datastorage_dualwrite_failure_total
# datastorage_fallback_mode_total
# datastorage_cache_hits_total
# datastorage_cache_misses_total
# datastorage_embedding_generation_duration_seconds
# datastorage_validation_failures_total
# datastorage_query_total
# datastorage_query_duration_seconds
```

---

## Troubleshooting

### Pod Not Starting

```bash
# Check pod status
kubectl describe pod -n kubernaut -l app=data-storage-service

# Common issues:
# 1. ImagePullBackOff - Check image name in deployment.yaml
# 2. CrashLoopBackOff - Check logs for errors
# 3. Pending - Check resource requests/limits
```

### Database Connection Failed

```bash
# Test database connectivity from pod
kubectl exec -it deployment/data-storage-service -n kubernaut -- \
  psql -h postgres-service -U slm_user -d action_history -c "SELECT 1;"

# Common issues:
# 1. Wrong DB_HOST in configmap.yaml
# 2. Wrong credentials in secret.yaml
# 3. PostgreSQL not running
# 4. NetworkPolicy blocking connection
```

### HNSW Index Errors

```bash
# Check PostgreSQL version
kubectl exec postgres-0 -n kubernaut -- \
  psql -U postgres -c "SELECT version();"

# Should show: PostgreSQL 16.x

# Check pgvector version
kubectl exec postgres-0 -n kubernaut -- \
  psql -U postgres -c "SELECT extversion FROM pg_extension WHERE extname = 'vector';"

# Should show: 0.5.1 or higher
```

---

## Scaling

### Horizontal Scaling

```bash
# Scale up
kubectl scale deployment/data-storage-service -n kubernaut --replicas=5

# Scale down
kubectl scale deployment/data-storage-service -n kubernaut --replicas=2

# Auto-scaling (optional)
kubectl autoscale deployment/data-storage-service -n kubernaut \
  --cpu-percent=80 \
  --min=3 \
  --max=10
```

### Vertical Scaling

Edit `deployment.yaml`:

```yaml
resources:
  requests:
    cpu: 200m      # Increase for higher load
    memory: 256Mi  # Increase for more concurrent connections
  limits:
    cpu: 1000m
    memory: 1Gi
```

---

## Monitoring

### Grafana Dashboard

```bash
# Import dashboard
# File: docs/services/stateless/data-storage/observability/grafana-dashboard.json

# 1. Navigate to Grafana → Dashboards → Import
# 2. Upload grafana-dashboard.json
# 3. Select Prometheus data source
# 4. Click "Import"
```

### Alerts

```bash
# Apply Prometheus alerting rules
# File: docs/services/stateless/data-storage/observability/prometheus-alerts.yaml

kubectl apply -f docs/services/stateless/data-storage/observability/prometheus-alerts.yaml
```

---

## Rollback

### Rollback to Previous Version

```bash
# Check rollout history
kubectl rollout history deployment/data-storage-service -n kubernaut

# Rollback to previous version
kubectl rollout undo deployment/data-storage-service -n kubernaut

# Rollback to specific revision
kubectl rollout undo deployment/data-storage-service -n kubernaut --to-revision=2

# Monitor rollback
kubectl rollout status deployment/data-storage-service -n kubernaut
```

---

## Uninstall

```bash
# Delete all resources
kubectl delete -k deploy/data-storage/

# Or delete individually
kubectl delete -f deploy/data-storage/
```

---

## Security Notes

### Production Checklist

- [ ] **Change default passwords** in `secret.yaml`
- [ ] **Use Kubernetes Secrets** instead of stringData
- [ ] **Enable TLS** for PostgreSQL connections (DB_SSL_MODE=require)
- [ ] **Review NetworkPolicy** and adjust for your cluster
- [ ] **Set resource limits** appropriate for your workload
- [ ] **Enable PodSecurityPolicy** or Pod Security Standards
- [ ] **Configure ServiceMonitor** for Prometheus scraping
- [ ] **Review RBAC permissions** and minimize as needed

### Secrets Management

```bash
# Create secret from file (recommended for production)
kubectl create secret generic data-storage-secret \
  --from-literal=DB_USER=your-user \
  --from-literal=DB_PASSWORD=your-password \
  --from-literal=EMBEDDING_API_KEY=sk-your-key \
  -n kubernaut

# Or use external secrets operator (e.g., Sealed Secrets, Vault)
```

---

## Performance Tuning

### Connection Pool Size

Edit `configmap.yaml`:

```yaml
DB_MAX_CONNECTIONS: "100"  # Increase for higher concurrency
```

### Query Timeouts

```yaml
QUERY_TIMEOUT: "60s"   # Increase for slow queries
WRITE_TIMEOUT: "30s"   # Increase for slow writes
```

### Embedding Cache

```yaml
CACHE_ENABLED: "true"
CACHE_TTL: "15m"  # Increase for better hit rate
```

---

## Support

**Documentation**: [docs/services/stateless/data-storage/README.md](../../docs/services/stateless/data-storage/README.md)
**Runbook**: [docs/services/stateless/data-storage/observability/ALERTING_RUNBOOK.md](../../docs/services/stateless/data-storage/observability/ALERTING_RUNBOOK.md)
**Troubleshooting**: See main README troubleshooting section

---

**Maintainer**: Kubernaut Data Storage Team
**Last Updated**: October 13, 2025
**Version**: 1.0

