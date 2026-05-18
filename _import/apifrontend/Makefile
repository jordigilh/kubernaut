IMG ?= quay.io/kubernaut-ai/apifrontend:latest
IMAGE_REGISTRY ?= quay.io/kubernaut-ai
IMAGE_TAG ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
IMAGE_ARCH ?= amd64
IMAGE_TARGET ?= production
CONTAINER_TOOL ?= $(shell command -v podman 2>/dev/null || echo docker)
APP_VERSION ?= $(IMAGE_TAG)
GIT_COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
CONTROLLER_GEN ?= $(shell which controller-gen 2>/dev/null)
GINKGO ?= $(shell which ginkgo 2>/dev/null || echo "go run github.com/onsi/ginkgo/v2/ginkgo")
LOCALBIN ?= $(shell pwd)/bin
COVERPKGS = ./internal/auth/...,./internal/ratelimit/...,./internal/security/...,./internal/httputil/...,./internal/logging/...,./internal/requestid/...,./internal/audit/...,./internal/metrics/...,./internal/agent/...,./internal/tools/...,./internal/ka/...,./internal/ds/...,./internal/session/...,./internal/config/...,./internal/handler/...,./internal/launcher/...,./internal/resilience/...,./internal/streaming/...,./internal/controller/...,./internal/prometheus/...,./internal/severity/...,./internal/tlswiring/...,./internal/validate/...

.PHONY: all
all: build

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

##@ Build

.PHONY: build
build: fmt vet ## Build the apifrontend binary
	go build -o bin/apifrontend ./cmd/apifrontend/

.PHONY: run
run: fmt vet ## Run locally with default config
	go run ./cmd/apifrontend/

.PHONY: fmt
fmt: ## Format Go source files
	go fmt ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

##@ Test

.PHONY: test
test: test-unit ## Run unit tests (alias)

.PHONY: test-unit
test-unit: fmt vet ## Run all unit tests with race detection and coverage
	$(GINKGO) -v --race --coverpkg=$(COVERPKGS) --coverprofile=coverage_unit_apifrontend.out ./internal/...

.PHONY: test-integration
test-integration: ## Run integration tests (matches CI runner)
	$(GINKGO) -v --race --tags=integration --coverpkg=$(COVERPKGS) --coverprofile=coverage_integration_apifrontend.out ./internal/...

.PHONY: test-bridge
test-bridge: fmt vet ## Run MCP bridge tests (pass GINKGO_LABEL="tier1" to filter)
	$(GINKGO) -v --race $(if $(GINKGO_LABEL),--label-filter="$(GINKGO_LABEL) && bridge",--label-filter="bridge") --coverpkg=$(COVERPKGS) --coverprofile=coverage_bridge_apifrontend.out ./internal/handler/...

.PHONY: test-integration-containers
test-integration-containers: ## Run integration tests with real DS+KA containers (Podman)
	$(GINKGO) -v --race --timeout=15m --coverpkg=$(COVERPKGS) --coverprofile=coverage_integration_apifrontend.out ./test/integration/...

.PHONY: test-all
test-all: test-unit test-integration ## Run unit + integration tests

##@ E2E

E2E_CLUSTER_NAME ?= apifrontend-e2e
E2E_NAMESPACE ?= kubernaut-system

.PHONY: test-e2e
test-e2e: ## Run E2E tests (full lifecycle: build, cluster, deploy, test, teardown)
	$(GINKGO) -v --race --timeout=20m --coverpkg=$(COVERPKGS) --coverprofile=coverage_e2e_apifrontend.out ./test/e2e/

.PHONY: test-e2e-reuse
test-e2e-reuse: ## Run E2E tests against an existing cluster (skip infra setup/teardown)
	AF_E2E_SKIP_INFRA=true \
	AF_E2E_BASE_URL=https://localhost:18443 \
	AF_E2E_CA_CERT=/tmp/apifrontend-e2e-certs/ca.crt \
	AF_E2E_DEX_URL=http://localhost:15556/dex \
	AF_E2E_CLIENT_ID=kubernaut-apifrontend \
	AF_E2E_CLIENT_SECRET=e2e-client-secret \
	AF_E2E_USERNAME=e2e-user@kubernaut.ai \
	AF_E2E_PASSWORD=password \
	$(GINKGO) -v --race --timeout=5m --coverpkg=$(COVERPKGS) --coverprofile=coverage_e2e_apifrontend.out ./test/e2e/

.PHONY: e2e-teardown
e2e-teardown: ## Delete the E2E Kind cluster
	kind delete cluster --name $(E2E_CLUSTER_NAME)

