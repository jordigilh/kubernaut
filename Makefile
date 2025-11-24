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
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

##@ Gateway Integration Tests

.PHONY: test-gateway
test-gateway: ## Run Gateway integration tests (envtest + Podman)
	@echo "🧪 Running Gateway integration tests with 2 parallel processors (envtest + Podman)..."
	@cd test/integration/gateway && ginkgo -v --procs=2

##@ Notification Service Integration Tests

.PHONY: test-integration-notification
test-integration-notification: ## Run Notification Service integration tests (Kind bootstrapped via Go)
	@echo "🧪 Running Notification Service integration tests..."
	@go test ./test/integration/notification/... -v -ginkgo.v -timeout=15m

##@ Service-Specific Integration Tests

.PHONY: test-integration-holmesgpt
test-integration-holmesgpt: ## Run HolmesGPT API integration tests (Python/pytest, ~1 min)
	@echo "🧪 Running HolmesGPT API integration tests..."
	@cd holmesgpt-api && \
		pip install -q -r requirements.txt && \
		pip install -q -r requirements-dev.txt && \
		MOCK_LLM=true pytest tests/integration/ -v --tb=short

.PHONY: test-integration-datastorage
test-integration-datastorage: ## Run Data Storage integration tests (PostgreSQL 16 via Podman, ~4 min)
	@if [ -z "$$POSTGRES_HOST" ]; then \
		echo "🔧 Starting PostgreSQL 16 with pgvector 0.5.1+ extension..."; \
		podman run -d --name datastorage-postgres -p 5432:5432 \
		-e POSTGRES_PASSWORD=postgres \
		-e POSTGRES_SHARED_BUFFERS=1GB \
			quay.io/jordigilh/pgvector:pg16 > /dev/null 2>&1 || \
		(echo "⚠️  PostgreSQL container already exists or failed to start" && \
			 podman start datastorage-postgres > /dev/null 2>&1) || true; \
		echo "⏳ Waiting for PostgreSQL to be ready..."; \
		sleep 5; \
		podman exec datastorage-postgres pg_isready -U postgres > /dev/null 2>&1 || \
			(echo "❌ PostgreSQL not ready" && exit 1); \
		echo "✅ PostgreSQL 16 ready"; \
	else \
		echo "🐳 Using external PostgreSQL at $$POSTGRES_HOST:$$POSTGRES_PORT (Docker Compose)"; \
	fi
	@if [ -z "$$POSTGRES_HOST" ]; then \
		echo "🔍 Verifying PostgreSQL and pgvector versions..."; \
		podman exec datastorage-postgres psql -U postgres -c "SELECT version();" | grep "PostgreSQL 16" || \
			(echo "❌ PostgreSQL version is not 16.x" && exit 1); \
		echo "🔧 Creating pgvector extension..."; \
		podman exec datastorage-postgres psql -U postgres -c "CREATE EXTENSION IF NOT EXISTS vector;" > /dev/null 2>&1 || \
			(echo "❌ Failed to create pgvector extension" && exit 1); \
		podman exec datastorage-postgres psql -U postgres -c "SELECT extversion FROM pg_extension WHERE extname = 'vector';" | grep -E "0\.[5-9]\.[1-9]|0\.[6-9]\.0|0\.[7-9]\.0|0\.[8-9]\.0" || \
			(echo "❌ pgvector version is not 0.5.1+" && exit 1); \
		echo "✅ Version validation passed (PostgreSQL 16 + pgvector 0.5.1+)"; \
		echo "🔍 Testing HNSW index creation (dry-run)..."; \
		podman exec datastorage-postgres psql -U postgres -d postgres -c "\
		CREATE TEMP TABLE hnsw_validation_test (id SERIAL PRIMARY KEY, embedding vector(384)); \
		CREATE INDEX hnsw_validation_test_idx ON hnsw_validation_test USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64);" \
		> /dev/null 2>&1 || \
			(echo "❌ HNSW index creation test failed - PostgreSQL/pgvector may not support HNSW" && exit 1); \
		echo "✅ HNSW index support verified"; \
	fi
	@echo "🧪 Running Data Storage integration tests..."
	@if [ -z "$$POSTGRES_HOST" ]; then \
		TEST_RESULT=0; \
	go test ./test/integration/datastorage/... -v -timeout 5m || TEST_RESULT=$$?; \
	echo "🧹 Cleaning up PostgreSQL container..."; \
	podman stop datastorage-postgres > /dev/null 2>&1 || true; \
	podman rm datastorage-postgres > /dev/null 2>&1 || true; \
	echo "✅ Cleanup complete"; \
		exit $$TEST_RESULT; \
	else \
		go test ./test/integration/datastorage/... -v -timeout 5m; \
	fi

