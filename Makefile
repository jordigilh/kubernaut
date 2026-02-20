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
# - HolmesGPT: pkg/holmesgpt/client contains oas_*_gen.go (ogen-generated client)
#
# Why DataStorage unit coverage is ~27%: Unit tests (462 specs) cover builders, validation, config,
# aggregation handlers (with mocks). The remaining ~349 functions at 0% are HTTP handlers, DLQ worker,
# repository/DB adapter, server bootstrap—exercised in integration and E2E tests. Unit-only coverage
# is intentionally lower; total coverage across all tiers is the complete picture.
# Run: make test-all-datastorage then see coverage_*_datastorage.out

# DataStorage coverage packages (hand-written only, excludes generated)
# DATASTORAGE_COVERPKG: Comma-separated list of packages for coverage instrumentation.
# IMPORTANT: No spaces after commas — Go's --coverpkg treats spaces as part of the package name.
DATASTORAGE_COVERPKG = github.com/jordigilh/kubernaut/pkg/datastorage/adapter/...,github.com/jordigilh/kubernaut/pkg/datastorage/audit/...,github.com/jordigilh/kubernaut/pkg/datastorage/config/...,github.com/jordigilh/kubernaut/pkg/datastorage/dlq/...,github.com/jordigilh/kubernaut/pkg/datastorage/metrics/...,github.com/jordigilh/kubernaut/pkg/datastorage/models/...,github.com/jordigilh/kubernaut/pkg/datastorage/query/...,github.com/jordigilh/kubernaut/pkg/datastorage/reconstruction/...,github.com/jordigilh/kubernaut/pkg/datastorage/repository/...,github.com/jordigilh/kubernaut/pkg/datastorage/repository/sql/...,github.com/jordigilh/kubernaut/pkg/datastorage/repository/sqlutil/...,github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow/...,github.com/jordigilh/kubernaut/pkg/datastorage/schema/...,github.com/jordigilh/kubernaut/pkg/datastorage/scoring/...,github.com/jordigilh/kubernaut/pkg/datastorage/server/...,github.com/jordigilh/kubernaut/pkg/datastorage/server/helpers/...,github.com/jordigilh/kubernaut/pkg/datastorage/server/middleware/...,github.com/jordigilh/kubernaut/pkg/datastorage/server/response/...,github.com/jordigilh/kubernaut/pkg/datastorage/validation/...

# Unit-testable package patterns (pure logic: config, validators, builders, formatters, metrics, classifiers)
# Integration-testable patterns (I/O-dependent: handlers, servers, DB adapters, K8s clients, workers)
# NOTE: These patterns are used by scripts/coverage/ AWK scripts - see Phase 2 refactoring
AIANALYSIS_UNIT_PATTERN = pkg/aianalysis/handlers/|pkg/aianalysis/metrics/|pkg/aianalysis/phase/|pkg/aianalysis/rego/|pkg/aianalysis/conditions
AUTHWEBHOOK_UNIT_PATTERN = pkg/authwebhook/config/|pkg/authwebhook/validation/|pkg/authwebhook/types
GATEWAY_UNIT_PATTERN = pkg/gateway/adapters/|pkg/gateway/config/|pkg/gateway/errors/|pkg/gateway/types/|pkg/gateway/processing/clock|pkg/gateway/processing/deduplication_types|pkg/gateway/processing/errors|pkg/gateway/processing/phase_checker|pkg/gateway/middleware/
NOTIFICATION_UNIT_PATTERN = pkg/notification/config/|pkg/notification/formatting/|pkg/notification/metrics/|pkg/notification/retry/|pkg/notification/routing/|pkg/notification/types|pkg/notification/conditions
REMEDIATIONORCHESTRATOR_UNIT_PATTERN = pkg/remediationorchestrator/audit/|pkg/remediationorchestrator/config/|pkg/remediationorchestrator/helpers/|pkg/remediationorchestrator/metrics/|pkg/remediationorchestrator/phase/|pkg/remediationorchestrator/routing/|pkg/remediationorchestrator/timeout/|pkg/remediationorchestrator/types|pkg/remediationorchestrator/handler/skip/|pkg/remediationorchestrator/interfaces
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
	@PATH="$(LOCALBIN):$$PATH" go generate ./pkg/holmesgpt/client/...
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
build-test-workflows: ## Build all test workflow OCI images (multi-arch: amd64 + arm64)
	@echo "📦 Building test workflow OCI images (multi-arch)..."
	@echo "  Registry:  $(WORKFLOW_REGISTRY)"
	@echo "  Version:   $(WORKFLOW_VERSION)"
	@echo "  Platforms: $(WORKFLOW_PLATFORMS)"
	@echo ""
	@# Phase 0: Build placeholder execution image referenced by all Tekton workflow schemas.
	@# All Tekton workflow-schema.yaml files point their execution.bundle at this image
	@# so the DataStorage bundle-existence check (crane.Head) passes during registration.
	@echo "  Building placeholder-execution -> $(WORKFLOW_REGISTRY)/placeholder-execution:$(WORKFLOW_VERSION)"
	$(call _build_workflow_manifest,$(WORKFLOW_REGISTRY)/placeholder-execution:$(WORKFLOW_VERSION),$(WORKFLOW_PLACEHOLDER_DIR)/Dockerfile,$(WORKFLOW_PLACEHOLDER_DIR)/)
	@# Phase 1: Build execution images for workflows with per-directory Dockerfiles.
	@# These contain runnable content (scripts, kubectl, etc.) but NOT workflow-schema.yaml.
	@# Tagged as :VERSION-exec so they can be referenced by digest in the schema.
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
		fi; \
	done
	@# Phase 2: Build schema-only images for ALL workflows using shared FROM scratch Dockerfile.
	@# DataStorage pulls these to extract /workflow-schema.yaml for catalog registration.
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

