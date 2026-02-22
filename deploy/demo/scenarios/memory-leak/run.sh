#!/usr/bin/env bash
# Predictive Memory Exhaustion Demo -- Automated Runner
# Scenario #129: predict_linear detects OOM trend -> graceful restart
#
# Prerequisites:
#   - Kind cluster with deploy/demo/overlays/kind/kind-cluster-config.yaml
#   - Prometheus with kube-state-metrics and cAdvisor scraping
#
# Usage: ./deploy/demo/scenarios/memory-leak/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-memory-leak"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/kind-config.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform
seed_scenario_workflow "memory-leak"

echo "============================================="
echo " Predictive Memory Exhaustion Demo (#129)"
echo "============================================="
echo ""

# Step 1: Deploy namespace and workload
echo "==> Step 1: Deploying namespace and leaky-app deployment..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"

# Step 2: Deploy Prometheus alerting rules
echo "==> Step 2: Deploying predict_linear alerting rule..."
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Step 3: Wait for deployment to be healthy
echo "==> Step 3: Waiting for leaky-app to be ready..."
kubectl wait --for=condition=Available deployment/leaky-app \
  -n "${NAMESPACE}" --timeout=120s
echo "  leaky-app is running."
kubectl get pods -n "${NAMESPACE}"
echo ""

# Step 4: Register workflow in DataStorage (placeholder)
echo "==> Step 4: Workflow registration..."
echo "  TODO: Build and push graceful-restart-v1 OCI bundle, register via DataStorage API."
echo "  For now, ensure the workflow is pre-seeded in the catalog."
echo ""

# Step 5: Memory leak in progress
echo "==> Step 5: Memory leak is building..."
echo "  The 'leaker' sidecar allocates ~1MB every 15 seconds (~4MB/min)."
echo "  With a 192Mi limit, predict_linear will fire once it projects OOM"
echo "  within 30 minutes, typically after 10-15 minutes of trend data."
echo ""
echo "  Monitor memory growth:"
echo "    kubectl top pods -n ${NAMESPACE} --containers"
echo ""

# Step 6: Wait for alert
echo "==> Step 6: Waiting for ContainerMemoryExhaustionPredicted alert (~12-15 min)..."
echo "  Check Prometheus: kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090"
echo "  The ContainerMemoryExhaustionPredicted alert should appear once"
echo "  predict_linear projects the leaker container exceeding 192Mi."
echo ""

# Step 7: Monitor pipeline
echo "==> Step 7: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "  Expected flow:"
echo "    Alert (PredictiveMemoryExhaust) -> Gateway -> SP -> AA (HAPI) -> RO -> WE"
echo "    LLM identifies linear memory growth -> selects GracefulRestart"
echo "    WE performs rolling restart -> memory usage resets"
echo "    EM verifies pod memory back to baseline"
echo ""
echo "==> To verify remediation succeeded:"
echo "    kubectl top pods -n ${NAMESPACE} --containers"
echo "    # Memory for 'leaker' should be back near baseline after restart"
echo "    kubectl rollout history deployment/leaky-app -n ${NAMESPACE}"
