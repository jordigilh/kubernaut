#!/usr/bin/env bash
# CrashLoopBackOff Helm Demo -- Automated Runner
# Scenario #135: Helm-managed bad config -> CrashLoopBackOff -> helm rollback
#
# Prerequisites:
#   - Kind cluster with Kubernaut services
#   - Helm 3 installed
#   - Prometheus with kube-state-metrics
#
# Usage: ./deploy/demo/scenarios/crashloop-helm/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-crashloop-helm"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/../kind-config-singlenode.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform
seed_scenario_workflow "crashloop-helm"

echo "============================================="
echo " Helm CrashLoopBackOff Remediation Demo (#135)"
echo "============================================="
echo ""

# Step 1: Install via Helm
echo "==> Step 1: Installing workload via Helm chart..."
helm upgrade --install demo-crashloop-helm "${SCRIPT_DIR}/chart" \
  -n "${NAMESPACE}" --create-namespace --wait --timeout 120s
echo "  Helm release installed. Deployment has app.kubernetes.io/managed-by: Helm label."
kubectl get pods -n "${NAMESPACE}"
echo ""

# Step 2: Deploy Prometheus alerting rules (outside Helm to keep it simple)
echo "==> Step 2: Deploying CrashLoop detection alerting rule..."
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Step 3: Baseline
echo "==> Step 3: Establishing healthy baseline (20s)..."
sleep 20
echo "  Baseline established."
echo ""

# Step 4: Inject
echo "==> Step 4: Injecting invalid nginx config via helm upgrade..."
bash "${SCRIPT_DIR}/inject-bad-config.sh"
echo ""

# Step 5: Monitor
echo "==> Step 5: Waiting for CrashLoop alert to fire (~2-3 min)..."
echo "  Check Prometheus: kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090"
echo ""
echo "==> Step 6: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,sp,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "  Expected flow:"
echo "    Alert -> Gateway -> SP -> AA (HAPI)"
echo "    LLM detects helmManaged=true, selects HelmRollback workflow"
echo "    WE runs helm rollback to previous working revision"
echo "    EM verifies pods are running"
echo ""
echo "==> Verify remediation:"
echo "    helm history demo-crashloop-helm -n ${NAMESPACE}"
echo "    kubectl get pods -n ${NAMESPACE}"
