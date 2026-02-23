#!/usr/bin/env bash
# Seed workflow catalog in DataStorage via REST API
# Registers OCI-based remediation workflows that the LLM can discover
#
# Usage:
#   ./deploy/demo/scripts/seed-workflows.sh                          # Seed all 17 demo workflows
#   ./deploy/demo/scripts/seed-workflows.sh --scenario crashloop     # Seed a single scenario
#   DATASTORAGE_URL=http://host:port ./seed-workflows.sh             # Custom DataStorage URL
#
# Default DATASTORAGE_URL: http://localhost:30081

set -euo pipefail

DATASTORAGE_URL="${DATASTORAGE_URL:-http://localhost:30081}"
SINGLE_SCENARIO=""
SEED_VERSION=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --scenario)
            SINGLE_SCENARIO="$2"
            shift 2
            ;;
        --version)
            SEED_VERSION="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [--scenario NAME] [--version TAG]"
            echo ""
            echo "Options:"
            echo "  --scenario NAME    Seed only the workflow for this scenario"
            echo "  --version TAG      Workflow image version tag (default: v1.0.0)"
            echo ""
            echo "Environment:"
            echo "  DATASTORAGE_URL    DataStorage API URL (default: http://localhost:30081)"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

echo "==> Seeding workflow catalog at ${DATASTORAGE_URL}"

# Get a ServiceAccount token for authentication (DD-AUTH-014)
SA_TOKEN=$(kubectl create token holmesgpt-api-sa -n kubernaut-system --duration=10m 2>/dev/null || echo "")
if [ -z "$SA_TOKEN" ]; then
    echo "WARNING: Could not create SA token, proceeding without auth"
fi

# POST a workflow to DataStorage
# Args: $1=schema_image $2=display_name
register_workflow() {
    local schema_image="$1"
    local name="$2"

    echo -n "  ${name}: "

    local curl_args=(-s -o /dev/null -w "%{http_code}" -X POST "${DATASTORAGE_URL}/api/v1/workflows"
        -H "Content-Type: application/json"
        -d "{\"schemaImage\":\"${schema_image}\"}")

    if [ -n "$SA_TOKEN" ]; then
        curl_args+=(-H "Authorization: Bearer ${SA_TOKEN}")
    fi

    local http_code
    http_code=$(curl "${curl_args[@]}")
    echo "HTTP ${http_code}"
}

REGISTRY="quay.io/kubernaut-cicd/test-workflows"
VERSION="${SEED_VERSION:-v1.0.0}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# scenario:image-name mappings (shared across build + seed scripts)
# shellcheck source=workflow-mappings.sh
source "${SCRIPT_DIR}/workflow-mappings.sh"

seed_count=0
skip_count=0

for entry in "${WORKFLOWS[@]}"; do
    SCENARIO="${entry%%:*}"
    IMAGE_NAME="${entry#*:}"
    SCHEMA_REF="${REGISTRY}/${IMAGE_NAME}-schema:${VERSION}"

    if [ -n "$SINGLE_SCENARIO" ] && [ "$SCENARIO" != "$SINGLE_SCENARIO" ]; then
        skip_count=$((skip_count + 1))
        continue
    fi

    register_workflow "${SCHEMA_REF}" "${IMAGE_NAME}"
    seed_count=$((seed_count + 1))
done

if [ -n "$SINGLE_SCENARIO" ] && [ "$seed_count" -eq 0 ]; then
    echo "ERROR: Scenario '${SINGLE_SCENARIO}' not found in workflow mappings."
    echo "Available scenarios: $(printf '%s\n' "${WORKFLOWS[@]}" | cut -d: -f1 | tr '\n' ' ')"
    exit 1
fi

echo ""
echo "==> Workflow seeding complete (${seed_count} registered)"
if [ "$skip_count" -gt 0 ]; then
    echo "    Skipped: ${skip_count}"
fi
echo "==> Verify: curl -s ${DATASTORAGE_URL}/api/v1/workflows | jq '.'"
