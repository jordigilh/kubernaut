#!/bin/bash
# Generate Python OpenAPI client for HolmesGPT-API
# DD-HAPI-005: Auto-regenerate to prevent urllib3 version conflicts
#
# This is equivalent to Go's `go generate` pattern:
# - Go: go generate ./pkg/holmesgpt/client/
# - Python: ./tests/integration/generate-client.sh
#
# The client is generated from api/openapi.json and NOT committed to git.
# It is regenerated on-demand during test setup.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
OPENAPI_SPEC="${PROJECT_ROOT}/api/openapi.json"
CLIENT_OUTPUT="${PROJECT_ROOT}/tests/clients/holmesgpt_api_client"

echo "🔧 Generating Python OpenAPI client (DD-HAPI-005)..."
echo "   OpenAPI Spec: ${OPENAPI_SPEC}"
echo "   Output: ${CLIENT_OUTPUT}"

# Check if OpenAPI spec exists
if [ ! -f "${OPENAPI_SPEC}" ]; then
    echo "❌ Error: OpenAPI spec not found at ${OPENAPI_SPEC}"
    exit 1
fi

# Remove old client if it exists
if [ -d "${CLIENT_OUTPUT}" ]; then
    echo "   Removing old client..."
    rm -rf "${CLIENT_OUTPUT}"
fi

# Generate new client using OpenAPI Generator
# Using container image to avoid local installation requirements
# This matches the original generator used (see tests/clients/holmesgpt_api_client/__init__.py)
echo "   Generating client..."

# Detect container runtime (prefer podman, fallback to docker)
if command -v podman &> /dev/null; then
    CONTAINER_RUNTIME="podman"
elif command -v docker &> /dev/null; then
    CONTAINER_RUNTIME="docker"
else
    echo "❌ Error: Neither podman nor docker found. Please install one."
    exit 1
fi

echo "   Using container runtime: ${CONTAINER_RUNTIME}"

${CONTAINER_RUNTIME} run --rm \
    -v "${PROJECT_ROOT}:/local" \
    openapitools/openapi-generator-cli:latest generate \
    -i /local/api/openapi.json \
    -g python \
    -o /local/tests/clients/holmesgpt_api_client_tmp \
    --additional-properties=packageName=holmesgpt_api_client,projectName=holmesgpt-api-client

# Move the generated package to the correct location
# openapi-generator creates: output_dir/holmesgpt_api_client/
# We want: tests/clients/holmesgpt_api_client/
if [ -d "${PROJECT_ROOT}/tests/clients/holmesgpt_api_client_tmp/holmesgpt_api_client" ]; then
    mv "${PROJECT_ROOT}/tests/clients/holmesgpt_api_client_tmp/holmesgpt_api_client" "${CLIENT_OUTPUT}"
    rm -rf "${PROJECT_ROOT}/tests/clients/holmesgpt_api_client_tmp"
else
    echo "❌ Error: Generated package structure unexpected"
    ls -la "${PROJECT_ROOT}/tests/clients/holmesgpt_api_client_tmp/" || true
    exit 1
fi

# Fix permissions (docker may create files as root)
if [ -d "${CLIENT_OUTPUT}" ]; then
    chmod -R u+w "${CLIENT_OUTPUT}"
    echo "✅ Client generated successfully"
else
    echo "❌ Error: Client generation failed"
    exit 1
fi

echo ""
echo "📦 Generated client location: ${CLIENT_OUTPUT}"
echo "📋 Pattern: DD-HAPI-005 (Auto-regenerate, never commit)"
echo ""

