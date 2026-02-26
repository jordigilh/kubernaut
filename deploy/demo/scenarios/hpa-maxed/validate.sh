#!/usr/bin/env bash
# Validate HPA Maxed Out scenario (#123)
# Waits for the full remediation pipeline, displays inline progress with LLM
# analysis, and asserts the expected outcome.
#
# Usage:
#   ./validate.sh                    # auto-approve (default)
#   ./validate.sh --interactive      # pause for manual RAR approval
#   ./validate.sh --no-color         # disable color output
#
# Prerequisites: run.sh has already deployed and injected load.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Parse flags
APPROVE_MODE="--auto-approve"
for arg in "$@"; do
    case "$arg" in
        --interactive) APPROVE_MODE="--interactive" ;;
        --no-color)    export NO_COLOR=1 ;;
    esac
done

# shellcheck source=../../scripts/validation-helper.sh
source "${SCRIPT_DIR}/../../scripts/validation-helper.sh"

NAMESPACE="demo-hpa"
ALERT_NAME="KubeHpaMaxedOut"
ALERT_TIMEOUT=300     # 5 min (for: 2m + scrape lag)
PIPELINE_TIMEOUT=600  # 10 min

echo ""
echo "  ${_c_bold}HPA Maxed Out Scenario Validation (#123)${_c_reset}"
echo "  ════════════════════════════════════════"
echo ""

# Phase 1: Wait for alert
wait_for_alert "$ALERT_NAME" "$NAMESPACE" "$ALERT_TIMEOUT"
show_alert "$ALERT_NAME"

# Phase 2: Wait for RR and poll pipeline to completion
wait_for_rr "$NAMESPACE" 120
poll_pipeline "$NAMESPACE" "$PIPELINE_TIMEOUT" "$APPROVE_MODE"

# Phase 3: Scenario-specific assertions
PHASE=$(get_rr_phase "$NAMESPACE")
OUTCOME=$(get_rr_outcome "$NAMESPACE")
EA_PHASE=$(get_ea_phase "$NAMESPACE")
MAX_REPLICAS=$(kubectl get hpa api-frontend -n "$NAMESPACE" -o jsonpath='{.spec.maxReplicas}' 2>/dev/null || echo "0")

assert_eq "$PHASE" "Completed" "RR overallPhase"
assert_eq "$OUTCOME" "Remediated" "RR outcome"
assert_gt "$MAX_REPLICAS" 3 "HPA maxReplicas raised"
assert_eq "$EA_PHASE" "Completed" "EA phase"

print_result "hpa-maxed"
