# Dynamic Toolset Service - Kubernetes Deployment

**Version**: V1.0
**Last Updated**: October 13, 2025

---

## Overview

This directory contains Kubernetes manifests for deploying the Dynamic Toolset Service in-cluster for E2E testing and production use.

**Deployment Mode**:
- **V1**: Out-of-cluster (development, local testing)
- **V2**: In-cluster (E2E testing, production) - **These manifests**

---

## Manifests

| File | Purpose |
|------|---------|
| `namespace.yaml` | Creates `kubernaut-system` namespace |
| `rbac.yaml` | ServiceAccount, ClusterRole, ClusterRoleBinding, Role, RoleBinding |
| `configmap.yaml` | Service configuration (discovery interval, namespaces) |
| `deployment.yaml` | Deployment with 1 replica, health probes, resource limits |
| `service.yaml` | ClusterIP services for HTTP (8080) and metrics (9090) |
| `kustomization.yaml` | Kustomize configuration for easy deployment |

---

## Prerequisites

- Kubernetes cluster 1.24+ (tested with 1.27+)
- kubectl CLI with cluster-admin permissions
- Container image: `kubernaut/dynamic-toolset:v1.0`

---

## Quick Start

### Option 1: Using Kustomize (Recommended)

```bash
# Deploy all resources
kubectl apply -k deploy/dynamic-toolset/

# Verify deployment
kubectl get pods -n kubernaut-system -l app=dynamic-toolset
kubectl logs -n kubernaut-system -l app=dynamic-toolset
```

### Option 2: Using kubectl (Individual Manifests)

```bash
# Apply manifests in order
kubectl apply -f deploy/dynamic-toolset/namespace.yaml
kubectl apply -f deploy/dynamic-toolset/rbac.yaml
kubectl apply -f deploy/dynamic-toolset/configmap.yaml
kubectl apply -f deploy/dynamic-toolset/deployment.yaml
kubectl apply -f deploy/dynamic-toolset/service.yaml

# Verify deployment
kubectl get all -n kubernaut-system
```

---

## Verification Steps

### 1. Check Pod Status

```bash
kubectl get pods -n kubernaut-system -l app=dynamic-toolset

# Expected output:
# NAME                               READY   STATUS    RESTARTS   AGE
# dynamic-toolset-xxxxxxxxxx-xxxxx   1/1     Running   0          30s
```

### 2. Check Health Endpoints

```bash
# Port-forward to access service
kubectl port-forward -n kubernaut-system svc/dynamic-toolset 8080:8080

# In another terminal, check health
curl http://localhost:8080/health
# Expected: {"status":"ok"}

curl http://localhost:8080/ready
# Expected: {"kubernetes":true}
```

### 3. Verify Service Discovery

```bash
# Get ServiceAccount token
export TOKEN=$(kubectl create token dynamic-toolset -n kubernaut-system --duration=1h)

# List discovered services
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/services

# Get toolset JSON
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/toolset
```

### 4. Check ConfigMap Generation

```bash
# Check if toolset ConfigMap was created
kubectl get configmap kubernaut-toolset-config -n kubernaut-system

# View ConfigMap contents
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o yaml
```

### 5. Verify Metrics

```bash
# Port-forward to metrics port
kubectl port-forward -n kubernaut-system svc/dynamic-toolset-metrics 9090:9090

# In another terminal, check metrics
curl http://localhost:9090/metrics | grep dynamictoolset

# Expected metrics:
# - dynamictoolset_services_discovered_total
# - dynamictoolset_discovery_duration_seconds
# - dynamictoolset_api_requests_total
# - dynamictoolset_configmap_updates_total
```

### 6. Check Logs

```bash
# Follow logs
kubectl logs -f -n kubernaut-system -l app=dynamic-toolset

# Expected log entries:
# - "Service discovery complete"
# - "ConfigMap updated"
# - "HTTP server started on :8080"
# - "Metrics server started on :9090"
```

---

## Configuration

### Environment Variables (via ConfigMap)

Edit `configmap.yaml` to change configuration:

```yaml
data:
  discovery-interval: "5m"  # Change to "1m" for faster discovery
  namespaces: "monitoring,observability,default"  # Add/remove namespaces
```

Apply changes:
```bash
kubectl apply -f deploy/dynamic-toolset/configmap.yaml
kubectl rollout restart deployment/dynamic-toolset -n kubernaut-system
```

### Resource Limits

Edit `deployment.yaml` to adjust resources:

```yaml
resources:
  requests:
    memory: "128Mi"  # Minimum required
    cpu: "250m"
  limits:
    memory: "256Mi"  # Maximum allowed
    cpu: "500m"
```

