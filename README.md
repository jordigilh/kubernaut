# Prometheus Alerts SLM

A proof-of-concept application that integrates Prometheus alerts with IBM Granite Small Language Models via Ollama to automatically respond to monitoring alerts by applying changes to an OpenShift/Kubernetes cluster.

## Features

- 🔗 **AlertManager Webhook Integration** - Receives Prometheus alerts via HTTP webhook
- 🧠 **IBM Granite Model Analysis** - Uses Granite 3.1 Dense 8B model via Ollama for intelligent alert analysis
- ⚡ **Automated Remediation** - Executes recommended actions on Kubernetes/OpenShift clusters
- 🚀 **Production Ready** - No mock dependencies, full LocalAI/Ollama integration
- 📊 **Observability** - Comprehensive logging and Prometheus metrics
- 🔒 **Security** - RBAC integration and secure webhook authentication

## Architecture

```
AlertManager → Webhook → Business Logic → Ollama/Granite → Action Execution → K8s/OpenShift
```

## Quick Start

### Prerequisites

- Go 1.23.9+
- Ollama installed and running
- OpenShift/Kubernetes cluster access
- IBM Granite model downloaded

### 1. Install Ollama and Granite Model

```bash
# Install Ollama (if not already installed)
curl -fsSL https://ollama.ai/install.sh | sh

# Pull and start Granite model
ollama pull granite3.1-dense:8b
ollama serve
```

### 2. Build and Run

```bash
# Clone and build
git clone <repository>
cd prometheus-alerts-slm
make build

# Test Ollama integration
./bin/test-slm

# Run application (dry-run mode)
SLM_PROVIDER="localai" \
SLM_ENDPOINT="http://localhost:11434" \
SLM_MODEL="granite3.1-dense:8b" \
DRY_RUN="true" \
./bin/prometheus-alerts-slm
```

### 3. Test Webhook

```bash
# Send test alert
curl -X POST http://localhost:8080/alerts \
  -H "Content-Type: application/json" \
  -d @test/fixtures/sample-alert.json
```

## Configuration

### Environment Variables

```bash
# SLM Configuration
export SLM_PROVIDER="localai"
export SLM_ENDPOINT="http://localhost:11434"
export SLM_MODEL="granite3.1-dense:8b"
export SLM_TEMPERATURE="0.3"
export SLM_MAX_TOKENS="500"

# OpenShift Configuration
export OPENSHIFT_CONTEXT="your-context"
export OPENSHIFT_NAMESPACE="default"

# Application Configuration
export LOG_LEVEL="info"
export DRY_RUN="true"
export WEBHOOK_PORT="8080"
export METRICS_PORT="9090"
```

### Configuration File

Create `config/app.yaml`:

```yaml
slm:
  provider: localai
  endpoint: http://localhost:11434
  model: granite3.1-dense:8b
  temperature: 0.3
  max_tokens: 500
  
openshift:
  namespace: default
  
actions:
  dry_run: false
  max_concurrent: 5
  cooldown_period: 5m
```

## Available Actions

The Granite model can recommend these automated actions:

- **`scale_deployment`** - Scale deployment replicas up/down
- **`restart_pod`** - Restart affected pods
- **`increase_resources`** - Increase CPU/memory limits
- **`notify_only`** - No automation, manual intervention required

## Example Analysis

**Input Alert:**
```json
{
  "alertname": "HighMemoryUsage",
  "severity": "warning",
  "description": "Pod using 95% memory"
}
```

**Granite Analysis:**
```json
{
  "action": "increase_resources",
  "parameters": {
    "memory_limit": "2Gi"
  },
  "confidence": 0.90,
  "reasoning": "Pod is using 95% memory. Increasing limit provides headroom."
}
```

## Deployment

### Kubernetes

```bash
# Deploy with Kustomize
make k8s-deploy

# Check status
make k8s-status

# View logs
make k8s-logs
```

### Docker Compose

```bash
# Start services
docker-compose up -d

# View logs
docker-compose logs -f
```

## Development

### Build Commands

```bash
make build          # Build binary
make test           # Run tests
make docker-build   # Build container
make lint           # Run linter
```

### Testing

```bash
# Test SLM integration
./bin/test-slm

# Test webhook
make test-webhook

# Test health endpoints
make test-health
```

## Production Deployment

1. **Configure AlertManager** to send webhooks to the service
2. **Set up RBAC** permissions for cluster operations
3. **Configure monitoring** with Prometheus
4. **Set resource limits** appropriate for your workload
5. **Review cooldown settings** to prevent action storms

## Validation Results

✅ **No Mock Dependencies** - All mock functionality removed  
✅ **Ollama Integration** - Tested with Granite 3.1 Dense 8B model  
✅ **JSON Response Parsing** - Robust extraction of action recommendations  
✅ **Error Handling** - Proper retry logic and timeout handling  
✅ **Health Checks** - Uses Ollama's `/api/tags` endpoint  
✅ **Alert Processing** - Complete webhook to action execution flow  
✅ **OpenShift Operations** - MCP client for cluster operations  

## Example Test Output

```
=== Testing Ollama Integration with Granite Model ===
Provider: localai
Endpoint: http://localhost:11434
Model: granite3.1-dense:8b

Testing Ollama health check...
✅ Ollama is healthy

=== Granite Model Analysis Results ===
Action: increase_resources
Confidence: 0.90
Reasoning: Pod is using 95% memory. Increasing limit provides headroom.
Parameters:
  memory_limit: 2Gi

✅ SUCCESS: Ollama/Granite integration working correctly!
```

## License

Apache 2.0

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.