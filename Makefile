<<<<<<< HEAD
# Makefile for kubernaut

# Variables
APP_NAME=kubernaut
VERSION?=latest
REGISTRY?=quay.io/jordigilh
IMAGE_NAME=$(REGISTRY)/$(APP_NAME)
NAMESPACE=kubernaut

# Go variables
GOOS?=linux
GOARCH?=amd64
CGO_ENABLED?=0

# Build variables
BUILD_DATE=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT=$(shell git rev-parse HEAD)
GIT_TAG=$(shell git describe --tags --always --dirty)

# LDFLAGS
LDFLAGS=-ldflags "-X main.version=$(GIT_TAG) -X main.commit=$(GIT_COMMIT) -X main.date=$(BUILD_DATE)"

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development
.PHONY: deps
deps: ## Download dependencies
	go mod download
	go mod tidy


.PHONY: envsetup
envsetup: ## Install environment setup dependencies for testing
	@echo "Installing envsetup dependencies..."
	go install sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.20
	go install github.com/onsi/ginkgo/v2/ginkgo@latest
	@echo "Setting up local envtest binaries..."
	mkdir -p bin
	$(eval ENVTEST_PATH := $(shell setup-envtest use --bin-dir ./bin -p path))
	@echo "Kubernetes test binaries installed to: $(ENVTEST_PATH)"

##@ Microservices Build - Approved 10-Service Architecture
.PHONY: build-all-services
=======
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

GATEWAY_CLUSTER ?= kubernaut-gateway-test

.PHONY: test-gateway-setup
test-gateway-setup: ## Setup Kind cluster for Gateway integration tests
	@./scripts/test-gateway-setup.sh

.PHONY: test-gateway-teardown
test-gateway-teardown: ## Teardown Gateway test cluster
	@kind delete cluster --name $(GATEWAY_CLUSTER) 2>/dev/null || true
	@rm -f /tmp/test-gateway-token.txt

.PHONY: test-gateway
test-gateway: ## Run Gateway integration tests (setup cluster if needed)
	@if ! kind get clusters 2>/dev/null | grep -q "^$(GATEWAY_CLUSTER)$$"; then \
		$(MAKE) test-gateway-setup; \
	fi
	@export TEST_TOKEN=$$(cat /tmp/test-gateway-token.txt) && \
	kubectl config use-context kind-$(GATEWAY_CLUSTER) && \
	cd test/integration/gateway && ginkgo -v

##@ Development (continued)

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd:allowDangerousTypes=true webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: test-integration-remediation
test-integration-remediation: manifests generate fmt vet setup-envtest ## Run RemediationRequest controller integration tests
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./test/integration/remediation/... -v -ginkgo.v

.PHONY: test-integration
test-integration: manifests generate fmt vet setup-envtest ## Run all integration tests
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./test/integration/... -v

# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the Manager Docker image locally.
# CertManager is installed by default; skip with:
# - CERT_MANAGER_INSTALL_SKIP=true
KIND_CLUSTER ?= kubernaut-temp-test-e2e

.PHONY: setup-test-e2e
setup-test-e2e: ## Set up a Kind cluster for e2e tests if it does not exist
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	$(KIND) create cluster --name $(KIND_CLUSTER)

.PHONY: test-e2e
test-e2e: setup-test-e2e manifests generate fmt vet ## Run the e2e tests. Expected an isolated environment using Kind.
	KIND_CLUSTER=$(KIND_CLUSTER) go test ./test/e2e/ -v -ginkgo.v
	$(MAKE) cleanup-test-e2e

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Tear down the Kind cluster used for e2e tests
	@$(KIND) delete cluster --name $(KIND_CLUSTER)

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

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name kubernaut-temp-builder
	$(CONTAINER_TOOL) buildx use kubernaut-temp-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm kubernaut-temp-builder
	rm Dockerfile.cross

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
CONTROLLER_TOOLS_VERSION ?= v0.18.0
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

##@ Microservices Build - Approved 10-Service Architecture
>>>>>>> crd_implementation
build-all-services: build-gateway-service build-alert-service build-ai-analysis build-workflow-service build-executor-service build-storage-service build-intelligence-service build-monitor-service build-context-service build-notification-service ## Build all 10 approved microservices

.PHONY: build-microservices
build-microservices: build-all-services ## Build all microservices (alias for build-all-services)

.PHONY: build-gateway-service
build-gateway-service: ## Build gateway service (webhook functionality)
	@echo "🔨 Building gateway service..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o bin/gateway-service ./cmd/gateway-service

.PHONY: build-webhook-service
build-webhook-service: build-gateway-service ## Build webhook service (alias for gateway-service)
	@echo "🔗 Webhook service is now part of gateway-service"

.PHONY: build-alert-service
build-alert-service: ## Build alert processor service
	@echo "🧠 Building alert processor service..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o bin/alert-service ./cmd/alert-service

.PHONY: build-workflow-service
build-workflow-service: ## Build workflow orchestrator service
	@echo "🎯 Building workflow orchestrator service..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o bin/workflow-service ./cmd/workflow-service

.PHONY: build-executor-service
build-executor-service: ## Build kubernetes executor service
	@echo "⚡ Building kubernetes executor service..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o bin/executor-service ./cmd/executor-service

.PHONY: build-storage-service
build-storage-service: ## Build data storage service
	@echo "📊 Building data storage service..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o bin/storage-service ./cmd/storage-service

.PHONY: build-intelligence-service
build-intelligence-service: ## Build intelligence service
	@echo "🔍 Building intelligence service..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o bin/intelligence-service ./cmd/intelligence-service

.PHONY: build-monitor-service
build-monitor-service: ## Build effectiveness monitor service
	@echo "📈 Building effectiveness monitor service..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o bin/monitor-service ./cmd/monitor-service

.PHONY: build-context-service
build-context-service: ## Build context API service
	@echo "🌐 Building context API service..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o bin/context-service ./cmd/context-service

.PHONY: build-notification-service
build-notification-service: ## Build notification service
	@echo "📢 Building notification service..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o bin/notification-service ./cmd/notification-service

.PHONY: build-context-api-service
build-context-api-service: ## Build context API service (placeholder)
	@echo "🔨 Building context API service..."
	@echo "⚠️  Context API service extraction pending - using monolith for now"

.PHONY: build-ai-analysis
build-ai-analysis: ## Build AI service
	@echo "🤖 Building AI service..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o bin/ai-analysis ./cmd/ai-analysis

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

.PHONY: lint
lint: ## Run linters (Go only)
	@echo "Running Go linter..."
	golangci-lint run

.PHONY: lint-go
lint-go: ## Run Go linter only
	@echo "Running Go linter..."
	golangci-lint run


.PHONY: fmt
fmt: ## Format code (Go only)
	@echo "Formatting Go code..."
	go fmt ./...

.PHONY: fmt-go
fmt-go: ## Format Go code only
	@echo "Formatting Go code..."
	go fmt ./...


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

##@ Container
.PHONY: docker-build
docker-build: ## Build monolithic container image
	docker build -t $(IMAGE_NAME):$(VERSION) .
	docker tag $(IMAGE_NAME):$(VERSION) $(IMAGE_NAME):latest

##@ Microservices Container Build
.PHONY: docker-build-microservices
docker-build-microservices: docker-build-gateway-service docker-build-ai-analysis ## Build all microservice container images

.PHONY: docker-build-gateway-service
docker-build-gateway-service: ## Build gateway service container image
	@echo "🐳 Building gateway service container..."
	docker build -f docker/gateway-service.Dockerfile -t $(REGISTRY)/kubernaut-gateway-service:$(VERSION) .
	docker tag $(REGISTRY)/kubernaut-gateway-service:$(VERSION) $(REGISTRY)/kubernaut-gateway-service:latest

.PHONY: docker-build-webhook-service
docker-build-webhook-service: docker-build-gateway-service ## Build webhook service container image (alias for gateway-service)
	@echo "🔗 Webhook service is now part of gateway-service"

