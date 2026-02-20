#!/usr/bin/env bash
# Add a NoSchedule taint to the worker node to block pod scheduling
set -euo pipefail

WORKER_NODE=$(kubectl get nodes -l kubernaut.ai/workload-node=true -o name | head -1)

if [ -z "$WORKER_NODE" ]; then
  echo "ERROR: No worker node with label kubernaut.ai/workload-node=true found."
  echo "Ensure the Kind cluster was created with the multi-node config."
  exit 1
fi

echo "==> Adding NoSchedule taint to ${WORKER_NODE}..."
kubectl taint nodes "${WORKER_NODE}" maintenance=scheduled:NoSchedule --overwrite

echo "==> Taint applied. Pods with nodeSelector for this node will remain Pending."
echo "    Watch: kubectl get pods -n demo-taint -w"
