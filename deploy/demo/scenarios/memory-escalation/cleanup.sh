#!/usr/bin/env bash
# Cleanup for Memory Escalation Demo (#168)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up Memory Escalation demo..."

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-memory-escalation --ignore-not-found --wait=true

echo "==> Waiting for namespace deletion to complete..."
while kubectl get ns demo-memory-escalation &>/dev/null; do
  sleep 2
done

echo "==> Restarting AlertManager to clear stale notification state..."
kubectl rollout restart statefulset/alertmanager-kube-prometheus-stack-alertmanager -n monitoring
kubectl rollout status statefulset/alertmanager-kube-prometheus-stack-alertmanager -n monitoring --timeout=60s

echo "==> Cleanup complete."