.PHONY: push-test-workflows
push-test-workflows: ## Push test workflow multi-arch manifests to registry
	@echo "📦 Pushing test workflow OCI images (multi-arch)..."
	@echo "  Registry:  $(WORKFLOW_REGISTRY)"
	@echo "  Version:   $(WORKFLOW_VERSION)"
	@echo "  Platforms: $(WORKFLOW_PLATFORMS)"
	@echo ""
	@# Push placeholder execution image (referenced by all Tekton workflow schemas)
	@echo "  Pushing placeholder-execution -> $(WORKFLOW_REGISTRY)/placeholder-execution:$(WORKFLOW_VERSION)"
	@$(CONTAINER_TOOL) manifest push --all "$(WORKFLOW_REGISTRY)/placeholder-execution:$(WORKFLOW_VERSION)" "docker://$(WORKFLOW_REGISTRY)/placeholder-execution:$(WORKFLOW_VERSION)"
	@echo "  ✅ Pushed $(WORKFLOW_REGISTRY)/placeholder-execution:$(WORKFLOW_VERSION)"
	@# Push execution images first (workflows with per-directory Dockerfiles)
	@for dir in $(WORKFLOW_FIXTURES_DIR)/*/; do \
		name=$$(basename "$$dir"); \
		if [ "$$name" = "README.md" ] || [ ! -f "$$dir/workflow-schema.yaml" ]; then continue; fi; \
		case "$$name" in *-v[0-9]*) continue ;; esac; \
		if [ -f "$$dir/Dockerfile" ]; then \
			ref="$(WORKFLOW_REGISTRY)/$$name:$(WORKFLOW_VERSION)-exec"; \
			echo "  Pushing $$name (exec) -> $$ref"; \
			$(CONTAINER_TOOL) manifest push --all "$$ref" "docker://$$ref" || exit 1; \
			echo "  ✅ Pushed $$ref"; \
		fi; \
	done
	@# Push schema images for all workflows
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
	@echo "✅ All test workflow images pushed to $(WORKFLOW_REGISTRY) ($(WORKFLOW_PLATFORMS))"

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
	@echo "🐳 Building HolmesGPT API Docker Image (E2E - Local Architecture)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📦 Dockerfile: holmesgpt-api/Dockerfile.e2e"
	@echo "📋 Requirements: requirements-e2e.txt (minimal dependencies)"
	@echo "💾 Size: ~800MB (excludes google-cloud-aiplatform 1.5GB)"
	@echo "🎯 Use Case: E2E testing, CI/CD"
	@echo ""
	@podman build \
		-t localhost/kubernaut-holmesgpt-api:e2e \
		-t localhost/kubernaut-holmesgpt-api:e2e-$$(git rev-parse --short HEAD) \
		-f holmesgpt-api/Dockerfile.e2e \
		.
	@echo ""
	@echo "✅ E2E image built successfully!"
	@echo "   Tags: localhost/kubernaut-holmesgpt-api:e2e"
	@echo "         localhost/kubernaut-holmesgpt-api:e2e-$$(git rev-parse --short HEAD)"

.PHONY: export-openapi-holmesgpt-api
export-openapi-holmesgpt-api: ## Export holmesgpt-api OpenAPI spec from FastAPI (ADR-045)
	@echo "📄 Exporting OpenAPI spec from FastAPI app (containerized)..."
	@mkdir -p holmesgpt-api/api
	@podman run --rm \
		-v $(CURDIR):/workspace:z \
		-w /workspace/holmesgpt-api \
		-e CONFIG_FILE=config.yaml \
		-e OPENAPI_EXPORT=1 \
		-e PYTHONUNBUFFERED=1 \
		registry.access.redhat.com/ubi9/python-312:latest \
		sh -c "find . -type d -name __pycache__ -exec rm -rf {} + 2>/dev/null; pip install -q ./src/clients/datastorage && pip install -q -r requirements-slim.txt && python3 -c 'from src.main import app; import json; print(json.dumps(app.openapi(), indent=2))' > api/openapi.json && echo 'Schema count:' && python3 -c 'import json; spec=json.load(open(\"api/openapi.json\")); print(len(spec.get(\"components\", {}).get(\"schemas\", {})))'"
	@echo "✅ OpenAPI spec exported: holmesgpt-api/api/openapi.json"

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
test-e2e-holmesgpt-api: ginkgo ensure-coverage-dirs generate-holmesgpt-client ## Run holmesgpt-api E2E tests (Kind cluster + Go Ginkgo tests, ~10 min)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 HolmesGPT API E2E Tests (Kind Cluster + Go Ginkgo Tests)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Pattern: DD-INTEGRATION-001 v2.0 (Go Ginkgo tests with Kind infrastructure)"
	@echo "🔧 Test Framework: Ginkgo/Gomega (Go BDD framework)"
	@echo "📦 Coverage: Python service code via coverage.py (DD-TEST-007)"
	@echo "⏱️  Expected Duration: ~10 minutes"
	@echo ""
	@echo "🔧 Step 1: Generate OpenAPI client (DD-HAPI-005)..."
	@cd holmesgpt-api/tests/integration && bash generate-client.sh && cd ../.. || exit 1
	@echo "✅ Client generated successfully"
	@echo ""
	@echo "🧪 Step 2: Run E2E tests (Go Ginkgo tests in test/e2e/holmesgpt-api/)..."
	@cd test/e2e/holmesgpt-api && $(GINKGO) -v --timeout=15m ./...
	@# DD-TEST-007: Python E2E coverage is collected via coverage.py inside the container
	@# The AfterSuite extracts .coverage from Kind node and generates a text report
	@if [ -f coverage_e2e_holmesgpt-api_python.txt ]; then \
		echo ""; \
		echo "📊 Python E2E coverage report: coverage_e2e_holmesgpt-api_python.txt"; \
		grep "TOTAL" coverage_e2e_holmesgpt-api_python.txt || echo "No TOTAL line found"; \
	else \
		echo "ℹ️  No Python E2E coverage data (set E2E_COVERAGE=true to enable)"; \
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
		-e PYTHONUNBUFFERED=1 \
		-e COVERAGE_FILE=/tmp/.coverage \
		registry.access.redhat.com/ubi9/python-312:latest \
		sh -c "pip install -q -r requirements.txt && pip install -q -r requirements-test.txt && pytest tests/unit/ -v --durations=20 --cov=src --cov-report=term-missing -o addopts='' && python -m coverage report --precision=2 --show-missing" 2>&1 | tee $(CURDIR)/coverage_unit_holmesgpt-api.txt
	@if [ -f $(CURDIR)/coverage_unit_holmesgpt-api.txt ]; then \
		echo ""; \
		echo "📊 Coverage report generated: coverage_unit_holmesgpt-api.txt"; \
		grep "TOTAL" $(CURDIR)/coverage_unit_holmesgpt-api.txt || echo "No coverage data"; \
	else \
		echo "⚠️  Coverage file not found (tests may have failed)"; \
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
	@echo "   Event → Gateway → RO → SP → AA → HAPI → WE(Job) → Notification"
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

##@ Demo Deployment (Issue #94)

# Demo image registry and tag (override for pre-built images)
DEMO_REGISTRY ?= quay.io/kubernaut-ai
DEMO_TAG ?= demo-v1.0
DEMO_CLUSTER_NAME ?= kubernaut-demo
DEMO_KUBECONFIG ?= $(HOME)/.kube/kubernaut-demo-config
DEMO_KIND_PROVIDER ?= $(shell command -v podman >/dev/null 2>&1 && echo "podman" || echo "docker")

# Go service → Dockerfile mapping (matches CI pipeline)
DEMO_SERVICES := datastorage gateway aianalysis authwebhook notification remediationorchestrator signalprocessing workflowexecution effectivenessmonitor
DEMO_DOCKERFILES_datastorage := docker/data-storage.Dockerfile
DEMO_DOCKERFILES_gateway := docker/gateway-ubi9.Dockerfile
DEMO_DOCKERFILES_aianalysis := docker/aianalysis.Dockerfile
DEMO_DOCKERFILES_authwebhook := docker/authwebhook.Dockerfile
DEMO_DOCKERFILES_notification := docker/notification-controller-ubi9.Dockerfile
DEMO_DOCKERFILES_remediationorchestrator := docker/remediationorchestrator-controller.Dockerfile
DEMO_DOCKERFILES_signalprocessing := docker/signalprocessing-controller.Dockerfile
DEMO_DOCKERFILES_workflowexecution := docker/workflowexecution-controller.Dockerfile
DEMO_DOCKERFILES_effectivenessmonitor := docker/effectivenessmonitor-controller.Dockerfile

# _demo_build_one builds a single demo service image.
# Usage: $(call _demo_build_one,<service>,<dockerfile>)
define _demo_build_one
	@echo "  Building $(1) from $(2)..."
	@$(CONTAINER_TOOL) build -t $(DEMO_REGISTRY)/$(1):$(DEMO_TAG) -f $(2) .
endef

.PHONY: demo-build-images
demo-build-images: generate ## Build all demo service images locally
	@echo "🐳 Building demo images ($(DEMO_REGISTRY):$(DEMO_TAG))..."
	$(foreach svc,$(DEMO_SERVICES),$(call _demo_build_one,$(svc),$(DEMO_DOCKERFILES_$(svc))))
	@echo "  Building holmesgpt-api..."
	@$(CONTAINER_TOOL) build -t $(DEMO_REGISTRY)/holmesgpt-api:$(DEMO_TAG) -f holmesgpt-api/Dockerfile.e2e .

.PHONY: demo-create-cluster
demo-create-cluster: ## Create Kind cluster for demo
	@echo "🏗️  Creating Kind cluster '$(DEMO_CLUSTER_NAME)'..."
	KIND_EXPERIMENTAL_PROVIDER=$(DEMO_KIND_PROVIDER) kind create cluster \
		--name $(DEMO_CLUSTER_NAME) \
		--config deploy/demo/overlays/kind/kind-cluster-config.yaml \
		--kubeconfig $(DEMO_KUBECONFIG)
	@echo "  Cluster created. KUBECONFIG=$(DEMO_KUBECONFIG)"

.PHONY: demo-load-images
demo-load-images: ## Load demo images into Kind cluster
	@echo "📦 Loading images into Kind cluster..."
	@for svc in $(DEMO_SERVICES); do \
		echo "  Loading $${svc}..."; \
		KIND_EXPERIMENTAL_PROVIDER=$(DEMO_KIND_PROVIDER) kind load docker-image \
			$(DEMO_REGISTRY)/$${svc}:$(DEMO_TAG) \
			--name $(DEMO_CLUSTER_NAME) ; \
	done
	@echo "  Loading holmesgpt-api..."
	@KIND_EXPERIMENTAL_PROVIDER=$(DEMO_KIND_PROVIDER) kind load docker-image \
		$(DEMO_REGISTRY)/holmesgpt-api:$(DEMO_TAG) \
		--name $(DEMO_CLUSTER_NAME)
	@echo "  All images loaded."

.PHONY: demo-deploy
demo-deploy: ## Deploy Kubernaut platform to Kind cluster (DEMO_TAG=<tag> DEMO_REGISTRY=<registry>)
	@echo "🚀 Deploying Kubernaut demo..."
	@echo "  Applying CRDs..."
	KUBECONFIG=$(DEMO_KUBECONFIG) kubectl apply -f config/crd/bases/
	@echo "  Setting image tags to $(DEMO_REGISTRY)/*:$(DEMO_TAG)..."
	@cd deploy/demo/overlays/kind && \
	for svc in $(DEMO_SERVICES) holmesgpt-api; do \
	    kustomize edit set image $(DEMO_REGISTRY)/$$svc:$(DEMO_TAG); \
	done
	KUBECONFIG=$(DEMO_KUBECONFIG) kubectl apply -k deploy/demo/overlays/kind/
	@echo "  Waiting for PostgreSQL..."
	KUBECONFIG=$(DEMO_KUBECONFIG) kubectl wait --for=condition=ready pod -l app=postgresql \
		-n kubernaut-system --timeout=120s
	@echo "  Applying migrations..."
	KUBECONFIG=$(DEMO_KUBECONFIG) ./deploy/demo/scripts/apply-migrations.sh
	@echo "  Generating AuthWebhook TLS certs..."
	KUBECONFIG=$(DEMO_KUBECONFIG) ./deploy/demo/scripts/generate-webhook-certs.sh
	@echo "  Waiting for all pods to be ready..."
	KUBECONFIG=$(DEMO_KUBECONFIG) kubectl wait --for=condition=ready pod --all \
		-n kubernaut-system --timeout=300s || true
	@echo "  Seeding workflow catalog..."
	KUBECONFIG=$(DEMO_KUBECONFIG) ./deploy/demo/scripts/seed-workflows.sh
	@echo ""
	@echo "✅ Kubernaut demo deployed!"
	@echo "   Gateway:      http://localhost:30080"
	@echo "   DataStorage:  http://localhost:30081"
	@echo "   Prometheus:   http://localhost:9190"
	@echo "   AlertManager: http://localhost:9193"
	@echo "   Grafana:      http://localhost:3000  (admin/kubernaut)"

.PHONY: demo-setup
demo-setup: demo-build-images demo-create-cluster demo-load-images demo-deploy ## Full demo setup (build, cluster, deploy)
	@echo ""
	@echo "🎉 Demo environment ready!"
	@echo "   Apply LLM credentials:  kubectl --kubeconfig $(DEMO_KUBECONFIG) apply -f deploy/demo/credentials/<your-provider>.yaml"
	@echo "   Trigger OOMKill demo:    make demo-trigger-oomkill"
	@echo "   Trigger high-usage demo: make demo-trigger-high-usage"
	@echo "   Reset workloads:         make demo-reset-workloads"

.PHONY: demo-trigger-oomkill
demo-trigger-oomkill: ## Deploy memory-eater OOMKill workload (triggers remediation pipeline)
	@echo "💥 Deploying memory-eater (OOMKill variant) to demo-workloads..."
	@KUBECONFIG=$(DEMO_KUBECONFIG) kubectl apply -f deploy/demo/base/workloads/namespace.yaml
	@KUBECONFIG=$(DEMO_KUBECONFIG) kubectl apply -f deploy/demo/base/workloads/memory-eater-oomkill.yaml
	@echo "  Deployed. Watch the pipeline:"
	@echo "    kubectl --kubeconfig $(DEMO_KUBECONFIG) get pods -n demo-workloads -w"
	@echo "    kubectl --kubeconfig $(DEMO_KUBECONFIG) get remediationrequests -A -w"

.PHONY: demo-trigger-high-usage
demo-trigger-high-usage: ## Deploy memory-eater high-usage workload (Prometheus alert path)
	@echo "📈 Deploying memory-eater (high-usage variant) to demo-workloads..."
	@KUBECONFIG=$(DEMO_KUBECONFIG) kubectl apply -f deploy/demo/base/workloads/namespace.yaml
	@KUBECONFIG=$(DEMO_KUBECONFIG) kubectl apply -f deploy/demo/base/workloads/memory-eater-high-usage.yaml
	@echo "  Deployed. Watch Prometheus alerts:"
	@echo "    curl -s http://localhost:9190/api/v1/alerts | jq '.data.alerts[] | {alertname: .labels.alertname, state}'"
	@echo "    kubectl --kubeconfig $(DEMO_KUBECONFIG) get remediationrequests -A -w"

.PHONY: demo-reset-workloads
demo-reset-workloads: ## Remove demo workloads (clean slate for re-triggering)
	@echo "🧹 Removing demo workloads..."
	@KUBECONFIG=$(DEMO_KUBECONFIG) kubectl delete -f deploy/demo/base/workloads/memory-eater-oomkill.yaml --ignore-not-found
	@KUBECONFIG=$(DEMO_KUBECONFIG) kubectl delete -f deploy/demo/base/workloads/memory-eater-high-usage.yaml --ignore-not-found
	@KUBECONFIG=$(DEMO_KUBECONFIG) kubectl delete remediationrequests --all -n demo-workloads --ignore-not-found
	@KUBECONFIG=$(DEMO_KUBECONFIG) kubectl delete signalprocessings --all -n demo-workloads --ignore-not-found
	@KUBECONFIG=$(DEMO_KUBECONFIG) kubectl delete aianalyses --all -n demo-workloads --ignore-not-found
	@KUBECONFIG=$(DEMO_KUBECONFIG) kubectl delete workflowexecutions --all -n demo-workloads --ignore-not-found
	@KUBECONFIG=$(DEMO_KUBECONFIG) kubectl delete effectivenessassessments --all -n demo-workloads --ignore-not-found
	@echo "  Workloads removed. Ready to re-trigger."

.PHONY: demo-teardown
demo-teardown: ## Destroy demo Kind cluster
	@echo "🧹 Tearing down demo cluster '$(DEMO_CLUSTER_NAME)'..."
	KIND_EXPERIMENTAL_PROVIDER=$(DEMO_KIND_PROVIDER) kind delete cluster --name $(DEMO_CLUSTER_NAME)
	rm -f $(DEMO_KUBECONFIG)
	@echo "  Demo cluster removed."

.PHONY: demo-status
demo-status: ## Show demo cluster status
	@echo "📊 Demo cluster status:"
	@KUBECONFIG=$(DEMO_KUBECONFIG) kubectl get pods -n kubernaut-system -o wide 2>/dev/null || echo "  Cluster not running"
	@KUBECONFIG=$(DEMO_KUBECONFIG) kubectl get pods -n demo-workloads -o wide 2>/dev/null || true

##@ Image Build & Push

# Registry, tag, and architecture (override via env or CLI)
IMAGE_REGISTRY ?= quay.io/kubernaut-ai
IMAGE_TAG ?= latest
# Auto-detect native architecture (maps uname output to Go-style names)
IMAGE_ARCH ?= $(shell uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')

# All Go services with their Dockerfile mappings (reuses DEMO_* mappings above)
IMAGE_SERVICES := $(DEMO_SERVICES)

# _image_build_one builds a single service image (native arch, arch-suffixed tag).
# Usage: $(call _image_build_one,<service>,<dockerfile>)
define _image_build_one
	@echo "  Building $(1) [$(IMAGE_ARCH)]..."
	@$(CONTAINER_TOOL) build -t $(IMAGE_REGISTRY)/$(1):$(IMAGE_TAG)-$(IMAGE_ARCH) -f $(2) .

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
	$(foreach svc,$(IMAGE_SERVICES),$(call _image_build_one,$(svc),$(DEMO_DOCKERFILES_$(svc))))
	@echo "  Building holmesgpt-api [$(IMAGE_ARCH)]..."
	@$(CONTAINER_TOOL) build -t $(IMAGE_REGISTRY)/holmesgpt-api:$(IMAGE_TAG)-$(IMAGE_ARCH) -f holmesgpt-api/Dockerfile .
	@echo ""
	@echo "✅ All images built ($(IMAGE_REGISTRY):$(IMAGE_TAG)-$(IMAGE_ARCH))."
	@echo "   Push with: make image-push IMAGE_TAG=$(IMAGE_TAG)"

.PHONY: image-push
image-push: ## Push arch-suffixed images to registry
	@echo "📤 Pushing images to $(IMAGE_REGISTRY)..."
	@echo "   Tag: $(IMAGE_TAG)-$(IMAGE_ARCH)"
	@echo ""
	$(foreach svc,$(IMAGE_SERVICES),$(call _image_push_one,$(svc)))
	@echo "  Pushing $(IMAGE_REGISTRY)/holmesgpt-api:$(IMAGE_TAG)-$(IMAGE_ARCH)..."
	@$(CONTAINER_TOOL) push $(IMAGE_REGISTRY)/holmesgpt-api:$(IMAGE_TAG)-$(IMAGE_ARCH)
	@echo ""
	@echo "✅ All images pushed to $(IMAGE_REGISTRY) with tag $(IMAGE_TAG)-$(IMAGE_ARCH)."

.PHONY: image-manifest
image-manifest: ## Create and push multi-arch manifests (run after both arches are pushed)
	@echo "🔗 Creating multi-arch manifests..."
	@echo "   Registry: $(IMAGE_REGISTRY)"
	@echo "   Tag:      $(IMAGE_TAG)"
	@echo "   Arches:   amd64, arm64"
	@echo ""
	@for svc in $(IMAGE_SERVICES) holmesgpt-api; do \
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
