#!/usr/bin/env bash
# Deploy ArgoCD (minimal install) for GitOps demo scenarios
# Uses the core-install manifest (~800MB-1.2GB RAM)
#
# Usage: ./deploy/demo/scenarios/gitops/scripts/setup-argocd.sh
set -euo pipefail

ARGOCD_NAMESPACE="argocd"

echo "==> Installing ArgoCD in namespace ${ARGOCD_NAMESPACE}..."

kubectl create namespace "${ARGOCD_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

kubectl apply -n "${ARGOCD_NAMESPACE}" \
  -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/core-install.yaml

echo "==> Waiting for ArgoCD pods to be ready..."
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/part-of=argocd \
  -n "${ARGOCD_NAMESPACE}" --timeout=300s

echo "==> Configuring ArgoCD to trust Gitea repository..."
GITEA_REPO_URL="http://gitea-http.gitea:3000/kubernaut/demo-gitops-repo.git"

kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: gitea-repo-creds
  namespace: ${ARGOCD_NAMESPACE}
  labels:
    argocd.argoproj.io/secret-type: repo-creds
stringData:
  type: git
  url: http://gitea-http.gitea:3000
  username: kubernaut
  password: kubernaut123
EOF

echo "==> ArgoCD setup complete"
echo "    Namespace: ${ARGOCD_NAMESPACE}"
echo "    Gitea repo registered: ${GITEA_REPO_URL}"
