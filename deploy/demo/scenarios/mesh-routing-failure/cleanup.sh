#!/usr/bin/env bash
# Cleanup for Linkerd Mesh Routing Failure Demo (#136)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up Linkerd Mesh Routing Failure demo..."

kubectl delete -f "${SCRIPT_DIR}/manifests/deny-policy.yaml" --ignore-not-found
kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete -f "${SCRIPT_DIR}/manifests/deployment.yaml" --ignore-not-found
kubectl delete namespace demo-mesh-failure --ignore-not-found

echo "==> Cleanup complete."
echo "    NOTE: Linkerd itself is NOT removed. To remove:"
echo "    linkerd install | kubectl delete -f -"
echo "    linkerd install --crds | kubectl delete -f -"
