# Deployment Guide

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
- HolmesGPT service: http://localhost:8090
- LocalAI endpoint: http://192.168.1.169:8080 (if configured)

### 3. Kubernetes (Production)

```bash
# Create namespace
kubectl create namespace kubernaut

# Create secrets
kubectl create secret generic holmesgpt-secrets \
  --from-literal=openai-api-key=your_api_key \
  -n kubernaut

# Deploy services
kubectl apply -f k8s/ -n kubernaut
```

## Configuration

### Go Service Environment Variables

```env
AI_SERVICES_HOLMESGPT_ENABLED=true
AI_SERVICES_HOLMESGPT_ENDPOINT=http://holmesgpt-service:8090
POSTGRES_HOST=postgres-service
POSTGRES_DB=prometheus_alerts
POSTGRES_USER=alertsuser
POSTGRES_PASSWORD=secretpassword
```

## Resource Requirements

### Minimum

- Go Service: 256Mi memory, 200m CPU (includes HolmesGPT client)
- HolmesGPT Service: 512Mi memory, 200m CPU

### Recommended Production

- Go Service: 1Gi memory, 500m CPU, 3 replicas (includes HolmesGPT client)
- HolmesGPT Service: 2Gi memory, 500m CPU, 2 replicas

## Monitoring

### Health Checks

```bash
# Go service health (includes HolmesGPT connectivity)
curl http://localhost:8080/health

# HolmesGPT service health
curl http://localhost:8090/health
```

### Metrics

Prometheus metrics available at:
- Go service: http://localhost:8080/metrics (includes HolmesGPT integration metrics)

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
      port: 8090  # HolmesGPT Service
```

## Troubleshooting

### Common Issues

**HolmesGPT service fails to start:**
- Check HolmesGPT container configuration
- Verify LLM provider connectivity
- Review startup logs for validation errors

**Go service can't connect to HolmesGPT:**
- Verify HolmesGPT service is running and healthy
- Check network connectivity between services
- Confirm HolmesGPT service URL configuration

**High latency:**
- Optimize HolmesGPT configuration
- Increase HolmesGPT service replicas
- Optimize LLM provider settings (model, max_tokens)

### Log Analysis

```bash
# Go service logs (includes HolmesGPT integration)
kubectl logs -f deployment/kubernaut -n kubernaut

# HolmesGPT service logs
kubectl logs -f deployment/holmesgpt-service -n kubernaut

# Filter for errors
kubectl logs deployment/holmesgpt-service -n kubernaut | grep ERROR
```
