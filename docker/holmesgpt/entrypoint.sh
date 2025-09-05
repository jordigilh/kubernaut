#!/bin/bash
set -e

echo "🤖 Starting HolmesGPT Container for Go Bindings"

# Display environment information
echo "📊 Environment Information:"
echo "  - Python version: $(python --version)"
echo "  - HolmesGPT version: $(holmes --version)"
echo "  - Working directory: ${HOLMES_WORKING_DIR:-/shared}"
echo "  - Session ID: ${HOLMES_SESSION_ID:-unknown}"
echo "  - Model: ${LLM_MODEL:-gpt-oss:20b}"
echo "  - Ollama URL: ${OPENAI_API_BASE:-http://host.docker.internal:11434/v1}"

# Check if kubectl is available and configured
if command -v kubectl &> /dev/null; then
    echo "  - kubectl version: $(kubectl version --client --short 2>/dev/null || echo 'not configured')"
    if [ -f "${KUBECONFIG:-/root/.kube/config}" ]; then
        echo "  - Kubernetes config: available"
    else
        echo "  - Kubernetes config: not available"
    fi
else
    echo "  - kubectl: not available"
fi

# Test Ollama connectivity
echo "🔗 Testing Ollama connectivity..."
if curl -s "${OPENAI_API_BASE%/v1}/api/version" > /dev/null 2>&1; then
    echo "  ✅ Ollama is accessible"

    # Test model availability
    if curl -s "${OPENAI_API_BASE%/v1}/api/tags" | grep -q "${LLM_MODEL}"; then
        echo "  ✅ Model ${LLM_MODEL} is available"
    else
        echo "  ⚠️  Model ${LLM_MODEL} may not be available"
    fi
else
    echo "  ❌ Ollama is not accessible at ${OPENAI_API_BASE%/v1}"
fi

# Create necessary directories
mkdir -p "${HOLMES_WORKING_DIR:-/shared}"
mkdir -p /root/.holmes

# Set up HolmesGPT configuration if not exists
if [ ! -f "/root/.holmes/config.yaml" ]; then
    echo "📝 Creating HolmesGPT configuration..."
    cat > /root/.holmes/config.yaml << EOF
# HolmesGPT Configuration for Container Bindings
llm:
  provider: "openai"
  model: "${LLM_MODEL:-gpt-oss:20b}"
  api_base: "${OPENAI_API_BASE:-http://host.docker.internal:11434/v1}"
  api_key: "${OPENAI_API_KEY:-ollama-local}"
  max_tokens: ${HOLMES_MAX_TOKENS:-4000}
  temperature: ${HOLMES_TEMPERATURE:-0.3}
  timeout: ${HOLMES_TIMEOUT:-300s}

# Session configuration
session:
  id: "${HOLMES_SESSION_ID:-container-session}"
  working_dir: "${HOLMES_WORKING_DIR:-/shared}"
  debug: ${HOLMES_DEBUG:-false}
  streaming: ${HOLMES_STREAMING:-true}

# Toolsets
toolsets:
  - kubernetes
  - prometheus
  - alertmanager
  - grafana

# Environment
environment:
  KUBECONFIG: "${KUBECONFIG:-/root/.kube/config}"
EOF
fi

# Test HolmesGPT CLI
echo "🧪 Testing HolmesGPT CLI..."
if holmes --version > /dev/null 2>&1; then
    echo "  ✅ HolmesGPT CLI is working"
else
    echo "  ❌ HolmesGPT CLI test failed"
    exit 1
fi

# Run a simple test query if requested
if [ "${HOLMES_RUN_TEST:-false}" = "true" ]; then
    echo "🔍 Running test query..."
    holmes ask "Hello, are you working?" --max-tokens 50 || echo "  ⚠️  Test query failed (this may be expected if Ollama is not ready)"
fi

echo "✅ HolmesGPT Container is ready!"
echo "📋 Container will remain running for Go bindings integration"

# Keep container running
exec tail -f /dev/null

