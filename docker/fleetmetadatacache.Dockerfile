# Fleet Metadata Cache - Multi-Architecture Dockerfile (ADR-027)
#
# Build targets (Issue #80):
#   production:  scratch runtime -- zero CVE surface, no shell (release.yml)
#   development: ubi10-minimal runtime -- debug tools, coverage support (ci-pipeline.yml)
#
# Usage:
#   Production:  podman build --target production -t fleetmetadatacache:v1.0 -f docker/fleetmetadatacache.Dockerfile .
#   Development: podman build --build-arg GOFLAGS=-cover -t fleetmetadatacache:dev -f docker/fleetmetadatacache.Dockerfile .

ARG BUILDER_IMAGE=registry.access.redhat.com/ubi10/go-toolset:1.26@sha256:ad1d5e19331fc80c28a6193c1f8489af93b8f54d06766f174de6d4ce1ec6a191
ARG BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-minimal:latest@sha256:b217fa65d8c21058887b18f005f587e47a17dd1281a5196ac88d01724a273dbd

# ============================================================================
# Stage 1: Build (native cross-compile, no QEMU needed for Go)
# ============================================================================
# SECURITY: BUILDER_IMAGE above is pinned by digest. Dependabot (docker
# ecosystem, .github/dependabot.yml) re-resolves this digest weekly.
FROM ${BUILDER_IMAGE} AS builder
ENV GOTOOLCHAIN=auto

ARG TARGETARCH
ARG GOOS=linux
ARG GOARCH=${TARGETARCH}
ARG GOFLAGS=""
ARG APP_VERSION=v1.6.0
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
	-o fleetmetadatacache ./cmd/fleetmetadatacache; \
	else \
	echo "Building production binary (with symbol stripping)..."; \
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
	-mod=mod \
	-ldflags="-s -w -X github.com/jordigilh/kubernaut/internal/version.Version=${APP_VERSION} -X github.com/jordigilh/kubernaut/internal/version.GitCommit=${GIT_COMMIT} -X github.com/jordigilh/kubernaut/internal/version.BuildDate=${BUILD_DATE}" \
	-o fleetmetadatacache ./cmd/fleetmetadatacache; \
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
COPY --from=builder /opt/app-root/src/fleetmetadatacache /fleetmetadatacache
USER 65534
EXPOSE 8080 8081
ENTRYPOINT ["/fleetmetadatacache"]

ARG APP_VERSION
ARG GIT_COMMIT
ARG BUILD_DATE

LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
    org.opencontainers.image.version="${APP_VERSION}" \
    org.opencontainers.image.revision="${GIT_COMMIT}" \
    org.opencontainers.image.created="${BUILD_DATE}" \
    org.opencontainers.image.title="kubernaut-fleetmetadatacache" \
    org.opencontainers.image.description="Fleet Metadata Cache service that polls MCP Gateway for managed cluster resources, caches metadata in Valkey, and exposes scope queries via REST API." \
    org.opencontainers.image.vendor="Kubernaut"

LABEL name="kubernaut-fleetmetadatacache" \
	vendor="Kubernaut" \
	summary="Kubernaut Fleet Metadata Cache" \
	description="Fleet Metadata Cache service that polls MCP Gateway for managed cluster resources, caches metadata in Valkey, and exposes scope queries via REST API for federated scope checking." \
	maintainer="jgil@redhat.com" \
	component="fleetmetadatacache" \
	part-of="kubernaut" \
	io.k8s.description="Fleet Metadata Cache for Kubernaut" \
	io.k8s.display-name="Kubernaut Fleet Metadata Cache" \
	io.openshift.tags="kubernaut,fleetmetadatacache,fleet,metadata,cache,multi-cluster,microservice"

# ============================================================================
# Stage 2b: Development/E2E runtime (ubi10-minimal -- debug + coverage, DD-TEST-007)
# Default stage when no --target is specified (backwards compatible with CI).
# ============================================================================
# SECURITY: BASE_IMAGE above is pinned by digest. Dependabot (docker
# ecosystem, .github/dependabot.yml) re-resolves this digest weekly.
FROM ${BASE_IMAGE} AS development
RUN microdnf update -y && \
    microdnf install -y ca-certificates tzdata shadow-utils && \
    microdnf clean all
RUN useradd -r -u 1001 -g root fleetmetadatacache-user
COPY --from=builder /opt/app-root/src/fleetmetadatacache /usr/local/bin/fleetmetadatacache
RUN chmod +x /usr/local/bin/fleetmetadatacache
USER 1001
EXPOSE 8080 8081
ENTRYPOINT ["/usr/local/bin/fleetmetadatacache"]

ARG APP_VERSION
ARG GIT_COMMIT
ARG BUILD_DATE

LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
    org.opencontainers.image.version="${APP_VERSION}" \
    org.opencontainers.image.revision="${GIT_COMMIT}" \
    org.opencontainers.image.created="${BUILD_DATE}" \
    org.opencontainers.image.title="kubernaut-fleetmetadatacache" \
    org.opencontainers.image.description="Fleet Metadata Cache service that polls MCP Gateway for managed cluster resources, caches metadata in Valkey, and exposes scope queries via REST API." \
    org.opencontainers.image.vendor="Kubernaut"
