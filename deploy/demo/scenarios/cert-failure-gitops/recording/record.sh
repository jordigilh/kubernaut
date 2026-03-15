#!/usr/bin/env bash
# Three-tape coordinator for the Certificate Failure GitOps demo.
#
# Usage: bash deploy/demo/scenarios/cert-failure-gitops/recording/record.sh
set -euo pipefail

RECORDING_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIO_DIR="$(cd "${RECORDING_DIR}/.." && pwd)"
SCENARIO_NAME="cert-failure-gitops"
DEMO_NS="demo-cert-gitops"
ALERT_NAME="CertManagerCertNotReady"
RESOURCE_TAPE="cert-failure-gitops-certs.tape"
SCREENS_TAPE="cert-failure-gitops-screens.tape"
APPROVAL_REQUIRED="false"
TERMINAL_STATE="Completed"
INJECT_CMD="bash ${SCENARIO_DIR}/run.sh inject"  # scenario infra — update path when available
SETUP_CMD="bash ${SCENARIO_DIR}/run.sh setup"  # scenario infra — update path when available
CLEANUP_CMD="bash ${SCENARIO_DIR}/cleanup.sh"  # scenario infra — update path when available

export SCENARIO_DIR="${RECORDING_DIR}"
source "$(cd "${RECORDING_DIR}/../../../../.." && pwd)/deploy/demo/scripts/record-scenario.sh"