.PHONY: docker-build-ai-analysis
docker-build-ai-analysis: ## Build AI service container image
	@echo "🤖 Building AI service container..."
	docker build -f docker/ai-service.Dockerfile -t $(REGISTRY)/kubernaut-ai-service:$(VERSION) .
	docker tag $(REGISTRY)/kubernaut-ai-service:$(VERSION) $(REGISTRY)/kubernaut-ai-service:latest

.PHONY: docker-push-microservices
docker-push-microservices: docker-push-webhook-service docker-push-ai-service ## Push all microservice container images

.PHONY: docker-push-webhook-service
docker-push-webhook-service: ## Push webhook service container image
	@echo "📤 Pushing webhook service container..."
	docker push $(REGISTRY)/kubernaut-webhook-service:$(VERSION)
	docker push $(REGISTRY)/kubernaut-webhook-service:latest

.PHONY: docker-push-ai-service
docker-push-ai-service: ## Push AI service container image
	@echo "🤖 Pushing AI service container..."
	docker push $(REGISTRY)/kubernaut-ai-service:$(VERSION)
	docker push $(REGISTRY)/kubernaut-ai-service:latest

.PHONY: docker-push
docker-push: ## Push container image
	docker push $(IMAGE_NAME):$(VERSION)
	docker push $(IMAGE_NAME):latest

.PHONY: docker-run
docker-run: ## Run container locally
	docker run --rm -p 8080:8080 -p 9090:9090 $(IMAGE_NAME):$(VERSION)

