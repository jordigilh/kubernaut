# Makefile for prometheus-alerts-slm

# Variables
APP_NAME=prometheus-alerts-slm
VERSION?=latest
REGISTRY?=quay.io/jordigilh
IMAGE_NAME=$(REGISTRY)/$(APP_NAME)
NAMESPACE=prometheus-alerts-slm

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

.PHONY: build
build: ## Build the application
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o bin/$(APP_NAME) ./cmd/$(APP_NAME)

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
	rm -rf bin/prometheus-alerts-slm bin/test-slm
	rm -f coverage.out coverage.html

.PHONY: clean-all
clean-all: ## Clean all build artifacts including test binaries (Go only)
	@echo "Cleaning all Go artifacts..."
	rm -rf bin/
	rm -f coverage.out coverage.html

##@ Container
.PHONY: docker-build
docker-build: ## Build container image
	docker build -t $(IMAGE_NAME):$(VERSION) .
	docker tag $(IMAGE_NAME):$(VERSION) $(IMAGE_NAME):latest

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
	ollama pull granite3.1-dense:8b
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
	curl -s -X POST http://localhost:11434/api/generate -d '{"model":"granite3.1-dense:8b","prompt":"Hello","stream":false}'

##@ Integration Testing (Hybrid Strategy)
# 🎯 STRATEGY: Kind for CI/CD and local testing, OCP for E2E tests

.PHONY: test-integration
test-integration: test-integration-kind ## Run integration tests (default: Kind cluster with real components)

.PHONY: test-integration-kind
test-integration-kind: envsetup ## Run integration tests with Kind cluster + real PostgreSQL + local LLM
	@echo "🏗️ Running integration tests with Kind cluster (Hybrid Strategy)..."
	@echo "  ├── Kubernetes: Real Kind cluster"
	@echo "  ├── Database: Real PostgreSQL + Vector DB (containerized)"
	@echo "  ├── LLM: Local model at localhost:8080"
	@echo "  └── Purpose: Local development and testing"
	@echo ""
	@echo "Starting containerized services..."
	make integration-services-start
	@echo "Setting up Kind cluster..."
	./scripts/setup-kind-cluster.sh
	@echo "Running integration tests..."
	$(eval KUBEBUILDER_ASSETS := $(shell pwd)/$(shell setup-envtest use --bin-dir ./bin -p path))
	KUBECONFIG=$$(kind get kubeconfig --name=prometheus-alerts-slm-test) \
	KUBEBUILDER_ASSETS=$(KUBEBUILDER_ASSETS) \
	LLM_ENDPOINT=http://localhost:8080 \
	LLM_MODEL=granite3.1-dense:8b \
	LLM_PROVIDER=localai \
	USE_FAKE_K8S_CLIENT=false \
	go test -v -tags=integration ./test/integration/... -timeout=45m
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
	KUBECONFIG=$$(kind get kubeconfig --name=prometheus-alerts-slm-test) \
	KUBEBUILDER_ASSETS=$(KUBEBUILDER_ASSETS) \
	USE_MOCK_LLM=true \
	CI=true \
	USE_FAKE_K8S_CLIENT=false \
	go test -v -tags=integration ./test/integration/... -timeout=30m
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

.PHONY: test-integration-quick
test-integration-quick: envsetup ## Run integration tests (skip slow tests)
	@echo "Running quick integration tests..."
	@echo "Using local envtest binaries..."
	$(eval KUBEBUILDER_ASSETS := $(shell pwd)/$(shell setup-envtest use --bin-dir ./bin -p path))
	KUBEBUILDER_ASSETS=$(KUBEBUILDER_ASSETS) SKIP_SLOW_TESTS=true LLM_ENDPOINT=http://localhost:11434 LLM_MODEL=granite3.1-dense:8b LLM_PROVIDER=ollama go test -v -tags=integration ./test/integration/... -timeout=15m

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
	KUBEBUILDER_ASSETS=$(KUBEBUILDER_ASSETS) USE_FAKE_K8S_CLIENT=true LLM_ENDPOINT=http://localhost:8080 LLM_MODEL=granite3.1-dense:8b LLM_PROVIDER=localai go test -v -tags=integration ./test/integration/... -timeout=30m

