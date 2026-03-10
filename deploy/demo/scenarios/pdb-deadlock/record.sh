#!/usr/bin/env bash
# Three-tape coordinator for the PDB Deadlock demo.
#
# Usage: bash deploy/demo/scenarios/pdb-deadlock/record.sh
set -euo pipefail

SCENARIO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIO_NAME="pdb-deadlock"
DEMO_NS="demo-pdb"
ALERT_NAME="KubePodDisruptionBudgetAtLimit"
RESOURCE_TAPE="pdb-deadlock-pods.tape"
SCREENS_TAPE="pdb-deadlock-screens.tape"
APPROVAL_REQUIRED="true"
TERMINAL_STATE="Completed"
INJECT_CMD="bash ${SCENARIO_DIR}/inject-drain.sh"
SETUP_CMD="bash ${SCENARIO_DIR}/cleanup.sh 2>/dev/null || true && kubectl apply -f ${SCENARIO_DIR}/manifests/ && kubectl wait --for=condition=Available deployment/payment-service -n demo-pdb --timeout=120s"
CLEANUP_CMD="bash ${SCENARIO_DIR}/cleanup.sh"

source "$(cd "${SCENARIO_DIR}/../../../.." && pwd)/deploy/demo/scripts/record-scenario.sh"
