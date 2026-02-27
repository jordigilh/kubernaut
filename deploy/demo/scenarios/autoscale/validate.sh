#!/usr/bin/env bash
# Validate autoscale scenario (#152) pipeline outcome.
# Called by run-scenario.sh or standalone:
#   ./deploy/demo/scenarios/autoscale/validate.sh [--auto-approve] [--no-color]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-autoscale"
APPROVE_MODE="${1:---auto-approve}"

# shellcheck source=../../scripts/validation-helper.sh
source "${SCRIPT_DIR}/../../scripts/validation-helper.sh"

# ── Clean stale blocked duplicates ──────────────────────────────────────────

for rr in $(kubectl get rr -n "${PLATFORM_NS}" -o jsonpath='{range .items[*]}{.metadata.name}={.status.overallPhase}={.spec.signalLabels.namespace}{"\n"}{end}' 2>/dev/null | grep "=Blocked=${NAMESPACE}" | cut -d= -f1); do
    kubectl delete rr "$rr" -n "${PLATFORM_NS}" --wait=false 2>/dev/null || true
done

# ── Wait for alert ──────────────────────────────────────────────────────────

wait_for_alert "KubePodSchedulingFailed" "${NAMESPACE}" 300
show_alert "KubePodSchedulingFailed"

# ── Wait for pipeline ──────────────────────────────────────────────────────

wait_for_rr "${NAMESPACE}" 120
poll_pipeline "${NAMESPACE}" 600 "${APPROVE_MODE}"

# ── Assertions ──────────────────────────────────────────────────────────────

log_phase "Running assertions..."

rr_phase=$(get_rr_phase "${NAMESPACE}")
assert_eq "$rr_phase" "Completed" "RR phase"

rr_outcome=$(get_rr_outcome "${NAMESPACE}")
assert_eq "$rr_outcome" "Remediated" "RR outcome"

sp_phase=$(get_sp_phase "${NAMESPACE}")
assert_eq "$sp_phase" "Completed" "SP phase"

aa_phase=$(get_aa_phase "${NAMESPACE}")
assert_eq "$aa_phase" "Completed" "AA phase"

rr_name=$(get_rr_name "${NAMESPACE}")
aa_name="ai-${rr_name}"

action_type=$(kubectl get aianalyses "${aa_name}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.selectedWorkflow.actionType}' 2>/dev/null || echo "")
assert_eq "$action_type" "ProvisionNode" "AA selected action type"

confidence=$(kubectl get aianalyses "${aa_name}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.selectedWorkflow.confidence}' 2>/dev/null || echo "")
assert_neq "$confidence" "" "AA confidence present"

wfe_phase=$(get_wfe_phase "${NAMESPACE}")
assert_eq "$wfe_phase" "Completed" "WFE phase"

# Verify new node was provisioned (at least 2 worker nodes after provisioning)
node_count=$(kubectl get nodes --no-headers 2>/dev/null | wc -l | tr -d ' ')
assert_gt "${node_count:-0}" "1" "Cluster has >1 node (new node provisioned)"

# Verify no Pending pods remain
pending_pods=$(kubectl get pods -n "${NAMESPACE}" --no-headers 2>/dev/null \
  | grep -c "Pending" || true)
assert_eq "${pending_pods:-0}" "0" "No pods in Pending state"

healthy_pods=$(kubectl get pods -n "${NAMESPACE}" --no-headers 2>/dev/null \
  | grep -c "Running" || true)
assert_gt "${healthy_pods:-0}" "0" "At least 1 healthy Running pod"

print_result "autoscale"
