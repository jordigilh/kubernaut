#!/usr/bin/env bash
# Orphaned PVC Demo -- No Action Required
# Scenario #122: Orphaned PVCs alert -> LLM determines no remediation needed
#
# KEY: No workflow is seeded in DataStorage for this scenario. Orphaned PVCs
# are housekeeping, not a real operational issue. The LLM evaluates the alert,
# correctly identifies it as benign dangling resources, and sets the AIAnalysis
# outcome to WorkflowNotNeeded. The RO then marks the RR as NoActionRequired.
#
# Prerequisites:
#   - Kind cluster (kubernaut-demo) with platform installed
#   - Prometheus with kube-state-metrics
#   - StorageClass "standard" available (default in Kind)
#
# Usage: ./deploy/demo/scenarios/orphaned-pvc-no-action/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-orphaned-pvc"

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

# NOTE: We intentionally do NOT seed a workflow for this scenario.
# Orphaned PVCs are housekeeping, not a critical issue. The LLM should
# correctly identify this as benign and conclude no action is needed.

echo "============================================="
echo " Orphaned PVC Demo (#122)"
echo " Dangling Resources -> NoActionRequired"
echo "============================================="
echo ""

# Step 1: Deploy namespace and workload
echo "==> Step 1: Deploying namespace and data-processor..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"

# Step 2: Deploy Prometheus alerting rules
echo "==> Step 2: Deploying orphaned PVC alerting rule..."
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Step 3: Wait for healthy deployment
echo "==> Step 3: Waiting for data-processor to be ready..."
kubectl wait --for=condition=Available deployment/data-processor \
  -n "${NAMESPACE}" --timeout=120s
echo "  data-processor is running."
kubectl get pods -n "${NAMESPACE}"
echo ""

# Step 4: Inject orphaned PVCs
echo "==> Step 4: Creating orphaned PVCs from simulated batch jobs..."
bash "${SCRIPT_DIR}/inject-orphan-pvcs.sh"
echo ""

echo "==> Step 5: Fault injected. Waiting for KubePersistentVolumeClaimOrphaned alert (~2 min)."

# Validate pipeline
if [ "${SKIP_VALIDATE}" != "true" ] && [ -f "${SCRIPT_DIR}/validate.sh" ]; then
    echo ""
    echo "==> Running validation pipeline..."
    bash "${SCRIPT_DIR}/validate.sh" "${APPROVE_MODE}"
fi
