#!/usr/bin/env bash
# Cleanup for Concurrent Cross-Namespace Demo (#172)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up Concurrent Cross-Namespace demo..."

for NS in demo-team-alpha demo-team-beta; do
  kubectl delete -f "${SCRIPT_DIR}/manifests/${NS#demo-}/prometheus-rule.yaml" --ignore-not-found 2>/dev/null || true
  kubectl delete namespace "${NS}" --ignore-not-found --wait=false
done

echo "==> Waiting for namespace deletion to complete..."
for NS in demo-team-alpha demo-team-beta; do
  while kubectl get ns "${NS}" &>/dev/null; do
    sleep 2
  done
done

echo "==> Restarting AlertManager to clear stale notification state..."
kubectl rollout restart statefulset/alertmanager-kube-prometheus-stack-alertmanager -n monitoring
kubectl rollout status statefulset/alertmanager-kube-prometheus-stack-alertmanager -n monitoring --timeout=60s

echo "==> Cleanup complete."
