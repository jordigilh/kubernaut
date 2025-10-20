# HolmesGPT API - Docker Build Guide

## 📋 Overview

This directory contains the Docker build configuration for the **HolmesGPT API service**, the only Python service in the Kubernaut ecosystem.

**Design Decision**: [DD-HOLMESGPT-012 - Minimal Internal Service Architecture](../docs/architecture/decisions/DD-HOLMESGPT-012-Minimal-Internal-Service-Architecture.md)

---

## 🏗️ Build Architecture

### Multi-Stage Build

```
Stage 1 (Builder)          Stage 2 (Runtime)
├── UBI9 Python 3.11      ├── UBI9 Python 3.11 (minimal)
├── Install dependencies  ├── Copy Python packages
├── Install HolmesGPT SDK ├── Copy application code
└── Copy source code      └── Run as non-root user (1001)
```

### Key Features

- ✅ **Red Hat UBI9** base image (Python 3.11)
- ✅ **Multi-stage build** for minimal runtime image
- ✅ **Non-root user** (UID 1001) for security
- ✅ **HolmesGPT SDK** from local dependencies
- ✅ **Health checks** via FastAPI endpoint
- ✅ **Production-ready** with minimal attack surface

---

## 🚀 Quick Start

### 1. Build Image

```bash
# From holmesgpt-api/ directory
./build.sh

# Or with custom tag
./build.sh v1.0.0

# Or manually
podman build -t kubernaut-holmesgpt-api:latest .
```

**Note**: Build is **self-contained** - HolmesGPT SDK is fetched from git during build.

### 2. Run Locally (Dev Mode)

```bash
podman run -d -p 8080:8080 \
  -e DEV_MODE=true \
  -e AUTH_ENABLED=false \
  quay.io/kubernaut/kubernaut-holmesgpt-api:latest
```

### 3. Test Service

```bash
# Health check
curl http://localhost:8080/health

# Readiness check
curl http://localhost:8080/ready

# Test recovery endpoint
curl -X POST http://localhost:8080/api/v1/recovery/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "incident_id": "test-001",
    "failed_action": {"type": "scale_deployment"},
    "failure_context": {"error": "timeout"}
  }'
```

---

## 📦 Build Files

| File | Purpose |
|------|---------|
| `Dockerfile` | Multi-stage build configuration |
| `.dockerignore` | Exclude unnecessary files from build context |
| `build.sh` | Automated build script with tagging |
| `deployment.yaml` | Kubernetes deployment manifest |

---

## 🔧 Build Configuration

### Base Images

```dockerfile
# Builder stage
FROM registry.access.redhat.com/ubi9/python-311:latest AS builder

# Runtime stage
FROM registry.access.redhat.com/ubi9/python-311:latest
```

### Dependencies

- **Python 3.11** (Red Hat UBI9)
- **FastAPI** + **uvicorn** (web framework)
- **aiohttp** (K8s API calls)
- **tenacity** (retry logic)
- **prometheus-client** (metrics)
- **HolmesGPT SDK** (installed from git: `github.com/robusta-dev/holmesgpt`)

### Security

- Non-root user (UID 1001)
- Read-only root filesystem
- No privilege escalation
- Minimal runtime dependencies

---

## 🌐 Networking

### Exposed Ports

| Port | Purpose |
|------|---------|
| 8080 | HTTP (FastAPI endpoints) |

### Endpoints

| Path | Purpose |
|------|---------|
| `/health` | Liveness probe |
| `/ready` | Readiness probe |
| `/api/v1/recovery/analyze` | Recovery analysis |
| `/api/v1/postexec/analyze` | Post-execution analysis |

---

## 🎯 Kubernetes Deployment

### Deploy to Cluster

```bash
kubectl apply -f deployment.yaml
```

### Resources Created

- **ServiceAccount**: `holmesgpt-api` (for K8s TokenReviewer)
- **ClusterRole**: TokenReviewer API access
- **ClusterRoleBinding**: Bind role to service account
- **ConfigMap**: Environment configuration
- **Deployment**: 2 replicas with health checks
- **Service**: ClusterIP on port 8080
- **NetworkPolicy**: Restrict traffic to Effectiveness Monitor and RemediationProcessor

### Network Policy

**Ingress**: Only from `effectiveness-monitor` and `remediationprocessor` pods
**Egress**: Only to `kube-apiserver`, `context-api`, and `ollama`

