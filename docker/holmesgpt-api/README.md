# HolmesGPT REST API Server

Source-built HolmesGPT with REST API capabilities using Red Hat UBI containers.

## Overview

This directory contains the build infrastructure for creating a production-ready HolmesGPT REST API server container that:

- **Builds HolmesGPT from source** for maximum security transparency
- **Uses Red Hat Universal Base Images** for enterprise compliance
- **Supports multi-architecture builds** (linux/amd64, linux/arm64)
- **Provides REST API endpoints** for programmatic access
- **Integrates with Kubernaut Context API** for enhanced investigations
- **Includes comprehensive security scanning** during build process

## Architecture

```
HolmesGPT API Server Container
├── Red Hat UBI9-micro (runtime)
├── HolmesGPT (built from source)
├── FastAPI REST endpoints
├── Context API integration
└── Security hardening
```

## Quick Start

### Prerequisites

- Podman installed and configured
- Git with submodule support
- Access to quay.io registry (for pushing)

### 1. Initialize HolmesGPT Source

```bash
# Initialize the HolmesGPT submodule
make holmesgpt-api-init

# Or manually:
git submodule update --init --recursive dependencies/holmesgpt
```

### 2. Build Container

```bash
# Build for multiple architectures
make holmesgpt-api-build

# Build for specific architecture
make holmesgpt-api-build-amd64
make holmesgpt-api-build-arm64

# Development build (faster, single arch)
make holmesgpt-api-build-dev
```

### 3. Test Container

```bash
# Test the built container
make holmesgpt-api-test

# Run security scan
make holmesgpt-api-security-scan
```

### 4. Run Locally

```bash
# Run the API server locally
make holmesgpt-api-run-local

# Check logs
make holmesgpt-api-logs

# Stop the container
make holmesgpt-api-stop-local
```

## Build System Components

### Files Structure

```
docker/holmesgpt-api/
├── Dockerfile                  # Multi-stage Red Hat UBI build
├── entrypoint.sh              # Container initialization script
├── security-check.py          # Security validation during build
├── requirements-holmesgpt.txt # Curated HolmesGPT dependencies
├── requirements-api.txt       # REST API dependencies
├── config/
│   └── settings.yaml         # Default configuration
└── README.md                 # This file

scripts/
├── build-holmesgpt-api.sh    # Multi-architecture build script
└── release-holmesgpt-api.sh  # Version management and release

dependencies/
└── holmesgpt/                # HolmesGPT source (git submodule)
```

### Security Features

- **Source-based build** - Full transparency of all components
- **Security scanning** - Automated vulnerability detection during build
- **Red Hat UBI base** - Enterprise-certified container images
- **Non-root execution** - Runs as unprivileged user (1001)
- **Read-only filesystem** - Minimizes attack surface
- **Minimal dependencies** - Only required packages included

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `HOLMESGPT_LLM_PROVIDER` | LLM provider (openai, anthropic, local_llm) | Required |
| `HOLMESGPT_LLM_API_KEY` | API key for LLM provider | Required |
| `HOLMESGPT_LLM_MODEL` | LLM model name | `gpt-4` |
| `HOLMESGPT_PORT` | HTTP server port | `8090` |
| `HOLMESGPT_METRICS_PORT` | Metrics server port | `9091` |
| `KUBERNAUT_CONTEXT_API_URL` | Context API endpoint | `http://context-api-service:8091` |
| `KUBECONFIG` | Kubernetes config path | `/root/.kube/config` |
| `DEBUG` | Enable debug logging | `false` |

### Example Usage

```bash
# Run with OpenAI
podman run -d \
  --name holmesgpt-api \
  -p 8090:8090 \
  -p 9091:9091 \
  -e HOLMESGPT_LLM_PROVIDER=openai \
  -e HOLMESGPT_LLM_API_KEY=sk-... \
  -e HOLMESGPT_LLM_MODEL=gpt-4 \
  quay.io/jordigilh/holmesgpt-api:latest

# Run with local LLM
podman run -d \
  --name holmesgpt-api \
  -p 8090:8090 \
  -p 9091:9091 \
  -e HOLMESGPT_LLM_PROVIDER=local_llm \
  -e HOLMESGPT_LLM_BASE_URL=http://local-llm:8080 \
  quay.io/jordigilh/holmesgpt-api:latest
```

## API Endpoints

### Investigation

- `POST /api/v1/investigate` - Start alert investigation
- `GET /api/v1/investigate/{id}` - Get investigation status
- `POST /api/v1/chat` - Interactive investigation chat
- `WebSocket /api/v1/chat/ws` - Real-time chat

### Management

- `GET /health` - Health check
- `GET /ready` - Readiness probe
- `GET /metrics` - Prometheus metrics
- `GET /docs` - API documentation

## Release Management

### Semantic Versioning

