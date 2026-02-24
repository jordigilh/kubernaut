#!/usr/bin/env bash
# No-Action-Required Demo -- Automated Runner
# Scenario #122: Disk pressure alert -> LLM determines no remediation workflow needed
#
# KEY: No workflow is seeded in DataStorage for this scenario. The LLM
# evaluates the alert, finds no matching workflow, and sets the AIAnalysis
# outcome to WorkflowNotNeeded. The RO then marks the RR as NoActionRequired.
#
# Prerequisites:
#   - Kind cluster with deploy/demo/overlays/kind/kind-cluster-config.yaml
#   - Prometheus with kube-state-metrics
#   - StorageClass "standard" available (default in Kind)
#
# Usage: ./deploy/demo/scenarios/disk-pressure/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-disk"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/kind-config.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform

# NOTE: We intentionally do NOT seed a workflow for this scenario.
# The absence of a matching workflow forces the LLM to conclude that
# no automated remediation is available, yielding WorkflowNotNeeded.

echo "============================================="
echo " No-Action-Required Demo (#122)"
echo " Disk Pressure -> WorkflowNotNeeded"
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
echo ""

# Step 4: Inject orphaned PVCs
echo "==> Step 4: Creating orphaned PVCs from simulated batch jobs..."
bash "${SCRIPT_DIR}/inject-orphan-pvcs.sh"
echo ""

# Step 5: Wait for alert
echo "==> Step 5: Waiting for orphaned PVC alert to fire (~2 min)..."
echo "  Check Prometheus: kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090"
echo "  The KubePersistentVolumeClaimOrphaned alert fires when >3 bound PVCs in namespace."
echo ""

# Step 6: Monitor pipeline
echo "==> Step 6: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "  Expected flow:"
echo "    Alert (KubePersistentVolumeClaimOrphaned) -> Gateway -> SP -> AA (HAPI)"
echo "    LLM identifies orphaned PVCs but finds NO matching workflow in catalog"
echo "    AA sets outcome: WorkflowNotNeeded (no automated remediation available)"
echo "    RO marks RR as Completed with outcome: NoActionRequired"
echo ""
echo "==> To verify pipeline outcome:"
echo "    kubectl get rr -n ${NAMESPACE} -o jsonpath='{.items[0].status.overallPhase}'"
echo "    # Should show 'Completed'"
echo "    kubectl get rr -n ${NAMESPACE} -o jsonpath='{.items[0].status.outcome}'"
echo "    # Should show 'NoActionRequired'"
echo "    kubectl get aa -n ${NAMESPACE} -o jsonpath='{.items[0].status.outcome}'"
echo "    # Should show 'WorkflowNotNeeded'"
