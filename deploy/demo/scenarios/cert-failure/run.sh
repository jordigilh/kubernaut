#!/usr/bin/env bash
# cert-manager Certificate Failure Demo -- Automated Runner
# Scenario #133: CA Secret deleted -> Certificate NotReady -> fix issuer
#
# Prerequisites:
#   - Kind cluster
#   - Prometheus with cert-manager metrics
#
# Usage: ./deploy/demo/scenarios/cert-failure/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-cert-failure"

APPROVE_MODE="--auto-approve"
SKIP_VALIDATE=""
for _arg in "$@"; do
    case "$_arg" in
        --auto-approve)  APPROVE_MODE="--auto-approve" ;;
        --interactive)   APPROVE_MODE="--interactive" ;;
        --no-validate)   SKIP_VALIDATE=true ;;
    esac
done

# shellcheck source=../../scripts/platform-helper.sh
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
require_demo_ready
# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
require_infra cert-manager

echo "============================================="
echo " cert-manager Certificate Failure Demo (#133)"
echo "============================================="
echo ""

# Step 1: Generate a self-signed CA and create the CA Secret
echo "==> Step 1: Generating self-signed CA key pair..."
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
echo "==> Step 7: Waiting for CertManagerCertNotReady alert to fire (~2-3 min)..."
echo "  cert-manager will fail to re-issue the certificate."
echo "  Check Prometheus: kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090"
echo ""

# Step 8: Validate pipeline
if [ "${SKIP_VALIDATE}" != "true" ] && [ -f "${SCRIPT_DIR}/validate.sh" ]; then
    echo ""
    echo "==> Running validation pipeline..."
    bash "${SCRIPT_DIR}/validate.sh" "${APPROVE_MODE}"
fi
