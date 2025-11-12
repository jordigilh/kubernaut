FROM golang:1.21-alpine

# Install test dependencies
RUN apk add --no-cache \
    git \
    make \
    podman \
    bash \
    curl \
    gcc \
    musl-dev

# Install Kind (for E2E tests)
RUN curl -Lo /usr/local/bin/kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64 && \
    chmod +x /usr/local/bin/kind

# Install kubectl (for E2E tests)
RUN curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && \
    chmod +x kubectl && \
    mv kubectl /usr/local/bin/

# Install ginkgo CLI
RUN go install github.com/onsi/ginkgo/v2/ginkgo@latest

WORKDIR /workspace

# Set Go environment
ENV GOFLAGS="-mod=mod"
ENV CGO_ENABLED=0

# Default: Run all tests
CMD ["make", "test-all-services"]

