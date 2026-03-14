# SignalProcessing Controller - Multi-Architecture Dockerfile using Red Hat UBI9 (ADR-027)
#
# Build targets (Issue #80):
#   production:  scratch runtime -- zero CVE surface, no shell (release.yml)
#   development: ubi10-minimal runtime -- debug tools, coverage support (ci-pipeline.yml)
#
# Usage:
#   Production:  podman build --target production -t signalprocessing:v1.0 -f docker/signalprocessing-controller.Dockerfile .
#   Development: podman build --build-arg GOFLAGS=-cover -t signalprocessing:dev -f docker/signalprocessing-controller.Dockerfile .

# ============================================================================
# Stage 1: Build
# ============================================================================
FROM registry.access.redhat.com/ubi10/go-toolset:1.25 AS builder

USER root
USER 1001

WORKDIR /opt/app-root/src
COPY --chown=1001:0 go.mod go.sum ./
COPY --chown=1001:0 . .

ARG APP_VERSION=v1.0.0
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
ARG GOFLAGS=""

# DD-TEST-007: Coverage builds use simple flags (no -a, -installsuffix, -extldflags)
RUN if [ "${GOFLAGS}" = "-cover" ]; then \
    echo "Building with coverage instrumentation (no symbol stripping)..."; \
    CGO_ENABLED=0 GOOS=linux GOFLAGS="${GOFLAGS}" go build \
    -mod=mod \
    -ldflags="-X github.com/jordigilh/kubernaut/internal/version.Version=${APP_VERSION} -X github.com/jordigilh/kubernaut/internal/version.GitCommit=${GIT_COMMIT} -X github.com/jordigilh/kubernaut/internal/version.BuildDate=${BUILD_DATE}" \
    -o signalprocessing-controller ./cmd/signalprocessing; \
    else \
    echo "Building production binary (with symbol stripping)..."; \
    CGO_ENABLED=0 GOOS=linux go build \
    -mod=mod \
    -ldflags="-s -w -X github.com/jordigilh/kubernaut/internal/version.Version=${APP_VERSION} -X github.com/jordigilh/kubernaut/internal/version.GitCommit=${GIT_COMMIT} -X github.com/jordigilh/kubernaut/internal/version.BuildDate=${BUILD_DATE}" \
    -o signalprocessing-controller ./cmd/signalprocessing; \
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
COPY --from=builder /opt/app-root/src/signalprocessing-controller /signalprocessing-controller
USER 65534
EXPOSE 9090 8081
ENTRYPOINT ["/signalprocessing-controller"]

LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
    org.opencontainers.image.version="${APP_VERSION}" \
    org.opencontainers.image.revision="${GIT_COMMIT}" \
    org.opencontainers.image.created="${BUILD_DATE}" \
    org.opencontainers.image.title="kubernaut-signalprocessing"

# ============================================================================
# Stage 2b: Development/E2E runtime (ubi10-minimal -- debug + coverage, DD-TEST-007)
# Default stage when no --target is specified (backwards compatible with CI).
# ============================================================================
FROM registry.access.redhat.com/ubi10/ubi-minimal:latest AS development
RUN microdnf update -y && \
    microdnf install -y ca-certificates tzdata && \
    microdnf clean all
RUN useradd -r -u 65532 -g root nonroot
COPY --from=builder /opt/app-root/src/signalprocessing-controller /usr/local/bin/signalprocessing-controller
RUN chmod +x /usr/local/bin/signalprocessing-controller
USER 65532
EXPOSE 9090 8081
ENTRYPOINT ["/usr/local/bin/signalprocessing-controller"]

LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
    org.opencontainers.image.version="${APP_VERSION}" \
    org.opencontainers.image.revision="${GIT_COMMIT}" \
    org.opencontainers.image.created="${BUILD_DATE}" \
    org.opencontainers.image.title="kubernaut-signalprocessing"
