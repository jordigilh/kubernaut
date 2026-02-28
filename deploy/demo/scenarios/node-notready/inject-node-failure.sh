#!/usr/bin/env bash
# Simulate node failure by pausing the Kind worker node container
# This makes kubelet stop reporting, causing the node to go NotReady
set -euo pipefail

WORKER_NODE=$(kubectl get nodes -l kubernaut.ai/managed=true -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

if [ -z "$WORKER_NODE" ]; then
  echo "ERROR: No worker node with label kubernaut.ai/managed=true found."
  echo "Ensure the Kind cluster was created with the multi-node config."
  exit 1
fi

echo "==> Pausing Kind node container: ${WORKER_NODE}..."
podman pause "${WORKER_NODE}"

echo "==> Node ${WORKER_NODE} paused. Kubelet will stop heartbeating."
echo "    The node will transition to NotReady within ~40 seconds."
echo "    Watch: kubectl get nodes -w"
echo ""
echo "    To restore (for cleanup): podman unpause ${WORKER_NODE}"
