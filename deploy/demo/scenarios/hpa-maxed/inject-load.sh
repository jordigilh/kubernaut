#!/usr/bin/env bash
# Generate sustained CPU load on api-frontend pods to push HPA to maxReplicas.
# Uses kubectl exec to run CPU-intensive processes inside the existing containers,
# keeping the cluster free of load-generator pods (avoids LLM detecting artificial
# load during AIAnalysis).
#
# The CPU burn runs in the background inside each pod and survives after this
# script exits. cleanup.sh kills the stress processes.
set -euo pipefail

NAMESPACE="demo-hpa"
LABEL_SELECTOR="app=api-frontend"

_stress_pods() {
    local pods
    pods=$(kubectl get pods -n "${NAMESPACE}" -l "${LABEL_SELECTOR}" \
        -o jsonpath='{.items[*].metadata.name}')

    if [ -z "$pods" ]; then
        echo "ERROR: No pods found with label ${LABEL_SELECTOR} in ${NAMESPACE}"
        return 1
    fi

    for pod in $pods; do
        if ! kubectl exec -n "${NAMESPACE}" "${pod}" -- pgrep yes </dev/null >/dev/null 2>&1; then
            echo "  Stressing pod ${pod}..."
            kubectl exec -n "${NAMESPACE}" "${pod}" -- sh -c \
                'for i in 1 2 3; do yes > /dev/null 2>&1 & done; exit 0' \
                </dev/null >/dev/null 2>&1
        fi
    done
}

echo "==> Starting CPU stress on api-frontend pods..."
_stress_pods

# When HPA scales up, new pods won't have stress. Wait and re-stress.
echo "==> Waiting 30s for HPA to scale, then stressing any new pods..."
sleep 30
_stress_pods

echo "==> All pods under CPU stress."
echo "    Watch HPA: kubectl get hpa -n ${NAMESPACE} -w"
echo "    Once currentReplicas == maxReplicas (3), the alert will fire after 2 min."
echo "    To stop manually: kubectl exec -n ${NAMESPACE} <pod> -- killall yes"
