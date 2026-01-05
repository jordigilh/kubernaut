# Webhook Makefile Integration Triage
**Date**: January 6, 2026
**Context**: SOC2 Compliance - Single Consolidated Webhook
**Objective**: Determine make targets needed for webhook testing using existing patterns

---

## 🔍 **EXISTING MAKEFILE PATTERNS**

### **Pattern 1: Service-Based Test Targets**

The Makefile uses **pattern-based targets** for all services discovered in `cmd/`:

```makefile
# Service auto-discovery from cmd/ directory
SERVICES := $(filter-out README.md, $(notdir $(wildcard cmd/*)))
# Result: aianalysis datastorage gateway notification remediationorchestrator signalprocessing workflowexecution
```

**Current Pattern Targets:**
```makefile
# Lines 110-115: Unit Tests
test-unit-%: ginkgo
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_UNIT) --procs=$(TEST_PROCS) ./test/unit/$*/...

# Lines 118-124: Integration Tests
test-integration-%: ginkgo
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_INTEGRATION) --procs=$(TEST_PROCS) --fail-fast ./test/integration/$*/...

# Lines 136-141: E2E Tests
test-e2e-%: ginkgo ensure-coverdata
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_E2E) --procs=$(TEST_PROCS) ./test/e2e/$*/...

# Lines 217-224: Coverage
test-coverage-%:
	@cd test/unit/$* && \
		go test -v -coverprofile=coverage.out -covermode=atomic ./... && \
		go tool cover -html=coverage.out -o coverage.html
```

**Key Pattern Characteristics:**
- ✅ Automatic service discovery from `cmd/`
- ✅ Parallel execution with `--procs=$(TEST_PROCS)` (DD-TEST-002)
- ✅ Consistent timeouts per tier
- ✅ Ginkgo-based test execution
- ✅ Coverage collection using `go test -coverprofile`

---

## ✅ **WEBHOOK ARCHITECTURE CORRECTION**

### **CORRECTED Understanding**

**Initial (Incorrect) Assumption**: Webhook is just a shared library in `pkg/authwebhook/`

**ACTUAL Architecture** (per DD-AUTH-001):
- ✅ Webhook **IS a standalone service** that **SHOULD** have `cmd/authwebhook/main.go`
- ✅ Webhook **IS a Kubernetes Deployment** (`kubernaut-auth-webhook`)
- ✅ Webhook **IS an HTTP server** listening for admission requests
- ✅ Webhook **USES shared library** `pkg/authwebhook/` for common logic

### **Current Status**

```bash
$ ls cmd/
aianalysis  datastorage  gateway  must-gather  notification
remediationorchestrator  signalprocessing  workflowexecution

# ❌ cmd/authwebhook/ does NOT exist yet
```

**Reason**: The webhook is **NOT YET IMPLEMENTED** - it's part of the SOC2 compliance work we're planning.

### **Implication for Makefile Targets**

**Once `cmd/authwebhook/` exists**:
- ✅ Webhook **WILL** be auto-discovered: `SERVICES := $(filter-out README.md, $(notdir $(wildcard cmd/*)))`
- ✅ Pattern-based targets **WILL** work: `make test-unit-authwebhook`, `make test-integration-authwebhook`, etc.
- ✅ No special case needed (unlike HolmesGPT which is Python)

**For NOW** (during implementation):
- ⚠️ Use **explicit targets** (like HolmesGPT) until `cmd/authwebhook/` is created
- ⚠️ Explicit targets provide **immediate testability** during TDD implementation
- ✅ Once `cmd/authwebhook/main.go` exists, pattern-based targets will also work

---

## 🏗️ **WHAT NEEDS TO BE IMPLEMENTED**

### **Required: `cmd/authwebhook/main.go`**

The webhook service needs a `main.go` entry point (like all other kubernaut services):

