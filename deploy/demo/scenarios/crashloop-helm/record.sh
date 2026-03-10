#!/usr/bin/env bash
# Three-tape coordinator for the CrashLoop Helm demo.
#
# Usage: bash deploy/demo/scenarios/crashloop-helm/record.sh
set -euo pipefail

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIO_NAME="crashloop-helm"
DEMO_NS="demo-crashloop-helm"
ALERT_NAME="KubePodCrashLooping"
RESOURCE_TAPE="crashloop-helm-pods.tape"
SCREENS_TAPE="crashloop-helm-screens.tape"
APPROVAL_REQUIRED="true"
TERMINAL_STATE="Completed"
INJECT_CMD="bash ${SCENARIO_DIR}/inject-bad-config.sh"
SETUP_CMD="bash ${SCENARIO_DIR}/cleanup.sh 2>/dev/null || true && helm upgrade --install demo-crashloop-helm ${SCENARIO_DIR}/chart -n demo-crashloop-helm --create-namespace --wait --timeout 120s && kubectl apply -f ${SCENARIO_DIR}/manifests/prometheus-rule.yaml"
CLEANUP_CMD="bash ${SCENARIO_DIR}/cleanup.sh"

source "$(cd "${SCENARIO_DIR}/../../../.." && pwd)/deploy/demo/scripts/record-scenario.sh"