.PHONY: test-integration-ai
test-integration-ai: ## Run AI Service integration tests (Redis via Podman, ~15s)
	@echo "🔧 Starting Redis cache..."
	@podman run -d --name ai-redis -p 6379:6379 quay.io/jordigilh/redis:7-alpine > /dev/null 2>&1 || \
		(echo "⚠️  Redis container already exists or failed to start" && \
		 podman start ai-redis > /dev/null 2>&1) || true
	@echo "⏳ Waiting for Redis to be ready..."
	@sleep 2
	@echo "✅ Redis ready"
	@echo "🧪 Running AI Service integration tests..."
	@TEST_RESULT=0; \
	go test ./test/integration/ai/... -v -timeout 5m || TEST_RESULT=$$?; \
	echo "🧹 Cleaning up Redis container..."; \
	podman stop ai-redis > /dev/null 2>&1 || true; \
	podman rm ai-redis > /dev/null 2>&1 || true; \
	echo "✅ Cleanup complete"; \
	exit $$TEST_RESULT

.PHONY: test-integration-toolset
test-integration-toolset: ## Run Dynamic Toolset integration tests (Kind bootstrapped via Go)
	@echo "🧪 Running Dynamic Toolset integration tests..."
	@go test ./test/integration/toolset/... -v -timeout 10m

.PHONY: test-integration-gateway-service
test-integration-gateway-service: test-gateway ## Run Gateway Service integration tests (alias for test-gateway)

.PHONY: test-integration-service-all
test-integration-service-all: ## Run ALL service-specific integration tests (sequential)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🚀 Running ALL Service-Specific Integration Tests"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo ""
	@echo "📊 Test Plan:"
	@echo "  1. Data Storage (Podman: PostgreSQL + pgvector) - ~30s"
	@echo "  2. AI Service (Podman: Redis) - ~15s"
	@echo "  3. Dynamic Toolset (Kind bootstrapped via Go) - ~3-5min"
	@echo "  4. Gateway Service (Kind bootstrapped via Go) - ~3-5min"
	@echo "  5. Notification Service (Kind bootstrapped via Go) - ~3-5min"
	@echo ""
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo ""
	@FAILED=0; \
	echo ""; \
	echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"; \
	echo "1️⃣  Data Storage Service (Podman)"; \
	echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"; \
	$(MAKE) test-integration-datastorage || FAILED=$$((FAILED + 1)); \
	echo ""; \
	echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"; \
	echo "2️⃣  AI Service (Podman)"; \
	echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"; \
	$(MAKE) test-integration-ai || FAILED=$$((FAILED + 1)); \
	echo ""; \
	echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"; \
	echo "3️⃣  Dynamic Toolset Service (Kind)"; \
	echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"; \
	$(MAKE) test-integration-toolset || FAILED=$$((FAILED + 1)); \
	echo ""; \
	echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"; \
	echo "4️⃣  Gateway Service (Kind)"; \
	echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"; \
	$(MAKE) test-integration-gateway-service || FAILED=$$((FAILED + 1)); \
	echo ""; \
	echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"; \
	echo "5️⃣  Notification Service (Kind)"; \
	echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"; \
	$(MAKE) test-integration-notification || FAILED=$$((FAILED + 1)); \
	echo ""; \
	echo "════════════════════════════════════════════════════════════════════════"; \
	if [ $$FAILED -eq 0 ]; then \
		echo "✅ ALL SERVICE-SPECIFIC INTEGRATION TESTS PASSED (5/5)"; \
	else \
		echo "❌ $$FAILED service(s) failed integration tests"; \
	fi; \
	echo "════════════════════════════════════════════════════════════════════════"; \
	exit $$FAILED

##@ Development (continued)

