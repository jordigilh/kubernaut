#!/usr/bin/env bash
# Three-tape coordinator for the Stuck Rollout demo.
#
# Usage: bash deploy/demo/scenarios/stuck-rollout/record.sh
set -euo pipefail

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIO_NAME="stuck-rollout"
DEMO_NS="demo-rollout"
ALERT_NAME="KubeDeploymentRolloutStuck"
RESOURCE_TAPE="stuck-rollout-pods.tape"
SCREENS_TAPE="stuck-rollout-screens.tape"
APPROVAL_REQUIRED="true"
TERMINAL_STATE="Completed"
INJECT_CMD="bash ${SCENARIO_DIR}/inject-bad-image.sh"
SETUP_CMD="bash ${SCENARIO_DIR}/cleanup.sh 2>/dev/null || true && kubectl apply -f ${SCENARIO_DIR}/manifests/ && kubectl wait --for=condition=Available deployment/checkout-api -n demo-rollout --timeout=120s"
CLEANUP_CMD="bash ${SCENARIO_DIR}/cleanup.sh"

source "$(cd "${SCENARIO_DIR}/../../../.." && pwd)/deploy/demo/scripts/record-scenario.sh"
