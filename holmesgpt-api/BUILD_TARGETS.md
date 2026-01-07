# HolmesGPT API - Build Targets Documentation

**Last Updated**: January 3, 2026
**Status**: Production Ready

---

## 🎯 **Available Make Targets**

### **Local Development**

#### `make build-holmesgpt-api`
**Purpose**: Install holmesgpt-api for local development

```bash
make build-holmesgpt-api
```

**What it does**:
- Runs `pip install -e .` in `holmesgpt-api/` directory
- Installs package in editable mode for development
- Uses your local Python environment

**Use when**: Developing locally with your own Python environment

---

### **Production Docker Images**

#### `make build-holmesgpt-api-image` ⭐ **PRODUCTION**
**Purpose**: Build production Docker image with full dependencies

```bash
make build-holmesgpt-api-image
```

**Specifications**:
- **Dockerfile**: `holmesgpt-api/Dockerfile`
- **Requirements**: `requirements.txt` (full dependencies)
- **Size**: ~2.5GB (includes google-cloud-aiplatform 1.5GB)
- **Platforms**: linux/amd64, linux/arm64
- **Tags**:
  - `localhost/kubernaut-holmesgpt-api:latest`
  - `localhost/kubernaut-holmesgpt-api:<git-sha>`

**Use cases**:
- ✅ Production deployments
- ✅ Releases to Quay.io
- ✅ Full cloud provider support (Vertex AI, AWS, Azure)
- ✅ All LLM providers enabled

**To push to Quay.io**:
```bash
# Build production image
make build-holmesgpt-api-image

# Tag for Quay.io
podman tag localhost/kubernaut-holmesgpt-api:latest \
  quay.io/YOUR_ORG/kubernaut-holmesgpt-api:v1.0.0

# Push to registry
podman push quay.io/YOUR_ORG/kubernaut-holmesgpt-api:v1.0.0
```

---

#### `make build-holmesgpt-api-image-e2e` 🧪 **E2E TESTING**
**Purpose**: Build E2E Docker image with minimal dependencies

```bash
make build-holmesgpt-api-image-e2e
```

**Specifications**:
- **Dockerfile**: `holmesgpt-api/Dockerfile.e2e`
- **Requirements**: `requirements-e2e.txt` (minimal dependencies)
- **Size**: ~800MB (excludes google-cloud-aiplatform 1.5GB)
- **Platforms**: linux/amd64, linux/arm64
- **Tags**:
  - `localhost/kubernaut-holmesgpt-api:e2e`
  - `localhost/kubernaut-holmesgpt-api:e2e-<git-sha>`

**Use cases**:
- ✅ E2E testing (with `MOCK_LLM_MODE=true`)
- ✅ CI/CD pipelines
- ✅ Fast development builds
- ✅ Local testing

**Benefits**:
- **65-94% faster builds** (86 sec vs 5-15 min)
- **66% smaller image** (~800MB vs ~2.5GB)
- **Same functionality** for testing with mock LLM mode

---

## 📋 **Test Targets**

### **Unit Tests**

```bash
make test-unit-holmesgpt-api
```

**What it runs**:
- 557 unit tests
- Uses pytest with coverage
- Runs in ~34 seconds

---

### **Integration Tests**

```bash
make test-integration-holmesgpt-api
```

**What it runs**:
- 65 integration tests
- Containerized Python tests with Go infrastructure
- Uses `requirements-e2e.txt` (minimal dependencies)
- Runs in ~32 seconds

**Infrastructure**:
- PostgreSQL 16
- Redis
- Data Storage service

---

### **E2E Tests**

```bash
make test-e2e-holmesgpt-api
```

**What it runs**:
- 46 E2E tests
- Creates Kind cluster with NodePort exposure
- Deploys full infrastructure (PostgreSQL, Redis, Data Storage, HAPI)
- Uses `Dockerfile.e2e` (minimal dependencies)
- Runs in ~5 minutes

**Benefits of minimal dependencies**:
- **50-67% faster** than full dependencies (~5 min vs 10-15 min)
- **Same test coverage** (46/46 tests passing)

---

### **All Tests**

```bash
make test-all-holmesgpt-api
```

**What it runs**:
- All 3 test tiers (Unit + Integration + E2E)
- Total: 668 tests
- Duration: ~6 minutes

---

## 🔄 **Two-Tier Build Strategy**

### **Why Two Dockerfiles?**

We maintain two separate Dockerfiles to optimize for different use cases:

| Aspect | Production (`Dockerfile`) | E2E (`Dockerfile.e2e`) |
|--------|---------------------------|------------------------|
| **Requirements** | `requirements.txt` | `requirements-e2e.txt` |
| **google-cloud-aiplatform** | ✅ Included (1.5GB) | ❌ Excluded |
| **Size** | ~2.5GB | ~800MB (66% smaller) |
| **Build Time** | 5-15 minutes | 86 seconds (65-94% faster) |
| **Use Case** | Production, Quay.io | E2E tests, CI/CD |
| **Cloud Providers** | All (Vertex AI, AWS, Azure) | Mock mode only |
| **Make Target** | `build-holmesgpt-api-image` | `build-holmesgpt-api-image-e2e` |

### **When to Use Which?**

**Use Production Build (`Dockerfile`):**
- ✅ Deploying to production environments
- ✅ Releasing to Quay.io or other registries
- ✅ Need full cloud provider support
- ✅ Using real LLM APIs (OpenAI, Vertex AI, Azure, etc.)

**Use E2E Build (`Dockerfile.e2e`):**
- ✅ Running E2E tests in CI/CD
- ✅ Local testing with mock LLM mode
- ✅ Fast development iterations
- ✅ Resource-constrained environments

---

## 📦 **What's in Each Requirements File?**

