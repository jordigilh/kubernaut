#!/usr/bin/env bash
# HPA Maxed Out Demo -- Automated Runner
# Scenario #123: HPA at ceiling -> temporarily raise maxReplicas
#
# Prerequisites:
#   - Kind cluster with deploy/demo/overlays/kind/kind-cluster-config.yaml
#   - Prometheus with kube-state-metrics
#   - metrics-server installed (for HPA CPU metrics)
#
# Usage: ./deploy/demo/scenarios/hpa-maxed/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-hpa"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/kind-config.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform
seed_scenario_workflow "hpa-maxed"
ensure_metrics_server

echo "============================================="
echo " HPA Maxed Out Demo (#123)"
echo "============================================="
echo ""

# Step 1: Deploy namespace, deployment, HPA, and service
echo "==> Step 1: Deploying namespace, api-frontend, HPA (max=3)..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"

# Step 2: Deploy Prometheus alerting rules
echo "==> Step 2: Deploying HPA maxed alerting rule..."
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Step 3: Wait for healthy deployment
echo "==> Step 3: Waiting for api-frontend to be ready..."
kubectl wait --for=condition=Available deployment/api-frontend \
  -n "${NAMESPACE}" --timeout=120s
echo "  api-frontend is running."
kubectl get pods -n "${NAMESPACE}"
kubectl get hpa -n "${NAMESPACE}"
echo ""

# Step 4: Establish baseline
echo "==> Step 4: Establishing baseline (15s)..."
sleep 15
echo ""

# Step 5: Inject CPU load
echo "==> Step 5: Generating CPU load to push HPA to ceiling..."
bash "${SCRIPT_DIR}/inject-load.sh"
echo ""

# Step 6: Wait for alert
echo "==> Step 6: Waiting for HPA to reach maxReplicas and alert to fire (~3-5 min)..."
echo "  Watch HPA: kubectl get hpa -n ${NAMESPACE} -w"
echo "  Check Prometheus: kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090"
echo ""

# Step 7: Monitor pipeline
echo "==> Step 7: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "  Expected flow:"
echo "    Alert (KubeHpaMaxedOut) -> Gateway -> SP -> AA (HAPI) -> RO -> WE"
echo "    LLM detects HPA (via detectedLabels:hpaEnabled) at ceiling"
echo "    Selects PatchHPA workflow -> raises maxReplicas"
echo "    HPA scales up -> EM verifies pods healthy"
echo ""
echo "==> To verify remediation succeeded:"
echo "    kubectl get hpa -n ${NAMESPACE}"
echo "    # maxReplicas should be > 3 (raised by workflow)"
