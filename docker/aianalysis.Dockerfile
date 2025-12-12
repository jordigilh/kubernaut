# AIAnalysis Controller Dockerfile
# Multi-stage build for minimal production image
#
# Build: docker build -f docker/aianalysis.Dockerfile -t kubernaut-aianalysis:latest .
# Run: docker run -p 9090:9090 -p 8081:8081 kubernaut-aianalysis:latest

# Stage 1: Build - Red Hat UBI9 Go 1.24 toolset (matches Data Storage)
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 AS builder

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

# Copy go.mod and go.sum first for better caching
COPY --chown=1001:0 go.mod go.sum ./
RUN go mod download

# Copy source code
COPY --chown=1001:0 . .

# Build the binary
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -a -installsuffix cgo \
    -o aianalysis-controller ./cmd/aianalysis

# Stage 2: Runtime - Red Hat UBI9 minimal (matches Data Storage)
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Install CA certificates for HTTPS calls
USER root
RUN microdnf update -y && \
    microdnf install -y ca-certificates tzdata && \
    microdnf clean all

# Create non-root user
RUN useradd -r -u 1001 -g root aianalysis-user

# Set working directory
WORKDIR /opt/app-root

# Copy binary from builder
COPY --from=builder /opt/app-root/src/aianalysis-controller /usr/local/bin/aianalysis-controller
RUN chmod +x /usr/local/bin/aianalysis-controller

# Switch to non-root user for security
USER 1001

# Expose ports
# 9090 - Prometheus metrics
# 8081 - Health probes (liveness/readiness)
EXPOSE 9090 8081

# Entrypoint
ENTRYPOINT ["/usr/local/bin/aianalysis-controller"]

# Red Hat UBI9 compatible metadata labels
LABEL name="kubernaut-aianalysis" \
    vendor="Kubernaut" \
    version="1.0.0" \
    release="1" \
    summary="AIAnalysis Controller - AI-Powered Kubernetes Analysis" \
    description="A Go microservice component of Kubernaut that provides AI-powered analysis for Kubernetes incidents using HolmesGPT integration." \
    maintainer="jgil@redhat.com" \
    component="aianalysis-controller" \
    part-of="kubernaut" \
    io.k8s.description="AIAnalysis Controller for Kubernaut" \
    io.k8s.display-name="Kubernaut AIAnalysis Controller" \
    io.openshift.tags="kubernaut,aianalysis,ai,llm,kubernetes,controller"


