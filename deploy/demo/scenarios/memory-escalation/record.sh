#!/usr/bin/env bash
# Three-tape coordinator for the Memory Escalation (Multi-Cycle) demo.
#
# CUSTOM coordinator — does NOT source record-scenario.sh.
# Handles 2 RR cycles:
#   Cycle 1: OOM detected → RR created → AwaitingApproval → approved →
#             workflow increases memory limits → Completed
#   Cycle 2: OOM happens AGAIN → new RR created → AI detects consecutive
#             failures → ManualReviewRequired (escalation)
#
# Orchestrates:
#   1. Setup — cleanup + apply manifests + wait for deployment
#   2. Start RR watcher (background)
#   3. Start pod watcher (background)
#   4. (No injection — OOM is built into the workload)
#   5. Wait for ContainerOOMKilling alert + capture data
#   6. Wait for AwaitingApproval (Cycle 1) + approve
#   7. Wait for first RR to reach Completed (Cycle 1 done)
#   8. Wait for SECOND RR to appear (Cycle 2 starts)
#   9. Wait for second RR to reach ManualReviewRequired (escalation)
#  10. Wait stabilization, stop both watcher tapes
#  11. Record screens tape
#  12. Cleanup
#
# Usage: bash deploy/demo/scenarios/memory-escalation/record.sh
set -euo pipefail

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCENARIO_DIR}/../../../.." && pwd)"
PLATFORM_NS="${PLATFORM_NS:-kubernaut-system}"
export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/kubernaut-demo-config}"
export PLATFORM_NS

ALERT_CACHE="${SCENARIO_DIR}/.alert-cache.json"
RESOURCE_STOP_FILE="/tmp/kubernaut-memory-escalation-resource-stop"
RR_STOP_FILE="/tmp/kubernaut-memory-escalation-rr-stop"
rm -f "${RESOURCE_STOP_FILE}" "${RR_STOP_FILE}"

_step=0
step() {
  _step=$((_step + 1))
  echo "==> [${_step}] $1"
}

echo "════════════════════════════════════════════"
echo "  Memory Escalation (Multi-Cycle) — Three-Tape Recording"
echo "════════════════════════════════════════════"
echo ""

# ── Step 1: Setup ─────────────────────────────────────
step "Running setup (cleanup + apply manifests + wait for deployment)..."
bash "${SCENARIO_DIR}/cleanup.sh" 2>/dev/null || true

# Shorten stabilization window for faster multi-cycle demo
kubectl get configmap remediationorchestrator-config -n "${PLATFORM_NS}" -o yaml \
  | sed 's/stabilizationWindow: "5m"/stabilizationWindow: "240s"/' \
  | kubectl apply -f - >/dev/null 2>&1 || true
kubectl rollout restart deploy/remediationorchestrator-controller -n "${PLATFORM_NS}" >/dev/null 2>&1 || true
kubectl rollout status deploy/remediationorchestrator-controller -n "${PLATFORM_NS}" --timeout=120s >/dev/null 2>&1 || true

kubectl apply -f "${SCENARIO_DIR}/manifests/namespace.yaml" >/dev/null 2>&1
kubectl apply -f "${SCENARIO_DIR}/manifests/" >/dev/null 2>&1
kubectl wait --for=condition=Available deployment/ml-worker -n demo-memory-escalation --timeout=180s >/dev/null 2>&1
sleep 20
echo "    Setup complete."

# ── Step 2: Start RR watcher (Tape B) ────────────────
step "Starting RR watcher tape (background)..."
vhs "${SCENARIO_DIR}/memory-escalation-rr.tape" &
RR_PID=$!
sleep 5

# ── Step 3: Start pod watcher (Tape A) ───────────────
step "Starting pod watcher tape (background)..."
vhs "${SCENARIO_DIR}/memory-escalation-pods.tape" &
RESOURCE_PID=$!
sleep 5

# ── Step 4: No injection ─────────────────────────────
echo "    (No injection — OOM is built into the workload)"

