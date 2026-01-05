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

# Kubernaut Must-Gather - Tekton Resources Collector
# BR-PLATFORM-001.6f: Collect Tekton Pipelines resources (CRITICAL P1 for V1.0)

set -euo pipefail

COLLECTION_DIR="${1}"
TEKTON_DIR="${COLLECTION_DIR}/tekton"

echo "Collecting Tekton resources..."

mkdir -p "${TEKTON_DIR}"/{pipelineruns,taskruns,pipelines,tasks,operator}

# Check if Tekton is installed
if ! kubectl get crd pipelineruns.tekton.dev > /dev/null 2>&1; then
    echo "  Warning: Tekton CRDs not found - Tekton may not be installed"
    echo "{ \"error\": \"Tekton not installed\" }" > "${TEKTON_DIR}/error.json"
    exit 0
fi

# Collect PipelineRuns from kubernaut-workflows namespace
echo "  - Collecting PipelineRuns from kubernaut-workflows..."
kubectl get pipelineruns -n kubernaut-workflows -o yaml \
    > "${TEKTON_DIR}/pipelineruns/all-pipelineruns.yaml" 2>/dev/null || {
    echo "    Warning: Failed to collect PipelineRuns"
    echo "---" > "${TEKTON_DIR}/pipelineruns/all-pipelineruns.yaml"
}

# Count PipelineRuns
PIPELINERUN_COUNT=$(kubectl get pipelineruns -n kubernaut-workflows --no-headers 2>/dev/null | wc -l || echo "0")
echo "    Collected ${PIPELINERUN_COUNT} PipelineRuns"

# Collect PipelineRun logs (last 24h)
echo "  - Collecting PipelineRun logs..."
PIPELINERUNS=$(kubectl get pipelineruns -n kubernaut-workflows --no-headers 2>/dev/null | awk '{print $1}' || echo "")

if [ -n "${PIPELINERUNS}" ]; then
    while IFS= read -r pr; do
        echo "    Collecting logs from PipelineRun: ${pr}"

        PR_DIR="${TEKTON_DIR}/pipelineruns/${pr}"
        mkdir -p "${PR_DIR}"

        # Get PipelineRun details
        kubectl get pipelinerun "${pr}" -n kubernaut-workflows -o yaml \
            > "${PR_DIR}/spec.yaml" 2>/dev/null || true

        # Get PipelineRun logs (via tkn or kubectl)
        kubectl logs -n kubernaut-workflows -l tekton.dev/pipelineRun="${pr}" \
            --since="${SINCE_DURATION}" \
            --tail=5000 \
            --timestamps \
            --prefix \
            > "${PR_DIR}/logs.txt" 2>/dev/null || {
            echo "      Warning: No logs found for PipelineRun ${pr}"
        }
    done <<< "${PIPELINERUNS}"
fi

# Collect TaskRuns from kubernaut-workflows namespace
echo "  - Collecting TaskRuns from kubernaut-workflows..."
kubectl get taskruns -n kubernaut-workflows -o yaml \
    > "${TEKTON_DIR}/taskruns/all-taskruns.yaml" 2>/dev/null || {
    echo "    Warning: Failed to collect TaskRuns"
    echo "---" > "${TEKTON_DIR}/taskruns/all-taskruns.yaml"
}

# Count TaskRuns
TASKRUN_COUNT=$(kubectl get taskruns -n kubernaut-workflows --no-headers 2>/dev/null | wc -l || echo "0")
echo "    Collected ${TASKRUN_COUNT} TaskRuns"

# Collect Pipeline definitions referenced by WorkflowExecution CRDs
echo "  - Collecting Pipeline definitions..."
kubectl get pipelines -n kubernaut-workflows -o yaml \
    > "${TEKTON_DIR}/pipelines/all-pipelines.yaml" 2>/dev/null || {
    echo "    Warning: No Pipelines found"
    echo "---" > "${TEKTON_DIR}/pipelines/all-pipelines.yaml"
}

# Collect Task definitions (kubernaut-action generic meta-task)
echo "  - Collecting Task definitions..."
kubectl get tasks -n kubernaut-workflows -o yaml \
    > "${TEKTON_DIR}/tasks/all-tasks.yaml" 2>/dev/null || {
    echo "    Warning: No Tasks found"
    echo "---" > "${TEKTON_DIR}/tasks/all-tasks.yaml"
}

# Collect Tekton operator infrastructure
echo "  - Collecting Tekton operator infrastructure..."

# Tekton operator pods (usually in tekton-pipelines namespace)
TEKTON_NS="tekton-pipelines"
if kubectl get namespace "${TEKTON_NS}" > /dev/null 2>&1; then
    echo "    Collecting Tekton operator pods from ${TEKTON_NS}..."

    kubectl get pods -n "${TEKTON_NS}" -o yaml \
        > "${TEKTON_DIR}/operator/operator-pods.yaml" 2>/dev/null || true

    # Collect operator logs
    OPERATOR_PODS=$(kubectl get pods -n "${TEKTON_NS}" -l app.kubernetes.io/part-of=tekton-pipelines --no-headers 2>/dev/null | awk '{print $1}' || echo "")

    if [ -n "${OPERATOR_PODS}" ]; then
        while IFS= read -r pod; do
            echo "      Collecting logs from Tekton pod: ${pod}"

            kubectl logs "${pod}" -n "${TEKTON_NS}" \
                --since="${SINCE_DURATION}" \
                --tail=5000 \
                --timestamps \
                --all-containers \
                > "${TEKTON_DIR}/operator/${pod}.log" 2>/dev/null || true
        done <<< "${OPERATOR_PODS}"
    fi

    # Collect Tekton ConfigMaps
    kubectl get configmaps -n "${TEKTON_NS}" -o yaml \
        > "${TEKTON_DIR}/operator/configmaps.yaml" 2>/dev/null || true

    # Collect Tekton webhook configuration
    kubectl get validatingwebhookconfigurations -l app.kubernetes.io/part-of=tekton-pipelines -o yaml \
        > "${TEKTON_DIR}/operator/webhooks.yaml" 2>/dev/null || true
else
    echo "    Warning: Tekton operator namespace ${TEKTON_NS} not found"
fi

echo "Tekton collection complete (${PIPELINERUN_COUNT} PipelineRuns, ${TASKRUN_COUNT} TaskRuns)"

