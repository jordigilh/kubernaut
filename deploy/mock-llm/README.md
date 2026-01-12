# Mock LLM Service - Kubernetes Deployment

Kubernetes deployment manifests for the Mock LLM service used in E2E tests.

## Overview

This directory contains Kubernetes manifests for deploying the Mock LLM service to a Kind cluster for E2E testing.

**Key Details**:
- **Namespace**: `kubernaut-system` (shared with all E2E services)
- **Service Type**: `ClusterIP` (internal only - no external access needed)
- **Internal URL**: `http://mock-llm:8080` (simplified DNS - same namespace)
- **Image**: `localhost/mock-llm:latest`
- **Purpose**: E2E testing only
- **Access Pattern**: Services inside Kind cluster access via short DNS name

## Quick Start

### Prerequisites

1. Build the Mock LLM image:
   ```bash
   cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
   podman build -t localhost/mock-llm:latest -f test/services/mock-llm/Dockerfile .
   ```

2. Load image to Kind cluster:
   ```bash
   kind load docker-image localhost/mock-llm:latest --name kubernaut-test
   ```

### Deploy to Kind

```bash
# Using kubectl
kubectl apply -k deploy/mock-llm/

# Or using kustomize
kustomize build deploy/mock-llm/ | kubectl apply -f -
```

### Verify Deployment

```bash
# Check pod status
kubectl get pods -n kubernaut-system -l app=mock-llm

# Check service (ClusterIP)
kubectl get svc -n kubernaut-system mock-llm

# Test from within cluster (primary access method - simplified DNS)
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -n kubernaut-system -- \
  curl http://mock-llm:8080/health

# Or use port-forward for local testing
kubectl port-forward -n kubernaut-system svc/mock-llm 8080:8080 &
curl http://127.0.0.1:8080/health
```

## Port Allocation

**Authoritative Reference**: `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md` v2.5

- **E2E Tests (Kind ClusterIP)**: No external port (internal only)
  - Namespace: `kubernaut-system` (shared with HAPI, DataStorage, etc.)
  - Internal URL: `http://mock-llm:8080` (simplified DNS - same namespace)
  - Access: Services use short DNS name (automatic Kubernetes resolution)
- **HAPI Integration (Podman)**: `18140` (localhost)
- **AIAnalysis Integration (Podman)**: `18141` (localhost)

**Note**:
- Integration tests use unique localhost ports to avoid collisions during parallel execution
- E2E tests use ClusterIP service in `kubernaut-system` (matches DataStorage pattern)
- Simplified DNS: `http://mock-llm:8080` instead of `http://mock-llm.mock-llm.svc.cluster.local:8080`

## Manifest Files

| File | Description |
|------|-------------|
| `01-deployment.yaml` | Mock LLM deployment (1 replica) in `kubernaut-system` |
| `02-service.yaml` | ClusterIP service (internal only - no NodePort) |
| `kustomization.yaml` | Kustomize configuration |

**Note**: No dedicated namespace file - Mock LLM deploys to existing `kubernaut-system` namespace (shared with all E2E services)

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MOCK_LLM_HOST` | `0.0.0.0` | Bind address |
| `MOCK_LLM_PORT` | `8080` | Internal container port |
| `MOCK_LLM_FORCE_TEXT` | `false` | Force text responses (no tool calls) |

### Resource Limits

- **Requests**: 64Mi memory, 100m CPU
- **Limits**: 128Mi memory, 200m CPU

### Health Checks

- **Liveness**: `/liveness` every 10s (after 10s)
- **Readiness**: `/readiness` every 5s (after 5s)

## Testing Endpoints

Once deployed, the Mock LLM service is accessible:
- **E2E (Kind)**: `http://mock-llm:8080` (ClusterIP - simplified DNS, same namespace)
- **HAPI Integration (Podman)**: `http://127.0.0.1:18140`
- **AIAnalysis Integration (Podman)**: `http://127.0.0.1:18141`

