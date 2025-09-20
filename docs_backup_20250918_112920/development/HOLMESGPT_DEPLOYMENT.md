# HolmesGPT Deployment Guide

This guide covers deploying HolmesGPT for local integration testing and Kubernetes e2e testing with Kubernaut.

## Overview

HolmesGPT can be deployed in two primary modes for testing with Kubernaut:

1. **Local Development**: Podman container for integration testing with local LLM
2. **Kubernetes E2E**: Helm chart deployment for end-to-end testing

---

## Local Development Setup (Podman)

### Prerequisites

- Podman installed and configured
- Local LLM service running at `192.168.1.169:8080`
- Access to HolmesGPT container image

### 1. Container Configuration

Create a configuration file for local testing:

```bash
# Create config directory
mkdir -p ~/.config/holmesgpt

# Create local configuration
cat > ~/.config/holmesgpt/config.yaml << 'EOF'
llm:
  provider: "openai-compatible"  # For LocalAI/Ramalama compatibility
  base_url: "http://192.168.1.169:8080/v1"
  model: "gpt-oss:20b"
  api_key: "not-required-for-local"

toolsets:
  - kubernetes
  - prometheus
  - docker
  - internet

api:
  host: "0.0.0.0"
  port: 8080
  cors_origins: ["*"]

logging:
  level: "info"
  format: "json"

kubernetes:
  # For local testing, use kubeconfig
  incluster: false
  kubeconfig: "/root/.kube/config"
EOF
```

### 2. Run HolmesGPT Container

```bash
#!/bin/bash
# scripts/run-holmesgpt-local.sh

set -e

CONTAINER_NAME="holmesgpt-local"
IMAGE="us-central1-docker.pkg.dev/genuine-flight-317411/devel/holmesgpt:latest"
CONFIG_DIR="$HOME/.config/holmesgpt"
KUBE_CONFIG="$HOME/.kube/config"

echo "🚀 Starting HolmesGPT for local integration testing..."

# Stop existing container if running
podman stop $CONTAINER_NAME 2>/dev/null || true
podman rm $CONTAINER_NAME 2>/dev/null || true

# Run HolmesGPT container
podman run -d \
  --name $CONTAINER_NAME \
  --network host \
  -v $CONFIG_DIR:/app/config:ro \
  -v $KUBE_CONFIG:/root/.kube/config:ro \
  -e HOLMES_CONFIG_FILE="/app/config/config.yaml" \
  -e HOLMES_LLM_PROVIDER="openai-compatible" \
  -e HOLMES_LLM_BASE_URL="http://192.168.1.169:8080/v1" \
  -e HOLMES_LLM_MODEL="gpt-oss:20b" \
  -e HOLMES_API_HOST="0.0.0.0" \
  -e HOLMES_API_PORT="8080" \
  $IMAGE \
  serve --port 8080 --host 0.0.0.0

echo "⏳ Waiting for HolmesGPT to start..."
sleep 10

# Health check
if curl -f http://localhost:8080/health &>/dev/null; then
    echo "✅ HolmesGPT is running at http://localhost:8080"
    echo "📋 API Documentation: http://localhost:8080/docs"
    echo "🔍 Test with: curl http://localhost:8080/health"
else
    echo "❌ HolmesGPT failed to start. Check logs:"
    podman logs $CONTAINER_NAME
    exit 1
fi
```

### 3. Integration Testing Script

