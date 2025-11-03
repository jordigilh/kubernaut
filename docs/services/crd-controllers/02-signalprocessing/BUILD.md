# RemediationProcessor Controller - Build Guide

**Version**: 1.0.0
**Last Updated**: October 21, 2025
**Maintainer**: kubernaut-dev@jordigilh.com

---

## Table of Contents

1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Local Development Setup](#local-development-setup)
4. [Building the Binary](#building-the-binary)
5. [Building the Container Image](#building-the-container-image)
6. [Multi-Architecture Builds](#multi-architecture-builds)
7. [Running Locally](#running-locally)
8. [Testing](#testing)
9. [Troubleshooting](#troubleshooting)

---

## Overview

The RemediationProcessor controller is a Kubernetes CRD controller responsible for:

- **Purpose**: Processing and enriching remediation requests with historical context, semantic classification, and deduplication
- **CRD**: `RemediationProcessing.remediation.kubernaut.io/v1alpha1`
- **Language**: Go 1.24+
- **Framework**: controller-runtime
- **Dependencies**: PostgreSQL (Data Storage), Context API (Historical Intelligence)

### Key Features
- **Context Enrichment**: Augments remediation requests with historical patterns from Context API
- **Semantic Classification**: Groups similar issues using configurable similarity thresholds
- **Deduplication**: Identifies and merges duplicate remediation requests within time windows
- **Pattern Recognition**: Learns from historical remediation outcomes stored in PostgreSQL

---

## Prerequisites

### Required Tools

| Tool | Minimum Version | Purpose |
|------|----------------|---------|
| **Go** | 1.24+ | Build controller binary |
| **Podman/Docker** | 20.10+ | Build container images |
| **kubectl** | 1.28+ | Deploy to Kubernetes |
| **make** | 4.0+ | Build automation |
| **PostgreSQL** | 14+ | Data storage (for local testing) |

### Optional Tools

| Tool | Purpose |
|------|---------|
| **kind** | Local Kubernetes cluster |
| **golangci-lint** | Code linting |
| **ginkgo** | Test execution |

### Installation Commands

```bash
# Go (via package manager)
# macOS
brew install go@1.24

# Linux
wget https://go.dev/dl/go1.24.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Podman (recommended for UBI9 images)
# macOS
brew install podman

# Linux
sudo dnf install -y podman  # Fedora/RHEL
sudo apt-get install -y podman  # Debian/Ubuntu

# kubectl
# macOS
brew install kubectl

# Linux
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

# kind (optional)
brew install kind  # macOS
# or
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64
chmod +x ./kind && sudo mv ./kind /usr/local/bin/kind
```

---

## Local Development Setup

### 1. Clone Repository

```bash
git clone https://github.com/jordigilh/kubernaut.git
cd kubernaut
```

### 2. Install Go Dependencies

```bash
go mod download
go mod verify
```

### 3. Setup Local PostgreSQL (for testing)

```bash
# Using Docker/Podman
podman run -d \
  --name remediationprocessor-postgres \
  -e POSTGRES_USER=remediation_user \
  -e POSTGRES_PASSWORD=changeme \
  -e POSTGRES_DATABASE=kubernaut_remediation \
  -p 5432:5432 \
  postgres:14-alpine

# Verify connection
psql -h localhost -U remediation_user -d kubernaut_remediation -c "SELECT version();"
```

### 4. Setup Context API (required dependency)

```bash
# If Context API is not already running
make deploy-context-api
```

### 5. Verify Installation

```bash
go version         # should show 1.24+
podman --version   # should show 20.10+
kubectl version --client
make --version
psql --version     # should show 14+
```

---

## Building the Binary

### Quick Build

```bash
# Build for current platform
make build-remediationprocessor
```

This creates `bin/remediationprocessor` with default settings.

### Custom Build

```bash
# Build with specific Go flags
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -o bin/remediationprocessor \
  -ldflags="-X github.com/jordigilh/kubernaut/internal/version.Version=v0.1.0" \
  ./cmd/remediationprocessor
```

### Build Options

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `CGO_ENABLED` | 0 | Disable CGO for static binary |
| `GOOS` | (auto) | Target OS (linux, darwin, windows) |
| `GOARCH` | (auto) | Target architecture (amd64, arm64) |

### Verify Build

```bash
# Check binary
ls -lh bin/remediationprocessor
file bin/remediationprocessor

# Run version check
./bin/remediationprocessor --version
```

---

## Building the Container Image

### Prerequisites

- Container tool (Podman recommended for UBI9)
- Access to Red Hat UBI9 registry (public, no auth needed)

### Single-Architecture Build

```bash
# Build for current platform
make docker-build-remediationprocessor-single

# This creates: quay.io/jordigilh/remediationprocessor:v0.1.0-$(uname -m)
```

### Custom Build

```bash
# Build with specific tag
podman build \
  -f docker/remediationprocessor.Dockerfile \
  -t myregistry/remediationprocessor:custom \
  --build-arg VERSION=v0.1.0 \
  --build-arg COMMIT=$(git rev-parse HEAD) \
  --build-arg BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ') \
  .
```

### Inspect Image

```bash
# Check image details
podman inspect quay.io/jordigilh/remediationprocessor:v0.1.0-$(uname -m)

# Check image size
podman images | grep remediationprocessor

# Expected size: ~150-200MB
```

### Test Container Locally

```bash
# Run container with environment variables
make docker-run-remediationprocessor

# Or manually:
podman run -d --rm \
  --name remediationprocessor-test \
  -p 8080:8080 \
  -p 8081:8081 \
  -e POSTGRES_HOST=host.containers.internal \
  -e POSTGRES_PORT=5432 \
  -e POSTGRES_USER=remediation_user \
  -e POSTGRES_PASSWORD=changeme \
  -e POSTGRES_DATABASE=kubernaut_remediation \
  -e CONTEXT_API_ENDPOINT=http://context-api:8080 \
  quay.io/jordigilh/remediationprocessor:v0.1.0-$(uname -m)

# Check health
curl http://localhost:8081/healthz
curl http://localhost:8081/readyz

# Check metrics
curl http://localhost:8080/metrics | grep remediationprocessor

# Stop container
make docker-stop-remediationprocessor
```

---

## Multi-Architecture Builds

### Prerequisites

- Podman with `buildx` support or Docker Buildx
- QEMU for cross-platform emulation (installed automatically with Podman)

### Build Multi-Arch Image

```bash
# Build for linux/amd64 and linux/arm64
make docker-build-remediationprocessor-multiarch

# This creates a manifest list supporting both architectures
```

### Manual Multi-Arch Build

```bash
# Using Podman
podman build --platform linux/amd64,linux/arm64 \
  -f docker/remediationprocessor.Dockerfile \
  -t quay.io/jordigilh/remediationprocessor:v0.1.0 \
  --build-arg VERSION=v0.1.0 \
  .

# Inspect manifest
podman manifest inspect quay.io/jordigilh/remediationprocessor:v0.1.0
```

### Push to Registry

```bash
# Login to registry
podman login quay.io

# Push multi-arch image
make docker-push-remediationprocessor

# Or manually:
podman manifest push \
  quay.io/jordigilh/remediationprocessor:v0.1.0 \
  docker://quay.io/jordigilh/remediationprocessor:v0.1.0
```

### Verify Multi-Arch

```bash
# Check manifest
podman manifest inspect quay.io/jordigilh/remediationprocessor:v0.1.0

# Expected output should show:
# - linux/amd64
# - linux/arm64
```

---

## Running Locally

### Option 1: Binary with Local Config

```bash
# Create local config
cat > /tmp/remediationprocessor-config.yaml <<EOF
namespace: kubernaut-system
metrics_address: ":8080"
health_address: ":8081"
leader_election: false
log_level: debug
max_concurrency: 10

kubernetes:
  qps: 20.0
  burst: 30

data_storage:
  postgres_host: localhost
  postgres_port: 5432
  postgres_user: remediation_user
  postgres_password: changeme
  postgres_database: kubernaut_remediation
  ssl_mode: disable

context:
  endpoint: http://localhost:8091
  timeout: 30
  max_retries: 3

classification:
  semantic_threshold: 0.85
  time_window_minutes: 60
  similarity_engine: cosine
  batch_size: 100
EOF

# Run controller
./bin/remediationprocessor \
  --config=/tmp/remediationprocessor-config.yaml \
  --leader-elect=false

# Expected output:
# INFO starting remediationprocessor controller
# INFO starting manager
```

### Option 2: Container with Docker/Podman

```bash
# Run using Makefile target
make docker-run-remediationprocessor

# View logs
make docker-logs-remediationprocessor

# Stop container
make docker-stop-remediationprocessor
```

### Health Checks

```bash
# Health endpoint
curl http://localhost:8081/healthz
# Expected: ok

# Readiness endpoint
curl http://localhost:8081/readyz
# Expected: ok

# Metrics endpoint
curl http://localhost:8080/metrics
# Expected: Prometheus metrics output
```

---

## Testing

### Unit Tests

```bash
# Run all unit tests
make test-remediationprocessor-unit

# Or manually:
go test -v -race -coverprofile=coverage.out \
  ./pkg/signalprocessing/...

# View coverage
go tool cover -html=coverage.out
```

### Integration Tests

```bash
# Prerequisites: Kind cluster, PostgreSQL, Context API
make test-remediationprocessor-integration

# Or manually:
go test -v -race \
  -tags=integration \
  ./test/integration/remediationprocessor/...
```

### Test Coverage Targets

| Package | Target Coverage |
|---------|----------------|
| `pkg/signalprocessing/config` | 100% (achieved) |
| `pkg/signalprocessing/controllers` | 80%+ |
| `pkg/signalprocessing/enrichment` | 85%+ |
| `pkg/signalprocessing/classification` | 85%+ |

---

## Troubleshooting

### Build Issues

#### Issue: `go build` fails with missing dependencies
**Solution**:
```bash
go mod tidy
go mod download
go mod verify
```

#### Issue: Container build fails with permission denied
**Solution**:
```bash
# Check Podman/Docker is running
podman version

# Use correct working directory (UBI9)
# Dockerfile should use /opt/app-root/src
grep WORKDIR docker/remediationprocessor.Dockerfile
```

#### Issue: Binary won't compile on macOS for linux/amd64
**Solution**:
```bash
# Explicitly set target platform
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -o bin/remediationprocessor-linux-amd64 \
  ./cmd/remediationprocessor
```

### Runtime Issues

#### Issue: Controller fails to start with "unable to load configuration"
**Solution**:
```bash
# Check config file exists and is readable
ls -l /etc/remediationprocessor/config.yaml

# Verify YAML syntax
yamllint /etc/remediationprocessor/config.yaml

# Check environment variable overrides
env | grep -E '(POSTGRES|CONTEXT|SEMANTIC)'
```

#### Issue: "PostgreSQL connection refused"
**Solution**:
```bash
# Check PostgreSQL is running
podman ps | grep postgres

# Test connection manually
psql -h localhost -U remediation_user -d kubernaut_remediation -c "SELECT 1;"

# Check firewall rules
sudo firewall-cmd --list-ports | grep 5432

# Verify config
grep postgres_host /etc/remediationprocessor/config.yaml
```

#### Issue: "Context API unreachable"
**Solution**:
```bash
# Check Context API is running
kubectl get pods -n kubernaut-system | grep context-api

# Test endpoint manually
curl http://context-api.kubernaut-system.svc.cluster.local:8080/health

# Check network policy
kubectl get networkpolicy -n kubernaut-system

# Verify DNS resolution
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nslookup context-api.kubernaut-system.svc.cluster.local
```

#### Issue: High memory usage
**Solution**:
```bash
# Check max_concurrency setting
grep max_concurrency /etc/remediationprocessor/config.yaml

# Reduce concurrency
export MAX_CONCURRENCY=5

# Check batch_size for classification
grep batch_size /etc/remediationprocessor/config.yaml

# Reduce batch size
export CLASSIFICATION_BATCH_SIZE=50

# Monitor memory
kubectl top pod -n kubernaut-system -l app=remediationprocessor
```

### Test Issues

#### Issue: Unit tests fail with "config validation error"
**Solution**:
```bash
# Run specific test with verbose output
go test -v ./pkg/signalprocessing/config -run TestValidateConfig

# Check test fixtures have all required fields
grep -r "postgres_password" pkg/signalprocessing/config/config_test.go
```

#### Issue: Integration tests timeout
**Solution**:
```bash
# Increase timeout
go test -v -timeout=10m \
  -tags=integration \
  ./test/integration/remediationprocessor/...

# Check dependencies are running
kubectl get pods -n kubernaut-system
podman ps | grep postgres

# Run tests with debug logging
LOG_LEVEL=debug go test -v ...
```

---

## Quick Reference

### Common Commands

```bash
# Build
make build-remediationprocessor

# Test
make test-remediationprocessor-unit

# Container
make docker-build-remediationprocessor
make docker-run-remediationprocessor
make docker-stop-remediationprocessor

# Deploy
make deploy-remediationprocessor

# Status
make status-remediationprocessor
make logs-remediationprocessor

# Clean
make clean-remediationprocessor
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CONTROLLER_NAMESPACE` | kubernaut-system | Controller namespace |
| `POSTGRES_HOST` | - | PostgreSQL host (required) |
| `POSTGRES_PORT` | 5432 | PostgreSQL port |
| `POSTGRES_USER` | - | PostgreSQL user (required) |
| `POSTGRES_PASSWORD` | - | PostgreSQL password (required) |
| `POSTGRES_DATABASE` | - | PostgreSQL database (required) |
| `CONTEXT_API_ENDPOINT` | - | Context API endpoint (required) |
| `SEMANTIC_THRESHOLD` | 0.85 | Similarity threshold (0.0-1.0) |
| `TIME_WINDOW_MINUTES` | 60 | Deduplication time window |
| `LOG_LEVEL` | info | Log level (debug, info, warn, error) |

---

**End of Build Guide**










