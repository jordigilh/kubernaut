# Platform-Agnostic Test Execution Environment

## Goal
Create a **consistent, reproducible test environment** that works identically:
- ✅ On developer laptops (macOS, Linux, Windows)
- ✅ In CI/CD pipelines (GitHub Actions, GitLab CI, Jenkins)
- ✅ In different cloud environments (AWS, GCP, Azure)
- ✅ With different container runtimes (Docker, Podman, containerd)

---

## Solution: Container-Based Test Execution

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Host Environment                          │
│                  (macOS, Linux, Windows)                     │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │         Container Runtime (Docker/Podman)               │ │
│  │                                                          │ │
│  │  ┌────────────────────────────────────────────────────┐ │ │
│  │  │     Test Container (golang:1.21)                   │ │ │
│  │  │                                                     │ │ │
│  │  │  • Go toolchain                                    │ │ │
│  │  │  • Test dependencies                               │ │ │
│  │  │  • envtest (fake K8s API)                          │ │ │
│  │  │  • Your code                                       │ │ │
│  │  │                                                     │ │ │
│  │  │  Runs: Unit + Integration Tests                    │ │ │
│  │  └────────────────────────────────────────────────────┘ │ │
│  │                                                          │ │
│  │  ┌────────────────────────────────────────────────────┐ │ │
│  │  │     Service Containers                             │ │ │
│  │  │  • PostgreSQL (pgvector)                           │ │ │
│  │  │  • Redis                                           │ │ │
│  │  └────────────────────────────────────────────────────┘ │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘

For E2E Tests (Kind):
┌─────────────────────────────────────────────────────────────┐
│                    Host Environment                          │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │         Container Runtime                               │ │
│  │                                                          │ │
│  │  ┌────────────────────────────────────────────────────┐ │ │
│  │  │     Test Container                                 │ │ │
│  │  │  • Go + Kind CLI                                   │ │ │
│  │  │  • Access to host Docker socket                    │ │ │
│  │  │                                                     │ │ │
│  │  │  Creates Kind cluster on host Docker              │ │ │
│  │  └────────────────────────────────────────────────────┘ │ │
│  │                                                          │ │
│  │  ┌────────────────────────────────────────────────────┐ │ │
│  │  │     Kind Cluster (on host Docker)                 │ │ │
│  │  │  • Control plane container                         │ │ │
│  │  │  • Worker node containers                          │ │ │
│  │  └────────────────────────────────────────────────────┘ │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

---

## Implementation

### 1. Docker Compose for Local Development

**File**: `docker-compose.test.yml`

```yaml
version: '3.8'

services:
  # Unit Tests - Fast, no dependencies
  unit-tests:
    image: golang:1.21
    volumes:
      - .:/workspace:ro
      - go-cache:/go/pkg/mod
    working_dir: /workspace
    command: make test
    environment:
      - CGO_ENABLED=0
      - GOCACHE=/go/pkg/mod/cache

  # Integration Tests - With service dependencies
  integration-tests:
    image: golang:1.21
    volumes:
      - .:/workspace:ro
      - go-cache:/go/pkg/mod
    working_dir: /workspace
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    environment:
      - POSTGRES_HOST=postgres
      - POSTGRES_PORT=5432
      - POSTGRES_USER=postgres
      - POSTGRES_PASSWORD=postgres
      - POSTGRES_DB=kubernaut_test
      - REDIS_HOST=redis
      - REDIS_PORT=6379
      - CGO_ENABLED=1  # Required for envtest
    command: make test-integration-service-all

  # E2E Tests - With Kind cluster
  e2e-tests:
    image: golang:1.21
    volumes:
      - .:/workspace:ro
      - go-cache:/go/pkg/mod
      - /var/run/docker.sock:/var/run/docker.sock  # Access host Docker
      - ${HOME}/.kube:/root/.kube:ro  # Optional: existing kubeconfig
    working_dir: /workspace
    environment:
      - CGO_ENABLED=0
    command: make test-e2e

  # Lint - Code quality checks
  lint:
    image: golangci/golangci-lint:v1.55
    volumes:
      - .:/workspace:ro
      - golangci-cache:/root/.cache/golangci-lint
    working_dir: /workspace
    command: golangci-lint run --timeout=10m

  # Format Check
  fmt-check:
    image: golang:1.21
    volumes:
      - .:/workspace:ro
    working_dir: /workspace
    command: sh -c 'test -z "$(gofmt -l .)" || (echo "Code not formatted. Run: make fmt" && gofmt -l . && exit 1)'

  # Build Verification
  build:
    image: golang:1.21
    volumes:
      - .:/workspace:ro
      - go-cache:/go/pkg/mod
    working_dir: /workspace
    command: go build -v ./cmd/...

  # Service Dependencies
  postgres:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
      POSTGRES_DB: kubernaut_test
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 5s
      timeout: 5s
      retries: 5
    volumes:
      - postgres-data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5
    volumes:
      - redis-data:/data

volumes:
  go-cache:
  golangci-cache:
  postgres-data:
  redis-data:
```

