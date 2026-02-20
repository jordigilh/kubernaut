#!/usr/bin/env bash
# PDB Deadlock Demo -- Automated Runner
# Scenario #124: Overly restrictive PDB blocks rolling update -> relax PDB
#
# Prerequisites:
#   - Kind cluster with deploy/demo/overlays/kind/kind-cluster-config.yaml
#   - Kubernaut services deployed (HAPI with real LLM backend)
#   - Prometheus with kube-state-metrics
#
# Usage: ./deploy/demo/scenarios/pdb-deadlock/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-pdb"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/kind-config.yaml" "${1:-}"

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
echo "==> Step 3: Waiting for payment-service to be ready..."
kubectl wait --for=condition=Available deployment/payment-service \
  -n "${NAMESPACE}" --timeout=120s
echo "  payment-service is running (2 replicas)."
kubectl get pods -n "${NAMESPACE}"
kubectl get pdb -n "${NAMESPACE}"
echo ""

# Step 4: Establish baseline
echo "==> Step 4: Establishing baseline (15s)..."
sleep 15
echo "  PDB shows: ALLOWED DISRUPTIONS = 0 (this is the problem)."
echo ""

# Step 5: Trigger rolling update (will be blocked by PDB)
echo "==> Step 5: Triggering rolling update (blocked by PDB)..."
bash "${SCRIPT_DIR}/inject-rolling-update.sh"
echo ""

# Step 6: Wait for alert
echo "==> Step 6: Waiting for PDB deadlock alert to fire (~3 min)..."
echo "  Check Prometheus: http://localhost:9190/alerts"
echo "  The KubernautPDBDeadlock alert fires when allowed disruptions = 0 for 3 min."
echo ""

# Step 7: Monitor pipeline
echo "==> Step 7: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "  Expected flow:"
echo "    Alert (PDBDeadlock) -> Gateway -> SP -> AA (HAPI) -> RO -> WE"
echo "    LLM detects PDB (via detectedLabels:pdbProtected) blocking updates"
echo "    Selects RelaxPDB workflow -> patches minAvailable to 1"
echo "    Blocked rollout resumes -> EM verifies pods healthy"
echo ""
echo "==> To verify remediation succeeded:"
echo "    kubectl get pdb -n ${NAMESPACE}"
echo "    # ALLOWED DISRUPTIONS should be > 0"
echo "    kubectl rollout status deployment/payment-service -n ${NAMESPACE}"
