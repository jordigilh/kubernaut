#!/usr/bin/env bash
# Validate cert-failure-gitops scenario (#160) pipeline outcome.
# Called by run-scenario.sh or standalone:
#   ./deploy/demo/scenarios/cert-failure-gitops/validate.sh [--auto-approve] [--no-color]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-cert-gitops"
APPROVE_MODE="${1:---auto-approve}"

# shellcheck source=../../scripts/validation-helper.sh
source "${SCRIPT_DIR}/../../scripts/validation-helper.sh"

# ── Clean stale blocked duplicates ──────────────────────────────────────────

for rr in $(kubectl get rr -n "${PLATFORM_NS}" -o jsonpath='{range .items[*]}{.metadata.name}={.status.overallPhase}={.spec.signalLabels.namespace}{"\n"}{end}' 2>/dev/null | grep "=Blocked=${NAMESPACE}" | cut -d= -f1); do
    kubectl delete rr "$rr" -n "${PLATFORM_NS}" --wait=false 2>/dev/null || true
done

# ── Wait for alert ──────────────────────────────────────────────────────────

wait_for_alert "CertManagerCertNotReady" "${NAMESPACE}" 300
show_alert "CertManagerCertNotReady"

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
assert_eq "$action_type" "GitRevertCommit" "AA selected action type"

confidence=$(kubectl get aianalyses "${aa_name}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.selectedWorkflow.confidence}' 2>/dev/null || echo "")
assert_neq "$confidence" "" "AA confidence present"

wfe_phase=$(get_wfe_phase "${NAMESPACE}")
assert_eq "$wfe_phase" "Completed" "WFE phase"

ea_phase=$(get_ea_phase "${NAMESPACE}")
assert_eq "$ea_phase" "Completed" "EA phase"

# Verify Certificate is Ready (git revert restored valid ClusterIssuer)
cert_ready=$(kubectl get certificate demo-app-cert -n "${NAMESPACE}" \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "")
assert_eq "$cert_ready" "True" "Certificate Ready"

# Verify ArgoCD application is Synced (git revert restored correct config)
argocd_sync=$(kubectl get application demo-cert-gitops -n argocd \
  -o jsonpath='{.status.sync.status}' 2>/dev/null || echo "")
assert_eq "$argocd_sync" "Synced" "ArgoCD application Synced"

argocd_health=$(kubectl get application demo-cert-gitops -n argocd \
  -o jsonpath='{.status.health.status}' 2>/dev/null || echo "")
assert_eq "$argocd_health" "Healthy" "ArgoCD application Healthy"

print_result "cert-failure-gitops"