### Scaling

```bash
# Scale to 2 replicas (for HA)
kubectl scale deployment/dynamic-toolset -n kubernaut-system --replicas=2

# Or edit deployment.yaml and reapply
```

---

## RBAC Permissions

The service requires the following permissions:

### ClusterRole Permissions

- `services`: get, list, watch (for service discovery)
- `configmaps`: get, list, create, update, patch, delete (for toolset management)
- `tokenreviews`: create (for authentication)
- `namespaces`: get, list (for namespace discovery)
- `endpoints`: get, list (for health checks)

### Namespace-Specific Role Permissions

- `configmaps` in `kubernaut-system`: Full management of `kubernaut-toolset-config`

---

## Troubleshooting

### Issue: Pod Not Starting

**Check Events**:
```bash
kubectl describe pod -n kubernaut-system -l app=dynamic-toolset
```

**Common Causes**:
- Image pull failure: Check `imagePullPolicy` and image availability
- Resource constraints: Check node resources
- RBAC issues: Verify ServiceAccount exists

### Issue: Service Not Discovering

**Check Logs**:
```bash
kubectl logs -n kubernaut-system -l app=dynamic-toolset | grep discovery
```

**Common Causes**:
- RBAC permissions insufficient: Check ClusterRole permissions
- Namespaces not configured: Check ConfigMap `namespaces` field
- Services don't match detector patterns: Check service labels/annotations

### Issue: ConfigMap Not Created

**Check RBAC**:
```bash
kubectl auth can-i create configmaps \
  --as=system:serviceaccount:kubernaut-system:dynamic-toolset \
  -n kubernaut-system
```

**Check Logs**:
```bash
kubectl logs -n kubernaut-system -l app=dynamic-toolset | grep ConfigMap
```

### Issue: Authentication Failures

**Verify TokenReview Access**:
```bash
kubectl auth can-i create tokenreviews.authentication.k8s.io \
  --as=system:serviceaccount:kubernaut-system:dynamic-toolset
```

**Check Logs**:
```bash
kubectl logs -n kubernaut-system -l app=dynamic-toolset | grep TokenReview
```

---

## Uninstalling

### Using Kustomize

```bash
kubectl delete -k deploy/dynamic-toolset/
```

### Using kubectl

```bash
kubectl delete -f deploy/dynamic-toolset/deployment.yaml
kubectl delete -f deploy/dynamic-toolset/service.yaml
kubectl delete -f deploy/dynamic-toolset/configmap.yaml
kubectl delete -f deploy/dynamic-toolset/rbac.yaml
# Optional: Delete namespace (removes all resources)
# kubectl delete namespace kubernaut-system
```

---

## E2E Testing

These manifests are designed for E2E testing. For E2E test scenarios, see:
- [E2E Test Plan](../../docs/services/stateless/dynamic-toolset/implementation/testing/03-e2e-test-plan.md)

**E2E Test Setup**:
```bash
# 1. Create Kind cluster
kind create cluster --name kubernaut-e2e

# 2. Deploy Dynamic Toolset
kubectl apply -k deploy/dynamic-toolset/

# 3. Deploy test services (Prometheus, Grafana, etc.)
kubectl apply -f test/e2e/fixtures/

# 4. Run E2E tests
go test -v ./test/e2e/toolset/...
```

---

## Future Enhancements

### Helm Chart

These manifests will be wrapped into a Helm chart for easier deployment:

```bash
# Future Helm deployment (V2)
helm install dynamic-toolset ./charts/dynamic-toolset \
  --namespace kubernaut-system \
  --create-namespace \
  --set discoveryInterval=5m \
  --set namespaces="monitoring,observability"
```

### Operator

These manifests will inform the Kubernaut Operator CRD design:

```yaml
# Future Operator CRD (V3)
apiVersion: kubernaut.io/v1alpha1
kind: DynamicToolset
metadata:
  name: dynamic-toolset
  namespace: kubernaut-system
spec:
  discoveryInterval: 5m
  namespaces:
  - monitoring
  - observability
  replicas: 2
```

---

## Related Documentation

- **[Implementation Plan](../../docs/services/stateless/dynamic-toolset/implementation/IMPLEMENTATION_PLAN_ENHANCED.md)**
- **[Production Readiness](../../docs/services/stateless/dynamic-toolset/implementation/PRODUCTION_READINESS_REPORT.md)**
- **[Handoff Summary](../../docs/services/stateless/dynamic-toolset/implementation/00-HANDOFF-SUMMARY.md)**

---

**Maintainer**: Kubernaut Development Team
**Date**: October 13, 2025
**Status**: ✅ **Ready for E2E Testing and Production Use**

