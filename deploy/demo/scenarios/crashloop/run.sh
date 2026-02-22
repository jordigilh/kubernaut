#!/usr/bin/env bash
# CrashLoopBackOff Demo -- Automated Runner
# Scenario #120: Bad config deploy -> CrashLoopBackOff -> rollback
#
# Prerequisites:
#   - Kind cluster with deploy/demo/overlays/kind/kind-cluster-config.yaml
#   - Prometheus with kube-state-metrics scraping
#
# Usage: ./deploy/demo/scenarios/crashloop/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-crashloop"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/kind-config.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform
seed_scenario_workflow "crashloop"

echo "============================================="
echo " CrashLoopBackOff Remediation Demo (#120)"
echo "============================================="
echo ""

# Step 1: Deploy namespace and workload with healthy config
echo "==> Step 1: Deploying namespace, healthy worker, and service..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/configmap.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"

# Step 2: Deploy Prometheus alerting rules
echo "==> Step 2: Deploying CrashLoop detection alerting rule..."
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Step 3: Wait for healthy deployment
echo "==> Step 3: Waiting for worker to be healthy..."
kubectl wait --for=condition=Available deployment/worker \
  -n "${NAMESPACE}" --timeout=120s
echo "  Worker is running with valid configuration."
kubectl get pods -n "${NAMESPACE}"
echo ""

# Step 4: Establish baseline (let Prometheus scrape healthy state)
echo "==> Step 4: Establishing healthy baseline (20s)..."
sleep 20
echo "  Baseline established. Restart count is 0."
echo ""

# Step 5: Inject bad configuration
echo "==> Step 5: Injecting invalid nginx config (triggers CrashLoopBackOff)..."
bash "${SCRIPT_DIR}/inject-bad-config.sh"
echo ""

# Step 6: Wait for alert
echo "==> Step 6: Waiting for CrashLoop alert to fire (~2-3 min)..."
echo "  Pods will fail to start with 'unknown directive' error."
echo "  Check Prometheus: kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090"
echo "  The KubePodCrashLooping alert fires after >3 restarts in 10 min."
echo ""

# Step 7: Monitor pipeline
echo "==> Step 7: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "  Expected flow:"
echo "    Alert (KubePodCrashLooping) -> Gateway -> SP -> AA (HAPI) -> RO -> WE"
echo "    LLM diagnoses bad config causing CrashLoop -> selects rollback"
echo "    WE rolls back deployment to previous working revision"
echo "    EM verifies pods are running and restart count stabilizes"
echo ""
echo "==> To verify remediation succeeded:"
echo "    kubectl get pods -n ${NAMESPACE}"
echo "    # All pods should be Running/Ready with no recent restarts"
echo "    kubectl rollout history deployment/worker -n ${NAMESPACE}"
