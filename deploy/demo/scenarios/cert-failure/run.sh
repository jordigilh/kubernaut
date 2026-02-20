#!/usr/bin/env bash
# cert-manager Certificate Failure Demo -- Automated Runner
# Scenario #133: CA Secret deleted -> Certificate NotReady -> fix issuer
#
# Prerequisites:
#   - Kind cluster
#   - Kubernaut services deployed (HAPI with real LLM backend)
#   - Prometheus with cert-manager metrics
#
# Usage: ./deploy/demo/scenarios/cert-failure/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-cert-failure"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/kind-config.yaml" "${1:-}"

echo "============================================="
echo " cert-manager Certificate Failure Demo (#133)"
echo "============================================="
echo ""

# Step 1: Install cert-manager if not present
echo "==> Step 1: Ensuring cert-manager is installed..."
if ! kubectl get namespace cert-manager &>/dev/null; then
  echo "  Installing cert-manager..."
  kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.1/cert-manager.yaml
  echo "  Waiting for cert-manager to be ready..."
  kubectl wait --for=condition=Available deployment/cert-manager -n cert-manager --timeout=120s
  kubectl wait --for=condition=Available deployment/cert-manager-webhook -n cert-manager --timeout=120s
  kubectl wait --for=condition=Available deployment/cert-manager-cainjector -n cert-manager --timeout=120s
  sleep 10
else
  echo "  cert-manager already installed."
fi

# Step 2: Generate a self-signed CA and create the CA Secret
echo "==> Step 2: Generating self-signed CA key pair..."
TMPDIR=$(mktemp -d)
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "${TMPDIR}/ca.key" -out "${TMPDIR}/ca.crt" \
  -days 365 -subj "/CN=Demo CA/O=Kubernaut"
kubectl create secret tls demo-ca-key-pair \
  --cert="${TMPDIR}/ca.crt" --key="${TMPDIR}/ca.key" \
  -n cert-manager --dry-run=client -o yaml | kubectl apply -f -
rm -rf "${TMPDIR}"
echo "  CA Secret created in cert-manager namespace."

# Step 3: Deploy scenario resources
echo "==> Step 3: Deploying namespace, ClusterIssuer, Certificate, and workload..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/clusterissuer.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/certificate.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Step 4: Wait for certificate to be issued
echo "==> Step 4: Waiting for Certificate to become Ready..."
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

# Step 5: Baseline
echo "==> Step 5: Establishing healthy baseline (20s)..."
sleep 20
echo "  Baseline established. Certificate is Ready, workload is healthy."
echo ""

# Step 6: Inject failure
echo "==> Step 6: Injecting failure (deleting CA Secret)..."
bash "${SCRIPT_DIR}/inject-broken-issuer.sh"
echo ""

# Step 7: Wait for alert
echo "==> Step 7: Waiting for CertificateNotReady alert to fire (~2-3 min)..."
echo "  cert-manager will fail to re-issue the certificate."
echo "  Check Prometheus: http://localhost:9190/alerts"
echo ""

# Step 8: Monitor pipeline
echo "==> Step 8: Pipeline in progress. Monitor with:"
echo "    kubectl get certificate -n ${NAMESPACE} -w"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "  Expected flow:"
echo "    Alert (CertificateNotReady) -> Gateway -> SP -> AA (HAPI)"
echo "    LLM diagnoses missing CA Secret -> selects FixCertificate workflow"
echo "    WE recreates the CA Secret -> cert-manager re-issues certificate"
echo "    EM verifies Certificate is Ready"
echo ""
echo "==> To verify remediation succeeded:"
echo "    kubectl get certificate -n ${NAMESPACE}"
echo "    # Certificate should show Ready=True"
