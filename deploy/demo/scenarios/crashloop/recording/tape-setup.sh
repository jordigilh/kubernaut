#!/usr/bin/env bash
# VHS tape setup: cleanup + stabilization window + deploy
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
PLATFORM_NS="${PLATFORM_NS:-kubernaut-system}"

# Scenario infrastructure (cleanup, manifests) lives in the scenario root,
# maintained by main. These references must be updated when scenario infra
# is added back to the scenario directory.
bash "${SCENARIO_DIR}/cleanup.sh" 2>/dev/null || true

kubectl get configmap remediationorchestrator-config -n "${PLATFORM_NS}" -o yaml \
  | sed 's/stabilizationWindow: "[^"]*"/stabilizationWindow: "720s"/' \
  | kubectl apply -f - >/dev/null 2>&1
kubectl rollout restart deploy/remediationorchestrator-controller -n "${PLATFORM_NS}" >/dev/null 2>&1
kubectl rollout status deploy/remediationorchestrator-controller -n "${PLATFORM_NS}" --timeout=120s >/dev/null 2>&1

kubectl apply -f "${SCENARIO_DIR}/manifests/namespace.yaml" >/dev/null 2>&1
kubectl apply -f "${SCENARIO_DIR}/manifests/" >/dev/null 2>&1
kubectl wait --for=condition=Available deployment/worker -n demo-crashloop --timeout=180s >/dev/null 2>&1
sleep 20
