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

echo "Collecting service logs..."

# V1.0 Services (8 total)
# Stateless HTTP Services (3): gateway, datastorage, holmesgpt-api
# CRD Controllers (5): notification-controller, signalprocessing-controller,
#                      aianalysis-controller, workflowexecution-controller,
#                      remediationorchestrator-controller

SERVICE_PATTERNS=(
    "gateway-"
    "datastorage-"
    "holmesgpt-api-"
    "notification-controller-"
    "signalprocessing-controller-"
    "aianalysis-controller-"
    "workflowexecution-controller-"
    "remediationorchestrator-controller-"
)

# Default namespaces if not set
if [ -z "${KUBERNAUT_NAMESPACES+x}" ]; then
    KUBERNAUT_NAMESPACES=("kubernaut-system" "kubernaut-notifications" "kubernaut-workflows")
fi

# Iterate through Kubernaut namespaces
for namespace in "${KUBERNAUT_NAMESPACES[@]}"; do
    echo "  - Namespace: ${namespace}"

    # Check if namespace exists
    if ! kubectl get namespace "${namespace}" > /dev/null 2>&1; then
        echo "    Warning: Namespace ${namespace} not found, skipping"
        continue
    fi

    # Get all pods in namespace
    PODS=$(kubectl get pods -n "${namespace}" --no-headers 2>/dev/null | awk '{print $1}' || echo "")

    if [ -z "${PODS}" ]; then
        echo "    No pods found in namespace ${namespace}"
        continue
    fi

    # Collect logs for each pod matching service patterns
    while IFS= read -r pod; do
        # Check if pod matches any service pattern
        MATCHED=false
        for pattern in "${SERVICE_PATTERNS[@]}"; do
            if [[ "${pod}" == ${pattern}* ]]; then
                MATCHED=true
                break
            fi
        done

        if [ "${MATCHED}" = false ]; then
            continue  # Skip non-Kubernaut pods
        fi

        echo "    Collecting logs from pod: ${pod}"

        POD_DIR="${LOGS_DIR}/${namespace}/${pod}"
        mkdir -p "${POD_DIR}"

        # Collect current logs
        kubectl logs "${pod}" -n "${namespace}" \
            --since="${SINCE_DURATION}" \
            --tail=10000 \
            --timestamps \
            --all-containers \
            > "${POD_DIR}/current.log" 2>&1 || {
            echo "      Warning: Failed to collect current logs from ${pod}"
        }

        # Collect previous logs (if pod has restarted)
        kubectl logs "${pod}" -n "${namespace}" \
            --previous \
            --tail=10000 \
            --timestamps \
            --all-containers \
            > "${POD_DIR}/previous.log" 2>/dev/null || {
            # No previous logs (pod hasn't restarted) - this is normal
            rm -f "${POD_DIR}/previous.log"
        }

        # Collect pod description
        kubectl describe pod "${pod}" -n "${namespace}" > "${POD_DIR}/describe.txt" 2>&1 || true

    done <<< "${PODS}"
done

# Count total logs collected
TOTAL_LOGS=$(find "${LOGS_DIR}" -name "*.log" 2>/dev/null | wc -l || echo "0")
echo "Service logs collection complete (${TOTAL_LOGS} log files)"

