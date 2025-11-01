# Data Storage Service - Docker Build Instructions

**Dockerfile**: `docker/datastorage-ubi9.Dockerfile`  
**Base Image**: Red Hat UBI9 (ADR-027)  
**Architectures**: linux/amd64, linux/arm64

---

## 📦 **MAKEFILE TARGETS**

Add these targets to the main `Makefile`:

```makefile
# =============================================================================
# Data Storage Service - Docker Build Targets
# =============================================================================

# Variables
DATASTORAGE_IMAGE_NAME := quay.io/jordigilh/data-storage
DATASTORAGE_VERSION := v1.0.0
DATASTORAGE_DOCKERFILE := docker/datastorage-ubi9.Dockerfile

# Detect host architecture
GOARCH := $(shell go env GOARCH)
GOOS := $(shell go env GOOS)

# Single-architecture build (for local development)
.PHONY: docker-build-datastorage-single
docker-build-datastorage-single:
	@echo "Building Data Storage Service Docker image for $(GOARCH)..."
	docker build \
		--build-arg GOARCH=$(GOARCH) \
		--build-arg GOOS=$(GOOS) \
		--build-arg VERSION=$(DATASTORAGE_VERSION) \
		-t $(DATASTORAGE_IMAGE_NAME):$(DATASTORAGE_VERSION)-$(GOARCH) \
		-f $(DATASTORAGE_DOCKERFILE) \
		.
	@echo "✅ Built: $(DATASTORAGE_IMAGE_NAME):$(DATASTORAGE_VERSION)-$(GOARCH)"

# Multi-architecture build (for production)
.PHONY: docker-build-datastorage-multi
docker-build-datastorage-multi:
	@echo "Building Data Storage Service Docker image for multiple architectures..."
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(DATASTORAGE_VERSION) \
		-t $(DATASTORAGE_IMAGE_NAME):$(DATASTORAGE_VERSION) \
		-t $(DATASTORAGE_IMAGE_NAME):latest \
		-f $(DATASTORAGE_DOCKERFILE) \
		--push \
		.
	@echo "✅ Built and pushed multi-arch image: $(DATASTORAGE_IMAGE_NAME):$(DATASTORAGE_VERSION)"

# Push single-architecture image
.PHONY: docker-push-datastorage
docker-push-datastorage:
	@echo "Pushing Data Storage Service Docker image..."
	docker push $(DATASTORAGE_IMAGE_NAME):$(DATASTORAGE_VERSION)-$(GOARCH)
	@echo "✅ Pushed: $(DATASTORAGE_IMAGE_NAME):$(DATASTORAGE_VERSION)-$(GOARCH)"

# Run Data Storage Service in Docker (for local testing)
.PHONY: docker-run-datastorage
docker-run-datastorage:
	@echo "Running Data Storage Service in Docker..."
	docker run -d \
		--name datastorage \
		-p 8080:8080 \
		-p 9090:9090 \
		-e DB_HOST=host.docker.internal \
		-e DB_PORT=5432 \
		-e DB_NAME=action_history \
		-e DB_USER=postgres \
		-e DB_PASSWORD=test \
		-e LOG_LEVEL=info \
		$(DATASTORAGE_IMAGE_NAME):$(DATASTORAGE_VERSION)-$(GOARCH)
	@echo "✅ Data Storage Service running on http://localhost:8080"
	@echo "   Metrics: http://localhost:9090/metrics"

# Stop and remove Data Storage Service container
.PHONY: docker-stop-datastorage
docker-stop-datastorage:
	@echo "Stopping Data Storage Service..."
	docker stop datastorage || true
	docker rm datastorage || true
	@echo "✅ Data Storage Service stopped"

# Build and run (convenience target)
.PHONY: docker-dev-datastorage
docker-dev-datastorage: docker-build-datastorage-single docker-run-datastorage
	@echo "✅ Data Storage Service built and running"
```

---

## 🚀 **USAGE EXAMPLES**

### **Local Development (Single Architecture)**

```bash
# Build for your current architecture
make docker-build-datastorage-single

# Build and run
make docker-dev-datastorage

# Check logs
docker logs -f datastorage

# Stop
make docker-stop-datastorage
```

### **Production (Multi-Architecture)**

```bash
# Prerequisites: Docker Buildx installed
docker buildx create --name multiarch --use

# Build and push multi-arch image
make docker-build-datastorage-multi

# This will build for both amd64 and arm64, and push to registry
```

### **Manual Build Commands**

