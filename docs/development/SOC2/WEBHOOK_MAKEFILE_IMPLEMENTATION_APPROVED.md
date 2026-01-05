# Webhook Makefile Implementation - APPROVED APPROACH
**Date**: January 6, 2026  
**Status**: ✅ **APPROVED** - Option B (Explicit Targets for TDD)  
**Decision**: Use explicit Makefile targets for immediate testability during TDD implementation

---

## ✅ **APPROVED APPROACH: Option B**

**Explicit Makefile Targets** (like HolmesGPT special case)

### **Rationale**

| Criterion | Why Option B |
|-----------|--------------|
| **TDD Methodology** | Tests must exist BEFORE `cmd/authwebhook/` |
| **Immediate Testability** | Can run tests on Day 1 of implementation |
| **Coverage Variants** | Explicit targets for unit/integration/E2E coverage |
| **Proven Pattern** | HolmesGPT uses explicit targets successfully |
| **Flexibility** | Can coexist with pattern-based targets later |

---

## 📋 **IMPLEMENTATION TASKS**

### **Task 1: Add Explicit Makefile Targets**

Add after line 437 in `Makefile` (after HolmesGPT special cases):

```makefile
##@ Special Cases - Authentication Webhook (Shared Library)

.PHONY: test-unit-authwebhook
test-unit-authwebhook: ginkgo ## Run authentication webhook unit tests
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Authentication Webhook - Unit Tests ($(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_UNIT) --procs=$(TEST_PROCS) ./test/unit/authwebhook/...

.PHONY: test-coverage-authwebhook
test-coverage-authwebhook: ## Run webhook unit tests with coverage
	@echo "📊 Running webhook unit tests with coverage..."
	@cd test/unit/authwebhook && \
		go test -v -p $(TEST_PROCS) -coverprofile=coverage.out -covermode=atomic ./... && \
		go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report: test/unit/authwebhook/coverage.html"

.PHONY: test-integration-authwebhook
test-integration-authwebhook: ginkgo ## Run webhook integration tests (envtest + real CRDs)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Authentication Webhook - Integration Tests ($(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Pattern: DD-INTEGRATION-001 v2.0 (envtest + programmatic infrastructure)"
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_INTEGRATION) --procs=$(TEST_PROCS) --fail-fast ./test/integration/authwebhook/...

.PHONY: test-coverage-integration-authwebhook
test-coverage-integration-authwebhook: ## Run webhook integration tests with coverage
	@echo "📊 Running webhook integration tests with coverage..."
	@cd test/integration/authwebhook && \
		go test -v -p $(TEST_PROCS) -coverprofile=coverage.out -covermode=atomic ./... && \
		go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report: test/integration/authwebhook/coverage.html"

.PHONY: test-e2e-authwebhook
test-e2e-authwebhook: ginkgo ensure-coverdata ## Run webhook E2E tests (Kind cluster)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Authentication Webhook - E2E Tests (Kind cluster, $(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_E2E) --procs=$(TEST_PROCS) ./test/e2e/authwebhook/...

.PHONY: test-coverage-e2e-authwebhook
test-coverage-e2e-authwebhook: ensure-coverdata ## Run webhook E2E tests with binary coverage
	@echo "📊 Running webhook E2E tests with binary coverage..."
	@echo "🔧 Step 1: Building webhook with coverage instrumentation..."
	@CGO_ENABLED=0 go build -cover -o bin/authwebhook-coverage ./cmd/authwebhook
	@echo "✅ Coverage-instrumented binary built: bin/authwebhook-coverage"
	@echo ""
	@echo "🐳 Step 2: Building Docker image with coverage binary..."
	@docker build -t authwebhook:e2e-coverage \
		--build-arg BINARY=bin/authwebhook-coverage \
		-f cmd/authwebhook/Dockerfile.e2e .
	@echo "✅ E2E coverage image built: authwebhook:e2e-coverage"
	@echo ""
	@echo "☸️  Step 3: Running E2E tests (Kind cluster with GOCOVERDIR)..."
	@WEBHOOK_IMAGE=authwebhook:e2e-coverage \
		GOCOVERDIR=$(PWD)/coverdata \
		$(GINKGO) -v --timeout=$(TEST_TIMEOUT_E2E) --procs=$(TEST_PROCS) ./test/e2e/authwebhook/...
	@echo ""
	@echo "📊 Step 4: Converting binary coverage to textfmt..."
	@go tool covdata textfmt -i=coverdata -o=coverage-e2e-authwebhook.out
	@go tool cover -html=coverage-e2e-authwebhook.out -o=coverage-e2e-authwebhook.html
	@echo "✅ E2E Coverage report: coverage-e2e-authwebhook.html"

.PHONY: test-all-authwebhook
test-all-authwebhook: ## Run all webhook test tiers (Unit + Integration + E2E)
	@echo "═══════════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Running ALL Authentication Webhook Tests (3 tiers)"
	@echo "═══════════════════════════════════════════════════════════════════════════════"
	@FAILED=0; \
	$(MAKE) test-unit-authwebhook || FAILED=$$((FAILED + 1)); \
	$(MAKE) test-integration-authwebhook || FAILED=$$((FAILED + 1)); \
	$(MAKE) test-e2e-authwebhook || FAILED=$$((FAILED + 1)); \
	if [ $$FAILED -gt 0 ]; then \
		echo "❌ $$FAILED test tier(s) failed"; \
		exit 1; \
	fi
	@echo "✅ All webhook test tiers completed successfully!"

.PHONY: test-coverage-all-authwebhook
test-coverage-all-authwebhook: ## Run all webhook test tiers with coverage collection
	@echo "═══════════════════════════════════════════════════════════════════════════════"
	@echo "📊 Running ALL Authentication Webhook Tests with Coverage (3 tiers)"
	@echo "═══════════════════════════════════════════════════════════════════════════════"
	@FAILED=0; \
	$(MAKE) test-coverage-authwebhook || FAILED=$$((FAILED + 1)); \
	$(MAKE) test-coverage-integration-authwebhook || FAILED=$$((FAILED + 1)); \
	$(MAKE) test-coverage-e2e-authwebhook || FAILED=$$((FAILED + 1)); \
	if [ $$FAILED -gt 0 ]; then \
		echo "❌ $$FAILED coverage tier(s) failed"; \
		exit 1; \
	fi
	@echo "✅ All webhook coverage tiers completed successfully!"
	@echo ""
	@echo "📊 Coverage Reports:"
	@echo "   Unit:        test/unit/authwebhook/coverage.html"
	@echo "   Integration: test/integration/authwebhook/coverage.html"
	@echo "   E2E:         coverage-e2e-authwebhook.html"

.PHONY: clean-authwebhook-integration
clean-authwebhook-integration: ## Clean webhook integration test infrastructure
	@echo "🧹 Cleaning webhook integration infrastructure..."
	@podman stop authwebhook_postgres_1 authwebhook_redis_1 authwebhook_datastorage_1 2>/dev/null || true
	@podman rm authwebhook_postgres_1 authwebhook_redis_1 authwebhook_datastorage_1 2>/dev/null || true
	@podman network rm authwebhook_test-network 2>/dev/null || true
	@echo "✅ Cleanup complete"
```