# ── Step 5: Wait for alert + capture data ─────────────
step "Waiting for ContainerOOMKilling alert to fire..."
ALERT_WAIT=0
while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 \
  -- amtool alert query alertname=ContainerOOMKilling \
  --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null \
  | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do
  sleep 10
  ALERT_WAIT=$((ALERT_WAIT + 10))
  echo "    Still waiting... (${ALERT_WAIT}s)"
done
echo "    Alert fired! Capturing alert data..."
kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 \
  -- amtool alert query alertname=ContainerOOMKilling \
  --alertmanager.url=http://localhost:9093 --output=json > "${ALERT_CACHE}" 2>/dev/null || true

# ── Step 6: Wait for AwaitingApproval (Cycle 1) + approve ─
step "Waiting for RR to reach AwaitingApproval (Cycle 1)..."
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

# ── Step 7: Wait for first RR to reach Completed (Cycle 1 done) ─
step "Waiting for first RR to reach Completed (Cycle 1)..."
CYCLE1_WAIT=0
while ! kubectl get remediationrequests -n "${PLATFORM_NS}" 2>/dev/null | grep -q Completed; do
  sleep 5
  CYCLE1_WAIT=$((CYCLE1_WAIT + 5))
  echo "    Still waiting for Completed... (${CYCLE1_WAIT}s)"
done
echo "    Cycle 1 complete — first RR reached Completed."

# ── Step 8: Wait for SECOND RR to appear (Cycle 2 starts) ─
step "Waiting for second RR to appear (Cycle 2)..."
RR_COUNT=1
CYCLE2_WAIT=0
while [ "$(kubectl get remediationrequests -n "${PLATFORM_NS}" -o name 2>/dev/null | wc -l | tr -d ' ')" -le 1 ]; do
  sleep 5
  CYCLE2_WAIT=$((CYCLE2_WAIT + 5))
  echo "    Still waiting for second RR... (${CYCLE2_WAIT}s)"
done
echo "    Second RR created — Cycle 2 started."

# ── Step 9: Wait for second RR to reach ManualReviewRequired (escalation) ─
step "Waiting for second RR to reach ManualReviewRequired (escalation)..."
MRR_WAIT=0
while ! kubectl get remediationrequests -n "${PLATFORM_NS}" 2>/dev/null | grep -q ManualReviewRequired; do
  sleep 5
  MRR_WAIT=$((MRR_WAIT + 5))
  echo "    Still waiting for ManualReviewRequired... (${MRR_WAIT}s)"
done
echo "    Second RR reached ManualReviewRequired — escalation complete."

# ── Step 10: Wait stabilization, stop both watcher tapes ─
step "Waiting 15s for resources to stabilize, then stopping watcher tapes..."
sleep 15
echo "    Signaling both watcher tapes to stop..."
touch "${RR_STOP_FILE}" "${RESOURCE_STOP_FILE}"
echo "    Waiting for VHS processes to finish encoding..."
wait $RR_PID 2>/dev/null || true
echo "    RR watcher tape finished."
wait $RESOURCE_PID 2>/dev/null || true
echo "    Pod watcher tape finished."
rm -f "${RR_STOP_FILE}" "${RESOURCE_STOP_FILE}"

# ── Step 11: Record screens tape (sequential) ─────────
step "Recording narration screens tape..."
export ALERT_CACHE
vhs "${SCENARIO_DIR}/memory-escalation-screens.tape"
echo "    Screens tape finished."

# ── Step 12: Cleanup ──────────────────────────────────
step "Cleaning up scenario resources..."
bash "${SCENARIO_DIR}/cleanup.sh" 2>/dev/null || true

echo ""
echo "════════════════════════════════════════════"
echo "  Recording complete!"
echo ""
echo "  Raw files:"
echo "    ${SCENARIO_DIR}/memory-escalation-pods-raw.mp4"
echo "    ${SCENARIO_DIR}/memory-escalation-rr-raw.mp4"
echo "    ${SCENARIO_DIR}/memory-escalation-screens-raw.mp4"
echo ""
echo "  Next: bash deploy/demo/scripts/splice-demo.sh memory-escalation"
echo "════════════════════════════════════════════"
