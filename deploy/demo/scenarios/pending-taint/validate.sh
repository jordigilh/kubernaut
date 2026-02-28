#!/usr/bin/env bash
# Validate pending-taint scenario (#122) pipeline outcome.
# Called by run-scenario.sh or standalone:
#   ./deploy/demo/scenarios/pending-taint/validate.sh [--auto-approve] [--no-color]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-taint"
APPROVE_MODE="${1:---auto-approve}"

# shellcheck source=../../scripts/validation-helper.sh
source "${SCRIPT_DIR}/../../scripts/validation-helper.sh"

# ── Clean stale blocked duplicates ──────────────────────────────────────────

for rr in $(kubectl get rr -n "${PLATFORM_NS}" -o jsonpath='{range .items[*]}{.metadata.name}={.status.overallPhase}={.spec.signalLabels.namespace}{"\n"}{end}' 2>/dev/null | grep "=Blocked=${NAMESPACE}" | cut -d= -f1); do
    kubectl delete rr "$rr" -n "${PLATFORM_NS}" --wait=false 2>/dev/null || true
done

# ── Wait for alert ──────────────────────────────────────────────────────────

wait_for_alert "KubePodNotScheduled" "${NAMESPACE}" 300
show_alert "KubePodNotScheduled"

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
assert_eq "$action_type" "RemoveTaint" "AA selected workflow action type"

confidence=$(kubectl get aianalyses "${aa_name}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.selectedWorkflow.confidence}' 2>/dev/null || echo "")
assert_neq "$confidence" "" "AA confidence present"

wfe_phase=$(get_wfe_phase "${NAMESPACE}")
assert_eq "$wfe_phase" "Completed" "WFE phase"

# Verify taint was removed from the target node
TARGET_NODE=$(kubectl get nodes -l kubernaut.ai/demo-taint-target=true -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
if [ -n "$TARGET_NODE" ]; then
  taints=$(kubectl get node "$TARGET_NODE" -o jsonpath='{.spec.taints[*].key}' 2>/dev/null || echo "")
  has_maintenance=$(echo "$taints" | grep -c "maintenance" || true)
  assert_eq "${has_maintenance:-0}" "0" "Taint 'maintenance' removed from $TARGET_NODE"
fi

# Verify pods are Running (no Pending)
pending_pods=$(kubectl get pods -n "${NAMESPACE}" --field-selector=status.phase=Pending \
  --no-headers 2>/dev/null | wc -l | tr -d ' ')
assert_eq "${pending_pods:-0}" "0" "No Pending pods in ${NAMESPACE}"

running_pods=$(kubectl get pods -n "${NAMESPACE}" --no-headers 2>/dev/null \
  | grep -c "Running" || true)
assert_gt "${running_pods:-0}" "0" "At least 1 Running pod in ${NAMESPACE}"

print_result "pending-taint"
