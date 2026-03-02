#!/usr/bin/env bash
# External Actor Simulator -- Resource Contention Demo
#
# Watches for spec changes on the contention-app Deployment and reverts
# memory limits back to the original value (64Mi). Simulates a GitOps tool,
# another controller, or a human operator that conflicts with Kubernaut's
# remediation actions.
#
# The script polls every 10 seconds. When it detects that memory limits have
# changed from the original 64Mi, it patches them back, simulating contention.
set -euo pipefail

NAMESPACE="demo-resource-contention"
DEPLOYMENT="contention-app"
ORIGINAL_LIMIT="64Mi"
ORIGINAL_REQUEST="32Mi"

echo "[external-actor] Watching ${DEPLOYMENT} in ${NAMESPACE} for spec changes..."

while true; do
  CURRENT_LIMIT=$(kubectl -n "${NAMESPACE}" get deployment "${DEPLOYMENT}" \
    -o jsonpath='{.spec.template.spec.containers[0].resources.limits.memory}' 2>/dev/null || echo "")

  if [[ -n "${CURRENT_LIMIT}" && "${CURRENT_LIMIT}" != "${ORIGINAL_LIMIT}" ]]; then
    echo "[external-actor] Detected spec change: memory limit ${CURRENT_LIMIT} != ${ORIGINAL_LIMIT}"
    echo "[external-actor] Reverting to original value (simulating GitOps sync)..."
    kubectl -n "${NAMESPACE}" patch deployment "${DEPLOYMENT}" --type=json \
      -p="[{\"op\":\"replace\",\"path\":\"/spec/template/spec/containers/0/resources/limits/memory\",\"value\":\"${ORIGINAL_LIMIT}\"},{\"op\":\"replace\",\"path\":\"/spec/template/spec/containers/0/resources/requests/memory\",\"value\":\"${ORIGINAL_REQUEST}\"}]"
    echo "[external-actor] Reverted. Kubernaut's remediation is now ineffective."
  fi

  sleep 10
done
