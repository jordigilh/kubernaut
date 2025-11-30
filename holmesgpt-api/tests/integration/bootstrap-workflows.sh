#!/bin/bash
# Bootstrap Workflow Data via REST API - DD-WORKFLOW-002 v3.0

set -e

DATA_STORAGE_URL="${DATA_STORAGE_URL:-http://localhost:18090}"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo "🔧 Bootstrapping workflows via REST API (DD-WORKFLOW-002 v3.0)..."
echo "   Data Storage URL: $DATA_STORAGE_URL"
echo ""

create_workflow() {
    local workflow_name="$1"
    local version="$2"
    local display_name="$3"
    local description="$4"
    local signal_type="$5"
    local severity="$6"
    local risk_tolerance="$7"
    local container_image="$8"

    local content="apiVersion: kubernaut.io/v1alpha1
kind: WorkflowSchema
metadata:
  workflow_id: ${workflow_name}
  version: \"${version}\"
  description: ${description}
labels:
  signal_type: ${signal_type}
  severity: ${severity}
  risk_tolerance: ${risk_tolerance}
parameters:
  - name: NAMESPACE
    type: string
    required: true
    description: Target namespace for the operation
  - name: TARGET_NAME
    type: string
    required: true
    description: Target resource name
execution:
  engine: tekton
  bundle: ${container_image}"

    local escaped_content=$(echo "$content" | jq -Rs .)

    local payload=$(cat <<EOF
{
    "workflow_name": "${workflow_name}",
    "version": "${version}",
    "name": "${display_name}",
    "description": "${description}",
    "content": ${escaped_content},
    "labels": {
        "signal-type": "${signal_type}",
        "severity": "${severity}",
        "risk-tolerance": "${risk_tolerance}"
    },
    "container_image": "${container_image}"
}
EOF
)

    local response=$(curl -s -w "\n%{http_code}" -X POST \
        "${DATA_STORAGE_URL}/api/v1/workflows" \
        -H "Content-Type: application/json" \
        -d "$payload")

    local http_code=$(echo "$response" | tail -1)
    local body=$(echo "$response" | sed '$d')

    if [ "$http_code" = "201" ]; then
        echo -e "  ${GREEN}✅${NC} Created: ${workflow_name} v${version}"
        return 0
    elif [ "$http_code" = "409" ]; then
        echo -e "  ${YELLOW}⚠️${NC}  Exists: ${workflow_name} v${version}"
        return 0
    else
        echo -e "  ${RED}❌${NC} Failed: ${workflow_name} v${version} (HTTP ${http_code})"
        echo "      Response: $body"
        return 1
    fi
}

echo "⏳ Waiting for Data Storage Service..."
for i in {1..30}; do
    if curl -sf "${DATA_STORAGE_URL}/health" > /dev/null 2>&1; then
        echo -e "${GREEN}✅ Data Storage Service ready${NC}"
        break
    fi
    [ $i -eq 30 ] && { echo -e "${RED}❌ Data Storage not available${NC}"; exit 1; }
    sleep 1
done
echo ""

echo "📦 Creating test workflows via API..."

create_workflow \
    "oomkill-increase-memory-limits" \
    "1.0.0" \
    "OOMKill Remediation - Increase Memory Limits" \
    "Increases memory limits for pods experiencing OOMKilled events" \
    "OOMKilled" \
    "critical" \
    "low" \
    "ghcr.io/kubernaut/workflows/oomkill-increase-memory:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000001"

create_workflow \
    "oomkill-scale-down-replicas" \
    "1.0.0" \
    "OOMKill Remediation - Scale Down Replicas" \
    "Reduces replica count for deployments experiencing OOMKilled" \
    "OOMKilled" \
    "high" \
    "medium" \
    "ghcr.io/kubernaut/workflows/oomkill-scale-down:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000002"

create_workflow \
    "crashloop-fix-configuration" \
    "1.0.0" \
    "CrashLoopBackOff - Fix Configuration" \
    "Identifies and fixes configuration issues causing CrashLoopBackOff" \
    "CrashLoopBackOff" \
    "high" \
    "low" \
    "ghcr.io/kubernaut/workflows/crashloop-fix-config:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000003"

create_workflow \
    "node-not-ready-drain-and-reboot" \
    "1.0.0" \
    "NodeNotReady - Drain and Reboot" \
    "Safely drains and reboots nodes in NotReady state" \
    "NodeNotReady" \
    "critical" \
    "low" \
    "ghcr.io/kubernaut/workflows/node-drain-reboot:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000004"

create_workflow \
    "image-pull-backoff-fix-credentials" \
    "1.0.0" \
    "ImagePullBackOff - Fix Registry Credentials" \
    "Fixes ImagePullBackOff errors by updating registry credentials" \
    "ImagePullBackOff" \
    "high" \
    "medium" \
    "ghcr.io/kubernaut/workflows/imagepull-fix-creds:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000005"

echo ""
echo "✅ Workflow bootstrap complete"