.PHONY: test-integration-ollama
test-integration-ollama: envsetup ## [LEGACY] Run integration tests with Ollama at localhost:11434 (use test-integration-kind instead)
	@echo "⚠️  LEGACY: Running integration tests with Ollama at localhost:11434..."
	@echo "💡 RECOMMENDED: Use 'make test-integration-kind' for local testing instead"
	@echo "Using local envtest binaries..."
	$(eval KUBEBUILDER_ASSETS := $(shell pwd)/$(shell setup-envtest use --bin-dir ./bin -p path))
	KUBEBUILDER_ASSETS=$(KUBEBUILDER_ASSETS) LLM_ENDPOINT=http://localhost:11434 LLM_MODEL=granite3.1-dense:8b LLM_PROVIDER=ollama go test -v -tags=integration ./test/integration/... -timeout=30m

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
	go test -v -tags=e2e ./test/e2e/... -timeout=120m
	@echo "E2E tests completed"

.PHONY: test-e2e-kind
test-e2e-kind: ## [ALTERNATIVE] Run e2e tests with KinD cluster (limited scenarios)
	@echo "⚠️  ALTERNATIVE: Running E2E tests with KinD cluster..."
	@echo "💡 NOTE: Limited to single/dual-node scenarios. Use test-e2e-ocp for full E2E testing"
	@echo "Setting up KinD cluster for e2e tests..."
	./scripts/setup-kind-cluster.sh
	@echo "Running e2e tests with KinD..."
	KUBECONFIG=$$(kind get kubeconfig --name=prometheus-alerts-slm-test) USE_KIND=true go test -v -tags=e2e ./test/e2e/... -run TestKindClusterOperations -timeout=45m
	@echo "Cleaning up KinD cluster..."
	./scripts/cleanup-kind-cluster.sh

.PHONY: test-e2e-monitoring
test-e2e-monitoring: ## Run e2e tests with full monitoring stack
	@echo "Setting up complete monitoring stack..."
	./scripts/setup-kind-cluster.sh
	@echo "Running complete monitoring flow tests..."
	KUBECONFIG=~/.kube/config USE_KIND=true go test -v -tags=e2e ./test/e2e/... -run TestCompleteMonitoringFlow -timeout=60m
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
	@echo "Remote cluster: $(REMOTE_HOST), Local AI: localhost:8080"
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
	curl -s http://localhost:8080/health >/dev/null 2>&1 && echo "✓ AI model running on localhost:8080" || echo "✗ AI model not running on localhost:8080"
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
	kind load docker-image prometheus-alerts-slm:latest --name prometheus-alerts-slm-test || echo "KinD cluster not running"

.PHONY: setup-kind
setup-kind: ## Setup KinD cluster for testing
	./scripts/setup-kind-cluster.sh

.PHONY: cleanup-kind
cleanup-kind: ## Cleanup KinD cluster
	./scripts/cleanup-kind-cluster.sh

.PHONY: test-webhook
test-webhook: ## Test webhook endpoint
	curl -X POST http://localhost:8080/alerts \
		-H "Content-Type: application/json" \
		-H "Authorization: Bearer test-token" \
		-d @test/fixtures/sample-alert.json

.PHONY: test-health
test-health: ## Test health endpoints
	curl -f http://localhost:8080/health
	curl -f http://localhost:8080/ready

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

##@ HolmesGPT REST API
.PHONY: holmesgpt-api-init
holmesgpt-api-init: ## Initialize HolmesGPT submodule
	@echo "🔄 Initializing HolmesGPT submodule..."
	git submodule update --init --recursive vendor/holmesgpt
	@echo "✅ HolmesGPT submodule initialized"

.PHONY: holmesgpt-api-update
holmesgpt-api-update: ## Update HolmesGPT submodule to latest
	@echo "🔄 Updating HolmesGPT submodule..."
	git submodule update --remote vendor/holmesgpt
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