# Mock LLM Service

Standalone OpenAI-compatible mock LLM server for integration and E2E testing.

## Overview

This service provides a deterministic mock LLM endpoint that mimics OpenAI API behavior, enabling reliable testing of HolmesGPT workflows without requiring real LLM calls.

**Key Features**:
- ✅ OpenAI API compatible (`/v1/chat/completions`)
- ✅ Tool call support (`search_workflow_catalog`, etc.)
- ✅ Multi-turn conversation handling
- ✅ Pre-defined scenarios for common test cases
- ✅ Health and metrics endpoints
- ✅ Zero external dependencies (Python stdlib only)

## Quick Start

### Local Development

```bash
# Run directly
cd test/services/mock-llm
python -m src.server

# Or with Python path
PYTHONPATH=. python src/server.py

# Server starts on http://127.0.0.1:11434
```

### In Python Tests

```python
from test.services.mock_llm.src import MockLLMServer

# Basic usage
with MockLLMServer(port=11434) as server:
    # Configure your LLM client
    llm_client = LLMClient(endpoint=server.url, model="mock-model")

    # Run your tests
    response = llm_client.chat("analyze OOMKilled signal")
    # ...

# With custom scenario
with MockLLMServer() as server:
    server.set_scenario("oomkilled")
    # Test OOMKilled-specific behavior
```

### In Go Integration Tests

```go
// Use programmatic podman deployment (see test/infrastructure/)
container, err := StartMockLLMContainer(ctx)
defer StopMockLLMContainer(ctx, container)

// Mock LLM available at http://localhost:30089
```

## API Endpoints

### Core Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/chat/completions` | OpenAI-compatible chat endpoint |
| `POST` | `/chat/completions` | Alias for OpenAI endpoint |
| `POST` | `/api/generate` | Ollama-compatible endpoint |
| `GET` | `/v1/models` | List available models |

### Health & Monitoring

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Overall health status |
| `GET` | `/readiness` | Kubernetes readiness probe |
| `GET` | `/liveness` | Kubernetes liveness probe |
| `GET` | `/metrics` | Prometheus-style metrics |

### Example Responses

**Health Check**:
```bash
$ curl http://localhost:11434/health
{
  "status": "healthy",
  "service": "mock-llm",
  "version": "1.0.0"
}
```

**Metrics**:
```bash
$ curl http://localhost:11434/metrics
{
  "uptime_seconds": 123.45,
  "requests_total": 42,
  "tool_calls_total": 15,
  "errors_total": 0,
  "requests_per_second": 0.34,
  "service": "mock-llm",
  "version": "1.0.0"
}
```

## Pre-defined Scenarios

The mock LLM includes pre-configured scenarios for common test cases:

| Scenario | Signal Type | Use Case |
|----------|-------------|----------|
| `oomkilled` | `OOMKilled` | Memory limit exceeded |
| `crashloop` | `CrashLoopBackOff` | Pod restart loops |
| `node_not_ready` | `NodeNotReady` | Node health issues |
| `recovery` | `OOMKilled` (recovery) | Failed remediation retry |

**Setting a scenario**:
```python
server.set_scenario("oomkilled")
```

Scenarios automatically detect signal types from prompt content:
```python
# Automatically uses "oomkilled" scenario
response = client.chat("analyze OOMKilled in production/api-server")
```

## Tool Call Support

The mock LLM supports tool calls following OpenAI's function calling format:

### Phase 1: Initial Request → Tool Call
```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "content": null,
      "tool_calls": [{
        "id": "call_abc123",
        "type": "function",
        "function": {
          "name": "search_workflow_catalog",
          "arguments": "{\"query\":\"OOMKilled critical\", ...}"
        }
      }]
    },
    "finish_reason": "tool_calls"
  }]
}
```

### Phase 2: After Tool Result → Final Analysis
```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "content": "Based on my investigation...\n\n```json\n{...}\n```"
    },
    "finish_reason": "stop"
  }]
}
```

## Tool Call Validation

Tests can validate that tools were called correctly:

```python
with MockLLMServer() as server:
    # Run test that should call search_workflow_catalog
    response = client.analyze_incident(...)

    # Validate tool was called
    call = server.assert_tool_called("search_workflow_catalog")
    assert call.arguments["query"] == "OOMKilled critical"

    # Or with specific arguments
    server.assert_tool_called_with(
        "search_workflow_catalog",
        query="OOMKilled critical"
    )
```

## Metrics Collection

The server automatically collects metrics:

```python
server = MockLLMServer()
server.start()

# After running tests
metrics = server.get_metrics()
print(f"Total requests: {metrics['requests_total']}")
print(f"Tool calls: {metrics['tool_calls_total']}")
print(f"Errors: {metrics['errors_total']}")
```

## Backward Compatibility

For tests that don't use tool calls:

```python
with MockLLMServer(force_text_response=True) as server:
    # Always returns text responses, never tool calls
    response = client.chat("analyze incident")
    # Response will have content field populated
```

## Architecture

```
test/services/mock-llm/
├── src/
│   ├── __init__.py           # Package exports
│   └── server.py             # Main server implementation
├── tests/
│   └── test_server.py        # Unit tests (to be created)
├── requirements.txt          # Production dependencies (none)
├── requirements-dev.txt      # Development dependencies
└── README.md                 # This file
```

## Deployment

### Integration Tests (Programmatic Podman)

```go
// test/infrastructure/mock_llm.go
func StartMockLLMContainer(ctx context.Context) (string, error) {
    cmd := exec.CommandContext(ctx, "podman", "run",
        "-d",
        "--rm",
        "--name", "mock-llm-test",
        "-p", "30089:8080",
        "localhost/mock-llm:latest",
    )
    // ... error handling
}

func StopMockLLMContainer(ctx context.Context, containerID string) error {
    // ... cleanup
}
```

### E2E Tests (Kind)

```yaml
# deploy/mock-llm/deployment.yaml
apiVersion: v1
kind: Service
metadata:
  name: mock-llm
spec:
  type: NodePort
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: 30089
```

## Port Allocation

**Authoritative Reference**: `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md`

- **Integration Tests**: `30089` (programmatic podman)
- **E2E Tests**: `30089` (Kind NodePort)

## Testing

```bash
# Run unit tests
cd test/services/mock-llm
pytest tests/ -v

# With coverage
pytest tests/ -v --cov=src --cov-report=term-missing

# Type checking
mypy src/

# Linting
ruff check src/
```

## Troubleshooting

### Server won't start
```bash
# Check if port is already in use
lsof -i :11434

# Use auto-assigned port
with MockLLMServer(port=0) as server:
    print(f"Running on {server.url}")
```

### Tool calls not working
```bash
# Ensure tools are passed in request
request_data = {
    "messages": [...],
    "tools": [{"type": "function", "function": {"name": "search_workflow_catalog", ...}}]
}

# Verify force_text_response is False
server = MockLLMServer(force_text_response=False)
```

### Metrics not resetting
```python
# Manually reset metrics between tests
from test.services.mock_llm.src.server import metrics_collector
metrics_collector.reset()
```

## Version History

- **v1.0.0** (2026-01-11): Initial standalone service extraction
  - Extracted from HAPI test code
  - Added health/readiness/liveness endpoints
  - Added metrics collection
  - Zero external dependencies
  - Tool call support preserved
  - Multi-scenario support

## License

Copyright 2025 Jordi Gil. Licensed under Apache License 2.0.
