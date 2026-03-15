# Gateway Service - Multi-Architecture Dockerfile using Red Hat UBI9
# Supports: linux/amd64, linux/arm64
# Based on: ADR-027 (Multi-Architecture Build Strategy with Red Hat UBI)
#
# Build targets (Issue #80):
#   production:  scratch runtime -- zero CVE surface, no shell (release.yml)
#   development: ubi10-minimal runtime -- debug tools, coverage support (ci-pipeline.yml)
#
# Usage:
#   Production:  podman build --target production -t gateway:v1.0 -f docker/gateway.Dockerfile .
#   Development: podman build --build-arg GOFLAGS=-cover -t gateway:dev -f docker/gateway.Dockerfile .

# ============================================================================
# Stage 1: Build (native cross-compile, no QEMU needed for Go)
# ============================================================================
FROM registry.access.redhat.com/ubi10/go-toolset:1.25 AS builder

ARG TARGETOS=linux
ARG TARGETARCH
ARG APP_VERSION=v1.1.0-rc0
ARG GIT_COMMIT=dev
ARG BUILD_DATE=dev
# GOFLAGS: -cover enables E2E coverage instrumentation (DD-TEST-007)
ARG GOFLAGS=""

USER root
RUN dnf update -y && \
	dnf install -y git ca-certificates tzdata && \
	dnf clean all
USER 1001

WORKDIR /opt/app-root/src
COPY --chown=1001:0 go.mod go.sum ./
COPY --chown=1001:0 . .

# DD-TEST-007: Coverage builds use simple flags (no -a, -installsuffix, -extldflags)
# Symbol stripping (-s -w) is incompatible with Go's binary coverage instrumentation
RUN if [ "${GOFLAGS}" = "-cover" ]; then \
	echo "Building with coverage instrumentation (no symbol stripping)..."; \
	CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOFLAGS="${GOFLAGS}" go build \
	-mod=mod \
	-ldflags="-X github.com/jordigilh/kubernaut/internal/version.Version=${APP_VERSION} -X github.com/jordigilh/kubernaut/internal/version.GitCommit=${GIT_COMMIT} -X github.com/jordigilh/kubernaut/internal/version.BuildDate=${BUILD_DATE}" \
	-o gateway \
	./cmd/gateway; \
	else \
	echo "Building production binary (with symbol stripping)..."; \
	CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
	-mod=mod \
	-ldflags="-w -s -extldflags '-static' -X github.com/jordigilh/kubernaut/internal/version.Version=${APP_VERSION} -X github.com/jordigilh/kubernaut/internal/version.GitCommit=${GIT_COMMIT} -X github.com/jordigilh/kubernaut/internal/version.BuildDate=${BUILD_DATE}" \
	-a -installsuffix cgo \
	-o gateway \
	./cmd/gateway; \
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
COPY --from=builder /opt/app-root/src/gateway /gateway
USER 65534
EXPOSE 8080 9090 8081
ENTRYPOINT ["/gateway"]

LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-gateway"
LABEL name="kubernaut-gateway" \
	vendor="Kubernaut" \
	summary="Kubernaut Gateway Service - Signal Ingestion & Processing" \
	description="Signal ingestion from Prometheus AlertManager and Kubernetes Events with deduplication, storm detection, priority assignment, and CRD creation. Multi-architecture (amd64/arm64) per ADR-027." \
	maintainer="jgil@redhat.com" \
	component="gateway" \
	part-of="kubernaut" \
	io.k8s.description="Gateway Service for signal ingestion, deduplication, and CRD creation" \
	io.k8s.display-name="Kubernaut Gateway Service" \
	io.openshift.tags="kubernaut,gateway,webhook,signal,alertmanager,prometheus,kubernetes-events,microservice"

# ============================================================================
# Stage 2b: Development/E2E runtime (ubi10-minimal -- debug + coverage, DD-TEST-007)
# Default stage when no --target is specified (backwards compatible with CI).
# ============================================================================
FROM registry.access.redhat.com/ubi10/ubi-minimal:latest AS development
RUN microdnf update -y && \
	microdnf install -y ca-certificates tzdata shadow-utils && \
	microdnf clean all
RUN useradd -r -u 1001 -g root gateway-user
COPY --from=builder /opt/app-root/src/gateway /usr/local/bin/gateway
RUN chmod +x /usr/local/bin/gateway
USER 1001
EXPOSE 8080 9090 8081
ENTRYPOINT ["/usr/local/bin/gateway"]

LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-gateway"
