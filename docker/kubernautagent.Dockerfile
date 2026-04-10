# Kubernaut Agent Service - Multi-Architecture Dockerfile (ADR-027)
#
# Replaces holmesgpt-api (Python) with native Go implementation (#433).
#
# Build targets (Issue #80):
#   production:  scratch runtime -- zero CVE surface, no shell (release.yml)
#   development: ubi10-minimal runtime -- debug tools, coverage support (ci-pipeline.yml)
#
# Usage:
#   Production:  podman build --target production -t kubernautagent:v1.3 -f docker/kubernautagent.Dockerfile .
#   Development: podman build --build-arg GOFLAGS=-cover -t kubernautagent:dev -f docker/kubernautagent.Dockerfile .

# ============================================================================
# Stage 1: Build (native cross-compile, no QEMU needed for Go)
# ============================================================================
FROM registry.access.redhat.com/ubi10/go-toolset:1.25 AS builder

ARG TARGETARCH
ARG GOOS=linux
ARG GOARCH=${TARGETARCH:-amd64}
ARG GOFLAGS=""
ARG APP_VERSION=v1.2.0
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
	-o kubernautagent \
	./cmd/kubernautagent; \
	else \
	echo "Building production binary (with symbol stripping)..."; \
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
	-mod=mod \
	-ldflags="-s -w -X github.com/jordigilh/kubernaut/internal/version.Version=${APP_VERSION} -X github.com/jordigilh/kubernaut/internal/version.GitCommit=${GIT_COMMIT} -X github.com/jordigilh/kubernaut/internal/version.BuildDate=${BUILD_DATE}" \
	-o kubernautagent \
	./cmd/kubernautagent; \
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
COPY --from=builder /opt/app-root/src/kubernautagent /kubernautagent
USER 65534
EXPOSE 8080
ENTRYPOINT ["/kubernautagent"]

ARG APP_VERSION=v1.2.0
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-agent" \
	org.opencontainers.image.description="AI-powered incident investigation agent replacing holmesgpt-api with native Go, LangChainGo, and client-go toolset." \
	org.opencontainers.image.vendor="Kubernaut"
LABEL name="kubernaut-agent" \
	vendor="Kubernaut" \
	summary="Kubernaut Agent - AI Incident Investigation (Go)" \
	description="LLM-driven root cause analysis and workflow selection using LangChainGo, client-go K8s toolset, Prometheus tools, and DataStorage custom tools. Multi-architecture (amd64/arm64) per ADR-027." \
	maintainer="jgil@redhat.com" \
	component="kubernautagent" \
	part-of="kubernaut" \
	io.k8s.description="Kubernaut Agent for AI-driven incident investigation and workflow selection" \
	io.k8s.display-name="Kubernaut Agent Service" \
	io.openshift.tags="kubernaut,agent,ai,llm,langchaingo,investigation,rca,workflow,microservice"

# ============================================================================
# Stage 2b: Development/E2E runtime (ubi10-minimal -- debug + coverage, DD-TEST-007)
# Default stage when no --target is specified (backwards compatible with CI).
# ============================================================================
FROM registry.access.redhat.com/ubi10/ubi-minimal:latest AS development
RUN microdnf update -y && \
	microdnf install -y ca-certificates tzdata shadow-utils && \
	microdnf clean all
RUN useradd -r -u 1001 -g root kubernautagent-user
COPY --from=builder /opt/app-root/src/kubernautagent /usr/local/bin/kubernautagent
RUN chmod +x /usr/local/bin/kubernautagent
USER 1001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/kubernautagent"]

ARG APP_VERSION=v1.2.0
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-agent" \
	org.opencontainers.image.description="AI-powered incident investigation agent replacing holmesgpt-api with native Go, LangChainGo, and client-go toolset." \
	org.opencontainers.image.vendor="Kubernaut"
