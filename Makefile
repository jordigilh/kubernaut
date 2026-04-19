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
# Auto-detect: prefer podman if available, fall back to docker.
CONTAINER_TOOL ?= $(shell command -v podman >/dev/null 2>&1 && echo podman || echo docker)

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

# Coverage Configuration: Exclude Generated Code
# - DataStorage: Excludes pkg/datastorage/ogen-client (OpenAPI-generated) and mocks
# - AgentClient: pkg/agentclient contains oas_*_gen.go (ogen-generated client)
#
# Why DataStorage unit coverage is ~27%: Unit tests (462 specs) cover builders, validation, config,
# aggregation handlers (with mocks). The remaining ~349 functions at 0% are HTTP handlers, DLQ worker,
# repository/DB adapter, server bootstrap—exercised in integration and E2E tests. Unit-only coverage
# is intentionally lower; total coverage across all tiers is the complete picture.
# Run: make test-all-datastorage then see coverage_*_datastorage.out

# DataStorage coverage packages (hand-written only, excludes generated)
# DATASTORAGE_COVERPKG: Comma-separated list of packages for coverage instrumentation.
# IMPORTANT: No spaces after commas — Go's --coverpkg treats spaces as part of the package name.
DATASTORAGE_COVERPKG = github.com/jordigilh/kubernaut/pkg/datastorage/adapter/...,github.com/jordigilh/kubernaut/pkg/datastorage/audit/...,github.com/jordigilh/kubernaut/pkg/datastorage/config/...,github.com/jordigilh/kubernaut/pkg/datastorage/dlq/...,github.com/jordigilh/kubernaut/pkg/datastorage/metrics/...,github.com/jordigilh/kubernaut/pkg/datastorage/models/...,github.com/jordigilh/kubernaut/pkg/datastorage/partition/...,github.com/jordigilh/kubernaut/pkg/datastorage/query/...,github.com/jordigilh/kubernaut/pkg/datastorage/reconstruction/...,github.com/jordigilh/kubernaut/pkg/datastorage/repository/...,github.com/jordigilh/kubernaut/pkg/datastorage/repository/sql/...,github.com/jordigilh/kubernaut/pkg/datastorage/repository/sqlutil/...,github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow/...,github.com/jordigilh/kubernaut/pkg/datastorage/schema/...,github.com/jordigilh/kubernaut/pkg/datastorage/server/...,github.com/jordigilh/kubernaut/pkg/datastorage/server/helpers/...,github.com/jordigilh/kubernaut/pkg/datastorage/server/middleware/...,github.com/jordigilh/kubernaut/pkg/datastorage/server/response/...,github.com/jordigilh/kubernaut/pkg/datastorage/validation/...

# Unit-testable package patterns (pure logic: config, validators, builders, formatters, metrics, classifiers)
# Integration-testable patterns (I/O-dependent: handlers, servers, DB adapters, K8s clients, workers)
# NOTE: These patterns are used by scripts/coverage/ AWK scripts - see Phase 2 refactoring
AIANALYSIS_UNIT_PATTERN = pkg/aianalysis/handlers/|pkg/aianalysis/metrics/|pkg/aianalysis/phase/|pkg/aianalysis/rego/|pkg/aianalysis/conditions
AUTHWEBHOOK_UNIT_PATTERN = pkg/authwebhook/config/|pkg/authwebhook/validation/|pkg/authwebhook/types
GATEWAY_UNIT_PATTERN = pkg/gateway/adapters/|pkg/gateway/config/|pkg/gateway/errors/|pkg/gateway/types/|pkg/gateway/processing/clock|pkg/gateway/processing/deduplication_types|pkg/gateway/processing/errors|pkg/gateway/processing/phase_checker|pkg/gateway/middleware/
NOTIFICATION_UNIT_PATTERN = pkg/notification/config/|pkg/notification/formatting/|pkg/notification/metrics/|pkg/notification/retry/|pkg/notification/routing/|pkg/notification/types|pkg/notification/conditions
REMEDIATIONORCHESTRATOR_UNIT_PATTERN = pkg/remediationorchestrator/audit/|pkg/remediationorchestrator/config/|pkg/remediationorchestrator/helpers/|pkg/remediationorchestrator/metrics/|pkg/remediationorchestrator/phase/|pkg/remediationorchestrator/routing/|pkg/remediationorchestrator/timeout/|pkg/remediationorchestrator/types|pkg/remediationorchestrator/interfaces
SIGNALPROCESSING_UNIT_PATTERN = pkg/signalprocessing/classifier/|pkg/signalprocessing/config/|pkg/signalprocessing/detection/|pkg/signalprocessing/metrics/|pkg/signalprocessing/ownerchain/|pkg/signalprocessing/phase/|pkg/signalprocessing/rego/|pkg/signalprocessing/conditions
WORKFLOWEXECUTION_UNIT_PATTERN = pkg/workflowexecution/config/|pkg/workflowexecution/metrics/|pkg/workflowexecution/phase/|pkg/workflowexecution/conditions
EFFECTIVENESSMONITOR_UNIT_PATTERN = pkg/effectivenessmonitor/config/|pkg/effectivenessmonitor/health/|pkg/effectivenessmonitor/alert/|pkg/effectivenessmonitor/metrics/|pkg/effectivenessmonitor/hash/|pkg/effectivenessmonitor/audit/|pkg/effectivenessmonitor/phase/|pkg/effectivenessmonitor/validity/|pkg/effectivenessmonitor/types

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
manifests: controller-gen sync-version ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd:allowDangerousTypes=true webhook paths="./api/..." paths="./internal/controller/..." output:crd:artifacts:config=config/crd/bases
	@echo "📋 Syncing CRDs to Helm chart..."
	@cp -f config/crd/bases/*.yaml charts/kubernaut/crds/
	@mkdir -p charts/kubernaut/files/crds
	@cp -f config/crd/bases/*.yaml charts/kubernaut/files/crds/
	@echo "✅ charts/kubernaut/crds/ and files/crds/ updated"
	@$(MAKE) sync-embed

.PHONY: sync-embed
sync-embed: ## Sync migration SQL and CRD YAMLs into pkg/shared/assets/ for Go embed (DD-4, Issue #578)
	@echo "📋 Syncing embedded assets for kubernaut-operator..."
	@mkdir -p pkg/shared/assets/migrations pkg/shared/assets/crds
	@cp -f migrations/*.sql pkg/shared/assets/migrations/
	@cp -f config/crd/bases/kubernaut.ai_*.yaml pkg/shared/assets/crds/
	@echo "✅ pkg/shared/assets/ updated (migrations + CRDs)"

.PHONY: validate-embed
validate-embed: sync-embed ## Validate embedded assets are in sync with source files (CI drift detection)
	@echo "🔍 Checking embedded assets for drift..."
	@if ! git diff --quiet pkg/shared/assets/; then \
		echo ""; \
		echo "❌ Embedded assets are out of sync!"; \
		echo ""; \
		echo "📋 Drifted files:"; \
		git diff --name-only pkg/shared/assets/; \
		echo ""; \
		echo "🔧 To fix: run 'make sync-embed' and commit the changes"; \
		exit 1; \
	fi
	@echo "✅ Embedded assets are in sync"

.PHONY: sync-version
sync-version: ## Propagate VERSION file to Chart.yaml, values, Dockerfiles, and docs
	@test -f VERSION || (echo "ERROR: VERSION file not found at repo root" && exit 1)
	@VER=$$(cat VERSION) && \
	echo "📌 Syncing version v$$VER from VERSION file..." && \
	sed -i.bak "s/^version: .*/version: $$VER/" charts/kubernaut/Chart.yaml && rm -f charts/kubernaut/Chart.yaml.bak && \
	sed -i.bak "s/^appVersion: .*/appVersion: \"$$VER\"/" charts/kubernaut/Chart.yaml && rm -f charts/kubernaut/Chart.yaml.bak && \
	sed -i.bak "s|db-migrate:v[0-9][0-9a-zA-Z._-]*|db-migrate:v$$VER|g" \
		charts/kubernaut/values.yaml \
		charts/kubernaut/values.schema.json \
		charts/kubernaut/values-airgap.yaml \
		charts/kubernaut/README.md \
		hack/airgap/imageset-config.yaml.tmpl && \
	rm -f charts/kubernaut/values.yaml.bak charts/kubernaut/values.schema.json.bak \
		charts/kubernaut/values-airgap.yaml.bak charts/kubernaut/README.md.bak \
		hack/airgap/imageset-config.yaml.tmpl.bak && \
	for df in docker/*.Dockerfile; do \
		sed -i.bak "s/^ARG APP_VERSION=v[0-9][0-9a-zA-Z._-]*/ARG APP_VERSION=v$$VER/" "$$df" && rm -f "$$df.bak"; \
	done && \
	echo "✅ Version v$$VER synced to all targets"

