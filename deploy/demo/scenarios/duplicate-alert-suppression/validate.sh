#!/usr/bin/env bash
# Validate duplicate-alert-suppression scenario pipeline outcome.
# Tests BR-DEDUP-001: 5 alerts from same Deployment → 1 RR (dedup).
#
# NOTE: Currently BLOCKED by #209 (circular duplicate blocking deadlock).
# This script validates the expected behavior once #209 is fixed.
#
# Called by run-scenario.sh or standalone:
#   ./deploy/demo/scenarios/duplicate-alert-suppression/validate.sh [--auto-approve] [--no-color]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-alert-storm"
APPROVE_MODE="${1:---auto-approve}"

# shellcheck source=../../scripts/validation-helper.sh
source "${SCRIPT_DIR}/../../scripts/validation-helper.sh"

# ── Wait for alert ──────────────────────────────────────────────────────────
# 5 pods crash → 5 KubePodCrashLooping alerts expected

wait_for_alert "KubePodCrashLooping" "${NAMESPACE}" 300
show_alert "KubePodCrashLooping"

# ── Wait for pipeline ──────────────────────────────────────────────────────

wait_for_rr "${NAMESPACE}" 120
poll_pipeline "${NAMESPACE}" 600 "${APPROVE_MODE}"

# ── Assertions ──────────────────────────────────────────────────────────────

log_phase "Running assertions..."

# Core dedup assertion: exactly 1 non-blocked RR for this namespace
active_rr_count=$(kubectl get rr -n "${PLATFORM_NS}" \
  -o jsonpath='{range .items[*]}{.status.overallPhase}={.spec.signalLabels.namespace}{"\n"}{end}' 2>/dev/null \
  | grep -v "^Blocked=" | grep "=${NAMESPACE}$" | wc -l | tr -d ' ')
assert_eq "$active_rr_count" "1" "Exactly 1 active (non-blocked) RR"

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
assert_eq "$action_type" "RollbackDeployment" "AA selected workflow"

wfe_phase=$(get_wfe_phase "${NAMESPACE}")
assert_eq "$wfe_phase" "Completed" "WFE phase"

# After rollback, pods should recover
healthy_pods=$(kubectl get pods -n "${NAMESPACE}" --no-headers 2>/dev/null \
  | grep -c "Running" || true)
assert_gt "${healthy_pods:-0}" "0" "At least 1 healthy Running pod after rollback"

# Dedup: blocked RRs should reference the active one as duplicateOf
blocked_count=$(kubectl get rr -n "${PLATFORM_NS}" \
  -o jsonpath='{range .items[*]}{.status.overallPhase}={.spec.signalLabels.namespace}{"\n"}{end}' 2>/dev/null \
  | grep "^Blocked=" | grep "=${NAMESPACE}$" | wc -l | tr -d ' ')
log_phase "Blocked duplicate RRs: ${blocked_count}"

print_result "duplicate-alert-suppression"