.PHONY: scaffold-controller
scaffold-controller: ## Interactive scaffolding for new CRD controller using production templates
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🛠️  CRD Controller Scaffolding"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo ""
	@echo "📚 Using Production Templates"
	@echo "   Location: docs/templates/crd-controller-gap-remediation/"
	@echo "   Guide: docs/templates/crd-controller-gap-remediation/GAP_REMEDIATION_GUIDE.md"
	@echo ""
	@echo "✨ Templates Available:"
	@echo "   • cmd-main-template.go.template - Main entry point"
	@echo "   • config-template.go.template - Configuration package"
	@echo "   • config-test-template.go.template - Config tests"
	@echo "   • metrics-template.go.template - Prometheus metrics"
	@echo "   • dockerfile-template - UBI9 multi-arch Dockerfile"
	@echo "   • makefile-targets-template - Build targets"
	@echo "   • configmap-template.yaml - K8s ConfigMap"
	@echo ""
	@read -p "Controller name (lowercase, no hyphens, e.g., remediationprocessor): " CONTROLLER_NAME; \
	if [ -z "$$CONTROLLER_NAME" ]; then \
		echo "❌ Error: Controller name is required"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "📁 Creating directory structure for $$CONTROLLER_NAME..."; \
	mkdir -p "cmd/$$CONTROLLER_NAME" && echo "   ✅ cmd/$$CONTROLLER_NAME"; \
	mkdir -p "pkg/$$CONTROLLER_NAME/config" && echo "   ✅ pkg/$$CONTROLLER_NAME/config"; \
	mkdir -p "pkg/$$CONTROLLER_NAME/metrics" && echo "   ✅ pkg/$$CONTROLLER_NAME/metrics"; \
	mkdir -p "api/$$CONTROLLER_NAME/v1alpha1" && echo "   ✅ api/$$CONTROLLER_NAME/v1alpha1"; \
	mkdir -p "internal/controller/$$CONTROLLER_NAME" && echo "   ✅ internal/controller/$$CONTROLLER_NAME"; \
	echo ""; \
	echo "✅ Directory structure created successfully!"; \
	echo ""; \
	echo "📝 Next Steps:"; \
	echo "   1. Copy templates from docs/templates/crd-controller-gap-remediation/"; \
	echo "      cp docs/templates/crd-controller-gap-remediation/cmd-main-template.go.template cmd/$$CONTROLLER_NAME/main.go"; \
	echo "      cp docs/templates/crd-controller-gap-remediation/config-template.go.template pkg/$$CONTROLLER_NAME/config/config.go"; \
	echo "      cp docs/templates/crd-controller-gap-remediation/config-test-template.go.template pkg/$$CONTROLLER_NAME/config/config_test.go"; \
	echo "      cp docs/templates/crd-controller-gap-remediation/metrics-template.go.template pkg/$$CONTROLLER_NAME/metrics/metrics.go"; \
	echo ""; \
	echo "   2. Replace placeholders in copied files:"; \
	echo "      - {{CONTROLLER_NAME}} → $$CONTROLLER_NAME"; \
	echo "      - {{PACKAGE_PATH}} → github.com/jordigilh/kubernaut"; \
	echo "      - {{CRD_GROUP}}/{{CRD_VERSION}}/{{CRD_KIND}} → your CRD details"; \
	echo ""; \
	echo "   3. Follow the Gap Remediation Guide:"; \
	echo "      docs/templates/crd-controller-gap-remediation/GAP_REMEDIATION_GUIDE.md"; \
	echo ""; \
	echo "   4. Add to Makefile build targets (see makefile-targets-template)"; \
	echo ""; \
	echo "════════════════════════════════════════════════════════════════════════"

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd:allowDangerousTypes=true webhook paths="./api/..." paths="./internal/controller/..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/..."

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./api/... ./cmd/... ./internal/... ./pkg/...

.PHONY: test-e2e
test-e2e: manifests generate fmt vet ## Run e2e tests (Kind bootstrapped via Go)
	@echo "🧪 Running e2e tests..."
	@go test ./test/e2e/... -v -ginkgo.v

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	$(GOLANGCI_LINT) config verify

##@ Build

PLATFORMS ?= linux/amd64,linux/arm64

.PHONY: docker-build
docker-build: ## Build multi-architecture docker image (linux/amd64, linux/arm64)
	@echo "🔨 Building multi-architecture image: ${IMG}"
	@echo "   Platforms: $(PLATFORMS)"
	$(CONTAINER_TOOL) build --platform=$(PLATFORMS) -t ${IMG} .
	@echo "✅ Multi-arch image built: ${IMG}"

.PHONY: docker-build-single
docker-build-single: ## Build single-architecture image (host arch only, for debugging)
	@echo "🔨 Building single-arch image for debugging: ${IMG}"
	$(CONTAINER_TOOL) build -t ${IMG}-$(shell uname -m) .
	@echo "✅ Single-arch image built: ${IMG}-$(shell uname -m)"

.PHONY: docker-push
docker-push: ## Push multi-architecture docker image to registry
	@echo "📤 Pushing multi-arch image: ${IMG}"
	$(CONTAINER_TOOL) manifest push ${IMG} docker://$(IMG) || $(CONTAINER_TOOL) push ${IMG}
	@echo "✅ Image pushed: ${IMG}"


.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KIND ?= kind
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
KUSTOMIZE_VERSION ?= v5.6.0
CONTROLLER_TOOLS_VERSION ?= v0.19.0
#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')
GOLANGCI_LINT_VERSION ?= v2.1.0

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

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

##@ Microservices Build

.PHONY: build-all-services
build-all-services: build-gateway-service build-datastorage build-dynamictoolset build-notification ## Build all Go services