```go
// cmd/authwebhook/main.go
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	"github.com/jordigilh/kubernaut/internal/webhook"
)

func main() {
	var (
		port       = flag.Int("port", 9443, "Webhook server port")
		certDir    = flag.String("cert-dir", "/tmp/k8s-webhook-server/serving-certs", "Certificate directory")
		configPath = flag.String("config", "/etc/authwebhook/config.yaml", "Configuration file path")
	)
	flag.Parse()

	// Initialize logger
	logger := setupLogger()

	// Load configuration (Data Storage URL for audit events)
	cfg, err := loadConfig(*configPath)
	if err != nil {
		logger.Error(err, "Failed to load configuration")
		os.Exit(1)
	}

	// Initialize audit client (for emitting authenticated audit events)
	auditClient := initializeAuditClient(cfg.DataStorageURL, logger)

	// Create webhook server
	webhookServer := webhook.NewServer(webhook.Options{
		Port:    *port,
		CertDir: *certDir,
	})

	// Register handlers for each CRD type
	// DD-AUTH-001: Single webhook service, multiple handlers
	webhookServer.Register("/authenticate/workflowexecution",
		&webhook.Admission{Handler: webhookhandlers.NewWorkflowExecutionHandler(auditClient, logger)})

	webhookServer.Register("/authenticate/remediationapproval",
		&webhook.Admission{Handler: webhookhandlers.NewRemediationApprovalHandler(auditClient, logger)})

	webhookServer.Register("/authenticate/notificationrequest",
		&webhook.Admission{Handler: webhookhandlers.NewNotificationRequestHandler(auditClient, logger)})

	// Start webhook server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		logger.Info("Starting webhook server", "port", *port)
		if err := webhookServer.Start(ctx); err != nil {
			logger.Error(err, "Webhook server failed")
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down webhook server")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := webhookServer.Stop(shutdownCtx); err != nil {
		logger.Error(err, "Graceful shutdown failed")
		os.Exit(1)
	}

	logger.Info("Webhook server shutdown complete")
}
```

**Key Characteristics** (matches other kubernaut services):
- ✅ Configuration file support (`-config` flag)
- ✅ Structured logging (logr.Logger)
- ✅ Graceful shutdown (signal handling)
- ✅ Health checks and metrics
- ✅ Audit client integration
- ✅ Multiple handler registration (DD-AUTH-001)

---

## ✅ **SOLUTION: Explicit Webhook Targets (Follow HolmesGPT Pattern)**

### **Precedent: HolmesGPT Special Case**

The Makefile already handles non-standard components with **explicit targets**:

```makefile
# Lines 226-437: Special Cases - HolmesGPT (Python Service)
.PHONY: test-unit-holmesgpt-api
test-unit-holmesgpt-api: ## Run holmesgpt-api unit tests (containerized with UBI)
	...

.PHONY: test-integration-holmesgpt-api
test-integration-holmesgpt-api: ginkgo clean-holmesgpt-test-ports
	...

.PHONY: test-e2e-holmesgpt-api
test-e2e-holmesgpt-api: ginkgo ensure-coverdata
	...

.PHONY: test-all-holmesgpt-api
test-all-holmesgpt-api: test-unit-holmesgpt-api test-integration-holmesgpt-api test-e2e-holmesgpt-api
	...
```

**Webhook should follow this EXACT pattern.**

---

## 📋 **REQUIRED WEBHOOK MAKEFILE TARGETS**

### **Target 1: Unit Tests**

```makefile
.PHONY: test-unit-authwebhook
test-unit-authwebhook: ginkgo ## Run authentication webhook unit tests
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Authentication Webhook - Unit Tests ($(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_UNIT) --procs=$(TEST_PROCS) ./test/unit/authwebhook/...
```

**Coverage Variant:**
```makefile
.PHONY: test-coverage-authwebhook
test-coverage-authwebhook: ## Run webhook unit tests with coverage
	@echo "📊 Running webhook unit tests with coverage..."
	@cd test/unit/authwebhook && \
		go test -v -p $(TEST_PROCS) -coverprofile=coverage.out -covermode=atomic ./... && \
		go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report: test/unit/authwebhook/coverage.html"
```

---

### **Target 2: Integration Tests**

