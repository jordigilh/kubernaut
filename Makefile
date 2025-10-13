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

##@ Notification Service Integration Tests
# Per ADR-017: NotificationRequest CRD-based notification service
# Requires Kind cluster with NotificationRequest CRD and controller deployed

NOTIFICATION_CLUSTER ?= kubernaut-integration
NOTIFICATION_NAMESPACE ?= kubernaut-notifications
NOTIFICATION_IMAGE ?= kubernaut-notification:latest
NOTIFICATION_CRD ?= config/crd/bases/notification.kubernaut.ai_notificationrequests.yaml

.PHONY: test-notification-setup
test-notification-setup: ## Setup Kind cluster and deploy Notification controller
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🚀 Notification Service Integration Test Setup"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo ""
	@echo "📋 Setup Steps:"
	@echo "  1. Ensure Kind cluster exists"
	@echo "  2. Generate CRD manifests"
	@echo "  3. Install NotificationRequest CRD"
	@echo "  4. Build controller image"
	@echo "  5. Load image into Kind"
	@echo "  6. Deploy controller"
	@echo "  7. Verify deployment"
	@echo ""
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "1️⃣  Ensuring Kind cluster exists: $(NOTIFICATION_CLUSTER)"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@KIND_CLUSTER_NAME=$(NOTIFICATION_CLUSTER) ./scripts/ensure-kind-cluster.sh
	@kubectl config use-context kind-$(NOTIFICATION_CLUSTER)
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "2️⃣  Generating CRD manifests"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@$(MAKE) manifests
	@echo "✅ CRD manifests generated"
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "3️⃣  Installing NotificationRequest CRD"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@if [ ! -f "$(NOTIFICATION_CRD)" ]; then \
		echo "❌ Error: CRD file not found: $(NOTIFICATION_CRD)"; \
		exit 1; \
	fi
	@kubectl apply -f $(NOTIFICATION_CRD)
	@echo "⏳ Waiting for CRD to be established..."
	@kubectl wait --for condition=established --timeout=60s crd/notificationrequests.notification.kubernaut.ai
	@echo "✅ NotificationRequest CRD installed and established"
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "4️⃣  Building and loading controller image"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@KIND_CLUSTER_NAME=$(NOTIFICATION_CLUSTER) ./scripts/build-notification-controller.sh --kind
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "5️⃣  Deploying Notification controller"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@kubectl apply -k deploy/notification/
	@echo "⏳ Waiting for controller deployment to be ready..."
	@kubectl wait --for=condition=available --timeout=120s \
		deployment/notification-controller -n $(NOTIFICATION_NAMESPACE) || \
		(echo "⚠️  Deployment not ready, checking status..." && \
		 kubectl get pods -n $(NOTIFICATION_NAMESPACE) && \
		 kubectl describe deployment/notification-controller -n $(NOTIFICATION_NAMESPACE) && \
		 exit 1)
	@echo "✅ Notification controller deployed successfully"
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "6️⃣  Verifying deployment"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "Namespace: $(NOTIFICATION_NAMESPACE)"
	@kubectl get pods -n $(NOTIFICATION_NAMESPACE)
	@echo ""
	@echo "Controller logs (last 10 lines):"
	@kubectl logs -n $(NOTIFICATION_NAMESPACE) deployment/notification-controller --tail=10 || true
	@echo ""
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "✅ NOTIFICATION SERVICE SETUP COMPLETE"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo ""
	@echo "📊 Deployment Status:"
	@echo "  • Kind Cluster: $(NOTIFICATION_CLUSTER)"
	@echo "  • Namespace: $(NOTIFICATION_NAMESPACE)"
	@echo "  • CRD: NotificationRequest.notification.kubernaut.ai"
	@echo "  • Controller: notification-controller"
	@echo ""
	@echo "🧪 Ready to run integration tests:"
	@echo "  make test-integration-notification"
	@echo ""

