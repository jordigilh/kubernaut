#!/usr/bin/env bash
# Three-tape coordinator for the SLO Burn demo.
#
# Usage: bash deploy/demo/scenarios/slo-burn/record.sh
set -euo pipefail

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIO_NAME="slo-burn"
DEMO_NS="demo-slo"
ALERT_NAME="ErrorBudgetBurn"
RESOURCE_TAPE="slo-burn-pods.tape"
SCREENS_TAPE="slo-burn-screens.tape"
APPROVAL_REQUIRED="true"
TERMINAL_STATE="Completed"
INJECT_CMD="bash ${SCENARIO_DIR}/inject-bad-config.sh"
SETUP_CMD="bash ${SCENARIO_DIR}/tape-setup.sh"
CLEANUP_CMD="bash ${SCENARIO_DIR}/cleanup.sh"

source "$(cd "${SCENARIO_DIR}/../../../.." && pwd)/deploy/demo/scripts/record-scenario.sh"
