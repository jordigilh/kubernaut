# WARP.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Project Overview

**prometheus-alerts-slm** is a proof-of-concept application that integrates Prometheus alerts with IBM/Red Hat's Granite Small Language Model (SLM) to automatically respond to monitoring alerts by applying changes to an OpenShift/Kubernetes cluster.

### High-Level Architecture

```
[Prometheus] → [Alert Manager] → [Webhook Receiver]
                                         ↓
                                  [Alert Processor]
                                         ↓
                              [SLM Engine (Granite)]
                                         ↓
                                 [Action Executor]
                                         ↓
                              [OpenShift MCP Client]
                                         ↓
                                 [OpenShift Cluster]
```

### Key Components

1. **Webhook Receiver**: HTTP endpoint that receives Prometheus AlertManager webhooks
2. **Alert Processor**: Parses and filters alerts, preparing them for SLM analysis
3. **SLM Integration**: Connects to Granite model to analyze alerts and determine actions
4. **Action Executor**: Executes the SLM-recommended actions via MCP
5. **MCP Client**: Model Context Protocol client for OpenShift operations

## Development Environment

### Prerequisites

- Go 1.23.9+ (arm64/amd64)
- OpenShift CLI (oc) 4.18.6+
- Active OpenShift cluster connection (`oc whoami` should work)
- Git for version control

### Initial Setup

```bash
# Initialize Go module
go mod init github.com/jordigilh/prometheus-alerts-slm

# Create basic project structure
mkdir -p cmd/prometheus-alerts-slm pkg/{webhook,processor,slm,executor,mcp} internal/config

# Install core dependencies
go get k8s.io/client-go@latest
go get k8s.io/apimachinery@latest
go get github.com/prometheus/alertmanager@latest
go get github.com/sirupsen/logrus@latest
```

### Environment Variables

```bash
# SLM Configuration
export SLM_ENDPOINT="https://granite-api.example.com/v1/completions"
export SLM_API_KEY="your-api-key"
export SLM_MODEL="granite-8b-instruct"

# OpenShift Configuration
export OPENSHIFT_CONTEXT="openshift-sso/api-stress-parodos-dev:6443/kube:admin"
export OPENSHIFT_NAMESPACE="default"

# Application Configuration
export WEBHOOK_PORT="8080"
export LOG_LEVEL="debug"
export METRICS_PORT="9090"
```

## Common Development Commands

### Building

```bash
# Build the application
go build -o bin/prometheus-alerts-slm ./cmd/prometheus-alerts-slm

# Build with version information
go build -ldflags "-X main.version=$(git describe --tags --always)" \
         -o bin/prometheus-alerts-slm ./cmd/prometheus-alerts-slm

# Install locally
go install ./cmd/prometheus-alerts-slm
```

### Testing

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Run specific package tests
go test ./pkg/webhook -v

# Run integration tests (requires cluster access)
go test ./test/integration -tags=integration
```

### Running Locally

```bash
# Run with default configuration
./bin/prometheus-alerts-slm

# Run with custom config file
./bin/prometheus-alerts-slm --config config/local.yaml

# Run in development mode with hot reload (using air)
go install github.com/air-verse/air@latest
air

# Run with debug logging
LOG_LEVEL=debug ./bin/prometheus-alerts-slm
```

## Prometheus Alert Handling

### Webhook Receiver Configuration

```yaml
# config/webhook.yaml
webhook:
  port: 8080
  path: /alerts
  auth:
    type: bearer
    token: ${WEBHOOK_AUTH_TOKEN}
```

### Testing Alert Reception

```bash
# Send test alert to webhook
curl -X POST http://localhost:8080/alerts \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-token" \
  -d '{
    "version": "4",
    "groupKey": "test-group",
    "status": "firing",
    "receiver": "slm-processor",
    "alerts": [{
      "status": "firing",
      "labels": {
        "alertname": "HighMemoryUsage",
        "severity": "warning",
        "namespace": "production",
        "pod": "app-xyz"
      },
      "annotations": {
        "description": "Pod app-xyz is using 95% memory"
      }
    }]
  }'

# Simulate AlertManager webhook
curl -X POST http://localhost:8080/alerts \
  -H "Content-Type: application/json" \
  --data-binary @test/fixtures/sample-alert.json