**Insertion Point**: Line 438 in Makefile (after HolmesGPT, before Legacy Aliases)

---

### **Task 2: Create Programmatic Infrastructure Setup**

**File**: `test/infrastructure/authwebhook.go`

```go
package infrastructure

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/onsi/ginkgo/v2"
)

const (
	// Container names (DD-INTEGRATION-001 v2.0 naming convention)
	AuthWebhookIntegrationPostgresContainer   = "authwebhook_postgres_1"
	AuthWebhookIntegrationRedisContainer      = "authwebhook_redis_1"
	AuthWebhookIntegrationDataStorageContainer = "authwebhook_datastorage_1"
	AuthWebhookIntegrationNetworkName         = "authwebhook_test-network"

	// Ports (avoid conflicts with other services)
	AuthWebhookIntegrationDataStoragePort = 18099 // HTTP port for Data Storage API
	AuthWebhookIntegrationPostgresPort    = 15435 // PostgreSQL port
	AuthWebhookIntegrationRedisPort       = 16381 // Redis port
)

// StartAuthWebhookIntegrationInfrastructure starts PostgreSQL, Redis, and Data Storage
// for webhook integration tests using programmatic podman commands.
//
// DD-INTEGRATION-001 v2.0: Programmatic infrastructure setup
// Pattern: Follows AIAnalysis integration test infrastructure
func StartAuthWebhookIntegrationInfrastructure(ctx context.Context) error {
	ginkgo.GinkgoWriter.Println("🏗️  Starting webhook integration infrastructure...")

	projectRoot, err := GetProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	// Step 1: Create network
	ginkgo.GinkgoWriter.Println("📡 Creating test network...")
	networkCmd := exec.Command("podman", "network", "create", AuthWebhookIntegrationNetworkName)
	if err := networkCmd.Run(); err != nil {
		// Network might already exist, which is fine
		ginkgo.GinkgoWriter.Printf("⚠️  Network creation warning (may already exist): %v\n", err)
	}

	// Step 2: Start PostgreSQL
	ginkgo.GinkgoWriter.Println("🐘 Starting PostgreSQL...")
	postgresCmd := exec.Command("podman", "run", "-d",
		"--name", AuthWebhookIntegrationPostgresContainer,
		"--network", AuthWebhookIntegrationNetworkName,
		"-p", fmt.Sprintf("%d:5432", AuthWebhookIntegrationPostgresPort),
		"-e", "POSTGRES_PASSWORD=test_password",
		"-e", "POSTGRES_DB=kubernaut",
		"docker.io/library/postgres:15-alpine",
	)
	if output, err := postgresCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start PostgreSQL: %w\nOutput: %s", err, output)
	}

	// Wait for PostgreSQL to be ready
	ginkgo.GinkgoWriter.Println("⏳ Waiting for PostgreSQL to be ready...")
	if err := waitForPostgres(ctx, "localhost", AuthWebhookIntegrationPostgresPort, "kubernaut", "postgres", "test_password"); err != nil {
		return fmt.Errorf("PostgreSQL failed to become ready: %w", err)
	}
	ginkgo.GinkgoWriter.Println("✅ PostgreSQL ready")

	// Step 3: Start Redis
	ginkgo.GinkgoWriter.Println("📮 Starting Redis...")
	redisCmd := exec.Command("podman", "run", "-d",
		"--name", AuthWebhookIntegrationRedisContainer,
		"--network", AuthWebhookIntegrationNetworkName,
		"-p", fmt.Sprintf("%d:6379", AuthWebhookIntegrationRedisPort),
		"docker.io/library/redis:7-alpine",
	)
	if output, err := redisCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start Redis: %w\nOutput: %s", err, output)
	}

	// Wait for Redis to be ready
	ginkgo.GinkgoWriter.Println("⏳ Waiting for Redis to be ready...")
	time.Sleep(2 * time.Second) // Redis is usually very fast
	ginkgo.GinkgoWriter.Println("✅ Redis ready")

	// Step 4: Build Data Storage service
	ginkgo.GinkgoWriter.Println("🔨 Building Data Storage service...")
	buildCmd := exec.Command("make", "build-datastorage")
	buildCmd.Dir = projectRoot
	if output, err := buildCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to build Data Storage: %w\nOutput: %s", err, output)
	}

	// Step 5: Start Data Storage
	ginkgo.GinkgoWriter.Println("💾 Starting Data Storage...")
	dsConfigPath := filepath.Join(projectRoot, "test", "integration", "authwebhook", "datastorage-config.yaml")
	dsCmd := exec.Command("podman", "run", "-d",
		"--name", AuthWebhookIntegrationDataStorageContainer,
		"--network", AuthWebhookIntegrationNetworkName,
		"-p", fmt.Sprintf("%d:8080", AuthWebhookIntegrationDataStoragePort),
		"-v", fmt.Sprintf("%s:/etc/datastorage/config.yaml:ro", dsConfigPath),
		"-e", fmt.Sprintf("DATABASE_URL=postgresql://postgres:test_password@%s:5432/kubernaut", AuthWebhookIntegrationPostgresContainer),
		"-e", fmt.Sprintf("REDIS_URL=redis://%s:6379", AuthWebhookIntegrationRedisContainer),
		"--entrypoint", "/bin/datastorage",
		"localhost/kubernaut-datastorage:latest",
		"-config", "/etc/datastorage/config.yaml",
	)
	if output, err := dsCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start Data Storage: %w\nOutput: %s", err, output)
	}

	// Wait for Data Storage to be ready
	ginkgo.GinkgoWriter.Println("⏳ Waiting for Data Storage to be ready...")
	if err := waitForHTTP(ctx, fmt.Sprintf("http://localhost:%d/health", AuthWebhookIntegrationDataStoragePort), 60*time.Second); err != nil {
		return fmt.Errorf("Data Storage failed to become ready: %w", err)
	}
	ginkgo.GinkgoWriter.Println("✅ Data Storage ready")

	ginkgo.GinkgoWriter.Println("✅ All webhook integration infrastructure ready")
	return nil
}

// StopAuthWebhookIntegrationInfrastructure stops all infrastructure containers
func StopAuthWebhookIntegrationInfrastructure() error {
	ginkgo.GinkgoWriter.Println("🧹 Stopping webhook integration infrastructure...")

	containers := []string{
		AuthWebhookIntegrationDataStorageContainer,
		AuthWebhookIntegrationRedisContainer,
		AuthWebhookIntegrationPostgresContainer,
	}

	for _, container := range containers {
		stopCmd := exec.Command("podman", "stop", container)
		_ = stopCmd.Run() // Ignore errors (container might not exist)

		rmCmd := exec.Command("podman", "rm", container)
		_ = rmCmd.Run() // Ignore errors (container might not exist)
	}

	// Remove network
	networkCmd := exec.Command("podman", "network", "rm", AuthWebhookIntegrationNetworkName)
	_ = networkCmd.Run() // Ignore errors (network might not exist)

	ginkgo.GinkgoWriter.Println("✅ Webhook integration infrastructure stopped")
	return nil
}
```

