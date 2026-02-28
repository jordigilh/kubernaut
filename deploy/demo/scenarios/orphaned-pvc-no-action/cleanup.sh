#!/usr/bin/env bash
# Cleanup for Orphaned PVC Demo (#122)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up Orphaned PVC demo..."

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-orphaned-pvc --ignore-not-found --wait=true

echo "==> Waiting for namespace deletion to complete..."
while kubectl get ns demo-orphaned-pvc &>/dev/null; do
  sleep 2
done

# Restart AlertManager so stale alert groups (repeat_interval=1h) don't
# suppress the fresh webhook notification for the new deployment.
echo "==> Restarting AlertManager to clear stale notification state..."
kubectl rollout restart statefulset/alertmanager-kube-prometheus-stack-alertmanager -n monitoring
kubectl rollout status statefulset/alertmanager-kube-prometheus-stack-alertmanager -n monitoring --timeout=60s

echo "==> Cleanup complete."
