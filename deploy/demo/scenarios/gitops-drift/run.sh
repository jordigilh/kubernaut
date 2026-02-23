#!/usr/bin/env bash
# GitOps Drift Remediation Demo -- Automated Runner
# Scenario #125: Signal != RCA (Pod crash -> ConfigMap is root cause)
#
# Prerequisites:
#   - Kind cluster with deploy/demo/overlays/kind/kind-cluster-config.yaml
#   - Gitea and ArgoCD installed (run setup scripts first)
#
# Usage: ./deploy/demo/scenarios/gitops-drift/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/kind-config.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform
seed_scenario_workflow "gitops-drift"

GITEA_NAMESPACE="gitea"
GITEA_ADMIN_USER="kubernaut"
GITEA_ADMIN_PASS="kubernaut123"
REPO_NAME="demo-gitops-repo"
NAMESPACE="demo-gitops"

echo "============================================="
echo " GitOps Drift Remediation Demo (#125)"
echo "============================================="
echo ""

# Step 1: Ensure GitOps infrastructure is up
echo "==> Step 1: Checking GitOps infrastructure..."
if ! kubectl get namespace gitea &>/dev/null; then
  echo "  Gitea not found. Installing..."
  bash "${SCRIPT_DIR}/../gitops/scripts/setup-gitea.sh"
fi
if ! kubectl get namespace argocd &>/dev/null; then
  echo "  ArgoCD not found. Installing..."
  bash "${SCRIPT_DIR}/../gitops/scripts/setup-argocd.sh"
fi
echo "  GitOps infrastructure ready."

# Step 2: Deploy Prometheus rules
echo "==> Step 2: Deploying Prometheus alerting rules..."
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Step 3: Create ArgoCD Application (namespace + workload managed by ArgoCD)
echo "==> Step 3: Creating ArgoCD Application..."
kubectl apply -f "${SCRIPT_DIR}/manifests/argocd-application.yaml"

echo "==> Step 4: Waiting for ArgoCD to sync and pods to be ready..."
sleep 10
kubectl wait --for=condition=Available deployment/web-frontend \
  -n "${NAMESPACE}" --timeout=120s
echo "  web-frontend is healthy."

# Step 5: Show initial state
echo ""
echo "==> Step 5: Initial state (healthy):"
kubectl get pods -n "${NAMESPACE}" -o wide
echo ""

# Step 6: Inject failure -- push bad ConfigMap to Gitea
echo "==> Step 6: Injecting failure (bad ConfigMap via Git commit)..."
WORK_DIR=$(mktemp -d)
kubectl port-forward -n "${GITEA_NAMESPACE}" svc/gitea-http 3000:3000 &
PF_PID=$!
sleep 3

cd "${WORK_DIR}"
git clone "http://${GITEA_ADMIN_USER}:${GITEA_ADMIN_PASS}@localhost:3000/${GITEA_ADMIN_USER}/${REPO_NAME}.git" repo
cd repo

# Break the ConfigMap: set an invalid NGINX_PORT that causes nginx to crash
cat > manifests/configmap.yaml <<'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: nginx-config
  namespace: demo-gitops
  labels:
    app: web-frontend
data:
  NGINX_PORT: "INVALID_PORT_CAUSES_CRASH"
  NGINX_WORKER_PROCESSES: "auto"
EOF

git add .
git config user.email "bad-actor@example.com"
git config user.name "Bad Deploy"
git commit -m "chore: update nginx config (broken value)"
git push origin main

kill "${PF_PID}" 2>/dev/null || true
cd /
rm -rf "${WORK_DIR}"

echo "  Bad commit pushed to Gitea. ArgoCD will sync the broken ConfigMap."
echo ""

# Step 7: Wait for pods to crash
echo "==> Step 7: Waiting for pods to enter CrashLoopBackOff..."
sleep 30
kubectl get pods -n "${NAMESPACE}"
echo ""

# Step 8: Watch the Kubernaut pipeline
echo "==> Step 8: Watching Kubernaut pipeline (Ctrl+C to stop watching)..."
echo "  Expected flow: Alert -> Gateway -> SP -> AA (HAPI) -> RO -> WE (git revert) -> EM"
echo ""
echo "  Monitoring CRDs:"
kubectl get remediationrequests,signalprocessings,aianalyses,workflowexecutions,effectivenessassessments \
  -n "${NAMESPACE}" 2>/dev/null || echo "  (no CRDs yet -- waiting for alert to fire)"

echo ""
echo "==> Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "==> To verify remediation succeeded:"
echo "    kubectl get pods -n ${NAMESPACE}"
echo "    # All pods should return to Running after git revert"
