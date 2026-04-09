# Mock LLM Service - Go Rewrite (#531)
#
# Multi-stage build following the gateway.Dockerfile pattern.
# Replaces the Python-based Mock LLM with a Go binary.
#
# Build targets:
#   production:  scratch runtime -- minimal image (<15MB), zero CVE surface
#   development: ubi10-minimal -- debug tools, coverage support
#
# Usage:
#   Production:  podman build --target production -t mock-llm:latest -f test/services/mock-llm/Dockerfile.go .
#   Development: podman build --build-arg GOFLAGS=-cover -t mock-llm:dev -f test/services/mock-llm/Dockerfile.go .
#
# Contract (drop-in replacement for Python Mock LLM):
#   - Port: 8080
#   - UID: 1001 (non-root)
#   - Health: GET /health → 200
#   - OpenAI: POST /v1/chat/completions
#   - Ollama: POST /api/chat, POST /api/generate

# ============================================================================
# Stage 1: Build
# ============================================================================
FROM registry.access.redhat.com/ubi10/go-toolset:1.25 AS builder

ARG TARGETARCH
ARG GOOS=linux
ARG GOARCH=${TARGETARCH:-amd64}
ARG GOFLAGS=""

USER root
RUN dnf update -y && \
	dnf install -y git ca-certificates tzdata && \
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
	-o mock-llm \
	./test/services/mock-llm/cmd/mock-llm; \
	else \
	echo "Building production binary..."; \
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
	-mod=mod \
	-ldflags="-s -w" \
	-o mock-llm \
	./test/services/mock-llm/cmd/mock-llm; \
	fi

# ============================================================================
# Stage 2a: Production runtime (scratch)
# ============================================================================
FROM scratch AS production
COPY --from=builder /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /opt/app-root/src/mock-llm /mock-llm
USER 1001
EXPOSE 8080
ENTRYPOINT ["/mock-llm"]

LABEL name="mock-llm-service" \
	version="1.0.0" \
	description="Go Mock LLM Service - OpenAI/Ollama compatible test endpoint" \
	maintainer="Jordi Gil" \
	license="Apache-2.0" \
	summary="Mock LLM Service - Drop-in replacement for Python Mock LLM"

# ============================================================================
# Stage 2b: Development/E2E runtime (ubi10-minimal)
# ============================================================================
FROM registry.access.redhat.com/ubi10/ubi-minimal:latest AS development
RUN microdnf update -y && \
	microdnf install -y ca-certificates tzdata shadow-utils && \
	microdnf clean all
RUN useradd -r -u 1001 -g root mockllm-user
COPY --from=builder /opt/app-root/src/mock-llm /usr/local/bin/mock-llm
RUN chmod +x /usr/local/bin/mock-llm
USER 1001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/mock-llm"]

LABEL name="mock-llm-service" \
	version="1.0.0" \
	description="Go Mock LLM Service (dev) - OpenAI/Ollama compatible test endpoint" \
	maintainer="Jordi Gil" \
	license="Apache-2.0" \
	summary="Mock LLM Service (dev) - Coverage instrumentation support"
