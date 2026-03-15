#!/usr/bin/env bash
# Three-tape coordinator for the GitOps Drift demo.
#
# Usage: bash deploy/demo/scenarios/gitops-drift/recording/record.sh
set -euo pipefail

RECORDING_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIO_DIR="$(cd "${RECORDING_DIR}/.." && pwd)"
SCENARIO_NAME="gitops-drift"
DEMO_NS="demo-gitops"
ALERT_NAME="KubePodCrashLooping"
RESOURCE_TAPE="gitops-drift-pods.tape"
SCREENS_TAPE="gitops-drift-screens.tape"
APPROVAL_REQUIRED="false"
TERMINAL_STATE="Completed"
INJECT_CMD="bash ${SCENARIO_DIR}/run.sh inject"  # scenario infra — update path when available
SETUP_CMD="bash ${SCENARIO_DIR}/run.sh setup"  # scenario infra — update path when available
CLEANUP_CMD="bash ${SCENARIO_DIR}/cleanup.sh"  # scenario infra — update path when available

export SCENARIO_DIR="${RECORDING_DIR}"
source "$(cd "${RECORDING_DIR}/../../../../.." && pwd)/deploy/demo/scripts/record-scenario.sh"
