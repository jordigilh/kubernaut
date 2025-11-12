# Image URL to use all building/pushing image targets
IMG ?= controller:latest

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
test-gateway: ## Run Gateway integration tests (Kind bootstrapped via Go)
	@echo "🧪 Running Gateway integration tests..."
	@cd test/integration/gateway && ginkgo -v

##@ Notification Service Integration Tests

.PHONY: test-integration-notification
test-integration-notification: ## Run Notification Service integration tests (Kind bootstrapped via Go)
	@echo "🧪 Running Notification Service integration tests..."
	@go test ./test/integration/notification/... -v -ginkgo.v -timeout=15m

##@ Service-Specific Integration Tests

.PHONY: test-integration-datastorage
test-integration-datastorage: ## Run Data Storage integration tests (PostgreSQL 16 via Podman, ~30s)
	@echo "🔧 Starting PostgreSQL 16 with pgvector 0.5.1+ extension..."
	@podman run -d --name datastorage-postgres -p 5432:5432 \
		-e POSTGRES_PASSWORD=postgres \
		-e POSTGRES_SHARED_BUFFERS=1GB \
		pgvector/pgvector:pg16 > /dev/null 2>&1 || \
		(echo "⚠️  PostgreSQL container already exists or failed to start" && \
		 podman start datastorage-postgres > /dev/null 2>&1) || true
	@echo "⏳ Waiting for PostgreSQL to be ready..."
	@sleep 5
	@podman exec datastorage-postgres pg_isready -U postgres > /dev/null 2>&1 || \
		(echo "❌ PostgreSQL not ready" && exit 1)
	@echo "✅ PostgreSQL 16 ready"
	@echo "🔍 Verifying PostgreSQL and pgvector versions..."
	@podman exec datastorage-postgres psql -U postgres -c "SELECT version();" | grep "PostgreSQL 16" || \
		(echo "❌ PostgreSQL version is not 16.x" && exit 1)
	@echo "🔧 Creating pgvector extension..."
	@podman exec datastorage-postgres psql -U postgres -c "CREATE EXTENSION IF NOT EXISTS vector;" > /dev/null 2>&1 || \
		(echo "❌ Failed to create pgvector extension" && exit 1)
	@podman exec datastorage-postgres psql -U postgres -c "SELECT extversion FROM pg_extension WHERE extname = 'vector';" | grep -E "0\.[5-9]\.[1-9]|0\.[6-9]\.0|0\.[7-9]\.0|0\.[8-9]\.0" || \
		(echo "❌ pgvector version is not 0.5.1+" && exit 1)
	@echo "✅ Version validation passed (PostgreSQL 16 + pgvector 0.5.1+)"
	@echo "🔍 Testing HNSW index creation (dry-run)..."
	@podman exec datastorage-postgres psql -U postgres -d postgres -c "\
		CREATE TEMP TABLE hnsw_validation_test (id SERIAL PRIMARY KEY, embedding vector(384)); \
		CREATE INDEX hnsw_validation_test_idx ON hnsw_validation_test USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64);" \
		> /dev/null 2>&1 || \
		(echo "❌ HNSW index creation test failed - PostgreSQL/pgvector may not support HNSW" && exit 1)
	@echo "✅ HNSW index support verified"
	@echo "🧪 Running Data Storage integration tests..."
	@TEST_RESULT=0; \
	go test ./test/integration/datastorage/... -v -timeout 5m || TEST_RESULT=$$?; \
	echo "🧹 Cleaning up PostgreSQL container..."; \
	podman stop datastorage-postgres > /dev/null 2>&1 || true; \
	podman rm datastorage-postgres > /dev/null 2>&1 || true; \
	echo "✅ Cleanup complete"; \
	exit $$TEST_RESULT

.PHONY: test-integration-contextapi
test-integration-contextapi: ## Run Context API integration tests (Redis via Podman + PostgreSQL, ~45s)
	@echo "🔧 Starting Redis for Context API..."
	@podman run -d --name contextapi-redis-test -p 6379:6379 redis:7-alpine > /dev/null 2>&1 || \
		(echo "⚠️  Redis container already exists or failed to start" && \
		 podman start contextapi-redis-test > /dev/null 2>&1) || true
	@echo "⏳ Waiting for Redis to be ready..."
	@sleep 2
	@podman exec contextapi-redis-test redis-cli ping > /dev/null 2>&1 || \
		(echo "❌ Redis not ready" && exit 1)
	@echo "✅ Redis ready"
	@echo "📝 NOTE: PostgreSQL required - run 'make bootstrap-dev' if not running"
	@echo "🧪 Running Context API integration tests..."
	@TEST_RESULT=0; \
	go test ./test/integration/contextapi/... -v -timeout 5m || TEST_RESULT=$$?; \
	echo "🧹 Cleaning up Redis container..."; \
	podman stop contextapi-redis-test > /dev/null 2>&1 || true; \
	podman rm contextapi-redis-test > /dev/null 2>&1 || true; \
	echo "✅ Cleanup complete"; \
	exit $$TEST_RESULT