---

### 2. Makefile Targets for Container Execution

**File**: `Makefile` (add these targets)

```makefile
##@ Container-Based Testing (Platform Agnostic)

.PHONY: docker-test
docker-test: ## Run all tests in containers (unit + integration)
	@echo "🐳 Running tests in containers..."
	docker-compose -f docker-compose.test.yml up --abort-on-container-exit --exit-code-from unit-tests unit-tests integration-tests
	docker-compose -f docker-compose.test.yml down -v

.PHONY: docker-test-unit
docker-test-unit: ## Run unit tests in container
	@echo "🐳 Running unit tests in container..."
	docker-compose -f docker-compose.test.yml run --rm unit-tests

.PHONY: docker-test-integration
docker-test-integration: ## Run integration tests in container
	@echo "🐳 Running integration tests in container..."
	docker-compose -f docker-compose.test.yml up --abort-on-container-exit --exit-code-from integration-tests integration-tests postgres redis
	docker-compose -f docker-compose.test.yml down -v

.PHONY: docker-test-e2e
docker-test-e2e: ## Run E2E tests in container with Kind
	@echo "🐳 Running E2E tests in container..."
	docker-compose -f docker-compose.test.yml run --rm e2e-tests

.PHONY: docker-lint
docker-lint: ## Run linter in container
	@echo "🐳 Running linter in container..."
	docker-compose -f docker-compose.test.yml run --rm lint

.PHONY: docker-fmt-check
docker-fmt-check: ## Check code formatting in container
	@echo "🐳 Checking code formatting in container..."
	docker-compose -f docker-compose.test.yml run --rm fmt-check

.PHONY: docker-build
docker-build: ## Build binaries in container
	@echo "🐳 Building binaries in container..."
	docker-compose -f docker-compose.test.yml run --rm build

.PHONY: docker-test-all
docker-test-all: docker-fmt-check docker-lint docker-build docker-test docker-test-e2e ## Run ALL tests in containers
	@echo "✅ All container-based tests passed!"

.PHONY: docker-clean
docker-clean: ## Clean up all Docker resources
	@echo "🧹 Cleaning up Docker resources..."
	docker-compose -f docker-compose.test.yml down -v --remove-orphans
	docker volume prune -f
```

---

### 3. GitHub Actions Workflow (Container-Based)

**File**: `.github/workflows/test-comprehensive-containers.yml`

```yaml
name: Test Suite (Container-Based)

on:
  push:
    branches: [ main, develop, 'feature/**' ]
  pull_request:
    branches: [ main, develop ]

jobs:
  unit-tests:
    name: 🧪 Unit Tests (Container)
    runs-on: ubuntu-latest
    container:
      image: golang:1.21
    steps:
      - uses: actions/checkout@v4
      
      - name: Cache Go modules
        uses: actions/cache@v4
        with:
          path: /go/pkg/mod
          key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
          restore-keys: |
            ${{ runner.os }}-go-
      
      - name: Run unit tests
        run: make test

  integration-tests:
    name: 🧩 Integration Tests (Container)
    runs-on: ubuntu-latest
    container:
      image: golang:1.21
    
    services:
      postgres:
        image: pgvector/pgvector:pg16
        env:
          POSTGRES_USER: postgres
          POSTGRES_PASSWORD: postgres
          POSTGRES_DB: kubernaut_test
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
      
      redis:
        image: redis:7-alpine
        options: >-
          --health-cmd "redis-cli ping"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    
    steps:
      - uses: actions/checkout@v4
      
      - name: Cache Go modules
        uses: actions/cache@v4
        with:
          path: /go/pkg/mod
          key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
      
      - name: Run integration tests
        env:
          POSTGRES_HOST: postgres
          POSTGRES_PORT: 5432
          POSTGRES_USER: postgres
          POSTGRES_PASSWORD: postgres
          POSTGRES_DB: kubernaut_test
          REDIS_HOST: redis
          REDIS_PORT: 6379
        run: make test-integration-service-all

  e2e-tests:
    name: 🚀 E2E Tests (Container + Kind)
    runs-on: ubuntu-latest
    container:
      image: golang:1.21
      options: --privileged  # Required for Kind
    
    steps:
      - uses: actions/checkout@v4
      
      - name: Install Docker CLI
        run: |
          apt-get update
          apt-get install -y docker.io
      
      - name: Install Kind
        run: |
          go install sigs.k8s.io/kind@v0.20.0
      
      - name: Run E2E tests
        run: make test-e2e

  lint:
    name: 🔍 Lint (Container)
    runs-on: ubuntu-latest
    container:
      image: golangci/golangci-lint:v1.55
    
    steps:
      - uses: actions/checkout@v4
      
      - name: Run golangci-lint
        run: golangci-lint run --timeout=10m

  format-check:
    name: 📝 Format Check (Container)
    runs-on: ubuntu-latest
    container:
      image: golang:1.21
    
    steps:
      - uses: actions/checkout@v4
      
      - name: Check code formatting
        run: |
          if [ -n "$(gofmt -l .)" ]; then
            echo "Code not formatted. Run 'make fmt'"
            gofmt -l .
            exit 1
          fi

  build:
    name: 🔨 Build (Container)
    runs-on: ubuntu-latest
    container:
      image: golang:1.21
    
    steps:
      - uses: actions/checkout@v4
      
      - name: Cache Go modules
        uses: actions/cache@v4
        with:
          path: /go/pkg/mod
          key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
      
      - name: Build all binaries
        run: go build -v ./cmd/...

  summary:
    name: 📊 Test Summary
    runs-on: ubuntu-latest
    needs: [unit-tests, integration-tests, e2e-tests, lint, format-check, build]
    if: always()
    
    steps:
      - name: Check test results
        run: |
          if [ "${{ needs.unit-tests.result }}" != "success" ] || \
             [ "${{ needs.integration-tests.result }}" != "success" ] || \
             [ "${{ needs.e2e-tests.result }}" != "success" ] || \
             [ "${{ needs.lint.result }}" != "success" ] || \
             [ "${{ needs.format-check.result }}" != "success" ] || \
             [ "${{ needs.build.result }}" != "success" ]; then
            echo "❌ Some tests failed"
            exit 1
          fi
          echo "✅ All tests passed!"
```

