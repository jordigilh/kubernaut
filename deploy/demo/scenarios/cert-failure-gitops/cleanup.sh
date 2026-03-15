#!/usr/bin/env bash
# Cleanup for cert-manager GitOps Demo (#134)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Cleaning up cert-manager GitOps demo..."

kubectl delete -f "${SCRIPT_DIR}/manifests/argocd-application.yaml" --ignore-not-found
kubectl delete -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml" --ignore-not-found
kubectl delete namespace demo-cert-gitops --ignore-not-found
kubectl delete clusterissuer demo-selfsigned-ca-gitops --ignore-not-found
kubectl delete secret demo-ca-key-pair -n cert-manager --ignore-not-found

echo "==> Cleanup complete."
echo "    NOTE: Gitea, ArgoCD, and cert-manager are left running for reuse."
