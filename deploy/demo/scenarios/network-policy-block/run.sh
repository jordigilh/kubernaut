#!/usr/bin/env bash
# NetworkPolicy Traffic Block Demo -- Automated Runner
# Scenario #138: Deny-all NetworkPolicy -> health checks fail -> fix policy
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-netpol"

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

echo "============================================="
echo " NetworkPolicy Traffic Block Demo (#138)"
echo "============================================="
echo ""

echo "==> Step 1: Deploying namespace, workload, and baseline NetworkPolicy..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/networkpolicy-allow.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

echo "==> Step 2: Waiting for deployment to be healthy..."
kubectl wait --for=condition=Available deployment/web-frontend \
  -n "${NAMESPACE}" --timeout=120s
kubectl get pods -n "${NAMESPACE}"
echo ""

echo "==> Step 3: Establishing healthy baseline (20s)..."
sleep 20
echo "  Baseline established."
echo ""

echo "==> Step 4: Injecting deny-all NetworkPolicy..."
bash "${SCRIPT_DIR}/inject-deny-all-netpol.sh"
echo ""

echo "==> Step 5: Waiting for KubeDeploymentReplicasMismatch alert (~3-4 min)..."
echo "  Health checks will fail -> pods become NotReady -> restarts begin."
echo "  Check Prometheus: kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090"
echo ""
# Validate pipeline
if [ "${SKIP_VALIDATE}" != "true" ] && [ -f "${SCRIPT_DIR}/validate.sh" ]; then
    echo ""
    echo "==> Running validation pipeline..."
    bash "${SCRIPT_DIR}/validate.sh" "${APPROVE_MODE}"
fi
