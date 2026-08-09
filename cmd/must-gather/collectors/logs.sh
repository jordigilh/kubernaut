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

# Kubernaut Must-Gather - Service Logs Collector
# BR-PLATFORM-001.3: Collect logs from all Kubernaut service pods

set -euo pipefail

COLLECTION_DIR="${1}"
LOGS_DIR="${COLLECTION_DIR}/logs"

# shellcheck source=../utils/namespace.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../utils/namespace.sh"

echo "Collecting service logs..."

# Issue #2037: collect logs from EVERY pod in the release namespace, instead
# of matching pod names against a maintained per-service prefix allowlist.
# That allowlist silently rotted as services were added (12 chart-managed
# services today vs. 8 when the list was last updated) -- support engineers
# received incomplete log collections with no signal collection was partial.
# This mirrors the drift-proof precedent already established by
# test/infrastructure/datastorage.go's MustGatherPodLogs.
#
# Scoped to RELEASE_NAMESPACE only (not WORKFLOW_NAMESPACE): Tekton job-pod
# logs in the workflow namespace are already collected more precisely by
# tekton.sh's PipelineRun-label-selector-based `kubectl logs`.

echo "  - Namespace: ${RELEASE_NAMESPACE}"

if ! kubectl get namespace "${RELEASE_NAMESPACE}" > /dev/null 2>&1; then
    echo "    Warning: Namespace ${RELEASE_NAMESPACE} not found, skipping"
    exit 0
fi

# Get all pods in the release namespace -- no per-service allowlist
PODS=$(kubectl get pods -n "${RELEASE_NAMESPACE}" --no-headers 2>/dev/null | awk '{print $1}' || echo "")

if [ -z "${PODS}" ]; then
    echo "    No pods found in namespace ${RELEASE_NAMESPACE}"
else
    while IFS= read -r pod; do
        [ -z "${pod}" ] && continue

        echo "    Collecting logs from pod: ${pod}"

        POD_DIR="${LOGS_DIR}/${RELEASE_NAMESPACE}/${pod}"
        mkdir -p "${POD_DIR}"

        # Collect current logs
        kubectl logs "${pod}" -n "${RELEASE_NAMESPACE}" \
            --since="${SINCE_DURATION}" \
            --tail=10000 \
            --timestamps \
            --all-containers \
            > "${POD_DIR}/current.log" 2>&1 || {
            echo "      Warning: Failed to collect current logs from ${pod}"
        }

        # Collect previous logs (if pod has restarted)
        kubectl logs "${pod}" -n "${RELEASE_NAMESPACE}" \
            --previous \
            --tail=10000 \
            --timestamps \
            --all-containers \
            > "${POD_DIR}/previous.log" 2>/dev/null || {
            # No previous logs (pod hasn't restarted) - this is normal
            rm -f "${POD_DIR}/previous.log"
        }

        # Collect pod description
        kubectl describe pod "${pod}" -n "${RELEASE_NAMESPACE}" > "${POD_DIR}/describe.txt" 2>&1 || true

    done <<< "${PODS}"
fi

# Count total logs collected
TOTAL_LOGS=$(find "${LOGS_DIR}" -name "*.log" 2>/dev/null | wc -l || echo "0")
echo "Service logs collection complete (${TOTAL_LOGS} log files)"

