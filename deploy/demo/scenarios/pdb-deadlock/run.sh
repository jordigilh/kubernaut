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

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/kind-config.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform
seed_scenario_workflow "pdb-deadlock"

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

# Step 7: Monitor pipeline
echo "==> Step 7: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "  Expected flow:"
echo "    Alert (KubePodDisruptionBudgetAtLimit) -> Gateway -> SP -> AA (HAPI) -> RO -> WE"
echo "    LLM detects PDB (via detectedLabels:pdbProtected) blocking node drain"
echo "    Selects RelaxPDB workflow -> patches minAvailable to 1"
echo "    Blocked drain resumes -> pods reschedule to control-plane"
echo "    EM verifies pods healthy after drain"
echo ""
echo "==> To verify remediation succeeded:"
echo "    kubectl get pdb -n ${NAMESPACE}"
echo "    # ALLOWED DISRUPTIONS should be > 0"
echo "    kubectl get nodes"
echo "    # Worker node should complete drain (SchedulingDisabled)"
echo "    kubectl get pods -n ${NAMESPACE} -o wide"
echo "    # Pods should be Running on control-plane node"
echo ""

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
