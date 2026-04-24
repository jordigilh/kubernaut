# HolmesGPT API - Quick Start

## ⚡ TL;DR

```bash
# Build (takes 5-15 min first time)
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
make build-holmesgpt-api

# Run
make run-holmesgpt-api

# Test (in another terminal)
curl http://localhost:8080/health

# Push to registry
make push-holmesgpt-api
```

---

## 📝 Available Make Targets

| Command | Description | Time |
|---------|-------------|------|
| `make build-holmesgpt-api` | Build container image | 5-15 min (first), 2-3 min (cached) |
| `make push-holmesgpt-api` | Push to quay.io/jordigilh | ~1 min |
| `make run-holmesgpt-api` | Run locally (dev mode) | Instant |
| `make test-holmesgpt-api` | Run tests in container | ~30 sec |

---

## 🔧 Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `HOLMESGPT_VERSION` | `latest` | Image tag version |
| `HOLMESGPT_REGISTRY` | `quay.io/jordigilh` | Container registry |
| `HOLMESGPT_IMAGE_NAME` | `kubernaut-holmesgpt-api` | Image name |

### Custom Build

```bash
# Build with specific version
make build-holmesgpt-api HOLMESGPT_VERSION=v1.0.0

# Build and push with custom registry
make build-holmesgpt-api HOLMESGPT_REGISTRY=docker.io/myorg
make push-holmesgpt-api HOLMESGPT_REGISTRY=docker.io/myorg
```

---

## 🐛 Common Issues

### Issue 1: Build is slow/hanging

**Cause**: HolmesGPT SDK has 85+ dependencies
**Solution**: This is normal - first build takes 5-15 minutes

### Issue 2: Out of memory

**Solution**:
```bash
podman machine set --memory 8192
podman machine restart
```

### Issue 3: Network timeout during pip install

**Solution**:
```bash
# Increase pip timeout
pip install --timeout=300 -r requirements.txt
```

---

## ✅ Verification

After build completes:

```bash
# 1. Check image exists
podman images | grep holmesgpt

# Expected output:
# kubernaut-holmesgpt-api  latest  abc123  5 minutes ago  450MB

# 2. Run container
make run-holmesgpt-api

# 3. Test health endpoint (in another terminal)
curl http://localhost:8080/health

# Expected output:
# {"status":"healthy","service":"holmesgpt-api",...}

# 4. Test recovery endpoint
curl -X POST http://localhost:8080/api/v1/recovery/analyze \
  -H "Content-Type: application/json" \
  -d '{"incident_id":"test-001","failed_action":{"type":"test"},"failure_context":{}}'

# Expected: 200 OK with recovery strategies
```

---

## 📦 What Gets Built

### Final Image Contents

```
kubernaut-holmesgpt-api:latest
├── Python 3.11 (Red Hat UBI9)
├── HolmesGPT SDK (from git)
│   └── 85+ dependencies
├── FastAPI application
│   ├── src/main.py
│   ├── src/middleware/
│   ├── src/extensions/
│   └── src/models/
└── Configuration
    ├── Health checks
    ├── Prometheus metrics
    └── Structured logging
```

### Image Size

- **Builder stage**: ~1.2 GB (temporary)
- **Runtime stage**: ~450-500 MB (final)

---

## 🚀 Next Steps

1. **Build the image**: `make build-holmesgpt-api`
2. **Test locally**: `make run-holmesgpt-api`
3. **Push to registry**: `make push-holmesgpt-api`
4. **Deploy to K8s**: `kubectl apply -f deployment.yaml`

---

## 📚 More Information

- **Full Build Guide**: [BUILD_NOTES.md](BUILD_NOTES.md)
- **Docker Details**: [DOCKER_README.md](DOCKER_README.md)
- **Service README**: [README.md](README.md)
- **Implementation Plan**: [../docs/services/stateless/holmesgpt-api/IMPLEMENTATION_PLAN_V3.0.md](../docs/services/stateless/holmesgpt-api/IMPLEMENTATION_PLAN_V3.0.md)

