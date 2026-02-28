#!/usr/bin/env bash
# Validate orphaned-pvc-no-action scenario (#122) pipeline outcome.
# Called by run-scenario.sh or standalone:
#   ./deploy/demo/scenarios/orphaned-pvc-no-action/validate.sh [--auto-approve] [--no-color]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-orphaned-pvc"
APPROVE_MODE="${1:---auto-approve}"

# shellcheck source=../../scripts/validation-helper.sh
source "${SCRIPT_DIR}/../../scripts/validation-helper.sh"

# ── Wait for alert ──────────────────────────────────────────────────────────

wait_for_alert "KubePersistentVolumeClaimOrphaned" "${NAMESPACE}" 300
show_alert "KubePersistentVolumeClaimOrphaned"

# ── Wait for pipeline ──────────────────────────────────────────────────────

wait_for_rr "${NAMESPACE}" 120
poll_pipeline "${NAMESPACE}" 300 "${APPROVE_MODE}"

# ── Assertions ──────────────────────────────────────────────────────────────

log_phase "Running assertions..."

rr_phase=$(get_rr_phase "${NAMESPACE}")
assert_eq "$rr_phase" "Completed" "RR phase"

rr_outcome=$(get_rr_outcome "${NAMESPACE}")
assert_eq "$rr_outcome" "NoActionRequired" "RR outcome"

sp_phase=$(get_sp_phase "${NAMESPACE}")
assert_eq "$sp_phase" "Completed" "SP phase"

aa_phase=$(get_aa_phase "${NAMESPACE}")
assert_eq "$aa_phase" "Completed" "AA phase"

aa_actionable=$(kubectl get aianalyses "ai-$(get_rr_name "${NAMESPACE}")" \
  -n "${PLATFORM_NS}" -o jsonpath='{.status.isActionable}' 2>/dev/null || echo "")
assert_eq "${aa_actionable:-false}" "false" "AA isActionable"

wfe_phase=$(get_wfe_phase "${NAMESPACE}")
assert_eq "$wfe_phase" "" "WFE should not exist"

ea_phase=$(get_ea_phase "${NAMESPACE}")
assert_eq "$ea_phase" "" "EA should not exist"

pvc_count=$(kubectl get pvc -n "${NAMESPACE}" --no-headers 2>/dev/null | wc -l | tr -d ' ')
assert_eq "$pvc_count" "5" "Orphaned PVCs still present (not cleaned)"

print_result "orphaned-pvc-no-action"
