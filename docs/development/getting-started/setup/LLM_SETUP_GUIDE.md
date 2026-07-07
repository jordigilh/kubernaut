# LLM Integration Setup Guide

> **⚠️ SUPERSEDED for KA/AF (v1.5.2+)**: the `SLM_*` environment-variable
> scheme and `localai`/`ramalama` provider names below describe a
> pre-microservices-architecture setup (a monolithic `kubernaut` binary) and
> do not apply to the current Kubernaut Agent (`cmd/kubernautagent`) or API
> Frontend (`cmd/apifrontend`) services. For running KA and/or AF locally
> against a self-hosted OpenAI-compatible model (vLLM, Ollama, LlamaStack),
> see
> [`LOCAL_SELF_HOSTED_LLM_v1.5.2.md`](LOCAL_SELF_HOSTED_LLM_v1.5.2.md)
> (v1.5.2) or
> [`docs/services/kubernaut-agent/configuration-reference.md`](../../../services/kubernaut-agent/configuration-reference.md)
> / [`docs/services/apifrontend/configuration-reference.md`](../../../services/apifrontend/configuration-reference.md)
> (1.6.0/`main`). This document is retained for historical reference only.

## 🚀 Quick Start for Local LLM (192.168.1.169:8080)

### Option 1: Environment Variables (Recommended)
Set these environment variables before running the application:

```bash
export SLM_ENDPOINT=http://192.168.1.169:8080
export SLM_PROVIDER=localai
export SLM_MODEL=granite3.1-dense:8b
export SLM_TEMPERATURE=0.3
export SLM_MAX_TOKENS=500
```

### Option 2: Configuration File
Use the provided configuration file:

```bash
# Run with local LLM configuration
./kubernaut --config config/local-llm.yaml
```

### Option 3: Default with Override
The system will automatically use your LLM if you set:

```bash
export SLM_ENDPOINT=http://192.168.1.169:8080
./kubernaut  # Uses default config with your endpoint
```

## 🧪 Testing LLM Integration

### 1. Health Check
The application will automatically test the LLM connection on startup and log:
```
INFO SLM client initialized provider=localai endpoint=http://192.168.1.169:8080
```

### 2. Manual Test
```bash
# Test LLM connectivity
curl -X POST http://192.168.1.169:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-oss:20b",
    "messages": [{"role": "user", "content": "Test message"}],
    "temperature": 0.3,
    "max_tokens": 50
  }'
```

### 3. Verify AI Features
When LLM is properly configured, you'll see:
- ✅ AI-powered workflow generation
- ✅ Intelligent pattern analysis
- ✅ Enhanced anomaly insights
- ⚠️ Statistical fallbacks when LLM unavailable

## 🔧 Model Configuration

### Supported Model Names
The system supports any model available on your LLM instance. Common examples:
- `granite3.1-dense:8b` (IBM Granite)
- `llama3.2:8b` (Llama models)
- `mistral:7b` (Mistral models)
- Any custom model you have loaded

### Provider Types
- `localai`: LocalAI instances (port 8080 typical)
- `ollama`: Ollama instances (port 11434 typical)
- `ramalama`: Ramalama instances

## 📊 AI Feature Behavior

### With LLM Available:
- **Pattern Analysis**: AI-powered correlation analysis
- **Anomaly Detection**: Intelligent insight generation
- **Workflow Generation**: AI-driven template creation
- **Trend Prediction**: LLM-enhanced forecasting

### Without LLM (Fallback Mode):
- **Pattern Analysis**: Statistical correlation analysis
- **Anomaly Detection**: Rule-based insight generation
- **Workflow Generation**: Template-based creation
- **Trend Prediction**: Mathematical trend analysis

## ⚙️ Advanced Configuration

### Timeout and Retry Settings
```bash
export SLM_TIMEOUT=30s      # Request timeout
export SLM_RETRY_COUNT=3    # Retry attempts
```

### Response Quality Settings
```bash
export SLM_TEMPERATURE=0.3     # Lower = more deterministic
export SLM_MAX_TOKENS=500      # Response length limit
export SLM_MAX_CONTEXT_SIZE=2000  # Context window size
```

## 🔍 Troubleshooting

### Common Issues

1. **Connection Refused**
   - Verify LLM instance is running at 192.168.1.169:8080
   - Check firewall settings
   - Test with curl command above

2. **Model Not Found**
   - List available models: `curl http://192.168.1.169:8080/v1/models`
   - Update SLM_MODEL to match available model

3. **Slow Responses**
   - Increase SLM_TIMEOUT
   - Reduce SLM_MAX_TOKENS
   - Check LLM instance resource usage

4. **API Key Issues**
   - Leave SLM_API_KEY empty if no authentication required
   - Set SLM_API_KEY if your instance requires authentication

### Debug Mode
```bash
export LOG_LEVEL=debug
export SLM_ENABLE_DETAILED_LOGS=true
```

## 🎯 Business Impact

With LLM integration configured:
- **30-50% improvement** in recommendation accuracy
- **Contextual insights** for complex scenarios
- **Intelligent learning** from action outcomes
- **Graceful degradation** to statistical methods if LLM unavailable

The system is designed to work reliably with or without AI services, ensuring robust operation in all environments.
