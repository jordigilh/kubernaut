# HolmesGPT API - Kubernetes Deployment

This directory contains Kubernetes manifests for deploying the HolmesGPT API service.

## Overview

**HolmesGPT API** provides AI-powered recovery analysis for failed Kubernetes remediation actions. It integrates with:
- **LLM Providers** (OpenAI, Claude, Vertex AI, or custom)
- **Context API** for historical context enrichment
- **Kubernetes API** for ServiceAccount token authentication

## Prerequisites

### 1. Kubernetes Cluster
- **Version**: 1.24+ (or OpenShift 4.10+)
- **Namespace**: `kubernaut-system` (created by Context API deployment)

### 2. Context API (REQUIRED)

**⚠️ CRITICAL**: HolmesGPT API requires Context API for historical context enrichment. Deploy Context API **before** deploying HolmesGPT API.

#### Deploy Context API

```bash
# OpenShift: Use BuildConfig for OCP-native build
oc apply -k docs/services/stateless/context-api/deployment/

# Or deploy existing Context API manifests
kubectl apply -f deploy/context-api-deployment.yaml
```

#### Verify Context API Deployment

```bash
# 1. Check pod is running
kubectl get pods -n kubernaut-system -l app=context-api
# Expected: 1/1 Running

# 2. Test health endpoint
kubectl exec -n kubernaut-system deployment/context-api -- curl -s http://localhost:8091/health
# Expected: {"status":"healthy","time":"..."}

# 3. Verify PostgreSQL connection (from logs)
kubectl logs -n kubernaut-system -l app=context-api | grep "PostgreSQL client created"
# Expected: "PostgreSQL client created successfully"

# 4. Verify Redis connection (from logs)
kubectl logs -n kubernaut-system -l app=context-api | grep "Cache manager created"
# Expected: "Cache manager created with Redis L1 + LRU L2"
```

#### Context API Dependencies

Context API requires these services in `kubernaut-system` namespace:
- **PostgreSQL**: With pgvector extension for semantic search
- **Redis**: For L1 caching layer

If these are not deployed, Context API will fail to start.

**See**: `docs/services/stateless/context-api/DEPLOYMENT.md` for full Context API setup guide.

### 3. LLM Provider Credentials
- **API Key**: For your chosen provider
- **Supported Providers**: OpenAI, Claude (Anthropic), Vertex AI, or custom

### 4. kubectl/oc CLI
- **kubectl**: 1.24+ or **oc**: 4.10+

## Quick Start

### 1. Update Secrets

Edit `04-secret.yaml` and replace placeholder values:

```yaml
stringData:
  LLM_API_KEY: "YOUR_ACTUAL_API_KEY"
  LLM_MODEL: "gpt-4"  # or "claude-3-opus", etc.
  LLM_ENDPOINT: ""     # optional: custom endpoint
```

### 2. Deploy

```bash
# Using kubectl
kubectl apply -k deploy/holmesgpt-api/

# Using oc (OpenShift)
oc apply -k deploy/holmesgpt-api/
```

### 3. Verify Deployment

```bash
# Check pods
kubectl get pods -n kubernaut-ai

# Check service
kubectl get svc -n kubernaut-ai

# View logs
kubectl logs -n kubernaut-ai -l app.kubernetes.io/name=holmesgpt-api
```

## Manifest Files

| File | Purpose |
|------|---------|
| `01-namespace.yaml` | Creates `kubernaut-ai` namespace |
| `02-serviceaccount.yaml` | ServiceAccount for HolmesGPT API |
| `03-rbac.yaml` | ClusterRole + ClusterRoleBinding (TokenReviewer access) |
| `04-secret.yaml` | LLM credentials and configuration |
| `05-configmap.yaml` | Application configuration |
| `06-deployment.yaml` | Main deployment (2 replicas, HA) |
| `07-service.yaml` | ClusterIP service (port 8080) |
| `08-networkpolicy.yaml` | Network security policies |
| `kustomization.yaml` | Kustomize configuration |

## Configuration

### Environment Variables

| Variable | Source | Purpose |
|----------|--------|---------|
| `LOG_LEVEL` | ConfigMap | Logging level (info, debug, error) |
| `DEV_MODE` | ConfigMap | Enable stub mode (true/false) |
| `LLM_API_KEY` | Secret | LLM provider API key |
| `LLM_MODEL` | Secret | LLM model name |
| `LLM_ENDPOINT` | Secret | Optional: custom LLM endpoint |
| `CONTEXT_API_URL` | Secret | Context API service URL |

### Resource Limits

```yaml
resources:
  requests:
    cpu: "200m"
    memory: "512Mi"
  limits:
    cpu: "1000m"
    memory: "2Gi"
```

Adjust based on your workload and LLM usage patterns.

## Security

### RBAC Permissions

HolmesGPT API requires:
- **TokenReviewer API**: For ServiceAccount token validation
- **Read Pods**: For investigation context (optional)
- **Read Events**: For investigation context (optional)

### Network Policy