```bash
# Patch release (1.0.0 -> 1.0.1)
make holmesgpt-api-release-patch

# Minor release (1.0.1 -> 1.1.0)
make holmesgpt-api-release-minor

# Major release (1.1.0 -> 2.0.0)
make holmesgpt-api-release-major

# Custom version
make holmesgpt-api-release-custom VERSION=1.2.3

# Dry run (preview changes)
make holmesgpt-api-release-dry-run
```

### Release Process

1. **Validation** - Checks working directory and tests
2. **Version Calculation** - Determines next version number
3. **Container Build** - Multi-architecture build with security scan
4. **Git Tagging** - Creates annotated git tag
5. **Registry Push** - Pushes container to quay.io
6. **Release Notes** - Generates release documentation

## Development

### Setup Development Environment

```bash
# One-time setup
make holmesgpt-api-dev-setup

# Update HolmesGPT source
make holmesgpt-api-update

# Clean build artifacts
make holmesgpt-api-clean
```

### Custom Build Options

```bash
# Build specific version
VERSION=1.0.0 make holmesgpt-api-build

# Build with custom image name
IMAGE_NAME=my-registry/holmesgpt-api make holmesgpt-api-build

# Build single architecture for speed
PLATFORMS=linux/amd64 make holmesgpt-api-build

# Skip security scan (faster development)
./scripts/build-holmesgpt-api.sh --no-security-scan
```

### Security Scanning

The build process includes automated security scanning:

- **Safety** - Python dependency vulnerability scan
- **Bandit** - Python code security analysis
- **Pip-audit** - Additional dependency security check
- **Trivy/Grype** - Container image vulnerability scan

Security thresholds:
- **High/Critical**: 0 allowed (build fails)
- **Medium**: 5 allowed
- **Low**: 20 allowed

## Kubernetes Deployment

### Basic Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: holmesgpt-api-server
  namespace: kubernaut-system
spec:
  replicas: 3
  selector:
    matchLabels:
      app: holmesgpt-api-server
  template:
    metadata:
      labels:
        app: holmesgpt-api-server
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1001
        fsGroup: 0
      containers:
      - name: holmesgpt-api
        image: quay.io/jordigilh/holmesgpt-api:latest
        ports:
        - containerPort: 8090
        - containerPort: 9091
        env:
        - name: HOLMESGPT_LLM_PROVIDER
          value: "openai"
        - name: HOLMESGPT_LLM_API_KEY
          valueFrom:
            secretKeyRef:
              name: holmesgpt-secrets
              key: llm-api-key
        resources:
          limits:
            memory: "2Gi"
            cpu: "1000m"
          requests:
            memory: "512Mi"
            cpu: "250m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8090
          initialDelaySeconds: 30
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /ready
            port: 8090
          initialDelaySeconds: 10
          periodSeconds: 10
```

### Service Configuration

```yaml
apiVersion: v1
kind: Service
metadata:
  name: holmesgpt-api-service
  namespace: kubernaut-system
spec:
  selector:
    app: holmesgpt-api-server
  ports:
  - name: http
    port: 8090
    targetPort: 8090
  - name: metrics
    port: 9091
    targetPort: 9091
```

## Troubleshooting

### Common Issues

1. **Submodule not initialized**
   ```bash
   # Fix: Initialize the submodule
   make holmesgpt-api-init
   ```

2. **Build fails with security errors**
   ```bash
   # Check security scan results
   make holmesgpt-api-security-scan

   # Skip security scan for development
   ./scripts/build-holmesgpt-api.sh --no-security-scan
   ```

3. **Container fails to start**
   ```bash
   # Check logs
   make holmesgpt-api-logs

   # Check environment variables
   podman exec holmesgpt-api-local env | grep HOLMESGPT
   ```

4. **LLM connectivity issues**
   ```bash
   # Test LLM endpoint manually
   curl -H "Authorization: Bearer $HOLMESGPT_LLM_API_KEY" \
        $HOLMESGPT_LLM_BASE_URL/v1/models
   ```

### Debug Mode

```bash
# Run with debug enabled
podman run -d \
  --name holmesgpt-api-debug \
  -p 8090:8090 \
  -e DEBUG=true \
  -e HOLMESGPT_LLM_PROVIDER=openai \
  -e HOLMESGPT_LLM_API_KEY=sk-... \
  quay.io/jordigilh/holmesgpt-api:latest

# View debug logs
podman logs -f holmesgpt-api-debug
```

## Support

- **Documentation**: [Architecture Design](../../docs/architecture/HOLMESGPT_REST_API_ARCHITECTURE.md)
- **Business Requirements**: [Requirements Document](../../docs/requirements/13_HOLMESGPT_REST_API_WRAPPER.md)
- **Issues**: [GitHub Issues](https://github.com/jordigilh/kubernaut/issues)
- **Container Registry**: [quay.io/jordigilh/holmesgpt-api](https://quay.io/repository/jordigilh/holmesgpt-api)
