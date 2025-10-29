# {{CONTROLLER_NAME}} Controller - Build Guide

**Controller**: {{CONTROLLER_NAME}}
**Version**: 1.0.0
**Last Updated**: 2025-10-22

---

## 📋 Table of Contents

1. [Prerequisites](#prerequisites)
2. [Local Development](#local-development)
3. [Building the Binary](#building-the-binary)
4. [Building Container Images](#building-container-images)
5. [Running Tests](#running-tests)
6. [Troubleshooting](#troubleshooting)

---

## 🔧 Prerequisites

### Required Tools

| Tool | Version | Purpose |
|---|---|---|
| Go | 1.21+ | Build controller binary |
| Docker/Podman | 20.10+ / 4.0+ | Build container images |
| kubectl | 1.28+ | Deploy to Kubernetes |
| Kind | 0.20+ | Local Kubernetes cluster |
| golangci-lint | 1.54+ | Code linting |

### Installation Commands

```bash
# Install Go (macOS)
brew install go@1.21

# Install Docker (macOS)
brew install --cask docker

# Or install Podman (preferred for rootless containers)
brew install podman
podman machine init
podman machine start

# Install kubectl
brew install kubernetes-cli

# Install Kind
brew install kind

# Install golangci-lint
brew install golangci-lint
```

### Environment Setup

```bash
# Set Go environment
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin

# Set container tool (docker or podman)
export CONTAINER_TOOL=podman

# Verify installations
go version
$CONTAINER_TOOL version
kubectl version --client
kind version
golangci-lint version
```

---

## 💻 Local Development

### 1. Clone Repository

```bash
git clone https://github.com/jordigilh/kubernaut.git
cd kubernaut
```

### 2. Install Dependencies

```bash
# Download Go modules
go mod download

# Verify dependencies
go mod verify
```

### 3. Generate Code (if using code generation)

```bash
# Generate CRD manifests and Go code
make generate

# Generate DeepCopy methods
make manifests
```

### 4. Run Controller Locally

```bash
# Ensure you have a kubeconfig pointing to a cluster
export KUBECONFIG=$HOME/.kube/config

# Run the controller
go run ./cmd/{{CONTROLLER_NAME}} \
  --config config.app/development.yaml \
  --metrics-bind-address :8080 \
  --health-probe-bind-address :8081

# Or use the Makefile
make {{CONTROLLER_NAME}}-build
./bin/{{BIN_NAME}} --config config.app/development.yaml
```

---

## 🏗️ Building the Binary

### Standard Build

```bash
# Build for current platform
make {{CONTROLLER_NAME}}-build

# Binary location
ls -lh bin/{{BIN_NAME}}
```

### Cross-Platform Build

```bash
# Build for Linux (amd64)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -a -installsuffix cgo \
  -ldflags="-w -s" \
  -o bin/{{BIN_NAME}}-linux-amd64 \
  ./cmd/{{CONTROLLER_NAME}}

# Build for Linux (arm64)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
  -a -installsuffix cgo \
  -ldflags="-w -s" \
  -o bin/{{BIN_NAME}}-linux-arm64 \
  ./cmd/{{CONTROLLER_NAME}}

# Build for macOS (amd64)
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
  -a -installsuffix cgo \
  -ldflags="-w -s" \
  -o bin/{{BIN_NAME}}-darwin-amd64 \
  ./cmd/{{CONTROLLER_NAME}}
```

### Optimized Build with Version Info

```bash
VERSION=$(git describe --tags --always --dirty)
COMMIT=$(git rev-parse HEAD)
BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')

go build \
  -ldflags="-w -s \
    -X main.Version=${VERSION} \
    -X main.Commit=${COMMIT} \
    -X main.BuildDate=${BUILD_DATE}" \
  -o bin/{{BIN_NAME}} \
  ./cmd/{{CONTROLLER_NAME}}
```

---

## 🐳 Building Container Images

### Single Architecture Build

```bash
# Build for current architecture
make {{CONTROLLER_NAME}}-docker-build

# Or manually
podman build \
  -f docker/{{IMAGE_NAME}}.Dockerfile \
  -t quay.io/jordigilh/{{IMAGE_NAME}}:v0.1.0 \
  .
```

### Multi-Architecture Build

```bash
# Build for multiple architectures (requires buildx)
make {{CONTROLLER_NAME}}-docker-build-multiarch

# Or manually with Podman
podman manifest create {{IMAGE_NAME}}:v0.1.0

podman buildx build \
  --platform linux/amd64,linux/arm64 \
  -f docker/{{IMAGE_NAME}}.Dockerfile \
  -t quay.io/jordigilh/{{IMAGE_NAME}}:v0.1.0 \
  --push \
  .
```

### Push to Registry

```bash
# Login to Quay.io
podman login quay.io

# Push image
make {{CONTROLLER_NAME}}-docker-push

# Or manually
podman push quay.io/jordigilh/{{IMAGE_NAME}}:v0.1.0
```

### Test Container Locally

```bash
# Run container with local kubeconfig
make {{CONTROLLER_NAME}}-docker-run

# Or manually
podman run --rm -it \
  -p 8080:8080 \
  -p 8081:8081 \
  -v $HOME/.kube/config:/root/.kube/config:ro \
  -e KUBECONFIG=/root/.kube/config \
  quay.io/jordigilh/{{IMAGE_NAME}}:v0.1.0
```

---

## 🧪 Running Tests

### Unit Tests

```bash
# Run all unit tests
make {{CONTROLLER_NAME}}-test

# Run with coverage
make {{CONTROLLER_NAME}}-test-coverage

# Run specific package tests
go test -v ./pkg/{{CONTROLLER_NAME}}/config/...

# Run with race detector
go test -race ./pkg/{{CONTROLLER_NAME}}/...
```

### Integration Tests

```bash
# Setup test environment (Kind cluster, dependencies)
make test-{{CONTROLLER_NAME}}-setup

# Run integration tests
make {{CONTROLLER_NAME}}-test-integration

# Cleanup test environment
make test-{{CONTROLLER_NAME}}-cleanup
```

### E2E Tests

```bash
# Deploy controller to test cluster
make {{CONTROLLER_NAME}}-deploy

# Run E2E tests
go test -v -tags=e2e ./test/e2e/{{CONTROLLER_NAME}}/...

# Check logs
make {{CONTROLLER_NAME}}-logs
```

### Test with Coverage Analysis

```bash
# Run all tests with coverage
go test -v -race -coverprofile=coverage.out -covermode=atomic \
  ./pkg/{{CONTROLLER_NAME}}/... \
  ./cmd/{{CONTROLLER_NAME}}/...

# Generate HTML coverage report
go tool cover -html=coverage.out -o coverage.html

# View coverage in browser
open coverage.html
```

---

## 🔍 Code Quality

### Linting

```bash
# Run all linters
make {{CONTROLLER_NAME}}-lint

# Run specific linters
golangci-lint run --disable-all --enable=errcheck ./pkg/{{CONTROLLER_NAME}}/...
golangci-lint run --disable-all --enable=staticcheck ./pkg/{{CONTROLLER_NAME}}/...
```

### Formatting

```bash
# Format code
make {{CONTROLLER_NAME}}-fmt

# Check formatting
gofmt -l ./pkg/{{CONTROLLER_NAME}}/ ./cmd/{{CONTROLLER_NAME}}/

# Format imports
goimports -w ./pkg/{{CONTROLLER_NAME}}/ ./cmd/{{CONTROLLER_NAME}}/
```

### Static Analysis

```bash
# Run go vet
go vet ./pkg/{{CONTROLLER_NAME}}/... ./cmd/{{CONTROLLER_NAME}}/...

# Run staticcheck
staticcheck ./pkg/{{CONTROLLER_NAME}}/... ./cmd/{{CONTROLLER_NAME}}/...

# Check for security issues
gosec ./pkg/{{CONTROLLER_NAME}}/... ./cmd/{{CONTROLLER_NAME}}/...
```

---

## 🐛 Troubleshooting

### Build Failures

#### Issue: Go module download fails

```bash
# Clear module cache
go clean -modcache

# Re-download modules
go mod download

# Verify go.sum
go mod verify
```

#### Issue: CGO errors

```bash
# Disable CGO
export CGO_ENABLED=0

# Rebuild
make {{CONTROLLER_NAME}}-build
```

### Container Build Failures

#### Issue: Build context too large

```bash
# Create .dockerignore file
cat > .dockerignore <<EOF
.git/
bin/
vendor/
*.test
coverage*.out
coverage*.html
EOF

# Rebuild
make {{CONTROLLER_NAME}}-docker-build
```

#### Issue: Multi-arch build fails

```bash
# Ensure buildx is set up
podman buildx create --use

# Verify platforms
podman buildx inspect --bootstrap

# Rebuild
make {{CONTROLLER_NAME}}-docker-build-multiarch
```

### Test Failures

#### Issue: Integration tests fail to connect to Kubernetes

```bash
# Verify Kind cluster is running
kind get clusters

# Verify kubeconfig
kubectl cluster-info

# Recreate test environment
make test-{{CONTROLLER_NAME}}-cleanup
make test-{{CONTROLLER_NAME}}-setup
make {{CONTROLLER_NAME}}-test-integration
```

#### Issue: Tests timeout

```bash
# Increase test timeout
go test -v -timeout 30m ./pkg/{{CONTROLLER_NAME}}/...

# Run with verbose output
go test -v -race ./pkg/{{CONTROLLER_NAME}}/...
```

---

## 📚 Additional Resources

- [Go Build Commands](https://go.dev/cmd/go/#hdr-Compile_packages_and_dependencies)
- [Podman Build Documentation](https://docs.podman.io/en/latest/markdown/podman-build.1.html)
- [Kubernetes Controller Runtime](https://github.com/kubernetes-sigs/controller-runtime)
- [golangci-lint Configuration](https://golangci-lint.run/usage/configuration/)

---

## 🤝 Contributing

For build-related issues or improvements:

1. Check existing issues in GitHub
2. Create detailed bug reports with build logs
3. Submit PRs with improvements to build process
4. Update this guide with new build scenarios

---

**Document Status**: ✅ **PRODUCTION-READY**
**Maintained By**: Kubernaut Development Team
