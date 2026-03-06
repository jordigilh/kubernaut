#!/usr/bin/env bash
# Cleanup for CrashLoopBackOff Demo (#120)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up CrashLoopBackOff demo..."

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-crashloop --ignore-not-found --wait=true

PLATFORM_NS="${PLATFORM_NS:-kubernaut-system}"
echo "==> Cleaning up stale platform resources..."
kubectl delete remediationrequests --all -n "$PLATFORM_NS" --ignore-not-found 2>/dev/null || true
kubectl delete notificationrequests --all -n "$PLATFORM_NS" --ignore-not-found 2>/dev/null || true
kubectl delete aianalyses --all -n "$PLATFORM_NS" --ignore-not-found 2>/dev/null || true
kubectl delete remediationapprovalrequests --all -n "$PLATFORM_NS" --ignore-not-found 2>/dev/null || true
kubectl exec -n "$PLATFORM_NS" deploy/postgresql -- psql -U slm_user -d action_history \
  -c "DELETE FROM audit_events WHERE event_data->>'target_resource' = 'demo-crashloop/Deployment/worker';" 2>/dev/null || true

echo "==> Waiting for namespace deletion to complete..."
while kubectl get ns demo-crashloop &>/dev/null; do
  sleep 2
done

# Restart AlertManager so stale alert groups (repeat_interval=1h) don't
# suppress the fresh webhook notification for the new deployment.
echo "==> Restarting AlertManager to clear stale notification state..."
kubectl rollout restart statefulset/alertmanager-kube-prometheus-stack-alertmanager -n monitoring
kubectl rollout status statefulset/alertmanager-kube-prometheus-stack-alertmanager -n monitoring --timeout=60s

echo "==> Cleanup complete."