.PHONY: build-microservices
build-microservices: build-all-services ## Build all microservices (alias for build-all-services)

.PHONY: build-gateway-service
build-gateway-service: ## Build gateway service
	@echo "🔨 Building gateway service..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o bin/gateway ./cmd/gateway

.PHONY: build-datastorage
build-datastorage: ## Build data storage service
	@echo "📊 Building data storage service..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o bin/datastorage ./cmd/datastorage

.PHONY: build-dynamictoolset
build-dynamictoolset: ## Build dynamic toolset service
	@echo "🔧 Building dynamic toolset service..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o bin/dynamictoolset ./cmd/dynamictoolset

.PHONY: build-notification
build-notification: ## Build notification service
	@echo "📢 Building notification service..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o bin/notification ./cmd/notification

.PHONY: test
test: ## Run unit tests (Go only) - Auto-discovers all test directories
	@echo "🧪 Running Unit Tests - Auto-Discovery"
	@echo "======================================"
	@echo ""
	@echo "🔍 Discovering test packages in ./test/unit/..."
	@echo ""
	@for dir in $$(find ./test/unit -name "*_test.go" -type f | xargs -I {} dirname {} | sort -u); do \
		package_name=$$(basename "$$dir"); \
		echo "✅ Testing $$package_name ($$dir)..."; \
		if ! go test -v "$$dir" -tags=unit --timeout=60s; then \
			echo "❌ FAILED: $$package_name"; \
			exit 1; \
		fi; \
		echo ""; \
	done
	@echo "🎉 ALL UNIT TESTS COMPLETED SUCCESSFULLY!"
	@echo "========================================"
	@echo ""
	@total_dirs=$$(find ./test/unit -name "*_test.go" -type f | xargs -I {} dirname {} | sort -u | wc -l); \
	echo "📊 Total Test Packages: $$total_dirs"
	@echo "📋 All tests discovered automatically from ./test/unit/"




.PHONY: test-coverage
test-coverage: ## Run unit tests with coverage (Go only)
	@echo "Running Go unit tests with coverage..."
	go test -coverprofile=coverage.out ./... -tags="!integration,!e2e"
	go tool cover -html=coverage.out -o coverage.html
	@echo "Go coverage report generated: coverage.html"

.PHONY: test-all
test-all: validate-integration test test-integration test-e2e ## Run all tests (unit, integration, e2e)
	@echo "All test suites completed"

.PHONY: test-ci
test-ci: ## Run tests suitable for CI environment with mocked LLM
	@echo "🚀 Running CI test suite with hybrid strategy..."
	@echo "  ├── Unit tests: Real Go tests"
	@echo "  ├── Integration tests: Real Kind + Real PostgreSQL + Mock LLM"
	@echo "  └── Strategy: Kind for CI/CD, OCP for E2E"
	make test
	make test-integration-kind-ci
	@echo "✅ CI tests completed successfully"

.PHONY: fmt
fmt: ## Format code
	@echo "Formatting Go code..."
	go fmt ./api/... ./cmd/... ./internal/... ./pkg/...

.PHONY: clean
clean: ## Clean build artifacts (Go only)
	@echo "Cleaning Go artifacts..."
	rm -rf bin/kubernaut bin/test-slm
	rm -f coverage.out coverage.html
	find test/ -name "*.test" -type f -delete 2>/dev/null || true

.PHONY: clean-all
clean-all: ## Clean all build artifacts including test binaries (Go only)
	@echo "Cleaning all Go artifacts..."
	rm -rf bin/
	rm -f coverage.out coverage.html
	find test/ -name "*.test" -type f -delete 2>/dev/null || true

##@ Microservices Container Build
.PHONY: docker-build-microservices
docker-build-microservices: docker-build-gateway-service ## Build all microservice container images

.PHONY: docker-build-gateway-service
docker-build-gateway-service: ## Build gateway service container image (multi-arch UBI9)
	@echo "🔨 Building multi-arch Gateway image (amd64 + arm64) - UBI9 per ADR-027"
	podman build --platform linux/amd64,linux/arm64 \
		-f docker/gateway-ubi9.Dockerfile \
		-t $(REGISTRY)/kubernaut-gateway:$(VERSION) .
	@echo "✅ Multi-arch UBI9 image built: $(REGISTRY)/kubernaut-gateway:$(VERSION)"

.PHONY: docker-build-gateway-ubi9
docker-build-gateway-ubi9: docker-build-gateway-service ## Build gateway service UBI9 image (alias for docker-build-gateway-service)
	@echo "🔗 Gateway service uses UBI9 by default"

