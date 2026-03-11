# WorkflowExecution Controller - Multi-Architecture Dockerfile using Red Hat UBI9 (ADR-027)
#
# Build targets (Issue #80):
#   production:  scratch runtime -- zero CVE surface, no shell (release.yml)
#   development: ubi9-minimal runtime -- debug tools, coverage support (ci-pipeline.yml)
#
# Usage:
#   Production:  podman build --target production -t workflowexecution:v1.0 -f docker/workflowexecution-controller.Dockerfile .
#   Development: podman build --build-arg GOFLAGS=-cover -t workflowexecution:dev -f docker/workflowexecution-controller.Dockerfile .

# ============================================================================
# Stage 1: Build (native cross-compile, no QEMU needed for Go)
# ============================================================================
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder

ARG GOFLAGS=""
ARG GOOS=linux
ARG GOARCH=amd64
ARG APP_VERSION=v1.0.0
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

USER root
RUN dnf update -y && \
	dnf install -y git ca-certificates tzdata && \
	dnf clean all
USER 1001

WORKDIR /opt/app-root/src
COPY --chown=1001:0 go.mod go.sum ./
COPY --chown=1001:0 . .

# DD-TEST-007: Coverage builds use simple flags (no -a, -installsuffix, -extldflags)
RUN if [ "${GOFLAGS}" = "-cover" ]; then \
	echo "Building with E2E coverage instrumentation (DD-TEST-007)..."; \
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} GOFLAGS=${GOFLAGS} go build \
	-mod=mod \
	-ldflags="-X github.com/jordigilh/kubernaut/internal/version.Version=${APP_VERSION} -X github.com/jordigilh/kubernaut/internal/version.GitCommit=${GIT_COMMIT} -X github.com/jordigilh/kubernaut/internal/version.BuildDate=${BUILD_DATE}" \
	-o workflowexecution \
	./cmd/workflowexecution; \
	else \
	echo "Production build with optimizations..."; \
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
	-mod=mod \
	-ldflags="-w -s -extldflags '-static' -X github.com/jordigilh/kubernaut/internal/version.Version=${APP_VERSION} -X github.com/jordigilh/kubernaut/internal/version.GitCommit=${GIT_COMMIT} -X github.com/jordigilh/kubernaut/internal/version.BuildDate=${BUILD_DATE}" \
	-a -installsuffix cgo \
	-o workflowexecution \
	./cmd/workflowexecution; \
	fi

# ============================================================================
# Stage 2a: Production runtime (scratch -- zero CVE surface, Issue #80)
# Trust chain artifacts (CA certs, timezone, passwd) copied from builder which
# installs ca-certificates and tzdata via dnf.
# ============================================================================
FROM scratch AS production
COPY --from=builder /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /opt/app-root/src/workflowexecution /workflowexecution
USER nobody
EXPOSE 8080 9090 8081
ENTRYPOINT ["/workflowexecution"]

LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-workflowexecution"
LABEL name="kubernaut-workflowexecution" \
	vendor="Kubernaut" \
	summary="Kubernaut WorkflowExecution Controller - Kubernetes CRD Controller" \
	description="Manages WorkflowExecution CRDs, handles workflow execution lifecycle, integrates with Tekton Pipelines, Kubernetes Jobs, and Ansible AAP/AWX, and manages workflow state transitions with fault isolation." \
	maintainer="jgil@redhat.com" \
	component="workflowexecution-controller" \
	part-of="kubernaut" \
	io.k8s.description="Kubernaut WorkflowExecution Controller for Kubernetes workflow execution" \
	io.k8s.display-name="Kubernaut WorkflowExecution Controller" \
	io.openshift.tags="kubernaut,kubernetes,controller,workflow,execution,tekton,ansible"

# ============================================================================
# Stage 2b: Development/E2E runtime (ubi9-minimal -- debug + coverage, DD-TEST-007)
# Default stage when no --target is specified (backwards compatible with CI).
# ============================================================================
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest AS development
RUN microdnf update -y && \
	microdnf install -y ca-certificates tzdata && \
	microdnf clean all
RUN useradd -r -u 1001 -g root workflowexecution-user
COPY --from=builder /opt/app-root/src/workflowexecution /usr/local/bin/workflowexecution
RUN chmod +x /usr/local/bin/workflowexecution
USER workflowexecution-user
EXPOSE 8080 9090 8081
ENTRYPOINT ["/usr/local/bin/workflowexecution"]

LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-workflowexecution"
