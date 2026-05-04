IMG ?= quay.io/kubernaut/apifrontend:latest
CONTROLLER_GEN ?= $(shell which controller-gen 2>/dev/null)
LOCALBIN ?= $(shell pwd)/bin

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

.PHONY: test
test: fmt vet
	go test ./... -coverprofile cover.out

.PHONY: test-integration
test-integration:
	go test ./... -tags=integration -coverprofile cover-integration.out

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

.PHONY: clean
clean:
	rm -rf bin/ cover.out cover-integration.out
