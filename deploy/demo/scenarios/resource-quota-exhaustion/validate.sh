#!/usr/bin/env bash
# Validate resource-quota-exhaustion scenario (#171) pipeline outcome.
# Called by run-scenario.sh or standalone:
#   ./deploy/demo/scenarios/resource-quota-exhaustion/validate.sh [--auto-approve] [--no-color]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-quota"
APPROVE_MODE="${1:---auto-approve}"

# shellcheck source=../../scripts/validation-helper.sh
source "${SCRIPT_DIR}/../../scripts/validation-helper.sh"

# ── Wait for alert ──────────────────────────────────────────────────────────
# ReplicaSet-level alert: spec_replicas>0 but status_replicas=0 (FailedCreate)

wait_for_alert "KubeResourceQuotaExhausted" "${NAMESPACE}" 180
show_alert "KubeResourceQuotaExhausted"

# ── Wait for pipeline ──────────────────────────────────────────────────────

wait_for_rr "${NAMESPACE}" 120
poll_pipeline "${NAMESPACE}" 300 "${APPROVE_MODE}"

# ── Assertions ──────────────────────────────────────────────────────────────

log_phase "Running assertions..."

rr_phase=$(get_rr_phase "${NAMESPACE}")
assert_eq "$rr_phase" "Failed" "RR phase (no workflow available)"

rr_name=$(get_rr_name "${NAMESPACE}")

requires_review=$(kubectl get rr "${rr_name}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.requiresManualReview}' 2>/dev/null || echo "")
assert_eq "$requires_review" "true" "RR requiresManualReview"

outcome=$(kubectl get rr "${rr_name}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.outcome}' 2>/dev/null || echo "")
assert_eq "$outcome" "ManualReviewRequired" "RR outcome"

sp_phase=$(get_sp_phase "${NAMESPACE}")
assert_eq "$sp_phase" "Completed" "SP phase"

# AA should exist but fail (no matching workflows)
aa_name="ai-${rr_name}"
aa_human=$(kubectl get aianalyses "${aa_name}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.needsHumanReview}' 2>/dev/null || echo "")
assert_eq "$aa_human" "true" "AA needsHumanReview"

aa_reason=$(kubectl get aianalyses "${aa_name}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.humanReviewReason}' 2>/dev/null || echo "")
assert_eq "$aa_reason" "no_matching_workflows" "AA humanReviewReason"

# No WFE should exist (no automated remediation)
wfe_phase=$(get_wfe_phase "${NAMESPACE}")
assert_eq "$wfe_phase" "" "WFE should not exist"

# Quota should still be exhausted: new RS has desired>0 but 0 current pods
stuck_rs=$(kubectl get rs -n "${NAMESPACE}" --no-headers 2>/dev/null \
  | awk '$2 > 0 && $3 == 0 {count++} END {print count+0}')
assert_gt "${stuck_rs:-0}" "0" "At least 1 RS stuck (desired>0, current=0)"

print_result "resource-quota-exhaustion"
