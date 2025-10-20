# HolmesGPT + Kubernaut Hybrid Environment Setup Guide

## Overview

This guide provides detailed instructions for setting up the **HolmesGPT + Kubernaut hybrid environment**, which allows HolmesGPT to connect directly to Kubernetes and Prometheus while using the Kubernaut Context API only for enriched, Kubernaut-specific context.

## Architecture

```
┌─────────────────┐    Direct Access    ┌─────────────────┐
│                 │ ──────────────────► │   Kubernetes    │
│                 │                     │      API        │
│                 │    Direct Access    ├─────────────────┤
│   HolmesGPT     │ ──────────────────► │   Prometheus    │
│                 │                     │    Metrics      │
│                 │  Kubernaut-specific ├─────────────────┤
│                 │ ──────────────────► │   Kubernaut     │
└─────────────────┘                     │   Context API   │
         │                              │ - Action History│
         │                              │ - Pattern Data  │
         ▼                              │ - Discovery     │
┌─────────────────┐                     └─────────────────┘
│   Local LLM     │
│ (gpt-oss-20b)   │
└─────────────────┘
```

## Quick Start

### Automated Setup

The fastest way to get started is using the automated setup script:

```bash
# Make sure you're in the kubernaut project root
cd /path/to/kubernaut

# Run the automated setup script
./scripts/setup-holmesgpt-environment.sh
```

The script will:
1. Check all prerequisites
2. Set up configuration files
3. Start the Local LLM (if not running)
4. Start the Kubernaut Context API
5. Start the HolmesGPT container
6. Validate the complete setup

### Stop Environment

To stop all services:

```bash
./scripts/stop-holmesgpt-environment.sh
```

## Manual Setup (Step by Step)

If you prefer to set up manually or need to troubleshoot, follow these detailed steps:

### Step 1: Prerequisites

#### Required Software
- **podman**: For container management
- **curl**: For API testing
- **jq**: For JSON processing
- **oc** or **kubectl**: For Kubernetes access

#### Installation Commands

**macOS (using Homebrew):**
```bash
brew install podman curl jq
```

**Kubernetes CLI:**
```bash
# Download from https://mirror.openshift.com/pub/openshift-v4/clients/ocp/
# Or use the web console download link
```

#### Verify Prerequisites
```bash
# Check all required commands
podman --version
curl --version
jq --version
oc version

# Check Kubernetes/Kubernetes access
oc whoami
oc get nodes
```

### Step 2: Project Build

Ensure the Kubernaut project is built:

```bash
cd /path/to/kubernaut
make build
```

This creates the `bin/context-api-production` binary needed for the Context API server.

### Step 3: Configuration Setup

#### Create Configuration Directory
```bash
mkdir -p ~/.config/holmesgpt
```

#### Create HolmesGPT Configuration
```bash
cat > ~/.config/holmesgpt/config.yaml << 'EOF'
# HolmesGPT Configuration for Kubernaut Integration
llm:
  provider: "openai-compatible"
  base_url: "http://host.containers.internal:8080/v1"
  model: "ggml-org/gpt-oss-20b-GGUF"
  timeout: 120

api:
  host: "0.0.0.0"
  port: 8090

toolsets:
  - name: "kubernaut-hybrid"
    description: "Kubernaut hybrid toolset with direct K8s/Prometheus access"
    config_file: "/app/config/kubernaut-toolset.yaml"

integration:
  kubernaut_context_api: "http://host.containers.internal:8091"
  prometheus_url: "http://prometheus:9090"

logging:
  level: "info"
  format: "json"

timeouts:
  default: "30s"
  investigation: "300s"
EOF
```

#### Copy Hybrid Toolset Configuration
```bash
cp config/holmesgpt-hybrid-toolset.yaml ~/.config/holmesgpt/kubernaut-toolset.yaml
```

### Step 4: Start Local LLM

#### Option 1: Using Ramalama (Recommended)
```bash
# Install ramalama if not already installed
pip install ramalama

# Start the LLM service
ramalama serve --port 8080 ggml-org/gpt-oss-20b-GGUF
```

#### Option 2: Using Ollama
```bash
# Pull and run the model
ollama pull ggml-org/gpt-oss-20b-GGUF
ollama serve --port 8080
```

#### Option 3: Using LocalAI
```bash
# Using Docker/Podman
podman run -d --name local-llm \
  -p 8080:8080 \
  -v ./localai-config:/build/models \
  quay.io/go-skynet/local-ai:latest
```

#### Verify LLM Service
```bash
# Test the LLM endpoint
curl http://localhost:8080/v1/models

# Expected response should include gpt-oss-20b-GGUF model
```

### Step 5: Start Kubernaut Context API

#### Check Configuration File
Ensure the Context API configuration exists:
```bash
ls -la config/dynamic-context-orchestration.yaml
```

#### Start Context API Server
```bash
# From the kubernaut project root
./bin/context-api-production --config config/dynamic-context-orchestration.yaml &
CONTEXT_API_PID=$!
echo "Context API started with PID: $CONTEXT_API_PID"
```