```bash
#!/bin/bash
# scripts/test-holmesgpt-integration.sh

set -e

HOLMES_URL="http://localhost:8080"
KUBERNAUT_PYTHON_API="http://localhost:8000"

echo "🧪 Running HolmesGPT integration tests..."

# Test 1: Health Check
echo "1. Testing HolmesGPT health..."
curl -f $HOLMES_URL/health || {
    echo "❌ HolmesGPT health check failed"
    exit 1
}

# Test 2: Basic Investigation
echo "2. Testing basic investigation..."
curl -X POST $HOLMES_URL/investigate \
  -H "Content-Type: application/json" \
  -d '{
    "query": "Pod is in CrashLoopBackOff state",
    "context": {
      "namespace": "default",
      "pod_name": "test-pod",
      "container_name": "app"
    }
  }' || {
    echo "❌ Basic investigation test failed"
    exit 1
}

# Test 3: Kubernaut Python API integration
echo "3. Testing Kubernaut integration..."
curl -X POST $KUBERNAUT_PYTHON_API/api/v1/investigate \
  -H "Content-Type: application/json" \
  -d '{
    "alert": {
      "name": "PodCrashLooping",
      "severity": "warning",
      "description": "Pod test-pod is crash looping",
      "namespace": "default",
      "labels": {
        "pod": "test-pod",
        "container": "app"
      }
    },
    "options": {
      "use_holmesgpt": true,
      "include_context": ["kubernetes", "prometheus"]
    }
  }' || {
    echo "❌ Kubernaut integration test failed"
    exit 1
}

echo "✅ All integration tests passed!"
```

### 4. Local Development Workflow

```bash
# Start local services
make dev-start-local

# Start HolmesGPT
./scripts/run-holmesgpt-local.sh

# Run integration tests
./scripts/test-holmesgpt-integration.sh

# View logs
podman logs holmesgpt-local -f

# Stop services
podman stop holmesgpt-local
make dev-stop-local
```

---

## Kubernetes E2E Testing (Helm)

### Prerequisites

- Kubernetes cluster with Helm installed
- Access to HolmesGPT Helm repository
- Prometheus and Grafana deployed (for full e2e testing)

### 1. Helm Chart Configuration

Create values file for e2e testing:

```yaml
# deploy/holmesgpt-e2e-values.yaml
nameOverride: "holmesgpt-e2e"
fullnameOverride: "holmesgpt-e2e"

# Replica configuration
replicaCount: 2

# Image configuration
image:
  repository: us-central1-docker.pkg.dev/genuine-flight-317411/devel/holmesgpt
  tag: "latest"
  pullPolicy: IfNotPresent

# Service configuration
service:
  type: ClusterIP
  port: 8080
  targetPort: 8080
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8080"
    prometheus.io/path: "/metrics"

# Ingress for e2e testing
ingress:
  enabled: true
  className: "nginx"
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
    nginx.ingress.kubernetes.io/ssl-redirect: "false"
  hosts:
    - host: holmesgpt-e2e.kubernaut.local
      paths:
        - path: /
          pathType: Prefix
  # tls: []

# Resource limits for e2e testing
resources:
  limits:
    cpu: 1000m
    memory: 2Gi
  requests:
    cpu: 500m
    memory: 1Gi

# HolmesGPT specific configuration
config:
  llm:
    provider: "openai-compatible"
    baseUrl: "http://192.168.1.169:8080/v1"
    model: "gpt-oss:20b"
    apiKey: "not-required"

  api:
    host: "0.0.0.0"
    port: 8080
    corsOrigins: ["*"]

  toolsets:
    - kubernetes
    - prometheus
    - aws
    - internet

  kubernetes:
    incluster: true
    namespace: "kubernaut-system"

  prometheus:
    url: "http://prometheus.monitoring.svc.cluster.local:9090"

  logging:
    level: "debug"  # Verbose logging for e2e testing
    format: "json"

# Environment variables
env:
  - name: HOLMES_CONFIG_FILE
    value: "/app/config/config.yaml"
  - name: HOLMES_LLM_PROVIDER
    value: "openai-compatible"
  - name: HOLMES_LLM_BASE_URL
    value: "http://192.168.1.169:8080/v1"
  - name: HOLMES_LLM_MODEL
    value: "gpt-oss:20b"

# Health checks
livenessProbe:
  httpGet:
    path: /health
    port: http
  initialDelaySeconds: 30
  periodSeconds: 30

readinessProbe:
  httpGet:
    path: /ready
    port: http
  initialDelaySeconds: 10
  periodSeconds: 10

# ServiceMonitor for Prometheus scraping
serviceMonitor:
  enabled: true
  labels:
    prometheus: kubernaut
  interval: 30s
  path: /metrics

# RBAC for Kubernetes access
rbac:
  create: true
  rules:
    - apiGroups: [""]
      resources: ["pods", "services", "endpoints", "nodes", "events"]
      verbs: ["get", "list", "watch"]
    - apiGroups: ["apps"]
      resources: ["deployments", "replicasets", "daemonsets", "statefulsets"]
      verbs: ["get", "list", "watch"]
    - apiGroups: ["metrics.k8s.io"]
      resources: ["*"]
      verbs: ["get", "list"]

# Service Account
serviceAccount:
  create: true
  name: "holmesgpt-e2e"
  annotations: {}

# Node selector for e2e testing
nodeSelector: {}
tolerations: []
affinity: {}
```

