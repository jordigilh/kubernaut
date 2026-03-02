#!/usr/bin/env bash
# Deploy ArgoCD (minimal install) for GitOps demo scenarios
# Uses the core-install manifest (~800MB-1.2GB RAM)
#
# Usage: ./deploy/demo/scenarios/gitops/scripts/setup-argocd.sh
set -euo pipefail

ARGOCD_NAMESPACE="argocd"

echo "==> Installing ArgoCD in namespace ${ARGOCD_NAMESPACE}..."

kubectl create namespace "${ARGOCD_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

kubectl apply -n "${ARGOCD_NAMESPACE}" --server-side --force-conflicts \
  -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/core-install.yaml

echo "==> Ensuring server.secretkey exists (core-install does not set it)..."
if ! kubectl get secret argocd-secret -n "${ARGOCD_NAMESPACE}" -o jsonpath='{.data.server\.secretkey}' 2>/dev/null | grep -q .; then
  kubectl patch secret argocd-secret -n "${ARGOCD_NAMESPACE}" --type merge \
    -p "{\"stringData\":{\"server.secretkey\":\"$(openssl rand -hex 32)\"}}"
fi

echo "==> Waiting for ArgoCD pods to be ready..."
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=argocd-application-controller \
  -n "${ARGOCD_NAMESPACE}" --timeout=300s
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=argocd-repo-server \
  -n "${ARGOCD_NAMESPACE}" --timeout=300s

echo "==> Ensuring default AppProject exists (core-install omits it)..."
kubectl apply -f - <<APPPROJ
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: default
  namespace: ${ARGOCD_NAMESPACE}
spec:
  description: Default project
  sourceRepos: ['*']
  destinations:
    - namespace: '*'
      server: '*'
  clusterResourceWhitelist:
    - group: '*'
      kind: '*'
APPPROJ

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

echo "==> Provisioning Git credentials for workflow execution namespace (DD-WE-006)..."
kubectl create namespace kubernaut-workflows 2>/dev/null || true
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: gitea-repo-creds
  namespace: kubernaut-workflows
  labels:
    kubernaut.ai/dependency-type: git-credentials
stringData:
  username: kubernaut
  password: kubernaut123
EOF

echo "==> ArgoCD setup complete"
echo "    Namespace: ${ARGOCD_NAMESPACE}"
echo "    Gitea repo registered: ${GITEA_REPO_URL}"
echo "    Git credentials provisioned in kubernaut-workflows (DD-WE-006)"