#### Verify Context API
```bash
# Wait a few seconds for startup, then test
sleep 5
curl http://localhost:8091/api/v1/context/health

# Expected response:
# {"service":"context-api","status":"healthy","timestamp":"...","version":"1.0.0"}
```

### Step 6: Start HolmesGPT Container

#### Pull HolmesGPT Image
```bash
# For macOS (ARM64), use x86_64 emulation
podman pull --platform linux/amd64 us-central1-docker.pkg.dev/genuine-flight-317411/devel/holmes:latest
```

#### Start HolmesGPT Container
```bash
# Stop any existing container
podman stop holmesgpt-kubernaut-hybrid 2>/dev/null || true
podman rm holmesgpt-kubernaut-hybrid 2>/dev/null || true

# Start new container with hybrid configuration
podman run -d \
  --name holmesgpt-kubernaut-hybrid \
  --platform linux/amd64 \
  --network host \
  -v ~/.config/holmesgpt:/app/config:ro,Z \
  -v ~/.kube:/root/.kube:ro,Z \
  -e HOLMES_LLM_PROVIDER="openai-compatible" \
  -e HOLMES_LLM_BASE_URL="http://host.containers.internal:8080/v1" \
  -e HOLMES_LLM_MODEL="ggml-org/gpt-oss-20b-GGUF" \
  -e HOLMES_API_HOST="0.0.0.0" \
  -e HOLMES_API_PORT="8090" \
  -e KUBERNAUT_CONTEXT_API="http://host.containers.internal:8091" \
  us-central1-docker.pkg.dev/genuine-flight-317411/devel/holmes:latest \
  bash -c "echo 'HolmesGPT ready for investigations' && sleep infinity"
```

#### Verify Container
```bash
# Check container status
podman ps | grep holmesgpt

# Test container access
podman exec -it holmesgpt-kubernaut-hybrid echo "Container accessible"
```

### Step 7: Environment Validation

#### Complete Validation Script
```bash
#!/bin/bash

echo "🧪 Validating HolmesGPT + Kubernaut Hybrid Environment"
echo "======================================================"

# Test 1: Local LLM
echo "1. Testing Local LLM..."
if curl -s http://localhost:8080/v1/models | jq -r '.models[0].name' | grep -q "gpt-oss-20b-GGUF"; then
    echo "   ✅ Local LLM: OK"
else
    echo "   ❌ Local LLM: FAILED"
fi

# Test 2: Kubernaut Context API
echo "2. Testing Kubernaut Context API..."
if curl -s http://localhost:8091/api/v1/context/health | jq -r '.status' | grep -q "healthy"; then
    echo "   ✅ Context API: OK"
else
    echo "   ❌ Context API: FAILED"
fi

# Test 3: HolmesGPT Container
echo "3. Testing HolmesGPT Container..."
if podman ps --format "table {{.Names}}" | grep -q "holmesgpt-kubernaut-hybrid"; then
    echo "   ✅ HolmesGPT Container: OK"
else
    echo "   ❌ HolmesGPT Container: FAILED"
fi

# Test 4: Kubernetes Access
echo "4. Testing Kubernetes Access..."
if oc get pods -n default >/dev/null 2>&1; then
    echo "   ✅ Kubernetes Access: OK"
else
    echo "   ⚠️  Kubernetes Access: Limited (check oc login)"
fi

# Test 5: Direct K8s Access from Container
echo "5. Testing Direct K8s Access from Container..."
if podman exec holmesgpt-kubernaut-hybrid kubectl get pods -n default >/dev/null 2>&1; then
    echo "   ✅ Container K8s Access: OK"
else
    echo "   ⚠️  Container K8s Access: Limited"
fi

echo ""
echo "🎊 Validation Complete!"
```

Save this as `validate-environment.sh`, make it executable, and run it:
```bash
chmod +x validate-environment.sh
./validate-environment.sh
```

## Usage Examples

### Basic HolmesGPT Investigation

```bash
# Run investigation using hybrid toolset
podman exec -it holmesgpt-kubernaut-hybrid \
  holmes investigate \
  --alert-name "PodCrashLoop" \
  --namespace "default" \
  --toolsets /app/config/kubernaut-toolset.yaml
```

### Test Individual Tools

#### Direct Kubernetes Access
```bash
# Get pods directly
podman exec -it holmesgpt-kubernaut-hybrid kubectl get pods -n default -o json

# Get pod logs
podman exec -it holmesgpt-kubernaut-hybrid kubectl logs -n default <pod-name>

# Describe pod
podman exec -it holmesgpt-kubernaut-hybrid kubectl describe pod -n default <pod-name>
```

#### Kubernaut Context API Access
```bash
# Health check
curl http://localhost:8091/api/v1/context/health

# Action history (if available)
curl http://localhost:8091/api/v1/context/action-history

# Pattern analysis (if available)
curl "http://localhost:8091/api/v1/context/patterns/test-pattern"
```

