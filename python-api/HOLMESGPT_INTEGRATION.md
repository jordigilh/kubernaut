# HolmesGPT Python API Integration Guide

This document provides comprehensive information about integrating with HolmesGPT v0.13.1 using the Python API.

## Overview

The Python FastAPI service now includes **real HolmesGPT integration** using the official HolmesGPT Python library (v0.13.1). This integration supports multiple LLM providers and provides both direct Python API access and CLI fallback.

## Installation & Setup

### 1. Install HolmesGPT

The service automatically installs HolmesGPT from the GitHub repository:

```bash
# Included in requirements.txt
holmesgpt @ git+https://github.com/robusta-dev/holmesgpt.git@0.13.1
```

### 2. Configure LLM Provider

HolmesGPT requires an LLM provider. Configure one of the supported providers:

#### OpenAI (Recommended)
```bash
export HOLMES_LLM_PROVIDER=openai
export OPENAI_API_KEY=your_openai_api_key_here
export HOLMES_LLM_MODEL=gpt-4  # or gpt-3.5-turbo
```

#### Azure OpenAI
```bash
export HOLMES_LLM_PROVIDER=azure
export AZURE_OPENAI_API_KEY=your_azure_api_key
export AZURE_OPENAI_ENDPOINT=https://your-resource.openai.azure.com/
export AZURE_OPENAI_API_VERSION=2023-12-01-preview
export HOLMES_LLM_MODEL=gpt-4
```

#### Anthropic Claude
```bash
export HOLMES_LLM_PROVIDER=anthropic
export ANTHROPIC_API_KEY=your_anthropic_api_key
export HOLMES_LLM_MODEL=claude-3-sonnet-20240229
```

#### AWS Bedrock
```bash
export HOLMES_LLM_PROVIDER=bedrock
export AWS_ACCESS_KEY_ID=your_aws_access_key
export AWS_SECRET_ACCESS_KEY=your_aws_secret_key
export AWS_REGION=us-east-1
export HOLMES_LLM_MODEL=anthropic.claude-3-sonnet-20240229-v1:0
```

## Architecture

### Integration Modes

The service supports **dual integration** with intelligent fallback:

```python
# 1. Direct Python API (Primary)
from holmesgpt import Holmes
holmes = Holmes(llm_config=llm_config)
result = await holmes.ask(prompt)

# 2. CLI Fallback (Secondary)
subprocess: holmes ask "query" --format json
```

### Service Layer Architecture

```
FastAPI Application
├── HolmesGPTService (Main Service)
│   ├── HolmesGPTWrapper (Python API)
│   └── CLI Fallback (Process Execution)
└── Configuration Management
    ├── LLM Provider Config
    └── HolmesGPT Options
```

## API Usage Examples

### Basic Ask Operation

```bash
curl -X POST "http://localhost:8000/ask" \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "My Kubernetes pods are crash looping. How do I investigate?",
    "context": {
      "kubernetes_context": {
        "namespace": "production",
        "deployment": "api-server"
      },
      "environment": "production"
    },
    "options": {
      "max_tokens": 2000,
      "temperature": 0.1,
      "model": "gpt-4"
    }
  }'
```

### Alert Investigation

```bash
curl -X POST "http://localhost:8000/investigate" \
  -H "Content-Type: application/json" \
  -d '{
    "alert": {
      "name": "HighMemoryUsage",
      "severity": "warning",
      "status": "firing",
      "starts_at": "2024-01-15T10:30:00Z",
      "labels": {
        "instance": "api-server-pod",
        "namespace": "production",
        "container": "api-server"
      },
      "annotations": {
        "description": "Memory usage above 80% for 5 minutes",
        "summary": "High memory usage detected"
      }
    },
    "investigation_context": {
      "include_metrics": true,
      "include_logs": true,
      "time_range": "2h"
    },
    "options": {
      "max_tokens": 3000,
      "temperature": 0.1
    }
  }'
```

## Configuration Reference

### Core HolmesGPT Settings

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `HOLMES_DIRECT_IMPORT` | Enable Python API integration | `true` |
| `HOLMES_CLI_FALLBACK` | Enable CLI fallback | `true` |
| `HOLMES_LLM_PROVIDER` | LLM provider (openai/azure/anthropic/bedrock) | `openai` |
| `HOLMES_LLM_MODEL` | Model to use | Provider-specific |
| `HOLMES_DEFAULT_MAX_TOKENS` | Default max tokens | `4000` |
| `HOLMES_DEFAULT_TEMPERATURE` | Default temperature | `0.3` |