.PHONY: test-integration-ai
test-integration-ai: ## Run AI Service integration tests (Redis via Podman, ~15s)
	@echo "🔧 Starting Redis cache..."
	@podman run -d --name ai-redis -p 6379:6379 redis:7-alpine > /dev/null 2>&1 || \
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
build-all-services: build-gateway-service build-context-api build-datastorage build-dynamictoolset build-notification ## Build all Go services

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
docker-build-microservices: docker-build-gateway-service docker-build-context-api ## Build all microservice container images

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
docker-push-microservices: docker-push-gateway-service docker-push-context-api ## Push all microservice container images

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

##@ Context API Service

# Context API Image Configuration
CONTEXT_API_IMG ?= quay.io/jordigilh/context-api:v0.1.0

.PHONY: build-context-api
build-context-api: ## Build Context API binary locally
	@echo "🔨 Building Context API binary..."
	go build -o bin/context-api cmd/contextapi/main.go
	@echo "✅ Binary: bin/context-api"

.PHONY: run-context-api
run-context-api: build-context-api ## Run Context API locally with config file
	@echo "🚀 Starting Context API..."
	./bin/context-api --config config/context-api.yaml

.PHONY: test-context-api
test-context-api: ## Run Context API unit tests
	@echo "🧪 Running Context API tests..."
	go test ./pkg/contextapi/... -v -cover

.PHONY: test-context-api-integration
test-context-api-integration: ## Run Context API integration tests
	@echo "🧪 Running Context API integration tests..."
	go test ./test/integration/contextapi/... -v

.PHONY: docker-build-context-api
docker-build-context-api: ## Build multi-architecture Context API image (podman + amd64/arm64)
	@echo "🔨 Building multi-architecture image: $(CONTEXT_API_IMG)"
	podman build --platform linux/amd64,linux/arm64 \
		-t $(CONTEXT_API_IMG) \
		-f docker/context-api.Dockerfile .
	@echo "✅ Multi-arch image built: $(CONTEXT_API_IMG)"

.PHONY: docker-push-context-api
docker-push-context-api: docker-build-context-api ## Push Context API multi-arch image to registry
	@echo "📤 Pushing multi-arch image: $(CONTEXT_API_IMG)"
	podman manifest push $(CONTEXT_API_IMG) docker://$(CONTEXT_API_IMG)
	@echo "✅ Image pushed: $(CONTEXT_API_IMG)"

.PHONY: docker-build-context-api-single
docker-build-context-api-single: ## Build single-arch debug image (current platform only)
	@echo "🔨 Building single-arch debug image: $(CONTEXT_API_IMG)-$(shell uname -m)"
	podman build -t $(CONTEXT_API_IMG)-$(shell uname -m) \
		-f docker/context-api.Dockerfile .
	@echo "✅ Single-arch debug image built: $(CONTEXT_API_IMG)-$(shell uname -m)"

.PHONY: docker-run-context-api
docker-run-context-api: docker-build-context-api ## Run Context API in container with environment variables
	@echo "🚀 Starting Context API container..."
	podman run -d --rm \
		--name context-api \
		-p 8091:8091 \
		-p 9090:9090 \
		-e DB_HOST=localhost \
		-e DB_PORT=5432 \
		-e DB_NAME=postgres \
		-e DB_USER=postgres \
		-e DB_PASSWORD=postgres \
		-e REDIS_ADDR=localhost:6379 \
		-e REDIS_DB=0 \
		-e LOG_LEVEL=info \
		$(CONTEXT_API_IMG)
	@echo "✅ Context API running: http://localhost:8091"
	@echo "📊 Metrics endpoint: http://localhost:9090/metrics"
	@echo "🛑 Stop with: make docker-stop-context-api"

.PHONY: docker-run-context-api-with-config
docker-run-context-api-with-config: docker-build-context-api ## Run Context API with mounted config file (local dev)
	@echo "🚀 Starting Context API container with config file..."
	podman run -d --rm \
		--name context-api \
		-p 8091:8091 \
		-p 9090:9090 \
		-v $(PWD)/config/context-api.yaml:/etc/context-api/config.yaml:ro \
		$(CONTEXT_API_IMG) \
		--config /etc/context-api/config.yaml
	@echo "✅ Context API running: http://localhost:8091"
	@echo "📊 Metrics endpoint: http://localhost:9090/metrics"
	@echo "🛑 Stop with: make docker-stop-context-api"

