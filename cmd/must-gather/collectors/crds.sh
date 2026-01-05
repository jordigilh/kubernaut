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

# Kubernaut Must-Gather - CRD Collector
# BR-PLATFORM-001.2: Collect all Kubernaut CRD instances

set -euo pipefail

COLLECTION_DIR="${1}"
CRD_DIR="${COLLECTION_DIR}/crds"

echo "Collecting Kubernaut CRDs..."

# Kubernaut CRD types (V1.0 - 6 types)
CRD_TYPES=(
    "remediationrequests.kubernaut.ai"
    "remediationapprovalrequests.kubernaut.ai"
    "signalprocessings.kubernaut.ai"
    "aianalyses.kubernaut.ai"
    "workflowexecutions.kubernaut.ai"
    "notificationrequests.kubernaut.ai"
)

for crd_type in "${CRD_TYPES[@]}"; do
    crd_name="${crd_type%%.*}"  # Extract name before first dot

    echo "  - Collecting ${crd_type}..."

    # Create CRD-specific directory
    mkdir -p "${CRD_DIR}/${crd_name}"

    # Collect CRD definition
    kubectl get crd "${crd_type}" -o yaml > "${CRD_DIR}/${crd_name}/crd-definition.yaml" 2>/dev/null || {
        echo "    Warning: CRD ${crd_type} not found (may not be installed)"
        continue
    }

    # Collect all instances across all namespaces
    kubectl get "${crd_type}" --all-namespaces -o yaml > "${CRD_DIR}/${crd_name}/all-instances.yaml" 2>/dev/null || {
        echo "    Warning: No instances of ${crd_type} found"
        echo "---" > "${CRD_DIR}/${crd_name}/all-instances.yaml"
    }

    # Count instances
    INSTANCE_COUNT=$(kubectl get "${crd_type}" --all-namespaces --no-headers 2>/dev/null | wc -l || echo "0")
    echo "    Collected ${INSTANCE_COUNT} instances"
done

echo "CRD collection complete (${#CRD_TYPES[@]} types)"

