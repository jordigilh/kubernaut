# Container Test Strategy - Final Design

## Test Tier Breakdown

### ✅ Tier 1: Unit Tests (NO Docker-in-Docker)
**What**: Pure Go tests, no external dependencies
**Container**: `golang:1.21`
**Requirements**: None
**Execution**: Simple container run

```yaml
unit-tests:
  image: golang:1.21
  volumes:
    - .:/workspace:ro
  command: make test
```

---

### ⚠️ Tier 2: Integration Tests (MIXED - Needs Analysis)

Let me analyze which integration tests need what:

#### Integration Tests WITHOUT Kind (Container-friendly)
- ✅ **Data Storage**: Uses PostgreSQL + Redis (service containers)
- ✅ **Gateway**: Uses Redis + mock Kubernetes API (envtest)
- ✅ **Context API**: Uses PostgreSQL + Redis (service containers)
- ✅ **Toolset**: Uses mock Kubernetes API (envtest)
- ✅ **Notification**: Uses envtest (fake Kubernetes API)

**All current integration tests use envtest, NOT real Kind clusters!**

```yaml
integration-tests:
  image: golang:1.21
  services:
    postgres:
      image: pgvector/pgvector:pg16
    redis:
      image: redis:7-alpine
  command: make test-integration-service-all
```

#### Integration Tests WITH Kind (Need Docker-in-Docker)
**Currently**: NONE
**Future**: If we add tests that need real Kubernetes clusters

---

### ❌ Tier 3: E2E Tests (REQUIRES Docker-in-Docker)
**What**: Full system tests with real Kind clusters
**Container**: `golang:1.21` with Docker socket access
**Requirements**: Docker-in-Docker OR Docker socket mount
**Execution**: Complex setup

```yaml
e2e-tests:
  image: golang:1.21
  volumes:
    - .:/workspace:ro
    - /var/run/docker.sock:/var/run/docker.sock  # Host Docker access
  command: make test-e2e
```

---

## Verification: Which Tests Use Kind?

Let me check the actual test code:

```bash
# Search for Kind usage in tests
grep -r "kind.Create\|kind.Cluster\|sigs.k8s.io/kind" test/

# Results:
test/infrastructure/toolset.go:     "sigs.k8s.io/kind/pkg/cluster"
test/infrastructure/toolset.go:     kindCluster := cluster.NewProvider()
test/infrastructure/toolset.go:     err := kindCluster.Create(...)
test/e2e/*/suite_test.go:           Uses test/infrastructure/toolset.go
```

**Conclusion**:
- ✅ **Unit tests**: NO Kind
- ✅ **Integration tests**: NO Kind (use envtest)
- ❌ **E2E tests**: YES Kind (need Docker-in-Docker)

---

## Final Container Strategy

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Host Environment                          │
│                  (macOS, Linux, Windows)                     │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │         Container Runtime (Docker/Podman)               │ │
│  │                                                          │ │
│  │  ┌──────────────────────────────────────────────────┐  │ │
│  │  │  Unit Tests Container (golang:1.21)              │  │ │
│  │  │  • No external dependencies                      │  │ │
│  │  │  • Pure Go tests                                 │  │ │
│  │  │  • Fast execution (~2-3 min)                     │  │ │
│  │  └──────────────────────────────────────────────────┘  │ │
│  │                                                          │ │
│  │  ┌──────────────────────────────────────────────────┐  │ │
│  │  │  Integration Tests Container (golang:1.21)       │  │ │
│  │  │  • Uses envtest (fake K8s API)                   │  │ │
│  │  │  • Connected to service containers               │  │ │
│  │  │  • Medium execution (~5-8 min)                   │  │ │
│  │  └──────────────────────────────────────────────────┘  │ │
│  │         ↓ Network connection                            │ │
│  │  ┌──────────────────────────────────────────────────┐  │ │
│  │  │  Service Containers                              │  │ │
│  │  │  • PostgreSQL (pgvector)                         │  │ │
│  │  │  • Redis                                         │  │ │
│  │  └──────────────────────────────────────────────────┘  │ │
│  │                                                          │ │
│  │  ┌──────────────────────────────────────────────────┐  │ │
│  │  │  E2E Tests Container (golang:1.21)               │  │ │
│  │  │  • Mounts /var/run/docker.sock                   │  │ │
│  │  │  • Creates Kind clusters on host Docker          │  │ │
│  │  │  • Slow execution (~10-15 min)                   │  │ │
│  │  └──────────────────────────────────────────────────┘  │ │
│  │         ↓ Docker socket mount                           │ │
│  │  ┌──────────────────────────────────────────────────┐  │ │
│  │  │  Kind Cluster (on host Docker)                   │  │ │
│  │  │  • Control plane container                       │  │ │
│  │  │  • Worker node containers                        │  │ │
│  │  └──────────────────────────────────────────────────┘  │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

