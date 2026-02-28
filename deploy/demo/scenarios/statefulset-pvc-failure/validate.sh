#!/usr/bin/env bash
# Validate statefulset-pvc-failure scenario (#137) pipeline outcome.
# Called by run-scenario.sh or standalone:
#   ./deploy/demo/scenarios/statefulset-pvc-failure/validate.sh [--auto-approve] [--no-color]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="demo-statefulset"
APPROVE_MODE="${1:---auto-approve}"

# shellcheck source=../../scripts/validation-helper.sh
source "${SCRIPT_DIR}/../../scripts/validation-helper.sh"

# ── Clean stale blocked duplicates ──────────────────────────────────────────

for rr in $(kubectl get rr -n "${PLATFORM_NS}" -o jsonpath='{range .items[*]}{.metadata.name}={.status.overallPhase}={.spec.signalLabels.namespace}{"\n"}{end}' 2>/dev/null | grep "=Blocked=${NAMESPACE}" | cut -d= -f1); do
    kubectl delete rr "$rr" -n "${PLATFORM_NS}" --wait=false 2>/dev/null || true
done

# ── Wait for alert ──────────────────────────────────────────────────────────

wait_for_alert "KubeStatefulSetReplicasMismatch" "${NAMESPACE}" 300
show_alert "KubeStatefulSetReplicasMismatch"

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
assert_eq "$action_type" "FixStatefulSetPVC" "AA selected workflow action type"

confidence=$(kubectl get aianalyses "${aa_name}" -n "${PLATFORM_NS}" \
  -o jsonpath='{.status.selectedWorkflow.confidence}' 2>/dev/null || echo "")
assert_neq "$confidence" "" "AA confidence present"

wfe_phase=$(get_wfe_phase "${NAMESPACE}")
assert_eq "$wfe_phase" "Completed" "WFE phase"

ready_replicas=$(kubectl get statefulset kv-store -n "${NAMESPACE}" \
  -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
assert_eq "$ready_replicas" "3" "StatefulSet has 3 ready replicas"

pvc_phase=$(kubectl get pvc data-kv-store-2 -n "${NAMESPACE}" \
  -o jsonpath='{.status.phase}' 2>/dev/null || echo "")
assert_eq "$pvc_phase" "Bound" "PVC data-kv-store-2 is Bound"

pvc_sc=$(kubectl get pvc data-kv-store-2 -n "${NAMESPACE}" \
  -o jsonpath='{.spec.storageClassName}' 2>/dev/null || echo "")
assert_neq "$pvc_sc" "broken-storage-class" "PVC storageClass is not broken"

pod_status=$(kubectl get pod kv-store-2 -n "${NAMESPACE}" \
  -o jsonpath='{.status.phase}' 2>/dev/null || echo "")
assert_eq "$pod_status" "Running" "Pod kv-store-2 is Running"

print_result "statefulset-pvc-failure"
