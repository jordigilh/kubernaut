# EffectivenessMonitor Controller Dockerfile
# Multi-stage build for minimal production image (ADR-027: UBI9 base images)
#
# Build: podman build -f docker/effectivenessmonitor-controller.Dockerfile -t kubernaut/effectivenessmonitor:latest .
# Run: podman run -p 9090:9090 -p 8081:8081 kubernaut/effectivenessmonitor:latest

# Stage 1: Build
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder

# Switch to root for setup
USER root

# Switch back to default user for security
USER 1001

# Set working directory (UBI9 go-toolset default)
WORKDIR /opt/app-root/src

# Copy go.mod and go.sum first for better caching
COPY --chown=1001:0 go.mod go.sum ./

# Copy source code
COPY --chown=1001:0 . .

# Build the binary
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown
# GOFLAGS: Optional build flags (e.g., -cover for E2E coverage profiling per E2E_COVERAGE_COLLECTION.md)
ARG GOFLAGS=""

# -mod=mod: Automatically download dependencies during build (per DD-BUILD-001)
# GOTOOLCHAIN=auto: Allow Go to download the required toolchain version (fixes go.mod version mismatch)
# When building with coverage (-cover), don't strip symbols (-s -w) as coverage needs them
RUN if [ "${GOFLAGS}" = "-cover" ]; then \
    echo "Building with coverage instrumentation (no symbol stripping)..."; \
    CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=auto GOFLAGS="${GOFLAGS}" go build \
    -mod=mod \
    -ldflags="-X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o effectivenessmonitor-controller ./cmd/effectivenessmonitor; \
    else \
    echo "Building production binary (with symbol stripping)..."; \
    CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=auto go build \
    -mod=mod \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o effectivenessmonitor-controller ./cmd/effectivenessmonitor; \
    fi

# Stage 2: Runtime
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

WORKDIR /

# CA certificates already included in UBI9 minimal

# Copy binary from builder
COPY --from=builder /opt/app-root/src/effectivenessmonitor-controller /effectivenessmonitor-controller

# Create non-root user
RUN useradd -r -u 65532 -g root nonroot
USER nonroot

# Expose ports
# 9090 - Prometheus metrics
# 8081 - Health probes (liveness/readiness)
EXPOSE 9090 8081

ENTRYPOINT ["/effectivenessmonitor-controller"]