.PHONY: docker-build-gateway-single
docker-build-gateway-single: ## Build single-arch debug image (current platform only)
	@echo "🔨 Building single-arch Gateway image for debugging (host arch: $(shell uname -m))"
	podman build -t $(REGISTRY)/kubernaut-gateway:$(VERSION)-$(shell uname -m) \
		-f docker/gateway-ubi9.Dockerfile .
	@echo "✅ Debug image: $(REGISTRY)/kubernaut-gateway:$(VERSION)-$(shell uname -m)"

.PHONY: docker-push-microservices
docker-push-microservices: docker-push-gateway-service ## Push all microservice container images

.PHONY: docker-push-gateway-service
docker-push-gateway-service: docker-build-gateway-service ## Push Gateway service multi-arch image
	@echo "📤 Pushing multi-arch Gateway image..."
	podman manifest push $(REGISTRY)/kubernaut-gateway:$(VERSION) docker://$(REGISTRY)/kubernaut-gateway:$(VERSION)
	@echo "✅ Image pushed: $(REGISTRY)/kubernaut-gateway:$(VERSION)"

.PHONY: docker-run
docker-run: ## Run container locally
	docker run --rm -p 8080:8080 -p 9090:9090 $(IMAGE_NAME):$(VERSION)

##@ HolmesGPT API Service (Python)

HOLMESGPT_IMAGE_NAME ?= kubernaut-holmesgpt-api
HOLMESGPT_VERSION ?= latest
HOLMESGPT_REGISTRY ?= quay.io/jordigilh

.PHONY: build-holmesgpt-api
build-holmesgpt-api: ## Build HolmesGPT API service container image (Python/FastAPI)
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "🐍 Building HolmesGPT API Service (Python/FastAPI)"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "Image: $(HOLMESGPT_REGISTRY)/$(HOLMESGPT_IMAGE_NAME):$(HOLMESGPT_VERSION)"
	@echo ""
	cd holmesgpt-api && podman build \
		-t $(HOLMESGPT_IMAGE_NAME):$(HOLMESGPT_VERSION) \
		-t $(HOLMESGPT_REGISTRY)/$(HOLMESGPT_IMAGE_NAME):$(HOLMESGPT_VERSION) \
		--label "build.date=$$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
		--label "build.version=$(HOLMESGPT_VERSION)" \
		.
	@echo ""
	@echo "✅ Build complete!"
	@echo "   Local: $(HOLMESGPT_IMAGE_NAME):$(HOLMESGPT_VERSION)"
	@echo "   Tagged: $(HOLMESGPT_REGISTRY)/$(HOLMESGPT_IMAGE_NAME):$(HOLMESGPT_VERSION)"

.PHONY: push-holmesgpt-api
push-holmesgpt-api: ## Push HolmesGPT API service container image to quay.io/jordigilh
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "📤 Pushing HolmesGPT API Service to Registry"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "Registry: $(HOLMESGPT_REGISTRY)/$(HOLMESGPT_IMAGE_NAME):$(HOLMESGPT_VERSION)"
	@echo ""
	podman push $(HOLMESGPT_REGISTRY)/$(HOLMESGPT_IMAGE_NAME):$(HOLMESGPT_VERSION)
	@echo ""
	@echo "✅ Push complete!"
	@echo "   Image: $(HOLMESGPT_REGISTRY)/$(HOLMESGPT_IMAGE_NAME):$(HOLMESGPT_VERSION)"

.PHONY: test-holmesgpt-api
test-holmesgpt-api: ## Run HolmesGPT API service tests in container
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "🧪 Testing HolmesGPT API Service"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	podman run --rm $(HOLMESGPT_IMAGE_NAME):$(HOLMESGPT_VERSION) pytest -v

.PHONY: run-holmesgpt-api
run-holmesgpt-api: ## Run HolmesGPT API service locally (dev mode)
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "🚀 Running HolmesGPT API Service (Dev Mode)"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "Port: 8080"
	@echo "Health: http://localhost:8080/health"
	@echo "Docs: http://localhost:8080/docs"
	@echo ""
	podman run --rm -p 8080:8080 \
		-e DEV_MODE=true \
		-e AUTH_ENABLED=false \
		$(HOLMESGPT_IMAGE_NAME):$(HOLMESGPT_VERSION)

##@ Kubernetes
.PHONY: k8s-namespace
k8s-namespace: ## Create namespace

##@ Per-Service Test Suites (All Tiers)