- **Ingress**: Only from Kubernaut services and monitoring
- **Egress**: Only to K8s API, Context API, and LLM providers (HTTPS)

### Pod Security

- **Non-root user** (UID 1000)
- **Read-only root filesystem**
- **No privilege escalation**
- **seccompProfile**: RuntimeDefault
- **Capabilities**: All dropped

## Health Checks

### Liveness Probe

```
GET /health
Initial Delay: 30s
Period: 10s
```

### Readiness Probe

```
GET /ready
Initial Delay: 10s
Period: 5s
```

## Monitoring

### Prometheus Metrics

Metrics available at: `http://holmesgpt-api:8080/metrics`

**Key Metrics**:
- `holmesgpt_investigations_total` - Total investigations
- `holmesgpt_investigation_duration_seconds` - Investigation duration
- `holmesgpt_llm_calls_total` - LLM API calls
- `holmesgpt_context_api_calls_total` - Context API calls

### Annotations

```yaml
prometheus.io/scrape: "true"
prometheus.io/port: "8080"
prometheus.io/path: "/metrics"
```

## Scaling

### Horizontal Scaling

```bash
# Scale to 5 replicas
kubectl scale deployment holmesgpt-api -n kubernaut-ai --replicas=5
```

### Considerations

- **Session Affinity**: Enabled (3 hours) for consistent LLM context
- **Pod Anti-Affinity**: Spreads replicas across nodes for HA
- **Resource Limits**: Ensure sufficient cluster resources for LLM processing

## Troubleshooting

### Common Issues

#### 1. Pods Not Starting

```bash
# Check pod events
kubectl describe pod -n kubernaut-ai -l app.kubernetes.io/name=holmesgpt-api

# Check logs
kubectl logs -n kubernaut-ai -l app.kubernetes.io/name=holmesgpt-api
```

#### 2. LLM Connection Failures

- Verify `LLM_API_KEY` in secret
- Check `LLM_MODEL` is correct for your provider
- Verify network policy allows outbound HTTPS
- Check logs for detailed error messages

#### 3. Context API Unavailable

**Symptoms**: HolmesGPT API degrades gracefully, but logs show Context API connection errors.

```bash
# 1. Verify Context API is running
kubectl get pods -n kubernaut-system -l app=context-api
# Expected: 1/1 Running (not CrashLoopBackOff)

# 2. Test Context API health from within cluster
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
  curl http://context-api.kubernaut-system.svc.cluster.local:8091/health
# Expected: {"status":"healthy","time":"..."}

# 3. Check Context API logs for errors
kubectl logs -n kubernaut-system -l app=context-api --tail=50

# 4. Verify Context API dependencies
kubectl get pods -n kubernaut-system -l app=postgres
kubectl get pods -n kubernaut-system -l app=redis
```

**Resolution**:
- If Context API is not deployed: Follow [Prerequisites](#2-context-api-required) to deploy it
- If Context API is in CrashLoopBackOff: Check PostgreSQL and Redis are available
- If Context API is running but unreachable: Verify NetworkPolicy allows traffic from HolmesGPT API

**Note**: HolmesGPT API will continue to function without Context API, but recommendations will lack historical context.

#### 4. Authentication Failures

- Verify ServiceAccount `holmesgpt-api` exists
- Check ClusterRoleBinding is applied
- Confirm RBAC permissions for TokenReviewer API

## Upgrade

### Rolling Update

```bash
# Update image tag in kustomization.yaml
kubectl apply -k deploy/holmesgpt-api/

# Check rollout status
kubectl rollout status deployment/holmesgpt-api -n kubernaut-ai
```

### Rollback

```bash
# Rollback to previous version
kubectl rollout undo deployment/holmesgpt-api -n kubernaut-ai
```

## Cleanup

```bash
# Delete all resources
kubectl delete -k deploy/holmesgpt-api/

# Or delete namespace (removes everything)
kubectl delete namespace kubernaut-ai
```

## Integration

### Calling HolmesGPT API

```bash
# From within cluster
curl -X POST http://holmesgpt-api.kubernaut-ai.svc.cluster.local:8080/api/v1/recovery/analyze \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_SERVICE_ACCOUNT_TOKEN" \
  -d '{
    "incident_id": "inc-001",
    "failed_action": {
      "type": "scale-deployment",
      "target": "deployment/api-server"
    },
    "failure_context": {
      "error": "timeout",
      "error_message": "Operation timed out after 60s"
    }
  }'
```

### AIAnalysis Controller Integration

The AIAnalysis CRD controller should configure the HolmesGPT API client:

```go
holmesClient := holmesgpt.NewClient(
    "http://holmesgpt-api.kubernaut-ai.svc.cluster.local:8080",
    holmesgpt.WithAuth(token),
)
```

## Support

For issues or questions:
- **Documentation**: `docs/services/stateless/holmesgpt-api/`
- **Logs**: `kubectl logs -n kubernaut-ai -l app.kubernetes.io/name=holmesgpt-api`
- **Metrics**: Access Prometheus metrics at `/metrics`



