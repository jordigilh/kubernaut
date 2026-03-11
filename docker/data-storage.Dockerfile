# Data Storage Service - Multi-Architecture Dockerfile using Red Hat UBI9
# Supports: linux/amd64, linux/arm64
# Based on: ADR-027 (Multi-Architecture Build Strategy with Red Hat UBI)
#
# Build targets (Issue #80):
#   production:  scratch runtime -- zero CVE surface, no shell (release.yml)
#   development: ubi9-minimal runtime -- debug tools, coverage support (ci-pipeline.yml)
#
# Usage:
#   Production:  podman build --target production -t data-storage:v1.0 -f docker/data-storage.Dockerfile .
#   Development: podman build --build-arg GOFLAGS=-cover -t data-storage:dev -f docker/data-storage.Dockerfile .

# ============================================================================
# Stage 1: Build (native cross-compile, no QEMU needed for Go)
# ============================================================================
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder

# Auto-detect target architecture from --platform flag
# Podman/Docker automatically set TARGETARCH when --platform is specified
ARG TARGETARCH
ARG GOOS=linux
# Use TARGETARCH if set (multi-arch build), otherwise detect from runtime
ARG GOARCH=${TARGETARCH:-amd64}
# Support coverage profiling for E2E tests (E2E_COVERAGE_COLLECTION.md)
ARG GOFLAGS=""
ARG APP_VERSION=v1.0.0
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

# Switch to root for package installation
USER root

# Install build dependencies (dnf update required for security compliance)
RUN dnf update -y && \
	dnf install -y git ca-certificates tzdata && \
	dnf clean all

# Switch back to default user for security
USER 1001

# Set working directory
WORKDIR /opt/app-root/src

# Copy go mod files
COPY --chown=1001:0 go.mod go.sum ./

# Copy source code
COPY --chown=1001:0 . .

# Build the Data Storage service binary
# CGO_ENABLED=0 for static linking (no C dependencies)
# Uses pgx pure-Go PostgreSQL driver (not lib/pq which requires CGO)
# GOOS and GOARCH from build args for multi-architecture support
# GOFLAGS can include -cover for E2E coverage profiling (E2E_COVERAGE_COLLECTION.md)
# -mod=mod: Automatically download dependencies during build (no separate go mod download step)
#
# DD-TEST-007: Coverage build uses SIMPLE flags (per SP team guidance)
# - Coverage: No -ldflags, -a, or -installsuffix (breaks coverage instrumentation)
# - Production: Keep all optimizations for size/performance
# NOTE: vendor/ excluded in .dockerignore, so we use -mod=mod
# Toolchain pinned to go1.25.3 in go.mod to match UBI9 go-toolset:1.25
RUN if [ "${GOFLAGS}" = "-cover" ]; then \
	echo "Building with coverage instrumentation (simple build per DD-TEST-007)..."; \
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} GOFLAGS=${GOFLAGS} go build \
	-mod=mod \
	-ldflags="-X github.com/jordigilh/kubernaut/internal/version.Version=${APP_VERSION} -X github.com/jordigilh/kubernaut/internal/version.GitCommit=${GIT_COMMIT} -X github.com/jordigilh/kubernaut/internal/version.BuildDate=${BUILD_DATE}" \
	-o data-storage \
	./cmd/datastorage/main.go; \
	else \
	echo "Building production binary (with symbol stripping)..."; \
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
	-mod=mod \
	-ldflags="-w -s -extldflags '-static' -X github.com/jordigilh/kubernaut/internal/version.Version=${APP_VERSION} -X github.com/jordigilh/kubernaut/internal/version.GitCommit=${GIT_COMMIT} -X github.com/jordigilh/kubernaut/internal/version.BuildDate=${BUILD_DATE}" \
	-a -installsuffix cgo \
	-o data-storage \
	./cmd/datastorage/main.go; \
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
COPY --from=builder /opt/app-root/src/data-storage /data-storage
COPY --from=builder /opt/app-root/src/api/openapi/data-storage-v1.yaml /usr/local/share/kubernaut/api/openapi/data-storage-v1.yaml
USER nobody
EXPOSE 8080 9090
ENTRYPOINT ["/data-storage"]
CMD []

LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-datastorage"
LABEL name="kubernaut-data-storage" \
	vendor="Kubernaut" \
	summary="Kubernaut Data Storage Service - Audit Trail Persistence" \
	description="A microservice component of Kubernaut that provides persistent storage for remediation audit trails, dual-write to PostgreSQL and vector databases, with pgvector integration for semantic search capabilities." \
	maintainer="jgil@redhat.com" \
	component="data-storage" \
	part-of="kubernaut" \
	io.k8s.description="Data Storage Service for audit trail persistence and vector search" \
	io.k8s.display-name="Kubernaut Data Storage Service" \
	io.openshift.tags="kubernaut,data-storage,audit,postgres,pgvector,database,persistence,microservice"

# ============================================================================
# Stage 2b: Development/E2E runtime (ubi9-minimal -- debug + coverage, DD-TEST-007)
# Default stage when no --target is specified (backwards compatible with CI).
# ============================================================================
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest AS development
RUN microdnf update -y && \
	microdnf install -y ca-certificates tzdata && \
	microdnf clean all
RUN useradd -r -u 1001 -g root data-storage-user
COPY --from=builder /opt/app-root/src/data-storage /usr/local/bin/data-storage
COPY --from=builder /opt/app-root/src/api/openapi/data-storage-v1.yaml /usr/local/share/kubernaut/api/openapi/data-storage-v1.yaml
RUN chmod +x /usr/local/bin/data-storage
USER data-storage-user
EXPOSE 8080 9090
ENTRYPOINT ["/usr/local/bin/data-storage"]
CMD []

LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-datastorage"
