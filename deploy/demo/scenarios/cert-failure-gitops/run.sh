#!/usr/bin/env bash
# cert-manager GitOps Demo -- Automated Runner
# Scenario #134: GitOps-managed Certificate failure -> git-based fix
#
# Prerequisites:
#   - Kind cluster with Kubernaut services
#   - cert-manager installed (or run cert-failure scenario first)
#
# Usage: ./deploy/demo/scenarios/cert-failure-gitops/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-cert-gitops"
GITEA_NAMESPACE="gitea"
GITEA_ADMIN_USER="kubernaut"
GITEA_ADMIN_PASS="kubernaut123"
REPO_NAME="demo-cert-gitops-repo"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/kind-config.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform
ensure_cert_manager

echo "============================================="
echo " cert-manager GitOps Failure Demo (#134)"
echo "============================================="
echo ""

# Step 1: Generate self-signed CA
echo "==> Step 1: Generating self-signed CA key pair..."
TMPDIR_CA=$(mktemp -d)
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "${TMPDIR_CA}/ca.key" -out "${TMPDIR_CA}/ca.crt" \
  -days 365 -subj "/CN=Demo CA/O=Kubernaut"
kubectl create secret tls demo-ca-key-pair \
  --cert="${TMPDIR_CA}/ca.crt" --key="${TMPDIR_CA}/ca.key" \
  -n cert-manager --dry-run=client -o yaml | kubectl apply -f -
rm -rf "${TMPDIR_CA}"

# Step 3: Ensure GitOps infrastructure
echo "==> Step 3: Checking GitOps infrastructure..."
if ! kubectl get namespace gitea &>/dev/null; then
  echo "  Installing Gitea..."
  bash "${SCRIPT_DIR}/../gitops/scripts/setup-gitea.sh"
fi
if ! kubectl get namespace argocd &>/dev/null; then
  echo "  Installing ArgoCD..."
  bash "${SCRIPT_DIR}/../gitops/scripts/setup-argocd.sh"
fi

# Step 4: Create Gitea repo with cert-manager manifests
echo "==> Step 4: Pushing cert-manager manifests to Gitea..."
WORK_DIR=$(mktemp -d)
kubectl port-forward -n "${GITEA_NAMESPACE}" svc/gitea-http 3000:3000 &
PF_PID=$!
sleep 3

# Create repo in Gitea
curl -s -X POST "http://localhost:3000/api/v1/user/repos" \
  -u "${GITEA_ADMIN_USER}:${GITEA_ADMIN_PASS}" \
  -H "Content-Type: application/json" \
  -d "{\"name\": \"${REPO_NAME}\", \"auto_init\": true}" -o /dev/null || true

cd "${WORK_DIR}"
git clone "http://${GITEA_ADMIN_USER}:${GITEA_ADMIN_PASS}@localhost:3000/${GITEA_ADMIN_USER}/${REPO_NAME}.git" repo
cd repo
git config user.email "kubernaut@kubernaut.ai"
git config user.name "Kubernaut Setup"

mkdir -p manifests

cat > manifests/namespace.yaml <<'MANIFEST'
apiVersion: v1
kind: Namespace
metadata:
  name: demo-cert-gitops
  labels:
    kubernaut.ai/environment: production
    kubernaut.ai/business-unit: platform
    kubernaut.ai/service-owner: platform-team
    kubernaut.ai/criticality: high
    kubernaut.ai/sla-tier: tier-1
MANIFEST

cat > manifests/clusterissuer.yaml <<'MANIFEST'
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: demo-selfsigned-ca-gitops
spec:
  ca:
    secretName: demo-ca-key-pair
MANIFEST

cat > manifests/certificate.yaml <<'MANIFEST'
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: demo-app-cert
  namespace: demo-cert-gitops
spec:
  secretName: demo-app-tls
  issuerRef:
    name: demo-selfsigned-ca-gitops
    kind: ClusterIssuer
  dnsNames:
    - demo-app.demo-cert-gitops.svc.cluster.local
    - demo-app
  duration: 2160h
  renewBefore: 360h
MANIFEST

cat > manifests/deployment.yaml <<'MANIFEST'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo-app
  namespace: demo-cert-gitops
  labels:
    app: demo-app
    kubernaut.ai/managed: "true"