.PHONY: e2e-port-forward
e2e-port-forward: ## Start port-forwards for AF and DEX (for test-e2e-reuse)
	kubectl --context kind-$(E2E_CLUSTER_NAME) port-forward -n $(E2E_NAMESPACE) svc/apifrontend 18443:8443 &
	kubectl --context kind-$(E2E_CLUSTER_NAME) port-forward -n $(E2E_NAMESPACE) svc/dex 15556:5556 &
	@for i in $$(seq 1 30); do curl -s --cacert /tmp/apifrontend-e2e-certs/ca.crt https://localhost:18443/healthz >/dev/null 2>&1 && break || sleep 1; done
	@for i in $$(seq 1 30); do curl -sf http://localhost:15556/dex/healthz >/dev/null 2>&1 && break || sleep 1; done
	@echo "Port-forwards ready: AF=https://localhost:18443, DEX=http://localhost:15556/dex"

.PHONY: e2e-teardown
e2e-teardown: ## Delete E2E Kind cluster
	kind delete cluster --name $(E2E_CLUSTER_NAME)

##@ Container

.PHONY: image-build
image-build: ## Build container image (production target, supports IMAGE_ARCH)
	$(CONTAINER_TOOL) build \
		--platform linux/$(IMAGE_ARCH) \
		--target $(IMAGE_TARGET) \
		--build-arg APP_VERSION=$(APP_VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE_REGISTRY)/apifrontend:$(IMAGE_TAG)-$(IMAGE_ARCH) .

.PHONY: image-push
image-push: ## Push container image to registry
	$(CONTAINER_TOOL) push $(IMAGE_REGISTRY)/apifrontend:$(IMAGE_TAG)-$(IMAGE_ARCH)

.PHONY: image-manifest
image-manifest: ## Create and push multi-arch manifest (amd64 + arm64)
	$(CONTAINER_TOOL) manifest rm $(IMAGE_REGISTRY)/apifrontend:$(IMAGE_TAG) 2>/dev/null || true
	$(CONTAINER_TOOL) manifest create $(IMAGE_REGISTRY)/apifrontend:$(IMAGE_TAG) \
		$(IMAGE_REGISTRY)/apifrontend:$(IMAGE_TAG)-amd64 \
		$(IMAGE_REGISTRY)/apifrontend:$(IMAGE_TAG)-arm64
	$(CONTAINER_TOOL) manifest push $(IMAGE_REGISTRY)/apifrontend:$(IMAGE_TAG) \
		docker://$(IMAGE_REGISTRY)/apifrontend:$(IMAGE_TAG)

.PHONY: cross-build
cross-build: ## Cross-compile binary for IMAGE_ARCH (no QEMU needed)
	CGO_ENABLED=0 GOOS=linux GOARCH=$(IMAGE_ARCH) go build \
		-ldflags="-s -w -X main.Version=$(APP_VERSION) -X main.GitCommit=$(GIT_COMMIT) -X main.BuildDate=$(BUILD_DATE)" \
		-o bin/apifrontend-$(IMAGE_ARCH) ./cmd/apifrontend/

##@ Generate

.PHONY: generate
generate: manifests ## Generate Go code from CRD types
	$(CONTROLLER_GEN) object paths="./api/..."

.PHONY: manifests
manifests: ## Generate CRD manifests and RBAC
	$(CONTROLLER_GEN) rbac:roleName=apifrontend-role crd paths="./api/..." output:crd:artifacts:config=config/crd/bases

##@ Coverage

.PHONY: coverage-report
coverage-report: ## Show per-tier coverage breakdown (table)
	@python3 scripts/coverage/coverage_report.py

.PHONY: coverage-report-markdown
coverage-report-markdown: ## Generate markdown coverage report for GitHub PR comments
	@python3 scripts/coverage/coverage_report.py --format markdown

.PHONY: coverage-report-json
coverage-report-json: ## Generate JSON coverage report for CI/CD integration
	@python3 scripts/coverage/coverage_report.py --format json

.PHONY: coverage-html
coverage-html: ## Generate HTML coverage report from unit test profile
	go tool cover -html=coverage_unit_apifrontend.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: coverage-func
coverage-func: ## Print per-function coverage from unit test profile
	go tool cover -func=coverage_unit_apifrontend.out

##@ Performance

.PHONY: test-perf-local
test-perf-local:
	@which k6 >/dev/null 2>&1 || { echo "k6 not found — install: https://k6.io/docs/get-started/installation/"; exit 1; }
	k6 run --dry-run tests/performance/scripts/health-baseline.js
	k6 run --dry-run tests/performance/scripts/mcp-tools-call.js
	k6 run --dry-run tests/performance/scripts/sse-streams.js
	k6 run --dry-run tests/performance/scripts/mixed-workload.js

##@ Deploy (Kind)

