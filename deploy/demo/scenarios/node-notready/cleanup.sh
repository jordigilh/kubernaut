#!/usr/bin/env bash
# Cleanup for Node NotReady Demo (#127)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up Node NotReady demo..."

# Restore the paused worker node
WORKER_NODE=$(kubectl get nodes -l kubernaut.ai/workload-node=true -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
if [ -n "$WORKER_NODE" ]; then
  echo "  Unpausing worker node: $WORKER_NODE"
  podman unpause "$WORKER_NODE" 2>/dev/null || true
  echo "  Uncordoning worker node: $WORKER_NODE"
  kubectl uncordon "$WORKER_NODE" 2>/dev/null || true
fi

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-node --ignore-not-found

echo "==> Cleanup complete."
