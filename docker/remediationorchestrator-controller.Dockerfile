# RemediationOrchestrator Controller - Multi-Architecture Dockerfile (ADR-027)
#
# Build targets (Issue #80):
#   production:  scratch runtime -- zero CVE surface, no shell (release.yml)
#   development: ubi10-minimal runtime -- debug tools, coverage support (ci-pipeline.yml)
#
# Usage:
#   Production:  podman build --target production -t remediationorchestrator:v1.0 -f docker/remediationorchestrator-controller.Dockerfile .
#   Development: podman build --build-arg GOFLAGS=-cover -t remediationorchestrator:dev -f docker/remediationorchestrator-controller.Dockerfile .

# ============================================================================
# Stage 1: Build (native cross-compile, no QEMU needed for Go)
# ============================================================================
FROM registry.access.redhat.com/ubi10/go-toolset:1.25 AS builder

ARG TARGETARCH
ARG GOOS=linux
ARG GOARCH=${TARGETARCH:-amd64}
ARG GOFLAGS=""
ARG APP_VERSION=v1.1.0-rc6
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
	echo "Building with coverage instrumentation (no symbol stripping)..."; \
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} GOFLAGS=${GOFLAGS} go build \
	-mod=mod \
	-ldflags="-X github.com/jordigilh/kubernaut/internal/version.Version=${APP_VERSION} -X github.com/jordigilh/kubernaut/internal/version.GitCommit=${GIT_COMMIT} -X github.com/jordigilh/kubernaut/internal/version.BuildDate=${BUILD_DATE}" \
	-o remediationorchestrator-controller ./cmd/remediationorchestrator; \
	else \
	echo "Building production binary (with symbol stripping)..."; \
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
	-mod=mod \
	-ldflags="-s -w -X github.com/jordigilh/kubernaut/internal/version.Version=${APP_VERSION} -X github.com/jordigilh/kubernaut/internal/version.GitCommit=${GIT_COMMIT} -X github.com/jordigilh/kubernaut/internal/version.BuildDate=${BUILD_DATE}" \
	-o remediationorchestrator-controller ./cmd/remediationorchestrator; \
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
COPY --from=builder /opt/app-root/src/remediationorchestrator-controller /remediationorchestrator-controller
USER 65534
EXPOSE 9090 8081
ENTRYPOINT ["/remediationorchestrator-controller"]

ARG APP_VERSION
ARG GIT_COMMIT
ARG BUILD_DATE

LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-remediationorchestrator" \
	org.opencontainers.image.description="Core orchestration controller that manages the remediation lifecycle from signal detection through workflow execution and effectiveness assessment." \
	org.opencontainers.image.vendor="Kubernaut"

LABEL name="kubernaut-remediationorchestrator" \
	vendor="Kubernaut" \
	summary="Kubernaut Remediation Orchestrator Controller" \
	description="Core orchestration controller that manages the remediation lifecycle from signal detection through workflow execution and effectiveness assessment." \
	maintainer="jgil@redhat.com" \
	component="remediationorchestrator-controller" \
	part-of="kubernaut" \
	io.k8s.description="Remediation Orchestrator Controller for Kubernaut" \
	io.k8s.display-name="Kubernaut Remediation Orchestrator" \
	io.openshift.tags="kubernaut,remediationorchestrator,controller,orchestration,remediation,microservice"

# ============================================================================
# Stage 2b: Development/E2E runtime (ubi10-minimal -- debug + coverage, DD-TEST-007)
# Default stage when no --target is specified (backwards compatible with CI).
# ============================================================================
FROM registry.access.redhat.com/ubi10/ubi-minimal:latest AS development
RUN microdnf update -y && \
	microdnf install -y ca-certificates tzdata shadow-utils && \
	microdnf clean all
RUN useradd -r -u 1001 -g root remediationorchestrator-user
COPY --from=builder /opt/app-root/src/remediationorchestrator-controller /usr/local/bin/remediationorchestrator-controller
RUN chmod +x /usr/local/bin/remediationorchestrator-controller
USER 1001
EXPOSE 9090 8081
ENTRYPOINT ["/usr/local/bin/remediationorchestrator-controller"]

ARG APP_VERSION
ARG GIT_COMMIT
ARG BUILD_DATE

LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-remediationorchestrator" \
	org.opencontainers.image.description="Core orchestration controller that manages the remediation lifecycle from signal detection through workflow execution and effectiveness assessment." \
	org.opencontainers.image.vendor="Kubernaut"
