#!/usr/bin/env bash
# PDB Deadlock Demo -- Automated Runner
# Scenario #124: Overly restrictive PDB blocks node drain -> relax PDB
#
# Prerequisites:
#   - Kind cluster with worker node (kubernaut.ai/managed=true)
#   - Prometheus with kube-state-metrics
#
# Usage: ./deploy/demo/scenarios/pdb-deadlock/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-pdb"

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
echo " PDB Deadlock Demo (#124)"
echo "============================================="
echo ""

# Step 1: Deploy namespace, deployment, PDB, and service
echo "==> Step 1: Deploying namespace, payment-service, and restrictive PDB..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"

# Step 2: Deploy Prometheus alerting rules
echo "==> Step 2: Deploying PDB deadlock alerting rule..."
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Step 3: Wait for healthy deployment
echo "==> Step 3: Waiting for payment-service to be ready on worker node..."
kubectl wait --for=condition=Available deployment/payment-service \
  -n "${NAMESPACE}" --timeout=120s
echo "  payment-service is running (2 replicas on worker node)."
kubectl get pods -n "${NAMESPACE}" -o wide
kubectl get pdb -n "${NAMESPACE}"
echo ""

# Step 4: Establish baseline
echo "==> Step 4: Establishing baseline (15s)..."
sleep 15
echo "  PDB shows: ALLOWED DISRUPTIONS = 0 (this is the problem)."
echo ""

# Step 5: Drain worker node (will be blocked by PDB)
echo "==> Step 5: Draining worker node (blocked by PDB)..."
bash "${SCRIPT_DIR}/inject-drain.sh"
echo ""

# Step 6: Wait for alert
echo "==> Step 6: Waiting for PDB deadlock alert to fire (~3 min)..."
echo "  Check Prometheus: kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090"
echo "  The KubePodDisruptionBudgetAtLimit alert fires when allowed disruptions = 0 for 3 min."
echo ""

# Step 7: Validate pipeline
if [ "${SKIP_VALIDATE}" != "true" ] && [ -f "${SCRIPT_DIR}/validate.sh" ]; then
    echo ""
    echo "==> Running validation pipeline..."
    bash "${SCRIPT_DIR}/validate.sh" "${APPROVE_MODE}"
fi

# Step 8: Post-maintenance -- uncordon worker and verify recovery
echo "==> Step 8: Post-maintenance -- uncordoning worker node..."
WORKER_NODE=$(kubectl get nodes -l kubernaut.ai/managed=true \
  -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
if [ -n "${WORKER_NODE}" ]; then
  kubectl uncordon "${WORKER_NODE}"
  echo "  Worker node ${WORKER_NODE} uncordoned."
else
  echo "  WARNING: No worker node found with label kubernaut.ai/managed=true"
fi

echo "  Waiting for all pods to be ready (60s timeout)..."
kubectl wait --for=condition=Available deployment/payment-service \
  -n "${NAMESPACE}" --timeout=60s
echo ""
echo "==> Final state:"
kubectl get nodes
kubectl get pdb -n "${NAMESPACE}"
kubectl get pods -n "${NAMESPACE}" -o wide
echo ""
echo "============================================="
echo " PDB Deadlock Demo -- COMPLETE"
echo "============================================="
