#!/bin/bash
# Local startup script for HolmesGPT API with Vertex AI
#
# This script sets required environment variables that litellm expects
# for Vertex AI integration, then starts the HolmesGPT API server.
#
# Usage: ./start-local.sh

set -e

echo "=== HolmesGPT API Local Startup ==="
echo ""

# Vertex AI Configuration
export LLM_MODEL="vertex_ai/claude-3-5-sonnet@20240620"
export VERTEXAI_PROJECT="itpc-gcp-eco-eng-claude"
export VERTEXAI_LOCATION="us-central1"

# Google Cloud Credentials
# Auto-detect ADC location
ADC_PATH="$HOME/.config/gcloud/application_default_credentials.json"
if [ -f "$ADC_PATH" ]; then
    export GOOGLE_APPLICATION_CREDENTIALS="$ADC_PATH"
    echo "✅ Found ADC credentials: $ADC_PATH"
else
    echo "⚠️  ADC credentials not found at: $ADC_PATH"
    echo "   Run: gcloud auth application-default login"
    exit 1
fi

# HolmesGPT API Configuration
export CONFIG_FILE="config-local.yaml"

# Kubernetes Configuration (for local testing)
export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/config}"

echo ""
echo "Environment Variables:"
echo "  LLM_MODEL: $LLM_MODEL"
echo "  VERTEXAI_PROJECT: $VERTEXAI_PROJECT"
echo "  VERTEXAI_LOCATION: $VERTEXAI_LOCATION"
echo "  GOOGLE_APPLICATION_CREDENTIALS: $GOOGLE_APPLICATION_CREDENTIALS"
echo "  CONFIG_FILE: $CONFIG_FILE"
echo "  KUBECONFIG: $KUBECONFIG"
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