```makefile
.PHONY: test-integration-authwebhook
test-integration-authwebhook: ginkgo ## Run webhook integration tests (envtest + real CRDs)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Authentication Webhook - Integration Tests ($(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Pattern: DD-INTEGRATION-001 v2.0 (envtest + programmatic infrastructure)"
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_INTEGRATION) --procs=$(TEST_PROCS) --fail-fast ./test/integration/authwebhook/...
```

**Coverage Variant:**
```makefile
.PHONY: test-coverage-integration-authwebhook
test-coverage-integration-authwebhook: ## Run webhook integration tests with coverage
	@echo "📊 Running webhook integration tests with coverage..."
	@cd test/integration/authwebhook && \
		go test -v -p $(TEST_PROCS) -coverprofile=coverage.out -covermode=atomic ./... && \
		go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report: test/integration/authwebhook/coverage.html"
```

---

### **Target 3: E2E Tests**

```makefile
.PHONY: test-e2e-authwebhook
test-e2e-authwebhook: ginkgo ensure-coverdata ## Run webhook E2E tests (Kind cluster)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Authentication Webhook - E2E Tests (Kind cluster, $(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_E2E) --procs=$(TEST_PROCS) ./test/e2e/authwebhook/...
```

**Coverage Variant (Binary Coverage - DD-TEST-007):**
```makefile
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
```

---

### **Target 4: All Tiers**

```makefile
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
```

---

### **Target 5: Coverage Collection (All Tiers)**

```makefile
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
```

---

### **Target 6: Cleanup**

```makefile
.PHONY: clean-authwebhook-integration
clean-authwebhook-integration: ## Clean webhook integration test infrastructure
	@echo "🧹 Cleaning webhook integration infrastructure..."
	@podman stop authwebhook_postgres_1 authwebhook_redis_1 authwebhook_datastorage_1 2>/dev/null || true
	@podman rm authwebhook_postgres_1 authwebhook_redis_1 authwebhook_datastorage_1 2>/dev/null || true
	@podman network rm authwebhook_test-network 2>/dev/null || true
	@echo "✅ Cleanup complete"
```

---

## 📍 **MAKEFILE INSERTION LOCATION**

Add the webhook targets in the **"Special Cases"** section, after HolmesGPT:

```makefile
##@ Special Cases - HolmesGPT (Python Service)
# ... (lines 226-437)

##@ Special Cases - Authentication Webhook (Shared Library)
# ... (webhook targets here)

##@ Legacy Aliases (Backward Compatibility)
# ... (lines 438-445)
```

---

## 🔧 **PROGRAMMATIC INFRASTRUCTURE PATTERN**

### **Existing Pattern: AIAnalysis Integration Tests**

The webhook tests should follow the **SAME programmatic Go pattern** as AIAnalysis:

**File**: `test/infrastructure/authwebhook.go`

```go
package infrastructure

import (
	"context"
	"os/exec"
	"fmt"
	"time"
)

const (
	// Container names (DD-INTEGRATION-001 v2.0 naming convention)
	AuthWebhookIntegrationPostgresContainer   = "authwebhook_postgres_1"
	AuthWebhookIntegrationRedisContainer      = "authwebhook_redis_1"
	AuthWebhookIntegrationDataStorageContainer = "authwebhook_datastorage_1"

	// Ports (avoid conflicts with other services)
	AuthWebhookIntegrationDataStoragePort = 18099
	AuthWebhookIntegrationPostgresPort    = 15435
	AuthWebhookIntegrationRedisPort       = 16381
)

// StartAuthWebhookIntegrationInfrastructure starts PostgreSQL, Redis, and Data Storage
// for webhook integration tests using programmatic podman commands.
func StartAuthWebhookIntegrationInfrastructure(ctx context.Context) error {
	// 1. Create network
	// 2. Start PostgreSQL
	// 3. Start Redis
	// 4. Start Data Storage
	// 5. Wait for health checks

	// Follow EXACT pattern from test/infrastructure/aianalysis.go
	return nil
}

// StopAuthWebhookIntegrationInfrastructure stops all infrastructure containers
func StopAuthWebhookIntegrationInfrastructure() error {
	// Stop containers in reverse order
	return nil
}
```

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