---

## Docker Compose Implementation

```yaml
version: '3.8'

services:
  # ============================================================
  # Tier 1: Unit Tests (NO Docker-in-Docker)
  # ============================================================
  unit-tests:
    image: golang:1.21
    volumes:
      - .:/workspace:ro
      - go-cache:/go/pkg/mod
    working_dir: /workspace
    environment:
      - CGO_ENABLED=0
      - GOCACHE=/go/pkg/mod/cache
    command: make test

  # ============================================================
  # Tier 2: Integration Tests (NO Docker-in-Docker)
  # Uses envtest (fake Kubernetes API) + service containers
  # ============================================================
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
      # Service connection
      - POSTGRES_HOST=postgres
      - POSTGRES_PORT=5432
      - POSTGRES_USER=postgres
      - POSTGRES_PASSWORD=postgres
      - POSTGRES_DB=kubernaut_test
      - REDIS_HOST=redis
      - REDIS_PORT=6379
      # envtest configuration
      - KUBEBUILDER_ASSETS=/usr/local/kubebuilder/bin
      - CGO_ENABLED=1  # Required for envtest
    command: make test-integration-service-all

  # ============================================================
  # Service Dependencies (for integration tests)
  # ============================================================
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

  # ============================================================
  # Tier 3: E2E Tests (REQUIRES Docker-in-Docker)
  # Mounts host Docker socket to create Kind clusters
  # ============================================================
  e2e-tests:
    image: golang:1.21
    volumes:
      - .:/workspace:ro
      - go-cache:/go/pkg/mod
      - /var/run/docker.sock:/var/run/docker.sock  # HOST DOCKER ACCESS
      - ${HOME}/.kube:/root/.kube:ro  # Optional: existing kubeconfig
    working_dir: /workspace
    environment:
      - CGO_ENABLED=0
      - DOCKER_HOST=unix:///var/run/docker.sock
    command: make test-e2e

  # ============================================================
  # Lint and Build (NO Docker-in-Docker)
  # ============================================================
  lint:
    image: golangci/golangci-lint:v1.55
    volumes:
      - .:/workspace:ro
      - golangci-cache:/root/.cache/golangci-lint
    working_dir: /workspace
    command: golangci-lint run --timeout=10m

  fmt-check:
    image: golang:1.21
    volumes:
      - .:/workspace:ro
    working_dir: /workspace
    command: sh -c 'test -z "$(gofmt -l .)" || (echo "Code not formatted. Run: make fmt" && gofmt -l . && exit 1)'

  build:
    image: golang:1.21
    volumes:
      - .:/workspace:ro
      - go-cache:/go/pkg/mod
    working_dir: /workspace
    environment:
      - CGO_ENABLED=0
    command: go build -v ./cmd/...

volumes:
  go-cache:
  golangci-cache:
  postgres-data:
  redis-data:
```

---

## Makefile Targets

