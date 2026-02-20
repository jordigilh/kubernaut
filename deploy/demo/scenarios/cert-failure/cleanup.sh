#!/usr/bin/env bash
# Cleanup for cert-manager Certificate Failure Demo (#133)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up cert-manager Certificate Failure demo..."

kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete -f "${SCRIPT_DIR}/manifests/certificate.yaml" --ignore-not-found
kubectl delete -f "${SCRIPT_DIR}/manifests/deployment.yaml" --ignore-not-found
kubectl delete -f "${SCRIPT_DIR}/manifests/clusterissuer.yaml" --ignore-not-found
kubectl delete secret demo-ca-key-pair -n cert-manager --ignore-not-found
kubectl delete namespace demo-cert-failure --ignore-not-found

echo "==> Cleanup complete."
echo "    NOTE: cert-manager itself is NOT removed. To remove:"
echo "    kubectl delete -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.1/cert-manager.yaml"
