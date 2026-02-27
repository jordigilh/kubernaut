#!/usr/bin/env bash
# Remediation Retry Demo -- Automated Runner
# Scenario #167: First remedy fails (restart) -> second remedy succeeds (rollback)
#
# Demonstrates how the platform escalates through workflow options:
# Cycle 1: restart-pods-v1 is the only available workflow -> restart cannot fix bad config -> WE fails
# Cycle 2: crashloop-rollback-v1 is also now available -> LLM selects rollback -> success
#
# Prerequisites:
#   - Kind cluster with Kubernaut platform deployed
#   - Prometheus with kube-state-metrics
#
# Usage: ./deploy/demo/scenarios/remediation-retry/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-remediation-retry"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/../kind-config-singlenode.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform

echo "============================================="
echo " Remediation Retry Demo (#167)"
echo " Cycle 1: Restart (fail) -> Cycle 2: Rollback (success)"
echo "============================================="
echo ""

# --- Cycle 1: Seed only restart-pods-v1 ---
echo "==============================="
echo " CYCLE 1: restart-pods-v1 only"
echo "==============================="
echo ""

echo "==> Seeding restart-pods-v1 workflow (the ONLY available workflow)..."
seed_scenario_workflow "remediation-retry"

echo "==> Step 1: Deploying namespace, healthy config, and worker..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/configmap.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

echo "==> Step 2: Waiting for healthy deployment..."
kubectl wait --for=condition=Available deployment/worker \
  -n "${NAMESPACE}" --timeout=120s
echo "  Worker is running with valid configuration."
echo ""

echo "==> Step 3: Establishing healthy baseline (20s)..."
sleep 20
echo ""

echo "==> Step 4: Injecting bad config (triggers CrashLoopBackOff)..."
bash "${SCRIPT_DIR}/inject-bad-config.sh"
echo ""

echo "==> Step 5: Waiting for CrashLoop alert and pipeline to process (~3-5 min)..."
echo "  The LLM will select restart-pods-v1 (the only option)."
echo "  A rolling restart cannot fix the bad config -> WE will FAIL."
echo ""
echo "  Monitor: kubectl get rr,aa,we -n ${NAMESPACE} -w"
echo ""
echo "  Waiting for pipeline cycle 1 to complete..."
echo "  (Press Ctrl+C to skip waiting and proceed to Cycle 2)"
echo ""

# Wait for the first RR to complete (fail)
for i in $(seq 1 60); do
  PHASE=$(kubectl get rr -n "${NAMESPACE}" -o jsonpath='{.items[0].status.overallPhase}' 2>/dev/null || echo "")
  if [ "${PHASE}" = "Completed" ] || [ "${PHASE}" = "Failed" ]; then
    echo "  Cycle 1 RR reached phase: ${PHASE}"
    break
  fi
  sleep 10
done
echo ""

# --- Cycle 2: Also seed crashloop-rollback-v1 ---
echo "==============================="
echo " CYCLE 2: + crashloop-rollback-v1"
echo "==============================="
echo ""

echo "==> Seeding crashloop-rollback-v1 workflow (now two options available)..."
seed_scenario_workflow "crashloop"

echo "==> The same CrashLoopBackOff alert will re-fire (pods are still crashing)."
echo "  The LLM now has two workflows to choose from:"
echo "    1. restart-pods-v1 (already failed -> lower confidence)"
echo "    2. crashloop-rollback-v1 (rollback to previous revision)"
echo "  Expected: LLM selects rollback -> WE succeeds."
echo ""
echo "  Monitor: kubectl get rr,aa,we -n ${NAMESPACE} -w"
echo ""

echo "==> To verify remediation succeeded:"
echo "    kubectl get rr -n ${NAMESPACE}"
echo "    # Should show: 1st RR failed (restart), 2nd RR succeeded (rollback)"
echo "    kubectl get pods -n ${NAMESPACE}"
echo "    # All pods should be Running/Ready with no recent restarts"
