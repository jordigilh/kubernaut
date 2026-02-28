#!/usr/bin/env bash
# Validate network-policy-block scenario (#138) pipeline outcome.
# Called by run-scenario.sh or standalone:
#   ./deploy/demo/scenarios/network-policy-block/validate.sh [--auto-approve] [--no-color]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-netpol"
APPROVE_MODE="${1:---auto-approve}"

# shellcheck source=../../scripts/validation-helper.sh
source "${SCRIPT_DIR}/../../scripts/validation-helper.sh"

# ── Clean stale blocked duplicates ──────────────────────────────────────────

for rr in $(kubectl get rr -n "${PLATFORM_NS}" -o jsonpath='{range .items[*]}{.metadata.name}={.status.overallPhase}={.spec.signalLabels.namespace}{"\n"}{end}' 2>/dev/null | grep "=Blocked=${NAMESPACE}" | cut -d= -f1); do
    kubectl delete rr "$rr" -n "${PLATFORM_NS}" --wait=false 2>/dev/null || true
done

# ── Wait for alert ──────────────────────────────────────────────────────────

wait_for_alert "KubePodCrashLooping" "${NAMESPACE}" 300

show_alert "KubePodCrashLooping"

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
assert_eq "$action_type" "FixNetworkPolicy" "AA selected workflow action type"

confidence=$(kubectl get aianalyses "${aa_name}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.selectedWorkflow.confidence}' 2>/dev/null || echo "")
assert_neq "$confidence" "" "AA confidence present"

wfe_phase=$(get_wfe_phase "${NAMESPACE}")
assert_eq "$wfe_phase" "Completed" "WFE phase"

ea_phase=$(get_ea_phase "${NAMESPACE}")
assert_eq "$ea_phase" "Completed" "EA phase"

# Verify deny-all NetworkPolicy was removed
netpol_count=$(kubectl get networkpolicies -n "${NAMESPACE}" \
  -l injected-by=kubernaut-demo --no-headers 2>/dev/null | wc -l | tr -d ' ')
assert_eq "${netpol_count:-0}" "0" "Deny-all NetworkPolicy removed"

# Verify pods are Running (not CrashLooping indefinitely)
ready_pods=$(kubectl get pods -n "${NAMESPACE}" -l app=web-frontend \
  --no-headers 2>/dev/null | grep -c "Running" || true)
assert_gt "${ready_pods:-0}" "0" "At least 1 web-frontend pod Running"

print_result "network-policy-block"
