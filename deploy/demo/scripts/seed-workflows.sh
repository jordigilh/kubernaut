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

while [[ $# -gt 0 ]]; do
    case "$1" in
        --scenario)
            SINGLE_SCENARIO="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [--scenario NAME]"
            echo ""
            echo "Options:"
            echo "  --scenario NAME    Seed only the workflow for this scenario"
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
VERSION="v1.0.0"

# scenario:image-name mappings (must match build-demo-workflows.sh)
# Schema images use the -schema suffix (two-image split: exec + schema)
WORKFLOWS=(
    "gitops-drift:git-revert-job"
    "autoscale:provision-node-job"
    "slo-burn:proactive-rollback-job"
    "memory-leak:graceful-restart-job"
    "crashloop:crashloop-rollback-job"
    "hpa-maxed:patch-hpa-job"
    "pdb-deadlock:relax-pdb-job"
    "pending-taint:remove-taint-job"
    "disk-pressure:cleanup-pvc-job"
    "node-notready:cordon-drain-job"
    "stuck-rollout:rollback-deployment-job"
    "cert-failure:fix-certificate-job"
    "cert-failure-gitops:fix-certificate-gitops-job"
    "crashloop-helm:helm-rollback-job"
    "mesh-routing-failure:fix-authz-policy-job"
    "statefulset-pvc-failure:fix-statefulset-pvc-job"
    "network-policy-block:fix-network-policy-job"
)

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

echo ""
echo "==> Workflow seeding complete (${seed_count} registered)"
if [ "$skip_count" -gt 0 ]; then
    echo "    Skipped: ${skip_count}"
fi
echo "==> Verify: curl -s ${DATASTORAGE_URL}/api/v1/workflows | jq '.'"
