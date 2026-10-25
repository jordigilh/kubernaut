# Deployment Guide

> **⚠️ Legacy reference**: This document predates the Go rewrite of HolmesGPT into
> **Kubernaut Agent (KA)** and the operator-managed deployment path. The Docker Compose,
> manual `kubectl apply -f k8s/`, and legacy environment-variable examples below are not
> supported production installation instructions. Use the
> [Kubernaut Operator installation guide](https://github.com/jordigilh/kubernaut-operator/blob/main/docs/installation/00-quickstart.md).

## Prerequisites

- Kubernetes cluster with RBAC enabled
- PostgreSQL database for action history (with pgvector extension for vector operations)
- LLM provider API key (OpenAI, Anthropic, Azure, or AWS) OR LocalAI endpoint
- **NEW in Milestone 1**: Separate PostgreSQL database for vector storage (optional)
- **NEW in Milestone 1**: File system access for report export with proper permissions

## Quick Start

### 1. Kind Cluster (Recommended for Development)

```bash
# Bootstrap complete integration environment
make bootstrap-dev-kind
```

Access:
- Webhook service: http://localhost:30800
- Prometheus: http://localhost:30090
- AlertManager: http://localhost:30093
- PostgreSQL: localhost:30432
- External LLM: http://192.168.1.169:8080

### 2. Docker Compose (DEPRECATED - Legacy Development)

> ⚠️ **DEPRECATED**: Use Kind cluster instead for better production parity

```bash
# Legacy setup (use make bootstrap-dev-kind instead)
make bootstrap-dev-compose
```

Access (legacy):
- Go service: http://localhost:8080
- HolmesGPT service (now Kubernaut Agent (KA)): http://localhost:8090
- LocalAI endpoint: http://192.168.1.169:8080 (if configured)

### 3. Kubernetes (Production)

Use the [Kubernaut Operator installation guide](https://github.com/jordigilh/kubernaut-operator/blob/main/docs/installation/00-quickstart.md)
for namespace, dependency, credential, and `Kubernaut` CR setup.

## Configuration

### Go Service Environment Variables

```env
# Legacy env vars for the pre-rewrite "HolmesGPT" service, now Kubernaut Agent (KA); current KA
# configuration is via a mounted config file, not these env vars — see the
# Kubernaut Operator CR configuration.
AI_SERVICES_HOLMESGPT_ENABLED=true
AI_SERVICES_HOLMESGPT_ENDPOINT=http://holmesgpt-service:8090
POSTGRES_HOST=postgres-service
POSTGRES_DB=prometheus_alerts
POSTGRES_USER=alertsuser
POSTGRES_PASSWORD=secretpassword
```

## Resource Requirements

### Minimum

- Go Service: 256Mi memory, 200m CPU (includes Kubernaut Agent (KA) client)
- Kubernaut Agent (KA) (formerly "HolmesGPT Service"): 512Mi memory, 200m CPU

### Recommended Production

- Go Service: 1Gi memory, 500m CPU, 3 replicas (includes Kubernaut Agent (KA) client)
- Kubernaut Agent (KA) (formerly "HolmesGPT Service"): 2Gi memory, 500m CPU, 2 replicas

## Monitoring

### Health Checks

```bash
# Go service health (includes Kubernaut Agent (KA) connectivity)
curl http://localhost:8080/health

# Kubernaut Agent (KA) health (formerly "HolmesGPT service health"; legacy port shown, current KA health port is 8081)
curl http://localhost:8090/health
```

### Metrics

Prometheus metrics available at:
- Go service: http://localhost:8080/metrics (includes Kubernaut Agent (KA) integration metrics)

## Security

### RBAC Configuration

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut
rules:
- apiGroups: [""]
  resources: ["pods", "nodes", "services"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets"]
  verbs: ["get", "list", "watch", "update", "patch", "scale"]
```

### Network Policies

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kubernaut-policy
spec:
  podSelector:
    matchLabels:
      app: kubernaut
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: prometheus
  egress:
  - to: []
    ports:
    - protocol: TCP
      port: 8090  # Kubernaut Agent (KA), formerly "HolmesGPT Service"; legacy port shown, current KA API port is 8443
```

## Troubleshooting

### Common Issues

**Kubernaut Agent (KA) fails to start** (formerly "HolmesGPT service fails to start"):
- Check the KA container configuration
- Verify LLM provider connectivity
- Review startup logs for validation errors

**Go service can't connect to Kubernaut Agent (KA)** (formerly "...to HolmesGPT"):
- Verify KA is running and healthy
- Check network connectivity between services
- Confirm KA service URL configuration

**High latency:**
- Optimize Kubernaut Agent (KA) configuration
- Increase KA replicas
- Optimize LLM provider settings (model, max_tokens)

### Log Analysis

```bash
# Go service logs (includes Kubernaut Agent (KA) integration)
kubectl logs -f deployment/kubernaut -n kubernaut

# Kubernaut Agent (KA) logs (formerly "HolmesGPT service logs"; legacy deployment name shown)
kubectl logs -f deployment/holmesgpt-service -n kubernaut

# Filter for errors
kubectl logs deployment/holmesgpt-service -n kubernaut | grep ERROR
```
