# Notification Controller - Multi-Architecture Dockerfile using Red Hat UBI9 (ADR-027)
#
# Build targets (Issue #80):
#   production:  scratch runtime -- zero CVE surface, no shell (release.yml)
#   development: ubi9-minimal runtime -- debug tools, coverage support (ci-pipeline.yml)
#
# Usage:
#   Production:  podman build --target production -t notification:v1.0 -f docker/notification-controller-ubi9.Dockerfile .
#   Development: podman build --build-arg GOFLAGS=-cover -t notification:dev -f docker/notification-controller-ubi9.Dockerfile .

# ============================================================================
# Stage 1: Build (native cross-compile, no QEMU needed for Go)
# ============================================================================
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder

ARG GOFLAGS=""
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
# GOARCH set automatically by podman's --platform flag
RUN if [ "${GOFLAGS}" = "-cover" ]; then \
	echo "Building with coverage instrumentation (no symbol stripping)..."; \
	CGO_ENABLED=0 GOOS=linux GOFLAGS="${GOFLAGS}" go build \
	-mod=mod \
	-ldflags="-X github.com/jordigilh/kubernaut/internal/version.Version=${APP_VERSION} -X github.com/jordigilh/kubernaut/internal/version.GitCommit=${GIT_COMMIT} -X github.com/jordigilh/kubernaut/internal/version.BuildDate=${BUILD_DATE}" \
	-o manager \
	./cmd/notification/main.go; \
    else \
	CGO_ENABLED=0 GOOS=linux go build \
	-mod=mod \
	-ldflags="-w -s -extldflags '-static' -X github.com/jordigilh/kubernaut/internal/version.Version=${APP_VERSION} -X github.com/jordigilh/kubernaut/internal/version.GitCommit=${GIT_COMMIT} -X github.com/jordigilh/kubernaut/internal/version.BuildDate=${BUILD_DATE}" \
	-a -installsuffix cgo \
	-o manager \
	./cmd/notification/main.go; \
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
COPY --from=builder /opt/app-root/src/manager /manager
USER nobody
EXPOSE 8080 8081
ENTRYPOINT ["/manager"]
CMD []

LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-notification"
LABEL name="kubernaut-notification-controller" \
	vendor="Kubernaut" \
	summary="Kubernaut Notification Controller - CRD-based Notification Management" \
	description="Manages NotificationRequest custom resources for delivering notifications to multiple channels (Console, Slack) with automatic retry, exponential backoff, and at-least-once delivery guarantees." \
	maintainer="jgil@redhat.com" \
	component="notification-controller" \
	part-of="kubernaut" \
	io.k8s.description="Notification Controller for Kubernetes-native notification delivery" \
	io.k8s.display-name="Kubernaut Notification Controller" \
	io.openshift.tags="kubernaut,notification,controller,crd,kubernetes,slack,console"

# ============================================================================
# Stage 2b: Development/E2E runtime (ubi9-minimal -- debug + coverage, DD-TEST-007)
# Default stage when no --target is specified (backwards compatible with CI).
# ============================================================================
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest AS development
RUN microdnf update -y && \
	microdnf install -y ca-certificates tzdata && \
	microdnf clean all
RUN useradd -r -u 1001 -g root notification-controller-user
COPY --from=builder /opt/app-root/src/manager /usr/local/bin/manager
RUN chmod +x /usr/local/bin/manager
USER notification-controller-user
EXPOSE 8080 8081
ENTRYPOINT ["/usr/local/bin/manager"]
CMD []

LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-notification"
