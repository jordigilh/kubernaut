#!/bin/bash
# Generate Data Storage OpenAPI Python Client
#
# This script generates the Python client from the Data Storage OpenAPI v3 spec
# and applies necessary fixes for relative imports.
#
# Usage: ./generate-datastorage-client.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
CLIENT_DIR="$SCRIPT_DIR/datastorage"
SPEC_FILE="$PROJECT_ROOT/api/openapi/data-storage-v1.yaml"

echo "🔧 Generating Data Storage OpenAPI Python Client..."
echo "   Spec: $SPEC_FILE"
echo "   Output: $CLIENT_DIR"
echo ""

# Clean previous generation
if [ -d "$CLIENT_DIR" ]; then
    echo "🧹 Cleaning previous client..."
    rm -rf "$CLIENT_DIR"
fi

# Generate client using podman
echo "📦 Generating client with openapi-generator-cli (urllib3 1.26.x compatible)..."
podman run --rm \
    -v "${PROJECT_ROOT}:/local:z" \
    openapitools/openapi-generator-cli generate \
    -i /local/api/openapi/data-storage-v1.yaml \
    -g python \
    -o /local/holmesgpt-api/src/clients \
    --package-name datastorage \
    --additional-properties=packageVersion=1.0.0,library=urllib3 \
    > /dev/null 2>&1

echo "✅ Client generated"

# Fix import paths for relative imports
echo "🔧 Fixing import paths..."

cd "$CLIENT_DIR"

# Fix datastorage.* imports to relative imports
find . -name "*.py" -exec sed -i '' 's/from datastorage\./from ./g' {} \;
find . -name "*.py" -exec sed -i '' 's/import datastorage\./from . import /g' {} \;

# Fix api/__init__.py - remove duplicate .api prefix
sed -i '' 's/from \.api\./from ./g' api/__init__.py

# Fix api/*.py files - use parent directory imports
cd api
sed -i '' 's/from \.api_client/from ..api_client/g' *.py
sed -i '' 's/from \.configuration/from ..configuration/g' *.py
sed -i '' 's/from \.exceptions/from ..exceptions/g' *.py
sed -i '' 's/from \.models/from ..models/g' *.py
sed -i '' 's/from \.api_response/from ..api_response/g' *.py
sed -i '' 's/from \.rest/from ..rest/g' *.py
cd ..

# Fix models/__init__.py - remove duplicate .models prefix
sed -i '' 's/from \.models\./from ./g' models/__init__.py

# Fix models/*.py files - remove duplicate .models prefix
cd models
sed -i '' 's/from \.models\./from ./g' *.py
cd ..

# Fix api_client.py - use relative imports
sed -i '' 's/from datastorage import rest/from . import rest/g' api_client.py
sed -i '' 's/from datastorage import configuration/from . import configuration/g' api_client.py
sed -i '' 's/datastorage\.models/models/g' api_client.py

echo "✅ Import paths fixed"

# Verify imports work
echo "🧪 Verifying client imports..."
cd "$PROJECT_ROOT/holmesgpt-api"
python3 -c "
from src.clients.datastorage.api.workflow_catalog_api_api import WorkflowCatalogAPIApi
from src.clients.datastorage.models import WorkflowSearchRequest, WorkflowSearchFilters
from src.clients.datastorage.api_client import ApiClient
from src.clients.datastorage.configuration import Configuration
print('✅ All imports successful')
" || {
    echo "❌ Import verification failed"
    exit 1
}

echo ""
echo "✅ Data Storage OpenAPI client generation complete!"
echo ""
echo "Usage in tests:"
echo "  from src.clients.datastorage.api.workflow_catalog_api_api import WorkflowCatalogAPIApi"
echo "  from src.clients.datastorage.models import WorkflowSearchRequest, WorkflowSearchFilters"
echo ""
echo "Spec: api/openapi/data-storage-v1.yaml (authoritative)"
echo "Status: ✅ Spec validated successfully (no --skip-validate-spec needed)"

