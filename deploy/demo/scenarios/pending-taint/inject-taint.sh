#!/usr/bin/env bash
# Add a NoSchedule taint to ALL managed worker nodes to block pod scheduling.
# Multi-node support: taints every node with kubernaut.ai/managed=true so that
# pods using that nodeSelector have nowhere to schedule.
set -euo pipefail

WORKER_NODES=$(kubectl get nodes -l kubernaut.ai/managed=true -o name)

if [ -z "$WORKER_NODES" ]; then
  echo "ERROR: No worker nodes with label kubernaut.ai/managed=true found."
  echo "Ensure the Kind cluster was created with the multi-node config."
  exit 1
fi

for node in $WORKER_NODES; do
  echo "==> Adding NoSchedule taint to ${node}..."
  kubectl taint nodes "${node}" maintenance=scheduled:NoSchedule --overwrite
done

echo "==> Taint applied to all managed workers. Pods with nodeSelector will remain Pending."
echo "    Watch: kubectl get pods -n demo-taint -w"