---

### **Task 3: Create Test Suite Files**

#### **Unit Test Suite**
**File**: `test/unit/authwebhook/suite_test.go`

```go
package authwebhook_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAuthWebhookUnit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AuthWebhook Unit Suite")
}
```

#### **Integration Test Suite**
**File**: `test/integration/authwebhook/suite_test.go`

```go
package authwebhook_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

func TestAuthWebhookIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AuthWebhook Integration Suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	By("Starting webhook integration infrastructure (PostgreSQL, Redis, Data Storage)")
	err := infrastructure.StartAuthWebhookIntegrationInfrastructure(ctx)
	Expect(err).ToNot(HaveOccurred(), "Failed to start webhook integration infrastructure")

	GinkgoWriter.Println("✅ Webhook integration infrastructure ready")
})

var _ = AfterSuite(func() {
	By("Stopping webhook integration infrastructure")
	err := infrastructure.StopAuthWebhookIntegrationInfrastructure()
	Expect(err).ToNot(HaveOccurred(), "Failed to stop webhook integration infrastructure")

	GinkgoWriter.Println("✅ Webhook integration infrastructure stopped")
})
```

#### **E2E Test Suite**
**File**: `test/e2e/authwebhook/suite_test.go`

```go
package authwebhook_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

func TestAuthWebhookE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AuthWebhook E2E Suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	By("Starting Kind cluster for webhook E2E tests")
	err := infrastructure.StartKindCluster(ctx, "webhook-e2e")
	Expect(err).ToNot(HaveOccurred(), "Failed to start Kind cluster")

	By("Deploying webhook to Kind cluster")
	err = infrastructure.DeployWebhookToKind(ctx)
	Expect(err).ToNot(HaveOccurred(), "Failed to deploy webhook")

	GinkgoWriter.Println("✅ Webhook E2E infrastructure ready")
})

var _ = AfterSuite(func() {
	By("Cleaning up Kind cluster")
	err := infrastructure.StopKindCluster("webhook-e2e")
	Expect(err).ToNot(HaveOccurred(), "Failed to stop Kind cluster")

	GinkgoWriter.Println("✅ Webhook E2E infrastructure cleaned up")
})
```