### E2E Testing (from inside Kind cluster)

```bash
# Services inside Kind cluster use simplified DNS (same namespace)
# Example: HAPI pod accessing Mock LLM
LLM_ENDPOINT=http://mock-llm:8080

# For local testing, use port-forward
kubectl port-forward -n kubernaut-system svc/mock-llm 8080:8080 &

# Health check (via port-forward)
curl http://127.0.0.1:8080/health

# Readiness
curl http://127.0.0.1:8080/readiness

# Metrics
curl http://127.0.0.1:8080/metrics

# Chat completions (OpenAI-compatible)
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mock-model",
    "messages": [{"role": "user", "content": "analyze OOMKilled signal"}],
    "tools": [{"type": "function", "function": {"name": "search_workflow_catalog"}}]
  }'
```

## Integration with Tests

### HAPI E2E Tests

The HAPI E2E tests access the Mock LLM service via simplified Kubernetes DNS:
- Environment variable: `LLM_ENDPOINT=http://mock-llm:8080` (same namespace - auto-resolves)
- Model: `LLM_MODEL=mock-model`
- Access Pattern: HAPI pod (kubernaut-system) → Mock LLM ClusterIP (kubernaut-system)
- **Rationale**: Both in same namespace, Kubernetes automatically resolves short DNS name

### AIAnalysis E2E Tests

The AIAnalysis E2E tests use the same simplified ClusterIP endpoint:
- Environment variable: `LLM_ENDPOINT=http://mock-llm:8080`
- Same namespace pattern as HAPI

### Integration Tests (Podman)

Integration tests use dedicated localhost ports (not Kind):
- HAPI: `http://127.0.0.1:18140`
- AIAnalysis: `http://127.0.0.1:18141`
- **Note**: Each service gets unique port to avoid parallel test collisions

## Troubleshooting

### Pod not starting

```bash
# Check pod logs
kubectl logs -n kubernaut-system -l app=mock-llm

# Check pod events
kubectl describe pod -n kubernaut-system -l app=mock-llm
```

### Image not found

```bash
# Verify image is loaded in Kind
podman exec -it kubernaut-test-control-plane crictl images | grep mock-llm

# Reload image if missing
kind load docker-image localhost/mock-llm:latest --name kubernaut-test
```

### Service not accessible

```bash
# Verify service endpoint
kubectl get endpoints -n kubernaut-system mock-llm

# Check if port is listening
kubectl port-forward -n kubernaut-system svc/mock-llm 8080:8080
curl http://127.0.0.1:8080/health
```

### Health check failing

```bash
# Test health endpoint from inside pod
kubectl exec -n kubernaut-system -l app=mock-llm -- \
  python -c "import urllib.request; print(urllib.request.urlopen('http://localhost:8080/health').read())"
```

## Cleanup

```bash
# Delete Mock LLM deployment
kubectl delete -k deploy/mock-llm/

# Or delete individual resources
kubectl delete deployment,service -n kubernaut-system -l app=mock-llm
```

**Note**: Do not delete `kubernaut-system` namespace - it's shared with other services

## Development

### Update Image

```bash
# Rebuild image
podman build -t localhost/mock-llm:latest -f test/services/mock-llm/Dockerfile .

# Reload to Kind
kind load docker-image localhost/mock-llm:latest --name kubernaut-test

# Restart pods to use new image
kubectl rollout restart deployment/mock-llm -n kubernaut-system
```

### View Logs

```bash
# Follow logs
kubectl logs -n kubernaut-system -l app=mock-llm -f

# View recent logs
kubectl logs -n kubernaut-system -l app=mock-llm --tail=100
```

## References

- **Service Source**: `test/services/mock-llm/`
- **Port Allocation**: `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md`
- **Migration Plan**: `docs/plans/MOCK_LLM_MIGRATION_PLAN.md`
- **Test Plan**: `docs/plans/MOCK_LLM_TEST_PLAN.md`
