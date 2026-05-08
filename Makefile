IMG ?= quay.io/kubernaut/apifrontend:latest
CONTROLLER_GEN ?= $(shell which controller-gen 2>/dev/null)
GINKGO ?= $(shell which ginkgo 2>/dev/null || echo "go run github.com/onsi/ginkgo/v2/ginkgo")
LOCALBIN ?= $(shell pwd)/bin
COVERPKGS = ./internal/auth/...,./internal/ratelimit/...,./internal/security/...,./internal/httputil/...,./internal/logging/...,./internal/requestid/...,./internal/audit/...,./internal/metrics/...,./internal/agent/...,./internal/tools/...,./internal/ka/...,./internal/ds/...,./internal/session/...,./internal/config/...,./internal/handler/...,./internal/launcher/...,./internal/resilience/...,./internal/streaming/...,./internal/controller/...

.PHONY: all
all: build

##@ Build

.PHONY: build
build: fmt vet
	go build -o bin/apifrontend ./cmd/apifrontend/

.PHONY: run
run: fmt vet
	go run ./cmd/apifrontend/

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: lint
lint:
	golangci-lint run ./...

##@ Test

.PHONY: test
test: test-unit

.PHONY: test-unit
test-unit: fmt vet
	$(GINKGO) -v --race --coverpkg=$(COVERPKGS) --coverprofile=cover.out ./internal/...

.PHONY: test-integration
test-integration:
	go test ./... -tags=integration -race -coverprofile cover-integration.out

.PHONY: test-all
test-all: test-unit test-integration

##@ Container

.PHONY: docker-build
docker-build:
	docker build -t $(IMG) .

.PHONY: docker-push
docker-push:
	docker push $(IMG)

##@ Generate

.PHONY: generate
generate: manifests
	$(CONTROLLER_GEN) object paths="./api/..."

.PHONY: manifests
manifests:
	$(CONTROLLER_GEN) rbac:roleName=apifrontend-role crd paths="./api/..." output:crd:artifacts:config=config/crd/bases

##@ Validate

.PHONY: verify-generate
verify-generate: generate
	git diff --exit-code ./api/ ./config/

.PHONY: validate-openapi
validate-openapi:
	@which vacuum >/dev/null 2>&1 || { echo "vacuum not found — install: go install github.com/daveshanley/vacuum@v0.26.4"; exit 1; }
	vacuum lint api/openapi/apifrontend-v1.yaml

.PHONY: clean
clean:
	rm -rf bin/ cover.out cover-integration.out
