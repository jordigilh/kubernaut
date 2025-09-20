# Environment Variables Reference

This document describes the environment variables used by the prometheus-alerts-slm system for LLM provider configuration.

## LLM Provider Configuration

### Required Variables

Use these standardized environment variables for all deployments:

```bash
# LLM Server Configuration
export LLM_ENDPOINT="http://192.168.1.169:8080" # LLM server endpoint
export LLM_MODEL="gpt-oss:20b"                  # Model name to use
export LLM_PROVIDER="ramalama"                   # Provider type: ollama, ramalama, localai

# Optional Test Configuration
export TEST_TIMEOUT="120s"                       # Test timeout duration
export MAX_RETRIES="3"                          # Maximum retry attempts
export LOG_LEVEL="debug"                        # Logging level
export SKIP_SLOW_TESTS="false"                  # Skip performance tests
export SKIP_INTEGRATION="false"                 # Skip integration tests
```

## Provider-Specific Configuration

### Ramalama (Default - OpenAI-compatible)
```bash
export LLM_PROVIDER="ramalama"
export LLM_ENDPOINT="http://192.168.1.169:8080"
export LLM_MODEL="gpt-oss:20b"
```

### Ollama (Alternative)
```bash
export LLM_PROVIDER="ollama"
export LLM_ENDPOINT="http://localhost:11434"
export LLM_MODEL="granite3.1-dense:8b"
```

### LocalAI (OpenAI-compatible)
```bash
export LLM_PROVIDER="localai"
export LLM_ENDPOINT="http://localai-server:8080"
export LLM_MODEL="ggml-gpt4all-j"
```

## Testing Configuration

### Integration Tests
```bash
export LLM_ENDPOINT="http://192.168.1.169:8080"
export LLM_MODEL="gpt-oss:20b"
export LLM_PROVIDER="ramalama"
go test -tags=integration ./test/integration/...
```

### Multi-Provider Testing
```bash
# Test with Ramalama (Default)
export LLM_PROVIDER="ramalama" LLM_ENDPOINT="http://192.168.1.169:8080"
export LLM_MODEL="gpt-oss:20b"
go test -tags=integration ./test/integration/...

# Test with Ollama (Alternative)
export LLM_PROVIDER="ollama" LLM_ENDPOINT="http://localhost:11434"
export LLM_MODEL="granite3.1-dense:8b"
go test -tags=integration ./test/integration/...
```

## Makefile Usage

All Makefile targets use the standardized LLM_* variables:

```bash
# Run integration tests with Ramalama (Default)
make test-integration

# Run integration tests with Ollama (Alternative)
make test-integration-ollama

# Quick integration tests
make test-integration-quick
```

## Troubleshooting

### Configuration Issues
```bash
# Verify configuration loading
go test -v ./test/integration/shared -run TestProviderDetection

# Check provider detection
export LLM_ENDPOINT="your-endpoint"
# Check logs for detected provider type
```

### Common Problems

1. **Provider mismatch**: Ensure `LLM_PROVIDER` matches your endpoint type
2. **Connection issues**: Verify LLM server is running and accessible
3. **Model availability**: Ensure the specified model is available on the server
4. **Case sensitivity**: All variable names are case-sensitive

### Debug Configuration Loading
```bash
# Enable debug logging
export LOG_LEVEL="debug"

# Run a simple test to see configuration loading
go test -v ./test/integration/shared
```

## Default Values

If no environment variables are set, the system uses these defaults:

- **LLM_ENDPOINT**: `http://192.168.1.169:8080`
- **LLM_MODEL**: `gpt-oss:20b`
- **LLM_PROVIDER**: Auto-detected based on endpoint (defaults to ramalama for port 8080)
- **TEST_TIMEOUT**: `120s`
- **MAX_RETRIES**: `3`
- **LOG_LEVEL**: `debug`