.PHONY: test-gateway-all
test-gateway-all: ## Run ALL Gateway tests (unit + integration + e2e)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Gateway Service - Complete Test Suite"
	@echo "════════════════════════════════════════════════════════════════════════"
	@FAILED=0; \
	echo ""; \
	echo "1️⃣  Unit Tests..."; \
	go test ./test/unit/gateway/... -v -timeout=5m || FAILED=$$((FAILED + 1)); \
	echo ""; \
	echo "2️⃣  Integration Tests..."; \
	$(MAKE) test-gateway || FAILED=$$((FAILED + 1)); \
	echo ""; \
	echo "3️⃣  E2E Tests..."; \
	go test ./test/e2e/gateway/... -v -ginkgo.v -timeout=15m || FAILED=$$((FAILED + 1)); \
	echo ""; \
	if [ $$FAILED -eq 0 ]; then \
		echo "✅ Gateway: ALL tests passed (3/3 tiers)"; \
	else \
		echo "❌ Gateway: $$FAILED tier(s) failed"; \
		exit 1; \
	fi

.PHONY: test-e2e-datastorage
test-e2e-datastorage: ## Run Data Storage E2E tests (Kind cluster, ~5-8 min)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Data Storage Service - E2E Test Suite"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Test Scenarios:"
	@echo "   1. Happy Path - Complete remediation audit trail"
	@echo "   2. DLQ Fallback - Service outage recovery"
	@echo "   3. Query API - Multi-filter timeline retrieval"
	@echo ""
	@echo "🏗️  Infrastructure: Kind cluster + PostgreSQL + Redis + Data Storage"
	@echo "⏱️  Duration: ~5-8 minutes (serial), ~3-5 minutes (parallel)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo ""
	@cd test/e2e/datastorage && ginkgo -v --label-filter="e2e"

.PHONY: test-e2e-gateway
test-e2e-gateway: ## Run Gateway Service E2E tests (Kind cluster, ~10-15 min)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Gateway Service - E2E Test Suite"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Test Scenarios:"
	@echo "   1. Storm Window TTL - Time-based deduplication"
	@echo "   2. K8s API Rate Limiting - Backpressure handling"
	@echo "   3. State-based Deduplication - Hash-based filtering"
	@echo "   4. Storm Buffering - Burst handling"
	@echo ""
	@echo "🏗️  Infrastructure: Kind cluster + Redis + Gateway Service"
	@PROCS=$$(sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 4); \
	echo "⚡ Note: E2E tests run with $$PROCS parallel processes (auto-detected)"; \
	echo "   Each process uses unique port-forward (8081-$$((8080+$$PROCS)))"; \
	echo "   Each test uses unique namespace for isolation"; \
	echo "════════════════════════════════════════════════════════════════════════"; \
	cd test/e2e/gateway && ginkgo -v --timeout=15m --procs=$$PROCS

.PHONY: test-e2e-toolset
test-e2e-toolset: ## Run Dynamic Toolset E2E tests (Kind cluster, ~10-15 min)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Dynamic Toolset Service - E2E Test Suite"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Test Scenarios:"
	@echo "   1. Discovery Lifecycle - Toolset registration and updates"
	@echo "   2. ConfigMap Updates - Dynamic configuration changes"
	@echo "   3. Namespace Filtering - Multi-tenant isolation"
	@echo ""
	@echo "🏗️  Infrastructure: Kind cluster + Dynamic Toolset Service"
	@echo "════════════════════════════════════════════════════════════════════════"
	@cd test/e2e/toolset && ginkgo -v --timeout=15m

.PHONY: test-e2e-notification
test-e2e-notification: ## Run Notification Service E2E tests (~5-10 min)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Notification Service - E2E Test Suite"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Test Scenarios:"
	@echo "   1. Audit Lifecycle - Message sent/failed/acknowledged events"
	@echo "   2. Audit Correlation - Remediation request tracing"
	@echo ""
	@echo "🏗️  Infrastructure: envtest + Audit integration"
	@echo "════════════════════════════════════════════════════════════════════════"
	@cd test/e2e/notification && ginkgo -v --timeout=10m

.PHONY: test-e2e-notification-files
test-e2e-notification-files: ## Run Notification File Delivery E2E tests (DD-NOT-002)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Notification Service - File-Based E2E Test Suite (DD-NOT-002 V3.0)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Test Scenarios:"
	@echo "   1. Complete Message Content Validation (BR-NOT-053)"
	@echo "   2. Data Sanitization Validation (BR-NOT-054)"
	@echo "   3. Priority Field Validation (BR-NOT-056)"
	@echo "   4. Concurrent Delivery Validation"
	@echo "   5. FileService Error Handling (CRITICAL)"
	@echo ""
	@echo "🏗️  Infrastructure: envtest + FileDeliveryService"
	@echo "📁 Output Directory: /tmp/kubernaut-e2e-notifications"
	@echo "🎯 Purpose: E2E Testing Infrastructure (validates message correctness)"
	@echo ""
	@echo "⚠️  Safety Note: FileService is E2E testing only, NOT used in production"
	@echo "════════════════════════════════════════════════════════════════════════"
	@cd test/e2e/notification && ginkgo -v --timeout=10m --focus="File-Based"

