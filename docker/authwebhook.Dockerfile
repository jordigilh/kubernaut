# AuthWebhook Service - Multi-Architecture Dockerfile (ADR-027)
# Supports: linux/amd64, linux/arm64
# Based on: ADR-027 (Multi-Architecture Build Strategy with Red Hat UBI)
#
# Build targets (Issue #80):
#   production:  scratch runtime -- zero CVE surface, no shell (release.yml)
#   development: ubi10-minimal runtime -- debug tools, coverage support (ci-pipeline.yml)
#
# Usage:
#   Production:  podman build --target production -t authwebhook:v1.0 -f docker/authwebhook.Dockerfile .
#   Development: podman build --build-arg GOFLAGS=-cover -t authwebhook:dev -f docker/authwebhook.Dockerfile .

# ============================================================================
# Stage 1: Build (native cross-compile, no QEMU needed for Go)
# ============================================================================
FROM registry.access.redhat.com/ubi10/go-toolset:1.25 AS builder

# Build arguments for multi-architecture support
ARG GOFLAGS=""
ARG GOOS=linux
ARG TARGETARCH
ARG GOARCH=${TARGETARCH}
ARG APP_VERSION=v1.2.0
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

# Switch to root for package installation
USER root

# Install build dependencies
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

# Build the Webhooks service binary
# DD-TEST-007: Coverage builds use simple flags (no -a, -installsuffix, -extldflags)
RUN if [ "${GOFLAGS}" = "-cover" ]; then \
	echo "Building with coverage instrumentation (no symbol stripping)..."; \
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} GOFLAGS=${GOFLAGS} go build \
	-mod=mod \
	-ldflags="-X github.com/jordigilh/kubernaut/internal/version.Version=${APP_VERSION} -X github.com/jordigilh/kubernaut/internal/version.GitCommit=${GIT_COMMIT} -X github.com/jordigilh/kubernaut/internal/version.BuildDate=${BUILD_DATE}" \
	-o authwebhook \
	./cmd/authwebhook/main.go; \
	else \
	echo "Building production binary (with symbol stripping)..."; \
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
	-mod=mod \
	-ldflags="-s -w -X github.com/jordigilh/kubernaut/internal/version.Version=${APP_VERSION} -X github.com/jordigilh/kubernaut/internal/version.GitCommit=${GIT_COMMIT} -X github.com/jordigilh/kubernaut/internal/version.BuildDate=${BUILD_DATE}" \
	-o authwebhook \
	./cmd/authwebhook/main.go; \
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
COPY --from=builder /opt/app-root/src/authwebhook /authwebhook
USER 65534
EXPOSE 9443
ENTRYPOINT ["/authwebhook"]
CMD []

ARG APP_VERSION=v1.2.0
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-authwebhook" \
	org.opencontainers.image.description="Kubernetes validating webhook for SubjectAccessReview-based authorization of Kubernaut API requests." \
	org.opencontainers.image.vendor="Kubernaut"
LABEL name="kubernaut-authwebhook" \
	vendor="Kubernaut" \
	summary="Kubernaut AuthWebhook Service - Kubernetes Admission Webhooks for SOC2 Attribution" \
	description="A microservice component of Kubernaut that provides Kubernetes admission webhooks for capturing authenticated user identity for operational decisions (SOC2 CC8.1 attribution). Handles WorkflowExecution block clearance, RemediationApprovalRequest approval/rejection, and NotificationRequest deletion attribution." \
	maintainer="jgil@redhat.com" \
	component="authwebhook" \
	part-of="kubernaut" \
	io.k8s.description="AuthWebhook Service for SOC2 CC8.1 operator attribution" \
	io.k8s.display-name="Kubernaut AuthWebhook Service" \
	io.openshift.tags="kubernaut,authwebhook,admission,authentication,soc2,audit,attribution,microservice"

# ============================================================================
# Stage 2b: Development/E2E runtime (ubi10-minimal -- debug + coverage, DD-TEST-007)
# Default stage when no --target is specified (backwards compatible with CI).
# ============================================================================
FROM registry.access.redhat.com/ubi10/ubi-minimal:latest AS development
RUN microdnf update -y && \
	microdnf install -y ca-certificates tzdata shadow-utils && \
	microdnf clean all
RUN useradd -r -u 1001 -g root webhooks-user
COPY --from=builder /opt/app-root/src/authwebhook /usr/local/bin/authwebhook
RUN chmod +x /usr/local/bin/authwebhook
USER 1001
EXPOSE 9443
ENTRYPOINT ["/usr/local/bin/authwebhook"]
CMD []

ARG APP_VERSION=v1.2.0
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-authwebhook" \
	org.opencontainers.image.description="Kubernetes validating webhook for SubjectAccessReview-based authorization of Kubernaut API requests." \
	org.opencontainers.image.vendor="Kubernaut"
