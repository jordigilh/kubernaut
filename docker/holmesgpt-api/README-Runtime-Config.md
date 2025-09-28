# HolmesGPT API Runtime Configuration

The HolmesGPT API container is now fully configurable at runtime using environment variables. You no longer need to rebuild the container to change the model endpoint or other settings.

## 🚀 Quick Start

### Use Your Current Model (localhost:8010)
```bash
podman run -d --network host \
  -e HOLMESGPT_LLM_BASE_URL=http://localhost:8010 \
  -e HOLMESGPT_LLM_PROVIDER=ollama \
  -e HOLMESGPT_LLM_MODEL=llama3.2 \
  holmesgpt-api:localhost-8010
```

### Use Standard Ollama (port 11434)
```bash
podman run -d --network host \
  -e HOLMESGPT_LLM_BASE_URL=http://localhost:11434 \
  -e HOLMESGPT_LLM_PROVIDER=ollama \
  -e HOLMESGPT_LLM_MODEL=llama3.2 \
  -e HOLMESGPT_PORT=8091 \
  holmesgpt-api:localhost-8010
```

### Use Remote Model Server
```bash
podman run -d --network host \
  -e HOLMESGPT_LLM_BASE_URL=http://192.168.1.169:8080 \
  -e HOLMESGPT_LLM_PROVIDER=ollama \
  -e HOLMESGPT_LLM_MODEL=your-model \
  -e HOLMESGPT_PORT=8092 \
  holmesgpt-api:localhost-8010
```

## 📋 Available Environment Variables

### LLM Configuration
| Variable | Default | Description | Examples |
|----------|---------|-------------|----------|
| `HOLMESGPT_LLM_BASE_URL` | `http://localhost:8010` | Model server endpoint | `http://localhost:11434`, `http://192.168.1.100:8080` |
| `HOLMESGPT_LLM_PROVIDER` | `ollama` | LLM provider type | `ollama`, `openai`, `anthropic` |
| `HOLMESGPT_LLM_MODEL` | `llama3.2` | Model name | `llama3.2`, `gpt-4`, `claude-3` |
| `HOLMESGPT_LLM_API_KEY` | _(none)_ | API key for external providers | `sk-...` for OpenAI |

### Server Configuration
| Variable | Default | Description |
|----------|---------|-------------|
| `HOLMESGPT_PORT` | `8090` | API server port |
| `HOLMESGPT_METRICS_PORT` | `9091` | Metrics server port |

### Logging Configuration
| Variable | Default | Description | Options |
|----------|---------|-------------|---------|
| `HOLMESGPT_LOG_LEVEL` | `INFO` | Logging level | `DEBUG`, `INFO`, `WARN`, `ERROR` |
| `HOLMESGPT_LOG_FORMAT` | `text` | Log output format | `text`, `json` |

### Security Configuration
| Variable | Default | Description |
|----------|---------|-------------|
| `HOLMESGPT_JWT_SECRET_KEY` | `dev-secret-key` | JWT signing key |
| `HOLMESGPT_JWT_ALGORITHM` | `HS256` | JWT algorithm |
| `HOLMESGPT_JWT_EXPIRE_MINUTES` | `60` | Token expiration time |

## 🛠️ Usage Examples

### Example 1: Development with Local Model
```bash
podman run -d --name holmesgpt-dev \
  --network host \
  -e HOLMESGPT_LLM_BASE_URL=http://localhost:8010 \
  -e HOLMESGPT_LOG_LEVEL=DEBUG \
  holmesgpt-api:localhost-8010

# Test: curl http://localhost:8090/health
```

### Example 2: Production with Remote Model
```bash
podman run -d --name holmesgpt-prod \
  --network host \
  -e HOLMESGPT_LLM_BASE_URL=http://production-model:8080 \
  -e HOLMESGPT_LLM_MODEL=production-llama \
  -e HOLMESGPT_LOG_LEVEL=WARN \
  -e HOLMESGPT_LOG_FORMAT=json \
  -e HOLMESGPT_JWT_SECRET_KEY=your-production-secret \
  holmesgpt-api:localhost-8010
```

### Example 3: Multiple Instances with Different Models
```bash
# Instance 1: Local model
podman run -d --name holmesgpt-local \
  --network host \
  -e HOLMESGPT_LLM_BASE_URL=http://localhost:8010 \
  -e HOLMESGPT_PORT=8090 \
  holmesgpt-api:localhost-8010

# Instance 2: Remote model
podman run -d --name holmesgpt-remote \
  --network host \
  -e HOLMESGPT_LLM_BASE_URL=http://192.168.1.169:8080 \
  -e HOLMESGPT_PORT=8091 \
  holmesgpt-api:localhost-8010
```

## 🔧 Helper Scripts

Use the provided helper script for common configurations:

```bash
# Show all options
./run-examples.sh menu

# Quick start with your current model
./run-examples.sh localhost-8010

# Switch to standard Ollama
./run-examples.sh ollama-default

# Use remote model
./run-examples.sh remote

# Clean up all containers
./run-examples.sh cleanup
```

## 🏥 Health Checks

Check if your configuration is working:

```bash
# Health check
curl http://localhost:8090/health

# Expected response:
# {"status":"degraded","services":{"holmesgpt_sdk":"healthy","context_api":"unhealthy"}}
```

The "degraded" status is normal - it indicates HolmesGPT SDK is healthy but Context API is unavailable (which is expected in most setups).

## 🔄 Switching Configurations

To change the model endpoint without rebuilding:

1. **Stop current container**: `podman stop container-name`
2. **Start with new config**: `podman run -e HOLMESGPT_LLM_BASE_URL=http://new-endpoint ...`

No rebuild required! 🎉
