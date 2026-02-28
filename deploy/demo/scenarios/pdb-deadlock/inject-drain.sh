#!/usr/bin/env bash
# Drain the worker node targeting only the payment-service pods.
# The PDB (minAvailable=2 with 2 replicas) blocks all evictions, so the drain hangs
# until Kubernaut relaxes the PDB.
#
# Uses --pod-selector to avoid evicting platform and monitoring pods that also
# run on the worker node in a 2-node Kind cluster.
set -euo pipefail

NAMESPACE="demo-pdb"

WORKER_NODE=$(kubectl get nodes -l kubernaut.ai/managed=true -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

if [ -z "$WORKER_NODE" ]; then
  echo "ERROR: No worker node with label kubernaut.ai/managed=true found."
  echo "Ensure the Kind cluster was created with the multi-node config."
  exit 1
fi

echo "==> Draining worker node: ${WORKER_NODE} (payment-service pods only)..."
echo "    The PDB (minAvailable=2 with 2 replicas) will BLOCK all evictions."
echo "    The drain will hang until Kubernaut relaxes the PDB."
echo ""

# Run drain in background so the demo script can continue.
# --pod-selector targets only payment-service pods, leaving platform pods running.
# --disable-eviction=false ensures the eviction API is used (respects PDB).
kubectl drain "${WORKER_NODE}" \
  --pod-selector=app=payment-service \
  --delete-emptydir-data \
  --timeout=0s &
DRAIN_PID=$!

echo "    Drain running in background (PID ${DRAIN_PID})."
echo "    Watch: kubectl get nodes"
echo "    Watch PDB: kubectl get pdb -n ${NAMESPACE}"
echo ""
echo "    Expected state: node ${WORKER_NODE} shows SchedulingDisabled,"
echo "    but pods remain Running because PDB blocks eviction."