.PHONY: docker-stop-context-api
docker-stop-context-api: ## Stop Context API container
	@echo "🛑 Stopping Context API container..."
	podman stop context-api || true
	@echo "✅ Context API stopped"

.PHONY: docker-logs-context-api
docker-logs-context-api: ## Show Context API container logs
	podman logs -f context-api

.PHONY: deploy-context-api
deploy-context-api: ## Deploy Context API to Kubernetes cluster
	@echo "🚀 Deploying Context API to Kubernetes..."
	kubectl apply -f deploy/context-api/
	@echo "✅ Context API deployed"
	@echo "⏳ Waiting for rollout..."
	kubectl rollout status deployment/context-api -n kubernaut-system

.PHONY: undeploy-context-api
undeploy-context-api: ## Remove Context API from Kubernetes cluster
	@echo "🗑️  Removing Context API from Kubernetes..."
	kubectl delete -f deploy/context-api/ || true
	@echo "✅ Context API removed"

.PHONY: validate-context-api-build
validate-context-api-build: ## Validate Context API build pipeline
	@echo "✅ Validating Context API build pipeline..."
	@echo "1️⃣  Building binary..."
	@$(MAKE) build-context-api
	@echo "2️⃣  Running unit tests..."
	@$(MAKE) test-context-api
	@echo "3️⃣  Building Docker image..."
	@$(MAKE) docker-build-context-api-single
	@echo "4️⃣  Testing container startup..."
	@podman run --rm -d --name context-api-validate -p 8091:8091 -p 9090:9090 \
		-e DB_HOST=localhost -e DB_PORT=5432 -e DB_NAME=test -e DB_USER=test -e DB_PASSWORD=test \
		-e REDIS_ADDR=localhost:6379 -e REDIS_DB=0 \
		$(CONTEXT_API_IMG)-$(shell uname -m) || true
	@sleep 3
	@curl -f http://localhost:8091/health && echo "✅ Health check passed" || echo "❌ Health check failed"
	@podman stop context-api-validate || true
	@echo "✅ Context API build pipeline validated"

##@ Context API E2E Tests

.PHONY: test-e2e-contextapi
test-e2e-contextapi: ## Run Context API E2E tests (Kind bootstrapped via Go)
	@echo "🧪 Running Context API E2E tests..."
	@cd test/e2e/contextapi && ginkgo -v

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

.PHONY: test-contextapi-all
test-contextapi-all: ## Run ALL Context API tests (unit + integration + e2e)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Context API - Complete Test Suite"
	@echo "════════════════════════════════════════════════════════════════════════"
	@FAILED=0; \
	echo ""; \
	echo "1️⃣  Unit Tests..."; \
	go test ./test/unit/contextapi/... -v -timeout=5m || FAILED=$$((FAILED + 1)); \
	echo ""; \
	echo "2️⃣  Integration Tests..."; \
	$(MAKE) test-integration-contextapi || FAILED=$$((FAILED + 1)); \
	echo ""; \
	echo "3️⃣  E2E Tests..."; \
	$(MAKE) test-e2e-contextapi || FAILED=$$((FAILED + 1)); \
	echo ""; \
	if [ $$FAILED -eq 0 ]; then \
		echo "✅ Context API: ALL tests passed (3/3 tiers)"; \
	else \
		echo "❌ Context API: $$FAILED tier(s) failed"; \
		exit 1; \
	fi

.PHONY: test-datastorage-all
test-datastorage-all: ## Run ALL Data Storage tests (unit + integration + performance)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Data Storage - Complete Test Suite"
	@echo "════════════════════════════════════════════════════════════════════════"
	@FAILED=0; \
	echo ""; \
	echo "1️⃣  Unit Tests..."; \
	go test ./test/unit/datastorage/... -v -timeout=5m || FAILED=$$((FAILED + 1)); \
	echo ""; \
	echo "2️⃣  Integration Tests..."; \
	$(MAKE) test-integration-datastorage || FAILED=$$((FAILED + 1)); \
	echo ""; \
	echo "3️⃣  Performance Tests..."; \
	go test ./test/performance/datastorage/... -v -bench=. -timeout=10m || FAILED=$$((FAILED + 1)); \
	echo ""; \
	if [ $$FAILED -eq 0 ]; then \
		echo "✅ Data Storage: ALL tests passed (3/3 tiers)"; \
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
	$(MAKE) test-contextapi-all || FAILED=$$((FAILED + 1)); \
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
		echo "✅ ALL SERVICES: Complete test suite passed (6/6 services)"; \
	else \
		echo "❌ FAILED: $$FAILED service(s) failed tests"; \
		exit 1; \
	fi; \
	echo "════════════════════════════════════════════════════════════════════════"
