#!/usr/bin/env bash
# Memory Limits GitOps (Ansible) Demo -- Automated Runner
# Scenario #312: OOMKill on GitOps-managed deployment -> Ansible/AWX updates
# memory limits in Git -> ArgoCD syncs -> EA verifies
#
# First demo scenario using the Ansible execution engine (AWX).
#
# Prerequisites:
#   - Kind cluster with Kubernaut platform deployed
#   - AWX deployed (run: bash deploy/demo/scripts/awx-helper.sh)
#   - Gitea + ArgoCD deployed
#   - Prometheus with kube-state-metrics
#
# Usage:
#   ./deploy/demo/scenarios/memory-limits-gitops-ansible/run.sh
#   ./deploy/demo/scenarios/memory-limits-gitops-ansible/run.sh setup
#   ./deploy/demo/scenarios/memory-limits-gitops-ansible/run.sh inject
#   ./deploy/demo/scenarios/memory-limits-gitops-ansible/run.sh all
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-memory-gitops-ansible"
GITEA_NAMESPACE="gitea"
GITEA_ADMIN_USER="kubernaut"
GITEA_ADMIN_PASS="kubernaut123"
REPO_NAME="demo-memory-gitops-repo"

APPROVE_MODE="--auto-approve"
SKIP_VALIDATE=""
SUBCOMMAND="all"
for _arg in "$@"; do
    case "$_arg" in
        --auto-approve)  APPROVE_MODE="--auto-approve" ;;
        --interactive)   APPROVE_MODE="--interactive" ;;
        --no-validate)   SKIP_VALIDATE=true ;;
        setup|inject|all) SUBCOMMAND="$_arg" ;;
    esac
done

# shellcheck source=../../scripts/platform-helper.sh
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
require_demo_ready
# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
require_infra awx
require_infra gitea
require_infra argocd

run_setup() {
echo "============================================="
echo " Memory Limits GitOps (Ansible) Demo (#312)"
echo " OOMKill -> AWX playbook -> git commit -> ArgoCD sync"
echo "============================================="
echo ""

# Step 1: Push deployment YAML to Gitea repo
echo "==> Step 1: Pushing deployment manifests to Gitea..."
WORK_DIR=$(mktemp -d)
kubectl port-forward -n "${GITEA_NAMESPACE}" svc/gitea-http 3000:3000 &
PF_PID=$!
sleep 3

# Create repo if it doesn't exist
curl -s -X POST "http://localhost:3000/api/v1/user/repos" \
  -u "${GITEA_ADMIN_USER}:${GITEA_ADMIN_PASS}" \
  -H "Content-Type: application/json" \
  -d "{\"name\": \"${REPO_NAME}\", \"auto_init\": true}" -o /dev/null 2>/dev/null || true

cd "${WORK_DIR}"
git clone "http://${GITEA_ADMIN_USER}:${GITEA_ADMIN_PASS}@localhost:3000/${GITEA_ADMIN_USER}/${REPO_NAME}.git" repo 2>/dev/null
cd repo
git config user.email "kubernaut@kubernaut.ai"
git config user.name "Kubernaut Setup"

mkdir -p memory-gitops-ansible

cat > memory-gitops-ansible/deployment.yaml <<'MANIFEST'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: memory-consumer
  namespace: demo-memory-gitops-ansible
  labels:
    app: memory-consumer
    kubernaut.ai/managed: "true"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: memory-consumer
  template:
    metadata:
      labels:
        app: memory-consumer
    spec:
      containers:
      - name: consumer
        image: alpine:3.19
        command: ["/bin/sh", "-c"]
        args:
        - |
          echo "Starting memory consumer..."
          # Allocate ~8Mi every 2 seconds until OOMKilled
          while true; do
            dd if=/dev/urandom bs=1M count=8 2>/dev/null | cat > /dev/null &
            sleep 2
          done
        resources:
          requests:
            memory: "32Mi"
            cpu: "10m"
          limits:
            memory: "64Mi"
            cpu: "50m"
MANIFEST

git add .
if git diff --cached --quiet 2>/dev/null; then
    echo "  Gitea repo already has deployment manifests."
else
    git commit -m "feat: initial memory-consumer deployment (64Mi limit)"
    git push origin main
    echo "  Deployment manifest pushed to Gitea."
fi

kill "${PF_PID}" 2>/dev/null || true
cd /
rm -rf "${WORK_DIR}"

# Step 2: Deploy namespace and Prometheus rule
echo "==> Step 2: Deploying namespace and alerting rule..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Speed up ArgoCD polling for demo
kubectl patch configmap argocd-cm -n argocd --type merge \
  -p '{"data":{"timeout.reconciliation":"60s"}}' 2>/dev/null || true

# Step 3: Deploy ArgoCD Application
echo "==> Step 3: Creating ArgoCD Application..."
kubectl apply -f "${SCRIPT_DIR}/manifests/argocd-application.yaml"

echo "  Waiting for ArgoCD sync..."
for i in $(seq 1 30); do
    if kubectl get deployment memory-consumer -n "${NAMESPACE}" &>/dev/null; then
        echo "  ArgoCD synced deployment (attempt ${i})."
        break
    fi
    sleep 5
done

echo "==> Step 4: Waiting for initial OOMKill (~30s-1m)..."
echo "  The memory-consumer allocates ~8Mi every 2s. With 64Mi limit, OOMKill expected shortly."
echo ""
}

run_inject() {
# The fault injection is built into the deployment itself -- the container
# consumes more memory than its 64Mi limit allows. Once the OOMKill occurs,
# Prometheus fires the ContainerOOMKilling alert, starting the pipeline.
echo "==> Fault injection: built-in (deployment consumes > 64Mi limit)"
echo "  Waiting for ContainerOOMKilling alert to fire..."
echo ""
}

run_monitor() {
echo "==> Pipeline in progress..."
echo ""
echo "  Expected flow:"
echo "    1. ContainerOOMKilling alert fires"
echo "    2. AI Analysis selects IncreaseMemoryLimits, provides NEW_MEMORY_LIMIT"
echo "    3. WFE dispatches to AWX (engine=ansible)"
echo "    4. Ansible playbook clones Gitea repo, updates memory limits, commits, pushes"
echo "    5. ArgoCD syncs the updated deployment"
echo "    6. EA verifies alert resolves"
echo ""

if [ "${SKIP_VALIDATE}" != "true" ] && [ -f "${SCRIPT_DIR}/validate.sh" ]; then
    echo ""
    echo "==> Running validation pipeline..."
    bash "${SCRIPT_DIR}/validate.sh" "${APPROVE_MODE}"
fi
}

case "$SUBCOMMAND" in
  setup)  run_setup ;;
  inject) run_inject ;;
  all)    run_setup; run_inject; run_monitor ;;
  *)      echo "Usage: $0 [setup|inject|all]"; exit 1 ;;
esac
