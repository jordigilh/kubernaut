#!/usr/bin/env bash
# Cleanup for Cluster Autoscaling Demo (#126)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up Cluster Autoscaling demo..."

# Kill any running provisioner agent
if pgrep -f "provisioner.sh" >/dev/null 2>&1; then
  echo "  Stopping provisioner agent..."
  pkill -f "provisioner.sh" || true
fi

# Delete the scale-request ConfigMap
kubectl delete cm scale-request -n kubernaut-system --ignore-not-found

# Check if a dynamically provisioned node exists and remove it
EXTRA_NODES=$(kubectl get nodes -o name 2>/dev/null | grep "worker-[0-9]" || true)
for NODE in $EXTRA_NODES; do
  NODE_NAME="${NODE#node/}"
  echo "  Removing dynamically provisioned node: $NODE_NAME"
  kubectl drain "$NODE_NAME" --ignore-daemonsets --delete-emptydir-data --force 2>/dev/null || true
  kubectl delete node "$NODE_NAME" --ignore-not-found
  podman rm -f "$NODE_NAME" 2>/dev/null || true
done

# Delete namespace and Prometheus rules
kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-autoscale --ignore-not-found

echo "==> Cleanup complete."
