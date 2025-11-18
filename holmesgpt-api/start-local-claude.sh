#!/bin/bash
# Local startup script for HolmesGPT API with Claude (Anthropic API)
#
# This script uses the same configuration as the cluster deployment
# - Uses Claude Haiku 4.5 (fast, good tool calling, cost-effective)
# - Requires ANTHROPIC_API_KEY environment variable or secret
#
# Usage: ./start-local-claude.sh

set -e

echo "=== HolmesGPT API - Claude (Anthropic) Local Startup ==="
echo ""

# Check if ANTHROPIC_API_KEY is set
if [ -z "$ANTHROPIC_API_KEY" ]; then
    echo "🔍 ANTHROPIC_API_KEY not in environment, checking cluster secret..."

    # Try to get from cluster secret
    if command -v kubectl &> /dev/null; then
        CLUSTER_KEY=$(kubectl get secret anthropic-api-key -n kubernaut-system -o jsonpath='{.data.api-key}' 2>/dev/null | base64 -d 2>/dev/null)
        if [ ! -z "$CLUSTER_KEY" ]; then
            export ANTHROPIC_API_KEY="$CLUSTER_KEY"
            echo "✅ Retrieved API key from cluster secret"
        else
            echo "❌ Could not retrieve API key from cluster"
            echo ""
            echo "Please set ANTHROPIC_API_KEY environment variable:"
            echo "  export ANTHROPIC_API_KEY='sk-ant-...'"
            echo ""
            echo "Or create the secret in the cluster:"
            echo "  kubectl create secret generic anthropic-api-key \\"
            echo "    --from-literal=api-key='sk-ant-...' \\"
            echo "    -n kubernaut-system"
            exit 1
        fi
    else
        echo "❌ kubectl not found and ANTHROPIC_API_KEY not set"
        echo ""
        echo "Please set ANTHROPIC_API_KEY environment variable:"
        echo "  export ANTHROPIC_API_KEY='sk-ant-...'"
        exit 1
    fi
else
    echo "✅ Using ANTHROPIC_API_KEY from environment"
fi

# Match cluster configuration
export CONFIG_FILE="config-local-claude.yaml"
export LLM_MODEL="claude-haiku-4-5-20251001"  # Match cluster model
export DEV_MODE="false"
export AUTH_ENABLED="false"

# Kubernetes Configuration (for local testing)
# Note: LiteLLM debug logging is now controlled by log_level in config-local-claude.yaml
export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/config}"

echo ""
echo "📋 Configuration:"
echo "  Provider: anthropic"
echo "  Model: $LLM_MODEL"
echo "  Config: $CONFIG_FILE"
echo "  API Key: ${ANTHROPIC_API_KEY:0:10}... (${#ANTHROPIC_API_KEY} chars)"
echo ""

# Ensure we're in the right directory
cd "$(dirname "$0")"

echo "🚀 Starting HolmesGPT API on http://localhost:8080"
echo "   Health check: http://localhost:8080/health"
echo "   API docs: http://localhost:8080/docs"
echo ""
echo "Press Ctrl+C to stop"
echo ""

# Start server with reload for development
exec python3 -m uvicorn src.main:app \
    --host 0.0.0.0 \
    --port 8080 \
    --reload \
    --log-level info

