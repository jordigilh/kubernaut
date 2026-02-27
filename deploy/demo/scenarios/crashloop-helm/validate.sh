#!/usr/bin/env bash
# Validate crashloop-helm scenario (#135) pipeline outcome.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-crashloop-helm"
APPROVE_MODE="${1:---auto-approve}"

# shellcheck source=../../scripts/validation-helper.sh
source "${SCRIPT_DIR}/../../scripts/validation-helper.sh"

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
assert_eq "$action_type" "HelmRollback" "AA selected workflow action type"

wfe_phase=$(get_wfe_phase "${NAMESPACE}")
assert_eq "$wfe_phase" "Completed" "WFE phase"

ea_phase=$(get_ea_phase "${NAMESPACE}")
assert_eq "$ea_phase" "Completed" "EA phase"

# Verify Helm was rolled back
helm_status=$(helm status demo-crashloop-helm -n "${NAMESPACE}" -o json 2>/dev/null | jq -r '.info.status')
assert_eq "$helm_status" "deployed" "Helm release status"

helm_desc=$(helm history demo-crashloop-helm -n "${NAMESPACE}" --max 1 -o json 2>/dev/null | jq -r '.[0].description')
assert_eq "$helm_desc" "Rollback to 1" "Helm last revision is a rollback"

# Verify pods are healthy
ready=$(kubectl get deployment worker -n "${NAMESPACE}" \
  -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
desired=$(kubectl get deployment worker -n "${NAMESPACE}" \
  -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "0")
assert_eq "$ready" "$desired" "All worker replicas ready"

print_result "crashloop-helm"
