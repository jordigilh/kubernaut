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
ensure_kind_cluster "${SCRIPT_DIR}/kind-config.yaml" "${1:-}"

echo "============================================="
echo " Linkerd Mesh Routing Failure Demo (#136)"
echo "============================================="
echo ""

# Step 1: Install Linkerd if not present
echo "==> Step 1: Ensuring Linkerd is installed..."
if ! kubectl get namespace linkerd &>/dev/null; then
  echo "  Installing Linkerd CRDs..."
  linkerd install --crds | kubectl apply -f -
  echo "  Installing Linkerd control plane..."
  linkerd install | kubectl apply -f -
  echo "  Waiting for Linkerd to be ready..."
  linkerd check --wait 120s
else
  echo "  Linkerd already installed."
fi

# Step 2: Deploy workload
echo "==> Step 2: Deploying namespace and meshed workload..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

echo "  Waiting for deployment to be ready..."
kubectl wait --for=condition=Available deployment/api-server \
  -n "${NAMESPACE}" --timeout=120s
echo "  Workload deployed with Linkerd sidecar."
kubectl get pods -n "${NAMESPACE}"
echo ""

# Step 3: Baseline
echo "==> Step 3: Establishing healthy baseline (20s)..."
sleep 20
echo "  Baseline established. Traffic flowing through Linkerd proxy."
echo ""

# Step 4: Inject
echo "==> Step 4: Injecting restrictive AuthorizationPolicy..."
bash "${SCRIPT_DIR}/inject-deny-policy.sh"
echo ""

# Step 5: Monitor
echo "==> Step 5: Waiting for high error rate alert (~2-3 min)..."
echo "  Linkerd proxy will deny all inbound traffic (403 Forbidden)."
echo "  Check Prometheus: http://localhost:9190/alerts"
echo ""
echo "==> Step 6: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
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