```

### Alert Filtering Rules

```yaml
# config/filters.yaml
filters:
  - name: "critical-only"
    conditions:
      severity: ["critical", "warning"]
  - name: "production-namespace"
    conditions:
      namespace: ["production", "staging"]
```

## SLM (Granite) Integration

### Configuration

```yaml
# config/slm.yaml
slm:
  endpoint: ${SLM_ENDPOINT}
  model: granite-8b-instruct
  auth:
    type: api_key
    key: ${SLM_API_KEY}
  prompt:
    template: |
      Analyze this Kubernetes alert and recommend an action:
      Alert: {{.AlertName}}
      Severity: {{.Severity}}
      Description: {{.Description}}
      Namespace: {{.Namespace}}
      Resource: {{.Resource}}
      
      Available actions:
      - scale_deployment: Scale deployment replicas
      - restart_pod: Restart the affected pod
      - increase_resources: Increase CPU/memory limits
      - notify_only: No automated action, notify operators
      
      Respond with JSON: {"action": "...", "parameters": {...}}
```

### Testing SLM Integration

```bash
# Test SLM connection
go run cmd/test-slm/main.go --alert test/fixtures/high-memory.json

# Mock SLM for development
export SLM_MOCK=true
export SLM_MOCK_RESPONSE='{"action":"scale_deployment","parameters":{"replicas":3}}'
```

## OpenShift/Kubernetes Operations

### MCP Setup

```yaml
# config/mcp.yaml
mcp:
  context: ${OPENSHIFT_CONTEXT}
  namespace: ${OPENSHIFT_NAMESPACE}
  service_account: prometheus-alerts-slm
  rbac:
    - apiGroups: ["apps"]
      resources: ["deployments", "statefulsets"]
      verbs: ["get", "list", "patch", "update"]
    - apiGroups: [""]
      resources: ["pods"]
      verbs: ["get", "list", "delete"]
```

### Common Operations

```bash
# Check current context
oc whoami --show-context

# Create service account for the application
oc create serviceaccount prometheus-alerts-slm

# Grant necessary permissions
oc adm policy add-role-to-user edit -z prometheus-alerts-slm

# Test connectivity
oc get pods -n ${OPENSHIFT_NAMESPACE}

# Apply test deployment
oc apply -f test/manifests/test-deployment.yaml

# Watch pods for testing auto-remediation
oc get pods -w
```

### Action Execution Examples

```go
// pkg/executor/actions.go
type ActionExecutor interface {
    ScaleDeployment(namespace, name string, replicas int32) error
    RestartPod(namespace, name string) error
    UpdateResources(namespace, name string, resources ResourceRequirements) error
}
```

## Testing Strategies

### Unit Tests

```bash
# Test alert parsing
go test ./pkg/webhook -run TestParseAlert

# Test SLM prompt generation
go test ./pkg/slm -run TestGeneratePrompt

# Test action execution (with mocks)
go test ./pkg/executor -run TestScaleDeployment
```

### Integration Tests

```bash
# Run with test cluster
export KUBECONFIG=test/kubeconfig
go test ./test/integration -tags=integration

# Test end-to-end flow
go test ./test/e2e -run TestAlertToAction
```

### Load Testing

```bash
# Install vegeta for load testing
go install github.com/tsenart/vegeta@latest

# Generate load on webhook
echo "POST http://localhost:8080/alerts" | \
  vegeta attack -duration=30s -rate=100 -body=test/fixtures/alert.json | \
  vegeta report
```

## Configuration Management

### Configuration File Structure

```yaml
# config/app.yaml
app:
  name: prometheus-alerts-slm
  version: 1.0.0
  
server:
  webhook_port: 8080
  metrics_port: 9090
  health_port: 8081
  
logging:
  level: info
  format: json
  
slm:
  endpoint: ${SLM_ENDPOINT}
  timeout: 30s
  retry_count: 3
  
openshift:
  context: ${OPENSHIFT_CONTEXT}
  namespace: ${OPENSHIFT_NAMESPACE}
  
actions:
  dry_run: false
  max_concurrent: 5
  cooldown_period: 5m
