# Kubernaut Must-Gather - Makefile
# BR-PLATFORM-001: Build, test, and deployment automation
#
# Testing Strategy:
#   - All tests run in container (host-agnostic, eliminates macOS vs Linux differences)
#   - Local dev: ARM64-only builds (Apple Silicon compatible)
#   - CI/Production: Multi-arch builds (AMD64 + ARM64)

.PHONY: help build build-local build-multiarch push test test-container test-e2e deploy-rbac clean

# Configuration
IMAGE_REGISTRY ?= quay.io/kubernaut
IMAGE_NAME ?= must-gather
IMAGE_TAG ?= v1.0.0
IMAGE_LATEST ?= latest
LOCAL_IMAGE_NAME ?= localhost/$(IMAGE_NAME)

# Platform auto-detection (amd64 for x86_64, arm64 for aarch64)
PLATFORM ?= $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

# Paths
TEST_DIR := test
TEMPLATES_DIR := templates

help: ## Show this help message
	@echo "Kubernaut Must-Gather - Available targets:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build-local: ## Build image for native platform (auto-detected: amd64 or arm64)
	@echo "Building $(PLATFORM) must-gather image for local development..."
	podman build --platform linux/$(PLATFORM) \
		-t $(LOCAL_IMAGE_NAME):test \
		.
	@echo "✅ Local $(PLATFORM) build complete!"

build-multiarch: ## Build multi-arch image (AMD64 + ARM64) for CI/production
	@echo "Building multi-arch must-gather image..."
	podman build --platform linux/amd64,linux/arm64 \
		-t $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) \
		-t $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_LATEST) \
		.
	@echo "✅ Multi-arch build complete!"

build: build-multiarch ## Alias for build-multiarch (default for CI)

push: ## Push must-gather image to registry
	@echo "Pushing images to $(IMAGE_REGISTRY)..."
	podman push $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
	podman push $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_LATEST)
	@echo "✅ Push complete!"

build-push: build-multiarch push ## Build multi-arch and push

test: test-container ## Run all tests in ARM64 container (host-agnostic)

test-container: build-local ## Run unit tests inside native platform container (auto-detected)
	@echo "Running unit tests in $(PLATFORM) container..."
	podman run --rm \
		--platform linux/$(PLATFORM) \
		-v $(PWD)/test:/must-gather/test:ro \
		-v $(PWD)/collectors:/usr/share/must-gather/collectors:ro \
		-v $(PWD)/sanitizers:/usr/share/must-gather/sanitizers:ro \
		-v $(PWD)/utils:/usr/share/must-gather/utils:ro \
		-v $(PWD)/gather.sh:/usr/bin/gather:ro \
		-w /must-gather \
		--entrypoint bash \
		$(LOCAL_IMAGE_NAME):test \
		-c "curl -sSL https://github.com/bats-core/bats-core/archive/refs/tags/v1.11.0.tar.gz | tar -xz && \
		    cd bats-core-1.11.0 && ./install.sh /usr/local && cd .. && rm -rf bats-core-1.11.0 && \
		    bats /must-gather/test/test_*.bats"
	@echo "✅ Container tests complete!"

test-verbose: build-local ## Run container tests with verbose output (native platform)
	@echo "Running unit tests in $(PLATFORM) container (verbose)..."
	podman run --rm \
		--platform linux/$(PLATFORM) \
		-v $(PWD)/test:/must-gather/test:ro \
		-v $(PWD)/collectors:/usr/share/must-gather/collectors:ro \
		-v $(PWD)/sanitizers:/usr/share/must-gather/sanitizers:ro \
		-v $(PWD)/utils:/usr/share/must-gather/utils:ro \
		-v $(PWD)/gather.sh:/usr/bin/gather:ro \
		-w /must-gather \
		--entrypoint bash \
		$(LOCAL_IMAGE_NAME):test \
		-c "curl -sSL https://github.com/bats-core/bats-core/archive/refs/tags/v1.11.0.tar.gz | tar -xz && \
		    cd bats-core-1.11.0 && ./install.sh /usr/local && cd .. && rm -rf bats-core-1.11.0 && \
		    bats --trace /must-gather/test/test_*.bats"

