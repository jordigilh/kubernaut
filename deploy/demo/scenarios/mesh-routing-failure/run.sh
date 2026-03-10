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
# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
require_infra linkerd

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

echo "  Waiting for deployments to be ready..."
kubectl wait --for=condition=Available deployment/api-server \
  -n "${NAMESPACE}" --timeout=120s
kubectl wait --for=condition=Available deployment/traffic-gen \
  -n "${NAMESPACE}" --timeout=120s
echo "  Workload and traffic generator deployed with Linkerd sidecars."
kubectl get pods -n "${NAMESPACE}"
echo ""

# Step 2: Baseline -- let traffic flow so Prometheus scrapes healthy metrics
echo "==> Step 2: Establishing healthy baseline (30s)..."
echo "  traffic-gen is sending requests to api-server through the mesh."
sleep 30
echo "  Baseline established."
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
# Validate pipeline
if [ "${SKIP_VALIDATE}" != "true" ] && [ -f "${SCRIPT_DIR}/validate.sh" ]; then
    echo ""
    echo "==> Running validation pipeline..."
    bash "${SCRIPT_DIR}/validate.sh" "${APPROVE_MODE}"
fi
