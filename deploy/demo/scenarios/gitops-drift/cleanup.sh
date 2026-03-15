#!/usr/bin/env bash
# Cleanup for GitOps Drift Demo (#125)
set -euo pipefail

echo "==> Cleaning up GitOps Drift demo..."

kubectl delete -f "$(dirname "$0")/manifests/argocd-application.yaml" --ignore-not-found
kubectl delete namespace demo-gitops --ignore-not-found
kubectl delete -f "$(dirname "$0")/manifests/prometheus-rule.yaml" --ignore-not-found

echo "==> Cleanup complete."
echo "    Note: Gitea and ArgoCD are left running for reuse by other scenarios."
echo "    To remove them: kubectl delete namespace gitea argocd"
