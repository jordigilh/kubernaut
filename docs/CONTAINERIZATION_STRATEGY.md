# Containerization Strategy - Prometheus Alerts SLM

**Goal**: Deploy the complete prometheus-alerts-slm system in containers with integrated LLM models
**Current State**: Application has Dockerfiles but relies on external Ollama service
**Target**: Self-contained deployment with embedded model serving

## Current Architecture Issues

### What We Have Now
```yaml
# Current deployment requires external Ollama
prometheus-alerts-slm:
  image: "app-only"
  environment:
    - OLLAMA_ENDPOINT: "http://ollama-service:11434"  # External dependency
    - OLLAMA_MODEL: "granite3.1-dense:2b"

ollama-service:  # Separate service required
  image: "ollama/ollama"
  volumes:
    - model-data:/root/.ollama  # Models downloaded at runtime
```

### Problems with Current Approach
- **External dependency** on separate Ollama service
- **Model download at runtime** (slow startup, internet dependency)
- **Complex deployment** requiring multiple containers
- **Version skew risk** between app and model service
- **Resource management complexity** (separate resource allocation)

## Containerization Approaches

### Approach 1: Self-Contained Container (Recommended for Production)

#### Architecture
```
prometheus-alerts-slm:all-in-one
├── Application binary
├── Embedded Ollama server
├── Pre-loaded Granite model
└── Unified configuration
```

#### Implementation Strategy
```dockerfile
# Multi-stage build for optimal size
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o prometheus-alerts-slm ./cmd/prometheus-alerts-slm

FROM nvidia/cuda:11.8-runtime-ubuntu22.04 AS runtime
# Install Ollama
RUN curl -fsSL https://ollama.com/install.sh | sh

# Pre-download the optimal model based on our analysis
RUN ollama serve & \
    sleep 5 && \
    ollama pull granite3.1-dense:2b && \
    ollama stop

# Copy application
COPY --from=builder /app/prometheus-alerts-slm /usr/local/bin/
COPY --from=builder /app/config /etc/prometheus-alerts-slm/

# Startup script to launch both services
COPY scripts/start-container.sh /start.sh
RUN chmod +x /start.sh

EXPOSE 8080 11434
CMD ["/start.sh"]
```

#### Startup Script
```bash
#!/bin/bash
# start-container.sh
set -e

# Start Ollama in background
ollama serve &
OLLAMA_PID=$!

# Wait for Ollama to be ready
sleep 5

# Verify model is available
ollama list | grep granite3.1-dense:2b || {
    echo "Model not found, pulling..."
    ollama pull granite3.1-dense:2b
}

# Set internal endpoint
export OLLAMA_ENDPOINT="http://localhost:11434"
export OLLAMA_MODEL="granite3.1-dense:2b"

# Start main application
exec prometheus-alerts-slm
```

#### Benefits
- **Self-contained**: No external dependencies
- **Fast startup**: Model pre-loaded
- **Simplified deployment**: Single container
- **Version consistency**: App and model bundled together
- **Resource efficiency**: Shared GPU/CPU resources

#### Drawbacks
- **Large image size**: ~3-4GB (model + runtime)
- **Build complexity**: Multi-stage with model download
- **Update overhead**: Full rebuild for model updates

### Approach 2: Init Container Pattern (Hybrid Approach)

