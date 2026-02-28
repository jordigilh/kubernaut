#!/usr/bin/env bash
# Linkerd Mesh Routing Failure Demo -- Automated Runner
# Scenario #136: AuthorizationPolicy blocks traffic -> fix policy
#
# Prerequisites:
#   - Kind cluster with Kubernaut services
#   - Prometheus scraping Linkerd metrics
#
# Usage: ./deploy/demo/scenarios/mesh-routing-failure/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-mesh-failure"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/../kind-config-singlenode.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform
seed_scenario_workflow "mesh-routing-failure"
ensure_linkerd

echo "============================================="
echo " Linkerd Mesh Routing Failure Demo (#136)"
echo "============================================="
echo ""

# Step 1: Deploy workload
echo "==> Step 1: Deploying namespace and meshed workload..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/linkerd-podmonitor.yaml"

echo "  Waiting for deployment to be ready..."
kubectl wait --for=condition=Available deployment/api-server \
  -n "${NAMESPACE}" --timeout=120s
echo "  Workload deployed with Linkerd sidecar."
kubectl get pods -n "${NAMESPACE}"
echo ""

# Step 2: Baseline
echo "==> Step 2: Establishing healthy baseline (20s)..."
sleep 20
echo "  Baseline established. Traffic flowing through Linkerd proxy."
echo ""

# Step 3: Inject
echo "==> Step 3: Injecting restrictive AuthorizationPolicy..."
bash "${SCRIPT_DIR}/inject-deny-policy.sh"
echo ""

# Step 4: Monitor
echo "==> Step 4: Waiting for high error rate alert (~2-3 min)..."
echo "  Linkerd proxy will deny all inbound traffic (403 Forbidden)."
echo "  Check Prometheus: kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090"
echo ""
echo "==> Step 5: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,wfe,ea -n kubernaut-system -w"
echo ""
echo "  Expected flow:"
echo "    Alert (HighErrorRate/MeshUnauthorized) -> Gateway -> SP -> AA (HAPI)"
echo "    LLM detects serviceMesh=linkerd, diagnoses AuthorizationPolicy block"
echo "    WE removes the deny-all AuthorizationPolicy"
echo "    EM verifies traffic flowing, error rate drops"
echo ""
echo "==> Verify remediation:"
echo "    kubectl get authorizationpolicies -n ${NAMESPACE}"
echo "    kubectl get pods -n ${NAMESPACE}"
