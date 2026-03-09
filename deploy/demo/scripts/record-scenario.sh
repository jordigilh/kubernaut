#!/usr/bin/env bash
# Shared three-tape coordinator for all demo scenarios.
#
# Orchestrates the recording of three concurrent VHS tapes:
#   Tape A — resource watcher (pods, certs, PVCs, etc.)
#   Tape B — RemediationRequest watcher
#   Tape C — narration/analysis screens (recorded sequentially after scenario completes)
#
# The coordinator controls the lifecycle of all tapes using sentinel files,
# stopping them simultaneously once the RR reaches its terminal state.
#
# Usage: source this from a scenario's record.sh after setting required variables.
#
# Required variables:
#   SCENARIO_NAME      e.g., "crashloop"
#   SCENARIO_DIR       absolute path to scenario directory
#   DEMO_NS            demo namespace, e.g., "demo-crashloop"
#   ALERT_NAME         Prometheus alert name, e.g., "KubePodCrashLooping"
#   RESOURCE_TAPE      filename of resource watcher tape, e.g., "crashloop-pods.tape"
#   SCREENS_TAPE       filename of screens tape, e.g., "crashloop-screens.tape"
#
# Optional variables (with defaults):
#   APPROVAL_REQUIRED  "true" or "false" (default: "false")
#   TERMINAL_STATE     "Completed" or "ManualReviewRequired" (default: "Completed")
#   INJECT_CMD         command to inject fault (default: empty = skip injection)
#   SETUP_CMD          setup command (default: "bash ${SCENARIO_DIR}/tape-setup.sh")
#   CLEANUP_CMD        cleanup command (default: "bash ${SCENARIO_DIR}/cleanup.sh")
#   ALERT_QUERY_NS     namespace filter for alert query (default: empty)
#   STABILIZE_WAIT     seconds after terminal state before stopping tapes (default: 15)
#   SCENARIO_TITLE     display title for the recording banner (default: SCENARIO_NAME)
#   PRE_SCREENS_HOOK   command to run before screens tape (default: empty)

set -euo pipefail

: "${SCENARIO_NAME:?SCENARIO_NAME is required}"
: "${SCENARIO_DIR:?SCENARIO_DIR is required}"
: "${DEMO_NS:?DEMO_NS is required}"
: "${ALERT_NAME:?ALERT_NAME is required}"
: "${RESOURCE_TAPE:?RESOURCE_TAPE is required}"
: "${SCREENS_TAPE:?SCREENS_TAPE is required}"

REPO_ROOT="$(cd "${SCENARIO_DIR}/../../../.." && pwd)"
PLATFORM_NS="${PLATFORM_NS:-kubernaut-system}"
export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/kubernaut-demo-config}"
export PLATFORM_NS

APPROVAL_REQUIRED="${APPROVAL_REQUIRED:-false}"
TERMINAL_STATE="${TERMINAL_STATE:-Completed}"
INJECT_CMD="${INJECT_CMD:-}"
SETUP_CMD="${SETUP_CMD:-bash ${SCENARIO_DIR}/tape-setup.sh}"
CLEANUP_CMD="${CLEANUP_CMD:-bash ${SCENARIO_DIR}/cleanup.sh}"
ALERT_QUERY_NS="${ALERT_QUERY_NS:-}"
STABILIZE_WAIT="${STABILIZE_WAIT:-15}"
SCENARIO_TITLE="${SCENARIO_TITLE:-${SCENARIO_NAME}}"
PRE_SCREENS_HOOK="${PRE_SCREENS_HOOK:-}"

RR_TAPE="${SCENARIO_NAME}-rr.tape"
ALERT_CACHE="${SCENARIO_DIR}/.alert-cache.json"
RESOURCE_STOP_FILE="/tmp/kubernaut-${SCENARIO_NAME}-resource-stop"
RR_STOP_FILE="/tmp/kubernaut-${SCENARIO_NAME}-rr-stop"
rm -f "${RESOURCE_STOP_FILE}" "${RR_STOP_FILE}"

_step=0
step() {
  _step=$((_step + 1))
  echo "==> [${_step}] $1"
}

echo "════════════════════════════════════════════"
echo "  ${SCENARIO_TITLE} — Three-Tape Recording"
echo "════════════════════════════════════════════"
echo ""

# ── Setup ─────────────────────────────────────
step "Running setup..."
eval "${SETUP_CMD}"
echo "    Setup complete."

