#!/usr/bin/env bash
# Cleanup for Pending Pods Taint Removal Demo (#122)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up Pending Taint demo..."

# Remove the injected taint from the worker node
WORKER_NODE=$(kubectl get nodes -l kubernaut.ai/managed=true -o name 2>/dev/null | head -1 || echo "")
if [ -n "$WORKER_NODE" ]; then
  echo "  Removing maintenance taint from ${WORKER_NODE}..."
  kubectl taint nodes "${WORKER_NODE}" maintenance- 2>/dev/null || true
fi

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-taint --ignore-not-found

echo "==> Cleanup complete."
