#!/usr/bin/env bash
# GitOps Drift Remediation Demo -- Automated Runner
# Scenario #125: Signal != RCA (Pod crash -> ConfigMap is root cause)
#
# Prerequisites:
#   - Kind cluster with deploy/demo/overlays/kind/kind-cluster-config.yaml
#   - Gitea and ArgoCD installed (run setup scripts first)
#
# Usage: ./deploy/demo/scenarios/gitops-drift/run.sh [setup|inject|all]
#   setup  -- deploy infrastructure, ArgoCD app, and establish healthy baseline
#   inject -- push bad ConfigMap via git (assumes setup already ran)
#   all    -- run full flow (default)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SUBCOMMAND="${1:-all}"
if [[ "$SUBCOMMAND" =~ ^(setup|inject|all)$ ]]; then shift || true; fi

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/../kind-config-singlenode.yaml" "${1:-}"

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

run_setup() {
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

# Step 2: Create ArgoCD Application (namespace + workload managed by ArgoCD)
echo "==> Step 2: Creating ArgoCD Application..."
kubectl apply -f "${SCRIPT_DIR}/manifests/argocd-application.yaml"

echo "==> Step 3: Waiting for ArgoCD to sync and pods to be ready..."
echo "  Waiting for namespace to be created by ArgoCD..."
for i in $(seq 1 60); do
  if kubectl get namespace "${NAMESPACE}" &>/dev/null; then
    break
  fi
  sleep 5
done
kubectl wait --for=condition=Available deployment/web-frontend \
  -n "${NAMESPACE}" --timeout=180s
echo "  web-frontend is healthy."

# Step 4: Deploy Prometheus rules (namespace now exists)
echo "==> Step 4: Deploying Prometheus alerting rules..."
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Step 5: Establish baseline
echo ""
echo "==> Step 5: Initial state (healthy):"
kubectl get pods -n "${NAMESPACE}" -o wide
echo ""
}

run_inject() {
# Step 6: Inject failure -- push bad ConfigMap to Gitea
echo "==> Step 6: Injecting failure (bad ConfigMap via Git commit)..."
WORK_DIR=$(mktemp -d)
kubectl port-forward -n "${GITEA_NAMESPACE}" svc/gitea-http 3000:3000 &
PF_PID=$!
sleep 3

cd "${WORK_DIR}"
git clone "http://${GITEA_ADMIN_USER}:${GITEA_ADMIN_PASS}@localhost:3000/${GITEA_ADMIN_USER}/${REPO_NAME}.git" repo
cd repo

# Break the ConfigMap: inject an invalid nginx directive
# Also update the deployment annotation to force a rollout
cat > manifests/configmap.yaml <<'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: nginx-config
  namespace: demo-gitops
  labels:
    app: web-frontend
data:
  nginx.conf: |
    worker_processes auto;
    error_log /var/log/nginx/error.log warn;
    pid /tmp/nginx.pid;

    events {
        worker_connections 1024;
    }

    http {
        # INVALID: causes nginx to fail on startup
        invalid_directive_that_breaks_nginx on;

        server {
            listen 8080;
            server_name _;

            location / {
                return 200 'healthy\n';
                add_header Content-Type text/plain;
            }

            location /healthz {
                return 200 'ok\n';
                add_header Content-Type text/plain;
            }
        }
    }
EOF

# Also update deployment to force a pod rollout with the new config
cat > manifests/deployment.yaml <<'DEPLOY_EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-frontend
  namespace: demo-gitops
  labels:
    app: web-frontend
    kubernaut.ai/managed: "true"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web-frontend
  template:
    metadata:
      labels:
        app: web-frontend
        kubernaut.ai/managed: "true"
      annotations:
        kubernaut.ai/config-version: "broken"
    spec:
      containers:
      - name: nginx
        image: nginx:1.27-alpine
        ports:
        - containerPort: 8080
        volumeMounts:
        - name: config
          mountPath: /etc/nginx/nginx.conf
          subPath: nginx.conf
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "128Mi"
            cpu: "100m"
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 3
          periodSeconds: 5
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 2
          periodSeconds: 3
      volumes:
      - name: config
        configMap:
          name: nginx-config
DEPLOY_EOF

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
}

run_monitor() {
# Step 7: Wait for ArgoCD to sync and pods to crash
echo "==> Step 7: Waiting for ArgoCD to sync and pods to enter CrashLoopBackOff..."
echo "  ArgoCD poll interval is ~3 min. Waiting..."
sleep 60
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
}

case "$SUBCOMMAND" in
  setup)  run_setup ;;
  inject) run_inject ;;
  all)    run_setup; run_inject; run_monitor ;;
  *)      echo "Usage: $0 [setup|inject|all]"; exit 1 ;;
esac