#### Prometheus Access (if available)
```bash
# Query metrics directly
curl "http://prometheus:9090/api/v1/query?query=up"

# From container (if Prometheus is accessible)
podman exec -it holmesgpt-kubernaut-hybrid \
  curl "http://prometheus:9090/api/v1/query?query=container_memory_usage_bytes"
```

## Troubleshooting

### Common Issues

#### 1. Local LLM Not Responding
```bash
# Check if LLM is running
curl http://localhost:8080/v1/models

# If not responding, restart LLM service
ramalama serve --port 8080 ggml-org/gpt-oss-20b-GGUF
```

#### 2. Context API Connection Refused
```bash
# Check if Context API is running
ps aux | grep context-api

# Check port binding
lsof -i :8091

# Restart Context API if needed
./bin/context-api-production --config config/dynamic-context-orchestration.yaml &
```

#### 3. HolmesGPT Container Issues
```bash
# Check container logs
podman logs holmesgpt-kubernaut-hybrid

# Restart container
podman stop holmesgpt-kubernaut-hybrid
podman rm holmesgpt-kubernaut-hybrid
# Run the container start command again
```

#### 4. Kubernetes Access Issues
```bash
# Check authentication
oc whoami

# Re-authenticate if needed
oc login <cluster-url>

# Check kubeconfig
ls -la ~/.kube/config
```

#### 5. Volume Mount Issues (macOS)
```bash
# If volume mounts fail, try without SELinux labels
podman run -d \
  --name holmesgpt-kubernaut-hybrid \
  --platform linux/amd64 \
  --network host \
  -v ~/.config/holmesgpt:/app/config:ro \
  -v ~/.kube:/root/.kube:ro \
  # ... rest of command
```

### Debug Commands

#### Check All Services
```bash
# LLM service
curl -s http://localhost:8080/v1/models | jq '.'

# Context API
curl -s http://localhost:8091/api/v1/context/health | jq '.'

# Container status
podman ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

# Container logs
podman logs holmesgpt-kubernaut-hybrid | tail -20
```

#### Network Connectivity
```bash
# From container to host services
podman exec -it holmesgpt-kubernaut-hybrid curl http://host.containers.internal:8080/v1/models
podman exec -it holmesgpt-kubernaut-hybrid curl http://host.containers.internal:8091/api/v1/context/health
```

## Performance Tuning

### Memory Optimization
```bash
# Increase LLM context size if needed
ramalama serve --port 8080 --context-size 4096 ggml-org/gpt-oss-20b-GGUF

# Monitor memory usage
podman stats holmesgpt-kubernaut-hybrid
```

### Network Optimization
```bash
# Use host networking for better performance (already configured)
# Monitor network connections
ss -tulpn | grep -E ':(8080|8091|8090)'
```

## Security Considerations

### RBAC for HolmesGPT
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: holmesgpt-hybrid-reader
rules:
# Read access to core resources
- apiGroups: [""]
  resources: ["pods", "services", "events", "nodes"]
  verbs: ["get", "list"]
# Read access to workloads
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets"]
  verbs: ["get", "list"]
# Log access
- apiGroups: [""]
  resources: ["pods/log"]
  verbs: ["get"]
```

### Network Security
- Context API should only be accessible from trusted networks
- Consider using TLS for Context API in production
- Implement authentication for Context API endpoints

## Monitoring and Maintenance

### Health Monitoring
Create a monitoring script:
```bash
#!/bin/bash
# health-monitor.sh

while true; do
    # Check all services
    echo "$(date): Health Check"

    # LLM
    curl -s http://localhost:8080/v1/models >/dev/null && echo "LLM: OK" || echo "LLM: FAILED"

    # Context API
    curl -s http://localhost:8091/api/v1/context/health >/dev/null && echo "Context API: OK" || echo "Context API: FAILED"

    # Container
    podman ps | grep -q holmesgpt-kubernaut-hybrid && echo "Container: OK" || echo "Container: FAILED"

    sleep 60
done
```

### Log Management
```bash
# Rotate container logs
podman logs holmesgpt-kubernaut-hybrid > holmesgpt-$(date +%Y%m%d).log

# Context API logs
# Check the Context API configuration for log file location
```

## Next Steps

After successful setup:

1. **Explore HolmesGPT Commands**: Try different investigation scenarios
2. **Customize Toolsets**: Modify the hybrid toolset for your specific needs
3. **Integration Testing**: Test with real alerts and incidents
4. **Performance Monitoring**: Monitor resource usage and response times
5. **Security Hardening**: Implement additional security measures for production use

## Support and Documentation

- **Architecture Guide**: `docs/deployment/HOLMESGPT_HYBRID_ARCHITECTURE.md`
- **Toolset Configuration**: `config/holmesgpt-hybrid-toolset.yaml`
- **Setup Scripts**: `scripts/setup-holmesgpt-environment.sh`
- **HolmesGPT Documentation**: Official HolmesGPT documentation
- **Kubernaut Documentation**: Project documentation in `docs/`
