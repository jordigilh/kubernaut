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

.PHONY: setup-python-venv
setup-python-venv: ## Set up Python virtual environment
	@echo "Setting up Python virtual environment..."
	@cd python-api && \
	if [ ! -d "venv" ]; then \
		echo "Creating virtual environment..."; \
		python3 -m venv venv; \
		echo "Installing base dependencies..."; \
		venv/bin/pip install --upgrade pip setuptools wheel; \
		echo "Installing Python dependencies..."; \
		venv/bin/pip install -r requirements.txt || { \
			echo "Warning: Some dependencies failed to install, continuing..."; \
		}; \
		echo "Virtual environment setup complete"; \
	else \
		echo "Virtual environment already exists"; \
	fi

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
test: setup-python-venv ## Run unit tests (Go and Python)
	@echo "Running Go unit tests..."
	go test -v ./... -tags="!integration,!e2e"
	@echo "Running Python unit tests..."
	cd python-api && $(MAKE) -f Makefile.test test-unit

.PHONY: test-python
test-python: setup-python-venv ## Run Python unit tests only
	@echo "Running Python unit tests..."
	cd python-api && $(MAKE) -f Makefile.test test-unit

.PHONY: test-go
test-go: ## Run Go unit tests only
	@echo "Running Go unit tests..."
	go test -v ./... -tags="!integration,!e2e"

.PHONY: test-coverage
test-coverage: setup-python-venv ## Run unit tests with coverage (Go and Python)
	@echo "Running Go unit tests with coverage..."
	go test -coverprofile=coverage.out ./... -tags="!integration,!e2e"
	go tool cover -html=coverage.out -o coverage.html
	@echo "Go coverage report generated: coverage.html"
	@echo "Running Python unit tests with coverage..."
	cd python-api && $(MAKE) -f Makefile.test test-coverage

.PHONY: test-all
test-all: validate-integration test test-integration test-e2e ## Run all tests (unit, integration, e2e)
	@echo "All test suites completed"

.PHONY: test-ci
test-ci: ## Run tests suitable for CI environment (Go and Python)
	@echo "Running CI test suite..."
	make test
	make test-integration-local
	@echo "CI tests completed"

.PHONY: lint
lint: setup-python-venv ## Run linters (Go and Python)
	@echo "Running Go linter..."
	golangci-lint run
	@echo "Running Python linter..."
	cd python-api && $(MAKE) -f Makefile.test test-lint

.PHONY: lint-go
lint-go: ## Run Go linter only
	@echo "Running Go linter..."
	golangci-lint run

.PHONY: lint-python
lint-python: setup-python-venv ## Run Python linter only
	@echo "Running Python linter..."
	cd python-api && $(MAKE) -f Makefile.test test-lint

.PHONY: fmt
fmt: setup-python-venv ## Format code (Go and Python)
	@echo "Formatting Go code..."
	go fmt ./...
	@echo "Formatting Python code..."
	cd python-api && $(MAKE) -f Makefile.test test-format-fix

.PHONY: fmt-go
fmt-go: ## Format Go code only
	@echo "Formatting Go code..."
	go fmt ./...

.PHONY: fmt-python
fmt-python: setup-python-venv ## Format Python code only
	@echo "Formatting Python code..."
	cd python-api && $(MAKE) -f Makefile.test test-format-fix

.PHONY: clean
clean: ## Clean build artifacts (Go and Python)
	@echo "Cleaning Go artifacts..."
	rm -rf bin/prometheus-alerts-slm bin/test-slm
	rm -f coverage.out coverage.html
	@echo "Cleaning Python artifacts..."
	cd python-api && $(MAKE) -f Makefile.test clean-test

.PHONY: clean-all
clean-all: ## Clean all build artifacts including test binaries (Go and Python)
	@echo "Cleaning all Go artifacts..."
	rm -rf bin/
	rm -f coverage.out coverage.html
	@echo "Cleaning all Python artifacts..."
	cd python-api && $(MAKE) -f Makefile.test clean-all

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

##@ Testing
.PHONY: test-integration
test-integration: envsetup ## Run integration tests with fake Kubernetes and local Ollama
	@echo "Running integration tests with fake Kubernetes and local Ollama..."
	@echo "Using local envtest binaries..."
	$(eval KUBEBUILDER_ASSETS := $(shell pwd)/$(shell setup-envtest use --bin-dir ./bin -p path))
	KUBEBUILDER_ASSETS=$(KUBEBUILDER_ASSETS) LLM_ENDPOINT=http://localhost:11434 LLM_MODEL=granite3.1-dense:8b LLM_PROVIDER=ollama go test -v -tags=integration ./test/integration/... -timeout=30m

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
test-integration-ramalama: envsetup setup-python-venv ## Run integration tests with ramalama
	@echo "Running integration tests with ramalama at 192.168.1.169:8080..."
	@echo "Using local envtest binaries..."
	$(eval KUBEBUILDER_ASSETS := $(shell pwd)/$(shell setup-envtest use --bin-dir ./bin -p path))
	KUBEBUILDER_ASSETS=$(KUBEBUILDER_ASSETS) LLM_ENDPOINT=http://192.168.1.169:8080 LLM_MODEL=ggml-org/gpt-oss-20b-GGUF LLM_PROVIDER=ollama go test -v -tags=integration ./test/integration/... -timeout=30m
	@echo "Running Python integration tests with ramalama..."
	cd python-api && RAMALAMA_URL=http://192.168.1.169:8080 HOLMES_LLM_PROVIDER=ramalama HOLMES_DEFAULT_MODEL=gpt-oss:20b $(MAKE) -f Makefile.test test-integration

.PHONY: test-e2e
test-e2e: ## Run e2e tests with local setup
	@echo "Running e2e tests with local setup..."
	go test -v -tags=e2e ./test/e2e/... -timeout=45m

.PHONY: test-e2e-kind
test-e2e-kind: ## Run e2e tests with KinD cluster
	@echo "Setting up KinD cluster for e2e tests..."
	./scripts/setup-kind-cluster.sh
	@echo "Running e2e tests with KinD..."
	KUBECONFIG=~/.kube/config USE_KIND=true go test -v -tags=e2e ./test/e2e/... -run TestKindClusterOperations -timeout=45m
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

# Default target
.DEFAULT_GOAL := help