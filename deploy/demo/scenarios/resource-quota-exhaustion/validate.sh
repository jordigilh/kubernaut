#!/usr/bin/env bash
# Validate resource-quota-exhaustion scenario (#171) pipeline outcome.
#
# This scenario has NO matching workflow for namespace-level ResourceQuota
# exhaustion. The LLM should escalate to ManualReviewRequired.
#
# Two valid paths exist:
#   Path A (1-pass): LLM directly escalates to ManualReviewRequired.
#   Path B (2-pass): LLM selects a semantically similar workflow (e.g.
#     IncreaseMemoryLimits), it fails, alert re-fires, second RR is created,
#     LLM uses remediation history to avoid repeating the mistake and escalates.
#     See #323 for the case study documenting this self-correction behavior.
#
# Called by run-scenario.sh or standalone:
#   ./deploy/demo/scenarios/resource-quota-exhaustion/validate.sh [--auto-approve] [--no-color]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-quota"
APPROVE_MODE="${1:---auto-approve}"

# shellcheck source=../../scripts/validation-helper.sh
source "${SCRIPT_DIR}/../../scripts/validation-helper.sh"

# ── Wait for alert ──────────────────────────────────────────────────────────

wait_for_alert "KubeResourceQuotaExhausted" "${NAMESPACE}" 480
show_alert "KubeResourceQuotaExhausted" "${NAMESPACE}"

# ── Pipeline: 1-or-2 pass loop ─────────────────────────────────────────────

REMEDIATION_LOOPS=0

wait_for_rr "${NAMESPACE}" 120
first_rr=$(get_rr_name "${NAMESPACE}")

if poll_pipeline "${NAMESPACE}" 600 "${APPROVE_MODE}"; then
    # Completed (Path A with ManualReviewRequired via Completed phase is unlikely,
    # but handle it)
    REMEDIATION_LOOPS=1
    DECISIVE_RR="$first_rr"
else
    # Pipeline terminated (Failed/TimedOut/Cancelled)
    REMEDIATION_LOOPS=1
    first_outcome=$(kubectl get rr "${first_rr}" -n "${PLATFORM_NS}" \
      -o jsonpath='{.status.outcome}' 2>/dev/null || echo "")

    if [ "$first_outcome" = "ManualReviewRequired" ]; then
        DECISIVE_RR="$first_rr"
    else
        log_info "First remediation outcome: ${first_outcome}. Waiting for self-correction loop (#323)..."
        REMEDIATION_LOOPS=2

        # Wait for a NEW RR (different from the first) to appear
        log_phase "Waiting for second RemediationRequest (alert re-fire)..."
        local_timeout=300
        local_elapsed=0
        while [ "$local_elapsed" -lt "$local_timeout" ]; do
            candidate=$(get_rr_name "${NAMESPACE}")
            if [ -n "$candidate" ] && [ "$candidate" != "$first_rr" ]; then
                log_success "Second RR ${candidate} created"
                break
            fi
            sleep 10
            local_elapsed=$((local_elapsed + 10))
        done

        if [ "$local_elapsed" -ge "$local_timeout" ]; then
            log_error "Timed out waiting for second RR (${local_timeout}s)"
            print_result "resource-quota-exhaustion"
            exit 1
        fi

        # Poll the second pipeline
        poll_pipeline "${NAMESPACE}" 600 "${APPROVE_MODE}" || true
        DECISIVE_RR=$(get_rr_name "${NAMESPACE}")
    fi
fi

# ── Assertions (on the decisive RR) ────────────────────────────────────────

log_phase "Running assertions..."
log_info "Remediation loops: ${REMEDIATION_LOOPS} (decisive RR: ${DECISIVE_RR})"

decisive_outcome=$(kubectl get rr "${DECISIVE_RR}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.outcome}' 2>/dev/null || echo "")
assert_eq "$decisive_outcome" "ManualReviewRequired" "RR outcome"

requires_review=$(kubectl get rr "${DECISIVE_RR}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.requiresManualReview}' 2>/dev/null || echo "")
assert_eq "$requires_review" "true" "RR requiresManualReview"

sp_name="sp-${DECISIVE_RR}"
sp_phase=$(kubectl get signalprocessings "${sp_name}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.phase}' 2>/dev/null || echo "")
assert_eq "$sp_phase" "Completed" "SP phase"

aa_name="ai-${DECISIVE_RR}"
aa_human=$(kubectl get aianalyses "${aa_name}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.needsHumanReview}' 2>/dev/null || echo "")
assert_eq "$aa_human" "true" "AA needsHumanReview"

aa_reason=$(kubectl get aianalyses "${aa_name}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.humanReviewReason}' 2>/dev/null || echo "")
assert_eq "$aa_reason" "no_matching_workflows" "AA humanReviewReason"

# Quota should still be exhausted: at least one RS has desired > ready
stuck_rs=$(kubectl get rs -n "${NAMESPACE}" --no-headers 2>/dev/null \
  | awk '$2 > $4 {count++} END {print count+0}')
assert_gt "${stuck_rs:-0}" "0" "At least 1 RS stuck (desired > ready)"

assert_in "$REMEDIATION_LOOPS" "Remediation loop count" "1" "2"

print_result "resource-quota-exhaustion"