---

## 🔍 Environment Variables

### Production Configuration

```bash
# Environment
ENVIRONMENT=production
DEV_MODE=false
AUTH_ENABLED=true

# LLM Configuration
LLM_PROVIDER=ollama
LLM_MODEL=llama2
LLM_ENDPOINT=http://ollama-service:11434

# Kubernetes API (auto-configured)
KUBERNETES_SERVICE_HOST=kubernetes.default.svc
KUBERNETES_SERVICE_PORT=443
```

### Development Configuration

```bash
# Environment
ENVIRONMENT=development
DEV_MODE=true
AUTH_ENABLED=false

# LLM Configuration
LLM_PROVIDER=mock
LLM_MODEL=test-model
LLM_ENDPOINT=http://localhost:11434
```

---

## 🧪 Testing

### Run Tests in Container

```bash
# Build test image
podman build -f Dockerfile -t holmesgpt-api-test:latest .

# Run pytest
podman run --rm holmesgpt-api-test:latest pytest -v

# Run with coverage
podman run --rm holmesgpt-api-test:latest pytest --cov=src --cov-report=term-missing
```

### Expected Output

```
tests/unit/test_health.py ............ [ 13%]
tests/unit/test_models.py ............ [ 32%]
tests/unit/test_recovery.py .......... [ 51%]
tests/unit/test_postexec.py .......... [ 70%]
tests/integration/test_sdk_integration.py .......... [100%]

104 passed in 5.23s
```

---

## 📊 Image Size

### Expected Sizes

```
Stage 1 (Builder): ~1.2 GB (includes build tools)
Stage 2 (Runtime): ~450 MB (minimal runtime only)
```

### Optimization

- Multi-stage build reduces final image size by ~60%
- Only runtime dependencies included
- No build tools, test files, or documentation in final image

---

## 🚢 Registry Push

### Push to Quay.io

```bash
# Tag for registry
podman tag kubernaut-holmesgpt-api:latest quay.io/kubernaut/kubernaut-holmesgpt-api:latest
podman tag kubernaut-holmesgpt-api:latest quay.io/kubernaut/kubernaut-holmesgpt-api:v1.0.0

# Push to registry
podman push quay.io/kubernaut/kubernaut-holmesgpt-api:latest
podman push quay.io/kubernaut/kubernaut-holmesgpt-api:v1.0.0
```

---

## 🐛 Troubleshooting

### Build Fails: "Failed to fetch HolmesGPT SDK"

**Solution**: Check network connectivity and git access:

```bash
# Test git access
git ls-remote https://github.com/robusta-dev/holmesgpt.git

# Build with verbose output
podman build --no-cache -t holmesgpt-api:latest .
```

### Container Exits Immediately

**Check logs**:
```bash
podman logs <container-id>
```

**Common issues**:
- Missing environment variables
- HolmesGPT SDK not installed correctly
- Port 8080 already in use

### Health Check Fails

**Test endpoint manually**:
```bash
podman exec -it <container-id> curl http://localhost:8080/health
```

**Check dependencies**:
```bash
podman exec -it <container-id> python -c "import src.main"
```

---

## 📚 References

- **Implementation Plan**: [IMPLEMENTATION_PLAN_V3.0.md](../docs/services/stateless/holmesgpt-api/IMPLEMENTATION_PLAN_V3.0.md)
- **Design Decision**: [DD-HOLMESGPT-012](../docs/architecture/decisions/DD-HOLMESGPT-012-Minimal-Internal-Service-Architecture.md)
- **Session Summary**: [SESSION_COMPLETE_OCT_17_2025.md](../docs/services/stateless/holmesgpt-api/docs/SESSION_COMPLETE_OCT_17_2025.md)

---

## ✅ Production Checklist

Before deploying to production:

- [ ] Build image successfully
- [ ] Run pytest (104/104 passing)
- [ ] Test health endpoint
- [ ] Test readiness endpoint
- [ ] Verify K8s ServiceAccount works
- [ ] Test TokenReviewer authentication
- [ ] Verify network policy restrictions
- [ ] Test Context API connectivity
- [ ] Test LLM provider connectivity
- [ ] Configure Prometheus metrics
- [ ] Set up log aggregation
- [ ] Configure resource limits
- [ ] Test pod restart recovery

---

**Build Complete!** 🎉

