#!/usr/bin/env bash
# Trigger a rolling update that will be blocked by the overly restrictive PDB
# The update itself is harmless (env var change) but the PDB prevents pod replacement
set -euo pipefail

NAMESPACE="demo-pdb"

echo "==> Triggering rolling update on payment-service..."
echo "    Adding environment variable to force new pod template..."
kubectl set env deployment/payment-service -n "${NAMESPACE}" \
  RELEASE_VERSION="$(date +%s)"

echo "==> Rolling update initiated."
echo "    The PDB (minAvailable=2 with 2 replicas) will BLOCK the rollout."
echo "    Watch: kubectl rollout status deployment/payment-service -n ${NAMESPACE}"
echo "    Watch PDB: kubectl get pdb -n ${NAMESPACE}"
echo ""
echo "    Expected state: new ReplicaSet created but old pods cannot be evicted."