KIND_CLUSTER_NAME ?= apifrontend-dev
KIND_CONFIG_DEV ?= deploy/kustomize/overlays/dev/kind-config.yaml
KIND_CONFIG_CI ?= deploy/kustomize/overlays/ci/kind-config.yaml
CERT_DIR ?= /tmp/apifrontend-dev-certs

.PHONY: kind-create
kind-create: ## Create a Kind cluster for development
	@which kind >/dev/null 2>&1 || { echo "kind not found — install: https://kind.sigs.k8s.io/docs/user/quick-start/#installation"; exit 1; }
	kind create cluster --name $(KIND_CLUSTER_NAME) --config $(KIND_CONFIG_DEV)

.PHONY: kind-delete
kind-delete: ## Delete the Kind cluster
	kind delete cluster --name $(KIND_CLUSTER_NAME)

.PHONY: kind-load
kind-load: image-build ## Build and load image into Kind cluster
	kind load docker-image $(IMAGE_REGISTRY)/apifrontend:$(IMAGE_TAG)-$(IMAGE_ARCH) --name $(KIND_CLUSTER_NAME)

.PHONY: generate-dev-certs
generate-dev-certs: ## Generate self-signed TLS certificates for dev
	bash deploy/kustomize/overlays/dev/generate-certs.sh $(CERT_DIR)
	kubectl create namespace kubernaut-system --dry-run=client -o yaml | kubectl apply -f -
	kubectl create secret tls apifrontend-tls --cert=$(CERT_DIR)/tls.crt --key=$(CERT_DIR)/tls.key -n kubernaut-system --dry-run=client -o yaml | kubectl apply -f -
	kubectl create secret generic apifrontend-ca --from-file=ca.crt=$(CERT_DIR)/ca.crt -n kubernaut-system --dry-run=client -o yaml | kubectl apply -f -

.PHONY: deploy-dev
deploy-dev: ## Deploy to Kind cluster using dev overlay
	kubectl apply -k deploy/kustomize/overlays/dev/
	kubectl rollout status deployment/apifrontend -n kubernaut-system --timeout=120s

.PHONY: deploy-ci
deploy-ci: ## Deploy to Kind cluster using CI overlay
	kubectl apply -k deploy/kustomize/overlays/ci/
	kubectl rollout status deployment/apifrontend -n kubernaut-system --timeout=120s

OVERLAY ?= dev

.PHONY: undeploy
undeploy: ## Remove kustomize-managed resources (OVERLAY=dev|ci)
	kubectl delete -k deploy/kustomize/overlays/$(OVERLAY)/ --ignore-not-found=true

##@ Validate

.PHONY: verify-generate
verify-generate: generate
	git diff --exit-code ./api/ ./config/

.PHONY: validate-openapi
validate-openapi:
	@which vacuum >/dev/null 2>&1 || { echo "vacuum not found — install: go install github.com/daveshanley/vacuum@v0.26.4"; exit 1; }
	vacuum lint api/openapi/apifrontend-v1.yaml

.PHONY: validate-maturity-ci
validate-maturity-ci:
	bash hack/validate-maturity.sh

.PHONY: validate-kustomize
validate-kustomize: ## Validate kustomize build for dev, ci, and e2e overlays
	@which kubectl >/dev/null 2>&1 || { echo "kubectl not found — install: https://kubernetes.io/docs/tasks/tools/"; exit 1; }
	kubectl kustomize deploy/kustomize/overlays/dev/ > /dev/null
	kubectl kustomize deploy/kustomize/overlays/ci/ > /dev/null
	kubectl kustomize deploy/kustomize/overlays/e2e/ > /dev/null
	@echo "Kustomize build validated for dev, ci, and e2e overlays"

##@ Local LLM Testing

.PHONY: test-llm-local
test-llm-local: ## Run LLM integration tests locally (requires LLM_PROJECT, LLM_REGION env vars)
	@echo "Running LLM integration tests (never runs in CI)..."
	go test -v -count=1 --tags=llm_integration ./internal/severity/...

##@ Security & Supply Chain

.PHONY: sbom
sbom:
	@which syft >/dev/null 2>&1 || { echo "syft not found — install: https://github.com/anchore/syft#installation"; exit 1; }
	syft packages $(IMG) -o cyclonedx-json > sbom.cdx.json
	@echo "SBOM generated: sbom.cdx.json"

.PHONY: image-scan
image-scan:
	@which trivy >/dev/null 2>&1 || { echo "trivy not found — install: https://aquasecurity.github.io/trivy/"; exit 1; }
	trivy image --severity CRITICAL,HIGH --exit-code 1 --ignorefile .trivyignore $(IMG)

##@ Clean

.PHONY: clean
clean: ## Remove build artifacts and coverage files
	rm -rf bin/ coverage_*.out cover.out cover-integration.out cover-bridge.out cover-it.out coverage.html sbom.cdx.json
