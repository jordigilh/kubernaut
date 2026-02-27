#!/usr/bin/env bash
# Concurrent Cross-Namespace Demo -- Automated Runner
# Scenario #172: Two teams, same issue, different risk tolerance -> different workflows
#
# Team Alpha (high risk tolerance) -> restart-pods-v1 (simpler, faster)
# Team Beta  (low risk tolerance)  -> crashloop-rollback-v1 (safer, more thorough)
#
# This scenario also fixes the SP Rego custom labels policy bug (package name mismatch).
#
# Prerequisites:
#   - Kind cluster with Kubernaut platform deployed
#   - Prometheus with kube-state-metrics
#
# Usage: ./deploy/demo/scenarios/concurrent-cross-namespace/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/../kind-config-singlenode.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform

echo "============================================="
echo " Concurrent Cross-Namespace Demo (#172)"
echo " Same Issue, Different Risk -> Different Workflows"
echo "============================================="
echo ""

# Step 0: Fix the SP Rego custom labels policy
echo "==> Step 0: Patching SP custom labels Rego policy (fix package name)..."
kubectl create configmap signalprocessing-customlabels-policy \
  --from-file=customlabels.rego="${SCRIPT_DIR}/rego/risk-tolerance.rego" \
  -n kubernaut-system \
  --dry-run=client -o yaml | kubectl apply -f -

echo "  Restarting SignalProcessing controller to pick up policy change..."
kubectl rollout restart deployment/signalprocessing-controller -n kubernaut-system
kubectl rollout status deployment/signalprocessing-controller -n kubernaut-system --timeout=60s
echo ""

# Step 1: Seed both workflows
echo "==> Step 1: Seeding workflows..."
echo "  restart-pods-v1 (for high risk-tolerance teams)..."
seed_scenario_workflow "remediation-retry"
echo "  crashloop-rollback-v1 (for low risk-tolerance teams)..."
seed_scenario_workflow "crashloop"
echo ""

# Step 2: Deploy both namespaces and workloads
echo "==> Step 2: Deploying team-alpha and team-beta workloads..."
for team in team-alpha team-beta; do
  echo "  Deploying ${team}..."
  kubectl apply -f "${SCRIPT_DIR}/manifests/${team}/namespace.yaml"
  kubectl apply -f "${SCRIPT_DIR}/manifests/${team}/configmap.yaml"
  kubectl apply -f "${SCRIPT_DIR}/manifests/${team}/deployment.yaml"
  kubectl apply -f "${SCRIPT_DIR}/manifests/${team}/prometheus-rule.yaml"
done
echo ""

# Step 3: Wait for healthy deployments
echo "==> Step 3: Waiting for both deployments to be healthy..."
kubectl wait --for=condition=Available deployment/worker -n demo-team-alpha --timeout=120s
kubectl wait --for=condition=Available deployment/worker -n demo-team-beta --timeout=120s
echo "  Both teams running."
echo ""

# Step 4: Establish baseline
echo "==> Step 4: Establishing healthy baseline (20s)..."
sleep 20
echo ""

# Step 5: Inject bad config into BOTH namespaces
echo "==> Step 5: Injecting bad config into both namespaces simultaneously..."
bash "${SCRIPT_DIR}/inject-both.sh"
echo ""

# Step 6: Monitor
echo "==> Step 6: Both pipelines running in parallel. Monitor with:"
echo "    kubectl get rr,aa,we -n demo-team-alpha -w"
echo "    kubectl get rr,aa,we -n demo-team-beta -w"
echo ""
echo "  Expected:"
echo "    Team Alpha (high risk tolerance):"
echo "      -> SP enriches with customLabels: {risk_tolerance: [high]}"
echo "      -> DataStorage boosts restart-pods-v1 (customLabels match)"
echo "      -> LLM selects restart-pods-v1 (simpler, aligns with risk tolerance)"
echo ""
echo "    Team Beta (low risk tolerance):"
echo "      -> SP enriches with customLabels: {risk_tolerance: [low]}"
echo "      -> DataStorage boosts crashloop-rollback-v1 (customLabels match)"
echo "      -> LLM selects crashloop-rollback-v1 (safer, more thorough)"
echo ""
echo "==> To verify:"
echo "    kubectl get aa -n demo-team-alpha -o jsonpath='{.items[0].status.selectedWorkflow.workflowId}'"
echo "    # Expected: restart-pods-v1"
echo "    kubectl get aa -n demo-team-beta -o jsonpath='{.items[0].status.selectedWorkflow.workflowId}'"
echo "    # Expected: crashloop-rollback-v1"
