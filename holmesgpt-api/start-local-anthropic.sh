#!/bin/bash
# Local startup script for HolmesGPT API with Anthropic Direct API
#
# This script uses Anthropic's direct API (like Cursor does) instead of Vertex AI.
# Bypasses all GCP permission issues.
#
# Prerequisites:
# 1. Get Anthropic API key from https://console.anthropic.com/
# 2. Set ANTHROPIC_API_KEY environment variable or pass as argument
#
# Usage:
#   ./start-local-anthropic.sh
#   OR
#   ./start-local-anthropic.sh YOUR_API_KEY_HERE

set -e

echo "=== HolmesGPT API Local Startup (Anthropic Direct) ==="
echo ""

# Check for API key
if [ -n "$1" ]; then
    export ANTHROPIC_API_KEY="$1"
    echo "✅ Using API key from command line argument"
elif [ -n "$ANTHROPIC_API_KEY" ]; then
    echo "✅ Using API key from environment variable"
else
    echo "❌ ERROR: ANTHROPIC_API_KEY not set"
    echo ""
    echo "Usage:"
    echo "  1. Set environment variable: export ANTHROPIC_API_KEY='your-api-key-here'"
    echo "  2. Or pass as argument: ./start-local-anthropic.sh 'your-api-key-here'"
    echo ""
    echo "Get your API key from: https://console.anthropic.com/"
    exit 1
fi

# Anthropic Direct Configuration (matches cluster config)
# Note: litellm uses "anthropic/" prefix for direct Anthropic API
export LLM_MODEL="anthropic/claude-haiku-4-5-20251001"  # Matches cluster deployment

# HolmesGPT API Configuration
export CONFIG_FILE="config-local.yaml"

# Kubernetes Configuration (for local testing)
export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/config}"

# Clear any Vertex AI variables to avoid confusion
unset VERTEXAI_PROJECT
unset VERTEXAI_LOCATION
unset GOOGLE_APPLICATION_CREDENTIALS

echo ""
echo "Configuration:"
echo "  Provider: Anthropic Direct API"
echo "  Model: claude-haiku-4-5-20251001 (matches cluster)"
echo "  API Key: ${ANTHROPIC_API_KEY:0:15}...${ANTHROPIC_API_KEY: -4}"
echo "  Config File: $CONFIG_FILE"
echo "  Kubernetes: $KUBECONFIG"
echo ""
echo "✅ No GCP permissions needed!"
echo ""

# Ensure we're in the right directory
cd "$(dirname "$0")"

echo "Starting HolmesGPT API on http://localhost:8080"
echo "Health check: http://localhost:8080/health"
echo "API docs: http://localhost:8080/docs"
echo ""
echo "Press Ctrl+C to stop"
echo ""

# Start server with reload for development
exec python3 -m uvicorn src.main:app \
    --host 0.0.0.0 \
    --port 8080 \
    --reload \
    --log-level info