---

## ✅ **COMPLIANCE VERIFICATION**

| Requirement | Status | Implementation |
|-------------|--------|----------------|
| **DD-TEST-002** (Parallel) | ✅ | `--procs=$(TEST_PROCS)` in all targets |
| **DD-INTEGRATION-001** | ✅ | Programmatic infrastructure setup |
| **DD-TEST-007** (E2E Coverage) | ✅ | Binary coverage collection target |
| **TDD Methodology** | ✅ | Tests can be written before `cmd/authwebhook/` |
| **Port Allocation** | ✅ | Unique ports (18099, 15435, 16381) |
| **HolmesGPT Pattern** | ✅ | Follows proven special case approach |

---

## 📊 **EXPECTED MAKE TARGET USAGE**

```bash
# During TDD Implementation (Day 1+)
make test-unit-authwebhook              # Run unit tests
make test-coverage-authwebhook          # Unit tests with coverage

# Integration testing (Day 2+)
make test-integration-authwebhook       # Run integration tests
make test-coverage-integration-authwebhook  # Integration with coverage

# E2E testing (Day 5-6)
make test-e2e-authwebhook               # Run E2E tests
make test-coverage-e2e-authwebhook      # E2E with binary coverage

# All tiers
make test-all-authwebhook               # Run all test tiers
make test-coverage-all-authwebhook      # All tiers with coverage

# Cleanup
make clean-authwebhook-integration      # Clean integration infrastructure
```

---

## 🎯 **SUCCESS CRITERIA**

- ✅ Make targets work BEFORE `cmd/authwebhook/` exists
- ✅ Parallel execution per DD-TEST-002
- ✅ Coverage collection per TESTING_GUIDELINES.md (70%/50%/50%)
- ✅ Programmatic infrastructure (no docker-compose.yml)
- ✅ TDD-friendly (tests run immediately)

---

## 📝 **NEXT ACTIONS**

1. ⬜ **Add Makefile targets** (copy/paste from Task 1)
2. ⬜ **Create `test/infrastructure/authwebhook.go`** (copy/paste from Task 2)
3. ⬜ **Create test suite files** (copy/paste from Task 3)
4. ⬜ **Verify with dry run**: `make test-unit-authwebhook` (should fail gracefully if no tests exist yet)
5. ⬜ **Begin TDD**: Write first failing test

---

**Status**: ✅ **READY FOR IMPLEMENTATION**  
**Approval**: User selected Option B  
**Timeline**: Ready for Day 1 of webhook TDD implementation

