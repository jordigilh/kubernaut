# API Frontend - Multi-Architecture Dockerfile (ADR-027)
#
# Build targets:
#   production:  scratch runtime -- zero CVE surface, no shell
#   development: ubi10-minimal runtime -- debug tools, coverage support (DD-TEST-007)
#
# Usage:
#   Production:  podman build --target production -t apifrontend:v1.0 .
#   Development: podman build --build-arg GOFLAGS=-cover -t apifrontend:dev .

# ============================================================================
# Stage 1: Build (native cross-compile, no QEMU needed for Go)
# ============================================================================
FROM registry.access.redhat.com/ubi10/go-toolset:1.25 AS builder

ARG TARGETARCH
ARG GOOS=linux
ARG GOARCH=${TARGETARCH}
ARG GOFLAGS=""
ENV GOTOOLCHAIN=auto
ARG APP_VERSION=v0.1.0
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

USER root
RUN dnf install -y git ca-certificates tzdata && \
	dnf clean all
USER 1001

WORKDIR /opt/app-root/src
COPY --chown=1001:0 go.mod go.sum ./
RUN go mod download
COPY --chown=1001:0 . .

RUN if [ "${GOFLAGS}" = "-cover" ]; then \
	echo "Building with coverage instrumentation..."; \
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} GOFLAGS=${GOFLAGS} go build \
	-mod=mod \
	-ldflags="-X main.Version=${APP_VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildDate=${BUILD_DATE}" \
	-o apifrontend \
	./cmd/apifrontend; \
	else \
	echo "Building production binary with FIPS (boringcrypto)..."; \
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} GOEXPERIMENT=boringcrypto go build \
	-mod=mod \
	-ldflags="-s -w -X main.Version=${APP_VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildDate=${BUILD_DATE}" \
	-o apifrontend \
	./cmd/apifrontend; \
	fi

# ============================================================================
# Stage 2a: Production runtime (scratch -- zero CVE surface)
# ============================================================================
FROM scratch AS production
COPY --from=builder /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /opt/app-root/src/apifrontend /apifrontend
USER 65534
EXPOSE 8443 8081 9090
ENTRYPOINT ["/apifrontend"]

ARG APP_VERSION=v0.1.0
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut-apifrontend" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-apifrontend" \
	org.opencontainers.image.description="MCP Streamable HTTP + A2A API Frontend for Kubernaut AI-driven remediation platform." \
	org.opencontainers.image.vendor="Kubernaut"
LABEL name="kubernaut-apifrontend" \
	vendor="Kubernaut" \
	summary="Kubernaut API Frontend - MCP Tool Bridge & A2A Protocol" \
	description="API Frontend serving MCP Streamable HTTP for AI agent tool dispatch with RBAC, audit, and metrics. Multi-architecture (amd64/arm64) per ADR-027." \
	maintainer="jgil@redhat.com" \
	component="apifrontend" \
	part-of="kubernaut" \
	io.k8s.description="API Frontend for MCP tool dispatch, RBAC enforcement, and A2A protocol" \
	io.k8s.display-name="Kubernaut API Frontend" \
	io.openshift.tags="kubernaut,apifrontend,mcp,a2a,ai-agent,microservice"

# ============================================================================
# Stage 2b: Development/E2E runtime (ubi10-minimal -- debug + coverage, DD-TEST-007)
# Default stage when no --target is specified.
# ============================================================================
FROM registry.access.redhat.com/ubi10/ubi-minimal:latest AS development
RUN microdnf install -y ca-certificates tzdata shadow-utils && \
	microdnf clean all
RUN useradd -r -u 1001 -g root apifrontend-user
COPY --from=builder /opt/app-root/src/apifrontend /usr/local/bin/apifrontend
RUN chmod +x /usr/local/bin/apifrontend
USER 1001
EXPOSE 8443 8081 9090
ENTRYPOINT ["/usr/local/bin/apifrontend"]

ARG APP_VERSION=v0.1.0
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut-apifrontend" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-apifrontend" \
	org.opencontainers.image.description="MCP Streamable HTTP + A2A API Frontend for Kubernaut AI-driven remediation platform." \
	org.opencontainers.image.vendor="Kubernaut"