### Provider-Specific Settings

#### OpenAI
- `OPENAI_API_KEY`: Your OpenAI API key
- `OPENAI_BASE_URL`: Custom base URL (optional)
- `OPENAI_ORGANIZATION`: Organization ID (optional)

#### Azure OpenAI
- `AZURE_OPENAI_API_KEY`: Azure OpenAI API key
- `AZURE_OPENAI_ENDPOINT`: Azure OpenAI endpoint
- `AZURE_OPENAI_API_VERSION`: API version

#### Anthropic
- `ANTHROPIC_API_KEY`: Anthropic API key

#### AWS Bedrock
- `AWS_ACCESS_KEY_ID`: AWS access key
- `AWS_SECRET_ACCESS_KEY`: AWS secret key
- `AWS_REGION`: AWS region

## Features & Capabilities

### 🎯 **Intelligent Integration**
- **Automatic Provider Detection**: Detects available LLM providers
- **Graceful Fallback**: Python API → CLI → Mock (for testing)
- **Health Monitoring**: Comprehensive health checks for all components

### 🚀 **Enhanced Performance**
- **Async Operations**: Full async/await support
- **Response Caching**: Configurable TTL-based caching
- **Connection Pooling**: Efficient resource management
- **Background Processing**: Non-blocking operations

### 📊 **Rich Context Support**
- **Kubernetes Integration**: Pod, deployment, namespace context
- **Prometheus Metrics**: Historical metrics integration
- **Alert Correlation**: Multi-alert pattern analysis
- **Custom Context**: Flexible context injection

### 🔍 **Advanced Investigation**
- **Root Cause Analysis**: AI-powered investigation
- **Remediation Recommendations**: Step-by-step solutions
- **Risk Assessment**: Confidence scoring and risk levels
- **Historical Learning**: Pattern recognition from past incidents

## Response Format

### Ask Response
```json
{
  "response": "Based on the crash looping pattern, I recommend...",
  "analysis": {
    "summary": "Pod crash loop analysis",
    "root_cause": "Memory limit exceeded",
    "urgency_level": "high",
    "affected_components": ["api-server", "database-connection"]
  },
  "recommendations": [
    {
      "action": "increase_memory_limit",
      "description": "Increase pod memory limit to 2Gi",
      "command": "kubectl patch deployment api-server -p '{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"api-server\",\"resources\":{\"limits\":{\"memory\":\"2Gi\"}}}]}}}}'",
      "risk": "low",
      "confidence": 0.9,
      "estimated_time": "2-3 minutes"
    }
  ],
  "confidence": 0.85,
  "model_used": "gpt-4",
  "processing_time": 2.3,
  "sources": ["kubernetes", "prometheus"]
}
```

### Investigation Response
```json
{
  "investigation": {
    "alert_analysis": {
      "summary": "Memory usage investigation for api-server",
      "root_cause": "Memory leak in session handling",
      "impact_assessment": "High - affects user experience",
      "urgency_level": "critical",
      "affected_components": ["api-server", "session-store"],
      "related_metrics": {
        "memory_usage_percent": 95,
        "gc_frequency": "high",
        "heap_size": "1.8Gi"
      }
    },
    "evidence": {
      "memory_trend": "increasing",
      "error_rate": 0.15,
      "response_time_p95": "5.2s"
    },
    "remediation_plan": [
      {
        "action": "restart_deployment",
        "description": "Restart deployment to clear memory leak",
        "risk": "medium",
        "confidence": 0.8
      },
      {
        "action": "enable_memory_profiling",
        "description": "Enable memory profiling for root cause analysis",
        "risk": "low",
        "confidence": 0.95
      }
    ]
  },
  "confidence": 0.88,
  "severity_assessment": "critical",
  "requires_human_intervention": true,
  "processing_time": 4.7
}
```

## Monitoring & Observability

### Health Checks
```bash
# Check overall health
curl http://localhost:8000/health

# Check HolmesGPT-specific health
curl http://localhost:8000/service/info
```

### Metrics
The service exposes comprehensive metrics:

- `holmesgpt_operations_total`: Total HolmesGPT operations
- `holmesgpt_operation_duration_seconds`: Operation duration
- `holmesgpt_operation_confidence`: Average confidence scores
- `holmesgpt_api_requests_total`: API request counts

### Logging
Structured logging with correlation:

```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "level": "INFO",
  "operation": "holmes_ask",
  "duration": 2.3,
  "confidence": 0.85,
  "model_used": "gpt-4",
  "prompt_length": 156,
  "tokens_used": 1250
}
```

## Troubleshooting

### Common Issues

#### 1. HolmesGPT Import Failed
```bash
# Check if HolmesGPT is installed
pip list | grep holmesgpt

# Reinstall if needed
pip install "git+https://github.com/robusta-dev/holmesgpt.git@0.13.1"
```

#### 2. LLM Provider Authentication
```bash
# Verify API key
curl -H "Authorization: Bearer $OPENAI_API_KEY" \
  https://api.openai.com/v1/models

# Check logs for authentication errors
docker-compose logs holmesgpt-api | grep -i "auth\|key\|token"
```

#### 3. Performance Issues
```bash
# Check metrics
curl http://localhost:9090/metrics | grep holmesgpt

# Monitor resource usage
docker stats holmesgpt-api

# Check cache performance
curl http://localhost:8000/health | jq '.checks.cache'
```

### Debug Mode

Enable debug logging:
```bash
export LOG_LEVEL=DEBUG
export HOLMES_ENABLE_DEBUG=true
```

## Development & Testing

### Local Development
```bash
# Start with real HolmesGPT integration
export OPENAI_API_KEY=your_key_here
./start-dev.sh

# Test integration
curl -X POST http://localhost:8000/ask \
  -H "Content-Type: application/json" \
  -d '{"prompt": "Test HolmesGPT integration"}'
```

### Testing with Mock
```bash
# Disable real integration for testing
export HOLMES_DIRECT_IMPORT=false
export HOLMES_CLI_FALLBACK=false

# Run tests
pytest tests/ -v
```

## Production Deployment

### Docker Deployment
```bash
# Build with HolmesGPT integration
docker build --target production -t holmesgpt-api:latest .

# Run with environment variables
docker run -d \
  --name holmesgpt-api \
  -p 8000:8000 \
  -e OPENAI_API_KEY=$OPENAI_API_KEY \
  -e HOLMES_LLM_PROVIDER=openai \
  -e HOLMES_LLM_MODEL=gpt-4 \
  holmesgpt-api:latest
```

### Kubernetes Deployment
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: holmesgpt-api
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: holmesgpt-api
        image: holmesgpt-api:latest
        env:
        - name: OPENAI_API_KEY
          valueFrom:
            secretKeyRef:
              name: holmesgpt-secrets
              key: openai-api-key
        - name: HOLMES_LLM_PROVIDER
          value: "openai"
        - name: HOLMES_LLM_MODEL
          value: "gpt-4"
```

## Security Considerations

### API Key Management
- Store API keys in secure secret management systems
- Use environment variables, never hardcode keys
- Rotate keys regularly
- Monitor API usage and costs

### Network Security
- Use HTTPS in production
- Implement proper authentication/authorization
- Configure rate limiting
- Monitor for suspicious activity

### Data Privacy
- Ensure compliance with data protection regulations
- Implement data retention policies
- Consider data anonymization for logs
- Review LLM provider data handling policies

## Performance Optimization

### Caching Strategy
```python
# Configure caching
CACHE_ENABLED=true
CACHE_TTL=300  # 5 minutes
CACHE_MAX_SIZE=1000
```

### Resource Limits
```yaml
resources:
  requests:
    memory: "512Mi"
    cpu: "250m"
  limits:
    memory: "2Gi"
    cpu: "1000m"
```

### Scaling Considerations
- Use horizontal pod autoscaling
- Monitor token usage and costs
- Implement circuit breakers for LLM calls
- Consider request queuing for high load

---

## Support & Contributing

For issues, questions, or contributions:

- **HolmesGPT Issues**: [GitHub Issues](https://github.com/robusta-dev/holmesgpt/issues)
- **API Documentation**: http://localhost:8000/docs
- **Health Status**: http://localhost:8000/health

This integration provides a **production-ready, scalable solution** for incorporating HolmesGPT's AI-powered investigation capabilities into your monitoring and alerting workflows.

