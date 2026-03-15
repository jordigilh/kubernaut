#!/usr/bin/env bash
# Record all demo scenario tapes sequentially.
# Usage: nohup bash deploy/demo/scripts/record-all-tapes.sh > /tmp/record-tapes.log 2>&1 &
#
# Each scenario: cleanup → record → log result.
# Skips scenarios that require infrastructure not yet installed (cert-manager).
# Safe to run under nohup -- survives terminal/IDE disconnects.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
cd "${REPO_ROOT}"

export KUBECONFIG=~/.kube/kubernaut-demo-config
PLATFORM_NS="kubernaut-system"
LOG="/tmp/record-tapes.log"

PASSED=0
FAILED=0
SKIPPED=0

log() { echo "[$(date '+%H:%M:%S')] $*"; }

cleanup_platform() {
  kubectl delete remediationrequests --all -n "${PLATFORM_NS}" --ignore-not-found >/dev/null 2>&1
  kubectl delete workflowexecutions --all -n "${PLATFORM_NS}" --ignore-not-found >/dev/null 2>&1
  kubectl delete effectivenessassessments --all -n "${PLATFORM_NS}" --ignore-not-found >/dev/null 2>&1
  kubectl delete aianalysis --all -n "${PLATFORM_NS}" --ignore-not-found >/dev/null 2>&1
  kubectl delete remediationapprovalrequests --all -n "${PLATFORM_NS}" --ignore-not-found >/dev/null 2>&1
  kubectl delete notificationrequests --all -n "${PLATFORM_NS}" --ignore-not-found >/dev/null 2>&1
}

record_tape() {
  local name="$1"
  local dir="deploy/demo/scenarios/${name}"
  local tape="${dir}/recording/${name}.tape"

  if [ ! -f "${tape}" ]; then
    log "SKIP ${name} -- no tape file"
    SKIPPED=$((SKIPPED + 1))
    return 0
  fi

  log "=========================================="
  log "RECORDING: ${name}"
  log "=========================================="

  log "  Cleanup: ${name}..."
  bash "${dir}/cleanup.sh" >/dev/null 2>&1 || true
  cleanup_platform
  sleep 3

  log "  Recording: ${tape}..."
  local start_time
  start_time=$(date +%s)

  if vhs "${tape}" 2>&1; then
    local elapsed=$(( $(date +%s) - start_time ))

    # Fix MP4 timing: VHS encodes long GIF pauses as single frames that the
    # MP4 encoder drops. Re-encode from the GIF to get correct playback speed.
    local gif="${dir}/recording/${name}.gif"
    local mp4="${dir}/recording/${name}.mp4"
    if [ -f "${gif}" ]; then
      log "  Fixing MP4 timing from GIF..."
      bash "${SCRIPT_DIR}/fix-mp4-timing.sh" "${gif}" "${mp4}" 2>&1 || log "  WARN: MP4 fix failed"
    fi

    log "  PASS: ${name} (${elapsed}s)"
    PASSED=$((PASSED + 1))
  else
    local elapsed=$(( $(date +%s) - start_time ))
    log "  FAIL: ${name} (${elapsed}s)"
    FAILED=$((FAILED + 1))
  fi

  log "  Post-cleanup: ${name}..."
  bash "${dir}/cleanup.sh" >/dev/null 2>&1 || true
  cleanup_platform
  sleep 5
}

log "============================================"
log " TAPE RECORDING SESSION"
log " Started: $(date)"
log "============================================"
log ""

# Order: simple scenarios first, complex later.
# Cert-manager scenarios last (may be skipped if CRDs missing).
SCENARIOS=(
  crashloop
  pending-taint
  hpa-maxed
  memory-escalation
  network-policy-block
  duplicate-alert-suppression
  statefulset-pvc-failure
  orphaned-pvc-no-action
  resource-quota-exhaustion
  pdb-deadlock
  mesh-routing-failure
  node-notready
  slo-burn
  crashloop-helm
  stuck-rollout
  autoscale
  memory-leak
  gitops-drift
  cert-failure
  cert-failure-gitops
)

log "Scenarios to record: ${#SCENARIOS[@]}"
log ""

for scenario in "${SCENARIOS[@]}"; do
  record_tape "${scenario}"
  log ""
done

log "============================================"
log " RECORDING SESSION COMPLETE"
log " Passed: ${PASSED}"
log " Failed: ${FAILED}"
log " Skipped: ${SKIPPED}"
log " Finished: $(date)"
log "============================================"