.PHONY: test-notification-teardown
test-notification-teardown: ## Teardown Notification controller (keeps Kind cluster)
	@echo "🧹 Cleaning up Notification controller deployment..."
	@kubectl delete -k deploy/notification/ --ignore-not-found=true
	@kubectl delete crd notificationrequests.notification.kubernaut.ai --ignore-not-found=true
	@echo "✅ Notification controller cleanup complete"
	@echo "💡 Tip: To delete Kind cluster, run: kind delete cluster --name $(NOTIFICATION_CLUSTER)"

.PHONY: test-notification-teardown-full
test-notification-teardown-full: ## Complete teardown including Kind cluster
	@echo "🧹 Full cleanup: Notification controller + Kind cluster..."
	@$(MAKE) test-notification-teardown
	@kind delete cluster --name $(NOTIFICATION_CLUSTER) 2>/dev/null || true
	@echo "✅ Full cleanup complete"

.PHONY: test-integration-notification
test-integration-notification: ## Run Notification Service integration tests (Kind cluster, ~3-5min)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🧪 Notification Service Integration Tests"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo ""
	@echo "📋 Test Scenarios:"
	@echo "  1. Basic CRD lifecycle (create → reconcile → complete)"
	@echo "  2. Delivery failure recovery (retry with exponential backoff)"
	@echo "  3. Graceful degradation (partial delivery success)"
	@echo ""
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo ""
	@echo "🔍 Checking deployment status..."
	@if ! kubectl get crd notificationrequests.notification.kubernaut.ai &> /dev/null; then \
		echo "⚠️  NotificationRequest CRD not found - running setup..."; \
		$(MAKE) test-notification-setup; \
	else \
		echo "✅ CRD already installed"; \
		if ! kubectl get deployment notification-controller -n $(NOTIFICATION_NAMESPACE) &> /dev/null; then \
			echo "⚠️  Controller not deployed - running setup..."; \
			$(MAKE) test-notification-setup; \
		else \
			echo "✅ Controller already deployed"; \
		fi; \
	fi
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "🧪 Running integration tests..."
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@go test ./test/integration/notification/... -v -ginkgo.v -timeout=30m
	@echo ""
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "✅ NOTIFICATION SERVICE INTEGRATION TESTS COMPLETE"
	@echo "════════════════════════════════════════════════════════════════════════"

##@ Service-Specific Integration Tests
# Per ADR-016: Service-Specific Integration Test Infrastructure
# Use Podman for database-only services, Kind for Kubernetes-dependent services

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
test-integration-toolset: ## Run Dynamic Toolset integration tests (Kind cluster, ~3-5min)
	@echo "🔧 Ensuring Kind cluster is running..."
	@./scripts/ensure-kind-cluster.sh
	@echo "🧪 Running Dynamic Toolset integration tests..."
	@go test ./test/integration/toolset/... -v -timeout 10m

.PHONY: test-integration-gateway-service
test-integration-gateway-service: ## Run Gateway Service integration tests (Kind cluster, uses existing test-gateway target)
	@echo "🔧 Running Gateway Service integration tests..."
	@$(MAKE) test-gateway

.PHONY: test-integration-service-all
test-integration-service-all: ## Run ALL service-specific integration tests (sequential)
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "🚀 Running ALL Service-Specific Integration Tests (per ADR-016)"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo ""
	@echo "📊 Test Plan:"
	@echo "  1. Data Storage (Podman: PostgreSQL + pgvector) - ~30s"
	@echo "  2. AI Service (Podman: Redis) - ~15s"
	@echo "  3. Dynamic Toolset (Kind: Kubernetes) - ~3-5min"
	@echo "  4. Gateway Service (Kind: Kubernetes) - ~3-5min"
	@echo "  5. Notification Service (Kind: Kubernetes + CRD) - ~3-5min"
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
build-all-services: build-gateway-service build-alert-service build-ai-analysis build-workflow-service build-executor-service build-storage-service build-intelligence-service build-monitor-service build-context-service build-notification-service ## Build all 10 approved microservices

.PHONY: build-microservices
build-microservices: build-all-services ## Build all microservices (alias for build-all-services)

.PHONY: build-gateway-service
build-gateway-service: ## Build gateway service (webhook functionality)
	@echo "🔨 Building gateway service..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o bin/gateway ./cmd/gateway

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