.PHONY: test-e2e-notification-metrics
test-e2e-notification-metrics: ## Run Notification Service Metrics E2E tests (BR-NOT-054)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Notification Service - Metrics E2E Test Suite (BR-NOT-054)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Test Scenarios:"
	@echo "   1. Metrics Endpoint Availability"
	@echo "   2. Notification Delivery Metrics (requests_total, attempts, duration)"
	@echo "   3. Controller Metrics (reconciliation duration, active notifications)"
	@echo "   4. Sanitization Metrics (redactions tracking)"
	@echo "   5. All 10 Key Metrics Validation"
	@echo ""
	@echo "🏗️  Infrastructure: envtest + Metrics Server"
	@echo "📊 Metrics Endpoint: http://localhost:8080/metrics"
	@echo "🎯 Purpose: Validate Prometheus metrics are exposed and accurate"
	@echo ""
	@echo "⚠️  Note: Tests validate metrics format and presence, not exact values"
	@echo "════════════════════════════════════════════════════════════════════════"
	@cd test/e2e/notification && ginkgo -v --timeout=10m --focus="Metrics E2E"

.PHONY: test-e2e-datastorage-parallel
test-e2e-datastorage-parallel: ## Run Data Storage E2E tests in parallel (3 processes, ~3-5 min)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Data Storage Service - E2E Test Suite (PARALLEL)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Test Scenarios (Parallel Execution):"
	@echo "   Process 1: Happy Path"
	@echo "   Process 2: DLQ Fallback"
	@echo "   Process 3: Query API"
	@echo ""
	@echo "🏗️  Infrastructure: 3x (Kind cluster + PostgreSQL + Redis + Data Storage)"
	@echo "⏱️  Duration: ~3-5 minutes (64% faster than serial)"
	@echo "🔒 Isolation: Complete namespace isolation per process"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo ""
	@cd test/e2e/datastorage && ginkgo -v --label-filter="e2e" --procs=3

.PHONY: test-datastorage-all
test-datastorage-all: ## Run ALL Data Storage tests (unit + integration + e2e)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Data Storage - Complete Test Suite (4 Tiers)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@FAILED=0; \
	echo ""; \
	echo "1️⃣  Unit Tests..."; \
	go test ./test/unit/datastorage/... -v -timeout=5m || FAILED=$$((FAILED + 1)); \
	echo ""; \
	echo "2️⃣  Integration Tests (Podman: PostgreSQL + Redis)..."; \
	$(MAKE) test-integration-datastorage || FAILED=$$((FAILED + 1)); \
	echo ""; \
	echo "3️⃣  E2E Tests (Kind cluster)..."; \
	$(MAKE) test-e2e-datastorage || FAILED=$$((FAILED + 1)); \
	echo ""; \
	echo "4️⃣  Performance Tests..."; \
	go test ./test/performance/datastorage/... -v -bench=. -timeout=10m || FAILED=$$((FAILED + 1)); \
	echo ""; \
	if [ $$FAILED -eq 0 ]; then \
		echo "✅ Data Storage: ALL tests passed (4/4 tiers)"; \
	else \
		echo "❌ Data Storage: $$FAILED tier(s) failed"; \
		exit 1; \
	fi

.PHONY: test-toolset-all
test-toolset-all: ## Run ALL Dynamic Toolset tests (unit + integration + e2e)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Dynamic Toolset - Complete Test Suite"
	@echo "════════════════════════════════════════════════════════════════════════"
	@FAILED=0; \
	echo ""; \
	echo "1️⃣  Unit Tests..."; \
	go test ./test/unit/toolset/... -v -timeout=5m || FAILED=$$((FAILED + 1)); \
	echo ""; \
	echo "2️⃣  Integration Tests..."; \
	$(MAKE) test-integration-toolset || FAILED=$$((FAILED + 1)); \
	echo ""; \
	echo "3️⃣  E2E Tests..."; \
	go test ./test/e2e/toolset/... -v -ginkgo.v -timeout=15m || FAILED=$$((FAILED + 1)); \
	echo ""; \
	if [ $$FAILED -eq 0 ]; then \
		echo "✅ Dynamic Toolset: ALL tests passed (3/3 tiers)"; \
	else \
		echo "❌ Dynamic Toolset: $$FAILED tier(s) failed"; \
		exit 1; \
	fi

