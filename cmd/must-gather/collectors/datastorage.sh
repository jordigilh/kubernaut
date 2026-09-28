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

# shellcheck source=../utils/namespace.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../utils/namespace.sh"

# DataStorage API configuration
# Issue #2037: build the URL from RELEASE_NAMESPACE instead of a hardcoded
# "kubernaut-system" literal, so collection still reaches DataStorage when the
# chart is installed into a non-default Helm release namespace. The Service
# name itself is "data-storage-service" (charts/kubernaut/templates/
# datastorage/datastorage.yaml), NOT "datastorage" -- confirmed against a
# real live-cluster install during this issue's own e2e-tests CI validation,
# where the old "datastorage" name silently resolved to nothing and every
# collection silently produced an empty error.json instead of real data.
if [[ -n "${DATASTORAGE_URL:-}" ]]; then
    DATASTORAGE_URL_EXPLICIT="true"
else
    DATASTORAGE_URL_EXPLICIT="false"
    DATASTORAGE_URL="http://data-storage-service.${RELEASE_NAMESPACE}.svc.cluster.local:8080"
fi
WORKFLOW_LIMIT="${WORKFLOW_LIMIT:-50}"
AUDIT_LIMIT="${AUDIT_LIMIT:-1000}"
AUDIT_TIMEFRAME="${AUDIT_TIMEFRAME:-24h}"
TIMEOUT="${DATASTORAGE_TIMEOUT:-30}"
KUBECTL_PROXY_PORT="${KUBECTL_PROXY_PORT:-8001}"
KUBECTL_PROXY_PID=""
KUBECTL_PROXY_URL=""

if [[ -z "${COLLECTION_DIR}" ]]; then
    echo "Usage: $0 <collection-directory>"
    exit 1
fi

echo "Collecting DataStorage API data..."
mkdir -p "${OUTPUT_DIR}"

# A local must-gather container can reach the Kubernetes API through its
# in-network kubeconfig, but it cannot resolve cluster-internal Service DNS
# names from the podman network (curl exit 6). Keep the direct URL as the fast
# path and lazily fall back to kubectl's authenticated Service proxy only for
# that DNS-specific failure. Explicit DATASTORAGE_URL overrides are never
# rewritten, so operators can still provide a routable endpoint directly.
cleanup_kubectl_proxy() {
    if [[ -n "${KUBECTL_PROXY_PID}" ]]; then
        kill "${KUBECTL_PROXY_PID}" 2>/dev/null || true
        wait "${KUBECTL_PROXY_PID}" 2>/dev/null || true
    fi
}
trap cleanup_kubectl_proxy EXIT

start_kubectl_proxy() {
    if [[ -n "${KUBECTL_PROXY_PID}" ]]; then
        return 0
    fi

    local proxy_log="${OUTPUT_DIR}/kubectl-proxy.log"
    echo "  DataStorage DNS is unavailable; starting Kubernetes API Service proxy..."
    kubectl proxy \
        --port="${KUBECTL_PROXY_PORT}" \
        --address=127.0.0.1 \
        --accept-hosts='^.*$' \
        >"${proxy_log}" 2>&1 &
    KUBECTL_PROXY_PID=$!

    for _ in $(seq 1 30); do
        if ! kill -0 "${KUBECTL_PROXY_PID}" 2>/dev/null; then
            echo "  ⚠️  Kubernetes API Service proxy exited; see ${proxy_log}"
            return 1
        fi
        if curl -sS --max-time 1 "http://127.0.0.1:${KUBECTL_PROXY_PORT}/version" >/dev/null 2>&1; then
            KUBECTL_PROXY_URL="http://127.0.0.1:${KUBECTL_PROXY_PORT}/api/v1/namespaces/${RELEASE_NAMESPACE}/services/http:data-storage-service:8080/proxy"
            DATASTORAGE_URL="${KUBECTL_PROXY_URL}"
            echo "  ✓ Kubernetes API Service proxy ready for DataStorage"
            return 0
        fi
        sleep 1
    done

    echo "  ⚠️  Kubernetes API Service proxy did not become ready; see ${proxy_log}"
    return 1
}

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

    # Attempt API call with timeout.
    if response=$(curl -s -f --max-time "${TIMEOUT}" "${DATASTORAGE_URL}${endpoint}" 2>&1); then
        # Success - save response
        echo "${response}" > "${output_file}"
        echo "    ✓ Collected ${description}"
        return 0
    else
        # Failure - capture error
        local exit_code=$?

        # curl exit 6 means the podman-side resolver cannot resolve the
        # cluster-internal Service name. Retry through the Kubernetes API
        # proxy, which uses the already-authenticated kubeconfig instead of
        # relying on cluster DNS from outside the cluster (BR-PLATFORM-001.6a).
        if [[ "${exit_code}" -eq 6 && "${DATASTORAGE_URL_EXPLICIT}" != "true" ]]; then
            if start_kubectl_proxy; then
                if response=$(curl -s -f --max-time "${TIMEOUT}" "${DATASTORAGE_URL}${endpoint}" 2>&1); then
                    echo "${response}" > "${output_file}"
                    echo "    ✓ Collected ${description} via Kubernetes API Service proxy"
                    return 0
                fi
                exit_code=$?
            fi
        fi
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
