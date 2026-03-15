#!/usr/bin/env bash
# Focused-stream coordinator for the CrashLoopBackOff demo.
#
# Orchestrates 8 tapes total:
#   • screens   – narration/analysis screens (sequential, after scenario)
#   • rr1       – RR: Created -> Analyzing
#   • rr2       – RR: Analyzing -> AwaitingApproval
#   • rr3       – RR: AwaitingApproval -> Executing -> Verifying
#   • rr4       – RR: Verifying -> Completed
#   • pods1     – Pods: Running -> CrashLoopBackOff
#   • pods2     – Pods: CrashLoopBackOff -> Running (after remediation)
#
# Each focused tape starts a fresh `kubectl get -w` and auto-terminates
# when its target phase appears (via Wait+Screen), producing clean,
# non-scrolling output ideal for splicing.
#
# Usage: bash deploy/demo/scenarios/crashloop/record.sh
set -euo pipefail

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCENARIO_DIR}/../../../.." && pwd)"
PLATFORM_NS="${PLATFORM_NS:-kubernaut-system}"
export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/kubernaut-demo-config}"
export PLATFORM_NS

ALERT_CACHE="${SCENARIO_DIR}/.alert-cache.json"
PODS2_STOP_FILE="/tmp/kubernaut-crashloop-pods2-stop"
rm -f "${PODS2_STOP_FILE}"

echo "════════════════════════════════════════════"
echo "  CrashLoopBackOff Demo — Focused Stream Recording"
echo "════════════════════════════════════════════"
echo ""

# ── Step 1: Setup ──────────────────────────────
echo "==> [1/11] Running tape-setup.sh..."
bash "${SCENARIO_DIR}/tape-setup.sh"
echo "    Setup complete."

# ── Step 2: Start pods1 + rr1 (background) ────
echo "==> [2/11] Starting focused tapes: pods1 (-> CrashLoopBackOff) + rr1 (-> Analyzing)..."
vhs "${SCENARIO_DIR}/crashloop-pods1.tape" &
PODS1_PID=$!
vhs "${SCENARIO_DIR}/crashloop-rr1.tape" &
RR1_PID=$!
sleep 5

# ── Step 3: Inject bad config ─────────────────
echo "==> [3/11] Injecting bad nginx config..."
bash "${SCENARIO_DIR}/inject-bad-config.sh"
echo "    Fault injected. Pods will begin crashing."

# ── Step 4: Wait for alert + capture data ─────
echo "==> [4/11] Waiting for KubePodCrashLooping alert to fire..."
ALERT_WAIT=0
while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 \
  -- amtool alert query alertname=KubePodCrashLooping \
  --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null \
  | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do
  sleep 10
  ALERT_WAIT=$((ALERT_WAIT + 10))
  echo "    Still waiting... (${ALERT_WAIT}s)"
done
echo "    Alert fired! Capturing alert data to ${ALERT_CACHE}..."
kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 \
  -- amtool alert query alertname=KubePodCrashLooping \
  --alertmanager.url=http://localhost:9093 --output=json > "${ALERT_CACHE}" 2>/dev/null || true

# ── Step 5: Wait for pods1 + rr1 to finish ────
echo "==> [5/11] Waiting for pods1 tape (CrashLoopBackOff detected)..."
wait $PODS1_PID 2>/dev/null || true
echo "    pods1 tape finished."

echo "    Waiting for rr1 tape (Analyzing detected)..."
wait $RR1_PID 2>/dev/null || true
echo "    rr1 tape finished."

# ── Step 6: Start rr2 (background) ────────────
echo "==> [6/11] Starting rr2 tape (-> AwaitingApproval)..."
vhs "${SCENARIO_DIR}/crashloop-rr2.tape" &
RR2_PID=$!

echo "    Waiting for rr2 tape (AwaitingApproval detected)..."
wait $RR2_PID 2>/dev/null || true
echo "    rr2 tape finished."

# ── Step 7: Start rr3 + pods2 before approval ─
echo "==> [7/11] Starting rr3 (-> Verifying) + pods2 (recovery) before approval..."
vhs "${SCENARIO_DIR}/crashloop-rr3.tape" &
RR3_PID=$!
vhs "${SCENARIO_DIR}/crashloop-pods2.tape" &
PODS2_PID=$!
sleep 5

# ── Step 8: Approve RAR ───────────────────────
echo "==> [8/11] Approving RAR..."
sleep 3
bash "${REPO_ROOT}/deploy/demo/scripts/approve-rar.sh"
echo "    RAR approved."

# ── Step 9: Wait for rr3, then stop pods2 ─────
echo "==> [9/11] Waiting for rr3 tape (Verifying detected)..."
wait $RR3_PID 2>/dev/null || true
echo "    rr3 tape finished."

echo "    Waiting 30s for pods to stabilize, then stopping pods2..."
sleep 30
touch "${PODS2_STOP_FILE}"
wait $PODS2_PID 2>/dev/null || true
rm -f "${PODS2_STOP_FILE}"
echo "    pods2 tape finished."

# ── Step 10: Start rr4 (-> Completed) ─────────
echo "==> [10/11] Starting rr4 tape (-> Completed, ~12 min stabilization)..."
vhs "${SCENARIO_DIR}/crashloop-rr4.tape" &
RR4_PID=$!

RR4_WAIT=0
while kill -0 $RR4_PID 2>/dev/null; do
  sleep 30
  RR4_WAIT=$((RR4_WAIT + 30))
  echo "    Still waiting for Completed... (${RR4_WAIT}s)"
done
wait $RR4_PID 2>/dev/null || true
echo "    rr4 tape finished."

# ── Step 11: Record screens tape (sequential) ─
echo "==> [11/11] Recording narration screens tape..."
export ALERT_CACHE
vhs "${SCENARIO_DIR}/crashloop-screens.tape"
echo "    Screens tape finished."

# ── Cleanup ───────────────────────────────────
echo ""
echo "==> Cleaning up scenario resources..."
bash "${SCENARIO_DIR}/cleanup.sh" 2>/dev/null || true

echo ""
echo "════════════════════════════════════════════"
echo "  Recording complete!"
echo ""
echo "  Raw files:"
echo "    ${SCENARIO_DIR}/crashloop-screens-raw.mp4"
echo "    ${SCENARIO_DIR}/crashloop-rr1-raw.mp4"
echo "    ${SCENARIO_DIR}/crashloop-rr2-raw.mp4"
echo "    ${SCENARIO_DIR}/crashloop-rr3-raw.mp4"
echo "    ${SCENARIO_DIR}/crashloop-rr4-raw.mp4"
echo "    ${SCENARIO_DIR}/crashloop-pods1-raw.mp4"
echo "    ${SCENARIO_DIR}/crashloop-pods2-raw.mp4"
echo ""
echo "  Next: bash deploy/demo/scripts/splice-demo.sh crashloop"
echo "════════════════════════════════════════════"
