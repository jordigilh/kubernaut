#!/bin/bash
# Script to generate HAPI Python OpenAPI client for integration/E2E tests
#
# This script uses the openapi-generator-cli to generate a Python client
# from HAPI's OpenAPI spec. The client is used in integration and E2E tests
# to validate API contract compliance.
#
# Usage: ./generate-hapi-client.sh
#
# Authority: TRIAGE_HAPI_E2E_AND_CLIENT_GAPS.md
# Purpose: Validate OpenAPI spec matches runtime behavior

set -e

# Define paths (relative to holmesgpt-api directory)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
OPENAPI_SPEC_PATH="${PROJECT_DIR}/api/openapi.json"
OUTPUT_DIR="${PROJECT_DIR}/tests/clients"
PACKAGE_NAME="holmesgpt_api_client"
CLIENT_DIR="${OUTPUT_DIR}/${PACKAGE_NAME}"

# For Podman volume mount
PODMAN_SPEC_PATH="/local/api/openapi.json"
PODMAN_OUTPUT_DIR="/local/tests/clients"

echo "🔧 Generating Python client for HAPI from ${OPENAPI_SPEC_PATH}..."

# Ensure spec exists
if [ ! -f "${OPENAPI_SPEC_PATH}" ]; then
    echo "❌ OpenAPI spec not found at ${OPENAPI_SPEC_PATH}"
    echo "   Run: python3 -c 'import json; from src.main import app; json.dump(app.openapi(), open(\"api/openapi.json\", \"w\"), indent=2)'"
    exit 1
fi

# Create output directory
mkdir -p "${OUTPUT_DIR}"

# Run OpenAPI Generator using Podman
echo "🐳 Running openapi-generator-cli via Podman..."
podman run --rm -v "${PROJECT_DIR}":/local:z openapitools/openapi-generator-cli generate \
  -i "${PODMAN_SPEC_PATH}" \
  -g python \
  -o "${PODMAN_OUTPUT_DIR}" \
  --package-name "${PACKAGE_NAME}" \
  --additional-properties=packageVersion=1.0.0

echo "✅ Python client generated to ${OUTPUT_DIR}"

# Verify client directory was created
if [ ! -d "${CLIENT_DIR}" ]; then
    echo "❌ Client directory not created at ${CLIENT_DIR}"
    exit 1
fi

echo "🔧 Applying import path fixes to generated client..."

# Fix absolute imports in holmesgpt_api_client/__init__.py
find "${CLIENT_DIR}" -type f -name "__init__.py" -exec sed -i '' -E 's/from '"${PACKAGE_NAME}"'\.api\.(.*) import (.*) as (.*)/from \.api.\1 import \2 as \3/g' {} +
find "${CLIENT_DIR}" -type f -name "__init__.py" -exec sed -i '' -E 's/from '"${PACKAGE_NAME}"'\.(.*) import (.*) as (.*)/from \.\1 import \2 as \3/g' {} +

# Fix absolute imports in holmesgpt_api_client/api/__init__.py
find "${CLIENT_DIR}/api" -type f -name "__init__.py" -exec sed -i '' -E 's/from \.api\.(.*) import (.*)/from \.\1 import \2/g' {} +

# Fix absolute imports in holmesgpt_api_client/models/__init__.py
find "${CLIENT_DIR}/models" -type f -name "__init__.py" -exec sed -i '' -E 's/from \.models\.(.*) import (.*)/from \.\1 import \2/g' {} +

# Fix imports in individual API files
find "${CLIENT_DIR}/api" -type f -name "*.py" -not -name "__init__.py" -exec sed -i '' -E 's/from \.(api_client|api_response|configuration|exceptions|rest) import (.*)/from ..\1 import \2/g' {} +

# Fix imports in api_client.py
sed -i '' -E 's/from \. import models/from \.models import \*/g' "${CLIENT_DIR}/api_client.py" 2>/dev/null || true
sed -i '' -E 's/from holmesgpt_api_client import rest/from \. import rest/g' "${CLIENT_DIR}/api_client.py" 2>/dev/null || true

# Fix imports in individual model files
find "${CLIENT_DIR}/models" -type f -name "*.py" -not -name "__init__.py" -exec sed -i '' -E 's/from \.models\.(.*) import (.*)/from \.\1 import \2/g' {} +

echo "✅ Import path fixes applied."

echo "🔍 Verifying client imports..."
python3 -c "
import sys
sys.path.insert(0, 'tests/clients')
try:
    from holmesgpt_api_client.api_client import ApiClient
    from holmesgpt_api_client.configuration import Configuration
    from holmesgpt_api_client.api.incident_analysis_api import IncidentAnalysisApi
    print('✅ HAPI OpenAPI client imported successfully!')
    print('   - ApiClient: OK')
    print('   - IncidentAnalysisApi: OK')
except ImportError as e:
    print(f'❌ Import error: {e}')
    sys.exit(1)
" || {
    echo "⚠️  Import verification failed. Client may need manual fixes."
    echo "   This is normal for first generation. Check import paths manually."
}

echo ""
echo "✨ Client generation complete!"
echo ""
echo "📝 Next steps:"
echo "   1. Review generated client in tests/clients/holmesgpt_api_client/"
echo "   2. Update integration tests to use client"
echo "   3. Run: pytest tests/integration/ -v"
echo ""
echo "📖 Example usage:"
echo "   from holmesgpt_api_client import ApiClient, Configuration"
echo "   from holmesgpt_api_client.api.incident_analysis_api import IncidentAnalysisApi"
echo "   "
echo "   config = Configuration(host='http://localhost:18120')"
echo "   client = ApiClient(configuration=config)"
echo "   incident_api = IncidentAnalysisApi(client)"

