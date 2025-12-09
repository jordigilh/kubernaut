# SignalProcessing Service - Build Guide

**Version**: 1.0
**Last Updated**: December 9, 2025
**Related**: [IMPLEMENTATION_PLAN_V1.31](IMPLEMENTATION_PLAN_V1.31.md)

---

## Overview

This document describes how to build, test, and develop the SignalProcessing CRD controller.

---

## Prerequisites

### Required Tools

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.21+ | Language runtime |
| Docker | 20.10+ | Container builds |
| kubectl | 1.28+ | Kubernetes CLI |
| kubebuilder | 3.14+ | CRD scaffolding |
| make | 4.0+ | Build automation |

### Optional Tools

| Tool | Version | Purpose |
|------|---------|---------|
| Kind | 0.20+ | Local K8s cluster for E2E tests |
| golangci-lint | 1.55+ | Linting |
| Ginkgo | 2.13+ | BDD test runner |

---

## Quick Start

```bash
# Clone the repository
git clone https://github.com/jordigilh/kubernaut.git
cd kubernaut

# Install dependencies
go mod download

# Build the controller
make build-signalprocessing

# Run unit tests
go test ./test/unit/signalprocessing/... -v

# Run integration tests (requires ENVTEST)
make test-integration-signalprocessing

# Run E2E tests (requires Kind)
make test-e2e-signalprocessing
```

---

## Build Commands

### Binary Build

```bash
# Build only SignalProcessing
go build -o bin/signalprocessing ./cmd/signalprocessing

# Build with version info
go build -ldflags "-X main.Version=$(git describe --tags)" \
  -o bin/signalprocessing ./cmd/signalprocessing

# Cross-compile for Linux (production)
GOOS=linux GOARCH=amd64 go build -o bin/signalprocessing-linux-amd64 \
  ./cmd/signalprocessing
```

### Container Build

```bash
# Build container image
docker build -t signalprocessing:latest \
  -f build/Dockerfile.signalprocessing .

# Build multi-arch image
docker buildx build --platform linux/amd64,linux/arm64 \
  -t signalprocessing:latest \
  -f build/Dockerfile.signalprocessing .
```

### CRD Generation

```bash
# Generate CRD manifests
make manifests

# Generate DeepCopy methods
make generate

# Verify generated code is up-to-date
make verify-codegen
```

---

## Project Structure

```
kubernaut/
├── api/signalprocessing/v1alpha1/   # CRD types
│   ├── signalprocessing_types.go    # Spec/Status definitions
│   ├── groupversion_info.go         # API group registration
│   └── zz_generated.deepcopy.go     # Generated DeepCopy
├── cmd/signalprocessing/            # Binary entry point
│   └── main.go
├── internal/controller/signalprocessing/
│   └── signalprocessing_controller.go  # Reconciler
├── pkg/signalprocessing/            # Business logic
│   ├── audit/                       # BR-SP-090: Audit client
│   ├── cache/                       # TTL cache
│   ├── classifier/                  # Environment, Priority, Business
│   ├── config/                      # Configuration
│   ├── detection/                   # Label detection
│   ├── enricher/                    # K8s context enrichment
│   ├── metrics/                     # DD-005 metrics
│   ├── ownerchain/                  # Owner chain builder
│   └── rego/                        # Rego policy engine
├── test/
│   ├── unit/signalprocessing/       # 194 unit tests
│   ├── integration/signalprocessing/ # 65 integration tests
│   └── e2e/signalprocessing/        # 11 E2E tests
└── config/
    └── crd/bases/                   # Generated CRD YAML
```

---

## Testing

### Unit Tests

```bash
# Run all unit tests
go test ./test/unit/signalprocessing/... -v

# Run specific test file
go test ./test/unit/signalprocessing/enricher_test.go -v

# Run with coverage
go test ./test/unit/signalprocessing/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Integration Tests (ENVTEST)

```bash
# Setup ENVTEST binaries (one-time)
make envtest
export KUBEBUILDER_ASSETS=$(./bin/setup-envtest use -p path)

# Run integration tests
go test ./test/integration/signalprocessing/... -v -timeout 10m
```

### E2E Tests (Kind)

```bash
# Create Kind cluster
kind create cluster --name signalprocessing-e2e \
  --kubeconfig ~/.kube/signalprocessing-e2e-config

# Run E2E tests
KUBECONFIG=~/.kube/signalprocessing-e2e-config \
  go test ./test/e2e/signalprocessing/... -v -timeout 30m

# Cleanup
kind delete cluster --name signalprocessing-e2e
```

---

## Development Workflow

### TDD Workflow (APDC)

Per [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc):

1. **Analysis**: Understand the business requirement (BR-SP-XXX)
2. **Plan**: Design the test cases and implementation approach
3. **Do-RED**: Write failing tests first
4. **Do-GREEN**: Implement minimal code to pass tests
5. **Do-REFACTOR**: Improve code quality
6. **Check**: Verify all tests pass and coverage is adequate

### Local Development

```bash
# Run controller locally (against current kubeconfig)
go run ./cmd/signalprocessing/main.go

# Run with debug logging
go run ./cmd/signalprocessing/main.go --zap-log-level=debug

# Run with custom config
CONFIG_PATH=/path/to/config.yaml go run ./cmd/signalprocessing/main.go
```

---

## Code Quality

### Linting

```bash
# Run golangci-lint
golangci-lint run ./...

# Fix auto-fixable issues
golangci-lint run --fix ./...
```

### Formatting

```bash
# Format code
go fmt ./...
goimports -w .

# Verify formatting
make verify-fmt
```

### Security Scanning

```bash
# Scan for vulnerabilities
govulncheck ./...

# Scan container image
trivy image signalprocessing:latest
```

---

## Dependencies

### Key Dependencies

| Package | Purpose |
|---------|---------|
| `sigs.k8s.io/controller-runtime` | Controller framework |
| `k8s.io/client-go` | Kubernetes client |
| `github.com/open-policy-agent/opa` | Rego policy engine |
| `github.com/go-logr/logr` | Structured logging (DD-005) |
| `github.com/prometheus/client_golang` | Metrics |
| `github.com/onsi/ginkgo/v2` | BDD testing |
| `github.com/onsi/gomega` | Test matchers |

### Updating Dependencies

```bash
# Update all dependencies
go get -u ./...
go mod tidy

# Update specific dependency
go get -u github.com/open-policy-agent/opa@latest
```

---

## CI/CD Integration

### GitHub Actions

```yaml
# .github/workflows/signalprocessing.yaml
name: SignalProcessing CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      - run: make test-unit-signalprocessing
      - run: make test-integration-signalprocessing
```

---

## Troubleshooting

### Common Build Issues

| Issue | Solution |
|-------|----------|
| `cannot find module` | Run `go mod download` |
| `undefined: controller.Manager` | Update controller-runtime version |
| `CRD not found` | Run `make manifests generate` |
| `ENVTEST not found` | Run `make envtest` and set `KUBEBUILDER_ASSETS` |

### Test Issues

| Issue | Solution |
|-------|----------|
| Integration tests timeout | Increase timeout: `-timeout 15m` |
| E2E tests fail to connect | Verify Kind cluster is running |
| Tests pass locally but fail in CI | Check for race conditions: `-race` |

---

## References

- [DD-006: Controller Scaffolding Strategy](../../../architecture/decisions/DD-006-controller-scaffolding-strategy.md)
- [DD-005: Observability Standards](../../../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md)
- [TESTING_GUIDELINES.md](../../../development/business-requirements/TESTING_GUIDELINES.md)
- [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc)

