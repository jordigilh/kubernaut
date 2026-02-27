#!/usr/bin/env bash
# Validate remediation-retry scenario (#167) pipeline outcome.
# Two-cycle escalation: Cycle 1 (restart) fails -> Cycle 2 (rollback) succeeds.
# Called by run-scenario.sh or standalone:
#   ./deploy/demo/scenarios/remediation-retry/validate.sh [--auto-approve] [--no-color]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-remediation-retry"
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

# ── Cycle 1: First pipeline (expected to fail) ─────────────────────────────

log_phase "Cycle 1: Waiting for first RemediationRequest..."
wait_for_rr "${NAMESPACE}" 120

first_rr=$(get_rr_name "${NAMESPACE}")
log_phase "Cycle 1: RR ${first_rr} — polling pipeline (expected: fail)..."

# Allow cycle 1 to fail without exiting the script
poll_pipeline "${NAMESPACE}" 600 "${APPROVE_MODE}" || true

# ── Cycle 2: Second pipeline (expected to succeed) ─────────────────────────

log_phase "Cycle 2: Waiting for second RemediationRequest..."

second_rr=""
for _i in $(seq 1 60); do
    current_rr=$(get_rr_name "${NAMESPACE}")
    if [ -n "$current_rr" ] && [ "$current_rr" != "$first_rr" ]; then
        second_rr="$current_rr"
        log_success "Cycle 2: RR ${second_rr} created"
        break
    fi
    sleep 10
done

if [ -z "$second_rr" ]; then
    log_error "Timed out waiting for second RemediationRequest"
    # Fall through to assertions — they will fail and report
fi

poll_pipeline "${NAMESPACE}" 600 "${APPROVE_MODE}"

# ── Assertions ──────────────────────────────────────────────────────────────

log_phase "Running assertions..."

# Verify at least 2 RRs were created for this namespace
rr_count=$(kubectl get rr -n "${PLATFORM_NS}" -o jsonpath='{range .items[*]}{.spec.signalLabels.namespace}{"\n"}{end}' 2>/dev/null \
  | grep -c "${NAMESPACE}" || true)
assert_gt "${rr_count:-0}" "1" "At least 2 RRs created (retry occurred)"

# Cycle 1 assertions (first RR — expected Failed)
first_phase=$(kubectl get rr "${first_rr}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.overallPhase}' 2>/dev/null || echo "")
assert_eq "$first_phase" "Failed" "Cycle 1 RR phase (restart failed)"

# Cycle 2 assertions (latest/second RR — expected Completed)
rr_phase=$(get_rr_phase "${NAMESPACE}")
assert_eq "$rr_phase" "Completed" "Cycle 2 RR phase"

rr_outcome=$(get_rr_outcome "${NAMESPACE}")
assert_eq "$rr_outcome" "Remediated" "Cycle 2 RR outcome"

sp_phase=$(get_sp_phase "${NAMESPACE}")
assert_eq "$sp_phase" "Completed" "SP phase"

aa_phase=$(get_aa_phase "${NAMESPACE}")
assert_eq "$aa_phase" "Completed" "AA phase"

rr_name=$(get_rr_name "${NAMESPACE}")
aa_name="ai-${rr_name}"

action_type=$(kubectl get aianalyses "${aa_name}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.selectedWorkflow.actionType}' 2>/dev/null || echo "")
assert_eq "$action_type" "RollbackDeployment" "Cycle 2 AA selected action type"

wfe_phase=$(get_wfe_phase "${NAMESPACE}")
assert_eq "$wfe_phase" "Completed" "Cycle 2 WFE phase"

healthy_pods=$(kubectl get pods -n "${NAMESPACE}" --no-headers 2>/dev/null \
  | grep -c "Running" || true)
assert_gt "${healthy_pods:-0}" "0" "At least 1 healthy Running pod"

crashing_pods=$(kubectl get pods -n "${NAMESPACE}" --no-headers 2>/dev/null \
  | grep -c "CrashLoopBackOff" || true)
assert_eq "${crashing_pods:-0}" "0" "No pods in CrashLoopBackOff"

print_result "remediation-retry"
