#!/usr/bin/env bash
# Validate concurrent-cross-namespace scenario (#172) pipeline outcome.
# Two namespaces with different risk tolerance -> different workflow selections.
#
# Called by run-scenario.sh or standalone:
#   ./deploy/demo/scenarios/concurrent-cross-namespace/validate.sh [--auto-approve] [--no-color]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NS_ALPHA="demo-team-alpha"
NS_BETA="demo-team-beta"
APPROVE_MODE="${1:---auto-approve}"

# shellcheck source=../../scripts/validation-helper.sh
source "${SCRIPT_DIR}/../../scripts/validation-helper.sh"

# ── Wait for alerts ──────────────────────────────────────────────────────────

wait_for_alert "KubePodCrashLooping" "${NS_ALPHA}" 300
show_alert "KubePodCrashLooping" "${NS_ALPHA}"

# ── Wait for both pipelines ──────────────────────────────────────────────────

log_phase "Waiting for Team Alpha pipeline..."
wait_for_rr "${NS_ALPHA}" 120
poll_pipeline "${NS_ALPHA}" 600 "${APPROVE_MODE}"

log_phase "Waiting for Team Beta pipeline..."
wait_for_rr "${NS_BETA}" 120
poll_pipeline "${NS_BETA}" 600 "${APPROVE_MODE}"

# ── Assertions ──────────────────────────────────────────────────────────────

log_phase "Running assertions..."

alpha_phase=$(get_rr_phase "${NS_ALPHA}")
assert_eq "$alpha_phase" "Completed" "Alpha RR phase"

alpha_outcome=$(get_rr_outcome "${NS_ALPHA}")
assert_eq "$alpha_outcome" "Remediated" "Alpha RR outcome"

beta_phase=$(get_rr_phase "${NS_BETA}")
assert_eq "$beta_phase" "Completed" "Beta RR phase"

beta_outcome=$(get_rr_outcome "${NS_BETA}")
assert_eq "$beta_outcome" "Remediated" "Beta RR outcome"

alpha_rr=$(get_rr_name "${NS_ALPHA}")
alpha_workflow=$(kubectl get aianalyses "ai-${alpha_rr}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.selectedWorkflow.workflowId}' 2>/dev/null || echo "")

beta_rr=$(get_rr_name "${NS_BETA}")
beta_workflow=$(kubectl get aianalyses "ai-${beta_rr}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.selectedWorkflow.workflowId}' 2>/dev/null || echo "")

assert_neq "$alpha_workflow" "" "Alpha AA selected a workflow"
assert_neq "$beta_workflow" "" "Beta AA selected a workflow"
assert_neq "$alpha_workflow" "$beta_workflow" "Different workflows selected (risk-based)"

log_info "Alpha workflow: ${alpha_workflow}"
log_info "Beta workflow:  ${beta_workflow}"

print_result "concurrent-cross-namespace"