##@ Kubernetes
.PHONY: k8s-namespace
k8s-namespace: ## Create namespace
<<<<<<< HEAD
	kubectl create namespace $(NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -

.PHONY: k8s-deploy
k8s-deploy: ## Deploy to Kubernetes
	kubectl apply -k deploy/

.PHONY: k8s-delete
k8s-delete: ## Delete from Kubernetes
	kubectl delete -k deploy/

.PHONY: k8s-logs
k8s-logs: ## View application logs
	kubectl logs -f deployment/$(APP_NAME) -n $(NAMESPACE)

.PHONY: k8s-logs-localai
k8s-logs-localai: ## View LocalAI logs
	kubectl logs -f deployment/localai -n $(NAMESPACE)

.PHONY: k8s-status
k8s-status: ## Check deployment status
	kubectl get pods,svc,deploy -n $(NAMESPACE)

.PHONY: k8s-port-forward
k8s-port-forward: ## Port forward to local machine
	kubectl port-forward svc/$(APP_NAME)-service 8080:8080 -n $(NAMESPACE)

##@ LocalAI
.PHONY: download-model
download-model: ## Download Granite model
	./scripts/download-model.sh

.PHONY: localai-test
localai-test: ## Test LocalAI connection
	curl -X POST http://localhost:8081/v1/chat/completions \
		-H "Content-Type: application/json" \
		-d '{"model":"granite-3.0-8b-instruct","messages":[{"role":"user","content":"Hello"}]}'

##@ Ollama Testing
.PHONY: ollama-start
ollama-start: ## Start Ollama with Granite model
	@echo "Starting Ollama server..."
	ollama serve &
	@echo "Waiting for Ollama to start..."
	sleep 5
	@echo "Pulling Granite model..."
	ollama pull hf://ggml-org/gpt-oss-20b-GGUF
	@echo "Ollama ready with Granite model"

.PHONY: ollama-stop
ollama-stop: ## Stop Ollama server
	@echo "Stopping Ollama server..."
	pkill ollama || true

.PHONY: ollama-test
ollama-test: ## Test Ollama connection and model
	@echo "Testing Ollama connectivity..."
	curl -s http://localhost:11434/api/tags
	@echo "Testing Granite model..."
	curl -s -X POST http://192.168.1.169:8080/v1/completions -d '{"model":"hf://ggml-org/gpt-oss-20b-GGUF","prompt":"Hello","max_tokens":10}'

##@ Integration Testing (Hybrid Strategy)
# 🎯 STRATEGY: Kind for CI/CD and local testing, OCP for E2E tests

.PHONY: test-integration
test-integration: test-integration-kind ## Run integration tests (default: Kind cluster with real components)

.PHONY: test-integration-kind
test-integration-kind: envsetup ## Run integration tests with Kind cluster + real PostgreSQL + local LLM
	@echo "🏗️ Running integration tests with Kind cluster (Hybrid Strategy)..."
	@echo "  ├── Kubernetes: Real Kind cluster"
	@echo "  ├── Database: Real PostgreSQL + Vector DB (containerized)"
	@echo "  ├── LLM: Local model at 192.168.1.169:8080"
	@echo "  └── Purpose: Local development and testing"
	@echo ""
	@echo "Starting containerized services..."
	make integration-services-start
	@echo "Setting up Kind cluster..."
	./scripts/setup-kind-cluster.sh
	@echo "Running integration tests..."
	$(eval KUBEBUILDER_ASSETS := $(shell pwd)/$(shell setup-envtest use --bin-dir ./bin -p path))
	KUBECONFIG=$$(kind get kubeconfig --name=kubernaut-test) \
	KUBEBUILDER_ASSETS=$(KUBEBUILDER_ASSETS) \
	LLM_ENDPOINT=$(or $(LLM_ENDPOINT),http://192.168.1.169:8080) \
	LLM_MODEL=$(or $(LLM_MODEL),hf://ggml-org/gpt-oss-20b-GGUF) \
	LLM_PROVIDER=$(or $(LLM_PROVIDER),ollama) \
	USE_FAKE_K8S_CLIENT=false \
	go test -v -tags=integration ./test/integration/... -timeout=90m
	@echo "Cleaning up..."
	./scripts/cleanup-kind-cluster.sh
	make integration-services-stop

.PHONY: test-integration-kind-ci
test-integration-kind-ci: envsetup ## Run integration tests with Kind cluster for CI/CD (mocked LLM)
	@echo "🤖 Running CI integration tests with Kind cluster..."
	@echo "  ├── Kubernetes: Real Kind cluster"
	@echo "  ├── Database: Real PostgreSQL + Vector DB (containerized)"
	@echo "  ├── LLM: Mocked (for CI/CD reliability)"
	@echo "  └── Purpose: CI/CD pipeline testing"
	@echo ""
	@echo "Starting containerized services..."
	make integration-services-start
	@echo "Setting up Kind cluster..."
	./scripts/setup-kind-cluster.sh
	@echo "Running CI integration tests with mocked LLM..."
	$(eval KUBEBUILDER_ASSETS := $(shell pwd)/$(shell setup-envtest use --bin-dir ./bin -p path))
	KUBECONFIG=$$(kind get kubeconfig --name=kubernaut-test) \
	KUBEBUILDER_ASSETS=$(KUBEBUILDER_ASSETS) \
	USE_MOCK_LLM=true \
	CI=true \
	USE_FAKE_K8S_CLIENT=false \
	go test -v -tags=integration ./test/integration/... -timeout=60m
	@echo "Cleaning up..."
	./scripts/cleanup-kind-cluster.sh
	make integration-services-stop

.PHONY: test-integration-local
test-integration-local: ## Run integration tests with Docker Compose
	@echo "Starting integration test environment..."
	docker-compose -f docker-compose.integration.yml up --build --abort-on-container-exit
	docker-compose -f docker-compose.integration.yml down

.PHONY: validate-integration
validate-integration: ## Validate prerequisites for integration testing
	@echo "Validating integration test prerequisites..."
	./scripts/validate-integration.sh

##@ Integration Infrastructure
.PHONY: integration-infrastructure-setup
integration-infrastructure-setup: ## Setup integration test infrastructure (PostgreSQL, Vector DB, Redis)
	@echo "Setting up integration test infrastructure..."
	./scripts/setup-integration-infrastructure.sh setup

.PHONY: integration-infrastructure-status
integration-infrastructure-status: ## Show integration infrastructure status
	@echo "Checking integration infrastructure status..."
	./scripts/setup-integration-infrastructure.sh status

.PHONY: integration-infrastructure-stop
integration-infrastructure-stop: ## Stop integration infrastructure services
	@echo "Stopping integration infrastructure services..."
	./scripts/setup-integration-infrastructure.sh stop

.PHONY: integration-infrastructure-restart
integration-infrastructure-restart: ## Restart integration infrastructure services
	@echo "Restarting integration infrastructure services..."
	./scripts/setup-integration-infrastructure.sh restart

.PHONY: test-integration-quick
test-integration-quick: envsetup integration-infrastructure-setup ## Run integration tests (skip slow tests)
	@echo "Running quick integration tests with updated LLM endpoint..."
	@echo "Using local envtest binaries and LLM at 192.168.1.169:8080..."
	$(eval KUBEBUILDER_ASSETS := $(shell pwd)/$(shell setup-envtest use --bin-dir ./bin -p path))
	KUBEBUILDER_ASSETS=$(KUBEBUILDER_ASSETS) SKIP_SLOW_TESTS=true LLM_ENDPOINT=http://192.168.1.169:8080 LLM_MODEL=hf://ggml-org/gpt-oss-20b-GGUF LLM_PROVIDER=ramalama go test -v -tags=integration ./test/integration/... -timeout=30m

.PHONY: test-integration-ramalama
test-integration-ramalama: envsetup ## Run integration tests with ramalama
	@echo "Running integration tests with ramalama at 192.168.1.169:8080..."
	@echo "Using local envtest binaries..."
	$(eval KUBEBUILDER_ASSETS := $(shell pwd)/$(shell setup-envtest use --bin-dir ./bin -p path))
	KUBEBUILDER_ASSETS=$(KUBEBUILDER_ASSETS) LLM_ENDPOINT=http://192.168.1.169:8080 LLM_MODEL=ggml-org/gpt-oss-20b-GGUF LLM_PROVIDER=ollama go test -v -tags=integration ./test/integration/... -timeout=30m

##@ Legacy Integration Testing (Deprecated - Use Kind targets above)

.PHONY: test-integration-fake-k8s
test-integration-fake-k8s: envsetup ## [LEGACY] Run integration tests with fake Kubernetes clients (use test-integration-kind-ci instead)
	@echo "⚠️  LEGACY: Running integration tests with fake Kubernetes clients..."
	@echo "💡 RECOMMENDED: Use 'make test-integration-kind-ci' for CI/CD instead"
	@echo "Using local envtest binaries for fallback..."
	$(eval KUBEBUILDER_ASSETS := $(shell pwd)/$(shell setup-envtest use --bin-dir ./bin -p path))
	KUBEBUILDER_ASSETS=$(KUBEBUILDER_ASSETS) USE_FAKE_K8S_CLIENT=true LLM_ENDPOINT=$(or $(LLM_ENDPOINT),http://192.168.1.169:8080) LLM_MODEL=$(or $(LLM_MODEL),ggml-org/gpt-oss-20b-GGUF) LLM_PROVIDER=$(or $(LLM_PROVIDER),ramalama) go test -v -tags=integration ./test/integration/... -timeout=30m

.PHONY: test-integration-ollama
test-integration-ollama: envsetup ## [LEGACY] Run integration tests with Ollama at localhost:11434 (use test-integration-kind instead)
	@echo "⚠️  LEGACY: Running integration tests with Ollama at localhost:11434..."
	@echo "💡 RECOMMENDED: Use 'make test-integration-kind' for local testing instead"
	@echo "Using local envtest binaries..."
	$(eval KUBEBUILDER_ASSETS := $(shell pwd)/$(shell setup-envtest use --bin-dir ./bin -p path))
	KUBEBUILDER_ASSETS=$(KUBEBUILDER_ASSETS) LLM_ENDPOINT=http://192.168.1.169:8080 LLM_MODEL=ggml-org/gpt-oss-20b-GGUF LLM_PROVIDER=ramalama go test -v -tags=integration ./test/integration/... -timeout=30m

##@ End-to-End Testing (Multi-Node OCP Strategy)
# 🎯 STRATEGY: Use OpenShift Container Platform for production-like E2E testing

.PHONY: test-e2e
test-e2e: test-e2e-ocp ## Run e2e tests (default: OpenShift Container Platform)

.PHONY: test-e2e-ocp
test-e2e-ocp: ## Run e2e tests with OpenShift Container Platform (production-like)
	@echo "🏢 Running E2E tests with OpenShift Container Platform..."
	@echo "  ├── Platform: OpenShift 4.18+ multi-node cluster"
	@echo "  ├── Testing: Production-like scenarios"
	@echo "  ├── Chaos: Multi-node failure scenarios"
	@echo "  └── Purpose: Production validation"
	@echo ""
	@echo "Setting up OCP cluster environment..."
	cd docs/development/e2e-testing && ./setup-complete-e2e-environment.sh
	@echo "Running comprehensive E2E tests..."
	go test -v -tags=e2e ./test/e2e/... -timeout=240m
	@echo "E2E tests completed"

.PHONY: test-e2e-kind
test-e2e-kind: ## [ALTERNATIVE] Run e2e tests with KinD cluster (limited scenarios)
	@echo "⚠️  ALTERNATIVE: Running E2E tests with KinD cluster..."
	@echo "💡 NOTE: Limited to single/dual-node scenarios. Use test-e2e-ocp for full E2E testing"
	@echo "Setting up KinD cluster for e2e tests..."
	./scripts/setup-kind-cluster.sh
	@echo "Running e2e tests with KinD..."
	KUBECONFIG=$$(kind get kubeconfig --name=kubernaut-test) USE_KIND=true go test -v -tags=e2e ./test/e2e/... -run TestKindClusterOperations -timeout=90m
	@echo "Cleaning up KinD cluster..."
	./scripts/cleanup-kind-cluster.sh

.PHONY: test-e2e-monitoring
test-e2e-monitoring: ## Run e2e tests with full monitoring stack
	@echo "Setting up complete monitoring stack..."
	./scripts/setup-kind-cluster.sh
	@echo "Running complete monitoring flow tests..."
	KUBECONFIG=~/.kube/config USE_KIND=true go test -v -tags=e2e ./test/e2e/... -run TestCompleteMonitoringFlow -timeout=120m
	@echo "Cleaning up test environment..."
	./scripts/cleanup-kind-cluster.sh

##@ E2E Infrastructure
.PHONY: setup-e2e-scripts
setup-e2e-scripts: ## Make all E2E testing scripts executable
	@echo "Making E2E testing scripts executable..."
	cd docs/development/e2e-testing && chmod +x *.sh
	@echo "E2E scripts are now executable"

.PHONY: setup-e2e-environment
setup-e2e-environment: setup-e2e-scripts ## Setup complete E2E testing environment (OCP + Kubernaut + AI + Chaos)
	@echo "Setting up complete E2E testing environment..."
	cd docs/development/e2e-testing && ./setup-complete-e2e-environment.sh

.PHONY: validate-e2e-environment
validate-e2e-environment: ## Validate complete E2E testing environment
	@echo "Validating E2E testing environment..."
	cd docs/development/e2e-testing && ./validate-complete-e2e-environment.sh --detailed

.PHONY: cleanup-e2e-environment
cleanup-e2e-environment: ## Cleanup complete E2E testing environment
	@echo "Cleaning up E2E testing environment..."
	cd docs/development/e2e-testing && ./cleanup-e2e-environment.sh

.PHONY: test-e2e-use-cases
test-e2e-use-cases: ## Run all Top 10 E2E use case tests
	@echo "Running Top 10 E2E use case tests..."
	cd docs/development/e2e-testing && ./run-e2e-tests.sh use-cases

.PHONY: test-e2e-chaos
test-e2e-chaos: ## Run chaos engineering E2E tests
	@echo "Running chaos engineering E2E tests..."
	cd docs/development/e2e-testing && ./run-e2e-tests.sh chaos

.PHONY: test-e2e-stress
test-e2e-stress: ## Run AI model stress E2E tests
	@echo "Running AI model stress E2E tests..."
	cd docs/development/e2e-testing && ./run-e2e-tests.sh stress

.PHONY: test-e2e-complete
test-e2e-complete: ## Run complete E2E test suite (all use cases, chaos, stress)
	@echo "Running complete E2E test suite..."
	cd docs/development/e2e-testing && ./run-e2e-tests.sh all

##@ E2E Infrastructure (Root User - RHEL 9.7)
.PHONY: setup-e2e-root
setup-e2e-root: ## Setup complete E2E testing environment as root on RHEL 9.7
	@echo "Setting up complete E2E testing environment as root..."
	@echo "NOTE: This requires root privileges and RHEL 9.7"
	cd docs/development/e2e-testing && sudo ./setup-complete-e2e-environment-root.sh

.PHONY: validate-e2e-root
validate-e2e-root: ## Validate E2E testing environment for root deployment
	@echo "Validating E2E testing environment for root..."
	cd docs/development/e2e-testing && sudo ./validate-baremetal-setup-root.sh

.PHONY: cleanup-e2e-root
cleanup-e2e-root: ## Cleanup E2E testing environment (root deployment)
	@echo "Cleaning up E2E testing environment (root)..."
	cd docs/development/e2e-testing && sudo ./cleanup-e2e-environment-root.sh

.PHONY: test-e2e-root
test-e2e-root: ## Run E2E tests on root deployment
	@echo "Running E2E tests on root deployment..."
	cd docs/development/e2e-testing && sudo ./run-e2e-tests-root.sh basic

.PHONY: deploy-cluster-root
deploy-cluster-root: ## Deploy only OpenShift cluster as root (no Kubernaut stack)
	@echo "Deploying OpenShift cluster as root..."
	cd docs/development/e2e-testing && sudo ./deploy-kcli-cluster-root.sh

##@ E2E Infrastructure (Remote Root - helios08)
# Remote host configuration
REMOTE_HOST=helios08
REMOTE_USER=root
REMOTE_PATH=/root/kubernaut-e2e
E2E_DIR=docs/development/e2e-testing

.PHONY: configure-e2e-remote
configure-e2e-remote: ## Configure and validate remote host connection (helios08)
	@echo "Configuring remote host connection for E2E deployment..."
	cd $(E2E_DIR) && chmod +x configure-remote-host.sh
	cd $(E2E_DIR) && ./configure-remote-host.sh $(REMOTE_HOST) $(REMOTE_USER)

##@ E2E Infrastructure (Hybrid Architecture)
# Hybrid deployment: Remote cluster + Local AI/Kubernaut/Tests
.PHONY: deploy-cluster-remote-only
deploy-cluster-remote-only: ## Deploy ONLY OpenShift cluster on remote host for hybrid setup
	@echo "Deploying OpenShift cluster only on remote host: $(REMOTE_HOST)"
	@echo "Architecture: Hybrid (cluster remote, AI+tests local)"
	cd $(E2E_DIR) && chmod +x deploy-cluster-only-remote.sh
	cd $(E2E_DIR) && ./deploy-cluster-only-remote.sh kubernaut-e2e kcli-baremetal-params-root.yml

.PHONY: setup-local-hybrid
setup-local-hybrid: ## Setup local Kubernaut to connect to remote cluster (hybrid)
	@echo "Setting up local Kubernaut for hybrid architecture..."
	@echo "Remote cluster: $(REMOTE_HOST), Local AI: 192.168.1.169:8080"
	cd $(E2E_DIR) && chmod +x setup-local-kubernaut-remote-cluster.sh
	cd $(E2E_DIR) && ./setup-local-kubernaut-remote-cluster.sh $(REMOTE_HOST) $(REMOTE_USER)

.PHONY: validate-hybrid-topology
validate-hybrid-topology: ## Validate hybrid network topology and connections
	@echo "Validating hybrid architecture network topology..."
	cd $(E2E_DIR) && test -f test-config-hybrid/validate-network-topology.sh && ./test-config-hybrid/validate-network-topology.sh || echo "Setup hybrid environment first"

.PHONY: test-e2e-hybrid
test-e2e-hybrid: ## Run E2E tests in hybrid architecture (local tests, remote cluster)
	@echo "Running hybrid E2E tests..."
	@echo "Tests run locally, managing remote cluster with local AI model"
	cd $(E2E_DIR) && test -f start-hybrid-kubernaut.sh && ./start-hybrid-kubernaut.sh || echo "Setup hybrid environment first"

.PHONY: status-hybrid
status-hybrid: ## Check status of hybrid deployment (remote cluster + local components)
	@echo "Checking hybrid deployment status..."
	@echo "=== Remote Cluster Status ==="
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "cd $(REMOTE_PATH) && export KUBECONFIG=./kubeconfig && oc get nodes 2>/dev/null || echo 'Cluster not accessible'"
	@echo "=== Local AI Model Status ==="
	curl -s http://192.168.1.169:8080/health >/dev/null 2>&1 && echo "✓ AI model running on 192.168.1.169:8080" || echo "✗ AI model not running on 192.168.1.169:8080"
	@echo "=== Local Components ==="
	pgrep kubernaut >/dev/null 2>&1 && echo "✓ Kubernaut running locally" || echo "✗ Kubernaut not running locally"
	pgrep postgres >/dev/null 2>&1 && echo "✓ PostgreSQL running locally" || echo "✗ PostgreSQL not running locally"

.PHONY: cleanup-hybrid
cleanup-hybrid: ## Cleanup hybrid deployment (remote cluster + local components)
	@echo "Cleaning up hybrid deployment..."
	@echo "=== Stopping local components ==="
	-pkill kubernaut 2>/dev/null || true
	-rm -rf local-config-remote/ test-config-hybrid/ local-monitoring/ 2>/dev/null || true
	@echo "=== Cleaning up remote cluster ==="
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "cd $(REMOTE_PATH) && ./cleanup-e2e-environment-root.sh --force" 2>/dev/null || echo "Remote cleanup skipped"

.PHONY: ssh-remote-cluster
ssh-remote-cluster: ## SSH to remote cluster host for manual management
	@echo "Connecting to remote cluster host: $(REMOTE_HOST)"
	@echo "Cluster path: $(REMOTE_PATH)"
	ssh $(REMOTE_USER)@$(REMOTE_HOST)

.PHONY: setup-e2e-remote
setup-e2e-remote: ## Setup complete E2E testing environment on remote host (helios08)
	@echo "Setting up complete E2E testing environment on remote host: $(REMOTE_HOST)"
	@echo "Testing SSH connection to $(REMOTE_HOST)..."
	@ssh -o ConnectTimeout=10 -o BatchMode=yes $(REMOTE_USER)@$(REMOTE_HOST) "echo 'SSH connection verified'" || (echo "ERROR: Cannot connect to $(REMOTE_HOST). Run 'make configure-e2e-remote' first." && exit 1)
	@echo "Copying E2E scripts to $(REMOTE_HOST)..."
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "mkdir -p $(REMOTE_PATH)"
	scp -r $(E2E_DIR)/* $(REMOTE_USER)@$(REMOTE_HOST):$(REMOTE_PATH)/
	@echo "Making scripts executable on remote host..."
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "cd $(REMOTE_PATH) && chmod +x *.sh"
	@echo "Running complete E2E environment setup on $(REMOTE_HOST)..."
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "cd $(REMOTE_PATH) && ./setup-complete-e2e-environment-root.sh"

.PHONY: validate-e2e-remote
validate-e2e-remote: ## Validate E2E testing environment on remote host (helios08)
	@echo "Validating E2E testing environment on remote host: $(REMOTE_HOST)"
	@echo "Copying validation scripts to $(REMOTE_HOST)..."
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "mkdir -p $(REMOTE_PATH)"
	scp $(E2E_DIR)/validate-baremetal-setup-root.sh $(E2E_DIR)/kcli-baremetal-params-root.yml $(REMOTE_USER)@$(REMOTE_HOST):$(REMOTE_PATH)/
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "cd $(REMOTE_PATH) && chmod +x validate-baremetal-setup-root.sh"
	@echo "Running validation on $(REMOTE_HOST)..."
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "cd $(REMOTE_PATH) && ./validate-baremetal-setup-root.sh kcli-baremetal-params-root.yml"

.PHONY: cleanup-e2e-remote
cleanup-e2e-remote: ## Cleanup E2E testing environment on remote host (helios08)
	@echo "Cleaning up E2E testing environment on remote host: $(REMOTE_HOST)"
	@echo "Testing SSH connection to $(REMOTE_HOST)..."
	@ssh -o ConnectTimeout=10 -o BatchMode=yes $(REMOTE_USER)@$(REMOTE_HOST) "echo 'SSH connection verified'" || (echo "ERROR: Cannot connect to $(REMOTE_HOST). Check SSH configuration." && exit 1)
	@echo "Copying cleanup script to $(REMOTE_HOST)..."
	scp $(E2E_DIR)/cleanup-e2e-environment-root.sh $(REMOTE_USER)@$(REMOTE_HOST):$(REMOTE_PATH)/
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "cd $(REMOTE_PATH) && chmod +x cleanup-e2e-environment-root.sh"
	@echo "Running cleanup on $(REMOTE_HOST)..."
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "cd $(REMOTE_PATH) && ./cleanup-e2e-environment-root.sh --force"
	@echo "Removing E2E directory from remote host..."
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "rm -rf $(REMOTE_PATH)"

.PHONY: test-e2e-remote
test-e2e-remote: ## Run E2E tests on remote host (helios08)
	@echo "Running E2E tests on remote host: $(REMOTE_HOST)"
	@echo "Copying test scripts to $(REMOTE_HOST)..."
	scp $(E2E_DIR)/run-e2e-tests-root.sh $(REMOTE_USER)@$(REMOTE_HOST):$(REMOTE_PATH)/
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "cd $(REMOTE_PATH) && chmod +x run-e2e-tests-root.sh"
	@echo "Running basic tests on $(REMOTE_HOST)..."
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "cd $(REMOTE_PATH) && ./run-e2e-tests-root.sh basic"

.PHONY: deploy-cluster-remote
deploy-cluster-remote: ## Deploy only OpenShift cluster on remote host (helios08)
	@echo "Deploying OpenShift cluster on remote host: $(REMOTE_HOST)"
	@echo "Copying cluster deployment scripts to $(REMOTE_HOST)..."
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "mkdir -p $(REMOTE_PATH)"
	scp $(E2E_DIR)/deploy-kcli-cluster-root.sh $(E2E_DIR)/kcli-baremetal-params-root.yml $(E2E_DIR)/validate-baremetal-setup-root.sh $(E2E_DIR)/setup-storage.sh $(REMOTE_USER)@$(REMOTE_HOST):$(REMOTE_PATH)/
	scp -r $(E2E_DIR)/storage $(REMOTE_USER)@$(REMOTE_HOST):$(REMOTE_PATH)/
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "cd $(REMOTE_PATH) && chmod +x *.sh"
	@echo "Running cluster deployment on $(REMOTE_HOST)..."
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "cd $(REMOTE_PATH) && ./deploy-kcli-cluster-root.sh kubernaut-e2e kcli-baremetal-params-root.yml"

.PHONY: status-e2e-remote
status-e2e-remote: ## Check status of E2E environment on remote host (helios08)
	@echo "Checking E2E environment status on remote host: $(REMOTE_HOST)"
	@echo "=== Remote Host Info ==="
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "hostname && uname -a"
	@echo "=== KCLI Clusters ==="
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "command -v kcli >/dev/null && kcli list cluster || echo 'KCLI not installed'"
	@echo "=== libvirt VMs ==="
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "command -v virsh >/dev/null && virsh list --all || echo 'libvirt not available'"
	@echo "=== Resource Usage ==="
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "free -h && df -h /var/lib/libvirt/images 2>/dev/null || df -h /"
	@echo "=== OpenShift Cluster Status ==="
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "test -f $(REMOTE_PATH)/kubeconfig && export KUBECONFIG=$(REMOTE_PATH)/kubeconfig && oc get nodes 2>/dev/null || echo 'No cluster access'"

.PHONY: logs-e2e-remote
logs-e2e-remote: ## View logs from remote E2E deployment (helios08)
	@echo "Viewing deployment logs from remote host: $(REMOTE_HOST)"
	@echo "=== Recent deployment logs ==="
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "find $(REMOTE_PATH) -name '*.log' -type f -mtime -1 -exec echo '=== {} ===' \; -exec tail -50 {} \; 2>/dev/null || echo 'No recent logs found'"
	@echo "=== KCLI deployment logs ==="
	ssh $(REMOTE_USER)@$(REMOTE_HOST) "find /root -name 'kcli-deploy-*.log' -type f -mtime -1 -exec echo '=== {} ===' \; -exec tail -100 {} \; 2>/dev/null || echo 'No KCLI logs found'"

.PHONY: ssh-e2e-remote
ssh-e2e-remote: ## SSH to remote host for manual management (helios08)
	@echo "Connecting to remote host: $(REMOTE_HOST)"
	@echo "Remote E2E directory: $(REMOTE_PATH)"
	ssh $(REMOTE_USER)@$(REMOTE_HOST)

.PHONY: build-test-image
build-test-image: build ## Build test container image for KinD
	kind load docker-image kubernaut:latest --name kubernaut-test || echo "KinD cluster not running"

.PHONY: setup-kind
setup-kind: ## Setup KinD cluster for testing
	./scripts/setup-kind-cluster.sh

.PHONY: cleanup-kind
cleanup-kind: ## Cleanup KinD cluster
	./scripts/cleanup-kind-cluster.sh

.PHONY: test-webhook
test-webhook: ## Test webhook endpoint
	curl -X POST http://192.168.1.169:8080/alerts \
		-H "Content-Type: application/json" \
		-H "Authorization: Bearer test-token" \
		-d @test/fixtures/sample-alert.json

.PHONY: test-health
test-health: ## Test health endpoints
	curl -f http://192.168.1.169:8080/health
	curl -f http://192.168.1.169:8080/ready

.PHONY: test-metrics
test-metrics: ## Test metrics endpoint
	curl -f http://localhost:9090/metrics

##@ Model Comparison
.PHONY: model-comparison-setup
model-comparison-setup: ## Setup ramallama and vllm infrastructure for model comparison
	@echo "Setting up model comparison infrastructure..."
	./scripts/setup_model_comparison.sh

.PHONY: model-comparison-test
model-comparison-test: ## Run model comparison tests (requires setup)
	@echo "Running model comparison tests..."
	./scripts/run_model_comparison.sh

.PHONY: model-comparison-stop
model-comparison-stop: ## Stop all model comparison servers
	@echo "Stopping model comparison infrastructure..."
	./scripts/stop_model_comparison.sh

.PHONY: model-comparison-clean
model-comparison-clean: model-comparison-stop ## Stop servers and clean up results
	@echo "Cleaning up model comparison results..."
	rm -rf model_comparison_results/
	rm -f logs/ramallama_*.log logs/vllm_*.log logs/ramallama_*.pid logs/vllm_*.pid
	@echo "Model comparison cleanup complete"

.PHONY: model-comparison-full
model-comparison-full: model-comparison-clean model-comparison-setup model-comparison-test ## Full model comparison workflow (setup, test, analyze)
	@echo "Full model comparison workflow completed!"

.PHONY: model-comparison-demo
model-comparison-demo: ## Demo model comparison using ollama (faster setup)
	@echo "Setting up model comparison demo with ollama..."
	./scripts/setup_model_comparison_ollama.sh
	@echo "Running model comparison demo tests..."
	go test ./test/integration/model_comparison -run "Ollama" -v
	@echo "Demo completed! Check model_comparison_report.md for results"

##@ Release
.PHONY: release
release: clean deps test build docker-build docker-push ## Build and release new version

.PHONY: tag
tag: ## Create git tag
	git tag -a v$(VERSION) -m "Release v$(VERSION)"
	git push origin v$(VERSION)

##@ Utilities
.PHONY: generate
generate: ## Generate code
	go generate ./...

.PHONY: mod-update
mod-update: ## Update dependencies
	go get -u ./...
	go mod tidy

.PHONY: security-scan
security-scan: ## Run security scan
	gosec ./...

##@ Integration Testing with Containerized Services
.PHONY: integration-services-start
integration-services-start: ## Start integration test services (PostgreSQL, Vector DB, Redis)
	@echo "Starting integration test services..."
	./scripts/run-integration-tests.sh start-services

.PHONY: integration-services-stop
integration-services-stop: ## Stop integration test services
	@echo "Stopping integration test services..."
	./scripts/run-integration-tests.sh stop-services

.PHONY: integration-services-status
integration-services-status: ## Show status of integration test services
	./scripts/run-integration-tests.sh status

.PHONY: integration-test
integration-test: ## Run integration tests (assumes services are running)
	@echo "Running integration tests..."
	./scripts/run-integration-tests.sh test

.PHONY: integration-test-with-services
integration-test-with-services: ## Run integration tests with automatic service management
	@echo "Running integration tests with automatic service management..."
	./scripts/run-integration-tests.sh test-with-services

.PHONY: integration-test-infrastructure
integration-test-infrastructure: ## Run infrastructure integration tests only
	@echo "Running infrastructure integration tests..."
	./scripts/run-integration-tests.sh test-infrastructure

.PHONY: integration-test-performance
integration-test-performance: ## Run performance integration tests only
	@echo "Running performance integration tests..."
	./scripts/run-integration-tests.sh test-performance

.PHONY: integration-test-vector
integration-test-vector: ## Run vector database integration tests only
	@echo "Running vector database integration tests..."
	./scripts/run-integration-tests.sh test-vector

.PHONY: integration-test-all
integration-test-all: ## Run all integration tests with full service lifecycle management
	@echo "Running all integration tests with service management..."
	./scripts/run-integration-tests.sh test-all

##@ Development Environment Management (Kind Cluster Primary)
.PHONY: bootstrap-dev cleanup-dev test-integration-dev bootstrap-dev-healthcheck
.PHONY: bootstrap-dev-compose cleanup-dev-compose  # Legacy docker-compose support

bootstrap-dev: bootstrap-dev-kind ## Bootstrap complete development environment with Kind cluster (PRIMARY METHOD)

bootstrap-external-deps: ## Bootstrap ONLY external dependencies (Kind cluster, PostgreSQL, Redis, Prometheus)
	@echo "🚀 Bootstrapping external dependencies..."
	@echo "📋 Components: Kind cluster, PostgreSQL, Redis, Prometheus, AlertManager"
	@echo "🎯 Strategy: Static infrastructure setup"
	@echo ""
	@./scripts/bootstrap-external-deps.sh || { \
		echo ""; \
		echo "❌ EXTERNAL DEPENDENCIES BOOTSTRAP FAILED"; \
		echo "🔧 Check prerequisites and try again"; \
		echo "💡 Use 'make build-and-deploy' after this succeeds"; \
		exit 1; \
	}

build-and-deploy: ## Build and deploy kubernaut components to Kind internal registry
	@echo "🔨 Building and deploying kubernaut components..."
	@echo "📋 Components: webhook-service, ai-service, holmesgpt-api"
	@echo "🎯 Strategy: Build images and deploy to Kind internal registry"
	@echo ""
	@echo "⚠️  Prerequisites: External dependencies must be running"
	@echo "   Run 'make bootstrap-external-deps' first if needed"
	@echo ""
	@./scripts/build-and-deploy.sh || { \
		echo ""; \
		echo "❌ BUILD AND DEPLOY FAILED"; \
		echo "🔧 Check that external dependencies are running"; \
		echo "💡 Run 'make bootstrap-external-deps' first"; \
		exit 1; \
	}

bootstrap-dev-kind: ## Bootstrap development environment using Kind cluster (RECOMMENDED)
	@echo "🚀 Bootstrapping Kind-based development environment..."
	@echo "📋 Requirements: kind, kubectl, docker/podman, go"
	@echo "🎯 Strategy: Split workflow - external deps first, then kubernaut components"
	@echo ""
	@echo "Phase 1: External dependencies..."
	@$(MAKE) bootstrap-external-deps || { \
		echo "❌ External dependencies failed"; \
		exit 1; \
	}
	@echo ""
	@echo "Phase 2: Kubernaut components..."
	@$(MAKE) build-and-deploy || { \
		echo "❌ Build and deploy failed"; \
		exit 1; \
	}
	@echo ""
	@echo "✅ Complete Kind-based development environment ready!"

bootstrap-dev-compose: ## Bootstrap development environment using docker-compose (DEPRECATED)
	@echo "⚠️  DEPRECATED: docker-compose bootstrap (use 'make bootstrap-dev-kind' instead)"
	@echo "🚀 Bootstrapping docker-compose development environment..."
	@echo "📋 Requirements: podman, podman-compose, go"
	@echo ""
	@./scripts/bootstrap-dev-environment.sh || { \
		echo ""; \
		echo "❌ COMPOSE BOOTSTRAP FAILED"; \
		echo "🔧 Ensure prerequisites are installed:"; \
		echo "   - podman (brew install podman)"; \
		echo "   - podman-compose (pip install podman-compose)"; \
		echo "   - podman machine running (podman machine start)"; \
		echo ""; \
		echo "💡 Recommended: Use 'make bootstrap-dev-kind' for better production parity"; \
		exit 1; \
	}

bootstrap-dev-healthcheck: ## Check health of all integration test dependencies
	@echo "🔍 Checking integration test dependencies health..."
	@./scripts/integration-health-check-simple.sh

cleanup-dev: cleanup-dev-kind ## Clean up development environment (Kind cluster primary)

cleanup-dev-kind: ## Clean up Kind-based development environment (RECOMMENDED)
	@echo "🧹 Cleaning up Kind development environment..."
	@./scripts/cleanup-kind-integration.sh

cleanup-dev-compose: ## Clean up docker-compose development environment (DEPRECATED)
	@echo "⚠️  DEPRECATED: docker-compose cleanup (use 'make cleanup-dev-kind' instead)"
	@echo "🧹 Cleaning up docker-compose development environment..."
	@./scripts/cleanup-dev-environment.sh

test-integration-dev: ## Run integration tests (assumes bootstrapped environment)
	@echo "🧪 Running integration tests..."
	@./scripts/run-tests.sh

test-ai-dev: ## Run AI integration tests only
	@./scripts/run-tests.sh ai

test-infrastructure-dev: ## Run infrastructure integration tests only
	@./scripts/run-tests.sh infrastructure

test-performance-dev: ## Run performance integration tests only
	@./scripts/run-tests.sh performance

test-quick-dev: ## Run quick integration tests only
	@./scripts/run-tests.sh quick

dev-setup: bootstrap-dev ## Alias for bootstrap-dev
	@echo "✅ Development environment ready!"
	@echo "Run 'make test-integration-dev' to start testing"

dev-teardown: cleanup-dev ## Alias for cleanup-dev
	@echo "✅ Development environment cleaned up!"

dev-status: ## Show status of development environment components
	@echo "🔍 Development Environment Status"
	@echo "================================="
	@echo ""
	@echo "📊 Service Status:"
	@echo "  LLM Service:        $$(curl -s http://192.168.1.169:8080/v1/models >/dev/null 2>&1 && echo '✅ Running' || echo '❌ Not running')"
	@echo "  PostgreSQL:         $$(PGPASSWORD=slm_password_dev psql -h localhost -p 5433 -U slm_user -d action_history -c 'SELECT 1;' >/dev/null 2>&1 && echo '✅ Running' || echo '❌ Not running')"
	@echo "  Vector DB:          $$(PGPASSWORD=vector_password_dev psql -h localhost -p 5434 -U vector_user -d vector_store -c 'SELECT 1;' >/dev/null 2>&1 && echo '✅ Running' || echo '❌ Not running')"
	@echo "  Redis Cache:        $$(echo 'PING' | redis-cli -h localhost -p 6380 -a integration_redis_password --no-auth-warning >/dev/null 2>&1 && echo '✅ Running' || echo '❌ Not running')"
	@echo "  Kind Cluster:       $$(kubectl get nodes >/dev/null 2>&1 && echo '✅ Running ('$$(kubectl get nodes --no-headers | wc -l | xargs)' nodes)' || echo '❌ Not running')"
	@echo ""
	@echo "🔧 Environment Config:"
	@echo "  Config File:        $$(test -f .env.development && echo '✅ .env.development exists' || echo '❌ .env.development missing')"
	@echo "  Built Binary:       $$(test -f bin/kubernaut && echo '✅ bin/kubernaut exists' || echo '❌ bin/kubernaut missing')"
	@echo ""
	@echo "💡 Commands:"
	@echo "  Bootstrap:          make bootstrap-dev"
	@echo "  Run Tests:          make test-integration-dev"
	@echo "  Cleanup:            make cleanup-dev"

##@ Kind Cluster Management
.PHONY: kind-deploy kind-undeploy kind-status kind-logs kind-port-forward

kind-deploy: ## Deploy kubernaut services to Kind cluster
	@echo "🚀 Deploying kubernaut services to Kind cluster..."
	kubectl apply -k deploy/integration/
	@echo "⏳ Waiting for deployments to be ready..."
	kubectl wait --for=condition=available --timeout=300s deployment --all -n kubernaut-integration
	@echo "✅ All services deployed successfully!"

kind-undeploy: ## Remove kubernaut services from Kind cluster
	@echo "🗑️  Removing kubernaut services from Kind cluster..."
	kubectl delete -k deploy/integration/ --ignore-not-found=true
	@echo "✅ Services removed successfully!"

kind-status: ## Show status of Kind cluster and services
	@echo "📊 Kind Cluster Status"
	@echo "====================="
	@echo ""
	@echo "🏗️ Cluster Info:"
	kubectl cluster-info --context kind-kubernaut-integration 2>/dev/null || echo "❌ Kind cluster not found"
	@echo ""
	@echo "🔧 Nodes:"
	kubectl get nodes -o wide 2>/dev/null || echo "❌ Cannot access cluster"
	@echo ""
	@echo "📦 Services in kubernaut-integration namespace:"
	kubectl get pods,svc -n kubernaut-integration 2>/dev/null || echo "❌ Namespace not found"
	@echo ""
	@echo "📈 Resource Usage:"
	kubectl top nodes 2>/dev/null || echo "⚠️  Metrics server not available"

kind-logs: ## Show logs from kubernaut services in Kind cluster
	@echo "📋 Kubernaut Service Logs"
	@echo "========================"
	@echo ""
	@echo "🔍 Webhook Service:"
	kubectl logs -l app=webhook-service -n kubernaut-integration --tail=50 --prefix=true 2>/dev/null || echo "❌ Webhook service not found"
	@echo ""
	@echo "🤖 AI Service:"
	kubectl logs -l app=ai-service -n kubernaut-integration --tail=50 --prefix=true 2>/dev/null || echo "❌ AI service not found"
	@echo ""
	@echo "🔬 HolmesGPT:"
	kubectl logs -l app=holmesgpt -n kubernaut-integration --tail=50 --prefix=true 2>/dev/null || echo "❌ HolmesGPT not found"

kind-port-forward: ## Set up port forwarding for Kind cluster services
	@echo "🔌 Setting up port forwarding for Kind cluster services..."
	@echo "📋 Access URLs will be:"
	@echo "  • Webhook Service: http://localhost:30800"
	@echo "  • Prometheus: http://localhost:30090"
	@echo "  • AlertManager: http://localhost:30093"
	@echo "  • PostgreSQL: localhost:30432"
	@echo ""
	@echo "🔗 Port forwarding is automatic via NodePort services"
	@echo "💡 Use 'make kind-status' to verify services are running"

dev-help: ## Show development environment help
	@echo "Kubernaut Development Environment (Kind Cluster Primary)"
	@echo "======================================================="
	@echo ""
	@echo "🚀 Quick Start (Kind - RECOMMENDED):"
	@echo "  1. Start your LLM model at 192.168.1.169:8080"
	@echo "  2. make bootstrap-dev-kind    # Setup Kind cluster + services"
	@echo "  3. make test-integration-dev  # Run tests"
	@echo "  4. make cleanup-dev-kind      # Clean up when done"
	@echo ""
	@echo "🔧 Environment Management:"
	@echo "  bootstrap-dev-kind      - Setup Kind cluster environment (RECOMMENDED)"
	@echo "  bootstrap-external-deps - Setup ONLY external dependencies (Kind, DB, monitoring)"
	@echo "  build-and-deploy        - Build and deploy kubernaut components to Kind internal registry"
	@echo "  bootstrap-dev-compose   - Setup docker-compose environment (DEPRECATED)"
	@echo "  bootstrap-dev-healthcheck - Check health of all dependencies"
	@echo "  cleanup-dev-kind        - Clean up Kind environment"
	@echo "  cleanup-dev-compose     - Clean up docker-compose environment"
	@echo "  dev-setup              - Alias for bootstrap-dev-kind"
	@echo "  dev-teardown           - Alias for cleanup-dev-kind"
	@echo "  dev-status             - Show status of all services"
	@echo ""
	@echo "🏗️ Kind Cluster Management:"
	@echo "  kind-deploy            - Deploy services to Kind cluster"
	@echo "  kind-undeploy          - Remove services from Kind cluster"
	@echo "  kind-status            - Show Kind cluster and service status"
	@echo "  kind-logs              - Show logs from kubernaut services"
	@echo "  kind-port-forward      - Setup port forwarding (info only)"
	@echo ""
	@echo "🧪 Testing:"
	@echo "  test-integration-dev   - Run all integration tests"
	@echo "  test-ai-dev           - Run AI integration tests"
	@echo "  test-infrastructure-dev - Run infrastructure tests"
	@echo "  test-performance-dev  - Run performance tests"
	@echo "  test-quick-dev        - Run quick tests only"
	@echo ""
	@echo "📋 Components Managed (Kind Cluster):"
	@echo "  ✓ Kind Kubernetes cluster (1 control-plane + 2 workers)"
	@echo "  ✓ PostgreSQL with pgvector extension"
	@echo "  ✓ Redis cache"
	@echo "  ✓ Prometheus + AlertManager monitoring"
	@echo "  ✓ Kubernaut webhook + AI services"
	@echo "  ✓ HolmesGPT integration"
	@echo "  ✗ LLM model (external at 192.168.1.169:8080)"
	@echo ""
	@echo "⚠️  Prerequisites:"
	@echo "  - LLM running at 192.168.1.169:8080 (ramalama/oss-gpt:20b)"
	@echo "  - kind, kubectl, docker/podman, go installed"
	@echo ""
	@echo "🔄 Migration from docker-compose:"
	@echo "  - Use 'make bootstrap-dev-compose' for legacy setup"
	@echo "  - Kind cluster provides better production parity"
	@echo "  - All services run as Kubernetes resources"

##@ HolmesGPT REST API
.PHONY: holmesgpt-api-init
holmesgpt-api-init: ## Initialize HolmesGPT submodule
	@echo "🔄 Initializing HolmesGPT submodule..."
	git submodule update --init --recursive dependencies/holmesgpt
	@echo "✅ HolmesGPT submodule initialized"

.PHONY: holmesgpt-api-update
holmesgpt-api-update: ## Update HolmesGPT submodule to latest
	@echo "🔄 Updating HolmesGPT submodule..."
	git submodule update --remote dependencies/holmesgpt
	@echo "✅ HolmesGPT submodule updated"

.PHONY: holmesgpt-api-build
holmesgpt-api-build: ## Build HolmesGPT REST API container (multi-arch)
	@echo "🏗️ Building HolmesGPT REST API container..."
	./scripts/build-holmesgpt-api.sh --image quay.io/jordigilh/holmesgpt-api --tag $(VERSION)

.PHONY: holmesgpt-api-build-amd64
holmesgpt-api-build-amd64: ## Build HolmesGPT REST API container (amd64 only)
	@echo "🏗️ Building HolmesGPT REST API container (amd64)..."
	./scripts/build-holmesgpt-api.sh --image quay.io/jordigilh/holmesgpt-api --tag $(VERSION) --platforms linux/amd64

.PHONY: holmesgpt-api-build-arm64
holmesgpt-api-build-arm64: ## Build HolmesGPT REST API container (arm64 only)
	@echo "🏗️ Building HolmesGPT REST API container (arm64)..."
	./scripts/build-holmesgpt-api.sh --image quay.io/jordigilh/holmesgpt-api --tag $(VERSION) --platforms linux/arm64

.PHONY: holmesgpt-api-build-dev
holmesgpt-api-build-dev: ## Build HolmesGPT REST API container for development (no cache, fast)
	@echo "🚀 Building HolmesGPT REST API container (development)..."
	./scripts/build-holmesgpt-api.sh --image quay.io/jordigilh/holmesgpt-api --tag dev-$(GIT_COMMIT) --platforms linux/$(GOARCH) --no-cache

.PHONY: holmesgpt-api-push
holmesgpt-api-push: ## Build and push HolmesGPT REST API container
	@echo "📤 Building and pushing HolmesGPT REST API container..."
	./scripts/build-holmesgpt-api.sh --image quay.io/jordigilh/holmesgpt-api --tag $(VERSION) --push

.PHONY: holmesgpt-api-test
holmesgpt-api-test: ## Test HolmesGPT REST API container
	@echo "🧪 Testing HolmesGPT REST API container..."
	@echo "Container functionality tests would run here"
	podman run --rm quay.io/jordigilh/holmesgpt-api:$(VERSION) python3.11 -c "import holmes; print('✅ HolmesGPT import OK')"

.PHONY: holmesgpt-api-security-scan
holmesgpt-api-security-scan: ## Run security scan on HolmesGPT REST API container
	@echo "🛡️ Running security scan on HolmesGPT REST API container..."
	@which trivy >/dev/null 2>&1 && trivy image quay.io/jordigilh/holmesgpt-api:$(VERSION) || echo "⚠️ Trivy not found, install for security scanning"
	@which grype >/dev/null 2>&1 && grype quay.io/jordigilh/holmesgpt-api:$(VERSION) || echo "⚠️ Grype not found, install for security scanning"

.PHONY: holmesgpt-api-run-local
holmesgpt-api-run-local: ## Run HolmesGPT REST API container locally
	@echo "🚀 Running HolmesGPT REST API container locally..."
	podman run -d \
		--name holmesgpt-api-local \
		-p 8090:8090 \
		-p 9091:9091 \
		-e HOLMESGPT_LLM_PROVIDER=${HOLMESGPT_LLM_PROVIDER:-openai} \
		-e HOLMESGPT_LLM_API_KEY=${HOLMESGPT_LLM_API_KEY} \
		-e HOLMESGPT_LLM_MODEL=${HOLMESGPT_LLM_MODEL:-gpt-4} \
		-e DEBUG=true \
		quay.io/jordigilh/holmesgpt-api:$(VERSION)
	@echo "✅ HolmesGPT REST API running at http://localhost:8090"
	@echo "📊 Metrics available at http://localhost:9091/metrics"

.PHONY: holmesgpt-api-stop-local
holmesgpt-api-stop-local: ## Stop local HolmesGPT REST API container
	@echo "🛑 Stopping local HolmesGPT REST API container..."
	podman stop holmesgpt-api-local || true
	podman rm holmesgpt-api-local || true

.PHONY: holmesgpt-api-logs
holmesgpt-api-logs: ## Show logs from local HolmesGPT REST API container
	podman logs -f holmesgpt-api-local

.PHONY: holmesgpt-api-shell
holmesgpt-api-shell: ## Open shell in HolmesGPT REST API container
	podman exec -it holmesgpt-api-local /bin/bash

##@ HolmesGPT Release Management
.PHONY: holmesgpt-api-release-patch
holmesgpt-api-release-patch: ## Release patch version (1.0.0 -> 1.0.1)
	@echo "🚀 Creating patch release..."
	./scripts/release-holmesgpt-api.sh --type patch

.PHONY: holmesgpt-api-release-minor
holmesgpt-api-release-minor: ## Release minor version (1.0.1 -> 1.1.0)
	@echo "🚀 Creating minor release..."
	./scripts/release-holmesgpt-api.sh --type minor

.PHONY: holmesgpt-api-release-major
holmesgpt-api-release-major: ## Release major version (1.1.0 -> 2.0.0)
	@echo "🚀 Creating major release..."
	./scripts/release-holmesgpt-api.sh --type major

.PHONY: holmesgpt-api-release-prerelease
holmesgpt-api-release-prerelease: ## Release prerelease version (1.1.0 -> 1.1.1-alpha)
	@echo "🚀 Creating prerelease..."
	./scripts/release-holmesgpt-api.sh --type prerelease --suffix alpha

.PHONY: holmesgpt-api-release-dry-run
holmesgpt-api-release-dry-run: ## Dry run release (show what would be done)
	@echo "🔍 Running release dry run..."
	./scripts/release-holmesgpt-api.sh --dry-run

.PHONY: holmesgpt-api-release-custom
holmesgpt-api-release-custom: ## Release custom version (usage: make holmesgpt-api-release-custom VERSION=1.2.3)
	@echo "🚀 Creating custom release: $(VERSION)..."
	./scripts/release-holmesgpt-api.sh $(VERSION)

##@ HolmesGPT Development
.PHONY: holmesgpt-api-dev-setup
holmesgpt-api-dev-setup: holmesgpt-api-init ## Setup HolmesGPT development environment
	@echo "🔧 Setting up HolmesGPT development environment..."
	chmod +x scripts/build-holmesgpt-api.sh
	chmod +x scripts/release-holmesgpt-api.sh
	chmod +x docker/holmesgpt-api/entrypoint.sh
	@echo "✅ HolmesGPT development environment ready"

.PHONY: test-holmesgpt-llm-only
test-holmesgpt-llm-only: ## Run holmesgpt-api LLM-only integration tests
	@echo "🧠 Running holmesgpt-api LLM-only tests..."
	cd docker/holmesgpt-api && \
	LLM_ENDPOINT=$(or $(LLM_ENDPOINT),http://192.168.1.169:8080) \
	LLM_PROVIDER=$(or $(LLM_PROVIDER),auto-detect) \
	USE_MOCK_LLM=$(or $(USE_MOCK_LLM),false) \
	PYTHON_CMD=/usr/local/bin/python3 \
	./run-llm-only-tests.sh

.PHONY: holmesgpt-api-clean
holmesgpt-api-clean: ## Clean HolmesGPT build artifacts
	@echo "🧹 Cleaning HolmesGPT build artifacts..."
	podman rmi quay.io/jordigilh/holmesgpt-api:$(VERSION) || true
	podman rmi quay.io/jordigilh/holmesgpt-api:dev-* || true
	podman system prune -f || true
	@echo "✅ Cleanup completed"

.PHONY: holmesgpt-api-all
holmesgpt-api-all: holmesgpt-api-dev-setup holmesgpt-api-build holmesgpt-api-test ## Setup, build, and test HolmesGPT REST API

# Default target
.DEFAULT_GOAL := help
=======
>>>>>>> crd_implementation