---

### 4. Usage Guide

#### Local Development (Any Platform)

```bash
# Run all tests
make docker-test-all

# Run specific test tier
make docker-test-unit
make docker-test-integration
make docker-test-e2e

# Run lint
make docker-lint

# Check formatting
make docker-fmt-check

# Build binaries
make docker-build

# Clean up
make docker-clean
```

#### With Podman (Docker alternative)

```bash
# Podman is Docker-compatible
alias docker=podman
alias docker-compose=podman-compose

# Same commands work!
make docker-test-all
```

#### CI/CD Integration

**GitHub Actions**: Use the workflow above
**GitLab CI**: Similar approach with `services:`
**Jenkins**: Use Docker agents
**CircleCI**: Use `docker` executor

---

## Benefits of This Approach

### ✅ Platform Agnostic
- Works on macOS, Linux, Windows
- Works with Docker, Podman, containerd
- Same commands everywhere

### ✅ Reproducible
- Exact same Go version (1.21)
- Exact same dependencies
- Exact same service versions (PostgreSQL 16, Redis 7)

### ✅ Isolated
- No pollution of host environment
- No version conflicts
- Clean state for each run

### ✅ Fast
- Cached Go modules
- Cached linter cache
- Parallel execution

### ✅ CI/CD Ready
- Works in any CI system
- No special CI configuration needed
- Same behavior locally and in CI

---

## Comparison: Native vs Container

| Aspect | Native Execution | Container Execution |
|--------|------------------|---------------------|
| **Setup** | Install Go, tools, services | Install Docker only |
| **Reproducibility** | ❌ Varies by environment | ✅ Identical everywhere |
| **Isolation** | ❌ Shares host environment | ✅ Fully isolated |
| **Speed** | ✅ Slightly faster | ⚠️ Small overhead (~5%) |
| **CI/CD** | ⚠️ Platform-specific | ✅ Platform-agnostic |
| **Debugging** | ✅ Direct access | ⚠️ Need to exec into container |
| **Resource Usage** | ✅ Lower | ⚠️ Higher (containers) |

---

## Migration Path

### Phase 1: Add Container Support (This PR)
- ✅ Add `docker-compose.test.yml`
- ✅ Add Makefile targets (`docker-test-*`)
- ✅ Document usage
- ⚠️ Keep existing native execution

**Time**: 1-2 hours
**Risk**: Low (additive only)

### Phase 2: Migrate CI to Containers (Next PR)
- ✅ Update GitHub Actions workflow
- ✅ Test in CI
- ✅ Validate all jobs pass
- ⚠️ Keep native workflow as backup

**Time**: 2-3 hours
**Risk**: Medium (CI changes)

### Phase 3: Make Containers Default (Future)
- ✅ Update documentation
- ✅ Remove native workflow
- ✅ Enforce container-based testing

**Time**: 1 hour
**Risk**: Low (already validated)

---

## Immediate Action for PR #17

### Option A: Fix Current CI First (Recommended)
1. Fix formatting: `make fmt`
2. Fix vendor: `go mod vendor`
3. Commit and push
4. Verify CI passes
5. **Then** add container support in follow-up PR

**Time**: 10 minutes
**Risk**: Minimal

### Option B: Add Containers Now
1. Fix formatting and vendor
2. Add `docker-compose.test.yml`
3. Add Makefile targets
4. Update GitHub Actions
5. Test locally
6. Commit and push

**Time**: 2-3 hours
**Risk**: Medium (more changes in one PR)

---

## Recommendation

**For PR #17**: Option A (fix current CI)
**For Next PR**: Add container support (Option B)

This gives you:
1. ✅ Quick fix for current failures
2. ✅ Time to properly test container approach
3. ✅ Cleaner git history
4. ✅ Lower risk

