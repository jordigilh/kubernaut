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
# RELEASE_NAMESPACE is mandatory (this Helm chart always deploys into it).
# OPERATOR_NAMESPACE is OPTIONAL: the separate kubernaut-operator component
# (quay.io/kubernaut-ai/kubernaut-operator) is not deployed by this Helm
# chart at all -- it's an independent install that most clusters won't have.
# When present, its controller-manager pod's logs matter just as much for
# operator-path deployments, so it's collected the same way, but its absence
# is the expected common case and must never produce a warning.
#
# Neither includes WORKFLOW_NAMESPACE: Tekton job-pod logs there are already
# collected more precisely by tekton.sh's PipelineRun-label-selector-based
# `kubectl logs`.

collect_namespace_pod_logs() {
    local namespace="$1"
    local optional="$2" # "true" -> silent skip if namespace absent

    if ! kubectl get namespace "${namespace}" > /dev/null 2>&1; then
        if [ "${optional}" = "true" ]; then
            return 0
        fi
        echo "    Warning: Namespace ${namespace} not found, skipping"
        return 0
    fi

    echo "  - Namespace: ${namespace}"

    # Get all pods in the namespace -- no per-service allowlist
    local pods
    pods=$(kubectl get pods -n "${namespace}" --no-headers 2>/dev/null | awk '{print $1}' || echo "")

    if [ -z "${pods}" ]; then
        echo "    No pods found in namespace ${namespace}"
        return 0
    fi

    while IFS= read -r pod; do
        [ -z "${pod}" ] && continue

        echo "    Collecting logs from pod: ${pod}"

        local pod_dir="${LOGS_DIR}/${namespace}/${pod}"
        mkdir -p "${pod_dir}"

        # Collect current logs
        kubectl logs "${pod}" -n "${namespace}" \
            --since="${SINCE_DURATION}" \
            --tail=10000 \
            --timestamps \
            --all-containers \
            > "${pod_dir}/current.log" 2>&1 || {
            echo "      Warning: Failed to collect current logs from ${pod}"
        }

        # Collect previous logs (if pod has restarted)
        kubectl logs "${pod}" -n "${namespace}" \
            --previous \
            --tail=10000 \
            --timestamps \
            --all-containers \
            > "${pod_dir}/previous.log" 2>/dev/null || {
            # No previous logs (pod hasn't restarted) - this is normal
            rm -f "${pod_dir}/previous.log"
        }

        # Collect pod description
        kubectl describe pod "${pod}" -n "${namespace}" > "${pod_dir}/describe.txt" 2>&1 || true

    done <<< "${pods}"
}

collect_namespace_pod_logs "${RELEASE_NAMESPACE}" "false"
collect_namespace_pod_logs "${OPERATOR_NAMESPACE}" "true"

# Issue #2036: mesh/gateway infra namespaces (Kuadrant's mcp-system/
# gateway-system/istio-system, Envoy AI Gateway's envoy-gateway-system/
# envoy-ai-gateway-system) that DeployFleetCoreInfra creates outside
# RELEASE_NAMESPACE/WORKFLOW_NAMESPACE/OPERATOR_NAMESPACE. EXTRA_NAMESPACES_CSV
# is a scalar (comma-joined) env var, not a bash array: gather.sh runs this
# collector as a separate `bash logs.sh` subprocess, and bash arrays cannot
# cross that boundary via export -- only scalar strings can. All treated as
# optional ("true"): which mesh (if any) is deployed varies per E2E lane.
if [ -n "${EXTRA_NAMESPACES_CSV:-}" ]; then
    IFS=',' read -ra EXTRA_NAMESPACES <<< "${EXTRA_NAMESPACES_CSV}"
    for extra_ns in "${EXTRA_NAMESPACES[@]}"; do
        [ -z "${extra_ns}" ] && continue
        collect_namespace_pod_logs "${extra_ns}" "true"
    done
fi

# Count total logs collected
TOTAL_LOGS=$(find "${LOGS_DIR}" -name "*.log" 2>/dev/null | wc -l || echo "0")
echo "Service logs collection complete (${TOTAL_LOGS} log files)"