# ── Start RR watcher (Tape B) ────────────────
step "Starting RR watcher tape (background)..."
vhs "${SCENARIO_DIR}/${RR_TAPE}" &
RR_PID=$!
sleep 5

# ── Start resource watcher (Tape A) ──────────
step "Starting resource watcher tape (background)..."
vhs "${SCENARIO_DIR}/${RESOURCE_TAPE}" &
RESOURCE_PID=$!
sleep 5

# ── Fault injection ──────────────────────────
if [ -n "${INJECT_CMD}" ]; then
  step "Injecting fault..."
  eval "${INJECT_CMD}"
  echo "    Fault injected."
else
  echo "    (No injection — fault is built into the workload)"
fi

# ── Wait for alert + capture data ─────────────
step "Waiting for ${ALERT_NAME} alert to fire..."
ALERT_WAIT=0
ALERT_QUERY="alertname=${ALERT_NAME}"
if [ -n "${ALERT_QUERY_NS}" ]; then
  ALERT_QUERY="${ALERT_QUERY},namespace=${ALERT_QUERY_NS}"
fi
while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 \
  -- amtool alert query "${ALERT_QUERY}" \
  --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null \
  | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do
  sleep 10
  ALERT_WAIT=$((ALERT_WAIT + 10))
  echo "    Still waiting... (${ALERT_WAIT}s)"
done
echo "    Alert fired! Capturing alert data..."
kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 \
  -- amtool alert query "${ALERT_QUERY}" \
  --alertmanager.url=http://localhost:9093 --output=json > "${ALERT_CACHE}" 2>/dev/null || true

# ── Approval (conditional) ────────────────────
if [ "${APPROVAL_REQUIRED}" = "true" ]; then
  step "Waiting for RR to reach AwaitingApproval..."
  AA_WAIT=0
  while ! kubectl get remediationrequests -n "${PLATFORM_NS}" 2>/dev/null | grep -q AwaitingApproval; do
    sleep 5
    AA_WAIT=$((AA_WAIT + 5))
    echo "    Still waiting... (${AA_WAIT}s)"
  done
  echo "    RR is AwaitingApproval. Approving RAR in 3s..."
  sleep 3
  bash "${REPO_ROOT}/deploy/demo/scripts/approve-rar.sh"
  echo "    RAR approved."
fi

# ── Wait for terminal state, then stop tapes ──
step "Waiting for RR to reach ${TERMINAL_STATE}..."
echo "    RR watcher PID: ${RR_PID}, Resource watcher PID: ${RESOURCE_PID}"
TERM_WAIT=0
while ! kubectl get remediationrequests -n "${PLATFORM_NS}" 2>/dev/null | grep -q "${TERMINAL_STATE}"; do
  sleep 5
  TERM_WAIT=$((TERM_WAIT + 5))
  echo "    Still waiting for ${TERMINAL_STATE}... (${TERM_WAIT}s)"
done
echo "    RR reached ${TERMINAL_STATE}! Waiting ${STABILIZE_WAIT}s for resources to stabilize..."
sleep "${STABILIZE_WAIT}"
echo "    Signaling both watcher tapes to stop..."
touch "${RR_STOP_FILE}" "${RESOURCE_STOP_FILE}"
echo "    Waiting for VHS processes to finish encoding..."
wait $RR_PID 2>/dev/null || true
echo "    RR watcher tape finished."
wait $RESOURCE_PID 2>/dev/null || true
echo "    Resource watcher tape finished."
rm -f "${RR_STOP_FILE}" "${RESOURCE_STOP_FILE}"

# ── Pre-screens hook (optional) ───────────────
if [ -n "${PRE_SCREENS_HOOK}" ]; then
  eval "${PRE_SCREENS_HOOK}"
fi

# ── Record screens tape (sequential) ─────────
step "Recording narration screens tape..."
export ALERT_CACHE
vhs "${SCENARIO_DIR}/${SCREENS_TAPE}"
echo "    Screens tape finished."

# ── Cleanup ──────────────────────────────────
step "Cleaning up scenario resources..."
eval "${CLEANUP_CMD}" 2>/dev/null || true

echo ""
echo "════════════════════════════════════════════"
echo "  Recording complete!"
echo ""
echo "  Raw files:"
for f in "${SCENARIO_DIR}"/*-raw.mp4; do
  [ -f "$f" ] && echo "    $f"
done
echo ""
echo "  Next: bash deploy/demo/scripts/splice-demo.sh ${SCENARIO_NAME}"
echo "════════════════════════════════════════════"