#### Architecture
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus-alerts-slm
spec:
  template:
    spec:
      initContainers:
      - name: model-downloader
        image: ollama/ollama:latest
        command: ["/bin/sh", "-c"]
        args:
        - |
          ollama serve &
          sleep 5
          ollama pull granite3.1-dense:2b
          ollama stop
          cp -r /root/.ollama/* /shared/models/
        volumeMounts:
        - name: model-storage
          mountPath: /shared/models

      containers:
      - name: ollama
        image: ollama/ollama:latest
        volumeMounts:
        - name: model-storage
          mountPath: /root/.ollama
        ports:
        - containerPort: 11434

      - name: app
        image: prometheus-alerts-slm:latest
        env:
        - name: OLLAMA_ENDPOINT
          value: "http://localhost:11434"
        - name: OLLAMA_MODEL
          value: "granite3.1-dense:2b"
        ports:
        - containerPort: 8080

      volumes:
      - name: model-storage
        emptyDir: {}
```

#### Benefits
- **Smaller app image**: App and model separate
- **Flexible model updates**: Init container can download different models
- **Resource separation**: Distinct resource allocation
- **Easier debugging**: Separate service logs

#### Drawbacks
- **Complex deployment**: Multiple containers to coordinate
- **Slower startup**: Model download on first run
- **Volume management**: Shared storage complexity

### Approach 3: Sidecar Pattern (Enterprise/Multi-Model)

#### Architecture
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus-alerts-slm
spec:
  template:
    spec:
      containers:
      - name: app
        image: prometheus-alerts-slm:latest
        env:
        - name: OLLAMA_ENDPOINT
          value: "http://localhost:11434"
        ports:
        - containerPort: 8080

      - name: model-server
        image: prometheus-alerts-slm-models:granite-2b
        ports:
        - containerPort: 11434
        resources:
          requests:
            memory: "3Gi"
            cpu: "2"
          limits:
            memory: "4Gi"
            cpu: "4"
```

#### Benefits
- **Model specialization**: Different model images for different use cases
- **Independent scaling**: Scale model server separately
- **Resource optimization**: Fine-tuned resource allocation
- **Multiple models**: Support for model routing/fallback

#### Drawbacks
- **Deployment complexity**: Separate lifecycle management
- **Network overhead**: Inter-container communication
- **Resource coordination**: Complex resource management

## Implementation Plan

### Phase 1: Self-Contained Production Image

Based on our model performance analysis, create a production-ready image:

```dockerfile
# Dockerfile.production
FROM nvidia/cuda:11.8-runtime-ubuntu22.04

# Install dependencies
RUN apt-get update && apt-get install -y \
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install Ollama
RUN curl -fsSL https://ollama.com/install.sh | sh

# Create app user
RUN useradd -m -s /bin/bash appuser

# Pre-download optimal model (based on our analysis)
USER appuser
RUN ollama serve & \
    sleep 10 && \
    ollama pull granite3.1-dense:2b && \
    pkill ollama

# Copy application binary
COPY prometheus-alerts-slm /usr/local/bin/
COPY config/ /etc/prometheus-alerts-slm/
COPY scripts/start-production.sh /start.sh

USER root
RUN chmod +x /start.sh /usr/local/bin/prometheus-alerts-slm
USER appuser

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
  CMD curl -f http://localhost:8080/health || exit 1

CMD ["/start.sh"]
```

### Phase 2: Multi-Architecture Support

```dockerfile
# Support both CPU and GPU deployments
ARG TARGETARCH
ARG CUDA_SUPPORT=false

FROM golang:1.23-alpine AS builder
ARG TARGETARCH
WORKDIR /app
COPY . .
RUN GOARCH=${TARGETARCH} go build -o prometheus-alerts-slm ./cmd/prometheus-alerts-slm

# Choose base image based on GPU support
FROM ubuntu:22.04 AS runtime-cpu
FROM nvidia/cuda:11.8-runtime-ubuntu22.04 AS runtime-gpu

FROM runtime-${CUDA_SUPPORT} AS runtime
# ... rest of Dockerfile
```

### Phase 3: Optimized Model Images

Create specialized images for different deployment scenarios:

```bash
# Build matrix based on our model performance analysis
docker build -t prometheus-alerts-slm:dense-8b \
  --build-arg MODEL=granite3.1-dense:8b \
  --build-arg MEMORY_LIMIT=8Gi .

docker build -t prometheus-alerts-slm:dense-2b \
  --build-arg MODEL=granite3.1-dense:2b \
  --build-arg MEMORY_LIMIT=3Gi .

docker build -t prometheus-alerts-slm:moe-1b \
  --build-arg MODEL=granite3.1-moe:1b \
  --build-arg MEMORY_LIMIT=2Gi .
```

## Deployment Configurations

### Kubernetes Deployment (Recommended)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus-alerts-slm
  labels:
    app: prometheus-alerts-slm
spec:
  replicas: 2  # For HA
  selector:
    matchLabels:
      app: prometheus-alerts-slm
  template:
    metadata:
      labels:
        app: prometheus-alerts-slm
    spec:
      containers:
      - name: prometheus-alerts-slm
        image: prometheus-alerts-slm:dense-2b-latest
        ports:
        - containerPort: 8080
          name: webhook
        - containerPort: 9090
          name: metrics
        env:
        - name: DRY_RUN
          value: "false"
        - name: LOG_LEVEL
          value: "info"
        resources:
          requests:
            memory: "3Gi"
            cpu: "2"
          limits:
            memory: "4Gi"
            cpu: "4"
            # GPU allocation if available
            nvidia.com/gpu: 1
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 60
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        volumeMounts:
        - name: config
          mountPath: /etc/prometheus-alerts-slm
      volumes:
      - name: config
        configMap:
          name: prometheus-alerts-slm-config
      nodeSelector:
        # Prefer nodes with GPU if available
        accelerator: nvidia-tesla-k80
      tolerations:
      - key: nvidia.com/gpu
        operator: Exists
        effect: NoSchedule
```

### Docker Compose (Development)
```yaml
version: '3.8'
services:
  prometheus-alerts-slm:
    build:
      context: .
      dockerfile: Dockerfile.production
      args:
        MODEL: granite3.1-dense:2b
    ports:
      - "8080:8080"
      - "9090:9090"
    environment:
      - DRY_RUN=true
      - LOG_LEVEL=debug
      - OLLAMA_ENDPOINT=http://localhost:11434
      - OLLAMA_MODEL=granite3.1-dense:2b
    volumes:
      - ./config:/etc/prometheus-alerts-slm:ro
    deploy:
      resources:
        limits:
          memory: 4G
        reservations:
          memory: 3G
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]
```

## Build & CI/CD Pipeline

### Multi-Stage Build Pipeline

```yaml
# .github/workflows/container-build.yml
name: Container Build & Push

on:
  push:
    branches: [main]
    tags: ['v*']

jobs:
  build:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        model: [dense-8b, dense-2b, moe-1b]

    steps:
    - uses: actions/checkout@v3

    - name: Set up Docker Buildx
      uses: docker/setup-buildx-action@v2

    - name: Build and push
      uses: docker/build-push-action@v4
      with:
        context: .
        file: Dockerfile.production
        build-args: |
          MODEL=granite3.1-${{ matrix.model }}
        tags: |
          prometheus-alerts-slm:${{ matrix.model }}-${{ github.sha }}
          prometheus-alerts-slm:${{ matrix.model }}-latest
        push: true
        cache-from: type=gha
        cache-to: type=gha,mode=max
```

## Resource Planning & Optimization

### Model-Specific Resource Requirements

| Model | Memory | CPU | GPU | Startup Time | Image Size |
|-------|---------|-----|-----|--------------|------------|
| **Dense 8B** | 6-8Gi | 4-6 cores | Optional | 60-90s | ~6GB |
| **Dense 2B** | 3-4Gi | 2-4 cores | Optional | 30-45s | ~3GB |
| **MoE 1B** | 2-3Gi | 1-2 cores | Optional | 15-30s | ~2GB |

### Cost Optimization Strategies

1. **Use Dense 2B by default** (optimal balance per our analysis)
2. **Reserve Dense 8B** for critical/security scenarios
3. **Use MoE 1B** for development/testing only
4. **Enable horizontal pod autoscaling** based on alert volume
5. **Implement node affinity** for GPU-optimized instances

## Security Considerations

### Image Security
```dockerfile
# Security best practices
FROM nvidia/cuda:11.8-runtime-ubuntu22.04

# Create non-root user
RUN useradd -r -u 1000 -m -c "App User" -d /app -s /sbin/nologin appuser

# Install security updates
RUN apt-get update && apt-get upgrade -y && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Set secure file permissions
COPY --chown=appuser:appuser prometheus-alerts-slm /app/
USER appuser

# Run with minimal privileges
WORKDIR /app
```

### Runtime Security
- **Read-only root filesystem** where possible
- **Capabilities dropping** (no CAP_SYS_ADMIN needed)
- **Network policies** to restrict model server access
- **Resource limits** to prevent DoS attacks
- **Regular image scanning** for vulnerabilities

## Monitoring & Observability

### Container-Specific Metrics
```go
// Additional metrics for containerized deployment
var (
    modelLoadTime = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "model_load_duration_seconds",
            Help: "Time taken to load the model at startup",
        }, []string{"model"})

    containerMemoryUsage = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "container_memory_usage_bytes",
            Help: "Container memory usage",
        }, []string{"container"})
)
```

### Health Checks
```go
// Enhanced health checks for containerized deployment
func (h *HealthHandler) ContainerHealth(w http.ResponseWriter, r *http.Request) {
    health := struct {
        Status      string    `json:"status"`
        ModelLoaded bool      `json:"model_loaded"`
        OllamaReady bool      `json:"ollama_ready"`
        Timestamp   time.Time `json:"timestamp"`
    }{
        Status:      "healthy",
        ModelLoaded: h.slmClient.IsHealthy(),
        OllamaReady: h.checkOllamaHealth(),
        Timestamp:   time.Now(),
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(health)
}
```

## Conclusion & Next Steps

### **Recommended Approach: Self-Contained Production Container**

1. **Build single container** with embedded Ollama and granite3.1-dense:2b model
2. **Pre-load model** during image build for fast startup
3. **Use multi-stage builds** to optimize final image size
4. **Implement proper health checks** and resource limits
5. **Support both CPU and GPU deployments**

### **Implementation Priority**
1. **Phase 1**: Create self-contained Dockerfile with Dense 2B model (**1-2 weeks**)
2. **Phase 2**: Add multi-architecture and GPU support (**1 week**)
3. **Phase 3**: Create specialized images for different models (**1 week**)
4. **Phase 4**: Implement CI/CD pipeline and security hardening (**1-2 weeks**)

### **Immediate Next Steps**
1. Update existing `Dockerfile` to include Ollama and model pre-loading
2. Create production startup script with proper service orchestration
3. Add container-specific health checks and monitoring
4. Test resource requirements with actual model loading
5. Implement CI/CD pipeline for automated builds

This containerization strategy ensures a production-ready, self-contained deployment that leverages our model performance analysis and provides the foundation for robust Kubernetes operations.

---

*Strategy based on current PoC architecture and Granite model performance analysis*
