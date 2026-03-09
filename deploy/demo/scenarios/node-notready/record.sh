#!/usr/bin/env bash
# Three-tape coordinator for the Node NotReady demo.
#
# Usage: bash deploy/demo/scenarios/node-notready/record.sh
set -euo pipefail

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIO_NAME="node-notready"
DEMO_NS="demo-node"
ALERT_NAME="KubeNodeNotReady"
RESOURCE_TAPE="node-notready-pods.tape"
SCREENS_TAPE="node-notready-screens.tape"
APPROVAL_REQUIRED="true"
TERMINAL_STATE="Completed"
INJECT_CMD="bash ${SCENARIO_DIR}/inject-node-failure.sh"
SETUP_CMD="bash ${SCENARIO_DIR}/tape-setup.sh"
CLEANUP_CMD="bash ${SCENARIO_DIR}/cleanup.sh"

source "$(cd "${SCENARIO_DIR}/../../../.." && pwd)/deploy/demo/scripts/record-scenario.sh"
