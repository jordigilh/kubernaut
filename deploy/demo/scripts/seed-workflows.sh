#!/usr/bin/env bash
# Seed workflow catalog in DataStorage via REST API
# Reads workflow-schema.yaml from each scenario directory and registers
# via the CreateWorkflowInline endpoint (POST /api/v1/workflows).
#
# Usage:
#   ./deploy/demo/scripts/seed-workflows.sh                          # Seed all demo workflows
#   ./deploy/demo/scripts/seed-workflows.sh --scenario crashloop     # Seed a single scenario
#   ./deploy/demo/scripts/seed-workflows.sh --continue-on-error      # Skip failures, report at end
#   DATASTORAGE_URL=http://host:port ./seed-workflows.sh             # Custom DataStorage URL
#
# Default DATASTORAGE_URL: http://localhost:30081

set -euo pipefail

DATASTORAGE_URL="${DATASTORAGE_URL:-http://localhost:30081}"
SINGLE_SCENARIO=""
CONTINUE_ON_ERROR=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --scenario)
            SINGLE_SCENARIO="$2"
            shift 2
            ;;
        --continue-on-error)
            CONTINUE_ON_ERROR=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [--scenario NAME] [--continue-on-error]"
            echo ""
            echo "Options:"
            echo "  --scenario NAME      Seed only the workflow for this scenario"
            echo "  --continue-on-error  Skip failures and report summary at end"
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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIOS_DIR="${SCRIPT_DIR}/../scenarios"

echo "==> Seeding workflow catalog at ${DATASTORAGE_URL}"

SA_TOKEN=$(kubectl create token holmesgpt-api-sa -n kubernaut-system --duration=10m 2>/dev/null || echo "")
if [ -z "$SA_TOKEN" ]; then
    echo "WARNING: Could not create SA token, proceeding without auth"
fi

# POST a workflow to DataStorage using inline YAML content.
# Args: $1=scenario_dir $2=display_name
register_workflow() {
    local scenario_dir="$1"
    local name="$2"
    local schema_file="${SCENARIOS_DIR}/${scenario_dir}/workflow/workflow-schema.yaml"

    echo -n "  ${name}: "

    if [ ! -f "$schema_file" ]; then
        echo "SKIPPED (no workflow-schema.yaml)"
        return 2
    fi

    local yaml_content
    yaml_content=$(cat "$schema_file")

    local payload
    payload=$(jq -n \
        --arg content "$yaml_content" \
        --arg source "api" \
        --arg registeredBy "seed-workflows-script" \
        '{ content: $content, source: $source, registeredBy: $registeredBy }')

    local curl_args=(-s -w "\n%{http_code}" -X POST "${DATASTORAGE_URL}/api/v1/workflows"
        -H "Content-Type: application/json"
        -d "$payload")

    if [ -n "$SA_TOKEN" ]; then
        curl_args+=(-H "Authorization: Bearer ${SA_TOKEN}")
    fi

    local response http_code body
    response=$(curl "${curl_args[@]}" 2>&1) || true
    http_code=$(echo "$response" | tail -1)
    body=$(echo "$response" | sed '$d')

    case "$http_code" in
        2[0-9][0-9])
            echo "OK (HTTP ${http_code})"
            return 0
            ;;
        409)
            echo "ALREADY EXISTS (HTTP 409)"
            return 0
            ;;
        *)
            echo "FAILED (HTTP ${http_code})"
            if [ -n "$body" ]; then
                local detail
                detail=$(echo "$body" | jq -r '.detail // .message // empty' 2>/dev/null || echo "$body")
                echo "    Reason: ${detail}" | head -3
            fi
            return 1
            ;;
    esac
}

# shellcheck source=workflow-mappings.sh
source "${SCRIPT_DIR}/workflow-mappings.sh"

ok_count=0
fail_count=0
skip_count=0
already_count=0
failed_names=()

for entry in "${WORKFLOWS[@]}"; do
    SCENARIO="${entry%%:*}"
    IMAGE_NAME="${entry#*:}"

    if [ -n "$SINGLE_SCENARIO" ] && [ "$SCENARIO" != "$SINGLE_SCENARIO" ]; then
        skip_count=$((skip_count + 1))
        continue
    fi

    rc=0
    register_workflow "${SCENARIO}" "${IMAGE_NAME}" || rc=$?
    case $rc in
        0) ok_count=$((ok_count + 1)) ;;
        2) skip_count=$((skip_count + 1)) ;;
        *)
            fail_count=$((fail_count + 1))
            failed_names+=("${IMAGE_NAME}")
            if [ "$CONTINUE_ON_ERROR" = false ]; then
                echo ""
                echo "ERROR: Workflow registration failed. Use --continue-on-error to skip failures."
                exit 1
            fi
            ;;
    esac
done

if [ -n "$SINGLE_SCENARIO" ] && [ "$ok_count" -eq 0 ] && [ "$fail_count" -eq 0 ] && [ "$skip_count" -eq 0 ]; then
    echo "ERROR: Scenario '${SINGLE_SCENARIO}' not found in workflow mappings."
    echo "Available scenarios: $(printf '%s\n' "${WORKFLOWS[@]}" | cut -d: -f1 | tr '\n' ' ')"
    exit 1
fi

echo ""
echo "==> Workflow seeding complete: ${ok_count} registered, ${fail_count} failed, ${skip_count} skipped"
if [ "$fail_count" -gt 0 ]; then
    echo "    Failed workflows:"
    for name in "${failed_names[@]}"; do
        echo "      - ${name}"
    done
fi
echo "==> Verify: curl -s ${DATASTORAGE_URL}/api/v1/workflows | jq '.'"

[ "$fail_count" -eq 0 ]
