#!/usr/bin/env bash
# Cleanup for HPA Maxed Out Demo (#123)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up HPA Maxed Out demo..."

# Kill any CPU stress processes running inside pods
for pod in $(kubectl get pods -n demo-hpa -l app=api-frontend -o name 2>/dev/null); do
    kubectl exec -n demo-hpa "$pod" -- killall yes 2>/dev/null || true
done

# Delete pipeline CRDs targeting this namespace
for rr in $(kubectl get remediationrequests -n kubernaut-system -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.signalLabels.namespace}{"\n"}{end}' 2>/dev/null | grep demo-hpa | cut -f1); do
    kubectl delete remediationrequest "$rr" -n kubernaut-system --ignore-not-found 2>/dev/null || true
done

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-hpa --ignore-not-found --wait=true

echo "==> Waiting for namespace deletion to complete..."
while kubectl get ns demo-hpa &>/dev/null; do
    sleep 2
done

# Restart AlertManager to clear stale notification state
echo "==> Restarting AlertManager..."
kubectl rollout restart statefulset/alertmanager-kube-prometheus-stack-alertmanager -n monitoring
kubectl rollout status statefulset/alertmanager-kube-prometheus-stack-alertmanager -n monitoring --timeout=60s

echo "==> Cleanup complete."
