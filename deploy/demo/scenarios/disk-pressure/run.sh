#!/usr/bin/env bash
# Disk Pressure / Orphaned PVC Cleanup Demo -- Automated Runner
# Scenario #121: Orphaned PVCs from batch jobs -> cleanup
#
# Prerequisites:
#   - Kind cluster with deploy/demo/overlays/kind/kind-cluster-config.yaml
#   - Kubernaut services deployed (HAPI with real LLM backend)
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

echo "============================================="
echo " Disk Pressure / PVC Cleanup Demo (#121)"
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
echo "  Check Prometheus: http://localhost:9190/alerts"
echo "  The KubernautOrphanedPVCs alert fires when >3 bound PVCs in namespace."
echo ""

# Step 6: Monitor pipeline
echo "==> Step 6: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "  Expected flow:"
echo "    Alert (OrphanedPVCs) -> Gateway -> SP -> AA (HAPI) -> RO -> WE"
echo "    LLM identifies orphaned PVCs from completed batch jobs"
echo "    Selects CleanupPVC workflow -> deletes unmounted PVCs"
echo "    EM verifies PVC count reduced"
echo ""
echo "==> To verify remediation succeeded:"
echo "    kubectl get pvc -n ${NAMESPACE}"
echo "    # Orphaned PVCs should be deleted"
