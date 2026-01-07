#!/bin/bash
# Copyright 2025 Jordi Gil
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Kubernaut Must-Gather - DataStorage API Collector
# BR-PLATFORM-001.6a: Collect workflow catalog and audit trail

set -euo pipefail

COLLECTION_DIR="${1:-}"
OUTPUT_DIR="${COLLECTION_DIR}/datastorage"

# DataStorage API configuration
DATASTORAGE_URL="${DATASTORAGE_URL:-http://datastorage.kubernaut-system.svc.cluster.local:8080}"
WORKFLOW_LIMIT="${WORKFLOW_LIMIT:-50}"
AUDIT_LIMIT="${AUDIT_LIMIT:-1000}"
AUDIT_TIMEFRAME="${AUDIT_TIMEFRAME:-24h}"
TIMEOUT="${DATASTORAGE_TIMEOUT:-30}"

if [[ -z "${COLLECTION_DIR}" ]]; then
    echo "Usage: $0 <collection-directory>"
    exit 1
fi

echo "Collecting DataStorage API data..."
mkdir -p "${OUTPUT_DIR}"

# Function to handle API errors
handle_api_error() {
    local endpoint="$1"
    local exit_code="$2"
    local output="$3"

    cat > "${OUTPUT_DIR}/error.json" <<EOF
{
  "error": "Failed to collect data from DataStorage API",
  "endpoint": "${endpoint}",
  "exit_code": ${exit_code},
  "message": "${output}",
  "timestamp": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
}
EOF

    echo "  ⚠️  DataStorage API unavailable: ${output}"
}

# Function to make API request with error handling
api_request() {
    local endpoint="$1"
    local output_file="$2"
    local description="$3"

    echo "  Collecting ${description}..."

    # Attempt API call with timeout
    if response=$(curl -s -f --max-time "${TIMEOUT}" "${DATASTORAGE_URL}${endpoint}" 2>&1); then
        # Success - save response
        echo "${response}" > "${output_file}"
        echo "    ✓ Collected ${description}"
        return 0
    else
        # Failure - capture error
        local exit_code=$?
        handle_api_error "${endpoint}" "${exit_code}" "${response}"
        return 1
    fi
}

# Collect workflows (limit 50, most recent)
if api_request "/api/v1/workflows?limit=${WORKFLOW_LIMIT}" \
               "${OUTPUT_DIR}/workflows.json" \
               "workflow catalog (limit ${WORKFLOW_LIMIT})"; then
    # Count workflows collected
    workflow_count=$(jq -r '.workflows | length' "${OUTPUT_DIR}/workflows.json" 2>/dev/null || echo "0")
    echo "    Workflows collected: ${workflow_count}"
fi

# Collect audit events (limit 1000, last 24h)
# Calculate timestamp for 24h ago (platform-independent)
if date --version &> /dev/null; then
    # GNU date (Linux)
    START_TIME=$(date -u --date="${AUDIT_TIMEFRAME} ago" +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u +"%Y-%m-%dT%H:%M:%SZ")
else
    # BSD date (macOS)
    START_TIME=$(date -u -v-24H +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u +"%Y-%m-%dT%H:%M:%SZ")
fi

if api_request "/api/v1/audit/events?limit=${AUDIT_LIMIT}&start_time=${START_TIME}" \
               "${OUTPUT_DIR}/audit-events.json" \
               "audit events (limit ${AUDIT_LIMIT}, last ${AUDIT_TIMEFRAME})"; then
    # Count audit events collected
    audit_count=$(jq -r '.data | length' "${OUTPUT_DIR}/audit-events.json" 2>/dev/null || echo "0")
    echo "    Audit events collected: ${audit_count}"
fi

echo "✓ DataStorage API collection complete"
