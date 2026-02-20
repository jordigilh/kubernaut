#!/usr/bin/env bash
# Node NotReady Demo -- Automated Runner
# Scenario #127: Node failure -> cordon + drain
#
# Prerequisites:
#   - Kind cluster with worker node (kubernaut.ai/workload-node=true)
#   - Kubernaut services deployed (HAPI with real LLM backend)
#   - Prometheus with kube-state-metrics
#   - Podman (to pause/unpause Kind node container)
#
# Usage: ./deploy/demo/scenarios/node-notready/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-node"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/kind-config.yaml" "${1:-}"

echo "============================================="
echo " Node NotReady Demo (#127)"
echo "============================================="
echo ""

# Step 1: Deploy namespace and workload
echo "==> Step 1: Deploying namespace and web-service (3 replicas)..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"

# Step 2: Deploy Prometheus alerting rules
echo "==> Step 2: Deploying NodeNotReady alerting rule..."
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Step 3: Wait for healthy deployment
echo "==> Step 3: Waiting for web-service to be ready..."
kubectl wait --for=condition=Available deployment/web-service \
  -n "${NAMESPACE}" --timeout=120s
echo "  web-service is running (3 replicas)."
kubectl get pods -n "${NAMESPACE}" -o wide
echo ""

# Step 4: Simulate node failure
echo "==> Step 4: Simulating node failure via podman pause..."
bash "${SCRIPT_DIR}/inject-node-failure.sh"
echo ""

# Step 5: Wait for alert
echo "==> Step 5: Waiting for NodeNotReady alert to fire (~1-2 min)..."
echo "  Check: kubectl get nodes -w"
echo "  Check Prometheus: http://localhost:9190/alerts"
echo ""

# Step 6: Monitor pipeline
echo "==> Step 6: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "  Expected flow:"
echo "    Alert (NodeNotReady) -> Gateway -> SP -> AA (HAPI) -> RO -> WE"
echo "    LLM diagnoses node failure -> selects CordonDrainNode"
echo "    WE cordons + drains node -> pods rescheduled to healthy nodes"
echo "    EM verifies all pods Running on healthy nodes"
echo ""
echo "==> To verify remediation succeeded:"
echo "    kubectl get nodes"
echo "    kubectl get pods -n ${NAMESPACE} -o wide"
echo ""
echo "==> To restore the node after demo:"
echo "    WORKER=\$(kubectl get nodes -l kubernaut.ai/workload-node=true -o name | head -1 | sed 's|node/||')"
echo "    podman unpause \$WORKER"
echo "    kubectl uncordon \$WORKER"
