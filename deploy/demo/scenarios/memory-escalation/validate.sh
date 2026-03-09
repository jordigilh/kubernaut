#!/usr/bin/env bash
# Validate memory-escalation scenario (#168) pipeline outcome.
# OOMKill -> increase limits -> OOMKill again -> escalation after N cycles.
#
# This scenario runs multiple cycles. We validate that:
# 1. The first cycle completes (Remediated with IncreaseMemoryLimits)
# 2. Eventually a Blocked/Skipped RR appears (ConsecutiveFailures escalation)
#
# Called by run-scenario.sh or standalone:
#   ./deploy/demo/scenarios/memory-escalation/validate.sh [--auto-approve] [--no-color]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-memory-escalation"
APPROVE_MODE="${1:---auto-approve}"

# shellcheck source=../../scripts/validation-helper.sh
source "${SCRIPT_DIR}/../../scripts/validation-helper.sh"

# ── Wait for alert ──────────────────────────────────────────────────────────

wait_for_alert "ContainerOOMKilling" "${NAMESPACE}" 300
show_alert "ContainerOOMKilling" "${NAMESPACE}"

# ── Wait for first cycle pipeline ──────────────────────────────────────────

wait_for_rr "${NAMESPACE}" 120
poll_pipeline "${NAMESPACE}" 600 "${APPROVE_MODE}"

# ── Assertions for first cycle ──────────────────────────────────────────────

log_phase "Running first-cycle assertions..."

rr_phase=$(get_rr_phase "${NAMESPACE}")
assert_eq "$rr_phase" "Completed" "First RR phase"

rr_outcome=$(get_rr_outcome "${NAMESPACE}")
assert_eq "$rr_outcome" "Remediated" "First RR outcome"

rr_name=$(get_rr_name "${NAMESPACE}")
aa_name="ai-${rr_name}"
action_type=$(kubectl get aianalyses "${aa_name}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.selectedWorkflow.actionType}' 2>/dev/null || echo "")
assert_eq "$action_type" "IncreaseMemoryLimits" "AA selected IncreaseMemoryLimits"

# ── Wait for escalation (subsequent cycles) ──────────────────────────────────
# The workload will OOMKill again. After 2-3 cycles, the RO should block.

log_phase "Waiting for escalation (Blocked RR after 2-3 cycles, timeout 900s)..."
ESCALATION_TIMEOUT=900
ESCALATION_ELAPSED=0

while [ "$ESCALATION_ELAPSED" -lt "$ESCALATION_TIMEOUT" ]; do
    blocked_rr=$(kubectl get rr -n "${PLATFORM_NS}" \
      -o jsonpath='{range .items[*]}{.metadata.name}={.status.overallPhase}={.spec.signalLabels.namespace}{"\n"}{end}' 2>/dev/null \
      | grep "=Blocked=${NAMESPACE}" | head -1 | cut -d= -f1 || true)

    if [ -n "$blocked_rr" ]; then
        block_reason=$(kubectl get rr "$blocked_rr" -n "${PLATFORM_NS}" \
          -o jsonpath='{.status.blockReason}' 2>/dev/null || echo "unknown")
        log_success "Escalation detected: RR ${blocked_rr} blocked (reason: ${block_reason})"
        break
    fi

    sleep 15
    ESCALATION_ELAPSED=$((ESCALATION_ELAPSED + 15))
    if [ $((ESCALATION_ELAPSED % 60)) -eq 0 ]; then
        log_phase "Still waiting for escalation... (${ESCALATION_ELAPSED}s)"
    fi
done

# ── Escalation assertions ──────────────────────────────────────────────────

log_phase "Running escalation assertions..."

blocked_count=$(kubectl get rr -n "${PLATFORM_NS}" \
  -o jsonpath='{range .items[*]}{.status.overallPhase}={.spec.signalLabels.namespace}{"\n"}{end}' 2>/dev/null \
  | grep "^Blocked=" | grep "=${NAMESPACE}$" | wc -l | tr -d ' ')
assert_gt "${blocked_count:-0}" "0" "At least 1 Blocked RR (escalation)"

total_rr=$(kubectl get rr -n "${PLATFORM_NS}" \
  -o jsonpath='{range .items[*]}{.spec.signalLabels.namespace}{"\n"}{end}' 2>/dev/null \
  | grep "^${NAMESPACE}$" | wc -l | tr -d ' ')
assert_gt "${total_rr:-0}" "1" "Multiple RRs created (multi-cycle)"

print_result "memory-escalation"
