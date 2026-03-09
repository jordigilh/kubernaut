#!/usr/bin/env bash
# Three-tape coordinator for the Cluster Autoscale demo.
#
# Usage: bash deploy/demo/scenarios/autoscale/record.sh
set -euo pipefail

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIO_NAME="autoscale"
DEMO_NS="demo-autoscale"
ALERT_NAME="KubePodSchedulingFailed"
RESOURCE_TAPE="autoscale-pods.tape"
SCREENS_TAPE="autoscale-screens.tape"
APPROVAL_REQUIRED="true"
TERMINAL_STATE="Completed"
INJECT_CMD="kubectl scale deployment/web-cluster --replicas=8 -n demo-autoscale"
SETUP_CMD="bash ${SCENARIO_DIR}/cleanup.sh 2>/dev/null || true && kubectl apply -f ${SCENARIO_DIR}/manifests/ && kubectl wait --for=condition=Available deployment/web-cluster -n demo-autoscale --timeout=120s"
CLEANUP_CMD="bash ${SCENARIO_DIR}/cleanup.sh"

source "$(cd "${SCENARIO_DIR}/../../../.." && pwd)/deploy/demo/scripts/record-scenario.sh"