.PHONY: test-notification-all
test-notification-all: ## Run ALL Notification tests (unit + integration)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Notification Service - Complete Test Suite"
	@echo "════════════════════════════════════════════════════════════════════════"
	@FAILED=0; \
	echo ""; \
	echo "1️⃣  Unit Tests..."; \
	go test ./test/unit/notification/... -v -timeout=5m || FAILED=$$((FAILED + 1)); \
	echo ""; \
	echo "2️⃣  Integration Tests..."; \
	$(MAKE) test-integration-notification || FAILED=$$((FAILED + 1)); \
	echo ""; \
	if [ $$FAILED -eq 0 ]; then \
		echo "✅ Notification: ALL tests passed (2/2 tiers)"; \
	else \
		echo "❌ Notification: $$FAILED tier(s) failed"; \
		exit 1; \
	fi

.PHONY: test-holmesgpt-all
test-holmesgpt-all: ## Run ALL HolmesGPT API tests (Python)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 HolmesGPT API - Complete Test Suite"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo ""
	@echo "Running Python test suite..."
	@cd holmesgpt-api && pytest -v --cov=. --cov-report=term-missing
	@echo ""
	@echo "✅ HolmesGPT API: ALL tests passed"

.PHONY: test-all-services
test-all-services: ## Run ALL tests for ALL services (sequential - use CI for parallel)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🚀 Complete Test Suite - All Services"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo ""
	@echo "⚠️  Note: Running sequentially. Use GitHub Actions for parallel execution."
	@echo ""
	@FAILED=0; \
	$(MAKE) test-gateway-all || FAILED=$$((FAILED + 1)); \
	echo ""; \
	$(MAKE) test-datastorage-all || FAILED=$$((FAILED + 1)); \
	echo ""; \
	$(MAKE) test-toolset-all || FAILED=$$((FAILED + 1)); \
	echo ""; \
	$(MAKE) test-notification-all || FAILED=$$((FAILED + 1)); \
	echo ""; \
	$(MAKE) test-holmesgpt-all || FAILED=$$((FAILED + 1)); \
	echo ""; \
	echo "════════════════════════════════════════════════════════════════════════"; \
	if [ $$FAILED -eq 0 ]; then \
		echo "✅ ALL SERVICES: Complete test suite passed (5/5 services)"; \
	else \
		echo "❌ FAILED: $$FAILED service(s) failed tests"; \
		exit 1; \
	fi; \
	echo "════════════════════════════════════════════════════════════════════════"

##@ Containerized Testing

.PHONY: test-container-build
test-container-build: ## Build test runner container
	@echo "🐳 Building test runner container..."
	podman build -f docker/test-runner.Dockerfile -t kubernaut-test-runner:latest .

.PHONY: test-container-unit
test-container-unit: ## Run unit tests in container (no external dependencies)
	@echo "🐳 Running unit tests in container (standalone, no external services)..."
	podman run --rm \
		-v $(PWD):/workspace:Z \
		-w /workspace \
		kubernaut-test-runner:latest \
		make test

.PHONY: test-container-integration
test-container-integration: ## Run integration tests in container
	@echo "🐳 Running integration tests in container..."
	podman-compose -f podman-compose.test.yml run --rm \
		-e POSTGRES_HOST=postgres \
		-e POSTGRES_PORT=5432 \
		-e REDIS_HOST=redis \
		-e REDIS_PORT=6379 \
		-e DATASTORAGE_URL=http://datastorage:8080 \
		test-runner sh -c "make test-integration-datastorage && make test-integration-notification"

.PHONY: test-container-e2e
test-container-e2e: ## Run E2E tests in container
	@echo "🐳 Running E2E tests in container..."
	podman-compose -f podman-compose.test.yml run --rm \
		-e KIND_EXPERIMENTAL_PROVIDER=podman \
		-e POSTGRES_HOST=postgres \
		-e POSTGRES_PORT=5432 \
		-e REDIS_HOST=redis \
		-e REDIS_PORT=6379 \
		test-runner sh -c "cd test/e2e/gateway && go test -v -ginkgo.v -timeout=15m"

.PHONY: test-container-all
test-container-all: ## Run ALL tests in container
	@echo "🐳 Running ALL tests in container..."
	podman-compose -f podman-compose.test.yml run --rm \
		-e POSTGRES_HOST=postgres \
		-e POSTGRES_PORT=5432 \
		-e REDIS_HOST=redis \
		-e REDIS_PORT=6379 \
		test-runner make test-all-services

.PHONY: test-container-shell
test-container-shell: ## Open shell in test container for debugging
	@echo "🐳 Opening shell in test container..."
	podman-compose -f podman-compose.test.yml run --rm \
		-e POSTGRES_HOST=postgres \
		-e POSTGRES_PORT=5432 \
		-e REDIS_HOST=redis \
		-e REDIS_PORT=6379 \
		test-runner /bin/bash

.PHONY: test-container-down
test-container-down: ## Stop and remove all test containers
	@echo "🐳 Stopping test containers..."
	podman-compose -f podman-compose.test.yml down -v
