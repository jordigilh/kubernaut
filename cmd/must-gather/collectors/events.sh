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

# Kubernaut Must-Gather - Kubernetes Events Collector
# BR-PLATFORM-001.4: Collect Kubernetes events for troubleshooting context

set -euo pipefail

COLLECTION_DIR="${1}"
EVENTS_DIR="${COLLECTION_DIR}/events"

echo "Collecting Kubernetes events..."

mkdir -p "${EVENTS_DIR}"

# Collect events from all Kubernaut namespaces
for namespace in "${KUBERNAUT_NAMESPACES[@]}"; do
    echo "  - Collecting events from namespace: ${namespace}"

    # Check if namespace exists
    if ! kubectl get namespace "${namespace}" > /dev/null 2>&1; then
        echo "    Warning: Namespace ${namespace} not found, skipping"
        continue
    fi

    # Collect events in YAML format
    kubectl get events -n "${namespace}" \
        --sort-by='.lastTimestamp' \
        -o yaml \
        > "${EVENTS_DIR}/events-${namespace}.yaml" 2>/dev/null || {
        echo "    Warning: Failed to collect events from ${namespace}"
        echo "---" > "${EVENTS_DIR}/events-${namespace}.yaml"
    }

    # Collect events in JSON format for automated analysis
    kubectl get events -n "${namespace}" \
        --sort-by='.lastTimestamp' \
        -o json \
        > "${EVENTS_DIR}/events-${namespace}.json" 2>/dev/null || {
        echo "    Warning: Failed to collect events JSON from ${namespace}"
        echo "{ \"items\": [] }" > "${EVENTS_DIR}/events-${namespace}.json"
    }

    # Count events
    EVENT_COUNT=$(jq -r '.items | length' "${EVENTS_DIR}/events-${namespace}.json" 2>/dev/null || echo "0")
    echo "    Collected ${EVENT_COUNT} events"
done

# Collect cluster-wide events (may include node events, etc.)
echo "  - Collecting cluster-wide events..."
kubectl get events --all-namespaces \
    --sort-by='.lastTimestamp' \
    -o yaml \
    > "${EVENTS_DIR}/events-cluster-wide.yaml" 2>/dev/null || {
    echo "    Warning: Failed to collect cluster-wide events"
}

# Filter for Kubernaut-related events
echo "  - Filtering Kubernaut-related events..."
kubectl get events --all-namespaces \
    --field-selector involvedObject.namespace=~kubernaut.* \
    -o json \
    > "${EVENTS_DIR}/events-kubernaut-filtered.json" 2>/dev/null || {
    echo "    Warning: Failed to filter Kubernaut events"
    echo "{ \"items\": [] }" > "${EVENTS_DIR}/events-kubernaut-filtered.json"
}

TOTAL_EVENTS=$(jq -r '.items | length' "${EVENTS_DIR}/events-kubernaut-filtered.json" 2>/dev/null || echo "0")
echo "Events collection complete (${TOTAL_EVENTS} Kubernaut-related events)"

