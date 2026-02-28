#!/usr/bin/env bash
# Resource Contention Demo -- Automated Runner
# Issue #231: Demonstrates external actor interference pattern
#
# Scenario: Kubernaut remediates a Deployment with OOMKill by increasing memory
# limits, but an external actor (simulating GitOps or another controller) reverts
# the spec back to the original misconfigured state. After N cycles, the RO
# detects the ineffective chain via DataStorage hash analysis (spec_drift) and
# escalates to human review.
#
# Flow:
#   1. Deploy workload with low memory limits (causes OOMKill)
#   2. Kubernaut detects alert -> creates RR -> AIA -> WFE applies fix
#   3. External actor script reverts memory limits back
#   4. OOMKill recurs -> new RR -> new cycle
#   5. After 3 cycles: CheckIneffectiveRemediationChain blocks with ManualReviewRequired
#
# Prerequisites:
#   - Kind cluster with Kubernaut platform deployed
#   - Prometheus with kube-state-metrics
#
# Usage: ./deploy/demo/scenarios/resource-contention/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-resource-contention"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/../kind-config-singlenode.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform
seed_scenario_workflow "resource-contention"

echo "============================================="
echo " Resource Contention Demo (Issue #231)"
echo " OOMKill -> Fix -> External Revert -> Repeat -> Escalate"
echo "============================================="
echo ""

echo ">> Step 1: Creating namespace and deploying workload..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

echo ">> Step 2: Starting external actor (runs in background)..."
bash "${SCRIPT_DIR}/scripts/external-actor.sh" &
EXTERNAL_ACTOR_PID=$!
trap "kill ${EXTERNAL_ACTOR_PID} 2>/dev/null || true" EXIT

echo ">> Step 3: Waiting for workload to become ready..."
kubectl -n "${NAMESPACE}" rollout status deployment/contention-app --timeout=60s || true

echo ""
echo ">> Demo is running. The following cycle will repeat:"
echo "    1. OOMKill alert fires -> Kubernaut creates RR"
echo "    2. AIA analyzes -> WFE increases memory limits"
echo "    3. External actor reverts limits back to original value"
echo "    4. OOMKill recurs"
echo "    5. After 3 cycles: RO detects ineffective chain via spec_drift"
echo "       -> Blocks with ManualReviewRequired"
echo ""
echo "==> To monitor progress:"
echo "    kubectl get rr -n ${NAMESPACE} -w"
echo ""
echo "==> To verify escalation outcome:"
echo "    kubectl get rr -n ${NAMESPACE} -o json | jq '.items[-1].status.outcome'"
echo "    # Should show 'ManualReviewRequired' after 3 cycles"
echo "    kubectl get rr -n ${NAMESPACE} -o json | jq '.items[-1].status.blockReason'"
echo "    # Should show 'IneffectiveChain'"
echo ""
echo ">> Press Ctrl+C to stop the demo"
wait
