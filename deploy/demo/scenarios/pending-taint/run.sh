#!/usr/bin/env bash
# Pending Pods Taint Removal Demo -- Automated Runner
# Scenario #122: Node taint blocks scheduling -> remove taint
#
# Prerequisites:
#   - Kind cluster with worker node (kubernaut.ai/managed=true)
#   - Prometheus with kube-state-metrics
#
# Usage: ./deploy/demo/scenarios/pending-taint/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-taint"

APPROVE_MODE="--auto-approve"
SKIP_VALIDATE=""
for _arg in "$@"; do
    case "$_arg" in
        --auto-approve)  APPROVE_MODE="--auto-approve" ;;
        --interactive)   APPROVE_MODE="--interactive" ;;
        --no-validate)   SKIP_VALIDATE=true ;;
    esac
done

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/../kind-config-multinode.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform
seed_scenario_workflow "pending-taint"

echo "============================================="
echo " Pending Pods - Taint Removal Demo (#122)"
echo "============================================="
echo ""

# Step 1: Apply taint to worker node FIRST (before deploying pods)
echo "==> Step 1: Tainting worker node with NoSchedule..."
bash "${SCRIPT_DIR}/inject-taint.sh"
echo ""

# Step 2: Deploy namespace and workload (pods will be Pending)
echo "==> Step 2: Deploying namespace and batch-processor..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"

# Step 3: Deploy Prometheus alerting rules
echo "==> Step 3: Deploying pending pods alerting rule..."
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Step 4: Verify pods are Pending
echo "==> Step 4: Verifying pods are stuck in Pending..."
sleep 5
kubectl get pods -n "${NAMESPACE}"
echo "  Pods should show Pending (taint blocks scheduling on worker node)."
echo ""

# Step 5: Wait for alert
echo "==> Step 5: Waiting for Pending alert to fire (~3 min)..."
echo "  Check Prometheus: kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090"
echo ""

# Step 6: Validate pipeline
if [ "${SKIP_VALIDATE}" != "true" ] && [ -f "${SCRIPT_DIR}/validate.sh" ]; then
    echo ""
    echo "==> Running validation pipeline..."
    bash "${SCRIPT_DIR}/validate.sh" "${APPROVE_MODE}"
fi