test-e2e: ## Run E2E tests (requires cluster)
	@echo "Running E2E tests..."
	@if ! command -v kubectl &> /dev/null; then \
		echo "❌ Error: kubectl not found"; \
		exit 1; \
	fi
	@if ! kubectl cluster-info &> /dev/null; then \
		echo "❌ Error: Cannot connect to cluster"; \
		exit 1; \
	fi
	KUBERNAUT_E2E_TESTS=1 bats $(TEST_DIR)/integration/test_e2e.bats
	@echo "✅ E2E tests complete!"

deploy-rbac: ## Deploy RBAC resources to cluster
	@echo "Deploying RBAC resources..."
	kubectl apply -f $(TEMPLATES_DIR)/serviceaccount.yaml
	kubectl apply -f $(TEMPLATES_DIR)/clusterrole.yaml
	kubectl apply -f $(TEMPLATES_DIR)/clusterrolebinding.yaml
	@echo "✅ RBAC deployed!"

deploy-rbac-kustomize: ## Deploy RBAC using kustomize
	@echo "Deploying RBAC resources with kustomize..."
	kubectl apply -k $(TEMPLATES_DIR)/
	@echo "✅ RBAC deployed!"

verify-rbac: ## Verify RBAC permissions
	@echo "Verifying RBAC permissions..."
	kubectl get serviceaccount kubernaut-must-gather -n default
	kubectl get clusterrole kubernaut-must-gather
	kubectl get clusterrolebinding kubernaut-must-gather
	kubectl auth can-i get pods --all-namespaces --as=system:serviceaccount:default:kubernaut-must-gather
	@echo "✅ RBAC verified!"

run-local: ## Run must-gather locally (requires cluster)
	@echo "Running must-gather locally..."
	./gather.sh --dest-dir=/tmp/must-gather

run-pod: deploy-rbac ## Run must-gather as Kubernetes pod
	@echo "Running must-gather pod..."
	kubectl run kubernaut-must-gather \
		--image=$(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) \
		--rm --attach --restart=Never \
		--serviceaccount=kubernaut-must-gather \
		-- /usr/bin/gather

clean: ## Clean up test artifacts and temporary files
	@echo "Cleaning up..."
	rm -rf /tmp/must-gather-test-*
	rm -rf /tmp/kubernaut-must-gather-*
	find . -name "*.pre-sanitize" -delete
	@echo "✅ Cleanup complete!"

clean-rbac: ## Remove RBAC resources from cluster
	@echo "Removing RBAC resources..."
	kubectl delete -f $(TEMPLATES_DIR)/ --ignore-not-found
	@echo "✅ RBAC removed!"

lint: ## Run shellcheck on bash scripts
	@echo "Running shellcheck..."
	@if ! command -v shellcheck &> /dev/null; then \
		echo "⚠️  Warning: shellcheck not found. Install with: brew install shellcheck"; \
	else \
		shellcheck gather.sh collectors/*.sh sanitizers/*.sh utils/*.sh || true; \
		echo "✅ Shellcheck complete!"; \
	fi

format: ## Format bash scripts with shfmt
	@echo "Formatting bash scripts..."
	@if ! command -v shfmt &> /dev/null; then \
		echo "⚠️  Warning: shfmt not found. Install with: brew install shfmt"; \
	else \
		shfmt -w -i 4 gather.sh collectors/*.sh sanitizers/*.sh utils/*.sh; \
		echo "✅ Format complete!"; \
	fi

validate: lint test ## Run linting and container tests

ci: lint build-multiarch ## CI pipeline: lint + multi-arch build (tests run separately in CI)

all: build-multiarch push deploy-rbac test-e2e ## Build multi-arch, push, deploy, and test everything

.DEFAULT_GOAL := help