.PHONY: generate
generate: controller-gen ogen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/..."
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./pkg/shared/types/..."
	@echo "📋 Generating OpenAPI spec copies for embedding (DD-API-002)..."
	@go generate ./pkg/datastorage/server/middleware/...
	@go generate ./pkg/audit/...
	@echo "📋 Generating AgentClient (ogen)..."
	@PATH="$(LOCALBIN):$$PATH" go generate ./pkg/agentclient/...
	@echo "✅ Generation complete"

.PHONY: generate-datastorage-client
generate-datastorage-client: ogen ## Generate DataStorage OpenAPI client from spec (DD-API-001)
	@echo "📋 Generating DataStorage Go client from api/openapi/data-storage-v1.yaml..."
	@go generate ./pkg/datastorage/ogen-client/...
	@echo "✅ Go client generated: pkg/datastorage/ogen-client/oas_*_gen.go"

.PHONY: generate-agentclient
generate-agentclient: ogen ## Generate AgentClient from OpenAPI spec
	@echo "📋 Generating AgentClient from OpenAPI spec..."
	@PATH="$(LOCALBIN):$$PATH" go generate ./pkg/agentclient/...
	@echo "✅ AgentClient generated successfully"

.PHONY: generate-crd-docs
generate-crd-docs: crd-ref-docs ## Generate CRD API reference docs from Go types
	@echo "📋 Generating CRD API reference from api/ types..."
	@mkdir -p docs/generated
	@$(CRD_REF_DOCS) \
		--source-path=api/ \
		--config=hack/crd-ref-docs/config.yaml \
		--templates-dir=hack/crd-ref-docs/templates/markdown \
		--renderer=markdown \
		--output-path=docs/generated/crds.md \
		--output-mode=single \
		--max-depth=10
	@hack/crd-ref-docs/clean-output.sh docs/generated/crds.md
	@echo "✅ CRD docs generated: docs/generated/crds.md"

.PHONY: fmt
fmt: ## Format code
	@echo "Formatting Go code..."
	go fmt ./api/... ./cmd/... ./internal/... ./pkg/...

