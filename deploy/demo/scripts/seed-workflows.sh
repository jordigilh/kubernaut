#!/usr/bin/env bash
# Seed workflow catalog in DataStorage via REST API
# Registers OCI-based remediation workflows that the LLM can discover
#
# Usage:
#   ./deploy/demo/scripts/seed-workflows.sh [DATASTORAGE_URL]
#
# Default DATASTORAGE_URL: http://localhost:30081

set -euo pipefail

DATASTORAGE_URL="${1:-http://localhost:30081}"
SA_TOKEN=""

echo "==> Seeding workflow catalog at ${DATASTORAGE_URL}"

# Get a ServiceAccount token for authentication (DD-AUTH-014)
get_sa_token() {
    SA_TOKEN=$(kubectl create token holmesgpt-api-sa -n kubernaut-system --duration=10m 2>/dev/null || echo "")
    if [ -z "$SA_TOKEN" ]; then
        echo "WARNING: Could not create SA token, proceeding without auth"
    fi
}

# POST a workflow to DataStorage
register_workflow() {
    local payload="$1"
    local name="$2"

    local auth_header=""
    if [ -n "$SA_TOKEN" ]; then
        auth_header="-H \"Authorization: Bearer ${SA_TOKEN}\""
    fi

    echo "  Registering workflow: ${name}"
    eval curl -s -X POST "${DATASTORAGE_URL}/api/v1/workflows" \
        -H "Content-Type: application/json" \
        ${auth_header} \
        -d "'${payload}'" \
        -o /dev/null -w "    HTTP %{http_code}\n"
}

get_sa_token

# Workflow 1: OOMKill Memory Increase (remediation for OOMKilled containers)
# DD-WORKFLOW-017: OCI-based registration -- DataStorage pulls the image,
# extracts /workflow-schema.yaml, and populates all catalog fields from it.
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/oomkill-increase-memory-job:latest"
}' "oomkill-increase-memory-job"

# Workflow 2: CrashLoop Config Fix (remediation for CrashLoopBackOff)
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/crashloop-config-fix-job:latest"
}' "crashloop-config-fix-job"

# ============================================
# Demo Scenario Workflows (#114, #119-#130)
# Built by: deploy/demo/scripts/build-demo-workflows.sh
# ============================================

# Workflow 3: GitOps Revert (#125 -- GitOps drift remediation)
# actionType: GitRevertCommit | detectedLabels: gitOpsTool: "*"
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/git-revert-job:v1.0.0"
}' "git-revert-job"

# Workflow 4: Node Provisioning (#126 -- Cluster autoscaling)
# actionType: ProvisionNode
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/provision-node-job:v1.0.0"
}' "provision-node-job"

# Workflow 5: Proactive Rollback (#128 -- SLO error budget burn)
# actionType: ProactiveRollback
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/proactive-rollback-job:v1.0.0"
}' "proactive-rollback-job"

echo ""
echo "==> Workflow seeding complete (5 workflows)"
echo "==> Verify: curl -s ${DATASTORAGE_URL}/api/v1/workflows | jq '.'"