```

### Deployment

```bash
# Build container image
podman build -t prometheus-alerts-slm:latest .

# Push to registry
podman push prometheus-alerts-slm:latest quay.io/jordigilh/prometheus-alerts-slm:latest

# Deploy to OpenShift
oc apply -f deploy/manifests/

# Create ConfigMap from config file
oc create configmap app-config --from-file=config/app.yaml

# Create Secret for sensitive data
oc create secret generic slm-credentials \
  --from-literal=api-key=${SLM_API_KEY}
```

## Troubleshooting

### Common Issues

```bash
# Check application logs
oc logs -f deployment/prometheus-alerts-slm

# Enable debug logging
oc set env deployment/prometheus-alerts-slm LOG_LEVEL=debug

# Check webhook connectivity
curl -v http://localhost:8080/health

# Verify SLM endpoint
curl -X POST ${SLM_ENDPOINT} \
  -H "Authorization: Bearer ${SLM_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"prompt": "test"}'

# Check OpenShift permissions
oc auth can-i update deployments

# Test MCP connection
oc proxy &
curl http://localhost:8001/api/v1/namespaces
```

### Debug Commands

```bash
# Run with delve debugger
dlv debug ./cmd/prometheus-alerts-slm

# Profile CPU usage
go run ./cmd/prometheus-alerts-slm -cpuprofile=cpu.prof
go tool pprof cpu.prof

# Profile memory usage
go run ./cmd/prometheus-alerts-slm -memprofile=mem.prof
go tool pprof mem.prof

# Trace execution
GODEBUG=trace=1 ./bin/prometheus-alerts-slm
```

## Project Structure

```
prometheus-alerts-slm/
├── cmd/
│   └── prometheus-alerts-slm/    # Main application entry point
│       └── main.go
├── pkg/                           # Public packages
│   ├── webhook/                   # Webhook receiver implementation
│   │   ├── handler.go
│   │   └── types.go
│   ├── processor/                 # Alert processing logic
│   │   ├── filter.go
│   │   └── parser.go
│   ├── slm/                       # SLM integration
│   │   ├── client.go
│   │   ├── prompt.go
│   │   └── response.go
│   ├── executor/                  # Action execution
│   │   ├── actions.go
│   │   └── kubernetes.go
│   └── mcp/                       # MCP protocol implementation
│       └── client.go
├── internal/                      # Private packages
│   ├── config/                    # Configuration management
│   │   └── config.go
│   └── metrics/                   # Prometheus metrics
│       └── collector.go
├── test/                          # Test files and fixtures
│   ├── fixtures/
│   ├── integration/
│   └── e2e/
├── deploy/                        # Deployment manifests
│   └── manifests/
├── config/                        # Configuration files
│   └── app.yaml
├── Dockerfile
├── Makefile
├── go.mod
└── go.sum
```

### Code Generation

```makefile
# Makefile targets
.PHONY: generate
generate:
	go generate ./...

.PHONY: mockgen
mockgen:
	mockgen -source=pkg/slm/client.go -destination=pkg/slm/mock_client.go

.PHONY: codegen
codegen:
	controller-gen crd paths=./pkg/... output:dir=deploy/crds
```

## Key Interfaces

```go
// pkg/webhook/handler.go
type AlertHandler interface {
    HandleAlert(ctx context.Context, alert Alert) error
}

// pkg/processor/filter.go
type AlertFilter interface {
    ShouldProcess(alert Alert) bool
}

// pkg/slm/client.go
type SLMClient interface {
    AnalyzeAlert(ctx context.Context, alert Alert) (Action, error)
}

// pkg/executor/actions.go
type ActionExecutor interface {
    Execute(ctx context.Context, action Action) error
}
```

## Performance Considerations

- Alert processing should complete within 30 seconds
- Implement circuit breakers for SLM API calls
- Use connection pooling for OpenShift API clients
- Implement rate limiting for action execution
- Cache SLM responses for similar alerts within a time window

## Security Notes

- Never log sensitive information (API keys, tokens)
- Use ServiceAccounts with minimal required permissions
- Validate all incoming webhook payloads
- Implement RBAC for OpenShift operations
- Use TLS for all external communications
- Rotate credentials regularly