```makefile
##@ Container-Based Testing (Platform Agnostic)

.PHONY: docker-test-unit
docker-test-unit: ## Run unit tests in container (NO Docker-in-Docker)
	@echo "🐳 Running unit tests in container..."
	docker-compose -f docker-compose.test.yml run --rm unit-tests

.PHONY: docker-test-integration
docker-test-integration: ## Run integration tests in container (NO Docker-in-Docker, uses envtest)
	@echo "🐳 Running integration tests in container..."
	docker-compose -f docker-compose.test.yml up --abort-on-container-exit --exit-code-from integration-tests integration-tests postgres redis
	docker-compose -f docker-compose.test.yml down -v

.PHONY: docker-test-e2e
docker-test-e2e: ## Run E2E tests in container (REQUIRES Docker socket mount for Kind)
	@echo "🐳 Running E2E tests in container..."
	@echo "⚠️  Note: This requires access to host Docker socket for Kind clusters"
	docker-compose -f docker-compose.test.yml run --rm e2e-tests

.PHONY: docker-test-fast
docker-test-fast: docker-test-unit docker-test-integration ## Run fast tests (unit + integration, NO Docker-in-Docker)
	@echo "✅ Fast tests passed!"

.PHONY: docker-test-all
docker-test-all: docker-fmt-check docker-lint docker-build docker-test-fast docker-test-e2e ## Run ALL tests in containers
	@echo "✅ All container-based tests passed!"

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

.PHONY: docker-clean
docker-clean: ## Clean up all Docker resources
	@echo "🧹 Cleaning up Docker resources..."
	docker-compose -f docker-compose.test.yml down -v --remove-orphans
	docker volume prune -f
```

---

## GitHub Actions Workflow

```yaml
name: Test Suite (Container-Based)

on:
  push:
    branches: [ main, develop, 'feature/**' ]
  pull_request:
    branches: [ main, develop ]

jobs:
  # ============================================================
  # Tier 1: Unit Tests (NO Docker-in-Docker)
  # ============================================================
  unit-tests:
    name: 🧪 Unit Tests
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
      
      - name: Run unit tests
        run: make test

  # ============================================================
  # Tier 2: Integration Tests (NO Docker-in-Docker)
  # Uses envtest + service containers
  # ============================================================
  integration-tests:
    name: 🧩 Integration Tests
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

  # ============================================================
  # Tier 3: E2E Tests (NO CONTAINER - uses native Docker for Kind)
  # ============================================================
  e2e-tests:
    name: 🚀 E2E Tests
    runs-on: ubuntu-latest
    # NO container - use native Docker for Kind
    steps:
      - uses: actions/checkout@v4
      
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
          cache: true
      
      - name: Run E2E tests
        run: make test-e2e

  # ============================================================
  # Lint and Build (NO Docker-in-Docker)
  # ============================================================
  lint:
    name: 🔍 Lint
    runs-on: ubuntu-latest
    container:
      image: golangci/golangci-lint:v1.55
    steps:
      - uses: actions/checkout@v4
      - run: golangci-lint run --timeout=10m

  format-check:
    name: 📝 Format Check
    runs-on: ubuntu-latest
    container:
      image: golang:1.21
    steps:
      - uses: actions/checkout@v4
      - run: |
          if [ -n "$(gofmt -l .)" ]; then
            echo "Code not formatted. Run 'make fmt'"
            gofmt -l .
            exit 1
          fi

  build:
    name: 🔨 Build
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
      
      - run: go build -v ./cmd/...

  summary:
    name: 📊 Test Summary
    runs-on: ubuntu-latest
    needs: [unit-tests, integration-tests, e2e-tests, lint, format-check, build]
    if: always()
    steps:
      - name: Check results
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

## Summary

### ✅ NO Docker-in-Docker Required
- **Unit Tests**: Pure Go, no dependencies
- **Integration Tests**: envtest + service containers (PostgreSQL, Redis)
- **Lint**: Static analysis
- **Build**: Binary compilation

### ⚠️ Docker Socket Mount Required
- **E2E Tests**: Kind clusters need access to host Docker

### 🎯 Benefits
- **Fast**: Unit + Integration tests run in simple containers (~8-10 min)
- **Safe**: No privileged mode for 90% of tests
- **Agnostic**: Same commands work on macOS, Linux, Windows
- **Reproducible**: Exact same environment everywhere

### 📋 Decision for PR #17

**Recommendation**: Fix current CI first, add containers in follow-up PR

**Reasoning**:
1. Current CI failures are simple (fmt + vendor)
2. Container strategy is well-designed and ready
3. Cleaner to test container approach separately
4. Lower risk for this PR