.PHONY: vet
vet: ## Run go vet against code
	go vet ./api/... ./cmd/... ./internal/... ./pkg/...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint
	$(GOLANGCI_LINT) run cmd/... pkg/... internal/... test/...

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
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_UNIT) --procs=$(TEST_PROCS) --coverprofile=coverage_unit_$*.out --covermode=atomic --coverpkg=github.com/jordigilh/kubernaut/pkg/$*/...,github.com/jordigilh/kubernaut/internal/controller/$*/... ./test/unit/$*/...
	@if [ -f coverage_unit_$*.out ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_unit_$*.out"; \
		go tool cover -func=coverage_unit_$*.out | grep total || echo "No coverage data"; \
	fi

# Kubernaut Agent unit tests: internal code lives at internal/kubernautagent/ (not internal/controller/)
.PHONY: test-unit-kubernautagent
test-unit-kubernautagent: ginkgo ensure-coverage-dirs ## Run kubernaut agent unit tests (coverpkg: pkg + internal/kubernautagent)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 kubernautagent - Unit Tests ($(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_UNIT) --procs=$(TEST_PROCS) --coverprofile=coverage_unit_kubernautagent.out --covermode=atomic --coverpkg=github.com/jordigilh/kubernaut/pkg/kubernautagent/...,github.com/jordigilh/kubernaut/internal/kubernautagent/... ./test/unit/kubernautagent/...
	@if [ -f coverage_unit_kubernautagent.out ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_unit_kubernautagent.out"; \
		go tool cover -func=coverage_unit_kubernautagent.out | grep total || echo "No coverage data"; \
	fi

# Gateway unit tests: no internal/controller/gateway/ exists, use pkg-only coverpkg
.PHONY: test-unit-gateway
test-unit-gateway: ginkgo ensure-coverage-dirs ## Run gateway unit tests (coverpkg: pkg/gateway only)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 gateway - Unit Tests ($(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_UNIT) --procs=$(TEST_PROCS) --coverprofile=coverage_unit_gateway.out --covermode=atomic --coverpkg=github.com/jordigilh/kubernaut/pkg/gateway/... ./test/unit/gateway/...
	@if [ -f coverage_unit_gateway.out ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_unit_gateway.out"; \
		go tool cover -func=coverage_unit_gateway.out | grep total || echo "No coverage data"; \
	fi

# Shared packages unit tests: tests for pkg/audit, pkg/cache, pkg/http, pkg/k8sutil, pkg/shared
# These packages are not standalone services (no cmd/ entry), so they have no service-level
# test target. This consolidated target runs all shared infrastructure package tests.
.PHONY: test-unit-shared-packages
test-unit-shared-packages: ginkgo ensure-coverage-dirs ## Run unit tests for shared infrastructure packages (audit, cache, http, k8sutil, shared)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 shared-packages - Unit Tests ($(TEST_PROCS) procs)"
	@echo "   Packages: pkg/audit, pkg/cache, pkg/http, pkg/k8sutil, pkg/shared"
	@echo "════════════════════════════════════════════════════════════════════════"
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_UNIT) --procs=$(TEST_PROCS) --coverprofile=coverage_unit_shared-packages.out --covermode=atomic --coverpkg=github.com/jordigilh/kubernaut/pkg/audit/...,github.com/jordigilh/kubernaut/pkg/cache/...,github.com/jordigilh/kubernaut/pkg/http/...,github.com/jordigilh/kubernaut/pkg/k8sutil/...,github.com/jordigilh/kubernaut/pkg/shared/... ./test/unit/audit/... ./test/unit/cache/... ./test/unit/http/... ./test/unit/k8sutil/... ./test/unit/shared/...
	@if [ -f coverage_unit_shared-packages.out ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_unit_shared-packages.out"; \
		go tool cover -func=coverage_unit_shared-packages.out | grep total || echo "No coverage data"; \
	fi

# DataStorage unit tests: exclude generated code (ogen-client, mocks) from coverage
.PHONY: test-unit-datastorage
test-unit-datastorage: ginkgo ensure-coverage-dirs ## Run datastorage unit tests (coverage excludes ogen-client)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 datastorage - Unit Tests ($(TEST_PROCS) procs) [coverage: hand-written code only]"
	@echo "════════════════════════════════════════════════════════════════════════"
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_UNIT) --procs=$(TEST_PROCS) --coverprofile=coverage_unit_datastorage.out --covermode=atomic --coverpkg=$(DATASTORAGE_COVERPKG) ./test/unit/datastorage/...
	@if [ -f coverage_unit_datastorage.out ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_unit_datastorage.out"; \
		go tool cover -func=coverage_unit_datastorage.out | grep total || echo "No coverage data"; \
	fi

# Shared packages integration tests: tests for pkg/shared (TLS, etc.)
# These packages are not standalone services (no cmd/ entry or internal/controller/),
# so the generic test-integration-% pattern generates a bogus coverpkg path.
# This dedicated target uses the correct coverpkg for shared packages only.
.PHONY: test-integration-shared
test-integration-shared: ginkgo ensure-coverage-dirs ## Run integration tests for shared infrastructure packages (TLS, etc.)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 shared - Integration Tests ($(TEST_PROCS) procs)"
	@echo "   Packages: pkg/shared (TLS)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_INTEGRATION) --procs=$(TEST_PROCS) --coverprofile=coverage_integration_shared.out --covermode=atomic --keep-going --coverpkg=github.com/jordigilh/kubernaut/pkg/shared/... ./test/integration/shared/...
	@if [ -f coverage_integration_shared.out ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_integration_shared.out"; \
		go tool cover -func=coverage_integration_shared.out | grep total || echo "No coverage data"; \
	fi

# Integration Tests
.PHONY: test-integration-%
test-integration-%: generate ginkgo setup-envtest ensure-coverage-dirs ## Run integration tests for specified service (e.g., make test-integration-gateway)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 $* - Integration Tests ($(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Pattern: DD-INTEGRATION-001 v2.0 (envtest + Podman dependencies)"
	@KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" $(GINKGO) -v --timeout=$(TEST_TIMEOUT_INTEGRATION) --procs=$(TEST_PROCS) --coverprofile=coverage_integration_$*.out --covermode=atomic --keep-going --coverpkg=github.com/jordigilh/kubernaut/pkg/$*/...,github.com/jordigilh/kubernaut/internal/controller/$*/... ./test/integration/$*/...
	@if [ -f coverage_integration_$*.out ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_integration_$*.out"; \
		go tool cover -func=coverage_integration_$*.out | grep total || echo "No coverage data"; \
	fi

# Kubernaut Agent integration tests: internal code lives at internal/kubernautagent/ (not internal/controller/)
.PHONY: test-integration-kubernautagent
test-integration-kubernautagent: generate ginkgo setup-envtest ensure-coverage-dirs ## Run kubernaut agent integration tests (coverpkg: pkg + internal/kubernautagent)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 kubernautagent - Integration Tests ($(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Pattern: DD-INTEGRATION-001 v2.0 (envtest + Podman dependencies)"
	@KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" $(GINKGO) -v --timeout=$(TEST_TIMEOUT_INTEGRATION) --procs=$(TEST_PROCS) --coverprofile=coverage_integration_kubernautagent.out --covermode=atomic --keep-going --coverpkg=github.com/jordigilh/kubernaut/pkg/kubernautagent/...,github.com/jordigilh/kubernaut/internal/kubernautagent/... ./test/integration/kubernautagent/...
	@if [ -f coverage_integration_kubernautagent.out ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_integration_kubernautagent.out"; \
		go tool cover -func=coverage_integration_kubernautagent.out | grep total || echo "No coverage data"; \
	fi

# DataStorage integration tests: exclude generated code from coverage
.PHONY: test-integration-datastorage
test-integration-datastorage: generate ginkgo setup-envtest ensure-coverage-dirs ## Run datastorage integration tests (coverage excludes ogen-client)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 datastorage - Integration Tests ($(TEST_PROCS) procs) [coverage: hand-written code only]"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Pattern: DD-INTEGRATION-001 v2.0 (envtest + Podman dependencies)"
	@KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" $(GINKGO) -v --timeout=$(TEST_TIMEOUT_INTEGRATION) --procs=$(TEST_PROCS) --coverprofile=coverage_integration_datastorage.out --covermode=atomic --keep-going --coverpkg=$(DATASTORAGE_COVERPKG) ./test/integration/datastorage/...
	@if [ -f coverage_integration_datastorage.out ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_integration_datastorage.out"; \
		go tool cover -func=coverage_integration_datastorage.out | grep total || echo "No coverage data"; \
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
	@GINKGO_CMD="$(GINKGO) -v --timeout=$(TEST_TIMEOUT_E2E) --procs=$(TEST_PROCS) --coverprofile=coverage_e2e_$*.out --covermode=atomic --coverpkg=github.com/jordigilh/kubernaut/pkg/$*/...,github.com/jordigilh/kubernaut/internal/controller/$*/..."; \
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
	@# DD-TEST-007: Prefer GOCOVERDIR binary coverage (deployed service instrumentation)
	@# over Ginkgo --coverprofile (test runner coverage only)
	@if [ -f coverage_e2e_$*_binary.out ]; then \
		echo "📊 Using GOCOVERDIR binary coverage (deployed service instrumentation)"; \
		cp coverage_e2e_$*_binary.out coverage_e2e_$*.out; \
	fi
	@if [ -f coverage_e2e_$*.out ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_e2e_$*.out"; \
		go tool cover -func=coverage_e2e_$*.out | grep total || echo "No coverage data"; \
	fi

# DataStorage E2E tests: exclude generated code from coverage; keep client pre-generation step
.PHONY: test-e2e-datastorage
test-e2e-datastorage: generate ginkgo ensure-coverage-dirs ## Run datastorage E2E tests (coverage excludes ogen-client)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 datastorage - E2E Tests (Kind cluster, $(TEST_PROCS) procs) [coverage: hand-written code only]"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🔍 Pre-validating DataStorage OpenAPI client generation..."
	@$(MAKE) generate-datastorage-client || { echo "❌ DataStorage client generation failed"; exit 1; }
	@echo "✅ DataStorage client validated successfully"
	@GINKGO_CMD="$(GINKGO) -v --timeout=$(TEST_TIMEOUT_E2E) --procs=$(TEST_PROCS) --coverprofile=coverage_e2e_datastorage.out --covermode=atomic --coverpkg=$(DATASTORAGE_COVERPKG)"; \
	if [ -n "$(GINKGO_LABEL)" ]; then GINKGO_CMD="$$GINKGO_CMD --label-filter='$(GINKGO_LABEL)'"; fi; \
	if [ -n "$(GINKGO_FOCUS)" ]; then GINKGO_CMD="$$GINKGO_CMD --focus='$(GINKGO_FOCUS)'"; fi; \
	if [ -n "$(GINKGO_SKIP)" ]; then GINKGO_CMD="$$GINKGO_CMD --skip='$(GINKGO_SKIP)'"; fi; \
	eval "$$GINKGO_CMD ./test/e2e/datastorage/..."
	@# DD-TEST-007: Prefer GOCOVERDIR binary coverage over Ginkgo --coverprofile
	@if [ -f coverage_e2e_datastorage_binary.out ]; then \
		echo "📊 Using GOCOVERDIR binary coverage (deployed service instrumentation)"; \
		cp coverage_e2e_datastorage_binary.out coverage_e2e_datastorage.out; \
	fi
	@if [ -f coverage_e2e_datastorage.out ]; then \
		echo ""; echo "📊 Coverage report generated: coverage_e2e_datastorage.out"; \
		go tool cover -func=coverage_e2e_datastorage.out | grep total || echo "No coverage data"; \
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

##@ Test Workflow Image Targets

# Registry for test workflow OCI images (DD-WORKFLOW-017, ADR-043)
# Override for CI: WORKFLOW_REGISTRY=ghcr.io/jordigilh/kubernaut/test-workflows
WORKFLOW_REGISTRY ?= quay.io/kubernaut-cicd/test-workflows
WORKFLOW_VERSION ?= v1.0.0
WORKFLOW_FIXTURES_DIR := test/fixtures/workflows
WORKFLOW_PLACEHOLDER_DIR := test/fixtures/execution-placeholder
# Platforms to build workflow images for (multi-arch manifest)
WORKFLOW_PLATFORMS ?= linux/amd64,linux/arm64

# _build_workflow_manifest builds a multi-arch manifest for one workflow.
# Usage: $(call _build_workflow_manifest,<ref>,<dockerfile>,<context-dir>)
define _build_workflow_manifest
	@$(CONTAINER_TOOL) rmi "$(1)" 2>/dev/null || true
	@$(CONTAINER_TOOL) manifest rm "$(1)" 2>/dev/null || true
	@$(CONTAINER_TOOL) build --platform $(WORKFLOW_PLATFORMS) --manifest "$(1)" -f "$(2)" "$(3)"
endef

.PHONY: build-test-workflows
build-test-workflows: ## Build test workflow OCI images (two-pass: exec→digest-sync→schema)
	@echo "📦 Building test workflow OCI images (two-pass build)..."
	@echo "  Registry:  $(WORKFLOW_REGISTRY)"
	@echo "  Version:   $(WORKFLOW_VERSION)"
	@echo "  Platforms: $(WORKFLOW_PLATFORMS)"
	@echo ""
	@# ══════════════════════════════════════════════════════════════════
	@# PASS 1: Build and push exec images, capture digests, sync schemas
	@#
	@# Schema YAML files reference exec bundles by sha256 digest.
	@# Rebuilding exec images changes their manifest list digest, so we
	@# push first, capture the remote digest via skopeo, and auto-update
	@# all schema files before building schema images in Pass 2.
	@# ══════════════════════════════════════════════════════════════════
	@echo "══ Pass 1: Exec images (build → push → digest-sync) ══"
	@echo ""
	@# --- placeholder-execution (referenced by 20+ schema files) ---
	@echo "  Building placeholder-execution -> $(WORKFLOW_REGISTRY)/placeholder-execution:$(WORKFLOW_VERSION)"
	$(call _build_workflow_manifest,$(WORKFLOW_REGISTRY)/placeholder-execution:$(WORKFLOW_VERSION),$(WORKFLOW_PLACEHOLDER_DIR)/Dockerfile,$(WORKFLOW_PLACEHOLDER_DIR)/)
	@echo "  Pushing placeholder-execution..."
	@$(CONTAINER_TOOL) manifest push --all "$(WORKFLOW_REGISTRY)/placeholder-execution:$(WORKFLOW_VERSION)" "docker://$(WORKFLOW_REGISTRY)/placeholder-execution:$(WORKFLOW_VERSION)"
	@DIGEST=$$(skopeo inspect --raw "docker://$(WORKFLOW_REGISTRY)/placeholder-execution:$(WORKFLOW_VERSION)" | sha256sum | awk '{print $$1}'); \
	echo "  ✅ placeholder-execution digest: sha256:$$DIGEST"; \
	echo "  Syncing placeholder-execution digest into schema files..."; \
	for f in $$(grep -rl --include='*.yaml' --include='*.go' 'placeholder-execution:$(WORKFLOW_VERSION)@sha256:' test/); do \
		sed -i.bak "s|placeholder-execution:$(WORKFLOW_VERSION)@sha256:[a-f0-9]\{64\}|placeholder-execution:$(WORKFLOW_VERSION)@sha256:$$DIGEST|g" "$$f" && rm -f "$$f.bak"; \
		echo "    ✏️  $$f"; \
	done
	@# --- Per-directory exec images (workflows with custom Dockerfiles) ---
	@for dir in $(WORKFLOW_FIXTURES_DIR)/*/; do \
		name=$$(basename "$$dir"); \
		if [ "$$name" = "README.md" ] || [ ! -f "$$dir/workflow-schema.yaml" ]; then continue; fi; \
		case "$$name" in *-v[0-9]*) continue ;; esac; \
		if [ -f "$$dir/Dockerfile" ]; then \
			ref="$(WORKFLOW_REGISTRY)/$$name:$(WORKFLOW_VERSION)-exec"; \
			echo "  Building $$name (exec) -> $$ref"; \
			$(CONTAINER_TOOL) rmi "$$ref" 2>/dev/null || true; \
			$(CONTAINER_TOOL) manifest rm "$$ref" 2>/dev/null || true; \
			$(CONTAINER_TOOL) build --platform $(WORKFLOW_PLATFORMS) --manifest "$$ref" -f "$$dir/Dockerfile" "$$dir" || exit 1; \
			echo "  Pushing $$name (exec)..."; \
			$(CONTAINER_TOOL) manifest push --all "$$ref" "docker://$$ref" || exit 1; \
			DIGEST=$$(skopeo inspect --raw "docker://$$ref" | sha256sum | awk '{print $$1}'); \
			echo "  ✅ $$name exec digest: sha256:$$DIGEST"; \
			sed -i.bak "s|$$name:$(WORKFLOW_VERSION)-exec@sha256:[a-f0-9]\{64\}|$$name:$(WORKFLOW_VERSION)-exec@sha256:$$DIGEST|g" "$$dir/workflow-schema.yaml" && rm -f "$$dir/workflow-schema.yaml.bak"; \
			echo "    ✏️  $$dir/workflow-schema.yaml"; \
		fi; \
	done
	@echo ""
	@# ══════════════════════════════════════════════════════════════════
	@# PASS 2: Build schema images (exec digests now synced in YAMLs)
	@# ══════════════════════════════════════════════════════════════════
	@echo "══ Pass 2: Schema images (build with synced digests) ══"
	@echo ""
	@for dir in $(WORKFLOW_FIXTURES_DIR)/*/; do \
		name=$$(basename "$$dir"); \
		if [ "$$name" = "README.md" ] || [ ! -f "$$dir/workflow-schema.yaml" ]; then continue; fi; \
		case "$$name" in *-v[0-9]*) continue ;; esac; \
		ref="$(WORKFLOW_REGISTRY)/$$name:$(WORKFLOW_VERSION)"; \
		echo "  Building $$name (schema) -> $$ref"; \
		$(CONTAINER_TOOL) rmi "$$ref" 2>/dev/null || true; \
		$(CONTAINER_TOOL) manifest rm "$$ref" 2>/dev/null || true; \
		$(CONTAINER_TOOL) build --platform $(WORKFLOW_PLATFORMS) --manifest "$$ref" -f "$(WORKFLOW_FIXTURES_DIR)/Dockerfile" "$$dir" || exit 1; \
	done
	@# Multi-version variants for version management E2E tests (07_workflow_version_management_test.go)
	@echo "  Building oom-recovery:v1.1.0 (version variant)"
	$(call _build_workflow_manifest,$(WORKFLOW_REGISTRY)/oom-recovery:v1.1.0,$(WORKFLOW_FIXTURES_DIR)/Dockerfile,$(WORKFLOW_FIXTURES_DIR)/oom-recovery-v1.1/)
	@echo "  Building oom-recovery:v2.0.0 (version variant)"
	$(call _build_workflow_manifest,$(WORKFLOW_REGISTRY)/oom-recovery:v2.0.0,$(WORKFLOW_FIXTURES_DIR)/Dockerfile,$(WORKFLOW_FIXTURES_DIR)/oom-recovery-v2.0/)
	@echo ""
	@echo "✅ All test workflow images built ($(WORKFLOW_PLATFORMS))"
	@echo "   Exec images pushed in Pass 1. Run 'make push-test-workflows' for schema images."

.PHONY: push-test-workflows
push-test-workflows: ## Push test workflow schema images to registry (exec images pushed during build)
	@echo "📦 Pushing test workflow schema images (multi-arch)..."
	@echo "  Registry:  $(WORKFLOW_REGISTRY)"
	@echo "  Version:   $(WORKFLOW_VERSION)"
	@echo "  Platforms: $(WORKFLOW_PLATFORMS)"
	@echo ""
	@# Push schema images (exec images already pushed during build-test-workflows Pass 1)
	@for dir in $(WORKFLOW_FIXTURES_DIR)/*/; do \
		name=$$(basename "$$dir"); \
		if [ "$$name" = "README.md" ] || [ ! -f "$$dir/workflow-schema.yaml" ]; then continue; fi; \
		case "$$name" in *-v[0-9]*) continue ;; esac; \
		ref="$(WORKFLOW_REGISTRY)/$$name:$(WORKFLOW_VERSION)"; \
		echo "  Pushing $$name (schema) -> $$ref"; \
		$(CONTAINER_TOOL) manifest push --all "$$ref" "docker://$$ref" || exit 1; \
		echo "  ✅ Pushed $$ref"; \
	done
	@# Multi-version variants for version management E2E tests
	@echo "  Pushing oom-recovery:v1.1.0 (version variant)"
	@$(CONTAINER_TOOL) manifest push --all "$(WORKFLOW_REGISTRY)/oom-recovery:v1.1.0" "docker://$(WORKFLOW_REGISTRY)/oom-recovery:v1.1.0"
	@echo "  Pushing oom-recovery:v2.0.0 (version variant)"
	@$(CONTAINER_TOOL) manifest push --all "$(WORKFLOW_REGISTRY)/oom-recovery:v2.0.0" "docker://$(WORKFLOW_REGISTRY)/oom-recovery:v2.0.0"
	@echo ""
	@echo "✅ All test workflow schema images pushed to $(WORKFLOW_REGISTRY) ($(WORKFLOW_PLATFORMS))"

##@ Tekton Bundle Image Targets

# Registry for Tekton Pipeline bundle images (separate from schema-only workflow images)
# Tekton bundles are built with `tkn bundle push` and contain Tekton Pipeline resources
# with required annotations (dev.tekton.image.apiVersion, dev.tekton.image.kind, etc.)
# Schema images (test-workflows/) are for DataStorage registration; bundles (tekton-bundles/) are for WFE execution.
TEKTON_BUNDLE_REGISTRY ?= quay.io/kubernaut-cicd/tekton-bundles
TEKTON_FIXTURES_DIR := test/fixtures/tekton

.PHONY: push-tekton-bundles
push-tekton-bundles: ## Build and push Tekton Pipeline bundle images (tkn bundle push builds+pushes in one step)
	@echo "📦 Building and pushing Tekton Pipeline bundles..."
	@echo "  Registry: $(TEKTON_BUNDLE_REGISTRY)"
	@echo "  Version:  $(WORKFLOW_VERSION)"
	@echo ""
	@echo "  Building+pushing hello-world Tekton bundle..."
	tkn bundle push "$(TEKTON_BUNDLE_REGISTRY)/hello-world:$(WORKFLOW_VERSION)" \
		-f $(TEKTON_FIXTURES_DIR)/hello-world-pipeline.yaml
	@echo "  ✅ Pushed $(TEKTON_BUNDLE_REGISTRY)/hello-world:$(WORKFLOW_VERSION)"
	@echo ""
	@echo "  Building+pushing failing Tekton bundle..."
	tkn bundle push "$(TEKTON_BUNDLE_REGISTRY)/failing:$(WORKFLOW_VERSION)" \
		-f $(TEKTON_FIXTURES_DIR)/failing-pipeline.yaml
	@echo "  ✅ Pushed $(TEKTON_BUNDLE_REGISTRY)/failing:$(WORKFLOW_VERSION)"
	@echo ""
	@echo "✅ All Tekton bundles pushed to $(TEKTON_BUNDLE_REGISTRY)"

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

.PHONY: validate-openapi-datastorage
validate-openapi-datastorage: ## Validate Data Storage OpenAPI spec syntax (CI - ADR-031)
	@echo "🔍 Validating Data Storage OpenAPI spec..."
	@docker run --rm -v "$(PWD):/local" openapitools/openapi-generator-cli:v7.2.0 validate \
		-i /local/api/openapi/data-storage-v1.yaml || \
		(echo "❌ OpenAPI spec validation failed!" && exit 1)
	@echo "✅ Data Storage OpenAPI spec is valid"

.PHONY: test-e2e-kubernautagent
test-e2e-kubernautagent: ginkgo ensure-coverage-dirs generate-agentclient ## Run Kubernaut Agent E2E tests (Kind cluster, ~10 min)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Kubernaut Agent E2E Tests (#433 — API Contract Parity)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Validates: Same OpenAPI contract as retired Python KA (HAPI)"
	@echo "🔧 Test Framework: Ginkgo/Gomega (Go BDD)"
	@echo "📦 Dockerfile: docker/kubernautagent.Dockerfile (ADR-027 UBI10)"
	@echo "⏱️  Expected Duration: ~10 minutes"
	@echo ""
	@echo "🧪 Running KA E2E tests (test/e2e/kubernautagent/)..."
	@$(GINKGO) -v --timeout=15m --coverprofile=coverage_e2e_kubernautagent.out --covermode=atomic --coverpkg=github.com/jordigilh/kubernaut/pkg/kubernautagent/...,github.com/jordigilh/kubernaut/internal/kubernautagent/... ./test/e2e/kubernautagent/...
	@if [ -f coverage_e2e_kubernautagent_binary.out ]; then \
		echo "📊 Using GOCOVERDIR binary coverage (deployed service instrumentation)"; \
		cp coverage_e2e_kubernautagent_binary.out coverage_e2e_kubernautagent.out; \
	fi
	@if [ -f coverage_e2e_kubernautagent.out ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_e2e_kubernautagent.out"; \
		go tool cover -func=coverage_e2e_kubernautagent.out | grep total || echo "No coverage data"; \
	fi

##@ Special Cases - Authentication Webhook

.PHONY: test-unit-authwebhook
test-unit-authwebhook: ginkgo ensure-coverage-dirs ## Run authentication webhook unit tests
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Authentication Webhook - Unit Tests ($(TEST_PROCS) procs)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_UNIT) --procs=$(TEST_PROCS) --coverprofile=coverage_unit_authwebhook.out --covermode=atomic --coverpkg=github.com/jordigilh/kubernaut/pkg/authwebhook/... ./test/unit/authwebhook/...
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
	@$(GINKGO) -v --timeout=$(TEST_TIMEOUT_E2E) --procs=$(TEST_PROCS) --coverprofile=coverage_e2e_authwebhook.out --covermode=atomic --coverpkg=github.com/jordigilh/kubernaut/pkg/authwebhook/... ./test/e2e/authwebhook/...
	@# DD-TEST-007: Prefer GOCOVERDIR binary coverage over Ginkgo --coverprofile
	@if [ -f coverage_e2e_authwebhook_binary.out ]; then \
		echo "📊 Using GOCOVERDIR binary coverage (deployed service instrumentation)"; \
		cp coverage_e2e_authwebhook_binary.out coverage_e2e_authwebhook.out; \
	fi
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

# Full Pipeline E2E: Complete remediation lifecycle test (Issue #39)
# Deploys ALL services in a single Kind cluster - requires ~6GB RAM
# CI/CD: Set IMAGE_REGISTRY + IMAGE_TAG to use pre-built images (fast)
# Local: Builds 3 images at a time (slow, ~20-30 min)
.PHONY: test-e2e-fullpipeline
test-e2e-fullpipeline: ginkgo ensure-coverage-dirs ## Run full pipeline E2E tests (all services, Kind cluster, ~30 min)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Full Pipeline E2E Tests (Issue #39)"
	@echo "   All Kubernaut services in a single Kind cluster"
	@echo "   Event → Gateway → RO → SP → AA → KA → WE(Job) → Notification"
	@echo "════════════════════════════════════════════════════════════════════════"
	@$(GINKGO) -v --timeout=50m --procs=$(TEST_PROCS) ./test/e2e/fullpipeline/...
	@echo "✅ Full Pipeline E2E tests completed!"

##@ Legacy Aliases (Backward Compatibility)

.PHONY: test-gateway
test-gateway: test-integration-gateway ## Legacy alias for Gateway integration tests

.PHONY: test
test: test-tier-unit ## Legacy alias: Run all unit tests

##@ Coverage Analysis

.PHONY: coverage-report-unit-testable
coverage-report-unit-testable: ## Show comprehensive coverage breakdown by test tier for all services
	@python3 scripts/coverage/coverage_report.py

.PHONY: coverage-report-json
coverage-report-json: ## Generate JSON coverage report for CI/CD integration
	@python3 scripts/coverage/coverage_report.py --format json

.PHONY: coverage-report-markdown
coverage-report-markdown: ## Generate markdown coverage report for GitHub PR comments
	@python3 scripts/coverage/coverage_report.py --format markdown

.PHONY: coverage-report
coverage-report: coverage-report-unit-testable ## Alias for coverage-report-unit-testable

# REMOVED: Legacy 150-line embedded implementation
# Replaced with modular scripts/coverage/report.sh (see Phase 1-3 refactoring)
# If rollback needed, see git history before this commit

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
CRD_REF_DOCS ?= $(LOCALBIN)/crd-ref-docs

## Tool Versions
KUSTOMIZE_VERSION ?= v5.6.0
CONTROLLER_TOOLS_VERSION ?= v0.19.0
OGEN_VERSION ?= v1.18.0
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')
GOLANGCI_LINT_VERSION ?= v2.1.0
GINKGO_VERSION ?= v2.28.1
CRD_REF_DOCS_VERSION ?= v0.3.0

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

.PHONY: crd-ref-docs
crd-ref-docs: $(CRD_REF_DOCS) ## Download crd-ref-docs locally if necessary
$(CRD_REF_DOCS): $(LOCALBIN)
	$(call go-install-tool,$(CRD_REF_DOCS),github.com/elastic/crd-ref-docs,$(CRD_REF_DOCS_VERSION))

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
lint-rules: lint-test-patterns lint-business-integration lint-tdd-compliance lint-naming-convention ## Run all cursor rule compliance checks

.PHONY: lint-naming-convention
lint-naming-convention: ## Check for legacy holmesgpt-api references (#691)
	@echo "🔍 Checking for legacy holmesgpt-api naming..."
	@HITS=$$(grep -rl "holmesgpt-api" config/ deploy/ internal/ pkg/ cmd/ charts/ 2>/dev/null || true); \
	if [ -n "$$HITS" ]; then \
		echo "❌ Legacy 'holmesgpt-api' references found:"; \
		echo "$$HITS"; \
		exit 1; \
	fi
	@echo "✅ No legacy holmesgpt-api references in source code"

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

##@ Image Build & Push

# Registry, tag, and architecture (override via env or CLI)
IMAGE_REGISTRY ?= quay.io/kubernaut-ai
IMAGE_TAG ?= latest
# Auto-detect native architecture (maps uname output to Go-style names)
IMAGE_ARCH ?= $(shell uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')

# Version metadata for container image labels and Go ldflags
# Read from VERSION file (single source of truth); override via env or CLI.
APP_VERSION ?= v$(shell cat VERSION 2>/dev/null || echo 0.0.0-dev)
GIT_COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE  ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

# Go linker flags for version injection via internal/version package
LDFLAGS ?= -ldflags "-X github.com/jordigilh/kubernaut/internal/version.Version=$(APP_VERSION) -X github.com/jordigilh/kubernaut/internal/version.GitCommit=$(GIT_COMMIT) -X github.com/jordigilh/kubernaut/internal/version.BuildDate=$(BUILD_DATE)"

# All Go services with their Dockerfile mappings
IMAGE_SERVICES := datastorage gateway aianalysis authwebhook notification remediationorchestrator signalprocessing workflowexecution effectivenessmonitor kubernautagent db-migrate
IMAGE_DOCKERFILES_datastorage := docker/data-storage.Dockerfile
IMAGE_DOCKERFILES_gateway := docker/gateway.Dockerfile
IMAGE_DOCKERFILES_aianalysis := docker/aianalysis.Dockerfile
IMAGE_DOCKERFILES_authwebhook := docker/authwebhook.Dockerfile
IMAGE_DOCKERFILES_notification := docker/notification-controller.Dockerfile
IMAGE_DOCKERFILES_remediationorchestrator := docker/remediationorchestrator-controller.Dockerfile
IMAGE_DOCKERFILES_signalprocessing := docker/signalprocessing-controller.Dockerfile
IMAGE_DOCKERFILES_workflowexecution := docker/workflowexecution-controller.Dockerfile
IMAGE_DOCKERFILES_effectivenessmonitor := docker/effectivenessmonitor-controller.Dockerfile
IMAGE_DOCKERFILES_kubernautagent := docker/kubernautagent.Dockerfile
IMAGE_DOCKERFILES_db-migrate := docker/db-migrate.Dockerfile

# IMAGE_TARGET: Dockerfile --target stage to build. Empty = last stage (development).
# Set IMAGE_TARGET=production for release builds (scratch runtime, zero CVE surface).
IMAGE_TARGET ?=

# _image_build_one builds a single service image for a specific platform.
# --platform ensures TARGETARCH is set correctly for cross-compilation (e.g., arm64 on amd64 host).
# Usage: $(call _image_build_one,<service>,<dockerfile>)
define _image_build_one
	@echo "  Building $(1) [$(IMAGE_ARCH)]$(if $(IMAGE_TARGET), (target: $(IMAGE_TARGET)),)..."
	@$(CONTAINER_TOOL) build --platform linux/$(IMAGE_ARCH) \
		$(if $(IMAGE_TARGET),--target $(IMAGE_TARGET),) \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE_REGISTRY)/$(1):$(IMAGE_TAG)-$(IMAGE_ARCH) -f $(2) .

endef

# _image_push_one pushes an arch-suffixed image for a single service.
# Usage: $(call _image_push_one,<service>)
define _image_push_one
	@echo "  Pushing $(IMAGE_REGISTRY)/$(1):$(IMAGE_TAG)-$(IMAGE_ARCH)..."
	@$(CONTAINER_TOOL) push $(IMAGE_REGISTRY)/$(1):$(IMAGE_TAG)-$(IMAGE_ARCH)

endef

# NOTE: Generated files (openapi_spec_data.yaml, ogen client) must be committed before
# running image-build on hosts without Go. Run `make generate` locally first if needed.
.PHONY: image-build
image-build: ## Build images for all services (native arch, arch-suffixed tag)
	@echo "🐳 Building service images [$(IMAGE_ARCH)]..."
	@echo "   Registry: $(IMAGE_REGISTRY)"
	@echo "   Tag:      $(IMAGE_TAG)-$(IMAGE_ARCH)"
	@echo ""
	$(foreach svc,$(IMAGE_SERVICES),$(call _image_build_one,$(svc),$(IMAGE_DOCKERFILES_$(svc))))
	@echo "  Building must-gather [$(IMAGE_ARCH)]..."
	@$(CONTAINER_TOOL) build --platform linux/$(IMAGE_ARCH) \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE_REGISTRY)/must-gather:$(IMAGE_TAG)-$(IMAGE_ARCH) -f cmd/must-gather/Dockerfile cmd/must-gather/
	@echo ""
	@echo "✅ All images built ($(IMAGE_REGISTRY):$(IMAGE_TAG)-$(IMAGE_ARCH))."
	@echo "   Push with: make image-push IMAGE_TAG=$(IMAGE_TAG)"

.PHONY: image-push
image-push: ## Push arch-suffixed images to registry
	@echo "📤 Pushing images to $(IMAGE_REGISTRY)..."
	@echo "   Tag: $(IMAGE_TAG)-$(IMAGE_ARCH)"
	@echo ""
	$(foreach svc,$(IMAGE_SERVICES),$(call _image_push_one,$(svc)))
	@echo "  Pushing $(IMAGE_REGISTRY)/must-gather:$(IMAGE_TAG)-$(IMAGE_ARCH)..."
	@$(CONTAINER_TOOL) push $(IMAGE_REGISTRY)/must-gather:$(IMAGE_TAG)-$(IMAGE_ARCH)
	@echo ""
	@echo "✅ All images pushed to $(IMAGE_REGISTRY) with tag $(IMAGE_TAG)-$(IMAGE_ARCH)."

.PHONY: image-manifest
image-manifest: ## Create and push multi-arch manifests (run after both arches are pushed)
	@echo "🔗 Creating multi-arch manifests..."
	@echo "   Registry: $(IMAGE_REGISTRY)"
	@echo "   Tag:      $(IMAGE_TAG)"
	@echo "   Arches:   amd64, arm64"
	@echo ""
	@for svc in $(IMAGE_SERVICES) must-gather; do \
	    echo "  Manifest: $$svc"; \
	    $(CONTAINER_TOOL) manifest rm $(IMAGE_REGISTRY)/$$svc:$(IMAGE_TAG) 2>/dev/null || true; \
	    $(CONTAINER_TOOL) manifest create $(IMAGE_REGISTRY)/$$svc:$(IMAGE_TAG) \
	        $(IMAGE_REGISTRY)/$$svc:$(IMAGE_TAG)-amd64 \
	        $(IMAGE_REGISTRY)/$$svc:$(IMAGE_TAG)-arm64; \
	    $(CONTAINER_TOOL) manifest push $(IMAGE_REGISTRY)/$$svc:$(IMAGE_TAG) \
	        docker://$(IMAGE_REGISTRY)/$$svc:$(IMAGE_TAG); \
	done
	@echo ""
	@echo "✅ All manifests pushed as $(IMAGE_REGISTRY):$(IMAGE_TAG)."

# Per-service image targets (e.g., make image-build-aianalysis IMAGE_TAG=demo-v1.0)
.PHONY: image-build-%
image-build-%: ## Build a single service image (specified arch via IMAGE_ARCH)
	@if [ "$*" = "must-gather" ]; then \
	    echo "  Building must-gather [$(IMAGE_ARCH)]..."; \
	    $(CONTAINER_TOOL) build --platform linux/$(IMAGE_ARCH) \
	        --build-arg APP_VERSION=$(APP_VERSION) \
	        --build-arg GIT_COMMIT=$(GIT_COMMIT) \
	        --build-arg BUILD_DATE=$(BUILD_DATE) \
	        -t $(IMAGE_REGISTRY)/must-gather:$(IMAGE_TAG)-$(IMAGE_ARCH) -f cmd/must-gather/Dockerfile cmd/must-gather/; \
	elif [ -n "$(IMAGE_DOCKERFILES_$*)" ]; then \
	    echo "  Building $* [$(IMAGE_ARCH)]$(if $(IMAGE_TARGET), (target: $(IMAGE_TARGET)),)..."; \
	    $(CONTAINER_TOOL) build --platform linux/$(IMAGE_ARCH) \
	        $(if $(IMAGE_TARGET),--target $(IMAGE_TARGET),) \
	        --build-arg APP_VERSION=$(APP_VERSION) \
	        --build-arg GIT_COMMIT=$(GIT_COMMIT) \
	        --build-arg BUILD_DATE=$(BUILD_DATE) \
	        -t $(IMAGE_REGISTRY)/$*:$(IMAGE_TAG)-$(IMAGE_ARCH) -f $(IMAGE_DOCKERFILES_$*) .; \
	else \
	    echo "ERROR: Unknown service '$*'. Available: $(IMAGE_SERVICES) must-gather"; exit 1; \
	fi

.PHONY: image-push-%
image-push-%: ## Push a single service image (arch-suffixed)
	@echo "  Pushing $(IMAGE_REGISTRY)/$*:$(IMAGE_TAG)-$(IMAGE_ARCH)..."
	@$(CONTAINER_TOOL) push $(IMAGE_REGISTRY)/$*:$(IMAGE_TAG)-$(IMAGE_ARCH)

.PHONY: image-manifest-%
image-manifest-%: ## Create and push multi-arch manifest for a single service
	@echo "  Manifest: $*"
	@$(CONTAINER_TOOL) manifest rm $(IMAGE_REGISTRY)/$*:$(IMAGE_TAG) 2>/dev/null || true
	@$(CONTAINER_TOOL) manifest create $(IMAGE_REGISTRY)/$*:$(IMAGE_TAG) \
	    $(IMAGE_REGISTRY)/$*:$(IMAGE_TAG)-amd64 \
	    $(IMAGE_REGISTRY)/$*:$(IMAGE_TAG)-arm64
	@$(CONTAINER_TOOL) manifest push $(IMAGE_REGISTRY)/$*:$(IMAGE_TAG) \
	    docker://$(IMAGE_REGISTRY)/$*:$(IMAGE_TAG)
	@echo "  ✅ Manifest pushed: $(IMAGE_REGISTRY)/$*:$(IMAGE_TAG)"
