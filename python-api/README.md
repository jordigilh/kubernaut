# HolmesGPT REST API

FastAPI service providing HolmesGPT integration for alert investigation and automated remediation. Direct Python API integration with fail-fast startup validation.

## Features

- Direct HolmesGPT v0.13.1 Python API integration
- Multi-LLM provider support (OpenAI, Anthropic, Azure, AWS, Ollama/Ramalama)
- Async performance with caching and connection pooling
- Prometheus metrics and health monitoring
- Auto-generated OpenAPI documentation
- Production-ready logging and error handling
- Fail-fast startup validation

## Quick Start

### Docker Compose
```bash
cd python-api
docker-compose up -d
```

**Access:**
- API: http://localhost:8000
- Documentation: http://localhost:8000/docs
- Metrics: http://localhost:9090/metrics

### Local Development
```bash
pip install -r requirements.txt
cp env.example .env
# Configure LLM provider credentials in .env
uvicorn app.main:app --reload --host 0.0.0.0 --port 8000
```

## Configuration

### Required Environment Variables

```env
# LLM Provider Configuration
HOLMES_LLM_PROVIDER=openai
OPENAI_API_KEY=your_openai_api_key
HOLMES_DEFAULT_MODEL=gpt-4

# Optional Performance Settings
CACHE_ENABLED=true
CACHE_TTL=300
REQUEST_TIMEOUT=600
MAX_CONCURRENT_REQUESTS=10
```

### Supported LLM Providers

**OpenAI:**
```env
HOLMES_LLM_PROVIDER=openai
OPENAI_API_KEY=sk-...
OPENAI_BASE_URL=https://api.openai.com/v1  # optional
```

**Anthropic:**
```env
HOLMES_LLM_PROVIDER=anthropic
ANTHROPIC_API_KEY=sk-ant-...
```

**Azure OpenAI:**
```env
HOLMES_LLM_PROVIDER=azure
AZURE_OPENAI_API_KEY=your_key
AZURE_OPENAI_ENDPOINT=https://your-resource.openai.azure.com/
```

**AWS Bedrock:**
```env
HOLMES_LLM_PROVIDER=bedrock
AWS_ACCESS_KEY_ID=your_access_key
AWS_SECRET_ACCESS_KEY=your_secret_key
AWS_REGION=us-east-1
```

**Ollama (On-Premises):**
```env
HOLMES_LLM_PROVIDER=ollama
OLLAMA_BASE_URL=http://ollama:11434
HOLMES_DEFAULT_MODEL=llama3.1:8b
```

**Ramalama (Local):**
```env
HOLMES_LLM_PROVIDER=ramalama
RAMALAMA_BASE_URL=http://localhost:8080
HOLMES_DEFAULT_MODEL=llama3.1:8b
```

## API Endpoints

### Health Check
```bash
GET /health
```

### Ask Question
```bash
POST /ask
Content-Type: application/json

{
  "prompt": "What could cause high memory usage in a Kubernetes pod?",
  "context": {
    "environment": "production",
    "namespace": "default"
  },
  "options": {
    "max_tokens": 1000,
    "temperature": 0.1
  }
}
```

### Investigate Alert
```bash
POST /investigate
Content-Type: application/json

{
  "alert": {
    "name": "HighMemoryUsage",
    "severity": "warning",
    "status": "firing",
    "labels": {
      "namespace": "production",
      "pod": "api-server-abc123"
    }
  },
  "context": {
    "kubernetes_context": {
      "deployment": "api-server",
      "replicas": 3
    },
    "prometheus_context": {
      "memory_usage": "85%"
    }
  }
}
```

## Monitoring

### Health Endpoint
```bash
curl http://localhost:8000/health
```

Response:
```json
{
  "healthy": true,
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z",
  "checks": {
    "holmesgpt": {
      "status": "healthy",
      "response_time": 0.123
    }
  }
}
```

### Metrics
Prometheus metrics available at `/metrics`:
- `holmesgpt_operations_total`
- `holmesgpt_operation_duration_seconds`
- `http_requests_total`
- `cache_operations_total`

## Performance

### Resource Requirements

**Development:**
- Memory: 256Mi
- CPU: 100m

**Production:**
- Memory: 512Mi-2Gi (depending on model complexity)
- CPU: 250m-1000m
- Replicas: 2-3 for high availability

### Response Times

| Operation | Typical Latency |
|-----------|----------------|
| Health Check | 10-50ms |
| Ask (simple) | 1-3s |
| Investigate | 2-5s |
| Complex Analysis | 3-8s |

## Development

### Project Structure
```
app/
├── main.py              # FastAPI application
├── config.py            # Configuration management
├── models/
│   ├── requests.py      # Request models
│   └── responses.py     # Response models
├── services/
│   ├── holmes_service.py       # Main service layer
│   └── holmesgpt_wrapper.py    # HolmesGPT integration
└── utils/
    ├── cache.py         # Caching utilities
    ├── metrics.py       # Prometheus metrics
    └── logging.py       # Structured logging
```

### Running Tests
```bash
# Install test dependencies
pip install pytest pytest-asyncio httpx

# Run tests
pytest tests/
```

### Docker Build
```bash
docker build -t holmesgpt-api .
```

## Troubleshooting

### Common Issues

**Service fails to start:**
- Verify LLM provider API key is valid
- Check HolmesGPT dependency installation
- Review startup logs for specific errors

**High latency:**
- Enable caching with `CACHE_ENABLED=true`
- Reduce `HOLMES_DEFAULT_MAX_TOKENS`
- Use faster models (e.g., gpt-3.5-turbo vs gpt-4)

**Authentication errors:**
- Verify API key format for your provider
- Check API key permissions and quotas
- Ensure base URLs are correct for Azure/custom endpoints

### Debug Logging
```env
LOG_LEVEL=DEBUG
```

This enables detailed logging including request/response data and timing information.
