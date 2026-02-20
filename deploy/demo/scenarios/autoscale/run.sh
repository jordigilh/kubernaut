#!/usr/bin/env bash
# Cluster Autoscaling Demo -- Automated Runner
# Scenario #126: Pods stuck Pending -> provision new Kind node via kubeadm join
#
# Prerequisites:
#   - Kind cluster with deploy/demo/overlays/kind/kind-cluster-config.yaml
#   - Kubernaut services deployed (HAPI with real LLM backend)
#   - Podman available on host
#
# Usage: ./deploy/demo/scenarios/autoscale/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-autoscale"
PROVISIONER_PID=""

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/kind-config.yaml" "${1:-}"

cleanup_provisioner() {
  if [ -n "$PROVISIONER_PID" ]; then
    echo "==> Stopping provisioner agent (PID: $PROVISIONER_PID)..."
    kill "$PROVISIONER_PID" 2>/dev/null || true
  fi
}
trap cleanup_provisioner EXIT

echo "============================================="
echo " Cluster Autoscaling Demo (#126)"
echo "============================================="
echo ""

# Step 1: Deploy namespace and workload manifests
echo "==> Step 1: Deploying namespace and workload..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"

# Step 2: Deploy Prometheus alerting rules
echo "==> Step 2: Deploying Prometheus alerting rules..."
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Step 3: Wait for initial deployment to be ready (2 replicas)
echo "==> Step 3: Waiting for initial deployment (2 replicas)..."
kubectl wait --for=condition=Available deployment/web-cluster \
  -n "${NAMESPACE}" --timeout=120s
echo "  web-cluster is healthy with 2 replicas."
kubectl get pods -n "${NAMESPACE}" -o wide
echo ""

# Step 4: Register workflow in DataStorage (placeholder)
echo "==> Step 4: Workflow registration..."
echo "  TODO: Build and push provision-node-v1 OCI bundle, register via DataStorage API."
echo "  For now, ensure the workflow is pre-seeded in the catalog."
echo ""

# Step 5: Start the host-side provisioner agent in background
echo "==> Step 5: Starting provisioner agent..."
bash "${SCRIPT_DIR}/provisioner.sh" &
PROVISIONER_PID=$!
echo "  Provisioner running (PID: $PROVISIONER_PID)"
echo ""

# Step 6: Inject failure -- scale beyond node capacity
echo "==> Step 6: Injecting failure (scaling to 8 replicas)..."
kubectl scale deployment/web-cluster --replicas=8 -n "${NAMESPACE}"
echo "  Scaled to 8 replicas. With 8x512Mi = 4GB requested, worker node cannot fit all."
echo ""

# Step 7: Show pending pods
echo "==> Step 7: Waiting 15s for pods to enter Pending state..."
sleep 15
kubectl get pods -n "${NAMESPACE}" -o wide
echo ""

# Step 8: Monitor
echo "==> Step 8: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "  Expected flow:"
echo "    Alert (FailedScheduling) -> Gateway -> SP -> AA (HAPI) -> RO -> WE"
echo "    WE Job creates ScaleRequest -> Provisioner adds node -> WE verifies -> EM"
echo ""
echo "==> To verify remediation succeeded:"
echo "    kubectl get nodes                    # New worker node visible"
echo "    kubectl get pods -n ${NAMESPACE} -o wide  # All pods Running"
