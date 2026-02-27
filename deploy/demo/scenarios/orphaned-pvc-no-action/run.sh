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

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/../kind-config-singlenode.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform

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
echo "    The alert fires when >3 bound PVCs exist in namespace ${NAMESPACE}."
echo ""
echo "  Expected pipeline:"
echo "    Alert -> Gateway -> SP -> AIAnalysis (benign, no workflow) -> NoActionRequired"