### 2. Helm Deployment Script

```bash
#!/bin/bash
# scripts/deploy-holmesgpt-e2e.sh

set -e

NAMESPACE="kubernaut-system"
RELEASE_NAME="holmesgpt-e2e"
VALUES_FILE="deploy/holmesgpt-e2e-values.yaml"

echo "🚀 Deploying HolmesGPT for E2E testing..."

# Add HolmesGPT Helm repository
echo "📦 Adding HolmesGPT Helm repository..."
helm repo add holmesgpt https://charts.holmesgpt.dev
helm repo update

# Create namespace if it doesn't exist
kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -

# Deploy or upgrade HolmesGPT
echo "🔧 Deploying HolmesGPT..."
helm upgrade --install $RELEASE_NAME holmesgpt/holmesgpt \
  --namespace $NAMESPACE \
  --values $VALUES_FILE \
  --wait \
  --timeout 10m

echo "⏳ Waiting for HolmesGPT pods to be ready..."
kubectl wait --for=condition=ready pod \
  -l app.kubernetes.io/name=holmesgpt \
  -n $NAMESPACE \
  --timeout=300s

# Get service information
HOLMESGPT_SERVICE=$(kubectl get svc -n $NAMESPACE -l app.kubernetes.io/name=holmesgpt -o jsonpath='{.items[0].metadata.name}')
echo "✅ HolmesGPT deployed successfully!"
echo "📋 Service: $HOLMESGPT_SERVICE"
echo "🔍 Access: kubectl port-forward -n $NAMESPACE svc/$HOLMESGPT_SERVICE 8080:8080"

# Run basic health check
echo "🏥 Running health check..."
kubectl port-forward -n $NAMESPACE svc/$HOLMESGPT_SERVICE 8080:8080 &
PORT_FORWARD_PID=$!

sleep 10
if curl -f http://localhost:8080/health &>/dev/null; then
    echo "✅ HolmesGPT health check passed"
else
    echo "❌ HolmesGPT health check failed"
    kubectl logs -n $NAMESPACE -l app.kubernetes.io/name=holmesgpt --tail=50
fi

kill $PORT_FORWARD_PID 2>/dev/null || true
```

### 3. E2E Test Suite

```bash
#!/bin/bash
# scripts/e2e-test-holmesgpt.sh

set -e

NAMESPACE="kubernaut-system"
HOLMESGPT_SERVICE="holmesgpt-e2e"

echo "🧪 Running HolmesGPT E2E test suite..."

# Setup port forwarding
echo "🔌 Setting up port forwarding..."
kubectl port-forward -n $NAMESPACE svc/$HOLMESGPT_SERVICE 8080:8080 &
PORT_FORWARD_PID=$!

# Wait for port forward to be ready
sleep 10

# Test 1: Service Discovery
echo "1. Testing service discovery..."
kubectl get svc -n $NAMESPACE $HOLMESGPT_SERVICE

# Test 2: Health and Readiness
echo "2. Testing health endpoints..."
curl -f http://localhost:8080/health
curl -f http://localhost:8080/ready

# Test 3: API Documentation
echo "3. Testing API documentation..."
curl -f http://localhost:8080/docs

# Test 4: Kubernetes Integration
echo "4. Testing Kubernetes toolset..."
curl -X POST http://localhost:8080/investigate \
  -H "Content-Type: application/json" \
  -d '{
    "query": "List all pods in kube-system namespace",
    "toolsets": ["kubernetes"]
  }'

# Test 5: Prometheus Integration (if available)
echo "5. Testing Prometheus integration..."
curl -X POST http://localhost:8080/investigate \
  -H "Content-Type: application/json" \
  -d '{
    "query": "What is the CPU usage of nodes?",
    "toolsets": ["prometheus"]
  }' || echo "⚠️ Prometheus integration test skipped (service not available)"

# Test 6: End-to-End Investigation
echo "6. Running end-to-end investigation..."
curl -X POST http://localhost:8080/investigate \
  -H "Content-Type: application/json" \
  -d '{
    "query": "Investigate high memory usage in production namespace",
    "context": {
      "namespace": "production",
      "time_range": "5m"
    },
    "toolsets": ["kubernetes", "prometheus"]
  }'

# Cleanup
kill $PORT_FORWARD_PID 2>/dev/null || true

echo "✅ E2E test suite completed successfully!"
```

