#!/usr/bin/env bash
# Mini-coordinator: Re-record just pods2 (pod recovery after remediation).
# Re-runs the scenario from scratch, but only records the pods2 tape.
set -euo pipefail

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCENARIO_DIR}/../../../.." && pwd)"
PLATFORM_NS="${PLATFORM_NS:-kubernaut-system}"
export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/kubernaut-demo-config}"
export PLATFORM_NS

PODS2_STOP_FILE="/tmp/kubernaut-crashloop-pods2-stop"
rm -f "${PODS2_STOP_FILE}"

echo "==> Pods2 re-recording: setting up scenario..."
bash "${SCENARIO_DIR}/tape-setup.sh"
echo "    Setup complete."

echo "==> Injecting bad config..."
bash "${SCENARIO_DIR}/inject-bad-config.sh"

echo "==> Waiting for AwaitingApproval..."
AA_WAIT=0
while ! kubectl get remediationrequests -n "${PLATFORM_NS}" 2>/dev/null | grep -q AwaitingApproval; do
  sleep 5
  AA_WAIT=$((AA_WAIT + 5))
  echo "    Still waiting... (${AA_WAIT}s)"
done
echo "    AwaitingApproval reached!"

echo "==> Starting pods2 tape (background)..."
vhs "${SCENARIO_DIR}/crashloop-pods2.tape" &
PODS2_PID=$!
sleep 5

echo "==> Approving RAR..."
sleep 3
bash "${REPO_ROOT}/deploy/demo/scripts/approve-rar.sh"

echo "==> Waiting 60s for pods to stabilize, then stopping pods2..."
sleep 60
touch "${PODS2_STOP_FILE}"
wait $PODS2_PID 2>/dev/null || true
rm -f "${PODS2_STOP_FILE}"
echo "    pods2 tape finished."

echo "==> Cleaning up..."
bash "${SCENARIO_DIR}/cleanup.sh" 2>/dev/null || true
echo "==> Done!"
