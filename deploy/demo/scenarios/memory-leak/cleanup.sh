#!/usr/bin/env bash
# Cleanup for Proactive Memory Exhaustion Demo (#129)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up Memory Leak demo..."

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-memory-leak --ignore-not-found --wait=true

echo "==> Waiting for namespace deletion to complete..."
while kubectl get ns demo-memory-leak &>/dev/null; do
  sleep 2
done

PLATFORM_NS="${PLATFORM_NS:-kubernaut-system}"
echo "==> Cleaning up stale platform resources..."
kubectl delete remediationrequests --all -n "$PLATFORM_NS" --ignore-not-found 2>/dev/null || true
kubectl delete notificationrequests --all -n "$PLATFORM_NS" --ignore-not-found 2>/dev/null || true
kubectl delete aianalyses --all -n "$PLATFORM_NS" --ignore-not-found 2>/dev/null || true
kubectl delete remediationapprovalrequests --all -n "$PLATFORM_NS" --ignore-not-found 2>/dev/null || true
kubectl exec -n "$PLATFORM_NS" deploy/postgresql -- psql -U slm_user -d action_history \
  -c "DELETE FROM audit_events WHERE event_data->>'target_resource' LIKE 'demo-memory-leak%';" 2>/dev/null || true

echo "==> Cleanup complete."
