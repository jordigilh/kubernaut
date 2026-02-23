#!/usr/bin/env bash
# Cleanup for Resource Quota Exhaustion Demo (#171)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up Resource Quota Exhaustion demo..."

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-quota --ignore-not-found --wait=true

echo "==> Waiting for namespace deletion to complete..."
while kubectl get ns demo-quota &>/dev/null; do
  sleep 2
done

# Restart AlertManager so stale alert groups (repeat_interval=1h) don't
# suppress the fresh webhook notification for the new deployment.
echo "==> Restarting AlertManager to clear stale notification state..."
kubectl rollout restart statefulset/alertmanager-kube-prometheus-stack-alertmanager -n monitoring
kubectl rollout status statefulset/alertmanager-kube-prometheus-stack-alertmanager -n monitoring --timeout=60s

echo "==> Cleanup complete."