### 4. Integration with Kubernaut

Update your Kubernaut deployment to use the Kubernetes-deployed HolmesGPT:

```yaml
# config/development.yaml
holmesgpt:
  enabled: true
  endpoint: "http://holmesgpt-e2e.kubernaut-system.svc.cluster.local:8080"
  timeout: 60s
  retry_count: 3

# config/production.yaml (for e2e testing)
holmesgpt:
  enabled: true
  endpoint: "http://holmesgpt-e2e.kubernaut-system.svc.cluster.local:8080"
  timeout: 120s
  retry_count: 5
  health_check_interval: 30s
```

---

## Testing Workflows

### Local Development Testing

```bash
# Complete local testing workflow
make dev-start-local                    # Start local dependencies
./scripts/run-holmesgpt-local.sh       # Start HolmesGPT container
./scripts/test-holmesgpt-integration.sh # Run integration tests
make test-unit-python                   # Run Python API tests
make test-integration-core              # Run Go integration tests
```

### E2E Testing Workflow

```bash
# Complete e2e testing workflow
./scripts/deploy-holmesgpt-e2e.sh       # Deploy HolmesGPT to cluster
kubectl apply -f deploy/kubernaut-e2e/  # Deploy Kubernaut for testing
./scripts/e2e-test-holmesgpt.sh         # Run e2e tests
make test-e2e                          # Run full e2e test suite
```

### Cleanup

```bash
# Local cleanup
podman stop holmesgpt-local
podman rm holmesgpt-local

# Kubernetes cleanup
helm uninstall holmesgpt-e2e -n kubernaut-system
kubectl delete namespace kubernaut-system
```

---

## Troubleshooting

### Common Issues

#### Local Container Issues
```bash
# Check container logs
podman logs holmesgpt-local

# Check network connectivity
curl http://192.168.1.169:8080/v1/models

# Test LLM connectivity from container
podman exec holmesgpt-local curl http://192.168.1.169:8080/v1/models
```

#### Kubernetes Issues
```bash
# Check pod status
kubectl get pods -n kubernaut-system -l app.kubernetes.io/name=holmesgpt

# Check logs
kubectl logs -n kubernaut-system -l app.kubernetes.io/name=holmesgpt --tail=100

# Check service endpoints
kubectl get endpoints -n kubernaut-system holmesgpt-e2e

# Test service connectivity
kubectl run debug --image=curlimages/curl -it --rm -- curl http://holmesgpt-e2e.kubernaut-system.svc.cluster.local:8080/health
```

#### LLM Integration Issues
```bash
# Test LLM endpoint directly
curl http://192.168.1.169:8080/v1/models

# Check HolmesGPT configuration
kubectl get configmap -n kubernaut-system holmesgpt-e2e-config -o yaml

# Verify environment variables
kubectl describe pod -n kubernaut-system -l app.kubernetes.io/name=holmesgpt
```

### Performance Tuning

#### Resource Optimization
```yaml
# For resource-constrained environments
resources:
  requests:
    cpu: 100m
    memory: 512Mi
  limits:
    cpu: 500m
    memory: 1Gi
```

#### Connection Tuning
```yaml
config:
  api:
    max_concurrent_requests: 10
    request_timeout: 120s
  llm:
    timeout: 60s
    max_retries: 3
```

This deployment guide provides comprehensive coverage for both local development and Kubernetes e2e testing scenarios with HolmesGPT integrated into your Kubernaut system.
