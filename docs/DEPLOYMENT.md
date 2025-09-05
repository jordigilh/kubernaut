# Deployment Guide

## Prerequisites

- Kubernetes cluster with RBAC enabled
- PostgreSQL database for action history
- LLM provider API key (OpenAI, Anthropic, Azure, or AWS)

## Quick Start

### 1. Docker Compose (Development)

```bash
cd python-api
docker-compose up -d
```

Access:
- Go service: http://localhost:8080
- Python API: http://localhost:8000
- API docs: http://localhost:8000/docs

### 2. Kubernetes (Production)

```bash
# Create namespace
kubectl create namespace prometheus-alerts-slm

# Create secrets
kubectl create secret generic holmesgpt-secrets \
  --from-literal=openai-api-key=your_api_key \
  -n prometheus-alerts-slm

# Deploy services
kubectl apply -f k8s/ -n prometheus-alerts-slm
```

## Configuration

### Go Service Environment Variables

```env
ENABLE_HOLMES_GPT=true
HOLMES_API_BASE_URL=http://holmesgpt-api-service:8000
POSTGRES_HOST=postgres-service
POSTGRES_DB=prometheus_alerts
POSTGRES_USER=alertsuser
POSTGRES_PASSWORD=secretpassword
```

### Python Service Environment Variables

```env
# Cloud LLM Provider
HOLMES_LLM_PROVIDER=openai
OPENAI_API_KEY=your_openai_api_key
HOLMES_DEFAULT_MODEL=gpt-4

# Or Local On-Premises LLM
HOLMES_LLM_PROVIDER=ollama
OLLAMA_BASE_URL=http://ollama-service:11434
HOLMES_DEFAULT_MODEL=llama3.1:8b

# Performance Settings
CACHE_ENABLED=true
REDIS_URL=redis://redis-service:6379
```

## Resource Requirements

### Minimum

- Go Service: 128Mi memory, 100m CPU
- Python Service: 256Mi memory, 100m CPU

### Recommended Production

- Go Service: 512Mi memory, 250m CPU, 3 replicas
- Python Service: 2Gi memory, 500m CPU, 2 replicas

## Monitoring

### Health Checks

```bash
# Go service health
curl http://localhost:8080/health

# Python service health
curl http://localhost:8000/health
```

### Metrics

Prometheus metrics available at:
- Go service: http://localhost:8080/metrics
- Python service: http://localhost:9090/metrics

## Security

### RBAC Configuration

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: prometheus-alerts-slm
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
  name: prometheus-alerts-slm-policy
spec:
  podSelector:
    matchLabels:
      app: prometheus-alerts-slm
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: prometheus
  egress:
  - to: []
    ports:
    - protocol: TCP
      port: 8000  # HolmesGPT API
```

## Troubleshooting

### Common Issues

**Python service fails to start:**
- Check LLM provider API key validity
- Verify HolmesGPT dependency installation
- Review startup logs for validation errors

**Go service can't connect to Python API:**
- Verify Python service is running and healthy
- Check network connectivity between services
- Confirm Python service URL configuration

**High latency:**
- Enable caching in Python service
- Increase Python service replicas
- Optimize LLM provider settings (model, max_tokens)

### Log Analysis

```bash
# Go service logs
kubectl logs -f deployment/prometheus-alerts-slm -n prometheus-alerts-slm

# Python service logs
kubectl logs -f deployment/holmesgpt-api -n prometheus-alerts-slm

# Filter for errors
kubectl logs deployment/holmesgpt-api -n prometheus-alerts-slm | grep ERROR
```