---

## 🎯 **COMPLIANCE WITH DD-TEST-002**

All webhook test targets **MUST** use parallel execution flags:

| Test Tier | Command | Parallel Flag | Compliance |
|-----------|---------|---------------|------------|
| **Unit** | `ginkgo` | `--procs=$(TEST_PROCS)` | ✅ DD-TEST-002 |
| **Integration** | `ginkgo` | `--procs=$(TEST_PROCS)` | ✅ DD-TEST-002 |
| **E2E** | `ginkgo` | `--procs=$(TEST_PROCS)` | ✅ DD-TEST-002 |
| **Unit Coverage** | `go test` | `-p $(TEST_PROCS)` | ✅ DD-TEST-002 |
| **Integration Coverage** | `go test` | `-p $(TEST_PROCS)` | ✅ DD-TEST-002 |

**Note**: `TEST_PROCS` is automatically detected:
```makefile
# Lines 32-39: Dynamically detect CPU cores
TEST_PROCS ?= $(shell nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)
```

---

## ✅ **TRIAGE SUMMARY**

| Item | Status | Action Required |
|------|--------|-----------------|
| **`cmd/authwebhook/` exists?** | ❌ NOT YET | Needs to be implemented (TDD) |
| **Pattern-based targets will work** | ✅ FUTURE | Once `cmd/authwebhook/main.go` exists |
| **Explicit targets needed NOW** | ✅ REQUIRED | For immediate TDD testability |
| **Webhook architecture** | ✅ CLARIFIED | Standalone service (not just shared lib) |
| **Programmatic infrastructure** | ✅ REQUIRED | Follow AIAnalysis pattern |
| **Parallel execution** | ✅ REQUIRED | `--procs=$(TEST_PROCS)` per DD-TEST-002 |
| **Coverage collection** | ✅ REQUIRED | Unit, Integration, E2E variants |
| **Makefile insertion location** | ✅ IDENTIFIED | After HolmesGPT special cases |

---

## 📝 **NEXT STEPS**

### **Phase 1: Create Webhook Service** (NEW - Must happen first)
1. ⬜ **Create `cmd/authwebhook/main.go`** (standalone webhook server)
2. ⬜ **Create `cmd/authwebhook/Dockerfile`** (for K8s deployment)
3. ⬜ **Create webhook configuration** (`config/authwebhook.yaml`)
4. ⬜ **Verify webhook is auto-discovered** by Makefile pattern-based targets

**Once webhook service exists, pattern-based targets work automatically**:
- `make build-authwebhook` (builds binary)
- `make test-unit-authwebhook` (runs unit tests)
- `make test-integration-authwebhook` (runs integration tests)
- `make test-e2e-authwebhook` (runs E2E tests)

### **Phase 2: Add Explicit Targets for TDD** (During implementation)
1. ⬜ **Add webhook Makefile targets** (6 explicit targets for immediate testability)
2. ⬜ **Create `test/infrastructure/authwebhook.go`** (programmatic infrastructure setup)
3. ⬜ **Create test suite files** (`test/unit/authwebhook/suite_test.go`, etc.)
4. ⬜ **Verify port allocation** (avoid conflicts with existing services)
5. ⬜ **Test execution** (`make test-all-authwebhook`)

### **Phase 3: Cleanup** (After webhook is fully implemented)
- ⬜ **Optional**: Remove explicit targets if pattern-based targets are sufficient
- ⬜ **Optional**: Keep explicit targets for additional coverage variants

---

## 🔗 **REFERENCES**

- **DD-INTEGRATION-001 v2.0**: Programmatic infrastructure pattern
- **DD-TEST-002**: Parallel test execution standard (4 procs)
- **DD-TEST-007**: E2E binary coverage collection
- **Existing Pattern**: Lines 110-141 (Makefile pattern-based targets)
- **Existing Special Case**: Lines 226-437 (HolmesGPT explicit targets)
- **Existing Infrastructure**: `test/infrastructure/aianalysis.go` (programmatic setup)