spec:
  replicas: 2
  selector:
    matchLabels:
      app: demo-app
  template:
    metadata:
      labels:
        app: demo-app
    spec:
      containers:
      - name: nginx
        image: nginx:1.27-alpine
        ports:
        - containerPort: 443
        volumeMounts:
        - name: tls
          mountPath: /etc/nginx/ssl
          readOnly: true
        resources:
          requests:
            memory: "32Mi"
            cpu: "10m"
          limits:
            memory: "64Mi"
            cpu: "50m"
      volumes:
      - name: tls
        secret:
          secretName: demo-app-tls
          optional: true
---
apiVersion: v1
kind: Service
metadata:
  name: demo-app
  namespace: demo-cert-gitops
  labels:
    app: demo-app
    kubernaut.ai/managed: "true"
spec:
  selector:
    app: demo-app
  ports:
  - port: 443
    targetPort: 443
  type: ClusterIP
MANIFEST

git add .
git commit -m "feat: initial cert-manager resources (healthy state)"
git push origin main

kill "${PF_PID}" 2>/dev/null || true
cd /
rm -rf "${WORK_DIR}"

# Step 5: Deploy ArgoCD Application
echo "==> Step 5: Creating ArgoCD Application..."
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/argocd-application.yaml"

echo "  Waiting for ArgoCD sync..."
sleep 15

echo "==> Step 6: Waiting for Certificate to become Ready..."
for i in $(seq 1 30); do
  STATUS=$(kubectl get certificate demo-app-cert -n "${NAMESPACE}" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "Unknown")
  if [ "$STATUS" = "True" ]; then
    echo "  Certificate is Ready."
    break
  fi
  echo "  Attempt $i/30: Certificate status=$STATUS, waiting..."
  sleep 5
done
kubectl get certificate -n "${NAMESPACE}"
echo ""

# Step 7: Baseline
echo "==> Step 7: Establishing healthy baseline (20s)..."
sleep 20
echo "  Baseline established."
echo ""

# Step 8: Inject failure via git push
echo "==> Step 8: Injecting failure (pushing broken ClusterIssuer via git)..."
WORK_DIR=$(mktemp -d)
kubectl port-forward -n "${GITEA_NAMESPACE}" svc/gitea-http 3000:3000 &
PF_PID=$!
sleep 3

cd "${WORK_DIR}"
git clone "http://${GITEA_ADMIN_USER}:${GITEA_ADMIN_PASS}@localhost:3000/${GITEA_ADMIN_USER}/${REPO_NAME}.git" repo
cd repo
git config user.email "bad-actor@example.com"
git config user.name "Bad Deploy"

# Break: change ClusterIssuer to reference a non-existent CA secret
cat > manifests/clusterissuer.yaml <<'MANIFEST'
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: demo-selfsigned-ca-gitops
spec:
  ca:
    secretName: nonexistent-ca-secret
MANIFEST

git add .
git commit -m "chore: update CA secret reference (broken)"
git push origin main

kill "${PF_PID}" 2>/dev/null || true
cd /
rm -rf "${WORK_DIR}"
echo "  Bad commit pushed. ArgoCD will sync broken ClusterIssuer."

# Delete existing cert to force re-issuance with broken issuer
sleep 10
kubectl delete secret demo-app-tls -n "${NAMESPACE}" --ignore-not-found
kubectl annotate certificate demo-app-cert -n "${NAMESPACE}" \
  cert-manager.io/issuing-trigger="manual-$(date +%s)" --overwrite 2>/dev/null || true

echo ""
echo "==> Step 9: Waiting for CertManagerCertNotReady alert (~2-3 min)..."
echo "  ArgoCD synced broken ClusterIssuer -> cert-manager cannot sign."
echo "  Check Prometheus: kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090"
echo ""
echo "==> Step 10: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "  Expected flow:"
echo "    Alert (CertManagerCertNotReady) -> Gateway -> SP -> AA (HAPI)"
echo "    LLM detects gitOpsManaged=true, gitOpsTool=argocd"
echo "    LLM selects git-based fix -> workflow reverts the bad commit"
echo "    ArgoCD re-syncs restored ClusterIssuer"
echo "    EM verifies Certificate is Ready"
echo ""
echo "==> Verify remediation:"
echo "    kubectl get certificate -n ${NAMESPACE}"
