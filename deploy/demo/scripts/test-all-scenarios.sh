#!/usr/bin/env bash
# test-all-scenarios.sh — Run every demo scenario sequentially, capture results.
#
# Usage:
#   bash deploy/demo/scripts/test-all-scenarios.sh
#   bash deploy/demo/scripts/test-all-scenarios.sh --start-from=memory-leak
#   bash deploy/demo/scripts/test-all-scenarios.sh --only=crashloop,memory-leak
#
# Results are written to deploy/demo/test-results-<timestamp>.txt
# Each scenario: run.sh --auto-approve → capture exit code → cleanup.sh → next
#
# Safe to run from a terminal outside Cursor (survives IDE crashes).
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIOS_DIR="${SCRIPT_DIR}/../scenarios"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
RESULTS_FILE="${SCRIPT_DIR}/../test-results-${TIMESTAMP}.txt"
LOGS_DIR="${SCRIPT_DIR}/../test-logs-${TIMESTAMP}"
mkdir -p "${LOGS_DIR}"

export KUBECONFIG="${HOME}/.kube/kubernaut-demo-config"
PLATFORM_NS="${PLATFORM_NS:-kubernaut-system}"

# Pre-flight: verify demo environment is set up
# shellcheck source=platform-helper.sh
source "${SCRIPT_DIR}/platform-helper.sh"
require_demo_ready

SCENARIO_ORDER=(
  crashloop
  crashloop-helm
  memory-leak
  autoscale
  hpa-maxed
  slo-burn
  stuck-rollout
  duplicate-alert-suppression
  mesh-routing-failure
  network-policy-block
  orphaned-pvc-no-action
  statefulset-pvc-failure
  resource-quota-exhaustion
  concurrent-cross-namespace
  memory-escalation
  resource-contention
  cert-failure
  cert-failure-gitops
  gitops-drift
  pdb-deadlock
  node-notready
  pending-taint
)

START_FROM=""
ONLY_LIST=""
for arg in "$@"; do
  case "$arg" in
    --start-from=*) START_FROM="${arg#*=}" ;;
    --only=*)       ONLY_LIST="${arg#*=}" ;;
  esac
done

should_run() {
  local name="$1"
  if [ -n "$ONLY_LIST" ]; then
    echo ",$ONLY_LIST," | grep -q ",$name," && return 0 || return 1
  fi
  return 0
}

passed=0
failed=0
skipped=0
declare -A results

header() {
  local msg="$1"
  printf '\n%s\n%s\n%s\n' \
    "$(printf '═%.0s' {1..60})" \
    "  $msg" \
    "$(printf '═%.0s' {1..60})"
}

{
  echo "Kubernaut Demo Scenario Test Run"
  echo "Started: $(date)"
  echo "Cluster: $(kubectl config current-context 2>/dev/null)"
  echo "Nodes:   $(kubectl get nodes --no-headers 2>/dev/null | wc -l | tr -d ' ')"
  echo ""
  echo "═══════════════════════════════════════════════════════════════"
} | tee "${RESULTS_FILE}"

started=false
if [ -z "$START_FROM" ]; then started=true; fi

for scenario in "${SCENARIO_ORDER[@]}"; do
  if ! $started; then
    if [ "$scenario" = "$START_FROM" ]; then
      started=true
    else
      results[$scenario]="SKIPPED (before --start-from)"
      skipped=$((skipped + 1))
      continue
    fi
  fi

  if ! should_run "$scenario"; then
    results[$scenario]="SKIPPED (not in --only)"
    skipped=$((skipped + 1))
    continue
  fi

  scenario_dir="${SCENARIOS_DIR}/${scenario}"
  if [ ! -f "${scenario_dir}/run.sh" ]; then
    results[$scenario]="SKIPPED (no run.sh)"
    skipped=$((skipped + 1))
    echo "  SKIP ${scenario}: no run.sh" | tee -a "${RESULTS_FILE}"
    continue
  fi

  log_file="${LOGS_DIR}/${scenario}.log"

  header "RUNNING: ${scenario}" | tee -a "${RESULTS_FILE}"
  echo "  Started: $(date)" | tee -a "${RESULTS_FILE}"
  start_ts=$(date +%s)

  # Run the scenario
  set +e
  bash "${scenario_dir}/run.sh" --auto-approve > "${log_file}" 2>&1
  run_exit=$?
  set -e

  end_ts=$(date +%s)
  elapsed=$(( end_ts - start_ts ))

  if [ $run_exit -eq 0 ]; then
    results[$scenario]="PASS (${elapsed}s)"
    passed=$((passed + 1))
    echo "  ✅ PASS  ${scenario}  (${elapsed}s)" | tee -a "${RESULTS_FILE}"
  else
    results[$scenario]="FAIL exit=${run_exit} (${elapsed}s)"
    failed=$((failed + 1))
    echo "  ❌ FAIL  ${scenario}  exit=${run_exit} (${elapsed}s)" | tee -a "${RESULTS_FILE}"
    echo "  Log: ${log_file}" | tee -a "${RESULTS_FILE}"
    # Show last 20 lines of failure
    echo "  --- last 20 lines ---" | tee -a "${RESULTS_FILE}"
    tail -20 "${log_file}" | sed 's/^/  | /' | tee -a "${RESULTS_FILE}"
    echo "  ---" | tee -a "${RESULTS_FILE}"
  fi

  # Cleanup
  echo "  Cleaning up ${scenario}..." | tee -a "${RESULTS_FILE}"
  if [ -f "${scenario_dir}/cleanup.sh" ]; then
    set +e
    bash "${scenario_dir}/cleanup.sh" >> "${log_file}" 2>&1
    cleanup_exit=$?
    set -e
    if [ $cleanup_exit -ne 0 ]; then
      echo "  ⚠️  Cleanup failed (exit=${cleanup_exit})" | tee -a "${RESULTS_FILE}"
    fi
  fi

  echo "  Finished: $(date)" | tee -a "${RESULTS_FILE}"

  # Brief pause between scenarios for AlertManager to settle
  echo "  Waiting 10s before next scenario..."
  sleep 10
done

# Summary
{
  echo ""
  echo "═══════════════════════════════════════════════════════════════"
  echo "  SUMMARY"
  echo "═══════════════════════════════════════════════════════════════"
  echo ""
  echo "  Passed:  ${passed}"
  echo "  Failed:  ${failed}"
  echo "  Skipped: ${skipped}"
  echo "  Total:   ${#SCENARIO_ORDER[@]}"
  echo ""
  echo "  Per-scenario results:"
  for scenario in "${SCENARIO_ORDER[@]}"; do
    printf "    %-35s %s\n" "${scenario}" "${results[$scenario]:-NOT RUN}"
  done
  echo ""
  echo "  Results: ${RESULTS_FILE}"
  echo "  Logs:    ${LOGS_DIR}/"
  echo ""
  echo "Finished: $(date)"
} | tee -a "${RESULTS_FILE}"