```bash
# Build for amd64
docker build \
  --build-arg GOARCH=amd64 \
  --build-arg VERSION=v1.0.0 \
  -t quay.io/jordigilh/data-storage:v1.0.0-amd64 \
  -f docker/datastorage-ubi9.Dockerfile \
  .

# Build for arm64 (on M1/M2 Mac or ARM server)
docker build \
  --build-arg GOARCH=arm64 \
  --build-arg VERSION=v1.0.0 \
  -t quay.io/jordigilh/data-storage:v1.0.0-arm64 \
  -f docker/datastorage-ubi9.Dockerfile \
  .
```

---

## 🔍 **VERIFICATION**

### **Verify Architecture**

```bash
# Check image architecture
docker inspect quay.io/jordigilh/data-storage:v1.0.0-amd64 | grep Architecture

# Run architecture check
docker run --rm quay.io/jordigilh/data-storage:v1.0.0-amd64 uname -m
# Expected: x86_64 (for amd64) or aarch64 (for arm64)
```

### **Verify Binary**

```bash
# Check binary is statically linked (no C dependencies)
docker run --rm quay.io/jordigilh/data-storage:v1.0.0-amd64 ldd /data-storage
# Expected: "not a dynamic executable" (good - static binary)

# Check binary size
docker run --rm quay.io/jordigilh/data-storage:v1.0.0-amd64 ls -lh /data-storage
# Expected: ~20-30MB (Go binary is typically small)
```

### **Verify Image Size**

```bash
# Check image size
docker images | grep data-storage
# Expected: ~100-200MB (UBI-micro is very small)
```

### **Test Health Check**

```bash
# Run container
docker run -d --name datastorage-test quay.io/jordigilh/data-storage:v1.0.0-amd64

# Wait 5 seconds for startup
sleep 5

# Check health status
docker inspect datastorage-test | grep -A5 Health
# Expected: "Status": "healthy"

# Cleanup
docker stop datastorage-test && docker rm datastorage-test
```

---

## 🔒 **SECURITY CONSIDERATIONS**

### **Non-Root User**
- ✅ Runs as UID 1001 (non-root)
- ✅ Uses `/sbin/nologin` shell (no shell access)

### **Minimal Attack Surface**
- ✅ Based on UBI-micro (smallest Red Hat base)
- ✅ No package manager in runtime image
- ✅ Static binary (no shared libraries)

### **Image Scanning**

```bash
# Scan with Trivy
trivy image quay.io/jordigilh/data-storage:v1.0.0-amd64

# Scan with Clair
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
  arminc/clair-scanner --clair=http://clair:6060 \
  quay.io/jordigilh/data-storage:v1.0.0-amd64
```

---

## 📊 **COMPARISON: UBI9 vs Alpine vs Scratch**

| Feature | UBI9-micro | Alpine | Scratch |
|---------|------------|--------|---------|
| **Size** | ~20MB | ~5MB | 0MB |
| **Security Updates** | ✅ Red Hat | ✅ Alpine | ❌ None |
| **Enterprise Support** | ✅ Yes | ❌ No | ❌ No |
| **Compliance** | ✅ FIPS, STIGs | ⚠️ Limited | ❌ None |
| **Debugging** | ⚠️ Limited | ✅ Good | ❌ None |
| **Package Manager** | ❌ (micro) | ✅ apk | ❌ None |

**Recommendation**: UBI9-micro for production (security + compliance + support)

---

## 🎯 **INTEGRATION WITH CI/CD**

### **GitHub Actions Workflow**

```yaml
name: Build and Push Data Storage Service

on:
  push:
    branches: [main]
    tags: ['v*']

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v2
      
      - name: Login to Quay.io
        uses: docker/login-action@v2
        with:
          registry: quay.io
          username: ${{ secrets.QUAY_USERNAME }}
          password: ${{ secrets.QUAY_PASSWORD }}
      
      - name: Extract version
        id: version
        run: echo "VERSION=${GITHUB_REF#refs/tags/}" >> $GITHUB_OUTPUT
      
      - name: Build and push multi-arch
        uses: docker/build-push-action@v4
        with:
          context: .
          file: docker/datastorage-ubi9.Dockerfile
          platforms: linux/amd64,linux/arm64
          push: true
          tags: |
            quay.io/jordigilh/data-storage:${{ steps.version.outputs.VERSION }}
            quay.io/jordigilh/data-storage:latest
          build-args: |
            VERSION=${{ steps.version.outputs.VERSION }}
```

---

**Date**: November 2, 2025  
**Dockerfile**: `docker/datastorage-ubi9.Dockerfile`  
**Status**: ✅ Production-Ready

