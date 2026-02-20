#!/usr/bin/env bash
# Stuck Rollout Demo -- Automated Runner
# Scenario #130: Bad image -> stuck rollout -> rollback
#
# Prerequisites:
#   - Kind cluster with deploy/demo/overlays/kind/kind-cluster-config.yaml
#   - Kubernaut services deployed (HAPI with real LLM backend)
#   - Prometheus with kube-state-metrics
#
# Usage: ./deploy/demo/scenarios/stuck-rollout/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-rollout"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/kind-config.yaml" "${1:-}"

echo "============================================="
echo " Stuck Rollout Demo (#130)"
echo "============================================="
echo ""

# Step 1: Deploy namespace, deployment, and service
echo "==> Step 1: Deploying namespace and checkout-api (3 replicas)..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"

# Step 2: Deploy Prometheus alerting rules
echo "==> Step 2: Deploying stuck rollout alerting rule..."
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Step 3: Wait for healthy deployment
echo "==> Step 3: Waiting for checkout-api to be ready..."
kubectl wait --for=condition=Available deployment/checkout-api \
  -n "${NAMESPACE}" --timeout=120s
echo "  checkout-api is running (3 replicas with nginx:1.27-alpine)."
kubectl get pods -n "${NAMESPACE}"
echo ""

# Step 4: Establish baseline
echo "==> Step 4: Establishing baseline (15s)..."
sleep 15
echo ""

# Step 5: Inject bad image
echo "==> Step 5: Injecting non-existent image tag (triggers stuck rollout)..."
bash "${SCRIPT_DIR}/inject-bad-image.sh"
echo ""

# Step 6: Wait for stuck rollout + alert
echo "==> Step 6: Waiting for rollout to exceed progressDeadlineSeconds (~2 min)..."
echo "  Then the KubernautStuckRollout alert fires after 1 min more (~3 min total)."
echo "  Check Prometheus: http://localhost:9190/alerts"
echo ""

# Step 7: Monitor pipeline
echo "==> Step 7: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "  Expected flow:"
echo "    Alert (StuckRollout) -> Gateway -> SP -> AA (HAPI) -> RO -> WE"
echo "    LLM diagnoses stuck rollout from bad image -> selects rollback"
echo "    WE rolls back deployment -> EM verifies all pods Running"
echo ""
echo "==> To verify remediation succeeded:"
echo "    kubectl get pods -n ${NAMESPACE}"
echo "    kubectl rollout history deployment/checkout-api -n ${NAMESPACE}"
