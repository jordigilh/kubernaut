#!/usr/bin/env bash
# Validate memory-limits-gitops-ansible scenario (#312) pipeline outcome.
# OOMKill -> AI Analysis selects IncreaseMemoryLimits -> AWX playbook updates
# memory limits in Gitea repo -> ArgoCD syncs -> EA verifies.
#
# Called by run-scenario.sh or standalone:
#   ./deploy/demo/scenarios/memory-limits-gitops-ansible/validate.sh [--auto-approve]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-memory-gitops-ansible"
PIPELINE_TIMEOUT=900
APPROVE_MODE="${1:---auto-approve}"

# shellcheck source=../../scripts/validation-helper.sh
source "${SCRIPT_DIR}/../../scripts/validation-helper.sh"

# ── Wait for alert ──────────────────────────────────────────────────────────

wait_for_alert "ContainerOOMKilling" "${NAMESPACE}" 480
show_alert "ContainerOOMKilling" "${NAMESPACE}"

# ── Wait for RR and poll pipeline ───────────────────────────────────────────

wait_for_rr "${NAMESPACE}" 120

# Scale deployment to 0 when entering Verifying phase so the OOMKills stop
# and the alert resolves naturally within the EA verification window.
on_verifying() {
    log_phase "Scaling memory-consumer to 0 (root cause fix)..."
    kubectl scale deployment/memory-consumer -n "${NAMESPACE}" --replicas=0 2>/dev/null || true
}
ON_VERIFYING_HOOK="on_verifying"

poll_pipeline "${NAMESPACE}" "${PIPELINE_TIMEOUT}" "${APPROVE_MODE}"

# ── Assertions ──────────────────────────────────────────────────────────────

log_phase "Running assertions..."

rr_phase=$(get_rr_phase "${NAMESPACE}")
assert_eq "$rr_phase" "Completed" "RR phase"

rr_outcome=$(get_rr_outcome "${NAMESPACE}")
assert_eq "$rr_outcome" "Remediated" "RR outcome"

rr_name=$(get_rr_name "${NAMESPACE}")
aa_name="ai-${rr_name}"
action_type=$(kubectl get aianalyses "${aa_name}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.selectedWorkflow.actionType}' 2>/dev/null || echo "")
assert_eq "$action_type" "IncreaseMemoryLimits" "AA selected action type"

# Verify the workflow execution used the ansible engine
wfe_name=$(kubectl get wfe -n "${PLATFORM_NS}" \
  -o jsonpath='{range .items[*]}{.metadata.name}={.metadata.labels.kubernaut\.ai/source-namespace}{"\n"}{end}' 2>/dev/null \
  | grep "=${NAMESPACE}$" | head -1 | cut -d= -f1 || true)
if [ -n "$wfe_name" ]; then
    wfe_engine=$(kubectl get wfe "${wfe_name}" -n "${PLATFORM_NS}" \
      -o jsonpath='{.spec.workflowRef.engine}' 2>/dev/null || echo "")
    assert_eq "$wfe_engine" "ansible" "WFE engine"
fi

print_result "memory-limits-gitops-ansible"
