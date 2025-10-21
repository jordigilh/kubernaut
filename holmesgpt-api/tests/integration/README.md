# HolmesGPT API - Integration Tests

This directory contains integration tests for the HolmesGPT API service.

## Test Modes

### 1. Mock Integration (Default)

**Use Case**: Local development without cluster access

```bash
# Run with mock Context API server
cd holmesgpt-api
pytest tests/integration/test_context_api_integration.py -v
```

**Behavior**:
- Creates local mock HTTP server
- Returns predefined test data
- Fast execution
- No external dependencies

### 2. Real Integration

**Use Case**: CI/CD pipeline with real Kubernetes cluster

```bash
# Set Context API URL environment variable
export CONTEXT_API_URL="http://context-api.kubernaut-system.svc.cluster.local:8091"

# Run tests against real Context API
cd holmesgpt-api
pytest tests/integration/test_context_api_integration.py -v
```

**Behavior**:
- Connects to real Context API service
- Uses actual historical data from PostgreSQL
- Tests real Redis caching
- Validates production behavior

**Prerequisites**:
- Context API deployed in `kubernaut-system` namespace
- PostgreSQL with pgvector extension available
- Redis cache service available
- Network access to Context API service

### 3. OpenShift/Kubernetes Integration

**From within cluster** (e.g., CI pod):

```bash
# Context API is accessible via Kubernetes DNS
export CONTEXT_API_URL="http://context-api.kubernaut-system.svc.cluster.local:8091"

# Run tests
pytest tests/integration/test_context_api_integration.py -v --tb=short
```

**From outside cluster** (e.g., local machine with oc/kubectl):

```bash
# Port-forward Context API service
oc port-forward -n kubernaut-system svc/context-api 8091:8091 &

# Set URL to localhost
export CONTEXT_API_URL="http://localhost:8091"

# Run tests
cd holmesgpt-api
pytest tests/integration/test_context_api_integration.py -v
```

## Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `CONTEXT_API_URL` | Context API base URL | `http://context-api.kubernaut-system.svc.cluster.local:8091` |

**Note**: If `CONTEXT_API_URL` is not set or empty, tests use mock server.

## Test Coverage

### Context API Integration

| Test Suite | Mock Mode | Real Mode | Purpose |
|------------|-----------|-----------|---------|
| **Client Initialization** | ✅ | ✅ | URL configuration, env vars |
| **Health Check** | ✅ | ✅ | Service availability |
| **Historical Context** | ✅ | ✅ | Data retrieval |
| **Success Rates** | ✅ | ✅ | Remediation action success data |
| **Similar Incidents** | ✅ | ✅ | Semantic search results |
| **Environment Patterns** | ✅ | ✅ | Historical patterns |
| **Error Handling** | ✅ | ✅ | Graceful degradation |
| **Performance** | ✅ | ✅ | Timeout, concurrent requests |

### Validation Strategy

**Mock Mode**:
- Validates exact response structure
- Tests client behavior with known data
- Fast feedback for development

**Real Mode**:
- Validates service availability
- Tests actual data retrieval
- Allows flexible response structure
- Validates production-like behavior

## CI/CD Integration

### GitHub Actions Example

```yaml
- name: Run Integration Tests
  env:
    CONTEXT_API_URL: http://context-api.kubernaut-system.svc.cluster.local:8091
  run: |
    cd holmesgpt-api
    pytest tests/integration/ -v --tb=short --maxfail=3
```

### OpenShift Pipeline Example

```yaml
- name: integration-tests
  image: quay.io/jordigilh/kubernaut-holmesgpt-api:latest
  env:
    - name: CONTEXT_API_URL
      value: "http://context-api.kubernaut-system.svc.cluster.local:8091"
  command:
    - /bin/sh
  args:
    - -c
    - |
      pytest tests/integration/ -v --tb=short
```

## Troubleshooting

### Tests Fail in Real Mode

```bash
# 1. Verify Context API is running
oc get pods -n kubernaut-system -l app=context-api

# 2. Test Context API health
curl http://context-api.kubernaut-system.svc.cluster.local:8091/health

# 3. Check Context API logs
oc logs -n kubernaut-system -l app=context-api --tail=50

# 4. Verify PostgreSQL and Redis are available
oc get pods -n kubernaut-system -l app=postgres
oc get pods -n kubernaut-system -l app=redis
```

### Port-Forward Not Working

```bash
# Kill existing port-forwards
pkill -f "port-forward.*context-api"

# Restart port-forward
oc port-forward -n kubernaut-system svc/context-api 8091:8091
```

### Network Policy Issues

If tests timeout in real mode, check NetworkPolicy:

```bash
# Verify Context API NetworkPolicy allows ingress
oc get networkpolicy -n kubernaut-system context-api -o yaml
```

## Test Development

### Adding New Integration Tests

1. **Add test to appropriate test suite**
2. **Use `context_api_base_url` fixture for URL**
3. **Add mode-specific assertions** (mock vs real)
4. **Document test mode behavior**

**Example**:

```python
@pytest.mark.asyncio
async def test_my_new_feature(self, context_api_base_url, context_api_mode):
    """
    Test description
    
    **Test Mode**: Real or Mock (based on CONTEXT_API_URL env var)
    """
    client = ContextAPIClient(base_url=context_api_base_url)
    
    result = await client.my_feature()
    
    # Mock mode: validate exact structure
    if context_api_mode == "mock":
        assert result["field"] == "expected_value"
    # Real mode: validate availability
    else:
        assert "field" in result or result.get("available") is False
```

## Related Documentation

- **Context API Deployment**: `docs/services/stateless/context-api/DEPLOYMENT.md`
- **HolmesGPT API README**: `holmesgpt-api/README.md`
- **Integration Test Patterns**: `docs/testing/integration-testing-guide.md`

