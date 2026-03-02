#!/usr/bin/env bash
# Memory Escalation Demo -- Automated Runner
# Scenario #168: OOMKill -> increase memory limits -> OOMKill again -> escalation
#
# Demonstrates how the platform handles diminishing remediation effectiveness:
# The ml-worker consumes unbounded memory (simulating a leak). Increasing limits
# only delays the OOMKill. After consecutive failures (same workflow, same issue),
# the RO escalates to human review via CheckConsecutiveFailures (for Failed RRs)
# or CheckIneffectiveRemediationChain (Issue #214: for Completed-but-ineffective
# RRs detected via DataStorage hash chain and spec_drift analysis).
#
# Prerequisites:
#   - Kind cluster with Kubernaut platform deployed
#   - Prometheus with kube-state-metrics
#
# Usage: ./deploy/demo/scenarios/memory-escalation/run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-memory-escalation"

# shellcheck source=../../scripts/kind-helper.sh
source "${SCRIPT_DIR}/../../scripts/kind-helper.sh"
ensure_kind_cluster "${SCRIPT_DIR}/../kind-config-singlenode.yaml" "${1:-}"

# shellcheck source=../../scripts/monitoring-helper.sh
source "${SCRIPT_DIR}/../../scripts/monitoring-helper.sh"
ensure_monitoring_stack
source "${SCRIPT_DIR}/../../scripts/platform-helper.sh"
ensure_platform
seed_scenario_workflow "memory-escalation"

echo "============================================="
echo " Memory Escalation Demo (#168)"
echo " OOMKill -> Increase Limits -> Repeat -> Escalate"
echo "============================================="
echo ""

# Step 1: Deploy namespace and workload
echo "==> Step 1: Deploying namespace and ml-worker..."
kubectl apply -f "${SCRIPT_DIR}/manifests/namespace.yaml"
kubectl apply -f "${SCRIPT_DIR}/manifests/deployment.yaml"

# Step 2: Deploy Prometheus alerting rules
echo "==> Step 2: Deploying OOMKill detection alerting rule..."
kubectl apply -f "${SCRIPT_DIR}/manifests/prometheus-rule.yaml"

# Step 3: Let the container run and get OOMKilled
echo "==> Step 3: Waiting for initial OOMKill (~1-2 min)..."
echo "  The ml-worker allocates 8Mi every 2s. With 64Mi limit, OOMKill in ~16s."
echo "  After OOMKill, Prometheus detects ContainerOOMKilling alert."
echo ""

# Step 4: Monitor pipeline
echo "==> Step 4: Pipeline in progress. Monitor with:"
echo "    kubectl get rr,aa,we,ea -n ${NAMESPACE} -w"
echo ""
echo "  Expected multi-cycle flow:"
echo "    Cycle 1: OOMKill -> increase limits (64Mi -> 128Mi) -> OOMKill recurs"
echo "    Cycle 2: OOMKill -> increase limits (128Mi -> 256Mi) -> OOMKill recurs"
echo "    Cycle 3: RO blocks via CheckConsecutiveFailures (Failed RRs) or"
echo "             CheckIneffectiveRemediationChain (Completed-but-ineffective RRs)"
echo "             -> Escalates to human review (ManualReviewRequired)"
echo ""
echo "  The increase-memory-limits workflow DOES work (pods run longer), but the"
echo "  underlying memory leak means OOMKill always recurs. The platform recognizes"
echo "  the pattern and stops throwing automated remediation at it."
echo ""
echo "==> To verify escalation outcome:"
echo "    kubectl get rr -n ${NAMESPACE} -o json | jq '.items[-1].status.outcome'"
echo "    # Should show 'Blocked' or 'NeedsHumanReview' after 2-3 cycles"
echo "    kubectl get rr -n ${NAMESPACE} -o json | jq '.items[-1].status.blockingCondition'"
echo "    # Should show 'ConsecutiveFailures'"
