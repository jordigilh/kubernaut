# Makefile - Consolidated Pattern-Based Build System
# Last Consolidated: December 29, 2025
# Total Targets: ~40 (down from 139)

##@ Configuration

# Image URL to use all building/pushing image targets
IMG ?= controller:latest

# Force Go to use modules directly, not vendor directory
export GOFLAGS := -mod=mod

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# Service auto-discovery from cmd/ directory
SERVICES := $(filter-out README.md must-gather, $(notdir $(wildcard cmd/*)))
# Result: aianalysis authwebhook datastorage gateway notification remediationorchestrator signalprocessing workflowexecution
# Note: must-gather is a bash tool, built separately via cmd/must-gather/Makefile

# Test configuration
# Dynamically detect CPU cores (works on Linux and macOS)
# Linux (GitHub Actions): nproc
# macOS: sysctl -n hw.ncpu
# Fallback to 4 if detection fails
TEST_PROCS ?= $(shell nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)
TEST_TIMEOUT_UNIT ?= 5m
TEST_TIMEOUT_INTEGRATION ?= 15m
TEST_TIMEOUT_E2E ?= 30m

##@ General

.PHONY: all
all: build-all

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
	@echo ""
	@echo "Available Services: $(SERVICES)"
	@echo ""
	@echo "Pattern-Based Targets:"
	@echo "  test-unit-<service>          Run unit tests for any service"
	@echo "  test-integration-<service>   Run integration tests for any service"
	@echo "  test-e2e-<service>           Run E2E tests for any service"
	@echo "  test-all-<service>           Run all test tiers for any service"
	@echo "  build-<service>              Build service binary"
	@echo ""
	@echo "Examples:"
	@echo "  make test-unit-gateway"
	@echo "  make test-integration-workflowexecution"
	@echo "  make build-notification"

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd:allowDangerousTypes=true webhook paths="./api/..." paths="./internal/controller/..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ogen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/..."
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./pkg/shared/types/..."
	@echo "📋 Generating OpenAPI spec copies for embedding (DD-API-002)..."
	@go generate ./pkg/datastorage/server/middleware/...
	@go generate ./pkg/audit/...
	@echo "📋 Generating HolmesGPT-API client (ogen)..."
	@PATH="$(LOCALBIN):$$PATH" go generate ./pkg/holmesgpt/client/...
	@echo "✅ Generation complete"

.PHONY: generate-datastorage-client
generate-datastorage-client: ogen ## Generate DataStorage OpenAPI client from spec (DD-API-001)
	@echo "📋 Generating DataStorage clients (Go + Python) from api/openapi/data-storage-v1.yaml..."
	@echo ""
	@echo "🔧 [1/2] Generating Go client with ogen..."
	@go generate ./pkg/datastorage/ogen-client/...
	@echo "✅ Go client generated: pkg/datastorage/ogen-client/oas_*_gen.go"
	@echo ""
	@echo "🔧 [2/2] Generating Python client..."
	@rm -rf holmesgpt-api/src/clients/datastorage
	@podman run --rm -v "$(PWD)":/local:z openapitools/openapi-generator-cli:v7.2.0 generate \
		-i /local/api/openapi/data-storage-v1.yaml \
		-g python \
		-o /local/holmesgpt-api/src/clients/datastorage \
		--package-name datastorage \
		--additional-properties=packageVersion=1.0.0
	@echo "✅ Python client generated: holmesgpt-api/src/clients/datastorage/"
	@echo ""
	@echo "✨ Both clients generated successfully!"
	@echo "   Go (ogen):  pkg/datastorage/ogen-client/"
	@echo "   Python:     holmesgpt-api/src/clients/datastorage/"
	@echo "   Spec:       api/openapi/data-storage-v1.yaml"

.PHONY: generate-holmesgpt-client
generate-holmesgpt-client: ogen ## Generate HolmesGPT-API client from OpenAPI spec
	@echo "📋 Generating HolmesGPT-API client from holmesgpt-api/api/openapi.json..."
	@go generate ./pkg/holmesgpt/client/...
	@echo "✅ HolmesGPT-API client generated successfully"

.PHONY: fmt
fmt: ## Format code
	@echo "Formatting Go code..."
	go fmt ./api/... ./cmd/... ./internal/... ./pkg/...

.PHONY: vet
vet: ## Run go vet against code
	go vet ./api/... ./cmd/... ./internal/... ./pkg/...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint
	$(GOLANGCI_LINT) run ./...

.PHONY: clean
clean: ## Clean build artifacts
	@echo "Cleaning Go artifacts..."
	rm -rf bin/*
	rm -f coverage.out coverage.html
	@echo "✅ Cleanup complete"

##@ Pattern-Based Service Targets

# Coverage Directory Setup
.PHONY: ensure-coverage-dirs
ensure-coverage-dirs: ## Ensure coverage directories exist for all test tiers
	@mkdir -p coverdata coverage-reports
	@chmod -f 777 coverdata coverage-reports 2>/dev/null || true

# Unit Tests
.PHONY: test-unit-%
test-unit-%: ginkgo ensure-coverage-dirs ## Run unit tests for specified service (e.g., make test-unit-gateway)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 $* - Unit Tests ($(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_UNIT) --procs=$(TEST_PROCS) --coverprofile=coverage_unit_$*.out --covermode=atomic ./test/unit/$*/...
	@if [ -f coverage_unit_$*.out ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_unit_$*.out"; \
		go tool cover -func=coverage_unit_$*.out | grep total || echo "No coverage data"; \
	fi

# Integration Tests
.PHONY: test-integration-%
test-integration-%: generate ginkgo setup-envtest ensure-coverage-dirs ## Run integration tests for specified service (e.g., make test-integration-gateway)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 $* - Integration Tests ($(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Pattern: DD-INTEGRATION-001 v2.0 (envtest + Podman dependencies)"
	@KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" $(GINKGO) -v --timeout=$(TEST_TIMEOUT_INTEGRATION) --procs=$(TEST_PROCS) --coverprofile=coverage_integration_$*.out --covermode=atomic --keep-going ./test/integration/$*/...
	@if [ -f coverage_integration_$*.out ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_integration_$*.out"; \
		go tool cover -func=coverage_integration_$*.out | grep total || echo "No coverage data"; \
	fi


# E2E Tests
.PHONY: test-e2e-%
test-e2e-%: generate ginkgo ensure-coverage-dirs ## Run E2E tests for specified service (e.g., make test-e2e-workflowexecution)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 $* - E2E Tests (Kind cluster, $(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@# Pre-generate DataStorage client to catch spec inconsistencies (DD-API-001)
	@if [ "$*" = "datastorage" ]; then \
		echo "🔍 Pre-validating DataStorage OpenAPI client generation..."; \
		$(MAKE) generate-datastorage-client || { \
			echo "❌ DataStorage client generation failed - OpenAPI spec may be invalid"; \
			exit 1; \
		}; \
		echo "✅ DataStorage client validated successfully"; \
	fi
	@GINKGO_CMD="$(GINKGO) -v --timeout=$(TEST_TIMEOUT_E2E) --procs=$(TEST_PROCS) --coverprofile=coverage_e2e_$*.out --covermode=atomic"; \
	if [ -n "$(GINKGO_LABEL)" ]; then \
		GINKGO_CMD="$$GINKGO_CMD --label-filter='$(GINKGO_LABEL)'"; \
		echo "🏷️  Label filter: $(GINKGO_LABEL)"; \
	fi; \
	if [ -n "$(GINKGO_FOCUS)" ]; then \
		GINKGO_CMD="$$GINKGO_CMD --focus='$(GINKGO_FOCUS)'"; \
		echo "🔍 Focusing on: $(GINKGO_FOCUS)"; \
	fi; \
	if [ -n "$(GINKGO_SKIP)" ]; then \
		GINKGO_CMD="$$GINKGO_CMD --skip='$(GINKGO_SKIP)'"; \
		echo "⏭️  Skipping: $(GINKGO_SKIP)"; \
	fi; \
	eval "$$GINKGO_CMD ./test/e2e/$*/..."
	@if [ -f coverage_e2e_$*.out ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_e2e_$*.out"; \
		go tool cover -func=coverage_e2e_$*.out | grep total || echo "No coverage data"; \
	fi

# All Tests for Service
.PHONY: test-all-%
test-all-%: ## Run all test tiers for specified service (e.g., make test-all-gateway)
	@echo "═══════════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Running ALL $* Tests (3 tiers)"
	@echo "═══════════════════════════════════════════════════════════════════════════════"
	@FAILED=0; \
	$(MAKE) test-unit-$* || FAILED=$$((FAILED + 1)); \
	$(MAKE) test-integration-$* || FAILED=$$((FAILED + 1)); \
	$(MAKE) test-e2e-$* || FAILED=$$((FAILED + 1)); \
	if [ $$FAILED -gt 0 ]; then \
		echo "❌ $$FAILED test tier(s) failed"; \
		exit 1; \
	fi

# Build Service Binary
.PHONY: build-%
build-%: ## Build specified service binary (e.g., make build-gateway)
	@echo "🔨 Building $* service..."
	@CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o bin/$* ./cmd/$*
	@echo "✅ Built: bin/$*"

##@ Tier Aggregations

.PHONY: test-tier-unit
test-tier-unit: $(addprefix test-unit-,$(SERVICES)) ## Run unit tests for all services

.PHONY: test-tier-integration
test-tier-integration: $(addprefix test-integration-,$(SERVICES)) ## Run integration tests for all services

.PHONY: test-tier-e2e
test-tier-e2e: ensure-coverage-dirs $(addprefix test-e2e-,$(SERVICES)) ## Run E2E tests for all services

.PHONY: test-all-services
test-all-services: $(addprefix test-all-,$(SERVICES)) ## Run all tests for all services

.PHONY: build-all-services
build-all-services: $(addprefix build-,$(SERVICES)) ## Build all Go services

.PHONY: build-all
build-all: build-all-services ## Build all services (alias)

##@ Docker Pattern Targets

.PHONY: docker-build-%
docker-build-%: ## Build service container image (e.g., make docker-build-gateway)
	@echo "🐳 Building Docker image for $*..."
	@$(CONTAINER_TOOL) build -t $(IMG) -f cmd/$*/Dockerfile .

.PHONY: docker-push-%
docker-push-%: docker-build-% ## Push service container image
	@echo "🐳 Pushing Docker image for $*..."
	@$(CONTAINER_TOOL) push $(IMG)

##@ Cleanup Pattern Targets

.PHONY: clean-%-integration
clean-%-integration: ## Clean integration test infrastructure for service
	@echo "🧹 Cleaning $* integration infrastructure..."
	@podman stop $*_postgres_1 $*_redis_1 $*_datastorage_1 2>/dev/null || true
	@podman rm $*_postgres_1 $*_redis_1 $*_datastorage_1 2>/dev/null || true
	@podman network rm $*_test-network 2>/dev/null || true
	@echo "✅ Cleanup complete"

.PHONY: clean-integration-all
clean-integration-all: $(addprefix clean-,$(addsuffix -integration,$(SERVICES))) ## Clean all integration infrastructures

.PHONY: clean-%-test-ports
clean-%-test-ports: ## Kill processes on test ports for service
	@echo "🧹 Cleaning test ports for $*..."
	@lsof -ti:8080,8081,5432,6379 | xargs kill -9 2>/dev/null || true
	@echo "✅ Test ports cleaned"

##@ Coverage Pattern Targets

.PHONY: test-coverage-%
test-coverage-%: ## Run unit tests with coverage for service
	@echo "📊 Running unit tests with coverage for $*..."
	@cd test/unit/$* && \
		go test -v -coverprofile=coverage.out -covermode=atomic ./... && \
		go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report: test/unit/$*/coverage.html"

##@ Special Cases - HolmesGPT (Python Service)

.PHONY: build-holmesgpt-api
build-holmesgpt-api: ## Build holmesgpt-api for local development (pip install)
	@echo "🐍 Building holmesgpt-api for local development..."
	@cd holmesgpt-api && pip install -e .

.PHONY: build-holmesgpt-api-image
build-holmesgpt-api-image: ## Build holmesgpt-api Docker image (PRODUCTION - full dependencies)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🐳 Building HolmesGPT API Docker Image (PRODUCTION)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📦 Dockerfile: holmesgpt-api/Dockerfile"
	@echo "📋 Requirements: requirements.txt (full dependencies)"
	@echo "💾 Size: ~2.5GB (includes google-cloud-aiplatform 1.5GB)"
	@echo "🎯 Use Case: Production deployments, Quay.io releases"
	@echo ""
	@cd holmesgpt-api && podman build \
		--platform linux/amd64,linux/arm64 \
		-t localhost/kubernaut-holmesgpt-api:latest \
		-t localhost/kubernaut-holmesgpt-api:$$(git rev-parse --short HEAD) \
		-f Dockerfile \
		.
	@echo ""
	@echo "✅ Production image built successfully!"
	@echo "   Tags: localhost/kubernaut-holmesgpt-api:latest"
	@echo "         localhost/kubernaut-holmesgpt-api:$$(git rev-parse --short HEAD)"
	@echo ""
	@echo "📤 To push to Quay.io:"
	@echo "   podman tag localhost/kubernaut-holmesgpt-api:latest quay.io/YOUR_ORG/kubernaut-holmesgpt-api:VERSION"
	@echo "   podman push quay.io/YOUR_ORG/kubernaut-holmesgpt-api:VERSION"

.PHONY: build-holmesgpt-api-image-e2e
build-holmesgpt-api-image-e2e: ## Build holmesgpt-api Docker image (E2E - minimal dependencies)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🐳 Building HolmesGPT API Docker Image (E2E)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📦 Dockerfile: holmesgpt-api/Dockerfile.e2e"
	@echo "📋 Requirements: requirements-e2e.txt (minimal dependencies)"
	@echo "💾 Size: ~800MB (excludes google-cloud-aiplatform 1.5GB)"
	@echo "🎯 Use Case: E2E testing, CI/CD"
	@echo ""
	@cd holmesgpt-api && podman build \
		--platform linux/amd64,linux/arm64 \
		-t localhost/kubernaut-holmesgpt-api:e2e \
		-t localhost/kubernaut-holmesgpt-api:e2e-$$(git rev-parse --short HEAD) \
		-f Dockerfile.e2e \
		.
	@echo ""
	@echo "✅ E2E image built successfully!"
	@echo "   Tags: localhost/kubernaut-holmesgpt-api:e2e"
	@echo "         localhost/kubernaut-holmesgpt-api:e2e-$$(git rev-parse --short HEAD)"

.PHONY: export-openapi-holmesgpt-api
export-openapi-holmesgpt-api: ## Export holmesgpt-api OpenAPI spec from FastAPI (ADR-045)
	@echo "📄 Exporting OpenAPI spec from FastAPI app..."
	@cd holmesgpt-api && mkdir -p api
	@cd holmesgpt-api && python3 -c "from src.main import app; import json; print(json.dumps(app.openapi(), indent=2))" > api/openapi.json
	@echo "✅ OpenAPI spec exported: holmesgpt-api/api/openapi.json"
	@cd holmesgpt-api && echo "📊 Schema count: $$(python3 -c \"import json; spec=json.load(open('api/openapi.json')); print(len(spec.get('components', {}).get('schemas', {})))\")"

.PHONY: validate-openapi-holmesgpt-api
validate-openapi-holmesgpt-api: export-openapi-holmesgpt-api ## Validate holmesgpt-api OpenAPI spec is committed (CI - ADR-045)
	@echo "🔍 Validating OpenAPI spec is up-to-date..."
	@cd holmesgpt-api && \
	if ! git diff --quiet api/openapi.json; then \
		echo ""; \
		echo "❌ OpenAPI spec drift detected!"; \
		echo ""; \
		echo "The generated OpenAPI spec differs from the committed version."; \
		echo ""; \
		echo "📋 Changes:"; \
		git diff api/openapi.json | head -50; \
		echo ""; \
		echo "🔧 To fix:"; \
		echo "  1. Run: make export-openapi-holmesgpt-api"; \
		echo "  2. Review: git diff holmesgpt-api/api/openapi.json"; \
		echo "  3. Commit: git add holmesgpt-api/api/openapi.json"; \
		echo ""; \
		exit 1; \
	fi
	@echo "✅ OpenAPI spec is up-to-date and committed"

.PHONY: validate-openapi-datastorage
validate-openapi-datastorage: ## Validate Data Storage OpenAPI spec syntax (CI - ADR-031)
	@echo "🔍 Validating Data Storage OpenAPI spec..."
	@docker run --rm -v "$(PWD):/local" openapitools/openapi-generator-cli:v7.2.0 validate \
		-i /local/api/openapi/data-storage-v1.yaml || \
		(echo "❌ OpenAPI spec validation failed!" && exit 1)
	@echo "✅ Data Storage OpenAPI spec is valid"

.PHONY: lint-holmesgpt-api
lint-holmesgpt-api: ## Run ruff linter on holmesgpt-api Python code
	@echo "🔍 Running ruff linter on holmesgpt-api..."
	@cd holmesgpt-api && ruff check src/ tests/
	@echo "✅ Linting complete"

.PHONY: clean-holmesgpt-api
clean-holmesgpt-api: ## Clean holmesgpt-api Python artifacts
	@echo "🧹 Cleaning holmesgpt-api Python artifacts..."
	@cd holmesgpt-api && rm -rf htmlcov/ .pytest_cache/ __pycache__/
	@cd holmesgpt-api && find . -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
	@cd holmesgpt-api && find . -type f -name "*.pyc" -delete 2>/dev/null || true
	@echo "✅ Cleaned holmesgpt-api artifacts"

.PHONY: test-integration-holmesgpt-api
test-integration-holmesgpt-api: ginkgo setup-envtest clean-holmesgpt-test-ports ensure-coverage-dirs ## Run holmesgpt-api integration tests (direct business logic calls)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🐍 HolmesGPT API Integration Tests (Direct Business Logic)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Pattern: Direct business logic calls (matches Go service testing)"
	@echo "🐍 Test Logic: Python calls src.extensions.*.llm_integration directly (no HTTP)"
	@echo "⏱️  Expected Duration: ~2 minutes (no HAPI container needed)"
	@echo ""
	@echo "🔧 Phase 0: Generating HAPI OpenAPI client (DD-API-001)..."
	@cd holmesgpt-api/tests/integration && bash generate-client.sh && cd ../../.. || (echo "❌ Client generation failed"; exit 1)
	@echo "✅ OpenAPI client generated (used for Data Storage audit validation only)"
	@echo ""
	@# FIX: HAPI-INT-CONFIG-001 - Run as standard Ginkgo test with envtest
	@# Architecture: Go sets up infrastructure (envtest + PostgreSQL + Redis + DataStorage with auth)
	@#              Python tests run in container via coordination test
	@echo "🏗️  Running HAPI integration tests (hybrid Go + Python pattern)..."
	@echo "   Pattern: DD-INTEGRATION-001 v2.0 + DD-AUTH-014 (envtest + auth)"
	@echo "   Infrastructure: Go (envtest, PostgreSQL, Redis, DataStorage with auth)"
	@echo "   Tests: Python (pytest in container, business logic calls)"
	@echo ""
	@KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" $(GINKGO) -v --timeout=20m --procs=1 --coverprofile=coverage_integration_holmesgpt-api.out --covermode=atomic ./test/integration/holmesgptapi/...
	@if [ -f coverage_integration_holmesgpt-api.out ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_integration_holmesgpt-api.out"; \
		go tool cover -func=coverage_integration_holmesgpt-api.out | grep total || echo "No coverage data"; \
	fi

.PHONY: test-e2e-holmesgpt-api
test-e2e-holmesgpt-api: ginkgo ensure-coverage-dirs ## Run holmesgpt-api E2E tests (Kind cluster + Python tests in container, ~10 min)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 HolmesGPT API E2E Tests (Kind Cluster + Containerized Python Tests)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Pattern: DD-INTEGRATION-001 v2.0 (Go-bootstrapped Kind infrastructure)"
	@echo "🐍 Test Logic: Python pytest in container (Red Hat UBI9, same as unit tests)"
	@echo "⏱️  Expected Duration: ~10 minutes"
	@echo ""
	@echo "🔧 Step 1: Generate OpenAPI client (DD-HAPI-005)..."
	@cd holmesgpt-api/tests/integration && bash generate-client.sh && cd ../.. || exit 1
	@echo "✅ Client generated successfully"
	@echo ""
	@echo "🧪 Step 2: Run E2E tests (Go infrastructure + Python tests in UBI9 container)..."
	@echo "   Container: registry.access.redhat.com/ubi9/python-312:latest"
	@echo "   Network: host (access NodePort services: HAPI 30120, DS 30098)"
	@cd test/e2e/holmesgpt-api && $(GINKGO) -v --timeout=15m --coverprofile=../../../coverage_e2e_holmesgpt-api.out --covermode=atomic
	@if [ -f coverage_e2e_holmesgpt-api.out ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_e2e_holmesgpt-api.out"; \
		go tool cover -func=coverage_e2e_holmesgpt-api.out | grep total || echo "No coverage data"; \
	fi
	@echo ""
	@echo "✅ All HAPI E2E tests completed"

.PHONY: test-all-holmesgpt-api
test-all-holmesgpt-api: test-unit-holmesgpt-api test-integration-holmesgpt-api test-e2e-holmesgpt-api ## Run all holmesgpt-api test tiers (Unit + Integration + E2E)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "✅ All holmesgpt-api test tiers completed successfully!"
	@echo "════════════════════════════════════════════════════════════════════════"

.PHONY: test-unit-holmesgpt-api
test-unit-holmesgpt-api: ensure-coverage-dirs ## Run holmesgpt-api unit tests (containerized with UBI)
	@echo "🧪 Running holmesgpt-api unit tests (containerized with Red Hat UBI)..."
	@podman run --rm \
		-v $(CURDIR):/workspace:z \
		-w /workspace/holmesgpt-api \
		registry.access.redhat.com/ubi9/python-312:latest \
		sh -c "pip install -q -r requirements.txt && pip install -q -r requirements-test.txt && pytest tests/unit/ -v --durations=20 --cov=src --cov-report=term --cov-report=term-missing | tee /workspace/coverage_unit_holmesgpt-api.txt"
	@if [ -f coverage_unit_holmesgpt-api.txt ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_unit_holmesgpt-api.txt"; \
		grep "TOTAL" coverage_unit_holmesgpt-api.txt || echo "No coverage data"; \
	fi

.PHONY: clean-holmesgpt-test-ports
clean-holmesgpt-test-ports: ## Clean up any stale HAPI integration test containers
	@echo "🧹 Cleaning up HAPI integration test containers..."
	@echo "   Container names: holmesgptapi_* (per DD-INTEGRATION-001 v2.0)"
	@podman stop holmesgptapi_postgres_1 holmesgptapi_redis_1 holmesgptapi_datastorage_1 2>/dev/null || true
	@podman rm holmesgptapi_postgres_1 holmesgptapi_redis_1 holmesgptapi_datastorage_1 holmesgptapi_migrations 2>/dev/null || true
	@podman network rm holmesgptapi_test-network 2>/dev/null || true
	@rm -f /tmp/hapi-integration-tests-complete
	@echo "✅ Container cleanup complete"

.PHONY: test-integration-holmesgpt-cleanup
test-integration-holmesgpt-cleanup: clean-holmesgpt-test-ports ## Complete cleanup of HAPI integration infrastructure
	@echo "🧹 Complete HAPI integration infrastructure cleanup..."
	@podman image prune -f --filter "label=test=holmesgptapi" 2>/dev/null || true
	@echo "✅ Complete cleanup done (containers + images)"

##@ Special Cases - Authentication Webhook

.PHONY: test-unit-authwebhook
test-unit-authwebhook: ginkgo ensure-coverage-dirs ## Run authentication webhook unit tests
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Authentication Webhook - Unit Tests ($(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_UNIT) --procs=$(TEST_PROCS) --coverprofile=coverage_unit_authwebhook.out --covermode=atomic ./test/unit/authwebhook/...
	@if [ -f coverage_unit_authwebhook.out ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_unit_authwebhook.out"; \
		go tool cover -func=coverage_unit_authwebhook.out | grep total || echo "No coverage data"; \
	fi

# test-integration-authwebhook now uses the general test-integration-% pattern (no override needed)
.PHONY: test-e2e-authwebhook
test-e2e-authwebhook: ginkgo ensure-coverage-dirs ## Run webhook E2E tests (Kind cluster)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Authentication Webhook - E2E Tests (Kind cluster, $(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_E2E) --procs=$(TEST_PROCS) --coverprofile=coverage_e2e_authwebhook.out --covermode=atomic ./test/e2e/authwebhook/...
	@if [ -f coverage_e2e_authwebhook.out ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_e2e_authwebhook.out"; \
		go tool cover -func=coverage_e2e_authwebhook.out | grep total || echo "No coverage data"; \
	fi

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

.PHONY: clean-authwebhook-integration
clean-authwebhook-integration: ## Clean webhook integration test infrastructure
	@echo "🧹 Cleaning webhook integration infrastructure..."
	@podman stop authwebhook_postgres_1 authwebhook_redis_1 authwebhook_datastorage_1 2>/dev/null || true
	@podman rm authwebhook_postgres_1 authwebhook_redis_1 authwebhook_datastorage_1 2>/dev/null || true
	@podman network rm authwebhook_test-network 2>/dev/null || true
	@echo "✅ Cleanup complete"

##@ Legacy Aliases (Backward Compatibility)

.PHONY: test-gateway
test-gateway: test-integration-gateway ## Legacy alias for Gateway integration tests

.PHONY: test
test: test-tier-unit ## Legacy alias: Run all unit tests

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
OGEN ?= $(LOCALBIN)/ogen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
GINKGO ?= $(LOCALBIN)/ginkgo

## Tool Versions
KUSTOMIZE_VERSION ?= v5.6.0
CONTROLLER_TOOLS_VERSION ?= v0.19.0
OGEN_VERSION ?= v1.18.0
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')
GOLANGCI_LINT_VERSION ?= v2.1.0
GINKGO_VERSION ?= v2.27.2

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: ogen
ogen: $(OGEN) ## Download ogen locally if necessary
$(OGEN): $(LOCALBIN)
	$(call go-install-tool,$(OGEN),github.com/ogen-go/ogen/cmd/ogen,$(OGEN_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory
	@echo "📦 Setting up ENVTEST binaries..."
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path
	@echo "✅ ENVTEST binaries installed in $(LOCALBIN)"

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: ginkgo
ginkgo: $(GINKGO) ## Download ginkgo locally if necessary
$(GINKGO): $(LOCALBIN)
	$(call go-install-tool,$(GINKGO),github.com/onsi/ginkgo/v2/ginkgo,$(GINKGO_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef

##@ Cursor Rule Compliance

.PHONY: lint-rules
lint-rules: lint-test-patterns lint-business-integration lint-tdd-compliance ## Run all cursor rule compliance checks

.PHONY: lint-test-patterns
lint-test-patterns: ## Check for test anti-patterns
	@echo "🔍 Checking for test anti-patterns..."
	@./scripts/validation/check-test-anti-patterns.sh

.PHONY: lint-business-integration
lint-business-integration: ## Check business code integration in main applications
	@echo "🔍 Checking business code integration..."
	@./scripts/validation/check-business-integration.sh

.PHONY: lint-tdd-compliance
lint-tdd-compliance: ## Check TDD compliance (BDD framework, BR references)
	@echo "🔍 Checking TDD compliance..."
	@./scripts/validation/check-tdd-compliance.sh
