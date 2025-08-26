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

.PHONY: build
build: ## Build the application
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o bin/$(APP_NAME) ./cmd/$(APP_NAME)

.PHONY: test
test: ## Run unit tests
	@echo "Running unit tests..."
	go test -v ./... -tags="!integration,!e2e"

.PHONY: test-coverage
test-coverage: ## Run unit tests with coverage
	@echo "Running unit tests with coverage..."
	go test -coverprofile=coverage.out ./... -tags="!integration,!e2e"
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

.PHONY: test-all
test-all: validate-integration test test-integration test-e2e ## Run all tests (unit, integration, e2e)
	@echo "All test suites completed"

.PHONY: test-ci
test-ci: ## Run tests suitable for CI environment
	@echo "Running CI test suite..."
	make test
	make test-integration-local
	@echo "CI tests completed"

.PHONY: lint
lint: ## Run linter
	golangci-lint run

.PHONY: fmt
fmt: ## Format code
	go fmt ./...

.PHONY: clean
clean: ## Clean build artifacts
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

##@ Testing
.PHONY: test-integration
test-integration: ## Run integration tests with local Ollama
	@echo "Running integration tests with local Ollama..."
	OLLAMA_ENDPOINT=http://localhost:11434 OLLAMA_MODEL=granite3.1-dense:8b go test -v -tags=integration ./test/integration/... -timeout=30m

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
test-integration-quick: ## Run integration tests (skip slow tests)
	@echo "Running quick integration tests..."
	SKIP_SLOW_TESTS=true OLLAMA_ENDPOINT=http://localhost:11434 OLLAMA_MODEL=granite3.1-dense:8b go test -v -tags=integration ./test/integration/... -timeout=15m

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