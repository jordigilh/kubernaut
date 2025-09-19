#!/bin/sh
set -e

# Environment validation
if [ -z "$HOLMESGPT_LLM_BASE_URL" ]; then
    echo "ERROR: HOLMESGPT_LLM_BASE_URL environment variable is required"
    exit 1
fi

if [ -z "$HOLMESGPT_LLM_MODEL" ]; then
    echo "ERROR: HOLMESGPT_LLM_MODEL environment variable is required"
    exit 1
fi

if [ -z "$HOLMESGPT_LLM_PROVIDER" ]; then
    echo "ERROR: HOLMESGPT_LLM_PROVIDER environment variable is required"
    exit 1
fi

# Log configuration
echo "🚀 Starting HolmesGPT API Server"
echo "   LLM Base URL: $HOLMESGPT_LLM_BASE_URL"
echo "   LLM Model: $HOLMESGPT_LLM_MODEL"
echo "   LLM Provider: $HOLMESGPT_LLM_PROVIDER"
echo "   Port: ${HOLMESGPT_PORT:-8090}"

# Start the application
exec python src/main.py