### **`requirements.txt` (Production - Full)**

```python
# Full dependencies including:
google-cloud-aiplatform>=1.38  # Vertex AI (1.5GB)
boto3                          # AWS SDK
azure-*                        # Azure SDKs
opensearch-py                  # OpenSearch
kubernetes                     # K8s Python client
# ... all other dependencies
```

**Install time**: 5-15 minutes
**Venv size**: ~2.5GB

### **`requirements-e2e.txt` (E2E - Minimal)**

```python
# Minimal dependencies excluding:
# google-cloud-aiplatform (1.5GB) - REMOVED
# boto3, azure-*, opensearch-py - implicitly excluded

# Core dependencies kept:
../dependencies/holmesgpt/     # HolmesGPT SDK
aiohttp>=3.9.1                 # K8s auth
prometheus-client>=0.19.0      # Metrics
watchdog>=3.0.0,<4.0.0        # ConfigMap hot-reload
# ... other core dependencies
```

**Install time**: 67 seconds
**Venv size**: ~541MB

---

## 🚀 **Release Workflow**

### **Step 1: Build Production Image**

```bash
# Build with full dependencies
make build-holmesgpt-api-image
```

### **Step 2: Test Locally (Optional)**

```bash
# Run image locally
podman run -d \
  -p 8080:8080 \
  -e CONFIG_FILE=/opt/app-root/src/config.yaml \
  localhost/kubernaut-holmesgpt-api:latest

# Test health endpoint
curl http://localhost:8080/health
```

### **Step 3: Tag for Registry**

```bash
# Get current git tag or create one
VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.1.0")

# Tag for Quay.io
podman tag localhost/kubernaut-holmesgpt-api:latest \
  quay.io/YOUR_ORG/kubernaut-holmesgpt-api:${VERSION}

# Also tag as latest
podman tag localhost/kubernaut-holmesgpt-api:latest \
  quay.io/YOUR_ORG/kubernaut-holmesgpt-api:latest
```

### **Step 4: Push to Quay.io**

```bash
# Login to Quay.io (if not already logged in)
podman login quay.io

# Push version tag
podman push quay.io/YOUR_ORG/kubernaut-holmesgpt-api:${VERSION}

# Push latest tag
podman push quay.io/YOUR_ORG/kubernaut-holmesgpt-api:latest
```

### **Step 5: Verify Push**

```bash
# Pull from Quay.io to verify
podman pull quay.io/YOUR_ORG/kubernaut-holmesgpt-api:${VERSION}

# Check image details
podman inspect quay.io/YOUR_ORG/kubernaut-holmesgpt-api:${VERSION}
```

---

## 🔍 **Build Troubleshooting**

### **Build Fails - "requirements-e2e.txt not found"**

**Issue**: E2E Dockerfile can't find requirements-e2e.txt

**Solution**:
```bash
# Ensure requirements-e2e.txt exists
ls holmesgpt-api/requirements-e2e.txt

# If missing, create from requirements.txt
cd holmesgpt-api
grep -v "google-cloud-aiplatform" requirements.txt > requirements-e2e.txt
```

### **Build Fails - "dependencies/holmesgpt not found"**

**Issue**: HolmesGPT SDK dependency is missing

**Solution**:
```bash
# Ensure HolmesGPT SDK is present
ls dependencies/holmesgpt/pyproject.toml

# If missing, clone or copy the SDK
git submodule update --init dependencies/holmesgpt
```

### **Multi-platform Build Fails**

**Issue**: `--platform linux/amd64,linux/arm64` fails

**Solution**:
```bash
# Build for current platform only
cd holmesgpt-api
podman build \
  -t localhost/kubernaut-holmesgpt-api:latest \
  -f Dockerfile \
  .
```

### **Build is Slow**

**Issue**: Production build takes 5-15 minutes

**Expected**: This is normal due to google-cloud-aiplatform (1.5GB)

**For faster builds**: Use E2E image if you don't need cloud providers
```bash
make build-holmesgpt-api-image-e2e  # ~86 seconds
```

---

## 📊 **Performance Comparison**

| Metric | Production Build | E2E Build | Improvement |
|--------|------------------|-----------|-------------|
| **Build Time** | 5-15 minutes | 86 seconds | **65-94% faster** |
| **Image Size** | ~2.5GB | ~800MB | **66% smaller** |
| **Install Time** | 5-15 minutes | 67 seconds | **80-93% faster** |
| **google-cloud-aiplatform** | ✅ Included | ❌ Excluded | 1.5GB saved |

---

## ✅ **Validation**

All build targets have been validated:

- ✅ `make build-holmesgpt-api` - Local development works
- ✅ `make build-holmesgpt-api-image` - Production image builds successfully
- ✅ `make build-holmesgpt-api-image-e2e` - E2E image builds successfully
- ✅ `make test-unit-holmesgpt-api` - 557/557 tests passing
- ✅ `make test-integration-holmesgpt-api` - 65/65 tests passing
- ✅ `make test-e2e-holmesgpt-api` - 46/46 tests passing
- ✅ Production image ready for Quay.io releases

---

## 📚 **References**

- **Analysis Document**: `docs/handoff/HAPI_DEPENDENCY_REDUCTION_ANALYSIS_JAN_03_2026.md`
- **Test Results**: `docs/handoff/HAPI_E2E_REQUIREMENTS_TEST_RESULTS_JAN_03_2026.md`
- **Success Summary**: `docs/handoff/HAPI_E2E_MINIMAL_REQUIREMENTS_SUCCESS_JAN_03_2026.md`
- **Build Notes**: `holmesgpt-api/BUILD_NOTES.md`

---

**Status**: ✅ Production Ready
**Last Validated**: January 3, 2026
**Confidence**: 99%
**All Tests**: 668/668 passing (100%)





