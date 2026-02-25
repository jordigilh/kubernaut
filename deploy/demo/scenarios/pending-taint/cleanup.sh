#!/usr/bin/env bash
# Cleanup for Pending Pods Taint Removal Demo (#122)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up Pending Taint demo..."

# Remove the injected taint from all managed worker nodes
WORKER_NODES=$(kubectl get nodes -l kubernaut.ai/managed=true -o name 2>/dev/null || echo "")
for node in $WORKER_NODES; do
  echo "  Removing maintenance taint from ${node}..."
  kubectl taint nodes "${node}" maintenance- 2>/dev/null || true
done

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-taint --ignore-not-found

echo "==> Cleanup complete."
