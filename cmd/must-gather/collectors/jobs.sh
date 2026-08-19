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

# Kubernaut Must-Gather - Kubernetes Jobs Collector
# BR-PLATFORM-001.6g: Collect native batchv1.Job resources created by the
# WorkflowExecution Job executor (Issue #2036)

set -euo pipefail

COLLECTION_DIR="${1}"
JOBS_DIR="${COLLECTION_DIR}/jobs"

echo "Collecting Kubernetes Jobs..."

mkdir -p "${JOBS_DIR}"

# BR-WE-018: WorkflowExecution's Job executor (pkg/workflowexecution/executor/
# job.go) creates native batchv1.Job resources in WORKFLOW_NAMESPACE
# (kubernaut-workflows) -- the same namespace Tekton PipelineRuns run in, but
# a DIFFERENT resource kind that tekton.sh's Tekton-CRD-scoped collection
# never sees, and that logs.sh never scans (it's scoped to
# RELEASE_NAMESPACE/OPERATOR_NAMESPACE only). Without this collector,
# Job-executor workflow failures had zero diagnostic coverage in must-gather.
kubectl get jobs -n "${WORKFLOW_NAMESPACE}" -o yaml \
    > "${JOBS_DIR}/all-jobs.yaml" 2>/dev/null || {
    echo "  Warning: Failed to collect Jobs"
    echo "---" > "${JOBS_DIR}/all-jobs.yaml"
}

JOBS=$(kubectl get jobs -n "${WORKFLOW_NAMESPACE}" --no-headers 2>/dev/null | awk '{print $1}' || echo "")

JOB_COUNT=0
if [ -n "${JOBS}" ]; then
    while IFS= read -r job; do
        [ -z "${job}" ] && continue
        JOB_COUNT=$((JOB_COUNT + 1))

        echo "  - Collecting Job: ${job}"
        JOB_DIR="${JOBS_DIR}/${job}"
        mkdir -p "${JOB_DIR}"

        kubectl get job "${job}" -n "${WORKFLOW_NAMESPACE}" -o yaml \
            > "${JOB_DIR}/spec.yaml" 2>/dev/null || true

        # A Job may own multiple Pods across retries (BackoffLimit); the
        # job-name label (set by the Job controller on every Pod it creates)
        # captures all of them, unlike a single `kubectl logs job/<name>`
        # call, which only follows the current/latest Pod.
        kubectl logs -n "${WORKFLOW_NAMESPACE}" -l job-name="${job}" \
            --since="${SINCE_DURATION}" \
            --tail=5000 \
            --timestamps \
            --prefix \
            --all-containers \
            > "${JOB_DIR}/logs.txt" 2>/dev/null || {
            echo "    Warning: No logs found for Job ${job}"
        }
    done <<< "${JOBS}"
fi

echo "Jobs collection complete (${JOB_COUNT} Jobs)"
